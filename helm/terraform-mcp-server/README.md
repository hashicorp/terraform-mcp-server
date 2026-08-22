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

### Server configuration (`mcpServer`)

| Key | Description | Default |
|-----|-------------|---------|
| `mcpServer.tfeAddress` | HCP Terraform / TFE address. In streamable-http mode this can only be set here; clients cannot override it. | `https://app.terraform.io` |
| `mcpServer.allowedOrigins` | Comma-separated CORS allowed origins. Empty with strict mode rejects all cross-origin requests. | `""` |
| `mcpServer.corsMode` | CORS mode: `strict`, `development`, or `disabled` | `strict` |
| `mcpServer.sharedSecret` | Optional shared secret sent as the `X-Tf-Mcp-Secret` header to identify a hosted deployment. Treat as a credential. | `""` |
| `mcpServer.logLevel` | Log level | `info` |
| `mcpServer.logFormat` | Log format: `text` or `json` | `json` |
| `mcpServer.heartbeatInterval` | Heartbeat interval for streamable-http; `0` disables | `"0"` |

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

Metrics are disabled by default. If you enable them, set `otel.metricsEndpoint` to your OTLP collector endpoint.

## Security notes

- **CORS defaults to strict.** With no `allowedOrigins` set, all cross-origin requests are rejected. Set `mcpServer.allowedOrigins` to your client origin(s).
- **The Terraform address is server-side only.** Clients cannot override `TFE_ADDRESS` via header or query parameter in streamable-http mode; it is fixed by `mcpServer.tfeAddress`.
- **Use TLS in front of the server.** Deploy behind an ingress or service that terminates TLS. The shared secret and any tokens are sent in headers and must not traverse plaintext connections.
- **`mcpServer.sharedSecret` is rendered into the pod environment.** For sensitive deployments, prefer supplying it via a values file you keep out of source control, or set it with `--set` at install time.
