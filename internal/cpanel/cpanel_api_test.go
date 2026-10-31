package cpanel_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"dnsupdater/internal/cpanel"
)

func b64s(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// RED: New deve preservar porta do URL e acrescentar :port quando ausente.
func TestNewPortHandling(t *testing.T) {
	c, err := cpanel.New("https://host.exemplo.com", 2083, "u", "k")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Base, ":2083") {
		t.Fatalf("Base = %q", c.Base)
	}
	c2, err := cpanel.New("https://host.exemplo.com:2084", 2083, "u", "k")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c2.Base, ":2084") || strings.Contains(c2.Base, ":2083") {
		t.Fatalf("Base = %q", c2.Base)
	}
}

// mockZone monta parse_zone com SOA + 1x A.
func mockZone(t *testing.T, zoneIP string) []cpanel.ZoneRecord {
	t.Helper()
	return []cpanel.ZoneRecord{
		{DnameB64: b64s("homeip.exemplo.pt."), DataB64: []string{b64s(zoneIP)}, LineIndex: 7, RecordType: "A"},
		{RecordType: "SOA", DataB64: []string{b64s("ns"), b64s("adm"), b64s("2024010101")}},
	}
}

// RED: ParseZone sucesso e header Authorization.
func TestParseZoneSuccess(t *testing.T) {
	var gotAuth, gotZone, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotZone = r.URL.Query().Get("zone")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 1,
			"data": []map[string]interface{}{
				{"dname_b64": b64s("homeip.exemplo.pt."), "data_b64": []string{b64s("1.2.3.4")}, "line_index": 7, "record_type": "A"},
				{"record_type": "SOA", "data_b64": []string{b64s("ns"), b64s("adm"), b64s("2024010101")}},
			},
		})
	}))
	defer srv.Close()

	c, _ := cpanel.New(srv.URL, 0, "user", "key")
	recs, err := c.ParseZone(t.Context(), "exemplo.pt")
	if err != nil {
		t.Fatalf("ParseZone: %v", err)
	}
	if gotAuth != "cpanel user:key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotUA != "dnsupdater/1.0" {
		t.Fatalf("user-agent = %q (hosts com WAF bloqueiam Go-http-client)", gotUA)
	}
	if gotZone != "exemplo.pt" {
		t.Fatalf("zone = %q", gotZone)
	}
	if len(recs) != 2 {
		t.Fatalf("len = %d", len(recs))
	}
	_ = mockZone(t, "1.2.3.4")
}

// RED: ParseZone com status 0 deve falhar (evita falso-positivo do .sh antigo).
func TestParseZoneStatusZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"errors":["denied"]}`))
	}))
	defer srv.Close()

	c, _ := cpanel.New(srv.URL, 0, "u", "k")
	if _, err := c.ParseZone(t.Context(), "exemplo.pt"); err == nil {
		t.Fatal("esperava erro status 0")
	}
}

// RED: MassEdit envia form zone/serial/add|edit e trata status 0.
func TestMassEditFormAndError(t *testing.T) {
	var gotForm url.Values
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":1}`))
	}))
	defer srv.Close()

	c, _ := cpanel.New(srv.URL, 0, "u", "k")
	payload := cpanel.AddPayload("homeip.exemplo.pt", 300, "5.6.7.8")
	if err := c.MassEdit(t.Context(), "exemplo.pt", "123", "add", payload); err != nil {
		t.Fatalf("MassEdit: %v", err)
	}
	if gotForm.Get("zone") != "exemplo.pt" || gotForm.Get("serial") != "123" {
		t.Fatalf("form = %v", gotForm)
	}
	if gotForm.Get("add") == "" {
		t.Fatalf("esperava campo add com JSON, form=%v", gotForm)
	}
	if gotAuth != "cpanel u:k" {
		t.Fatalf("auth = %q", gotAuth)
	}

	// status 0 -> erro (regressão: .sh antigo ignorava e notificava sucesso).
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"errors":["bad serial"]}`))
	}))
	defer srv2.Close()
	c2, _ := cpanel.New(srv2.URL, 0, "u", "k")
	if err := c2.MassEdit(t.Context(), "exemplo.pt", "x", "edit", payload); err == nil {
		t.Fatal("esperava erro status 0")
	}
	if err := c2.MassEdit(t.Context(), "exemplo.pt", "x", "bogus", payload); err == nil {
		t.Fatal("esperava erro operação inválida")
	}
}

// RED: 403 deve trazer dica de diagnóstico (token/privilégios), não só o código.
func TestParseZone403Hint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("Access denied"))
	}))
	defer srv.Close()

	c, _ := cpanel.New(srv.URL, 0, "u", "k")
	_, err := c.ParseZone(t.Context(), "exemplo.pt")
	if err == nil {
		t.Fatal("esperava erro 403")
	}
	if !strings.Contains(err.Error(), "privilégios") {
		t.Fatalf("erro sem dica: %v", err)
	}
}

// RED: DecodeB64 tolera sem padding e não-base64.
func TestDecodeB64Tolerant(t *testing.T) {
	if got := cpanel.DecodeB64(b64s("2024010101")); got != "2024010101" {
		t.Fatalf("std = %q", got)
	}
	raw := strings.TrimRight(b64s("ola"), "=")
	if got := cpanel.DecodeB64(raw); got != "ola" {
		t.Fatalf("raw = %q", got)
	}
	if got := cpanel.DecodeB64(""); got != "" {
		t.Fatalf("vazio = %q", got)
	}
}
