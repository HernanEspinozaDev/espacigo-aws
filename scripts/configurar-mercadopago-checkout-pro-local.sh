#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

python3 - "$@" <<'PY'
import argparse
import base64
import getpass
import json
import os
from pathlib import Path
import re
import secrets
import stat
import tempfile
import urllib.error
import urllib.parse
import urllib.request

ORIGIN = "https://prueba-mercadopago.testingpage.store"
CALLBACK = ORIGIN + "/api/backend/host/mercadopago/oauth/callback"
WEBHOOK = ORIGIN + "/api/v1/webhooks/mercadopago"
DIRECTORY = Path(".local/secrets")
CONFIG = DIRECTORY / "mercadopago-test.env"
APP_ID = re.compile(r"[0-9]{8,20}")
TEST_TOKEN = re.compile(r"(?:TEST|APP_USR)-[A-Za-z0-9_-]{12,220}")


class SetupError(Exception):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def read_private(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.getuid():
        raise SetupError("Se requiere un archivo regular, de tu usuario y con permisos 600.")
    return path.read_text()


def read_config():
    if not CONFIG.exists() and not CONFIG.is_symlink():
        return {}
    values = {}
    for line in read_private(CONFIG).splitlines():
        if not line or line.startswith("#"):
            continue
        name, separator, value = line.partition("=")
        if not separator or name in values or not re.fullmatch(r"MERCADOPAGO_[A-Z_]+", name):
            raise SetupError("El archivo local contiene una línea inválida o duplicada.")
        values[name] = value
    return values


def encryption_key_valid(value):
    try:
        return len(base64.b64decode(value, validate=True)) == 32
    except (ValueError, TypeError):
        return False


def validate(values):
    app_id = values.get("MERCADOPAGO_CLIENT_ID", "")
    if not APP_ID.fullmatch(app_id):
        raise SetupError("El Client ID debe contener entre 8 y 20 dígitos.")
    if not re.fullmatch(r"[!-~]{8,256}", values.get("MERCADOPAGO_CLIENT_SECRET", "")):
        raise SetupError("El Client Secret está vacío o tiene un formato inválido.")
    if not TEST_TOKEN.fullmatch(values.get("MERCADOPAGO_TEST_ACCESS_TOKEN", "")):
        raise SetupError("Usa el Access Token de la pestaña Prueba, con prefijo APP_USR- o TEST-.")
    if not encryption_key_valid(values.get("MERCADOPAGO_TOKEN_ENCRYPTION_KEY", "")):
        raise SetupError("La clave local de cifrado debe tener 32 bytes en base64.")
    if values.get("MERCADOPAGO_TEST_MODE") != "true" or values.get("MERCADOPAGO_OAUTH_REDIRECT_URI") != CALLBACK:
        raise SetupError("Reconfigura el modo de prueba y el callback del túnel.")
    return app_id


def request(path, label, *, form=None, token=None):
    data = urllib.parse.urlencode(form).encode() if form else None
    headers = {"Accept": "application/json"}
    if form:
        headers["Content-Type"] = "application/x-www-form-urlencoded"
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request("https://api.mercadopago.com" + path, data=data, headers=headers)
    try:
        with urllib.request.build_opener(NoRedirect()).open(req, timeout=12) as response:
            payload = json.loads(response.read(128 * 1024))
            if not isinstance(payload, dict):
                raise ValueError
            return payload
    except urllib.error.HTTPError as error:
        raise SetupError(f"{label}: Mercado Pago devolvió HTTP {error.code}.") from None
    except (OSError, ValueError):
        raise SetupError(f"{label}: no se pudo obtener una respuesta válida de Mercado Pago.") from None


def write_private(prefix, content):
    fd, filename = tempfile.mkstemp(prefix=prefix, dir=DIRECTORY)
    path = Path(filename)
    try:
        with os.fdopen(fd, "w") as handle:
            handle.write(content)
            handle.flush()
            os.fsync(handle.fileno())
    except BaseException:
        path.unlink(missing_ok=True)
        raise
    return path


def check(values):
    app_id = validate(values)
    signature_path = values.get("MERCADOPAGO_WEBHOOK_SECRET_FILE", "")
    if not signature_path or len(read_private(Path(signature_path)).strip()) < 32:
        raise SetupError("Falta un archivo válido con la firma de Webhooks.")
    print(f"Configuración en disco válida para la aplicación {app_id}, en modo prueba.")


def setup(previous):
    try:
        tty_input = open("/dev/tty", "r")
    except OSError:
        raise SetupError("Ejecuta el script desde una terminal interactiva.") from None
    with tty_input, open("/dev/tty", "w", buffering=1) as tty_prompt:
        current = previous.get("MERCADOPAGO_CLIENT_ID", "")
        default = f" [Enter = {current}]" if APP_ID.fullmatch(current) else ""
        tty_prompt.write(f"Client ID de la aplicación Checkout Pro{default}: ")
        tty_prompt.flush()
        entered = tty_input.readline()
        if not entered:
            raise EOFError
        app_id = entered.strip() or current
        if not APP_ID.fullmatch(app_id):
            raise SetupError("El Client ID debe contener entre 8 y 20 dígitos.")
        print(f"Ingresa las credenciales de la aplicación {app_id}.", flush=True)
        client_secret = getpass.getpass("Client Secret OAuth: ", stream=tty_prompt).strip()
        token = getpass.getpass("Access Token de la pestaña Prueba: ", stream=tty_prompt).strip()
        signature = getpass.getpass("Firma secreta de Webhooks de esa aplicación: ", stream=tty_prompt).strip()

    values = {
        "MERCADOPAGO_CLIENT_ID": app_id,
        "MERCADOPAGO_CLIENT_SECRET": client_secret,
        "MERCADOPAGO_OAUTH_REDIRECT_URI": CALLBACK,
        "MERCADOPAGO_TEST_MODE": "true",
        "MERCADOPAGO_TOKEN_ENCRYPTION_KEY": previous.get("MERCADOPAGO_TOKEN_ENCRYPTION_KEY") or base64.b64encode(secrets.token_bytes(32)).decode(),
        "MERCADOPAGO_TEST_ACCESS_TOKEN": token,
    }
    validate(values)
    if not re.fullmatch(r"[!-~]{32,512}", signature):
        raise SetupError("La firma secreta de Webhooks está vacía o incompleta.")

    request("/oauth/token", "Client ID y Client Secret", form={
        "grant_type": "client_credentials", "client_id": app_id, "client_secret": client_secret,
    })
    print("Mercado Pago aceptó el par Client ID y Client Secret.")
    if request("/users/me", "Access Token", token=token).get("site_id") != "MLC":
        raise SetupError("El Access Token no corresponde a una cuenta de Chile (MLC).")
    print("Mercado Pago aceptó el Access Token para una cuenta de Chile.")

    DIRECTORY.mkdir(mode=0o700, parents=True, exist_ok=True)
    webhook_file = write_private(f"mercadopago-webhook-{app_id}-", signature + "\n")
    try:
        values["MERCADOPAGO_WEBHOOK_SECRET_FILE"] = str(webhook_file)
        candidate = write_private(".mp-config-", "".join(f"{key}={value}\n" for key, value in values.items()))
        try:
            os.replace(candidate, CONFIG)
        finally:
            candidate.unlink(missing_ok=True)
    except BaseException:
        webhook_file.unlink(missing_ok=True)
        raise
    print(f"Configuración guardada para {app_id}. Reinicia Go y Next para aplicarla.")


def main():
    parser = argparse.ArgumentParser(description="Configura Checkout Pro para pruebas locales.")
    modes = parser.add_mutually_exclusive_group()
    modes.add_argument("--check", action="store_true", help="Valida la configuración local sin modificarla.")
    modes.add_argument("--urls", action="store_true", help="Muestra los enlaces públicos del túnel.")
    args = parser.parse_args()
    if args.check:
        check(read_config())
    elif not args.urls:
        setup(read_config())
    print(f"Callback OAuth: {CALLBACK}")
    print(f"Webhooks → Prueba → Pagos: {WEBHOOK}")
    print(f"Vincular anfitrión: {ORIGIN}/anfitrion/mercado-pago")


try:
    main()
except SetupError as error:
    raise SystemExit(f"Error: {error}") from None
except (KeyboardInterrupt, EOFError):
    raise SystemExit("Configuración cancelada.") from None
except OSError:
    raise SystemExit("Error: no se pudo leer o guardar la configuración privada local.") from None
PY
