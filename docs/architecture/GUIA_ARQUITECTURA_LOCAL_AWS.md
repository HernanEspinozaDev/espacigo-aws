# Guía Maestra de Arquitectura: Plan AWS vs. Entorno Local Emulado

**Repositorio:** `espacigo-aws`  
**Objetivo:** Manual de referencia para ingenieros y agentes de IA. Explica la arquitectura AWS de producción, la implementación local con `aws-emulator` y por qué no se requieren credenciales reales de IAM.

---

## 1. ¿Por qué `espacigo-aws` corre en un emulador local?

El proyecto original (`espaciGo`) estuvo históricamente atado a Google Cloud Platform (GCP). Esa arquitectura generaba fricción insostenible:
- **Cuellos de botella en Cloud SQL**: Toda migración de base de datos debía ejecutarse a través de Cloud Build (`release_main.py`), obligando a sincronizar ledgers remotos, emitir preflights y esperar rollouts en la nube antes de poder probar el código.
- **Costos y bloqueos en integraciones**: Probar servicios como verificación de identidad biométrica o notificaciones por correo requería pagar por uso en proveedores de nube y configurar túneles hacia internet.
- **Lentitud en el ciclo de feedback**: El desarrollador o agente no podía probar cambios de inmediato en su máquina.

### La Solución en `espacigo-aws`:
Se diseñó un plan de arquitectura nativo para AWS y se adaptó para correr **100% de manera local**:
1. **Emulador AWS Oficial (`aws-emulator` v1.0.385)**: Corre en `https://127.0.0.1:9297` bajo la cuenta aislada `000000000000`. Proporciona endpoints compatibles para S3, SSM Parameter Store, CloudWatch Logs e IAM.
2. **PostgreSQL 18 Local**: Corre en un contenedor Docker (`espacigo_aws_local`) sin intermediarios ni limitaciones de Cloud SQL.
3. **Mailpit Local**: Emula el servicio de correo Amazon SES en los puertos `:1025` (SMTP) y `:8025` (interfaz web de inspección de correos).

> [!WARNING]
> **REGLA DE ORO:** Este repositorio está diseñado exclusivamente para desarrollo y pruebas locales. **Ningún agente ni desarrollador debe solicitar credenciales de AWS reales** ni configurar perfiles IAM hacia cuentas en la nube (como la cuenta `805068224315`). Todos los comandos y SDKs deben apuntar al emulador local.

---

## 2. Diagrama de Arquitectura: Nube AWS (Producción) vs. Entorno Local

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

---

## 3. Matriz Comparativa: Componente por Componente

| Dimensión / Componente | Plan de Arquitectura AWS (Nube / Prod) | Entorno Local en `espacigo-aws` | Nivel de Paridad |
|---|---|---|---|
| **Cómputo Backend** | AWS ECS Fargate en subnets privadas de VPC tras ALB. | Binario Go nativo (`cmd/api`) en `http://127.0.0.1:8080`. | **95%** (Mismo código Go, mismos handlers). |
| **Cómputo Frontend** | Next.js en Vercel o contenedor ECS con CloudFront. | Next.js en modo desarrollo en `http://127.0.0.1:3000`. | **98%** (Mismo código React y rutas BFF). |
| **Base de Datos** | Amazon RDS PostgreSQL 18 (`db.t4g.micro`) provisionado con Terraform. | Contenedor Docker PostgreSQL 18 (`espacigo_localbackend_20261024-database-1`). | **100%** (Misma versión del motor y 97 migraciones). |
| **Almacenamiento (S3)** | Amazon S3 con cifrado SSE-KMS, Versioning y Bloqueo Público. | S3 en `aws-emulator:9297` (`espacigo-media-local`, `espacigo-evidence-local`, etc.). | **95%** (Mismo AWS SDK v2, llamadas idénticas). |
| **Configuración (SSM)** | AWS Systems Manager Parameter Store (`/espacigo/prod/*`). | SSM Parameter Store en `aws-emulator` (`/espacigo/local/*`). | **100%** (Mismos parámetros creados con Terraform). |
| **Correos (SES)** | Amazon Simple Email Service (SES) con dominio verificado. | Servidor SMTP Mailpit local en `:1025` con UI web en `:8025`. | **95%** (Inspección inmediata de HTML sin spam). |
| **Logs & Auditoría** | Amazon CloudWatch Log Groups. | CloudWatch Logs en `aws-emulator` + consola stdout. | **90%** (Registros disponibles localmente). |
| **Seguridad e IAM** | IAM Task Role y Execution Role con políticas estrictas. | IAM emulado (`arn:aws:iam::000000000000:role/...`). | **90%** (Valida permisos sin requerir cuenta real). |
| **Costo Operativo** | Estimado en ~$25 - $35 USD/mes (Free Tier elegible). | **$0 USD** (100% gratuito e ilimitado). | **Óptimo**. |

---

## 4. Mapa de Accesos y Secretos Locales

Todos los accesos requeridos para operar el proyecto están preconfigurados:

### 1. URLs de Servicios
- **Aplicación Web**: [http://127.0.0.1:3000](http://127.0.0.1:3000)
- **Servidor API Go**: [http://127.0.0.1:8080](http://127.0.0.1:8080)
- **Emulador AWS**: `https://127.0.0.1:9297`
- **Bandeja de Correos (Mailpit)**: [http://127.0.0.1:8025](http://127.0.0.1:8025)
- **Base de Datos PostgreSQL**: `127.0.0.1:5432` / `172.24.0.3:5432`

### 2. Secretos y Archivos Sensibles (Ignorados por Git en `.local/`)
- `.local/secrets/db_admin_password`: Contraseña del superusuario `espacigo_admin`.
- `.local/secrets/runtime_password`: Contraseña de aplicación para `espacigo_api_login` y `espacigo_runtime`.
- `~/.aws/emulator/9297.env`: Variables de entorno de conexión segura para `aws-emulator`.
- `~/.aws/emulator/9297/ca.pem`: Certificado CA local para conexiones TLS del emulador.

### 3. Comandos de Gestión Rápida
Todo el ciclo de vida se opera desde [`scripts/local-dev.sh`](../../scripts/local-dev.sh):
```bash
./scripts/local-dev.sh status      # Diagnóstico completo de salud
./scripts/local-dev.sh bootstrap   # Aprovisionamiento Terraform y permisos DB
./scripts/local-dev.sh api         # Inicia el backend Go (:8080)
./scripts/local-dev.sh web         # Inicia el frontend Next.js (:3000)
./scripts/local-dev.sh test        # Corre todas las suites de prueba
```
