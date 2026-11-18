variable "aws_region" {
  description = "Región AWS activa (inicial: us-east-2 Ohio; objetivo futuro: región AWS Chile)"
  type        = string
  default     = "us-east-2"
}

variable "environment" {
  description = "Ambiente de despliegue (prod, staging)"
  type        = string
  default     = "prod"
}

variable "monthly_budget_limit_usd" {
  description = "Límite de alerta mensual en AWS Budgets (Meta: < 30 USD/mes)"
  type        = string
  default     = "28.0"
}

variable "budget_alert_email" {
  description = "Correo para recibir alertas de presupuesto AWS Budgets"
  type        = string
  default     = "contacto@hotusoft.com"
}

variable "rds_instance_class" {
  description = "Clase de cómputo para Amazon RDS PostgreSQL 18"
  type        = string
  default     = "db.t4g.micro"
}

variable "public_tls_certificate_arn" {
  description = "ARN del certificado ACM emitido en la región del proyecto. Null omite el NLB público."
  type        = string
  default     = null
}

variable "cors_allowed_origins" {
  description = "Orígenes CORS permitidos (Cloudflare Frontend)"
  type        = string
  default     = "https://espacigo.testingpage.store,https://espacigo-demo.testingpage.store"
}

variable "frontend_public_origin" {
  description = "URL base pública del frontend en producción"
  type        = string
  default     = "https://espacigo.testingpage.store"
}


variable "enable_persistent_secretsmanager_endpoint" {
  description = "Enable only when private production tasks require continuous access to Secrets Manager."
  type        = bool
  default     = false
}

variable "migrator_seed_image" {
  description = "Digest of a promoted dbmigrate image used to register the private migration task. Null leaves the task unprovisioned."
  type        = string
  default     = null
}
