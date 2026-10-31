package lock_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dnsupdater/internal/lock"
)

// RED: acquire -> release liberta; segundo acquire concorrente falha.
func TestAcquireRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".lock_update")
	rel, err := lock.Acquire(p, 10*time.Minute)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := lock.Acquire(p, 10*time.Minute); err == nil {
		t.Fatal("esperava locked no segundo acquire")
	}
	rel()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("lock devia ser removido no release")
	}
	// Após release, novo acquire OK.
	rel2, err := lock.Acquire(p, 10*time.Minute)
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	rel2()
}

// RED: lock stale (>staleAfter) é expirado e reutilizado (evita deadlock do .sh).
func TestStaleLockExpires(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".lock_update")
	rel, err := lock.Acquire(p, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	rel() // liberta mas recria ficheiro stale manualmente
	if err := os.WriteFile(p, []byte("123 old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	rel2, err := lock.Acquire(p, 10*time.Minute)
	if err != nil {
		t.Fatalf("stale devia expirar: %v", err)
	}
	rel2()
}
