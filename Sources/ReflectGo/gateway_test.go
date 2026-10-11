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
	// Only a wait longer than the queue limit is reported as busy.
	previous := gatewayQueueWait
	gatewayQueueWait = 50 * time.Millisecond
	defer func() { gatewayQueueWait = previous }()
	g.lease() <- struct{}{}
	if w := request("secret", "one"); w.Code != 409 {
		t.Fatal(w.Code)
	}
	g.release()
	if len(loads) != 2 {
		t.Fatal("busy request changed models")
	}
}

// Desktop apps overlap a short background request (a conversation title) with
// the main turn. The second request must wait for its turn, not fail with 409.
func TestGatewayQueuesOverlappingRequests(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if strings.Contains(string(data), "main") {
			<-release
		}
		w.Write(data)
	}))
	defer upstream.Close()
	g := &ModelGateway{key: "secret", ctx: context.Background(), defaultModel: "one", models: map[string]Model{"one": {Name: "one.gguf"}}}
	g.load = func(context.Context, Model) (string, error) { return upstream.URL, nil }
	send := func(body string) chan *httptest.ResponseRecorder {
		out := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer secret")
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			out <- w
		}()
		return out
	}
	main := send(`{"model":"one","input":"main"}`)
	time.Sleep(50 * time.Millisecond)
	title := send(`{"model":"one","input":"title"}`)
	select {
	case w := <-title:
		t.Fatal("overlapping request did not wait for the active one:", w.Code)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for _, done := range []chan *httptest.ResponseRecorder{main, title} {
		select {
		case w := <-done:
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("queued request never completed")
		}
	}
}

func TestGatewayQueuedRequestLeavesWhenClientCancels(t *testing.T) {
	g := &ModelGateway{key: "secret", ctx: context.Background(), defaultModel: "one", models: map[string]Model{"one": {Name: "one.gguf"}}}
	g.load = func(context.Context, Model) (string, error) { t.Fatal("cancelled queued request loaded a model"); return "", nil }
	g.lease() <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"one"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer secret")
	done := make(chan struct{})
	go func() { g.ServeHTTP(httptest.NewRecorder(), r); close(done) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queued request ignored client cancellation")
	}
	g.release()
	select {
	case g.lease() <- struct{}{}:
		g.release()
	default:
		t.Fatal("lease leaked after a cancelled wait")
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
	select {
	case g.lease() <- struct{}{}:
		g.release()
	default:
		t.Fatal("lease leaked")
	}
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
