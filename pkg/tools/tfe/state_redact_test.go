// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"testing"
)

// parseTestState parses a state v4 JSON document for tests.
func parseTestState(t *testing.T, doc string) *terraformState {
	t.Helper()
	var s terraformState
	if err := json.Unmarshal([]byte(doc), &s); err != nil {
		t.Fatalf("parsing test state: %v", err)
	}
	return &s
}

func TestManifestRedactionRealCtyPaths(t *testing.T) {
	state := parseTestState(t, `{
		"version": 4,
		"resources": [{
			"mode": "managed",
			"type": "aws_db_instance",
			"name": "main",
			"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
			"instances": [{
				"attributes": {"id": "db-1", "password": "hunter2"},
				"sensitive_attributes": [[{"type": "get_attr", "value": "password"}]]
			}]
		}]
	}`)

	resources := extractResources(state)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	got := resources[0].Attributes["password"]
	if got != redactedSensitive {
		t.Fatalf("password not redacted via cty path: got %v, want %q", got, redactedSensitive)
	}
	if resources[0].Attributes["id"] != "db-1" {
		t.Fatalf("non-sensitive attribute was altered: %v", resources[0].Attributes["id"])
	}
}

// TestManifestRedactionLegacyAndNested verifies the legacy flat-string form and a nested map path.
func TestManifestRedactionLegacyAndNested(t *testing.T) {
	state := parseTestState(t, `{
		"version": 4,
		"resources": [{
			"mode": "managed", "type": "x", "name": "y", "provider": "p",
			"instances": [{
				"attributes": {"token": "abc", "creds": {"secret": "s3cr3t", "user": "u"}},
				"sensitive_attributes": [
					"token",
					[{"type": "get_attr", "value": "creds"}, {"type": "get_attr", "value": "secret"}]
				]
			}]
		}]
	}`)

	attrs := extractResources(state)[0].Attributes
	if attrs["token"] != redactedSensitive {
		t.Errorf("legacy flat-key redaction failed: %v", attrs["token"])
	}
	creds := attrs["creds"].(map[string]interface{})
	if creds["secret"] != redactedSensitive {
		t.Errorf("nested path redaction failed: %v", creds["secret"])
	}
	if creds["user"] != "u" {
		t.Errorf("sibling value should be untouched: %v", creds["user"])
	}
}

func TestManifestRedactionListIndex(t *testing.T) {
	state := parseTestState(t, `{
		"version": 4,
		"resources": [{
			"mode": "managed", "type": "x", "name": "y", "provider": "p",
			"instances": [{
				"attributes": {"rules": [{"name": "ok"}, {"name": "secret-value"}]},
				"sensitive_attributes": [[
					{"type": "get_attr", "value": "rules"},
					{"type": "index", "value": 1},
					{"type": "get_attr", "value": "name"}
				]]
			}]
		}]
	}`)

	rules := extractResources(state)[0].Attributes["rules"].([]interface{})
	if rules[1].(map[string]interface{})["name"] != redactedSensitive {
		t.Errorf("list-index redaction failed: %v", rules[1])
	}
	if rules[0].(map[string]interface{})["name"] != "ok" {
		t.Errorf("sibling list element should be untouched: %v", rules[0])
	}
}

// TestDeepCopyMapFailClosedSemantics verifies the deep copy does not alias the original.
func TestDeepCopyMapFailClosedSemantics(t *testing.T) {
	orig := map[string]interface{}{"a": map[string]interface{}{"b": "c"}}
	cp, err := deepCopyMap(orig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cp["a"].(map[string]interface{})["b"] = "mutated"
	if orig["a"].(map[string]interface{})["b"] != "c" {
		t.Errorf("deepCopyMap aliased the original map")
	}

	if out, err := deepCopyMap(nil); out != nil || err != nil {
		t.Errorf("deepCopyMap(nil) should return (nil, nil), got (%v, %v)", out, err)
	}
}

// TestRedactionDoesNotMutateInput verifies the source state's attributes are untouched by extraction.
func TestRedactionDoesNotMutateInput(t *testing.T) {
	state := parseTestState(t, `{
		"version": 4,
		"resources": [{
			"mode": "managed", "type": "x", "name": "y", "provider": "p",
			"instances": [{
				"attributes": {"password": "hunter2"},
				"sensitive_attributes": [[{"type": "get_attr", "value": "password"}]]
			}]
		}]
	}`)

	_ = extractResources(state)
	if state.Resources[0].Instances[0].Attributes["password"] != "hunter2" {
		t.Errorf("source state was mutated by redaction: %v", state.Resources[0].Instances[0].Attributes["password"])
	}
}
