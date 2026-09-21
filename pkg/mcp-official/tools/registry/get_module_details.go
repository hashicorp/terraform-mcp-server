// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

const moduleBasePath = "registry://modules"

type GetModuleDetailsArguments struct {
	ModuleID string `json:"module_id"`
}

func GetModuleDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_module_details",
		Description: "Fetches up-to-date documentation on how to use a Terraform module. You must call 'search_modules' first to obtain the exact valid and compatible module_id required to use this tool.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Retrieve documentation for a specific Terraform module",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"module_id": {
					Type:        "string",
					Description: "Exact valid and compatible module_id retrieved from search_modules (e.g., 'squareops/terraform-kubernetes-mongodb/mongodb/2.1.1', 'GoogleCloudPlatform/vertex-ai/google/0.2.0')",
				},
			},
			Required: []string{"module_id"},
		},
	}
}

func GetModuleDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetModuleDetailsArguments) (*mcp.CallToolResult, any, error) {
	logger := log.StandardLogger()

	moduleID := input.ModuleID
	if moduleID == "" {
		return nil, nil, fmt.Errorf("module_id cannot be empty")
	}

	if err := validateModuleID(moduleID); err != nil {
		return nil, nil, err
	}
	moduleID = strings.ToLower(moduleID)

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	response, err := getModuleDetails(ctx, httpClient, moduleID, 0, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("module not found: %s - use search_modules first to find valid module IDs", moduleID)
	}

	moduleData, err := unmarshalTerraformModule(response)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse module details: %w", err)
	}
	if moduleData == "" {
		return nil, nil, fmt.Errorf("no module data returned for %s - try a different module_id", moduleID)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: moduleData}},
	}, nil, nil
}

func getModuleDetails(ctx context.Context, httpClient *http.Client, moduleID string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleID != "" {
		uri = fmt.Sprintf("modules/%s", moduleID)
	}

	uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	response, err := registryapi.SendRegistryCall(ctx, httpClient, http.MethodGet, uri, logger)
	if err != nil {
		return nil, fmt.Errorf("getting module(s) for: %v, please provide a different provider name like aws, azurerm or google etc", moduleID)
	}

	return response, nil
}

func unmarshalTerraformModule(response []byte) (string, error) {
	var terraformModule registryapi.TerraformModuleVersionDetails
	if err := json.Unmarshal(response, &terraformModule); err != nil {
		return "", fmt.Errorf("unmarshalling module details: %w", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# %s/%s/%s\n\n", moduleBasePath, terraformModule.Namespace, terraformModule.Name))
	builder.WriteString(fmt.Sprintf("**Description:** %s\n\n", terraformModule.Description))
	builder.WriteString(fmt.Sprintf("**Module Version:** %s\n\n", terraformModule.Version))
	builder.WriteString(fmt.Sprintf("**Namespace:** %s\n\n", terraformModule.Namespace))
	builder.WriteString(fmt.Sprintf("**Source:** %s\n\n", terraformModule.Source))

	if len(terraformModule.Root.Inputs) > 0 {
		builder.WriteString("### Inputs\n\n")
		builder.WriteString("| Name | Type | Description | Default | Required |\n")
		builder.WriteString("|---|---|---|---|---|\n")
		for _, input := range terraformModule.Root.Inputs {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | `%v` | %t |\n",
				input.Name,
				input.Type,
				input.Description,
				input.Default,
				input.Required,
			))
		}
		builder.WriteString("\n")
	}

	if len(terraformModule.Root.Outputs) > 0 {
		builder.WriteString("### Outputs\n\n")
		builder.WriteString("| Name | Description |\n")
		builder.WriteString("|---|---|\n")
		for _, output := range terraformModule.Root.Outputs {
			builder.WriteString(fmt.Sprintf("| %s | %s |\n", output.Name, output.Description))
		}
		builder.WriteString("\n")
	}

	if len(terraformModule.Root.ProviderDependencies) > 0 {
		builder.WriteString("### Provider Dependencies\n\n")
		builder.WriteString("| Name | Namespace | Source | Version |\n")
		builder.WriteString("|---|---|---|---|\n")
		for _, dependency := range terraformModule.Root.ProviderDependencies {
			builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				dependency.Name,
				dependency.Namespace,
				dependency.Source,
				dependency.Version,
			))
		}
		builder.WriteString("\n")
	}

	if len(terraformModule.Examples) > 0 {
		builder.WriteString("### Examples\n\n")
		for _, example := range terraformModule.Examples {
			builder.WriteString(fmt.Sprintf("#### %s\n\n", example.Name))
			if example.Readme != "" {
				builder.WriteString("**Readme:**\n\n")
				builder.WriteString(example.Readme)
				builder.WriteString("\n\n")
			}
		}
		builder.WriteString("\n")
	}

	return builder.String(), nil
}

func validateModuleID(moduleID string) error {
	parts := strings.Split(moduleID, "/")
	if len(parts) != 4 {
		return fmt.Errorf("invalid module ID format '%s'. Expected format: namespace/name/provider/version (4 parts). Use search_modules to find valid module IDs", moduleID)
	}
	return nil
}
