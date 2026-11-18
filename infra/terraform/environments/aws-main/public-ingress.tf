resource "aws_security_group" "load_balancer" {
  name        = "espacigo-nlb-sg-${var.environment}"
  description = "Security group del balanceador TLS de EspaciGo"
  vpc_id      = aws_vpc.main.id
}

resource "aws_vpc_security_group_ingress_rule" "public_tls" {
  count             = var.public_tls_certificate_arn == null ? 0 : 1
  security_group_id = aws_security_group.load_balancer.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
}

resource "aws_vpc_security_group_egress_rule" "load_balancer_to_web" {
  security_group_id            = aws_security_group.load_balancer.id
  referenced_security_group_id = aws_security_group.ecs_task.id
  description                  = "Trafico y health check de Next.js"
  ip_protocol                  = "tcp"
  from_port                    = 4000
  to_port                      = 4000
}

resource "aws_lb" "public" {
  count                            = var.public_tls_certificate_arn == null ? 0 : 1
  name                             = "espacigo-${var.environment}"
  load_balancer_type               = "network"
  internal                         = false
  subnets                          = [aws_subnet.public[0].id]
  security_groups                  = [aws_security_group.load_balancer.id]
  enable_cross_zone_load_balancing = false
}

resource "aws_lb_target_group" "web" {
  count       = length(aws_lb.public)
  name        = "espacigo-web-${var.environment}"
  port        = 4000
  protocol    = "TCP"
  target_type = "ip"
  vpc_id      = aws_vpc.main.id

  health_check {
    protocol = "HTTP"
    path     = "/api/ready"
    matcher  = "200"
  }
}

resource "aws_lb_listener" "web_tls" {
  count             = length(aws_lb.public)
  load_balancer_arn = aws_lb.public[0].arn
  port              = 443
  protocol          = "TLS"
  certificate_arn   = var.public_tls_certificate_arn
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web[0].arn
  }
}
