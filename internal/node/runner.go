package node

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type OneClickRunner struct{ Binary string }

func (r OneClickRunner) Preflight(ctx context.Context, source, repository, revision, planHash string) error {
	return r.run(ctx, append([]string{"preflight", source}, planIdentityArgs(repository, revision, planHash)...)...)
}
func (r OneClickRunner) Deploy(ctx context.Context, source, repository, revision, planHash string, hostPort int) error {
	args := []string{"deploy", source, "--yes"}
	if hostPort > 0 {
		args = append(args, fmt.Sprintf("--port=%d", hostPort))
	}
	args = append(args, planIdentityArgs(repository, revision, planHash)...)
	return r.run(ctx, args...)
}
func planIdentityArgs(repository, revision, planHash string) []string {
	return []string{
		"--source-ref=https://github.com/" + repository,
		"--source-type=github",
		"--source-revision=" + revision,
		"--expected-plan-hash=" + planHash,
	}
}
func (r OneClickRunner) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LANG=C.UTF-8",
		"GIT_TERMINAL_PROMPT=0",
	}
	// Repository build output may contain application secrets. The node never
	// forwards child stdout/stderr to the control plane in V0.1.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("oneclick %s failed: %w", args[0], err)
	}
	return nil
}
