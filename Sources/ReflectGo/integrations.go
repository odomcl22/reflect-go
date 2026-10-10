package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type integrationInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Available   bool   `json:"available"`
	Detail      string `json:"detail"`
	Connected   bool   `json:"connected"`
	ActionLabel string `json:"actionLabel"`
}
type integrationRequest struct {
	ID             string       `json:"id"`
	Project        string       `json:"project"`
	Settings       *RunSettings `json:"settings,omitempty"`
	ConfirmRestart bool         `json:"confirmRestart"`
}
type integrationManager struct {
	root       string
	engine     *EngineManager
	mu         sync.Mutex
	closeProxy func()
}

func newIntegrationManager(root string, engine *EngineManager) *integrationManager {
	return &integrationManager{root: root, engine: engine}
}
func (m *integrationManager) connected() bool {
	path, err := claudeJournalPath(m.root)
	if err != nil {
		return false
	}
	var j profileJournal
	if readJSON(path, &j) != nil {
		return false
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	return j.Home == home && j.Host == host
}
func (m *integrationManager) inventory() []integrationInfo {
	claude := claudeDesktopApp() != ""
	claudeDetail := "Connect up to five local models. Experimental: desktop conversations and tools still need testing. Previous connection settings are saved for restoration."
	if !claude {
		claudeDetail = "Requires Claude Desktop installed on a Mac."
	}
	_, deepSeekError := findDeepSeek()
	deepSeekDetail := "Open the installed harness in your browser with shared model choices and settings stored on the drive."
	if deepSeekError != nil {
		deepSeekDetail = deepSeekError.Error()
	}
	_, webError := portableClient(m.root, "opencode", false)
	return []integrationInfo{
		{ID: "claude-desktop", Name: "Claude Desktop", Kind: "Experimental desktop connection", Available: claude, Connected: m.connected(), Detail: claudeDetail, ActionLabel: "Connect Claude Desktop"},
		{ID: "deepseek", Name: "DeepSeek Harness", Kind: "Installed harness · browser interface", Available: deepSeekError == nil, Detail: deepSeekDetail, ActionLabel: "Open DeepSeek Harness"},
		{ID: "opencode-web", Name: "OpenCode Web", Kind: "Portable graphical harness", Available: webError == nil, Detail: "Work in OpenCode’s graphical browser interface with your shared local models. Included with the portable OpenCode pack.", ActionLabel: "Open OpenCode Web"},
		{ID: "codex-desktop", Name: "Codex in ChatGPT Desktop", Kind: "Integration target", Detail: "Desktop connection is not available yet. Use Codex CLI with your local models.", ActionLabel: "Not available yet"},
		{ID: "opencode-desktop", Name: "OpenCode Desktop", Kind: "Integration target", Detail: "Desktop connection is not available yet. OpenCode CLI and Web are available.", ActionLabel: "Not available yet"},
		{ID: "antigravity", Name: "Antigravity", Kind: "Integration target", Detail: "Local-model compatibility is being evaluated. No connection is available yet.", ActionLabel: "Not available yet"},
	}
}
func (m *integrationManager) routes(mux *http.ServeMux) {
	send := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/integrations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		send(w, 200, map[string]any{"integrations": m.inventory()})
	})
	for _, action := range []string{"launch", "disconnect"} {
		action := action
		mux.HandleFunc("/api/integrations/"+action, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "POST JSON required", 405)
				return
			}
			var p integrationRequest
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
			d.DisallowUnknownFields()
			if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF {
				http.Error(w, "Invalid integration request", 400)
				return
			}
			if !m.mu.TryLock() {
				http.Error(w, "Another app connection is in progress", 409)
				return
			}
			defer m.mu.Unlock()
			if err := r.Context().Err(); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if p.ID == "claude-desktop" {
				if runtime.GOOS != "darwin" {
					http.Error(w, "Claude Desktop connection is currently macOS only", 400)
					return
				}
				if action == "disconnect" && !m.connected() {
					http.Error(w, "There is no Reflect Go desktop connection recorded for this computer; no app or settings changed", 409)
					return
				}
				running, err := claudeDesktopRunning()
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if running && !p.ConfirmRestart {
					send(w, 409, map[string]any{"needsRestart": true, "message": "Claude Desktop must restart to change its connection. Finish its current work before continuing. Reflect Go will preserve unrelated settings and restore its connection fields when disconnected."})
					return
				}
				if action == "launch" && m.connected() {
					http.Error(w, "Claude already has a recorded Reflect Go connection. Restore previous settings before changing its offered models.", 409)
					return
				}
			}
			var message string
			var err error
			if action == "disconnect" {
				if p.ID != "claude-desktop" {
					http.Error(w, "This app does not change a host connection profile", 400)
					return
				}
				err = runDesktopChange(r.Context(), p.ConfirmRestart, false, desktopChangeActions{
					Running: claudeDesktopRunning,
					Quit:    quitClaudeDesktop,
					Apply: func() error {
						if err := restoreClaudeProfile(m.root); err != nil {
							return err
						}
						if m.closeProxy != nil {
							m.closeProxy()
							m.closeProxy = nil
						}
						return nil
					},
					Open: openClaudeDesktop,
				})
				message = "Claude's previous connection settings were restored."
			} else {
				message, err = m.launch(r.Context(), p)
			}
			if err != nil {
				if errors.Is(err, errDesktopRestart) {
					send(w, 409, map[string]any{"needsRestart": true, "message": err.Error()})
					return
				}
				http.Error(w, err.Error(), 400)
				return
			}
			send(w, 200, map[string]string{"message": message})
		})
	}
}
func (m *integrationManager) launch(ctx context.Context, p integrationRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	available := false
	for _, i := range m.inventory() {
		if i.ID == p.ID {
			available = i.Available
			break
		}
	}
	if !available {
		return "", errors.New("this integration is unavailable; review its app card")
	}
	project, err := resolveProject(m.root, p.Project)
	if err != nil {
		return "", err
	}
	status := m.engine.snapshot()
	shared := configuredRunSettings(status)
	if p.Settings != nil {
		shared = *p.Settings
	}
	if !status.Active && !status.Preparing && shared.Context < 16384 {
		shared.Context = 16384
	}
	if shared.Context < 16384 {
		return "", errors.New("this harness needs 16,384 context. Finish active work and stop the engine before opening it")
	}
	prefs, err := loadLaunchPreferences(m.root)
	if err != nil {
		return "", err
	}
	settings := gatewayRunSettings(shared, prefs)
	if (status.Active || status.Preparing) && !sameEngineConfiguration(status.Settings, settings) {
		return "", errors.New("the engine is using different settings; finish active work and stop it before changing its offered models")
	}
	cfg, err := loadSettings(m.root)
	if err != nil {
		return "", err
	}
	o, err := settings.options()
	if err != nil {
		return "", err
	}
	if err = validateModelIDs(settings.Enabled); err != nil {
		return "", err
	}
	inv, err := inventory(m.root, cfg, o)
	if err != nil {
		return "", err
	}
	if !inv.Hardware.Backend.Available {
		return "", errors.New(inv.Hardware.Backend.Reason)
	}
	availableModels := []ClientModel{}
	for _, model := range inv.Models {
		if model.Eligible && (settings.Enabled == nil || containsID(settings.Enabled, model.ID)) {
			availableModels = append(availableModels, ClientModel{ID: model.ID, Name: model.Name})
		}
	}
	offered, _, err := integrationModels(availableModels, "", shared)
	if err != nil {
		return "", err
	}
	if p.ID == "claude-desktop" && len(offered) > 5 {
		return "", errors.New("offer at most five models before connecting Claude Desktop")
	}
	if err = ensureClientEngine(ctx, m.engine, settings); err != nil {
		return "", err
	}
	c, key, err := connection(m.root)
	if err != nil {
		return "", err
	}
	response, err := localRequest(m.root, "GET", "/v1/models", nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("local model catalog is unavailable (HTTP %d); review Diagnostics", response.StatusCode)
	}
	models, err := clientModels(response.Body, c)
	if err != nil {
		return "", err
	}
	models, c.ID, err = integrationModels(models, c.ID, shared)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch p.ID {
	case "deepseek":
		return launchDeepSeekContext(ctx, m.root, project, c, key, models)
	case "opencode-web":
		return launchOpenCodeWebContext(ctx, m.root, project, c, key, models)
	case "claude-desktop":
		base, closeProxy, err := startClaudeDesktopProxy(c, key, models)
		if err != nil {
			return "", err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			closeProxy()
			return "", err
		}
		host, err := os.Hostname()
		if err != nil {
			closeProxy()
			return "", err
		}
		journal, err := planClaudeProfile(home, host, base, key)
		if err != nil {
			closeProxy()
			return "", err
		}
		path, err := claudeJournalPath(m.root)
		if err == nil {
			if _, e := os.Stat(path); e == nil {
				err = errors.New("a desktop connection is already recorded; restore it before reconnecting")
			} else if !os.IsNotExist(e) {
				err = e
			}
		}
		if err != nil {
			closeProxy()
			return "", err
		}
		// Validation and the reversible edit plan are complete before asking the
		// desktop app to quit. Recheck running state in case it changed meanwhile.
		err = runDesktopChange(ctx, p.ConfirmRestart, true, desktopChangeActions{
			Running:  claudeDesktopRunning,
			Quit:     quitClaudeDesktop,
			Apply:    func() error { return applyProfileJournal(path, journal) },
			Open:     openClaudeDesktop,
			Rollback: func() error { return restoreClaudeProfile(m.root) },
		})
		if err != nil {
			closeProxy()
			return "", err
		}
		m.closeProxy = closeProxy
		return "Claude Desktop opened with Reflect Go's local model slots. This experimental adapter has protocol/configuration tests; desktop task workflows remain unqualified. Project selection happens inside Claude Desktop.", nil
	}
	return "", errors.New("unknown app")
}
func (m *integrationManager) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeProxy != nil {
		m.closeProxy()
		m.closeProxy = nil
	}
	if m.connected() {
		if err := restoreClaudeProfile(m.root); err != nil {
			fmt.Fprintln(os.Stderr, "Claude connection restore needs attention:", err)
		}
	}
	closeAdditionalHarnesses(m.root)
}

// Extra models enabled for another CLI belong in the shared gateway but not in
// this app's catalog. Use the shared default when it is offered, then fall back
// deterministically to an offered model if that default is absent on this host.
func integrationModels(models []ClientModel, defaultID string, shared RunSettings) ([]ClientModel, string, error) {
	if shared.Model != "" {
		defaultID = modelID(shared.Model)
	}
	selected, id, err := selectedClientModels(models, defaultID, ClientLaunchPreference{Enabled: shared.Enabled})
	if err != nil {
		return nil, "", err
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID == id && selected[j].ID != id })
	return selected, id, nil
}

var errDesktopRestart = errors.New("Claude Desktop must restart to change its connection. Finish its current work before continuing")

type desktopChangeActions struct {
	Running  func() (bool, error)
	Quit     func(context.Context) error
	Apply    func() error
	Open     func(context.Context) error
	Rollback func() error
}

func openClaudeDesktop(ctx context.Context) error {
	app := claudeDesktopApp()
	if app == "" {
		return errors.New("Claude Desktop is no longer available; reopen it manually after checking the connection settings")
	}
	return exec.CommandContext(ctx, "/usr/bin/open", "-a", app).Run()
}

// Called only after model/engine validation and a reversible profile plan exist.
// Disconnect reopens only a desktop app that was already running; connect opens
// it deliberately. Cancelling after an applied connection rolls that edit back.
func runDesktopChange(ctx context.Context, confirmRestart, openWhenClosed bool, actions desktopChangeActions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	running, err := actions.Running()
	if err != nil {
		return err
	}
	if running && !confirmRestart {
		return errDesktopRestart
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if running {
		quitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = actions.Quit(quitCtx)
		cancel()
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = actions.Apply(); err != nil {
		return err
	}
	rollback := func(err error) error {
		if actions.Rollback != nil {
			return errors.Join(err, actions.Rollback())
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if openWhenClosed || running {
		if err = actions.Open(ctx); err != nil {
			return rollback(err)
		}
	}
	return nil
}
