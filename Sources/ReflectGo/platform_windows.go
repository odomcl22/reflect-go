package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var jobHandle uintptr

func init() {
	// Keep a missing optional GPU DLL from opening a blocking OS error dialog.
	kernel.NewProc("SetErrorMode").Call(0x0001 | 0x0002 | 0x8000)
}

func systemPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32") + ";" + os.Getenv("SystemRoot")
}

type memoryStatus struct {
	Length, Load                                                                          uint32
	TotalPhys, AvailPhys, TotalPage, AvailPage, TotalVirtual, AvailVirtual, AvailExtended uint64
}

func detectHardware() (Hardware, error) {
	var mem memoryStatus
	mem.Length = uint32(unsafe.Sizeof(mem))
	ok, _, err := kernel.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&mem)))
	if ok == 0 {
		return Hardware{}, fmt.Errorf("cannot read physical memory: %v", err)
	}
	return Hardware{OS: "Windows", CPU: os.Getenv("PROCESSOR_IDENTIFIER"), Architecture: runtime.GOARCH, PhysicalMemoryBytes: mem.TotalPhys, CPUThreads: runtime.NumCPU(), ScreeningBudgetBytes: mem.TotalPhys / 2}, nil
}

// Every child inherits this job. Windows closes our non-inheritable job handle
// even on force termination, terminating the engine tree with it.
func setupProcessTree() error {
	type basic struct {
		ProcessTime, JobTime   int64
		Flags                  uint32
		MinWorking, MaxWorking uintptr
		Active                 uint32
		Affinity               uintptr
		Priority, Scheduling   uint32
	}
	type extended struct {
		Basic                                          basic
		IO                                             [6]uint64
		ProcessMemory, JobMemory, PeakProcess, PeakJob uintptr
	}
	job, _, err := kernel.NewProc("CreateJobObjectW").Call(0, 0)
	if job == 0 {
		return fmt.Errorf("cannot create engine ownership job: %v", err)
	}
	info := extended{}
	info.Basic.Flags = 0x2000
	ok, _, err := kernel.NewProc("SetInformationJobObject").Call(job, 9, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if ok == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return fmt.Errorf("cannot configure engine ownership: %v", err)
	}
	process, _, _ := kernel.NewProc("GetCurrentProcess").Call()
	ok, _, err = kernel.NewProc("AssignProcessToJobObject").Call(job, process)
	if ok == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return fmt.Errorf("host policy prevents safe process ownership: %v", err)
	}
	jobHandle = job
	return nil
}
func replaceFile(source, target string) error {
	from, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	ok, _, err := kernel.NewProc("MoveFileExW").Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), 0x1|0x8)
	if ok == 0 {
		return err
	}
	return nil
}
func lockNamed(root, name string) (*os.File, error) {
	path, err := statePath(root, name)
	if err != nil {
		return nil, err
	}
	wide, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(wide, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("a provider is already running from this folder, or its lock is unavailable: %w", err)
	}
	return os.NewFile(uintptr(handle), path), nil
}
func runRuntime(path string, args, env []string, dir string) error {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	hideEngineWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Reflect Go runtime PID: %d\n", cmd.Process.Pid)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-signals:
		cmd.Process.Kill()
		return <-done
	}
}
func checkPort(port int) error {
	l, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable; stop the other provider or choose another port", port)
	}
	return l.Close()
}
func openBrowser(url string) error {
	name, _ := syscall.UTF16PtrFromString("open")
	target, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	value, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(target)), 0, 0, 1)
	if value <= 32 {
		return fmt.Errorf("Windows could not open the browser (%d)", value)
	}
	return nil
}
func processMemory(pid int) uint64 {
	handle, err := syscall.OpenProcess(0x1000|0x10, false, uint32(pid))
	if err != nil {
		return 0
	}
	defer syscall.CloseHandle(handle)
	type counters struct {
		Size, Faults                                                                    uint32
		Peak, Working, PeakPaged, Paged, PeakNonpaged, Nonpaged, Pagefile, PeakPagefile uintptr
	}
	var c counters
	c.Size = uint32(unsafe.Sizeof(c))
	ok, _, _ := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(uintptr(handle), uintptr(unsafe.Pointer(&c)), unsafe.Sizeof(c))
	if ok == 0 {
		return 0
	}
	return uint64(c.Working)
}

func lockProvider(root string) (*os.File, error) { return lockNamed(root, "provider.lock") }

func hideEngineWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
