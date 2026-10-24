# Deploy Silo on Kubernetes  [![Docker Pulls](https://img.shields.io/docker/pulls/pgsty/silo.svg?maxAge=604800)](https://hub.docker.com/r/pgsty/silo/)

Silo is a high performance distributed object storage server, designed for large-scale private cloud infrastructure. Silo is designed in a cloud-native manner to scale sustainably in multi-tenant environments. Orchestration platforms like Kubernetes provide perfect cloud-native environment to deploy and scale Silo.

## Silo Deployment on Kubernetes

There are multiple options to deploy Silo on Kubernetes:

- The Silo Helm chart in [`helm/silo`](../../../helm/silo) supports direct
  Kubernetes deployment. The upstream MinIO Operator is a separate project;
  using it with Silo requires explicit image, command, and compatibility
  validation.

- See the chart's [migration notes](../../../helm/silo/README.md) before
  upgrading an existing release so selectors, names, and service accounts stay
  stable.

## Monitoring Silo in Kubernetes

Silo server exposes un-authenticated liveness endpoints so Kubernetes can natively identify unhealthy Silo containers. Silo also exposes Prometheus compatible data on a different endpoint to enable Prometheus users to natively monitor their Silo deployments.

## Explore Further

- [Silo Erasure Code QuickStart Guide](https://silo.pgsty.com/operations/concepts/erasure-coding/)
- [Kubernetes Documentation](https://kubernetes.io/docs/home/)
- [Helm package manager for kubernetes](https://helm.sh/)
