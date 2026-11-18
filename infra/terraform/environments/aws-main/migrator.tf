locals {
  provision_migrator = var.migrator_seed_image != null
  migrator_family    = "espacigo-db-migrate-${var.environment}"
  migrator_ecr_arn   = "arn:aws:ecr:${var.aws_region}:${data.aws_caller_identity.current.account_id}:repository/espacigo-dbmigrate"
}

data "aws_secretsmanager_secret" "migrator" {
  count = local.provision_migrator ? 1 : 0
  name  = "espacigo/${var.environment}/database/migrator"
}

resource "aws_cloudwatch_log_group" "migrator" {
  count             = local.provision_migrator ? 1 : 0
  name              = "/aws/ecs/${local.migrator_family}"
  retention_in_days = 14
}

resource "aws_iam_role" "migrator_execution" {
  count = local.provision_migrator ? 1 : 0
  name  = "${local.migrator_family}-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
      Condition = {
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id }
        ArnLike      = { "aws:SourceArn" = "arn:aws:ecs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:*" }
      }
    }]
  })
}

resource "aws_iam_role_policy" "migrator_execution" {
  count      = local.provision_migrator ? 1 : 0
  depends_on = [aws_vpc_endpoint.private_services, aws_vpc_endpoint.s3]
  name       = "db-migration-execution"
  role       = aws_iam_role.migrator_execution[0].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = "ecr:GetAuthorizationToken", Resource = "*" },
      {
        Effect   = "Allow"
        Action   = ["ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"]
        Resource = local.migrator_ecr_arn
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.migrator[0].arn}:log-stream:migration/dbmigrate/*"
      },
      {
        Effect   = "Allow"
        Action   = "secretsmanager:GetSecretValue"
        Resource = data.aws_secretsmanager_secret.migrator[0].arn
        Condition = {
          StringEquals = { "aws:SourceVpce" = try(aws_vpc_endpoint.private_services["secretsmanager"].id, "") }
        }
      },
    ]
  })
}

resource "aws_ecs_task_definition" "migrator" {
  count                    = local.provision_migrator ? 1 : 0
  family                   = local.migrator_family
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.migrator_execution[0].arn
  cpu                      = 256
  memory                   = 512

  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([{
    name                   = "dbmigrate"
    image                  = var.migrator_seed_image
    essential              = true
    readonlyRootFilesystem = true
    command                = ["-aws-release"]
    environment = [
      { name = "AWS_RDS_HOST", value = aws_db_instance.postgres.address },
      { name = "DATABASE_SET_ROLE", value = "espacigo_schema_owner" },
      {
        name  = "DATABASE_URL"
        value = "postgres://espacigo_migrator_login@${aws_db_instance.postgres.endpoint}/espacigo_prod?sslmode=verify-full&sslrootcert=/etc/ssl/rds-ca.pem"
      },
    ]
    secrets = [{
      name      = "DATABASE_ECS_PASSWORD"
      valueFrom = "${data.aws_secretsmanager_secret.migrator[0].arn}:password::"
    }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.migrator[0].name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "migration"
      }
    }
  }])

  lifecycle {
    precondition {
      condition = can(regex(
        "^${data.aws_caller_identity.current.account_id}\\.dkr\\.ecr\\.${var.aws_region}\\.amazonaws\\.com/espacigo-dbmigrate@sha256:[0-9a-f]{64}$",
        var.migrator_seed_image,
      )) && var.enable_persistent_secretsmanager_endpoint
      error_message = "Use a promoted private ECR digest and enable the persistent Secrets Manager endpoint."
    }
  }

  depends_on = [aws_iam_role_policy.migrator_execution]
}

output "migrator_task_definition_arn" {
  value = try(aws_ecs_task_definition.migrator[0].arn, null)
}

output "migrator_task_network" {
  value = local.provision_migrator ? jsonencode({
    awsvpcConfiguration = {
      subnets        = [aws_subnet.private[0].id]
      securityGroups = [aws_security_group.ecs_task.id]
      assignPublicIp = "DISABLED"
    }
  }) : null
}
