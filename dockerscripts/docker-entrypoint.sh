#!/bin/sh
#

# Run Silo by default, while translating the legacy argv-level command name.
# An explicitly supplied shell or utility remains an explicit entrypoint command.
case "${1:-}" in
	"")
		set -- silo
		;;
	minio)
		shift
		set -- silo "$@"
		;;
	silo)
		;;
	-*|server|fmt-gen|healthcheck)
		set -- silo "$@"
		;;
esac

ensure_writable_home() {
	# The image is commonly run with an arbitrary UID or with the legacy
	# MINIO_USERNAME drop-user path. Do not leave those processes pointing at
	# root's inaccessible home: Silo probes its default configuration directory
	# during process initialization.
	if [ -z "${HOME:-}" ] || [ "${HOME}" = /root ] || [ ! -d "${HOME}" ] || [ ! -w "${HOME}" ]; then
		HOME=/tmp
		export HOME
	fi
}

docker_switch_user() {
	ensure_writable_home
	if [ -n "${MINIO_USERNAME}" ] && [ -n "${MINIO_GROUPNAME}" ]; then
		if [ -n "${MINIO_UID}" ] && [ -n "${MINIO_GID}" ]; then
			exec chroot --userspec="${MINIO_UID}:${MINIO_GID}" / "$@"
		else
			echo "${MINIO_USERNAME}:x:1000:1000:${MINIO_USERNAME}:/:/sbin/nologin" >>/etc/passwd
			echo "${MINIO_GROUPNAME}:x:1000" >>/etc/group
			exec chroot --userspec="${MINIO_USERNAME}:${MINIO_GROUPNAME}" / "$@"
		fi
	else
		exec "$@"
	fi
}

## DEPRECATED and unsupported - switch to user if applicable.
docker_switch_user "$@"
