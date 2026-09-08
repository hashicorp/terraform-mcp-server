# End-to-End Tests

The E2E tests build and run the `terraform-mcp-server` Docker image, then test
the server through both supported MCP transports:

- Stdio
- Streamable HTTP

The tests use the official MCP Go SDK and call the real registry-backed tools.

## Test Organization

The E2E tests are organized by tool:

- `search_providers_test.go`
- `provider_details_test.go`
- `search_modules_test.go`
- `module_details_test.go`
- `search_policies_test.go`
- `policy_details_test.go`
- `latest_module_version_test.go`
- `latest_provider_version_test.go`
- `cors_e2e_test.go`

Each tool test runs against both Stdio and HTTP using a fresh MCP session.

## Running the Tests

Docker must be running because the tests build and start the server image.

Run the complete E2E suite:

```bash
make test-e2e
```

Run all E2E tests directly:

```bash
go test ./e2e -v -count=1
```

Run the provider search tests:

```bash
go test ./e2e -run 'TestSearchProviders' -v -count=1
```

Run the CORS tests:

```bash
go test ./e2e -run '^TestCORSE2E$' -v -count=1
```

## Test Lifecycle

`TestMain` performs package-level setup and cleanup:

1. Builds `terraform-mcp-server:test-e2e` once.
2. Runs all E2E tests.
3. Stops any remaining test containers.

Each transport test creates and closes its own MCP session. HTTP tests also
create and stop their own Docker container.

## CORS Testing

CORS tests use direct HTTP requests instead of the MCP SDK so they can verify:

- `Origin` request handling
- `Access-Control-Allow-Origin`
- `Access-Control-Allow-Methods`
- HTTP status codes
- `OPTIONS` preflight behavior

## Requirements

- Docker
- Go
- Network access to the Terraform Registry
