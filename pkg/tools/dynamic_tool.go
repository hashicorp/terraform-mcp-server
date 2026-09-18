// Copyright IBM Corp. 2025
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
	filter             toolsets.ToolFilter
}

var globalToolRegistry *DynamicToolRegistry

// registerDynamicTools registers the global tool registry
func registerDynamicTools(mcpServer *server.MCPServer, logger *log.Logger, filter toolsets.ToolFilter) {
	globalToolRegistry = &DynamicToolRegistry{
		sessionsWithTFE:    make(map[string]bool),
		tfeToolsRegistered: false,
		mcpServer:          mcpServer,
		logger:             logger,
		filter:             filter,
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

	tfOpsEnabled := isTerraformOperationsEnabled()

	for _, td := range toolsets.AllTools {
		if !td.RequiresTFE || !r.filter.IsToolEnabled(td.Name) {
			continue
		}
		if td.RequiresTFOps && !tfOpsEnabled {
			continue
		}

		switch td.Name {
		case "create_no_code_workspace":
			// Needs *server.MCPServer too, for elicitation
			tool := r.createDynamicTFEToolWithElicitation(td.Name, tfeTools.CreateNoCodeWorkspace)
			r.mcpServer.AddTool(tool.Tool, tool.Handler)
			continue
		case "create_run":
			// create_run is always registered when its toolset is enabled. Unlike the
			// other RequiresTFOps tools, ENABLE_TF_OPERATIONS doesn't hide it — it just
			// swaps which factory (safe vs. full) backs it
			factory := tfeTools.CreateRunSafe
			if tfOpsEnabled {
				factory = tfeTools.CreateRun
			}
			tool := r.createDynamicTFETool(td.Name, factory)
			r.mcpServer.AddTool(tool.Tool, tool.Handler)
			continue
		}
		factory, ok := toolFactories[td.Name]
		if !ok {
			r.logger.Warnf("no tool factory registered for %q; skipping", td.Name)
			continue
		}
		tool := r.createDynamicTFETool(td.Name, factory)
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
