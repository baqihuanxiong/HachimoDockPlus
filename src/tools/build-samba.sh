#!/bin/sh
# Download ARM packages without running ARM maintainer scripts on the build host.
set -eu
cd "$(dirname "$0")/.."
mkdir -p .cache/samba-debs .cache/samba-root build/samba
tools/dev.sh true
docker run --rm -v "$(pwd):/work" hachimodock-go-lvgl:dev sh -c '
  python3 -c "from pathlib import Path; import shutil; [p.unlink() for p in Path(\"/work/.cache/samba-debs\").glob(\"*.deb\")]; [shutil.rmtree(p, ignore_errors=True) for p in [\"/work/.cache/samba-root\", \"/work/build/samba\"]]"
  dpkg --add-architecture armhf
  apt-get update -qq
  apt-get install --download-only --no-install-recommends -y \
    -o Dir::Cache::archives=/work/.cache/samba-debs \
    samba-libs:armhf libpopt0:armhf libcups2:armhf libbsd0:armhf
  cd /work/.cache/samba-debs
  apt-get download samba:armhf samba-common-bin:armhf
  cd /work
  for package in .cache/samba-debs/*.deb; do dpkg-deb -x "$package" .cache/samba-root; done
  python3 tools/collect_samba.py
  chown -R '"$(id -u):$(id -g)"' build/samba
'
tar -czf build/samba-runtime.tgz -C build/samba bin lib libexec modules licenses VERSION.txt SOURCES.txt
sha256sum build/samba-runtime.tgz
du -h build/samba-runtime.tgz
