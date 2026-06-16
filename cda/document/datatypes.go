// cda/document/datatypes.go
// Parsers for each CDA primitive data type.
// Every function accepts a *etree.Element and returns the corresponding typed struct.
// nil input is handled gracefully — the caller never needs to nil-check before calling.
//
// Key rule: SelectElements is used for every attribute that can repeat.
// nav() replaces FindElement for path navigation to eliminate deep-path XPath calls.

package cdadocument

import (
	"strings"

	"github.com/beevik/etree"
)

// ========================
// Navigation helper
// ========================

// nav navigates a path of direct-child element tags from el, returning nil if any
// step produces no match or if el is nil. Use this instead of FindElement so that
// no FindElement call appears in cda/document/*.go files.
func nav(el *etree.Element, tags ...string) *etree.Element {
	cur := el
	for _, tag := range tags {
		if cur == nil {
			return nil
		}
		cur = cur.SelectElement(tag)
	}
	return cur
}

// ========================
// CDA primitive parsers
// ========================

// parseII extracts an HL7 II (Instance Identifier) from an element.
func parseII(el *etree.Element) CDAII {
	if el == nil {
		return CDAII{}
	}
	return CDAII{
		Root:                   el.SelectAttrValue("root", ""),
		Extension:              el.SelectAttrValue("extension", ""),
		AssigningAuthorityName: el.SelectAttrValue("assigningAuthorityName", ""),
		DisplayName:            el.SelectAttrValue("displayName", ""),
	}
}

// parseCD extracts an HL7 CD/CE/CS coded concept from an element.
func parseCD(el *etree.Element) CDACode {
	if el == nil {
		return CDACode{}
	}
	c := CDACode{
		Code:           el.SelectAttrValue("code", ""),
		DisplayName:    el.SelectAttrValue("displayName", ""),
		CodeSystem:     el.SelectAttrValue("codeSystem", ""),
		CodeSystemName: el.SelectAttrValue("codeSystemName", ""),
		NullFlavor:     el.SelectAttrValue("nullFlavor", ""),
	}
	if ot := el.SelectElement("originalText"); ot != nil {
		c.OriginalText = strings.TrimSpace(ot.Text())
	}
	for _, t := range el.SelectElements("translation") {
		c.Translations = append(c.Translations, parseCD(t))
	}
	return c
}

// parseTS extracts an HL7 TS (timestamp) from an element.
func parseTS(el *etree.Element) CDATime {
	if el == nil {
		return CDATime{}
	}
	return CDATime{
		Value:      el.SelectAttrValue("value", ""),
		Operator:   el.SelectAttrValue("operator", ""),
		NullFlavor: el.SelectAttrValue("nullFlavor", ""),
	}
}

// parsePQ extracts an HL7 PQ (Physical Quantity) from an element.
func parsePQ(el *etree.Element) CDAQuantity {
	if el == nil {
		return CDAQuantity{}
	}
	return CDAQuantity{
		Value: el.SelectAttrValue("value", ""),
		Unit:  el.SelectAttrValue("unit", ""),
	}
}

// parsePN extracts an HL7 PN (Person Name) from an element.
func parsePN(el *etree.Element) CDAName {
	if el == nil {
		return CDAName{}
	}
	n := CDAName{
		Use: el.SelectAttrValue("use", ""),
	}
	if prefix := el.SelectElement("prefix"); prefix != nil {
		n.Prefix = strings.TrimSpace(prefix.Text())
	}
	for _, given := range el.SelectElements("given") {
		if v := strings.TrimSpace(given.Text()); v != "" {
			n.Given = append(n.Given, v)
		}
	}
	if family := el.SelectElement("family"); family != nil {
		n.Family = strings.TrimSpace(family.Text())
	}
	if suffix := el.SelectElement("suffix"); suffix != nil {
		n.Suffix = strings.TrimSpace(suffix.Text())
	}
	if vt := el.SelectElement("validTime"); vt != nil {
		n.ValidTime = parseIVL_TS(vt)
	}
	return n
}

// parseAD extracts an HL7 AD (Postal Address) from an element.
func parseAD(el *etree.Element) CDAAddress {
	if el == nil {
		return CDAAddress{}
	}
	a := CDAAddress{
		Use:        el.SelectAttrValue("use", ""),
		NullFlavor: el.SelectAttrValue("nullFlavor", ""),
	}
	for _, line := range el.SelectElements("streetAddressLine") {
		if v := strings.TrimSpace(line.Text()); v != "" {
			a.StreetLines = append(a.StreetLines, v)
		}
	}
	if city := el.SelectElement("city"); city != nil {
		a.City = strings.TrimSpace(city.Text())
	}
	if state := el.SelectElement("state"); state != nil {
		a.State = strings.TrimSpace(state.Text())
	}
	if pc := el.SelectElement("postalCode"); pc != nil {
		a.PostalCode = strings.TrimSpace(pc.Text())
	}
	if country := el.SelectElement("country"); country != nil {
		a.Country = strings.TrimSpace(country.Text())
	}
	if county := el.SelectElement("county"); county != nil {
		a.County = strings.TrimSpace(county.Text())
	}
	if vt := el.SelectElement("useablePeriod"); vt != nil {
		a.ValidTime = parseIVL_TS(vt)
	}
	return a
}

// parseTEL extracts an HL7 TEL (Telecommunication Address) from an element.
func parseTEL(el *etree.Element) CDATelecom {
	if el == nil {
		return CDATelecom{}
	}
	t := CDATelecom{
		Use:   el.SelectAttrValue("use", ""),
		Value: el.SelectAttrValue("value", ""),
	}
	switch {
	case strings.HasPrefix(t.Value, "tel:"):
		t.Type = "phone"
	case strings.HasPrefix(t.Value, "fax:"):
		t.Type = "fax"
	case strings.HasPrefix(t.Value, "mailto:"):
		t.Type = "email"
	case strings.HasPrefix(t.Value, "http://"), strings.HasPrefix(t.Value, "https://"):
		t.Type = "url"
	default:
		t.Type = "phone"
	}
	return t
}

// parseIVL_TS extracts an HL7 IVL<TS> (interval of timestamps) from an element.
func parseIVL_TS(el *etree.Element) CDATimeRange { //nolint:revive
	if el == nil {
		return CDATimeRange{}
	}
	r := CDATimeRange{
		NullFlavor: el.SelectAttrValue("nullFlavor", ""),
	}
	if v := el.SelectAttrValue("value", ""); v != "" {
		r.Value = CDATime{Value: v}
	}
	if low := el.SelectElement("low"); low != nil {
		r.Low = parseTS(low)
	}
	if high := el.SelectElement("high"); high != nil {
		r.High = parseTS(high)
	}
	return r
}

// parseValue extracts a polymorphic CDA <value> element.
// The xsi:type attribute determines which HL7 data type applies.
func parseValue(el *etree.Element) *CDAValue {
	if el == nil {
		return nil
	}
	v := &CDAValue{
		NullFlavor: el.SelectAttrValue("nullFlavor", ""),
	}
	// xsi:type attribute is set with namespace prefix in the raw XML; etree stores
	// attribute names as literal strings including the prefix.
	xsiType := el.SelectAttrValue("xsi:type", "")
	if xsiType == "" {
		// Some documents omit the namespace prefix.
		xsiType = el.SelectAttrValue("type", "")
	}
	v.Type = xsiType

	switch {
	case xsiType == "CD" || xsiType == "CE" || xsiType == "CS" || xsiType == "CV":
		c := parseCD(el)
		v.Code = &c
	case xsiType == "PQ":
		q := parsePQ(el)
		v.Quantity = &q
	case xsiType == "ST" || xsiType == "ED":
		v.Text = strings.TrimSpace(el.Text())
	case xsiType == "BL":
		boolStr := el.SelectAttrValue("value", "false")
		b := boolStr == "true"
		v.Boolean = &b
	case xsiType == "INT":
		v.Text = el.SelectAttrValue("value", "")
	case xsiType == "REAL":
		v.Text = el.SelectAttrValue("value", "")
	case xsiType == "TS":
		t := parseTS(el)
		tr := CDATimeRange{Value: t}
		v.TimeRange = &tr
	case xsiType == "IVL_TS":
		tr := parseIVL_TS(el)
		v.TimeRange = &tr
	default:
		// Fallback: try as a coded value, then as a PQ, then as text.
		if code := el.SelectAttrValue("code", ""); code != "" {
			c := parseCD(el)
			v.Code = &c
			if v.Type == "" {
				v.Type = "CD"
			}
		} else if val := el.SelectAttrValue("value", ""); val != "" {
			v.Text = val
		} else {
			v.Text = strings.TrimSpace(el.Text())
		}
	}
	return v
}

// ========================
// Shared composite parsers
// ========================

// parseOrganization extracts a CDAOrganization from an element.
func parseOrganization(el *etree.Element) CDAOrganization {
	if el == nil {
		return CDAOrganization{}
	}
	org := CDAOrganization{}
	for _, idEl := range el.SelectElements("id") {
		org.Ids = append(org.Ids, parseII(idEl))
	}
	for _, nameEl := range el.SelectElements("name") {
		if v := strings.TrimSpace(nameEl.Text()); v != "" {
			org.Names = append(org.Names, v)
		}
	}
	for _, addrEl := range el.SelectElements("addr") {
		org.Addresses = append(org.Addresses, parseAD(addrEl))
	}
	for _, telEl := range el.SelectElements("telecom") {
		org.Telecoms = append(org.Telecoms, parseTEL(telEl))
	}
	return org
}

// parseAssignedEntity extracts a CDAAssignedEntity from an element.
func parseAssignedEntity(el *etree.Element) CDAAssignedEntity {
	if el == nil {
		return CDAAssignedEntity{}
	}
	ae := CDAAssignedEntity{}
	for _, idEl := range el.SelectElements("id") {
		ae.Ids = append(ae.Ids, parseII(idEl))
	}
	if codeEl := el.SelectElement("code"); codeEl != nil {
		ae.Code = parseCD(codeEl)
	}
	for _, addrEl := range el.SelectElements("addr") {
		ae.Addresses = append(ae.Addresses, parseAD(addrEl))
	}
	for _, telEl := range el.SelectElements("telecom") {
		ae.Telecoms = append(ae.Telecoms, parseTEL(telEl))
	}
	if apEl := nav(el, "assignedPerson"); apEl != nil {
		person := CDAPerson{}
		for _, nameEl := range apEl.SelectElements("name") {
			person.Names = append(person.Names, parsePN(nameEl))
		}
		ae.AssignedPerson = &person
	}
	if orgEl := el.SelectElement("representedOrganization"); orgEl != nil {
		org := parseOrganization(orgEl)
		ae.RepresentedOrganization = &org
	}
	return ae
}

// ========================
// NullFlavor awareness
// ========================

// addrNullFlavor returns CDAAddress{NullFlavor: nf} for addresses with null flavor only.
// Used internally; kept here alongside parseAD for co-location.
func addrNullFlavor(el *etree.Element) CDAAddress {
	a := parseAD(el)
	return a // parseAD already captures nullFlavor
}
