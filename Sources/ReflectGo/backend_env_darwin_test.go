package main

import (
	"slices"
	"strings"
	"testing"
)

func TestMacRuntimeKeepsCPUModeSeparateFromMetal(t *testing.T) {
	t.Setenv("GGML_METAL_DEVICES", "99")
	root := t.TempDir()
	cpu, err := runtimeEnv(root, Backend{Path: root, Kind: "cpu"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cpu, "GGML_METAL_DEVICES=0") || slices.Contains(cpu, "GGML_METAL_DEVICES=99") {
		t.Fatal("CPU launch must suppress the Metal registry without inheriting host overrides")
	}
	metal, err := runtimeEnv(root, Backend{Path: root, Kind: "metal"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range metal {
		if strings.HasPrefix(value, "GGML_METAL_DEVICES=") {
			t.Fatal("Metal launch must use the runtime's default device enumeration")
		}
	}
}
