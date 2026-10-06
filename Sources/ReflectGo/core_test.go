package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArguments(t *testing.T) {
	for _, args := range [][]string{{"serve", "--port", "80"}, {"serve", "--context", "0"}, {"run"}, {"serve", "--rpc", "example.invalid"}, {"serve", "--max-tokens", "20"}, {"models", "--temperature", "NaN"}, {"models", "--threads", "0"}, {"models", "--backend", "cuda"}, {"models", "extra"}} {
		if _, e := parseOptions(args); e == nil {
			t.Errorf("accepted invalid input %v", args)
		}
	}
	if o, e := parseOptions([]string{"serve", "--context", "4096", "--backend", "cpu"}); e != nil || o.Context != 4096 || o.Backend != "cpu" {
		t.Fatal(o, e)
	}
}
func TestPortablePathBoundaries(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	for _, relative := range []string{"../secret", filepath.Join(outside, "secret")} {
		if _, e := safePath(root, relative); e == nil {
			t.Fatalf("escape allowed: %s", relative)
		}
	}
	if e := os.Symlink(outside, filepath.Join(root, "state")); e != nil {
		t.Skip(e)
	}
	if _, e := statePath(root, "provider-key"); e == nil {
		t.Fatal("external state allowed")
	}
	if _, e := os.Stat(filepath.Join(outside, "provider-key")); !os.IsNotExist(e) {
		t.Fatal("outside was changed")
	}
}
func TestAtomicSettingsAndTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	v := defaultRunSettings()
	if e := writeJSON(path, v); e != nil {
		t.Fatal(e)
	}
	v.Context = 4096
	if e := writeJSON(path, v); e != nil {
		t.Fatal(e)
	}
	var got RunSettings
	if e := readJSON(path, &got); e != nil || got.Context != 4096 {
		t.Fatal(got, e)
	}
	os.WriteFile(path, []byte(`{} {}`), 0600)
	if e := readJSON(path, &got); e == nil {
		t.Fatal("trailing JSON accepted")
	}
}
func ggufFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, name)
	b := make([]byte, 2048)
	copy(b, "GGUF")
	binary.LittleEndian.PutUint32(b[4:], 3)
	binary.LittleEndian.PutUint64(b[8:], 1)
	binary.LittleEndian.PutUint64(b[16:], 1)
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	return root, path
}
func TestModelScreen(t *testing.T) {
	root, path := ggufFixture(t, "model.gguf")
	low := inspectModel(path, root, 2048, 8*GiB)
	high := inspectModel(path, root, 32768, 8*GiB)
	if !low.Eligible || high.Eligible || high.EstimatedBytes <= low.EstimatedBytes {
		t.Fatal(low, high)
	}
	for _, name := range []string{"model-00001-of-00002.gguf", "mmproj-test.gguf"} {
		r, p := ggufFixture(t, name)
		if m := inspectModel(p, r, 2048, 8*GiB); m.Eligible || m.Issue == "" {
			t.Fatal(m)
		}
	}
	os.WriteFile(path, []byte("GGUF"), 0600)
	if m := inspectModel(path, root, 2048, 8*GiB); m.Eligible {
		t.Fatal(m)
	}
}
func TestExplicitModelNeverSubstituted(t *testing.T) {
	inv := Inventory{DefaultModel: "big", Models: []Model{{Name: "small", Eligible: true}, {Name: "big", Eligible: false}}}
	if _, e := chooseModel(inv, Options{Command: "serve", Model: "big"}); e == nil {
		t.Fatal("ineligible model silently replaced")
	}
	if m, e := chooseModel(inv, Options{Command: "serve"}); e != nil || m.Name != "small" {
		t.Fatal(m, e)
	}
}
func TestBoundedKVMetadata(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint64(1<<40))
	if n := estimateKV(&b, 1); n != 524288 {
		t.Fatal(n)
	}
	if n := estimateKV(strings.NewReader(""), 10001); n != 524288 {
		t.Fatal(n)
	}
}
func TestDeviceParsing(t *testing.T) {
	input := "MTL0: Apple M1 (10922 MiB, 10922 MiB free)\n BLAS: Accelerate (0 MiB, 0 MiB free)\n Vulkan1: Test GPU (8192 MiB, 6000 MiB free)\n"
	metal := parseDevices(input, "metal")
	vulkan := parseDevices(input, "vulkan")
	if len(metal) != 1 || metal[0].DeviceID != "MTL0" || len(vulkan) != 1 || vulkan[0].FreeBytes != 6000<<20 {
		t.Fatal(metal, vulkan)
	}
	if len(parseDevices("Vulkan0: Broken (0 MiB, 0 MiB free)", "vulkan")) != 0 {
		t.Fatal("zero memory device accepted")
	}
}
func TestExplicitGPUInsufficientMemory(t *testing.T) {
	for _, kind := range []string{"vulkan", "metal"} {
		_, e := backendForModel(t.TempDir(), Backend{Available: true, Kind: kind, MemoryBytes: 4 * GiB, FreeBytes: 3 * GiB}, Model{EstimatedBytes: 4 * GiB}, kind, false)
		if e == nil {
			t.Fatalf("oversized explicit %s selection accepted", kind)
		}
	}
}

func TestMissingRuntimeReported(t *testing.T) {
	root := t.TempDir()
	writeJSON(filepath.Join(root, "runtime-targets.json"), map[string]RuntimePack{})
	b := chooseBackend(root, "auto", false)
	if b.Available || !strings.Contains(b.Reason, "CPU fallback") {
		t.Fatal(b)
	}
}
func TestProviderKeyAndStrictLoopback(t *testing.T) {
	root := t.TempDir()
	p, e := providerKey(root)
	if e != nil {
		t.Fatal(e)
	}
	key, _ := os.ReadFile(p)
	providerKey(root)
	again, _ := os.ReadFile(p)
	if !bytes.Equal(key, again) {
		t.Fatal("key changed")
	}
	c, _ := statePath(root, "provider.json")
	writeJSON(c, Connection{Base: "http://127.0.0.1:8766", Context: 4096})
	if _, _, e = connection(root); e != nil {
		t.Fatal(e)
	}
	for _, base := range []string{"http://example.com:8766", "http://127.0.0.1:8766@evil.test", "http://127.0.0.1:8766/path", "http://localhost:8766"} {
		writeJSON(c, Connection{Base: base, Context: 4096})
		if _, _, e = connection(root); e == nil {
			t.Fatal(base)
		}
	}
	os.WriteFile(p, []byte("broken"), 0600)
	if _, e = providerKey(root); e == nil {
		t.Fatal("bad key accepted")
	}
}
func TestManifestIntegrityAndEscape(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test")
	os.WriteFile(path, []byte("good"), 0600)
	sum, _ := hashFile(path)
	mf := filepath.Join(root, "manifest.json")
	writeJSON(mf, map[string]string{"test": sum})
	if e := verify(root, Settings{}, io.Discard); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, []byte("bad"), 0600)
	if e := verify(root, Settings{}, io.Discard); e == nil {
		t.Fatal("tamper accepted")
	}
	writeJSON(mf, map[string]string{"../outside": sum})
	if e := verify(root, Settings{}, io.Discard); e == nil {
		t.Fatal("escape accepted")
	}
}
func TestExclusiveLock(t *testing.T) {
	root := t.TempDir()
	a, e := lockNamed(root, "workstation.lock")
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if b, e := lockNamed(root, "workstation.lock"); e == nil {
		b.Close()
		t.Fatal("duplicate lock accepted")
	}
	a.Close()
	b, e := lockNamed(root, "workstation.lock")
	if e != nil {
		t.Fatal(e)
	}
	b.Close()
}
func TestBrowserGuard(t *testing.T) {
	key := strings.Repeat("b", 64)
	host := "127.0.0.1:23456"
	handler := guardedUI(host, key, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, c := range []struct {
		path, host, origin, auth string
		status                   int
	}{{"/", host, "", "", 204}, {"/api/status", host, "", "", 401}, {"/api/status", host, "", key, 401}, {"/api/status", host, "", "Bearer " + key, 204}, {"/api/status", "evil.test", "", "Bearer " + key, 403}, {"/api/status", host, "https://evil.test", "Bearer " + key, 403}, {"/api/status", host, "http://" + host, "Bearer " + key, 204}} {
		r := httptest.NewRequest("GET", "http://"+host+c.path, nil)
		r.Host = c.host
		r.Header.Set("Origin", c.origin)
		r.Header.Set("Authorization", c.auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != c.status {
			t.Fatalf("%+v got %d", c, w.Code)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
}
func TestClientCloudEnvironmentRemoved(t *testing.T) {
	for _, k := range []string{"OPENAI_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "HTTPS_PROXY"} {
		t.Setenv(k, "must-not-inherit")
	}
	for _, entry := range cleanClientEnv() {
		if strings.Contains(entry, "must-not-inherit") {
			t.Fatal(entry)
		}
	}
}
func TestLogRedactionAndMetrics(t *testing.T) {
	root := t.TempDir()
	p, _ := statePath(root, "workstation-engine.log")
	os.WriteFile(p, []byte(strings.Repeat("a", 64)), 0600)
	if strings.Contains(readEngineLog(root), strings.Repeat("a", 64)) {
		t.Fatal("key not redacted")
	}
	m := parseMetrics("# comment\nllamacpp:tokens_predicted_total 123\nllamacpp:tokens_predicted_seconds_total 2.5\ninvalid NaN\n")
	if m["llamacpp:tokens_predicted_total"] != 123 || m["llamacpp:tokens_predicted_seconds_total"] != 2.5 {
		t.Fatal(m)
	}
}
