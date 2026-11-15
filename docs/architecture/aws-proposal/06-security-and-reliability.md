# 06 — Seguridad, Cumplimiento (Ley 21.719) y Análisis Fundamentado de Recuperación (RPO / RTO en Single-AZ vs. Multi-AZ)

**Fecha de actualización:** 15 de noviembre de 2026  

---

## 1. Análisis Fundamentado de RPO y RTO en Amazon RDS PostgreSQL Single-AZ (Sin Supuestos Optimistas)

En respuesta a la directriz expresa del propietario (*"No asumir que Single-AZ garantiza RPO de cinco minutos ni RTO de 15–20 minutos. Solicito estimaciones fundamentadas y pruebas de recuperación"*), se desglosa la mecánica interna de **Amazon RDS for PostgreSQL 18 en modo Single-AZ** en `us-east-2`:

### 1.1 ¿Cómo funciona realmente el RPO (Recovery Point Objective) en RDS Single-AZ?
En RDS PostgreSQL con backups automáticos y PITR habilitados:
1. Cada transacción confirmada (`COMMIT`) se escribe síncronamente en el archivo **WAL (Write-Ahead Log)** dentro del volumen **Amazon EBS (`gp3`)** local de esa única Zona de Disponibilidad (ej. `us-east-2a`).
2. Un proceso asíncrono (`pg_archiver` gestionado por RDS) copia a **Amazon S3** (que sí es multi-AZ en `us-east-2`) cada segmento WAL de **64 MB** cuando se llena **o** cuando se cumple el parámetro `archive_timeout` (fijado por defecto en RDS en **300 segundos = 5 minutos**).
3. **Por qué Single-AZ NO garantiza RPO ≤ 5 minutos en todos los escenarios de fallo:**

| Tipo de Incidente en Single-AZ (`us-east-2`) | ¿Qué ocurre con los datos escritos en los últimos minutos? | RPO Real Fundamentado | Nivel de Garantía |
| :--- | :--- | :---: | :--- |
| **Escenario 1: Error lógico / humano o bug de aplicación** *(ej. `DELETE` accidental o corrupción de datos con la instancia RDS viva)* | La instancia RDS sigue viva y archiva normalmente el segmento WAL activo hacia S3 (o se fuerza con `SELECT pg_switch_wal()`). | **0 a 5 minutos** | **Alto** *(Verificable consultando `LatestRestorableTime` en la API de RDS).* |
| **Escenario 2: Caída del proceso PostgreSQL o fallo de la VM EC2 subyacente (con el volumen EBS `gp3` intacto)** | RDS reinicia el motor o monta el mismo volumen EBS `gp3` en otro host EC2 de la misma AZ y ejecuta el `crash recovery` leyendo el WAL directamente desde el disco EBS. | **0 segundos (Sin pérdida de transacciones confirmadas)** | **Alto** *(Mientras el volumen EBS de la AZ no sufra daño).* |
| **Escenario 3: Destrucción física súbita del volumen EBS o corte total de la AZ antes de que `archiver` suba a S3** | Los commits que estaban en el segmento WAL abierto dentro del volumen EBS local y que **aún no habían sido copiados asíncronamente a S3** no pueden leerse hasta que la AZ se recupere. Si se restaura inmediatamente en otra AZ vía PITR desde S3, se restaura hasta `LatestRestorableTime`. | **5 a 15 minutos**<br>*(Puede llegar a **15–30 min** si un pico masivo de escrituras retrasó la cola del archiver hacia S3)* | **Sin garantía de RPO cero.** Para garantizar RPO = 0 ante pérdida súbita de una AZ se requiere **RDS Multi-AZ** (replicación física síncrona a nivel de bloque en una segunda AZ). |

---

### 1.2 ¿Cómo funciona realmente el RTO (Recovery Time Objective) en RDS Single-AZ?
A diferencia de **RDS Multi-AZ** (donde una instancia standby pre-aprovisionada en `us-east-2b` toma el control en **60 a 120 segundos** actualizando el CNAME DNS), en **Single-AZ no existe ninguna instancia standby esperando**. Por tanto, el tiempo de recuperación depende del tipo de incidente:

#### Desglose Cronometrado del RTO en una Restauración PITR Completa (Escenario Crítico)

| Fase del Procedimiento de Recuperación en `us-east-2` | Duración Mínima (Automatizada) | Duración Conservadora (Con Intervención Humana) | Explicación Técnica de por qué Toma este Tiempo |
| :--- | :---: | :---: | :--- |
| **1. Detección, Alerta y Decisión del Operador** | 3 min | 15 min | La alarma de CloudWatch detecta el fallo (2–3 evaluaciones de 1 min), envía correo vía SNS y el operador confirma ejecutar el restore. |
| **2. Creación de la Nueva Instancia RDS y Restauración del Snapshot Base (`restore-db-instance-to-point-in-time`)** | **12 min** | **25 min** | **Cuello de botella principal de AWS RDS:** Aunque la base de EspaciGo pese solo 50 MB en un volumen de 20 GB, el plano de control de RDS tarda entre **12 y 25 minutos** en aprovisionar la nueva VM `db.t4g.micro`, adjuntar el volumen EBS, inicializar el almacenamiento desde S3 y levantar el motor PostgreSQL 18. |
| **3. Descarga y Replay de Segmentos WAL desde S3 hasta el Timestamp Objetivo** | 2 min | 8 min | RDS descarga desde S3 todos los archivos WAL generados entre la hora del último snapshot diario (06:00 UTC) y el minuto del incidente, y los aplica secuencialmente. |
| **4. Ejecución de Replay de Supresión Ley 21.719 (`PT-16`) y Verificación de Esquema (`cmd/dbmigrate -plan-only`)** | 2 min | 5 min | Paso obligatorio en EspaciGo: lanzar tarea efímera en ECS contra la nueva instancia para ejecutar `/api local-privacy-replay-apply` (no revivir datos eliminados) y validar integridad de las 84 migraciones. |
| **5. Conmutación del Endpoint (`DATABASE_HOST` o `modify-db-instance` rename) y Reinicio de Tarea ECS** | 2 min | 5 min | Actualizar el parámetro `/espacigo/prod/DATABASE_HOST` en SSM Parameter Store y forzar el redespliegue de la tarea ECS Fargate (`0.25 vCPU`). |
| **RTO TOTAL FUNDAMENTADO (PITR Single-AZ)** | **~21 minutos** | **~58 minutos** | **Conclusión honesta:** Un restore PITR en Single-AZ **NO tarda 15 minutos si requiere intervención humana**; toma entre **21 y 58 minutos**. Cumple al límite el requisito `RNF-010` (RTO ≤ 60 min) **únicamente si el runbook está automatizado en un script/workflow listo para disparar**. |

---

### 1.3 Protocolo de Pruebas de Recuperación (`PT-11` y `PT-16`) para Ejecutar en `us-east-2`

Antes de dar por cerrada la migración en `us-east-2` (y para documentar evidencia empírica real en el **Capítulo XII y Anexo F** de la tesis, donde `PT-11` figura actualmente pendiente), se ejecutará el siguiente ensayo cronometrado:

1. **Medición empírica del RPO (`lag` de `LatestRestorableTime`):**
   - Insertar un registro de prueba con timestamp exacto `T_commit` en la base en `us-east-2`.
   - Consultar cada 30 segundos `aws rds describe-db-instances --db-instance-identifier espacigo-db --query "DBInstances[0].LatestRestorableTime"` hasta que `LatestRestorableTime >= T_commit`.
   - Registrar `RPO_medido = T_disponible_en_S3 - T_commit` (esperado: 3 a 6 minutos).
2. **Medición empírica del RTO (Simulacro completo `restore_drill.sh`):**
   - Disparar `aws rds restore-db-instance-to-point-in-time` hacia `espacigo-db-drill` en la **segunda zona de disponibilidad (`us-east-2b`)** y medir con cronómetro:
     - `T_rds_available`: Tiempo hasta que la instancia entra en estado `available`.
     - `T_schema_and_privacy_verified`: Tiempo de ejecución de `cmd/dbmigrate -schema-evidence-only` + `/api local-privacy-replay-apply`.
   - Comprobar que los permisos de `espacigo_runtime`, `espacigo_schema_owner` y `espacigo_privacy_cleanup_worker` funcionan **sin errores** (resolviendo definitivamente el fallo histórico del ensayo `GCP03`).
   - Destruir `espacigo-db-drill` al finalizar (costo total del simulacro de 1 hora en `db.t4g.micro`: **$0.02 USD**).

---

### 1.4 Evaluación de Cuándo Activar Multi-AZ (Antes del Lanzamiento Comercial)

| Criterio | Mantener Single-AZ (`db.t4g.micro` = $13.98 USD/mes) | Activar Multi-AZ (`db.t4g.micro` = $27.96 USD/mes o `db.t4g.small` = $51.32 USD/mes) |
| :--- | :--- | :--- |
| **Cuándo corresponde** | Durante los 6 meses del AWS Free Plan, defensa de título, pruebas de usuarios piloto sin cobros reales o < 50 arriendos/mes. | **Hito obligatorio antes de habilitar cobros reales con Mercado Pago Split 1:1 y SimpleFirma en producción comercial.** |
| **Cómo se activa** | `multi_az = false` en Terraform. | Cambiar `multi_az = true` en Terraform (`aws rds modify-db-instance --multi-az --apply-immediately`). AWS crea el standby en segundo plano sin apagar la base primaria. |
| **Efecto en RPO / RTO / SLA** | RPO: 5–15 min / RTO: 21–58 min / Sin SLA contractual de AWS. | **RPO = 0 segundos** (replicación síncrona en 2 AZs) / **RTO = 60–120 segundos** (failover automático de DNS) / **SLA contractual AWS = 99,95%** (cumple `RNF-009` ≥ 99,9%). |

---

## 2. Controles de Seguridad, IAM, Cifrado y Observabilidad en `us-east-2`

1. **Principio de Mínimo Privilegio (IAM Roles separados):**
   - `espacigo-github-oidc-deploy-role`: Autenticación OIDC desde GitHub Actions sin llaves permanentes; no tiene permiso para leer secretos de aplicación ni objetos de usuarios en S3.
   - `espacigo-ecs-execution-role`: Solo puede desencriptar los parámetros de `SSM Parameter Store` en el arranque del contenedor.
   - `espacigo-api-task-role`: Acceso restringido a los prefijos autorizados del bucket S3 en `us-east-2`.
   - `espacigo-migrator-task-role`: Rol exclusivo de la tarea efímera `cmd/dbmigrate` con acceso únicamente a la credencial de `espacigo_migrator_login`.
2. **Seguridad de Red y Aislamiento de PostgreSQL 18:**
   - RDS en subred privada aislada (`publicly_accessible = false`, sin ruta a Internet Gateway).
   - Security Group `sg-espacigo-db` que solo acepta TCP `5432` desde `sg-espacigo-app`.
   - `rds.force_ssl = 1` en RDS y `DATABASE_SSLMODE=verify-full` con `/etc/ssl/certs/rds-global-bundle.pem` en Go.
   - El contenedor `espacigo-api` escucha solo en `127.0.0.1:8080` dentro de la tarea Fargate; el único puerto abierto hacia el exterior de la tarea es el `4000` (Next.js BFF), restringido en `sg-espacigo-app` a los rangos IP oficiales de Cloudflare.
3. **Cifrado en Reposo y Tránsito:**
   - **En tránsito:** TLS 1.3 (Usuario $\leftrightarrow$ Cloudflare Santiago), TLS 1.2/1.3 Full Strict (Cloudflare $\leftrightarrow$ Contenedor en `us-east-2`), TLS `verify-full` (Go $\leftrightarrow$ RDS PostgreSQL 18) y STARTTLS `:587` (Go $\leftrightarrow$ `smtp.zoho.com`).
   - **En reposo:** Cifrado AES-256 en el volumen EBS `gp3` y snapshots de RDS (`storage_encrypted = true`), cifrado `SSE-S3` en todos los buckets S3 + **cifrado autenticado `AES-256-GCM` a nivel de aplicación Go** para documentos KYC (`ESPACIGODOC1`) y contratos PDF, e inmutabilidad WORM mediante **S3 Object Lock** para logs de auditoría (`RNF-017` y `RNF-043`).
4. **Monitoreo y Observabilidad en CloudWatch (`$0.00/mes` en Always Free Tier):**
   - Retención de logs configurada a `14 días` en CloudWatch Logs (`/ecs/espacigo-app`, `/ecs/espacigo-dbmigrate`).
   - **8 Alarmas de CloudWatch conectadas a SNS (correo del equipo):**
     1. `RDS-CPUUtilization > 70%` (5 min)
     2. `RDS-CPUCreditBalance < 25` (evita sobreconsumo en `db.t4g.micro`)
     3. `RDS-FreeableMemory < 150 MB`
     4. `RDS-FreeStorageSpace < 3 GB`
     5. `RDS-DatabaseConnections > 35`
     6. `ECS-CPUUtilization > 80%`
     7. `ECS-MemoryUtilization > 85%`
     8. `ECS-RunningTaskCount < 1` (alerta inmediata si la tarea única se detiene)
