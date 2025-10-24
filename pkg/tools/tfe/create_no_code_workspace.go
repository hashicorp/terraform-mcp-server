// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
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
			mcp.WithString("project_id",
				mcp.Required(),
				mcp.Description("The ID of the project to use"),
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
	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client", err)
	}
	if tfeClient == nil {
		return nil, utils.LogAndReturnError(logger, "getting Terraform client - please ensure TFE_TOKEN and TFE_ADDRESS are properly configured", nil)
	}

	params, err := extractRequestParams(request, logger)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(params.noCodeModuleID, "nocode-") {
		return nil, utils.LogAndReturnError(logger, "no_code_module_id must start with 'nocode-'", nil)
	}

	project, noCodeModule, moduleMetadata, err := fetchModuleData(ctx, tfeClient, params.projectID, params.noCodeModuleID, logger)
	if err != nil {
		return nil, err
	}

	elicitationProperties, requestedVars := buildElicitationSchema(moduleMetadata, noCodeModule)

	result, err := requestVariableValues(ctx, mcpServer, params.noCodeModuleID, elicitationProperties, requestedVars, logger)
	if err != nil {
		return nil, err
	}

	variables, err := processElicitationResponse(result, requestedVars, elicitationProperties, logger)
	if err != nil {
		return nil, err
	}

	workspace, err := tfeClient.RegistryNoCodeModules.CreateWorkspace(ctx, params.noCodeModuleID, &tfe.RegistryNoCodeModuleCreateWorkspaceOptions{
		Name:      params.workspaceName,
		Project:   project,
		Variables: variables,
		AutoApply: &params.autoApply,
	})
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "creating No Code module workspace", err)
	}

	logger.Infof("Created No Code module workspace: %s", workspace.ID)
	buf, err := getWorkspaceDetailsForTools(ctx, "create_no_code_workspace", tfeClient, workspace, logger)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "getting workspace details for tools", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}

type workspaceParams struct {
	noCodeModuleID string
	workspaceName  string
	projectID      string
	autoApply      bool
}

func extractRequestParams(request mcp.CallToolRequest, logger *log.Logger) (*workspaceParams, error) {
	noCodeModuleID, err := request.RequireString("no_code_module_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "the 'no_code_module_id' parameter is required", err)
	}

	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "the 'workspace_name' parameter is required", err)
	}

	projectID, err := request.RequireString("project_id")
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "the 'project_id' parameter is required", err)
	}

	return &workspaceParams{
		noCodeModuleID: noCodeModuleID,
		workspaceName:  workspaceName,
		projectID:      projectID,
		autoApply:      request.GetBool("auto_apply", false),
	}, nil
}

func fetchModuleData(ctx context.Context, tfeClient *tfe.Client, projectID, noCodeModuleID string, logger *log.Logger) (*tfe.Project, *tfe.RegistryNoCodeModule, *client.ModuleMetadata, error) {
	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return nil, nil, nil, utils.LogAndReturnError(logger, "reading project", err)
	}

	noCodeModule, err := tfeClient.RegistryNoCodeModules.Read(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleReadOptions{
		Include: []tfe.RegistryNoCodeModuleIncludeOpt{tfe.RegistryNoCodeIncludeVariableOptions},
	})
	if err != nil {
		return nil, nil, nil, utils.LogAndReturnError(logger, "reading No Code module", err)
	}

	registryModule, err := tfeClient.RegistryModules.Read(ctx, tfe.RegistryModuleID{ID: noCodeModule.RegistryModule.ID})
	if err != nil {
		return nil, nil, nil, utils.LogAndReturnError(logger, "reading Registry module", err)
	}

	metadataPath := path.Join("/api/registry/private/v2/modules", registryModule.Namespace, registryModule.Name, registryModule.Provider, "metadata", noCodeModule.VersionPin)
	metadataData, err := utils.MakeCustomGetRequestRaw(ctx, tfeClient, metadataPath, map[string][]string{"organization_name": {noCodeModule.Organization.Name}})
	if err != nil {
		return nil, nil, nil, utils.LogAndReturnError(logger, "making module metadata API request", err)
	}

	var moduleMetadata client.ModuleMetadata
	if err := json.Unmarshal(metadataData, &moduleMetadata); err != nil {
		return nil, nil, nil, utils.LogAndReturnError(logger, "unmarshalling module metadata", err)
	}

	return project, noCodeModule, &moduleMetadata, nil
}

func buildElicitationSchema(moduleMetadata *client.ModuleMetadata, noCodeModule *tfe.RegistryNoCodeModule) (map[string]any, []string) {
	elicitationProperties := make(map[string]any)
	requestedVars := make([]string, 0, len(moduleMetadata.Data.Attributes.InputVariables))

	for _, inputVar := range moduleMetadata.Data.Attributes.InputVariables {
		property := buildPropertySchema(inputVar, noCodeModule)
		elicitationProperties[inputVar.Name] = property
		requestedVars = append(requestedVars, inputVar.Name)
	}

	return elicitationProperties, requestedVars
}

func buildPropertySchema(inputVar struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
}, noCodeModule *tfe.RegistryNoCodeModule) map[string]any {
	property := map[string]any{
		"title":       inputVar.Name,
		"description": inputVar.Description,
		"type":        mapTerraformTypeToJSON(inputVar.Type),
	}

	if enumOptions := findEnumOptions(inputVar.Name, inputVar.Type, noCodeModule.VariableOptions); enumOptions != nil {
		property["enum"] = enumOptions
	}

	return property
}

func mapTerraformTypeToJSON(tfType string) string {
	switch tfType {
	case "string":
		return "string"
	case "number":
		return "number"
	case "bool":
		return "boolean"
	default:
		return "string"
	}
}

func findEnumOptions(varName, varType string, variableOptions []*tfe.NoCodeVariableOption) any {
	for _, varOpt := range variableOptions {
		if varOpt.VariableName != varName || len(varOpt.Options) == 0 {
			continue
		}

		switch varType {
		case "number":
			return convertToFloatEnum(varOpt.Options)
		case "bool":
			return convertToBoolEnum(varOpt.Options)
		default:
			return varOpt.Options
		}
	}
	return nil
}

func convertToFloatEnum(options []string) []float64 {
	result := make([]float64, 0, len(options))
	for _, opt := range options {
		if floatVal, err := strconv.ParseFloat(opt, 64); err == nil {
			result = append(result, floatVal)
		}
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

func convertToBoolEnum(options []string) []bool {
	result := make([]bool, 0, len(options))
	for _, opt := range options {
		if boolVal, err := strconv.ParseBool(opt); err == nil {
			result = append(result, boolVal)
		}
	}
	if len(result) > 0 {
		return result
	}
	return nil
}

func requestVariableValues(ctx context.Context, mcpServer *server.MCPServer, moduleID string, properties map[string]any, required []string, logger *log.Logger) (*mcp.ElicitationResult, error) {
	request := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: fmt.Sprintf("The No Code module '%s' requires %d variable(s) to create the workspace. Please provide values for the required variables.", moduleID, len(required)),
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}

	result, err := mcpServer.RequestElicitation(ctx, request)
	if err != nil {
		return nil, utils.LogAndReturnError(logger, "failed to request elicitation", err)
	}

	return result, nil
}

func processElicitationResponse(result *mcp.ElicitationResult, requestedVars []string, elicitationProperties map[string]any, logger *log.Logger) ([]*tfe.Variable, error) {
	switch result.Action {
	case mcp.ElicitationResponseActionDecline:
		return nil, utils.LogAndReturnError(logger, "No Code module workspace creation declined by user", nil)
	case mcp.ElicitationResponseActionCancel:
		return nil, utils.LogAndReturnError(logger, "No Code module workspace creation cancelled by user", nil)
	case mcp.ElicitationResponseActionAccept:
		return extractVariablesFromResponse(result.Content, requestedVars, elicitationProperties, logger)
	default:
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("unexpected elicitation response action: %s", result.Action), nil)
	}
}

func extractVariablesFromResponse(content any, requestedVars []string, elicitationProperties map[string]any, logger *log.Logger) ([]*tfe.Variable, error) {
	data, ok := content.(map[string]any)
	if !ok {
		return nil, utils.LogAndReturnError(logger, "elicitation response content is not a map", fmt.Errorf("expected map[string]any, got %T", content))
	}

	variables := make([]*tfe.Variable, 0, len(requestedVars))
	for _, varName := range requestedVars {
		variable, err := createVariable(varName, data, elicitationProperties, logger)
		if err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}

	return variables, nil
}

func createVariable(varName string, data map[string]any, elicitationProperties map[string]any, logger *log.Logger) (*tfe.Variable, error) {
	valueRaw, exists := data[varName]
	if !exists {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("required variable '%s' is missing from elicitation response", varName), nil)
	}

	propertyDef, ok := elicitationProperties[varName].(map[string]any)
	if !ok {
		return nil, utils.LogAndReturnError(logger, fmt.Sprintf("invalid property definition for variable '%s'", varName), nil)
	}

	varType, _ := propertyDef["type"].(string)
	if varType == "" {
		varType = "string"
	}

	value, err := convertVariableValue(varName, varType, valueRaw, logger)
	if err != nil {
		return nil, err
	}

	return &tfe.Variable{
		Key:      varName,
		Value:    value,
		Category: tfe.CategoryTerraform,
	}, nil
}

func convertVariableValue(varName, varType string, valueRaw any, logger *log.Logger) (string, error) {
	switch varType {
	case "string":
		strValue, ok := valueRaw.(string)
		if !ok {
			return "", utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a string", varName), fmt.Errorf("got %T", valueRaw))
		}
		if strValue == "" {
			return "", utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' cannot be empty", varName), nil)
		}
		return strValue, nil

	case "number":
		return convertNumberValue(varName, valueRaw, logger)

	case "boolean":
		boolValue, ok := valueRaw.(bool)
		if !ok {
			return "", utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a boolean", varName), fmt.Errorf("got %T", valueRaw))
		}
		return fmt.Sprintf("%t", boolValue), nil

	default:
		jsonValue, err := json.Marshal(valueRaw)
		if err != nil {
			return "", utils.LogAndReturnError(logger, fmt.Sprintf("failed to marshal variable '%s'", varName), err)
		}
		return string(jsonValue), nil
	}
}

func convertNumberValue(varName string, valueRaw any, logger *log.Logger) (string, error) {
	switch v := valueRaw.(type) {
	case float64:
		return fmt.Sprintf("%v", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case string:
		return v, nil
	default:
		return "", utils.LogAndReturnError(logger, fmt.Sprintf("variable '%s' must be a number", varName), fmt.Errorf("got %T", valueRaw))
	}
}
