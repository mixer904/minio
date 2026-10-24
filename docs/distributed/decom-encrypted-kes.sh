#!/bin/bash

if [ -n "$TEST_DEBUG" ]; then
	set -x
fi

pkill silo
pkill kes
rm -rf /tmp/xl

if [ ! -f ./mc ]; then
	"$(git rev-parse --show-toplevel)/buildscripts/install-mcli.sh" ./mc
fi

if [ ! -f ./kes ]; then
	wget --quiet -O kes https://github.com/minio/kes/releases/latest/download/kes-linux-amd64 &&
		chmod +x kes
fi

if ! openssl version &>/dev/null; then
	apt install openssl || sudo apt install opensssl
fi

# Start KES Server
(./kes server --dev 2>&1 >kes-server.log) &
kes_pid=$!
sleep 5s
API_KEY=$(grep "API Key" <kes-server.log | awk -F" " '{print $3}')
(openssl s_client -connect 127.0.0.1:7373 2>/dev/null 1>public.crt)

export CI=true
export MINIO_KMS_KES_ENDPOINT=https://127.0.0.1:7373
export MINIO_KMS_KES_API_KEY="${API_KEY}"
export MINIO_KMS_KES_KEY_NAME=minio-default-key
export MINIO_KMS_KES_CAPATH=public.crt
export MC_HOST_mysilo="http://minioadmin:minioadmin@localhost:9000/"

(silo server http://localhost:9000/tmp/xl/{1...10}/disk{0...1} 2>&1 >/dev/null) &
pid=$!

./mc ready mysilo

./mc admin user add mysilo/ silo123 silo123
./mc admin user add mysilo/ silo12345 silo12345

./mc admin policy create mysilo/ rw ./docs/distributed/rw.json
./mc admin policy create mysilo/ lake ./docs/distributed/rw.json

./mc admin policy attach mysilo/ rw --user=silo123
./mc admin policy attach mysilo/ lake --user=silo12345

./mc mb -l mysilo/versioned
./mc mb -l mysilo/versioned-1

./mc encrypt set sse-s3 mysilo/versioned
./mc encrypt set sse-kms minio-default-key mysilo/versioned-1

./mc mirror internal mysilo/versioned/ --quiet >/dev/null
./mc mirror internal mysilo/versioned-1/ --quiet >/dev/null

## Soft delete (creates delete markers)
./mc rm -r --force mysilo/versioned >/dev/null
./mc rm -r --force mysilo/versioned-1 >/dev/null

## mirror again to create another set of version on top
./mc mirror internal mysilo/versioned/ --quiet >/dev/null
./mc mirror internal mysilo/versioned-1/ --quiet >/dev/null

expected_checksum=$(./mc cat internal/dsync/drwmutex.go | md5sum)

user_count=$(./mc admin user list mysilo/ | wc -l)
policy_count=$(./mc admin policy list mysilo/ | wc -l)

kill $pid

(silo server http://localhost:9000/tmp/xl/{1...10}/disk{0...1} http://localhost:9001/tmp/xl/{11...30}/disk{0...3} 2>&1 >/tmp/expanded_1.log) &
pid_1=$!

(silo server --address ":9001" http://localhost:9000/tmp/xl/{1...10}/disk{0...1} http://localhost:9001/tmp/xl/{11...30}/disk{0...3} 2>&1 >/tmp/expanded_2.log) &
pid_2=$!

./mc ready mysilo

expanded_user_count=$(./mc admin user list mysilo/ | wc -l)
expanded_policy_count=$(./mc admin policy list mysilo/ | wc -l)

if [ "$user_count" -ne "$expanded_user_count" ]; then
	echo "BUG: original user count differs from expanded setup"
	exit 1
fi

if [ "$policy_count" -ne "$expanded_policy_count" ]; then
	echo "BUG: original policy count  differs from expanded setup"
	exit 1
fi

./mc version info mysilo/versioned | grep -q "versioning is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "expected versioning enabled after expansion"
	exit 1
fi

./mc encrypt info mysilo/versioned | grep -q "Auto encryption 'sse-s3' is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "expected encryption enabled after expansion"
	exit 1
fi

./mc version info mysilo/versioned-1 | grep -q "versioning is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "expected versioning enabled after expansion"
	exit 1
fi

./mc encrypt info mysilo/versioned-1 | grep -q "Auto encryption 'sse-kms' is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "expected encryption enabled after expansion"
	exit 1
fi

./mc mirror cmd mysilo/versioned/ --quiet >/dev/null
./mc mirror cmd mysilo/versioned-1/ --quiet >/dev/null

./mc ls -r mysilo/versioned/ >expanded_ns.txt
./mc ls -r --versions mysilo/versioned/ >expanded_ns_versions.txt
./mc ls -r mysilo/versioned-1/ >expanded_ns_1.txt
./mc ls -r --versions mysilo/versioned-1/ >expanded_ns_versions_1.txt

./mc admin decom start mysilo/ http://localhost:9000/tmp/xl/{1...10}/disk{0...1}

until $(./mc admin decom status mysilo/ | grep -q Complete); do
	echo "waiting for decom to finish..."
	sleep 1s
done

kill $pid_1
kill $pid_2

sleep 5s

(silo server --address ":9001" http://localhost:9001/tmp/xl/{11...30}/disk{0...3} 2>&1 >/tmp/removed.log) &
pid=$!

sleep 30s

export MC_HOST_mysilo="http://minioadmin:minioadmin@localhost:9001/"

./mc ready mysilo

decom_user_count=$(./mc admin user list mysilo/ | wc -l)
decom_policy_count=$(./mc admin policy list mysilo/ | wc -l)

if [ "$user_count" -ne "$decom_user_count" ]; then
	echo "BUG: original user count differs after decommission"
	exit 1
fi

if [ "$policy_count" -ne "$decom_policy_count" ]; then
	echo "BUG: original policy count differs after decommission"
	exit 1
fi

./mc version info mysilo/versioned | grep -q "versioning is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected versioning enabled after decommission"
	exit 1
fi

./mc encrypt info mysilo/versioned | grep -q "Auto encryption 'sse-s3' is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected encryption enabled after expansion"
	exit 1
fi

./mc version info mysilo/versioned-1 | grep -q "versioning is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected versioning enabled after decommission"
	exit 1
fi

./mc encrypt info mysilo/versioned-1 | grep -q "Auto encryption 'sse-kms' is enabled"
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected encryption enabled after expansion"
	exit 1
fi

got_checksum=$(./mc cat mysilo/versioned/dsync/drwmutex.go | md5sum)
if [ "${expected_checksum}" != "${got_checksum}" ]; then
	echo "BUG: decommission failed on encrypted objects: expected ${expected_checksum} got ${got_checksum}"
	exit 1
fi

got_checksum_1=$(./mc cat mysilo/versioned-1/dsync/drwmutex.go | md5sum)
if [ "${expected_checksum}" != "${got_checksum_1}" ]; then
	echo "BUG: decommission failed on encrypted objects: expected ${expected_checksum} got ${got_checksum_1}"
	exit 1
fi

./mc ls -r mysilo/versioned >decommissioned_ns.txt
./mc ls -r --versions mysilo/versioned >decommissioned_ns_versions.txt
./mc ls -r mysilo/versioned-1 >decommissioned_ns_1.txt
./mc ls -r --versions mysilo/versioned-1 >decommissioned_ns_versions_1.txt

out=$(diff -qpruN expanded_ns.txt decommissioned_ns.txt)
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected no missing entries after decommission: $out"
	exit 1
fi

out=$(diff -qpruN expanded_ns_versions.txt decommissioned_ns_versions.txt)
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected no missing entries after decommission: $out"
	exit 1
fi

out1=$(diff -qpruN expanded_ns_1.txt decommissioned_ns_1.txt)
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected no missing entries after decommission: $out1"
	exit 1
fi

out1=$(diff -qpruN expanded_ns_versions_1.txt decommissioned_ns_versions_1.txt)
ret=$?
if [ $ret -ne 0 ]; then
	echo "BUG: expected no missing entries after decommission: $out1"
	exit 1
fi

./s3-check-md5 -versions -access-key minioadmin -secret-key minioadmin -endpoint http://127.0.0.1:9001/ -bucket versioned
./s3-check-md5 -versions -access-key minioadmin -secret-key minioadmin -endpoint http://127.0.0.1:9001/ -bucket versioned-1

kill $pid
kill $kes_pid
