// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"encoding/json"
	"fmt"
)

const redactedSensitive = "[REDACTED - sensitive]"

// redactSensitiveAttrs returns a copy of attrs with every path named in sensitivePaths
// (the instance's "sensitive_attributes" manifest from state) overwritten with a
// redaction marker. The input map is never mutated.
func redactSensitiveAttrs(attrs map[string]interface{}, sensitivePaths []interface{}) (map[string]interface{}, error) {
	result, err := deepCopyMap(attrs)
	if err != nil {
		return nil, err
	}
	if len(sensitivePaths) == 0 || len(result) == 0 {
		return result, nil
	}
	for _, raw := range sensitivePaths {
		steps := pathSteps(raw)
		if len(steps) > 0 {
			setAtPath(result, steps)
		}
	}
	return result, nil
}

// pathSteps converts one sensitive_attributes entry into an ordered list of steps,
// each a string (map key) or an int (list index).
func pathSteps(raw interface{}) []interface{} {
	switch p := raw.(type) {
	case string: // legacy flat-key form
		if p == "" {
			return nil
		}
		return []interface{}{p}
	case []interface{}:
		steps := make([]interface{}, 0, len(p))
		for _, seg := range p {
			steps = append(steps, normalizeStep(seg))
		}
		return steps
	}
	return nil
}

func normalizeStep(seg interface{}) interface{} {
	switch s := seg.(type) {
	case string:
		return s
	case float64:
		return int(s)
	case map[string]interface{}:
		switch v := s["value"].(type) {
		case string:
			return v
		case float64:
			return int(v)
		}
	}
	return fmt.Sprintf("\x00unmatched:%v", seg)
}

// setAtPath redacts the value at steps within obj, descending maps and slices.
func setAtPath(obj interface{}, steps []interface{}) {
	if len(steps) == 0 || obj == nil {
		return
	}
	last := len(steps) == 1
	switch c := obj.(type) {
	case map[string]interface{}:
		key, ok := steps[0].(string)
		if !ok {
			return
		}
		if last {
			if _, exists := c[key]; exists {
				c[key] = redactedSensitive
			}
			return
		}
		setAtPath(c[key], steps[1:])
	case []interface{}:
		idx, ok := steps[0].(int)
		if !ok || idx < 0 || idx >= len(c) {
			return
		}
		if last {
			c[idx] = redactedSensitive
			return
		}
		setAtPath(c[idx], steps[1:])
	}
}

func deepCopyMap(m map[string]interface{}) (map[string]interface{}, error) {
	if m == nil {
		return nil, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("copying attributes for redaction: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("copying attributes for redaction: %w", err)
	}
	return out, nil
}
