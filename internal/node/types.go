package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const Version = "0.1.0"

type Config struct {
	ControlURL      string
	StateDir        string
	OneClickBin     string
	EnrollmentToken string
	PollWait        time.Duration
	HTTPTimeout     time.Duration
	AllowHTTP       bool // tests/dev only; production CLI never enables this by default.
}

func LoadConfig() (Config, error) {
	cfg := Config{
		ControlURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("ONECLICK_CONTROL_URL")), "/"),
		StateDir:        envOr("ONECLICK_NODE_STATE_DIR", "/var/lib/oneclick-node"),
		OneClickBin:     envOr("ONECLICK_BIN", "/usr/local/bin/oneclick"),
		EnrollmentToken: strings.TrimSpace(os.Getenv("ONECLICK_ENROLLMENT_TOKEN")),
		PollWait:        durationEnv("ONECLICK_POLL_WAIT", 25*time.Second),
		HTTPTimeout:     durationEnv("ONECLICK_HTTP_TIMEOUT", 45*time.Second),
		AllowHTTP:       os.Getenv("ONECLICK_ALLOW_HTTP") == "1",
	}
	if cfg.ControlURL == "" {
		return Config{}, fmt.Errorf("ONECLICK_CONTROL_URL is required")
	}
	if !cfg.AllowHTTP && !strings.HasPrefix(cfg.ControlURL, "https://") {
		return Config{}, fmt.Errorf("ONECLICK_CONTROL_URL must use https")
	}
	if cfg.PollWait < time.Second || cfg.PollWait > 55*time.Second {
		return Config{}, fmt.Errorf("ONECLICK_POLL_WAIT must be between 1s and 55s")
	}
	if cfg.HTTPTimeout <= cfg.PollWait {
		return Config{}, fmt.Errorf("ONECLICK_HTTP_TIMEOUT must exceed ONECLICK_POLL_WAIT")
	}
	return cfg, nil
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func durationEnv(k string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

type NodeState struct {
	NodeID           string `json:"node_id"`
	ControlPublicKey string `json:"control_public_key"`
}

type EnrollmentRequest struct {
	EnrollmentToken string       `json:"enrollment_token"`
	NodePublicKey   string       `json:"node_public_key"`
	AgentVersion    string       `json:"agent_version"`
	Host            HostSnapshot `json:"host"`
}

type EnrollmentResponse struct {
	NodeID           string `json:"node_id"`
	ControlPublicKey string `json:"control_public_key"`
}

type HostSnapshot struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CPUCount      int    `json:"cpu_count"`
	DockerPresent bool   `json:"docker_present"`
	NVIDIAVisible bool   `json:"nvidia_visible"`
}

func SnapshotHost() HostSnapshot {
	host, _ := os.Hostname()
	return HostSnapshot{
		Hostname:      host,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		CPUCount:      runtime.NumCPU(),
		DockerPresent: commandExists("docker"),
		NVIDIAVisible: commandExists("nvidia-smi"),
	}
}

type CommandEnvelope struct {
	Payload   string `json:"payload"`   // base64url of exact UTF-8 JSON bytes
	Signature string `json:"signature"` // base64url Ed25519 signature over decoded payload bytes
}

type CommandPayload struct {
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	IssuedAt  string        `json:"issued_at"`
	ExpiresAt string        `json:"expires_at"`
	PlanHash  string        `json:"plan_hash"`
	Deploy    DeployCommand `json:"deploy"`
}

type DeployCommand struct {
	Repository string `json:"repository"` // owner/repo only
	CommitSHA  string `json:"commit_sha"`
	HostPort   int    `json:"host_port,omitempty"`
}

type SourceCredential struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type CommandResult struct {
	CommandID  string `json:"command_id"`
	Status     string `json:"status"`
	PlanHash   string `json:"plan_hash"`
	Revision   string `json:"revision"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type journalEntry struct {
	ID    string `json:"id"`
	State string `json:"state"`
	At    string `json:"at"`
}

func (c CommandPayload) MarshalForLog() string {
	b, _ := json.Marshal(map[string]any{
		"id": c.ID, "kind": c.Kind, "plan_hash": c.PlanHash,
		"repository": c.Deploy.Repository, "commit_sha": c.Deploy.CommitSHA,
	})
	return string(b)
}

func StatePaths(dir string) (statePath, keyPath, journalPath string) {
	return filepath.Join(dir, "state.json"), filepath.Join(dir, "node.key"), filepath.Join(dir, "journal.jsonl")
}
