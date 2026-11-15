#!/usr/bin/env python3
"""Host-side UI probe for the current Lyra Zero W profile; standard library only.

Input injection is deliberately limited to verified ARMv7 evdev and MT devices.
It does not emulate GPIO keys or certify physical touch-panel operation.
"""

import argparse
from pathlib import Path
import re
import shlex
import struct
import subprocess
import sys
import zlib


def remote(host, command, data=None):
    result = subprocess.run(
        ["ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", host, command],
        input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30,
    )
    if result.returncode:
        raise RuntimeError(result.stderr.decode(errors="replace").strip() or "SSH failed")
    return result.stdout


def point(x, y):
    if not (0 <= x < 640 and 0 <= y < 480):
        raise ValueError("logical coordinates must be within 640x480")
    return 479 - y, x


def event(kind, code, value):
    return struct.pack("<llHHi", 0, 0, kind, code, value)


def input_script(points, device, interval):
    if not re.fullmatch(r"/dev/input/event\d+", device):
        raise ValueError("--device must be /dev/input/eventN")
    if not points or not 0.01 <= interval <= 5:
        raise ValueError("input requires points and an interval from 0.01 to 5 seconds")

    def write(payload):
        octal = "".join("\\%03o" % byte for byte in payload)
        return "printf '%b' " + shlex.quote(octal) + " > " + shlex.quote(device)

    release = event(3, 0x2F, 0) + event(3, 0x39, -1) + event(0, 0, 0)
    lines = ["set -eu", "trap " + shlex.quote(write(release)) + " EXIT",
             "trap 'exit 130' HUP INT TERM"]
    for index, (x, y) in enumerate(points):
        raw_x, raw_y = point(x, y)
        payload = event(3, 0x2F, 0)
        if index == 0:
            payload += event(3, 0x39, 19)
        payload += event(3, 0x35, raw_x) + event(3, 0x36, raw_y) + event(0, 0, 0)
        lines.extend([write(payload), "sleep %.4f" % interval])
    # EXIT releases on success as well as on a shell error or handled signal.
    return "\n".join(lines) + "\n"


def touch_identity(devices, device):
    number = device.rsplit("/", 1)[-1]
    for block in devices.split("\n\n"):
        handlers = re.search(r"^H: Handlers=(.*)$", block, re.M)
        if not handlers or number not in handlers.group(1).split():
            continue
        name = re.search(r'^N: Name="(.*)"$', block, re.M)
        absolute = re.search(r"^B: ABS=(.*)$", block, re.M)
        if not name or not absolute:
            break
        bits = 0
        for word in absolute.group(1).split():
            value = int(word, 16)
            if value > 0xFFFFFFFF:
                raise ValueError("unexpected evdev bitmap word size")
            bits = (bits << 32) | value
        if not all(bits & (1 << code) for code in (0x2F, 0x35, 0x36, 0x39)):
            break
        if not any(token in name.group(1).lower() for token in ("ft5x06", "cst826", "focaltech")):
            raise ValueError("touch-device name differs from this board profile")
        return name.group(1)
    raise ValueError("selected device is not the verified multitouch device")


def check_framebuffer(host):
    values = remote(host, "cat /sys/class/graphics/fb0/virtual_size; "
                    "cat /sys/class/graphics/fb0/bits_per_pixel; "
                    "cat /sys/class/graphics/fb0/stride").decode().splitlines()
    if values != ["480,640", "32", "1920"]:
        raise ValueError("framebuffer differs from the 480x640, 32-bpp board profile")


def framebuffer_png(raw):
    if len(raw) != 480 * 640 * 4:
        raise ValueError("unexpected framebuffer byte count")
    rows = []
    for y in range(480):
        row = bytearray([0])  # PNG filter: None.
        for x in range(640):
            offset = (x * 480 + 479 - y) * 4
            row.extend(raw[offset:offset + 3][::-1])
        rows.append(row)

    def chunk(kind, data):
        return (struct.pack(">I", len(data)) + kind + data
                + struct.pack(">I", zlib.crc32(kind + data)))

    return (b"\x89PNG\r\n\x1a\n"
            + chunk(b"IHDR", struct.pack(">IIBBBBB", 640, 480, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"".join(rows))) + chunk(b"IEND", b""))


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", required=True, help="SSH host or user@host")
    parser.add_argument("--device", help="verified /dev/input/eventN; required for input")
    parser.add_argument("--dry-run", action="store_true", help="print input script without SSH")
    commands = parser.add_subparsers(dest="command", required=True)
    screen = commands.add_parser("screenshot")
    screen.add_argument("output", type=Path, help="new PNG path; existing evidence is not overwritten")
    tap = commands.add_parser("tap")
    tap.add_argument("x", type=int)
    tap.add_argument("y", type=int)
    swipe = commands.add_parser("swipe")
    for name in ("x", "y", "end_x", "end_y"):
        swipe.add_argument(name, type=int)
    swipe.add_argument("--duration", type=float, default=0.4)
    swipe.add_argument("--steps", type=int, default=12)
    args = parser.parse_args(argv)
    if not args.host or args.host.startswith("-") or any(c.isspace() for c in args.host):
        parser.error("invalid SSH host")
    if args.command == "screenshot":
        if args.dry_run:
            parser.error("--dry-run is for input commands")
        check_framebuffer(args.host)
        data = framebuffer_png(remote(args.host, "cat /dev/fb0"))
        args.output.parent.mkdir(parents=True, exist_ok=True)
        with args.output.open("xb") as output:
            output.write(data)
        print(args.output)
        return 0

    if not args.device:
        parser.error("input requires --device selected from /proc/bus/input/devices")
    if args.command == "tap":
        points, interval = [(args.x, args.y)], 0.08
    else:
        if not 1 <= args.steps <= 120 or not 0.05 <= args.duration <= 5:
            parser.error("swipe requires 1..120 steps and 0.05..5 seconds")
        point(args.end_x, args.end_y)
        points = [(round(args.x + (args.end_x - args.x) * i / args.steps),
                   round(args.y + (args.end_y - args.y) * i / args.steps))
                  for i in range(args.steps + 1)]
        interval = args.duration / (args.steps + 1)
    script = input_script(points, args.device, interval)
    if args.dry_run:
        print(script, end="")
        return 0
    architecture = remote(args.host, "uname -m").decode().strip()
    if architecture != "armv7l":
        raise ValueError("16-byte evdev injection is restricted to the verified armv7l profile")
    check_framebuffer(args.host)
    identity = touch_identity(remote(args.host, "cat /proc/bus/input/devices").decode(), args.device)
    print("Touch:", identity)
    remote(args.host, "sh -s", script.encode())
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        print("error:", error, file=sys.stderr)
        sys.exit(1)
