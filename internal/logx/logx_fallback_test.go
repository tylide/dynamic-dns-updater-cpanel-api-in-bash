package logx_test

import (
	"os"
	"path/filepath"
	"testing"

	"dnsupdater/internal/logx"
)

// RED: fallback local nunca vazio e termina em dnsupdater.log.
func TestLocalFallbackPath(t *testing.T) {
	p := logx.LocalFallbackPath()
	if p == "" {
		t.Fatal("fallback vazio")
	}
	if filepath.Base(p) != "dnsupdater.log" {
		t.Fatalf("fallback = %q", p)
	}
}

// RED: só paths absolutos contam como "sistema" (disparam link local).
func TestIsSystemLogPath(t *testing.T) {
	if !logx.IsSystemLogPath("/var/log/dnsupdater.log") {
		t.Fatal("abs devia ser sistema")
	}
	if logx.IsSystemLogPath("dnsupdater.log") || logx.IsSystemLogPath("") {
		t.Fatal("relativo/vazio não é sistema")
	}
}

// RED: cria link pasta-atual -> alvo, idempotente, sem esmagar ficheiro regular.
func TestEnsureLocalLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.log")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if err := logx.EnsureLocalLink(target); err != nil {
		t.Fatalf("link: %v", err)
	}
	link := filepath.Join(dir, "dnsupdater.log")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("link ausente: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("esperava symlink")
	}
	// Idempotente: segunda chamada mantém.
	if err := logx.EnsureLocalLink(target); err != nil {
		t.Fatalf("re-link: %v", err)
	}

	// Ficheiro regular existente nunca é substituído.
	dir2 := t.TempDir()
	t2 := filepath.Join(dir2, "real.log")
	if err := os.WriteFile(t2, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "dnsupdater.log"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir2)
	if err := logx.EnsureLocalLink(t2); err != nil {
		t.Fatalf("link2: %v", err)
	}
	fi2, _ := os.Lstat(filepath.Join(dir2, "dnsupdater.log"))
	if fi2.Mode()&os.ModeSymlink != 0 {
		t.Fatal("não devia esmagar ficheiro regular")
	}
}
