package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGatewayRoutesAndRejectsBusy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { data, _ := io.ReadAll(r.Body); w.Write(data) }))
	defer upstream.Close()
	loads := []string{}
	g := &ModelGateway{key: "secret", ctx: context.Background(), defaultModel: "one", models: map[string]Model{"one": {Name: "one.gguf"}, "two": {Name: "two.gguf"}}}
	g.load = func(ctx context.Context, m Model) (string, error) {
		loads = append(loads, m.Name)
		return upstream.URL, nil
	}
	request := func(key, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q}`, id)))
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w
	}
	if w := request("wrong", "one"); w.Code != 401 || len(loads) != 0 {
		t.Fatal("unauthenticated request loaded model")
	}
	for _, id := range []string{"one", "one", "two"} {
		w := request("secret", id)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var v map[string]string
		json.Unmarshal(w.Body.Bytes(), &v)
		if v["model"] != id {
			t.Fatal("wrong model routed")
		}
	}
	if strings.Join(loads, ",") != "one.gguf,two.gguf" {
		t.Fatal(loads)
	}
	if w := request("secret", "missing"); w.Code != 400 {
		t.Fatal(w.Code)
	}
	g.lease.Lock()
	if w := request("secret", "one"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	g.lease.Unlock()
	if len(loads) != 2 {
		t.Fatal("busy request changed models")
	}
}
func TestGatewayCancellationReleasesLease(t *testing.T) {
	g := &ModelGateway{key: "secret", ctx: context.Background(), defaultModel: "one", models: map[string]Model{"one": {Name: "one.gguf"}}}
	entered := make(chan struct{})
	g.load = func(ctx context.Context, m Model) (string, error) { close(entered); <-ctx.Done(); return "", ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"one"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() { g.ServeHTTP(httptest.NewRecorder(), r); close(done) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release load")
	}
	if !g.lease.TryLock() {
		t.Fatal("lease leaked")
	}
	g.lease.Unlock()
}
func TestGatewayCatalogDoesNotLoad(t *testing.T) {
	g := &ModelGateway{key: "secret", ctx: context.Background(), models: map[string]Model{"one": {Name: "one.gguf"}}}
	g.load = func(context.Context, Model) (string, error) { t.Fatal("catalog loaded model"); return "", nil }
	r := httptest.NewRequest("GET", "/v1/models", nil)
	r.Header.Set("x-api-key", "secret")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "one.gguf") {
		t.Fatal(w.Body.String())
	}
}
