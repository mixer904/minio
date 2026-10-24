#!/usr/bin/env bash

set -euo pipefail

# Exercise both directions of the on-disk compatibility contract using an
# immutable pre-rebrand image and a container built from the current checkout.
# Every Docker resource is uniquely named and removed explicitly; this test
# never prunes unrelated images, containers, networks, or volumes.

repo_dir="$(git rev-parse --show-toplevel)"
old_image="${OLD_IMAGE:-docker.io/pgsty/minio@sha256:b6bfe7239bfc83fb90d31612d9704d86039dd714f7904b3f1ad68f211e602372}"
new_image="${NEW_IMAGE:-silo-upgrade-test:dev}"
suffix="$(date +%s)-$$"
network="silo-upgrade-net-${suffix}"
volume="silo-upgrade-data-${suffix}"
old_container="silo-upgrade-old-${suffix}"
new_container="silo-upgrade-new-${suffix}"
rollback_container="silo-upgrade-rollback-${suffix}"
root_user=silo-upgrade-admin
root_password=silo-upgrade-secret-123

cleanup() {
	status=$?
	trap - EXIT
	if [ "${status}" -ne 0 ]; then
		for name in "${old_container}" "${new_container}" "${rollback_container}"; do
			docker logs "${name}" 2>/dev/null | tail -n 80 >&2 || true
		done
	fi
	if [ "${KEEP_UPGRADE_TEST_RESOURCES:-0}" = 1 ]; then
		printf 'Retained Docker resources for inspection: %s %s\n' "${network}" "${volume}" >&2
		exit "${status}"
	fi
	docker rm -f "${old_container}" "${new_container}" "${rollback_container}" >/dev/null 2>&1 || true
	docker network rm "${network}" >/dev/null 2>&1 || true
	docker volume rm "${volume}" >/dev/null 2>&1 || true
	exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

wait_ready() {
	name="$1"
	ready=""
	for _ in $(seq 1 90); do
		if docker logs "${name}" 2>&1 | grep -q 'API:'; then
			ready=1
			break
		fi
		if [ "$(docker inspect -f '{{.State.Running}}' "${name}")" != true ]; then
			break
		fi
		sleep 1
	done
	if [ -z "${ready}" ]; then
		echo "Server did not become ready: ${name}" >&2
		return 1
	fi
}

start_server() {
	name="$1"
	image="$2"
	shift 2
	docker run -d --name "${name}" --network "${network}" \
		--mount "source=${volume},target=/data" \
		-e MINIO_CI_CD=1 -e MINIO_ROOT_USER="${root_user}" -e MINIO_ROOT_PASSWORD="${root_password}" \
		"${image}" "$@" >/dev/null
	wait_ready "${name}"
	docker exec "${name}" mcli alias set local http://127.0.0.1:9000 "${root_user}" "${root_password}" >/dev/null
}

stop_server() {
	name="$1"
	docker stop -t 20 "${name}" >/dev/null
	test "$(docker inspect -f '{{.State.ExitCode}}' "${name}")" = 0
	docker logs "${name}" 2>&1 | grep -q 'Exiting on signal'
	docker rm "${name}" >/dev/null
}

command -v docker >/dev/null
docker info >/dev/null
if ! docker image inspect "${old_image}" >/dev/null 2>&1; then
	docker pull "${old_image}"
fi

if [ "${SILO_UPGRADE_SKIP_BUILD:-0}" != 1 ]; then
	make -C "${repo_dir}" docker TAG="${new_image}"
fi
docker image inspect "${new_image}" >/dev/null

docker network create "${network}" >/dev/null
docker volume create "${volume}" >/dev/null

start_server "${old_container}" "${old_image}" minio server /data --address :9000
docker exec "${old_container}" mcli mb local/compat >/dev/null
docker exec "${old_container}" mcli version enable local/compat >/dev/null
printf 'old-version-1\n' | docker exec -i "${old_container}" mcli pipe local/compat/versioned.txt >/dev/null
printf 'old-version-2\n' | docker exec -i "${old_container}" mcli pipe local/compat/versioned.txt >/dev/null
test "$(docker exec "${old_container}" mcli ls --versions local/compat/versioned.txt | grep -c 'versioned.txt')" -ge 2
docker exec "${old_container}" mcli mb --with-lock local/locked >/dev/null
printf 'locked-by-old\n' | docker exec -i "${old_container}" mcli pipe local/locked/object.txt >/dev/null
dd if=/dev/zero bs=1048576 count=70 2>/dev/null | docker exec -i "${old_container}" mcli pipe local/compat/multipart.bin >/dev/null
test "$(docker exec "${old_container}" mcli stat --json local/compat/multipart.bin | jq -r '.size')" = 73400320
docker exec "${old_container}" mcli admin user add local migration-user migration-secret-123 >/dev/null
docker exec "${old_container}" mcli admin policy attach local readwrite --user migration-user >/dev/null
stop_server "${old_container}"

start_server "${new_container}" "${new_image}" silo server /data --address :9000
test "$(docker exec "${new_container}" mcli cat local/compat/versioned.txt)" = old-version-2
test "$(docker exec "${new_container}" mcli cat local/locked/object.txt)" = locked-by-old
test "$(docker exec "${new_container}" mcli stat --json local/compat/multipart.bin | jq -r '.size')" = 73400320
docker exec "${new_container}" mcli admin user info local migration-user >/dev/null
docker exec "${new_container}" mcli alias set migrated http://127.0.0.1:9000 migration-user migration-secret-123 >/dev/null
printf 'written-by-silo\n' | docker exec -i "${new_container}" mcli pipe migrated/compat/silo.txt >/dev/null
stop_server "${new_container}"

start_server "${rollback_container}" "${old_image}" minio server /data --address :9000
test "$(docker exec "${rollback_container}" mcli cat local/compat/versioned.txt)" = old-version-2
test "$(docker exec "${rollback_container}" mcli cat local/compat/silo.txt)" = written-by-silo
test "$(docker exec "${rollback_container}" mcli stat --json local/compat/multipart.bin | jq -r '.size')" = 73400320
stop_server "${rollback_container}"

echo "MinIO-to-Silo data upgrade and rollback checks passed"
