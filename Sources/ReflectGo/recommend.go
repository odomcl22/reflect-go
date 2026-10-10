package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
)

func hashFileContext(ctx context.Context, path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	b := make([]byte, 1<<20)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		n, e := f.Read(b)
		if n > 0 {
			h.Write(b[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type Recommendation struct {
	Model   string `json:"model"`
	Context int    `json:"context"`
	Backend string `json:"backend"`
	Purpose string `json:"purpose"`
	Reason  string `json:"reason"`
}

func recommend(inv Inventory, purpose string) Recommendation {
	r := Recommendation{Context: 2048, Backend: "auto", Purpose: purpose, Reason: "Download or import a compatible model first. Estimates are not a guarantee of fit."}
	budget := inv.Hardware.ScreeningBudgetBytes
	if available := inv.Hardware.AvailableMemoryBytes; available > 0 {
		budget = min(budget, available*8/10)
	}
	models := append([]Model(nil), inv.Models...)
	sort.Slice(models, func(i, j int) bool { return models[i].Bytes > models[j].Bytes })
	contexts := []int{4096, 2048}
	if purpose == "coding" {
		contexts = []int{16384, 8192}
	}
	for _, m := range models {
		if purpose == "coding" && modelPurpose(m.Name) == "chat" {
			continue
		}
		for _, c := range contexts {
			candidate := inspectModel(m.Path, inv.ModelsPath, c, budget)
			if candidate.Eligible {
				r.Model = m.Name
				r.Context = c
				r.Reason = "Fits the conservative RAM budget at this context (capped by 80% of reported available memory where measurable). Automatic compute probes available acceleration. Coding/tool reliability still depends on the model."
				return r
			}
		}
	}
	if purpose == "coding" {
		r.Reason = "No installed coding candidate passes the memory screen at 8,192 context. Use Chat or a smaller coding model; a tiny chat demonstration is not coding qualification."
	}
	return r
}
func modelPurpose(name string) string {
	for _, m := range modelCatalog() {
		if m.Filename == name {
			return m.Purpose
		}
	}
	return "unverified"
}
