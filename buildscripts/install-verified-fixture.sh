#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -ne 3 ]; then
	echo "usage: $0 SOURCE SHA256 TARGET" >&2
	exit 2
fi

source_ref=$1
expected=${2,,}
target=$3
if ! printf '%s\n' "${expected}" | grep -Eq '^[0-9a-f]{64}$'; then
	echo "expected checksum must be a lowercase SHA-256 digest" >&2
	exit 1
fi
if [ ! -d "$(dirname "${target}")" ]; then
	echo "target directory does not exist: $(dirname "${target}")" >&2
	exit 1
fi

tmp_file=$(mktemp "${TMPDIR:-/tmp}/silo-fixture.XXXXXX")
trap 'rm -f "${tmp_file}"' EXIT
case ${source_ref} in
	https://*)
		curl --fail --location --retry 3 --silent --show-error \
			"${source_ref}" --output "${tmp_file}"
		;;
	*)
		if [ ! -f "${source_ref}" ]; then
			echo "fixture is not a regular file: ${source_ref}" >&2
			exit 1
		fi
		cp "${source_ref}" "${tmp_file}"
		;;
esac

if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "${tmp_file}" | awk '{print $1}')
else
	actual=$(shasum -a 256 "${tmp_file}" | awk '{print $1}')
fi
if [ "${actual}" != "${expected}" ]; then
	echo "fixture checksum mismatch: expected ${expected}, got ${actual}" >&2
	exit 1
fi

install -m 0755 "${tmp_file}" "${target}"
