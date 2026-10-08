// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"
)

const defaultStateMaxSizeMB = 200

// stateMaxSizeBytes returns the maximum accepted size of a downloaded state file.
func stateMaxSizeBytes() int64 {
	mb := defaultStateMaxSizeMB
	if v := os.Getenv("TF_STATE_MAX_SIZE_MB"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			mb = parsed
		}
	}
	return int64(mb) * 1024 * 1024
}

// loadWorkspaceState downloads and parses the current state version for the given org/workspace.
func loadWorkspaceState(ctx context.Context, tfeClient *tfe.Client, orgName, workspaceName string) (*terraformState, error) {
	workspace, err := tfeClient.Workspaces.Read(ctx, orgName, workspaceName)
	if err != nil {
		return nil, fmt.Errorf("workspace '%s' not found in org '%s': %w", workspaceName, orgName, err)
	}

	sv, err := tfeClient.StateVersions.ReadCurrent(ctx, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to read current state version: %w", err)
	}
	if sv.DownloadURL == "" {
		return nil, fmt.Errorf("workspace '%s' has no state to download yet", workspaceName)
	}

	raw, err := tfeClient.StateVersions.Download(ctx, sv.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download state: %w", err)
	}
	if maxBytes := stateMaxSizeBytes(); int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("state file is %d bytes, exceeding the %d byte limit (set %s to override)",
			len(raw), maxBytes, "TF_STATE_MAX_SIZE_MB")
	}

	var state terraformState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state JSON: %w", err)
	}
	return &state, nil
}

// resolveWorkspaceState returns the current parsed state for the given org/workspace.
func resolveWorkspaceState(ctx context.Context, orgName, workspaceName string, logger *log.Logger) (*terraformState, error) {
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured: %w", err)
	}

	return loadWorkspaceState(ctx, tfeClient, orgName, workspaceName)
}

// extractResources flattens all resource instances from state, applying redaction.
func extractResources(state *terraformState) []extractedResource {
	var out []extractedResource
	for _, res := range state.Resources {
		for _, inst := range res.Instances {
			attrs, err := redactSensitiveAttrs(inst.Attributes, inst.SensitiveAttributes)
			if err != nil {
				// Fail closed: never return raw attributes if they could not be copied/redacted.
				attrs = map[string]interface{}{"_redaction_error": "[REDACTED - could not safely process attributes]"}
			}
			module := res.Module
			if module == "" {
				module = "(root)"
			}
			deps := inst.Dependencies
			if deps == nil {
				deps = []string{}
			}
			sa := inst.SensitiveAttributes
			if sa == nil {
				sa = []interface{}{}
			}
			out = append(out, extractedResource{
				Address:             buildResourceAddress(res, inst),
				Type:                res.Type,
				Name:                res.Name,
				Module:              module,
				Mode:                res.Mode,
				Provider:            res.Provider,
				Attributes:          attrs,
				SensitiveAttributes: sa,
				Dependencies:        deps,
			})
		}
	}
	return out
}

func buildResourceAddress(res stateResource, inst stateInstance) string {
	prefix := ""
	if res.Module != "" {
		prefix = res.Module + "."
	}
	if res.Mode == "data" {
		prefix += "data."
	}
	base := fmt.Sprintf("%s%s.%s", prefix, res.Type, res.Name)
	if inst.IndexKey == nil {
		return base
	}
	switch v := inst.IndexKey.(type) {
	case string:
		return fmt.Sprintf(`%s["%s"]`, base, v)
	case float64:
		return fmt.Sprintf("%s[%d]", base, int(v))
	default:
		return fmt.Sprintf("%s[%v]", base, v)
	}
}
