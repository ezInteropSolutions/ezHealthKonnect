package services

import (
	"strings"
	"testing"
)

func TestFailedMessagesSQL(t *testing.T) {
	q := failedMessagesSQL("messages_intf_6f1b0c1e_0000_4000_8000_000000000001", 30)
	for _, want := range []string{"FROM messages_intf_6f1b0c1e_0000_4000_8000_000000000001", "status = 'failed'", "received_at", "INTERVAL '30 minutes'"} {
		if !strings.Contains(q, want) {
			t.Errorf("%q lacks %q", q, want)
		}
	}
	if derefString(nil) != "" || derefString(strPtr("x")) != "x" {
		t.Error("derefString")
	}
	// a rule's interface becomes its message table the same way the table was named
	if got := "messages_intf_" + sanitizeUUID("6f1b0c1e-0000-4000-8000-000000000001"); !safeTableNameRegex.MatchString(got) {
		t.Errorf("%q is not an accepted table name", got)
	}
}

func strPtr(s string) *string { return &s }

func TestFailedMessagesAlertMessageAndSeverity(t *testing.T) {
	warn, crit := 1.0, 5.0
	rule := &AlertRule{RuleType: "FAILED_MESSAGES", Condition: "gt", ThresholdWarning: &warn, ThresholdCritical: &crit, EvaluationWindowMins: 60}
	s := &AnalyticsService{}
	if got, ok := s.determineSeverity(rule, 1); ok {
		t.Errorf("1 failure is not over a threshold of 1: %q", got)
	}
	if got, ok := s.determineSeverity(rule, 2); !ok || got != "warning" {
		t.Errorf("2 failures: %q %v", got, ok)
	}
	if msg := s.buildAlertMessage(rule, 2, "CSV Load"); !strings.Contains(msg, "2 failed messages on CSV Load") {
		t.Errorf("message: %q", msg)
	}
}
