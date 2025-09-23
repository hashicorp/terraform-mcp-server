# <img src="public/images/Terraform-LogoMark_onDark.svg" width="30" align="left" style="margin-right: 12px;"/> Terraform MCP Server

The Terraform MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction)
server that provides seamless integration with Terraform Registry APIs, enabling advanced
automation and interaction capabilities for Infrastructure as Code (IaC) development.

## Features

- **Dual Transport Support**: Both Stdio and StreamableHTTP transports
- **Terraform Provider Discovery**: Query and explore Terraform providers and their documentation
- **Module Search & Analysis**: Search and retrieve detailed information about Terraform modules
- **Registry Integration**: Direct integration with Terraform Registry APIs
- **Container Ready**: Docker support for easy deployment

> **Caution:** The outputs and recommendations provided by the MCP server are generated dynamically and may vary based on the query, model, and the connected MCP server. Users should **thoroughly review all outputs/recommendations** to ensure they align with their organization's **security best practices**, **cost-efficiency goals**, and **compliance requirements** before implementation.

> **Security Note:** When using the StreamableHTTP transport in production, always configure the `MCP_ALLOWED_ORIGINS` environment variable to restrict access to trusted origins only. This helps prevent DNS rebinding attacks and other cross-origin vulnerabilities.

## Prerequisites

1. To run the server in a container, you will need to have [Docker](https://www.docker.com/) installed.
2. Once Docker is installed, you will need to ensure Docker is running.

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

**Environment Variables:**

| Variable | Description | Default |
|----------|-------------|---------|
| `TRANSPORT_MODE` | Set to `streamable-http` to enable HTTP transport (legacy `http` value still supported) | `stdio` |
| `TRANSPORT_HOST` | Host to bind the HTTP server | `127.0.0.1` |
| `TRANSPORT_PORT` | HTTP server port | `8080` |
| `MCP_ENDPOINT` | HTTP server endpoint path | `/mcp` |
| `MCP_SESSION_MODE` | Session mode: `stateful` or `stateless` | `stateful` |
| `MCP_ALLOWED_ORIGINS` | Comma-separated list of allowed origins for CORS | `""` (empty) |
| `MCP_CORS_MODE` | CORS mode: `strict`, `development`, or `disabled` | `strict` |
| `MCP_TLS_CERT_FILE` | Path to TLS cert file, required for non-localhost deployment (e.g. `/path/to/cert.pem`) | `""` (empty) |
| `MCP_TLS_KEY_FILE` |  Path to TLS key file, required for non-localhost deployment (e.g. `/path/to/key.pem`)| `""` (empty) |
| `MCP_RATE_LIMIT_GLOBAL` | Global rate limit (format: `rps:burst`) | `10:20` |
| `MCP_RATE_LIMIT_SESSION` | Per-session rate limit (format: `rps:burst`) | `5:10` |
| `ENABLE_TF_OPERATIONS` | Enable tools that require explicit approval | `false` |

## Command Line Options

```bash
# Stdio mode
terraform-mcp-server stdio [--log-file /path/to/log]

# StreamableHTTP mode
terraform-mcp-server streamable-http [--transport-port 8080] [--transport-host 127.0.0.1] [--mcp-endpoint /mcp] [--log-file /path/to/log]
```

## Session Modes

The Terraform MCP Server supports two session modes when using the StreamableHTTP transport:

- **Stateful Mode (Default)**: Maintains session state between requests, enabling context-aware operations.
- **Stateless Mode**: Each request is processed independently without maintaining session state, which can be useful for high-availability deployments or when using load balancers.

To enable stateless mode, set the environment variable:
```bash
export MCP_SESSION_MODE=stateless
```

## Installation

### Usage with VS Code

Add the following JSON block to your User Settings (JSON) file in VS Code. You can do this by pressing `Ctrl + Shift + P` and typing `Preferences: Open User Settings (JSON)`. 

More about using MCP server tools in VS Code's [agent mode documentation](https://code.visualstudio.com/docs/copilot/chat/mcp-servers).

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
          "hashicorp/terraform-mcp-server"
        ]
      }
    }
  }
}
```

Optionally, you can add a similar example (i.e. without the mcp key) to a file called `.vscode/mcp.json` in your workspace. This will allow you to share the configuration with others.

```json
{
  "servers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server"
      ]
    }
  }
}
```


[<img alt="Install in VS Code (docker)" src="https://img.shields.io/badge/VS_Code-VS_Code?style=flat-square&label=Install%20Terraform%20MCP&color=0098FF">](https://vscode.dev/redirect?url=vscode%3Amcp%2Finstall%3F%7B%22name%22%3A%22terraform%22%2C%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22hashicorp%2Fterraform-mcp-server%22%5D%7D)
[<img alt="Install in VS Code Insiders (docker)" src="https://img.shields.io/badge/VS_Code_Insiders-VS_Code_Insiders?style=flat-square&label=Install%20Terraform%20MCP&color=24bfa5">](https://insiders.vscode.dev/redirect?url=vscode-insiders%3Amcp%2Finstall%3F%7B%22name%22%3A%22terraform%22%2C%22command%22%3A%22docker%22%2C%22args%22%3A%5B%22run%22%2C%22-i%22%2C%22--rm%22%2C%22hashicorp%2Fterraform-mcp-server%22%5D%7D)

### Usage with Cursor

Add this to your Cursor config (`~/.cursor/mcp.json`) or via Settings → Cursor Settings → MCP:

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server"
      ]
    }
  }
}
```


<a href="cursor://anysphere.cursor-deeplink/mcp/install?name=terraform&config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCJoYXNoaWNvcnAvdGVycmFmb3JtLW1jcC1zZXJ2ZXIiXX0%3D">
  <img alt="Add terraform MCP server to Cursor" src="https://cursor.com/deeplink/mcp-install-dark.png" height="48" />
</a>

[Install Terraform MCP server in Cursor](cursor://anysphere.cursor-deeplink/mcp/install?name=terraform&config=eyJjb21tYW5kIjoiZG9ja2VyIiwiYXJncyI6WyJydW4iLCItaSIsIi0tcm0iLCJoYXNoaWNvcnAvdGVycmFmb3JtLW1jcC1zZXJ2ZXIiXX0%3D)

### Usage with Claude Desktop / Amazon Q Developer / Amazon Q CLI

More about using MCP server tools in Claude Desktop [user documentation](https://modelcontextprotocol.io/quickstart/user).
Read more about using MCP server in Amazon Q from the [documentation](https://docs.aws.amazon.com/amazonq/latest/qdeveloper-ug/qdev-mcp.html).

```json
{
  "mcpServers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "hashicorp/terraform-mcp-server"
      ]
    }
  }
}
```

### Usage with Claude Code

More about using and adding MCP server tools in Claude Code [user documentation](https://docs.claude.com/en/docs/claude-code/mcp)

#### Local (stdio) Transport

```sh
claude mcp add terraform -s user -t stdio -- docker run -i --rm hashicorp/terraform-mcp-server
```

#### Remote (streamable-http) Transport

```sh
# Run server (example)
docker run -p 8080:8080 --rm -e TRANSPORT_MODE=streamable-http -e TRANSPORT_HOST=0.0.0.0 hashicorp/terraform-mcp-server

# Add to Claude Code
claude mcp add --transport http terraform http://localhost:8080/mcp
```

## Tool Configuration

### Available Toolsets

The following sets of tools are available for the [public Terraform registry](https://registry.terraform.io):

| Toolset     | Tool                         | Description                                                                                                                                                                                                                                                     |
|-------------|------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `providers` | `search_providers`           | Queries the Terraform Registry to find and list available documentation for a specific provider using the specified `service_slug`. Returns a list of provider document IDs with their titles and categories for resources, data sources, functions, or guides. |
| `providers` | `get_provider_details`       | Fetches the complete documentation content for a specific provider resource, data source, or function using a document ID obtained from the `search_providers` tool. Returns the raw documentation in markdown format.                                          |
| `providers` | `get_latest_provider_version`| Fetches the complete documentation content for a specific provider resource, data source, or function using a document ID obtained from the `search_providers` tool. Returns the raw documentation in markdown format.                                          |
| `modules`   | `search_modules`             | Searches the Terraform Registry for modules based on specified `module_query` with pagination. Returns a list of module IDs with their names, descriptions, download counts, verification status, and publish dates                                             |
| `modules`   | `get_module_details`         | Retrieves detailed documentation for a module using a module ID obtained from the `search_modules` tool including inputs, outputs, configuration, submodules, and examples.                                                                                     |
| `modules`   | `get_latest_module_version`  | Retrieves detailed documentation for a module using a module ID obtained from the `search_modules` tool including inputs, outputs, configuration, submodules, and examples.                                                                                     |
| `policies`  | `search_policies`            | Queries the Terraform Registry to find and list the appropriate Sentinel Policy based on the provided query `policy_query`. Returns a list of matching policies with terraform_policy_id(s) with their name, title and download counts.                         |
| `policies`  | `get_policy_details`         | Retrieves detailed documentation for a policy set using a terraform_policy_id obtained from the `search_policies` tool including policy readme and implementation details.                                                                                      |

The following sets of tools are available for HCP Terraform or Terraform Enterprise:

| Toolset     | Tool                        | Description                                                             |
|-------------|-----------------------------|-------------------------------------------------------------------------|
| `orgs`      | `list_organizations`        | Lists all Terraform organizations accessible to the authenticated user. |
| `projects`  | `list_projects`             | Lists all projects within a specified Terraform organization.           |

## Resource Configuration

### Available resources

| Resource URI | Description |
|--------------|-------------|
| `/terraform/style-guide` | Terraform Style Guide - Provides access to the official Terraform style guide documentation in markdown format |
| `/terraform/module-development` | Terraform Module Development Guide - Comprehensive guide covering module composition, structure, providers, publishing, and refactoring best practices |

### Available Resource Templates

| Resouce Template URI | Description |
|--------------|-------------|
| `/terraform/providers/{namespace}/name/{name}/version/{version}` | Provider Resource Template - Dynamically retrieves detailed documentation and overview for any Terraform provider by namespace, name, and version |


### Install from source

Use the latest release version:

```console
go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@latest
```

Use the main branch:

```console
go install github.com/hashicorp/terraform-mcp-server/cmd/terraform-mcp-server@main
```

```json
{
  "mcp": {
    "servers": {
      "terraform": {
        "command": "/path/to/terraform-mcp-server",
        "args": ["stdio"]
      }
    }
  }
}
```

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
