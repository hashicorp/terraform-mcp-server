// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

// Package planchanges reads the redacted JSON plan of a Terraform run and
// reduces it to the resources and outputs it changes. Only addresses, names and
// actions are kept, never attribute values.
package planchanges

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/go-tfe"
)

// MaxEntries caps resource_changes and resource_drift each, so a large plan
// still fits in an agent's context. Counts always cover every change.
const MaxEntries = 500

var (
	runIDPattern  = regexp.MustCompile(`^run-[A-Za-z0-9]+$`)
	planIDPattern = regexp.MustCompile(`^plan-[A-Za-z0-9]+$`)

	// ErrNotReady is returned when the plan has not produced JSON output yet.
	ErrNotReady = errors.New("plan JSON output is not available yet; the plan may still be running")
)

// ResourceChange is one resource the plan changes.
type ResourceChange struct {
	Action          string `json:"action"`
	Address         string `json:"address"`
	PreviousAddress string `json:"previous_address,omitempty"`
	Importing       bool   `json:"importing,omitempty"`
}

// OutputChange is one root module output the plan changes.
type OutputChange struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}

// Summary is the reduced plan.
type Summary struct {
	Counts          map[string]int   `json:"counts"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
	ResourceDrift   []ResourceChange `json:"resource_drift"`
	OutputChanges   []OutputChange   `json:"output_changes"`
	Truncated       bool             `json:"truncated"`
}

type planJSON struct {
	ResourceChanges []resourceChangeJSON  `json:"resource_changes"`
	ResourceDrift   []resourceChangeJSON  `json:"resource_drift"`
	OutputChanges   map[string]changeJSON `json:"output_changes"`
}

type resourceChangeJSON struct {
	Address         string     `json:"address"`
	PreviousAddress string     `json:"previous_address"`
	Change          changeJSON `json:"change"`
}

type changeJSON struct {
	Actions   []string        `json:"actions"`
	Importing json.RawMessage `json:"importing"`
}

// Fetch downloads the redacted JSON plan for exactly one of runID or planID.
// Unlike the json-output endpoint, json-output-redacted does not require
// workspace admin access.
func Fetch(ctx context.Context, client *tfe.Client, runID, planID string) ([]byte, error) {
	runID, planID = strings.TrimSpace(runID), strings.TrimSpace(planID)

	var path string
	switch {
	case runID != "" && planID != "":
		return nil, errors.New("provide only one of run_id or plan_id")
	case runID != "":
		if !runIDPattern.MatchString(runID) {
			return nil, fmt.Errorf("invalid run_id: %q", runID)
		}
		path = fmt.Sprintf("runs/%s/plan/json-output-redacted", url.PathEscape(runID))
	case planID != "":
		if !planIDPattern.MatchString(planID) {
			return nil, fmt.Errorf("invalid plan_id: %q", planID)
		}
		path = fmt.Sprintf("plans/%s/json-output-redacted", url.PathEscape(planID))
	default:
		return nil, errors.New("one of run_id or plan_id is required")
	}

	req, err := client.NewRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := req.Do(ctx, &buf); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, ErrNotReady
	}
	return buf.Bytes(), nil
}

// Summarize reduces a JSON plan to the resources and outputs it changes.
func Summarize(raw []byte) (*Summary, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, ErrNotReady
	}

	var plan planJSON
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	summary := &Summary{
		Counts:          map[string]int{},
		ResourceChanges: []ResourceChange{},
		ResourceDrift:   []ResourceChange{},
		OutputChanges:   []OutputChange{},
	}

	changes := reduce(plan.ResourceChanges)
	for _, c := range changes {
		summary.Counts[c.Action]++
	}
	summary.ResourceChanges, summary.Truncated = capEntries(changes)

	drift, driftTruncated := capEntries(reduce(plan.ResourceDrift))
	summary.ResourceDrift = drift
	summary.Truncated = summary.Truncated || driftTruncated

	for name, change := range plan.OutputChanges {
		if action := classify(change.Actions, false, ""); action != "" {
			summary.OutputChanges = append(summary.OutputChanges, OutputChange{Action: action, Name: name})
		}
	}
	sort.Slice(summary.OutputChanges, func(i, j int) bool {
		return summary.OutputChanges[i].Name < summary.OutputChanges[j].Name
	})

	return summary, nil
}

func reduce(in []resourceChangeJSON) []ResourceChange {
	out := []ResourceChange{}
	for _, rc := range in {
		importing := len(rc.Change.Importing) > 0 && string(rc.Change.Importing) != "null"
		action := classify(rc.Change.Actions, importing, rc.PreviousAddress)
		if action == "" {
			continue
		}
		c := ResourceChange{Action: action, Address: rc.Address, PreviousAddress: rc.PreviousAddress}
		if importing && action != "import" {
			c.Importing = true
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i].Action), rank(out[j].Action); ri != rj {
			return ri < rj
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// classify maps Terraform's action list to one action. It returns "" for
// entries that change nothing.
func classify(actions []string, importing bool, previousAddress string) string {
	switch strings.Join(actions, ",") {
	case "create,delete", "delete,create":
		return "replace"
	case "no-op":
		switch {
		case importing:
			return "import"
		case previousAddress != "":
			return "move"
		}
		return ""
	case "read", "":
		return ""
	}
	return strings.Join(actions, ",")
}

func rank(action string) int {
	switch action {
	case "delete":
		return 0
	case "replace":
		return 1
	case "forget":
		return 2
	case "update":
		return 3
	case "create":
		return 4
	case "import":
		return 5
	case "move":
		return 6
	}
	return 7
}

func capEntries(in []ResourceChange) ([]ResourceChange, bool) {
	if len(in) > MaxEntries {
		return in[:MaxEntries], true
	}
	return in, false
}
