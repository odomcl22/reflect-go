package main

// Desktop clients own their host profiles. Journal only the fields Reflect Go
// changes, and refuse to overwrite a later change made by the client or user.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
	"os"
	"path/filepath"
	"strings"
)

type profileEdit struct {
	Path         string            `json:"path"`
	Key          string            `json:"key"`
	Before       json.RawMessage   `json:"before,omitempty"`
	After        json.RawMessage   `json:"after,omitempty"`
	Format       string            `json:"format,omitempty"`
	Created      bool              `json:"created,omitempty"`
	Delete       bool              `json:"delete,omitempty"`
	AllowedAfter []json.RawMessage `json:"allowed_after,omitempty"`
}
type profileJournal struct {
	Host  string        `json:"host"`
	Home  string        `json:"home"`
	Edits []profileEdit `json:"edits"`
}

func profileJSON(path string) (map[string]json.RawMessage, error) {
	value := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return value, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(path, ".jsonc") {
		data, err = hujson.Standardize(data)
		if err != nil {
			return nil, fmt.Errorf("cannot read desktop JSONC settings: %s", filepath.Base(path))
		}
	}
	if err = json.Unmarshal(data, &value); err != nil || value == nil {
		return nil, fmt.Errorf("cannot read desktop JSON settings: %s", filepath.Base(path))
	}
	return value, nil
}
func equalJSON(a, b json.RawMessage) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}
func newProfileEdit(path, key string, after any) (profileEdit, error) {
	return newProfileEditFormat(path, key, after, "")
}
func newProfileEditFormat(path, key string, after any, format string) (profileEdit, error) {
	value, err := readProfile(path, format)
	if err != nil {
		return profileEdit{}, err
	}
	data, err := json.Marshal(after)
	_, statErr := os.Stat(path)
	return profileEdit{Created: os.IsNotExist(statErr), Path: path, Key: key, Before: profileValue(value, key), After: data, Format: format}, err
}
func applyProfileJournal(path string, j profileJournal) error {
	if _, err := os.Stat(path); err == nil {
		return errors.New("a desktop connection is already recorded; disconnect it before changing models")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := writeJSON(path, j); err != nil {
		return err
	}
	for _, edit := range j.Edits {
		value, err := readProfile(edit.Path, edit.Format)
		if err == nil && !equalJSON(profileValue(value, edit.Key), edit.Before) {
			err = errors.New("desktop settings changed during connection; retry")
		}
		if err == nil {
			err = os.MkdirAll(filepath.Dir(edit.Path), 0700)
		}
		if err == nil {
			if edit.Delete {
				putProfileValue(value, edit.Key, nil)
			} else {
				putProfileValue(value, edit.Key, edit.After)
			}
			err = writeProfile(edit.Path, value, edit.Format)
		}
		if err != nil {
			restoreErr := restoreProfileJournal(path, j.Host, j.Home)
			return errors.Join(err, restoreErr)
		}
	}
	return nil
}
func restoreProfileJournal(path, host, home string) error {
	var j profileJournal
	if err := readJSON(path, &j); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if j.Host != host || j.Home != home {
		return errors.New("this desktop connection belongs to another computer; restore it there")
	}
	var issues []error
	for i := len(j.Edits) - 1; i >= 0; i-- {
		edit := j.Edits[i]
		value, err := readProfile(edit.Path, edit.Format)
		if err != nil {
			issues = append(issues, err)
			continue
		}
		if equalJSON(profileValue(value, edit.Key), edit.Before) {
			continue
		}
		if !isOwnedProfileValue(profileValue(value, edit.Key), edit) {
			issues = append(issues, fmt.Errorf("%s changed outside Reflect Go; preserved it", edit.Key))
			continue
		}
		if len(edit.Before) == 0 {
			putProfileValue(value, edit.Key, nil)
		} else {
			putProfileValue(value, edit.Key, edit.Before)
		}
		if err = writeProfile(edit.Path, value, edit.Format); err != nil {
			issues = append(issues, err)
		}
	}
	if len(issues) > 0 {
		return errors.Join(issues...)
	}
	for _, edit := range j.Edits {
		if edit.Created {
			value, err := readProfile(edit.Path, edit.Format)
			if err == nil && len(value) == 0 {
				if err = os.Remove(edit.Path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}
	return os.Remove(path)
}

func expectedProfileValue(edit profileEdit) json.RawMessage {
	if edit.Delete {
		return nil
	}
	return edit.After
}
func isOwnedProfileValue(value json.RawMessage, edit profileEdit) bool {
	if equalJSON(value, expectedProfileValue(edit)) {
		return true
	}
	for _, allowed := range edit.AllowedAfter {
		if equalJSON(value, allowed) {
			return true
		}
	}
	return false
}
func readProfile(path, format string) (map[string]json.RawMessage, error) {
	if format == "" {
		return profileJSON(path)
	}
	if format != "toml" {
		return nil, errors.New("unsupported profile format")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err = toml.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("cannot read TOML settings: %s", filepath.Base(path))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	if string(raw) != "null" {
		err = json.Unmarshal(raw, &result)
	}
	return result, err
}
func writeProfile(path string, value map[string]json.RawMessage, format string) error {
	if format == "" {
		if strings.HasSuffix(path, ".jsonc") {
			return writeJSONCProfile(path, value)
		}
		return writeJSON(path, value)
	}
	// Keep TOML's integer and date types for untouched values. Round-tripping
	// all settings through JSON would turn integers into floats and break Codex.
	parsed := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err = toml.Unmarshal(data, &parsed); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for key := range parsed {
		if _, ok := value[key]; !ok {
			delete(parsed, key)
		}
	}
	for key, raw := range value {
		old, _ := json.Marshal(parsed[key])
		if equalJSON(old, raw) {
			continue
		}
		var v any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&v); err != nil {
			return err
		}
		if n, ok := v.(json.Number); ok {
			if integer, err := n.Int64(); err == nil {
				v = integer
			} else {
				number, err := n.Float64()
				if err != nil {
					return err
				}
				v = number
			}
		}
		parsed[key] = v
	}
	data, err := toml.Marshal(parsed)
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0600)
}

// Patch only changed leaves, retaining comments on unrelated OpenCode settings.
func writeJSONCProfile(path string, value map[string]json.RawMessage) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return writeJSON(path, value)
	}
	if err != nil {
		return err
	}
	node, err := hujson.Parse(data)
	if err != nil {
		return err
	}
	before, err := profileJSON(path)
	if err != nil {
		return err
	}
	var operations []map[string]any
	var diff func(map[string]json.RawMessage, map[string]json.RawMessage, string)
	diff = func(old, next map[string]json.RawMessage, parent string) {
		for key := range old {
			if _, exists := next[key]; !exists {
				operations = append(operations, map[string]any{"op": "remove", "path": parent + "/" + jsonPointer(key)})
			}
		}
		for key, raw := range next {
			if equalJSON(old[key], raw) {
				continue
			}
			pointer := parent + "/" + jsonPointer(key)
			var a, b map[string]json.RawMessage
			if json.Unmarshal(old[key], &a) == nil && a != nil && json.Unmarshal(raw, &b) == nil && b != nil {
				diff(a, b, pointer)
				continue
			}
			op := "replace"
			if _, exists := old[key]; !exists {
				op = "add"
			}
			operations = append(operations, map[string]any{"op": op, "path": pointer, "value": raw})
		}
	}
	diff(before, value, "")
	if len(operations) == 0 {
		return nil
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	if err = node.Patch(patch); err != nil {
		return err
	}
	return atomicWrite(path, node.Pack(), 0600)
}
func jsonPointer(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func profileValue(value map[string]json.RawMessage, key string) json.RawMessage {
	first, rest, nested := strings.Cut(key, "/")
	if !nested {
		return value[first]
	}
	var child map[string]json.RawMessage
	if json.Unmarshal(value[first], &child) != nil {
		return nil
	}
	return profileValue(child, rest)
}
func putProfileValue(value map[string]json.RawMessage, key string, raw json.RawMessage) {
	first, rest, nested := strings.Cut(key, "/")
	if !nested {
		if len(raw) == 0 {
			delete(value, first)
		} else {
			value[first] = raw
		}
		return
	}
	child := map[string]json.RawMessage{}
	_ = json.Unmarshal(value[first], &child)
	if child == nil {
		child = map[string]json.RawMessage{}
	}
	putProfileValue(child, rest, raw)
	if len(child) == 0 {
		delete(value, first)
	} else {
		value[first], _ = json.Marshal(child)
	}
}
