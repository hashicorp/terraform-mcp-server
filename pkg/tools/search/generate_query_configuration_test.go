// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package search

import (
	"strings"
	"testing"
)

// ── humanType ────────────────────────────────────────────────────────────────

func TestHumanType_Primitives(t *testing.T) {
	cases := []struct {
		raw  any
		want string
	}{
		{nil, "string"},
		{"string", "string"},
		{"bool", "bool"},
		{"number", "number"},
	}
	for _, tc := range cases {
		got := humanType(tc.raw)
		if got != tc.want {
			t.Errorf("humanType(%v) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestHumanType_Collections(t *testing.T) {
	cases := []struct {
		raw  any
		want string
	}{
		// ["list", "string"] → list(string)
		{[]any{"list", "string"}, "list(string)"},
		// ["set", "string"] → set(string)
		{[]any{"set", "string"}, "set(string)"},
		// ["map", "string"] → map(string)
		{[]any{"map", "string"}, "map(string)"},
		// empty array → "any"
		{[]any{}, "any"},
		// single element
		{[]any{"bool"}, "bool"},
	}
	for _, tc := range cases {
		got := humanType(tc.raw)
		if got != tc.want {
			t.Errorf("humanType(%v) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestHumanType_Object(t *testing.T) {
	// ["object", {"region": "string", "count": "number"}]
	// Keys must be sorted: count, region
	raw := []any{"object", map[string]any{"region": "string", "count": "number"}}
	got := humanType(raw)
	want := "object({count: number, region: string})"
	if got != want {
		t.Errorf("humanType(object) = %q, want %q", got, want)
	}
}

func TestHumanType_NestedCollection(t *testing.T) {
	// ["list", ["object", {"key": "string"}]]
	raw := []any{"list", []any{"object", map[string]any{"key": "string"}}}
	got := humanType(raw)
	want := "list(object({key: string}))"
	if got != want {
		t.Errorf("humanType(nested) = %q, want %q", got, want)
	}
}

// ── stripMarkdownLinks ───────────────────────────────────────────────────────

func TestStripMarkdownLinks_NoLinks(t *testing.T) {
	s := "plain description with no links"
	got := stripMarkdownLinks(s)
	if got != s {
		t.Errorf("stripMarkdownLinks(%q) = %q, want identity", s, got)
	}
}

func TestStripMarkdownLinks_SingleLink(t *testing.T) {
	s := "See [AWS docs](https://example.com) for details."
	got := stripMarkdownLinks(s)
	want := "See AWS docs for details."
	if got != want {
		t.Errorf("stripMarkdownLinks(%q) = %q, want %q", s, got, want)
	}
}

func TestStripMarkdownLinks_MultipleLinks(t *testing.T) {
	s := "[first](https://a.com) and [second](https://b.com)"
	got := stripMarkdownLinks(s)
	want := "first and second"
	if got != want {
		t.Errorf("stripMarkdownLinks(%q) = %q, want %q", s, got, want)
	}
}

func TestStripMarkdownLinks_BracketWithoutLink(t *testing.T) {
	// A bare "[" with no matching "](...)" should not panic and should pass through.
	s := "incomplete [ bracket"
	got := stripMarkdownLinks(s)
	if got != s {
		t.Errorf("stripMarkdownLinks(%q) = %q, want identity", s, got)
	}
}

// ── parseListResourceSchemas ─────────────────────────────────────────────────

func TestParseListResourceSchemas_BareMap(t *testing.T) {
	raw := `{"aws_instance": {"block": {"attributes": {"id": {"type": "string", "required": true}}}}}`
	schemas, err := parseListResourceSchemas(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := schemas["aws_instance"]; !ok {
		t.Error("expected aws_instance in schemas")
	}
}

func TestParseListResourceSchemas_WrappedMap(t *testing.T) {
	raw := `{"list_resource_schemas": {"aws_instance": {"block": {"attributes": {}}}}}`
	schemas, err := parseListResourceSchemas(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := schemas["aws_instance"]; !ok {
		t.Error("expected aws_instance in schemas")
	}
}

func TestParseListResourceSchemas_InvalidJSON(t *testing.T) {
	_, err := parseListResourceSchemas("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseListResourceSchemas_EmptyWrappedMap(t *testing.T) {
	raw := `{"list_resource_schemas": null}`
	_, err := parseListResourceSchemas(raw)
	if err == nil {
		t.Error("expected error when list_resource_schemas is null")
	}
}

// ── filterResourceTypes ──────────────────────────────────────────────────────

func TestFilterResourceTypes_AllWhenEmpty(t *testing.T) {
	schemas := map[string]listResourceEntry{
		"aws_instance":  {},
		"aws_s3_bucket": {},
	}
	got := filterResourceTypes(schemas, "")
	if len(got) != 2 {
		t.Errorf("expected 2 resource types, got %d", len(got))
	}
	// Must be sorted.
	if got[0] != "aws_instance" || got[1] != "aws_s3_bucket" {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestFilterResourceTypes_Filtered(t *testing.T) {
	schemas := map[string]listResourceEntry{
		"aws_instance":  {},
		"aws_s3_bucket": {},
		"aws_vpc":       {},
	}
	got := filterResourceTypes(schemas, "aws_vpc, aws_instance")
	if len(got) != 2 {
		t.Errorf("expected 2, got %d: %v", len(got), got)
	}
}

func TestFilterResourceTypes_NoneFound(t *testing.T) {
	schemas := map[string]listResourceEntry{"aws_instance": {}}
	got := filterResourceTypes(schemas, "nonexistent_type")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// ── block_types in catalog and examples ──────────────────────────────────────

func TestWriteResourceCatalog_IncludesBlockTypes(t *testing.T) {
	schemas := map[string]listResourceEntry{
		"aws_instance": {
			Block: listResourceBlock{
				Attributes: map[string]listResourceAttribute{
					"id": {Type: "string", Optional: true, Description: "Instance ID"},
				},
				BlockTypes: map[string]listResourceBlockType{
					"filter": {
						NestingMode: "list",
						Block: listResourceBlock{
							Attributes: map[string]listResourceAttribute{
								"name":   {Type: "string", Required: true},
								"values": {Type: []any{"list", "string"}, Required: true},
							},
						},
					},
				},
			},
		},
	}

	var b strings.Builder
	writeResourceCatalog(&b, schemas, []string{"aws_instance"})
	out := b.String()

	if !strings.Contains(out, "Block-type attributes") {
		t.Error("expected 'Block-type attributes' section in catalog")
	}
	if !strings.Contains(out, "`filter`") {
		t.Error("expected filter block type in catalog")
	}
	if !strings.Contains(out, "list") {
		t.Error("expected nesting_mode 'list' in catalog")
	}
	if !strings.Contains(out, "repeatable") {
		t.Error("expected repeatable note for list nesting_mode")
	}
	if !strings.Contains(out, "`name`") {
		t.Error("expected sub-attribute 'name' in catalog")
	}
}

func TestWriteResourceCatalog_EmptyBlockTypesHiddenSection(t *testing.T) {
	// A resource with only scalar attributes should not emit a block-type section.
	schemas := map[string]listResourceEntry{
		"aws_instance": {
			Block: listResourceBlock{
				Attributes: map[string]listResourceAttribute{
					"id": {Type: "string", Optional: true},
				},
			},
		},
	}

	var b strings.Builder
	writeResourceCatalog(&b, schemas, []string{"aws_instance"})
	out := b.String()

	if strings.Contains(out, "Block-type attributes") {
		t.Error("should not emit block-type section when BlockTypes is empty")
	}
}

func TestBuildExampleAttributes_IncludesRequiredBlockType(t *testing.T) {
	attrs := map[string]listResourceAttribute{}
	blockTypes := map[string]listResourceBlockType{
		"filter": {
			Required:    true,
			NestingMode: "list",
			Block: listResourceBlock{
				Attributes: map[string]listResourceAttribute{
					"name":   {Type: "string", Required: true},
					"values": {Type: []any{"list", "string"}, Required: true},
				},
			},
		},
	}

	result := buildExampleAttributes(attrs, blockTypes)
	if len(result) == 0 {
		t.Fatal("expected at least one entry for required block type")
	}
	entry := result[0]
	if entry["attribute"] != "filter" {
		t.Errorf("expected attribute=filter, got %v", entry["attribute"])
	}
	val, ok := entry["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected block value to be map[string]any, got %T", entry["value"])
	}
	if _, hasName := val["name"]; !hasName {
		t.Error("expected 'name' sub-attribute in block example value")
	}
}

func TestBuildExampleAttributes_SkipsOptionalBlockType(t *testing.T) {
	blockTypes := map[string]listResourceBlockType{
		"filter": {Required: false, NestingMode: "list"},
	}
	result := buildExampleAttributes(nil, blockTypes)
	if len(result) != 0 {
		t.Errorf("expected no entries for optional block type, got %d", len(result))
	}
}

// ── instructions content ─────────────────────────────────────────────────────

func TestWriteInstructions_ContainsPreConditions(t *testing.T) {
	var b strings.Builder
	writeInstructions(&b, "hashicorp", "aws", "6.33.0")
	out := b.String()

	checks := []string{
		"Pre-conditions",
		"1.14.0",
		"latest-query-run",
		"execute_query",
		"get_query_status",
		"get_query_summary",
		"organization_name",
		"workspace_name",
		"generate_config_out",
		"nesting_mode",
		"list` or `set",
		"100",
		"ordinary managed resources are not automatically list resources",
		"never infer or substitute a provider version",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("instructions missing expected content: %q", want)
		}
	}
	if strings.Contains(out, `"relationships"`) {
		t.Error("instructions should not require callers to construct JSON:API relationships")
	}
	if strings.Contains(out, "/api/v2/queries") {
		t.Error("instructions should use get_query_status rather than the raw query API")
	}
}

func TestWriteInstructions_PayloadContainsGenerateConfigOut(t *testing.T) {
	var b strings.Builder
	writeInstructions(&b, "hashicorp", "aws", "6.33.0")
	out := b.String()

	if !strings.Contains(out, "\"generate_config_out\"") {
		t.Error("payload template must include generate_config_out field")
	}
}

// ── example payload ───────────────────────────────────────────────────────────

func TestWriteExamplePayload_ContainsGenerateConfigOut(t *testing.T) {
	schemas := map[string]listResourceEntry{
		"aws_instance": {Block: listResourceBlock{}},
	}
	var b strings.Builder
	writeExamplePayload(&b, schemas, []string{"aws_instance"}, "aws", "hashicorp", "6.33.0")
	out := b.String()

	// generate_config_out is intentionally omitted from the example payload
	// (it defaults to false when absent). The comment explaining this should appear.
	if strings.Contains(out, `"generate_config_out"`) {
		t.Error("example payload must NOT include generate_config_out — it should be omitted to show the default-false behaviour")
	}
	if !strings.Contains(out, "generate_config_out") {
		t.Error("example payload output must still mention generate_config_out (in the comment)")
	}
	if !strings.Contains(out, `"limit": 100`) {
		t.Error("example payload must contain default limit of 100")
	}
	if strings.Contains(out, `"organization_name"`) || strings.Contains(out, `"workspace_name"`) {
		t.Error("example query_configuration must not contain workspace selectors because they are separate tool inputs")
	}
	if strings.Contains(out, `"relationships"`) || strings.Contains(out, `"data"`) {
		t.Error("example query_configuration must not contain the JSON:API envelope")
	}
}

// ── variable notes ────────────────────────────────────────────────────────────

func TestWriteVariableNotes_ContainsMistakeRows(t *testing.T) {
	var b strings.Builder
	writeVariableNotes(&b)
	out := b.String()

	checks := []string{
		"generate_config_out",
		"nesting_mode",
		"UI default is 100",
		"assuming an S3 bucket can use `aws_s3_bucket`",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("variable notes missing expected content: %q", want)
		}
	}
}

// ── sortedKeys ────────────────────────────────────────────────────────────────

func TestSortedKeys_Deterministic(t *testing.T) {
	m := map[string]int{"c": 3, "a": 1, "b": 2}
	got := sortedKeys(m)
	want := []string{"a", "b", "c"}
	for i, k := range got {
		if k != want[i] {
			t.Errorf("sortedKeys index %d = %q, want %q", i, k, want[i])
		}
	}
}

func TestSortedKeys_Empty(t *testing.T) {
	m := map[string]int{}
	got := sortedKeys(m)
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}
