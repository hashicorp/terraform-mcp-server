// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type SearchModulesArguments struct {
	ModuleQuery   string `json:"module_query"`
	CurrentOffset int    `json:"current_offset,omitempty"`
}

func SearchModulesTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "search_modules",
		Description: `Resolves a Terraform module name to obtain a compatible module_id for the get_module_details tool and returns a list of matching Terraform modules.
You MUST call this function before 'get_module_details' to obtain a valid and compatible module_id.
When selecting the best match, consider the following:
	- Name similarity to the query
	- Description relevance
	- Verification status (verified)
	- Download counts (popularity)
Return the selected module_id and explain your choice. If there are multiple good matches, mention this but proceed with the most relevant one.
If no modules were found, reattempt the search with a new moduleName query.`,
		Annotations: &mcp.ToolAnnotations{
			Title:           "Search and match Terraform modules based on name and relevance",
			OpenWorldHint:   ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"module_query": {
					Type:        "string",
					Description: "The query to search for Terraform modules",
				},
				"current_offset": {
					Type:        "integer",
					Description: "Current offset for pagination",
					Minimum:     ptr(0.0),
					Default:     json.RawMessage(`0`),
				},
			},
			Required: []string{"module_query"},
		},
	}
}

func SearchModulesFunc(ctx context.Context, request *mcp.CallToolRequest, input SearchModulesArguments) (*mcp.CallToolResult, any, error) {
	logger := log.StandardLogger()
	moduleQuery := strings.ToLower(input.ModuleQuery)
	if moduleQuery == "" {
		return nil, nil, fmt.Errorf("missing required input: module_query")
	}

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	response, err := sendSearchModulesCall(ctx, httpClient, moduleQuery, input.CurrentOffset, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("no modules found for query: %s - try a different search term", moduleQuery)
	}

	modulesData, err := unmarshalTerraformModules(response, moduleQuery)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse module results for query: %s", moduleQuery)
	}
	if modulesData == "" {
		return nil, nil, fmt.Errorf("no modules found for query: %s - try a different search term", moduleQuery)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: modulesData}},
	}, nil, nil
}

func sendSearchModulesCall(ctx context.Context, httpClient *http.Client, moduleQuery string, currentOffset int, logger *log.Logger) ([]byte, error) {
	uri := "modules"
	if moduleQuery != "" {
		uri = fmt.Sprintf("%s/search?q='%s'&offset=%v", uri, url.PathEscape(moduleQuery), currentOffset)
	} else {
		uri = fmt.Sprintf("%s?offset=%v", uri, currentOffset)
	}

	response, err := registryapi.SendRegistryCall(ctx, httpClient, http.MethodGet, uri, logger)
	if err != nil {
		return nil, fmt.Errorf("getting module(s) for: %v, call error: %v", moduleQuery, err)
	}

	return response, nil
}

func unmarshalTerraformModules(response []byte, moduleQuery string) (string, error) {
	var terraformModules registryapi.TerraformModules
	if err := json.Unmarshal(response, &terraformModules); err != nil {
		return "", fmt.Errorf("unmarshalling modules: %w", err)
	}
	if len(terraformModules.Data) == 0 {
		return "", fmt.Errorf("no modules found for query: %s", moduleQuery)
	}

	sort.Slice(terraformModules.Data, func(i, j int) bool {
		return terraformModules.Data[i].Downloads > terraformModules.Data[j].Downloads
	})

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Available Terraform Modules (top matches) for %s\n\n Each result includes:\n", moduleQuery))
	builder.WriteString("- module_id: The module ID (format: namespace/name/provider-name/module-version)\n")
	builder.WriteString("- Name: The name of the module\n")
	builder.WriteString("- Description: A short description of the module\n")
	builder.WriteString("- Downloads: The total number of times the module has been downloaded\n")
	builder.WriteString("- Verified: Verification status of the module\n")
	builder.WriteString("- Published: The date and time when the module was published\n")
	builder.WriteString("\n\n---\n\n")
	for _, module := range terraformModules.Data {
		builder.WriteString(fmt.Sprintf("- module_id: %s\n", module.ID))
		builder.WriteString(fmt.Sprintf("- Name: %s\n", module.Name))
		builder.WriteString(fmt.Sprintf("- Description: %s\n", module.Description))
		builder.WriteString(fmt.Sprintf("- Downloads: %d\n", module.Downloads))
		builder.WriteString(fmt.Sprintf("- Verified: %t\n", module.Verified))
		builder.WriteString(fmt.Sprintf("- Published: %s\n", module.PublishedAt))
		builder.WriteString("---\n\n")
	}

	return builder.String(), nil
}
