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
// Returns nil for null-flavored or unrecognised types.
func CDAValueToFHIR(v cdadocument.CDAValue) interface{} {
	if v.NullFlavor != "" {
		return nil
	}
	switch v.Type {
	case "PQ":
		if v.Quantity != nil {
			return CDAQuantityToFHIR(*v.Quantity)
		}
	case "CD", "CE", "CS", "CV":
		if v.Code != nil {
			return CDACodeToCodeableConcept(*v.Code)
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
