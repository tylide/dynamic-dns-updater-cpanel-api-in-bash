package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings.
// Precedence: flags > OS env > .env file > defaults.
type Config struct {
	CpanelURL    string
	CpanelPort   int
	Username     string
	APIKey       string
	Domain       string
	TTL          int
	GetIPURLs    []string
	Subdomain    string
	NtfyServer   string
	NtfyTopic    string
	NtfyToken    string
	LogFile      string
	StateFile    string
	Interval     time.Duration
	ForceRefresh time.Duration
	DryRun       bool
	Verbose      bool
}

// Overrides come from CLI flags (empty/zero = no override).
type Overrides struct {
	ConfigPath   string
	LogFile      string
	StateFile    string
	Interval     time.Duration
	ForceRefresh time.Duration
	DryRun       bool
	DryRunSet    bool
	Verbose      bool
}

var DefaultGetIPURLs = []string{
	"https://checkip.amazonaws.com",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://api.ipify.org",
	"https://v4.ident.me",
	"https://ident.me",
	"https://ifconfig.co/ip",
	"https://ipinfo.io/ip",
	"https://wtfismyip.com/text",
	"https://ipecho.net/plain",
	"https://l2.io/ip",
	"https://myexternalip.com/raw",
	"https://ipv4.icanhazip.com",
	"https://ifconfig.io/ip",
	"https://api.ipify.org?format=json", // JSON {"ip":"..."} também é aceite
}

// Load reads .env file + OS env and applies overrides.
// Precedence per key: CLI flags > DNSUPDATER_* OS env > DNSUPDATER_* in file
// > legacy bare names in file (old .env keeps working) > defaults.
// Bare OS env is never read: host vars like USERNAME (login name, always set
// on Windows) must not override the file.
func Load(ov Overrides) (*Config, error) {
	file := map[string]string{}

	// 1. .env file candidates.
	candidates := []string{}
	if ov.ConfigPath != "" {
		candidates = append(candidates, ov.ConfigPath)
	} else {
		candidates = append(candidates, ".env")
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), ".env"))
		}
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		m, err := parseDotEnv(p)
		if err == nil {
			for k, v := range m {
				if _, ok := file[k]; !ok {
					file[k] = v
				}
			}
			break // first readable file wins
		}
	}

	// Resolução com prefixo (ver doc do Load).
	get := func(names ...string) string {
		if len(names) == 0 {
			return ""
		}
		if v, ok := os.LookupEnv(names[0]); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		for _, n := range names {
			if v := strings.TrimSpace(file[n]); v != "" {
				return v
			}
		}
		return ""
	}

	cfg := &Config{
		CpanelURL:    get("DNSUPDATER_CPANEL_URL", "CPANEL_URL"),
		Username:     get("DNSUPDATER_USERNAME", "USERNAME"),
		APIKey:       get("DNSUPDATER_APIKEY", "APIKEY"),
		Domain:       get("DNSUPDATER_DOMAIN", "DOMAIN"),
		Subdomain:    get("DNSUPDATER_SUBDOMAIN", "SUBDOMAIN"),
		NtfyServer:   strings.TrimRight(get("DNSUPDATER_NTFY_SERVER", "NTFY_SERVER"), "/"),
		NtfyTopic:    get("DNSUPDATER_NTFY_TOPIC", "NTFY_TOPIC"),
		NtfyToken:    get("DNSUPDATER_NTFY_TOKEN", "NTFY_TOKEN"),
		LogFile:      firstNonEmpty(ov.LogFile, get("DNSUPDATER_LOG_FILE", "LOG_FILE"), DefaultLogFile()),
		StateFile:    firstNonEmpty(ov.StateFile, get("DNSUPDATER_STATE_FILE", "STATE_FILE"), defaultStateFile()),
		TTL:          300,
		Interval:     5 * time.Minute,
		ForceRefresh: time.Hour,
	}

	if v := get("DNSUPDATER_CPANEL_PORT", "CPANEL_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("CPANEL_PORT inválido: %q", v)
		}
		cfg.CpanelPort = p
	} else {
		cfg.CpanelPort = 2083
	}

	if v := get("DNSUPDATER_TTL", "TTL"); v != "" {
		ttl, err := strconv.Atoi(v)
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("TTL inválido: %q", v)
		}
		cfg.TTL = ttl
	}

	cfg.GetIPURLs = parseURLList(
		get("DNSUPDATER_GET_IP_URLS", "GET_IP_URLS"),
		get("DNSUPDATER_GET_IP_URL", "GET_IP_URL"),
	)
	if ov.Interval > 0 {
		cfg.Interval = ov.Interval
	} else if v := get("DNSUPDATER_INTERVAL", "INTERVAL"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("INTERVAL inválido: %q", v)
		}
		cfg.Interval = d
	}
	if ov.ForceRefresh > 0 {
		cfg.ForceRefresh = ov.ForceRefresh
	} else if v := get("DNSUPDATER_FORCE_REFRESH", "FORCE_REFRESH"); v != "" {
		d, err := parseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("FORCE_REFRESH inválido: %q", v)
		}
		cfg.ForceRefresh = d
	}
	if ov.DryRunSet {
		cfg.DryRun = ov.DryRun
	} else {
		cfg.DryRun = parseBool(get("DNSUPDATER_DRY_RUN", "DRY_RUN"))
	}
	cfg.Verbose = ov.Verbose || parseBool(get("DNSUPDATER_VERBOSE", "VERBOSE"))

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate checks required fields.
func (c *Config) Validate() error {
	var missing []string
	for k, v := range map[string]string{
		"CPANEL_URL": c.CpanelURL,
		"USERNAME":   c.Username,
		"APIKEY":     c.APIKey,
		"DOMAIN":     c.Domain,
		"SUBDOMAIN":  c.Subdomain,
	} {
		if strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("vars não definidas no .env/env: %s", strings.Join(missing, ", "))
	}
	if len(c.GetIPURLs) == 0 {
		return fmt.Errorf("nenhuma URL de IP configurada")
	}
	// NTFY é opcional: se algum definido, exige todos.
	n := (nonEmpty(c.NtfyServer) + nonEmpty(c.NtfyTopic) + nonEmpty(c.NtfyToken))
	if n > 0 && n < 3 {
		return fmt.Errorf("ntfy parcial: defina NTFY_SERVER, NTFY_TOPIC e NTFY_TOKEN ou nenhum")
	}
	return nil
}

// NtfyEnabled reports whether notifications are configured.
func (c *Config) NtfyEnabled() bool {
	return c.NtfyServer != "" && c.NtfyTopic != "" && c.NtfyToken != ""
}

// FQDN builds sub.domain unless sub already qualifies.
func (c *Config) FQDN() string {
	sub := strings.Trim(strings.TrimSpace(c.Subdomain), ".")
	dom := strings.Trim(strings.TrimSpace(c.Domain), ".")
	if sub == "" || sub == "@" {
		return dom
	}
	lowSub := strings.ToLower(strings.TrimSuffix(sub, "."))
	lowDom := strings.ToLower(dom)
	if lowSub == lowDom || strings.HasSuffix(lowSub, "."+lowDom) {
		return strings.TrimSuffix(sub, ".")
	}
	// Allow IP/host validation: not needed, just concat.
	if net.ParseIP(sub) != nil {
		return sub
	}
	return sub + "." + dom
}

func parseURLList(plurals, legacy string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" || seen[raw] {
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		seen[raw] = true
		out = append(out, raw)
	}
	for _, p := range strings.Split(plurals, ",") {
		for _, q := range strings.Fields(p) {
			add(q)
		}
	}
	if len(out) == 0 && strings.TrimSpace(legacy) != "" {
		add(strings.TrimSpace(legacy))
	}
	if len(out) == 0 {
		out = append(out, DefaultGetIPURLs...)
	}
	return out
}

func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		// Strip inline comments only when value is unquoted and has " #".
		if !strings.HasPrefix(v, `"`) && !strings.HasPrefix(v, `'`) {
			if i := strings.Index(v, " #"); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
		}
		v = unquote(v)
		if k != "" {
			m[k] = v
		}
	}
	return m, sc.Err()
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			inner := v[1 : len(v)-1]
			// Basic unescape for double quotes.
			if v[0] == '"' {
				inner = strings.ReplaceAll(inner, `\"`, `"`)
				inner = strings.ReplaceAll(inner, `\\`, `\`)
			}
			return inner
		}
	}
	return v
}

func parseDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	// Plain seconds fallback: "300".
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	return 0, fmt.Errorf("duração inválida")
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func nonEmpty(s string) int {
	if strings.TrimSpace(s) != "" {
		return 1
	}
	return 0
}

// DefaultLogFile é /var/log/dnsupdater.log no Unix e ./dnsupdater.log no Windows
// (onde /var não existe).
func DefaultLogFile() string {
	if runtime.GOOS == "windows" {
		return "dnsupdater.log"
	}
	return "/var/log/dnsupdater.log"
}

func defaultStateFile() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "dnsupdater", "state.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "state.json"
	}
	return filepath.Join(home, ".cache", "dnsupdater", "state.json")
}
