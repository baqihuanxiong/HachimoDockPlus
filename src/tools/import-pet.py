#!/usr/bin/env python3
"""Prepare an offline MP4 pet pack for the existing SMB assets workflow.

Input: an exported official appearance folder (videos/*.mp4) or a flat folder.
No credentials, original photos or desktop-local paths are copied.
"""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path, nargs="?", help="exported appearance directory")
    parser.add_argument("--builtin", action="store_true", help="use the pinned upstream Westie test clips")
    parser.add_argument("--output", type=Path, required=True, help="new pack directory; never overwrites an existing pack")
    parser.add_argument("--name", default="我的桌宠")
    parser.add_argument("--copy", action="store_true", help="copy already-normalized MP4 files without FFmpeg")
    args = parser.parse_args()
    project = Path(__file__).resolve().parents[2]
    if args.builtin == bool(args.source):
        parser.error("choose a source directory or --builtin")
    source = (project / "HachimoDock/pc/public/terrier-clips" if args.builtin else args.source).resolve()
    output = args.output.absolute()
    if output.exists() or output.is_symlink():
        parser.error("output already exists; choose a new directory")
    clips = {}
    for folder in (source, source / "videos"):
        if folder.is_symlink():
            parser.error("video directories cannot be symlinks")
        if not folder.is_dir():
            continue
        for path in sorted(folder.iterdir()):
            if path.suffix.lower() != ".mp4" or path.name.startswith("."):
                continue
            if path.is_symlink() or not path.is_file() or not re.fullmatch(r"[a-z][a-z0-9_.-]{0,63}", path.stem):
                parser.error("invalid video file")
            if path.stem in clips:
                parser.error("duplicate state video")
            clips[path.stem] = path
    if not 1 <= len(clips) <= 64:
        parser.error("a pack must contain 1 to 64 state videos")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".pet-import-", dir=output.parent) as temp:
        stage = Path(temp)
        (stage / "videos").mkdir()
        for state, path in clips.items():
            target = stage / "videos" / f"{state}.mp4"
            if args.copy:
                shutil.copyfile(path, target)
            else:
                native = shutil.which("ffmpeg")
                command = [native] if native else [
                    "docker", "run", "--rm", "--user", f"{os.getuid()}:{os.getgid()}",
                    "-v", f"{source}:/input:ro", "-v", f"{stage}:/output", "hachimodock-go-lvgl:dev", "ffmpeg",
                ]
                input_path = str(path) if native else "/input/" + str(path.relative_to(source))
                output_path = str(target) if native else "/output/videos/" + target.name
                command += ["-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-i", input_path,
                            "-an", "-sn", "-dn", "-vf", "fps=15,scale=640:480:force_original_aspect_ratio=decrease,pad=640:480:(ow-iw)/2:(oh-ih)/2:black",
                            "-c:v", "libx264", "-preset", "fast", "-crf", "27", "-profile:v", "baseline", "-pix_fmt", "yuv420p",
                            "-movflags", "+faststart", output_path]
                subprocess.run(command, check=True)
            if not 0 < target.stat().st_size <= 16 << 20:
                parser.error("a video exceeds the 16 MiB SMB resource limit")
        metadata = {"format": "local-pet-v1", "name": "西高地" if args.builtin else args.name,
                    "families": [{"family": state, "file": f"videos/{state}.mp4"} for state in clips]}
        (stage / "manifest.json").write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n")
        if args.builtin:
            shutil.copyfile(project / "HachimoDock/LICENSE", stage / "HACHIMODOCK-LICENSE.txt")
            revision = subprocess.check_output(["git", "-C", str(project / "HachimoDock"), "rev-parse", "HEAD"], text=True).strip()
            (stage / "SOURCE.txt").write_text(f"HachimoDock, Copyright (C) 2026 Yizheng Wang\nhttps://github.com/YizhengWw/HachimoDock\nCommit: {revision}\nOriginal: pc/public/terrier-clips/*.mp4\nConverted to 640x480 / 15 fps H.264 MP4; audio omitted.\nUpstream noncommercial license applies; not covered by this project's MIT license.\n")
        size = sum(path.stat().st_size for path in stage.rglob("*") if path.is_file())
        if size > 24 << 20:
            parser.error("pack exceeds 24 MiB; SMB assets must also have room for fonts and other resources")
        os.rename(stage, output)
    print(f"Prepared {len(clips)} states ({size / (1 << 20):.1f} MiB): {output}")
    print("Copy into SMB assets/pets/, set ui.pet.directory, then apply configuration and restart.")


if __name__ == "__main__":
    main()
