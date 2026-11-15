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
  echo -n "[1/4] Verificando aws-emulator (https://127.0.0.1:9297)... "
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
  echo -n "[2/4] Verificando S3 Buckets en aws-emulator... "
  bucket_count=$(aws s3 ls 2>/dev/null | grep -c "espacigo-" || true)
  echo "$bucket_count buckets detectados"

  # SSM Parameters
  echo -n "[3/4] Verificando SSM Parameter Store (/espacigo/local/)... "
  param_count=$(aws ssm get-parameters-by-path --path "/espacigo/local/" --recursive --query "Parameters[].Name" --output text 2>/dev/null | wc -w || echo "0")
  echo "$param_count parametros configurados"

  # PostgreSQL
  echo -n "[4/4] Verificando PostgreSQL 18 ($DB_CONTAINER)... "
  admin_pass=$(cat "${REPO_ROOT}/.local/secrets/db_admin_password" 2>/dev/null || echo "")
  if docker exec -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" -c "SELECT 1" >/dev/null 2>&1; then
    mig_count=$(docker exec -e PGPASSWORD="$admin_pass" "$DB_CONTAINER" psql -U espacigo_admin -d "$DB_NAME" -tAc "SELECT count(*) FROM public.schema_migrations" 2>/dev/null || echo "0")
    echo "ONLINE (Base de datos: $DB_NAME, $mig_count migraciones aplicadas)"
  else
    echo "OFFLINE o error de autenticacion"
    return 1
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

  export DATABASE_CONNECTION_MODE=direct
  export DATABASE_HOST="$DB_HOST"
  export DATABASE_PORT="$DB_PORT"
  export DATABASE_NAME="$DB_NAME"
  export DATABASE_USER="espacigo_api_login"
  export DATABASE_SET_ROLE="espacigo_runtime"
  export DATABASE_PASSWORD_FILE="${REPO_ROOT}/.local/secrets/runtime_password"
  export DATABASE_SSLMODE=disable

  export BLOB_STORE_BACKEND=s3
  export S3_STORAGE_BUCKET="espacigo-evidence-local"
  export S3_MEDIA_BUCKET="espacigo-media-local"
  export DOCUMENT_VERIFICATION_STORAGE_BACKEND=s3
  export DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET="espacigo-kyc-identity-local"
  export DOCUMENT_VERIFICATION_SPACE_S3_BUCKET="espacigo-kyc-space-local"
  export LOCAL_PRIVACY_REPLAY_BACKEND=s3
  export LOCAL_PRIVACY_REPLAY_BUCKET="espacigo-privacy-replay-local"
  export LOCAL_CONTRACT_ENCRYPTION_KEY_FILE="${REPO_ROOT}/.local/secrets/local_contract_encryption_key"
  export LOCAL_PAYMENT_WEBHOOK_SECRET_FILE="${REPO_ROOT}/.local/secrets/local_payment_webhook_secret"
  export HTTP_ADDR=":8080"

  (
    cd "$REPO_ROOT"
    go run ./cmd/api
  )
}

cmd_web() {
  echo "=== [EspaciGo AWS] Iniciando Frontend Next.js en http://127.0.0.1:3000 ==="
  (
    cd "${REPO_ROOT}/frontend"
    npm run dev
  )
}

usage() {
  cat <<EOF
Uso: $0 <comando>

Comandos disponibles:
  status     Verifica el estado del emulador AWS, PostgreSQL y componentes locales.
  bootstrap  Aprovisiona los buckets S3, parámetros SSM, logs IAM y roles en aws-emulator.
  test       Ejecuta los tests de Go (S3/cifrado) y tests del frontend Next.js.
  api        Inicia la API Go conectada a aws-emulator y a PostgreSQL espacigo_aws_local.
  web        Inicia el frontend Next.js en modo desarrollo.

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
