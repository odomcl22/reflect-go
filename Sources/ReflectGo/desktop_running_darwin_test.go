//go:build darwin

package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// A process that only mentions the app's path (a terminal loop, log viewer or
// editor) must not make a quit look unfinished.
func TestDesktopRunningIgnoresProcessesMentioningAppPath(t *testing.T) {
	s := desktopSpecification("codex-desktop")
	out, err := exec.Command("/usr/bin/osascript", "-e", `application id "`+s.Bundle+`" is running`).Output()
	if err != nil {
		t.Skip("cannot query running applications:", err)
	}
	if strings.TrimSpace(string(out)) == "true" {
		t.Skip("ChatGPT is running; this check needs it closed")
	}
	decoy := exec.Command("/bin/sh", "-c", "sleep 30; : /Applications/ChatGPT.app/Contents/MacOS/ChatGPT")
	if err := decoy.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { decoy.Process.Kill(); decoy.Wait() }()
	time.Sleep(200 * time.Millisecond)
	running, err := desktopRunning("codex-desktop")
	if err != nil {
		t.Fatal(err)
	}
	if running {
		t.Fatal("an unrelated process mentioning ChatGPT's path was treated as ChatGPT")
	}
}
