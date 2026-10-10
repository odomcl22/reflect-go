package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// Desktop startup goes through the saved provider record and the authenticated
// readiness request, not just the proxy's in-memory Connection value.
func TestProviderReadinessAtSupportedContextSizes(t *testing.T) {
	root := t.TempDir()
	keyPath, err := providerKey(root)
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+strings.TrimSpace(string(key)) || r.URL.Path != "/metrics" {
			http.Error(w, "unexpected readiness request", http.StatusUnauthorized)
			return
		}
		io.WriteString(w, "llamacpp:requests_processing 0\n")
	}))
	defer server.Close()
	path, err := statePath(root, "provider.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, context := range []int{512, 16384, 32768, 49152, 65536, 131072} {
		t.Run(fmt.Sprint(context), func(t *testing.T) {
			if err := writeJSON(path, Connection{Base: server.URL, Context: context, Gateway: true}); err != nil {
				t.Fatal(err)
			}
			response, err := localRequest(root, "GET", "/metrics", nil)
			if err != nil {
				t.Fatalf("saved provider cannot become ready: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("readiness HTTP status: %d", response.StatusCode)
			}
		})
	}
	before := calls.Load()
	for _, context := range []int{-1, 0, 511, 131073} {
		if err := writeJSON(path, Connection{Base: server.URL, Context: context}); err != nil {
			t.Fatal(err)
		}
		if response, err := localRequest(root, "GET", "/metrics", nil); err == nil {
			response.Body.Close()
			t.Fatalf("invalid context %d reached the provider", context)
		}
	}
	if calls.Load() != before {
		t.Fatal("invalid provider settings made an HTTP request")
	}
}
