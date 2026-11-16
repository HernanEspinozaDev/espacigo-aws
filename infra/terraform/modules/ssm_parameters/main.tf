terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }
}

variable "environment" {
  description = "Ambiente de despliegue (local, staging, prod)"
  type        = string
}

variable "cors_allowed_origins" {
  description = "Orígenes permitidos para CORS separados por coma"
  type        = string
  default     = "http://127.0.0.1:3000,http://127.0.0.1:4200,http://127.0.0.1:4201"
}

variable "frontend_public_origin" {
  description = "Origen público del frontend"
  type        = string
  default     = "http://127.0.0.1:3000"
}

variable "database_host" {
  description = "Host de base de datos PostgreSQL"
  type        = string
  default     = "172.24.0.3"
}

variable "database_port" {
  description = "Puerto de base de datos PostgreSQL"
  type        = string
  default     = "5432"
}

variable "database_name" {
  description = "Nombre de la base de datos PostgreSQL"
  type        = string
  default     = "espacigo_aws_local"
}

variable "database_user" {
  description = "Usuario login de base de datos"
  type        = string
  default     = "espacigo_api_login"
}

variable "database_role" {
  description = "Rol de ejecución en PostgreSQL (SET ROLE)"
  type        = string
  default     = "espacigo_runtime"
}

variable "bucket_names" {
  description = "Mapa con los nombres de los buckets S3"
  type        = map(string)
  default     = {}
}

locals {
  prefix = "/espacigo/${var.environment}"
  parameters = {
    "${local.prefix}/app_profile"              = var.environment
    "${local.prefix}/cors_allowed_origins"     = var.cors_allowed_origins
    "${local.prefix}/frontend_public_origin"   = var.frontend_public_origin
    "${local.prefix}/database/host"            = var.database_host
    "${local.prefix}/database/port"            = var.database_port
    "${local.prefix}/database/name"            = var.database_name
    "${local.prefix}/database/user"            = var.database_user
    "${local.prefix}/database/set_role"        = var.database_role
    "${local.prefix}/s3/evidence_bucket"       = lookup(var.bucket_names, "evidence", "espacigo-evidence-${var.environment}")
    "${local.prefix}/s3/media_bucket"          = lookup(var.bucket_names, "media", "espacigo-media-${var.environment}")
    "${local.prefix}/s3/kyc_identity_bucket"   = lookup(var.bucket_names, "kyc_identity", "espacigo-kyc-identity-${var.environment}")
    "${local.prefix}/s3/kyc_space_bucket"      = lookup(var.bucket_names, "kyc_space", "espacigo-kyc-space-${var.environment}")
    "${local.prefix}/s3/privacy_replay_bucket" = lookup(var.bucket_names, "privacy_replay", "espacigo-privacy-replay-${var.environment}")
  }
}

resource "aws_ssm_parameter" "params" {
  for_each  = local.parameters
  name      = each.key
  value     = each.value
  type      = "String"
  overwrite = true

  tags = {
    Environment = var.environment
    Project     = "espacigo"
  }
}

output "parameter_names" {
  description = "Lista de parámetros SSM creados"
  value       = [for k, v in aws_ssm_parameter.params : v.name]
}
