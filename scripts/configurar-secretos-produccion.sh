#!/usr/bin/env bash
set -euo pipefail

# Carga secretos de produccion hacia AWS SSM Parameter Store (sin VPC endpoint
# de Secrets Manager para ahorrar costos) y AWS Secrets Manager (migrator y
# Mercado Pago) usando el perfil de produccion.
#
# Modos de uso:
#   1) Generar plantilla local segura (.local/secrets/produccion.env, fuera de Git):
#      ./scripts/configurar-secretos-produccion.sh --init-template
#
#   2) Subir los secretos rellenados desde ese archivo a AWS:
#      aws login --profile espacigo-prod
#      ./scripts/configurar-secretos-produccion.sh --from-file .local/secrets/produccion.env
#
#   3) Modo interactivo:
#      aws login --profile espacigo-prod
#      ./scripts/configurar-secretos-produccion.sh

PROFILE="${ESPACIGO_PROD_PROFILE:-espacigo-prod}"
REGION="${ESPACIGO_PROD_REGION:-us-east-2}"
ENVIRONMENT="${ESPACIGO_ENVIRONMENT:-prod}"
ORIGIN="${ESPACIGO_FRONTEND_ORIGIN:-https://espacigo.testingpage.store}"

MODE="interactive"
ENV_FILE=""
DEFAULT_TEMPLATE=".local/secrets/produccion.env"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --init-template)
      MODE="init"
      if [[ $# -gt 1 && "${2:-}" != --* ]]; then
        ENV_FILE="$2"
        shift 2
      else
        ENV_FILE="$DEFAULT_TEMPLATE"
        shift
      fi
      ;;
    --from-file)
      MODE="file"
      ENV_FILE="${2:?Falta ruta del archivo de secretos}"
      shift 2
      ;;
    *)
      echo "Uso: $0 [--init-template [ruta]] [--from-file <ruta>]" >&2
      exit 2
      ;;
  esac
done

GEN_DB_API_PASS="$(openssl rand -base64 24 | tr -d '/+=')"
GEN_DB_MIGRATOR_PASS="$(openssl rand -base64 24 | tr -d '/+=')"
GEN_CURSOR_KEY="$(openssl rand -hex 32)"
GEN_TOTP_KEY="$(openssl rand -base64 32)"
GEN_TOTP_JSON="{\"active_key_id\":\"k1\",\"keys\":{\"k1\":\"${GEN_TOTP_KEY}\"}}"
GEN_MP_ENC_KEY="$(openssl rand -base64 32)"

if [[ "$MODE" == "init" ]]; then
  mkdir -p "$(dirname "$ENV_FILE")"
  chmod 700 "$(dirname "$ENV_FILE")" 2>/dev/null || true
  if [[ -f "$ENV_FILE" ]]; then
    echo "Ya existe ${ENV_FILE}; no se sobrescribe para proteger tus secretos."
    exit 0
  fi
  umask 077
  cat > "$ENV_FILE" <<EOF
# Secretos de produccion EspaciGo (archivo local en .local/, fuera de Git)
# Solo necesitas completar tus 3 credenciales de Mercado Pago Produccion.
# (Si dejas SMTP_PASSWORD vacio, el script intentara leerlo desde espacigo/${ENVIRONMENT}/auth/smtp-* en Secrets Manager.)

MP_CLIENT_ID=""
MP_CLIENT_SECRET=""
MP_WEBHOOK_SECRET=""

# Opcional: si ya existe en AWS Secrets Manager, dejalo vacio para copiarlo automaticamente
SMTP_PASSWORD=""

# Autogeneradas para la nueva BD RDS y llaves criptograficas (puedes conservarlas)
DB_API_PASSWORD="${GEN_DB_API_PASS}"
DB_MIGRATOR_PASSWORD="${GEN_DB_MIGRATOR_PASS}"
CURSOR_KEY="${GEN_CURSOR_KEY}"
TOTP_KEYRING_JSON='${GEN_TOTP_JSON}'
MP_TOKEN_ENCRYPTION_KEY="${GEN_MP_ENC_KEY}"
EOF
  chmod 600 "$ENV_FILE"
  echo "Plantilla creada (chmod 600): ${ENV_FILE}"
  exit 0
fi

# Aislamiento estricto respecto al aws-emulator local.
unset AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3 AWS_ENDPOINT_URL_SSM AWS_CA_BUNDLE

aws_prod() {
  aws --profile "$PROFILE" --region "$REGION" "$@"
}

echo "=== Configuracion de Secretos de Produccion (${PROFILE} / ${REGION} / env=${ENVIRONMENT}) ==="
if ! CALLER_ARN="$(aws_prod sts get-caller-identity --query 'Arn' --output text 2>/dev/null)"; then
  echo "ERROR: No hay sesion activa para el perfil '${PROFILE}'." >&2
  echo "Ejecuta primero: aws login --profile ${PROFILE}" >&2
  exit 1
fi
echo "Identidad AWS verificada: ${CALLER_ARN}"
echo

prompt_secret() {
  local label="$1"
  local default_gen="${2:-}"
  local val=""
  if [[ -n "$default_gen" ]]; then
    read -rsp "${label} [Enter para autogenerar]: " val
    echo
    if [[ -z "$val" ]]; then
      val="$default_gen"
    fi
  else
    while [[ -z "$val" ]]; do
      read -rsp "${label}: " val
      echo
    done
  fi
  printf '%s' "$val"
}

fetch_existing_smtp_from_sm() {
  local secret_id
  secret_id="$(aws_prod secretsmanager list-secrets \
    --filters "Key=name,Values=espacigo/${ENVIRONMENT}/auth/smtp" \
    --query 'SecretList[0].ARN' \
    --output text 2>/dev/null || true)"
  if [[ -n "$secret_id" && "$secret_id" != "None" ]]; then
    local raw
    raw="$(aws_prod secretsmanager get-secret-value --secret-id "$secret_id" --query 'SecretString' --output text 2>/dev/null || true)"
    if [[ -n "$raw" ]]; then
      python3 -c 'import json,sys; d=json.loads(sys.argv[1]); print(d.get("password",""))' "$raw" 2>/dev/null || true
    fi
  fi
}

upsert_ssm_secure() {
  local name="$1"
  local value="$2"
  local desc="$3"
  aws_prod ssm put-parameter \
    --name "$name" \
    --type SecureString \
    --value "$value" \
    --description "$desc" \
    --overwrite \
    --no-cli-pager >/dev/null
  echo "OK SSM: ${name}"
}

upsert_sm_secret() {
  local name="$1"
  local secret_json="$2"
  local desc="$3"
  local arn=""
  if aws_prod secretsmanager describe-secret --secret-id "$name" >/dev/null 2>&1; then
    arn="$(aws_prod secretsmanager put-secret-value \
      --secret-id "$name" \
      --secret-string "$secret_json" \
      --query 'ARN' \
      --output text)"
  else
    arn="$(aws_prod secretsmanager create-secret \
      --name "$name" \
      --description "$desc" \
      --secret-string "$secret_json" \
      --query 'ARN' \
      --output text)"
  fi
  printf '%s' "$arn"
}

if [[ "$MODE" == "file" ]]; then
  if [[ ! -f "$ENV_FILE" ]]; then
    echo "ERROR: No existe el archivo '${ENV_FILE}'." >&2
    exit 1
  fi
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
  DB_API_PASSWORD="${DB_API_PASSWORD:-$GEN_DB_API_PASS}"
  DB_MIGRATOR_PASSWORD="${DB_MIGRATOR_PASSWORD:-$GEN_DB_MIGRATOR_PASS}"
  CURSOR_KEY="${CURSOR_KEY:-$GEN_CURSOR_KEY}"
  TOTP_KEYRING_JSON="${TOTP_KEYRING_JSON:-$GEN_TOTP_JSON}"
  MP_TOKEN_ENCRYPTION_KEY="${MP_TOKEN_ENCRYPTION_KEY:-$GEN_MP_ENC_KEY}"

  if [[ -z "${SMTP_PASSWORD:-}" ]]; then
    echo "Buscando SMTP_PASSWORD existente en Secrets Manager (espacigo/${ENVIRONMENT}/auth/smtp*)..."
    SMTP_PASSWORD="$(fetch_existing_smtp_from_sm)"
    if [[ -z "$SMTP_PASSWORD" ]]; then
      echo "ERROR: No se encontro SMTP_PASSWORD en Secrets Manager y esta vacio en ${ENV_FILE}." >&2
      exit 1
    fi
    echo "OK: SMTP_PASSWORD recuperado desde Secrets Manager."
  fi

  for var in MP_CLIENT_ID MP_CLIENT_SECRET MP_WEBHOOK_SECRET; do
    if [[ -z "${!var:-}" ]]; then
      echo "ERROR: Falta completar '${var}' en ${ENV_FILE}." >&2
      exit 1
    fi
  done
else
  echo "--- 1. Parametros SSM para ECS API (ahorro sin VPC endpoint de Secrets Manager) ---"
  DB_API_PASSWORD="$(prompt_secret "Password de base de datos (espacigo_api_login)" "$GEN_DB_API_PASS")"
  CURSOR_KEY="$(prompt_secret "PUBLIC_CATALOG_ECS_CURSOR_KEY (hex 64 chars)" "$GEN_CURSOR_KEY")"
  SMTP_PASSWORD="$(prompt_secret "Password SMTP de produccion (Enter para intentar leer de Secrets Manager)" "$(fetch_existing_smtp_from_sm)")"
  TOTP_KEYRING_JSON="$(prompt_secret "TOTP_KEYRING_JSON" "$GEN_TOTP_JSON")"

  echo
  echo "--- 2. Secrets Manager: Migrator DB (para CodeBuild / ECS dbmigrate) ---"
  DB_MIGRATOR_PASSWORD="$(prompt_secret "Password de migraciones (espacigo_migrator_login)" "$GEN_DB_MIGRATOR_PASS")"

  echo
  echo "--- 3. Secrets Manager: Mercado Pago Produccion ---"
  read -rp "MERCADOPAGO_CLIENT_ID (App ID de produccion): " MP_CLIENT_ID
  while [[ -z "$MP_CLIENT_ID" ]]; do
    read -rp "MERCADOPAGO_CLIENT_ID (App ID de produccion): " MP_CLIENT_ID
  done
  MP_CLIENT_SECRET="$(prompt_secret "MERCADOPAGO_CLIENT_SECRET (produccion)")"
  MP_WEBHOOK_SECRET="$(prompt_secret "MERCADOPAGO_WEBHOOK_SECRET (firma secreta webhooks)")"
  MP_TOKEN_ENCRYPTION_KEY="$(prompt_secret "MERCADOPAGO_TOKEN_ENCRYPTION_KEY (Base64 32 bytes)" "$GEN_MP_ENC_KEY")"
fi

upsert_ssm_secure "/espacigo/${ENVIRONMENT}/database/api_password" "$DB_API_PASSWORD" "Database API runtime password"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/database/cursor_key" "$CURSOR_KEY" "Public catalog cursor HMAC key"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/auth/smtp_password" "$SMTP_PASSWORD" "SMTP password for transactional auth emails"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/auth/totp_keyring" "$TOTP_KEYRING_JSON" "TOTP AES keyring JSON"

MIGRATOR_JSON="$(python3 -c 'import json,sys; print(json.dumps({"password": sys.argv[1]}))' "$DB_MIGRATOR_PASSWORD")"
MIGRATOR_ARN="$(upsert_sm_secret "espacigo/${ENVIRONMENT}/database/migrator" "$MIGRATOR_JSON" "Database migrator login password")"
echo "OK Secrets Manager: ${MIGRATOR_ARN}"

MP_JSON="$(python3 -c 'import json,sys; print(json.dumps({"client_id":sys.argv[1],"client_secret":sys.argv[2],"token_encryption_key":sys.argv[3],"webhook_secret":sys.argv[4]}))' "$MP_CLIENT_ID" "$MP_CLIENT_SECRET" "$MP_TOKEN_ENCRYPTION_KEY" "$MP_WEBHOOK_SECRET")"
MP_ARN="$(upsert_sm_secret "espacigo/${ENVIRONMENT}/payments/mercadopago" "$MP_JSON" "Mercado Pago Chile production OAuth and webhook secrets")"
echo "OK Secrets Manager: ${MP_ARN}"

echo
echo "=== Resumen listo para CloudFormation y Panel de Mercado Pago ==="
echo "  DatabaseMigratorSecretArn = ${MIGRATOR_ARN}"
echo "  MercadoPagoSecretArn      = ${MP_ARN}"
echo
echo "=== URLs a configurar en Mercado Pago (Aplicacion de Produccion) ==="
echo "  OAuth Redirect URI : ${ORIGIN}/api/backend/host/mercadopago/oauth/callback"
echo "  Webhook URL        : ${ORIGIN}/api/v1/webhooks/mercadopago"
echo "  Origin / Return    : ${ORIGIN}"
