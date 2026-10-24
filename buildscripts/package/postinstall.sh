#!/bin/sh

set -eu

sysusers_file=/usr/lib/sysusers.d/silo.conf

group_exists() {
	if command -v getent >/dev/null 2>&1; then
		getent group silo >/dev/null 2>&1
		return
	fi

	[ -r /etc/group ] || return 1
	while IFS=: read -r group_name _; do
		[ "${group_name}" = silo ] && return 0
	done < /etc/group
	return 1
}

if ! id -u silo >/dev/null 2>&1; then
	if command -v systemd-sysusers >/dev/null 2>&1; then
		systemd-sysusers "${sysusers_file}"
	elif command -v useradd >/dev/null 2>&1; then
		nologin_shell=/usr/sbin/nologin
		[ -x "${nologin_shell}" ] || nologin_shell=/sbin/nologin
		if group_exists; then
			useradd --system --gid silo --no-create-home --shell "${nologin_shell}" --comment "Silo object storage service" silo
		else
			useradd --system --user-group --no-create-home --shell "${nologin_shell}" --comment "Silo object storage service" silo
		fi
	elif command -v adduser >/dev/null 2>&1 && command -v addgroup >/dev/null 2>&1; then
		nologin_shell=/sbin/nologin
		[ -x "${nologin_shell}" ] || nologin_shell=/bin/false
		group_exists || addgroup -S silo
		adduser -S -D -H -G silo -s "${nologin_shell}" silo
	else
		echo "Unable to create the silo system account: systemd-sysusers, useradd, or BusyBox adduser is required" >&2
		exit 1
	fi
fi

if command -v systemctl >/dev/null 2>&1; then
	systemctl daemon-reload >/dev/null 2>&1 || true
fi
