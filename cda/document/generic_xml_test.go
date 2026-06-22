// cda/document/generic_xml_test.go
package cdadocument

import (
	"testing"

	"github.com/beevik/etree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseXMLString(t *testing.T, xml string) *etree.Element {
	t.Helper()
	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromString(xml))
	return doc.Root()
}

func TestGenericXMLToJSON_LeafElementCollapsesToString(t *testing.T) {
	root := parseXMLString(t, `<root><title>Hello</title></root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	assert.Equal(t, "Hello", out["title"])
}

func TestGenericXMLToJSON_AttributesPrefixed(t *testing.T) {
	root := parseXMLString(t, `<root><id root="1.2.3" extension="X1"/></root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	id := out["id"].(map[string]interface{})
	assert.Equal(t, "1.2.3", id["@root"])
	assert.Equal(t, "X1", id["@extension"])
}

func TestGenericXMLToJSON_RepeatedTagBecomesArray(t *testing.T) {
	root := parseXMLString(t, `<root>
		<templateId root="A"/>
		<templateId root="B"/>
		<templateId root="C"/>
	</root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	tids, ok := out["templateId"].([]interface{})
	require.True(t, ok, "three templateId siblings should become an array")
	require.Len(t, tids, 3)
	assert.Equal(t, "A", tids[0].(map[string]interface{})["@root"])
	assert.Equal(t, "B", tids[1].(map[string]interface{})["@root"])
	assert.Equal(t, "C", tids[2].(map[string]interface{})["@root"])
}

func TestGenericXMLToJSON_SingleOccurrenceNotWrappedInArray(t *testing.T) {
	root := parseXMLString(t, `<root><templateId root="A"/></root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	_, isArray := out["templateId"].([]interface{})
	assert.False(t, isArray, "a single occurrence should not be array-wrapped")
}

func TestGenericXMLToJSON_TailTextAfterChildElementCaptured(t *testing.T) {
	// "y" is XML "tail" text -- it follows <b/> as a sibling text node.
	// A naive implementation using only Element.Text() would drop it.
	root := parseXMLString(t, `<root><a>x<b/>y</a></root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	a := out["a"].(map[string]interface{})
	assert.Contains(t, a["#text"], "x")
	assert.Contains(t, a["#text"], "y")
}

func TestGenericXMLToJSON_NegationIndAndNullFlavorSurviveGenerically(t *testing.T) {
	// The exact pattern that needed individual struct fixes in the typed
	// model (negationInd, nullFlavor) should need ZERO special-casing here
	// -- that's the entire point of the generic mirror.
	root := parseXMLString(t, `<root>
		<observation classCode="OBS" moodCode="EVN" negationInd="true">
			<id nullFlavor="UNK"/>
			<doseQuantity nullFlavor="UNK"/>
		</observation>
	</root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	obs := out["observation"].(map[string]interface{})
	assert.Equal(t, "true", obs["@negationInd"])
	id := obs["id"].(map[string]interface{})
	assert.Equal(t, "UNK", id["@nullFlavor"])
	dq := obs["doseQuantity"].(map[string]interface{})
	assert.Equal(t, "UNK", dq["@nullFlavor"])
}

func TestGenericXMLToJSON_UnknownVendorExtensionElementSurvivesWithNoCodeChange(t *testing.T) {
	// Simulates a made-up, never-seen-before vendor extension element/attr.
	// No struct anywhere knows about "epic:customFlag" -- it must still
	// appear, proving there is no enumerable whitelist to fall through.
	root := parseXMLString(t, `<root>
		<substanceAdministration classCode="SBADM">
			<vendorSpecificThing neverModeled="surpriseValue">
				<anotherNewElement>deeply nested surprise text</anotherNewElement>
			</vendorSpecificThing>
		</substanceAdministration>
	</root>`)
	out := GenericXMLToJSON(root).(map[string]interface{})
	sa := out["substanceAdministration"].(map[string]interface{})
	vst := sa["vendorSpecificThing"].(map[string]interface{})
	assert.Equal(t, "surpriseValue", vst["@neverModeled"])
	assert.Equal(t, "deeply nested surprise text", vst["anotherNewElement"])
}

func TestGenericXMLToJSON_NilElementReturnsNil(t *testing.T) {
	assert.Nil(t, GenericXMLToJSON(nil))
}
