package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPortablePackIntegrityAndPath(t *testing.T) {
	root := t.TempDir()
	relative := "harnesses/codex/" + platformID() + "/codex"
	if strings.HasPrefix(platformID(), "windows") {
		relative += ".exe"
	}
	path := filepath.Join(root, relative)
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("test executable"), 0700)
	helper := "harnesses/tools/" + platformID() + "/rg"
	os.MkdirAll(filepath.Dir(filepath.Join(root, helper)), 0700)
	os.WriteFile(filepath.Join(root, helper), []byte("helper"), 0700)
	helperHash, _ := hashFile(filepath.Join(root, helper))
	writeJSON(filepath.Join(root, "harnesses/helpers.json"), map[string]string{helper: helperHash})
	hash, _ := hashFile(path)
	records := map[string]HarnessPack{"codex/" + platformID(): {Path: relative, SHA256: hash, Version: "test"}}
	manifest := filepath.Join(root, "harnesses/installed.json")
	writeJSON(manifest, records)
	if p, e := portableClient(root, "codex", true); e != nil || p != path {
		t.Fatalf("%s %v", p, e)
	}
	os.WriteFile(path, []byte("damaged"), 0700)
	if _, e := portableClient(root, "codex", true); e == nil {
		t.Fatal("damaged pack accepted")
	}
	records["codex/"+platformID()] = HarnessPack{Path: "../outside", SHA256: hash}
	writeJSON(manifest, records)
	if _, e := portableClient(root, "codex", false); e == nil {
		t.Fatal("escaping pack record accepted")
	}
	if _, e := portableClient(root, "other", false); e == nil {
		t.Fatal("unknown client accepted")
	}
}

func TestPortableProfilesIgnoreHostCredentialsAndFlags(t *testing.T) {
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "OPENCODE_CONFIG_CONTENT", "NODE_OPTIONS", "AWS_PROFILE", "HTTPS_PROXY"} {
		t.Setenv(key, "host-secret-must-not-inherit")
	}
	root := t.TempDir()
	env, err := clientEnvironment(root, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{}
	for _, entry := range env {
		if strings.Contains(entry, "host-secret-must-not-inherit") {
			t.Fatal(entry)
		}
		p := strings.SplitN(entry, "=", 2)
		if _, ok := vars[p[0]]; ok {
			t.Fatal("duplicate environment key")
		}
		vars[p[0]] = p[1]
	}
	for _, key := range []string{"HOME", "USERPROFILE", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "APPDATA", "TEMP"} {
		if !contained(vars[key], root) {
			t.Fatal(key, vars[key])
		}
	}
	if vars["OPENCODE_PURE"] != "true" {
		t.Fatal("external plugins enabled")
	}
}

func TestReconnectRejectsNonlocalOrMalformedTargets(t *testing.T) {
	token := strings.Repeat("a", 64)
	if e := validateUISession(UISession{Base: "http://127.0.0.1:18879", Token: token}); e != nil {
		t.Fatal(e)
	}
	for _, base := range []string{"https://example.com:18879", "http://localhost:18879", "http://127.0.0.1:80", "http://127.0.0.1:18879/redirect", "http://user@127.0.0.1:18879"} {
		if validateUISession(UISession{Base: base, Token: token}) == nil {
			t.Fatal(base)
		}
	}
	if validateUISession(UISession{Base: "http://127.0.0.1:18879", Token: "bad"}) == nil {
		t.Fatal("bad token accepted")
	}
}
