# AGENTS.md — Entornos y ramas de EspaciGo

## Flujo obligatorio

| Rama | Entorno | Operación |
| --- | --- | --- |
| `desarrollo-local` | AWS Emulator, PostgreSQL y Mailpit locales | Cambios de código e infraestructura, fixtures y pruebas de desarrollo. |
| `main` | Producción en AWS real | Recibe cambios mediante PR desde `desarrollo-local` con CI aprobada. GitHub Actions inicia la entrega continua del SHA integrado. |

1. Trabaja y prueba en `desarrollo-local`. Publica esa rama y promueve a `main` por PR con merge commit para conservar la relación entre ambas ramas. Usa Conventional Commits.
2. Ejecuta `scripts/local-dev.sh` y usa credenciales del emulador sólo para desarrollo local. Mantén `.local/`, dumps, tokens y llaves fuera de Git.
3. Opera producción mediante el pipeline AWS de `main`. GitHub Actions asume un rol limitado vía OIDC y restringido al entorno `produccion`. Para operaciones humanas, usa AWS CLI v2 con `aws login` y un perfil de producción explícito en la Región seleccionada. Mantén las sesiones, los endpoints y los secretos separados de los del emulador.
4. Mantén Go en AWS SDK v2. Usa endpoint local y `UsePathStyle` para S3 emulado. Usa la resolución estándar de AWS para producción. Conserva la arquitectura AWS del repositorio.
5. Mantén las funciones de ensayo y los datos sintéticos en el entorno local. La demo de arriendo usa un pago de prueba y una bandera de arranque local.
6. Sigue el [plan de despliegue y CI/CD](docs/operations/aws-production-deployment.md) para preparar el destino de AWS, activar el pipeline y recuperar un despliegue.

---

## 1. Entorno de desarrollo con `aws-emulator`

El proyecto original (`espaciGo`) estaba fuertemente acoplado a Google Cloud Platform (Cloud SQL, Cloud Run, Cloud Storage, y pipelines estrictos de Cloud Build con `release_main.py`). Esto provocaba:
- **Bloqueo continuo en desarrollo**: Cualquier cambio en la base de datos o API requería preflights complejos, validación de ledgers y aprobaciones de infraestructura en la nube.
- **Costos y límites en pruebas**: Servicios externos como KYC (Didit) y webhooks exigían URLs HTTPS públicas hacia internet y pagos por transacción, impidiendo probar el flujo en `localhost`.
- **Falta de soberanía local**: No se podía correr el sistema completo de forma autosuficiente sin conexión a internet ni dependencias cloud.

### Servicios locales en `espacigo-aws`
El entorno de desarrollo corre localmente mediante `aws-emulator` en `https://127.0.0.1:9297`.
Esto proporciona:
- **Servicios AWS emulados**: Los servicios S3, SSM Parameter Store, CloudWatch Logs e IAM corren localmente respondiendo a llamadas del AWS SDK for Go v2 y AWS CLI.
- **Velocidad y libertad total**: Podemos crear migraciones de base de datos (`db/migrations/`), ejecutar pruebas E2E, modificar modelos y subir archivos multimedia a S3 local sin costo ni latencia de red.
- **Aislamiento Criptográfico**: Las credenciales locales (`~/.aws/emulator/9297.env`) aíslan completamente el emulador de los perfiles de producción.

---

## 2. Inventario Completo de Accesos y Servicios Locales

Todos los servicios y secretos necesarios para levantar, probar y auditar la plataforma se encuentran disponibles localmente:

| Servicio / Componente | Dirección / URL Local | Protocolo / Puerto | Credenciales / Ubicación |
|---|---|---|---|
| **Frontend Web (Next.js)** | [http://127.0.0.1:3000](http://127.0.0.1:3000) | HTTP :3000 | No requiere autenticación previa para navegar. |
| **Backend API (Go)** | [http://127.0.0.1:8080](http://127.0.0.1:8080) | HTTP :8080 | Expone `/api/v1/...` (consumido vía BFF Next.js). |
| **AWS Emulator** | `https://127.0.0.1:9297` | HTTPS :9297 | `Account: 000000000000`, CA TLS: `~/.aws/emulator/9297/ca.pem`, Env: `~/.aws/emulator/9297.env`. |
| **PostgreSQL 18** | `127.0.0.1:5432` | TCP :5432 | DB: `espacigo_aws_local`<br/>Admin: `espacigo_admin`<br/>App Login: `espacigo_api_login`<br/>Runtime: `espacigo_runtime` |
| **Mailpit (SES / Emails)** | [http://127.0.0.1:8025](http://127.0.0.1:8025) (Web UI)<br/>`127.0.0.1:1025` (SMTP) | HTTP :8025<br/>SMTP :1025 | Bandeja de entrada web para inspeccionar correos de verificación y arriendos. |

### Ubicación de Secretos y Certificados (No se comitean)
- **Secretos de Base de Datos**: `.local/secrets/`
  - `.local/secrets/db_admin_password`: Clave de `espacigo_admin`.
  - `.local/secrets/runtime_password`: Clave de `espacigo_api_login` / `espacigo_runtime`.
- **Configuración de AWS Emulator**:
  - `~/.aws/emulator/9297.env`: Carga automáticamente las variables seguras de entorno local.
  - `~/.aws/emulator/9297/ca.pem`: Certificado CA local para llamadas HTTPS con `curl` o AWS CLI.

---

## 3. Comparativa: Plan de Arquitectura AWS (Nube) vs. Entorno Local (`aws-emulator`)

El diseño en `espacigo-aws` mantiene equivalencia directa componente por componente entre la arquitectura destino de producción en AWS y el entorno emulado local:

| Componente de Arquitectura | Plan de Arquitectura AWS (Nube / Prod) | Implementación Local en `espacigo-aws` |
|---|---|---|
| **Cómputo Backend** | AWS ECS Fargate en subnets privadas tras Application Load Balancer (ALB). | Binario Go nativo (`cmd/api`) corriendo en `http://127.0.0.1:8080`. |
| **Cómputo Frontend** | Next.js en Vercel o contenedor ECS Fargate con CloudFront CDN. | Next.js dev server en `http://127.0.0.1:3000` con BFF loopback. |
| **Base de Datos Relacional** | Amazon RDS PostgreSQL 18 multi-AZ (`db.t4g.micro` / provisionado con Terraform). | Contenedor Docker PostgreSQL 18.6 (`espacigo_localbackend_20261024-database-1`). |
| **Almacenamiento S3** | Amazon S3 (5 buckets con SSE-KMS, Versioning y Bloqueo Público). | S3 en `aws-emulator:9297` (`espacigo-media-local`, `espacigo-evidence-local`, `espacigo-kyc-identity-local`, etc.). |
| **Configuración & Parámetros** | AWS Systems Manager (SSM) Parameter Store (`/espacigo/prod/*`). | SSM en `aws-emulator` (`/espacigo/local/*`), aprovisionado con Terraform local. |
| **Logs & Auditoría** | Amazon CloudWatch Log Groups con retención configurable. | CloudWatch Logs en `aws-emulator` + salida directa a terminal. |
| **Roles & Privilegios** | IAM Task Roles y Execution Roles de mínimo privilegio. | IAM Roles emulados en `aws-emulator` (`arn:aws:iam::000000000000:role/...`). |
| **Servicio de Correo** | Amazon Simple Email Service (SES). | Servidor SMTP Mailpit local en `127.0.0.1:1025` con interfaz web en `:8025`. |
| **Procesamiento Asíncrono** | AWS SQS FIFO / EventBridge para tareas en segundo plano. | Workers integrados en Go (`cmd/privacy-cleanup-worker`). |

---

## 4. Comandos de Operación del Repositorio

El script [`scripts/local-dev.sh`](./scripts/local-dev.sh) es la herramienta central para gestionar todo el stack:

```bash
# 1. Verificar estado y salud de todos los servicios locales
./scripts/local-dev.sh status

# 2. Inicializar / reaprovisionar infraestructura en aws-emulator y permisos PostgreSQL
./scripts/local-dev.sh bootstrap

# 3. Correr pruebas automatizadas (Go y Next.js)
./scripts/local-dev.sh test

# 4. Iniciar servidor API Go en segundo plano o foreground
./scripts/local-dev.sh api

# 5. Iniciar servidor Next.js
./scripts/local-dev.sh web
```

---
