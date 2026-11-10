package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func attentionFixture(t *testing.T, arch string, heads, pattern []uint64, badArray bool) string {
	t.Helper()
	var metadata bytes.Buffer
	count := uint64(0)
	text := func(s string) { binary.Write(&metadata, binary.LittleEndian, uint64(len(s))); metadata.WriteString(s) }
	key := func(s string, kind uint32) { count++; text(s); binary.Write(&metadata, binary.LittleEndian, kind) }
	key("general.architecture", 8)
	text(arch)
	number := func(k string, v uint32) { key(arch+"."+k, 4); binary.Write(&metadata, binary.LittleEndian, v) }
	number("block_count", uint32(len(heads)))
	number("attention.head_count", 32)
	number("context_length", 131072)
	window, dim := uint32(2048), uint32(128)
	if arch == "gemma4" {
		window = 1024
		dim = 512
		number("attention.key_length_swa", 256)
		number("attention.value_length_swa", 256)
	}
	number("attention.sliding_window", window)
	number("attention.key_length", dim)
	number("attention.value_length", dim)
	key(arch+".attention.head_count_kv", 9)
	binary.Write(&metadata, binary.LittleEndian, uint32(4))
	binary.Write(&metadata, binary.LittleEndian, uint64(len(heads)))
	for _, v := range heads {
		binary.Write(&metadata, binary.LittleEndian, uint32(v))
	}
	key(arch+".attention.sliding_window_pattern", 9)
	binary.Write(&metadata, binary.LittleEndian, uint32(7))
	if badArray {
		pattern = pattern[:len(pattern)-1]
	}
	binary.Write(&metadata, binary.LittleEndian, uint64(len(pattern)))
	for _, v := range pattern {
		metadata.WriteByte(byte(v))
	}
	var file bytes.Buffer
	file.WriteString("GGUF")
	binary.Write(&file, binary.LittleEndian, uint32(3))
	binary.Write(&file, binary.LittleEndian, uint64(1))
	binary.Write(&file, binary.LittleEndian, count)
	file.Write(metadata.Bytes())
	data := make([]byte, 4096)
	copy(data, file.Bytes())
	path := filepath.Join(t.TempDir(), arch+".gguf")
	os.WriteFile(path, data, 0600)
	return path
}
func TestSlidingWindowEstimatesMatchPinnedSingleSlotCacheAllocation(t *testing.T) {
	for _, arch := range []string{"gemma4", "muse-glimmer"} {
		t.Run(arch, func(t *testing.T) {
			layers := 60
			if arch == "muse-glimmer" {
				layers = 52
			}
			heads, pattern := make([]uint64, layers), make([]uint64, layers)
			for i := range heads {
				if arch == "gemma4" {
					heads[i] = 16
					if (i+1)%6 == 0 {
						heads[i] = 4
					} else {
						pattern[i] = 1
					}
				} else {
					heads[i] = 2
					if (i+1)%4 != 0 {
						pattern[i] = 1
					}
				}
			}
			path := attentionFixture(t, arch, heads, pattern, false)
			for _, context := range []int{2048, 16384, 49152} {
				model := inspectModel(path, filepath.Dir(path), context, 48*GiB)
				var cache uint64
				// Runtime source reference values: 10 global/50 local Gemma layers;
				// 13 global/39 local Muse layers. FP16 includes the existing 50% margin.
				if arch == "gemma4" {
					cache = uint64(context)*184320 + 1280*1228800
				} else {
					swa := min(uint64(context), uint64(2304))
					cache = uint64(context)*19968 + swa*59904
				}
				if context > 32768 {
					cache = (cache*9 + 31) / 32
				}
				want := uint64(4096*1.3 + float64(GiB) + float64(cache))
				if model.Issue != "" || model.EstimatedBytes != want {
					t.Fatalf("%s context %d: got %+v; want %d", arch, context, model, want)
				}
			}
			malformed := attentionFixture(t, arch, heads, pattern, true)
			if model := inspectModel(malformed, filepath.Dir(malformed), 49152, 128*GiB); !strings.Contains(model.Issue, "attention metadata") {
				t.Fatal("unverified long context accepted", model)
			}
		})
	}
}
func TestRealLargeModelProfilesWhenProvided(t *testing.T) {
	dir := os.Getenv("REFLECT_GO_QA_MODELS")
	if dir == "" {
		t.Skip("Set REFLECT_GO_QA_MODELS for checksum-verified external GGUF metadata checks")
	}
	for _, model := range modelCatalog() {
		if model.ID != "gemma4-31b-qat-q4-0" && model.ID != "muse-glimmer-30b-q4-k-m" {
			continue
		}
		for _, context := range []int{2048, 16384, 49152} {
			got := inspectModel(filepath.Join(dir, model.Filename), dir, context, macMemoryBudget(48*GiB))
			if got.Issue != "" || !got.Eligible {
				t.Fatal(got)
			}
			if context == 2048 && model.EstimatedMemory2K != got.EstimatedBytes {
				t.Fatal("pre-download estimate differs from inspected GGUF", model.Name, model.EstimatedMemory2K, got.EstimatedBytes)
			}
			t.Logf("%s: context=%d estimated=%.2f GiB, within 48-GiB Mac screening budget", model.Name, context, float64(got.EstimatedBytes)/float64(GiB))
		}
	}
}
