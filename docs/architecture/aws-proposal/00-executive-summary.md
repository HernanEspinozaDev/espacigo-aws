# 00 — Resumen Ejecutivo: Rediseño Cloud-Native en AWS (`us-east-2` $\rightarrow$ AWS Chile) — Proyecto EspaciGo

**Fecha de actualización:** 15 de noviembre de 2026  
**Estado:** Arquitectura verificada contra la cuenta AWS real `805068224315` (`--profile aws-dev`, región `us-east-2` Ohio, plan `FREE` activo hasta el `2027-05-15`).  
**Repositorios auditados:** `espaciGo` (rama local `19dabb1` y rama `main` `7b01a47`) y `Proyectotitulo` (Memoria de Título INACAP, Noviembre 2026).

---

## 1. Estado Verificado de tu Cuenta AWS (`805068224315` — `us-east-2` Ohio)

Mediante consultas 100% de solo lectura (`aws freetier`, `aws rds`, `aws service-quotas`, `aws ec2`, `aws ce` con `--profile aws-dev`) se confirmó el estado exacto de tu cuenta:

| Parámetro Consultado | Valor Real Verificado en tu Cuenta AWS | Impacto en el Proyecto EspaciGo |
| :--- | :--- | :--- |
| **Account ID y Tipo de Plan** | Cuenta `805068224315` — `accountPlanType: "FREE"`, `accountPlanStatus: "ACTIVE"` | Tope de gasto protegido por el AWS Free Plan; expira el **15 de mayo de 2027** (`2027-05-15T05:28:43Z`). |
| **Saldo Actual de Créditos Promocionales** | **`USD $100.00` disponibles** (`accountPlanRemainingCredits: 100.0 USD`) + **$0.00 USD consumidos** en noviembre | Tienes el 100% del crédito inicial intacto, más **$100.00 USD adicionales (`NOT_STARTED`)** listos para desbloquear con 5 de las 6 actividades de `$20.00 USD` c/u (`Budgets`, `RDS`, `DynamoDB`, `Lambda`, `EC2`, `Bedrock`). |
| **Disponibilidad de PostgreSQL 18.6 en `us-east-2`** | Versiones `18.1` a **`18.6`** habilitadas (`postgres18`). Clases **`db.t4g.micro`** y **`db.t4g.small`** disponibles con almacenamiento **`gp3`** y `MultiAZCapable: True` | **Paridad 100% exacta** con la versión `PostgreSQL 18.6` que corre actualmente en Docker local y en GCP Cloud SQL (`espacigo-testprod-db`). |
| **Cuotas de AWS Fargate en `us-east-2`** | `Fargate On-Demand vCPU`: **`30.0 vCPUs`**<br>`Fargate Spot vCPU`: **`30.0 vCPUs`** | **100% habilitado.** Nuestra arquitectura solo requiere **`0.25 vCPU`** (menos del 1% de tu cuota autorizada). |
| **Zonas de Disponibilidad Activas en `us-east-2`** | `us-east-2a` (`use2-az1`), `us-east-2b` (`use2-az2`), `us-east-2c` (`use2-az3`) | 3 zonas disponibles para subredes y simulacros de restauración cross-AZ. |
| **Managed Prefix Lists en `us-east-2`** | S3 IPv4: `pl-7ba54012` / S3 IPv6: `pl-b428cddd` | Permite configurar el Gateway VPC Endpoint de S3 sin costo ($0/mes) y sin NAT Gateway. |
| **Recursos actuales (`ECS`, `ECR`, `SSM`, `Budgets`)** | `0` clústeres ECS, `0` repositorios ECR, `0` parámetros SSM, `0` presupuestos en AWS Budgets | Cuenta completamente limpia. Crear el primer presupuesto en **AWS Budgets** activará tus alertas y desbloqueará automáticamente la actividad `7cd845309d890faf7270bb426560c01e` (**+$20.00 USD de crédito**). |

---

## 2. Decisiones Arquitectónicas Confirmadas e Incorporadas

| # | Eje de Decisión | Definición Confirmada por el Propietario | Cómo se Materializa en el Diseño Técnico |
| :---: | :--- | :--- | :--- |
| **1** | **Región Inicial y Región Futura** | - **Inicial:** **`us-east-2` (US East — Ohio)**.<br>- **Futura:** **AWS South America (Chile — `sa-west-1`)**, operativa en acceso anticipado desde el 11-nov-2026 y próxima a disponibilidad general (GA). | Cero hardcoding regional. Todos los módulos de OpenTofu/Terraform reciben `var.aws_region = "us-east-2"` y permiten migrar a la región de Chile cambiando esa variable y ejecutando copia cross-region de snapshot RDS + S3 sync. |
| **2** | **Presupuesto y AWS Free Plan** | - **Plan actual:** **AWS Free Plan de 6 meses** ($100 USD activos + $100 USD desbloqueables = **$200 USD**).<br>- **Meta post-créditos:** Mantener la operación base **por debajo de USD $30.00/mes**. | La arquitectura diseñada en `us-east-2` tiene un costo mensual verificado de **$25.11 a $26.41 USD/mes** después de los créditos (24/7), incluyendo **AWS Budgets** con alertas al 50%, 80% y 100%. |
| **3** | **PostgreSQL 18.6 y Recuperación** | - Iniciar con **Amazon RDS for PostgreSQL 18.6 Single-AZ (`db.t4g.micro`)** con backups automáticos, PITR, cifrado y monitoreo.<br>- **Sin supuestos infundados:** Documentar los límites reales de RPO/RTO en Single-AZ y evaluar Multi-AZ antes del lanzamiento comercial. | Se documenta por qué Single-AZ logra un RPO típico de ~5 min por `archive_timeout=300s`, pero puede sufrir **RPO de 5 a 15+ min** ante pérdida física abrupta del volumen EBS local y un **RTO real de 21 a 58 minutos** en una restauración PITR completa con replay de privacidad (`PT-16`). |
| **4** | **Frontend Next.js 16, Cloudflare y Evaluadores** | - Mantener **Next.js 16 en contenedor** (sin reescribir el BFF).<br>- Usar **Cloudflare** como DNS, proxy CDN y capa de protección.<br>- Sustituir Google IAP por **Cloudflare Zero Trust Access**.<br>- Comparar rigurosamente contra **Cloudflare Workers**. | Se co-ubican `espacigo-web` (Next.js :4000) y `espacigo-api` (Go :8080 en `127.0.0.1`) dentro de una misma Task de ECS Fargate en `us-east-2` (latencia BFF$\rightarrow$API `< 0.1 ms`), evitando el efecto *ping-pong* (~130 ms por llamada secuencial) que ocurriría si el BFF corriera en Cloudflare Workers en Chile y la BD en Ohio. |
| **5** | **Dominio, Correo Zoho y Tesis** | - Mantener el dominio actual (`testingpage.store`) y el correo en **Zoho SMTP (`contacto@hotusoft.com`)**.<br>- Actualizar el **Capítulo XIV y Anexo H** en `Proyectotitulo` una vez validados los costos reales en AWS. | Se conserva intacto el adaptador `internal/adapters/zohoauth/mailer.go` (`smtp.zoho.com:587` con STARTTLS). |

---

## 3. Resumen de Costos Mensuales en `us-east-2` (Ohio) — Meta `< $30 USD/mes`

| Servicio AWS / Componente (`us-east-2`) | Especificación Exacta Verificada en `us-east-2` | Costo Mensual Sin Créditos (USD) | Costo con Créditos AWS Free Plan (Hasta Mayo 2027) |
| :--- | :--- | :---: | :---: |
| **Amazon RDS for PostgreSQL 18.6** | `db.t4g.micro` (2 vCPU Graviton2, 1 GiB RAM, Single-AZ, 730 h) | **$11.68** | **$0.00** *(Descuenta de créditos + desbloquea actividad de **+$20 USD**)* |
| **Almacenamiento RDS + Backups + PITR** | `20 GB gp3` (`$0.115/GB`) + 20 GB Backups/PITR en S3 gratis (retención 7 días) | **$2.30** | **$0.00** *(Descuenta de créditos)* |
| **Amazon ECS con AWS Fargate (ARM64)** | 1 Task co-ubicada (`web` + `api`): `0.25 vCPU` (`$5.91`) + `0.5 GB` (`$1.30`) a `1.0 GB` RAM (`$2.60`) 24/7 | **$7.21 – $8.51** | **$0.00** *(Descuenta de créditos; o ~$2.65/mes si se usa `FARGATE_SPOT`)* |
| **Dirección IPv4 Pública (ECS Task)** | 1 IPv4 pública dinámica en la tarea Fargate (`$0.005/hora × 730 h`, sin NAT Gateway ni ALB) | **$3.65** | **$0.00** *(Descuenta de créditos)* |
| **Amazon S3 + Gateway VPC Endpoint** | `5 GB` S3 Standard (`$0.12`) con `SSE-S3` + cifrado de aplicación `AES-256-GCM` + `pl-7ba54012` ($0) | **$0.12** | **$0.00** *(Descuenta de créditos)* |
| **Amazon ECR (con Lifecycle Policy)** | `< 1.5 GB` para las 3 imágenes (`api`, `web`, `dbmigrate`), reteniendo máx. 5 tags | **$0.15** | **$0.00** *(Descuenta de créditos)* |
| **SSM Parameter Store + CloudWatch + AWS Budgets** | 8 secretos `SecureString` Standard (`$0`), `< 5 GB` logs CW con retención 14 días (`$0`), 8 alarmas (`$0`), 2 Budgets (`$0`) | **$0.00** | **$0.00** *(Always Free + Budgets desbloquea actividad de **+$20 USD**)* |
| **Cloudflare Free + Zoho Mail Actual** | DNS Proxy, DDoS, WAF Free, Cache Rules, **Zero Trust Access (≤ 50 usuarios)** y Zoho SMTP `:587` | **$0.00** | **$0.00** |
| **TOTAL MENSUAL OPERATIVO 24/7** | **Piso Operativo Completo en `us-east-2`** | **$25.11 – $26.41 USD/mes**<br>*(~$24.600 – $25.900 CLP/mes)* | **$0.00 USD/mes de bolsillo**<br>*(Tus $100 USD actuales + $40–$100 USD de actividades cubren los 6 meses completos hasta mayo 2027)* |

---

## 4. Índice de Entregables en `docs/architecture/aws-proposal/`

- [00-executive-summary.md](./00-executive-summary.md) — Resumen ejecutivo con datos verificados de la cuenta `805068224315` en `us-east-2`.
- [01-current-state-audit.md](./01-current-state-audit.md) — Auditoría forense del estado actual (Software, GCP, PostgreSQL y CI/CD).
- [02-requirements-and-assumptions.md](./02-requirements-and-assumptions.md) — Requisitos y registro de las 5 decisiones cerradas por el propietario.
- [03-architecture-alternatives.md](./03-architecture-alternatives.md) — Evaluación de alternativas y comparativa profunda: **Next.js 16 en Contenedor vs. Cloudflare Workers** + **Cloudflare Zero Trust Access**.
- [04-recommended-architecture.md](./04-recommended-architecture.md) — Arquitectura detallada en **`us-east-2` (Ohio)** y diseño preparado para migrar a **AWS South America (Chile)**.
- [05-cost-analysis.md](./05-cost-analysis.md) — Modelo FinOps detallado en `us-east-2` (`< $30 USD/mes`), actividades de crédito de la cuenta `805068224315` y borrador comparativo para el **Capítulo XIV y Anexo H** de la tesis.
- [06-security-and-reliability.md](./06-security-and-reliability.md) — Seguridad, análisis fundamentado de **RPO y RTO reales en RDS Single-AZ vs. Multi-AZ**, backups y protocolo de pruebas de recuperación.
- [07-cicd-and-iac.md](./07-cicd-and-iac.md) — Diseño de IaC multi-región (`us-east-2` $\rightarrow$ Chile), GitHub Actions OIDC y migraciones SQL versionadas.
- [08-migration-roadmap.md](./08-migration-roadmap.md) — Hoja de ruta de migración fase 1 (`GCP` $\rightarrow$ `AWS us-east-2`) y fase 2 (`AWS us-east-2` $\rightarrow$ `AWS Chile`).
- [09-risks-and-decisions.md](./09-risks-and-decisions.md) — Matriz comparativa ponderada, ADRs cerrados y hoja de ruta priorizada.
- [10-open-questions.md](./10-open-questions.md) — Resultado verificado de la cuenta `805068224315` y compuerta de autorización previa a implementar.
- [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh) — Script corregido y verificado con `--profile aws-dev`.
