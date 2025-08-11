# HCP Terraform Organizations MCP Implementation Plan

## Overview
This document outlines the plan to implement MCP (Model Context Protocol) support for fetching organizations from HCP Terraform. The implementation will add a new HCP Terraform client alongside the existing Terraform Registry client, with secure token-based authentication.

## API Reference
- **Base URL**: `https://app.terraform.io/api/v2`
- **Endpoint**: `GET /organizations`
- **Documentation**: https://developer.hashicorp.com/terraform/cloud-docs/api-docs/organizations#list-organizations
- **Authentication**: Bearer token in `Authorization` header

## Authentication Strategy
The implementation will support two authentication methods with the following precedence:
1. **Environment Variable**: `HCP_TERRAFORM_TOKEN` 
2. **HTTP Header**: `Authorization: Bearer <token>` in the tool request
3. **Fallback**: If neither is present, decline the tool call with an appropriate error message

## Implementation Components

### 1. HCP Terraform Client Package (`pkg/client/hcp_terraform/`)
Create a dedicated package for HCP Terraform API interactions following Go best practices:

#### Structure:
```
/pkg/client/hcp_terraform/
├── client.go       # Main client implementation
├── types.go        # All API response/request types
├── errors.go       # Custom error types and handling
└── client_test.go  # Client tests
```

#### Main Client (`pkg/client/hcp_terraform/client.go`):
```go
package hcp_terraform

// Client handles authentication and API calls to HCP Terraform
type Client struct {
    httpClient *http.Client
    logger     *log.Logger
    baseURL    string
}

// NewClient creates a new HCP Terraform client
func NewClient(logger *log.Logger) *Client

// GetOrganizations fetches organizations from HCP Terraform
func (c *Client) GetOrganizations(token string, opts *OrganizationListOptions) (*OrganizationResponse, error)

// ValidateToken checks if the provided token is valid
func (c *Client) ValidateToken(token string) error
```

**Features:**
- HTTP client with retry logic (similar to existing registry client)
- Rate limiting handling (x-ratelimit-reset header support)
- Proper error handling and logging
- Configurable timeouts

### 2. HCP Terraform Types (`pkg/client/hcp_terraform/types.go`)
Define Go structs matching the HCP Terraform API response format within the dedicated package:

```go
package hcp_terraform

import "time"

// OrganizationResponse represents the API response structure
type OrganizationResponse struct {
    Data  []Organization     `json:"data"`
    Links PaginationLinks    `json:"links"`
    Meta  PaginationMetadata `json:"meta"`
}

// Organization represents a single HCP Terraform organization
type Organization struct {
    ID            string                    `json:"id"`
    Type          string                    `json:"type"`
    Attributes    OrganizationAttributes    `json:"attributes"`
    Relationships OrganizationRelationships `json:"relationships,omitempty"`
    Links         OrganizationLinks         `json:"links,omitempty"`
}

// OrganizationAttributes contains organization details
type OrganizationAttributes struct {
    Name                       string                 `json:"name"`
    Email                      string                 `json:"email"`
    CreatedAt                  time.Time              `json:"created-at"`
    ExternalID                 string                 `json:"external-id"`
    CollaboratorAuthPolicy     string                 `json:"collaborator-auth-policy"`
    SessionTimeout             *int                   `json:"session-timeout"`
    SessionRemember            *int                   `json:"session-remember"`
    PlanExpired                bool                   `json:"plan-expired"`
    PlanExpiresAt              *time.Time             `json:"plan-expires-at"`
    PlanIsTrial                bool                   `json:"plan-is-trial"`
    PlanIsEnterprise           bool                   `json:"plan-is-enterprise"`
    PlanIdentifier             string                 `json:"plan-identifier"`
    CostEstimationEnabled      bool                   `json:"cost-estimation-enabled"`
    Permissions                OrganizationPermissions `json:"permissions"`
    TwoFactorConformant        bool                   `json:"two-factor-conformant"`
    AssessmentsEnforced        bool                   `json:"assessments-enforced"`
    DefaultExecutionMode       string                 `json:"default-execution-mode"`
    // Add other attributes as needed
}

// OrganizationPermissions contains user permissions for the organization
type OrganizationPermissions struct {
    CanUpdate                bool `json:"can-update"`
    CanDestroy               bool `json:"can-destroy"`
    CanAccessViaTeams        bool `json:"can-access-via-teams"`
    CanCreateModule          bool `json:"can-create-module"`
    CanCreateTeam            bool `json:"can-create-team"`
    CanCreateWorkspace       bool `json:"can-create-workspace"`
    CanManageUsers           bool `json:"can-manage-users"`
    CanManageSubscription    bool `json:"can-manage-subscription"`
    CanManageSSO             bool `json:"can-manage-sso"`
    CanUpdateOAuth           bool `json:"can-update-oauth"`
    CanUpdateSentinel        bool `json:"can-update-sentinel"`
    CanUpdateSSHKeys         bool `json:"can-update-ssh-keys"`
    CanUpdateAPIToken        bool `json:"can-update-api-token"`
    CanTraverse              bool `json:"can-traverse"`
    CanStartTrial            bool `json:"can-start-trial"`
    CanUpdateAgentPools      bool `json:"can-update-agent-pools"`
    CanManageTags            bool `json:"can-manage-tags"`
    CanManageVarsets         bool `json:"can-manage-varsets"`
    CanReadVarsets           bool `json:"can-read-varsets"`
    CanManagePublicProviders bool `json:"can-manage-public-providers"`
    CanCreateProvider        bool `json:"can-create-provider"`
    CanManagePublicModules   bool `json:"can-manage-public-modules"`
    CanCreateProject         bool `json:"can-create-project"`
}

// OrganizationRelationships contains related resources
type OrganizationRelationships struct {
    DefaultAgentPool      *RelationshipData `json:"default-agent-pool,omitempty"`
    OAuthTokens          *RelationshipData `json:"oauth-tokens,omitempty"`
    AuthenticationToken  *RelationshipData `json:"authentication-token,omitempty"`
    EntitlementSet       *RelationshipData `json:"entitlement-set,omitempty"`
    Subscription         *RelationshipData `json:"subscription,omitempty"`
}

// RelationshipData represents a relationship to another resource
type RelationshipData struct {
    Data  interface{}       `json:"data,omitempty"`
    Links map[string]string `json:"links,omitempty"`
}

// OrganizationLinks contains API links for the organization
type OrganizationLinks struct {
    Self string `json:"self"`
}

// PaginationLinks contains pagination URLs
type PaginationLinks struct {
    Self  string  `json:"self"`
    First string  `json:"first"`
    Prev  *string `json:"prev"`
    Next  *string `json:"next"`
    Last  string  `json:"last"`
}

// PaginationMetadata contains pagination information
type PaginationMetadata struct {
    Pagination PaginationInfo `json:"pagination"`
}

// PaginationInfo contains detailed pagination data
type PaginationInfo struct {
    CurrentPage int  `json:"current-page"`
    PageSize    int  `json:"page-size"`
    PrevPage    *int `json:"prev-page"`
    NextPage    *int `json:"next-page"`
    TotalPages  int  `json:"total-pages"`
    TotalCount  int  `json:"total-count"`
}

// OrganizationListOptions for query parameters
type OrganizationListOptions struct {
    Query      string `url:"q,omitempty"`
    QueryEmail string `url:"q[email],omitempty"`
    QueryName  string `url:"q[name],omitempty"`
    PageNumber int    `url:"page[number],omitempty"`
    PageSize   int    `url:"page[size],omitempty"`
}
```

#### Custom Errors (`pkg/client/hcp_terraform/errors.go`):
```go
package hcp_terraform

import (
    "fmt"
    "net/http"
)

// Error types for better error handling
type ErrorType string

const (
    ErrorTypeAuthentication ErrorType = "authentication"
    ErrorTypeAuthorization  ErrorType = "authorization" 
    ErrorTypeRateLimit      ErrorType = "rate_limit"
    ErrorTypeNotFound       ErrorType = "not_found"
    ErrorTypeValidation     ErrorType = "validation"
    ErrorTypeNetwork        ErrorType = "network"
    ErrorTypeUnknown        ErrorType = "unknown"
)

// HCPTerraformError represents errors from HCP Terraform API
type HCPTerraformError struct {
    Type       ErrorType
    Message    string
    StatusCode int
    RetryAfter *int // For rate limiting
    Err        error
}

func (e *HCPTerraformError) Error() string {
    if e.StatusCode > 0 {
        return fmt.Sprintf("HCP Terraform API error (%d): %s", e.StatusCode, e.Message)
    }
    return fmt.Sprintf("HCP Terraform error: %s", e.Message)
}

func (e *HCPTerraformError) Unwrap() error {
    return e.Err
}

// NewErrorFromResponse creates an appropriate error from HTTP response
func NewErrorFromResponse(resp *http.Response, err error) *HCPTerraformError {
    hcpErr := &HCPTerraformError{
        StatusCode: resp.StatusCode,
        Err:        err,
    }
    
    switch resp.StatusCode {
    case http.StatusUnauthorized:
        hcpErr.Type = ErrorTypeAuthentication
        hcpErr.Message = "Invalid or missing authentication token"
    case http.StatusForbidden:
        hcpErr.Type = ErrorTypeAuthorization
        hcpErr.Message = "Insufficient permissions"
    case http.StatusNotFound:
        hcpErr.Type = ErrorTypeNotFound
        hcpErr.Message = "Resource not found"
    case http.StatusTooManyRequests:
        hcpErr.Type = ErrorTypeRateLimit
        hcpErr.Message = "Rate limit exceeded"
        // Extract retry-after header if present
    default:
        hcpErr.Type = ErrorTypeUnknown
        hcpErr.Message = "Unknown API error"
    }
    
    return hcpErr
}
```

### 3. HCP Terraform Tools Package (`pkg/tools/hcp_terraform/`)
Create a dedicated package for all HCP Terraform-related MCP tools:

#### Structure:
```
/pkg/tools/hcp_terraform/
├── organizations.go     # Organizations tool implementation
├── auth.go             # Token resolution utilities
├── tools_test.go       # Tools tests
└── common.go           # Shared utilities for HCP tools
```

#### Organizations Tool (`pkg/tools/hcp_terraform/organizations.go`):
```go
package hcp_terraform

import (
    "context"
    "fmt"
    "os"
    "strings"
    
    "github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
    "github.com/hashicorp/terraform-mcp-server/pkg/utils"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
    log "github.com/sirupsen/logrus"
)

// GetOrganizations creates the MCP tool for listing organizations
func GetOrganizations(hcpClient *hcp_terraform.Client, logger *log.Logger) server.ServerTool {
    return server.ServerTool{
        Tool: mcp.Tool{
            Name: "get_hcp_terraform_organizations",
            Description: "Fetches all organizations from HCP Terraform that the authenticated user has access to",
            InputSchema: mcp.ToolInputSchema{
                Type: "object",
                Properties: map[string]interface{}{
                    "query": {
                        "type":        "string",
                        "description": "Optional search query to filter organizations by name or email",
                    },
                    "page_size": {
                        "type":        "integer",
                        "description": "Number of organizations per page (default: 20)",
                        "minimum":     1,
                        "maximum":     100,
                        "default":     20,
                    },
                    "authorization": {
                        "type":        "string",
                        "description": "Optional Bearer token for authentication (if not provided via environment)",
                    },
                },
            },
        },
        Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
            return getOrganizationsHandler(hcpClient, request, logger)
        },
    }
}

func getOrganizationsHandler(hcpClient *hcp_terraform.Client, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
    // Resolve authentication token
    token, err := resolveToken(request)
    if err != nil {
        return nil, utils.LogAndReturnError(logger, "token resolution", err)
    }
    
    // Parse request parameters
    opts := &hcp_terraform.OrganizationListOptions{
        PageSize: request.GetInt("page_size", 20),
    }
    
    if query := request.GetString("query", ""); query != "" {
        opts.Query = query
    }
    
    // Fetch organizations
    response, err := hcpClient.GetOrganizations(token, opts)
    if err != nil {
        return nil, utils.LogAndReturnError(logger, "fetching HCP Terraform organizations", err)
    }
    
    // Format response
    result := formatOrganizationsResponse(response)
    return mcp.NewToolResultText(result), nil
}
```

#### Authentication Utilities (`pkg/tools/hcp_terraform/auth.go`):
```go
package hcp_terraform

import (
    "fmt"
    "os"
    "strings"
    
    "github.com/mark3labs/mcp-go/mcp"
)

// resolveToken implements secure token resolution with proper precedence
func resolveToken(request mcp.CallToolRequest) (string, error) {
    // 1. Check environment variable first (highest precedence)
    if token := os.Getenv("HCP_TERRAFORM_TOKEN"); token != "" {
        return token, nil
    }
    
    // 2. Check tool request parameters
    if authParam, ok := request.Params.Arguments["authorization"].(string); ok && authParam != "" {
        // Handle both "Bearer <token>" and raw token formats
        if strings.HasPrefix(authParam, "Bearer ") {
            return strings.TrimPrefix(authParam, "Bearer "), nil
        }
        return authParam, nil
    }
    
    // 3. No token found
    return "", fmt.Errorf("HCP Terraform token required: provide via HCP_TERRAFORM_TOKEN environment variable or authorization parameter")
}

// validateTokenFormat performs basic token format validation
func validateTokenFormat(token string) error {
    if len(token) == 0 {
        return fmt.Errorf("token cannot be empty")
    }
    
    // HCP Terraform tokens typically start with specific prefixes
    validPrefixes := []string{"org-", "team-", "user-"}
    hasValidPrefix := false
    
    for _, prefix := range validPrefixes {
        if strings.HasPrefix(token, prefix) {
            hasValidPrefix = true
            break
        }
    }
    
    if !hasValidPrefix {
        return fmt.Errorf("token format appears invalid: expected token to start with org-, team-, or user-")
    }
    
    return nil
}
```

### 4. Integration Points

#### Update `pkg/tools/tools.go`
Add the new HCP Terraform tools to the initialization:

```go
import (
    // ... existing imports ...
    hcp_tools "github.com/hashicorp/terraform-mcp-server/pkg/tools/hcp_terraform"
    "github.com/hashicorp/terraform-mcp-server/pkg/client/hcp_terraform"
)

func InitTools(hcServer *server.MCPServer, registryClient *http.Client, logger *log.Logger) {
    // ... existing Terraform Registry tools ...
    
    // HCP Terraform tools
    hcpClient := hcp_terraform.NewClient(logger)
    
    // Organizations tool
    getHCPOrganizationsTool := hcp_tools.GetOrganizations(hcpClient, logger)
    hcServer.AddTool(getHCPOrganizationsTool.Tool, getHCPOrganizationsTool.Handler)
    
    // Future HCP tools can be added here:
    // getHCPWorkspacesTool := hcp_tools.GetWorkspaces(hcpClient, logger)
    // hcServer.AddTool(getHCPWorkspacesTool.Tool, getHCPWorkspacesTool.Handler)
}
```

#### Update Security Headers (Optional Enhancement)
Modify `pkg/client/security.go` to allow HCP Terraform authorization headers:

```go
// Update Access-Control-Allow-Headers to include Authorization
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id, Authorization")
```

## Package Organization Benefits

### Why This Structure is Optimal:

#### 1. **Dedicated HCP Terraform Client Package** (`/pkg/client/hcp_terraform/`)
- ✅ **Single Responsibility**: Only handles HCP Terraform API interactions
- ✅ **Type Safety**: All HCP types are co-located and namespace-protected  
- ✅ **Future-Proof**: Easy to extend with workspaces, runs, state management
- ✅ **Import Clarity**: Clean imports like `import "pkg/client/hcp_terraform"`
- ✅ **Testing Isolation**: Package-specific tests and mocks

#### 2. **Dedicated HCP Tools Package** (`/pkg/tools/hcp_terraform/`)
- ✅ **Logical Grouping**: All HCP Terraform MCP tools in one place
- ✅ **Shared Utilities**: Common authentication and formatting logic
- ✅ **Scalability**: Easy to add new tools (workspaces, runs, variables)
- ✅ **Separation of Concerns**: Tools logic separate from client logic

#### 3. **Go Best Practices Followed**:
- ✅ **Package Naming**: Short, descriptive, all lowercase
- ✅ **Interface Segregation**: Client focused on API, tools focused on MCP
- ✅ **Error Handling**: Custom error types with proper wrapping
- ✅ **Dependency Injection**: Client injected into tools
- ✅ **Testability**: Each package can be tested independently

### Directory Structure Overview:
```
/pkg/
├── client/
│   ├── hcp_terraform/          # 🆕 HCP Terraform client package
│   │   ├── client.go           # Main API client
│   │   ├── types.go            # All API types
│   │   ├── errors.go           # Custom errors
│   │   └── client_test.go      # Client tests
│   ├── common.go               # Existing registry client
│   ├── registry.go             # Existing registry functions
│   ├── types.go                # Existing registry types
│   └── security.go             # Existing security
└── tools/
    ├── hcp_terraform/          # 🆕 HCP Terraform tools package
    │   ├── organizations.go    # Organizations MCP tool
    │   ├── auth.go             # Authentication utilities
    │   ├── common.go           # Shared tool utilities
    │   └── tools_test.go       # Tools tests
    ├── get_module_details.go   # Existing registry tools
    ├── search_modules.go       # Existing registry tools
    └── tools.go                # Updated initialization
```

### 5. Error Handling Strategy
Implement comprehensive error handling for various scenarios:

- **Authentication Errors**: 401/403 responses
- **Rate Limiting**: 429 responses with retry logic
- **Network Errors**: Connection timeouts, DNS failures
- **Invalid Token**: Clear error messages for token validation
- **Malformed Responses**: JSON parsing errors
- **Missing Permissions**: Organizations user doesn't have access to

### 6. Response Format
The tool will return organizations in a structured format:

```json
{
  "organizations": [
    {
      "id": "hashicorp",
      "name": "hashicorp",
      "email": "admin@hashicorp.com",
      "external_id": "org-ABC123",
      "created_at": "2021-08-24T23:10:04.675Z",
      "permissions": {
        "can_create_workspace": true,
        "can_manage_users": true,
        // ... other permissions
      },
      "plan": {
        "identifier": "team",
        "is_trial": false,
        "is_enterprise": false
      }
    }
  ],
  "pagination": {
    "current_page": 1,
    "total_pages": 2,
    "total_count": 25,
    "page_size": 20
  }
}
```

## Security Considerations

1. **Token Storage**: Environment variables preferred over request parameters
2. **Token Validation**: Validate token format and API accessibility before making requests
3. **Logging**: Never log sensitive tokens, only log success/failure and sanitized errors
4. **Rate Limiting**: Respect HCP Terraform API rate limits with proper backoff
5. **Error Messages**: Provide helpful error messages without exposing sensitive information

## Testing Strategy

### Unit Tests
- Token resolution logic with various input combinations
- API response parsing with different organization structures
- Error handling for various HTTP status codes
- Pagination logic testing

### Integration Tests
- End-to-end tool execution with valid tokens
- Authentication failure scenarios
- Rate limiting behavior
- CORS handling with Authorization headers

### E2E Tests
- Full MCP tool execution via HTTP and stdio transports
- Token validation across different authentication methods
- Error propagation to MCP clients

## Documentation Updates

1. **README.md**: Add HCP Terraform integration documentation
2. **Environment Variables**: Document `HCP_TERRAFORM_TOKEN` usage
3. **Tool Usage Examples**: Provide example MCP tool calls
4. **Authentication Guide**: Explain token acquisition and usage

## Future Enhancements

1. **Additional HCP Terraform APIs**: Workspaces, runs, state management
2. **Token Caching**: Cache validated tokens to reduce API calls
3. **Organization Filtering**: Client-side filtering by permissions/features
4. **Workspace Management**: CRUD operations for workspaces
5. **State File Access**: Read/write Terraform state files
6. **Run Management**: Trigger and monitor Terraform runs

## Implementation Timeline

1. **Phase 1**: Core client and types implementation
2. **Phase 2**: Tool implementation with basic functionality
3. **Phase 3**: Authentication and error handling
4. **Phase 4**: Testing and validation
5. **Phase 5**: Documentation and integration

This plan provides a comprehensive foundation for implementing HCP Terraform organization fetching while maintaining security best practices and following the existing codebase patterns.
