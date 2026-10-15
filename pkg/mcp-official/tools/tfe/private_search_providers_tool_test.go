// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchPrivateProvidersTool(t *testing.T) {
	tool := SearchPrivateProvidersTool()

	assert.Equal(t, "search_private_providers", tool.Name)
	assert.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Search for private providers in Terraform Cloud/Enterprise", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema := inputSchema(t, tool.InputSchema)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)
	assert.Equal(t,
		[]string{"terraform_org_name", "search_query", "registry_name", "page", "pageSize"},
		schema.PropertyOrder,
	)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	registryName := schema.Properties["registry_name"]
	require.NotNil(t, registryName)
	assert.Equal(t, []any{"private", "public"}, registryName.Enum)
	assert.JSONEq(t, `"private"`, string(registryName.Default))
	assert.Contains(t, schema.Properties, "page")
	assert.Contains(t, schema.Properties, "pageSize")
}

func TestSearchPrivateProvidersOutputSchema(t *testing.T) {
	schema, ok := SearchPrivateProvidersTool().OutputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "object", schema.Type)
	assert.Equal(t,
		[]string{"items", "current-page", "prev-page", "next-page", "total-count", "total-pages"},
		schema.PropertyOrder,
	)
	assert.Equal(t, []string{"items"}, schema.Required)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	items := schema.Properties["items"]
	require.NotNil(t, items)
	assert.Equal(t, "array", items.Type)
	assert.Empty(t, items.Types)
	require.NotNil(t, items.Items)
	assert.Equal(t, "object", items.Items.Type)
	require.NotNil(t, items.Items.AdditionalProperties)
	assert.NotNil(t, items.Items.AdditionalProperties.Not)

	versions := items.Items.Properties["versions"]
	require.NotNil(t, versions)
	assert.Equal(t, "array", versions.Type)
	assert.Empty(t, versions.Types)
	require.NotNil(t, versions.Items)
	assert.Equal(t, "string", versions.Items.Type)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []PrivateProviderSummaryList{
		{Items: make([]PrivateProviderSummary, 0)},
		{
			Items: []PrivateProviderSummary{{
				ID:              "prov-123",
				ProviderAddress: "acme/example",
				Name:            "example",
				Namespace:       "acme",
				RegistryName:    "private",
				CreatedAt:       "2026-01-02T03:04:05Z",
				UpdatedAt:       "2026-02-03T04:05:06Z",
				Versions:        []string{"2.0.0"},
			}},
			PaginationDetails: PaginationDetails{CurrentPage: 1, TotalCount: 1, TotalPages: 1},
		},
	}
	for _, result := range results {
		data, err := json.Marshal(result)
		require.NoError(t, err)
		var value any
		require.NoError(t, json.Unmarshal(data, &value))
		assert.NoError(t, resolved.Validate(&value))
	}
}

func TestSearchPrivateProvidersFuncValidation(t *testing.T) {
	tests := []struct {
		name  string
		input SearchPrivateProvidersArguments
		want  string
	}{
		{
			name:  "blank organization",
			input: SearchPrivateProvidersArguments{TerraformOrgName: "   "},
			want:  "terraform_org_name must not be blank",
		},
		{
			name: "invalid registry",
			input: SearchPrivateProvidersArguments{
				TerraformOrgName: "example-org",
				RegistryName:     "partner",
			},
			want: `registry_name must be one of ["private" "public"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := SearchPrivateProvidersFunc(t.Context(), nil, tt.input)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestPrivateProviderSummaryList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		got := privateProviderSummaryList(&tfe.RegistryProviderList{})

		require.NotNil(t, got.Items)
		assert.Empty(t, got.Items)
		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":[]}`, string(encoded))
	})

	t.Run("maps providers and pagination", func(t *testing.T) {
		got := privateProviderSummaryList(&tfe.RegistryProviderList{
			Items: []*tfe.RegistryProvider{
				{
					ID:           "prov-123",
					Name:         "example",
					Namespace:    "acme",
					RegistryName: tfe.PrivateRegistry,
					CreatedAt:    "2026-01-02T03:04:05Z",
					UpdatedAt:    "2026-02-03T04:05:06Z",
					RegistryProviderVersions: []*tfe.RegistryProviderVersion{
						{Version: "2.0.0"},
						nil,
						{Version: "1.0.0"},
					},
				},
			},
			Pagination: &tfe.Pagination{
				CurrentPage: 1,
				NextPage:    2,
				TotalCount:  3,
				TotalPages:  2,
			},
		})

		require.Len(t, got.Items, 1)
		assert.Equal(t, PrivateProviderSummary{
			ID:              "prov-123",
			ProviderAddress: "acme/example",
			Name:            "example",
			Namespace:       "acme",
			RegistryName:    "private",
			CreatedAt:       "2026-01-02T03:04:05Z",
			UpdatedAt:       "2026-02-03T04:05:06Z",
			Versions:        []string{"2.0.0", "1.0.0"},
		}, got.Items[0])
		assert.Equal(t, PaginationDetails{
			CurrentPage: 1,
			NextPage:    2,
			TotalCount:  3,
			TotalPages:  2,
		}, got.PaginationDetails)
	})
}
