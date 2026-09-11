// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GetProviderDetailsArguments struct {
	ProviderDocID string `json:"provider_doc_id" jsonschema:"Exact tfprovider-compatible provider_doc_id, (e.g., '8894603', '8906901') retrieved from 'search_providers'"`
}

func GetProviderDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_provider_details",
		Description: `Fetches up-to-date documentation for a specific service from a Terraform provider. You must call 'search_providers' tool first to obtain the exact tfprovider-compatible provider_doc_id required to use this tool.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Fetch detailed Terraform provider documentation using a document ID",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
	}
}

func GetProviderDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetProviderDetailsArguments) (*mcp.CallToolResult, any, error) {
	providerDocID := strings.TrimSpace(input.ProviderDocID)
	if providerDocID == "" {
		return nil, nil, fmt.Errorf("provider_doc_id cannot be empty")
	}
	if _, err := strconv.Atoi(providerDocID); err != nil {
		return nil, nil, fmt.Errorf("provider_doc_id must be a valid number - use search_providers first to find valid IDs")
	}

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	detailResp, err := registryapi.SendRegistryCall(ctx, httpClient, "GET", path.Join("provider-docs", providerDocID), log.StandardLogger(), "v2")
	if err != nil {
		return nil, nil, fmt.Errorf("provider doc not found: %s - use search_providers first to find valid provider_doc_id values", providerDocID)
	}

	var details registryapi.ProviderResourceDetails
	if err := json.Unmarshal(detailResp, &details); err != nil {
		return nil, nil, fmt.Errorf("failed to parse provider docs for %s", providerDocID)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: details.Data.Attributes.Content}},
	}, nil, nil
}

func ptr[T any](v T) *T {
	return &v
}
