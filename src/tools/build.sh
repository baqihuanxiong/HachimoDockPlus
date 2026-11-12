#!/bin/sh
# Run inside tools/Dockerfile's build image, from src/.
set -eu
cd "$(dirname "$0")/.."
target="${1:-native}"
case "$target" in
  native) cc=gcc; arch=amd64; cmake_system="";;
  arm) cc=arm-linux-gnueabihf-gcc; arch=arm; cmake_system="-DCMAKE_SYSTEM_NAME=Linux";;
  *) echo 'usage: tools/build.sh native|arm' >&2; exit 2;;
esac
out="$(pwd)/build/$target"
mkdir -p "$out"
cmake -S .cache/lvgl -B "$out/lvgl" \
  -DCMAKE_BUILD_TYPE=MinSizeRel -DCMAKE_C_COMPILER="$cc" \
  -DLV_BUILD_CONF_PATH="$(pwd)/internal/ui/lvgl/lv_conf.h" \
  -DCONFIG_LV_BUILD_EXAMPLES=OFF -DCONFIG_LV_BUILD_DEMOS=OFF -DCONFIG_LV_USE_THORVG_INTERNAL=OFF \
  -DCMAKE_C_FLAGS='-ffunction-sections -fdata-sections' $cmake_system
cmake --build "$out/lvgl" --target lvgl -j 4
lib="$(find "$out/lvgl" -name liblvgl.a -print -quit)"
test -n "$lib"
CGO_ENABLED=1 CC="$cc" GOOS=linux GOARCH="$arch" GOARM=7 \
  CGO_LDFLAGS="$lib -lm -Wl,--gc-sections" \
  go build -p 2 -tags 'lvgl,stdjson,goolm' -trimpath -ldflags='-s -w' -o "$out/dock" ./cmd/dock
printf 'Built %s/dock\n' "$out"
