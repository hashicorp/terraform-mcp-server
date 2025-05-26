# Terraform MCP Server

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

## Terraform Cloud Integration

To use the Terraform Cloud tools, you need to provide authentication credentials in your configuration. 
Add the following environment variables to any configuration:

- `HCP_TFE_TOKEN`: Your Terraform Cloud API token
- `HCP_TFE_ADDRESS`: The address of your Terraform Cloud/Enterprise instance (optional, defaults to "https://app.terraform.io")

Example environment configuration:
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
      ], 
      "env": {
        "HCP_TFE_TOKEN": "your_terraform_cloud_token",
        "HCP_TFE_ADDRESS": "https://app.terraform.io"
      }
    }
  }
}
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

To use Terraform Cloud features, add the environment variables as described in the [Terraform Cloud Integration](#terraform-cloud-integration) section.

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

For Terraform Cloud integration, add the environment variables as described in the [Terraform Cloud Integration](#terraform-cloud-integration) section.

More about using MCP server tools in Claude Desktop [user documentation](https://code.visualstudio.com/docs/copilot/chat/mcp-servers).

### Usage with Claude Desktop

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

For Terraform Cloud integration, add the environment variables as described in the [Terraform Cloud Integration](#terraform-cloud-integration) section.

## Tool Configuration

### Available Toolsets

The following sets of tools are available:

#### Terraform Registry Tools

| Toolset     | Tool                   | Description                                                                                                                                                                                                                                                    |
|-------------|------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `providers` | `resolveProviderDocID` | Queries the Terraform Registry to find and list available documentation for a specific provider using the specified `serviceSlug`. Returns a list of provider document IDs with their titles and categories for resources, data sources, functions, or guides. |
| `providers` | `getProviderDocs`      | Fetches the complete documentation content for a specific provider resource, data source, or function using a document ID obtained from the `resolveProviderDocID` tool. Returns the raw documentation in markdown format.                                     |
| `modules`   | `searchModules`        | Searches the Terraform Registry for modules based on specified `moduleQuery` with pagination. Returns a list of module IDs with their names, descriptions, download counts, verification status, and publish dates                                             |
| `modules`   | `moduleDetails`        | Retrieves detailed documentation for a module using a module ID obtained from the `searchModules` tool including inputs, outputs, configuration, submodules, and examples.                                                                                     |

#### Terraform Enterprise Tools

| Toolset        | Tool                     | Description                                                                                                                                                              |
|----------------|--------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `organization` | `searchOrganizations`    | Searches for organizations the authenticated user has access to in Terraform Cloud/Enterprise. An optional query can be passed to filter organizations by name or email. |
| `organization` | `getOrganizationDetails` | Retrieves detailed information about a specific organization the authenticated user has access to in Terraform Cloud/Enterprise using the provided organization name.    |

### Build from source

If you don't have Docker, you can use `make build` to build the binary directly from source code. You should configure your server to use the built executable as its `command`.

1. Clone the repository:
```bash
git clone https://github.com/hashicorp/terraform-mcp-server.git
cd terraform-mcp-server
```

2. Build the binary:
```bash
make build
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

To use Terraform Cloud features with the locally built binary, add the environment variables as described in the [Terraform Cloud Integration](#terraform-cloud-integration) section.

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

For Terraform Cloud integration with locally built image, add the environment variables as described in the [Terraform Cloud Integration](#terraform-cloud-integration) section.

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
