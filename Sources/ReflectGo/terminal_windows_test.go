package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Run on Windows: reproduce a GUI-launched console with NUL stdio, repair it,
// then verify the native CLI grandchild sees console handles for all streams.
func TestWindowsClientConsoleHandles(t *testing.T) {
	result := os.Getenv("REFLECT_CONSOLE_TEST_RESULT")
	mode := os.Getenv("REFLECT_CONSOLE_TEST_MODE")
	if mode != "" {
		fail := func(err error) {
			_ = os.WriteFile(result, []byte(err.Error()), 0600)
			os.Exit(1)
		}
		if mode == "repair" {
			var state uint32
			if syscall.GetConsoleMode(syscall.Handle(os.Stdin.Fd()), &state) == nil {
				fail(fmt.Errorf("fixture did not reproduce disconnected stdin"))
			}
			if err := prepareClientTerminal(); err != nil {
				fail(err)
			}
			self, _ := os.Executable()
			child := exec.Command(self, "-test.run=^TestWindowsClientConsoleHandles$")
			child.Env = append(os.Environ(), "REFLECT_CONSOLE_TEST_MODE=verify")
			child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := child.Run(); err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
		for i, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
			var state uint32
			if err := syscall.GetConsoleMode(syscall.Handle(file.Fd()), &state); err != nil {
				fail(fmt.Errorf("Go stream %d is not a console: %w", i, err))
			}
			handle, err := syscall.GetStdHandle(-10 - i)
			if err != nil {
				fail(err)
			}
			if err = syscall.GetConsoleMode(handle, &state); err != nil {
				fail(fmt.Errorf("native stream %d is not a console: %w", i, err))
			}
		}
		_ = os.WriteFile(result, []byte("PASS: native CLI inherited console input, output and error"), 0600)
		os.Exit(0)
	}
	result = filepath.Join(t.TempDir(), "console-result.txt")
	self, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, self, "-test.run=^TestWindowsClientConsoleHandles$")
	child.Env = append(os.Environ(), "REFLECT_CONSOLE_TEST_RESULT="+result, "REFLECT_CONSOLE_TEST_MODE=repair")
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x10}
	// Deliberately leave stdio nil, matching the workstation's GUI launch.
	if err := child.Run(); err != nil {
		data, _ := os.ReadFile(result)
		t.Fatalf("console launch regression: %v %s", err, data)
	}
	data, err := os.ReadFile(result)
	if err != nil || string(data) != "PASS: native CLI inherited console input, output and error" {
		t.Fatalf("missing console verification: %v %s", err, data)
	}
}
