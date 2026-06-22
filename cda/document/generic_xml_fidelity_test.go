// cda/document/generic_xml_fidelity_test.go
//
// Permanent regression guard for GenericXMLToJSON's completeness claim.
// Runs the real, value-presence fidelity check (every XML leaf value must
// appear somewhere in the generic JSON mirror) against a small, committed,
// multi-vendor corpus -- not just one hand-picked sample.
//
// This is the test that should fail if a future change to GenericXMLToJSON
// (or its helpers) ever silently drops data, regardless of which EHR vendor
// produced the source document.
package cdadocument

import (
	"strings"
	"testing"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/require"
)

// corpusFixtures lists the committed multi-vendor sample CCDs under
// testdata/corpus/. Sourced from https://github.com/chb/sample_ccdas
// (Cerner, Kareo, mTuitive, PracticeFusion) -- kept small deliberately so
// they're cheap to commit; the full 14-vendor/747-document corpus (also
// including Allscripts, EMERGE, Greenway, HL7, Kinsights, NextGen, NIST,
// Partners HealthCare, Transitions of Care, Vitera) was verified manually
// at 100% coverage and is not committed here due to size.
var corpusFixtures = []string{
	"corpus/cerner_sample.xml",
	"corpus/kareo_sample.xml",
	"corpus/mtuitive_sample.xml",
	"corpus/practicefusion_sample.xml",
}

func TestGenericXMLToJSON_CorpusFidelity(t *testing.T) {
	for _, fixture := range corpusFixtures {
		t.Run(fixture, func(t *testing.T) {
			_, root := loadXML(t, fixture)

			xmlValues := collectXMLLeafValues(root)
			require.NotEmpty(t, xmlValues, "fixture should contain real data")

			generic := GenericXMLToJSON(root)
			jsonValues := make(map[string]bool)
			collectGenericJSONLeafValues(generic, jsonValues)

			var missing []string
			for v := range xmlValues {
				if !jsonValues[v] {
					missing = append(missing, v)
				}
			}
			require.Empty(t, missing,
				"%s: %d XML values present in the source document never appear "+
					"anywhere in GenericXMLToJSON's output -- this breaks the "+
					"completeness guarantee that is this function's entire purpose",
				fixture, len(missing))
		})
	}
}

// collectXMLLeafValues walks every element and returns the set of
// non-trivial attribute values and own-text values found anywhere in the
// tree. "xmlns" and single/zero-length tokens are excluded as pure noise.
func collectXMLLeafValues(el *etree.Element) map[string]bool {
	out := make(map[string]bool)
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		for _, a := range e.Attr {
			if a.Key == "xmlns" {
				continue
			}
			v := strings.TrimSpace(a.Value)
			if len(v) >= 2 {
				out[v] = true
			}
		}
		if text := elementOwnText(e); len(text) >= 2 {
			out[text] = true
		}
		for _, child := range e.ChildElements() {
			walk(child)
		}
	}
	walk(el)
	return out
}

// collectGenericJSONLeafValues walks a GenericXMLToJSON result (map /
// []interface{} / string) and records every string value encountered.
func collectGenericJSONLeafValues(v interface{}, out map[string]bool) {
	switch t := v.(type) {
	case map[string]interface{}:
		for _, child := range t {
			collectGenericJSONLeafValues(child, out)
		}
	case []interface{}:
		for _, child := range t {
			collectGenericJSONLeafValues(child, out)
		}
	case string:
		s := strings.TrimSpace(t)
		if s != "" {
			out[s] = true
		}
	}
}
