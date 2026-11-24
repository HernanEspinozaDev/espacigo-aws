#!/usr/bin/env bash
set -euo pipefail

# Controla el encendido/apagado de recursos de produccion en AWS para ahorrar
# costos durante periodos sin usuarios o sin pruebas activas.
#
# Uso:
#   ./scripts/prod-power.sh status
#   ./scripts/prod-power.sh pause
#   ./scripts/prod-power.sh start

PROFILE="${ESPACIGO_PROD_PROFILE:-espacigo-prod}"
REGION="${ESPACIGO_PROD_REGION:-us-east-2}"
ECS_CLUSTER="espacigo-production"
ECS_SERVICE="espacigo-app-prod"
RDS_INSTANCE="espacigo-db-prod"
NAT_INSTANCE_NAME="espacigo-nat-prod"

# Aislamiento respecto al aws-emulator local.
unset AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3 AWS_ENDPOINT_URL_SSM AWS_CA_BUNDLE

aws_prod() {
  aws --profile "$PROFILE" --region "$REGION" "$@"
}

get_nat_id() {
  aws_prod ec2 describe-instances \
    --filters "Name=tag:Name,Values=${NAT_INSTANCE_NAME}" "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[0].Instances[0].InstanceId' \
    --output text
}

get_nat_state() {
  local nat_id="$1"
  aws_prod ec2 describe-instances \
    --instance-ids "$nat_id" \
    --query 'Reservations[0].Instances[0].State.Name' \
    --output text
}

get_rds_state() {
  aws_prod rds describe-db-instances \
    --db-instance-identifier "$RDS_INSTANCE" \
    --query 'DBInstances[0].DBInstanceStatus' \
    --output text
}

get_ecs_counts() {
  aws_prod ecs describe-services \
    --cluster "$ECS_CLUSTER" \
    --services "$ECS_SERVICE" \
    --query 'services[0].[desiredCount,runningCount,status]' \
    --output text
}

cmd_status() {
  local nat_id nat_state rds_state ecs_info desired running ecs_status
  nat_id="$(get_nat_id)"
  nat_state="$(get_nat_state "$nat_id")"
  rds_state="$(get_rds_state)"
  ecs_info="$(get_ecs_counts)"
  read -r desired running ecs_status <<< "$ecs_info"

  echo "=== Estado de produccion (${PROFILE} / ${REGION}) ==="
  echo "  NAT (${nat_id}): ${nat_state}"
  echo "  RDS (${RDS_INSTANCE}): ${rds_state}"
  echo "  ECS (${ECS_SERVICE}): desired=${desired}, running=${running}, status=${ecs_status}"
}

cmd_pause() {
  local nat_id nat_state rds_state
  nat_id="$(get_nat_id)"

  echo "[1/3] Bajando ECS ${ECS_SERVICE} a desiredCount=0..."
  aws_prod ecs update-service \
    --cluster "$ECS_CLUSTER" \
    --service "$ECS_SERVICE" \
    --desired-count 0 \
    --no-cli-pager >/dev/null

  echo "[2/3] Verificando RDS ${RDS_INSTANCE}..."
  rds_state="$(get_rds_state)"
  if [[ "$rds_state" == "available" ]]; then
    echo "      Deteniendo RDS ${RDS_INSTANCE} (AWS lo reinicia automaticamente tras 7 dias)..."
    aws_prod rds stop-db-instance --db-instance-identifier "$RDS_INSTANCE" --no-cli-pager >/dev/null
  else
    echo "      RDS ya esta en estado '${rds_state}', omitiendo."
  fi

  echo "[3/3] Verificando NAT ${nat_id}..."
  nat_state="$(get_nat_state "$nat_id")"
  if [[ "$nat_state" == "running" ]]; then
    echo "      Deteniendo instancia NAT ${nat_id}..."
    aws_prod ec2 stop-instances --instance-ids "$nat_id" --no-cli-pager >/dev/null
  else
    echo "      NAT ya esta en estado '${nat_state}', omitiendo."
  fi

  echo "Pausa solicitada. Ejecuta './scripts/prod-power.sh status' para confirmar que todo quedo detenido."
}

cmd_start() {
  local nat_id nat_state rds_state
  nat_id="$(get_nat_id)"

  echo "[1/3] Verificando NAT ${nat_id}..."
  nat_state="$(get_nat_state "$nat_id")"
  if [[ "$nat_state" == "stopped" ]]; then
    echo "      Iniciando instancia NAT ${nat_id}..."
    aws_prod ec2 start-instances --instance-ids "$nat_id" --no-cli-pager >/dev/null
  fi
  echo "      Esperando a que NAT este running..."
  aws_prod ec2 wait instance-running --instance-ids "$nat_id"

  echo "[2/3] Verificando RDS ${RDS_INSTANCE}..."
  rds_state="$(get_rds_state)"
  if [[ "$rds_state" == "stopped" ]]; then
    echo "      Iniciando RDS ${RDS_INSTANCE}..."
    aws_prod rds start-db-instance --db-instance-identifier "$RDS_INSTANCE" --no-cli-pager >/dev/null
  fi
  echo "      Esperando a que RDS este available (puede tardar 2-4 min)..."
  aws_prod rds wait db-instance-available --db-instance-identifier "$RDS_INSTANCE"

  echo "[3/3] Levantando ECS ${ECS_SERVICE} a desiredCount=1..."
  aws_prod ecs update-service \
    --cluster "$ECS_CLUSTER" \
    --service "$ECS_SERVICE" \
    --desired-count 1 \
    --no-cli-pager >/dev/null
  echo "      Esperando a que el servicio ECS este estable..."
  aws_prod ecs wait services-stable --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE"

  echo "Produccion activa."
  cmd_status
}

case "${1:-}" in
  status) cmd_status ;;
  pause|stop) cmd_pause ;;
  start|resume) cmd_start ;;
  *)
    echo "Uso: $0 {status|pause|start}" >&2
    exit 1
    ;;
esac
