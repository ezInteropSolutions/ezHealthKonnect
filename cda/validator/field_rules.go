package validator

import (
	"fmt"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
)

// fieldCheckFunc examines a present section's typed entries and returns FieldReports
// for the section's SHALL and SHOULD fields as defined in the schema.
type fieldCheckFunc func(section *cdadocument.CDASection, schemaDef *cdaSchema.CDASectionDef) []FieldReport

// sectionFieldCheckers maps sectionKey → a dedicated field checker.
// Sections absent from this map fall back to genericFieldChecker.
var sectionFieldCheckers = map[string]fieldCheckFunc{
	"allergiesAndIntolerances": allergyFieldChecker,
	"medications":              medicationFieldChecker,
	"problems":                 problemFieldChecker,
	"vitalSigns":               vitalSignsFieldChecker,
	"results":                  resultsFieldChecker,
}

// resolveFieldChecker returns the registered checker for the given section key,
// or genericFieldChecker when no specific checker is registered.
func resolveFieldChecker(key string) fieldCheckFunc {
	if fn, ok := sectionFieldCheckers[key]; ok {
		return fn
	}
	return genericFieldChecker
}

// genericFieldChecker reports on SHALL/SHOULD entry code and effective-time presence
// for sections that do not have a dedicated checker.
func genericFieldChecker(section *cdadocument.CDASection, schemaDef *cdaSchema.CDASectionDef) []FieldReport {
	if schemaDef == nil {
		return nil
	}
	hasEntries := len(section.Entries) > 0
	summary := fmt.Sprintf("%d entries", len(section.Entries))

	var reports []FieldReport
	for _, f := range schemaDef.Fields {
		if f.Conformance == "MAY" {
			continue
		}
		r := FieldReport{
			FieldPath:   "Entry." + f.Key,
			Conformance: f.Conformance,
		}
		if !hasEntries {
			r.Status = "missing"
			r.ValueSummary = "no entries in section"
		} else {
			r.Status = "populated"
			r.ValueSummary = summary
		}
		reports = append(reports, r)
	}
	return reports
}

// allergyFieldChecker validates the SHALL fields for allergiesAndIntolerances:
//   - medicationAllergyCode → participant playingEntity substance code
//   - status → entry or nested observation statusCode
func allergyFieldChecker(section *cdadocument.CDASection, _ *cdaSchema.CDASectionDef) []FieldReport {
	if len(section.Entries) == 0 {
		return []FieldReport{
			{FieldPath: "Entry.Participants[].ParticipantRole.PlayingEntity.Code", Conformance: "SHALL", Status: "missing", ValueSummary: "no allergy acts"},
			{FieldPath: "Entry.StatusCode", Conformance: "SHALL", Status: "missing", ValueSummary: "no allergy acts"},
		}
	}

	// Substance code from direct participants or nested observation participants.
	substanceCode := ""
outer:
	for _, e := range section.Entries {
		for _, p := range e.Participants {
			if p.ParticipantRole.PlayingEntity != nil && p.ParticipantRole.PlayingEntity.Code.Code != "" {
				substanceCode = p.ParticipantRole.PlayingEntity.Code.Code
				break outer
			}
		}
		for _, rel := range e.EntryRelationships {
			for _, pp := range rel.Entry.Participants {
				if pp.ParticipantRole.PlayingEntity != nil && pp.ParticipantRole.PlayingEntity.Code.Code != "" {
					substanceCode = pp.ParticipantRole.PlayingEntity.Code.Code
					break outer
				}
			}
		}
	}

	substStatus, substSummary := "missing", "no substance code found"
	if substanceCode != "" {
		substStatus = "populated"
		substSummary = "substance: " + substanceCode
	}

	// Status code from entry or nested entryRelationship observation.
	statusCode := ""
	for _, e := range section.Entries {
		if e.StatusCode != "" {
			statusCode = e.StatusCode
			break
		}
		for _, rel := range e.EntryRelationships {
			if rel.Entry.StatusCode != "" {
				statusCode = rel.Entry.StatusCode
				break
			}
		}
	}
	statusStatus := "missing"
	if statusCode != "" {
		statusStatus = "populated"
	}

	return []FieldReport{
		{FieldPath: "Entry.Participants[].ParticipantRole.PlayingEntity.Code", Conformance: "SHALL", Status: substStatus, ValueSummary: substSummary},
		{FieldPath: "Entry.StatusCode", Conformance: "SHALL", Status: statusStatus, ValueSummary: statusCode},
	}
}

// medicationFieldChecker validates medications SHALL fields:
//   - drugCode  → Entry.Consumable.ManufacturedProduct.ManufacturedMaterial.Code
//   - status    → Entry.StatusCode
//   - doseQuantity (SHOULD) → Entry.DoseQuantity
func medicationFieldChecker(section *cdadocument.CDASection, _ *cdaSchema.CDASectionDef) []FieldReport {
	if len(section.Entries) == 0 {
		return []FieldReport{
			{FieldPath: "Entry.Consumable.ManufacturedProduct.ManufacturedMaterial.Code", Conformance: "SHALL", Status: "missing", ValueSummary: "no medication entries"},
			{FieldPath: "Entry.StatusCode", Conformance: "SHALL", Status: "missing", ValueSummary: "no medication entries"},
			{FieldPath: "Entry.DoseQuantity", Conformance: "SHOULD", Status: "missing", ValueSummary: "no medication entries"},
		}
	}

	// Drug code from consumable.
	drugCode := ""
	for _, e := range section.Entries {
		if e.Consumable != nil && e.Consumable.ManufacturedProduct.ManufacturedMaterial != nil {
			if c := e.Consumable.ManufacturedProduct.ManufacturedMaterial.Code.Code; c != "" {
				drugCode = c
				break
			}
		}
	}
	drugStatus, drugSummary := "missing", "no drug code in consumable"
	if drugCode != "" {
		drugStatus = "populated"
		drugSummary = "RxNorm " + drugCode
	}

	// Status code.
	statusCode := ""
	for _, e := range section.Entries {
		if e.StatusCode != "" {
			statusCode = e.StatusCode
			break
		}
	}
	statusStatus := "missing"
	if statusCode != "" {
		statusStatus = "populated"
	}

	// Dose quantity (SHOULD).
	doseStr := ""
	for _, e := range section.Entries {
		if e.DoseQuantity != nil && e.DoseQuantity.Value != "" {
			doseStr = e.DoseQuantity.Value
			if e.DoseQuantity.Unit != "" {
				doseStr += " " + e.DoseQuantity.Unit
			}
			break
		}
	}
	doseStatus := "missing"
	if doseStr != "" {
		doseStatus = "populated"
	}

	return []FieldReport{
		{FieldPath: "Entry.Consumable.ManufacturedProduct.ManufacturedMaterial.Code", Conformance: "SHALL", Status: drugStatus, ValueSummary: drugSummary},
		{FieldPath: "Entry.StatusCode", Conformance: "SHALL", Status: statusStatus, ValueSummary: statusCode},
		{FieldPath: "Entry.DoseQuantity", Conformance: "SHOULD", Status: doseStatus, ValueSummary: doseStr},
	}
}

// problemFieldChecker validates problems/conditions SHALL fields:
//   - conditionCode → Entry.Code or nested observation code
func problemFieldChecker(section *cdadocument.CDASection, _ *cdaSchema.CDASectionDef) []FieldReport {
	if len(section.Entries) == 0 {
		return []FieldReport{
			{FieldPath: "Entry.Code", Conformance: "SHALL", Status: "missing", ValueSummary: "no problem entries"},
		}
	}

	condCode := ""
	for _, e := range section.Entries {
		if e.Code.Code != "" {
			condCode = e.Code.Code
			break
		}
		for _, rel := range e.EntryRelationships {
			if rel.Entry.Code.Code != "" {
				condCode = rel.Entry.Code.Code
				break
			}
		}
		if condCode != "" {
			break
		}
	}
	codeStatus := "missing"
	if condCode != "" {
		codeStatus = "populated"
	}
	return []FieldReport{
		{FieldPath: "Entry.Code", Conformance: "SHALL", Status: codeStatus, ValueSummary: condCode},
	}
}

// vitalSignsFieldChecker validates vital signs SHALL fields from organizer components:
//   - component code   → Entry.Components[].Code
//   - component value  → Entry.Components[].Value (PQ)
func vitalSignsFieldChecker(section *cdadocument.CDASection, _ *cdaSchema.CDASectionDef) []FieldReport {
	if len(section.Entries) == 0 {
		return []FieldReport{
			{FieldPath: "Entry.Components[].Code", Conformance: "SHALL", Status: "missing", ValueSummary: "no vital sign organizers"},
			{FieldPath: "Entry.Components[].Value", Conformance: "SHALL", Status: "missing", ValueSummary: "no vital sign organizers"},
		}
	}

	hasCode, hasValue := false, false
	for _, e := range section.Entries {
		if e.Code.Code != "" {
			hasCode = true
		}
		for _, comp := range e.Components {
			if comp.Code.Code != "" {
				hasCode = true
			}
			if comp.Value != nil {
				if comp.Value.Text != "" || (comp.Value.Quantity != nil && comp.Value.Quantity.Value != "") {
					hasValue = true
				}
			}
		}
	}

	summary := fmt.Sprintf("%d organizer(s)", len(section.Entries))
	codeStatus := "missing"
	if hasCode {
		codeStatus = "populated"
	}
	valStatus := "missing"
	if hasValue {
		valStatus = "populated"
	}
	return []FieldReport{
		{FieldPath: "Entry.Components[].Code", Conformance: "SHALL", Status: codeStatus, ValueSummary: summary},
		{FieldPath: "Entry.Components[].Value", Conformance: "SHALL", Status: valStatus, ValueSummary: summary},
	}
}

// resultsFieldChecker validates lab results SHALL fields:
//   - testCode → Entry.Code or component code
func resultsFieldChecker(section *cdadocument.CDASection, _ *cdaSchema.CDASectionDef) []FieldReport {
	if len(section.Entries) == 0 {
		return []FieldReport{
			{FieldPath: "Entry.Code", Conformance: "SHALL", Status: "missing", ValueSummary: "no result entries"},
		}
	}

	hasCode := false
	for _, e := range section.Entries {
		if e.Code.Code != "" {
			hasCode = true
			break
		}
		for _, comp := range e.Components {
			if comp.Code.Code != "" {
				hasCode = true
				break
			}
		}
		if hasCode {
			break
		}
	}

	codeStatus := "missing"
	if hasCode {
		codeStatus = "populated"
	}
	return []FieldReport{
		{FieldPath: "Entry.Code", Conformance: "SHALL", Status: codeStatus, ValueSummary: fmt.Sprintf("%d result entries", len(section.Entries))},
	}
}
