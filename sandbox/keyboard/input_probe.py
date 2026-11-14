#!/usr/bin/env python3
"""Monitor Lyra Zero W baseboard SW1/2/3 and ADS1015 joystick (stdlib only)."""

import argparse
import contextlib
import ctypes
import fcntl
import math
import mmap
import os
from pathlib import Path
import signal
import statistics
import struct
import sys
import time


BUTTONS = ('SW1', 'SW2', 'SW3', 'JOYSTICK_KEY')
OFFSETS = (2, 3, 4, 5)  # GPIO0_A2..A5; active low, internal pull-ups.
GPIO_REQUEST = struct.Struct('=64II64s32sIi')
# Linux gpio.h ABI v1: input | active low | bias pull up.
GPIO_FLAGS = (1 << 0) | (1 << 2) | (1 << 5)
GPIO_GET_LINEHANDLE = 0xC000B403 | (GPIO_REQUEST.size << 16)
GPIO_GET_VALUES = 0xC040B408


@contextlib.contextmanager
def buttons(path):
    chip = os.open(path, os.O_RDONLY | os.O_CLOEXEC)
    try:
        info = bytearray(68)
        fcntl.ioctl(chip, 0x8044B401, info, True)  # GPIO_GET_CHIPINFO_IOCTL
        if bytes(info[32:64]).rstrip(b'\0') != b'gpio0':
            raise ValueError('Expected gpio0; refusing to configure another GPIO bank')
        request = bytearray(GPIO_REQUEST.pack(
            *(OFFSETS + (0,) * 60), GPIO_FLAGS, bytes(64),
            b'hachimodock-input-probe', len(OFFSETS), -1))
        fcntl.ioctl(chip, GPIO_GET_LINEHANDLE, request, True)
        handle = GPIO_REQUEST.unpack(request)[-1]
    finally:
        os.close(chip)
    try:
        def read():
            values = bytearray(64)
            fcntl.ioctl(handle, GPIO_GET_VALUES, values, True)
            return tuple(values[:len(OFFSETS)])  # 1 = pressed (active low).
        with rk3506_pullups():
            yield read
    finally:
        os.close(handle)


@contextlib.contextmanager
def rk3506_pullups():
    # This SDK's gpio-rockchip set_config only handles debounce. Bias flags
    # silently do nothing. Use the same register/encoding as pinctrl-rockchip:
    # rk3506_calc_pull_reg_and_bit(), GPIO0_A PULL at IOC_PMU + 0x200.
    node = Path('/sys/firmware/devicetree/base/syscon@ff950000')
    if (b'rockchip,rk3506-ioc-pmu' not in node.joinpath('compatible').read_bytes().split(b'\0')
            or struct.unpack('>II', node.joinpath('reg').read_bytes())
            != (0xFF950000, 0x10000)):
        raise ValueError('Unsupported IOC_PMU device tree; refusing register access')
    fd = os.open('/dev/mem', os.O_RDWR | os.O_SYNC | os.O_CLOEXEC)
    try:
        mapping = mmap.mmap(fd, mmap.PAGESIZE, flags=mmap.MAP_SHARED,
                            prot=mmap.PROT_READ | mmap.PROT_WRITE, offset=0xFF950000)
    finally:
        os.close(fd)
    register = ctypes.c_uint32.from_buffer(mapping, 0x200)
    mask = sum(3 << (pin * 2) for pin in OFFSETS)
    pullup = sum(1 << (pin * 2) for pin in OFFSETS)
    previous = register.value & mask
    try:
        # Upper half is Rockchip write mask: leave A0/A1 (I2C) and A6/A7 alone.
        register.value = (mask << 16) | pullup
        if register.value & mask != pullup:
            raise OSError('GPIO0_A2..A5 pull-up readback failed')
        yield
    finally:
        register.value = (mask << 16) | previous
        del register
        mapping.close()


class I2CMessage(ctypes.Structure):
    _fields_ = [('addr', ctypes.c_uint16), ('flags', ctypes.c_uint16),
                ('length', ctypes.c_uint16),
                ('buffer', ctypes.POINTER(ctypes.c_ubyte))]


class I2CTransfer(ctypes.Structure):
    _fields_ = [('messages', ctypes.POINTER(I2CMessage)),
                ('count', ctypes.c_uint32)]


class ADS1015:
    def __init__(self, fd, address):
        self.fd = fd
        self.address = address

    def transfer(self, payload, read_size=0):
        output = (ctypes.c_ubyte * len(payload))(*payload)
        incoming = (ctypes.c_ubyte * read_size)()
        messages = [I2CMessage(self.address, 0, len(output), output)]
        if read_size:
            messages.append(I2CMessage(self.address, 1, read_size, incoming))
        array = (I2CMessage * len(messages))(*messages)
        transfer = I2CTransfer(array, len(messages))
        completed = fcntl.ioctl(self.fd, 0x0707,
                                bytearray(bytes(transfer)), True)  # I2C_RDWR
        if completed != len(messages):
            raise OSError('Incomplete I2C transfer to ADS1015')
        return bytes(incoming)

    def read_register(self, register):
        return int.from_bytes(self.transfer([register], 2), 'big')

    def write_register(self, register, value):
        self.transfer([register, value >> 8, value & 255])

    def read_axis(self, channel):
        # OS=start, MUX=AINn-GND, PGA=+/-4.096V, single-shot,
        # DR=1600 SPS, comparator disabled. AIN0=0xc383; AIN1=0xd383.
        config = 0x8383 | ((4 + channel) << 12)
        self.write_register(1, config)
        deadline = time.monotonic() + 0.05
        time.sleep(0.001)  # 625 us conversion + oscillator startup/margin.
        while not self.read_register(1) & 0x8000:
            if time.monotonic() >= deadline:
                raise TimeoutError('ADS1015 conversion timed out')
            time.sleep(0.0005)
        word = self.read_register(0)
        return (word if word < 0x8000 else word - 0x10000) >> 4

    def read_axes(self):
        return self.read_axis(0), self.read_axis(1)


@contextlib.contextmanager
def adc(path, address):
    fd = os.open(path, os.O_RDWR | os.O_CLOEXEC)
    try:
        # Reject an address owned by a kernel driver; never use I2C_SLAVE_FORCE.
        fcntl.ioctl(fd, 0x0703, address)
        device = ADS1015(fd, address)
        previous = device.read_register(1)
        try:
            yield device
        finally:
            device.write_register(1, previous & 0x7FFF)  # Do not trigger OS.
    finally:
        os.close(fd)


class Debouncer:
    def __init__(self, values, now, delay):
        self.stable = list(values)
        self.pending = list(values)
        self.since = [now] * len(values)
        self.delay = delay

    def update(self, values, now):
        events = []
        for i, value in enumerate(values):
            if value != self.pending[i]:
                self.pending[i], self.since[i] = value, now
            if value != self.stable[i] and now - self.since[i] >= self.delay:
                self.stable[i] = value
                events.append((BUTTONS[i], value))
        return events


def normalize(raw, center):
    # 2 mV/count at +/-4.096V; joystick supply is nominally 3.3V.
    span = center if raw < center else 1650 - center
    return max(-1.0, min(1.0, (raw - center) / span))


def axis_direction(value, previous, deadzone, hysteresis):
    if value >= deadzone:
        return 1
    if value <= -deadzone:
        return -1
    if previous and value * previous > deadzone - hysteresis:
        return previous
    return 0


def direction_label(directions):
    return '/'.join(axis + ('+' if value > 0 else '-')
                    for axis, value in zip('XY', directions) if value) or 'CENTER'


def axis_text(raw, center, directions):
    x, y = (normalize(v, c) for v, c in zip(raw, center))
    return ('raw=(%4d,%4d) V=(%.3f,%.3f) xy=(%+.3f,%+.3f) dir=%s'
            % (*raw, raw[0] * 0.002, raw[1] * 0.002,
               x, y, direction_label(directions)))


def monitor(args):
    start = time.monotonic()

    def emit(event):
        print('[%9.3f] %s' % (time.monotonic() - start, event), flush=True)

    with buttons(args.gpiochip) as read_buttons, adc(args.i2c, args.address) as device:
        emit('READY GPIO0_A2..A5 input/pull-up, ADS1015 %s@0x%02x'
             % (args.i2c, args.address))
        emit('CALIBRATE keep joystick centered for 0.5 seconds')
        samples = []
        while time.monotonic() - start < 0.5 or not samples:
            samples.append(device.read_axes())
            time.sleep(0.01)
        center = tuple(statistics.median(pair[i] for pair in samples) for i in (0, 1))
        if any(not 350 <= value <= 1300 for value in center):
            raise ValueError('Joystick center outside 0.7..2.6V: %s; release '
                             'joystick and check AIN0/1 wiring, then restart' % (center,))
        emit('CENTER raw=(%.1f,%.1f) V=(%.3f,%.3f)'
             % (*center, center[0] * 0.002, center[1] * 0.002))
        values = read_buttons()
        keys = Debouncer(values, time.monotonic(), args.debounce_ms / 1000)
        for name, value in zip(BUTTONS, values):
            emit('%s INIT %s' % (name, 'PRESSED' if value else 'RELEASED'))
        directions = (0, 0)
        last_raw = None
        next_axes = next_status = time.monotonic()
        while not args.duration or time.monotonic() - start < args.duration:
            tick = time.monotonic()
            for name, value in keys.update(read_buttons(), tick):
                emit('%s %s' % (name, 'PRESS' if value else 'RELEASE'))
            if tick >= next_axes:
                raw = device.read_axes()
                new_directions = tuple(axis_direction(normalize(v, c), old,
                                       args.deadzone, 0.05)
                                       for v, c, old in zip(raw, center, directions))
                if (args.raw or last_raw is None or new_directions != directions
                        or max(abs(a - b) for a, b in zip(raw, last_raw)) >= 12):
                    emit('JOYSTICK MOVE ' + axis_text(raw, center, new_directions))
                    last_raw = raw
                directions = new_directions
                next_axes = time.monotonic() + 0.02
                if tick >= next_status:
                    emit('STATE ' + ' '.join('%s=%d' % pair
                         for pair in zip(BUTTONS, keys.stable)) + ' '
                         + axis_text(raw, center, directions))
                    next_status = time.monotonic() + 1
            time.sleep(max(0, 0.005 - (time.monotonic() - tick)))
    emit('STOP GPIO handles closed; pull resistors and ADS1015 config restored')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gpiochip', default='/dev/gpiochip0')
    parser.add_argument('--i2c', default='/dev/i2c-2')
    parser.add_argument('--address', type=lambda s: int(s, 0), default=0x48)
    parser.add_argument('--debounce-ms', type=float, default=20)
    parser.add_argument('--deadzone', type=float, default=0.20)
    parser.add_argument('--duration', type=float, default=0,
                        help='stop after N seconds, including calibration (0: forever)')
    parser.add_argument('--raw', action='store_true', help='print every axis sample')
    args = parser.parse_args()
    if not 0x03 <= args.address <= 0x77:
        parser.error('--address must be a usable 7-bit I2C address')
    if not math.isfinite(args.deadzone) or not 0.05 < args.deadzone < 1:
        parser.error('--deadzone must be between 0.05 and 1')
    if any(not math.isfinite(v) or v < 0 for v in (args.duration, args.debounce_ms)):
        parser.error('--duration and --debounce-ms must be finite and nonnegative')

    def stop(signum, frame):
        raise KeyboardInterrupt

    signal.signal(signal.SIGTERM, stop)
    try:
        monitor(args)
    except KeyboardInterrupt:
        print('\nStopped; GPIO handles closed and ADC cleanup attempted.', flush=True)
    except (OSError, ValueError) as exc:
        print('ERROR: %s (check root permissions, GPIO ownership, pull-up support '
              'and ADS1015 wiring; do not run another ADC reader concurrently)'
              % exc, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
