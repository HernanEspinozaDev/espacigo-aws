#!/usr/bin/env python3
"""Explicit local transport to one pinned Docker PostgreSQL Unix socket.

This does not select a database, grant authority, read credentials or execute SQL.
Keep it alive only for the operation and pin cluster/database identity in the client.
"""
import argparse
import json
import re
import socket
import socketserver
import subprocess
import threading

PERL_RELAY = r"""
use strict; use warnings; use IO::Socket::UNIX; use IO::Select;
my $s = IO::Socket::UNIX->new(Type => SOCK_STREAM,
    Peer => '/var/run/postgresql/.s.PGSQL.5432') or die "socket unavailable\n";
binmode STDIN; binmode STDOUT;
my $set = IO::Select->new(\*STDIN, $s);
while (my @ready = $set->can_read) {
    for my $src (@ready) {
        my $n = sysread($src, my $buffer, 65536);
        exit 0 unless defined($n) && $n > 0;
        my $dst = fileno($src) == fileno(STDIN) ? $s : \*STDOUT;
        my $offset = 0;
        while ($offset < $n) {
            my $written = syswrite($dst, $buffer, $n - $offset, $offset);
            exit 1 unless defined($written) && $written > 0;
            $offset += $written;
        }
    }
}
"""


def validate_container(container_id):
    if not re.fullmatch(r"[0-9a-f]{64}", container_id):
        raise ValueError("a full lowercase container ID is required")
    result = subprocess.run(
        ["docker", "inspect", "--format", "{{json .State}}", container_id],
        check=True, capture_output=True, text=True,
    )
    if not json.loads(result.stdout).get("Running"):
        raise ValueError("pinned container is not running")


def handler_for(container_id):
    class Relay(socketserver.BaseRequestHandler):
        def handle(self):
            process = subprocess.Popen(
                ["docker", "exec", "-i", "--user", "postgres", container_id,
                 "perl", "-e", PERL_RELAY],
                stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL, bufsize=0,
            )
            assert process.stdin is not None and process.stdout is not None
            upstream, downstream = process.stdin, process.stdout

            def upload():
                try:
                    while data := self.request.recv(65536):
                        view = memoryview(data)
                        while view:
                            n = upstream.write(view)
                            if not n:
                                return
                            view = view[n:]
                except (OSError, ValueError):
                    pass
                finally:
                    try:
                        upstream.close()
                    except OSError:
                        pass

            sender = threading.Thread(target=upload, daemon=True)
            sender.start()
            try:
                while data := downstream.read(65536):
                    self.request.sendall(data)
            except OSError:
                pass
            finally:
                try:
                    self.request.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                sender.join(timeout=5)
                downstream.close()
    return Relay


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--container-id", required=True)
    parser.add_argument("--port", type=int, default=0)
    args = parser.parse_args()
    validate_container(args.container_id)
    with socketserver.ThreadingTCPServer(
        ("127.0.0.1", args.port), handler_for(args.container_id)
    ) as server:
        server.daemon_threads = True
        print(json.dumps({"host": "127.0.0.1", "port": server.server_address[1],
                          "container_id": args.container_id}), flush=True)
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
