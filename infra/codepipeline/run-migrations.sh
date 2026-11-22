#!/usr/bin/env bash
set -euo pipefail
export AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true

: "${EXPECTED_SOURCE_SHA:?}" "${EXPECTED_AWS_ACCOUNT_ID:?}" "${EXPECTED_AWS_REGION:?}"
: "${CODEBUILD_SRC_DIR_ReleaseManifest:?}" "${ECS_CLUSTER:?}" "${ECS_MIGRATOR_TASK:?}" "${ECS_MIGRATOR_NETWORK:?}"
[[ "$EXPECTED_SOURCE_SHA" =~ ^[0-9a-f]{40}$ && "$CODEBUILD_RESOLVED_SOURCE_VERSION" == "$EXPECTED_SOURCE_SHA" ]]
[[ "$AWS_DEFAULT_REGION" == "$EXPECTED_AWS_REGION" ]]
export AWS_REGION="$EXPECTED_AWS_REGION"
[[ "$(aws sts get-caller-identity --query Account --output text)" == "$EXPECTED_AWS_ACCOUNT_ID" ]]
jq -e '.awsvpcConfiguration |
  .assignPublicIp == "DISABLED" and (.subnets | length == 1) and (.securityGroups | length > 0)
' <<<"$ECS_MIGRATOR_NETWORK" >/dev/null

migrator_image="$(jq -er --arg sha "$EXPECTED_SOURCE_SHA" '
  select(.sha == $sha) | .images.dbmigrate
' "$CODEBUILD_SRC_DIR_ReleaseManifest/release.json")"
prefix="$EXPECTED_AWS_ACCOUNT_ID.dkr.ecr.$EXPECTED_AWS_REGION.amazonaws.com/espacigo-dbmigrate@sha256:"
[[ "$migrator_image" == "$prefix"* && "${migrator_image#"$prefix"}" =~ ^[0-9a-f]{64}$ ]]

template="$(aws ecs describe-task-definition --task-definition "$ECS_MIGRATOR_TASK" --query taskDefinition)"
definition="$(jq -ec --arg image "$migrator_image" '
  select(.networkMode == "awsvpc" and (.requiresCompatibilities | index("FARGATE") != null))
  | select(.taskRoleArn == null and (.executionRoleArn | type == "string"))
  | select((.containerDefinitions | length) == 1 and .containerDefinitions[0].name == "dbmigrate"
      and .containerDefinitions[0].essential and .containerDefinitions[0].readonlyRootFilesystem)
  | {
    family, taskRoleArn, executionRoleArn, networkMode, containerDefinitions, volumes,
    placementConstraints, requiresCompatibilities, cpu, memory, runtimePlatform, ephemeralStorage, pidMode
  }
  | with_entries(select(.value != null))
  | .containerDefinitions |= map(
      if .name == "dbmigrate" then
        .image = $image | .entryPoint = ["/app/dbmigrate"] | .command = ["-aws-release"]
      else . end)
' <<<"$template")"

task_definition="$(aws ecs register-task-definition --cli-input-json "$definition" \
  --query taskDefinition.taskDefinitionArn --output text)"
task="$(aws ecs run-task --cluster "$ECS_CLUSTER" --task-definition "$task_definition" \
  --launch-type FARGATE --platform-version 1.4.0 --network-configuration "$ECS_MIGRATOR_NETWORK" |
  jq -er 'select(.failures == [] and (.tasks | length == 1)) | .tasks[0].taskArn')"
printf 'Tarea de migración %s\n' "$task"
aws ecs wait tasks-stopped --cluster "$ECS_CLUSTER" --tasks "$task"
result="$(aws ecs describe-tasks --cluster "$ECS_CLUSTER" --tasks "$task")"
if ! jq -e '.failures == [] and (.tasks | length == 1) and
           any(.tasks[0].containers[]; .name == "dbmigrate" and .exitCode == 0)' <<<"$result" >/dev/null; then
  jq '{failures, tasks: [.tasks[] | {stopCode, stoppedReason, containers: [.containers[] | {name, exitCode, reason}]}]}' \
    <<<"$result" >&2
  log_group="$(jq -r '.containerDefinitions[] | select(.name == "dbmigrate") | .logConfiguration.options["awslogs-group"] // empty' <<<"$template" 2>/dev/null)" || true
  log_prefix="$(jq -r '.containerDefinitions[] | select(.name == "dbmigrate") | .logConfiguration.options["awslogs-stream-prefix"] // empty' <<<"$template" 2>/dev/null)" || true
  task_id="${task##*/}"
  if [[ -n "${log_group:-}" && -n "${log_prefix:-}" && -n "${task_id:-}" ]]; then
    printf 'Logs de dbmigrate (%s / %s):\n' "$log_group" "${log_prefix}/dbmigrate/${task_id}" >&2 || true
    aws logs get-log-events \
      --log-group-name "$log_group" \
      --log-stream-name "${log_prefix}/dbmigrate/${task_id}" \
      --limit 80 | jq -r '.events[].message' >&2 || true
  fi
  exit 1
fi
