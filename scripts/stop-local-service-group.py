#!/usr/bin/env python3
"""Stop one listener only when its isolated process group belongs to EspaciGo."""

from __future__ import annotations

import os
import re
import signal
import subprocess
import sys
import time
from pathlib import Path


def fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def process_identity(pid: int) -> tuple[int, int, int, str, str] | None:
    process = Path("/proc") / str(pid)
    try:
        stat = (process / "stat").read_text()
        fields = stat[stat.rfind(")") + 2 :].split()
        uid = int((process / "status").read_text().split("Uid:", 1)[1].split()[0])
        cwd = str((process / "cwd").resolve())
        command = (process / "cmdline").read_bytes().replace(b"\0", b" ").decode(errors="replace")
        comm = (process / "comm").read_text().strip()
        return uid, int(fields[2]), int(fields[3]), cwd, f"{comm} {command}"
    except (FileNotFoundError, PermissionError, IndexError, ValueError):
        return None


def listen_pids(port: int) -> set[int]:
    result = subprocess.run(
        ["ss", "-H", "-ltnp", "sport", "=", f":{port}"],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode not in (0, 1):
        fail("no se pudo revisar el listener TCP del servicio local")
    return {int(pid) for pid in re.findall(r"pid=(\d+)", result.stdout)}


def session_processes(sid: int) -> dict[int, tuple[int, int, int, str, str]]:
    members: dict[int, tuple[int, int, int, str, str]] = {}
    pids: set[int] = set()
    for child in Path("/proc").iterdir():
        if child.name.isdigit():
            identity = process_identity(int(child.name))
            if identity and identity[2] == sid:
                members[int(child.name)] = identity
    return members


def scoped_session(port: int, service: str, repo: Path) -> tuple[int, set[int]] | None:
    listeners = listen_pids(port)
    if not listeners:
        return None

    sessions: set[int] = set()
    for pid in listeners:
        identity = process_identity(pid)
        if identity is None:
            fail(f"el proceso que ocupa {port} cambió durante la comprobación")
        uid, pgid, sid, cwd, command = identity
        expected_cwd = repo / "frontend" if service == "Web" else repo
        if uid != os.getuid() or Path(cwd) != expected_cwd and expected_cwd not in Path(cwd).parents:
            fail(f"el listener {port} no pertenece a este checkout de EspaciGo")
        if service == "Web" and not any(marker in command for marker in ("next", "server.js")):
            fail("el listener web no parece ser el servidor Next de este checkout")
        if service == "API" and "api" not in command.lower():
            fail("el listener API no parece ser el backend Go de este checkout")
        sessions.add(sid)

    if len(sessions) != 1:
        fail(f"el puerto {port} tiene más de una sesión de procesos")
    sid = sessions.pop()
    members = session_processes(sid)
    if not listeners.issubset(members):
        fail(f"la sesión del listener {port} cambió durante la comprobación")
    groups: set[int] = set()
    for uid, pgid, member_sid, cwd, _ in members.values():
        cwd_path = Path(cwd)
        if (
            uid != os.getuid()
            or member_sid != sid
            or (cwd_path != repo and repo not in cwd_path.parents)
        ):
            fail(f"la sesión del listener {port} incluye procesos fuera del checkout")
        groups.add(pgid)
    return sid, groups


def stop(port: int, service: str, repo: Path) -> None:
    session = scoped_session(port, service, repo)
    if session is None:
        print(f"{service}: el puerto {port} ya está libre.")
        return

    sid, groups = session
    for pgid in groups:
        os.killpg(pgid, signal.SIGTERM)
    for _ in range(20):
        if not listen_pids(port):
            print(f"{service}: listener del puerto {port} detenido de forma ordenada.")
            return
        time.sleep(0.5)

    # Revalidate the same isolated group before a final termination signal.
    if scoped_session(port, service, repo) != session:
        fail(f"el servicio {service} no se detuvo y su grupo cambió")
    for pgid in groups:
        os.killpg(pgid, signal.SIGKILL)
    for _ in range(10):
        if not listen_pids(port):
            print(f"{service}: listener del puerto {port} detenido tras agotar SIGTERM.")
            return
        time.sleep(0.5)
    fail(f"el puerto {port} continúa ocupado después de detener {service}")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        fail("uso: stop-local-service-group.py <puerto> <API|Web> <raíz-del-checkout>")
    stop(int(sys.argv[1]), sys.argv[2], Path(sys.argv[3]).resolve())
