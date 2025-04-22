package tfenterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	// "io"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/hashicorp/go-tfe"

	// "github.com/google/go-github/v69/github" // Removed github client
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"hcp-terraform-mcp-server/pkg/hashicorp" // Add import for hashicorp package
)

// --- TFE Tools --- //

// displayWorkspace is a helper struct to marshal tfe.Workspace correctly,
// handling jsonapi.NullableAttr[time.Time] fields.
type displayWorkspace struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	Organization     string    `json:"organization,omitempty"` // Assuming organization is available or needed
	CreatedAt        time.Time `json:"created_at"`             // Corrected: Standard time.Time
	VCSRepo          string    `json:"vcs_repo,omitempty"`     // Example: Add other relevant fields
	TerraformVersion string    `json:"terraform_version,omitempty"`
	// TODO: Identify and add the actual field using jsonapi.NullableAttr[time.Time]
	// Example: SomeNullableTimeField *time.Time `json:"some_nullable_time_field,omitempty"`
}

// ListOrganizations creates a tool to list TFE organizations.
func ListOrganizations(tfeClient *tfe.Client, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_organizations",
			mcp.WithDescription(t("TOOL_LIST_ORGANIZATIONS_DESCRIPTION", "List organizations accessible by the credential.")),
			// TODO: Add pagination parameters here using the correct mcp-go mechanism
			// Example (conceptual):
			// mcp.WithInteger("page_number", mcp.Description("Page number"), mcp.Optional()),
			// mcp.WithInteger("page_size", mcp.Description("Page size"), mcp.Optional()),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// TODO: Parse pagination options
			// pageNumber, _ := OptionalParam[int](request, "page_number")
			// pageSize, _ := OptionalParam[int](request, "page_size")

			opts := &tfe.OrganizationListOptions{
				ListOptions: tfe.ListOptions{
					// PageNumber: pageNumber, // Set parsed value
					// PageSize:   pageSize,   // Set parsed value (with defaults/max)
				},
			}

			result, err := tfeClient.Organizations.List(ctx, opts)
			if err != nil {
				// Handle API errors gracefully, maybe return as ToolResult error
				// Check for specific TFE error types if needed
				return mcp.NewToolResultError(fmt.Sprintf("failed to list organizations: %v", err)), nil
			}

			// Marshal the result (Items) to JSON
			r, err := json.Marshal(result.Items)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal organization list response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListProjects creates a tool to list TFE projects within an organization.
func ListProjects(tfeClient *tfe.Client, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_projects",
			mcp.WithDescription(t("TOOL_LIST_PROJECTS_DESCRIPTION", "List projects within a specific organization.")),
			mcp.WithString("organization",
				mcp.Required(),
				mcp.Description("The name of the organization."),
			),
			// TODO: Add pagination parameters here
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := hashicorp.RequiredParam[string](request, "organization")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// TODO: Parse pagination options
			opts := &tfe.ProjectListOptions{
				ListOptions: tfe.ListOptions{
					// Set parsed pagination values
				},
			}

			result, err := tfeClient.Projects.List(ctx, orgName, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list projects for organization %s: %v", orgName, err)), nil
			}

			// Marshal the result (Items) to JSON
			r, err := json.Marshal(result.Items)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal project list response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

// ListWorkspaces creates a tool to list TFE workspaces within an organization.
func ListWorkspaces(tfeClient *tfe.Client, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_workspaces",
			mcp.WithDescription(t("TOOL_LIST_WORKSPACES_DESCRIPTION", "List workspaces within a specific organization.")),
			mcp.WithString("organization",
				mcp.Required(),
				mcp.Description("The name of the organization."),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			orgName, err := hashicorp.RequiredParam[string](request, "organization")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			opts := &tfe.WorkspaceListOptions{
				ListOptions: tfe.ListOptions{
					// Set parsed pagination values
				},
			}

			result, err := tfeClient.Workspaces.List(ctx, orgName, opts)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to list workspaces for organization %s: %v", orgName, err)), nil
			}

			// Create a slice of our displayWorkspace for marshaling
			displayItems := make([]displayWorkspace, 0, len(result.Items))
			for _, item := range result.Items {
				dw := displayWorkspace{
					ID:          item.ID,
					Name:        item.Name,
					Description: item.Description,
					// Organization: item.Organization.Name, // Assuming Organization is a struct with Name
					CreatedAt: item.CreatedAt, // Corrected: Direct assignment for time.Time
					// VCSRepo: item.VCSRepoIdentifier(), // Check correct way to get VCS info if needed
					TerraformVersion: item.TerraformVersion,
					// Add other field assignments here
				}

				// TODO: Handle the actual nullable time field here
				// if item.SomeFieldUsingNullableAttr.Present {
				//  nullableTime := item.SomeFieldUsingNullableAttr.Value
				// 	dw.SomeNullableTimeField = &nullableTime
				// }

				displayItems = append(displayItems, dw)
			}

			// Marshal the displayItems slice instead of result.Items
			r, err := json.Marshal(displayItems)
			if err != nil {
				// Provide more context in the error message
				return nil, fmt.Errorf("failed to marshal display workspace list response: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

/* // Removed GitHub Search Functions
// SearchRepositories creates a tool to search for GitHub repositories.
func SearchRepositories(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_repositories",
// ... rest of SearchRepositories ...
}

// SearchCode creates a tool to search for code across GitHub repositories.
func SearchCode(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_code",
// ... rest of SearchCode ...
}

// SearchUsers creates a tool to search for GitHub users.
func SearchUsers(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("search_users",
// ... rest of SearchUsers ...
}
*/
