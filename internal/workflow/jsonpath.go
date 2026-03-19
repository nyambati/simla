package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// applyPath extracts or merges JSON data according to an AWS-style Reference
// Path (a restricted subset of JSONPath).  The path must start with "$".
//
// Special values:
//   - ""   → return data unchanged (AWS default when the field is omitted)
//   - "$"  → return data unchanged (root reference)
//   - "$$" → not supported locally; treated as "$"
//
// Only simple dot-notation paths are supported (e.g. "$.foo.bar").
// Array subscripts ("$.items[0]") and filter expressions are not supported.
func applyPath(data []byte, path string) ([]byte, error) {
	if path == "" || path == "$" || path == "$$" {
		return data, nil
	}

	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("invalid reference path %q: must start with \"$.\"", path)
	}

	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("cannot unmarshal input for path %q: %w", path, err)
	}

	// Walk the path segments after the leading "$.".
	segments := strings.Split(path[2:], ".")
	current := root
	for _, seg := range segments {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q: expected object at segment %q, got %T", path, seg, current)
		}
		val, exists := m[seg]
		if !exists {
			return nil, fmt.Errorf("path %q: key %q not found", path, seg)
		}
		current = val
	}

	out, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("path %q: cannot marshal result: %w", path, err)
	}
	return out, nil
}

// mergePath writes value into the object at the given ResultPath.
//
// ResultPath semantics (AWS):
//   - ""   → discard the task result; return input unchanged
//   - "$"  → replace the entire effective input with the task result
//   - "$.foo.bar" → set/overwrite input["foo"]["bar"] = taskResult
//
// Intermediate objects are created automatically if missing.
func mergePath(input []byte, result []byte, resultPath string) ([]byte, error) {
	// Discard task result — return input unchanged.
	if resultPath == "" {
		return input, nil
	}

	// Replace the whole document with the task result.
	if resultPath == "$" {
		return result, nil
	}

	if !strings.HasPrefix(resultPath, "$.") {
		return nil, fmt.Errorf("invalid ResultPath %q: must start with \"$.\"", resultPath)
	}

	var root map[string]any
	if err := json.Unmarshal(input, &root); err != nil {
		// If input is not an object, start with an empty object.
		root = map[string]any{}
	}

	var value any
	if err := json.Unmarshal(result, &value); err != nil {
		return nil, fmt.Errorf("ResultPath %q: cannot unmarshal task result: %w", resultPath, err)
	}

	segments := strings.Split(resultPath[2:], ".")
	setNested(root, segments, value)

	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("ResultPath %q: cannot marshal output: %w", resultPath, err)
	}
	return out, nil
}

// setNested walks the map creating intermediate objects as needed and then
// sets the leaf key to value.
func setNested(m map[string]any, keys []string, value any) {
	if len(keys) == 1 {
		m[keys[0]] = value
		return
	}
	child, ok := m[keys[0]].(map[string]any)
	if !ok {
		child = map[string]any{}
	}
	m[keys[0]] = child
	setNested(child, keys[1:], value)
}

// evaluateCondition evaluates a single ChoiceRule condition against the given
// JSON document and returns true when the condition is satisfied.
func evaluateCondition(data []byte, rule ChoiceRuleEval) (bool, error) {
	// Logical combinators.
	if len(rule.And) > 0 {
		for _, sub := range rule.And {
			ok, err := evaluateCondition(data, sub)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}

	if len(rule.Or) > 0 {
		for _, sub := range rule.Or {
			ok, err := evaluateCondition(data, sub)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}

	if rule.Not != nil {
		ok, err := evaluateCondition(data, *rule.Not)
		if err != nil {
			return false, err
		}
		return !ok, nil
	}

	// Leaf rule: resolve the variable.
	if rule.Variable == "" {
		return false, fmt.Errorf("choice rule has no Variable set")
	}

	rawVar, err := applyPath(data, rule.Variable)
	if err != nil {
		return false, fmt.Errorf("choice rule variable %q: %w", rule.Variable, err)
	}

	// Unmarshal the resolved variable.
	var varVal any
	if err := json.Unmarshal(rawVar, &varVal); err != nil {
		return false, fmt.Errorf("choice rule: cannot parse variable value: %w", err)
	}

	return applyLeafComparison(varVal, rule)
}

// applyLeafComparison checks the comparison operators on a single ChoiceRuleEval.
func applyLeafComparison(v any, rule ChoiceRuleEval) (bool, error) {
	switch {
	// --- String comparisons ---
	case rule.StringEquals != nil:
		s, ok := v.(string)
		return ok && s == *rule.StringEquals, nil
	case rule.StringLessThan != nil:
		s, ok := v.(string)
		return ok && s < *rule.StringLessThan, nil
	case rule.StringGreaterThan != nil:
		s, ok := v.(string)
		return ok && s > *rule.StringGreaterThan, nil
	case rule.StringLessThanEquals != nil:
		s, ok := v.(string)
		return ok && s <= *rule.StringLessThanEquals, nil
	case rule.StringGreaterThanEquals != nil:
		s, ok := v.(string)
		return ok && s >= *rule.StringGreaterThanEquals, nil
	case rule.StringMatches != nil:
		s, ok := v.(string)
		if !ok {
			return false, nil
		}
		return matchGlob(*rule.StringMatches, s), nil

	// --- Numeric comparisons ---
	// JSON numbers unmarshal to float64.
	case rule.NumericEquals != nil:
		n, ok := v.(float64)
		return ok && n == *rule.NumericEquals, nil
	case rule.NumericLessThan != nil:
		n, ok := v.(float64)
		return ok && n < *rule.NumericLessThan, nil
	case rule.NumericGreaterThan != nil:
		n, ok := v.(float64)
		return ok && n > *rule.NumericGreaterThan, nil
	case rule.NumericLessThanEquals != nil:
		n, ok := v.(float64)
		return ok && n <= *rule.NumericLessThanEquals, nil
	case rule.NumericGreaterThanEquals != nil:
		n, ok := v.(float64)
		return ok && n >= *rule.NumericGreaterThanEquals, nil

	// --- Boolean comparisons ---
	case rule.BooleanEquals != nil:
		b, ok := v.(bool)
		return ok && b == *rule.BooleanEquals, nil

	// --- Type checks ---
	case rule.IsNull:
		return v == nil, nil
	case rule.IsPresent:
		return v != nil, nil
	case rule.IsString:
		_, ok := v.(string)
		return ok, nil
	case rule.IsNumeric:
		_, ok := v.(float64)
		return ok, nil
	case rule.IsBoolean:
		_, ok := v.(bool)
		return ok, nil
	}

	return false, fmt.Errorf("choice rule has no recognisable comparison operator")
}

// ChoiceRuleEval is a flat Go representation of a config.ChoiceRule that uses
// pointer fields so we can distinguish "not set" from the zero value.
type ChoiceRuleEval struct {
	Variable string

	StringEquals            *string
	StringLessThan          *string
	StringGreaterThan       *string
	StringLessThanEquals    *string
	StringGreaterThanEquals *string
	StringMatches           *string

	NumericEquals            *float64
	NumericLessThan          *float64
	NumericGreaterThan       *float64
	NumericLessThanEquals    *float64
	NumericGreaterThanEquals *float64

	BooleanEquals *bool

	IsNull      bool
	IsPresent   bool
	IsString    bool
	IsNumeric   bool
	IsBoolean   bool
	IsTimestamp bool

	And []ChoiceRuleEval
	Or  []ChoiceRuleEval
	Not *ChoiceRuleEval

	Next string
}

// matchGlob implements the limited glob matching defined by AWS Step Functions:
// only "*" wildcards are supported and they match any sequence of characters.
func matchGlob(pattern, s string) bool {
	// Split pattern on "*" and verify each segment appears in order.
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	remaining := s
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(remaining, part)
		if idx < 0 {
			return false
		}
		// First segment must match at position 0 (no leading wildcard).
		if i == 0 && idx != 0 {
			return false
		}
		remaining = remaining[idx+len(part):]
	}
	// Last segment must consume the rest of the string (no trailing wildcard)
	// only when the pattern doesn't end with "*".
	if !strings.HasSuffix(pattern, "*") && remaining != "" {
		return false
	}
	return true
}
