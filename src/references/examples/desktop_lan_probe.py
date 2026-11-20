#!/usr/bin/env python3
"""Independent host client for the official Desktop LAN presentation contract.

Requires Python cryptography. Pairing creates a private key file; never publish
it. Protocol adapted from HachimoDock pc/src-tauri/src/lan_agent.rs at 165401b,
Copyright (C) 2026 Yizheng Wang; upstream LICENSE applies to derived portions.
"""
import argparse
import hashlib
import hmac
import json
import os
from pathlib import Path
import secrets
import socket
import struct
import time

from cryptography.hazmat.primitives.ciphers.aead import AESGCM

PRIME = int("""
ffffffffffffffffc90fdaa22168c234c4c6628b80dc1cd129024e088a67cc74
020bbea63b139b22514a08798e3404ddef9519b3cd3a431b302b0a6df25f1437
4fe1356d6d51c245e485b576625e7ec6f44c42e9a637ed6b0bff5cb6f406b7ed
ee386bfb5a899fa5ae9f24117c4b1fe649286651ece45b3dc2007cb8a163bf05
98da48361c55d39a69163fa8fd24cf5f83655d23dca3ad961c62f356208552bb
9ed529077096966d670c354e4abc9804f1746c08ca18217c32905e462e36ce3b
e39e772c180e86039b2783a2ec07a28fb5c55df06f4c52c9de2bcbf695581718
3995497cea956ae515d2261898fa051015728e5a8aaac42dad33170d04507a33
a85521abdf1cba64ecfb850458dbef0a8aea71575d060c7db3970f85a6e1e4c7
abf5ae8cdb0933d71e8c94e04a25619dcee3d2261ad2ee6bf12ffa06d98a0864
d87602733ec86a64521f2b18177b200cbbe117577a615d6c770988c0bad946e2
08e24fa074e5ab3143db5bfce0fd108e4b82d120a93ad2caffffffffffffffff
""".replace("\n", ""), 16)


def sha(*parts):
    return hashlib.sha512(b"".join(parts)).digest()


def encoded(value, width=None):
    return value.to_bytes(width or max(1, (value.bit_length() + 7) // 8), "big")


def read_exact(stream, size):
    data = bytearray()
    while len(data) < size:
        chunk = stream.recv(size - len(data))
        if not chunk:
            raise EOFError("connection closed")
        data.extend(chunk)
    return bytes(data)


def connect(host, port):
    stream = socket.create_connection((host, port), 10)
    stream.settimeout(10)
    client = secrets.token_bytes(16)
    stream.sendall(client)
    server = read_exact(stream, 16)
    return stream, b"P4LAN1" + client + server


def pair(host, port):
    while True:
        private = int.from_bytes(secrets.token_bytes(32), "big") | (1 << 255)
        public = encoded(pow(5, private, PRIME))
        if len(public) == 384 and public[0] < 128:
            break
    stream, transcript = connect(host, port)
    with stream:
        stream.sendall(struct.pack(">BH", 5, len(public)) + public)
        salt = read_exact(stream, 16)
        size, = struct.unpack(">H", read_exact(stream, 2))
        if not 1 <= size <= 384:
            raise ValueError("invalid server public key")
        peer = read_exact(stream, size)
        peer_value = int.from_bytes(peer, "big")
        if not 0 < peer_value < PRIME:
            raise ValueError("invalid SRP server key")
        k = int.from_bytes(sha(encoded(PRIME), encoded(5, 384)), "big")
        u = int.from_bytes(sha(public, encoded(peer_value, 384)), "big")
        x = int.from_bytes(sha(salt, sha(b"Hachimodock:Hachimodock-auto-v1")), "big")
        base = (peer_value - k * pow(5, x, PRIME)) % PRIME
        secret = sha(encoded(pow(base, private + u * x, PRIME)))
        group = bytes(a ^ b for a, b in zip(sha(encoded(PRIME)), sha(encoded(5, 384))))
        proof = sha(group, sha(b"Hachimodock"), salt, public, peer, secret)
        stream.sendall(proof)
        if not hmac.compare_digest(read_exact(stream, 64), sha(public, proof, secret)):
            raise ValueError("server proof rejected")
    return hmac.digest(secret, transcript, "sha256")[:16]


def exchange(host, port, key, mode, body=b"", corrupt=False):
    stream, transcript = connect(host, port)
    with stream:
        cipher = AESGCM(hmac.digest(key, transcript, "sha256")[:16])
        header = struct.pack(">BH", mode, len(body))
        sealed = cipher.encrypt(bytes(12), body, header)
        if corrupt:
            sealed = sealed[:-1] + bytes([sealed[-1] ^ 1])
        stream.sendall(header + sealed)
        aad, size = header, 0
        if mode == 6:
            length = read_exact(stream, 2)
            size, = struct.unpack(">H", length)
            if not 1 <= size <= 576:
                raise ValueError("invalid status length")
            aad += length
        return cipher.decrypt(b"\x01" + bytes(11), read_exact(stream, size + 16), aad)


def discover(host):
    nonce = secrets.token_hex(16)
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as stream:
        stream.settimeout(5)
        stream.sendto(("HACHI-DISCOVER-1 " + nonce).encode(), (host, 29462))
        data, address = stream.recvfrom(512)
    reply = json.loads(data)
    if reply["nonce"] != nonce or reply["protocol"] != 2 or address[1] != 29462:
        raise ValueError("invalid discovery reply")
    return reply


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True)
    parser.add_argument("--port", type=int, default=29461)
    parser.add_argument("--key-file", type=Path, required=True)
    parser.add_argument("--pair", action="store_true", help="enroll an unpaired device and create key-file")
    parser.add_argument("--skip-discovery", action="store_true", help="test-only: pair through an SSH TCP forward; verify UDP separately")
    parser.add_argument("--state", choices=["idle", "working", "waiting_user", "done", "error"])
    parser.add_argument("--seconds", type=float, default=6)
    parser.add_argument("--tamper", action="store_true", help="verify that a modified tag is rejected")
    args = parser.parse_args()
    if args.pair:
        if args.key_file.exists():
            parser.error("key file already exists")
        if not args.skip_discovery:
            print("Discovery:", json.dumps(discover(args.host), ensure_ascii=False))
        key = pair(args.host, args.port)
        args.key_file.parent.mkdir(parents=True, exist_ok=True)
        with os.fdopen(os.open(args.key_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w") as file:
            file.write(key.hex() + "\n")
    else:
        if args.key_file.stat().st_mode & 0o077:
            parser.error("key file must have mode 600")
        key = bytes.fromhex(args.key_file.read_text().strip())
    status = json.loads(exchange(args.host, args.port, key, 6))
    print("Status:", json.dumps(status, ensure_ascii=False))
    if args.tamper:
        try:
            exchange(args.host, args.port, key, 2, b'{"source":"codex","status":{"state":"error"}}', True)
        except (EOFError, OSError):
            print("Modified authentication tag rejected")
        else:
            raise AssertionError("modified tag was accepted")
    if args.state:
        end = time.monotonic() + args.seconds
        revision = int(time.time() * 1000)
        while time.monotonic() < end:
            remaining = max(0, int((end - time.monotonic()) * 1000)) if args.state in ("done", "error") else 0
            body = {"source": "codex", "status": {"state": args.state}, "sessions": {
                "agentId": "codex", "sessionId": "lan-probe", "displayEnabled": True,
                "activeSessionIds": ["lan-probe"] if args.state in ("working", "waiting_user") else [],
                "sessions": [] if args.state == "idle" else [{"id": "lan-probe", "title": "Wi-Fi 协议验证", "content": "正在接收 Desktop 格式的会话状态", "state": args.state,
                    "transitionRevision": revision, "terminalRemainingMs": remaining}]}}
            exchange(args.host, args.port, key, 2, json.dumps(body, ensure_ascii=False).encode())
            time.sleep(min(2, max(0, end - time.monotonic())))
        print("Snapshot delivery verified:", args.state)


if __name__ == "__main__":
    main()
