#!/bin/sh
# Installs an application release; does not touch boot, rootfs images or autostart.
set -eu
umask 077
cd "$(dirname "$0")/.."
host="${1:-root@192.168.5.156}"
test -f build/arm/dock
test -f .local/config.json
version="$(sha256sum build/arm/dock | cut -c1-12)"
package="$(pwd)/build/package-$version"
mkdir -p "$package/assets" "$package/secrets"
cp build/arm/dock "$package/dock"
cp .local/config.json "$package/config.json"
cp .local/secrets/*.key "$package/secrets/"
cp .local/assets/* "$package/assets/"
chmod 700 "$package" "$package/secrets"
chmod 600 "$package/config.json" "$package"/secrets/*.key
tar -czf "build/package-$version.tgz" -C "$package" .
chmod 600 "build/package-$version.tgz"
scp -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=2 \
  "build/package-$version.tgz" "$host:/tmp/hachimodock-package-$version.tgz"
ssh -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=2 \
  "$host" sh -s -- "$version" <<'REMOTE'
set -eu
version="$1"
base=/opt/hachimodock
stage="/tmp/hachimodock-stage-$version"
mkdir -p "$stage" "$base/releases/$version"
chmod 700 "$stage" "$base"
tar -xzf "/tmp/hachimodock-package-$version.tgz" -C "$stage"
cp "$stage/dock" "$base/releases/$version/dock"
chmod 755 "$base/releases/$version/dock"
cp -r "$stage/assets" "$base/"
# Preserve board-local endpoints and credentials on subsequent code deployments.
if [ ! -e "$base/config.json" ]; then
  cp "$stage/config.json" "$base/config.json"
  cp -r "$stage/secrets" "$base/"
  chmod 600 "$base/config.json" "$base"/secrets/*.key
  chmod 700 "$base/secrets"
fi
ln -sfn "releases/$version" "$base/current"
rm -rf "$stage" "/tmp/hachimodock-package-$version.tgz"
printf 'Installed %s/current/dock; run with -config %s/config.json\n' "$base" "$base"
REMOTE
