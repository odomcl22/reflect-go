package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestModelQueueVerifiesBothAndPreservesExisting(t *testing.T) {
	root, dir := productFixture(t)
	data := fakeGGUF()
	sum := sha256.Sum256(data)
	var mu sync.Mutex
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.URL.Path)
		mu.Unlock()
		w.Write(data)
	}))
	defer server.Close()
	d := newDownloads(root)
	defer d.close()
	d.client = server.Client()
	for _, id := range []string{"one", "two"} {
		d.catalog = append(d.catalog, CatalogModel{ID: id, Filename: id + ".gguf", Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), URL: server.URL + "/" + id})
	}
	if e := d.startMany([]string{"one", "one"}); e == nil {
		t.Fatal("duplicate accepted")
	}
	if e := d.startMany([]string{"one", "missing"}); e == nil {
		t.Fatal("unknown model accepted")
	}
	if e := d.startMany([]string{"one", "two"}); e != nil {
		t.Fatal(e)
	}
	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		t.Fatal("queue stalled")
	}
	s := d.snapshot()
	if s.Active || s.Stage != "complete" || s.Completed != 2 || s.TotalModels != 2 {
		t.Fatal(s)
	}
	mu.Lock()
	if len(calls) != 2 || calls[0] != "/one" || calls[1] != "/two" {
		t.Fatal(calls)
	}
	mu.Unlock()
	// A completed pair can be selected again without downloading or overwriting.
	if e := d.startMany([]string{"one", "two"}); e != nil {
		t.Fatal(e)
	}
	<-d.done
	mu.Lock()
	if len(calls) != 2 {
		t.Fatal("existing models redownloaded", calls)
	}
	mu.Unlock()
	path := filepath.Join(dir, "one.gguf")
	damaged := append([]byte(nil), data...)
	damaged[100] ^= 1
	os.WriteFile(path, damaged, 0600)
	if e := d.startMany([]string{"one", "two"}); e != nil {
		t.Fatal(e)
	}
	<-d.done
	if d.snapshot().Stage != "error" {
		t.Fatal("corrupt existing model accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(damaged) {
		t.Fatal("existing model overwritten")
	}
}

func TestModelQueueCanPause(t *testing.T) {
	root, _ := productFixture(t)
	requested := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(requested); <-r.Context().Done() }))
	defer server.Close()
	d := newDownloads(root)
	defer d.close()
	d.client = server.Client()
	d.catalog = []CatalogModel{{ID: "one", Filename: "one.gguf", Size: 4096, URL: server.URL}}
	if e := d.startMany([]string{"one"}); e != nil {
		t.Fatal(e)
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("download not started")
	}
	d.stop()
	select {
	case <-d.done:
	case <-time.After(5 * time.Second):
		t.Fatal("pause stalled")
	}
	if d.snapshot().Stage != "paused" || d.snapshot().Active {
		t.Fatal(d.snapshot())
	}
}
