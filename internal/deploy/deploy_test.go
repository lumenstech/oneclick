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

func TestComposeExecutorRunsConfigSafetyCheckBeforeUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	log := installFakeDocker(t, `{"services":{"web":{"image":"nginx"}}}`)
	p := analyzer.Plan{
		App:     analyzer.App{Name: "demo"},
		Source:  analyzer.Source{Type: "github", Ref: "https://github.com/acme/demo", Revision: "abc123"},
		Runtime: analyzer.Runtime{Deployable: true, Executor: "docker-compose"},
	}
	if err := Run(p, root, Options{Confirm: true}); err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(readLog(t, log), "\n")
	if len(calls) != 2 {
		t.Fatalf("docker calls=%#v", calls)
	}
	project := composeProjectName(p)
	wantConfig := "compose -p " + project + " -f compose.yaml config --format json --no-env-resolution --no-interpolate --no-path-resolution"
	if calls[0] != wantConfig {
		t.Fatalf("config call=%q want=%q", calls[0], wantConfig)
	}
	wantUp := "compose -p " + project + " -f compose.yaml up -d --build"
	if calls[1] != wantUp {
		t.Fatalf("up call=%q want=%q", calls[1], wantUp)
	}
}

func TestComposeRejectsPrivilegedCanonicalModelBeforeUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n    privileged: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	log := installFakeDocker(t, `{"services":{"web":{"image":"nginx","privileged":true}}}`)
	p := analyzer.Plan{App: analyzer.App{Name: "demo"}, Runtime: analyzer.Runtime{Deployable: true, Executor: "docker-compose"}}
	err := Run(p, root, Options{Confirm: true})
	if err == nil || !strings.Contains(err.Error(), "privileged") {
		t.Fatalf("expected privileged rejection, got %v", err)
	}
	calls := strings.Split(readLog(t, log), "\n")
	if len(calls) != 1 || !strings.Contains(calls[0], " config ") {
		t.Fatalf("unsafe compose progressed beyond config: %#v", calls)
	}
}

func TestComposeRejectsProviderBeforeDockerIsInvoked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  database:\n    provider:\n      type: hostile-helper\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	log := installFakeDocker(t, `{"services":{}}`)
	p := analyzer.Plan{App: analyzer.App{Name: "demo"}, Runtime: analyzer.Runtime{Deployable: true, Executor: "docker-compose"}}
	err := Run(p, root, Options{Confirm: true})
	if err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("expected provider rejection, got %v", err)
	}
	if _, statErr := os.Stat(log); !os.IsNotExist(statErr) {
		t.Fatalf("docker should not be invoked for raw provider rejection; stat=%v", statErr)
	}
}

func TestDockerExecutorPreservesContainerPort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell harness is POSIX-only")
	}
	log := installFakeDocker(t, `{}`)
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
	if !strings.Contains(got[2], "--security-opt no-new-privileges:true") {
		t.Fatalf("run safety options missing: %q", got[2])
	}
	if !strings.Contains(got[2], "-p 3000:8000") {
		t.Fatalf("run call=%q", got[2])
	}
}

func installFakeDocker(t *testing.T, configJSON string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "docker.log")
	script := filepath.Join(bin, "docker")
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ \"$1\" = compose ] && echo \"$*\" | grep -q ' config '; then printf '%s\\n' '" + configJSON + "'; fi\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
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
