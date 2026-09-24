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

func TestSearchPrivateModulesTool(t *testing.T) {
	tool := SearchPrivateModulesTool()

	assert.Equal(t, "search_private_modules", tool.Name)
	assert.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Search for private modules in Terraform Cloud/Enterprise", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema := inputSchema(t, tool.InputSchema)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)
	assert.Equal(t,
		[]string{"terraform_org_name", "search_query", "page", "pageSize"},
		schema.PropertyOrder,
	)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)
	assert.Contains(t, schema.Properties, "page")
	assert.Contains(t, schema.Properties, "pageSize")
}

func TestSearchPrivateModulesOutputSchema(t *testing.T) {
	schema, ok := SearchPrivateModulesTool().OutputSchema.(*jsonschema.Schema)
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

	noCodeModuleIDs := items.Items.Properties["no_code_module_ids"]
	require.NotNil(t, noCodeModuleIDs)
	assert.Equal(t, "array", noCodeModuleIDs.Type)
	assert.Empty(t, noCodeModuleIDs.Types)
	require.NotNil(t, noCodeModuleIDs.Items)
	assert.Equal(t, "string", noCodeModuleIDs.Items.Type)

	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	results := []PrivateModuleSummaryList{
		{Items: make([]PrivateModuleSummary, 0)},
		{
			Items: []PrivateModuleSummary{{
				PrivateModuleID: "acme/vpc/aws",
				Name:            "vpc",
				Namespace:       "acme",
				Provider:        "aws",
				RegistryName:    "private",
				CreatedAt:       "2026-01-02T03:04:05Z",
				UpdatedAt:       "2026-02-03T04:05:06Z",
				NoCode:          true,
				NoCodeModuleIDs: []string{"nocode-123"},
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

func TestSearchPrivateModulesFuncValidation(t *testing.T) {
	_, _, err := SearchPrivateModulesFunc(t.Context(), nil, SearchPrivateModulesArguments{
		TerraformOrgName: "   ",
	})

	assert.EqualError(t, err, "terraform_org_name must not be blank")
}

func TestPrivateModuleSummaryList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		got := privateModuleSummaryList(&tfe.RegistryModuleList{})

		require.NotNil(t, got.Items)
		assert.Empty(t, got.Items)
		encoded, err := json.Marshal(got)
		require.NoError(t, err)
		assert.JSONEq(t, `{"items":[]}`, string(encoded))
	})

	t.Run("maps modules and pagination", func(t *testing.T) {
		got := privateModuleSummaryList(&tfe.RegistryModuleList{
			Items: []*tfe.RegistryModule{
				{
					Name:         "vpc",
					Namespace:    "acme",
					Provider:     "aws",
					RegistryName: tfe.PrivateRegistry,
					CreatedAt:    "2026-01-02T03:04:05Z",
					UpdatedAt:    "2026-02-03T04:05:06Z",
					NoCode:       true,
					RegistryNoCodeModule: []*tfe.RegistryNoCodeModule{
						{ID: "nocode-123"},
						nil,
						{ID: "nocode-456"},
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
		assert.Equal(t, PrivateModuleSummary{
			PrivateModuleID: "acme/vpc/aws",
			Name:            "vpc",
			Namespace:       "acme",
			Provider:        "aws",
			RegistryName:    "private",
			CreatedAt:       "2026-01-02T03:04:05Z",
			UpdatedAt:       "2026-02-03T04:05:06Z",
			NoCode:          true,
			NoCodeModuleIDs: []string{"nocode-123", "nocode-456"},
		}, got.Items[0])
		assert.Equal(t, PaginationDetails{
			CurrentPage: 1,
			NextPage:    2,
			TotalCount:  3,
			TotalPages:  2,
		}, got.PaginationDetails)
	})

	t.Run("omits no-code relationships for a regular module", func(t *testing.T) {
		got := privateModuleSummaryList(&tfe.RegistryModuleList{
			Items: []*tfe.RegistryModule{
				{
					Name:      "network",
					Namespace: "acme",
					Provider:  "aws",
					RegistryNoCodeModule: []*tfe.RegistryNoCodeModule{
						{ID: "nocode-ignored"},
					},
				},
			},
		})

		require.Len(t, got.Items, 1)
		require.NotNil(t, got.Items[0].NoCodeModuleIDs)
		assert.Empty(t, got.Items[0].NoCodeModuleIDs)
	})
}
