// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package search

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
	result.NextAction = "Download and unpack the archive in an agent-controlled directory immediately. Preserve the complete tree and provider lock selections; author resource/import HCL, optionally run terraform fmt/validate locally, and review the proposed changes with the user. The URL is a temporary bearer capability; do not log or share it. No archive was downloaded by the MCP server."
	return result
}

const importHandoffRequestTimeout = 30 * time.Second
