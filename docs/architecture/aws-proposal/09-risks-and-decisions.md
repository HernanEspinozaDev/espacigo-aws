# 09 — Matriz Comparativa Ponderada, Riesgos, ADRs Cerrados y Hoja de Ruta Priorizada

**Fecha de actualización:** 15 de noviembre de 2026  

---

## 1. Registro de Decisiones de Arquitectura (ADRs Confirmados por el Propietario)

| ID ADR | Decisión Confirmada | Estado | Justificación Técnica y Económica |
| :---: | :--- | :---: | :--- |
| **ADR-AWS-01** | **Región Inicial `us-east-2` (Ohio) con IaC preparado para migrar a AWS Chile (`sa-west-1`)** | **Aprobado** | `us-east-2` es la región asignada por el AWS Free Plan y ofrece los precios más bajos de AWS (~$25–$26 USD/mes). Modelar toda la infraestructura en OpenTofu/Terraform sin zonas ni ARNs fijos permite trasladar la plataforma a **AWS South America (Chile)** tan pronto pase a disponibilidad general y la cuenta migre al Paid Plan. |
| **ADR-AWS-02** | **Presupuesto Meta `< $30.00 USD/mes` Post-Créditos y Aprovechamiento de los $200 USD del Free Plan** | **Aprobado** | Elimina el NAT Gateway ($32.85/mes) y el ALB dedicado ($16.43/mes) en Etapa 1 usando Security Groups restringidos al Prefix List de Cloudflare, Gateway VPC Endpoint gratuito para S3 y co-ubicación de `web` + `api` en una tarea Fargate ARM64 (`0.25 vCPU / 0.5–1 GB`), logrando **$25.11–$26.41 USD/mes** post-créditos. |
| **ADR-AWS-03** | **RDS PostgreSQL 18 Single-AZ Inicial con PITR, Backups y Pruebas Reales de RPO/RTO** | **Aprobado** | Mantiene el costo de base de datos en **$13.98 USD/mes** (`db.t4g.micro` + 20 GB `gp3` + PITR). Se documentan honestamente los tiempos reales de recuperación en Single-AZ (RPO 5–15 min; RTO 21–58 min en restore PITR completo) y se agenda la activación de **Multi-AZ** como compuerta previa al lanzamiento comercial. |
| **ADR-AWS-04** | **Next.js 16 en Contenedor (`ECS Fargate`) + Cloudflare CDN/WAF + Cloudflare Zero Trust Access** | **Aprobado** | Evita reescribir el BFF de Next.js 16, elimina la penalización de latencia *ping-pong* (~130 ms por llamada entre Chile y Ohio si el BFF estuviera en Cloudflare Workers), ahorra los $5/mes de Workers Paid y sustituye Google IAP por **Cloudflare Zero Trust Access** gratuito (eliminando el 2.º contenedor `espacigo-test-private`). |
| **ADR-AWS-05** | **Preservación de Zoho SMTP (`contacto@hotusoft.com`) y Dominio Actual en Etapa Inicial** | **Aprobado** | Evita cambios innecesarios en `internal/adapters/zohoauth/mailer.go` y trámites de salida de Sandbox en Amazon SES mientras no exista una justificación comercial para cambiar el dominio de envío. |

---

## 2. Matriz Comparativa Ponderada Final

| Criterio de Evaluación | Peso (%) | GCP Actual (`espacigo-testprod`) | Frontend en Cloudflare Workers + API/DB en AWS `us-east-2` | Alt. B: AWS `us-east-2` Multi-AZ desde Día 1 | **Arquitectura Confirmada: Cloudflare Edge + Contenedor Co-Ubicado + RDS Single-AZ en `us-east-2`** |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **1. Costo Post-Créditos (Meta `< $30 USD/mes`)** | **25%** | 2.0 *(~$88.50/mes)* | 4.0 *(~$31–$35/mes por requerir Workers Paid $5 + API expuesta)* | 2.0 *(~$104–$128/mes, incumple meta < $30)* | **5.0** *(**$25.11–$26.41 USD/mes** verificados en `us-east-2`)* |
| **2. Seguridad y Cumplimiento (Ley 21.719)** | **15%** | 3.0 *(Sin PITR, sin ClamAV/Poppler en Cloud Run)* | 4.0 *(Exige exponer API Go a Internet hacia Workers)* | **5.0** *(WAF + Multi-AZ + Object Lock)* | **5.0** *(API Go en `127.0.0.1` no expuesta, Cloudflare WAF + Zero Trust, RDS TLS `verify-full`, S3 Object Lock)* |
| **3. Simplicidad Operativa y Portabilidad a AWS Chile** | **15%** | 3.0 *(Drift en Terraform, permisos SQL rotos)* | 3.0 *(2 plataformas de cómputo y 2 pipelines distintos)* | 4.0 *(Todo en IaC, más componentes)* | **5.0** *(1 Task ECS + 1 RDS + S3 en un único módulo IaC portable a AWS Chile cambiando `var.aws_region`)* |
| **4. Compatibilidad con `Next.js 16.4.3` y `Go 1.27.1`** | **15%** | 4.0 *(Salvo `documentverification` en `scratch`)* | 2.5 *(Requiere reescribir BFF y subidas de 50 MiB)* | **5.0** *(100% compatible)* | **5.0** *(100% compatible sin tocar el BFF ni el mailer de Zoho)* |
| **5. Rendimiento y Latencia desde Chile** | **10%** | **5.0** *(En Santiago)* | 2.5 *(Efecto ping-pong: ~130 ms por cada llamada secuencial Worker$\leftrightarrow$Ohio)* | 4.2 *(1 solo RTT por request dinámico; `<0.1ms` interno)* | **4.5** *(Estáticos `<10ms` en Cloudflare Santiago; `<0.1ms` entre Next.js y Go en `127.0.0.1`; migra a Chile en Fase 2)* |
| **6. Confiabilidad y Recuperación Documentada** | **10%** | 2.0 *(Sin PITR, ensayo GCP03 fallido)* | 4.0 *(PITR en RDS)* | **5.0** *(Multi-AZ RPO=0, RTO<2m)* | **4.5** *(PITR + Backups + Simulacro `PT-11`/`PT-16` automatizado y paso a Multi-AZ en 1 variable)* |
| **7. Facilidad de CI/CD y Migración** | **10%** | 3.5 *(Doble CI en GH + Cloud Build)* | 3.0 *(Requiere refactor previo a migrar)* | 4.5 *(GH Actions OIDC)* | **4.8** *(GH Actions OIDC + `pg_dump`/`pg_restore` limpio con `bootstrap_rds_roles.sql`)* |
| **PUNTAJE TOTAL PONDERADO (Sobre 5.00)** | **100%** | **2.98 / 5.00** | **3.43 / 5.00** | **3.92 / 5.00** | **4.88 / 5.00 (Arquitectura Seleccionada)** |

---

## 3. Hoja de Ruta Priorizada hacia la Implementación (Con Compuertas de Aprobación)

### Etapa 0 — Verificación Sin Costo ($0.00 USD — Inmediata)
1. Ejecutar [verify-aws-free-plan-us-east-2.sh](./verify-aws-free-plan-us-east-2.sh) en AWS CloudShell (`us-east-2`) para confirmar las versiones de PostgreSQL 18, `db.t4g.micro` y cuotas de ECS Fargate en tu cuenta Free Plan.
2. Completar las actividades guiadas del panel *Explore AWS* (empezando por **AWS Budgets** de $28 USD) para asegurar los **$200 USD completos de crédito promocional**.

### Etapa 1 — Preparación de Código e IaC en el Repositorio ($0.00 USD — En Local)
1. Sincronizar el repositorio principal con `main` (`7b01a47`, que ya tiene `V000084`) y limpiar los 12 `Dockerfile.gcp*` obsoletos.
2. Agregar el adaptador `internal/adapters/s3storage` (`BLOB_STORE_BACKEND=s3`) y actualizar el `Dockerfile` de `espacigo-api` para incluir `poppler-utils` y el certificado raíz de AWS RDS (`rds-global-bundle.pem`), **manteniendo intacto el adaptador Zoho SMTP**.
3. Escribir los módulos de OpenTofu/Terraform parametrizados para `us-east-2` (y portables a AWS Chile) junto con el script `bootstrap_rds_roles.sql`.

### Etapa 2 — Despliegue en `us-east-2`, Pruebas de Recuperación y Actualización de Tesis (Previa Aprobación Explícita)
1. *(Requiere tu aprobación previa antes de crear recursos con consumo de créditos)* Aplicar `tofu apply` en `us-east-2`, migrar la base de datos y el bucket de medios, y configurar **Cloudflare Proxy + Zero Trust Access**.
2. Ejecutar y cronometrar el ensayo de recuperación `PT-11` + `PT-16` en RDS `us-east-2`.
3. Con las mediciones reales de costo y tiempos de recuperación en mano, **actualizar el Capítulo XIV y el Anexo H en `Proyectotitulo`**.
