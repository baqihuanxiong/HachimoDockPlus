#!/bin/sh
# Independent CLI; no LVGL, PicoClaw, Python, MiLoCo service or CGO dependency.
set -eu
cd "$(dirname "$0")/.."
target="${1:-arm}"
case "$target" in
  arm) arch=arm;;
  native) arch="$(go env GOARCH)";;
  *) echo 'usage: tools/build-mijia.sh arm|native' >&2; exit 2;;
esac
out="$(pwd)/build/$target"
mkdir -p "$out"
project="$(pwd)"
cd extensions/mijia
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOARM=7 \
  go build -trimpath -ldflags='-s -w' -o "$out/mijia" .
printf 'Built %s/mijia\n' "$out"
stage="$(mktemp -d "$out/mijia-package-XXXXXX")"
trap 'rm -rf "$stage"' EXIT
mkdir -p "$stage/agent/bin" "$stage/agent/skills/mijia"
cp "$out/mijia" "$stage/agent/bin/mijia"
cp SKILL.md README.md Xiaomi-Miloco-LICENSE.md THIRD_PARTY_NOTICES.md "$stage/agent/skills/mijia/"
suffix="$target"
[ "$target" != arm ] || suffix=armv7
tar -czf "$project/build/mijia-extension-$suffix.tar.gz" -C "$stage" agent
printf 'Packaged build/mijia-extension-%s.tar.gz\n' "$suffix"
