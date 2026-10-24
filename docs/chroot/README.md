# Deploy Silo on Chrooted Environment [![Docker Pulls](https://img.shields.io/docker/pulls/pgsty/silo.svg?maxAge=604800)](https://hub.docker.com/r/pgsty/silo/)

Chroot allows user based namespace isolation on many standard Linux deployments.

## 1. Prerequisites

- Familiarity with [chroot](http://man7.org/linux/man-pages/man2/chroot.2.html)
- Chroot installed on your machine.

## 2. Install Silo in Chroot

Build Silo from source and install it in the chroot directory:

```sh
# From a checked-out Silo source tree
go build -o silo .

# Create the bin directory in your chroot
mkdir -p /mnt/export/${USER}/bin

# Copy the built binary to the chroot directory
cp ./silo /mnt/export/${USER}/bin/silo
chmod +x /mnt/export/${USER}/bin/silo
```

Versioned binaries and native packages are also available from the
[Silo download page](https://silo.pgsty.com/download/).

Bind your `proc` mount to the target chroot directory

```
sudo mount --bind /proc /mnt/export/${USER}/proc
```

## 3. Run Standalone Silo in Chroot

### GNU/Linux

```sh
sudo chroot --userspec username:group /mnt/export/${USER} /bin/silo --config-dir=/.silo server /data

Endpoint:  http://192.168.1.92:9000  http://65.19.167.92:9000
AccessKey: MVPSPBW4NP2CMV1W3TXD
SecretKey: X3RKxEeFOI8InuNWoPsbG+XEVoaJVCqbvxe+PTOa
...
...
```

Instance is now accessible on the host at port 9000, proceed to access the Web browser at <http://127.0.0.1:9000/>

## Explore Further

- [Silo Erasure Code Overview](https://silo.pgsty.com/operations/concepts/erasure-coding/)
- [Use `mc` with Silo Server](https://silo.pgsty.com/reference/minio-mc/)
- [Use `aws-cli` with Silo Server](https://silo.pgsty.com/integrations/aws-cli-with-minio/)
- [Use `minio-go` SDK with Silo Server](https://silo.pgsty.com/developers/go/minio-go/)
- [The Silo documentation website](https://silo.pgsty.com/docs/)
