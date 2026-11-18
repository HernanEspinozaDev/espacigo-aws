resource "aws_security_group" "service_endpoints" {
  name        = "espacigo-service-endpoints-sg-${var.environment}"
  description = "HTTPS privado desde las tareas ECS a los endpoints de AWS"
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "HTTPS desde tareas ECS"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_task.id]
  }
}

resource "aws_vpc_endpoint" "private_services" {
  for_each = toset(concat(
    ["ecr.api", "ecr.dkr", "logs"],
    var.enable_persistent_secretsmanager_endpoint ? ["secretsmanager"] : [],
  ))

  vpc_id              = aws_vpc.main.id
  service_name        = "com.amazonaws.${var.aws_region}.${each.key}"
  vpc_endpoint_type   = "Interface"
  private_dns_enabled = true
  subnet_ids          = [aws_subnet.private[0].id]
  security_group_ids  = [aws_security_group.service_endpoints.id]

  tags = {
    Name = "espacigo-endpoint-${each.key}-${var.environment}"
  }
}
