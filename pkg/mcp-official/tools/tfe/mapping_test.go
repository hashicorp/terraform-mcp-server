// Copyright IBM Corp. 2025, 2026
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertMapsAllAttributes fails when upstream has a jsonapi attr or primary field
// that ours doesn't. relations are skipped since we flatten those on purpose.
// skip is for fields we're leaving out intentionally
// we leverage this in order to catch drifts when go-tfe adds a field we haven't mapped yet.
func assertMapsAllAttributes(t *testing.T, upstream, ours reflect.Type, skip map[string]string) {
	t.Helper()

	ourFields := make(map[string]bool, ours.NumField())
	for i := 0; i < ours.NumField(); i++ {
		ourFields[ours.Field(i).Name] = true
	}

	for i := 0; i < upstream.NumField(); i++ {
		field := upstream.Field(i)
		tag := field.Tag.Get("jsonapi")
		if !strings.HasPrefix(tag, "attr,") && !strings.HasPrefix(tag, "primary,") {
			continue
		}
		if _, skipped := skip[field.Name]; skipped {
			continue
		}
		assert.True(t, ourFields[field.Name],
			"%s.%s is not mapped in %s; add it or add it to the skip list with a reason",
			upstream.Name(), field.Name, ours.Name())
	}
}
