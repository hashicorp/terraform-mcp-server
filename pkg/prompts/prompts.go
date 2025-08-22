// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package prompts

import (
	"context"
	_ "embed"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

//go:embed create_new_env.md
var createNewEnvPrompt string

// Prompt names, identifiers and descriptions
const (
	PromptNameEnvironmentSetup = "hcp_terraform_environment_setup"
	PromptDescriptionEnvironmentSetup = "Comprehensive guide for setting up a new HCP Terraform workspace environment with configuration, variables, state management, and deployment automation. Uses semi-automated settings with automatic planning (auto_queue_runs=true) but manual approval required for all deployments (auto_apply=false)."

)

// InitPrompts registers all prompts with the MCP server
func InitPrompts(hcServer *server.MCPServer, logger *log.Logger) {
	logger.Info("Initializing prompts...")

	// Register the HCP Terraform Environment Setup prompt
	createNewEnvPromptTool := CreateNewEnvironmentPrompt(logger)
	hcServer.AddPrompt(createNewEnvPromptTool.Prompt, createNewEnvPromptTool.Handler)

	logger.Info("Prompts initialized successfully")
}

// PromptTool represents a prompt with its handler
type PromptTool struct {
	Prompt  mcp.Prompt
	Handler server.PromptHandlerFunc
}

// CreateNewEnvironmentPrompt creates the HCP Terraform environment setup prompt
func CreateNewEnvironmentPrompt(logger *log.Logger) *PromptTool {
	prompt := mcp.Prompt{
		Name:        PromptNameEnvironmentSetup,
		Description: PromptDescriptionEnvironmentSetup,
	}

	handler := func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		logger.Debugf("Processing prompt request for: %s", request.Params.Name)

		result := &mcp.GetPromptResult{
			Description: prompt.Description,
			Messages: []mcp.PromptMessage{
				{
					Role: "user",
					Content: mcp.TextContent{
						Type: "text",
						Text: createNewEnvPrompt,
					},
				},
			},
		}

		logger.Debugf("Returning environment setup prompt")

		return result, nil
	}

	return &PromptTool{
		Prompt:  prompt,
		Handler: handler,
	}
}
