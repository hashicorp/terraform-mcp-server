// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrivateProviderDetailsTool(t *testing.T) {
	tool := GetPrivateProviderDetailsTool()

	assert.Equal(t, "get_private_provider_details", tool.Name)
	assert.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get detailed information about a private provider", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema := inputSchema(t, tool.InputSchema)
	assert.Equal(t, []string{
		"terraform_org_name",
		"private_provider_namespace",
		"private_provider_name",
	}, schema.Required)
	assert.Equal(t, []string{
		"terraform_org_name",
		"private_provider_namespace",
		"private_provider_name",
		"registry_name",
		"include_versions",
	}, schema.PropertyOrder)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	registryName := schema.Properties["registry_name"]
	require.NotNil(t, registryName)
	assert.Equal(t, enumOf(validRegistries...), registryName.Enum)
	assert.JSONEq(t, `"private"`, string(registryName.Default))

	includeVersions := schema.Properties["include_versions"]
	require.NotNil(t, includeVersions)
	assert.Equal(t, "boolean", includeVersions.Type)
	assert.JSONEq(t, "true", string(includeVersions.Default))
}

func TestGetPrivateProviderDetailsFuncValidation(t *testing.T) {
	tests := []struct {
		name  string
		input GetPrivateProviderDetailsArguments
		want  string
	}{
		{
			name: "blank organization",
			input: GetPrivateProviderDetailsArguments{
				TerraformOrgName:         "   ",
				PrivateProviderNamespace: "acme",
				PrivateProviderName:      "example",
			},
			want: "terraform_org_name must not be blank",
		},
		{
			name: "blank namespace",
			input: GetPrivateProviderDetailsArguments{
				TerraformOrgName:         "acme",
				PrivateProviderNamespace: "   ",
				PrivateProviderName:      "example",
			},
			want: "private_provider_namespace must not be blank",
		},
		{
			name: "blank provider name",
			input: GetPrivateProviderDetailsArguments{
				TerraformOrgName:         "acme",
				PrivateProviderNamespace: "acme",
				PrivateProviderName:      "   ",
			},
			want: "private_provider_name must not be blank",
		},
		{
			name: "invalid registry",
			input: GetPrivateProviderDetailsArguments{
				TerraformOrgName:         "acme",
				PrivateProviderNamespace: "acme",
				PrivateProviderName:      "example",
				RegistryName:             "partner",
			},
			want: `registry_name must be one of ["private" "public"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetPrivateProviderDetailsFunc(t.Context(), nil, tt.input)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestPrivateProviderDetailsText(t *testing.T) {
	provider := &tfe.RegistryProvider{
		ID:           "prov-123",
		Name:         "example",
		Namespace:    "acme",
		RegistryName: tfe.PrivateRegistry,
		CreatedAt:    "2026-01-02T03:04:05Z",
		UpdatedAt:    "2026-02-03T04:05:06Z",
		Permissions:  tfe.RegistryProviderPermissions{CanDelete: true},
		Organization: &tfe.Organization{Name: "acme", Email: "admin@example.com"},
		RegistryProviderVersions: []*tfe.RegistryProviderVersion{
			{
				ID:        "version-123",
				Version:   "2.0.0",
				CreatedAt: "2026-02-01T00:00:00Z",
				UpdatedAt: "2026-02-02T00:00:00Z",
				KeyID:     "key-123",
				Permissions: tfe.RegistryProviderVersionPermissions{
					CanUploadAsset: true,
					CanDelete:      true,
				},
				RegistryProviderPlatforms: []*tfe.RegistryProviderPlatform{
					{OS: "linux", Arch: "amd64"},
					nil,
					{OS: "darwin", Arch: "arm64"},
				},
			},
			nil,
		},
		Links: map[string]interface{}{
			"self":    "https://example.com/provider",
			"ignored": 42,
		},
	}

	t.Run("includes provider and version details", func(t *testing.T) {
		text := privateProviderDetailsText(provider, true)

		assert.Contains(t, text, "Private Provider Details: acme/example")
		assert.Contains(t, text, `source = "acme/example"`)
		assert.Contains(t, text, `version = "2.0.0"`)
		assert.Contains(t, text, "- ID: prov-123")
		assert.Contains(t, text, "- Email: admin@example.com")
		assert.Contains(t, text, "- Can Delete: true")
		assert.Contains(t, text, "Available Versions (1):")
		assert.Contains(t, text, "Key ID: key-123")
		assert.Contains(t, text, "Permissions: upload-asset, delete")
		assert.Contains(t, text, "Platforms: linux/amd64, darwin/arm64")
		assert.Contains(t, text, "- self: https://example.com/provider")
		assert.NotContains(t, text, "ignored: 42")
	})

	t.Run("omits version details when disabled", func(t *testing.T) {
		text := privateProviderDetailsText(&tfe.RegistryProvider{
			ID:           provider.ID,
			Name:         provider.Name,
			Namespace:    provider.Namespace,
			RegistryName: provider.RegistryName,
		}, false)

		assert.NotContains(t, text, "Available Versions")
		assert.NotContains(t, text, "No version information")
		assert.NotContains(t, text, "version =")
	})

	t.Run("reports missing requested versions", func(t *testing.T) {
		text := privateProviderDetailsText(&tfe.RegistryProvider{
			Name:      "example",
			Namespace: "acme",
		}, true)

		assert.Contains(t, text, "No version information is available for this provider.")
	})
}
