package state_test

import (
	"path/filepath"
	"testing"
	"time"

	"dnsupdater/internal/state"
)

func TestFresh(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	s := &state.State{PublicIP: "1.2.3.4", ZoneIP: "1.2.3.4", InSync: true, LastCheck: time.Now()}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Fresh("1.2.3.4", time.Hour) {
		t.Fatal("esperava fresh")
	}
	if loaded.Fresh("5.6.7.8", time.Hour) {
		t.Fatal("IP diferente devia invalidar")
	}
	loaded.LastCheck = time.Now().Add(-2 * time.Hour)
	if loaded.Fresh("1.2.3.4", time.Hour) {
		t.Fatal("force-refresh expirado devia invalidar")
	}
}
