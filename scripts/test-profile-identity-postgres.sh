#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
profile_test_path="$(mktemp -d /tmp/esg-profile.XXXXXX)"
profile_test_container="espacigo-profile-integration-$$"
mkdir -m 0777 "$profile_test_path/socket"
cleanup_profile_test() {
  docker exec --user root "$profile_test_container" chmod 0777 /var/run/postgresql >/dev/null 2>&1 || true
  docker rm -f "$profile_test_container" >/dev/null 2>&1 || true
  rm -rf -- "$profile_test_path"
}
trap cleanup_profile_test EXIT
docker run --rm -d --pull=never --name "$profile_test_container" --network none \
    --tmpfs /var/lib/postgresql:rw,size=1g \
    --mount "type=bind,src=$profile_test_path/socket,dst=/var/run/postgresql" \
    -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=espacigo_profile_test postgres:18 >/dev/null
for i in $(seq 1 100); do
  ready_count="$(docker logs "$profile_test_container" 2>&1 | rg -c 'database system is ready to accept connections' || true)"
  if [[ "${ready_count:-0}" -ge 2 ]]; then break; fi
  sleep 0.25
done
profile_test_psql() {
  docker exec -i "$profile_test_container" psql -X -U postgres -d espacigo_profile_test -v ON_ERROR_STOP=1 "$@"
}
profile_test_psql -q <<'SQL'
CREATE ROLE espacigo_schema_owner NOLOGIN;
CREATE ROLE espacigo_runtime NOLOGIN;
GRANT USAGE,CREATE ON SCHEMA public TO espacigo_schema_owner;
GRANT USAGE ON SCHEMA public TO espacigo_runtime;
ALTER DEFAULT PRIVILEGES FOR ROLE espacigo_schema_owner IN SCHEMA public GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO espacigo_runtime;
SET ROLE espacigo_schema_owner;
CREATE TABLE public.usuario(id uuid PRIMARY KEY, correo_original text NOT NULL, correo_normalizado text NOT NULL DEFAULT '', actualizado_en timestamptz NOT NULL DEFAULT now(), hash_clave text NOT NULL DEFAULT '', estado text NOT NULL);
CREATE TABLE public.sesion(id uuid PRIMARY KEY, usuario_id uuid NOT NULL REFERENCES public.usuario(id), expira_en timestamptz NOT NULL, ultima_actividad_en timestamptz NOT NULL, revocada_en timestamptz);
CREATE TABLE public.token_accion(usuario_id uuid NOT NULL REFERENCES public.usuario(id), invalidado_en timestamptz, consumido_en timestamptz);
CREATE TABLE public.cuenta_totp(
    account_id uuid PRIMARY KEY REFERENCES public.usuario(id),
    secret_ciphertext bytea, key_version text, pending_secret_ciphertext bytea, pending_key_version text,
    enabled_at timestamptz, last_verified_step bigint, created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE public.cuenta_totp_backup_code(account_id uuid NOT NULL, used_at timestamptz);
CREATE TABLE public.cotizacion_arriendo_comercial(id uuid PRIMARY KEY, propietario_id uuid NOT NULL, arrendatario_id uuid NOT NULL);
SQL
{ printf 'SET ROLE espacigo_schema_owner;\n'; cat db/migrations/V000007__perfil_identidad_didit.sql db/migrations/V000008__espacigo_pinpass.sql db/migrations/V000009__pinpass_codigo_correo.sql; } | profile_test_psql -q
profile_test_psql -q <<'SQL'
DO $$
BEGIN
  IF has_table_privilege('espacigo_runtime','public.account_identity_rut_claim','UPDATE')
    OR has_table_privilege('espacigo_runtime','public.account_identity_rut_fingerprint','DELETE')
    OR has_table_privilege('espacigo_runtime','public.account_pinpass_audit','SELECT')
  THEN RAISE EXCEPTION 'runtime privileges too broad';
  END IF;
END;
$$;
SQL
ESPACIGO_PROFILE_TEST_DATABASE_URL="postgres://postgres@localhost/espacigo_profile_test?host=$profile_test_path/socket" GOCACHE="${GOCACHE:-/tmp/espacigo-profile-go-cache}" \
    go test ./internal/profilekyc ./cmd/api -run "^TestPostgres(ProfileVerificationAndPinpass|PinpassPasswordStepUp)$" -count=1 -v
