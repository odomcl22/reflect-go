package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RuntimePack struct {
	Path     string `json:"path"`
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	Minimum  string `json:"minimum"`
}
type Backend struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Device      string `json:"device"`
	DeviceID    string `json:"deviceID,omitempty"`
	MemoryBytes uint64 `json:"memoryBytes"`
	FreeBytes   uint64 `json:"freeBytes"`
	Available   bool   `json:"available"`
	Reason      string `json:"reason"`
	Path        string `json:"-"`
	Minimum     string `json:"minimum"`
}
type boundedBuffer struct {
	mu    sync.Mutex
	b     []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.b = append(b.b, p...)
	if len(b.b) > b.limit {
		b.b = b.b[len(b.b)-b.limit:]
	}
	return n, nil
}
func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.Clone(b.b))
}
func platformID() string {
	name := runtime.GOOS
	if name == "darwin" {
		name = "macos"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return name + "-" + arch
}
func runtimeEnv(root, pack string) ([]string, error) {
	paths := map[string]string{}
	for _, name := range []string{"home", "cache", "tmp", "config"} {
		path, err := statePath(root, name)
		if err != nil {
			return nil, err
		}
		if err = os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
		paths[name] = path
	}
	path := pack + string(os.PathListSeparator) + systemPath()
	env := []string{"PATH=" + path, "HOME=" + paths["home"], "CFFIXED_USER_HOME=" + paths["home"], "XDG_CACHE_HOME=" + paths["cache"], "XDG_CONFIG_HOME=" + paths["config"], "LLAMA_CACHE=" + paths["cache"], "TMPDIR=" + paths["tmp"] + string(filepath.Separator), "TMP=" + paths["tmp"], "TEMP=" + paths["tmp"], "HF_HUB_OFFLINE=1", "LANG=en_US.UTF-8", "LC_ALL=C"}
	if runtime.GOOS == "linux" {
		env = append(env, "LD_LIBRARY_PATH="+pack)
	}
	for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	return env, nil
}
func runtimeBinary(pack, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(pack, name)
}
func probeRuntime(root, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtimeBinary(path, "llama-server"), args...)
	env, err := runtimeEnv(root, path)
	if err != nil {
		return "", err
	}
	cmd.Env = env
	cmd.Dir = path
	out := &boundedBuffer{limit: 64000}
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Run()
	if ctx.Err() != nil {
		err = fmt.Errorf("runtime probe timed out")
	}
	return out.String(), err
}

var deviceLine = regexp.MustCompile(`(?m)^\s*(MTL\d+|Vulkan\d+):\s*(.+?)\s*\((\d+) MiB,\s*(\d+) MiB free\)`)

func parseDevices(text, kind string) []Backend {
	var values []Backend
	for _, m := range deviceLine.FindAllStringSubmatch(text, -1) {
		if kind == "metal" && !strings.HasPrefix(m[1], "MTL") {
			continue
		}
		if kind == "vulkan" && !strings.HasPrefix(m[1], "Vulkan") {
			continue
		}
		total, _ := strconv.ParseUint(m[3], 10, 64)
		free, _ := strconv.ParseUint(m[4], 10, 64)
		if total == 0 || total > 1<<30 {
			continue
		}
		values = append(values, Backend{Kind: kind, DeviceID: m[1], Device: m[2], MemoryBytes: total << 20, FreeBytes: free << 20, Available: true})
	}
	return values
}
func chooseBackend(root, requested string, cpu bool) Backend {
	var packs map[string]RuntimePack
	if err := readJSON(filepath.Join(root, "runtime-targets.json"), &packs); err != nil {
		return Backend{Reason: "Cannot read packaged runtime inventory: " + err.Error()}
	}
	if cpu {
		requested = "cpu"
	}
	if requested == "" {
		requested = "auto"
	}
	platform := platformID()
	gpuKind := "vulkan"
	if runtime.GOOS == "darwin" {
		gpuKind = "metal"
	}
	attempt := func(kind string) Backend {
		id := platform + "-" + kind
		if kind == "cpu" && runtime.GOOS == "darwin" {
			id = platform + "-metal"
		}
		pack, ok := packs[id]
		if !ok {
			return Backend{Kind: kind, Reason: "No " + kind + " runtime is packaged for " + platform}
		}
		path, err := safePath(root, filepath.FromSlash(pack.Path))
		if err != nil {
			return Backend{Kind: kind, Reason: err.Error()}
		}
		b := Backend{ID: id, Kind: kind, Path: path, Device: "CPU", Minimum: pack.Minimum}
		if !contained(runtimeBinary(path, "llama-server"), root) {
			b.Reason = "Runtime file is missing or leaves the phase folder"
			return b
		}
		args := []string{"--list-devices"}
		if kind == "cpu" {
			args = []string{"--version"}
		}
		output, err := probeRuntime(root, path, args...)
		if err != nil {
			b.Reason = fmt.Sprintf("%s runtime cannot start (%v). Requires %s; check drivers, libraries and executable storage. %s", kind, err, pack.Minimum, strings.TrimSpace(output))
			return b
		}
		if kind == "cpu" {
			b.Available = true
			return b
		}
		devices := parseDevices(output, kind)
		if len(devices) == 0 {
			b.Reason = "No usable " + kind + " device was reported by the packaged runtime"
			return b
		}
		best := devices[0]
		for _, d := range devices[1:] {
			if d.FreeBytes > best.FreeBytes {
				best = d
			}
		}
		best.ID = id
		best.Path = path
		best.Minimum = pack.Minimum
		return best
	}
	if requested != "auto" {
		if requested != "cpu" && requested != gpuKind {
			return Backend{Reason: "Backend " + requested + " is unavailable on this platform"}
		}
		return attempt(requested)
	}
	accelerated := attempt(gpuKind)
	if accelerated.Available {
		return accelerated
	}
	fallback := attempt("cpu")
	fallback.Reason = "CPU fallback: " + accelerated.Reason
	if !fallback.Available {
		fallback.Reason += ". CPU runtime also failed its launch probe; see doctor and the platform requirements."
	}
	return fallback
}
func backendForModel(root string, b Backend, m Model, requested string, cpu bool) (Backend, error) {
	if !b.Available {
		return b, fmt.Errorf("%s", b.Reason)
	}
	if b.Kind == "vulkan" || b.Kind == "metal" {
		budget := b.MemoryBytes * 8 / 10
		if free := b.FreeBytes * 9 / 10; free < budget {
			budget = free
		}
		if m.EstimatedBytes > budget {
			if requested != "" && requested != "auto" && !cpu {
				return b, fmt.Errorf("selected model/context exceeds the conservative GPU memory budget; lower context or choose CPU")
			}
			fallback := chooseBackend(root, "cpu", true)
			fallback.Reason = "CPU fallback: selected model/context exceeds the conservative GPU memory budget"
			if !fallback.Available {
				return fallback, fmt.Errorf("CPU fallback is unavailable")
			}
			return fallback, nil
		}
	}
	return b, nil
}
