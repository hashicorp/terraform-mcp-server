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

const workspaceLockedError = "workspace %q is locked and cannot accept new runs. Use the force_unlock_workspace tool to unlock first"

// Run types shared by the safe and destructive variants of the create_run tool.
const (
	runTypePlanAndApply    = "plan_and_apply"
	runTypeRefreshState    = "refresh_state"
	runTypePlanOnly        = "plan_only"
	runTypeAllowEmptyApply = "allow_empty_apply"
	runTypeAutoApprove     = "auto_approve"
	runTypeIsDestroy       = "is_destroy"
)

// CreateRunArguments is the unmarshal target for both create_run variants. The
// wire contract - descriptions, run_type enum, defaults and which fields are
// required - is declared by createRunInputSchema, so these fields carry no
// jsonschema tags.
type CreateRunArguments struct {
	TerraformOrgName string `json:"terraform_org_name"`
	WorkspaceName    string `json:"workspace_name"`
	RunType          string `json:"run_type,omitempty"`
	Message          string `json:"message,omitempty"`
}

// createRunInputSchema builds the create_run input schema. The safe and
// destructive variants differ only in the run types they accept.
func createRunInputSchema(runTypes []any) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"terraform_org_name": {
				Type:        "string",
				Description: terraformOrgNameDescription,
			},
			"workspace_name": {
				Type:        "string",
				Description: "The name of the workspace to create a run in",
			},
			"run_type": {
				Type:        "string",
				Description: "A run type for the run",
				Enum:        runTypes,
				Default:     json.RawMessage(`"` + runTypePlanAndApply + `"`),
			},
			"message": {
				Type:        "string",
				Description: "Optional message for the run",
				Default:     json.RawMessage(`"` + defaultRunComment + `"`),
			},
		},
		Required: []string{"terraform_org_name", "workspace_name"},
	}
}

// CreateRunSafeTool creates a tool to create a new Terraform run without
// destructive options.
func CreateRunSafeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_run",
		Description: "Creates a new Terraform run in the specified workspace.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform run",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(false),
		},
		InputSchema: createRunInputSchema([]any{
			runTypePlanAndApply,
			runTypeRefreshState,
			runTypePlanOnly,
			runTypeAllowEmptyApply,
		}),
	}
}

// CreateRunSafeFunc creates a run, rejecting the destructive run types.
func CreateRunSafeFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments) (*mcp.CallToolResult, any, error) {
	return createRun(ctx, request, input, false)
}

// CreateRunTool creates a tool to create a new Terraform run, including the
// destructive run types.
func CreateRunTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_run",
		Description: "Creates a new Terraform run in the specified workspace.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a new Terraform run",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
		},
		InputSchema: createRunInputSchema([]any{
			runTypePlanAndApply,
			runTypeRefreshState,
			runTypePlanOnly,
			runTypeAllowEmptyApply,
			runTypeAutoApprove,
			runTypeIsDestroy,
		}),
	}
}

// CreateRunFunc creates a run, allowing the destructive run types.
func CreateRunFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments) (*mcp.CallToolResult, any, error) {
	return createRun(ctx, request, input, true)
}

// createRun holds the logic shared by both create_run variants. When
// allowDestructive is false the auto_approve and is_destroy run types are
// rejected instead of being applied to the run options.
func createRun(ctx context.Context, request *mcp.CallToolRequest, input CreateRunArguments, allowDestructive bool) (*mcp.CallToolResult, any, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("missing required input: terraform_org_name")
	}

	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("missing required input: workspace_name")
	}

	runType := input.RunType
	if runType == "" {
		runType = runTypePlanAndApply
	}

	message := input.Message
	if message == "" {
		message = defaultRunComment
	}

	options := &tfe.RunCreateOptions{}
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
		if !allowDestructive {
			return nil, nil, fmt.Errorf("run_type %q is not available, Terraform operations are disabled", runType)
		}
		options.AutoApply = tfe.Bool(true)
	case runTypeIsDestroy:
		if !allowDestructive {
			return nil, nil, fmt.Errorf("run_type %q is not available, Terraform operations are disabled", runType)
		}
		options.IsDestroy = tfe.Bool(true)
	default:
		return nil, nil, fmt.Errorf("invalid run_type: %s", runType)
	}
	options.Message = &message

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	workspace, err := tfeClient.Workspaces.Read(ctx, terraformOrgName, workspaceName)
	if err != nil {
		return nil, nil, fmt.Errorf("workspace '%s' not found in org '%s': %w", workspaceName, terraformOrgName, err)
	}

	if workspace.Locked {
		return nil, nil, fmt.Errorf(workspaceLockedError, workspaceName)
	}
	options.Workspace = workspace

	run, err := tfeClient.Runs.Create(ctx, *options)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create run: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := jsonapi.MarshalPayloadWithoutIncluded(buf, run); err != nil {
		return nil, nil, fmt.Errorf("failed to marshal run response: %w", err)
	}

	return textResult(buf.String()), nil, nil
}
