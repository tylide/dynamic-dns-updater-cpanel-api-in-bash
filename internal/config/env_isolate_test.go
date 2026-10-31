package config_test

import (
	"os"
	"testing"
)

// isolateEnv remove vars do host (ex: USERNAME=dark) para os testes
// .env não sofrerem overlay do OS env. Restaura no cleanup.
func isolateEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"CPANEL_URL", "CPANEL_PORT", "USERNAME", "APIKEY", "DOMAIN", "TTL",
		"GET_IP_URL", "GET_IP_URLS", "SUBDOMAIN",
		"NTFY_SERVER", "NTFY_TOPIC", "NTFY_TOKEN",
		"LOG_FILE", "STATE_FILE", "INTERVAL", "FORCE_REFRESH",
		"DRY_RUN", "VERBOSE",
		"DNSUPDATER_CPANEL_URL", "DNSUPDATER_CPANEL_PORT",
		"DNSUPDATER_USERNAME", "DNSUPDATER_APIKEY", "DNSUPDATER_DOMAIN",
		"DNSUPDATER_TTL", "DNSUPDATER_GET_IP_URL", "DNSUPDATER_GET_IP_URLS",
		"DNSUPDATER_SUBDOMAIN", "DNSUPDATER_NTFY_SERVER", "DNSUPDATER_NTFY_TOPIC",
		"DNSUPDATER_NTFY_TOKEN", "DNSUPDATER_LOG_FILE", "DNSUPDATER_STATE_FILE",
		"DNSUPDATER_INTERVAL", "DNSUPDATER_FORCE_REFRESH", "DNSUPDATER_DRY_RUN",
		"DNSUPDATER_VERBOSE",
	}
	orig := map[string]*string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			c := v
			orig[k] = &c
			_ = os.Unsetenv(k)
		} else {
			orig[k] = nil
		}
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if v := orig[k]; v == nil {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, *v)
			}
		}
	})
}
