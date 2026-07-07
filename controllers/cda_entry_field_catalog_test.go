// controllers/cda_entry_field_catalog_test.go
package controllers

import (
	"reflect"
	"strings"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

// cdaEntryFieldsExcludedFromDirectCatalog documents every CDAEntry JSON field
// deliberately left out of cdaDirectEntryFields (cda_schema_controller.go),
// with the reason: either a different Add Field pattern already reaches it
// (participant / related-act / author-performer), or it's a nested struct /
// internal discriminator rather than a leaf value a "Direct field on this
// entry" SourcePath can target in one hop.
var cdaEntryFieldsExcludedFromDirectCatalog = map[string]string{
	"additionalValues":  "plural sibling of value — an edge case beyond the primary field, not the first thing a user searches for",
	"participants":      `reached via the "Participant field" Add Field pattern, not "Direct field on this entry"`,
	"components":        "nested organizer sub-entries, not a leaf value",
	"entryRelationships": `reached via the "Related act (by relationship type)" Add Field pattern`,
	"entryType":          `internal type discriminator ("observation"/"act"/...), not a clinical value to map`,
	"templateIds":        "entry-level template OID list, not a clinical value to map",
	"effectiveTimes":     "plural sibling of effectiveTime — an edge case beyond the primary field",
	"authors":            `reached via the "Author / performer identity" Add Field pattern`,
	"performers":         `reached via the "Author / performer identity" Add Field pattern`,
	"informants":         "nested struct — no Add Field pattern surfaces this yet, and no real corpus entry has been seen using it",
	"consumable":         "nested struct — reaching a leaf value needs further drill-down (e.g. consumable.manufacturedProduct.manufacturedMaterial.code), not a one-hop direct field",
	"product":            "nested struct — reaching a leaf value needs further drill-down, not a one-hop direct field",
}

// TestCDADirectEntryFields_CoversEveryCDAEntryField is a drift guard: if
// CDAEntry (cda/document/types.go) gains a new top-level JSON field, this
// test fails until that field is explicitly triaged into either
// cdaDirectEntryFields or cdaEntryFieldsExcludedFromDirectCatalog above — so
// a new field can never silently go missing from the "Direct field on this
// entry" smart search catalog without a deliberate decision being made.
func TestCDADirectEntryFields_CoversEveryCDAEntryField(t *testing.T) {
	catalogPaths := make(map[string]bool, len(cdaDirectEntryFields))
	for _, f := range cdaDirectEntryFields {
		catalogPaths[f.Path] = true
	}

	entryType := reflect.TypeOf(cdadocument.CDAEntry{})
	liveFieldNames := make(map[string]bool, entryType.NumField())

	for i := 0; i < entryType.NumField(); i++ {
		field := entryType.Field(i)
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		name := strings.Split(jsonTag, ",")[0]
		if name == "" {
			continue
		}
		liveFieldNames[name] = true

		if catalogPaths[name] {
			continue
		}
		if _, excluded := cdaEntryFieldsExcludedFromDirectCatalog[name]; excluded {
			continue
		}
		t.Errorf(
			"CDAEntry field %q (Go field %s) is in neither cdaDirectEntryFields nor "+
				"cdaEntryFieldsExcludedFromDirectCatalog — triage it into one of the two so "+
				`the "Direct field on this entry" smart search catalog doesn't silently drift out of sync`,
			name, field.Name,
		)
	}

	// Reverse guard: an excluded name that no longer exists on CDAEntry
	// (renamed/removed) would silently document nothing — fail loudly.
	for excludedName := range cdaEntryFieldsExcludedFromDirectCatalog {
		if !liveFieldNames[excludedName] {
			t.Errorf("cdaEntryFieldsExcludedFromDirectCatalog references %q, which no longer exists as a CDAEntry JSON field — remove the stale entry", excludedName)
		}
	}
}
