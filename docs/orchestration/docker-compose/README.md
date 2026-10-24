# Deploy Silo on Docker Compose  [![Docker Pulls](https://img.shields.io/docker/pulls/pgsty/silo.svg?maxAge=604800)](https://hub.docker.com/r/pgsty/silo/)

Docker Compose allows defining and running single host, multi-container Docker applications.

With Compose, you use a Compose file to configure Silo services. Then, using a single command, you can create and launch all the Distributed Silo instances from your configuration. Distributed Silo instances will be deployed in multiple containers on the same host. This is a great way to set up development, testing, and staging environments, based on Distributed Silo.

## 1. Prerequisites

* Familiarity with [Docker Compose](https://docs.docker.com/compose/overview/).
* Docker installed on your machine. Download the relevant installer from [here](https://www.docker.com/community-edition#/download).

## 2. Run Distributed Silo on Docker Compose

To deploy distributed Silo with Docker Compose, download the local
[`docker-compose.yaml`](docker-compose.yaml) and [`nginx.conf`](nginx.conf) into
the same working directory. Compose pulls the Silo image, so no source build is
required. Then run one of the commands below.

### GNU/Linux and macOS

```sh
docker-compose pull
docker-compose up
```

or

```sh
docker stack deploy --compose-file docker-compose.yaml silo
```

### Windows

```sh
docker-compose.exe pull
docker-compose.exe up
```

or

```sh
docker stack deploy --compose-file docker-compose.yaml silo
```

The S3 API is accessible through the load balancer on port 9000 and the Silo
Console on port 9001. Open <http://127.0.0.1:9001/> in a browser. Four Silo
server instances are reverse proxied through Nginx.

### Notes

* By default the Docker Compose file uses the Docker image for latest Silo server release. You can change the image tag to pull a specific [Silo Docker image](https://hub.docker.com/r/pgsty/silo/).

* Four distributed Silo instances are created by default. You can add more
  Silo services (up to 16 total) to the Compose deployment. To add a service:
  * Replicate a service definition and change the name of the new service appropriately.
  * Update the command section in each service.
  * Add a new Silo server instance to the upstream directive in the Nginx configuration file.

  Read more about distributed Silo [here](https://silo.pgsty.com/operations/deployments/baremetal-deploy-minio-as-a-container/).

### Explore Further

* [Overview of Docker Compose](https://docs.docker.com/compose/overview/)
* [Silo Docker Quickstart Guide](https://silo.pgsty.com/operations/deployments/baremetal-deploy-minio-as-a-container/)
* [Silo Erasure Code QuickStart Guide](https://silo.pgsty.com/operations/concepts/erasure-coding/)
