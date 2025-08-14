# Tools for Terraform Workspace Replication

## Overview

This document outlines the tools required to implement workspace replication functionality in the Terraform MCP server. The goal is to create atomic, reusable tools that can be composed to replicate a Terraform workspace while changing the workspace name and adding appropriate tags to resources.

## Use Case: Workspace Replication

**Objective**: Replicate an existing Terraform workspace and create a new workspace from it with:
- A new workspace name (user-specified)
- All the same configuration, variables, and settings as the source workspace
- Tags on resources matching the new workspace name

## Atomic Tools Required

### 1. Workspace Management Tools

#### 1.1 `get_hcp_terraform_workspaces`
- **Purpose**: List and filter workspaces in an organization
- **Functionality**: 
  - List all workspaces in an organization
  - Filter by name, tags, project, etc.
  - Support pagination
- **Parameters**:
  - `organization_name` (required)
  - `workspace_name` (optional, for filtering)
  - `page_size`, `page_number` (optional)
  - `search_filters` (optional, for tags, project, etc.)
- **Returns**: List of workspace objects with metadata

#### 1.2 `get_hcp_terraform_workspace_details`
- **Purpose**: Get complete details of a specific workspace
- **Functionality**: Retrieve all workspace configuration and metadata
- **Parameters**:
  - `workspace_id` OR (`organization_name` + `workspace_name`)
  - `include_relationships` (optional, for related resources)
- **Returns**: Complete workspace object with all attributes and relationships

#### 1.3 `create_hcp_terraform_workspace`
- **Purpose**: Create a new workspace with specified configuration
- **Functionality**: Create workspace with all configuration options
- **Parameters**:
  - `organization_name` (required)
  - `workspace_name` (required)
  - `workspace_config` (object with all workspace attributes)
  - `project_id` (optional)
  - `tag_bindings` (optional)
- **Returns**: Created workspace object

#### 1.4 `update_hcp_terraform_workspace`
- **Purpose**: Update an existing workspace configuration
- **Functionality**: Modify workspace settings, tags, etc.
- **Parameters**:
  - `workspace_id` (required)
  - `updates` (object with attributes to update)
- **Returns**: Updated workspace object

#### 1.5 `delete_hcp_terraform_workspace`
- **Purpose**: Delete a workspace (safe or force delete)
- **Functionality**: Remove workspace with safety checks
- **Parameters**:
  - `workspace_id` (required)
  - `force_delete` (optional, boolean)
- **Returns**: Success/failure status

### 2. Variables Management Tools

#### 2.1 `get_hcp_terraform_workspace_variables`
- **Purpose**: List all variables for a workspace
- **Functionality**: Retrieve Terraform and environment variables
- **Parameters**:
  - `workspace_id` (required)
- **Returns**: List of variable objects

#### 2.2 `create_hcp_terraform_workspace_variable`
- **Purpose**: Create a new variable in a workspace
- **Functionality**: Add Terraform or environment variable
- **Parameters**:
  - `workspace_id` (required)
  - `variable_config` (object with key, value, category, etc.)
- **Returns**: Created variable object

#### 2.3 `update_hcp_terraform_workspace_variable`
- **Purpose**: Update an existing workspace variable
- **Functionality**: Modify variable value, description, etc.
- **Parameters**:
  - `workspace_id` (required)
  - `variable_id` (required)
  - `updates` (object with attributes to update)
- **Returns**: Updated variable object

#### 2.4 `delete_hcp_terraform_workspace_variable`
- **Purpose**: Delete a workspace variable
- **Functionality**: Remove variable from workspace
- **Parameters**:
  - `workspace_id` (required)
  - `variable_id` (required)
- **Returns**: Success/failure status

#### 2.5 `bulk_create_hcp_terraform_workspace_variables`
- **Purpose**: Create multiple variables at once
- **Functionality**: Efficiently create many variables
- **Parameters**:
  - `workspace_id` (required)
  - `variables` (array of variable config objects)
- **Returns**: List of created variable objects

### 3. Configuration Management Tools

#### 3.1 `get_hcp_terraform_configuration_versions`
- **Purpose**: List configuration versions for a workspace
- **Functionality**: Retrieve all configuration versions
- **Parameters**:
  - `workspace_id` (required)
  - `page_size`, `page_number` (optional)
- **Returns**: List of configuration version objects

#### 3.2 `get_hcp_terraform_configuration_version_details`
- **Purpose**: Get details of a specific configuration version
- **Functionality**: Retrieve configuration version metadata
- **Parameters**:
  - `configuration_version_id` (required)
  - `include_ingress_attributes` (optional)
- **Returns**: Configuration version object with details

#### 3.3 `download_hcp_terraform_configuration_files`
- **Purpose**: Download configuration files from a configuration version
- **Functionality**: Get the tar.gz archive of Terraform files
- **Parameters**:
  - `configuration_version_id` (required)
  - `output_path` (optional, where to save files)
- **Returns**: File content or success status

#### 3.4 `create_hcp_terraform_configuration_version`
- **Purpose**: Create a new configuration version
- **Functionality**: Create version and get upload URL
- **Parameters**:
  - `workspace_id` (required)
  - `auto_queue_runs` (optional, boolean)
  - `speculative` (optional, boolean)
- **Returns**: Configuration version object with upload URL

#### 3.5 `upload_hcp_terraform_configuration_files`
- **Purpose**: Upload configuration files to a configuration version
- **Functionality**: Upload tar.gz archive to the upload URL
- **Parameters**:
  - `upload_url` (required)
  - `configuration_files` (required, file content or path)
- **Returns**: Success/failure status

### 4. State Management Tools

#### 4.1 `get_hcp_terraform_current_state_version`
- **Purpose**: Get the current state version for a workspace
- **Functionality**: Retrieve the active state
- **Parameters**:
  - `workspace_id` (required)
- **Returns**: Current state version object

#### 4.2 `get_hcp_terraform_state_versions`
- **Purpose**: List state versions for a workspace
- **Functionality**: Retrieve all state versions
- **Parameters**:
  - `workspace_id` (required)
  - `page_size`, `page_number` (optional)
- **Returns**: List of state version objects

#### 4.3 `download_hcp_terraform_state_version`
- **Purpose**: Download state data from a state version
- **Functionality**: Get the state file content
- **Parameters**:
  - `state_version_id` (required)
  - `format` (optional: 'raw' or 'json')
- **Returns**: State file content

#### 4.4 `create_hcp_terraform_state_version`
- **Purpose**: Create a new state version
- **Functionality**: Upload new state to workspace
- **Parameters**:
  - `workspace_id` (required)
  - `state_content` (required)
  - `serial` (required)
  - `lineage` (optional)
- **Returns**: Created state version object

### 5. Tag Management Tools

#### 5.1 `get_hcp_terraform_workspace_tags`
- **Purpose**: Get all tags for a workspace
- **Functionality**: Retrieve both flat tags and key-value tag bindings
- **Parameters**:
  - `workspace_id` (required)
  - `include_inherited` (optional, boolean)
- **Returns**: Tag objects (flat tags and tag bindings)

#### 5.2 `create_hcp_terraform_workspace_tag_bindings`
- **Purpose**: Add key-value tags to a workspace
- **Functionality**: Create tag bindings
- **Parameters**:
  - `workspace_id` (required)
  - `tag_bindings` (array of key-value pairs)
- **Returns**: Created tag binding objects

#### 5.3 `update_hcp_terraform_workspace_tag_bindings`
- **Purpose**: Update existing tag bindings
- **Functionality**: Modify tag values
- **Parameters**:
  - `workspace_id` (required)
  - `tag_updates` (array of key-value pairs)
- **Returns**: Updated tag binding objects

#### 5.4 `delete_hcp_terraform_workspace_tags`
- **Purpose**: Remove tags from a workspace
- **Functionality**: Delete specified tags
- **Parameters**:
  - `workspace_id` (required)
  - `tag_keys` (array of tag keys to remove)
- **Returns**: Success/failure status

### 6. Workspace Locking Tools

#### 6.1 `lock_hcp_terraform_workspace`
- **Purpose**: Lock a workspace to prevent concurrent operations
- **Functionality**: Set workspace lock with reason
- **Parameters**:
  - `workspace_id` (required)
  - `reason` (optional)
- **Returns**: Workspace object with lock status

#### 6.2 `unlock_hcp_terraform_workspace`
- **Purpose**: Unlock a workspace
- **Functionality**: Remove workspace lock
- **Parameters**:
  - `workspace_id` (required)
  - `force_unlock` (optional, boolean)
- **Returns**: Workspace object with lock status

### 7. Remote State Consumer Tools

#### 7.1 `get_hcp_terraform_remote_state_consumers`
- **Purpose**: Get workspaces that can access this workspace's state
- **Functionality**: List remote state consumers
- **Parameters**:
  - `workspace_id` (required)
- **Returns**: List of workspace objects that can consume state

#### 7.2 `add_hcp_terraform_remote_state_consumers`
- **Purpose**: Add workspaces as remote state consumers
- **Functionality**: Grant state access to other workspaces
- **Parameters**:
  - `workspace_id` (required)
  - `consumer_workspace_ids` (array)
- **Returns**: Success/failure status

#### 7.3 `remove_hcp_terraform_remote_state_consumers`
- **Purpose**: Remove remote state consumer access
- **Functionality**: Revoke state access
- **Parameters**:
  - `workspace_id` (required)
  - `consumer_workspace_ids` (array)
- **Returns**: Success/failure status

## Implementation Plan

### Phase 1: Core Workspace Tools (Priority 1)
1. `get_hcp_terraform_workspaces`
2. `get_hcp_terraform_workspace_details`
3. `create_hcp_terraform_workspace`
4. `update_hcp_terraform_workspace`

### Phase 2: Variables Management (Priority 1)
1. `get_hcp_terraform_workspace_variables`
2. `bulk_create_hcp_terraform_workspace_variables`
3. `create_hcp_terraform_workspace_variable`

### Phase 3: Configuration Management (Priority 2)
1. `get_hcp_terraform_configuration_versions`
2. `download_hcp_terraform_configuration_files`
3. `create_hcp_terraform_configuration_version`
4. `upload_hcp_terraform_configuration_files`

### Phase 4: State Management (Priority 2)
1. `get_hcp_terraform_current_state_version`
2. `download_hcp_terraform_state_version`
3. `create_hcp_terraform_state_version`

### Phase 5: Supporting Tools (Priority 3)
1. Tag management tools
2. Workspace locking tools
3. Remote state consumer tools

## Workspace Replication Workflow

Using the atomic tools above, the workspace replication workflow would be:

1. **Source Analysis**:
   - `get_hcp_terraform_workspace_details` (source workspace)
   - `get_hcp_terraform_workspace_variables` (source workspace)
   - `get_hcp_terraform_configuration_versions` (source workspace)
   - `get_hcp_terraform_current_state_version` (source workspace)

2. **Configuration Preparation**:
   - `download_hcp_terraform_configuration_files` (latest config)
   - Modify configuration to add/update tags with new workspace name
   - `download_hcp_terraform_state_version` (if needed)

3. **Workspace Creation**:
   - `create_hcp_terraform_workspace` (with new name and config)
   - `bulk_create_hcp_terraform_workspace_variables` (copy all variables)
   - `create_hcp_terraform_workspace_tag_bindings` (add workspace name tag)

4. **Configuration Upload**:
   - `create_hcp_terraform_configuration_version`
   - `upload_hcp_terraform_configuration_files` (modified config)

5. **State Management** (if copying state):
   - `lock_hcp_terraform_workspace`
   - `create_hcp_terraform_state_version` (modified state)
   - `unlock_hcp_terraform_workspace`

## Design Principles

1. **Atomic Operations**: Each tool performs one specific operation
2. **Idempotent**: Tools can be called multiple times safely
3. **Error Handling**: Comprehensive error responses with actionable information
4. **Composable**: Tools can be combined to create complex workflows
5. **Reusable**: Tools are generic enough for multiple use cases
6. **Consistent**: All tools follow the same parameter and response patterns
7. **Well-Documented**: Clear descriptions, parameters, and examples

## Additional Considerations

### Authentication
All tools will use the existing HCP Terraform authentication mechanism (Bearer token from environment variable or parameter).

### Rate Limiting
Tools should implement appropriate rate limiting and retry logic for HCP Terraform API limits.

### Validation
Input validation should be performed before making API calls to provide early feedback on errors.

### Logging
Comprehensive logging for debugging and audit trails.

### Configuration File Modification
For the replication use case, we need utilities to:
- Parse Terraform configuration files
- Add/modify default tags in provider blocks
- Update variable values if needed
- Handle different provider types (AWS, Azure, GCP, etc.)

This toolset provides a comprehensive foundation for workspace replication while maintaining the flexibility to support many other HCP Terraform automation use cases.

## Detailed Workspace Replication Tool Sequence

Based on the atomic tools defined above, here is the detailed step-by-step sequence for achieving Terraform workspace replication:

### **Step 1: Source Analysis** 
Gather all information from the source workspace:
1. `get_hcp_terraform_workspace_details` - Get complete workspace configuration and metadata from the source workspace
2. `get_hcp_terraform_workspace_variables` - Retrieve all Terraform and environment variables from the source workspace
3. `get_hcp_terraform_configuration_versions` - List all configuration versions to identify the latest/desired version
4. `get_hcp_terraform_current_state_version` - Get the current state version (if state copying is required)

### **Step 2: Configuration Preparation**
Download and modify configuration files:
1. `download_hcp_terraform_configuration_files` - Download the latest configuration files from the source workspace
2. **Configuration Modification Step**: Parse and modify the downloaded configuration files to:
   - Add/update default tags with the new workspace name
   - Update any workspace-specific variable references
   - Modify provider configurations if needed
3. `download_hcp_terraform_state_version` - Download state file (optional, only if state copying is needed)

### **Step 3: Workspace Creation**
Create the new workspace and set up variables/tags:
1. `create_hcp_terraform_workspace` - Create new workspace with the specified name and configuration copied from source
2. `bulk_create_hcp_terraform_workspace_variables` - Copy all variables from the source workspace to the new workspace
3. `create_hcp_terraform_workspace_tag_bindings` - Add workspace name tag and any other required tags to the new workspace

### **Step 4: Configuration Upload**
Upload the modified configuration to the new workspace:
1. `create_hcp_terraform_configuration_version` - Create a new configuration version in the target workspace and get upload URL
2. `upload_hcp_terraform_configuration_files` - Upload the modified configuration files to the new workspace

### **Step 5: State Management** (Optional - if copying state)
Handle state file replication with proper locking:
1. `lock_hcp_terraform_workspace` - Lock the new workspace to prevent concurrent operations during state upload
2. `create_hcp_terraform_state_version` - Upload the modified state file to the new workspace
3. `unlock_hcp_terraform_workspace` - Unlock the workspace to allow normal operations

### **Error Handling and Rollback Considerations**

If any step fails during the replication process:
- Use `delete_hcp_terraform_workspace` to clean up partially created workspaces
- Implement proper error logging and user notification
- Consider implementing checkpoint/resume functionality for large workspaces

### **Key Benefits of This Sequenced Approach**

- **Atomic Operations**: Each step uses specific, single-purpose tools that can be tested and validated independently
- **Safety**: Workspace locking prevents concurrent modifications during critical state operations
- **Flexibility**: State copying is optional and can be skipped if starting with a fresh state
- **Consistency**: All workspace settings, variables, and configurations are preserved exactly
- **Customization**: Configuration files are modified during the process to update tags and workspace-specific settings
- **Rollback Capability**: Failed operations can be safely rolled back without affecting the source workspace
- **Audit Trail**: Each step provides clear logging and status information for troubleshooting

This detailed sequence provides a robust, production-ready foundation for workspace replication while maintaining the ability to customize the replicated workspace during the process.

### Replication workflow sequence 
You can now build complete workspace replication workflows using these atomic tools following the exact sequence outlined in the original document:

Source Analysis → Get workspace details, variables, configurations, state
Configuration Preparation → Download and modify configs
Workspace Creation → Create new workspace with variables and tags
Configuration Upload → Upload modified configurations
State Management → Handle state with proper locking
Remote State Setup → Configure state sharing between workspaces
The entire toolkit is now complete, tested, and ready for production use!

