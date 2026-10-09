// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var identityVersionProviders = map[string]workspaceProvider{
	"aws_iam_role": {Source: "registry.terraform.io/hashicorp/aws", Name: "aws", Version: "6.62.0"},
}

// Search identity_version is required evidence on every found-resource record.
// Zero is valid; anything that is not a nonnegative integer literal that fits an
// int makes the whole QueryRun invalid.
func TestParseImportDiscoveryRequiresValidIdentityVersion(t *testing.T) {
	data := string(phase0Fixture(t, "query.ndjson"))
	const original = `"identity_version":0,`
	require.Contains(t, data, original)

	invalid := map[string]string{
		"missing":          "",
		"null":             `"identity_version":null,`,
		"negative":         `"identity_version":-1,`,
		"negative zero":    `"identity_version":-0,`,
		"fraction":         `"identity_version":1.5,`,
		"integral float":   `"identity_version":1.0,`,
		"exponent":         `"identity_version":1e0,`,
		"string":           `"identity_version":"0",`,
		"boolean":          `"identity_version":true,`,
		"object":           `"identity_version":{},`,
		"array":            `"identity_version":[0],`,
		"out of range":     `"identity_version":9223372036854775808,`,
		"far out of range": `"identity_version":123456789012345678901234567890,`,
	}
	for name, replacement := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			items, err := parseImportDiscovery([]byte(strings.Replace(data, original, replacement, 1)), "qry-fixture", identityVersionProviders)
			require.Error(t, err)
			assert.Equal(t, "query_identity_version_invalid", importDiagnosticCode(err))
			assert.Nil(t, items, "no partial candidate list")
		})
	}

	var baseID string
	for _, version := range []int{0, 1, 7, 2147483647} {
		t.Run(fmt.Sprintf("valid %d", version), func(t *testing.T) {
			items, err := parseImportDiscovery([]byte(strings.Replace(data, original, fmt.Sprintf(`"identity_version":%d,`, version), 1)), "qry-fixture", identityVersionProviders)
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.NotNil(t, items[0].IdentityVersion)
			assert.Equal(t, version, *items[0].IdentityVersion)
			if baseID == "" {
				baseID = items[0].CandidateID
			}
			assert.Equal(t, baseID, items[0].CandidateID, "candidate_id does not include the identity version")
		})
	}
}

// One bad record among 100 makes the whole QueryRun non-selectable, wherever it is.
func TestParseImportDiscoveryOneInvalidVersionInvalidatesTheWholeQueryRun(t *testing.T) {
	good := string(largeDiscoveryLog(100))
	require.Equal(t, 100, strings.Count(good, `"identity_version":0,`))
	items, err := parseImportDiscovery([]byte(good), "qry-large", identityVersionProviders)
	require.NoError(t, err)
	require.Len(t, items, 100)

	lines := strings.Split(good, "\n")
	for _, idx := range []int{0, 57, 99} {
		t.Run(fmt.Sprintf("row %d", idx), func(t *testing.T) {
			bad := append([]string(nil), lines...)
			bad[idx] = strings.Replace(bad[idx], `"identity_version":0,`, "", 1)
			items, err := parseImportDiscovery([]byte(strings.Join(bad, "\n")), "qry-large", identityVersionProviders)
			require.Error(t, err)
			assert.Equal(t, "query_identity_version_invalid", importDiagnosticCode(err))
			assert.Empty(t, items)
		})
	}
}

func TestSelectionRejectsACandidateWithoutAVersion(t *testing.T) {
	for name, version := range map[string]*int{"absent": nil, "negative": intPtr(-1)} {
		d := &importDiscovery{WorkspaceID: "ws", Candidates: []importDiscoveryCandidate{{CandidateID: "c1", Identity: map[string]any{"k": "v"}, IdentityVersion: version}}}
		_, err := selectImportCandidates(d, []importSelection{{CandidateID: "c1", ManagedType: "t"}}, "ws")
		require.Error(t, err, name)
		assert.Equal(t, "query_identity_version_invalid", importDiagnosticCode(err), name)
	}
	d := &importDiscovery{WorkspaceID: "ws", Candidates: []importDiscoveryCandidate{{CandidateID: "c1", Identity: map[string]any{"k": "v"}, IdentityVersion: intPtr(0)}}}
	got, err := selectImportCandidates(d, []importSelection{{CandidateID: "c1", ManagedType: "t"}}, "ws")
	require.NoError(t, err)
	assert.Len(t, got, 1, "version zero is valid")
}

func TestInvalidIdentityVersionBlocksPrepareAndPaging(t *testing.T) {
	f := importBackendFixture(t)
	f.queryLog = []byte(strings.Replace(string(f.queryLog), `"identity_version":0,`, "", 1))

	_, err := ReadDiscoveryPage(context.Background(), f.client, "qry-fixture", DiscoveryFilter{})
	require.Error(t, err)
	assert.Equal(t, "query_identity_version_invalid", importDiagnosticCode(err), "paging exposes no selectable rows")

	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	assert.Equal(t, "blocked", out.Status)
	assert.Equal(t, "query_selection", out.Stage)
	assert.Contains(t, out.Diagnostics, "query_identity_version_invalid")
	assert.Nil(t, out.Carry, "no carry from a non-selectable QueryRun")
	assert.Empty(t, out.Candidates)
	assert.Equal(t, "8", out.ContractVersion)
	assert.Contains(t, out.NextAction, "whole QueryRun is not selectable")
	assert.Contains(t, out.NextAction, importNothingCreated)
	assert.False(t, forbiddenNextActionWords.MatchString(out.NextAction), out.NextAction)

	result, rerr := importToolResult(out)
	require.NoError(t, rerr)
	assert.True(t, result.IsError, "a blocked prepare is an execution error")
}

// The serialized text and structured results, and the output schema, never
// carry a combined verdict, and plan findings never make the result an error.
func TestVerifyResultVariantsSerializeWithoutOverall(t *testing.T) {
	schema := VerifyImportPlanDefinition().OutputSchema
	assert.Equal(t, "object", schema.Type)
	assert.NotContains(t, schema.Properties, "overall")
	for _, p := range []string{"status", "changes", "object_identity", "identity_unsupported", "attention", "unselected", "plan", "next_action", "contract_version"} {
		assert.Contains(t, schema.Properties, p)
	}
	assert.NotContains(t, schema.Required, "overall")
	assert.Contains(t, schema.Required, "status")
	assert.Contains(t, schema.Required, "next_action")
	assert.Contains(t, VerifyImportPlanDefinition().Description, "no overall or combined verdict")
	schemaJSON, err := json.Marshal(schema)
	require.NoError(t, err)
	var schemaDoc any
	require.NoError(t, json.Unmarshal(schemaJSON, &schemaDoc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("verify-import-plan.json", schemaDoc))
	validator, err := compiler.Compile("verify-import-plan.json")
	require.NoError(t, err)
	assertResult := func(t *testing.T, out importVerified) {
		t.Helper()
		result, err := importToolResult(out)
		require.NoError(t, err)
		text, ok := mcp.AsTextContent(result.Content[0])
		require.True(t, ok)
		var wire any
		require.NoError(t, json.Unmarshal([]byte(text.Text), &wire))
		require.NoError(t, validator.Validate(wire), "tool result must conform to its advertised output schema")
		assert.JSONEq(t, text.Text, stringMustMarshal(t, result.StructuredContent))
	}
	for _, status := range []string{"pending", "failed", "carry_required", "blocked"} {
		t.Run("response schema "+status, func(t *testing.T) {
			assertResult(t, importVerified{ContractVersion: importToolContractVersion, Status: status, Diagnostics: []string{}, NextAction: "Review the response."})
		})
	}

	versionsFor := func(n, v int) map[string]int {
		m := map[string]int{}
		for i := 0; i < n; i++ {
			m[fmt.Sprintf("aws_iam_role.r%d", i)] = v
		}
		return m
	}
	for _, n := range []int{1, 100} {
		for _, tc := range []struct {
			name       string
			support    string
			build      func(f verifyFixture) []byte
			prefix     string
			contains   []string
			notContain []string
		}{
			{
				name:    "all matched",
				support: importIdentitySupported,
				build:   func(f verifyFixture) []byte { return planJSON("1.16.1", f.cleanEntries(true), "") },
				contains: []string{"identities matched the carried selection", importClosingRule,
					"no changes and no other actions"},
				notContain: []string{"Identity uncertainty", importConfidenceReportRule},
			},
			{
				name:    "versions differ",
				support: importIdentitySupported,
				build: func(f verifyFixture) []byte {
					return planWithIdentityVersions(f, f.cleanEntries(true), versionsFor(n, 1), false)
				},
				prefix:     fmt.Sprintf("%d selected identities are unverified because the Search and plan identity schema versions differ", n),
				contains:   []string{importConfidenceReportRule, "no changes and no other actions", "Identity uncertainty alone"},
				notContain: []string{"Fix the configuration", "adjust the configuration", importClosingRule},
			},
			{
				name:    "plan version not reported",
				support: importIdentitySupported,
				build: func(f verifyFixture) []byte {
					return planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{}, false)
				},
				prefix:     fmt.Sprintf("%d selected identities are unverified because the plan did not report an identity schema version", n),
				contains:   []string{reasonIdentityVersionNotCompared, importConfidenceReportRule, "Identity uncertainty alone"},
				notContain: []string{"Fix the configuration", importClosingRule, "unsupported"},
			},
			{
				name:       "identity unsupported",
				support:    importIdentityNone,
				build:      func(f verifyFixture) []byte { return planJSON("1.16.1", f.cleanEntries(false), "") },
				prefix:     fmt.Sprintf("Identity could not be checked for %d items", n),
				contains:   []string{importConfidenceReportRule, "no changes and no other actions", "prepare-time target schema lacked identity", "does not establish the speculative plan's current schema"},
				notContain: []string{"Fix the configuration"},
			},
			{
				name:    "identity mismatch",
				support: importIdentitySupported,
				build: func(f verifyFixture) []byte {
					e := f.cleanEntries(true)
					e[0].afterIdentity = `{"account_id":"123456789012","name":"someone-else"}`
					return planJSON("1.16.1", e, "")
				},
				prefix:     "1 selected items have an identity that differs",
				contains:   []string{"Stop and review this selection with the user", importConfidenceReportRule},
				notContain: []string{importClosingRule},
			},
			{
				name:    "updates with uncertain identity",
				support: importIdentitySupported,
				build: func(f verifyFixture) []byte {
					e := f.cleanEntries(true)
					for i := range e {
						e[i].actions, e[i].after = []string{"update"}, `{"name":"x","path":"/y/"}`
					}
					return planWithIdentityVersions(f, e, versionsFor(n, 1), false)
				},
				prefix:   fmt.Sprintf("%d selected identities are unverified because the Search and plan identity schema versions differ", n),
				contains: []string{fmt.Sprintf("%d selected items with updates or replacements", n), "adjust the configuration", "Identity uncertainty alone"},
			},
			{
				name:    "selected imports not taking effect",
				support: importIdentitySupported,
				build: func(f verifyFixture) []byte {
					e := f.cleanEntries(true)
					for i := range e {
						e[i].actions = []string{"create"}
					}
					return planJSON("1.16.1", e, `,"deferred_changes":[{}],"resource_drift":[{"address":"aws_iam_role.d","change":{"actions":["update"]}}]`)
				},
				contains: []string{fmt.Sprintf("%d of %d selected imports not taking effect", n, n), "1 deferred changes", "refresh drift 1", "Fix the configuration listed in attention"},
			},
		} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, n), func(t *testing.T) {
				f := newVerifyFixture(t, n, tc.support, "1.16.1")
				out := verifyFacts(t, f, tc.build(f))

				if tc.prefix != "" {
					assert.True(t, strings.HasPrefix(out.NextAction, tc.prefix), out.NextAction)
				}
				for _, want := range tc.contains {
					assert.Contains(t, out.NextAction, want)
				}
				for _, unwanted := range tc.notContain {
					assert.NotContains(t, out.NextAction, unwanted)
				}
				assert.Equal(t, n, out.Selected)
				assert.LessOrEqual(t, len(out.Attention), maxVerifyAttention)

				result, err := importToolResult(out)
				require.NoError(t, err)
				assert.False(t, result.IsError, "plan findings and identity uncertainty are not execution errors")
				text, ok := mcp.AsTextContent(result.Content[0])
				require.True(t, ok)
				var wire map[string]any
				require.NoError(t, json.Unmarshal([]byte(text.Text), &wire))
				require.NoError(t, validator.Validate(wire), "tool result must conform to its advertised output schema")
				assert.NotContains(t, wire, "overall")
				assert.Equal(t, "plan_available", wire["status"])
				assert.Equal(t, importToolContractVersion, wire["contract_version"])
				assert.Equal(t, out.NextAction, wire["next_action"])
				for _, key := range []string{"changes", "object_identity", "unselected", "plan"} {
					assert.Contains(t, wire, key, "independent axes and counts are always present")
				}
				structured, err := json.Marshal(result.StructuredContent)
				require.NoError(t, err)
				assert.JSONEq(t, text.Text, string(structured), "text and structured content agree")
				for key := range wire {
					assert.Contains(t, schema.Properties, key, "serialized field %s is described by the output schema", key)
				}
				assert.Less(t, len(text.Text), 64*1024)
				if n == 100 {
					envelope, err := json.Marshal(result)
					require.NoError(t, err)
					t.Logf("100 selections, %s: JSON text %d bytes, tool result with structured and text copies %d bytes", tc.name, len(text.Text), len(envelope))
				}
			})
		}
	}
}

func stringMustMarshal(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func TestVerifySelectedDriftAndOverlappingUnselectedActions(t *testing.T) {
	f := newVerifyFixture(t, 1, importIdentitySupported, "1.16.1")
	other := planEntry{address: "aws_iam_role.other", actions: []string{"update"}}
	entries := append(f.cleanEntries(true), other)
	plan := planJSON("1.16.1", entries, `,"resource_drift":[{"address":"aws_iam_role.r0","change":{"actions":["update"]}}]`)
	out := verifyFacts(t, f, plan)
	require.NotNil(t, out.Unselected)
	assert.Equal(t, 1, out.Unselected.ExtraImports)
	assert.Equal(t, 1, out.Unselected.OtherManagedAction)
	assert.Equal(t, 1, out.Unselected.Drift)
	assert.Contains(t, out.NextAction, "extra imports 1, other managed actions 1 outside the selection (counts may overlap)")
	assert.Contains(t, out.NextAction, "refresh drift 1 (may include selected addresses)")
	assert.NotContains(t, out.NextAction, "3 other actions outside the selection")
	result, err := importToolResult(out)
	require.NoError(t, err)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	assert.Contains(t, text.Text, "may include selected addresses")
}
