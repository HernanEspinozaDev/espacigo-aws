#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_DEV="${REPO_ROOT}/scripts/local-dev.sh"
PATH="/usr/local/go/bin:${HOME}/go/bin:${HOME}/.hermes/tools/pnpm-12.15.0-linux-x64:${HOME}/.hermes/tools/node-26.12.1-linux-x64/bin:${PATH}"
export PATH
RUN_DIR="${REPO_ROOT}/.local/run"
LOG_DIR="${REPO_ROOT}/.local/logs"
DB_CONTAINER="espacigo_localbackend_20261024-database-1"
DB_PORT="15432"
DB_NAME="espacigo_aws_local"
PUBLIC_ORIGIN="https://prueba-mercadopago.testingpage.store"
API_HEALTH="http://127.0.0.1:8080/health/ready"
WEB_HEALTH="http://127.0.0.1:3000/api/ready"

mkdir -p "$RUN_DIR" "$LOG_DIR"
umask 077
chmod 700 "$RUN_DIR" "$LOG_DIR"

http_ready() {
  curl --silent --fail --max-time 2 "$1" >/dev/null 2>&1
}

tcp_open() {
  python3 - "$1" <<'PY'
import socket, sys
s=socket.socket(); s.settimeout(0.5)
try: s.connect(("127.0.0.1", int(sys.argv[1])))
except OSError: raise SystemExit(1)
finally: s.close()
PY
}

if [[ "${1:-}" == status ]]; then
  for name in API Web; do
    if [[ "$name" == API ]]; then port=8080; url="$API_HEALTH"; else port=3000; url="$WEB_HEALTH"; fi
    if http_ready "$url"; then state=healthy; else state=down; fi
    pids="$(fuser -n tcp "$port" 2>/dev/null || true)"
    printf '%s state=%s port=%s pids=%s\n' "$name" "$state" "$port" "${pids:-none}"
  done
  if command -v pg_isready >/dev/null 2>&1 && pg_isready -q -h 127.0.0.1 -p "$DB_PORT" -d "$DB_NAME"; then db_state=ready; else db_state=unavailable; fi
  if http_ready "${PUBLIC_ORIGIN}/api/ready"; then tunnel_state=healthy; else tunnel_state=unavailable; fi
  printf 'Database forward: %s\nCloudflare HTTPS: %s\nLogs: %s\n' "$db_state" "$tunnel_state" "$LOG_DIR"
  exit 0
fi

exec >>"${LOG_DIR}/launcher.log" 2>&1
if ! command -v flock >/dev/null 2>&1; then
  echo "ERROR: flock no está disponible para evitar lanzamientos concurrentes."
  exit 1
fi
if [[ "${ESPACIGO_START_LOCK_HELD:-0}" != 1 ]]; then
  exec 9>"${RUN_DIR}/start.lock"
  if ! flock -n 9; then
    echo "Ya hay otro inicio local en curso; no lanzaré instancias adicionales."
    exit 0
  fi
fi

wait_http() {
  local url="$1" attempt
  for attempt in $(seq 1 60); do
    http_ready "$url" && return 0
    sleep 1
  done
  return 1
}

record_listener_pid() {
  local name="$1" port="$2" pids
  command -v fuser >/dev/null 2>&1 || return 0
  pids="$(fuser -n tcp "$port" 2>/dev/null | xargs || true)"
  [[ -n "$pids" ]] || return 0
  printf '%s\n' "$pids" >"${RUN_DIR}/${name}.pid"
  chmod 600 "${RUN_DIR}/${name}.pid"
}

ensure_db_forward() {
  if command -v pg_isready >/dev/null 2>&1 && pg_isready -q -h 127.0.0.1 -p "$DB_PORT" -d "$DB_NAME"; then
    echo "PostgreSQL loopback ya disponible en 127.0.0.1:${DB_PORT}."
    return 0
  fi
  if tcp_open "$DB_PORT"; then
    echo "ERROR: 127.0.0.1:${DB_PORT} está ocupado, pero no responde como PostgreSQL; no modificaré ese listener." >&2
    return 1
  fi
  if ! command -v docker >/dev/null 2>&1 || ! command -v socat >/dev/null 2>&1 || ! command -v setsid >/dev/null 2>&1; then
    echo "ERROR: se requieren Docker, socat y setsid para recuperar el forward PostgreSQL local." >&2
    return 1
  fi
  local state db_ip
  state="$(docker inspect -f '{{.State.Status}}' "$DB_CONTAINER" 2>/dev/null || true)"
  [[ "$state" == running ]] || { echo "ERROR: PostgreSQL de desarrollo no está activo; no iniciaré otro contenedor." >&2; return 1; }
  db_ip="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$DB_CONTAINER")"
  [[ -n "$db_ip" ]] || { echo "ERROR: no pude obtener la IP del PostgreSQL de desarrollo." >&2; return 1; }
  nohup setsid -f socat "TCP-LISTEN:${DB_PORT},bind=127.0.0.1,reuseaddr,fork" "TCP:${db_ip}:5432" 9>&- >>"${LOG_DIR}/postgres-forward.log" 2>&1 </dev/null &
  for _ in $(seq 1 15); do
    if command -v pg_isready >/dev/null 2>&1 && pg_isready -q -h 127.0.0.1 -p "$DB_PORT" -d "$DB_NAME"; then
      record_listener_pid postgres-forward "$DB_PORT"
      echo "Forward PostgreSQL listo en 127.0.0.1:${DB_PORT}."
      return 0
    fi
    sleep 1
  done
  echo "ERROR: el forward PostgreSQL no quedó listo; revisa ${LOG_DIR}/postgres-forward.log." >&2
  return 1
}

start_service() {
  local name="$1" port="$2" url="$3" log="$4"; shift 4
  local -a service_env=()
  while [[ $# -gt 0 && "$1" != -- ]]; do service_env+=("$1"); shift; done
  [[ $# -gt 0 ]] && shift
  if http_ready "$url"; then
    record_listener_pid "$name" "$port"
    echo "${name}: ya está activo (${url})."
    return 0
  fi
  if tcp_open "$port"; then
    echo "${name}: el puerto ${port} ya está ocupado; esperaré el health check sin iniciar otro proceso."
    if wait_http "$url"; then return 0; fi
    echo "ERROR: el listener existente en ${port} no alcanzó health; no lo detuve ni reemplacé." >&2
    return 1
  fi
  if ! command -v setsid >/dev/null 2>&1; then
    echo "ERROR: setsid no está disponible para desacoplar ${name} de esta sesión." >&2
    return 1
  fi
  nohup setsid -f env "${service_env[@]}" "$@" 9>&- >>"${LOG_DIR}/${log}" 2>&1 </dev/null &
  local pid=$!
  echo "$pid" >"${RUN_DIR}/${name}.pid"
  chmod 600 "${RUN_DIR}/${name}.pid"
  if wait_http "$url"; then
    record_listener_pid "$name" "$port"
    echo "${name}: iniciado en segundo plano; log ${LOG_DIR}/${log}."
    return 0
  fi
  echo "ERROR: ${name} no alcanzó health; log disponible en ${LOG_DIR}/${log}." >&2
  return 1
}

ensure_db_forward

start_service API 8080 "$API_HEALTH" api.log \
  HTTP_ADDR=127.0.0.1:8080 \
  FRONTEND_PUBLIC_ORIGIN="$PUBLIC_ORIGIN" \
  CORS_ALLOWED_ORIGINS="$PUBLIC_ORIGIN,http://127.0.0.1:3000,http://localhost:3000" \
  COMMERCIAL_RENTAL_FLOW_ENABLED=1 \
  MERCADOPAGO_CHECKOUT_ENABLED="${MERCADOPAGO_CHECKOUT_ENABLED:-0}" \
  MERCADOPAGO_WEBHOOK_ENABLED="${MERCADOPAGO_WEBHOOK_ENABLED:-0}" \
  MERCADOPAGO_PAYMENTS_API_ENABLED="${MERCADOPAGO_PAYMENTS_API_ENABLED:-0}" \
  MERCADOPAGO_RETURN_ORIGIN="${MERCADOPAGO_RETURN_ORIGIN:-$PUBLIC_ORIGIN}" \
  MERCADOPAGO_TEST_MODE=true \
  ESPACIGO_LOCAL_DB_HOST=127.0.0.1 \
  ESPACIGO_LOCAL_DB_PORT="$DB_PORT" \
  -- "${LOCAL_DEV}" api

if [[ -n "${ESPACIGO_NEXT_STANDALONE_DIR:-}" ]]; then
  standalone_dir="$(realpath "${ESPACIGO_NEXT_STANDALONE_DIR}")"
  if [[ ! -f "${standalone_dir}/server.js" ]]; then
    echo "ERROR: no existe el servidor Next compilado en ${standalone_dir}." >&2
    exit 1
  fi
  start_service Web 3000 "$WEB_HEALTH" web.log \
    FRONTEND_PUBLIC_ORIGIN="$PUBLIC_ORIGIN" \
    API_BACKEND_BASE_URL="http://127.0.0.1:8080/api/v1" \
    HOSTNAME=127.0.0.1 \
    PORT=3000 \
    NODE_ENV=production \
    -- bash -c 'cd "$1" && exec node server.js' _ "$standalone_dir"
else
  start_service Web 3000 "$WEB_HEALTH" web.log \
    FRONTEND_PUBLIC_ORIGIN="$PUBLIC_ORIGIN" \
    NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED="${MERCADOPAGO_PAYMENTS_API_ENABLED:-0}" \
    -- "${LOCAL_DEV}" web
fi

if http_ready "${PUBLIC_ORIGIN}/api/ready"; then
  echo "Cloudflare tunnel: HTTPS público responde correctamente."
else
  echo "Cloudflare tunnel: no verificado; API y Web locales siguen activos. No iniciaré ni reemplazaré túneles." >&2
fi

if curl --cacert "${HOME}/.aws/emulator/9297/ca.pem" --silent --fail --max-time 2 https://127.0.0.1:9297/__admin/health >/dev/null 2>&1; then
  echo "aws-emulator: disponible en el perfil local."
else
  echo "aws-emulator: no responde; no iniciaré ni reemplazaré el contenedor ni aprovisionaré recursos." >&2
fi
if http_ready "http://127.0.0.1:8025/api/v1/info"; then
  echo "Mailpit: disponible."
else
  echo "Mailpit: no responde; no iniciaré ni reemplazaré el contenedor." >&2
fi

echo "Listo. Web: ${PUBLIC_ORIGIN} (local http://127.0.0.1:3000) | API: http://127.0.0.1:8080"
