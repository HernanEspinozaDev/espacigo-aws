# 08 — Hoja de Ruta de Migración en Dos Fases: (1) GCP $\rightarrow$ AWS `us-east-2` y (2) AWS `us-east-2` $\rightarrow$ AWS Chile

**Fecha de actualización:** 15 de noviembre de 2026  

---

## 1. Fase 1: Migración desde GCP (`southamerica-west1`) hacia AWS `us-east-2` (Ohio)

### 1.1 Pre-Chequeo Obligatorio en tu Cuenta AWS Free Plan (Sin Costo)
Antes de aprovisionar cualquier recurso que consuma créditos:
1. Abrir **AWS CloudShell** en la consola de AWS (región `us-east-2` Ohio) y ejecutar el script de solo lectura [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh).
2. Confirmar en la salida del script:
   - Que `db.t4g.micro` con almacenamiento `gp3` y motor `postgres` `18.x` aparece listado en `describe-orderable-db-instance-options`.
   - Que `ecs`, `ecr` y `ssm` responden sin bloqueo de cuenta Free Plan.
3. Configurar en la consola el presupuesto de **AWS Budgets ($28.00 USD/mes)** para activar las alertas preventivas y recibir el primer bono de **+$20.00 USD en créditos** del panel *Explore AWS*.

### 1.2 Preparación en el Repositorio `espaciGo` (Sin tocar Producción)
1. **Sincronizar el checkout con `main` (`7b01a47`):** Asegurar que están presentes las 84 migraciones (`V000001`–`V000084`) y el flujo `FLUJO-01..07`.
2. **Implementar `internal/adapters/s3storage` (`BLOB_STORE_BACKEND=s3`):** Para almacenar imágenes WebP (`media`), evidencias, el manifiesto de supresión (`completed-suppressions-v1.json`) y los documentos cifrados con `AES-256-GCM` (`documentverification`) en el bucket S3 de `us-east-2`.
3. **Empaquetar `poppler-utils` y `/etc/ssl/certs/rds-global-bundle.pem` en la imagen de `espacigo-api`:** Manteniendo el adaptador de correo actual hacia **Zoho SMTP (`smtp.zoho.com:587`, `contacto@hotusoft.com`)**.

### 1.3 Procedimiento de Migración de PostgreSQL 18.6 (Sin AWS DMS)
Dado que la base de datos actual en Cloud SQL pesa **< 50 MB**, usar AWS DMS sería innecesariamente complejo y propenso a perder roles globales (como ocurrió en `GCP-04`). El procedimiento limpio en 5 pasos es:

1. **Inicialización Declarativa de Roles en RDS (`bootstrap_rds_roles.sql`):**
   - Conectado a la nueva instancia RDS `db.t4g.micro` en `us-east-2` como usuario maestro, se crean los roles requeridos (`espacigo_schema_owner`, `espacigo_document_verification_fn`, `espacigo_runtime`, `espacigo_privacy_cleanup_worker`, `espacigo_gadm_bootstrap`, `espacigo_gcp06_test_access`, `espacigo_migrator_login`, `espacigo_api_login`).
   - Se asigna el schema `public` a `espacigo_schema_owner` y **se revoca la membresía automática del usuario maestro** (`REVOKE espacigo_schema_owner, espacigo_document_verification_fn, espacigo_runtime FROM postgres;`) para cumplir los guards estrictos de `V000081`.
2. **Ensayo en Seco (Dry-Run) antes de la Ventana de Corte:**
   - Exportar un dump de prueba desde Cloud SQL y restaurarlo en RDS `us-east-2` verificando que `cmd/dbmigrate -plan-only` reporta **84 migraciones aplicadas y 0 pendientes**.
3. **Ventana de Corte Controlada (~15 a 25 minutos):**
   - Poner `espacigo-test-api` en modo mantenimiento / drenar workers en GCP.
   - Ejecutar `pg_dump --format=custom --no-owner --no-acl --exclude-schema=topology --exclude-schema=tiger*` desde Cloud SQL.
   - Sincronizar el delta final de objetos desde GCS (`espacigo-testprod-media-447373903153`) hacia el bucket S3 en `us-east-2` verificando hashes SHA-256.
   - Ejecutar `pg_restore --no-owner --role=espacigo_schema_owner` hacia RDS PostgreSQL 18.6 en `us-east-2`.
   - Ejecutar `/api local-privacy-replay-apply` (`PT-16` Ley 21.719) y `cmd/dbmigrate -schema-evidence-only`.
4. **Conmutación de DNS y Activación de Cloudflare Zero Trust Access:**
   - En Cloudflare, cambiar el origen de `espacigo.testingpage.store` y `espacigo-demo.testingpage.store` hacia la tarea ECS Fargate en `us-east-2`, activando el **Proxy Naranja (`proxied = true`)** y la política de **Cloudflare Zero Trust Access** sobre `espacigo-demo.testingpage.store`.
   - Como los secretos `BFF_SIGNING_SECRET` y `AUTH_JWT_SECRET` y la tabla `public.sesion` se conservan idénticos, las sesiones y cuentas continúan operando sin fricción.
5. **Ejecución del Simulacro de Recuperación (`PT-11` y `PT-16`) y Retirada de GCP:**
   - Ejecutar el simulacro cronometrado de restauración PITR en `us-east-2` para documentar el RPO y RTO reales en la tesis.
   - Tras **7 días de estabilidad en AWS `us-east-2`**, destruir los recursos de GCP `espacigo-testprod` antes de que venza el Free Trial de GCP.

---

## 2. Fase 2 (Futura): Migración Regional desde `us-east-2` (Ohio) hacia `AWS South America (Chile)`

Una vez que finalice el período del AWS Free Plan (o se decida pasar al Paid Plan para el lanzamiento comercial) y **AWS South America (Chile — `sa-west-1`)** tenga disponibilidad general (GA) verificada para RDS PostgreSQL 18 y ECS Fargate:

1. **Verificación de Servicios y Precios en AWS Chile:**
   - Ejecutar `REGION="sa-west-1" bash verify-aws-free-plan-us-east-2.sh` para confirmar soporte de `postgres 18`, `postgis`, `db.t4g.micro`/`small` y precios locales en Santiago.
2. **Despliegue de Infraestructura Idéntica por IaC (`tofu apply`):**
   - Ejecutar el mismo módulo de OpenTofu/Terraform cambiando únicamente `aws_region = "sa-west-1"` (y `rds_multi_az = true` si corresponde al lanzamiento comercial).
3. **Traslado de Datos AWS-a-AWS (`us-east-2` $\rightarrow$ `AWS Chile`):**
   - Copiar el snapshot cifrado de RDS entre regiones (`aws rds copy-db-snapshot`) o ejecutar `pg_dump -Fc` / `pg_restore` durante una ventana de 15 minutos + `aws s3 sync` entre el bucket de Ohio y el bucket de Santiago.
4. **Cambio de IP de Origen en Cloudflare (< 5 segundos):**
   - Actualizar la IP de origen en Cloudflare hacia la tarea en AWS Chile. El tiempo de respuesta dinámico desde Chile caerá de ~180–250 ms a **~15–40 ms**, con el 100% de los datos residiendo físicamente en Santiago de Chile.
