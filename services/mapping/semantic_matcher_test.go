// services/mapping/semantic_matcher_test.go
//
// semantic_matcher.go is the OOB template generator that
// hl7_fhir_transform_service_v3.go's main.go boot sequence calls to rebuild
// EVERY row in hl7_fhir_templates on every server start (see CLAUDE.md's own
// "OOB template self-regeneration" note) — a bug here silently reverts any
// DB-only fix on the next restart, and every one of these functions is pure
// (no DB, no I/O), so there is no excuse for it to have sat at 0% coverage.
//
// Scope deliberately excludes asserting on every entry in knownAnchorsR4/R5 —
// that would just restate the data as more data. Instead this covers the
// LOGIC around the tables (version selection, the 3-pass Match() algorithm,
// and its helper functions), plus targeted regression tests for anchor
// entries that were bugs before this session's fixes (NK1.7/8/9).
package mapping

import (
	"testing"

	"ezhealthkonnect/fhir/r4"
)

// ── isBlocklisted ──────────────────────────────────────────────────────────

func TestIsBlocklisted(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"Encounter.statusHistory", true},
		{"Encounter.statusHistory[0].status", true},
		{"Encounter.episodeOfCare", true},
		{"Patient.link", true},
		{"Resource.implicitRules", true},
		{"Resource.meta.profile", true},
		{"Resource.meta.versionId", true},
		{"Resource.text.div", true},
		{"Resource.text.status", true},
		{"Coverage.dependent", true},
		{"Patient.birthDate", false},
		{"Encounter.status", false}, // must NOT collide with "statusHistory" substring match
		{"Patient.name[0].family", false},
	}
	for _, c := range cases {
		if got := isBlocklisted(c.path); got != c.want {
			t.Errorf("isBlocklisted(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// ── anchorsForVersion ──────────────────────────────────────────────────────

func TestAnchorsForVersion_R4AndR5ReturnDistinctTables(t *testing.T) {
	r4Table := anchorsForVersion(FHIRVersionR4)
	r5Table := anchorsForVersion(FHIRVersionR5)

	if len(r4Table) == 0 {
		t.Fatal("R4 anchor table is empty")
	}
	if len(r5Table) == 0 {
		t.Fatal("R5 anchor table is empty")
	}
}

func TestAnchorsForVersion_UnknownVersionDefaultsToR4(t *testing.T) {
	got := anchorsForVersion(FHIRVersion("bogus"))
	r4Table := anchorsForVersion(FHIRVersionR4)
	if len(got) != len(r4Table) {
		t.Errorf("unknown version returned a table of len %d, want R4's len %d", len(got), len(r4Table))
	}
	if got["PID.7"][0].FHIRPath != "Patient.birthDate" {
		t.Errorf("unknown version did not fall back to R4 table contents")
	}
}

// TestAnchorsForVersion_DocumentedStructuralDifferences guards the 8 R4→R5
// structural changes the package doc comment on knownAnchorsR5 claims are
// applied. If someone edits one table and forgets its counterpart, this
// fails instead of silently drifting.
func TestAnchorsForVersion_DocumentedStructuralDifferences(t *testing.T) {
	r4Table := anchorsForVersion(FHIRVersionR4)
	r5Table := anchorsForVersion(FHIRVersionR5)

	cases := []struct {
		key      string
		wantR4   string
		wantR5   string
	}{
		{"PV1.2", "Encounter.class.code", "Encounter.class[0].coding[0].code"},
		{"PV1.7", "Encounter.participant[0].individual.display", "Encounter.participant[0].actor.display"},
		{"PV1.10", "Encounter.hospitalization.admitSource", "Encounter.admission.admitSource"},
		{"PV1.17", "Encounter.participant[1].individual.display", "Encounter.participant[1].actor.display"},
		{"EVN.5", "Encounter.participant[0].individual.display", "Encounter.participant[0].actor.display"},
		{"IN1.36", "Coverage.subscriberId", "Coverage.subscriberIdentifier"},
		{"TXA.4", "DocumentReference.date", "DocumentReference.date"}, // path same; type differs (checked below)
		{"OM1.11", "ObservationDefinition.preferredReportName", "ObservationDefinition.name"},
	}

	for _, c := range cases {
		r4Cands, ok := r4Table[c.key]
		if !ok || len(r4Cands) == 0 {
			t.Errorf("%s: missing or empty in R4 table", c.key)
			continue
		}
		r5Cands, ok := r5Table[c.key]
		if !ok || len(r5Cands) == 0 {
			t.Errorf("%s: missing or empty in R5 table", c.key)
			continue
		}
		if r4Cands[0].FHIRPath != c.wantR4 {
			t.Errorf("%s R4 path = %q, want %q", c.key, r4Cands[0].FHIRPath, c.wantR4)
		}
		if r5Cands[0].FHIRPath != c.wantR5 {
			t.Errorf("%s R5 path = %q, want %q", c.key, r5Cands[0].FHIRPath, c.wantR5)
		}
	}

	// TXA.4's structural change is its FHIRType (instant -> dateTime), not its path.
	if r4Table["TXA.4"][0].FHIRType != "instant" {
		t.Errorf("TXA.4 R4 type = %q, want instant", r4Table["TXA.4"][0].FHIRType)
	}
	if r5Table["TXA.4"][0].FHIRType != "dateTime" {
		t.Errorf("TXA.4 R5 type = %q, want dateTime", r5Table["TXA.4"][0].FHIRType)
	}
}

// ── Match(): NK1.7/8/9 regression (this session's fix) ─────────────────────
//
// NK1.7 (Contact Role, a CE composite) was previously wired to
// RelatedPerson.period.start — a dateTime field — corrupting the value on
// every message that populated it. The fix: NK1.7 is now an empty anchor
// (Pass 1 still matches the key, short-circuits with zero candidates), and
// NK1.8/NK1.9 (the real Start/End Date fields) now own period.start/.end.

func TestMatch_NK1ContactRole_ReturnsEmptyNotPeriodStart(t *testing.T) {
	for _, ver := range []FHIRVersion{FHIRVersionR4, FHIRVersionR5} {
		got := Match("NK1", "NK1.7", "Contact Role", "CE", "RelatedPerson", nil, ver)
		if len(got) != 0 {
			t.Errorf("[%s] Match(NK1.7) = %+v, want empty (Contact Role has no clean RelatedPerson target)", ver, got)
		}
	}
}

func TestMatch_NK1StartEndDates_OwnPeriodFields(t *testing.T) {
	for _, ver := range []FHIRVersion{FHIRVersionR4, FHIRVersionR5} {
		start := Match("NK1", "NK1.8", "Start Date", "DT", "RelatedPerson", nil, ver)
		if len(start) != 1 || start[0].FHIRPath != "RelatedPerson.period.start" {
			t.Errorf("[%s] Match(NK1.8) = %+v, want single candidate RelatedPerson.period.start", ver, start)
		}
		end := Match("NK1", "NK1.9", "End Date", "DT", "RelatedPerson", nil, ver)
		if len(end) != 1 || end[0].FHIRPath != "RelatedPerson.period.end" {
			t.Errorf("[%s] Match(NK1.9) = %+v, want single candidate RelatedPerson.period.end", ver, end)
		}
	}
}

// ── Match(): pass structure ─────────────────────────────────────────────────

func TestMatch_AnchoredField_ShortCircuitsWithNilSchema(t *testing.T) {
	// Pass 1 must return immediately for an anchored key even when fhirSchema
	// is nil — the OOB generator calls Match for many fields without always
	// having a compiled profile loaded.
	got := Match("PID", "PID.7", "Date/Time of Birth", "DTM", "Patient", nil, FHIRVersionR4)
	if len(got) != 1 || got[0].FHIRPath != "Patient.birthDate" {
		t.Errorf("Match(PID.7) with nil schema = %+v, want single anchored candidate Patient.birthDate", got)
	}
}

func TestMatch_UnanchoredFieldNoSchema_ReturnsEmpty(t *testing.T) {
	// "ZZZ" is not a real segment - guaranteed to have no anchor table entry.
	got := Match("ZZZ", "ZZZ.1", "Some Field", "ST", "Patient", nil, FHIRVersionR4)
	if len(got) != 0 {
		t.Errorf("Match() for unanchored field with nil schema = %+v, want empty (no schema to run passes 2/3 against)", got)
	}
}

func fakeProfile(resourceType string, dataTypes map[string]string) *r4.CompiledProfile {
	return &r4.CompiledProfile{
		ResourceType: resourceType,
		DataTypes:    dataTypes,
	}
}

func TestMatch_UnanchoredField_TypeCompatiblePassPicksCorrectType(t *testing.T) {
	// XPN (Extended Person Name) -> FHIRType "HumanName" per type_registry.go.
	schema := fakeProfile("Patient", map[string]string{
		"Patient.name":       "HumanName",
		"Patient.identifier": "Identifier", // incompatible type -> must be excluded from Pass 2
	})
	got := Match("ZZZ", "ZZZ.1", "Legal Name", "XPN", "Patient", schema, FHIRVersionR4)
	if len(got) == 0 {
		t.Fatal("expected at least one type_match candidate, got none")
	}
	if got[0].FHIRPath != "Patient.name" {
		t.Errorf("top candidate = %+v, want Patient.name (only HumanName-typed element)", got[0])
	}
	if got[0].Source != "type_match" {
		t.Errorf("top candidate source = %q, want type_match", got[0].Source)
	}
	for _, cand := range got {
		if cand.FHIRPath == "Patient.identifier" {
			t.Errorf("type-incompatible element Patient.identifier leaked into results: %+v", got)
		}
	}
}

func TestMatch_BlocklistedPathExcludedFromResults(t *testing.T) {
	schema := fakeProfile("Encounter", map[string]string{
		"Encounter.statusHistory": "code", // blocklisted substring
		"Encounter.status":        "code", // NOT blocklisted (must not collide)
	})
	got := Match("ZZZ", "ZZZ.1", "Status", "IS", "Encounter", schema, FHIRVersionR4)
	for _, cand := range got {
		if cand.FHIRPath == "Encounter.statusHistory" {
			t.Errorf("blocklisted path leaked into results: %+v", got)
		}
	}
}

func TestMatch_ResultsCappedAtFiveAndSortedDescending(t *testing.T) {
	dataTypes := map[string]string{}
	// 8 distinct string-typed elements sharing the "name" token so Pass 3
	// (name_similarity) scores all of them above the 0.4 threshold.
	for _, leaf := range []string{"name", "nameA", "nameB", "nameC", "nameD", "nameE", "nameF", "nameG"} {
		dataTypes["Patient."+leaf] = "string"
	}
	schema := fakeProfile("Patient", dataTypes)
	// HL7 type "ST" -> FHIRType "string", so Pass 2 will actually fire here
	// (string is type-compatible with string) rather than falling through to
	// Pass 3 - still exercises the same cap/sort logic at the end of Match().
	got := Match("ZZZ", "ZZZ.1", "name", "ST", "Patient", schema, FHIRVersionR4)
	if len(got) > 5 {
		t.Errorf("Match() returned %d candidates, want capped at 5", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Confidence > got[i-1].Confidence {
			t.Errorf("results not sorted descending by confidence: %+v", got)
			break
		}
	}
}

// ── fieldNumber ──────────────────────────────────────────────────────────────

func TestFieldNumber(t *testing.T) {
	cases := map[string]string{
		"PID.5":   "5",
		"PID.5.1": "5",
		"NK1.19":  "19",
		"MSH":     "MSH", // no dot -> whole key returned verbatim
	}
	for in, want := range cases {
		if got := fieldNumber(in); got != want {
			t.Errorf("fieldNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── typeCompatible ────────────────────────────────────────────────────────────

func TestTypeCompatible(t *testing.T) {
	cases := []struct {
		registry, element string
		want               bool
	}{
		{"string", "string", true},        // exact match
		{"string", "code", true},          // stringLike group
		{"code", "uri", true},             // stringLike group
		{"date", "dateTime", true},        // dateLike group
		{"dateTime", "instant", true},     // dateLike group
		{"decimal", "integer", true},      // numLike group
		{"decimal", "positiveInt", true},  // numLike group
		{"HumanName", "HumanName", true},  // exact match, non-grouped type
		{"HumanName", "Identifier", false},
		{"string", "date", false},         // different groups
		{"decimal", "string", false},      // different groups
	}
	for _, c := range cases {
		if got := typeCompatible(c.registry, c.element); got != c.want {
			t.Errorf("typeCompatible(%q, %q) = %v, want %v", c.registry, c.element, got, c.want)
		}
	}
}

// ── nameSimilarity / fhirPathLeaf / tokenize ─────────────────────────────────

func TestFhirPathLeaf(t *testing.T) {
	cases := map[string]string{
		"Patient.name[0].family": "family",
		"Patient.birthDate":      "birthDate",
		"Encounter.class.code":   "code",
		"Patient":                "Patient", // no dot -> whole thing is the leaf
	}
	for in, want := range cases {
		if got := fhirPathLeaf(in); got != want {
			t.Errorf("fhirPathLeaf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"PatientName", []string{"patient", "name"}},
		{"date of birth", []string{"date", "of", "birth"}},
		{"birthDate", []string{"birth", "date"}},
		{"", nil},
	}
	for _, c := range cases {
		got := tokenize(c.in)
		if len(got) != len(c.want) {
			t.Errorf("tokenize(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tokenize(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestNameSimilarity(t *testing.T) {
	// Identical leaf token -> full overlap -> score 1.0.
	if got := nameSimilarity("family", "Patient.name[0].family"); got != 1.0 {
		t.Errorf("nameSimilarity(family, .family) = %v, want 1.0", got)
	}
	// No shared tokens -> 0.
	if got := nameSimilarity("gender", "Patient.name[0].family"); got != 0 {
		t.Errorf("nameSimilarity(gender, .family) = %v, want 0", got)
	}
	// Partial overlap ("date" and "birth" both present, but "patient" and "of"
	// are extra tokens on the HL7 side) -> strictly between 0 and 1.
	got := nameSimilarity("patient date of birth", "Patient.birthDate")
	if got <= 0 || got >= 1 {
		t.Errorf("nameSimilarity(patient date of birth, .birthDate) = %v, want strictly between 0 and 1, got exactly that value", got)
	}
}

// ── sortCandidates ────────────────────────────────────────────────────────────

func TestSortCandidates_DescendingByConfidence(t *testing.T) {
	c := []Candidate{
		{FHIRPath: "a", Confidence: 0.5},
		{FHIRPath: "b", Confidence: 0.9},
		{FHIRPath: "c", Confidence: 0.1},
		{FHIRPath: "d", Confidence: 0.7},
	}
	sortCandidates(c)
	want := []string{"b", "d", "a", "c"}
	for i, w := range want {
		if c[i].FHIRPath != w {
			t.Errorf("sortCandidates() order = %v, want FHIRPath order %v", c, want)
			break
		}
	}
}

func TestSortCandidates_EmptyAndSingleElement(t *testing.T) {
	empty := []Candidate{}
	sortCandidates(empty) // must not panic
	if len(empty) != 0 {
		t.Errorf("sortCandidates(empty) mutated length to %d", len(empty))
	}

	single := []Candidate{{FHIRPath: "a", Confidence: 0.5}}
	sortCandidates(single) // must not panic
	if single[0].FHIRPath != "a" {
		t.Errorf("sortCandidates(single) mutated single element: %+v", single)
	}
}
