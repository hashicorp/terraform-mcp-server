# terraform-mcp-server

HashiCorp's official [Terraform MCP server](https://github.com/hashicorp/terraform-mcp-server) (`hashicorp/terraform-mcp-server:1.3.0`), packaged as an Agent Plugin. It gives your agent live Terraform Registry provider docs, module details, latest versions, and Sentinel policies. With credentials, it adds HCP Terraform and Terraform Enterprise tools.

## Requirements

- Docker must be installed and running.
- For HCP Terraform or Terraform Enterprise tools, you need a Terraform API token. Without one, the server starts with the public registry tools only.

## Configure your credentials

The server reads two variables:

| Variable | Purpose |
|----------|---------|
| `TFE_ADDRESS` | HCP Terraform or Terraform Enterprise base URL. Defaults to `https://app.terraform.io` |
| `TFE_TOKEN` | API token used to authenticate requests |

The plugin's `mcp.json` forwards these to the container by name (`docker run -e TFE_ADDRESS -e TFE_TOKEN`), so they must be set in the environment the agent client passes to the MCP subprocess. Supply them through your client's supported configuration mechanism.

### Quick test for one session

If your client inherits shell environment variables:

1. In a terminal, set the variables:

   ```bash
   export TFE_ADDRESS="https://app.terraform.io"
   export TFE_TOKEN="your-terraform-token"
   ```

2. Start the agent client from that same terminal. A client opened from the Dock or Spotlight may not see these variables.

These variables last only until you close that terminal. To keep them, follow the setup below.

## Make the variables persistent

These steps use macOS as an example. Other operating systems need equivalent steps to store the token and set the variables.

### macOS

#### Step 1: Store the token in Keychain

This keeps the token out of plain-text files. You only do this once.

1. Run:

   ```bash
   security add-generic-password -a "$USER" -s terraform-tfe-token -w
   ```

2. When asked for a password, paste your Terraform API token and press Return. Nothing appears as you paste.
3. When asked to retype it, paste the token again and press Return. The command saves the token as a Keychain item named `terraform-tfe-token` and prints nothing on success.
4. Check that it saved. This command prints the token:

   ```bash
   security find-generic-password -s terraform-tfe-token -w
   ```

Because you paste the token at a prompt rather than typing it into the command, it never appears in your shell history. Step 2 uses the command above to read the token from the Keychain each time a shell starts, so you never write the token itself into any file. You can also see the item in the **Keychain Access** app.

> [!TIP]
> **Managing the token later**
>
> - **Replace it:** repeat this step with `-U` added: `security add-generic-password -U -a "$USER" -s terraform-tfe-token -w`. Without `-U`, the command fails because the item already exists.
> - **Delete it:** run `security delete-generic-password -s terraform-tfe-token`.

#### Step 2: Set the variables for your client

1. Open `~/.zshrc` (zsh is the macOS default shell) and add these lines at the end:

   ```bash
   export TFE_ADDRESS="https://app.terraform.io"
   export TFE_TOKEN="$(security find-generic-password -s terraform-tfe-token -w 2>/dev/null)"
   ```

2. Save the file.
3. Open a new terminal, or reload the file in your current one:

   ```bash
   source ~/.zshrc
   ```

4. Start the client from that terminal. For example, with Kiro's CLI:

   ```bash
   kiro-cli --v3
   ```

5. Test the connection by sending this prompt in the client:

   ```text
   Test whoami using the Terraform MCP
   ```

   Because `kiro-cli` inherits the environment of the terminal you launch it from, it passes `TFE_ADDRESS` and `TFE_TOKEN` through to the MCP subprocess.

## Keep your token safe

Never put your token in `plugin.json`, `mcp.json`, chat, or any committed file. Store it in your operating system's keychain, a private file, a secret manager, or your client's own secret store. Your shell profile should only read the token from one of these, never contain it.
