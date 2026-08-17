// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
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
	noCodeModuleID, err := request.RequireString("no_code_module_id")
	if err != nil {
		return ToolError(logger, "missing required input: no_code_module_id", err)
	}
	workspaceName, err := request.RequireString("workspace_name")
	if err != nil {
		return ToolError(logger, "missing required input: workspace_name", err)
	}
	projectID, err := request.RequireString("project_id")
	if err != nil {
		return ToolError(logger, "missing required input: project_id", err)
	}

	noCodeModuleID = strings.TrimSpace(noCodeModuleID)
	workspaceName = strings.TrimSpace(workspaceName)
	projectID = strings.TrimSpace(projectID)
	autoApply := request.GetBool("auto_apply", false)

	if !strings.HasPrefix(noCodeModuleID, "nocode-") {
		return ToolError(logger, "no_code_module_id must start with 'nocode-'", nil)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}
	if tfeClient == nil {
		return ToolError(logger, "failed to get Terraform client - ensure TFE_TOKEN and TFE_ADDRESS are configured", nil)
	}

	project, noCodeModule, moduleMetadata, err := fetchModuleData(ctx, tfeClient, projectID, noCodeModuleID)
	if err != nil {
		return ToolError(logger, err.Error(), nil)
	}

	// parse the returned module meta data to get required variable names for the module and all other variabels information correctly
	elicitationSchema := buildElicitationSchema(moduleMetadata, noCodeModule)

	// return client response for the elicitation message
	result, err := requestVariableValues(ctx, mcpServer, noCodeModuleID, elicitationSchema)
	if err != nil {
		return ToolError(logger, err.Error(), nil)
	}

	variables, err := processElicitationResponse(result, elicitationSchema)
	if err != nil {
		return ToolError(logger, err.Error(), nil)
	}

	workspace, err := tfeClient.RegistryNoCodeModules.CreateWorkspace(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleCreateWorkspaceOptions{
		Name:      workspaceName,
		Project:   project,
		Variables: variables,
		AutoApply: &autoApply,
	})
	if err != nil {
		return ToolError(logger, "failed to create No Code module workspace", err)
	}

	logger.Infof("Created No Code module workspace: %s", workspace.ID)
	buf, err := getWorkspaceDetailsForTools(ctx, "create_no_code_workspace", tfeClient, workspace, logger)
	if err != nil {
		return ToolError(logger, "failed to get workspace details", err)
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func fetchModuleData(ctx context.Context, tfeClient *tfe.Client, projectID, noCodeModuleID string) (*tfe.Project, *tfe.RegistryNoCodeModule, *client.ModuleMetadata, error) {
	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read project: %w", err)
	}

	noCodeModule, err := tfeClient.RegistryNoCodeModules.Read(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleReadOptions{
		Include: []tfe.RegistryNoCodeModuleIncludeOpt{tfe.RegistryNoCodeIncludeVariableOptions},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read No Code module: %w", err)
	}

	registryModule, err := tfeClient.RegistryModules.Read(ctx, tfe.RegistryModuleID{ID: noCodeModule.RegistryModule.ID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read Registry module: %w", err)
	}

	metadataPath := path.Join("/api/registry/private/v2/modules", registryModule.Namespace, registryModule.Name, registryModule.Provider, "metadata", noCodeModule.VersionPin)
	metadataRequest, err := tfeClient.NewRequestWithAdditionalQueryParams(
		http.MethodGet,
		metadataPath,
		nil,
		map[string][]string{"organization_name": {noCodeModule.Organization.Name}},
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create module metadata request: %w", err)
	}

	var moduleMetadata client.ModuleMetadata
	if err := metadataRequest.DoJSON(ctx, &moduleMetadata); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch module metadata: %w", err)
	}

	return project, noCodeModule, &moduleMetadata, nil
}

type moduleElicitationSchema struct {
	properties    map[string]any
	variableNames []string
	requiredNames []string
	requiredSet   map[string]struct{}
}

func buildElicitationSchema(moduleMetadata *client.ModuleMetadata, noCodeModule *tfe.RegistryNoCodeModule) *moduleElicitationSchema {
	inputCount := len(moduleMetadata.Data.Attributes.InputVariables)
	schema := &moduleElicitationSchema{
		properties:    make(map[string]any, inputCount),
		variableNames: make([]string, 0, inputCount),
		requiredNames: make([]string, 0, inputCount),
		requiredSet:   make(map[string]struct{}, inputCount),
	}

	for _, inputVar := range moduleMetadata.Data.Attributes.InputVariables {
		property := map[string]any{
			"title":       inputVar.Name,
			"description": inputVar.Description,
			"type":        mapTerraformTypeToJSON(inputVar.Type),
		}
		if enumOptions := findEnumOptions(inputVar.Name, inputVar.Type, noCodeModule.VariableOptions); enumOptions != nil {
			property["enum"] = enumOptions
		}

		schema.properties[inputVar.Name] = property
		schema.variableNames = append(schema.variableNames, inputVar.Name)
		if inputVar.Required {
			schema.requiredNames = append(schema.requiredNames, inputVar.Name)
			schema.requiredSet[inputVar.Name] = struct{}{}
		}
	}

	return schema
}

func (s *moduleElicitationSchema) isRequired(variableName string) bool {
	_, required := s.requiredSet[variableName]
	return required
}

func mapTerraformTypeToJSON(tfType string) string {
	switch tfType {
	case "string", "number":
		return tfType
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

func requestVariableValues(ctx context.Context, mcpServer *server.MCPServer, moduleID string, schema *moduleElicitationSchema) (*mcp.ElicitationResult, error) {
	request := mcp.ElicitationRequest{
		Params: mcp.ElicitationParams{
			Message: fmt.Sprintf("The No Code module '%s' requires %d variable(s) to create the workspace. Please provide values for the required variables.", moduleID, len(schema.requiredNames)),
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": schema.properties,
				"required":   schema.requiredNames,
			},
		},
	}

	result, err := mcpServer.RequestElicitation(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to request elicitation: %w", err)
	}

	return result, nil
}

func processElicitationResponse(result *mcp.ElicitationResult, schema *moduleElicitationSchema) ([]*tfe.Variable, error) {
	switch result.Action {
	case mcp.ElicitationResponseActionDecline:
		return nil, fmt.Errorf("workspace creation declined by user")
	case mcp.ElicitationResponseActionCancel:
		return nil, fmt.Errorf("workspace creation cancelled by user")
	case mcp.ElicitationResponseActionAccept:
		return extractVariablesFromResponse(result.Content, schema)
	default:
		return nil, fmt.Errorf("unexpected elicitation response action: %s", result.Action)
	}
}

func extractVariablesFromResponse(content any, schema *moduleElicitationSchema) ([]*tfe.Variable, error) {
	data, ok := content.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("elicitation response content is not a map, got %T", content)
	}

	variables := make([]*tfe.Variable, 0, len(schema.variableNames))
	for _, varName := range schema.variableNames {
		valueRaw, exists := data[varName]
		if !exists {
			if schema.isRequired(varName) {
				return nil, fmt.Errorf("required variable '%s' is missing from response", varName)
			}
			continue
		}

		// A blank optional form field means that the caller did not override the
		// module value. Omit it from the API request so Terraform uses its default.
		if strValue, isString := valueRaw.(string); isString && strValue == "" && !schema.isRequired(varName) {
			continue
		}

		variable, err := createVariable(varName, valueRaw, schema.properties)
		if err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}

	return variables, nil
}

func createVariable(varName string, valueRaw any, elicitationProperties map[string]any) (*tfe.Variable, error) {
	propertyDef, ok := elicitationProperties[varName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid property definition for variable '%s'", varName)
	}

	varType, _ := propertyDef["type"].(string)
	if varType == "" {
		varType = "string"
	}

	value, err := convertVariableValue(varName, varType, valueRaw)
	if err != nil {
		return nil, err
	}

	return &tfe.Variable{
		Key:      varName,
		Value:    value,
		Category: tfe.CategoryTerraform,
	}, nil
}

func convertVariableValue(varName, varType string, valueRaw any) (string, error) {
	switch varType {
	case "string":
		strValue, ok := valueRaw.(string)
		if !ok {
			return "", fmt.Errorf("variable '%s' must be a string, got %T", varName, valueRaw)
		}
		if strValue == "" {
			return "", fmt.Errorf("variable '%s' cannot be empty", varName)
		}
		return strValue, nil

	case "number":
		return convertNumberValue(varName, valueRaw)

	case "boolean":
		boolValue, ok := valueRaw.(bool)
		if !ok {
			return "", fmt.Errorf("variable '%s' must be a boolean, got %T", varName, valueRaw)
		}
		return fmt.Sprintf("%t", boolValue), nil

	default:
		jsonValue, err := json.Marshal(valueRaw)
		if err != nil {
			return "", fmt.Errorf("failed to marshal variable '%s': %w", varName, err)
		}
		return string(jsonValue), nil
	}
}

func convertNumberValue(varName string, valueRaw any) (string, error) {
	switch v := valueRaw.(type) {
	case float64:
		return fmt.Sprintf("%v", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("variable '%s' must be a number, got %T", varName, valueRaw)
	}
}
