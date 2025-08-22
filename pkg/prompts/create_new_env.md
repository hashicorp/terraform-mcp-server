# HCP Terraform Environment Setup Guide

## ⚠️ SEMI-AUTOMATED CONFIGURATION RECOMMENDED ⚠️

**Recommended Workspace Settings (Safe Semi-Automated):**
- auto_apply: **false** (Manual approval always required)
- auto_queue_runs: **true** (Automatic planning for faster feedback)
- execution_mode: "remote" (Secure execution environment)

**Why These Settings:**
- **Automatic Planning**: Configuration changes trigger immediate plan generation
- **Manual Control**: Every apply still requires explicit human approval
- **Fast Feedback**: Quick identification of configuration issues
- **Audit Trail**: All deployments require manual approval and comments
- **Best Practice**: Balances efficiency with safety for most environments

**Workflow for All Changes:**
1. Upload configuration → Automatic plan creation
2. Plan review with `get_hcp_terraform_plan`
3. Manual approval with `apply_hcp_terraform_run`
4. Monitoring with `get_hcp_terraform_run`

**Alternative Manual Controls (if needed):**
- Use `create_hcp_terraform_run` for manual plan creation
- Use `cancel_hcp_terraform_run` to stop operations if needed
- Use `discard_hcp_terraform_run` to cancel planned changes

## Overview

This guide provides a step-by-step workflow for creating a new HCP Terraform workspace environment with configuration and state management. Use this guide to establish a complete, functional Terraform environment in HCP Terraform.

## Prerequisites

Before starting, ensure you have:
- Access to an HCP Terraform organization
- Authentication token (via `HCP_TERRAFORM_TOKEN` environment variable or explicit token)
- Terraform configuration files ready for upload (optional)
- Required variable values and workspace settings

## Step-by-Step Workflow

### Step 1: Workspace Creation
Create the foundational workspace with basic configuration.

**Tool:** `create_hcp_terraform_workspace`

**Required Information:**
- `organization_name` - Your HCP Terraform organization name
- `workspace_name` - Unique name for the new workspace

**Optional Configuration:**
- `description` - Workspace description for documentation
- `execution_mode` - Choose: `remote` (default), `local`, or `agent`
- `terraform_version` - Specify Terraform version (defaults to latest)
- `auto_apply` - Enable automatic plan application (default: false)
- `working_directory` - Subdirectory for Terraform execution
- `project_id` - Assign to specific project (defaults to organization default)
- `tag_bindings` - Initial tags (can add more later)

**Example:** Create workspace for staging environment
```json
{
  "organization_name": "my-org",
  "workspace_name": "staging-environment",
  "description": "Staging environment for application testing",
  "execution_mode": "remote",
  "auto_apply": false,
  "tag_bindings": [
    {"key": "environment", "value": "staging"},
    {"key": "team", "value": "platform"}
  ]
}
```

### Step 2: Variable Configuration
Set up Terraform variables and environment variables needed for your infrastructure.

**For Multiple Variables:** `bulk_create_hcp_terraform_workspace_variables`

**Required Information:**
- `workspace_id` - ID from Step 1 workspace creation response
- `variables` - Array of variable objects

**For Single Variables:** `create_hcp_terraform_workspace_variable`

**Variable Object Structure:**
- `key` - Variable name
- `value` - Variable value  
- `category` - Type: `terraform` or `env`
- `sensitive` - Mark as sensitive (default: false)
- `hcl` - HCL format for terraform variables (default: false)
- `description` - Variable documentation

**Example:** Configure infrastructure variables
```json
{
  "workspace_id": "ws-abc123",
  "variables": [
    {
      "key": "aws_region",
      "value": "us-west-2", 
      "category": "terraform",
      "description": "AWS region for resources"
    },
    {
      "key": "environment",
      "value": "staging",
      "category": "terraform", 
      "description": "Environment name for resource tagging"
    },
    {
      "key": "AWS_ACCESS_KEY_ID",
      "value": "AKIA...",
      "category": "env",
      "sensitive": true,
      "description": "AWS access key for provider authentication"
    }
  ]
}
```

### Step 3: Additional Tag Management (Optional)
Add environment-specific tags for resource organization and cost tracking.

**Tool:** `create_hcp_terraform_workspace_tag_bindings`

**Required Information:**
- `workspace_id` - Workspace ID from Step 1
- `tag_bindings` - Array of key-value tag pairs

**Example:** Add operational tags
```json
{
  "workspace_id": "ws-abc123", 
  "tag_bindings": [
    {"key": "cost-center", "value": "engineering"},
    {"key": "owner", "value": "platform-team"},
    {"key": "backup-schedule", "value": "daily"}
  ]
}
```

### Step 4: Configuration Upload (If You Have Terraform Files)
Upload your Terraform configuration files to the workspace.

**Step 4a:** Create configuration version
**Tool:** `create_hcp_terraform_configuration_version`

**Required Information:**
- `workspace_id` - Workspace ID from Step 1
- `auto_queue_runs` - Whether to auto-trigger plans (default: false)
- `speculative` - Mark as speculative run (default: false)

**Step 4b:** Upload configuration files  
**Tool:** `upload_hcp_terraform_configuration_files`

**Required Information:**
- `upload_url` - URL from Step 4a response
- `configuration_files` - Base64-encoded tar.gz of your Terraform files

**Example Sequence:**
```json
// Step 4a
{
  "workspace_id": "ws-abc123",
  "auto_queue_runs": false,
  "speculative": false
}

// Step 4b (use upload_url from 4a response)
{
  "upload_url": "https://archivist.terraform.io/v1/...",
  "configuration_files": "H4sIAAAAA...base64content..."
}
```

### Step 5: State Management Setup (Optional)
Configure initial state if migrating from existing infrastructure or setting up remote state.

**For Existing State Migration:**

**Step 5a:** Prepare state data
- Export existing state: `terraform state pull > terraform.tfstate`
- Ensure state format compatibility
- Base64 encode the state file content

**Step 5b:** Upload state version
**Tool:** `create_hcp_terraform_state_version`

**Required Information:**
- `workspace_id` - Workspace ID from Step 1
- `state_content_base64` - Base64-encoded state file
- `serial` - State serial number (increment from existing)
- `lineage` - State lineage UUID (preserve from existing)

**Example:**
```json
{
  "workspace_id": "ws-abc123",
  "state_content_base64": "ewogICJ2ZXJzaW9uIjogNCwKICAi...",
  "serial": 1,
  "lineage": "12345678-1234-1234-1234-123456789012"
}
```

### Step 6: Infrastructure Deployment (Plan/Apply Management)
Once your workspace is configured, deploy your infrastructure. The approach depends on your workspace automation settings.

## Scenario A: Fully Automated (auto_queue_runs=true, auto_apply=true)

**Best for:** Development environments, simple automation workflows

When both automation flags are enabled:
1. Configuration uploads automatically trigger plan runs
2. Successful plans are automatically applied
3. No manual intervention required

**Setup during workspace creation:**
```json
{
  "organization_name": "my-org",
  "workspace_name": "dev-environment",
  "auto_apply": true,
  "queue_all_runs": true
}
```

**Configuration upload automatically triggers deployment:**
```json
{
  "workspace_id": "ws-abc123",
  "auto_queue_runs": true,
  "speculative": false
}
```

## Scenario B: Semi-Automated (auto_queue_runs=true, auto_apply=false)

**Best for:** Staging environments, workflows requiring plan review

Plans are automatically triggered but require manual approval:

1. **Upload triggers automatic planning:**
```json
{
  "workspace_id": "ws-abc123",
  "auto_queue_runs": true,
  "speculative": false
}
```

2. **Monitor run status:**
**Tool:** `list_hcp_terraform_runs`
```json
{
  "workspace_id": "ws-abc123",
  "status": "planned",
  "page_size": 10
}
```

3. **Review plan details:**
**Tool:** `get_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "include": ["plan"]
}
```

4. **Get detailed plan information:**
**Tool:** `get_hcp_terraform_plan`
```json
{
  "plan_id": "plan-abc123"
}
```

5. **Apply after review:**
**Tool:** `apply_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "comment": "Approved after review - deploying staging infrastructure"
}
```

**Or discard if issues found:**
**Tool:** `discard_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "comment": "Discarding due to unexpected resource changes"
}
```

## Scenario C: Manual Control (auto_queue_runs=false, auto_apply=false)

**Best for:** Production environments, compliance-required workflows

Complete manual control over all run operations:

1. **Create run manually:**
**Tool:** `create_hcp_terraform_run`
```json
{
  "workspace_id": "ws-abc123",
  "message": "Production deployment v1.2.3",
  "plan_only": false,
  "is_destroy": false,
  "refresh": true,
  "target_addrs": ["module.web_servers", "module.database"]
}
```

2. **Monitor run progress:**
**Tool:** `get_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "include": ["plan", "apply", "configuration-version"]
}
```

3. **List all workspace runs:**
**Tool:** `list_hcp_terraform_runs`
```json
{
  "workspace_id": "ws-abc123",
  "status": "planned",
  "operation": "plan_and_apply",
  "page_size": 20
}
```

4. **Review detailed plan:**
**Tool:** `get_hcp_terraform_plan`
```json
{
  "plan_id": "plan-abc123"
}
```

5. **Apply when ready:**
**Tool:** `apply_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "comment": "Production deployment approved by team lead"
}
```

## Advanced Run Management

### Emergency Operations

**Cancel running operation:**
**Tool:** `cancel_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "comment": "Canceling due to critical issue discovered",
  "force_cancel": false
}
```

**Force cancel (use with extreme caution):**
```json
{
  "run_id": "run-abc123",
  "comment": "Emergency force cancel",
  "force_cancel": true
}
```

### Destroy Operations

**Create destroy run:**
**Tool:** `create_hcp_terraform_run`
```json
{
  "workspace_id": "ws-abc123",
  "message": "Destroying staging environment",
  "is_destroy": true,
  "auto_apply": false,
  "target_addrs": ["module.staging_web"]
}
```

### Plan-Only Operations

**Create plan-only run for validation:**
```json
{
  "workspace_id": "ws-abc123",
  "message": "Validation run for configuration changes",
  "plan_only": true,
  "refresh": true
}
```

### Targeted Operations

**Target specific resources:**
```json
{
  "workspace_id": "ws-abc123",
  "message": "Update only web servers",
  "target_addrs": [
    "module.web_servers.aws_instance.web[0]",
    "module.web_servers.aws_instance.web[1]"
  ]
}
```

**Force resource replacement:**
```json
{
  "workspace_id": "ws-abc123",
  "message": "Replace database instances",
  "replace_addrs": [
    "module.database.aws_db_instance.primary"
  ]
}
```

### Detailed Run Monitoring

**Get complete run information:**
**Tool:** `get_hcp_terraform_run`
```json
{
  "run_id": "run-abc123",
  "include": ["plan", "apply", "configuration-version", "workspace", "created-by"]
}
```

**Get apply details:**
**Tool:** `get_hcp_terraform_apply`
```json
{
  "apply_id": "apply-abc123"
}
```

**Filter runs by criteria:**
**Tool:** `list_hcp_terraform_runs`
```json
{
  "workspace_id": "ws-abc123",
  "status": "errored",
  "operation": "plan_and_apply",
  "source": "api",
  "page_number": 1,
  "page_size": 50
}
```

## Run Status Flow

Understanding run status transitions:

1. **pending** → Run is queued
2. **planning** → Plan is being generated
3. **planned** → Plan complete, ready for apply/discard
4. **applying** → Apply in progress
5. **applied** → Successfully completed
6. **discarded** → Plan was discarded
7. **errored** → Failed during execution
8. **canceled** → Manually canceled

**Security Considerations:**
- **Production**: Always use manual approval (auto_apply=false)
- **Staging**: Semi-automated with plan review recommended
- **Development**: Full automation acceptable with proper safeguards
- **Destroy operations**: Always require manual approval regardless of automation settings
- **Force cancel**: Use only in emergencies, may leave infrastructure in inconsistent state

## Post-Setup Verification

After completing the setup:

1. **Verify Workspace:** Check workspace appears in HCP Terraform UI
2. **Test Variables:** Ensure all variables are properly configured
3. **Validate Configuration:** Run a plan to verify Terraform syntax
4. **Check Permissions:** Confirm team access and run permissions
5. **Test State:** Verify state operations work correctly
6. **Monitor Runs:** Check run status and logs in HCP Terraform UI
7. **Verify Infrastructure:** Confirm resources are created as expected

## Workflow Summary

### Basic Setup Flow
```
1. Create Workspace → 2. Configure Variables → 3. Add Tags → 4. Upload Config → 5. Setup State → 6. Deploy Infrastructure
     ↓                        ↓                    ↓              ↓               ↓                    ↓
[create_workspace]    [bulk_create_variables]  [create_tags]  [config_version]  [state_version]     [run_management]
```

### Run Management Scenarios

**Scenario A: Fully Automated (auto_queue_runs=true, auto_apply=true)**
```
Upload Config → Auto Plan → Auto Apply → Done
     ↓               ↓           ↓        ↓
[config_version] [automatic] [automatic] [applied]
```

**Scenario B: Semi-Automated (auto_queue_runs=true, auto_apply=false)**
```
Upload Config → Auto Plan → Review → Manual Apply/Discard
     ↓               ↓         ↓            ↓
[config_version] [automatic] [get_plan] [apply_run/discard_run]
```

**Scenario C: Manual Control (auto_queue_runs=false, auto_apply=false)**
```
Manual Plan → Review → Manual Apply → Monitor → Complete
     ↓          ↓           ↓           ↓          ↓
[create_run] [get_plan] [apply_run] [get_run] [get_apply]
```

### Emergency Operations
```
Cancel Run → Monitor Status → Cleanup
     ↓             ↓             ↓
[cancel_run] [get_run_status] [manual_fixes]
```

## Choosing the Right Automation Level

### Environment-Based Recommendations

**Development Environments:**
- **Settings:** `auto_queue_runs=true`, `auto_apply=true`
- **Rationale:** Fast iteration, low risk, immediate feedback
- **Workflow:** Upload config → automatic deployment
- **Best for:** Feature development, testing, experimentation

**Staging/Testing Environments:**
- **Settings:** `auto_queue_runs=true`, `auto_apply=false`
- **Rationale:** Automatic planning with human review before deployment
- **Workflow:** Upload config → auto plan → review → manual apply
- **Best for:** Integration testing, QA validation, pre-production verification

**Production Environments:**
- **Settings:** `auto_queue_runs=false`, `auto_apply=false`
- **Rationale:** Complete control, mandatory review, compliance requirements
- **Workflow:** Manual plan → review → manual apply → monitoring
- **Best for:** Critical infrastructure, regulated environments, high-stakes deployments

**Disaster Recovery/Emergency:**
- **Settings:** Manual control with `force_cancel` capability
- **Rationale:** Maximum control during crisis situations
- **Workflow:** Emergency assessment → targeted operations → careful monitoring
- **Best for:** Incident response, emergency fixes, rollbacks

### Compliance and Governance

**SOC 2 / ISO 27001 Environments:**
- Require manual approval for all production changes
- Implement change approval workflows
- Maintain audit trails with detailed comments
- Use `comment` fields for approval references

**Financial Services / HIPAA:**
- Multi-person approval required
- Complete audit logging
- No auto-apply in any environment
- Regular compliance validation runs

**Agile/DevOps Teams:**
- Progressive automation (dev → staging → prod)
- Feature branch protection with manual prod deployment
- Automated testing with manual production gates
- Continuous monitoring and rollback capabilities

## Error Handling

If any step fails:
- **Workspace Creation Failure:** Check organization permissions and name uniqueness
- **Variable Creation Failure:** Verify workspace_id and variable format
- **Configuration Upload Failure:** Check file format and upload URL validity  
- **State Upload Failure:** Verify state format and serial number sequence
- **Run Creation Failure:** Check workspace permissions and configuration status
- **Plan Failure:** Review Terraform syntax, variable values, and provider authentication
- **Apply Failure:** Verify plan approval, resource permissions, and infrastructure state
- **Run Cancellation Issues:** Check run status and use force_cancel only when necessary
- **Run Status Errors:** Monitor with `get_hcp_terraform_run` and check HCP Terraform UI for detailed logs

### Run-Specific Error Recovery

**Plan Errors:**
1. Check configuration syntax with `get_hcp_terraform_plan`
2. Verify variable values and provider credentials
3. Review Terraform version compatibility
4. Check workspace state consistency

**Apply Errors:**
1. Use `get_hcp_terraform_apply` for detailed error information
2. Check infrastructure provider quotas and permissions
3. Verify resource dependencies and timing
4. Consider partial apply with targeted operations

**Stuck Runs:**
1. Monitor with `list_hcp_terraform_runs` for status updates
2. Use `cancel_hcp_terraform_run` for graceful termination
3. Use `force_cancel` only as last resort for unresponsive runs
4. Check workspace locking status if runs won't start

## Security Best Practices

1. **Sensitive Variables:** Always mark credentials and secrets as sensitive
2. **Access Control:** Use appropriate workspace permissions and team access
3. **State Security:** Ensure state files don't contain exposed secrets
4. **Tag Compliance:** Use consistent tagging for security and compliance tracking

## Available Tools Reference

All the tools used in this guide are already implemented in the MCP server:

**Workspace Management:**
- `get_hcp_terraform_workspaces` - List workspaces with filtering
- `get_hcp_terraform_workspace_details` - Get complete workspace information
- `create_hcp_terraform_workspace` - Create new workspace
- `update_hcp_terraform_workspace` - Update workspace settings

**Variable Management:**
- `get_hcp_terraform_workspace_variables` - List workspace variables
- `create_hcp_terraform_workspace_variable` - Create single variable
- `bulk_create_hcp_terraform_workspace_variables` - Create multiple variables
- `update_hcp_terraform_workspace_variable` - Update existing variable
- `delete_hcp_terraform_workspace_variable` - Remove variable

**Configuration Management:**
- `get_hcp_terraform_configuration_versions` - List configuration versions
- `create_hcp_terraform_configuration_version` - Create new config version
- `download_hcp_terraform_configuration_files` - Download existing configs
- `upload_hcp_terraform_configuration_files` - Upload configuration files

**State Management:**
- `get_hcp_terraform_current_state_version` - Get current state version
- `download_hcp_terraform_state_version` - Download state data
- `create_hcp_terraform_state_version` - Upload new state version

**Run Management (Complete Manual Control):**
- `create_hcp_terraform_run` - Create new runs (plan/apply/destroy/plan-only)
- `get_hcp_terraform_run` - Get detailed run information with relationships
- `list_hcp_terraform_runs` - List workspace runs with filtering and pagination
- `apply_hcp_terraform_run` - Apply a planned run (manual approval)
- `discard_hcp_terraform_run` - Discard a planned run without applying
- `cancel_hcp_terraform_run` - Cancel running operations (with force option)

**Plan & Apply Details:**
- `get_hcp_terraform_plan` - Get detailed plan information and resource changes
- `get_hcp_terraform_apply` - Get detailed apply information and results

**Tag Management:**
- `get_hcp_terraform_workspace_tags` - List workspace tags
- `create_hcp_terraform_workspace_tag_bindings` - Add tags to workspace
- `update_hcp_terraform_workspace_tag_bindings` - Update existing tags
- `delete_hcp_terraform_workspace_tags` - Remove tags

**Workspace Locking:**
- `lock_hcp_terraform_workspace` - Lock workspace for safe operations
- `unlock_hcp_terraform_workspace` - Unlock workspace

**Remote State:**
- `get_hcp_terraform_remote_state_consumers` - List state consumers
- `add_hcp_terraform_remote_state_consumers` - Grant state access
- `remove_hcp_terraform_remote_state_consumers` - Revoke state access

**Organization Management:**
- `get_hcp_terraform_organizations` - List accessible organizations

This comprehensive toolkit provides complete coverage for HCP Terraform workspace lifecycle management and **full manual run control**, making it possible to implement complex workflows like environment replication, automated infrastructure provisioning, and compliance-driven deployment pipelines with complete oversight and control.

## Next Steps

After environment setup:
- Configure workspace notifications
- Set up VCS integration (if not done during creation)
- Configure run triggers and automation
- Set up team access and permissions
- Monitor infrastructure health and costs
- Plan future infrastructure scaling and updates

This guide ensures a complete, secure, and well-organized HCP Terraform environment setup that follows HashiCorp best practices and includes full infrastructure deployment capabilities.
