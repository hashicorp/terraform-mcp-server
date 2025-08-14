# Workspace Orchestrator Package

Provides comprehensive workspace analysis functionality for the Terraform MCP server.

## Components

- **`types.go`**: Data structures for workspace analysis
- **`workspace_analyzer.go`**: Main orchestrator service  
- **`analysis_steps.go`**: Individual analysis step implementations
- **`api_client.go`**: Token resolution and utility functions

## Usage

```go
analyzer := orchestrator.NewWorkspaceAnalyzer(hcpClient, logger)
request := &orchestrator.WorkspaceAnalysisRequest{
    WorkspaceID: "ws-12345",
    Authorization: "token",
}
response, err := analyzer.AnalyzeWorkspace(request)
```

## Analysis Workflow

1. **Workspace Details** (Critical) - Core workspace information
2. **Variables** (Non-critical) - Terraform and environment variables  
3. **Configurations** (Non-critical) - Configuration versions
4. **State** (Non-critical) - Current state version
5. **Tags** (Non-critical) - Workspace tags
6. **Remote Consumers** (Conditional) - Remote state consumers

## Error Handling

- Critical steps: Failure stops execution
- Non-critical steps: Logged but analysis continues  
- Partial results: Always provide maximum available information

## Implementation Status

Currently uses placeholder implementations. TODO items:
- Implement actual HCP Terraform API calls
- Add response data parsing
- Integrate with existing tool handlers
