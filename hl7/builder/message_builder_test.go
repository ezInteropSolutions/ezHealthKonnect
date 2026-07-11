package builder

import (
	"strings"
	"testing"

	"ezhealthkonnect/hl7"
)

func TestSegment_Set_FlatField(t *testing.T) {
	seg := NewSegment("PID")
	if err := seg.Set("PID.3", "MRN12345"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	got := seg.Render()
	want := "PID|||MRN12345"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestSegment_Set_ComponentAndSubcomponent(t *testing.T) {
	seg := NewSegment("PID")
	if err := seg.Set("PID.5.1", "Doe"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := seg.Set("PID.5.2", "John"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	got := seg.Render()
	want := "PID|||||Doe^John"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestSegment_Set_WrongSegmentPrefix_ReturnsError(t *testing.T) {
	seg := NewSegment("PID")
	if err := seg.Set("OBX.5", "value"); err == nil {
		t.Fatal("expected an error when fieldKey belongs to a different segment")
	}
}

func TestSegment_Set_MissingComponentsRenderEmpty(t *testing.T) {
	// Component 1 and 3 set, component 2 left unset -- must still render as
	// "a^^c", not "a^c" (which would silently shift c into position 2).
	seg := NewSegment("XYZ")
	seg.Set("XYZ.1.1", "a")
	seg.Set("XYZ.1.3", "c")
	got := seg.Render()
	want := "XYZ|a^^c"
	if got != want {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestBuildMessage_MSHStructure(t *testing.T) {
	msh := MSHConfig{
		MessageType:  "ADT",
		TriggerEvent: "A01",
		Version:      "2.5.1",
		ControlID:    "CTRL123",
		Timestamp:    "20260101120000",
	}
	msg := BuildMessage(msh, nil, BuildOptions{})

	if !strings.HasPrefix(msg, "MSH|^~\\&|ezHealthKonnect|EHK||") {
		t.Errorf("unexpected MSH prefix: %q", msg)
	}
	if !strings.Contains(msg, "|ADT^A01|CTRL123|P|2.5.1") {
		t.Errorf("expected message type/control ID/processing ID/version fields, got: %q", msg)
	}
	if !strings.HasSuffix(msg, "\r") {
		t.Errorf("expected message to end with a bare CR terminator, got: %q", msg)
	}
}

func TestBuildMessage_SegmentOrderAndTerminator(t *testing.T) {
	pid := NewSegment("PID")
	pid.Set("PID.3", "12345")
	pv1 := NewSegment("PV1")
	pv1.Set("PV1.2", "I")

	msg := BuildMessage(MSHConfig{MessageType: "ADT", TriggerEvent: "A01", Version: "2.5.1"}, []*Segment{pid, pv1}, BuildOptions{})

	lines := strings.Split(strings.TrimRight(msg, "\r"), "\r")
	if len(lines) != 3 {
		t.Fatalf("expected 3 segments (MSH, PID, PV1), got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "MSH") {
		t.Errorf("expected first segment to be MSH, got %q", lines[0])
	}
	if lines[1] != "PID|||12345" {
		t.Errorf("PID line = %q, want %q", lines[1], "PID|||12345")
	}
	if lines[2] != "PV1||I" {
		t.Errorf("PV1 line = %q, want %q", lines[2], "PV1||I")
	}
}

// TestBuildMessage_RoundTripsThroughRealParser is the strongest correctness
// check available: build a message with known field values, then feed the
// raw output through the EXISTING hl7.ParseWithRealSchema parser (an
// independent oracle this package never modifies) and assert the parsed
// fields match what was configured.
func TestBuildMessage_RoundTripsThroughRealParser(t *testing.T) {
	hl7.InitRealSchemaLoader("../../schemas/hl7")

	pid := NewSegment("PID")
	pid.Set("PID.3", "MRN98765")
	pid.Set("PID.5.1", "Doe")
	pid.Set("PID.5.2", "Jane")

	msg := BuildMessage(MSHConfig{MessageType: "ADT", TriggerEvent: "A01", Version: "2.5.1"}, []*Segment{pid}, BuildOptions{})

	parsed := hl7.ParseWithRealSchema(msg)
	if !parsed.Success {
		t.Fatalf("ParseWithRealSchema failed: %v", parsed.Error)
	}

	pidSeg, ok := parsed.BasicSegments["PID"]
	if !ok {
		t.Fatal("expected parsed message to contain a PID segment")
	}
	if got := pidSeg.Fields["PID.3"]; got != "MRN98765" {
		t.Errorf("parsed PID.3 = %q, want MRN98765", got)
	}
	if got := pidSeg.Fields["PID.5"]; got != "Doe^Jane" {
		t.Errorf("parsed PID.5 = %q, want Doe^Jane", got)
	}
}
