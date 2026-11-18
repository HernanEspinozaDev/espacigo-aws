locals {
  migrate_project = "espacigo-main-migrate"
  pilot_artifacts = "espacigo-pilot-artifacts-${data.aws_caller_identity.current.account_id}-${var.aws_region}"
  ecs_scope       = "arn:aws:ecs:${var.aws_region}:${data.aws_caller_identity.current.account_id}"
}

data "aws_ecs_cluster" "production" {
  count        = local.provision_migrator ? 1 : 0
  cluster_name = "espacigo-production"
}

resource "aws_cloudwatch_log_group" "migrate_build" {
  count             = local.provision_migrator ? 1 : 0
  name              = "/aws/codebuild/${local.migrate_project}"
  retention_in_days = 14
}

resource "aws_iam_role" "migrate_build" {
  count = local.provision_migrator ? 1 : 0
  name  = "${local.migrate_project}-service"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "codebuild.amazonaws.com" }
      Condition = {
        StringEquals = {
          "aws:SourceAccount" = data.aws_caller_identity.current.account_id
          "aws:SourceArn"     = "arn:aws:codebuild:${var.aws_region}:${data.aws_caller_identity.current.account_id}:project/${local.migrate_project}"
        }
      }
    }]
  })
}

resource "aws_iam_role_policy" "migrate_build" {
  count = local.provision_migrator ? 1 : 0
  role  = aws_iam_role.migrate_build[0].id
  name  = "run-private-migration"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["s3:GetBucketLocation", "s3:GetBucketVersioning"]
        Resource = "arn:aws:s3:::${local.pilot_artifacts}"
      },
      {
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:GetObjectVersion"]
        Resource = "arn:aws:s3:::${local.pilot_artifacts}/espacigo-main-pilot/*"
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "${aws_cloudwatch_log_group.migrate_build[0].arn}:log-stream:*"
      },
      {
        Effect   = "Allow"
        Action   = ["ecs:DescribeTaskDefinition", "ecs:RegisterTaskDefinition"]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = "ecs:RunTask"
        Resource = "${local.ecs_scope}:task-definition/${local.migrator_family}:*"
        Condition = {
          ArnEquals = { "ecs:cluster" = data.aws_ecs_cluster.production[0].arn }
        }
      },
      {
        Effect   = "Allow"
        Action   = "ecs:DescribeTasks"
        Resource = "${local.ecs_scope}:task/${data.aws_ecs_cluster.production[0].cluster_name}/*"
      },
      {
        Effect   = "Allow"
        Action   = "iam:PassRole"
        Resource = aws_iam_role.migrator_execution[0].arn
        Condition = {
          StringEquals = { "iam:PassedToService" = "ecs-tasks.amazonaws.com" }
        }
      },
    ]
  })
}

resource "aws_codebuild_project" "migrate" {
  count         = local.provision_migrator ? 1 : 0
  name          = local.migrate_project
  service_role  = aws_iam_role.migrate_build[0].arn
  build_timeout = 20

  source {
    type      = "CODEPIPELINE"
    buildspec = "infra/codepipeline/migrate-buildspec.yml"
  }

  artifacts {
    type = "CODEPIPELINE"
  }

  environment {
    type            = "LINUX_CONTAINER"
    compute_type    = "BUILD_GENERAL1_SMALL"
    image           = "aws/codebuild/standard:7.0"
    privileged_mode = false

    dynamic "environment_variable" {
      for_each = {
        EXPECTED_AWS_ACCOUNT_ID = data.aws_caller_identity.current.account_id
        EXPECTED_AWS_REGION     = var.aws_region
        ECS_CLUSTER             = data.aws_ecs_cluster.production[0].arn
        ECS_MIGRATOR_TASK       = aws_ecs_task_definition.migrator[0].arn
        ECS_MIGRATOR_NETWORK = jsonencode({
          awsvpcConfiguration = {
            subnets        = [aws_subnet.private[0].id]
            securityGroups = [aws_security_group.ecs_task.id]
            assignPublicIp = "DISABLED"
          }
        })
      }

      content {
        name  = environment_variable.key
        value = environment_variable.value
      }
    }
  }

  logs_config {
    cloudwatch_logs {
      status     = "ENABLED"
      group_name = aws_cloudwatch_log_group.migrate_build[0].name
    }
  }

  depends_on = [aws_iam_role_policy.migrate_build]
}

output "migrate_codebuild_project_arn" {
  value = try(aws_codebuild_project.migrate[0].arn, null)
}

resource "aws_iam_role_policy" "pipeline_migrate" {
  count = local.provision_migrator ? 1 : 0
  name  = "run-espacigo-migration-build"
  role  = "espacigo-pilot-codepipeline"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["codebuild:StartBuild", "codebuild:BatchGetBuilds"]
        Resource = aws_codebuild_project.migrate[0].arn
      },
    ]
  })
}
