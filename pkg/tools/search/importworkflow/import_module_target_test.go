// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bindAddresses points each fixture binding at the given absolute address.
func bindAddresses(f verifyFixture, addrs ...string) verifyFixture {
	for i, a := range addrs {
		f.bindings[i].TargetAddress = a
		f.byCand[f.bindings[i].CandidateID] = a
	}
	return f
}

func TestVerifyMatchesModuleAndIndexedAddressesExactly(t *testing.T) {
	addrs := []string{
		"aws_iam_role.root",
		"module.network.aws_iam_role.r",
		"module.a.module.b.aws_iam_role.r",
		`module.network["east"].aws_iam_role.r`,
		`aws_iam_role.set["a"]`,
		`module.m[0].aws_iam_role.set["k"]`,
	}
	f := bindAddresses(newVerifyFixture(t, len(addrs), importIdentitySupported, "1.16.1"), addrs...)
	out := verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(true), ""))
	assert.Equal(t, len(addrs), out.Changes[changeNone])
	assert.Equal(t, len(addrs), out.ObjectIdentity[identityMatched])
	assert.Zero(t, out.Unselected.ExtraImports)
}

func TestVerifySameNameInAnotherModuleIsNotConflated(t *testing.T) {
	f := bindAddresses(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), "module.a.aws_iam_role.r")
	entries := f.cleanEntries(true)
	// The import landed in module.b instead of the bound module.a.
	entries[0].address = "module.b.aws_iam_role.r"
	out := verifyFacts(t, f, planJSON("1.16.1", entries, ""))
	assert.Equal(t, 1, out.Changes[changeNotInPlan])
	assert.Equal(t, 0, out.ObjectIdentity[identityMatched])
	assert.Equal(t, 1, out.Unselected.ExtraImports, "the import at the other address is reported, not accepted")
	assert.Contains(t, out.NextAction, "no plan entry has exactly that target_address")
}

func TestVerifyWrongOrDifferentlyQuotedKeyIsNotInPlan(t *testing.T) {
	for name, bound := range map[string]string{
		"other key":    `module.network["west"].aws_iam_role.r`,
		"single quote": `module.network['east'].aws_iam_role.r`,
		"unindexed":    `module.network.aws_iam_role.r`,
		"numeric":      `module.network[0].aws_iam_role.r`,
	} {
		t.Run(name, func(t *testing.T) {
			f := bindAddresses(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), bound)
			entries := f.cleanEntries(true)
			entries[0].address = `module.network["east"].aws_iam_role.r`
			out := verifyFacts(t, f, planJSON("1.16.1", entries, ""))
			assert.Equal(t, 1, out.Changes[changeNotInPlan])
			assert.Contains(t, out.NextAction, "not taking effect")
		})
	}
}

func TestVerifyReportsASiblingCreateWhenAModuleIsCalledTwice(t *testing.T) {
	f := bindAddresses(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), "module.east.aws_iam_role.r")
	entries := f.cleanEntries(true)
	entries = append(entries, planEntry{address: "module.west.aws_iam_role.r", actions: []string{"create"}, importing: "null"})
	out := verifyFacts(t, f, planJSON("1.16.1", entries, ""))
	assert.Equal(t, 1, out.Changes[changeNone], "the selected import is fine, the sibling instance is not")
	assert.Contains(t, out.NextAction, "other managed actions 1 outside the selection")
	assert.Equal(t, 1, out.Changes[changeNone])
	assert.Equal(t, 1, out.Unselected.OtherManagedAction)
	require.Len(t, out.Unselected.Addresses, 1)
	assert.Equal(t, "module.west.aws_iam_role.r", out.Unselected.Addresses[0].Address)
	assert.Equal(t, "other_managed_action", out.Unselected.Addresses[0].Kind)
}

func TestVerifyReportsASiblingKeyOfAnIndexedResource(t *testing.T) {
	f := bindAddresses(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), `aws_iam_role.set["a"]`)
	entries := f.cleanEntries(true)
	entries = append(entries, planEntry{address: `aws_iam_role.set["b"]`, actions: []string{"update"}, importing: "null"})
	out := verifyFacts(t, f, planJSON("1.16.1", entries, ""))
	assert.Contains(t, out.NextAction, "other managed actions 1 outside the selection")
	assert.Equal(t, 1, out.Unselected.OtherManagedAction)
}

func TestVerifyDeferredModuleInstanceIsNotClean(t *testing.T) {
	f := bindAddresses(newVerifyFixture(t, 1, importIdentitySupported, "1.16.1"), `module.network["east"].aws_iam_role.r`)
	out := verifyFacts(t, f, planJSON("1.16.1", f.cleanEntries(true), `,"deferred_changes":[{"reason":"instance_count_unknown","resource_change":{"address":"module.network[\"west\"].aws_iam_role.r"}}]`))
	assert.Equal(t, 1, out.Plan.DeferredChanges)
	assert.Contains(t, out.NextAction, "1 deferred changes")
	assert.NotContains(t, out.NextAction, "no changes and no other actions")
}

func TestModuleTargetGuidancePointsAtEvidenceAndKeepsTheRootDefault(t *testing.T) {
	r := importModuleTargetRule
	for _, want := range []string{
		"Default: the resource block and its individual import block both go in the root module",
		"Only if the user wants the resource in a local module", "a new workspace", "the module must be among the files you upload",
		"concrete absolute to address", "compare it with the exact address the plan shows", "repair any not_in_plan",
		"complete configuration", "providers mapping", "import block documentation for the target Terraform version",
		"import block in the root module with a concrete absolute to address",
		"Terraform rejects a provider argument on an import block whose to names a module",
		"keep the identity and its scoping values", "every instance", "ask before you change a module's inputs or calls",
		"Never edit a module that is not local", ".terraform/modules",
		"used by other workspaces", "cannot check that", "a clean plan here says nothing about them",
	} {
		assert.Contains(t, r, want)
	}
	assert.Contains(t, strings.Join(importToolInstructions, " "), r)
	assert.Contains(t, importConfirmationRule, "name the module, each file and module call you changed")
	assert.Contains(t, importConfirmationRule, "other workspaces using the module cannot be checked")
	assert.Contains(t, VerifyImportPlanDefinition().Description, `module.network["east"]`)
	assert.Contains(t, strings.Join(importDownloadInstructions, " "), "local module")
	assert.NotContains(t, strings.Join(importDownloadInstructions, " "), "remote root configuration only")

	// The generated blocks stay the default; the rule is not a blanket removal.
	assert.Contains(t, importKeepGeneratedRule, "Keep the generated resource and import blocks as returned")
	assert.NotContains(t, strings.ToLower(importModuleTargetRule), "always remove")
	assert.Equal(t, 1, strings.Count(importKeepGeneratedRule, "module"), "the keep-generated rule only points at the module guidance")

	// The archive root never moves, and the generated-blocks rule points at the
	// module guidance instead of contradicting it.
	assert.Contains(t, importArchiveRootRule, "the archive root never moves, even when a resource goes in a local module")
	assert.Contains(t, importArchiveRootRule, "unless the module placement guidance applies")
	assert.NotContains(t, importArchiveRootRule, "or in a local module")
	assert.Contains(t, importKeepGeneratedRule, "the module placement guidance is one such case")
	assert.NotContains(t, importKeepGeneratedRule, "provider argument")
}

func TestPreparedNextActionPointsAtModuleGuidanceBeforeAnyEditing(t *testing.T) {
	for name, next := range map[string]string{
		"existing configuration": importPreparedNextAction(&importPrepared{HasCurrentConfiguration: true}, false, false),
		"new workspace":          importPreparedNextAction(&importPrepared{}, false, false),
	} {
		assert.Contains(t, next, importModuleTargetShort, name)
		assert.Less(t, strings.Index(next, importModuleTargetShort), strings.Index(next, "authoring directory"), name)
		assert.NotContains(t, next, importModuleTargetRule, name, "the full rule is not repeated in next_action")
	}
	assert.Contains(t, importModuleTargetShort, "Root module is the default")
	assert.Contains(t, importModuleTargetShort, "module placement guidance")
	// A blocked or stopped response does not send the agent toward editing.
	assert.NotContains(t, importPreparedNextAction(&importPrepared{}, false, true), importModuleTargetShort)
}
