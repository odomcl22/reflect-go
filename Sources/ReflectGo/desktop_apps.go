package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type desktopSpec struct{ Name, Bundle, Executable string }

func desktopSpecification(id string) desktopSpec {
	switch id {
	case "claude-desktop":
		return desktopSpec{"Claude", "com.anthropic.claudefordesktop", "Claude"}
	case "codex-desktop":
		return desktopSpec{"ChatGPT", "com.openai.codex", "ChatGPT"}
	case "opencode-desktop":
		return desktopSpec{"OpenCode", "ai.opencode.desktop", "OpenCode"}
	}
	return desktopSpec{}
}
func desktopApp(id string) string {
	s := desktopSpecification(id)
	if s.Name == "" {
		return ""
	}
	home, _ := os.UserHomeDir()
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{filepath.Join("/Applications", s.Name+".app"), filepath.Join(home, "Applications", s.Name+".app")}
	case "windows":
		for _, base := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles")} {
			if base != "" {
				candidates = append(candidates, filepath.Join(base, "Programs", s.Name, s.Executable+".exe"), filepath.Join(base, s.Name, s.Executable+".exe"))
			}
		}
	case "linux":
		if id == "opencode-desktop" {
			if p, e := exec.LookPath("opencode-desktop"); e == nil {
				return p
			}
		}
	}
	for _, p := range candidates {
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	return ""
}
func desktopRunning(id string) (bool, error) {
	s := desktopSpecification(id)
	if s.Name == "" {
		return false, errors.New("unknown desktop app")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `if (Get-Process -Name '`+s.Executable+`' -ErrorAction SilentlyContinue) { exit 0 } else { exit 1 }`)
	} else {
		pattern := regexp.QuoteMeta(s.Name + ".app/Contents/MacOS/" + s.Executable)
		if runtime.GOOS == "linux" {
			pattern = "opencode-desktop"
		}
		cmd = exec.CommandContext(ctx, "pgrep", "-f", pattern)
	}
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var e *exec.ExitError
	if errors.As(err, &e) && e.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("could not check whether %s is running", s.Name)
}
func quitDesktop(ctx context.Context, id string) error {
	s := desktopSpecification(id)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `tell application id "`+s.Bundle+`" to quit`)
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `Get-Process -Name '`+s.Executable+`' -ErrorAction SilentlyContinue | ForEach-Object { if ($_.MainWindowHandle -ne 0) { [void]$_.CloseMainWindow() } }`)
	default:
		return errors.New("close OpenCode Desktop before changing its connection")
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("close %s and retry: %w", s.Name, err)
	}
	for {
		running, err := desktopRunning(id)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s is still open; its work was not force-closed", s.Name)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
func openDesktop(ctx context.Context, id string) error {
	app := desktopApp(id)
	if app == "" {
		return errors.New("desktop application is no longer available")
	}
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "/usr/bin/open", "-a", app).Run()
	}
	cmd := exec.Command(app)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
func desktopJournalPath(root, id string) (string, error) { return statePath(root, id+"-restore.json") }
func desktopConfigPaths(home, id string) (string, string) {
	if id == "codex-desktop" {
		return filepath.Join(home, ".codex", "config.toml"), filepath.Join(home, ".codex", "reflect-go-models.json")
	}
	base := filepath.Join(home, ".config")
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		base = v
	}
	file := filepath.Join(base, "opencode", "opencode.json")
	if _, err := os.Stat(filepath.Join(base, "opencode", "opencode.jsonc")); err == nil {
		file = filepath.Join(base, "opencode", "opencode.jsonc")
	}
	return file, ""
}
func planDesktopProfile(home, host, id, base, key string, c Connection, models []ClientModel) (profileJournal, error) {
	j := profileJournal{Home: home, Host: host}
	path, catalog := desktopConfigPaths(home, id)
	add := func(path, key string, value any, format string) error {
		e, err := newProfileEditFormat(path, key, value, format)
		if err == nil {
			// Choosing another offered model inside a harness is part of this
			// connection, so it must not prevent restoring the original model.
			if key == "model" || key == "small_model" {
				for _, model := range models {
					selected := model.ID
					if id == "opencode-desktop" {
						selected = "reflect_go/" + selected
					}
					raw, _ := json.Marshal(selected)
					e.AllowedAfter = append(e.AllowedAfter, raw)
				}
			}
			j.Edits = append(j.Edits, e)
		}
		return err
	}
	if id == "codex-desktop" {
		native := map[string]json.RawMessage{}
		cache := filepath.Join(home, ".codex", "models_cache.json")
		if data, err := os.ReadFile(cache); err == nil {
			if json.Unmarshal(data, &native) != nil {
				return j, errors.New("Codex model cache is unreadable; open ChatGPT once before connecting")
			}
		}
		var entries []json.RawMessage
		_ = json.Unmarshal(native["models"], &entries)
		combined := []any{}
		for _, entry := range entries {
			var v map[string]any
			if json.Unmarshal(entry, &v) == nil && !strings.HasPrefix(fmt.Sprint(v["slug"]), "reflect-") {
				combined = append(combined, v)
			}
		}
		for _, model := range models {
			combined = append(combined, codexDesktopModel(model, c.Context))
		}
		if err := add(catalog, "models", combined, ""); err != nil {
			return j, err
		}
		for _, v := range []struct {
			k string
			v any
		}{{"model_catalog_json", catalog}, {"openai_base_url", base}, {"model", c.ID}} {
			if err := add(path, v.k, v.v, "toml"); err != nil {
				return j, err
			}
		}
		for _, key := range []string{"model_provider", "profile"} {
			e, err := newProfileEditFormat(path, key, nil, "toml")
			if err != nil {
				return j, err
			}
			e.Delete = true
			e.After = nil
			j.Edits = append(j.Edits, e)
		}

		auth := filepath.Join(home, ".codex", "auth.json")
		if _, err := os.Stat(auth); os.IsNotExist(err) {
			for _, v := range []struct {
				k string
				v any
			}{{"auth_mode", "apikey"}, {"OPENAI_API_KEY", "reflect-go-local-desktop"}} {
				raw, _ := json.Marshal(v.v)
				j.Edits = append(j.Edits, profileEdit{Path: auth, Key: v.k, After: raw, Created: true})
			}
		} else if err != nil {
			return j, err
		}

	} else if id == "opencode-desktop" {
		if os.Getenv("OPENCODE_CONFIG_CONTENT") != "" || os.Getenv("OPENCODE_CONFIG") != "" {
			return j, errors.New("OpenCode has an environment configuration override; use OpenCode Web or remove that override before connecting Desktop")
		}
		offered := map[string]any{}
		for _, m := range models {
			offered[m.ID] = map[string]any{"name": "Reflect Go · " + m.Name, "tool_call": true, "limit": map[string]int{"context": c.Context, "output": 4096}}
		}
		provider := map[string]any{"name": "Reflect Go", "npm": "@ai-sdk/openai-compatible", "options": map[string]string{"baseURL": base + "/v1", "apiKey": key}, "models": offered}
		for _, v := range []struct {
			k string
			v any
		}{{"provider/reflect_go", provider}, {"model", "reflect_go/" + c.ID}, {"small_model", "reflect_go/" + c.ID}} {
			if err := add(path, v.k, v.v, ""); err != nil {
				return j, err
			}
		}
	} else {
		return j, errors.New("unknown desktop profile")
	}
	return j, nil
}
func restoreDesktopProfile(root, id string) error {
	if id == "claude-desktop" {
		return restoreClaudeProfile(root)
	}
	path, err := desktopJournalPath(root, id)
	if err != nil {
		return err
	}
	var j profileJournal
	if err = readJSON(path, &j); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	config, catalog := desktopConfigPaths(home, id)
	for _, e := range j.Edits {
		valid := id == "codex-desktop" && ((e.Path == catalog && e.Key == "models" && e.Format == "") || (e.Path == config && e.Format == "toml" && containsID([]string{"model_catalog_json", "openai_base_url", "model", "model_provider", "profile"}, e.Key)))
		valid = valid || (id == "codex-desktop" && e.Path == filepath.Join(home, ".codex", "auth.json") && e.Format == "" && e.Created && len(e.Before) == 0 && ((e.Key == "auth_mode" && string(e.After) == `"apikey"`) || (e.Key == "OPENAI_API_KEY" && string(e.After) == `"reflect-go-local-desktop"`)))
		valid = valid || (id == "opencode-desktop" && e.Path == config && e.Format == "" && containsID([]string{"provider/reflect_go", "model", "small_model"}, e.Key))
		if !valid {
			return errors.New("desktop restore record contains an unexpected setting; no files changed")
		}
	}
	return restoreProfileJournal(path, host, home)
}
func codexDesktopModel(m ClientModel, context int) map[string]any {
	return map[string]any{"slug": m.ID, "display_name": "Reflect Go · " + m.Name, "description": "Local model on your portable drive", "visibility": "list", "supported_in_api": true, "priority": 0, "context_window": context, "max_context_window": context, "effective_context_window_percent": 90, "auto_compact_token_limit": context * 80 / 100, "input_modalities": []string{"text"}, "base_instructions": "You are a coding assistant running a local model through Reflect Go. Follow the user's instructions and respect tool permissions.", "default_reasoning_level": nil, "supported_reasoning_levels": []any{}, "shell_type": "unified_exec", "supports_reasoning_summaries": false, "supports_reasoning_summary_parameter": false, "support_verbosity": false, "supports_parallel_tool_calls": false, "supports_search_tool": false, "experimental_supported_tools": []any{}, "additional_speed_tiers": []any{}, "service_tiers": []any{}, "truncation_policy": map[string]any{"mode": "tokens", "limit": 10000}}
}
