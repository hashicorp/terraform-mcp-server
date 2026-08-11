// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWhoAmI(t *testing.T) {
	s := newTestingSession(t)
	defer s.Close()

	result, resultText := callTool(t, s, "whoami", map[string]any{})

	require.False(t, result.IsError, "whoami should not return an error")
	require.NotEmpty(t, resultText, "whoami should return a non-empty response")

	assert.NotEmpty(t, gjson.Get(resultText, "username").String(), "response should contain a non-empty username")
	assert.NotEmpty(t, gjson.Get(resultText, "email").String(), "response should contain a non-empty email")
	assert.NotEmpty(t, gjson.Get(resultText, "is_service_account").String(), "response should contain a non-empty is_service_account flag")
}
