// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"

	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPrivateModuleDetailsTool(t *testing.T) {
	tool := GetPrivateModuleDetailsTool()

	assert.Equal(t, "get_private_module_details", tool.Name)
	assert.Contains(t, tool.Description, "submodule")
	require.NotNil(t, tool.Annotations)
	assert.Equal(t, "Get detailed information about a private module", tool.Annotations.Title)
	assert.True(t, tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Annotations.OpenWorldHint)
	require.NotNil(t, tool.Annotations.DestructiveHint)
	assert.False(t, *tool.Annotations.DestructiveHint)

	schema := inputSchema(t, tool.InputSchema)
	assert.Equal(t, []string{"terraform_org_name", "private_module_id"}, schema.Required)
	assert.Equal(t,
		[]string{"terraform_org_name", "private_module_id", "registry_name", "private_module_version"},
		schema.PropertyOrder,
	)
	require.NotNil(t, schema.AdditionalProperties)
	assert.NotNil(t, schema.AdditionalProperties.Not)

	moduleID := schema.Properties["private_module_id"]
	require.NotNil(t, moduleID)
	assert.Contains(t, moduleID.Examples, "my-tfc-org/vpc/aws")

	registryName := schema.Properties["registry_name"]
	require.NotNil(t, registryName)
	assert.Equal(t, enumOf(validRegistries...), registryName.Enum)
	assert.JSONEq(t, `"private"`, string(registryName.Default))
}

func TestGetPrivateModuleDetailsFuncValidation(t *testing.T) {
	tests := []struct {
		name  string
		input GetPrivateModuleDetailsArguments
		want  string
	}{
		{
			name:  "blank organization",
			input: GetPrivateModuleDetailsArguments{PrivateModuleID: "acme/vpc/aws"},
			want:  "terraform_org_name must not be blank",
		},
		{
			name:  "blank module ID",
			input: GetPrivateModuleDetailsArguments{TerraformOrgName: "acme", PrivateModuleID: "   "},
			want:  "private_module_id must not be blank",
		},
		{
			name:  "too few module ID parts",
			input: GetPrivateModuleDetailsArguments{TerraformOrgName: "acme", PrivateModuleID: "acme/vpc"},
			want:  "private_module_id must be in namespace/name/provider format",
		},
		{
			name:  "blank module ID part",
			input: GetPrivateModuleDetailsArguments{TerraformOrgName: "acme", PrivateModuleID: "acme//aws"},
			want:  "private_module_id must be in namespace/name/provider format",
		},
		{
			name: "invalid registry",
			input: GetPrivateModuleDetailsArguments{
				TerraformOrgName: "acme",
				PrivateModuleID:  "acme/vpc/aws",
				RegistryName:     "partner",
			},
			want: `registry_name must be one of ["private" "public"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GetPrivateModuleDetailsFunc(t.Context(), nil, tt.input)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestTerraformRegistryModulePath(t *testing.T) {
	t.Run("private latest version", func(t *testing.T) {
		got := terraformRegistryModulePath(tfe.RegistryModuleID{
			Namespace:    "acme",
			Name:         "vpc",
			Provider:     "aws",
			RegistryName: tfe.PrivateRegistry,
		}, "")

		assert.Equal(t, "/api/registry/v1/modules/acme/vpc/aws", got)
		assert.NotEqual(t, '/', got[len(got)-1])
	})

	t.Run("public explicit version escapes segments", func(t *testing.T) {
		got := terraformRegistryModulePath(tfe.RegistryModuleID{
			Namespace:    "acme corp",
			Name:         "vpc?",
			Provider:     "google beta",
			RegistryName: tfe.PublicRegistry,
		}, "1.0.0 rc1")

		assert.Equal(t,
			"/api/registry/public/v1/modules/acme%20corp/vpc%3F/google%20beta/1.0.0%20rc1",
			got,
		)
	})
}

func TestPrivateModuleDetailsText(t *testing.T) {
	registryModule := &tfe.RegistryModule{
		Name:         "vpc",
		Namespace:    "acme",
		Provider:     "aws",
		RegistryName: tfe.PrivateRegistry,
		CreatedAt:    "2026-01-02T03:04:05Z",
		UpdatedAt:    "2026-02-03T04:05:06Z",
		Organization: &tfe.Organization{Name: "acme"},
		Permissions: &tfe.RegistryModulePermissions{
			CanDelete: true,
			CanResync: true,
			CanRetry:  true,
		},
		VCSRepo: &tfe.VCSRepo{
			Identifier:        "acme/vpc",
			DisplayIdentifier: "acme/vpc",
			Branch:            "main",
			IngressSubmodules: true,
			RepositoryHTTPURL: "https://example.com/acme/vpc",
			ServiceProvider:   "github",
		},
	}
	registryDetails := &tfeclient.TerraformModuleVersionDetails{
		Version:     "1.2.3",
		Description: "VPC module",
		Root: tfeclient.ModulePart{
			Readme: "# Root README\n\nRoot documentation.\n\n## Inputs\nGenerated input docs.",
			Inputs: []tfeclient.ModuleInput{
				{Name: "region", Type: "string", Description: "AWS region", Default: "us-east-1"},
			},
			Outputs: []tfeclient.ModuleOutput{
				{Name: "vpc_id", Description: "VPC ID"},
			},
			Dependencies: []tfeclient.ModuleDependency{
				{Name: "labels", Source: "acme/labels/aws", Version: "~> 1.0"},
			},
		},
		Submodules: []tfeclient.ModulePart{
			{
				Name:   "child",
				Path:   "modules/child",
				Readme: "# Child README\n\nChild documentation.",
				Inputs: []tfeclient.ModuleInput{
					{Name: "message", Type: "string", Description: "Child message", Required: true},
				},
				Outputs: []tfeclient.ModuleOutput{
					{Name: "result", Description: "Child result"},
				},
				Dependencies: []tfeclient.ModuleDependency{
					{Name: "child_dep", Source: "acme/child/aws", Version: "1.0.0"},
				},
				ProviderDependencies: []tfeclient.ModuleProviderDependency{
					{Name: "random", Namespace: "hashicorp", Source: "hashicorp/random", Version: ">= 3.0"},
				},
				Resources: []tfeclient.ModuleResource{
					{Name: "random_pet.child", Type: "random_pet"},
				},
			},
		},
	}

	text := privateModuleDetailsText(registryModule, registryDetails, "app.terraform.io")

	assert.Contains(t, text, `source = "app.terraform.io/acme/vpc/aws"`)
	assert.Contains(t, text, `version = "1.2.3"`)
	assert.Contains(t, text, "- Version: 1.2.3")
	assert.Contains(t, text, "- Description: VPC module")
	assert.Contains(t, text, "Root Module:")
	assert.Contains(t, text, "| region | string | AWS region | `us-east-1` | false |")
	assert.Contains(t, text, "| vpc_id | VPC ID |")
	assert.Contains(t, text, "| labels | acme/labels/aws | ~> 1.0 |")
	assert.Contains(t, text, "# Root README")
	assert.NotContains(t, text, "Generated input docs.")
	assert.Contains(t, text, "Submodule: child")
	assert.Contains(t, text, "- Path: modules/child")
	assert.Contains(t, text, "| message | string | Child message | `` | true |")
	assert.Contains(t, text, "| result | Child result |")
	assert.Contains(t, text, "| child_dep | acme/child/aws | 1.0.0 |")
	assert.Contains(t, text, "| random | hashicorp | hashicorp/random | >= 3.0 |")
	assert.Contains(t, text, "| random_pet.child | random_pet |")
	assert.Contains(t, text, "# Child README")
	assert.Contains(t, text, "Organization:\n- Name: acme")
	assert.Contains(t, text, "- Can Resync: true")
	assert.Contains(t, text, "- Ingress Submodules: Yes")
	assert.Contains(t, text, "- Repository URL: https://example.com/acme/vpc")
}

func TestPrivateModuleDetailsTextWithoutRegistryDetails(t *testing.T) {
	text := privateModuleDetailsText(&tfe.RegistryModule{
		Name:         "vpc",
		Namespace:    "acme",
		Provider:     "aws",
		RegistryName: tfe.PrivateRegistry,
	}, nil, "app.terraform.io")

	assert.Contains(t, text, `source = "app.terraform.io/acme/vpc/aws"`)
	assert.NotContains(t, text, "version =")
	assert.NotContains(t, text, "Root Module:")
	assert.NotContains(t, text, "Submodule:")
}

func TestRemoveReadmeSections(t *testing.T) {
	readme := "# Module\n\nKeep this.\n\n## Inputs\nRemove input docs.\n\n### Outputs\nRemove output docs.\n\n## Usage\nKeep usage."

	assert.Equal(t, "# Module\n\nKeep this.\n\n## Usage\nKeep usage.", removeReadmeSections(readme))
}

func TestFormatModuleInputDefault(t *testing.T) {
	assert.Empty(t, formatModuleInputDefault(nil))
	assert.Equal(t, "false", formatModuleInputDefault(false))
	assert.Equal(t, "3", formatModuleInputDefault(3))
}
