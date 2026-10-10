package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type openCodeWebSession struct {
	root, url, signature string
	cmd                  *exec.Cmd
	done                 chan struct{}
	server               *http.Server
	err                  error
}

var openCodeWebSessions = struct {
	sync.Mutex
	items map[string]*openCodeWebSession
}{items: map[string]*openCodeWebSession{}}

func launchOpenCodeWeb(root, project string, c Connection, key string, models []ClientModel) (string, error) {
	return launchOpenCodeWebContext(context.Background(), root, project, c, key, models)
}

func launchOpenCodeWebContext(ctx context.Context, root, project string, c Connection, key string, models []ClientModel) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir, err := resolveProject(root, project)
	if err != nil {
		return "", err
	}
	openCodeWebSessions.Lock()
	defer openCodeWebSessions.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	lookup := root + "\x00" + dir
	signature := fmt.Sprint(c, models, modelID(key))
	if current := openCodeWebSessions.items[lookup]; current != nil {
		select {
		case <-current.done:
			_ = current.server.Close()
			delete(openCodeWebSessions.items, lookup)
		default:
			if current.signature != signature {
				return "", errors.New("OpenCode Web is still open with its earlier model settings. Close Reflect Go and reopen it to apply the new selection without interrupting a running task")
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "OpenCode Web is ready in your browser.", openBrowser(current.url)
		}
	}
	// Reuse the real CLI adapter, including its isolated settings, model
	// catalog, local key and approval policy. `serve` exposes the same web UI
	// as `web`, without opening an unauthenticated browser page prematurely.
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	profile := &ClientLaunchPreference{Enabled: ids, DefaultModel: c.ID}
	cmd, err := clientCommandWithProfile(root, "opencode", "portable", dir, "", profile)
	if err != nil {
		return "", err
	}
	password, err := additionalHarnessToken()
	if err != nil {
		return "", err
	}
	cmd.Args = []string{cmd.Path, "serve", "--hostname", "127.0.0.1", "--port", "0"}
	cmd.Env = append(cmd.Env, "OPENCODE_SERVER_USERNAME=reflect-go", "OPENCODE_SERVER_PASSWORD="+password)
	cmd.Stdin = nil
	logPath, err := statePath(root, "opencode-web-"+modelID(dir)+".log")
	if err != nil {
		return "", err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	cmd.Stdout, cmd.Stderr = log, log
	prepareAdditionalHarness(cmd)
	if err := ctx.Err(); err != nil {
		log.Close()
		return "", err
	}
	if err := cmd.Start(); err != nil {
		log.Close()
		return "", err
	}
	session := &openCodeWebSession{root: root, cmd: cmd, signature: signature, done: make(chan struct{})}
	go func() {
		session.err = cmd.Wait()
		log.Close()
		close(session.done)
	}()
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stopAndWaitAdditionalHarness(cmd, session.done)
			return "", ctx.Err()
		case <-session.done:
			return "", fmt.Errorf("OpenCode Web exited before becoming ready: %v; see its launch log in state", session.err)
		case <-deadline.C:
			stopAndWaitAdditionalHarness(cmd, session.done)
			return "", errors.New("OpenCode Web did not become ready within 25 seconds; see its launch log in state")
		case <-tick.C:
			if err := ctx.Err(); err != nil {
				stopAndWaitAdditionalHarness(cmd, session.done)
				return "", err
			}
			data, _ := os.ReadFile(logPath)
			address := openCodeWebReadyURL(string(data))
			if address == "" {
				continue
			}
			server, connectURL, err := startOpenCodeWebBridge(address, password, dir)
			if err != nil {
				stopAndWaitAdditionalHarness(cmd, session.done)
				return "", err
			}
			if err := ctx.Err(); err != nil {
				_ = server.Close()
				stopAndWaitAdditionalHarness(cmd, session.done)
				return "", err
			}
			session.server, session.url = server, connectURL
			openCodeWebSessions.items[lookup] = session
			if err := openBrowser(connectURL); err != nil {
				return "", fmt.Errorf("OpenCode Web is ready, but its browser could not open: %w", err)
			}
			return "OpenCode Web opened with your Reflect Go models. This is the graphical browser client; the native OpenCode Desktop adapter is still being qualified.", nil
		}
	}
}

func closeAdditionalHarnesses(root string) {
	closeDeepSeekHarnesses(root)
	openCodeWebSessions.Lock()
	defer openCodeWebSessions.Unlock()
	for key, session := range openCodeWebSessions.items {
		if session.root != root {
			continue
		}
		_ = session.server.Close()
		select {
		case <-session.done:
		default:
			stopAndWaitAdditionalHarness(session.cmd, session.done)
		}
		delete(openCodeWebSessions.items, key)
	}
}

func stopAndWaitAdditionalHarness(cmd *exec.Cmd, done <-chan struct{}) {
	select {
	case <-done:
		return
	default:
	}
	stopAdditionalHarness(cmd)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		forceStopAdditionalHarness(cmd)
		select {
		case <-done:
		case <-time.After(time.Second):
		}
	}
}

func additionalHarnessToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

var openCodeWebURLPattern = regexp.MustCompile(`(?m)^opencode server listening on (http://127\.0\.0\.1:[0-9]+)(?:\s|$)`)

func openCodeWebReadyURL(log string) string {
	match := openCodeWebURLPattern.FindStringSubmatch(log)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func startOpenCodeWebBridge(address, password, project string) (*http.Server, string, error) {
	upstream, err := url.Parse(address)
	if err != nil || upstream.Scheme != "http" || upstream.Hostname() != "127.0.0.1" || upstream.Port() == "" {
		return nil, "", errors.New("invalid OpenCode local server address")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	token, err := additionalHarnessToken()
	if err != nil {
		listener.Close()
		return nil, "", err
	}
	address = "http://" + listener.Addr().String()
	handler := newOpenCodeWebBridge(upstream, password, token, address, project)
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return server, address + "/__reflect_connect#" + token, nil
}

// An HttpOnly, same-site browser session fronts the authenticated OpenCode
// server. Its Basic credential and the inference credential never enter a URL.
func newOpenCodeWebBridge(upstream *url.URL, password, token, address, project string) http.Handler {
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.Transport = &http.Transport{Proxy: nil}
	proxy.FlushInterval = -1
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.SetBasicAuth("reflect-go", password)
		r.Header.Del("Cookie")
		r.Header.Set("Origin", upstream.String())
		if r.Header.Get("x-opencode-directory") == "" {
			r.Header.Set("x-opencode-directory", project)
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "OpenCode Web stopped. Reopen it from Reflect Go.", http.StatusBadGateway)
	}
	host := strings.TrimPrefix(address, "http://")
	cookieName := "reflect_opencode_" + strings.TrimPrefix(modelID(host), "reflect-")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != address) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "local origin required", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/__reflect_connect" && r.Method == "GET" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
			_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>Opening OpenCode</title><p id="status">Opening OpenCode…</p><script>const token=location.hash.slice(1);history.replaceState(null,'','/__reflect_connect');fetch('/__reflect_session',{method:'POST',body:token}).then(r=>{if(!r.ok)throw Error();location.replace('/')}).catch(()=>document.getElementById('status').textContent='Reopen OpenCode Web from Reflect Go.');</script>`)
			return
		}
		if r.URL.Path == "/__reflect_session" && r.Method == "POST" {
			body, err := io.ReadAll(io.LimitReader(r.Body, 128))
			if err != nil || subtle.ConstantTimeCompare(body, []byte(token)) != 1 {
				http.Error(w, "reopen from Reflect Go", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			w.WriteHeader(http.StatusNoContent)
			return
		}
		cookie, err := r.Cookie(cookieName)
		if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(token)) != 1 {
			http.Error(w, "Open OpenCode Web from Reflect Go to connect.", http.StatusUnauthorized)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
