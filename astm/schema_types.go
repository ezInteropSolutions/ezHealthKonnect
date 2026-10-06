// astm/schema_types.go
// Layers 1-2 of the ASTM E1394-97 schema-driven engine: fields (with
// intra-field components/repeats as first-class constructs) and records (the
// shared library unit, authored once per record-type letter).
//
// Deliberately NOT mirroring edi/schema_types.go's X12LoopDef/
// X12TransactionSetDef recursive-loop-tree machinery (TriggerDiscriminator,
// Wrapper, two-pass sibling matching) — see this feature's own plan doc
// ("Architectural correction — do not copy EDI X12's loop engine wholesale").
// ASTM's record types are a fixed one-character alphabet where every letter
// is already its own unique trigger, and the overall message shape
// (H -> [P -> [O -> [R|C]]]* -> [Q]* -> L) is fixed by the STANDARD itself,
// not something that varies per message the way X12's loop tree varies per
// transaction set. That fixed shape is walked as plain Go control flow in
// astm/record_engine.go; ASTMMessageDef below exists only to name which
// record types a given message profile actually uses (documentation +
// validator input), not to describe a variable tree.
package astm

// ASTMFieldDef defines one field within a record. It is EXACTLY ONE of: a
// plain scalar (DataType set), a component field (Component set, for values
// joined by the component delimiter "^" within this one field position —
// e.g. O5's Universal Test ID is often "^^^GLU" tests-code^^^), or a
// repeating field (Repeat set, for values joined by the repeat delimiter "\"
// within this one position — e.g. multiple Universal Test IDs on one O
// record). Mutually exclusive with each other, same convention as
// edi.X12ElementDef's DataType/Composite split.
type ASTMFieldDef struct {
	Pos        int             `json:"pos"`                 // record-relative field position, 1-based, matching ASTM's own numbering (field 1 is the record-type letter itself and is never represented here — numbering here starts at the first delimited field AFTER the record type+sequence)
	Key        string          `json:"key"`                 // canonical field key, e.g. "specimenID"
	Name       string          `json:"name,omitempty"`      // display only
	DataType   ASTMDataType    `json:"dataType,omitempty"`  // set for a plain scalar field
	Component  *ASTMComponentDef `json:"component,omitempty"` // set for a component (^-delimited) field
	Repeat     *ASTMRepeatDef  `json:"repeat,omitempty"`    // set for a repeat (\-delimited) field
	FixedValue string          `json:"fixedValue,omitempty"`
	ValueSet   string          `json:"valueSet,omitempty"` // optional code-list reference, documentation only in phase 1
	Required   bool            `json:"required,omitempty"`
}

// ASTMComponentDef describes sub-fields joined by the component separator
// ("^") within ONE field position (e.g. O5 Universal Test ID:
// "^^^GLU" = alternate^alternate^alternate^GLU).
type ASTMComponentDef struct {
	SubFields []*ASTMFieldDef `json:"subFields"` // each with its own Pos ("1","2",...) relative to the component, Key, DataType
}

// ASTMRepeatDef describes a field position that may itself repeat via the
// repeat separator ("\") — e.g. multiple Universal Test IDs on one O record.
// ItemDef describes the shape of ONE repeated item (which may itself be a
// plain scalar or a component).
type ASTMRepeatDef struct {
	ItemKey string        `json:"itemKey"` // canonical key for one repeated item
	Item    *ASTMFieldDef `json:"item"`    // the shape of one item — Pos is unused here (repeats don't sub-position), DataType/Component describe its content
}

// ASTMRecordDef is the shared-library unit — authored ONCE per record-type
// letter in schemas/e1394_97/records/<ID>.json and referenced by ID from
// astm/record_engine.go's fixed block walk, never duplicated.
type ASTMRecordDef struct {
	ID         string          `json:"id"`   // "H" | "P" | "O" | "R" | "C" | "Q" | "L" — the addressing key and shared-library filename
	Name       string          `json:"name,omitempty"`
	Fields     []*ASTMFieldDef `json:"fields,omitempty"`
	SourceRefs []string        `json:"sourceRefs,omitempty"` // which free source(s) this definition was cross-validated against — see this feature's own Pre-Phase sourcing gate
}

// ASTMMessageDef names which record types a given message profile
// recognizes/expects — ASTM doesn't version by transaction set the way X12
// does, so phase 1 of this engine expects exactly one registered message
// (schemas/e1394_97/messages/generic_lab_result.json). Kept as a named,
// loaded construct (rather than hardcoding "generic_lab_result" as a Go
// string literal) purely so a second message profile could be added later as
// pure schema data, matching this project's own anti-hardcoding standard,
// even though phase 1 only ever has one.
type ASTMMessageDef struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	UsesComments  bool     `json:"usesComments,omitempty"`  // whether this profile's own real traffic includes C records
	UsesQuery     bool     `json:"usesQuery,omitempty"`      // whether this profile's own real traffic includes Q records (host-query mode)
	SourceRefs    []string `json:"sourceRefs,omitempty"`
}

// ASTMSpecDef is the fully-loaded, resolved schema: the shared record
// library plus every registered message profile.
type ASTMSpecDef struct {
	SpecVersion string
	Records     map[string]*ASTMRecordDef // keyed by record-type letter, e.g. "H"
	Messages    map[string]*ASTMMessageDef
}
