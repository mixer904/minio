#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "${script_dir}/.." && pwd)"
dist_dir="${DIST_DIR:-${repo_dir}/dist}"
nfpm_config="${NFPM_CONFIG:-${repo_dir}/.github/nfpm.yml}"

if [ -z "${PKG_VERSION:-}" ]; then
  echo "PKG_VERSION is required" >&2
  exit 1
fi

# Re-validate the shape release.yml derived from the tag. The packages are
# named from this, so a malformed value would ship under a name no repository
# can order against.
if ! [[ "${PKG_VERSION}" =~ ^[0-9]{14}\.0\.0$ ]]; then
  echo "Invalid PKG_VERSION: ${PKG_VERSION}" >&2
  exit 1
fi

# The PGSTY release segment, PGDG-style. sign-release-rpms.sh declares the
# same value as expected_release, and test-release.yml asserts the two agree.
PKG_RELEASE="1PGSTY"

if ! command -v nfpm >/dev/null 2>&1; then
  echo "nfpm is required" >&2
  exit 1
fi

if [ ! -f "${nfpm_config}" ]; then
  echo "Missing nFPM config: ${nfpm_config}" >&2
  exit 1
fi

# nfpm resolves a relative content src against the current directory, not
# against the config file, so the unit path is passed in absolute. Otherwise
# this only works when invoked from the repository root and fails elsewhere on
# a message that names the file rather than the cause.
unit_file="${repo_dir}/silo.service"
defaults_file="${repo_dir}/silo.env"
sysusers_file="${repo_dir}/silo.sysusers"
license_file="${repo_dir}/LICENSE"
notice_file="${repo_dir}/NOTICE"
postinstall_file="${repo_dir}/buildscripts/package/postinstall.sh"
preremove_file="${repo_dir}/buildscripts/package/preremove.sh"
if [ ! -f "${unit_file}" ]; then
  echo "Missing systemd unit: ${unit_file}" >&2
  exit 1
fi
if [ ! -f "${defaults_file}" ]; then
  echo "Missing defaults file: ${defaults_file}" >&2
  exit 1
fi
if [ ! -f "${sysusers_file}" ]; then
  echo "Missing sysusers file: ${sysusers_file}" >&2
  exit 1
fi
for distributed_doc in "${license_file}" "${notice_file}"; do
  if [ ! -s "${distributed_doc}" ]; then
    echo "Missing license material: ${distributed_doc}" >&2
    exit 1
  fi
done
for lifecycle_script in "${postinstall_file}" "${preremove_file}"; do
  if [ ! -x "${lifecycle_script}" ]; then
    echo "Missing executable package lifecycle script: ${lifecycle_script}" >&2
    exit 1
  fi
done

packages_dir="${dist_dir}/packages"
mkdir -p "${packages_dir}"

# Two spaces, no trailing newline: sign-release-rpms.sh parses these files to
# check download integrity before it signs, and regenerates them afterwards in
# the same shape.
sha256_file() {
  local file="$1"
  local digest

  if command -v sha256sum >/dev/null 2>&1; then
    digest="$(sha256sum "${file}" | awk '{print $1}')"
  else
    digest="$(shasum -a 256 "${file}" | awk '{print $1}')"
  fi

  printf '%s  %s' "${digest}" "$(basename "${file}")" > "${file}.sha256sum"
}

find_binary() {
  local goarch="$1"
  local matches
  local count

  # Must resolve to exactly one binary. Picking the first of several build
  # variants (an added goamd64 level, a stale dist entry) would silently ship a
  # package whose contents do not match its name.
  matches="$(find "${dist_dir}" -maxdepth 2 -type f \
    -path "${dist_dir}/silo_linux_${goarch}*/silo" | sort)"
  count="$(printf '%s' "${matches}" | grep -c . || true)"

  if [ "${count}" -eq 0 ]; then
    echo "Missing GoReleaser binary for linux/${goarch}" >&2
    exit 1
  fi
  if [ "${count}" -ne 1 ]; then
    echo "Expected exactly one GoReleaser binary for linux/${goarch}, found ${count}:" >&2
    printf '%s\n' "${matches}" >&2
    exit 1
  fi

  printf '%s\n' "${matches}"
}

build_arch() {
  local goarch="$1"
  local rpm_arch="$2"
  local deb_arch="$3"
  local apk_arch="$4"
  local source
  local rpm_file
  local deb_file
  local apk_file

  source="$(find_binary "${goarch}")"

  # These names are the public download names and must not drift; RPM and DEB
  # carry the PGSTY release number (nfpm renders it as the RPM Release tag and
  # as the Debian revision after a dash). APK stays bare: Alpine pkgrel only
  # admits -r<integer>, so a lettered release cannot ride along there.
  rpm_file="${packages_dir}/silo-${PKG_VERSION}-${PKG_RELEASE}.${rpm_arch}.rpm"
  deb_file="${packages_dir}/silo_${PKG_VERSION}-${PKG_RELEASE}_${deb_arch}.deb"
  apk_file="${packages_dir}/silo_${PKG_VERSION}_${apk_arch}.apk"

  (
    cd "${repo_dir}"
    export NFPM_UNIT="${unit_file}" NFPM_DEFAULTS="${defaults_file}" NFPM_SYSUSERS="${sysusers_file}" \
      NFPM_LICENSE="${license_file}" NFPM_NOTICE="${notice_file}"
    PKG_VERSION="${PKG_VERSION}" NFPM_RELEASE="${PKG_RELEASE}" NFPM_ARCH="${goarch}" NFPM_SOURCE="${source}" \
      nfpm package --config "${nfpm_config}" --packager rpm --target "${rpm_file}"
    PKG_VERSION="${PKG_VERSION}" NFPM_RELEASE="${PKG_RELEASE}" NFPM_ARCH="${goarch}" NFPM_SOURCE="${source}" \
      nfpm package --config "${nfpm_config}" --packager deb --target "${deb_file}"
    PKG_VERSION="${PKG_VERSION}" NFPM_RELEASE='' NFPM_ARCH="${goarch}" NFPM_SOURCE="${source}" \
      nfpm package --config "${nfpm_config}" --packager apk --target "${apk_file}"
  )

  sha256_file "${rpm_file}"
  sha256_file "${deb_file}"
  sha256_file "${apk_file}"
}

build_arch amd64 x86_64 amd64 x86_64
build_arch arm64 aarch64 arm64 aarch64

find "${packages_dir}" -maxdepth 1 -type f | sort
