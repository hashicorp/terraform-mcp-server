>Note: In order to implement these custom instructions into the MCP server copy them into `cmd/terraform-mcp-server/instructions.md` and rebuild the MCP binary or docker image to use it.

# Terraform MCP Server Usage Instructions

## Overview
The Terraform MCP (Model Context Protocol) server is a specialized tool that enables LLMs to generate high-quality Terraform code and manage Terraform workflows through API integrations with HCP Terraform and Terraform Enterprise.

## Core Capabilities

### 1. Code Generation Enhancement

- **Registry Integration**: Connects to both public and private Terraform registries for module and provider information

- **Style Guide Compliance**: Provides access to Terraform styling guide resources for consistent HCL/TF file generation

- **Module Development**: Supports creation of reusable Terraform modules following best practices

### 2. Workflow Automation

- **API Operations**: Executes HCP Terraform and Terraform Enterprise commands via API calls

- **Iterative Development**: Enables automated testing, refinement, and enhancement of Terraform configurations

- **State Management**: Facilitates proper state handling and workspace management

## Operational Guidelines

### Pre-Generation Phase

**ALWAYS** consult the MCP server before generating any Terraform code to:

1. Retrieve latest provider documentation and constraints
2. Access organization-specific styling guidelines
3. Identify available modules and their requirements
4. Understand enterprise-specific policies and compliance requirements

### Registry Search Priority

When enterprise tools are enabled AND a Terraform token is provided:

1. **First**: Search private registries for modules and providers
2. **Second**: If no results found in private registries, fall back to public registry
3. **Document**: Note the source registry in comments for transparency

### Provider Consistency Rules

**CRITICAL**: Maintain provider version consistency across all modules in a project:

- Verify provider requirements before module creation
- Ensure all modules declare compatible provider version constraints
- Flag any provider version conflicts before code generation
- Use explicit version pinning when required by organization policies

### Validation Workflow

Execute validation in this specific order:

1. **terraform validate**: Run immediately after code generation
   - Verify syntax correctness
   - Check resource attribute validity
   - Ensure provider configuration completeness
2. **terraform plan**: Only execute after successful validation
   - Review resource changes
   - Identify potential issues before apply
   - Capture plan output for review

### User Confirmation Requirements
**MANDATORY**: Request explicit user confirmation before executing any of these destructive operations:
- `create_run`: Initiates a new Terraform run
- `apply_run`: Applies changes to infrastructure
- `discard_run`: Discards a planned run
- `cancel_run`: Cancels an in-progress run

**Confirmation prompt should include**:
- Clear description of the operation
- List of resources to be affected
- Potential risks or impacts
- Request for explicit "yes/no" confirmation

## Error Handling

### Common Scenarios

1. **Registry Access Failure**: 
   - Log the error
   - Attempt fallback to alternative registry
   - Inform user of limitations

2. **Validation Errors**:
   - Parse error messages
   - Provide specific remediation steps
   - Re-validate after corrections

3. **Plan Failures**:
   - Analyze plan output for root causes
   - Suggest configuration adjustments
   - Document assumptions that may need verification

## Best Practices for LLM Implementation

### 1. Context Preservation
- Maintain state of previous MCP server queries within the session
- Track which registries have been searched
- Remember user preferences and requirements stated earlier

### 2. Progressive Enhancement
- Start with minimal viable configuration
- Iteratively add complexity based on validation results
- Use MCP server feedback to refine generated code

### 3. Documentation Generation
- Include inline comments explaining non-obvious configurations
- Document registry sources for modules
- Add README sections for complex modules

### 4. Security Considerations
- Never expose Terraform tokens in outputs
- Sanitize sensitive data in error messages
- Follow principle of least privilege for API operations

## Troubleshooting Checklist
- [ ] MCP server connection verified
- [ ] Appropriate registry searched based on token availability
- [ ] Style guide resources retrieved and applied
- [ ] Provider consistency validated across modules
- [ ] terraform validate executed successfully
- [ ] terraform plan reviewed (if applicable)
- [ ] User confirmation obtained for destructive operations
