package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestUISessionHandshakeRequiresOwnedBrowserlessLaunch(t *testing.T) {
	session := UISession{Base: "http://127.0.0.1:18879", Token: strings.Repeat("a", 64)}
	for _, tc := range []struct {
		name, owned, noBrowser string
		handshake              bool
	}{
		{"normal browser launch", "", "", false},
		{"normal browserless launch", "", "1", false},
		{"owned browser launch", "1", "", false},
		{"owned browserless launch", "1", "1", true},
		{"owned flag requires exact value", "true", "1", false},
		{"browserless flag requires exact value", "1", "true", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REFLECT_GO_UI_OWNED", tc.owned)
			t.Setenv("REFLECT_GO_NO_BROWSER", tc.noBrowser)
			var stdout, stderr bytes.Buffer
			if err := announceUISession(session, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			if tc.handshake {
				if strings.Count(stdout.String(), "\n") != 1 {
					t.Fatal("native stdout must contain exactly one JSON line")
				}
				var got UISession
				decoder := json.NewDecoder(&stdout)
				if err := decoder.Decode(&got); err != nil {
					t.Fatalf("native handshake is not JSON: %v", err)
				}
				if got != session {
					t.Fatal("native handshake did not carry the ready session")
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					t.Fatal("native stdout contains extra output")
				}
			} else {
				if strings.Contains(stdout.String()+stderr.String(), session.Token) {
					t.Fatal("ordinary launcher output exposed a session credential")
				}
				if tc.owned == "1" && stdout.Len() != 0 {
					t.Fatal("owned informational output must use stderr")
				}
				if !strings.Contains(stdout.String()+stderr.String(), session.Base) {
					t.Fatal("ordinary launcher lost its local base URL")
				}
			}
			if strings.Contains(stderr.String(), session.Token) {
				t.Fatal("stderr must never expose the session credential")
			}
		})
	}
}

type failedHandshakeWriter struct{}

func (failedHandshakeWriter) Write([]byte) (int, error) {
	return 0, errors.New("closed native output pipe")
}

func TestUISessionHandshakeReportsClosedOutputPipe(t *testing.T) {
	t.Setenv("REFLECT_GO_UI_OWNED", "1")
	t.Setenv("REFLECT_GO_NO_BROWSER", "1")
	err := announceUISession(UISession{Base: "http://127.0.0.1:18879", Token: strings.Repeat("b", 64)}, failedHandshakeWriter{}, io.Discard)
	if err == nil {
		t.Fatal("failed native handshake was silently accepted")
	}
}

func TestOwnedUIRejectsExistingSessionInsteadOfReopening(t *testing.T) {
	t.Setenv("REFLECT_GO_UI_OWNED", "1")
	t.Setenv("REFLECT_GO_NO_BROWSER", "1")
	root := t.TempDir()
	owner, err := lockNamed(root, "browser-ui.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	// No server or browser should be launched, and an existing process must not
	// be adopted by a native window whose stdin cannot own that process.
	err = serveUI(root)
	if err == nil || !strings.Contains(err.Error(), "Close its existing window or launcher") {
		t.Fatalf("owned duplicate did not return an actionable ownership error: %v", err)
	}
}
