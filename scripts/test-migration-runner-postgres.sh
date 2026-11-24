#!/usr/bin/env bash
# Creates its own networkless disposable cluster; never accepts an external DSN.
set -euo pipefail
ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
SCOPE="${1:-full}"
case "$SCOPE" in
  full) GO_TEST_PATTERN='^Test(BaselinePostgresIntegration|Runner|PlanOnly|MigratorSerializes|CurrentSchemaMigration|RentalCombinedFixtureOptIn)' ;;
  aws-bootstrap|aws-bootstrap-preview) GO_TEST_PATTERN='^TestRunnerAWSBootstrapReferenceCatalogPostgres$' ;;
  checkout-pro) GO_TEST_PATTERN='^TestCurrentSchemaMigrationCheckoutProUpToV111$' ;;
  v19) GO_TEST_PATTERN='^TestCurrentSchemaMigrationV19WeeklyHours$' ;;
  v76) GO_TEST_PATTERN='^TestGalleryCapacityTransitionV68ToV76$' ;;
  authority-window) GO_TEST_PATTERN='^Test(NativeKYCAuthorityWindow|OwnerSensitiveMigrationChoreography)Postgres$' ;;
  *) echo 'usage: test-migration-runner-postgres.sh [full|checkout-pro|aws-bootstrap|aws-bootstrap-preview|v19|v76|authority-window]' >&2; exit 2 ;;
esac
IMAGE='postgis/postgis:18-3.6@sha256:60f6ad1d21ea86a67d47780b9a0d1e1d200500f62b19293fa834d0dea80b8677'
if [[ "$SCOPE" == authority-window ]]; then
  IMAGE='postgres@sha256:c7fedce47eff6a78cb35dad6b627f0d684107f75ab15c0870e40c093766ba452'
fi
NAME="espacigo-migration-ci-$$"
TMP_DIR="$(mktemp -d "${TMPDIR:?set TMPDIR to your scratch directory}/espacigo-migration-ci.XXXXXX")"
SOCKET_DIR="$TMP_DIR/socket"
OWNED_CONTAINER_ID=''
OWNER_UID="$(id -u)"
OWNER_GID="$(id -g)"
mkdir -m 0777 "$SOCKET_DIR"
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ -n "$OWNED_CONTAINER_ID" ]]; then
    if [[ "$SCOPE" == aws-bootstrap-preview && "$status" -eq 0 ]]; then
      if [[ "$(docker inspect --format '{{.State.Running}}' "$OWNED_CONTAINER_ID")" != true ]] ||
         ! docker exec "$OWNED_CONTAINER_ID" pg_isready -U postgres -d espacigo_prod >/dev/null 2>&1; then
        echo 'refusing to retain a stopped or unhealthy test-only preview container' >&2
        exit 1
      fi
      echo "RETAINED TEST-ONLY PREVIEW: container=$OWNED_CONTAINER_ID name=$NAME socket=$SOCKET_DIR database=espacigo_prod"
      echo "RETAINED TEST-ONLY PREVIEW DSN: postgres://espacigo_api_login@/espacigo_prod?host=$SOCKET_DIR&sslmode=disable"
      echo "RETAINED TEST-ONLY MIGRATOR DSN: postgres://espacigo_migrator_login@/espacigo_prod?host=$SOCKET_DIR&sslmode=disable"
      echo 'This V1..V100 fixture used test-only authority choreography; it is not an operational E1 bootstrap.'
      exit 0
    fi
    if [[ "$(docker inspect --format '{{.State.Running}}' "$OWNED_CONTAINER_ID")" == true ]]; then
      if ! docker exec --user 0 "$OWNED_CONTAINER_ID" chown "$OWNER_UID:$OWNER_GID" /var/run/postgresql; then
        echo 'failed to restore ownership of own socket directory' >&2
        exit 1
      fi
    fi
    if ! docker rm -f "$OWNED_CONTAINER_ID" >/dev/null; then
      echo 'failed to remove own migration container; preserving socket directory' >&2
      exit 1
    fi
    if ! python3 - "$OWNED_CONTAINER_ID" <<'PY'
import subprocess,sys
r=subprocess.run(['docker','inspect',sys.argv[1]],capture_output=True,text=True)
assert r.returncode!=0 and r.stderr.strip().lower()==('error: no such object: '+sys.argv[1]), 'owned container absence not verified'
print('PASS: owned migration container removed; exact-ID absence verified')
PY
    then exit 1; fi
  fi
  python3 -c 'import pathlib,shutil,sys; p=pathlib.Path(sys.argv[1]); shutil.rmtree(p); assert not p.exists(); print("PASS: own migration scratch removed and absence verified")' "$TMP_DIR"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if docker inspect "$NAME" >/dev/null 2>&1; then
  echo 'refusing to reuse an existing migration test container' >&2
  exit 1
fi
OWNED_CONTAINER_ID="$(docker create --name "$NAME" --network none \
  --tmpfs /var/lib/postgresql:rw,size=2g \
  --tmpfs /docker-entrypoint-initdb.d:rw,noexec,nosuid,size=1m \
  --mount "type=bind,src=$SOCKET_DIR,dst=/var/run/postgresql" \
  -e POSTGRES_HOST_AUTH_METHOD=trust "$IMAGE")"
python3 - "$TMP_DIR/container-receipt.json" "$OWNED_CONTAINER_ID" "$NAME" <<'PY'
import json,os,sys
fd=os.open(sys.argv[1],os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
with os.fdopen(fd,'w') as f:json.dump({'id':sys.argv[2],'name':sys.argv[3]},f)
PY
docker start "$OWNED_CONTAINER_ID" >/dev/null
ready=false
for _ in $(seq 1 160); do
  ready_count="$(docker logs "$NAME" 2>&1 | grep -cF 'database system is ready to accept connections' || true)"
  if [[ "${ready_count:-0}" -ge 2 ]] && [[ -S "$SOCKET_DIR/.s.PGSQL.5432" ]] && docker exec "$NAME" pg_isready -U postgres -d postgres >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.25
done
[[ "$ready" == true ]] || { docker logs --tail=30 "$NAME" >&2; echo 'disposable PostgreSQL did not become ready' >&2; exit 1; }
ISOLATION_TOKEN="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
printf '%s\n' "$ISOLATION_TOKEN" > "$SOCKET_DIR/.espacigo-migration-ci"
chmod 0600 "$SOCKET_DIR/.espacigo-migration-ci"
# Assert the resolved resource boundaries before any test can create/drop DBs.
python3 - "$NAME" <<'PY'
import json,subprocess,sys
c=json.loads(subprocess.check_output(['docker','inspect',sys.argv[1]],text=True))[0]
assert c['HostConfig']['NetworkMode']=='none'
assert not c['HostConfig'].get('PortBindings')
assert not any(m['Type']=='volume' for m in c['Mounts'])
socket=next(m['Source'] for m in c['Mounts'] if m['Destination']=='/var/run/postgresql')
print('ISOLATED TARGET: container='+c['Name'].lstrip('/')+' image='+c['Config']['Image']+' network=none ports=none volumes=none PGDATA=tmpfs socket='+socket)
PY
env -u DATABASE_URL -u DATABASE_SET_ROLE -u PGPASSWORD -u PGPASSFILE -u PGOPTIONS -u PGSERVICE -u PGSERVICEFILE \
  TEST_MIGRATOR_CI_ISOLATED=1 \
  TEST_MIGRATOR_RETAIN_AWS_PREVIEW="$([[ "$SCOPE" == aws-bootstrap-preview ]] && echo 1 || echo 0)" \
  TEST_DATABASE_URL="postgres://postgres@localhost/postgres?host=$SOCKET_DIR&sslmode=disable" \
  TEST_MIGRATION_AUTHORITY_REPORT_DIR="$TMP_DIR" \
  go test -race -json -count=1 -timeout=3m \
  -run "$GO_TEST_PATTERN" \
  ./internal/migrator > "$TMP_DIR/results.jsonl" 2>&1 || {
    cat "$TMP_DIR/results.jsonl" >&2
    exit 1
  }
if [[ "$SCOPE" == aws-bootstrap || "$SCOPE" == aws-bootstrap-preview ]]; then
  python3 - "$TMP_DIR/results.jsonl" <<'PY'
import json, sys
with open(sys.argv[1]) as results:
    ran = any(
        event.get("Test") == "TestRunnerAWSBootstrapReferenceCatalogPostgres"
        and event.get("Action") == "pass"
        for event in map(json.loads, results)
    )
if not ran:
    raise SystemExit("AWS bootstrap rehearsal was not executed successfully")
print("PASS: exact V1-V100 migration ledger and reference-only bootstrap")
PY
elif [[ "$SCOPE" == v19 ]]; then
  echo 'PASS: isolated disposable PostgreSQL V1-V19 weekly-hours migration and trigger tests.'
elif [[ "$SCOPE" == v76 ]]; then
  echo 'PASS: isolated disposable PostgreSQL V1-V68-to-V76 gallery capacity transition (10 to 30), concurrent 30th slot, and 31st-slot rejection.'
elif [[ "$SCOPE" == authority-window ]]; then
  echo 'PASS: isolated PostgreSQL 18.7 V95/V96 authority-window checks plus owner-sensitive migration choreography, transactional rollback, ownership and ledger readback; no persistent database touched.'
elif [[ "$SCOPE" == checkout-pro ]]; then
  python3 - "$TMP_DIR/results.jsonl" <<'PY'
import json, sys
root = 'TestCurrentSchemaMigrationCheckoutProUpToV111'
required = {root}
for baseline in (0, 75):
    prefix = f'{root}/baseline-{baseline}'
    required.update({prefix, prefix+'/v97-upgrade', prefix+'/v98-expand',
        prefix+'/commercial-quote-suppression', prefix+'/commercial-quote-persistence',
        prefix+'/commercial-quote-persistence/retention-purge-publication-lock',
        prefix+'/commercial-quote-persistence/admin-local-account-block',
        prefix+'/commercial-quote-persistence/authentication-account-lock',
        prefix+'/contract-identity-reader'})
    required.update(prefix+'/commercial-quote-persistence/authentication-account-lock/'+name for name in
        ('email','verification-id','action-token-id','session-hash'))
actions = {}
with open(sys.argv[1]) as source:
    for line in source:
        event = json.loads(line)
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'):
            actions[event['Test']] = event['Action']
missing = sorted(name for name in required if actions.get(name) != 'pass')
if missing:
    raise SystemExit('Checkout Pro canonical V111 evidence missing/failed/skipped: ' + ', '.join(missing))
print(f'PASS: canonical V1–V111 migration sequence plus Checkout Pro quote suppression/persistence and contract identity passed for fresh and V75 upgrade fixtures.')
PY
else
  python3 - "$TMP_DIR/results.jsonl" <<'PY'
import json, sys
required = {'TestBaselinePostgresIntegration'}
identity_required = {f'TestRunnerAuthorityIdentityReaderPostgres/{name}' for name in [
    'read-only-repeatable-identity', 'reject-read-write', 'reject-read-committed',
    'missing-role-no-partial', 'missing-object-no-partial', 'foreign-database-no-partial',
]}
required.update(identity_required)
acl_required = {f'TestRunnerAuthorityACLReaderPostgres/{name}' for name in [
    'reject-shadowed-transaction-guard', 'reject-untrusted-search-path',
    'identity-reject-shadowed-current-database', 'ignore-shadowed-cardinality',
    'ignore-shadowed-oid-operator',
    'null-acl-builtins-and-columns', 'explicit-public-column-and-grant-option',
    'empty-acl-distinct-from-null', 'quoted-canonical-identity',
    'reject-read-write', 'reject-read-committed', 'missing-object-no-partial',
    'foreign-database-no-partial', 'invalid-scope-no-partial', 'canceled-no-partial',
]}
required.update(acl_required)
extended_authority_required = {f'TestRunnerAuthorityDefaultACLReaderPostgres/{name}' for name in [
    'captures-explicit-all-object-classes-and-global-scope',
    'absent-owner-defaults-explicit-empty-observation', 'explicit-empty-function-default',
    'rejects-missing-owner-with-zero-observation',
]}
extended_authority_required.update(['TestRunnerAuthorityEffectiveReaderPostgres', 'TestRunnerAuthorityACLCatalogPostgres'])
required.update(extended_authority_required)
operational_required = {f'TestRunnerSingleOwnerEffectiveRolePostgres/{name}' for name in [
    'success', 'missing-membership', 'wrong-owner', 'role-not-restored',
    'ddl-rollback', 'ledger-rollback', 'before-authority-rollback',
    'after-authority-rollback', 'committed-postcheck-failure',
]}
required.update(operational_required)
actions = {}
with open(sys.argv[1]) as source:
    for line in source:
        event = json.loads(line)
        if event.get('Test') and event.get('Action') in ('pass', 'fail', 'skip'):
            actions[event['Test']] = event['Action']
missing = sorted(name for name in required if actions.get(name) != 'pass')
if missing:
    raise SystemExit('Baseline migration evidence missing/failed/skipped: ' + ', '.join(missing))
print(f'PASS: baseline integration, authority identity reader {len(identity_required)}, scoped ACL reader {len(acl_required)}, extended authority {len(extended_authority_required)} and single-owner role gates {len(operational_required)} executed, none skipped.')
PY
  env -u DATABASE_URL -u DATABASE_SET_ROLE -u PGPASSWORD -u PGPASSFILE -u PGOPTIONS -u PGSERVICE -u PGSERVICEFILE \
  TEST_RENTALFLOW_POSTGRES_ISOLATED=1 \
  TEST_RENTALFLOW_POSTGRES_ISOLATION_TOKEN="$ISOLATION_TOKEN" \
  TEST_DATABASE_URL="postgres://postgres@localhost/postgres?host=$SOCKET_DIR&sslmode=disable" \
  go test -race -json -count=1 -timeout=3m ./internal/adapters/postgres/rentalflow > "$TMP_DIR/rentalflow.log" 2>&1 || {
    cat "$TMP_DIR/rentalflow.log" >&2
    exit 1
  }
cat "$TMP_DIR/rentalflow.log"
python3 - "$TMP_DIR/rentalflow.log" <<'PY'
import json, sys
root = 'TestPostgresEarlyRentalInitialInsert'
required = [root] + [f'{root}/{name}' for name in (
    'SuccessCommit', 'CallerRollback', 'ActualOccupancyFailure', 'EventInsertFailure',
)] + ['TestPostgresPublicIntervalsUsesWeeklyScheduleAndOccupancy']
read_offer_root = 'TestPostgresEarlyRentalReadOffer'
read_offer_cases = [
    'InitialBothParticipants',
    'NonParticipantHidden',
    'InactiveActorHidden',
    'BlockedActorHidden',
    'PersistedSnapshotNotCurrentListing',
    'CurrentIdentityConfirmation',
]
required += [read_offer_root] + [f'{read_offer_root}/{name}' for name in read_offer_cases]
lock_root = 'TestPostgresEarlyRentalCreateKeyLock'
lock_cases = [
    'SameActorKeyCommitRelease',
    'SameActorKeyRollbackRelease',
    'DifferentActorAndKeyNoBlocking',
    'DifferentActorAndKeyNoBlocking/DifferentKey',
    'DifferentActorAndKeyNoBlocking/DifferentActor',
    'CanceledWaiterReleasesLocks',
]
required += [lock_root] + [f'{lock_root}/{name}' for name in lock_cases]
replay_root = 'TestPostgresEarlyRentalCreateReplayLookup'
replay_cases = [
    'MissingNoWrites',
    'MatchingKeyReturnsOriginal',
    'ChangedFingerprintConflict',
    'AmbiguousSameFingerprint',
    'AmbiguousDifferentFingerprint',
    'ActorAndOpaqueKeyIsolation',
]
required += [replay_root] + [f'{replay_root}/{name}' for name in replay_cases]
claim_root = 'TestPostgresEarlyRentalCreateClaim'
claim_cases = [
    'CommitMetadataAndReplay', 'MissingOffer', 'WrongActor', 'DuplicateKey', 'CallerRollback',
]
required += [claim_root] + [f'{claim_root}/{name}' for name in claim_cases]
protocol_root = 'TestPostgresEarlyRentalCreateProtocol'
protocol_cases = ['SameKeySamePayload', 'SameKeyDifferentPayload']
required += [protocol_root] + [f'{protocol_root}/{name}' for name in protocol_cases]
required += [
    'TestPostgresEarlyRentalReadOffer/InactiveCounterpartVisible',
    'TestPostgresEarlyRentalReadCanceledSentPayment',
]
resolver_root = 'TestPostgresEarlyRentalSnapshotRuntime'
required += [resolver_root] + [f'{resolver_root}/{name}' for name in (
    'SnapshotAvailable', 'CustomTariffConflict',
)]
events_root = 'TestPostgresEarlyRentalReadEvents'
required += [events_root] + [f'{events_root}/{name}' for name in (
    'FullHistoryBothParticipants', 'CompleteSuffixAndEmptyCursor',
    'NonParticipantHiddenAtHighCursor', 'BlockedParticipantHiddenAtHighCursor',
)]
list_offers_root = 'TestPostgresEarlyRentalListOffers'
required += [list_offers_root] + [f'{list_offers_root}/{name}' for name in (
    'BothParticipants', 'OutsiderHidden', 'RoleFiltering',
    'StableKeysetPages', 'ReadOnlyNoWrites',
)]
list_transactions_root = 'TestPostgresEarlyRentalListTransactions'
required += [list_transactions_root] + [f'{list_transactions_root}/{name}' for name in (
    'BothParticipants', 'OutsiderHidden', 'InactiveActorHidden', 'BlockedParticipantHidden',
    'RoleFiltering', 'StateFiltering', 'StableKeysetPages', 'ReadOnlyNoWrites',
)]
confirmation_root = 'TestPostgresEarlyRentalApplyConfirmation'
required += [confirmation_root] + [f'{confirmation_root}/{name}' for name in (
    'IdentityCommit', 'ContractCommit', 'CallerRollback',
)]
payment_send_root = 'TestPostgresEarlyRentalApplyPaymentSend'
required += [payment_send_root] + [f'{payment_send_root}/{name}' for name in (
    'SendCommit', 'CallerRollback',
)]
actions = {}
with open(sys.argv[1]) as source:
    for line in source:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if event.get('Test') in required and event.get('Action') in ('pass', 'fail', 'skip'):
            actions[event['Test']] = event['Action']
missing = sorted(name for name in required if actions.get(name) != 'pass')
if missing:
    raise SystemExit('Required rentalflow cases missing/failed/skipped: ' + ', '.join(missing))
print('PASS: required rentalflow cases passed: ' + ', '.join(required) + '; fixture is V1..V61 plus exact V99 DDL, not migration history; no persistent database apply.')
PY
echo 'PASS: disposable PostgreSQL runner (V1-V98, V75 upgrade, V96-to-V97 authority lifecycle, V97-to-V98 expand, V84 legacy recovery/finalization, quotes/suppression, plan/no-op/failure/checksum/concurrency) and rentalflow availability/early-rental integration on V1-V61 plus exact V99 fixture; cleanup checked separately on exit.'
fi
