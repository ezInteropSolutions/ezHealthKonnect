package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapDeviceUseStatements converts typed CDA medical equipment section entries to FHIR
// DeviceUseStatement resources.
//
// C-CDA structure: supply entry
//   - Device code from entry.Code or participant CSM playingDevice code
//   - Status from entry.StatusCode
//   - Timing from entry.EffectiveTime
func MapDeviceUseStatements(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for i, entry := range entries {
		r := buildDeviceUseResource(i+1, entry, patientRef)
		if len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

func buildDeviceUseResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "DeviceUseStatement",
		"id":           fmt.Sprintf("device-%d", idx),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}

	r["status"] = deviceStatus(entry.StatusCode)

	// Device code from CSM participant playingDevice or direct entry code
	deviceCode := entry.Code
	for _, p := range entry.Participants {
		if p.TypeCode == "DEV" || p.TypeCode == "CSM" {
			if p.ParticipantRole.PlayingEntity != nil {
				deviceCode = p.ParticipantRole.PlayingEntity.Code
			}
			break
		}
	}
	if cc := transforms.CDACodeToCodeableConcept(deviceCode); cc != nil {
		r["device"] = map[string]interface{}{"codeableConcept": cc}
	}

	// Timing
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["timingPeriod"] = period
	} else if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["timingDateTime"] = dt
	}

	return r
}

func deviceStatus(statusCode string) string {
	switch statusCode {
	case "active":
		return "active"
	case "completed":
		return "completed"
	default:
		return "active"
	}
}
