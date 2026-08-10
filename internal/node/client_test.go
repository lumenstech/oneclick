package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnrollAndSignedHeartbeat(t *testing.T) {
	controlPub, _, _ := ed25519.GenerateKey(rand.Reader)
	var heartbeatSigned bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/enroll":
			var in EnrollmentRequest
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatal(err)
			}
			if in.EnrollmentToken != "enroll-once" || in.NodePublicKey == "" {
				t.Fatal("bad enrollment body")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(EnrollmentResponse{NodeID: "node_1", ControlPublicKey: rawB64.EncodeToString(controlPub)})
		case "/v1/nodes/node_1/heartbeat":
			heartbeatSigned = r.Header.Get("X-OneClick-Signature") != "" && r.Header.Get("X-OneClick-Node-Id") == "node_1"
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := Config{ControlURL: srv.URL, StateDir: t.TempDir(), EnrollmentToken: "enroll-once", HTTPTimeout: time.Second, AllowHTTP: true}
	id, _, err := LoadOrCreateIdentity(cfg.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	st, err := Enroll(context.Background(), cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	id.State = st
	c := NewControlClient(cfg, id)
	c.Client = srv.Client()
	if err := c.Heartbeat(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !heartbeatSigned {
		t.Fatal("heartbeat was not signed")
	}
}
