package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/pelletier/go-toml/v2"
)

func TestDesktopProfilesRestoreOnlyOwnedFields(t *testing.T) {
	for _, id := range []string{"codex-desktop", "opencode-desktop"} {
		t.Run(id, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("OPENCODE_CONFIG", "")
			t.Setenv("OPENCODE_CONFIG_CONTENT", "")
			config, _ := desktopConfigPaths(home, id)
			os.MkdirAll(filepath.Dir(config), 0700)
			initial := `{"model":"usual","provider":{"existing":{"name":"Keep me"}},"permission":{"*":"ask"}}`
			if id == "codex-desktop" {
				initial = "model = 'usual'\nmodel_provider = 'openai'\nmodel_context_window = 12345\n[features]\napps = true\n"
			}
			os.WriteFile(config, []byte(initial), 0600)
			j, err := planDesktopProfile(home, "test-host", id, "http://127.0.0.1:1234/secret", "local-key", Connection{ID: "reflect-test", Context: 49152}, []ClientModel{{ID: "reflect-test", Name: "test.gguf"}})
			if err != nil {
				t.Fatal(err)
			}
			record := filepath.Join(home, "restore.json")
			if err = applyProfileJournal(record, j); err != nil {
				t.Fatal(err)
			}
			if id == "codex-desktop" {
				data, _ := os.ReadFile(config)
				var v map[string]any
				if err = toml.Unmarshal(data, &v); err != nil {
					t.Fatal(err)
				}
				if _, ok := v["model_context_window"].(int64); !ok {
					t.Fatal("integer config type changed")
				}
				if _, ok := v["model_provider"]; ok {
					t.Fatal("native provider not selected")
				}
			}
			format := ""
			if id == "codex-desktop" {
				format = "toml"
			}
			v, _ := readProfile(config, format)
			v["unrelated_new_setting"] = json.RawMessage("true")
			if err = writeProfile(config, v, format); err != nil {
				t.Fatal(err)
			}
			if err = restoreProfileJournal(record, "test-host", home); err != nil {
				t.Fatal(err)
			}
			v, _ = readProfile(config, format)
			if string(v["model"]) != `"usual"` || string(v["unrelated_new_setting"]) != "true" {
				t.Fatal("restoration lost settings", v)
			}
			if id == "opencode-desktop" {
				if len(profileValue(v, "provider/reflect_go")) != 0 || len(profileValue(v, "provider/existing")) == 0 {
					t.Fatal("wrong provider restored")
				}
			}
		})
	}
}
func TestCodexDesktopRoutesLocalWithoutAccountCredential(t *testing.T) {
	localCalls, cloudCalls := 0, 0
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localCalls++
		if r.Header.Get("Authorization") != "Bearer local-key" || r.Header.Get("ChatGPT-Account-ID") != "" || r.URL.Path != "/v1/responses" {
			t.Error("local request exposed account headers or wrong path")
		}
		data, _ := io.ReadAll(r.Body)
		if !bytes.Contains(data, []byte(`"store":false`)) {
			t.Error("local normalization absent")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: local\n\n")
	}))
	defer local.Close()
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloudCalls++; io.WriteString(w, "native") }))
	defer cloud.Close()
	p := &codexDesktopProxy{host: "127.0.0.1:1111", prefix: "/secret", key: "local-key", models: map[string]bool{"reflect-test": true}}
	p.local, _ = desktopReverseProxy(local.URL + "/v1")
	p.api, _ = desktopReverseProxy(cloud.URL)
	p.account = p.api
	for _, encoding := range []string{"", "gzip", "zstd"} {
		payload := []byte(`{"model":"reflect-test","input":"hi","stream":true}`)
		var buffer bytes.Buffer
		switch encoding {
		case "gzip":
			z := gzip.NewWriter(&buffer)
			z.Write(payload)
			z.Close()
			payload = buffer.Bytes()
		case "zstd":
			z, _ := zstd.NewWriter(nil)
			payload = z.EncodeAll(payload, nil)
			z.Close()
		}
		r := httptest.NewRequest("POST", "http://"+p.host+"/secret/v1/responses", bytes.NewReader(payload))
		r.Header.Set("Content-Encoding", encoding)
		r.Header.Set("Authorization", "Bearer account-secret")
		r.Header.Set("ChatGPT-Account-ID", "private-account")
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "data: local") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, test := range []struct {
		path, model, auth string
		code              int
	}{{"/secret/v1/responses", "reflect-missing", "account-secret", 400}, {"/wrong/v1/responses", "reflect-test", "account-secret", 404}, {"/secret/v1/responses", "gpt-native", "local-key", 401}, {"/secret/v1/responses", "gpt-native", "account-secret", 200}} {
		r := httptest.NewRequest("POST", "http://"+p.host+test.path, strings.NewReader(`{"model":"`+test.model+`"}`))
		r.Header.Set("Authorization", "Bearer "+test.auth)
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != test.code {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if localCalls != 3 || cloudCalls != 1 {
		t.Fatal("routing counts", localCalls, cloudCalls)
	}
}
func TestDesktopContextOptions(t *testing.T) {
	for _, n := range []string{"49152", "65536", "131072"} {
		if _, err := parseOptions([]string{"serve", "--context", n}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := parseOptions([]string{"serve", "--context", "131073"}); err == nil {
		t.Fatal("unbounded context accepted")
	}
}

func TestOpenCodeCommentedConfigAndCreatedAuthRestoration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	directory := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(directory, 0700)
	path := filepath.Join(directory, "opencode.jsonc")
	original := `{
 // Keep my permissions and explanatory comments.
 "permission":{"*":"ask",},
 "provider": { /* existing provider */ "other":{"name":"https://example.invalid"}, },
 }`
	os.WriteFile(path, []byte(original), 0600)
	for _, id := range []string{"opencode-desktop", "codex-desktop"} {
		j, err := planDesktopProfile(home, "host", id, "http://127.0.0.1:1234", "key", Connection{ID: "reflect-test", Context: 49152}, []ClientModel{{ID: "reflect-test", Name: "test"}})
		if err != nil {
			t.Fatal(err)
		}
		record := filepath.Join(home, id+"-restore.json")
		if err = applyProfileJournal(record, j); err != nil {
			t.Fatal(err)
		}
		if err = restoreProfileJournal(record, "host", home); err != nil {
			t.Fatal(err)
		}
	}
	result, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(result, []byte("Keep my permissions")) || !bytes.Contains(result, []byte("existing provider")) {
		t.Fatal("comments lost", err, string(result))
	}
	parsed, err := profileJSON(path)
	if err != nil || len(profileValue(parsed, "provider/reflect_go")) != 0 || len(profileValue(parsed, "provider/other")) == 0 {
		t.Fatal("wrong settings restored", err)
	}
	for _, name := range []string{"auth.json", "config.toml", "reflect-go-models.json"} {
		if _, err := os.Stat(filepath.Join(home, ".codex", name)); !os.IsNotExist(err) {
			t.Fatal("temporary Codex file left behind", name, err)
		}
	}
}

func TestClaudeDesktopBoundsLocalOutput(t *testing.T) {
	base, observed := claudeProxyFixture(t, 1)
	status, _, _ := claudeProxyRequest(t, base, "POST", "/v1/messages", `{"model":"claude-fable-5","max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"max"},"messages":[{"role":"user","content":"hello"}]}`, map[string]string{"Authorization": "Bearer fixture-key"}, "")
	if status != 200 {
		t.Fatal(status)
	}
	requests := observed()
	if len(requests) != 1 {
		t.Fatal("missing request")
	}
	body := requests[0].Body
	if string(body["max_tokens"]) != "4096" || body["thinking"] != nil || body["output_config"] != nil {
		t.Fatal("unsupported effort/output budget forwarded", body)
	}
}

// Opt-in: uses a real app-server with a temporary profile, never the running UI.
func TestInstalledCodexDesktopCatalog(t *testing.T) {
	binary := os.Getenv("REFLECT_GO_TEST_CODEX_BINARY")
	if binary == "" {
		t.Skip("set REFLECT_GO_TEST_CODEX_BINARY")
	}
	home := t.TempDir()
	base := "http://127.0.0.1:1/reflect-go/test"
	if engine := os.Getenv("REFLECT_GO_TEST_ENGINE_BASE"); engine != "" {
		key, err := os.ReadFile(os.Getenv("REFLECT_GO_TEST_ENGINE_KEY_FILE"))
		if err != nil {
			t.Fatal(err)
		}
		var closeProxy func()
		base, closeProxy, err = startCodexDesktopProxy(Connection{Base: engine, Context: 49152}, strings.TrimSpace(string(key)), []ClientModel{{ID: "reflect-test", Name: "Test model"}})
		if err != nil {
			t.Fatal(err)
		}
		defer closeProxy()
	}
	j, err := planDesktopProfile(home, "test-host", "codex-desktop", base, "test-key", Connection{ID: "reflect-test", Context: 49152}, []ClientModel{{ID: "reflect-test", Name: "Test model"}})
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(home, "restore.json")
	if err = applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	defer restoreProfileJournal(journal, "test-host", home)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "CODEX_HOME="+filepath.Join(home, ".codex"))
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); cancel(); cmd.Wait() }()
	encoder := json.NewEncoder(in)
	decoder := json.NewDecoder(out)
	rpc := func(id int, method string, params any) map[string]json.RawMessage {
		t.Helper()
		encoder.Encode(map[string]any{"id": id, "method": method, "params": params})
		for {
			var v map[string]json.RawMessage
			if err := decoder.Decode(&v); err != nil {
				t.Fatalf("app-server %s: %v: %s", method, err, stderr.String())
			}
			if string(v["id"]) == fmt.Sprint(id) {
				if len(v["error"]) != 0 {
					t.Fatalf("app-server error: %s", v["error"])
				}
				return v
			}
		}
	}
	rpc(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "reflect_go_test", "version": "0.4.8"}, "capabilities": map[string]bool{"experimentalApi": true}})
	encoder.Encode(map[string]any{"method": "initialized"})
	result := rpc(2, "model/list", map[string]any{"includeHidden": true})
	if !bytes.Contains(result["result"], []byte("reflect-test")) {
		t.Fatalf("local model missing: %s", result["result"])
	}
	t.Log("Installed app-server loaded Reflect Go's model catalog without touching the host account")
	if os.Getenv("REFLECT_GO_TEST_ENGINE_BASE") != "" {
		started := rpc(3, "thread/start", map[string]any{"model": "reflect-test", "cwd": home, "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": true})
		var result struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if err = json.Unmarshal(started["result"], &result); err != nil {
			t.Fatal(err)
		}
		rpc(4, "turn/start", map[string]any{"threadId": result.Thread.ID, "input": []any{map[string]any{"type": "text", "text": "Reply exactly OK. Do not use any tools.", "text_elements": []any{}}}})
		textFound := false
		for {
			var event map[string]json.RawMessage
			if err = decoder.Decode(&event); err != nil {
				t.Fatalf("inference failed: %v: %s", err, stderr.String())
			}
			if string(event["method"]) == `"item/agentMessage/delta"` {
				textFound = true
			}
			if string(event["method"]) == `"error"` {
				t.Fatalf("model error: %s", event["params"])
			}
			if string(event["method"]) == `"turn/completed"` {
				if !textFound || !bytes.Contains(event["params"], []byte(`"status":"completed"`)) {
					t.Fatalf("turn did not succeed: %s", event["params"])
				}
				break
			}
		}
		t.Log("Installed desktop app-server completed a real local model turn through the private Responses router")
	}
}

func TestInstalledOpenCodeGlobalProfile(t *testing.T) {
	binary := os.Getenv("REFLECT_GO_TEST_OPENCODE_BINARY")
	if binary == "" {
		t.Skip("set REFLECT_GO_TEST_OPENCODE_BINARY")
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	j, err := planDesktopProfile(home, "host", "opencode-desktop", "http://127.0.0.1:18766", "fixture-key", Connection{ID: "reflect-test", Context: 49152}, []ClientModel{{ID: "reflect-test", Name: "Test model"}})
	if err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(home, "restore.json")
	if err = applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	defer restoreProfileJournal(journal, "host", home)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "debug", "config")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+filepath.Join(home, "data"), "XDG_CACHE_HOME="+filepath.Join(home, "cache"), "XDG_STATE_HOME="+filepath.Join(home, "state"), "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true", "OPENCODE_DISABLE_DEFAULT_PLUGINS=true", "OPENCODE_DISABLE_PROJECT_CONFIG=true", "OPENCODE_CONFIG_DIR=")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(output, &value) != nil {
		t.Fatal("OpenCode did not return JSON config")
	}
	if string(value["model"]) != `"reflect_go/reflect-test"` || !bytes.Contains(value["provider"], []byte("http://127.0.0.1:18766/v1")) {
		t.Fatal("OpenCode did not load the configured local model")
	}
	t.Log("OpenCode loaded the global provider and selected model from the desktop configuration")
}

func TestDesktopModelPickerDoesNotBlockDisconnect(t *testing.T) {
	for _, id := range []string{"codex-desktop", "opencode-desktop"} {
		home := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("OPENCODE_CONFIG", "")
		t.Setenv("OPENCODE_CONFIG_CONTENT", "")
		path, _ := desktopConfigPaths(home, id)
		format := ""
		if id == "codex-desktop" {
			format = "toml"
		}
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := writeProfile(path, map[string]json.RawMessage{"model": json.RawMessage(`"original"`)}, format); err != nil {
			t.Fatal(err)
		}
		j, err := planDesktopProfile(home, "host", id, "http://127.0.0.1:1", "key", Connection{ID: "reflect-a", Context: 49152}, []ClientModel{{ID: "reflect-a"}, {ID: "reflect-b"}})
		if err != nil {
			t.Fatal(err)
		}
		journal := filepath.Join(home, "restore.json")
		if err = applyProfileJournal(journal, j); err != nil {
			t.Fatal(err)
		}
		value, _ := readProfile(path, format)
		selected := "reflect-b"
		if id == "opencode-desktop" {
			selected = "reflect_go/" + selected
		}
		value["model"], _ = json.Marshal(selected)
		if err = writeProfile(path, value, format); err != nil {
			t.Fatal(err)
		}
		if err = restoreProfileJournal(journal, "host", home); err != nil {
			t.Fatal(err)
		}
		value, _ = readProfile(path, format)
		if string(value["model"]) != `"original"` {
			t.Fatal("selected local model survived disconnect")
		}
	}
}
