// Command dnsupdater keeps a cPanel DNS A record in sync with the public IP.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"dnsupdater/internal/config"
	"dnsupdater/internal/cpanel"
	"dnsupdater/internal/ip"
	"dnsupdater/internal/lock"
	"dnsupdater/internal/logx"
	"dnsupdater/internal/notify"
	"dnsupdater/internal/state"
)

var version = "dev"

func main() {
	var (
		configPath   = flag.String("config", "", "caminho do .env (default: ./.env ou dir do binário)")
		daemon       = flag.Bool("daemon", false, "loop contínuo com --interval em vez de run único")
		once         = flag.Bool("once", false, "run único (default quando sem --daemon)")
		interval     = flag.Duration("interval", 0, "intervalo do daemon (ex: 5m, default 5m)")
		forceRefresh = flag.Duration("force-refresh", 0, "força parse_zone mesmo com IP igual (ex: 1h, default 1h)")
		force        = flag.Bool("force", false, "ignora cache state.json e consulta sempre a API")
		logFile      = flag.String("log-file", "", "ficheiro de log (default /var/log/dnsupdater.log no Unix, ./dnsupdater.log no Windows; cai para pasta local sem permissão)")
		stateFile    = flag.String("state-file", "", "ficheiro de estado (default cache do user)")
		dryRun       = flag.Bool("dry-run", false, "não altera a zona, só loga")
		verbose      = flag.Bool("verbose", false, "espelha INFO no stderr")
		showVersion  = flag.Bool("version", false, "mostra versão")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("dnsupdater %s\n", version)
		return
	}

	ov := config.Overrides{
		ConfigPath:   *configPath,
		LogFile:      *logFile,
		StateFile:    *stateFile,
		Interval:     *interval,
		ForceRefresh: *forceRefresh,
		DryRun:       *dryRun,
		DryRunSet:    wasSet("dry-run"),
		Verbose:      *verbose,
	}
	cfg, err := config.Load(ov)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dnsupdater: config: %v\n", err)
		os.Exit(1)
	}

	logger, _ := openLogger(cfg.LogFile, cfg.Verbose)
	defer logger.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	lockPath := defaultLockPath()
	release, err := lock.Acquire(lockPath, 10*time.Minute)
	if err != nil {
		logger.Errorf("lock: %v", err)
		_ = logger.Close()
		os.Exit(1)
	}

	if *daemon && !*once {
		// Daemon: defers run on return, lock held for lifetime.
		defer release()
		logger.Infof(" arranque daemon version=%s interval=%s force-refresh=%s fqdn=%s", version, cfg.Interval, cfg.ForceRefresh, cfg.FQDN())
		t := time.NewTicker(cfg.Interval)
		defer t.Stop()
		if code := runOnce(ctx, cfg, logger, *force); code != 0 && ctx.Err() == nil {
			logger.Errorf(" primeira execução falhou (continua daemon)")
		}
		for {
			select {
			case <-ctx.Done():
				logger.Infof(" shutdown")
				return
			case <-t.C:
				_ = runOnce(ctx, cfg, logger, *force)
			}
		}
	}

	// NOTE: os.Exit skips defers, so release/close explicitly.
	code := runOnce(ctx, cfg, logger, *force)
	release()
	_ = logger.Close()
	os.Exit(code)
}

func runOnce(ctx context.Context, cfg *config.Config, logger *logx.Logger, force bool) int {
	start := time.Now()
	logger.Infof(" check fqdn=%s domain=%s", cfg.FQDN(), cfg.Domain)

	publicIP, provider, err := ip.GetPublicIP(ctx, cfg.GetIPURLs, 5*time.Second)
	if err != nil {
		logger.Errorf(" ip público: %v", err)
		_ = notify.Send(ctx, cfg.NtfyServer, cfg.NtfyTopic, cfg.NtfyToken,
			"DNS updater falhou", "max", "warning", fmt.Sprintf("Falha a obter IP público: %v", err))
		return 1
	}
	logger.Infof(" ip público=%s via %s", publicIP, provider)

	if !force {
		st, err := state.Load(cfg.StateFile)
		if err != nil {
			logger.Warnf(" state: %v", err)
		} else if st.Fresh(publicIP, cfg.ForceRefresh) {
			logger.Infof(" skip: IP igual ao último sync (%s há %s), sem bater na API",
				publicIP, time.Since(st.LastCheck).Round(time.Second))
			return 0
		}
	}

	cp, err := cpanel.New(cfg.CpanelURL, cfg.CpanelPort, cfg.Username, cfg.APIKey)
	if err != nil {
		logger.Errorf(" cpanel client: %v", err)
		return 1
	}

	ctxZone, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	records, err := cp.ParseZone(ctxZone, cfg.Domain)
	if err != nil {
		logger.Errorf(" parse_zone: %v", err)
		_ = notify.Send(ctx, cfg.NtfyServer, cfg.NtfyTopic, cfg.NtfyToken,
			"cPanel inacessível", "max", "no_entry", fmt.Sprintf("parse_zone falhou: %v", err))
		return 1
	}

	serial, err := cpanel.Serial(records)
	if err != nil {
		logger.Errorf(" serial SOA: %v", err)
		return 1
	}

	found, err := cpanel.FindRecord(records, cfg.Subdomain, cfg.Domain)
	fqdn := cfg.FQDN()
	if err != nil {
		if !cpanel.IsNotFound(err) {
			logger.Errorf(" find_record: %v", err)
			return 1
		}
		// ADD path.
		logger.Infof(" registro A não existe, vai adicionar %s -> %s", fqdn, publicIP)
		if cfg.DryRun {
			logger.Infof(" dry-run: add %s ttl=%d ip=%s serial=%s (sem alterar)", fqdn, cfg.TTL, publicIP, serial)
			return 0
		}
		payload := cpanel.AddPayload(fqdn, cfg.TTL, publicIP)
		if err := cp.MassEdit(ctxZone, cfg.Domain, serial, "add", payload); err != nil {
			logger.Errorf(" mass_edit add: %v", err)
			return 1
		}
		saveState(cfg.StateFile, publicIP, publicIP, logger)
		msg := fmt.Sprintf("Subdomain: %s IP: %s (%s)", fqdn, publicIP, time.Since(start).Round(time.Second))
		logger.Infof(" adicionado %s", msg)
		_ = notify.Send(ctx, cfg.NtfyServer, cfg.NtfyTopic, cfg.NtfyToken,
			"New subdomain added", "high", "heavy_plus_sign", msg)
		return 0
	}

	// EDIT path.
	if found.IP == publicIP {
		logger.Infof(" sem mudança: %s já é %s (line %d)", fqdn, publicIP, found.LineIndex)
		saveState(cfg.StateFile, publicIP, found.IP, logger)
		return 0
	}
	logger.Infof(" mudança: %s API=%s público=%s (line %d)", fqdn, found.IP, publicIP, found.LineIndex)
	if cfg.DryRun {
		logger.Infof(" dry-run: edit %s ttl=%d %s -> %s serial=%s (sem alterar)",
			fqdn, cfg.TTL, found.IP, publicIP, serial)
		return 0
	}
	payload := cpanel.EditPayload(found.LineIndex, found.Dname, cfg.TTL, found.RecordType, publicIP)
	if payload["dname"] == "" {
		payload["dname"] = fqdn
	}
	if err := cp.MassEdit(ctxZone, cfg.Domain, serial, "edit", payload); err != nil {
		logger.Errorf(" mass_edit edit: %v", err)
		return 1
	}
	saveState(cfg.StateFile, publicIP, publicIP, logger)
	msg := fmt.Sprintf("Subdomain: %s %s -> %s (%s)", fqdn, found.IP, publicIP, time.Since(start).Round(time.Second))
	logger.Infof(" atualizado %s", msg)
	_ = notify.Send(ctx, cfg.NtfyServer, cfg.NtfyTopic, cfg.NtfyToken,
		"Subdomain updated", "high", "up", msg)
	return 0
}

func saveState(path, publicIP, zoneIP string, logger *logx.Logger) {
	st := &state.State{PublicIP: publicIP, ZoneIP: zoneIP, InSync: true, LastCheck: time.Now()}
	if err := st.Save(path); err != nil {
		logger.Warnf(" state save: %v", err)
	}
}

func defaultLockPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".lock_update")
	}
	return ".lock_update"
}

// openLogger tenta o path principal e cai para pasta local quando não há
// permissão ou o dir não existe (caso Windows). Quando o log fica em /var,
// cria link ./dnsupdater.log -> alvo (best-effort, *.log está gitignored).
// Devolve logger + path efetivo.
func openLogger(primary string, verbose bool) (*logx.Logger, string) {
	if logger, err := logx.New(primary, verbose); err == nil {
		if logx.IsSystemLogPath(primary) {
			_ = logx.EnsureLocalLink(primary)
		}
		if logger == nil {
			logger, _ = logx.New("", true)
		}
		return logger, primary
	} else {
		fallback := logx.LocalFallbackPath()
		if fallback != primary {
			if logger2, err2 := logx.New(fallback, verbose); err2 == nil {
				fmt.Fprintf(os.Stderr, "dnsupdater: aviso: log %s: %v (a usar %s)\n", primary, err, fallback)
				logger2.Warnf("fallback: %s indisponível (%v), a usar %s", primary, err, fallback)
				return logger2, fallback
			}
		}
		fmt.Fprintf(os.Stderr, "dnsupdater: aviso: log %s: %v (a usar stderr)\n", primary, err)
		logger, _ := logx.New("", true)
		if logger == nil {
			// Nunca nil na prática; guarda contra regressão.
			panic("logx.New retornou nil")
		}
		return logger, ""
	}
}

func wasSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
