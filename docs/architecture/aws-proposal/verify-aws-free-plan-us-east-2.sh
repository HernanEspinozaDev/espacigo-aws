#!/usr/bin/env bash
# ==============================================================================
# Script de Verificación Read-Only para AWS Free Plan en us-east-2 (Ohio)
# Proyecto: EspaciGo (Cuenta AWS 805068224315)
# Seguridad: 100% SOLO LECTURA (Describe / List / Get). No crea ni modifica nada.
# ==============================================================================
set -euo pipefail

export PATH="$PATH:/usr/local/bin:$HOME/.local/bin"
PROFILE="${AWS_PROFILE:-aws-dev}"
REGION="${AWS_REGION:-us-east-2}"

echo "=================================================================="
echo " 1. Identidad de Cuenta AWS y Región Activa (Perfil: ${PROFILE}, Región: ${REGION})"
echo "=================================================================="
aws sts get-caller-identity --profile "${PROFILE}" --region "${REGION}" --output table

echo ""
echo "=================================================================="
echo ' 2. Estado del AWS Free Plan, Saldo de Créditos y Actividades ($20 c/u)'
echo "=================================================================="
aws freetier get-account-plan-state --profile "${PROFILE}" --region us-east-1 --output table
aws freetier list-account-activities --profile "${PROFILE}" --region us-east-1 \
  --query 'activities[].{Title:title,Status:status,RewardUSD:reward.credit.amount}' \
  --output table

echo ""
echo "=================================================================="
echo " 3. Zonas de Disponibilidad en ${REGION}"
echo "=================================================================="
aws ec2 describe-availability-zones --profile "${PROFILE}" --region "${REGION}" \
  --query 'AvailabilityZones[].{ZoneName:ZoneName,ZoneId:ZoneId,State:State}' \
  --output table

echo ""
echo "=================================================================="
echo " 4. Versiones de PostgreSQL 18 disponibles en RDS (${REGION})"
echo "=================================================================="
aws rds describe-db-engine-versions \
  --profile "${PROFILE}" \
  --region "${REGION}" \
  --engine postgres \
  --default-only \
  --query 'DBEngineVersions[].{DefaultVersion:EngineVersion,Family:DBParameterGroupFamily}' \
  --output table

aws rds describe-db-engine-versions \
  --profile "${PROFILE}" \
  --region "${REGION}" \
  --engine postgres \
  --query 'DBEngineVersions[?starts_with(EngineVersion, `18.`)].{Version:EngineVersion,Family:DBParameterGroupFamily}' \
  --output table

echo ""
echo "=================================================================="
echo " 5. Disponibilidad de db.t4g.micro y db.t4g.small con gp3 en PG 18.6 (${REGION})"
echo "=================================================================="
aws rds describe-orderable-db-instance-options \
  --profile "${PROFILE}" \
  --region "${REGION}" \
  --engine postgres \
  --engine-version 18.6 \
  --db-instance-class db.t4g.micro \
  --query 'OrderableDBInstanceOptions[?StorageType==`gp3`].{Version:EngineVersion,Class:DBInstanceClass,Storage:StorageType,MultiAZCapable:MultiAZCapable}' \
  --output table

aws rds describe-orderable-db-instance-options \
  --profile "${PROFILE}" \
  --region "${REGION}" \
  --engine postgres \
  --engine-version 18.6 \
  --db-instance-class db.t4g.small \
  --query 'OrderableDBInstanceOptions[?StorageType==`gp3`].{Version:EngineVersion,Class:DBInstanceClass,Storage:StorageType,MultiAZCapable:MultiAZCapable}' \
  --output table

echo ""
echo "=================================================================="
echo " 6. Cuotas de AWS Fargate y Estado de ECS / ECR / SSM en ${REGION}"
echo "=================================================================="
aws service-quotas list-service-quotas \
  --profile "${PROFILE}" \
  --region "${REGION}" \
  --service-code fargate \
  --query 'Quotas[].{QuotaName:QuotaName,Value:Value}' \
  --output table

echo -n "- ECS Clusters: "
aws ecs list-clusters --profile "${PROFILE}" --region "${REGION}" --query 'clusterArns' --output text
echo -n "- ECR Repositorios: "
aws ecr describe-repositories --profile "${PROFILE}" --region "${REGION}" --query 'repositories[].repositoryName' --output text
echo -n "- SSM Parameters: "
aws ssm describe-parameters --profile "${PROFILE}" --region "${REGION}" --max-results 5 --query 'Parameters[].Name' --output text

echo ""
echo "=================================================================="
echo " 7. Presupuestos (AWS Budgets)"
echo "=================================================================="
ACCOUNT_ID=$(aws sts get-caller-identity --profile "${PROFILE}" --query Account --output text)
aws budgets describe-budgets --profile "${PROFILE}" --account-id "${ACCOUNT_ID}" \
  --query 'Budgets[].{Name:BudgetName,Limit:BudgetLimit.Amount,Unit:BudgetLimit.Unit}' \
  --output table
