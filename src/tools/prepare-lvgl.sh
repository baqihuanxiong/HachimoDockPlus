#!/bin/sh
# Keep local fixes reproducible for the pinned LVGL checkout.
set -eu
cd "$(dirname "$0")/.."
test "$(git -C .cache/lvgl rev-parse HEAD)" = c033a98afddd65aaafeebea625382a94020fe4a7
patch=../../patches/lvgl-span-line-cache.patch
if ! git -C .cache/lvgl apply --reverse --check "$patch" 2>/dev/null; then
  git -C .cache/lvgl apply "$patch"
fi
