// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetPrivateModuleDetails creates a tool to get detailed information about a private module.
func GetPrivateModuleDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_private_module_details",
			mcp.WithDescription(`This tool retrieves detailed information about a specific private module in your Terraform Cloud/Enterprise organization. It provides comprehensive details including inputs, outputs, dependencies, versions, and usage examples. It also reports every submodule (nested module) published with the module, each with its own inputs, outputs, provider dependencies, resources, and README. The private_module_id format is 'module-namespace/module-name/module-provider-name'. This can be obtained by calling 'search_private_modules' first to obtain the exact private_module_id required to use this tool. This tool requires a valid Terraform token to be configured.`),
			mcp.WithTitleAnnotation("Get detailed information about a private module"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description(terraformOrgNameDescription),
			),
			mcp.WithString("private_module_id",
				mcp.Required(),
				mcp.Description(`The private module ID should be in the format 'module-namespace/module-name/module-provider-name' (for example, 'my-tfc-org/vpc/aws' or 'my-module-namespace/vm/azurerm'). The module-namespace is usually the name of the Terraform organization. Obtain this ID by calling 'search_private_modules'.`),
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
	terraformOrgName, err := RequireTrimmedString(request, "terraform_org_name")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_org_name", err)
	}
	moduleID, err := RequireTrimmedString(request, "private_module_id")
	if err != nil {
		return ToolError(logger, "missing required input: private_module_id", err)
	}

	registryName := GetTrimmedString(request, "registry_name", "private")
	moduleVersion := GetTrimmedString(request, "private_module_version", "")

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", err)
	}

	parts := strings.Split(moduleID, "/")
	if len(parts) != 3 {
		return ToolError(logger, "private_module_id must be in format 'module-namespace/module-name/module-provider-name'", nil)
	}

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

	var module *tfe.RegistryModule
	var terraformRegistryModule *client.TerraformModuleVersionDetails

	module, err = tfeClient.RegistryModules.Read(ctx, tfeModuleID)
	if err != nil {
		return ToolErrorf(logger, "module not found: %s - use search_private_modules to find valid module IDs", moduleID)
	}

	terraformRegistryModule, err = readTerraformRegistryModuleDetails(ctx, tfeClient, tfeModuleID, moduleVersion)
	if err != nil {
		logger.WithError(err).Warn("failed to get detailed module information from Terraform Registry, continuing with basic info")
	}

	return buildPrivateModuleDetailsResponse(module, terraformRegistryModule, tfeClient.BaseURL().Host, logger), nil
}

func readTerraformRegistryModuleDetails(ctx context.Context, tfeClient *tfe.Client, moduleID tfe.RegistryModuleID, moduleVersion string) (*client.TerraformModuleVersionDetails, error) {
	basePath := "/api/registry/v1/modules"
	if moduleID.RegistryName == tfe.PublicRegistry {
		basePath = "/api/registry/public/v1/modules"
	}

	u := fmt.Sprintf("%s/%s/%s/%s",
		basePath,
		url.PathEscape(moduleID.Namespace),
		url.PathEscape(moduleID.Name),
		url.PathEscape(moduleID.Provider),
	)

	// The registry uses /{namespace}/{name}/{provider} for the latest version
	// and appends /{version} when a specific version is requested.
	if moduleVersion != "" {
		u += "/" + url.PathEscape(moduleVersion)
	}

	req, err := tfeClient.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}

	module := &client.TerraformModuleVersionDetails{}
	if err := req.DoJSON(ctx, module); err != nil {
		return nil, err
	}

	return module, nil
}

func buildPrivateModuleDetailsResponse(registryModule *tfe.RegistryModule, terraformRegistryModule *client.TerraformModuleVersionDetails, tfeHostAddress string, logger *log.Logger) *mcp.CallToolResult {

	registryPath := path.Join(tfeHostAddress, registryModule.Namespace, registryModule.Name, registryModule.Provider)

	// Use the exact version represented by the details response so the generated
	// Terraform usage example matches the inputs, outputs, and submodules below.
	moduleVersion := ""
	if terraformRegistryModule != nil && terraformRegistryModule.Version != "" {
		moduleVersion = terraformRegistryModule.Version
	}

	var builder strings.Builder
	builder.WriteString("Usage:\n")
	builder.WriteString("To use this private module in your Terraform configuration:\n\n")
	builder.WriteString("```hcl\n")
	builder.WriteString(fmt.Sprintf("module %q {\n", registryModule.Name))
	builder.WriteString(fmt.Sprintf("  source = %q\n", registryPath))

	if moduleVersion != "" {
		builder.WriteString(fmt.Sprintf("  version = %q\n", moduleVersion))
	}

	builder.WriteString("\n")
	builder.WriteString("  # Add your module inputs here\n")
	builder.WriteString("}\n")
	builder.WriteString("```\n\n")

	builder.WriteString("Basic Information:\n")
	builder.WriteString(fmt.Sprintf("- Name: %s\n", registryModule.Name))
	builder.WriteString(fmt.Sprintf("- Namespace: %s\n", registryModule.Namespace))
	builder.WriteString(fmt.Sprintf("- Provider: %s\n", registryModule.Provider))
	builder.WriteString(fmt.Sprintf("- Registry: %s\n", registryModule.RegistryName))
	if moduleVersion != "" {
		builder.WriteString(fmt.Sprintf("- Version: %s\n", moduleVersion))
	}
	builder.WriteString(fmt.Sprintf("- Created: %s\n", registryModule.CreatedAt))
	builder.WriteString(fmt.Sprintf("- Updated: %s\n", registryModule.UpdatedAt))
	builder.WriteString(fmt.Sprintf("- No Code Module: %t\n", registryModule.NoCode))

	if terraformRegistryModule != nil && terraformRegistryModule.Description != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", terraformRegistryModule.Description))
	}
	builder.WriteString("\n")

	if terraformRegistryModule != nil {
		if modulePartHasDetails(terraformRegistryModule.Root) {
			builder.WriteString("Root Module:\n")
			writeModulePartDetails(&builder, terraformRegistryModule.Root)
			writeModulePartReadme(&builder, terraformRegistryModule.Root)
		}

		// Submodules are reported the same way as the root module so that each one
		// documents its own inputs, outputs, dependencies, resources, and README.
		for _, submodule := range terraformRegistryModule.Submodules {
			builder.WriteString(fmt.Sprintf("Submodule: %s\n", submodule.Name))
			builder.WriteString(fmt.Sprintf("- Path: %s\n\n", submodule.Path))
			writeModulePartDetails(&builder, submodule)
			writeModulePartReadme(&builder, submodule)
		}
	}

	if registryModule.Organization != nil {
		builder.WriteString("Organization:\n")
		builder.WriteString(fmt.Sprintf("- Name: %s\n", registryModule.Organization.Name))
		builder.WriteString("\n")
	}

	if registryModule.Permissions != nil {
		builder.WriteString("Permissions:\n")
		builder.WriteString(fmt.Sprintf("- Can Delete: %t\n", registryModule.Permissions.CanDelete))
		builder.WriteString(fmt.Sprintf("- Can Resync: %t\n", registryModule.Permissions.CanResync))
		builder.WriteString(fmt.Sprintf("- Can Retry: %t\n", registryModule.Permissions.CanRetry))
		builder.WriteString("\n")
	}

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

// modulePartHasDetails reports whether a module part has anything worth writing,
// so that a heading is never emitted for a part the registry reported as empty.
func modulePartHasDetails(modulePart client.ModulePart) bool {
	return len(modulePart.Inputs) > 0 ||
		len(modulePart.Outputs) > 0 ||
		len(modulePart.Dependencies) > 0 ||
		len(modulePart.ProviderDependencies) > 0 ||
		len(modulePart.Resources) > 0 ||
		modulePart.Readme != ""
}

func writeModulePartDetails(builder *strings.Builder, modulePart client.ModulePart) {
	if len(modulePart.Inputs) > 0 {
		builder.WriteString("Inputs:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|------|------|-------------|---------|----------|\n")
		for _, input := range modulePart.Inputs {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | `%s` | %t |\n",
				input.Name,
				input.Type,
				input.Description,
				formatModuleInputDefault(input.Default),
				input.Required,
			))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.Outputs) > 0 {
		builder.WriteString("Outputs:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|------|-------------|\n")
		for _, output := range modulePart.Outputs {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				output.Name,
				output.Description,
			))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.Dependencies) > 0 {
		builder.WriteString("Dependencies:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Source | Version |\n")
		builder.WriteString("|------|--------|---------|\n")
		for _, dep := range modulePart.Dependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
				dep.Name,
				dep.Source,
				dep.Version,
			))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.ProviderDependencies) > 0 {
		builder.WriteString("Provider Dependencies:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|------|-----------|--------|----------|\n")
		for _, dep := range modulePart.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dep.Name,
				dep.Namespace,
				dep.Source,
				dep.Version,
			))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.Resources) > 0 {
		builder.WriteString("Resources:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Type |\n")
		builder.WriteString("|------|------|\n")
		for _, resource := range modulePart.Resources {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n",
				resource.Name,
				resource.Type,
			))
		}
		builder.WriteString("\n")
	}
}

func formatModuleInputDefault(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func writeModulePartReadme(builder *strings.Builder, modulePart client.ModulePart) {
	if modulePart.Readme == "" {
		return
	}

	cleanedReadme := removeReadmeSections(modulePart.Readme)
	if cleanedReadme == "" {
		return
	}

	builder.WriteString("README:\n")
	builder.WriteString(strings.Repeat("-", 20))
	builder.WriteByte('\n')
	builder.WriteString(cleanedReadme)
	builder.WriteString("\n\n")
}

func removeReadmeSections(readme string) string {
	lines := strings.Split(readme, "\n")
	var result []string
	skipSection := false

	for _, line := range lines {
		lowerLine := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lowerLine, "##") || strings.HasPrefix(lowerLine, "###") || strings.HasPrefix(lowerLine, "####") {
			if strings.Contains(lowerLine, "inputs") ||
				strings.Contains(lowerLine, "outputs") ||
				strings.Contains(lowerLine, "dependencies") ||
				strings.Contains(lowerLine, "provider dependencies") ||
				strings.Contains(lowerLine, "resources") {
				skipSection = true
				continue
			} else {
				skipSection = false
			}
		}

		if !skipSection {
			result = append(result, line)
		}
	}

	cleaned := strings.Join(result, "\n")
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")

	return strings.TrimSpace(cleaned)
}
