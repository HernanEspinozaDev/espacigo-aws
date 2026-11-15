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

variable "retention_in_days" {
  description = "Retención de logs en días (0 = indefinido para local/dev)"
  type        = number
  default     = 7
}

locals {
  log_groups = [
    "/aws/ecs/espacigo-api",
    "/aws/ecs/espacigo-web",
    "/aws/ecs/espacigo-privacy-worker",
    "/aws/ecs/espacigo-dbmigrate"
  ]
}

resource "aws_cloudwatch_log_group" "groups" {
  for_each          = toset(local.log_groups)
  name              = each.value
  retention_in_days = var.retention_in_days

  tags = {
    Environment = var.environment
    Project     = "espacigo"
  }
}

output "log_group_names" {
  description = "Nombres de los CloudWatch Log Groups creados"
  value       = [for k, v in aws_cloudwatch_log_group.groups : v.name]
}

output "log_group_arns" {
  description = "ARNs de los CloudWatch Log Groups creados"
  value       = { for k, v in aws_cloudwatch_log_group.groups : k => v.arn }
}
