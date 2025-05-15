# Terraform MCP Server

The Terraform MCP Server is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction)
server that provides seamless integration with Terraform Registry APIs, enabling advanced
automation and interaction capabilities for Infrastructure as Code (IaC) development.

## Use Cases

- Automating Terraform provider and module discovery
- Extracting and analyzing data from Terraform Registry
- Getting detailed information about provider resources and data sources
- Exploring and understanding Terraform modules

> **Caution:** The outputs and recommendations provided by the MCP server are generated dynamically and may vary based on the query, model, and connected MCP servers. Users should **thoroughly review all outputs/recommendations** to ensure they align with their organization's **security best practices**, **cost-efficiency goals**, and **compliance requirements** before implementation.

## Prerequisites

1. To run the server in a container, you will need to have [Docker](https://www.docker.com/) installed.
2. Once Docker is installed, you will also need to ensure Docker is running.

> **Note**: Currently, the Docker image needs to be built locally as it's not yet available in Docker Hub. We plan to release the official Docker image through HashiCorp's Docker Hub registry in the future. Follow [issue/31](https://github.com/hashicorp/terraform-mcp-server/issues/31) for updates

## Building the Docker Image

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

This will create a local Docker image that you can use in the following configurations.

## Installation

### Usage with VS Code

Add the following JSON block to your User Settings (JSON) file in VS Code. You can do this by pressing `Ctrl + Shift + P` and typing `Preferences: Open User Settings (JSON)`.

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
          "terraform-mcp-server"
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
        "terraform-mcp-server"
      ]
    }
  }
}
```

More about using MCP server tools in VS Code's [agent mode documentation](https://code.visualstudio.com/docs/copilot/chat/mcp-servers).

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
        "terraform-mcp-server"
      ]
    }
  }
}
```

### Build from source

If you don't have Docker, you can use `make build` to build the binary directly from source code. You should configure your server to use the built executable as its `command`. For example:

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

## Tool Configuration

### Available Toolsets

The following sets of tools are available:

| Toolset     | Tool                       | Description                                                                                                           |
|-------------|----------------------------|-----------------------------------------------------------------------------------------------------------------------|
| `providers` | `providerDetails`          | Get comprehensive information about a specific provider, including its resources, data sources, functions, guides, and overview |
| `providers` | `providerResourceDetails`  | Get detailed documentation, schema, and code examples for a specific provider resource or data source                 |
| `modules`   | `searchModules`            | Search and list available Terraform modules with filtering and pagination support                                      |
| `modules`   | `moduleDetails`            | Get comprehensive details about a specific module, including its inputs, outputs, root configuration, submodules, and examples |


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
