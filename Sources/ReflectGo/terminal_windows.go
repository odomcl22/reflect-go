package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// os/exec passes NUL handles when the GUI parent has no stdio, even with
// CREATE_NEW_CONSOLE. Reopen the new console before starting an interactive CLI.
func prepareClientTerminal() error {
	in, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("cannot connect terminal input: %w", err)
	}
	out, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		in.Close()
		return fmt.Errorf("cannot connect terminal output: %w", err)
	}
	// Retain the files even if a native handle update fails partway through.
	os.Stdin, os.Stdout, os.Stderr = in, out, out
	set := kernel.NewProc("SetStdHandle")
	for _, entry := range []struct {
		id   int32
		file *os.File
	}{{-10, in}, {-11, out}, {-12, out}} {
		ok, _, e := set.Call(uintptr(uint32(entry.id)), entry.file.Fd())
		if ok == 0 {
			return fmt.Errorf("cannot attach terminal handle %d: %w", entry.id, e)
		}
	}
	// Keep both Go's streams and the native handle table attached to the console.
	return nil
}

func launchTerminal(root, client, project, task string) error {
	self := filepath.Join(root, "bin", platformID(), "reflectgo.exe")
	args := []string{"connect", client, "--project", project, "--hold"}
	if task != "" {
		args = append(args, "--task", task)
	}
	cmd := exec.Command(self, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x10}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
