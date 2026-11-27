import hashlib
import json
import os
import re
import subprocess
import tempfile
from pathlib import Path
from urllib.parse import urlencode

import boto3
import psycopg
from psycopg import sql
from botocore.config import Config
from botocore.exceptions import BotoCoreError, ClientError

ROOT = Path(__file__).parent
CA = ROOT / "rds-ca.pem"
SECRETS = boto3.client(
    "secretsmanager", endpoint_url=os.environ["SECRETS_ENDPOINT"],
    config=Config(connect_timeout=4, read_timeout=5, retries={"total_max_attempts": 2}),
)


def bootstrap(host, password):
    query = urlencode({"sslmode": "verify-full", "sslrootcert": str(CA)})
    with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as password_file:
        password_file.write(password)
        password_file.flush()
        result = subprocess.run(
            [str(ROOT / "dbmigrate"), "-aws-bootstrap-v80", "-dir", str(ROOT / "migrations")],
            env={
                "DATABASE_URL": f"postgres://espacigo_admin@{host}:5432/espacigo_prod?{query}",
                "DATABASE_PASSWORD_FILE": password_file.name,
                "AWS_RDS_HOST": host,
                "DBMIGRATE_TIMEOUT": "150s",
            },
            timeout=160, capture_output=True, text=True, check=False,
        )
    outcome = json.loads(result.stdout) if result.stdout else {"verified": False}
    if result.returncode:
        # Keep diagnostic metadata. Never return SQL, full exceptions or connection strings.
        filename = re.search(r"V\d{6}__[a-zA-Z0-9_]+\.sql", result.stderr)
        state = re.search(r"SQLSTATE ([0-9A-Z]{5})", result.stderr)
        outcome["failed_migration"] = filename.group() if filename else None
        outcome["sqlstate"] = state.group(1) if state else None
        outcome["verified"] = False
    return outcome


def migration_roles(conn):
    names = [
        "espacigo_admin", "espacigo_migrator_login", "espacigo_schema_owner",
        "espacigo_document_verification_fn", "espacigo_api_login", "espacigo_runtime", "postgres",
    ]
    roles = conn.execute("""
        SELECT rolname, rolcanlogin, rolsuper, rolcreaterole, rolinherit
        FROM pg_catalog.pg_roles WHERE rolname = ANY(%s) ORDER BY rolname
    """, (names,)).fetchall()
    memberships = conn.execute("""
        SELECT r.rolname, m.rolname, g.rolname, a.admin_option, a.inherit_option, a.set_option
        FROM pg_catalog.pg_auth_members a
        JOIN pg_catalog.pg_roles r ON r.oid=a.roleid
        JOIN pg_catalog.pg_roles m ON m.oid=a.member
        JOIN pg_catalog.pg_roles g ON g.oid=a.grantor
        WHERE r.rolname=ANY(%s) OR m.rolname=ANY(%s) ORDER BY 1,2,3
    """, (names[1:], names[1:])).fetchall()
    owners = conn.execute("""
        SELECT c.relname, pg_catalog.pg_get_userbyid(c.relowner)
        FROM pg_catalog.pg_class c
        WHERE c.relnamespace='public'::regnamespace
          AND c.relname IN ('schema_migrations','usuario','espacio','ocupacion','outbox_evento_local')
        ORDER BY 1
    """).fetchall()
    return {
        "roles": [dict(zip(("role", "login", "superuser", "create_role", "inherit"), row)) for row in roles],
        "memberships": [
            dict(zip(("role", "member", "grantor", "admin", "inherit", "set"), row))
            for row in memberships
        ],
        "owners": dict(owners),
        "schema_owner_can_create": conn.execute(
            "SELECT has_schema_privilege('espacigo_schema_owner','public','CREATE')"
        ).fetchone()[0],
        "schema_owner_can_read_control_identity": conn.execute("""
            SELECT has_function_privilege(
                'espacigo_schema_owner','pg_catalog.pg_control_system()','EXECUTE')
        """).fetchone()[0],
    }


def security_acl_audit(conn):
    """Read-only metadata audit of SECURITY DEFINER functions, runtime ACLs, and espacigo_* roles."""
    secdef_rows = conn.execute("""
        SELECT n.nspname || '.' || p.proname,
               p.proconfig,
               pg_catalog.has_function_privilege('public', p.oid, 'EXECUTE'),
               pg_catalog.has_function_privilege('espacigo_runtime', p.oid, 'EXECUTE'),
               pg_catalog.has_function_privilege('espacigo_api_login', p.oid, 'EXECUTE')
        FROM pg_catalog.pg_proc p
        JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
        WHERE p.prosecdef
          AND n.nspname NOT IN ('pg_catalog', 'information_schema')
        ORDER BY 1, p.oid
    """).fetchall()
    has_search_path = [
        any(item.startswith("search_path=") for item in (proconfig or ()))
        for _, proconfig, _, _, _ in secdef_rows
    ]
    sensitive_names = [
        "schema_migrations",
        "evento_auditoria_local",
        "administrador_global",
        "sesion_administrativa",
        "mercadopago_seller_connection",
        "mercadopago_oauth_state",
        "identity_provider_attempt",
        "aviso_local_recuperacion_auditoria",
        "verificacion_historial_local",
    ]
    public_tables = conn.execute("""
        SELECT c.relname,
               pg_catalog.has_table_privilege('espacigo_runtime', c.oid, 'SELECT'),
               pg_catalog.has_table_privilege('espacigo_runtime', c.oid, 'INSERT'),
               pg_catalog.has_table_privilege('espacigo_runtime', c.oid, 'UPDATE'),
               pg_catalog.has_table_privilege('espacigo_runtime', c.oid, 'DELETE')
        FROM pg_catalog.pg_class c
        WHERE c.relnamespace = 'public'::pg_catalog.regnamespace
          AND c.relkind IN ('r', 'p')
        ORDER BY 1
    """).fetchall()
    rls_enabled_count = conn.execute("""
        SELECT count(*)
        FROM pg_catalog.pg_class c
        JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relkind IN ('r', 'p')
          AND c.relrowsecurity
          AND n.nspname NOT IN ('pg_catalog', 'information_schema')
          AND NOT pg_catalog.starts_with(n.nspname, 'pg_')
    """).fetchone()[0]
    public_table_map = {
        name: {"select": sel, "insert": ins, "update": upd, "delete": dele}
        for name, sel, ins, upd, dele in public_tables
    }
    memberships = conn.execute("""
        SELECT r.rolname, m.rolname, a.set_option, a.inherit_option, a.admin_option
        FROM pg_catalog.pg_auth_members a
        JOIN pg_catalog.pg_roles r ON r.oid = a.roleid
        JOIN pg_catalog.pg_roles m ON m.oid = a.member
        WHERE pg_catalog.starts_with(r.rolname, 'espacigo_')
           OR pg_catalog.starts_with(m.rolname, 'espacigo_')
        ORDER BY 1, 2, 3, 4, 5
    """).fetchall()
    roles = conn.execute("""
        SELECT rolname, rolcanlogin, rolsuper, rolcreaterole, rolcreatedb, rolinherit, rolbypassrls
        FROM pg_catalog.pg_roles
        WHERE pg_catalog.starts_with(rolname, 'espacigo_')
        ORDER BY 1
    """).fetchall()
    return {
        "security_definer": {
            "total": len(secdef_rows),
            "with_search_path": sum(1 for ok in has_search_path if ok),
            "public_execute_count": sum(1 for _, _, pub_exec, _, _ in secdef_rows if pub_exec),
            "runtime_execute_count": sum(1 for _, _, _, rt_exec, _ in secdef_rows if rt_exec),
            "api_login_execute_count": sum(1 for _, _, _, _, api_exec in secdef_rows if api_exec),
            "public_execute_functions": [
                name for name, _, pub_exec, _, _ in secdef_rows if pub_exec
            ][:100],
            "missing_search_path_functions": [
                name for (name, _, _, _, _), ok in zip(secdef_rows, has_search_path) if not ok
            ],
        },
        "runtime_table_privileges": {
            "public_base_tables": len(public_tables),
            "rls_enabled_count": rls_enabled_count,
            "runtime_select_count": sum(1 for _, sel, _, _, _ in public_tables if sel),
            "runtime_insert_count": sum(1 for _, _, ins, _, _ in public_tables if ins),
            "runtime_update_count": sum(1 for _, _, _, upd, _ in public_tables if upd),
            "runtime_delete_count": sum(1 for _, _, _, _, dele in public_tables if dele),
            "sensitive_tables": {
                name: public_table_map[name]
                for name in sensitive_names
                if name in public_table_map
            },
        },
        "all_espacigo_memberships": [
            dict(zip(("role", "member", "set", "inherit", "admin"), row))
            for row in memberships
        ],
        "all_espacigo_roles": [
            dict(zip(("role", "login", "superuser", "create_role", "create_db", "inherit", "bypass_rls"), row))
            for row in roles
        ],
    }


def inspect(host, password):
    with psycopg.connect(
        host=host, port=5432, dbname="espacigo_prod", user="espacigo_admin",
        password=password, sslmode="verify-full", sslrootcert=str(CA), connect_timeout=6,
        options="-c default_transaction_read_only=on -c statement_timeout=6000",
    ) as conn:
        checks = conn.execute((ROOT / "preflight_empty.sql").read_text()).fetchone()[0]
        if checks["can_read_control_identity"]:
            checks["control_identity_readable"] = conn.execute(
                "SELECT system_identifier IS NOT NULL FROM pg_catalog.pg_control_system()"
            ).fetchone()[0]
        if checks["has_migration_ledger"]:
            checks["ledger_count"], checks["ledger_max"] = conn.execute(
                "SELECT count(*), max(version) FROM public.schema_migrations"
            ).fetchone()
            checks["migration_authority"] = migration_roles(conn)
            checks["security_acl_audit"] = security_acl_audit(conn)
        return checks


def prepare_migrator(host, admin_password):
    secret_id = os.environ["MIGRATOR_SECRET_ARN"]
    credential = json.loads(SECRETS.get_secret_value(SecretId=secret_id)["SecretString"])
    login, owner = "espacigo_migrator_login", "espacigo_schema_owner"
    if credential.get("username") != login:
        return {"ok": False, "stage": "migrator_secret_identity"}

    options = dict(host=host, port=5432, dbname="espacigo_prod", sslmode="verify-full",
                   sslrootcert=str(CA), connect_timeout=6)
    with psycopg.connect(**options, user="espacigo_admin", password=admin_password) as conn:
        # Keep grants and login changes atomic, and serialize with the Go migrator.
        conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
        if conn.execute("""
            SELECT count(*)=80 AND min(version)=1 AND max(version)=80
            FROM public.schema_migrations
        """).fetchone()[0] is not True:
            return {"ok": False, "stage": "expected_v80"}
        if conn.execute("""
            SELECT count(*)=2
            FROM pg_catalog.pg_roles
            WHERE rolname IN (%s,%s) AND rolcanlogin = (rolname=%s)
              AND NOT (rolsuper OR rolcreaterole OR rolcreatedb OR rolreplication
                       OR rolbypassrls OR rolinherit)
        """, (login, owner, login)).fetchone()[0] is not True:
            return {"ok": False, "stage": "unexpected_role_attributes"}
        conn.execute("""
            GRANT espacigo_schema_owner TO espacigo_migrator_login
            WITH ADMIN FALSE, INHERIT FALSE, SET TRUE
        """)
        conn.execute("SET LOCAL password_encryption = 'scram-sha-256'")
        # Send a SCRAM verifier so SQL statement logging cannot expose the password.
        verifier = conn.pgconn.encrypt_password(credential["password"].encode(), login.encode(), b"scram-sha-256")
        conn.execute(sql.SQL("ALTER ROLE espacigo_migrator_login PASSWORD {}").format(
            sql.Literal(verifier.decode())
        ))

    with psycopg.connect(**options, user=login, password=credential["password"],
                         options="-c default_transaction_read_only=on") as conn:
        conn.execute("SET ROLE espacigo_schema_owner")
        session, effective = conn.execute("SELECT session_user, current_user").fetchone()
    return {"ok": session == login and effective == owner, "login": session, "effective_role": effective}



def use_prefix(host, password, last):
    with tempfile.TemporaryDirectory() as directory:
        path = Path(directory)
        source = ROOT / "migrations"
        for migration in source.glob("V*.sql"):
            if int(migration.name[1:7]) <= last:
                (path / migration.name).symlink_to(migration)
        passfile = path / "pgpass"
        fd = os.open(passfile, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write(f"{host}:5432:espacigo_prod:espacigo_migrator_login:{password}\n")
        query = urlencode({"sslmode": "verify-full", "sslrootcert": str(CA)})
        result = subprocess.run(
            [str(ROOT / "dbmigrate"), "-dir", directory],
            env={
                "DATABASE_URL": f"postgres://espacigo_migrator_login@{host}:5432/espacigo_prod?{query}",
                "DATABASE_SET_ROLE": "espacigo_schema_owner", "PGPASSFILE": str(passfile),
                "DBMIGRATE_TIMEOUT": "15s",
            },
            timeout=18, capture_output=True, text=True, check=False,
        )
        if result.returncode:
            state = re.search(r"SQLSTATE ([0-9A-Z]{5})", result.stderr)
            version = re.search(r"V\d{6}", result.stderr)
            reasons = ("must be owner", "permission denied", "privilege contract",
                       "prerequisite missing", "checksum drift", "dedicated migrator identity",
                       "object owners require", "unexpected ACL", "preflight lacks",
                       "memberships touching", "search path or catalog evidence")
            return {"ok": False, "stage": f"migration_prefix_{last}",
                    "version": version.group() if version else None,
                    "sqlstate": state.group(1) if state else None,
                    "reason": next((reason for reason in reasons if reason in result.stderr), None)}
    return {"ok": True}


def canonical_ledger(conn, target):
    rows = conn.execute("SELECT version,name,checksum FROM public.schema_migrations ORDER BY version").fetchall()
    paths = sorted((ROOT / "migrations").glob("V*.sql"))
    verified = [
        (i, migration.name, hashlib.sha256(migration.read_bytes()).hexdigest())
        for i, migration in enumerate(paths, 1)
    ]
    fingerprint = hashlib.sha256(
        "".join(f"{digest}  {name}\n" for _, name, digest in verified).encode()
    ).hexdigest()
    approved = {
        83: "2bf266d73e8838b16087a7ffce27604d2aece2904faec71054dd98a77b9dc26f",
        93: "7f6f9f7551fb0735bce4ad3ee94146a3d809e7bc9fb8fd6064e20e99df0a0e96",
        98: "288d2877e2a7686dc263f9d3be89b8be0b7c99b3bde58d383357883c4b901122",
        100: "a9a60ebc7e0b926230fb7f1d4140ca2ea12af2617d48d78f775919d435bced61",
    }
    if (len(paths) != target or fingerprint != approved[target]
            or not 80 <= len(rows) <= target or rows != verified[:len(rows)]):
        raise RuntimeError("Unreviewed production migration history")
    return len(rows)


def advance_migrations(host, admin_password, target):
    credential = json.loads(SECRETS.get_secret_value(
        SecretId=os.environ["MIGRATOR_SECRET_ARN"])["SecretString"])
    if credential.get("username") != "espacigo_migrator_login":
        return {"ok": False, "stage": "migrator_secret_identity"}
    stage = "v81_setup"
    options = dict(host=host, port=5432, dbname="espacigo_prod", sslmode="verify-full",
                   sslrootcert=str(CA), connect_timeout=6,
                   options="-c statement_timeout=10000 -c lock_timeout=3000")
    try:
        with psycopg.connect(**options, user="espacigo_admin", password=admin_password) as conn:
            # Serialize invocations of this operation while the Go child takes its own runner lock.
            conn.execute("SELECT pg_advisory_lock(1163087943, 1296648019)")
            conn.commit()
            with conn.transaction():
                conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                version = canonical_ledger(conn, target)
                if version == 80:
                    conn.execute("""
                        GRANT espacigo_document_verification_fn TO espacigo_admin
                            WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
                        CREATE SCHEMA IF NOT EXISTS document_verification_private
                            AUTHORIZATION espacigo_document_verification_fn;
                        REVOKE espacigo_document_verification_fn FROM espacigo_admin;
                        GRANT USAGE ON SCHEMA public TO espacigo_document_verification_fn, espacigo_schema_owner;
                        GRANT SELECT(id,estado), UPDATE(id) ON public.usuario TO espacigo_document_verification_fn;
                        GRANT SELECT(id,propietario_id), UPDATE(id) ON public.espacio TO espacigo_document_verification_fn;
                        GRANT espacigo_document_verification_fn TO espacigo_schema_owner
                            WITH ADMIN FALSE, INHERIT FALSE, SET TRUE;
                        GRANT SELECT, INSERT ON public.schema_migrations TO espacigo_schema_owner;
                    """)
            if version == 80:
                result = use_prefix(host, credential["password"], 81)
                if not result["ok"]:
                    return result
            stage = "v82_setup"
            with conn.transaction():
                conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                version = canonical_ledger(conn, target)
                if version < 83:
                    conn.execute("""
                        GRANT espacigo_schema_owner TO espacigo_admin
                            WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
                        GRANT CREATE ON SCHEMA public TO espacigo_schema_owner;
                        ALTER FUNCTION public.document_verification_space_owner_changed()
                            OWNER TO espacigo_schema_owner;
                        GRANT UPDATE(id) ON public.document_verification_reviewer_grant
                            TO espacigo_schema_owner WITH GRANT OPTION;
                        REVOKE espacigo_schema_owner FROM espacigo_admin;
                    """)
            if version < 83:
                try:
                    result = use_prefix(host, credential["password"], 83)
                finally:
                    stage = "v82_close"
                    with conn.transaction():
                        conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                        conn.execute("""
                            GRANT espacigo_schema_owner TO espacigo_admin
                                WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
                            ALTER FUNCTION public.document_verification_space_owner_changed()
                                OWNER TO espacigo_admin;
                            REVOKE UPDATE(id) ON public.document_verification_reviewer_grant
                                FROM espacigo_schema_owner CASCADE;
                            GRANT UPDATE(id) ON public.document_verification_reviewer_grant TO espacigo_runtime;
                            REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner;
                            REVOKE espacigo_schema_owner FROM espacigo_admin;
                        """)
                if not result["ok"]:
                    return result
            if target >= 93:
                stage = "v84_to_v93"
                result = advance_identity(conn, host, admin_password, credential["password"], target)
                if not result["ok"]:
                    return result
            if target == 98:
                stage = "v94_to_v98"
                result = advance_tail(conn, host, credential["password"])
                if not result["ok"]:
                    return result
            stage = "ledger_readback"
            with conn.transaction():
                version = canonical_ledger(conn, target)
            return {"ok": version == target, "verified_ledger": version}
    except RuntimeError as err:
        allowed = ("V94 preparation requires a fresh outbox",
                   "Review the existing postgres role before assigning the outbox",
                   "Unexpected historical outbox owner", "Unexpected outbox ownership",
                   "Unreviewed production migration history")
        return {"ok": False, "stage": stage, "reason": str(err) if str(err) in allowed else None}
    except psycopg.Error as err:
        detail = (err.diag.message_primary or "").lower()
        reasons = ("must be owner", "permission denied", "must be able to set role",
                   "admin option", "insufficient privilege", "dependent privileges")
        return {"ok": False, "stage": stage, "sqlstate": err.sqlstate,
                "reason": next((reason for reason in reasons if reason in detail), None)}


def advance_identity(conn, host, admin_password, password, target):
    with conn.transaction():
        initial = canonical_ledger(conn, target)
    if initial == 83:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8") as credential_file:
            credential_file.write(admin_password)
            credential_file.flush()
            query = urlencode({"sslmode": "verify-full", "sslrootcert": str(CA)})
            result = subprocess.run(
                [str(ROOT / "dbmigrate"), "-aws-runtime-v83", "-dir", str(ROOT / "migrations")],
                env={
                    "DATABASE_URL": f"postgres://espacigo_admin@{host}:5432/espacigo_prod?{query}",
                    "DATABASE_PASSWORD_FILE": credential_file.name, "AWS_RDS_HOST": host,
                    "DBMIGRATE_TIMEOUT": "15s",
                },
                timeout=18, capture_output=True, text=True, check=False,
            )
            if result.returncode:
                state = re.search(r"SQLSTATE ([0-9A-Z]{5})", result.stderr)
                return {"ok": False, "stage": "v83_runtime_acl",
                        "sqlstate": state.group(1) if state else None}
    for version in range(initial + 1, 94):
        temporary_create = version not in {87, 91}
        reference = version in {84, 85, 89}
        with conn.transaction():
            conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
            if canonical_ledger(conn, target) != version - 1:
                return {"ok": False, "stage": "unexpected_predecessor", "version": version}
            if version == 87:
                conn.execute("""
                    GRANT espacigo_privacy_cleanup_worker TO espacigo_privacy_worker_login
                    WITH ADMIN FALSE, INHERIT FALSE, SET TRUE
                """)
            if temporary_create:
                conn.execute("GRANT CREATE ON SCHEMA public TO espacigo_schema_owner")
            if reference:
                conn.execute("GRANT REFERENCES(id) ON public.usuario TO espacigo_schema_owner")
        try:
            result = use_prefix(host, password, version)
        finally:
            with conn.transaction():
                conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                if reference:
                    conn.execute("REVOKE REFERENCES(id) ON public.usuario FROM espacigo_schema_owner")
                if temporary_create:
                    conn.execute("REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner")
        if not result["ok"]:
            result["verified_predecessor"] = version - 1
            return result
    return {"ok": True}


def prepare_outbox_v94(conn):
    # The historical migration pins the outbox owner by name. In this RDS
    # database, provision that role solely as a non-login object owner.
    owner = conn.execute("""
        SELECT pg_catalog.pg_get_userbyid(relowner)
        FROM pg_catalog.pg_class WHERE oid='public.outbox_evento_local'::regclass
    """).fetchone()[0]
    if owner == "espacigo_admin":
        if conn.execute("SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='postgres')").fetchone()[0]:
            raise RuntimeError("Review the existing postgres role before assigning the outbox")
        conn.execute("""
            CREATE ROLE postgres NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
                NOINHERIT NOREPLICATION NOBYPASSRLS;
            COMMENT ON ROLE postgres IS 'EspaciGo RDS V94 outbox object owner, no login';
            GRANT postgres TO espacigo_admin WITH ADMIN FALSE, INHERIT TRUE, SET TRUE;
            GRANT CREATE ON SCHEMA public TO postgres;
            ALTER TABLE public.outbox_evento_local OWNER TO postgres;
        """)
    elif owner == "postgres":
        tagged = conn.execute("""
            SELECT pg_catalog.shobj_description(oid,'pg_authid') =
                   'EspaciGo RDS V94 outbox object owner, no login'
              AND NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole
                    OR rolinherit OR rolreplication OR rolbypassrls)
            FROM pg_catalog.pg_roles WHERE rolname='postgres'
        """).fetchone()
        if tagged != (True,):
            raise RuntimeError("Unexpected historical outbox owner")
        conn.execute("GRANT postgres TO espacigo_admin WITH ADMIN FALSE, INHERIT TRUE, SET TRUE")
    else:
        raise RuntimeError("Unexpected outbox ownership")
    if conn.execute("SELECT EXISTS (SELECT 1 FROM public.outbox_evento_local)").fetchone()[0]:
        raise RuntimeError("V94 preparation requires a fresh outbox")
    conn.execute("""
        GRANT SELECT(id,agregado_tipo,tipo,clave_deduplicacion,entregada_en,retirar_en,cancelada_en,fallo_terminal_en),
              DELETE ON public.outbox_evento_local TO espacigo_schema_owner;
        GRANT UPDATE(id) ON public.outbox_evento_local TO espacigo_schema_owner;
        REVOKE CREATE ON SCHEMA public FROM postgres;
        REVOKE postgres FROM espacigo_admin;
    """)


def advance_tail(conn, host, password):
    with conn.transaction():
        initial = canonical_ledger(conn, 98)
    for version in range(initial + 1, 99):
        refs = {97: ("usuario", "espacio"), 98: ("espacio",)}.get(version, ())
        with conn.transaction():
            conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
            if canonical_ledger(conn, 98) != version - 1:
                return {"ok": False, "stage": "unexpected_predecessor", "version": version}
            conn.execute("GRANT CREATE ON SCHEMA public TO espacigo_schema_owner")
            if version == 94:
                prepare_outbox_v94(conn)
            if version in {95, 96}:
                relation = "document_verification_case" if version == 95 else "document_verification_event"
                conn.execute("GRANT espacigo_schema_owner TO espacigo_admin "
                             "WITH ADMIN FALSE, INHERIT FALSE, SET TRUE")
                conn.execute(sql.SQL("ALTER TABLE public.{} OWNER TO espacigo_schema_owner").format(
                    sql.Identifier(relation)))
                conn.execute("REVOKE espacigo_schema_owner FROM espacigo_admin")
            for relation in refs:
                conn.execute(sql.SQL("GRANT REFERENCES(id) ON public.{} TO espacigo_schema_owner").format(
                    sql.Identifier(relation)))
        try:
            result = use_prefix(host, password, version)
        finally:
            with conn.transaction():
                conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                for relation in refs:
                    conn.execute(sql.SQL("REVOKE REFERENCES(id) ON public.{} FROM espacigo_schema_owner").format(
                        sql.Identifier(relation)))
                conn.execute("REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner")
        if not result["ok"]:
            result["verified_predecessor"] = version - 1
            return result
    return {"ok": True}


def complete_baseline(host, admin_password):
    credential = json.loads(SECRETS.get_secret_value(
        SecretId=os.environ["MIGRATOR_SECRET_ARN"])["SecretString"])
    if credential.get("username") != "espacigo_migrator_login":
        return {"ok": False, "stage": "migrator_secret_identity"}
    stage = "v100_setup"
    try:
        with psycopg.connect(host=host, port=5432, dbname="espacigo_prod", user="espacigo_admin",
                             password=admin_password, sslmode="verify-full", sslrootcert=str(CA),
                             connect_timeout=6, options="-c statement_timeout=30000 -c lock_timeout=3000",
                             autocommit=True) as conn:
            conn.execute("SELECT pg_advisory_lock(1163087943, 1296648019)")
            with conn.transaction():
                conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                version = canonical_ledger(conn, 100)
                if version not in {99, 100}:
                    return {"ok": False, "stage": "v99_required"}
                if version == 99:
                    conn.execute("GRANT CREATE ON SCHEMA public TO espacigo_schema_owner")
            if version == 99:
                try:
                    result = use_prefix(host, credential["password"], 100)
                finally:
                    with conn.transaction():
                        conn.execute("SELECT pg_advisory_xact_lock(1163087943, 1296648018)")
                        conn.execute("REVOKE CREATE ON SCHEMA public FROM espacigo_schema_owner")
                if not result["ok"]:
                    return result
            stage = "v100_ledger"
            if canonical_ledger(conn, 100) != 100:
                return {"ok": False, "stage": stage}
            stage = "reference_catalog"
            conn.execute((ROOT / "finalize_reference_only.sql").read_text())
            stage = "routine_ddl"
            conn.execute((ROOT / "migration-owner.sql").read_text())
            catalog = conn.execute("""
                SELECT count(*) FILTER (WHERE activa),
                       (SELECT count(*) FROM public.region_publica WHERE catalogo_version='SUBDERE_DPA_2018'),
                       (SELECT count(*) FROM public.comuna_publica WHERE catalogo_version='SUBDERE_DPA_2018')
                FROM public.categoria_espacio
            """).fetchone()
            can_create, api_can_create = conn.execute("""
                SELECT has_schema_privilege('espacigo_schema_owner','public','CREATE'),
                       has_schema_privilege('espacigo_runtime','public','CREATE')
            """).fetchone()
            return {"ok": catalog == (9, 16, 345) and can_create and not api_can_create,
                    "verified_ledger": 100, "categories": catalog[0], "regions": catalog[1],
                    "communes": catalog[2], "routine_ddl": can_create}
    except psycopg.Error as err:
        return {"ok": False, "stage": stage, "sqlstate": err.sqlstate}


def main(event, _context):
    try:
        secret = json.loads(SECRETS.get_secret_value(SecretId=os.environ["RDS_SECRET_ARN"])["SecretString"])
    except ClientError as err:
        return {"ok": False, "stage": "secrets_manager", "code": err.response["Error"]["Code"]}
    except BotoCoreError as err:
        return {"ok": False, "stage": "secrets_manager", "code": type(err).__name__}
    if secret.get("username") != "espacigo_admin":
        return {"ok": False, "stage": "secret_identity"}
    host, password = os.environ["RDS_HOST"], secret["password"]
    try:
        action = (event or {}).get("action")
        if action in {"prepare-migrator", "advance-v83", "advance-v93", "advance-v98", "advance-v100"}:
            if not os.getenv("MIGRATOR_SECRET_ARN"):
                return {"ok": False, "stage": "migrator_configuration_disabled"}
            if action == "advance-v100":
                return complete_baseline(host, password)
            if action != "prepare-migrator":
                return advance_migrations(host, password, int(action.removeprefix("advance-v")))
            return prepare_migrator(host, password)
        before = inspect(host, password)
        if os.environ["BOOTSTRAP_V80"] != "1":
            return {"ok": True, "checks": before}
        outcome = bootstrap(host, password)
        after = inspect(host, password)
        return {"ok": outcome["verified"], "before": before, "after": after, "migration": outcome}
    except ClientError as err:
        error = err.response["Error"]
        message = error.get("Message", "").lower()
        policies = ("vpc endpoint policy", "service control policy", "resource control policy",
                    "identity-based policy", "resource-based policy", "permissions boundary", "kms")
        return {"ok": False, "stage": "migrator_secret", "code": error["Code"],
                "policy": next((policy for policy in policies if policy in message), None)}
    except BotoCoreError as err:
        return {"ok": False, "stage": "migrator_secret", "code": type(err).__name__}
    except psycopg.Error as err:
        return {"ok": False, "stage": "postgresql", "code": err.sqlstate or type(err).__name__}
    except subprocess.TimeoutExpired:
        return {"ok": False, "stage": "bootstrap_timeout", "checks": inspect(host, password)}
