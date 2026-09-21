// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/google/jsonschema-go/jsonschema"
	registryapi "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	log "github.com/sirupsen/logrus"
)

type GetPolicyDetailsArguments struct {
	TerraformPolicyID string `json:"terraform_policy_id"`
}

func GetPolicyDetailsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "get_policy_details",
		Description: "Fetches up-to-date documentation for a specific policy from the Terraform registry. You must call 'search_policies' first to obtain the exact terraform_policy_id required to use this tool.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Fetch detailed Terraform policy documentation using a terraform_policy_id",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    true,
			DestructiveHint: jsonschema.Ptr(false),
		},
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"terraform_policy_id": {
					Type:        "string",
					Description: "Matching terraform_policy_id retrieved from search_policies, e.g. 'policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1'",
				},
			},
			Required: []string{"terraform_policy_id"},
		},
	}
}

func GetPolicyDetailsFunc(ctx context.Context, request *mcp.CallToolRequest, input GetPolicyDetailsArguments) (*mcp.CallToolResult, any, error) {
	logger := log.StandardLogger()

	terraformPolicyID := strings.TrimSpace(input.TerraformPolicyID)
	if terraformPolicyID == "" {
		return nil, nil, fmt.Errorf("terraform_policy_id cannot be empty - use search_policies first to find valid policy IDs")
	}

	httpClient, err := client.GetHttpClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get http client for public Terraform registry: %w", err)
	}

	uri := (&url.URL{
		Path:     terraformPolicyID,
		RawQuery: url.Values{"include": {"policies,policy-modules,policy-library"}}.Encode(),
	}).String()
	policyResponse, err := registryapi.SendRegistryCall(ctx, httpClient, http.MethodGet, uri, logger, "v2")
	if err != nil {
		return nil, nil, fmt.Errorf("policy not found: %s - verify the terraform_policy_id is correct or use search_policies to find valid IDs", terraformPolicyID)
	}

	var policyDetails registryapi.TerraformPolicyDetails
	if err := json.Unmarshal(policyResponse, &policyDetails); err != nil {
		return nil, nil, fmt.Errorf("failed to parse policy details for %s", terraformPolicyID)
	}

	policyData := formatPolicyDetails(policyDetails, terraformPolicyID, logger)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: policyData}},
	}, nil, nil
}

func formatPolicyDetails(policyDetails registryapi.TerraformPolicyDetails, terraformPolicyID string, logger *log.Logger) string {
	readme := utils.ExtractReadme(policyDetails.Data.Attributes.Readme)
	readme = strings.TrimSpace(strings.TrimSuffix(readme, "---"))

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("## Policy details about %s \n\n%s", terraformPolicyID, readme))

	var policyList strings.Builder
	var moduleList strings.Builder
	for _, policy := range policyDetails.Included {
		if policy.Type == "policy-modules" {
			const moduleTemplate = `
module "{{.Name}}" {
	source = "https://registry.terraform.io/v2/{{.PolicyID}}/policy-module/{{.Name}}.sentinel?checksum=sha256:{{.Shasum}}"
}
`
			templateData := struct {
				Name     string
				PolicyID string
				Shasum   string
			}{
				Name:     policy.Attributes.Name,
				PolicyID: terraformPolicyID,
				Shasum:   policy.Attributes.Shasum,
			}
			if err := template.Must(template.New("module").Parse(moduleTemplate)).Execute(&moduleList, templateData); err != nil {
				logger.WithError(err).Error("failed to render module template")
			}
		}

		if policy.Type == "policies" {
			policyList.WriteString(fmt.Sprintf("- POLICY_NAME: %s\n- POLICY_CHECKSUM: sha256:%s\n", policy.Attributes.Name, policy.Attributes.Shasum))
			policyList.WriteString("\n---\n")
		}
	}

	builder.WriteString("---\n\n")
	builder.WriteString("## Usage\n\n")
	builder.WriteString("Add the following configuration to a `sentinel.hcl` file. Create one policy block for each policy listed below using this template.\n")
	builder.WriteString("\n```hcl\n")

	const hclTemplate = `
{{- if .ModuleList }}
{{ .ModuleList }}
{{- end }}
policy "<<POLICY_NAME>>" {
  source = "https://registry.terraform.io/v2{{ .TerraformPolicyID }}/policy/<<POLICY_NAME>>.sentinel?checksum=<<POLICY_CHECKSUM>>"
  enforcement_level = "advisory"
}
`
	templateData := struct {
		ModuleList        string
		TerraformPolicyID string
	}{
		ModuleList:        moduleList.String(),
		TerraformPolicyID: terraformPolicyID,
	}
	var hclBuilder strings.Builder
	if err := template.Must(template.New("hclPolicy").Parse(hclTemplate)).Execute(&hclBuilder, templateData); err != nil {
		logger.WithError(err).Error("failed to render HCL policy template")
	}

	builder.WriteString(hclBuilder.String())
	builder.WriteString("\n```\n")
	builder.WriteString(fmt.Sprintf("Available policies with SHA for %s are: \n\n", terraformPolicyID))
	builder.WriteString(policyList.String())

	return builder.String()
}
