// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GetLatestProviderVersionArguments struct {
	Namespace string `json:"namespace" jsonschema:"The namespace of the Terraform provider, typically the company or GitHub organization that created it, e.g. hashicorp"`
	Name      string `json:"name" jsonschema:"The name of the Terraform provider, e.g. aws, azurerm, or google"`
}

func GetLatestProviderVersionTool() *mcp.Tool {
	input, err := jsonschema.For[GetLatestProviderVersionArguments](nil)
	if err != nil {
		panic(err)
	}

	return &mcp.Tool{
		Name:        "get_latest_provider_version",
		Description: "Fetches the latest version of a Terraform provider from the public registry",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Get Latest Provider Version",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: input,
	}
}

// GetLatestProviderVersionFunc returns the get_latest_provider_version handler.
// Logs from the shared registry client are forwarded to logger.
func GetLatestProviderVersionFunc(logger *slog.Logger) mcp.ToolHandlerFor[GetLatestProviderVersionArguments, any] {
	logrusLogger := logging.WrapSlog(logger)
	return func(ctx context.Context, request *mcp.CallToolRequest, input GetLatestProviderVersionArguments) (*mcp.CallToolResult, any, error) {
		return getLatestProviderVersion(ctx, request, input, logrusLogger)
	}
}

func getLatestProviderVersion(ctx context.Context, request *mcp.CallToolRequest, input GetLatestProviderVersionArguments, logger *log.Logger) (*mcp.CallToolResult, any, error) {
	namespace := strings.TrimSpace(input.Namespace)
	if namespace == "" {
		return nil, nil, fmt.Errorf("missing required input: namespace")
	}
	namespace = strings.ToLower(namespace)

	name := strings.TrimSpace(input.Name)
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
