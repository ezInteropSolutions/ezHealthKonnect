// services/cda_coverage/report_test.go
package cdacoverage

import "testing"

func TestBuildReport_AllTouched_FullCoverage(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Medications", SectionKey: "medications", SectionTitle: "Medications", EntryIndex: 0},
		{Category: "Medications", SectionKey: "medications", SectionTitle: "Medications", EntryIndex: 1},
	}
	touched := map[string]struct{}{"medications#0": {}, "medications#1": {}}

	report := BuildReport(inventory, touched)
	if report.OverallCoveragePct != 100 {
		t.Errorf("OverallCoveragePct = %v, want 100", report.OverallCoveragePct)
	}
	if report.HasGaps() {
		t.Error("HasGaps() = true, want false when everything was touched")
	}
	if len(report.Categories) != 1 || report.Categories[0].Found != 2 || report.Categories[0].Total != 2 {
		t.Errorf("unexpected category stats: %+v", report.Categories)
	}
	if report.ElementCoveragePct != nil {
		t.Errorf("ElementCoveragePct = %v, want nil (no element-level data in this entry-only inventory)", *report.ElementCoveragePct)
	}
}

func TestBuildReport_PartialCoverage_ListsMissed(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Medications", SectionKey: "medications", SectionTitle: "Medications", EntryIndex: 0},
		{Category: "Medications", SectionKey: "medications", SectionTitle: "Medications", EntryIndex: 1},
		{Category: "Medications", SectionKey: "medications", SectionTitle: "Medications", EntryIndex: 2},
	}
	touched := map[string]struct{}{"medications#0": {}, "medications#2": {}}

	report := BuildReport(inventory, touched)
	if got := report.OverallCoveragePct; got < 66 || got > 67 {
		t.Errorf("OverallCoveragePct = %v, want ~66.67", got)
	}
	if !report.HasGaps() {
		t.Fatal("HasGaps() = false, want true")
	}
	if len(report.Categories) != 1 {
		t.Fatalf("got %d categories, want 1", len(report.Categories))
	}
	cat := report.Categories[0]
	if cat.Found != 2 || cat.Total != 3 {
		t.Errorf("Found=%d Total=%d, want Found=2 Total=3", cat.Found, cat.Total)
	}
	if len(cat.Missed) != 1 || cat.Missed[0].EntryIndex != 1 {
		t.Errorf("Missed = %+v, want exactly entryIndex=1", cat.Missed)
	}
	// Structural location only — never a clinical value.
	if cat.Missed[0].SectionKey != "medications" || cat.Missed[0].SectionTitle != "Medications" {
		t.Errorf("Missed[0] location fields wrong: %+v", cat.Missed[0])
	}
}

func TestBuildReport_UnclassifiedSection_AlwaysMissed(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Unclassified", SectionKey: "", SectionTitle: "Advance Directives", EntryIndex: 0},
	}
	// Even an empty touched set, or one containing an unrelated key, must never
	// mark an unclassified item (TrackingKey() == "") as found.
	touched := map[string]struct{}{"": {}, "medications#0": {}}

	report := BuildReport(inventory, touched)
	if report.OverallCoveragePct != 0 {
		t.Errorf("OverallCoveragePct = %v, want 0", report.OverallCoveragePct)
	}
	if len(report.Categories) != 1 || len(report.Categories[0].Missed) != 1 {
		t.Fatalf("unexpected report: %+v", report.Categories)
	}
}

func TestBuildReport_EmptyInventory(t *testing.T) {
	report := BuildReport(nil, map[string]struct{}{})
	if report.OverallCoveragePct != 100 {
		t.Errorf("OverallCoveragePct for empty inventory = %v, want 100 (nothing to miss)", report.OverallCoveragePct)
	}
	if report.HasGaps() {
		t.Error("HasGaps() = true for an empty inventory, want false")
	}
	if len(report.Categories) != 0 {
		t.Errorf("got %d categories, want 0", len(report.Categories))
	}
}

func TestBuildReport_SchemaVersion(t *testing.T) {
	report := BuildReport(nil, map[string]struct{}{})
	if report.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", report.SchemaVersion, currentSchemaVersion)
	}
}

// TestBuildReport_ElementLevel_PartiallyMissedEntry is the core proof for
// element granularity: an entry that IS touched at entry level (counts
// toward Found, same as before) can still surface its own missed elements
// separately -- the whole reason this was escalated past entry-level.
func TestBuildReport_ElementLevel_PartiallyMissedEntry(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Vital Signs", SectionKey: "vitalSigns", SectionTitle: "Vital Signs", EntryIndex: 0},
		{Category: "Vital Signs", SectionKey: "vitalSigns", SectionTitle: "Vital Signs", EntryIndex: 0, ElementPath: "code[0]"},
		{Category: "Vital Signs", SectionKey: "vitalSigns", SectionTitle: "Vital Signs", EntryIndex: 0, ElementPath: "statusCode[0]"},
		{Category: "Vital Signs", SectionKey: "vitalSigns", SectionTitle: "Vital Signs", EntryIndex: 0, ElementPath: "performer[0]"},
	}
	// Entry itself touched (via its "code" field, in practice), AND code
	// element specifically touched -- statusCode/performer never recorded.
	touched := map[string]struct{}{
		"vitalSigns#0":         {},
		"vitalSigns#0/code[0]": {},
	}

	report := BuildReport(inventory, touched)
	if len(report.Categories) != 1 {
		t.Fatalf("got %d categories, want 1", len(report.Categories))
	}
	cat := report.Categories[0]

	// Entry-level accounting is UNCHANGED by element data -- this entry is
	// still simply "found," same as it would be with no element items at all.
	if cat.Found != 1 || cat.Total != 1 {
		t.Errorf("entry-level Found=%d Total=%d, want Found=1 Total=1 (element data must not change entry-level counts)", cat.Found, cat.Total)
	}

	// This is EXACTLY the shape a real user reported as confusing/wrong (a
	// 2026-08-08 production report): entry-level says 100% (Found==Total)
	// while real element-level gaps still exist. CategoryStat.ElementsFound/
	// Total and Report.ElementCoveragePct are the fix -- a UI should prefer
	// these once they're present, since they correctly show 50%, not 100%.
	if cat.ElementsFound != 1 || cat.ElementsTotal != 2 {
		t.Errorf("category ElementsFound=%d ElementsTotal=%d, want 1/2 (summed clinical counts across this category's entries)", cat.ElementsFound, cat.ElementsTotal)
	}
	if report.ElementCoveragePct == nil {
		t.Fatal("ElementCoveragePct = nil, want a real percentage since this document has element-level data")
	}
	if *report.ElementCoveragePct != 50 {
		t.Errorf("ElementCoveragePct = %v, want 50 (1/2 clinical elements found) -- NOT 100 (the misleading entry-level OverallCoveragePct)", *report.ElementCoveragePct)
	}

	// But it still shows up in Missed, WITH element detail, since it has
	// genuine element-level gaps.
	if len(cat.Missed) != 1 {
		t.Fatalf("got %d missed item(s), want 1 (the entry, for its element gaps)", len(cat.Missed))
	}
	item := cat.Missed[0]
	// Clinical-only pair excludes "performer[0]" (a CDA RIM Participation
	// wrapper -- see element_classifier.go): code (found) + statusCode
	// (missed) = 2 clinical elements, 1 found.
	if item.ElementsFound != 1 || item.ElementsTotal != 2 {
		t.Errorf("ElementsFound=%d ElementsTotal=%d, want 1/2 (clinical-only, performer excluded)", item.ElementsFound, item.ElementsTotal)
	}
	// Raw ground-truth pair still counts all 3 (code, statusCode, performer).
	if item.ElementsFoundAll != 1 || item.ElementsTotalAll != 3 {
		t.Errorf("ElementsFoundAll=%d ElementsTotalAll=%d, want 1/3 (raw, unfiltered)", item.ElementsFoundAll, item.ElementsTotalAll)
	}
	if len(item.MissedElements) != 2 {
		t.Fatalf("got %d missed elements, want 2 (statusCode, performer) -- got %+v", len(item.MissedElements), item.MissedElements)
	}
	gotPaths := map[string]bool{}
	gotAdmin := map[string]bool{}
	for _, g := range item.MissedElements {
		gotPaths[g.Path] = true
		gotAdmin[g.Path] = g.Administrative
		if g.Label == "" {
			t.Errorf("ElementGap %q has no Label", g.Path)
		}
	}
	if !gotPaths["statusCode[0]"] || !gotPaths["performer[0]"] {
		t.Errorf("MissedElements = %+v, want statusCode[0] and performer[0]", item.MissedElements)
	}
	if gotAdmin["statusCode[0]"] {
		t.Errorf("statusCode[0] incorrectly tagged Administrative")
	}
	if !gotAdmin["performer[0]"] {
		t.Errorf("performer[0] should be tagged Administrative (CDA RIM Participation wrapper)")
	}
}

// TestBuildReport_ElementLevel_FullyTouchedEntry_NoMissedEntry proves the
// converse: an entry with element items, ALL of which are touched, must not
// appear in Missed at all (not even with an empty MissedElements list) --
// only entries with something to report belong there.
func TestBuildReport_ElementLevel_FullyTouchedEntry_NoMissedEntry(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0},
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0, ElementPath: "code[0]"},
	}
	touched := map[string]struct{}{"vitalSigns#0": {}, "vitalSigns#0/code[0]": {}}

	report := BuildReport(inventory, touched)
	if report.HasGaps() {
		t.Errorf("HasGaps() = true, want false -- every item (entry and its one element) was touched")
	}
	if len(report.Categories[0].Missed) != 0 {
		t.Errorf("Missed = %+v, want empty", report.Categories[0].Missed)
	}
}

// TestBuildReport_ElementLevel_RollUp_AncestorCoversDescendants proves
// isCovered's roll-up: a rule that reads a composite element as one opaque
// node (e.g. a cda_timerange_to_onset-style transform reading
// "effectiveTime[0]" whole, never touching "effectiveTime[0].low[0]"
// individually) must not make the inventory's separately-enumerated child
// elements show up as false gaps.
func TestBuildReport_ElementLevel_RollUp_AncestorCoversDescendants(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0},
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0, ElementPath: "effectiveTime[0]"},
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0, ElementPath: "effectiveTime[0].low[0]"},
		{Category: "Vital Signs", SectionKey: "vitalSigns", EntryIndex: 0, ElementPath: "effectiveTime[0].high[0]"},
	}
	// Only the ancestor ("effectiveTime[0]") is directly recorded -- low/high
	// were never individually touched, but must roll up as covered anyway.
	touched := map[string]struct{}{
		"vitalSigns#0":                   {},
		"vitalSigns#0/effectiveTime[0]": {},
	}

	report := BuildReport(inventory, touched)
	if report.HasGaps() {
		t.Errorf("HasGaps() = true, want false -- low/high must roll up as covered via their touched ancestor")
	}
	item := report.Categories[0]
	if item.Total != 1 || item.Found != 1 {
		t.Fatalf("unexpected entry-level stats: %+v", item)
	}
}

func TestBuildReport_CategoryOrder_IsFirstSeenInInventory(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Problems", SectionKey: "problems", EntryIndex: 0},
		{Category: "Medications", SectionKey: "medications", EntryIndex: 0},
		{Category: "Problems", SectionKey: "problems", EntryIndex: 1},
	}
	report := BuildReport(inventory, map[string]struct{}{})
	if len(report.Categories) != 2 {
		t.Fatalf("got %d categories, want 2", len(report.Categories))
	}
	if report.Categories[0].Category != "Problems" || report.Categories[1].Category != "Medications" {
		t.Errorf("category order = [%s, %s], want [Problems, Medications] (first-seen order)",
			report.Categories[0].Category, report.Categories[1].Category)
	}
	if report.Categories[0].Total != 2 {
		t.Errorf("Problems.Total = %d, want 2", report.Categories[0].Total)
	}
}

func TestBuildReport_USCDIClasses_CarriedFromInventoryItems(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Medications", SectionKey: "medications", EntryIndex: 0, USCDIClasses: []string{"Medications"}},
		{Category: "Medications", SectionKey: "medications", EntryIndex: 1, USCDIClasses: []string{"Medications"}},
	}
	report := BuildReport(inventory, map[string]struct{}{})
	if len(report.Categories) != 1 {
		t.Fatalf("got %d categories, want 1", len(report.Categories))
	}
	got := report.Categories[0].USCDIClasses
	if len(got) != 1 || got[0] != "Medications" {
		t.Errorf("USCDIClasses = %v, want [Medications]", got)
	}
}

// TestBuildReport_USCDIClasses_UnionedAcrossEntriesInSameCategory covers the
// rare-but-real case a section's own class set differs entry-to-entry within
// the SAME category grouping (Category, not SectionKey, is what BuildReport
// groups by) — the result must be the deduped union, not just the first
// entry's set silently winning.
func TestBuildReport_USCDIClasses_UnionedAcrossEntriesInSameCategory(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Social History", SectionKey: "socialHistory", EntryIndex: 0, USCDIClasses: []string{"Health Status Assessments"}},
		{Category: "Social History", SectionKey: "socialHistory", EntryIndex: 1, USCDIClasses: []string{"Care Plan", "Health Status Assessments"}},
	}
	report := BuildReport(inventory, map[string]struct{}{})
	got := report.Categories[0].USCDIClasses
	want := map[string]bool{"Care Plan": true, "Health Status Assessments": true}
	if len(got) != len(want) {
		t.Fatalf("USCDIClasses = %v, want %d deduped classes", got, len(want))
	}
	for _, class := range got {
		if !want[class] {
			t.Errorf("unexpected class %q, want only %v", class, want)
		}
	}
}

// TestBuildReport_USCDIClasses_NilWhenItemsHaveNone guards the "absence
// isn't an error" contract: a report built from items with no USCDIClasses
// (no vocabulary supplied, or the section isn't in uscdi_v3.json yet) must
// omit the field entirely (nil, not an empty-but-present slice) so it reads
// identically to a pre-schema-version-5 report — matching currentSchemaVersion's
// own additive-only discipline.
func TestBuildReport_USCDIClasses_NilWhenItemsHaveNone(t *testing.T) {
	inventory := []InventoryItem{
		{Category: "Problems", SectionKey: "problems", EntryIndex: 0},
	}
	report := BuildReport(inventory, map[string]struct{}{})
	if report.Categories[0].USCDIClasses != nil {
		t.Errorf("USCDIClasses = %v, want nil", report.Categories[0].USCDIClasses)
	}
}

func TestReport_SchemaVersion_Is5(t *testing.T) {
	report := BuildReport(nil, map[string]struct{}{})
	if report.SchemaVersion != 5 {
		t.Errorf("SchemaVersion = %d, want 5 (USCDI v3 class bridging) — if this changed intentionally, "+
			"update this test's expectation and currentSchemaVersion's own doc comment together", report.SchemaVersion)
	}
}
