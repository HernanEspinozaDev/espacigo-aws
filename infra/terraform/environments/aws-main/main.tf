terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "espacigo"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

# 1. Descubrimiento dinámico de zonas de disponibilidad (Cero hardcoding para migración Ohio -> Chile)
data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  selected_azs = slice(data.aws_availability_zones.available.names, 0, 2)
}

# 2. Red VPC (2 Subnets públicas, 2 Subnets privadas aisladas, IGW y S3 Gateway Endpoint)
resource "aws_vpc" "main" {
  cidr_block           = "10.20.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "espacigo-vpc-${var.environment}"
  }
}

resource "aws_internet_gateway" "gw" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "espacigo-igw-${var.environment}"
  }
}

resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.main.id
  cidr_block              = "10.20.${count.index}.0/24"
  availability_zone       = local.selected_azs[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name = "espacigo-public-subnet-${count.index + 1}-${var.environment}"
  }
}

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.20.${count.index + 10}.0/24"
  availability_zone = local.selected_azs[count.index]

  tags = {
    Name = "espacigo-private-subnet-${count.index + 1}-${var.environment}"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.gw.id
  }

  tags = {
    Name = "espacigo-public-rt-${var.environment}"
  }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "espacigo-private-rt-${var.environment}"
  }
}

resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# Gateway VPC Endpoint gratuito para S3 (sin cargos de NAT Gateway)
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.main.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.public.id, aws_route_table.private.id]

  tags = {
    Name = "espacigo-s3-endpoint-${var.environment}"
  }
}

# 3. Security Groups
resource "aws_security_group" "ecs_task" {
  name        = "espacigo-ecs-task-sg-${var.environment}"
  description = "Reglas de trafico para la tarea ECS Fargate (Next.js + Go API)"
  vpc_id      = aws_vpc.main.id

  ingress {
    description = "Ingreso HTTP desde Cloudflare al frontend Next.js"
    from_port   = 4000
    to_port     = 4000
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "Salida general para HTTPS y SMTP Zoho 587"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "espacigo-ecs-task-sg-${var.environment}"
  }
}

resource "aws_security_group" "rds" {
  name        = "espacigo-rds-sg-${var.environment}"
  description = "Permitir PostgreSQL 5432 solo desde la tarea ECS"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "PostgreSQL desde tarea ECS"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_task.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "espacigo-rds-sg-${var.environment}"
  }
}

# 4. Amazon RDS PostgreSQL 18 (Single-AZ, db.t4g.micro, gp3 20GB, cifrado AES256)
resource "aws_db_subnet_group" "rds" {
  name       = "espacigo-rds-subnet-group-${var.environment}"
  subnet_ids = aws_subnet.private[*].id

  tags = {
    Name = "espacigo-rds-subnet-group-${var.environment}"
  }
}

resource "aws_db_instance" "postgres" {
  identifier                  = "espacigo-db-${var.environment}"
  engine                      = "postgres"
  engine_version              = "18.1"
  instance_class              = var.rds_instance_class
  allocated_storage           = 20
  max_allocated_storage       = 50
  storage_type                = "gp3"
  multi_az                    = var.rds_multi_az
  db_subnet_group_name        = aws_db_subnet_group.rds.name
  vpc_security_group_ids      = [aws_security_group.rds.id]
  publicly_accessible         = false
  storage_encrypted           = true
  auto_minor_version_upgrade  = true
  allow_major_version_upgrade = false
  backup_retention_period     = 7
  backup_window               = "06:00-07:00"
  maintenance_window          = "Sun:07:00-Sun:08:00"
  copy_tags_to_snapshot       = true
  deletion_protection         = true
  skip_final_snapshot         = false
  final_snapshot_identifier   = "espacigo-final-snapshot-${var.environment}"

  db_name  = "espacigo_${var.environment}"
  username = "espacigo_admin"
  password = "change-me-via-secretsmanager-or-cli"

  lifecycle {
    ignore_changes = [password]
  }

  tags = {
    Name = "espacigo-rds-${var.environment}"
  }
}

# 5. Módulo S3 Storage (5 buckets con SSE-S3 AES-256)
module "s3_storage" {
  source        = "../../modules/s3_storage"
  environment   = var.environment
  bucket_suffix = var.environment
}

# 6. Módulo Observabilidad (CloudWatch Logs)
module "observability" {
  source            = "../../modules/observability"
  environment       = var.environment
  retention_in_days = 14
}

# 7. Módulo IAM (Least-Privilege Roles para ECS Task y Execution)
data "aws_caller_identity" "current" {}

module "iam" {
  source      = "../../modules/iam"
  environment = var.environment
  account_id  = data.aws_caller_identity.current.account_id
  aws_region  = var.aws_region
  bucket_arns = values(module.s3_storage.bucket_arns)
}

# 8. Módulo SSM Parameters
module "ssm_parameters" {
  source                 = "../../modules/ssm_parameters"
  environment            = var.environment
  database_host          = aws_db_instance.postgres.address
  database_port          = tostring(aws_db_instance.postgres.port)
  database_name          = aws_db_instance.postgres.db_name
  database_user          = "espacigo_api_login"
  database_role          = "espacigo_runtime"
  cors_allowed_origins   = var.cors_allowed_origins
  frontend_public_origin = var.frontend_public_origin
  bucket_names           = module.s3_storage.bucket_names
}

# 9. AWS Budgets Alert ($28.00 USD/mes < $30.00 meta)
resource "aws_budgets_budget" "cost_limit" {
  name              = "espacigo-monthly-budget-${var.environment}"
  budget_type       = "COST"
  limit_amount      = var.monthly_budget_limit_usd
  limit_unit        = "USD"
  time_unit         = "MONTHLY"
  time_period_start = "2026-11-01_00:00"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_alert_email]
  }
}
