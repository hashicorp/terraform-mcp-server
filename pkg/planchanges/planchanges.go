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
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/go-tfe"
)

// MaxEntries caps resource_changes and resource_drift each, so a large plan
// still fits in an agent's context. Counts always cover every change.
const MaxEntries = 500

// ErrNotReady is returned when the plan has not produced JSON output yet.
var ErrNotReady = errors.New("plan JSON output is not available yet; the plan may still be running")

// actionOrder sorts the most destructive changes first.
var actionOrder = []string{"delete", "replace", "forget", "update", "create", "import", "move"}

// ResourceChange is one resource the plan changes.
type ResourceChange struct {
	Action          string `json:"action"`
	Address         string `json:"address"`
	PreviousAddress string `json:"previous_address,omitempty"`
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
// Unlike json-output, json-output-redacted does not need workspace admin access.
func Fetch(ctx context.Context, client *tfe.Client, runID, planID string) ([]byte, error) {
	runID, planID = strings.TrimSpace(runID), strings.TrimSpace(planID)

	var path string
	switch {
	case runID != "" && planID != "":
		return nil, errors.New("provide only one of run_id or plan_id")
	case runID != "":
		path = fmt.Sprintf("runs/%s/plan/json-output-redacted", url.PathEscape(runID))
	case planID != "":
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

	changes := reduce(plan.ResourceChanges)
	drift := reduce(plan.ResourceDrift)

	summary := &Summary{
		Counts:          map[string]int{},
		ResourceChanges: changes,
		ResourceDrift:   drift,
		OutputChanges:   []OutputChange{},
		Truncated:       len(changes) > MaxEntries || len(drift) > MaxEntries,
	}
	for _, c := range changes {
		summary.Counts[c.Action]++
	}
	if len(changes) > MaxEntries {
		summary.ResourceChanges = changes[:MaxEntries]
	}
	if len(drift) > MaxEntries {
		summary.ResourceDrift = drift[:MaxEntries]
	}

	for name, change := range plan.OutputChanges {
		if action := classify(change, ""); action != "" {
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
		if action := classify(rc.Change, rc.PreviousAddress); action != "" {
			out = append(out, ResourceChange{Action: action, Address: rc.Address, PreviousAddress: rc.PreviousAddress})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if oi, oj := slices.Index(actionOrder, out[i].Action), slices.Index(actionOrder, out[j].Action); oi != oj {
			return oi < oj
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// classify maps Terraform's action list to one action, or "" when nothing changes.
func classify(change changeJSON, previousAddress string) string {
	switch actions := strings.Join(change.Actions, ","); actions {
	case "create,delete", "delete,create":
		return "replace"
	case "no-op":
		switch {
		case len(change.Importing) > 0 && string(change.Importing) != "null":
			return "import"
		case previousAddress != "":
			return "move"
		}
		return ""
	case "read", "":
		return ""
	default:
		return actions
	}
}
