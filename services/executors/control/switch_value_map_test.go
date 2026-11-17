package control

import "testing"

func TestMapSwitchValue(t *testing.T) {
	cfg := map[string]interface{}{
		"valueMap":          map[string]interface{}{"A40": "merge", "A30": "Merge", "A47": "replace_id"},
		"valueMapOverrides": map[string]interface{}{"a30": "ignore", "A02": "register", "A47": "  "},
	}
	cases := []struct {
		in, want string
		ci       bool
	}{
		{"A40", "merge", true},
		{"a40", "merge", true},
		{"A30", "ignore", true},   // override wins over the default
		{"A47", "replace_id", true}, // a blank override is ignored
		{"A02", "register", true},
		{"A99", "A99", true}, // unmapped: unchanged
		{"a40", "a40", false}, // case-sensitive step: no match
	}
	for _, c := range cases {
		if got := mapSwitchValue(cfg, c.in, true, c.ci); got != c.want {
			t.Errorf("%q (ci=%v) = %q, want %q", c.in, c.ci, got, c.want)
		}
	}
	if got := mapSwitchValue(nil, "A40", true, true); got != "A40" {
		t.Errorf("no map: %q", got)
	}
	if got := mapSwitchValue(map[string]interface{}{"valueMap": "bad"}, "A40", true, false); got != "A40" {
		t.Errorf("malformed map: %q", got)
	}
}
