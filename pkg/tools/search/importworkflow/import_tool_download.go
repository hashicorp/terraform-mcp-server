// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"slices"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	log "github.com/sirupsen/logrus"
)

type importDownload struct {
	ContractVersion        string   `json:"contract_version"`
	Status                 string   `json:"status"`
	WorkspaceID            string   `json:"workspace_id,omitempty"`
	ConfigurationVersionID string   `json:"configuration_version_id,omitempty"`
	ConfigurationRole      string   `json:"configuration_role,omitempty"`
	WorkingDirectory       string   `json:"working_directory,omitempty"`
	DownloadURL            string   `json:"download_url,omitempty"`
	Instructions           []string `json:"instructions,omitempty"`
	Diagnostics            []string `json:"diagnostics"`
	NextAction             string   `json:"next_action"`
}

func (d importDownload) isBlocked() bool { return d.Status == "blocked" }

var importDownloadInstructions = []string{
	"Use a binary-capable client to GET download_url directly into a file; send no Authorization header. A text/JSON-only sandbox cannot safely transfer a binary archive: use an approved binary-capable client or ask the user to perform the transfer. Do not decode or print archive bytes into tool output.",
	"Treat download_url as a secret. Do not print it, log it, or store it. Call again for a fresh URL on expiry; diagnose persistent denial or other transfer failures rather than assuming expiry.",
	importArchiveURLRule,
	"Extract into the authoring directory, which must be empty or new. Check archive paths and links before extracting. Keep .terraform.lock.hcl and do not add secrets.",
	importArchiveRootRule,
	importSensitiveFileRule,
	importSecretFilesRule,
	"This workflow supports a target workspace whose configuration root setting (working_directory) is empty. If it is set, stop and tell the user. A resource may still go in a local module inside the archive (see the module placement guidance from prepare_import).",
}

// downloadImportConfiguration returns the short-lived archive location of the
// workspace's current configuration version, or, when prepare_import returns agent_schema_required, the state run's configuration version. It never fetches archive bytes
// and never reads the QueryRun log.
func downloadImportConfiguration(ctx context.Context, c *tfe.Client, input importPrepareInput, requestedCV string, logger *log.Logger) importDownload {
	out := importDownload{ContractVersion: importToolContractVersion, Status: "blocked", Diagnostics: []string{}}
	ctxResult := lookupImportConfiguration(ctx, c, input, requestedCV, logger)
	out.WorkspaceID = ctxResult.WorkspaceID
	switch ctxResult.Status {
	case "blank_workspace":
		out.Status = "blank_workspace"
		out.NextAction = "The target workspace has no current configuration or state. " + importNewWorkspaceDirectoryQuestion + " " + importBlankAuthoringDirectoryRule + " Author the complete configuration and lock there, then call create_import_cv. " + importConfirmationRule
		return out
	case "available":
		if ctxResult.Context == nil || ctxResult.Context.ConfigurationVersionID != requestedCV {
			out.Diagnostics = append(out.Diagnostics, "configuration_version_not_current")
			out.NextAction = "The requested configuration version is not the workspace's current one. Call prepare_import again to get the current configuration version ID. No URL was returned."
			return out
		}
		out.Status = "available"
		out.ConfigurationVersionID = ctxResult.Context.ConfigurationVersionID
		out.ConfigurationRole = ctxResult.Context.ConfigurationRole
		out.WorkingDirectory = ctxResult.Context.WorkingDirectory
		out.DownloadURL = ctxResult.Context.DownloadURL
		out.Instructions = slices.Clone(importDownloadInstructions)
		out.NextAction = "Download and extract the archive into the authoring directory (empty or new), then author the resource and import blocks there and review them with the user. The server downloaded nothing."
		return out
	}
	out.Diagnostics = append(out.Diagnostics, ctxResult.Diagnostics...)
	if len(ctxResult.Diagnostics) > 0 && ctxResult.Diagnostics[len(ctxResult.Diagnostics)-1] == "configuration_version_not_current" {
		out.NextAction = "The requested configuration version is neither the workspace's current one nor the one used by the run that produced its current state (when that run has no plan). Call prepare_import again for the IDs. No URL was returned."
	}
	if out.NextAction == "" {
		out.NextAction = "Resolve the reported diagnostic and call this tool again. No archive was downloaded by the server."
	}
	return out
}

// GetImportConfigurationDownloadDefinition describes the download tool.
func GetImportConfigurationDownloadDefinition() mcp.Tool {
	return mcp.NewTool("get_import_configuration_download",
		mcp.WithDescription(`Required arguments: organization_name and workspace_name of the target, prepared_target_workspace_id=prepare_import.workspace_id, configuration_version_id=prepare_import.current_configuration_version_id (or state_run_configuration_version_id when agent_schema_required). Call only after the user chooses an authoring directory. Return a short-lived secret download_url; the server does not transfer archive bytes.

The server checks the target workspace ID before releasing a bearer URL; this is not authorization or archive attestation. It checks the current or permitted state-run CV and returns configuration_role to distinguish them. Use a binary-capable client for plain GET to a file, with no Authorization header; extract into the authoring directory after checking paths. If a sandbox exposes only text/json or cannot write binary files, use an approved client-side binary transfer or ask the user; never serialize bytes or signed URLs into printed tool output. A shell-only client needs secure secret injection into curl configuration/stdin without echoing the URL in the command or logs; if that cannot be assured, stop and ask the user. Call again if the URL expires. A blank workspace returns blank_workspace.`),
		mcp.WithTitleAnnotation("Get current configuration download URL"),
		mcp.WithReadOnlyHintAnnotation(true), mcp.WithDestructiveHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true), mcp.WithIdempotentHintAnnotation(true),
		mcp.WithString("organization_name", mcp.Required(), mcp.Description("HCP Terraform organization.")),
		mcp.WithString("workspace_name", mcp.Required(), mcp.Description("Target workspace name.")),
		mcp.WithString("configuration_version_id", mcp.Required(), mcp.Description("Current or permitted state-run configuration version ID from prepare_import.")),
		mcp.WithString("prepared_target_workspace_id", mcp.Required(), mcp.Description("Target workspace_id returned by prepare_import; checked before returning a bearer URL.")),
		mcp.WithSchemaAdditionalProperties(false),
		mcp.WithOutputSchema[importDownload]())
}

// HandleGetImportConfigurationDownload serves get_import_configuration_download.
func HandleGetImportConfigurationDownload(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	var args struct {
		Organization string `json:"organization_name"`
		Workspace    string `json:"workspace_name"`
		CVID         string `json:"configuration_version_id"`
		TargetID     string `json:"prepared_target_workspace_id"`
	}
	blocked := func(code, next string) (*mcp.CallToolResult, error) {
		return importToolResult(importDownload{ContractVersion: importToolContractVersion, Status: "blocked", Diagnostics: []string{code}, NextAction: next})
	}
	if err := decodeImportToolArguments(request.GetArguments(), &args, maxDownloadArgumentBytes); err != nil || !importInputName(args.Organization) || !importInputName(args.Workspace) || !importInputName(args.CVID) || !importInputName(args.TargetID) {
		return blocked("import_input_invalid", "Supply organization_name, workspace_name, prepared_target_workspace_id and configuration_version_id from prepare_import.")
	}
	if err := client.AuthorizeOrganization(ctx, args.Organization); err != nil {
		return blocked("organization_not_allowed", "Use an organization allowed by this server.")
	}
	ctx, cancel := context.WithTimeout(ctx, importToolRequestTimeout)
	defer cancel()
	c, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return blocked("backend_client_unavailable", "Supply current backend credentials and retry.")
	}
	return importToolResult(downloadImportConfiguration(ctx, c, importPrepareInput{Organization: args.Organization, Workspace: args.Workspace, PreparedTargetID: args.TargetID}, args.CVID, logger))
}
