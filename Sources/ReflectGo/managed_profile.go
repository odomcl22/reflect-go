package main

// Desktop clients own their host profiles. Journal only the fields Reflect Go
// changes, and refuse to overwrite a later change made by the client or user.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type profileEdit struct {
	Path   string          `json:"path"`
	Key    string          `json:"key"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after"`
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
	value, err := profileJSON(path)
	if err != nil {
		return profileEdit{}, err
	}
	data, err := json.Marshal(after)
	return profileEdit{Path: path, Key: key, Before: value[key], After: data}, err
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
		value, err := profileJSON(edit.Path)
		if err == nil && !equalJSON(value[edit.Key], edit.Before) {
			err = errors.New("desktop settings changed during connection; retry")
		}
		if err == nil {
			err = os.MkdirAll(filepath.Dir(edit.Path), 0700)
		}
		if err == nil {
			value[edit.Key] = edit.After
			err = writeJSON(edit.Path, value)
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
		value, err := profileJSON(edit.Path)
		if err != nil {
			issues = append(issues, err)
			continue
		}
		if equalJSON(value[edit.Key], edit.Before) {
			continue
		}
		if !equalJSON(value[edit.Key], edit.After) {
			issues = append(issues, fmt.Errorf("%s changed outside Reflect Go; preserved it", edit.Key))
			continue
		}
		if len(edit.Before) == 0 {
			delete(value, edit.Key)
		} else {
			value[edit.Key] = edit.Before
		}
		if err = writeJSON(edit.Path, value); err != nil {
			issues = append(issues, err)
		}
	}
	if len(issues) > 0 {
		return errors.Join(issues...)
	}
	return os.Remove(path)
}
