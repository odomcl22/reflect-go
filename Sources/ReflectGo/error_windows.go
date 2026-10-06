package main

import (
	"syscall"
	"unsafe"
)

func showLaunchError(err error) {
	console, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	if console != 0 {
		return
	}
	title, _ := syscall.UTF16PtrFromString("Reflect Go")
	text, _ := syscall.UTF16PtrFromString(err.Error() + "\n\nKeep the complete Phase 4.5 folder together. Save this message if you need help.")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
