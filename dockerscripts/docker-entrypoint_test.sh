#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

mkdir -p "${work_dir}/bin"
cat >"${work_dir}/bin/silo" <<'EOF'
#!/bin/sh
printf '%s\n' silo "$@" >"${ENTRYPOINT_CAPTURE}"
if [ -n "${ENTRYPOINT_HOME_CAPTURE:-}" ]; then
	printf '%s\n' "${HOME:-}" >"${ENTRYPOINT_HOME_CAPTURE}"
fi
EOF
chmod +x "${work_dir}/bin/silo"

cat >"${work_dir}/bin/chroot" <<'EOF'
#!/bin/sh
printf '%s\n' "$1" "$2" >"${ENTRYPOINT_CHROOT_CAPTURE}"
shift 2
exec "$@"
EOF
chmod +x "${work_dir}/bin/chroot"

run_case() {
	local name="$1"
	local expected="$2"
	shift 2
	local capture="${work_dir}/${name}.actual"
	local expected_file="${work_dir}/${name}.expected"

	ENTRYPOINT_CAPTURE="${capture}" PATH="${work_dir}/bin:${PATH}" \
		"${script_dir}/docker-entrypoint.sh" "$@"
	printf '%s' "${expected}" >"${expected_file}"
	diff -u "${expected_file}" "${capture}"
}

run_case default $'silo\n'
run_case server $'silo\nserver\n/data\n' server /data
run_case option $'silo\n--version\n' --version
run_case healthcheck $'silo\nhealthcheck\nready\n' healthcheck ready
run_case explicit-silo $'silo\nserver\n/data\n' silo server /data
run_case legacy-minio $'silo\nserver\n/data\n' minio server /data

shell_capture="${work_dir}/shell.actual"
# shellcheck disable=SC2016 # ENTRYPOINT_CAPTURE is expanded by the child shell.
ENTRYPOINT_CAPTURE="${shell_capture}" PATH="${work_dir}/bin:${PATH}" \
	"${script_dir}/docker-entrypoint.sh" sh -c 'printf "%s\n" shell >"${ENTRYPOINT_CAPTURE}"'
test "$(cat "${shell_capture}")" = shell

rootless_capture="${work_dir}/rootless.actual"
rootless_home_capture="${work_dir}/rootless-home.actual"
rootless_chroot_capture="${work_dir}/rootless-chroot.actual"
ENTRYPOINT_CAPTURE="${rootless_capture}" \
	ENTRYPOINT_HOME_CAPTURE="${rootless_home_capture}" \
	ENTRYPOINT_CHROOT_CAPTURE="${rootless_chroot_capture}" \
	HOME=/root MINIO_USERNAME=silo-user MINIO_GROUPNAME=silo-group \
	MINIO_UID=1001 MINIO_GID=1002 PATH="${work_dir}/bin:${PATH}" \
	"${script_dir}/docker-entrypoint.sh" silo --version
test "$(cat "${rootless_home_capture}")" = /tmp
test "$(sed -n '1p' "${rootless_chroot_capture}")" = --userspec=1001:1002
test "$(sed -n '2p' "${rootless_chroot_capture}")" = /
test "$(cat "${rootless_capture}")" = $'silo\n--version'

arbitrary_uid_capture="${work_dir}/arbitrary-uid.actual"
arbitrary_uid_home_capture="${work_dir}/arbitrary-uid-home.actual"
ENTRYPOINT_CAPTURE="${arbitrary_uid_capture}" \
	ENTRYPOINT_HOME_CAPTURE="${arbitrary_uid_home_capture}" \
	HOME="${work_dir}/missing-home" PATH="${work_dir}/bin:${PATH}" \
	"${script_dir}/docker-entrypoint.sh" silo --version
test "$(cat "${arbitrary_uid_home_capture}")" = /tmp
test "$(cat "${arbitrary_uid_capture}")" = $'silo\n--version'

echo "docker entrypoint argv compatibility tests passed"
