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

	type SearchArgs struct {
		// Required field
		TerraformOrgName string `json:"terraform_org_name" jsonschema:"description=The Terraform organization name,required"`

		// Optional fields (will be empty strings if not provided)
		ProjectID    string `json:"project_id" jsonschema:"description=Filter by project ID"`
		SearchQuery  string `json:"search_query" jsonschema:"description=Search term"`
		Tags         string `json:"tags" jsonschema:"description=Comma-separated tags"`
		ExcludeTags  string `json:"exclude_tags" jsonschema:"description=Tags to exclude"`
		WildcardName string `json:"wildcard_name" jsonschema:"description=Wildcard pattern"`
	}

	mcp.AddTool(svr, &mcp.Tool{
		Name:        "list_workspaces",
		Description: "List Terraform workspaces in an organization",
	}, func(ctx context.Context, request *mcp.CallToolRequest, input SearchArgs) (*mcp.CallToolResult, *WorkspaceSummaryList, error) {
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

		session := request.Session
		if session == nil {
			return nil, nil, fmt.Errorf("no active session")
		}
		var client *tfe.Client
		if value, ok := activeTfeClients.Load(session.ID()); ok {
			// Try to get existing client
			client = value.(*tfe.Client)
		}
		var err error
		if client == nil {
			log.Printf("TFE client not found, creating a new one")
			client, err = CreateTfeClientForSession(ctx, session.ID())
			if err != nil {
				return nil, nil, err
			}
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
	})
	return svr
}
