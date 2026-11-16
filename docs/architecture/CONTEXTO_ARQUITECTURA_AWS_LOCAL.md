# Guía de Contexto de Arquitectura: Plan AWS vs. Entorno Local Emulado

> [!IMPORTANT]
> **DIRECTIVA PARA CUALQUIER AGENTE DE IA O DESARROLLADOR:**
> - **NUNCA solicites credenciales IAM de AWS ni intentes conectar a cuentas en la nube.** Este repositorio está configurado 100% para desarrollo local soberano.
> - **Todos los accesos, URLs y contraseñas ya están preconfigurados.** Consulta [AGENTS.md](file:///home/nandev/Documentos/projects/espacigo-aws/AGENTS.md) y [docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md](file:///home/nandev/Documentos/projects/espacigo-aws/docs/architecture/GUIA_ARQUITECTURA_LOCAL_AWS.md).
> - **0% dependencias de Google Cloud Platform (GCP).** Cero Cloud Run, Cloud SQL, Cloud Storage y Cloud Build.

---

## 1. ¿Por qué trabajamos con `aws-emulator` y por qué se migró desde GCP?

En el proyecto original (`espaciGo`), la plataforma dependía de Google Cloud Platform (GCP). Esa arquitectura generaba fricción crítica:
- **Bloqueo por Cloud SQL**: Toda migración de base de datos debía procesarse mediante Cloud Build (`release_main.py`), obligando a sincronizar ledgers remotos, emitir preflights y esperar rollouts en la nube antes de poder probar el código.
- **Costos y bloqueos en integraciones**: Probar servicios como verificación de identidad biométrica (Didit) o notificaciones por correo requería pagar por uso en proveedores de nube y configurar túneles hacia internet.
- **Lentitud en el ciclo de feedback**: El desarrollador o agente no podía probar cambios de inmediato en su máquina.

### La Solución en `espacigo-aws`:
Se migró la arquitectura a **AWS-Native** y se adaptó para correr **100% de manera local**:
1. **Emulador AWS Oficial (`aws-emulator` v1.0.385)**: Corre en `https://127.0.0.1:9297` bajo la cuenta aislada `000000000000`. Proporciona endpoints compatibles para S3, SSM Parameter Store, CloudWatch Logs e IAM.
2. **PostgreSQL 18 Local**: Corre en un contenedor Docker (`espacigo_aws_local`) sin intermediarios ni limitaciones de Cloud SQL.
3. **Mailpit Local**: Emula el servicio de correo Amazon SES en los puertos `:1025` (SMTP) y `:8025` (interfaz web de inspección de correos).

---

## 2. Mapa Completo de Accesos y Secretos Locales

Todos los servicios y secretos necesarios para levantar, probar y auditar la plataforma se encuentran disponibles localmente:

| Servicio / Componente | Dirección / URL Local | Protocolo / Puerto | Credenciales / Ubicación |
|---|---|---|---|
| **Frontend Web (Next.js)** | [http://127.0.0.1:3000](http://127.0.0.1:3000) | HTTP :3000 | No requiere autenticación previa para navegar. |
| **Backend API (Go)** | [http://127.0.0.1:8080](http://127.0.0.1:8080) | HTTP :8080 | Expone `/api/v1/...` (consumido vía BFF Next.js). |
| **AWS Emulator** | `https://127.0.0.1:9297` | HTTPS :9297 | `Account: 000000000000`, CA TLS: `~/.aws/emulator/9297/ca.pem`, Env: `~/.aws/emulator/9297.env`. |
| **PostgreSQL 18** | `127.0.0.1:5432` / `172.24.0.3:5432` | TCP :5432 | DB: `espacigo_aws_local`<br/>Admin: `espacigo_admin`<br/>App Login: `espacigo_api_login`<br/>Runtime: `espacigo_runtime` |
| **Mailpit (SES / Emails)** | [http://127.0.0.1:8025](http://127.0.0.1:8025) (Web UI)<br/>`127.0.0.1:1025` (SMTP) | HTTP :8025<br/>SMTP :1025 | Bandeja de entrada web para inspeccionar correos de verificación y arriendos. |

### Ubicación de Secretos y Certificados (No se comitean)
- **Secretos de Base de Datos**: `.local/secrets/`
  - `.local/secrets/db_admin_password`: Clave de `espacigo_admin`.
  - `.local/secrets/runtime_password`: Clave de `espacigo_api_login` / `espacigo_runtime`.
- **Configuración de AWS Emulator**:
  - `~/.aws/emulator/9297.env`: Carga automáticamente las variables seguras de entorno local.
  - `~/.aws/emulator/9297/ca.pem`: Certificado CA local para llamadas HTTPS con `curl` o AWS CLI.

---

## 3. Comparativa: Plan de Arquitectura AWS de Producción vs. Entorno Local

```mermaid
flowchart TB
    subgraph AWS_Cloud["Arquitectura AWS en la Nube (Plan de Producción - us-east-2 / Chile)"]
        direction TB
        CF["CloudFront CDN"] --> ALB["Application Load Balancer (ALB)"]
        ALB --> ECS_Web["ECS Fargate: Next.js (Web Frontend)"]
        ALB --> ECS_API["ECS Fargate: Go API Backend"]
        
        ECS_API --> RDS[("Amazon RDS PostgreSQL 18<br/>Multi-AZ db.t4g.micro")]
        ECS_API --> S3_Prod[("Amazon S3 (5 Buckets)<br/>SSE-KMS & Versioning")]
        ECS_API --> SSM_Prod["AWS Systems Manager<br/>Parameter Store"]
        ECS_API --> SES_Prod["Amazon SES (Transaccional)"]
        ECS_API --> CW_Prod["Amazon CloudWatch Logs"]
        
        IAM_Prod["IAM Roles de Mínimo Privilegio<br/>(Task Role & Execution Role)"] -.-> ECS_API
    end

    subgraph AWS_Local["Implementación Local en espacigo-aws (100% en 127.0.0.1)"]
        direction TB
        Host["Host Local (Desarrollador / Agente)"] --> Next_Local["Next.js 16 Dev Server<br/>http://127.0.0.1:3000"]
        Next_Local -->|BFF Loopback HTTP| Go_Local["Go API Backend (cmd/api)<br/>http://127.0.0.1:8080"]
        
        Go_Local --> DB_Local[("Docker PostgreSQL 18<br/>127.0.0.1:5432 (espacigo_aws_local)")]
        Go_Local -->|AWS SDK v2 (PathStyle)| S3_Local[("aws-emulator S3<br/>https://127.0.0.1:9297")]
        Go_Local -->|AWS SDK v2| SSM_Local["aws-emulator SSM (/espacigo/local/*)"]
        Go_Local -->|SMTP :1025| Mailpit["Mailpit (Emulador SES)<br/>Web UI: http://127.0.0.1:8025"]
        Go_Local --> CW_Local["aws-emulator CloudWatch Logs"]
        
        IAM_Local["IAM Emulado (Acct: 000000000000)<br/>~/.aws/emulator/9297.env"] -.-> Go_Local
    end
```

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
