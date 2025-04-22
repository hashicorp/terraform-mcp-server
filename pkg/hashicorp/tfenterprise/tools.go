package tfenterprise

import (
	"github.com/github/github-mcp-server/pkg/toolsets"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/server"
)

var DefaultTools = []string{"all"}

func InitToolsets(passedToolsets []string, readOnly bool, tfeClient *tfe.Client, t translations.TranslationHelperFunc) (*toolsets.ToolsetGroup, error) {
	// Create a new toolset group
	tsg := toolsets.NewToolsetGroup(readOnly)

	// Define all available features with their default state (disabled)
	// Create toolsets
	workspaces := toolsets.NewToolset("workspaces", "HCP Terraform related tools").
		AddReadTools(
			toolsets.NewServerTool(ListWorkspaces(tfeClient, t)),
		// toolsets.NewServerTool(GetFileContents(getClient, t)),
		// toolsets.NewServerTool(ListCommits(getClient, t)),
		// toolsets.NewServerTool(SearchCode(getClient, t)),
		// toolsets.NewServerTool(GetCommit(getClient, t)),
		// toolsets.NewServerTool(ListBranches(getClient, t)),
		).
		AddWriteTools(
		// toolsets.NewServerTool(CreateOrUpdateFile(getClient, t)),
		// toolsets.NewServerTool(CreateRepository(getClient, t)),
		// toolsets.NewServerTool(ForkRepository(getClient, t)),
		// toolsets.NewServerTool(CreateBranch(getClient, t)),
		// toolsets.NewServerTool(PushFiles(getClient, t)),
		)
	organizations := toolsets.NewToolset("organizations", "HCP Terraform Organizations related tools").
		AddReadTools(
			toolsets.NewServerTool(ListOrganizations(tfeClient, t)),
		// toolsets.NewServerTool(SearchIssues(getClient, t)),
		// toolsets.NewServerTool(ListIssues(getClient, t)),
		// toolsets.NewServerTool(GetIssueComments(getClient, t)),
		).
		AddWriteTools(
		// toolsets.NewServerTool(CreateIssue(getClient, t)),
		// toolsets.NewServerTool(AddIssueComment(getClient, t)),
		// toolsets.NewServerTool(UpdateIssue(getClient, t)),
		)
	users := toolsets.NewToolset("users", "HCP Terraform Users related tools").
		AddReadTools(
		// toolsets.NewServerTool(SearchUsers(getClient, t)),
		)
	projects := toolsets.NewToolset("projects", "HCP Terraform Projects related tools").
		AddReadTools(
			toolsets.NewServerTool(ListProjects(tfeClient, t)),
		// toolsets.NewServerTool(ListPullRequests(getClient, t)),
		// toolsets.NewServerTool(GetPullRequestFiles(getClient, t)),
		// toolsets.NewServerTool(GetPullRequestStatus(getClient, t)),
		// toolsets.NewServerTool(GetPullRequestComments(getClient, t)),
		// toolsets.NewServerTool(GetPullRequestReviews(getClient, t)),
		).
		AddWriteTools(
		// toolsets.NewServerTool(MergePullRequest(getClient, t)),
		// toolsets.NewServerTool(UpdatePullRequestBranch(getClient, t)),
		// toolsets.NewServerTool(CreatePullRequestReview(getClient, t)),
		// toolsets.NewServerTool(CreatePullRequest(getClient, t)),
		// toolsets.NewServerTool(UpdatePullRequest(getClient, t)),
		// toolsets.NewServerTool(AddPullRequestReviewComment(getClient, t)),
		)

	// Keep experiments alive so the system doesn't error out when it's always enabled
	experiments := toolsets.NewToolset("experiments", "Experimental features that are not considered stable yet")

	// Add toolsets to the group
	tsg.AddToolset(workspaces)
	tsg.AddToolset(organizations)
	tsg.AddToolset(users)
	tsg.AddToolset(projects)
	tsg.AddToolset(experiments)
	// Enable the requested features

	if err := tsg.EnableToolsets(passedToolsets); err != nil {
		return nil, err
	}

	return tsg, nil
}

func InitContextToolset(tfeClient *tfe.Client, t translations.TranslationHelperFunc) *toolsets.Toolset {
	// Create a new context toolset
	contextTools := toolsets.NewToolset("context", "Tools that provide context about the current user and HCP Terraform context you are operating in").
		AddReadTools(
		// toolsets.NewServerTool(GetMe(tfeClient, t)),
		)
	contextTools.Enabled = true
	return contextTools
}

// InitDynamicToolset creates a dynamic toolset that can be used to enable other toolsets, and so requires the server and toolset group as arguments
func InitDynamicToolset(s *server.MCPServer, tsg *toolsets.ToolsetGroup, t translations.TranslationHelperFunc) *toolsets.Toolset {
	// Create a new dynamic toolset
	// Need to add the dynamic toolset last so it can be used to enable other toolsets
	dynamicToolSelection := toolsets.NewToolset("dynamic", "Discover HCP Terraform MCP tools that can help achieve tasks by enabling additional sets of tools, you can control the enablement of any toolset to access its tools when this toolset is enabled.").
		AddReadTools(
		// toolsets.NewServerTool(ListAvailableToolsets(tsg, t)),
		// toolsets.NewServerTool(GetToolsetsTools(tsg, t)),
		// toolsets.NewServerTool(EnableToolset(s, tsg, t)),
		)
	dynamicToolSelection.Enabled = true
	return dynamicToolSelection
}
