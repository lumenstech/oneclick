package node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type ControlPlane interface {
	Heartbeat(context.Context) error
	Poll(context.Context, time.Duration) (*CommandEnvelope, error)
	SourceCredential(context.Context, string) (SourceCredential, error)
	SendResult(context.Context, CommandResult) error
}

type SourceFetcher interface {
	Fetch(context.Context, string, string, string, string) error
}

type DeploymentRunner interface {
	Preflight(context.Context, string, string, string, string) error
	Deploy(context.Context, string, string, string, string, int) error
}

type Service struct {
	Config   Config
	Identity Identity
	Client   ControlPlane
	Fetcher  SourceFetcher
	Runner   DeploymentRunner
	Journal  Journal
	Now      func() time.Time
}

func NewService(cfg Config, id Identity) *Service {
	_, _, journal := StatePaths(cfg.StateDir)
	client := NewControlClient(cfg, id)
	return &Service{Config: cfg, Identity: id, Client: client, Fetcher: NewGitHubFetcher(client.Client), Runner: OneClickRunner{Binary: cfg.OneClickBin}, Journal: Journal{Path: journal, ResultsDir: filepath.Join(cfg.StateDir, "results")}, Now: time.Now}
}

func (s *Service) Run(ctx context.Context) error {
	_ = s.Client.Heartbeat(ctx)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		env, err := s.Client.Poll(ctx, s.Config.PollWait)
		if err != nil {
			log.Printf("node poll error: %s", sanitizeMessage(err.Error()))
			if !sleepContext(ctx, 5*time.Second) {
				return nil
			}
			continue
		}
		if env == nil {
			continue
		}
		cmd, err := VerifyCommand(*env, s.Identity.State.ControlPublicKey, s.Now())
		if err != nil {
			log.Printf("command rejected: %s", sanitizeMessage(err.Error()))
			continue
		}
		if err := s.execute(ctx, cmd); err != nil {
			log.Printf("command %s failed: %s", cmd.ID, sanitizeMessage(err.Error()))
		}
	}
}

func (s *Service) execute(ctx context.Context, cmd CommandPayload) error {
	started := s.Now().UTC()
	if err := s.Journal.Reserve(cmd.ID, started); err != nil {
		if errors.Is(err, ErrReplay) {
			if prior, ok, loadErr := s.Journal.LoadResult(cmd.ID); loadErr == nil && ok {
				return s.Client.SendResult(ctx, prior)
			}
		}
		return err
	}
	result := CommandResult{CommandID: cmd.ID, Status: "failed", PlanHash: cmd.PlanHash, Revision: cmd.Deploy.CommitSHA, StartedAt: started.Format(time.RFC3339)}
	state := "failed"
	defer func() { _ = s.Journal.Complete(cmd.ID, state, s.Now()) }()

	cred, err := s.Client.SourceCredential(ctx, cmd.ID)
	if err != nil {
		return s.finish(ctx, &result, "source_credential", err)
	}
	work := filepath.Join(s.Config.StateDir, "work", cmd.ID)
	_ = os.RemoveAll(work)
	defer os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o700); err != nil {
		return s.finish(ctx, &result, "workspace", err)
	}
	if err := s.Fetcher.Fetch(ctx, cmd.Deploy.Repository, cmd.Deploy.CommitSHA, cred.Token, work); err != nil {
		return s.finish(ctx, &result, "source_fetch", err)
	}
	cred.Token = "" // drop reference as early as possible
	if err := s.Runner.Preflight(ctx, work, cmd.Deploy.Repository, cmd.Deploy.CommitSHA, cmd.PlanHash); err != nil {
		return s.finish(ctx, &result, "preflight", err)
	}
	if err := s.Runner.Deploy(ctx, work, cmd.Deploy.Repository, cmd.Deploy.CommitSHA, cmd.PlanHash, cmd.Deploy.HostPort); err != nil {
		return s.finish(ctx, &result, "deploy", err)
	}
	result.Status = "succeeded"
	result.FinishedAt = s.Now().UTC().Format(time.RFC3339)
	state = "succeeded"
	if err := s.Journal.SaveResult(result); err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return s.Client.SendResult(ctx, result)
}

func (s *Service) finish(ctx context.Context, result *CommandResult, code string, err error) error {
	result.ErrorCode = code
	result.Message = sanitizeMessage(err.Error())
	result.FinishedAt = s.Now().UTC().Format(time.RFC3339)
	if saveErr := s.Journal.SaveResult(*result); saveErr != nil {
		return fmt.Errorf("%s: %v; save result: %w", code, err, saveErr)
	}
	_ = s.Client.SendResult(ctx, *result)
	return fmt.Errorf("%s: %w", code, err)
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
