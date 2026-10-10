package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func main() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	var process, native uint16
	kernel := syscall.NewLazyDLL("kernel32.dll")
	var ok uintptr
	probe := kernel.NewProc("IsWow64Process2")
	if probe.Find() == nil {
		ok, _, _ = probe.Call(^uintptr(0), uintptr(unsafe.Pointer(&process)), uintptr(unsafe.Pointer(&native)))
	}
	target := "windows-x64"
	if ok != 0 && native == 0xaa64 {
		target = "windows-arm64"
	}
	if ok == 0 && (os.Getenv("PROCESSOR_ARCHITEW6432") == "ARM64" || os.Getenv("PROCESSOR_ARCHITECTURE") == "ARM64") {
		target = "windows-arm64"
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(self), "bin", target, "reflectgo-window.exe"), "ui")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = cmd.Start(); err == nil {
		_ = cmd.Process.Release()
		return
	}
	text, _ := syscall.UTF16PtrFromString("Reflect Go could not start. Extract the complete ZIP first, then keep this launcher beside the bin, runtime and config folders.\n\n" + err.Error())
	title, _ := syscall.UTF16PtrFromString("Reflect Go")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
