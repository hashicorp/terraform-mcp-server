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
1. Resolve a proposed Search source from explicit arguments first, then server-configured query defaults. Without an explicit workspace override, use `TF_MCP_QUERY_WORKSPACE`, then `TF_WORKSPACE`. Without an explicit organization override, use `TF_CLOUD_ORGANIZATION`; there is no MCP-specific organization variable. Overrides are per field.
2. Before the first workspace-scoped call for a new query, show the resolved organization and source workspace and ask the user to confirm them. Pass explicit corrections as `organization_name` or `workspace_name` overrides and reuse the confirmed source throughout the query workflow.
3. Search tools return `input_required` as a normal, non-error response with resolved values, sources, and missing inputs when query context is incomplete, without making scoped API requests. Treat this as a clarification request, not a failure: ask only for the missing inputs rather than presenting an error or raw JSON.
4. Resolve the proposed import destination separately from an explicit selection, then `TF_MCP_IMPORT_WORKSPACE`, then `TF_WORKSPACE`. Always prompt for and confirm that destination when import is requested, even when a default is configured or the intended target is the Search source. Never treat a query workspace default as import authorization or use `TF_MCP_QUERY_WORKSPACE` as the import target. After confirmation, omit target arguments to use the confirmed import defaults or pass the user's explicit selection as an override.
5. `prepare_import` and `verify_import_plan` are read-only preparation and verification tools. They do not upload configuration, create a speculative Run, apply an import, or modify infrastructure.

Environment variables must be passed to the MCP server process by the client configuration (for example, its environment settings) or deployment. Do not assume the server can read the agent's local shell environment or client configuration files. Never read or expose tokens while resolving context.

**Query Execution Approval**:
- Confirm the resolved query source before provider discovery. Source confirmation is distinct from approval to execute a query.
- Before calling `execute_query`, show the resolved organization, workspace, and query selections (provider, resource types, and filters, if any). Explain that this starts a query run but does not create or modify infrastructure.
- Ask the user to approve execution and wait for their approval. Do not execute if approval is declined or absent. If the target scope or query selections change, summarize the changes and obtain approval again.
- This approval is agent-driven guidance, not a server-enforced parameter or status.

**Terraform Search (when Search tools are available)**:
1. **Follow the Search workflow**: These steps supersede the generic Tool Usage Guidelines and Code Generation workflow for Search query configuration and import authoring.
2. **Start or resume a query**: For a new query, confirm the source organization and workspace, then use `provider_list_schema_list` → `generate_query_configuration` → `execute_query` → `get_query_status` → `get_query_summary`. Poll with the same `query_run_id`; do not submit another query merely because polling remains active. An existing authorized QueryRun ID may resume status and summary review.
3. **Keep query versions scoped to Search**: Use the provider versions and schemas returned by the Search tools. Do not replace them with `get_latest_provider_version`; a Search catalog version is source-query evidence, not a target-version recommendation.
4. **Stop for Search-only intent**: Report the summary and stop. Do not request an import target or local path. A selectable summary contains every result matching its presentation filters, not every cloud resource; selection is explicit and only for requested import.

**Search-to-import (only when `prepare_import` and `verify_import_plan` are available and import is requested)**:
1. **Confirm selection and target**: After the user selects displayed `query_result_marker` values, confirm the target workspace in the same organization and the existing user-designated local configuration directory. Markers provide continuity, not import IDs, authorization or identity proof.
2. **Require the target lock file**: Inspect the target configuration tree and its exact locked provider releases before `prepare_import`. If `.terraform.lock.hcl` is missing or ambiguous, alert the user that exact target releases cannot be determined. Do not run `terraform init`, select versions, substitute Search catalog versions or call `prepare_import`; wait for further instructions.
3. **Prepare with locked releases**: Pass each provider source and exact version relevant to the selected imports. Public namespace/type shorthand is accepted; custom/private sources retain their hostname. Use the target's locked releases, not Search catalog versions.
4. **Author and validate locally**: Author and review local HCL using the target's locked releases. Use provider documentation applicable to the locked release; do not replace or upgrade it through a latest-version lookup. Format and, when appropriate, validate the authored HCL. These are diagnostics and do not authorize a Plan.
5. **Plan and verify after approval**: Obtain separate approval for one plain HCP-connected speculative `terraform plan`. Retain and reconcile its exact CLI Run URL and `run_id`, then call `verify_import_plan`. Poll the same Run ID; never guess the latest Run or blindly create another. Review the exact full Plan and report Plan actions and object identity independently.
6. **Stop before apply**: End with reviewed HCL and findings. Do not continue to `terraform apply`, `terraform import` or `apply_run`; any eventual apply is a separate, independently reviewed and authorized workflow.

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
