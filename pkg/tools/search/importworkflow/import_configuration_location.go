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
type importArchiveLocation struct {
	ConfigurationVersionID string `json:"configuration_version_id"`
	ConfigurationRole      string `json:"configuration_role"`
	DownloadURL            string `json:"download_url"`
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

// importConfigurationLookup is the result of looking up the archive location of
// a target workspace's configuration version. Status is blocked,
// blank_workspace or available.
type importConfigurationLookup struct {
	Status      string
	WorkspaceID string
	Context     *importArchiveLocation
	Diagnostics []string
}

// lookupImportConfiguration finds the archive location of the workspace's
// current configuration version, or, when requestedCV names another version,
// of the configuration version of the run that produced the current state
// (agent-supplied schema path, ADR 0008). Any other version fails closed. The
// server never fetches archive bytes.
func lookupImportConfiguration(ctx context.Context, c *tfe.Client, input importPrepareInput, requestedCV string, logger *log.Logger) importConfigurationLookup {
	result := importConfigurationLookup{Status: "blocked", Diagnostics: []string{}}
	fail := func(err error) importConfigurationLookup {
		result.Diagnostics = append(result.Diagnostics, importDiagnosticCode(err))
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
	if !importInputName(input.PreparedTargetID) || w.ID != input.PreparedTargetID {
		return fail(importEvidenceFailure("prepared_target_workspace_mismatch"))
	}
	if w.CurrentConfigurationVersion == nil || w.CurrentConfigurationVersion.ID == "" {
		if err := checkImportBlankBaseline(ctx, c, w); err == nil {
			result.Status = "blank_workspace"
			return result
		} else if importDiagnosticCode(err) == "evidence_access_denied" || importDiagnosticCode(err) == "backend_evidence_unavailable" || importDiagnosticCode(err) == "evidence_read_interrupted" {
			return fail(err)
		}
		return fail(importEvidenceFailure("configuration_source_unavailable"))
	}
	cvID := w.CurrentConfigurationVersion.ID
	stateRunCV := requestedCV != "" && requestedCV != cvID
	if stateRunCV {
		if err := verifyImportStateRunConfiguration(ctx, c, w.ID, requestedCV); err != nil {
			return fail(err)
		}
		cvID = requestedCV
	}
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
	if current.ID != w.ID || importCurrentConfigurationID(current) != importCurrentConfigurationID(w) || current.WorkingDirectory != w.WorkingDirectory {
		return fail(importEvidenceFailure("baseline_changed"))
	}
	role := "current_configuration"
	if stateRunCV {
		role = "state_run_configuration"
	}
	result.Context = &importArchiveLocation{ConfigurationVersionID: cvID, ConfigurationRole: role, DownloadURL: location, WorkingDirectory: w.WorkingDirectory}
	result.Status = "available"
	return result
}

const importToolRequestTimeout = 30 * time.Second

// verifyImportStateRunConfiguration checks that cvID is the configuration
// version of the run that produced the workspace's current state, and that the
// run has no plan (this path only applies when the schema artifact is missing).
func verifyImportStateRunConfiguration(ctx context.Context, c *tfe.Client, workspaceID, cvID string) error {
	sv, err := readImportCurrentState(ctx, c, workspaceID)
	if err != nil {
		return err
	}
	r, err := readImportSchemaRun(ctx, c, workspaceID, sv)
	if err != nil && importDiagnosticCode(err) != "schema_source_plan_unavailable" {
		return err
	}
	if r == nil || r.ConfigurationVersion == nil || r.ConfigurationVersion.ID != cvID {
		return importEvidenceFailure("configuration_version_not_current")
	}
	if err == nil {
		return importEvidenceFailure("configuration_version_not_current")
	}
	return nil
}
