// edi/schema_types.go
// Layers 1-4 of the X12 schema-driven engine: elements (with composites and
// intra-segment repeats as first-class constructs), segments (the shared
// library unit), loops (recursive nesting), and transaction sets (pure
// composition). See edi/datatypes.go for Layer 0.
//
// Unlike cda/schema_types.go, these types are unmarshaled directly from
// their on-disk JSON — X12's structure doesn't need CDA's separate on-disk/
// resolved-type split, since a loop's SegmentIDs are looked up by ID against
// the shared library at walk time (edi/loop_engine.go), not merged/resolved
// into pointers at load time.
package edi

// X12ElementDef defines one element within a segment. It is EXACTLY ONE of:
// a plain scalar (DataType set), a composite (Composite set), or the anchor
// for an intra-segment repeat group handled separately via X12SegmentDef's
// own Repeats field — a repeat is a property of a GROUP of positions, not of
// one element, so it is never expressed on X12ElementDef itself.
type X12ElementDef struct {
	Pos        string           `json:"pos"`                  // segment-relative position, e.g. "01" — matches X12's own numbering
	Key        string           `json:"key"`                  // canonical field key, e.g. "patientControlNumber"
	Name       string           `json:"name,omitempty"`        // display only
	DataType   X12DataType      `json:"dataType,omitempty"`    // set for a plain scalar element
	Composite  *X12CompositeDef `json:"composite,omitempty"`   // set for a composite element (mutually exclusive with DataType)
	FixedValue string           `json:"fixedValue,omitempty"`  // e.g. ST01 == "835" — validated + defaulted on build
	ValueSet   string           `json:"valueSet,omitempty"`    // optional code-list reference, for TypeID elements
	MinLength  int              `json:"minLength,omitempty"`
	MaxLength  int              `json:"maxLength,omitempty"`
	Required   bool             `json:"required,omitempty"` // element-level requiredness within a present segment
}

// X12CompositeDef describes sub-elements joined by the component separator
// within ONE element position (e.g. SVC01 "HC:99213" = qualifier ":"
// procedure code). Reusable wherever a composite element occurs.
type X12CompositeDef struct {
	SubElements []*X12ElementDef `json:"subElements"` // each with its own Pos ("1","2",...), Key, DataType
}

// X12RepeatDef is an intra-segment repeating GROUP of element positions
// (e.g. CAS02-04, CAS05-07, CAS08-10, ... — reason/amount/quantity,
// repeating up to MaxGroups times within ONE segment instance). Modeled as
// its own reusable construct — the intra-segment counterpart to a loop's
// repeat, one level lower.
type X12RepeatDef struct {
	Key         string           `json:"key"`         // canonical array field key, e.g. "adjustments"
	StartPos    int              `json:"startPos"`     // first element position of the first repeat group
	GroupSize   int              `json:"groupSize"`    // positions per repeat, e.g. 3 for CAS's reason/amount/qty
	MaxGroups   int              `json:"maxGroups"`    // e.g. 6 for CAS
	GroupFields []*X12ElementDef `json:"groupFields"`  // field defs for ONE group; Pos is relative to the group's own start (1-based)
}

// X12SegmentDef is the shared-library unit — authored ONCE per segment ID in
// schemas/segments/<ID>.json and referenced by ID from every loop/header/
// trailer slot that uses it, never duplicated.
type X12SegmentDef struct {
	ID         string           `json:"id"`   // e.g. "CLP" — the addressing key, and the shared-library filename
	Name       string           `json:"name,omitempty"`
	Usage      string           `json:"usage,omitempty"`  // "required" | "situational"
	MaxUse     string           `json:"maxUse,omitempty"` // "1" | ">1" — how many TIMES this segment ID may repeat within its loop/header/trailer position
	Elements    []*X12ElementDef `json:"elements,omitempty"`
	Repeats     []*X12RepeatDef  `json:"repeats,omitempty"`     // intra-segment repeat groups, if any (e.g. CAS)
	SyntaxRules []SyntaxRule     `json:"syntaxRules,omitempty"` // element-relational constraints (P/C/L/R/E) — see SyntaxRule's own doc comment
	SourceRefs  []string         `json:"sourceRefs,omitempty"`  // which companion guide(s) this definition was verified against
}

// SyntaxRule expresses one of X12's standard element-relational constraints
// within a single segment occurrence — independent of each element's own
// required/situational status, and INDEPENDENT of edi/datatypes.go's own
// per-element Validate. E.g. BPR's real "P0607" syntax note means "if BPR06
// is present, BPR07 must be too, and vice versa" — a constraint about the
// RELATIONSHIP between two elements, not either element's own shape.
//
// Deliberately never enforced as a blocking condition anywhere in this
// package — edi/validator reports violations as warnings only, per this
// project's own explicit "flexible, not rigid" design goal (2026-09-01):
// parsing and building are never affected by these rules, and a pipeline
// is always free to ignore validator warnings entirely.
type SyntaxRule struct {
	// Type is one of the 5 standard X12 syntax-note letters:
	//   "P" Paired/Corequisite    — if ANY position is present, ALL must be.
	//   "C" Conditional           — if Positions[0] is present, all of
	//                               Positions[1:] become required.
	//   "L" List Conditional      — if ANY of Positions[1:] is present,
	//                               Positions[0] becomes required (the
	//                               shape CAS's own per-trio rules use,
	//                               e.g. "if an amount is present, its
	//                               reason code must be too").
	//   "R" Required (at least one) — at least one of Positions must be present.
	//   "E" Exclusion             — at most one of Positions may be present.
	Type      string   `json:"type"`
	Positions []string `json:"positions"` // element positions this rule relates, e.g. ["06","07"]

	// Source is empty for every OOB-schema-loaded rule (the on-disk JSON
	// never sets it) and "custom" for a rule an edi.validate step's own
	// customRules config appends at Execute() time — never mutating the
	// loaded spec itself (see edi_validate_executor.go's specWithCustomRules).
	// Purely informational: edi/validator.evaluateSyntaxRule treats every
	// rule identically regardless of Source; only edi/validator.Issue.Source
	// (copied straight from here) lets the UI distinguish which fired.
	Source string `json:"source,omitempty"`
}

// Repeats01 reports whether this segment may occur more than once
// consecutively at a single loop/header/trailer position.
func (s *X12SegmentDef) RepeatsMultiple() bool {
	return s != nil && s.MaxUse == ">1"
}

// IsRequired reports whether this segment must be present at its position.
func (s *X12SegmentDef) IsRequired() bool {
	return s != nil && s.Usage == "required"
}

// X12LoopDef is one loop: an ordered list of segment references plus nested
// child loops. Loop relations ARE the Loops field itself — a parent loop's
// own child-loop list, recursing to unbounded depth. X12 loop nesting is a
// strict containment tree (a loop has exactly one parent), not a graph, so
// parent-to-children pointers fully express every relation that exists; no
// separate "relation" type is needed.
type X12LoopDef struct {
	ID         string        `json:"id"`             // e.g. "2100" — the addressing key
	Name       string        `json:"name,omitempty"` // display only
	Repeat     string        `json:"repeat,omitempty"` // "1" | ">1" | "1..999" — cardinality + validator input
	SegmentIDs []string      `json:"segmentIds,omitempty"` // ordered segment ID references into the shared library, strict order — matched BEFORE Loops
	Loops      []*X12LoopDef `json:"loops,omitempty"`      // nested child loops (2100 -> 2110)

	// TrailerSegmentIDs are this loop's OWN segments that come AFTER its
	// nested Loops close — the loop-level counterpart to
	// X12TransactionSetDef's own Header/Trailer split, one level down. Most
	// loops don't need this (a loop's own segments all precede its
	// children — 835 never needed it), but some genuinely do: 999's own
	// 2000 loop is AK2, then a repeating 2100 child loop, THEN IK5 — a
	// segment that structurally closes the loop out, not one that opens it.
	// Empty for the overwhelming majority of loops.
	TrailerSegmentIDs []string `json:"trailerSegmentIds,omitempty"`

	// TriggerDiscriminator disambiguates SIBLING loops that share the exact
	// same trigger segment ID — e.g. 837's own provider-role loops (2310A-F:
	// Referring/Rendering/Service Facility/Supervising/Ambulance Pick-up/
	// Drop-off for professional; Attending/Operating/Other Operating/
	// Rendering/Service Facility/Referring for institutional) are ALL
	// triggered by a bare "NM1", distinguished in real data only by NM1-01's
	// own value (Entity Identifier Code). Without this, matchLoops' own
	// schema-declaration-order matching (loop_engine.go's own doc comment)
	// silently mis-assigns a token to whichever earlier-declared, not-yet-
	// consumed sibling comes first — correct ONLY when every present sibling
	// happens to appear in the data in the exact order the schema declares
	// them, which real 837 files routinely violate (a claim might have an
	// Attending provider and a Service Facility but no Operating/Rendering
	// physician in between). Nil for the overwhelming majority of loops —
	// this was explicitly anticipated and deferred in loop_engine.go's own
	// original doc comment ("if a future transaction set ever needs one...
	// add an explicit element-value discriminator then, not now") until a
	// real transaction set (837, via real-sample testing) actually needed
	// it. See matchLoops' own doc comment for the two-pass matching
	// algorithm this enables.
	TriggerDiscriminator *X12LoopTriggerDiscriminator `json:"triggerDiscriminator,omitempty"`

	// Wrapper marks this loop as WRAPPED: a single Start segment (e.g. "LS")
	// written/matched ONCE before the FULL repeating set of this loop's own
	// instances (not once per instance, unlike SegmentIDs/TrailerSegmentIDs),
	// and a single End segment (e.g. "LE") once after the last instance —
	// X12's own generic "Loop Header"/"Loop Trailer" bracket, used where a
	// loop needs an explicit boundary marker its own trigger segment alone
	// can't provide (271's own loop 2120, Benefit Related Entity, is the
	// motivating case: NM1-triggered, repeat up to 23, needing LS/LE to mark
	// where the WHOLE set starts/ends since NM1 alone is already a trigger
	// shared by other sibling loops at the same nesting level). Nil for every
	// other loop in this schema — this is a narrow, opt-in mechanism, not a
	// replacement for TriggerDiscriminator/TrailerSegmentIDs, which solve
	// different problems (sibling disambiguation, per-instance trailing
	// segments) at the SAME structural level SegmentIDs already covers.
	Wrapper *X12LoopWrapperDef `json:"wrapper,omitempty"`
}

// X12LoopWrapperDef names the Start/End segment IDs bracketing one loop's
// WHOLE repeating instance set — see X12LoopDef.Wrapper's own doc comment.
// Both IDs must already exist in the shared segment library (LS/LE are
// generic, content-free "loop header"/"loop trailer" segments — LS01/LE01
// simply echo the wrapped loop's own numeric ID, e.g. "LS*2120~"/"LE*2120~" —
// modeled the same way any other shared segment is, no special-casing here).
type X12LoopWrapperDef struct {
	Start string `json:"start"` // e.g. "LS"
	End   string `json:"end"`   // e.g. "LE"
}

// X12LoopTriggerDiscriminator names one element (by its own canonical Key,
// resolved against the trigger segment's own X12SegmentDef.Elements at match
// time — never a raw numeric position) and the value it must equal for this
// loop to claim a given trigger-segment occurrence.
type X12LoopTriggerDiscriminator struct {
	ElementKey string `json:"elementKey"`
	Value      string `json:"value"`
}

// RepeatsMultiple reports whether this loop may occur more than once at its
// position in the parent loop/transaction set.
func (l *X12LoopDef) RepeatsMultiple() bool {
	return l != nil && l.Repeat != "" && l.Repeat != "1"
}

// X12TransactionSetDef is pure composition — header segments, a loop tree,
// and trailer segments, all referencing the shared segment library by ID.
// This is the ONLY place one transaction set (835) differs from another
// (837): everything it references is shared.
//
// TransactionSetID is the schema's own registry key — for a transaction set
// with only one real-world variant (835, 999) this equals the raw X12 ST01
// value; for one with multiple variants sharing the same ST01 (837P/837I are
// both literally "837") it's the human-legible disambiguated form ("837P"),
// and STTransactionSetID carries the real ST01 value to write/expect on the
// wire. FunctionalIdentifierCode (GS01) and VersionReleaseIndustryCode
// (GS08) are the other two envelope-level values that vary by transaction
// set, previously hardcoded as 835-only fixedValues on the shared
// envelope.json/segments/ST.json — see EffectiveST01's own doc comment for
// why ST01 alone needed a second field where GS01/GS08 didn't.
type X12TransactionSetDef struct {
	TransactionSetID           string        `json:"transactionSetId"`             // e.g. "835", or "837P" for a disambiguated variant
	STTransactionSetID         string        `json:"stTransactionSetId,omitempty"` // the raw ST01 value, e.g. "837" — only set when it differs from TransactionSetID
	FunctionalIdentifierCode   string        `json:"functionalIdentifierCode,omitempty"`   // GS01, e.g. "HP" (835), "HC" (837), "FA" (999)
	VersionReleaseIndustryCode string        `json:"versionReleaseIndustryCode,omitempty"` // GS08, e.g. "005010X221A1"
	Version                    string        `json:"version,omitempty"`           // e.g. "005010X221A1" — documentation only, distinct from GS08 above
	HeaderSegmentIDs           []string      `json:"headerSegmentIds,omitempty"`  // ST, BPR, TRN, REF, DTM — references into the shared library
	Loops                      []*X12LoopDef `json:"loops,omitempty"`             // top-level loops: 1000A, 1000B, 2000, ...
	TrailerSegmentIDs          []string      `json:"trailerSegmentIds,omitempty"` // PLB, SE
}

// EffectiveST01 returns the raw value to write/expect in ST01 on the wire —
// STTransactionSetID when the schema set one (a disambiguated variant like
// 837P/837I, whose own TransactionSetID is more specific than the real X12
// value), otherwise TransactionSetID itself (835, 999 — single-variant sets
// where the registry key already IS the real ST01 value).
func (t *X12TransactionSetDef) EffectiveST01() string {
	if t == nil {
		return ""
	}
	if t.STTransactionSetID != "" {
		return t.STTransactionSetID
	}
	return t.TransactionSetID
}

// X12EnvelopeDef is the ISA/GS/GE/IEA shape, orthogonal to the transaction-set
// layers above and shared across every transaction set.
type X12EnvelopeDef struct {
	ISA []*X12ElementDef `json:"isa"`
	GS  []*X12ElementDef `json:"gs"`
	GE  []*X12ElementDef `json:"ge"`
	IEA []*X12ElementDef `json:"iea"`
}

// X12SpecDef is the fully-loaded, resolved schema: the shared segment
// library plus every registered transaction set and the shared envelope.
type X12SpecDef struct {
	SpecVersion     string
	Segments        map[string]*X12SegmentDef       // the resolved shared library, keyed by ID
	TransactionSets map[string]*X12TransactionSetDef // keyed by transaction set ID, e.g. "835"
	Envelope        *X12EnvelopeDef
}
