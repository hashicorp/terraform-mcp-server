# Terraform MCP Server Helm Chart

A Helm chart for deploying the [Terraform MCP Server](https://github.com/hashicorp/terraform-mcp-server) on Kubernetes in streamable-http mode.

## Prerequisites

- Kubernetes 1.23+
- Helm 3.8+

## Installing

```bash
helm install terraform-mcp-server ./helm/terraform-mcp-server
```

By default this deploys the server pointing at public HCP Terraform (`https://app.terraform.io`) with strict CORS and OpenTelemetry metrics disabled.

To install against a Terraform Enterprise instance and allow a specific client origin:

```bash
helm install terraform-mcp-server ./helm/terraform-mcp-server \
  --set mcpServer.tfeAddress=https://tfe.example.com \
  --set mcpServer.allowedOrigins=https://ide.example.com
```

## Uninstalling

```bash
helm uninstall terraform-mcp-server
```

## Configuration

The server is configured through the `mcpServer` values, which map to the server's environment variables. Only values that are set are passed to the container.

### Common values

| Key | Description | Default |
|-----|-------------|---------|
| `replicaCount` | Number of server replicas | `1` |
| `image.repository` | Server image repository | `hashicorp/terraform-mcp-server` |
| `image.tag` | Image tag (defaults to the chart appVersion) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `resources` | Pod resource requests/limits | see values.yaml |
| `autoscaling.enabled` | Enable a HorizontalPodAutoscaler | `false` |
| `volumes` | Extra volumes for the pod | `[]` |
| `volumeMounts` | Extra volume mounts for the container | `[]` |

### Server configuration (`mcpServer`)

| Key | Description | Default |
|-----|-------------|---------|
| `mcpServer.tfeAddress` | HCP Terraform / TFE address. In streamable-http mode this can only be set here; clients cannot override it. | `https://app.terraform.io` |
| `mcpServer.allowedOrigins` | Comma-separated CORS allowed origins. Empty with strict mode rejects all cross-origin requests. | `""` |
| `mcpServer.corsMode` | CORS mode: `strict`, `development`, or `disabled` | `strict` |
| `mcpServer.allowedOrganizations` | Restrict tool calls to these Terraform organizations. Empty allows any organization the token can reach. | `[]` |
| `mcpServer.sessionMode` | `stateful` or `stateless`. Use `stateless` when running multiple replicas behind a load balancer without session affinity. | `stateful` |
| `mcpServer.enableTfOperations` | Enables the destructive tools (`delete_team`, `force_unlock_workspace`, and similar). | `false` |
| `mcpServer.redirectRootURL` | Where to redirect a browser that hits `/`. Defaults to the Terraform MCP docs page when unset. | `""` |
| `mcpServer.tls.certFile` | Path to a TLS certificate inside the container. Mount it with `volumes` and `volumeMounts`. | `""` |
| `mcpServer.tls.keyFile` | Path to the matching TLS key inside the container. | `""` |
| `mcpServer.logLevel` | Log level | `info` |
| `mcpServer.logFormat` | Log format: `text` or `json` | `json` |
| `mcpServer.heartbeatInterval` | Heartbeat interval for streamable-http; `0` disables | `"0"` |

### TLS

The server can terminate TLS itself rather than relying on an ingress. Mount a certificate and key, then point the server at the mount paths:

```yaml
volumes:
  - name: tls
    secret:
      secretName: terraform-mcp-server-tls

volumeMounts:
  - name: tls
    mountPath: /tls
    readOnly: true

mcpServer:
  tls:
    certFile: /tls/tls.crt
    keyFile: /tls/tls.key
```

### Ingress

| Key | Description | Default |
|-----|-------------|---------|
| `ingress.enabled` | Enable an Ingress resource | `false` |
| `ingress.className` | Ingress class name | `""` |
| `ingress.annotations` | Ingress annotations (set these to match your ingress controller) | `{}` |
| `ingress.hosts` | Ingress hosts and paths | see values.yaml |
| `ingress.tls` | Ingress TLS configuration | `[]` |

Ingress is disabled by default and has no cloud-specific annotations. Set the annotations and class appropriate to your cluster's ingress controller.

### OpenTelemetry metrics

| Key | Description | Default |
|-----|-------------|---------|
| `otel.enabled` | Export OpenTelemetry metrics | `false` |
| `otel.metricsEndpoint` | OTLP metrics endpoint. If unset, the server uses its own default. Set this to your OTLP collector. | `""` |
| `otel.serviceName` | Service name reported in metrics | `terraform-mcp-server` |

Metrics are disabled by default, and none of the `OTEL_` environment variables are set on the container unless `otel.enabled` is true. If you enable them, set `otel.metricsEndpoint` to your OTLP collector endpoint.

The chart sets `OTEL_INSTANCE_ID` from the pod UID so each replica reports a distinct `service.instance.id`. Without it, telemetry from every replica is attributed to a single instance.

## Security notes

- **CORS defaults to strict.** With no `allowedOrigins` set, all cross-origin requests are rejected. Set `mcpServer.allowedOrigins` to your client origin(s).
- **The Terraform address is server-side only.** Clients cannot override `TFE_ADDRESS` via header or query parameter in streamable-http mode; it is fixed by `mcpServer.tfeAddress`.
- **Use TLS.** Either terminate at an ingress or configure `mcpServer.tls` so the server serves TLS itself. Terraform tokens are sent in request headers and must not traverse plaintext connections.
- **Destructive tools are off by default.** `mcpServer.enableTfOperations` gates the tools that delete or force-unlock Terraform resources. Leave it `false` unless you specifically want those available.
