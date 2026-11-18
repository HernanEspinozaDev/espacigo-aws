# AGENTS.md — Entornos y ramas de EspaciGo

## Flujo obligatorio

| Rama | Entorno | Operación |
| --- | --- | --- |
| `desarrollo-local` | AWS Emulator, PostgreSQL y Mailpit locales | Cambios de código e infraestructura, fixtures y pruebas de desarrollo. |
| `main` | Producción en AWS real | Recibe commits probados de `desarrollo-local` mediante promoción explícita del usuario. La entrega continua se prepara en AWS CodePipeline para el SHA integrado. |

1. Trabaja y prueba en `desarrollo-local` con Conventional Commits. Separa en commits los cambios de aplicación compartidos, los exclusivos del entorno local y los de configuración de AWS. Promueve a `main` sólo los cambios que el usuario autorice expresamente y que hayan pasado las comprobaciones correspondientes.
2. Ejecuta `scripts/local-dev.sh` y usa credenciales del emulador sólo para desarrollo local. Mantén `.local/`, dumps, tokens y llaves fuera de Git.
3. GitHub Actions ejecuta la CI. Para la entrega continua en AWS, prepara CodePipeline con una conexión de GitHub y roles de servicio específicos. Mantén desactivado el job de despliegue OIDC en el proyecto administrado mientras la SCP bloquee los proveedores IAM. Para operaciones humanas, usa AWS CLI v2 con `aws login` y un perfil de producción explícito en la Región seleccionada. Mantén las sesiones, los endpoints y los secretos separados de los del emulador.
4. Mantén Go en AWS SDK v2. Usa endpoint local y `UsePathStyle` para S3 emulado. Usa la resolución estándar de AWS para producción. Conserva la arquitectura AWS del repositorio.
5. Comprueba RDS de producción con [el procedimiento temporal](docs/operations/aws-production-deployment.md#comprobar-rds-con-recursos-temporales). Cada ejecución aprovisiona una Lambda privada, un endpoint de Secrets Manager y un rol acotado. Durante la revisión de migraciones en RDS, conserva abiertos el endpoint, la Lambda y sus permisos hasta que el usuario indique explícitamente retirarlos. Reutiliza el recibo activo para las siguientes comprobaciones. Fuera de esa ventana, verifica la retirada al terminar. Para las migraciones usa un único login operativo, `espacigo_migrator_login`, que asume `espacigo_schema_owner`. En local, `scripts/local-dev.sh migrate-plan` y `scripts/local-dev.sh migrate` usan ese mismo par con una credencial exclusiva de la base local.
6. Mantén las funciones de ensayo y los datos sintéticos en el entorno local. La demo de arriendo usa un pago de prueba y una bandera de arranque local.
7. Sigue el [plan de despliegue y CI/CD](docs/operations/aws-production-deployment.md) para preparar el destino de AWS, activar el pipeline y recuperar un despliegue.

### Alcance de una promoción a `main`

- En una entrega de aplicación, promueve el código compartido y las migraciones aprobadas. Los cambios de `scripts/local-dev.sh`, `scripts/iniciar-local.sh`, `infra/terraform/environments/local-emulator/`, la configuración del emulador, Mailpit y los datos de demostración se quedan en `desarrollo-local`.
- Crea y prueba las migraciones en `desarrollo-local`. Promueve cada SQL junto con su clasificación en `db/release-policy.json`. El despliegue en AWS ejecuta y verifica las migraciones aprobadas antes de actualizar la aplicación. La carga inicial de RDS contiene el esquema y catálogos de referencia.
- Desde V101, crea y modifica objetos ordinarios con `espacigo_migrator_login` en sesión y `espacigo_schema_owner` como rol efectivo y propietario. Promueve el mismo archivo SQL probado localmente. Para un objeto histórico con dueño distinto, revisa y prepara una transferencia puntual que preserve sus permisos antes de cambiarlo. Conserva los dueños específicos de extensiones, objetos gestionados por PostgreSQL y funciones con autoridad propia.
- La configuración de AWS en `main` se conserva en las entregas de aplicación. Los cambios a `infra/terraform/environments/aws-main/`, al pipeline o al script de despliegue requieren una promoción explícita de infraestructura. Incluye en esa revisión cualquier cambio a módulos compartidos de Terraform que afecte a producción. Guarda los secretos en AWS Secrets Manager o en el entorno local correspondiente, fuera de Git.
- Antes de publicar, parte de `origin/main` actualizado y revisa el diff completo del candidato. Usa fast-forward únicamente si todos los commits incluidos corresponden a la entrega aprobada. Si la rama mezcla cambios locales y compartidos, incorpora sólo los commits compartidos aprobados mediante `cherry-pick`. Si un commit mezcla ambos tipos de cambios, sepáralos antes de promoverlo. Valida el candidato resultante antes de publicar `main`.
- Con el repositorio privado actual puedes publicar directamente la promoción aprobada. Un PR desde `desarrollo-local` es opcional si su diff contiene sólo cambios autorizados. Si GitHub exige PR y hay cambios locales en el diff, detén el pase hasta preparar una rama de promoción limpia compatible con las comprobaciones de GitHub. Sigue trabajando en `desarrollo-local` después de la entrega.

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
| **Cómputo Backend** | AWS ECS Fargate en la subred privada principal, con acceso por loopback desde Next.js. | Binario Go nativo (`cmd/api`) corriendo en `http://127.0.0.1:8080`. |
| **Cómputo Frontend** | Next.js en la misma tarea Fargate, con NLB TLS de una zona en el despliegue inicial. ALB multizona queda para una etapa futura. | Next.js dev server en `http://127.0.0.1:3000` con BFF loopback. |
| **Base de Datos Relacional** | Amazon RDS PostgreSQL 18 inicialmente Single-AZ (`db.t4g.micro`). Multi-AZ queda para una etapa futura. | Contenedor Docker PostgreSQL 18.6 (`espacigo_localbackend_20261024-database-1`). |
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
