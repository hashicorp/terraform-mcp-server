// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func PolicyDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_policy_details",
			mcp.WithDescription(`Fetches detailed policy information from either the public Terraform Registry or TFE/TFC.
- For public registry policy sets: Use terraform_policy_id from 'search_policies' (e.g., 'policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1')
- For TFE/TFC individual policies: Use policy_id (e.g., 'pol-u3S5p2Uwk21keu1s')
The tool automatically detects the ID format and routes to the appropriate backend.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform policy documentation from Registry or TFE/TFC"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("terraform_policy_id",
				mcp.Required(),
				mcp.Description("Policy identifier: either terraform_policy_id from 'search_policies' (e.g., 'policies/hashicorp/...') or TFE policy_id (e.g., 'pol-...')"),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPolicyDetailsHandler(ctx, request, logger)
		},
	}
}

// getTFEPolicyDetails fetches individual policy details from TFE/TFC
func getTFEPolicyDetails(ctx context.Context, policyID string, logger *log.Logger) (*mcp.CallToolResult, error) {
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client - TFE/TFC authentication required for policy IDs starting with 'pol-'", err)
	}

	// Read the policy from TFE
	policy, err := tfeClient.Policies.Read(ctx, policyID)
	if err != nil {
		return ToolErrorf(logger, "failed to read policy '%s' from TFE/TFC: %v", policyID, err)
	}

	// Build response with policy details
	enforcementLevel := string(policy.EnforcementLevel)
	query := ""
	if policy.Query != nil {
		query = *policy.Query
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("## TFE/TFC Policy Details: %s\n\n", policy.Name))
	builder.WriteString(fmt.Sprintf("**ID**: %s\n", policy.ID))
	builder.WriteString(fmt.Sprintf("**Name**: %s\n", policy.Name))
	if policy.Description != "" {
		builder.WriteString(fmt.Sprintf("**Description**: %s\n", policy.Description))
	}
	builder.WriteString(fmt.Sprintf("**Kind**: %s\n", string(policy.Kind)))
	builder.WriteString(fmt.Sprintf("**Enforcement Level**: %s\n", enforcementLevel))
	builder.WriteString(fmt.Sprintf("**Policy Set Count**: %d\n", policy.PolicySetCount))
	if query != "" {
		builder.WriteString(fmt.Sprintf("**Query**: %s\n", query))
	}
	builder.WriteString(fmt.Sprintf("**Updated At**: %s\n", policy.UpdatedAt.Format("2006-01-02T15:04:05Z")))

	return mcp.NewToolResultText(builder.String()), nil
}

func getPolicyDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	terraformPolicyID, err := request.RequireString("terraform_policy_id")
	if err != nil {
		return ToolError(logger, "missing required input: terraform_policy_id", err)
	}
	if terraformPolicyID == "" {
		return ToolError(logger, "terraform_policy_id cannot be empty", nil)
	}

	// Auto-detect ID type: TFE policy IDs start with "pol-", registry IDs start with "policies/"
	if strings.HasPrefix(terraformPolicyID, "pol-") {
		// This is a TFE/TFC policy ID - fetch from TFE
		return getTFEPolicyDetails(ctx, terraformPolicyID, logger)
	}

	// Default to public registry for policy sets
	httpClient, err := client.GetHttpClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get http client for public Terraform registry", err)
	}

	policyResp, err := client.SendRegistryCall(httpClient, "GET", (&url.URL{Path: terraformPolicyID, RawQuery: url.Values{"include": {"policies,policy-modules,policy-library"}}.Encode()}).String(), logger, "v2")
	if err != nil {
		return ToolErrorf(logger, "policy not found: %s - verify the terraform_policy_id is correct or use search_policies to find valid IDs", terraformPolicyID)
	}

	var policyDetails client.TerraformPolicyDetails
	if err := json.Unmarshal(policyResp, &policyDetails); err != nil {
		return ToolErrorf(logger, "failed to parse policy details for %s", terraformPolicyID)
	}

	readme := utils.ExtractReadme(policyDetails.Data.Attributes.Readme)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("## Policy details about %s \n\n%s", terraformPolicyID, readme))
	policyList := ""
	moduleList := ""
	for _, policy := range policyDetails.Included {
		if policy.Type == "policy-modules" {
			var moduleBuilder strings.Builder
			tmpl := `
module "{{.Name}}" {
	source = "https://registry.terraform.io/v2{{.PolicyID}}/policy-module/{{.Name}}.sentinel?checksum=sha256:{{.Shasum}}"
}
`
			type moduleData struct {
				Name     string
				PolicyID string
				Shasum   string
			}
			t := template.Must(template.New("module").Parse(tmpl))
			err := t.Execute(&moduleBuilder, moduleData{
				Name:     policy.Attributes.Name,
				PolicyID: terraformPolicyID,
				Shasum:   policy.Attributes.Shasum,
			})
			if err != nil {
				logger.WithError(err).Error("failed to render module template")
			}
			moduleList += moduleBuilder.String()
		}

		if policy.Type == "policies" {
			policyList += fmt.Sprintf("- POLICY_NAME: %s\n- POLICY_CHECKSUM: sha256:%s\n", policy.Attributes.Name, policy.Attributes.Shasum)
			policyList += "\n---\n"
		}
	}
	builder.WriteString("---\n")
	builder.WriteString("## Usage\n\n")
	builder.WriteString("Generate the content for a HashiCorp Configuration Language (HCL) file named policies.hcl. This file should define a set of policies. For each policy provided, create a distinct policy block using the following template.\n")
	builder.WriteString("\n```hcl\n")
	hclTmpl := `
{{- if .ModuleList }}
{{ .ModuleList }}
{{- end }}
policy "<<POLICY_NAME>>" {
  source = "https://registry.terraform.io/v2{{ .TerraformPolicyID }}/policy/<<POLICY_NAME>>.sentinel?checksum=<<POLICY_CHECKSUM>>"
  enforcement_level = "advisory"
}
`
	type hclTemplateData struct {
		ModuleList        string
		TerraformPolicyID string
	}
	var hclBuilder strings.Builder
	t := template.Must(template.New("hclPolicy").Parse(hclTmpl))
	err = t.Execute(&hclBuilder, hclTemplateData{
		ModuleList:        moduleList,
		TerraformPolicyID: terraformPolicyID,
	})
	if err != nil {
		logger.WithError(err).Error("failed to render HCL policy template")
	}
	hclTemplate := hclBuilder.String()
	builder.WriteString(hclTemplate)
	builder.WriteString("\n```\n")
	builder.WriteString(fmt.Sprintf("Available policies with SHA for %s are: \n\n", terraformPolicyID))
	builder.WriteString(policyList)

	policyData := builder.String()
	return mcp.NewToolResultText(policyData), nil
}
