package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed ui/*
var uiFiles embed.FS

type RunSettings struct {
	Model       string   `json:"model"`
	Enabled     []string `json:"enabled,omitempty"`
	Host        string   `json:"host,omitempty"`
	Context     int      `json:"context"`
	Backend     string   `json:"backend"`
	Threads     int      `json:"threads"`
	Temperature float64  `json:"temperature"`
	Port        int      `json:"port"`
}

func defaultRunSettings() RunSettings {
	return RunSettings{Context: 16384, Backend: "auto", Temperature: .2, Port: 8766}
}
func (s RunSettings) options() (Options, error) {
	args := []string{"serve", "--context", strconv.Itoa(s.Context), "--backend", s.Backend, "--temperature", strconv.FormatFloat(s.Temperature, 'f', -1, 64), "--port", strconv.Itoa(s.Port)}
	if s.Threads != 0 {
		args = append(args, "--threads", strconv.Itoa(s.Threads))
	}
	if s.Model != "" {
		args = append(args, "--model", s.Model)
	}
	return parseOptions(args)
}

type EngineStatus struct {
	Active         bool        `json:"active"`
	Ready          bool        `json:"ready"`
	Stopping       bool        `json:"stopping"`
	Preparing      bool        `json:"preparing"`
	Testing        bool        `json:"testing"`
	Status         string      `json:"status"`
	Error          string      `json:"error"`
	Log            string      `json:"log"`
	Generated      int         `json:"generated"`
	Speed          float64     `json:"speed"`
	ResidentBytes  uint64      `json:"residentBytes"`
	StartupSeconds float64     `json:"startupSeconds"`
	Settings       RunSettings `json:"settings"`
	Backend        string      `json:"backend"`
	Device         string      `json:"device"`
}
type EngineManager struct {
	startMu    sync.Mutex // Serialize preparation and cancellation before another start.
	mu         sync.Mutex
	root       string
	status     EngineStatus
	owner      *os.File
	lock       *os.File
	done       chan struct{}
	session    uint64
	started    time.Time
	announced  bool
	enginePID  int
	failures   int
	cancelTest context.CancelFunc
}

func newManager(root string) *EngineManager {
	m := &EngineManager{root: root}
	m.status.Status = "Engine stopped"
	m.status.Settings = defaultRunSettings()
	if path, err := statePath(root, "workstation-web.json"); err == nil {
		var s RunSettings
		if err = readJSON(path, &s); err == nil {
			if _, e := s.options(); e == nil {
				m.status.Settings = s
			} else {
				m.status.Error = "Saved settings were invalid. Review the restored defaults."
			}
		} else if !os.IsNotExist(err) {
			m.status.Error = "Saved settings could not be read. Review the restored defaults."
		}
	}
	return m
}
func (m *EngineManager) snapshot() EngineStatus { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *EngineManager) start(s RunSettings) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	o, err := s.options()
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.status.Active || m.status.Preparing {
		m.mu.Unlock()
		return errors.New("stop the current engine first")
	}
	m.session++
	id := m.session
	m.status.Preparing = true
	m.status.Error = ""
	m.status.Status = "Checking hardware and model"
	m.mu.Unlock()
	var lock *os.File
	fail := func(err error) error {
		if lock != nil {
			lock.Close()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.session == id {
			m.status.Preparing = false
			m.status.Status = "Could not start"
			m.status.Error = err.Error()
		}
		return err
	}
	cfg, err := loadSettings(m.root)
	if err != nil {
		return fail(err)
	}
	inv, err := inventory(m.root, cfg, o)
	if err != nil {
		return fail(err)
	}
	if !inv.Hardware.Backend.Available {
		return fail(errors.New(inv.Hardware.Backend.Reason))
	}
	if o.Threads > inv.Hardware.CPUThreads {
		return fail(errors.New("thread count exceeds this computer's available CPUs"))
	}
	// The selected model is a preferred default; loading is deferred to requests.
	lock, err = lockNamed(m.root, "workstation.lock")
	if err != nil {
		return fail(errors.New("this phase already has an engine open in another window"))
	}
	path, err := statePath(m.root, "workstation-engine.log")
	if err != nil {
		return fail(err)
	}
	previous, err := statePath(m.root, "workstation-previous.log")
	if err != nil {
		return fail(err)
	}
	if _, err = os.Stat(path); err == nil {
		if err = os.Remove(previous); err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
		if err = os.Rename(path, previous); err != nil {
			return fail(err)
		}
	}
	log, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	defer log.Close()
	self, _ := os.Executable()
	args := []string{"_supervise", "gateway", "--model", s.Model, "--context", strconv.Itoa(s.Context), "--backend", s.Backend, "--temperature", strconv.FormatFloat(s.Temperature, 'f', -1, 64), "--port", strconv.Itoa(s.Port)}
	if len(s.Enabled) > 0 {
		args = append(args, "--enabled-ids", strings.Join(s.Enabled, ","))
	}
	if s.Threads > 0 {
		args = append(args, "--threads", strconv.Itoa(s.Threads))
	}
	read, write, err := os.Pipe()
	if err != nil {
		return fail(err)
	}
	defer read.Close()
	cmd := exec.Command(self, args...)
	cmd.Stdin = read
	cmd.Stdout = log
	cmd.Stderr = log
	m.mu.Lock()
	if m.session != id {
		m.mu.Unlock()
		write.Close()
		return fail(errors.New("start cancelled"))
	}
	hideEngineWindow(cmd)
	if err = cmd.Start(); err != nil {
		m.mu.Unlock()
		write.Close()
		return fail(err)
	}
	m.owner = write
	m.lock = lock
	lock = nil
	m.done = make(chan struct{})
	done := m.done
	m.started = time.Now()
	m.announced = false
	m.enginePID = 0
	m.failures = 0
	m.status = EngineStatus{Active: true, Status: "Starting model service", Settings: s}
	m.mu.Unlock()
	if prefs, e := statePath(m.root, "workstation-web.json"); e == nil {
		writeJSON(prefs, s)
	}
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		if m.session == id {
			requested := m.status.Stopping
			m.status.Active = false
			m.status.Ready = false
			m.status.Stopping = false
			m.status.Testing = false
			m.status.ResidentBytes = 0
			m.status.Status = "Engine stopped"
			if m.cancelTest != nil {
				m.cancelTest()
			}
			if !requested {
				m.status.Status = "Engine exited"
				m.status.Error = fmt.Sprintf("Engine exited: %v. Review Diagnostics and retry.", err)
			}
			if m.owner != nil {
				m.owner.Close()
				m.owner = nil
			}
			if m.lock != nil {
				m.lock.Close()
				m.lock = nil
			}
			m.status.Log = readEngineLog(m.root)
			if !requested {
				for _, line := range strings.Split(m.status.Log, "\n") {
					if strings.Contains(line, "Reflect Go:") && !strings.HasPrefix(line, "Reflect Go: exit status ") {
						m.status.Error = line
					}
				}
			}
		}
		close(done)
		m.mu.Unlock()
	}()
	go m.poll(id, done)
	return nil
}
func (m *EngineManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Preparing {
		m.session++
		m.status.Preparing = false
		m.status.Status = "Engine stopped"
	}
	if m.status.Active && !m.status.Stopping {
		m.status.Stopping = true
		m.status.Ready = false
		m.status.Status = "Stopping engine"
		if m.cancelTest != nil {
			m.cancelTest()
		}
		if m.owner != nil {
			m.owner.Close()
			m.owner = nil
		}
	}
}
func (m *EngineManager) waitStopped() error {
	m.stop()
	m.mu.Lock()
	done := m.done
	m.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(12 * time.Second):
		return errors.New("engine shutdown is still pending; do not eject the drive")
	}
}

var secretPattern = regexp.MustCompile(`\b[0-9a-fA-F]{64}\b`)
var pidPattern = regexp.MustCompile(`Reflect Go (engine|runtime) PID: (\d+)`)

func readEngineLog(root string) string {
	path, err := statePath(root, "workstation-engine.log")
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > 48000 {
		f.Seek(-48000, io.SeekEnd)
	}
	data, _ := io.ReadAll(io.LimitReader(f, 48000))
	return secretPattern.ReplaceAllString(string(data), "[redacted]")
}
func (m *EngineManager) poll(id uint64, done chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		log := readEngineLog(m.root)
		m.mu.Lock()
		if m.session != id || !m.status.Active || m.status.Stopping {
			m.mu.Unlock()
			continue
		}
		m.status.Log = log
		for _, p := range pidPattern.FindAllStringSubmatch(log, -1) {
			if m.enginePID == 0 || p[1] == "runtime" {
				m.enginePID, _ = strconv.Atoi(p[2])
			}
		}
		base := fmt.Sprintf("http://127.0.0.1:%d", m.status.Settings.Port)
		if strings.Contains(log, "llama_server: listening on "+base) {
			m.announced = true
		}
		announced := m.announced
		pid := m.enginePID
		// Cold model reads from a portable drive can exceed five minutes.
		timeout := m.status.StartupSeconds == 0 && time.Since(m.started) > 10*time.Minute
		m.mu.Unlock()
		if timeout {
			m.mu.Lock()
			m.status.Error = "Startup exceeded ten minutes. Check Diagnostics, then try a smaller model/context."
			m.mu.Unlock()
			m.stop()
			continue
		}
		if pid > 0 {
			ram := processMemory(pid)
			m.mu.Lock()
			if m.session == id {
				m.status.ResidentBytes = ram
			}
			m.mu.Unlock()
		}
		if !announced {
			continue
		}
		response, err := localRequest(m.root, "GET", "/metrics", nil)
		var metrics map[string]float64
		if err == nil {
			if response.StatusCode != 200 {
				err = errors.New("metrics unavailable")
			} else {
				data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				metrics = parseMetrics(string(data))
			}
			response.Body.Close()
		}
		m.mu.Lock()
		if m.session != id || !m.status.Active || m.status.Stopping {
			m.mu.Unlock()
			continue
		}
		if err != nil {
			m.failures++
			m.status.Ready = false
			m.status.Status = "Checking connection"
			if m.failures >= 3 {
				m.status.Status = "Connection unavailable; stop and restart when your client task is finished"
			}
		} else {
			m.failures = 0
			m.status.Ready = true
			if m.status.StartupSeconds == 0 {
				m.status.StartupSeconds = time.Since(m.started).Seconds()
			}
			m.status.Status = "Engine ready"
			if metrics["llamacpp:requests_processing"] > 0 {
				m.status.Status = "Engine working"
			}
			tokens, seconds := metrics["llamacpp:tokens_predicted_total"], metrics["llamacpp:tokens_predicted_seconds_total"]
			m.status.Generated = int(tokens)
			if seconds > 0 {
				m.status.Speed = tokens / seconds
			}
			if c, _, e := connection(m.root); e == nil {
				m.status.Backend = c.Backend
				m.status.Device = c.Device
			}
		}
		m.mu.Unlock()
	}
}
func parseMetrics(text string) map[string]float64 {
	out := map[string]float64{}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 2 {
			if n, e := strconv.ParseFloat(fields[1], 64); e == nil && n >= 0 && n < 1e15 {
				out[fields[0]] = n
			}
		}
	}
	return out
}
func (m *EngineManager) quickTest() (string, error) {
	m.mu.Lock()
	if !m.status.Ready || m.status.Testing {
		m.mu.Unlock()
		return "", errors.New("wait for an idle, ready engine")
	}
	id := m.session
	m.status.Testing = true
	ctx, cancel := context.WithTimeout(context.Background(), 11*time.Minute)
	m.cancelTest = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		if m.session == id {
			m.status.Testing = false
		}
		m.mu.Unlock()
	}()
	c, key, err := connection(m.root)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]any{"model": "reflect-go-local", "max_tokens": 64, "temperature": .2, "messages": []map[string]string{{"role": "system", "content": "You run locally through Reflect Go on this computer. Be brief."}, {"role": "user", "content": "Write one friendly sentence welcoming me to my portable AI workstation."}}})
	request, _ := http.NewRequestWithContext(ctx, "POST", c.Base+"/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	client := *localHTTP
	client.Timeout = 11 * time.Minute
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("provider returned %d", response.StatusCode)
	}
	var result struct {
		Choices []struct{ Message struct{ Content string } }
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("empty provider response")
	}
	return result.Choices[0].Message.Content, nil
}
func serveUI(root string) error {
	uiLock, err := lockNamed(root, "browser-ui.lock")
	if err != nil {
		return reopenUI(root)
	}
	defer uiLock.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	host := listener.Addr().String()
	base := "http://" + host
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	manager := newManager(root)
	downloads := newDownloads(root)
	defer downloads.close()
	defer manager.waitStopped()
	mux := http.NewServeMux()
	productRoutes(mux, root, manager, downloads)
	quit := make(chan struct{}, 1)
	if os.Getenv("REFLECT_GO_UI_OWNED") == "1" {
		go func() {
			io.Copy(io.Discard, os.Stdin)
			select {
			case quit <- struct{}{}:
			default:
			}
		}()
	}
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	decode := func(w http.ResponseWriter, r *http.Request, v any) bool {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return false
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON required", 415)
			return false
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		d.DisallowUnknownFields()
		if err := d.Decode(v); err != nil {
			http.Error(w, err.Error(), 400)
			return false
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			http.Error(w, "Trailing JSON", 400)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		clients := map[string]bool{}
		for _, name := range []string{"codex", "claude", "opencode"} {
			_, e := portableClient(root, name, false)
			clients[name] = e == nil
		}
		status := manager.snapshot()
		var gateway GatewayState
		if status.Ready {
			gateway, _ = gatewayStatus(root)
		}
		if gateway.PID > 0 {
			status.ResidentBytes = processMemory(gateway.PID)
		}
		send(w, map[string]any{"engine": status, "gateway": gateway, "clients": clients, "clientReports": productStatus(root), "download": downloads.snapshot(), "platform": platformID(), "version": version})
	})
	mux.HandleFunc("/api/inventory", func(w http.ResponseWriter, r *http.Request) {
		s, e := loadSettings(root)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		o := Options{Backend: r.URL.Query().Get("backend")}
		o.Context, e = strconv.Atoi(r.URL.Query().Get("context"))
		if e != nil || o.Context < 512 || o.Context > 32768 {
			http.Error(w, "Invalid context", 400)
			return
		}
		inv, e := inventory(root, s, o)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		send(w, inv)
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		var s RunSettings
		if !decode(w, r, &s) {
			return
		}
		if e := manager.start(s); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		send(w, map[string]bool{"started": true})
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		var payload struct{}
		if !decode(w, r, &payload) {
			return
		}
		manager.stop()
		send(w, map[string]bool{"stopping": true})
	})
	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		var payload struct{}
		if !decode(w, r, &payload) {
			return
		}
		result, e := manager.quickTest()
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		send(w, map[string]string{"response": result})
	})
	mux.HandleFunc("/api/client", func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Client  string `json:"client"`
			Project string `json:"project"`
			Task    string `json:"task"`
		}
		if !decode(w, r, &p) {
			return
		}
		if !manager.snapshot().Ready {
			http.Error(w, "Start this workstation's engine first", 409)
			return
		}
		if _, e := portableClient(root, p.Client, false); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		var projectErr error
		p.Project, projectErr = resolveProject(root, p.Project)
		if projectErr != nil {
			http.Error(w, projectErr.Error(), 400)
			return
		}
		if e := launchTerminal(root, p.Client, p.Project, p.Task); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		send(w, map[string]bool{"opened": true})
	})
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		var payload struct{}
		if !decode(w, r, &payload) {
			return
		}
		if err := manager.waitStopped(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		send(w, map[string]bool{"closed": true})
		select {
		case quit <- struct{}{}:
		default:
		}
	})
	assets, _ := fs.Sub(uiFiles, "ui")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	handler := guardedUI(host, token, mux)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	defer server.Close()
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Serve(listener) }()
	page, err := statePath(root, "OPEN-WORKSTATION.html")
	if err != nil {
		return err
	}
	sessionPath, err := statePath(root, "browser-session.json")
	if err != nil {
		return err
	}
	if err = writeJSON(sessionPath, UISession{Base: base, Token: token}); err != nil {
		return err
	}
	defer os.Remove(sessionPath)
	uri := base + "/#" + token
	file := `<!doctype html><meta charset="utf-8"><title>Open Reflect Go</title><a href="` + html.EscapeString(uri) + `">Open your local Reflect Go workstation</a>`
	if err = atomicWrite(page, []byte(file), 0600); err != nil {
		return err
	}
	defer os.Remove(page)
	fmt.Printf("Reflect Go workstation: %s\nKeep this launcher running. Use Quit workstation before ejecting.\n", base)
	if os.Getenv("REFLECT_GO_NO_BROWSER") != "1" {
		if err = openBrowser(uri); err != nil {
			fmt.Fprintf(os.Stderr, "Open %s in your browser.\n", page)
			showLaunchError(fmt.Errorf("Could not open the default browser. Open this file to reconnect: %s", page))
		}
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	select {
	case <-signals:
	case <-quit:
	case err := <-serverDone:
		return err
	}
	result := manager.waitStopped()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Allow the Quit response to flush before closing active connections.
	if e := server.Shutdown(ctx); result == nil {
		result = e
	}
	return result
}

func guardedUI(host, token string, next http.Handler) http.Handler {
	base := "http://" + host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if r.Host != host {
			http.Error(w, "Invalid host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != base {
			http.Error(w, "Invalid origin", 403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(auth), []byte(token)) != 1 {
				http.Error(w, "Open Reflect Go from its launcher to authenticate", 401)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
