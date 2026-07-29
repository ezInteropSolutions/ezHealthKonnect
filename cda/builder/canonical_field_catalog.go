// cda/builder/canonical_field_catalog.go
//
// Public, read-only catalog of every canonical field key cda.build actually
// reads — for a section, straight from the CDA schema's own CDAFieldDef.Key
// vocabulary (entry_archetypes.go's buildEntry/writeFieldValue); for the
// header, from header_fields.go's declarative mapping tables plus a small,
// explicitly-documented list of keys written by hand-coded logic in
// document_builder.go (fixed-OID identifiers, IG-mandated fallback
// defaults, repeating groups) that were never table-driven to begin with.
//
// This is the single source of truth the cda.map_to_canonical pipeline
// step's field-picker UI is built from (via a small API endpoint) — so a
// user configuring a no-code CSV/DB-to-CCD mapping only ever sees target
// keys cda.build genuinely consumes, never a stale or invented vocabulary.
package builder

import cdaSchema "ezhealthkonnect/cda"

// CanonicalFieldInfo describes one canonical field key available as a
// mapping target — for sections this includes the field's companion
// Display/System/Unit/Family variants (only the ones the field actually
// defines an XPath for), mirroring entry_archetypes.go's writeFieldValue
// exactly.
type CanonicalFieldInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	DataType    string `json:"dataType,omitempty"`
	Conformance string `json:"conformance,omitempty"` // SHALL | SHOULD | MAY, from CDAFieldDef.Conformance — empty for header-catalog entries (see header_requirements.go for header conformance)
}

// CanonicalSectionInfo describes one section cda.build can populate — every
// section in the schema, unlike GetSections' declarative-engine-dispatched
// subset (controllers/cda_schema_controller.go): cda.build walks
// CDASchemaLoader.AllSections() directly and isn't gated by which sections
// the CDA→FHIR declarative engine happens to dispatch, so the two lists are
// deliberately different views over the same schema.
type CanonicalSectionInfo struct {
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	LOINCCode   string `json:"loincCode"`
	Conformance string `json:"conformance"`
}

// SectionCatalog returns every section cda.build/cda.map_to_canonical can
// populate, for the mapping UI's section-key picker.
func SectionCatalog(loader *cdaSchema.CDASchemaLoader) []CanonicalSectionInfo {
	sections := loader.AllSections()
	out := make([]CanonicalSectionInfo, 0, len(sections))
	for _, s := range sections {
		if s.IsHeader {
			continue
		}
		out = append(out, CanonicalSectionInfo{
			Key: s.Key, DisplayName: s.DisplayName, LOINCCode: s.LOINCCode, Conformance: s.Conformance,
		})
	}
	return out
}

// SectionFieldCatalog returns every canonical field key buildEntry reads for
// sectionKey's ENTRY-level fields, sourced live from the schema
// (cdaSchema.CDASectionDef.Fields) — never duplicated/re-derived. For a
// section with no RepeatingGroups this is the section's complete field
// vocabulary; for a section that DOES declare one (e.g. Vital Signs), these
// are specifically the fields shared once per entry (e.g. the Organizer's
// own effectiveTime) — see SectionRepeatingGroupCatalog for the per-item
// vocabulary used inside the loop. Returns nil if sectionKey is unknown.
func SectionFieldCatalog(loader *cdaSchema.CDASchemaLoader, sectionKey string) []CanonicalFieldInfo {
	sec := loader.GetSection(sectionKey)
	if sec == nil {
		return nil
	}
	out := make([]CanonicalFieldInfo, 0, len(sec.Fields)*2)
	for _, f := range sec.Fields {
		out = append(out, expandFieldWithCompanions(f)...)
	}
	return out
}

// SectionRepeatingGroupInfo describes one section's declared "loop" point —
// the no-code Group By UI's per-item field picker needs both the group's own
// canonical key (map_to_canonical's groupedItemsKey target, e.g.
// "components") and its per-item field vocabulary, which is a DIFFERENT set
// of canonical keys than the section's entry-level SectionFieldCatalog (e.g.
// Vital Signs' per-component "vitalCode"/"value" vs. the organizer's own
// "effectiveTime").
type SectionRepeatingGroupInfo struct {
	Key    string               `json:"key"`
	Fields []CanonicalFieldInfo `json:"fields"`
}

// SectionRepeatingGroupCatalog returns sectionKey's declared RepeatingGroup
// for the no-code Group By UI, or nil if the section doesn't declare one.
// Only the first RepeatingGroup is exposed — every section declaring one
// today (Vital Signs) has exactly one; a section needing multiple
// independently-configurable loops would need this to become a slice, not
// something the current schema/UI has a real case for yet.
func SectionRepeatingGroupCatalog(loader *cdaSchema.CDASchemaLoader, sectionKey string) *SectionRepeatingGroupInfo {
	sec := loader.GetSection(sectionKey)
	if sec == nil || len(sec.RepeatingGroups) == 0 {
		return nil
	}
	group := sec.RepeatingGroups[0]
	fields := make([]CanonicalFieldInfo, 0, len(group.Fields)*2)
	for _, f := range group.Fields {
		fields = append(fields, expandFieldWithCompanions(f)...)
	}
	return &SectionRepeatingGroupInfo{Key: group.Key, Fields: fields}
}

// expandFieldWithCompanions projects one CDAFieldDef into its primary
// CanonicalFieldInfo plus whichever Display/System/Unit/Family companions it
// actually defines an XPath for — shared by SectionFieldCatalog and
// SectionRepeatingGroupCatalog so the two never drift on this convention.
// Companions inherit the parent field's conformance level — the schema
// doesn't level them independently (e.g. a SHALL code's display text isn't
// itself a separately-leveled SHALL/SHOULD field in ccda_2_1.json).
func expandFieldWithCompanions(f *cdaSchema.CDAFieldDef) []CanonicalFieldInfo {
	out := []CanonicalFieldInfo{{Key: f.Key, Label: f.USCDIElement, DataType: f.DataType, Conformance: f.Conformance}}
	if f.XPathDisplay != "" {
		out = append(out, CanonicalFieldInfo{Key: f.Key + "Display", Label: f.USCDIElement + " (display text)", DataType: "ST", Conformance: f.Conformance})
	}
	if f.XPathSystem != "" {
		out = append(out, CanonicalFieldInfo{Key: f.Key + "System", Label: f.USCDIElement + " (code system OID)", DataType: "OID", Conformance: f.Conformance})
	}
	if f.XPathUnit != "" {
		out = append(out, CanonicalFieldInfo{Key: f.Key + "Unit", Label: f.USCDIElement + " (unit)", DataType: "ST", Conformance: f.Conformance})
	}
	if f.XPathFamily != "" {
		out = append(out, CanonicalFieldInfo{Key: f.Key + "Family", Label: f.USCDIElement + " (family/component)", DataType: "ST", Conformance: f.Conformance})
	}
	return out
}

// headerMappingsToCatalog/codedMappingsToCatalog adapt header_fields.go's
// existing declarative tables (built for XML writing, keyed internally) to
// the public CanonicalFieldInfo shape — no duplication of the tables
// themselves, just a projection.
func headerMappingsToCatalog(mappings []headerFieldMapping, dataType string) []CanonicalFieldInfo {
	out := make([]CanonicalFieldInfo, 0, len(mappings))
	for _, m := range mappings {
		out = append(out, CanonicalFieldInfo{Key: m.CanonicalKey, Label: m.CanonicalKey, DataType: dataType})
	}
	return out
}

func codedMappingsToCatalog(mappings []codedFieldMapping) []CanonicalFieldInfo {
	out := make([]CanonicalFieldInfo, 0, len(mappings)*2)
	for _, m := range mappings {
		out = append(out, CanonicalFieldInfo{Key: m.CodeKey, Label: m.CodeKey, DataType: "CD"})
		out = append(out, CanonicalFieldInfo{Key: m.DisplayKey, Label: m.CodeKey + " display", DataType: "ST"})
	}
	return out
}

// prefixKeys namespaces every entry's Key under prefix+"." — used for
// patientAddressFields, since header.patient.address is a genuinely nested
// object in the canonical JSON (see writePatientHeader in
// document_builder.go: `patientData["address"].(map[string]interface{})`),
// unlike every other patient field which sits flat under header.patient.
// Target dot-paths in map_to_canonical_executor.go are resolved relative to
// header.<group> via executors.UpdateFieldValue, so this prefix is what
// makes "address.street" land in the right nested spot instead of a bogus
// flat "header.patient.address.street"-shaped sibling key.
func prefixKeys(fields []CanonicalFieldInfo, prefix string) []CanonicalFieldInfo {
	out := make([]CanonicalFieldInfo, len(fields))
	for i, f := range fields {
		out[i] = CanonicalFieldInfo{Key: prefix + "." + f.Key, Label: f.Label, DataType: f.DataType}
	}
	return out
}

// headerBespokeFields lists canonical header keys written by hand-coded
// logic in document_builder.go rather than a declarative table — each is
// genuinely special-cased there and documented at its own call site:
//   - npi (writeNPI): fixed-OID identifier, always NPI-rooted regardless of group
//   - documentationOf's effectiveTimeLow/High (writeDocumentationOfHeader):
//     CCD SHALL element with an IG-mandated nullFlavor="UNK" fallback, not a
//     plain optional scalar
//   - encompassingEncounter's id/effectiveTimeLow/High/dischargeDispositionCode
//     (writeEncompassingEncounterHeader): a bespoke, fully-optional group
//     with no declarative-table equivalent
//   - informant's orgName (writePersonReference includeOrg=true): shares
//     personItemFields for name, but org name is a bespoke addition specific
//     to informants (documentationOf performers don't get one)
var headerBespokeFields = map[string][]CanonicalFieldInfo{
	"patient": {
		{Key: "phone", Label: "Phone", DataType: "ST"},
	},
	"author": {
		{Key: "npi", Label: "NPI", DataType: "ST"},
		{Key: "phone", Label: "Phone", DataType: "ST"},
	},
	"informant": {
		{Key: "npi", Label: "NPI", DataType: "ST"},
		{Key: "orgName", Label: "Represented Organization Name", DataType: "ST"},
	},
	"documentationOfPerformer": {
		{Key: "npi", Label: "NPI", DataType: "ST"},
	},
	"documentationOf": {
		{Key: "effectiveTimeLow", Label: "Service Event Start", DataType: "TS"},
		{Key: "effectiveTimeHigh", Label: "Service Event End", DataType: "TS"},
	},
	"encompassingEncounter": {
		{Key: "id", Label: "Encounter ID", DataType: "ST"},
		{Key: "effectiveTimeLow", Label: "Encounter Start", DataType: "TS"},
		{Key: "effectiveTimeHigh", Label: "Encounter End", DataType: "TS"},
		{Key: "dischargeDispositionCode", Label: "Discharge Disposition Code", DataType: "CD"},
	},
}

// HeaderFieldCatalog returns every canonical field key writeXHeader (in
// document_builder.go) reads for the given header group. Recognized groups:
// "patient" (scalars + address + coded fields), "author", "informant",
// "documentationOf" (service-event-level fields; use
// "documentationOfPerformer" for its nested performers[] item fields), and
// "encompassingEncounter" (includes healthCareFacilityFields, since
// writeEncompassingEncounterHeader nests location/healthCareFacility inline).
// Returns nil for an unrecognized group.
func HeaderFieldCatalog(group string) []CanonicalFieldInfo {
	switch group {
	case "patient":
		out := headerMappingsToCatalog(patientScalarFields, "ST")
		out = append(out, prefixKeys(headerMappingsToCatalog(patientAddressFields, "ST"), "address")...)
		out = append(out, codedMappingsToCatalog(patientCodedFields)...)
		out = append(out, headerBespokeFields["patient"]...)
		return out
	case "author":
		out := headerMappingsToCatalog(authorScalarFields, "ST")
		out = append(out, prefixKeys(headerMappingsToCatalog(patientAddressFields, "ST"), "address")...)
		out = append(out, headerBespokeFields["author"]...)
		return out
	case "informant":
		out := headerMappingsToCatalog(personItemFields, "ST")
		out = append(out, headerBespokeFields["informant"]...)
		return out
	case "documentationOf":
		return headerBespokeFields["documentationOf"]
	case "documentationOfPerformer":
		out := headerMappingsToCatalog(personItemFields, "ST")
		out = append(out, headerBespokeFields["documentationOfPerformer"]...)
		return out
	case "encompassingEncounter":
		out := headerMappingsToCatalog(healthCareFacilityFields, "ST")
		out = append(out, headerBespokeFields["encompassingEncounter"]...)
		return out
	default:
		return nil
	}
}
