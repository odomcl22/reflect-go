package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLauncherRootSupportsCleanAndPreservedLayouts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "folder with spaces")
	os.Mkdir(root, 0700)
	executable := filepath.Join(root, "Reflect Go.exe")
	if launcherRoot(executable) != root {
		t.Fatal("flat phase layout lost")
	}
	support := filepath.Join(root, "Reflect Go Files")
	os.WriteFile(support, []byte("wrong type"), 0600)
	if launcherRoot(executable) != root {
		t.Fatal("support file treated as a directory")
	}
	os.Remove(support)
	os.Mkdir(support, 0700)
	if launcherRoot(executable) != support {
		t.Fatal("clean download layout not found")
	}
}
