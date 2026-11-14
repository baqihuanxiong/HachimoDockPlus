#!/bin/sh
# PAM8403 SPK_EN on GPIO0_B4 (GPIO12), per the baseboard schematic.
set -eu

case "${1:-}" in
    on) level=high ;;
    off) level=low ;;
    *) echo "Usage: $0 on|off" >&2; exit 2 ;;
esac

if [ ! -d /sys/class/gpio/gpio12 ]; then
    echo 12 > /sys/class/gpio/export
fi
# Set direction and initial output level together.
echo "$level" > /sys/class/gpio/gpio12/direction
