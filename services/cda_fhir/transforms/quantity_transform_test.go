package transforms

import (
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

// TestCDAValueToFHIR_NullFlavorCDWithTranslation_StillResolves is a
// regression test for a real bug a 747-file sample_ccdas corpus run found:
// CDAValueToFHIR used to blanket-return nil whenever v.NullFlavor != "",
// even when the nested Code carried a usable Translation (a common
// real-world idiom -- the primary code is marked nullFlavor="UNK"/"OTH"
// while a <translation> child holds the actual mapped code). This was the
// single largest cause of Condition.code coming back completely empty
// across the corpus.
func TestCDAValueToFHIR_NullFlavorCDWithTranslation_StillResolves(t *testing.T) {
	v := cdadocument.CDAValue{
		Type:       "CD",
		NullFlavor: "UNK",
		Code: &cdadocument.CDACode{
			NullFlavor: "UNK",
			Translations: []cdadocument.CDACode{
				{Code: "233604007", DisplayName: "Pneumonia", CodeSystem: "2.16.840.1.113883.6.96"},
			},
		},
	}
	got := CDAValueToFHIR(v)
	cc, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("CDAValueToFHIR(nullFlavor CD w/ translation) = %v (%T), want a CodeableConcept map", got, got)
	}
	codings, _ := cc["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("expected at least one coding from the translation, got none")
	}
	coding, _ := codings[0].(map[string]interface{})
	if coding["code"] != "233604007" {
		t.Errorf("coding = %v, want code=233604007 (from Translation)", coding)
	}
}

// TestCDAValueToFHIR_NullFlavorCDNoData_StillReturnsNil confirms the fix
// didn't loosen the genuinely-empty case: a nullFlavor code with no code, no
// display, and no translations must still produce nil (so dataAbsentReason
// fallbacks elsewhere keep firing correctly).
func TestCDAValueToFHIR_NullFlavorCDNoData_StillReturnsNil(t *testing.T) {
	v := cdadocument.CDAValue{
		Type:       "CD",
		NullFlavor: "UNK",
		Code:       &cdadocument.CDACode{NullFlavor: "UNK"},
	}
	got := CDAValueToFHIR(v)
	if got != nil {
		t.Errorf("CDAValueToFHIR(genuinely empty nullFlavor CD) = %v, want nil", got)
	}
}

// TestCDAValueToFHIR_NullFlavorNoType_ReturnsNil covers the shape used by
// declarative_oob_rules_test.go's existing
// TestDeclarativeEngine_Results_NullFlavorValue_ProducesDataAbsentReason --
// a bare nullFlavor value with no Type and no nested data at all must still
// resolve to nil, unaffected by this fix.
func TestCDAValueToFHIR_NullFlavorNoType_ReturnsNil(t *testing.T) {
	v := cdadocument.CDAValue{NullFlavor: "NI"}
	got := CDAValueToFHIR(v)
	if got != nil {
		t.Errorf("CDAValueToFHIR(bare nullFlavor, no type) = %v, want nil", got)
	}
}
