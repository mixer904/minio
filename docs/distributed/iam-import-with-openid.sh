#!/bin/bash

if [ -n "$TEST_DEBUG" ]; then
	set -x
fi

pkill silo
docker rm -f $(docker ps -aq)
rm -rf /tmp/openid{1..4}

export MC_HOST_mysilo="http://minioadmin:minioadmin@localhost:22000"
# The service account used below is already present in iam configuration getting imported
export MC_HOST_mysilo1="http://dillon-service-2:dillon-service-2@localhost:22000"

# Start Silo instance
export CI=true

if [ ! -f ./mc ]; then
	"$(git rev-parse --show-toplevel)/buildscripts/install-mcli.sh" ./mc
fi

mc -v

# Start openid server
(
	cd ./minio-iam-testing
	make docker-images
	make docker-run
	cd -
)

(silo server --address :22000 --console-address :10000 http://localhost:22000/tmp/openid{1...4} 2>&1 >/tmp/server.log) &
./mc ready mysilo
./mc mb mysilo/test-bucket
./mc cp /etc/hosts mysilo/test-bucket

./mc idp openid add mysilo \
	config_url="http://localhost:5556/dex/.well-known/openid-configuration" \
	client_id="minio-client-app" \
	client_secret="minio-client-app-secret" \
	scopes="openid,groups,email,profile" \
	redirect_uri="http://127.0.0.1:10000/oauth_callback" \
	display_name="Login via dex1" \
	role_policy="consoleAdmin"

./mc admin service restart mysilo --json
./mc ready mysilo
./mc admin cluster iam import mysilo docs/distributed/samples/myminio-iam-info-openid.zip

# Verify if buckets / objects accessible using service account
echo "Verifying buckets and objects access for the imported service account"

./mc ls mysilo1/ --json
BKT_COUNT=$(./mc ls mysilo1/ --json | jq '.key' | wc -l)
if [ "${BKT_COUNT}" -ne 1 ]; then
	echo "BUG: Expected no of bucket: 1, Found: ${BKT_COUNT}"
	exit 1
fi

BKT_NAME=$(./mc ls mysilo1/ --json | jq '.key' | sed 's/"//g' | sed 's\/\\g')
if [[ ${BKT_NAME} != "test-bucket" ]]; then
	echo "BUG: Expected bucket: test-bucket, Found: ${BKT_NAME}"
	exit 1
fi

./mc ls mysilo1/test-bucket
OBJ_COUNT=$(./mc ls mysilo1/test-bucket --json | jq '.key' | wc -l)
if [ "${OBJ_COUNT}" -ne 1 ]; then
	echo "BUG: Expected no of objects: 1, Found: ${OBJ_COUNT}"
	exit 1
fi

OBJ_NAME=$(./mc ls mysilo1/test-bucket --json | jq '.key' | sed 's/"//g')
if [[ ${OBJ_NAME} != "hosts" ]]; then
	echo "BUG: Expected object: hosts, Found: ${BKT_NAME}"
	exit 1
fi

# Finally kill running processes
pkill silo
docker rm -f $(docker ps -aq)
