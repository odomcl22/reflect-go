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
	"sync"
	"testing"
	"time"
)

func TestClaudeProfilePlanAndRestorePreserveHostSettings(t *testing.T) {
	home := t.TempDir()
	normal, third, meta, profile := claudeProfilePaths(home)
	fixtureProfile(t, normal, map[string]any{"deploymentMode": "consumer", "mcpServers": map[string]any{"existing": map[string]any{"command": "fixture-command"}}})
	fixtureProfile(t, third, map[string]any{"theme": "dark"})
	entries := []map[string]any{{"id": "existing-profile", "name": "Existing work", "extra": "preserve"}}
	fixtureProfile(t, meta, map[string]any{"entries": entries, "appliedId": "existing-profile", "unrelated": true})
	before := map[string][]byte{}
	for _, path := range []string{normal, third, meta} {
		before[path], _ = os.ReadFile(path)
	}
	j, err := planClaudeProfile(home, "fixture-host", "http://127.0.0.1:43210", "fixture-key")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range before {
		got, _ := os.ReadFile(path)
		if string(got) != string(want) {
			t.Fatal("planning modified host settings")
		}
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("planning created its profile")
	}
	journal := filepath.Join(home, "restore.json")
	if err := applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	var gotEntries []map[string]any
	if err := json.Unmarshal(profileField(t, meta, "entries"), &gotEntries); err != nil {
		t.Fatal(err)
	}
	if len(gotEntries) != 2 || gotEntries[0]["id"] != "existing-profile" || gotEntries[0]["extra"] != "preserve" || gotEntries[1]["id"] != claudeProfileID {
		t.Fatalf("profile plan replaced unrelated entries: %#v", gotEntries)
	}
	if !equalJSON(profileField(t, normal, "mcpServers"), json.RawMessage(`{"existing":{"command":"fixture-command"}}`)) {
		t.Fatal("existing MCP settings changed")
	}
	if !equalJSON(profileField(t, profile, "inferenceGatewayBaseUrl"), json.RawMessage(`"http://127.0.0.1:43210"`)) {
		t.Fatal("wrong local gateway configured")
	}
	if !equalJSON(profileField(t, profile, "inferenceGatewayAuthScheme"), json.RawMessage(`"bearer"`)) {
		t.Fatal("gateway auth scheme missing")
	}
	// Host preferences added after connection are outside Reflect Go's fields.
	values, _ := profileJSON(third)
	values["fontSize"] = json.RawMessage(`16`)
	if err := writeJSON(third, values); err != nil {
		t.Fatal(err)
	}
	if err := restoreProfileJournal(journal, j.Host, j.Home); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{normal, meta} {
		got, _ := profileJSON(path)
		var want map[string]json.RawMessage
		json.Unmarshal(before[path], &want)
		for key, value := range want {
			if !equalJSON(got[key], value) {
				t.Fatalf("%s field %s was not restored", filepath.Base(path), key)
			}
		}
	}
	if len(profileField(t, third, "deploymentMode")) != 0 || !equalJSON(profileField(t, third, "fontSize"), json.RawMessage(`16`)) {
		t.Fatal("restore lost unrelated host changes")
	}
	values, _ = profileJSON(profile)
	if len(values) != 0 {
		t.Fatal("Reflect Go gateway credentials survived restore")
	}
}

func TestClaudeProfilePlanRejectsExistingOwnedEntry(t *testing.T) {
	for _, existing := range []string{"profile", "endpoint", "entry", "selected", "invalid-list"} {
		t.Run(existing, func(t *testing.T) {
			home := t.TempDir()
			_, _, meta, profile := claudeProfilePaths(home)
			switch existing {
			case "profile":
				fixtureProfile(t, profile, map[string]any{"inferenceProvider": "user-provider"})
			case "endpoint":
				fixtureProfile(t, profile, map[string]any{"inferenceGatewayBaseUrl": "http://127.0.0.1:12345"})
			case "selected":
				fixtureProfile(t, meta, map[string]any{"appliedId": claudeProfileID})
			case "entry":
				fixtureProfile(t, meta, map[string]any{"entries": []any{map[string]any{"id": claudeProfileID, "name": "Already configured"}}})
			case "invalid-list":
				fixtureProfile(t, meta, map[string]any{"entries": map[string]any{"unknown": true}})
			}
			if _, err := planClaudeProfile(home, "host", "http://127.0.0.1:12345", "key"); err == nil {
				t.Fatal("existing or unrecognized profile was accepted")
			}
		})
	}
}

func TestClaudeReconnectPreservesDefaultsAddedByDesktop(t *testing.T) {
	home := t.TempDir()
	_, _, _, profile := claudeProfilePaths(home)
	journal := filepath.Join(home, "restore.json")
	cycle := func() {
		t.Helper()
		j, err := planClaudeProfile(home, "host", "http://127.0.0.1:12345", "local-test-key")
		if err != nil {
			t.Fatal(err)
		}
		if err := applyProfileJournal(journal, j); err != nil {
			t.Fatal(err)
		}
		if err := restoreProfileJournal(journal, j.Host, j.Home); err != nil {
			t.Fatal(err)
		}
	}
	cycle()
	// Observed after restarting the installed Claude desktop on disconnect.
	defaults := map[string]any{"chatTabEnabled": true, "builtinBrowserEnabled": true, "inferenceCredentialKind": "static", "inferenceGatewayApiKey": "vendor-placeholder"}
	fixtureProfile(t, profile, defaults)
	cycle()
	got, err := profileJSON(profile)
	want, _ := json.Marshal(defaults)
	actual, _ := json.Marshal(got)
	if err != nil || !equalJSON(actual, want) {
		t.Fatal("reconnect did not restore Claude's inactive profile defaults")
	}
}

type claudeUpstreamRequest struct {
	Method, Path, Host string
	Header             http.Header
	Body               map[string]json.RawMessage
}

func claudeProxyFixture(t *testing.T, modelCount int) (base string, requests func() []claudeUpstreamRequest) {
	t.Helper()
	var mu sync.Mutex
	var observed []claudeUpstreamRequest
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid forwarded JSON", 400)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			http.Error(w, "upstream key missing", 401)
			return
		}
		if r.Header.Get("Origin") != "" && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "wrong upstream origin", 403)
			return
		}
		mu.Lock()
		observed = append(observed, claudeUpstreamRequest{r.Method, r.URL.Path, r.Host, r.Header.Clone(), body})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"fixture response"}]}`)
	}))
	t.Cleanup(upstream.Close)
	models := make([]ClientModel, modelCount)
	for i := range models {
		models[i] = ClientModel{ID: "reflect-" + string(rune('a'+i)), Name: "Portable " + string(rune('A'+i))}
	}
	base, closeProxy, err := startClaudeDesktopProxy(Connection{Base: upstream.URL, Context: 16384}, "fixture-key", models)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeProxy)
	return base, func() []claudeUpstreamRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]claudeUpstreamRequest(nil), observed...)
	}
}

func claudeProxyRequest(t *testing.T, base, method, path, body string, headers map[string]string, host string) (int, []byte, http.Header) {
	t.Helper()
	r, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	if host != "" {
		r.Host = host
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, data, response.Header
}

func TestClaudeDesktopProxyAuthenticationAndCatalog(t *testing.T) {
	base, observed := claudeProxyFixture(t, 5)
	for _, auth := range []string{"", "Bearer wrong", "fixture-key"} {
		status, _, _ := claudeProxyRequest(t, base, "GET", "/v1/models", "", map[string]string{"Authorization": auth}, "")
		if status != 401 {
			t.Fatalf("bad auth %q returned %d", auth, status)
		}
	}
	for _, check := range []struct {
		headers map[string]string
		host    string
	}{
		{map[string]string{"Authorization": "Bearer fixture-key"}, "attacker.invalid"},
		{map[string]string{"Authorization": "Bearer fixture-key", "Origin": "https://attacker.invalid"}, ""},
	} {
		status, _, _ := claudeProxyRequest(t, base, "GET", "/v1/models", "", check.headers, check.host)
		if status != 403 {
			t.Fatalf("foreign host/origin returned %d", status)
		}
	}
	status, data, headers := claudeProxyRequest(t, base, "GET", "/v1/models", "", map[string]string{"Authorization": "Bearer fixture-key"}, "")
	if status != 200 || headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("catalog request failed: %d %s", status, data)
	}
	var catalog struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"display_name"`
			Context int    `json:"max_tokens"`
		}
		First string `json:"first_id"`
		Last  string `json:"last_id"`
		More  bool   `json:"has_more"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Data) != 5 || catalog.First != claudeSlots[0].ID || catalog.Last != claudeSlots[4].ID || catalog.More {
		t.Fatalf("incorrect offered catalog: %s", data)
	}
	for i, entry := range catalog.Data {
		if entry.ID != claudeSlots[i].ID || entry.Name != "Reflect Go · Portable "+string(rune('A'+i)) || entry.Context != 16384 {
			t.Fatalf("slot %d: %#v", i, entry)
		}
	}
	if len(observed()) != 0 {
		t.Fatal("catalog or rejected auth reached inference upstream")
	}
}

func TestClaudeDesktopProxyMapsSlotsAndSanitizesUpstream(t *testing.T) {
	base, observed := claudeProxyFixture(t, 2)
	for _, pair := range []struct{ requested, want, path string }{{claudeSlots[1].ID, "reflect-b", "/v1/messages"}, {"reflect-a", "reflect-a", "/v1/messages/count_tokens"}} {
		body := `{"model":"` + pair.requested + `","messages":[{"role":"user","content":"fixture prompt"}],"tools":[{"name":"read_file","input_schema":{"type":"object"}},{"type":"custom","name":"fixture_tool"}]}`
		status, data, _ := claudeProxyRequest(t, base, "POST", pair.path, body, map[string]string{"Authorization": "Bearer fixture-key", "Origin": base, "Cookie": "private=session", "Proxy-Authorization": "Bearer private-proxy", "x-api-key": "unrelated-key"}, "")
		if status != 200 {
			t.Fatalf("mapped request failed: %d %s", status, data)
		}
		got := observed()[len(observed())-1]
		var model string
		json.Unmarshal(got.Body["model"], &model)
		if model != pair.want || got.Path != pair.path {
			t.Fatalf("wrong route/model: %#v", got)
		}
		if got.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Fatal("upstream authentication missing")
		}
		for _, header := range []string{"Cookie", "Proxy-Authorization", "x-api-key"} {
			if got.Header.Get(header) != "" {
				t.Fatalf("private %s forwarded", header)
			}
		}
		if !strings.Contains(string(got.Body["messages"]), "fixture prompt") || !strings.Contains(string(got.Body["tools"]), "read_file") {
			t.Fatal("client messages/tools lost")
		}
		proxyURL, _ := url.Parse(base)
		if got.Host == proxyURL.Host {
			t.Fatal("upstream Host was not rewritten")
		}
	}
}

func TestClaudeDesktopProxyRejectsUnsupportedRequests(t *testing.T) {
	base, observed := claudeProxyFixture(t, 1)
	for _, tc := range []struct {
		name, method, path, body string
		want                     int
	}{
		{"unknown model", "POST", "/v1/messages", `{"model":"not-offered"}`, 400},
		{"unoffered slot", "POST", "/v1/messages", `{"model":"` + claudeSlots[4].ID + `"}`, 400},
		{"server tool", "POST", "/v1/messages", `{"model":"reflect-a","tools":[{"type":"web_search_20250305","name":"web_search"}]}`, 400},
		{"invalid JSON", "POST", "/v1/messages", `{`, 400},
		{"trailing JSON", "POST", "/v1/messages", `{"model":"reflect-a"}{}`, 400},
		{"non-string model", "POST", "/v1/messages", `{"model":42}`, 400},
		{"unsupported endpoint", "POST", "/v1/files", `{}`, 404},
		{"unsupported method", "DELETE", "/v1/messages", `{}`, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, data, _ := claudeProxyRequest(t, base, tc.method, tc.path, tc.body, map[string]string{"Authorization": "Bearer fixture-key"}, "")
			if status != tc.want {
				t.Fatalf("got %d want %d: %s", status, tc.want, data)
			}
		})
	}
	if len(observed()) != 0 {
		t.Fatal("unsupported request reached upstream")
	}
}

func TestClaudeDesktopProxyRejectsEmptyAndOversizeCatalog(t *testing.T) {
	for _, count := range []int{0, 6} {
		_, closeProxy, err := startClaudeDesktopProxy(Connection{Base: "http://127.0.0.1:12345"}, "key", make([]ClientModel, count))
		if closeProxy != nil {
			closeProxy()
		}
		if err == nil {
			t.Fatalf("accepted %d offered models", count)
		}
	}
}
