package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"github.com/klauspost/compress/zstd"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// A private, unguessable URL keeps native desktop authentication separate from
// Reflect Go's engine credential. Local requests never fall back to the cloud.
type codexDesktopProxy struct {
	host, prefix, key   string
	models              map[string]bool
	local, api, account *httputil.ReverseProxy
}

func (p *codexDesktopProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != p.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+p.host) {
		http.Error(w, "Invalid local origin", 403)
		return
	}
	suffix, ok := strings.CutPrefix(r.URL.Path, p.prefix)
	if !ok || !strings.HasPrefix(suffix, "/") {
		http.NotFound(w, r)
		return
	}
	if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "Use HTTP Responses transport", 426)
		return
	}
	if r.Method != "POST" || !containsID([]string{"/responses", "/v1/responses", "/responses/compact", "/v1/responses/compact"}, suffix) {
		http.NotFound(w, r)
		return
	}
	body := http.MaxBytesReader(w, r.Body, 32<<20)
	defer body.Close()
	var reader io.Reader = body
	switch r.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(body)
		if err != nil {
			http.Error(w, "Invalid compressed request", 400)
			return
		}
		defer gz.Close()
		reader = gz
	case "zstd":
		z, err := zstd.NewReader(body, zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderConcurrency(1))
		if err != nil {
			http.Error(w, "Invalid compressed request", 400)
			return
		}
		defer z.Close()
		reader = z
	default:
		http.Error(w, "Unsupported request encoding", 415)
		return
	}
	data, err := io.ReadAll(io.LimitReader(reader, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		http.Error(w, "Request is too large or incomplete", 413)
		return
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(data, &payload) != nil {
		http.Error(w, "Invalid request JSON", 400)
		return
	}
	var model string
	_ = json.Unmarshal(payload["model"], &model)
	r.URL.Path = "/" + strings.TrimPrefix(strings.TrimPrefix(suffix, "/"), "v1/")
	var proxy *httputil.ReverseProxy
	if p.models[model] {
		if strings.HasSuffix(r.URL.Path, "/compact") {
			http.Error(w, "Local desktop compaction is not available; start a new conversation before the context fills", 400)
			return
		}
		// These account-only controls are not capabilities of the local model.
		delete(payload, "service_tier")
		delete(payload, "reasoning")
		var outputTokens int
		_ = json.Unmarshal(payload["max_output_tokens"], &outputTokens)
		if outputTokens <= 0 || outputTokens > 4096 {
			payload["max_output_tokens"] = json.RawMessage("4096")
		}
		payload["store"] = json.RawMessage("false")
		data, _ = json.Marshal(payload)
		r.Header = make(http.Header)
		r.Header.Set("Authorization", "Bearer "+p.key)
		r.Header.Set("Content-Type", "application/json")
		proxy = p.local
	} else {
		if model == "" || strings.HasPrefix(model, "reflect-") {
			http.Error(w, "This local model is not offered; reconnect Reflect Go to refresh the catalog", 400)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" || auth == "Bearer "+p.key || strings.Contains(auth, "reflect-go-local") || strings.Contains(auth, "ollama-local") {
			http.Error(w, "Your normal provider requires its own sign-in", 401)
			return
		}
		proxy = p.api
		if r.Header.Get("ChatGPT-Account-ID") != "" {
			proxy = p.account
		}
		r.Header.Del("Cookie")
		r.Header.Del("Origin")
		r.Header.Del("Proxy-Authorization")
	}
	r.Header.Del("Content-Encoding")
	r.Header.Del("Content-Length")
	r.Header.Set("Content-Type", "application/json")
	r.Body = io.NopCloser(bytes.NewReader(data))
	r.ContentLength = int64(len(data))
	r.URL.RawPath = ""
	proxy.ServeHTTP(w, r)
}
func desktopReverseProxy(base string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	if target.Host == "" {
		return nil, errors.New("missing provider host")
	}
	p := httputil.NewSingleHostReverseProxy(target)
	director := p.Director
	p.Director = func(r *http.Request) { director(r); r.Host = target.Host }
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	p.Transport = transport
	p.FlushInterval = -1
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "The selected provider is unavailable. Local requests are never sent to a cloud model as a fallback.", 503)
	}
	return p, nil
}
func startCodexDesktopProxy(c Connection, key string, models []ClientModel) (string, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	p := &codexDesktopProxy{host: listener.Addr().String(), prefix: "/reflect-go/" + key, key: key, models: map[string]bool{}}
	for _, m := range models {
		p.models[m.ID] = true
	}
	p.local, err = desktopReverseProxy(c.Base + "/v1")
	if err != nil {
		listener.Close()
		return "", nil, err
	}
	p.api, _ = desktopReverseProxy("https://api.openai.com/v1")
	p.account, _ = desktopReverseProxy("https://chatgpt.com/backend-api/codex")
	server := &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go server.Serve(listener)
	return "http://" + p.host + p.prefix, func() { server.Close() }, nil
}
