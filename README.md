# Dynamic DNS Updater for cPanel (Go)

> **Nota:** a partir da `v1.0.0` o programa foi **refatorado de Bash para Go**
> — já não é apenas Bash. O binário `dnsupdater` é o atualizador; o
> `update_ip.sh` é só um shim compatível (compila e delega) e o Bash original
> está preservado em `update_ip.bash.legacy`.

Single static binary that keeps a cPanel DNS `A` record in sync with your
public IP. No `curl`/`jq`/`dig` needed at runtime — just the binary and a
`.env` file.

## How it works

1. Gets the public IP trying each provider in order (15 known-good plaintext
   endpoints: amazon, ifconfig.me, icanhazip, ipify, ident.me, ifconfig.co,
   ipinfo, wtfismyip, ipecho, l2.io, myexternalip, ifconfig.io…;
   `{"ip":"..."}` JSON answers are accepted too), first valid answer wins.
2. Short-circuit: if the IP is unchanged since the last successful sync
   (see `state.json`), it exits without hitting the cPanel API. A periodic
   `force-refresh` (default 1h) re-checks anyway to catch manual edits.
3. Source of truth is always the **cPanel API** (`parse_zone`), never local
   DNS (avoids cache/propagation false positives). If the zone IP differs,
   it calls `mass_edit_zone` (`add` when missing, `edit` otherwise).
4. Lock file prevents overlapping runs; stale locks (>10 min) expire.
5. Optional `ntfy` notification on change/failure (disabled when unconfigured).

## Requirements

- cPanel account with UAPI/DNS access.
- Go 1.24+ **only to build** — or use a prebuilt binary (see below).
- `update_ip.sh` keeps the old cron path working: it compiles `./dnsupdater`
  on first run (or when sources change) and delegates to it.

## Install

```bash
git clone <repo-url>
cd dnsupdater
cp .env.example .env   # edit with your values
```

Build:

```bash
go build -trimpath -ldflags="-s -w" -o dnsupdater ./cmd/dnsupdater
# or
make build
# or simply: ./update_ip.sh --dry-run   # shim builds automatically
```

## Configuration (`.env`)

Canonical names use the `DNSUPDATER_` prefix so they never collide with
system vars (e.g. `USERNAME`, always set on Windows). Legacy bare names
(`CPANEL_URL`, `USERNAME`, …) still work **in the file**; the `update_ip.sh`
shim renames them to `DNSUPDATER_*` in place on first run (backup in
`.env.bak`). The OS environment is only read with the prefix.

| Var | Required | Default | Description |
| --- | --- | --- | --- |
| `DNSUPDATER_CPANEL_URL` | yes | – | e.g. `https://cpanel.example.com` |
| `DNSUPDATER_CPANEL_PORT` | no | `2083` | appended when the URL has no port |
| `DNSUPDATER_USERNAME` / `DNSUPDATER_APIKEY` | yes | – | cPanel UAPI credentials |
| `DNSUPDATER_DOMAIN` / `DNSUPDATER_SUBDOMAIN` | yes | – | `SUBDOMAIN` may be short (`homeip`) or FQDN |
| `DNSUPDATER_TTL` | no | `300` | DNS TTL for add/edit |
| `DNSUPDATER_GET_IP_URLS` | no | 15 tested endpoints (amazon, ifconfig.me, icanhazip, ipify, ident.me, …) | comma/space separated fallback list |
| `DNSUPDATER_NTFY_SERVER` / `DNSUPDATER_NTFY_TOPIC` / `DNSUPDATER_NTFY_TOKEN` | no | – | all three or none (none = disabled) |
| `DNSUPDATER_LOG_FILE` | no | `/var/log/dnsupdater.log` (Unix), `./dnsupdater.log` (Windows) | falls back to local `dnsupdater.log` when unwritable |
| `DNSUPDATER_STATE_FILE` | no | user cache dir | set explicitly for cron-as-root consistency |
| `DNSUPDATER_INTERVAL` | no | `5m` | daemon loop interval |
| `DNSUPDATER_FORCE_REFRESH` | no | `1h` | re-check API even with unchanged IP |
| `DNSUPDATER_DRY_RUN` / `DNSUPDATER_VERBOSE` | no | `false` | log without changing the zone / mirror INFO to stderr |

Precedence per key: CLI flags > `DNSUPDATER_*` OS env > `DNSUPDATER_*` in
`.env` > legacy bare names in `.env` > defaults.

## Usage

```bash
./dnsupdater --once                              # single check (cron)
./dnsupdater --once --dry-run --verbose         # test safely
./dnsupdater --daemon --interval 5m             # long-running loop
./dnsupdater --once --force                     # ignore state cache
./dnsupdater --version
```

Cron stays exactly as before (shim is a drop-in at the same path):

```cron
*/5 * * * * /path/to/dnsupdater/update_ip.sh
```

### Running as a service

**Linux (systemd, recomendado em vez de cron):**

```bash
sudo install -m 0755 dist/dnsupdater-linux-amd64 /usr/local/bin/dnsupdater
sudo mkdir -p /etc/dnsupdater
sudo install -m 0600 .env /etc/dnsupdater/.env
sudo install -m 0644 contrib/dnsupdater.service contrib/dnsupdater.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dnsupdater.timer
systemctl list-timers dnsupdater.timer   # confirma agendamento
journalctl -u dnsupdater.service -e      # últimas runs (detalhe em /var/log/dnsupdater.log)
```

O timer corre `--once` de 5 em 5 min (`OnUnitActiveSec`). Alternativa com
daemon permanente: `ExecStart=/usr/local/bin/dnsupdater --daemon` com
`Restart=always` — o timer acima é preferível (sem processo residente).

**Windows (Task Scheduler, a cada 5 min como SYSTEM):**

```cmd
schtasks /Create /TN "DNSUpdater" /TR "C:\dnsupdater\dnsupdater-windows-amd64.exe --once" /SC MINUTE /MO 5 /RU SYSTEM
schtasks /Run /TN "DNSUpdater"
```

O log usa `./dnsupdater.log` junto ao `.exe` (fallback automático no Windows).

systemd example (`.service` + `.timer` running `--once` every 5 min) and
Windows (`dnsupdater-windows-*.exe --once` via Task Scheduler) follow the
same pattern: one-shot execution, no daemon required.

## Cross-compile

```bash
make build-all   # linux/amd64,arm64,armv7 + windows/amd64,arm64 + darwin/amd64,arm64
```

Binaries + `sha256sums.txt` land in `dist/`. On Raspberry Pi just `git pull`
and run `./update_ip.sh` — it natively compiles for the local arch.

## Logging

- Active log: `LOG_FILE` (default `/var/log/dnsupdater.log`).
- No permission / no `/var` (Windows) → local `./dnsupdater.log` next to the
  binary, with a warning.
- When the log lives under `/var`, a `dnsupdater.log` symlink is created in
  the app dir for convenience (`*.log` is gitignored).
- Rotation at 10 MB (`dnsupdater.log.1`); see `contrib/dnsupdater.logrotate`
  for a `logrotate` alternative (`sudo cp contrib/dnsupdater.logrotate
  /etc/logrotate.d/dnsupdater`).
- First run as non-root: `sudo touch /var/log/dnsupdater.log &&
  sudo chown $USER /var/log/dnsupdater.log`, or set `LOG_FILE` to a writable
  path.

## State & lock

- `state.json` (user cache by default): last public/zone IP + timestamp.
  Delete it or pass `--force` to force a full API check.
- `.lock_update` next to the binary prevents overlaps; remove it manually if
  a stale lock error persists after a crash (auto-expires after 10 min).

## Troubleshooting

| Symptom | Cause / fix |
| --- | --- |
| `parse_zone HTTP 403: Access denied` | API key invalid/expired, token without DNS privileges, IP not allowlisted (Host Access Control/cPHulk), **or wrong user**: a system `USERNAME` env var used to leak into auth — fixed, only `DNSUPDATER_*` env / `.env` are read |
| `todos os providers falharam` | no outbound HTTP; check proxy/firewall (uses `HTTP(S)_PROXY`) |
| `locked (... existe ...)` | previous run crashed; lock auto-expires in 10 min or delete `.lock_update` |
| `ntfy parcial` | set all three `NTFY_*` or none |
| shim says `go build falhou` | install Go or copy a `dist/` binary to `./dnsupdater` |

## Legacy

The original Bash implementation is kept at `update_ip.bash.legacy` for
reference. `update_ip.sh` is now a compatibility shim (same path, same
`.env`), not the updater itself.

## Development

```bash
go vet ./...
go test ./... -count=1
go test ./... -cover
```
