#!/bin/bash

set -e
# Enable tracing if set.
[ -n "$BASH_XTRACEFD" ] && set -x

function _init() {
	## All binaries are static make sure to disable CGO.
	export CGO_ENABLED=0

	## Cross-compile only the OS/arch combinations we actually publish, kept in
	## sync with the goos/goarch matrix in .github/goreleaser.yml. Compile-checking
	## targets we never ship (ppc64le, s390x, mips*, riscv64, 386, arm, the BSDs)
	## spent CI minutes on unshipped code and timed the gate out on a cold cache.
	SUPPORTED_OSARCH="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
}

function _build() {
	local osarch=$1
	IFS=/ read -r -a arr <<<"$osarch"
	os="${arr[0]}"
	arch="${arr[1]}"
	package=$(go list -f '{{.ImportPath}}')
	printf -- "--> %15s:%s\n" "${osarch}" "${package}"

	# go build -trimpath to build the binary.
	export GOOS=$os
	export GOARCH=$arch
	export GO111MODULE=on
	go build -trimpath -tags kqueue -o /dev/null
}

function main() {
	echo "Testing builds for OS/Arch: ${SUPPORTED_OSARCH}"
	for each_osarch in ${SUPPORTED_OSARCH}; do
		_build "${each_osarch}"
	done
}

_init && main "$@"
