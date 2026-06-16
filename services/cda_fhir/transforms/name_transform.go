package transforms

import cdadocument "ezhealthkonnect/cda/document"

// CDANameToFHIR converts a typed CDAName (PN) to a FHIR HumanName map.
// Returns nil when the name carries no data fields.
func CDANameToFHIR(name cdadocument.CDAName) map[string]interface{} {
	if name.Family == "" && len(name.Given) == 0 && name.Prefix == "" && name.Suffix == "" {
		return nil
	}
	hn := map[string]interface{}{}
	switch name.Use {
	case "L", "":
		hn["use"] = "official"
	case "A":
		hn["use"] = "nickname"
	case "P":
		hn["use"] = "anonymous"
	default:
		hn["use"] = "usual"
	}
	if name.Family != "" {
		hn["family"] = name.Family
	}
	if len(name.Given) > 0 {
		var given []interface{}
		for _, g := range name.Given {
			if g != "" {
				given = append(given, g)
			}
		}
		if len(given) > 0 {
			hn["given"] = given
		}
	}
	if name.Prefix != "" {
		hn["prefix"] = []interface{}{name.Prefix}
	}
	if name.Suffix != "" {
		hn["suffix"] = []interface{}{name.Suffix}
	}
	if period := CDATimeRangeToPeriod(name.ValidTime); period != nil {
		hn["period"] = period
	}
	// Return nil if the only field set was "use"
	if len(hn) <= 1 {
		return nil
	}
	return hn
}

// CDAAddressToFHIR converts a typed CDAAddress (AD) to a FHIR Address map.
// Returns nil for null-flavored or completely empty addresses.
func CDAAddressToFHIR(addr cdadocument.CDAAddress) map[string]interface{} {
	if addr.NullFlavor != "" {
		return nil
	}
	if len(addr.StreetLines) == 0 && addr.City == "" && addr.State == "" && addr.PostalCode == "" {
		return nil
	}
	a := map[string]interface{}{}
	switch addr.Use {
	case "HP":
		a["use"] = "home"
	case "WP":
		a["use"] = "work"
	case "TMP":
		a["use"] = "temp"
	default:
		if addr.Use != "" {
			a["use"] = "home"
		}
	}
	if len(addr.StreetLines) > 0 {
		var lines []interface{}
		for _, l := range addr.StreetLines {
			if l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) > 0 {
			a["line"] = lines
		}
	}
	if addr.City != "" {
		a["city"] = addr.City
	}
	if addr.County != "" {
		a["district"] = addr.County
	}
	if addr.State != "" {
		a["state"] = addr.State
	}
	if addr.PostalCode != "" {
		a["postalCode"] = addr.PostalCode
	}
	if addr.Country != "" {
		a["country"] = addr.Country
	}
	if period := CDATimeRangeToPeriod(addr.ValidTime); period != nil {
		a["period"] = period
	}
	if len(a) == 0 {
		return nil
	}
	return a
}

// CDATelecomToFHIR converts a typed CDATelecom (TEL) to a FHIR ContactPoint map.
// Returns nil when value is empty.
func CDATelecomToFHIR(tel cdadocument.CDATelecom) map[string]interface{} {
	if tel.Value == "" {
		return nil
	}
	cp := map[string]interface{}{}
	switch {
	case len(tel.Value) > 4 && tel.Value[:4] == "tel:":
		cp["system"] = "phone"
		cp["value"] = tel.Value[4:]
	case len(tel.Value) > 7 && tel.Value[:7] == "mailto:":
		cp["system"] = "email"
		cp["value"] = tel.Value[7:]
	case len(tel.Value) > 4 && tel.Value[:4] == "fax:":
		cp["system"] = "fax"
		cp["value"] = tel.Value[4:]
	default:
		cp["system"] = "other"
		cp["value"] = tel.Value
	}
	switch tel.Use {
	case "HP":
		cp["use"] = "home"
	case "WP":
		cp["use"] = "work"
	case "MC":
		cp["use"] = "mobile"
	}
	return cp
}
