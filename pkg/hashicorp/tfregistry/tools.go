package tfregistry

import (
	"net/http"

	"github.com/github/github-mcp-server/pkg/toolsets"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/mark3labs/mcp-go/server"
)

var DefaultTools = []string{"all"}

func InitToolsets(passedToolsets []string, readOnly bool, registryClient *http.Client, t translations.TranslationHelperFunc) (*toolsets.ToolsetGroup, error) {

	tsg := toolsets.NewToolsetGroup(readOnly)

	providers := toolsets.NewToolset("providers", "Terraform registry related tools").
		AddReadTools(
			toolsets.NewServerTool(ListProviders(registryClient, t)),
		).
		AddWriteTools()

	// Keep experiments alive so the system doesn't error out when it's always enabled
	experiments := toolsets.NewToolset("experiments", "Experimental features that are not considered stable yet")

	// Add toolsets to the group
	tsg.AddToolset(providers)
	tsg.AddToolset(experiments)

	// Enable the requested features
	if err := tsg.EnableToolsets(passedToolsets); err != nil {
		return nil, err
	}

	return tsg, nil
}

func InitContextToolset(registryClient *http.Client, t translations.TranslationHelperFunc) *toolsets.Toolset {
	// Create a new context toolset
	contextTools := toolsets.NewToolset("context", "Tools that provide context about the current user and Terraform context you are operating in").
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
	dynamicToolSelection := toolsets.NewToolset("dynamic", "Discover Terraform MCP tools that can help achieve tasks by enabling additional sets of tools, you can control the enablement of any toolset to access its tools when this toolset is enabled.").
		AddReadTools(
		// toolsets.NewServerTool(ListAvailableToolsets(tsg, t)),
		// toolsets.NewServerTool(GetToolsetsTools(tsg, t)),
		// toolsets.NewServerTool(EnableToolset(s, tsg, t)),
		)
	dynamicToolSelection.Enabled = true
	return dynamicToolSelection
}
