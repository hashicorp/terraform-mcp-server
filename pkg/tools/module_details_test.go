// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"strings"
	"testing"
)

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
	out, err := unmarshalTerraformModule(resp)
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
	out, err := unmarshalTerraformModule(resp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out, "A test module") {
		t.Errorf("expected output to contain description, got %q", out)
	}
}

func TestUnmarshalModuleSingular_InvalidJSON(t *testing.T) {
	resp := []byte(`not a json`)
	_, err := unmarshalTerraformModule(resp)
	if err == nil || !strings.Contains(err.Error(), "unmarshalling module details") {
		t.Errorf("expected unmarshalling error, got %v", err)
	}
}
