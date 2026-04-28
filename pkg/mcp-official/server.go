package mcpofficial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/hashicorp/terraform-mcp-server/version"
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

func NewServer() *mcp.Server {
	svr := mcp.NewServer(
		&mcp.Implementation{
			Name:    "terraform-mcp-official",
			Version: "1.0.0",
		},
		nil,
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

func CreateTfeClientForSession(ctx context.Context, sessionID string) (*tfe.Client, error) {
	var err error
	terraformAddress, ok := ctx.Value(contextKey(TerraformAddress)).(string)
	if !ok || terraformAddress == "" {
		terraformAddress = utils.GetEnv(TerraformAddress, DefaultTerraformAddress)
	}

	terraformToken, ok := ctx.Value(contextKey(TerraformToken)).(string)
	if !ok || terraformToken == "" {
		terraformToken = utils.GetEnv(TerraformToken, "")
	}
	if terraformToken == "" {
		log.Print("Terraform token is empty")
		return nil, fmt.Errorf("terraform token is required but not found in context or environment variables")
	}

	client, err := NewTfeClient(sessionID, terraformAddress, parseTerraformSkipTLSVerify(ctx), terraformToken)
	return client, err
}

func NewTfeClient(sessionId string, terraformAddress string, terraformSkipTLSVerify bool, terraformToken string) (*tfe.Client, error) {
	if terraformToken == "" {
		log.Print("No Terraform token provided, TFE client will not be available")
		return nil, fmt.Errorf("required input: no Terraform token provided")
	}

	config := &tfe.Config{
		Address:           terraformAddress,
		Token:             terraformToken,
		RetryServerErrors: true,
		Headers:           make(http.Header),
	}

	config.Headers.Set("User-Agent", fmt.Sprintf("terraform-mcp-server/%s", version.GetHumanVersion()))
	config.HTTPClient = createHTTPClient(terraformSkipTLSVerify)

	client, err := tfe.NewClient(config)
	if err != nil {
		log.Printf("Failed to create a Terraform Cloud/Enterprise client: %v", err)
		return nil, err
	}

	activeTfeClients.Store(sessionId, client)
	log.Printf("Created TFE client for session_id: %s", sessionId)
	return client, nil
}

func parseTerraformSkipTLSVerify(ctx context.Context) bool {
	terraformSkipTLSVerifyStr, ok := ctx.Value(contextKey(TerraformSkipTLSVerify)).(string)
	if !ok || terraformSkipTLSVerifyStr == "" {
		terraformSkipTLSVerifyStr = utils.GetEnv(TerraformSkipTLSVerify, "")
	}
	if terraformSkipTLSVerifyStr != "" {
		terraformSkipTLSVerify, err := strconv.ParseBool(terraformSkipTLSVerifyStr)
		if err == nil {
			return terraformSkipTLSVerify
		}
	}
	return false
}

// createHTTPClient initializes a retryable HTTP client
func createHTTPClient(insecureSkipVerify bool) *http.Client {
	retryClient := retryablehttp.NewClient()
	retryClient.Logger = log.New()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
	}
	transport.Proxy = http.ProxyFromEnvironment

	retryClient.HTTPClient = cleanhttp.DefaultClient()
	retryClient.HTTPClient.Timeout = 10 * time.Second
	retryClient.HTTPClient.Transport = transport
	retryClient.RetryMax = 3

	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			resetAfterInt, err := strconv.ParseInt(resetAfter, 10, 64)
			if err != nil {
				return 0
			}
			resetAfterTime := time.Unix(resetAfterInt, 0)
			return time.Until(resetAfterTime)
		}
		return 0
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			resetAfter := resp.Header.Get("x-ratelimit-reset")
			return resetAfter != "", nil
		}
		return false, nil
	}

	return retryClient.StandardClient()
}
