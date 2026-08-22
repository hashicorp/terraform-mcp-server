# HCP Terraform And Terraform Enterprise Tool Behavior

## 1. Scope And Shared Rules

This document specifies the 48 tools in the `terraform` toolset. Every tool is
dynamically registered and requires a constructed primary-server TFE client in
the calling MCP session. Client construction does not prove token
authorization. Backend permissions, edition, entitlements, and API version can
make an otherwise valid call fail.

All results are one MCP text content item. JSON and JSON:API mentioned below
are serialized into text rather than returned as MCP `structuredContent`.
None of these tools advertises an MCP output schema.

The primary server does not enable input-schema validation. Advertised required
fields, types, defaults, enums, ranges, lengths, and patterns are not enforced
unless a handler or `go-tfe` repeats them. Unknown fields are not rejected by
schema validation, though middleware and handlers can still inspect them.
Handler behavior in the tables below is the runtime contract.

Every tool advertises `idempotentHint: false` and `openWorldHint: true` because
those are the MCP library defaults when constructors do not override them.
Omitted read-only and destructive options similarly serialize as
`readOnlyHint: false` and `destructiveHint: true`.

Tools marked with shared pagination use `page` and `pageSize` as specified in
[runtime.md](runtime.md#92-shared-pagination).

## 2. Advertised Annotation Matrix

| Tool | Annotation title | Read-only | Destructive | Open world |
| --- | --- | --- | --- | --- |
| `whoami` | `Get current Terraform identity` | true | false | true |
| `get_token_permissions` | `Get permissions for current token` | true | false | true |
| `list_terraform_orgs` | `List all Terraform organizations` | true | false | true |
| `list_terraform_projects` | `List all Terraform projects` | true | false | true |
| `create_project` | `Create a new Terraform project` | false | false | true |
| `get_project` | `Get a Terraform project by ID` | true | false | true |
| `delete_project` | `Delete a Terraform project by ID` | false | true | true |
| `list_teams` | `List teams in a Terraform Cloud organization.` | true | false | true |
| `get_team` | `Fetch full details for a single team by ID` | true | false | true |
| `create_team` | `Create a new team in a Terraform organization` | false | false | true |
| `add_team_member` | `Add member to a Terraform team` | false | false | true |
| `grant_team_access` | `Grant team access to a workspace or project` | false | false | true |
| `delete_team` | `Deletes a Terraform Team by "team_id"` | false | true | true |
| `list_workspaces` | `List Terraform workspaces with queries` | true | false | true |
| `get_workspace_details` | `Get detailed information about a Terraform workspace` | true | false | true |
| `create_workspace` | `Create a new Terraform workspace` | false | false | true |
| `create_no_code_workspace` | `Create a No Code module workspace` | false | true | true |
| `update_workspace` | `Update an existing Terraform workspace` | false | false | true |
| `delete_workspace_safely` | `Safely delete a Terraform workspace by ID` | false | true | true |
| `force_unlock_workspace` | `Force unlock a Terraform workspace by ID` | false | true | true |
| `list_runs` | `List Terraform runs` | true | false | true |
| `get_run_details` | `Get detailed information about a Terraform run` | true | false | true |
| `get_run_comments` | `Get all comments for a given Terraform run.` | true | false | true |
| `create_run` safe | `Create a new Terraform run` | false | false | true |
| `create_run` full | `Create a new Terraform run` | false | true | true |
| `action_run` | `Apply, Discard or Cancel a Terraform run` | false | true | true |
| `get_plan_details` | `Get detailed information about a Terraform plan` | true | false | true |
| `get_plan_logs` | `Get logs for a Terraform plan` | true | false | true |
| `get_plan_json_output` | `Get JSON output for a Terraform plan` | true | false | true |
| `get_apply_details` | `Get detailed information about a Terraform apply` | true | false | true |
| `get_apply_logs` | `Get logs for a Terraform apply` | true | false | true |
| `get_sentinel_mock` | `Get Sentinel mock data for a Terraform plan` | true | false | true |
| `list_workspace_variables` | unset | false | true | true |
| `create_workspace_variable` | unset | false | true | true |
| `update_workspace_variable` | unset | false | true | true |
| `list_variable_sets` | unset | false | true | true |
| `create_variable_set` | unset | false | true | true |
| `create_variable_in_variable_set` | unset | false | true | true |
| `delete_variable_in_variable_set` | unset | false | true | true |
| `attach_variable_set_to_workspaces` | unset | false | true | true |
| `detach_variable_set_from_workspaces` | unset | false | true | true |
| `create_workspace_tags` | unset | false | true | true |
| `read_workspace_tags` | unset | true | false | true |
| `attach_policy_set_to_workspaces` | unset | false | true | true |
| `list_workspace_policy_sets` | unset | true | true | true |
| `list_stacks` | `List Terraform workspaces with queries` | true | false | true |
| `get_stack_details` | `Get detailed information about a Terraform Stack` | true | false | true |
| `list_state_versions` | `List all States Versions` | true | false | true |
| `get_state_version` | `Gets StateVersion with state_version_id` | true | false | true |

The matrix is descriptive metadata, not enforcement. For example,
`create_workspace` mutates backend state despite `destructiveHint: false`, and
`get_sentinel_mock` creates a plan-export object despite `readOnlyHint: true`.

## 3. Identity And Organizations

### 3.1 `whoami`

Purpose: identify the user or service account represented by the active token.

Input: no properties.

Success is exact-shape JSON:

```json
{
  "username": "<string>",
  "email": "<string>",
  "is_service_account": false
}
```

API and serialization failures are tool errors.

### 3.2 `get_token_permissions`

Purpose: list enabled organization-level permissions for the active token.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |

Success is a JSON array containing only the human-readable names of mapped
permissions whose backend values are true. No mapped permission returns `[]`.
Ordering is not stable because the implementation iterates a Go map.
Organization and API failures are tool errors.

The mapped labels are:

```text
Create Teams
Create Workspaces
Create Workspace Migrations
Deploy NoCode Modules
Destroy
Manage Auditing
Manage NoCodeModules
Manage Run Tasks
Traverse
Update
Update API Tokens
Update OAuth
Update Sentinel
Update HYOK Configuration
View HYOK Feature Information
Enable Stacks
Create Projects
```

### 3.3 `list_terraform_orgs`

Purpose: list organizations visible to the active token.

Input: shared pagination only.

Success is JSON with `items` and dependency-owned pagination fields. Every item
contains `organization_name`, `organization_email`, and `created_at`. An empty
page is a tool error with `no organizations to list`. API, pagination, and
serialization failures are tool errors.

## 4. Projects

### 4.1 `list_terraform_projects`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Empty string is explicitly rejected |
| `page`, `pageSize` | number | Shared pagination | Shared pagination behavior |

Success is JSON with `items`, each containing `project_id` and `project_name`,
plus dependency-owned pagination. An empty result succeeds with `items: []`.
API, pagination, and serialization failures are tool errors.

### 4.2 `create_project`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `project_name` | string | Required; advertised length 3-40 and pattern `^[A-Za-z0-9_-][A-Za-z0-9 _-]*[A-Za-z0-9_-]$` | Trimmed; advertised length/pattern are not locally enforced |
| `description` | string | Optional; advertised maximum length 256 | Trimmed; omitted when empty; advertised maximum is not locally enforced |
| `default_execution_mode` | string | Optional; no schema enum | Trimmed/lowercased; non-empty value must be `local`, `agent`, or `remote` |

Success is JSON containing only `project_id` and `project_name`. Validation,
API, and serialization failures are tool errors.

Here, validation includes the explicit execution-mode check, `go-tfe`'s local
non-empty organization/name checks, and backend validation; the server does not
enforce the advertised name/description length or pattern constraints.

This tool creates backend state even though its destructive hint is false and
does not require `ENABLE_TF_OPERATIONS`.

### 4.3 `get_project`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `project_id` | string | Required | Trimmed |

Success is a JSON object that always contains `project_id`, `project_name`, and
`is_unified`. It conditionally contains `description`,
`default_execution_mode`, `auto_destroy_activity_duration`,
`organization_name`, `default_agent_pool_id`, and `default_agent_pool_name`
when those values or relationships exist. API and serialization failures are
tool errors.

### 4.4 `delete_project`

Availability: registered only when `ENABLE_TF_OPERATIONS=true`.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `project_id` | string | Required | Trimmed |

The tool calls the backend project delete operation. Success is exact-form text
`project "<id>" deleted`. Unknown projects, non-empty projects/stacks, missing
permissions, and API failures are tool errors.

## 5. Teams

### 5.1 `list_teams`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `team_names` | string | Optional | Comma-split; each exact name is trimmed |
| `search_query` | string | Optional | Trimmed substring query |
| `page`, `pageSize` | number | Shared pagination | Shared pagination behavior |

Success is JSON with `items` and pagination. Each item contains `id`, `name`,
`visibility`, and the exact JSON key `users-count`. No matching teams is a tool
error. API, pagination, and serialization failures are tool errors.

### 5.2 `get_team`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `team_id` | string | Required | Trimmed |

Success is the team's dependency-owned JSON:API `data` object without included
resources. It can contain the team attributes and relationships exposed by the
pinned `go-tfe` version. API and serialization failures are tool errors.

### 5.3 `create_team`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `team_name` | string | Required | Trimmed |
| `visibility` | string | Optional; no schema enum/default | Trimmed/lowercased; non-empty value must be `secret` or `organization`; omission delegates to backend default |

Success is JSON with `team_id`, `team_name`, and `visibility`.
`user_count` is included only when non-zero. Validation, API, and serialization
failures are tool errors.

This mutation is available without `ENABLE_TF_OPERATIONS`.

### 5.4 `add_team_member`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `team_id` | string | Required | Trimmed |
| `username` | string | Optional | Trimmed |
| `organization_membership_id` | string | Optional | Trimmed |

Exactly one of `username` and `organization_membership_id` MUST be non-empty.
Username addition is subject to backend membership/invitation rules;
organization membership IDs can represent pending invitations.

Success is `Successfully added member to team "<team-id>"`. Invalid one-of
input and API failures are tool errors.

### 5.5 `grant_team_access`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `team_id` | string | Required | Trimmed |
| `access_level` | string | Required | Trimmed/lowercased |
| `workspace_id` | string | Optional | Trimmed |
| `project_id` | string | Optional | Trimmed |

Exactly one target ID MUST be non-empty. Workspace access accepts `admin`,
`read`, `write`, or `plan`. Project access accepts `admin`, `read`, `write`, or
`maintain`. There is no `custom` access value.

Workspace success is JSON with `id`, `team_id`, `workspace_id`, and `access`.
Project success substitutes `project_id`. Invalid target/access combinations
and API or serialization failures are tool errors.

### 5.6 `delete_team`

Availability: registered only when `ENABLE_TF_OPERATIONS=true`.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `team_id` | string | Required | Trimmed |

The backend deletion also removes associated memberships and access grants.
Success is `Team "<id>" deleted`. API failures are tool errors.

## 6. Workspaces

### 6.1 `list_workspaces`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `search_query` | string | Optional | Passed as search filter |
| `project_id` | string | Optional | Passed as project filter |
| `tags` | string | Optional | Comma-split and each tag trimmed |
| `exclude_tags` | string | Optional | Comma-split and each tag trimmed |
| `wildcard_name` | string | Optional | Passed as wildcard filter |
| `page`, `pageSize` | number | Shared pagination | Shared pagination behavior |

Success is JSON with `items` and pagination. Each item contains `id`,
`workspace_name`, `description`, `environment`, `created_at`, and
`execution_mode`. No matches, including a nonexistent organization, are tool
errors. API, pagination, and serialization failures are tool errors.

### 6.2 `get_workspace_details`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Required | Trimmed |

Success is JSON:API with primary type `tool`. Its attributes/relationships
represent `success: true`, `workspace_id`, the nested dependency-owned
workspace, workspace variables, and `readme`.

The variable request reads one default backend page of direct workspace
variables and does not include inherited variable-set values. The response
field is incorrectly declared as a polymorphic relationship: it does not
serialize variable IDs, keys, or values. A variable whose `Workspace`
relationship is populated contributes that workspace resource identifier,
potentially repeatedly; a variable without it contributes no relationship
entry. An empty/nil collection, including one produced after a list failure, is
omitted from JSON:API rather than serialized explicitly. The tool starts with
an embedded CLI-run README whose organization and workspace placeholders are
replaced. If the backend returns a non-empty workspace README, it replaces the
embedded text. Backend README errors, nil readers, read errors, and empty
content silently retain the fallback.

Workspace lookup and serialization failures are tool errors.

### 6.3 `create_workspace`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Required | Trimmed |
| `description` | string | Optional | Sent only when non-empty |
| `terraform_version` | string | Optional | Sent only when non-empty |
| `working_directory` | string | Optional | Sent only when non-empty |
| `auto_apply` | string | Optional | Case-insensitive exact `true` becomes true; all other and omitted values become false |
| `execution_mode` | string | Optional | Case-insensitive `remote`, `local`, or `agent`; omission leaves the API field unset |
| `project_id` | string | Optional | Adds a project relationship when non-empty |
| `vcs_repo_identifier` | string | Optional | Enables VCS configuration when non-empty |
| `vcs_repo_branch` | string | Optional | Sent only with VCS configuration and when non-empty |
| `vcs_repo_oauth_token_id` | string | Optional | Required at runtime when `vcs_repo_identifier` is non-empty |
| `tags` | string | Optional | Comma-split, trimmed, and blank entries discarded |

The create request MUST set source name `terraform-mcp-server` and always send
the computed `auto_apply` value. Although the schema description says remote is
the execution default, omission does not send an execution mode and delegates
to backend behavior.

Success is a JSON:API `tool` wrapper with `success: true`, `workspace_id`, and
the nested workspace. It does not include variables or README. Duplicate names,
invalid execution mode, incomplete VCS configuration, API failures, and
serialization failures are tool errors.

The tool creates backend state, is not operations-gated, and advertises
`destructiveHint: false`.

### 6.4 `create_no_code_workspace`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `no_code_module_id` | string | Required | Must begin `nocode-`; not trimmed |
| `workspace_name` | string | Required | Not trimmed |
| `project_id` | string | Required | Not trimmed |
| `auto_apply` | boolean | Optional; default false | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; otherwise defaults false |

The tool MUST read the project, no-code module with variable options, backing
registry module, and private module metadata before creating the workspace.

It MUST issue one MCP elicitation request whose object schema contains every
metadata input and marks every one required, regardless of the metadata's
`required` flag. Type mapping is:

| Terraform metadata type | Elicitation type | Created value |
| --- | --- | --- |
| `string` | string | Non-empty string |
| `number` | number | String form of float, integer, or accepted string response |
| `bool` | boolean | `true` or `false` string |
| Any other type | string | String value under the current mapping |

No-code variable options become elicitation enums. Invalid numeric or boolean
option strings are dropped. If an options list exists but every value fails
conversion, the property receives `"enum": null` rather than omitting the enum.
The handler does not validate an accepted response against enum membership.
Every created variable uses Terraform category. Metadata sensitivity is not
propagated to the created variable.

An accepted elicitation response MUST contain every requested key with the
handler's expected runtime type. String variables reject empty strings. Number
variables accept floats, integers, and arbitrary strings, including empty and
non-numeric strings; they are not numerically revalidated. Decline produces
`workspace creation declined by user`; cancel produces `workspace creation
cancelled by user`. Missing values, wrong non-number types, unexpected actions,
lookup failures, and API failures are tool errors.

Success is a JSON:API `tool` wrapper with `success`, `workspace_id`, and nested
workspace. This destructive mutation is not gated by
`ENABLE_TF_OPERATIONS`.

### 6.5 `update_workspace`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Required | Trimmed |
| `new_name` | string | Optional | Sent only when non-empty |
| `description` | string | Optional | Sent only when non-empty; cannot clear the field |
| `terraform_version` | string | Optional | Sent only when non-empty |
| `working_directory` | string | Optional | Sent only when non-empty; cannot clear the field |
| `auto_apply` | string | Optional | A supplied non-empty case-insensitive `true` is true; every other supplied non-empty value is false |
| `execution_mode` | string | Optional | Case-insensitive `remote`, `local`, or `agent` |
| `queue_all_runs` | string | Optional | Same string-boolean conversion |
| `speculative_enabled` | string | Optional | Same string-boolean conversion |
| `trigger_prefixes` | string | Optional | Non-empty value is comma-split and trimmed |
| `file_triggers_enabled` | string | Optional | Same string-boolean conversion |
| `tags` | string | Optional | Ignored whenever non-empty; emits a warning |

Empty optional strings are omitted from the update. Trigger prefixes cannot be
cleared: the branch intended to send an empty list is unreachable. Supplying an
unrecognized non-empty boolean string writes false rather than rejecting it. A
call with no non-empty optional values still invokes the backend update API with
an empty update object.

Success is direct dependency-owned JSON:API serialization of the updated
workspace. Invalid execution mode, API failures, and serialization failures are
tool errors. This mutation is not operations-gated and advertises
`destructiveHint: false`.

### 6.6 `delete_workspace_safely`

Availability: registered only when `ENABLE_TF_OPERATIONS=true`.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `workspace_id` | string | Required | Trimmed |

The tool MUST read the workspace and then call the backend safe-delete-by-ID
operation. It performs no local run-status, force-unlock, dry-run, or ID-prefix
logic. The backend rejects deletion while managed resources remain.

Success is a JSON:API `tool` wrapper built from the pre-deletion workspace with
`success: true` and `workspace_id`. Lookup, safe-delete, and serialization
failures are tool errors.

### 6.7 `force_unlock_workspace`

Availability: registered only when `ENABLE_TF_OPERATIONS=true`.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `workspace_id` | string | Required | Trimmed |

The tool MUST read the workspace and reject it if `locked` is false. A locked
workspace is passed to the backend force-unlock operation. Success is
`Workspace "<id>" is now unlocked`. Lookup, already-unlocked, permission, and
API failures are tool errors.

## 7. Runs, Plans, Applies, And Sentinel

### 7.1 `list_runs`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Optional | Non-empty selects workspace-scoped listing |
| `vcs_username` | string | Optional | Passed as backend user filter |
| `status` | array of strings | Optional; item enum below | A schema-conforming array is ignored because the handler reads only strings; an unadvertised string is used as the filter |
| `page`, `pageSize` | number | Shared pagination | Shared pagination behavior |

The status item enum is:

```text
pending, fetching, fetching_completed, pre_plan_running, pre_plan_completed,
queuing, plan_queued, planning, planned, cost_estimating, cost_estimated,
policy_checking, policy_override, policy_soft_failed, policy_checked,
confirmed, post_plan_running, post_plan_completed, planned_and_finished,
planned_and_saved, apply_queued, applying, applied, discarded, errored,
canceled, force_canceled
```

Compatibility note: a decoded JSON array is not a string, so the accessor
deterministically returns its empty default and omits status filtering.

Without `workspace_name`, the tool lists organization runs. With it, the tool
first resolves the workspace then lists its runs. Success is JSON with `items`
and pagination. Every item contains `id`, `status`, `message`, `source`,
`created_at`, `has_changes`, `is_destroy`, `plan_only`, `refresh_only`, and
`workspace_name`. Empty results succeed with `items: []`. Lookup, API,
pagination, and serialization failures are tool errors.

Workspace-scoped output uses backend pagination. Organization-scoped output
copies only current, previous, and next page into a full pagination structure,
so its total count and total pages serialize as zero even when items exist.

### 7.2 `get_run_details`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `run_id` | string | Required | Passed without trimming |

Success is dependency-owned JSON:API for the run without included resources.
Lookup, API, and serialization failures are tool errors.

### 7.3 `get_run_comments`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `run_id` | string | Required | Trimmed and all leading `#` characters removed |

Success is JSON:

```json
{"items":[{"id":"<comment-id>","body":"<body>"}]}
```

No comments succeeds with `items: []`. API and serialization failures are tool
errors. The handler makes one backend list request and omits pagination, so it
does not guarantee every comment when the API paginates.

### 7.4 `create_run`

The registered variant depends on `ENABLE_TF_OPERATIONS`.

Common input:

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Required | Trimmed |
| `run_type` | string | Optional; default `plan_and_apply`; enum varies | Defaults to `plan_and_apply` |
| `message` | string | Optional; safe schema default is the standard message | Both handlers default to `Triggered via Terraform MCP Server`; an explicit empty string omits the API message |

Safe run types:

| Value | API option |
| --- | --- |
| `plan_and_apply` | `auto_apply=false` |
| `refresh_state` | `refresh_only=true` |
| `plan_only` | `plan_only=true` |
| `allow_empty_apply` | `allow_empty_apply=true` |

The full variant adds:

| Value | API option |
| --- | --- |
| `auto_approve` | `auto_apply=true` |
| `is_destroy` | `is_destroy=true` |

The tool MUST resolve the workspace and reject a locked workspace with:

```text
workspace "<name>" is locked and cannot accept new runs. Use the force_unlock_workspace tool to unlock first
```

Because schemas are not validated, an unknown string `run_type` reaches the
handler and creates a generic run with none of the switch-specific options.

Success is dependency-owned run JSON:API returned immediately after creation;
the tool does not poll planning. The safe variant serializes included resources
while the full variant explicitly omits included resources. Lookup, API, and
serialization failures are tool errors.

### 7.5 `action_run`

Availability: registered only when `ENABLE_TF_OPERATIONS=true`.

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `run_action` | string | Required; enum `apply`, `discard`, `cancel` | Selects one backend action |
| `run_id` | string | Required | Passed to the backend |
| `comment` | string | Optional | Omission defaults to `Triggered via Terraform MCP Server`; an explicit empty string is sent empty |

Success returns immediately after the backend action:

| Action | Success text |
| --- | --- |
| `apply` | Begins `Run approved and applied successfully` and advises calling `get_run_details` |
| `discard` | `Run discarded successfully` |
| `cancel` | `Run canceled successfully` |

Invalid actions and backend failures are tool errors. The tool does not wait
for apply completion.

### 7.6 `get_plan_details`

Required string input: `plan_id`. The handler does not trim or validate it;
`go-tfe` rejects an empty ID locally before HTTP.

Success is dependency-owned plan JSON:API without included resources. Lookup,
API, and serialization failures are tool errors.

### 7.7 `get_plan_logs`

Required string input: `plan_id`. The handler does not trim or validate it;
`go-tfe` rejects an empty ID locally before HTTP.

The `go-tfe` log reader can poll until the plan reaches a completed state and
uses retry backoff up to two seconds. It strips STX (`0x02`) and ETX (`0x03`)
framing bytes, so success is the decoded log stream rather than necessarily the
backend bytes byte-for-byte. A successful empty stream returns empty text. API
and stream-read failures are tool errors. Polling can exceed the HTTP server's
30-second write timeout.

### 7.8 `get_plan_json_output`

Required string input: `plan_id`. The handler does not trim or validate it;
`go-tfe` rejects an empty ID locally before HTTP.

Success is the raw backend plan JSON byte-for-byte as text. The server does not
parse or validate it, and a successful empty response becomes empty text. API
failures are tool errors.

### 7.9 `get_apply_details`

Required string input: `apply_id`. The handler does not trim or validate it;
`go-tfe` rejects an empty ID locally before HTTP.

Success is dependency-owned apply JSON:API without included resources. Lookup,
API, and serialization failures are tool errors.

### 7.10 `get_apply_logs`

Required string input: `apply_id`. The handler does not trim or validate it;
`go-tfe` rejects an empty ID locally before HTTP.

The tool MUST read apply status before requesting logs. For `errored`,
`finished`, or `canceled`, the `go-tfe` log reader returns text after stripping
STX/ETX framing bytes. A successful empty stream becomes empty text.

For every nonterminal status, the result is a successful non-error message:

```text
Apply <id> is currently in status "<status>". Wait for the status to change to a terminal state (finished, errored, canceled) before calling again.
```

Lookup, log API, and stream-read failures are tool errors.

The backend/library also recognizes `unreachable` as completed, but this
handler does not; it returns the same successful wait message for that status.

### 7.11 `get_sentinel_mock`

Required string input: `plan_id`. It is passed through without trimming or
local non-empty validation.

The tool MUST create a `sentinel-mock-bundle-v0` plan export, then poll its
status at most 30 times with a two-second wait after each pending or queued
status. Context cancellation interrupts a wait.

On `finished`, it downloads the tar.gz bytes and returns JSON text with exact
fields:

```json
{
  "plan_id": "<plan-id>",
  "plan_export_id": "<export-id>",
  "data_type": "sentinel-mock-bundle-v0",
  "format": "base64-tar-gz",
  "data": "<standard-base64>"
}
```

Export `errored`, `canceled`, `expired`, unexpected status, timeout, context
cancellation, API failure, and download failure are tool errors.

The maximum polling period is roughly 60 seconds, while the default HTTP write
timeout is 30 seconds. A long-running call over Streamable HTTP can therefore
lose its transport response before tool polling completes.

## 8. Workspace Variables

These tools do not explicitly advertise titles or behavior hints.

### 8.1 `list_workspace_variables`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `workspace_name` | string | Required | Passed without trimming |
| `page`, `pageSize` | number | Shared pagination | Applied to API request |

The tool resolves the workspace and returns JSON:API serialization of the
direct workspace-variable slice, normally a top-level `data` array. Inherited
variable-set values are not included. Backend pagination is not included in the
output. An empty list succeeds. Lookup, API, pagination, and serialization
failures are tool errors.

### 8.2 `create_workspace_variable`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `workspace_name` | string | Required | Passed without trimming |
| `key` | string | Required | Sent as key |
| `value` | string | Required | Sent as value |
| `description` | string | Optional; default empty | Always sent, including empty |
| `category` | string | Optional; enum `terraform`, `env`; default `env` | Exact `terraform` selects Terraform; every other handler value selects environment |
| `sensitive` | boolean | Optional; default false | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; always sent |
| `hcl` | boolean | Optional; default false | Same coercion; always sent |

Success is `Created variable <key> with ID <id>`. Workspace lookup and API
failures are tool errors.

The complete argument map, including `value`, is logged at info level by global
tool middleware.

### 8.3 `update_workspace_variable`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `workspace_name` | string | Required | Passed without trimming |
| `variable_id` | string | Required | Sent as variable ID |
| `key` | string | Required | Always sent |
| `value` | string | Required | Always sent |
| `sensitive` | boolean | Optional; default false | A schema-conforming boolean is ignored; only an unadvertised non-empty string is acted upon |
| `hcl` | boolean | Optional; default false | A schema-conforming boolean is ignored; only an unadvertised non-empty string is acted upon |
| `description` | string | Optional | Sent only when non-empty; cannot clear description |

Compatibility note: the boolean schema and string accessor disagree.
Schema-conforming booleans deterministically return the accessor's empty
default and are omitted from the update. The unadvertised exact string `true`
sends true; any other non-empty string sends false.

Success is `Updated variable <key> with ID <id>`. Workspace lookup and API
failures are tool errors. The complete argument map, including `value`, is
logged at info level.

## 9. Variable Sets

These tools do not explicitly advertise titles or behavior hints. None is
gated by `ENABLE_TF_OPERATIONS`.

### 9.1 `list_variable_sets`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `query` | string | Optional | Defaults to empty |
| `page`, `pageSize` | number | Shared pagination | Applied to API request |

Success is JSON:API serialization of the variable-set slice without included
resources. Backend pagination is omitted. An empty list succeeds. API,
pagination, and serialization failures are tool errors.

### 9.2 `create_variable_set`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `name` | string | Required | Sent as name |
| `description` | string | Optional | Defaults to empty and is always sent |
| `global` | boolean | Optional; default false | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; always sent |

Success is `Successfully created variable set <name> with ID <id>`. API
failures are tool errors.

### 9.3 `create_variable_in_variable_set`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `variable_set_id` | string | Required | Sent as set ID |
| `key` | string | Required | Sent as key |
| `value` | string | Required | Sent as value |
| `description` | string | Optional | Defaults to empty and is always sent |
| `category` | string | Optional; enum `terraform`, `env`; default `terraform` | Exact `env` selects environment; every other handler value selects Terraform |
| `hcl` | boolean | Optional; default false | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; always sent |
| `sensitive` | boolean | Optional; default false | Same coercion; always sent |

Success is `Successfully created variable <key> with ID <id> in variable set
<set-id>`. API failures are tool errors. The unredacted `value` is logged by
global tool middleware.

### 9.4 `delete_variable_in_variable_set`

Required string inputs: `variable_set_id` and `variable_id`.

Success is `Successfully deleted variable <variable-id> from variable set
<set-id>`. API failures are tool errors. This destructive operation is neither
operations-gated nor explicitly annotated; clients receive the MCP library's
default `destructiveHint: true`.

### 9.5 `attach_variable_set_to_workspaces`

Required string inputs: `variable_set_id` and comma-separated `workspace_ids`.

The handler MUST split on commas and trim each ID but MUST NOT remove empty
segments. For example, `ws-a,,ws-b` creates three workspace references, one
with an empty ID. `go-tfe` validates every ID before making an HTTP request, so
any empty segment deterministically produces a local dependency error.

Success is `Successfully attached variable set <id> to <N> workspaces`. A
successful count cannot include an empty segment. Local validation and API
failures are tool errors.

### 9.6 `detach_variable_set_from_workspaces`

Input parsing is identical to the attach tool, including retaining empty
segments and `go-tfe` rejection of them. Success is `Successfully detached
variable set <id> from <N> workspaces`. Local validation and API failures are
tool errors.

## 10. Workspace Tags And Policy Sets

### 10.1 `create_workspace_tags`

Required string inputs: `terraform_org_name`, `workspace_name`, and `tags`.

The tool comma-splits and trims `tags`. A plain item becomes a tag binding with
only a key. An item containing `:` splits on the first colon; the trimmed left
side is the key and the trimmed right side is the value. Blank items and items
with blank keys are discarded. An all-blank value submits an empty binding
list to `go-tfe`, which rejects it locally with `TagBindings are required`
before an HTTP request.

Success is `Added <N> tags to workspace <name>`. Workspace lookup and API
failures are tool errors. This mutation has no explicit behavior options and
therefore receives the library defaults, including `destructiveHint: true`.

### 10.2 `read_workspace_tags`

Required string inputs: `terraform_org_name` and `workspace_name`.

The tool separately fetches legacy tags and tag bindings. Success begins:

```text
Workspace <name> has <N> tags: <comma-separated names>
```

If bindings exist, it appends `Workspace <name> has <N> tag bindings:
<comma-separated key or key:value values>` with no inserted space or newline
between the two sentences. Empty collections succeed. Workspace or either list
API failure is a tool error. Each collection comes from one backend request;
the handler does not follow pagination.

### 10.3 `attach_policy_set_to_workspaces`

Required string inputs: `policy_set_id` and comma-separated `workspace_ids`.

The handler trims and discards empty workspace IDs. If no valid IDs remain, it
returns a tool error. Global policy sets cannot be attached through the backend
API. Success is `Successfully attached policy set <id> to <N> workspace(s)`.
API failures are tool errors. This mutation has no explicit behavior
options, receives the default `destructiveHint: true`, and is not
operations-gated.

### 10.4 `list_workspace_policy_sets`

Required string inputs: `terraform_org_name` and `workspace_id`.

The tool MUST fetch all organization policy-set pages in groups of 100 with
workspace relationships included. It selects global sets and sets directly
related to the workspace.

The tool never reads or validates the workspace itself. A nonexistent
`workspace_id` can therefore return all global policy sets as successful
matches.

A non-empty success is an indented JSON array. Every object contains `id`,
`name`, `description`, `kind`, `global`, and `reason`; reason is `global` or
`directly attached`. No matching sets return successful plain text `No policy
sets are attached to workspace <id>` rather than an empty array. API and
serialization failures are tool errors.

The constructor explicitly marks this tool read-only but does not override the
library's destructive default, so clients receive both `readOnlyHint: true` and
`destructiveHint: true`.

## 11. Stacks

### 11.1 `list_stacks`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `search_query` | string | Optional | Trimmed |
| `project_id` | string | Optional | Trimmed |
| `page`, `pageSize` | number | Shared pagination | Applied to API request |

Success is JSON with `items` and pagination. Each item uses the exact keys `ID`
with uppercase letters, `name`, and `description`. A backend pagination
`TotalCount` of zero is a tool error, including a valid no-match search. A page
with no items can still succeed if total count is non-zero. API, pagination,
and serialization failures are tool errors.

The advertised title incorrectly refers to workspaces; clients observe the
title shown in the annotation matrix.

### 11.2 `get_stack_details`

Required string input: `stack_id`, trimmed at runtime.

Success is dependency-owned stack JSON:API without included resources. Lookup,
API, and serialization failures are tool errors.

## 12. State Versions

### 12.1 `list_state_versions`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `workspace_name` | string | Required | Trimmed |
| `page`, `pageSize` | number | Shared pagination | Applied to API request |

Success is JSON with `items` and pagination. Each item contains `id`,
`created_at`, `serial`, `terraform_version`, `vcs_commit_sha`,
`vcs_commit_url`, and `state_version`. An empty page is a tool error. API,
pagination, and serialization failures are tool errors.

### 12.2 `get_state_version`

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `state_version_id` | string | Optional | Trimmed; all leading `#` characters removed |
| `workspace_id` | string | Optional | Trimmed; all leading `#` characters removed |

At least one identifier MUST be non-empty at runtime. If both are present,
`state_version_id` takes precedence and reads that exact version. Otherwise the
tool reads the workspace's current state version.

Success is direct `encoding/json` serialization of the dependency-owned
`go-tfe.StateVersion` object, not JSON:API. Because that type primarily carries
JSON:API rather than `encoding/json` tags, output uses exported Go field names
such as `ID`, `CreatedAt`, and `DownloadURL` and can include hosted-state URLs.
Missing identifiers, API failures, and serialization failures are tool errors.
