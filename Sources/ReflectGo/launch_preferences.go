package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// A nil model list inherits the workstation selection. An empty list deliberately
// offers no models, so it must survive JSON round trips without omitempty.
type ClientLaunchPreference struct {
	Enabled      []string `json:"enabled"`
	DefaultModel string   `json:"defaultModel"`
}
type LaunchPreferences struct {
	Project string                            `json:"project"`
	Clients map[string]ClientLaunchPreference `json:"clients"`
}
type ClientLaunchRequest struct {
	Client   string       `json:"client"`
	Project  *string      `json:"project,omitempty"`
	Task     string       `json:"task"`
	Settings *RunSettings `json:"settings,omitempty"`
}

var stableModelID = regexp.MustCompile(`^reflect-[a-f0-9]{16}$`)
var launchProfileName = regexp.MustCompile(`^launch-profile-[a-f0-9]{32}\.json$`)
var errLaunchConflict = errors.New("launch conflict")

func defaultLaunchPreferences() LaunchPreferences {
	return LaunchPreferences{Clients: map[string]ClientLaunchPreference{"codex": {}, "claude": {}, "opencode": {}}}
}
func validateModelIDs(ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if !stableModelID.MatchString(id) {
			return errors.New("invalid model selection; refresh the model library")
		}
		if seen[id] {
			return errors.New("a model was selected more than once")
		}
		seen[id] = true
	}
	return nil
}
func validateLaunchPreferences(p LaunchPreferences) error {
	if len(p.Project) > 4096 || strings.ContainsRune(p.Project, '\x00') {
		return errors.New("invalid project folder")
	}
	for name, c := range p.Clients {
		if !validClient(name) {
			return errors.New("choose Codex, Claude Code or OpenCode")
		}
		if err := validateModelIDs(c.Enabled); err != nil {
			return err
		}
		if c.DefaultModel != "" && !stableModelID.MatchString(c.DefaultModel) {
			return errors.New("invalid default model; refresh the model library")
		}
		if c.DefaultModel != "" && c.Enabled != nil && !containsID(c.Enabled, c.DefaultModel) {
			return fmt.Errorf("%s default model must be included in its offered models", name)
		}
	}
	return nil
}
func loadLaunchPreferences(root string) (LaunchPreferences, error) {
	p := defaultLaunchPreferences()
	path, err := statePath(root, "launch-preferences.json")
	if err != nil {
		return p, err
	}
	if err = readJSON(path, &p); err != nil && !os.IsNotExist(err) {
		return p, errors.New("saved launch preferences could not be read; review and save Apps settings again")
	}
	if err = validateLaunchPreferences(p); err != nil {
		return p, err
	}
	if p.Clients == nil {
		p.Clients = defaultLaunchPreferences().Clients
	}
	for _, name := range []string{"codex", "claude", "opencode"} {
		if _, ok := p.Clients[name]; !ok {
			p.Clients[name] = ClientLaunchPreference{}
		}
	}
	return p, nil
}
func containsID(ids []string, id string) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}
func savedClientPreference(root, name string) (ClientLaunchPreference, error) {
	prefs, err := loadLaunchPreferences(root)
	if err != nil {
		return ClientLaunchPreference{}, err
	}
	p := prefs.Clients[name]
	if p.Enabled == nil {
		path, err := statePath(root, "workstation-web.json")
		if err != nil {
			return p, err
		}
		var shared RunSettings
		if err := readJSON(path, &shared); err == nil {
			if shared.Enabled != nil {
				p.Enabled = append([]string{}, shared.Enabled...)
			}
		} else if !os.IsNotExist(err) {
			return p, errors.New("saved model selection could not be read; save the model settings again in Reflect Go")
		}
	}
	return p, nil
}

// Filter the advertised catalog, and select a default that is actually in it.
// Missing explicitly selected models are reported instead of widening access.
func selectedClientModels(models []ClientModel, defaultID string, p ClientLaunchPreference) ([]ClientModel, string, error) {
	selected := []ClientModel{}
	for _, m := range models {
		if p.Enabled == nil || containsID(p.Enabled, m.ID) {
			selected = append(selected, m)
		}
	}
	if len(selected) == 0 {
		return nil, "", errors.New("no compatible model is offered to this client; enable a model in Apps settings or download one in Models")
	}
	if p.Enabled != nil {
		for _, id := range p.Enabled {
			found := false
			for _, m := range selected {
				if m.ID == id {
					found = true
					break
				}
			}
			if !found {
				return nil, "", errors.New("a selected client model is missing, disabled globally, or does not fit at this context; review the offered models in Apps settings")
			}
		}
	}
	if p.DefaultModel != "" {
		defaultID = p.DefaultModel
	}
	for _, m := range selected {
		if m.ID == defaultID {
			return selected, defaultID, nil
		}
	}
	if p.DefaultModel != "" {
		return nil, "", errors.New("the saved default is unavailable to this client; choose an offered model or Automatic in Apps settings")
	}
	return selected, selected[0].ID, nil
}
func clientMinimumContext(name string) int {
	if name == "opencode" {
		return 16384
	}
	return 8192
}

// Keep launch orchestration independent of terminal/process APIs so cancellation
// and protection of an already-running engine can be exercised without a model.
type clientLaunchEngine interface {
	snapshot() EngineStatus
	start(RunSettings) error
	stop()
}
type ClientLauncher struct {
	root         string
	engine       clientLaunchEngine
	launchMu     sync.Mutex
	prefsMu      sync.Mutex
	validatePack func(string) error
	inventory    func(RunSettings) (Inventory, error)
	open         func(string, string, string, string) error
	lastClient   string
	lastOpened   time.Time
}

func newClientLauncher(root string, engine clientLaunchEngine) *ClientLauncher {
	l := &ClientLauncher{root: root, engine: engine}
	l.validatePack = func(name string) error { _, err := portableClient(root, name, true); return err }
	l.inventory = func(s RunSettings) (Inventory, error) {
		cfg, err := loadSettings(root)
		if err != nil {
			return Inventory{}, err
		}
		o, err := s.options()
		if err != nil {
			return Inventory{}, err
		}
		return inventory(root, cfg, o)
	}
	l.open = func(client, project, task, profile string) error {
		return launchTerminal(root, client, project, task, profile)
	}
	return l
}
func (l *ClientLauncher) launch(ctx context.Context, p ClientLaunchRequest) error {
	if !l.launchMu.TryLock() {
		return fmt.Errorf("%w: another client is opening; wait for its terminal", errLaunchConflict)
	}
	defer l.launchMu.Unlock()
	if !validClient(p.Client) {
		return errors.New("choose Codex, Claude Code or OpenCode")
	}
	if p.Client == l.lastClient && time.Since(l.lastOpened) < 2*time.Second {
		return fmt.Errorf("%w: this client just opened; check its terminal before opening another", errLaunchConflict)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := l.validatePack(p.Client); err != nil {
		return err
	}
	prefs, err := loadLaunchPreferences(l.root)
	if err != nil {
		return err
	}
	project := prefs.Project
	if p.Project != nil {
		project = *p.Project
	}
	project, err = resolveProject(l.root, project)
	if err != nil {
		return fmt.Errorf("project folder is unavailable on this computer: %w. Choose another folder or clear it to use the portable Workspace", err)
	}
	status := l.engine.snapshot()
	settings := configuredRunSettings(status)
	if p.Settings != nil {
		settings = *p.Settings
	}
	if _, err = settings.options(); err != nil {
		return err
	}
	if err = validateModelIDs(settings.Enabled); err != nil {
		return err
	}
	shared := settings
	clientPreference := prefs.Clients[p.Client]
	if clientPreference.Enabled == nil && shared.Enabled != nil {
		clientPreference.Enabled = append([]string{}, shared.Enabled...)
	}
	if !status.Active && !status.Preparing && settings.Context < clientMinimumContext(p.Client) {
		settings.Context = clientMinimumContext(p.Client)
		shared.Context = settings.Context
	}
	settings = gatewayRunSettings(shared, prefs)
	if settings.Context < clientMinimumContext(p.Client) {
		return fmt.Errorf("%w: %s needs at least %s context tokens. Finish the current task, stop the engine, then open the client again", errLaunchConflict, p.Client, formatContext(clientMinimumContext(p.Client)))
	}
	if (status.Active || status.Preparing) && !sameEngineConfiguration(status.Settings, settings) {
		return fmt.Errorf("%w: the running engine uses different settings. Finish its current task and stop it before applying these changes", errLaunchConflict)
	}
	inv, err := l.inventory(settings)
	if err != nil {
		return err
	}
	if !inv.Hardware.Backend.Available {
		return errors.New(inv.Hardware.Backend.Reason)
	}
	available := []ClientModel{}
	defaultID := ""
	for _, m := range inv.Models {
		if m.Eligible && (settings.Enabled == nil || containsID(settings.Enabled, m.ID)) {
			available = append(available, ClientModel{ID: m.ID, Name: m.Name})
			if m.Name == settings.Model {
				defaultID = m.ID
			}
		}
	}
	selected, id, err := selectedClientModels(available, defaultID, clientPreference)
	if err != nil {
		return err
	}
	frozen := ClientLaunchPreference{Enabled: []string{}, DefaultModel: id}
	for _, m := range selected {
		frozen.Enabled = append(frozen.Enabled, m.ID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = ensureClientEngine(ctx, l.engine, settings); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	profile, err := writeLaunchProfile(l.root, frozen)
	if err != nil {
		return err
	}
	if err = l.open(p.Client, project, p.Task, profile); err != nil {
		path, _ := statePath(l.root, profile)
		os.Remove(path)
		return err
	}
	l.lastClient = p.Client
	l.lastOpened = time.Now()
	return nil
}
func formatContext(n int) string {
	if n == 16384 {
		return "16,384"
	}
	return "8,192"
}
func configuredRunSettings(status EngineStatus) RunSettings {
	if status.SavedSettings != nil {
		return *status.SavedSettings
	}
	return status.Settings
}
func gatewayRunSettings(shared RunSettings, prefs LaunchPreferences) RunSettings {
	settings := shared
	if settings.Enabled != nil {
		// The gateway serves the union, but configured/saved choices remain
		// separate so an inherited client does not gain another client's models.
		settings.Enabled = append([]string{}, shared.Enabled...)
		for _, name := range []string{"codex", "claude", "opencode"} {
			for _, id := range prefs.Clients[name].Enabled {
				if !containsID(settings.Enabled, id) {
					settings.Enabled = append(settings.Enabled, id)
				}
			}
		}
	}
	shared.configured = nil
	settings.configured = &shared
	return settings
}
func sameEngineConfiguration(a, b RunSettings) bool {
	if a.Context != b.Context || a.Backend != b.Backend || a.Threads != b.Threads || a.Temperature != b.Temperature || a.Port != b.Port {
		return false
	}
	if (a.Enabled == nil) != (b.Enabled == nil) || len(a.Enabled) != len(b.Enabled) {
		return false
	}
	for _, id := range a.Enabled {
		if !containsID(b.Enabled, id) {
			return false
		}
	}
	return true // Defaults can differ by client without restarting the gateway.
}
func ensureClientEngine(ctx context.Context, engine clientLaunchEngine, settings RunSettings) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	initial := engine.snapshot()
	if initial.Stopping {
		return fmt.Errorf("%w: wait for the engine to finish stopping, then open the client", errLaunchConflict)
	}
	if (initial.Active || initial.Preparing) && !sameEngineConfiguration(initial.Settings, settings) {
		return fmt.Errorf("%w: the running engine uses different settings. Finish its current task and stop it before applying these changes", errLaunchConflict)
	}
	started := false
	if !initial.Active && !initial.Preparing {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := engine.start(settings); err != nil {
			return err
		}
		started = true
	}
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			if started && !engine.snapshot().Ready {
				engine.stop()
			}
			return err
		}
		s := engine.snapshot()
		if s.Ready && !s.Stopping {
			return nil
		}
		if !s.Active && !s.Preparing {
			if s.Error != "" {
				return errors.New(s.Error)
			}
			return errors.New("engine startup was cancelled; open the client again when ready")
		}
		select {
		case <-ctx.Done():
			if started && !engine.snapshot().Ready {
				engine.stop()
			}
			return ctx.Err()
		case <-timer.C:
			if started && !engine.snapshot().Ready {
				engine.stop()
			}
			return errors.New("local model service did not become ready within 45 seconds; check Diagnostics and retry")
		case <-tick.C:
		}
	}
}
func writeLaunchProfile(root string, p ClientLaunchPreference) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	name := "launch-profile-" + hex.EncodeToString(raw) + ".json"
	path, err := statePath(root, name)
	if err != nil {
		return "", err
	}
	return name, writeJSON(path, p)
}
func readLaunchProfile(root, name string) (ClientLaunchPreference, error) {
	var p ClientLaunchPreference
	if !launchProfileName.MatchString(name) {
		return p, errors.New("invalid launch profile")
	}
	path, err := statePath(root, name)
	if err != nil {
		return p, err
	}
	if err = readJSON(path, &p); err != nil {
		return p, errors.New("client launch profile is unavailable; reopen the client from Reflect Go")
	}
	defer os.Remove(path)
	if err = validateLaunchPreferences(LaunchPreferences{Clients: map[string]ClientLaunchPreference{"codex": p}}); err != nil {
		return p, err
	}
	return p, nil
}
func (l *ClientLauncher) routes(mux *http.ServeMux, manager *EngineManager, decode func(http.ResponseWriter, *http.Request, any) bool, send func(http.ResponseWriter, any)) {
	mux.HandleFunc("/api/client", func(w http.ResponseWriter, r *http.Request) {
		var p ClientLaunchRequest
		if !decode(w, r, &p) {
			return
		}
		if err := l.launch(r.Context(), p); err != nil {
			code := 400
			if errors.Is(err, errLaunchConflict) {
				code = 409
			}
			http.Error(w, err.Error(), code)
			return
		}
		send(w, map[string]bool{"opened": true})
	})
	mux.HandleFunc("/api/launch-preferences", func(w http.ResponseWriter, r *http.Request) {
		l.prefsMu.Lock()
		defer l.prefsMu.Unlock()
		if r.Method == "GET" {
			p, err := loadLaunchPreferences(l.root)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			warning := ""
			if p.Project != "" {
				if _, err := resolveProject(l.root, p.Project); err != nil {
					p.Project = ""
					warning = "The saved project folder is unavailable on this computer. Choose another folder or use the portable Workspace."
				}
			}
			send(w, map[string]any{"project": p.Project, "clients": p.Clients, "projectWarning": warning})
			return
		}
		var p LaunchPreferences
		if !decode(w, r, &p) {
			return
		}
		if err := validateLaunchPreferences(p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if p.Project != "" {
			project, err := resolveProject(l.root, p.Project)
			if err != nil {
				http.Error(w, "Choose an existing project folder or clear it to use the portable Workspace", 400)
				return
			}
			p.Project = project
		}
		path, err := statePath(l.root, "launch-preferences.json")
		if err == nil {
			err = writeJSON(path, p)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		send(w, p)
	})
	mux.HandleFunc("/api/run-settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			send(w, configuredRunSettings(manager.snapshot()))
			return
		}
		var s RunSettings
		if !decode(w, r, &s) {
			return
		}
		if _, err := s.options(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := validateModelIDs(s.Enabled); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		manager.startMu.Lock()
		defer manager.startMu.Unlock()
		manager.mu.Lock()
		defer manager.mu.Unlock()
		if manager.status.Active || manager.status.Preparing {
			http.Error(w, "Stop the engine after the current task finishes before saving engine settings", 409)
			return
		}
		path, err := statePath(l.root, "workstation-web.json")
		if err == nil {
			err = writeJSON(path, s)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		manager.status.Settings = s
		manager.status.SavedSettings = &s
		send(w, s)
	})
}
