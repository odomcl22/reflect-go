//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func prepareAdditionalHarness(cmd *exec.Cmd) { hideEngineWindow(cmd) }

func stopAdditionalHarness(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// No shell interpolation. Restrict termination to this launched PID tree.
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = os.Getenv("WINDIR")
	}
	if systemRoot != "" {
		// GUI launchers can have a minimal PATH. Resolve the OS utility directly
		// instead of silently leaving the owned server running when lookup fails.
		kill := exec.Command(filepath.Join(systemRoot, "System32", "taskkill.exe"), "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		hideEngineWindow(kill)
		if kill.Run() == nil {
			return
		}
	}
	// At least stop the process we launched if the system utility is unavailable.
	_ = cmd.Process.Kill()
}

func forceStopAdditionalHarness(cmd *exec.Cmd) { stopAdditionalHarness(cmd) }
