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
# Completa tus 3 credenciales de Mercado Pago Produccion.
# Los campos vacios conservan el valor que ya existe en AWS (o se autogeneran si no existen).

MP_CLIENT_ID=""
MP_CLIENT_SECRET=""
MP_WEBHOOK_SECRET=""

# Opcionales (dejar vacios para conservar los ya cargados en AWS)
SMTP_PASSWORD=""
DB_API_PASSWORD=""
DB_MIGRATOR_PASSWORD=""
CURSOR_KEY=""
TOTP_KEYRING_JSON=""
MP_TOKEN_ENCRYPTION_KEY=""
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

ssm_exists() {
  aws_prod ssm get-parameter --name "$1" --query 'Parameter.Name' --output text >/dev/null 2>&1
}

sm_exists() {
  aws_prod secretsmanager describe-secret --secret-id "$1" >/dev/null 2>&1
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

ensure_ssm_secure() {
  local name="$1"
  local value="$2"
  local fallback="$3"
  local desc="$4"
  if [[ -n "$value" ]]; then
    upsert_ssm_secure "$name" "$value" "$desc"
  elif ssm_exists "$name"; then
    echo "OK SSM (conservado existente): ${name}"
  elif [[ -n "$fallback" ]]; then
    upsert_ssm_secure "$name" "$fallback" "$desc"
  else
    echo "ERROR: Falta '${name}' (no existe en AWS ni en ${ENV_FILE})." >&2
    exit 1
  fi
}

upsert_sm_secret() {
  local name="$1"
  local secret_json="$2"
  local desc="$3"
  local arn=""
  if sm_exists "$name"; then
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
  for var in MP_CLIENT_ID MP_CLIENT_SECRET MP_WEBHOOK_SECRET; do
    if [[ -z "${!var:-}" ]]; then
      echo "ERROR: Falta completar '${var}' en ${ENV_FILE}." >&2
      exit 1
    fi
  done
else
  read -rp "MERCADOPAGO_CLIENT_ID (App ID de produccion): " MP_CLIENT_ID
  read -rsp "MERCADOPAGO_CLIENT_SECRET (produccion): " MP_CLIENT_SECRET; echo
  read -rsp "MERCADOPAGO_WEBHOOK_SECRET (firma secreta webhooks): " MP_WEBHOOK_SECRET; echo
fi

ensure_ssm_secure "/espacigo/${ENVIRONMENT}/database/api_password" "${DB_API_PASSWORD:-}" "$GEN_DB_API_PASS" "Database API runtime password"
ensure_ssm_secure "/espacigo/${ENVIRONMENT}/database/cursor_key" "${CURSOR_KEY:-}" "$GEN_CURSOR_KEY" "Public catalog cursor HMAC key"
ensure_ssm_secure "/espacigo/${ENVIRONMENT}/auth/smtp_password" "${SMTP_PASSWORD:-}" "" "SMTP password for transactional auth emails"
ensure_ssm_secure "/espacigo/${ENVIRONMENT}/auth/totp_keyring" "${TOTP_KEYRING_JSON:-}" "$GEN_TOTP_JSON" "TOTP AES keyring JSON"

MIGRATOR_NAME="espacigo/${ENVIRONMENT}/database/migrator"
if [[ -n "${DB_MIGRATOR_PASSWORD:-}" ]]; then
  MIGRATOR_JSON="$(python3 -c 'import json,sys; print(json.dumps({"password": sys.argv[1]}))' "$DB_MIGRATOR_PASSWORD")"
  MIGRATOR_ARN="$(upsert_sm_secret "$MIGRATOR_NAME" "$MIGRATOR_JSON" "Database migrator login password")"
  echo "OK Secrets Manager: ${MIGRATOR_ARN}"
elif sm_exists "$MIGRATOR_NAME"; then
  MIGRATOR_ARN="$(aws_prod secretsmanager describe-secret --secret-id "$MIGRATOR_NAME" --query 'ARN' --output text)"
  echo "OK Secrets Manager (conservado existente): ${MIGRATOR_ARN}"
else
  MIGRATOR_JSON="$(python3 -c 'import json,sys; print(json.dumps({"password": sys.argv[1]}))' "$GEN_DB_MIGRATOR_PASS")"
  MIGRATOR_ARN="$(upsert_sm_secret "$MIGRATOR_NAME" "$MIGRATOR_JSON" "Database migrator login password")"
  echo "OK Secrets Manager (autogenerado): ${MIGRATOR_ARN}"
fi

MP_ENC_KEY="${MP_TOKEN_ENCRYPTION_KEY:-}"
if [[ -z "$MP_ENC_KEY" ]] && sm_exists "espacigo/${ENVIRONMENT}/payments/mercadopago"; then
  EXISTING_MP_RAW="$(aws_prod secretsmanager get-secret-value --secret-id "espacigo/${ENVIRONMENT}/payments/mercadopago" --query 'SecretString' --output text 2>/dev/null || true)"
  MP_ENC_KEY="$(python3 -c 'import json,sys; d=json.loads(sys.argv[1]); print(d.get("token_encryption_key",""))' "$EXISTING_MP_RAW" 2>/dev/null || true)"
fi
MP_ENC_KEY="${MP_ENC_KEY:-$GEN_MP_ENC_KEY}"

MP_JSON="$(python3 -c 'import json,sys; print(json.dumps({"client_id":sys.argv[1],"client_secret":sys.argv[2],"token_encryption_key":sys.argv[3],"webhook_secret":sys.argv[4]}))' "$MP_CLIENT_ID" "$MP_CLIENT_SECRET" "$MP_ENC_KEY" "$MP_WEBHOOK_SECRET")"
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
