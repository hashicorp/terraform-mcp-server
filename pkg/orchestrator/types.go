// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package orchestrator

import (
	"fmt"
	"time"
)

// WorkspaceAnalysisRequest represents the input for workspace analysis
type WorkspaceAnalysisRequest struct {
	WorkspaceID      string `json:"workspace_id,omitempty"`
	OrganizationName string `json:"organization_name,omitempty"`
	WorkspaceName    string `json:"workspace_name,omitempty"`
	Authorization    string `json:"authorization,omitempty"`
}

// Validate checks if the request has required parameters
func (r *WorkspaceAnalysisRequest) Validate() error {
	if r.WorkspaceID == "" && (r.OrganizationName == "" || r.WorkspaceName == "") {
		return fmt.Errorf("either workspace_id OR (organization_name + workspace_name) is required")
	}
	return nil
}

// WorkspaceAnalysisResponse represents the comprehensive workspace analysis result
type WorkspaceAnalysisResponse struct {
	WorkspaceDetails    *WorkspaceDetails     `json:"workspace_details"`
	Variables           *VariablesSummary     `json:"variables"`
	Configurations      *ConfigurationSummary `json:"configurations"`
	StateInfo           *StateVersionSummary  `json:"state_info"`
	Tags                *TagsSummary          `json:"tags"`
	RemoteConsumers     *RemoteConsumerInfo   `json:"remote_consumers,omitempty"`
	Summary             string                `json:"summary"`
	Errors              []string              `json:"errors,omitempty"`
	ExecutionTime       time.Duration         `json:"execution_time"`
}

// WorkspaceDetails contains essential workspace information
type WorkspaceDetails struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Organization     string    `json:"organization"`
	ExecutionMode    string    `json:"execution_mode"`
	TerraformVersion string    `json:"terraform_version"`
	AutoApply        bool      `json:"auto_apply"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Locked           bool      `json:"locked"`
	WorkingDirectory string    `json:"working_directory"`
}

// VariablesSummary contains aggregated variable information
type VariablesSummary struct {
	TotalCount         int            `json:"total_count"`
	TerraformVarsCount int            `json:"terraform_vars_count"`
	EnvironmentVars    int            `json:"environment_vars_count"`
	SensitiveVarsCount int            `json:"sensitive_vars_count"`
	Variables          []VariableInfo `json:"variables"`
}

// VariableInfo represents individual variable details
type VariableInfo struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Category    string `json:"category"`
	Sensitive   bool   `json:"sensitive"`
	HCL         bool   `json:"hcl"`
	Description string `json:"description"`
}

// ConfigurationSummary contains configuration version information
type ConfigurationSummary struct {
	LatestVersion  string              `json:"latest_version"`
	Status         string              `json:"status"`
	UploadedAt     time.Time           `json:"uploaded_at"`
	Source         string              `json:"source"`
	TotalVersions  int                 `json:"total_versions"`
	RecentVersions []ConfigurationInfo `json:"recent_versions"`
}

// ConfigurationInfo represents individual configuration version details
type ConfigurationInfo struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Speculative bool      `json:"speculative"`
	UploadedAt  time.Time `json:"uploaded_at"`
	Source      string    `json:"source"`
}

// StateVersionSummary contains current state information
type StateVersionSummary struct {
	CurrentVersion string    `json:"current_version"`
	Serial         int       `json:"serial"`
	CreatedAt      time.Time `json:"created_at"`
	Size           int64     `json:"size,omitempty"`
	ResourcesCount int       `json:"resources_count,omitempty"`
	OutputsCount   int       `json:"outputs_count,omitempty"`
}

// TagsSummary contains workspace tags information
type TagsSummary struct {
	TotalCount int       `json:"total_count"`
	Tags       []TagInfo `json:"tags"`
}

// TagInfo represents individual tag details
type TagInfo struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RemoteConsumerInfo contains information about workspaces consuming this workspace's state
type RemoteConsumerInfo struct {
	ConsumerCount int                  `json:"consumer_count"`
	Consumers     []WorkspaceReference `json:"consumers"`
}

// WorkspaceReference represents a reference to another workspace
type WorkspaceReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ===== Phase 1: Configuration Preparation Types =====

// ConfigPreparationRequest represents the input for configuration preparation
type ConfigPreparationRequest struct {
	WorkspaceID       string            `json:"workspace_id"`
	NewWorkspaceName  string            `json:"new_workspace_name"`
	TagUpdates        map[string]string `json:"tag_updates"`
	VariableUpdates   map[string]string `json:"variable_updates,omitempty"`
	ProviderUpdates   map[string]interface{} `json:"provider_updates,omitempty"`
	Authorization     string            `json:"authorization,omitempty"`
}

// Validate checks if the configuration preparation request has required parameters
func (r *ConfigPreparationRequest) Validate() error {
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if r.NewWorkspaceName == "" {
		return fmt.Errorf("new_workspace_name is required")
	}
	return nil
}

// ConfigPreparationResponse represents the result of configuration preparation
type ConfigPreparationResponse struct {
	ModifiedConfigContent   string   `json:"modified_config_content"` // base64 encoded tar.gz
	OriginalConfigVersionID string   `json:"original_config_version_id"`
	ModificationsSummary    []string `json:"modifications_summary"`
	ParsedFiles             []string `json:"parsed_files"`
	Errors                  []string `json:"errors,omitempty"`
	ProcessingTimeSeconds   int      `json:"processing_time_seconds"`
}

// TerraformConfig represents parsed Terraform configuration structure
type TerraformConfig struct {
	ProviderBlocks map[string]*ProviderBlock `json:"provider_blocks"`
	ResourceBlocks map[string]*ResourceBlock `json:"resource_blocks"`
	Variables      map[string]*Variable      `json:"variables"`
	Outputs        map[string]*Output        `json:"outputs"`
	LocalValues    map[string]*Local         `json:"local_values"`
	ModuleCalls    map[string]*ModuleCall    `json:"module_calls"`
	DataSources    map[string]*DataSource    `json:"data_sources"`
}

// ProviderBlock represents a Terraform provider configuration
type ProviderBlock struct {
	Name         string                 `json:"name"`
	Alias        string                 `json:"alias,omitempty"`
	Version      string                 `json:"version,omitempty"`
	Configuration map[string]interface{} `json:"configuration"`
	DefaultTags  map[string]string      `json:"default_tags,omitempty"`
	Region       string                 `json:"region,omitempty"`
	FileName     string                 `json:"file_name"`
	LineNumber   int                    `json:"line_number"`
}

// ResourceBlock represents a Terraform resource
type ResourceBlock struct {
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Configuration map[string]interface{} `json:"configuration"`
	Tags         map[string]string      `json:"tags,omitempty"`
	FileName     string                 `json:"file_name"`
	LineNumber   int                    `json:"line_number"`
}

// Variable represents a Terraform variable
type Variable struct {
	Name         string      `json:"name"`
	Type         string      `json:"type,omitempty"`
	Description  string      `json:"description,omitempty"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Sensitive    bool        `json:"sensitive,omitempty"`
	FileName     string      `json:"file_name"`
	LineNumber   int         `json:"line_number"`
}

// Output represents a Terraform output
type Output struct {
	Name        string      `json:"name"`
	Value       interface{} `json:"value"`
	Description string      `json:"description,omitempty"`
	Sensitive   bool        `json:"sensitive,omitempty"`
	FileName    string      `json:"file_name"`
	LineNumber  int         `json:"line_number"`
}

// Local represents a Terraform local value
type Local struct {
	Name       string      `json:"name"`
	Value      interface{} `json:"value"`
	FileName   string      `json:"file_name"`
	LineNumber int         `json:"line_number"`
}

// ModuleCall represents a Terraform module call
type ModuleCall struct {
	Name         string                 `json:"name"`
	Source       string                 `json:"source"`
	Version      string                 `json:"version,omitempty"`
	Configuration map[string]interface{} `json:"configuration"`
	FileName     string                 `json:"file_name"`
	LineNumber   int                    `json:"line_number"`
}

// DataSource represents a Terraform data source
type DataSource struct {
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	Configuration map[string]interface{} `json:"configuration"`
	FileName     string                 `json:"file_name"`
	LineNumber   int                    `json:"line_number"`
}

// VariableBlock represents a Terraform variable block
type VariableBlock struct {
	Name         string      `json:"name"`
	Type         string      `json:"type,omitempty"`
	Description  string      `json:"description,omitempty"`
	Default      interface{} `json:"default,omitempty"`
	Sensitive    bool        `json:"sensitive,omitempty"`
	Validation   []map[string]interface{} `json:"validation,omitempty"`
	FileName     string      `json:"file_name"`
	LineNumber   int         `json:"line_number"`
}

// OutputBlock represents a Terraform output block
type OutputBlock struct {
	Name        string      `json:"name"`
	Value       string      `json:"value"`
	Description string      `json:"description,omitempty"`
	Sensitive   bool        `json:"sensitive,omitempty"`
	FileName    string      `json:"file_name"`
	LineNumber  int         `json:"line_number"`
}
