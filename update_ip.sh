#!/usr/bin/env bash
# Shim de compatibilidade: mantém o mesmo caminho/cron (.env) dos clientes atuais.
# Na primeira execução (ou quando o fonte Go mudou) compila o binário nativo
# e depois delega tudo para ele: ./dnsupdater --once "$@"
# Lógica bash antiga guardada em update_ip.bash.legacy
set -u

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$DIR/dnsupdater"
SRC_DIR="$DIR/cmd/dnsupdater"

log_line() {
    local msg="$1"
    local logfile="$2"
    local line
    line="$(date -u +%FT%TZ) [SHIM] $msg"
    if [ -n "$logfile" ]; then
        mkdir -p "$(dirname "$logfile")" 2>/dev/null
        echo "$line" >> "$logfile" 2>/dev/null || echo "$line" >&2
    else
        echo "$line" >&2
    fi
}

# LOG_FILE: flag --log-file tem prioridade, depois .env, senão default.
LOGFILE="/var/log/dnsupdater.log"
for _a in "$@"; do
    case "$_a" in
        --log-file=*) LOGFILE="${_a#--log-file=}"; break ;;
    esac
done
if [ "$LOGFILE" = "/var/log/dnsupdater.log" ]; then
    prev=""
    for _a in "$@"; do
        if [ "$prev" = "--log-file" ]; then LOGFILE="$_a"; break; fi
        prev="$_a"
    done
fi
if [ -f "$DIR/.env" ] && [ "$LOGFILE" = "/var/log/dnsupdater.log" ]; then
    # shellcheck disable=SC1091
    parsed="$(grep -E '^[[:space:]]*(export[[:space:]]+)?LOG_FILE=' "$DIR/.env" 2>/dev/null | tail -n1 | cut -d= -f2-)"
    parsed="$(echo "$parsed" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"
    if [ -n "${parsed:-}" ]; then
        LOGFILE="$parsed"
    fi
fi

# Default no Windows (sem /var): pasta local.
case "$(uname -s 2>/dev/null)" in
    MINGW*|MSYS*|CYGWIN*|Windows*) [ "$LOGFILE" = "/var/log/dnsupdater.log" ] && LOGFILE="$DIR/dnsupdater.log" ;;
esac

# Fallback: sem permissão ou dir inexistente -> pasta local.
# Se ficar em /var, cria link ./dnsupdater.log -> alvo (*.log está gitignored).
case "$LOGFILE" in
    /*|[A-Za-z]:*)
        if ! mkdir -p "$(dirname "$LOGFILE")" 2>/dev/null || ! touch "$LOGFILE" 2>/dev/null; then
            echo "$(date -u +%FT%TZ) [SHIM] aviso: $LOGFILE sem escrita, a usar $DIR/dnsupdater.log" >&2
            LOGFILE="$DIR/dnsupdater.log"
        else
            LINK="$DIR/dnsupdater.log"
            if [ "$LINK" != "$LOGFILE" ] && [ ! -e "$LINK" ] && [ ! -L "$LINK" ]; then
                ln -s "$LOGFILE" "$LINK" 2>/dev/null || true
            fi
        fi
        ;;
esac

NEED_BUILD=0
if [ ! -x "$BIN" ]; then
    NEED_BUILD=1
else
    # Recompila se qualquer fonte for mais novo que o binário.
    if [ -f "$DIR/go.mod" ] && [ "$DIR/go.mod" -nt "$BIN" ]; then
        NEED_BUILD=1
    elif [ -d "$SRC_DIR" ] && [ -n "$(find "$DIR/cmd" "$DIR/internal" "$DIR/go.mod" -newer "$BIN" -print -quit 2>/dev/null)" ]; then
        NEED_BUILD=1
    fi
fi

if [ "$NEED_BUILD" -eq 1 ]; then
    if ! command -v go >/dev/null 2>&1; then
        log_line "ERRO: binário ausente e 'go' não instalado. Instala golang (apt install golang-go) ou copia o binário pré-compilado de dist/ para $BIN" "$LOGFILE"
        echo "dnsupdater: 'go' não encontrado e $BIN ausente" >&2
        exit 1
    fi
    log_line "a compilar $BIN (go build)..." "$LOGFILE"
    BUILD_TMP="$DIR/.go-build.log.tmp"
    if (cd "$DIR" && go build -trimpath -ldflags="-s -w" -o "$BIN" ./cmd/dnsupdater) >"$BUILD_TMP" 2>&1; then
        # Espelha output do build para o log principal (best-effort, sem falhar se sem permissão).
        cat "$BUILD_TMP" >>"$LOGFILE" 2>/dev/null || cat "$BUILD_TMP" >&2
        rm -f "$BUILD_TMP"
        chmod +x "$BIN" 2>/dev/null || true
        log_line "compilação OK" "$LOGFILE"
    else
        rc=$?
        cat "$BUILD_TMP" >>"$LOGFILE" 2>/dev/null || cat "$BUILD_TMP" >&2
        rm -f "$BUILD_TMP"
        log_line "ERRO: go build falhou (rc=$rc)" "$LOGFILE"
        echo "dnsupdater: go build falhou" >&2
        exit 1
    fi
fi

exec "$BIN" --once "$@"
