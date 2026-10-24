# How to monitor Silo server with Grafana

[Grafana](https://grafana.com/) allows you to query, visualize, alert on and understand your metrics no matter where they are stored. Create, explore, and share dashboards with your team and foster a data driven culture.

## Prerequisites

- Prometheus and Silo configured as explained in [document here](https://github.com/pgsty/silo/blob/main/docs/metrics/prometheus/README.md).
- Grafana installed as explained [here](https://grafana.com/grafana/download).

## Silo Grafana dashboards

Import the checked-in JSON dashboards for the view you need:

- [Overview dashboard](minio-dashboard.json)
- [Node replication dashboard](replication/minio-replication-node.json)
- [Cluster replication dashboard](replication/minio-replication-cluster.json)
- [Bucket dashboard](bucket/minio-bucket.json)
- [Node dashboard](node/minio-node.json)

The filenames and Prometheus query identifiers retain `minio` for monitoring compatibility. Their displayed product titles use Silo. Treat these dashboards as maintained examples and adapt thresholds, variables, and panels to your deployment.
