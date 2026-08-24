// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// listResourceSchemaFile is the top-level shape of a provider schema snapshot
// produced by `terraform providers schema -json`.
type listResourceSchemaFile struct {
	ListResourceSchemas map[string]listResourceEntry `json:"list_resource_schemas"`
}

// listResourceEntry is a single resource type entry in list_resource_schemas.
type listResourceEntry struct {
	Block listResourceBlock `json:"block"`
}

// listResourceBlock holds the attribute and block_types maps for a list
// resource. Both are present in real terraform providers schema -json output.
type listResourceBlock struct {
	Attributes map[string]listResourceAttribute `json:"attributes"`
	// BlockTypes holds nested block attributes (e.g. "filter", "tags").
	// Both Attributes and BlockTypes are present in real `terraform providers schema -json`
	// output and are needed to fully describe the filterable surface of a list resource.
	BlockTypes map[string]listResourceBlockType `json:"block_types"`
}

// listResourceBlockType describes a nested block attribute such as "filter".
// nesting_mode controls whether the block may appear multiple times:
// "list" and "set" → repeatable; "single" and "map" → not repeatable.
type listResourceBlockType struct {
	// NestingMode is "single", "list", "set", or "map".
	// "list" and "set" mean the same block-type attribute may be submitted
	// multiple times and the backend coalesces the values into an array.
	NestingMode string            `json:"nesting_mode"`
	Required    bool              `json:"required"`
	Block       listResourceBlock `json:"block"`
}

// listResourceAttribute captures the per-attribute metadata we expose to the
// agent.
type listResourceAttribute struct {
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Optional    bool   `json:"optional"`
	Type        any    `json:"type"` // "string" | "bool" | "number" | ["list","string"] | ...
}

// mdLinkRe strips markdown hyperlinks, keeping the link text.
// e.g. "[AWS docs](https://example.com)" → "AWS docs"
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

// GenerateQueryConfiguration returns the MCP tool that accepts a
// list_resource_schemas JSON blob and emits structured instructions plus an
// annotated scaffold the agent can use to build a No-Code Query Config payload.
func GenerateQueryConfiguration(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("generate_query_configuration",
			mcp.WithDescription(generateQueryConfigDescription),
			mcp.WithTitleAnnotation("Generate a No-Code Query Configuration from a list_resource_schemas schema"),
			mcp.WithOpenWorldHintAnnotation(false),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("list_resource_schemas",
				mcp.Required(),
				mcp.Description(
					"The list_resource_schemas JSON object for a single provider version. "+
						"This is the value returned by the `provider_list_schema_list` tool, "+
						"or the value of the `list_resource_schemas` key in a provider schema "+
						"snapshot produced by `terraform providers schema -json`. "+
						"The value must be a JSON object whose keys are resource type names "+
						"(e.g. \"aws_instance\") and whose values are schema blocks containing "+
						"an `attributes` map and optionally a `block_types` map.",
				),
			),
			mcp.WithString("provider_name",
				mcp.Required(),
				mcp.Description(
					"Short provider name used as the `provider` value in generated list blocks. "+
						"Examples: \"aws\", \"azurerm\", \"google\".",
				),
			),
			mcp.WithString("provider_namespace",
				mcp.Required(),
				mcp.Description(
					"Provider namespace. Examples: \"hashicorp\", \"Azure\", \"juju\".",
				),
			),
			mcp.WithString("provider_version",
				mcp.Required(),
				mcp.Description("Provider version string, e.g. \"6.33.0\"."),
			),
			mcp.WithString("resource_types",
				mcp.Description(
					"Optional comma-separated list of resource type names to include in the "+
						"output (e.g. \"aws_instance,aws_s3_bucket\"). "+
						"When omitted all resource types in the schema are described.",
				),
			),
		),
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return generateQueryConfigurationHandler(ctx, request, logger)
		},
	}
}

func generateQueryConfigurationHandler(_ context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	// ── 1. Parse inputs ───────────────────────────────────────────────────────
	rawSchema, err := request.RequireString("list_resource_schemas")
	if err != nil || rawSchema == "" {
		return toolErrorf(logger, "missing required input: list_resource_schemas")
	}

	providerName, err := request.RequireString("provider_name")
	if err != nil || providerName == "" {
		return toolErrorf(logger, "missing required input: provider_name")
	}
	providerName = strings.ToLower(providerName)

	providerNamespace, err := request.RequireString("provider_namespace")
	if err != nil || providerNamespace == "" {
		return toolErrorf(logger, "missing required input: provider_namespace")
	}

	providerVersion, err := request.RequireString("provider_version")
	if err != nil || providerVersion == "" {
		return toolErrorf(logger, "missing required input: provider_version")
	}

	resourceTypesFlag := request.GetString("resource_types", "")

	// ── 2. Unmarshal the schema ───────────────────────────────────────────────
	//
	// Accept both the raw map form {"aws_instance": {...}, ...} and the
	// wrapped file form {"list_resource_schemas": {"aws_instance": {...}, ...}}.
	schemas, err := parseListResourceSchemas(rawSchema)
	if err != nil {
		return toolErrorf(logger, "failed to parse list_resource_schemas: %v", err)
	}
	if len(schemas) == 0 {
		return toolErrorf(logger, "list_resource_schemas contains no resource types")
	}

	// ── 3. Filter to requested resource types ─────────────────────────────────
	resourceTypes := filterResourceTypes(schemas, resourceTypesFlag)
	if len(resourceTypes) == 0 {
		return toolErrorf(logger, "none of the requested resource types were found in the schema")
	}

	// ── 4. Build the output ───────────────────────────────────────────────────
	var b strings.Builder
	writeInstructions(&b, providerNamespace, providerName, providerVersion)
	writeResourceCatalog(&b, schemas, resourceTypes)
	writeExamplePayload(&b, schemas, resourceTypes, providerName, providerNamespace, providerVersion)
	writeVariableNotes(&b)

	return mcp.NewToolResultText(b.String()), nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toolErrorf(logger *log.Logger, format string, args ...any) (*mcp.CallToolResult, error) {
	msg := fmt.Sprintf(format, args...)
	if logger != nil {
		logger.Errorf("generate_query_configuration error: %s", msg)
	}
	return mcp.NewToolResultError(msg), nil
}

// parseListResourceSchemas accepts either the wrapped file format or the bare
// resource-type map. It uses a structural heuristic: a bare map whose first
// entry has a "block" child is the resource-type map; anything else is treated
// as the wrapped file format.
func parseListResourceSchemas(raw string) (map[string]listResourceEntry, error) {
	// Try the bare map first. A valid bare map has entries with a "block" key
	// rather than a top-level "list_resource_schemas" key.
	var bare map[string]listResourceEntry
	if err := json.Unmarshal([]byte(raw), &bare); err == nil {
		if _, hasWrapper := bare["list_resource_schemas"]; !hasWrapper {
			return bare, nil
		}
	}

	// Try the wrapped file format.
	var wrapped listResourceSchemaFile
	if err := json.Unmarshal([]byte(raw), &wrapped); err != nil {
		return nil, fmt.Errorf("input is not valid JSON: %w", err)
	}
	if wrapped.ListResourceSchemas == nil {
		return nil, fmt.Errorf("no list_resource_schemas key found in input")
	}
	return wrapped.ListResourceSchemas, nil
}

// filterResourceTypes returns the sorted slice of resource type keys to
// include, optionally narrowed by the comma-separated resourceTypesFlag.
func filterResourceTypes(schemas map[string]listResourceEntry, resourceTypesFlag string) []string {
	if resourceTypesFlag == "" {
		keys := make([]string, 0, len(schemas))
		for k := range schemas {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}

	requested := strings.Split(resourceTypesFlag, ",")
	var result []string
	for _, rt := range requested {
		rt = strings.TrimSpace(rt)
		if _, ok := schemas[rt]; ok {
			result = append(result, rt)
		}
	}
	sort.Strings(result)
	return result
}

// humanType converts a raw Terraform type value (string, bool, number, or a
// JSON array like ["list","string"] or ["object",{"key":"string"}]) to a
// readable label that matches Terraform type expression syntax.
func humanType(raw any) string {
	if raw == nil {
		return "string"
	}
	switch v := raw.(type) {
	case string:
		return v
	case []any:
		if len(v) == 0 {
			return "any"
		}
		// First element is the type constructor name (e.g. "list", "object").
		outer, _ := v[0].(string)
		if len(v) == 1 {
			return outer
		}
		// ["object", {"key": "type", ...}] — render as object({key: type, ...})
		if outer == "object" {
			if attrs, ok := v[1].(map[string]any); ok {
				keys := make([]string, 0, len(attrs))
				for k := range attrs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				pairs := make([]string, 0, len(keys))
				for _, k := range keys {
					pairs = append(pairs, fmt.Sprintf("%s: %s", k, humanType(attrs[k])))
				}
				return fmt.Sprintf("object({%s})", strings.Join(pairs, ", "))
			}
		}
		// ["list","string"], ["set","string"], ["map","string"], etc.
		return fmt.Sprintf("%s(%s)", outer, humanType(v[1]))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// stripMarkdownLinks removes markdown hyperlinks from s, keeping the link text.
// Handles multiple links in a single string safely.
func stripMarkdownLinks(s string) string {
	return mdLinkRe.ReplaceAllString(s, "$1")
}

// cleanDescription strips markdown links, collapses newlines, and truncates.
func cleanDescription(raw string) string {
	desc := strings.ReplaceAll(raw, "\n", " ")
	desc = stripMarkdownLinks(desc)
	if len(desc) > 120 {
		desc = desc[:117] + "..."
	}
	return desc
}

// ── output writers ────────────────────────────────────────────────────────────

func writeInstructions(b *strings.Builder, namespace, name, version string) {
	b.WriteString("# No-Code Query Configuration Guide\n\n")
	b.WriteString(fmt.Sprintf("Provider: **%s/%s** version **%s**\n\n", namespace, name, version))

	b.WriteString("## What is a No-Code Query Config?\n\n")
	b.WriteString("A No-Code Query Config describes the provider and resources passed to the\n")
	b.WriteString("`execute_query` MCP tool to create **and immediately execute** a Terraform Search query from HCP Terraform\n")
	b.WriteString("without writing `.tfquery.hcl` files by hand.\n\n")

	b.WriteString("## Pre-conditions\n\n")
	b.WriteString("Before calling the endpoint, verify:\n\n")
	b.WriteString("1. The workspace's Terraform version is **>= 1.14.0**. Requests against older versions are rejected with `422 Unprocessable Entity`.\n")
	b.WriteString("2. After `execute_query` succeeds, pass the ID from its `latest-query-run` relationship to the `get_query_status` MCP tool. Once it returns a terminal status, pass the same ID to `get_query_summary` to retrieve the parsed result. Do not use curl or call the query API directly.\n")
	b.WriteString("3. Pass `organization_name` and `workspace_name` separately when calling `execute_query`; it resolves the workspace ID through go-tfe.\n\n")

	b.WriteString("## Query configuration structure\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "generate_config_out": false,
  "no_code_query_providers": [
    {
      "namespace": "<provider-namespace>",
      "name":      "<provider-name>",
      "version":   "<provider-version>",
      "no_code_query_resources": [
        {
          "body": {
            "resource_type": "<resource-type-name>",
            "limit":         100,
            "attributes": [
              { "attribute": "<attr-name>", "value": "<attr-value>" }
            ]
          }
        }
      ]
    }
  ]
}
`)
	b.WriteString("```\n\n")

	b.WriteString("## Rules for building the payload\n\n")
	b.WriteString("1. **`organization_name` / `workspace_name`** — pass these separately to `execute_query`; do not include them in this JSON object.\n")
	b.WriteString("2. **`generate_config_out`** — optional boolean. Omit it (or set to `false`) to skip HCL scaffolding. Set to `true` to instruct Terraform to emit a `generated_config.tf` file containing importable HCL for each discovered resource.\n")
	b.WriteString("3. **`namespace` / `name` / `version`** — must match a provider returned by `provider_list_schema_list`.\n")
	b.WriteString("4. **`resource_type`** — must be one of the resource type keys listed in the schema catalog below.\n")
	b.WriteString("5. **`attributes`** — include only attributes that appear in the schema for that resource type.\n")
	b.WriteString("   - Omit optional attributes that you do not need to filter on.\n")
	b.WriteString("   - Required attributes MUST be included.\n")
	b.WriteString("   - Scalar attributes: value is a string, bool, or number.\n")
	b.WriteString("   - Block-type attributes: value is a JSON object (see section below).\n")
	b.WriteString("6. **`limit`** — optional positive integer; caps results returned. Omitting it is valid; the HCP Terraform UI defaults to **100**. Explicitly pass `100` to match UI behaviour.\n")
	b.WriteString("7. **Variable injection** — if a value should come from a Terraform workspace variable, use `${var.<name>}` as the exact value string (no prefix or suffix text).\n")
	b.WriteString("   The backend automatically emits a `variable { <name> {} }` declaration in `main.tfquery.json`.\n\n")

	b.WriteString("## How the payload becomes a Terraform query\n\n")
	b.WriteString("HCP Terraform transforms the submitted no-code query into two files that are uploaded as a configuration tarball:\n\n")
	b.WriteString("- **`main.tfquery.json`** — the list query block (resource type, provider, config, optional limit, variable references)\n")
	b.WriteString("- **`main.tf.json`** — the provider bootstrap block (`terraform.required_providers`)\n\n")
	b.WriteString("Example `main.tfquery.json` for a single resource block:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{
  "list": {
    "<resource_type>": {
      "<ncqryres-label>": {
        "provider": "<provider-name>",
        "limit":    100,
        "config": {
          "<attr-name>": "<attr-value>"
        }
      }
    }
  },
  "variable": {
    "<var-name>": {}
  }
}
`)
	b.WriteString("```\n\n")

	b.WriteString("## Block-type (nested) attributes\n\n")
	b.WriteString("Some attributes are **block types** — they accept a nested JSON object as their value rather than a scalar.\n")
	b.WriteString("Block types are listed separately in the catalog below (marked **block-type**).\n\n")
	b.WriteString("Pass a block-type attribute as a JSON object for `value`:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{ "attribute": "filter", "value": { "name": "tag:Owner", "values": ["TeamA"] } }
`)
	b.WriteString("```\n\n")
	b.WriteString("### Repeatable block types\n\n")
	b.WriteString("Block types with `nesting_mode` of `list` or `set` may appear **more than once** in the `attributes` array — submit multiple entries with the same `attribute` key. The backend coalesces them into an array:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`"attributes": [
  { "attribute": "filter", "value": { "name": "tag:Owner",  "values": ["TeamA"] } },
  { "attribute": "filter", "value": { "name": "tag:Region", "values": ["us-east-1"] } }
]
`)
	b.WriteString("```\n\n")
	b.WriteString("Block types with `nesting_mode` of `single` or `map` must appear **at most once**.\n\n")
	b.WriteString("---\n\n")
}

func writeResourceCatalog(b *strings.Builder, schemas map[string]listResourceEntry, resourceTypes []string) {
	b.WriteString("## Resource Type Catalog\n\n")
	b.WriteString("The following resource types are available from this provider version.\n")
	b.WriteString("For each type, scalar attributes and block-type attributes are listed separately.\n\n")

	for _, rt := range resourceTypes {
		entry := schemas[rt]
		b.WriteString(fmt.Sprintf("### `%s`\n\n", rt))

		attrs := entry.Block.Attributes
		blockTypes := entry.Block.BlockTypes

		if len(attrs) == 0 && len(blockTypes) == 0 {
			b.WriteString("_No filterable attributes — the resource type can be listed without any config._\n\n")
			continue
		}

		// ── Scalar attributes ──────────────────────────────────────────────
		if len(attrs) > 0 {
			attrNames := sortedKeys(attrs)

			b.WriteString("**Scalar attributes**\n\n")
			b.WriteString("| Attribute | Required | Type | Description |\n")
			b.WriteString("|-----------|----------|------|-------------|\n")

			for _, attrName := range attrNames {
				attr := attrs[attrName]
				reqLabel := "optional"
				if attr.Required {
					reqLabel = "**required**"
				}
				desc := cleanDescription(attr.Description)
				b.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | %s |\n",
					attrName, reqLabel, humanType(attr.Type), desc))
			}
			b.WriteString("\n")
		}

		// ── Block-type attributes ──────────────────────────────────────────
		if len(blockTypes) > 0 {
			btNames := sortedKeys(blockTypes)

			b.WriteString("**Block-type attributes** (pass as a JSON object for `value`)\n\n")
			b.WriteString("| Block attribute | Required | nesting_mode | Sub-attributes |\n")
			b.WriteString("|-----------------|----------|--------------|----------------|\n")

			for _, btName := range btNames {
				bt := blockTypes[btName]
				reqLabel := "optional"
				if bt.Required {
					reqLabel = "**required**"
				}

				// Summarise sub-attributes as "key(type)" pairs.
				subAttrs := bt.Block.Attributes
				subNames := sortedKeys(subAttrs)
				subSummary := make([]string, 0, len(subNames))
				for _, sub := range subNames {
					subSummary = append(subSummary, fmt.Sprintf("`%s` (%s)", sub, humanType(subAttrs[sub].Type)))
				}
				subStr := strings.Join(subSummary, ", ")
				if subStr == "" {
					subStr = "_none_"
				}

				nestingMode := bt.NestingMode
				if nestingMode == "" {
					nestingMode = "single"
				}
				repeatNote := ""
				if nestingMode == "list" || nestingMode == "set" {
					repeatNote = " _(repeatable)_"
				}

				b.WriteString(fmt.Sprintf("| `%s` | %s | `%s`%s | %s |\n",
					btName, reqLabel, nestingMode, repeatNote, subStr))
			}
			b.WriteString("\n")
		}
	}
}

func writeExamplePayload(b *strings.Builder, schemas map[string]listResourceEntry, resourceTypes []string, providerName, providerNamespace, providerVersion string) {
	b.WriteString("## Example No-Code Query Payload\n\n")
	b.WriteString("The example below includes up to 3 resource types from this provider. ")
	b.WriteString("Required scalar and block-type attributes are filled with placeholder values; optional ones are omitted. ")
	b.WriteString("`generate_config_out` is omitted — it defaults to `false`; add it only when you need Terraform to emit `generated_config.tf`.\n\n")

	// Pick up to 3 resource types for the example.
	exampleTypes := resourceTypes
	if len(exampleTypes) > 3 {
		exampleTypes = exampleTypes[:3]
	}

	resources := make([]map[string]any, 0, len(exampleTypes))
	for _, rt := range exampleTypes {
		entry := schemas[rt]
		attrs := buildExampleAttributes(entry.Block.Attributes, entry.Block.BlockTypes)
		body := map[string]any{
			"resource_type": rt,
			"limit":         100,
			"attributes":    attrs,
		}
		resources = append(resources, map[string]any{"body": body})
	}

	payload := map[string]any{
		"no_code_query_providers": []any{
			map[string]any{
				"namespace":               providerNamespace,
				"name":                    providerName,
				"version":                 providerVersion,
				"no_code_query_resources": resources,
			},
		},
	}

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		b.WriteString("_(could not render example payload)_\n\n")
		return
	}

	b.WriteString("```json\n")
	b.Write(jsonBytes)
	b.WriteString("\n```\n\n")
}

// buildExampleAttributes creates placeholder attribute entries from both scalar
// attributes and block types. Required items always appear; optional ones are
// skipped to keep the example concise.
func buildExampleAttributes(attrs map[string]listResourceAttribute, blockTypes map[string]listResourceBlockType) []map[string]any {
	var result []map[string]any

	// Required scalar attributes first.
	for _, name := range sortedKeys(attrs) {
		attr := attrs[name]
		if !attr.Required {
			continue
		}
		result = append(result, map[string]any{
			"attribute": name,
			"value":     exampleValueForType(name, attr.Type),
		})
	}

	// Required block-type attributes.
	for _, name := range sortedKeys(blockTypes) {
		bt := blockTypes[name]
		if !bt.Required {
			continue
		}
		result = append(result, map[string]any{
			"attribute": name,
			"value":     buildExampleBlockValue(bt.Block.Attributes),
		})
	}

	return result
}

// buildExampleBlockValue builds a placeholder object for a block-type value
// from its sub-attribute schema.
func buildExampleBlockValue(subAttrs map[string]listResourceAttribute) map[string]any {
	obj := make(map[string]any, len(subAttrs))
	for _, name := range sortedKeys(subAttrs) {
		attr := subAttrs[name]
		obj[name] = exampleValueForType(name, attr.Type)
	}
	if len(obj) == 0 {
		obj["<key>"] = "<value>"
	}
	return obj
}

// exampleValueForType returns a type-appropriate placeholder value.
func exampleValueForType(attrName string, rawType any) any {
	t := humanType(rawType)
	switch {
	case t == "bool" || t == "boolean":
		return false
	case t == "number":
		return 0
	case strings.HasPrefix(t, "list") || strings.HasPrefix(t, "set"):
		return []string{"<value>"}
	case strings.HasPrefix(t, "map") || strings.HasPrefix(t, "object"):
		return map[string]string{"<key>": "<value>"}
	default:
		return fmt.Sprintf("<your-%s>", strings.ReplaceAll(attrName, "_", "-"))
	}
}

func writeVariableNotes(b *strings.Builder) {
	b.WriteString("---\n\n")
	b.WriteString("## Variable Injection Reference\n\n")
	b.WriteString("To reference a Terraform workspace variable, use `${var.<name>}` as the attribute value:\n\n")
	b.WriteString("```json\n")
	b.WriteString(`{ "attribute": "region", "value": "${var.aws_region}" }
`)
	b.WriteString("```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- The entire value string must be `${var.<name>}` (no prefix/suffix text).\n")
	b.WriteString("- The variable name follows Terraform identifier syntax: `[a-zA-Z_][a-zA-Z0-9_-]*`.\n")
	b.WriteString("- Each referenced variable name automatically generates a `variable { <name> {} }` declaration in `main.tfquery.json`.\n")
	b.WriteString("- Nested arrays and objects are walked recursively, so variable references inside block-type values are also extracted.\n\n")

	b.WriteString("## Common Mistakes to Avoid\n\n")
	b.WriteString("| Mistake | Correct approach |\n")
	b.WriteString("|---------|------------------|\n")
	b.WriteString("| Using an attribute name not in the schema for that resource type | Only include attributes listed in the catalog above |\n")
	b.WriteString("| Setting `resource_type` to a data source (e.g. `aws_ami`) | Only list-resource types (found in `list_resource_schemas`) are valid |\n")
	b.WriteString("| Omitting a **required** attribute | Required attributes must always be present |\n")
	b.WriteString("| Passing `${var.name}` inside a longer string | Variable injection only works for exact-match values |\n")
	b.WriteString("| Sending a non-positive or non-integer `limit` | Omit `limit` or use a positive integer (UI default is 100) |\n")
	b.WriteString("| Repeating a `single` nesting_mode block type | Only `list` and `set` block types may appear more than once |\n")
	b.WriteString("| Forgetting `generate_config_out` when you need HCL scaffolding | Set `generate_config_out: true` in the top-level attributes |\n\n")
}

// sortedKeys returns a sorted slice of keys from any map[string]T.
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// generateQueryConfigDescription is the MCP tool description seen by the LLM.
const generateQueryConfigDescription = `Generates a No-Code Query Configuration guide for a specific Terraform provider version.

Given a 'list_resource_schemas' JSON object (returned by the provider_list_schema_list tool
or produced by 'terraform providers schema -json'), this tool:

1. Explains the query_configuration structure accepted by execute_query, which calls
   POST /api/v2/search/no-code-query to immediately run a No-Code Search query.
2. Documents the workspace Terraform version pre-condition and how to monitor the
   resulting query run with get_query_status.
3. Catalogs every resource type with its scalar attributes AND block-type attributes
   (name, type, required/optional, nesting_mode, sub-attributes, description)
   so the agent knows exactly which filters can be applied and how to express them.
4. Explains block-type (nested object) attributes and repeatable block semantics
   controlled by nesting_mode (list/set = repeatable; single/map = not).
5. Documents the generate_config_out flag that controls HCL scaffolding output.
6. Provides a ready-to-use example payload pre-filled with placeholder values for
   required scalar and block-type attributes.
7. Documents variable injection syntax (${var.<name>}) and common mistakes.
8. Directs the agent to pass organization_name and workspace_name separately when calling execute_query.

Use this tool before constructing a no-code query payload whenever you have
access to a provider's list_resource_schemas data. The output is self-contained
guidance — the agent should use it to generate a correct, schema-validated query
configuration without needing to consult external documentation.

IMPORTANT: Only attributes present in the schema for the chosen resource type
may appear in 'attributes'. Required attributes must always be included.`
