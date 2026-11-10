package main

import (
	"os"
	"path/filepath"
)

func launcherRoot(executable string) string {
	root := filepath.Dir(executable)
	support := filepath.Join(root, "Reflect Go Files")
	if info, err := os.Stat(support); err == nil && info.IsDir() {
		return support
	}
	return root
}
