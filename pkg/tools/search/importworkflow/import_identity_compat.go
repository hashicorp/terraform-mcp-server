// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package importworkflow

import (
	"encoding/json"
	"sort"
)

// How a selected type's Search identity relates to the target identity schema.
// The Search identity version comes from the provider release that ran the
// query; the target identity version comes from the schema of the run that
// produced the target's state. They are not provider releases and not the
// managed resource's own schema version.
const (
	identityCompatSameVersion    = "compatible_same_version"
	identityCompatVersionDiffers = "compatible_version_differs"
	identityCompatVersionUnknown = "compatible_version_unknown"
	identityCompatShapeDiffers   = "shape_differs"
	identityCompatNoTarget       = "target_has_no_identity"
	identityCompatTargetNotRead  = "target_not_read"
)

// importIdentityCompatibility states facts about one selected type. It never
// edits the generated import block and does not replace the plan, which
// verify_import_plan reads.
type importIdentityCompatibility struct {
	Status                string   `json:"status"`
	SearchIdentityVersion *int     `json:"search_identity_version,omitempty"`
	TargetIdentityVersion *int     `json:"target_identity_version,omitempty"`
	MissingRequiredKeys   []string `json:"missing_required_keys,omitempty"`
	UnknownKeys           []string `json:"unknown_keys,omitempty"`
	AbsentOptionalKeys    []string `json:"absent_optional_keys,omitempty"`
	Guidance              string   `json:"guidance"`
}

// searchIdentityVersion returns the one version every candidate of a type
// carries, or nil when none does or they disagree.
func searchIdentityVersion(candidates []importDiscoveryCandidate) *int {
	var version *int
	for _, c := range candidates {
		if c.IdentityVersion == nil {
			return nil
		}
		if version != nil && *version != *c.IdentityVersion {
			return nil
		}
		v := *c.IdentityVersion
		version = &v
	}
	return version
}

// identitySchemaVersion reads the numeric version of a target identity schema.
func identitySchemaVersion(schema map[string]any) *int {
	var f float64
	switch n := schema["version"].(type) {
	case float64:
		f = n
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return nil
		}
		f = parsed
	default:
		return nil
	}
	if f < 0 || f != float64(int(f)) {
		return nil
	}
	v := int(f)
	return &v
}

// importIdentityCompat compares the identity keys the Search candidates carry
// with the target identity schema. support is the type's identity_support.
func importIdentityCompat(support string, target map[string]any, candidates []importDiscoveryCandidate) *importIdentityCompatibility {
	out := &importIdentityCompatibility{SearchIdentityVersion: searchIdentityVersion(candidates)}
	switch {
	case support == importIdentityNone:
		out.Status, out.Guidance = identityCompatNoTarget, identityCompatGuidanceNoTarget
		return out
	case support != importIdentitySupported || target == nil:
		out.Status, out.Guidance = identityCompatTargetNotRead, identityCompatGuidanceNotRead
		return out
	}
	out.TargetIdentityVersion = identitySchemaVersion(target)

	searchKeys := map[string]bool{}
	for _, c := range candidates {
		for k := range c.Identity {
			searchKeys[k] = true
		}
	}
	attrs, _ := target["attributes"].(map[string]any)
	for name, raw := range attrs {
		attr, _ := raw.(map[string]any)
		required, _ := attr["required_for_import"].(bool)
		if searchKeys[name] {
			continue
		}
		if required {
			out.MissingRequiredKeys = append(out.MissingRequiredKeys, name)
		} else {
			out.AbsentOptionalKeys = append(out.AbsentOptionalKeys, name)
		}
	}
	for k := range searchKeys {
		if _, ok := attrs[k]; !ok {
			out.UnknownKeys = append(out.UnknownKeys, k)
		}
	}
	sort.Strings(out.MissingRequiredKeys)
	sort.Strings(out.UnknownKeys)
	sort.Strings(out.AbsentOptionalKeys)

	switch {
	case len(out.MissingRequiredKeys) > 0 || len(out.UnknownKeys) > 0:
		out.Status, out.Guidance = identityCompatShapeDiffers, identityCompatGuidanceShape
	case out.SearchIdentityVersion == nil || out.TargetIdentityVersion == nil:
		out.Status, out.Guidance = identityCompatVersionUnknown, identityCompatGuidanceVersionUnknown
	case *out.SearchIdentityVersion != *out.TargetIdentityVersion:
		out.Status, out.Guidance = identityCompatVersionDiffers, identityCompatGuidanceVersionDiffers
	default:
		out.Status, out.Guidance = identityCompatSameVersion, identityCompatGuidanceSame
	}
	if len(out.AbsentOptionalKeys) > 0 {
		out.Guidance += " " + identityCompatGuidanceOptionalScope
	}
	return out
}

// addImportIdentityCompatibility fills each prepared type with its assessment.
// Candidates are grouped by the managed type the caller proposed.
func addImportIdentityCompatibility(types []importPreparedType, candidates []importDiscoveryCandidate, selections []importSelection) {
	byType := map[string][]importDiscoveryCandidate{}
	for i, c := range candidates {
		key := c.Provider.Source + "/" + selections[i].ManagedType
		byType[key] = append(byType[key], c)
	}
	for i := range types {
		t := &types[i]
		t.IdentityCompatibility = importIdentityCompat(t.IdentitySupport, t.IdentitySchema, byType[t.ProviderSource+"/"+t.ManagedType])
	}
}
