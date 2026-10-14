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

const PrepareImportDescription = `Read-only preparation for explicitly chosen Search results, not a configuration upload or a Terraform plan.

Before calling: ask whether the import target is the Search workspace or another workspace in the same organization. Confirm the user-designated existing local directory; the MCP server cannot read it. Inspect its .terraform.lock.hcl and pass exact versions by provider source. Select 1–100 query_result_marker values from get_query_summary and propose each managed_type. No V0 candidate_id alias. This call checks full QueryRun evidence, target execution mode and optional historical schema (which never attests the locked release). A prepared result contains selected observations/HCL drafts for agent review and an unchanged import_context for finished-plan verification. For authoring, inspect intended resource or editable local-module placement, existing inputs and references, exact addresses and effective provider mappings in the local tree. Treat generated HCL as drafts: prioritize Search-to-target identity and managed-schema differences using the locked provider release's resource/import documentation and the target Terraform version's official import and module documentation. Do not substitute a Search list schema for the target managed schema or guess an identity mapping. No archive, CV, Run or local path is accepted; the separately approved Terraform CLI plan belongs to the agent after authoring. Before planning, review the existing CLI cloud or remote-backend configuration and confirm the effective host, organization, project (when relevant) and workspace with the user, respecting explicit cloud settings, applicable CLI fallbacks and workspace-selection overrides without dumping the environment. Confirm authentication is available for that host without reading or printing credentials; MCP server configuration and authentication are separate from Terraform CLI configuration and authentication. Follow next_action; a marker is continuity, not authorization or an import ID. Approval in this Search-to-import workflow is for one speculative plan only; preparation does not authorize apply. Hand off the proposed HCL and findings after Plan review.`

const VerifyImportPlanDescription = `Read-only check of the exact CLI-originated HCP Terraform speculative Run, not a way to start a Plan.

 After preparing/authoring in the existing user-designated tree, review changed files, known upload scope, provider aliases/import forms and CLI/HCP version >=1.5 with the user. Confirm CLI host/organization/workspace against the intended target (CLI and MCP authentication are separate); Terraform init can create workspaces or migrate state and must not do so without separate approval. After explicit approval for one speculative plan, run plain terraform plan from that directory (cd or terraform -chdir=<path>), without -out; never run apply or terraform import. The CLI handles upload/Run creation, not Search MCP. Compare the CLI's human Run URL host/org/workspace with the confirmed target and pass only its unambiguous run-… ID here. Missing URL or uncertain command outcome requires reconciliation, not latest-Run guessing or blind retry. Poll pending with the same ID; supply unchanged prepare_import import_context and every query_result_marker-to-target_address binding for finished classification. If Plan JSON is blocked, use the value-free diagnostic to inspect the same Run/Plan rather than starting a new Run. For a finished Plan, ask whether the user prefers the Run UI link to review it outside the session or an in-session summary. Only with the user's choice and an available get_plan_json_output tool, use the observed plan_id; raw Plan JSON can contain sensitive values visible in this session, so summarize without reproducing them. run_ui_url is an optional browser navigation link for known HCP hosts, not a signed Plan artifact URL; otherwise reuse the confirmed CLI browser Run URL. An import with update actions requires configuration/diff review, not apply. Report identity uncertainty before Plan actions; if identity schema versions differ or a Plan version is absent, offer further investigation only on request and keep object_identity unverified regardless of corroboration. change and object_identity are independent. After full-Plan review, report both findings and hand off the proposed HCL. Do not run terraform apply, terraform import or apply_run as a continuation of this Search-to-import workflow; any eventual apply belongs to a separate, independently reviewed and authorized workflow.`

func decodeImportArguments(raw any, target any, limit int) error {
	data, err := json.Marshal(raw)
	if err != nil || len(data) > limit {
		return queryEvidenceError("import_arguments_invalid")
	}
	if err := decodeQueryEvidenceLimit(data, target, limit, 64); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if dec.Decode(target) != nil {
		return queryEvidenceError("import_arguments_invalid")
	}
	if _, err := dec.Token(); err != io.EOF {
		return queryEvidenceError("import_arguments_invalid")
	}
	return nil
}

func importContextSchemaProperties() map[string]any {
	stringMap := map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}
	return map[string]any{
		"query_run_id": map[string]any{"type": "string"},
		"providers": map[string]any{
			"type": "object",
			"additionalProperties": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source":  map[string]any{"type": "string"},
					"version": map[string]any{"type": "string"},
				},
				"required":             []string{"source", "version"},
				"additionalProperties": false,
			},
		},
		"target": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace_id":      map[string]any{"type": "string"},
				"organization_name": map[string]any{"type": "string"},
				"terraform_version": map[string]any{"type": "string"},
				"identity_support":  stringMap,
			},
			"required":             []string{"workspace_id", "organization_name", "identity_support"},
			"additionalProperties": false,
		},
		"locked_providers": stringMap,
		"selected_results": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query_result_marker":     map[string]any{"type": "string"},
					"list_type":               map[string]any{"type": "string"},
					"managed_type":            map[string]any{"type": "string"},
					"identity":                map[string]any{"type": "object", "additionalProperties": true},
					"search_identity_version": map[string]any{"type": "integer"},
				},
				"required":             []string{"query_result_marker", "list_type", "managed_type", "identity", "search_identity_version"},
				"additionalProperties": false,
			},
		},
		"selection_digest": map[string]any{"type": "string"},
	}
}

func requiredObjectProperties(names ...string) mcp.PropertyOption {
	return func(schema map[string]any) {
		schema["required"] = names
	}
}

func PrepareImport(logger *log.Logger) server.ServerTool {
	return server.ServerTool{Tool: mcp.NewTool("prepare_import", mcp.WithDescription(PrepareImportDescription),
		mcp.WithTitleAnnotation("Prepare explicit Search import selection"), mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithString("organization_name", mcp.Required()), mcp.WithString("workspace_name", mcp.Required()), mcp.WithString("workspace_id", mcp.Description("Optional mismatch check only.")), mcp.WithString("query_run_id", mcp.Required()),
		mcp.WithArray("selections", mcp.Required(), mcp.MinItems(1), mcp.MaxItems(100), mcp.Items(map[string]any{"type": "object", "required": []string{"query_result_marker", "managed_type"}, "properties": map[string]any{"query_result_marker": map[string]any{"type": "string"}, "managed_type": map[string]any{"type": "string"}}, "additionalProperties": false})),
		mcp.WithObject("locked_providers", mcp.Required(), mcp.Description("Exact local .terraform.lock.hcl versions by provider source; agent-reported, not server-attested.")),
		mcp.WithSchemaAdditionalProperties(false), mcp.WithOutputSchema[PrepareImportResult]()),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input PrepareImportInput
			if err := decodeImportArguments(request.GetRawArguments(), &input, 64<<10); err != nil {
				return toolErrorf(logger, "prepare_import", "%v", err)
			}
			c, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return toolErrorf(logger, "prepare_import", "client_unavailable")
			}
			result, err := ReadPrepareImport(ctx, c, input)
			if err != nil {
				return toolErrorf(logger, "prepare_import", "%v", err)
			}
			text, err := FormatImportResult(result)
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
		mcp.WithString("organization_name", mcp.Required()), mcp.WithString("workspace_name", mcp.Required()), mcp.WithString("run_id", mcp.Required()),
		mcp.WithObject("import_context", mcp.Description("Unchanged prepare_import import_context, required for finished Plan classification."),
			mcp.Properties(importContextSchemaProperties()),
			requiredObjectProperties("query_run_id", "providers", "target", "locked_providers", "selected_results", "selection_digest"),
			mcp.AdditionalProperties(false)),
		mcp.WithArray("bindings", mcp.Description("One query_result_marker and exact target_address per selection, required for finished Plan classification."), mcp.MaxItems(100), mcp.Items(map[string]any{"type": "object", "required": []string{"query_result_marker", "target_address"}, "properties": map[string]any{"query_result_marker": map[string]any{"type": "string"}, "target_address": map[string]any{"type": "string"}}, "additionalProperties": false})),
		mcp.WithArray("detail_addresses", mcp.Description("Optional selected target addresses to inspect without values (maximum 20)."), mcp.MaxItems(20), mcp.Items(map[string]any{"type": "string"})),
		mcp.WithSchemaAdditionalProperties(false), mcp.WithOutputSchema[VerifyImportResult]()),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var input VerifyImportInput
			if err := decodeImportArguments(request.GetRawArguments(), &input, 512<<10); err != nil {
				return toolErrorf(logger, "verify_import_plan", "%v", err)
			}
			c, err := client.GetTfeClientFromContext(ctx, logger)
			if err != nil {
				return toolErrorf(logger, "verify_import_plan", "client_unavailable")
			}
			result, err := ReadVerifyImportPlan(ctx, c, input)
			if err != nil {
				return toolErrorf(logger, "verify_import_plan", "%v", err)
			}
			text, err := FormatImportResult(result)
			if err != nil {
				return toolErrorf(logger, "verify_import_plan", "%v", err)
			}
			return mcp.NewToolResultStructured(result, text), nil
		},
	}
}
