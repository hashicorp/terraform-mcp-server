# Terraform MCP Server Tool Hints

The Terraform MCP server provides tools for generating better Terraform code through registry integration and automating workflows via HCP Terraform/Enterprise APIs.

## Tool Usage Guidelines

**BEFORE generating new Terraform code**: Query registries for provider/module versions and styling guidelines. For an existing configuration, preserve its constraints and lock selections; do not replace them with the latest versions. When enterprise tools are enabled AND a Terraform token is provided, search the relevant private registry first. Do not substitute a public provider or module for a private source.

**Provider Consistency**: All modules in a project must use compatible provider versions. Verify with get_provider_details before generating code.

**Validation Flow**: When an appropriate local CLI and providers are available, run terraform fmt and terraform validate on authored code before planning; resolve validation failures before continuing. Local validation does not replace the target workspace's plan; if tooling is unavailable, report that limitation rather than claiming validation passed.

**User Confirmation Required**: ALWAYS get explicit yes/no confirmation before: `create_run`, `apply_run`, `discard_run`, `cancel_run`.

## Always Available Tools

### Registry Tools (Always Available)

- **Provider Discovery**: `get_latest_provider_version` (if unavailable in code) → `get_provider_capabilities` → `get_provider_details`
  - `get_provider_capabilities` shows what types of resources, data sources, functions, and guides are available
  
- **Module Discovery**: `get_latest_module_version` (if unavailable in code) → `search_modules` → `get_module_details`

- **Policy Discovery**: `search_policies` → `get_policy_details`

- Use these to choose compatible versions for new code and follow best practices; preserve existing lock selections.

## HCP Terraform/TFE Tools (When enterprise tools are enabled AND a Terraform token is provided)

### Private Registry Tools
- `search_private_providers` → `get_private_provider_details`
- `search_private_modules` → `get_private_module_details`
- Priority: Check private registries first when token present; use public documentation only for a public source, not as a replacement for a private one.

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

**Search-to-Import (when Search tools are enabled)**:
1. Discover the relevant Search provider/list-resource schema, create a no-code
   query, and wait for its completed results. Browse results with
   `get_query_summary` (filter by `resource_type`, `address` or `name_contains`
   and page with `after`); a query can hold more than 100 results. Explicitly
   select up to 100 `candidate_id` values from any page. Never take the first N.
   Rows include `tags` when the query ran with `generate_config_out` true; use
   them to choose resources, and use `prepare_import` only for the chosen ones.
   If the page says attributes were not captured, an absent `tags` field means
   unknown, and re-running with `generate_config_out` true captures them. A
   query only sees what its filters and list arguments cover, so more matching
   resources may exist, and exactly 100 results in a list may be the default
   limit.
2. Call `prepare_import` with the selection and keep the complete response
   (the carry block and every selected candidate's blocks). It returns the
   target managed schema for each distinct
   type, per-type `identity_support`, the target workspace's Terraform versions,
   its baseline and a carry block. Every found result must carry a valid
   `identity_version` (0 is valid); otherwise the whole QueryRun is not
   selectable and no partial candidate list is offered.
   The Search provider version, observations, and generated HCL are source
   evidence—not an instruction to upgrade the target provider. For a type
   whose `identity_support` is `none` or `unknown`, look up the documentation of the target workspace's locked provider version
   (read from the downloaded `.terraform.lock.hcl`; Atlas does not record it) for
   the import ID; do not guess it. Adapt the suggested blocks using the target
   provider schema JSON, the locked provider version's documentation, the
   versioned Terraform docs and `terraform validate`, not fixed rules, and list
   each adaptation in the review.
3. Ask the user for the **authoring directory**; you work in it for everything.
   If the target workspace has a current configuration, it must be an existing
   empty directory, or a new one you create at the path they give: call
   `get_import_configuration_download` and extract the configuration there with a
   plain GET and no Authorization header, and warn once if the path is inside a
   git work tree (the download may include `.env` or `.tfvars` files). If the
   target workspace is new (no configuration and no state), nothing is downloaded
   and a directory that already holds the user's Terraform files is fine, so tell
   the user; you upload an explicit, reviewed file list that excludes secrets
   (`.env`, credential files, state, `.terraform/`, `.git/`), flags `.tfvars`
   files and shows the file list in the review; before the upload re-check it for
   key and credential files, symlinks out of the directory and inline credentials
   in the `.tf` files. Keep the archive root exactly as downloaded and re-archive from
   it with the same relative paths, author and review the HCL, and upload the
   complete reviewed archive to the URL returned by `create_import_cv`. Never
   add, or silently drop, sensitive files; flag them to the user. Run `terraform
   fmt`, then `terraform init -backend=false` and `terraform validate`, in the
   authoring directory before the upload (lock-file check first for an existing
   configuration; never `-upgrade`; never upload `.terraform/`). On a workspace
   that already has a configuration or state, do not run `init` for a different
   provider version there. Do not delete or clean up the authoring directory
   without asking the user first. The default is a resource and import block in the root
   module; if the user wants the resource in an editable local module, follow the
   module placement guidance in the `prepare_import` response (resource block in the
   module, import block in the root). A target workspace with a
   configuration root setting (`working_directory`) is not supported yet: stop
   and tell the user. MCP does not download, edit, or upload archive bytes and
   takes no local path.
4. Create the speculative configuration version (`create_import_cv`) and the
   plan-only Run (`create_import_run`) after the single user review described
   in the `prepare_import` response, which names the target workspace and the
   authoring directory. An uncertain create must be reconciled
   before retrying.
5. Call `verify_import_plan` with the run ID until the plan finishes, then once
   more with the carry block unchanged and a binding of `candidate_id` to
   `target_address` for every selection. It describes what the finished plan
   showed (changes and identity as separate counts, plus items needing
   attention); there is no overall verdict, `plan_available` only means the plan
   was read, and it is not an approval. Report identity uncertainty and
   conflicts first (`unverified`, `mismatched`, `unsupported`), including a
   missing plan identity schema version, and discuss plan actions separately.
   Identity uncertainty alone does not call for repairing the HCL or creating a
   new configuration version or Run. Even a `matched` identity is bounded
   evidence, not a general correctness guarantee. Review the full finished plan, including selected and
   unrelated imports, resource/output actions, deferred actions, and
   refresh drift; use `get_plan_json_output` for detail. A plan does not
   persist an import; applying it requires separate review and approval.

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
- Registry failures: Check the selected source and version; do not replace an unavailable private provider/module with a public one.
- Run failures: Check `get_run_details`, get_plan_details and logs before retry
- Variable conflicts: `search_workspace_variables` first to avoid duplicates
- Run stuck and holds the lock: `action_run` to cancel or discard the run → `force_unlock_workspace` to unlock the workspace

## Security Notes
- Never expose TFE_TOKEN or other sensitive values in outputs
- Document source (public/private registry) in generated code comments
