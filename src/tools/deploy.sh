#!/bin/sh
# Installs an application release; does not touch boot, rootfs images or autostart.
set -eu
umask 077
cd "$(dirname "$0")/.."
host="${1:-root@192.168.4.202}"
test -f build/arm/dock
test -f .local/config.json
version="$(sha256sum build/arm/dock | cut -c1-12)"
package="$(pwd)/build/package-$version"
mkdir -p "$package/assets/app-icons" "$package/secrets"
cp build/arm/dock "$package/dock"
cp .local/config.json "$package/config.json"
cp .local/secrets/*.key "$package/secrets/"
cp -r .local/assets/. "$package/assets/"
cp assets/app-icons/*.png "$package/assets/app-icons/"
cp assets/app-icons/PICOCLAW-LICENSE.txt "$package/assets/app-icons/"
chmod 700 "$package" "$package/secrets"
chmod 600 "$package/config.json" "$package"/secrets/*.key
tar -czf "build/package-$version.tgz" -C "$package" .
chmod 600 "build/package-$version.tgz"
scp -O -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=2 \
  "build/package-$version.tgz" "$host:/tmp/hachimodock-package-$version.tgz"
ssh -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=2 \
  "$host" sh -s -- "$version" <<'REMOTE'
set -eu
version="$1"
base=/opt/hachimodock
stage="/tmp/hachimodock-stage-$version"
service=/etc/init.d/S99hachimodock
restart=0
cleanup() {
  result=$?
  trap - EXIT
  rm -rf "$stage" "/tmp/hachimodock-package-$version.tgz"
  rm -f "$base/releases/$version/dock.new"
  if [ "$restart" = 1 ]; then "$service" start || :; fi
  exit "$result"
}
trap cleanup EXIT
mkdir -p "$stage" "$base/releases/$version"
chmod 700 "$stage" "$base"
tar -xzf "/tmp/hachimodock-package-$version.tgz" -C "$stage"
# Refuse before stopping the live UI if NAND cannot hold another release.
available=$(df -Pk "$base" | awk 'NR==2 {print $4}')
required=$(( $(wc -c < "$stage/dock") / 1024 + $(du -sk "$stage/assets" | awk '{print $1}') + 8192 ))
if [ "$available" -lt "$required" ]; then
  echo "Not enough space for release: archive old releases first (need ${required} KiB, available ${available} KiB)" >&2
  exit 1
fi
# Stop before replacing an existing executable, including identical redeployments.
if [ -x "$service" ] && "$service" status >/dev/null 2>&1; then
  "$service" stop
  restart=1
fi
cp "$stage/dock" "$base/releases/$version/dock.new"
chmod 755 "$base/releases/$version/dock.new"
mv -f "$base/releases/$version/dock.new" "$base/releases/$version/dock"
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
if [ "$restart" = 1 ]; then "$service" start; restart=0; fi
printf 'Installed %s/current/dock; run with -config %s/config.json\n' "$base" "$base"
REMOTE
