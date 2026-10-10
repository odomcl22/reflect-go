package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func productFixture(t *testing.T) (string, string) {
	t.Helper()
	collection := t.TempDir()
	root := filepath.Join(collection, "Phase")
	os.MkdirAll(filepath.Join(root, "config"), 0700)
	data, _ := json.Marshal(Settings{Context: 2048, MaxTokens: 256, Temperature: .2, ModelsDirectory: "../Models", ProviderContext: 8192})
	os.WriteFile(filepath.Join(root, "config", "portableai.json"), data, 0600)
	models := filepath.Join(collection, "Models")
	os.MkdirAll(models, 0700)
	return root, models
}
func fakeGGUF() []byte {
	data := make([]byte, 4096)
	copy(data, "GGUF")
	binary.LittleEndian.PutUint32(data[4:], 3)
	binary.LittleEndian.PutUint64(data[8:], 1)
	binary.LittleEndian.PutUint64(data[16:], 1)
	return data
}
func TestDownloadResumeAndIntegrity(t *testing.T) {
	root, dir := productFixture(t)
	data := fakeGGUF()
	sum := sha256.Sum256(data)
	m := CatalogModel{ID: "test", Filename: "test.gguf", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	os.WriteFile(filepath.Join(dir, "test.gguf.partial"), data[:1024], 0600)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=1024-" {
			t.Error("resume header missing")
		}
		w.Header().Set("Content-Range", "bytes 1024-4095/4096")
		w.WriteHeader(206)
		w.Write(data[1024:])
	}))
	defer server.Close()
	m.URL = server.URL
	d := newDownloads(root)
	d.client = server.Client()
	if e := d.download(context.Background(), m); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "test.gguf"))
	if string(got) != string(data) {
		t.Fatal("download corrupted")
	}
	if e := d.download(context.Background(), m); e == nil {
		t.Fatal("overwrote installed model")
	}
}
func TestDownloadRejectsCorruption(t *testing.T) {
	root, dir := productFixture(t)
	data := fakeGGUF()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer server.Close()
	m := CatalogModel{ID: "test", Filename: "bad.gguf", Size: int64(len(data)), SHA256: strings.Repeat("0", 64), URL: server.URL}
	d := newDownloads(root)
	d.client = server.Client()
	if e := d.download(context.Background(), m); e == nil {
		t.Fatal("accepted invalid hash")
	}
	if _, e := os.Stat(filepath.Join(dir, "bad.gguf")); !os.IsNotExist(e) {
		t.Fatal("exposed damaged model")
	}
}
func TestImportAndWorkspaceBoundaries(t *testing.T) {
	root, dir := productFixture(t)
	if _, e := resolveProject(root, root); e == nil {
		t.Fatal("runtime accepted as workspace")
	}
	workspace, e := resolveProject(root, "")
	if e != nil || filepath.Base(workspace) != "Workspace" {
		t.Fatal(workspace, e)
	}
	if e = importModel(root, "../escape.gguf", strings.NewReader("GGUF"), 4); e == nil {
		t.Fatal("path traversal accepted")
	}
	data := fakeGGUF()
	if e = importModel(root, "test.gguf", strings.NewReader(string(data)), int64(len(data))); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, "test.gguf")); e != nil {
		t.Fatal(e)
	}
}
func TestCodexCatalogArgumentsBeforeTask(t *testing.T) {
	root, _ := productFixture(t)
	options := []string{"exec", "--", "fix this"}
	env := []string{}
	c := Connection{Gateway: true, ID: "one", Context: 8192}
	if err := configureCatalog(root, "codex", c, []ClientModel{{ID: "one", Name: "First"}, {ID: "two", Name: "Second"}}, &options, &env); err != nil {
		t.Fatal(err)
	}
	if options[len(options)-2] != "--" || options[len(options)-1] != "fix this" {
		t.Fatal(options)
	}
	var catalog struct{ Models []map[string]any }
	p, _ := statePath(root, "codex-models.json")
	if e := readJSON(p, &catalog); e != nil || len(catalog.Models) != 2 {
		t.Fatal(e, catalog)
	}
}

func TestOpenCodeCompactionFitsSmallContext(t *testing.T) {
	root, _ := productFixture(t)
	config := map[string]any{"provider": map[string]any{"reflect_go": map[string]any{}}}
	data, _ := json.Marshal(config)
	env := []string{"OPENCODE_CONFIG_CONTENT=" + string(data)}
	options := []string{}
	if err := configureCatalog(root, "opencode", Connection{Gateway: true, ID: "one", Context: 8192}, []ClientModel{{ID: "one", Name: "First"}}, &options, &env); err != nil {
		t.Fatal(err)
	}
	json.Unmarshal([]byte(strings.TrimPrefix(env[0], "OPENCODE_CONFIG_CONTENT=")), &config)
	if config["compaction"].(map[string]any)["reserved"].(float64) != 2048 {
		t.Fatal(config)
	}
}
func TestRecommendationRespectsReportedAvailableMemory(t *testing.T) {
	root, dir := productFixture(t)
	_ = root
	path := filepath.Join(dir, "candidate.gguf")
	os.WriteFile(path, fakeGGUF(), 0600)
	inv := Inventory{Hardware: Hardware{ScreeningBudgetBytes: 16 * GiB, AvailableMemoryBytes: GiB}, ModelsPath: dir, Models: []Model{{Name: "candidate.gguf", Path: path, Bytes: 4096}}}
	if got := recommend(inv, "chat"); got.Model != "" {
		t.Fatal("recommended a model above available memory", got)
	}
}

func TestChatExplicitCancelReleasesProvider(t *testing.T) {
	root, _ := productFixture(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	provider, _ := statePath(root, "provider.json")
	if err := writeJSON(provider, Connection{Base: upstream.URL, Context: 16384}); err != nil {
		t.Fatal(err)
	}
	key, _ := statePath(root, "provider-key")
	os.WriteFile(key, []byte(strings.Repeat("a", 64)), 0600)
	manager := newManager(root)
	manager.status.Ready = true
	downloads := newDownloads(root)
	defer downloads.close()
	mux := http.NewServeMux()
	productRoutes(mux, root, manager, downloads)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := httptest.NewRequest("POST", "/api/chat", strings.NewReader(`{"model":"test","messages":[{"role":"user","content":"Hello"}]}`))
		r.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(httptest.NewRecorder(), r)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("chat did not reach provider")
	}
	r := httptest.NewRequest("POST", "/api/chat/cancel", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not receive cancellation")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("chat handler did not exit")
	}
}
