// services/executors/cda_element_translation_table.go
//
// The field-by-field translation table cda_element_translation.go's walker
// consults. Built exhaustively by reading every struct in cda/document/
// types.go and every parse function in cda/document/datatypes.go +
// entry_parser.go + header_parser.go (not sampled/guessed) -- verifying, for
// each typed field, exactly which raw XML shape the parser actually reads it
// from. Two root kinds a caller can start a walk from: "CDAEntry" (every
// row.Scope/row.SourcePath resolution -- declarative_engine.go's applyRow)
// and "CDAHeader" (declarative_engine.go's BuildHeaderResourceWithCoverage
// Prefix, resolving one of the 4 real header constructs CDA Coverage
// Audit's inventory also walks -- services/cda_coverage/inventory.go's
// buildHeaderInventory). CDAHeader/CDAPatient/CDACustodian/etc. were
// originally out of scope (see project history) until that extension.
//
// Three real, verified kinds of typed-tree vs raw-XML mismatch (see this
// package's cda_element_translation.go for the walker that consumes this
// table):
//
//  1. Pluralized-collection renames (xlRenamed): typed JSON tags pluralize
//     Go-idiomatically ("entryRelationships", "participants", "performers");
//     raw XML tags are always singular ("entryRelationship", "participant",
//     "performer") -- repetition is expressed by GenericXMLToJSON's
//     auto-array-on-repeat convention, never the tag name. Several are
//     IRREGULAR, not simple depluralization -- a generic depluralizer would
//     silently produce a wrong path for these:
//       - CDAAddress.Addresses ("addresses")   -> raw tag "addr"
//       - CDAAddress.StreetLines               -> raw tag "streetAddressLine"
//       - CDAAddress.ValidTime                 -> raw tag "useablePeriod"
//     This is exactly why this file is a hand-verified table, not a
//     heuristic.
//
//  2. Synthetic wrapper segments with no raw counterpart, resolved from DATA
//     not the path string (xlActChild / xlRenamedActChild): a raw
//     <entryRelationship> directly contains its act-type child (<observation
//     >/<act>/<organizer>/<substanceAdministration>/<procedure>/<encounter>/
//     <supply> -- entry_parser.go's entryActTypes) with NO wrapper element,
//     so CDAEntryRelationship.Entry's ".entry" typed segment consumes no raw
//     XML level at all (xlActChild). CDAEntry.Components DOES cross a real
//     "<component>" wrapper first (parseComponents' `el.SelectElements(
//     "component")`, each holding one act, same extractAct() call
//     entryRelationships uses) -- xlRenamedActChild. Both need the ALREADY-
//     RESOLVED node's own EntryType value to know which literal tag applies;
//     it cannot be derived from the path string.
//
//  3. Datatype attribute-flattening (xlAttr / xlInlineStruct): CDACode/
//     CDAQuantity/CDATime's own fields (.Code, .DisplayName, .Value, .Unit,
//     ...) are read as ATTRIBUTES directly off whatever element datatypes.go
//     was handed (parseCD/parsePQ/parseTS all read SelectAttrValue on their
//     input el, never a further nested child) -- xlAttr, terminal, no path
//     growth (element-granularity tracking never needs the exact attribute
//     name, only that ITS OWNING ELEMENT was read). CDAValue is the sharpest
//     example: ALL of .Code/.Quantity/.Text/.Boolean/.Integer/.Real/
//     .TimeRange read from the SAME single <value> element (parseValue's
//     xsi:type switch is called with el = the value element itself, never a
//     child) -- every one of those sub-structs is xlInlineStruct, not
//     xlSame, so "value.code.code"/"value.quantity.value"/"value.integer"
//     all resolve to the identical XML path "value" no matter which
//     xsi:type-specific field a mapping row happens to read.
//     CDATimeRange is the one exception worth flagging explicitly: .Low/
//     .High ARE real nested child elements (parseIVL_TS's `el.SelectElement(
//     "low")`) even when CDATimeRange itself is reached via an inline path
//     (e.g. "value.timeRange.low.value" -- "timeRange" inline-flattens onto
//     the value element, but "low" is a genuine "<low>" child OF that same
//     value element, correctly modeled by CDATimeRange's own table entry
//     being reused identically regardless of how CDATimeRange was reached).
package executors

// cdaFieldTranslation classifies how one typed-tree field name relates to
// the raw XML mirror's shape for the same underlying data. See this file's
// header comment for the three real mismatch kinds each of these encodes.
type cdaFieldTranslation int

const (
	// xlSame: raw XML tag equals the typed field's own key; a real,
	// separately-addressable child element -- xmlPath grows by one segment
	// (or, when multiSegment is set, several -- see cdaFieldRule.multiSegment).
	xlSame cdaFieldTranslation = iota
	// xlRenamed: same as xlSame, but the raw XML tag differs from the typed
	// key (cdaFieldRule.xmlKey names it).
	xlRenamed
	// xlAttr: terminal scalar -- an XML attribute OR the element's own text,
	// element-granularity tracking doesn't distinguish which. xmlPath does
	// not grow; no nextKind (nothing can be nested further under a scalar).
	xlAttr
	// xlInlineStruct: a sub-struct whose OWN fields live on the SAME raw XML
	// node as their parent (e.g. CDAValue.Code's fields are attributes
	// directly on <value>, not a nested <code> child) -- xmlPath does not
	// grow, but nextKind switches context so further segments resolve
	// correctly against the (unchanged) current node.
	xlInlineStruct
	// xlActChild: data-dependent, no wrapper element consumed -- see this
	// file's header comment, kind 2. Resolved via the already-resolved
	// node's own EntryType at walk time (cda_element_translation.go's
	// cdaActTypeOf), never from the path string.
	xlActChild
	// xlRenamedActChild: data-dependent, but a real wrapper element (rule.xmlKey,
	// e.g. "component") is consumed FIRST, then the act tag nested inside it.
	xlRenamedActChild
	// xlUnmapped: no safe translation exists (see this file's header comment
	// for the two real cases: CDATelecom.Type, computed with no raw
	// counterpart; CDAEntry.AdditionalValues, whose original sibling <value>
	// index entry_parser.go doesn't preserve). Callers must skip recording
	// entirely rather than guess -- under-recording only delays a real gap
	// being reported, never fabricates a false one.
	xlUnmapped
)

// cdaFieldRule is the translation for one typed field name within one
// struct-kind context (see cdaElementTranslationTable).
type cdaFieldRule struct {
	// xmlKey is the raw XML tag/attribute name; empty means identical to the
	// typed field's own key (the xlSame case). For multiSegment rules, this
	// is a complete dot-separated raw path (e.g. "precondition.criterion.value"),
	// appended verbatim.
	xmlKey string
	kind   cdaFieldTranslation
	// nextKind is the struct-kind context to use for any path segment nested
	// under this field -- meaningful for xlSame/xlRenamed/xlInlineStruct/
	// xlActChild/xlRenamedActChild only (xlActChild/xlRenamedActChild always
	// resolve to "CDAEntry" regardless of what's set here, since the child is
	// itself a full clinical act).
	nextKind string
	// firstOnly marks a typed field that is a singular alias for the FIRST
	// occurrence of a raw tag that can repeat (e.g. CDAEntry.EffectiveTime
	// aliases EffectiveTimes[0] -- entry_parser.go: "entry.EffectiveTime =
	// entry.EffectiveTimes[0].Range"). The translator must emit an explicit
	// index 0, not a bare/ambiguous segment, since the raw mirror's own
	// "effectiveTime" key legitimately holds an array once there's more than
	// one occurrence.
	firstOnly bool
	// multiSegment marks a rule whose xmlKey is a complete dotted raw path
	// (several real XML levels collapsed into one CDAEntry-level typed
	// field, e.g. Precondition/ReferenceRangeText/RelatedSubjectCode/Product)
	// rather than a single renamed segment. No per-level index is applied --
	// every documented real occurrence of this shape is singular/unrepeated
	// at each intermediate level.
	multiSegment bool
}

// cdaKnownActTypes mirrors cda/document/entry_parser.go's entryActTypes
// exactly (duplicated, not imported, to avoid a services/executors ->
// cda/document import -- this package already sits below cda/document in
// the dependency graph; the seven-element list is small and stable enough
// that duplication is safer than introducing a cycle-risking import for it).
var cdaKnownActTypes = map[string]struct{}{
	"observation":             {},
	"act":                     {},
	"organizer":               {},
	"substanceAdministration": {},
	"procedure":               {},
	"encounter":               {},
	"supply":                  {},
}

func isKnownCDAActType(entryType string) bool {
	_, ok := cdaKnownActTypes[entryType]
	return ok
}

// lookupCDAFieldRule looks up kind's rule for key. Returns hasRule=false for
// any struct-kind/field combination not in the table -- the walker treats
// that identically to xlUnmapped (Translatable=false), so an incomplete
// table entry degrades safely rather than guessing.
func lookupCDAFieldRule(kind, key string) (cdaFieldRule, bool) {
	fields, ok := cdaElementTranslationTable[kind]
	if !ok {
		return cdaFieldRule{}, false
	}
	rule, ok := fields[key]
	return rule, ok
}

// cdaElementTranslationTable is keyed by struct-kind name (the exact Go type
// name in cda/document/types.go), then by that struct's own JSON tag names.
// Every rootKind a real caller uses (see declarative_engine.go's applyRow,
// always "CDAEntry") and every kind reachable from it is covered.
var cdaElementTranslationTable = map[string]map[string]cdaFieldRule{
	"CDAEntry": {
		"id":                 {kind: xlSame, nextKind: "CDAII"},
		"code":               {kind: xlSame, nextKind: "CDACode"},
		"text":               {kind: xlSame},
		"statusCode":         {kind: xlSame},
		"interpretationCode": {kind: xlSame, nextKind: "CDACode"},
		"referenceRangeText": {xmlKey: "referenceRange.observationRange.text", kind: xlSame, multiSegment: true},
		"effectiveTime":      {xmlKey: "effectiveTime", kind: xlSame, nextKind: "CDATimeRange", firstOnly: true},
		"effectiveTimes":     {xmlKey: "effectiveTime", kind: xlRenamed, nextKind: "CDAEffectiveTimeEntry"},
		"value":              {xmlKey: "value", kind: xlSame, nextKind: "CDAValue", firstOnly: true},
		"additionalValues":   {kind: xlUnmapped}, // see file header, kind 3 note + this file's package doc
		"participants":       {xmlKey: "participant", kind: xlRenamed, nextKind: "CDAParticipant"},
		"components":         {xmlKey: "component", kind: xlRenamedActChild},
		"entryRelationships": {xmlKey: "entryRelationship", kind: xlRenamed, nextKind: "CDAEntryRelationship"},
		"entryType":          {kind: xlAttr},
		"classCode":          {kind: xlAttr},
		"moodCode":           {kind: xlAttr},
		"nullFlavor":         {kind: xlAttr},
		"negationInd":        {kind: xlAttr},
		"templateIds":        {xmlKey: "templateId", kind: xlRenamed},
		"repeatNumber":       {kind: xlSame},
		"authors":            {xmlKey: "author", kind: xlRenamed, nextKind: "CDAAuthor"},
		"performers":         {xmlKey: "performer", kind: xlRenamed, nextKind: "CDAPerformer"},
		"informants":         {xmlKey: "informant", kind: xlRenamed, nextKind: "CDAInformant"},
		"consumable":         {kind: xlSame, nextKind: "CDAConsumable"},
		"routeCode":          {kind: xlSame, nextKind: "CDACode"},
		"doseQuantity":       {kind: xlSame, nextKind: "CDAQuantity"},
		"rateQuantity":       {kind: xlSame, nextKind: "CDAQuantity"},
		"approachSiteCode":         {kind: xlSame, nextKind: "CDACode"},
		"administrationUnitCode":   {kind: xlSame, nextKind: "CDACode"},
		"maxDoseQuantity":          {kind: xlSame, nextKind: "CDARatio"},
		"precondition":             {xmlKey: "precondition.criterion.value", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"quantity":                 {kind: xlSame, nextKind: "CDAQuantity"},
		"product":                  {xmlKey: "product.manufacturedProduct", kind: xlSame, nextKind: "CDAManufacturedProduct", multiSegment: true},
		"targetSiteCode":           {kind: xlSame, nextKind: "CDACode"},
		"relatedSubjectCode":       {xmlKey: "subject.relatedSubject.code", kind: xlSame, nextKind: "CDACode", multiSegment: true},
	},
	"CDAEntryRelationship": {
		"typeCode":     {kind: xlAttr},
		"inversionInd": {kind: xlAttr},
		"entry":        {kind: xlActChild}, // no wrapper -- see file header, kind 2
	},
	"CDACode": {
		"code":           {kind: xlAttr},
		"displayName":    {kind: xlAttr},
		"codeSystem":     {kind: xlAttr},
		"codeSystemName": {kind: xlAttr},
		"nullFlavor":     {kind: xlAttr},
		"originalText":   {kind: xlSame},
		"translations":   {xmlKey: "translation", kind: xlRenamed, nextKind: "CDACode"},
	},
	"CDAValue": {
		// Every field here shares the SAME raw <value> element -- xsi:type
		// discriminates which is populated, but none of them advance the XML
		// path (see file header, kind 3).
		"type":       {kind: xlAttr},
		"code":       {kind: xlInlineStruct, nextKind: "CDACode"},
		"quantity":   {kind: xlInlineStruct, nextKind: "CDAQuantity"},
		"text":       {kind: xlAttr},
		"boolean":    {kind: xlAttr},
		"integer":    {kind: xlAttr},
		"real":       {kind: xlAttr},
		"timeRange":  {kind: xlInlineStruct, nextKind: "CDATimeRange"},
		"nullFlavor": {kind: xlAttr},
	},
	"CDAQuantity": {
		"value":      {kind: xlAttr},
		"unit":       {kind: xlAttr},
		"nullFlavor": {kind: xlAttr},
	},
	"CDATime": {
		"value":      {kind: xlAttr},
		"operator":   {kind: xlAttr},
		"nullFlavor": {kind: xlAttr},
	},
	"CDATimeRange": {
		"low":        {kind: xlSame, nextKind: "CDATime"},
		"high":       {kind: xlSame, nextKind: "CDATime"},
		"value":      {kind: xlInlineStruct, nextKind: "CDATime"}, // CDATimeRange.Value.* reads the SAME element's own @value/@operator -- see datatypes.go parseIVL_TS/parseValue(TS)
		"nullFlavor": {kind: xlAttr},
	},
	"CDAII": {
		"root":                   {kind: xlAttr},
		"extension":              {kind: xlAttr},
		"assigningAuthorityName": {kind: xlAttr},
		"displayName":            {kind: xlAttr},
		"nullFlavor":             {kind: xlAttr},
	},
	"CDAEffectiveTimeEntry": {
		"xsiType": {kind: xlAttr},
		"range":   {kind: xlInlineStruct, nextKind: "CDATimeRange"}, // parseIVL_TS(etEl) reads the effectiveTime element itself
		"period":  {kind: xlSame, nextKind: "CDAQuantity"},
	},
	"CDAParticipant": {
		"typeCode":        {kind: xlAttr},
		"functionCode":    {kind: xlSame, nextKind: "CDACode"},
		"time":            {kind: xlSame, nextKind: "CDATimeRange"},
		"participantRole": {kind: xlSame, nextKind: "CDAParticipantRole"},
	},
	"CDAParticipantRole": {
		"classCode":     {kind: xlAttr},
		"ids":           {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"code":          {kind: xlSame, nextKind: "CDACode"},
		"addresses":     {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
		"telecoms":      {xmlKey: "telecom", kind: xlRenamed, nextKind: "CDATelecom"},
		"playingEntity": {kind: xlSame, nextKind: "CDAPlayingEntity"},
		"scopingEntity": {kind: xlSame, nextKind: "CDAEntity"},
	},
	"CDAPlayingEntity": {
		"classCode": {kind: xlAttr},
		"code":      {kind: xlSame, nextKind: "CDACode"},
		"names":     {xmlKey: "name", kind: xlRenamed, nextKind: "CDAName"},
		"desc":      {kind: xlSame},
	},
	"CDAEntity": {
		"ids":  {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"code": {kind: xlSame, nextKind: "CDACode"},
		"desc": {kind: xlSame},
	},
	"CDAAuthor": {
		"time":           {kind: xlSame, nextKind: "CDATime"},
		"assignedAuthor": {kind: xlSame, nextKind: "CDAAssignedAuthor"},
	},
	"CDAAssignedAuthor": {
		"ids":                     {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"addresses":               {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
		"telecoms":                {xmlKey: "telecom", kind: xlRenamed, nextKind: "CDATelecom"},
		"assignedPerson":          {kind: xlSame, nextKind: "CDAPerson"},
		"assignedAuthoringDevice": {kind: xlSame, nextKind: "CDADevice"},
		"representedOrganization": {kind: xlSame, nextKind: "CDAOrganization"},
	},
	"CDAPerson": {
		"names": {xmlKey: "name", kind: xlRenamed, nextKind: "CDAName"},
	},
	"CDADevice": {
		"softwareName":          {kind: xlSame},
		"manufacturerModelName": {kind: xlSame},
	},
	"CDAOrganization": {
		"ids":       {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"names":     {xmlKey: "name", kind: xlRenamed},
		"addresses": {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
		"telecoms":  {xmlKey: "telecom", kind: xlRenamed, nextKind: "CDATelecom"},
	},
	"CDAPerformer": {
		"typeCode":       {kind: xlAttr},
		"functionCode":   {kind: xlSame, nextKind: "CDACode"},
		"time":           {kind: xlSame, nextKind: "CDATimeRange"},
		"assignedEntity": {kind: xlSame, nextKind: "CDAAssignedEntity"},
	},
	"CDAInformant": {
		"assignedEntity": {kind: xlSame, nextKind: "CDAAssignedEntity"},
	},
	"CDAAssignedEntity": {
		"ids":                     {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"code":                    {kind: xlSame, nextKind: "CDACode"},
		"addresses":               {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
		"telecoms":                {xmlKey: "telecom", kind: xlRenamed, nextKind: "CDATelecom"},
		"assignedPerson":          {kind: xlSame, nextKind: "CDAPerson"},
		"representedOrganization": {kind: xlSame, nextKind: "CDAOrganization"},
	},
	"CDAName": {
		"use":       {kind: xlAttr},
		"prefix":    {kind: xlSame},
		"given":     {kind: xlSame},
		"family":    {kind: xlSame},
		"suffix":    {kind: xlSame},
		"validTime": {kind: xlSame, nextKind: "CDATimeRange"},
	},
	"CDAAddress": {
		"use":         {kind: xlAttr},
		"streetLines": {xmlKey: "streetAddressLine", kind: xlRenamed}, // IRREGULAR -- see file header, kind 1
		"city":        {kind: xlSame},
		"state":       {kind: xlSame},
		"postalCode":  {kind: xlSame},
		"country":     {kind: xlSame},
		"county":      {kind: xlSame},
		"validTime":   {xmlKey: "useablePeriod", kind: xlRenamed, nextKind: "CDATimeRange"}, // IRREGULAR
		"nullFlavor":  {kind: xlAttr},
	},
	"CDATelecom": {
		"use":   {kind: xlAttr},
		"value": {kind: xlAttr},
		"type":  {kind: xlUnmapped}, // computed from @value's prefix, see file header
	},
	"CDAConsumable": {
		"manufacturedProduct": {kind: xlSame, nextKind: "CDAManufacturedProduct"},
	},
	"CDAManufacturedProduct": {
		"ids":                      {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"manufacturedMaterial":     {kind: xlSame, nextKind: "CDAMaterial"},
		"manufacturerOrganization": {kind: xlSame, nextKind: "CDAOrganization"},
	},
	"CDAMaterial": {
		"code":          {kind: xlSame, nextKind: "CDACode"},
		"name":          {kind: xlSame},
		"lotNumberText": {kind: xlSame},
	},
	"CDARatio": {
		"numerator":   {kind: xlSame, nextKind: "CDAQuantity"},
		"denominator": {kind: xlSame, nextKind: "CDAQuantity"},
	},

	// ======================================================================
	// Document header (cda/document/header_parser.go) -- added for CDA
	// Coverage Audit's header-field extension. "CDAHeader" is a SECOND root
	// kind (alongside "CDAEntry") a caller can start a translated walk from
	// -- see declarative_engine.go's BuildHeaderResourceWithCoveragePrefix,
	// the only caller that ever passes rootKind="CDAHeader". "authors" is
	// deliberately absent from this table: which raw <author> gets used is
	// resolved by firstAuthorWithPersonIndexed, a Go predicate ("first
	// author with a usable assignedPerson"), never expressible as a plain/
	// wildcard/predicate path segment the way every other field here is.
	// ======================================================================
	"CDAHeader": {
		// header_parser.go:71-72 -- parsePatient's role := nav(root,
		// "recordTarget", "patientRole"): TWO real raw levels collapse into
		// the one typed "patient" field.
		"patient": {xmlKey: "recordTarget.patientRole", kind: xlSame, nextKind: "CDAPatient", multiSegment: true},
		// header_parser.go:215-216 -- parseCustodian's custEl := nav(root,
		// "custodian", "assignedCustodian").
		"custodian": {xmlKey: "custodian.assignedCustodian", kind: xlSame, nextKind: "CDACustodian", multiSegment: true},
		// header_parser.go:290-291 -- parseComponentOf's eeEl := nav(root,
		// "componentOf", "encompassingEncounter").
		"encompassingEncounter": {xmlKey: "componentOf.encompassingEncounter", kind: xlSame, nextKind: "CDAEncounter", multiSegment: true},
	},
	// header_parser.go:71-150 (parsePatient). Ids/Addresses/Telecoms read
	// directly off patientRole (role.SelectElements at header_parser.go:80-
	// 92); every other field reads off the nested <patient> demographics
	// element ONE level deeper (patEl.SelectElement*, header_parser.go:101-
	// 146) -- hence multiSegment with a "patient."-prefixed xmlKey for
	// those, so the translated path grows by "patient[0].<tag>[idx]" rather
	// than a bare "<tag>[idx]" that would collide with patientRole's own
	// direct children.
	"CDAPatient": {
		"ids":           {xmlKey: "id", kind: xlRenamed, nextKind: "CDAII"},
		"addresses":     {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
		"telecoms":      {xmlKey: "telecom", kind: xlRenamed, nextKind: "CDATelecom"},
		"names":         {xmlKey: "patient.name", kind: xlSame, nextKind: "CDAName", multiSegment: true},
		"birthDate":     {xmlKey: "patient.birthTime", kind: xlSame, nextKind: "CDATime", multiSegment: true},
		"gender":        {xmlKey: "patient.administrativeGenderCode", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"deceasedInd":   {xmlKey: "patient.deceasedInd", kind: xlSame, multiSegment: true}, // sdtc:deceasedInd, matched by local name -- header_parser.go:114
		"maritalStatus": {xmlKey: "patient.maritalStatusCode", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"race":          {xmlKey: "patient.raceCode", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"ethnicity":     {xmlKey: "patient.ethnicGroupCode", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"religion":      {xmlKey: "patient.religiousAffiliationCode", kind: xlSame, nextKind: "CDACode", multiSegment: true},
		"languages":     {xmlKey: "patient.languageCommunication", kind: xlSame, nextKind: "CDALanguage", multiSegment: true},
	},
	// header_parser.go:132-147 -- parsePatient's languageCommunication loop.
	// Code and ProficiencyLevel are BOTH irregular renames, not identical to
	// their typed field name -- verified line by line, not assumed.
	"CDALanguage": {
		"code":             {xmlKey: "languageCode", kind: xlRenamed},             // IRREGULAR (header_parser.go:134-135)
		"preferenceInd":    {kind: xlSame},                                        // header_parser.go:137-138
		"modeCode":         {kind: xlSame, nextKind: "CDACode"},                   // header_parser.go:140-141
		"proficiencyLevel": {xmlKey: "proficiencyLevelCode", kind: xlRenamed, nextKind: "CDACode"}, // IRREGULAR (header_parser.go:143-144)
	},
	// header_parser.go:215-224 -- parseCustodian. AssignedCustodian is a
	// SAME-NODE wrapper: nav() (called to resolve "CDAHeader"."custodian"
	// above) already consumed BOTH real raw levels ("custodian" AND
	// "assignedCustodian") before this Go field is even populated -- it has
	// no raw XML level of its own. xlInlineStruct (zero path growth, kind-
	// context switch only), NOT xlSame -- xlSame here would append a
	// phantom second "assignedCustodian[0]" segment that never matches the
	// inventory's real "custodian[0].assignedCustodian[0].
	// representedCustodianOrganization[0]" path.
	"CDACustodian": {
		"assignedCustodian": {kind: xlInlineStruct, nextKind: "CDAAssignedCustodian"},
	},
	"CDAAssignedCustodian": {
		"representedCustodianOrganization": {kind: xlSame, nextKind: "CDAOrganization"},
	},
	// header_parser.go:290-331 -- parseComponentOf. Every field matches its
	// raw tag exactly (no irregular renames in this chain).
	"CDAEncounter": {
		"id":                       {kind: xlSame, nextKind: "CDAII"},
		"effectiveTime":            {kind: xlSame, nextKind: "CDATimeRange"},
		"dischargeDispositionCode": {kind: xlSame, nextKind: "CDACode"},
		"location":                 {kind: xlSame, nextKind: "CDALocation"},
	},
	"CDALocation": {
		"healthCareFacility": {kind: xlSame, nextKind: "CDAHealthCareFacility"},
	},
	"CDAHealthCareFacility": {
		"code":                        {kind: xlSame, nextKind: "CDACode"},
		"location":                    {kind: xlSame, nextKind: "CDAPlace"},
		"serviceProviderOrganization": {kind: xlSame, nextKind: "CDAOrganization"},
	},
	// header_parser.go:312-320 -- CDAPlace.Names is []CDAName (structured,
	// needs nextKind), a DIFFERENT shape than CDAOrganization.Names
	// ([]string, no nextKind, see that entry above) -- verified against
	// types.go, not assumed from the shared field name.
	"CDAPlace": {
		"names":     {xmlKey: "name", kind: xlRenamed, nextKind: "CDAName"},
		"addresses": {xmlKey: "addr", kind: xlRenamed, nextKind: "CDAAddress"},
	},
}
