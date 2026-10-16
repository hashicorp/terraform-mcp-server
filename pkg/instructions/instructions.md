# Terraform MCP Server Tool Hints

The Terraform MCP server provides tools for generating better Terraform code through registry integration and automating workflows via HCP Terraform/Enterprise APIs.

## Tool Usage Guidelines

**Version selection**: For new standalone configuration with no governing existing constraints or lock selections, query the available registries for current compatible provider/module versions and guidance. When editing existing configuration, preserve its constraints and exact `.terraform.lock.hcl` selections unless the user separately requests an upgrade. For Search-to-import, the target's exact locked provider releases govern authoring; the Search catalog version is source-query evidence, not a target-version recommendation.

**Provider consistency**: Use provider versions compatible with the complete configuration and module constraints. `get_provider_details` can provide version-specific documentation, but does not override an existing lock selection.

**Validation and planning**: `terraform fmt` and `validate` are diagnostics, not authorization for a Plan. Run `terraform plan` only when the active workflow and the user authorize it. Search-to-import permits one separately approved plain HCP-connected speculative Plan after local authoring review; validation success does not grant that approval.

**User confirmation required**: Always get explicit confirmation before `create_run`, `apply_run`, `discard_run` or `cancel_run`, and before the client-side Search-to-import speculative `terraform plan`.

## Always Available Tools

### Registry Tools (Always Available)

- **Provider Discovery**: `get_latest_provider_version` (if unavailable in code) → `get_provider_capabilities` → `get_provider_details`
  - `get_provider_capabilities` shows what types of resources, data sources, functions, and guides are available
  
- **Module Discovery**: `get_latest_module_version` (if unavailable in code) → `search_modules` → `get_module_details`

- **Policy Discovery**: `search_policies` → `get_policy_details`

- Use Registry tools for version-specific capabilities and guidance. Choose current compatible versions only when existing configuration, constraints and lock selections do not govern.

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
For a new query, confirm the source organization and workspace, then use `provider_list_schema_list` → `generate_query_configuration` → `execute_query` → `get_query_status` → `get_query_summary`. Poll with the same `query_run_id`; do not submit another query merely because polling remains active. An existing authorized QueryRun ID may resume status/summary review.

For Search-only intent, report the summary and stop. Do not request an import target or local path. A selectable summary contains every result matching its presentation filters, not every cloud resource; selection is explicit and only for requested import.

**Search-to-import (only when `prepare_import` and `verify_import_plan` are available and import is requested)**:
After the user selects displayed `query_result_marker` values, confirm the target workspace in the same organization and the existing user-designated local configuration directory. Inspect that target configuration tree and its exact locked provider releases before `prepare_import`. If `.terraform.lock.hcl` is missing or ambiguous, alert the user that exact target provider releases cannot be determined, do not run `terraform init`, select versions, substitute Search catalog versions or call `prepare_import`, and wait for further instructions. For each provider source relevant to the selected imports in the target configuration, copy the complete quoted hostname/namespace/type source and selected version from its `.terraform.lock.hcl` into `locked_providers`; include custom registry hostnames and do not abbreviate `registry.terraform.io/hashicorp/aws` to `hashicorp/aws`. Use the target's locked releases, not the Search catalog versions. Markers provide continuity, not import IDs, authorization or identity proof.

After preparation, author and review local HCL. Obtain separate approval for one plain HCP-connected speculative `terraform plan`, retain and reconcile its exact CLI Run URL/`run_id`, and call `verify_import_plan`. Poll the same Run ID; never guess the latest Run or blindly create another. Review the exact full Plan and report Plan actions and object identity independently.

This workflow ends with reviewed HCL and findings. It does not continue to `terraform apply`, `terraform import` or `apply_run`; any eventual apply is a separate, independently reviewed and authorized workflow.

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
