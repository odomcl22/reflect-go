package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const version = "0.4.10-preview"
const GiB = uint64(1 << 30)
const runtimeUBatch = 128

type Settings struct {
	Context         int     `json:"context"`
	MaxTokens       int     `json:"max_tokens"`
	Temperature     float64 `json:"temperature"`
	ModelsDirectory string  `json:"models_directory"`
	ProviderContext int     `json:"provider_context"`
	ProviderModel   string  `json:"provider_model"`
}
type Options struct {
	Command, Model, Prompt, Backend   string
	EnabledIDs                        string
	Context, MaxTokens, Threads, Port int
	Temperature                       float64
	CPU, Verbose, Internal            bool
}
type Model struct {
	Name           string `json:"name"`
	ID             string `json:"id"`
	Purpose        string `json:"purpose"`
	Bytes          uint64 `json:"bytes"`
	EstimatedBytes uint64 `json:"estimatedBytes"`
	Eligible       bool   `json:"eligible"`
	Issue          string `json:"issue,omitempty"`
	Path           string `json:"-"`
}
type Hardware struct {
	OS                   string  `json:"os"`
	CPU                  string  `json:"cpu"`
	Architecture         string  `json:"architecture"`
	PhysicalMemoryBytes  uint64  `json:"physicalMemoryBytes"`
	AvailableMemoryBytes uint64  `json:"availableMemoryBytes,omitempty"`
	CPUThreads           int     `json:"cpuThreads"`
	MetalDevice          *string `json:"metalDevice,omitempty"`
	ScreeningBudgetBytes uint64  `json:"screeningBudgetBytes"`
	Backend              Backend `json:"backend"`
}
type Inventory struct {
	Hardware     Hardware `json:"hardware"`
	Context      int      `json:"context"`
	DefaultModel string   `json:"defaultModel"`
	ModelsPath   string   `json:"modelsPath"`
	Models       []Model  `json:"models"`
	Version      string   `json:"version"`
}

func rootDirectory() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(exe)
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "runtime-targets.json")); err == nil {
			return dir, nil
		}
		dir = filepath.Dir(dir)
	}
	return "", errors.New("keep the complete Reflect Go folder together; runtime-targets.json is missing")
}
func contained(path, root string) bool {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Walk before creating directories or opening writable files. Missing descendants
// are fine; links/reparse points must resolve inside their allowed parent.
func safePath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("invalid relative path")
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the portable folder")
	}
	path := root
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		if _, err := os.Lstat(path); err == nil {
			if !contained(path, root) {
				return "", fmt.Errorf("path must stay inside the portable folder: %s", relative)
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return path, nil
}
func statePath(root, name string) (string, error) {
	state, err := safePath(root, "state")
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(state, 0700); err != nil {
		return "", fmt.Errorf("drive needs a writable state folder: %w", err)
	}
	return safePath(state, name)
}
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".reflect-write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(name, path)
}
func readJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 4<<20))
	if err = d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("unexpected trailing JSON data")
	}
	return nil
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'), 0600)
}
func loadSettings(root string) (Settings, error) {
	var s Settings
	path, err := safePath(root, filepath.Join("config", "portableai.json"))
	if err != nil {
		return s, err
	}
	if err = readJSON(path, &s); err != nil {
		return s, fmt.Errorf("invalid configuration: %w", err)
	}
	if s.Context < 512 || s.Context > 131072 {
		return s, errors.New("context must be 512...131072")
	}
	if s.MaxTokens <= 0 || s.MaxTokens >= s.Context {
		return s, errors.New("max_tokens must be positive and smaller than context")
	}
	if math.IsNaN(s.Temperature) || math.IsInf(s.Temperature, 0) || s.Temperature < 0 || s.Temperature > 2 {
		return s, errors.New("temperature must be between 0 and 2")
	}
	if s.ModelsDirectory == "" {
		s.ModelsDirectory = "models"
	}
	if s.ModelsDirectory != "models" && s.ModelsDirectory != "Models" && s.ModelsDirectory != "../Models" {
		return s, errors.New("models_directory must be models, Models or ../Models")
	}
	if s.ProviderContext == 0 {
		s.ProviderContext = 16384
	}
	if s.ProviderContext < 512 || s.ProviderContext > 131072 {
		return s, errors.New("provider_context must be 512...131072")
	}
	return s, nil
}
func modelsDirectory(root string, s Settings) (string, error) {
	parent := root
	name := "models"
	if s.ModelsDirectory == "Models" {
		name = "Models"
	}
	if s.ModelsDirectory == "../Models" {
		parent = filepath.Dir(root)
		name = "Models"
	}
	return safePath(parent, name)
}

var splitModel = regexp.MustCompile(`-\d{5}-of-\d{5}\.gguf$`)

func inventory(root string, s Settings, o Options) (Inventory, error) {
	h, err := detectHardware()
	if err != nil {
		return Inventory{}, err
	}
	h.Backend = chooseBackend(root, o.Backend, o.CPU)
	if h.Backend.Kind == "metal" && h.Backend.Available {
		device := h.Backend.Device
		h.MetalDevice = &device
	}
	context := o.Context
	if context == 0 {
		context = s.ProviderContext
	}
	dir, err := modelsDirectory(root, s)
	if err != nil {
		return Inventory{}, err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return Inventory{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Inventory{}, fmt.Errorf("cannot read Models folder: %w", err)
	}
	value := Inventory{Hardware: h, Context: context, DefaultModel: s.ProviderModel, ModelsPath: dir, Models: []Model{}, Version: version}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "._") || !strings.EqualFold(filepath.Ext(name), ".gguf") {
			continue
		}
		model := inspectModel(filepath.Join(dir, name), dir, context, h.ScreeningBudgetBytes)
		value.Models = append(value.Models, model)
	}
	sort.Slice(value.Models, func(i, j int) bool {
		a, b := value.Models[i], value.Models[j]
		if a.Bytes == b.Bytes {
			return a.Name < b.Name
		}
		return a.Bytes < b.Bytes
	})
	return value, nil
}
func inspectModel(path, dir string, context int, budget uint64) Model {
	m := Model{Name: filepath.Base(path), Path: path, ID: modelID(filepath.Base(path)), Purpose: modelPurpose(filepath.Base(path))}
	var kv = uint64(524288)
	var metadata modelMemoryInfo
	err := func() error {
		if !contained(path, dir) {
			return errors.New("symlink points outside the portable folder")
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("not a regular file")
		}
		m.Bytes = uint64(info.Size())
		if splitModel.MatchString(m.Name) {
			return errors.New("split GGUF is not supported")
		}
		if strings.HasPrefix(strings.ToLower(m.Name), "mmproj") {
			return errors.New("multimodal projector is not a text model")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		var header [24]byte
		if _, err = io.ReadFull(f, header[:]); err != nil || string(header[:4]) != "GGUF" {
			return errors.New("not a GGUF model or incomplete header")
		}
		v := binary.LittleEndian.Uint32(header[4:])
		if (v != 2 && v != 3) || binary.LittleEndian.Uint64(header[8:]) == 0 || binary.LittleEndian.Uint64(header[16:]) == 0 || m.Bytes <= 1024 {
			return errors.New("invalid or empty GGUF header")
		}
		kv = estimateKV(f, binary.LittleEndian.Uint64(header[16:]), &metadata)
		if metadata.Context > 0 && uint64(context) > metadata.Context {
			return fmt.Errorf("model supports %d context tokens; choose a shorter context", metadata.Context)
		}
		if context > 32768 && !metadata.CompactKV {
			return errors.New("long desktop context requires verified attention metadata for compact KV memory")
		}
		return nil
	}()
	kvBytes := uint64(context) * kv
	if metadata.SlidingWindow > 0 {
		// Pinned llama-kv-cache-iswa.cpp: one slot, ubatch 128, SWA
		// min(base cells, window + ubatch), padded to 256 cells.
		baseCells := (uint64(context) + 255) / 256 * 256
		swaCells := (min(baseCells, metadata.SlidingWindow+runtimeUBatch) + 255) / 256 * 256
		kvBytes = baseCells*metadata.FullKV + swaCells*metadata.SlidingKV
	}
	if context > 32768 && metadata.CompactKV {
		kvBytes = (kvBytes*9 + 31) / 32
	} // Q4_0: 18 bytes per block of 32, versus FP16.
	estimate := float64(m.Bytes)*1.3 + float64(GiB) + float64(kvBytes)
	if estimate >= float64(math.MaxInt64) {
		m.EstimatedBytes = math.MaxInt64
	} else {
		m.EstimatedBytes = uint64(estimate)
	}
	if err != nil {
		m.Issue = err.Error()
	}
	m.Eligible = err == nil && m.EstimatedBytes <= budget
	return m
}

// Larger unified-memory Macs can reserve a quarter of physical RAM for the OS.
// This is a screening ceiling, not a measurement of currently free memory.
func macMemoryBudget(ram uint64) uint64 {
	if ram >= 32*GiB {
		return ram / 4 * 3
	}
	return ram / 2
}

type modelMemoryInfo struct {
	Context       uint64
	CompactKV     bool
	FullKV        uint64
	SlidingKV     uint64
	SlidingWindow uint64
}

func estimateKV(r io.Reader, count uint64, metadata ...*modelMemoryInfo) uint64 {
	const fallback = uint64(524288)
	if count > 10000 {
		return fallback
	}
	rd := &io.LimitedReader{R: r, N: 32 << 20}
	numbers := map[string]uint64{}
	arrays := map[string][]uint64{}
	arch := ""
	uintN := func(n int) (uint64, error) {
		var b [8]byte
		_, err := io.ReadFull(rd, b[:n])
		return binary.LittleEndian.Uint64(b[:]), err
	}
	text := func() (string, error) {
		n, err := uintN(8)
		if err != nil || n > uint64(rd.N) || n > 32<<20 {
			return "", errors.New("oversized GGUF string")
		}
		b := make([]byte, n)
		_, err = io.ReadFull(rd, b)
		return string(b), err
	}
	var value func(uint64, int) (uint64, error)
	value = func(t uint64, depth int) (uint64, error) {
		sizes := map[uint64]int{0: 1, 1: 1, 2: 2, 3: 2, 4: 4, 5: 4, 6: 4, 7: 1, 10: 8, 11: 8, 12: 8}
		if n, ok := sizes[t]; ok {
			return uintN(n)
		}
		if t == 8 {
			_, err := text()
			return 0, err
		}
		if t != 9 || depth > 0 {
			return 0, errors.New("unsupported GGUF metadata")
		}
		sub, err := uintN(4)
		if err != nil {
			return 0, err
		}
		length, err := uintN(8)
		if err != nil || length > 1000000 {
			return 0, errors.New("oversized array")
		}
		for i := uint64(0); i < length; i++ {
			if _, err = value(sub, depth+1); err != nil {
				return 0, err
			}
		}
		return 0, nil
	}
	for i := uint64(0); i < count; i++ {
		key, err := text()
		if err != nil {
			return fallback
		}
		t, err := uintN(4)
		if err != nil {
			return fallback
		}
		if key == "general.architecture" && t == 8 {
			arch, err = text()
			if err != nil {
				return fallback
			}
		} else if t == 9 && (strings.HasSuffix(key, ".attention.head_count_kv") || strings.HasSuffix(key, ".attention.sliding_window_pattern")) {
			sub, e := uintN(4)
			if e != nil || (sub != 4 && sub != 5 && sub != 7 && sub != 10 && sub != 11) {
				return fallback
			}
			length, e := uintN(8)
			if e != nil || length == 0 || length > 256 {
				return fallback
			}
			values := make([]uint64, length)
			for j := range values {
				values[j], e = value(sub, 1)
				if e != nil {
					return fallback
				}
			}
			arrays[key] = values
		} else {
			n, err := value(t, 0)
			if err != nil {
				return fallback
			}
			if t == 4 || t == 10 {
				numbers[key] = n
			}
		}
	}
	if len(metadata) > 0 {
		metadata[0].Context = numbers[arch+".context_length"]
	}
	if arch == "gemma4" || arch == "muse-glimmer" {
		var info modelMemoryInfo
		if !slidingAttentionProfile(arch, numbers, arrays, &info) {
			return fallback
		}
		if len(metadata) > 0 {
			info.Context = metadata[0].Context
			*metadata[0] = info
		}
		return info.FullKV + info.SlidingKV
	}
	if arch != "qwen2" && arch != "qwen3" && arch != "llama" && arch != "mistral" {
		return fallback
	}
	n := func(key string) float64 { return float64(numbers[arch+"."+key]) }
	layers, heads, kvheads, embedding := n("block_count"), n("attention.head_count"), n("attention.head_count_kv"), n("embedding_length")
	if layers < 1 || layers > 256 || heads < 1 || heads > 1024 || kvheads < 1 || kvheads > heads || embedding < 1 || embedding > 65536 {
		return fallback
	}
	k, v := n("attention.key_length"), n("attention.value_length")
	if k == 0 {
		k = embedding / heads
	}
	if v == 0 {
		v = embedding / heads
	}
	if k > 8192 || v > 8192 {
		return fallback
	}
	estimate := layers * kvheads * (k + v) * 3
	if estimate > 16<<20 {
		return fallback
	}
	if len(metadata) > 0 {
		metadata[0].CompactKV = uint64(k)%32 == 0 && uint64(v)%32 == 0
	}
	return uint64(math.Max(65536, estimate))
}

func slidingAttentionProfile(arch string, numbers map[string]uint64, arrays map[string][]uint64, info *modelMemoryInfo) bool {
	n := func(key string) uint64 { return numbers[arch+"."+key] }
	layers, heads, window := n("block_count"), n("attention.head_count"), n("attention.sliding_window")
	pattern := arrays[arch+".attention.sliding_window_pattern"]
	kvHeads := arrays[arch+".attention.head_count_kv"]
	if layers == 0 || layers > 256 || heads == 0 || heads > 1024 || window == 0 || window > 1<<20 || len(pattern) != int(layers) {
		return false
	}
	if len(kvHeads) != 0 && len(kvHeads) != int(layers) {
		return false
	}
	k, v := n("attention.key_length"), n("attention.value_length")
	sk, sv := n("attention.key_length_swa"), n("attention.value_length_swa")
	if sk == 0 {
		sk = k
	}
	if sv == 0 {
		sv = v
	}
	for _, dim := range []uint64{k, v, sk, sv} {
		if dim == 0 || dim > 8192 {
			return false
		}
	}
	var full, sliding uint64
	var widestV uint64
	counts := make([]uint64, len(pattern))
	for i := range pattern {
		count := n("attention.head_count_kv")
		if len(kvHeads) > 0 {
			count = kvHeads[i]
		}
		if count == 0 || count > heads || heads%count != 0 || pattern[i] > 1 {
			return false
		}
		counts[i] = count
		width := count * v
		if pattern[i] == 1 {
			width = count * sv
		}
		widestV = max(widestV, width)
	}
	// llama-kv-cache.cpp pads transposed V rows to the widest layer. Use
	// that upper bound even when Flash Attention can avoid the padding.
	for i, count := range counts {
		if pattern[i] == 1 {
			sliding += (count*sk + widestV) * 3
		} else {
			full += (count*k + widestV) * 3
		}
	}
	if full == 0 || sliding == 0 || full+sliding > 16<<20 {
		return false
	}
	info.FullKV, info.SlidingKV, info.SlidingWindow = full, sliding, window
	info.CompactKV = k%32 == 0 && v%32 == 0 && sk%32 == 0 && sv%32 == 0
	return true
}
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	return hex.EncodeToString(hash.Sum(nil)), err
}
func manifest(root string) (map[string]string, error) {
	var hashes map[string]string
	err := readJSON(filepath.Join(root, "manifest.json"), &hashes)
	if err == nil && len(hashes) == 0 {
		err = errors.New("empty bundle manifest")
	}
	return hashes, err
}
func verify(root string, s Settings, out io.Writer) error {
	hashes, err := manifest(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		parent, rel := root, filepath.FromSlash(name)
		if strings.HasPrefix(name, "models/") {
			parent, err = modelsDirectory(root, s)
			if err != nil {
				return err
			}
			rel = filepath.FromSlash(strings.TrimPrefix(name, "models/"))
		}
		path, err := safePath(parent, rel)
		if err != nil {
			return fmt.Errorf("integrity path rejected: %s", name)
		}
		actual, err := hashFile(path)
		if err != nil || actual != hashes[name] {
			return fmt.Errorf("integrity check failed: %s", name)
		}
		fmt.Fprintln(out, "OK "+name)
	}
	fmt.Fprintf(out, "Verified %d packaged files. This detects changes; it is not publisher authentication.\n", len(hashes))
	return nil
}
