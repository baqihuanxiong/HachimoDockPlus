#!/usr/bin/env python3
"""Loopback-only gateway fixture for board UI checks; contains synthetic users only.

Forward with ssh -R 18081:127.0.0.1:18081, and point a temporary board config
at http://127.0.0.1:18081. Never proxy reset requests to a real gateway.
"""
import argparse
from datetime import datetime, timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import threading
from urllib.parse import urlparse


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=18081)
    parser.add_argument("--state", type=Path, required=True)
    args = parser.parse_args()
    lock = threading.Lock()
    users = {i: {"id": i, "role": 100 if i == 1 else 1, "quota": i * 500000}
             for i in range(1, 4)}
    writes = []

    def persist():
        args.state.write_text(json.dumps({"users": users, "writes": writes}, indent=2) + "\n")

    persist()

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def reply(self, data, status=200):
            body = json.dumps(data).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            path = urlparse(self.path).path
            sampled = datetime.now() - timedelta(seconds=5)
            now = int(sampled.timestamp())
            if path == "/api/status":
                data = {"quota_per_unit": 500000}
            elif path == "/api/log/":
                rows = []
                for i in range(7):
                    rows.append({"id": i + 1, "user_id": 2, "username": "fixture-alice",
                                 "created_at": int((sampled - timedelta(days=i)).timestamp()),
                                 "type": 2, "model_name": "fixture-qwen", "quota": (i + 1) * 500000,
                                 "channel": 1})
                rows.append({"id": 8, "user_id": 3, "username": "fixture-bob",
                             "created_at": now, "type": 5, "model_name": "fixture-mimo", "channel": 2})
                data = {"total": len(rows), "items": rows}
            elif path == "/api/channel/":
                rows = [{"id": 1, "name": "Fixture Qwen", "status": 1, "response_time": 120},
                        {"id": 2, "name": "Fixture MiMo", "status": 1, "response_time": 320},
                        {"id": 3, "name": "Fixture Disabled", "status": 2}]
                data = {"total": len(rows), "items": rows}
            elif path == "/api/user/self":
                data = users[1]
            elif path == "/api/user/":
                data = {"total": len(users), "items": list(users.values())}
            elif path.startswith("/api/user/") and path.rsplit("/", 1)[-1].isdigit():
                data = users.get(int(path.rsplit("/", 1)[-1]))
            elif path == "/v1/platform/token_plan":
                self.reply({"object": "token_plan", "plan_code": "fixture",
                            "quota": {"total_credits": 1000, "used_credits": 250, "remaining_credits": 750},
                            "period": {"end_at": "2026-12-15 23:59:59"}})
                return
            else:
                self.reply({"success": False}, 404)
                return
            self.reply({"success": True, "data": data})

        def do_POST(self):
            if urlparse(self.path).path != "/api/user/manage":
                self.reply({"success": False}, 404)
                return
            length = int(self.headers.get("Content-Length", "0"))
            if not 0 < length < 4096:
                self.reply({"success": False}, 400)
                return
            payload = json.loads(self.rfile.read(length))
            with lock:
                if (payload.get("id") not in users or payload.get("action") != "add_quota"
                        or payload.get("mode") != "override" or payload.get("value") != 100000000):
                    self.reply({"success": False}, 400)
                    return
                users[payload["id"]]["quota"] = payload["value"]
                writes.append(payload)
                persist()
            self.reply({"success": True})

    server = ThreadingHTTPServer(("127.0.0.1", args.port), Handler)
    print(f"Synthetic NewAPI fixture listening on loopback:{args.port}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
