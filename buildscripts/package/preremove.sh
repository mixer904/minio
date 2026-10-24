#!/bin/sh

set -eu

# Debian passes "remove" for an actual removal, RPM passes 0 to %preun, and
# Alpine runs pre-deinstall only for removal and passes the old dotted version.
# Upgrade paths deliberately leave the running service untouched.
case "${1:-}" in
	remove|0|*.*)
		if command -v systemctl >/dev/null 2>&1; then
			systemctl disable --now silo.service >/dev/null 2>&1 || true
		fi
		;;
esac
