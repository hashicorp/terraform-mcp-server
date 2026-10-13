// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// TODO: Remove `defaultRunMessage` const once runs_action.go is merged, as it defines the same const.
	defaultRunMessage       = "Triggered via Terraform MCP Server"
	runTypePlanAndApply     = "plan_and_apply"
	runTypeRefreshState     = "refresh_state"
	runTypePlanOnly         = "plan_only"
	runTypeAllowEmptyApply  = "allow_empty_apply"
	runTypeAutoApprove      = "auto_approve"
	runTypeDestroy          = "is_destroy"
	workspaceLockedRunError = "workspace %q is locked and cannot accept new runs. Use the force_unlock_workspace tool to unlock first"
)

var (
	safeRunTypes = []string{
		runTypePlanAndApply,
		runTypeRefreshState,
		runTypePlanOnly,
		runTypeAllowEmptyApply,
	}
	allRunTypes = append(append([]string{}, safeRunTypes...), runTypeAutoApprove, runTypeDestroy)
)

// CreateRunArguments holds the inputs for creating a run.
type CreateRunArguments struct {
	TerraformOrgName string  `json:"terraform_org_name"`
	WorkspaceName    string  `json:"workspace_name"`
	RunType          string  `json:"run_type,omitempty"`
	Message          *string `json:"message,omitempty"`
}

// CreateRunResponse reports the run created by the tool.
type CreateRunResponse struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// CreateRunSafeTool describes create_run without destructive run types.
func CreateRunSafeTool() *mcp.Tool {
	return createRunTool(safeRunTypes, false)
}

// CreateRunTool describes create_run with all supported run types.
func CreateRunTool() *mcp.Tool {
	return createRunTool(allRunTypes, true)
}

func createRunTool(runTypes []string, destructive bool) *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_run",
		Description: "Creates a new Terraform run in the specified workspace.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The name of the Terraform Cloud/Enterprise organization",
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace to create a run in",
				},
				"run_type": {
					Type:        "string",
					Description: "The type of run to create",
					Enum:        enumOf(runTypes...),
					Default:     json.RawMessage(`"` + runTypePlanAndApply + `"`),
				},
				"message": {
					Type:        "string",
					Description: "Optional message for the run; use an empty string to omit the message",
					Default:     json.RawMessage(`"` + defaultRunMessage + `"`),
				},
			},
			PropertyOrder:        []string{"terraform_org_name", "workspace_name", "run_type", "message"},
			Required:             []string{"terraform_org_name", "workspace_name"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"run_id":       {Type: "string"},
				"workspace_id": {Type: "string"},
				"status":       {Type: "string"},
				"message":      {Type: "string"},
			},
			PropertyOrder:        []string{"run_id", "workspace_id", "status", "message"},
			Required:             []string{"run_id", "workspace_id", "status", "message"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform run",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(destructive),
		},
	}
}

func CreateRunSafeFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments) (*mcp.CallToolResult, *CreateRunResponse, error) {
	return createRun(ctx, request, input, safeRunTypes)
}

func CreateRunFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments) (*mcp.CallToolResult, *CreateRunResponse, error) {
	return createRun(ctx, request, input, allRunTypes)
}

func createRun(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments, allowedRunTypes []string) (*mcp.CallToolResult, *CreateRunResponse, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}

	runType := input.RunType
	if runType == "" {
		runType = runTypePlanAndApply
	}
	if !slices.Contains(allowedRunTypes, runType) {
		return nil, nil, fmt.Errorf("run_type %q must be one of: %s", runType, strings.Join(allowedRunTypes, ", "))
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("reading workspace %q in organization %q: %w", workspaceName, terraformOrgName, err)
	}
	if workspace.Locked {
		return nil, nil, fmt.Errorf(workspaceLockedRunError, workspaceName)
	}

	options := runCreateOptions(workspace, runType, input.Message)
	run, err := tfeClient.Runs.Create(ctx, options)
	if err != nil {
		return nil, nil, fmt.Errorf("creating run in workspace %q: %w", workspaceName, err)
	}

	return nil, &CreateRunResponse{
		RunID:       run.ID,
		WorkspaceID: workspace.ID,
		Status:      string(run.Status),
		Message:     run.Message,
	}, nil
}

func runCreateOptions(workspace *tfe.Workspace, runType string, message *string) tfe.RunCreateOptions {
	options := tfe.RunCreateOptions{Workspace: workspace}

	switch runType {
	case runTypePlanAndApply:
		options.AutoApply = tfe.Bool(false)
	case runTypeRefreshState:
		options.RefreshOnly = tfe.Bool(true)
	case runTypePlanOnly:
		options.PlanOnly = tfe.Bool(true)
	case runTypeAllowEmptyApply:
		options.AllowEmptyApply = tfe.Bool(true)
	case runTypeAutoApprove:
		options.AutoApply = tfe.Bool(true)
	case runTypeDestroy:
		options.IsDestroy = tfe.Bool(true)
	}

	runMessage := defaultRunMessage
	if message != nil {
		runMessage = *message
	}
	if runMessage != "" {
		options.Message = tfe.String(runMessage)
	}

	return options
}
