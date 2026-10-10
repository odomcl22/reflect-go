package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureProfile(t *testing.T, path string, values map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, values); err != nil {
		t.Fatal(err)
	}
}

func profileField(t *testing.T, path, key string) json.RawMessage {
	t.Helper()
	values, err := profileJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	return values[key]
}

func TestManagedProfileRestorePreservesUnrelatedSettings(t *testing.T) {
	dir := t.TempDir()
	path, journal := filepath.Join(dir, "app.json"), filepath.Join(dir, "restore.json")
	fixtureProfile(t, path, map[string]any{"provider": "original", "theme": "dark", "nested": map[string]any{"keep": true}})
	provider, err := newProfileEdit(path, "provider", "reflect-go")
	if err != nil {
		t.Fatal(err)
	}
	added, err := newProfileEdit(path, "temporaryKey", "local-value")
	if err != nil {
		t.Fatal(err)
	}
	j := profileJournal{Host: "fixture-host", Home: dir, Edits: []profileEdit{provider, added}}
	if err := applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	values, _ := profileJSON(path)
	values["theme"] = json.RawMessage(`"light"`)
	values["addedByApp"] = json.RawMessage(`{"value":42}`)
	if err := writeJSON(path, values); err != nil {
		t.Fatal(err)
	}
	if err := restoreProfileJournal(journal, j.Host, j.Home); err != nil {
		t.Fatal(err)
	}
	values, _ = profileJSON(path)
	for key, want := range map[string]string{"provider": `"original"`, "theme": `"light"`, "nested": `{"keep":true}`, "addedByApp": `{"value":42}`} {
		if !equalJSON(values[key], json.RawMessage(want)) {
			t.Fatalf("%s was not preserved/restored: %s", key, values[key])
		}
	}
	if _, ok := values["temporaryKey"]; ok {
		t.Fatal("newly introduced field was not removed")
	}
	if _, err := os.Stat(journal); !os.IsNotExist(err) {
		t.Fatalf("successful restore retained journal: %v", err)
	}
}

func TestManagedProfileRestorePreservesConflictingEdit(t *testing.T) {
	dir := t.TempDir()
	path, journal := filepath.Join(dir, "app.json"), filepath.Join(dir, "restore.json")
	fixtureProfile(t, path, map[string]any{"provider": "original", "other": "before"})
	provider, _ := newProfileEdit(path, "provider", "reflect-go")
	other, _ := newProfileEdit(path, "other", "reflect-go-other")
	j := profileJournal{Host: "host", Home: dir, Edits: []profileEdit{provider, other}}
	if err := applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	values, _ := profileJSON(path)
	values["provider"] = json.RawMessage(`"user-changed"`)
	if err := writeJSON(path, values); err != nil {
		t.Fatal(err)
	}
	err := restoreProfileJournal(journal, j.Host, j.Home)
	if err == nil || !strings.Contains(err.Error(), "provider changed outside Reflect Go") {
		t.Fatalf("expected conflict explanation, got %v", err)
	}
	if !equalJSON(profileField(t, path, "provider"), json.RawMessage(`"user-changed"`)) {
		t.Fatal("restore overwrote an intervening edit")
	}
	if !equalJSON(profileField(t, path, "other"), json.RawMessage(`"before"`)) {
		t.Fatal("non-conflicting field was not restored")
	}
	if _, err := os.Stat(journal); err != nil {
		t.Fatalf("conflict must retain recovery journal: %v", err)
	}
	// A later retry may safely finish after the user resolves the conflict.
	values, _ = profileJSON(path)
	values["provider"] = provider.After
	if err := writeJSON(path, values); err != nil {
		t.Fatal(err)
	}
	if err := restoreProfileJournal(journal, j.Host, j.Home); err != nil {
		t.Fatal(err)
	}
}

func TestManagedProfileRejectsForeignComputerAndHome(t *testing.T) {
	for _, mismatch := range []string{"host", "home"} {
		t.Run(mismatch, func(t *testing.T) {
			dir := t.TempDir()
			path, journal := filepath.Join(dir, "app.json"), filepath.Join(dir, "restore.json")
			fixtureProfile(t, path, map[string]any{"provider": "original"})
			edit, _ := newProfileEdit(path, "provider", "reflect-go")
			j := profileJournal{Host: "own-host", Home: dir, Edits: []profileEdit{edit}}
			if err := applyProfileJournal(journal, j); err != nil {
				t.Fatal(err)
			}
			host, home := j.Host, j.Home
			if mismatch == "host" {
				host = "different-host"
			} else {
				home = filepath.Join(dir, "different-home")
			}
			before, _ := os.ReadFile(path)
			if err := restoreProfileJournal(journal, host, home); err == nil {
				t.Fatal("foreign restore accepted")
			}
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("foreign restore changed settings")
			}
			if _, err := os.Stat(journal); err != nil {
				t.Fatal("foreign restore removed journal")
			}
		})
	}
}

func TestManagedProfileRollsBackAppliedFieldsOnConcurrentChange(t *testing.T) {
	dir := t.TempDir()
	path, journal := filepath.Join(dir, "app.json"), filepath.Join(dir, "restore.json")
	fixtureProfile(t, path, map[string]any{"first": "old", "second": "old"})
	first, _ := newProfileEdit(path, "first", "new")
	second, _ := newProfileEdit(path, "second", "new")
	fixtureProfile(t, path, map[string]any{"first": "old", "second": "changed-after-plan"})
	err := applyProfileJournal(journal, profileJournal{Host: "host", Home: dir, Edits: []profileEdit{first, second}})
	if err == nil {
		t.Fatal("connection overwrote a concurrent change")
	}
	if !equalJSON(profileField(t, path, "first"), json.RawMessage(`"old"`)) {
		t.Fatal("partial connection was not rolled back")
	}
	if !equalJSON(profileField(t, path, "second"), json.RawMessage(`"changed-after-plan"`)) {
		t.Fatal("concurrent change was not preserved")
	}
}

func TestManagedProfileRejectsMalformedJSONAndDuplicateConnection(t *testing.T) {
	dir := t.TempDir()
	path, journal := filepath.Join(dir, "app.json"), filepath.Join(dir, "restore.json")
	for _, body := range []string{"null", "[]", "{broken", `{"a":1}{"b":2}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := newProfileEdit(path, "provider", "reflect-go"); err == nil {
			t.Fatalf("accepted malformed profile %q", body)
		}
	}
	fixtureProfile(t, path, map[string]any{"provider": "before"})
	edit, _ := newProfileEdit(path, "provider", "after")
	j := profileJournal{Host: "host", Home: dir, Edits: []profileEdit{edit}}
	if err := applyProfileJournal(journal, j); err != nil {
		t.Fatal(err)
	}
	if err := applyProfileJournal(journal, j); err == nil {
		t.Fatal("duplicate connection replaced restore journal")
	}
	if err := restoreProfileJournal(journal, j.Host, j.Home); err != nil {
		t.Fatal(err)
	}
	if !equalJSON(profileField(t, path, "provider"), json.RawMessage(`"before"`)) {
		t.Fatal("duplicate attempt lost original value")
	}
}
