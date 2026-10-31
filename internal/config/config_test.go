package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"dnsupdater/internal/config"
)

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func baseEnv() string {
	return "CPANEL_URL=https://cpanel.exemplo.com\nCPANEL_PORT=2083\nUSERNAME=u\nAPIKEY=k\nDOMAIN=exemplo.pt\nTTL=300\nSUBDOMAIN=homeip\n"
}

func TestLoadDefaults(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv())
	// Isolate .env lookup: chdir to temp with only our file via ConfigPath.
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.GetIPURLs) == 0 {
		t.Fatal("esperava defaults de IP")
	}
	if cfg.LogFile != "/var/log/dnsupdater.log" {
		t.Fatalf("log default = %q", cfg.LogFile)
	}
	if cfg.FQDN() != "homeip.exemplo.pt" {
		t.Fatalf("fqdn = %q", cfg.FQDN())
	}
}

func TestLegacySingleURL(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv()+"GET_IP_URL=checkip.amazonaws.com\n")
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetIPURLs) != 1 || cfg.GetIPURLs[0] != "https://checkip.amazonaws.com" {
		t.Fatalf("urls = %v", cfg.GetIPURLs)
	}
}

func TestMissingRequired(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, "DOMAIN=x\n")
	if _, err := config.Load(config.Overrides{ConfigPath: p}); err == nil {
		t.Fatal("esperava erro de validação")
	}
}

func TestPartialNtfy(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv()+"NTFY_SERVER=https://ntfy.sh\n")
	if _, err := config.Load(config.Overrides{ConfigPath: p}); err == nil {
		t.Fatal("esperava erro ntfy parcial")
	}
}
