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
	"Download the archive with a plain HTTP GET that writes binary bytes to a file. Send no Authorization header: the URL is already authorized.",
	"Treat download_url as a secret. Do not print it, log it, or store it. If the download fails or the URL has expired, call this tool again for a fresh one.",
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
		mcp.WithDescription(`Return a short-lived download URL for the target workspace's current configuration archive, so the calling agent does not need its own HCP Terraform API access.

Call this only after prepare_import reports has_current_configuration and the user has chosen the authoring directory. Pass prepared_target_workspace_id from prepare_import.workspace_id and current_configuration_version_id, or when status is agent_schema_required the state_run_configuration_version_id, from prepare_import. The server checks the target workspace ID before releasing a bearer URL; this is not authorization or archive attestation. The tool checks that the CV is the workspace's current configuration version or the permitted state-run configuration version, and returns configuration_role to distinguish them. It returns the URL and download instructions: use a plain GET with no Authorization header, extract into the authoring directory, and treat the URL as a secret. Call again for a fresh URL if it expires. For a workspace with no configuration it returns blank_workspace. The server never downloads the archive.`),
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
