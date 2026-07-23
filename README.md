# <img src="public/images/Terraform-LogoMark_onDark.svg" width="30" align="left" style="margin-right: 12px;"/> Terraform MCP Server

The Terraform MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction)
server that provides seamless integration with Terraform Registry APIs, enabling advanced
automation and interaction capabilities for Infrastructure as Code (IaC) development.

## Features

- **Dual Transport Support**: Both Stdio and StreamableHTTP transports with configurable endpoints
- **Terraform Registry Integration**: Direct integration with public Terraform Registry APIs for providers, modules, and policies
- **HCP Terraform & Terraform Enterprise Support**: Full workspace management, organization/project listing, and private registry access
- **Workspace Operations**: Create, update, delete workspaces with support for variables, tags, and run management
- **OTel metrics for monitoring tool usage**: Integration with open telemetry meters to track tool-call volume, latency and failures in Streamable HTTP mode. Also exposes default http server metrics when this feature is enabled


> **Security Note:** Depending on the query, the MCP server may expose certain Terraform data to the MCP client and LLM. Do not use the MCP server with untrusted MCP clients or LLMs.

> **Legal Note:** Your use of a third party MCP Client/LLM is subject solely to the terms of use for such MCP/LLM, and IBM is not responsible for the performance of such third party tools. IBM expressly disclaims any and all warranties and liability for third party MCP Clients/LLMs, and may not be able to provide support to resolve issues which are caused by the third party tools.

> **Caution:**  The outputs and recommendations provided by the MCP server are generated dynamically and may vary based on the query, model, and the connected MCP client. Users should thoroughly review all outputs/recommendations to ensure they align with their organization’s security best practices, cost-efficiency goals, and compliance requirements before implementation.

## Prerequisites

1. Ensure [Docker](https://www.docker.com/) is installed and running to use the server in a containerized environment.
1. Install an AI assistant that supports the Model Context Protocol (MCP).

## Command Line Options

**Environment Variables:**

| Variable | Description | Default |
|----------|-------------|---------|
| `TFE_ADDRESS` | Sets the Terraform Enterprise/HCP Terraform address for API calls. Must include the protocol (e.g., `https://app.terraform.io`). In streamable-http mode this is the only way to set the address; it cannot be supplied by clients via header or query parameter. | Optional |
| `TFE_TOKEN` | Terraform Enterprise API token | `""` (empty) |
| `TF_MCP_SHARED_SECRET` | Shared secret sent as the `X-Tf-Mcp-Secret` header on requests to HCP Terraform / TFE, used to identify requests originating from a hosted MCP deployment. Should only be used over TLS. | `""` (empty) |
| `TFE_SKIP_TLS_VERIFY` | Skip HCP Terraform or Terraform Enterprise TLS verification | `false` |
| `LOG_LEVEL` | Logging level: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` (overrides `--log-level` flag) | `info` |
| `LOG_FORMAT` | Logging format: `text` or `json` (overrides `--log-format` flag)| `text` |
| `TRANSPORT_MODE` | Set to `streamable-http` to enable HTTP transport (legacy `http` value still supported) | `stdio` |
| `TRANSPORT_HOST` | Host to bind the HTTP server | `127.0.0.1` |
| `TRANSPORT_PORT` | HTTP server port | `8080` |
| `MCP_ENDPOINT` | HTTP server endpoint path | `/mcp` |
| `MCP_REDIRECT_ROOT_URL` | URL to redirect requests to `/` to | `""` |
| `MCP_KEEP_ALIVE` | Keep-alive interval for SSE connections (e.g., 30s, 1m). 0 to disable | `0` |
| `MCP_SESSION_MODE` | Session mode: `stateful` or `stateless` | `stateful` |
| `MCP_ALLOWED_ORIGINS` | Comma-separated list of allowed origins for CORS | `""` (empty) |
| `MCP_CORS_MODE` | CORS mode: `strict`, `development`, or `disabled` | `strict` |
| `MCP_TLS_CERT_FILE` | Path to TLS cert file, required for non-localhost deployment (e.g. `/path/to/cert.pem`) | `""` (empty) |
| `MCP_TLS_KEY_FILE` |  Path to TLS key file, required for non-localhost deployment (e.g. `/path/to/key.pem`)| `""` (empty) |
| `MCP_RATE_LIMIT_GLOBAL` | Global rate limit (format: `rps:burst`) | `10:20` |
| `MCP_RATE_LIMIT_SESSION` | Per-session rate limit (format: `rps:burst`) | `5:10` |
| `MCP_ORGANIZATION_ALLOWLIST` | CSV list of HCP Terraform organization names allowed to access the HTTP server | `""` (empty) |
| `MCP_FORWARD_CLIENT_IP` | Forward the client IP to HCP Terraform / TFE via `X-Forwarded-For`. Set to `true` to enable | `false` |
| `MCP_REMOTE_IP_METHOD` | How the client IP is sourced when forwarding is enabled: `RemoteAddr` (direct connection only), `X-Real-IP`, or `X-Forwarded-For` | `RemoteAddr` |
| `MCP_XFF_TRUSTED_HOPS` | Number of trusted proxy hops counted from the right of the `X-Forwarded-For` chain. Only used when `MCP_REMOTE_IP_METHOD=X-Forwarded-For` | `0` |
| `ENABLE_TF_OPERATIONS` | Enable tools that require explicit approval | `false` |
| `OTEL_METRICS_ENABLED` | Enable tools and server metrics using otel | `false` |
| `OTEL_METRICS_SERVICE_VERSION` | Version of the terraform-mcp-server sending metrics, which is used to set metric attributes. It also helps track metrics across different deployments | `latest` |
| `OTEL_METRICS_SERVICE_NAME` | Identifies the source of the metrics (e.g., "terraform-mcp-server") | `terraform-mcp-server` |
| `OTEL_METRICS_EXPORT_INTERVAL` | Controls the frequency of metric flushes | `2` |
| `OTEL_METRICS_ENDPOINT` | URL of your OTel Collector or backend | `localhost:4318` |
| `INSTANA_ENABLED` | Enable Instana instrumentation (metrics and HTTP request tracing) for the streamable-http server. Requires an Instana agent that is reachable by the server. | `false` |


```bash
# Stdio mode
terraform-mcp-server stdio [--log-file /path/to/log] [--log-level info] [--log-format text] [--toolsets <toolsets>] [--tools <tools>]

# StreamableHTTP mode
terraform-mcp-server streamable-http [--transport-port 8080] [--transport-host 127.0.0.1] [--mcp-endpoint /mcp] [--organization-allowlist <orgs-csv>] [--log-file /path/to/log] [--log-level info] [--log-format text] [--toolsets <toolsets>] [--tools <tools>]
```

## Instructions

Default instructions for the MCP server is located in `cmd/terraform-mcp-server/instructions.md`, if those do not seem appropriate for your organization's Terraform practices or if the MCP server is producing inaccurate responses, please replace them with your own instructions and rebuild the container or binary. An example of such instruction is located in `instructions/example-mcp-instructions.md`

`AGENTS.md` essentially behaves as READMEs for coding agents: a dedicated, predictable place to provide the context and instructions to help AI coding agents work on your project. One `AGENTS.md` file works with different coding agents. An example of such instruction is located in `instructions/example-AGENTS.md`, in order to use it commit a file name `AGENTS.md` to the directory where your Terraform configurations reside.

## Installation

### Usage with Visual Studio Code

Add the following JSON block to your User Settings (JSON) file in VS Code. You can do this by pressing `Ctrl + Shift + P` and typing `Preferences: Open User Settings (JSON)`.

More about using MCP server tools in VS Code's [agent mode documentation](https://code.visualstudio.com/docs/copilot/chat/mcp-servers).

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "mcp": {
    "servers": {
      "terraform": {
        "command": "docker",
        "args": [
          "run",
          "-i",
          "--rm",
          "-e", "TFE_TOKEN=${input:tfe_token}",
          "-e", "TFE_ADDRESS=${input:tfe_address}",
          "hashicorp/terraform-mcp-server:1.1.0"
        ]
      }
    },
    "inputs": [
      {
        "type": "promptString",
        "id": "tfe_token",
        "description": "Terraform API Token",
        "password": true
      },
      {
        "type": "promptString",
        "id": "tfe_address",
        "description": "Terraform Address",
        "password": false
      }
    ]
  }
}
```
</td>
<td>

```json
{
  "mcp": {
    "servers": {
      "terraform": {
        "command": "docker",
        "args": [
          "run",
          "-i",
          "--rm",
          "hashicorp/terraform-mcp-server:0.2.3"
        ]
      }
    }
  }
}
```

</td>
</tr>
</table>

Optionally, you can add a similar example (i.e. without the mcp key) to a file called `.vscode/mcp.json` in your workspace. This will allow you to share the configuration with others.

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "servers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "TFE_TOKEN=${input:tfe_token}",
        "-e", "TFE_ADDRESS=${input:tfe_address}",
        "hashicorp/terraform-mcp-server:1.1.0"
      ]
    }
  },
  "inputs": [
    {
      "type": "promptString",
      "id": "tfe_token",
      "description": "Terraform API Token",
      "password": true
    },
    {
      "type": "promptString",
      "id": "tfe_address",
      "description": "Terraform Address",
      "password": false
    }
  ]
}
```

</td>
<td>

```json
{
  "servers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server:0.2.3"
      ]
    }
  }
}
```
</td>
</tr>
</table>


[<img alt="Install in VS Code (docker)" src="https://img.shields.io/badge/VS_Code-VS_Code?style=flat-square&label=Install%20Terraform%20MCP&color=0098FF">](https://vscode.dev/redirect?url=vscode%3Amcp%2Finstall%3F%7B%22name%22%3A%22terraform%22%2C%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22hashicorp%2Fterraform-mcp-server%22%5D%7D)
[<img alt="Install in VS Code Insiders (docker)" src="https://img.shields.io/badge/VS_Code_Insiders-VS_Code_Insiders?style=flat-square&label=Install%20Terraform%20MCP&color=24bfa5">](https://insiders.vscode.dev/redirect?url=vscode-insiders%3Amcp%2Finstall%3F%7B%22name%22%3A%22terraform%22%2C%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22hashicorp%2Fterraform-mcp-server%22%5D%7D)

### Usage with Cursor

Add this to your Cursor config (`~/.cursor/mcp.json`) or via Settings → Cursor Settings → MCP:

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "TFE_ADDRESS=<<PASTE_TFE_ADDRESS_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:1.1.0"
      ]
    }
  }
}
```

</td>
<td>

```json
{
  "servers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server:0.2.3"
      ]
    }
  }
}
```
</td>
</tr>
</table>

<a href="cursor://anysphere.cursor-deeplink/mcp/install?name=terraform&config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCJoYXNoaWNvcnAvdGVycmFmb3JtLW1jcC1zZXJ2ZXIiXX0%3D">
  <img alt="Add terraform MCP server to Cursor" src="https://cursor.com/deeplink/mcp-install-dark.png" height="32" />
</a>

### Usage with Claude Desktop / Amazon Q Developer / Kiro CLI

More about using MCP server tools in Claude Desktop [user documentation](https://modelcontextprotocol.io/quickstart/user). Read more about using MCP server in [Amazon Q Developer](https://docs.aws.amazon.com/amazonq/latest/qdeveloper-ug/qdev-mcp.html) and [Kiro CLI](https://kiro.dev/docs/mcp/).

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "TFE_ADDRESS=<<PASTE_TFE_ADDRESS_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:1.1.0"
      ]
    }
  }
}
```

</td>
<td>

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server:0.2.3"
      ]
    }
  }
}
```
</td>
</tr>
</table>

### Usage with Claude Code

More about using and adding MCP server tools in Claude Code [user documentation](https://docs.claude.com/en/docs/claude-code/mcp)

- Local (`stdio`) Transport

```sh
claude mcp add terraform -s user -t stdio -- docker run -i --rm hashicorp/terraform-mcp-server
```

- Remote (`streamable-http`) Transport

```sh
# Run server (example)
docker run -p 8080:8080 --rm -e TRANSPORT_MODE=streamable-http -e TRANSPORT_HOST=0.0.0.0 hashicorp/terraform-mcp-server

# Add to Claude Code
claude mcp add --transport http terraform http://localhost:8080/mcp
```

### Usage with Codex CLI

[Codex CLI](https://github.com/openai/codex) is OpenAI's official coding agent that runs in your terminal. It supports MCP servers for extended capabilities.

#### Prerequisites

- Node.js 18+ installed
- [Codex CLI](https://github.com/openai/codex) installed: `npm install -g @openai/codex`
- OpenAI API key configured

#### Configuration

Codex CLI uses a JSON configuration file for MCP servers. Add the Terraform MCP server to your Codex config at `~/.codex/config.json`:

**Stdio Transport (Recommended)**

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "TFE_ADDRESS=<<PASTE_TFE_ADDRESS_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:0.4.0"
      ]
    }
  }
}
```

**HTTP Transport**

```bash
# Run the MCP server in HTTP mode
docker run -p 8080:8080 --rm -e TRANSPORT_MODE=streamable-http -e TRANSPORT_HOST=0.0.0.0 hashicorp/terraform-mcp-server:0.4.0

# Add to Codex CLI config
openai config set mcpServers.terraform.url=http://localhost:8080/mcp
```

#### Usage Examples

Once configured, you can use Terraform tools within Codex:

```bash
codex "Search for AWS providers in the Terraform Registry"
codex "List my HCP Terraform workspaces"
codex "Create a new Terraform workspace for the staging environment"
```

For more details on MCP configuration in Codex, refer to the [Codex CLI documentation](https://github.com/openai/codex/tree/main/codex-cli#mcp-agent-mode).


### Usage with Gemini extensions

For security, avoid hardcoding your credentials, create or update `~/.gemini/.env` (where ~ is your home or project directory) for storing HCP Terraform or Terraform Enterprise credentials

```
# ~/.gemini/.env
TFE_ADDRESS=your_tfe_address_here
TFE_TOKEN=your_tfe_token_here
```

Install the extension & run Gemini

```
gemini extensions install https://github.com/hashicorp/terraform-mcp-server
gemini
```

### Usage with Bob IDE / Shell

More about using and adding MCP servers tools in Bob IDE or Shell [Using MCP in Bob](https://bob.ibm.com/docs/ide/configuration/mcp/mcp-in-bob).

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e", "TFE_ADDRESS=<<PASTE_TFE_ADDRESS_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:1.1.0"
      ],
      "disabled": false
    }
  }
}
```

</td>
<td>

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server:0.2.3"
      ],
      "disabled": false
    }
  }
}
```

</td>
</tr>
</table>

## Install from source

Use the latest release version:

```console
go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@latest
```

Use the main branch:

```console
go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@main
```

<table>
<tr><th>Version 0.3.0+ or greater</th><th>Version 0.2.3 or lower</th></tr>
<tr valign=top>
<td>

```json
{
  "mcp": {
    "servers": {
      "terraform": {
        "type": "stdio",
        "command": "/path/to/terraform-mcp-server",
        "env": {
          "TFE_TOKEN": "<<TFE_TOKEN_HERE>>"
        },
      }
    }
  }
}
```

</td>
<td>

```json
{
  "mcp": {
    "servers": {
      "terraform": {
        "type": "stdio",
        "command": "/path/to/terraform-mcp-server"
      }
    }
  }
}
```
</td>
</tr>
</table>

## Building the Docker Image locally

Before using the server, you need to build the Docker image locally:

1. Clone the repository:
```bash
git clone https://github.com/hashicorp/terraform-mcp-server.git
cd terraform-mcp-server
```

2. Build the Docker image:
```bash
make docker-build
```

3. This will create a local Docker image that you can use in the following configuration.

```bash
# Run in stdio mode
docker run -i --rm terraform-mcp-server:dev

# Run in streamable-http mode
docker run -p 8080:8080 --rm -e TRANSPORT_MODE=streamable-http -e TRANSPORT_HOST=0.0.0.0 terraform-mcp-server:dev

# Filter tools (optional)
docker run -i --rm terraform-mcp-server:dev --toolsets=registry,terraform
docker run -i --rm terraform-mcp-server:dev --tools=search_providers,get_provider_details
```

> **Note:** When running in Docker, you should set `TRANSPORT_HOST=0.0.0.0` to allow connections from outside the container.

4. (Optional) Test connection in http mode

```bash
# Test the connection
curl http://localhost:8080/health
```

5. You can use it on your AI assistant as follow:

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "terraform-mcp-server:dev"
      ]
    }
  }
}
```

## Available Tools

[Check out available tools here :link:](https://developer.hashicorp.com/terraform/docs/tools/mcp-server/reference#available-tools)

## Available Resources

[Check out available resources here :link:](https://developer.hashicorp.com/terraform/docs/tools/mcp-server/reference#available-tools)

## Available Metrics

Two kinds of metrics are collected.
First, standard HTTP server metrics are added by wrapping the HTTP mux with otelhttp.NewHandler(...). This emits:

1. http.server.request.body.size
2. http.server.response.body.size
3. http.server.request.duration

Second, the MCP server records custom tool metrics around tool execution using MCP hooks (BeforeCallTool / AfterCallTool). These emit:

1. mcp_tool_calls_total
2. mcp_tool_errors_total
3. mcp_tool_duration_seconds


### Tool Filtering

Control which tools are available using `--toolsets` (groups) or `--tools` (individual):

```bash
# Enable tool groups (default: registry)
terraform-mcp-server --toolsets=registry,terraform

# Enable specific tools only
terraform-mcp-server --tools=search_providers,get_provider_details,list_workspaces
```

Available toolsets: `registry`, `registry-private`, `terraform`, `all`, `default`. See `pkg/toolsets/mapping.go` for individual tool names. Cannot use both flags together.

## Transport Support

The Terraform MCP Server supports multiple transport protocols:

### 1. Stdio Transport (Default)
Standard input/output communication using JSON-RPC messages. Ideal for local development and direct integration with MCP clients.

### 2. StreamableHTTP Transport
Modern HTTP-based transport supporting both direct HTTP requests and Server-Sent Events (SSE) streams. This is the recommended transport for remote/distributed setups.

**Features:**
- **Endpoint**: `http://{hostname}:8080/mcp`
- **Health Check**: `http://{hostname}:8080/health`
- **Environment Configuration**: Set `TRANSPORT_MODE=http` or `TRANSPORT_PORT=8080` to enable
- **Organization Allowlist**: Set `MCP_ORGANIZATION_ALLOWLIST` or `--organization-allowlist` to a CSV list of allowed HCP Terraform organization names

## Session Modes

The Terraform MCP Server supports two session modes when using the StreamableHTTP transport:

- **Stateful Mode (Default)**: Maintains session state between requests, enabling context-aware operations.
- **Stateless Mode**: Each request is processed independently without maintaining session state, which can be useful for high-availability deployments or when using load balancers.

To enable stateless mode, set the environment variable:
```bash
export MCP_SESSION_MODE=stateless
```

## Token Passthrough for Centralized Deployments

When running the MCP server centrally (StreamableHTTP mode) for multiple users, each user can pass their own Terraform token via HTTP headers for RBAC enforcement. This allows a single server instance to serve multiple users with different permissions.

When `MCP_ORGANIZATION_ALLOWLIST` or `--organization-allowlist` is configured, the allowlist must be a CSV list of HCP Terraform organization names. The server requires `Authorization: Bearer <token>` and rejects requests unless that token can access at least one organization in the CSV allowlist. The bearer token takes precedence if the request also includes a `TFE_TOKEN` header, ensuring the token validated by the allowlist is the token used for Terraform API requests. Organization name matching is case-insensitive. If the configured CSV value parses to zero organization names, the server exits with a malformed organization allowlist error.

## Client IP Forwarding

When running the MCP server centrally behind a proxy or load balancer, you can forward the originating client's IP to HCP Terraform / TFE via the `X-Forwarded-For` header. This is off by default and must be enabled with `MCP_FORWARD_CLIENT_IP=true`.

When enabled, the server sources the client IP according to `MCP_REMOTE_IP_METHOD`:

| Method | Behavior |
|--------|----------|
| `RemoteAddr` (default) | Uses only the address of the direct TCP connection. Ignores `X-Forwarded-For` and `X-Real-IP`. |
| `X-Real-IP` | Uses the `X-Real-IP` header if it is a valid IP, otherwise falls back to `RemoteAddr`. |
| `X-Forwarded-For` | Uses the `X-Forwarded-For` chain, selecting the entry `MCP_XFF_TRUSTED_HOPS` positions from the right. Falls back to `RemoteAddr` if the value is missing or invalid. |

### Trust model

`X-Forwarded-For` and `X-Real-IP` are set by clients and intermediary proxies, so they can be spoofed unless a trusted proxy in front of the server overwrites them. For this reason the default is `RemoteAddr`, which trusts only the peer the server is directly connected to. Only enable `X-Real-IP` or `X-Forwarded-For` when the server sits behind a proxy you control that sets these headers.

### Trusted hops

When using `X-Forwarded-For`, `MCP_XFF_TRUSTED_HOPS` is the number of proxies you operate between the server and the internet. Hops are counted from the right of the chain, since each proxy appends the address it received the request from and the rightmost entry is set by the proxy closest to the server. The server skips that many trusted entries and takes the next one to the left.

For example, with `MCP_XFF_TRUSTED_HOPS=1` and a header of `200.1.2.3, 10.1.1.10`, the server selects `200.1.2.3`. With `MCP_XFF_TRUSTED_HOPS=2` and `108.0.0.1, 200.1.2.3, 10.1.1.10, 192.168.0.1`, it selects `200.1.2.3`. If the hop count is greater than the number of entries, or the selected entry is not a valid IP, the server falls back to `RemoteAddr`.

Setting the hop count too low will trust a client-supplied value; setting it too high will trust an address further into your own infrastructure. Set it to the exact number of proxies you run.

### Limitations

- The server reads only the first `X-Forwarded-For` header on a request. It is valid for a request to carry multiple `X-Forwarded-For` headers, but Go's standard library returns only the first, and the server does not join them. If your proxy chain emits multiple headers, configure it to emit a single combined `X-Forwarded-For` header.
- IPv4 and IPv6 addresses are both supported. Values that are not valid IPs are rejected and the server falls back to `RemoteAddr`.

### Migrating from earlier versions

Earlier versions used the leftmost `X-Forwarded-For` value when the header was present, with no configuration. This was insecure, since the leftmost value is the most easily spoofed. The default is now `RemoteAddr`. **If you run the server behind a proxy and rely on `X-Forwarded-For` being forwarded to HCP Terraform / TFE, set `MCP_REMOTE_IP_METHOD=X-Forwarded-For` and `MCP_XFF_TRUSTED_HOPS` to the number of proxies you operate.**

### Supported Headers

| Header | Description |
|--------|-------------|
| `TFE_TOKEN` | Terraform API token |
| `Authorization: Bearer <token>` | Alternative method using standard Bearer auth |
| `TFE_SKIP_TLS_VERIFY` | Skip TLS verification for the request |

### Example: curl

```bash
# Using TFE_TOKEN header
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "TFE_TOKEN: your-user-token" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",...}'

# Using Authorization Bearer header
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-user-token" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize",...}'
```

### Security Considerations

- **TFE_ADDRESS cannot be set by clients.** In streamable-http mode the Terraform address is sourced only from the server-side `TFE_ADDRESS` environment variable (or the default). Requests that attempt to set `TFE_ADDRESS` via HTTP header or query parameter are rejected with a 403. This prevents a client from redirecting requests, and the `Authorization` token, to a malicious server.
- **Hosted deployment identification:** setting `TF_MCP_SHARED_SECRET` sends that value as the `X-Tf-Mcp-Secret` header on every HCP Terraform / TFE request, letting the backend identify requests from a known hosted deployment (e.g. to apply IP allowlists). It is a static secret sent in a header, so only use it over TLS and treat the value as a credential.
- **Never pass tokens in query parameters** - the server will reject such requests with a 400 error.
- Always use TLS (`MCP_TLS_CERT_FILE`/`MCP_TLS_KEY_FILE`) when deploying centrally to protect tokens in transit.
- Configure `MCP_ALLOWED_ORIGINS` to restrict which clients can connect.

### Centralized Deployment Example

```bash
# Start server centrally (no token set server-side)
docker run -p 8080:8080 \
  -e TRANSPORT_MODE=streamable-http \
  -e TRANSPORT_HOST=0.0.0.0 \
  -e TFE_ADDRESS=https://tfe.company.com \
  -e MCP_TLS_CERT_FILE=/certs/server.pem \
  -e MCP_TLS_KEY_FILE=/certs/server-key.pem \
  -e MCP_ALLOWED_ORIGINS=https://ide.company.com \
  -e MCP_ORGANIZATION_ALLOWLIST=team-alpha,team-beta \
  -v /path/to/certs:/certs \
  hashicorp/terraform-mcp-server:1.1.0
```

Users then connect with their individual tokens passed via headers, enabling per-user RBAC enforcement.

## Troubleshooting

### Corporate Proxy / TLS Inspection (Zscaler, etc.)

If you're behind a corporate proxy that performs TLS inspection (like Zscaler Internet Access), you may see certificate errors:
```
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

**Solution: Mount your corporate CA certificate into the container:**
```bash
docker run -i --rm \
  -v /path/to/corporate-ca.pem:/etc/ssl/certs/corporate-ca.pem \
  -e SSL_CERT_FILE=/etc/ssl/certs/corporate-ca.pem \
  hashicorp/terraform-mcp-server:1.1.0
```

For MCP client configurations:
```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/path/to/corporate-ca.pem:/etc/ssl/certs/corporate-ca.pem",
        "-e", "SSL_CERT_FILE=/etc/ssl/certs/corporate-ca.pem",
        "-e", "TFE_TOKEN=<>",
        "hashicorp/terraform-mcp-server:1.1.0"
      ]
    }
  }
}
```

**Alternative: Run the binary directly**

If Docker is not permitted in your environment, you can install and run the server binary directly, which will use your system's certificate store:
```bash
go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@latest
terraform-mcp-server stdio
```
## Development

### Prerequisites
- Go (check [go.mod](./go.mod) file for specific version)
- Docker (optional, for container builds)

### Available Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the binary |
| `make test` | Run all tests |
| `make test-e2e` | Run end-to-end tests |
| `make docker-build` | Build Docker image |
| `make run-http` | Run HTTP server locally |
| `make docker-run-http` | Run HTTP server in Docker |
| `make test-http` | Test HTTP health endpoint |
| `make clean` | Remove build artifacts |
| `make help` | Show all available commands |

## Contributing

1. Fork the repository
2. Create your feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

## License

This project is licensed under the terms of the MPL-2.0 open source license. Please refer to [LICENSE](./LICENSE) file for the full terms.

## Security

For security issues, please contact security@hashicorp.com or follow our [security policy](https://www.hashicorp.com/en/trust/security/vulnerability-management).

## Support

For bug reports and feature requests, please open an issue on GitHub.

For general questions and discussions, open a GitHub Discussion.
