package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func checkFreeSpace(path string, required uint64) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var free uint64
	ok, _, e := kernel.NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0)
	if ok == 0 {
		return e
	}
	if free < required {
		return fmt.Errorf("not enough space on this drive; need %.2f GiB free", float64(required)/float64(GiB))
	}
	return nil
}
