# HCP Terraform MCP Server

### Build from Docker
```
docker build -t hcp-terraform-mcp-server .
```

If you plan to push the Docker image to a Docker registry, update the image reference accordingly. The current configuration is designed for local use.

#### Usage with Claude Desktop
```JSON
{
  "mcpServers": {
    "hcp-terraform": {
      "command": "docker",
      "args": [
        "run",
        "-i",
        "--rm",
        "-e",
        "HCP_TFE_TOKEN",
        "docker.io/library/hcp-terraform-mcp-server"
      ],
      "env": {
        "HCP_TFE_TOKEN": "<YOUR_TOKEN>"
      }
    }
  }
}
```

#### Usage with VS Code
```JSON
{
  "mcp": {
    "inputs": [
      {
        "type": "promptString",
        "id": "hcp_tfe_token",
        "description": "HCP Terraform API Token",
        "password": true
      }
    ],
    "servers": {
      "github": {
        "command": "docker",
        "args": [
          "run",
          "-i",
          "--rm",
          "-e",
          "HCP_TFE_TOKEN",
          "docker.io/library/hcp-terraform-mcp-server"
        ],
        "env": {
          "HCP_TFE_TOKEN": "${input:hcp_tfe_token}"
        }
      }
    }
  }
}
```

### Build from source

If you don't have Docker, you can use `go build` to build the binary in the
`cmd/hcp-terraform-mcp-server` directory, and use the `hcp-terraform-mcp-server stdio` command with the `HCP_TFE_TOKEN` environment variable set to your token. To specify the output location of the build, use the `-o` flag. You should configure your server to use the built executable as its `command`. For example:

#### Usage with Claude Desktop
```JSON
{
  "mcpServers": {
    "hcp-terraform": {
      "command": "/path/to/hcp-terraform-mcp-server",
      "args": ["stdio"],
      "env": {
        "HCP_TFE_TOKEN": "<YOUR_TOKEN>"
      }
    }
  }
}
```

#### Usage with VS Code
```JSON
{
  "mcp": {
    "servers": {
      "github": {
        "command": "/path/to/hcp-terraform-mcp-server",
        "args": ["stdio"],
        "env": {
          "HCP_TFE_TOKEN": "<YOUR_TOKEN>"
        }
      }
    }
  }
}
```
