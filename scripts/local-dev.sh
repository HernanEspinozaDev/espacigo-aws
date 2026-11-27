#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EMULATOR_ENV="${HOME}/.aws/emulator/9297.env"
DB_CONTAINER="espacigo_localbackend_20261024-database-1"
DB_NAME="espacigo_aws_local"
DB_HOST="172.24.0.3"
DB_PORT="5432"
MIGRATOR_PASSWORD_FILE="${REPO_ROOT}/.local/secrets/migrator_password"

ensure_env() {
  if [[ -f "$EMULATOR_ENV" ]]; then
    # shellcheck disable=SC1090
    . "$EMULATOR_ENV"
  else
    echo "ERROR: aws-emulator env not found at $EMULATOR_ENV" >&2
    exit 1
  fi
  local location_pilot_env="${REPO_ROOT}/.local/secrets/amazon-location/pilot.env"
  if [[ -r "$location_pilot_env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$location_pilot_env"
    set +a
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
  local location_pilot_env="${REPO_ROOT}/.local/secrets/amazon-location/pilot.env"
  if [[ -r "$location_pilot_env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$location_pilot_env"
    set +a
  fi
  bash "${REPO_ROOT}/scripts/local-aws-bootstrap.sh"

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

-- Broad bootstrap grants above are retained for unrelated local services.
-- Restore the narrower, migration-defined KYB/compliance privileges afterward.
DO $restore_kyb_acl$
BEGIN
    IF pg_catalog.to_regclass('public.kyb_verifications') IS NOT NULL THEN
        REVOKE ALL ON TABLE public.organizacion, public.kyb_verifications,
            public.kyb_verification_document, public.kyb_rut_claim,
            public.kyb_verification_event FROM espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE (created_at) ON public.organizacion TO espacigo_runtime;
        GRANT SELECT ON public.kyb_verifications, public.kyb_verification_document TO espacigo_runtime;
        GRANT INSERT (id, organization_id, submitted_by, idempotency_key, legal_name,
            company_rut_fingerprint, company_rut_key_version, company_rut_ciphertext,
            representative_name, representative_rut_fingerprint, representative_rut_key_version,
            representative_rut_ciphertext, sii_giro_code, status, submitted_at, created_at, updated_at)
            ON public.kyb_verifications TO espacigo_runtime;
        GRANT UPDATE (status, rejection_reason, reviewer_id, reviewed_at, updated_at)
            ON public.kyb_verifications TO espacigo_runtime;
        GRANT INSERT (id, verification_id, document_kind, storage_ref, mime_type, size_bytes, content_sha256)
            ON public.kyb_verification_document TO espacigo_runtime;
        GRANT INSERT (rut_kind, fingerprint, verification_id, organization_id),
            SELECT (verification_id), UPDATE (active) ON public.kyb_rut_claim TO espacigo_runtime;
        GRANT SELECT, INSERT ON public.kyb_verification_event TO espacigo_runtime;
        REVOKE ALL ON SEQUENCE public.kyb_verification_event_id_seq FROM espacigo_runtime;
        GRANT USAGE ON SEQUENCE public.kyb_verification_event_id_seq TO espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.kyb_verification_event_immutable() FROM espacigo_runtime;
    END IF;

    IF pg_catalog.to_regclass('public.organizacion_miembro') IS NOT NULL THEN
        GRANT UPDATE (updated_at, trade_name, public_summary, logo_media_id, commune_code,
            region_code, sii_giro_code, grace_started_at, grace_due_at, compliance_status,
            disabled_at, disabled_reason, disabled_by, last_reminder_stage)
            ON public.organizacion TO espacigo_runtime;
        REVOKE ALL ON TABLE public.organizacion_rut_fingerprint, public.organizacion_miembro,
            public.organizacion_invitacion, public.anfitrion_cumplimiento_personal,
            public.anfitrion_cumplimiento_documento, public.space_kyb_verification,
            public.host_compliance_event FROM espacigo_runtime;
        GRANT SELECT, INSERT ON public.organizacion_rut_fingerprint TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON public.organizacion_miembro, public.organizacion_invitacion TO espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON public.anfitrion_cumplimiento_personal,
            public.anfitrion_cumplimiento_documento, public.space_kyb_verification TO espacigo_runtime;
        GRANT SELECT, INSERT ON public.host_compliance_event TO espacigo_runtime;
        REVOKE ALL ON SEQUENCE public.host_compliance_event_id_seq FROM espacigo_runtime;
        GRANT USAGE ON SEQUENCE public.host_compliance_event_id_seq TO espacigo_runtime;
        REVOKE EXECUTE ON FUNCTION public.organizacion_miembro_requires_legal_rep(),
            public.kyb_bind_company_rut(), public.kyb_submission_requires_active_member(),
            public.kyb_start_space_grace(), public.space_kyb_owner_guard(),
            public.host_compliance_event_immutable() FROM espacigo_runtime;
        GRANT EXECUTE ON FUNCTION public.kyb_space_eligibility(uuid,timestamptz),
            public.kyb_expire_noncompliant_profiles(timestamptz) TO espacigo_runtime;
    END IF;
    IF pg_catalog.to_regclass('public.espacio_solicitud_documental') IS NOT NULL THEN
        REVOKE ALL ON TABLE public.espacio_solicitud_documental FROM espacigo_runtime;
        GRANT SELECT ON TABLE public.espacio_solicitud_documental TO espacigo_runtime;
    END IF;
    IF pg_catalog.to_regclass('public.kyb_compliance_notice_outbox') IS NOT NULL THEN
        REVOKE ALL ON TABLE public.kyb_compliance_notice_outbox FROM espacigo_runtime;
        GRANT SELECT, INSERT, UPDATE ON TABLE public.kyb_compliance_notice_outbox TO espacigo_runtime;
    END IF;
END
$restore_kyb_acl$;
EOF
  fi

  cmd_migrator_setup

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
  if [[ -n "${NEXT_PUBLIC_AMAZON_LOCATION_REGION:-}" && -n "${NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY:-}" ]]; then
    printf '%s\n' \
      "NEXT_PUBLIC_AMAZON_LOCATION_REGION=${NEXT_PUBLIC_AMAZON_LOCATION_REGION}" \
      "NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY=${NEXT_PUBLIC_AMAZON_LOCATION_MAP_KEY}" \
      >> "${REPO_ROOT}/frontend/.env.local"
  fi

  echo "=== Bootstrap completado exitosamente ==="
}

local_migration_host() {
  docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$DB_CONTAINER"
}

cmd_migrator_setup() {
  local host password
  host="$(local_migration_host)"
  docker exec -i "$DB_CONTAINER" sh -ec '
    PGPASSWORD="$(cat /run/secrets/db_admin_password)" exec psql -h 127.0.0.1 -Xq -v ON_ERROR_STOP=1 -U espacigo_admin -d "$1"
  ' sh "$DB_NAME" <<'SQL'
DO $setup$
BEGIN
    IF current_database() <> 'espacigo_aws_local' OR session_user <> 'espacigo_admin' THEN
        RAISE EXCEPTION 'Unexpected local migration target';
    END IF;
    CREATE EXTENSION IF NOT EXISTS postgis;
    CREATE EXTENSION IF NOT EXISTS btree_gist;
    CREATE EXTENSION IF NOT EXISTS unaccent;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_runtime') THEN
        CREATE ROLE espacigo_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_api_login') THEN
        CREATE ROLE espacigo_api_login LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_migrator_login') THEN
        CREATE ROLE espacigo_migrator_login LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_schema_owner') THEN
        CREATE ROLE espacigo_schema_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_document_verification_fn') THEN
        CREATE ROLE espacigo_document_verification_fn NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_privacy_cleanup_worker') THEN
        CREATE ROLE espacigo_privacy_cleanup_worker NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_privacy_worker_login') THEN
        CREATE ROLE espacigo_privacy_worker_login LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_auth_members
        WHERE member = 'espacigo_api_login'::regrole AND roleid = 'espacigo_runtime'::regrole
    ) THEN
        GRANT espacigo_runtime TO espacigo_api_login WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
    END IF;
    IF (SELECT count(*) FROM pg_roles
        WHERE rolname IN ('espacigo_migrator_login', 'espacigo_schema_owner')
          AND rolcanlogin = (rolname = 'espacigo_migrator_login')
          AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit)) <> 2 THEN
        RAISE EXCEPTION 'Unexpected local migration roles';
    END IF;
    IF (SELECT count(*) FROM pg_auth_members
        WHERE member = 'espacigo_migrator_login'::regrole) > 1
        OR EXISTS (SELECT 1 FROM pg_auth_members
                   WHERE member = 'espacigo_migrator_login'::regrole
                     AND (roleid <> 'espacigo_schema_owner'::regrole
                          OR admin_option OR inherit_option OR NOT set_option)) THEN
        RAISE EXCEPTION 'Unexpected local migration memberships';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_auth_members WHERE member = 'espacigo_migrator_login'::regrole) THEN
        GRANT espacigo_schema_owner TO espacigo_migrator_login
            WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
    END IF;
END
$setup$;
SQL
  python3 - "$MIGRATOR_PASSWORD_FILE" <<'PYTHON'
import os
import secrets
import stat
import sys
from pathlib import Path

path = Path(sys.argv[1])
try:
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or stat.S_IMODE(info.st_mode) != 0o600:
        raise SystemExit("El archivo de contraseña local debe ser privado, regular y pertenecer al operador")
else:
    with os.fdopen(fd, "w") as file:
        file.write(secrets.token_urlsafe(48) + "\n")
PYTHON
  password="$(cat "$MIGRATOR_PASSWORD_FILE")"
  if [[ ! "$password" =~ ^[A-Za-z0-9_-]{64}$ ]]; then
    echo "ERROR: formato inesperado en la credencial local del migrador." >&2
    return 1
  fi
  if ! PGPASSWORD="$password" psql -h "$host" -p "$DB_PORT" -XqAtw -U espacigo_migrator_login \
    -d "$DB_NAME" -c 'SELECT 1' >/dev/null 2>&1; then
    printf '\\password espacigo_migrator_login\n%s\n%s\n' "$password" "$password" |
      docker exec -i "$DB_CONTAINER" sh -ec '
        PGPASSWORD="$(cat /run/secrets/db_admin_password)" exec psql -h 127.0.0.1 -Xq -v ON_ERROR_STOP=1 -U espacigo_admin -d "$1"
      ' sh "$DB_NAME"
  fi
  local identity
  identity="$(PGPASSWORD="$password" psql -h "$host" -p "$DB_PORT" -XqAtw -v ON_ERROR_STOP=1 \
    -U espacigo_migrator_login -d "$DB_NAME" -c 'SET ROLE espacigo_schema_owner; SELECT session_user, current_user')"
  [[ "$identity" == 'espacigo_migrator_login|espacigo_schema_owner' ]]
  echo "Login local verificado: $identity"
}

cmd_migrator_enable_ddl() {
  docker exec -i "$DB_CONTAINER" sh -ec '
    PGPASSWORD="$(cat /run/secrets/db_admin_password)" exec psql -h 127.0.0.1 -Xq -v ON_ERROR_STOP=1 -U espacigo_admin -d "$1"
  ' sh "$DB_NAME" < "${REPO_ROOT}/db/bootstrap/migration-owner.sql"
  echo "DDL habilitado para el propietario de migraciones en PostgreSQL local."
}

cmd_migrations() (
  local host passfile
  host="$(local_migration_host)"
  if [[ ! -r "$MIGRATOR_PASSWORD_FILE" ]]; then
    echo "ERROR: ejecuta migrator-setup para preparar la credencial local." >&2
    return 1
  fi
  umask 077
  passfile="$(mktemp "${REPO_ROOT}/.local/secrets/migrator.XXXXXX")"
  trap 'rm -f -- "$passfile"' EXIT
  printf '%s:%s:%s:%s:%s\n' "$host" "$DB_PORT" "$DB_NAME" espacigo_migrator_login \
    "$(cat "$MIGRATOR_PASSWORD_FILE")" > "$passfile"
  cd "$REPO_ROOT"
  env -u PGPASSWORD -u PGSERVICE -u PGSERVICEFILE -u PGOPTIONS -u PGHOSTADDR \
    PGPASSFILE="$passfile" DATABASE_CONNECTION_MODE=direct DATABASE_PASSWORD_FILE='' \
    DATABASE_URL="postgres://espacigo_migrator_login@$host:$DB_PORT/$DB_NAME?sslmode=disable" \
    DATABASE_SET_ROLE=espacigo_schema_owner go run ./cmd/dbmigrate "$@"
)

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
  local api_db_host api_db_port
  echo "=== [EspaciGo AWS] Iniciando Servidor API Go en http://127.0.0.1:8080 ==="
  ensure_env
  local commune_boundaries="${REPO_ROOT}/.local/geo/dpa_2023_communes.bin.gz"
  if [[ -z "${ESPACIGO_COMMUNE_BOUNDARIES_FILE:-}" && -r "$commune_boundaries" ]]; then
    export ESPACIGO_COMMUNE_BOUNDARIES_FILE="$commune_boundaries"
  fi
  if [[ "${ESPACIGO_LOCAL_ONLY:-0}" == 1 ]]; then
    export MERCADOPAGO_CHECKOUT_ENABLED=0
    export MERCADOPAGO_WALLET_PURCHASE_ENABLED=0
    export MERCADOPAGO_PAYMENTS_API_ENABLED=0
    export MERCADOPAGO_WEBHOOK_ENABLED=0
  fi
  export ENVIRONMENT=development
  load_mercadopago_oauth_env

  # Base de datos
  export DATABASE_CONNECTION_MODE="direct"
  api_db_host="${ESPACIGO_LOCAL_DB_HOST:-$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$DB_CONTAINER")}"
  api_db_port="$DB_PORT"
  if [[ -n "${ESPACIGO_LOCAL_DB_PORT:-}" ]]; then
    if [[ "$api_db_host" != "127.0.0.1" || ! "$ESPACIGO_LOCAL_DB_PORT" =~ ^[0-9]{1,5}$ ]] || (( 10#$ESPACIGO_LOCAL_DB_PORT < 1 || 10#$ESPACIGO_LOCAL_DB_PORT > 65535 )); then
      echo "ERROR: ESPACIGO_LOCAL_DB_PORT requires ESPACIGO_LOCAL_DB_HOST=127.0.0.1 and a valid TCP port." >&2
      return 1
    fi
    api_db_port="$ESPACIGO_LOCAL_DB_PORT"
  fi
  export DATABASE_HOST="$api_db_host"
  export DATABASE_PORT="$api_db_port"
  export DATABASE_NAME="$DB_NAME"
  export DATABASE_USER="espacigo_api_login"
  export DATABASE_SET_ROLE="espacigo_runtime"
  export DATABASE_PASSWORD_FILE="${REPO_ROOT}/.local/secrets/runtime_password"
  export DATABASE_SSLMODE="disable"

  # Parámetros runtime de aplicación
  export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
  export FRONTEND_PUBLIC_ORIGIN="${FRONTEND_PUBLIC_ORIGIN:-http://127.0.0.1:3000}"
  export CORS_ALLOWED_ORIGINS="${CORS_ALLOWED_ORIGINS:-http://127.0.0.1:3000,http://localhost:3000}"
  export LOCAL_AUTH_PROTOTYPE="1"
  export KYB_LOCAL_TERMS_V2="1"
  case "${COMMERCIAL_RENTAL_FLOW_ENABLED:-0}" in
    1)
      export COMMERCIAL_RENTAL_FLOW_ENABLED="1"
      export LOCAL_BOOKING_TRIAL="0"
      ;;
    0|"")
      unset COMMERCIAL_RENTAL_FLOW_ENABLED
      export LOCAL_BOOKING_TRIAL="1"
      ;;
    *)
      echo "ERROR: COMMERCIAL_RENTAL_FLOW_ENABLED must be 0 or 1." >&2
      return 1
      ;;
  esac
  export LOCAL_VERIFICATION_IP_LIMIT="100"
  export LOCAL_SMTP_ADDR="127.0.0.1:1025"

  # Almacenamiento S3 en aws-emulator
  export BLOB_STORE_BACKEND="s3"
  export S3_STORAGE_BUCKET="espacigo-evidence-local"
  export S3_MEDIA_BUCKET="espacigo-media-local"
  export DOCUMENT_VERIFICATION_STORAGE_BACKEND="s3"
  export DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET="espacigo-kyc-identity-local"
  export DOCUMENT_VERIFICATION_SPACE_S3_BUCKET="espacigo-kyc-space-local"
  export KYB_DOCUMENTS_S3_BUCKET="espacigo-kyb-documents-local"
  export LOCAL_PRIVACY_REPLAY_BACKEND="s3"
  export LOCAL_PRIVACY_REPLAY_BUCKET="espacigo-privacy-replay-local"

  # Keyrings y secretos locales
  export DOCUMENT_VERIFICATION_ENCRYPTION_KEYRING_FILE="${REPO_ROOT}/.local/secrets/document-verification-keyring.json"
  export LOCAL_CONTRACT_ENCRYPTION_KEY_FILE="${REPO_ROOT}/.local/secrets/local_contract_encryption_key"
  export LOCAL_PUBLIC_CATALOG_CURSOR_KEY_FILE="${REPO_ROOT}/.local/secrets/public_catalog_cursor_key"
  export LOCAL_PAYMENT_WEBHOOK_SECRET_FILE="${REPO_ROOT}/.local/secrets/local_payment_webhook_secret"
  export IDENTITY_DECLARATION_KEYRING_FILE="${REPO_ROOT}/.local/secrets/identity-declaration-keyring.json"
  export IDENTITY_RUT_HMAC_KEYRING_FILE="${REPO_ROOT}/.local/secrets/identity-rut-hmac-keyring.json"
  export KYB_DOCUMENT_KEYRING_FILE="${REPO_ROOT}/.local/secrets/kyb-document-keyring.json"
  export KYB_RUT_ENCRYPTION_KEYRING_FILE="${REPO_ROOT}/.local/secrets/kyb-rut-encryption-keyring.json"
  export KYB_RUT_HMAC_KEYRING_FILE="${REPO_ROOT}/.local/secrets/kyb-rut-hmac-keyring.json"
  export CONTRACT_PROPOSAL_KEYRING_FILE="${CONTRACT_PROPOSAL_KEYRING_FILE:-${REPO_ROOT}/.local/secrets/identity-declaration-keyring.json}"

  (
    cd "$REPO_ROOT"
    go run ./cmd/api
  )
}

load_mercadopago_oauth_env() {
  local file="${REPO_ROOT}/.local/secrets/mercadopago-test.env" line key value mode owner
  [[ -e "$file" ]] || return 0
  if [[ -L "$file" || ! -f "$file" || ! -r "$file" ]]; then
    echo "ERROR: Mercado Pago config must be a readable regular file." >&2
    return 1
  fi
  mode="$(stat -c '%a' "$file")"
  owner="$(stat -c '%u' "$file")"
  if [[ "$mode" != "600" || "$owner" != "$(id -u)" ]]; then
    echo "ERROR: Mercado Pago config must be owned by the current user with mode 600." >&2
    return 1
  fi
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || { echo "ERROR: Invalid Mercado Pago config line." >&2; return 1; }
    key="${line%%=*}"
    value="${line#*=}"
    case "$key" in
      MERCADOPAGO_TEST_ACCESS_TOKEN|MERCADOPAGO_CLIENT_ID|MERCADOPAGO_CLIENT_SECRET|MERCADOPAGO_OAUTH_REDIRECT_URI|MERCADOPAGO_TOKEN_ENCRYPTION_KEY|MERCADOPAGO_TEST_MODE|MERCADOPAGO_WEBHOOK_SECRET_FILE) ;;
      *) echo "ERROR: Unsupported key in Mercado Pago config." >&2; return 1 ;;
    esac
    [[ "$value" != *$'\r'* ]] || { echo "ERROR: Invalid Mercado Pago config value." >&2; return 1; }
    export "$key=$value"
  done < "$file"
  if [[ -n "${MERCADOPAGO_CLIENT_ID:-}" || -n "${MERCADOPAGO_CLIENT_SECRET:-}" || -n "${MERCADOPAGO_OAUTH_REDIRECT_URI:-}" || -n "${MERCADOPAGO_TOKEN_ENCRYPTION_KEY:-}" ]]; then
    export MERCADOPAGO_TEST_MODE="${MERCADOPAGO_TEST_MODE:-true}"
  fi
}

cmd_web() {
  local location_pilot_env="${REPO_ROOT}/.local/secrets/amazon-location/pilot.env"
  if [[ -r "$location_pilot_env" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$location_pilot_env"
    set +a
  fi
  echo "=== [EspaciGo AWS] Iniciando Frontend Next.js; abrir en ${FRONTEND_PUBLIC_ORIGIN:-http://127.0.0.1:3000} ==="
  export API_BACKEND_BASE_URL="http://127.0.0.1:8080/api/v1"
  export ENVIRONMENT=development
  export FRONTEND_PUBLIC_ORIGIN="${FRONTEND_PUBLIC_ORIGIN:-http://127.0.0.1:3000}"
  export PORT="3000"
  (
    cd "${REPO_ROOT}/frontend"
    if command -v pnpm >/dev/null 2>&1; then
      pnpm dev -p 3000
    elif command -v corepack >/dev/null 2>&1; then
      corepack pnpm dev -p 3000
    else
      echo "ERROR: pnpm no está disponible en PATH (packageManager fijado en frontend/package.json)." >&2
      return 127
    fi
  )
}

usage() {
  cat <<EOF
Uso: $0 <comando>

Comandos disponibles:
  status     Verifica el estado del emulador AWS, PostgreSQL, Mailpit y componentes locales.
  bootstrap  Aprovisiona los buckets S3, parámetros SSM, logs IAM, roles y Mailpit.
  migrator-setup  Prepara y comprueba el login local dedicado a migraciones.
  migrator-enable-ddl  Habilita el DDL del rol local después de V100.
  migrate-plan    Muestra migraciones locales aplicadas y pendientes.
  migrate         Aplica migraciones locales con el login dedicado.
  test       Ejecuta los tests de Go (S3/cifrado) y tests del frontend Next.js.
  api        Inicia la API Go con todas las variables de entorno necesarias para registro y login.
  web        Inicia el frontend Next.js conectado al backend Go local.

EOF
}

case "${1:-}" in
  status) cmd_status ;;
  bootstrap) cmd_bootstrap ;;
  migrator-setup) cmd_migrator_setup ;;
  migrator-enable-ddl) cmd_migrator_enable_ddl ;;
  migrate-plan) cmd_migrations -plan-only ;;
  migrate) cmd_migrations ;;
  test) cmd_test ;;
  api) cmd_api ;;
  web) cmd_web ;;
  *) usage; exit 1 ;;
esac
