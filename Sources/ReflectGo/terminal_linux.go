package main

import (
	"errors"
	"os"
	"os/exec"
)

func launchTerminal(root, client, project, task string, profile ...string) error {
	self, _ := os.Executable()
	args := []string{self, "connect", client, "--project", project, "--hold"}
	if len(profile) > 0 && profile[0] != "" {
		args = append(args, "--launch-profile", profile[0])
	}
	if task != "" {
		args = append(args, "--task", task)
	}
	for _, name := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
		if path, err := exec.LookPath(name); err == nil {
			flag := "-e"
			if name == "gnome-terminal" {
				flag = "--"
			}
			cmd := exec.Command(path, append([]string{flag}, args...)...)
			if err = cmd.Start(); err != nil {
				return err
			}
			go cmd.Wait()
			return nil
		}
	}
	return errors.New("no supported terminal found; run reflectgo connect from your terminal")
}
