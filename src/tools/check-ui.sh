#!/bin/sh
# Run inside the build container after tools/build.sh native.
set -eu
cd "$(dirname "$0")/.."
gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/native_ui.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/ui-check
build/native/ui-check "$(pwd)/.local/assets/wqy-microhei.ttc"

gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/music_ui.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/music-ui-check
build/native/music-ui-check "$(pwd)/.local/assets/wqy-microhei.ttc"

gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/newapi_ui.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/newapi-ui-check
build/native/newapi-ui-check "$(pwd)/.local/assets/wqy-microhei.ttc"

gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/picoclaw_scroll.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/picoclaw-scroll-check
build/native/picoclaw-scroll-check "$(pwd)/.local/assets/wqy-microhei.ttc"

gcc -std=gnu11 -DLV_CONF_INCLUDE_SIMPLE -Iinternal/ui/lvgl -I.cache/lvgl \
  tests/picoclaw_process.c build/native/lvgl/lib/liblvgl.a -lm -o build/native/picoclaw-process-check
build/native/picoclaw-process-check "$(pwd)/.local/assets/wqy-microhei.ttc"
