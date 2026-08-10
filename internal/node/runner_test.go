package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerNeverNeedsShellAndBindsApprovedPlan(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args.txt")
	bin := filepath.Join(dir, "oneclick")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + logPath + "\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { t.Fatal(err) }
	r := OneClickRunner{Binary: bin}
	source := filepath.Join(dir, "source with spaces")
	_ = os.MkdirAll(source, 0o700)
	sha := strings.Repeat("a", 40)
	hash := strings.Repeat("b", 64)
	if err := r.Deploy(context.Background(), source, "acme/demo", sha, hash, 3000); err != nil { t.Fatal(err) }
	b, _ := os.ReadFile(logPath)
	got := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{"deploy", source, "--yes", "--port=3000", "--source-ref=https://github.com/acme/demo", "--source-type=github", "--source-revision=" + sha, "--expected-plan-hash=" + hash}
	if len(got) != len(want) { t.Fatalf("args %#v", got) }
	for i := range want { if got[i] != want[i] { t.Fatalf("arg %d = %q want %q", i, got[i], want[i]) } }
}

func TestRunnerDoesNotForwardParentSecretsOrChildOutput(t *testing.T) {
	dir := t.TempDir()
	envLog := filepath.Join(dir, "env.txt")
	bin := filepath.Join(dir, "oneclick")
	script := "#!/bin/sh\nprintf '%s' \"$ONECLICK_CONTROL_SECRET\" > \"" + envLog + "\"\necho 'repo-secret-value' >&2\nexit 7\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { t.Fatal(err) }
	t.Setenv("ONECLICK_CONTROL_SECRET", "must-not-reach-child")
	err := (OneClickRunner{Binary: bin}).Preflight(context.Background(), dir, "acme/demo", strings.Repeat("a", 40), strings.Repeat("b", 64))
	if err == nil { t.Fatal("expected child failure") }
	if strings.Contains(err.Error(), "repo-secret-value") || strings.Contains(err.Error(), "must-not-reach-child") { t.Fatalf("error leaked child or parent secret: %v", err) }
	b, readErr := os.ReadFile(envLog)
	if readErr != nil { t.Fatal(readErr) }
	if len(b) != 0 { t.Fatalf("parent secret reached child environment: %q", string(b)) }
}
