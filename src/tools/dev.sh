#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
image=hachimodock-go-lvgl:dev
if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build -t "$image" -f tools/Dockerfile .
fi
mkdir -p .cache/go-mod .cache/go-build build
exec docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e GOCACHE=/work/.cache/go-build \
  -e GOMODCACHE=/work/.cache/go-mod -e GOMAXPROCS=2 \
  -v "$(pwd):/work" "$image" "$@"
