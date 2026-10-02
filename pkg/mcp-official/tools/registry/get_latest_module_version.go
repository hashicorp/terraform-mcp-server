// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GetLatestModuleVersionArguments struct {
	ModulePublisher string `json:"module_publisher" jsonschema:"The publisher of the module, e.g., 'hashicorp', 'aws-ia', 'terraform-google-modules', or 'Azure'"`
	ModuleName      string `json:"module_name" jsonschema:"The name of the module, usually the service or group of services being deployed, e.g. 'security-group' or 'secrets-manager'"`
	ModuleProvider  string `json:"module_provider" jsonschema:"The Terraform provider for the module, e.g. 'aws', 'google', or 'azurerm'"`
}

func GetLatestModuleVersionTool() *mcp.Tool {
	input, err := jsonschema.For[GetLatestModuleVersionArguments](nil)
	if err != nil {
		panic(err)
	}

	return &mcp.Tool{
		Name:        "get_latest_module_version",
		Description: "Fetches the latest version of a Terraform module from the public registry",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Latest Module Version",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: input,
	}
}

func GetLatestModuleVersionFunc(logger *slog.Logger) mcp.ToolHandlerFor[GetLatestModuleVersionArguments, any] {
	logrusLogger := logging.WrapSlog(logger)
	return func(ctx context.Context, request *mcp.CallToolRequest, input GetLatestModuleVersionArguments) (*mcp.CallToolResult, any, error) {
		return getLatestModuleVersion(ctx, request, input, logrusLogger)
	}
}

func getLatestModuleVersion(ctx context.Context, request *mcp.CallToolRequest, input GetLatestModuleVersionArguments, logger *log.Logger) (*mcp.CallToolResult, any, error) {
	modulePublisher := strings.ToLower(strings.TrimSpace(input.ModulePublisher))
	if modulePublisher == "" {
		return nil, nil, fmt.Errorf("missing required input: module_publisher")
	}

	moduleName := strings.ToLower(strings.TrimSpace(input.ModuleName))
	if moduleName == "" {
		return nil, nil, fmt.Errorf("missing required input: module_name")
	}

	moduleProvider := strings.ToLower(strings.TrimSpace(input.ModuleProvider))
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
