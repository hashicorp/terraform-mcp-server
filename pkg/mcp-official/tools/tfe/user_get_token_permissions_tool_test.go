// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokenPermissionsTool(t *testing.T) {
	tool := GetTokenPermissionsTool()

	assert.Equal(t, "get_token_permissions", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Nil(t, tool.InputSchema, "the schema is inferred from GetTokenPermissionsArguments")

	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get permissions for current token", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
}

// The tool relies on an inferred schema, so the struct tags are the public
// contract: dropping the jsonschema tag or adding omitempty would silently
// change what clients are told to send.
func TestGetTokenPermissionsArgumentsSchema(t *testing.T) {
	schema, err := jsonschema.For[GetTokenPermissionsArguments](nil)
	require.NoError(t, err)

	assert.Equal(t, "object", schema.Type)
	assert.Equal(t, []string{"terraform_org_name"}, schema.Required)

	orgName := schema.Properties["terraform_org_name"]
	require.NotNil(t, orgName)
	assert.Equal(t, "string", orgName.Type)
	assert.Equal(t, "The name of the Terraform Cloud/Enterprise organization", orgName.Description)
}

func TestGetTokenPermissionsFunc_RejectsBlankOrgName(t *testing.T) {
	tests := []struct {
		name             string
		terraformOrgName string
	}{
		{name: "empty", terraformOrgName: ""},
		{name: "spaces", terraformOrgName: "   "},
		{name: "tab and newline", terraformOrgName: "\t\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Validation runs before the client is acquired, so a nil request is enough.
			result, permissions, err := GetTokenPermissionsFunc(t.Context(), nil, GetTokenPermissionsArguments{
				TerraformOrgName: test.terraformOrgName,
			})

			require.Error(t, err)
			assert.ErrorContains(t, err, "terraform_org_name must not be blank")
			assert.Nil(t, result)
			assert.Nil(t, permissions)
		})
	}
}
