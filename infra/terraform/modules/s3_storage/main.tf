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

variable "bucket_suffix" {
  description = "Sufijo para los nombres de los buckets"
  type        = string
  default     = ""
}

locals {
  suffix = var.bucket_suffix != "" ? "-${var.bucket_suffix}" : "-${var.environment}"
  buckets = {
    evidence       = "espacigo-evidence${local.suffix}"
    media          = "espacigo-media${local.suffix}"
    kyc_identity   = "espacigo-kyc-identity${local.suffix}"
    kyc_space      = "espacigo-kyc-space${local.suffix}"
    privacy_replay = "espacigo-privacy-replay${local.suffix}"
  }
}

resource "aws_s3_bucket" "buckets" {
  for_each      = local.buckets
  bucket        = each.value
  force_destroy = var.environment == "local"

  tags = {
    Environment = var.environment
    Project     = "espacigo"
    Purpose     = each.key
  }
}

resource "aws_s3_bucket_versioning" "versioning" {
  for_each = aws_s3_bucket.buckets
  bucket   = each.value.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_public_access_block" "public_access_block" {
  for_each = aws_s3_bucket.buckets
  bucket   = each.value.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "encryption" {
  for_each = aws_s3_bucket.buckets
  bucket   = each.value.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = false
  }
}

output "bucket_names" {
  description = "Nombres de los buckets S3 aprovisionados"
  value       = { for k, v in aws_s3_bucket.buckets : k => v.id }
}

output "bucket_arns" {
  description = "ARNs de los buckets S3 aprovisionados"
  value       = { for k, v in aws_s3_bucket.buckets : k => v.arn }
}
