# Terraform MCP Server Tool Hints

The Terraform MCP server provides tools for generating better Terraform code through registry integration and automating workflows via HCP Terraform/Enterprise APIs.

## Tool Usage Guidelines

**Before generating new Terraform code**: Consult relevant provider/module documentation. For an existing configuration, preserve its constraints and locked provider versions; do not silently select the latest version or substitute a public source for a private one.

**Provider Consistency**: All modules in a project must use compatible provider versions. Verify with get_provider_details before generating code.

**Validation Flow**: Use terraform fmt and, when appropriate tooling is available, terraform validate on authored code. Do not treat local validation as an HCP Terraform plan. Obtain approval before a plan that may start a remote Run.

**User Confirmation Required**: Get explicit approval before `create_run`, `apply_run`, `discard_run`, `cancel_run`, or a CLI plan that may create an HCP Terraform Run. Approval for a plan is not approval to apply.

## Always Available Tools

### Registry Tools (Always Available)

- **Provider Discovery**: `get_latest_provider_version` (if unavailable in code) → `get_provider_capabilities` → `get_provider_details`
  - `get_provider_capabilities` shows what types of resources, data sources, functions, and guides are available
  
- **Module Discovery**: `get_latest_module_version` (if unavailable in code) → `search_modules` → `get_module_details`

- **Policy Discovery**: `search_policies` → `get_policy_details`

- Use these to choose compatible versions for new code; preserve existing lock selections.

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

**Terraform Search (when Search tools are available)**:
1. Before a new workspace-scoped query, confirm its organization and source workspace (A). Use `provider_list_schema_list` → `generate_query_configuration` → `execute_query`; retain the returned `query_run_id` and reviewed creation-time inputs separately. An existing QueryRun ID can be used to review the same result without starting another query.
2. Call `get_query_status` with that ID until terminal, then `get_query_summary`. Follow any paging and selectability guidance it provides; do not assume the first response contains every result. Search-only requests end here—do not ask for an import target or local path.

**Search-to-import (only when `prepare_import` and `verify_import_plan` are available and import is requested)**:
1. Confirm whether the target is A or another workspace B in the same organization. Obtain the user's existing local configuration path, review its configuration and exact locked provider versions, and explicitly select candidates before `prepare_import`. The server cannot inspect a client-local path. Treat drafts and identity/schema evidence as guidance, not proof of a matching cloud object.
2. Author and review changes in that existing configuration. After explicit approval, run a plain CLI `terraform plan` for the confirmed HCP destination; do not use Search tools to upload configuration or create a Run. If the CLI Run outcome or destination is uncertain, stop and reconcile—do not guess or blindly retry.
3. Give `verify_import_plan` the exact Run ID from the checked CLI Run URL. Follow its polling and carry/binding guidance, review the full HCP Plan separately, and report plan actions and identity uncertainty separately. Hand off the configuration for normal review; do not apply or claim an import persisted.

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
