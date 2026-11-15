#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
SECRETS_DIR="$ROOT_DIR/.local/secrets"
EMULATOR_ENV="${AWS_EMULATOR_ENV_FILE:-$HOME/.aws/emulator/9297.env}"

BUCKETS=(
  "espacigo-evidence-local"
  "espacigo-media-local"
  "espacigo-kyc-identity-local"
  "espacigo-kyc-space-local"
  "espacigo-privacy-replay-local"
)

ensure_local_secrets() {
  umask 077
  mkdir -p "$SECRETS_DIR"
  chmod 0700 "$SECRETS_DIR"

  python3 - "$SECRETS_DIR" <<'PY'
import json
import os
import pathlib
import re
import secrets
import stat
import sys

secrets_dir = pathlib.Path(sys.argv[1])
if not secrets_dir.is_dir() or secrets_dir.is_symlink():
    raise SystemExit(f"refusing non-directory secrets path: {secrets_dir}")
os.chmod(secrets_dir, 0o700)

def write_atomic_0600(path: pathlib.Path, content: str) -> None:
    if path.exists():
        st = path.lstat()
        if not stat.S_ISREG(st.st_mode) or stat.S_ISLNK(st.st_mode):
            raise SystemExit(f"refusing non-regular secret file: {path}")
        os.chmod(path, 0o600)
        if path.stat().st_size > 0:
            return
        path.unlink()
    fd = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, "w", encoding="ascii") as handle:
        handle.write(content)
    os.chmod(path, 0o600)

# 1. Document verification keyring JSON (current_version + keys)
doc_key = secrets.token_hex(32)
doc_keyring_payload = json.dumps(
    {"current_version": "v1", "keys": {"v1": doc_key}},
    indent=2,
) + "\n"
for name in ("document-verification-keyring.json", "document_verification_encryption_keyring.json"):
    path = secrets_dir / name
    write_atomic_0600(path, doc_keyring_payload)
    parsed = json.loads(path.read_text(encoding="ascii"))
    if parsed.get("current_version") != "v1" or not re.fullmatch(r"[0-9a-f]{64}", parsed.get("keys", {}).get("v1", "")):
        raise SystemExit(f"invalid document verification keyring in {path}")

# 2. Identity declaration keyring JSON (current + keys)
decl_key = secrets.token_hex(32)
decl_keyring_payload = json.dumps(
    {"current": "v1", "keys": {"v1": decl_key}},
    indent=2,
) + "\n"
for name in ("identity-declaration-keyring.json", "identity_declaration_keyring.json"):
    path = secrets_dir / name
    write_atomic_0600(path, decl_keyring_payload)
    parsed = json.loads(path.read_text(encoding="ascii"))
    if parsed.get("current") != "v1" or not re.fullmatch(r"[0-9a-f]{64}", parsed.get("keys", {}).get("v1", "")):
        raise SystemExit(f"invalid identity declaration keyring in {path}")

# 3. Identity RUT HMAC keyring JSON (active + keys)
rut_key = secrets.token_hex(32)
rut_keyring_payload = json.dumps(
    {"active": "v1", "keys": {"v1": rut_key}},
    indent=2,
) + "\n"
for name in ("identity-rut-hmac-keyring.json", "identity_rut_hmac_keyring.json"):
    path = secrets_dir / name
    write_atomic_0600(path, rut_keyring_payload)
    parsed = json.loads(path.read_text(encoding="ascii"))
    if parsed.get("active") != "v1" or not re.fullmatch(r"[0-9a-f]{64}", parsed.get("keys", {}).get("v1", "")):
        raise SystemExit(f"invalid identity RUT HMAC keyring in {path}")

# 4. 32-byte hex secrets (64 lowercase hex characters)
hex_groups = [
    ("local_contract_encryption_key", "contract-encryption-key"),
    ("public_catalog_cursor_key", "public-catalog-cursor-key"),
    ("local_payment_webhook_secret", "payment-webhook-secret"),
    ("db_admin_password",),
    ("runtime_password",),
]
for group in hex_groups:
    val = secrets.token_hex(32) + "\n"
    for name in group:
        path = secrets_dir / name
        write_atomic_0600(path, val)
        actual = path.read_text(encoding="ascii").strip()
        if not re.fullmatch(r"[0-9a-f]{64}", actual):
            raise SystemExit(f"invalid 32-byte hex secret in {path}")
PY
  echo "PASS: local development secrets verified in $SECRETS_DIR (dir=0700, files=0600)"
}

bootstrap_aws_emulator() {
  if [[ ! -f "$EMULATOR_ENV" ]]; then
    echo "ERROR: aws-emulator environment file not found at $EMULATOR_ENV" >&2
    exit 1
  fi

  (
    export PATH="/opt/aws-emulator/bin:$HOME/.local/bin:/usr/local/bin:$PATH"
    # shellcheck disable=SC1090
    . "$EMULATOR_ENV"

    health_Raw="$(curl --cacert "$AWS_CA_BUNDLE" -fsS "$AWS_ENDPOINT_URL/__admin/health")"
    health_status="$(python3 - "$health_Raw" <<'PY'
import json, sys
raw = sys.argv[1].strip()
if raw == "ok":
    print("ok")
else:
    data = json.loads(raw)
    print(data.get("status", ""))
PY
)"
    if [[ "$health_status" != "ok" ]]; then
      echo "ERROR: aws-emulator health check failed: $health_Raw" >&2
      exit 1
    fi
    echo "PASS: aws-emulator health check == ok ($AWS_ENDPOINT_URL/__admin/health)"

    account_id="$(aws sts get-caller-identity --query 'Account' --output text)"
    if [[ "$account_id" != "000000000000" ]]; then
      echo "ERROR: unexpected AWS caller identity account: $account_id (expected 000000000000)" >&2
      exit 1
    fi
    echo "PASS: aws sts get-caller-identity Account == $account_id"

    for bucket in "${BUCKETS[@]}"; do
      if aws s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
        echo "OK: S3 bucket already exists: $bucket"
      else
        aws s3api create-bucket --bucket "$bucket" >/dev/null
        echo "CREATED: S3 bucket: $bucket"
      fi
    done

    echo "PASS: all 5 local S3 buckets verified on $AWS_ENDPOINT_URL"
  )
}

ensure_local_secrets
bootstrap_aws_emulator
