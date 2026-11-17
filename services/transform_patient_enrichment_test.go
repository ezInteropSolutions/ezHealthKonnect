package services

import "testing"

func TestFHIRInstant(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-10T14:00:00+05:30":     "2026-11-10T14:00:00+05:30",
		"2026-11-10T14:00:00Z":          "2026-11-10T14:00:00Z",
		"2026-11-10T14:00:00":           "2026-11-10T14:00:00Z",
		"2026-11-10T14:00:00.123":       "2026-11-10T14:00:00.123Z",
		"2026-11-10T14:00":              "2026-11-10T14:00:00Z",
		"2026-11-10":                    "2026-11-10T00:00:00Z",
		"not a time":                    "",
	} {
		if got := fhirInstant(in); got != want {
			t.Errorf("fhirInstant(%q) = %q, want %q", in, got, want)
		}
	}
}
