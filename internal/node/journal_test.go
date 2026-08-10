package node

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestJournalRefusesReplayAndCachesResult(t *testing.T) {
	dir := t.TempDir()
	j := Journal{Path: filepath.Join(dir, "journal.jsonl"), ResultsDir: filepath.Join(dir, "results")}
	now := time.Now()
	if err := j.Reserve("cmd_1", now); err != nil {
		t.Fatal(err)
	}
	if err := j.Reserve("cmd_1", now); !errors.Is(err, ErrReplay) {
		t.Fatalf("expected replay, got %v", err)
	}
	want := CommandResult{CommandID: "cmd_1", Status: "succeeded", PlanHash: "abc", Revision: "def"}
	if err := j.SaveResult(want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := j.LoadResult("cmd_1")
	if err != nil || !ok || got.Status != "succeeded" {
		t.Fatalf("bad cached result: %#v %v %v", got, ok, err)
	}
}
