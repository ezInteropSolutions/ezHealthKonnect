// hl7/builder/message_builder.go
//
// Schema-agnostic HL7 v2 message serializer -- the inverse of
// hl7/parser.go's ParseHL7MessageEnhanced. Segment/FieldTree model field
// values by 1-based position (matching that parser's own "segment name is
// fields[0] when split on the field separator, field N is fields[N]"
// indexing exactly), so a message this package builds parses back
// identically through hl7.ParseWithRealSchema — confirmed by
// message_builder_test.go's round-trip tests.
//
// Deliberately takes no *hl7.RealHL7Schema dependency: building a message
// from already-resolved (fieldKey -> value) pairs needs no schema knowledge
// at all, only correct positional encoding — the schema is only needed for
// the no-code UI's field catalog (see field_catalog.go), exactly the same
// "schema is a catalog concern, not a runtime dependency" split
// fhir/builder/canonical_field_catalog.go's read side already follows.
package builder

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	fieldSeparator        = "|"
	componentSeparator    = "^"
	subcomponentSeparator = "&"

	// defaultSegmentTerminator is the HL7 v2 spec's own segment terminator
	// convention (bare CR) — deliberately NOT the CRLF
	// services/connectors/tcp_mllp_inbound.go's ACK/NACK generator uses,
	// which is a documented, feature-specific choice for generated
	// acknowledgements only, not a general-purpose convention to copy here.
	defaultSegmentTerminator = "\r"

	// defaultEncodingChars is MSH.2's standard value (component ^, repetition
	// ~, escape \, subcomponent &).
	defaultEncodingChars = "^~\\&"
)

// BuildOptions configures BuildMessage's serialization.
type BuildOptions struct {
	// SegmentTerminator joins rendered segments. Default: defaultSegmentTerminator.
	SegmentTerminator string
}

// MSHConfig carries the header fields BuildMessage auto-populates into the
// MSH segment — structurally special (MSH.1 IS the field separator itself,
// with no separator preceding it) so it isn't modeled as a generic Segment.
// Any field left empty gets a sensible default (see renderMSH).
type MSHConfig struct {
	MessageType          string // MSH.9.1, e.g. "ADT"
	TriggerEvent         string // MSH.9.2, e.g. "A01"
	Version              string // MSH.12, e.g. "2.5.1"
	SendingApplication   string // MSH.3, default "ezHealthKonnect" (mirrors ACKConfig's own default)
	SendingFacility      string // MSH.4, default "EHK" (mirrors ACKConfig's own default)
	ReceivingApplication string // MSH.5
	ReceivingFacility    string // MSH.6
	ProcessingID         string // MSH.11, default "P"
	ControlID            string // MSH.10, auto-generated when empty
	Timestamp            string // MSH.7, auto-generated (current time) when empty
}

// FieldTree holds one field/component/subcomponent value tree within a
// Segment being built. A leaf (no Subs) renders as Scalar; a node with Subs
// renders every position from 1..max present (missing positions render as
// empty, preserving positional meaning) joined by componentSeparator one
// level deep, subcomponentSeparator any level deeper.
type FieldTree struct {
	Scalar string
	Subs   map[int]*FieldTree
}

func (t *FieldTree) render(depth int) string {
	if t == nil {
		return ""
	}
	if len(t.Subs) == 0 {
		return t.Scalar
	}
	sep := componentSeparator
	if depth > 0 {
		sep = subcomponentSeparator
	}
	max := 0
	for k := range t.Subs {
		if k > max {
			max = k
		}
	}
	parts := make([]string, max)
	for i := 1; i <= max; i++ {
		parts[i-1] = t.Subs[i].render(depth + 1)
	}
	return strings.Join(parts, sep)
}

func (t *FieldTree) set(positions []int, value string) {
	if len(positions) == 0 {
		t.Scalar = value
		return
	}
	if t.Subs == nil {
		t.Subs = make(map[int]*FieldTree)
	}
	pos := positions[0]
	child, ok := t.Subs[pos]
	if !ok {
		child = &FieldTree{}
		t.Subs[pos] = child
	}
	child.set(positions[1:], value)
}

// Segment holds one segment instance's field values, keyed by 1-based field
// number.
type Segment struct {
	Name   string
	Fields map[int]*FieldTree
}

// NewSegment constructs an empty segment builder for name (e.g. "PID").
func NewSegment(name string) *Segment {
	return &Segment{Name: name, Fields: make(map[int]*FieldTree)}
}

// Set writes value at fieldKey — the SAME fully-qualified "SEGMENT.N",
// "SEGMENT.N.C", or "SEGMENT.N.C.S" form field_catalog.go's
// SegmentFieldCatalog exposes (matching the HL7 path convention
// services/executors/field_utils.go already establishes pipeline-wide, e.g.
// "PID.3", "PID.5.1") — not a bare segment-relative path, so a config value
// pasted into the wrong segment's block fails loudly instead of silently
// writing to the wrong position.
func (s *Segment) Set(fieldKey string, value string) error {
	prefix := s.Name + "."
	if !strings.HasPrefix(fieldKey, prefix) {
		return fmt.Errorf("hl7/builder: field key %q does not belong to segment %q", fieldKey, s.Name)
	}
	rel := strings.TrimPrefix(fieldKey, prefix)
	parts := strings.Split(rel, ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("hl7/builder: empty field position in key %q", fieldKey)
	}

	fieldNum, err := strconv.Atoi(parts[0])
	if err != nil || fieldNum < 1 {
		return fmt.Errorf("hl7/builder: invalid field number %q in key %q", parts[0], fieldKey)
	}
	positions := make([]int, 0, len(parts)-1)
	for _, p := range parts[1:] {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return fmt.Errorf("hl7/builder: invalid component position %q in key %q", p, fieldKey)
		}
		positions = append(positions, n)
	}

	tree, ok := s.Fields[fieldNum]
	if !ok {
		tree = &FieldTree{}
		s.Fields[fieldNum] = tree
	}
	tree.set(positions, value)
	return nil
}

// Render serializes the segment as one pipe-delimited HL7 line (no trailing
// terminator — BuildMessage adds that between/after segments).
func (s *Segment) Render() string {
	max := 0
	for k := range s.Fields {
		if k > max {
			max = k
		}
	}
	parts := make([]string, max+1)
	parts[0] = s.Name
	for i := 1; i <= max; i++ {
		parts[i] = s.Fields[i].render(0)
	}
	return strings.Join(parts, fieldSeparator)
}

// renderMSH builds the MSH segment line — structurally special, not a
// generic Segment: MSH.1 is the field separator character itself (never
// preceded by a separator), exactly mirroring hl7/parser.go's own
// "MSH.1 is the field separator itself (implicit)" parsing convention, so
// fields[1] after a "|" split lands on MSH.2 (encoding characters), not MSH.1.
func renderMSH(cfg MSHConfig) string {
	sendingApp := cfg.SendingApplication
	if sendingApp == "" {
		sendingApp = "ezHealthKonnect"
	}
	sendingFacility := cfg.SendingFacility
	if sendingFacility == "" {
		sendingFacility = "EHK"
	}
	processingID := cfg.ProcessingID
	if processingID == "" {
		processingID = "P"
	}
	timestamp := cfg.Timestamp
	if timestamp == "" {
		timestamp = time.Now().Format("20060102150405")
	}
	controlID := cfg.ControlID
	if controlID == "" {
		controlID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	messageTypeField := cfg.MessageType
	if cfg.TriggerEvent != "" {
		messageTypeField = messageTypeField + componentSeparator + cfg.TriggerEvent
	}

	fields := []string{
		"MSH",
		defaultEncodingChars,
		sendingApp,
		sendingFacility,
		cfg.ReceivingApplication,
		cfg.ReceivingFacility,
		timestamp,
		"", // MSH.8 Security — not populated by default
		messageTypeField,
		controlID,
		processingID,
		cfg.Version,
	}
	return strings.Join(fields, fieldSeparator)
}

// BuildMessage assembles MSH followed by segments (in the given order) into
// a complete HL7 v2 message string, terminating every segment (including the
// last) with opts.SegmentTerminator — the safe, standard convention every
// downstream MLLP/file consumer already tolerates.
func BuildMessage(msh MSHConfig, segments []*Segment, opts BuildOptions) string {
	terminator := opts.SegmentTerminator
	if terminator == "" {
		terminator = defaultSegmentTerminator
	}

	lines := make([]string, 0, len(segments)+1)
	lines = append(lines, renderMSH(msh))
	for _, seg := range segments {
		lines = append(lines, seg.Render())
	}
	return strings.Join(lines, terminator) + terminator
}
