// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/jsonapi"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/utils"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// CreateNoCodeWorkspace creates a tool to create a No Code module workspace.
func CreateNoCodeWorkspace(logger *log.Logger, mcpServer *server.MCPServer) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("create_no_code_workspace",
			mcp.WithDescription(`Creates a new Terraform No Code module workspace. The tool uses the MCP elicitation feature to automatically discover and collect required variables from the user.`),
			mcp.WithTitleAnnotation("Create a No Code module workspace"),
			mcp.WithOpenWorldHintAnnotation(true),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithString("no_code_module_id",
				mcp.Required(),
				mcp.Description("The ID of the No Code module to create a workspace for"),
			),
			mcp.WithString("workspace_name",
				mcp.Required(),
				mcp.Description("The name of the workspace to create"),
			),
			mcp.WithBoolean("auto_apply",
				mcp.Description("Whether to automatically apply changes in the workspace: 'true' or 'false'"),
				mcp.DefaultBool(false),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return createNoCodeWorkspaceHandler(ctx, req, logger, mcpServer)
		},
	}
}

func createNoCodeWorkspaceHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger, mcpServer *server.MCPServer) (*mcp.CallToolResult, error) {
	// Get a Terraform client from context
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}
	if tfeClient == nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", nil)
	}

	noCodeModuleID, err := request.RequireString("no_code_module_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "the 'no_code_module_id' parameter is required", err)
	}

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "the 'workspace_name' parameter is required", err)
	}
	autoApply := request.GetBool("auto_apply", false)

	// Check if the noCodeModuleID starts with "nocode-"
	if !strings.HasPrefix(noCodeModuleID, "nocode-") {
		return nil, utils.LogAndReturnError(logger, "no_code_module_id must start with 'nocode-'", nil)
	}

	noCodeModule, err := tfeClient.RegistryNoCodeModules.Read(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleReadOptions{
		Include: []tfe.RegistryNoCodeModuleIncludeOpt{
			tfe.RegistryNoCodeIncludeVariableOptions,
		},
	})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "reading No Code module", err)
	}

	registryModule, err := tfeClient.RegistryModules.Read(ctx, tfe.RegistryModuleID{ID: noCodeModule.RegistryModule.ID})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "error reading Registry module", err)
	}

	// Make a custom metadata call
	customMetadataPath := path.Join("/api/registry/private/v2/modules", registryModule.Namespace, registryModule.Name, registryModule.Provider, "metadata", noCodeModule.VersionPin)
	inputVariableData, err := utils.MakeCustomRequestWithDoRaw(ctx, tfeClient, customMetadataPath, map[string][]string{"organization_name": {noCodeModule.Organization.Name}})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "making module metadata API request", err)
	}

	var moduleMetadata client.ModuleMetadata
	if err := json.Unmarshal(inputVariableData, &moduleMetadata); err != nil {
		return nil, utils.LogAndReturnError(logger, "unmarshalling module metadata", err)
	}

	// Build elicitation schema and collect variable information
	var variables []*tfe.Variable
	var elicitationProperties = make(map[string]any)
	var requiredVars []string

	// Process each variable option from the No Code module
	for _, inputVar := range moduleMetadata.Data.Attributes.InputVariables {
		property := make(map[string]any)

		// Map Terraform variable types to JSON Schema types
		// Could be any of the Terraform variable types
		// string, number, bool, list, set, map or null
		switch inputVar.Type {
		case "string":
			property["type"] = "string"
		case "number":
			property["type"] = "number"
		case "bool":
			property["type"] = "boolean"
		default:
			// Default to string for unknown types
			property["type"] = "string"
		}

		property["title"] = inputVar.Name
		property["description"] = inputVar.Description

		for _, varOpt := range noCodeModule.VariableOptions {
			if varOpt.VariableName == inputVar.Name {
				if len(varOpt.Options) > 0 {
					// Add options as enum if available
					enumOptions := make([]string, len(varOpt.Options))
					copy(enumOptions, varOpt.Options)
					property["enum"] = enumOptions
				}
				break
			}
		}

		elicitationProperties[inputVar.Name] = property

		if inputVar.Required {
			requiredVars = append(requiredVars, inputVar.Name)
		}
	}

	elicitationRequest := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: fmt.Sprintf("The No Code module '%s' requires %d variable(s) to create the workspace. Please provide values for the required variables.", noCodeModuleID, len(requiredVars)),
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": elicitationProperties,
				"required":   requiredVars,
			},
		},
	}

	// Request elicitation from the client
	result, err := mcpServer.RequestElicitation(ctx, elicitationRequest)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "failed to request elicitation", err)
	}

	// Handle the user's response
	switch result.Action {
	case mcp.ElicitationResponseActionAccept:
		// Extract the provided variable values
		data, ok := result.Content.(map[string]any)
		if !ok {
			return nil, utils.LogAndReturnError(logger, "elicitation response content is not a map", fmt.Errorf("expected map[string]any, got %T", result.Content))
		}

		// Process each provided variable
		for _, varName := range requiredVars {
			valueRaw, exists := data[varName]
			if !exists {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("required variable '%s' is missing from elicitation response", varName), nil)
			}

			// Get the property definition to determine the type
			propertyDef, ok := elicitationProperties[varName].(map[string]any)
			if !ok {
				return nil, utils.LogAndReturnError(logger, fmt.Sprintf("invalid property definition for variable '%s'", varName), nil)
			}

			varType, ok := propertyDef["type"].(string)
			if !ok {
				varType = "string" // Default to string if type is not specified
			}

			var value string
			// Cast the value based on its type
			switch varType {
			case "string":
				strValue, ok := valueRaw.(string)
				if !ok {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a string", varName), fmt.Errorf("got %T", valueRaw))
				}
				if strValue == "" {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' cannot be empty", varName), nil)
				}
				value = strValue

			case "number":
				// Handle both float64 (from JSON) and int
				switch v := valueRaw.(type) {
				case float64:
					value = fmt.Sprintf("%v", v)
				case int:
					value = fmt.Sprintf("%d", v)
				case string:
					// Allow string representation of numbers
					value = v
				default:
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a number", varName), fmt.Errorf("got %T", valueRaw))
				}

			case "boolean":
				boolValue, ok := valueRaw.(bool)
				if !ok {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a boolean", varName), fmt.Errorf("got %T", valueRaw))
				}
				value = fmt.Sprintf("%t", boolValue)

			default:
				// For complex types (list, map, etc.), convert to JSON string
				jsonValue, err := json.Marshal(valueRaw)
				if err != nil {
					return nil, utils.LogAndReturnError(logger, fmt.Sprintf("failed to marshal variable '%s'", varName), err)
				}
				value = string(jsonValue)
			}

			// Add the variable to our array
			workspaceVariable := &tfe.Variable{
				Key:      varName,
				Value:    value,
				Category: tfe.CategoryTerraform,
			}
			variables = append(variables, workspaceVariable)
		}

	case mcp.ElicitationResponseActionDecline:
		return nil, utils.LogAndReturnError(logger, "No Code module workspace creation cancelled by user", nil)

	case mcp.ElicitationResponseActionCancel:
		return nil, utils.LogAndReturnError(logger, "No Code module workspace creation cancelled by user", nil)

	default:
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("unexpected elicitation response action: %s", result.Action), nil)
	}

	noCodeModuleWorkspace, err := tfeClient.RegistryNoCodeModules.CreateWorkspace(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleCreateWorkspaceOptions{
		Name:      workspaceName,
		Variables: variables,
		AutoApply: &autoApply,
	})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "creating No Code module workspace", err)
	}

	logger.Infof("Created No Code module workspace: %s", noCodeModuleWorkspace.ID)
	var buf bytes.Buffer
	if err := jsonapi.MarshalPayload(&buf, noCodeModuleWorkspace); err != nil {
		return nil, utils.LogAndReturnError(logger, "marshaling run response", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.NewTextContent(buf.String()),
		},
	}, nil
}
