package mcpofficial

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	log "github.com/sirupsen/logrus"
)

var activeTfeClients sync.Map

type contextKey string

const (
	TerraformAddress        = "TFE_ADDRESS"
	TerraformToken          = "TFE_TOKEN"
	TerraformSkipTLSVerify  = "TFE_SKIP_TLS_VERIFY"
	DefaultTerraformAddress = "https://app.terraform.io"
)

func NewServer(heartbeatInterval time.Duration) *mcp.Server {
	var svrOptions *mcp.ServerOptions
	if heartbeatInterval > 0 {
		log.Infof("HTTP heartbeat enabled with interval: %v", heartbeatInterval)
		svrOptions = &mcp.ServerOptions{
			KeepAlive: heartbeatInterval,
		}
	}
	svr := mcp.NewServer(
		&mcp.Implementation{
			Name:    "terraform-mcp-official",
			Version: "1.0.0",
		},
		svrOptions,
	)

	// list_workspaces
	type WorkspaceSummary struct {
		ID            string    `json:"id"`
		Name          string    `json:"workspace_name"`
		Description   string    `json:"description"`
		Environment   string    `json:"environment"`
		CreatedAt     time.Time `json:"created_at"`
		ExecutionMode string    `json:"execution_mode"`
	}

	type WorkspaceSummaryList struct {
		Items []*WorkspaceSummary `json:"items"`
	}

	type ListWorkspacesArgs struct {
		TerraformOrgName string `json:"terraform_org_name" jsonschema:"required,description=The Terraform organization name"`
		ProjectID        string `json:"project_id,omitempty" jsonschema:"description=Filter by project ID"`
		SearchQuery      string `json:"search_query,omitempty" jsonschema:"description=Search term"`
		Tags             string `json:"tags,omitempty" jsonschema:"description=Comma-separated tags"`
		ExcludeTags      string `json:"exclude_tags,omitempty" jsonschema:"description=Tags to exclude"`
		WildcardName     string `json:"wildcard_name,omitempty" jsonschema:"description=Wildcard pattern"`
	}

	mcp.AddTool(svr, &mcp.Tool{
		Name:        "list_workspaces",
		Description: "List Terraform workspaces in an organization",
	}, func(ctx context.Context, request *mcp.CallToolRequest, input ListWorkspacesArgs) (*mcp.CallToolResult, *WorkspaceSummaryList, error) {
		terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
		if terraformOrgName == "" {
			return nil, nil, fmt.Errorf("terraform_org_name is required")
		}

		var tags []string
		if input.Tags != "" {
			tags = strings.Split(strings.TrimSpace(input.Tags), ",")
			for i, tag := range tags {
				tags[i] = strings.TrimSpace(tag)
			}
		}

		var excludeTags []string
		if input.ExcludeTags != "" {
			excludeTags = strings.Split(strings.TrimSpace(input.ExcludeTags), ",")
			for i, tag := range excludeTags {
				excludeTags[i] = strings.TrimSpace(tag)
			}
		}

		client, err := getClientFromSession(ctx, request)
		if err != nil {
			return nil, nil, err
		}

		workspaces, err := client.Workspaces.List(ctx, terraformOrgName, &tfe.WorkspaceListOptions{
			ProjectID:    input.ProjectID,
			Search:       input.SearchQuery,
			Tags:         strings.Join(tags, ","),
			ExcludeTags:  strings.Join(excludeTags, ","),
			WildcardName: input.WildcardName,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list workspaces in org '%s': %w", terraformOrgName, err)
		}
		if len(workspaces.Items) == 0 {
			return nil, nil, fmt.Errorf("no workspaces found in organization %q", terraformOrgName)
		}

		summaries := make([]*WorkspaceSummary, len(workspaces.Items))
		for i, w := range workspaces.Items {
			summaries[i] = &WorkspaceSummary{
				ID:            w.ID,
				Name:          w.Name,
				Description:   w.Description,
				Environment:   w.Environment,
				CreatedAt:     w.CreatedAt,
				ExecutionMode: w.ExecutionMode,
			}
		}
		return nil, &WorkspaceSummaryList{Items: summaries}, nil
	})

	// attach_policy_set_to_workspaces
	type AttachPolicySetArgs struct {
		PolicySetID  string `json:"policy_set_id" jsonschema:"required,description=The ID of the policy set to attach (e.g. polset-3yVQZvHzf5j3WRJ1)"`
		WorkspaceIDs string `json:"workspace_ids" jsonschema:"required,description=Comma-separated list of workspace IDs to attach the policy set to"`
	}

	type AttachPolicySetResult struct {
		Message string `json:"message"`
		Count   int    `json:"workspaces_attached"`
	}

	mcp.AddTool(svr, &mcp.Tool{
		Name:        "attach_policy_set_to_workspaces",
		Description: "Attach a policy set to one or more workspaces. Note: Policy sets marked as global cannot be attached to individual workspaces.",
	}, func(ctx context.Context, request *mcp.CallToolRequest, input AttachPolicySetArgs) (*mcp.CallToolResult, *AttachPolicySetResult, error) {
		policySetID := strings.TrimSpace(input.PolicySetID)
		if policySetID == "" {
			return nil, nil, fmt.Errorf("policy_set_id is required")
		}

		workspaceIDsList := strings.Split(input.WorkspaceIDs, ",")
		var workspaces []*tfe.Workspace
		for _, id := range workspaceIDsList {
			trimmedID := strings.TrimSpace(id)
			if trimmedID != "" {
				workspaces = append(workspaces, &tfe.Workspace{ID: trimmedID})
			}
		}

		if len(workspaces) == 0 {
			return nil, nil, fmt.Errorf("no valid workspace IDs provided")
		}

		client, err := getClientFromSession(ctx, request)
		if err != nil {
			return nil, nil, err
		}

		err = client.PolicySets.AddWorkspaces(ctx, policySetID, tfe.PolicySetAddWorkspacesOptions{
			Workspaces: workspaces,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to attach policy set '%s' to workspaces: %w", policySetID, err)
		}

		return nil, &AttachPolicySetResult{
			Message: fmt.Sprintf("Successfully attached policy set %s to %d workspace(s)", policySetID, len(workspaces)),
			Count:   len(workspaces),
		}, nil
	})

	return svr
}

// getClientFromSession extracts or creates a TFE client for the current session
func getClientFromSession(ctx context.Context, request *mcp.CallToolRequest) (*tfe.Client, error) {
	session := request.Session
	if session == nil {
		return nil, fmt.Errorf("no active session")
	}

	if value, ok := activeTfeClients.Load(session.ID()); ok {
		return value.(*tfe.Client), nil
	}

	log.Printf("TFE client not found, creating a new one")
	client, err := CreateTfeClientForSession(ctx, session.ID())
	if err != nil {
		return nil, err
	}
	return client, nil
}
