package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func shellQuote(text string) string { return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'" }
func launchTerminal(root, client, project, task string) error {
	self, _ := os.Executable()
	path, err := statePath(root, "open-"+client+".command")
	if err != nil {
		return err
	}
	args := []string{self, "connect", client, "--project", project}
	if task != "" {
		args = append(args, "--task", task)
	}
	for i := range args {
		args[i] = shellQuote(args[i])
	}
	text := "#!/bin/sh\n" + strings.Join(args, " ") + "\nresult=$?\nprintf '\\nSession ended. Press Return to close.\\n'\nread answer\nexit $result\n"
	if err = atomicWrite(path, []byte(text), 0700); err != nil {
		return err
	}
	if err = exec.Command("/usr/bin/open", "-a", "Terminal", path).Run(); err != nil {
		return fmt.Errorf("could not open Terminal: %w", err)
	}
	return nil
}
