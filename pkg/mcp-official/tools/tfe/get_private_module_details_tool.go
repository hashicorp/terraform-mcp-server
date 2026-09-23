// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GetPrivateModuleDetailsArguments identifies a private module and optional version.
type GetPrivateModuleDetailsArguments struct {
	TerraformOrgName     string `json:"terraform_org_name"`
	PrivateModuleID      string `json:"private_module_id"`
	RegistryName         string `json:"registry_name,omitempty"`
	PrivateModuleVersion string `json:"private_module_version,omitempty"`
}

// GetPrivateModuleDetailsTool describes the get_private_module_details tool.
func GetPrivateModuleDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_private_module_details",
		Description: "This tool retrieves detailed information about a specific private module in your Terraform Cloud/Enterprise organization. It provides inputs, outputs, dependencies, versions, usage examples, and every published submodule with its own inputs, outputs, provider dependencies, resources, and README. Obtain the required namespace/name/provider private_module_id by calling search_private_modules first. This tool requires a valid Terraform token to be configured.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_org_name": {
					Type:        "string",
					Description: "The Terraform organization name",
				},
				"private_module_id": {
					Type:        "string",
					Description: "The namespace/name/provider module ID returned by search_private_modules",
					Examples:    []any{"my-tfc-org/vpc/aws", "my-module-namespace/vm/azurerm"},
				},
				"registry_name": {
					Type:        "string",
					Description: "The Terraform registry containing the module",
					Enum:        enumOf(validRegistries...),
					Default:     json.RawMessage(`"private"`),
				},
				"private_module_version": {
					Type:        "string",
					Description: "Specific module version to retrieve. If omitted, the registry resolves the latest version",
				},
			},
			Required:      []string{"terraform_org_name", "private_module_id"},
			PropertyOrder: []string{"terraform_org_name", "private_module_id", "registry_name", "private_module_version"},
			AdditionalProperties: &jsonschema.Schema{
				Not: &jsonschema.Schema{},
			},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get detailed information about a private module",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
	}
}

// GetPrivateModuleDetailsFunc retrieves module details using the caller's Terraform session.
func GetPrivateModuleDetailsFunc(
	ctx context.Context,
	request *mcp.CallToolRequest,
	input GetPrivateModuleDetailsArguments,
) (*mcp.CallToolResult, any, error) {
	terraformOrgName := strings.TrimSpace(input.TerraformOrgName)
	if terraformOrgName == "" {
		return nil, nil, fmt.Errorf("terraform_org_name must not be blank")
	}

	moduleID := strings.TrimSpace(input.PrivateModuleID)
	if moduleID == "" {
		return nil, nil, fmt.Errorf("private_module_id must not be blank")
	}
	parts := strings.Split(moduleID, "/")
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("private_module_id must be in namespace/name/provider format")
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return nil, nil, fmt.Errorf("private_module_id must be in namespace/name/provider format")
		}
	}

	registryName := strings.TrimSpace(input.RegistryName)
	if registryName == "" {
		registryName = privateRegistry
	}
	if !slices.Contains(validRegistries, registryName) {
		return nil, nil, fmt.Errorf("registry_name must be one of %q", validRegistries)
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	tfeModuleID := tfe.RegistryModuleID{
		Organization: terraformOrgName,
		Namespace:    parts[0],
		Name:         parts[1],
		Provider:     parts[2],
		RegistryName: tfe.RegistryName(registryName),
	}
	module, err := tfeClient.RegistryModules.Read(ctx, tfeModuleID)
	if err != nil {
		return nil, nil, fmt.Errorf("getting private module %q in organization %q: %w", moduleID, terraformOrgName, err)
	}

	registryDetails, err := readTerraformRegistryModuleDetails(
		ctx,
		tfeClient,
		tfeModuleID,
		strings.TrimSpace(input.PrivateModuleVersion),
	)
	if err != nil {
		registryDetails = nil
	}

	return textResult(privateModuleDetailsText(module, registryDetails, tfeClient.BaseURL().Host)), nil, nil
}

func readTerraformRegistryModuleDetails(
	ctx context.Context,
	tfeClient *tfe.Client,
	moduleID tfe.RegistryModuleID,
	moduleVersion string,
) (*tfeclient.TerraformModuleVersionDetails, error) {
	req, err := tfeClient.NewRequest("GET", terraformRegistryModulePath(moduleID, moduleVersion), nil)
	if err != nil {
		return nil, err
	}

	module := &tfeclient.TerraformModuleVersionDetails{}
	if err := req.DoJSON(ctx, module); err != nil {
		return nil, err
	}

	return module, nil
}

func terraformRegistryModulePath(moduleID tfe.RegistryModuleID, moduleVersion string) string {
	basePath := "/api/registry/v1/modules"
	if moduleID.RegistryName == tfe.PublicRegistry {
		basePath = "/api/registry/public/v1/modules"
	}

	modulePath := path.Join(
		basePath,
		url.PathEscape(moduleID.Namespace),
		url.PathEscape(moduleID.Name),
		url.PathEscape(moduleID.Provider),
	)
	if moduleVersion != "" {
		modulePath += "/" + url.PathEscape(moduleVersion)
	}

	return modulePath
}

func privateModuleDetailsText(
	registryModule *tfe.RegistryModule,
	registryDetails *tfeclient.TerraformModuleVersionDetails,
	tfeHostAddress string,
) string {
	registryPath := path.Join(tfeHostAddress, registryModule.Namespace, registryModule.Name, registryModule.Provider)

	moduleVersion := ""
	if registryDetails != nil {
		moduleVersion = registryDetails.Version
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
	builder.WriteString("\n  # Add your module inputs here\n}\n```\n\n")

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
	if registryDetails != nil && registryDetails.Description != "" {
		builder.WriteString(fmt.Sprintf("- Description: %s\n", registryDetails.Description))
	}
	builder.WriteString("\n")

	if registryDetails != nil {
		if modulePartHasDetails(registryDetails.Root) {
			builder.WriteString("Root Module:\n")
			writeModulePartDetails(&builder, registryDetails.Root)
			writeModulePartReadme(&builder, registryDetails.Root)
		}

		for _, submodule := range registryDetails.Submodules {
			builder.WriteString(fmt.Sprintf("Submodule: %s\n", submodule.Name))
			builder.WriteString(fmt.Sprintf("- Path: %s\n\n", submodule.Path))
			writeModulePartDetails(&builder, submodule)
			writeModulePartReadme(&builder, submodule)
		}
	}

	if registryModule.Organization != nil {
		builder.WriteString("Organization:\n")
		builder.WriteString(fmt.Sprintf("- Name: %s\n\n", registryModule.Organization.Name))
	}

	if registryModule.Permissions != nil {
		builder.WriteString("Permissions:\n")
		builder.WriteString(fmt.Sprintf("- Can Delete: %t\n", registryModule.Permissions.CanDelete))
		builder.WriteString(fmt.Sprintf("- Can Resync: %t\n", registryModule.Permissions.CanResync))
		builder.WriteString(fmt.Sprintf("- Can Retry: %t\n\n", registryModule.Permissions.CanRetry))
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

	return builder.String()
}

func modulePartHasDetails(modulePart tfeclient.ModulePart) bool {
	return len(modulePart.Inputs) > 0 ||
		len(modulePart.Outputs) > 0 ||
		len(modulePart.Dependencies) > 0 ||
		len(modulePart.ProviderDependencies) > 0 ||
		len(modulePart.Resources) > 0 ||
		modulePart.Readme != ""
}

func writeModulePartDetails(builder *strings.Builder, modulePart tfeclient.ModulePart) {
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
			builder.WriteString(fmt.Sprintf("| %s | %s |\n", output.Name, output.Description))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.Dependencies) > 0 {
		builder.WriteString("Dependencies:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Source | Version |\n")
		builder.WriteString("|------|--------|---------|\n")
		for _, dependency := range modulePart.Dependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s |\n", dependency.Name, dependency.Source, dependency.Version))
		}
		builder.WriteString("\n")
	}

	if len(modulePart.ProviderDependencies) > 0 {
		builder.WriteString("Provider Dependencies:\n")
		builder.WriteString(strings.Repeat("-", 20))
		builder.WriteByte('\n')
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|------|-----------|--------|----------|\n")
		for _, dependency := range modulePart.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dependency.Name,
				dependency.Namespace,
				dependency.Source,
				dependency.Version,
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
			builder.WriteString(fmt.Sprintf("| %s | %s |\n", resource.Name, resource.Type))
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

func writeModulePartReadme(builder *strings.Builder, modulePart tfeclient.ModulePart) {
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
	result := make([]string, 0, len(lines))
	skipSection := false

	for _, line := range lines {
		lowerLine := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lowerLine, "##") {
			if strings.Contains(lowerLine, "inputs") ||
				strings.Contains(lowerLine, "outputs") ||
				strings.Contains(lowerLine, "dependencies") ||
				strings.Contains(lowerLine, "provider dependencies") ||
				strings.Contains(lowerLine, "resources") {
				skipSection = true
				continue
			}
			skipSection = false
		}

		if !skipSection {
			result = append(result, line)
		}
	}

	cleaned := strings.Join(result, "\n")
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}
