package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
)

type ClientModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func clientModels(response io.Reader, c Connection) ([]ClientModel, error) {
	var result struct {
		Data []ClientModel `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	if !c.Gateway {
		return []ClientModel{{ID: "reflect-go-local", Name: c.Model}}, nil
	}
	if len(result.Data) == 0 {
		return nil, errors.New("no model is available at this context; download a model or apply recommended settings")
	}
	sort.Slice(result.Data, func(i, j int) bool { return result.Data[i].ID < result.Data[j].ID })
	return result.Data, nil
}
func codexCatalog(root string, models []ClientModel, context int) (string, error) {
	entries := []map[string]any{}
	for _, m := range models {
		entries = append(entries, map[string]any{
			"slug": m.ID, "display_name": m.Name, "description": "Local model on your Reflect Go drive; coding capability varies by model.",
			"context_window": context, "effective_context_window_percent": 90, "shell_type": "default", "visibility": "list", "supported_in_api": true, "priority": 0,
			"base_instructions":          "You are a local coding assistant. Inspect files and use tools to make requested changes. Verify results and report failures honestly.",
			"supported_reasoning_levels": []any{}, "default_reasoning_level": nil, "supports_reasoning_summaries": false, "experimental_supported_tools": []string{}, "support_verbosity": false,
			"default_verbosity": nil, "apply_patch_tool_type": nil, "truncation_policy": map[string]any{"mode": "tokens", "limit": min(10000, context/3)}, "input_modalities": []string{"text"}, "supports_parallel_tool_calls": false,
		})
	}
	path, e := statePath(root, "codex-models.json")
	if e != nil {
		return "", e
	}
	return path, writeJSON(path, map[string]any{"models": entries})
}
func configureCatalog(root, name string, c Connection, models []ClientModel, options *[]string, env *[]string) error {
	if !c.Gateway {
		return nil
	}
	id := c.ID
	if id == "" {
		id = models[0].ID
	}
	switch name {
	case "codex":
		path, err := codexCatalog(root, models, c.Context)
		if err != nil {
			return err
		}
		// Later -c values override the legacy single-model connector defaults.
		extra := []string{"-c", "model=" + strconv.Quote(id), "-c", "model_catalog_json=" + strconv.Quote(path)}
		position := len(*options)
		for i, v := range *options {
			if v == "--" {
				position = i
				break
			}
		}
		tail := append([]string(nil), (*options)[position:]...)
		*options = append((*options)[:position], extra...)
		*options = append(*options, tail...)
	case "claude":
		// Claude accepts custom IDs through /model; it has no arbitrary catalog UI
		// equivalent to Codex/OpenCode. Print the exact supported IDs at launch.
		for i := range *env {
			for _, key := range []string{"ANTHROPIC_MODEL=", "ANTHROPIC_DEFAULT_HAIKU_MODEL=", "ANTHROPIC_DEFAULT_SONNET_MODEL=", "ANTHROPIC_DEFAULT_OPUS_MODEL=", "ANTHROPIC_SMALL_FAST_MODEL="} {
				if len((*env)[i]) >= len(key) && (*env)[i][:len(key)] == key {
					(*env)[i] = key + id
				}
			}
		}
		for i := 0; i+1 < len(*options); i++ {
			if (*options)[i] == "--model" {
				(*options)[i+1] = id
			}
		}
		fmt.Println("Available local models (use /model ID where supported by this Claude version):")
		for _, m := range models {
			fmt.Printf("  %s  %s\n", m.ID, m.Name)
		}
	case "opencode":
		for i, v := range *env {
			const prefix = "OPENCODE_CONFIG_CONTENT="
			if len(v) > len(prefix) && v[:len(prefix)] == prefix {
				var config map[string]any
				if err := json.Unmarshal([]byte(v[len(prefix):]), &config); err != nil {
					return err
				}
				entries := map[string]any{}
				for _, m := range models {
					entries[m.ID] = map[string]any{"name": m.Name, "tool_call": true, "reasoning": false, "limit": map[string]int{"context": c.Context, "output": min(2048, c.Context/4)}}
				}
				config["compaction"] = map[string]any{"auto": true, "prune": false, "reserved": min(2048, c.Context/4)}
				config["model"] = "reflect_go/" + id
				config["small_model"] = "reflect_go/" + id
				config["provider"].(map[string]any)["reflect_go"].(map[string]any)["models"] = entries
				data, err := json.Marshal(config)
				if err != nil {
					return err
				}
				(*env)[i] = prefix + string(data)
			}
		}
		for i := 0; i+1 < len(*options); i++ {
			if (*options)[i] == "--model" {
				(*options)[i+1] = "reflect_go/" + id
			}
		}
	}
	return nil
}
