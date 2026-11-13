// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSummarizeLatencies(t *testing.T) {
	t.Run("no calls", func(t *testing.T) {
		assert.Equal(t, latencySummary{}, summarizeLatencies(nil))
	})

	t.Run("one call", func(t *testing.T) {
		s := summarizeLatencies([]time.Duration{42 * time.Millisecond})
		assert.Equal(t, latencySummary{count: 1, p50: 42 * time.Millisecond, p95: 42 * time.Millisecond, p99: 42 * time.Millisecond, max: 42 * time.Millisecond}, s)
	})

	t.Run("percentiles use nearest rank", func(t *testing.T) {
		// 100ms down to 1ms so the input starts out of order
		var durations []time.Duration
		for i := 100; i >= 1; i-- {
			durations = append(durations, time.Duration(i)*time.Millisecond)
		}
		s := summarizeLatencies(durations)
		assert.Equal(t, 100, s.count)
		assert.Equal(t, 50*time.Millisecond, s.p50)
		assert.Equal(t, 95*time.Millisecond, s.p95)
		assert.Equal(t, 99*time.Millisecond, s.p99)
		assert.Equal(t, 100*time.Millisecond, s.max)
		assert.Equal(t, 100*time.Millisecond, durations[0], "the input slice shouldn't get reordered")
	})
}

func TestRoundDuration(t *testing.T) {
	assert.Equal(t, 183*time.Millisecond, roundDuration(183456*time.Microsecond))
	assert.Equal(t, 457*time.Microsecond, roundDuration(456789*time.Nanosecond))
}
