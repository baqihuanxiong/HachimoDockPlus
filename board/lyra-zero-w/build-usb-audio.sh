#!/bin/bash
# Run inside the board's build container; outputs stay under ignored sdk/.
set -euo pipefail

project=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
sdk=${LYRA_SDK:-$project/sdk/luckyfox-lyra/source}
build=${LYRA_BUILD:-$project/sdk/luckyfox-lyra/build/cm108b}
artifacts=${LYRA_ARTIFACTS:-$project/sdk/luckyfox-lyra/artifacts/cm108b}
export ARCH=arm
export CROSS_COMPILE="$sdk/prebuilts/gcc/linux-x86/arm/gcc-arm-10.3-2021.07-x86_64-arm-none-linux-gnueabihf/bin/arm-none-linux-gnueabihf-"

test -f "$sdk/kernel-6.1/Makefile"
test "$(git --git-dir="$sdk/.repo/projects/kernel-6.1.git" rev-parse HEAD)" = \
    1e631165ecec0ed841de793305423acd61a2bbdd
"${CROSS_COMPILE}gcc" --version | head -n 1
mkdir -p "$build" "$artifacts"
kmake=(make -C "$sdk/kernel-6.1" O="$build")
"${kmake[@]}" rk3506_luckfox_defconfig rk3506-display.config
cp "$build/.config" "$artifacts/baseline.config"
(
    cd "$build"
    "$sdk/kernel-6.1/scripts/kconfig/merge_config.sh" -m .config \
        "$project/board/lyra-zero-w/usb-audio.config"
)
"${kmake[@]}" olddefconfig

# This module package targets the existing 6.1.99 SDK kernel. Only these
# additions may differ from its baseline; do not rebuild its ALSA core.
python3 - "$artifacts/baseline.config" "$build/.config" <<'PY'
import sys
from pathlib import Path

def read(path):
    return dict(line.split('=', 1) for line in Path(path).read_text().splitlines()
                if line.startswith('CONFIG_'))

before, after = map(read, sys.argv[1:])
expected = {
    'CONFIG_SND_HWDEP': 'm',
    'CONFIG_SND_RAWMIDI': 'm',
    'CONFIG_SND_USB_AUDIO': 'm',
    'CONFIG_SND_USB_AUDIO_USE_MEDIA_CONTROLLER': 'y',
}
changes = {key: after.get(key) for key in before.keys() | after.keys()
           if before.get(key) != after.get(key)}
if changes != expected:
    raise SystemExit(f'Unexpected kernel configuration changes: {changes}')
if after.get('CONFIG_MODVERSIONS') is not None:
    raise SystemExit('This recipe requires the SDK baseline without MODVERSIONS')
print('Verified audio-only kernel configuration changes:', changes)
PY

test "$("${kmake[@]}" -s kernelrelease)" = 6.1.99
# vmlinux supplies the exported symbols needed by modpost. It is not deployed.
"${kmake[@]}" -j"${JOBS:-8}" vmlinux modules
cp "$build/.config" "$artifacts/kernel.config"
cp "$build/Module.symvers" "$artifacts/Module.symvers"
package="$artifacts/modules"
for module in sound/core/snd-hwdep.ko sound/core/snd-rawmidi.ko \
    sound/usb/snd-usbmidi-lib.ko sound/usb/snd-usb-audio.ko; do
    target="$package/lib/modules/6.1.99/kernel/$module"
    install -D -m 644 "$build/$module" "$target"
    "${CROSS_COMPILE}strip" --strip-debug "$target"
done
(
    cd "$package"
    find lib -name '*.ko' -type f -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
    tar -czf "$artifacts/usb-audio-modules.tar.gz" lib SHA256SUMS
)
printf 'Module package: %s\n' "$artifacts/usb-audio-modules.tar.gz"
