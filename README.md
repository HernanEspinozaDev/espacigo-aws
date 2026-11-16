# espaciGo AWS (Local-First Architecture)

Plataforma SaaS para la gestión, arriendo flexible, verificación documental (KYC/espacios) y legalización de espacios comerciales e inmobiliarios en Chile, operando **100% AWS-native en entorno local** mediante `aws-emulator` y PostgreSQL 18.

> [!IMPORTANT]
> **DIRECTIVA PARA AGENTES DE IA Y DESARROLLADORES:**
> - **NUNCA solicites credenciales IAM de AWS ni intentes conectar a cuentas en la nube.** Este repositorio está configurado 100% para desarrollo local soberano.
> - **Todos los accesos, URLs y contraseñas ya están preconfigurados.** Consulta [AGENTS.md](./AGENTS.md) y [docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md](./docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md).
> - **0% dependencias de Google Cloud Platform (GCP).** Cero Cloud Run, Cloud SQL, Cloud Storage y Cloud Build.

---

## 1. Visión General de la Arquitectura

Esta edición elimina por completo cualquier dependencia de proveedores de nube externos y opera íntegramente sobre servicios de AWS emulados localmente, garantizando más del **95% de paridad funcional** con el plan de arquitectura para producción en AWS (`us-east-2` Ohio / AWS Chile):

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

| Servicio | URL / Endpoint Local | Protocolo / Puerto | Descripción |
| :--- | :--- | :--- | :--- |
| **Frontend Web** | [http://127.0.0.1:3000](http://127.0.0.1:3000) | HTTP :3000 | UI Next.js 16 + BFF (`src/app/api/backend/`) |
| **Backend API** | [http://127.0.0.1:8080](http://127.0.0.1:8080) | HTTP :8080 | Núcleo de negocio en Go 1.27 (`cmd/api`) |
| **`aws-emulator`** | `https://127.0.0.1:9297` | HTTPS :9297 | Emulador oficial AWS (S3, SSM, Logs, IAM). `Account: 000000000000` |
| **PostgreSQL 18** | `127.0.0.1:5432` / `172.24.0.3:5432` | TCP :5432 | Base de datos `espacigo_aws_local` |
| **Mailpit** | [http://127.0.0.1:8025](http://127.0.0.1:8025) | HTTP :8025 / SMTP :1025 | Interfaz Web para correos transaccionales (Emulador SES) |

---

## 3. Comparativa: Plan de Arquitectura AWS de Producción vs. Entorno Local

| Componente | Arquitectura AWS en la Nube (Producción) | Entorno Local en `espacigo-aws` |
|---|---|---|
| **Cómputo Backend** | AWS ECS Fargate tras Application Load Balancer (ALB). | Binario Go nativo (`cmd/api`) en `:8080`. |
| **Cómputo Frontend** | Next.js en Vercel o contenedor ECS con CloudFront CDN. | Next.js dev server en `:3000`. |
| **Base de Datos** | Amazon RDS PostgreSQL 18 Multi-AZ (`db.t4g.micro`). | Contenedor Docker PostgreSQL 18 (`espacigo_aws_local`). |
| **Almacenamiento (S3)** | Amazon S3 con cifrado SSE-KMS y Versioning. | S3 en `aws-emulator:9297` (5 buckets `espacigo-*-local`). |
| **Parámetros & Config** | AWS Systems Manager (SSM) Parameter Store. | SSM Parameter Store en `aws-emulator` (`/espacigo/local/*`). |
| **Servicio de Correos** | Amazon Simple Email Service (SES). | Servidor SMTP Mailpit local en `:1025` y Web UI en `:8025`. |
| **Logs & Métricas** | Amazon CloudWatch Log Groups. | CloudWatch Logs en `aws-emulator` y salida a consola. |
| **Costos de Pruebas** | Requiere facturación AWS y tarjetas de crédito. | **$0 USD (100% gratuito y sin límites)**. |

Para el análisis técnico detallado, consulta [docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md](./docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md) y los 12 reportes en [docs/architecture/aws-proposal/](./docs/architecture/aws-proposal/).

---

## 4. CLI Unificado de Desarrollo (`scripts/local-dev.sh`)

Para gestionar todo el ciclo de vida local de forma sencilla y estandarizada, utiliza el script [`scripts/local-dev.sh`](./scripts/local-dev.sh):

```bash
# 1. Comprobar la salud de todos los componentes (aws-emulator, S3, SSM y DB)
./scripts/local-dev.sh status

# 2. Aprovisionar buckets S3, parámetros SSM, logs y roles IAM en el emulador
./scripts/local-dev.sh bootstrap

# 3. Ejecutar las suites de pruebas de Go (S3/Crypto) y del frontend BFF
./scripts/local-dev.sh test

# 4. Iniciar el servidor API Go conectado al stack emulado
./scripts/local-dev.sh api

# 5. Iniciar el frontend Next.js en modo desarrollo
./scripts/local-dev.sh web
```

---

## 5. Infraestructura como Código (IaC con Terraform)

El directorio [`infra/terraform/`](./infra/terraform/) contiene módulos reutilizables y entornos desacoplados:

- **`infra/terraform/modules/`**:
  - `s3_storage`: 5 buckets S3 con Versioning, SSE-S3 AES-256 y Public Access Block.
  - `ssm_parameters`: Parámetros de configuración bajo `/espacigo/${env}/*`.
  - `observability`: CloudWatch Log Groups para API, Web, Migrator y Privacy Worker.
  - `iam`: Roles y políticas de privilegios mínimos para ECS Task y Execution.
- **`infra/terraform/environments/local-emulator/`**: Provider AWS apuntando a `https://127.0.0.1:9297` con CA Bundle local.
- **`infra/terraform/environments/aws-main/`**: Configuración multi-región agnóstica para producción (`us-east-2` Ohio / AWS Chile) con descubrimiento dinámico de AZs, RDS PostgreSQL 18 `db.t4g.micro`, VPC y alerta de AWS Budgets (< USD $30/mes).

---

## 6. Ciclo de Modificaciones: "Lo que está en `main` sirve para `aws-emulator`"

1. **Frontend:** Edita en `frontend/` y verifica cambios en `http://127.0.0.1:3000`. Corre `cd frontend && pnpm test` antes de comitear.
2. **Backend:** Edita en `internal/` o `cmd/` usando AWS SDK for Go v2. Corre `go test ./internal/adapters/...` para validar integración con `aws-emulator`.
3. **Migraciones:** Agrega archivos `V0000XX__descripcion.sql` en `db/migrations/` y aplícalos con `cmd/dbmigrate`.
4. **Infraestructura:** Ajusta los módulos en `infra/terraform/modules/` y aplica en `local-emulator`.
