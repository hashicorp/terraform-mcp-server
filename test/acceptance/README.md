# Acceptance Test Suite

The acceptance test suite exercises the high level functionality of the MCP server against the real registry and TFE/TFC APIs.  

Tests added to this suite should make meaningful assertions to ensure that the behaviour of the server actually makes sense. 

## Run the acceptance test suite

To run these tests an active token for accessing the TFE/TFC API is required. 

```bash
TFE_TOKEN=<token> make test-acc 
```
