package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"dnsupdater/internal/state"
)

// RED: ficheiro inexistente = estado vazio sem erro.
func TestLoadMissing(t *testing.T) {
	s, err := state.Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if s.PublicIP != "" || s.InSync {
		t.Fatalf("estado devia ser vazio: %+v", s)
	}
}

// RED: JSON corrupto deve erro (não panic).
func TestLoadCorrupt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(p, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Load(p); err == nil {
		t.Fatal("esperava erro JSON corrupto")
	}
}
