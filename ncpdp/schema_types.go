// ncpdp/schema_types.go
//
// NCPDP SCRIPT (pharmacy e-prescribing) is XML, structurally closer to this
// codebase's CDA engine than to its EDI X12 engine: elements are uniquely
// named at each tree position (<Patient>, <Pharmacy>, <Prescriber>,
// <MedicationPrescribed>), not generic RIM tags requiring predicate
// disambiguation, and there is no classCode/moodCode/templateId boilerplate
// concept at all. So this schema is deliberately much thinner than
// cda/schema_types.go — just two building blocks:
//
//   - NCPDPGroupDef: a reusable, named structural element (Name, Address,
//     CommunicationNumbers, Patient, Prescriber, MedicationPrescribed, ...)
//     — the segment-library equivalent, authored once and referenced by
//     composition from every group/transaction that needs it (mirrors
//     edi/schema_types.go's X12SegmentDef and cda/schemas/.../entries/*.json
//     template reuse — adding a new group is a pure schema-data change).
//   - NCPDPFieldDef: one leaf value, addressed by an xmlpath-grammar path
//     RELATIVE TO its enclosing group's own element (the exact same "tag",
//     "tag/@attr", "tag[@attr='value']" grammar xmlpath.WriteAtXPath/
//     TryFindAtXPath already implement) — so both the parser and the builder
//     reuse ezhealthkonnect/xmlpath directly instead of a bespoke NCPDP
//     path-resolution mechanism. A field whose OWN element also carries an
//     attribute (e.g. Header's <To Qualifier="P">6557744</To>) is simply two
//     ordinary field entries: one with XPath "To" (sets text) and one with
//     XPath "To/@Qualifier" (sets the attribute) — both resolve to the SAME
//     bare-tag child by xmlpath's own first-match semantics, no special
//     casing needed.
package ncpdp

// NCPDPFieldDef is one leaf value within a group, addressed by an
// xmlpath-grammar path relative to the enclosing group's own element.
type NCPDPFieldDef struct {
	Key      string `json:"key"`      // canonical field key used in ParseResult/BuildInput JSON
	XPath    string `json:"xpath"`    // xmlpath.WriteAtXPath/TryFindAtXPath grammar, relative to the group's anchor
	Name     string `json:"name"`     // human-readable label, for schema-introspection UI
	DataType string `json:"dataType"` // "string" | "date" | "time" | "decimal" | "code"
	Required bool   `json:"required"`
	// FallbackXPath is tried on PARSE ONLY when XPath resolves to nothing —
	// real-world NCPDP SCRIPT senders sometimes encode a "coded element" as
	// an attribute+text-content leaf (e.g. `<ProductCode Qualifier="ND">
	// 00093505601</ProductCode>`) rather than this schema's own canonical
	// nested `<Code>`/`<Qualifier>` sub-element form (confirmed correct
	// against dgoradia/ncpdp's own real sample) — found via a second real,
	// independent source (cosyte/ncpdp's own test fixtures) using BOTH
	// shapes, even within the SAME message for sibling elements (DrugCoded's
	// own ProductCode vs. DrugDBCode). The special value "." means "the
	// anchor element's own text content" (self-reference — parser.go's
	// extractValue handles this directly, not via xmlpath's own segment
	// walk, which has no "self" concept). Build/serialize direction is
	// UNCHANGED — this engine always WRITES the nested form, which both
	// real sources agree is valid; the fallback exists purely for read-side
	// tolerance of a real, confirmed alternate wire shape.
	FallbackXPath string `json:"fallbackXPath,omitempty"`
}

// NCPDPGroupRef is one reference from a group (or transaction) to a shared
// group definition, composed at a specific position in the tree.
type NCPDPGroupRef struct {
	Key        string `json:"key"`                  // canonical key used in ParseResult/BuildInput JSON at this position
	GroupKey   string `json:"groupKey"`              // reference into NCPDPSpecDef.Groups
	XMLElement string `json:"xmlElement,omitempty"`  // overrides the referenced group's own XMLElement when this position uses a different tag (e.g. "Sender"/"Receiver" both referencing the same TertiaryIdentificationHolder shape)
	Required   bool   `json:"required"`
	Repeatable bool   `json:"repeatable"` // true => this position holds an array (e.g. Diagnosis, COO)
}

// NCPDPGroupDef is one reusable, named structural XML element — the
// segment-library equivalent. A group's own Fields are addressed relative
// to ITS element; nested Groups compose further structure beneath it.
type NCPDPGroupDef struct {
	Key        string          `json:"key"`
	XMLElement string          `json:"xmlElement"` // default tag name when this group is referenced without an override
	Name       string          `json:"name"`
	Fields     []NCPDPFieldDef `json:"fields,omitempty"`
	Groups     []NCPDPGroupRef `json:"groups,omitempty"`
	// ElementOrder, when set, is the exact child-tag write order for this
	// group's own element — several real NCPDP groups interleave plain
	// fields and nested groups (e.g. HumanPatient: Name, Gender,
	// DateOfBirth, Address, CommunicationNumbers), which this schema's own
	// separate Fields/Groups arrays can't express positionally. The builder
	// applies this via xmlpath.ReorderChildrenByTag as a post-hoc pass,
	// mirroring cda/builder's identical solution to the identical problem
	// ("construction order != schema order"). Omitted when Fields/Groups
	// already write in valid order by construction (no interleaving).
	ElementOrder []string `json:"elementOrder,omitempty"`
	SourceRefs   []string `json:"sourceRefs,omitempty"` // provenance — which real sample/reference this group's shape was sourced from
}

// FieldByKey returns the field def matching key, or nil.
func (g *NCPDPGroupDef) FieldByKey(key string) *NCPDPFieldDef {
	if g == nil {
		return nil
	}
	for i := range g.Fields {
		if g.Fields[i].Key == key {
			return &g.Fields[i]
		}
	}
	return nil
}

// GroupRefByKey returns the group-ref matching key, or nil.
func (g *NCPDPGroupDef) GroupRefByKey(key string) *NCPDPGroupRef {
	if g == nil {
		return nil
	}
	for i := range g.Groups {
		if g.Groups[i].Key == key {
			return &g.Groups[i]
		}
	}
	return nil
}

// NCPDPTransactionDef is the root composition for one SCRIPT transaction
// type (NewRx, CancelRx, CancelRxResponse, RxChangeRequest,
// RxChangeResponse, ...) — pure composition of shared groups, exactly like
// edi/schema_types.go's X12TransactionSetDef composes shared segments/loops.
// Adding a new transaction type (a later phase) never needs new Go code.
type NCPDPTransactionDef struct {
	Key          string          `json:"key"` // e.g. "NewRx" — also the <Body> child element name
	Name         string          `json:"name"`
	Fields       []NCPDPFieldDef `json:"fields,omitempty"`
	Groups       []NCPDPGroupRef `json:"groups,omitempty"`
	ElementOrder []string        `json:"elementOrder,omitempty"` // see NCPDPGroupDef.ElementOrder
	SourceRefs   []string        `json:"sourceRefs,omitempty"`
}

// FieldByKey returns the field def matching key, or nil.
func (t *NCPDPTransactionDef) FieldByKey(key string) *NCPDPFieldDef {
	if t == nil {
		return nil
	}
	for i := range t.Fields {
		if t.Fields[i].Key == key {
			return &t.Fields[i]
		}
	}
	return nil
}

// GroupRefByKey returns the group-ref matching key, or nil.
func (t *NCPDPTransactionDef) GroupRefByKey(key string) *NCPDPGroupRef {
	if t == nil {
		return nil
	}
	for i := range t.Groups {
		if t.Groups[i].Key == key {
			return &t.Groups[i]
		}
	}
	return nil
}

// NCPDPMessageAttrs are the root <Message> element's own fixed attributes
// (DatatypesVersion/TransportVersion/TransactionDomain/TransactionVersion/
// StructuresVersion/ECLVersion) — the same across every transaction type for
// one SCRIPT version, so defined once at the spec level rather than per
// transaction.
type NCPDPMessageAttrs struct {
	DatatypesVersion string `json:"datatypesVersion"`
	TransportVersion string `json:"transportVersion"`
	TransactionDomain string `json:"transactionDomain"`
	TransactionVersion string `json:"transactionVersion"`
	StructuresVersion string `json:"structuresVersion"`
	ECLVersion       string `json:"eclVersion"`
}

// NCPDPSpecDef is the fully-resolved in-memory spec for one SCRIPT version.
type NCPDPSpecDef struct {
	Version      string                          `json:"version"`
	MessageAttrs NCPDPMessageAttrs                `json:"messageAttrs"`
	HeaderGroupKey string                        `json:"headerGroupKey"` // key into Groups for the shared <Header> shape
	Groups       map[string]*NCPDPGroupDef        `json:"-"`
	Transactions map[string]*NCPDPTransactionDef  `json:"-"`
}

// Header returns the resolved header group, or nil if not configured.
func (s *NCPDPSpecDef) Header() *NCPDPGroupDef {
	if s == nil || s.HeaderGroupKey == "" {
		return nil
	}
	return s.Groups[s.HeaderGroupKey]
}
