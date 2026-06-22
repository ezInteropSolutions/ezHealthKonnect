package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapAllergies converts typed CDA allergy/intolerance section entries to FHIR
// AllergyIntolerance resources. Each outer concern act becomes one resource.
//
// C-CDA structure: outer act → SUBJ entryRelationship → allergy observation
//   - Allergy type from observation value (CD: Drug allergy, Food intolerance)
//   - Substance from CSM participant → playingEntity.code
//   - Reactions from MFST entry relationships
//   - Severity from SUBJ SEV observation nested in the allergy observation
func MapAllergies(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildAllergyResource(i+1, entry, patientRef)
		if len(r) > 2 { // more than just resourceType + id
			resources = append(resources, r)
		}
	}
	return resources
}

func buildAllergyResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "AllergyIntolerance",
		"id":           fmt.Sprintf("allergy-%d", idx),
	}
	if patientRef != "" {
		r["patient"] = ref(patientRef)
	}

	// Clinical status from outer act statusCode
	r["clinicalStatus"] = transforms.AllergyStatusToFHIR(entry.StatusCode)

	// Onset date from outer act effectiveTime.low
	if onset := transforms.CDATimeRangeToOnset(entry.EffectiveTime); onset != "" {
		r["onsetDateTime"] = onset
	}

	// Walk SUBJ entryRelationship to find the allergy observation
	allergyObs := findRelByTypeCode(entry.EntryRelationships, "SUBJ")
	if allergyObs == nil {
		// Flat structure (no concern wrapper) — the entry itself is the observation
		allergyObs = &entry
	}

	// Verification status: refuted when the allergy observation (or the outer
	// concern act) is negated — e.g. a "No Known Allergies" assertion — confirmed
	// per US Core requirement otherwise.
	verificationCode, verificationDisplay := "confirmed", "Confirmed"
	if entry.NegationInd || allergyObs.NegationInd {
		verificationCode, verificationDisplay = "refuted", "Refuted"
	}
	r["verificationStatus"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification",
				"code":    verificationCode,
				"display": verificationDisplay,
			},
		},
	}

	// Allergy type from observation value code
	if allergyObs.Value != nil && allergyObs.Value.Code != nil {
		r["type"] = transforms.AllergyTypeToFHIR(allergyObs.Value.Code.Code)
	}

	// Substance from CSM participant
	for _, p := range allergyObs.Participants {
		if p.TypeCode == "CSM" && p.ParticipantRole.PlayingEntity != nil {
			substCode := p.ParticipantRole.PlayingEntity.Code
			if cc := transforms.CDACodeToCodeableConcept(substCode); cc != nil {
				r["code"] = cc
				r["category"] = transforms.AllergyCategoryFromSubstanceSystem(substCode.CodeSystem)
			}
			break
		}
	}

	// US Core requires AllergyIntolerance.code to be present. When no CSM
	// substance participant carries a usable code — most commonly a "No Known
	// Allergies" assertion, where the substance playingEntity is intentionally
	// nullFlavor="NA" — fall back to the allergy observation's own assertion
	// code/value (e.g. SNOMED 419199007 "Allergy to substance (disorder)") so
	// the resource still carries a meaningful code instead of none at all.
	if _, hasCode := r["code"]; !hasCode && allergyObs.Value != nil && allergyObs.Value.Code != nil {
		if cc := transforms.CDACodeToCodeableConcept(*allergyObs.Value.Code); cc != nil {
			r["code"] = cc
		}
	}

	// Reactions from MFST entry relationships
	var reactions []interface{}
	for _, rel := range allergyObs.EntryRelationships {
		if rel.TypeCode != "MFST" {
			continue
		}
		rxn := map[string]interface{}{}
		if rel.Entry.Value != nil {
			if cc := transforms.CDAValueToFHIR(*rel.Entry.Value); cc != nil {
				rxn["manifestation"] = []interface{}{cc}
			}
		} else if cc := transforms.CDACodeToCodeableConcept(rel.Entry.Code); cc != nil {
			rxn["manifestation"] = []interface{}{cc}
		}
		// Severity from SEV sub-relationship
		sevObs := findRelByTypeCode(rel.Entry.EntryRelationships, "SUBJ")
		if sevObs != nil && sevObs.Code.Code == "SEV" {
			if sevObs.Value != nil && sevObs.Value.Code != nil {
				rxn["severity"] = transforms.AllergyReactionSeverityToFHIR(sevObs.Value.Code.Code)
			}
		}
		if len(rxn) > 0 {
			reactions = append(reactions, rxn)
		}
	}
	if len(reactions) > 0 {
		r["reaction"] = reactions
	}

	return r
}
