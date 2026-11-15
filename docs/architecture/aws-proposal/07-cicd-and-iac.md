# 07 — Estrategia de CI/CD, Infraestructura como Código (IaC Multi-Región `us-east-2` $\rightarrow$ AWS Chile) y Migraciones

**Fecha de actualización:** 15 de noviembre de 2026  

---

## 1. Infraestructura como Código (OpenTofu / Terraform) Preparada para `us-east-2` $\rightarrow$ AWS Chile

### 1.1 Diseño Modular Agnóstico de Región
Para cumplir la directriz del propietario (*"No diseñar la infraestructura suponiendo que utilizaremos `us-east-1` o `sa-east-1`. Diseñar mediante IaC para facilitar una futura migración regional"* hacia **AWS South America — Chile**), todos los módulos de OpenTofu/Terraform (`>= 1.9` / Terraform `>= 1.10`) se parametrizan sin ningún valor regional fijo:

```hcl
# infra/terraform/environments/aws-main/variables.tf
variable "aws_region" {
  description = "Región AWS de despliegue (Inicial: us-east-2 Ohio; Futura: región AWS Chile)"
  type        = string
  default     = "us-east-2"
}

variable "monthly_budget_limit_usd" {
  description = "Límite de alerta en AWS Budgets (Meta: < 30 USD/mes)"
  type        = string
  default     = "28.0"
}

variable "rds_instance_class" {
  description = "Clase de instancia RDS PostgreSQL 18"
  type        = string
  default     = "db.t4g.micro"
}

variable "rds_multi_az" {
  description = "Activar Multi-AZ antes del lanzamiento comercial"
  type        = bool
  default     = false
}
```

### 1.2 Descubrimiento Dinámico en los Módulos HCL (Cero Hardcoding)
1. **Zonas de Disponibilidad (`data.aws_availability_zones`):** Se seleccionan automáticamente las 2 primeras AZs disponibles en `var.aws_region` (`us-east-2a` y `us-east-2b` hoy; `sa-west-1a` y `sa-west-1b` al migrar a Chile).
2. **Gateway VPC Endpoint de S3:** Se construye dinámicamente como `service_name = "com.amazonaws.${var.aws_region}.s3"`.
3. **Versión de PostgreSQL 18 (`data.aws_rds_engine_version`):** Consulta en tiempo real la versión menor vigente de `postgres` `18` soportada en la región activa.
4. **State Locking Nativo en S3 (`use_lockfile = true`):** El backend remoto de Terraform utiliza un bucket S3 con `use_lockfile = true` (escritura condicional `.tflock` en S3), sin requerir DynamoDB.

---

## 2. Pipeline CI/CD en GitHub Actions con OIDC hacia `us-east-2`

### 2.1 Por qué Unificamos CI/CD en GitHub Actions ($0.00 USD/mes)
- En GCP se ejecutaban los mismos tests dos veces: primero en GitHub Actions (`application-quality.yml`, `frontend-quality.yml`, `database-quality.yml`) y luego dentro de Cloud Build (`quality_gate.py`).
- En AWS consolidamos todo en **GitHub Actions** autenticándose mediante **OIDC (`aws-actions/configure-aws-credentials@v4`)** contra el rol IAM `espacigo-github-oidc-deploy-role` en `us-east-2` (sin llaves `AWS_SECRET_ACCESS_KEY` de larga duración).
- El parámetro `AWS_REGION` vive como una variable de repositorio en GitHub (`vars.AWS_REGION = "us-east-2"`), de modo que el día que se migre a AWS Chile basta con cambiar esa variable en GitHub.

### 2.2 Flujo Completo de Despliegue y Migraciones Versionadas de PostgreSQL 18
1. **Quality & Security Gate (En cada PR y Push a `main`):**
   - Levanta el contenedor de prueba `postgis/postgis:18-3.6` en el runner de GitHub Actions.
   - Ejecuta `gofmt`, `go vet`, `govulncheck`, `go test ./...`, aplica las 84+ migraciones en limpio, valida `db/release-policy.json` (solo estrategia `expand`) y ejecuta `pnpm lint`, `typecheck`, `test` y `build` en `frontend/`.
2. **Construcción Multi-Stage ARM64 y Push a Amazon ECR (`us-east-2`):**
   - Compila `espacigo-api` (con `poppler-utils` y `rds-global-bundle.pem`), `espacigo-web` (`Next.js 16 standalone`) y `espacigo-dbmigrate` para `linux/arm64` (Graviton2).
   - Publica en ECR `us-east-2`, verifica el escaneo automático de vulnerabilidades (`Scan on Push`) y extrae el digest inmutable `@sha256:...`.
3. **Ejecución Segura de Migraciones SQL (Sin Bastión SSH ni Comandos Locales):**
   - Si el clasificador detecta archivos `.sql` nuevos respecto al watermark en S3:
     1. Lanza una tarea efímera `aws ecs run-task` en `us-east-2` con `cmd/dbmigrate -plan-only` (modo `READ ONLY`, verificando que la secuencia `V000001..V000084+` y los checksums SHA-256 en `public.schema_migrations` estén intactos).
     2. Verifica que exista un backup automático reciente y dispara un snapshot manual en RDS (`aws rds create-db-snapshot --db-snapshot-identifier espacigo-pre-release-<sha>`), esperando a que su estado sea `available`.
     3. Lanza `aws ecs run-task` con `cmd/dbmigrate -release` (adquiere `pg_advisory_lock(0x45535047, 0x4d494752)` bajo `espacigo_migrator_login` / `SET ROLE espacigo_schema_owner` y aplica cada archivo dentro de una transacción).
     4. Genera y guarda el recibo de evidencia `-schema-evidence-only` en S3.
4. **Despliegue Rolling en ECS Fargate con Circuit Breaker y Rollback Automático:**
   - Registra la nueva `Task Definition` apuntando a los digests `@sha256:...` de `espacigo-web` y `espacigo-api` y ejecuta `aws ecs update-service --deployment-configuration "deploymentCircuitBreaker={enable=true,rollback=true}"`.
   - ECS levanta la nueva tarea en paralelo, verifica `/health/ready` (`127.0.0.1:8080`) y `/healthz` (`:4000`), y solo entonces drena la tarea anterior.
   - Si el healthcheck falla, **ECS Circuit Breaker revierte automáticamente el servicio a la Task Definition anterior** (mientras que las migraciones SQL, al ser estrictamente `expand-only`, nunca se revierten de forma destructiva).

---

## 3. Detección Automática de Cambios No Autorizados (Drift Detection)

Un workflow programado en GitHub Actions (`drift-detection.yml`) se ejecuta cada lunes (y tras cada cambio de infraestructura) usando un rol OIDC de solo lectura para correr `tofu plan -detailed-exitcode` en `us-east-2` y Cloudflare, alertando si cualquier parámetro en consola diverge del repositorio.
