# Runtime Behavior

## 1. Server Identity And Capabilities

The primary MCP server MUST advertise:

- Name `terraform-mcp-server`.
- The base version embedded from `version/VERSION`. MCP initialization omits
  prerelease and build metadata that `GetHumanVersion` adds for display.
- Tool support with list-change notifications enabled.
- Resource support with subscriptions and list-change notifications enabled.
- Elicitation support.
- The text embedded from `cmd/terraform-mcp-server/instructions.md` as server
  instructions.

The primary server MUST NOT advertise or register MCP prompts.

The server instructions are advisory model guidance. They do not add tools or
server-side authorization. Some names in the embedded instructions do not
correspond to registered tools; the tool catalogs in this specification are
authoritative for availability.

## 2. Process Invocation

### 2.1 Commands

| Invocation | Behavior |
| --- | --- |
| `terraform-mcp-server` | Start the primary server over stdio |
| `terraform-mcp-server stdio` | Start the primary server over stdio |
| `terraform-mcp-server streamable-http` | Start the primary Streamable HTTP server |
| `terraform-mcp-server http` | Start Streamable HTTP and emit Cobra's deprecation notice |
| `terraform-mcp-server --version` | Print `Terraform MCP Server` and `Version: <human-version>` |

The stdio transport MUST read JSON-RPC from stdin and write JSON-RPC to stdout.
It MUST write `Terraform MCP Server running on stdio` to stderr after starting.
The configured server logger uses stderr unless a log file is configured.
Package-global Logrus calls, including some rate-limit and experimental-server
logs, continue to use stderr even when `--log-file` is set.

### 2.2 CLI Flags

| Flag | Scope | Default | Behavior |
| --- | --- | --- | --- |
| `--log-file` | Persistent | empty | Append logs to the named file, creating it with mode `0666` subject to umask |
| `--log-level` | Persistent | `info` | One of Logrus `trace`, `debug`, `info`, `warn`, `warning`, `error`, `fatal`, or `panic`, case-insensitive but not whitespace-trimmed |
| `--log-format` | Persistent | `text` | `text` or `json`, case-insensitive |
| `--toolsets` | Persistent | `all` | Comma-separated toolset selection |
| `--tools` | Persistent | empty | Comma-separated individual tool selection |
| `--transport-host` | HTTP commands | `127.0.0.1` | Listener host |
| `--transport-port`, `-p` | HTTP commands | `8080` | Listener port as a string |
| `--heartbeat-interval` | HTTP commands | `0` | Go duration; zero disables heartbeat |
| `--mcp-endpoint` | HTTP commands | `/mcp` | MCP path, normalized to begin at `/` |
| `--organization-allowlist` | HTTP commands | empty | Comma-separated allowed organization names |

`LOG_LEVEL` and `LOG_FORMAT` override their CLI flags. Invalid log values MUST
produce a warning and fall back to `info` or `text`, respectively.

`--tools` and an explicitly supplied `--toolsets` MUST be treated as mutually
exclusive and terminate startup. The non-empty default value of `--toolsets`
does not conflict with `--tools` unless the user explicitly set the flag.
An explicitly empty `--tools=` does not enter individual-tool mode and does not
conflict with `--toolsets`; ordinary toolset selection applies.

### 2.3 Environment-Selected HTTP Mode

The process MUST enter Streamable HTTP mode before Cobra command parsing when
either condition is true:

- `TRANSPORT_MODE` is exactly `http` or `streamable-http`.
- Any of `TRANSPORT_PORT`, `TRANSPORT_HOST`, or `MCP_ENDPOINT` is non-empty.

This comparison is case-sensitive. In this path, command-line arguments are
not parsed. Persistent CLI values therefore remain their compiled defaults,
while environment variables read directly by the process still apply.

The environment-selected defaults are host `127.0.0.1`, port `8080`, endpoint
`/mcp`, no heartbeat, and toolsets `all`.

## 3. Streamable HTTP Surface

### 3.1 Listener And Routes

The MCP endpoint MUST be normalized with path joining and registered both with
and without one trailing slash. The default routes are:

| Route | Behavior |
| --- | --- |
| `/mcp` and `/mcp/` | Primary MCP Streamable HTTP handler |
| `/health` | Unauthenticated health response |
| `/mcp/official` and `/mcp/official/` | Experimental handler when enabled |

The endpoint is not checked against reserved routes. Configuring it as
`/health` causes a conflicting ServeMux registration panic. Configuring it as
`/` also conflicts when the root redirect is enabled.

If `MCP_REDIRECT_ROOT_URL` is non-empty, the root ServeMux pattern `/` MUST
return HTTP 303 to that URL. Because `/` is a catch-all pattern, otherwise
unmatched paths also reach this redirect. Without it, unmatched paths return
the Go HTTP mux 404 response unless the configured MCP endpoint itself is `/`,
in which case the primary MCP handler is the catch-all.

The health handler does not restrict the HTTP method and MUST return HTTP 200,
`Content-Type: application/json`, and this shape:

```json
{
  "status": "ok",
  "service": "terraform-mcp-server",
  "transport": "streamable-http",
  "endpoint": "/mcp",
  "version": "<human-version>"
}
```

The health and redirect handlers are outside the MCP authentication, Terraform
context, and CORS wrappers.

The HTTP server MUST use 30-second read, read-header, and write timeouts, a
60-second idle timeout, and a five-second graceful-shutdown timeout.

### 3.2 Sessions And Heartbeats

The primary HTTP server is stateful by default. `MCP_SESSION_MODE` enables
stateless mode only when its case-insensitive value is exactly `stateless`.
All other values select stateful mode.

In environment-selected HTTP mode, `MCP_HEARTBEAT_INTERVAL` is parsed as a Go
duration. A missing, invalid, zero, or negative value disables heartbeat, and a
positive value configures MCP HTTP heartbeats. An explicit `streamable-http` or
`http` command reads only `--heartbeat-interval`; this environment variable
does not override that flag and does not itself select HTTP mode. The similarly
named `MCP_KEEP_ALIVE` documented by older material has no effect.

### 3.3 CORS And Origin Validation

`MCP_CORS_MODE` defaults to `strict`. Mode comparisons are case-sensitive and
the only modes with special behavior are `strict`, `development`, and
`disabled`.

`MCP_ALLOWED_ORIGINS` is a comma-separated exact-match list. Entries are
trimmed. Empty entries are retained if present in a non-empty list but do not
match a non-empty origin.

Origin handling MUST follow this table:

| Condition | Result |
| --- | --- |
| No `Origin` header | Bypass origin validation in every mode |
| Exact configured origin | Allow |
| `development` and a value beginning `http://` or `https://` plus `localhost:`, `127.0.0.1:`, or `[::1]:` | Allow |
| `disabled` | Allow every origin |
| Any other origin | HTTP 403 `Origin not allowed` |

For an allowed non-empty origin, the response MUST echo that origin and set:

```text
Access-Control-Allow-Origin: <origin>
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type, Mcp-Session-Id, Authorization
Access-Control-Max-Age: 3600
```

An allowed `OPTIONS` request MUST return HTTP 200 without invoking the MCP
handler. Browser preflight does not advertise `TFE_TOKEN` or
`TFE_SKIP_TLS_VERIFY` as allowed headers.

Development matching is raw string-prefix matching, not URL or port
validation. A malformed value, path-bearing value, or non-numeric port is still
allowed if it has one of the accepted prefixes.

### 3.4 MCP Host And Wire Validation

By default, both MCP implementations perform built-in DNS-rebinding protection after the
application's outer HTTP wrappers. When a request arrives through a loopback
local address but its `Host` is not `localhost` or a loopback IP, the MCP
handler returns HTTP 403 with `Forbidden: invalid Host header "<host>"`.
Requests arriving through non-loopback local addresses are not subject to this
Host check. Health and redirect routes bypass it.

The two MCP HTTP implementations have additional wire differences:

| Behavior | Primary endpoint | Official endpoint |
| --- | --- | --- |
| POST body limit | No explicit cap before `io.ReadAll` | 4 MiB by default; excess returns 413 |
| POST content type | Requires media type `application/json`; invalid returns 400 | Requires `application/json`; invalid returns 415 unless compatibility-disabled |
| POST `Accept` | No explicit requirement | Must include both `application/json` and `text/event-stream`; invalid returns 400 |
| Stateful GET | Can generate/register a session when the session header is absent | Requires `Accept: text/event-stream` and an existing `Mcp-Session-Id` |

The official SDK supports additional protocol-version checks and method rules
owned by that dependency. The primary handler accepts POST, GET, DELETE, and
HEAD at its endpoint; HEAD returns 200 and otherwise unsupported methods return
404.

### 3.5 TLS

`MCP_TLS_CERT_FILE` and `MCP_TLS_KEY_FILE` MUST either both be absent or both be
present. When present, both files MUST exist, be readable, and load as an X.509
certificate/key pair. Invalid configuration MUST fail startup.

TLS MUST use a minimum version of 1.2. The implementation configures X25519,
P-256, P-384, and X25519MLKEM768 curve preferences and ECDHE AES-GCM or
ChaCha20-Poly1305 TLS 1.2 cipher suites. Go controls TLS 1.3 suites.

Without TLS, startup MUST fail for hosts other than `localhost`, `127.0.0.1`,
`::1`, `[::1]`, and `0.0.0.0`, compared case-insensitively. In particular,
`0.0.0.0` is treated as local and may be served over plaintext.

## 4. Configuration Environment

| Variable | Default | Effective behavior |
| --- | --- | --- |
| `TRANSPORT_MODE` | stdio | Exact `http` or `streamable-http` selects HTTP |
| `TRANSPORT_HOST` | `127.0.0.1` | Non-empty value selects HTTP and sets listener host |
| `TRANSPORT_PORT` | `8080` | Non-empty value selects HTTP and sets listener port |
| `MCP_ENDPOINT` | `/mcp` | Non-empty value selects HTTP and sets endpoint |
| `MCP_REDIRECT_ROOT_URL` | empty | Enables the catch-all 303 redirect |
| `MCP_SESSION_MODE` | stateful | Case-insensitive exact `stateless` enables stateless mode |
| `MCP_HEARTBEAT_INTERVAL` | `0` | Positive Go duration enables heartbeat only in environment-selected HTTP mode |
| `MCP_ALLOWED_ORIGINS` | empty | Comma-separated exact CORS origins |
| `MCP_CORS_MODE` | `strict` | Origin policy mode |
| `MCP_TLS_CERT_FILE` | empty | Server certificate path |
| `MCP_TLS_KEY_FILE` | empty | Server private-key path |
| `MCP_RATE_LIMIT_GLOBAL` | `10:20` | Global `requests-per-second:burst` |
| `MCP_RATE_LIMIT_SESSION` | `5:10` | Per-session `requests-per-second:burst` |
| `MCP_ORGANIZATION_ALLOWLIST` | absent | Comma-separated organization gate; an explicitly empty value is invalid |
| `MCP_FORWARD_CLIENT_IP` | empty | Exact `true` enables client-IP forwarding |
| `MCP_REMOTE_IP_METHOD` | `RemoteAddr` | `RemoteAddr`, `X-Real-IP`, or `X-Forwarded-For` |
| `MCP_XFF_TRUSTED_HOPS` | `0` | Positive trusted-proxy count for XFF selection |
| `TFE_ADDRESS` | `https://app.terraform.io` | Server-side Terraform API address |
| `TFE_TOKEN` | empty | Terraform API token fallback |
| `TFE_SKIP_TLS_VERIFY` | false | Go boolean controlling outbound TLS verification |
| `TF_MCP_SHARED_SECRET` | empty | Outbound `X-Tf-Mcp-Secret` header to Terraform APIs |
| `ENABLE_TF_OPERATIONS` | false | Case-insensitive exact `true` enables selected operations |
| `LOG_LEVEL` | `info` | Overrides CLI logging level |
| `LOG_FORMAT` | `text` | Overrides CLI logging format |
| `OTEL_METRICS_ENABLED` | false | Exact `true` enables HTTP metrics setup |
| `OTEL_METRICS_ENDPOINT` | `localhost:4318` | OTLP/HTTP collector endpoint |
| `OTEL_METRICS_EXPORT_INTERVAL` | `2s` | Parseable Go duration; zero/negative values make the OTel reader use its own 60-second default |
| `OTEL_METRICS_SERVICE_NAME` | `terraform-mcp-server` | OTel service name |
| `OTEL_METRICS_SERVICE_VERSION` | human version | OTel service version |
| `INSTANA_ENABLED` | false | Exact `true` enables HTTP Instana instrumentation |
| `INSTANA_SERVICE_NAME` | `terraform-mcp-server` | Instana service name |
| `TF_X_OFFICIAL_SDK_ENABLED` | false | Exact `true` enables the experimental endpoint |
| `MCPGODEBUG` | empty | Official SDK compatibility parameters as comma-separated `key=value` pairs |

Outbound clients also honor Go's standard proxy environment and certificate
environment, including `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, and
`SSL_CERT_FILE` where supported by the Go runtime.

The official SDK parses `MCPGODEBUG` during package initialization even when
the official endpoint is disabled. Any comma-separated item without `=` causes
a startup panic. Relevant value-`1` switches include
`disablelocalhostprotection`, `enableoriginverification`,
`allowsessionsinstateless`, and `disablecontenttypecheck`; they respectively
alter only the official SDK's Host protection, built-in origin verification,
stateless session compatibility, and POST content-type validation.

## 5. Tool Selection And Advertisement

### 5.1 Toolsets

The valid toolsets are:

| Name | Contents |
| --- | --- |
| `registry` | Nine public Registry tools |
| `registry-private` | Four TFE-client-dependent private Registry tools |
| `terraform` | Forty-eight TFE-client-dependent Terraform tools |
| `all` | Every selected toolset/tool mapping |
| `default` | Expands to `registry` |

The CLI flag default is `all`; this differs from the meaning of the special
value `default`.

Toolset names MUST be trimmed, deduplicated, and compared case-sensitively.
Invalid names MUST produce a warning. They remain in the internal selection
but map to no tools. An empty explicit toolset selection enables no tools,
although resources remain available.

Individual tool names MUST be trimmed, deduplicated, and compared
case-sensitively against the complete 61-tool map. Invalid names produce a
warning and are ignored. If no valid individual names remain, selection MUST
fall back to the `default` toolset, but only when the non-empty `--tools` value
actually entered individual-tool mode.

### 5.2 Static And Dynamic Registration

Selected public Registry tools MUST be registered during startup.

Selected private Registry and Terraform tools MUST begin process-global
registration when the first session constructs a TFE client. Construction
requires a non-empty token but does not validate its authorization; an invalid
token can trigger registration. `go-tfe` synchronously requests `/api/v2/ping`
during construction: a transport failure prevents construction, while any
received HTTP status, including 401 or 500, permits construction to return.
Registration uses separate sequential
`AddTool` calls, is not atomic, and can emit multiple list-change notifications.
Another session can temporarily observe a partially registered catalog.

The application deliberately keeps the tools advertised after all TFE sessions
end. Every dynamic tool invocation after global registration MUST still check
the calling session:

- No session context returns a tool error requiring an active session.
- A session without a valid TFE client returns a tool error explaining that a
  valid token and configuration are required.
- A cached client found for an unregistered session causes that session to be
  registered before invocation continues.

Before global registration, a direct call to one of these names returns the MCP
protocol error `tool '<name>' not found`; it cannot reach the availability
wrapper.

The primary HTTP server attempts session initialization during registration,
before `tools/list`, and before `tools/call`. This permits sessions routed
between server instances to recreate local clients.

### 5.3 Terraform Operations Gate

`ENABLE_TF_OPERATIONS` is enabled when its case-insensitive value is exactly
`true`. It controls only this behavior:

| Behavior | Disabled | Enabled |
| --- | --- | --- |
| `delete_team` | Not registered | Registered if selected |
| `delete_project` | Not registered | Registered if selected |
| `delete_workspace_safely` | Not registered | Registered if selected |
| `force_unlock_workspace` | Not registered | Registered if selected |
| `action_run` | Not registered | Registered if selected |
| `create_run` | Safe schema/handler | Full schema/handler |

All other selected mutations remain available after client construction
regardless of this flag.

## 6. Session Clients And Credentials

### 6.1 Primary Server Session Lifecycle

Each initialized primary-server session MUST attempt to create:

- A Terraform API client if a token can be found.
- A public HTTP client regardless of Terraform authentication success.

Stateful clients are cached by MCP session ID. A separate equality-check field
stores a SHA-256 token hash, while the cached `go-tfe` client necessarily
retains its configured raw token. If a later request for the same session
carries a different token, the old cached client is deleted and a new one is
created. Session teardown removes both clients, the TFE-session marker, and the
per-session rate limiter.

Client reuse compares only the token hash. Changing only
`TFE_SKIP_TLS_VERIFY`, client IP, `TF_MCP_SHARED_SECRET`, or other client build
settings does not rebuild the primary TFE client. The public HTTP client is
reused solely by session ID and keeps its initial TLS setting. TFE client
settings remain sticky until a token change or teardown recreates that client;
the public client's TLS setting remains sticky until session teardown and is
not changed by a token change.

Stateless requests have an empty session ID and MUST create a new Terraform
client for each TFE tool request. Public HTTP clients may be stored under the
empty session ID by the current cache implementation.

### 6.2 Credential Resolution

For stdio and primary session initialization, Terraform configuration resolves
in this order:

| Value | Precedence |
| --- | --- |
| Address | Request context, then `TFE_ADDRESS`, then `https://app.terraform.io` |
| Token | Request context, then `TFE_TOKEN`, then Terraform CLI credentials file |
| Skip TLS verify | Request context, then `TFE_SKIP_TLS_VERIFY`, then false |

On Unix, the credentials file is
`~/.terraform.d/credentials.tfrc.json`. On Windows it is under
`%APPDATA%/terraform.d`, with the platform implementation's home-directory
fallback. The server selects `credentials[<TFE_ADDRESS hostname>].token`.
Missing files, malformed JSON, missing host entries, and empty hostnames prevent
creation of the TFE client but do not prevent public Registry tools from being
used.

`TFE_SKIP_TLS_VERIFY` is parsed with Go boolean syntax. Missing or invalid
values become false. The same setting configures both the Terraform API client
and the per-session public HTTP client.

### 6.3 HTTP Request Configuration

For primary Streamable HTTP requests, token precedence is:

1. Exact case-sensitive `Authorization: Bearer <token>`.
2. `TFE_TOKEN` request header.
3. `TFE_TOKEN` query parameter.
4. Server `TFE_TOKEN` environment variable.
5. Terraform CLI credentials file during stateful session creation.

A non-empty token query parameter MUST return HTTP 400 only when neither a
bearer token nor a `TFE_TOKEN` header was already selected. If a higher-priority
token exists, the query token is ignored rather than rejected.

Clients MUST NOT set `TFE_ADDRESS` by request header or query parameter. Either
non-empty form returns HTTP 403 before MCP handling, preventing token
redirection. An explicitly empty header or query value is treated as absent.
The address always comes from server configuration.

`TFE_SKIP_TLS_VERIFY` may be supplied by request header, query parameter, or
server environment in that order. Browser CORS preflight does not permit this
custom header. In a stateful session, a changed value does not alter already
cached clients.

### 6.4 Outbound Requests

Terraform API clients MUST enable retry of server errors and send:

- `User-Agent: terraform-mcp-server/<human-version>`.
- `X-Forwarded-For: <selected-client-ip>` when forwarding is enabled and an IP
  is available.
- `X-Tf-Mcp-Secret: <TF_MCP_SHARED_SECRET>` when configured.

The shared secret and selected client IP apply only to the primary TFE client,
not the experimental official-SDK client. They are captured when a stateful
client is built; a later request with a different selected IP or secret does not
update that client unless it is recreated.

## 7. Organization Allowlist

The allowlist comes from `MCP_ORGANIZATION_ALLOWLIST` when that environment
variable exists; otherwise the HTTP CLI flag is used if explicitly supplied.
Entries MUST be trimmed and lowercased. Empty entries are discarded. Duplicate
entries are harmless. A configured value yielding no names, including an
explicitly empty environment value, MUST fail startup.

When configured, an organization-membership wrapper is installed for both the
primary and official MCP endpoints. Request processing order is CORS/origin,
Terraform-context extraction, then organization membership. Consequently an
allowed `OPTIONS` preflight returns before authentication, a rejected origin
returns 403 first, an address override returns 403 first, and a token query can
return 400 before the allowlist's missing-bearer response.

Requests that reach the membership wrapper follow these rules:

- `Authorization` MUST contain the exact prefix `Bearer ` and a non-empty token.
- A `TFE_TOKEN` header or server token does not satisfy this gate.
- The supplied bearer token MUST be able to list at least one allowlisted
  organization, compared case-insensitively.
- Organization listing MUST page in groups of 100 until a match or end.
- Missing bearer authentication returns HTTP 401.
- An unauthorized Terraform token returns HTTP 401.
- A valid token with no allowed organization returns HTTP 403.
- Client initialization or membership-list failures return HTTP 502.
- A request-provided Terraform address still returns HTTP 403.

Every request reaching this gate creates a fresh uncached TFE client, including
the synchronous ping, and then calls `Organizations.List`. That validation
client receives the selected client IP and configured shared-secret headers,
including when the request is for the official endpoint.

After HTTP validation, primary tool middleware MUST reject a call when its
arguments contain a non-empty `terraform_org_name` outside the allowlist. This
is a tool error and comparison is case-insensitive after trimming.

Tools without a `terraform_org_name` argument bypass the tool-level check.
Consequently, ID-based operations are constrained by the token's backend
permissions, not by an additional server-side lookup of the ID's organization.

The experimental official server has no equivalent tool-level organization
check. HTTP validation proves only that the request bearer can access at least
one allowlisted organization. Its `list_workspaces` call can name another
organization accessible to the official server's environment token, which can
also differ from the bearer token.

## 8. Client IP Forwarding

Forwarding is disabled unless `MCP_FORWARD_CLIENT_IP` is exactly `true`.

When enabled, the server MUST select a syntactically valid IPv4 or IPv6 address
using `MCP_REMOTE_IP_METHOD`:

| Method | Selection |
| --- | --- |
| `RemoteAddr` | Direct TCP peer; default and fallback for invalid method names |
| `X-Real-IP` | Valid `X-Real-IP`, else direct TCP peer |
| `X-Forwarded-For` | Entry immediately left of the configured trusted hops, else direct TCP peer |

`MCP_XFF_TRUSTED_HOPS` must be a positive integer to affect XFF selection.
Missing, invalid, zero, or negative values become zero, which makes XFF
selection fail and fall back to the direct peer.

The selected address is added to each public Registry request and to a primary
Terraform client's headers when that client is constructed. Stateful Terraform
client reuse can therefore retain an earlier request's address.

## 9. Tool Invocation Semantics

### 9.0 Input Schemas And Annotation Defaults

The primary server advertises JSON Schemas but does not enable the MCP
library's input-schema validator. It also does not enable output-schema
validation, and no primary tool declares an `outputSchema`.

Consequently, advertised required fields, types, enums, patterns, lengths,
minimums, maximums, and defaults are advisory to clients. The server accepts
unknown properties at the schema layer and does not inject schema defaults.
Middleware or handlers can still inspect an undeclared property, notably
`terraform_org_name` in allowlist middleware and `after` in pagination. Actual
acceptance is defined by each handler and these accessor rules:

| Accessor | Runtime behavior |
| --- | --- |
| `RequireString` | Requires an actual JSON string and errors when absent or another type |
| `GetString` | Returns only an actual JSON string; otherwise returns its handler-supplied default |
| `GetInt` | Accepts integers, truncates JSON numbers, parses decimal integer strings, and otherwise returns its default |
| `GetBool` | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; otherwise returns its default |
| Shared pagination helper | Requires decoded JSON numbers as `float64`; wrong types error rather than coerce |

Every primary `mcp.NewTool` starts with wire-visible annotation defaults
`readOnlyHint: false`, `destructiveHint: true`, `idempotentHint: false`, and
`openWorldHint: true`. Tool constructors override some of those values. An
annotation described as not explicitly set in source still serializes with its
default value.

### 9.1 Results And Errors

Primary tool handlers MUST normally return one text content item. JSON,
JSON:API, Markdown, logs, and base64 payloads are all transported as text.

Handler validation failures and upstream API failures MUST normally return a
non-nil `CallToolResult` with `isError: true`, one text item, and no Go handler
error. The displayed text is the compatibility-visible error.

Rate-limit failures are exceptions: they MUST return no tool result and a
protocol/handler error with one of these exact messages:

```text
rate limit exceeded: too many requests globally
rate limit exceeded: too many requests from this session
```

Resource read failures also return resource protocol errors rather than tool
error content.

Before a dynamic tool is registered, an attempted call produces a tool-not-
found protocol error. Unknown tools, rate-limited calls, and other failures
before handler invocation do not pass through ordinary tool-result behavior.

Handlers generally assume that a successful upstream response includes all
relationships used by `go-tfe`. They do not install panic recovery or convert
every structurally partial success into a tool error. Notable unchecked
relationships occur in token permissions, workspace organization data, team
access responses, run workspace summaries, and no-code module data.

### 9.2 Shared Pagination

Tools documented with shared pagination advertise optional JSON numbers:

| Parameter | Schema | Runtime |
| --- | --- | --- |
| `page` | Minimum 1 | Default 1; zero also selects default |
| `pageSize` | Minimum 1, maximum 100 | Default 30; zero also selects default |

The runtime requires decoded values to be `float64`, truncates fractional
values to integers, and does not enforce the advertised minimum or maximum.
Wrong types become tool errors. A helper parses an unadvertised `after` string
if present, but current list tools do not use it.

### 9.3 Rate Limiting

Tool calls over all transports share a process-global token-bucket limiter and
use a second limiter when a non-empty session ID is available. Defaults are:

| Scope | Sustained rate | Burst |
| --- | ---: | ---: |
| Global | 10 requests/second | 20 |
| Session | 5 requests/second | 10 |

Configuration syntax is `rps:burst`. Both values must parse and be positive;
otherwise that scope retains its default. The global limiter is checked first.
Stateless calls with an empty session ID skip the per-session limiter.

### 9.4 Tool Logging

Every primary tool call that passes the outer rate limiter and reaches the
argument-logging middleware MUST log the tool name and complete argument map at
info level. Values are not redacted. This includes variable values and any
other secrets passed as tool arguments. Public Registry response bodies are
additionally logged at trace level.

The rate limiter is outside the argument logger. Rate-limited calls are not
argument-logged. Calls rejected as unknown before middleware, including dynamic
tools called before registration, are also not argument-logged. Some package-
global logs bypass the configured server logger and remain on stderr.

## 10. Public Registry HTTP Contract

Public Registry requests MUST construct their initial URL at
`https://registry.terraform.io` using hard-coded v1 or v2 paths and never use
`TFE_ADDRESS`. The HTTP client retains Go's default redirect policy, so an HTTP
redirect can move a request to another origin and up to ten redirects may be
followed.

The HTTP client MUST:

- Use a ten-second timeout per retry attempt; the full retried operation can
  exceed ten seconds.
- Honor environment proxy configuration.
- Use `TFE_SKIP_TLS_VERIFY` for TLS verification behavior.
- Send the server user agent and optional selected client IP.
- Retry up to three times only for HTTP 429 responses that include
  `x-ratelimit-reset`.
- Interpret a valid `x-ratelimit-reset` as a Unix timestamp and wait until it.
  A malformed or past non-empty value still enables retry but produces zero or
  non-positive backoff. A future timestamp can impose an arbitrarily long wait
  outside the ten-second request timeout.

Only HTTP 200 is accepted. A non-retried final non-200 response, regardless of
its actual status, is exposed internally as `error: 404 Not Found` and normally
becomes a tool or resource error. After exhausting retried 429 responses,
`retryablehttp` instead returns a `giving up after 4 attempt(s)` error before
the fake-404 check. The non-retried non-200 branch returns before closing its
response body.

Registry calls and guide-resource fetches do not attach the MCP request context
to their outbound HTTP requests. MCP cancellation therefore does not directly
cancel those calls. The per-attempt HTTP timeout does not bound retry backoff.
Primary and official TFE clients use the same inner ten-second transport with
up to three 429 retries. `go-tfe` adds an outer retry policy of up to 30 retries
for 429 and transport failures and, because `RetryServerErrors` is enabled, 5xx
responses. Qualifying failures can therefore cause nested retry amplification.

## 11. Resources

Resources are registered independently of tool filtering and Terraform
authentication. They require an active MCP session so a public HTTP client can
be obtained.

### 11.1 Terraform Style Guide

The resource metadata is:

| Field | Value |
| --- | --- |
| URI | `/terraform/style-guide` |
| Name | `Terraform Style Guide` |
| Description | `Terraform Style Guide` |
| MIME type | `text/markdown` |

Reading it MUST fetch the fixed Terraform `v1.12.x` `style.mdx` document from
HashiCorp's `web-unified-docs` repository. HTTP 200 returns exactly one text
resource with the resource URI, `text/markdown`, and the unmodified body. Any
network, non-200, or body-read failure fails the resource read.

### 11.2 Module Development Guide

The resource metadata is:

| Field | Value |
| --- | --- |
| URI | `/terraform/module-development` |
| Name | `Terraform Module Development Guide` |
| Description | `Terraform Module Development Guide` |
| MIME type | `text/markdown` |

Reading it MUST sequentially fetch six Terraform `v1.12.x` documents and
return six text resources in this order:

| Result URI | Source document |
| --- | --- |
| `/terraform/module-development/index` | `modules/develop/index.mdx` |
| `/terraform/module-development/composition` | `modules/develop/composition.mdx` |
| `/terraform/module-development/structure` | `modules/develop/structure.mdx` |
| `/terraform/module-development/providers` | `modules/develop/providers.mdx` |
| `/terraform/module-development/publish` | `modules/develop/publish.mdx` |
| `/terraform/module-development/refactoring` | `modules/develop/refactoring.mdx` |

Each result uses `text/markdown` and the unmodified body. Any failed fetch
aborts the entire read; partial content is not returned.

### 11.3 Provider Resource Template

The registered template is normalized by Go path joining to:

```text
registry:/providers/{namespace}/name/{name}/version/{version}
```

Its name is `Provider details`, description is `Describes details for a
Terraform provider`, and advertised MIME type is `application/json`.

On read, the handler extracts namespace, name, and version from URI path
segments. Empty, `latest`, or unsupported version syntax resolves the latest
public Registry version. It then resolves the provider-version ID and fetches
provider overview documentation.

Success MUST contain exactly one text resource whose MIME type is
`text/markdown`, whose text is the concatenated overview documentation, and
whose URI is the template string rather than the concrete requested URI. The
returned MIME type intentionally differs from the advertised MIME type.

## 12. Observability

### 12.1 OpenTelemetry Metrics

Metrics setup occurs only on Streamable HTTP startup. It is enabled only when
`OTEL_METRICS_ENABLED` is exactly `true`. The OTLP/HTTP exporter connects to the
configured endpoint without transport TLS. Exporter or meter initialization
failure is logged and does not prevent server startup.

The server creates these instruments:

| Metric | Type | Behavior |
| --- | --- | --- |
| `mcp_tool_calls_total` | Counter | Incremented after each observed primary tool result |
| `mcp_tool_errors_total` | Counter | Incremented only when the result is a `CallToolResult` with `isError: true` |
| `mcp_tool_duration_seconds` | Histogram | Tool duration in seconds |
| `mcp_client_type_total` | Counter | Incremented after initialization and again before tool calls when client info is available |

Tool metrics include `tool.name`, `service.name`, and `service.version`.
Client metrics include client name, version, title, description, service name,
and service version. A protocol/handler error prevents the after-tool hook, so
rate-limit failures, unknown tools, and similar protocol errors increment none
of the custom tool call, error, or duration instruments. Only completed primary
tool results reach these custom metrics.

Tool start times are keyed only by formatted JSON-RPC request ID, without a
session component. Concurrent sessions reusing the same ID can overwrite one
another and produce incorrect durations. A protocol error skips cleanup of that
ID's start entry, so unique errored IDs can remain in the process map.

When the environment switch is enabled, `otelhttp` also wraps the entire HTTP
mux and emits its standard HTTP server instrumentation. This includes health,
redirect, authentication failures, and the experimental endpoint, even though
the latter has no primary custom tool hooks.

### 12.2 Instana

When `INSTANA_ENABLED` is exactly `true`, HTTP startup MUST initialize an
Instana collector using `INSTANA_SERVICE_NAME` or the default server name and
wrap the entire HTTP mux for tracing. Stdio does not initialize Instana.

## 13. Experimental Official SDK Endpoint

When `TF_X_OFFICIAL_SDK_ENABLED` is exactly `true`, the HTTP server MUST mount a
second MCP implementation at `<endpoint>/official` and its trailing-slash form.
It MUST use the same outer CORS, Terraform-context, and organization-membership
HTTP wrappers as the primary endpoint.

The experimental server MUST:

- Identify itself as `terraform-mcp-official` with the embedded base version.
- Use the configured heartbeat as its SDK keepalive when positive.
- Use the same stateful/stateless transport setting.
- Expose `list_workspaces` only when that tool is selected.
- Expose no resources, resource templates, prompts, instructions, elicitation,
  primary rate limiting, primary tool logging, or primary session hooks.
- Advertise the official SDK's default empty logging capability and advertise
  tool list-change support when `list_workspaces` is registered.

Its `list_workspaces` input contains required `terraform_org_name` and optional
`project_id`, `search_query`, comma-separated `tags`, `exclude_tags`, and
`wildcard_name`. It exposes no pagination input, makes one backend list request,
and does not follow a next-page link. Its advertised description nevertheless
claims pagination and refers to `get_workspace_details`, which this endpoint
does not expose. Empty results are errors. A success contains both typed
`structuredContent` and the official SDK's JSON text fallback. The `items`
entries contain `id`, `workspace_name`, `description`, `environment`,
`created_at`, and `execution_mode`.

Missing credentials, backend failures, and empty results from the typed handler
are converted by the official SDK into `isError: true` tool results with one
text item rather than JSON-RPC protocol errors.

The official implementation creates one process-wide Terraform client on its
first tool call and reuses the first creation result, including an error. Due
to a distinct private context-key type, it does not consume token/address/TLS
values inserted by the primary Terraform-context middleware and therefore
normally resolves those values from server environment only. It does not read
the Terraform CLI credentials file and does not add the primary shared-secret
or client-IP headers. The HTTP allowlist can consequently validate a request's
bearer token while the tool itself uses a different environment token. There is
no tool-level allowlist check on the requested `terraform_org_name`.

## 14. Shutdown

SIGINT and SIGTERM MUST cancel the server context. Stdio stops listening and
returns. HTTP attempts graceful shutdown for up to five seconds. Stateful MCP
session teardown invokes client and limiter cleanup when the transport emits
the unregister hook.
