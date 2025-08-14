# Workspace Analysis Prompt

## Purpose
Provides comprehensive workspace analysis including details, variables, configurations, and state information.

## Workflow
When a user requests workspace details, execute these steps:

1. **Workspace Details** (Required) - Core workspace configuration
2. **Variables** (Optional) - Terraform and environment variables  
3. **Configurations** (Optional) - Latest configuration versions
4. **State** (Optional) - Current state version and health
5. **Tags** (Optional) - Workspace metadata tags
6. **Remote Consumers** (Conditional) - Workspaces consuming this state

## Response Format
- **Workspace Overview**: Name, execution mode, settings
- **Variables Summary**: Count by type, sensitive variables (values hidden)
- **Configuration Status**: Latest version and upload status
- **State Information**: Current version, serial number, freshness
- **Metadata**: Tags and remote state sharing status

## Error Handling
- Critical failure (workspace details): Stop and return error
- Non-critical failures: Continue with partial results and log warnings
- Always provide maximum available information
