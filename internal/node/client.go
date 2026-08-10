package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ControlClient struct {
	BaseURL string
	Client  *http.Client
	NodeID  string
	Private ed25519.PrivateKey
	Now     func() time.Time
}

func NewControlClient(cfg Config, id Identity) *ControlClient {
	return &ControlClient{
		BaseURL: cfg.ControlURL,
		Client:  &http.Client{Timeout: cfg.HTTPTimeout},
		NodeID:  id.State.NodeID,
		Private: id.Private,
		Now:     time.Now,
	}
}

func Enroll(ctx context.Context, cfg Config, id Identity) (NodeState, error) {
	if cfg.EnrollmentToken == "" {
		return NodeState{}, fmt.Errorf("ONECLICK_ENROLLMENT_TOKEN is required for first enrollment")
	}
	reqBody := EnrollmentRequest{
		EnrollmentToken: cfg.EnrollmentToken,
		NodePublicKey:   rawB64.EncodeToString(id.Public),
		AgentVersion:    Version,
		Host:            SnapshotHost(),
	}
	b, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ControlURL+"/v1/nodes/enroll", bytes.NewReader(b))
	if err != nil {
		return NodeState{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "oneclick-node/"+Version)
	client := &http.Client{Timeout: cfg.HTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return NodeState{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return NodeState{}, fmt.Errorf("enrollment failed: http %d: %s", resp.StatusCode, sanitizeMessage(string(msg)))
	}
	var out EnrollmentResponse
	if err := decodeJSONLimited(resp.Body, 64<<10, &out); err != nil {
		return NodeState{}, err
	}
	pub, err := rawB64.DecodeString(out.ControlPublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize || !validID(out.NodeID) {
		return NodeState{}, fmt.Errorf("invalid enrollment response")
	}
	return NodeState{NodeID: out.NodeID, ControlPublicKey: out.ControlPublicKey}, nil
}

func (c *ControlClient) doJSON(ctx context.Context, method, path string, in, out any) (*http.Response, error) {
	var body []byte
	var err error
	if in != nil {
		body, err = json.Marshal(in)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oneclick-node/"+Version)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := SignRequest(req, body, c.NodeID, c.Private, c.Now()); err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		defer resp.Body.Close()
		if err := decodeJSONLimited(resp.Body, 4<<20, out); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func (c *ControlClient) Heartbeat(ctx context.Context) error {
	path := "/v1/nodes/" + url.PathEscape(c.NodeID) + "/heartbeat"
	resp, err := c.doJSON(ctx, http.MethodPost, path, SnapshotHost(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("heartbeat failed: http %d", resp.StatusCode)
	}
	return nil
}

func (c *ControlClient) Poll(ctx context.Context, wait time.Duration) (*CommandEnvelope, error) {
	sec := int(wait / time.Second)
	path := "/v1/nodes/" + url.PathEscape(c.NodeID) + "/commands?wait_seconds=" + strconv.Itoa(sec)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "oneclick-node/"+Version)
	if err := SignRequest(req, nil, c.NodeID, c.Private, c.Now()); err != nil {
		return nil, err
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("poll failed: http %d: %s", resp.StatusCode, sanitizeMessage(string(msg)))
	}
	var env CommandEnvelope
	if err := decodeJSONLimited(resp.Body, 128<<10, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

func (c *ControlClient) SourceCredential(ctx context.Context, commandID string) (SourceCredential, error) {
	path := "/v1/nodes/" + url.PathEscape(c.NodeID) + "/commands/" + url.PathEscape(commandID) + "/source-credential"
	var out SourceCredential
	resp, err := c.doJSON(ctx, http.MethodGet, path, nil, &out)
	if err != nil {
		return SourceCredential{}, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return SourceCredential{}, fmt.Errorf("source credential failed: http %d", resp.StatusCode)
	}
	if strings.TrimSpace(out.Token) == "" {
		return SourceCredential{}, fmt.Errorf("empty source credential")
	}
	exp, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil || !exp.After(c.Now()) {
		return SourceCredential{}, fmt.Errorf("source credential expired or invalid")
	}
	return out, nil
}

func (c *ControlClient) SendResult(ctx context.Context, result CommandResult) error {
	path := "/v1/nodes/" + url.PathEscape(c.NodeID) + "/commands/" + url.PathEscape(result.CommandID) + "/result"
	resp, err := c.doJSON(ctx, http.MethodPost, path, result, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("result upload failed: http %d", resp.StatusCode)
	}
	return nil
}
