// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/tfe"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// DynamicToolRegistry manages the availability of tools based on session state
type DynamicToolRegistry struct {
	mu                 sync.RWMutex
	sessionsWithTFE    map[string]bool // sessionID -> hasTFEClient
	tfeToolsRegistered bool
	mcpServer          *server.MCPServer
	logger             *log.Logger
}

var globalToolRegistry *DynamicToolRegistry

// registerDynamicTools registers the global tool registry
func registerDynamicTools(mcpServer *server.MCPServer, logger *log.Logger) {
	globalToolRegistry = &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          mcpServer,
		logger:             logger,
	}

	// Set the callback in the client package to avoid circular imports
	client.SetToolRegistryCallback(globalToolRegistry)
}

// GetDynamicToolRegistry returns the global tool registry instance
func GetDynamicToolRegistry() *DynamicToolRegistry {
	return globalToolRegistry
}

// RegisterSessionWithTFE marks a session as having a valid TFE client
func (r *DynamicToolRegistry) RegisterSessionWithTFE(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessionsWithTFE[sessionID] = true
	r.logger.Info("Session registered with TFE client")

	// If this is the first session with TFE, register the tools
	if !r.tfeToolsRegistered {
		r.registerTFETools()
	}
}

// UnregisterSessionWithTFE removes a session from the TFE registry
func (r *DynamicToolRegistry) UnregisterSessionWithTFE(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.sessionsWithTFE, sessionID)
	r.logger.Info("Session unregistered from TFE client")

	// If no sessions have TFE clients, we could unregister tools
	// but since MCP doesn't support tool removal, we keep them registered
	// and rely on runtime checks
}

// HasSessionWithTFE checks if a specific session has a TFE client
func (r *DynamicToolRegistry) HasSessionWithTFE(sessionID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.sessionsWithTFE[sessionID]
}

// HasAnySessionWithTFE checks if any session has a TFE client
func (r *DynamicToolRegistry) HasAnySessionWithTFE() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.sessionsWithTFE) > 0
}

// registerTFETools registers TFE tools with the MCP server
func (r *DynamicToolRegistry) registerTFETools() {
	if r.tfeToolsRegistered {
		return
	}

	r.logger.Info("Registering TFE tools - first session with valid TFE client detected")

	// Create TFE tools with dynamic availability checking
	listTerraformOrgsTool := r.createDynamicTFETool("list_terraform_orgs", tfeTools.ListTerraformOrgs)
	r.mcpServer.AddTool(listTerraformOrgsTool.Tool, listTerraformOrgsTool.Handler)

	listTerraformProjectsTool := r.createDynamicTFETool("list_terraform_projects", tfeTools.ListTerraformProjects)
	r.mcpServer.AddTool(listTerraformProjectsTool.Tool, listTerraformProjectsTool.Handler)

	// Workspace management tools
	ListWorkspacesTool := r.createDynamicTFETool("list_workspaces", tfeTools.ListWorkspaces)
	r.mcpServer.AddTool(ListWorkspacesTool.Tool, ListWorkspacesTool.Handler)

	getWorkspaceDetailsTool := r.createDynamicTFETool("get_workspace_details", tfeTools.GetWorkspaceDetails)
	r.mcpServer.AddTool(getWorkspaceDetailsTool.Tool, getWorkspaceDetailsTool.Handler)

	createWorkspaceTool := r.createDynamicTFETool("create_workspace", tfeTools.CreateWorkspace)
	r.mcpServer.AddTool(createWorkspaceTool.Tool, createWorkspaceTool.Handler)

	updateWorkspaceTool := r.createDynamicTFETool("update_workspace", tfeTools.UpdateWorkspace)
	r.mcpServer.AddTool(updateWorkspaceTool.Tool, updateWorkspaceTool.Handler)

	deleteWorkspaceSafelyTool := r.createDynamicTFETool("delete_workspace_safely", tfeTools.DeleteWorkspaceSafely)
	r.mcpServer.AddTool(deleteWorkspaceSafelyTool.Tool, deleteWorkspaceSafelyTool.Handler)

	// Private provider tools
	searchPrivateProvidersTool := r.createDynamicTFETool("search_private_providers", tfeTools.SearchPrivateProviders)
	r.mcpServer.AddTool(searchPrivateProvidersTool.Tool, searchPrivateProvidersTool.Handler)

	getPrivateProviderDetailsTool := r.createDynamicTFETool("get_private_provider_details", tfeTools.GetPrivateProviderDetails)
	r.mcpServer.AddTool(getPrivateProviderDetailsTool.Tool, getPrivateProviderDetailsTool.Handler)

	// Private module tools
	searchPrivateModulesTool := r.createDynamicTFETool("search_private_modules", tfeTools.SearchPrivateModules)
	r.mcpServer.AddTool(searchPrivateModulesTool.Tool, searchPrivateModulesTool.Handler)

	getPrivateModuleDetailsTool := r.createDynamicTFETool("get_private_module_details", tfeTools.GetPrivateModuleDetails)
	r.mcpServer.AddTool(getPrivateModuleDetailsTool.Tool, getPrivateModuleDetailsTool.Handler)

	// Terraform run tools
	listRunsTool := r.createDynamicTFETool("list_runs", tfeTools.ListRuns)
	r.mcpServer.AddTool(listRunsTool.Tool, listRunsTool.Handler)

	createRunTool := r.createDynamicTFETool("create_run", tfeTools.CreateRun)
	r.mcpServer.AddTool(createRunTool.Tool, createRunTool.Handler)

	actionRunTool := r.createDynamicTFETool("action_run", tfeTools.ActionRun)
	r.mcpServer.AddTool(actionRunTool.Tool, actionRunTool.Handler)

	getRunDetailsTool := r.createDynamicTFETool("get_run_details", tfeTools.GetRunDetails)
	r.mcpServer.AddTool(getRunDetailsTool.Tool, getRunDetailsTool.Handler)

	r.tfeToolsRegistered = true
}

// createDynamicTFETool creates a TFE tool with dynamic availability checking
func (r *DynamicToolRegistry) createDynamicTFETool(toolName string, toolFactory func(*log.Logger) server.ServerTool) server.ServerTool {
	originalTool := toolFactory(r.logger)

	// Wrap the handler with dynamic availability checking
	wrappedHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Get session from context
		session := server.ClientSessionFromContext(ctx)
		if session == nil {
			r.logger.WithField("tool", toolName).Warn("TFE tool called without session context")
			return mcp.NewToolResultError("This tool requires an active session with valid Terraform Cloud/Enterprise configuration."), nil
		}

		// Check if this session has a valid TFE client
		sessionID := session.SessionID()
		if !r.HasSessionWithTFE(sessionID) {
			// Double-check by looking at the actual client state
			tfeClient := client.GetTfeClient(sessionID)
			if tfeClient == nil {
				r.logger.WithFields(log.Fields{
					"tool": toolName,
				}).Warn("TFE tool called but session has no valid TFE client")

				return mcp.NewToolResultError("This tool is not available. This tool requires a valid Terraform Cloud/Enterprise token and configuration. Please ensure TFE_TOKEN and TFE_ADDRESS environment variables are properly set."), nil
			}
			// If we found a valid client that wasn't registered, register it now
			r.RegisterSessionWithTFE(sessionID)
		}

		// Tool is available, proceed with original handler
		return originalTool.Handler(ctx, req)
	}

	return server.ServerTool{
		Tool:    originalTool.Tool,
		Handler: wrappedHandler,
	}
}
