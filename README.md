# <img src="public/images/Terraform-LogoMark_onDark.svg" width="30" align="left" style="margin-right: 12px;"/> Terraform MCP Server

The Terraform MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction)
server that provides seamless integration with Terraform Registry APIs, enabling advanced
automation and interaction capabilities for Infrastructure as Code (IaC) development.

## Use Cases

- Automating Terraform provider and module discovery
- Extracting and analyzing data from Terraform Registry
- Getting detailed information about provider resources and data sources
- Exploring and understanding Terraform modules

> **Caution:** The outputs and recommendations provided by the MCP server are generated dynamically and may vary based on the query, model, and the connected MCP server. Users should **thoroughly review all outputs/recommendations** to ensure they align with their organization's **security best practices**, **cost-efficiency goals**, and **compliance requirements** before implementation.

## Prerequisites

1. To run the server in a container, you will need to have [Docker](https://www.docker.com/) installed.
2. Once Docker is installed, you will need to ensure Docker is running.

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


### Usage with Claude Desktop

More about using MCP server tools in Claude Desktop [user documentation](https://modelcontextprotocol.io/quickstart/user).

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

## Tool Configuration

### Available Toolsets

The following sets of tools are available:

| Toolset     | Tool                   | Description                                                                                                                                                                                                                                                    |
|-------------|------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `providers` | `resolveProviderDocID` | Queries the Terraform Registry to find and list available documentation for a specific provider using the specified `serviceSlug`. Returns a list of provider document IDs with their titles and categories for resources, data sources, functions, or guides. |
| `providers` | `getProviderDocs`      | Fetches the complete documentation content for a specific provider resource, data source, or function using a document ID obtained from the `resolveProviderDocID` tool. Returns the raw documentation in markdown format.                                     |
| `modules`   | `searchModules`        | Searches the Terraform Registry for modules based on specified `moduleQuery` with pagination. Returns a list of module IDs with their names, descriptions, download counts, verification status, and publish dates                                             |
| `modules`   | `moduleDetails`        | Retrieves detailed documentation for a module using a module ID obtained from the `searchModules` tool including inputs, outputs, configuration, submodules, and examples.                                                                                     |

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

This will create a local Docker image that you can use in the following configuration.

```json
{
  "servers": {
    "terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "terraform-mcp-server"
      ]
    }
  }
}
```

## Development

### Prerequisites
- Go (check [go.mod](./go.mod) file for specific version)
- Docker (optional, for container builds)

### Running Tests
```bash
# Run all tests
make test

# Run e2e tests
make test-e2e
```

### Available Make Commands
```bash
make build        # Build the binary
make test         # Run all tests
make test-e2e     # Run end-to-end tests
make clean        # Remove build artifacts
make deps         # Download dependencies
make docker-build # Build docker image
```

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
