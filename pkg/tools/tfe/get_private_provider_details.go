// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// GetPrivateProviderDetails creates a tool to get detailed information about a private provider.
func GetPrivateProviderDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_private_provider_details",
			mcp.WithDescription(`This tool retrieves information about a specific private provider in your Terraform Cloud/Enterprise organization.
It provides details on how to use the provider, permissions, available versions, and more. This tool requires a valid Terraform token to be configured.
`),
			mcp.WithTitleAnnotation("Get detailed information about a private provider"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_org_name",
				mcp.Required(),
				mcp.Description("The Terraform Cloud/Enterprise organization name"),
			),
			mcp.WithString("private_provider_namespace",
				mcp.Required(),
				mcp.Description("The namespace of the private provider in your Terraform Cloud/Enterprise organization. For public registry, use the namespace from the public Terraform registry."),
			),
			mcp.WithString("private_provider_name",
				mcp.Required(),
				mcp.Description("The name of the private provider"),
			),
			mcp.WithString("registry_name",
				mcp.Description("The type of Terraform registry to search within Terraform Cloud/Enterprise (e.g., 'private', 'public')"),
				mcp.Enum("private", "public"),
				mcp.DefaultString("private"),
			),
			mcp.WithBoolean("include_versions",
				mcp.Description("Whether to include detailed version information"),
				mcp.DefaultBool(true),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPrivateProviderDetailsHandler(ctx, request, logger)
		},
	}
}

func getPrivateProviderDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// Get Terraform organization name
	terraformOrgName, err := request.RequireString("terraform_org_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "The 'terraform_org_name' parameter is required for the Terraform Cloud/Enterprise organization.", err)
	}
	terraformOrgName = strings.TrimSpace(terraformOrgName)

	// Get Terraform provider namespace
	privateProviderNamespace, err := request.RequireString("private_provider_namespace")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "private_provider_namespace is required", err)
	}
	privateProviderNamespace = strings.TrimSpace(privateProviderNamespace)

	// Get Terraform provider name
	privateProviderName, err := request.RequireString("private_provider_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "private_provider_name is required", err)
	}
	privateProviderName = strings.TrimSpace(privateProviderName)

	// Get optional parameters
	registryName := strings.TrimSpace(request.GetString("registry_name", "private"))
	includeVersions := request.GetBool("include_versions", true)

	// Get the terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		err = utils.LogAndReturnError(logger, "failed to get terraform client for TFE, ensure TFE_TOKEN and TFE_ADDRESS are properly set.", err)
		return mcp.NewToolResultError(fmt.Sprintf("failed to get terraform client for TFE: %v", err)), nil
	}

	// Create provider ID
	providerID := tfe.RegistryProviderID{
		OrganizationName: terraformOrgName,
		Namespace:        privateProviderNamespace,
		Name:             privateProviderName,
		RegistryName:     tfe.RegistryName(registryName),
	}

	// Prepare read options
	readOptions := &tfe.RegistryProviderReadOptions{}
	if includeVersions {
		includeOpts := []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
		readOptions.Include = includeOpts
	}

	logger.WithFields(log.Fields{
		"terraform_org_name":         terraformOrgName,
		"private_provider_namespace": privateProviderNamespace,
		"private_provider_name":      privateProviderName,
		"registry_name":              registryName,
		"include_versions":           includeVersions,
	}).Info("Getting private provider details")

	// Call the TFE API to get provider details
	provider, err := tfeClient.RegistryProviders.Read(ctx, providerID, readOptions)
	if err != nil {
		logger.WithError(err).Error("failed to get private provider details")
		return mcp.NewToolResultError(fmt.Sprintf("failed to get private provider details: %v", err)), nil
	}

	// Build response
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Private Provider Details: %s/%s\n", provider.Namespace, provider.Name))
	builder.WriteString(strings.Repeat("=", 50) + "\n\n")

	// Usage information
	builder.WriteString("Usage:\n")
	builder.WriteString("To use this private provider in your Terraform configuration:\n\n")
	builder.WriteString("```hcl\n")
	builder.WriteString("terraform {\n")
	builder.WriteString("  required_providers {\n")
	builder.WriteString(fmt.Sprintf("    %s = {\n", provider.Name))
	builder.WriteString(fmt.Sprintf("      source = \"%s/%s\"\n", provider.Namespace, provider.Name))
	if len(provider.RegistryProviderVersions) > 0 {
		// Use the first (presumably latest) version
		builder.WriteString(fmt.Sprintf("      version = \"%s\"\n", provider.RegistryProviderVersions[0].Version))
	}
	builder.WriteString("    }\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	builder.WriteString("```\n")

	// Basic information
	builder.WriteString("Basic Information:\n")
	builder.WriteString(fmt.Sprintf("- ID: %s\n", provider.ID))
	builder.WriteString(fmt.Sprintf("- Name: %s\n", provider.Name))
	builder.WriteString(fmt.Sprintf("- Namespace: %s\n", provider.Namespace))
	builder.WriteString(fmt.Sprintf("- Registry: %s\n", provider.RegistryName))
	builder.WriteString(fmt.Sprintf("- Created: %s\n", provider.CreatedAt))
	builder.WriteString(fmt.Sprintf("- Updated: %s\n", provider.UpdatedAt))
	builder.WriteString("\n")

	// Organization information
	if provider.Organization != nil {
		builder.WriteString("Organization:\n")
		builder.WriteString(fmt.Sprintf("- Name: %s\n", provider.Organization.Name))
		if provider.Organization.Email != "" {
			builder.WriteString(fmt.Sprintf("- Email: %s\n", provider.Organization.Email))
		}
		builder.WriteString("\n")
	}

	// Permissions
	builder.WriteString("Permissions:\n")
	builder.WriteString(fmt.Sprintf("- Can Delete: %t\n", provider.Permissions.CanDelete))
	builder.WriteString("\n")

	// Versions information
	if includeVersions && len(provider.RegistryProviderVersions) > 0 {
		builder.WriteString(fmt.Sprintf("Available Versions (%d):\n", len(provider.RegistryProviderVersions)))

		for i, version := range provider.RegistryProviderVersions {
			builder.WriteString(fmt.Sprintf("%d. Version: %s\n", i+1, version.Version))
			builder.WriteString(fmt.Sprintf("   ID: %s\n", version.ID))
			builder.WriteString(fmt.Sprintf("   Created: %s\n", version.CreatedAt))
			builder.WriteString(fmt.Sprintf("   Updated: %s\n", version.UpdatedAt))

			if version.KeyID != "" {
				builder.WriteString(fmt.Sprintf("   Key ID: %s\n", version.KeyID))
			}

			// Show permissions for this version
			builder.WriteString("   Permissions: ")
			var perms []string
			if version.Permissions.CanUploadAsset {
				perms = append(perms, "upload-asset")
			}
			if version.Permissions.CanDelete {
				perms = append(perms, "delete")
			}
			builder.WriteString(strings.Join(perms, ", "))
			builder.WriteString("\n")

			// Show platforms if available
			if len(version.RegistryProviderPlatforms) > 0 {
				builder.WriteString("   Platforms: ")
				var platforms []string
				for _, platform := range version.RegistryProviderPlatforms {
					platforms = append(platforms, fmt.Sprintf("%s/%s", platform.OS, platform.Arch))
				}
				builder.WriteString(strings.Join(platforms, ", "))
				builder.WriteString("\n")
			}

			builder.WriteString("\n")
		}
	} else if includeVersions {
		builder.WriteString("No version information is available for this provider.\n\n")
	}

	// Links information
	if len(provider.Links) > 0 {
		builder.WriteString("Links:\n")
		for key, value := range provider.Links {
			if strValue, ok := value.(string); ok {
				builder.WriteString(fmt.Sprintf("- %s: %s\n", key, strValue))
			}
		}
		builder.WriteString("\n")
	}

	logger.WithFields(log.Fields{
		"terraform_org_name":         terraformOrgName,
		"private_provider_namespace": privateProviderNamespace,
		"private_provider_name":      privateProviderName,
		"versions_count":             len(provider.RegistryProviderVersions),
	}).Info("Successfully retrieved private provider details")

	return mcp.NewToolResultText(builder.String()), nil
}
