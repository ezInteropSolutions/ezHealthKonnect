// services/executors/condition.go
//
// EvaluateCondition is the single, shared {field, operator, value} evaluator
// behind conditional pipeline logic. It was originally a private method on
// control.IfThenElseExecutor; extracted here (unchanged behavior) so any
// step needing a condition check — control.if_then_else/control.switch_case,
// and now hl7.build's per-segment/per-field Condition — evaluates identical
// semantics through one implementation, rather than each consumer growing
// its own slightly-different condition language.
package executors

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EvaluateCondition evaluates condition (a {field, operator, value} or
// {field, operator, compareToField} map, e.g. from control.if_then_else's
// own config) against data, resolving field/compareToField via the same
// HL7-aware GetNestedValue every other condition/routing check already uses.
//
// Supported operators: equals, not_equals, contains, starts_with, ends_with,
// greater_than, greater_than_or_equal, less_than, less_than_or_equal,
// exists, not_exists, regex_match, in_list.
func EvaluateCondition(condition map[string]interface{}, data map[string]interface{}) (bool, error) {
	field, _ := condition["field"].(string)
	operator, _ := condition["operator"].(string)

	fieldValue := GetNestedValue(data, field)

	var compareValue interface{}
	if compareToField, _ := condition["compareToField"].(string); compareToField != "" {
		compareValue = GetNestedValue(data, compareToField)
	} else {
		compareValue = condition["value"]
	}

	switch operator {
	case "equals":
		return fmt.Sprintf("%v", fieldValue) == fmt.Sprintf("%v", compareValue), nil

	case "not_equals":
		return fmt.Sprintf("%v", fieldValue) != fmt.Sprintf("%v", compareValue), nil

	case "contains":
		return strings.Contains(fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", compareValue)), nil

	case "starts_with":
		return strings.HasPrefix(fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", compareValue)), nil

	case "ends_with":
		return strings.HasSuffix(fmt.Sprintf("%v", fieldValue), fmt.Sprintf("%v", compareValue)), nil

	case "greater_than":
		return conditionToFloat64(fieldValue) > conditionToFloat64(compareValue), nil

	case "greater_than_or_equal":
		return conditionToFloat64(fieldValue) >= conditionToFloat64(compareValue), nil

	case "less_than":
		return conditionToFloat64(fieldValue) < conditionToFloat64(compareValue), nil

	case "less_than_or_equal":
		return conditionToFloat64(fieldValue) <= conditionToFloat64(compareValue), nil

	case "exists":
		return fieldValue != nil, nil

	case "not_exists":
		return fieldValue == nil, nil

	case "regex_match":
		pattern := fmt.Sprintf("%v", compareValue)
		matched, err := regexp.MatchString(pattern, fmt.Sprintf("%v", fieldValue))
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %v", err)
		}
		return matched, nil

	case "in_list":
		valueList, ok := compareValue.([]interface{})
		if !ok {
			return false, fmt.Errorf("value must be a list for 'in_list' operator")
		}
		fieldStr := fmt.Sprintf("%v", fieldValue)
		for _, item := range valueList {
			if fmt.Sprintf("%v", item) == fieldStr {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// conditionToFloat64 mirrors control.toFloat64 exactly (kept private/local —
// not worth exporting a numeric-coercion helper this narrow).
func conditionToFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}
