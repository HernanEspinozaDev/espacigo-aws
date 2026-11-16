terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }
}

variable "aws_region" {
  description = "Región AWS emulada (us-east-1 por defecto en aws-emulator)"
  type        = string
  default     = "us-east-1"
}

variable "emulator_endpoint" {
  description = "Endpoint URL de aws-emulator"
  type        = string
  default     = "https://127.0.0.1:9297"
}

variable "custom_ca_bundle" {
  description = "Ruta al certificado CA TLS generado por aws-emulator"
  type        = string
  default     = "~/.aws/emulator/9297/ca.pem"
}

variable "shared_credentials_file" {
  description = "Ruta al archivo de credenciales de aws-emulator"
  type        = string
  default     = "~/.aws/emulator/9297/emulator_credentials"
}

provider "aws" {
  region                      = var.aws_region
  shared_credentials_files    = [pathexpand(var.shared_credentials_file)]
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true
  custom_ca_bundle            = pathexpand(var.custom_ca_bundle)

  endpoints {
    s3   = var.emulator_endpoint
    ssm  = var.emulator_endpoint
    logs = var.emulator_endpoint
    iam  = var.emulator_endpoint
    sts  = var.emulator_endpoint
  }
}

# 1. Buckets S3 (5 buckets para EspaciGo en aws-emulator)
module "s3_storage" {
  source        = "../../modules/s3_storage"
  environment   = "local"
  bucket_suffix = "local"
}

# 2. Parámetros SSM en /espacigo/local/*
module "ssm_parameters" {
  source                 = "../../modules/ssm_parameters"
  environment            = "local"
  database_host          = "172.24.0.3"
  database_port          = "5432"
  database_name          = "espacigo_aws_local"
  database_user          = "espacigo_api_login"
  database_role          = "espacigo_runtime"
  cors_allowed_origins   = "http://127.0.0.1:3000,http://127.0.0.1:4200,http://127.0.0.1:4201"
  frontend_public_origin = "http://127.0.0.1:3000"
  bucket_names           = module.s3_storage.bucket_names
}

# 3. Observabilidad CloudWatch Logs
module "observability" {
  source            = "../../modules/observability"
  environment       = "local"
  retention_in_days = 1
}

# 4. Roles IAM y Políticas de Privilegios Mínimos
module "iam" {
  source      = "../../modules/iam"
  environment = "local"
  account_id  = "000000000000"
  aws_region  = var.aws_region
  bucket_arns = values(module.s3_storage.bucket_arns)
}

output "s3_bucket_names" {
  value = module.s3_storage.bucket_names
}

output "ssm_parameter_names" {
  value = module.ssm_parameters.parameter_names
}

output "log_group_names" {
  value = module.observability.log_group_names
}

output "iam_task_role_arn" {
  value = module.iam.api_task_role_arn
}
