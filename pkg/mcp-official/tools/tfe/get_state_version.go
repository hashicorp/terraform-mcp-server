// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StateVersionDetails map[string]any

type GetStateVersionArguments struct {
	StateVersionID string `json:"state_version_id,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
}

func GetStateVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_state_version",
		Description: "Retrieves a Terraform state version. If state_version_id is provided, retrieves that specific state version. Otherwise, retrieves the latest state version for the specified workspace. One of state_version_id or workspace_id must be provided.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"state_version_id": {
					Type:        "string",
					Description: "Optional StateVersion id to fetch exact version",
				},
				"workspace_id": {
					Type:        "string",
					Description: "Optional Workspace id to fetch latest version",
				},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Terraform state version",
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetStateVersionFunc(ctx context.Context, request *mcp.CallToolRequest, input GetStateVersionArguments) (*mcp.CallToolResult, StateVersionDetails, error) {
	workspaceID := strings.TrimSpace(input.WorkspaceID)
	stateVersionID := strings.TrimSpace(input.StateVersionID)

	if workspaceID == "" && stateVersionID == "" {
		return nil, nil, fmt.Errorf("One of state_version_id or workspace_id must be provided")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	var sv *tfe.StateVersion
	if stateVersionID != "" {
		sv, err = tfeClient.StateVersions.Read(ctx, stateVersionID)
	} else {
		sv, err = tfeClient.StateVersions.ReadCurrent(ctx, workspaceID)
	}

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get state version: %w", err)
	}

	var buf bytes.Buffer
	if err := jsonapi.MarshalPayloadWithoutIncluded(&buf, sv); err != nil {
		return nil, nil, fmt.Errorf("failed to marshal state version: %w", err)
	}

	var output StateVersionDetails
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		return nil, nil, fmt.Errorf("failed to decode state version payload: %w", err)
	}

	return nil, output, nil
}
