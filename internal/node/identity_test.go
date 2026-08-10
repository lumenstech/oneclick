package node

import (
	"os"
	"testing"
)

func TestIdentityPersists(t *testing.T) {
	dir := t.TempDir()
	id, enrolled, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if enrolled {
		t.Fatal("unexpected enrolled state")
	}
	_, keyPath, _ := StatePaths(dir)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %o", info.Mode().Perm())
	}
	st := NodeState{NodeID: "node_1", ControlPublicKey: rawB64.EncodeToString(id.Public)}
	if err := SaveState(dir, st); err != nil {
		t.Fatal(err)
	}
	id2, enrolled, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !enrolled || id2.State.NodeID != "node_1" {
		t.Fatal("state not loaded")
	}
	if string(id.Private) != string(id2.Private) {
		t.Fatal("private key changed")
	}
}
