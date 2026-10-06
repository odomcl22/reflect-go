package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func detectHardware() (Hardware, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Hardware{}, err
	}
	var ram uint64
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) > 1 && parts[0] == "MemTotal:" {
			ram, _ = strconv.ParseUint(parts[1], 10, 64)
			ram *= 1024
		}
	}
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		if b, e := os.ReadFile(path); e == nil {
			limit, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
			if limit > 0 && limit < ram {
				ram = limit
			}
		}
	}
	if ram == 0 {
		return Hardware{}, fmt.Errorf("cannot read physical memory")
	}
	cpu := runtime.GOARCH
	if data, e := os.ReadFile("/proc/cpuinfo"); e == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && (strings.TrimSpace(parts[0]) == "model name" || strings.TrimSpace(parts[0]) == "Hardware") {
				cpu = strings.TrimSpace(parts[1])
				break
			}
		}
	}
	osName := "Linux"
	if b, e := os.ReadFile("/etc/os-release"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
			}
		}
	}
	return Hardware{OS: osName, CPU: cpu, Architecture: runtime.GOARCH, PhysicalMemoryBytes: ram, CPUThreads: runtime.NumCPU(), ScreeningBudgetBytes: ram / 2}, nil
}
func processMemory(pid int) uint64 {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		v := strings.Fields(line)
		if len(v) > 1 && v[0] == "VmRSS:" {
			n, _ := strconv.ParseUint(v[1], 10, 64)
			return n * 1024
		}
	}
	return 0
}
