# End To End (e2e) Tests

The purpose of the E2E tests is to have a simple (currently) test that gives maintainers some confidence when adding new resources/tools. It does this by:
 * Building the `terraform-mcp-server` docker image
 * Running the image
 * Interacting with the server via stdio
 * Issuing requests that interact with the existing Resources/Tools

## Running the Tests

A service must be running that supports image building and container creation via the `docker` CLI.

```
make test-e2e
```

Running the tests:

```
make test-e2e
=== RUN   TestE2E
    e2e_test.go:92: Building Docker image for e2e tests...
    e2e_test.go:38: Starting Stdio MCP client...
=== RUN   TestE2E/Initialize
Initialized with server: terraform-mcp-server test-e2e

=== RUN   TestE2E/CallTool_list_providers
    e2e_test.go:83: Raw response content: aws, google, azurerm, kubernetes, github, docker, null, random
--- PASS: TestE2E (2.30s)
    --- PASS: TestE2E/Initialize (0.55s)
    --- PASS: TestE2E/CallTool_list_providers (0.00s)
PASS
ok      terraform-mcp-server/e2e    2.771s
```
