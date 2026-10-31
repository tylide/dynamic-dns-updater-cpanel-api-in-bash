package notify_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dnsupdater/internal/notify"
)

// RED: desativado quando server/topic vazios (não deve falhar nem chamar rede).
func TestSendDisabled(t *testing.T) {
	if err := notify.Send(t.Context(), "", "", "", "t", "high", "up", "m"); err != nil {
		t.Fatalf("disabled devia ser nil: %v", err)
	}
}

// RED: envia headers Title/Priority/Tags + Bearer.
func TestSendHeaders(t *testing.T) {
	var gotTitle, gotPriority, gotTags, gotAuth, gotBody, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotTags = r.Header.Get("Tags")
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	err := notify.Send(t.Context(), srv.URL, "topico", "tk_123", "Subdomain updated", "high", "up", "hello")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotTitle != "Subdomain updated" || gotPriority != "high" || gotTags != "up" {
		t.Fatalf("headers = %q %q %q", gotTitle, gotPriority, gotTags)
	}
	if gotAuth != "Bearer tk_123" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotUA != "dnsupdater/1.0" {
		t.Fatalf("user-agent = %q", gotUA)
	}
	if gotBody != "hello" {
		t.Fatalf("body = %q", gotBody)
	}
}

// RED: HTTP não-2xx deve erro (para o caller logar, não fingir sucesso).
func TestSendHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if err := notify.Send(t.Context(), srv.URL, "t", "", "x", "", "", "m"); err == nil {
		t.Fatal("esperava erro HTTP 500")
	}
}
