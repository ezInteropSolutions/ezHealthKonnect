package controllers

import (
	"testing"
	"time"
)

func TestIsUUID(t *testing.T) {
	good := []string{"11111111-2222-3333-4444-555555555555", "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE", "9d8083d2-7009-437d-9fed-d066cf685c36"}
	for _, s := range good {
		if !isUUID(s) {
			t.Errorf("isUUID(%q) = false, want true", s)
		}
	}
	bad := []string{"", "x", "messages_intf_abc", "11111111-2222-3333-4444-55555555555", "11111111-2222-3333-4444-5555555555555",
		`11111111-2222-3333-4444-555555555555"; DROP TABLE users; --`, " 11111111-2222-3333-4444-555555555555"}
	for _, s := range bad {
		if isUUID(s) {
			t.Errorf("isUUID(%q) = true, want false", s)
		}
	}
}

func TestQuoteIdentifier(t *testing.T) {
	if got := quoteIdentifier("messages_intf_abc"); got != `"messages_intf_abc"` {
		t.Errorf("got %s", got)
	}
	// An embedded double quote must be doubled, never allowed to terminate the identifier.
	if got := quoteIdentifier(`a"b`); got != `"a""b"` {
		t.Errorf("got %s", got)
	}
}

func at(h int) time.Time { return time.Date(2026, 10, 27, h, 0, 0, 0, time.UTC) }

func TestRankTraceMatches_ExactMessageIDBeatsNewerCorrelationMatches(t *testing.T) {
	rows := []traceMatch{
		{ID: "r1", MessageID: "tcp_old", CorrelationID: "CTRL1", ReceivedAt: at(8)},
		{ID: "r2", MessageID: "CTRL1", CorrelationID: "OTHER", ReceivedAt: at(7)}, // exact message_id match, oldest
		{ID: "r3", MessageID: "tcp_new", CorrelationID: "CTRL1", ReceivedAt: at(9)},
	}
	got := rankTraceMatches(rows, "CTRL1")
	if got[0].MessageID != "CTRL1" || got[0].MatchedBy != "message_id" {
		t.Fatalf("first = %+v, want the exact message_id match", got[0])
	}
	if got[1].MessageID != "tcp_new" || got[2].MessageID != "tcp_old" {
		t.Errorf("correlation matches must be newest first, got %s then %s", got[1].MessageID, got[2].MessageID)
	}
	if traceAmbiguous(got) {
		t.Error("an exact message_id match is unambiguous, even when other messages share it as a correlation id")
	}
}

func TestRankTraceMatches_ReusedCorrelationIDIsAmbiguousAndNewestFirst(t *testing.T) {
	rows := []traceMatch{
		{ID: "r1", MessageID: "tcp_1", CorrelationID: "MVPTEST003", ReceivedAt: at(8)},
		{ID: "r2", MessageID: "tcp_2", CorrelationID: "MVPTEST003", ReceivedAt: at(10)},
	}
	got := rankTraceMatches(rows, "MVPTEST003")
	if got[0].MessageID != "tcp_2" {
		t.Errorf("must trace the most recent message first, got %s", got[0].MessageID)
	}
	for _, m := range got {
		if m.MatchedBy != "correlation_id" {
			t.Errorf("%s matched_by = %q, want correlation_id", m.MessageID, m.MatchedBy)
		}
	}
	if !traceAmbiguous(got) {
		t.Error("two messages sharing a correlation id must be reported as ambiguous so the user can pick")
	}
}

func TestRankTraceMatches_RowUUIDCountsAsExact(t *testing.T) {
	rows := []traceMatch{{ID: "uuid-1", MessageID: "tcp_1", CorrelationID: "C", ReceivedAt: at(8)}}
	got := rankTraceMatches(rows, "uuid-1")
	if got[0].MatchedBy != "message_id" || traceAmbiguous(got) {
		t.Errorf("a row-uuid lookup is an exact match: %+v", got[0])
	}
}

func TestTraceAmbiguous_SingleMatchIsNeverAmbiguous(t *testing.T) {
	if traceAmbiguous(rankTraceMatches([]traceMatch{{ID: "r", MessageID: "m", CorrelationID: "c", ReceivedAt: at(1)}}, "c")) {
		t.Error("one match cannot be ambiguous")
	}
	if traceAmbiguous(nil) {
		t.Error("no matches cannot be ambiguous")
	}
}
