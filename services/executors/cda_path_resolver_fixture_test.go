// services/executors/cda_path_resolver_fixture_test.go
//
// Phase 1 exit criteria: "standalone test suite proving wildcard/predicate
// resolution against real fixture documents (reuse negation_and_frequency.xml,
// supply_and_nullflavors.xml, the corpus fixtures already committed under
// cda/document/testdata/)." These tests parse the real fixture files through
// the same two production steps cda_parser_service.go runs --
// cdadocument.GenericXMLToJSON (the "xml.*" mirror) and
// cdadocument.NewCDAParser(...).ParseDocument (the typed "document.*" tree,
// then JSON-round-tripped exactly as ParsedJSON["document"] is once it
// leaves the in-process handoff) -- rather than hand-built fixtures, so a
// resolver bug that only shows up against a real parse can't hide behind a
// fixture that's accidentally too convenient.
package executors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"

	"github.com/beevik/etree"
)

// cdaTestdataRoot resolves cda/document/testdata relative to this file,
// mirroring document_mapper_bench_test.go's repo-root discovery (services/
// executors is two levels below the repo root, same depth as services/
// cda_fhir).
func cdaTestdataRoot(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "cda", "document", "testdata")
}

func cdaSchemaRoot(t testing.TB) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "cda", "schemas")
}

// parsedCDAFixture holds both representations a resolver call can target,
// built from the same parsed document -- exactly the two keys
// cda_parser_service.go assembles into ParsedJSON ("xml" and "document").
type parsedCDAFixture struct {
	xmlMirror   map[string]interface{}
	documentMap map[string]interface{}
}

// loadParsedCDAFixture reads relPath under cda/document/testdata, parses it
// with the real etree + CDAParser + GenericXMLToJSON production code, and
// JSON-round-trips the typed document exactly as the live *CDADocument Go
// object does once it crosses the pipeline boundary (see field_utils.go's
// resolveCDADocumentFieldValue doc comment on why this round-trip matters).
func loadParsedCDAFixture(t testing.TB, relPath string) parsedCDAFixture {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(cdaTestdataRoot(t), relPath))
	if err != nil {
		t.Fatalf("reading testdata %s: %v", relPath, err)
	}
	raw := string(data)

	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatalf("parsing XML in %s: %v", relPath, err)
	}
	root := doc.Root()

	genericXML := cdadocument.GenericXMLToJSON(root)
	xmlMap, ok := genericXML.(map[string]interface{})
	if !ok {
		t.Fatalf("GenericXMLToJSON(%s) did not return a map at the document root", relPath)
	}

	loader, err := cdaSchema.NewCDASchemaLoader(cdaSchemaRoot(t))
	if err != nil {
		t.Fatalf("loading CDA schema: %v", err)
	}
	typedDoc := cdadocument.NewCDAParser(loader).ParseDocument(root, raw)

	encoded, err := json.Marshal(typedDoc)
	if err != nil {
		t.Fatalf("marshalling typed document for %s: %v", relPath, err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		t.Fatalf("unmarshalling typed document for %s: %v", relPath, err)
	}

	return parsedCDAFixture{xmlMirror: xmlMap, documentMap: documentMap}
}

// ===============================================================
// negation_and_frequency.xml -- negated allergy assertion (SUBJ + negationInd)
// and a substanceAdministration with two effectiveTime entries (duration +
// frequency) plus repeatNumber.
// ===============================================================

func TestRealFixture_NegationAndFrequency_DocumentTree_NegationViaPredicate(t *testing.T) {
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	path := "sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[typeCode=SUBJ].entry.negationInd"
	got := ResolveCDAPath(fx.documentMap, path, false)
	negated, ok := got.(bool)
	if !ok || !negated {
		t.Fatalf("resolved %v, want true -- this is the exact field "+
			"TestNegationIndParsed (cda/document/parser_test.go) already proves "+
			"the parser captures; the resolver must reach it via a predicate, "+
			"not a hardcoded index", got)
	}
}

func TestRealFixture_NegationAndFrequency_DocumentTree_TemplateIDDiscriminator(t *testing.T) {
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	// The nested assertion observation carries two templateIds
	// (TestEntryTemplateIdsParsed asserts both at the parser level). Confirm
	// the resolver's templateId predicate matches either one independently.
	for _, templateID := range []string{
		"2.16.840.1.113883.10.20.22.4.7",
		"2.16.840.1.113883.10.20.24.3.90",
	} {
		path := "sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[typeCode=SUBJ].entry[templateId=" + templateID + "]"
		if got := ResolveCDAPath(fx.documentMap, path, false); got == nil {
			t.Errorf("templateId %s did not match via the resolver", templateID)
		}
	}

	wrongID := "2.16.840.1.113883.10.20.22.4.999"
	path := "sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[typeCode=SUBJ].entry[templateId=" + wrongID + "]"
	if got := ResolveCDAPath(fx.documentMap, path, false); got != nil {
		t.Errorf("got %v, want nil for a templateId the entry does not carry", got)
	}
}

func TestRealFixture_NegationAndFrequency_XMLMirror_NegationViaPredicate(t *testing.T) {
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	// Allergies is the first <component> under <structuredBody> in this
	// fixture (see negation_and_frequency.xml's document order).
	path := "component.structuredBody.component[0].section.entry.act.entryRelationship[typeCode=SUBJ].observation.@negationInd"
	got := ResolveCDAPath(fx.xmlMirror, path, true)
	if got != "true" {
		t.Fatalf("resolved %v, want the string \"true\" via the xml.* mirror "+
			"(attributes are strings on this backend, unlike the typed tree's bool)", got)
	}
}

func TestRealFixture_NegationAndFrequency_BothBackendsAgreeOnPresence(t *testing.T) {
	// Cross-backend consistency check: the typed tree and the generic mirror
	// describe the same source document, so a predicate that matches on one
	// must also find the equivalent node on the other -- they just disagree
	// on the leaf's Go type (bool vs "true"/"false" string), not on whether
	// the match exists.
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	docPath := "sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[typeCode=SUBJ].entry.negationInd"
	xmlPath := "component.structuredBody.component[0].section.entry.act.entryRelationship[typeCode=SUBJ].observation.@negationInd"

	docResult := ResolveCDAPath(fx.documentMap, docPath, false)
	xmlResult := ResolveCDAPath(fx.xmlMirror, xmlPath, true)

	if docResult != true {
		t.Errorf("document tree: got %v, want bool true", docResult)
	}
	if xmlResult != "true" {
		t.Errorf("xml mirror: got %v, want string \"true\"", xmlResult)
	}
}

func TestRealFixture_NegationAndFrequency_DocumentTree_WildcardOverEffectiveTimes(t *testing.T) {
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	// TestMultipleEffectiveTimesParsed (parser_test.go) proves the parser
	// captures both the IVL_TS (duration) and PIVL_TS (frequency) entries in
	// document order. The resolver's wildcard must collect both, not just
	// the first.
	path := "sectionsByKey.medications.entries[0].effectiveTimes[*].xsiType"
	got := ResolveCDAPaths(fx.documentMap, path, false)
	want := []interface{}{"IVL_TS", "PIVL_TS"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRealFixture_NegationAndFrequency_XMLMirror_WildcardOverEffectiveTimes(t *testing.T) {
	fx := loadParsedCDAFixture(t, "negation_and_frequency.xml")

	// Medications is the second <component>. Two sibling <effectiveTime>
	// elements on substanceAdministration auto-array on the mirror backend.
	path := "component.structuredBody.component[1].section.entry.substanceAdministration.effectiveTime[*].@type"
	got := ResolveCDAPaths(fx.xmlMirror, path, true)
	want := []interface{}{"IVL_TS", "PIVL_TS"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// ===============================================================
// supply_and_nullflavors.xml -- nullFlavor handling, no @value attribute,
// and a <supply> entryRelationship carrying its own quantity/product.
// ===============================================================

func TestRealFixture_SupplyAndNullFlavors_DocumentTree_FallbackChainToNullFlavor(t *testing.T) {
	fx := loadParsedCDAFixture(t, "supply_and_nullflavors.xml")

	// doseQuantity has nullFlavor="UNK" and no @value attribute --
	// CDAQuantity.Value's `json:",omitempty"` tag means the "value" key is
	// absent entirely after the round-trip, not present-but-empty. This is
	// the exact "field A, else field B" shape cross-cutting finding #1 calls
	// for: TestDoseQuantityNullFlavorParsed already proves the parser
	// captures the nullFlavor; this proves the resolver's fallback chain
	// reaches it when the primary field is unusable.
	got := ResolveCDAFallback(fx.documentMap, false,
		"sectionsByKey.medications.entries[0].doseQuantity.value",
		"sectionsByKey.medications.entries[0].doseQuantity.nullFlavor",
	)
	if got != "UNK" {
		t.Fatalf("got %v, want \"UNK\" via the fallback chain", got)
	}
}

func TestRealFixture_SupplyAndNullFlavors_DocumentTree_SupplyEntryRelationshipPredicate(t *testing.T) {
	fx := loadParsedCDAFixture(t, "supply_and_nullflavors.xml")

	// TestSupplyEntryRelationship_RepeatNumberQuantityAndProductParsed
	// (parser_test.go) proves the parser captures the REFR-typed supply
	// entryRelationship's own quantity. Confirm the resolver reaches the
	// same field via a typeCode predicate instead of a hardcoded index.
	path := "sectionsByKey.medications.entries[0].entryRelationships[typeCode=REFR].entry.quantity.value"
	got := ResolveCDAPath(fx.documentMap, path, false)
	if got != "15" {
		t.Fatalf("got %v, want \"15\"", got)
	}
}

func TestRealFixture_SupplyAndNullFlavors_XMLMirror_SupplyEntryRelationshipPredicate(t *testing.T) {
	fx := loadParsedCDAFixture(t, "supply_and_nullflavors.xml")

	path := "component.structuredBody.component[0].section.entry.substanceAdministration.entryRelationship[typeCode=REFR].supply.quantity.@value"
	got := ResolveCDAPath(fx.xmlMirror, path, true)
	if got != "15" {
		t.Fatalf("got %v, want \"15\"", got)
	}
}

func TestRealFixture_SupplyAndNullFlavors_DocumentTree_AuthorIDNullFlavor(t *testing.T) {
	fx := loadParsedCDAFixture(t, "supply_and_nullflavors.xml")

	// TestAuthorIdNullFlavorParsed proves the parser keeps a nullFlavor=UNK
	// <id> with no root/extension. Entry-level Authors is a plain (non-
	// repeating-in-this-fixture) slice -- exercised here via [0], the
	// already-supported index form, to confirm it still composes with the
	// new resolver rather than only the predicate/wildcard forms.
	path := "sectionsByKey.medications.entries[0].authors[0].assignedAuthor.ids[0].nullFlavor"
	got := ResolveCDAPath(fx.documentMap, path, false)
	if got != "UNK" {
		t.Fatalf("got %v, want \"UNK\"", got)
	}
}
