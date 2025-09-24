---
applyTo: "**/*.{tf,hcl}"
description: "HashiCorp style guidelines for writing Terraform code"
---

# Terraform Code Style Guidelines

This project follows HashiCorp's official Terraform style guide for consistent, maintainable infrastructure-as-code.

## Project Context

Terraform configurations define infrastructure resources using HashiCorp Configuration Language (HCL). 
These instructions ensure code consistency, readability, and maintainability across all Terraform files.

## Module and Repository Structure

Organize your Terraform modules and repositories as follows:

```
├── README.md
├── main.tf
├── variables.tf
├── outputs.tf
├── ...
├── modules/
│   ├── nestedA/
│   │   ├── README.md
│   │   ├── variables.tf
│   │   ├── main.tf
│   │   ├── outputs.tf
│   ├── nestedB/
│   ├── .../
├── examples/
│   ├── exampleA/
│   │   ├── main.tf
│   ├── exampleB/
│   ├── .../
```

### Required Files and Directories
- `main.tf` – Primary resource and data source definitions. Must exist in every module, even if empty.
- `variables.tf` – Input variable definitions (alphabetical order). Must exist in every module, even if empty.
- `outputs.tf` – Output value definitions (alphabetical order). Must exist in every module, even if empty.
- `README.md` – Required in the root module. Describes the module's purpose, usage, and may include diagrams or examples.

### Recommended Module Structure
- `LICENSE` – Recommended in the root of the module, especially for public modules, to clarify usage rights.
- `providers.tf` – Provider configurations and requirements.
- `terraform.tf` – Terraform version and provider requirements.
- `backend.tf` – Backend configuration for state storage. (only needed for root modules)
- `locals.tf` – Local value definitions.
- `modules/` – Local module definitions in `./modules/<module_name>`. Nested modules with a `README.md` are considered public; those without are considered internal.
- `examples/` – Directory containing usage examples for the module and submodules. Each example may have its own README. Example `module` blocks should use the external source address, not a relative path.

### Additional Structure Guidance
- Split large configurations into logical files (e.g., `network.tf`, `compute.tf`, `storage.tf`).
- Keep module repositories focused on single infrastructure concerns.
- Use three-part naming for module repositories: `terraform-<PROVIDER>-<NAME>`.
- Store local modules in `./modules/<module_name>`.
- For nested modules, include a `README.md` if the module is intended for external use; omit for internal-only modules.

## Tools and Frameworks
- Use `terraform fmt` before every commit to ensure consistent formatting
- Use `terraform validate` to check syntax and internal consistency
- Use TFLint for additional static code analysis and organization-specific rules
- Run `terraform fmt -recursive` to format all subdirectories

## Code Formatting
- Indent two spaces for each nesting level
- Align equals signs when multiple single-line arguments appear consecutively
- Place arguments at the top of blocks, followed by nested blocks with one blank line separation
- Put meta-arguments (count, for_each) first, followed by other arguments, then nested blocks
- Place lifecycle blocks last, separated by blank lines
- Separate top-level blocks with one blank line

## Resource Organization
- Define data sources before the resources that reference them to build logically
- Group related resources together (networking, compute, storage)
- Order resource parameters consistently: meta-arguments, resource-specific parameters, nested blocks, lifecycle, depends_on

## Resource Naming
- Use descriptive nouns separated by underscores
- Do not include the resource type in the resource name
- Wrap resource type and name in double quotes
- Example: `resource "aws_instance" "web_server" {}` not `resource "aws_instance" "webserver_instance" {}`

## Variables and Outputs
- Define `type` and `description` for every variable
- Include reasonable `default` values for optional variables
- Set `sensitive = true` for passwords and private keys
- Order variable parameters: type, description, default, sensitive, validation blocks
- Order output parameters: description, value, sensitive
- Use descriptive names with underscores to separate words

## Comments and Documentation
- Use `#` for single and multi-line comments (not `//` or `/* */`)
- Write self-documenting code; use comments only to clarify complexity
- Add comments above resource blocks to explain non-obvious business logic
- Use the workspace name and project name as tags, if applicable, to resources

## Local Values
- Use local values sparingly to avoid making code harder to understand
- Define in `locals.tf` if referenced across multiple files
- Define at the top of a file if specific to that file only
- Use descriptive nouns with underscores for local value names

## Dynamic Resource Management
- Use `count` for nearly identical resources
- Use `for_each` when resources need distinct values that cannot be derived from integers
- Use `count` with conditional expressions for optional resources: `count = var.enable_feature ? 1 : 0`
- Use meta-arguments sparingly and add comments whenever applicable

## Provider Configuration
- Always include a default provider configuration
- Define all providers in the same file (`providers.tf`)
- Define the default provider first, then aliased providers
- Use `alias` as the first parameter in non-default provider blocks

## Version Management
- Pin Terraform version using `required_version` in terraform block
- Pin provider versions using exact versions in `required_providers`
- Pin module versions when sourcing from registries
- Example: `version = "5.34.0"` not `version = "~> 5.0"`

### Version Constraint Best Practices
- Use the minimal version constraint necessary to ensure compatibility; avoid overly broad or overly restrictive constraints.
- Prefer the pessimistic constraint operator (`~>`) for modules and providers to allow safe updates within a compatible version range.
- Avoid using only the equals (`=`) operator unless you must lock to a single version for reproducibility or known issues.
- Document the rationale for version constraints in code comments, especially if using strict or unusual constraints.
- Regularly review and update version constraints to keep dependencies current and secure.
- Avoid open-ended constraints (e.g., `>`, `>=` without an upper bound) in production code, as this can lead to unexpected breaking changes.
- Test your configuration with the allowed version range to ensure compatibility.

## Security and Secrets
- Never commit `terraform.tfstate` files or `.terraform` directories
- Use dynamic provider credentials when possible
- Access secrets from external secret management systems
- Set `sensitive = true` for sensitive variables
- Use environment variables for provider credentials

## Testing and Validation
- Write Terraform tests for modules using the test framework
- Run tests as part of CI/CD pipelines
- Use variable validation blocks for restrictive requirements
- Include input validation with meaningful error messages

## State Management
- Use remote state storage (S3, HCP Terraform, etc.)
- Use `tfe_outputs` data source for sharing data between workspaces
- Use provider-specific data sources instead of sharing full state files
- Separate environments into different workspaces or directories

## Error Handling
- Include meaningful descriptions for all variables and outputs
- Use validation blocks with clear error messages
- Structure code to fail fast with descriptive errors
- Validate input parameters before resource creation
