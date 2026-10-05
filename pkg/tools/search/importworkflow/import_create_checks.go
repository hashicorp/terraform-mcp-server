// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"errors"
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

// Absence must be established by the backend, not inferred from a missing CV
// relationship alone. An inaccessible state is not an empty workspace.
func checkImportBlankBaseline(ctx context.Context, c *tfe.Client, w *tfe.Workspace) error {
	current, err := c.Workspaces.ReadByID(ctx, w.ID)
	if err != nil {
		return importReadError(err, 0)
	}
	if current.ID != w.ID || current.Organization == nil || w.Organization == nil || !strings.EqualFold(current.Organization.Name, w.Organization.Name) || importCurrentConfigurationID(current) != "" || current.ExecutionMode != "remote" || current.WorkingDirectory != "" || current.VCSRepo != nil {
		return importEvidenceFailure("blank_workspace_baseline_changed_or_unsupported")
	}
	_, err = c.StateVersions.ReadCurrent(ctx, w.ID)
	if errors.Is(err, tfe.ErrResourceNotFound) {
		return nil
	}
	if err != nil {
		return importReadError(err, 0)
	}
	return importEvidenceFailure("blank_workspace_has_state")
}
