# Bucket Quota Configuration Quickstart Guide [![Docker Pulls](https://img.shields.io/docker/pulls/pgsty/silo.svg?maxAge=604800)](https://hub.docker.com/r/pgsty/silo/)

![quota](https://raw.githubusercontent.com/pgsty/silo/main/docs/bucket/quota/bucketquota.png)

Buckets can be configured to have `Hard` quota - it disallows writes to the bucket after configured quota limit is reached.

## Prerequisites

- Install Silo - [Silo Quickstart Guide](https://silo.pgsty.com/operations/deployments/baremetal-deploy-minio-on-redhat-linux/#procedure).
- [Use `mc` with Silo Server](https://silo.pgsty.com/reference/minio-mc/#quickstart)

## Set bucket quota configuration

### Set a hard quota of 1GB for a bucket `mybucket` on Silo object storage

```sh
mc admin bucket quota mysilo/mybucket --hard 1gb
```

### Verify the quota configured on `mybucket` on Silo

```sh
mc admin bucket quota mysilo/mybucket
```

### Clear bucket quota configuration for `mybucket` on Silo

```sh
mc admin bucket quota mysilo/mybucket --clear
```
