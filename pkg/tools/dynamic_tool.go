// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
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

	for _, td := range r.filter.EnabledTools(func(td toolsets.ToolDef) bool { return td.RequiresTFE }) {
		if td.RequiresTFOps && !tfOpsEnabled {
			continue
		}

		var built server.ServerTool
		if special, ok := specialFactories[td.Name]; ok {
			built = special(r.logger, r.mcpServer, tfOpsEnabled)
		} else if factory, ok := toolFactories[td.Name]; ok {
			built = factory(r.logger)
		} else {
			r.logger.Warnf("no tool factory registered for %q; skipping", td.Name)
			continue
		}

		r.mcpServer.AddTool(built.Tool, r.wrapWithAvailabilityCheck(td.Name, built.Handler))
	}

	r.tfeToolsRegistered = true
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
