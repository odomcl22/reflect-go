package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	ID                   string       `json:"id"`
	Project              string       `json:"project"`
	Settings             *RunSettings `json:"settings,omitempty"`
	ConfirmRestart       bool         `json:"confirmRestart"`
	ConfirmEngineRestart bool         `json:"confirmEngineRestart"`
}
type integrationManager struct {
	root       string
	engine     *EngineManager
	mu         sync.Mutex
	closeProxy func()
	proxies    map[string]func()
}

func newIntegrationManager(root string, engine *EngineManager) *integrationManager {
	return &integrationManager{root: root, engine: engine, proxies: map[string]func(){}}
}
func (m *integrationManager) connected() bool {
	return m.connectedID("claude-desktop")
}
func (m *integrationManager) connectedID(id string) bool {
	path, err := desktopJournalPath(m.root, id)
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
	claude := desktopApp("claude-desktop") != ""
	claudeDetail := "Uses 49,152 context with compact KV memory when the selected model fits. Disconnect or quit restores the previous connection and restarts the app; no sign-out."
	if !claude {
		claudeDetail = "Requires a supported Claude Desktop installation."
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
		{ID: "codex-desktop", Name: "Codex in ChatGPT Desktop", Kind: "Preview desktop connection", Available: desktopApp("codex-desktop") != "", Connected: m.connectedID("codex-desktop"), Detail: "Adds local models alongside your existing catalog. Local requests stay local; selecting an existing cloud model uses your normal account. Restores settings on disconnect or quit. Requires a supported installed ChatGPT app.", ActionLabel: "Connect Codex Desktop"},
		{ID: "opencode-desktop", Name: "OpenCode Desktop", Kind: "Preview desktop connection", Available: desktopApp("opencode-desktop") != "", Connected: m.connectedID("opencode-desktop"), Detail: "Adds a local provider to your installed desktop config. Your previous model and provider settings return on disconnect or quit. Project selection happens in OpenCode.", ActionLabel: "Connect OpenCode Desktop"},
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
			if isDesktopIntegration(p.ID) {
				if action == "disconnect" && !m.connectedID(p.ID) {
					http.Error(w, "There is no Reflect Go desktop connection recorded for this computer; no app or settings changed", 409)
					return
				}
				if action == "launch" && m.connectedID(p.ID) {
					http.Error(w, "Disconnect the recorded connection before changing its models", 409)
					return
				}
				running, err := desktopRunning(p.ID)
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if running && !p.ConfirmRestart {
					send(w, 409, map[string]any{"needsRestart": true, "message": "This desktop app must restart to change its connection. Finish its current work. Disconnecting or quitting Reflect Go will restore its previous settings and restart it again; sign-out is not required."})
					return
				}
			}

			var message string
			var err error
			if action == "disconnect" {
				if !isDesktopIntegration(p.ID) {
					http.Error(w, "This app does not change a host connection profile", 400)
					return
				}
				err = m.disconnect(r.Context(), p.ID, p.ConfirmRestart)
				message = "Previous connection restored. No sign-out was needed."
			} else {
				message, err = m.launch(r.Context(), p)
			}

			if err != nil {
				if errors.Is(err, errDesktopEngineRestart) {
					send(w, 409, map[string]any{"needsEngineRestart": true, "message": err.Error()})
					return
				}
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
	minimum := 16384
	if isDesktopIntegration(p.ID) {
		minimum = 49152
	}
	restartEngine := (status.Active || status.Preparing) && shared.Context < minimum
	if restartEngine && !p.ConfirmEngineRestart {
		return "", errDesktopEngineRestart
	}
	if shared.Context < minimum {
		shared.Context = minimum
	}

	prefs, err := loadLaunchPreferences(m.root)
	if err != nil {
		return "", err
	}
	settings := gatewayRunSettings(shared, prefs)
	if (status.Active || status.Preparing) && !restartEngine && !sameEngineConfiguration(status.Settings, settings) {
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
	if restartEngine {
		if err = m.engine.waitStopped(); err != nil {
			return "", err
		}
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
	case "claude-desktop", "codex-desktop", "opencode-desktop":
		return m.launchDesktop(ctx, p, c, key, models)
	}

	return "", errors.New("unknown app")
}
func isDesktopIntegration(id string) bool {
	return id == "claude-desktop" || id == "codex-desktop" || id == "opencode-desktop"
}

var errDesktopEngineRestart = errors.New("Desktop apps need a larger context than the running service. Finish all client tasks before restarting it with 49,152 tokens and compact KV memory")

func (m *integrationManager) launchDesktop(ctx context.Context, p integrationRequest, c Connection, key string, models []ClientModel) (string, error) {
	base := c.Base
	closeProxy := func() {}
	var err error
	switch p.ID {
	case "claude-desktop":
		base, closeProxy, err = startClaudeDesktopProxy(c, key, models)
	case "codex-desktop":
		base, closeProxy, err = startCodexDesktopProxy(c, key, models)
	}
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
	var journal profileJournal
	if p.ID == "claude-desktop" {
		journal, err = planClaudeProfile(home, host, base, key)
	} else {
		journal, err = planDesktopProfile(home, host, p.ID, base, key, c, models)
	}
	if err != nil {
		closeProxy()
		return "", err
	}
	path, err := desktopJournalPath(m.root, p.ID)
	if err != nil {
		closeProxy()
		return "", err
	}
	err = runDesktopChange(ctx, p.ConfirmRestart, true, desktopChangeActions{Running: func() (bool, error) { return desktopRunning(p.ID) }, Quit: func(ctx context.Context) error { return quitDesktop(ctx, p.ID) }, Apply: func() error { return applyProfileJournal(path, journal) }, Open: func(ctx context.Context) error { return openDesktop(ctx, p.ID) }, Rollback: func() error { return restoreDesktopProfile(m.root, p.ID) }})
	if err != nil {
		closeProxy()
		return "", err
	}
	m.proxies[p.ID] = closeProxy
	return "Desktop opened with your local models. Disconnect or quit Reflect Go to restore its previous connection; sign-out is not needed. Select the project inside the desktop app.", nil
}
func (m *integrationManager) disconnect(ctx context.Context, id string, confirmed bool) error {
	err := runDesktopChange(ctx, confirmed, false, desktopChangeActions{Running: func() (bool, error) { return desktopRunning(id) }, Quit: func(ctx context.Context) error { return quitDesktop(ctx, id) }, Apply: func() error { return restoreDesktopProfile(m.root, id) }, Open: func(ctx context.Context) error { return openDesktop(ctx, id) }})
	if err == nil {
		if close := m.proxies[id]; close != nil {
			close()
			delete(m.proxies, id)
		}
	}
	return err
}
func (m *integrationManager) disconnectAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range []string{"claude-desktop", "opencode-desktop", "codex-desktop"} {
		if m.connectedID(id) {
			if err := m.disconnect(ctx, id, true); err != nil {
				return fmt.Errorf("could not disconnect %s: %w", id, err)
			}
		}
	}
	return nil
}
func (m *integrationManager) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := m.disconnectAll(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Desktop connection restore needs attention:", err)
	}
	for _, close := range m.proxies {
		close()
	}
	if m.closeProxy != nil {
		m.closeProxy()
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

var errDesktopRestart = errors.New("The desktop app must restart to change its connection. Finish its current work before continuing")

type desktopChangeActions struct {
	Running  func() (bool, error)
	Quit     func(context.Context) error
	Apply    func() error
	Open     func(context.Context) error
	Rollback func() error
}

// Called only after model/engine validation and a reversible profile plan exist.
// Disconnect reopens only a desktop app that was already running; connect opens
// it deliberately. Cancelling after an applied connection rolls that edit back.
// A graceful quit can wait on the app's own confirmation (ChatGPT asks before
// closing when scheduled tasks exist), so allow time for the person to answer.
// Stay under WebKit's 60-second request limit: if the page's request expires
// first, the change is cancelled with only "Load failed" shown.
const desktopQuitWait = 45 * time.Second

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
		quitCtx, cancel := context.WithTimeout(ctx, desktopQuitWait)
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
