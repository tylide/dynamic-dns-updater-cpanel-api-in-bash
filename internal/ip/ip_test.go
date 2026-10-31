package ip_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dnsupdater/internal/ip"
)

// RED: fallback deve usar o segundo provider quando o primeiro falha.
func TestGetPublicIPFallback(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GREEN: garante User-Agent (ifconfig.me bloqueia sem ele).
		if r.Header.Get("User-Agent") == "" {
			t.Error("esperava User-Agent")
		}
		_, _ = w.Write([]byte("  203.0.113.7\n"))
	}))
	defer good.Close()

	got, provider, err := ip.GetPublicIP(context.Background(), []string{bad.URL, good.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("GetPublicIP: %v", err)
	}
	if got != "203.0.113.7" {
		t.Fatalf("ip = %q", got)
	}
	if provider != good.URL {
		t.Fatalf("provider = %q", provider)
	}
}

// RED: corpo inválido (HTML do ifconfig.me sem /ip) deve ser rejeitado.
func TestGetPublicIPRejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>seu ip</html>"))
	}))
	defer srv.Close()

	if _, _, err := ip.GetPublicIP(context.Background(), []string{srv.URL}, 2*time.Second); err == nil {
		t.Fatal("esperava erro para HTML")
	}
}

// RED: todos falham -> erro agregado.
func TestGetPublicIPAllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
	}))
	defer srv.Close()

	if _, _, err := ip.GetPublicIP(context.Background(), []string{srv.URL}, time.Second); err == nil {
		t.Fatal("esperava erro")
	}
	if _, _, err := ip.GetPublicIP(context.Background(), nil, time.Second); err == nil {
		t.Fatal("esperava erro para lista vazia")
	}
}

// RED: IPv6 deve ser aceite.
func TestGetPublicIPIPv6(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("2001:db8::1\n"))
	}))
	defer srv.Close()

	got, _, err := ip.GetPublicIP(context.Background(), []string{srv.URL}, 2*time.Second)
	if err != nil {
		t.Fatalf("IPv6: %v", err)
	}
	if got != "2001:db8::1" {
		t.Fatalf("ip = %q", got)
	}
}
