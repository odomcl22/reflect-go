package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeLaunchEngine struct {
	mu            sync.Mutex
	state         EngineStatus
	starts, stops int
	ready         bool
	started       chan struct{}
}

func (e *fakeLaunchEngine) snapshot() EngineStatus { e.mu.Lock(); defer e.mu.Unlock(); return e.state }
func (e *fakeLaunchEngine) start(s RunSettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starts++
	saved := s
	if s.configured != nil {
		saved = *s.configured
	}
	s.configured = nil
	e.state = EngineStatus{Active: true, Ready: e.ready, Settings: s, SavedSettings: &saved}
	if e.started != nil {
		close(e.started)
	}
	return nil
}
func (e *fakeLaunchEngine) stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stops++
	e.state.Active = false
	e.state.Ready = false
}
func fixtureClientLauncher(t *testing.T) (*ClientLauncher, *fakeLaunchEngine) {
	t.Helper()
	root, _ := productFixture(t)
	e := &fakeLaunchEngine{state: EngineStatus{Settings: defaultRunSettings()}, ready: true}
	l := newClientLauncher(root, e)
	l.validatePack = func(string) error { return nil }
	l.inventory = func(s RunSettings) (Inventory, error) {
		return Inventory{Hardware: Hardware{Backend: Backend{Available: true}}, Models: []Model{{ID: modelID("first.gguf"), Name: "first.gguf", Eligible: true}, {ID: modelID("second.gguf"), Name: "second.gguf", Eligible: true}}}, nil
	}
	return l, e
}
func saveTestLaunchPreferences(t *testing.T, root string, p LaunchPreferences) {
	t.Helper()
	path, err := statePath(root, "launch-preferences.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = writeJSON(path, p); err != nil {
		t.Fatal(err)
	}
}
func TestOneClickLaunchStartsServiceAndFreezesSelectedCatalog(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	chosen := modelID("second.gguf")
	prefs := defaultLaunchPreferences()
	prefs.Clients["codex"] = ClientLaunchPreference{Enabled: []string{chosen}, DefaultModel: chosen}
	saveTestLaunchPreferences(t, l.root, prefs)
	opened := 0
	l.open = func(client, project, task, profile string) error {
		opened++
		if !e.snapshot().Ready {
			t.Fatal("opened a client before service ready")
		}
		if filepath.Base(project) != "Workspace" {
			t.Fatal(project)
		}
		// Editing future settings must not change the command currently opening.
		saveTestLaunchPreferences(t, l.root, defaultLaunchPreferences())
		frozen, err := readLaunchProfile(l.root, profile)
		if err != nil {
			return err
		}
		models, id, err := selectedClientModels([]ClientModel{{ID: modelID("first.gguf")}, {ID: chosen}}, modelID("first.gguf"), frozen)
		if err != nil || id != chosen || len(models) != 1 || models[0].ID != chosen {
			t.Fatalf("launch profile widened or lost default: %+v %s %v", models, id, err)
		}
		if _, err := os.Stat(filepath.Join(l.root, "state", profile)); !os.IsNotExist(err) {
			t.Fatal("consumed profile remained on disk")
		}
		return nil
	}
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex"}); err != nil {
		t.Fatal(err)
	}
	if opened != 1 || e.starts != 1 {
		t.Fatal(opened, e.starts)
	}
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex"}); !errors.Is(err, errLaunchConflict) {
		t.Fatal("duplicate click opened client", err)
	}
	if opened != 1 {
		t.Fatal("duplicate terminal")
	}
}
func TestOneClickDoesNotStartForInvalidPrerequisites(t *testing.T) {
	for _, reason := range []string{"pack", "project", "no models", "missing model"} {
		t.Run(reason, func(t *testing.T) {
			l, e := fixtureClientLauncher(t)
			opened := false
			l.open = func(string, string, string, string) error { opened = true; return nil }
			p := ClientLaunchRequest{Client: "codex"}
			switch reason {
			case "pack":
				l.validatePack = func(string) error { return errors.New("missing pack") }
			case "project":
				missing := filepath.Join(t.TempDir(), "missing")
				p.Project = &missing
			case "no models":
				prefs := defaultLaunchPreferences()
				prefs.Clients["codex"] = ClientLaunchPreference{Enabled: []string{}}
				saveTestLaunchPreferences(t, l.root, prefs)
			case "missing model":
				prefs := defaultLaunchPreferences()
				prefs.Clients["codex"] = ClientLaunchPreference{Enabled: []string{modelID("missing.gguf")}}
				saveTestLaunchPreferences(t, l.root, prefs)
			}
			if err := l.launch(context.Background(), p); err == nil {
				t.Fatal("invalid launch accepted")
			}
			if e.starts != 0 || opened {
				t.Fatal("invalid launch caused side effects")
			}
		})
	}
}
func TestOneClickPreservesActiveEngine(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	e.state.Active = true
	e.state.Ready = true
	e.state.Settings.Context = 8192
	l.open = func(string, string, string, string) error { t.Fatal("opened incompatible client"); return nil }
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "opencode"}); !errors.Is(err, errLaunchConflict) {
		t.Fatal(err)
	}
	if e.starts != 0 || e.stops != 0 || !e.state.Ready {
		t.Fatal("changed active engine")
	}
	settings := e.state.Settings
	settings.Port++
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex", Settings: &settings}); !errors.Is(err, errLaunchConflict) {
		t.Fatal(err)
	}
	if e.starts != 0 || e.stops != 0 {
		t.Fatal("restarted active engine")
	}
}
func TestOneClickCancellationAndConcurrentClick(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	e.ready = false
	e.started = make(chan struct{})
	l.open = func(string, string, string, string) error { t.Error("opened cancelled client"); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.launch(ctx, ClientLaunchRequest{Client: "codex"}) }()
	select {
	case <-e.started:
	case <-time.After(time.Second):
		t.Fatal("service was not started")
	}
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "claude"}); !errors.Is(err, errLaunchConflict) {
		t.Fatal("concurrent click not rejected", err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("launch ignored cancellation")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.starts != 1 || e.stops != 1 {
		t.Fatal("cancelled startup left running", e.starts, e.stops)
	}
}
func TestLaunchPreferencesEmptyAndInheritedSelectionsStayDistinct(t *testing.T) {
	root, _ := productFixture(t)
	p := defaultLaunchPreferences()
	p.Clients["claude"] = ClientLaunchPreference{Enabled: []string{}}
	saveTestLaunchPreferences(t, root, p)
	got, err := loadLaunchPreferences(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Clients["claude"].Enabled == nil || got.Clients["codex"].Enabled != nil {
		t.Fatal("lost explicit empty or inherit", got)
	}
	data, _ := json.Marshal(RunSettings{Enabled: []string{}})
	if !strings.Contains(string(data), `"enabled":[]`) {
		t.Fatal(string(data))
	}
	if _, _, err := selectedClientModels([]ClientModel{{ID: modelID("test.gguf")}}, "", got.Clients["claude"]); err == nil {
		t.Fatal("empty selection exposed models")
	}
}
func TestStaleSavedProjectRequiresExplicitWorkspaceChoice(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	p := defaultLaunchPreferences()
	p.Project = filepath.Join(t.TempDir(), "absent")
	saveTestLaunchPreferences(t, l.root, p)
	l.open = func(string, string, string, string) error { return nil }
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex"}); err == nil || !strings.Contains(err.Error(), "Workspace") {
		t.Fatal(err)
	}
	if e.starts != 0 {
		t.Fatal("started with stale project")
	}
	blank := ""
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex", Project: &blank}); err != nil {
		t.Fatal(err)
	}
}
func TestStoppedEngineAdaptsContextBeforeCheckingModelFit(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	e.state.Settings.Context = 2048
	l.inventory = func(s RunSettings) (Inventory, error) {
		if s.Context != 16384 {
			t.Fatal("screened the wrong context", s.Context)
		}
		return Inventory{Hardware: Hardware{Backend: Backend{Available: true}}, Models: []Model{{ID: modelID("test.gguf"), Name: "test.gguf", Eligible: true}}}, nil
	}
	l.open = func(string, string, string, string) error { return nil }
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "opencode"}); err != nil {
		t.Fatal(err)
	}
	if e.snapshot().Settings.Context != 16384 {
		t.Fatal("client started at insufficient context")
	}
}

func TestCustomClientSelectionExpandsStoppedGatewayOnly(t *testing.T) {
	l, e := fixtureClientLauncher(t)
	first, second := modelID("first.gguf"), modelID("second.gguf")
	e.state.Settings.Enabled = []string{first}
	p := defaultLaunchPreferences()
	p.Clients["codex"] = ClientLaunchPreference{Enabled: []string{second}, DefaultModel: second}
	saveTestLaunchPreferences(t, l.root, p)
	l.open = func(_, _, _, name string) error {
		profile, err := readLaunchProfile(l.root, name)
		if err != nil {
			return err
		}
		if len(profile.Enabled) != 1 || profile.Enabled[0] != second {
			t.Fatal("client inherited unselected gateway model", profile)
		}
		return nil
	}
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "codex"}); err != nil {
		t.Fatal(err)
	}
	if got := e.snapshot().Settings.Enabled; !containsID(got, first) || !containsID(got, second) {
		t.Fatal("gateway missed union", got)
	}
	if got := configuredRunSettings(e.snapshot()).Enabled; len(got) != 1 || got[0] != first {
		t.Fatal("client-specific models widened shared defaults", got)
	}
	// The next inherited client should still receive only the original shared set.
	l.open = func(_, _, _, name string) error {
		profile, err := readLaunchProfile(l.root, name)
		if err != nil {
			return err
		}
		if len(profile.Enabled) != 1 || profile.Enabled[0] != first {
			t.Fatal("inherited client received another client's private selection", profile)
		}
		return nil
	}
	if err := l.launch(context.Background(), ClientLaunchRequest{Client: "claude"}); err != nil {
		t.Fatal(err)
	}
	if e.starts != 1 {
		t.Fatal("second client restarted shared gateway")
	}
}

func TestConnectorCatalogUsesFrozenSelection(t *testing.T) {
	root, _ := productFixture(t)
	first, second := modelID("first.gguf"), modelID("second.gguf")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []ClientModel{{ID: first, Name: "First"}, {ID: second, Name: "Second"}}})
	}))
	defer server.Close()
	provider, _ := statePath(root, "provider.json")
	if err := writeJSON(provider, Connection{Base: server.URL, Gateway: true, ID: first, Context: 16384}); err != nil {
		t.Fatal(err)
	}
	key, _ := statePath(root, "provider-key")
	os.WriteFile(key, []byte(strings.Repeat("a", 64)), 0600)
	bin := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	os.WriteFile(filepath.Join(bin, name), []byte("fixture; never executed"), 0700)
	t.Setenv("PATH", bin)
	// The saved preference offers every model; a frozen launch overrides it.
	saveTestLaunchPreferences(t, root, defaultLaunchPreferences())
	frozen := ClientLaunchPreference{Enabled: []string{second}, DefaultModel: second}
	cmd, err := clientCommandWithProfile(root, "codex", "host", t.TempDir(), "", &frozen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), `model="`+second+`"`) {
		t.Fatal("client did not receive chosen default", cmd.Args)
	}
	var catalog struct {
		Models []struct {
			Slug string `json:"slug"`
		} `json:"models"`
	}
	path, _ := statePath(root, "codex-models.json")
	if err := readJSON(path, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 1 || catalog.Models[0].Slug != second {
		t.Fatal("client catalog exposed unselected models", catalog)
	}
}
