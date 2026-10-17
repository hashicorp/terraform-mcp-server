# Terraform MCP Server Tool Hints

The Terraform MCP server provides tools for generating better Terraform code through registry integration and automating workflows via HCP Terraform/Enterprise APIs.

## Tool Usage Guidelines

**BEFORE generating any Terraform code**: Query registries for latest provider/module versions and styling guidelines. When enterprise tools are enabled AND a Terraform token is provided, search private registries first, then fall back to public.

**Provider Consistency**: All modules in a project must use compatible provider versions. Verify with get_provider_details before generating code.

**Validation Flow**: Run terraform validate immediately after generation, then terraform plan only if validation passes. Use terraform fmt to format code as needed.

**User Confirmation Required**: ALWAYS get explicit yes/no confirmation before: `create_run`, `apply_run`, `discard_run`, `cancel_run`.

## Always Available Tools

### Registry Tools (Always Available)

- **Provider Discovery**: `get_latest_provider_version` (if unavailable in code) → `get_provider_capabilities` → `get_provider_details`
  - `get_provider_capabilities` shows what types of resources, data sources, functions, and guides are available
  
- **Module Discovery**: `get_latest_module_version` (if unavailable in code) → `search_modules` → `get_module_details`

- **Policy Discovery**: `search_policies` → `get_policy_details`

- Use these to ensure generated code uses current versions and follows best practices

## HCP Terraform/TFE Tools (When enterprise tools are enabled AND a Terraform token is provided)

### Private Registry Tools
- `search_private_providers` → `get_private_provider_details`
- `search_private_modules` → `get_private_module_details`
- Priority: Check private registries first when token present, public as fallback

### Workspace Management
- **Discovery**: `search_workspaces` (empty query returns all) → `get_workspace_details`
- **Operations**: `create_workspace`, `update_workspace`, `delete_workspace_safely`, `force_unlock_workspace`
- `delete_workspace_safely` only works if workspace has no managed resources

### Run Execution
- **Discovery**: `search_run` (empty query returns all) → `get_run_details` (supports json output)
- **Operations**: `create_run` → `apply_run` OR `discard_run` OR `cancel_run`
- **Monitoring**: `get_plan_details`/`get_plan_logs` for plans, `get_apply_details`/`get_apply_logs` for applies
- Always check run status before attempting operations

### Variable Management
**Workspace Variables**:
- `search_workspace_variables` (empty query returns all)
- `create_workspace_variable`, `update_workspace_variable`, `delete_workspace_variable`

**Variable Sets** (for sharing across workspaces/projects):
- `search_variable_sets` → `get_variable_set_details`
- `create_variable_set`, `update_variable_set`, `delete_variable_set`
- `create_variable_in_variable_set`, `update_variable_in_variable_set`, `delete_variable_from_variable_set`
- `attach/detach_variable_set_to_workspaces`, `attach/detach_variable_set_to_projects`

## Workflow Patterns

**Query and Import Context**:
1. Use server-configured environment values by default. Omit scope arguments unless the user explicitly specifies an organization or workspace; pass those values as arguments to override the corresponding defaults. Do not invent scope arguments or ask the user to select defaults again.
2. Without an explicit workspace override, use MCP-specific workspace configuration first: `TF_MCP_QUERY_WORKSPACE` for discovery queries and `TF_MCP_IMPORT_WORKSPACE` for the import destination. These are independent; never use one as the other's default.
3. If the relevant MCP variable is unset, use `TF_WORKSPACE`. Without an explicit organization override, use `TF_CLOUD_ORGANIZATION`; there is no MCP-specific organization variable. Overrides are per field: changing the organization does not discard the workspace default, or vice versa.
4. Use configured environment values directly without asking the user to confirm. Ask only for missing inputs, preserving context already resolved rather than asking for both values again.
5. Search tools return `input_required` as a normal, non-error response with resolved values, sources, and missing inputs when context is incomplete, without making scoped API requests. Treat this as a clarification request, not a failure: ask the user only for the missing inputs rather than presenting an error or raw JSON. Pass those values as explicit `organization_name` or `workspace_name` arguments and reuse resolved context throughout the query workflow.
6. Resolve the import destination separately before any import workflow. Generated importable configuration does not itself import resources, and this server has no dedicated import tool. Do not treat the query workspace as the import destination or create an import run without the required operation approval.

Environment variables must be passed to the MCP server process by the client configuration (for example, its environment settings) or deployment. Do not assume the server can read the agent's local shell environment or client configuration files. Never read or expose tokens while resolving context.

**Query Execution Approval**:
- Resolve configured scope automatically and perform provider discovery and query configuration generation without asking for scope confirmation. Ask only for missing scope values reported by `input_required`.
- Before calling `execute_query`, show the resolved organization, workspace, and query selections (provider, resource types, and filters, if any). Explain that this starts a query run but does not create or modify infrastructure.
- Ask the user to approve execution and wait for their approval. Do not execute if approval is declined or absent. If the target scope or query selections change, summarize the changes and obtain approval again.
- This approval is agent-driven guidance, not a server-enforced parameter or status. It does not reintroduce confirmation of configured environment values.

**Code Generation**:
1. `search_modules`/`search_providers` for available resources
2. `get_latest_provider_version` if no version available in existing code
3. `get_module_details` for module requirements
4. Generate code with discovered constraints

**Run Management**:
1. `search_workspaces` → select target
2. `create_run` → get_run_details to monitor
3. `get_plan_details/logs` to review changes
4. User confirmation → `apply_run` OR `discard_run`

**Variable Configuration**:
1. `search_workspace_variables` to check existing
2. `create/update_workspace_variable` as needed
3. For multi-workspace: `create_variable_set` → `attach_variable_set_to_workspaces`

## Error Handling
- Registry failures: Try private first (if token), fallback to public
- Run failures: Check `get_run_details`, get_plan_details and logs before retry
- Variable conflicts: `search_workspace_variables` first to avoid duplicates
- Run stuck and holds the lock: `action_run` to cancel or discard the run → `force_unlock_workspace` to unlock the workspace

## Security Notes
- Never expose TFE_TOKEN or other sensitive values in outputs
- Document source (public/private registry) in generated code comments