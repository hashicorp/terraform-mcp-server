package tfregistry

import (
	"context"
	"strings"
	"net/http"

	// "io"

	"github.com/github/github-mcp-server/pkg/translations"

	// "github.com/google/go-github/v69/github" // Removed github client
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	// "hcp-terraform-mcp-server/pkg/hashicorp" // Add import for hashicorp package
)

// ListProviders creates a tool to list Terraform providers.
func ListProviders(registryClient *http.Client, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_providers",
			mcp.WithDescription(t("TOOL_LIST_PROVIDERS_DESCRIPTION", "List providers accessible by the credential.")),
			// TODO: Add pagination parameters here using the correct mcp-go mechanism
			// Example (conceptual):
			// mcp.WithInteger("page_number", mcp.Description("Page number"), mcp.Optional()),
			// mcp.WithInteger("page_size", mcp.Description("Page size"), mcp.Optional()),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// TODO: Parse pagination options
			// pageNumber, _ := OptionalParam[int](request, "page_number")
			// pageSize, _ := OptionalParam[int](request, "page_size")
			
			commonProviders := []string{
				"aws", "google", "azurerm", "kubernetes", 
				"github", "docker", "null", "random",
			}

			// resources := make([]map[string]interface{}, 0, len(commonProviders))
			// for _, provider := range commonProviders {
			// 	resources = append(resources, map[string]interface{}{
			// 		"uri":         fmt.Sprintf("registry://providers/hashicorp/%s", provider),
			// 		"title":       provider,
			// 		"description": fmt.Sprintf("Terraform provider for %s", provider),
			// 	})
			// }

			return mcp.NewToolResultText(strings.Join(commonProviders, ", ")), nil
		}
}
