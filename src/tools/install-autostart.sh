#!/bin/sh
# Install only after deploying and checking a release. Existing defaults are preserved.
set -eu
cd "$(dirname "$0")/.."
host="${1:-root@192.168.4.202}"
scp -O -o ConnectTimeout=10 tools/dock-run tools/S99hachimodock "$host:/tmp/"
ssh -o ConnectTimeout=10 "$host" sh -s <<'REMOTE'
set -eu
test -x /opt/hachimodock/current/dock
test -r /opt/hachimodock/config.json
if [ -x /etc/init.d/S99hachimodock ]; then /etc/init.d/S99hachimodock stop; fi
cp /tmp/dock-run /opt/hachimodock/dock-run
cp /tmp/S99hachimodock /etc/init.d/S99hachimodock
chmod 755 /opt/hachimodock/dock-run /etc/init.d/S99hachimodock
mkdir -p /etc/default
if [ ! -e /etc/default/hachimodock ]; then
 printf '%s\n' '# Set to 1 only when there is no existing NTP service.' 'DOCK_SYNC_TIME=0' > /etc/default/hachimodock
 chmod 600 /etc/default/hachimodock
fi
rm -f /tmp/dock-run /tmp/S99hachimodock
/etc/init.d/S99hachimodock start
/etc/init.d/S99hachimodock status
REMOTE
