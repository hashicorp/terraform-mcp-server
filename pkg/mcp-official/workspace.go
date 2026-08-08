package mcpofficial

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-tfe"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type WorkspaceSummary struct {
	ID            string    `json:"id"`
	Name          string    `json:"workspace_name"`
	Description   string    `json:"description"`
	Environment   string    `json:"environment"`
	CreatedAt     time.Time `json:"created_at"`
	ExecutionMode string    `json:"execution_mode"`
}

// WorkspaceSummaryList contains the list of workspace summaries and pagination details
type WorkspaceSummaryList struct {
	Items []*WorkspaceSummary `json:"items"`
}

type ListWorkspacesArguments struct {
	// Required field
	TerraformOrgName string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`

	// Optional fields (will be empty strings if not provided)
	ProjectID    string `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
	SearchQuery  string `json:"search_query,omitempty" jsonschema:"Search term"`
	Tags         string `json:"tags,omitempty" jsonschema:"Comma-separated tags"`
	ExcludeTags  string `json:"exclude_tags,omitempty" jsonschema:"Tags to exclude"`
	WildcardName string `json:"wildcard_name,omitempty" jsonschema:"Wildcard pattern"`
}

func ListWorkpsacesTool() *mcp.Tool {
	trueVal := true
	falseVal := false
	return &mcp.Tool{
		Name:        "list_workspaces",
		Description: "Search and list Terraform workspaces within a specified organization. Returns all workspaces when no filters are applied, or filters results based on name patterns, tags, or search queries. Supports pagination for large result sets. Returns a truncated summary of the workspace, use get_workspace_details to get the full details for a specific workspace.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "List Terraform workspaces with queries",
			OpenWorldHint:   &trueVal,
			ReadOnlyHint:    trueVal,
			DestructiveHint: &falseVal,
		},
	}
}

func ListWorkspacesFunc(ctx context.Context, request *mcp.CallToolRequest, input ListWorkspacesArguments) (*mcp.CallToolResult, *WorkspaceSummaryList, error) {
	log.Info("ListWorkspaces for official mcp go-dk called..")
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	projectID := input.ProjectID
	searchQuery := input.SearchQuery
	tagsStr := input.Tags
	excludeTagsStr := input.ExcludeTags
	wildcardName := input.WildcardName

	var tags []string
	if tagsStr != "" {
		tags = strings.Split(strings.TrimSpace(tagsStr), ",")
		for i, tag := range tags {
			tags[i] = strings.TrimSpace(tag)
		}
	}

	var excludeTags []string
	if excludeTagsStr != "" {
		excludeTags = strings.Split(strings.TrimSpace(excludeTagsStr), ",")
		for i, tag := range excludeTags {
			excludeTags[i] = strings.TrimSpace(tag)
		}
	}

	client, err := GetTfeClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	workspaces, err := client.Workspaces.List(ctx, terraformOrgName, &tfe.WorkspaceListOptions{
		ProjectID:    projectID,
		Search:       searchQuery,
		Tags:         strings.Join(tags, ","),
		ExcludeTags:  strings.Join(excludeTags, ","),
		WildcardName: wildcardName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list workspaces in org '%s': %w", terraformOrgName, err)
	}
	if len(workspaces.Items) == 0 {
		return nil, nil, fmt.Errorf("no workspaces to list in organization %q", terraformOrgName)
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
	return nil, &WorkspaceSummaryList{
		Items: summaries,
	}, nil
}
