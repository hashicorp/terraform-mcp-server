# Workspace Replication Implementation Plan - Option A: Extended Single Orchestrator

## 📊 Progress Summary
- ✅ **Phase 1 Complete**: Configuration Preparation Flow (100%)
- 🚧 **Phase 2-6**: Remaining flows (0% - Ready for implementation)
- ✅ **Dependencies**: HCL parsing libraries added
- ✅ **New Tool**: `prepare_workspace_configuration` MCP tool available
- 📈 **Tool Count**: Updated from 35 to 36 tools

## Overview
This plan extends the existing workspace orchestrator to support complete workspace replication functionality across 5 additional flows:
1. ✅ Configuration Preparation → Download and modify configs **COMPLETED**
2. ⏳ Workspace Creation → Create new workspace with variables and tags  
3. ⏳ Configuration Upload → Upload modified configurations
4. ⏳ State Management → Handle state with proper locking
5. ⏳ Remote State Setup → Configure state sharing between workspaces

## Implementation Strategy
- **Approach**: Extend existing `pkg/orchestrator/` package
- **Architecture**: Keep clean separation of concerns with focused, testable components
- **Integration**: Leverage existing HCP Terraform tools and add new orchestration layers
- **Error Handling**: Comprehensive error handling with rollback capabilities

---

## Phase 1: Configuration Preparation Flow ✅ **COMPLETED**

### 1.1 Data Types & Structures ✅ **COMPLETED**
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [x] Add `ConfigPreparationRequest` struct
  - `WorkspaceID string`
  - `NewWorkspaceName string`
  - `TagUpdates map[string]string`
  - `VariableUpdates map[string]string` (optional)
  - `ProviderUpdates map[string]interface{}` (optional)

- [x] Add `ConfigPreparationResponse` struct
  - `ModifiedConfigContent string` (base64 encoded tar.gz)
  - `OriginalConfigVersionID string`
  - `ModificationsSummary []string`
  - `ParsedFiles []string`
  - `Errors []string`

- [x] Add `TerraformConfig` struct for HCL parsing
  - `ProviderBlocks map[string]*ProviderBlock`
  - `ResourceBlocks map[string]*ResourceBlock`
  - `Variables map[string]*Variable`
  - `Outputs map[string]*Output`
  - `LocalValues map[string]*Local`

### 1.2 Configuration Modifier Component ✅ **COMPLETED**
**File**: `pkg/orchestrator/config_modifier.go` (NEW)

**TODO Items**:
- [x] Create `ConfigModifier` struct
  - Add fields for HCL parser, terraform inspector
  - Add configuration for supported providers (AWS, Azure, GCP, etc.)

- [x] Implement `ParseTerraformConfig(content []byte) (*TerraformConfig, error)`
  - Parse tar.gz content
  - Extract and parse individual .tf files
  - Build comprehensive configuration structure
  - Handle HCL syntax errors gracefully

- [x] Implement `AddDefaultTags(config *TerraformConfig, tags map[string]string) error`
  - Identify provider blocks (aws, azurerm, google, etc.)
  - Add/merge default_tags for supported providers
  - Handle existing default_tags conflicts
  - Support provider-specific tag syntax

- [x] Implement `UpdateVariableReferences(config *TerraformConfig, updates map[string]string) error`
  - Parse variable references in configuration
  - Update variable default values
  - Update variable descriptions if needed
  - Validate variable type consistency

- [x] Implement `UpdateProviderConfigurations(config *TerraformConfig, updates map[string]interface{}) error`
  - Modify provider block configurations
  - Update region/location settings
  - Handle provider aliases
  - Validate provider configuration syntax

- [x] Implement `SerializeConfig(config *TerraformConfig) ([]byte, error)`
  - Convert back to HCL format
  - Maintain original formatting where possible
  - Create tar.gz archive
  - Return base64 encoded content

- [x] Add `ValidateModifiedConfig(content []byte) error`
  - Run terraform validate equivalent
  - Check HCL syntax
  - Validate provider configurations
  - Report detailed validation errors

### 1.3 Analysis Steps Extension ✅ **COMPLETED**
**File**: `pkg/orchestrator/analysis_steps.go` (EXTEND)

**TODO Items**:
- [x] Implement `PrepareConfiguration(ctx context.Context, req ConfigPreparationRequest) (*ConfigPreparationResponse, error)`
  - Call `get_hcp_terraform_configuration_versions` to get latest version
  - Call `download_hcp_terraform_configuration_files` to get config content
  - Use ConfigModifier to parse and modify configuration
  - Apply tag updates for new workspace name
  - Apply any variable reference updates
  - Apply provider configuration updates
  - Validate modified configuration
  - Return response with modified content and summary

### 1.4 MCP Tool Integration ✅ **COMPLETED**
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [x] Add `ConfigurationPreparator()` MCP tool function
  - Define tool schema for configuration preparation
  - Handle ConfigPreparationRequest parameters
  - Call orchestrator's PrepareConfiguration method
  - Return formatted response for MCP clients

---

## Phase 2: Workspace Creation Flow

### 2.1 Data Types & Structures  
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [ ] Add `WorkspaceCreationRequest` struct
  - `SourceWorkspaceID string`
  - `NewWorkspaceName string`
  - `OrganizationName string`
  - `ProjectID string` (optional)
  - `AdditionalTags map[string]string` (optional)
  - `VariableOverrides map[string]interface{}` (optional)
  - `WorkspaceSettings map[string]interface{}` (optional)

- [ ] Add `WorkspaceCreationResponse` struct
  - `NewWorkspaceID string`
  - `NewWorkspaceName string`
  - `CreatedVariableCount int`
  - `AppliedTags []string`
  - `CopiedSettings []string`
  - `Errors []string`

- [ ] Add `VariableReplication` struct
  - `SourceVariable interface{}`
  - `TargetVariable interface{}`
  - `Modified bool`
  - `OverrideApplied bool`

### 2.2 Analysis Steps Extension
**File**: `pkg/orchestrator/analysis_steps.go` (EXTEND)

**TODO Items**:
- [ ] Implement `createReplicatedWorkspace(ctx context.Context, req WorkspaceCreationRequest) (*WorkspaceCreationResponse, error)`
  - Call `get_hcp_terraform_workspace_details` for source workspace
  - Extract workspace configuration (execution mode, terraform version, etc.)
  - Call `create_hcp_terraform_workspace` with new name and copied settings
  - Handle project assignment if specified
  - Return new workspace details

- [ ] Implement `replicateWorkspaceVariables(ctx context.Context, sourceWorkspaceID, targetWorkspaceID string, overrides map[string]interface{}) ([]VariableReplication, error)`
  - Call `get_hcp_terraform_workspace_variables` for source workspace
  - Apply any variable overrides specified in request
  - Call `bulk_create_hcp_terraform_workspace_variables` for efficient creation
  - Handle variable type conversions (string, number, bool, HCL)
  - Handle sensitive variable replication
  - Return replication summary

- [ ] Implement `applyWorkspaceTags(ctx context.Context, workspaceID string, tags map[string]string) error`
  - Add workspace name tag automatically
  - Merge with any additional tags from request
  - Call `create_hcp_terraform_workspace_tag_bindings`
  - Handle tag conflicts and validation

### 2.3 MCP Tool Integration
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [ ] Add `WorkspaceCreator()` MCP tool function
  - Define tool schema for workspace creation
  - Handle WorkspaceCreationRequest parameters
  - Call orchestrator's createReplicatedWorkspace method
  - Return formatted creation summary

---

## Phase 3: Configuration Upload Flow

### 3.1 Data Types & Structures
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [ ] Add `ConfigUploadRequest` struct
  - `WorkspaceID string`
  - `ConfigContent string` (base64 encoded tar.gz)
  - `AutoQueueRuns bool`
  - `Speculative bool`
  - `WaitForUpload bool`
  - `TimeoutMinutes int`

- [ ] Add `ConfigUploadResponse` struct
  - `ConfigurationVersionID string`
  - `UploadStatus string`
  - `UploadURL string`
  - `QueuedRunID string` (optional)
  - `ProcessingTimeSeconds int`
  - `Errors []string`

- [ ] Add `UploadStatus` constants
  - `UploadStatusPending`
  - `UploadStatusUploaded`
  - `UploadStatusProcessing`
  - `UploadStatusReady`
  - `UploadStatusErrored`

### 3.2 Upload Manager Component
**File**: `pkg/orchestrator/upload_manager.go` (NEW)

**TODO Items**:
- [ ] Create `UploadManager` struct
  - Add HTTP client for file uploads
  - Add status polling configuration
  - Add timeout and retry settings

- [ ] Implement `CreateConfigurationVersion(ctx context.Context, workspaceID string, options ConfigUploadRequest) (*ConfigVersionInfo, error)`
  - Call `create_hcp_terraform_configuration_version`
  - Extract upload URL and configuration version ID
  - Return structured information

- [ ] Implement `UploadConfigurationFiles(ctx context.Context, uploadURL, configContent string) error`
  - Decode base64 content
  - Upload tar.gz to the provided URL
  - Handle upload failures and retries
  - Validate successful upload response

- [ ] Implement `WaitForProcessing(ctx context.Context, configVersionID string, timeoutMinutes int) (*ConfigVersionStatus, error)`
  - Poll configuration version status
  - Wait for processing completion
  - Handle timeout scenarios
  - Return final status

### 3.3 Analysis Steps Extension
**File**: `pkg/orchestrator/analysis_steps.go` (EXTEND)

**TODO Items**:
- [ ] Implement `uploadConfiguration(ctx context.Context, req ConfigUploadRequest) (*ConfigUploadResponse, error)`
  - Create configuration version using UploadManager
  - Upload configuration files
  - Optionally wait for processing completion
  - Check for queued runs if auto_queue_runs is enabled
  - Return comprehensive upload status

### 3.4 MCP Tool Integration
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [ ] Add `ConfigurationUploader()` MCP tool function
  - Define tool schema for configuration upload
  - Handle ConfigUploadRequest parameters
  - Call orchestrator's uploadConfiguration method
  - Return upload status and any queued runs

---

## Phase 4: State Management Flow

### 4.1 Data Types & Structures
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [ ] Add `StateManagementRequest` struct
  - `SourceWorkspaceID string`
  - `TargetWorkspaceID string`
  - `CopyState bool`
  - `StateModifications map[string]interface{}` (optional)
  - `PreserveLocking bool`
  - `ForceUnlock bool` (optional)

- [ ] Add `StateManagementResponse` struct
  - `SourceStateVersionID string`
  - `TargetStateVersionID string`
  - `StateCopied bool`
  - `StateModified bool`
  - `LockingStatus string`
  - `Serial int`
  - `Lineage string`
  - `Errors []string`

- [ ] Add `StateModification` struct
  - `ModificationType string` (resource_rename, tag_update, etc.)
  - `SourcePath string`
  - `TargetPath string`
  - `NewValue interface{}`

### 4.2 State Manager Component
**File**: `pkg/orchestrator/state_manager.go` (NEW)

**TODO Items**:
- [ ] Create `StateManager` struct
  - Add JSON parser for state files
  - Add configuration for state modification rules
  - Add locking timeout settings

- [ ] Implement `DownloadState(ctx context.Context, workspaceID string) (*StateContent, error)`
  - Call `get_hcp_terraform_current_state_version`
  - Call `download_hcp_terraform_state_version`
  - Parse state JSON content
  - Extract metadata (serial, lineage, terraform_version)

- [ ] Implement `ModifyStateForNewWorkspace(stateContent *StateContent, modifications map[string]interface{}) (*StateContent, error)`
  - Update resource names if specified
  - Update resource tags to match new workspace
  - Modify provider configurations if needed
  - Update state metadata (increment serial)
  - Preserve resource dependencies

- [ ] Implement `UploadState(ctx context.Context, workspaceID string, stateContent *StateContent) error`
  - Call `lock_hcp_terraform_workspace` for safety
  - Call `create_hcp_terraform_state_version`
  - Call `unlock_hcp_terraform_workspace`
  - Handle locking failures and retries

- [ ] Implement `ValidateStateConsistency(stateContent *StateContent) error`
  - Check state file JSON schema
  - Validate resource references
  - Check for circular dependencies
  - Validate terraform version compatibility

### 4.3 Analysis Steps Extension
**File**: `pkg/orchestrator/analysis_steps.go` (EXTEND)

**TODO Items**:
- [ ] Implement `manageState(ctx context.Context, req StateManagementRequest) (*StateManagementResponse, error)`
  - Download state from source workspace if CopyState is true
  - Apply state modifications if specified
  - Upload modified state to target workspace
  - Handle workspace locking throughout the process
  - Return comprehensive state management status

### 4.4 MCP Tool Integration
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [ ] Add `StateManager()` MCP tool function
  - Define tool schema for state management
  - Handle StateManagementRequest parameters
  - Call orchestrator's manageState method
  - Return state management results

---

## Phase 5: Remote State Setup Flow

### 5.1 Data Types & Structures
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [ ] Add `RemoteStateSetupRequest` struct
  - `ProducerWorkspaceID string`
  - `ConsumerWorkspaceIDs []string`
  - `GlobalRemoteState bool`
  - `Operation string` (add, remove, replace)

- [ ] Add `RemoteStateSetupResponse` struct
  - `ProducerWorkspaceID string`
  - `ConfiguredConsumers []string`
  - `RemovedConsumers []string`
  - `GlobalStateEnabled bool`
  - `Errors []string`

- [ ] Add `RemoteStateOperation` constants
  - `OperationAdd`
  - `OperationRemove`
  - `OperationReplace`

### 5.2 Analysis Steps Extension
**File**: `pkg/orchestrator/analysis_steps.go` (EXTEND)

**TODO Items**:
- [ ] Implement `setupRemoteState(ctx context.Context, req RemoteStateSetupRequest) (*RemoteStateSetupResponse, error)`
  - Get current remote state consumers using `get_hcp_terraform_remote_state_consumers`
  - Based on operation type:
    - Add: Call `add_hcp_terraform_remote_state_consumers`
    - Remove: Call `remove_hcp_terraform_remote_state_consumers`  
    - Replace: Remove all existing, then add new consumers
  - Update workspace global remote state setting if specified
  - Return comprehensive setup status

### 5.3 MCP Tool Integration
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [ ] Add `RemoteStateSetup()` MCP tool function
  - Define tool schema for remote state setup
  - Handle RemoteStateSetupRequest parameters
  - Call orchestrator's setupRemoteState method
  - Return remote state configuration results

---

## Phase 6: Complete Workspace Replication Orchestrator

### 6.1 Main Replication Types
**File**: `pkg/orchestrator/types.go` (EXTEND)

**TODO Items**:
- [ ] Add `WorkspaceReplicationRequest` struct
  - `SourceWorkspaceID string`
  - `NewWorkspaceName string`
  - `OrganizationName string`
  - `ProjectID string` (optional)
  - `CopyState bool`
  - `SetupRemoteState bool`
  - `TagUpdates map[string]string` (optional)
  - `VariableOverrides map[string]interface{}` (optional)
  - `ConsumerWorkspaceIDs []string` (optional)

- [ ] Add `WorkspaceReplicationResponse` struct
  - `SourceWorkspaceID string`
  - `NewWorkspaceID string`
  - `NewWorkspaceName string`
  - `StepsCompleted []string`
  - `StepsFailed []string`
  - `AnalysisResults *WorkspaceAnalysisResponse`
  - `ConfigPreparationResults *ConfigPreparationResponse`
  - `CreationResults *WorkspaceCreationResponse`
  - `UploadResults *ConfigUploadResponse`
  - `StateResults *StateManagementResponse`
  - `RemoteStateResults *RemoteStateSetupResponse`
  - `TotalTimeSeconds int`
  - `Errors []string`

### 6.2 Main Orchestrator Extension
**File**: `pkg/orchestrator/workspace_analyzer.go` (EXTEND)

**TODO Items**:
- [ ] Add `ReplicateWorkspace(ctx context.Context, req WorkspaceReplicationRequest) (*WorkspaceReplicationResponse, error)`
  - Initialize response structure with timing
  - **Step 1**: Call existing `AnalyzeWorkspace` for source analysis
  - **Step 2**: Call `prepareConfiguration` to download and modify configs
  - **Step 3**: Call `createReplicatedWorkspace` to create new workspace
  - **Step 4**: Call `uploadConfiguration` to upload modified configs
  - **Step 5**: Call `manageState` if CopyState is true
  - **Step 6**: Call `setupRemoteState` if SetupRemoteState is true
  - Implement comprehensive error handling with rollback
  - Track timing for each step
  - Return detailed results for all steps

- [ ] Add `rollbackFailedReplication(ctx context.Context, workspaceID string, completedSteps []string) error`
  - Delete created workspace if creation completed
  - Clean up any uploaded configurations
  - Remove any partially configured remote state
  - Log rollback actions for audit trail

### 6.3 Error Handling & Recovery
**File**: `pkg/orchestrator/error_handler.go` (NEW)

**TODO Items**:
- [ ] Create `ReplicationErrorHandler` struct
  - Add error categorization (recoverable, non-recoverable)
  - Add retry logic for transient failures
  - Add rollback strategy definitions

- [ ] Implement `HandleReplicationError(ctx context.Context, err error, step string, workspaceID string) (*ErrorResolution, error)`
  - Categorize error types (authentication, permission, rate limit, etc.)
  - Determine if error is recoverable
  - Execute appropriate rollback actions
  - Provide actionable error messages

- [ ] Implement `RetryWithBackoff(ctx context.Context, operation func() error, maxRetries int) error`
  - Exponential backoff for rate limiting
  - Different retry strategies per error type
  - Respect context cancellation

### 6.4 Main MCP Tool
**File**: `pkg/tools/hcp_terraform/workspace_orchestrator.go` (EXTEND)

**TODO Items**:
- [ ] Add `WorkspaceReplicator()` MCP tool function
  - Define comprehensive tool schema for workspace replication
  - Handle WorkspaceReplicationRequest parameters
  - Call orchestrator's ReplicateWorkspace method
  - Return detailed replication results
  - Handle long-running operation status

---

## Phase 7: Dependencies & Testing

### 7.1 External Dependencies
**File**: `go.mod` (UPDATE)

**TODO Items**:
- [ ] Add HCL parsing dependencies
  - `github.com/hashicorp/hcl/v2`
  - `github.com/hashicorp/terraform-config-inspect`
  - `github.com/zclconf/go-cty`
- [ ] Add JSON processing for state files
  - `github.com/tidwall/gjson` (for JSON path operations)
- [ ] Add HTTP utilities for uploads
  - Ensure existing HTTP client supports multipart uploads

### 7.2 Unit Tests
**TODO Items**:
- [ ] Create `pkg/orchestrator/config_modifier_test.go`
  - Test HCL parsing with various provider types
  - Test tag injection for AWS, Azure, GCP providers
  - Test variable reference updates
  - Test configuration serialization

- [ ] Create `pkg/orchestrator/state_manager_test.go`
  - Test state file parsing and modification
  - Test state lineage and serial handling
  - Test state validation

- [ ] Create `pkg/orchestrator/upload_manager_test.go`
  - Test configuration upload process
  - Test status polling and timeout handling
  - Mock HTTP upload endpoints

- [ ] Extend `pkg/orchestrator/workspace_analyzer_test.go`
  - Test complete replication workflow
  - Test error handling and rollback scenarios
  - Test step-by-step progress tracking

### 7.3 Integration Tests
**File**: `e2e/workspace_replication_e2e_test.go` (NEW)

**TODO Items**:
- [ ] Create end-to-end replication test
  - Set up source workspace with configuration
  - Execute complete replication workflow
  - Verify all components are correctly replicated
  - Test with different provider types (AWS, Azure, GCP)

- [ ] Create error scenario tests
  - Test partial failure recovery
  - Test rollback functionality
  - Test rate limiting handling

### 7.4 Documentation
**TODO Items**:
- [ ] Update `README.md` with replication examples
- [ ] Create `docs/workspace_replication.md` with detailed usage
- [ ] Add inline code documentation for all new components
- [ ] Create example replication requests in `examples/`

---

## Implementation Order & Dependencies

### Sprint 1: Foundation (Week 1) ✅ **COMPLETED**
1. ✅ Complete Phase 1: Configuration Preparation Flow
2. ✅ Add HCL dependencies and basic parsing
3. ✅ Create ConfigModifier with basic tag injection

### Sprint 2: Core Replication (Week 2) 🚧 **IN PROGRESS**
4. ⏳ Complete Phase 2: Workspace Creation Flow
5. ⏳ Complete Phase 3: Configuration Upload Flow
6. ⏳ Basic end-to-end test for config preparation → creation → upload

### Sprint 3: Advanced Features (Week 3) ⏭️ **PLANNED**
7. ⏳ Complete Phase 4: State Management Flow
8. ⏳ Complete Phase 5: Remote State Setup Flow
9. ⏳ Add comprehensive error handling

### Sprint 4: Integration & Polish (Week 4) ⏭️ **PLANNED**
10. ⏳ Complete Phase 6: Main Replication Orchestrator
11. ⏳ Complete Phase 7: Testing & Documentation
12. ⏳ Performance optimization and production readiness

## Success Criteria

### Functional Requirements
- [ ] Successfully replicate workspace with all settings preserved
- [ ] Modify configuration files to add appropriate tags
- [ ] Copy all variables with override capability
- [ ] Optionally copy state with proper modifications
- [ ] Set up remote state consumers when requested
- [ ] Comprehensive error handling with rollback

### Non-Functional Requirements
- [ ] Complete replication in under 5 minutes for typical workspaces
- [ ] Handle workspaces with up to 100 variables
- [ ] Support configuration files up to 10MB
- [ ] Graceful handling of API rate limits
- [ ] Comprehensive logging and audit trail

### Quality Requirements
- [ ] 90%+ test coverage for all new components
- [ ] Integration tests for common provider types
- [ ] Performance benchmarks for large workspaces
- [ ] Security review for sensitive data handling
- [ ] Documentation with practical examples

---

## Notes & Considerations

### Security Considerations
- Sensitive variables must be handled securely during replication
- State files may contain sensitive information requiring careful handling
- Authentication tokens should never be logged or exposed

### Performance Considerations  
- Large configuration files may require streaming processing
- Bulk variable creation is more efficient than individual creation
- State file modifications should be done in memory when possible
- Consider parallel processing where safe (e.g., variable creation)

### Provider Compatibility
- Tag injection must support provider-specific syntax
- AWS: `default_tags` in provider block
- Azure: `default_tags` in provider block
- GCP: `default_labels` in provider block
- Handle provider aliases and multiple provider instances

### Future Enhancements
- Support for Terraform modules and module sources
- Workspace template system for common replication patterns
- Scheduled replication for disaster recovery
- Replication across different HCP Terraform organizations
- Integration with CI/CD pipelines for automated workspace management

This plan provides a comprehensive roadmap for implementing complete workspace replication functionality while maintaining clean architecture and thorough testing.
