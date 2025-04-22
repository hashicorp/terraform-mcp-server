package tfregistry

import (
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterResources(s *server.MCPServer, getClient GetClientFn, t translations.TranslationHelperFunc) {
	s.AddResourceTemplate(GetTerraformWorkspaceResourceContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceBranchContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceCommitContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourceTagContent(getClient, t))
	// s.AddResourceTemplate(GetRepositoryResourcePrContent(getClient, t))
}
