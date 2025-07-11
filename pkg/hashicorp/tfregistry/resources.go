// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tfregistry

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// Base URL for the Terraform style guide and module development guide markdown files
const terraformGuideRawURL = "https://raw.githubusercontent.com/hashicorp/web-unified-docs/main/content/terraform/v1.12.x/docs/language"

// RegisterResources adds the new resource
func RegisterResources(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {
	hcServer.AddResource(TerraformStyleGuideResource(registryClient, logger))
	hcServer.AddResource(TerraformModuleDevGuideResource(registryClient, logger))
}

// TerraformStyleGuideResource returns the resource and handler for the style guide
func TerraformStyleGuideResource(httpClient *http.Client, logger *log.Logger) (mcp.Resource, server.ResourceHandlerFunc) {
	resourceURI := "/terraform/style-guide"
	description := "Terraform Style Guide"

	return mcp.NewResource(
			resourceURI,
			description,
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription(description),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			resp, err := httpClient.Get(fmt.Sprintf("%s/style.mdx", terraformGuideRawURL))
			if err != nil {
				return nil, logAndReturnError(logger, "Error fetching Terraform Style Guide markdown", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, logAndReturnError(logger, "Non-200 response fetching Terraform Style Guide markdown", fmt.Errorf("status: %s", resp.Status))
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, logAndReturnError(logger, "Error reading Terraform Style Guide markdown", err)
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{
					MIMEType: "text/markdown",
					URI:      resourceURI,
					Text:     string(body),
				},
			}, nil
		}
}

// TerraformModuleDevGuideResource returns a resource and handler for the Terraform Module Development Guide markdown files
func TerraformModuleDevGuideResource(httpClient *http.Client, logger *log.Logger) (mcp.Resource, server.ResourceHandlerFunc) {
	resourceURI := "/terraform/module-development"
	description := "Terraform Module Development Guide"

	var urls = []struct {
		Name string
		URL  string
	}{
		{"index", fmt.Sprintf("%s/modules/develop/index.mdx", terraformGuideRawURL)},
		{"composition", fmt.Sprintf("%s/modules/develop/composition.mdx", terraformGuideRawURL)},
		{"structure", fmt.Sprintf("%s/modules/develop/structure.mdx", terraformGuideRawURL)},
		{"providers", fmt.Sprintf("%s/modules/develop/providers.mdx", terraformGuideRawURL)},
		{"publish", fmt.Sprintf("%s/modules/develop/publish.mdx", terraformGuideRawURL)},
		{"refactoring", fmt.Sprintf("%s/modules/develop/refactoring.mdx", terraformGuideRawURL)},
	}

	return mcp.NewResource(
			resourceURI,
			description,
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription(description),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			var contents []mcp.ResourceContents
			for _, u := range urls {
				resp, err := httpClient.Get(u.URL)
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("Error fetching %s markdown", u.Name), err)
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					return nil, logAndReturnError(logger, fmt.Sprintf("Non-200 response fetching %s markdown", u.Name), fmt.Errorf("status: %s", resp.Status))
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("Error reading %s markdown", u.Name), err)
				}
				contents = append(contents, mcp.TextResourceContents{
					MIMEType: "text/markdown",
					URI:      fmt.Sprintf("%s/%s", resourceURI, u.Name),
					Text:     string(body),
				})
			}
			return contents, nil
		}
}
