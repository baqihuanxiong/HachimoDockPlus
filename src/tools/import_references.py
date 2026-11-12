#!/usr/bin/env python3
"""Import the user-provided local credentials without printing their values."""
import json
import os
import re
from pathlib import Path

root = Path(__file__).resolve().parents[1]
sections = {}
section = ""
for line in (root / "references/secret.md").read_text().splitlines():
    if line.startswith("## "):
        section = line[3:].strip().lower()
        sections[section] = {}
    match = re.match(r"\s*(?:[-*]\s*)?(URL|API Key|Model)\s*[:：]\s*(.+)", line, re.I)
    if section and match:
        sections[section][match[1].lower()] = match[2].strip().strip("` ")

local = root / ".local"
secrets = local / "secrets"
secrets.mkdir(parents=True, exist_ok=True, mode=0o700)
os.chmod(local, 0o700)
os.chmod(secrets, 0o700)
cfg = json.loads((root / "config.example.json").read_text())
cfg["data_dir"] = "data"
cfg["ui"]["font"] = "assets/wqy-microhei.ttc"
cfg["apps"]["picoclaw"]["tts"]["prompt_wav"] = "assets/reference.wav"
# Retain the original workspace location when refreshing a legacy installation.
existing_path = local / "config.json"
if existing_path.exists():
    existing = json.loads(existing_path.read_text())
    previous_app = existing.get("apps", {}).get("picoclaw", {})
    previous_data = previous_app.get("data_dir")
    if "llm" in existing:
        previous_data = existing.get("data_dir")
    if previous_data:
        cfg["apps"]["picoclaw"]["data_dir"] = previous_data

defaults = {"llm": "https://newapi.zi-cloud.net/v1", "asr": "wss://funasr-nano.zi-cloud.net", "tts": "https://cosyvoice.zi-cloud.net"}
for name in defaults:
    values = sections.get(name, {})
    key = values.get("api key", "")
    if not key or "\n" in key:
        raise SystemExit(f"Missing {name} API key in local references")
    path = secrets / (name + ".key")
    path.write_text(key + "\n")
    path.chmod(0o600)
    cfg["apps"]["picoclaw"][name]["key_file"] = "secrets/" + path.name
    cfg["apps"]["picoclaw"][name]["url"] = values.get("url", "").strip() or defaults[name]
cfg["apps"]["picoclaw"]["llm"]["model"] = sections.get("llm", {}).get("model", "")
if not cfg["apps"]["picoclaw"]["llm"]["model"]:
    raise SystemExit("Missing LLM model in local references")
output = local / "config.json"
output.write_text(json.dumps(cfg, ensure_ascii=False, indent=2) + "\n")
output.chmod(0o600)
print("Created ignored .local/config.json and private credential files (values not displayed).")
