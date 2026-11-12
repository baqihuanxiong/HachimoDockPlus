#!/bin/sh
# Run inside the build container after tools/build.sh native.
set -eu
cd "$(dirname "$0")/.."
gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/native_ui.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/ui-check
build/native/ui-check "$(pwd)/.local/assets/wqy-microhei.ttc"
