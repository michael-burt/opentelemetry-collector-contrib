# Aerospike Receiver

<!-- status autogenerated section -->
| Status        |           |
| ------------- |-----------|
| Stability     | [alpha]: metrics   |
| Distributions | [contrib] |
| Issues        | [![Open issues](https://img.shields.io/github/issues-search/open-telemetry/opentelemetry-collector-contrib?query=is%3Aissue%20is%3Aopen%20label%3Areceiver%2Faerospike%20&label=open&color=orange&logo=opentelemetry)](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues?q=is%3Aopen+is%3Aissue+label%3Areceiver%2Faerospike) [![Closed issues](https://img.shields.io/github/issues-search/open-telemetry/opentelemetry-collector-contrib?query=is%3Aissue%20is%3Aclosed%20label%3Areceiver%2Faerospike%20&label=closed&color=blue&logo=opentelemetry)](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues?q=is%3Aclosed+is%3Aissue+label%3Areceiver%2Faerospike) |
| Code coverage | [![codecov](https://codecov.io/github/open-telemetry/opentelemetry-collector-contrib/graph/main/badge.svg?component=receiver_aerospike)](https://app.codecov.io/gh/open-telemetry/opentelemetry-collector-contrib/tree/main/?components%5B0%5D=receiver_aerospike&displayType=list) |
| [Code Owners](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/CONTRIBUTING.md#becoming-a-code-owner)    | [@antonblock](https://www.github.com/antonblock) \| Seeking more code owners! |

[alpha]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/component-stability.md#alpha
[contrib]: https://github.com/open-telemetry/opentelemetry-collector-releases/tree/main/distributions/otelcol-contrib
<!-- end autogenerated section -->

The Aerospike receiver is designed to collect performance metrics from one or
more Aerospike nodes. It uses the
[official Go client](https://github.com/aerospike/aerospike-client-go/tree/v5/)
to connect and collect.

Aerospike versions 4.9, 5.x, and 6.x are supported.


## Configuration

Configuration parameters:

- `endpoint` (default localhost:3000): Aerospike host ex: 127.0.0.1:3000.
- `tlsname` Endpoint tls name. Used by the client during TLS connections. See [Aerospike authentication](https://docs.aerospike.com/server/guide/security/tls#standard-authentication) for mor details.
- `collect_cluster_metrics` (default false): Whether discovered peer nodes should be collected.
- `collection_interval` (default = 60s): This receiver collects metrics on an interval. Valid time units are ns, us (or µs), ms, s, m, h.
- `initial_delay` (default = `1s`): defines how long this receiver waits before starting.
- `username` (Enterprise Edition only.)
- `password` (Enterprise Edition only.)
- `tls` (default empty/no tls) tls configuration for connection to Aerospike nodes. More information at [OpenTelemetry's tls config page](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/README.md).

### Example Configuration

```yaml
receivers:
    aerospike:
        endpoint: "localhost:3000"
        tlsname: ""
        collect_cluster_metrics: false
        collection_interval: 30s
```

## Metrics

Details about the metrics produced by this receiver can be found in [metadata.yaml](./metadata.yaml)
