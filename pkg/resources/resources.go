// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// Base URL for the Terraform style guide and module development guide markdown files
const terraformGuideRawURL = "https://raw.githubusercontent.com/hashicorp/web-unified-docs/main/content/terraform/v1.12.x/docs/language"

// RegisterResources adds the new resource
func RegisterResources(hcServer *server.MCPServer, logger *log.Logger) {
	hcServer.AddResource(TerraformStyleGuideResource(logger))
	hcServer.AddResource(TerraformModuleDevGuideResource(logger))
}

// TerraformStyleGuideResource returns the resource and handler for the style guide
func TerraformStyleGuideResource(logger *log.Logger) (mcp.Resource, server.ResourceHandlerFunc) {
	resourceURI := "/terraform/style-guide"
	description := "Terraform Style Guide"

	return mcp.NewResource(
			resourceURI,
			description,
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription(description),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {

			// Get a simple http client to access the public Terraform registry from context
			httpClient, err := client.GetHttpClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting http client for public Terraform registry", err)
			}

			resp, err := httpClient.Get(fmt.Sprintf("%s/style.mdx", terraformGuideRawURL))
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting URL for Terraform Style Guide markdown", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, utils.LogAndReturnError(logger, "fetching Terraform Style Guide markdown", fmt.Errorf("status: %s", resp.Status))
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "reading Terraform Style Guide markdown", err)
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
func TerraformModuleDevGuideResource(logger *log.Logger) (mcp.Resource, server.ResourceHandlerFunc) {
	resourceURI := "/terraform/module-development"
	description := "Terraform Module Development Guide"

	var urls = []struct {
		Name string
		URL  string
	}{
		{"index", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/index.mdx")},
		{"composition", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/composition.mdx")},
		{"structure", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/structure.mdx")},
		{"providers", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/providers.mdx")},
		{"publish", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/publish.mdx")},
		{"refactoring", fmt.Sprintf("%s/%s", terraformGuideRawURL, "modules/develop/refactoring.mdx")},
	}

	return mcp.NewResource(
			resourceURI,
			description,
			mcp.WithMIMEType("text/markdown"),
			mcp.WithResourceDescription(description),
		),
		func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			// Get a simple http client to access the public Terraform registry from context
			httpClient, err := client.GetHttpClientFromContext(ctx, logger)
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "getting http client for public Terraform registry", err)
			}

			var contents []mcp.ResourceContents
			for _, u := range urls {
				resp, err := httpClient.Get(u.URL)
				if err != nil {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("fetching %s markdown", u.Name), err)
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("fetching %s markdown, status not ok", u.Name), fmt.Errorf("status: %s", resp.Status))
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("reading %s markdown", u.Name), err)
				}
				contents = append(contents, mcp.TextResourceContents{
					MIMEType: "text/markdown",
					URI:      path.Join(resourceURI, u.Name),
					Text:     string(body),
				})
			}
			return contents, nil
		}
}
