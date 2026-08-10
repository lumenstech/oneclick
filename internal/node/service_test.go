package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeControl struct {
	results         []CommandResult
	credentialCalls int
}

func (f *fakeControl) Heartbeat(context.Context) error                               { return nil }
func (f *fakeControl) Poll(context.Context, time.Duration) (*CommandEnvelope, error) { return nil, nil }
func (f *fakeControl) SourceCredential(context.Context, string) (SourceCredential, error) {
	f.credentialCalls++
	return SourceCredential{Token: "opaque", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}, nil
}
func (f *fakeControl) SendResult(_ context.Context, r CommandResult) error {
	f.results = append(f.results, r)
	return nil
}

type fakeFetcher struct{ calls int }

func (f *fakeFetcher) Fetch(_ context.Context, repo, sha, token, dest string) error {
	f.calls++
	return os.WriteFile(filepath.Join(dest, "Dockerfile"), []byte("FROM scratch\n"), 0o600)
}

type fakeRunner struct {
	preflight, deploy        int
	repo, revision, planHash string
}

func (r *fakeRunner) Preflight(_ context.Context, _ string, repo, revision, planHash string) error {
	r.preflight++
	r.repo, r.revision, r.planHash = repo, revision, planHash
	return nil
}
func (r *fakeRunner) Deploy(_ context.Context, _ string, repo, revision, planHash string, _ int) error {
	r.deploy++
	r.repo, r.revision, r.planHash = repo, revision, planHash
	return nil
}

func TestExecuteAtMostOnceAndResendsCachedResult(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	dir := t.TempDir()
	ctl := &fakeControl{}
	fetch := &fakeFetcher{}
	run := &fakeRunner{}
	svc := &Service{Config: Config{StateDir: dir}, Identity: Identity{Private: priv, Public: pub, State: NodeState{NodeID: "node_1", ControlPublicKey: rawB64.EncodeToString(pub)}}, Client: ctl, Fetcher: fetch, Runner: run, Journal: Journal{Path: filepath.Join(dir, "journal.jsonl"), ResultsDir: filepath.Join(dir, "results")}, Now: func() time.Time { return now }}
	cmd := validCommand(now)
	if err := svc.execute(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if err := svc.execute(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if fetch.calls != 1 || run.deploy != 1 || ctl.credentialCalls != 1 {
		t.Fatalf("command executed more than once: fetch=%d deploy=%d creds=%d", fetch.calls, run.deploy, ctl.credentialCalls)
	}
	if run.repo != cmd.Deploy.Repository || run.revision != cmd.Deploy.CommitSHA || run.planHash != cmd.PlanHash {
		t.Fatalf("approved plan identity not bound to runner: repo=%q rev=%q hash=%q", run.repo, run.revision, run.planHash)
	}
	if len(ctl.results) != 2 || ctl.results[0].Status != "succeeded" || ctl.results[1].Status != "succeeded" {
		t.Fatalf("cached result not resent: %#v", ctl.results)
	}
}
