#!/bin/sh
# Install the isolated SMB runtime. No guest account or default password is created.
set -eu
cd "$(dirname "$0")/.."
host="${1:-root@192.168.4.202}"
runtime_only=0
case "${2:-}" in '') ;; --runtime-only) runtime_only=1;; *) exit 2;; esac
test -f build/samba-runtime.tgz
bundle_id=$(sha256sum build/samba-runtime.tgz | cut -d' ' -f1)
stage=$(mktemp -d build/smb-install-XXXXXX)
trap 'rm -rf "$stage"' EXIT
cp build/samba-runtime.tgz tools/samba-run tools/S98hachimodock-smb tools/dock-run "$stage/"
printf '%s\n' "$bundle_id" > "$stage/bundle-id"
tar -czf "$stage/package.tgz" -C "$stage" samba-runtime.tgz samba-run S98hachimodock-smb dock-run bundle-id
scp -O -o ConnectTimeout=10 "$stage/package.tgz" "$host:/tmp/hachimodock-smb-install.tgz"
ssh -o ConnectTimeout=10 "$host" sh -s -- "$runtime_only" <<'REMOTE'
set -eu
umask 077
runtime_only=$1
base=/opt/hachimodock
stage=$(mktemp -d /tmp/hachimodock-smb-install-XXXXXX)
restart_ui=0
cleanup() {
  result=$?
  trap - EXIT
  rm -rf "$stage" /tmp/hachimodock-smb-install.tgz
  if [ "$restart_ui" = 1 ]; then /etc/init.d/S99hachimodock start || :; fi
  exit "$result"
}
trap cleanup EXIT
tar -xzf /tmp/hachimodock-smb-install.tgz -C "$stage"
if [ "$runtime_only" = 0 ]; then
  "$base/current/dock" -h 2>&1 | grep -q prepare-system || { echo 'Deploy the system-settings application first' >&2; exit 1; }
fi
available=$(df -Pk "$base" | awk 'NR==2 {print $4}')
required=$(( $(wc -c < "$stage/samba-runtime.tgz") / 1024 + 4096 ))
[ "$available" -ge "$required" ] || { echo 'Not enough NAND space for compressed SMB runtime' >&2; exit 1; }
if [ -x /etc/init.d/S98hachimodock-smb ]; then /etc/init.d/S98hachimodock-smb stop; fi
mkdir -p "$base/samba" "$base/system/smb" /usr/lib/arm-linux-gnueabihf /usr/libexec
for link in /usr/lib/arm-linux-gnueabihf/samba /usr/libexec/samba; do
  if [ -e "$link" ] || [ -L "$link" ]; then
    [ -L "$link" ] && readlink "$link" | grep -q '^/tmp/hachimodock-samba-runtime/' || { echo "Existing system Samba path: $link" >&2; exit 1; }
  fi
done
ln -sfn /tmp/hachimodock-samba-runtime/modules /usr/lib/arm-linux-gnueabihf/samba
ln -sfn /tmp/hachimodock-samba-runtime/libexec /usr/libexec/samba
cp "$stage/samba-runtime.tgz" "$base/samba/runtime.tgz.new"
mv "$base/samba/runtime.tgz.new" "$base/samba/runtime.tgz"
cp "$stage/bundle-id" "$base/samba/bundle-id"
for command in smbd smbpasswd smbcontrol testparm; do cp "$stage/samba-run" "$base/samba/$command"; chmod 755 "$base/samba/$command"; done
cp "$stage/S98hachimodock-smb" /etc/init.d/S98hachimodock-smb
chmod 755 /etc/init.d/S98hachimodock-smb
"$base/samba/smbd" --version
if [ "$runtime_only" = 0 ]; then
  if /etc/init.d/S99hachimodock status >/dev/null 2>&1; then /etc/init.d/S99hachimodock stop; restart_ui=1; fi
  cp "$stage/dock-run" "$base/dock-run"
  chmod 755 "$base/dock-run"
  "$base/current/dock" -config "$base/config.json" -prepare-system
fi
/etc/init.d/S98hachimodock-smb start
printf '%s\n' 'SMB runtime installed; set its username and password in Settings / System.'
REMOTE
