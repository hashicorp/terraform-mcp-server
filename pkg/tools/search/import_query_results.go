// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

type workspaceProvider struct {
	Source  string `json:"source"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

const importPreparationContractVersion = "4"
const maxImportPreparationBytes = 256 * 1024

type importPrepareInput struct {
	Phase        string `json:"phase"`
	Organization string `json:"organization_name"`
	Workspace    string `json:"workspace_name"`
	QueryID      string `json:"query_run_id"`
	Selections   []struct {
		CandidateID string `json:"candidate_id"`
		ManagedType string `json:"managed_type"`
	} `json:"selections"`
	BaselineCVID           string `json:"baseline_cv_id,omitempty"`
	BaselineStateID        string `json:"baseline_state_id,omitempty"`
	BaselineSerial         int64  `json:"baseline_state_serial,omitempty"`
	ConfirmSpeculativeRun  bool   `json:"confirm_speculative_run,omitempty"`
	ConfigurationVersionID string `json:"configuration_version_id,omitempty"`
	RunID                  string `json:"run_id,omitempty"`
	TargetAddress          string `json:"target_address,omitempty"`
	SchemaCVID             string `json:"schema_cv_id,omitempty"`
	SchemaRunID            string `json:"schema_run_id,omitempty"`
}

type importPreparation struct {
	ContractVersion        string                      `json:"contract_version"`
	Status                 string                      `json:"status"`
	Stage                  string                      `json:"stage"`
	Organization           string                      `json:"organization_name,omitempty"`
	WorkspaceID            string                      `json:"workspace_id,omitempty"`
	QueryRunID             string                      `json:"query_run_id,omitempty"`
	ExecutionMode          string                      `json:"execution_mode,omitempty"`
	Baseline               *importAPIBaseline          `json:"baseline,omitempty"`
	Selection              *importDiscoveryCandidate   `json:"selection,omitempty"`
	SchemaSource           *importAPISchemaSource      `json:"schema_source,omitempty"`
	ManagedType            string                      `json:"managed_type,omitempty"`
	ManagedTypeSupport     string                      `json:"managed_type_support,omitempty"`
	ManagedSchema          map[string]any              `json:"managed_schema,omitempty"`
	IdentitySchema         map[string]any              `json:"identity_schema,omitempty"`
	EvidenceStatus         string                      `json:"evidence_status,omitempty"`
	SourceSchemaComparison string                      `json:"source_schema_comparison,omitempty"`
	ValidationStatus       string                      `json:"validation_status,omitempty"`
	Notes                  []string                    `json:"notes,omitempty"`
	AgentInstructions      []string                    `json:"agent_instructions,omitempty"`
	Diagnostics            []string                    `json:"diagnostics"`
	NextAction             string                      `json:"next_action"`
	Context                *importConfigurationHandoff `json:"configuration_context,omitempty"`
	Execution              *importExecutionResponse    `json:"execution,omitempty"`
	WorkflowContext        *importWorkflowContext      `json:"workflow_context,omitempty"`
	Continuation           *importContinuation         `json:"continuation,omitempty"`
}

// These are copy-forward hints, not a server-side attempt record. In
// particular, a status lookup does not establish the query that authored a CV.
type importWorkflowContext struct {
	Organization           string `json:"organization_name"`
	Workspace              string `json:"workspace_name"`
	WorkspaceID            string `json:"workspace_id"`
	QueryRunID             string `json:"query_run_id,omitempty"`
	CandidateID            string `json:"candidate_id,omitempty"`
	ManagedType            string `json:"managed_type,omitempty"`
	TargetAddress          string `json:"target_address,omitempty"`
	BaselineCVID           string `json:"baseline_cv_id,omitempty"`
	BaselineStateID        string `json:"baseline_state_id,omitempty"`
	BaselineSerial         int64  `json:"baseline_state_serial,omitempty"`
	SchemaCVID             string `json:"schema_cv_id,omitempty"`
	SchemaRunID            string `json:"schema_run_id,omitempty"`
	ConfigurationVersionID string `json:"configuration_version_id,omitempty"`
	RunID                  string `json:"run_id,omitempty"`
	PlanID                 string `json:"plan_id,omitempty"`
}

type importContinuation struct {
	NextPhase      string         `json:"next_phase"`
	Arguments      map[string]any `json:"arguments"`
	RequiredInputs []string       `json:"required_inputs,omitempty"`
	Precondition   string         `json:"precondition"`
}

type importAPIBaseline struct {
	ConfigurationVersionID string `json:"configuration_version_id,omitempty"`
	StateVersionID         string `json:"state_version_relationship_id,omitempty"`
	StateSerial            int64  `json:"state_serial"`
	WorkingDirectory       string `json:"working_directory"`
}

type importAPISchemaSource struct {
	ConfigurationVersionID        string `json:"configuration_version_id,omitempty"`
	ConfigurationBaselineRelation string `json:"configuration_baseline_relation"`
	StateVersionID                string `json:"state_version_id"`
	RunID                         string `json:"run_id"`
	PlanID                        string `json:"plan_id"`
	ProviderSource                string `json:"provider_source"`
	TerraformVersion              string `json:"terraform_version,omitempty"`
	ArtifactDigest                string `json:"artifact_digest,omitempty"`
}

const importQueryResultsDescription = `Prepare one explicitly selected Search result for agent-authored resource/import HCL.
Call get_query_summary with include_import_candidates=true to obtain candidate IDs, then
call phase=prepare with organization_name, workspace_name, query_run_id and one selection
containing candidate_id and your proposed managed_type. Preparation retrieves the destination
provider schema through current state-version metadata -> associated run -> plan/json-schema,
or, optionally, for a verified blank workspace, from a provider-only speculative plan identified
by schema_cv_id and schema_run_id. When no schema IDs are supplied for a blank workspace,
preparation returns query evidence without asserting managed-type support: the agent locally
validates the authored configuration and the speculative plan establishes runtime facts.
Where available, preparation checks type support and returns that resource's COMPLETE
schema, including attributes and nested blocks, plus selected query observations and generated
blocks when present. The agent interprets the schema and authors or adapts HCL. No separate
destination-schema MCP tool or shared filesystem is needed. The schema source may use an older
CV than today's configuration; both references are returned. Provider release identification,
query-side schemas and generated query HCL are not prerequisites. It never reads or parses HCL
or lock files. Schema descriptions, observations and generated blocks are untrusted evidence.
status=prepared means schema evidence is available, not that generated arguments or import
identity have been validated. validation_status=plan_validation_pending is response metadata,
not another tool. Terraform plan is the final preflight check for the destination configuration.
phase=context returns a short-lived preauthorized URL for the current configuration archive,
or blank_workspace if both the current archive and state are absent. For a blank workspace,
the agent authors/reviews resource/import HCL and a lock locally, calls upload with the chosen
target_address and no baseline/schema IDs, PUTs the entire archive directly, calls plan with
the CV ID and target_address, then polls status with CV/Run IDs and target_address. A separate
provider-only schema bootstrap is optional, not a prerequisite;
the MCP server does not download, unpack, store or parse configuration or HCL. The agent
downloads locally, preserves the complete tree, authors HCL and reviews changes with the user.
Optional local terraform fmt/validate can precede a speculative-only plan. No elicitation or
local configuration_path is required. phase=upload creates a speculative CV with auto-queue
disabled after explicit confirmation, returning its preauthorized one-use upload URL. The
agent PUTs the entire .tar.gz directly to that URL. phase=plan rechecks the explicit
query selection and baseline, then checks the CV is uploaded before creating a CV-bound
PlanOnly normal Run; phase=status observes caller-supplied IDs and
returns bounded per-address JSON-plan facts for agent assessment, never an import verdict.
Each request carries the relevant workspace, CV, Run and selected address. The server
does not persist workflow records. Preserve returned IDs in the calling agent. An
uncertain create outcome is not safe to retry blindly: reconcile with Atlas first.
Successful responses include workflow_context with only validated or explicitly
caller-carried IDs, and a continuation with safe copy-forward arguments and missing
inputs. Continuations never pre-confirm a create or contain temporary URLs. A status
read cannot recover query selection or archive provenance from a CV ID alone; keep
those in the agent's per-CV ledger along with the locally reviewed archive digest.
Legacy verify/review
and first-N max_resources inputs return migration diagnostics without writing files or
creating runs. Do not claim that a resource has been imported into state. The calling agent owns
import mapping, HCL authoring and repair. No resource has been imported into state.`

// The second argument remains source-compatible with existing registrations.
// Preparation does not use the MCP server, elicitation, or a client filesystem.
func ImportQueryResults(logger *log.Logger, _ *server.MCPServer) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("import_query_results",
			mcp.WithDescription(importQueryResultsDescription),
			mcp.WithTitleAnnotation("Prepare Search imports with destination schemas"),
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(false),
			mcp.WithString("phase", mcp.Required(), mcp.Enum("prepare", "verify", "review", "upload", "plan", "status", "context"), mcp.Description("prepare: selected evidence/schema when available; context: current CV or blank workspace; upload/plan: speculative import with target_address, or optional blank-workspace schema probe without it; status: CV/Run plan facts.")),
			mcp.WithString("organization_name", mcp.Description("Required for all phases.")),
			mcp.WithString("workspace_name", mcp.Description("Required for all phases.")),
			mcp.WithString("query_run_id", mcp.Description("Finished no-code query; required for prepare, upload and plan.")),
			mcp.WithArray("selections", mcp.Description("Exactly one candidate from get_query_summary and an agent-proposed managed resource type; list-resource names do not establish managed-type mappings."), mcp.MinItems(1), mcp.MaxItems(1), mcp.Items(map[string]any{
				"type": "object", "required": []string{"candidate_id", "managed_type"},
				"properties":           map[string]any{"candidate_id": map[string]any{"type": "string"}, "managed_type": map[string]any{"type": "string"}},
				"additionalProperties": false,
			})),
			mcp.WithString("baseline_cv_id", mcp.Description("For upload/plan: current configuration version ID obtained from context/prepare.")),
			mcp.WithString("baseline_state_id", mcp.Description("For upload/plan: current state version ID returned by prepare.")),
			mcp.WithNumber("baseline_state_serial", mcp.Description("For upload/plan: current state serial returned by prepare.")),
			mcp.WithBoolean("confirm_speculative_run", mcp.Description("For upload and plan, including blank-workspace bootstrap: confirm only a speculative, non-applying operation; user approval is separate from MCP tool permission.")),
			mcp.WithString("configuration_version_id", mcp.Description("For plan/status: CV ID returned by upload; validated against the workspace.")),
			mcp.WithString("run_id", mcp.Description("For status after plan: Run ID returned by plan; validated against the CV/workspace.")),
			mcp.WithString("schema_cv_id", mcp.Description("For blank-workspace prepare/upload/plan: speculative provider-only bootstrap CV ID.")),
			mcp.WithString("schema_run_id", mcp.Description("For blank-workspace prepare/upload/plan: plan-only bootstrap Run ID; validated against schema_cv_id and workspace.")),
			mcp.WithString("target_address", mcp.Description("For blank-workspace direct-import upload, plan and post-Run status: agent-chosen destination managed-resource address to inspect in the plan. Omit only for an optional provider-only schema probe.")),
			mcp.WithSchemaAdditionalProperties(false),
			mcp.WithOutputSchema[importPreparation]()),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return importQueryResultsHandler(ctx, request, logger)
		},
	}
}

func importQueryResultsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	response := importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: "input_validation", Diagnostics: []string{}}
	args := request.GetArguments()
	phase, _ := args["phase"].(string)
	if phase == "verify" || phase == "review" || args["max_resources"] != nil || args["configuration_path"] != nil || args["query_configuration"] != nil || args["generated_configuration"] != nil || args["output_file"] != nil || args["import_id"] != nil || args["configuration_file"] != nil || args["attempt_id"] != nil || args["idempotency_key"] != nil || args["review_id"] != nil {
		response.Diagnostics = []string{"legacy_import_contract"}
		response.NextAction = "Use prepare/context for schema and CV handoff; upload/plan/status now use explicit workspace, CV and Run IDs. The agent owns HCL review. Legacy review/attempt IDs cannot authorize execution."
		return importPreparationResult(response)
	}
	raw, err := json.Marshal(args)
	var input importPrepareInput
	if err == nil && len(raw) <= 64*1024 {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		err = d.Decode(&input)
	} else {
		err = importEvidenceFailure("input_invalid")
	}
	selectionNeeded := input.Phase == "prepare" || input.Phase == "upload" || input.Phase == "plan"
	validPhase := input.Phase == "prepare" || input.Phase == "context" || input.Phase == "upload" || input.Phase == "plan" || input.Phase == "status"
	if err != nil || !validPhase || !importInputName(input.Organization) || !importInputName(input.Workspace) || (selectionNeeded && (!importInputName(input.QueryID) || len(input.Selections) != 1 || !strings.HasPrefix(input.Selections[0].CandidateID, "candidate-") || len(input.Selections[0].CandidateID) != 74 || !importInputName(input.Selections[0].ManagedType))) || (!selectionNeeded && (input.QueryID != "" || len(input.Selections) != 0)) || ((input.Phase == "prepare" || input.Phase == "context") && (input.ConfigurationVersionID != "" || input.RunID != "" || input.ConfirmSpeculativeRun || input.TargetAddress != "")) || (input.SchemaCVID == "") != (input.SchemaRunID == "") {
		response.Diagnostics = []string{"import_input_invalid"}
		response.NextAction = "Supply organization_name and workspace_name; prepare/upload/plan require query_run_id and one candidate_id/managed_type selection."
		return importPreparationResult(response)
	}
	if err := client.AuthorizeOrganization(ctx, input.Organization); err != nil {
		response.Diagnostics = []string{"organization_not_allowed"}
		response.NextAction = "Use an organization allowed by this server."
		return importPreparationResult(response)
	}
	ctx, cancel := context.WithTimeout(ctx, importHandoffRequestTimeout)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		response.Diagnostics = []string{"backend_client_unavailable"}
		response.NextAction = "Supply current backend credentials and retry preparation."
		return importPreparationResult(response)
	}
	switch input.Phase {
	case "context":
		response = importConfigurationContextFromAPIs(ctx, c, input, logger)
	case "upload":
		response = createImportSpeculativeCV(ctx, c, input, logger)
	case "plan":
		response = createImportPlanRun(ctx, c, input, logger)
	case "status":
		response = readImportExecutionStatus(ctx, c, input)
	default:
		response = prepareImportFromAPIs(ctx, c, input)
	}
	addImportContinuation(input, &response)
	return importPreparationResult(response)
}

func addImportContinuation(input importPrepareInput, response *importPreparation) {
	if response.WorkspaceID == "" || (response.Status == "blocked" && response.Execution == nil) {
		return
	}
	refs := &importWorkflowContext{Organization: response.Organization, Workspace: input.Workspace, WorkspaceID: response.WorkspaceID}
	if refs.Organization == "" {
		refs.Organization = input.Organization
	}
	if response.Execution != nil {
		refs.ConfigurationVersionID = response.Execution.ConfigurationVersionID
		refs.RunID = response.Execution.RunID
		refs.PlanID = response.Execution.PlanID
		refs.TargetAddress = response.Execution.TargetAddress
	}
	if input.Phase == "prepare" && (response.Status == "prepared" || response.Status == "ready_for_authoring") || input.Phase == "upload" && response.Status == "awaiting_agent_upload" || input.Phase == "plan" && response.Status == "pending" {
		refs.QueryRunID = input.QueryID
		refs.CandidateID = input.Selections[0].CandidateID
		refs.ManagedType = input.Selections[0].ManagedType
		refs.TargetAddress = input.TargetAddress
		if input.Phase == "prepare" {
			if response.Baseline != nil {
				refs.BaselineCVID = response.Baseline.ConfigurationVersionID
				refs.BaselineStateID = response.Baseline.StateVersionID
				refs.BaselineSerial = response.Baseline.StateSerial
			}
		} else {
			refs.BaselineCVID, refs.BaselineStateID, refs.BaselineSerial = input.BaselineCVID, input.BaselineStateID, input.BaselineSerial
		}
		refs.SchemaCVID, refs.SchemaRunID = input.SchemaCVID, input.SchemaRunID
	}
	response.WorkflowContext = refs
	args := map[string]any{"organization_name": refs.Organization, "workspace_name": refs.Workspace}
	switch {
	case input.Phase == "prepare" && (response.Status == "prepared" || response.Status == "ready_for_authoring"):
		args["phase"], args["query_run_id"] = "upload", refs.QueryRunID
		args["selections"] = []map[string]string{{"candidate_id": refs.CandidateID, "managed_type": refs.ManagedType}}
		if refs.BaselineCVID != "" {
			args["baseline_cv_id"], args["baseline_state_id"], args["baseline_state_serial"] = refs.BaselineCVID, refs.BaselineStateID, refs.BaselineSerial
		}
		if refs.SchemaCVID != "" {
			args["schema_cv_id"], args["schema_run_id"] = refs.SchemaCVID, refs.SchemaRunID
		}
		required := []string{"confirm_speculative_run"}
		if refs.BaselineCVID == "" && refs.SchemaCVID == "" {
			required = append(required, "target_address")
		}
		response.Continuation = &importContinuation{NextPhase: "upload", Arguments: args, RequiredInputs: required, Precondition: "Agent must author, locally validate and obtain review of the complete configuration and lock. Add target_address for a direct blank-workspace import and explicitly confirm only a speculative CV create."}
	case input.Phase == "upload" && response.Status == "awaiting_agent_upload":
		args["phase"], args["configuration_version_id"] = "status", refs.ConfigurationVersionID
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Agent directly PUTs the reviewed complete archive to execution.upload_url before polling. Never put that URL in a persistent ledger or continuation."}
	case input.Phase == "plan" && response.Status == "pending":
		args["phase"], args["configuration_version_id"], args["run_id"] = "status", refs.ConfigurationVersionID, refs.RunID
		if refs.TargetAddress != "" {
			args["target_address"] = refs.TargetAddress
		}
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Poll this exact CV/Run pair. The Run is plan-only and no resource has been imported into persisted state."}
	case input.Phase == "status" && response.Status == "pending":
		args["phase"], args["configuration_version_id"], args["run_id"] = "status", refs.ConfigurationVersionID, refs.RunID
		if refs.TargetAddress != "" {
			args["target_address"] = refs.TargetAddress
		}
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Read this same CV/Run pair again; do not create another Run merely because it is pending."}
	case input.Phase == "status" && response.Status == "ready_for_plan":
		args["phase"], args["configuration_version_id"] = "plan", refs.ConfigurationVersionID
		response.Continuation = &importContinuation{NextPhase: "plan", Arguments: args, RequiredInputs: []string{"query_run_id", "selections", "confirm_speculative_run"}, Precondition: "Incomplete call: bring the reviewed selection and target_address for an import, plus baseline_cv_id/baseline_state_id/baseline_state_serial for existing workspaces or schema_cv_id/schema_run_id when applicable. Omit target_address only for an optional provider-schema probe. This CV read cannot prove which selection created the archive; reconcile uncertain Run creates before POST."}
	case input.Phase == "status" && response.Status == "awaiting_agent_upload":
		args["phase"], args["configuration_version_id"] = "status", refs.ConfigurationVersionID
		response.Continuation = &importContinuation{NextPhase: "status", Arguments: args, Precondition: "Use the original upload URL to PUT the agent-owned archive before polling. The URL cannot be reacquired from this CV status response."}
	}
}

func importInputName(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "/\\\x00\r\n") && value != "." && value != ".."
}

func importPreparationResult(response importPreparation) (*mcp.CallToolResult, error) {
	raw, err := json.Marshal(response)
	if err != nil || len(raw) > maxImportPreparationBytes {
		response = importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: "response", Diagnostics: []string{"evidence_response_limit"}, NextAction: "The complete response exceeds 256 KiB. No schema or execution details were truncated into a misleading success response; retain backend CV/Run IDs from earlier successful calls and reconcile uncertain creates in Atlas."}
		raw, _ = json.Marshal(response)
	}
	result := mcp.NewToolResultStructured(response, string(raw))
	result.IsError = response.Status == "blocked" || response.Status == "failed"
	return result, nil
}

func prepareImportFromAPIs(ctx context.Context, c *tfe.Client, input importPrepareInput) importPreparation {
	result := importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: "workspace", Organization: input.Organization, QueryRunID: input.QueryID, SourceSchemaComparison: "not_requested", Diagnostics: []string{}, NextAction: "Resolve the reported evidence gap, then repeat preparation."}
	fail := func(err error) importPreparation {
		result.Diagnostics = append(result.Diagnostics, importDiagnosticCode(err))
		return result
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	result.WorkspaceID = w.ID
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return fail(importEvidenceFailure("workspace_ownership_unverified"))
	}
	if err := client.AuthorizeOrganization(ctx, w.Organization.Name); err != nil {
		return fail(importEvidenceFailure("organization_not_allowed"))
	}
	result.ExecutionMode = w.ExecutionMode
	result.Baseline = &importAPIBaseline{ConfigurationVersionID: importCurrentConfigurationID(w), WorkingDirectory: w.WorkingDirectory}
	if result.Baseline.ConfigurationVersionID == "" {
		result.Notes = append(result.Notes, "configuration_baseline_unavailable")
	}
	result.Stage = "query_selection"
	discovery, err := readImportDiscovery(ctx, c, input.QueryID)
	if err != nil {
		return fail(err)
	}
	if discovery.WorkspaceID != w.ID {
		return fail(importEvidenceFailure("query_workspace_mismatch"))
	}
	for _, candidate := range discovery.Candidates {
		if candidate.CandidateID == input.Selections[0].CandidateID {
			selected := candidate
			result.Selection = &selected
		}
	}
	if result.Selection == nil {
		return fail(importEvidenceFailure("selected_candidate_not_found"))
	}
	result.ManagedType = input.Selections[0].ManagedType
	result.ManagedTypeSupport = "unknown"
	result.Stage = "current_state"
	if result.Baseline.ConfigurationVersionID != "" && input.SchemaRunID != "" {
		return fail(importEvidenceFailure("bootstrap_schema_not_for_existing_workspace"))
	}
	// A genuinely empty workspace has no state-associated schema. A separate
	// provider-only speculative plan supplies the backend schema instead.
	if result.Baseline.ConfigurationVersionID == "" && input.SchemaRunID != "" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return fail(err)
		}
	}
	sv, stateErr := readImportCurrentState(ctx, c, w.ID)
	if result.Baseline.ConfigurationVersionID == "" && importDiagnosticCode(stateErr) == "current_state_unavailable_or_inaccessible" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return fail(err)
		}
		if input.SchemaCVID == "" {
			result.Status, result.Stage = "ready_for_authoring", "blank_workspace"
			result.AgentInstructions = importBlankWorkspaceInstructions
			result.ValidationStatus = "plan_validation_pending"
			result.EvidenceStatus = "selected_query_candidate_only; managed_schema_not_verified; configuration_not_validated"
			result.Notes = append(result.Notes, "No destination provider schema exists in this empty workspace. The agent must validate its proposed provider configuration, resource and import blocks locally; the speculative plan establishes runtime facts.")
			result.NextAction = "Agent authors and reviews the complete resource/import configuration and lock locally. Call upload with target_address and no baseline/schema IDs, PUT the archive directly, then call plan with that CV ID and target_address. Inspect per-address plan facts; never claim an apply or persisted import."
			return result
		}
		run, err := readImportBootstrapSchemaRun(ctx, c, w, input.SchemaCVID, input.SchemaRunID)
		if err != nil {
			return fail(err)
		}
		result.SchemaSource = &importAPISchemaSource{ConfigurationVersionID: input.SchemaCVID, ConfigurationBaselineRelation: "bootstrap_speculative_no_current_configuration", RunID: run.ID, PlanID: run.Plan.ID, ProviderSource: result.Selection.Provider.Source, TerraformVersion: run.TerraformVersion}
		managed, identity, digest, err := readImportManagedSchema(ctx, c, run.ID, result.SchemaSource.ProviderSource, result.ManagedType)
		if err != nil {
			return fail(err)
		}
		if err := decodeImportEvidenceJSONLimit(managed, &result.ManagedSchema, maxImportSchemaBytes); err != nil {
			return fail(err)
		}
		if len(identity) > 0 {
			if err := decodeImportEvidenceJSON(identity, &result.IdentitySchema); err != nil {
				return fail(err)
			}
		}
		result.SchemaSource.ArtifactDigest = digest
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return fail(err)
		}
		result.ManagedTypeSupport = "supported"
		result.EvidenceStatus = "bootstrap_speculative_plan_schema; selected_type_checked; configuration_not_validated"
		result.Status, result.Stage, result.ValidationStatus = "prepared", "ready_for_authoring", "plan_validation_pending"
		result.AgentInstructions = importPreparationInstructions[:]
		result.NextAction = "Agent authors and reviews the import configuration using this bootstrap plan schema; upload a new speculative CV with the bootstrap IDs. Bootstrap did not change persisted state."
		return result
	}
	if stateErr != nil {
		return fail(stateErr)
	}
	result.Baseline.StateVersionID = sv.ID
	result.Baseline.StateSerial = sv.Serial
	result.Stage = "schema_source"
	r, err := readImportSchemaRun(ctx, c, w.ID, sv)
	if err != nil {
		return fail(err)
	}
	result.SchemaSource = &importAPISchemaSource{StateVersionID: sv.ID, RunID: r.ID, PlanID: r.Plan.ID, ProviderSource: result.Selection.Provider.Source, TerraformVersion: r.TerraformVersion, ConfigurationBaselineRelation: "unknown"}
	if r.ConfigurationVersion != nil {
		result.SchemaSource.ConfigurationVersionID = r.ConfigurationVersion.ID
		if r.ConfigurationVersion.ID != "" && result.Baseline.ConfigurationVersionID != "" {
			result.SchemaSource.ConfigurationBaselineRelation = "same_configuration_version"
			if r.ConfigurationVersion.ID != result.Baseline.ConfigurationVersionID {
				result.SchemaSource.ConfigurationBaselineRelation = "different_configuration_version"
				result.Notes = append(result.Notes, "schema_source_uses_different_configuration_version")
			}
		}
	}
	result.Stage = "managed_schema"
	managed, identity, digest, err := readImportManagedSchema(ctx, c, r.ID, result.SchemaSource.ProviderSource, result.ManagedType)
	if err != nil {
		if importDiagnosticCode(err) == "managed_schema_type_missing" {
			result.ManagedTypeSupport = "unsupported"
		}
		return fail(err)
	}
	if err := decodeImportEvidenceJSONLimit(managed, &result.ManagedSchema, maxImportSchemaBytes); err != nil {
		return fail(err)
	}
	if len(identity) > 0 {
		if err := decodeImportEvidenceJSON(identity, &result.IdentitySchema); err != nil {
			return fail(err)
		}
	}
	result.SchemaSource.ArtifactDigest = digest
	result.ManagedTypeSupport = "supported"
	result.EvidenceStatus = "destination_schema_acquired; resource_type_checked; configuration_not_validated"
	result.Stage = "baseline_recheck"
	current, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	currentState, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		return fail(err)
	}
	if current.ID != w.ID || importCurrentConfigurationID(current) != result.Baseline.ConfigurationVersionID || current.WorkingDirectory != w.WorkingDirectory || currentState.ID != sv.ID || currentState.Serial != sv.Serial || currentState.Run == nil || currentState.Run.ID != r.ID {
		return fail(importEvidenceFailure("baseline_changed"))
	}
	result.Status = "prepared"
	result.Stage = "ready_for_authoring"
	result.ValidationStatus = "plan_validation_pending"
	result.AgentInstructions = importPreparationInstructions[:]
	result.NextAction = "Author or adapt resource/import HCL locally using the selected schema and query evidence; resolve missing inputs and provider wiring, review changes with the user, then request a speculative CV through upload."
	return result
}

func importCurrentConfigurationID(w *tfe.Workspace) string {
	if w.CurrentConfigurationVersion != nil {
		return w.CurrentConfigurationVersion.ID
	}
	return ""
}
