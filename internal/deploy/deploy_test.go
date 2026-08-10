package deploy

import (
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
