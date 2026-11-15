"""Pure host-side checks; no SSH, device writes, or private configuration."""

import struct
import contextlib
import io
import unittest
from unittest.mock import patch
import zlib

import board_ui_probe as probe


class BoardUIProbeTests(unittest.TestCase):
    def test_event_abi_and_coordinates(self):
        self.assertEqual(probe.point(0, 0), (479, 0))
        self.assertEqual(probe.point(639, 479), (0, 639))
        packet = probe.event(3, 0x39, -1)
        self.assertEqual(len(packet), 16)
        self.assertEqual(struct.unpack("<llHHi", packet), (0, 0, 3, 0x39, -1))
        for point in ((-1, 0), (640, 0), (0, 480)):
            with self.assertRaises(ValueError):
                probe.point(*point)

    def test_input_release_and_device_validation(self):
        script = probe.input_script([(64, 126), (64, 100)], "/dev/input/event0", 0.02)
        self.assertIn(" EXIT\n", script)
        self.assertIn("HUP INT TERM", script)
        self.assertEqual(script.count("sleep 0.0200"), 2)
        with self.assertRaises(ValueError):
            probe.input_script([(1, 2)], "/dev/fb0", 0.08)
        devices = ('N: Name="2-0038 generic ft5x06 (00)"\n'
                   'H: Handlers=event0 cpufreq\nB: ABS=2608000 3\n\n'
                   'N: Name="bt-powerkey"\nH: Handlers=kbd event1\n')
        self.assertIn("ft5x06", probe.touch_identity(devices, "/dev/input/event0"))
        with self.assertRaises(ValueError):
            probe.touch_identity(devices, "/dev/input/event1")

    def test_framebuffer_rotation_and_png(self):
        raw = bytearray(480 * 640 * 4)
        expected = {(0, 0): (255, 0, 0), (639, 0): (0, 255, 0),
                    (0, 479): (0, 0, 255), (639, 479): (10, 20, 30)}
        for (x, y), rgb in expected.items():
            offset = (x * 480 + 479 - y) * 4
            raw[offset:offset + 4] = bytes((*reversed(rgb), 0))
        png = probe.framebuffer_png(raw)
        self.assertEqual(png[:8], b"\x89PNG\r\n\x1a\n")
        offset, packed = 8, bytearray()
        while offset < len(png):
            length = struct.unpack(">I", png[offset:offset + 4])[0]
            kind = png[offset + 4:offset + 8]
            if kind == b"IDAT":
                packed.extend(png[offset + 8:offset + 8 + length])
            offset += length + 12
        pixels = zlib.decompress(packed)
        for (x, y), rgb in expected.items():
            start = y * (640 * 3 + 1) + 1 + x * 3
            self.assertEqual(pixels[start:start + 3], bytes(rgb))
        with self.assertRaises(ValueError):
            probe.framebuffer_png(b"bad")

    def test_dry_run_never_connects(self):
        output = io.StringIO()
        with patch.object(probe, "remote", side_effect=AssertionError("unexpected SSH")):
            with contextlib.redirect_stdout(output):
                result = probe.main(["--host", "root@board", "--device", "/dev/input/event0",
                                     "--dry-run", "tap", "64", "126"])
        self.assertEqual(result, 0)
        self.assertIn("trap ", output.getvalue())


if __name__ == "__main__":
    unittest.main()
