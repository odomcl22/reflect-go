package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func sysctlValue(name string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	b, _ := exec.CommandContext(ctx, "/usr/sbin/sysctl", "-n", name).Output()
	return strings.TrimSpace(string(b))
}
func detectHardware() (Hardware, error) {
	ram, err := strconv.ParseUint(sysctlValue("hw.memsize"), 10, 64)
	if err != nil || ram == 0 {
		return Hardware{}, fmt.Errorf("cannot read physical memory")
	}
	return Hardware{OS: "macOS " + sysctlValue("kern.osproductversion"), CPU: sysctlValue("machdep.cpu.brand_string"), Architecture: runtime.GOARCH, PhysicalMemoryBytes: ram, CPUThreads: runtime.NumCPU(), ScreeningBudgetBytes: ram / 2}, nil
}
func processMemory(pid int) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n * 1024
}
