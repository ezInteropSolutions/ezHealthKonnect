// ncpdp/util.go
//
// Small canonical-value helpers shared by ncpdp/builder and ncpdp/validator
// (and, later, ncpdp.map_to_canonical) — factored out here rather than
// duplicated per package, per this repo's own no-copy-paste standard.
package ncpdp

import "fmt"

// StringValue converts a canonical record value to a string, returning
// ok=false for nil/missing/empty values so callers skip it.
func StringValue(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		if t == "" {
			return "", false
		}
		return t, true
	case fmt.Stringer:
		return t.String(), true
	default:
		return fmt.Sprintf("%v", t), true
	}
}

// ToSlice normalizes a canonical record's repeatable-group value (which may
// arrive as []interface{} from JSON, or []map[string]interface{} from a
// Go-constructed caller) into []interface{}.
func ToSlice(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case []map[string]interface{}:
		out := make([]interface{}, len(t))
		for i, m := range t {
			out[i] = m
		}
		return out
	default:
		return nil
	}
}
