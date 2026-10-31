package ip

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"
)

// GetPublicIP tries each URL in order, returns first valid IP.
// Each attempt has its own timeout; User-Agent is required by ifconfig.me.
func GetPublicIP(ctx context.Context, urls []string, perTry time.Duration) (string, string, error) {
	if len(urls) == 0 {
		return "", "", fmt.Errorf("lista de providers vazia")
	}
	var errs []string
	for _, u := range urls {
		ip, err := fetchOne(ctx, u, perTry)
		if err == nil {
			return ip, u, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", u, err))
	}
	return "", "", fmt.Errorf("todos os providers falharam: %s", strings.Join(errs, "; "))
}

func fetchOne(ctx context.Context, url string, perTry time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, perTry)
	defer cancel()
	req, err := newRequest(ctx, url)
	if err != nil {
		return "", err
	}
	resp, err := sharedClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	// Providers return plain text, sometimes with newline.
	candidate := strings.TrimSpace(string(body))
	// Take first line/field in case of extra text.
	if i := strings.IndexAny(candidate, " \t\r\n"); i >= 0 {
		candidate = strings.TrimSpace(candidate[:i])
	}
	addr, err := netip.ParseAddr(candidate)
	if err != nil {
		return "", fmt.Errorf("resposta inválida %q", trunc(candidate, 64))
	}
	if !addr.Is4() && !addr.Is6() {
		return "", fmt.Errorf("IP inválido %q", candidate)
	}
	return addr.String(), nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
