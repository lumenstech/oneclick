package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPrepareLocalGitBindsRevision(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q")
	run(t, dir, "git", "config", "user.email", "oneclick@example.invalid")
	run(t, dir, "git", "config", "user.name", "OneClick Test")
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "Dockerfile")
	run(t, dir, "git", "commit", "-qm", "test")

	c, err := Prepare(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Type != "local" {
		t.Fatalf("type=%q", c.Type)
	}
	if len(c.Revision) != 40 {
		t.Fatalf("expected commit SHA, got %q", c.Revision)
	}
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v: %s", name, err, out)
	}
}
