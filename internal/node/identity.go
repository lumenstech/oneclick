package node

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
)

type Identity struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
	State   NodeState
}

func LoadOrCreateIdentity(stateDir string) (Identity, bool, error) {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return Identity{}, false, err
	}
	statePath, keyPath, _ := StatePaths(stateDir)
	var priv ed25519.PrivateKey
	keyBytes, err := os.ReadFile(keyPath)
	switch {
	case err == nil:
		decoded, err := rawB64.DecodeString(string(keyBytes))
		if err != nil || len(decoded) != ed25519.PrivateKeySize {
			return Identity{}, false, fmt.Errorf("invalid node private key")
		}
		priv = ed25519.PrivateKey(decoded)
	case os.IsNotExist(err):
		_, p, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return Identity{}, false, err
		}
		priv = p
		if err := os.WriteFile(keyPath, []byte(rawB64.EncodeToString(priv)), 0o600); err != nil {
			return Identity{}, false, err
		}
	default:
		return Identity{}, false, err
	}

	var st NodeState
	b, err := os.ReadFile(statePath)
	if err == nil {
		if err := json.Unmarshal(b, &st); err != nil {
			return Identity{}, false, fmt.Errorf("invalid state.json: %w", err)
		}
		if st.NodeID == "" || st.ControlPublicKey == "" {
			return Identity{}, false, fmt.Errorf("incomplete node state")
		}
		return Identity{Private: priv, Public: priv.Public().(ed25519.PublicKey), State: st}, true, nil
	}
	if !os.IsNotExist(err) {
		return Identity{}, false, err
	}
	return Identity{Private: priv, Public: priv.Public().(ed25519.PublicKey)}, false, nil
}

func SaveState(stateDir string, st NodeState) error {
	if st.NodeID == "" || st.ControlPublicKey == "" {
		return fmt.Errorf("incomplete node state")
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	statePath, _, _ := StatePaths(stateDir)
	tmp := statePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath)
}
