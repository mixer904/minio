#!/bin/bash

if [ -n "$TEST_DEBUG" ]; then
	set -x
fi

pkill silo
docker rm -f $(docker ps -aq)
rm -rf /tmp/ldap{1..4}
rm -rf /tmp/ldap1{1..4}

if [ ! -f ./mc ]; then
	"$(git rev-parse --show-toplevel)/buildscripts/install-mcli.sh" ./mc
fi

mc -v

# Start LDAP server
echo "Copying docs/distributed/samples/bootstrap-complete.ldif => minio-iam-testing/ldap/50-bootstrap.ldif"
cp docs/distributed/samples/bootstrap-complete.ldif minio-iam-testing/ldap/50-bootstrap.ldif || exit 1
cd ./minio-iam-testing
make docker-images
make docker-run
cd -

export MC_HOST_mysilo="http://minioadmin:minioadmin@localhost:22000"
export MC_HOST_mysilo1="http://minioadmin:minioadmin@localhost:24000"

# Start Silo instance
export CI=true
(silo server --address :22000 --console-address :10000 http://localhost:22000/tmp/ldap{1...4} 2>&1 >/dev/null) &
sleep 30
./mc ready mysilo

./mc idp ldap add mysilo server_addr=localhost:389 server_insecure=on \
	lookup_bind_dn=cn=admin,dc=min,dc=io lookup_bind_password=admin \
	user_dn_search_base_dn=dc=min,dc=io user_dn_search_filter="(uid=%s)" \
	group_search_base_dn=ou=swengg,dc=min,dc=io group_search_filter="(&(objectclass=groupOfNames)(member=%d))"

./mc admin service restart mysilo --json
./mc ready mysilo
./mc admin cluster iam import mysilo docs/distributed/samples/myminio-iam-info.zip
sleep 10

# Verify the list of users and service accounts from the import
./mc admin user list mysilo
USER_COUNT=$(./mc admin user list mysilo | wc -l)
if [ "${USER_COUNT}" -ne 2 ]; then
	echo "BUG: Expected no of users: 2 Found: ${USER_COUNT}"
	exit 1
fi
./mc admin user svcacct list mysilo "uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io" --json
SVCACCT_COUNT_1=$(./mc admin user svcacct list mysilo "uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io" --json | jq '.accessKey' | wc -l)
if [ "${SVCACCT_COUNT_1}" -ne 2 ]; then
	echo "BUG: Expected svcacct count for 'uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io': 2. Found: ${SVCACCT_COUNT_1}"
	exit 1
fi
./mc admin user svcacct list mysilo "uid=dillon,ou=people,ou=swengg,dc=min,dc=io" --json
SVCACCT_COUNT_2=$(./mc admin user svcacct list mysilo "uid=dillon,ou=people,ou=swengg,dc=min,dc=io" --json | jq '.accessKey' | wc -l)
if [ "${SVCACCT_COUNT_2}" -ne 2 ]; then
	echo "BUG: Expected svcacct count for 'uid=dillon,ou=people,ou=swengg,dc=min,dc=io': 2. Found: ${SVCACCT_COUNT_2}"
	exit 1
fi

# Kill Silo and LDAP to start afresh with missing groups/DN
pkill silo
docker rm -f $(docker ps -aq)
rm -rf /tmp/ldap{1..4}

# Deploy the LDAP config witg missing groups/DN
echo "Copying docs/distributed/samples/bootstrap-partial.ldif => minio-iam-testing/ldap/50-bootstrap.ldif"
cp docs/distributed/samples/bootstrap-partial.ldif minio-iam-testing/ldap/50-bootstrap.ldif || exit 1
cd ./minio-iam-testing
make docker-images
make docker-run
cd -

(silo server --address ":24000" --console-address :10000 http://localhost:24000/tmp/ldap1{1...4} 2>&1 >/dev/null) &
sleep 30
./mc ready mysilo1

./mc idp ldap add mysilo1 server_addr=localhost:389 server_insecure=on \
	lookup_bind_dn=cn=admin,dc=min,dc=io lookup_bind_password=admin \
	user_dn_search_base_dn=dc=min,dc=io user_dn_search_filter="(uid=%s)" \
	group_search_base_dn=ou=hwengg,dc=min,dc=io group_search_filter="(&(objectclass=groupOfNames)(member=%d))"

./mc admin service restart mysilo1 --json
./mc ready mysilo1
./mc admin cluster iam import mysilo1 docs/distributed/samples/myminio-iam-info.zip
sleep 10

# Verify the list of users and service accounts from the import
./mc admin user list mysilo1
USER_COUNT=$(./mc admin user list mysilo1 | wc -l)
if [ "${USER_COUNT}" -ne 1 ]; then
	echo "BUG: Expected no of users: 1 Found: ${USER_COUNT}"
	exit 1
fi
./mc admin user svcacct list mysilo1 "uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io" --json
SVCACCT_COUNT_1=$(./mc admin user svcacct list mysilo1 "uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io" --json | jq '.accessKey' | wc -l)
if [ "${SVCACCT_COUNT_1}" -ne 2 ]; then
	echo "BUG: Expected svcacct count for 'uid=bobfisher,ou=people,ou=hwengg,dc=min,dc=io': 2. Found: ${SVCACCT_COUNT_1}"
	exit 1
fi
./mc admin user svcacct list mysilo1 "uid=dillon,ou=people,ou=swengg,dc=min,dc=io" --json
SVCACCT_COUNT_2=$(./mc admin user svcacct list mysilo1 "uid=dillon,ou=people,ou=swengg,dc=min,dc=io" --json | jq '.accessKey' | wc -l)
if [ "${SVCACCT_COUNT_2}" -ne 0 ]; then
	echo "BUG: Expected svcacct count for 'uid=dillon,ou=people,ou=swengg,dc=min,dc=io': 0. Found: ${SVCACCT_COUNT_2}"
	exit 1
fi

# Finally kill running processes
pkill silo
docker rm -f $(docker ps -aq)
