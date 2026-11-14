#!/usr/bin/env python3
"""Replace both Lyra FIT/resource DTBs while retaining kernel and logo images.

This supports the inspected Lyra boot layout, not arbitrary signed firmware.
It only writes local output files; it never accesses an MTD device.
"""

import argparse
import hashlib
import json
import struct
from pathlib import Path


def fdt_properties(data):
    """Return (path, property) -> (value, absolute value offset)."""
    if len(data) < 40:
        raise ValueError("Truncated FDT header")
    magic, total, start, strings_start, _, version, _, _, strings_size, size = (
        struct.unpack_from(">10I", data)
    )
    if magic != 0xD00DFEED or version != 17 or total > len(data):
        raise ValueError("Unsupported FDT header")
    if start + size > total or strings_start + strings_size > total:
        raise ValueError("FDT sections exceed header size")
    strings = data[strings_start:strings_start + strings_size]
    offset, stack, properties = start, [], {}
    while offset < start + size:
        token = struct.unpack_from(">I", data, offset)[0]
        offset += 4
        if token == 1:
            end = data.index(0, offset)
            stack.append(data[offset:end].decode())
            offset = (end + 4) & ~3
        elif token == 2:
            stack.pop()
        elif token == 3:
            length, name_offset = struct.unpack_from(">II", data, offset)
            offset += 8
            name = strings[name_offset:strings.index(0, name_offset)].decode()
            if offset + length > start + size:
                raise ValueError("FDT property exceeds structure")
            path = "/" + "/".join(stack[1:])
            properties[path, name] = (data[offset:offset + length], offset)
            offset = (offset + length + 3) & ~3
        elif token == 4:
            continue
        elif token == 9:
            return properties
        else:
            raise ValueError(f"Unexpected FDT token {token}")
    raise ValueError("Missing FDT END token")


def u32(properties, path, name):
    return struct.unpack(">I", properties[path, name][0])[0]


def images(data, properties):
    result = {}
    for name in ("fdt", "kernel", "resource"):
        path = "/images/" + name
        position = u32(properties, path, "data-position")
        length = u32(properties, path, "data-size")
        if position < struct.unpack_from(">I", data, 4)[0]:
            raise ValueError("Expected external FIT image data")
        if position + length > len(data):
            raise ValueError("FIT image data exceeds file")
        payload = data[position:position + length]
        if properties[path + "/hash", "algo"][0] != b"sha256\0":
            raise ValueError("Expected SHA-256 FIT hash")
        digest = hashlib.sha256(payload).digest()
        if digest != properties[path + "/hash", "value"][0]:
            raise ValueError(f"Invalid original {name} hash")
        result[name] = (position, payload, digest.hex())
    return result


def patch_resource(resource, old_dtb, dtb):
    """Update rk-kernel.dtb and its SHA-1; retain every other resource byte."""
    if resource[:4] != b"RSCE" or resource[8:11] != b"\x01\x01\x01":
        raise ValueError("Unsupported Rockchip resource header")
    count = struct.unpack_from("<I", resource, 12)[0]
    entries = []
    logos = {}
    for index in range(count):
        entry = 512 + index * 512
        if resource[entry:entry + 4] != b"ENTR":
            raise ValueError("Invalid resource entry")
        name = resource[entry + 4:entry + 224].split(b"\0", 1)[0].decode()
        hash_size, block, size = struct.unpack_from("<III", resource, entry + 256)
        start = block * 512
        payload = resource[start:start + size]
        if hash_size != 20 or hashlib.sha1(payload).digest() != resource[entry + 224:entry + 244]:
            raise ValueError(f"Invalid resource hash: {name}")
        entries.append((name, entry, start, size))
        if name != "rk-kernel.dtb":
            logos[name] = hashlib.sha256(payload).hexdigest()
    selected = [e for e in entries if e[0] == "rk-kernel.dtb"]
    if len(selected) != 1:
        raise ValueError("Expected one resource DTB")
    _, entry, start, size = selected[0]
    if resource[start:start + size] != old_dtb or len(dtb) != size:
        raise ValueError("Resource DTB differs from FIT DTB or size changed")
    result = bytearray(resource)
    result[start:start + size] = dtb
    result[entry + 224:entry + 244] = hashlib.sha1(dtb).digest()
    for name, _, offset, length in entries:
        if name != "rk-kernel.dtb" and result[offset:offset + length] != resource[offset:offset + length]:
            raise ValueError(f"Logo changed: {name}")
    return result, logos


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("original", type=Path)
    parser.add_argument("dtb", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    original, dtb = args.original.read_bytes(), args.dtb.read_bytes()
    properties = fdt_properties(original)
    if any("signature" in path and name == "value" for path, name in properties):
        raise ValueError("Signed FIT needs signing keys; refusing to modify")
    original_images = images(original, properties)
    replacement = fdt_properties(dtb)
    if struct.unpack_from(">I", dtb, 4)[0] != len(dtb):
        raise ValueError("DTB contains unexpected trailing bytes")

    # Compare the actual old DTB, not the bootloader-modified live device tree.
    old = fdt_properties(original_images["fdt"][1])
    allowed = {
        "/dsi@ff640000": {"rockchip,lane-rate"},
        "/dsi@ff640000/panel@0": {
            "dsi,lanes", "dsi,format", "dsi,flags", "bpc", "bus-format",
            "width-mm", "height-mm", "reset-gpios", "enable-gpios",
            "backlight", "prepare-delay-ms", "init-delay-ms",
            "enable-delay-ms", "panel-init-sequence", "panel-exit-sequence",
        },
        "/dsi@ff640000/panel@0/display-timings/timing0": {
            "clock-frequency", "hactive", "vactive", "hfront-porch",
            "hsync-len", "hback-porch", "vfront-porch", "vsync-len",
            "vback-porch", "hsync-active", "vsync-active", "de-active",
            "pixelclk-active",
        },
        "/pwm@ff932000": {"status"},
        "/i2c@ff060000": {"clock-frequency"},
        "/i2c@ff060000/gt9271@5d": {"status"},
        "/i2c@ff060000/gt911@14": {"status"},
        "/i2c@ff060000/ws-bl@45": {"status"},
        "/i2c@ff060000/ft5406@38": {
            "compatible", "reg", "status", "touchscreen-size-x",
            "touchscreen-size-y", "touchscreen-inverted-x",
            "touchscreen-inverted-y", "touchscreen-swapped-x-y",
            "interrupts", "interrupts-extended", "reset-gpios",
        },
    }
    changes = []
    for path, name in sorted(old.keys() | replacement.keys()):
        before = old.get((path, name), (None,))[0]
        after = replacement.get((path, name), (None,))[0]
        if before == after:
            continue
        if name not in allowed.get(path, set()):
            raise ValueError(f"Unrelated device-tree change: {path}:{name}")
        changes.append(f"{path}:{name}")
    if not changes:
        raise ValueError("No device-tree changes")

    # U-Boot USING_KERNEL_DTB may override FIT fdt with resource rk-kernel.dtb.
    # Keep both copies and their allocated sizes identical; don't relocate data.
    previous_total = u32(properties, "/", "totalsize")
    if previous_total < max(p + len(b) for p, b, _ in original_images.values()):
        raise ValueError("Original FIT totalsize does not cover image data")
    if any(b != 0xFF for b in original[previous_total:]):
        raise ValueError("Unexpected data after FIT totalsize")
    position, old_dtb, _ = original_images["fdt"]
    if len(dtb) > len(old_dtb):
        raise ValueError("Replacement exceeds original DTB allocation")
    padded_dtb = bytearray(dtb)
    padded_dtb.extend(b"\0" * (len(old_dtb) - len(dtb)))
    struct.pack_into(">I", padded_dtb, 4, len(padded_dtb))
    fdt_properties(padded_dtb)
    resource, logos = patch_resource(original_images["resource"][1], old_dtb, padded_dtb)
    total = (previous_total + 2047) & ~2047
    if total > 12 * 1024 * 1024:
        raise ValueError("New image exceeds the inspected 12 MiB boot partition")
    output = bytearray(original[:previous_total])
    output.extend(b"\xff" * (total - len(output)))
    output[position:position + len(padded_dtb)] = padded_dtb
    resource_position = original_images["resource"][0]
    output[resource_position:resource_position + len(resource)] = resource
    for key, value in {
        ("/", "totalsize"): struct.pack(">I", total),
        ("/images/fdt", "data-position"): struct.pack(">I", position),
        ("/images/fdt", "data-size"): struct.pack(">I", len(padded_dtb)),
        ("/images/fdt/hash", "value"): hashlib.sha256(padded_dtb).digest(),
        ("/images/resource/hash", "value"): hashlib.sha256(resource).digest(),
    }.items():
        old_value, offset = properties[key]
        if len(old_value) != len(value):
            raise ValueError("FIT metadata size changed")
        output[offset:offset + len(value)] = value
    verified = images(output, fdt_properties(output))
    if verified["kernel"] != original_images["kernel"]:
        raise ValueError("Kernel changed")
    if verified["fdt"][1] != padded_dtb:
        raise ValueError("FIT DTB verification failed")
    patch_resource(verified["resource"][1], padded_dtb, padded_dtb)
    args.output.write_bytes(output)
    manifest = {
        "original_sha256": hashlib.sha256(original).hexdigest(),
        "output_sha256": hashlib.sha256(output).hexdigest(),
        "output_bytes": len(output),
        "dtb_sha256": hashlib.sha256(padded_dtb).hexdigest(),
        "kernel_sha256_unchanged": verified["kernel"][2],
        "resource_sha256": verified["resource"][2],
        "logo_sha256_unchanged": logos,
        "device_tree_changes": changes,
    }
    args.output.with_suffix(".manifest.json").write_text(
        json.dumps(manifest, indent=2) + "\n"
    )
    print(json.dumps(manifest, indent=2))


if __name__ == "__main__":
    main()
