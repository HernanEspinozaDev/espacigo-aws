#!/usr/bin/env python3
"""Comprueba RDS privado con recursos temporales y AWS CLI v2."""
import argparse
import json
import os
import re
import secrets as random_secrets
import signal
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REGION = "us-east-2"
PROJECT = "805068224315"
DATABASE = "espacigo-db-prod"
BUCKET = f"espacigo-pilot-artifacts-{PROJECT}-{REGION}"
SERVICE = f"com.amazonaws.{REGION}.secretsmanager"
ENI_ACTIONS = ["ec2:" + action for action in (
    "CreateNetworkInterface", "DescribeNetworkInterfaces", "DescribeSubnets",
    "DeleteNetworkInterface", "AssignPrivateIpAddresses", "UnassignPrivateIpAddresses",
)]
TAG = "EspaciGoTemporaryRDSCheck"


class AWSFailure(RuntimeError):
    pass


def aws(profile, *args, missing=()):
    excluded = {"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
                "AWS_SECURITY_TOKEN", "AWS_CA_BUNDLE", "AWS_DEFAULT_PROFILE"}
    env = {k: v for k, v in os.environ.items()
           if k not in excluded and not k.startswith("AWS_ENDPOINT_URL")}
    env.update(AWS_IGNORE_CONFIGURED_ENDPOINT_URLS="true",
               AWS_EC2_METADATA_DISABLED="true", AWS_RETRY_MODE="standard", AWS_MAX_ATTEMPTS="3")
    result = subprocess.run(
        ["aws", "--profile", profile, "--region", REGION, "--no-cli-pager",
         "--output", "json", "--cli-connect-timeout", "10", "--cli-read-timeout", "240",
         *args], env=env, text=True, capture_output=True, check=False,
    )
    if result.returncode:
        if any(f"({code})" in result.stderr for code in missing):
            return {}
        raise AWSFailure(f"{args[0]} {args[1]}: {result.stderr.strip()}")
    return json.loads(result.stdout) if result.stdout.strip() else {}


def policy(*statements):
    return json.dumps({"Version": "2012-10-17", "Statement": statements})


def build_package(directory, bootstrap, ceiling=80):
    package = directory / "package"
    package.mkdir()
    subprocess.run([
        "uv", "pip", "install", "--target", str(package), "--link-mode", "copy",
        "--python-version", "3.13",
        "--python-platform", "x86_64-manylinux_2_28", "--only-binary", ":all:",
        "psycopg[binary]==3.2.13",
    ], check=True)
    for source in (ROOT / "infra/codepipeline/rds-check/handler.py",
                   ROOT / "db/bootstrap/aws-main/preflight_empty.sql"):
        (package / source.name).write_bytes(source.read_bytes())
    subprocess.run([
        "curl", "--fail", "--silent", "--show-error", "--max-time", "30",
        f"https://truststore.pki.rds.amazonaws.com/{REGION}/{REGION}-bundle.pem",
        "--output", str(package / "rds-ca.pem"),
    ], check=True)
    if ceiling == 100:
        for source in (ROOT / "db/bootstrap/aws-main/finalize_reference_only.sql",
                       ROOT / "db/bootstrap/migration-owner.sql"):
            (package / source.name).write_bytes(source.read_bytes())
    if bootstrap:
        migrations = package / "migrations"
        migrations.mkdir()
        paths = subprocess.check_output(
            ["git", "ls-tree", "--name-only", "origin/main", "db/migrations/"],
            cwd=ROOT, text=True,
        ).splitlines()
        for path in paths:
            name = Path(path).name
            if name.startswith("V") and name[1:7].isdigit() and 1 <= int(name[1:7]) <= ceiling:
                (migrations / name).write_bytes(subprocess.check_output(
                    ["git", "show", f"origin/main:{path}"], cwd=ROOT,
                ))
        subprocess.run(
            ["go", "build", "-trimpath", "-o", str(package / "dbmigrate"), "./cmd/dbmigrate"],
            cwd=ROOT, env=dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0"),
            check=True,
        )
    # Lambda unpacks files under its own execution identity.
    for item in package.rglob("*"):
        item.chmod(0o755 if item.is_dir() or item.name == "dbmigrate" else 0o644)
    archive = directory / "probe.zip"
    subprocess.run(["zip", "-q", "-r", str(archive), "."], cwd=package, check=True)
    return archive


def wait(check, description, timeout=1500):
    deadline = time.monotonic() + timeout
    while not check():
        if time.monotonic() >= deadline:
            raise RuntimeError(f"Tiempo agotado: {description}")
        time.sleep(5)


def cleanup(call, name, vpc):
    # Retain the ENI policy and role until Lambda releases its dedicated interface.
    failures = []

    def remove(*args, **kwargs):
        try:
            return call(*args, **kwargs)
        except AWSFailure as err:
            failures.append(str(err))
            return None

    absent = ("ResourceNotFoundException",)
    role_absent = ("NoSuchEntity",)
    group_filter = f"Name=tag:{TAG},Values={name}"
    endpoint = remove("ec2", "describe-vpc-endpoints", "--filters", group_filter,
                      f"Name=vpc-id,Values={vpc}", f"Name=service-name,Values={SERVICE}")
    if endpoint is not None:
        for item in endpoint["VpcEndpoints"]:
            if item["State"] not in {"deleting", "deleted"}:
                response = remove("ec2", "delete-vpc-endpoints", "--vpc-endpoint-ids",
                                  item["VpcEndpointId"])
                if response and response.get("Unsuccessful"):
                    failures.append(str(response["Unsuccessful"]))

    remove("lambda", "delete-function", "--function-name", name, missing=absent)
    remove("iam", "delete-role-policy", "--role-name", name, "--policy-name",
           "ReadDatabaseSecret", missing=role_absent)

    key = f"temporary-rds-check/{name}.zip"
    versions = remove("s3api", "list-object-versions", "--bucket", BUCKET,
                      "--expected-bucket-owner", PROJECT, "--prefix", key)
    if versions is not None:
        for item in versions.get("Versions", []) + versions.get("DeleteMarkers", []):
            if item["Key"] == key:
                remove("s3api", "delete-object", "--bucket", BUCKET, "--key", key,
                       "--expected-bucket-owner", PROJECT, "--version-id", item["VersionId"])

    if not failures and endpoint is not None and endpoint["VpcEndpoints"]:
        wait(lambda: not any(item["State"] != "deleted" for item in call(
            "ec2", "describe-vpc-endpoints", "--filters", group_filter,
            f"Name=vpc-id,Values={vpc}")["VpcEndpoints"]), "retirada del endpoint")
    if failures:
        raise RuntimeError("\n".join(failures))

    groups = call("ec2", "describe-security-groups", "--filters", group_filter,
                  f"Name=vpc-id,Values={vpc}")["SecurityGroups"]
    for group in groups:
        group_id = group["GroupId"]
        print(f"Esperando que Lambda libere la red de {name}", flush=True)
        wait(lambda: not call(
            "ec2", "describe-network-interfaces", "--filters",
            f"Name=group-id,Values={group_id}")["NetworkInterfaces"], "retirada de la ENI de Lambda")
        call("ec2", "delete-security-group", "--group-id", group_id,
             missing=("InvalidGroup.NotFound",))
    call("iam", "delete-role-policy", "--role-name", name, "--policy-name", "LambdaNetwork",
         missing=role_absent)
    call("iam", "delete-role", "--role-name", name, missing=role_absent)


def probe(call, args, name, db, subnet, archive, response):
    vpc = db["DBSubnetGroup"]["VpcId"]
    key = f"temporary-rds-check/{name}.zip"
    secrets = db["MasterUserSecret"]["SecretArn"]
    tags = {TAG: name}
    function = f"arn:aws:lambda:{REGION}:{PROJECT}:function:{name}"
    role_arn = f"arn:aws:iam::{PROJECT}:role/{name}"
    group = call("ec2", "create-security-group", "--vpc-id", vpc,
                 "--group-name", name, "--description", "Isolate temporary Lambda ENI",
                 "--tag-specifications", json.dumps([
                     {"ResourceType": "security-group", "Tags": [{"Key": TAG, "Value": name}]}]))["GroupId"]
    endpoint = call(
        "ec2", "create-vpc-endpoint", "--vpc-id", vpc, "--service-name", SERVICE,
        "--vpc-endpoint-type", "Interface", "--no-private-dns-enabled", "--client-token", name,
        "--subnet-ids", subnet, "--security-group-ids", args.endpoint_sg,
        "--policy-document", policy({
            "Effect": "Allow", "Principal": "*", "Action": "secretsmanager:GetSecretValue",
            "Resource": secrets, "Condition": {"ArnEquals": {"aws:PrincipalArn": role_arn}},
        }), "--tag-specifications", json.dumps([
            {"ResourceType": "vpc-endpoint", "Tags": [{"Key": TAG, "Value": name}]}]),
    )["VpcEndpoint"]["VpcEndpointId"]

    def endpoint_dns():
        current = call("ec2", "describe-vpc-endpoints", "--vpc-endpoint-ids", endpoint)["VpcEndpoints"][0]
        if current["State"] not in {"pending", "available"}:
            raise RuntimeError(f"Endpoint {endpoint}: {current['State']}")
        if current["State"] == "available" and current.get("DnsEntries"):
            return current["DnsEntries"][0]["DnsName"]
        return ""

    call("iam", "create-role", "--role-name", name, "--tags", f"Key={TAG},Value={name}",
         "--assume-role-policy-document", policy({
             "Effect": "Allow", "Principal": {"Service": "lambda.amazonaws.com"},
             "Action": "sts:AssumeRole",
         }))
    call("iam", "put-role-policy", "--role-name", name, "--policy-name", "LambdaNetwork",
         "--policy-document", policy(
             {"Effect": "Allow", "Action": ENI_ACTIONS, "Resource": "*"},
             {"Effect": "Deny", "Action": ENI_ACTIONS, "Resource": "*",
              "Condition": {"ArnEquals": {"lambda:SourceFunctionArn": function}}},
         ))
    call("iam", "put-role-policy", "--role-name", name, "--policy-name", "ReadDatabaseSecret",
         "--policy-document", policy({
             "Effect": "Allow", "Action": "secretsmanager:GetSecretValue", "Resource": secrets,
             "Condition": {"StringEquals": {"aws:SourceVpce": endpoint}},
         }))
    obj = call("s3api", "put-object", "--bucket", BUCKET, "--key", key,
               "--expected-bucket-owner", PROJECT,
               "--server-side-encryption", "AES256", "--body", str(archive))
    code = {"S3Bucket": BUCKET, "S3Key": key}
    if obj.get("VersionId"):
        code["S3ObjectVersion"] = obj["VersionId"]
    wait(endpoint_dns, "disponibilidad del endpoint", timeout=420)
    environment = {"Variables": {"RDS_HOST": db["Endpoint"]["Address"],
                                  "RDS_SECRET_ARN": secrets,
                                  "SECRETS_ENDPOINT": "https://" + endpoint_dns(),
                                  "BOOTSTRAP_V80": "1" if args.bootstrap_v80 else "0"}}

    def create_lambda():
        try:
            call("lambda", "create-function", "--function-name", name, "--runtime", "python3.13",
                 "--handler", "handler.main", "--role", role_arn, "--code", json.dumps(code),
                 "--timeout", "200" if args.bootstrap_v80 else "60", "--memory-size", "512", "--environment", json.dumps(environment),
                 "--vpc-config", json.dumps({"SubnetIds": [subnet],
                                            "SecurityGroupIds": [args.client_sg, group]}),
                 "--tags", json.dumps(tags))
        except AWSFailure as err:
            if "cannot be assumed by Lambda" in str(err):
                return False
            raise
        return True

    wait(create_lambda, "propagación del rol IAM", timeout=120)
    call("lambda", "wait", "function-active-v2", "--function-name", name)
    invoke(call, name, response)


def invoke(call, name, response, action=None):
    options = ["--cli-binary-format", "raw-in-base64-out",
               "--payload", json.dumps({"action": action})] if action else []
    metadata = call("lambda", "invoke", "--function-name", name, *options, str(response))
    result = json.loads(response.read_text())
    print(json.dumps(result, indent=2), flush=True)
    if metadata.get("FunctionError") or not result.get("ok"):
        raise RuntimeError("Falló la comprobación en Lambda")
    return result


def configure_migrator(call, name, vpc, response, target=None):
    function = call("lambda", "get-function", "--function-name", name)
    config = function["Configuration"]
    db = call("rds", "describe-db-instances", "--db-instance-identifier", DATABASE)["DBInstances"][0]
    admin_secret = db["MasterUserSecret"]["SecretArn"]
    role_arn = f"arn:aws:iam::{PROJECT}:role/{name}"
    variables = config["Environment"]["Variables"]
    if (function.get("Tags", {}).get(TAG) != name or config["Role"] != role_arn
            or config["VpcConfig"]["VpcId"] != vpc or config["State"] != "Active"
            or variables.get("RDS_SECRET_ARN") != admin_secret
            or variables.get("RDS_HOST") != db["Endpoint"]["Address"]
            or variables.get("BOOTSTRAP_V80") != "0"):
        raise RuntimeError("La Lambda del recibo no coincide con el acceso privado de producción")
    endpoints = call("ec2", "describe-vpc-endpoints", "--filters", f"Name=tag:{TAG},Values={name}",
                     f"Name=vpc-id,Values={vpc}", f"Name=service-name,Values={SERVICE}")["VpcEndpoints"]
    endpoint, = [item["VpcEndpointId"] for item in endpoints if item["State"] == "available"]
    secret_name = "espacigo/prod/database/migrator"
    secret_tags = [{"Key": "EspaciGoPurpose", "Value": "dedicated-migration-login"}]

    with tempfile.TemporaryDirectory(prefix="espacigo-rds-migrator-") as temporary:
        directory = Path(temporary)
        archive = build_package(directory, bootstrap=target is not None, ceiling=target or 80)
        credential = call("secretsmanager", "describe-secret", "--secret-id", secret_name,
                          missing=("ResourceNotFoundException",))
        if not credential and target:
            raise RuntimeError("Falta la credencial dedicada del migrador")
        if not credential:
            secret_file = directory / "credential.json"
            secret_file.write_text(json.dumps({
                "username": "espacigo_migrator_login", "password": random_secrets.token_urlsafe(48),
            }))
            try:
                credential = call("secretsmanager", "create-secret", "--name", secret_name,
                                  "--description", "Dedicated EspaciGo production migration login",
                                  "--tags", json.dumps(secret_tags),
                                  "--secret-string", f"file://{secret_file}")
            finally:
                secret_file.unlink()
        elif credential.get("DeletedDate") or secret_tags[0] not in credential.get("Tags", []):
            raise RuntimeError("El secreto existente requiere revisión antes de reutilizarlo")

        allowed_secrets = [admin_secret, credential["ARN"]]
        call("ec2", "modify-vpc-endpoint", "--vpc-endpoint-id", endpoint, "--policy-document", policy({
            "Effect": "Allow", "Principal": "*", "Action": "secretsmanager:GetSecretValue",
            "Resource": allowed_secrets, "Condition": {"ArnEquals": {"aws:PrincipalArn": role_arn}},
        }))
        call("iam", "put-role-policy", "--role-name", name, "--policy-name", "ReadDatabaseSecret",
             "--policy-document", policy({
                 "Effect": "Allow", "Action": "secretsmanager:GetSecretValue",
                 "Resource": allowed_secrets, "Condition": {"StringEquals": {"aws:SourceVpce": endpoint}},
             }))
        key = f"temporary-rds-check/{name}.zip"
        obj = call("s3api", "put-object", "--bucket", BUCKET, "--key", key,
                   "--expected-bucket-owner", PROJECT, "--server-side-encryption", "AES256",
                   "--body", str(archive))
        version = ["--s3-object-version", obj["VersionId"]] if obj.get("VersionId") else []
        call("lambda", "update-function-code", "--function-name", name, "--s3-bucket", BUCKET,
             "--s3-key", key, *version)

    def update_env(settings, timeout):
        call("lambda", "wait", "function-updated-v2", "--function-name", name)
        call("lambda", "update-function-configuration", "--function-name", name,
             "--environment", json.dumps({"Variables": settings}), "--timeout", str(timeout))
        call("lambda", "wait", "function-updated-v2", "--function-name", name)

    readonly_env = {k: v for k, v in variables.items() if k != "MIGRATOR_SECRET_ARN"}
    try:
        update_env(dict(readonly_env, MIGRATOR_SECRET_ARN=credential["ARN"]), 180 if target else config["Timeout"])
        result = invoke(call, name, response, action=f"advance-v{target}" if target else "prepare-migrator")
        if not target and (result.get("login"), result.get("effective_role")) != (
                "espacigo_migrator_login", "espacigo_schema_owner"):
            raise RuntimeError("La comprobación no devolvió el login y rol del migrador")
    finally:
        update_env(readonly_env, config["Timeout"])
    if not target:
        print(f"Credencial de migraciones verificada: {secret_name}")



def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profile", default="espacigo-prod")
    parser.add_argument("--client-sg", default="sg-025ab27a631119417")
    parser.add_argument("--endpoint-sg", default="sg-02aa643577370cdd0")
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--cleanup", metavar="RECIBO", type=Path)
    modes.add_argument("--keep-open", action="store_true",
                       help="mantener abierto el acceso de diagnóstico hasta --cleanup")
    modes.add_argument("--advance-v100", metavar="RECIBO", type=Path,
                       help="completar V100 y habilitar el ciclo normal después de verificar V99")
    modes.add_argument("--advance-v98", metavar="RECIBO", type=Path,
                       help="aplicar el tramo revisado hasta V98 usando la Lambda activa")
    modes.add_argument("--advance-v93", metavar="RECIBO", type=Path,
                       help="aplicar el tramo revisado hasta V93 usando la Lambda activa")
    modes.add_argument("--advance-v83", metavar="RECIBO", type=Path,
                       help="aplicar el tramo revisado V81–V83 usando la Lambda activa")
    modes.add_argument("--configure-migrator", metavar="RECIBO", type=Path,
                       help="preparar y verificar el login dedicado usando la Lambda activa")
    modes.add_argument("--reuse", metavar="RECIBO", type=Path,
                       help="comprobar la base usando la Lambda ya activa")
    modes.add_argument("--bootstrap-v80", action="store_true",
                       help="aplicar exclusivamente V1–V80 canónicas de origin/main")
    args = parser.parse_args()
    target = None
    staged_receipt = None
    for version in (83, 93, 98, 100):
        if (candidate := getattr(args, f"advance_v{version}")) is not None:
            target, staged_receipt = version, candidate
            break
    os.umask(0o077)

    def terminate(_signum, _frame):
        raise SystemExit(143)

    signal.signal(signal.SIGTERM, terminate)

    def call(*command, **kwargs):
        return aws(args.profile, *command, **kwargs)

    if call("sts", "get-caller-identity").get("Account") != PROJECT:
        parser.error("el perfil no corresponde al proyecto EspaciGo")
    if args.cleanup or args.reuse or args.configure_migrator or staged_receipt:
        receipt_path = args.cleanup or args.reuse or args.configure_migrator or staged_receipt
        receipt = json.loads(receipt_path.read_text())
        name, vpc = receipt["run"], receipt["vpc"]
        if not re.fullmatch(r"espacigo-rds-check-[0-9a-f]{32}", name) or not re.fullmatch(r"vpc-[0-9a-f]+", vpc):
            parser.error("recibo inválido")
        if args.cleanup:
            cleanup(call, name, vpc)
            print(f"Limpieza confirmada: {name}")
        else:
            if not receipt.get("keep_open"):
                parser.error("el recibo no corresponde a un acceso abierto")
            if args.configure_migrator or staged_receipt:
                suffix = f".v{target}.json" if target else ".migrator.json"
                configure_migrator(call, name, vpc, receipt_path.with_suffix(suffix), target=target)
            else:
                invoke(call, name, receipt_path.with_suffix(".result.json"))
        return
    db = call("rds", "describe-db-instances", "--db-instance-identifier", DATABASE)["DBInstances"][0]
    vpc = db["DBSubnetGroup"]["VpcId"]
    if (db["DBInstanceStatus"] != "available" or db["DBName"] != "espacigo_prod"
            or db["Engine"] != "postgres" or db["PubliclyAccessible"] or db["MultiAZ"]
            or db["MasterUsername"] != "espacigo_admin" or db["Endpoint"]["Port"] != 5432
            or db["MasterUserSecret"]["SecretStatus"] != "active"):
        parser.error("RDS no cumple el contrato privado Single-AZ del piloto")
    subnet, = [item["SubnetIdentifier"] for item in db["DBSubnetGroup"]["Subnets"]
               if item["SubnetAvailabilityZone"]["Name"] == db["AvailabilityZone"]]
    name = "espacigo-rds-check-" + uuid.uuid4().hex
    receipts = ROOT / ".local/aws-rds-check"
    receipts.mkdir(parents=True, exist_ok=True)
    receipt_path = receipts / f"{name}.json"
    with tempfile.TemporaryDirectory(prefix="espacigo-rds-check-") as temporary:
        scratch = Path(temporary)
        archive = build_package(scratch, args.bootstrap_v80)
        with receipt_path.open("x") as receipt_file:
            json.dump({"run": name, "vpc": vpc, "keep_open": args.keep_open}, receipt_file)
        print(f"Recibo de limpieza: {receipt_path}", flush=True)
        try:
            probe(call, args, name, db, subnet, archive, receipt_path.with_suffix(".result.json"))
        finally:
            if args.keep_open:
                print(f"Recursos conservados para reutilización: {receipt_path}", flush=True)
            else:
                cleanup(call, name, vpc)
                print(f"Limpieza confirmada: {name}")


if __name__ == "__main__":
    main()
