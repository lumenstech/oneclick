package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func signedEnvelope(t *testing.T, p CommandPayload, priv ed25519.PrivateKey) CommandEnvelope {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return CommandEnvelope{Payload: rawB64.EncodeToString(b), Signature: rawB64.EncodeToString(ed25519.Sign(priv, b))}
}

func validCommand(now time.Time) CommandPayload {
	return CommandPayload{ID: "cmd_123", Kind: "deploy", IssuedAt: now.Add(-time.Minute).UTC().Format(time.RFC3339), ExpiresAt: now.Add(5 * time.Minute).UTC().Format(time.RFC3339), PlanHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Deploy: DeployCommand{Repository: "owner/repo", CommitSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", HostPort: 3000}}
}

func TestVerifyCommand(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	env := signedEnvelope(t, validCommand(now), priv)
	got, err := VerifyCommand(env, rawB64.EncodeToString(pub), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Deploy.Repository != "owner/repo" {
		t.Fatalf("unexpected repo %q", got.Deploy.Repository)
	}
	env.Signature = rawB64.EncodeToString(make([]byte, ed25519.SignatureSize))
	if _, err := VerifyCommand(env, rawB64.EncodeToString(pub), now); err == nil {
		t.Fatal("expected bad signature")
	}
}

func TestExpiredCommandRejected(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now().UTC()
	p := validCommand(now)
	p.ExpiresAt = now.Add(-time.Second).Format(time.RFC3339)
	if _, err := VerifyCommand(signedEnvelope(t, p, priv), rawB64.EncodeToString(pub), now); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestSignRequest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Unix(1_800_000_000, 0).UTC()
	req, _ := http.NewRequest(http.MethodPost, "https://control.example/v1/nodes/n1/result?x=1", nil)
	if err := SignRequest(req, nil, "n1", priv, now); err != nil {
		t.Fatal(err)
	}
	ts := req.Header.Get("X-OneClick-Timestamp")
	nonce := req.Header.Get("X-OneClick-Nonce")
	hash := req.Header.Get("X-OneClick-Body-SHA256")
	canonical := req.Method + "\n" + req.URL.RequestURI() + "\n" + ts + "\n" + nonce + "\n" + hash
	sig, _ := rawB64.DecodeString(req.Header.Get("X-OneClick-Signature"))
	if !ed25519.Verify(pub, []byte(canonical), sig) {
		t.Fatal("signature did not verify")
	}
}
