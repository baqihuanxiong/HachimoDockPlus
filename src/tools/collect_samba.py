#!/usr/bin/env python3
"""Collect ARM Samba executables and their ELF dependency closure, not a rootfs."""
from pathlib import Path
import re
import shutil
import subprocess

root = Path(".cache/samba-root")
out = Path("build/samba")
out.mkdir(parents=True, exist_ok=True)
for name in ("bin", "lib", "libexec", "modules", "licenses"):
    (out / name).mkdir(exist_ok=True)
search = [root / "usr/lib/arm-linux-gnueabihf", root / "lib/arm-linux-gnueabihf",
          root / "usr/lib/arm-linux-gnueabihf/samba"]
# The board's glibc is newer than Bookworm's. Do not mix libc/loader versions.
system = {"libc.so.6", "libm.so.6", "libpthread.so.0", "libdl.so.2", "librt.so.1",
          "ld-linux-armhf.so.3", "libresolv.so.2", "libutil.so.1", "libanl.so.1"}
pending, copied = [], set()

def collect(source, target):
    shutil.copyfile(source, target)
    target.chmod(0o755)
    pending.append(source)

for relative in ("usr/sbin/smbd", "usr/bin/smbpasswd", "usr/bin/smbcontrol", "usr/bin/testparm"):
    collect(root / relative, out / "bin" / Path(relative).name)
for name in ("samba-dcerpcd", "rpcd_classic", "rpcd_epmapper", "rpcd_lsad", "rpcd_winreg"):
    collect(root / "usr/libexec/samba" / name, out / "libexec" / name)
# NTLM and the smbpasswd backend are built in. Include installed GENSEC/ldb modules
# for normal standalone-server probing, but not AD's auth_samba4 module.
module_root = search[-1]
for directory in ("gensec", "ldb"):
    for source in sorted((module_root / directory).glob("*.so")):
        destination = out / "modules" / directory / source.name
        destination.parent.mkdir(parents=True, exist_ok=True)
        collect(source, destination)
while pending:
    source = pending.pop()
    info = subprocess.check_output(["readelf", "-d", str(source)], text=True)
    for name in re.findall(r"\(NEEDED\).*\[(.*?)\]", info):
        if name in system or name in copied:
            continue
        resolved = next((directory / name for directory in search if (directory / name).is_file()), None)
        if resolved is None:
            raise SystemExit("Missing ARM shared library: " + name)
        copied.add(name)
        collect(resolved, out / "lib" / name)
for directory in sorted((root / "usr/share/doc").iterdir()):
    copyright = directory / "copyright"
    if copyright.is_file():
        shutil.copyfile(copyright, out / "licenses" / (directory.name + ".txt"))
version = subprocess.check_output(["dpkg-deb", "-f", str(next(Path('.cache/samba-debs').glob('samba_*_armhf.deb'))), "Version"], text=True).strip()
(out / "VERSION.txt").write_text("Debian Bookworm Samba " + version + "\n")
(out / "SOURCES.txt").write_text("Binary packages: Debian bookworm / bookworm-security, armhf\n"
    "Corresponding sources: https://sources.debian.org/src/samba/\n"
    "https://www.debian.org/distrib/packages\n"
    "Samba and bundled dependencies retain their respective licenses in licenses/.\n")
print("Collected", len(copied), "shared libraries;", sum(p.stat().st_size for p in out.rglob('*') if p.is_file()), "bytes")
