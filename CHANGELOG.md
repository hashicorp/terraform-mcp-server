## 0.2.0

SECURITY

* Updated Docker base image to `scratch` for smaller, more secure production images.
* Integrated security scanning (CodeQL, security scanner) and improved CI workflows for better code quality and vulnerability detection.
* Update golang stdlib version to 1.24.4

FEATURES

* Added support for publishing Docker images to Amazon ECR
* Added support for searching and getting documentation for policies from the Terraform Registry
* Enhanced toolset for resolving provider documentation, fetching provider docs, searching modules, and retrieving module details from the Terraform Registry.
* Added support for Streamable HTTP, see [#99](https://github.com/hashicorp/terraform-mcp-server/pull/99)

IMPROVEMENTS

* Migrated to `stretchr/testify` for more robust test assertions and refactored test structure for maintainability.
* Improved and expanded README with installation, usage, and development instructions.
* Refined GitHub Actions workflows for more reliable builds, security scanning, and dependency management.
* Updated and pinned dependencies for improved reliability and security.
* Upgraded `mcp-go` from 0.27.0 to 0.32.0 to support streamable HTTP, update how tool arguments are accesseed. see [#99](https://github.com/hashicorp/terraform-mcp-server/pull/99)
* Updated e2e test to accomodate both stdio and HTTP mode, improve test report by adding test name and improve clean up process. see [#99](https://github.com/hashicorp/terraform-mcp-server/pull/99)

FIXES

- Fixed function names and improved documentation links for better usability.
- Addressed issues with CI security scanner and permissions.
- Corrected Go module name in `go.mod` for compatibility.

## 0.1.0 (May 20, 2025)

FEATURES

- First public release of Terraform MCP Server.
- Provides seamless integration with Terraform Registry APIs for provider and module discovery, documentation retrieval, and advanced IaC automation.
- Initial support for VS Code and Claude Desktop integration.
- Includes basic CI/CD, Docker build, and test infrastructure.
