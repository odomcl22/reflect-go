package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type HarnessPack struct {
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	License string `json:"license"`
}

func validClient(name string) bool { return name == "codex" || name == "claude" || name == "opencode" }

func portableClient(root, name string, check bool) (string, error) {
	if !validClient(name) {
		return "", errors.New("choose Codex, Claude Code or OpenCode")
	}
	path, err := safePath(root, "harnesses/installed.json")
	if err != nil {
		return "", err
	}
	var packs map[string]HarnessPack
	if err = readJSON(path, &packs); err != nil {
		return "", errors.New("portable harness packs are missing; keep the complete Phase 4.5 folder together")
	}
	pack, ok := packs[name+"/"+platformID()]
	if !ok {
		return "", fmt.Errorf("%s portable pack is unavailable for %s", name, platformID())
	}
	ext := ""
	if strings.HasPrefix(platformID(), "windows") {
		ext = ".exe"
	}
	expected := "harnesses/" + name + "/" + platformID() + "/" + name + ext
	if pack.Path != expected || len(pack.SHA256) != 64 {
		return "", errors.New("invalid harness pack record")
	}
	path, err = safePath(root, pack.Path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s portable executable is missing", name)
	}
	if check {
		if err := verifyHarnessHelpers(root, name); err != nil {
			return "", err
		}
		hash, e := hashFile(path)
		if e != nil || hash != pack.SHA256 {
			return "", fmt.Errorf("%s pack failed its integrity check; restore this pack before launching", name)
		}
	}
	return path, nil
}

func clientEnvironment(root, name string) ([]string, error) {
	base, err := statePath(root, "harnesses/"+name+"/"+platformID())
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	dirs := map[string]string{}
	for _, part := range []string{"home", "config", "data", "cache", "state", "tmp", "codex", "claude", "appdata", "localappdata"} {
		path, e := safePath(base, part)
		if e != nil {
			return nil, e
		}
		if e = os.MkdirAll(path, 0700); e != nil {
			return nil, e
		}
		dirs[part] = path
	}
	values := map[string]string{}
	// Carry OS/terminal and development-tool discovery, never provider credentials,
	// host profiles, injected runtime hooks or caller-supplied harness flags.
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT", "LANG", "LC_ALL", "LC_CTYPE", "TERM", "COLORTERM", "TERM_PROGRAM", "SHELL", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"} {
		if value := os.Getenv(key); value != "" {
			values[key] = value
		}
	}
	for key, part := range map[string]string{"HOME": "home", "USERPROFILE": "home", "XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "XDG_STATE_HOME": "state", "TMPDIR": "tmp", "TMP": "tmp", "TEMP": "tmp", "CODEX_HOME": "codex", "CLAUDE_CONFIG_DIR": "claude", "APPDATA": "appdata", "LOCALAPPDATA": "localappdata"} {
		values[key] = dirs[part]
	}
	values["NO_PROXY"] = "127.0.0.1,localhost"
	values["DISABLE_AUTOUPDATER"] = "1"
	values["DISABLE_TELEMETRY"] = "1"
	values["DISABLE_ERROR_REPORTING"] = "1"
	values["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
	values["OPENCODE_PURE"] = "true"
	values["OPENCODE_DISABLE_AUTOUPDATE"] = "true"
	values["OPENCODE_DISABLE_MODELS_FETCH"] = "true"
	values["OPENCODE_DISABLE_PROJECT_CONFIG"] = "true"
	values["OPENCODE_DISABLE_CLAUDE_CODE"] = "true"
	values["OPENCODE_DISABLE_DEFAULT_PLUGINS"] = "true"
	values["OPENCODE_DISABLE_LSP_DOWNLOAD"] = "true"
	values["OPENCODE_CONFIG_DIR"] = dirs["config"]
	values["PATH"] = filepath.Join(root, "harnesses", "tools", platformID()) + string(os.PathListSeparator) + values["PATH"]
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+values[k])
	}
	return env, nil
}

func verifyHarnessHelpers(root, name string) error {
	record, err := safePath(root, "harnesses/helpers.json")
	if err != nil {
		return err
	}
	var hashes map[string]string
	if err = readJSON(record, &hashes); err != nil {
		return errors.New("portable helper index is missing")
	}
	prefix := "harnesses/" + name + "/" + platformID() + "/"
	tools := "harnesses/tools/" + platformID() + "/"
	found := false
	for relative, expected := range hashes {
		if !strings.HasPrefix(relative, prefix) && !strings.HasPrefix(relative, tools) {
			continue
		}
		path, e := safePath(root, relative)
		if e != nil {
			return e
		}
		actual, e := hashFile(path)
		if e != nil || actual != expected {
			return fmt.Errorf("portable helper is missing or damaged: %s", relative)
		}
		if strings.HasPrefix(relative, tools) {
			found = true
		}
	}
	if !found {
		return errors.New("portable search helper is missing")
	}
	return nil
}
