# Registry Tool Behavior

## 1. Scope

This document specifies the nine `registry` tools and four
`registry-private` tools. All results use one MCP text content item. JSON and
Markdown mentioned below are encoded in that text item.

Public Registry tools are registered at startup when selected. Private
Registry tools are dynamically registered after the first primary session
constructs a TFE client when those tools are selected, and require a TFE client
in the calling session. Client construction does not prove token authorization.
Once registered, private tools remain globally listed after that session ends.

All thirteen tools explicitly advertise `readOnlyHint: true`,
`destructiveHint: false`, and `openWorldHint: true`. The primary MCP library's
default also makes all thirteen advertise `idempotentHint: false`.

The primary server does not validate advertised input schemas. Required fields,
types, enums, defaults, and ranges below describe wire metadata; handler
accessors and explicit checks define runtime acceptance. Unknown arguments are
not rejected merely for being absent from schema, though middleware may inspect
them. None of these tools advertises an output schema. The experimental
official endpoint exposes no Registry tools.

Provider version syntax is considered supported only when it matches
`^v?(\d+\.\d+\.\d+(-[a-zA-Z0-9]+)?)$`. Other syntax, including dotted
prereleases and build metadata, is treated as a request for the latest version
where a handler performs version resolution.

Shared public HTTP behavior is specified in
[runtime.md](runtime.md#10-public-registry-http-contract). In particular,
ordinary non-200 responses become the synthetic text `404 Not Found`.
`search_policies` and `get_latest_module_version` include that synthetic status
in their tool errors; several other handlers replace it with more generic text.

## 2. Public Registry Tools

### 2.1 `search_providers`

Purpose: locate provider documentation and produce a `provider_doc_id` for
`get_provider_details`.

Annotation title:
`Identify the most relevant provider document ID for a Terraform service`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `provider_name` | string | Required | Lowercased |
| `provider_namespace` | string | Required | Lowercased; an absent/empty value reaching the handler defaults to `hashicorp` |
| `service_slug` | string | Required | Lowercased; an empty value is rejected |
| `provider_document_type` | string | Required; enum below | Missing/wrong-type defaults to `resources`; an explicit string is used exactly as supplied because it overwrites an earlier normalized value |
| `provider_version` | string | Optional | Lowercased; supported syntax is accepted without existence lookup; absent, `latest`, or unsupported syntax resolves latest |

`provider_document_type` accepts:

```text
resources
data-sources
functions
guides
overview
actions
list-resources
```

#### Behavior

For a supported explicit version, the tool uses that value without first
checking that it exists. For latest/unsupported syntax, it looks up latest in
the requested namespace; only failure of that latest-version lookup retries in
the `hashicorp` namespace. A later documentation or provider-version-ID failure
does not trigger namespace fallback.

Resource and data-source discovery uses public Registry v1 data and filters by
the lowercased `service_slug`. Other document categories use v2 provider-doc
APIs and can page until an empty page. For v2 categories, `service_slug` is
required and checked for emptiness but does not filter results; overview always
uses the overview/index path. Description-snippet lookup failures for
individual matches are non-fatal.

For v1 categories, success is Markdown-like text headed `Available
Documentation` with repeated `providerDocID`, `Title`, `Category`, and
`Description` fields. Every v2 success begins `# <provider> provider docs`.
Non-overview v2 content then contains the same candidate-list fields; overview
contains overview content instead.

No matching v1 or non-overview v2 documents is a tool error. An overview with
no returned document succeeds with only the provider-docs heading.
Provider/version resolution, Registry transport, and response parsing failures
are otherwise tool errors.

Compatibility note: explicit uppercase or empty `provider_document_type` is not
normalized at the final dispatch. It normally enters the v1 path and fails to
match a category rather than using the advertised enum/default.

### 2.2 `get_provider_details`

Purpose: fetch one complete provider document selected by `search_providers`.

Annotation title:
`Fetch detailed Terraform provider documentation using a document ID`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `provider_doc_id` | string | Required | Must be non-empty base-10 integer text |

The tool MUST call the public Registry v2 provider-document endpoint for that
ID and return the document content verbatim. Empty, non-numeric, unknown, and
unreadable IDs are tool errors.

### 2.3 `get_latest_provider_version`

Purpose: resolve the latest public version of a provider.

Annotation title: `Get Latest Provider Version`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `namespace` | string | Required | Lowercased |
| `name` | string | Required | Lowercased |

Success MUST be only the version string. Registry and parse failures are tool
errors.

### 2.4 `get_provider_capabilities`

Purpose: summarize the documentation categories supported by a provider.

Annotation title:
`Get Terraform provider capabilities and supported features`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `namespace` | string | Required | Lowercased |
| `name` | string | Required | Lowercased |
| `version` | string | Optional | Defaults to `latest`; `latest` or unsupported syntax resolves the latest version |

The tool MUST fetch v1 provider documentation and count only HCL documents. It
groups documents by category. A category with at most ten entries includes all
entries; a larger category includes three examples and an `and N more` line.
Categories with no entries are omitted. Category order is not stable because
the implementation iterates a map.

Success begins `Provider Capabilities: <namespace>/<name> (v<version>)`. A
provider with no HCL capabilities returns successful text `No capabilities
found for this provider.` Registry and parse failures are tool errors.

### 2.5 `search_modules`

Purpose: search public modules and produce compatible four-part `module_id`
values for `get_module_details`.

Annotation title:
`Search and match Terraform modules based on name and relevance`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `module_query` | string | Required | Lowercased; an explicitly empty string is accepted and requests an unfiltered list |
| `current_offset` | number | Optional; default 0; minimum 0 | Numbers are truncated, decimal integer strings are accepted, and unconvertible values default to 0; negative values are accepted |

The tool fetches one Registry search page and sorts the returned modules by
descending download count. Success is Markdown-like text containing, for each
module, `module_id`, name, description, download count, verification status,
and publication date.

No results and response parsing failures are tool errors. Every Registry
transport/status failure is deliberately relabeled as `no modules found for
query: <query> - try a different search term`.

### 2.6 `get_module_details`

Purpose: fetch documentation for one exact public module version.

Annotation title:
`Retrieve documentation for a specific Terraform module`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `module_id` | string | Required | Must be non-empty and contain exactly four `/`-separated components; the complete value is lowercased |

The required format is:

```text
namespace/name/provider/version
```

Success is Markdown headed `registry://modules/<namespace>/<name>` and includes
the description, version, namespace, and source. Inputs, outputs, provider
dependencies, and example README sections are included when present. The root
module README and submodule details are decoded but not emitted.

An invalid four-part format, unknown module, transport failure, or response
parse failure is a tool error. Components are not otherwise validated locally.

### 2.7 `get_latest_module_version`

Purpose: resolve the latest public version of a module.

Annotation title: `Get Latest Module Version`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `module_publisher` | string | Required | Lowercased |
| `module_name` | string | Required | Lowercased |
| `module_provider` | string | Required | Lowercased |

Success MUST be only the version string. Missing modules and Registry or parse
failures are tool errors.

### 2.8 `search_policies`

Purpose: search public Terraform policies and produce a
`terraform_policy_id` for `get_policy_details`.

Annotation title:
`Search and match Terraform policies based on name and relevance`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `policy_query` | string | Required | Must be non-empty and is lowercased |

The tool MUST fetch at most the first 100 public policies with latest versions
included and perform local literal substring matching against lowercase policy
title or name. It does not request subsequent pages.

Success is text headed `Matching Terraform Policies` with repeated
`terraform_policy_id`, name, title, and download count. An empty query, no
matches, and Registry or parse failures are tool errors.

### 2.9 `get_policy_details`

Purpose: fetch a public policy library entry and construct usage guidance.

Annotation title:
`Fetch detailed Terraform policy documentation using a terraform_policy_id`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_policy_id` | string | Required | Must be non-empty; no local path-format validation |

The tool fetches policy metadata, policy modules, and policy-library
inclusions. Success is Markdown containing the first extracted README section,
policy-module blocks, an advisory `policies.hcl` template, and policy
names/checksums. A valid response with no included policies can still succeed
with an empty policy list.

Missing IDs and Registry or parse failures are tool errors.

Compatibility note: IDs returned by `search_policies` begin `policies/...`
without a leading slash. The generated HCL URLs concatenate that ID directly
after `/v2`, producing `https://registry.terraform.io/v2policies/...`.

## 3. Private Registry Tools

### 3.1 `search_private_modules`

Purpose: list or search modules in an organization's private Registry,
including no-code module identifiers.

Annotation title:
`Search for private modules in Terraform Cloud/Enterprise`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Passed without trimming |
| `search_query` | string | Optional | Defaults to empty; passed without trimming |
| `page_size` | number | Optional; minimum 1, maximum 100 | `GetInt` truncates numbers and accepts decimal integer strings; unconvertible values default to 100; runtime validates 1-100 |
| `page_number` | number | Optional; minimum 1 | `GetInt` truncates numbers and accepts decimal integer strings; unconvertible values default to 1; runtime validates minimum 1 |

Success is plain text containing organization, page information, and each
module's `private_module_id`, name, namespace, registry, provider, timestamps,
no-code status, and `no_code_module_id` when available. A `Search Query` line is
included only for a non-empty query. Non-empty results end with pagination.

No matches return a successful explanatory message before pagination is
appended. For non-empty results, pagination is printed only when the API
supplies a non-nil pagination object. API and explicit range-validation
failures are tool errors.

### 3.2 `get_private_module_details`

Purpose: return usage and metadata for one private or public module visible
through HCP Terraform/TFE.

Annotation title: `Get detailed information about a private module`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `private_module_id` | string | Required | Whole value is trimmed and must have exactly three `/`-separated components; components are not individually trimmed, lowercased, or checked for emptiness/syntax |
| `registry_name` | string | Optional; enum `private`, `public`; default `private` | Missing/wrong-type defaults to `private`; explicit strings are trimmed but empty or invalid values are not locally rejected |
| `private_module_version` | string | Optional | Trimmed; empty requests API-selected/latest details |

The module ID format is:

```text
namespace/name/provider
```

Success is Markdown/plain text containing an HCL module block and basic
metadata. Organization, permissions, and VCS sections are conditional. Inputs,
outputs, provider dependencies, resources, and a cleaned non-empty root README
are included only when richer registry-module metadata supplies them. The HCL
version is omitted when the base module has no version-status entries.

`private_module_version` applies only to the richer metadata request. The HCL
block takes its version from the first base-module version-status entry, so it
can disagree with the requested metadata version. The HCL source always uses
the configured TFE hostname, including when `registry_name` is `public`.

Every failure to read the base registry module, including authorization,
transport, validation, and absence, is collapsed to a `module not found` tool
error. Failure to fetch the richer Terraform module details is non-fatal; the
tool returns the available basic information.

### 3.3 `search_private_providers`

Purpose: list or search providers visible through an organization's private or
public Registry view.

Annotation title:
`Search for private providers in Terraform Cloud/Enterprise`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `search_query` | string | Optional | Trimmed; defaults to empty |
| `registry_name` | string | Optional; enum `private`, `public`; default `private` | Missing/wrong-type defaults to `private`; explicit strings are trimmed; empty omits the registry filter and invalid values are not locally rejected |
| `page_size` | number | Optional; minimum 1, maximum 100 | `GetInt` truncates numbers and accepts decimal integer strings; unconvertible values default to 20; runtime validates 1-100 |
| `page_number` | number | Optional; minimum 1 | `GetInt` truncates numbers and accepts decimal integer strings; unconvertible values default to 1; runtime validates minimum 1 |

Success is plain text containing organization, registry, page, each provider's
namespace/name, ID, registry, and timestamps, followed by pagination
information. A search line is included only for a non-empty query. A `Versions`
line is included only when versions exist.

No matches return a successful explanatory message before pagination is
appended. For non-empty results, pagination is printed only when the API
supplies a non-nil pagination object. API and explicit range-validation
failures are tool errors.

### 3.4 `get_private_provider_details`

Purpose: return usage, metadata, permissions, versions, and platforms for one
provider visible through HCP Terraform/TFE.

Annotation title: `Get detailed information about a private provider`.

#### Input

| Name | Type | Schema | Runtime behavior |
| --- | --- | --- | --- |
| `terraform_org_name` | string | Required | Trimmed |
| `private_provider_namespace` | string | Required | Trimmed |
| `private_provider_name` | string | Required | Trimmed |
| `registry_name` | string | Optional; enum `private`, `public`; default `private` | Missing/wrong-type defaults to `private`; explicit strings are trimmed but empty or invalid values are not locally rejected |
| `include_versions` | boolean | Optional; default true | Accepts booleans, Go-parseable boolean strings, and numeric zero/nonzero values; otherwise defaults to true |

Success is plain text/Markdown with an HCL `required_providers` example,
provider ID/name/namespace/registry, timestamps, and permissions. Organization
and links are conditional. When versions exist, the HCL block uses the first
one; version/platform sections are included when requested. The HCL version is
omitted if none are returned, and a `no version information` message appears
only when `include_versions` is true.

The HCL provider source is always `<namespace>/<name>` without a TFE hostname,
including for the default private-registry mode; Terraform therefore interprets
that address as public-registry syntax.

Every base-provider read failure, including authorization, transport,
validation, and absence, is collapsed to a `provider not found` tool error that
directs the caller to search first.
