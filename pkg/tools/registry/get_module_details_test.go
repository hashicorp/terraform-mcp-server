// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
)

func unmarshalAndFormatModule(response []byte) (string, error) {
	module, err := unmarshalTerraformModuleDetails(response)
	if err != nil {
		return "", err
	}
	return formatTerraformModule(module), nil
}

// --- UnmarshalModuleSingular ---
func TestUnmarshalModuleSingular_ValidAllFields(t *testing.T) {
	resp := []byte(`{
		"id": "namespace/name/provider/1.0.0",
		"owner": "owner",
		"namespace": "namespace",
		"name": "name",
		"version": "1.0.0",
		"provider": "provider",
		"provider_logo_url": "",
		"description": "A test module",
		"source": "source",
		"tag": "",
		"published_at": "2023-01-01T00:00:00Z",
		"downloads": 1,
		"verified": true,
		"root": {
			"path": "",
			"name": "root",
			"readme": "",
			"empty": false,
			"inputs": [
				{"name": "input1", "type": "string", "description": "desc", "default": "val", "required": true}
			],
			"outputs": [
				{"name": "output1", "description": "desc"}
			],
			"dependencies": [],
			"provider_dependencies": [
				{"name": "prov1", "namespace": "ns", "source": "src", "version": "1.0.0"}
			],
			"resources": []
		},
		"submodules": [],
		"examples": [
			{"path": "", "name": "example1", "readme": "example readme", "empty": false, "inputs": [], "outputs": [], "dependencies": [], "provider_dependencies": [], "resources": []}
		],
		"providers": ["provider"],
		"versions": ["1.0.0"],
		"deprecation": null
	}`)
	out, err := unmarshalAndFormatModule(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "A test module") {
		t.Errorf("expected output to contain module description, got %q", out)
	}
	if !strings.Contains(out, "input1") {
		t.Errorf("expected output to contain input variable, got %q", out)
	}
	if !strings.Contains(out, "example1") {
		t.Errorf("expected output to contain example name, got %q", out)
	}
	if strings.Contains(out, "example readme") {
		t.Errorf("expected output to omit example README details, got %q", out)
	}
}

func TestUnmarshalModuleSingular_ListsSubmodules(t *testing.T) {
	resp := []byte(`{
		"id": "namespace/name/provider/1.0.0",
		"owner": "owner",
		"namespace": "namespace",
		"name": "name",
		"version": "1.0.0",
		"provider": "provider",
		"provider_logo_url": "",
		"description": "A test module",
		"source": "source",
		"tag": "",
		"published_at": "2023-01-01T00:00:00Z",
		"downloads": 1,
		"verified": true,
		"root": {
			"path": "",
			"name": "root",
			"readme": "",
			"empty": false,
			"inputs": [],
			"outputs": [],
			"dependencies": [],
			"provider_dependencies": [],
			"resources": []
		},
		"submodules": [
			{"path": "modules/private", "name": "private", "readme": "private readme", "empty": false, "inputs": [], "outputs": [], "dependencies": [], "provider_dependencies": [], "resources": []}
		],
		"examples": [],
		"providers": ["provider"],
		"versions": ["1.0.0"],
		"deprecation": null
	}`)
	out, err := unmarshalAndFormatModule(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "private") {
		t.Errorf("expected output to contain submodule name, got %q", out)
	}
	if strings.Contains(out, "private readme") {
		t.Errorf("expected output to omit submodule README details, got %q", out)
	}
}

func TestUnmarshalModuleSingular_EmptySections(t *testing.T) {
	resp := []byte(`{
		"id": "namespace/name/provider/1.0.0",
		"owner": "owner",
		"namespace": "namespace",
		"name": "name",
		"version": "1.0.0",
		"provider": "provider",
		"provider_logo_url": "",
		"description": "A test module",
		"source": "source",
		"tag": "",
		"published_at": "2023-01-01T00:00:00Z",
		"downloads": 1,
		"verified": true,
		"root": {
			"path": "",
			"name": "root",
			"readme": "",
			"empty": false,
			"inputs": [],
			"outputs": [],
			"dependencies": [],
			"provider_dependencies": [],
			"resources": []
		},
		"submodules": [],
		"examples": [],
		"providers": ["provider"],
		"versions": ["1.0.0"],
		"deprecation": null
	}`)
	out, err := unmarshalAndFormatModule(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "A test module") {
		t.Errorf("expected output to contain description, got %q", out)
	}
}

func TestUnmarshalModuleSingular_InvalidJSON(t *testing.T) {
	resp := []byte(`not a json`)
	_, err := unmarshalAndFormatModule(resp)
	if err == nil || !strings.Contains(err.Error(), "unmarshalling module details") {
		t.Errorf("expected unmarshalling error, got %v", err)
	}
}

func TestFormatTerraformModulePart_ListExamples(t *testing.T) {
	module := testTerraformModuleDetails()

	out, err := formatTerraformModulePart(module, moduleExamplesKind, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "example1") {
		t.Errorf("expected output to contain example name, got %q", out)
	}
	if strings.Contains(out, "example readme") {
		t.Errorf("expected output to omit example README details from list view, got %q", out)
	}
	if !strings.Contains(out, "get_module_examples") {
		t.Errorf("expected output to explain how to fetch one example, got %q", out)
	}
}

func TestFormatTerraformModulePart_SelectedExample(t *testing.T) {
	module := testTerraformModuleDetails()

	out, err := formatTerraformModulePart(module, moduleExamplesKind, "example1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "example readme") {
		t.Errorf("expected output to contain selected example README, got %q", out)
	}
	if !strings.Contains(out, "example_input") {
		t.Errorf("expected output to contain selected example inputs, got %q", out)
	}
}

func TestFormatTerraformModulePart_SelectedSubmodule(t *testing.T) {
	module := testTerraformModuleDetails()

	out, err := formatTerraformModulePart(module, moduleSubmodulesKind, "private")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "private readme") {
		t.Errorf("expected output to contain selected submodule README, got %q", out)
	}
	if !strings.Contains(out, "submodule_input") {
		t.Errorf("expected output to contain selected submodule inputs, got %q", out)
	}
}

func TestFormatTerraformModulePart_PathMatchIsCaseSensitive(t *testing.T) {
	module := client.TerraformModuleVersionDetails{
		Namespace: "namespace",
		Name:      "name",
		Version:   "1.0.0",
		Provider:  "provider",
		Examples: []client.ModulePart{
			{Path: "examples/Private", Name: "private-upper"},
			{Path: "examples/private", Name: "private-lower"},
		},
	}

	out, err := formatTerraformModulePart(module, moduleExamplesKind, "examples/private")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "private-lower") {
		t.Errorf("expected case-sensitive path match to select 'private-lower', got %q", out)
	}
	if strings.Contains(out, "private-upper") {
		t.Errorf("expected case-sensitive path match to NOT select 'private-upper', got %q", out)
	}
}

func TestFormatTerraformModulePart_SelectedPartNotFound(t *testing.T) {
	module := testTerraformModuleDetails()

	_, err := formatTerraformModulePart(module, moduleExamplesKind, "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "Available examples: example1") {
		t.Errorf("expected error to list available examples, got %q", err.Error())
	}
}

func TestFormatTerraformModulePart_ListEmpty(t *testing.T) {
	module := client.TerraformModuleVersionDetails{
		Namespace: "namespace",
		Name:      "name",
		Version:   "1.0.0",
		Provider:  "provider",
		Examples:  nil,
	}

	out, err := formatTerraformModulePart(module, moduleExamplesKind, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "No examples found for this module.") {
		t.Errorf("expected empty-list message, got %q", out)
	}
	if strings.Contains(out, "To fetch one example") {
		t.Errorf("expected empty-list output to omit fetch instructions, got %q", out)
	}
}

func TestEscapeMarkdownTableCell(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"plain text", "plain text"},
		{"a|b", `a\|b`},
		{"line1\nline2", "line1 line2"},
		{"line1\r\nline2", "line1 line2"},
		{"a|b\nc", `a\|b c`},
	}
	for _, c := range cases {
		if got := escapeMarkdownTableCell(c.in); got != c.want {
			t.Errorf("escapeMarkdownTableCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- ValidateModuleID ---
func TestValidateModuleID_ValidFormat(t *testing.T) {
	validIDs := []string{
		"hashicorp/consul/aws/0.1.0",
		"terraform-aws-modules/vpc/aws/3.14.0",
		"namespace/name/provider/1.0.0",
	}

	for _, id := range validIDs {
		err := validateModuleID(id)
		if err != nil {
			t.Errorf("expected no error for valid module ID %q, got %v", id, err)
		}
	}
}

func TestValidateModuleID_InvalidFormat(t *testing.T) {
	testCases := []struct {
		moduleID string
		name     string
	}{
		{"", "empty string"},
		{"hashicorp", "single part"},
		{"hashicorp/consul", "two parts"},
		{"hashicorp/consul/aws", "three parts"},
		{"hashicorp/consul/aws/1.0.0/extra", "five parts"},
	}

	for _, tc := range testCases {
		err := validateModuleID(tc.moduleID)
		if err == nil {
			t.Errorf("expected error for %s (%q), got nil", tc.name, tc.moduleID)
		}
		if !strings.Contains(err.Error(), "invalid module ID format") {
			t.Errorf("expected error message to contain 'invalid module ID format', got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "Expected format: namespace/name/provider/version (4 parts)") {
			t.Errorf("expected error message to contain format hint, got %q", err.Error())
		}
	}
}

func testTerraformModuleDetails() client.TerraformModuleVersionDetails {
	return client.TerraformModuleVersionDetails{
		ID:          "namespace/name/provider/1.0.0",
		Namespace:   "namespace",
		Name:        "name",
		Version:     "1.0.0",
		Provider:    "provider",
		Description: "A test module",
		Source:      "source",
		Root: client.ModulePart{
			Name: "root",
		},
		Examples: []client.ModulePart{
			{
				Path:   "examples/example1",
				Name:   "example1",
				Readme: "example readme",
				Inputs: []client.ModuleInput{
					{Name: "example_input", Type: "string", Description: "desc", Default: "value", Required: false},
				},
			},
		},
		Submodules: []client.ModulePart{
			{
				Path:   "modules/private",
				Name:   "private",
				Readme: "private readme",
				Inputs: []client.ModuleInput{
					{Name: "submodule_input", Type: "string", Description: "desc", Default: nil, Required: true},
				},
			},
		},
	}
}
