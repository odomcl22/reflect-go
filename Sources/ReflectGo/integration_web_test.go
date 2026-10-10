package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstalledOpenCodeWebStartup(t *testing.T) {
	source := os.Getenv("REFLECT_GO_TEST_PACK_ROOT")
	if source == "" {
		t.Skip("set REFLECT_GO_TEST_PACK_ROOT to validate the portable OpenCode server")
	}
	root, project := t.TempDir(), t.TempDir()
	var packs map[string]HarnessPack
	var helpers map[string]string
	if err := readJSON(filepath.Join(source, "harnesses", "installed.json"), &packs); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join(source, "harnesses", "helpers.json"), &helpers); err != nil {
		t.Fatal(err)
	}
	pack, ok := packs["opencode/"+platformID()]
	if !ok {
		t.Fatal("OpenCode portable pack is absent")
	}
	link := func(relative string) {
		src, err := safePath(source, relative)
		if err != nil {
			t.Fatal(err)
		}
		dst, err := safePath(root, relative)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			t.Fatal(err)
		}
		// Immutable executables only; state/config files are newly generated.
		if err := os.Link(src, dst); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	link(pack.Path)
	selectedHelpers := map[string]string{}
	for relative, digest := range helpers {
		if strings.HasPrefix(relative, "harnesses/opencode/"+platformID()+"/") || strings.HasPrefix(relative, "harnesses/tools/"+platformID()+"/") {
			link(relative)
			selectedHelpers[relative] = digest
		}
	}
	if err := writeJSON(filepath.Join(root, "harnesses", "installed.json"), map[string]HarnessPack{"opencode/" + platformID(): pack}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "harnesses", "helpers.json"), selectedHelpers); err != nil {
		t.Fatal(err)
	}
	model := ClientModel{ID: modelID("test.gguf"), Name: "Reflect test model"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.Error(w, "No inference is performed by the startup test", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": []ClientModel{model}})
	}))
	defer provider.Close()
	c := Connection{Base: provider.URL, ID: model.ID, Context: 16384, Gateway: true}
	path, err := statePath(root, "provider.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, c); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state", "provider-key"), []byte(strings.Repeat("a", 64)), 0600); err != nil {
		t.Fatal(err)
	}
	cmd, err := clientCommandWithProfile(root, "opencode", "portable", project, "", &ClientLaunchPreference{Enabled: []string{model.ID}, DefaultModel: model.ID})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Args = []string{cmd.Path, "serve", "--hostname", "127.0.0.1", "--port", "0"}
	cmd.Env = append(cmd.Env, "OPENCODE_SERVER_USERNAME=reflect-go", "OPENCODE_SERVER_PASSWORD=test-web-password")
	cmd.Stdin = nil
	logPath := filepath.Join(root, "startup.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	prepareAdditionalHarness(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		stopAdditionalHarness(cmd)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(logPath)
		if address := openCodeWebReadyURL(string(data)); address != "" {
			client := &http.Client{Timeout: 5 * time.Second}
			req, _ := http.NewRequest("GET", address+"/", nil)
			req.SetBasicAuth("reflect-go", "test-web-password")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			page, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			response.Body.Close()
			if response.StatusCode != 200 || !strings.Contains(strings.ToLower(string(page)), "<html") {
				t.Fatalf("OpenCode did not serve its graphical UI: HTTP %d, %.300s", response.StatusCode, page)
			}
			req, _ = http.NewRequest("GET", address+"/provider", nil)
			req.SetBasicAuth("reflect-go", "test-web-password")
			req.Header.Set("x-opencode-directory", project)
			response, err = client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			catalog, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			response.Body.Close()
			if response.StatusCode != 200 || !strings.Contains(string(catalog), model.ID) {
				t.Fatalf("OpenCode did not expose the selected local model: HTTP %d, %.300s", response.StatusCode, catalog)
			}
			t.Log("Portable OpenCode served its graphical UI and selected local model catalog with isolated settings")
			return
		}
		select {
		case err := <-done:
			t.Fatalf("OpenCode exited: %v\n%s", err, data)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("OpenCode did not start:\n%s", data)
}

func TestOpenCodeBridgeRequiresSessionAndPreservesLocalScope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "reflect-go" || password != "private-bridge-password" {
			t.Error("upstream credentials not applied")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("x-opencode-directory") != "/test/workspace" {
			t.Error("browser credentials leaked or project was not supplied")
		}
		io.WriteString(w, "ready")
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	bridge := newOpenCodeWebBridge(u, "private-bridge-password", "session-test-token", "http://127.0.0.1:12345", "/test/workspace")
	request := func(method, path, body, cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:12345"+path, strings.NewReader(body))
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		bridge.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/", "", "", ""); w.Code != 401 {
		t.Fatal("unauthenticated browser reached coding server")
	}
	if w := request("POST", "/__reflect_session", "session-test-token", "", "https://attacker.example"); w.Code != 403 {
		t.Fatal("cross-origin browser established session")
	}
	w := request("POST", "/__reflect_session", "session-test-token", "", "http://127.0.0.1:12345")
	if w.Code != 204 || !strings.Contains(w.Header().Get("Set-Cookie"), "HttpOnly") || !strings.Contains(w.Header().Get("Set-Cookie"), "SameSite=Strict") {
		t.Fatal("session cookie is missing browser protection")
	}
	cookie := w.Result().Cookies()[0]
	w = request("GET", "/", "", cookie.Name+"="+cookie.Value, "")
	if w.Code != 200 || w.Body.String() != "ready" {
		t.Fatal("authenticated browser did not reach coding server")
	}
}

// Explicit opt-in keeps normal unit tests independent of installed software.
// This starts only an isolated local web server and never opens a browser.
func TestInstalledDeepSeekStartup(t *testing.T) {
	if os.Getenv("REFLECT_GO_TEST_DEEPSEEK") != "1" {
		t.Skip("set REFLECT_GO_TEST_DEEPSEEK=1 to check the installed DeepSeek Harness")
	}
	client, err := findDeepSeek()
	if err != nil {
		t.Fatal(err)
	}
	root, project := t.TempDir(), t.TempDir()
	c := Connection{Base: "http://127.0.0.1:8766", Context: 16384, ID: "reflect-test"}
	cmd, err := deepSeekCommand(root, project, c, "test-local-credential", []ClientModel{{ID: c.ID, Name: "Reflect test model"}}, client)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "startup.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	prepareAdditionalHarness(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		stopAdditionalHarness(cmd)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
		}
	}()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(logPath)
		if address := deepSeekReadyURL(string(data)); address != "" {
			client := &http.Client{Timeout: 5 * time.Second}
			response, err := client.Get(address)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			page, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			if response.StatusCode != 200 || !strings.Contains(string(page), "__DSH_BOOT__") {
				t.Fatalf("DeepSeek did not serve its real UI: HTTP %d", response.StatusCode)
			}
			request, _ := http.NewRequest("POST", address+"/api/llm.models", strings.NewReader(`{"type":"client-request","rpcId":"reflect-test","method":"llm.models","payload":{}}`))
			request.Header.Set("Content-Type", "application/json")
			response, err = client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			catalog, _ := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			response.Body.Close()
			if response.StatusCode != 200 || !strings.Contains(string(catalog), `"id":"reflect-test"`) || !strings.Contains(string(catalog), `"ok":true`) {
				t.Fatalf("DeepSeek model catalog failed: HTTP %d, %.1000s", response.StatusCode, catalog)
			}
			t.Log("Installed DeepSeek served its graphical UI and selected model catalog with an isolated Reflect Go profile")
			return
		}
		select {
		case err := <-done:
			t.Fatalf("DeepSeek exited: %v\n%s", err, string(data))
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("DeepSeek did not start:\n%s", string(data))
}
