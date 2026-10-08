// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func schemaFromJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func sqsCandidates(version *int, identity map[string]any) []importDiscoveryCandidate {
	return []importDiscoveryCandidate{{IdentityVersion: version, Identity: identity}}
}

// The identity schemas below are copied from terraform providers schema -json
// for hashicorp/aws 6.18.0 (version 0) and 6.19.0, 6.27.0 and 6.66.0 (version 1).
const (
	sqsIdentityV0 = `{"version":0,"attributes":{"account_id":{"type":"string","optional_for_import":true},"region":{"type":"string","optional_for_import":true},"url":{"type":"string","required_for_import":true}}}`
	sqsIdentityV1 = `{"version":1,"attributes":{"url":{"type":"string","required_for_import":true}}}`
)

func TestIdentityCompatSQSAcrossProviderReleases(t *testing.T) {
	queue := "https://sqs.us-west-2.amazonaws.com/123456789012/q"

	t.Run("6.66.0 search into a 6.27.0 target: same identity schema", func(t *testing.T) {
		got := importIdentityCompat(importIdentitySupported, schemaFromJSON(t, sqsIdentityV1), sqsCandidates(intPtr(1), map[string]any{"url": queue}))
		assert.Equal(t, identityCompatSameVersion, got.Status)
		assert.Equal(t, 1, *got.SearchIdentityVersion)
		assert.Equal(t, 1, *got.TargetIdentityVersion)
		assert.Empty(t, got.MissingRequiredKeys)
		assert.Empty(t, got.UnknownKeys)
	})

	t.Run("6.66.0 search into a 6.18.0 target: keys fit, versions differ, scope keys absent", func(t *testing.T) {
		got := importIdentityCompat(importIdentitySupported, schemaFromJSON(t, sqsIdentityV0), sqsCandidates(intPtr(1), map[string]any{"url": queue}))
		assert.Equal(t, identityCompatVersionDiffers, got.Status)
		assert.Equal(t, []string{"account_id", "region"}, got.AbsentOptionalKeys)
		assert.Contains(t, got.Guidance, "differ")
		assert.Contains(t, got.Guidance, identityCompatGuidanceOptionalScope)
	})

	t.Run("a 6.18.0-shaped search identity into a 6.19.0 target: unknown keys", func(t *testing.T) {
		got := importIdentityCompat(importIdentitySupported, schemaFromJSON(t, sqsIdentityV1), sqsCandidates(intPtr(0), map[string]any{"url": queue, "account_id": "123456789012", "region": "us-west-2"}))
		assert.Equal(t, identityCompatShapeDiffers, got.Status)
		assert.Equal(t, []string{"account_id", "region"}, got.UnknownKeys)
		assert.Empty(t, got.MissingRequiredKeys)
		assert.Equal(t, 0, *got.SearchIdentityVersion, "version 0 is a real version")
		assert.Equal(t, 1, *got.TargetIdentityVersion)
	})
}

func TestIdentityCompatCases(t *testing.T) {
	v0 := `{"version":0,"attributes":{"account_id":{"type":"string","optional_for_import":true},"name":{"type":"string","required_for_import":true}}}`
	both := map[string]any{"account_id": "123456789012", "name": "r"}
	for _, tt := range []struct {
		name       string
		support    string
		target     map[string]any
		candidates []importDiscoveryCandidate
		status     string
	}{
		{"same version zero", importIdentitySupported, schemaFromJSON(t, v0), sqsCandidates(intPtr(0), both), identityCompatSameVersion},
		{"search version absent", importIdentitySupported, schemaFromJSON(t, v0), sqsCandidates(nil, both), identityCompatVersionUnknown},
		{"target version absent", importIdentitySupported, schemaFromJSON(t, `{"attributes":{"name":{"required_for_import":true}}}`), sqsCandidates(intPtr(0), map[string]any{"name": "r"}), identityCompatVersionUnknown},
		{"required key missing", importIdentitySupported, schemaFromJSON(t, v0), sqsCandidates(intPtr(0), map[string]any{"account_id": "1"}), identityCompatShapeDiffers},
		{"target has no identity", importIdentityNone, nil, sqsCandidates(intPtr(0), both), identityCompatNoTarget},
		{"target not read", importIdentityUnknown, nil, sqsCandidates(intPtr(0), both), identityCompatTargetNotRead},
		{"supported but schema missing", importIdentitySupported, nil, sqsCandidates(intPtr(0), both), identityCompatTargetNotRead},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := importIdentityCompat(tt.support, tt.target, tt.candidates)
			assert.Equal(t, tt.status, got.Status)
			assert.NotEmpty(t, got.Guidance)
		})
	}

	t.Run("missing required key is named", func(t *testing.T) {
		got := importIdentityCompat(importIdentitySupported, schemaFromJSON(t, v0), sqsCandidates(intPtr(0), map[string]any{"account_id": "1"}))
		assert.Equal(t, []string{"name"}, got.MissingRequiredKeys)
	})

	t.Run("candidates that disagree on version leave it unknown", func(t *testing.T) {
		c := append(sqsCandidates(intPtr(0), both), sqsCandidates(intPtr(1), both)...)
		assert.Equal(t, identityCompatVersionUnknown, importIdentityCompat(importIdentitySupported, schemaFromJSON(t, v0), c).Status)
	})
}

func TestPrepareImportReportsIdentityCompatibilityAndCarriesSearchVersion(t *testing.T) {
	f := importBackendFixture(t)
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)

	require.Len(t, out.Types, 1)
	compat := out.Types[0].IdentityCompatibility
	require.NotNil(t, compat)
	assert.Equal(t, identityCompatSameVersion, compat.Status)
	assert.Equal(t, 0, *compat.SearchIdentityVersion)
	assert.Equal(t, 0, *compat.TargetIdentityVersion)
	assert.NotEmpty(t, compat.Guidance)

	// The version rides beside candidate_id, which is unchanged.
	c := out.Carry.Candidates[0]
	require.NotNil(t, c.SearchIdentityVersion)
	assert.Equal(t, 0, *c.SearchIdentityVersion)
	p := out.Carry.Providers[c.ListType]
	assert.Equal(t, c.CandidateID, importCandidateID("qry-fixture", workspaceProvider{Source: p.Source, Version: p.Version}, c.ListType, c.Identity))
	assert.Equal(t, "7", out.ContractVersion)
	assert.Equal(t, "7", importToolContractVersion)

	// The carried version is covered by the digest.
	changed := *out.Carry
	changed.Candidates = append([]importCarryCandidate(nil), out.Carry.Candidates...)
	changed.Candidates[0].SearchIdentityVersion = intPtr(5)
	assert.NotEqual(t, out.Carry.SelectionDigest, importCarryDigest(changed))

	encoded, _ := json.Marshal(out)
	assert.Contains(t, string(encoded), `"identity_compatibility"`)
	assert.Contains(t, string(encoded), `"search_identity_version":0`)
}

func TestPrepareImportTargetWithoutIdentityIsReported(t *testing.T) {
	f := importBackendFixture(t)
	schema := schemaFromJSON(t, string(f.responses["/schema-download"]))
	for _, p := range schema["provider_schemas"].(map[string]any) {
		delete(p.(map[string]any), "resource_identity_schemas")
	}
	encoded, _ := json.Marshal(schema)
	f.responses["/schema-download"] = encoded
	out := prepareImportTool(context.Background(), f.client, importFixtureInput(t))
	require.Equal(t, "prepared", out.Status, out.Diagnostics)
	// A 1.12 or later schema without an identity entry means the type has none.
	if out.Types[0].IdentitySupport == importIdentityNone {
		assert.Equal(t, identityCompatNoTarget, out.Types[0].IdentityCompatibility.Status)
	} else {
		assert.Equal(t, identityCompatTargetNotRead, out.Types[0].IdentityCompatibility.Status)
	}
}

func TestIdentityCompatibilityTextKeepsTheNarrowIDException(t *testing.T) {
	joined := strings.Join(importToolInstructions, " ")
	assert.Contains(t, joined, importIdentityCompatibilityRule)
	assert.Contains(t, importIdentityCompatibilityRule, "not provider releases")
	assert.Contains(t, importKeepGeneratedRule, "a difference in provider release or identity version alone is not a reason to switch")
	assert.Contains(t, importKeepGeneratedRule, "gives an id import form")
	assert.Contains(t, importKeepGeneratedRule, "Do not replace an identity import with an id import unless identity_support")
	assert.Contains(t, importKeepGeneratedShort, "unless identity_compatibility shows the identity does not fit")
	assert.Contains(t, PrepareImportDefinition().Description, "identity_compatibility")
	for _, g := range []string{identityCompatGuidanceSame, identityCompatGuidanceVersionDiffers, identityCompatGuidanceVersionUnknown, identityCompatGuidanceShape, identityCompatGuidanceNoTarget, identityCompatGuidanceNotRead, identityCompatGuidanceOptionalScope} {
		assert.False(t, forbiddenNextActionWords.MatchString(g), g)
	}
	assert.Contains(t, identityCompatGuidanceShape, "Do not guess a key mapping")
}

// planWithIdentityVersions returns a plan whose planned_values carry identity
// schema versions, one per address, with module addresses nested.
func planWithIdentityVersions(f verifyFixture, entries []planEntry, versions map[string]int, nested bool) []byte {
	var root, child []string
	for addr, v := range versions {
		r := fmt.Sprintf(`{"address":%q,"identity_schema_version":%d}`, addr, v)
		if nested && strings.HasPrefix(addr, "module.") {
			child = append(child, r)
		} else {
			root = append(root, r)
		}
	}
	pv := fmt.Sprintf(`"planned_values":{"root_module":{"resources":[%s],"child_modules":[{"address":"module.m","resources":[%s]}]}}`, strings.Join(root, ","), strings.Join(child, ","))
	return planJSON("1.16.1", entries, ","+pv)
}

func withSearchVersion(f verifyFixture, v *int) verifyFixture {
	for i := range f.carry.Candidates {
		f.carry.Candidates[i].SearchIdentityVersion = v
	}
	f.carry.SelectionDigest = importCarryDigest(*f.carry)
	return f
}

func TestVerifyIdentityVersionComparison(t *testing.T) {
	t.Run("same version, including zero, is compared and matched", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
		out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{"aws_iam_role.r0": 0}, false))
		assert.Equal(t, 1, out.ObjectIdentity[identityMatched])
		assert.Equal(t, "no_unintended_changes", out.Overall)
		assert.Empty(t, out.Attention)
	})

	t.Run("versions differ: equal values are unverified, not matched", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 2, importIdentitySupported, "1.16.1"), intPtr(0))
		out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{"aws_iam_role.r0": 1, "aws_iam_role.r1": 1}, false))
		assert.Equal(t, 0, out.ObjectIdentity[identityMatched])
		assert.Equal(t, 2, out.ObjectIdentity[identityUnverified])
		require.Len(t, out.Attention, 2)
		assert.Equal(t, reasonIdentityVersionDiffers, out.Attention[0].IdentityReason)
		assert.Equal(t, "needs_iteration", out.Overall)
		assert.Contains(t, out.NextAction, "2 selected identities are unverified because the Search and plan identity schema versions differ")
		assert.False(t, forbiddenNextActionWords.MatchString(out.NextAction), out.NextAction)
	})

	t.Run("plan version zero against a search version one differs", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(1))
		out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{"aws_iam_role.r0": 0}, false))
		assert.Equal(t, 1, out.ObjectIdentity[identityUnverified])
	})

	t.Run("versions differ and values differ is still unverified, not mismatched", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
		e := f.cleanEntries(true)
		e[0].afterIdentity = `{"account_id":"123456789012","name":"other"}`
		out := verifyFacts(t, f, planWithIdentityVersions(f, e, map[string]int{"aws_iam_role.r0": 1}, false))
		assert.Equal(t, 1, out.ObjectIdentity[identityUnverified])
		assert.Equal(t, 0, out.ObjectIdentity[identityMismatched])
	})

	t.Run("a missing version is noted on a match, never silently compared", func(t *testing.T) {
		for name, tc := range map[string]struct {
			search   *int
			versions map[string]int
		}{
			"search version absent": {nil, map[string]int{"aws_iam_role.r0": 0}},
			"plan version absent":   {intPtr(0), map[string]int{}},
		} {
			f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), tc.search)
			out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), tc.versions, false))
			assert.Equal(t, 1, out.ObjectIdentity[identityMatched], name)
			// matched items are counted, not listed; check through the classifier
			_, reason := classifyIdentity("1.16.1", f.carry.Candidates[0], importIdentitySupported, json.RawMessage(`{"account_id":"123456789012","name":"role-0"}`), nil)
			assert.Contains(t, reason, suffixVersionNotCompared, name)
		}
	})

	t.Run("a child module address is found", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
		f.bindings[0].TargetAddress = `module.m.aws_iam_role.r0`
		f.byCand[f.bindings[0].CandidateID] = f.bindings[0].TargetAddress
		out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{`module.m.aws_iam_role.r0`: 1}, true))
		assert.Equal(t, 1, out.ObjectIdentity[identityUnverified])
		require.Len(t, out.Attention, 1)
		assert.Equal(t, reasonIdentityVersionDiffers, out.Attention[0].IdentityReason)
	})

	t.Run("no after_identity and a type with no identity stays unsupported", func(t *testing.T) {
		f := withSearchVersion(newVerifyFixture(t, 1, importIdentityNone, "1.16.1"), intPtr(0))
		out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(false), map[string]int{}, false))
		assert.Equal(t, 1, out.ObjectIdentity[identityUnsupported])
		assert.Equal(t, 0, out.ObjectIdentity[identityMatched])
	})
}

func TestVerifyAcceptsCarryWithSearchVersionAndKeepsCandidateIDs(t *testing.T) {
	f := withSearchVersion(newVerifyFixture(t, 2, importIdentitySupported, "1.16.1"), intPtr(1))
	byCand, err := validateImportCarry(f.carry, f.bindings)
	require.NoError(t, err)
	assert.Len(t, byCand, 2)

	// Removing the version from a carry that had one breaks the digest.
	tampered := *f.carry
	tampered.Candidates = append([]importCarryCandidate(nil), f.carry.Candidates...)
	tampered.Candidates[0].SearchIdentityVersion = nil
	_, err = validateImportCarry(&tampered, f.bindings)
	assert.ErrorContains(t, err, "carry_digest_mismatch")

	// A carry without a version, as an older caller built it, still validates.
	plain := newVerifyFixture(t, 2, importIdentitySupported, "1.16.1")
	_, err = validateImportCarry(plain.carry, plain.bindings)
	assert.NoError(t, err)
}

func TestVerifyNextActionLeadsWithUnverifiedIdentityAndDoesNotAskForIteration(t *testing.T) {
	f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
	out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{"aws_iam_role.r0": 1}, false))
	require.Equal(t, 1, out.ObjectIdentity[identityUnverified])
	next := out.NextAction

	assert.True(t, strings.HasPrefix(next, "1 selected identities are unverified because the Search and plan identity schema versions differ (both known, unequal)"), next)
	assert.Contains(t, next, "even if the values look equal and the plan only imports")
	assert.Contains(t, next, "An import-only plan says what Terraform proposes")
	assert.Contains(t, next, "the only open item is identity")
	assert.Contains(t, next, importConfidenceReportRule)
	// Nothing needs iterating: do not send the agent to rewrite a block.
	assert.NotContains(t, next, "adjust the configuration")
	assert.NotContains(t, next, "create a new configuration version")
	assert.False(t, forbiddenNextActionWords.MatchString(next), next)
}

func TestVerifyNextActionStillAsksForIterationWhenThePlanHasOtherChanges(t *testing.T) {
	f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
	e := f.cleanEntries(true)
	e[0].actions = []string{"update"}
	out := verifyFacts(t, f, planWithIdentityVersions(f, e, map[string]int{"aws_iam_role.r0": 1}, false))
	assert.True(t, strings.HasPrefix(out.NextAction, "1 selected identities are unverified"), out.NextAction)
	assert.Contains(t, out.NextAction, "adjust the configuration")
	assert.NotContains(t, out.NextAction, "the only open item is identity")
}

func TestVerifyNextActionHasNoIdentityLeadWhenMatched(t *testing.T) {
	f := withSearchVersion(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), intPtr(0))
	out := verifyFacts(t, f, planWithIdentityVersions(f, f.cleanEntries(true), map[string]int{"aws_iam_role.r0": 0}, false))
	assert.NotContains(t, out.NextAction, "unverified")
	assert.NotContains(t, out.NextAction, importConfidenceReportRule)
}

func TestPreparedNextActionNamesTypesWhoseIdentityFitIsNotConfirmed(t *testing.T) {
	typ := func(name, status string) importPreparedType {
		return importPreparedType{ManagedType: name, IdentityCompatibility: &importIdentityCompatibility{Status: status}}
	}
	assert.Empty(t, identityCompatNextAction(nil))
	assert.Empty(t, identityCompatNextAction([]importPreparedType{typ("aws_sqs_queue", identityCompatSameVersion)}))

	for _, status := range []string{identityCompatVersionDiffers, identityCompatVersionUnknown, identityCompatShapeDiffers, identityCompatNoTarget, identityCompatTargetNotRead} {
		out := &importPrepared{HasCurrentConfiguration: true, Types: []importPreparedType{typ("aws_sqs_queue", status), typ("aws_iam_role", identityCompatSameVersion), typ("aws_sqs_queue", status)}}
		next := importPreparedNextAction(out, false, false)
		assert.True(t, strings.HasPrefix(next, importKeepGeneratedShort), status)
		assert.Contains(t, next, "Identity compatibility is not confirmed for aws_sqs_queue (types[].identity_compatibility); tell the user before authoring.", status)
		assert.NotContains(t, next, "aws_iam_role", status)
		assert.Contains(t, next, importIdentityUnverifiedLead, status)
		assert.Equal(t, 1, strings.Count(next, importIdentityUnverifiedLead), status)
		assert.False(t, forbiddenNextActionWords.MatchString(identityCompatNextAction(out.Types)), status)
	}

	// The new-workspace branch gets it too, after the keep-generated rule.
	blank := &importPrepared{Types: []importPreparedType{typ("aws_sqs_queue", identityCompatVersionDiffers)}}
	assert.Contains(t, importPreparedNextAction(blank, false, false), importIdentityUnverifiedLead)
}

func TestIdentityTextStatesUnverifiedAndAvoidsVerdictWords(t *testing.T) {
	for _, s := range []string{importIdentityUnverifiedLead, importConfidenceReportRule, identityCompatGuidanceVersionDiffers, identityCompatGuidanceVersionUnknown, identityCompatGuidanceOptionalScope} {
		assert.False(t, forbiddenNextActionWords.MatchString(s), s)
	}
	assert.Contains(t, importIdentityUnverifiedLead, "even if Terraform plans only the import")
	assert.Contains(t, identityCompatGuidanceVersionDiffers, "Identity will stay unverified")
	assert.Contains(t, identityCompatGuidanceVersionDiffers, "may predate the plan")
	assert.Contains(t, identityCompatGuidanceOptionalScope, "from its provider configuration or from the remote API")
	assert.Contains(t, importConfidenceReportRule, "confident")
	assert.Contains(t, importConfidenceReportRule, "unsure")
	assert.Contains(t, importConfidenceReportRule, "conflicting")
	assert.Contains(t, strings.Join(importToolInstructions, " "), importConfidenceReportRule)
	assert.Contains(t, importIdentityCompatibilityRule, importIdentityUnverifiedLead)
	assert.Contains(t, importKeepGeneratedRule, "A difference in identity schema version alone is neither a reason to change the block")
	desc := VerifyImportPlanDefinition().Description
	assert.Contains(t, desc, "both available and unequal")
	assert.Contains(t, desc, "an import-only plan does not confirm identity")
	assert.Contains(t, PrepareImportDefinition().Description, "identity stays unverified even if the plan only imports")
}
