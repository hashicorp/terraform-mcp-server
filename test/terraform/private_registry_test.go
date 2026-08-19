package terraform

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-tfe"
	mcpclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateRegistryModules(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)

	// Create an isolated private module registry record.
	moduleName := randomName("mcp-test-module-")
	moduleProvider := "testprovider"
	module, err := client.RegistryModules.Create(t.Context(), tfeOrgName, tfe.RegistryModuleCreateOptions{
		Name:         tfe.String(moduleName),
		Provider:     tfe.String(moduleProvider),
		RegistryName: tfe.PrivateRegistry,
	})
	require.NoError(t, err, "failed to create test private module")

	// Build the private module registry locator used by version, details, and cleanup APIs.
	moduleLocator := tfe.RegistryModuleID{
		Organization: tfeOrgName,
		Namespace:    module.Namespace,
		Name:         module.Name,
		Provider:     module.Provider,
		RegistryName: tfe.PrivateRegistry,
	}
	defer client.RegistryModules.DeleteProvider(t.Context(), moduleLocator)

	// Create a version of the private module and upload the module source code.
	const moduleVersion = "1.0.0"
	version, err := client.RegistryModules.CreateVersion(t.Context(), moduleLocator, tfe.RegistryModuleCreateVersionOptions{
		Version: tfe.String(moduleVersion),
	})
	require.NoError(t, err, "failed to create test private module version")
	require.NoError(t, client.RegistryModules.Upload(t.Context(), *version, "testdata/private_registry_module"), "failed to upload test private module")

	// Module source is processed asynchronously after upload. Wait until the root
	// module and submodule details are available through the registry API.
	registryDetails := waitFor(t, 2*time.Minute, fmt.Sprintf("private module %q version %s to finish processing", moduleLocator.Name, moduleVersion), func(ctx context.Context) (*mcpclient.TerraformModuleVersionDetails, error) {
		u := fmt.Sprintf("/api/registry/v1/modules/%s/%s/%s/%s",
			url.PathEscape(moduleLocator.Namespace),
			url.PathEscape(moduleLocator.Name),
			url.PathEscape(moduleLocator.Provider),
			url.PathEscape(moduleVersion),
		)
		req, err := client.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		module := &mcpclient.TerraformModuleVersionDetails{}
		if err := req.DoJSON(ctx, module); err != nil {
			return nil, err
		}

		ready := len(module.Root.Inputs) > 0 && len(module.Root.Outputs) > 0 && module.Root.Readme != "" &&
			len(module.Submodules) > 0 && len(module.Submodules[0].Inputs) > 0 && len(module.Submodules[0].Outputs) > 0 &&
			len(module.Submodules[0].ProviderDependencies) > 0 && len(module.Submodules[0].Resources) > 0 && module.Submodules[0].Readme != ""
		if !ready {
			return nil, nil
		}
		return module, nil
	})

	privateModuleAddress := strings.Join([]string{module.Namespace, module.Name, module.Provider}, "/")

	t.Run("Search private modules", func(t *testing.T) {
		result, resultText := callTool(t, s, "search_private_modules", map[string]any{
			"terraform_org_name": tfeOrgName,
			"search_query":       moduleName,
		})
		require.False(t, result.IsError, "search_private_modules should not return an error")
		require.NotEmpty(t, resultText, "search_private_modules response must not be empty")

		// Verify against the TFE API directly.
		modules, err := client.RegistryModules.List(t.Context(), tfeOrgName, &tfe.RegistryModuleListOptions{
			Search: moduleName,
		})
		require.NoError(t, err)
		require.Len(t, modules.Items, 1, "the unique search should return only one private module we created")
		require.NotNil(t, modules.Items[0], "the TFE API should return a private module")

		directModule := modules.Items[0]
		expectedModuleAddress := strings.Join([]string{directModule.Namespace, directModule.Name, directModule.Provider}, "/")
		// TODO: The private registry tools currently return plain text, so verify
		// expected values with assert.Contains(). Use structured field assertions if
		// these tools return structured output after the MCP SDK migration.
		assert.Contains(t, resultText, expectedModuleAddress)
	})

	t.Run("Get private module details", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_private_module_details", map[string]any{
			"terraform_org_name":     tfeOrgName,
			"private_module_id":      privateModuleAddress,
			"private_module_version": moduleVersion,
		})

		t.Logf("Get private module details: %v", resultText)

		require.False(t, result.IsError, "get_private_module_details should not return an error")
		require.NotEmpty(t, resultText, "get_private_module_details response must not be empty")

		// Verify against the TFE API directly.
		expectedModuleAddress := strings.Join([]string{registryDetails.Namespace, registryDetails.Name, registryDetails.Provider}, "/")

		// TODO: update this after MCP SKD migration
		assert.Contains(t, resultText, expectedModuleAddress)
		assert.Contains(t, resultText, registryDetails.Root.Inputs[0].Name)
		assert.Contains(t, resultText, registryDetails.Root.Inputs[0].Description)
		assert.Contains(t, resultText, registryDetails.Root.Outputs[0].Name)
		assert.Contains(t, resultText, registryDetails.Root.Outputs[0].Description)
		assert.Contains(t, resultText, strings.TrimSpace(registryDetails.Root.Readme))

		submodule := registryDetails.Submodules[0]
		assert.Contains(t, resultText, "Submodule: "+submodule.Name)
		assert.Contains(t, resultText, submodule.Path)
		assert.Contains(t, resultText, submodule.Inputs[0].Name)
		assert.Contains(t, resultText, submodule.Outputs[0].Name)
		assert.Contains(t, resultText, submodule.ProviderDependencies[0].Source)
		assert.Contains(t, resultText, submodule.Resources[0].Type)
		assert.Contains(t, resultText, strings.TrimSpace(submodule.Readme))
	})
}

func TestPrivateRegistryProviders(t *testing.T) {
	requireTfOperations(t)

	s := newTestingSession(t)
	defer s.Close()

	client := tfeClient(t)

	// Create an isolated private provider registry record.
	providerName := randomName("mcp-test-provider-")
	provider, err := client.RegistryProviders.Create(t.Context(), tfeOrgName, tfe.RegistryProviderCreateOptions{
		Name:         providerName,
		Namespace:    tfeOrgName,
		RegistryName: tfe.PrivateRegistry,
	})
	require.NoError(t, err, "failed to create test private provider")

	// Build the composite locator used by details and cleanup APIs.
	providerLocator := tfe.RegistryProviderID{
		OrganizationName: tfeOrgName,
		Namespace:        provider.Namespace,
		Name:             provider.Name,
		RegistryName:     tfe.PrivateRegistry,
	}
	defer client.RegistryProviders.Delete(t.Context(), providerLocator)

	t.Run("Search private providers", func(t *testing.T) {
		result, resultText := callTool(t, s, "search_private_providers", map[string]any{
			"terraform_org_name": tfeOrgName,
			"search_query":       providerName,
			"registry_name":      "private",
		})
		require.False(t, result.IsError, "search_private_providers should not return an error")
		require.NotEmpty(t, resultText, "search_private_providers response must not be empty")

		// Verify against the TFE API directly.
		providers, err := client.RegistryProviders.List(t.Context(), tfeOrgName, &tfe.RegistryProviderListOptions{
			Search:       providerName,
			RegistryName: tfe.PrivateRegistry,
		})
		require.NoError(t, err)
		require.Len(t, providers.Items, 1, "the unique search should return only one private provider we created")
		require.NotNil(t, providers.Items[0], "the TFE API should return a private provider")

		directProvider := providers.Items[0]
		expectedProviderAddress := strings.Join([]string{directProvider.Namespace, directProvider.Name}, "/")

		// TODO: update this after MCP SKD migration
		assert.Contains(t, resultText, expectedProviderAddress)
		assert.Contains(t, resultText, directProvider.ID)
	})

	t.Run("Get private provider details", func(t *testing.T) {
		result, resultText := callTool(t, s, "get_private_provider_details", map[string]any{
			"terraform_org_name":         tfeOrgName,
			"private_provider_namespace": provider.Namespace,
			"private_provider_name":      provider.Name,
			"include_versions":           false,
		})
		require.False(t, result.IsError, "get_private_provider_details should not return an error")
		require.NotEmpty(t, resultText, "get_private_provider_details response must not be empty")

		// Verify against the TFE API directly.
		directProvider, err := client.RegistryProviders.Read(t.Context(), providerLocator, &tfe.RegistryProviderReadOptions{})
		require.NoError(t, err)
		require.NotNil(t, directProvider, "the TFE API should return the private provider")

		expectedProviderAddress := strings.Join([]string{directProvider.Namespace, directProvider.Name}, "/")

		// TODO: update this after MCP SKD migration
		assert.Contains(t, resultText, expectedProviderAddress)
		assert.Contains(t, resultText, directProvider.ID)
	})
}
