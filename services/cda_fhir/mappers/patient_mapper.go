// Package mappers contains per-section typed CDA→FHIR resource builders.
// Every function accepts typed cda/document structs; no XPath, no etree, no map inputs.
package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapPatient converts a typed CDAPatient to a FHIR Patient resource.
//
// ALL identifiers, names, addresses, and telecoms are mapped — never just the first one.
// Race and ethnicity extensions require a separate profile-injection step in document_mapper.go.
func MapPatient(p cdadocument.CDAPatient) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-1",
	}

	// ── Identifiers: ALL <id> elements ──────────────────────────────────────
	if len(p.Ids) > 0 {
		var identifiers []interface{}
		for _, id := range p.Ids {
			if ident := transforms.IIToIdentifier(id); ident != nil {
				identifiers = append(identifiers, ident)
			}
		}
		if len(identifiers) > 0 {
			r["identifier"] = identifiers
		}
	}

	// ── Names: ALL <name> elements (official first, then aliases) ───────────
	if len(p.Names) > 0 {
		// Stable sort: use="L" (legal/official) first
		ordered := sortNamesByUse(p.Names)
		var names []interface{}
		for _, n := range ordered {
			if hn := transforms.CDANameToFHIR(n); hn != nil {
				names = append(names, hn)
			}
		}
		if len(names) > 0 {
			r["name"] = names
		}
	}

	// ── Addresses: ALL <addr> elements ──────────────────────────────────────
	if len(p.Addresses) > 0 {
		var addresses []interface{}
		for _, a := range p.Addresses {
			if addr := transforms.CDAAddressToFHIR(a); addr != nil {
				addresses = append(addresses, addr)
			}
		}
		if len(addresses) > 0 {
			r["address"] = addresses
		}
	}

	// ── Telecoms: ALL <telecom> elements ────────────────────────────────────
	if len(p.Telecoms) > 0 {
		var telecoms []interface{}
		for _, t := range p.Telecoms {
			if cp := transforms.CDATelecomToFHIR(t); cp != nil {
				telecoms = append(telecoms, cp)
			}
		}
		if len(telecoms) > 0 {
			r["telecom"] = telecoms
		}
	}

	// ── Demographics ────────────────────────────────────────────────────────
	if dob := transforms.CDATimeToFHIRDate(p.BirthDate); dob != "" {
		r["birthDate"] = dob
	}
	if p.Gender.Code != "" || p.Gender.NullFlavor == "" {
		if gender := transforms.GenderToFHIR(p.Gender.Code); gender != "" {
			r["gender"] = gender
		}
	}
	if p.MaritalStatus.Code != "" {
		if cc := transforms.CDACodeToCodeableConcept(p.MaritalStatus); cc != nil {
			r["maritalStatus"] = cc
		}
	}

	// ── Language preferences ────────────────────────────────────────────────
	if len(p.Languages) > 0 {
		var comms []interface{}
		for _, lang := range p.Languages {
			if lang.Code == "" {
				continue
			}
			comm := map[string]interface{}{
				"language": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"system": "urn:ietf:bcp:47",
							"code":   lang.Code,
						},
					},
				},
			}
			if lang.PreferenceInd {
				comm["preferred"] = true
			}
			comms = append(comms, comm)
		}
		if len(comms) > 0 {
			r["communication"] = comms
		}
	}

	return r
}

// MapAuthor converts the first document author's assigned person to a FHIR Practitioner.
// Returns nil when no usable author data is present.
func MapAuthor(authors []cdadocument.CDAAuthor) map[string]interface{} {
	for i, a := range authors {
		if a.AssignedAuthor.AssignedPerson == nil {
			continue
		}
		person := a.AssignedAuthor.AssignedPerson
		if len(person.Names) == 0 {
			continue
		}

		p := map[string]interface{}{
			"resourceType": "Practitioner",
			"id":           fmt.Sprintf("author-%d", i+1),
		}

		var names []interface{}
		for _, n := range person.Names {
			if hn := transforms.CDANameToFHIR(n); hn != nil {
				names = append(names, hn)
			}
		}
		if len(names) > 0 {
			p["name"] = names
		}

		// Identifiers (NPI, etc.)
		if len(a.AssignedAuthor.Ids) > 0 {
			var idents []interface{}
			for _, id := range a.AssignedAuthor.Ids {
				if ident := transforms.IIToIdentifier(id); ident != nil {
					idents = append(idents, ident)
				}
			}
			if len(idents) > 0 {
				p["identifier"] = idents
			}
		}

		// Telecoms
		if len(a.AssignedAuthor.Telecoms) > 0 {
			var telecoms []interface{}
			for _, t := range a.AssignedAuthor.Telecoms {
				if cp := transforms.CDATelecomToFHIR(t); cp != nil {
					telecoms = append(telecoms, cp)
				}
			}
			if len(telecoms) > 0 {
				p["telecom"] = telecoms
			}
		}

		return p // return on first author with a person
	}
	return nil
}

// MapCustodian converts the document custodian organization to a FHIR Organization.
// Returns nil when no custodian name is present.
func MapCustodian(custodian cdadocument.CDACustodian) map[string]interface{} {
	org := custodian.AssignedCustodian.RepresentedCustodianOrganization
	if len(org.Names) == 0 {
		return nil
	}
	r := map[string]interface{}{
		"resourceType": "Organization",
		"id":           "custodian-1",
		"name":         org.Names[0],
		// us-core-organization requires `active`; the custodian of a document
		// we just received is, by definition, an active organization.
		"active": true,
	}
	if len(org.Ids) > 0 {
		var idents []interface{}
		for _, id := range org.Ids {
			if ident := transforms.IIToIdentifier(id); ident != nil {
				idents = append(idents, ident)
			}
		}
		if len(idents) > 0 {
			r["identifier"] = idents
		}
	}
	if len(org.Addresses) > 0 {
		var addresses []interface{}
		for _, a := range org.Addresses {
			if addr := transforms.CDAAddressToFHIR(a); addr != nil {
				addresses = append(addresses, addr)
			}
		}
		if len(addresses) > 0 {
			r["address"] = addresses
		}
	}
	return r
}

// ============================================================
// private helpers
// ============================================================

// sortNamesByUse returns a copy of names with use="L" (legal) first,
// other uses following in original order. Stable for deterministic FHIR output.
func sortNamesByUse(names []cdadocument.CDAName) []cdadocument.CDAName {
	out := make([]cdadocument.CDAName, 0, len(names))
	// legal names first
	for _, n := range names {
		if n.Use == "L" || n.Use == "" {
			out = append(out, n)
		}
	}
	// then everything else
	for _, n := range names {
		if n.Use != "L" && n.Use != "" {
			out = append(out, n)
		}
	}
	return out
}
