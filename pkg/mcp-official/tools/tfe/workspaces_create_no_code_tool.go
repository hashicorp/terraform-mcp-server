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

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/hashicorp/go-tfe"
	tfeclient "github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/hashicorp/terraform-mcp-server/pkg/mcp-official/client"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CreateNoCodeWorkspaceArguments holds the inputs for creating a no-code workspace.
type CreateNoCodeWorkspaceArguments struct {
	NoCodeModuleID string `json:"no_code_module_id"`
	WorkspaceName  string `json:"workspace_name"`
	ProjectID      string `json:"project_id"`
	AutoApply      bool   `json:"auto_apply,omitempty"`
}

// CreateNoCodeWorkspaceResponse reports the no-code workspace created by the tool.
type CreateNoCodeWorkspaceResponse struct {
	WorkspaceID    string `json:"workspace_id"`
	WorkspaceName  string `json:"workspace_name"`
	ProjectID      string `json:"project_id"`
	NoCodeModuleID string `json:"no_code_module_id"`
	AutoApply      bool   `json:"auto_apply"`
}

// CreateNoCodeWorkspaceTool describes the create_no_code_workspace tool.
func CreateNoCodeWorkspaceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "create_no_code_workspace",
		Description: "Creates a new Terraform No Code module workspace. The tool uses MCP elicitation to discover and collect required variables from the user.",
		InputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"no_code_module_id": {
					Type:        "string",
					Description: "The ID of the No Code module to create a workspace for",
					Pattern:     `^nocode-`,
				},
				"workspace_name": {
					Type:        "string",
					Description: "The name of the workspace to create",
				},
				"project_id": {
					Type:        "string",
					Description: "The ID of the project to use",
				},
				"auto_apply": {
					Type:        "boolean",
					Description: "Whether to automatically apply changes in the workspace",
					Default:     json.RawMessage("false"),
				},
			},
			PropertyOrder:        []string{"no_code_module_id", "workspace_name", "project_id", "auto_apply"},
			Required:             []string{"no_code_module_id", "workspace_name", "project_id"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		OutputSchema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"workspace_id":      {Type: "string"},
				"workspace_name":    {Type: "string"},
				"project_id":        {Type: "string"},
				"no_code_module_id": {Type: "string"},
				"auto_apply":        {Type: "boolean"},
			},
			PropertyOrder:        []string{"workspace_id", "workspace_name", "project_id", "no_code_module_id", "auto_apply"},
			Required:             []string{"workspace_id", "workspace_name", "project_id", "no_code_module_id", "auto_apply"},
			AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		},
		Annotations: &mcp.ToolAnnotations{
			Title:           "Create a No Code module workspace",
			OpenWorldHint:   jsonschema.Ptr(true),
			ReadOnlyHint:    false,
			DestructiveHint: jsonschema.Ptr(true),
		},
	}
}

func CreateNoCodeWorkspaceFunc(ctx context.Context, request *mcp.CallToolRequest, input CreateNoCodeWorkspaceArguments) (*mcp.CallToolResult, *CreateNoCodeWorkspaceResponse, error) {
	noCodeModuleID := strings.TrimSpace(input.NoCodeModuleID)
	if noCodeModuleID == "" {
		return nil, nil, fmt.Errorf("no_code_module_id must not be blank")
	}
	if !strings.HasPrefix(noCodeModuleID, "nocode-") {
		return nil, nil, fmt.Errorf("no_code_module_id must start with 'nocode-'")
	}

	workspaceName := strings.TrimSpace(input.WorkspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace_name must not be blank")
	}
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return nil, nil, fmt.Errorf("project_id must not be blank")
	}

	session, ok := request.GetSession().(*mcp.ServerSession)
	if !ok || session == nil {
		return nil, nil, fmt.Errorf("request does not have an active MCP server session")
	}

	tfeClient, err := client.GetTfeClient(ctx, client.SessionIDFromRequest(request))
	if err != nil {
		return nil, nil, fmt.Errorf("getting Terraform client: %w", err)
	}

	project, noCodeModule, moduleMetadata, err := fetchNoCodeModuleData(ctx, tfeClient, projectID, noCodeModuleID)
	if err != nil {
		return nil, nil, err
	}

	elicitationSchema := buildNoCodeElicitationSchema(moduleMetadata, noCodeModule)
	result, err := session.Elicit(ctx, &mcp.ElicitParams{
		Message:         fmt.Sprintf("The No Code module '%s' requires %d variable(s) to create the workspace. Please provide values for the required variables.", noCodeModuleID, len(elicitationSchema.schema.Required)),
		RequestedSchema: elicitationSchema.schema,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("requesting variable values: %w", err)
	}

	variables, err := processNoCodeElicitationResponse(result, elicitationSchema)
	if err != nil {
		return nil, nil, err
	}

	workspace, err := tfeClient.RegistryNoCodeModules.CreateWorkspace(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleCreateWorkspaceOptions{
		Name:      workspaceName,
		Project:   project,
		Variables: variables,
		AutoApply: tfe.Bool(input.AutoApply),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("creating No Code module workspace %q: %w", workspaceName, err)
	}

	return nil, &CreateNoCodeWorkspaceResponse{
		WorkspaceID:    workspace.ID,
		WorkspaceName:  workspace.Name,
		ProjectID:      projectID,
		NoCodeModuleID: noCodeModuleID,
		AutoApply:      input.AutoApply,
	}, nil
}

type noCodeElicitationSchema struct {
	schema        *jsonschema.Schema
	variableNames []string
	requiredSet   map[string]struct{}
}

func mapTerraformTypeToJSON(terraformType string) string {
	switch terraformType {
	case "string", "number":
		return terraformType
	case "bool":
		return "boolean"
	default:
		return "string"
	}
}

func fetchNoCodeModuleData(ctx context.Context, tfeClient *tfe.Client, projectID, noCodeModuleID string) (*tfe.Project, *tfe.RegistryNoCodeModule, *tfeclient.ModuleMetadata, error) {
	project, err := tfeClient.Projects.Read(ctx, projectID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading project %q: %w", projectID, err)
	}

	noCodeModule, err := tfeClient.RegistryNoCodeModules.Read(ctx, noCodeModuleID, &tfe.RegistryNoCodeModuleReadOptions{
		Include: []tfe.RegistryNoCodeModuleIncludeOpt{tfe.RegistryNoCodeIncludeVariableOptions},
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading No Code module %q: %w", noCodeModuleID, err)
	}

	registryModule, err := tfeClient.RegistryModules.Read(ctx, tfe.RegistryModuleID{ID: noCodeModule.RegistryModule.ID})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading Registry module: %w", err)
	}

	metadataPath := path.Join("/api/registry/private/v2/modules", registryModule.Namespace, registryModule.Name, registryModule.Provider, "metadata", noCodeModule.VersionPin)
	metadataRequest, err := tfeClient.NewRequestWithAdditionalQueryParams(http.MethodGet, metadataPath, nil, map[string][]string{"organization_name": {noCodeModule.Organization.Name}})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating module metadata request: %w", err)
	}

	var moduleMetadata tfeclient.ModuleMetadata
	if err := metadataRequest.DoJSON(ctx, &moduleMetadata); err != nil {
		return nil, nil, nil, fmt.Errorf("fetching module metadata: %w", err)
	}

	return project, noCodeModule, &moduleMetadata, nil
}

func buildNoCodeElicitationSchema(moduleMetadata *tfeclient.ModuleMetadata, noCodeModule *tfe.RegistryNoCodeModule) *noCodeElicitationSchema {
	inputVariables := moduleMetadata.Data.Attributes.InputVariables
	properties := make(map[string]*jsonschema.Schema, len(inputVariables))
	variableNames := make([]string, 0, len(inputVariables))
	required := make([]string, 0, len(inputVariables))
	requiredSet := make(map[string]struct{}, len(inputVariables))

	for _, inputVariable := range inputVariables {
		property := &jsonschema.Schema{
			Type:        mapTerraformTypeToJSON(inputVariable.Type),
			Title:       inputVariable.Name,
			Description: inputVariable.Description,
		}
		if enumOptions := findNoCodeEnumOptions(inputVariable.Name, inputVariable.Type, noCodeModule.VariableOptions); enumOptions != nil {
			property.Enum = enumOptions
		}
		properties[inputVariable.Name] = property
		variableNames = append(variableNames, inputVariable.Name)
		if inputVariable.Required {
			required = append(required, inputVariable.Name)
			requiredSet[inputVariable.Name] = struct{}{}
		}
	}

	return &noCodeElicitationSchema{
		schema: &jsonschema.Schema{
			Type:          "object",
			Properties:    properties,
			PropertyOrder: variableNames,
			Required:      required,
		},
		variableNames: variableNames,
		requiredSet:   requiredSet,
	}
}

func findNoCodeEnumOptions(name, variableType string, options []*tfe.NoCodeVariableOption) []any {
	for _, option := range options {
		if option.VariableName != name || len(option.Options) == 0 {
			continue
		}
		switch variableType {
		case "number":
			values := convertNoCodeFloatEnum(option.Options)
			if len(values) > 0 {
				result := make([]any, len(values))
				for i, value := range values {
					result[i] = value
				}
				return result
			}
		case "bool":
			values := convertNoCodeBoolEnum(option.Options)
			if len(values) > 0 {
				result := make([]any, len(values))
				for i, value := range values {
					result[i] = value
				}
				return result
			}
		default:
			result := make([]any, len(option.Options))
			for i, value := range option.Options {
				result[i] = value
			}
			return result
		}
	}
	return nil
}

func processNoCodeElicitationResponse(result *mcp.ElicitResult, schema *noCodeElicitationSchema) ([]*tfe.Variable, error) {
	switch result.Action {
	case "decline":
		return nil, fmt.Errorf("workspace creation declined by user")
	case "cancel":
		return nil, fmt.Errorf("workspace creation cancelled by user")
	case "accept":
		return extractNoCodeVariables(result.Content, schema)
	default:
		return nil, fmt.Errorf("unexpected elicitation response action: %s", result.Action)
	}
}

func extractNoCodeVariables(content map[string]any, schema *noCodeElicitationSchema) ([]*tfe.Variable, error) {
	variables := make([]*tfe.Variable, 0, len(schema.variableNames))
	for _, name := range schema.variableNames {
		value, exists := content[name]
		if !exists {
			if _, required := schema.requiredSet[name]; required {
				return nil, fmt.Errorf("required variable '%s' is missing from response", name)
			}
			continue
		}
		if stringValue, ok := value.(string); ok && stringValue == "" {
			if _, required := schema.requiredSet[name]; !required {
				continue
			}
		}

		property := schema.schema.Properties[name]
		converted, err := convertNoCodeVariableValue(name, property.Type, value)
		if err != nil {
			return nil, err
		}
		variables = append(variables, &tfe.Variable{Key: name, Value: converted, Category: tfe.CategoryTerraform})
	}
	return variables, nil
}

func convertNoCodeVariableValue(name, variableType string, value any) (string, error) {
	switch variableType {
	case "string":
		stringValue, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("variable '%s' must be a string, got %T", name, value)
		}
		if stringValue == "" {
			return "", fmt.Errorf("variable '%s' cannot be empty", name)
		}
		return stringValue, nil
	case "number":
		switch number := value.(type) {
		case float64:
			return fmt.Sprintf("%v", number), nil
		case int:
			return fmt.Sprintf("%d", number), nil
		case string:
			return number, nil
		default:
			return "", fmt.Errorf("variable '%s' must be a number, got %T", name, value)
		}
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return "", fmt.Errorf("variable '%s' must be a boolean, got %T", name, value)
		}
		return strconv.FormatBool(boolean), nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("marshaling variable %q: %w", name, err)
		}
		return string(data), nil
	}
}

func convertNoCodeFloatEnum(options []string) []float64 {
	values := make([]float64, 0, len(options))
	for _, option := range options {
		value, err := strconv.ParseFloat(option, 64)
		if err == nil {
			values = append(values, value)
		}
	}
	return values
}

func convertNoCodeBoolEnum(options []string) []bool {
	values := make([]bool, 0, len(options))
	for _, option := range options {
		value, err := strconv.ParseBool(option)
		if err == nil {
			values = append(values, value)
		}
	}
	return values
}
