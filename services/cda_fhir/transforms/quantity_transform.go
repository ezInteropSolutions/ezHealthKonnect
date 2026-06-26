package transforms

import (
	"strconv"

	cdadocument "ezhealthkonnect/cda/document"
)

// CDAQuantityToFHIR converts a typed CDAQuantity (PQ) to a FHIR Quantity map.
// Returns nil when value is empty.
func CDAQuantityToFHIR(q cdadocument.CDAQuantity) map[string]interface{} {
	if q.Value == "" {
		return nil
	}
	qty := map[string]interface{}{}
	if f, err := strconv.ParseFloat(q.Value, 64); err == nil {
		qty["value"] = f
	} else {
		qty["value"] = q.Value
	}
	if q.Unit != "" {
		qty["unit"] = q.Unit
		qty["system"] = "http://unitsofmeasure.org"
		qty["code"] = q.Unit
	}
	return qty
}

// CDAValueToFHIR converts a polymorphic CDAValue to a FHIR-compatible Go value.
// Returns nil when no branch below can extract usable data.
//
// Deliberately does NOT blanket-return nil on v.NullFlavor != "" (the
// previous behavior): a 747-file real-world corpus run
// (github.com/chb/sample_ccdas) found CD-typed values commonly carry
// nullFlavor="UNK"/"OTH" on the value itself while a <translation> child
// holds the actual usable code (e.g. a SNOMED problem code translated from
// a vendor-internal primary code marked nullFlavor) -- common enough to
// account for the single largest non-Immunization validation gap found
// (Condition.code missing). CDACodeToCodeableConcept already has its own
// correct nullFlavor-aware emptiness check (nullFlavor AND no code AND no
// display AND no text -- see that function's own check), so delegating to
// it for the CD/CE/CS/CV case is enough; every other case below already
// guards on its own nil/empty-string check, so a genuinely-empty
// nullFlavor'd PQ/ST/BL/INT/REAL/TS/IVL_TS value still correctly falls
// through to the final "return nil".
func CDAValueToFHIR(v cdadocument.CDAValue) interface{} {
	switch v.Type {
	case "PQ":
		// Explicit nil check before returning, not a bare "return
		// CDAQuantityToFHIR(...)" -- CDAQuantityToFHIR's own nil return is
		// typed (map[string]interface{}), and returning a typed nil through
		// this function's interface{} return type would box it into a
		// NON-nil interface (Go's classic typed-nil-in-interface trap),
		// breaking every "if CDAValueToFHIR(...) != nil" caller. Same fix
		// applied to the CD/CE/CS/CV case below.
		if v.Quantity != nil {
			if q := CDAQuantityToFHIR(*v.Quantity); q != nil {
				return q
			}
		}
	case "CD", "CE", "CS", "CV":
		if v.Code != nil {
			if cc := CDACodeToCodeableConcept(*v.Code); cc != nil {
				return cc
			}
		}
	case "ST", "ED":
		if v.Text != "" {
			return v.Text
		}
	case "BL":
		if v.Boolean != nil {
			return *v.Boolean
		}
	case "INT":
		if v.Integer != nil {
			return *v.Integer
		}
	case "REAL":
		if v.Real != nil {
			return *v.Real
		}
	case "TS":
		return CDATimeToFHIRDateTime(cdadocument.CDATime{Value: v.Text})
	case "IVL_TS":
		if v.TimeRange != nil {
			return CDATimeRangeToPeriod(*v.TimeRange)
		}
	}
	return nil
}
