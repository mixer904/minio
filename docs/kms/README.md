# KMS Guide

Silo uses a key-management-system (KMS) to support SSE-S3. If a client requests SSE-S3, or auto-encryption is enabled, the Silo server encrypts each object with a unique object key which is protected by a master key managed by the KMS.

## Quick Start

Silo supports KMS integration through a KES-compatible endpoint. Deploy and
control your own [KES](https://github.com/minio/kes) instance, create a dedicated
client identity and key, and then configure Silo with the compatibility
environment variables below. Do not use a public or shared KES instance for
real data.

### 1. Set the Silo-to-KES configuration

```sh
export MINIO_KMS_KES_ENDPOINT=https://kes.example.com:7373
export MINIO_KMS_KES_KEY_FILE=/etc/silo/kes/client.key
export MINIO_KMS_KES_CERT_FILE=/etc/silo/kes/client.crt
export MINIO_KMS_KES_KEY_NAME=silo-default-key
```

### 2. Start the Silo server

```sh
export MINIO_ROOT_USER=minio
export MINIO_ROOT_PASSWORD=silo123
silo server ~/export
```

The certificate, private key, KES policy, and named key must be provisioned by
the administrator. Protect the client private key and use TLS verification in
every environment.

## Configuration Guides

A typical Silo deployment that uses a KMS for SSE-S3 looks like this:

```
    ┌────────────┐
    │ ┌──────────┴─┬─────╮          ┌────────────┐
    └─┤ ┌──────────┴─┬───┴──────────┤ ┌──────────┴─┬─────────────────╮
      └─┤ ┌──────────┴─┬─────┬──────┴─┤ KES Server ├─────────────────┤
        └─┤   Silo    ├─────╯        └────────────┘            ┌────┴────┐
          └────────────┘                                        │   KMS   │
                                                                └─────────┘
```

In a given setup, there are `n` Silo instances talking to `m` KES servers but only `1` central KMS. The most simple setup consists of `1` Silo server or cluster talking to `1` KMS via `1` KES server.

The main difference between various Silo-KMS deployments is the KMS implementation. The following table helps you select the right option for your use case:

| KMS                                                                                          | Purpose                                                           |
|:---------------------------------------------------------------------------------------------|:------------------------------------------------------------------|
| [Hashicorp Vault](https://github.com/minio/kes/wiki/Hashicorp-Vault-Keystore)                | Local KMS. Silo and KMS on-prem (**Recommended**)                |
| [AWS-KMS + SecretsManager](https://github.com/minio/kes/wiki/AWS-SecretsManager)             | Cloud KMS. Silo in combination with a managed KMS installation   |
| [Gemalto KeySecure /Thales CipherTrust](https://github.com/minio/kes/wiki/Gemalto-KeySecure) | Local KMS. Silo and KMS On-Premises.                             |
| [Google Cloud Platform SecretManager](https://github.com/minio/kes/wiki/GCP-SecretManager)   | Cloud KMS. Silo in combination with a managed KMS installation   |
| [FS](https://github.com/minio/kes/wiki/Filesystem-Keystore)                                  | Local testing or development (**Not recommended for production**) |

The Silo-to-KES configuration is the same regardless of the KMS implementation.
Consult the KES project documentation for the selected keystore.

### Further references

- [Run Silo with TLS / HTTPS](https://silo.pgsty.com/operations/network-encryption/)
- [Tweak the KES server configuration](https://github.com/minio/kes/wiki/Configuration)
- [Run a load balancer in front of KES](https://github.com/minio/kes/wiki/TLS-Proxy)
- [Understand the KES server concepts](https://github.com/minio/kes/wiki/Concepts)

## Auto Encryption

Auto-Encryption is useful when Silo administrator wants to ensure that all data stored on Silo is encrypted at rest.

### Using `mc encrypt` (recommended)

Silo automatically encrypts all objects on buckets if KMS is successfully configured and bucket encryption configuration is enabled for each bucket as shown below:

```
mc encrypt set sse-s3 mysilo/bucket/
```

Verify if Silo has `sse-s3` enabled

```
mc encrypt info mysilo/bucket/
Auto encryption 'sse-s3' is enabled
```

### Using environment (not-recommended)

Silo automatically encrypts all objects on buckets if KMS is successfully configured and following ENV is enabled:

```
export MINIO_KMS_AUTO_ENCRYPTION=on
```

### Verify auto-encryption

> Note that auto-encryption only affects requests without S3 encryption headers. So, if a S3 client sends
> e.g. SSE-C headers, Silo will encrypt the object with the key sent by the client and won't reach out to
> the configured KMS.

To verify auto-encryption, use the following `mc` command:

```
mc cp test.file mysilo/bucket/
test.file:              5 B / 5 B  ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓  100.00% 337 B/s 0s
```

```
mc stat mysilo/bucket/test.file
Name      : test.file
...
Encrypted :
  X-Amz-Server-Side-Encryption: AES256
```

## Encrypted Private Key

Silo supports encrypted KES client private keys. Therefore, you can use
an password-protected private keys for `MINIO_KMS_KES_KEY_FILE`.

When using password-protected private keys for accessing KES you need to
provide the password via:

```
export MINIO_KMS_KES_KEY_PASSWORD=<your-password>
```

Note that Silo only supports encrypted private keys - not encrypted certificates.
Certificates are no secrets and sent in plaintext as part of the TLS handshake.

## Explore Further

- [Use `mc` with Silo Server](https://silo.pgsty.com/reference/minio-mc/)
- [Use `aws-cli` with Silo Server](https://silo.pgsty.com/integrations/aws-cli-with-minio/)
- [Use `minio-go` SDK with Silo Server](https://silo.pgsty.com/developers/go/minio-go/)
- [The Silo documentation website](https://silo.pgsty.com/docs/)
