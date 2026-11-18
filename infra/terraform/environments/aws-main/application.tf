locals {
  provision_app = var.app_seed_images != null
  app_family    = "espacigo-app-${var.environment}"
  ecr_registry  = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.aws_region}.amazonaws.com"
  ecs_trust = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Condition = {
        StringEquals = { "aws:SourceAccount" = data.aws_caller_identity.current.account_id }
        ArnLike      = { "aws:SourceArn" = "${local.ecs_scope}:*" }
      }
    }]
  })
}

data "aws_secretsmanager_secret" "api" {
  count = local.provision_app ? 1 : 0
  name  = "espacigo/${var.environment}/database/api"
}

resource "aws_cloudwatch_log_group" "app" {
  count             = local.provision_app ? 1 : 0
  name              = "/aws/ecs/${local.app_family}"
  retention_in_days = 14
}

resource "aws_iam_role" "app_execution" {
  count              = local.provision_app ? 1 : 0
  name               = "${local.app_family}-execution"
  assume_role_policy = local.ecs_trust
}

resource "aws_iam_role_policy" "app_execution" {
  count      = local.provision_app ? 1 : 0
  depends_on = [aws_vpc_endpoint.private_services, aws_vpc_endpoint.s3]
  name       = "private-app-execution"
  role       = aws_iam_role.app_execution[0].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = "ecr:GetAuthorizationToken", Resource = "*" },
      {
        Effect = "Allow"
        Action = ["ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"]
        Resource = [for repo in ["espacigo-api", "espacigo-web"] :
        "arn:aws:ecr:${var.aws_region}:${data.aws_caller_identity.current.account_id}:repository/${repo}"]
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.app[0].arn}:log-stream:application/*"
      },
      {
        Effect   = "Allow"
        Action   = "secretsmanager:GetSecretValue"
        Resource = data.aws_secretsmanager_secret.api[0].arn
        Condition = {
          StringEquals = { "aws:SourceVpce" = try(aws_vpc_endpoint.private_services["secretsmanager"].id, "") }
        }
      }
    ]
  })
}

resource "aws_iam_role" "app_runtime" {
  count              = local.provision_app ? 1 : 0
  name               = "${local.app_family}-runtime"
  assume_role_policy = local.ecs_trust
}

resource "aws_iam_role_policy" "app_runtime" {
  count = local.provision_app ? 1 : 0
  name  = "read-public-catalog-assets"
  role  = aws_iam_role.app_runtime[0].id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "s3:GetObject"
      Resource = [for key in ["evidence", "media"] : "${module.s3_storage.bucket_arns[key]}/*"]
    }]
  })
}

resource "aws_ecs_task_definition" "app" {
  count                    = local.provision_app ? 1 : 0
  family                   = local.app_family
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  execution_role_arn       = aws_iam_role.app_execution[0].arn
  task_role_arn            = aws_iam_role.app_runtime[0].arn
  cpu                      = 512
  memory                   = 1024

  runtime_platform {
    cpu_architecture        = "X86_64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name                   = "api"
      image                  = var.app_seed_images.api
      essential              = true
      readonlyRootFilesystem = true
      environment = [for name, value in {
        APP_PROFILE            = "production"
        HTTP_ADDR              = "127.0.0.1:8080"
        AWS_REGION             = var.aws_region
        DATABASE_HOST          = aws_db_instance.postgres.address
        DATABASE_PORT          = tostring(aws_db_instance.postgres.port)
        DATABASE_NAME          = aws_db_instance.postgres.db_name
        DATABASE_USER          = "espacigo_api_login"
        DATABASE_SET_ROLE      = "espacigo_runtime"
        DATABASE_SSLMODE       = "verify-full"
        DATABASE_SSLROOTCERT   = "/etc/ssl/rds-ca.pem"
        FRONTEND_PUBLIC_ORIGIN = var.frontend_public_origin
        CORS_ALLOWED_ORIGINS   = var.cors_allowed_origins
        BLOB_STORE_BACKEND     = "s3"
        S3_STORAGE_BUCKET      = module.s3_storage.bucket_names.evidence
        S3_MEDIA_BUCKET        = module.s3_storage.bucket_names.media
      } : { name = name, value = value }]
      secrets = [
        { name = "DATABASE_ECS_PASSWORD", valueFrom = "${data.aws_secretsmanager_secret.api[0].arn}:password::" },
        { name = "PUBLIC_CATALOG_ECS_CURSOR_KEY", valueFrom = "${data.aws_secretsmanager_secret.api[0].arn}:cursor_key::" },
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.app[0].name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "application"
        }
      }
    },
    {
      name      = "web"
      image     = var.app_seed_images.web
      essential = true
      dependsOn = [{ containerName = "api", condition = "START" }]
      environment = [
        { name = "API_BACKEND_BASE_URL", value = "http://127.0.0.1:8080/api/v1" },
        { name = "FRONTEND_PUBLIC_ORIGIN", value = var.frontend_public_origin },
        { name = "HOSTNAME", value = "0.0.0.0" },
      ]
      portMappings = [{ containerPort = 4000, hostPort = 4000, protocol = "tcp" }]
      healthCheck = {
        command     = ["CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:4000/api/ready"]
        interval    = 15
        timeout     = 5
        retries     = 3
        startPeriod = 40
      }
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.app[0].name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "application"
        }
      }
    }
  ])

  lifecycle {
    precondition {
      condition = var.enable_persistent_secretsmanager_endpoint && alltrue([
        for repo, image in var.app_seed_images :
        startswith(image, "${local.ecr_registry}/espacigo-${repo}@sha256:") &&
        can(regex("@sha256:[0-9a-f]{64}$", image))
      ])
      error_message = "Use promoted private ECR digests and the private Secrets Manager endpoint."
    }
  }

  depends_on = [aws_iam_role_policy.app_execution, aws_iam_role_policy.app_runtime]
}

resource "aws_ecs_service" "app" {
  count                              = local.provision_app && var.public_tls_certificate_arn != null ? 1 : 0
  name                               = local.app_family
  cluster                            = data.aws_ecs_cluster.production[0].arn
  task_definition                    = aws_ecs_task_definition.app[0].arn
  launch_type                        = "FARGATE"
  platform_version                   = "1.4.0"
  desired_count                      = 1
  health_check_grace_period_seconds  = 60
  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  network_configuration {
    subnets         = [aws_subnet.private[0].id]
    security_groups = [aws_security_group.ecs_task.id]
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.web[0].arn
    container_name   = "web"
    container_port   = 4000
  }

  depends_on = [aws_lb_listener.web_tls]

  lifecycle {
    ignore_changes = [task_definition] # CodePipeline owns subsequent application revisions.
  }
}

output "app_task_definition_arn" {
  value = try(aws_ecs_task_definition.app[0].arn, null)
}

output "public_nlb_dns" {
  value = try(aws_lb.public[0].dns_name, null)
}
