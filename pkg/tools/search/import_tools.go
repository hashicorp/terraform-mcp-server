// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

const PrepareImportDescription = `Read-only preparation for explicitly selected Search results; it does not read
local files, upload configuration or create a Terraform Plan or Run.

Call only after the user requests import, selects displayed
query_result_marker values and confirms a target workspace in the source
organization and an existing local configuration directory. The agent must
resolve the proposed target workspace from TF_MCP_IMPORT_WORKSPACE, then
TF_WORKSPACE, unless the user supplies an explicit target. Always present and
confirm the proposed target independently from the Search source. Pass the
accepted terraform_org_name and workspace_name explicitly; omitted fields use
the server's scope-confirmation response before scoped API access. Never use TF_MCP_QUERY_WORKSPACE
as an import target.
The agent must inspect that directory's .terraform.lock.hcl. If the lock file is missing or
ambiguous, alert the user that exact target provider releases cannot be determined;
do not run terraform init, select versions, substitute Search catalog versions or
call this tool, and wait for further instructions. Pass exact provider releases
by source and a proposed managed_type for each selected marker. Public
namespace/type shorthand is accepted; custom/private sources require
hostname/namespace/type. The server canonicalizes sources, then
revalidates complete QueryRun evidence, target eligibility and optional
historical schema.

A prepared result contains selected observations, HCL drafts and unchanged
import_context for finished-Plan verification. Markers are continuity values,
not import IDs or authorization. Search list types do not prove managed types;
generated HCL is a draft, and historical schema does not attest the locked
release, provider scope, identity mapping or valid final HCL. Use locked-release
resource/import documentation and the target Terraform version while reviewing
exact addresses, effective provider mappings and editable local-module effects.

On a prepared result, author and review the proposed local changes with the
user. Any HCP-connected speculative Plan requires separate approval.`

const VerifyImportPlanDescription = `Read-only verification of one exact ordinary HCP Terraform speculative Run; it
does not create a Run, modify configuration or apply changes.

Pass the unambiguous run_id retained from the reviewed CLI Run URL for the
confirmed target organization/workspace. Reuse the target confirmed for
prepare_import by passing terraform_org_name and workspace_name explicitly.
For omitted fields, the server requests confirmation; do not use TF_MCP_QUERY_WORKSPACE as
the target. Do not search for or substitute a newer Run. import_context and
bindings may be omitted while polling, but a
finished Plan requires the unchanged prepare_import context and exactly one
selected marker-to-address binding per selection. The tool checks the Run's
backend workspace association, plan-only/no-auto-apply evidence, Plan and
configuration-version relationships, lifecycle state and—when eligible—the
associated Plan JSON.

Follow the returned status and next_action. Poll pending with the same Run ID.
Blocked or failed results do not provide successful Plan classification. A
plan_available result contains bounded facts about selected actions, object
identity and other Plan effects; change and object_identity are independent,
and neither the summary nor a no-op import proves identity, uploaded bytes,
provider scope or apply safety.

Review the exact full Plan through run_ui_url or, only if the user chooses
in-session review and the Terraform toolset exposes get_plan_json_output, with
the backend-observed plan_id. Raw Plan JSON can contain sensitive values;
summarize without reproducing them and state if full review was not completed.
This Search-to-import workflow ends with reviewed HCL and findings; it does not
continue to terraform apply, terraform import or apply_run.`

// decodeImportArguments validates strict JSON structure and decodes one tool input value.
func decodeImportArguments(raw any, target any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return searchDiagnosticError(importArgumentsInvalidCode)
	}
	if err := validateQueryEvidenceJSON(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if dec.Decode(target) != nil {
		return searchDiagnosticError(importArgumentsInvalidCode)
	}
	if _, err := dec.Token(); err != io.EOF {
		return searchDiagnosticError(importArgumentsInvalidCode)
	}
	return nil
}

func PrepareImport(logger *log.Logger) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("prepare_import", mcp.WithDescription(PrepareImportDescription),
		mcp.WithTitleAnnotation("Prepare explicit Search import selection"), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString(ScopeOrganizationInput, mcp.Description(ImportOrganizationDescription)), mcp.WithString(ScopeWorkspaceInput, mcp.Description(ImportWorkspaceDescription)), mcp.WithString("workspace_id", mcp.Description("Optional mismatch check only.")), mcp.WithString("query_run_id", mcp.Required()),
		mcp.WithArray("selections", mcp.Required(), mcp.MinItems(1), mcp.Items(map[string]any{"type": "object", "required": []string{"query_result_marker", "managed_type"}, "properties": map[string]any{"query_result_marker": map[string]any{"type": "string"}, "managed_type": map[string]any{"type": "string"}}, "additionalProperties": false})),
		mcp.WithObject("locked_providers", mcp.Required(), mcp.Description("Exact local .terraform.lock.hcl versions keyed by provider source. Public namespace/type shorthand is accepted; custom/private sources require hostname/namespace/type. Sources are canonicalized. Agent-reported, not server-attested.")),
		mcp.WithSchemaAdditionalProperties(false), mcp.WithOutputSchema[PrepareImportResult]()),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input PrepareImportInput
			if err := decodeImportArguments(request.GetRawArguments(), &input); err != nil {
				return toolErrorf(logger, "prepare_import", "decoding prepare_import arguments: %v", err)
			}
			scope, interactionResult := resolveMark3LabsScope(ctx, ResolveImportScope(input.OrganizationName, input.WorkspaceName), ImportTargetScope)
			if interactionResult != nil {
				return interactionResult, nil
			}
			input.OrganizationName, input.WorkspaceName = scope.Organization.Value, scope.Workspace.Value
			if err := ValidatePrepareImportInput(input); err != nil {
				return toolErrorf(logger, "prepare_import", "validating prepare_import arguments: %v", err)
			}
			c, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return toolErrorf(logger, "prepare_import", "%v", TerraformClientUnavailableError(err))
			}
			result, text, err := PrepareImportToolOutput(ctx, c, input)
			if err != nil {
				return toolErrorf(logger, "prepare_import", "%v", err)
			}
			return mcp.NewToolResultStructured(result, text), nil
		},
	}
}

func VerifyImportPlan(logger *log.Logger) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("verify_import_plan", mcp.WithDescription(VerifyImportPlanDescription),
		mcp.WithTitleAnnotation("Inspect exact CLI speculative Run and Plan"), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithInputSchema[VerifyImportInput](), mcp.WithOutputSchema[VerifyImportResult]()),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input VerifyImportInput
			if err := decodeImportArguments(request.GetRawArguments(), &input); err != nil {
				return toolErrorf(logger, "verify_import_plan", "decoding verify_import_plan arguments: %v", err)
			}
			scope, interactionResult := resolveMark3LabsScope(ctx, ResolveImportScope(input.OrganizationName, input.WorkspaceName), ImportTargetScope)
			if interactionResult != nil {
				return interactionResult, nil
			}
			input.OrganizationName, input.WorkspaceName = scope.Organization.Value, scope.Workspace.Value
			c, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return toolErrorf(logger, "verify_import_plan", "%v", TerraformClientUnavailableError(err))
			}
			result, text, err := VerifyImportPlanToolOutput(ctx, c, input)
			if err != nil {
				return toolErrorf(logger, "verify_import_plan", "%v", err)
			}
			return mcp.NewToolResultStructured(result, text), nil
		},
	}
}
