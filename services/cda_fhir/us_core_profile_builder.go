// services/cda_fhir/us_core_profile_builder.go
// USCoreProfileBuilder injects US Core R4 6.1.0 meta.profile URLs and Patient
// demographic extensions (race, ethnicity, birthsex) into mapped FHIR resources.
//
// Called after createFHIRResourceFromSection() for every resource produced by
// GenericCDAFHIRMapper.  Stateless; safe for concurrent use.

package cdafhir

// =========================================================
// Profile URL constants (US Core R4 6.1.0)
// =========================================================

const (
	usCoreBase = "http://hl7.org/fhir/us/core/StructureDefinition/"

	profilePatient           = usCoreBase + "us-core-patient"
	profileAllergyIntolerance = usCoreBase + "us-core-allergyintolerance"
	profileCondition         = usCoreBase + "us-core-condition-problems-health-concerns"
	profileEncounterDiagnosis  = usCoreBase + "us-core-condition-encounter-diagnosis"
	profileObservation         = usCoreBase + "us-core-observation-clinical-result"
	profileObservationVitals = usCoreBase + "us-core-vital-signs"
	profileObservationLab    = usCoreBase + "us-core-observation-lab"
	profileImmunization      = usCoreBase + "us-core-immunization"
	profileProcedure         = usCoreBase + "us-core-procedure"
	profileEncounter         = usCoreBase + "us-core-encounter"
	profilePractitioner      = usCoreBase + "us-core-practitioner"
	profileOrganization      = usCoreBase + "us-core-organization"
	profileCoverage          = usCoreBase + "us-core-coverage"
	profileGoal              = usCoreBase + "us-core-goal"
	profileFamilyMemberHistory = usCoreBase + "us-core-familymemberhistory"
	profileCarePlan          = usCoreBase + "us-core-careplan"
	profileServiceRequest    = usCoreBase + "us-core-servicerequest"
	profileMedicationRequest = usCoreBase + "us-core-medicationrequest"
	profileDocumentReference = usCoreBase + "us-core-documentreference"

	// US Core extension URIs
	extRace      = usCoreBase + "us-core-race"
	extEthnicity = usCoreBase + "us-core-ethnicity"
	extBirthSex  = usCoreBase + "us-core-birthsex"

	// OMB race/ethnicity code system OID
	ombRaceEthnicitySys = "urn:oid:2.16.840.1.113883.6.238"
)

// resourceTypeToProfile maps a FHIR resource type to its primary US Core R4 profile.
// The template-level useCoreProfile field overrides this default.
var resourceTypeToProfile = map[string]string{
	"Patient":             profilePatient,
	"AllergyIntolerance":  profileAllergyIntolerance,
	"Condition":           profileCondition,
	// MedicationStatement has no US Core 8.x profile — it was removed in US Core 5+.
	// Use the base FHIR resource without a profile declaration.
	"MedicationRequest":   profileMedicationRequest,
	"Observation":         profileObservation,
	"Immunization":        profileImmunization,
	"Procedure":           profileProcedure,
	"Encounter":           profileEncounter,
	"Practitioner":        profilePractitioner,
	"Organization":        profileOrganization,
	"Coverage":            profileCoverage,
	"Goal":                profileGoal,
	"FamilyMemberHistory": profileFamilyMemberHistory,
	"CarePlan":            profileCarePlan,
	"ServiceRequest":      profileServiceRequest,
}

// =========================================================
// USCoreProfileBuilder
// =========================================================

// USCoreProfileBuilder injects US Core profile metadata into FHIR resources.
type USCoreProfileBuilder struct{}

// NewUSCoreProfileBuilder returns a ready-to-use builder.
func NewUSCoreProfileBuilder() *USCoreProfileBuilder {
	return &USCoreProfileBuilder{}
}

// InjectProfiles sets meta.profile on each resource and adds Patient demographic
// extensions from the parsedCDA header when present.
//
// templateProfile is the section-level useCoreProfile override from the template;
// it takes precedence over the resourceTypeToProfile default.
//
// patientHeader is the parsedCDA["header"]["patient"] map, needed for race/ethnicity
// extensions on Patient resources. Pass nil when not available.
func (b *USCoreProfileBuilder) InjectProfiles(
	resources []map[string]interface{},
	templateProfile string,
	patientHeader map[string]interface{},
) {
	for _, r := range resources {
		b.injectProfile(r, templateProfile)
		if strField(r, "resourceType") == "Patient" {
			b.injectPatientExtensions(r, patientHeader)
		}
	}
}

// InjectProfile sets meta.profile on a single resource.
func (b *USCoreProfileBuilder) InjectProfile(resource map[string]interface{}, templateProfile string) {
	b.injectProfile(resource, templateProfile)
}

func (b *USCoreProfileBuilder) injectProfile(r map[string]interface{}, templateProfile string) {
	profile := templateProfile
	if profile == "" {
		rt := strField(r, "resourceType")
		profile = resourceTypeToProfile[rt]
	}
	if profile == "" {
		return
	}

	meta, ok := r["meta"].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{}
	}

	// Merge: preserve any existing profiles, add ours if not already present.
	existing, _ := meta["profile"].([]interface{})
	for _, p := range existing {
		if s, ok := p.(string); ok && s == profile {
			// Already present — nothing to do.
			return
		}
	}
	meta["profile"] = append(existing, profile)
	r["meta"] = meta
}

// injectPatientExtensions adds US Core race, ethnicity, and birthsex extensions
// to a Patient resource from the CDA document header demographics.
func (b *USCoreProfileBuilder) injectPatientExtensions(
	patient map[string]interface{},
	header map[string]interface{},
) {
	if header == nil {
		return
	}

	existing, _ := patient["extension"].([]interface{})
	var newExts []interface{}

	// Race extension
	if race := strField(header, "race"); race != "" {
		raceDisplay := strField(header, "raceDisplay")
		if raceDisplay == "" {
			raceDisplay = race
		}
		raceExt := buildRaceExtension(race, raceDisplay)
		if !hasExtension(existing, extRace) {
			newExts = append(newExts, raceExt)
		}
	}

	// Ethnicity extension
	if eth := strField(header, "ethnicity"); eth != "" {
		ethDisplay := strField(header, "ethnicityDisplay")
		if ethDisplay == "" {
			ethDisplay = eth
		}
		ethExt := buildEthnicityExtension(eth, ethDisplay)
		if !hasExtension(existing, extEthnicity) {
			newExts = append(newExts, ethExt)
		}
	}

	// Birth sex extension (administrative gender at birth, distinct from gender identity)
	if birthSex := strField(header, "birthSex"); birthSex != "" {
		if !hasExtension(existing, extBirthSex) {
			newExts = append(newExts, map[string]interface{}{
				"url":         extBirthSex,
				"valueCode":   birthSex,
			})
		}
	}

	if len(newExts) > 0 {
		patient["extension"] = append(existing, newExts...)
	}
}

// =========================================================
// Extension builders
// =========================================================

func buildRaceExtension(code, display string) map[string]interface{} {
	return map[string]interface{}{
		"url": extRace,
		"extension": []interface{}{
			map[string]interface{}{
				"url": "ombCategory",
				"valueCoding": NewCoding(code, display, ombRaceEthnicitySys),
			},
			map[string]interface{}{
				"url":         "text",
				"valueString": display,
			},
		},
	}
}

func buildEthnicityExtension(code, display string) map[string]interface{} {
	return map[string]interface{}{
		"url": extEthnicity,
		"extension": []interface{}{
			map[string]interface{}{
				"url": "ombCategory",
				"valueCoding": NewCoding(code, display, ombRaceEthnicitySys),
			},
			map[string]interface{}{
				"url":         "text",
				"valueString": display,
			},
		},
	}
}

// hasExtension returns true when any entry in exts has the given URL.
func hasExtension(exts []interface{}, url string) bool {
	for _, raw := range exts {
		if m, ok := raw.(map[string]interface{}); ok {
			if u, ok := m["url"].(string); ok && u == url {
				return true
			}
		}
	}
	return false
}

