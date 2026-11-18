data "aws_ssm_parameter" "al2023_arm64" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_security_group" "nat" {
  name        = "espacigo-nat-sg-${var.environment}"
  description = "NAT instance for private ECS outbound SMTP/HTTPS"
  vpc_id      = aws_vpc.main.id
}

resource "aws_vpc_security_group_ingress_rule" "nat_from_ecs" {
  security_group_id            = aws_security_group.nat.id
  referenced_security_group_id = aws_security_group.ecs_task.id
  ip_protocol                  = "-1"
}

resource "aws_vpc_security_group_egress_rule" "nat_outbound_all" {
  security_group_id = aws_security_group.nat.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

resource "aws_instance" "nat" {
  ami                         = data.aws_ssm_parameter.al2023_arm64.value
  instance_type               = "t4g.micro"
  subnet_id                   = aws_subnet.public[0].id
  vpc_security_group_ids      = [aws_security_group.nat.id]
  associate_public_ip_address = true
  source_dest_check           = false

  user_data = <<-EOF
    #!/bin/bash
    set -euo pipefail
    dnf install -y iptables-nft
    sysctl -w net.ipv4.ip_forward=1
    echo 'net.ipv4.ip_forward = 1' > /etc/sysctl.d/99-nat.conf
    iface=$(ip route show default | awk '{print $5; exit}')
    iptables -t nat -C POSTROUTING -o "$iface" -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -o "$iface" -j MASQUERADE
  EOF

  metadata_options {
    http_tokens = "required"
  }

  tags = {
    Name = "espacigo-nat-${var.environment}"
  }
}

resource "aws_route" "private_nat" {
  route_table_id         = aws_route_table.private.id
  destination_cidr_block = "0.0.0.0/0"
  network_interface_id   = aws_instance.nat.primary_network_interface_id
}
