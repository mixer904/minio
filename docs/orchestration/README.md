# Silo Deployment Quickstart Guide [![Docker Pulls](https://img.shields.io/docker/pulls/pgsty/silo.svg?maxAge=604800)](https://hub.docker.com/r/pgsty/silo/)

Silo is a cloud-native application designed to scale in a sustainable manner in multi-tenant environments. Orchestration platforms provide perfect launchpad for Silo to scale. Below is the list of Silo deployment documents for various orchestration platforms:

| Orchestration platforms                                                                            |
|:---------------------------------------------------------------------------------------------------|
| [`Kubernetes`](https://silo.pgsty.com/operations/deployments/kubernetes/)                                |

## Why is Silo cloud-native?

The term cloud-native revolves around the idea of applications deployed as micro services, that scale well. It is not about just retrofitting monolithic applications onto modern container based compute environment. A cloud-native application is portable and resilient by design, and can scale horizontally by simply replicating. Modern orchestration platforms like Kubernetes, DC/OS make replicating and managing containers in huge clusters easier than ever.

While containers provide isolated application execution environment, orchestration platforms allow seamless scaling by helping replicate and manage containers. Silo extends this by adding isolated storage environment for each tenant.

Silo is built ground up on the cloud-native premise. With features like erasure-coding, distributed and shared setup, it focuses only on storage and does it very well. While, it can be scaled by just replicating Silo instances per tenant via an orchestration platform.

> In a cloud-native environment, scalability is not a function of the application but the orchestration platform.

In a typical modern infrastructure deployment, application, database, key-store, etc. already live in containers and are managed by orchestration platforms. Silo brings robust, scalable, AWS S3 compatible object storage to the lot.

```mermaid
flowchart LR
    users["Applications and users"] --> gateway["Ingress or load balancer"]
    gateway --> silo1["Silo tenant A"]
    gateway --> silo2["Silo tenant B"]
    orchestrator["Kubernetes or another orchestrator"] --> silo1
    orchestrator --> silo2
    silo1 --> storage1["Dedicated persistent storage"]
    silo2 --> storage2["Dedicated persistent storage"]
```
