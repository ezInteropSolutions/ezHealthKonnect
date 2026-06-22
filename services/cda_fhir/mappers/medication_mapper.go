package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapMedications converts typed CDA medication section entries to FHIR resources.
//
// CDA moodCode dispatch:
//   - "INT" (intent/prescription) → MedicationRequest  (intent: "order")
//   - "EVN" (event/administered) or default → MedicationStatement
//
// C-CDA structure: substanceAdministration entry
//   - Drug code from consumable.manufacturedProduct.manufacturedMaterial.code
//   - Status from substanceAdministration statusCode
//   - Effective period from effectiveTime
//   - Route from routeCode
//   - Dose from doseQuantity
func MapMedications(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		var r map[string]interface{}
		if entry.MoodCode == "INT" {
			r = buildMedicationRequestResource(i+1, entry, patientRef)
		} else {
			r = buildMedicationStatementResource(i+1, entry, patientRef)
		}
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

// buildMedicationRequestResource maps a CDA substanceAdministration with moodCode="INT"
// (intent / prescription order) to a FHIR R4 MedicationRequest resource.
func buildMedicationRequestResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "MedicationRequest",
		"id":           fmt.Sprintf("medication-%d", idx),
		"intent":       "order",
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.MedicationRequestStatusToFHIR(entry.StatusCode)

	// Requester — US Core (us-core-21) requires this whenever intent="order".
	r["requester"] = requesterReference(entry)

	// Medication code from consumable
	if entry.Consumable != nil {
		mp := entry.Consumable.ManufacturedProduct
		if mp.ManufacturedMaterial != nil {
			if cc := transforms.CDACodeToCodeableConcept(mp.ManufacturedMaterial.Code); cc != nil {
				r["medicationCodeableConcept"] = cc
			}
		}
	}

	// Authored on / effective period
	if onset := transforms.CDATimeRangeToOnset(entry.EffectiveTime); onset != "" {
		r["authoredOn"] = onset
	}

	// Dosage instruction
	dosage := map[string]interface{}{}
	if entry.RouteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.RouteCode); cc != nil {
			dosage["route"] = cc
		}
	}
	if entry.DoseQuantity != nil {
		if qty := transforms.CDAQuantityToFHIR(*entry.DoseQuantity); qty != nil {
			dosage["doseAndRate"] = []interface{}{
				map[string]interface{}{"doseQuantity": qty},
			}
		}
	}
	applyDosageTiming(entry, dosage)
	applyDosageText(entry, dosage)
	if len(dosage) > 0 {
		r["dosageInstruction"] = []interface{}{dosage}
	}

	if reasons := indicationReasonCodes(entry); len(reasons) > 0 {
		r["reasonCode"] = reasons
	}

	return r
}

// buildMedicationStatementResource maps a CDA substanceAdministration with moodCode="EVN"
// (event / historically administered medication) to a FHIR R4 MedicationStatement resource.
func buildMedicationStatementResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "MedicationStatement",
		"id":           fmt.Sprintf("medication-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = transforms.MedicationStatusToFHIR(entry.StatusCode)

	// Medication code from consumable
	if entry.Consumable != nil {
		mp := entry.Consumable.ManufacturedProduct
		if mp.ManufacturedMaterial != nil {
			if cc := transforms.CDACodeToCodeableConcept(mp.ManufacturedMaterial.Code); cc != nil {
				r["medicationCodeableConcept"] = cc
			}
		}
	}

	// Effective period
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["effectivePeriod"] = period
	} else if onset := transforms.CDATimeRangeToOnset(entry.EffectiveTime); onset != "" {
		r["effectiveDateTime"] = onset
	}

	// Dosage
	dosage := map[string]interface{}{}
	if entry.RouteCode != nil {
		if cc := transforms.CDACodeToCodeableConcept(*entry.RouteCode); cc != nil {
			dosage["route"] = cc
		}
	}
	if entry.DoseQuantity != nil {
		if qty := transforms.CDAQuantityToFHIR(*entry.DoseQuantity); qty != nil {
			dosage["doseAndRate"] = []interface{}{
				map[string]interface{}{"doseQuantity": qty},
			}
		}
	}
	applyDosageTiming(entry, dosage)
	applyDosageText(entry, dosage)
	if len(dosage) > 0 {
		r["dosage"] = []interface{}{dosage}
	}

	if reasons := indicationReasonCodes(entry); len(reasons) > 0 {
		r["reasonCode"] = reasons
	}

	return r
}

// applyDosageTiming sets dosage["timing"]["repeat"]["frequency"/"period"/"periodUnit"]
// from the entry's PIVL_TS/EIVL_TS effectiveTime (C-CDA's dosing-frequency idiom,
// e.g. "every 12 hours") — additive alongside any existing "boundsPeriod" already
// set from the primary IVL_TS effectiveTime.
func applyDosageTiming(entry cdadocument.CDAEntry, dosage map[string]interface{}) {
	for _, et := range entry.EffectiveTimes {
		if (et.XSIType != "PIVL_TS" && et.XSIType != "EIVL_TS") || et.Period == nil {
			continue
		}
		timing, ok := dosage["timing"].(map[string]interface{})
		if !ok {
			timing = map[string]interface{}{}
		}
		repeat, ok := timing["repeat"].(map[string]interface{})
		if !ok {
			repeat = map[string]interface{}{}
		}
		repeat["frequency"] = 1
		if et.Period.Value != "" {
			repeat["period"] = et.Period.Value
		}
		if et.Period.Unit != "" {
			repeat["periodUnit"] = et.Period.Unit
		}
		timing["repeat"] = repeat
		dosage["timing"] = timing
		return
	}
}

// medicationFreeTextSigTemplateID and instructionV2TemplateID discriminate the
// two distinct C-CDA templates that carry medication instruction text via an
// entryRelationship's nested entry.Text (resolved against section narrative
// by resolveEntryRefs) — see IG p.590-593 (Instruction (V2)) and p.624-625
// (Medication Free Text Sig). Both use the identical
// <text><reference value="#id"/></text> shape; templateId is the only
// reliable discriminator between them.
const (
	medicationFreeTextSigTemplateID = "2.16.840.1.113883.10.20.22.4.147"
	instructionV2TemplateID         = "2.16.840.1.113883.10.20.22.4.20"
)

func hasTemplateID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// applyDosageText sets dosage["text"] from a nested Medication Free Text Sig
// (entryRelationship[@typeCode='COMP']) and dosage["patientInstruction"] from
// a nested Instruction (V2) act (entryRelationship[@typeCode='SUBJ',
// @inversionInd='true']) — both are additive alongside any structured
// route/dose/timing already set on dosage.
func applyDosageText(entry cdadocument.CDAEntry, dosage map[string]interface{}) {
	for _, rel := range entry.EntryRelationships {
		nested := rel.Entry
		if nested.Text == "" {
			continue
		}
		switch {
		case rel.TypeCode == "COMP" && hasTemplateID(nested.TemplateIds, medicationFreeTextSigTemplateID):
			dosage["text"] = nested.Text
		case rel.TypeCode == "SUBJ" && rel.InversionInd && hasTemplateID(nested.TemplateIds, instructionV2TemplateID):
			dosage["patientInstruction"] = nested.Text
		}
	}
}

// indicationReasonCodes converts every entryRelationship[@typeCode='RSON']
// (Indication (V2) observation) into a FHIR reasonCode CodeableConcept.
// Unlike most relationship lookups in this package, RSON can repeat (multiple
// indications for one medication), so this collects all matches rather than
// using findRelByTypeCode (first-match only).
func indicationReasonCodes(entry cdadocument.CDAEntry) []interface{} {
	var reasons []interface{}
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode != "RSON" {
			continue
		}
		indication := rel.Entry
		var cc map[string]interface{}
		if indication.Value != nil {
			if v, ok := transforms.CDAValueToFHIR(*indication.Value).(map[string]interface{}); ok {
				cc = v
			}
		}
		if cc == nil {
			cc = transforms.CDACodeToCodeableConcept(indication.Code)
		}
		if cc != nil {
			reasons = append(reasons, cc)
		}
	}
	return reasons
}

// requesterReference builds the FHIR Reference for MedicationRequest.requester
// (required by US Core's us-core-21 constraint whenever intent="order") from
// the substanceAdministration entry's own <performer>/<author> data — prefer
// the performer (who carried out/ordered the act), falling back to the entry's
// author. Mirrors the existing inline display-only Reference convention used
// for Encounter.participant (see buildEncounterParticipant in encounter_mapper.go)
// rather than minting a separate Practitioner resource. Always returns a
// non-empty Reference so the SHALL constraint is met even when the source
// document carries no name for either.
func requesterReference(entry cdadocument.CDAEntry) map[string]interface{} {
	for _, p := range entry.Performers {
		if p.AssignedEntity.AssignedPerson == nil || len(p.AssignedEntity.AssignedPerson.Names) == 0 {
			continue
		}
		if display := buildDisplayFromName(p.AssignedEntity.AssignedPerson.Names[0]); display != "" {
			return map[string]interface{}{"display": display}
		}
	}
	for _, a := range entry.Authors {
		if a.AssignedAuthor.AssignedPerson == nil || len(a.AssignedAuthor.AssignedPerson.Names) == 0 {
			continue
		}
		if display := buildDisplayFromName(a.AssignedAuthor.AssignedPerson.Names[0]); display != "" {
			return map[string]interface{}{"display": display}
		}
	}
	return map[string]interface{}{"display": "Ordering Provider"}
}
