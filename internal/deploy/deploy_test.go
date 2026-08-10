package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lumenstech/oneclick/internal/analyzer"
)

func TestRequiresExplicitConfirmation(t *testing.T) {
	p := analyzer.Plan{Runtime: analyzer.Runtime{Deployable: true, Executor: "docker"}}
	err := Run(p, t.TempDir(), Options{})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestFailsClosedForUnsupportedExecutor(t *testing.T) {
	p := analyzer.Plan{Runtime: analyzer.Runtime{Deployable: false, Executor: "unsupported"}}
	err := Run(p, t.TempDir(), Options{Confirm: true})
	if err == nil {
		t.Fatal("expected unsupported deployment to fail")
	}
}

func TestComposeExecutorCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	log := installFakeDocker(t)
	p := analyzer.Plan{
		App:     analyzer.App{Name: "demo"},
		Runtime: analyzer.Runtime{Deployable: true, Executor: "docker-compose"},
	}
	if err := Run(p, t.TempDir(), Options{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	got := readLog(t, log)
	if got != "compose up -d --build" {
		t.Fatalf("docker args=%q", got)
	}
}

func TestDockerExecutorPreservesContainerPort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	log := installFakeDocker(t)
	p := analyzer.Plan{
		App:     analyzer.App{Name: "demo"},
		Source:  analyzer.Source{Type: "github", Ref: "https://github.com/example/demo", Revision: "abc123"},
		Runtime: analyzer.Runtime{Deployable: true, Executor: "docker"},
		Ports:   []int{8000},
	}
	p.PlanHash = analyzer.PlanHash(p)
	if err := Run(p, t.TempDir(), Options{Confirm: true, Port: 3000}); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(readLog(t, log), "\n")
	if len(got) != 3 {
		t.Fatalf("docker calls=%#v", got)
	}
	if !strings.HasPrefix(got[0], "build -t oneclick/demo:") || !strings.HasSuffix(got[0], " .") {
		t.Fatalf("build call=%q", got[0])
	}
	if got[1] != "rm -f oneclick-demo" {
		t.Fatalf("rm call=%q", got[1])
	}
	if !strings.Contains(got[2], "-p 3000:8000") {
		t.Fatalf("run call=%q", got[2])
	}
}

func installFakeDocker(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "docker.log")
	script := filepath.Join(bin, "docker")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ONECLICK_DOCKER_LOG\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ONECLICK_DOCKER_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}
