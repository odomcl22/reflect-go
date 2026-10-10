//go:build darwin || linux

package main

import (
	"os/exec"
	"syscall"
)

func prepareAdditionalHarness(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopAdditionalHarness(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// This process group was created only for a Reflect Go-owned launch.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
}

func forceStopAdditionalHarness(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
