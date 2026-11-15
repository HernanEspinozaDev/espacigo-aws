# espaciGo AWS (Local-First Architecture)

Plataforma SaaS para la gestión, arriendo flexible, verificación documental (KYC/espacios) y legalización de espacios comerciales e inmobiliarios en Chile, operando **100% AWS-native en entorno local** mediante `aws-emulator` y PostgreSQL 18.

---

## Arquitectura Local (`espacigo-aws`)

Esta edición elimina por completo cualquier dependencia de Google Cloud Platform (cero Cloud Run, Cloud SQL, Cloud Storage y Cloud Build) y opera íntegramente sobre servicios locales:

1. **Almacenamiento S3 Local (`aws-emulator`):**
   - **Endpoint:** `https://127.0.0.1:9297`
   - **Cuenta aislada:** `000000000000` (`us-east-1`)
   - **Modo de direccionamiento:** `UsePathStyle: true`
   - **CA TLS:** `~/.aws/emulator/9297/ca.pem`
   - **Perfil de entorno:** `~/.aws/emulator/9297.env`
   - **Buckets S3 locales:**
     - `espacigo-evidence-local` (`S3_BUCKET`) — evidencia privada de disputas y contratos
     - `espacigo-media-local` (`S3_MEDIA_BUCKET`) — galería multimedia de espacios
     - `espacigo-kyc-identity-local` (`DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET`) — documentos de identidad cifrados
     - `espacigo-kyc-space-local` (`DOCUMENT_VERIFICATION_SPACE_S3_BUCKET`) — antecedentes de espacios cifrados
     - `espacigo-privacy-replay-local` (`LOCAL_PRIVACY_REPLAY_BUCKET`) — registro durable de supresión/privacidad

2. **Base de Datos Operativa (PostgreSQL 18 + PostGIS):**
   - **Base de datos local:** `espacigo_aws_local` (`127.0.0.1:5432`)
   - **Extensiones requeridas:** `btree_gist`, `postgis`, `unaccent`
   - **Conexión directa:** `DATABASE_CONNECTION_MODE=direct`

3. **Backend Core (`cmd/api` y `cmd/dbmigrate`):**
   - Go 1.27 con AWS SDK for Go v2 (`service/s3`) apuntando por defecto a `https://127.0.0.1:9297` con `UsePathStyle: true` y verificación TLS con `~/.aws/emulator/9297/ca.pem`.

4. **Frontend Web (`frontend/`):**
   - Next.js 16 (App Router, React 19, TypeScript estricto) con BFF en `src/app/api/backend/` comunicándose directamente con el backend Go sin cabeceras ni metadatos serverless externos.

---

## Puesta en Marcha Local

### 1. Bootstrap de `aws-emulator` y secretos locales

Ejecuta el script idempotente de bootstrap para verificar la salud de `aws-emulator` (`https://127.0.0.1:9297`), confirmar la cuenta `000000000000`, crear los 5 buckets S3 locales y generar las llaves de desarrollo en `.local/secrets/` (directorio `0700`, archivos `0600`):

```bash
./scripts/local-aws-bootstrap.sh
```

Para inspeccionar los buckets creados usando una subshell segura:

```bash
(
  export PATH="/opt/aws-emulator/bin:$HOME/.local/bin:/usr/local/bin:$PATH"
  . ~/.aws/emulator/9297.env
  aws s3api list-buckets
)
```

### 2. Preparar y migrar la base de datos PostgreSQL 18 (`espacigo_aws_local`)

Crea la base de datos `espacigo_aws_local` (si aún no existe) y aplica las migraciones con `cmd/dbmigrate`:

```bash
psql -h 127.0.0.1 -U postgres -d postgres -v ON_ERROR_STOP=1 <<'SQL'
SELECT 'CREATE DATABASE espacigo_aws_local'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'espacigo_aws_local')\gexec
\c espacigo_aws_local
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS unaccent;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'espacigo_runtime') THEN
    CREATE ROLE espacigo_runtime NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT;
  END IF;
END
$$;
SQL

DATABASE_URL="postgres://postgres@127.0.0.1:5432/espacigo_aws_local?sslmode=disable" \
DATABASE_CONNECTION_MODE=direct \
go run ./cmd/dbmigrate -dir db/migrations
```

### 3. Levantar el Backend Go (`cmd/api`)

```bash
(
  . ~/.aws/emulator/9297.env
  export HTTP_ADDR="127.0.0.1:8080"
  export FRONTEND_PUBLIC_ORIGIN="http://127.0.0.1:3000"
  export CORS_ALLOWED_ORIGINS="http://127.0.0.1:3000,http://localhost:3000"
  export LOCAL_AUTH_PROTOTYPE="1"
  export LOCAL_BOOKING_TRIAL="1"
  export LOCAL_PUBLIC_CATALOG_TRIAL="1"
  export DATABASE_CONNECTION_MODE="direct"
  export DATABASE_HOST="127.0.0.1"
  export DATABASE_PORT="5432"
  export DATABASE_NAME="espacigo_aws_local"
  export DATABASE_USER="postgres"
  export DATABASE_SSLMODE="disable"
  export BLOB_STORE_BACKEND="s3"
  export S3_BUCKET="espacigo-evidence-local"
  export S3_MEDIA_BUCKET="espacigo-media-local"
  export DOCUMENT_VERIFICATION_STORAGE_BACKEND="s3"
  export DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET="espacigo-kyc-identity-local"
  export DOCUMENT_VERIFICATION_SPACE_S3_BUCKET="espacigo-kyc-space-local"
  export DOCUMENT_VERIFICATION_ENCRYPTION_KEYRING_FILE="$PWD/.local/secrets/document-verification-keyring.json"
  export LOCAL_PRIVACY_REPLAY_BACKEND="s3"
  export LOCAL_PRIVACY_REPLAY_BUCKET="espacigo-privacy-replay-local"
  export LOCAL_CONTRACT_ENCRYPTION_KEY_FILE="$PWD/.local/secrets/local_contract_encryption_key"
  export LOCAL_PUBLIC_CATALOG_CURSOR_KEY_FILE="$PWD/.local/secrets/public_catalog_cursor_key"
  export LOCAL_PAYMENT_WEBHOOK_SECRET_FILE="$PWD/.local/secrets/local_payment_webhook_secret"
  export IDENTITY_DECLARATION_KEYRING_FILE="$PWD/.local/secrets/identity-declaration-keyring.json"
  export IDENTITY_RUT_HMAC_KEYRING_FILE="$PWD/.local/secrets/identity-rut-hmac-keyring.json"
  go run ./cmd/api
)
```

### 4. Levantar el Frontend Next.js 16 (`frontend/`)

```bash
cd frontend
pnpm install --frozen-lockfile
API_BACKEND_BASE_URL="http://127.0.0.1:8080/api/v1" \
FRONTEND_PUBLIC_ORIGIN="http://127.0.0.1:3000" \
pnpm dev --hostname 127.0.0.1 --port 3000
```

---

## Verificación de Calidad

- **Backend Go:**
  ```bash
  go test ./...
  go vet ./...
  ```
- **Frontend Next.js (BFF y adaptadores):**
  ```bash
  cd frontend
  node --test tooling/*.test.mjs
  ```
