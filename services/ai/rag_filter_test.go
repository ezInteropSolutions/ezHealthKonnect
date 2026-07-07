// services/ai/rag_filter_test.go
// Tests for filterSourceTypesForContext — confirms it only narrows RAG
// retrieval when there's a confident signal, and never regresses general
// (no-context) questions to a narrower search than before.
package ai

import "testing"

func TestFilterSourceTypesForContext_NoSignalReturnsNil(t *testing.T) {
	cases := []RequestContext{
		{},
		{Page: "dashboard"},
		{InterfaceID: "iface-1"}, // interface alone, no message type shape, isn't a strong enough signal
	}
	for _, c := range cases {
		if got := filterSourceTypesForContext(c); got != nil {
			t.Errorf("filterSourceTypesForContext(%+v) = %v, want nil (unfiltered)", c, got)
		}
	}
}

func TestFilterSourceTypesForContext_HL7ShapeNarrowsToHL7Skill(t *testing.T) {
	got := filterSourceTypesForContext(RequestContext{MessageType: "ADT^A01"})
	if got == nil {
		t.Fatal("expected a filter for an HL7-shaped message type")
	}
	want := map[string]bool{"hl7_v2": true, "operational": true, "app_docs": true}
	for _, f := range got {
		if want[f] {
			delete(want, f)
		}
	}
	if len(want) != 0 {
		t.Errorf("expected filter to include hl7_v2/operational/app_docs, got %v", got)
	}
}
