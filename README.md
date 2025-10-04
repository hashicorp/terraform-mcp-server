# <img src="public/images/Terraform-LogoMark_onDark.svg" width="30" align="left" style="margin-right: 12px;"/> Terraform MCP Server

The Terraform MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction)
server that provides seamless integration with Terraform Registry APIs, enabling advanced
automation and interaction capabilities for Infrastructure as Code (IaC) development.

## Features

- **Dual Transport Support**: Both Stdio and StreamableHTTP transports with configurable endpoints
- **Terraform Registry Integration**: Direct integration with public Terraform Registry APIs for providers, modules, and policies
- **HCP Terraform & Terraform Enterprise Support**: Full workspace management, organization/project listing, and private registry access
- **Workspace Operations**: Create, update, delete workspaces with support for variables, tags, and run management

> **Security Note:** At this stage, the MCP server is intended for local use only. If using the StreamableHTTP transport, always configure the MCP_ALLOWED_ORIGINS environment variable to restrict access to trusted origins only. This helps prevent DNS rebinding attacks and other cross-origin vulnerabilities.

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
| `TFE_ADDRESS` | HCP Terraform or TFE address | `"https://app.terraform.io"` |
| `TFE_TOKEN` | Terraform Enterprise API token | `""` (empty) |
| `TFE_SKIP_TLS_VERIFY` | Skip HCP Terraform or Terraform Enterprise TLS verification | `false` |
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

```bash
# Stdio mode
terraform-mcp-server stdio [--log-file /path/to/log]

# StreamableHTTP mode
terraform-mcp-server streamable-http [--transport-port 8080] [--transport-host 127.0.0.1] [--mcp-endpoint /mcp] [--log-file /path/to/log]
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
          "-e", "TFE_HOSTNAME=${input:tfe_hostname}",
          "hashicorp/terraform-mcp-server:0.3.0"
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
        "id": "tfe_hostname",
        "description": "Terraform Hostname",
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
        "-e", "TFE_HOSTNAME=${input:tfe_hostname}",
        "hashicorp/terraform-mcp-server:0.3.0"
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
      "id": "tfe_hostname",
      "description": "Terraform hostname",
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
        "-e", "TFE_HOSTNAME=<<PASTE_TFE_HOSTNAME_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:0.3.0"
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

### Usage with Claude Desktop / Amazon Q Developer / Amazon Q CLI

More about using MCP server tools in Claude Desktop [user documentation](https://modelcontextprotocol.io/quickstart/user). Read more about using MCP server in Amazon Q from the [documentation](https://docs.aws.amazon.com/amazonq/latest/qdeveloper-ug/qdev-mcp.html).

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
        "-e", "TFE_HOSTNAME=<<PASTE_TFE_HOSTNAME_HERE>>",
        "-e", "TFE_TOKEN=<<PASTE_TFE_TOKEN_HERE>>",
        "hashicorp/terraform-mcp-server:0.3.0"
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

## Session Modes

The Terraform MCP Server supports two session modes when using the StreamableHTTP transport:

- **Stateful Mode (Default)**: Maintains session state between requests, enabling context-aware operations.
- **Stateless Mode**: Each request is processed independently without maintaining session state, which can be useful for high-availability deployments or when using load balancers.

To enable stateless mode, set the environment variable:
```bash
export MCP_SESSION_MODE=stateless
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
