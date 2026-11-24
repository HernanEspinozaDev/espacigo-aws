#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -s "${HOME}/.nvm/nvm.sh" ]]; then
  # shellcheck disable=SC1091
  source "${HOME}/.nvm/nvm.sh"
fi

echo "=== [1/4] Verificando Mercado Pago CLI (mpcli) en Sandbox MLC ==="
if ! command -v mpcli >/dev/null 2>&1; then
  echo "ERROR: mpcli no está instalado en PATH." >&2
  exit 1
fi
mpcli --version

echo "=== [2/4] Ejecutando tests de contrato y conexión mpcli en Go (internal/checkoutapi) ==="
MPCLI_SANDBOX_VERIFY=1 go test -v -count=1 ./internal/checkoutapi -run 'TestMPCLI'

echo "=== [3/4] Verificando conexión OAuth del vendedor y operaciones en PostgreSQL local ==="
RUNTIME_PW_FILE="${ROOT_DIR}/.local/secrets/runtime_password"
DB_PORT="${DATABASE_PORT:-15433}"
DB_NAME="${DATABASE_NAME:-espacigo_payments_local}"

if [[ -f "$RUNTIME_PW_FILE" ]]; then
  PGPASSFILE=/dev/null PGPASSWORD="$(cat "$RUNTIME_PW_FILE")" \
    psql -h 127.0.0.1 -p "$DB_PORT" -U espacigo_api_login -d "$DB_NAME" -v ON_ERROR_STOP=1 <<'SQL'
SET ROLE espacigo_runtime;
SELECT
  anfitrion_id,
  seller_id,
  live_mode,
  connection_state,
  (public_key IS NOT NULL AND public_key <> '') AS has_public_key,
  token_expires_at > NOW() AS token_unexpired,
  updated_at
FROM public.mercadopago_seller_connection
ORDER BY updated_at DESC
LIMIT 5;

SELECT
  id AS operation_id,
  cotizacion_id AS quote_id,
  checkout_method,
  estado AS status,
  base_clp,
  test_mode,
  (payment_create_claimed_at IS NOT NULL) AS claimed,
  COALESCE(provider_payment_id, '') AS provider_payment_id,
  COALESCE(provider_payment_status, '') AS provider_status,
  COALESCE(reconciliation_reason, '') AS reconciliation_reason,
  actualizada_en
FROM public.checkout_preferencia_comercial
ORDER BY actualizada_en DESC
LIMIT 5;
SQL
else
  echo "AVISO: No se encontró ${RUNTIME_PW_FILE}; omitiendo chequeo de base de datos local."
fi

echo "=== [4/4] Verificando contrato OpenAPI 3.1 del módulo Mercado Pago ==="
go test -v -count=1 ./internal/openapicontract -run 'TestMercadoPago'
echo "OK: Todas las comprobaciones de conexión y contrato de Mercado Pago pasaron."
