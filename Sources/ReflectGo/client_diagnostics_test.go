package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClientLaunchFailureIsSaved(t *testing.T) {
	root := t.TempDir()
	if err := connectClient(root, []string{"codex", "--project", filepath.Join(root, "missing")}); err == nil {
		t.Fatal("missing project unexpectedly accepted")
	}
	data, err := os.ReadFile(filepath.Join(root, "state/client-launch-codex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record["stage"] != "checking project folder" || record["error"] == nil {
		t.Fatalf("lost connector failure: %s", data)
	}
	secret := strings.Repeat("a", 64)
	recordClientLaunch(root, "codex", "preflight", errors.New("failed with "+secret))
	data, _ = os.ReadFile(filepath.Join(root, "state/client-launch-codex.json"))
	if strings.Contains(string(data), secret) || !strings.Contains(string(data), "[redacted]") {
		t.Fatal("diagnostic leaked local API key")
	}
}

func TestClientPreflightExplainsAuthenticationFailure(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, name), []byte("test fixture; never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	key := strings.Repeat("a", 64)
	for _, status := range []int{401, 403, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+key {
					t.Error("preflight sent wrong credentials or path")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			path, _ := statePath(root, "provider.json")
			writeJSON(path, Connection{Base: server.URL, Context: 16384})
			os.WriteFile(filepath.Join(root, "state/provider-key"), []byte(key+"\n"), 0600)
			_, err := clientCommand(root, "codex", "host", t.TempDir(), "")
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
				t.Fatalf("unhelpful preflight failure: %v", err)
			}
			if status != 503 && !strings.Contains(err.Error(), "rejected the connection key") {
				t.Fatalf("auth failure not identified: %v", err)
			}
		})
	}
}
