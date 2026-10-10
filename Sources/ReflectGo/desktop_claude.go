package main

import (
	"bytes"
	"context"
	"crypto/subtle"
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
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const claudeProfileID = "48dcf153-22eb-49df-861c-83b1b453da18"

var claudeSlots = []struct{ ID, Name, Family, Created string }{
	{"claude-fable-5", "Fable 5", "fable", "2026-06-09T00:00:00Z"},
	{"claude-opus-5", "Opus 5", "opus", "2026-07-24T00:00:00Z"},
	{"claude-sonnet-5", "Sonnet 5", "sonnet", "2026-06-30T00:00:00Z"},
	{"claude-haiku-4-5-20251001", "Haiku 4.5", "haiku", "2025-10-01T00:00:00Z"},
	{"claude-sonnet-4-6", "Sonnet 4.6", "sonnet", "2025-11-18T00:00:00Z"},
}

type claudeDesktopProxy struct {
	host, key  string
	connection Connection
	models     []ClientModel
	proxy      *httputil.ReverseProxy
}

func (p *claudeDesktopProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Host != p.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+p.host) {
		http.Error(w, "Invalid local origin", 403)
		return
	}
	key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(key), []byte(p.key)) != 1 {
		http.Error(w, "Invalid local gateway key", 401)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/v1/models" {
		data := []map[string]any{}
		for i, m := range p.models {
			s := claudeSlots[i]
			data = append(data, map[string]any{"id": s.ID, "type": "model", "display_name": "Reflect Go · " + m.Name, "created_at": s.Created, "max_tokens": p.connection.Context, "anthropic_family_tier": s.Family, "is_family_default": i < 4})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": data, "first_id": claudeSlots[0].ID, "last_id": claudeSlots[len(data)-1].ID, "has_more": false})
		return
	}
	if r.Method != "POST" || (r.URL.Path != "/v1/messages" && r.URL.Path != "/v1/messages/count_tokens") {
		http.Error(w, "Unsupported desktop gateway request", 404)
		return
	}
	var body map[string]json.RawMessage
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	if d.Decode(&body) != nil || body == nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid desktop request", 400)
		return
	}
	var requested string
	json.Unmarshal(body["model"], &requested)
	id := ""
	for i, m := range p.models {
		if requested == claudeSlots[i].ID || requested == m.ID {
			id = m.ID
			break
		}
	}
	if id == "" {
		http.Error(w, "This model is not offered by Reflect Go; reconnect Claude to refresh its models", 400)
		return
	}
	// Pass client tools to llama.cpp, but do not imply access to vendor-hosted
	// search or other server tools that this local text runtime cannot execute.
	var tools []struct {
		Type string `json:"type"`
	}
	json.Unmarshal(body["tools"], &tools)
	for _, tool := range tools {
		if tool.Type != "" && tool.Type != "custom" {
			http.Error(w, "This preview supports client tools only; the requested vendor-hosted tool is unavailable locally", 400)
			return
		}
	}
	body["model"], _ = json.Marshal(id)
	data, _ := json.Marshal(body)
	r.Body = io.NopCloser(bytes.NewReader(data))
	r.ContentLength = int64(len(data))
	r.Header.Del("Origin")
	r.Header.Del("Cookie")
	r.Header.Del("Proxy-Authorization")
	r.Header.Del("x-api-key")
	p.proxy.ServeHTTP(w, r)
}
func startClaudeDesktopProxy(c Connection, key string, models []ClientModel) (string, func(), error) {
	if len(models) == 0 || len(models) > len(claudeSlots) {
		return "", nil, errors.New("Claude Desktop accepts one to five offered models in this preview")
	}
	u, err := url.Parse(c.Base)
	if err != nil {
		return "", nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	p := &claudeDesktopProxy{host: listener.Addr().String(), key: key, connection: c, models: models}
	p.proxy = httputil.NewSingleHostReverseProxy(u)
	director := p.proxy.Director
	p.proxy.Director = func(r *http.Request) { director(r); r.Host = u.Host; r.Header.Set("Authorization", "Bearer "+key) }
	p.proxy.Transport = localHTTP.Transport
	p.proxy.FlushInterval = -1
	p.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Reflect Go's engine is unavailable. Reopen Reflect Go and reconnect Claude.", 503)
	}
	server := &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go server.Serve(listener)
	return "http://" + p.host, func() { server.Close() }, nil
}
func claudeProfilePaths(home string) (normal, third, meta, profile string) {
	base := filepath.Join(home, "Library", "Application Support")
	normal = filepath.Join(base, "Claude", "claude_desktop_config.json")
	third = filepath.Join(base, "Claude-3p", "claude_desktop_config.json")
	meta = filepath.Join(base, "Claude-3p", "configLibrary", "_meta.json")
	profile = filepath.Join(filepath.Dir(meta), claudeProfileID+".json")
	return
}
func planClaudeProfile(home, host, base, key string) (profileJournal, error) {
	j := profileJournal{Host: host, Home: home}
	normal, third, meta, profile := claudeProfilePaths(home)
	if _, err := os.Stat(profile); err == nil {
		v, e := profileJSON(profile)
		if e != nil || len(v) != 0 {
			return j, errors.New("Reflect Go's desktop profile already exists; restore the recorded connection first")
		}
	} else if !os.IsNotExist(err) {
		return j, err
	}
	metadata, err := profileJSON(meta)
	if err != nil {
		return j, err
	}
	var entries []map[string]any
	if raw := metadata["entries"]; len(raw) > 0 && json.Unmarshal(raw, &entries) != nil {
		return j, errors.New("unrecognized Claude profile list; no settings changed")
	}
	for _, entry := range entries {
		if entry["id"] == claudeProfileID {
			return j, errors.New("a Reflect Go profile entry already exists; disconnect it first")
		}
	}
	entries = append(entries, map[string]any{"id": claudeProfileID, "name": "Reflect Go"})
	changes := []struct {
		path, key string
		value     any
	}{
		{profile, "inferenceProvider", "gateway"}, {profile, "inferenceGatewayBaseUrl", base}, {profile, "inferenceGatewayApiKey", key}, {profile, "inferenceGatewayAuthScheme", "bearer"},
		{profile, "deploymentDisplayName", "Reflect Go"}, {profile, "chatTabEnabled", true}, {profile, "disableEssentialTelemetry", true}, {profile, "disableNonessentialTelemetry", true},
		{meta, "entries", entries}, {meta, "appliedId", claudeProfileID}, {third, "deploymentMode", "3p"}, {normal, "deploymentMode", "3p"},
	}
	for _, change := range changes {
		e, err := newProfileEdit(change.path, change.key, change.value)
		if err != nil {
			return j, err
		}
		j.Edits = append(j.Edits, e)
	}
	return j, nil
}
func claudeJournalPath(root string) (string, error) {
	return statePath(root, "claude-desktop-restore.json")
}
func restoreClaudeProfile(root string) error {
	path, err := claudeJournalPath(root)
	if err != nil {
		return err
	}
	var j profileJournal
	if err = readJSON(path, &j); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	normal, third, meta, profile := claudeProfilePaths(home)
	allowed := map[string]map[string]bool{normal: {"deploymentMode": true}, third: {"deploymentMode": true}, meta: {"entries": true, "appliedId": true}, profile: {"inferenceProvider": true, "inferenceGatewayBaseUrl": true, "inferenceGatewayApiKey": true, "inferenceGatewayAuthScheme": true, "deploymentDisplayName": true, "chatTabEnabled": true, "disableEssentialTelemetry": true, "disableNonessentialTelemetry": true}}
	for _, edit := range j.Edits {
		if !allowed[edit.Path][edit.Key] {
			return errors.New("desktop restore record contains an unexpected setting; no files changed")
		}
	}
	return restoreProfileJournal(path, host, home)
}
func claudeDesktopApp() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{"/Applications/Claude.app", filepath.Join(home, "Applications", "Claude.app")} {
		if s, e := os.Stat(p); e == nil && s.IsDir() {
			return p
		}
	}
	return ""
}
func claudeDesktopRunning() (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := exec.CommandContext(ctx, "/usr/bin/pgrep", "-f", "Claude.app/Contents/MacOS/Claude").Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, errors.New("could not check whether Claude is running; close Claude before connecting")
}
func quitClaudeDesktop(ctx context.Context) error {
	// Ask the application to quit normally. Never force-kill work in progress.
	if err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `tell application id "com.anthropic.claudefordesktop" to quit`).Run(); err != nil {
		return fmt.Errorf("Claude did not quit; close it and retry: %w", err)
	}
	for {
		running, err := claudeDesktopRunning()
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("Claude is still open; finish its active work and close it before reconnecting")
		case <-time.After(200 * time.Millisecond):
		}
	}
}
