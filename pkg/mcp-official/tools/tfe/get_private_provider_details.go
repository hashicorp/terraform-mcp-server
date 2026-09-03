// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetPrivateProviderDetailsArguments holds the input parameters for getting private provider details.
type GetPrivateProviderDetailsArguments struct {
	// Required fields
	TerraformOrgName         string `json:"terraform_org_name" jsonschema:"The Terraform organization name"`
	PrivateProviderNamespace string `json:"private_provider_namespace" jsonschema:"The namespace of the private provider in your Terraform Cloud/Enterprise organization. For public registry, use the namespace from the public Terraform registry."`
	PrivateProviderName      string `json:"private_provider_name" jsonschema:"The name of the private provider"`

	// Optional fields (will be zero values if not provided)
	RegistryName    string `json:"registry_name,omitempty" jsonschema:"The type of Terraform registry to search within Terraform Cloud/Enterprise ('private' or 'public'). Defaults to 'private'"`
	IncludeVersions *bool  `json:"include_versions,omitempty" jsonschema:"Whether to include detailed version information. Defaults to true"`
}

func GetPrivateProviderDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "get_private_provider_details",
		Description: `This tool retrieves information about a specific private provider in your Terraform Cloud/Enterprise organization.
It provides details on how to use the provider, permissions, available versions, and more. This tool requires a valid Terraform token to be configured.
`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a private provider",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetPrivateProviderDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPrivateProviderDetailsArguments) (*mcp.CallToolResult, *string, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	privateProviderNamespace := strings.TrimSpace(input.PrivateProviderNamespace)
	privateProviderName := strings.TrimSpace(input.PrivateProviderName)

	registryName := strings.TrimSpace(input.RegistryName)
	if registryName == "" {
		registryName = "private"
	}

	includeVersions := true
	if input.IncludeVersions != nil {
		includeVersions = *input.IncludeVersions
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, err
	}

	providerID := tfe.RegistryProviderID{
		OrganizationName: terraformOrgName,
		Namespace:        privateProviderNamespace,
		Name:             privateProviderName,
		RegistryName:     tfe.RegistryName(registryName),
	}

	readOptions := &tfe.RegistryProviderReadOptions{}
	if includeVersions {
		includeOpts := []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
		readOptions.Include = includeOpts
	}

	provider, err := tfeClient.RegistryProviders.Read(ctx, providerID, readOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("provider not found: %s/%s - use search_private_providers to find valid providers: %w", privateProviderNamespace, privateProviderName, err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Private Provider Details: %s/%s\n", provider.Namespace, provider.Name))
	builder.WriteString(strings.Repeat("=", 50) + "\n\n")

	builder.WriteString("Usage:\n")
	builder.WriteString("To use this private provider in your Terraform configuration:\n\n")
	builder.WriteString("```hcl\n")
	builder.WriteString("terraform {\n")
	builder.WriteString("  required_providers {\n")
	builder.WriteString(fmt.Sprintf("    %s = {\n", provider.Name))
	builder.WriteString(fmt.Sprintf("      source = \"%s/%s\"\n", provider.Namespace, provider.Name))
	if len(provider.RegistryProviderVersions) > 0 {
		builder.WriteString(fmt.Sprintf("      version = \"%s\"\n", provider.RegistryProviderVersions[0].Version))
	}
	builder.WriteString("    }\n")
	builder.WriteString("  }\n")
	builder.WriteString("}\n")
	builder.WriteString("```\n")

	builder.WriteString("Basic Information:\n")
	builder.WriteString(fmt.Sprintf("- ID: %s\n", provider.ID))
	builder.WriteString(fmt.Sprintf("- Name: %s\n", provider.Name))
	builder.WriteString(fmt.Sprintf("- Namespace: %s\n", provider.Namespace))
	builder.WriteString(fmt.Sprintf("- Registry: %s\n", provider.RegistryName))
	builder.WriteString(fmt.Sprintf("- Created: %s\n", provider.CreatedAt))
	builder.WriteString(fmt.Sprintf("- Updated: %s\n", provider.UpdatedAt))
	builder.WriteString("\n")

	if provider.Organization != nil {
		builder.WriteString("Organization:\n")
		builder.WriteString(fmt.Sprintf("- Name: %s\n", provider.Organization.Name))
		if provider.Organization.Email != "" {
			builder.WriteString(fmt.Sprintf("- Email: %s\n", provider.Organization.Email))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("Permissions:\n")
	builder.WriteString(fmt.Sprintf("- Can Delete: %t\n", provider.Permissions.CanDelete))
	builder.WriteString("\n")

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

	if len(provider.Links) > 0 {
		builder.WriteString("Links:\n")
		for key, value := range provider.Links {
			if strValue, ok := value.(string); ok {
				builder.WriteString(fmt.Sprintf("- %s: %s\n", key, strValue))
			}
		}
		builder.WriteString("\n")
	}

	result := builder.String()
	return nil, &result, nil
}
