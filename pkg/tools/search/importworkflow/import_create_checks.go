// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/go-tfe"
)

// A CV read has no workspace relationship in go-tfe. The workspace-scoped
// typed list establishes membership, with a strict scan bound (never treat an
// incomplete scan as proof of absence). The caller cannot assert ownership.
func readImportExecutionCV(ctx context.Context, c *tfe.Client, input importPrepareInput) (*tfe.Workspace, *tfe.ConfigurationVersion, error) {
	if !importInputName(input.ConfigurationVersionID) || input.QueryID != "" || len(input.Selections) != 0 {
		return nil, nil, importEvidenceFailure("execution_input_invalid")
	}
	w, err := c.Workspaces.Read(ctx, input.Organization, input.Workspace)
	if err != nil {
		return nil, nil, importReadError(err, 0)
	}
	if w.Organization == nil || !strings.EqualFold(w.Organization.Name, input.Organization) {
		return nil, nil, importEvidenceFailure("workspace_ownership_unverified")
	}
	found := false
	for page := 1; page <= 20; page++ {
		list, err := c.ConfigurationVersions.List(ctx, w.ID, &tfe.ConfigurationVersionListOptions{ListOptions: tfe.ListOptions{PageNumber: page, PageSize: 100}})
		if err != nil {
			return nil, nil, importReadError(err, 0)
		}
		if list == nil || list.Pagination == nil {
			return nil, nil, importEvidenceFailure("execution_cv_membership_unverified")
		}
		for _, item := range list.Items {
			if item != nil && item.ID == input.ConfigurationVersionID {
				found = true
			}
		}
		if found {
			break
		}
		if list.Pagination.NextPage == 0 {
			return nil, nil, importEvidenceFailure("execution_cv_workspace_mismatch")
		}
		if list.Pagination.NextPage <= page {
			return nil, nil, importEvidenceFailure("execution_cv_membership_unverified")
		}
		if page == 20 {
			return nil, nil, importEvidenceFailure("execution_cv_membership_limit")
		}
	}
	cv, err := c.ConfigurationVersions.Read(ctx, input.ConfigurationVersionID)
	if err != nil {
		return nil, nil, importReadError(err, 0)
	}
	if cv.ID != input.ConfigurationVersionID || !cv.Speculative || cv.Provisional || cv.AutoQueueRuns {
		return nil, nil, importEvidenceFailure("execution_cv_not_speculative")
	}
	return w, cv, nil
}

func validImportTargetAddress(address string) bool {
	return strings.TrimSpace(address) == address && address != "" && len(address) <= 512 && !strings.ContainsAny(address, "\x00\r\n")
}

// importBlankWorkspaceDocument is the part of a workspace read that the blank
// baseline needs. Relationships stay raw because the SDK's typed workspace
// cannot tell an omitted current-state relationship (state access denied) from
// an explicit null (authorized, no current state).
type importBlankWorkspaceDocument struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			ExecutionMode    string          `json:"execution-mode"`
			WorkingDirectory string          `json:"working-directory"`
			VCSRepo          json.RawMessage `json:"vcs-repo"`
			Permissions      struct {
				CanReadStateVersions *bool `json:"can-read-state-versions"`
			} `json:"permissions"`
		} `json:"attributes"`
		Relationships map[string]json.RawMessage `json:"relationships"`
	} `json:"data"`
}

// importRelationshipData returns the raw "data" member of a relationship and
// whether both the relationship and its data member were present.
func importRelationshipData(rels map[string]json.RawMessage, name string) (json.RawMessage, bool, error) {
	raw, ok := rels[name]
	if !ok {
		return nil, false, nil
	}
	var rel map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rel); err != nil || rel == nil {
		return nil, false, importEvidenceFailure("evidence_json_invalid")
	}
	data, ok := rel["data"]
	return data, ok, nil
}

// A blank baseline is established only from one bounded workspace read made
// with the current credentials: the caller may read state versions, and the
// current-state relationship is explicitly present and null. A missing
// permission or an omitted relationship means state access is not established,
// never that the workspace is blank. A current-state 404 is ambiguous, so it
// can only corroborate; any other response blocks. The checks are sequential
// reads, not an atomic guarantee against a concurrent state change.
func checkImportBlankBaseline(ctx context.Context, c *tfe.Client, w *tfe.Workspace) error {
	body, status, err := readImportBackendJSON(ctx, c, "workspaces/"+url.PathEscape(w.ID), maxImportPreparationBytes)
	if err != nil || status != http.StatusOK {
		return importReadError(err, status)
	}
	var doc importBlankWorkspaceDocument
	if err := decodeImportEvidenceJSONLimit(body, &doc, maxImportPreparationBytes); err != nil {
		return err
	}
	d := doc.Data
	if d.ID != w.ID || w.Organization == nil {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	orgData, orgPresent, err := importRelationshipData(d.Relationships, "organization")
	if err != nil || !orgPresent {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	var org struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(orgData, &org) != nil || !strings.EqualFold(org.ID, w.Organization.Name) {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	if cvData, present, err := importRelationshipData(d.Relationships, "current-configuration-version"); err != nil {
		return err
	} else if !present || string(bytes.TrimSpace(cvData)) != "null" {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	if d.Attributes.ExecutionMode != "remote" || d.Attributes.WorkingDirectory != "" || (len(d.Attributes.VCSRepo) != 0 && string(bytes.TrimSpace(d.Attributes.VCSRepo)) != "null") {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	if d.Attributes.Permissions.CanReadStateVersions != nil && !*d.Attributes.Permissions.CanReadStateVersions {
		return importEvidenceFailure("evidence_access_denied")
	}
	stateData, present, err := importRelationshipData(d.Relationships, "current-state-version")
	if err != nil {
		return err
	}
	if d.Attributes.Permissions.CanReadStateVersions == nil || !present {
		return importEvidenceFailure("backend_evidence_unavailable")
	}
	if string(bytes.TrimSpace(stateData)) != "null" {
		var state struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(stateData, &state) != nil || state.ID == "" {
			return importEvidenceFailure("evidence_json_invalid")
		}
		return importEvidenceFailure("blank_workspace_has_state")
	}
	// Corroboration only: a 404 alone could not establish a blank workspace.
	var stateStatus int
	ctx = tfe.ContextWithResponseHeaderHook(ctx, func(code int, _ http.Header) { stateStatus = code })
	_, err = c.StateVersions.ReadCurrent(ctx, w.ID)
	if errors.Is(err, tfe.ErrResourceNotFound) {
		return nil
	}
	if err != nil {
		return importReadError(err, stateStatus)
	}
	return importEvidenceFailure("blank_workspace_has_state")
}
