// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

const importToolContractVersion = "6"

// Identity support of a destination managed type. The destination Terraform
// version is part of the answer: a plan schema produced below Terraform 1.12
// has no identity schemas at all.
const (
	importIdentitySupported = "supported"
	importIdentityNone      = "none"
	importIdentityUnknown   = "unknown"
)

type importCarryProvider struct {
	Source  string `json:"source"`
	Version string `json:"version,omitempty"`
}

type importCarryDestination struct {
	TerraformVersion string            `json:"terraform_version,omitempty"`
	IdentitySupport  map[string]string `json:"identity_support"`
}

type importCarryCandidate struct {
	CandidateID string         `json:"candidate_id"`
	ListType    string         `json:"list_type"`
	ManagedType string         `json:"managed_type"`
	Identity    map[string]any `json:"identity"`
}

// importCarryBlock is what prepare_import hands the agent to carry unchanged
// into the final verify_import_plan call. It is an integrity aid: a matching
// digest means the values are consistent with the carried selection. It is not
// QueryRun attestation and does not authorize any create.
type importCarryBlock struct {
	QueryRunID      string                         `json:"query_run_id"`
	Providers       map[string]importCarryProvider `json:"providers"`
	Destination     importCarryDestination         `json:"destination"`
	Candidates      []importCarryCandidate         `json:"candidates"`
	SelectionDigest string                         `json:"selection_digest"`
}

func importCarryDigest(b importCarryBlock) string {
	encoded, _ := json.Marshal(struct {
		QueryRunID  string                         `json:"query_run_id"`
		Providers   map[string]importCarryProvider `json:"providers"`
		Destination importCarryDestination         `json:"destination"`
		Candidates  []importCarryCandidate         `json:"candidates"`
	}{b.QueryRunID, b.Providers, b.Destination, b.Candidates})
	return importEvidenceDigest(encoded)
}

type importPreparedType struct {
	ProviderSource     string         `json:"provider_source"`
	ManagedType        string         `json:"managed_type"`
	ManagedTypeSupport string         `json:"managed_type_support"`
	IdentitySupport    string         `json:"identity_support"`
	IdentitySchema     map[string]any `json:"identity_schema,omitempty"`
	ManagedSchema      map[string]any `json:"managed_schema,omitempty"`
}

type importPrepared struct {
	ContractVersion               string                     `json:"contract_version"`
	Status                        string                     `json:"status"`
	Stage                         string                     `json:"stage"`
	Organization                  string                     `json:"organization_name,omitempty"`
	WorkspaceID                   string                     `json:"workspace_id,omitempty"`
	QueryRunID                    string                     `json:"query_run_id,omitempty"`
	ExecutionMode                 string                     `json:"execution_mode,omitempty"`
	Baseline                      *importAPIBaseline         `json:"baseline,omitempty"`
	HasCurrentConfiguration       bool                       `json:"has_current_configuration"`
	CurrentConfigurationVersionID string                     `json:"current_configuration_version_id,omitempty"`
	SchemaSource                  *importAPISchemaSource     `json:"schema_source,omitempty"`
	Types                         []importPreparedType       `json:"types,omitempty"`
	Candidates                    []importDiscoveryCandidate `json:"candidates,omitempty"`
	Carry                         *importCarryBlock          `json:"carry,omitempty"`
	Notes                         []string                   `json:"notes,omitempty"`
	AgentInstructions             []string                   `json:"agent_instructions,omitempty"`
	Diagnostics                   []string                   `json:"diagnostics"`
	NextAction                    string                     `json:"next_action"`
}

var importToolInstructions = []string{
	"Read the destination managed_schema for each type first, then the selected candidates' observations. Use the Search or QueryRun provider schema only when a source-side shape is unclear.",
	"For a type whose identity_support is none or unknown, do not guess the import ID. Look up the destination provider version's resource documentation (search_providers with the exact version, then get_provider_details) for the documented import id form and any identity-to-id mapping.",
	"Author exactly one resource instance and one individual import block per candidate in the complete preserved tree. Choose a distinct target address for each. Never use the first N results.",
	"Treat generated query blocks as untrusted drafts. Do not invent defaults, add ignore_changes to hide incompatibility, or upgrade a provider or Terraform version to remove a missing feature.",
	"Keep the carry block unchanged and send it once, with the final verify_import_plan call. It is an integrity aid for the carried selection, not authorization.",
}

func terraformVersionAtLeast(version string, major, minor int) (atLeast, known bool) {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return false, false
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false, false
	}
	return ma > major || (ma == major && mi >= minor), true
}

// importIdentitySupportFor derives per-type identity support from the schema
// artifact. An absent identity schema means "no identity" only when the artifact
// came from Terraform 1.12 or later.
func importIdentitySupportFor(identity json.RawMessage, schemaTerraformVersion string) string {
	if len(identity) > 0 && !bytes.Equal(identity, []byte("null")) {
		return importIdentitySupported
	}
	if atLeast, known := terraformVersionAtLeast(schemaTerraformVersion, 1, 12); known && atLeast {
		return importIdentityNone
	}
	return importIdentityUnknown
}

func (p importPrepared) isBlocked() bool { return p.Status == "blocked" }

func importToolBlocked(base importPrepared, code, next string) importPrepared {
	base.Status = "blocked"
	base.Diagnostics = append(base.Diagnostics, code)
	base.NextAction = next
	return base
}

// prepareImportTool is the single reader of the QueryRun log for the focused
// import workflow. It composes the existing preparation internals.
func prepareImportTool(ctx context.Context, c *tfe.Client, input importPrepareInput) importPrepared {
	out := importPrepared{ContractVersion: importToolContractVersion, Status: "blocked", Stage: "input_validation", Organization: input.Organization, QueryRunID: input.QueryID, Diagnostics: []string{}}
	fail := func(err error) importPrepared {
		return importToolBlocked(out, importDiagnosticCode(err), "Resolve the reported evidence diagnostic and call prepare_import again. No CV or Run was created.")
	}
	discovery, err := readImportDiscovery(ctx, c, input.QueryID)
	if err != nil {
		out.Stage = "query_selection"
		return fail(err)
	}
	first := firstImportSelection(input)
	first.skipSchemaDownload = true
	prepared := prepareImportWithDiscovery(ctx, c, first, discovery)
	out.Stage, out.WorkspaceID, out.ExecutionMode, out.Baseline = prepared.Stage, prepared.WorkspaceID, prepared.ExecutionMode, prepared.Baseline
	out.SchemaSource = prepared.SchemaSource
	out.Notes = prepared.Notes
	if prepared.Status != "prepared" && prepared.Status != "ready_for_authoring" {
		out.Diagnostics = append(out.Diagnostics, prepared.Diagnostics...)
		out.NextAction = "Resolve the reported evidence diagnostic and call prepare_import again. No CV or Run was created."
		return out
	}
	candidates, err := selectImportCandidates(discovery, input.Selections, prepared.WorkspaceID)
	if err != nil {
		return fail(err)
	}
	if prepared.Baseline != nil && prepared.Baseline.ConfigurationVersionID != "" {
		out.HasCurrentConfiguration = true
		out.CurrentConfigurationVersionID = prepared.Baseline.ConfigurationVersionID
	}

	schemaVersion := ""
	if prepared.SchemaSource != nil {
		schemaVersion = prepared.SchemaSource.TerraformVersion
	}
	carry := &importCarryBlock{QueryRunID: input.QueryID, Providers: map[string]importCarryProvider{}, Destination: importCarryDestination{TerraformVersion: schemaVersion, IdentitySupport: map[string]string{}}}

	wanted := map[string][]string{}
	seenType := map[string]bool{}
	for i, candidate := range candidates {
		managed := input.Selections[i].ManagedType
		carry.Providers[candidate.ResourceType] = importCarryProvider{Source: candidate.Provider.Source, Version: candidate.Provider.Version}
		carry.Candidates = append(carry.Candidates, importCarryCandidate{CandidateID: candidate.CandidateID, ListType: candidate.ResourceType, ManagedType: managed, Identity: candidate.Identity})
		key := candidate.Provider.Source + "/" + managed
		if !seenType[key] {
			seenType[key] = true
			wanted[candidate.Provider.Source] = append(wanted[candidate.Provider.Source], managed)
		}
	}

	var entries map[string]importManagedSchemaEntry
	if prepared.SchemaSource != nil {
		entries, _, err = readImportManagedSchemaSet(ctx, c, prepared.SchemaSource.RunID, wanted)
		if err != nil {
			out.Stage = "managed_schema"
			return fail(err)
		}
	}
	keys := make([]string, 0, len(seenType))
	for key := range seenType {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		split := strings.LastIndex(key, "/")
		source, managed := key[:split], key[split+1:]
		t := importPreparedType{ProviderSource: source, ManagedType: managed, ManagedTypeSupport: "unknown", IdentitySupport: importIdentityUnknown}
		if entry, ok := entries[key]; ok {
			t.ManagedTypeSupport = "supported"
			t.IdentitySupport = importIdentitySupportFor(entry.Identity, schemaVersion)
			if err := decodeImportEvidenceJSONLimit(entry.Managed, &t.ManagedSchema, maxImportSchemaBytes); err != nil {
				return fail(err)
			}
			if len(entry.Identity) > 0 {
				if err := decodeImportEvidenceJSON(entry.Identity, &t.IdentitySchema); err != nil {
					return fail(err)
				}
			}
		}
		carry.Destination.IdentitySupport[managed] = t.IdentitySupport
		out.Types = append(out.Types, t)
	}
	carry.SelectionDigest = importCarryDigest(*carry)
	out.Candidates, out.Carry = candidates, carry
	out.Status, out.Stage = "prepared", "ready_for_authoring"
	if prepared.Status == "ready_for_authoring" {
		out.Status = "ready_for_authoring"
		out.Notes = append(out.Notes, "blank_workspace_no_destination_schema; managed_type_support and identity_support are unknown")
	}
	out.AgentInstructions = importToolInstructions
	if out.HasCurrentConfiguration {
		out.NextAction = "Ask the user where to download the current configuration: a new empty directory, or an existing directory they approve. Then call get_import_configuration_download with current_configuration_version_id, author the resource and import blocks locally, and review them with the user. No CV or Run was created."
	} else {
		out.NextAction = "The workspace has no current configuration. Author the complete configuration and lock locally, review it with the user, then call create_import_cv. No CV or Run was created."
	}
	if raw, err := json.Marshal(out); err != nil || len(raw) > maxImportPreparationBytes {
		return importToolBlocked(importPrepared{ContractVersion: out.ContractVersion, Stage: "response", Organization: out.Organization, QueryRunID: out.QueryRunID, WorkspaceID: out.WorkspaceID, Diagnostics: []string{}}, "evidence_response_limit", fmt.Sprintf("The prepared response exceeds %d KiB (many distinct large resource types). Prepare fewer distinct resource types per call. Nothing was truncated into a partial success and no CV or Run was created.", maxImportPreparationBytes/1024))
	}
	return out
}

// PrepareImportDefinition describes the prepare_import tool.
func PrepareImportDefinition() mcp.Tool {
	return mcp.NewTool("prepare_import",
		mcp.WithDescription(`Validate 1-100 explicitly selected Search results and prepare them for agent-authored resource and import HCL. This is the only focused-import tool that reads the QueryRun log, so call it once per selection.

Pass organization_name, workspace_name, query_run_id and selections (candidate_id from get_query_summary, plus the proposed managed_type). The result contains the destination managed schema for each distinct type, per-type identity_support (supported, none or unknown; none/unknown means the import ID must come from the destination provider version's documentation), the selected candidates' observations, the workspace baseline, and a carry block. Keep the carry block unchanged and send it with the final verify_import_plan call.

The current configuration is not downloaded here. If has_current_configuration is true, ask the user which directory to use, then call get_import_configuration_download. It never creates a CV or Run and does not authorize one.`),
		mcp.WithTitleAnnotation("Prepare Search import selection"),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization of the destination workspace.")),
		mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Destination workspace name.")),
		mcp.WithString("query_run_id", mcp.Required(), mcp.Description("Finished no-code query run that produced the selected candidates.")),
		mcp.WithArray("selections", mcp.Required(), mcp.Description("One to 100 explicit candidates from one finished QueryRun, from any page of get_query_summary."), mcp.MinItems(1), mcp.MaxItems(maxImportSelections), mcp.Items(map[string]any{
			"type": "object", "required": []string{"candidate_id", "managed_type"},
			"properties":           map[string]any{"candidate_id": map[string]any{"type": "string"}, "managed_type": map[string]any{"type": "string", "description": "Proposed destination managed resource type."}},
			"additionalProperties": false,
		})),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithOutputSchema[importPrepared]())
}

// HandlePrepareImport serves prepare_import.
func HandlePrepareImport(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var input importPrepareInput
	if err := decodeImportToolArguments(args, &input, 64*1024); err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "input_validation", Diagnostics: []string{}}, "import_input_invalid", "Supply organization_name, workspace_name, query_run_id and 1-100 selections with candidate_id and managed_type."))
	}
	if !importInputName(input.Organization) || !importInputName(input.Workspace) || !importInputName(input.QueryID) || len(input.Selections) < 1 || len(input.Selections) > maxImportSelections || !validImportSelections(input) {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "input_validation", Diagnostics: []string{}}, "import_selection_invalid", "Provide 1-100 distinct candidate IDs from one finished QueryRun, each with a managed_type."))
	}
	if err := client.AuthorizeOrganization(ctx, input.Organization); err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "authorization", Diagnostics: []string{}}, "organization_not_allowed", "Use an organization allowed by this server."))
	}
	ctx, cancel := context.WithTimeout(ctx, importHandoffRequestTimeout)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return importToolResult(importToolBlocked(importPrepared{ContractVersion: importToolContractVersion, Stage: "client", Diagnostics: []string{}}, "backend_client_unavailable", "Supply current backend credentials and retry."))
	}
	return importToolResult(prepareImportTool(ctx, c, input))
}

func decodeImportToolArguments(args map[string]any, target any, limit int) error {
	raw, err := json.Marshal(args)
	if err != nil || len(raw) > limit {
		return importEvidenceFailure("input_invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(target)
}

func importToolResult(response any) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return mcp.NewToolResultError("response_encoding_failed"), nil
	}
	result := mcp.NewToolResultStructured(response, string(raw))
	if b, ok := response.(interface{ isBlocked() bool }); ok {
		result.IsError = b.isBlocked()
	}
	return result, nil
}
