#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCATION_PILOT_ENV="${REPO_ROOT}/.local/secrets/amazon-location/pilot.env"
if [[ -r "$LOCATION_PILOT_ENV" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$LOCATION_PILOT_ENV"
  set +a
fi
FRONTEND_DIR="${REPO_ROOT}/frontend"
RUN_DIR="${REPO_ROOT}/.local/run"
LOG_DIR="${REPO_ROOT}/.local/logs"
ACTIVE_NEXT_FILE="${RUN_DIR}/active-next-standalone"
PREVIEW_LOG="${LOG_DIR}/next-preview-build.log"
RESTART_LOG="${LOG_DIR}/restart-local.log"
PUBLIC_ORIGIN="https://prueba-mercadopago.testingpage.store"
LOCAL_ONLY="${ESPACIGO_LOCAL_ONLY:-0}"
if [[ "$LOCAL_ONLY" == 1 ]]; then
  APP_ORIGIN="http://127.0.0.1:3000"
else
  APP_ORIGIN="$PUBLIC_ORIGIN"
fi
API_HEALTH="http://127.0.0.1:8080/health/ready"
WEB_HEALTH="http://127.0.0.1:3000/api/ready"
BUILD_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
DIST_DIR=".next-preview-${BUILD_ID}"
DIST_PATH="${FRONTEND_DIR}/${DIST_DIR}"
STANDALONE_DIR="${DIST_PATH}/standalone"
REUSE_BUILD=0
PREVIOUS_STANDALONE=""
if [[ -f "$ACTIVE_NEXT_FILE" ]]; then
  IFS= read -r PREVIOUS_STANDALONE <"$ACTIVE_NEXT_FILE" || true
  if [[ "$PREVIOUS_STANDALONE" != "$FRONTEND_DIR"/.next-preview-*/standalone || ! -f "$PREVIOUS_STANDALONE/server.js" ]]; then
    echo "ERROR: el registro del servidor Next activo no apunta a una compilación local válida."
    exit 1
  fi
fi
if [[ -n "${ESPACIGO_NEXT_STANDALONE_DIR:-}" ]]; then
  STANDALONE_DIR="$(realpath "${ESPACIGO_NEXT_STANDALONE_DIR}")"
  DIST_PATH="$(dirname "$STANDALONE_DIR")"
  DIST_DIR="$(basename "$DIST_PATH")"
  if [[ "$STANDALONE_DIR" != "$FRONTEND_DIR"/.next-preview-*/standalone || ! -f "$STANDALONE_DIR/server.js" || ! -d "$STANDALONE_DIR/.next/static" ]]; then
    echo "ERROR: el servidor standalone indicado no es un artefacto de prueba aislado válido."
    exit 1
  fi
  REUSE_BUILD=1
fi
PREVIEW_PORT=""
PREVIEW_STARTED=0
CONFIG_BACKUP_DIR=""
CONFIG_BACKUP_ACTIVE=0

mkdir -p "$RUN_DIR" "$LOG_DIR"
umask 077
chmod 700 "$RUN_DIR" "$LOG_DIR"
exec >>"$RESTART_LOG" 2>&1

if ! command -v flock >/dev/null 2>&1; then
  echo "ERROR: flock no está disponible para serializar los reinicios locales."
  exit 1
fi
exec 9>"${RUN_DIR}/start.lock"
if ! flock -n 9; then
  echo "ERROR: hay otro inicio o reinicio local en curso."
  exit 1
fi

if [[ "$LOCAL_ONLY" == 1 ]]; then
  export MERCADOPAGO_CHECKOUT_ENABLED=0
  export MERCADOPAGO_WALLET_PURCHASE_ENABLED=0
  export MERCADOPAGO_PAYMENTS_API_ENABLED=0
  export MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY=0
  export MERCADOPAGO_WEBHOOK_ENABLED=0
  export NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED=0
  export NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED=0
else
  export MERCADOPAGO_CHECKOUT_ENABLED="${MERCADOPAGO_CHECKOUT_ENABLED:-1}"
  export MERCADOPAGO_WALLET_PURCHASE_ENABLED="${MERCADOPAGO_WALLET_PURCHASE_ENABLED:-1}"
  export MERCADOPAGO_PAYMENTS_API_ENABLED="${MERCADOPAGO_PAYMENTS_API_ENABLED:-1}"
  export MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY="${MERCADOPAGO_CHECKOUT_API_SELLER_PUBLIC_KEY:-1}"
  export MERCADOPAGO_WEBHOOK_ENABLED="${MERCADOPAGO_WEBHOOK_ENABLED:-1}"
  export NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED="${NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED:-1}"
  export NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED="${NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED:-1}"
fi
export MERCADOPAGO_TEST_MODE=true

http_ready() {
  curl --silent --fail --max-time 3 "$1" >/dev/null 2>&1
}

wait_http() {
  local url="$1" attempt
  for attempt in $(seq 1 60); do
    http_ready "$url" && return 0
    sleep 1
  done
  return 1
}

port_open() {
  python3 - "$1" <<'PY'
import socket, sys
s = socket.socket()
s.settimeout(0.4)
try:
    s.connect(("127.0.0.1", int(sys.argv[1])))
except OSError:
    raise SystemExit(1)
finally:
    s.close()
PY
}

stop_service() {
  python3 "${REPO_ROOT}/scripts/stop-local-service-group.py" "$1" "$2" "$REPO_ROOT"
}

stop_preview() {
  if [[ "$PREVIEW_STARTED" == 1 && -n "$PREVIEW_PORT" ]]; then
    stop_service "$PREVIEW_PORT" Web
    PREVIEW_STARTED=0
  fi
}

restore_previous_services() {
  stop_service 3000 Web || true
  stop_service 8080 API || true
  if [[ -n "$PREVIOUS_STANDALONE" ]]; then
    ESPACIGO_NEXT_STANDALONE_DIR="$PREVIOUS_STANDALONE" ESPACIGO_START_LOCK_HELD=1 \
      "${REPO_ROOT}/scripts/iniciar-local-fondo.sh" || true
  else
    env -u ESPACIGO_NEXT_STANDALONE_DIR ESPACIGO_START_LOCK_HELD=1 \
      "${REPO_ROOT}/scripts/iniciar-local-fondo.sh" || true
  fi
}

restore_generated_config() {
  if [[ "$CONFIG_BACKUP_ACTIVE" == 1 ]]; then
    cp -p "${CONFIG_BACKUP_DIR}/tsconfig.json" "${FRONTEND_DIR}/tsconfig.json"
    cp -p "${CONFIG_BACKUP_DIR}/next-env.d.ts" "${FRONTEND_DIR}/next-env.d.ts"
    CONFIG_BACKUP_ACTIVE=0
  fi
}

cleanup() {
  stop_preview || true
  restore_generated_config
  if [[ "$REUSE_BUILD" == 0 && -d "$DIST_PATH" && ! -f "${DIST_PATH}/BUILD_READY" ]]; then
    rm -rf -- "$DIST_PATH"
  fi
  if [[ -n "$CONFIG_BACKUP_DIR" ]]; then
    rm -rf -- "$CONFIG_BACKUP_DIR"
  fi
}
trap cleanup EXIT

if ! command -v pnpm >/dev/null 2>&1; then
  echo "ERROR: pnpm no está disponible para compilar el servidor de prueba."
  exit 1
fi

for candidate in $(seq 3301 3310); do
  if ! port_open "$candidate"; then
    PREVIEW_PORT="$candidate"
    break
  fi
done
if [[ -z "$PREVIEW_PORT" ]]; then
  echo "ERROR: no hay un puerto libre entre 3301 y 3310 para probar Next compilado."
  exit 1
fi

if [[ "$REUSE_BUILD" == 0 ]]; then
  echo "Compilando una copia aislada de Next en ${DIST_DIR}; el sitio activo sigue atendiendo tráfico."
  CONFIG_BACKUP_DIR="$(mktemp -d "${RUN_DIR}/preview-config.XXXXXX")"
  cp -p "${FRONTEND_DIR}/tsconfig.json" "${CONFIG_BACKUP_DIR}/tsconfig.json"
  cp -p "${FRONTEND_DIR}/next-env.d.ts" "${CONFIG_BACKUP_DIR}/next-env.d.ts"
  CONFIG_BACKUP_ACTIVE=1
  if ! (
    cd "$FRONTEND_DIR"
    ESPACIGO_NEXT_DIST_DIR="$DIST_DIR" \
      NEXT_PUBLIC_MP_WALLET_PURCHASE_ENABLED=1 \
      NEXT_PUBLIC_MP_CHECKOUT_API_TEST_ENABLED=1 \
      pnpm exec next build --webpack
  ) >>"$PREVIEW_LOG" 2>&1; then
    echo "ERROR: la compilación aislada falló; se mantienen los servidores activos. Revisa ${PREVIEW_LOG}."
    exit 1
  fi
  restore_generated_config

  if [[ ! -f "${STANDALONE_DIR}/server.js" || ! -d "${DIST_PATH}/static" ]]; then
    echo "ERROR: la compilación no produjo el servidor standalone esperado."
    exit 1
  fi
  mkdir -p "${STANDALONE_DIR}/public"
  cp -a "${FRONTEND_DIR}/public/." "${STANDALONE_DIR}/public/"
else
  echo "Usando la compilación standalone aislada ${DIST_DIR}; se validará antes del corte."
fi

# Next resolves assets from <standalone>/<distDir>/static and indexes them at
# process startup. Copy them before launching every candidate server.
if [[ ! -d "${DIST_PATH}/static" ]]; then
  echo "ERROR: no se encontró el directorio static del build ${DIST_PATH}."
  exit 1
fi
mkdir -p "${STANDALONE_DIR}/${DIST_DIR}"
rm -rf -- "${STANDALONE_DIR}/${DIST_DIR}/static"
cp -a "${DIST_PATH}/static" "${STANDALONE_DIR}/${DIST_DIR}/static"

(
  cd "$STANDALONE_DIR"
  nohup setsid -f env \
    NODE_ENV=production \
    HOSTNAME=127.0.0.1 \
    PORT="$PREVIEW_PORT" \
    API_BACKEND_BASE_URL=http://127.0.0.1:8080/api/v1 \
    FRONTEND_PUBLIC_ORIGIN="$APP_ORIGIN" \
    node server.js 9>&- >>"${LOG_DIR}/next-preview.log" 2>&1 </dev/null &
)
PREVIEW_STARTED=1

if ! wait_http "http://127.0.0.1:${PREVIEW_PORT}/api/ready"; then
  echo "ERROR: Next compilado no alcanzó health; el sitio actual sigue intacto. Revisa ${LOG_DIR}/next-preview.log."
  exit 1
fi
if ! node "${REPO_ROOT}/scripts/smoke-local-preview.mjs" "http://127.0.0.1:${PREVIEW_PORT}/acceso"; then
  echo "ERROR: Next compilado falló el smoke de navegador; el sitio actual sigue intacto."
  exit 1
fi
stop_preview

echo "La compilación pasó health y smoke de navegador. Reiniciando Go y Next del checkout local."
stop_service 3000 Web
stop_service 8080 API
if port_open 3000 || port_open 8080; then
  echo "ERROR: un puerto de EspaciGo sigue ocupado; no iniciaré procesos duplicados."
  exit 1
fi

if ! ESPACIGO_NEXT_STANDALONE_DIR="$STANDALONE_DIR" ESPACIGO_LOCAL_ONLY="$LOCAL_ONLY" ESPACIGO_START_LOCK_HELD=1 \
  "${REPO_ROOT}/scripts/iniciar-local-fondo.sh"; then
  echo "ERROR: el inicio standalone falló; intentaré restaurar el modo Next anterior de EspaciGo."
  restore_previous_services
  exit 1
fi

for url in "$API_HEALTH" "$WEB_HEALTH"; do
  if ! wait_http "$url"; then
    echo "ERROR: la URL no alcanzó health tras el reinicio: ${url}"
    restore_previous_services
    exit 1
  fi
done
if [[ "$LOCAL_ONLY" != 1 ]]; then
  if ! wait_http "${PUBLIC_ORIGIN}/api/ready"; then
    echo "ERROR: el túnel público no responde; Next y Go locales permanecen activos."
    exit 1
  fi
  if ! node "${REPO_ROOT}/scripts/smoke-local-preview.mjs" "${PUBLIC_ORIGIN}/acceso"; then
    echo "ERROR: el navegador detectó inestabilidad; restaurando el modo Next anterior."
    restore_previous_services
    exit 1
  fi
else
  echo "Modo local-only: omití comprobaciones del túnel público."
fi

touch "${DIST_PATH}/BUILD_READY"
pointer_tmp="${ACTIVE_NEXT_FILE}.tmp.$$"
printf '%s\n' "$STANDALONE_DIR" >"$pointer_tmp"
chmod 600 "$pointer_tmp"
mv -f -- "$pointer_tmp" "$ACTIVE_NEXT_FILE"
if [[ "$LOCAL_ONLY" == 1 ]]; then
  echo "Listo. Go y Next compilado están activos en localhost; no se consultó la URL pública."
else
  echo "Listo. Go y Next compilado están activos, y el acceso público pasó el smoke de navegador."
fi
