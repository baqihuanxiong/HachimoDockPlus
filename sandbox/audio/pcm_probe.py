#!/usr/bin/env python3
"""Generate a stereo speaker probe or summarize a mono microphone WAV.

The probe is seven seconds: silence, left 997 Hz, silence, right 1499 Hz,
silence, both 777 Hz, silence. Tones peak at -18 dBFS with short fades.
Recordings and generated WAV files belong under ignored sdk/ artifacts.
"""

import argparse
import array
import json
import math
import sys
import wave


def db(value):
    return round(20 * math.log10(max(value, 1e-9)), 2)


def tone(path):
    rate = 48000
    samples = array.array('h')
    for second in range(7):
        for frame in range(rate):
            fade = min(1, frame / 480, (rate - 1 - frame) / 480)
            frequency = {1: 997, 3: 1499, 5: 777}.get(second, 0)
            value = round(32767 * 10 ** (-18 / 20) * fade
                          * math.sin(2 * math.pi * frequency * frame / rate))
            samples.extend((value if second in (1, 5) else 0,
                            value if second in (3, 5) else 0))
    if sys.byteorder != 'little':
        samples.byteswap()
    with wave.open(path, 'wb') as wav:
        wav.setparams((2, 2, rate, 0, 'NONE', 'not compressed'))
        wav.writeframes(samples.tobytes())


def analyze(path):
    with wave.open(path, 'rb') as wav:
        if wav.getnchannels() != 1 or wav.getsampwidth() != 2:
            raise SystemExit('Expected mono PCM16 WAV')
        rate = wav.getframerate()
        samples = array.array('h', wav.readframes(wav.getnframes()))
    if sys.byteorder != 'little':
        samples.byteswap()
    if not samples:
        raise SystemExit('Empty recording')
    windows = []
    for start in range(0, len(samples), rate // 4):
        block = samples[start:start + rate // 4]
        mean = sum(block) / len(block)
        centered = [(value - mean) / 32768 for value in block]
        rms = math.sqrt(sum(value * value for value in centered) / len(block))
        peaks = {}
        for frequency in (777, 997, 1499):
            coefficient = 2 * math.cos(2 * math.pi * frequency / rate)
            previous = previous2 = 0.0
            for value in centered:
                current = value + coefficient * previous - previous2
                previous2, previous = previous, current
            power = max(0, previous ** 2 + previous2 ** 2
                        - coefficient * previous * previous2)
            peaks[str(frequency)] = db(2 * math.sqrt(power) / len(block))
        windows.append({'start_s': round(start / rate, 2), 'rms_dbfs': db(rms),
                        'tone_peak_dbfs': peaks})
    print(json.dumps({'file': path, 'sample_rate': rate,
                      'duration_s': len(samples) / rate,
                      'peak_dbfs': db(max(abs(value) for value in samples) / 32768),
                      'clipped_samples': sum(abs(value) >= 32767 for value in samples),
                      'windows': windows}, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=('tone', 'analyze'))
    parser.add_argument('path')
    args = parser.parse_args()
    (tone if args.command == 'tone' else analyze)(args.path)
