// services/executors/cda_path_resolver_test.go
//
// Phase 1 unit tests for the wildcard/predicate path resolver. Two tiers:
//  1. Pure grammar/mechanics tests against hand-built fixtures shaped like
//     each backend's real output (fast, isolated, no XML parsing).
//  2. End-to-end tests against the real fixture files named in the sprint
//     plan's Phase 1 exit criteria (negation_and_frequency.xml,
//     supply_and_nullflavors.xml) — see cda_path_resolver_fixture_test.go.
package executors

import "testing"

// ===============================================================
// PATH PARSING
// ===============================================================

func TestParseCDAPathSegment_Plain(t *testing.T) {
	seg := parseCDAPathSegment("entry")
	if seg.kind != segmentPlain || seg.key != "entry" {
		t.Fatalf("got %+v, want plain key=entry", seg)
	}
}

func TestParseCDAPathSegment_Wildcard(t *testing.T) {
	seg := parseCDAPathSegment("entries[*]")
	if seg.kind != segmentWildcard || seg.key != "entries" {
		t.Fatalf("got %+v, want wildcard key=entries", seg)
	}
}

func TestParseCDAPathSegment_Index(t *testing.T) {
	seg := parseCDAPathSegment("entries[12]")
	if seg.kind != segmentIndex || seg.key != "entries" || seg.index != 12 {
		t.Fatalf("got %+v, want index key=entries index=12", seg)
	}
}

func TestParseCDAPathSegment_SinglePredicate(t *testing.T) {
	seg := parseCDAPathSegment("entryRelationships[typeCode=SUBJ]")
	if seg.kind != segmentPredicate || seg.key != "entryRelationships" {
		t.Fatalf("got %+v, want predicate key=entryRelationships", seg)
	}
	if len(seg.predicates) != 1 || seg.predicates[0] != (cdaPredicate{key: "typeCode", value: "SUBJ"}) {
		t.Fatalf("predicates = %+v, want [{typeCode SUBJ}]", seg.predicates)
	}
}

func TestParseCDAPathSegment_CompoundPredicate(t *testing.T) {
	// Exact notation from CDA_FHIR_MAPPING_INVENTORY.md's cross-cutting
	// finding #1: "entryRelationship[typeCode=SUBJ, inversionInd=true]".
	seg := parseCDAPathSegment("entryRelationship[typeCode=SUBJ, inversionInd=true]")
	if seg.kind != segmentPredicate {
		t.Fatalf("got kind %v, want segmentPredicate", seg.kind)
	}
	want := []cdaPredicate{{key: "typeCode", value: "SUBJ"}, {key: "inversionInd", value: "true"}}
	if len(seg.predicates) != 2 || seg.predicates[0] != want[0] || seg.predicates[1] != want[1] {
		t.Fatalf("predicates = %+v, want %+v", seg.predicates, want)
	}
}

func TestParseCDAPathSegment_UnknownPredicateKeyDropped(t *testing.T) {
	// "bogus" is outside the closed key set -- the whole bracket degrades to
	// a literal (never-matching) plain key, not a parse error.
	seg := parseCDAPathSegment("entries[bogus=1]")
	if seg.kind != segmentPlain || seg.key != "entries[bogus=1]" {
		t.Fatalf("got %+v, want plain literal key=\"entries[bogus=1]\"", seg)
	}
}

func TestParseCDAPathSegment_UnclosedBracketIsLiteral(t *testing.T) {
	seg := parseCDAPathSegment("entries[0")
	if seg.kind != segmentPlain || seg.key != "entries[0" {
		t.Fatalf("got %+v, want plain literal (unclosed bracket never matches)", seg)
	}
}

func TestParseCDAPath_TemplateIDOIDInsideBracketNotSplitOnDots(t *testing.T) {
	// Regression: a naive strings.Split(path, ".") shreds a templateId OID
	// (always dot-separated, e.g. "2.16.840.1.113883...") into bogus
	// segments wherever it appears inside a "[...]" bracket. This is the
	// single most common real path shape Phase 2 will generate (every
	// templateId-conditional dispatch case in the inventory), so it gets its
	// own explicit test, not just incidental coverage via the discriminator
	// tests below.
	path := "entry[templateId=2.16.840.1.113883.10.20.22.4.147].text"
	segs := parseCDAPath(path)
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2 (the bracket's internal dots must not split it): %+v", len(segs), segs)
	}
	if segs[0].kind != segmentPredicate || segs[0].key != "entry" {
		t.Fatalf("segment 0 = %+v, want predicate key=entry", segs[0])
	}
	if len(segs[0].predicates) != 1 || segs[0].predicates[0].value != "2.16.840.1.113883.10.20.22.4.147" {
		t.Fatalf("predicate value = %+v, want the full unmangled OID", segs[0].predicates)
	}
	if segs[1].kind != segmentPlain || segs[1].key != "text" {
		t.Fatalf("segment 1 = %+v, want plain key=text", segs[1])
	}
}

func TestParseCDAPath_MultiSegment(t *testing.T) {
	segs := parseCDAPath("entries[0].entryRelationships[typeCode=SUBJ].entry.negationInd")
	if len(segs) != 4 {
		t.Fatalf("got %d segments, want 4: %+v", len(segs), segs)
	}
	if segs[0].kind != segmentIndex || segs[1].kind != segmentPredicate ||
		segs[2].kind != segmentPlain || segs[3].kind != segmentPlain {
		t.Fatalf("segment kinds = %+v", segs)
	}
}

// ===============================================================
// NODE-SET RESOLUTION -- typed "document.*" tree backend (isXMLMirror=false)
// ===============================================================

// documentTreeFixture mirrors the JSON shape negation_and_frequency.xml
// produces once round-tripped through encoding/json (see
// cda_path_resolver_fixture_test.go for the real parsed version) -- typed
// fields use the JSON tags from cda/document/types.go: "typeCode",
// "inversionInd", "classCode", "moodCode", "templateIds" (plural, flat
// string array), "entryRelationships" (plural).
func documentTreeFixture() map[string]interface{} {
	return map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{
				"entryType":   "act",
				"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.30"},
				"entryRelationships": []interface{}{
					map[string]interface{}{
						"typeCode":     "SUBJ",
						"inversionInd": false,
						"entry": map[string]interface{}{
							"entryType":   "observation",
							"negationInd": true,
							"templateIds": []interface{}{
								"2.16.840.1.113883.10.20.22.4.7",
								"2.16.840.1.113883.10.20.24.3.90",
							},
						},
					},
				},
			},
		},
		// A second entry whose entryRelationships discriminate Free Text Sig
		// (COMP, templateId .147) from Instruction(V2) (SUBJ+inversionInd,
		// templateId .20) -- the byte-identical-shape worked example from
		// cross-cutting finding #1, expressed as the chained-predicate path
		// the design note specifies.
		"sigEntry": map[string]interface{}{
			"entryRelationships": []interface{}{
				map[string]interface{}{
					"typeCode": "COMP",
					"entry": map[string]interface{}{
						"text":        "Take with food",
						"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.147"},
					},
				},
				map[string]interface{}{
					"typeCode":     "SUBJ",
					"inversionInd": true,
					"entry": map[string]interface{}{
						"text":        "Avoid alcohol",
						"templateIds": []interface{}{"2.16.840.1.113883.10.20.22.4.20"},
					},
				},
			},
		},
	}
}

func TestResolveCDAPath_DocumentTree_PlainAndIndex(t *testing.T) {
	root := documentTreeFixture()
	got := ResolveCDAPath(root, "entries[0].entryType", false)
	if got != "act" {
		t.Fatalf("got %v, want \"act\"", got)
	}
}

func TestResolveCDAPath_DocumentTree_PredicateThenNestedField(t *testing.T) {
	root := documentTreeFixture()
	path := "entries[0].entryRelationships[typeCode=SUBJ].entry.negationInd"
	got := ResolveCDAPath(root, path, false)
	negated, ok := got.(bool)
	if !ok || !negated {
		t.Fatalf("GetFieldValue-equivalent resolution = %v, want true", got)
	}
}

func TestResolveCDAPath_DocumentTree_TemplateIDMembership(t *testing.T) {
	root := documentTreeFixture()
	path := "entries[0].entryRelationships[typeCode=SUBJ].entry[templateId=2.16.840.1.113883.10.20.24.3.90]"
	got := ResolveCDAPath(root, path, false)
	if got == nil {
		t.Fatal("expected the SUBJ-nested entry to match its second templateId, got nil")
	}
}

func TestResolveCDAPath_DocumentTree_TemplateIDMismatch_ReturnsNil(t *testing.T) {
	root := documentTreeFixture()
	path := "entries[0].entryRelationships[typeCode=SUBJ].entry[templateId=2.16.840.1.113883.10.20.22.4.999]"
	got := ResolveCDAPath(root, path, false)
	if got != nil {
		t.Fatalf("got %v, want nil (no entry carries this templateId)", got)
	}
}

func TestResolveCDAPath_DocumentTree_CompoundPredicate(t *testing.T) {
	root := documentTreeFixture()
	path := "entries[0].entryRelationships[typeCode=SUBJ,inversionInd=false].entry.negationInd"
	got := ResolveCDAPath(root, path, false)
	if negated, ok := got.(bool); !ok || !negated {
		t.Fatalf("compound predicate (typeCode AND inversionInd) failed to match: got %v", got)
	}
}

func TestResolveCDAPath_DocumentTree_CompoundPredicateWrongValue_NoMatch(t *testing.T) {
	root := documentTreeFixture()
	// inversionInd is false on the fixture's SUBJ relationship -- asking for
	// true must not match (this is exactly what discriminates Instruction(V2)
	// from a plain SUBJ reference elsewhere).
	path := "entries[0].entryRelationships[typeCode=SUBJ,inversionInd=true].entry.negationInd"
	got := ResolveCDAPath(root, path, false)
	if got != nil {
		t.Fatalf("got %v, want nil (inversionInd=true should not match a false field)", got)
	}
}

// TestResolveCDAPath_DocumentTree_FreeTextSigVsInstructionV2Discriminator is
// the star worked example from the inventory's cross-cutting finding #1:
// two entryRelationships with the identical {text, templateIds} shape,
// distinguished only by (typeCode[,inversionInd]) at the wrapper level and
// templateId one level in on the nested entry -- exactly
// medication_mapper.go's applyDosageText, expressed as two chained-predicate
// paths instead of Go.
func TestResolveCDAPath_DocumentTree_FreeTextSigVsInstructionV2Discriminator(t *testing.T) {
	root := documentTreeFixture()

	freeTextSigPath := "sigEntry.entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.147].text"
	got := ResolveCDAPath(root, freeTextSigPath, false)
	if got != "Take with food" {
		t.Fatalf("Free Text Sig path resolved to %v, want %q", got, "Take with food")
	}

	instructionV2Path := "sigEntry.entryRelationships[typeCode=SUBJ,inversionInd=true].entry[templateId=2.16.840.1.113883.10.20.22.4.20].text"
	got = ResolveCDAPath(root, instructionV2Path, false)
	if got != "Avoid alcohol" {
		t.Fatalf("Instruction(V2) path resolved to %v, want %q", got, "Avoid alcohol")
	}

	// Crucially: swapping the templateId between the two paths must NOT
	// cross-match, proving templateId (not just typeCode) is load-bearing.
	crossedPath := "sigEntry.entryRelationships[typeCode=COMP].entry[templateId=2.16.840.1.113883.10.20.22.4.20].text"
	if got := ResolveCDAPath(root, crossedPath, false); got != nil {
		t.Fatalf("got %v, want nil -- the COMP relationship's nested entry does not carry the Instruction(V2) templateId", got)
	}
}

// TestResolveCDAPath_DocumentTree_CodePredicate_SeverityDiscriminator is the
// Phase 3 worked example that justified adding "code" to cdaPredicateKeys:
// allergy_mapper.go/condition_mapper.go both find a nested severity
// observation among a problem/reaction's SUBJ relationships by requiring the
// nested entry's own code.code == "SEV" (Severity Obs V2, CONF:1098-19169) --
// a fixed-value act code, not a RIM attribute, so it needs the same nested-
// element handling templateId already gets.
func TestResolveCDAPath_DocumentTree_CodePredicate_SeverityDiscriminator(t *testing.T) {
	root := map[string]interface{}{
		"entryRelationships": []interface{}{
			map[string]interface{}{
				"typeCode": "SUBJ",
				"entry": map[string]interface{}{
					"code":  map[string]interface{}{"code": "SEV"},
					"value": map[string]interface{}{"code": map[string]interface{}{"code": "24484000"}},
				},
			},
		},
	}
	path := "entryRelationships[typeCode=SUBJ].entry[code=SEV].value.code.code"
	got := ResolveCDAPath(root, path, false)
	if got != "24484000" {
		t.Fatalf("got %v, want \"24484000\" (the SEV-coded nested observation's value)", got)
	}
}

// TestResolveCDAPath_DocumentTree_ChainedPredicate_NarrowsAmongCollidingSiblings
// proves the guarantee CDAStepBuilder.js's Add Field "Browse Test Data" leaf
// disambiguation depends on: when TWO entryRelationships share the SAME
// typeCode (a real, common shape — e.g. two Reaction Observations each linked
// via typeCode=MFST, one per manifestation), a chained
// "entryRelationships[typeCode=X].entry[code=Y]" path resolves the WHOLE
// multi-segment chain together, correctly narrowing to whichever sibling's
// nested entry actually has code=Y — NOT just the first typeCode=X match. If
// this ever regressed to "collapse to the first bracket match before
// evaluating the next", the frontend's disambiguation would silently start
// resolving to the wrong sibling instead of a genuinely narrowed one.
func TestResolveCDAPath_DocumentTree_ChainedPredicate_NarrowsAmongCollidingSiblings(t *testing.T) {
	root := map[string]interface{}{
		"entryRelationships": []interface{}{
			map[string]interface{}{
				"typeCode": "MFST",
				"entry": map[string]interface{}{
					"code":  map[string]interface{}{"code": "39579001"},
					"value": map[string]interface{}{"code": map[string]interface{}{"code": "111"}},
				},
			},
			map[string]interface{}{
				"typeCode": "MFST",
				"entry": map[string]interface{}{
					"code":  map[string]interface{}{"code": "247472004"},
					"value": map[string]interface{}{"code": map[string]interface{}{"code": "222"}},
				},
			},
		},
	}

	got := ResolveCDAPath(root, "entryRelationships[typeCode=MFST].entry[code=247472004].value.code.code", false)
	if got != "222" {
		t.Fatalf("got %v, want \"222\" (the SECOND MFST sibling's value, selected by its own nested code) — got the wrong sibling or collapsed to the first typeCode match before applying the code predicate", got)
	}

	// Same chain, targeting the FIRST sibling's code, to confirm this isn't
	// coincidentally always resolving to whichever happens to be last/first.
	got = ResolveCDAPath(root, "entryRelationships[typeCode=MFST].entry[code=39579001].value.code.code", false)
	if got != "111" {
		t.Fatalf("got %v, want \"111\" (the FIRST MFST sibling's value)", got)
	}
}

func TestResolveCDAPath_DocumentTree_CodePredicate_WrongCodeNoMatch(t *testing.T) {
	root := map[string]interface{}{
		"entry": map[string]interface{}{
			"code": map[string]interface{}{"code": "NOT-SEV"},
		},
	}
	if got := ResolveCDAPath(root, "entry[code=SEV]", false); got != nil {
		t.Fatalf("got %v, want nil -- code=NOT-SEV must not match a code=SEV predicate", got)
	}
}

func TestResolveCDAPath_DocumentTree_CodePredicate_NonObjectCodeFailsClosed(t *testing.T) {
	// A node whose "code" isn't a nested object (e.g. a bare string, or
	// absent entirely) must not match -- and must not panic.
	root := map[string]interface{}{
		"entry": map[string]interface{}{"code": "bare-string-not-an-object"},
	}
	if got := ResolveCDAPath(root, "entry[code=SEV]", false); got != nil {
		t.Fatalf("got %v, want nil for a non-object code field", got)
	}
}

// TestResolveCDAPath_DocumentTree_XSITypePredicate_PIVLFrequencyDiscriminator
// is the second Phase 3 addition: medication_mapper.go's applyDosageTiming
// needs to find the PIVL_TS/EIVL_TS effectiveTime entry (the dosing-frequency
// one), not the primary IVL_TS duration entry, among CDAEntry.EffectiveTimes.
func TestResolveCDAPath_DocumentTree_XSITypePredicate_PIVLFrequencyDiscriminator(t *testing.T) {
	root := map[string]interface{}{
		"effectiveTimes": []interface{}{
			map[string]interface{}{"xsiType": "IVL_TS", "range": map[string]interface{}{}},
			map[string]interface{}{"xsiType": "PIVL_TS", "period": map[string]interface{}{"value": "12", "unit": "h"}},
		},
	}
	path := "effectiveTimes[xsiType=PIVL_TS].period.value"
	got := ResolveCDAPath(root, path, false)
	if got != "12" {
		t.Fatalf("got %v, want \"12\" (the PIVL_TS entry's period.value, not the IVL_TS one)", got)
	}
}

func TestResolveCDAPath_DocumentTree_TypePredicate_ObservationValuePolymorphicDispatch(t *testing.T) {
	// CDAValue.Type's JSON tag is "type" -- a DIFFERENT field from
	// CDAEffectiveTimeEntry.XSIType's "xsiType" tag above, even though both
	// represent an xsi:type discriminator on the CDA side. Vital
	// Signs/Results/Social History's Observation.value[x] polymorphic
	// dispatch (observation_mapper.go's setObservationValue) needs this one.
	root := map[string]interface{}{
		"value": map[string]interface{}{"type": "PQ", "quantity": map[string]interface{}{"value": "71.0", "unit": "[in_us]"}},
	}
	got := ResolveCDAPath(root, "value[type=PQ].quantity.value", false)
	if got != "71.0" {
		t.Fatalf("got %v, want \"71.0\" (the PQ value's quantity)", got)
	}
	if got := ResolveCDAPath(root, "value[type=CD].code", false); got != nil {
		t.Fatalf("got %v, want nil -- value.type is PQ, not CD", got)
	}
}

func TestResolveCDAPath_DocumentTree_ORListPredicate_MatchesEitherValue(t *testing.T) {
	// procedure_mapper.go:54 checks `p.TypeCode == "PRF" || p.TypeCode ==
	// "SPRF"` (Performer / Secondary Performer) when finding a Procedure's
	// performer -- a real Go && a single-value predicate can't express.
	root := map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"typeCode": "REF", "name": "referrer"},
			map[string]interface{}{"typeCode": "SPRF", "name": "secondary performer"},
		},
	}
	got := ResolveCDAPaths(root, "participants[typeCode=PRF|SPRF].name", false)
	if len(got) != 1 || got[0] != "secondary performer" {
		t.Fatalf("got %v, want [\"secondary performer\"] (REF must not match a PRF|SPRF predicate)", got)
	}
}

func TestResolveCDAPath_DocumentTree_ORListPredicate_NoMatchAmongEitherValue_ReturnsNil(t *testing.T) {
	root := map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{"typeCode": "REF", "name": "referrer"},
		},
	}
	got := ResolveCDAPaths(root, "participants[typeCode=PRF|SPRF].name", false)
	if got != nil {
		t.Fatalf("got %v, want nil -- REF matches neither PRF nor SPRF", got)
	}
}

func TestResolveCDAPath_DocumentTree_EntryTypePredicate_PlanOfCareDispatch(t *testing.T) {
	// plan_of_care_mapper.go's dispatchPlanOfCareEntry switches on
	// entry.EntryType after moodCode -- the EntryMatch mechanism
	// (declarative_engine.go's entryMatchesPredicate) reuses this exact
	// bracket grammar against a single already-resolved entry node.
	root := map[string]interface{}{
		"entries": []interface{}{
			map[string]interface{}{"entryType": "encounter", "name": "planned-appt"},
			map[string]interface{}{"entryType": "supply", "name": "planned-supply"},
		},
	}
	got := ResolveCDAPaths(root, "entries[entryType=encounter].name", false)
	if len(got) != 1 || got[0] != "planned-appt" {
		t.Fatalf("got %v, want [\"planned-appt\"]", got)
	}
	if got := ResolveCDAPaths(root, "entries[entryType=procedure].name", false); got != nil {
		t.Fatalf("got %v, want nil -- neither entry is entryType=procedure", got)
	}
}

func TestResolveCDAPaths_DocumentTree_WildcardCollectsAll(t *testing.T) {
	root := map[string]interface{}{
		"effectiveTimes": []interface{}{
			map[string]interface{}{"xsiType": "IVL_TS"},
			map[string]interface{}{"xsiType": "PIVL_TS"},
		},
	}
	got := ResolveCDAPaths(root, "effectiveTimes[*].xsiType", false)
	if len(got) != 2 || got[0] != "IVL_TS" || got[1] != "PIVL_TS" {
		t.Fatalf("got %v, want [IVL_TS PIVL_TS]", got)
	}
}

func TestResolveCDAPath_DocumentTree_OutOfRangeIndex_ReturnsNil(t *testing.T) {
	root := documentTreeFixture()
	got := ResolveCDAPath(root, "entries[99].entryType", false)
	if got != nil {
		t.Fatalf("got %v, want nil for an out-of-range index", got)
	}
}

func TestResolveCDAPath_MalformedBracket_FailsClosedNotPanic(t *testing.T) {
	root := documentTreeFixture()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("resolver panicked on malformed path: %v", r)
		}
	}()
	if got := ResolveCDAPath(root, "entries[0", false); got != nil {
		t.Fatalf("got %v, want nil for an unclosed bracket", got)
	}
	if got := ResolveCDAPath(root, "entries[bogus=1].entryType", false); got != nil {
		t.Fatalf("got %v, want nil for an unknown predicate key", got)
	}
}

// ===============================================================
// NODE-SET RESOLUTION -- generic "xml.*" mirror backend (isXMLMirror=true)
// ===============================================================

// xmlMirrorFixture mirrors GenericXMLToJSON's actual output shape: XML
// attributes prefixed with "@", a singular XML tag name as the key (not the
// typed tree's pluralised JSON tag), and templateId expressed as one-or-more
// child elements carrying "@root" rather than a flat string array.
func xmlMirrorFixture() map[string]interface{} {
	return map[string]interface{}{
		"act": map[string]interface{}{
			"@classCode": "ACT",
			"@moodCode":  "EVN",
			"templateId": map[string]interface{}{"@root": "2.16.840.1.113883.10.20.22.4.30"},
			"entryRelationship": map[string]interface{}{
				"@typeCode":     "SUBJ",
				"@inversionInd": "false",
				"observation": map[string]interface{}{
					"@classCode":   "OBS",
					"@moodCode":    "EVN",
					"@negationInd": "true",
					"templateId": []interface{}{
						map[string]interface{}{"@root": "2.16.840.1.113883.10.20.22.4.7"},
						map[string]interface{}{"@root": "2.16.840.1.113883.10.20.24.3.90"},
					},
				},
			},
		},
	}
}

func TestResolveCDAPath_XMLMirror_AttributeRead(t *testing.T) {
	root := xmlMirrorFixture()
	got := ResolveCDAPath(root, "act.@classCode", true)
	if got != "ACT" {
		t.Fatalf("got %v, want \"ACT\"", got)
	}
}

func TestResolveCDAPath_XMLMirror_PredicateOnSingleOccurrence(t *testing.T) {
	// act.entryRelationship is a single occurrence -- GenericXMLToJSON
	// collapses it to a bare object, not a 1-element array. The predicate
	// bracket must still match via asCDANodeList's implicit wrap.
	root := xmlMirrorFixture()
	got := ResolveCDAPath(root, "act.entryRelationship[typeCode=SUBJ].observation.@negationInd", true)
	if got != "true" {
		t.Fatalf("got %v, want the string \"true\" -- XML attributes are strings, unlike the typed tree's real bool", got)
	}
}

func TestResolveCDAPath_XMLMirror_TemplateIDChildElementMatch(t *testing.T) {
	root := xmlMirrorFixture()
	path := "act.entryRelationship[typeCode=SUBJ].observation[templateId=2.16.840.1.113883.10.20.24.3.90].@negationInd"
	got := ResolveCDAPath(root, path, true)
	if got != "true" {
		t.Fatalf("got %v, want \"true\" -- observation carries this templateId as a child element, not a flat array", got)
	}
}

func TestResolveCDAPath_XMLMirror_TemplateIDOnSingleOccurrence(t *testing.T) {
	// act.templateId is a single occurrence (one <templateId> child) --
	// collapsed to a bare object. The templateId predicate must still match
	// it via the same asCDANodeList wrap used for typeCode/classCode/etc.
	root := xmlMirrorFixture()
	got := ResolveCDAPath(root, "act[templateId=2.16.840.1.113883.10.20.22.4.30].@classCode", true)
	if got != "ACT" {
		t.Fatalf("got %v, want \"ACT\"", got)
	}
}

func TestResolveCDAPaths_XMLMirror_WildcardOverChildElementArray(t *testing.T) {
	root := xmlMirrorFixture()
	got := ResolveCDAPaths(root, "act.entryRelationship.observation.templateId[*].@root", true)
	want := []interface{}{
		"2.16.840.1.113883.10.20.22.4.7",
		"2.16.840.1.113883.10.20.24.3.90",
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// ===============================================================
// FALLBACK CHAIN
// ===============================================================

func TestResolveCDAFallback_FirstNonEmptyWins(t *testing.T) {
	root := map[string]interface{}{
		"a": map[string]interface{}{}, // "value" key absent
		"b": map[string]interface{}{"value": "fallback-value"},
	}
	got := ResolveCDAFallback(root, false, "a.value", "b.value")
	if got != "fallback-value" {
		t.Fatalf("got %v, want %q", got, "fallback-value")
	}
}

func TestResolveCDAFallback_PrimaryWinsWhenPresent(t *testing.T) {
	root := map[string]interface{}{
		"a": map[string]interface{}{"value": "primary"},
		"b": map[string]interface{}{"value": "fallback"},
	}
	got := ResolveCDAFallback(root, false, "a.value", "b.value")
	if got != "primary" {
		t.Fatalf("got %v, want %q (primary should win when present)", got, "primary")
	}
}

func TestResolveCDAFallback_EmptyStringSkipped(t *testing.T) {
	root := map[string]interface{}{
		"a": map[string]interface{}{"value": ""},
		"b": map[string]interface{}{"value": "fallback"},
	}
	got := ResolveCDAFallback(root, false, "a.value", "b.value")
	if got != "fallback" {
		t.Fatalf("got %v, want %q -- an empty string must be treated as absent", got, "fallback")
	}
}

func TestResolveCDAFallback_AllMissing_ReturnsNil(t *testing.T) {
	root := map[string]interface{}{}
	got := ResolveCDAFallback(root, false, "a.value", "b.value")
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// ===============================================================
// asCDANodeList NORMALISATION
// ===============================================================

func TestAsCDANodeList_RealArrayPassedThrough(t *testing.T) {
	in := []interface{}{"x", "y"}
	got := asCDANodeList(in)
	if len(got) != 2 {
		t.Fatalf("got %v, want the same 2-element slice", got)
	}
}

func TestAsCDANodeList_BareObjectWrappedAsSingleton(t *testing.T) {
	in := map[string]interface{}{"@root": "X"}
	got := asCDANodeList(in)
	if len(got) != 1 || got[0].(map[string]interface{})["@root"] != "X" {
		t.Fatalf("got %v, want a 1-element list wrapping the bare object", got)
	}
}

func TestAsCDANodeList_NilStaysNil(t *testing.T) {
	got := asCDANodeList(nil)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

// ===============================================================
// FIELD_UTILS.GO DISPATCH INTEGRATION -- proves the new "xml." path type and
// the document.*/xml.* wiring through GetFieldValue/UpdateFieldValue, not
// just the internal resolver functions.
// ===============================================================

func TestIsCDAXMLMirrorPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"xml.component.structuredBody", true},
		{"document.sectionsByKey.medications", false},
		{"allergiesAndIntolerances.medicationAllergyCode", false},
	}
	for _, tc := range cases {
		if got := IsCDAXMLMirrorPath(tc.path); got != tc.want {
			t.Errorf("IsCDAXMLMirrorPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestDetectPathType_CDAXMLMirror(t *testing.T) {
	if got := DetectPathType("xml.component.structuredBody"); got != PathTypeCDAXMLMirror {
		t.Errorf("DetectPathType = %v, want %v", got, PathTypeCDAXMLMirror)
	}
}

func TestGetFieldValue_XMLMirrorPath_ResolvesPredicate(t *testing.T) {
	data := map[string]interface{}{"xml": xmlMirrorFixture()}
	path := "xml.act.entryRelationship[typeCode=SUBJ].observation.@negationInd"
	got := GetFieldValue(data, path)
	if got != "true" {
		t.Fatalf("GetFieldValue(%q) = %v, want \"true\"", path, got)
	}
}

func TestGetFieldValue_DocumentPath_ResolvesWildcard(t *testing.T) {
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"effectiveTimes": []interface{}{
				map[string]interface{}{"xsiType": "IVL_TS"},
				map[string]interface{}{"xsiType": "PIVL_TS"},
			},
		},
	}
	got := GetFieldValue(data, "document.effectiveTimes[*].xsiType")
	if got != "IVL_TS" {
		t.Fatalf("GetFieldValue(wildcard) = %v, want first match \"IVL_TS\" (single-value contract)", got)
	}
}

func TestGetFieldValue_XMLMirrorPath_MissingXMLKey_ReturnsNilNotPanic(t *testing.T) {
	data := map[string]interface{}{"document": map[string]interface{}{}}
	got := GetFieldValue(data, "xml.component.structuredBody")
	if got != nil {
		t.Errorf("expected nil when \"xml\" key is absent, got %v", got)
	}
}

func TestUpdateFieldValue_XMLMirrorPath_AlwaysRejected(t *testing.T) {
	data := map[string]interface{}{"xml": xmlMirrorFixture()}
	if ok := UpdateFieldValue(data, "xml.act.@classCode", "CHANGED"); ok {
		t.Fatal("UpdateFieldValue on an xml.* path should always return false -- the mirror is read-only")
	}
	// Confirm nothing was mutated despite the rejected write.
	if got := GetFieldValue(data, "xml.act.@classCode"); got != "ACT" {
		t.Fatalf("xml mirror was mutated despite UpdateFieldValue returning false: got %v", got)
	}
}
