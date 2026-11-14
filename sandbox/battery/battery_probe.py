#!/usr/bin/env python3
"""Read the Lyra Zero W baseboard INA226 using only Python's standard library."""

import argparse
import contextlib
import ctypes
import fcntl
import json
import math
import os
import signal
import sys
import time


# Schematic U11 = 10 mOhm. CAL = 0.00512 / (0.00025 A * 0.01 Ohm).
SHUNT_OHM = 0.01
CURRENT_LSB_A = 0.00025
CALIBRATION = 0x0800
# 16 averages, bus/shunt each 1.1 ms: one complete measurement ~35.2 ms.
CONFIG_IDLE = 0x4520
CONFIG_TRIGGER = 0x4523


class I2CMessage(ctypes.Structure):
    _fields_ = [('addr', ctypes.c_uint16), ('flags', ctypes.c_uint16),
                ('length', ctypes.c_uint16),
                ('buffer', ctypes.POINTER(ctypes.c_ubyte))]


class I2CTransfer(ctypes.Structure):
    _fields_ = [('messages', ctypes.POINTER(I2CMessage)),
                ('count', ctypes.c_uint32)]


class INA226:
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
        completed = fcntl.ioctl(self.fd, 0x0707, bytearray(bytes(transfer)), True)
        if completed != len(messages):
            raise OSError('Incomplete I2C transfer to INA226')
        return bytes(incoming)

    def read_register(self, register):
        return int.from_bytes(self.transfer([register], 2), 'big')

    def write_register(self, register, value):
        self.transfer([register, value >> 8, value & 255])

    def sample(self):
        # Triggered mode leaves all output registers stable after CVRF is set.
        self.write_register(0, CONFIG_TRIGGER)
        deadline = time.monotonic() + 0.5
        while True:
            status = self.read_register(6)
            if status & 8:
                break
            if time.monotonic() >= deadline:
                raise TimeoutError('INA226 conversion timed out')
            time.sleep(0.002)
        if status & 4:
            raise ValueError('INA226 math overflow; current/power are invalid')
        if self.read_register(5) != CALIBRATION:
            raise ValueError('INA226 calibration changed during measurement')
        raw = {name: self.read_register(register) for name, register in
               (('shunt', 1), ('bus', 2), ('power', 3), ('current', 4))}
        result = decode(raw)
        result['raw'] = raw
        result['mask'] = status
        return result


def signed16(value):
    return value if value < 0x8000 else value - 0x10000


def decode(raw):
    shunt = signed16(raw['shunt']) * 2.5e-6
    current = signed16(raw['current']) * CURRENT_LSB_A
    bus = raw['bus'] * 0.00125
    if raw['bus'] & 0x8000:
        raise ValueError('Invalid INA226 bus voltage sign bit')
    if abs(signed16(raw['shunt'])) >= 32760:
        raise ValueError('INA226 shunt voltage at full scale')
    if abs(current - shunt / SHUNT_OHM) > CURRENT_LSB_A * 1.1:
        raise ValueError('INA226 current and shunt voltage disagree')
    state = ('DISCHARGING' if current > 0.005 else
             'CHARGING' if current < -0.005 else 'IDLE')
    return {'bus_v': bus, 'battery_side_v': bus + shunt,
            'shunt_mv': shunt * 1000, 'current_ma': current * 1000,
            'power_w': bus * current, 'state': state}


@contextlib.contextmanager
def sensor(path, address, emit):
    fd = os.open(path, os.O_RDWR | os.O_CLOEXEC)
    try:
        # Advisory lock serializes this script's instances, including on other addresses.
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        # Reject devices owned by a kernel driver; never I2C_SLAVE_FORCE.
        fcntl.ioctl(fd, 0x0703, address)
        device = INA226(fd, address)
        manufacturer = device.read_register(0xFE)
        die = device.read_register(0xFF)
        if manufacturer != 0x5449 or die & 0xFFF0 != 0x2260:
            raise ValueError('Not an INA226: manufacturer=0x%04x die=0x%04x'
                             % (manufacturer, die))
        previous_config = device.read_register(0)
        previous_calibration = device.read_register(5)
        emit('INIT', i2c=path, address=address, manufacturer=manufacturer, die=die,
             previous_config=previous_config, previous_calibration=previous_calibration,
             config=CONFIG_TRIGGER, calibration=CALIBRATION)
        try:
            device.write_register(0, CONFIG_IDLE)
            device.write_register(5, CALIBRATION)
            if (device.read_register(0) != CONFIG_IDLE or
                    device.read_register(5) != CALIBRATION):
                raise OSError('INA226 configuration readback failed')
            yield device
        finally:
            # Attempt both restorations even when one register write fails.
            try:
                device.write_register(5, previous_calibration)
            finally:
                device.write_register(0, previous_config & 0x7FFF)
            restored_config = device.read_register(0)
            restored_calibration = device.read_register(5)
            if (restored_config != previous_config or
                    restored_calibration != previous_calibration):
                raise OSError('INA226 restoration readback failed')
            emit('RESTORED', config=restored_config, calibration=restored_calibration)
    finally:
        os.close(fd)


def monitor(args):
    start = time.monotonic()

    def emit(event, **values):
        elapsed = time.monotonic() - start
        if args.json:
            print(json.dumps(dict(event=event, elapsed_s=round(elapsed, 6), **values)),
                  flush=True)
        elif event == 'SAMPLE':
            print(('[%8.3f] %-11s VBUS=%.5f V Vbat~=%.5f V '
                   'I=%+.2f mA P=%+.4f W Vshunt=%+.4f mV') %
                  (elapsed, values['state'], values['bus_v'], values['battery_side_v'],
                   values['current_ma'], values['power_w'], values['shunt_mv']), flush=True)
        else:
            print('[%8.3f] %s %s' % (elapsed, event, json.dumps(values)), flush=True)

    count = 0
    try:
        with sensor(args.i2c, args.address, emit) as device:
            while args.count == 0 or count < args.count:
                tick = time.monotonic()
                emit('SAMPLE', **device.sample())
                count += 1
                if args.count == 0 or count < args.count:
                    time.sleep(max(0, args.interval - (time.monotonic() - tick)))
    except KeyboardInterrupt:
        emit('STOP', samples=count)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--i2c', default='/dev/i2c-2')
    parser.add_argument('--address', type=lambda value: int(value, 0), default=0x40)
    parser.add_argument('--count', type=int, default=0, help='sample count; 0 runs until Ctrl+C')
    parser.add_argument('--interval', type=float, default=1.0, help='sample interval in seconds')
    parser.add_argument('--json', action='store_true', help='emit JSON Lines including raw registers')
    args = parser.parse_args()
    if not 0x08 <= args.address <= 0x77:
        parser.error('--address must be a usable 7-bit I2C address')
    if args.count < 0:
        parser.error('--count must be nonnegative')
    if not math.isfinite(args.interval) or args.interval < 0.05:
        parser.error('--interval must be finite and at least 0.05 seconds')

    def stop(signum, frame):
        # Ignore subsequent termination signals so register restoration can finish.
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        raise KeyboardInterrupt

    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    try:
        monitor(args)
    except (OSError, ValueError) as error:
        print('ERROR: %s' % error, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
