package config_test

import (
	"runtime"
	"testing"
	"time"

	"dnsupdater/internal/config"
)

// RED: default de log por OS (Unix /var, Windows pasta local).
func TestDefaultLogFile(t *testing.T) {
	got := config.DefaultLogFile()
	if got == "" {
		t.Fatal("default vazio")
	}
	if runtime.GOOS == "windows" {
		if got != "dnsupdater.log" {
			t.Fatalf("windows default = %q", got)
		}
	} else if got != "/var/log/dnsupdater.log" {
		t.Fatalf("unix default = %q", got)
	}
}

// RED: .env com export, aspas, comentários e GET_IP_URLS com vírgulas.
func TestDotEnvRichParsing(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, `
# comentário
export CPANEL_URL="https://cpanel.exemplo.com"
CPANEL_PORT=2083
USERNAME='user com espaço'
APIKEY=abc # inline
DOMAIN=exemplo.pt
TTL=600
SUBDOMAIN=homeip
GET_IP_URLS=https://a.example, https://b.example/ip https://c.example
LOG_FILE=/tmp/x.log
INTERVAL=90
FORCE_REFRESH=30m
DRY_RUN=true
VERBOSE=1
`)
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Username != "user com espaço" {
		t.Fatalf("username = %q", cfg.Username)
	}
	if cfg.APIKey != "abc" {
		t.Fatalf("apikey = %q", cfg.APIKey)
	}
	if len(cfg.GetIPURLs) != 3 {
		t.Fatalf("urls = %v", cfg.GetIPURLs)
	}
	if cfg.TTL != 600 || cfg.Interval != 90*time.Second || cfg.ForceRefresh != 30*time.Minute {
		t.Fatalf("ttl=%d interval=%s force=%s", cfg.TTL, cfg.Interval, cfg.ForceRefresh)
	}
	if !cfg.DryRun || !cfg.Verbose {
		t.Fatal("dry-run/verbose deviam ser true")
	}
}

// RED: overrides de flags ganham do ficheiro.
func TestOverridesWin(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv())
	cfg, err := config.Load(config.Overrides{
		ConfigPath:   p,
		LogFile:      "/tmp/flag.log",
		StateFile:    "/tmp/flag-state.json",
		Interval:     2 * time.Minute,
		ForceRefresh: 5 * time.Minute,
		DryRun:       true,
		DryRunSet:    true,
		Verbose:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogFile != "/tmp/flag.log" || cfg.Interval != 2*time.Minute || !cfg.DryRun {
		t.Fatalf("overrides ignorados: %+v", cfg)
	}
}

// RED: FQDN cobre @, short, FQDN com ponto.
func TestFQDNVariants(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv())
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FQDN() != "homeip.exemplo.pt" {
		t.Fatalf("fqdn = %q", cfg.FQDN())
	}
	cfg.Subdomain = "@"
	if cfg.FQDN() != "exemplo.pt" {
		t.Fatalf("fqdn @ = %q", cfg.FQDN())
	}
	cfg.Subdomain = "homeip.exemplo.pt."
	if cfg.FQDN() != "homeip.exemplo.pt" {
		t.Fatalf("fqdn full = %q", cfg.FQDN())
	}
}
