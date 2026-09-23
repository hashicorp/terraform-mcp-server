// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetPrivateProviderDetailsArguments identifies a private provider and controls version details.
type GetPrivateProviderDetailsArguments struct {
	TerraformOrgName         string `json:"terraform_org_name"`
	PrivateProviderNamespace string `json:"private_provider_namespace"`
	PrivateProviderName      string `json:"private_provider_name"`
	RegistryName             string `json:"registry_name,omitempty"`
	IncludeVersions          *bool  `json:"include_versions,omitempty"`
}

// GetPrivateProviderDetailsTool describes the get_private_provider_details tool.
func GetPrivateProviderDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_private_provider_details",
		Description: "This tool retrieves information about a specific private provider in your Terraform Cloud/Enterprise organization. It provides details on how to use the provider, permissions, available versions, and more. This tool requires a valid Terraform token to be configured.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The Terraform organization name",
				},
				"private_provider_namespace": {
					Type:        "string",
					Description: "The namespace of the private provider. For the public registry, use its public Terraform Registry namespace",
				},
				"private_provider_name": {
					Type:        "string",
					Description: "The name of the private provider",
				},
				"registry_name": {
					Type:        "string",
					Description: "The Terraform registry containing the provider",
					Enum:        enumOf(validRegistries...),
					Default:     json.RawMessage(`"private"`),
				},
				"include_versions": {
					Type:        "boolean",
					Description: "Whether to include detailed version information",
					Default:     json.RawMessage("true"),
				},
			},
			Required: []string{
				"terraform_org_name",
				"private_provider_namespace",
				"private_provider_name",
			},
			PropertyOrder: []string{
				"terraform_org_name",
				"private_provider_namespace",
				"private_provider_name",
				"registry_name",
				"include_versions",
			},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a private provider",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// GetPrivateProviderDetailsFunc retrieves provider details using the caller's Terraform session.
func GetPrivateProviderDetailsFunc(
	ctx context.Context,
	request *mcp.CallToolRequest,
	input GetPrivateProviderDetailsArguments,
) (*mcp.CallToolResult, any, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	providerNamespace := strings.TrimSpace(input.PrivateProviderNamespace)
	if providerNamespace == "" {
		return nil, nil, fmt.Errorf("private_provider_namespace must not be blank")
	}

	providerName := strings.TrimSpace(input.PrivateProviderName)
	if providerName == "" {
		return nil, nil, fmt.Errorf("private_provider_name must not be blank")
	}

	registryName := strings.TrimSpace(input.RegistryName)
	if registryName == "" {
		registryName = privateRegistry
	}
	if !slices.Contains(validRegistries, registryName) {
		return nil, nil, fmt.Errorf("registry_name must be one of %q", validRegistries)
	}

	includeVersions := true
	if input.IncludeVersions != nil {
		includeVersions = *input.IncludeVersions
	}

	readOptions := &tfe.RegistryProviderReadOptions{}
	if includeVersions {
		readOptions.Include = []tfe.RegistryProviderIncludeOps{tfe.RegistryProviderVersionsInclude}
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	providerID := tfe.RegistryProviderID{
		OrganizationName: terraformOrgName,
		Namespace:        providerNamespace,
		Name:             providerName,
		RegistryName:     tfe.RegistryName(registryName),
	}
	provider, err := tfeClient.RegistryProviders.Read(ctx, providerID, readOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("getting private provider %q in organization %q: %w", providerNamespace+"/"+providerName, terraformOrgName, err)
	}

	return textResult(privateProviderDetailsText(provider, includeVersions)), nil, nil
}

func privateProviderDetailsText(provider *tfe.RegistryProvider, includeVersions bool) string {
	versions := make([]*tfe.RegistryProviderVersion, 0, len(provider.RegistryProviderVersions))
	for _, version := range provider.RegistryProviderVersions {
		if version != nil {
			versions = append(versions, version)
		}
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
	if len(versions) > 0 {
		builder.WriteString(fmt.Sprintf("      version = \"%s\"\n", versions[0].Version))
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
	builder.WriteString(fmt.Sprintf("- Can Delete: %t\n\n", provider.Permissions.CanDelete))

	if includeVersions && len(versions) > 0 {
		builder.WriteString(fmt.Sprintf("Available Versions (%d):\n", len(versions)))
		for i, version := range versions {
			builder.WriteString(fmt.Sprintf("%d. Version: %s\n", i+1, version.Version))
			builder.WriteString(fmt.Sprintf("   ID: %s\n", version.ID))
			builder.WriteString(fmt.Sprintf("   Created: %s\n", version.CreatedAt))
			builder.WriteString(fmt.Sprintf("   Updated: %s\n", version.UpdatedAt))

			if version.KeyID != "" {
				builder.WriteString(fmt.Sprintf("   Key ID: %s\n", version.KeyID))
			}

			permissions := make([]string, 0, 2)
			if version.Permissions.CanUploadAsset {
				permissions = append(permissions, "upload-asset")
			}
			if version.Permissions.CanDelete {
				permissions = append(permissions, "delete")
			}
			builder.WriteString(fmt.Sprintf("   Permissions: %s\n", strings.Join(permissions, ", ")))

			platforms := make([]string, 0, len(version.RegistryProviderPlatforms))
			for _, platform := range version.RegistryProviderPlatforms {
				if platform != nil {
					platforms = append(platforms, platform.OS+"/"+platform.Arch)
				}
			}
			if len(platforms) > 0 {
				builder.WriteString(fmt.Sprintf("   Platforms: %s\n", strings.Join(platforms, ", ")))
			}

			builder.WriteString("\n")
		}
	} else if includeVersions {
		builder.WriteString("No version information is available for this provider.\n\n")
	}

	if len(provider.Links) > 0 {
		builder.WriteString("Links:\n")
		for key, value := range provider.Links {
			if stringValue, ok := value.(string); ok {
				builder.WriteString(fmt.Sprintf("- %s: %s\n", key, stringValue))
			}
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
