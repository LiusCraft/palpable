#!/usr/bin/env python3
"""Exercise the firmware C++ client against a running Palpable /ws gateway."""
import argparse
import base64
from contextlib import contextmanager
import hashlib
import json
import os
from pathlib import Path
import socket
import struct
import subprocess
import tempfile
import time
from urllib.parse import urlparse
from urllib.request import urlopen


def read_exact(connection, length):
    data = bytearray()
    while len(data) < length:
        chunk = connection.recv(length - len(data))
        if not chunk:
            raise RuntimeError("Gateway disconnected")
        data.extend(chunk)
    return bytes(data)


def send(connection, opcode, payload):
    header = bytes([0x80 | opcode])
    length = len(payload)
    if length < 126:
        header += bytes([0x80 | length])
    else:
        header += bytes([0x80 | 126]) + struct.pack("!H", length)
    mask = os.urandom(4)
    masked = bytes(value ^ mask[i % 4] for i, value in enumerate(payload))
    connection.sendall(header + mask + masked)


def receive(connection):
    header = read_exact(connection, 2)
    if header[0] & 0x80 == 0 or header[1] & 0x80:
        raise RuntimeError("Unexpected fragmented or masked server frame")
    length = header[1] & 127
    if length == 126:
        length = struct.unpack("!H", read_exact(connection, 2))[0]
    elif length == 127:
        length = struct.unpack("!Q", read_exact(connection, 8))[0]
    if length > 16384:
        raise RuntimeError("Oversized server frame")
    return header[0] & 15, read_exact(connection, length)


@contextmanager
def gateway(server, url):
    if server is None:
        yield url
        return
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        port = reservation.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="palpable-firmware-") as directory:
        config = Path(directory) / "smoke.yaml"
        config.write_text(f'addr: "127.0.0.1:{port}"\ndb_path: ":memory:"\n')
        with (Path(directory) / "server.log").open("w+") as log:
            process = subprocess.Popen([str(server.resolve()), "-config", str(config)], stdout=log, stderr=log)
            try:
                deadline = time.monotonic() + 15
                while True:
                    try:
                        with urlopen(f"http://127.0.0.1:{port}/healthz", timeout=1) as response:
                            if response.status == 200:
                                break
                    except OSError:
                        pass
                    if process.poll() is not None or time.monotonic() >= deadline:
                        log.seek(0)
                        raise RuntimeError("Smoke gateway failed to start: " + log.read())
                    time.sleep(0.05)
                yield f"ws://127.0.0.1:{port}/ws"
            finally:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()


def probe(probe_path, address):
    url = urlparse(address)
    if url.scheme != "ws" or not url.hostname:
        raise RuntimeError("Smoke test expects a local ws:// gateway")
    with socket.create_connection((url.hostname, url.port or 80), timeout=10) as connection:
        key = base64.b64encode(os.urandom(16)).decode()
        request = (f"GET {url.path or '/ws'} HTTP/1.1\r\nHost: {url.netloc}\r\n"
                   f"Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\n"
                   "Sec-WebSocket-Version: 13\r\n\r\n")
        connection.sendall(request.encode())
        response = bytearray()
        while not response.endswith(b"\r\n\r\n"):
            response.extend(read_exact(connection, 1))
            if len(response) > 4096:
                raise RuntimeError("Oversized upgrade response")
        expected = base64.b64encode(hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest()).decode()
        if not response.startswith(b"HTTP/1.1 101 ") or expected.encode() not in response:
            raise RuntimeError(f"WebSocket upgrade failed: {response!r}")
        process = subprocess.Popen([str(probe_path)], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
        try:
            hello = process.stdout.readline().strip()
            if json.loads(hello)["type"] != "hello":
                raise RuntimeError("Firmware probe did not emit hello")
            send(connection, 1, hello.encode())
            opcode, ack = receive(connection)
            if opcode != 1 or json.loads(ack)["type"] != "hello.ack":
                raise RuntimeError(f"Gateway rejected firmware handshake: {ack!r}")
            outgoing, _ = process.communicate(ack.decode() + "\n", timeout=10)
            if process.returncode:
                raise RuntimeError(f"Firmware rejected gateway ack: exit {process.returncode}")
            for line in outgoing.splitlines():
                if line == "audio":
                    send(connection, 2, b"\xfb\x03\xff\xfe\xff\xfe\xff\xfe")
                else:
                    send(connection, 1, line.encode())
            # Pong is a barrier: the gateway has read all preceding firmware frames.
            send(connection, 9, b"palpable-smoke")
            while True:
                opcode, payload = receive(connection)
                if opcode == 10 and payload == b"palpable-smoke":
                    break
                if opcode == 9:
                    send(connection, 10, payload)
                elif opcode == 8:
                    raise RuntimeError("Gateway closed the session")
                elif opcode == 1:
                    message = json.loads(payload)
                    if message.get("code") in ["bad_message", "unsupported"]:
                        raise RuntimeError(f"Gateway rejected firmware message: {message}")
            send(connection, 8, struct.pack("!H", 1000))
            print("Real gateway accepted firmware hello, wake, mic start, binary Opus and mic stop")
        finally:
            if process.poll() is None:
                process.kill()
            process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="ws://127.0.0.1:8080/ws")
    parser.add_argument("--probe", type=Path, default=Path(__file__).resolve().parents[1] / ".build/host/gateway_probe")
    parser.add_argument("--server-bin", type=Path, help="Launch a temporary gateway using this server binary")
    args = parser.parse_args()
    with gateway(args.server_bin, args.url) as address:
        probe(args.probe, address)


if __name__ == "__main__":
    main()
