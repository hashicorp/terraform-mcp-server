// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetPrivateModuleDetails creates a tool to get detailed information about a private module.
func GetPrivateModuleDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_private_module_details",
			mcp.WithDescription(`This tool retrieves detailed information about a specific private module in your Terraform Cloud/Enterprise organization.
It provides comprehensive details including inputs, outputs, dependencies, versions, and usage examples. The private_module_id format is 'module-namespace/module-name/module-provider-name'.
This can be obtained by calling 'search_private_modules' first to obtain the exact private_module_id required to use this tool. This tool requires a valid Terraform token to be configured.`),
			mcp.WithTitleAnnotation("Get detailed information about a private module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("private_module_id",
				mcp.Required(),
				mcp.Description(`The private module ID should be in the format 'module-namespace/module-name/module-provider-name' (for example, 'my-tfc-org/vpc/aws' or 'my-module-namespace/vm/azurerm').
The module-namespace is usually the name of the Terraform organization. Obtain this ID by calling 'search_private_modules'.`),
			),
			mcp.WithString("registry_name",
				mcp.Description("The type of Terraform registry to search within Terraform Cloud/Enterprise (e.g., 'private', 'public')"),
				mcp.Enum("private", "public"),
				mcp.DefaultString("private"),
			),
			mcp.WithString("private_module_version",
				mcp.Description("Specific version of the module to retrieve details for. If not provided, the latest version will be used"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPrivateModuleDetailsHandler(ctx, request, logger)
		},
	}
}

func getPrivateModuleDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get Terraform org name
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required for the Terraform Cloud/Enterprise organization.", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	// Get Terraform module id
	moduleID, err := request.RequireString("private_module_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "private_module_id is required", err)
	}
	moduleID = strings.TrimSpace(moduleID)

	// Get optional parameters
	registryName := strings.TrimSpace(request.GetString("registry_name", "private"))
	moduleVersion := strings.TrimSpace(request.GetString("private_module_version", ""))

	// Get the terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		err = utils.LogAndReturnError(logger, "failed to get terraform client for TFE, ensure TFE_TOKEN and TFE_ADDRESS are properly set.", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to get terraform client for TFE: %v", err)), nil
	}

	// Split moduleID into org name, module name, and provider
	parts := strings.Split(moduleID, "/")
	if len(parts) != 3 {
		return mcp.NewToolResultError("private_module_id must be in the format 'module-namespace/module-name/module-provider-name'"), nil
	}
	// Create module ID for TFE API
	tfeModuleID := tfe.RegistryModuleID{
		Organization: terraformOrgName,
		Namespace:    parts[0],
		Name:         parts[1],
		Provider:     parts[2],
		RegistryName: tfe.RegistryName(registryName),
	}

	logger.WithFields(log.Fields{
		"terraform_org_name":     terraformOrgName,
		"private_module_id":      moduleID,
		"private_module_version": moduleVersion,
	}).Info("Getting private module details")

	// Call the TFE API to get module details
	var module *tfe.RegistryModule
	var terraformRegistryModule *tfe.TerraformRegistryModule

	// Get the parent module for additional details
	module, err = tfeClient.RegistryModules.Read(ctx, tfeModuleID)
	if err != nil {
		logger.WithError(err).Error("failed to read private module details")
		return mcp.NewToolResultError(fmt.Sprintf("failed to read private module details: %v", err)), nil
	}

	// Get detailed module information from Terraform Registry (specific version or latest), it'll automatically use the latest version with empty string
	terraformRegistryModule, err = tfeClient.RegistryModules.ReadTerraformRegistryModule(ctx, tfeModuleID, moduleVersion)
	if err != nil {
		logger.WithError(err).Warn("failed to get detailed module information from Terraform Registry, continuing with basic info")
	}

	return buildPrivateModuleDetailsResponse(module, terraformRegistryModule, tfeClient.BaseURL().Host, logger), nil
}

func buildPrivateModuleDetailsResponse(registryModule *tfe.RegistryModule,
	terraformRegistryModule *tfe.TerraformRegistryModule,
	tfeHostAddress string,
	logger *log.Logger) *mcp.CallToolResult {

	// Set the registry path for the module
	registryPath := path.Join(tfeHostAddress, registryModule.Namespace, registryModule.Name, registryModule.Provider)

	// Usage information
	var builder strings.Builder
	builder.WriteString("Usage:\n")
	builder.WriteString("To use this private module in your Terraform configuration:\n\n")
	builder.WriteString("```hcl\n")
	builder.WriteString(fmt.Sprintf("module \"%s\" {\n", registryModule.Name))
	builder.WriteString(fmt.Sprintf("  source = \"%s\"\n", registryPath))

	// Use specific version if provided, otherwise use latest available
	if len(registryModule.VersionStatuses) > 0 {
		// Find the latest version from version statuses
		for _, versionStatus := range registryModule.VersionStatuses {
			builder.WriteString(fmt.Sprintf("  version = \"%s\"\n", versionStatus.Version))
			break // Use the first one as it's typically the latest
		}
	}

	builder.WriteString("\n")
	builder.WriteString("  # Add your module inputs here\n")
	builder.WriteString("}\n")
	builder.WriteString("```\n\n")

	// Basic information
	builder.WriteString("Basic Information:\n")
	builder.WriteString(fmt.Sprintf("- Name: %s\n", registryModule.Name))
	builder.WriteString(fmt.Sprintf("- Namespace: %s\n", registryModule.Namespace))
	builder.WriteString(fmt.Sprintf("- Provider: %s\n", registryModule.Provider))
	builder.WriteString(fmt.Sprintf("- Registry: %s\n", registryModule.RegistryName))
	builder.WriteString(fmt.Sprintf("- Created: %s\n", registryModule.CreatedAt))
	builder.WriteString(fmt.Sprintf("- Updated: %s\n", registryModule.UpdatedAt))
	builder.WriteString(fmt.Sprintf("- No Code Module: %t\n", registryModule.NoCode))

	// Add description if available from TerraformRegistryModule
	if terraformRegistryModule != nil && terraformRegistryModule.Description != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", terraformRegistryModule.Description))
	}
	builder.WriteString("\n")

	// Inputs section
	if terraformRegistryModule != nil && len(terraformRegistryModule.Root.Inputs) > 0 {
		builder.WriteString("Inputs:\n")
		builder.WriteString(strings.Repeat("-", 20) + "\n")
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|------|------|-------------|---------|----------|\n")
		for _, input := range terraformRegistryModule.Root.Inputs {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | `%s` | %t |\n",
				input.Name,
				input.Type,
				input.Description,
				input.Default,
				input.Required,
			))
		}
		builder.WriteString("\n")
	}

	// Outputs section
	if terraformRegistryModule != nil && len(terraformRegistryModule.Root.Outputs) > 0 {
		builder.WriteString("Outputs:\n")
		builder.WriteString(strings.Repeat("-", 20) + "\n")
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|------|-------------|\n")
		for _, output := range terraformRegistryModule.Root.Outputs {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				output.Name,
				output.Description,
			))
		}
		builder.WriteString("\n")
	}

	// Provider Dependencies section
	if terraformRegistryModule != nil && len(terraformRegistryModule.Root.ProviderDependencies) > 0 {
		builder.WriteString("Provider Dependencies:\n")
		builder.WriteString(strings.Repeat("-", 20) + "\n")
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|------|-----------|--------|----------|\n")
		for _, dep := range terraformRegistryModule.Root.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dep.Name,
				dep.Namespace,
				dep.Source,
				dep.Version,
			))
		}
		builder.WriteString("\n")
	}

	// Resources section
	if terraformRegistryModule != nil && len(terraformRegistryModule.Root.Resources) > 0 {
		builder.WriteString("Resources:\n")
		builder.WriteString(strings.Repeat("-", 20) + "\n")
		builder.WriteString("| Name | Type |\n")
		builder.WriteString("|------|------|\n")
		for _, resource := range terraformRegistryModule.Root.Resources {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				resource.Name,
				resource.Type,
			))
		}
		builder.WriteString("\n")
	}

	// Organization information
	if registryModule.Organization != nil {
		builder.WriteString("Organization:\n")
		builder.WriteString(fmt.Sprintf("- Name: %s\n", registryModule.Organization.Name))
		builder.WriteString("\n")
	}

	// Permissions
	if registryModule.Permissions != nil {
		builder.WriteString("Permissions:\n")
		builder.WriteString(fmt.Sprintf("- Can Delete: %t\n", registryModule.Permissions.CanDelete))
		builder.WriteString(fmt.Sprintf("- Can Resync: %t\n", registryModule.Permissions.CanResync))
		builder.WriteString(fmt.Sprintf("- Can Retry: %t\n", registryModule.Permissions.CanRetry))
		builder.WriteString("\n")
	}

	// VCS Repository information
	if registryModule.VCSRepo != nil {
		builder.WriteString("VCS Repository:\n")
		builder.WriteString(fmt.Sprintf("- Identifier: %s\n", registryModule.VCSRepo.Identifier))
		builder.WriteString(fmt.Sprintf("- Display Identifier: %s\n", registryModule.VCSRepo.DisplayIdentifier))
		builder.WriteString(fmt.Sprintf("- Branch: %s\n", registryModule.VCSRepo.Branch))
		if registryModule.VCSRepo.IngressSubmodules {
			builder.WriteString("- Ingress Submodules: Yes\n")
		}
		if registryModule.VCSRepo.RepositoryHTTPURL != "" {
			builder.WriteString(fmt.Sprintf("- Repository URL: %s\n", registryModule.VCSRepo.RepositoryHTTPURL))
		}
		if registryModule.VCSRepo.ServiceProvider != "" {
			builder.WriteString(fmt.Sprintf("- Service Provider: %s\n", registryModule.VCSRepo.ServiceProvider))
		}
		builder.WriteString("\n")
	}

	// README section
	if terraformRegistryModule != nil && terraformRegistryModule.Root.Readme != "" {
		cleanedReadme := removeReadmeSections(terraformRegistryModule.Root.Readme)
		builder.WriteString("README:\n")
		builder.WriteString(strings.Repeat("-", 20) + "\n")
		builder.WriteString(cleanedReadme)
	}

	logger.WithFields(log.Fields{
		"private_module_id":        registryModule.ID,
		"private_module_namespace": registryModule.Namespace,
		"private_module_name":      registryModule.Name,
		"private_module_provider":  registryModule.Provider,
		"versions_count":           len(registryModule.VersionStatuses),
		"has_detailed_info":        terraformRegistryModule != nil,
	}).Info("Successfully retrieved private module details")

	return mcp.NewToolResultText(builder.String())
}

// Manually remove README sections because it's already included in the response
func removeReadmeSections(readme string) string {
	// Split by lines and reconstruct, skipping sections we want to remove
	lines := strings.Split(readme, "\n")
	var result []string
	skipSection := false

	for _, line := range lines {
		// Check if this line is a section header we want to remove
		lowerLine := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lowerLine, "##") || strings.HasPrefix(lowerLine, "###") || strings.HasPrefix(lowerLine, "####") {
			// Check if it's one of the sections we want to remove
			if strings.Contains(lowerLine, "inputs") ||
				strings.Contains(lowerLine, "outputs") ||
				strings.Contains(lowerLine, "dependencies") ||
				strings.Contains(lowerLine, "provider dependencies") ||
				strings.Contains(lowerLine, "resources") {
				skipSection = true
				continue
			} else {
				// It's a different section header, stop skipping
				skipSection = false
			}
		}

		// If we're not skipping this section, add the line
		if !skipSection {
			result = append(result, line)
		}
	}

	cleaned := strings.Join(result, "\n")

	// Clean up multiple consecutive newlines that might result from section removal
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")

	return strings.TrimSpace(cleaned)
}
