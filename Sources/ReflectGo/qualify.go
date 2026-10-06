package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

func qualify(root string, s Settings, o Options) (result error) {
	report := map[string]any{"version": version, "time_utc": time.Now().UTC().Format(time.RFC3339), "platform": platformID(), "scope": "executed on this host; does not qualify other devices", "passed": false}
	path, err := statePath(root, "qualification.json")
	if err != nil {
		return err
	}
	defer func() {
		if result != nil {
			report["error"] = result.Error()
		}
		if e := writeJSON(path, report); e != nil && result == nil {
			result = e
		}
		fmt.Println("Qualification report:", path)
	}()
	fmt.Println("Checking packaged software and shared model hashes…")
	if err = verify(root, s, io.Discard); err != nil {
		return err
	}
	report["integrity"] = true
	inv, err := inventory(root, s, o)
	if err != nil {
		return err
	}
	report["hardware"] = inv.Hardware
	o.Command = "serve"
	model, err := chooseModel(inv, o)
	if err != nil {
		return err
	}
	prefs := defaultRunSettings()
	prefs.Model = model.Name
	prefs.Context = inv.Context
	prefs.Backend = o.Backend
	if o.CPU {
		prefs.Backend = "cpu"
	}
	prefs.Port = o.Port
	if o.Threads != 0 {
		prefs.Threads = o.Threads
	}
	report["model_file"] = model.Name
	report["context"] = prefs.Context
	report["requested_backend"] = prefs.Backend
	manager := newManager(root)
	defer manager.waitStopped()
	fmt.Println("Loading", model.Name)
	if err = manager.start(prefs); err != nil {
		return err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for !manager.snapshot().Ready {
		status := manager.snapshot()
		if !status.Active {
			return fmt.Errorf("engine failed: %s", status.Error)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("engine readiness timed out")
		}
		time.Sleep(250 * time.Millisecond)
	}
	c, key, err := connection(root)
	if err != nil {
		return err
	}
	response, err := localHTTP.Get(c.Base + "/v1/models")
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		return fmt.Errorf("unauthenticated request was not rejected: %d", response.StatusCode)
	}
	report["unauthenticated_status"] = 401
	for _, route := range []string{"/v1/models", "/metrics"} {
		response, err = localRequest(root, "GET", route, nil)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("%s returned %d", route, response.StatusCode)
		}
	}
	report["authenticated_api"] = true
	body, _ := json.Marshal(map[string]any{"model": "reflect-go-local", "stream": true, "max_tokens": 32, "messages": []map[string]string{{"role": "user", "content": "Reply with one friendly sentence about running locally."}}})
	req, _ := http.NewRequest("POST", c.Base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := *localHTTP
	client.Timeout = 90 * time.Second
	response, err = client.Do(req)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if err != nil {
		return err
	}
	if response.StatusCode != 200 || !bytes.Contains(data, []byte("[DONE]")) || !bytes.Contains(data, []byte("delta")) {
		return fmt.Errorf("streaming response did not complete successfully")
	}
	report["streaming"] = true
	text, err := manager.quickTest()
	if err != nil || strings.TrimSpace(text) == "" {
		return fmt.Errorf("generation test failed: %v", err)
	}
	report["response"] = text
	report["backend"] = manager.snapshot().Backend
	report["runtime_goos"] = runtime.GOOS
	if err = manager.waitStopped(); err != nil {
		return err
	}
	if err = checkPort(prefs.Port); err != nil {
		return fmt.Errorf("listener remained after shutdown: %w", err)
	}
	report["shutdown"] = true
	report["passed"] = true
	fmt.Fprintln(os.Stdout, "PASS: integrity, real model load, authentication, metrics, streaming, generation and shutdown.")
	return nil
}
