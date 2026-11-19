#!/usr/bin/env bash
set -Eeuo pipefail

REPO_ROOT="/home/nandev/Documentos/projects/espacigo-aws"
LOCAL_DEV="${REPO_ROOT}/scripts/local-dev.sh"
# Los lanzadores gráficos no siempre cargan el PATH de la sesión interactiva.
PATH="/usr/local/go/bin:${HOME}/go/bin:${HOME}/.hermes/tools/pnpm-12.15.0-linux-x64:${HOME}/.hermes/tools/node-26.12.1-linux-x64/bin:${PATH}"
export PATH

if [[ ! -x "$LOCAL_DEV" ]]; then
  printf 'No se encontró el comando ejecutable: %s\n' "$LOCAL_DEV" >&2
  read -r -p 'Presiona Enter para cerrar...' _ || true
  exit 1
fi

printf '%s\n' '=== EspaciGo AWS: inicio local de Backend y Web ==='
printf '%s\n' 'API: http://127.0.0.1:8080 | Web: http://127.0.0.1:3000'
printf '%s\n' 'Se abrirán dos servicios en esta terminal. Ctrl+C los detiene.'
printf '\n'

pids=()
cleanup() {
  trap - INT TERM EXIT
  for pid in "${pids[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
  for pid in "${pids[@]:-}"; do
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup INT TERM EXIT

"$LOCAL_DEV" api &
pids+=("$!")
"$LOCAL_DEV" web &
pids+=("$!")

set +e
wait -n "${pids[@]}"
result=$?
set -e
if (( result != 0 )); then
  printf '\nUno de los servicios terminó con error (código %s).\n' "$result" >&2
  if [[ -t 0 ]]; then
    read -r -p 'Presiona Enter para cerrar...' _ || true
  fi
fi
exit "$result"
