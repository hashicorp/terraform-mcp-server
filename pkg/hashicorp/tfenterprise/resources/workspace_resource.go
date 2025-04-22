package tfenterprise

import (
	"context"
	"errors"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetTerraformWorkspaceResourceContent defines the resource template and handler for getting workspace content.
func GetTerraformWorkspaceResourceContent(tfeClient *tfe.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
	return mcp.NewResourceTemplate(
			"workspace://{organization}/{workspace}/contents{/path*}", // Resource template
			t("RESOURCE_WORKSPACE_CONTENT_DESCRIPTION", "Workspace Content"),
		),
		WorkspaceResourceContentsHandler(tfeClient)
}

// RepositoryResourceContentsHandler returns a handler function for repository content requests.
func WorkspaceResourceContentsHandler(tfeClient *tfe.Client) func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// the matcher will give []string with one element
		// https://github.com/mark3labs/mcp-go/pull/54
		o, ok := request.Params.Arguments["organization"].([]string)
		if !ok || len(o) == 0 {
			return nil, errors.New("organization is required")
		}
		organization := o[0]

		workspaces, err := tfeClient.Workspaces.List(ctx, organization, nil)
		if err != nil {
			return nil, err
		}

		if workspaces != nil {
			var resources []mcp.ResourceContents
			for _, entry := range workspaces.Items {
				// A workspace itself doesn't have a file-like MIME type,
				// represent it plainly.
				var uri string = "#" // Default URI
				if linkObj, ok := entry.Links["self-html"]; ok {
					if strLink, ok := linkObj.(string); ok { // Assert the interface{} to string
						uri = strLink // Assign if assertion is successful
					}
				}
				resources = append(resources, mcp.TextResourceContents{
					URI:      uri, // Use the asserted or default URI
					MIMEType: "text/plain",
					Text:     entry.Name, // Use the workspace name
				})
			}
			return resources, nil
		}

		// Return an empty list gracefully.
		return []mcp.ResourceContents{}, nil
	}
}
