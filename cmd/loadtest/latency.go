// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// latencySummary holds the call durations from one run.
type latencySummary struct {
	count              int
	p50, p95, p99, max time.Duration
}

// summarizeLatencies sorts the durations and picks percentiles by nearest
// rank, so each value is an actual call's duration.
func summarizeLatencies(durations []time.Duration) latencySummary {
	if len(durations) == 0 {
		return latencySummary{}
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	return latencySummary{
		count: len(sorted),
		p50:   percentile(sorted, 50),
		p95:   percentile(sorted, 95),
		p99:   percentile(sorted, 99),
		max:   sorted[len(sorted)-1],
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank-1, 0)]
}

func (s latencySummary) String() string {
	return fmt.Sprintf("p50 %s  p95 %s  p99 %s  max %s",
		roundDuration(s.p50), roundDuration(s.p95), roundDuration(s.p99), roundDuration(s.max))
}

// roundDuration shows milliseconds for normal calls and microseconds for
// fast local ones so the output stays readable.
func roundDuration(d time.Duration) time.Duration {
	if d < time.Millisecond {
		return d.Round(time.Microsecond)
	}
	return d.Round(time.Millisecond)
}
