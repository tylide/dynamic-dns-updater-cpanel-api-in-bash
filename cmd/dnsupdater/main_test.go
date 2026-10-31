package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"dnsupdater/internal/config"
	"dnsupdater/internal/logx"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

type cpanelMock struct {
	t          *testing.T
	zoneIP     string
	serial     string
	withA      bool
	parseCalls *atomic.Int32
	editCalls  *atomic.Int32
	lastOp     *string
	lastJSON   *string
	srv        *httptest.Server
}

func newCpanelMock(t *testing.T, zoneIP string, withA bool) *cpanelMock {
	t.Helper()
	m := &cpanelMock{t: t, zoneIP: zoneIP, serial: "2024010101", withA: withA,
		parseCalls: &atomic.Int32{}, editCalls: &atomic.Int32{},
		lastOp: new(string), lastJSON: new(string)}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "parse_zone") {
			m.parseCalls.Add(1)
			data := []map[string]interface{}{
				{"record_type": "SOA", "data_b64": []string{b64("ns"), b64("adm"), b64(m.serial)}},
			}
			if m.withA {
				data = append(data, map[string]interface{}{
					"dname_b64": b64("homeip.exemplo.pt."), "data_b64": []string{b64(m.zoneIP)},
					"line_index": 7, "record_type": "A",
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": 1, "data": data})
			return
		}
		if strings.Contains(r.URL.Path, "mass_edit_zone") {
			m.editCalls.Add(1)
			_ = r.ParseForm()
			for _, op := range []string{"add", "edit"} {
				if v := r.PostForm.Get(op); v != "" {
					*m.lastOp = op
					*m.lastJSON = v
				}
			}
			_, _ = w.Write([]byte(`{"status":1}`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func ipMock(t *testing.T, ip string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(ip + "\n"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testCfg(t *testing.T, cpanelURL, ipURL string) *config.Config {
	t.Helper()
	u, _ := url.Parse(cpanelURL)
	port := 0
	// New() extrai porta do URL; passa 0 para não duplicar.
	return &config.Config{
		CpanelURL: u.Scheme + "://" + u.Host, CpanelPort: port,
		Username: "u", APIKey: "k", Domain: "exemplo.pt", TTL: 300,
		GetIPURLs: []string{ipURL}, Subdomain: "homeip",
		LogFile: "", StateFile: filepath.Join(t.TempDir(), "state.json"),
		Interval: time.Minute, ForceRefresh: time.Hour,
	}
}

func testLogger() *logx.Logger {
	l, _ := logx.New("", false)
	return l
}

// RED: sem mudança (API == público) -> 0, sem mass_edit, guarda state.
func TestRunOnceNoChange(t *testing.T) {
	ipSrv := ipMock(t, "1.2.3.4")
	cp := newCpanelMock(t, "1.2.3.4", true)
	cfg := testCfg(t, cp.srv.URL, ipSrv.URL)
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if cp.editCalls.Load() != 0 {
		t.Fatal("não devia chamar mass_edit sem mudança")
	}
}

// RED: IP diferente -> edit com line_index e novo IP.
func TestRunOnceEdit(t *testing.T) {
	ipSrv := ipMock(t, "5.6.7.8")
	cp := newCpanelMock(t, "1.2.3.4", true)
	cfg := testCfg(t, cp.srv.URL, ipSrv.URL)
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if cp.editCalls.Load() != 1 || *cp.lastOp != "edit" {
		t.Fatalf("editCalls=%d op=%q", cp.editCalls.Load(), *cp.lastOp)
	}
	if !strings.Contains(*cp.lastJSON, "5.6.7.8") {
		t.Fatalf("payload sem novo IP: %s", *cp.lastJSON)
	}
}

// RED: sem registro A -> add.
func TestRunOnceAdd(t *testing.T) {
	ipSrv := ipMock(t, "9.9.9.9")
	cp := newCpanelMock(t, "", false)
	cfg := testCfg(t, cp.srv.URL, ipSrv.URL)
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if *cp.lastOp != "add" || !strings.Contains(*cp.lastJSON, "9.9.9.9") {
		t.Fatalf("op=%q payload=%s", *cp.lastOp, *cp.lastJSON)
	}
}

// RED: dry-run nunca altera.
func TestRunOnceDryRun(t *testing.T) {
	ipSrv := ipMock(t, "5.6.7.8")
	cp := newCpanelMock(t, "1.2.3.4", true)
	cfg := testCfg(t, cp.srv.URL, ipSrv.URL)
	cfg.DryRun = true
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if cp.editCalls.Load() != 0 {
		t.Fatal("dry-run não devia chamar mass_edit")
	}
}

// RED: state Fresh pula a API (fonte da verdade continua cPanel quando consulta).
func TestRunOnceSkipsAPIWhenFresh(t *testing.T) {
	ipSrv := ipMock(t, "1.2.3.4")
	cp := newCpanelMock(t, "1.2.3.4", true)
	cfg := testCfg(t, cp.srv.URL, ipSrv.URL)
	// Primeira passagem grava state.
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 0 {
		t.Fatalf("primeira: code = %d", code)
	}
	callsAfterFirst := cp.parseCalls.Load()
	// Segunda sem force: deve usar state e nem bater no parse_zone.
	if code := runOnce(context.Background(), cfg, testLogger(), false); code != 0 {
		t.Fatalf("segunda: code = %d", code)
	}
	if cp.parseCalls.Load() != callsAfterFirst {
		t.Fatal("state fresh devia pular parse_zone")
	}
}

// RED: falha de IP -> exit 1, sem panicar.
func TestRunOnceIPFailure(t *testing.T) {
	cp := newCpanelMock(t, "1.2.3.4", true)
	cfg := testCfg(t, cp.srv.URL, "http://127.0.0.1:9")
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 1 {
		t.Fatalf("code = %d, esperava 1", code)
	}
}

// RED: parse_zone 403 real -> exit 1 (regressão do falso-positivo do .sh).
func TestRunOnceParse403(t *testing.T) {
	ipSrv := ipMock(t, "1.2.3.4")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("Access denied"))
	}))
	defer srv.Close()
	cfg := testCfg(t, srv.URL, ipSrv.URL)
	if code := runOnce(context.Background(), cfg, testLogger(), true); code != 1 {
		t.Fatalf("code = %d, esperava 1", code)
	}
}

// RED: sem permissão cai para pasta local em vez de só stderr.
func TestOpenLoggerFallback(t *testing.T) {
	logger, actual := openLogger("/proc/dnsupdater-test-inexistente.log", false)
	defer logger.Close()
	if actual == "" || actual == "/proc/dnsupdater-test-inexistente.log" {
		t.Fatalf("actual = %q, esperava fallback local", actual)
	}
	logger.Infof("fallback ok")
	if _, err := os.Stat(actual); err != nil {
		t.Fatalf("ficheiro fallback ausente: %v", err)
	}
	_ = os.Remove(actual)
}

// RED: log em path absoluto cria link ./dnsupdater.log -> alvo.
func TestOpenLoggerSystemLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "var.log")
	t.Chdir(dir)
	logger, actual := openLogger(target, false)
	defer logger.Close()
	if actual != target {
		t.Fatalf("actual = %q", actual)
	}
	link := filepath.Join(dir, "dnsupdater.log")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("link ausente: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("esperava symlink local -> log em /var")
	}
	_ = os.Remove(link)
	// Limpa eventual link criado no exe-dir do binário de teste.
	if exe, err := os.Executable(); err == nil {
		if l := filepath.Join(filepath.Dir(exe), "dnsupdater.log"); l != link {
			if fi, err := os.Lstat(l); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(l)
			}
		}
	}
}

var _ = os.Getenv // evita import não usado se editar
