// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GetLatestProviderVersionArguments struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

func GetLatestProviderVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_latest_provider_version",
		Description: "Fetches the latest version of a Terraform provider from the public registry",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Latest Provider Version",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"namespace": {
					Type:        "string",
					Description: "The namespace of the Terraform provider, typically the company or GitHub organization that created it, e.g. hashicorp",
				},
				"name": {
					Type:        "string",
					Description: "The name of the Terraform provider, e.g. aws, azurerm, or google",
				},
			},
			Required: []string{"namespace", "name"},
		},
	}
}

func GetLatestProviderVersionFunc(ctx context.Context, request *mcp.CallToolRequest, input GetLatestProviderVersionArguments) (*mcp.CallToolResult, any, error) {
	// TODO: Replace with structured slog logging
	logger := log.StandardLogger()

	namespace := GetString(input.Namespace, "")
	if namespace == "" {
		return nil, nil, fmt.Errorf("missing required input: namespace")
	}
	namespace = strings.ToLower(namespace)

	name := GetString(input.Name, "")
	if name == "" {
		return nil, nil, fmt.Errorf("missing required input: name")
	}
	name = strings.ToLower(name)

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	version, err := registryapi.GetLatestProviderVersion(ctx, httpClient, namespace, name, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("provider not found: %s/%s - verify the namespace and provider name are correct", namespace, name)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: version},
		},
	}, nil, nil
}
