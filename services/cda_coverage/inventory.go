// services/cda_coverage/inventory.go
//
// Builds the "everything that was actually in the source CCD" ground-truth
// inventory for CDA Coverage Audit, by walking ParsedJSON["xml"] — the
// lossless, schema-agnostic GenericXMLToJSON mirror (cda/document/generic_xml.go)
// — never ParsedJSON["document"]/["sections"], which are curated,
// whitelist-limited views this feature exists to check against (see
// cda_parser_service.go's own doc comment on that distinction).
//
// Two granularities: entry-level (section + entry index, the original
// design) and, per-interface opt-in, element-level (additionally, every
// individual XML element within each entry — never a separate item per
// attribute; @nullFlavor/@codeSystemName etc. fold into their owning
// element, not their own item). Neither ever surfaces clinical values, only
// structural location — see MissedItem's own doc comment (report.go).
package cdacoverage

import (
	"sort"
	"strconv"
	"strings"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/uscdi"
)

// InventoryItem is one section entry, or (when ElementPath is set) one
// specific element within an entry, found in the source document.
type InventoryItem struct {
	// Category is the human clinical/administrative grouping shown in the
	// report — CDASectionDef.USCDIClass for a recognized section, or
	// "Unclassified" for a section this schema has no templateId/LOINC
	// match for at all (itself a useful signal — see BuildInventory).
	Category string

	// SectionKey is the schema key (e.g. "medications") for a recognized
	// section, or empty for an unclassified one.
	SectionKey string

	// SectionTitle is a human-readable label: the schema's DisplayName when
	// recognized, otherwise the section's own raw <title> text (or a
	// templateId-based fallback if even that's missing).
	SectionTitle string

	// EntryIndex is this entry's position within the section, in document
	// order — the same indexing convention every recording call site in
	// cda_coverage_tracker.go's consumers uses.
	EntryIndex int

	// ElementPath is "" for an entry-level item (the original, always-built
	// granularity), or this element's path relative to its entry (e.g.
	// "code", "effectiveTime[0].low[0]") for an element-level item —
	// walkEntryElements' own output, in the identical index-every-segment
	// convention services/executors/cda_element_translation.go's translator
	// independently produces (see that file's own doc comment on why both
	// sides must agree without either needing information only the other
	// has).
	ElementPath string

	// USCDIClasses is the real, ONC-verified USCDI v3 class set this item's
	// SectionKey resolves to via uscdi_v3.json (uscdi.USCDIVocabulary.
	// GetByCDASection) — nil when the vocabulary wasn't supplied, the
	// section is unclassified, or (the common case today) uscdi_v3.json
	// simply doesn't have an entry for this section yet. Deliberately
	// distinct from Category (CDASectionDef.USCDIClass) — see this
	// package's own plan doc on why the schema's own "uscdiClass" field
	// isn't a verified USCDI crosswalk and must never be conflated with
	// this one. A section can resolve to MORE than one class at once (e.g.
	// socialHistory -> Health Status Assessments and Care Plan and Patient
	// Demographics/Information) — always the full set, never collapsed to
	// one, and never narrowed to "the one class THIS entry belongs to"
	// (that would need resolving a raw element path back to a schema
	// CDAFieldDef.Key, which nothing does today).
	USCDIClasses []string
}

// TrackingKey returns the CDAEntryKey (optionally "/elementPath"-suffixed)
// this item would be recorded under if touched — "" for unclassified
// sections, which by construction can never be touched (nothing in the
// pipeline can reference a section by a key it doesn't have).
func (i InventoryItem) TrackingKey() string {
	if i.SectionKey == "" {
		return ""
	}
	key := executors.CDAEntryKey(i.SectionKey, i.EntryIndex)
	if i.ElementPath == "" {
		return key
	}
	return key + "/" + i.ElementPath
}

// BuildInventory walks xmlMirror (ParsedJSON["xml"]) and returns one
// InventoryItem per <entry> found across every <section> in the document's
// structured body, classified via schemaLoader the same way
// cda_parser_service.go's resolveSectionKey classifies sections at parse
// time (templateId first, then LOINC code) — reused here, not
// re-implemented, so classification never silently drifts between parsing
// and auditing. Entry-level granularity only — see BuildInventoryWithGranularity
// for the element-level variant.
func BuildInventory(xmlMirror map[string]interface{}, schemaLoader *cdaSchema.CDASchemaLoader, vocabulary *uscdi.USCDIVocabulary) []InventoryItem {
	return BuildInventoryWithGranularity(xmlMirror, schemaLoader, vocabulary, false)
}

// BuildInventoryWithGranularity is BuildInventory, plus element-level items
// (one per XML element within each entry, not just the entry itself) when
// elementLevel is true. The interface's own cda_coverage_audit_config.
// granularity field controls this — see services/cda_coverage/worker_pool.go.
// vocabulary may be nil (every item's USCDIClasses is then nil too) — this
// feature works exactly as before without it, per report.go's own
// schema-version-4-vs-5 additive discipline.
func BuildInventoryWithGranularity(xmlMirror map[string]interface{}, schemaLoader *cdaSchema.CDASchemaLoader, vocabulary *uscdi.USCDIVocabulary, elementLevel bool) []InventoryItem {
	if xmlMirror == nil || schemaLoader == nil {
		return nil
	}

	sectionNodes := executors.ResolveCDAPaths(xmlMirror, "component.structuredBody.component[*].section", true)

	var items []InventoryItem

	// Unstructured Document has no structuredBody at all (see
	// buildNonXMLBodyInventoryItem's own doc comment) — without this, such a
	// document would produce zero sectionNodes above and read identically to
	// "nothing was found" in the report, rather than the correct, expected
	// shape for this document type.
	if len(sectionNodes) == 0 {
		if item := buildNonXMLBodyInventoryItem(xmlMirror); item != nil {
			items = append(items, *item)
		}
	}

	for _, sn := range sectionNodes {
		sectionMap, ok := sn.(map[string]interface{})
		if !ok {
			continue
		}
		category, sectionKey, title := classifySection(sectionMap, schemaLoader)
		uscdiClasses := uscdiClassesForSection(vocabulary, sectionKey)

		entries := executors.ResolveCDAPaths(sectionMap, "entry[*]", true)
		for idx, entryNode := range entries {
			items = append(items, InventoryItem{
				Category:     category,
				SectionKey:   sectionKey,
				SectionTitle: title,
				EntryIndex:   idx,
				USCDIClasses: uscdiClasses,
			})
			if !elementLevel {
				continue
			}
			entryMap, ok := entryNode.(map[string]interface{})
			if !ok {
				continue
			}
			for _, elementPath := range walkEntryElements(entryMap, "") {
				items = append(items, InventoryItem{
					Category:     category,
					SectionKey:   sectionKey,
					SectionTitle: title,
					EntryIndex:   idx,
					ElementPath:  elementPath,
					USCDIClasses: uscdiClasses,
				})
			}
		}
	}
	items = append(items, buildHeaderInventory(xmlMirror, vocabulary, elementLevel)...)
	return items
}

// uscdiClassesForSection resolves sectionKey's real USCDI v3 class set —
// a thin wrapper around uscdi.USCDIVocabulary.ClassesForSection (the class-
// resolution logic itself lives there now, shared with cda/builder's Build
// Requirements bridge, rather than duplicated in each consumer). Kept as its
// own named function here (not inlined at each call site) purely to avoid
// touching every call site's nil-vocabulary variable name across this file
// and inventory_test.go.
func uscdiClassesForSection(vocabulary *uscdi.USCDIVocabulary, sectionKey string) []string {
	return vocabulary.ClassesForSection(sectionKey)
}

// buildNonXMLBodyInventoryItem reports a document with no structuredBody at
// all — CDA's Unstructured Document type, whose <component> is a
// <nonXMLBody> instead (see cda/document/types.go's CDAUnstructuredBody) —
// as its own single pseudo-item, under the SAME "document.unstructuredBody"
// key services/cda_fhir/declarative_document_mapper.go's own
// CoverageTracker.Record call uses. Without this, such a document
// legitimately produces zero real sections (there are none — not a parsing
// failure) and would read identically to "nothing was found" in the report,
// when it's actually the correct, expected shape for this document type.
// Returns nil when component.nonXMLBody isn't present at all (every other
// document type, or a genuinely empty/malformed structuredBody document —
// this function only ever fires something real, never guesses).
func buildNonXMLBodyInventoryItem(xmlMirror map[string]interface{}) *InventoryItem {
	nodes := executors.ResolveCDAPaths(xmlMirror, "component.nonXMLBody", true)
	if len(nodes) == 0 {
		return nil
	}
	return &InventoryItem{
		Category:     headerCategory,
		SectionKey:   "document.unstructuredBody",
		SectionTitle: "Unstructured Content",
		EntryIndex:   0,
	}
}

// headerCategory groups every document-header InventoryItem in the report
// under one clinical/administrative category, distinct from any section
// category a schema-recognized or "Unclassified" body section could ever
// produce.
const headerCategory = "Document Header"

// buildHeaderInventory walks the document HEADER's own raw XML (a sibling
// of "component.structuredBody", never enumerated by the section walk
// above) for every header data group cda/document/header_parser.go parses
// into CDAHeader.
//
// Patient, Custodian, EncompassingEncounter (shared by 2 call sites, see
// that mapper's own comment) and Author are the 4 header constructs CDA
// Coverage Audit's mapping engine actually builds FHIR resources from
// (services/cda_fhir/declarative_engine.go's
// BuildHeaderResourceWithCoveragePrefix, wired from declarative_document_
// mapper.go's 5 real header call sites) -- these can show real found/missed
// splits.
//
// LegalAuthenticator, DocumentationOf/ServiceEvent, Informant, and
// RelatedDocument are parsed by header_parser.go but have NO
// BuildHeaderResource call site anywhere (declarative_document_mapper.go's
// own doc comment on DeclarativeMapDocument explains why for
// LegalAuthenticator specifically: kept at parity with the legacy
// MapDocument(), which also never built one). They are walked here anyway
// and will always report 100% missed -- that IS the correct, useful signal
// per this feature's own founding principle (see Performers on Vital Signs,
// or an unselected Author in headerRepeatingInventory below): a field with
// real data but no rule that ever reads it must show as a gap, not be
// silently invisible to the report. Without walking them, a document that
// carries e.g. a legal attestation or a referral's related-document link
// would give zero signal that this content is never used downstream.
//
// Every raw path string below is a hand-written constant, not derived from
// the typed-tree translator -- consistent with this file's own design
// principle of walking ONLY the raw mirror with zero typed-tree dependency
// (a section entry's inventory items are built the identical way, via
// ResolveCDAPaths against xmlMirror directly). The 4 tracked constructs'
// paths have been independently traced to produce byte-identical strings to
// what services/executors/cda_element_translation_table.go's
// "CDAHeader"-rooted rules emit for the same construct (e.g. "patient" ->
// "recordTarget[0].patientRole[0]"); the 4 untracked ones have no
// translator counterpart to match against (nothing ever resolves a path
// against them), so their raw tag names are traced directly against
// header_parser.go instead.
func buildHeaderInventory(xmlMirror map[string]interface{}, vocabulary *uscdi.USCDIVocabulary, elementLevel bool) []InventoryItem {
	var items []InventoryItem
	items = append(items, headerSingularInventory(xmlMirror, vocabulary, elementLevel,
		"header.patient", "Patient Demographics", "recordTarget.patientRole", "recordTarget[0].patientRole[0]")...)
	items = append(items, headerSingularInventory(xmlMirror, vocabulary, elementLevel,
		"header.custodian", "Custodian", "custodian.assignedCustodian", "custodian[0].assignedCustodian[0]")...)
	items = append(items, headerSingularInventory(xmlMirror, vocabulary, elementLevel,
		"header.encompassingEncounter", "Encompassing Encounter", "componentOf.encompassingEncounter", "componentOf[0].encompassingEncounter[0]")...)
	items = append(items, headerRepeatingInventory(xmlMirror, vocabulary, elementLevel,
		"header.author", "Author", "author")...)

	// Never mapped to any FHIR resource today (see doc comment above) --
	// always report as a full gap, not silently omitted.
	items = append(items, headerSingularInventory(xmlMirror, vocabulary, elementLevel,
		"header.legalAuthenticator", "Legal Authenticator", "legalAuthenticator", "legalAuthenticator[0]")...)
	items = append(items, headerSingularInventory(xmlMirror, vocabulary, elementLevel,
		"header.documentationOf", "Documentation Of (Service Event)", "documentationOf.serviceEvent", "documentationOf[0].serviceEvent[0]")...)
	items = append(items, headerRepeatingInventory(xmlMirror, vocabulary, elementLevel,
		"header.informant", "Informant", "informant")...)
	items = append(items, headerRepeatingInventory(xmlMirror, vocabulary, elementLevel,
		"header.relatedDocument", "Related Document", "relatedDocument")...)
	items = append(items, headerParticipantInventory(xmlMirror, vocabulary, elementLevel,
		"header.relatedPerson", "Related Person", "IND")...)
	return items
}

// headerSingularInventory builds the entry-level (and, if elementLevel,
// element-level) InventoryItems for one singular header construct -- at
// most one node exists in the raw document (rawPath resolves to 0 or 1
// nodes; see BuildHeaderResourceWithCoveragePrefix's own doc comment on why
// EncompassingEncounter's TWO call sites still share this SAME single
// tracking unit). xmlPrefix is that node's own basePath, in the identical
// index-every-segment convention every recorded coverage key already uses.
func headerSingularInventory(xmlMirror map[string]interface{}, vocabulary *uscdi.USCDIVocabulary, elementLevel bool, sectionKey, sectionTitle, rawPath, xmlPrefix string) []InventoryItem {
	nodes := executors.ResolveCDAPaths(xmlMirror, rawPath, true)
	if len(nodes) == 0 {
		return nil
	}
	nodeMap, ok := nodes[0].(map[string]interface{})
	if !ok {
		return nil
	}
	uscdiClasses := uscdiClassesForSection(vocabulary, sectionKey)
	items := []InventoryItem{{Category: headerCategory, SectionKey: sectionKey, SectionTitle: sectionTitle, EntryIndex: 0, USCDIClasses: uscdiClasses}}
	if !elementLevel {
		return items
	}
	for _, elementPath := range walkEntryElements(nodeMap, xmlPrefix) {
		items = append(items, InventoryItem{
			Category:     headerCategory,
			SectionKey:   sectionKey,
			SectionTitle: sectionTitle,
			EntryIndex:   0,
			ElementPath:  elementPath,
			USCDIClasses: uscdiClasses,
		})
	}
	return items
}

// headerRepeatingInventory walks EVERY raw occurrence of rawTag at the
// document root (e.g. every "author", "informant", "relatedDocument"), each
// as its own trackable unit at its own real raw index -- not just whichever
// one (if any) the typed side's own selection logic picks. This matters
// most for author: firstAuthorWithPersonIndexed picks the first author WITH
// a usable person; other raw authors, e.g. an authoring device with no
// person, are real, present content that was never used. Ground truth means
// walking what's actually IN the document, not what the mapping engine
// happened to pick -- same reasoning already established for Performers on
// Vital Signs (a field with real data but no rule ever reads it correctly
// shows as a gap, not as silently excluded).
func headerRepeatingInventory(xmlMirror map[string]interface{}, vocabulary *uscdi.USCDIVocabulary, elementLevel bool, sectionKey, sectionTitle, rawTag string) []InventoryItem {
	return headerRepeatingInventoryQuery(xmlMirror, vocabulary, elementLevel, sectionKey, sectionTitle, rawTag+"[*]", rawTag)
}

// headerParticipantInventory walks every raw <participant typeCode="IND">
// element at the document root — the AppxA "Related Person Relationship and
// Name Participant" header construct (header.relatedPerson.json). Needs a
// predicate query ("participant[typeCode='IND']") rather than
// headerRepeatingInventory's plain "rawTag[*]" wildcard, since "participant"
// is a raw tag shared with other participation kinds elsewhere in CDA (a
// single bracket group is ResolveCDAPaths' segmentPredicate kind, which
// already resolves to every matching node — see cda_path_resolver.go's own
// parseCDAPathSegment doc comment on why "[pred][*]" chained brackets aren't
// supported, so the query is exactly one bracket group, not two) — otherwise
// identical walk, factored into the shared helper below. Never marked
// "touched" today (no FHIR-mapping rule reads Related Person data yet) —
// reports as a full gap whenever present, the same deliberate, correct
// signal header.legalAuthenticator/header.documentationOf already establish
// for header constructs with real content but no reader.
func headerParticipantInventory(xmlMirror map[string]interface{}, vocabulary *uscdi.USCDIVocabulary, elementLevel bool, sectionKey, sectionTitle, typeCode string) []InventoryItem {
	query := "participant[typeCode='" + typeCode + "']"
	return headerRepeatingInventoryQuery(xmlMirror, vocabulary, elementLevel, sectionKey, sectionTitle, query, "participant")
}

// headerRepeatingInventoryQuery is the shared walk both
// headerRepeatingInventory and headerParticipantInventory run — every raw
// match of query at the document root, each as its own trackable unit at
// its own real raw index, not just whichever one (if any) the typed side's
// own selection logic picks. This matters most for author:
// firstAuthorWithPersonIndexed picks the first author WITH a usable person;
// other raw authors, e.g. an authoring device with no person, are real,
// present content that was never used. Ground truth means walking what's
// actually IN the document, not what the mapping engine happened to pick --
// same reasoning already established for Performers on Vital Signs (a field
// with real data but no rule ever reads it correctly shows as a gap, not as
// silently excluded). prefixTag is separate from query since a predicate
// query ("participant[typeCode='IND']") isn't itself a valid element-path
// prefix segment — callers pass the bare tag name their own query resolves
// against.
func headerRepeatingInventoryQuery(xmlMirror map[string]interface{}, vocabulary *uscdi.USCDIVocabulary, elementLevel bool, sectionKey, sectionTitle, query, prefixTag string) []InventoryItem {
	var items []InventoryItem
	uscdiClasses := uscdiClassesForSection(vocabulary, sectionKey)
	for idx, a := range executors.ResolveCDAPaths(xmlMirror, query, true) {
		aMap, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		items = append(items, InventoryItem{Category: headerCategory, SectionKey: sectionKey, SectionTitle: sectionTitle, EntryIndex: idx, USCDIClasses: uscdiClasses})
		if !elementLevel {
			continue
		}
		prefix := prefixTag + "[" + strconv.Itoa(idx) + "]"
		for _, elementPath := range walkEntryElements(aMap, prefix) {
			items = append(items, InventoryItem{
				Category:     headerCategory,
				SectionKey:   sectionKey,
				SectionTitle: sectionTitle,
				EntryIndex:   idx,
				ElementPath:  elementPath,
				USCDIClasses: uscdiClasses,
			})
		}
	}
	return items
}

// walkEntryElements recursively enumerates every XML element under node (an
// "entry[i]" node, or one of its own descendants) as its own path relative
// to the entry root, in the SAME index-every-segment convention
// services/executors/cda_element_translation.go's translator independently
// produces for the identical underlying element — see that file's doc
// comment for why neither side can rely on the other's information (this
// walker sees only "how many raw occurrences of this tag," never "is this
// typed Go field genuinely singular").
//
// Element granularity only: "@attr"-prefixed keys and "#text" (own text)
// describe the CURRENT element, not a separate one, and are skipped — this
// alone removes most administrative-attribute noise (@nullFlavor,
// @codeSystemName, etc. never become their own "missed" item).
//
// Keys are visited in sorted order so the resulting item list — and
// therefore report ordering — is deterministic run to run for the same
// document shape, useful for regression testing.
func walkEntryElements(node map[string]interface{}, prefix string) []string {
	keys := make([]string, 0, len(node))
	for k := range node {
		if strings.HasPrefix(k, "@") || k == "#text" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var paths []string
	for _, key := range keys {
		for idx, child := range asNodeList(node[key]) {
			childPath := key + "[" + strconv.Itoa(idx) + "]"
			if prefix != "" {
				childPath = prefix + "." + childPath
			}
			paths = append(paths, childPath)
			if childMap, ok := child.(map[string]interface{}); ok {
				paths = append(paths, walkEntryElements(childMap, childPath)...)
			}
		}
	}
	return paths
}

// classifySection resolves category/sectionKey/title for one <section>
// node from the generic XML mirror. Mirrors cda/document/section_parser.go's
// resolveKey exactly — templateId, then LOINC, then title normalization —
// so classification never silently drifts between parsing (what the typed
// tree, and therefore the FHIR mapper, keys this section under) and
// auditing (what this report calls it). Returns ("Unclassified", "",
// <best-effort title>) only when NONE of the three tiers produce anything
// at all (no templateId match, no LOINC match, and either no <title> or a
// title too irregular for even the generic camelCase fallback to key) —
// itself a useful report signal, not an error.
//
// A title that DOES normalize to a key, but where that key isn't in this
// schema's own catalog (e.g. "Reason for Visit" -> "reasonForVisit", a key
// the mapping engine's DefaultNarrativeSectionDefs registry recognizes but
// ccda_2_1.json's section catalog does not), still gets returned as a real
// SectionKey — not "Unclassified" — using the section's own title as both
// category and display name. This matters beyond cosmetics: SectionKey=""
// can never be "touched" (see InventoryItem.TrackingKey's own doc comment),
// so leaving it empty here would make this section's coverage permanently
// unrecordable even on the rare path where the mapping engine DOES capture
// its content (e.g. the narrative DocumentReference pass, which keys off
// this exact same title-derived key via doc.SectionsByKey).
func classifySection(sectionMap map[string]interface{}, schemaLoader *cdaSchema.CDASchemaLoader) (category, sectionKey, title string) {
	for _, tid := range asNodeList(sectionMap["templateId"]) {
		tidMap, ok := tid.(map[string]interface{})
		if !ok {
			continue
		}
		root, _ := tidMap["@root"].(string)
		if root == "" {
			continue
		}
		if def := schemaLoader.GetSectionByTemplateID(root); def != nil {
			return categoryFor(def), def.Key, def.DisplayName
		}
	}

	if codeMap, ok := sectionMap["code"].(map[string]interface{}); ok {
		if code, _ := codeMap["@code"].(string); code != "" {
			if def := schemaLoader.GetSectionByLOINC(code); def != nil {
				return categoryFor(def), def.Key, def.DisplayName
			}
		}
	}

	// NormalizeSectionTitleToKey must run against the section's OWN raw
	// <title> text, not rawSectionTitle's "(untitled section)" placeholder
	// below -- that placeholder exists only for display in the genuinely-
	// unclassified case, and would otherwise itself normalize into a bogus
	// non-empty key ("untitledSection") for every title-less section,
	// masking the one case classifySection must still report "Unclassified"
	// for.
	ownTitle, _ := sectionMap["title"].(string)
	if derivedKey := cdadocument.NormalizeSectionTitleToKey(ownTitle); derivedKey != "" {
		if def := schemaLoader.GetSection(derivedKey); def != nil {
			return categoryFor(def), def.Key, def.DisplayName
		}
		return ownTitle, derivedKey, ownTitle
	}

	return "Unclassified", "", rawSectionTitle(sectionMap)
}

// categoryFor returns the clinical category label for a recognized section
// — its own USCDIClass when set, falling back to the schema's human display
// name so a section rarely surfaces as a blank category.
func categoryFor(def *cdaSchema.CDASectionDef) string {
	if def.USCDIClass != "" {
		return def.USCDIClass
	}
	return def.DisplayName
}

// rawSectionTitle extracts a best-effort human label for a section the
// schema doesn't recognize at all — its own <title> text if present,
// otherwise a generic placeholder. Never a clinical value: a section title
// is administrative/structural metadata about the document, not patient data.
func rawSectionTitle(sectionMap map[string]interface{}) string {
	if title, ok := sectionMap["title"].(string); ok && title != "" {
		return title
	}
	return "(untitled section)"
}

// asNodeList mirrors cda_path_resolver.go's unexported asCDANodeList — the
// generic mirror's "auto-array-on-repeat" convention (a single occurrence
// collapses to a bare object instead of a one-element array) means a
// single-templateId section needs the same normalization ResolveCDAPaths
// already applies internally; duplicated here (not exported from
// cda_path_resolver.go) since it's a two-line, package-private concern.
func asNodeList(v interface{}) []interface{} {
	switch t := v.(type) {
	case []interface{}:
		return t
	case nil:
		return nil
	default:
		return []interface{}{t}
	}
}
