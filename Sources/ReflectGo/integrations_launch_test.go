package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIntegrationModelsPreserveSharedSelectionAndDefault(t *testing.T) {
	first, second, other := modelID("first.gguf"), modelID("second.gguf"), modelID("private.gguf")
	shared := defaultRunSettings()
	shared.Enabled = []string{first, second}
	shared.Model = "second.gguf"
	prefs := defaultLaunchPreferences()
	prefs.Clients["codex"] = ClientLaunchPreference{Enabled: []string{other}}
	gateway := gatewayRunSettings(shared, prefs)
	if !containsID(gateway.Enabled, other) {
		t.Fatal("gateway lost CLI model")
	}
	models, id, err := integrationModels([]ClientModel{{ID: first}, {ID: other}, {ID: second}}, other, shared)
	if err != nil || id != second || len(models) != 2 || models[0].ID != second || models[1].ID != first {
		t.Fatal(models, id, err)
	}
	shared.Enabled = []string{}
	if _, _, err := integrationModels(models, second, shared); err == nil {
		t.Fatal("empty shared selection exposed another client's model")
	}
}
func TestDesktopChangeRequiresRestartConfirmationBeforeMutations(t *testing.T) {
	events := []string{}
	actions := desktopChangeActions{Running: func() (bool, error) { return true, nil }, Quit: func(context.Context) error { events = append(events, "quit"); return nil }, Apply: func() error { events = append(events, "apply"); return nil }, Open: func(context.Context) error { events = append(events, "open"); return nil }}
	if err := runDesktopChange(context.Background(), false, true, actions); !errors.Is(err, errDesktopRestart) {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatal("restart was not confirmed", events)
	}
	if err := runDesktopChange(context.Background(), true, true, actions); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"quit", "apply", "open"}) {
		t.Fatal(events)
	}
}
func TestDesktopQuitAllowsTimeForAppConfirmation(t *testing.T) {
	var remaining time.Duration
	actions := desktopChangeActions{Running: func() (bool, error) { return true, nil }, Quit: func(ctx context.Context) error {
		deadline, _ := ctx.Deadline()
		remaining = time.Until(deadline)
		return nil
	}, Apply: func() error { return nil }, Open: func(context.Context) error { return nil }}
	if err := runDesktopChange(context.Background(), true, true, actions); err != nil {
		t.Fatal(err)
	}
	if remaining < 30*time.Second {
		t.Fatal("quit wait too short for a desktop app's own quit confirmation:", remaining)
	}
	if remaining >= time.Minute {
		t.Fatal("quit wait outlasts the web view's 60-second request limit:", remaining)
	}
}
func TestDesktopDisconnectDoesNotOpenClosedApp(t *testing.T) {
	events := []string{}
	actions := desktopChangeActions{Running: func() (bool, error) { return false, nil }, Quit: func(context.Context) error { events = append(events, "quit"); return nil }, Apply: func() error { events = append(events, "restore"); return nil }, Open: func(context.Context) error { events = append(events, "open"); return nil }}
	if err := runDesktopChange(context.Background(), false, false, actions); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"restore"}) {
		t.Fatal("disconnect launched a previously closed app", events)
	}
}
func TestCancelledDesktopChangeDoesNotEditOrLaunch(t *testing.T) {
	for _, when := range []string{"before", "during quit", "after apply"} {
		t.Run(when, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			events := []string{}
			actions := desktopChangeActions{Running: func() (bool, error) { return true, nil }, Quit: func(context.Context) error {
				events = append(events, "quit")
				if when == "during quit" {
					cancel()
				}
				return nil
			}, Apply: func() error {
				events = append(events, "apply")
				if when == "after apply" {
					cancel()
				}
				return nil
			}, Open: func(context.Context) error { events = append(events, "open"); return nil }, Rollback: func() error { events = append(events, "rollback"); return nil }}
			if when == "before" {
				cancel()
			}
			if err := runDesktopChange(ctx, true, true, actions); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			want := []string{}
			if when == "during quit" {
				want = []string{"quit"}
			}
			if when == "after apply" {
				want = []string{"quit", "apply", "rollback"}
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatal(events, want)
			}
		})
	}
}
func TestFailedDesktopOpenRestoresPreviousConnection(t *testing.T) {
	events := []string{}
	failure := errors.New("could not open desktop")
	actions := desktopChangeActions{Running: func() (bool, error) { return false, nil }, Apply: func() error { events = append(events, "apply"); return nil }, Open: func(context.Context) error { events = append(events, "open"); return failure }, Rollback: func() error { events = append(events, "restore"); return nil }}
	if err := runDesktopChange(context.Background(), false, true, actions); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"apply", "open", "restore"}) {
		t.Fatal(events)
	}
}
func TestUnownedDesktopDisconnectStopsBeforeAnyHostAction(t *testing.T) {
	root, _ := productFixture(t)
	manager := newIntegrationManager(root, newManager(root))
	mux := http.NewServeMux()
	manager.routes(mux)
	// No journal: this fails before checking or quitting any host application.
	req := httptest.NewRequest("POST", "/api/integrations/disconnect", strings.NewReader(`{"id":"claude-desktop","confirmRestart":true}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code == 200 {
		t.Fatal("disconnect succeeded without an owned connection")
	}
	if manager.closeProxy != nil {
		t.Fatal("started a proxy during disconnect")
	}
}
