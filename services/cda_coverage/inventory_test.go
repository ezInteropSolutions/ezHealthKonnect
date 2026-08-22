// services/cda_coverage/inventory_test.go
package cdacoverage

import (
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/uscdi"
)

func testSchemaLoader(t *testing.T) *cdaSchema.CDASchemaLoader {
	t.Helper()
	loader, err := cdaSchema.NewCDASchemaLoader("../../cda/schemas")
	if err != nil {
		t.Fatalf("NewCDASchemaLoader: %v", err)
	}
	return loader
}

func testVocabulary(t *testing.T) *uscdi.USCDIVocabulary {
	t.Helper()
	vocab, err := uscdi.NewUSCDIVocabulary("../../cda/schemas/uscdi_v3.json")
	if err != nil {
		t.Fatalf("NewUSCDIVocabulary: %v", err)
	}
	return vocab
}

// socialHistorySectionNode builds a synthetic GenericXMLToJSON-shaped
// <section> node matching the real Social History section's templateId —
// the richest real multi-USCDI-class section (Health Status Assessments,
// Care Plan, and Patient Demographics/Information all resolve here — see
// uscdi_v3.json's smokingStatus/sdohAssessment/sexualOrientation entries).
func socialHistorySectionNode(n int) map[string]interface{} {
	entries := make([]interface{}, n)
	for i := range entries {
		entries[i] = map[string]interface{}{"@typeCode": "DRIV"}
	}
	return map[string]interface{}{
		"templateId": []interface{}{
			map[string]interface{}{"@root": "2.16.840.1.113883.10.20.22.2.17", "@extension": "2015-08-01"},
		},
		"code":  map[string]interface{}{"@code": "29762-2"},
		"title": "Social History",
		"entry": entries,
	}
}

// medicationsSectionNode builds a synthetic GenericXMLToJSON-shaped <section>
// node matching the real Medications section's templateId (see
// cda/schemas/ccda_2_1.json), with n <entry> children.
func medicationsSectionNode(n int) map[string]interface{} {
	entries := make([]interface{}, n)
	for i := range entries {
		entries[i] = map[string]interface{}{"@typeCode": "DRIV"}
	}
	return map[string]interface{}{
		"templateId": []interface{}{
			map[string]interface{}{"@root": "2.16.840.1.113883.10.20.22.2.1.1", "@extension": "2014-06-09"},
		},
		"code":  map[string]interface{}{"@code": "10160-0"},
		"title": "Medications",
		"entry": entries,
	}
}

// unrecognizedSectionNode builds a section whose templateId/LOINC code match
// nothing in the schema — the "Unclassified" case.
func unrecognizedSectionNode(title string, n int) map[string]interface{} {
	entries := make([]interface{}, n)
	for i := range entries {
		entries[i] = map[string]interface{}{"@typeCode": "DRIV"}
	}
	return map[string]interface{}{
		"templateId": map[string]interface{}{"@root": "2.16.840.1.113883.99.99.99.99"},
		"code":       map[string]interface{}{"@code": "99999-9"},
		"title":      title,
		"entry":      entries,
	}
}

func mirrorWithSections(sections ...interface{}) map[string]interface{} {
	return map[string]interface{}{
		"component": map[string]interface{}{
			"structuredBody": map[string]interface{}{
				"component": sections,
			},
		},
	}
}

func TestBuildInventory_RecognizedSection(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(
		map[string]interface{}{"section": medicationsSectionNode(3)},
	)

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	for i, item := range items {
		if item.SectionKey != "medications" {
			t.Errorf("item[%d].SectionKey = %q, want %q", i, item.SectionKey, "medications")
		}
		if item.Category != "Medications" {
			t.Errorf("item[%d].Category = %q, want %q", i, item.Category, "Medications")
		}
		if item.EntryIndex != i {
			t.Errorf("item[%d].EntryIndex = %d, want %d", i, item.EntryIndex, i)
		}
		if want := executors.CDAEntryKey("medications", i); item.TrackingKey() != want {
			t.Errorf("item[%d].TrackingKey() = %q, want %q", i, item.TrackingKey(), want)
		}
	}
}

func TestBuildInventory_UnrecognizedSection_IsUnclassified(t *testing.T) {
	loader := testSchemaLoader(t)
	// Real "Unclassified" (empty SectionKey, permanently untrackable) now
	// only happens when a section has NO title at all -- once
	// classifySection's third tier (title normalization) was added, ANY
	// non-empty title produces a real, trackable key, even one the schema
	// doesn't register (see TestBuildInventory_TitleFallback_UnregisteredKey
	// below) -- exactly matching cda/document/section_parser.go's own
	// resolveKey, which the typed tree (and therefore the FHIR mapper)
	// always uses. A titled-but-gibberish section is no longer a valid
	// "Unclassified" fixture; a title-less one is the only remaining real
	// case (CDA technically allows a section with no <title>).
	mirror := mirrorWithSections(
		map[string]interface{}{"section": unrecognizedSectionNode("", 2)},
	)

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	for _, item := range items {
		if item.Category != "Unclassified" {
			t.Errorf("Category = %q, want %q", item.Category, "Unclassified")
		}
		if item.SectionKey != "" {
			t.Errorf("SectionKey = %q, want empty for an unrecognized section", item.SectionKey)
		}
		if item.SectionTitle != "(untitled section)" {
			t.Errorf("SectionTitle = %q, want %q", item.SectionTitle, "(untitled section)")
		}
		// An unclassified item can never be "touched" — nothing in the
		// pipeline can reference a section by a key it doesn't have.
		if item.TrackingKey() != "" {
			t.Errorf("TrackingKey() = %q, want empty for an unclassified item", item.TrackingKey())
		}
	}
}

// TestBuildInventory_TitleFallback_RegisteredKey covers classifySection's
// third tier (title normalization) landing on a key the schema DOES
// recognize -- e.g. a section whose templateId/LOINC don't match anything
// (real EHRs sometimes omit or mis-code these) but whose <title> is exactly
// "Advance Directives", which cda/document/section_parser.go's own
// titleKeyMap maps to "advanceDirectives", a real ccda_2_1.json section.
// Must resolve identically to how section_parser.go's resolveKey (the
// function that actually builds the typed tree the FHIR mapper reads) would
// classify the same raw section -- a real, non-schema-registered category
// would silently drift coverage auditing away from what the mapper does.
func TestBuildInventory_TitleFallback_RegisteredKey(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(
		map[string]interface{}{"section": unrecognizedSectionNode("Advance Directives", 1)},
	)

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	item := items[0]
	if item.SectionKey != "advanceDirectives" {
		t.Errorf("SectionKey = %q, want %q", item.SectionKey, "advanceDirectives")
	}
	if item.Category == "Unclassified" {
		t.Errorf("Category = %q, want the schema's real category for advanceDirectives, not Unclassified", item.Category)
	}
	if item.TrackingKey() == "" {
		t.Errorf("TrackingKey() empty, want a real key now that the section resolved via title fallback")
	}
}

// TestBuildInventory_TitleFallback_UnregisteredKey covers the case the user
// actually hit: a section (e.g. "Reason for Visit", LOINC 29299-5) that the
// TYPED parser (section_parser.go) classifies via title fallback into a
// real key ("reasonForVisit") that services/cda_fhir's
// DefaultNarrativeSectionDefs registry knows about, but ccda_2_1.json's own
// section catalog does NOT register. Before this tier existed, this
// produced Category="Unclassified", SectionKey="" -- permanently
// unrecordable (TrackingKey()=="") even on the rare path where the mapping
// engine's narrative DocumentReference pass DOES capture this section's
// content via the identical derived key. Must now report the section's own
// title as both category and display name, with a real, recordable key --
// not "Unclassified".
func TestBuildInventory_TitleFallback_UnregisteredKey(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(
		map[string]interface{}{"section": unrecognizedSectionNode("Reason for Visit", 1)},
	)

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	item := items[0]
	if item.SectionKey != "reasonForVisit" {
		t.Errorf("SectionKey = %q, want %q", item.SectionKey, "reasonForVisit")
	}
	if item.Category != "Reason for Visit" {
		t.Errorf("Category = %q, want the section's own title %q, not Unclassified", item.Category, "Reason for Visit")
	}
	if item.TrackingKey() == "" {
		t.Errorf("TrackingKey() empty, want a real key now that the section resolved via title fallback")
	}
}

func TestBuildInventory_MultipleSections_MixedRecognition(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(
		map[string]interface{}{"section": medicationsSectionNode(2)},
		map[string]interface{}{"section": unrecognizedSectionNode("Something New", 1)},
	)

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (2 medications + 1 title-fallback-classified 'Something New')", len(items))
	}
}

func TestBuildInventory_NilInputs(t *testing.T) {
	loader := testSchemaLoader(t)
	if items := BuildInventory(nil, loader, nil); items != nil {
		t.Errorf("BuildInventory(nil, loader) = %v, want nil", items)
	}
	if items := BuildInventory(mirrorWithSections(), nil, nil); items != nil {
		t.Errorf("BuildInventory(mirror, nil) = %v, want nil", items)
	}
}

func TestBuildInventory_NoSections_ReturnsEmpty(t *testing.T) {
	loader := testSchemaLoader(t)
	items := BuildInventory(mirrorWithSections(), loader, nil)
	if len(items) != 0 {
		t.Errorf("got %d items, want 0 for a document with no sections", len(items))
	}
}

func TestBuildInventory_DefaultsToEntryLevel(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(map[string]interface{}{"section": medicationsSectionNode(2)})
	if got := BuildInventory(mirror, loader, nil); len(got) != 2 {
		t.Errorf("BuildInventory (entry-level default) = %d items, want 2 (no element items)", len(got))
	}
}

func TestWalkEntryElements_AttributesAndOwnTextSkipped(t *testing.T) {
	node := map[string]interface{}{
		"@classCode":  "OBS",
		"@moodCode":   "EVN",
		"#text":       "some narrative text",
		"code":        map[string]interface{}{"@code": "8867-4"},
	}
	got := walkEntryElements(node, "")
	if len(got) != 1 || got[0] != "code[0]" {
		t.Errorf("walkEntryElements = %v, want exactly [\"code[0]\"] (attributes/#text must never become their own item)", got)
	}
}

func TestWalkEntryElements_RepeatedTag_DistinctIndices(t *testing.T) {
	node := map[string]interface{}{
		"performer": []interface{}{
			map[string]interface{}{"@typeCode": "PRF"},
			map[string]interface{}{"@typeCode": "SPRF"},
		},
	}
	got := walkEntryElements(node, "")
	want := []string{"performer[0]", "performer[1]"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("walkEntryElements = %v, want %v", got, want)
	}
}

func TestWalkEntryElements_SingleOccurrence_StillIndexed(t *testing.T) {
	// GenericXMLToJSON collapses a single occurrence to a bare object, not a
	// one-element array -- walkEntryElements must still index it (matching
	// services/executors/cda_element_translation.go's own convention; see
	// that file's doc comment on why both sides must agree without either
	// needing information only the other has).
	node := map[string]interface{}{
		"code": map[string]interface{}{"@code": "8867-4"},
	}
	got := walkEntryElements(node, "")
	if len(got) != 1 || got[0] != "code[0]" {
		t.Errorf("walkEntryElements = %v, want [\"code[0]\"]", got)
	}
}

func TestWalkEntryElements_RecursesIntoNestedElements(t *testing.T) {
	node := map[string]interface{}{
		"effectiveTime": map[string]interface{}{
			"low":  map[string]interface{}{"@value": "20240101"},
			"high": map[string]interface{}{"@value": "20240102"},
		},
	}
	got := walkEntryElements(node, "")
	want := map[string]bool{
		"effectiveTime[0]":          true,
		"effectiveTime[0].low[0]":  true,
		"effectiveTime[0].high[0]": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d paths, want %d -- got %v", len(got), len(want), got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected path %q", p)
		}
	}
}

func TestBuildInventoryWithGranularity_ElementLevel(t *testing.T) {
	loader := testSchemaLoader(t)
	entry := map[string]interface{}{
		"@typeCode": "DRIV",
		"code":      map[string]interface{}{"@code": "10160-0"},
		"statusCode": map[string]interface{}{"@code": "completed"},
	}
	mirror := mirrorWithSections(map[string]interface{}{"section": map[string]interface{}{
		"templateId": []interface{}{
			map[string]interface{}{"@root": "2.16.840.1.113883.10.20.22.2.1.1", "@extension": "2014-06-09"},
		},
		"code":  map[string]interface{}{"@code": "10160-0"},
		"title": "Medications",
		"entry": []interface{}{entry},
	}})

	entryLevel := BuildInventoryWithGranularity(mirror, loader, nil, false)
	if len(entryLevel) != 1 {
		t.Fatalf("entry-level: got %d items, want 1", len(entryLevel))
	}

	elementLevel := BuildInventoryWithGranularity(mirror, loader, nil, true)
	// 1 entry-level item + 2 element items (code, statusCode).
	if len(elementLevel) != 3 {
		t.Fatalf("element-level: got %d items, want 3 -- got %+v", len(elementLevel), elementLevel)
	}
	var sawEntryItem, sawCode, sawStatusCode bool
	for _, item := range elementLevel {
		switch {
		case item.ElementPath == "":
			sawEntryItem = true
			if want := "medications#0"; item.TrackingKey() != want {
				t.Errorf("entry item TrackingKey() = %q, want %q", item.TrackingKey(), want)
			}
		case item.ElementPath == "code[0]":
			sawCode = true
			if want := "medications#0/code[0]"; item.TrackingKey() != want {
				t.Errorf("code item TrackingKey() = %q, want %q", item.TrackingKey(), want)
			}
		case item.ElementPath == "statusCode[0]":
			sawStatusCode = true
		}
	}
	if !sawEntryItem || !sawCode || !sawStatusCode {
		t.Errorf("missing expected items -- entry=%v code=%v statusCode=%v (got %+v)", sawEntryItem, sawCode, sawStatusCode, elementLevel)
	}
}

// TestBuildInventoryWithGranularity_NoHeaderData_ProducesNoHeaderItems is
// the regression guard for buildHeaderInventory's addition to
// BuildInventoryWithGranularity: mirrorWithSections() sets no
// "recordTarget"/"author"/"custodian"/"componentOf" key at all, so the new
// header walk must be a provable no-op for every pre-existing fixture in
// this file.
func TestBuildInventoryWithGranularity_NoHeaderData_ProducesNoHeaderItems(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(map[string]interface{}{"section": medicationsSectionNode(1)})
	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	for _, item := range items {
		if item.Category == headerCategory {
			t.Errorf("unexpected header item from a mirror with no header data: %+v", item)
		}
	}
}

// mirrorWithHeader adds document-header raw XML branches (siblings of
// "component", never touched by the structured-body walk) to a
// mirrorWithSections()-shaped fixture.
func mirrorWithHeader(mirror map[string]interface{}, header map[string]interface{}) map[string]interface{} {
	for k, v := range header {
		mirror[k] = v
	}
	return mirror
}

func TestBuildInventoryWithGranularity_HeaderPatient(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithHeader(mirrorWithSections(), map[string]interface{}{
		"recordTarget": map[string]interface{}{
			"patientRole": map[string]interface{}{
				"id": map[string]interface{}{"@root": "2.16.840.1.113883.4.1"},
				"patient": map[string]interface{}{
					"name": map[string]interface{}{"family": "Smith"},
				},
			},
		},
	})

	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	var sawEntry, sawId, sawName bool
	for _, item := range items {
		if item.SectionKey != "header.patient" {
			continue
		}
		if item.Category != headerCategory {
			t.Errorf("Category = %q, want %q", item.Category, headerCategory)
		}
		switch item.ElementPath {
		case "":
			sawEntry = true
			if want := "header.patient#0"; item.TrackingKey() != want {
				t.Errorf("entry item TrackingKey() = %q, want %q", item.TrackingKey(), want)
			}
		case "recordTarget[0].patientRole[0].id[0]":
			sawId = true
		case "recordTarget[0].patientRole[0].patient[0].name[0]":
			sawName = true
		}
	}
	if !sawEntry || !sawId || !sawName {
		t.Errorf("missing expected header.patient items -- entry=%v id=%v name=%v (got %+v)", sawEntry, sawId, sawName, items)
	}
}

func TestBuildInventoryWithGranularity_HeaderAuthors_MultipleDistinctEntries(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithHeader(mirrorWithSections(), map[string]interface{}{
		"author": []interface{}{
			map[string]interface{}{"assignedAuthor": map[string]interface{}{"assignedAuthoringDevice": map[string]interface{}{"softwareName": "Epic EHR"}}},
			map[string]interface{}{"assignedAuthor": map[string]interface{}{"assignedPerson": map[string]interface{}{"name": map[string]interface{}{"family": "Jones"}}}},
		},
	})

	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	entryIndices := map[int]bool{}
	var sawAuthor0Device, sawAuthor1Person bool
	for _, item := range items {
		if item.SectionKey != "header.author" {
			continue
		}
		if item.ElementPath == "" {
			entryIndices[item.EntryIndex] = true
			continue
		}
		if item.EntryIndex == 0 && strings.Contains(item.ElementPath, "assignedAuthoringDevice") {
			sawAuthor0Device = true
		}
		if item.EntryIndex == 1 && strings.Contains(item.ElementPath, "assignedPerson") {
			sawAuthor1Person = true
		}
	}
	if !entryIndices[0] || !entryIndices[1] {
		t.Errorf("expected distinct entry-level items for author#0 and author#1 -- got %+v", entryIndices)
	}
	if !sawAuthor0Device || !sawAuthor1Person {
		t.Errorf("missing expected per-author element items -- device=%v person=%v (got %+v)", sawAuthor0Device, sawAuthor1Person, items)
	}
}

func TestBuildInventoryWithGranularity_HeaderCustodian(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithHeader(mirrorWithSections(), map[string]interface{}{
		"custodian": map[string]interface{}{
			"assignedCustodian": map[string]interface{}{
				"representedCustodianOrganization": map[string]interface{}{
					"name": "Acme Clinic",
				},
			},
		},
	})

	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	var sawEntry, sawName bool
	for _, item := range items {
		if item.SectionKey != "header.custodian" {
			continue
		}
		if item.ElementPath == "" {
			sawEntry = true
		}
		if item.ElementPath == "custodian[0].assignedCustodian[0].representedCustodianOrganization[0].name[0]" {
			sawName = true
		}
	}
	if !sawEntry || !sawName {
		t.Errorf("missing expected header.custodian items -- entry=%v name=%v (got %+v)", sawEntry, sawName, items)
	}
}

func TestBuildInventoryWithGranularity_HeaderEncompassingEncounter(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithHeader(mirrorWithSections(), map[string]interface{}{
		"componentOf": map[string]interface{}{
			"encompassingEncounter": map[string]interface{}{
				"id": map[string]interface{}{"@root": "2.16.840.1.113883.19"},
				"location": map[string]interface{}{
					"healthCareFacility": map[string]interface{}{
						"code": map[string]interface{}{"@code": "1160-1"},
					},
				},
			},
		},
	})

	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	var sawEntry, sawId, sawFacilityCode bool
	for _, item := range items {
		if item.SectionKey != "header.encompassingEncounter" {
			continue
		}
		if item.ElementPath == "" {
			sawEntry = true
		}
		if item.ElementPath == "componentOf[0].encompassingEncounter[0].id[0]" {
			sawId = true
		}
		if item.ElementPath == "componentOf[0].encompassingEncounter[0].location[0].healthCareFacility[0].code[0]" {
			sawFacilityCode = true
		}
	}
	if !sawEntry || !sawId || !sawFacilityCode {
		t.Errorf("missing expected header.encompassingEncounter items -- entry=%v id=%v facilityCode=%v (got %+v)", sawEntry, sawId, sawFacilityCode, items)
	}
}

// TestBuildInventoryWithGranularity_HeaderUntrackedGroups_AlwaysGap covers
// legalAuthenticator, documentationOf/serviceEvent, informant, and
// relatedDocument -- header data groups header_parser.go parses into
// CDAHeader but that have NO BuildHeaderResource call site anywhere (see
// buildHeaderInventory's own doc comment). Unlike Patient/Author/Custodian/
// EncompassingEncounter, these can never be "touched" -- nothing in the
// pipeline ever calls CoverageTracker.Record for them -- so they must
// always appear in the inventory (real ground truth: this data exists) but
// their TrackingKey() must never come back "found" in a report, matching
// the same "real data, no rule ever reads it" precedent already established
// for Performers on Vital Signs and an unselected Author.
func TestBuildInventoryWithGranularity_HeaderUntrackedGroups_AlwaysGap(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithHeader(mirrorWithSections(), map[string]interface{}{
		"legalAuthenticator": map[string]interface{}{
			"time": map[string]interface{}{"@value": "20260101"},
		},
		"documentationOf": map[string]interface{}{
			"serviceEvent": map[string]interface{}{
				"effectiveTime": map[string]interface{}{"low": map[string]interface{}{"@value": "20260101"}},
			},
		},
		"informant": []interface{}{
			map[string]interface{}{"assignedEntity": map[string]interface{}{"id": map[string]interface{}{"@root": "2.16.840.1.113883.4.6"}}},
		},
		"relatedDocument": []interface{}{
			map[string]interface{}{"@typeCode": "RPLC", "parentDocument": map[string]interface{}{"id": map[string]interface{}{"@root": "1.2.3.4"}}},
		},
	})

	wantSectionKeys := map[string]string{
		"header.legalAuthenticator": "",
		"header.documentationOf":    "",
		"header.informant":          "",
		"header.relatedDocument":    "",
	}
	seenEntry := map[string]bool{}
	seenElement := map[string]bool{}

	items := BuildInventoryWithGranularity(mirror, loader, nil, true)
	for _, item := range items {
		if _, want := wantSectionKeys[item.SectionKey]; !want {
			continue
		}
		if item.Category != headerCategory {
			t.Errorf("%s: Category = %q, want %q", item.SectionKey, item.Category, headerCategory)
		}
		if item.ElementPath == "" {
			seenEntry[item.SectionKey] = true
			continue
		}
		seenElement[item.SectionKey] = true
		// Ground truth only -- nothing ever records these, so every one of
		// these items must report as missed.
		if item.TrackingKey() == "" {
			t.Errorf("%s: TrackingKey() empty, want a real (always-unrecorded) key", item.SectionKey)
		}
	}
	for key := range wantSectionKeys {
		if !seenEntry[key] {
			t.Errorf("missing entry-level item for %s", key)
		}
		if !seenElement[key] {
			t.Errorf("missing element-level items for %s", key)
		}
	}
}

// TestBuildInventory_NonXMLBody_ProducesPseudoItem covers Phase 5
// (Unstructured Document): a document with no structuredBody at all — its
// own <component> is a <nonXMLBody> instead — must still produce a real
// InventoryItem (SectionKey "document.unstructuredBody"), not silently
// zero items, which would read identically to "nothing was found" even
// though this is the correct, expected shape for this document type.
func TestBuildInventory_NonXMLBody_ProducesPseudoItem(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := map[string]interface{}{
		"component": map[string]interface{}{
			"nonXMLBody": map[string]interface{}{
				"text": map[string]interface{}{
					"@mediaType": "application/pdf",
				},
			},
		},
	}

	items := BuildInventory(mirror, loader, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (the nonXMLBody pseudo-item)", len(items))
	}
	item := items[0]
	if item.SectionKey != "document.unstructuredBody" {
		t.Errorf("SectionKey = %q, want %q", item.SectionKey, "document.unstructuredBody")
	}
	if item.TrackingKey() != "document.unstructuredBody#0" {
		t.Errorf("TrackingKey() = %q, want %q", item.TrackingKey(), "document.unstructuredBody#0")
	}
}

// TestBuildInventory_NoStructuredBodyNoNonXMLBody_ProducesNoPseudoItem is a
// regression guard: an empty/malformed mirror (neither structuredBody nor
// nonXMLBody present) must NOT fabricate a pseudo-item — that would be
// inventing ground truth, not reporting it.
func TestBuildInventory_NoStructuredBodyNoNonXMLBody_ProducesNoPseudoItem(t *testing.T) {
	loader := testSchemaLoader(t)
	items := BuildInventory(map[string]interface{}{}, loader, nil)
	if len(items) != 0 {
		t.Errorf("got %d items, want 0 for a mirror with neither structuredBody nor nonXMLBody -- %+v", len(items), items)
	}
}

func TestUSCDIClassesForSection_NilVocabularyOrEmptySectionKey_ReturnsNil(t *testing.T) {
	vocab := testVocabulary(t)
	if got := uscdiClassesForSection(nil, "medications"); got != nil {
		t.Errorf("uscdiClassesForSection(nil vocab, ...) = %v, want nil", got)
	}
	if got := uscdiClassesForSection(vocab, ""); got != nil {
		t.Errorf("uscdiClassesForSection(vocab, \"\") = %v, want nil", got)
	}
}

// TestUSCDIClassesForSection_UnmappedSection_ReturnsNilNotAFabricatedGuess
// guards the honesty guarantee this whole bridge exists for: a section
// uscdi_v3.json has no entry for yet must read as "not yet in the USCDI v3
// vocabulary," never as a fabricated empty-but-non-nil "no coverage" signal.
// "goals" is deliberately used here since it's a real, schema-recognized
// section with no USCDI v3 vocabulary entry as of this test (unlike most
// sections touched during Phase A/A2, which now do) — if a future Phase
// A3 adds one, this test should be repointed at another still-unmapped
// section rather than deleted, so the "unmapped renders as nil" guarantee
// stays covered.
func TestUSCDIClassesForSection_UnmappedSection_ReturnsNilNotAFabricatedGuess(t *testing.T) {
	vocab := testVocabulary(t)
	if got := uscdiClassesForSection(vocab, "not-a-real-section-key"); got != nil {
		t.Errorf("uscdiClassesForSection(vocab, unmapped) = %v, want nil", got)
	}
}

func TestUSCDIClassesForSection_SingleClassSection(t *testing.T) {
	vocab := testVocabulary(t)
	got := uscdiClassesForSection(vocab, "medications")
	if len(got) != 1 || got[0] != "Medications" {
		t.Errorf("uscdiClassesForSection(vocab, medications) = %v, want [Medications]", got)
	}
}

// TestUSCDIClassesForSection_MultiClassSection guards the finding that a
// single CDA section can legitimately carry elements from more than one
// USCDI class at once — socialHistory is the richest real example (Health
// Status Assessments' smokingStatus/pregnancyStatus, Care Plan's
// sdohAssessment, Patient Demographics/Information's sexualOrientation/
// genderIdentity all live in this one section). A naive "one class per
// section" implementation would silently drop classes here.
func TestUSCDIClassesForSection_MultiClassSection(t *testing.T) {
	vocab := testVocabulary(t)
	got := uscdiClassesForSection(vocab, "socialHistory")
	want := map[string]bool{"Care Plan": true, "Health Status Assessments": true, "Patient Demographics/Information": true}
	if len(got) != len(want) {
		t.Fatalf("uscdiClassesForSection(vocab, socialHistory) = %v, want %d classes", got, len(want))
	}
	for _, class := range got {
		if !want[class] {
			t.Errorf("unexpected class %q in socialHistory's USCDI class set: %v", class, got)
		}
	}
}

// TestBuildInventoryWithGranularity_PopulatesUSCDIClassesEndToEnd proves the
// full wiring (BuildInventoryWithGranularity -> classifySection ->
// uscdiClassesForSection -> InventoryItem.USCDIClasses), not just the
// resolver function in isolation.
func TestBuildInventoryWithGranularity_PopulatesUSCDIClassesEndToEnd(t *testing.T) {
	loader := testSchemaLoader(t)
	vocab := testVocabulary(t)
	mirror := mirrorWithSections(map[string]interface{}{"section": medicationsSectionNode(2)})

	items := BuildInventoryWithGranularity(mirror, loader, vocab, false)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	for i, item := range items {
		if len(item.USCDIClasses) != 1 || item.USCDIClasses[0] != "Medications" {
			t.Errorf("item[%d].USCDIClasses = %v, want [Medications]", i, item.USCDIClasses)
		}
	}
}

// TestBuildInventoryWithGranularity_NilVocabulary_LeavesUSCDIClassesNil
// confirms the feature is fully opt-in at the vocabulary level too (not just
// per-interface via cda_coverage_audit_config) — a nil vocabulary (e.g. the
// vocabulary file failed to load at startup) must not panic and must leave
// every item's USCDIClasses nil, matching pre-Phase-B report shape exactly.
func TestBuildInventoryWithGranularity_NilVocabulary_LeavesUSCDIClassesNil(t *testing.T) {
	loader := testSchemaLoader(t)
	mirror := mirrorWithSections(map[string]interface{}{"section": medicationsSectionNode(1)})

	items := BuildInventoryWithGranularity(mirror, loader, nil, false)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].USCDIClasses != nil {
		t.Errorf("USCDIClasses = %v, want nil with a nil vocabulary", items[0].USCDIClasses)
	}
}
