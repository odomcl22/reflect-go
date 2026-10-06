package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Only the explicit acceptance runner enables this. Production connectors keep
// interactive approvals; this fixture test permits its bounded edit/test task.
func TestPortableHarnessWorkflow(t *testing.T) {
	root := os.Getenv("REFLECT_GO_TEST_BUNDLE")
	if root == "" {
		t.Skip("requires the explicit real-model acceptance runner")
	}
	name := os.Getenv("REFLECT_GO_TEST_CLIENT")
	project := os.Getenv("REFLECT_GO_TEST_PROJECT")
	if !validClient(name) || project == "" {
		t.Fatal("invalid fixture configuration")
	}
	target := filepath.Join(project, "roundtrip.txt")
	os.Remove(target)
	task := "Create roundtrip.txt in the current project with exactly REFLECT-PORTABLE-OK followed by a newline. Make the actual file write using a tool, then run cat roundtrip.txt using your shell tool to verify it. Do not only describe the file."
	if name == "codex" {
		task = "Call exec_command to run this exact command in the current project: printf 'REFLECT-PORTABLE-OK\\n' > roundtrip.txt . Then call exec_command to run cat roundtrip.txt. Report the output. Do not replace these tool calls with Markdown."
	}
	cmd, err := clientCommand(root, name, "portable", project, task)
	if err != nil {
		t.Fatal(err)
	}
	if name == "claude" {
		// Read/Edit are confined by the outer OS sandbox; only this test command
		// receives a shell-tool allow rule. No permission bypass mode is used.
		cmd.Args = append(cmd.Args[:len(cmd.Args)-1], "--allowedTools", "Read,Edit,Write,Bash(cat roundtrip.txt)", "--", task)
	}
	if name == "opencode" {
		cmd.Env = append(cmd.Env, `OPENCODE_PERMISSION={"*":"deny","read":"allow","edit":"allow","glob":"allow","grep":"allow","bash":{"*":"deny","cat roundtrip.txt":"allow"}}`)
	}
	if name != "codex" {
		profile := os.Getenv("REFLECT_GO_TEST_PROFILE")
		if profile == "" {
			t.Fatal("outer sandbox required")
		}
		wrapped := exec.Command("/usr/bin/sandbox-exec", append([]string{"-p", profile, cmd.Path}, cmd.Args[1:]...)...)
		wrapped.Env = cmd.Env
		wrapped.Dir = cmd.Dir
		wrapped.Stdin = cmd.Stdin
		wrapped.Stdout = cmd.Stdout
		wrapped.Stderr = cmd.Stderr
		cmd = wrapped
	}
	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}
	// Read actual bytes independently of the assistant's claimed result.
	data, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(data, []byte("REFLECT-PORTABLE-OK\n")) {
		t.Fatalf("file roundtrip failed: %q %v", data, err)
	}
	record, _ := json.Marshal(map[string]any{"client": name, "tests_passed": true, "output": string(data)})
	t.Log(string(record))
}
