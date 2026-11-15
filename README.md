# espaciGo AWS (Local-First Architecture)

Plataforma SaaS para la gestión, arriendo flexible, verificación documental (KYC/espacios) y legalización de espacios comerciales e inmobiliarios en Chile, operando **100% AWS-native en entorno local** mediante `aws-emulator` y PostgreSQL 18.

---

## 1. Visión General de la Arquitectura

Esta edición elimina por completo cualquier dependencia de Google Cloud Platform (cero Cloud Run, Cloud SQL, Cloud Storage y Cloud Build) y opera íntegramente sobre servicios de AWS emulados localmente, garantizando más del **95% de paridad funcional** con el despliegue de producción en AWS (`us-east-2` Ohio / AWS Chile):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Host Local (127.0.0.1) — Cero Dependencias de GCP ni Cuenta AWS Real         │
│                                                                              │
│  ┌──────────────────────────┐        ┌────────────────────────────────────┐  │
│  │ Frontend Next.js 16.4.3  │        │ Backend Go 1.27 (cmd/api :8080)    │  │
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
│                                      │ espacigo_aws_   │  │ S3 Buckets (5)│  │
│                                      │ local           │  │ SSM Params(13)│  │
│                                      │ Roles aislados  │  │ CW Logs (4)   │  │
│                                      │ 97 migraciones  │  │ IAM Roles (2) │  │
│                                      └─────────────────┘  └───────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Servicios, Puertos y Endpoints Locales

| Servicio | URL / Endpoint Local | Descripción |
| :--- | :--- | :--- |
| **Frontend Web** | `http://127.0.0.1:3000` | UI Next.js 16 + BFF (`src/app/api/backend/`) |
| **Backend API** | `http://127.0.0.1:8080` | Núcleo de negocio en Go 1.27 (`cmd/api`) |
| **`aws-emulator`** | `https://127.0.0.1:9297` | Emulador oficial AWS (S3, SSM, Logs, IAM) |
| **PostgreSQL 18** | `127.0.0.1:5432` | Base de datos `espacigo_aws_local` |
| **Mailpit** | `http://127.0.0.1:8025` | UI Web para inspección de emails de prueba (SMTP `:1025`) |

---

## 3. CLI Unificado de Desarrollo (`scripts/local-dev.sh`)

Para gestionar todo el ciclo de vida local de forma sencilla y estandarizada, utiliza el script [`scripts/local-dev.sh`](./scripts/local-dev.sh):

```bash
# 1. Comprobar la salud de todos los componentes (aws-emulator, S3, SSM y DB)
./scripts/local-dev.sh status

# 2. Aprovisionar buckets S3, parámetros SSM, logs y roles IAM en el emulador
./scripts/local-dev.sh bootstrap

# 3. Ejecutar las suites de pruebas de Go (S3/Crypto) y del frontend BFF (634 tests)
./scripts/local-dev.sh test

# 4. Iniciar el servidor API Go conectado al stack emulado
./scripts/local-dev.sh api

# 5. Iniciar el frontend Next.js en modo desarrollo
./scripts/local-dev.sh web
```

---

## 4. Infraestructura como Código (IaC con Terraform)

El directorio [`infra/terraform/`](./infra/terraform/) contiene módulos reutilizables y entornos desacoplados:

- **`infra/terraform/modules/`**:
  - `s3_storage`: 5 buckets S3 con Versioning, SSE-S3 AES-256 y Public Access Block.
  - `ssm_parameters`: Parámetros de configuración bajo `/espacigo/${env}/*`.
  - `observability`: CloudWatch Log Groups para API, Web, Migrator y Privacy Worker.
  - `iam`: Roles y políticas de privilegios mínimos para ECS Task y Execution.
- **`infra/terraform/environments/local-emulator/`**: Provider AWS apuntando a `https://127.0.0.1:9297` con CA Bundle local.
- **`infra/terraform/environments/aws-main/`**: Configuración multi-región agnóstica para producción (`us-east-2` Ohio / AWS Chile) con descubrimiento dinámico de AZs, RDS PostgreSQL 18 `db.t4g.micro`, VPC y alerta de AWS Budgets (< USD $30/mes).

---

## 5. Ciclo de Modificaciones: "Lo que está en `main` sirve para `aws-emulator`"

1. **Frontend:** Edita en `frontend/` y verifica cambios en `http://127.0.0.1:3000`. Corre `cd frontend && node --test tooling/*.test.mjs` antes de comitear.
2. **Backend:** Edita en `internal/` o `cmd/` usando AWS SDK for Go v2. Corre `go test ./internal/adapters/...` para validar integración con `aws-emulator`.
3. **Migraciones:** Agrega archivos `V0000XX__descripcion.sql` en `db/migrations/` y aplícalos con `cmd/dbmigrate`.
4. **Infraestructura:** Ajusta los módulos en `infra/terraform/modules/` y aplica en `local-emulator`.

> **Nota:** La guía interna y detallada para agentes y operadores de desarrollo local reside en `.local/` (excluida del control de versiones).
