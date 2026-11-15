# 11 — Plan Maestro de Migración y Limpieza: `espacigo-aws` (100% Local con `aws-emulator` + PostgreSQL 18)

**Fecha de corte:** 15 de noviembre de 2026  
**Rama fuente actualizada:** `origin/main` (`2ec46c3` — `merge(kyc): reconcile identity verification work in progress with main`, incluye migraciones hasta `V000097__commercial_rental_quote_snapshots.sql`)  
**Repositorio destino:** `/home/nandev/Documentos/projects/espacigo-aws` (`https://github.com/HernanEspinozaDev/espacigo-aws.git`, verificado `isEmpty: true` vía `gh`)  
**Emulador AWS local:** `https://127.0.0.1:9297` (`aws-emulator v1.0.385`, cuenta `000000000000`, aislado de la cuenta real `805068224315`)  
**Base de datos local dedicada:** PostgreSQL 18.6 (`espacigo_aws_local`)

---

## 1. Estado Verificado del Paso 1 (`aws-emulator` Activo + `main` Sincronizado)

1. **Sincronización de `main` a `origin/main` (`2ec46c3`):**
   - Se ejecutó `git fetch origin main` y `git merge --ff-only origin/main` en el worktree de `main` (`/home/nandev/Documentos/projects/espaciGo-main-integration`), avanzando desde `a3d57b3` hasta `2ec46c3` (889 archivos actualizados, incluyendo el flujo completo de verificación de identidad KYC nativo, cola de revisores y migraciones `V000084` a `V000097`).
2. **Instalación y arranque de `aws-emulator` (`v1.0.385`):**
   - Binario nativo instalado en `/home/nandev/.local/bin/aws-emulator` (`/home/nandev/.local/opt/aws-emulator/bin/aws-emulator`) e imagen Docker oficial construida desde el paquete RPM LTS (`v1.0.385`, commit `813af480bf7f16d5418b64e4f5644a74a7da11c5`).
   - Servicio en ejecución persistente ligado exclusivamente a `127.0.0.1:9297`.
   - Archivos de conexión y CA TLS generados en `/home/nandev/.aws/emulator/9297/` (`ca.pem`, `config`, `emulator_credentials`, `truststore.p12`) y script de aislamiento `/home/nandev/.aws/emulator/9297.env` (que ejecuta `unset` de `AWS_PROFILE` y de cualquier credencial real antes de apuntar a `https://127.0.0.1:9297`).
   - **Nota sobre telemetría (`aws-emulator`):** Por defecto el binario recopila métricas anónimas de uso; puede desactivarse pasando `-t off` o exportando `EMULATOR_TELEMETRY=off`.
3. **Healthcheck y Smoke Test S3 verificados:**
   - `curl -sf --cacert ~/.aws/emulator/9297/ca.pem https://127.0.0.1:9297/__admin/health` → `ok`
   - `aws sts get-caller-identity` → `Account: "000000000000"`, `Arn: "arn:aws:iam::000000000000:user/emulator"`
   - Smoke test S3 (`create-bucket`, `put-object` con metadatos `key_version=v1`, `head-object`, `get-object`, `delete-object`, `delete-bucket`) completado exitosamente con cifrado `AES256` y checksums `CRC64NVME`.

---

## 2. Arquitectura Objetivo en `espacigo-aws` (100% Local)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Host Local (127.0.0.1) — Cero Dependencias de GCP ni Cuenta AWS Real         │
│                                                                              │
│  ┌──────────────────────────┐        ┌────────────────────────────────────┐  │
│  │ Frontend Next.js 16.4.3  │        │ Backend Go 1.26 (cmd/api :8080)    │  │
│  │ + React 19.3.0 (:3000)   │───────▶│ + cmd/privacy-cleanup-worker       │  │
│  │ BFF /api/backend/[...p]  │  HTTP  │ + cmd/dbmigrate                    │  │
│  │ (Sin Cloud Run OIDC/IAP) │ Loopbk │ (Sin cloud.google.com/go/*)        │  │
│  └──────────────────────────┘        └─────────┬──────────────────┬───────┘  │
│                                                │                  │          │
│                           DATABASE_CONNECTION_MODE=direct   AWS SDK for Go v2│
│                                                │            UsePathStyle=true│
│                                                ▼                  ▼          │
│                                      ┌─────────────────┐  ┌───────────────┐  │
│                                      │ PostgreSQL 18.6 │  │ aws-emulator  │  │
│                                      │ 127.0.0.1:5432  │  │ 127.0.0.1:9297│  │
│                                      │ DB dedicada:    │  │ Acct: 000..000│  │
│                                      │ espacigo_aws_   │  │ S3 Buckets:   │  │
│                                      │ local           │  │ - media       │  │
│                                      │ Roles:          │  │ - evidence    │  │
│                                      │ - admin         │  │ - kyc-id-docs │  │
│                                      │ - migrator      │  │ - space-docs  │  │
│                                      │ - runtime       │  │ - privacy-rep │  │
│                                      │ - kyc_purge     │  └───────────────┘  │
│                                      │ - privacy_wrkr  │                     │
│                                      └─────────────────┘                     │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Plan Maestro de Ejecución por Fases Atómicas

### FASE A: Inventario Exacto de Extracción y Purga (`espaciGo` @ `2ec46c3` → `espacigo-aws`)

#### A.1. Lo que SE ELIMINA (0% rastro de GCP y 0% basura histórica)
1. **Historial y metadatos viejos:**
   - Directorio `.git` antiguo (se inicializa un repositorio virgen con `git init` + `git branch -M main`), `.gcloudignore`, archivo suelto `eliminada`, `.local/`.
2. **Binarios y paquetes exclusivos de GCP (28 directorios eliminados):**
   - **17 comandos en `cmd/`:** `cmd/gcp03origin`, `cmd/gcp03restore`, `cmd/gcp03restore-credential`, `cmd/gcp03scratch`, `cmd/gcp03scratch-credential`, `cmd/gcp04baseline-restore`, `cmd/gcp04preflight`, `cmd/gcp04replay-credential`, `cmd/gcp04w7b1`, `cmd/gcp04w7b1probe`, `cmd/gcp04w7bootstrap`, `cmd/gcp04w8catalogprobe`, `cmd/gcp04w9release`, `cmd/gcp06adminbootstrap`, `cmd/gcp06sqlaccess`, `cmd/gcp06sqlinspect`, `cmd/gcp06testaccess`.
   - **11 paquetes en `internal/`:** `internal/gcp03origin`, `internal/gcp03restore`, `internal/gcp03restorecredential`, `internal/gcp03scratch`, `internal/gcp04credential`, `internal/gcp04w7b1`, `internal/gcp04w8release`, `internal/gcp04w9release`, `internal/gcp06sqlaccess`, `internal/gcp06sqlinspect`, `internal/gcp06testaccess`.
3. **Adaptadores y conectores de GCP en el Backend:**
   - `internal/database/cloudsql/` (`connector.go`, `connector_test.go`).
   - `internal/adapters/cloudstorage/` (`store.go`, `media_store.go`, `replay_registry.go` y sus tests de GCS).
   - Dependencias en `go.mod` / `go.sum`: `cloud.google.com/go/cloudsqlconn`, `cloud.google.com/go/storage`, `cloud.google.com/go/auth`, `cloud.google.com/go/auth/oauth2adapt`, `google.golang.org/api` y todas las indirectas de `cloud.google.com/*`.
4. **Dockerfiles, Cloud Build, Terraform y Scripts de GCP:**
   - **14 Dockerfiles de GCP:** `Dockerfile.gcp03-v64-transfer`, `Dockerfile.gcp03origin`, `Dockerfile.gcp03origin-import`, `Dockerfile.gcp03origin-recovery`, `Dockerfile.gcp03restore`, `Dockerfile.gcp04-baseline-restore`, `Dockerfile.gcp04-replay-smoke`, `Dockerfile.gcp04-w7-b1-runtime`, `Dockerfile.gcp04-w7-bootstrap`, `Dockerfile.gcp04-w8-catalog-probe`, `Dockerfile.gcp06-admin-bootstrap`, `Dockerfile.gcp06-sql-inspect`, `Dockerfile.gcp06-test-access`, `Dockerfile.runtime-migrator`.
   - **7 pipelines de Cloud Build:** `cloudbuild.yaml`, `cloudbuild-db-recovery-inspect.yaml`, `cloudbuild-db-recovery-provision.yaml`, `cloudbuild-images.yaml`, `cloudbuild-job-provision.yaml`, `cloudbuild-migrator-image.yaml`, `cloudbuild-web-images.yaml`.
   - **Infraestructura y scripts de GCP:** `infra/terraform/environments/testprod/gcp*`, `scripts/gcp/`, `scripts/gcp*.sh`, `scripts/cloudsql*`.
   - **Planes y reportes históricos de evidencia:** todo el árbol `planning/` con miles de reportes de corridas pasadas (`evidencia_vi04_*`, `evidencia_vi05_*`, `gcp-03`, `gcp-04`, `gcp-06`, etc.).
5. **Rastros de GCP en el Frontend (`frontend/`):**
   - `frontend/src/lib/api/cloud-run-identity.ts` (obtención de token OIDC contra `metadata.google.internal`) y su invocación en `frontend/src/app/api/backend/[...path]/route.ts` (`X-Serverless-Authorization`, variables `CLOUD_RUN_*`).

#### A.2. Lo que SE CONSERVA y MIGRA (Código Vivo y Limpio)
1. **Comandos Backend (`cmd/`):**
   - `cmd/api` (servidor HTTP principal + workers integrados).
   - `cmd/dbmigrate` (migrador transaccional con advisory lock, checksums SHA-256 y bootstrap de roles).
   - `cmd/privacy-cleanup-worker` (worker de purga ARCO/retención).
   - `cmd/gadm-admin`, `cmd/dataset-loader`, `cmd/netprobe`, `cmd/kyc-fake-provider`, `cmd/local-kyc-mailbox`, `cmd/rentaldemo`, `cmd/wizardtrialsetup`.
2. **Dominio y Adaptadores (`internal/`):**
   - Todos los dominios de negocio vivos (`adminglobal`, `adminlocal`, `booking`, `calendar`, `contract`, `conversation`, `damageclaim`, `database`, `dbbootstrap`, `dispute`, `documentverification`, `gallery`, `identity`, `identityverification`, `localnotice`, `m02local`, `media`, `migrator`, `occupancy`, `openapicontract`, `operation`, `platform`, `pricing`, `privacy`, `publiccatalog`, `rentaldemo`, `rentalflow`, `reputation`, `spaces`, `verification`).
   - Adaptadores vivos (`clamav`, `credentials`, `devauth`, `documentcodec`, `documentfs`, `documentscanhttp`, `documentstorage`, `evidencefs`, `fakebooking`, `mediafs`, `password`, `postgres`, `zohoauth`), renombrando `internal/adapters/documentgcs` → `internal/adapters/documents3` y creando `internal/adapters/s3storage`.
3. **Base de Datos (`db/`):**
   - `db/migrations/` (`V000001` a `V000097`), `db/queries/`, `db/schema/`, `db/seeds/`, `db/repair/`, `sqlc.yaml`.
4. **Frontend (`frontend/`):**
   - Aplicación completa Next.js `16.4.3` + React `19.3.0`, BFF limpio en `src/app/api/backend/[...path]/route.ts`, suites de pruebas unitarias/integración (`frontend/tooling/*.mjs`) y E2E Playwright (`frontend/tests/*.spec.ts`).
5. **Documentación y Contenedores Locales:**
   - `Dockerfile`, `Dockerfile.dbmigrate`, `compose.yaml` (actualizado para `aws-emulator` y `espacigo_aws_local`), `DESIGN.md`, y la suite de arquitectura AWS en `docs/architecture/aws-proposal/`.

---

### FASE B: Refactorización a AWS SDK for Go v2 (`internal/adapters/s3storage`) y `aws-emulator`

1. **Dependencias Go (`go.mod`):**
   - Eliminar todos los paquetes `cloud.google.com/go/*` y `google.golang.org/api`.
   - Incorporar **AWS SDK for Go v2**:
     - `github.com/aws/aws-sdk-go-v2`
     - `github.com/aws/aws-sdk-go-v2/config`
     - `github.com/aws/aws-sdk-go-v2/credentials`
     - `github.com/aws/aws-sdk-go-v2/service/s3`
2. **Nuevo paquete `internal/adapters/s3storage`:**
   - **`client.go`:** Constructor común `NewS3Client(ctx, Config)` que soporta por defecto el entorno local de `aws-emulator`:
     - Endpoint por defecto o vía `AWS_ENDPOINT_URL` (`https://127.0.0.1:9297`).
     - `UsePathStyle = true` (obligatorio para direccionamiento local por path `https://127.0.0.1:9297/<bucket>/<key>`).
     - Carga automática del certificado CA local (`AWS_CA_BUNDLE` / `~/.aws/emulator/9297/ca.pem`) en el `http.Transport` con `MinVersion: tls.VersionTLS12` (sin desactivar jamás la verificación TLS).
     - Región por defecto `us-east-1` (o `AWS_REGION`) y credenciales de `emulator_credentials` / variables de entorno locales para la cuenta `000000000000`.
   - **`store.go` (`s3storage.Store`):** Reemplazo directo de `cloudstorage.Store`:
     - Implementa `Put(ctx, id, blob)`, `PutWithMetadata(ctx, id, blob, metadata)` usando `IfNoneMatch: aws.String("*")` (semántica create-only idéntica a `DoesNotExist: true`), `Get(ctx, id)`, `GetWithMetadata(ctx, id, maxBytes)` (con `HeadObject` + `GetObject` acotado por `io.LimitReader` y `IfMatch: head.ETag`), y `Delete(ctx, id)`.
   - **`media_store.go` (`s3storage.MediaStore`):** Reemplazo directo de `cloudstorage.MediaStore`:
     - Implementa `Put(ctx, key, b, mime)` (`ContentType: "image/webp"`, `CacheControl: "private, no-store"`, `IfNoneMatch: "*"`, `Metadata: {"sha256": ...}`, `Provider: "s3"`), `Get(ctx, key, generation)`, `Generation(ctx, key)` y `Delete(ctx, key, generation)`.
   - **`replay_registry.go` (`s3storage.ReplayRegistry`):** Reemplazo directo de `cloudstorage.ReplayRegistry`:
     - Implementa `Load(ctx)` y `CompareAndSwap(ctx, expectedRevision, manifest)` sobre S3 (`privacy-replay-v1.json`) utilizando `IfNoneMatch: "*"` (cuando `expectedRevision == ""`) o `IfMatch` con revisión/ETag para garantizar atomicidad CAS frente a `aws-emulator` y S3 real.
3. **Renombrado `internal/adapters/documentgcs` → `internal/adapters/documents3`:**
   - Conserva intacto el cifrado de sobre AES-256-GCM con keyring (`documentcodec.Seal` / `documentcodec.Open` y metadato `key_version`), inyectando dos instancias de `s3storage.Store` (una para documentos de identidad KYC y otra para documentos de dominio de espacios).
4. **Cableado en `cmd/api` y `cmd/privacy-cleanup-worker`:**
   - En `cmd/api/config.go`, `cmd/api/database_pool.go` y `cmd/privacy-cleanup-worker/main.go`: eliminar `DATABASE_CONNECTION_MODE=cloudsql` y `CLOUD_SQL_*`, dejando exclusivamente `DATABASE_CONNECTION_MODE=direct`.
   - En `cmd/api/runtime_storage.go`, `cmd/api/runtime_privacy_replay.go` y `cmd/privacy-cleanup-worker/main.go`: reemplazar el backend `"gcs"` por `"s3"` y las variables `CLOUD_STORAGE_*` / `DOCUMENT_VERIFICATION_*_GCS_BUCKET` por:
     - `BLOB_STORE_BACKEND=s3` (o `filesystem`)
     - `S3_STORAGE_BUCKET=espacigo-evidence-local`
     - `S3_STORAGE_OBJECT_PREFIX`
     - `S3_MEDIA_BUCKET=espacigo-media-local`
     - `DOCUMENT_VERIFICATION_STORAGE_BACKEND=s3` (o `filesystem`)
     - `DOCUMENT_VERIFICATION_IDENTITY_S3_BUCKET=espacigo-kyc-identity-local`
     - `DOCUMENT_VERIFICATION_SPACE_S3_BUCKET=espacigo-kyc-space-local`
     - `LOCAL_PRIVACY_REPLAY_BACKEND=s3` (o `filesystem`)
     - `LOCAL_PRIVACY_REPLAY_BUCKET=espacigo-privacy-replay-local`
5. **Script de Bootstrap Local de Buckets (`scripts/local-aws-bootstrap.sh`):**
   - Crea de forma idempotente en `aws-emulator` (`https://127.0.0.1:9297`, cuenta `000000000000`) los 5 buckets locales separados por propósito:
     1. `espacigo-evidence-local`
     2. `espacigo-media-local`
     3. `espacigo-kyc-identity-local`
     4. `espacigo-kyc-space-local`
     5. `espacigo-privacy-replay-local`

---

### FASE C: Base de Datos PostgreSQL Local Aislada (`espacigo_aws_local`)

1. **Diagnóstico del estado actual de PostgreSQL local:**
   - Actualmente corre el contenedor `espacigo_localbackend_20261024-database-1` (PostgreSQL `18.6-1.pgdg13+2`) y el servicio local en `127.0.0.1:5432`, con la base `espacigo_local` usada por el repositorio antiguo.
2. **Aislamiento de la nueva base de datos `espacigo_aws_local`:**
   - Sin tocar ni alterar `espacigo_local`, se aprovisionará la base de datos dedicada **`espacigo_aws_local`** junto con los roles requeridos por el modelo de privilegios mínimos del proyecto:
     - `espacigo_admin` (propietario administrativo / bootstrap)
     - `espacigo_migrator` (`NOLOGIN`, propietario de los objetos del esquema `app`)
     - `espacigo_runtime` (rol de ejecución para `cmd/api` con permisos DML/EXECUTE acotados)
     - `espacigo_kyc_retention_purge` (`NOLOGIN`, rol de purga de declaraciones KYC)
     - `espacigo_privacy_cleanup_worker` (`NOLOGIN`, rol asumido por `cmd/privacy-cleanup-worker` vía `SET ROLE`)
3. **Ejecución limpia de migraciones (`V000001` → `V000097`):**
   - Ejecución desde cero de `cmd/dbmigrate` sobre `espacigo_aws_local`, aplicando las 97 migraciones versionadas en orden estricto, verificando `app.schema_migrations` (97 filas, 0 pendientes, checksums SHA-256 íntegros) y los permisos de los roles.

---

### FASE D: Pruebas Integrales End-to-End Locales

1. **Auditoría Cero-GCP automatizada:**
   - Verificación con `grep`/`ripgrep` en todo `/home/nandev/Documentos/projects/espacigo-aws` de que existen **0 coincidencias** de `cloud.google.com`, `CLOUD_SQL`, `CLOUD_STORAGE`, `cloudsql`, `gcloud`, `cloudbuild`, `metadata.google.internal` o `X-Serverless-Authorization`.
2. **Compilación y Tests Backend Go:**
   - `go mod tidy` + `go vet ./...` + `go build ./cmd/...`
   - Ejecución de tests unitarios y de integración de `internal/adapters/s3storage`, `internal/adapters/documents3`, `cmd/api`, `cmd/dbmigrate` y `cmd/privacy-cleanup-worker` apuntando en vivo contra `aws-emulator` (`https://127.0.0.1:9297`) y PostgreSQL (`espacigo_aws_local`).
3. **Prueba Real de Subida/Lectura/Borrado Cifrado en S3 (`aws-emulator`):**
   - Subida y lectura de documento KYC cifrado (`documents3` → bucket `espacigo-kyc-identity-local`), imagen WebP de espacio (`s3storage.MediaStore` → bucket `espacigo-media-local`), evidencia operativa (`s3storage.Store` → bucket `espacigo-evidence-local`) y snapshot de supresión (`s3storage.ReplayRegistry` → bucket `espacigo-privacy-replay-local`).
4. **Compilación y Tests Frontend Next.js 16 / BFF:**
   - `npm test` / pruebas del gateway BFF (`frontend/tooling/backend-gateway-credentials.test.mjs`, `bff-gateway-empty-response.test.mjs`, etc.) y verificación de salud full-stack (`Frontend -> BFF -> Go API -> PostgreSQL espacigo_aws_local + S3 aws-emulator`).

---

### FASE E: Inicialización Git y Publicación en GitHub (`espacigo-aws`)

1. **Inicialización limpia:**
   - En `/home/nandev/Documentos/projects/espacigo-aws`:
     - `git init`
     - `git branch -M main`
     - Configuración de `.gitignore` limpio (excluyendo `.env*`, secretos locales, `.next/`, `node_modules/`, binarios compilados).
2. **Commit fundacional y vinculación remota:**
   - `git remote add origin https://github.com/HernanEspinozaDev/espacigo-aws.git`
   - Commit estructurado con la arquitectura limpia en AWS SDK v2 + `aws-emulator` + PostgreSQL 18 (`V000001`–`V000097`).
3. **Autenticación y Push con `gh`:**
   - Uso de la sesión activa de `gh` (`HernanEspinozaDev`, verificada con scopes `repo`, `workflow`) para ejecutar `git push -u origin main` y confirmar el estado del remoto con `git status` y `gh repo view`.

---

## 4. Orquestación de Subagentes para la Ejecución

En cuanto autorices el inicio de la ejecución de las Fases A–E, desplegaré subagentes paralelos especializados para acelerar el trabajo sin bloqueos:
- **Subagente 1 (Extractor & Purgador GCP — Fase A):** Creación de `/home/nandev/Documentos/projects/espacigo-aws` desde `main` (`2ec46c3`), eliminación quirúrgica de todos los directorios/archivos de GCP y limpieza del BFF en Next.js.
- **Subagente 2 (Ingeniero AWS SDK v2 & S3 Storage — Fase B):** Implementación completa de `internal/adapters/s3storage`, `internal/adapters/documents3`, refactor de `cmd/api` y `cmd/privacy-cleanup-worker`, y creación de tests de integración contra `https://127.0.0.1:9297`.
- **Subagente 3 (Ingeniero PostgreSQL Local & Migraciones — Fase C):** Aprovisionamiento de `espacigo_aws_local`, bootstrap de roles y ejecución/verificación de las 97 migraciones con `cmd/dbmigrate`.
- **Coordinador Principal (Fases D & E):** Verificación integral end-to-end contra `aws-emulator` + `espacigo_aws_local`, auditoría Cero-GCP y `git push -u origin main` a `https://github.com/HernanEspinozaDev/espacigo-aws.git`.
