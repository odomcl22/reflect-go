//go:build darwin || linux

package main

import (
	"fmt"
	"syscall"
)

func checkFreeSpace(path string, required uint64) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return err
	}
	if uint64(s.Bavail)*uint64(s.Bsize) < required {
		return fmt.Errorf("not enough space on this drive; need %.2f GiB free", float64(required)/float64(GiB))
	}
	return nil
}
