# 05 — Análisis FinOps en `us-east-2` (Ohio), Optimización del AWS Free Plan y Tablas para el Capítulo XIV / Anexo H

**Fecha de actualización:** 15 de noviembre de 2026  
**Región de cálculo:** `us-east-2` (US East — Ohio), con tarifas oficiales On-Demand vigentes en noviembre de 2026.  
**Meta presupuestaria del propietario:** Mantener la operación mensual **por debajo de USD $30.00/mes** después de los créditos, aprovechando al máximo los **6 meses y hasta USD $200 en créditos del AWS Free Plan**.

---

## 1. Estrategia para Maximizar tus Créditos en el AWS Free Plan ($100 $\rightarrow$ $200 USD)

Tu cuenta se encuentra en el **AWS Free Plan** (duración máxima de **6 meses** con tope duro que impide cargos no autorizados a tu tarjeta). Este plan entrega **$100 USD al registrarse** y permite desbloquear **otros $100 USD adicionales ($20 USD por cada una de las 5 actividades guiadas del panel *Explore AWS* en `us-east-2`)**:

| Actividad Guiada en Consola (`us-east-2`) | Bono de Crédito | Cómo Realizarla Sin Gastar más de $0.05 USD |
| :--- | :---: | :--- |
| **1. Configurar un presupuesto en AWS Budgets** | **+$20.00 USD** | Es el **Paso 1 obligatorio** de nuestra arquitectura. Creas el presupuesto mensual de $28 USD con alertas y recibes inmediatamente $20 USD de crédito. |
| **2. Lanzar una instancia en Amazon RDS** | **+$20.00 USD** | Se desbloquea automáticamente al aprovisionar nuestra base de datos `espacigo-db` (`db.t4g.micro` PostgreSQL 18) o una instancia de prueba inicial. |
| **3. Lanzar y terminar una instancia EC2** | **+$20.00 USD** | Lanzas una instancia `t3.micro` en la consola, esperas 2 minutos a que marque completada la actividad y la **terminas (`Terminate`) inmediatamente** (costo < $0.01 USD). |
| **4. Crear una función en AWS Lambda** | **+$20.00 USD** | Creas una función "Hello World" con Function URL desde el asistente y luego la borras (100% cubierta por el Always Free Tier: $0.00 USD). |
| **5. Probar un prompt en Amazon Bedrock** | **+$20.00 USD** | Ejecutas 1 prompt de texto de 20 palabras en el Playground de Bedrock en `us-east-2` (costo < $0.001 USD). |
| **TOTAL DE CRÉDITOS ACUMULADOS** | **$200.00 USD** | Con un consumo mensual de **~$25.11 a $26.41 USD/mes**, los $200 USD cubren **el 100% de los 6 meses completos del Free Plan (~$155 USD en 6 meses)**, sobrando ~$45 USD de margen. |

---

## 2. Estimación Detallada de Costos Mensuales por Servicio en `us-east-2` (Ohio)

### 2.1 Escenario Base Operativo 24/7 (`< $30.00 USD/mes` después de los créditos)

| Servicio AWS (`us-east-2` Ohio) | Configuración Técnica Exacta | Fórmula de Cálculo Oficial (`us-east-2`, 730 horas/mes) | Costo Mensual Sin Créditos (USD) | Costo Mensual Estimado en CLP (~$980 CLP/USD) |
| :--- | :--- | :--- | :---: | :---: |
| **Amazon RDS for PostgreSQL 18 (Cómputo)** | `db.t4g.micro` (2 vCPU Graviton2, 1 GiB RAM, Single-AZ) | `$0.0160 / hora × 730 horas` | **$11.68** | $11.446 CLP |
| **Amazon RDS Storage + Backups + PITR** | `20 GB gp3` (3.000 IOPS, 125 MB/s) + 20 GB Backups automáticos y PITR en S3 (retención 7 días) | `20 GB × $0.115 / GB-mes` *(RDS provisiona backup gratis hasta el 100% del disco = 20 GB)* | **$2.30** *(o $1.60 según tarifa EBS base)* | $2.254 CLP |
| **Amazon ECS con AWS Fargate (ARM64 Graviton2)** | 1 Task 24/7 con 2 contenedores (`espacigo-web` :4000 + `espacigo-api` :8080 en `127.0.0.1`):<br>- **Perfil A:** `0.25 vCPU + 0.5 GB RAM`<br>- **Perfil B:** `0.25 vCPU + 1.0 GB RAM` | - vCPU ARM64: `0.25 × $0.03238 × 730h = $5.91`<br>- RAM 0.5 GB: `0.5 × $0.00356 × 730h = $1.30`<br>- RAM 1.0 GB: `1.0 × $0.00356 × 730h = $2.60`<br>- Disco efímero: 20 GB incluidos gratis | **$7.21** *(Perfil 0.5 GB)*<br>**$8.51** *(Perfil 1.0 GB)* | $7.066 – $8.340 CLP |
| **IPv4 Pública en Uso (ECS Task)** | 1 dirección IPv4 pública asignada a la interfaz ENI de la tarea Fargate (protegida por Security Group de Cloudflare) | `$0.005 / hora × 730 horas` | **$3.65** | $3.577 CLP |
| **Amazon VPC, Subnets, IGW y S3 Gateway Endpoint** | 1 VPC, 4 subredes, Internet Gateway, Egress-Only IGW (IPv6) y Gateway Endpoint a S3 | Sin costo por hora ni por GB procesado | **$0.00** *(NAT Gateway evitado)* | $0 CLP |
| **Amazon S3 Standard (`us-east-2`)** | `5 GB` de imágenes WebP, documentos KYC/espacios (`AES-256-GCM`), contratos y logs WORM + `SSE-S3` | `5 GB × $0.023 / GB-mes` + ~20.000 requests | **$0.12** | $118 CLP |
| **Amazon ECR (`us-east-2`)** | 3 repositorios (`api`, `web`, `dbmigrate`) con Lifecycle Policy (máx. 5 digests por repo, `< 1.5 GB` total) | `1.5 GB × $0.10 / GB-mes` | **$0.15** | $147 CLP |
| **AWS SSM Parameter Store (`Standard`)** | 8 secretos `SecureString` cifrados con la llave KMS por defecto `alias/aws/ssm` | Tier Standard: gratis hasta 10.000 parámetros | **$0.00** | $0 CLP |
| **Amazon CloudWatch + AWS Budgets** | Logs con retención de 14 días (`< 5 GB/mes`), 8 alarmas operativas y 2 presupuestos en AWS Budgets | Incluido en el *Always Free Tier* permanente de AWS | **$0.00** | $0 CLP |
| **Transferencia de Datos (Egress)** | `< 50 GB/mes` desde AWS hacia Cloudflare (el resto se sirve desde el caché de Cloudflare en Santiago) | Primeros **100 GB/mes hacia Internet son GRATIS** en AWS | **$0.00** | $0 CLP |
| **Borde Cloudflare + Correo Zoho SMTP** | Cloudflare Plan Free (DNS, Proxy, WAF básico, Cache Rules, **Zero Trust Access ≤ 50 usuarios**) + Zoho Mail actual | Planes actuales sin cargo adicional en AWS | **$0.00** | $0 CLP |
| **TOTAL MENSUAL (24/7 SIN CRÉDITOS)** | **Operación continua ininterrumpida en `us-east-2`** | **Suma verificada por SKU** | **$25.11 – $26.41 USD/mes** | **~$24.600 – $25.880 CLP/mes** |
| **TOTAL ANUAL (SIN CRÉDITOS)** | **12 meses continuos sin créditos** | `12 × costo mensual` | **$301.32 – $316.92 USD/año** | **~$295.000 – $310.500 CLP/año** |

> [!TIP]
> **Opciones para Reducir Aún Más el Consumo (~$16 a $21 USD/mes) Durante el Desarrollo:**
> 1. **Fargate Spot para el contenedor en etapa no comercial:** Si la tarea de ECS Fargate usa `FARGATE_SPOT`, el costo del contenedor (`0.25 vCPU / 1 GB`) baja de $8.51 a **~$2.65 USD/mes**, llevando el total mensual a **~$20.55 USD/mes**.
> 2. **Apagado programado nocturno (01:00 a 08:00 AM Chile) cuando no haya evaluaciones:** Detener la tarea Fargate y la instancia RDS durante 7 horas nocturnas reduce un ~29% las horas de cómputo e IPv4, bajando la factura mensual a **~$18.50 USD/mes**.

---

### 2.2 Proyección de Escenarios de Escalamiento Futuro (Medio y Alto)

| Escenario | Configuración de Cómputo y Base de Datos | Costo Mensual en `us-east-2` (Ohio) | Costo Mensual Estimado en `AWS Chile` (cuando sea GA, asumiendo +30–40% regional) |
| :--- | :--- | :---: | :---: |
| **Escenario 1: Bajo / Actual (Etapa 1 Single-AZ)**<br>*(Hasta 100k req/mes, validación y piloto inicial)* | 1 Task Fargate (`0.25 vCPU / 0.5–1 GB`) + RDS `db.t4g.micro` Single-AZ + PITR + Cloudflare Free | **$25.11 – $26.41 USD/mes**<br>*(~$25.000 CLP/mes)* | **~$34.00 – $38.00 USD/mes**<br>*(~$34.000 – $37.000 CLP/mes)* |
| **Escenario 2: Medio / Pre-Comercial con RDS Multi-AZ**<br>*(1M req/mes, 100–200 concurrentes, SLA 99.95% en BD)* | 2 Tasks Fargate (`0.5 vCPU / 1 GB` en 2 AZs) + RDS `db.t4g.small` **Multi-AZ** ($46.72) + 20 GB `gp3` Multi-AZ ($4.60) + ALB ($16.43) | **$104.24 USD/mes**<br>*(~$102.000 CLP/mes)*<br>*(O **$58.90 USD/mes** con `db.t4g.micro` Multi-AZ sin ALB)* | **~$138.00 – $148.00 USD/mes**<br>*(~$135.000 – $145.000 CLP/mes)* |
| **Escenario 3: Alto / Escala `RNF-030`**<br>*(5M req/mes, 500 concurrentes, 100 WPS)* | 2–6 Tasks Fargate (`1 vCPU / 2 GB`) + RDS `db.m7g.large` **Multi-AZ** + 50 GB `gp3` + ALB + 150 GB S3 | **$385.50 USD/mes**<br>*(~$378.000 CLP/mes)* | **~$510.00 – $540.00 USD/mes**<br>*(~$500.000 – $530.000 CLP/mes)* |

---

## 3. Configuración de Alertas de Presupuesto en AWS Budgets

En el módulo `infra/terraform/environments/bootstrap`, el primer recurso aprovisionado es `aws_budgets_budget.espacigo_monthly`:

1. **Presupuesto 1 — Control Estricto Post-Créditos (`espacigo-monthly-cap-30usd`):**
   - **Monto límite:** **`USD $28.00 / mes`** (colchón de $2.00 antes del techo de $30.00 USD).
   - **Métrica:** `UnblendedCost` (mide el consumo bruto real **antes** de descontar créditos promocionales, para que puedas vigilar el gasto real incluso mientras tengas los $200 USD de crédito).
   - **4 Reglas de Notificación por Correo:**
     - **Alerta 1 (50% Actual = $14.00 USD):** Confirmación de mitad de mes normal.
     - **Alerta 2 (80% Actual = $22.40 USD):** Aviso preventivo.
     - **Alerta 3 (100% Forecasted / Proyectado = $28.00 USD):** Se dispara apenas la proyección de AWS estime que cerrarás el mes por encima de $28 USD.
     - **Alerta 4 (100% Actual = $28.00 USD):** Alerta crítica al equipo fundador.
2. **Presupuesto 2 — Monitoreo de Consumo de Créditos (`espacigo-net-spend-zero`):**
   - **Monto límite:** **`USD $1.00 / mes`** sobre `NetUnblendedCost` (costo neto después de créditos), que te avisa instantáneamente si algún servicio no elegible generara más de $1 USD de cargo neto.

---

## 4. Insumo Preparado para Actualizar el Capítulo XIV y Anexo H en `Proyectotitulo`

Una vez validada la ejecución en tu cuenta de AWS, la siguiente tabla comparativa documenta formalmente para el **Capítulo XIV (Factibilidad Económica)** y el **Anexo H** de la Memoria de Título el impacto financiero del rediseño desde GCP hacia AWS + Cloudflare:

| Indicador Económico / Ítem de Infraestructura | Diseño Previo en GCP (`03_06_infraestructura.md` / `espacigo-testprod`) | Nuevo Diseño Cloud-Native en AWS (`us-east-2` + Cloudflare Edge) | Ahorro / Impacto Neto para el Proyecto |
| :--- | :---: | :---: | :--- |
| **Gasto Mensual en Etapa de Validación / Año 1 (Sin créditos)** | **$88,50 USD/mes** (`testprod` real)<br>*($24,65 USD/mes solo apagando entorno en m1–6)* | **$25,11 – $26,41 USD/mes**<br>*(Operativo 24/7 con PITR activo)* | **−70% de reducción** frente al gasto actual de `espacigo-testprod` |
| **Gasto Mensual en Etapa Piloto (Meses 7 a 36 de la Tesis)** | **$307,72 USD/mes netos**<br>*($366.192 CLP/mes con IVA)* | **$26,41 USD/mes** *(Single-AZ)* a **$58,90 – $104,24 USD/mes** *(Multi-AZ)* | **Ahorro de $203 a $281 USD/mes** (~$240.000 a $330.000 CLP/mes) |
| **Gasto Mensual en Perfil Alta Disponibilidad (HA)** | **$564,55 USD/mes netos**<br>*($671.818 CLP/mes con IVA)* | **$104,24 USD/mes** (`db.t4g.small` Multi-AZ) a **$385,50 USD/mes** (`db.m7g.large` Multi-AZ) | **−32% a −81% menor costo** por eliminar Balanceador Global + Cloud Armor usando Cloudflare |
| **Desembolso Acumulado de Caja Cloud en 36 Meses (3 Años)** | **~$11.230.000 CLP** *(6m ensayo + 30m piloto en GCP)* | **~$780.000 a $2.450.000 CLP** *(con los primeros 6 meses a $0 por AWS Free Plan)* | **Ahorro acumulado de ~$8.780.000 a $10.450.000 CLP en 3 años** |
| **Impacto en el VAN del Escenario Bootstrap (Tasa 12%, 3 años)** | **VAN = −$15.140.998 CLP** | Mejora el VAN en **+$7,1M a +$8,4M CLP** en valor presente neto | Reduce en más de la mitad el déficit del período de incubación sin alterar la comisión del 3% |
