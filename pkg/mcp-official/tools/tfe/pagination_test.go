// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputSchema asserts the tool carries an explicit object schema and returns it.
func inputSchema(t *testing.T, schema any) *jsonschema.Schema {
	t.Helper()

	s, ok := schema.(*jsonschema.Schema)
	require.True(t, ok, "input schema should be a *jsonschema.Schema, got %T", schema)
	require.Equal(t, "object", s.Type)
	return s
}

func TestPaginationListOptions(t *testing.T) {
	tests := []struct {
		name  string
		input Pagination
		want  tfe.ListOptions
	}{
		{name: "unset", input: Pagination{}, want: tfe.ListOptions{PageNumber: 1, PageSize: 30}},
		{name: "negative page", input: Pagination{Page: -3}, want: tfe.ListOptions{PageNumber: 1, PageSize: 30}},
		{name: "page size below min", input: Pagination{Page: 2, PageSize: 0}, want: tfe.ListOptions{PageNumber: 2, PageSize: 30}},
		{name: "page size above max", input: Pagination{Page: 2, PageSize: 101}, want: tfe.ListOptions{PageNumber: 2, PageSize: 30}},
		{name: "within range", input: Pagination{Page: 3, PageSize: 100}, want: tfe.ListOptions{PageNumber: 3, PageSize: 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.input.ListOptions())
		})
	}
}

func TestPaginationDetails(t *testing.T) {
	t.Run("nil pagination", func(t *testing.T) {
		assert.Equal(t, PaginationDetails{}, paginationDetails(nil))
	})

	t.Run("maps every field", func(t *testing.T) {
		details := paginationDetails(&tfe.Pagination{
			CurrentPage:  2,
			PreviousPage: 1,
			NextPage:     3,
			TotalCount:   45,
			TotalPages:   5,
		})

		assert.Equal(t, PaginationDetails{
			CurrentPage:  2,
			PreviousPage: 1,
			NextPage:     3,
			TotalCount:   45,
			TotalPages:   5,
		}, details)
	})
}

func TestPaginationSchemaProperties(t *testing.T) {
	properties := paginationSchemaProperties()

	page := properties["page"]
	require.NotNil(t, page)
	assert.Equal(t, "integer", page.Type)
	require.NotNil(t, page.Minimum)
	assert.Equal(t, float64(defaultPage), *page.Minimum)

	pageSize := properties["pageSize"]
	require.NotNil(t, pageSize)
	assert.Equal(t, "integer", pageSize.Type)
	require.NotNil(t, pageSize.Minimum)
	assert.Equal(t, float64(minPageSize), *pageSize.Minimum)
	require.NotNil(t, pageSize.Maximum)
	assert.Equal(t, float64(maxPageSize), *pageSize.Maximum)
}

func TestPaginationSchemaPropertiesAreIndependent(t *testing.T) {
	properties := paginationSchemaProperties()
	properties["terraform_org_name"] = &jsonschema.Schema{Type: "string"}

	assert.NotContains(t, paginationSchemaProperties(), "terraform_org_name")
}
