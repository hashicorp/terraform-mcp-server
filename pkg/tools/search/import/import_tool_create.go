// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

// importCreated is the response of both confirmed creates. A create never
// reads the QueryRun log: it re-checks only the workspace and its baseline.
type importCreated struct {
	ContractVersion        string   `json:"contract_version"`
	Status                 string   `json:"status"`
	WorkspaceID            string   `json:"workspace_id,omitempty"`
	ConfigurationVersionID string   `json:"configuration_version_id,omitempty"`
	UploadURL              string   `json:"upload_url,omitempty"`
	UploadInstructions     string   `json:"upload_instructions,omitempty"`
	RunID                  string   `json:"run_id,omitempty"`
	CreateOutcome          string   `json:"create_outcome,omitempty"`
	Diagnostics            []string `json:"diagnostics"`
	NextAction             string   `json:"next_action"`
}

func (c importCreated) isBlocked() bool { return c.Status == "blocked" }

type importCreateArgs struct {
	Organization    string `json:"organization_name"`
	Workspace       string `json:"workspace_name"`
	BaselineCVID    string `json:"baseline_cv_id"`
	BaselineStateID string `json:"baseline_state_id"`
	BaselineSerial  *int64 `json:"baseline_state_serial"`
	Confirm         bool   `json:"confirm_speculative_run"`
	CVID            string `json:"configuration_version_id"`
}

func importCreateBlocked(code, next string, extra ...string) importCreated {
	return importCreated{ContractVersion: importToolContractVersion, Status: "blocked", Diagnostics: append([]string{code}, extra...), NextAction: next + " No CV or Run was created by this request."}
}

// validImportCreateBaseline accepts either all three existing-workspace
// baseline fields or none (a verified blank workspace).
func validImportCreateBaseline(a importCreateArgs) bool {
	existing := a.BaselineCVID != "" || a.BaselineStateID != "" || a.BaselineSerial != nil
	if !existing {
		return true
	}
	return importInputName(a.BaselineCVID) && importInputName(a.BaselineStateID) && a.BaselineSerial != nil
}

// checkImportCreateBaseline re-reads the workspace and confirms it is still
// the supported remote, API-upload root at the baseline the agent prepared
// against. No QueryRun evidence is involved.
func checkImportCreateBaseline(ctx context.Context, c *tfe.Client, a importCreateArgs) (*tfe.Workspace, error) {
	w, err := c.Workspaces.Read(ctx, a.Organization, a.Workspace)
	if err != nil {
		return nil, importReadError(err, 0)
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, a.Organization) {
		return nil, importEvidenceFailure("workspace_ownership_unverified")
	}
	if err := client.AuthorizeOrganization(ctx, w.Organization.Name); err != nil {
		return nil, importEvidenceFailure("organization_not_allowed")
	}
	if w.ExecutionMode != "remote" || w.WorkingDirectory != "" || w.VCSRepo != nil {
		return nil, importEvidenceFailure("execution_source_not_supported")
	}
	if a.BaselineCVID == "" {
		if err := checkImportBlankBaseline(ctx, c, w); err != nil {
			return nil, err
		}
		return w, nil
	}
	if importCurrentConfigurationID(w) != a.BaselineCVID {
		return nil, importEvidenceFailure("baseline_changed")
	}
	sv, err := readImportCurrentState(ctx, c, w.ID)
	if err != nil {
		return nil, err
	}
	if sv.ID != a.BaselineStateID || sv.Serial != *a.BaselineSerial {
		return nil, importEvidenceFailure("baseline_changed")
	}
	return w, nil
}

func createImportCV(ctx context.Context, c *tfe.Client, a importCreateArgs, logger *log.Logger) importCreated {
	if !a.Confirm {
		return importCreateBlocked("confirm_speculative_run_required", "After the user reviews the exact archive, set confirm_speculative_run=true to authorize only a speculative CV create.")
	}
	if !importInputName(a.Organization) || !importInputName(a.Workspace) || !validImportCreateBaseline(a) || a.CVID != "" {
		return importCreateBlocked("import_input_invalid", "Supply organization_name and workspace_name, and either all of baseline_cv_id, baseline_state_id and baseline_state_serial from prepare_import, or none for a blank workspace.")
	}
	w, err := checkImportCreateBaseline(ctx, c, a)
	if err != nil {
		return importCreateBlocked(importDiagnosticCode(err), "Resolve the reported diagnostic; call prepare_import again if the baseline changed.")
	}
	out := importCreated{ContractVersion: importToolContractVersion, WorkspaceID: w.ID, Diagnostics: []string{}}
	mutation, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importCreateBlocked("backend_client_unavailable", "Supply current backend credentials and retry.")
	}
	no, yes := false, true
	cv, err := mutation.ConfigurationVersions.Create(ctx, w.ID, tfe.ConfigurationVersionCreateOptions{AutoQueueRuns: &no, Speculative: &yes})
	if err != nil {
		out.Status, out.CreateOutcome = "blocked", "unknown"
		out.Diagnostics = append(out.Diagnostics, "cv_create_outcome_unknown")
		out.NextAction = "The create outcome is unknown. List the workspace's configuration versions in HCP Terraform and reconcile before creating another. Do not retry blindly."
		return out
	}
	out.ConfigurationVersionID = cv.ID
	if cv.ID == "" || !cv.Speculative || cv.Provisional || cv.AutoQueueRuns || !validImportArtifactLocation(ctx, cv.UploadURL) {
		out.Status, out.CreateOutcome = "blocked", "unknown"
		out.Diagnostics = append(out.Diagnostics, "cv_create_response_invalid")
		out.NextAction = "The create response was not the expected speculative CV. Reconcile the workspace's configuration versions before creating another."
		return out
	}
	out.Status, out.CreateOutcome = "awaiting_agent_upload", "created"
	out.UploadURL = cv.UploadURL
	out.UploadInstructions = "Keep upload_url secret and use it once. PUT the complete reviewed .tar.gz (Content-Type: application/octet-stream) directly to it. If the URL is lost, the server cannot return it again: reconcile the pending CV before considering a new reviewed create. Never upload individual .tf files."
	out.NextAction = "PUT the complete archive to upload_url, confirm the upload succeeded, get a separate confirmation from the user, then call create_import_run with this configuration_version_id. The server has not read the archive."
	return out
}

func createImportRun(ctx context.Context, c *tfe.Client, a importCreateArgs, logger *log.Logger) importCreated {
	if !a.Confirm {
		return importCreateBlocked("confirm_speculative_run_required", "Get explicit user confirmation, then set confirm_speculative_run=true to authorize only a CV-bound plan-only Run.")
	}
	if !importInputName(a.Organization) || !importInputName(a.Workspace) || !importInputName(a.CVID) || !validImportCreateBaseline(a) {
		return importCreateBlocked("import_input_invalid", "Supply organization_name, workspace_name, configuration_version_id from create_import_cv, and the same baseline fields (or none for a blank workspace).")
	}
	lookup := importPrepareInput{Organization: a.Organization, Workspace: a.Workspace, ConfigurationVersionID: a.CVID}
	_, cv, err := readImportExecutionCV(ctx, c, lookup)
	if err != nil {
		return importCreateBlocked(importDiagnosticCode(err), "Resolve the reported diagnostic. Use the configuration_version_id returned by create_import_cv.")
	}
	if cv.Status != tfe.ConfigurationUploaded {
		return importCreateBlocked("execution_cv_not_uploaded", "The configuration version is not uploaded yet. PUT the archive and retry.")
	}
	w, err := checkImportCreateBaseline(ctx, c, a)
	if err != nil {
		return importCreateBlocked(importDiagnosticCode(err), "Resolve the reported diagnostic; call prepare_import again if the baseline changed.")
	}
	out := importCreated{ContractVersion: importToolContractVersion, WorkspaceID: w.ID, ConfigurationVersionID: cv.ID, Diagnostics: []string{}}
	mutation, err := client.NewTfeClientForImportMutation(ctx, logger)
	if err != nil {
		return importCreateBlocked("backend_client_unavailable", "Supply current backend credentials and retry.")
	}
	message := "Speculative Search import inspection"
	r, err := mutation.Runs.Create(ctx, tfe.RunCreateOptions{Workspace: w, ConfigurationVersion: cv, PlanOnly: tfe.Bool(true), AutoApply: tfe.Bool(false), AllowConfigGeneration: tfe.Bool(false), Message: &message})
	if err != nil {
		out.Status, out.CreateOutcome = "blocked", "unknown"
		out.Diagnostics = append(out.Diagnostics, "run_create_outcome_unknown")
		out.NextAction = "The Run create outcome is unknown. Inspect this workspace's Runs for this configuration version before another create. Do not retry blindly."
		return out
	}
	out.RunID = r.ID
	if r.ID == "" || !r.PlanOnly || r.Workspace == nil || r.Workspace.ID != w.ID || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cv.ID {
		out.Status, out.CreateOutcome = "blocked", "unknown"
		out.Diagnostics = append(out.Diagnostics, "run_association_unverified")
		out.NextAction = "The Run did not match the expected workspace, configuration version and plan-only options. Inspect it in HCP Terraform before anything else."
		return out
	}
	out.Status, out.CreateOutcome = "pending", "created"
	out.NextAction = "Poll verify_import_plan with this run_id until the plan finishes. The Run is plan-only and has not imported anything into state."
	return out
}

func importCreateDefinition(name, title, description string, run bool) mcp.Tool {
	opts := []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithTitleAnnotation(title),
		mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(false),
		mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization.")),
		mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Destination workspace name.")),
		mcp.WithString("baseline_cv_id", mcp.Description("Current configuration version ID from prepare_import. Omit all baseline fields only for a verified blank workspace.")),
		mcp.WithString("baseline_state_id", mcp.Description("Current state version ID from prepare_import.")),
		mcp.WithNumber("baseline_state_serial", mcp.Description("Current state serial from prepare_import.")),
		mcp.WithBoolean("confirm_speculative_run", mcp.Required(), mcp.Description("Set true only after explicit user confirmation of this one speculative, non-applying create.")),
	}
	if run {
		opts = append(opts, mcp.WithString("configuration_version_id", mcp.Required(), mcp.Description("Uploaded configuration version ID returned by create_import_cv.")))
	}
	opts = append(opts, mcp.WithSchemaAdditionalProperties(false), mcp.WithOutputSchema[importCreated]())
	return mcp.NewTool(name, opts...)
}

// CreateImportCVDefinition describes create_import_cv.
func CreateImportCVDefinition() mcp.Tool {
	return importCreateDefinition("create_import_cv", "Create speculative import configuration version", `Create a speculative, non-provisional configuration version (auto-queue off) in the destination workspace and return its one-use upload URL. This is a mutation and needs explicit user confirmation of the exact reviewed archive: set confirm_speculative_run=true.

Pass the baseline fields from prepare_import (or none for a verified blank workspace). The server re-checks that the workspace is still a remote, API-upload root at that baseline; it does not re-read the QueryRun and does not read the archive. PUT the complete reviewed .tar.gz directly to upload_url, then call create_import_run. If the outcome is unknown, reconcile in HCP Terraform; never retry blindly.`, false)
}

// CreateImportRunDefinition describes create_import_run.
func CreateImportRunDefinition() mcp.Tool {
	return importCreateDefinition("create_import_run", "Create CV-bound plan-only import run", `Create a plan-only Run bound to the uploaded speculative configuration version (AutoApply=false, config generation off). This is a mutation and needs a separate explicit user confirmation: set confirm_speculative_run=true.

Pass the configuration_version_id from create_import_cv and the same baseline fields. The server re-checks that the CV belongs to the workspace, is speculative and uploaded, and that the workspace baseline is unchanged. It does not re-read the QueryRun. Then call verify_import_plan with the returned run_id. A plan-only Run never imports into state. If the outcome is unknown, reconcile; never retry blindly. This is separate from create_run, which is unchanged.`, true)
}

func handleImportCreate(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, run bool) (*mcp.CallToolResult, error) {
	var a importCreateArgs
	if err := decodeImportToolArguments(request.GetArguments(), &a, 8*1024); err != nil {
		return importToolResult(importCreateBlocked("import_input_invalid", "Check the input fields against the tool schema."))
	}
	if err := client.AuthorizeOrganization(ctx, a.Organization); err != nil {
		return importToolResult(importCreateBlocked("organization_not_allowed", "Use an organization allowed by this server."))
	}
	ctx, cancel := context.WithTimeout(ctx, importHandoffRequestTimeout)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return importToolResult(importCreateBlocked("backend_client_unavailable", "Supply current backend credentials and retry."))
	}
	if run {
		return importToolResult(createImportRun(ctx, c, a, logger))
	}
	return importToolResult(createImportCV(ctx, c, a, logger))
}

// HandleCreateImportCV serves create_import_cv.
func HandleCreateImportCV(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	return handleImportCreate(ctx, request, logger, false)
}

// HandleCreateImportRun serves create_import_run.
func HandleCreateImportRun(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	return handleImportCreate(ctx, request, logger, true)
}
