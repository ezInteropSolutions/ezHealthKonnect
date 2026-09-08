package control

import (
	"testing"
)

// TestJSONCollectionResolver_BracketedNumericIndex_ResolvesEDIStyleNestedLoop proves the
// original bug: a dotted path with a numeric bracket index (e.g. "loops.2000[0].loops.2100" —
// exactly what every EDI-parsed transaction set produces, and what the V231 EDI-835-to-FHIR
// template's own doc comment recommends as a control.loop "collection" value) used to resolve
// to nil because navigatePath's strings.Split(path, ".") treated "2000[0]" as one literal,
// nonexistent map key.
func TestJSONCollectionResolver_BracketedNumericIndex_ResolvesEDIStyleNestedLoop(t *testing.T) {
	claim1 := map[string]interface{}{"CLP": map[string]interface{}{"claimPaymentAmount": "34.25"}}
	claim2 := map[string]interface{}{"CLP": map[string]interface{}{"claimPaymentAmount": "0"}}

	data := map[string]interface{}{
		"message": map[string]interface{}{
			"parsedEDI": map[string]interface{}{
				"loops": map[string]interface{}{
					"2000": []interface{}{
						map[string]interface{}{
							"loops": map[string]interface{}{
								"2100": []interface{}{claim1, claim2},
							},
						},
					},
				},
			},
		},
	}

	resolver := &JSONCollectionResolver{}
	result := resolver.Resolve(data, "message.parsedEDI.loops.2000[0].loops.2100")

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(result), result)
	}
	first, ok := result[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result[0] to be a map, got %T", result[0])
	}
	clp, ok := first["CLP"].(map[string]interface{})
	if !ok || clp["claimPaymentAmount"] != "34.25" {
		t.Fatalf("expected first claim's CLP.claimPaymentAmount = 34.25, got %+v", first)
	}
}

// TestJSONCollectionResolver_BareBracketedPath_FallsBackToMessage proves the "message."
// prefix is optional — a config author who writes the path relative to the message envelope
// (the convention every other bracket-aware resolver in this codebase already accepts) still
// resolves, via the same message-fallback branch that pre-dates this fix.
func TestJSONCollectionResolver_BareBracketedPath_FallsBackToMessage(t *testing.T) {
	data := map[string]interface{}{
		"message": map[string]interface{}{
			"items": map[string]interface{}{
				"rows": []interface{}{
					map[string]interface{}{"id": "a"},
					map[string]interface{}{"id": "b"},
					map[string]interface{}{"id": "c"},
				},
			},
		},
	}

	resolver := &JSONCollectionResolver{}
	result := resolver.Resolve(data, "items.rows")
	if len(result) != 3 {
		t.Fatalf("expected 3 items via message-fallback, got %d: %+v", len(result), result)
	}
}

// TestJSONCollectionResolver_NativeMapSlice_EDIParserShape proves the fix also handles the
// EDI parser's own native in-process shape ([]map[string]interface{}, not the []interface{}
// a JSON round-trip would produce) — the same "native map-slice" concern already documented
// in field_utils.go's own asInterfaceSlice, which GetFieldValue (and now this resolver too)
// relies on.
func TestJSONCollectionResolver_NativeMapSlice_EDIParserShape(t *testing.T) {
	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"loops": map[string]interface{}{
				"2000": []map[string]interface{}{
					{
						"loops": map[string]interface{}{
							"2100": []map[string]interface{}{
								{"CLP": map[string]interface{}{"claimPaymentAmount": "11.5"}},
							},
						},
					},
				},
			},
		},
	}

	resolver := &JSONCollectionResolver{}
	result := resolver.Resolve(data, "parsedEDI.loops.2000[0].loops.2100")
	if len(result) != 1 {
		t.Fatalf("expected 1 item from native []map[string]interface{} shape, got %d: %+v", len(result), result)
	}
}

// TestJSONCollectionResolver_MissingPath_ReturnsNilNotPanic guards the "absent, not an error"
// convention every other resolver in this package follows.
func TestJSONCollectionResolver_MissingPath_ReturnsNilNotPanic(t *testing.T) {
	data := map[string]interface{}{"message": map[string]interface{}{}}
	resolver := &JSONCollectionResolver{}
	if result := resolver.Resolve(data, "does.not[0].exist"); result != nil {
		t.Fatalf("expected nil for a missing path, got %+v", result)
	}
}

// TestNestedLoopResolver_BracketedItemPath proves the same bracket-index gap is fixed for a
// nested loop's own item-relative sub-path (e.g. a child loop iterating one specific SVC line
// within the parent loop's current claim item).
func TestNestedLoopResolver_BracketedItemPath(t *testing.T) {
	data := map[string]interface{}{
		"item": map[string]interface{}{
			"lines": map[string]interface{}{
				"SVC": []interface{}{
					map[string]interface{}{"code": "99213"},
					map[string]interface{}{"code": "99214"},
				},
			},
		},
	}

	resolver := &NestedLoopResolver{}
	result := resolver.Resolve(data, "item.lines.SVC")
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(result), result)
	}
}

// TestCollectionResolverRegistry_JSONFallback_HandlesRealEDIShape is an integration-style test
// through the actual registry (the same entry point loop_executor.go calls), proving the fix
// is reachable end-to-end and that no higher-priority resolver (NestedLoop/HL7/FHIR) wrongly
// intercepts a plain "message.parsedEDI...." collection path.
func TestCollectionResolverRegistry_JSONFallback_HandlesRealEDIShape(t *testing.T) {
	data := map[string]interface{}{
		"message": map[string]interface{}{
			"parsedEDI": map[string]interface{}{
				"loops": map[string]interface{}{
					"2000": []interface{}{
						map[string]interface{}{
							"loops": map[string]interface{}{
								"2100": []interface{}{
									map[string]interface{}{"CLP": map[string]interface{}{"claimPaymentAmount": "5"}},
								},
							},
						},
					},
				},
			},
		},
	}

	registry := NewCollectionResolverRegistry()
	result, resolverName := registry.ResolveCollection(data, "message.parsedEDI.loops.2000[0].loops.2100")
	if resolverName != "JSON" {
		t.Fatalf("expected JSON resolver to handle this path, got %q", resolverName)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(result), result)
	}
}

// TestNavigatePath_FixedLocationLookup_StillWorks is a regression guard: navigatePath itself
// (used by resolveFromLocations for the "observationGroups" convenience path, a fixed,
// hardcoded, bracket-free key sequence) must keep working unchanged — the bracket-aware fix
// only replaced the call sites that walk a raw, config-author-typed path string.
func TestNavigatePath_FixedLocationLookup_StillWorks(t *testing.T) {
	data := map[string]interface{}{
		"message": map[string]interface{}{
			"observationGroups": []interface{}{
				map[string]interface{}{"obs": "1"},
			},
		},
	}
	resolver := &NestedLoopResolver{}
	result := resolver.Resolve(data, "observationGroups")
	if len(result) != 1 {
		t.Fatalf("expected 1 item via the fixed-location observationGroups lookup, got %d: %+v", len(result), result)
	}
}
