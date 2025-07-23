// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	log "github.com/sirupsen/logrus"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func PolicyDetails(registryClient *http.Client, logger *log.Logger) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("policyDetails",
			mcp.WithDescription(`Fetches up-to-date documentation for a specific policy from the Terraform registry. You must call 'searchPolicies' first to obtain the exact terraformPolicyID required to use this tool.`),
			mcp.WithTitleAnnotation("Fetch detailed Terraform policy documentation using a terraformPolicyID"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithString("terraformPolicyID",
				mcp.Required(),
				mcp.Description("Matching terraformPolicyID retrieved from the 'searchPolicies' tool (e.g., 'policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1')"),
			),
		), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			terraformPolicyID, err := request.RequireString("terraformPolicyID")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "terraformPolicyID is required and must be a string, it is fetched by running the searchPolicies tool", err)
			}
			if terraformPolicyID == "" {
				return nil, utils.LogAndReturnError(logger, "terraformPolicyID cannot be empty, it is fetched by running the searchPolicies tool", nil)
			}

			policyResp, err := client.SendRegistryCall(registryClient, "GET", fmt.Sprintf("%s?include=policies,policy-modules,policy-library", terraformPolicyID), logger, "v2")
			if err != nil {
				return nil, utils.LogAndReturnError(logger, "Failed to fetch policy details: registry API did not return a successful response", err)
			}

			var policyDetails client.TerraformPolicyDetails
			if err := json.Unmarshal(policyResp, &policyDetails); err != nil {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("error unmarshalling policy details for %s", terraformPolicyID), err)
			}

			readme := utils.ExtractReadme(policyDetails.Data.Attributes.Readme)
			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("## Policy details about %s \n\n%s", terraformPolicyID, readme))
			policyList := ""
			moduleList := ""
			for _, policy := range policyDetails.Included {
				if policy.Type == "policy-modules" {
					moduleList += fmt.Sprintf(`
module "%s" {
  source = "https://registry.terraform.io/v2%s/policy-module/%s.sentinel?checksum=sha256:%s"
}
`, policy.Attributes.Name, terraformPolicyID, policy.Attributes.Name, policy.Attributes.Shasum)
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
			hclTemplate := fmt.Sprintf(`
%s
policy "<<POLICY_NAME>>" {
  source = "https://registry.terraform.io/v2%s/policy/<<POLICY_NAME>>.sentinel?checksum=<<POLICY_CHECKSUM>>"
  enforcement_level = "advisory"
}
`, moduleList, terraformPolicyID)
			builder.WriteString(hclTemplate)
			builder.WriteString("\n```\n")
			builder.WriteString(fmt.Sprintf("Available policies with SHA for %s are: \n\n", terraformPolicyID))
			builder.WriteString(policyList)

			policyData := builder.String()
			return mcp.NewToolResultText(policyData), nil
		}
}
