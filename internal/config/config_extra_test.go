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

// RED: USERNAME do ambiente (login do sistema, sempre definido no Windows)
// nunca sobrepõe o .env — era a causa do 403 com token bom.
func TestUsernameIgnoresBareEnv(t *testing.T) {
	isolateEnv(t)
	t.Setenv("USERNAME", "dark")
	p := writeEnv(t, baseEnv()) // USERNAME=u
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "u" {
		t.Fatalf("username = %q, esperava o do .env", cfg.Username)
	}
}

// RED: DNSUPDATER_USERNAME explícito ganha do .env (override p/ automação).
func TestDNSUpdaterUsernameOverride(t *testing.T) {
	isolateEnv(t)
	t.Setenv("USERNAME", "dark")
	t.Setenv("DNSUPDATER_USERNAME", "explicito")
	p := writeEnv(t, baseEnv()) // USERNAME=u
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "explicito" {
		t.Fatalf("username = %q", cfg.Username)
	}
}

// RED: .env só com chaves prefixadas funciona (formato canónico novo).
func TestPrefixedFileKeys(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, "DNSUPDATER_CPANEL_URL=https://cpanel.exemplo.com\n"+
		"DNSUPDATER_CPANEL_PORT=2083\n"+
		"DNSUPDATER_USERNAME=u\n"+
		"DNSUPDATER_APIKEY=k\n"+
		"DNSUPDATER_DOMAIN=exemplo.pt\n"+
		"DNSUPDATER_SUBDOMAIN=homeip\n")
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Username != "u" || cfg.Domain != "exemplo.pt" || cfg.FQDN() != "homeip.exemplo.pt" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

// RED: DNSUPDATER_* do ambiente ganha do legado no ficheiro.
func TestPrefixedEnvBeatsLegacyFile(t *testing.T) {
	isolateEnv(t)
	t.Setenv("DNSUPDATER_DOMAIN", "novo.pt")
	p := writeEnv(t, baseEnv()) // DOMAIN=exemplo.pt
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "novo.pt" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

// RED: bare do ambiente é ignorado para todas as chaves (anti-colisão).
func TestBareEnvIgnored(t *testing.T) {
	isolateEnv(t)
	t.Setenv("DOMAIN", "evil.pt")
	t.Setenv("USERNAME", "dark")
	p := writeEnv(t, baseEnv())
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "exemplo.pt" || cfg.Username != "u" {
		t.Fatalf("bare env vazou: %+v", cfg)
	}
}

// RED: legado singular com lista também divide (regressão valhalla:
// lista colada em GET_IP_URL era tratada como 1 URL e falhava tudo).
func TestLegacySingularList(t *testing.T) {
	isolateEnv(t)
	p := writeEnv(t, baseEnv()+"GET_IP_URL=https://a.example, https://b.example https://c.example\n")
	cfg, err := config.Load(config.Overrides{ConfigPath: p})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetIPURLs) != 3 {
		t.Fatalf("urls = %v", cfg.GetIPURLs)
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
