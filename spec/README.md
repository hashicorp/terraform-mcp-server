# Terraform MCP Server Behavioral Specification

## Status

This directory specifies the externally observable behavior of the server in
this repository. It is an implementation-derived specification, not a product
roadmap. It records behavior that clients, operators, and compatibility tests
can observe, including behavior that appears accidental or differs from the
user-facing README.

The implementation is authoritative when this specification and the code
disagree. A behavior change in the implementation should update this
specification in the same change.

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** describe the
current compatibility contract. They do not imply that every behavior is
desirable.

## Documents

| Document | Scope |
| --- | --- |
| [runtime.md](runtime.md) | Process startup, MCP capabilities, transports, sessions, authentication, security, filtering, errors, resources, and observability |
| [registry-tools.md](registry-tools.md) | Public and private Terraform Registry tool contracts |
| [terraform-tools.md](terraform-tools.md) | HCP Terraform and Terraform Enterprise tool contracts |

## Capability Summary

The primary server identifies itself as `terraform-mcp-server` and can expose:

| Capability | Count | Availability |
| --- | ---: | --- |
| Public Registry tools | 9 | Registered at startup when selected |
| Private Registry tools | 4 | Registered after the first session constructs a TFE client when selected |
| HCP Terraform/TFE tools | 48 | Registered after the first session constructs a TFE client when selected |
| Concrete resources | 2 | Always registered |
| Resource templates | 1 | Always registered |
| Prompts | 0 | Not supported |

With the CLI default `--toolsets=all`, the primary server advertises 9 tools
before a Terraform client is constructed, 56 tools after client construction
when `ENABLE_TF_OPERATIONS` is not `true`, and 61 tools after client
construction when it is `true`. Client construction requires a non-empty token
but does not prove that the token is authorized; an invalid token can therefore
trigger advertisement while later API calls fail.

An optional experimental server at `<mcp-endpoint>/official` identifies itself
as `terraform-mcp-official` and exposes only its own `list_workspaces` tool.

## Specification Conventions

Tool parameter tables use these terms:

| Term | Meaning |
| --- | --- |
| Required | The advertised JSON Schema marks the property as required |
| Optional | The property may be omitted from the advertised input |
| Schema default | A default advertised in JSON Schema |
| Runtime default | The value the handler uses when the property is absent or empty |
| Tool error | A successful JSON-RPC tool response with `isError: true` and text content |
| Protocol error | A transport or JSON-RPC handler error with no tool result |
| JSON text | JSON serialized into MCP text content, not MCP structured content |

Unless a tool contract says otherwise:

- Primary-server successes contain exactly one MCP text content item.
- Primary-server validation and upstream API failures are tool errors.
- HCP Terraform/TFE tools require an active session with a valid Terraform
  client, even after the tool has become globally advertised.
- Timestamps and dependency-owned JSON:API fields retain the formats emitted
  by the pinned `go-tfe` and JSON:API libraries.
- Backend authorization, licensing, and feature availability can add errors
  beyond those enumerated here.

## Compatibility Notes

The specification intentionally preserves these broad implementation facts:

- Tool advertisement is process-global after the first session constructs a
  TFE client, but usable backend authorization remains session-specific.
- Primary input schemas are advertised but not server-validated. Handler
  accessors and explicit checks, rather than JSON Schema, define accepted input.
- `ENABLE_TF_OPERATIONS` gates only five named tools and the destructive
  `create_run` variant; it is not a general mutation lock.
- Tool annotations are metadata. The MCP library fills omitted primary-tool
  hints with defaults, and several mutating tools advertise
  `destructiveHint: false`.
- Most JSON-shaped results are text, not `structuredContent`.
- JSON Schema constraints and handler validation do not always match. Known
  behaviorally significant mismatches are stated in the relevant contracts.
- Empty-list behavior is tool-specific and is not normalized across the API.
