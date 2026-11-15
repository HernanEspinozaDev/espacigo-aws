# 10 — Resultados Verificados de tu Cuenta AWS (`805068224315`) y Compuerta de Aprobación

**Fecha de actualización:** 15 de noviembre de 2026  
**Perfil AWS CLI verificado (solo lectura):** `--profile aws-dev --region us-east-2` (`/home/nandev/.local/bin/aws`, `aws-cli/2.40.3`)

---

## 1. Resultados Reales de la Verificación Read-Only en `us-east-2` (Ohio)

Se corrigieron los dos flags en [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh) (`--default-only` sin espacio y el filtro `StorageType=='gp3'` dentro de `--query`) y se consultaron en modo de solo lectura las APIs de `freetier` y `service-quotas` para completar la radiografía exacta de tu cuenta:

1. **Estado del AWS Free Plan y Saldo de Créditos (`aws freetier get-account-plan-state`):**
   - **Account ID:** `805068224315`
   - **Plan:** `FREE` (`ACTIVE`)
   - **Fecha de expiración de los 6 meses:** **`15 de mayo de 2027`** (`2027-05-15T05:28:43Z`).
   - **Créditos disponibles hoy:** **`USD $100.00`** (`accountPlanRemainingCredits: 100.0 USD`).
   - **Consumo acumulado (1–15 nov 2026):** **`USD $0.00`** en `UnblendedCost` y `NetUnblendedCost`.
2. **Actividades de Recompensa Disponibles (`aws freetier list-account-activities` — +$100.00 USD adicionales):**
   Tu cuenta tiene **6 actividades en estado `NOT_STARTED` de $20.00 USD cada una** (puedes completar hasta 5 para sumar el máximo de **+$100.00 USD** adicionales y llegar a **$200.00 USD**):
   - `7cd845309d890faf7270bb426560c01e`: *"Set up a cost budget using AWS Budgets"* (**+$20.00 USD** — se completará automáticamente en el Paso 1 de nuestro bootstrap, a costo $0).
   - `8205347977b6d2134653bc122ea7d4f3`: *"Create an Aurora or RDS database"* (**+$20.00 USD** — se completará al lanzar nuestra instancia RDS PostgreSQL 18.6).
   - `5312d35447350cf5e1e090cb6cc57b80`: *"Create your first Amazon DynamoDB table"* (**+$20.00 USD** — crear una tabla DynamoDB en modo On-Demand vacía cuesta **$0.00 USD**).
   - `798b3b7acc72600f4aed6544f1c1e7fb`: *"Create a web app using AWS Lambda"* (**+$20.00 USD** — 100% gratis bajo el Always Free Tier).
   - `6fffb441b82fa60bf08e7e869e0d96fb`: *"Launch an instance using EC2"* (**+$20.00 USD**).
   - `8ad1ecaa66a78d4e3c2f8a678d28b595`: *"Use a foundation model in the Amazon Bedrock playground"* (**+$20.00 USD**).
3. **Motor PostgreSQL 18.6 y Clases de Instancia en `us-east-2`:**
   - Versiones disponibles: `17.7–17.11` y **`18.1–18.6` (`postgres18`)**.
   - Clases **`db.t4g.micro`** y **`db.t4g.small`** con almacenamiento **`gp3`** y `MultiAZCapable: True` están **100% habilitadas** en `us-east-2`.
4. **Cuotas de AWS Fargate y Zonas en `us-east-2` (`aws service-quotas`):**
   - `Fargate On-Demand vCPU resource count`: **`30.0 vCPUs`** (solo necesitamos `0.25 vCPU`).
   - `Fargate Spot vCPU resource count`: **`30.0 vCPUs`**.
   - Zonas activas: `us-east-2a` (`use2-az1`), `us-east-2b` (`use2-az2`), `us-east-2c` (`use2-az3`).
   - Managed Prefix List oficial para el Gateway Endpoint de S3 en `us-east-2`: `pl-7ba54012` (IPv4) y `pl-b428cddd` (IPv6).

---

## 2. Compuerta de Autorización para el Siguiente Paso

La propuesta de arquitectura y el modelo de costos en `us-east-2` están completos, verificados contra tu cuenta `805068224315` y documentados en los archivos `00` al `10`.

Cuando me des tu autorización expresa, podemos proceder en este orden seguro:
1. **Fase A (Sin consumo de créditos — $0.00 USD):** Preparar en el repositorio los módulos de OpenTofu/Terraform (`infra/terraform/environments/aws-main` y `bootstrap`), el adaptador S3 (`internal/adapters/s3storage`) y aplicar únicamente el presupuesto de **AWS Budgets ($28 USD/mes)** para activar las alertas y desbloquear los primeros **+$20 USD de crédito**.
2. **Fase B (Previa confirmación tuya):** Desplegar RDS PostgreSQL 18.6 (`db.t4g.micro`), S3, ECR y la tarea de ECS Fargate en `us-east-2`.
