package builder

import (
	"testing"
	"time"
)

func TestParseTimezoneOffset_Valid(t *testing.T) {
	cases := map[string]int{
		"-0500": -5 * 3600,
		"+0530": 5*3600 + 30*60,
		"+0000": 0,
		"-0000": 0,
	}
	for offset, wantSeconds := range cases {
		loc := parseTimezoneOffset(offset)
		_, gotSeconds := time.Now().In(loc).Zone()
		if gotSeconds != wantSeconds {
			t.Errorf("parseTimezoneOffset(%q): got %d seconds, want %d", offset, gotSeconds, wantSeconds)
		}
	}
}

func TestParseTimezoneOffset_EmptyOrMalformed_FallsBackToUTC(t *testing.T) {
	for _, offset := range []string{"", "garbage", "UTC", "-5:00", "+99999"} {
		loc := parseTimezoneOffset(offset)
		if loc != time.UTC {
			t.Errorf("parseTimezoneOffset(%q): expected fallback to UTC, got %v", offset, loc)
		}
	}
}

func TestNowCDATimestamp_AlwaysIncludesOffsetSuffix(t *testing.T) {
	ts := nowCDATimestamp("")
	if len(ts) != len("20060102150405-0700") {
		t.Fatalf("expected a 19-char timestamp with offset suffix, got %q (len %d)", ts, len(ts))
	}
	if ts[14:19] != "+0000" {
		t.Errorf("expected default offset +0000 for empty input, got %q", ts[14:])
	}
}

func TestNowCDATimestamp_CustomOffset(t *testing.T) {
	ts := nowCDATimestamp("-0500")
	if ts[14:19] != "-0500" {
		t.Errorf("expected -0500 offset suffix, got %q (full: %q)", ts[14:], ts)
	}
}

func TestNowCDATimestamp_MalformedOffset_FallsBackToUTC(t *testing.T) {
	ts := nowCDATimestamp("not-an-offset")
	if ts[14:19] != "+0000" {
		t.Errorf("expected fallback to +0000 for malformed offset, got %q (full: %q)", ts[14:], ts)
	}
}
