package main

import (
	"strings"
	"testing"
)

func TestKnownInvalidMetalOutputIsBlockedWithoutDisablingOtherModels(t *testing.T) {
	b := Backend{Available: true, Kind: "metal", MemoryBytes: 8 * GiB, FreeBytes: 8 * GiB}
	model := Model{Name: "granite-4.0-1b-Q4_K_M.gguf", EstimatedBytes: GiB}
	if _, err := backendForModel(t.TempDir(), b, model, "metal", false); err == nil || !strings.Contains(err.Error(), "requires CPU") {
		t.Fatal(err)
	}
	if _, err := backendForModel(t.TempDir(), b, model, "auto", false); err == nil {
		t.Fatal("missing required CPU runtime did not fail closed")
	}
	model.Name = "another-model.gguf"
	if got, err := backendForModel(t.TempDir(), b, model, "auto", false); err != nil || got.Kind != "metal" {
		t.Fatal(got, err)
	}
}
