package logx_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsupdater/internal/logx"
)

// RED: cria dir + ficheiro e escreve linha INFO.
func TestNewWritesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "dnsupdater.log")
	l, err := logx.New(p, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Infof("ola %d", 42)
	_ = l.Close()

	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ola 42") || !strings.Contains(string(b), "[INFO]") {
		t.Fatalf("log = %q", b)
	}
}

// RED: path impossível devolve erro mas logger continua utilizável (stderr).
func TestNewUnwritableReturnsErr(t *testing.T) {
	// /proc é read-only: open falha sempre para user comum.
	l, err := logx.New("/proc/dnsupdater-test.log", false)
	if err == nil {
		t.Fatal("esperava erro de open")
	}
	if l == nil {
		t.Fatal("logger nil")
	}
	l.Warnf("não deve panicar")
	_ = l.Close()
}
