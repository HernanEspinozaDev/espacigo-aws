#!/usr/bin/env bash
# Exercise the location a visitor sees against the installed local schema.
# Only structural SQL and the five named reference tables are exported.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
source_container=espacigo_localbackend_20261024-database-1
source_database=espacigo_aws_local
test_database=phase0_location_test
bounds_file="${ESPACIGO_COMMUNE_BOUNDARIES_FILE:-$repo_root/.local/geo/dpa_2023_communes.bin.gz}"
image='postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677'
[[ -r "$bounds_file" ]] || { echo 'Faltan los límites comunales locales.' >&2; exit 1; }

test_root="$(mktemp -d /tmp/espacigo-phase0-pg.XXXXXX)"
test_socket_dir="$test_root/socket"
mkdir -m 0777 "$test_socket_dir"
test_container=""
cleanup() {
  if [[ -n "$test_container" ]]; then
    docker stop -t 3 "$test_container" >/dev/null 2>&1 || true
    docker rm -f "$test_container" >/dev/null 2>&1 || true
  fi
  rmdir "$test_socket_dir" "$test_root" || true
}
trap cleanup EXIT

test_container="$(docker create --pull never --network none \
  --tmpfs /var/lib/postgresql:rw,size=2g \
  --tmpfs /docker-entrypoint-initdb.d:rw,noexec,nosuid,size=1m \
  --mount "type=bind,src=$test_socket_dir,dst=/var/run/postgresql" \
  -e POSTGRES_HOST_AUTH_METHOD=trust "$image")"
docker start "$test_container" >/dev/null
ready=0
for attempt in $(seq 1 45); do
  ready_logs="$(docker logs "$test_container" 2>&1 | rg -c "database system is ready to accept connections" || true)"
  if [[ "${ready_logs:-0}" -ge 2 && -S "$test_socket_dir/.s.PGSQL.5432" ]] &&
    docker exec "$test_container" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
[[ "$ready" == 1 ]] || { echo 'El PostgreSQL desechable no se inició.' >&2; exit 1; }

docker exec "$test_container" createdb -U postgres "$test_database"
docker exec "$source_container" pg_dump -w -U espacigo_admin -d "$source_database" \
  --schema-only --no-owner --no-privileges |
  docker exec -i "$test_container" psql -qX -v ON_ERROR_STOP=1 -U postgres -d "$test_database" >/dev/null
docker exec "$source_container" pg_dump -w -U espacigo_admin -d "$source_database" \
  --data-only --no-owner --no-privileges \
  --table=public.categoria_espacio --table=public.categoria_perfil_atributos \
  --table=public.catalogo_ubicacion_publica --table=public.region_publica --table=public.comuna_publica |
  docker exec -i "$test_container" psql -qX -v ON_ERROR_STOP=1 -U postgres -d "$test_database" >/dev/null

env -u DATABASE_URL -u DATABASE_SET_ROLE -u PGOPTIONS -u PGSERVICE -u PGPASSFILE -u PGPASSWORD \
  PHASE0_ISOLATED_DATABASE_URL="postgres://postgres@localhost/$test_database?host=$test_socket_dir&sslmode=disable" \
  ESPACIGO_COMMUNE_BOUNDARIES_FILE="$bounds_file" \
  go test -count=1 -timeout=3m -v ./internal/adapters/postgres/spaces -run '^TestPhase0LocationFromDraftToPublicMap$'
