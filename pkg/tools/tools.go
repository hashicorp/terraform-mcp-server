// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"net/http"

	"github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
	hcp_tools "github.com/hashicorp/terraform-mcp-server/pkg/tools/hcp_terraform"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {

	// Provider tools
	getResolveProviderDocIDTool := ResolveProviderDocID(registryClient, logger)
	hcServer.AddTool(getResolveProviderDocIDTool.Tool, getResolveProviderDocIDTool.Handler)

	getProviderDocsTool := GetProviderDocs(registryClient, logger)
	hcServer.AddTool(getProviderDocsTool.Tool, getProviderDocsTool.Handler)

	getLatestProviderVersionTool := GetLatestProviderVersion(registryClient, logger)
	hcServer.AddTool(getLatestProviderVersionTool.Tool, getLatestProviderVersionTool.Handler)

	// Module tools
	getSearchModulesTool := SearchModules(registryClient, logger)
	hcServer.AddTool(getSearchModulesTool.Tool, getSearchModulesTool.Handler)

	getModuleDetailsTool := ModuleDetails(registryClient, logger)
	hcServer.AddTool(getModuleDetailsTool.Tool, getModuleDetailsTool.Handler)

	getLatestModuleVersionTool := GetLatestModuleVersion(registryClient, logger)
	hcServer.AddTool(getLatestModuleVersionTool.Tool, getLatestModuleVersionTool.Handler)

	// Policy tools
	getSearchPoliciesTool := SearchPolicies(registryClient, logger)
	hcServer.AddTool(getSearchPoliciesTool.Tool, getSearchPoliciesTool.Handler)

	getPolicyDetailsTool := PolicyDetails(registryClient, logger)
	hcServer.AddTool(getPolicyDetailsTool.Tool, getPolicyDetailsTool.Handler)

	// HCP Terraform tools
	hcpClient := hcp_terraform.NewClient(logger)

	// Organizations tool
	getHCPOrganizationsTool := hcp_tools.GetOrganizations(hcpClient, logger)
	hcServer.AddTool(getHCPOrganizationsTool.Tool, getHCPOrganizationsTool.Handler)

	// Workspace tools (Phase 1: Core Workspace Tools)
	getHCPWorkspacesTool := hcp_tools.GetWorkspaces(hcpClient, logger)
	hcServer.AddTool(getHCPWorkspacesTool.Tool, getHCPWorkspacesTool.Handler)

	getHCPWorkspaceDetailsTool := hcp_tools.GetWorkspaceDetails(hcpClient, logger)
	hcServer.AddTool(getHCPWorkspaceDetailsTool.Tool, getHCPWorkspaceDetailsTool.Handler)

	createHCPWorkspaceTool := hcp_tools.CreateWorkspace(hcpClient, logger)
	hcServer.AddTool(createHCPWorkspaceTool.Tool, createHCPWorkspaceTool.Handler)

	updateHCPWorkspaceTool := hcp_tools.UpdateWorkspace(hcpClient, logger)
	hcServer.AddTool(updateHCPWorkspaceTool.Tool, updateHCPWorkspaceTool.Handler)

	// Variable tools (Phase 2: Variables Management)
	getHCPWorkspaceVariablesTool := hcp_tools.GetWorkspaceVariables(hcpClient, logger)
	hcServer.AddTool(getHCPWorkspaceVariablesTool.Tool, getHCPWorkspaceVariablesTool.Handler)

	createHCPWorkspaceVariableTool := hcp_tools.CreateWorkspaceVariable(hcpClient, logger)
	hcServer.AddTool(createHCPWorkspaceVariableTool.Tool, createHCPWorkspaceVariableTool.Handler)

	updateHCPWorkspaceVariableTool := hcp_tools.UpdateWorkspaceVariable(hcpClient, logger)
	hcServer.AddTool(updateHCPWorkspaceVariableTool.Tool, updateHCPWorkspaceVariableTool.Handler)

	deleteHCPWorkspaceVariableTool := hcp_tools.DeleteWorkspaceVariable(hcpClient, logger)
	hcServer.AddTool(deleteHCPWorkspaceVariableTool.Tool, deleteHCPWorkspaceVariableTool.Handler)

	bulkCreateHCPWorkspaceVariablesTool := hcp_tools.BulkCreateWorkspaceVariables(hcpClient, logger)
	hcServer.AddTool(bulkCreateHCPWorkspaceVariablesTool.Tool, bulkCreateHCPWorkspaceVariablesTool.Handler)

	// Configuration management tools (Phase 3: Configuration Management)
	getHCPConfigurationVersionsTool := hcp_tools.GetConfigurationVersions(hcpClient, logger)
	hcServer.AddTool(getHCPConfigurationVersionsTool.Tool, getHCPConfigurationVersionsTool.Handler)

	createHCPConfigurationVersionTool := hcp_tools.CreateConfigurationVersion(hcpClient, logger)
	hcServer.AddTool(createHCPConfigurationVersionTool.Tool, createHCPConfigurationVersionTool.Handler)

	downloadHCPConfigurationFilesTool := hcp_tools.DownloadConfigurationFiles(hcpClient, logger)
	hcServer.AddTool(downloadHCPConfigurationFilesTool.Tool, downloadHCPConfigurationFilesTool.Handler)

	uploadHCPConfigurationFilesTool := hcp_tools.UploadConfigurationFiles(hcpClient, logger)
	hcServer.AddTool(uploadHCPConfigurationFilesTool.Tool, uploadHCPConfigurationFilesTool.Handler)

	// Streaming configuration tools (NEW)
	createAndUploadConfigStreamingTool := hcp_tools.CreateAndUploadConfigurationWithStreaming(hcpClient, logger)
	hcServer.AddTool(createAndUploadConfigStreamingTool.Tool, createAndUploadConfigStreamingTool.Handler)

	// State management tools (Phase 4: State Management)
	getCurrentHCPStateVersionTool := hcp_tools.GetCurrentStateVersion(hcpClient, logger)
	hcServer.AddTool(getCurrentHCPStateVersionTool.Tool, getCurrentHCPStateVersionTool.Handler)

	downloadHCPStateVersionTool := hcp_tools.DownloadStateVersion(hcpClient, logger)
	hcServer.AddTool(downloadHCPStateVersionTool.Tool, downloadHCPStateVersionTool.Handler)

	createHCPStateVersionTool := hcp_tools.CreateStateVersion(hcpClient, logger)
	hcServer.AddTool(createHCPStateVersionTool.Tool, createHCPStateVersionTool.Handler)

	// Tag management tools (Phase 5: Supporting Tools - Tag Management)
	getHCPWorkspaceTagsTool := hcp_tools.GetWorkspaceTags(hcpClient, logger)
	hcServer.AddTool(getHCPWorkspaceTagsTool.Tool, getHCPWorkspaceTagsTool.Handler)

	createHCPWorkspaceTagBindingsTool := hcp_tools.CreateWorkspaceTagBindings(hcpClient, logger)
	hcServer.AddTool(createHCPWorkspaceTagBindingsTool.Tool, createHCPWorkspaceTagBindingsTool.Handler)

	updateHCPWorkspaceTagBindingsTool := hcp_tools.UpdateWorkspaceTagBindings(hcpClient, logger)
	hcServer.AddTool(updateHCPWorkspaceTagBindingsTool.Tool, updateHCPWorkspaceTagBindingsTool.Handler)

	deleteHCPWorkspaceTagsTool := hcp_tools.DeleteWorkspaceTags(hcpClient, logger)
	hcServer.AddTool(deleteHCPWorkspaceTagsTool.Tool, deleteHCPWorkspaceTagsTool.Handler)

	// Workspace locking tools (Phase 5: Supporting Tools - Workspace Locking)
	lockHCPWorkspaceTool := hcp_tools.LockWorkspace(hcpClient, logger)
	hcServer.AddTool(lockHCPWorkspaceTool.Tool, lockHCPWorkspaceTool.Handler)

	unlockHCPWorkspaceTool := hcp_tools.UnlockWorkspace(hcpClient, logger)
	hcServer.AddTool(unlockHCPWorkspaceTool.Tool, unlockHCPWorkspaceTool.Handler)

	// Remote state consumer tools (Phase 5: Supporting Tools - Remote State Consumers)
	getHCPRemoteStateConsumersTool := hcp_tools.GetRemoteStateConsumers(hcpClient, logger)
	hcServer.AddTool(getHCPRemoteStateConsumersTool.Tool, getHCPRemoteStateConsumersTool.Handler)

	addHCPRemoteStateConsumersTool := hcp_tools.AddRemoteStateConsumers(hcpClient, logger)
	hcServer.AddTool(addHCPRemoteStateConsumersTool.Tool, addHCPRemoteStateConsumersTool.Handler)

	removeHCPRemoteStateConsumersTool := hcp_tools.RemoveRemoteStateConsumers(hcpClient, logger)
	hcServer.AddTool(removeHCPRemoteStateConsumersTool.Tool, removeHCPRemoteStateConsumersTool.Handler)

	// Run management tools (Phase 6: Run Management)
	createHCPRunTool := hcp_tools.CreateRun(hcpClient, logger)
	hcServer.AddTool(createHCPRunTool.Tool, createHCPRunTool.Handler)

	getHCPRunTool := hcp_tools.GetRun(hcpClient, logger)
	hcServer.AddTool(getHCPRunTool.Tool, getHCPRunTool.Handler)

	listHCPRunsTool := hcp_tools.ListRuns(hcpClient, logger)
	hcServer.AddTool(listHCPRunsTool.Tool, listHCPRunsTool.Handler)

	// Run actions tools (Phase 6b: Run Actions)
	applyHCPRunTool := hcp_tools.ApplyRun(hcpClient, logger)
	hcServer.AddTool(applyHCPRunTool.Tool, applyHCPRunTool.Handler)

	discardHCPRunTool := hcp_tools.DiscardRun(hcpClient, logger)
	hcServer.AddTool(discardHCPRunTool.Tool, discardHCPRunTool.Handler)

	cancelHCPRunTool := hcp_tools.CancelRun(hcpClient, logger)
	hcServer.AddTool(cancelHCPRunTool.Tool, cancelHCPRunTool.Handler)

	// Plan/Apply details tools (Phase 6c: Plan/Apply Details)
	getHCPPlanTool := hcp_tools.GetPlan(hcpClient, logger)
	hcServer.AddTool(getHCPPlanTool.Tool, getHCPPlanTool.Handler)

	getHCPApplyTool := hcp_tools.GetApply(hcpClient, logger)
	hcServer.AddTool(getHCPApplyTool.Tool, getHCPApplyTool.Handler)

	logger.Infof("Initialized %d tools (including HCP Terraform tools)", 43) // Updated count: 8 registry tools + 35 HCP tools (including 1 new streaming tool)
}
