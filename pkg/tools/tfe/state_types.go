// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

// terraformState represents a Terraform state file (v4 format), as downloaded via
// StateVersions.Download. See https://developer.hashicorp.com/terraform/internals/json-format
// for the schema this models.
type terraformState struct {
	Version          int                    `json:"version"`
	TerraformVersion string                 `json:"terraform_version"`
	Serial           int64                  `json:"serial"`
	Lineage          string                 `json:"lineage"`
	Outputs          map[string]stateOutput `json:"outputs"`
	Resources        []stateResource        `json:"resources"`
}

// stateOutput represents a single Terraform output value.
type stateOutput struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

// stateResource represents a resource block in Terraform state.
type stateResource struct {
	Mode      string          `json:"mode"`
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Module    string          `json:"module"`
	Provider  string          `json:"provider"`
	Instances []stateInstance `json:"instances"`
}

// stateInstance represents one instance of a resource.
type stateInstance struct {
	IndexKey            interface{}            `json:"index_key"`
	Attributes          map[string]interface{} `json:"attributes"`
	SensitiveAttributes []interface{}          `json:"sensitive_attributes"`
	Dependencies        []string               `json:"dependencies"`
}

// extractedResource is a flattened, redacted resource instance with a computed address.
type extractedResource struct {
	Address             string                 `json:"address"`
	Type                string                 `json:"type"`
	Name                string                 `json:"name"`
	Module              string                 `json:"module"`
	Mode                string                 `json:"mode"`
	Provider            string                 `json:"provider"`
	Attributes          map[string]interface{} `json:"attributes"`
	SensitiveAttributes []interface{}          `json:"sensitive_attributes"`
	Dependencies        []string               `json:"dependencies"`
}
