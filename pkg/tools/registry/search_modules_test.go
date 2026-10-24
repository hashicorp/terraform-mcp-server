// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"
)

func TestUnmarshalTerraformModules_ValidModules(t *testing.T) {
	resp := []byte(`{
		"meta": {
			"limit": 15,
			"current_offset": 0
		},
		"modules": [
			{
				"id": "hashicorp/consul/aws/0.1.0",
				"owner": "hashicorp",
				"namespace": "hashicorp",
				"name": "consul",
				"version": "0.1.0",
				"provider": "aws",
				"description": "Consul module",
				"source": "https://github.com/hashicorp/terraform-aws-consul",
				"tag": "0.1.0",
				"published_at": "2023-01-01T00:00:00Z",
				"downloads": 50,
				"verified": true
			},
			{
				"id": "terraform-aws-modules/vpc/aws/3.14.0",
				"owner": "terraform-aws-modules",
				"namespace": "terraform-aws-modules",
				"name": "vpc",
				"version": "3.14.0",
				"provider": "aws",
				"description": "VPC module",
				"source": "https://github.com/terraform-aws-modules/terraform-aws-vpc",
				"tag": "v3.14.0",
				"published_at": "2023-01-02T00:00:00Z",
				"downloads": 200,
				"verified": true
			}
		]
	}`)

	out, err := unmarshalTerraformModules(resp, "aws", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(out, "Available Terraform Modules (top matches) for aws") {
		t.Errorf("expected header with query, got %q", out)
	}
	if !strings.Contains(out, "terraform-aws-modules/vpc/aws/3.14.0") {
		t.Errorf("expected vpc module in output, got %q", out)
	}
	if !strings.Contains(out, "hashicorp/consul/aws/0.1.0") {
		t.Errorf("expected consul module in output, got %q", out)
	}

	// Verify modules are sorted by downloads descending: vpc (200) should appear before consul (50)
	vpcIndex := strings.Index(out, "terraform-aws-modules/vpc/aws/3.14.0")
	consulIndex := strings.Index(out, "hashicorp/consul/aws/0.1.0")
	if vpcIndex == -1 || consulIndex == -1 || vpcIndex > consulIndex {
		t.Errorf("expected higher download module to appear before lower download module, vpcIndex=%d consulIndex=%d", vpcIndex, consulIndex)
	}
}

func TestUnmarshalTerraformModules_EmptyModules(t *testing.T) {
	resp := []byte(`{
		"meta": {
			"limit": 15,
			"current_offset": 0
		},
		"modules": []
	}`)

	out, err := unmarshalTerraformModules(resp, "unknown", nil)
	if err != nil {
		t.Fatalf("expected no error for empty modules array, got %v", err)
	}
	if out != "" {
		t.Errorf("expected empty string output for empty modules, got %q", out)
	}
}

func TestUnmarshalTerraformModules_EmptyObject(t *testing.T) {
	resp := []byte(`{}`)

	out, err := unmarshalTerraformModules(resp, "unknown", nil)
	if err != nil {
		t.Fatalf("expected no error for empty object, got %v", err)
	}
	if out != "" {
		t.Errorf("expected empty string output for empty object, got %q", out)
	}
}

func TestUnmarshalTerraformModules_InvalidJSON(t *testing.T) {
	resp := []byte(`not a json`)

	_, err := unmarshalTerraformModules(resp, "unknown", nil)
	if err == nil {
		t.Fatal("expected unmarshalling error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshalling modules") {
		t.Errorf("expected error message to contain 'unmarshalling modules', got %q", err.Error())
	}
}

func TestSearchModulesTool_Definition(t *testing.T) {
	serverTool := SearchModules(nil)
	if serverTool.Tool.Name != "search_modules" {
		t.Errorf("expected tool name 'search_modules', got %q", serverTool.Tool.Name)
	}
}
