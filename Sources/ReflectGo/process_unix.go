//go:build darwin || linux

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func systemPath() string                      { return "/usr/bin:/bin:/usr/sbin:/sbin" }
func setupProcessTree() error                 { return nil }
func replaceFile(source, target string) error { return os.Rename(source, target) }
func lockNamed(root, name string) (*os.File, error) {
	path, err := statePath(root, name)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("a provider is already running from this folder")
	}
	return os.NewFile(uintptr(fd), path), nil
}
func runRuntime(path string, args, env []string, dir string) error {
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(path, append([]string{path}, args...), env)
}
func checkPort(port int) error {
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable; stop the other provider or choose another port", port)
	}
	return l.Close()
}
func openBrowser(url string) error {
	if filepath.Separator != '/' {
		return fmt.Errorf("unsupported platform")
	}
	return platformOpenBrowser(url)
}

func lockProvider(root string) (*os.File, error) {
	f, err := lockNamed(root, "provider.lock")
	if err == nil {
		_, _, e := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFD, 0)
		if e != 0 {
			f.Close()
			return nil, e
		}
	}
	return f, err
}

func hideEngineWindow(cmd *exec.Cmd) {}
