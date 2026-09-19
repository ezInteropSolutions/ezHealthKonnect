// ncpdptelecom/schema_types.go
// The schema model for the NCPDP Telecommunication D.0 engine — mirrors
// edi/schema_types.go's own layering (field -> segment -> transaction) but
// adapted for D.0's real, simpler shape: no composites/sub-element repeats,
// no loop trees. A segment's own fields are a flat list; a transaction is a
// flat composition of segment references, split into two buckets:
//   - TransmissionGroupSegments: segments appearing exactly once per
//     transmission (e.g. Patient, Insurance).
//   - TransactionGroupSegments: segments that repeat as a GS-delimited
//     cluster, one cluster per claim/drug being submitted (e.g. Claim,
//     Pricing) — confirmed byte-for-byte from a real reference
//     implementation's own fixture (see manifest.json's own sourceRefs).
//
// Request and response directions share the SAME transaction CODE on the
// wire (e.g. "B1" appears in both a claim request and its own adjudication
// response) but use fully disjoint segment-identifier ranges (01-16 request,
// 20-29 response) — so, unlike EDI X12 278's own same-ST01-both-directions
// case, there is no runtime ambiguity to resolve by content-sniffing.
// Request and response are simply two separate TelecomTransactionDef
// entries in the spec's own Transactions map, keyed by direction (the
// caller always knows which one it's parsing/building).
package ncpdptelecom

import "fmt"

// TelecomFieldDef is one field within a segment or the fixed-width header.
type TelecomFieldDef struct {
	Key      string          `json:"key"`
	FieldID  string          `json:"fieldId,omitempty"` // 2-char wire identifier, e.g. "C4" — empty only for header fields, which are positional
	Name     string          `json:"name"`
	DataType TelecomDataType `json:"dataType"`
	Required bool            `json:"required"`
	Places   int             `json:"places,omitempty"` // implied decimal places, for R/RO types only
	Width    int             `json:"width,omitempty"`  // fixed width: REQUIRED for header fields (positional slicing); used for zero-padding N/R/RO fields on build
}

// TelecomSegmentDef is one D.0 segment type (e.g. Patient, Claim, Response
// Status), identified on the wire by its own AM field value.
type TelecomSegmentDef struct {
	Key        string            `json:"key"`
	Identifier string            `json:"identifier"` // the 2-char segment ID value carried in the AM field, e.g. "01"
	Name       string            `json:"name"`
	Fields     []TelecomFieldDef `json:"fields"`
	SourceRefs []string          `json:"sourceRefs,omitempty"`
}

// FieldByID returns the field definition whose wire FieldID matches id, or
// nil if this segment has no such field (an unrecognized field is not an
// error — see the validator's own flexible-not-rigid philosophy).
func (s *TelecomSegmentDef) FieldByID(id string) *TelecomFieldDef {
	for i := range s.Fields {
		if s.Fields[i].FieldID == id {
			return &s.Fields[i]
		}
	}
	return nil
}

// TelecomTransactionDef is one direction (request or response) of one D.0
// transaction code (e.g. "B1" request, "B1" response).
type TelecomTransactionDef struct {
	Code                      string   `json:"code"` // the wire Transaction Code value, e.g. "B1" — identical for both directions
	Name                      string   `json:"name"`
	Direction                 string   `json:"direction"` // "request" | "response"
	TransmissionGroupSegments []string `json:"transmissionGroupSegments"`
	TransactionGroupSegments  []string `json:"transactionGroupSegments"`
	SourceRefs                []string `json:"sourceRefs,omitempty"`
}

// TelecomSpecDef is the fully-resolved, in-memory D.0 spec.
type TelecomSpecDef struct {
	Version              string
	HeaderFields         []TelecomFieldDef
	Segments             map[string]*TelecomSegmentDef // keyed by segment Key (schema name)
	segmentsByIdentifier map[string]*TelecomSegmentDef // keyed by wire Identifier — built once at load time
	Transactions         map[string]*TelecomTransactionDef // keyed by "<code>_request" / "<code>_response"
}

// SegmentByIdentifier resolves a segment definition by its wire-level AM
// identifier value (e.g. "01" -> the Patient segment). Used by the parser
// to dispatch each raw segment it encounters.
func (s *TelecomSpecDef) SegmentByIdentifier(id string) *TelecomSegmentDef {
	return s.segmentsByIdentifier[id]
}

// TransactionKey builds the composite key this spec's own Transactions map
// uses — a small helper so the parser/builder/loader never hand-format this
// string independently and risk drifting out of sync.
func TransactionKey(code, direction string) string {
	return code + "_" + direction
}

// buildIndexes populates segmentsByIdentifier from Segments — called once
// by the loader after every segment file has been read.
func (s *TelecomSpecDef) buildIndexes() error {
	s.segmentsByIdentifier = make(map[string]*TelecomSegmentDef, len(s.Segments))
	for _, seg := range s.Segments {
		if existing, ok := s.segmentsByIdentifier[seg.Identifier]; ok {
			return fmt.Errorf("ncpdptelecom: segment identifier %q used by both %q and %q", seg.Identifier, existing.Key, seg.Key)
		}
		s.segmentsByIdentifier[seg.Identifier] = seg
	}
	return nil
}
