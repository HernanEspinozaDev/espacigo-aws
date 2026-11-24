#!/usr/bin/env bash
set -euo pipefail

# Carga interactivamente los secretos de produccion hacia AWS SSM Parameter Store
# (sin endpoint VPC de Secrets Manager para ahorrar costos) y AWS Secrets Manager
# (migrator y Mercado Pago) usando el perfil de produccion.
#
# Uso:
#   aws login --profile espacigo-prod
#   ./scripts/configurar-secretos-produccion.sh

PROFILE="${ESPACIGO_PROD_PROFILE:-espacigo-prod}"
REGION="${ESPACIGO_PROD_REGION:-us-east-2}"
ENVIRONMENT="${ESPACIGO_ENVIRONMENT:-prod}"
ORIGIN="${ESPACIGO_FRONTEND_ORIGIN:-https://espacigo.testingpage.store}"

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

GEN_CURSOR_KEY="$(openssl rand -hex 32)"
GEN_TOTP_KEY="$(openssl rand -base64 32)"
GEN_TOTP_JSON="{\"active_key_id\":\"k1\",\"keys\":{\"k1\":\"${GEN_TOTP_KEY}\"}}"
GEN_MP_ENC_KEY="$(openssl rand -base64 32)"

echo "--- 1. Parametros SSM para ECS API (ahorro sin VPC endpoint de Secrets Manager) ---"
DB_API_PASS="$(prompt_secret "Password de base de datos (espacigo_api_login)")"
CURSOR_KEY="$(prompt_secret "PUBLIC_CATALOG_ECS_CURSOR_KEY (hex 64 chars)" "$GEN_CURSOR_KEY")"
SMTP_PASS="$(prompt_secret "Password SMTP de produccion (AUTH_SMTP_ECS_PASSWORD)")"
TOTP_JSON="$(prompt_secret "TOTP_KEYRING_JSON" "$GEN_TOTP_JSON")"

upsert_ssm_secure "/espacigo/${ENVIRONMENT}/database/api_password" "$DB_API_PASS" "Database API runtime password"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/database/cursor_key" "$CURSOR_KEY" "Public catalog cursor HMAC key"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/auth/smtp_password" "$SMTP_PASS" "SMTP password for transactional auth emails"
upsert_ssm_secure "/espacigo/${ENVIRONMENT}/auth/totp_keyring" "$TOTP_JSON" "TOTP AES keyring JSON"

echo
echo "--- 2. Secrets Manager: Migrator DB (para CodeBuild / ECS dbmigrate) ---"
DB_MIGRATOR_PASS="$(prompt_secret "Password de migraciones (espacigo_migrator_login)")"
MIGRATOR_JSON="$(python3 -c 'import json,sys; print(json.dumps({"password": sys.argv[1]}))' "$DB_MIGRATOR_PASS")"
MIGRATOR_ARN="$(upsert_sm_secret "espacigo/${ENVIRONMENT}/database/migrator" "$MIGRATOR_JSON" "Database migrator login password")"
echo "OK Secrets Manager: ${MIGRATOR_ARN}"

echo
echo "--- 3. Secrets Manager: Mercado Pago Produccion ---"
read -rp "MERCADOPAGO_CLIENT_ID (App ID de produccion): " MP_CLIENT_ID
while [[ -z "$MP_CLIENT_ID" ]]; do
  read -rp "MERCADOPAGO_CLIENT_ID (App ID de produccion): " MP_CLIENT_ID
done
MP_CLIENT_SECRET="$(prompt_secret "MERCADOPAGO_CLIENT_SECRET (produccion)")"
MP_WEBHOOK_SECRET="$(prompt_secret "MERCADOPAGO_WEBHOOK_SECRET (firma secreta webhooks)")"
MP_ENC_KEY="$(prompt_secret "MERCADOPAGO_TOKEN_ENCRYPTION_KEY (Base64 32 bytes)" "$GEN_MP_ENC_KEY")"

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
