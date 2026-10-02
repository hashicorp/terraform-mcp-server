// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-tfe"
)

// parseTagBindings splits a comma-separated tag list into tag bindings. An entry
// containing a colon becomes a key-value binding, splitting on the first colon only, so
// "owner:team:platform" binds "owner" to "team:platform". Any other entry becomes a
// key-only binding. Entries that are blank, or whose key is blank, are dropped.
func parseTagBindings(tags string) []*tfe.TagBinding {
	var bindings []*tfe.TagBinding
	for _, entry := range strings.Split(tags, ",") {
		entry = strings.TrimSpace(entry)

		key, value, hasValue := strings.Cut(entry, ":")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if hasValue {
			bindings = append(bindings, &tfe.TagBinding{Key: key, Value: strings.TrimSpace(value)})
			continue
		}
		bindings = append(bindings, &tfe.TagBinding{Key: key})
	}
	return bindings
}

// formatTagBinding renders a binding the way callers write it, as "key:value" for a
// key-value binding and a bare "key" otherwise. The tag tools share this so a tag reads
// back in the form it was created with.
func formatTagBinding(binding *tfe.TagBinding) string {
	if binding.Value == "" {
		return binding.Key
	}
	return fmt.Sprintf("%s:%s", binding.Key, binding.Value)
}
