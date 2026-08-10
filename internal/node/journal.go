package node

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrReplay = errors.New("command replay refused")

type Journal struct {
	Path       string
	ResultsDir string
}

func (j Journal) Reserve(id string, now time.Time) error {
	if !validID(id) {
		return fmt.Errorf("invalid command id")
	}
	if seen, err := j.Has(id); err != nil {
		return err
	} else if seen {
		return ErrReplay
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(j.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(journalEntry{ID: id, State: "reserved", At: now.UTC().Format(time.RFC3339)})
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (j Journal) Complete(id, state string, now time.Time) error {
	f, err := os.OpenFile(j.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(journalEntry{ID: id, State: state, At: now.UTC().Format(time.RFC3339)})
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (j Journal) Has(id string) (bool, error) {
	f, err := os.Open(j.Path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	buf := make([]byte, 64<<10)
	s.Buffer(buf, 256<<10)
	for s.Scan() {
		var e journalEntry
		if json.Unmarshal(s.Bytes(), &e) == nil && e.ID == id {
			return true, nil
		}
	}
	return false, s.Err()
}

func (j Journal) SaveResult(r CommandResult) error {
	if !validID(r.CommandID) {
		return fmt.Errorf("invalid command id")
	}
	dir := j.ResultsDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(j.Path), "results")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, r.CommandID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (j Journal) LoadResult(id string) (CommandResult, bool, error) {
	if !validID(id) {
		return CommandResult{}, false, fmt.Errorf("invalid command id")
	}
	dir := j.ResultsDir
	if dir == "" {
		dir = filepath.Join(filepath.Dir(j.Path), "results")
	}
	b, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if os.IsNotExist(err) {
		return CommandResult{}, false, nil
	}
	if err != nil {
		return CommandResult{}, false, err
	}
	var r CommandResult
	if err := json.Unmarshal(b, &r); err != nil {
		return CommandResult{}, false, err
	}
	if r.CommandID != id {
		return CommandResult{}, false, fmt.Errorf("stored result id mismatch")
	}
	return r, true, nil
}
