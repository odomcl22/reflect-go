package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type clientRoundTrip func(*http.Request) (*http.Response, error)

func (f clientRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func clientArchive(t *testing.T, name string, kind byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	data := []byte("native client fixture")
	h := &tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: kind}
	if kind != tar.TypeReg {
		h.Size = 0
		h.Linkname = "../../outside"
		data = nil
	}
	if e := tw.WriteHeader(h); e != nil {
		t.Fatal(e)
	}
	if _, e := tw.Write(data); e != nil {
		t.Fatal(e)
	}
	if e := tw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestClaudeInstallerIntegrityAndNoRebaseline(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "corrupt"}[corrupt], func(t *testing.T) {
			root, _ := productFixture(t)
			index := filepath.Join(root, "harnesses/installed.json")
			os.MkdirAll(filepath.Dir(index), 0700)
			original := []byte("{}\n")
			os.WriteFile(index, original, 0600)
			sha, _ := hashFile(index)
			unrelated := strings.Repeat("a", 64)
			writeJSON(filepath.Join(root, "manifest.json"), map[string]string{"harnesses/installed.json": sha, "unrelated.txt": unrelated})
			binary := "claude"
			if strings.HasPrefix(platformID(), "windows") {
				binary += ".exe"
			}
			archive := clientArchive(t, "package/"+binary, tar.TypeReg)
			digest := sha512.Sum512(archive)
			integrity := "sha512-" + base64.StdEncoding.EncodeToString(digest[:])
			if corrupt {
				integrity = "sha512-invalid"
			}
			installer := newClientInstall(root)
			installer.catalog, _ = json.Marshal(map[string]any{"version": "fixture", "packs": map[string]claudePack{platformID(): {URL: "https://registry.npmjs.org/fixture.tgz", Integrity: integrity, Unpacked: 1024, License: "vendor"}}})
			installer.client = &http.Client{Transport: clientRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "registry.npmjs.org" {
					t.Fatal("unexpected download host")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(archive)), ContentLength: int64(len(archive))}, nil
			})}
			err := installer.install(context.Background())
			destination := filepath.Join(root, "harnesses/claude", platformID(), binary)
			if corrupt {
				if err == nil {
					t.Fatal("corrupt publisher digest accepted")
				}
				if _, e := os.Stat(destination); !os.IsNotExist(e) {
					t.Fatal("corrupt client was exposed")
				}
				got, _ := os.ReadFile(index)
				if !bytes.Equal(got, original) {
					t.Fatal("index changed after failed download")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, e := os.ReadFile(destination)
				if e != nil || string(got) != "native client fixture" {
					t.Fatal(e, string(got))
				}
				var manifest map[string]string
				readJSON(filepath.Join(root, "manifest.json"), &manifest)
				if manifest["unrelated.txt"] != unrelated {
					t.Fatal("unrelated file rebaselined")
				}
				actual, _ := hashFile(index)
				if manifest["harnesses/installed.json"] != actual {
					t.Fatal("new index not protected")
				}
				// A modified index must fail before any further network request or install.
				os.WriteFile(index, []byte("{\"tampered\":{}}"), 0600)
				if e := installer.install(context.Background()); e == nil {
					t.Fatal("tampered index accepted")
				}
			}
		})
	}
}
func TestClaudeArchiveBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  byte
		limit int64
	}{
		{"../outside", tar.TypeReg, 1024}, {"/package/claude", tar.TypeReg, 1024},
		{"package/link", tar.TypeSymlink, 1024}, {"package/link", tar.TypeLink, 1024},
		{"package/claude", tar.TypeReg, 1},
	} {
		t.Run(tc.name+string(tc.kind), func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "archive.tgz")
			os.WriteFile(archive, clientArchive(t, tc.name, tc.kind), 0600)
			stage := filepath.Join(root, "stage")
			os.Mkdir(stage, 0700)
			if e := extractClaudeArchive(context.Background(), archive, stage, false, tc.limit); e == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	archive := filepath.Join(root, "archive.tgz")
	os.WriteFile(archive, clientArchive(t, "package/claude", tar.TypeReg), 0600)
	if e := extractClaudeArchive(ctx, archive, root, false, 1024); e == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestMacMemoryBudget(t *testing.T) {
	for _, tc := range []struct{ ram, budget uint64 }{{8, 4}, {16, 8}, {24, 12}, {32, 24}, {48, 36}, {128, 96}} {
		if got := macMemoryBudget(tc.ram * GiB); got != tc.budget*GiB {
			t.Fatalf("%d GiB: %d", tc.ram, got)
		}
	}
}
