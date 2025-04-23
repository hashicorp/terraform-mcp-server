package tfregistry

import (
	"net/http"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterResources(s *server.MCPServer, registryClient *http.Client, t translations.TranslationHelperFunc) {
	// s.AddResourceTemplate(GetProviderResourceContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceBranchContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceCommitContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceTagContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourcePrContent(getClient, t))
}

// func GetProviderResourceContent(tfeClient *tfe.Client, t translations.TranslationHelperFunc) (mcp.ResourceTemplate, server.ResourceTemplateHandlerFunc) {
// 	return mcp.NewResourceTemplate(
// 			"workspace://{organization}/{workspace}/contents{/path*}", // Resource template
// 			t("RESOURCE_WORKSPACE_CONTENT_DESCRIPTION", "Workspace Content"),
// 		),
// 		WorkspaceResourceContentsHandler(tfeClient)
// }
