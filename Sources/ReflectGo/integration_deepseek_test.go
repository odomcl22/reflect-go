package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCancelledGraphicalLaunchDoesNotPrepareState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	for _, launch := range []func(context.Context, string, string, Connection, string, []ClientModel) (string, error){launchDeepSeekContext, launchOpenCodeWebContext} {
		if _, err := launch(ctx, root, "", Connection{}, "", nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled launch continued: %v", err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled launch prepared a profile: %v %v", entries, err)
	}
}

func TestDeepSeekUsesOwnedSettingsAndCatalog(t *testing.T) {
	root, project := t.TempDir(), t.TempDir()
	connection := Connection{Base: "http://127.0.0.1:8766", Context: 16384, ID: "second"}
	cmd, err := deepSeekCommand(root, project, connection, "local-test-credential", []ClientModel{{ID: "first", Name: "Small model"}, {ID: "second", Name: "Coding model"}}, "/not-run/dsh")
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, v := range cmd.Env {
		parts := strings.SplitN(v, "=", 2)
		env[parts[0]] = parts[1]
	}
	if !contained(env["DSH_HOME"], root) || env[deepSeekKeyEnv] != "local-test-credential" || env["DSH_PERMISSION_MODE"] != "workspace-write" {
		t.Fatal("launch did not isolate data/credentials and retain approvals")
	}
	settingsData, err := os.ReadFile(filepath.Join(env["DSH_HOME"], "reflect-go-settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settingsData), "local-test-credential") {
		t.Fatal("launch credential was stored in settings")
	}
	var settings map[string]any
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["agent-default-model"].(map[string]any)["model"] != "second" {
		t.Fatal("default model was not passed to DeepSeek")
	}
	provider := settings["llm-pi-ai"].(map[string]any)["providers"].(map[string]any)["reflect-go"].(map[string]any)
	if provider["api"] != "openai-completions" || provider["baseURL"] != connection.Base+"/v1" || len(provider["models"].([]any)) != 2 {
		t.Fatal("selected model catalog or local endpoint missing")
	}
	if cmd.Dir != project || !strings.Contains(strings.Join(cmd.Args, " "), "--host 127.0.0.1 --port 0 --no-open") {
		t.Fatal("wrong project or exposure of harness beyond loopback")
	}
}

func TestDeepSeekRejectsRemoteEndpointAndTooSmallContext(t *testing.T) {
	models := []ClientModel{{ID: "one", Name: "Model"}}
	for _, c := range []Connection{{Base: "https://example.com", Context: 16384}, {Base: "http://127.0.0.1:8766", Context: 8192}, {Base: "http://127.0.0.1:8766/remote", Context: 16384}} {
		if _, err := deepSeekCommand(t.TempDir(), t.TempDir(), c, "test", models, "/not-run/dsh"); err == nil {
			t.Fatalf("invalid connection accepted: %+v", c)
		}
	}
}

func TestDeepSeekReadyOnlyAcceptsLoopbackAnnouncement(t *testing.T) {
	if got := deepSeekReadyURL("startup\ndsh web: http://127.0.0.1:30123\ndsh web: opening the default browser\n"); got != "http://127.0.0.1:30123" {
		t.Fatal(got)
	}
	for _, text := range []string{"dsh web: http://evil.example:3000", "request: dsh web: http://127.0.0.1:3000", "dsh web: http://127.0.0.1:3000.evil.test"} {
		if deepSeekReadyURL(text) != "" {
			t.Fatal("untrusted launch URL accepted")
		}
	}
}
