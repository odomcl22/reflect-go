package main

// The gateway owns the public endpoint. Runtime processes are private, and the
// request lease remains held until streaming finishes, so switching cannot evict
// a model that another request is using.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func modelID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "reflect-" + hex.EncodeToString(sum[:8])
}

type GatewayState struct {
	Stage       string  `json:"stage"`
	Model       string  `json:"model"`
	Error       string  `json:"error,omitempty"`
	LoadSeconds float64 `json:"loadSeconds"`
	PID         int     `json:"pid"`
}
type ModelGateway struct {
	lease                         sync.Mutex
	mu                            sync.Mutex
	root, key, host, defaultModel string
	models                        map[string]Model
	ctx                           context.Context
	load                          func(context.Context, Model) (string, error)
	state                         GatewayState
	target                        string
}

func (g *ModelGateway) snapshot() GatewayState { g.mu.Lock(); defer g.mu.Unlock(); return g.state }
func (g *ModelGateway) setState(stage, model, detail string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state.Stage = stage
	g.state.Model = model
	g.state.Error = detail
}
func (g *ModelGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if g.host != "" && r.Host != g.host {
		http.Error(w, "Invalid host", 403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+g.host {
		http.Error(w, "Invalid origin", 403)
		return
	}
	auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if auth == r.Header.Get("Authorization") {
		auth = r.Header.Get("x-api-key")
	}
	if subtle.ConstantTimeCompare([]byte(auth), []byte(g.key)) != 1 {
		http.Error(w, "Invalid API key. Reopen the client from Reflect Go.", 401)
		return
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/v1/models":
			data := []map[string]any{}
			for id, m := range g.models {
				data = append(data, map[string]any{"id": id, "object": "model", "owned_by": "reflect-go", "name": m.Name})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data})
			return
		case "/reflect/status":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(g.snapshot())
			return
		case "/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		case "/metrics":
			// Metrics never compete with inference or trigger a model load.
			g.mu.Lock()
			target := g.target
			g.mu.Unlock()
			if target == "" {
				fmt.Fprintln(w, "# No model loaded")
				return
			}
			g.proxy(w, r, target, nil)
			return
		}
	}
	allowed := r.URL.Path == "/v1/chat/completions" || r.URL.Path == "/v1/responses" || r.URL.Path == "/v1/responses/compact" || r.URL.Path == "/v1/messages" || r.URL.Path == "/v1/messages/count_tokens"
	if !allowed || r.Method != "POST" {
		http.Error(w, "Unsupported local API route", 404)
		return
	}
	if !g.lease.TryLock() {
		w.Header().Set("Retry-After", "2")
		http.Error(w, "The local model is busy. Finish or cancel the active response, then retry.", 409)
		return
	}
	defer g.lease.Unlock()
	var body map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		http.Error(w, "Invalid or oversized JSON request", 400)
		return
	}
	var id string
	json.Unmarshal(body["model"], &id)
	if id == "" || id == "reflect-go-local" {
		id = g.defaultModel
	}
	m, ok := g.models[id]
	if !ok {
		http.Error(w, "Model is unavailable on this computer. Refresh the client catalog or choose a compatible model.", 400)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(g.ctx, cancel)
	defer stop()
	state := g.snapshot()
	g.mu.Lock()
	target := g.target
	g.mu.Unlock()
	if state.Model != m.Name || target == "" {
		start := time.Now()
		g.setState("loading", m.Name, "")
		var err error
		target, err = g.load(ctx, m)
		if err != nil {
			g.mu.Lock()
			g.target = ""
			g.mu.Unlock()
			g.setState("error", m.Name, err.Error())
			http.Error(w, "Model could not load: "+err.Error(), 503)
			return
		}
		g.mu.Lock()
		g.target = target
		g.state.LoadSeconds = time.Since(start).Seconds()
		g.mu.Unlock()
	}
	g.setState("working", m.Name, "")
	defer func() {
		if g.snapshot().Stage == "working" {
			g.setState("ready", m.Name, "")
		}
	}()
	body["model"], _ = json.Marshal(id)
	data, _ := json.Marshal(body)
	g.proxy(w, r.WithContext(ctx), target, data)
}
func (g *ModelGateway) proxy(w http.ResponseWriter, r *http.Request, target string, data []byte) {
	u, _ := url.Parse(target)
	p := httputil.NewSingleHostReverseProxy(u)
	p.Transport = localHTTP.Transport
	p.FlushInterval = -1
	if data != nil {
		r.Body = io.NopCloser(bytes.NewReader(data))
		r.ContentLength = int64(len(data))
	}
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		g.mu.Lock()
		if g.target == target {
			g.target = ""
		}
		g.mu.Unlock()
		g.setState("error", g.snapshot().Model, "Runtime connection ended. Retry to reload the model.")
		http.Error(w, "Local runtime connection ended. Retry or restart the engine.", 502)
	}
	p.ServeHTTP(w, r)
}

func serveGateway(root string, s Settings, o Options, inv Inventory) error {
	lock, err := lockProvider(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", o.Port))
	if err != nil {
		return fmt.Errorf("port %d is in use; choose another port", o.Port)
	}
	defer listener.Close()
	keyPath, err := providerKey(root)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	g := &ModelGateway{root: root, key: strings.TrimSpace(string(data)), host: listener.Addr().String(), models: map[string]Model{}, ctx: ctx, state: GatewayState{Stage: "available"}}
	for _, m := range inv.Models {
		if m.Eligible && (o.EnabledIDs == "" || strings.Contains(","+o.EnabledIDs+",", ","+modelID(m.Name)+",")) {
			id := modelID(m.Name)
			g.models[id] = m
			if g.defaultModel == "" || m.Name == o.Model || m.Name == inv.DefaultModel && o.Model == "" {
				g.defaultModel = id
			}
		}
	}
	var child *exec.Cmd
	var done chan error
	stopChild := func() {
		if child == nil {
			return
		}
		child.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			child.Process.Kill()
			<-done
		}
		child = nil
		g.mu.Lock()
		g.target = ""
		g.state.PID = 0
		g.mu.Unlock()
	}
	defer stopChild()
	g.load = func(requestCtx context.Context, m Model) (string, error) {
		stopChild()
		portListener, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			return "", e
		}
		port := portListener.Addr().(*net.TCPAddr).Port
		portListener.Close()
		self, e := os.Executable()
		if e != nil {
			return "", e
		}
		args := []string{"serve", "--internal", "--model", m.Name, "--port", strconv.Itoa(port), "--context", strconv.Itoa(inv.Context), "--backend", o.Backend, "--temperature", strconv.FormatFloat(o.Temperature, 'f', -1, 64)}
		if o.Temperature < 0 {
			args[len(args)-1] = strconv.FormatFloat(s.Temperature, 'f', -1, 64)
		}
		if o.CPU { args = append(args,"--cpu") }
		if o.Threads > 0 {
			args = append(args, "--threads", strconv.Itoa(o.Threads))
		}
		child = exec.Command(self, args...)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		hideEngineWindow(child)
		if e = child.Start(); e != nil {
			child = nil
			return "", e
		}
		g.mu.Lock()
		g.state.PID = child.Process.Pid
		g.mu.Unlock()
		done = make(chan error, 1)
		cmd := child
		go func() { done <- cmd.Wait() }()
		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		timer := time.NewTimer(10 * time.Minute)
		defer timer.Stop()
		tick := time.NewTicker(300 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-requestCtx.Done():
				stopChild()
				return "", requestCtx.Err()
			case <-timer.C:
				stopChild()
				return "", errors.New("loading exceeded ten minutes; try a smaller model or context")
			case e := <-done:
				child = nil
				return "", fmt.Errorf("runtime exited (%v); review Diagnostics", e)
			case <-tick.C:
				req, _ := http.NewRequestWithContext(requestCtx, "GET", base+"/health", nil)
				req.Header.Set("Authorization", "Bearer "+g.key)
				response, e := localHTTP.Do(req)
				if e == nil {
					response.Body.Close()
					if response.StatusCode == 200 {
						return base, nil
					}
				}
			}
		}
	}
	path, err := statePath(root, "provider.json")
	if err != nil {
		return err
	}
	defaultFile := ""
	if m, ok := g.models[g.defaultModel]; ok {
		defaultFile = m.Name
	}
	if err = writeJSON(path, map[string]any{"base_url": "http://" + g.host, "model": g.defaultModel, "model_file": defaultFile, "context": inv.Context, "backend": inv.Hardware.Backend.Kind, "device": inv.Hardware.Backend.Device, "gateway": true}); err != nil {
		return err
	}
	defer os.Remove(path)
	server := &http.Server{Handler: g, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	defer server.Close()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	fmt.Println("llama_server: listening on http://" + g.host)
	fmt.Println("Reflect Go model gateway available. Models load when requested.")
	select {
	case <-ctx.Done():
	case err = <-serverDone:
		cancel()
	}
	// Cancel active requests before waiting for their lease and unloading the child.
	cancel()
	server.Close()
	g.lease.Lock()
	defer g.lease.Unlock()
	return err
}
