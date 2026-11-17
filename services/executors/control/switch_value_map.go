package control

import (
	"fmt"
	"strings"
)

// mapSwitchValue translates the value a switch_case reads into the label its
// cases are written against, using the step's optional "valueMap" (shipped
// defaults) and "valueMapOverrides" (a site's own choices, which win). It
// lets a template route many inputs (HL7 trigger events A40, A30, A34 ...)
// to one branch ("merge") without a case per input, and lets a site change
// the routing in a settings form instead of in the pipeline.
//
// Keys are compared with the same options as the cases (trimmed, and
// case-insensitive when the step is). A value with no entry is returned
// unchanged, so it falls through to the plain cases and the default.
func mapSwitchValue(config map[string]interface{}, value string, trim, caseInsensitive bool) string {
	norm := func(s string) string {
		if trim {
			s = strings.TrimSpace(s)
		}
		if caseInsensitive {
			s = strings.ToLower(s)
		}
		return s
	}
	want := norm(value)
	for _, name := range []string{"valueMapOverrides", "valueMap"} {
		table, ok := config[name].(map[string]interface{})
		if !ok {
			continue
		}
		for k, v := range table {
			if norm(k) == want {
				if label := strings.TrimSpace(fmt.Sprintf("%v", v)); label != "" {
					return norm(label)
				}
			}
		}
	}
	return value
}
