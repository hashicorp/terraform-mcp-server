package tfenterprise

import (
	"fmt"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/hashicorp/go-tfe"
	"github.com/mark3labs/mcp-go/server"
)

func Init(hcServer *server.MCPServer, token string, address string, enabled []string, readOnly bool, t translations.TranslationHelperFunc) error {
	config := &tfe.Config{
		Address:           address,
		Token:             token,
		RetryServerErrors: true, // Example configuration, adjust as needed
	}

	tfeClient, err := tfe.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create TFE client: %v", err)
	}

	toolsets, err := InitToolsets(enabled, readOnly, tfeClient, t)
	if err != nil {
		return fmt.Errorf("failed to initialize toolsets: %v", err)
	}
	context := InitContextToolset(tfeClient, t)
	dynamic := InitDynamicToolset(hcServer, toolsets, t)
	if err != nil {
		return fmt.Errorf("failed to initialize dynamic toolset: %v", err)
	}

	toolsets.RegisterTools(hcServer)
	context.RegisterTools(hcServer)
	dynamic.RegisterTools(hcServer)

	return nil
}
