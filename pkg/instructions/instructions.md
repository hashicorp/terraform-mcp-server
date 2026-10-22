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

**Terraform Search (when Search tools are available)**:
1. **Follow the Search workflow**: These steps supersede the generic Tool Usage Guidelines and Code Generation workflow for Search query configuration and import authoring.
2. **Confirm and reuse the query source**: For a new query, complete the tool's source-scope interaction when requested. Configured `TF_CLOUD_ORGANIZATION` and `TF_MCP_QUERY_WORKSPACE` values, with `TF_WORKSPACE` as the workspace fallback, are proposed defaults—not authorization. Do not assume the agent can inspect the server environment; use the tool interaction to confirm or correct the complete organization/workspace pair, then reuse the accepted `terraform_org_name` and `workspace_name` explicitly throughout the query workflow.
3. **Start or resume a query**: Use `provider_list_schema_list` → `generate_query_configuration` → `execute_query` → `get_query_status` → `get_query_summary`. An existing authorized QueryRun ID may resume status and summary review.
4. **Approve execution separately**: Source confirmation does not approve query execution. Immediately before `execute_query`, show the source and query selections, explain that execution starts a QueryRun without modifying infrastructure, and wait for approval.
5. **Keep query versions scoped to Search**: Use the provider versions and schemas returned by the Search tools. Do not replace them with `get_latest_provider_version`; a Search catalog version is source-query evidence, not a target-version recommendation.
6. **Stop for Search-only intent**: Report the summary and stop. Do not request an import target or local path. A selectable summary contains every result matching its presentation filters, not every cloud resource; selection is explicit and only for requested import.

**Search-to-import (only when `prepare_import` and `verify_import_plan` are available and import is requested)**:
1. **Confirm selection and target independently**: After the user selects displayed `query_result_marker` values, complete the tool's import-target interaction when requested. Configured `TF_CLOUD_ORGANIZATION` and `TF_MCP_IMPORT_WORKSPACE` values, with `TF_WORKSPACE` as the workspace fallback, are independent proposed defaults—not authorization; never use `TF_MCP_QUERY_WORKSPACE` as the import target. Query-source confirmation never confirms the import target, even when their values are equal. Use the tool interaction rather than assuming the agent can inspect the server environment, and reuse the accepted `terraform_org_name` and `workspace_name` explicitly for preparation and verification. Confirm the target workspace in the same organization and the existing user-designated local configuration directory. Markers provide continuity, not import IDs, authorization or identity proof.
2. **Require the target lock file**: Inspect the target configuration tree and its exact locked provider releases before `prepare_import`. If `.terraform.lock.hcl` is missing or ambiguous, alert the user that exact target releases cannot be determined. Do not run `terraform init`, select versions, substitute Search catalog versions or call `prepare_import`; wait for further instructions.
3. **Prepare with locked releases**: Pass each provider source and exact version relevant to the selected imports. Public namespace/type shorthand is accepted; custom/private sources retain their hostname. Use the target's locked releases, not Search catalog versions.
4. **Author and validate locally**: Author and review local HCL using the target's locked releases. Use provider documentation applicable to the locked release; do not replace or upgrade it through a latest-version lookup. Format and, when appropriate, validate the authored HCL. These are diagnostics and do not authorize a Plan.
5. **Plan and verify after approval**: Obtain separate approval for one plain HCP-connected speculative `terraform plan`. Retain its exact CLI Run URL and `run_id`, then call `verify_import_plan`. Review Plan actions and object identity independently.
6. **Stop before apply**: End with reviewed HCL and findings. Do not continue automatically to `terraform apply`, `terraform import` or `apply_run`; any eventual apply is a separate, independently reviewed and authorized workflow.

**Code Generation**:
1. `search_modules`/`search_provider_docs` for available resources
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