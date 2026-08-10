package node

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func SignRequest(req *http.Request, body []byte, nodeID string, private ed25519.PrivateKey, now time.Time) error {
	if nodeID == "" || len(private) != ed25519.PrivateKeySize {
		return fmt.Errorf("node signing identity unavailable")
	}
	nonce, err := randomNonce(18)
	if err != nil {
		return err
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(sum[:])
	canonical := strings.Join([]string{req.Method, req.URL.RequestURI(), timestamp, nonce, bodyHash}, "\n")
	sig := ed25519.Sign(private, []byte(canonical))
	req.Header.Set("X-OneClick-Node-Id", nodeID)
	req.Header.Set("X-OneClick-Timestamp", timestamp)
	req.Header.Set("X-OneClick-Nonce", nonce)
	req.Header.Set("X-OneClick-Body-SHA256", bodyHash)
	req.Header.Set("X-OneClick-Signature", rawB64.EncodeToString(sig))
	return nil
}

func VerifyCommand(env CommandEnvelope, controlPublicKey string, now time.Time) (CommandPayload, error) {
	pub, err := rawB64.DecodeString(controlPublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return CommandPayload{}, fmt.Errorf("invalid pinned control public key")
	}
	payloadBytes, err := rawB64.DecodeString(env.Payload)
	if err != nil || len(payloadBytes) == 0 || len(payloadBytes) > 64<<10 {
		return CommandPayload{}, fmt.Errorf("invalid command payload encoding")
	}
	sig, err := rawB64.DecodeString(env.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return CommandPayload{}, fmt.Errorf("invalid command signature encoding")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), payloadBytes, sig) {
		return CommandPayload{}, fmt.Errorf("command signature verification failed")
	}
	var cmd CommandPayload
	if err := jsonUnmarshalStrict(payloadBytes, &cmd); err != nil {
		return CommandPayload{}, err
	}
	if err := validateCommand(cmd, now); err != nil {
		return CommandPayload{}, err
	}
	return cmd, nil
}

func validateCommand(c CommandPayload, now time.Time) error {
	if !validID(c.ID) {
		return fmt.Errorf("invalid command id")
	}
	if c.Kind != "deploy" {
		return fmt.Errorf("unsupported command kind: %s", c.Kind)
	}
	issued, err := time.Parse(time.RFC3339, c.IssuedAt)
	if err != nil {
		return fmt.Errorf("invalid issued_at")
	}
	expires, err := time.Parse(time.RFC3339, c.ExpiresAt)
	if err != nil {
		return fmt.Errorf("invalid expires_at")
	}
	if issued.After(now.Add(5 * time.Minute)) {
		return fmt.Errorf("command issued_at is too far in the future")
	}
	if !expires.After(now) || expires.Sub(issued) > 15*time.Minute {
		return fmt.Errorf("command expired or lifetime exceeds 15 minutes")
	}
	if !isHexLen(c.PlanHash, 64) {
		return fmt.Errorf("invalid plan_hash")
	}
	if !validRepo(c.Deploy.Repository) {
		return fmt.Errorf("invalid repository")
	}
	if !(isHexLen(c.Deploy.CommitSHA, 40) || isHexLen(c.Deploy.CommitSHA, 64)) {
		return fmt.Errorf("invalid commit_sha")
	}
	if c.Deploy.HostPort < 0 || c.Deploy.HostPort > 65535 {
		return fmt.Errorf("invalid host_port")
	}
	return nil
}
