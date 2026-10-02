// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
)

// A URL is a bearer capability even if it contains no Atlas API token. Do not
// log it, store it, or expose it beyond the authorized caller's tool response.
type importConfigurationHandoff struct {
	ConfigurationVersionID string `json:"configuration_version_id"`
	DownloadURL            string `json:"download_url"`
	URLValidity            string `json:"url_validity"`
	WorkingDirectory       string `json:"working_directory"`
}

// Only the stable Atlas URL is authenticated through go-tfe. The SDK client's
// redirect policy is scoped to this request so the server never fetches bytes
// from the preauthorized artifact URL. A later call reacquires a fresh URL.
func readImportConfigurationLocation(ctx context.Context, c *tfe.Client, cvID string) (string, error) {
	if !importInputName(cvID) {
		return "", importEvidenceFailure("configuration_source_unavailable")
	}
	req, err := c.NewRequest(http.MethodGet, "configuration-versions/"+url.PathEscape(cvID)+"/download", nil)
	if err != nil {
		return "", importEvidenceFailure("configuration_download_unavailable")
	}
	status := 0
	location := ""
	ctx = tfe.ContextWithResponseHeaderHook(ctx, func(code int, headers http.Header) {
		status, location = code, headers.Get("Location")
	})
	if err := req.Do(ctx, nil); err != nil {
		return "", importReadError(err, status)
	}
	if status != http.StatusFound || location == "" {
		return "", importEvidenceFailure("configuration_download_location_unavailable")
	}
	if !validImportArtifactLocation(ctx, location) {
		return "", importEvidenceFailure("configuration_download_location_invalid")
	}
	return location, nil
}

func validImportArtifactLocation(ctx context.Context, location string) bool {
	u, err := url.Parse(location)
	return err == nil && u.Host != "" && u.User == nil && u.Fragment == "" && (u.Scheme == "https" || (u.Scheme == "http" && strings.HasPrefix(client.TFEAddressFromContext(ctx), "http://")))
}

func importConfigurationContextFromAPIs(ctx context.Context, c *tfe.Client, input importPrepareInput, logger *log.Logger) importPreparation {
	result := importPreparation{ContractVersion: importPreparationContractVersion, Status: "blocked", Stage: "configuration_handoff", Organization: input.Organization, Diagnostics: []string{}}
	fail := func(err error) importPreparation {
		result.Diagnostics = append(result.Diagnostics, importDiagnosticCode(err))
		result.NextAction = "Resolve the configuration handoff diagnostic and retry. No archive was downloaded by the server."
		return result
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return fail(importEvidenceFailure("workspace_ownership_unverified"))
	}
	result.WorkspaceID = w.ID
	if w.CurrentConfigurationVersion == nil || w.CurrentConfigurationVersion.ID == "" {
		if err := checkImportBlankBaseline(ctx, c, w); err == nil {
			result.Status, result.Stage = "blank_workspace", "configuration_handoff"
			result.Baseline = &importAPIBaseline{WorkingDirectory: w.WorkingDirectory}
			result.NextAction = "There is no current configuration archive or state. Select a finished query candidate, author and locally validate the complete provider/resource/import configuration and lock, then use upload and plan with target_address for a speculative-only import inspection. A separate provider-only schema probe is optional, not required."
			return result
		} else if importDiagnosticCode(err) == "evidence_access_denied" || importDiagnosticCode(err) == "backend_evidence_unavailable" || importDiagnosticCode(err) == "evidence_read_interrupted" {
			return fail(err)
		}
		return fail(importEvidenceFailure("configuration_source_unavailable"))
	}
	cvID := w.CurrentConfigurationVersion.ID
	result.Baseline = &importAPIBaseline{ConfigurationVersionID: cvID, WorkingDirectory: w.WorkingDirectory}
	cv, err := c.ConfigurationVersions.Read(ctx, cvID)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	if cv.ID != cvID || cv.Status != tfe.ConfigurationUploaded {
		return fail(importEvidenceFailure("configuration_source_not_uploaded"))
	}
	// Construct a second SDK client with identical authentication and transport,
	// but do not follow this one endpoint's redirect into an archive download.
	redirectClient, err := client.NewTfeClientForDownloadLocation(ctx, logger)
	if err != nil {
		return fail(importEvidenceFailure("backend_client_unavailable"))
	}
	location, err := readImportConfigurationLocation(ctx, redirectClient, cvID)
	if err != nil {
		return fail(err)
	}
	current, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return fail(importReadError(err, 0))
	}
	if current.ID != w.ID || importCurrentConfigurationID(current) != cvID || current.WorkingDirectory != w.WorkingDirectory {
		return fail(importEvidenceFailure("baseline_changed"))
	}
	result.Context = &importConfigurationHandoff{ConfigurationVersionID: cvID, DownloadURL: location, URLValidity: "temporary; use immediately and request a fresh context handoff if expired (nominally one minute)", WorkingDirectory: w.WorkingDirectory}
	result.Status, result.Stage = "available", "configuration_handoff"
	result.Notes = append(result.Notes, "current_state_not_observed_by_context; use prepare to verify current state ID and serial before upload or plan")
	result.NextAction = "Choose an isolated temporary directory or a user-approved directory in the agent's workspace before client download; neither is an MCP filesystem path. This URL is short-lived: if the choice takes time or the URL expires, request a fresh context URL. Use a client-local binary-capable HTTP GET to a file; a browser is not required. Read archive bytes, not text/JSON, using a method supported by your client. Keep the signed URL out of printed output, logs and exposed command arguments; if no safe method exists, stop and ask instead of retrying a disconnected browser. Verify the archive before unpacking, preserve the full tree and provider lock, exclude client-generated .terraform caches and do not add secrets; review sensitive existing files rather than dropping them. Check working_directory (only remote root is supported) before uploading the complete reviewed archive. MCP downloaded no archive."
	return result
}

const importHandoffRequestTimeout = 30 * time.Second
