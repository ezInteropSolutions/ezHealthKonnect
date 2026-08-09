// services/executors/cda_element_translation_test.go
//
// Table-driven proof for cda_element_translation.go's walker + the
// cda_element_translation_table.go rules it consults -- exercised directly
// against hand-built typed-tree fixtures (map[string]interface{}, matching
// what json.Marshal(cdadocument.CDAEntry{...}) then Unmarshal into a generic
// map would produce), no engine/document-mapper involved. Real path values
// throughout are pulled from services/cda_fhir/declarative_oob_rules.go's
// actual MappingRow.Scope/SourcePath usage, confirmed via grep, not invented.
package executors

import "testing"

func TestResolveCDAPathsTranslated_SimpleRename(t *testing.T) {
	// Real rule: SourcePath: "authors[0].assignedAuthor.assignedPerson.names[0]"
	entry := map[string]interface{}{
		"authors": []interface{}{
			map[string]interface{}{
				"assignedAuthor": map[string]interface{}{
					"assignedPerson": map[string]interface{}{
						"names": []interface{}{
							map[string]interface{}{"family": "Smith"},
						},
					},
				},
			},
		},
	}
	got := ResolveCDAPathTranslated(entry, "authors[0].assignedAuthor.assignedPerson.names[0]", "CDAEntry")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	// Every segment gets an explicit index, including single-occurrence
	// ones (assignedAuthor[0], not bare assignedAuthor) -- required so this
	// matches what services/cda_coverage's inventory walker independently
	// produces from the raw XML mirror alone, with no typed-schema
	// knowledge of which fields are "genuinely singular."
	want := "author[0].assignedAuthor[0].assignedPerson[0].name[0]"
	if got.XMLPath != want {
		t.Errorf("XMLPath = %q, want %q", got.XMLPath, want)
	}
}

func TestResolveCDAPathsTranslated_ActChild_DataDependent(t *testing.T) {
	// Real rule shape: Scope: "entryRelationships[typeCode=SUBJ].entry",
	// SourcePath: "value.code.code" -- proves the entryRelationship wrapper
	// consumes no extra "entry" XML level, and the act tag is read from the
	// resolved node's own entryType, not guessed from the path.
	makeEntry := func(actType string) map[string]interface{} {
		return map[string]interface{}{
			"entryRelationships": []interface{}{
				map[string]interface{}{
					"typeCode": "SUBJ",
					"entry": map[string]interface{}{
						"entryType": actType,
						"value": map[string]interface{}{
							"code": map[string]interface{}{"code": "271649006", "codeSystem": "2.16.840.1.113883.6.96"},
						},
					},
				},
			},
		}
	}

	t.Run("observation", func(t *testing.T) {
		got := ResolveCDAPathTranslated(makeEntry("observation"), "entryRelationships[typeCode=SUBJ].entry.value.code.code", "CDAEntry")
		if !got.Translatable {
			t.Fatalf("Translatable = false, want true")
		}
		want := "entryRelationship[0].observation[0].value[0]"
		if got.XMLPath != want {
			t.Errorf("XMLPath = %q, want %q", got.XMLPath, want)
		}
	})

	t.Run("substanceAdministration -- proves data-dependence, not a hardcoded tag", func(t *testing.T) {
		got := ResolveCDAPathTranslated(makeEntry("substanceAdministration"), "entryRelationships[typeCode=SUBJ].entry.value.code.code", "CDAEntry")
		if !got.Translatable {
			t.Fatalf("Translatable = false, want true")
		}
		want := "entryRelationship[0].substanceAdministration[0].value[0]"
		if got.XMLPath != want {
			t.Errorf("XMLPath = %q, want %q", got.XMLPath, want)
		}
	})

	t.Run("unknown entryType -- fails closed, never guesses", func(t *testing.T) {
		got := ResolveCDAPathTranslated(makeEntry("bogus"), "entryRelationships[typeCode=SUBJ].entry.value", "CDAEntry")
		if got.Translatable {
			t.Errorf("Translatable = true for unknown entryType %q, want false", "bogus")
		}
	})
}

func TestResolveCDAPathsTranslated_RenamedActChild_Components(t *testing.T) {
	// Unlike entryRelationships[...].entry, CDAEntry.Components crosses a
	// REAL "<component>" wrapper before the data-dependent act tag.
	entry := map[string]interface{}{
		"components": []interface{}{
			map[string]interface{}{
				"entryType":  "observation",
				"statusCode": "completed",
			},
		},
	}
	got := ResolveCDAPathTranslated(entry, "components[0].statusCode", "CDAEntry")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	want := "component[0].observation[0].statusCode[0]"
	if got.XMLPath != want {
		t.Errorf("XMLPath = %q, want %q", got.XMLPath, want)
	}
}

func TestResolveCDAPathsTranslated_IrregularRenames(t *testing.T) {
	// CDAAddress: none of these are simple depluralization -- a generic
	// heuristic would get every one of them wrong. CDAEntry itself has no
	// Addresses field (only nested participant/performer/author roles do),
	// so this fixture nests through CDAParticipantRole -- a real shape, e.g.
	// EncounterMappingRules' LOC participant.
	entry := map[string]interface{}{
		"participants": []interface{}{
			map[string]interface{}{
				"participantRole": map[string]interface{}{
					"addresses": []interface{}{
						map[string]interface{}{
							"city":        "Springfield",
							"streetLines": []interface{}{"123 Main St"},
							"validTime":   map[string]interface{}{"low": map[string]interface{}{"value": "20200101"}},
						},
					},
				},
			},
		},
	}
	const prefix = "participants[0].participantRole."
	const wantPrefix = "participant[0].participantRole[0]."

	cases := []struct {
		name, path, want string
	}{
		{"addresses -> addr", prefix + "addresses[0].city", wantPrefix + "addr[0].city[0]"},
		// streetLines is a bare (unbracketed) plain segment over a typed
		// []string field -- encoding/json always serialises a Go slice as a
		// JSON array regardless of length (unlike GenericXMLToJSON's
		// single-occurrence-collapses-to-bare-object mirror convention), so
		// this hits the genuinely-ambiguous "-1: no index" case (see
		// ResolveCDAPathsTranslated's segmentPlain comment) -- real rules
		// always bracket a field they need a specific element of, so this
		// under-matches safely rather than guessing an index.
		{"streetLines -> streetAddressLine", prefix + "addresses[0].streetLines", wantPrefix + "addr[0].streetAddressLine"},
		{"validTime -> useablePeriod", prefix + "addresses[0].validTime.low.value", wantPrefix + "addr[0].useablePeriod[0].low[0]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveCDAPathTranslated(entry, c.path, "CDAEntry")
			if !got.Translatable {
				t.Fatalf("Translatable = false, want true")
			}
			if got.XMLPath != c.want {
				t.Errorf("XMLPath = %q, want %q", got.XMLPath, c.want)
			}
		})
	}
}

func TestResolveCDAPathsTranslated_CDAValue_AllShapesShareOneElement(t *testing.T) {
	// Real rule shapes: SourcePath "value[type=PQ].quantity", "value[type=CD]
	// .code.code", "value[type=ST].text" (observationValueXDispatchRows-style
	// dispatch) -- every xsi:type variant must resolve to the identical XML
	// path, since they all read the SAME <value> element.
	makeEntry := func(valueType string) map[string]interface{} {
		return map[string]interface{}{
			"value": map[string]interface{}{
				"type":     valueType,
				"code":     map[string]interface{}{"code": "X"},
				"quantity": map[string]interface{}{"value": "120", "unit": "mm[Hg]"},
				"text":     "some text",
			},
		}
	}

	cases := []struct{ valueType, path string }{
		{"PQ", "value[type=PQ].quantity"},
		{"CD", "value[type=CD].code.code"},
		{"ST", "value[type=ST].text"},
	}
	const want = "value[0]"
	for _, c := range cases {
		t.Run(c.valueType, func(t *testing.T) {
			got := ResolveCDAPathTranslated(makeEntry(c.valueType), c.path, "CDAEntry")
			if !got.Translatable {
				t.Fatalf("Translatable = false, want true")
			}
			if got.XMLPath != want {
				t.Errorf("XMLPath = %q, want %q (all value[x] shapes must collapse to the same element)", got.XMLPath, want)
			}
		})
	}
}

func TestResolveCDAPathsTranslated_TimeRange_NestedVsFlattened(t *testing.T) {
	// CDATimeRange.Low/.High are REAL nested elements; CDATimeRange.Value is
	// inline on the SAME node CDATimeRange itself was reached on. Both must
	// be modeled correctly, not conflated.
	entry := map[string]interface{}{
		"effectiveTime": map[string]interface{}{
			"low":   map[string]interface{}{"value": "20240101"},
			"high":  map[string]interface{}{"value": "20240102"},
			"value": map[string]interface{}{"value": "20240101"},
		},
	}

	t.Run("low is a real nested element", func(t *testing.T) {
		got := ResolveCDAPathTranslated(entry, "effectiveTime.low.value", "CDAEntry")
		if !got.Translatable {
			t.Fatalf("Translatable = false, want true")
		}
		want := "effectiveTime[0].low[0]"
		if got.XMLPath != want {
			t.Errorf("XMLPath = %q, want %q", got.XMLPath, want)
		}
	})

	t.Run("value flattens onto the effectiveTime element itself", func(t *testing.T) {
		got := ResolveCDAPathTranslated(entry, "effectiveTime.value.value", "CDAEntry")
		if !got.Translatable {
			t.Fatalf("Translatable = false, want true")
		}
		want := "effectiveTime[0]"
		if got.XMLPath != want {
			t.Errorf("XMLPath = %q, want %q (must NOT grow past effectiveTime -- .value is inline)", got.XMLPath, want)
		}
	})
}

func TestResolveCDAPathsTranslated_FirstOnly_EffectiveTime(t *testing.T) {
	// CDAEntry.EffectiveTime aliases EffectiveTimes[0] (entry_parser.go) --
	// must always translate to an explicit index 0, not a bare/ambiguous
	// segment, since the raw mirror's "effectiveTime" key legitimately holds
	// an array once a real document has more than one occurrence
	// (substanceAdministration commonly does: duration + dosing frequency).
	entry := map[string]interface{}{
		"effectiveTime": map[string]interface{}{"low": map[string]interface{}{"value": "20240101"}},
	}
	got := ResolveCDAPathTranslated(entry, "effectiveTime.low", "CDAEntry")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	if got.XMLPath != "effectiveTime[0].low[0]" {
		t.Errorf("XMLPath = %q, want explicit index 0", got.XMLPath)
	}
}

func TestResolveCDAPathsTranslated_MultiSegmentCollapse_Precondition(t *testing.T) {
	// Real field: CDAEntry.Precondition collapses "<precondition><criterion>
	// <value>" (three real XML levels) into one typed field.
	entry := map[string]interface{}{
		"precondition": map[string]interface{}{"code": "PRN-CRITERION"},
	}
	got := ResolveCDAPathTranslated(entry, "precondition.code", "CDAEntry")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	if got.XMLPath != "precondition[0].criterion[0].value[0]" {
		t.Errorf("XMLPath = %q, want the full collapsed raw path, indexed at every level", got.XMLPath)
	}
}

func TestResolveCDAPathsTranslated_Unmapped(t *testing.T) {
	cases := []struct {
		name, path string
		entry      map[string]interface{}
	}{
		{
			name: "CDATelecom.Type has no raw counterpart",
			path: "authors[0].assignedAuthor.telecoms[0].type",
			entry: map[string]interface{}{
				"authors": []interface{}{
					map[string]interface{}{"assignedAuthor": map[string]interface{}{
						"telecoms": []interface{}{map[string]interface{}{"value": "tel:555-1234", "type": "phone"}},
					}},
				},
			},
		},
		{
			name: "AdditionalValues -- original sibling index not preserved",
			path: "additionalValues[0].code.code",
			entry: map[string]interface{}{
				"additionalValues": []interface{}{
					map[string]interface{}{"code": map[string]interface{}{"code": "X"}},
				},
			},
		},
		{
			name:  "unknown field entirely",
			path:  "notARealField",
			entry: map[string]interface{}{"notARealField": "x"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveCDAPathTranslated(c.entry, c.path, "CDAEntry")
			if got.Translatable {
				t.Errorf("Translatable = true, want false (XMLPath=%q)", got.XMLPath)
			}
		})
	}
}

// TestResolveCDAPathsTranslated_MultiSegment_RepeatingLastLevel proves the
// cdaXMLSegment generalization added for CDA Coverage Audit's header-field
// extension: a multiSegment rule's real walk index must apply to the LAST
// dotted level (the one that can genuinely repeat, e.g. CDAPatient.Names ->
// "patient.name" -- <patient> itself is singular, but <name> repeats), not
// force every level to [0] the way the original (pre-header-work) rule did.
// Real rule: PatientMappingRules()'s Scope "names[*]" (declarative_oob_
// rules.go) resolves against CDAPatient at header_parser.go:101-103's
// patEl.SelectElements("name") -- a genuinely repeating raw tag reached
// through the singular collapsed "patient" wrapper.
func TestResolveCDAPathsTranslated_MultiSegment_RepeatingLastLevel(t *testing.T) {
	patient := map[string]interface{}{
		"names": []interface{}{
			map[string]interface{}{"family": "Smith"},
			map[string]interface{}{"family": "Jones"},
		},
	}
	got := ResolveCDAPathTranslated(patient, "names[1]", "CDAPatient")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	want := "patient[0].name[1]"
	if got.XMLPath != want {
		t.Errorf("XMLPath = %q, want %q -- the real repeating index (1) must land on the LAST segment (name), with the singular collapsed wrapper (patient) forced to [0]", got.XMLPath, want)
	}

	// The pre-fix behavior (force [0] on every level) would have collapsed
	// names[0] and names[1] onto the identical key -- confirm they now
	// genuinely differ.
	got0 := ResolveCDAPathTranslated(patient, "names[0]", "CDAPatient")
	if got0.XMLPath == got.XMLPath {
		t.Errorf("names[0] and names[1] resolved to the SAME XMLPath %q -- multiSegment index collision (the exact bug this fix closes)", got0.XMLPath)
	}
}

// TestResolveCDAPathsTranslated_CDACustodian_InlineStruct_NoDoubledSegment
// proves CDACustodian.AssignedCustodian is xlInlineStruct, not xlSame:
// header_parser.go:216 (nav(root, "custodian", "assignedCustodian")) already
// consumes BOTH real raw levels before this Go field is populated, so
// resolving fields relative to a CDACustodian node must NOT grow the path by
// a further "assignedCustodian[0]" segment -- that would double-count it
// once this result gets composed onto the "custodian[0].assignedCustodian[0]"
// basePath BuildHeaderResourceWithCoveragePrefix already establishes when
// resolving "CDAHeader"."custodian" itself (see declarative_engine.go).
// Real rule: CustodianMappingRules()'s SourcePath "assignedCustodian.
// representedCustodianOrganization.names[0]".
func TestResolveCDAPathsTranslated_CDACustodian_InlineStruct_NoDoubledSegment(t *testing.T) {
	custodian := map[string]interface{}{
		"assignedCustodian": map[string]interface{}{
			"representedCustodianOrganization": map[string]interface{}{
				"names": []interface{}{"Acme Clinic"},
			},
		},
	}
	got := ResolveCDAPathTranslated(custodian, "assignedCustodian.representedCustodianOrganization.names[0]", "CDACustodian")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	want := "representedCustodianOrganization[0].name[0]"
	if got.XMLPath != want {
		t.Errorf("XMLPath = %q, want %q -- assignedCustodian must contribute ZERO path growth (xlInlineStruct); a leading \"assignedCustodian[0].\" here would double-count it against the caller's own already-established basePath", got.XMLPath, want)
	}
}

// TestResolveCDAPathsTranslated_CDAHeader_Patient proves the new "CDAHeader"
// root kind resolves CDAHeader.Patient to its real raw location -- TWO real
// XML levels (recordTarget, patientRole) collapsing into the one typed
// "patient" field (header_parser.go:71-72).
func TestResolveCDAPathsTranslated_CDAHeader_Patient(t *testing.T) {
	header := map[string]interface{}{
		"patient": map[string]interface{}{
			"ids": []interface{}{map[string]interface{}{"root": "2.16.840.1.113883.4.1"}},
		},
	}
	got := ResolveCDAPathTranslated(header, "patient", "CDAHeader")
	if !got.Translatable {
		t.Fatalf("Translatable = false, want true")
	}
	if got.XMLPath != "recordTarget[0].patientRole[0]" {
		t.Errorf("XMLPath = %q, want %q", got.XMLPath, "recordTarget[0].patientRole[0]")
	}
	if got.Kind != "CDAPatient" {
		t.Errorf("Kind = %q, want %q", got.Kind, "CDAPatient")
	}
}

func TestResolveCDAPathsTranslated_MalformedPathNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked: %v", r)
		}
	}()
	entry := map[string]interface{}{"code": map[string]interface{}{"code": "X"}}
	for _, path := range []string{
		"",
		"code[",
		"code[*",
		"code[bogus=key=val]",
		"a.b.c.d.e.f.g",
		"entryRelationships[typeCode=SUBJ].entry.entry.entry",
	} {
		_ = ResolveCDAPathsTranslated(entry, path, "CDAEntry")
	}
}

// TestResolveCDAPaths_Unaffected is a direct regression net for this
// package's PRE-EXISTING, heavily-used ResolveCDAPaths/ResolveCDAPath (see
// cda_path_resolver.go) -- this file's new walker reuses their unexported
// grammar (parseCDAPath, segment kinds, asCDANodeList, matchesCDAPredicates)
// but must never modify their behavior for any existing caller.
func TestResolveCDAPaths_Unaffected(t *testing.T) {
	entry := map[string]interface{}{
		"entryRelationships": []interface{}{
			map[string]interface{}{"typeCode": "SUBJ", "entry": map[string]interface{}{"entryType": "observation", "value": "x"}},
		},
	}
	nodes := ResolveCDAPaths(entry, "entryRelationships[typeCode=SUBJ].entry.value", false)
	if len(nodes) != 1 || nodes[0] != "x" {
		t.Fatalf("ResolveCDAPaths regressed: got %v", nodes)
	}
}
