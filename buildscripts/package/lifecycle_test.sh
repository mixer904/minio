#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "${script_dir}/../.." && pwd)"
postinstall="${script_dir}/postinstall.sh"
preremove="${script_dir}/preremove.sh"
test_dir="$(mktemp -d)"
fakebin="${test_dir}/bin"
log_file="${test_dir}/calls.log"
useradd_shell=/usr/sbin/nologin
[ -x "${useradd_shell}" ] || useradd_shell=/sbin/nologin
busybox_shell=/sbin/nologin
[ -x "${busybox_shell}" ] || busybox_shell=/bin/false

cleanup() {
	rm -rf "${test_dir}"
}
trap cleanup EXIT

mkdir -p "${fakebin}"
touch "${log_file}"

# One dispatcher represents every external command used by the lifecycle
# scripts. The tested scripts run with no host utilities in PATH, so a green
# result cannot create a real account or touch the host service manager.
cat > "${fakebin}/fake-command" <<'EOF'
#!/bin/sh
set -eu

command_name=${0##*/}
case "${command_name}" in
	id)
		[ "${PACKAGE_TEST_USER_EXISTS:-0}" = 1 ]
		;;
	getent)
		[ "${PACKAGE_TEST_GROUP_EXISTS:-0}" = 1 ]
		;;
	systemd-sysusers|useradd|addgroup|adduser|systemctl)
		{
			printf '%s' "${command_name}"
			for argument in "$@"; do
				printf ' %s' "${argument}"
			done
			printf '\n'
		} >> "${PACKAGE_TEST_LOG}"
		;;
	*)
		echo "unexpected fake command: ${command_name}" >&2
		exit 1
		;;
esac
EOF
chmod +x "${fakebin}/fake-command"

link_command() {
	ln -sf fake-command "${fakebin}/$1"
}

unlink_optional_commands() {
	rm -f \
		"${fakebin}/systemd-sysusers" \
		"${fakebin}/useradd" \
		"${fakebin}/adduser" \
		"${fakebin}/addgroup"
}

reset_log() {
	: > "${log_file}"
}

run_postinstall() {
	PACKAGE_TEST_LOG="${log_file}" \
	PACKAGE_TEST_USER_EXISTS="${1}" \
	PACKAGE_TEST_GROUP_EXISTS="${2}" \
	PATH="${fakebin}" \
		/bin/sh "${postinstall}"
}

run_preremove() {
	PACKAGE_TEST_LOG="${log_file}" PATH="${fakebin}" \
		/bin/sh "${preremove}" "$@"
}

assert_log_line() {
	grep -Fx -- "$1" "${log_file}" >/dev/null
}

reject_log_text() {
	if grep -F -- "$1" "${log_file}" >/dev/null; then
		echo "unexpected lifecycle call containing '$1':" >&2
		cat "${log_file}" >&2
		exit 1
	fi
}

link_command id
link_command getent
link_command systemctl

# Clean install through systemd-sysusers. Side-by-side safety is represented
# by the fact that the only service-manager operation is daemon-reload: no old
# service is stopped, disabled, enabled, masked, or restarted.
unlink_optional_commands
link_command systemd-sysusers
reset_log
run_postinstall 0 0
assert_log_line "systemd-sysusers /usr/lib/sysusers.d/silo.conf"
assert_log_line "systemctl daemon-reload"
test "$(wc -l < "${log_file}" | tr -d ' ')" -eq 2

# An existing service account is preserved without modification.
reset_log
run_postinstall 1 0
test "$(cat "${log_file}")" = "systemctl daemon-reload"

# useradd creates a private group only when one does not already exist. An
# administrator may pre-create group silo with the legacy GID; that group must
# be reused rather than causing installation to fail.
unlink_optional_commands
link_command useradd
reset_log
run_postinstall 0 0
assert_log_line "useradd --system --user-group --no-create-home --shell ${useradd_shell} --comment Silo object storage service silo"
reject_log_text "--gid silo"

reset_log
run_postinstall 0 1
assert_log_line "useradd --system --gid silo --no-create-home --shell ${useradd_shell} --comment Silo object storage service silo"
reject_log_text "--user-group"

# BusyBox follows the same existing-group contract.
unlink_optional_commands
link_command adduser
link_command addgroup
reset_log
run_postinstall 0 0
assert_log_line "addgroup -S silo"
assert_log_line "adduser -S -D -H -G silo -s ${busybox_shell} silo"

reset_log
run_postinstall 0 1
reject_log_text "addgroup"
assert_log_line "adduser -S -D -H -G silo -s ${busybox_shell} silo"

# Debian remove, RPM erase, and Alpine deinstall stop the Silo unit. Upgrade
# arguments must leave the running service alone.
for removal_argument in remove 0 20260214120000.0.0-r0; do
	reset_log
	run_preremove "${removal_argument}"
	test "$(cat "${log_file}")" = "systemctl disable --now silo.service"
done

for upgrade_argument in upgrade 1; do
	reset_log
	run_preremove "${upgrade_argument}"
	test ! -s "${log_file}"
done

# The package deliberately leaves legacy ownership changes to an explicit
# systemd drop-in. Lifecycle scripts must never rewrite ownership or touch the
# old unit, and the base unit must expose overridable User/Group directives.
grep -Fx 'User=silo' "${repo_dir}/silo.service" >/dev/null
grep -Fx 'Group=silo' "${repo_dir}/silo.service" >/dev/null
if grep -Ein '\b(chown|chgrp|usermod|groupmod)\b|minio\.service' \
	"${postinstall}" "${preremove}"; then
	echo "package lifecycle scripts must not mutate data ownership or the legacy service" >&2
	exit 1
fi

echo "Silo package lifecycle checks passed"
