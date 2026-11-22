#!/usr/bin/env bash
set -euo pipefail

export AWS_PAGER=""

AWS_PROFILE="${AWS_PROFILE:-espacigo-prod}"
AWS_REGION="${AWS_REGION:-us-east-2}"
PIPELINE_NAME="${PIPELINE_NAME:-espacigo-main-pilot}"
ECS_CLUSTER="${ECS_CLUSTER:-espacigo-production}"
ECS_SERVICE="${ECS_SERVICE:-espacigo-app-prod}"
PRODUCTION_HEALTH_URL="${PRODUCTION_HEALTH_URL:-}"
POLL_INTERVAL="${POLL_INTERVAL:-10}"

usage() {
  cat <<'EOF'
Uso: watch-deploy.sh [opciones]

Monitorea en vivo un despliegue de EspaciGo en AWS (CodePipeline, ECS y /api/ready).

Opciones:
  --profile <perfil>      Perfil de AWS CLI (default: AWS_PROFILE o espacigo-prod)
  --region <region>       Región de AWS (default: AWS_REGION o us-east-2)
  --pipeline <nombre>     Nombre de CodePipeline (default: PIPELINE_NAME o espacigo-main-pilot)
  --health-url <url>      URL base de producción para consultar <url>/api/ready (opcional)
  --cluster <cluster>     Cluster ECS a verificar al finalizar (default: ECS_CLUSTER o espacigo-production)
  --service <servicio>    Servicio ECS a verificar al finalizar (default: ECS_SERVICE o espacigo-app-prod)
  --interval <segundos>   Intervalo de polling en segundos (default: POLL_INTERVAL o 10)
  -h, --help              Muestra esta ayuda

Variables de entorno equivalentes:
  AWS_PROFILE, AWS_REGION, PIPELINE_NAME, ECS_CLUSTER, ECS_SERVICE,
  PRODUCTION_HEALTH_URL, POLL_INTERVAL
EOF
}

require_arg() {
  local opt="$1" val="${2:-}"
  if [[ -z "$val" ]]; then
    printf 'Error: %s requiere un valor.\n' "$opt" >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      require_arg "$1" "${2:-}"
      AWS_PROFILE="$2"
      shift 2
      ;;
    --profile=*)
      AWS_PROFILE="${1#*=}"
      require_arg "--profile" "$AWS_PROFILE"
      shift
      ;;
    --region)
      require_arg "$1" "${2:-}"
      AWS_REGION="$2"
      shift 2
      ;;
    --region=*)
      AWS_REGION="${1#*=}"
      require_arg "--region" "$AWS_REGION"
      shift
      ;;
    --pipeline)
      require_arg "$1" "${2:-}"
      PIPELINE_NAME="$2"
      shift 2
      ;;
    --pipeline=*)
      PIPELINE_NAME="${1#*=}"
      require_arg "--pipeline" "$PIPELINE_NAME"
      shift
      ;;
    --health-url)
      require_arg "$1" "${2:-}"
      PRODUCTION_HEALTH_URL="$2"
      shift 2
      ;;
    --health-url=*)
      PRODUCTION_HEALTH_URL="${1#*=}"
      require_arg "--health-url" "$PRODUCTION_HEALTH_URL"
      shift
      ;;
    --cluster)
      require_arg "$1" "${2:-}"
      ECS_CLUSTER="$2"
      shift 2
      ;;
    --cluster=*)
      ECS_CLUSTER="${1#*=}"
      require_arg "--cluster" "$ECS_CLUSTER"
      shift
      ;;
    --service)
      require_arg "$1" "${2:-}"
      ECS_SERVICE="$2"
      shift 2
      ;;
    --service=*)
      ECS_SERVICE="${1#*=}"
      require_arg "--service" "$ECS_SERVICE"
      shift
      ;;
    --interval)
      require_arg "$1" "${2:-}"
      POLL_INTERVAL="$2"
      shift 2
      ;;
    --interval=*)
      POLL_INTERVAL="${1#*=}"
      require_arg "--interval" "$POLL_INTERVAL"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Error: opción desconocida: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! [[ "$POLL_INTERVAL" =~ ^[1-9][0-9]*$ ]]; then
  printf 'Error: POLL_INTERVAL debe ser un entero positivo (recibido: %s)\n' "$POLL_INTERVAL" >&2
  exit 2
fi

for cmd in aws jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    printf 'Error: se requiere %s en PATH.\n' "$cmd" >&2
    exit 1
  fi
done

if [[ -n "$PRODUCTION_HEALTH_URL" ]] && ! command -v curl >/dev/null 2>&1; then
  printf 'Error: se requiere curl para consultar PRODUCTION_HEALTH_URL.\n' >&2
  exit 1
fi

aws_cmd() {
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" --output json "$@"
}

print_git_refs() {
  if ! command -v git >/dev/null 2>&1 || ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    return 0
  fi

  local branch local_sha remote_sha remote_source
  printf '== Referencias Git ==\n'
  for branch in main publicacion-aws; do
    local_sha="$(git rev-parse --verify --quiet "refs/heads/${branch}" || true)"
    remote_sha="$(git ls-remote --heads origin "$branch" 2>/dev/null | awk 'NR==1 {print $1}' || true)"
    remote_source="origin"
    if [[ -z "$remote_sha" ]]; then
      remote_sha="$(git rev-parse --verify --quiet "refs/remotes/origin/${branch}" || true)"
      remote_source="origin(local-ref)"
    fi
    printf '  %-16s local=%s  %s=%s\n' \
      "$branch" \
      "${local_sha:-no-disponible}" \
      "$remote_source" \
      "${remote_sha:-no-disponible}"
  done
}

extract_codebuild_id() {
  local external_id="${1:-}" external_url="${2:-}"
  if [[ "$external_id" == arn:aws:codebuild:*:build/* ]]; then
    printf '%s\n' "${external_id##*:build/}"
    return 0
  fi
  if [[ -n "$external_id" ]] && [[ "$external_url" == *codebuild* || "$external_id" =~ ^[^:]+:[0-9a-fA-F-]{8,}$ ]]; then
    printf '%s\n' "$external_id"
    return 0
  fi
  if [[ "$external_url" == *codebuild* && "$external_url" =~ /build/([^/?#]+) ]]; then
    printf '%s\n' "${BASH_REMATCH[1]//%3A/:}"
    return 0
  fi
  return 1
}

print_codebuild_logs() {
  local build_id="$1"
  local build_json log_group log_stream
  build_json="$(aws_cmd codebuild batch-get-builds --ids "$build_id" 2>/dev/null || true)"
  if [[ -z "$build_json" ]]; then
    return 0
  fi
  log_group="$(jq -r '.builds[0].logs.groupName // .builds[0].logs.cloudWatchLogs.groupName // empty' <<<"$build_json" 2>/dev/null || true)"
  log_stream="$(jq -r '.builds[0].logs.streamName // .builds[0].logs.cloudWatchLogs.streamName // empty' <<<"$build_json" 2>/dev/null || true)"
  if [[ -n "$log_group" && -n "$log_stream" ]]; then
    printf 'Últimos logs de CodeBuild (%s / %s):\n' "$log_group" "$log_stream" >&2
    aws_cmd logs get-log-events \
      --log-group-name "$log_group" \
      --log-stream-name "$log_stream" \
      --limit 60 2>/dev/null | jq -r '.events[].message' >&2 || true
  fi
}

report_failure_and_exit() {
  local stage_name="$1" action_name="$2" status="$3" summary="$4" err_code="$5" err_msg="$6" ext_id="$7" ext_url="$8"
  printf '\n[ERROR] Etapa "%s" / Acción "%s" finalizó con estado %s\n' "$stage_name" "$action_name" "$status" >&2
  if [[ -n "$err_code" || -n "$err_msg" ]]; then
    printf '  errorDetails: %s%s\n' "${err_code:+[$err_code] }" "${err_msg:-sin detalle}" >&2
  elif [[ -n "$summary" ]]; then
    printf '  summary: %s\n' "$summary" >&2
  fi
  if [[ -n "$ext_url" ]]; then
    printf '  externalExecutionUrl: %s\n' "$ext_url" >&2
  fi

  local build_id=""
  if build_id="$(extract_codebuild_id "$ext_id" "$ext_url")"; then
    printf '  codebuildBuildId: %s\n' "$build_id" >&2
    print_codebuild_logs "$build_id" || true
  fi
  exit 1
}

format_stage_line() {
  local state_json="$1" active_exec_id="$2"
  local -a expected_stages=("Source" "Validate" "Publish" "Migrate" "Deploy")
  local -a out=()
  local stage stage_status stage_exec display_status

  for stage in "${expected_stages[@]}"; do
    stage_status="$(jq -r --arg s "$stage" '
      (.stageStates[] | select(.stageName == $s) | .latestExecution.status) // "Pending"
    ' <<<"$state_json")"
    stage_exec="$(jq -r --arg s "$stage" '
      (.stageStates[] | select(.stageName == $s) | .latestExecution.pipelineExecutionId) // ""
    ' <<<"$state_json")"
    if [[ -n "$active_exec_id" && -n "$stage_exec" && "$stage_exec" != "$active_exec_id" ]]; then
      display_status="Pending(prev:${stage_status})"
    else
      display_status="$stage_status"
    fi
    out+=("${stage}=${display_status}")
  done

  local extra_stages
  extra_stages="$(jq -r '
    .stageStates[].stageName | select(. != "Source" and . != "Validate" and . != "Publish" and . != "Migrate" and . != "Deploy")
  ' <<<"$state_json")"
  if [[ -n "$extra_stages" ]]; then
    while IFS= read -r stage; do
      [[ -z "$stage" ]] && continue
      stage_status="$(jq -r --arg s "$stage" '(.stageStates[] | select(.stageName == $s) | .latestExecution.status) // "Pending"' <<<"$state_json")"
      out+=("${stage}=${stage_status}")
    done <<<"$extra_stages"
  fi

  printf '%s' "${out[*]}"
}

printf '== Watch Deploy EspaciGo ==\n'
printf 'Profile: %s | Region: %s | Pipeline: %s | ECS: %s/%s | Poll: %ss\n' \
  "$AWS_PROFILE" "$AWS_REGION" "$PIPELINE_NAME" "$ECS_CLUSTER" "$ECS_SERVICE" "$POLL_INTERVAL"
if [[ -n "${AWS_ENDPOINT_URL:-}" ]]; then
  printf 'Aviso: AWS_ENDPOINT_URL está definido (%s).\n' "$AWS_ENDPOINT_URL" >&2
fi
print_git_refs
printf '\nMonitoreando pipeline "%s"...\n' "$PIPELINE_NAME"

while true; do
  state_json="$(aws_cmd codepipeline get-pipeline-state --name "$PIPELINE_NAME")"
  active_exec_id="$(jq -r '.stageStates[0].latestExecution.pipelineExecutionId // empty' <<<"$state_json")"
  stage_line="$(format_stage_line "$state_json" "$active_exec_id")"
  printf '[%s] exec=%s | %s\n' "$(date '+%H:%M:%S')" "${active_exec_id:0:8}" "$stage_line"

  # Revisar fallos de acciones en la ejecución activa (o sin executionId asignado aún)
  failed_action_json="$(jq -c --arg exec "$active_exec_id" '
    [
      .stageStates[]
      | select(($exec == "") or ((.latestExecution.pipelineExecutionId // "") == $exec))
      | . as $stage
      | ($stage.actionStates[]?
          | select((.latestExecution.status // "") == "Failed")
          | {
              stageName: $stage.stageName,
              actionName: .actionName,
              status: .latestExecution.status,
              summary: (.latestExecution.summary // ""),
              errorCode: (.latestExecution.errorDetails.code // ""),
              errorMessage: (.latestExecution.errorDetails.message // ""),
              externalExecutionId: (.latestExecution.externalExecutionId // ""),
              externalExecutionUrl: (.latestExecution.externalExecutionUrl // "")
            })
    ][0] // empty
  ' <<<"$state_json")"

  if [[ -n "$failed_action_json" ]]; then
    report_failure_and_exit \
      "$(jq -r '.stageName' <<<"$failed_action_json")" \
      "$(jq -r '.actionName' <<<"$failed_action_json")" \
      "$(jq -r '.status' <<<"$failed_action_json")" \
      "$(jq -r '.summary' <<<"$failed_action_json")" \
      "$(jq -r '.errorCode' <<<"$failed_action_json")" \
      "$(jq -r '.errorMessage' <<<"$failed_action_json")" \
      "$(jq -r '.externalExecutionId' <<<"$failed_action_json")" \
      "$(jq -r '.externalExecutionUrl' <<<"$failed_action_json")"
  fi

  failed_stage_json="$(jq -c --arg exec "$active_exec_id" '
    [
      .stageStates[]
      | select(($exec == "") or ((.latestExecution.pipelineExecutionId // "") == $exec))
      | select((.latestExecution.status // "") == "Failed" or (.latestExecution.status // "") == "Cancelled" or (.latestExecution.status // "") == "Stopped")
      | {stageName, status: .latestExecution.status}
    ][0] // empty
  ' <<<"$state_json")"

  if [[ -n "$failed_stage_json" ]]; then
    printf '\n[ERROR] Etapa "%s" terminó con estado %s.\n' \
      "$(jq -r '.stageName' <<<"$failed_stage_json")" \
      "$(jq -r '.status' <<<"$failed_stage_json")" >&2
    exit 1
  fi

  if jq -e --arg exec "$active_exec_id" '
    ($exec != "")
    and (.stageStates | length > 0)
    and all(.stageStates[]; (.latestExecution.pipelineExecutionId // "") == $exec and (.latestExecution.status // "") == "Succeeded")
  ' <<<"$state_json" >/dev/null; then
    printf '\nTodas las etapas de CodePipeline completaron Succeeded (exec=%s).\n' "$active_exec_id"
    break
  fi

  sleep "$POLL_INTERVAL"
done

printf '\n== Verificación ECS (%s / %s) ==\n' "$ECS_CLUSTER" "$ECS_SERVICE"
ecs_json="$(aws_cmd ecs describe-services --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE")"
if ! jq -e '(.failures | length == 0) and (.services | length == 1)' <<<"$ecs_json" >/dev/null; then
  printf 'Error al consultar servicio ECS:\n' >&2
  jq '.' <<<"$ecs_json" >&2
  exit 1
fi

running_count="$(jq -r '.services[0].runningCount // 0' <<<"$ecs_json")"
desired_count="$(jq -r '.services[0].desiredCount // 0' <<<"$ecs_json")"
rollout_state="$(jq -r '
  ([.services[0].deployments[] | select(.status == "PRIMARY")][0].rolloutState
   // .services[0].deployments[0].rolloutState
   // "UNKNOWN")
' <<<"$ecs_json")"

printf 'ECS Service: runningCount=%s desiredCount=%s rolloutState=%s\n' \
  "$running_count" "$desired_count" "$rollout_state"

if [[ "$rollout_state" == "FAILED" ]] || [[ "$running_count" -ne "$desired_count" ]] || [[ "$rollout_state" != "COMPLETED" ]]; then
  printf 'Error: el servicio ECS no alcanzó estado estable esperado.\n' >&2
  exit 1
fi

if [[ -n "$PRODUCTION_HEALTH_URL" ]]; then
  health_endpoint="${PRODUCTION_HEALTH_URL%/}/api/ready"
  printf '\n== Verificación Healthcheck (%s) ==\n' "$health_endpoint"
  curl -fsS --max-time 15 "$health_endpoint"
  printf '\nHealthcheck OK.\n'
fi

printf '\nDespliegue completado y verificado correctamente.\n'
