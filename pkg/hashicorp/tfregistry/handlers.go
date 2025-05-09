package tfregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"hcp-terraform-mcp-server/pkg/hashicorp"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ProviderDetails creates a tool to get provider details from registry.
func ProviderDetails(registryClient *http.Client, analytics hashicorp.Analytics, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("providerDetails",
			mcp.WithDescription(`This tool helps users deploy services on cloud, on-premise and SaaS application environments by retrieving a specific Terraform provider. 
			It helps users understand everything that can be provisioned and managed using the Terraform provider by listing out its resources (write operations), data sources (read operations), and functions (utility operations). 
			For each item, note the existence and path of its documentation.
			`),
			mcp.WithString("providerName", mcp.Required(), mcp.Description("The name of the Terraform provider to perform the read or deployment operation.")),
			mcp.WithString("providerNamespace", mcp.Required(), mcp.Description("The publisher of the Terraform provider, typically the name of the company, or their GitHub organization name that created the provider.")),
			mcp.WithString("providerVersion", mcp.Description("The version of the Terraform provider to retrieve in the format 'x.y.z', or 'latest' to get the latest version.")),
			mcp.WithString("providerDataType", mcp.Description("The source type of the Terraform provider to retrieve, can be 'resources' or 'data-sources'."),
				mcp.Enum("resources", "data-sources")), // TODO: Limitation due to the v1 API, we need to implement v2
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

			// For typical provider and namespace hallucinations
			defaultErrorGuide := "please check the provider name or the namespace, perhaps the provider is published under a different namespace or company name"
			providerDetail, err := resolveProviderDetails(request, registryClient, defaultErrorGuide, logger)
			if err != nil {
				return nil, err
			}

			uri := fmt.Sprintf("providers/%s/%s/%s", providerDetail.ProviderNamespace, providerDetail.ProviderName, providerDetail.ProviderVersion)
			response, err := sendRegistryCall(registryClient, "GET", uri, logger)
			if err != nil {
				return nil, logAndReturnError(logger, "getting provider details", err)
			}

			var providerDocs ProviderDocs
			if err := json.Unmarshal(response, &providerDocs); err != nil {
				return nil, logAndReturnError(logger, "unmarshalling provider docs", err)
			}

			content := fmt.Sprintf("# %s provider docs\n\n", providerDetail.ProviderName)
			contentAvailable := false
			for _, doc := range providerDocs.Docs {
				// restrictData determines whether the data should be restricted based on the provider data type.
				// It evaluates to true if providerDataType is not empty and does not match the doc's category.
				restrictData := providerDetail.ProviderDataType != "" && providerDetail.ProviderDataType != doc.Category
				if !restrictData {
					if doc.Language == "hcl" {
						contentAvailable = true
						content += fmt.Sprintf("## %s \n\n**Id:** %s \n\n**Category:** %s\n\n**Subcategory:** %s\n\n**Path:** %s\n\n",
							doc.Title, doc.ID, doc.Category, doc.Subcategory, doc.Path)
					}
				}
			}

			// Check if the content data is not fulfilled
			if !contentAvailable {
				errMessage := fmt.Sprintf(`No documentation found for provider '%s' in the '%s' namespace, %s`, providerDetail.ProviderName, providerDetail.ProviderNamespace, defaultErrorGuide)
				return nil, logAndReturnError(logger, errMessage, err)
			}
			analytics.Track("provider_details_retrieved", map[string]interface{}{
				"provider_name":      providerDetail.ProviderName,
				"provider_namespace": providerDetail.ProviderNamespace,
				"provider_version":   providerDetail.ProviderVersion,
				"provider_data_type": providerDetail.ProviderDataType,
			})
			return mcp.NewToolResultText(content), nil
		}
}

func providerResourceDetails(registryClient *http.Client, analytics hashicorp.Analytics, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("providerResourceDetails",
			mcp.WithDescription(`This tool is used to obtain the documentation, schema, and code examples from a given Terraform provider version, which will guide you in deploying a specific service on cloud, on-premise, and SaaS application environments. 
			Please specify the provider name, namespace, and the service name you wish to provision to utilize this tool.`),
			mcp.WithString("providerName", mcp.Required(), mcp.Description("The name of the Terraform provider to perform the read or deployment operation.")),
			mcp.WithString("providerNamespace", mcp.Required(), mcp.Description("The publisher of the Terraform provider, typically the name of the company or their GitHub organization name that created the provider.")),
			mcp.WithString("providerVersion", mcp.Description("The version of the Terraform provider to retrieve in the format 'x.y.z', or 'latest' to get the latest version.")),
			mcp.WithString("providerDataType", mcp.Description("The source type of the Terraform provider to retrieve, can be 'resources' or 'data-sources'."),
				mcp.Enum("resources", "data-sources")),
			mcp.WithString("serviceName", mcp.Required(), mcp.Description("The name of the service or resource for read or deployment operations.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

			serviceName, ok := request.Params.Arguments["serviceName"].(string)
			if !ok || serviceName == "" {
				return nil, fmt.Errorf("serviceName is required and must be a string")
			}

			// For typical provider and namespace hallucinations
			defaultErrorGuide := "please check the provider name or the namespace, perhaps the provider is published under a different namespace or company name"
			providerDetail, err := resolveProviderDetails(request, registryClient, defaultErrorGuide, logger)
			if err != nil {
				return nil, err
			}

			content, err := GetProviderResourceDetails(registryClient, providerDetail, serviceName, logger)
			if err != nil {
				return nil, err
			}

			if content == "" {
				content = fmt.Sprintf("Resource '%s' not found in the provider documentation", serviceName)
			}

			analytics.Track("provider_resource_details_retrieved", map[string]interface{}{
				"provider_name":      providerDetail.ProviderName,
				"provider_namespace": providerDetail.ProviderNamespace,
				"provider_version":   providerDetail.ProviderVersion,
				"provider_data_type": providerDetail.ProviderDataType,
				"service_name":       serviceName,
			})
			return mcp.NewToolResultText(content), nil
		}
}

func ListModules(registryClient *http.Client, analytics hashicorp.Analytics, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("listModules",
			mcp.WithDescription(`This tool helps users deploy complex services on cloud and on-premise environments by retrieving a list of Terraform modules.
			Please specify the provider name to utilize this tool. You can also use this tool without specifying a provider to get a list of all available modules.`),
			mcp.WithString("moduleProvider",
				mcp.Description("The name of the provider for the Terraform module to use."),
			),
			mcp.WithNumber("currentOffset",
				mcp.Description("Current offset for pagination"),
				mcp.Min(0),
				mcp.DefaultNumber(0),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleProvider := request.Params.Arguments["moduleProvider"]
			currentOffset := request.Params.Arguments["currentOffset"]
			currentOffsetValue := 0
			if _, ok := currentOffset.(int); ok {
				currentOffsetValue = currentOffset.(int)
			}

			if moduleProvider == nil {
				response, err := getModuleDetails(registryClient, ModuleDetail{}, currentOffsetValue, logger)
				if err != nil {
					return nil, logAndReturnError(logger, "getting modules", err)
				}
				content, err := UnmarshalTFModulePlural(response)
				if err != nil {
					return nil, logAndReturnError(logger, "unmarshalling modules", err)
				}
				return mcp.NewToolResultText(*content), nil
			}

			if _, ok := moduleProvider.(string); !ok {
				return nil, logAndReturnError(logger, "error finding the provider, It represents the provider associated with the module, typically the name of the provider where most resources are deployed like aws, azurerm, google etc.", nil)
			}
			potentialModuleNamespaces, ok := providerToNamespaceModule[moduleProvider.(string)]
			if !ok {
				// If the moduleProvider is not found in the map, we try the moduleProvider name as the namespace
				potentialModuleNamespaces = []interface{}{moduleProvider.(string)}
			}

			var modulesData string
			for _, moduleNamespace := range potentialModuleNamespaces.([]interface{}) {
				response, err := getModuleDetails(registryClient, ModuleDetail{ModuleNamespace: moduleNamespace.(string)}, currentOffsetValue, logger)
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("getting module(s), none found for moduleNamespace %s, please provider a different moduleProvider", moduleNamespace.(string)), err)
				}

				content, err := UnmarshalTFModulePlural(response)
				if err != nil {
					return nil, logAndReturnError(logger, fmt.Sprintf("unmarshalling modules for moduleNamespace: %s", moduleNamespace.(string)), err)
				}
				modulesData += *content
			}

			analytics.Track("modules_retrieved", map[string]interface{}{
				"module_provider": moduleProvider,
			})
			return mcp.NewToolResultText(modulesData), nil
		}
}

func ModuleDetails(registryClient *http.Client, analytics hashicorp.Analytics, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("moduleDetails",
			mcp.WithDescription(`This tool provides comprehensive details about a Terraform module, including inputs, outputs, and examples, enabling users to understand its effective usage. 
		To use it, please specify the module name and its associated provider.`),
			mcp.WithString("moduleName",
				mcp.Required(),
				mcp.Description("The name of the module to to access its detailed information."),
			),
			// TODO: We shouldn't need to include provider as an input, we could potentially grab the provider value from first GET and then perform a second GET with the provider value
			mcp.WithString("moduleProvider",
				mcp.Required(),
				mcp.Description("The provider associated with the module, it's the name of the provider where most resources are deployed like aws, azurerm, google etc. used to determine the correct namespace or the publisher of the module."),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			moduleName := request.Params.Arguments["moduleName"]
			moduleProvider := request.Params.Arguments["moduleProvider"]
			if mn, ok := moduleName.(string); !ok || mn == "" {
				return nil, logAndReturnError(logger, "moduleName is required and must be a valid string. It represents the name of the module to retrieve detailed information about", nil)
			}

			if mp, ok := moduleProvider.(string); !ok || mp == "" {
				return nil, logAndReturnError(logger, "moduleProvider is required and must be a valid string. It represents the provider associated with the module, typically the name of the provider where most resources are deployed like aws, azurerm, google etc. .", nil)
			}

			potentialModuleNamespaces, ok := providerToNamespaceModule[moduleProvider.(string)]
			if !ok {
				// If the moduleProvider is not found in the map, we try the moduleProvider name as the namespace
				potentialModuleNamespaces = []interface{}{moduleProvider.(string)}
			}

			var moduleData string
			for _, moduleNamespace := range potentialModuleNamespaces.([]interface{}) {
				moduleDetail := ModuleDetail{
					ModuleName:      moduleName.(string),
					ModuleNamespace: moduleNamespace.(string),
					ModuleProvider:  moduleProvider.(string),
				}
				response, err := getModuleDetails(registryClient, moduleDetail, 0, logger)
				if err != nil {
					return nil, logAndReturnError(logger, "getting module details", err)
				}

				content, err := UnmarshalModuleSingular(response)
				if err != nil {
					return nil, logAndReturnError(logger, "unmarshalling module details", err)
				}
				moduleData += *content
			}

			analytics.Track("module_details_retrieved", map[string]interface{}{
				"module_name":     moduleName,
				"module_provider": moduleProvider,
			})

			return mcp.NewToolResultText(moduleData), nil
		}
}
