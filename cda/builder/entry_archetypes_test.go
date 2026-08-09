// cda/builder/entry_archetypes_test.go
//
// Isolated tests for buildEntry's RepeatingGroup loop mechanism (the "Block
// 1" loop engine) — deliberately using a synthetic CDASectionDef, not a real
// section, so the mechanism itself is proven before any real section (Vital
// Signs) depends on it. This is the WriteAtXPath positional-predicate spike
// the implementation plan called out as the one real open risk.
package builder

import (
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	"github.com/beevik/etree"
)

func newTestSectionEl() *etree.Element {
	doc := etree.NewDocument()
	return doc.CreateElement("section")
}

func syntheticRepeatingSection() *cdaSchema.CDASectionDef {
	return &cdaSchema.CDASectionDef{
		Key:              "syntheticVitals",
		EntryElementPath: "organizer",
		RepeatingGroups: []cdaSchema.RepeatingGroup{
			{
				Key:                    "components",
				WrapperTag:             "component",
				ObservationElementPath: "observation",
				TemplateID:             "2.16.840.1.113883.10.20.22.4.27",
				TemplateIDExt:          "2014-06-09",
				Fields: []*cdaSchema.CDAFieldDef{
					{Key: "vitalCode", XPath: "code/@code", XPathDisplay: "code/@displayName"},
					{Key: "value", XPath: "value/@value", XPathUnit: "value/@unit"},
				},
			},
		},
	}
}

// TestWriteRepeatingGroups_MultipleItemsProduceDistinctSiblings is the core
// spike: N array items must produce N distinct <component> siblings under
// one <organizer>, not N attributes of one element and not N items
// collapsed onto one element.
func TestWriteRepeatingGroups_MultipleItemsProduceDistinctSiblings(t *testing.T) {
	sec := syntheticRepeatingSection()
	record := map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{"vitalCode": "8480-6", "vitalCodeDisplay": "Systolic BP", "value": "120", "valueUnit": "mm[Hg]"},
			map[string]interface{}{"vitalCode": "8462-4", "vitalCodeDisplay": "Diastolic BP", "value": "80", "valueUnit": "mm[Hg]"},
			map[string]interface{}{"vitalCode": "8310-5", "vitalCodeDisplay": "Body temperature", "value": "37.0", "valueUnit": "Cel"},
		},
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	doc := etree.NewDocument()
	doc.SetRoot(sectionEl.Copy())
	xml, err := doc.WriteToString()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	organizerEl := sectionEl.FindElement("entry/organizer")
	if organizerEl == nil {
		t.Fatalf("expected an <organizer> element, got:\n%s", xml)
	}
	components := organizerEl.SelectElements("component")
	if len(components) != 3 {
		t.Fatalf("expected 3 <component> siblings, got %d:\n%s", len(components), xml)
	}

	wantCodes := []string{"8480-6", "8462-4", "8310-5"}
	wantValues := []string{"120", "80", "37.0"}
	for i, comp := range components {
		obs := comp.SelectElement("observation")
		if obs == nil {
			t.Fatalf("component %d: expected a nested <observation>, got:\n%s", i, xml)
		}
		if obs.SelectAttrValue("classCode", "") != "OBS" || obs.SelectAttrValue("moodCode", "") != "EVN" {
			t.Errorf("component %d: expected observation boilerplate classCode=OBS moodCode=EVN, got: %+v", i, obs.Attr)
		}
		tid := obs.SelectElement("templateId")
		if tid == nil || tid.SelectAttrValue("root", "") != "2.16.840.1.113883.10.20.22.4.27" || tid.SelectAttrValue("extension", "") != "2014-06-09" {
			t.Errorf("component %d: expected Vital Sign Observation templateId, got: %+v", i, tid)
		}
		codeEl := obs.SelectElement("code")
		if codeEl == nil || codeEl.SelectAttrValue("code", "") != wantCodes[i] {
			t.Errorf("component %d: expected code=%q, got: %+v", i, wantCodes[i], codeEl)
		}
		valueEl := obs.SelectElement("value")
		if valueEl == nil || valueEl.SelectAttrValue("value", "") != wantValues[i] {
			t.Errorf("component %d: expected value=%q, got: %+v", i, wantValues[i], valueEl)
		}
	}

	// The SAME component (not a 4th one) must carry both of its own fields —
	// i.e. re-resolving component[1] for a second field must reuse the
	// existing element, not create a new sibling.
	if len(components[0].SelectElements("observation")) != 1 {
		t.Errorf("expected exactly one <observation> under the first component, got %d", len(components[0].SelectElements("observation")))
	}
}

// TestWriteRepeatingGroups_AbsentGroupKey_NoOp confirms a section declaring
// a RepeatingGroup is completely unaffected when the canonical record simply
// doesn't carry that array key — the regression guard for every one of the
// other 37 sections that will never set RepeatingGroups at all.
func TestWriteRepeatingGroups_AbsentGroupKey_NoOp(t *testing.T) {
	sec := syntheticRepeatingSection()
	record := map[string]interface{}{} // no "components" key at all

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	organizerEl := sectionEl.FindElement("entry/organizer")
	if organizerEl == nil {
		t.Fatal("expected the organizer root to still be created (EntryElementPath is independent of RepeatingGroups)")
	}
	if len(organizerEl.SelectElements("component")) != 0 {
		t.Errorf("expected zero <component> elements when record has no array at the group's key, got %d", len(organizerEl.SelectElements("component")))
	}
}

// TestWriteRepeatingGroups_RequiredPathsSkipsIncompleteItems verifies an
// item missing a required canonical key is skipped entirely (no empty/junk
// component), and — critically — the skip doesn't leave a gap in the
// resulting sibling numbering (WriteAtXPath's positional predicate requires
// strictly sequential indices).
func TestWriteRepeatingGroups_RequiredPathsSkipsIncompleteItems(t *testing.T) {
	sec := syntheticRepeatingSection()
	sec.RepeatingGroups[0].RequiredPaths = []string{"vitalCode", "value"}
	record := map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{"vitalCode": "8480-6", "value": "120"},
			map[string]interface{}{"vitalCode": "8462-4"}, // missing "value" — must be skipped
			map[string]interface{}{"vitalCode": "8310-5", "value": "37.0"},
		},
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	organizerEl := sectionEl.FindElement("entry/organizer")
	components := organizerEl.SelectElements("component")
	if len(components) != 2 {
		t.Fatalf("expected 2 <component> elements (one skipped for missing required data), got %d", len(components))
	}
	// Both surviving components must have a fully-populated <value> — proof
	// there's no gap-numbered empty component wedged between them.
	for i, comp := range components {
		obs := comp.SelectElement("observation")
		if obs == nil || obs.SelectElement("value") == nil {
			t.Errorf("component %d: expected a populated <value>, got nil", i)
		}
	}
}

// TestWriteFieldValue_SkipIfXPathPresent verifies a field is skipped when
// another field already wrote a value at the configured guard path, and
// written normally when that path is empty.
func TestWriteFieldValue_SkipIfXPathPresent(t *testing.T) {
	entryEl := etree.NewDocument().CreateElement("entry")
	primary := &cdaSchema.CDAFieldDef{Key: "specificCode", XPath: "code/@code"}
	fallback := &cdaSchema.CDAFieldDef{Key: "genericCode", XPath: "code/@code", SkipIfXPathPresent: "code/@code"}

	record := map[string]interface{}{"specificCode": "44054006", "genericCode": "SHOULD-NOT-APPEAR"}
	writeFieldValue(entryEl, primary, record)
	writeFieldValue(entryEl, fallback, record)

	codeEl := entryEl.SelectElement("code")
	if codeEl == nil || codeEl.SelectAttrValue("code", "") != "44054006" {
		t.Errorf("expected the primary field's value to win, got: %+v", codeEl)
	}

	// Now the reverse order with no primary value present — fallback should write.
	entryEl2 := etree.NewDocument().CreateElement("entry")
	record2 := map[string]interface{}{"genericCode": "125605004"}
	writeFieldValue(entryEl2, fallback, record2)
	codeEl2 := entryEl2.SelectElement("code")
	if codeEl2 == nil || codeEl2.SelectAttrValue("code", "") != "125605004" {
		t.Errorf("expected fallback to write when guard path is empty, got: %+v", codeEl2)
	}
}

// syntheticEntryRelationshipSection mirrors Medication Activity's real shape:
// WrapperTag is a BARE "entryRelationship" (no attribute predicate),
// disambiguated via WrapperAttr/WrapperAttrValue — the fix for the real
// collision risk found while designing the Medications' Indications
// RepeatingGroup (a section with OTHER, unrelated entryRelationship
// children sharing the same parent).
func syntheticEntryRelationshipSection() *cdaSchema.CDASectionDef {
	return &cdaSchema.CDASectionDef{
		Key:              "syntheticMedication",
		EntryElementPath: "substanceAdministration",
		RepeatingGroups: []cdaSchema.RepeatingGroup{
			{
				Key:                    "indications",
				WrapperTag:             "entryRelationship",
				WrapperAttr:            "typeCode",
				WrapperAttrValue:       "RSON",
				ObservationElementPath: "observation",
				TemplateID:             "2.16.840.1.113883.10.20.22.4.19",
				TemplateIDExt:          "2014-06-09",
				Fields: []*cdaSchema.CDAFieldDef{
					{Key: "indicationCode", XPath: "value/@code", XPathDisplay: "value/@displayName"},
				},
			},
		},
	}
}

// TestWriteRepeatingGroups_WrapperAttr_SetsConstantAttributeOnEachWrapper
// verifies WrapperAttr/WrapperAttrValue is stamped on every created wrapper
// — the mechanism Medication Activity's repeatable "0..* entryRelationship
// typeCode='RSON'" (CONF:1098-7536/7537) needs, since the bare tag alone
// can't disambiguate from other entryRelationship purposes.
func TestWriteRepeatingGroups_WrapperAttr_SetsConstantAttributeOnEachWrapper(t *testing.T) {
	sec := syntheticEntryRelationshipSection()
	record := map[string]interface{}{
		"indications": []interface{}{
			map[string]interface{}{"indicationCode": "44054006"},
			map[string]interface{}{"indicationCode": "38341003"},
		},
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	rootEl := sectionEl.FindElement("entry/substanceAdministration")
	if rootEl == nil {
		t.Fatal("expected a <substanceAdministration> root element")
	}
	relationships := rootEl.SelectElements("entryRelationship")
	if len(relationships) != 2 {
		t.Fatalf("expected 2 <entryRelationship> siblings, got %d", len(relationships))
	}
	for i, rel := range relationships {
		if rel.SelectAttrValue("typeCode", "") != "RSON" {
			t.Errorf("entryRelationship %d: expected typeCode=RSON, got %+v", i, rel.Attr)
		}
	}
}

// TestWriteRepeatingGroups_WrapperAttr_DoesNotCollideWithUnrelatedSameTagSibling
// is the actual regression guard for the bug found during design: an
// EXISTING <entryRelationship> sibling created for a totally different
// purpose (e.g. a REFR-typed structural entryRelationship, simulating
// Medication Supply Order) must be left completely untouched — the loop
// engine must never mistake it for one of "its own" wrapper slots, and must
// never miscount around it.
func TestWriteRepeatingGroups_WrapperAttr_DoesNotCollideWithUnrelatedSameTagSibling(t *testing.T) {
	sec := syntheticEntryRelationshipSection()
	record := map[string]interface{}{
		"indications": []interface{}{
			map[string]interface{}{"indicationCode": "44054006"},
		},
	}

	sectionEl := newTestSectionEl()
	// Pre-create an UNRELATED entryRelationship (different typeCode, some
	// other purpose) directly under the same root BEFORE buildEntry runs —
	// simulating a StructuralTemplateIDs-created sibling that already exists
	// by the time writeRepeatingGroups executes (buildEntry runs
	// StructuralTemplateIDs before writeRepeatingGroups).
	entryEl := sectionEl.CreateElement("entry")
	entryEl.CreateAttr("typeCode", "DRIV")
	rootEl := entryEl.CreateElement("substanceAdministration")
	preexisting := rootEl.CreateElement("entryRelationship")
	preexisting.CreateAttr("typeCode", "REFR")
	preexisting.CreateElement("marker").SetText("do-not-touch")

	writeRepeatingGroups(rootEl, nil, sec, record)

	relationships := rootEl.SelectElements("entryRelationship")
	if len(relationships) != 2 {
		t.Fatalf("expected 2 <entryRelationship> total (1 pre-existing + 1 new indication), got %d", len(relationships))
	}

	// The pre-existing REFR one must be byte-for-byte untouched.
	refr := relationships[0]
	if refr.SelectAttrValue("typeCode", "") != "REFR" {
		t.Errorf("expected the pre-existing entryRelationship's typeCode to remain REFR, got %q", refr.SelectAttrValue("typeCode", ""))
	}
	if marker := refr.SelectElement("marker"); marker == nil || marker.Text() != "do-not-touch" {
		t.Errorf("expected the pre-existing entryRelationship's own content to be untouched, got marker=%v", marker)
	}
	if refr.SelectElement("observation") != nil {
		t.Error("the pre-existing REFR entryRelationship must NOT have gained a nested <observation> from the Indications loop")
	}

	// The new RSON one must be correctly built.
	rson := relationships[1]
	if rson.SelectAttrValue("typeCode", "") != "RSON" {
		t.Errorf("expected the new entryRelationship's typeCode to be RSON, got %q", rson.SelectAttrValue("typeCode", ""))
	}
	obs := rson.SelectElement("observation")
	if obs == nil {
		t.Fatal("expected the new RSON entryRelationship to have a nested <observation>")
	}
	if code := obs.SelectElement("value"); code == nil || code.SelectAttrValue("code", "") != "44054006" {
		t.Errorf("expected the Indication observation's value/@code=44054006, got %+v", code)
	}
}

// TestWriteRepeatingGroups_SectionWithNoRepeatingGroups_Unaffected is a
// belt-and-suspenders regression guard using a REAL existing section
// (problems, from the loaded schema) to confirm writeRepeatingGroups is a
// true no-op for every section that doesn't declare any RepeatingGroups —
// the actual production case for all 37 sections not touched this round.
func TestWriteRepeatingGroups_SectionWithNoRepeatingGroups_Unaffected(t *testing.T) {
	loader := loadTestSchema(t)
	sec := loader.GetSection("problems")
	if sec == nil {
		t.Fatal("expected to load the real \"problems\" section")
	}
	if len(sec.RepeatingGroups) != 0 {
		t.Fatal("expected \"problems\" to declare no RepeatingGroups in this round")
	}

	before := strings.Repeat("x", 0) // sentinel, just documents intent
	_ = before

	sectionEl := newTestSectionEl()
	record := map[string]interface{}{"conditionCode": "44054006"}
	buildEntry(sectionEl, sec, record)

	// writeRepeatingGroups ranges over an empty slice and returns immediately
	// — this assertion just confirms buildEntry still produced its normal
	// flat output (the conditionCode value), proving no side effect from the
	// new code path being present but unused.
	doc := etree.NewDocument()
	doc.SetRoot(sectionEl.Copy())
	xml, _ := doc.WriteToString()
	if !strings.Contains(xml, `code="44054006"`) {
		t.Errorf("expected normal flat-field output to be unaffected, got:\n%s", xml)
	}
}

// TestBuildEntry_ClassCodeAndMoodCodeOverrides proves the EntryClassCodeOverride
// (found necessary for Result Organizer's classCode="BATTERY", CONF only
// visible in that template's own worked example) and EntryMoodCodeOverride
// (found necessary for Instruction (V2)'s moodCode="INT", CONF:1098-7392)
// mechanisms both correctly override tagBoilerplate's generic per-tag
// defaults, using a synthetic section so the mechanism itself is proven
// independent of any one real section's other quirks.
func TestBuildEntry_ClassCodeAndMoodCodeOverrides(t *testing.T) {
	sec := &cdaSchema.CDASectionDef{
		Key:                    "syntheticInstruction",
		EntryElementPath:       "act",
		EntryClassCodeOverride: "PCPR",
		EntryMoodCodeOverride:  "INT",
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "topicCode", XPath: "act/code/@code"},
		},
	}
	record := map[string]interface{}{"topicCode": "182904002"}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	actEl := sectionEl.FindElement("entry/act")
	if actEl == nil {
		t.Fatal("expected an <act> root element")
	}
	if got := actEl.SelectAttrValue("classCode", ""); got != "PCPR" {
		t.Errorf("classCode = %q, want PCPR (override, not the generic \"ACT\" tagBoilerplate default)", got)
	}
	if got := actEl.SelectAttrValue("moodCode", ""); got != "INT" {
		t.Errorf("moodCode = %q, want INT (override, not the generic \"EVN\" tagBoilerplate default)", got)
	}
	// statusCode wasn't overridden — tagBoilerplate's own "act" default
	// ("active") must still apply untouched, proving the two new overrides
	// are independent of the existing EntryStatusCodeOverride mechanism.
	if sc := actEl.SelectElement("statusCode"); sc == nil || sc.SelectAttrValue("code", "") != "active" {
		t.Errorf("expected the untouched tagBoilerplate statusCode=\"active\" default, got: %+v", sc)
	}
}

// TestWriteFieldValue_InjectsXsiTypeOnValueElement proves the fix for a real
// gap a schema validator found: <value> is CDA's polymorphic ANY-typed
// element and rejects any attribute at all without an explicit xsi:type
// override. Covers all 3 cases: an explicit DataType (CD), the "ANY"
// disambiguation via XPathUnit presence (PQ), and "ANY" without a unit (CD).
func TestWriteFieldValue_InjectsXsiTypeOnValueElement(t *testing.T) {
	sec := &cdaSchema.CDASectionDef{
		Key: "syntheticValueTypes",
		Fields: []*cdaSchema.CDAFieldDef{
			// Fields are written relative to <entry> itself (buildEntry
			// calls writeFieldValue(entryEl, ...)), not nested under
			// EntryElementPath's rootEl — no "observation/" prefix needed.
			{Key: "codedField", DataType: "CD", XPath: "codedResult/value/@code", XPathDisplay: "codedResult/value/@displayName"},
			{Key: "quantityField", DataType: "ANY", XPath: "quantityResult/value/@value", XPathUnit: "quantityResult/value/@unit"},
			{Key: "anyNoUnitField", DataType: "ANY", XPath: "anyResult/value/@code"},
		},
	}
	record := map[string]interface{}{
		"codedField":        "44054006",
		"codedFieldDisplay": "Diabetes",
		"quantityField":     "98",
		"quantityFieldUnit": "mg/dL",
		"anyNoUnitField":    "12345",
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	codedVal := sectionEl.FindElement("entry/codedResult/value")
	if codedVal == nil || codedVal.SelectAttrValue("xsi:type", "") != "CD" {
		t.Fatalf("expected xsi:type=CD on the explicit-CD field's value, got: %+v", codedVal)
	}
	// xsi:type must be the FIRST attribute (matches real CDA documents'
	// convention), even though @displayName was set on the same element by
	// a later, separate WriteAtXPath call. Attr.Key holds only the LOCAL
	// name ("type"), not the full "xsi:type" — FullKey() reconstructs the
	// prefixed form.
	if len(codedVal.Attr) < 2 || codedVal.Attr[0].FullKey() != "xsi:type" {
		t.Errorf("expected xsi:type as the first attribute, got: %+v", codedVal.Attr)
	}

	quantityVal := sectionEl.FindElement("entry/quantityResult/value")
	if quantityVal == nil || quantityVal.SelectAttrValue("xsi:type", "") != "PQ" {
		t.Errorf("expected xsi:type=PQ (disambiguated via XPathUnit) on the ANY-typed quantity field's value, got: %+v", quantityVal)
	}

	anyVal := sectionEl.FindElement("entry/anyResult/value")
	if anyVal == nil || anyVal.SelectAttrValue("xsi:type", "") != "CD" {
		t.Errorf("expected xsi:type=CD (ANY with no unit defaults to CD) on the plain ANY field's value, got: %+v", anyVal)
	}
}

// TestBuildEntry_EveryAnchorGetsGeneratedID proves ensureGeneratedID fires
// at all 4 injection points buildEntry/writeRepeatingGroups use: rootEl,
// obsEl, a StructuralTemplateIDs anchor, and a RepeatingGroup item — a real
// schema validator found every one of these missing an <id> across the
// board.
func TestBuildEntry_EveryAnchorGetsGeneratedID(t *testing.T) {
	// Real schema field/anchor paths are always fully-qualified from "entry/"
	// (e.g. allergiesAndIntolerances's own observationElementPath is
	// "entry/act/entryRelationship[...]/observation", not implicitly
	// nested under EntryElementPath's "act") — matched here so this
	// synthetic section behaves like a real one, not a shortcut that
	// happens to only work by accident.
	sec := &cdaSchema.CDASectionDef{
		Key:                    "syntheticIDs",
		EntryElementPath:       "act",
		ObservationElementPath: "act/entryRelationship[@typeCode='SUBJ']/observation",
		StructuralTemplateIDs: []cdaSchema.StructuralTemplateAnchor{
			{
				Path:       "act/entryRelationship[@typeCode='SUBJ']/observation/entryRelationship[@typeCode='REFR']/observation[templateId/@root='TID-ANCHOR']",
				TemplateID: "TID-ANCHOR",
			},
		},
		RepeatingGroups: []cdaSchema.RepeatingGroup{
			{
				// A distinct wrapper tag ("component", not "entryRelationship")
				// is deliberate: it avoids etree.FindElement's plain (non-
				// predicated) tag matching from ambiguously grabbing the OTHER
				// entryRelationship this section already has (the
				// ObservationElementPath one) when this test looks the
				// RepeatingGroup item back up below.
				Key:                    "items",
				WrapperTag:             "component",
				ObservationElementPath: "observation",
				TemplateID:             "TID-REPEATING",
			},
		},
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "anchorMarker", XPath: "act/entryRelationship[@typeCode='SUBJ']/observation/entryRelationship[@typeCode='REFR']/observation[templateId/@root='TID-ANCHOR']/value/@code"},
		},
	}
	record := map[string]interface{}{
		"anchorMarker": "marker-value",
		"items": []interface{}{
			map[string]interface{}{"itemMarker": "item-1"},
		},
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	rootEl := sectionEl.FindElement("entry/act")
	if rootEl == nil || rootEl.SelectElement("id") == nil {
		t.Error("expected a generated <id> on rootEl (act)")
	}
	obsEl := sectionEl.FindElement("entry/act/entryRelationship[@typeCode='SUBJ']/observation")
	if obsEl == nil || obsEl.SelectElement("id") == nil {
		t.Error("expected a generated <id> on obsEl (observation)")
	}
	anchorEl := sectionEl.FindElement("entry/act/entryRelationship[@typeCode='SUBJ']/observation/entryRelationship[@typeCode='REFR']/observation")
	if anchorEl == nil || anchorEl.SelectElement("id") == nil {
		t.Error("expected a generated <id> on the StructuralTemplateIDs anchor observation")
	}
	repeatingObs := sectionEl.FindElement("entry/act/component/observation")
	if repeatingObs == nil || repeatingObs.SelectElement("id") == nil {
		t.Error("expected a generated <id> on the RepeatingGroup item's observation")
	}

	// Each generated id must actually be a distinct value, not the same
	// string reused everywhere.
	ids := map[string]bool{}
	for _, el := range []*etree.Element{rootEl, obsEl, anchorEl, repeatingObs} {
		root := el.SelectElement("id").SelectAttrValue("root", "")
		if root == "" {
			t.Errorf("expected a non-empty generated id root on %s", el.Tag)
		}
		if ids[root] {
			t.Errorf("expected distinct generated ids, got a duplicate: %s", root)
		}
		ids[root] = true
	}
}

// TestBuildEntry_StructuralElementOrder_RootLevel reproduces a real bug a
// schema validator found even after the earlier templateId-only reorder
// fix: applyTagBoilerplate creates <statusCode> (for a tag with a non-empty
// StatusCode default, e.g. "act" -> "active") BEFORE templateId/id ever
// exist, so root-level elements ended up as statusCode, templateId, id —
// invalid per CDA's schema sequence (templateId, id, code, statusCode, ...).
func TestBuildEntry_StructuralElementOrder_RootLevel(t *testing.T) {
	sec := &cdaSchema.CDASectionDef{
		Key:              "syntheticOrder",
		EntryElementPath: "act",
		EntryTemplateID:  "TID-ROOT",
		EntryFixedCode:   "CONC",
		Fields: []*cdaSchema.CDAFieldDef{
			// "act/" prefix required — fields resolve relative to entryEl,
			// not rootEl (the same lesson TestBuildEntry_EveryAnchorGetsGeneratedID
			// documents above).
			{Key: "marker", XPath: "act/entryRelationship/@typeCode"},
		},
	}
	record := map[string]interface{}{"marker": "SUBJ"}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	actEl := sectionEl.FindElement("entry/act")
	if actEl == nil {
		t.Fatal("expected an <act> root element")
	}
	tags := make([]string, 0)
	for _, c := range actEl.ChildElements() {
		tags = append(tags, c.Tag)
	}
	want := []string{"templateId", "id", "code", "statusCode", "entryRelationship"}
	if len(tags) != len(want) {
		t.Fatalf("expected children %v, got %v", want, tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("child %d: expected %q, got %q (full order: %v)", i, want[i], tags[i], tags)
		}
	}
}

// TestBuildEntry_StructuralElementOrder_StructuralTemplateIDsAnchor
// reproduces the exact real-world bug: Allergy's Reaction Observation
// anchor path has a predicate (observation[templateId/@root='...']), so
// templateId gets created as a side effect of the FIRST field write that
// resolves through it (see applyPredicateConstraints) — meaning <value> was
// already this element's content before the anchor's own FixedCode/id ever
// got added, and the old templateId-only reorder left the final order as
// templateId, value, statusCode, code, id (invalid) instead of templateId,
// id, code, statusCode, value.
func TestBuildEntry_StructuralElementOrder_StructuralTemplateIDsAnchor(t *testing.T) {
	sec := &cdaSchema.CDASectionDef{
		Key:              "syntheticAnchorOrder",
		EntryElementPath: "act",
		StructuralTemplateIDs: []cdaSchema.StructuralTemplateAnchor{
			{
				Path:            "act/entryRelationship[@typeCode='MFST']/observation[templateId/@root='TID-REACTION']",
				TemplateID:      "TID-REACTION",
				FixedCode:       "ASSERTION",
				FixedCodeSystem: "2.16.840.1.113883.5.4",
			},
		},
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "reactionCode", DataType: "CD",
				XPath:        "act/entryRelationship[@typeCode='MFST']/observation[templateId/@root='TID-REACTION']/value/@code",
				XPathDisplay: "act/entryRelationship[@typeCode='MFST']/observation[templateId/@root='TID-REACTION']/value/@displayName",
			},
		},
	}
	record := map[string]interface{}{
		"reactionCode": "247472004", "reactionCodeDisplay": "Hives",
	}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	obsEl := sectionEl.FindElement("entry/act/entryRelationship/observation")
	if obsEl == nil {
		t.Fatal("expected the Reaction Observation nested element")
	}
	tags := make([]string, 0)
	for _, c := range obsEl.ChildElements() {
		tags = append(tags, c.Tag)
	}
	want := []string{"templateId", "id", "code", "statusCode", "value"}
	if len(tags) != len(want) {
		t.Fatalf("expected children %v, got %v", want, tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("child %d: expected %q, got %q (full order: %v)", i, want[i], tags[i], tags)
		}
	}
}

// TestWriteRepeatingGroups_StructuralElementOrder reproduces the same bug
// for a RepeatingGroup item (e.g. Vital Sign Observation): templateId is
// injected before the fields loop, but FixedCode/id are only added
// afterward, so the old templateId-only reorder left id/code stranded
// after value/effectiveTime.
func TestWriteRepeatingGroups_StructuralElementOrder(t *testing.T) {
	sec := syntheticRepeatingSection()

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{"vitalCode": "8480-6", "value": "120", "valueUnit": "mm[Hg]"},
		},
	})

	obsEl := sectionEl.FindElement("entry/organizer/component/observation")
	if obsEl == nil {
		t.Fatal("expected the component's nested observation")
	}
	tags := make([]string, 0)
	for _, c := range obsEl.ChildElements() {
		tags = append(tags, c.Tag)
	}
	// Two templateId elements are expected now: the dated one this
	// RepeatingGroup declares (TemplateIDExt="2014-06-09") plus its
	// R1.1-compat bare-root companion (ensureR1CompatTemplateID) — see
	// TestWriteRepeatingGroups_R1CompatTemplateID for a dedicated check of
	// that pairing's own attributes.
	want := []string{"templateId", "templateId", "id", "code", "statusCode", "value"}
	if len(tags) != len(want) {
		t.Fatalf("expected children %v, got %v", want, tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("child %d: expected %q, got %q (full order: %v)", i, want[i], tags[i], tags)
		}
	}
}

// TestWriteCodeTranslation_AddsTranslationUnderExistingCode verifies the
// basic case: el has a <code>, no <translation> yet, code is non-empty ->
// a <translation> child is appended with the given code/system/display.
func TestWriteCodeTranslation_AddsTranslationUnderExistingCode(t *testing.T) {
	el := newTestSectionEl()
	codeEl := el.CreateElement("code")
	codeEl.CreateAttr("code", "55607006")

	writeCodeTranslation(el, "75326-9", "2.16.840.1.113883.6.1", "Problem")

	trans := codeEl.SelectElement("translation")
	if trans == nil {
		t.Fatal("expected a <translation> child under <code>")
	}
	if got := trans.SelectAttrValue("code", ""); got != "75326-9" {
		t.Errorf("expected translation code 75326-9, got %q", got)
	}
	if got := trans.SelectAttrValue("codeSystem", ""); got != "2.16.840.1.113883.6.1" {
		t.Errorf("expected translation codeSystem, got %q", got)
	}
	if got := trans.SelectAttrValue("displayName", ""); got != "Problem" {
		t.Errorf("expected translation displayName, got %q", got)
	}
}

// TestWriteCodeTranslation_NoOpWhenCodeMissingOrAlreadyTranslated covers the
// two guard conditions: an empty translation code is a no-op (most sections
// don't need one), and el having no <code> at all is a no-op (never force-
// creates a container just to hold a translation).
func TestWriteCodeTranslation_NoOpWhenCodeMissingOrAlreadyTranslated(t *testing.T) {
	el := newTestSectionEl()
	writeCodeTranslation(el, "", "", "")
	if el.SelectElement("code") != nil {
		t.Error("expected no <code> to be created for an empty translation code")
	}

	codeEl := el.CreateElement("code")
	codeEl.CreateAttr("code", "55607006")
	writeCodeTranslation(el, "75326-9", "2.16.840.1.113883.6.1", "Problem")
	writeCodeTranslation(el, "99999-9", "2.16.840.1.113883.6.1", "Should not overwrite")
	translations := codeEl.SelectElements("translation")
	if len(translations) != 1 {
		t.Fatalf("expected exactly one <translation> (no duplicate on repeat call), got %d", len(translations))
	}
	if got := translations[0].SelectAttrValue("code", ""); got != "75326-9" {
		t.Errorf("expected the FIRST translation to stick, got %q", got)
	}
}

// TestEnsureAssignedEntityIDs_FallsBackToNullFlavorUnk verifies an
// assignedEntity created without an NPI-derived <id> (e.g. only name fields
// mapped) gets a nullFlavor="UNK" fallback id, reordered to be first —
// CDA's base AssignedEntity type is unconditionally SHALL [1..*] id.
func TestEnsureAssignedEntityIDs_FallsBackToNullFlavorUnk(t *testing.T) {
	entryEl := newTestSectionEl()
	ae := entryEl.CreateElement("performer").CreateElement("assignedEntity")
	person := ae.CreateElement("assignedPerson")
	person.CreateElement("name").CreateElement("given").SetText("Sarah")

	ensureAssignedEntityIDs(entryEl)

	children := ae.ChildElements()
	if len(children) < 2 || children[0].Tag != "id" {
		t.Fatalf("expected <id> to be first child of assignedEntity, got %v", elementTags(children))
	}
	if got := children[0].SelectAttrValue("nullFlavor", ""); got != "UNK" {
		t.Errorf("expected nullFlavor=UNK fallback id, got %q", got)
	}
}

// TestEnsureAssignedEntityIDs_DoesNotOverwriteRealID verifies a real,
// NPI-derived <id> is left untouched (never replaced by the nullFlavor
// fallback) — the "don't clobber real data" guard.
func TestEnsureAssignedEntityIDs_DoesNotOverwriteRealID(t *testing.T) {
	entryEl := newTestSectionEl()
	ae := entryEl.CreateElement("performer").CreateElement("assignedEntity")
	ae.CreateElement("id").CreateAttr("extension", "1234567890")

	ensureAssignedEntityIDs(entryEl)

	ids := ae.SelectElements("id")
	if len(ids) != 1 {
		t.Fatalf("expected exactly one <id> (no fallback added), got %d", len(ids))
	}
	if got := ids[0].SelectAttrValue("extension", ""); got != "1234567890" {
		t.Errorf("expected the real NPI extension preserved, got %q", got)
	}
}

// TestEnsureR1CompatTemplateID_AddsBareRootCompanion is the core mechanism
// test: a templateId with a dated extension gets a second, bare (no
// extension) sibling with the SAME root — verified against the real 2012
// R1.1 IG this session (its own "Entry Change Tracking Table" plus literal
// <templateId root="..."/> examples throughout confirm the root never
// changed between R1.1 and R2.1, only the dated @extension was added later).
func TestEnsureR1CompatTemplateID_AddsBareRootCompanion(t *testing.T) {
	el := newTestSectionEl()
	el.CreateElement("templateId").CreateAttr("root", "2.16.840.1.113883.10.20.22.4.3")
	el.SelectElement("templateId").CreateAttr("extension", "2015-08-01")

	ensureR1CompatTemplateID(el, "2.16.840.1.113883.10.20.22.4.3", "2015-08-01")

	tids := el.SelectElements("templateId")
	if len(tids) != 2 {
		t.Fatalf("expected 2 templateId elements, got %d", len(tids))
	}
	if got := tids[1].SelectAttrValue("root", ""); got != "2.16.840.1.113883.10.20.22.4.3" {
		t.Errorf("expected compat companion's root to match, got %q", got)
	}
	if tids[1].SelectAttrValue("extension", "") != "" {
		t.Errorf("expected compat companion to have NO extension, got %q", tids[1].SelectAttrValue("extension", ""))
	}
}

// TestEnsureR1CompatTemplateID_NoOpWhenExtensionEmpty verifies a genuinely
// new, R2.1-only template (no extension in this schema's own data, e.g.
// Nutrition — confirmed via a zero-hit search of the entire R1.1 IG for its
// OIDs) never gets a spurious compat companion.
func TestEnsureR1CompatTemplateID_NoOpWhenExtensionEmpty(t *testing.T) {
	el := newTestSectionEl()
	el.CreateElement("templateId").CreateAttr("root", "2.16.840.1.113883.10.20.22.4.124")

	ensureR1CompatTemplateID(el, "2.16.840.1.113883.10.20.22.4.124", "")

	if len(el.SelectElements("templateId")) != 1 {
		t.Errorf("expected no companion added when extension is empty, got %d templateId elements", len(el.SelectElements("templateId")))
	}
}

// TestEnsureR1CompatTemplateID_DoesNotDuplicateExistingBareCompanion covers
// the StructuralTemplateIDs anchor case: a predicate-driven field write can
// already have created a bare (root-only) templateId as a side effect
// (applyPredicateConstraints) BEFORE injectTemplateID ever runs — the compat
// step must reuse that existing bare element, never add a redundant third.
func TestEnsureR1CompatTemplateID_DoesNotDuplicateExistingBareCompanion(t *testing.T) {
	el := newTestSectionEl()
	el.CreateElement("templateId").CreateAttr("root", "2.16.840.1.113883.10.20.22.4.8") // pre-existing bare one

	ensureR1CompatTemplateID(el, "2.16.840.1.113883.10.20.22.4.8", "2014-06-09")

	if len(el.SelectElements("templateId")) != 1 {
		t.Errorf("expected the pre-existing bare templateId to be reused, not duplicated, got %d", len(el.SelectElements("templateId")))
	}
}

// TestInjectTemplateID_WiresInR1Compat proves injectTemplateID itself (not
// just the standalone helper) produces both elements — this is what every
// entry/obs/anchor/RepeatingGroup call site actually relies on.
func TestInjectTemplateID_WiresInR1Compat(t *testing.T) {
	el := newTestSectionEl()
	injectTemplateID(el, "2.16.840.1.113883.10.20.22.4.16", "2014-06-09")

	tids := el.SelectElements("templateId")
	if len(tids) != 2 {
		t.Fatalf("expected 2 templateId elements from one injectTemplateID call, got %d", len(tids))
	}
	withExt, bare := tids[0], tids[1]
	if withExt.SelectAttrValue("extension", "") != "2014-06-09" {
		t.Errorf("expected first templateId to carry the extension, got %+v", withExt)
	}
	if bare.SelectAttrValue("extension", "") != "" {
		t.Errorf("expected second templateId to be bare, got %+v", bare)
	}
}

// TestIsEmptyPlaceholder_Cases documents the exact emptiness rule
// removeEmptyPlaceholderRoot relies on: no <code>, no <value>, and no
// children beyond the universal templateId/id/statusCode boilerplate.
func TestIsEmptyPlaceholder_Cases(t *testing.T) {
	empty := newTestSectionEl()
	empty.CreateElement("templateId")
	empty.CreateElement("id")
	empty.CreateElement("statusCode")
	if !isEmptyPlaceholder(empty) {
		t.Error("expected boilerplate-only element to be treated as empty")
	}

	withCode := newTestSectionEl()
	withCode.CreateElement("code")
	if isEmptyPlaceholder(withCode) {
		t.Error("expected an element with <code> to NOT be treated as empty")
	}

	withOtherChild := newTestSectionEl()
	withOtherChild.CreateElement("component") // e.g. a RepeatingGroup wrapper
	if isEmptyPlaceholder(withOtherChild) {
		t.Error("expected an element with a non-boilerplate child to NOT be treated as empty")
	}
}

// TestRemoveEmptyPlaceholderRoot_RemovesWhenSiblingCarriesRealData is the
// Social History regression case: a bare rootEl the section's own
// entryElementPath unconditionally created, sitting empty next to a REAL
// second observation a predicated field wrote instead — the empty one must
// be dropped, since <entry> only permits exactly one clinical statement.
func TestRemoveEmptyPlaceholderRoot_RemovesWhenSiblingCarriesRealData(t *testing.T) {
	entryEl := newTestSectionEl()
	rootEl := entryEl.CreateElement("observation")
	rootEl.CreateElement("templateId")
	rootEl.CreateElement("id")
	rootEl.CreateElement("statusCode")
	real := entryEl.CreateElement("observation")
	real.CreateElement("code").CreateAttr("code", "11367-0")

	removeEmptyPlaceholderRoot(entryEl, rootEl)

	if len(entryEl.ChildElements()) != 1 {
		t.Fatalf("expected only the real observation to remain, got %d children", len(entryEl.ChildElements()))
	}
	if entryEl.ChildElements()[0] != real {
		t.Error("expected the surviving child to be the real observation, not the empty placeholder")
	}
}

// TestRemoveEmptyPlaceholderRoot_KeepsSoleChildEvenIfEmpty verifies a
// section whose rootEl is its ONLY child under <entry> is never removed,
// even if genuinely empty (e.g. a record with zero mappable data) — dropping
// it would leave a completely empty <entry>, a different, unrelated problem
// this fix isn't meant to solve.
func TestRemoveEmptyPlaceholderRoot_KeepsSoleChildEvenIfEmpty(t *testing.T) {
	entryEl := newTestSectionEl()
	rootEl := entryEl.CreateElement("observation")
	rootEl.CreateElement("templateId")
	rootEl.CreateElement("id")
	rootEl.CreateElement("statusCode")

	removeEmptyPlaceholderRoot(entryEl, rootEl)

	if len(entryEl.ChildElements()) != 1 {
		t.Errorf("expected the sole (even if empty) child to be kept, got %d children", len(entryEl.ChildElements()))
	}
}

// TestInjectCodeSystemName_KnownOID verifies a recognized codeSystem OID
// gets its conventional codeSystemName label auto-derived — e.g. real IG
// worked examples (Figure 219, Severity Observation) show
// codeSystem="2.16.840.1.113883.6.96" codeSystemName="SNOMED CT" together;
// this engine only ever wrote the codeSystem half.
func TestInjectCodeSystemName_KnownOID(t *testing.T) {
	el := newTestSectionEl()
	el.CreateAttr("codeSystem", "2.16.840.1.113883.6.96")
	injectCodeSystemName(el, "2.16.840.1.113883.6.96")
	if got := el.SelectAttrValue("codeSystemName", ""); got != "SNOMED CT" {
		t.Errorf("codeSystemName = %q, want SNOMED CT", got)
	}
}

// TestInjectCodeSystemName_UnknownOID_NoOp verifies an unrecognized OID is
// left alone rather than guessed at.
func TestInjectCodeSystemName_UnknownOID_NoOp(t *testing.T) {
	el := newTestSectionEl()
	injectCodeSystemName(el, "2.16.840.1.99999.1.2.3")
	if got := el.SelectAttrValue("codeSystemName", ""); got != "" {
		t.Errorf("expected no codeSystemName for an unrecognized OID, got %q", got)
	}
}

// TestInjectCodeSystemName_DoesNotOverwriteExisting verifies a real,
// per-record codeSystemName (if a section ever supplies one explicitly) is
// never clobbered by the auto-derived label.
func TestInjectCodeSystemName_DoesNotOverwriteExisting(t *testing.T) {
	el := newTestSectionEl()
	el.CreateAttr("codeSystemName", "Custom Label")
	injectCodeSystemName(el, "2.16.840.1.113883.6.96")
	if got := el.SelectAttrValue("codeSystemName", ""); got != "Custom Label" {
		t.Errorf("expected existing codeSystemName preserved, got %q", got)
	}
}

func elementTags(els []*etree.Element) []string {
	tags := make([]string, len(els))
	for i, e := range els {
		tags[i] = e.Tag
	}
	return tags
}

// TestBuildAlternateEntry_ProducesOwnRootShapeWithFields is an isolated,
// synthetic-section spike for buildAlternateEntry (Medical Equipment's
// Procedure Activity Procedure2 second entry shape is the real motivating
// case — see AlternateEntryArchetype's own doc comment) — proven on its own
// before depending on the real schema/BuildDocument path (already covered
// end-to-end in document_builder_test.go).
func TestBuildAlternateEntry_ProducesOwnRootShapeWithFields(t *testing.T) {
	alt := &cdaSchema.AlternateEntryArchetype{
		EntriesKey:         "procedureEntries",
		EntryElementPath:   "procedure",
		EntryTemplateID:    "2.16.840.1.113883.10.20.22.4.14",
		EntryTemplateIDExt: "2014-06-09",
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "code", XPath: "procedure/code/@code"},
			{Key: "status", XPath: "procedure/statusCode/@code"},
		},
	}
	record := map[string]interface{}{"code": "87717006", "status": "completed"}

	sectionEl := newTestSectionEl()
	entryEl := buildAlternateEntry(sectionEl, alt, record)
	if entryEl == nil {
		t.Fatal("expected a non-nil <entry> element")
	}

	procEl := entryEl.SelectElement("procedure")
	if procEl == nil {
		t.Fatalf("expected a <procedure> root element, got: %+v", entryEl.ChildElements())
	}
	if procEl.SelectAttrValue("classCode", "") != "PROC" || procEl.SelectAttrValue("moodCode", "") != "EVN" {
		t.Errorf("expected procedure tag boilerplate classCode=PROC moodCode=EVN, got: %+v", procEl.Attr)
	}
	tid := procEl.SelectElement("templateId")
	if tid == nil || tid.SelectAttrValue("root", "") != "2.16.840.1.113883.10.20.22.4.14" || tid.SelectAttrValue("extension", "") != "2014-06-09" {
		t.Errorf("expected Procedure Activity Procedure2's templateId, got: %+v", tid)
	}
	if procEl.SelectElement("id") == nil {
		t.Error("expected a generated <id>")
	}
	codeEl := procEl.SelectElement("code")
	if codeEl == nil || codeEl.SelectAttrValue("code", "") != "87717006" {
		t.Errorf("expected code=87717006, got: %+v", codeEl)
	}
	statusEl := procEl.SelectElement("statusCode")
	if statusEl == nil || statusEl.SelectAttrValue("code", "") != "completed" {
		t.Errorf("expected statusCode=completed, got: %+v", statusEl)
	}
}

// TestBuildEntry_StructuralTemplateAnchor_FixedStatusCode_OverridesTagDefault
// verifies StructuralTemplateAnchor.FixedStatusCode (Immunization's nested
// Substance Administered Act, CONF:1098-31505, SHALL statusCode="completed"
// even though a bare "act" tag's own tagBoilerplate default is "active") —
// isolated from the full immunizations schema so this one mechanism is
// proven on its own.
func TestBuildEntry_StructuralTemplateAnchor_FixedStatusCode_OverridesTagDefault(t *testing.T) {
	sec := &cdaSchema.CDASectionDef{
		Key:              "syntheticSeries",
		EntryElementPath: "substanceAdministration",
		StructuralTemplateIDs: []cdaSchema.StructuralTemplateAnchor{
			{
				Path:             "substanceAdministration/entryRelationship[@typeCode='COMP',@inversionInd='true']/act",
				TemplateID:       "2.16.840.1.113883.10.20.22.4.118",
				FixedCode:        "416118004",
				FixedCodeSystem:  "2.16.840.1.113883.6.96",
				FixedCodeDisplay: "Administration",
				FixedStatusCode:  "completed",
			},
		},
		Fields: []*cdaSchema.CDAFieldDef{
			{Key: "seriesDate", XPath: "substanceAdministration/entryRelationship[@typeCode='COMP',@inversionInd='true']/act/effectiveTime/@value"},
		},
	}
	record := map[string]interface{}{"seriesDate": "20240115"}

	sectionEl := newTestSectionEl()
	buildEntry(sectionEl, sec, record)

	actEl := sectionEl.FindElement("entry/substanceAdministration/entryRelationship/act")
	if actEl == nil {
		doc := etree.NewDocument()
		doc.SetRoot(sectionEl.Copy())
		xml, _ := doc.WriteToString()
		t.Fatalf("expected a nested <act>, got:\n%s", xml)
	}
	if actEl.SelectAttrValue("classCode", "") != "ACT" || actEl.SelectAttrValue("moodCode", "") != "EVN" {
		t.Errorf("expected act tag boilerplate classCode=ACT moodCode=EVN, got: %+v", actEl.Attr)
	}
	statusEl := actEl.SelectElement("statusCode")
	if statusEl == nil {
		t.Fatal("expected a <statusCode> element")
	}
	if got := statusEl.SelectAttrValue("code", ""); got != "completed" {
		t.Errorf("expected FixedStatusCode override to produce statusCode=completed (not act's default \"active\"), got %q", got)
	}
	codeEl := actEl.SelectElement("code")
	if codeEl == nil || codeEl.SelectAttrValue("code", "") != "416118004" || codeEl.SelectAttrValue("codeSystem", "") != "2.16.840.1.113883.6.96" {
		t.Errorf("expected the anchor's FixedCode (416118004/SNOMED), got: %+v", codeEl)
	}
	seqEl := sectionEl.FindElement("entry/substanceAdministration/entryRelationship")
	if seqEl == nil || seqEl.SelectAttrValue("typeCode", "") != "COMP" || seqEl.SelectAttrValue("inversionInd", "") != "true" {
		t.Errorf("expected entryRelationship typeCode=COMP inversionInd=true, got: %+v", seqEl)
	}
}
