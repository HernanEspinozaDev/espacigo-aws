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

variable "bucket_arns" {
  description = "Lista o mapa de ARNs de buckets S3 permitidos"
  type        = list(string)
}

variable "account_id" {
  description = "ID de cuenta AWS (000000000000 en emulator)"
  type        = string
  default     = "000000000000"
}

variable "aws_region" {
  description = "Región AWS activa"
  type        = string
  default     = "us-east-1"
}

# 1. ECS Task Execution Role (para ECS agent: pulls de ECR y envío de logs a CloudWatch)
resource "aws_iam_role" "ecs_task_execution_role" {
  name = "espacigo-ecs-task-execution-role-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Environment = var.environment
    Project     = "espacigo"
  }
}

resource "aws_iam_role_policy" "ecs_task_execution_logs_policy" {
  name = "espacigo-ecs-task-execution-logs"
  role = aws_iam_role.ecs_task_execution_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${var.account_id}:log-group:/aws/ecs/espacigo-*:*"
      }
    ]
  })
}

# 2. ECS API Task Role (para el contenedor Go API en tiempo de ejecución: acceso a S3 y SSM)
resource "aws_iam_role" "api_task_role" {
  name = "espacigo-api-task-role-${var.environment}"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ecs-tasks.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Environment = var.environment
    Project     = "espacigo"
  }
}

resource "aws_iam_role_policy" "api_task_s3_policy" {
  name = "espacigo-api-s3-access"
  role = aws_iam_role.api_task_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:ListBucket",
          "s3:GetBucketLocation"
        ]
        Resource = var.bucket_arns
      },
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject"
        ]
        Resource = [for arn in var.bucket_arns : "${arn}/*"]
      }
    ]
  })
}

resource "aws_iam_role_policy" "api_task_ssm_policy" {
  name = "espacigo-api-ssm-access"
  role = aws_iam_role.api_task_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ssm:GetParameter",
          "ssm:GetParameters",
          "ssm:GetParametersByPath"
        ]
        Resource = "arn:aws:ssm:${var.aws_region}:${var.account_id}:parameter/espacigo/${var.environment}/*"
      }
    ]
  })
}

output "ecs_task_execution_role_arn" {
  description = "ARN del rol de ejecución de tareas ECS"
  value       = aws_iam_role.ecs_task_execution_role.arn
}

output "api_task_role_arn" {
  description = "ARN del rol de runtime de la API Go"
  value       = aws_iam_role.api_task_role.arn
}
