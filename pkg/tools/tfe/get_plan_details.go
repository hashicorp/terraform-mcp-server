// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-mcp-server/pkg/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	log "github.com/sirupsen/logrus"
)

// JSON plan structs for parsing the execution plan output
type jsonPlan struct {
	FormatVersion   string                    `json:"format_version"`
	TerraformVersion string                   `json:"terraform_version"`
	Applyable       bool                      `json:"applyable"`
	Complete        bool                      `json:"complete"`
	Errored         bool                      `json:"errored"`
	ResourceChanges []resourceChange          `json:"resource_changes"`
	OutputChanges   map[string]outputChange   `json:"output_changes"`
	ResourceDrift   []resourceChange          `json:"resource_drift"`
}

type resourceChange struct {
	Address       string `json:"address"`
	PrevAddress   string `json:"previous_address,omitempty"`
	ModuleAddress string `json:"module_address,omitempty"`
	Mode          string `json:"mode"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	Index         any    `json:"index,omitempty"`
	Change        change `json:"change"`
	ActionReason  string `json:"action_reason,omitempty"`
}

type change struct {
	Actions         []string    `json:"actions"`
	Before          interface{} `json:"before"`
	After           interface{} `json:"after"`
	AfterUnknown    interface{} `json:"after_unknown,omitempty"`
	BeforeSensitive interface{} `json:"before_sensitive,omitempty"`
	AfterSensitive  interface{} `json:"after_sensitive,omitempty"`
}

type outputChange struct {
	Change change `json:"change"`
}

// GetPlanDetails creates a tool to get the detailed execution plan for a Terraform run.
func GetPlanDetails(logger *log.Logger) server.ServerTool {
	return server.ServerTool{
		Tool: mcp.NewTool("get_plan_details",
			mcp.WithDescription(`Fetches the detailed execution plan for a Terraform run, showing all resource changes (additions, modifications, deletions) the plan intends to make. Accepts a run ID, retrieves the associated plan, and returns a formatted summary of all resource and output changes with before/after diffs.`),
			mcp.WithTitleAnnotation("Get detailed plan changes for a Terraform run"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("run_id",
				mcp.Required(),
				mcp.Description("The ID of the run to get plan details for"),
			),
		),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return getPlanDetailsHandler(ctx, req, logger)
		},
	}
}

func getPlanDetailsHandler(ctx context.Context, request mcp.CallToolRequest, logger *log.Logger) (*mcp.CallToolResult, error) {
	runID, err := request.RequireString("run_id")
	if err != nil {
		return ToolError(logger, "missing required input: run_id", err)
	}

	tfeClient, err := client.GetTfeClientFromContext(ctx, logger)
	if err != nil {
		return ToolError(logger, "failed to get Terraform client", err)
	}

	// Read the run with the plan relationship included so we get the plan ID
	run, err := tfeClient.Runs.ReadWithOptions(ctx, runID, &tfe.RunReadOptions{
		Include: []tfe.RunIncludeOpt{tfe.RunPlan},
	})
	if err != nil {
		return ToolErrorf(logger, "run not found: %s", runID)
	}

	if run.Plan == nil {
		return ToolErrorf(logger, "no plan associated with run: %s", runID)
	}

	plan := run.Plan

	// Try to fetch the full JSON execution plan
	jsonOutput, err := tfeClient.Plans.ReadJSONOutput(ctx, plan.ID)
	if err != nil {
		// JSON output may be unavailable (plan not finished, old TF version, etc.)
		// Fall back to returning basic plan metadata + logs
		logger.WithError(err).Warn("Could not fetch JSON plan output, falling back to plan metadata")
		return buildPlanMetadataResponse(ctx, run, plan, tfeClient, logger), nil
	}

	if len(jsonOutput) == 0 {
		// 204 No Content — plan hasn't completed yet
		return buildPlanMetadataResponse(ctx, run, plan, tfeClient, logger), nil
	}

	// Parse the JSON execution plan
	var parsed jsonPlan
	if err := json.Unmarshal(jsonOutput, &parsed); err != nil {
		logger.WithError(err).Warn("Could not parse JSON plan output, falling back to plan metadata")
		return buildPlanMetadataResponse(ctx, run, plan, tfeClient, logger), nil
	}

	return buildFormattedPlanResponse(run, plan, &parsed), nil
}

// buildPlanMetadataResponse returns a summary when the full JSON plan is unavailable.
// It also fetches plan logs when the plan has errored or completed, providing
// visibility into errors and other diagnostic output.
func buildPlanMetadataResponse(ctx context.Context, run *tfe.Run, plan *tfe.Plan, tfeClient *tfe.Client, logger *log.Logger) *mcp.CallToolResult {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Plan Details for Run %s\n\n", run.ID))
	sb.WriteString(fmt.Sprintf("**Plan ID:** %s\n", plan.ID))
	sb.WriteString(fmt.Sprintf("**Plan Status:** %s\n", plan.Status))
	sb.WriteString(fmt.Sprintf("**Run Status:** %s\n", run.Status))
	sb.WriteString(fmt.Sprintf("**Has Changes:** %t\n", plan.HasChanges))
	sb.WriteString(fmt.Sprintf("**Resource Additions:** %d\n", plan.ResourceAdditions))
	sb.WriteString(fmt.Sprintf("**Resource Changes:** %d\n", plan.ResourceChanges))
	sb.WriteString(fmt.Sprintf("**Resource Destructions:** %d\n", plan.ResourceDestructions))
	sb.WriteString(fmt.Sprintf("**Resource Imports:** %d\n", plan.ResourceImports))

	// For plans that haven't started or are still queued, logs won't be available yet
	switch plan.Status {
	case "pending", "queued", "unreachable":
		sb.WriteString(fmt.Sprintf("\n> **Note:** Detailed resource changes are not yet available because the plan status is `%s`. ", plan.Status))
		sb.WriteString("Re-run this tool after the plan has finished to see the full execution plan.\n")
		return mcp.NewToolResultText(sb.String())
	}

	// For errored, canceled, running, or finished plans, fetch the logs
	logReader, err := tfeClient.Plans.Logs(ctx, plan.ID)
	if err != nil {
		logger.WithError(err).Warn("Could not fetch plan logs")
		if plan.Status == "errored" {
			sb.WriteString("\n> **Note:** The plan errored but logs could not be retrieved.\n")
		}
		return mcp.NewToolResultText(sb.String())
	}

	logBytes, err := io.ReadAll(logReader)
	if err != nil {
		logger.WithError(err).Warn("Could not read plan logs")
		return mcp.NewToolResultText(sb.String())
	}

	logContent := string(logBytes)

	if logContent != "" {
		// Truncate very large logs, keeping the tail (errors are usually at the end)
		const maxLogSize = 50000
		truncated := false
		if len(logContent) > maxLogSize {
			logContent = logContent[len(logContent)-maxLogSize:]
			truncated = true
		}

		sb.WriteString("\n## Plan Logs\n\n")
		if truncated {
			sb.WriteString("*(Log output truncated -- showing last portion which typically contains errors)*\n\n")
		}
		sb.WriteString("```\n")
		sb.WriteString(logContent)
		if !strings.HasSuffix(logContent, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	} else if plan.Status == "errored" {
		sb.WriteString("\n> **Note:** The plan errored but no log output is available.\n")
	}

	return mcp.NewToolResultText(sb.String())
}

// buildFormattedPlanResponse builds a formatted response from the parsed JSON plan.
func buildFormattedPlanResponse(run *tfe.Run, plan *tfe.Plan, parsed *jsonPlan) *mcp.CallToolResult {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Plan Details for Run %s\n\n", run.ID))
	sb.WriteString(fmt.Sprintf("**Plan ID:** %s\n", plan.ID))
	sb.WriteString(fmt.Sprintf("**Plan Status:** %s\n", plan.Status))
	sb.WriteString(fmt.Sprintf("**Run Status:** %s\n", run.Status))
	if parsed.TerraformVersion != "" {
		sb.WriteString(fmt.Sprintf("**Terraform Version:** %s\n", parsed.TerraformVersion))
	}
	sb.WriteString(fmt.Sprintf("**Applyable:** %t\n", parsed.Applyable))
	if parsed.Errored {
		sb.WriteString("**Errored:** true\n")
	}

	// Summary counts
	adds, changes, destroys, replaces, noops := countActions(parsed.ResourceChanges)
	sb.WriteString(fmt.Sprintf("\n## Summary: %d to add, %d to change, %d to destroy", adds, changes, destroys))
	if replaces > 0 {
		sb.WriteString(fmt.Sprintf(", %d to replace", replaces))
	}
	sb.WriteString("\n")

	// Resource drift section
	if len(parsed.ResourceDrift) > 0 {
		sb.WriteString(fmt.Sprintf("\n## Resource Drift (%d detected)\n\n", len(parsed.ResourceDrift)))
		sb.WriteString("Changes detected outside of Terraform:\n\n")
		for _, rc := range parsed.ResourceDrift {
			writeResourceChange(&sb, &rc)
		}
	}

	// Resource changes section
	if len(parsed.ResourceChanges) > 0 {
		// Separate no-ops from actual changes for clarity
		var actualChanges []resourceChange
		for _, rc := range parsed.ResourceChanges {
			action := summarizeActions(rc.Change.Actions)
			if action != "no-op" && action != "read" {
				actualChanges = append(actualChanges, rc)
			}
		}

		if len(actualChanges) > 0 {
			sb.WriteString(fmt.Sprintf("\n## Resource Changes (%d)\n\n", len(actualChanges)))
			for _, rc := range actualChanges {
				writeResourceChange(&sb, &rc)
			}
		}

		if noops > 0 {
			sb.WriteString(fmt.Sprintf("\n*(%d resources unchanged, not shown)*\n", noops))
		}
	} else {
		sb.WriteString("\n## Resource Changes\n\nNo resource changes planned.\n")
	}

	// Output changes section
	if len(parsed.OutputChanges) > 0 {
		sb.WriteString(fmt.Sprintf("\n## Output Changes (%d)\n\n", len(parsed.OutputChanges)))
		for name, oc := range parsed.OutputChanges {
			action := summarizeActions(oc.Change.Actions)
			sb.WriteString(fmt.Sprintf("- **%s** (%s)\n", name, action))
		}
	}

	return mcp.NewToolResultText(sb.String())
}

// countActions tallies the different action types across all resource changes.
func countActions(changes []resourceChange) (adds, updates, destroys, replaces, noops int) {
	for _, rc := range changes {
		switch summarizeActions(rc.Change.Actions) {
		case "create":
			adds++
		case "update":
			updates++
		case "delete":
			destroys++
		case "replace (delete, create)", "replace (create, delete)":
			replaces++
		case "no-op", "read":
			noops++
		}
	}
	return
}

// summarizeActions converts the actions array into a human-readable string.
func summarizeActions(actions []string) string {
	if len(actions) == 1 {
		switch actions[0] {
		case "create":
			return "create"
		case "delete":
			return "delete"
		case "update":
			return "update"
		case "read":
			return "read"
		case "no-op":
			return "no-op"
		}
	}
	if len(actions) == 2 {
		if actions[0] == "delete" && actions[1] == "create" {
			return "replace (delete, create)"
		}
		if actions[0] == "create" && actions[1] == "delete" {
			return "replace (create, delete)"
		}
	}
	return strings.Join(actions, ", ")
}

// actionSymbol returns a symbol prefix for the action type.
func actionSymbol(action string) string {
	switch action {
	case "create":
		return "+"
	case "delete":
		return "-"
	case "update":
		return "~"
	case "replace (delete, create)", "replace (create, delete)":
		return "-/+"
	case "read":
		return "<="
	default:
		return " "
	}
}

// writeResourceChange writes a single resource change entry to the string builder.
func writeResourceChange(sb *strings.Builder, rc *resourceChange) {
	action := summarizeActions(rc.Change.Actions)
	symbol := actionSymbol(action)

	sb.WriteString(fmt.Sprintf("### %s %s (%s)\n", symbol, rc.Address, action))

	if rc.ActionReason != "" {
		sb.WriteString(fmt.Sprintf("  *Reason: %s*\n", formatActionReason(rc.ActionReason)))
	}

	// Show attribute diffs
	diff := buildAttributeDiff(rc.Change.Before, rc.Change.After, action)
	if diff != "" {
		sb.WriteString("\n```diff\n")
		sb.WriteString(diff)
		sb.WriteString("```\n\n")
	} else {
		sb.WriteString("\n")
	}
}

// formatActionReason converts machine-readable action reasons to human-readable text.
func formatActionReason(reason string) string {
	switch reason {
	case "replace_because_tainted":
		return "resource is tainted, so must be replaced"
	case "replace_because_cannot_update":
		return "provider requires replacement to apply changes"
	case "replace_by_request":
		return "replacement was requested"
	case "delete_because_no_resource_config":
		return "no resource configuration found"
	case "delete_because_no_module":
		return "module instance no longer declared"
	case "delete_because_wrong_repetition":
		return "instance key not compatible with repetition mode"
	case "delete_because_count_index":
		return "count index out of range"
	case "delete_because_each_key":
		return "for_each key not in current configuration"
	case "read_because_config_unknown":
		return "configuration values unknown until apply"
	case "read_because_dependency_pending":
		return "depends on resource with pending changes"
	default:
		return reason
	}
}

// buildAttributeDiff compares before/after values and produces a compact diff.
func buildAttributeDiff(before, after interface{}, action string) string {
	var sb strings.Builder

	switch action {
	case "create":
		// Show only the "after" values for new resources
		if afterMap, ok := after.(map[string]interface{}); ok {
			writeMapValues(&sb, afterMap, "+ ", 0)
		}
	case "delete":
		// Show only the "before" values for deleted resources
		if beforeMap, ok := before.(map[string]interface{}); ok {
			writeMapValues(&sb, beforeMap, "- ", 0)
		}
	case "update", "replace (delete, create)", "replace (create, delete)":
		// Show a diff of changed attributes
		beforeMap, beforeOk := before.(map[string]interface{})
		afterMap, afterOk := after.(map[string]interface{})
		if beforeOk && afterOk {
			writeDiff(&sb, beforeMap, afterMap, 0)
		}
	}

	result := sb.String()

	// Truncate very large diffs to keep output reasonable
	const maxDiffSize = 4000
	if len(result) > maxDiffSize {
		result = result[:maxDiffSize] + "\n... (diff truncated, too many changes to display)\n"
	}

	return result
}

// writeMapValues writes all key/value pairs with a given prefix (for create/delete).
func writeMapValues(sb *strings.Builder, m map[string]interface{}, prefix string, depth int) {
	indent := strings.Repeat("  ", depth)
	for key, val := range m {
		formatted := formatValue(val, depth+1)
		sb.WriteString(fmt.Sprintf("%s%s%s = %s\n", prefix, indent, key, formatted))
	}
}

// writeDiff compares two maps and writes only the differences.
func writeDiff(sb *strings.Builder, before, after map[string]interface{}, depth int) {
	indent := strings.Repeat("  ", depth)
	allKeys := mergeKeys(before, after)

	for _, key := range allKeys {
		bVal, bExists := before[key]
		aVal, aExists := after[key]

		if !bExists && aExists {
			// New attribute
			sb.WriteString(fmt.Sprintf("+ %s%s = %s\n", indent, key, formatValue(aVal, depth+1)))
		} else if bExists && !aExists {
			// Removed attribute
			sb.WriteString(fmt.Sprintf("- %s%s = %s\n", indent, key, formatValue(bVal, depth+1)))
		} else if bExists && aExists {
			// Attribute exists in both — check if changed
			if !valuesEqual(bVal, aVal) {
				sb.WriteString(fmt.Sprintf("~ %s%s = %s -> %s\n", indent, key, formatValue(bVal, depth+1), formatValue(aVal, depth+1)))
			}
		}
	}
}

// mergeKeys returns a deduplicated sorted list of keys from both maps.
func mergeKeys(a, b map[string]interface{}) []string {
	seen := make(map[string]bool)
	var keys []string
	for k := range a {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range b {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	// Sort for deterministic output
	sortStrings(keys)
	return keys
}

// sortStrings sorts a slice of strings in place (simple insertion sort to avoid importing sort).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// formatValue formats a value for display, truncating very large or deeply nested values.
func formatValue(val interface{}, depth int) string {
	if val == nil {
		return "null"
	}

	switch v := val.(type) {
	case string:
		if len(v) > 200 {
			return fmt.Sprintf("%q... (truncated)", v[:200])
		}
		return fmt.Sprintf("%q", v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []interface{}:
		if len(v) == 0 {
			return "[]"
		}
		if depth > 2 {
			return fmt.Sprintf("[...%d items]", len(v))
		}
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, formatValue(item, depth+1))
		}
		joined := strings.Join(items, ", ")
		if len(joined) > 200 {
			return fmt.Sprintf("[...%d items]", len(v))
		}
		return fmt.Sprintf("[%s]", joined)
	case map[string]interface{}:
		if len(v) == 0 {
			return "{}"
		}
		if depth > 2 {
			return fmt.Sprintf("{...%d keys}", len(v))
		}
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("{...%d keys}", len(v))
		}
		s := string(data)
		if len(s) > 200 {
			return fmt.Sprintf("{...%d keys}", len(v))
		}
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

// valuesEqual performs a deep equality check for JSON-decoded values.
func valuesEqual(a, b interface{}) bool {
	// Use JSON marshaling for reliable comparison
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	if aErr != nil || bErr != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}
