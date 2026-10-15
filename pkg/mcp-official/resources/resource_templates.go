// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path"

	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/logging"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

// RegisterResourceTemplates adds the provider details resource template to the server
func RegisterResourceTemplates(svr *mcp.Server, logger *slog.Logger) {
	svr.AddResourceTemplate(
		providerResourceTemplate(
			path.Join(utils.PROVIDER_BASE_PATH, "{namespace}", "name", "{name}", "version", "{version}"),
			"Provider details",
			logger,
		),
	)
}

func providerResourceTemplate(resourceURI string, description string, logger *slog.Logger) (*mcp.ResourceTemplate, mcp.ResourceHandler) {
	logrusLogger := logging.WrapSlog(logger)

	return &mcp.ResourceTemplate{
			URITemplate: resourceURI,
			Name:        description,
			Description: "Describes details for a Terraform provider",
			MIMEType:    "application/json",
		},
		func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			logrusLogger.Infof("Provider resource template - resourceURI: %s", request.Params.URI)

			// Get a simple http client to access the public Terraform registry
			httpClient, err := client.GetHttpClient(ctx, client.SessionID(request.Session))
			if err != nil {
				return nil, utils.LogAndReturnError(logrusLogger, "getting http client for public Terraform registry", err)
			}
			providerDocs, err := providerResourceTemplateHelper(ctx, httpClient, request.Params.URI, logrusLogger)
			if err != nil {
				return nil, utils.LogAndReturnError(logrusLogger, "getting provider details for resource template", err)
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{
					{
						MIMEType: "text/markdown",
						URI:      resourceURI,
						Text:     providerDocs,
					},
				},
			}, nil
		}
}

// providerResourceTemplateHelper fetches the provider details based on the resource URI
func providerResourceTemplateHelper(ctx context.Context, httpClient *http.Client, resourceURI string, logger *log.Logger) (string, error) {
	namespace, name, version, err := utils.ExtractProviderNameAndVersion(resourceURI)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "extracting provider name and version", err)
	}
	logger.Debugf("Extracted namespace: %s, name: %s, version: %s", namespace, name, version)

	if version == "" || version == "latest" || !utils.IsValidProviderVersionFormat(version) {
		version, err = registryapi.GetLatestProviderVersion(ctx, httpClient, namespace, name, logger)
		if err != nil {
			return "", utils.LogAndReturnError(logger, fmt.Sprintf("getting %s/%s latest provider version for resource template", namespace, name), err)
		}
	}

	providerVersionUri := path.Join(utils.PROVIDER_BASE_PATH, namespace, "name", name, "version", version)
	logger.Debugf("Provider resource template - providerVersionUri: %s", providerVersionUri)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider details for resource template", err)
	}

	// Get the provider-version-id for the specified provider version
	providerVersionID, err := registryapi.GetProviderVersionID(ctx, httpClient, namespace, name, version, logger)
	logger.Debugf("Provider resource template - Provider version id providerVersionID: %s, providerVersionUri: %s", providerVersionID, providerVersionUri)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider details for provider-version-id", err)
	}

	// Get all the docs based on provider version id
	providerDocs, err := registryapi.GetProviderOverviewDocs(ctx, httpClient, providerVersionID, logger)
	logger.Debugf("Provider resource template - Provider docs providerVersionID: %s", providerVersionID)
	if err != nil {
		return "", utils.LogAndReturnError(logger, "getting provider details for docs with provider-version-id", err)
	}

	// Only return the provider overview
	return providerDocs, nil
}
