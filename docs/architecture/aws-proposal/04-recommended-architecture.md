# 04 — Arquitectura Recomendada en `us-east-2` (Ohio) y Diseño Preparado para AWS Chile

**Fecha de actualización:** 15 de noviembre de 2026  

---

## 1. Arquitectura Seleccionada para `us-east-2` (Ohio)

La arquitectura recomendada se adapta específicamente a la región inicial **`us-east-2` (US East — Ohio)** bajo el **AWS Free Plan**, manteniendo la meta de **menos de USD $30.00 mensuales** después de los créditos (**$25.11 a $26.41 USD/mes** verificados) y garantizando mediante **OpenTofu / Terraform** una migración futura limpia hacia **AWS South America (Chile — `sa-west-1`)** cuando pase a disponibilidad general y la cuenta cambie al Paid Plan.

### Decisiones Clave de la Arquitectura en `us-east-2`
1. **Borde en Santiago de Chile (Cloudflare) + Cómputo Co-Ubicado en `us-east-2` (ECS Fargate ARM64):**
   - El tráfico de usuarios públicos (`espacigo.testingpage.store`) y evaluadores (`espacigo-demo.testingpage.store`) ingresa por el PoP de **Cloudflare en Santiago de Chile (< 10 ms)**.
   - **Cloudflare Zero Trust Access (Plan Free, hasta 50 usuarios)** reemplaza a Google IAP en `espacigo-demo.testingpage.store`, permitiendo eliminar el segundo contenedor duplicado (`espacigo-test-private`).
   - En `us-east-2`, una única tarea de **Amazon ECS con AWS Fargate (`linux/arm64` Graviton2, `0.25 vCPU` y `0.5 GB` a `1.0 GB` RAM)** ejecuta en el mismo espacio de red (`awsvpc`) los dos contenedores:
     - `espacigo-web` (Next.js 16.4.3 standalone en `0.0.0.0:4000`).
     - `espacigo-api` (Go 1.27.1 + `poppler-utils` en `127.0.0.1:8080`, junto con sus 13 goroutines periódicas).
   - **Beneficio doble (Latencia + FinOps):** Todas las llamadas del BFF Next.js hacia la API Go ocurren por loopback `http://127.0.0.1:8080` en **< 0.1 ms** sin salir a la red ni requerir `metadata.google.internal`, y ambos contenedores comparten la reserva de `0.25 vCPU` por solo **$7.21 a $8.51 USD/mes** (frente a ~$21 USD/mes en Cloud Run + ~$23 USD/mes de Load Balancer en GCP).
2. **Base de Datos Relacional en `us-east-2` (`Amazon RDS for PostgreSQL 18` Single-AZ):**
   - Instancia **`db.t4g.micro` (2 vCPU Graviton2 burstable, 1 GiB RAM, Single-AZ)** en subred privada aislada (`10.20.10.0/24` en `us-east-2a` y `10.20.11.0/24` en `us-east-2b`), con **20 GB SSD `gp3`**, cifrado en reposo AES-256, `rds.force_ssl = 1` (`DATABASE_SSLMODE=verify-full`), backups automáticos diarios (7 días) y **PITR habilitado** hacia S3.
   - Costo en `us-east-2`: **$11.68 USD/mes** (cómputo) + **$2.30 USD/mes** (20 GB `gp3` + 20 GB PITR incluidos) = **$13.98 USD/mes**.
3. **Almacenamiento y Cifrado Sin Costos Fijos de KMS CMK ($0.12 USD/mes):**
   - Para no gastar $1.00/mes por cada llave CMK en KMS cuando el presupuesto meta es `< $30 USD/mes`, los buckets S3 en `us-east-2` utilizan el cifrado administrado gratuito **SSE-S3 (`AES-256`)** combinado con el **cifrado autenticado en capa de aplicación (`AES-256-GCM`)** que el backend Go **ya realiza** para documentos KYC (`internal/adapters/documentfs`) y contratos PDF (`internal/adapters/postgres/contracts`), y **SSM Parameter Store Standard (`SecureString`)** usa la llave gratuita por defecto de la cuenta (`alias/aws/ssm`).
4. **Correo Transaccional Preservado en Zoho SMTP ($0.00 USD en AWS):**
   - Se mantiene sin cambios la salida hacia **`smtp.zoho.com:587` (STARTTLS)** con el remitente `contacto@hotusoft.com` (`internal/adapters/zohoauth/mailer.go`). En AWS `us-east-2`, el tráfico saliente hacia el puerto TCP `587` no tiene bloqueos (AWS solo restringe el puerto 25).

---

## 2. Verificación de Servicios Habilitados en el AWS Free Plan (`us-east-2`)

| Servicio AWS Requerido | Estado en el AWS Free Plan (6 meses / hasta $200 USD) | Bono de Crédito Adicional en Consola (*Explore AWS*) | Cómo se Descuenta / Factura |
| :--- | :---: | :---: | :--- |
| **AWS Budgets** | **100% Habilitado** | **Otorga +$20 USD de crédito** al crear el primer presupuesto en la consola. | **Gratuito ($0.00/mes)** para alertas de presupuesto. |
| **Amazon RDS for PostgreSQL 18 (`db.t4g.micro`)** | **100% Habilitado** *(Restringe instancias grandes; permite `t4g.micro`, `t3.micro`, `t4g.small`)* | **Otorga +$20 USD de crédito** al lanzar la primera instancia RDS. | Descuenta ~$13.98/mes del saldo de créditos promocionales ($100–$200 USD). |
| **Amazon VPC, Subnets, Security Groups, IGW y S3 Gateway Endpoint** | **100% Habilitado** | N/A | **Gratuito ($0.00/mes)** (1 IPv4 pública en uso descuenta $3.65/mes de los créditos). |
| **Amazon S3 (`us-east-2`)** | **100% Habilitado** | N/A | Descuenta ~$0.12/mes (para 5 GB) de los créditos. |
| **AWS Systems Manager (SSM) Parameter Store (`Standard`)** | **100% Habilitado** | N/A | **Gratuito ($0.00/mes)** hasta 10.000 parámetros `SecureString`. |
| **Amazon CloudWatch Logs & Alarms** | **100% Habilitado** | N/A | **Always Free ($0.00/mes)** hasta 5 GB de logs, 10 métricas y 10 alarmas. |
| **Amazon ECR (`us-east-2`)** | **100% Habilitado** | N/A | Descuenta ~$0.15/mes (< 1.5 GB con Lifecycle Policy) de los créditos. |
| **Amazon ECS con AWS Fargate (`us-east-2`)** | **Habilitado** *(Verificar cuota de vCPU Fargate con [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh))* | N/A *(Si también haces las 3 tareas guiadas de EC2, Lambda y Bedrock en 10 min y las borras al instante, sumas otros **+$60 USD** para llegar al tope de **$200 USD**)* | Descuenta ~$7.21–$8.51/mes de los créditos. *(Plan de contingencia: si una cuenta Free Plan nueva tuviera Fargate restringido a 0 vCPU, el mismo Docker Compose corre en una instancia EC2 `t4g.small` / `t3.small` en la misma subred por ~$12/mes).* |

---

## 3. Diseño de IaC Agnóstico de Región: Cómo Migrar de `us-east-2` (Ohio) a `AWS Chile` (`sa-west-1`)

La región **AWS South America (Chile)** inició operaciones en acceso anticipado el **11 de noviembre de 2026** con 3 zonas de disponibilidad en Santiago y entrará en disponibilidad general (GA) durante las próximas semanas. Para que el cambio de `us-east-2` a Chile no requiera rediseñar ni un solo archivo de Terraform:

1. **Cero Zonas de Disponibilidad Hardcodeadas:**
   En lugar de escribir `"us-east-2a"` o `"us-east-2b"`, el módulo `aws-network` consulta dinámicamente las zonas activas de la región configurada:
   ```hcl
   variable "aws_region" {
     description = "Región AWS activa (inicial: us-east-2; objetivo futuro: región AWS Chile)"
     type        = string
     default     = "us-east-2"
   }

   data "aws_availability_zones" "available" {
     state = "available"
   }

   locals {
     azs = slice(data.aws_availability_zones.available.names, 0, 2)
   }
   ```
2. **Lookup Dinámico de Endpoints y Prefix Lists:**
   El Gateway VPC Endpoint de S3 se resuelve como `"com.amazonaws.${var.aws_region}.s3"`, y la versión del motor RDS se consulta con `data.aws_rds_engine_version.postgres18` para evitar fallos si la versión menor en Chile es `18.6` o `18.7`.
3. **Procedimiento de Migración Futura `us-east-2` (Ohio) $\rightarrow$ `AWS Chile (Santiago)` en 4 Pasos:**
   - **Paso 1 (Verificación Previa):** Ejecutar el script [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh) cambiando `REGION` al código de la región de Chile para confirmar que RDS PostgreSQL 18 (`postgis`, `btree_gist`, `unaccent`) y ECS Fargate ARM64 están disponibles en GA y dentro del presupuesto.
   - **Paso 2 (Infraestructura en Chile):** Aplicar los mismos módulos de OpenTofu/Terraform apuntando a la región de Chile (`var.aws_region = "<codigo-region-chile>"`).
   - **Paso 3 (Copia de Datos Cross-Region AWS-to-AWS):**
     - Copiar el snapshot cifrado de RDS desde `us-east-2` hacia la región de Chile (`aws rds copy-db-snapshot --source-db-snapshot-identifier arn:aws:rds:us-east-2:...`) o ejecutar `pg_dump -Fc` + `pg_restore` (si el volumen sigue siendo < 1 GB, `pg_dump` tarda menos de 2 minutos).
     - Sincronizar el bucket S3 con `aws s3 sync s3://espacigo-media-us-east-2 s3://espacigo-media-chile`.
   - **Paso 4 (Conmutación Instantánea en Cloudflare):** Actualizar la IP/hostname de origen en Cloudflare hacia la tarea ECS en Chile. La latencia total de las peticiones dinámicas bajará automáticamente de **~180–250 ms (Ohio)** a **~15–40 ms (Santiago local)**.
