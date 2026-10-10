package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DeepSeek Harness is a separate browser-based coding harness. This adapter
// uses its documented settings-provider patch; it never edits ~/.dsh.
const deepSeekKeyEnv = "REFLECT_GO_DSH_API_KEY"

var deepSeekSessions = struct {
	sync.Mutex
	items map[string]*deepSeekSession
}{items: map[string]*deepSeekSession{}}

type deepSeekSession struct {
	root string
	cmd  *exec.Cmd
	key  string
	url  string
	done chan struct{}
	err  error
}

func findDeepSeek() (string, error) {
	if path, err := exec.LookPath("dsh"); err == nil {
		return path, nil
	}
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin", "dsh"))
	}
	if appData := os.Getenv("APPDATA"); runtime.GOOS == "windows" && filepath.IsAbs(appData) {
		candidates = append(candidates, filepath.Join(appData, "npm", "dsh.cmd"))
	}
	for _, path := range candidates {
		if f, err := os.Stat(path); err == nil && f.Mode().IsRegular() {
			return path, nil
		}
	}
	return "", errors.New("DeepSeek Harness is not installed on this computer; install dsh from its official project, then reopen Reflect Go. A portable DeepSeek pack is not included yet")
}

func launchDeepSeek(root, project string, c Connection, key string, models []ClientModel) (string, error) {
	return launchDeepSeekContext(context.Background(), root, project, c, key, models)
}

func launchDeepSeekContext(ctx context.Context, root, project string, c Connection, key string, models []ClientModel) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	client, err := findDeepSeek()
	if err != nil {
		return "", err
	}
	dir, err := resolveProject(root, project)
	if err != nil {
		return "", err
	}
	deepSeekSessions.Lock()
	defer deepSeekSessions.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sessionKey := root + "\x00" + dir
	if current := deepSeekSessions.items[sessionKey]; current != nil {
		select {
		case <-current.done:
			delete(deepSeekSessions.items, sessionKey)
		default:
			if current.key != modelID(key) {
				return "", errors.New("DeepSeek Harness is open with an earlier local connection key. Close Reflect Go and reopen it before starting a new session")
			}
			// Regenerate only our launch settings. DSH observes this provider
			// document and updates the model list without replacing host profiles.
			if _, err := deepSeekCommand(root, dir, c, key, models, client); err != nil {
				return "", err
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if err := openBrowser(current.url); err != nil {
				return "", err
			}
			return "DeepSeek Harness is open with your Reflect Go models.", nil
		}
	}
	cmd, err := deepSeekCommand(root, dir, c, key, models, client)
	if err != nil {
		return "", err
	}
	logPath, err := statePath(root, "deepseek-launch-"+modelID(dir)+".log")
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
		return "", fmt.Errorf("start DeepSeek Harness: %w", err)
	}
	session := &deepSeekSession{root: root, cmd: cmd, key: modelID(key), done: make(chan struct{})}
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
			return "", fmt.Errorf("DeepSeek Harness exited before opening: %v; see the DeepSeek launch log in state", session.err)
		case <-deadline.C:
			stopAndWaitAdditionalHarness(cmd, session.done)
			return "", errors.New("DeepSeek Harness did not become ready within 25 seconds; see the DeepSeek launch log in state")
		case <-tick.C:
			if err := ctx.Err(); err != nil {
				stopAndWaitAdditionalHarness(cmd, session.done)
				return "", err
			}
			data, _ := os.ReadFile(logPath)
			if address := deepSeekReadyURL(string(data)); address != "" {
				if err := ctx.Err(); err != nil {
					stopAndWaitAdditionalHarness(cmd, session.done)
					return "", err
				}
				session.url = address
				deepSeekSessions.items[sessionKey] = session
				if err := openBrowser(address); err != nil {
					return "", fmt.Errorf("DeepSeek Harness is running, but its browser could not open: %w", err)
				}
				return "DeepSeek Harness opened. Choose a Reflect Go model in its model picker; the engine loads it when you send a message.", nil
			}
		}
	}
}

func closeDeepSeekHarnesses(root string) {
	deepSeekSessions.Lock()
	defer deepSeekSessions.Unlock()
	for key, session := range deepSeekSessions.items {
		if session.root != root {
			continue
		}
		select {
		case <-session.done:
		default:
			stopAndWaitAdditionalHarness(session.cmd, session.done)
		}
		delete(deepSeekSessions.items, key)
	}
}

func deepSeekCommand(root, dir string, c Connection, key string, models []ClientModel, client string) (*exec.Cmd, error) {
	if len(models) == 0 {
		return nil, errors.New("enable at least one model for DeepSeek Harness")
	}
	if c.Context < 16384 {
		return nil, errors.New("DeepSeek Harness needs a model eligible at 16,384 context tokens for this preview")
	}
	baseURL, err := url.Parse(c.Base)
	if err != nil || baseURL.Scheme != "http" || baseURL.Hostname() != "127.0.0.1" || baseURL.Port() == "" || baseURL.User != nil || baseURL.Path != "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("DeepSeek Harness requires a local Reflect Go connection")
	}
	env, err := clientEnvironment(root, "deepseek")
	if err != nil {
		return nil, err
	}
	base, err := statePath(root, "harnesses/deepseek/"+platformID())
	if err != nil {
		return nil, err
	}
	// A project gets its own DSH_HOME so an already-open harness keeps its
	// project and sessions when another project opens from Reflect Go.
	profile := filepath.Join(base, "projects", modelID(dir))
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, err
	}
	settingsPath := filepath.Join(profile, "reflect-go-settings.yaml")
	patchPath := filepath.Join(profile, "reflect-go.cordis.yml")
	primary := c.ID
	found := false
	entries := make([]map[string]any, 0, len(models))
	for _, m := range models {
		if m.ID == primary {
			found = true
		}
		entries = append(entries, map[string]any{"id": m.ID, "name": m.Name, "input": []string{"text"}, "contextWindow": c.Context, "maxTokens": min(2048, c.Context/4), "reasoningEfforts": false})
	}
	if !found {
		primary = models[0].ID
	}
	settings := map[string]any{
		"agent-default-model": map[string]string{"provider": "reflect-go", "model": primary},
		"llm-pi-ai": map[string]any{"providers": map[string]any{"reflect-go": map[string]any{
			"displayName": "Reflect Go", "apiKeyEnv": deepSeekKeyEnv, "api": "openai-completions", "baseURL": c.Base + "/v1", "models": entries,
			"compat": map[string]any{"supportsDeveloperRole": false, "maxTokensField": "max_tokens"},
		}}},
	}
	// JSON is valid YAML. Using the standard encoder keeps paths/model names
	// as data and avoids adding a YAML expression or shell-injection surface.
	if err := writeJSON(settingsPath, settings); err != nil {
		return nil, err
	}
	patch := []map[string]any{
		{"id": "settings", "config": map[string]string{"path": settingsPath}},
		{"id": "web-search-deepseek", "disabled": true},
		{"id": "session-telemetry-otel", "disabled": true},
	}
	if err := writeJSON(patchPath, patch); err != nil {
		return nil, err
	}
	env = append(env, "DSH_HOME="+profile, deepSeekKeyEnv+"="+key, "DSH_PERMISSION_MODE=workspace-write", "DSH_TELEMETRY_MODE=off")
	args := []string{"web", "--patch", patchPath, "--host", "127.0.0.1", "--port", "0", "--no-open"}
	cmd, err := deepSeekExecutableCommand(client, args)
	if err != nil {
		return nil, err
	}
	cmd.Env, cmd.Dir = env, dir
	return cmd, nil
}

func deepSeekExecutableCommand(client string, args []string) (*exec.Cmd, error) {
	entry := client
	if runtime.GOOS == "windows" && (strings.EqualFold(filepath.Ext(client), ".cmd") || strings.EqualFold(filepath.Ext(client), ".bat")) {
		entry = filepath.Join(filepath.Dir(client), "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
		if _, err := os.Stat(entry); err != nil {
			return nil, errors.New("DeepSeek's Windows Node entrypoint is missing; reinstall its official package")
		}
	}
	if resolved, err := filepath.EvalSymlinks(entry); err == nil {
		entry = resolved
	}
	if strings.EqualFold(filepath.Ext(entry), ".js") || strings.EqualFold(filepath.Ext(entry), ".mjs") {
		node, err := exec.LookPath("node")
		if err != nil {
			return nil, errors.New("DeepSeek Harness requires Node.js on this computer")
		}
		return exec.Command(node, append([]string{entry}, args...)...), nil
	}
	return exec.Command(client, args...), nil
}

var deepSeekURLPattern = regexp.MustCompile(`(?m)^dsh web: (http://127\.0\.0\.1:[0-9]+)(?:\s|$)`)

func deepSeekReadyURL(log string) string {
	match := deepSeekURLPattern.FindStringSubmatch(log)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
