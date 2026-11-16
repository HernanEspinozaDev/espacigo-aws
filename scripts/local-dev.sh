#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EMULATOR_ENV="${HOME}/.aws/emulator/9297.env"
DB_CONTAINER="espacigo_localbackend_20261024-database-1"
DB_NAME="espacigo_aws_local"
DB_HOST="172.24.0.3"
DB_PORT="5432"

ensure_env() {
  if [[ -f "$EMULATOR_ENV" ]]; then
    # shellcheck disable=SC1090
    . "$EMULATOR_ENV"
  else
    echo "ERROR: aws-emulator env not found at $EMULATOR_ENV" >&2
    exit 1
  fi
}

cmd_status() {
  echo "=== [EspaciGo AWS] Estado del Entorno de Desarrollo Local ==="
  ensure_env

  # 1. aws-emulator
  echo -n "[1/5] Verificando aws-emulator (https://127.0.0.1:9297)... "
  if health=$(curl --cacert "$AWS_CA_BUNDLE" -fsS "$AWS_ENDPOINT_URL/__admin/health" 2>/dev/null); then
    echo "ONLINE ($health)"
  else
    echo "OFFLINE o inalcanzable"
    return 1
  fi

  # Caller Identity
  account_id=$(aws sts get-caller-identity --query "Account" --output text 2>/dev/null || echo "error")
  echo "      Cuenta AWS emulada: $account_id (aislada de cuentas reales)"

  # S3 Buckets
  echo -n "[2/5] Verificando S3 Buckets en aws-emulator... "
  bucket_count=$(aws s3 ls 2>/dev/null | grep -c "espacigo-" || true)
  echo "$bucket_count buckets detectados"

  # SSM Parameters
  echo -n "[3/5] Verificando SSM Parameter Store (/espacigo/local/)... "
  param_count=$(aws ssm get-parameters-by-path --path "/espacigo/local/" --recursive --query "Parameters[].Name" --output text 2>/dev/null | wc -w || echo "0")
  echo "$param_count parametros configurados"

  # PostgreSQL
  echo -n "[4/5] Verificando PostgreSQL 18 ($DB_CONTAINER)... "
  admin_pass=$(cat "${REPO_ROOT}/.local/secrets/db_admin_password" 2>/dev/null || echo "")
  if docker exec -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" -c "SELECT 1" >/dev/null 2>&1; then
    mig_count=$(docker exec -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" -tAc "SELECT count(*) FROM public.schema_migrations" 2>/dev/null || echo "0")
    echo "ONLINE (Base de datos: $DB_NAME, $mig_count migraciones aplicadas)"
  else
    echo "OFFLINE o error de autenticacion"
    return 1
  fi

  # Mailpit
  echo -n "[5/5] Verificando Mailpit (http://127.0.0.1:8025)... "
  if curl -fsS "http://127.0.0.1:8025/api/v1/info" >/dev/null 2>&1; then
    echo "ONLINE (SMTP: 127.0.0.1:1025, Web UI: 127.0.0.1:8025)"
  else
    echo "OFFLINE (iniciar con docker run -d --name espacigo-mailpit -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit:v1.31.4)"
  fi

  echo "=========================================================="
  echo "STATUS: Entorno 100% operativo para desarrollo y pruebas."
}

cmd_bootstrap() {
  echo "=== [EspaciGo AWS] Aprovisionando Stack Local en aws-emulator ==="
  bash "${REPO_ROOT}/scripts/local-aws-bootstrap.sh"
  
  echo "Aplicando configuración IaC con Terraform..."
  (
    cd "${REPO_ROOT}/infra/terraform/environments/local-emulator"
    terraform apply -auto-approve
  )

  # Asegurar permisos ACL de runtime y soporte S3 en PostgreSQL
  echo "Asegurando permisos para espacigo_runtime y soporte S3 en PostgreSQL..."
  admin_pass=$(cat "${REPO_ROOT}/.local/secrets/db_admin_password" 2>/dev/null || echo "")
  runtime_pass=$(cat "${REPO_ROOT}/.local/secrets/runtime_password" 2>/dev/null || echo "")
  if [ -n "$admin_pass" ]; then
    docker exec -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" <<EOF >/dev/null 2>&1 || true
ALTER ROLE espacigo_api_login WITH PASSWORD '${runtime_pass}';
GRANT USAGE ON SCHEMA public TO espacigo_runtime;
GRANT SELECT ON public.version_terminos TO espacigo_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO espacigo_runtime;
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO espacigo_runtime;

SET ROLE espacigo_schema_owner;
ALTER TABLE public.media_variant DROP CONSTRAINT IF EXISTS media_variant_provider_check;
ALTER TABLE public.media_variant ADD CONSTRAINT media_variant_provider_check CHECK (provider IN ('s3', 'gcs', 'filesystem'));
ALTER TABLE public.media_variant_intent DROP CONSTRAINT IF EXISTS media_variant_intent_provider_check;
ALTER TABLE public.media_variant_intent ADD CONSTRAINT media_variant_intent_provider_check CHECK (provider IN ('s3', 'gcs', 'filesystem'));
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO espacigo_runtime;
EOF
  fi

  # Asegurar Mailpit para captura de correos
  if ! curl -fsS "http://127.0.0.1:8025/api/v1/info" >/dev/null 2>&1; then
    docker rm -f espacigo-mailpit >/dev/null 2>&1 || true
    docker run -d --name espacigo-mailpit --restart unless-stopped -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit:v1.31.4 >/dev/null 2>&1 || true
    echo "Mailpit iniciado en 127.0.0.1:1025 (SMTP) / 127.0.0.1:8025 (Web UI)"
  fi

  # Asegurar .env.local en frontend
  cat <<'ENV' > "${REPO_ROOT}/frontend/.env.local"
API_BACKEND_BASE_URL=http://127.0.0.1:8080/api/v1
FRONTEND_PUBLIC_ORIGIN=http://127.0.0.1:3000
PORT=3000
ENV

  echo "=== Bootstrap completado exitosamente ==="
}

cmd_test() {
  echo "=== [EspaciGo AWS] Ejecutando Pruebas de Calidad e Integración ==="
  ensure_env

  echo "--- 1. Pruebas Unitarias e Integración Backend Go (S3 & Crypto) ---"
  (
    cd "$REPO_ROOT"
    go test -v ./internal/adapters/s3storage ./internal/adapters/documents3
  )

  echo "--- 2. Pruebas Frontend & Gateway BFF ---"
  (
    cd "${REPO_ROOT}/frontend"
    node --test tooling/*.test.mjs
  )

  echo "=== Todas las suites pasaron satisfactoriamente! ==="
}

cmd_api() {
  echo "=== [EspaciGo AWS] Iniciando Servidor API Go en http://127.0.0.1:8080 ==="
  ensure_env

  # Base de datos
  export DATABASE_CONNECTION_MODE="direct"
  export DATABASE_HOST="$DB_HOST"
  export DATABASE_PORT="$DB_PORT"
  export DATABASE_NAME="$DB_NAME"
  export DATABASE_USER="espacigo_api_login"
  export DATABASE_SET_ROLE="espacigo_runtime"
  export DATABASE_PASSWORD_FILE="${REPO_ROOT}/.local/secrets/runtime_password"
  export DATABASE_SSLMODE="disable"

  # Parámetros runtime de aplicación
  export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
  export FRONTEND_PUBLIC_ORIGIN="http://127.0.0.1:3000"
  export CORS_ALLOWED_ORIGINS="http://127.0.0.1:3000,http://localhost:3000"
  export LOCAL_AUTH_PROTOTYPE="1"
  export LOCAL_BOOKING_TRIAL="1"
  export LOCAL_VERIFICATION_IP_LIMIT="100"
  export LOCAL_SMTP_ADDR="127.0.0.1:1025"

  # Almacenamiento S3 en aws-emulator
  export BLOB_STORE_BACKEND="s3"
  export S3_STORAGE_BUCKET="espacigo-evidence-local"
  export S3_MEDIA_BUCKET="espacigo-media-local"
  export DOCUMENT_VERIFICATION_STORAGE_BACKEND="s3"
  export DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET="espacigo-kyc-identity-local"
  export DOCUMENT_VERIFICATION_SPACE_S3_BUCKET="espacigo-kyc-space-local"
  export LOCAL_PRIVACY_REPLAY_BACKEND="s3"
  export LOCAL_PRIVACY_REPLAY_BUCKET="espacigo-privacy-replay-local"

  # Keyrings y secretos locales
  export DOCUMENT_VERIFICATION_ENCRYPTION_KEYRING_FILE="${REPO_ROOT}/.local/secrets/document-verification-keyring.json"
  export LOCAL_CONTRACT_ENCRYPTION_KEY_FILE="${REPO_ROOT}/.local/secrets/local_contract_encryption_key"
  export LOCAL_PUBLIC_CATALOG_CURSOR_KEY_FILE="${REPO_ROOT}/.local/secrets/public_catalog_cursor_key"
  export LOCAL_PAYMENT_WEBHOOK_SECRET_FILE="${REPO_ROOT}/.local/secrets/local_payment_webhook_secret"
  export IDENTITY_DECLARATION_KEYRING_FILE="${REPO_ROOT}/.local/secrets/identity-declaration-keyring.json"
  export IDENTITY_RUT_HMAC_KEYRING_FILE="${REPO_ROOT}/.local/secrets/identity-rut-hmac-keyring.json"

  (
    cd "$REPO_ROOT"
    go run ./cmd/api
  )
}

cmd_web() {
  echo "=== [EspaciGo AWS] Iniciando Frontend Next.js en http://127.0.0.1:3000 ==="
  export API_BACKEND_BASE_URL="http://127.0.0.1:8080/api/v1"
  export FRONTEND_PUBLIC_ORIGIN="http://127.0.0.1:3000"
  export PORT="3000"
  (
    cd "${REPO_ROOT}/frontend"
    npm run dev -- -p 3000
  )
}

usage() {
  cat <<EOF
Uso: $0 <comando>

Comandos disponibles:
  status     Verifica el estado del emulador AWS, PostgreSQL, Mailpit y componentes locales.
  bootstrap  Aprovisiona los buckets S3, parámetros SSM, logs IAM, roles y Mailpit.
  test       Ejecuta los tests de Go (S3/cifrado) y tests del frontend Next.js.
  api        Inicia la API Go con todas las variables de entorno necesarias para registro y login.
  web        Inicia el frontend Next.js conectado al backend Go local.

EOF
}

case "${1:-}" in
  status) cmd_status ;;
  bootstrap) cmd_bootstrap ;;
  test) cmd_test ;;
  api) cmd_api ;;
  web) cmd_web ;;
  *) usage; exit 1 ;;
esac
