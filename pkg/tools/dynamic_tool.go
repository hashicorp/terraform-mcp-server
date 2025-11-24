// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	tfeTools "github.com/hashicorp/terraform-mcp-server/pkg/tools/tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/toolsets"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
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
	enabledToolsets    []string
}

var globalToolRegistry *DynamicToolRegistry

// registerDynamicTools registers the global tool registry
func registerDynamicTools(mcpServer *server.MCPServer, logger *log.Logger, enabledToolsets []string) {
	globalToolRegistry = &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          mcpServer,
		logger:             logger,
		enabledToolsets:    enabledToolsets,
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

// isTerraformOperationsEnabled checks if ENABLE_TF_OPERATIONS is set to true
func isTerraformOperationsEnabled() bool {
	envVar := utils.GetEnv("ENABLE_TF_OPERATIONS", "false")
	return strings.ToLower(envVar) == "true"
}

// registerTFETools registers TFE tools with the MCP server
func (r *DynamicToolRegistry) registerTFETools() {
	if r.tfeToolsRegistered {
		return
	}

	r.logger.Info("Registering TFE tools - first session with valid TFE client detected")

	// Terraform toolset - Organization and Project tools
	if toolsets.IsToolEnabled("list_terraform_orgs", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_terraform_orgs", tfeTools.ListTerraformOrgs)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("list_terraform_projects", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_terraform_projects", tfeTools.ListTerraformProjects)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Terraform toolset - Workspace management tools
	if toolsets.IsToolEnabled("list_workspaces", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_workspaces", tfeTools.ListWorkspaces)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_workspace_details", r.enabledToolsets) {
		tool := r.createDynamicTFETool("get_workspace_details", tfeTools.GetWorkspaceDetails)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("create_workspace", r.enabledToolsets) {
		tool := r.createDynamicTFETool("create_workspace", tfeTools.CreateWorkspace)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("update_workspace", r.enabledToolsets) {
		tool := r.createDynamicTFETool("update_workspace", tfeTools.UpdateWorkspace)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Only register delete_workspace_safely if TF operations are enabled AND toolset is enabled
	if isTerraformOperationsEnabled() && toolsets.IsToolEnabled("delete_workspace_safely", r.enabledToolsets) {
		tool := r.createDynamicTFETool("delete_workspace_safely", tfeTools.DeleteWorkspaceSafely)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Registry-private toolset - Private provider tools
	if toolsets.IsToolEnabled("search_private_providers", r.enabledToolsets) {
		tool := r.createDynamicTFETool("search_private_providers", tfeTools.SearchPrivateProviders)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_private_provider_details", r.enabledToolsets) {
		tool := r.createDynamicTFETool("get_private_provider_details", tfeTools.GetPrivateProviderDetails)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Registry-private toolset - Private module tools
	if toolsets.IsToolEnabled("search_private_modules", r.enabledToolsets) {
		tool := r.createDynamicTFETool("search_private_modules", tfeTools.SearchPrivateModules)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_private_module_details", r.enabledToolsets) {
		tool := r.createDynamicTFETool("get_private_module_details", tfeTools.GetPrivateModuleDetails)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Terraform toolset - Workspace tags tools
	if toolsets.IsToolEnabled("create_workspace_tags", r.enabledToolsets) {
		tool := r.createDynamicTFETool("create_workspace_tags", tfeTools.CreateWorkspaceTags)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("read_workspace_tags", r.enabledToolsets) {
		tool := r.createDynamicTFETool("read_workspace_tags", tfeTools.ReadWorkspaceTags)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Terraform toolset - Run tools
	if toolsets.IsToolEnabled("list_runs", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_runs", tfeTools.ListRuns)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Create run tool with conditional options based on TF operations setting
	if toolsets.IsToolEnabled("create_run", r.enabledToolsets) {
		var tool server.ServerTool
		if isTerraformOperationsEnabled() {
			tool = r.createDynamicTFETool("create_run", tfeTools.CreateRun)
		} else {
			tool = r.createDynamicTFETool("create_run", tfeTools.CreateRunSafe)
		}
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Only register action_run if TF operations are enabled AND toolset is enabled
	if isTerraformOperationsEnabled() && toolsets.IsToolEnabled("action_run", r.enabledToolsets) {
		tool := r.createDynamicTFETool("action_run", tfeTools.ActionRun)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("create_no_code_workspace", r.enabledToolsets) {
		tool := r.createDynamicTFEToolWithElicitation("create_no_code_workspace", tfeTools.CreateNoCodeWorkspace)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("get_run_details", r.enabledToolsets) {
		tool := r.createDynamicTFETool("get_run_details", tfeTools.GetRunDetails)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Terraform toolset - Variable set tools
	if toolsets.IsToolEnabled("list_variable_sets", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_variable_sets", tfeTools.ListVariableSets)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("create_variable_set", r.enabledToolsets) {
		tool := r.createDynamicTFETool("create_variable_set", tfeTools.CreateVariableSet)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("create_variable_in_variable_set", r.enabledToolsets) {
		tool := r.createDynamicTFETool("create_variable_in_variable_set", tfeTools.CreateVariableInVariableSet)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("delete_variable_in_variable_set", r.enabledToolsets) {
		tool := r.createDynamicTFETool("delete_variable_in_variable_set", tfeTools.DeleteVariableInVariableSet)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Attach/detach variable sets to/from workspaces
	if toolsets.IsToolEnabled("attach_variable_set_to_workspaces", r.enabledToolsets) {
		tool := r.createDynamicTFETool("attach_variable_set_to_workspaces", tfeTools.AttachVariableSetToWorkspaces)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("detach_variable_set_from_workspaces", r.enabledToolsets) {
		tool := r.createDynamicTFETool("detach_variable_set_from_workspaces", tfeTools.DetachVariableSetFromWorkspaces)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	// Terraform toolset - Variable tools
	if toolsets.IsToolEnabled("list_workspace_variables", r.enabledToolsets) {
		tool := r.createDynamicTFETool("list_workspace_variables", tfeTools.ListWorkspaceVariables)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("create_workspace_variable", r.enabledToolsets) {
		tool := r.createDynamicTFETool("create_workspace_variable", tfeTools.CreateWorkspaceVariable)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	if toolsets.IsToolEnabled("update_workspace_variable", r.enabledToolsets) {
		tool := r.createDynamicTFETool("update_workspace_variable", tfeTools.UpdateWorkspaceVariable)
		r.mcpServer.AddTool(tool.Tool, tool.Handler)
	}

	r.tfeToolsRegistered = true
}

// createDynamicTFETool creates a TFE tool with dynamic availability checking
func (r *DynamicToolRegistry) createDynamicTFETool(toolName string, toolFactory func(*log.Logger) server.ServerTool) server.ServerTool {
	originalTool := toolFactory(r.logger)
	return server.ServerTool{
		Tool:    originalTool.Tool,
		Handler: r.wrapWithAvailabilityCheck(toolName, originalTool.Handler),
	}
}

// createDynamicTFEToolWithElicitation creates a TFE tool with dynamic availability checking that also needs MCPServer for elicitation
func (r *DynamicToolRegistry) createDynamicTFEToolWithElicitation(toolName string, toolFactory func(*log.Logger, *server.MCPServer) server.ServerTool) server.ServerTool {
	originalTool := toolFactory(r.logger, r.mcpServer)
	return server.ServerTool{
		Tool:    originalTool.Tool,
		Handler: r.wrapWithAvailabilityCheck(toolName, originalTool.Handler),
	}
}

// wrapWithAvailabilityCheck wraps a tool handler with dynamic TFE availability checking
func (r *DynamicToolRegistry) wrapWithAvailabilityCheck(toolName string, originalHandler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		return originalHandler(ctx, req)
	}
}
