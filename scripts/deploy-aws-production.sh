#!/usr/bin/env bash
set -euo pipefail
shopt -s inherit_errexit

fail() { echo "$*" >&2; exit 1; }

[[ "${GITHUB_REF:-}" == refs/heads/main ]] || fail "El despliegue requiere main."
[[ "${GITHUB_SHA:-}" =~ ^[0-9a-f]{40}$ && "$(git rev-parse HEAD)" == "$GITHUB_SHA" ]] ||
  fail "El checkout debe corresponder al SHA validado."
: "${AWS_REGION:?}" "${EXPECTED_AWS_ACCOUNT_ID:?}" "${ECS_CLUSTER:?}" "${ECS_SERVICE:?}" "${ECS_MIGRATOR_TASK:?}"
: "${ECS_MIGRATOR_NETWORK:?}" "${ECR_API_REPOSITORY:?}" "${ECR_WEB_REPOSITORY:?}" "${ECR_MIGRATOR_REPOSITORY:?}"
: "${PRODUCTION_HEALTH_URL:?}" "${GITHUB_RUN_ID:?}" "${GITHUB_RUN_ATTEMPT:?}"
[[ "$PRODUCTION_HEALTH_URL" == https://* ]] || fail "La comprobación pública requiere HTTPS."
[[ "$EXPECTED_AWS_ACCOUNT_ID" =~ ^[0-9]{12}$ && "$EXPECTED_AWS_ACCOUNT_ID" != 000000000000 ]] ||
  fail "Se requiere el identificador del proyecto AWS de producción."
[[ -z "${AWS_ENDPOINT_URL:-}${AWS_ENDPOINT_URL_STS:-}${AWS_ENDPOINT_URL_ECR:-}${AWS_ENDPOINT_URL_ECS:-}" ]] ||
  fail "Quita los endpoints personalizados de AWS."
jq -e '.awsvpcConfiguration |
  (.assignPublicIp == "DISABLED") and (.subnets | length > 0) and (.securityGroups | length > 0)'   <<<"$ECS_MIGRATOR_NETWORK" >/dev/null || fail "La tarea de migración requiere una red privada."

export AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true AWS_PAGER='' AWS_DEFAULT_OUTPUT=json

aws_prod() { aws --region "$AWS_REGION" "$@"; }
current_service() {
  aws_prod ecs describe-services --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE" |
    jq -ec 'select(.failures | length == 0) | .services | select(length == 1) | .[0] | select(.status == "ACTIVE")'
}
require_current_main() {
  [[ "$(git ls-remote origin refs/heads/main | cut -f1)" == "$GITHUB_SHA" ]] ||
    fail "Hay otro SHA en main. Ejecuta el workflow para ese commit."
}
task_definition() {
  aws_prod ecs describe-task-definition --task-definition "$1" |
    jq -ec '.taskDefinition | del(.taskDefinitionArn,.revision,.status,.requiresAttributes,.compatibilities,.registeredAt,.registeredBy,.deregisteredAt)'
}
register_definition() {
  aws_prod ecs register-task-definition --cli-input-json "$1" --query taskDefinition.taskDefinitionArn --output text
}
registry_repo() {
  aws_prod ecr describe-repositories --repository-names "$1" |
    jq -er --arg registry "$registry" '
      .repositories[] | select(.imageTagMutability == "IMMUTABLE") |
      .repositoryUri | select(startswith($registry + "/"))'
}
publish() {
  local name="$1" uri="$2" digest
  shift 2
  docker buildx build --platform linux/amd64 --tag "$uri:$tag" --push "$@" >&2
  digest="$(aws_prod ecr describe-images --repository-name "$name" --image-ids "imageTag=$tag" --query 'imageDetails[0].imageDigest' --output text)"
  [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail "ECR no devolvió un digest válido para $name."
  printf '%s@%s\n' "$uri" "$digest"
}
app_healthy() {
  [[ "$(curl --silent --show-error --max-time 12 --proto '=https' --output /dev/null --write-out '%{http_code}' "$PRODUCTION_HEALTH_URL")" == 200 ]]
}

require_current_main
[[ "$(aws_prod sts get-caller-identity --query Account --output text)" == "$EXPECTED_AWS_ACCOUNT_ID" ]] ||
  fail "El rol apunta a otro proyecto AWS."
service="$(current_service)"
jq -e '.deploymentController.type == "ECS" and .desiredCount > 0 and (.loadBalancers | length > 0)' <<<"$service" >/dev/null ||
  fail "El servicio requiere despliegue rolling y un balanceador activo."
previous_task="$(jq -er '.taskDefinition' <<<"$service")"
app_template="$(task_definition "$previous_task")"
migrator_template="$(task_definition "$ECS_MIGRATOR_TASK")"
jq -e '(.containerDefinitions | map(.name) | sort) == ["api","web"]' <<<"$app_template" >/dev/null ||
  fail "La tarea de aplicación requiere los contenedores api y web."
jq -e '(.containerDefinitions | map(.name)) == ["dbmigrate"]' <<<"$migrator_template" >/dev/null ||
  fail "La tarea de verificación requiere el contenedor dbmigrate."
jq -e 'all(.containerDefinitions[]; all(.environment[]?;
  ((.name | startswith("LOCAL_")) | not) and ((.name | startswith("AWS_ENDPOINT_URL")) | not)))'   <<<"$app_template" >/dev/null || fail "La tarea de producción contiene configuración local."
jq -e 'any(.containerDefinitions[]; .name == "api" and
  any(.environment[]?; .name == "APP_PROFILE" and .value == "production"))' <<<"$app_template" >/dev/null ||
  fail "La API requiere un perfil de producción preparado y verificado."

registry="$EXPECTED_AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
[[ "$ECR_API_REPOSITORY" != "$ECR_WEB_REPOSITORY" && "$ECR_API_REPOSITORY" != "$ECR_MIGRATOR_REPOSITORY" &&
   "$ECR_WEB_REPOSITORY" != "$ECR_MIGRATOR_REPOSITORY" ]] || fail "Usa tres repositorios ECR distintos."
api_uri="$(registry_repo "$ECR_API_REPOSITORY")"
web_uri="$(registry_repo "$ECR_WEB_REPOSITORY")"
migrator_uri="$(registry_repo "$ECR_MIGRATOR_REPOSITORY")"
aws_prod ecr get-login-password | docker login --username AWS --password-stdin "$registry"
tag="$GITHUB_SHA-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
api_image="$(publish "$ECR_API_REPOSITORY" "$api_uri" --file Dockerfile --target api .)"
web_image="$(publish "$ECR_WEB_REPOSITORY" "$web_uri" --file frontend/Dockerfile frontend)"
migrator_image="$(publish "$ECR_MIGRATOR_REPOSITORY" "$migrator_uri" --file Dockerfile.dbmigrate .)"

migration_task="$(register_definition "$(jq -c --arg image "$migrator_image"   '.containerDefinitions[0] |= (.image = $image | .entryPoint = ["/app/dbmigrate"] | .command = ["-plan-only","-require-current"])'   <<<"$migrator_template")")"
migration_run="$(aws_prod ecs run-task --cluster "$ECS_CLUSTER" --task-definition "$migration_task"   --launch-type FARGATE --network-configuration "$ECS_MIGRATOR_NETWORK")"
migration_arn="$(jq -er 'select(.failures | length == 0) | .tasks | select(length == 1) | .[0].taskArn' <<<"$migration_run")"
aws_prod ecs wait tasks-stopped --cluster "$ECS_CLUSTER" --tasks "$migration_arn"
aws_prod ecs describe-tasks --cluster "$ECS_CLUSTER" --tasks "$migration_arn" |
  jq -e '.failures == [] and (.tasks | length == 1) and any(.tasks[0].containers[]; .name == "dbmigrate" and .exitCode == 0)' >/dev/null ||
  fail "El esquema requiere revisión o migración. Consulta la tarea $migration_arn."

require_current_main
[[ "$(current_service | jq -er '.taskDefinition')" == "$previous_task" ]] ||
  fail "El servicio cambió durante la preparación. Repite el workflow."
new_task="$(register_definition "$(jq -c --arg api "$api_image" --arg web "$web_image"   '.containerDefinitions |= map(.image = (if .name == "api" then $api else $web end))' <<<"$app_template")")"

update_service() {
  aws_prod ecs update-service --cluster "$ECS_CLUSTER" --service "$ECS_SERVICE" --task-definition "$1"     --deployment-configuration 'deploymentCircuitBreaker={enable=true,rollback=true}' >/dev/null
}

printf 'SHA=%s\nAnterior=%s\nNueva=%s\nAPI=%s\nWeb=%s\nMigrador=%s\n'   "$GITHUB_SHA" "$previous_task" "$new_task" "$api_image" "$web_image" "$migrator_image"
update_service "$new_task"
if aws_prod ecs wait services-stable --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE" &&
   [[ "$(current_service | jq -er '.taskDefinition')" == "$new_task" ]] && app_healthy; then
  echo "Despliegue verificado."
else
  active_task="$(current_service | jq -er '.taskDefinition')"
  if [[ "$active_task" == "$new_task" ]]; then
    update_service "$previous_task"
    aws_prod ecs wait services-stable --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE"
  fi
  fail "El despliegue no superó la comprobación. Revisa ECS. Revisión anterior: $previous_task."
fi
