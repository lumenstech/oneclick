package analyzer

import "testing"

func TestStablePlanHashIgnoresFallbackWorkspaceServiceName(t *testing.T) {
	base := Plan{
		Version: 1,
		App: App{Name: "demo"},
		Source: Source{Type: "github", Ref: "https://github.com/acme/demo", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Runtime: Runtime{Primary: "docker", Detected: []string{"docker"}, Executor: "docker", Deployable: true},
		Requirements: Requirements{CPUCoresMin: 2, MemoryMBMin: 2048, DiskMBMin: 2048},
		Services: []Service{{Name: "oneclick-repo-123", Kind: "docker"}},
		Deployment: Deployment{RollbackSupported: false},
	}
	other := base
	other.Services = []Service{{Name: "node-work-command-456", Kind: "docker"}}
	if StablePlanHash(base) != StablePlanHash(other) {
		t.Fatal("stable plan hash changed with temporary checkout directory")
	}
}

func TestStablePlanHashKeepsExplicitServiceIdentity(t *testing.T) {
	base := Plan{Version: 1, App: App{Name: "demo"}, Services: []Service{{Name: "ollama", Kind: "model-runtime"}}}
	other := base
	other.Services = []Service{{Name: "vllm", Kind: "model-server"}}
	if StablePlanHash(base) == StablePlanHash(other) {
		t.Fatal("explicit service identity must remain part of the hash")
	}
}
