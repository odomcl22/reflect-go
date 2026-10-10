package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

func productRoutes(mux *http.ServeMux, root string, manager *EngineManager, downloads *DownloadManager) {
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	post := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "POST JSON required", 405)
			return false
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		d.DisallowUnknownFields()
		if err := d.Decode(v); err != nil {
			http.Error(w, "Invalid request", 400)
			return false
		}
		if d.Decode(new(any)) != io.EOF {
			http.Error(w, "Unexpected trailing data", 400)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/preferences", func(w http.ResponseWriter, r *http.Request) {
		path, e := statePath(root, "appearance.json")
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		p := struct {
			Theme string `json:"theme"`
		}{"system"}
		if r.Method == "POST" {
			if !post(w, r, &p) {
				return
			}
			if p.Theme != "dark" && p.Theme != "light" && p.Theme != "system" {
				http.Error(w, "Choose Dark, Light or System", 400)
				return
			}
			if e = writeJSON(path, p); e != nil {
				http.Error(w, e.Error(), 500)
				return
			}
		} else if r.Method == "GET" {
			_ = readJSON(path, &p)
		} else {
			http.Error(w, "Method not allowed", 405)
			return
		}
		send(w, p)
	})
	mux.HandleFunc("/api/catalog", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"models": modelCatalog(), "download": downloads.snapshot()})
	})
	mux.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			ID string `json:"id"`
		}
		if !post(w, r, &p) {
			return
		}
		if err := downloads.start(p.ID); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		send(w, downloads.snapshot())
	})
	mux.HandleFunc("/api/download/cancel", func(w http.ResponseWriter, r *http.Request) {
		var p struct{}
		if !post(w, r, &p) {
			return
		}
		downloads.stop()
		send(w, downloads.snapshot())
	})
	mux.HandleFunc("/api/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if r.ContentLength <= 0 {
			http.Error(w, "File size required", 400)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<30)
		if err := importModel(root, r.URL.Query().Get("name"), r.Body, r.ContentLength); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		send(w, map[string]bool{"imported": true})
	})
	mux.HandleFunc("/api/recommend", func(w http.ResponseWriter, r *http.Request) {
		s, e := loadSettings(root)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		inv, e := inventory(root, s, Options{Context: 2048, Backend: "auto"})
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		purpose := r.URL.Query().Get("purpose")
		if purpose != "coding" && purpose != "chat" {
			http.Error(w, "Choose chat or coding", 400)
			return
		}
		send(w, recommend(inv, purpose))
	})
	mux.HandleFunc("/api/provider-models", func(w http.ResponseWriter, r *http.Request) {
		response, e := localRequest(root, "GET", "/v1/models", nil)
		if e != nil {
			http.Error(w, "Start the engine first", 409)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		io.Copy(w, io.LimitReader(response.Body, 1<<20))
	})
	var chatLease, chatMu sync.Mutex
	var cancelChat context.CancelFunc
	mux.HandleFunc("/api/chat/cancel", func(w http.ResponseWriter, r *http.Request) {
		var p struct{}
		if !post(w, r, &p) {
			return
		}
		chatMu.Lock()
		if cancelChat != nil {
			cancelChat()
		}
		chatMu.Unlock()
		send(w, map[string]bool{"cancelled": true})
	})
	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if !post(w, r, &p) {
			return
		}
		if !chatLease.TryLock() {
			http.Error(w, "A chat response is already active", 409)
			return
		}
		defer chatLease.Unlock()
		ctx, cancel := context.WithCancel(r.Context())
		chatMu.Lock()
		cancelChat = cancel
		chatMu.Unlock()
		defer func() { cancel(); chatMu.Lock(); cancelChat = nil; chatMu.Unlock() }()
		if !manager.snapshot().Ready {
			http.Error(w, "Start the engine before chatting", 409)
			return
		}
		if len(p.Messages) == 0 || len(p.Messages) > 100 {
			http.Error(w, "Start a new conversation; the message limit is 100", 400)
			return
		}
		messages := []map[string]string{{"role": "system", "content": "You run locally through Reflect Go. Your weights live on the portable drive; this computer supplies compute. You have no web, file or terminal tools in this chat."}}
		for _, m := range p.Messages {
			if m.Role != "user" && m.Role != "assistant" {
				http.Error(w, "Invalid message role", 400)
				return
			}
			messages = append(messages, map[string]string{"role": m.Role, "content": m.Content})
		}
		c, key, e := connection(root)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		body, _ := json.Marshal(map[string]any{"model": p.Model, "messages": messages, "stream": true, "stream_options": map[string]bool{"include_usage": true}, "max_tokens": min(2048, c.Context/4), "temperature": .2})
		req, e := http.NewRequestWithContext(ctx, "POST", c.Base+"/v1/chat/completions", bytes.NewReader(body))
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		client := *localHTTP
		client.Timeout = 0
		response, e := client.Do(req)
		if e != nil {
			http.Error(w, "Local response ended; check Diagnostics or retry", 502)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(response.StatusCode)
		buffer := make([]byte, 4096)
		for {
			n, err := response.Body.Read(buffer)
			if n > 0 {
				if _, e = w.Write(buffer[:n]); e != nil {
					return
				}
				if flush, ok := w.(http.Flusher); ok {
					flush.Flush()
				}
			}
			if err != nil {
				return
			}
		}
	})
}
func productStatus(root string) map[string]any {
	reports := map[string]any{}
	home, _ := os.UserHomeDir()
	for _, name := range []string{"codex", "claude", "opencode"} {
		path, e := statePath(root, "client-launch-"+name+".json")
		if e != nil {
			continue
		}
		var report map[string]any
		if readJSON(path, &report) == nil {
			if detail, ok := report["error"].(string); ok {
				detail = strings.ReplaceAll(detail, root, "[drive]")
				if home != "" {
					detail = strings.ReplaceAll(detail, home, "[home]")
				}
				report["error"] = secretPattern.ReplaceAllString(detail, "[redacted]")
			}
			reports[name] = report
		}
	}
	return reports
}
func gatewayStatus(root string) (GatewayState, error) {
	response, e := localRequest(root, "GET", "/reflect/status", nil)
	if e != nil {
		return GatewayState{}, e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return GatewayState{}, errors.New("gateway unavailable")
	}
	var s GatewayState
	e = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&s)
	return s, e
}

// Keep diagnostics copy separate from raw logs and user-supplied paths.
func safeError(root, text string) string {
	home, _ := os.UserHomeDir()
	text = strings.ReplaceAll(text, root, "[drive]")
	if home != "" {
		text = strings.ReplaceAll(text, home, "[home]")
	}
	return secretPattern.ReplaceAllString(text, "[redacted]")
}
