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

type GetLatestModuleVersionArguments struct {
	ModulePublisher string `json:"module_publisher"`
	ModuleName      string `json:"module_name"`
	ModuleProvider  string `json:"module_provider"`
}

func GetLatestModuleVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_latest_module_version",
		Description: "Fetches the latest version of a Terraform module from the public registry",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Latest Module Version",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"module_publisher": {
					Type:        "string",
					Description: "The publisher of the module, e.g., 'hashicorp', 'aws-ia', 'terraform-google-modules', or 'Azure'",
				},
				"module_name": {
					Type:        "string",
					Description: "The name of the module, usually the service or group of services being deployed, e.g. 'security-group' or 'secrets-manager'",
				},
				"module_provider": {
					Type:        "string",
					Description: "The Terraform provider for the module, e.g. 'aws', 'google', or 'azurerm'",
				},
			},
			Required: []string{"module_publisher", "module_name", "module_provider"},
		},
	}
}

func GetLatestModuleVersionFunc(ctx context.Context, request *mcp.CallToolRequest, input GetLatestModuleVersionArguments) (*mcp.CallToolResult, any, error) {
	logger := log.StandardLogger()

	modulePublisher := strings.ToLower(input.ModulePublisher)
	if modulePublisher == "" {
		return nil, nil, fmt.Errorf("missing required input: module_publisher")
	}

	moduleName := strings.ToLower(input.ModuleName)
	if moduleName == "" {
		return nil, nil, fmt.Errorf("missing required input: module_name")
	}

	moduleProvider := strings.ToLower(input.ModuleProvider)
	if moduleProvider == "" {
		return nil, nil, fmt.Errorf("missing required input: module_provider")
	}

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	uri := fmt.Sprintf("modules/%s/%s/%s", modulePublisher, moduleName, moduleProvider)
	response, err := registryapi.SendRegistryCall(ctx, httpClient, http.MethodGet, uri, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching module information for %s/%s from the %s provider: %w", modulePublisher, moduleName, moduleProvider, err)
	}

	var moduleVersionDetails registryapi.TerraformModuleVersionDetails
	if err := json.Unmarshal(response, &moduleVersionDetails); err != nil {
		return nil, nil, fmt.Errorf("unmarshalling module information for %s/%s from the %s provider: %w", modulePublisher, moduleName, moduleProvider, err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: moduleVersionDetails.Version}},
	}, nil, nil
}
