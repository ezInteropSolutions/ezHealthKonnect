package rules

import (
	"fmt"

	"ezhealthkonnect/services/cda_fhir/assembly"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
)

const (
	bpSystolicLOINC  = "8480-6"
	bpDiastolicLOINC = "8462-4"
	bpPanelLOINC     = "85354-9"
	loincSystem      = "http://loinc.org"
	bpPanelText      = "Blood pressure panel"
)

// BPPanelSynthesisRule detects standalone Systolic (LOINC 8480-6) and Diastolic
// (LOINC 8462-4) Observation resources that were produced as independent resources
// (not caught by extractBloodPressurePanels in observation_mapper.go), and synthesizes
// a Blood Pressure Panel Observation (LOINC 85354-9) grouping them as components
// per the FHIR "bp" / US Core vital-signs profile.
//
// The rule pairs Systolic+Diastolic observations that share the same subject
// reference (Patient) and the nearest effectiveDateTime. Each pair is replaced by
// one panel Observation; the two component observations are added to ctx.Removed
// and redirected to the panel.
type BPPanelSynthesisRule struct{}

// NewBPPanelSynthesisRule returns a ready-to-register BPPanelSynthesisRule.
func NewBPPanelSynthesisRule() *BPPanelSynthesisRule { return &BPPanelSynthesisRule{} }

// Name implements AssemblyRule.
func (r *BPPanelSynthesisRule) Name() string { return "bp_panel_synthesis" }

// Apply finds Systolic/Diastolic observation pairs and synthesises BP panel resources.
func (r *BPPanelSynthesisRule) Apply(ctx *assembly.AssemblyContext) error {
	// Collect candidate observations that are not already removed.
	type bpCandidate struct {
		idx      int
		resource map[string]interface{}
		loincCode string
		subject   string
		effective string
	}

	var candidates []bpCandidate
	for i, res := range ctx.Resources {
		if res["resourceType"] != "Observation" {
			continue
		}
		id, _ := res["id"].(string)
		if ctx.Removed[fmt.Sprintf("Observation/%s", id)] {
			continue
		}
		code := firstCodingCode(res)
		if code != bpSystolicLOINC && code != bpDiastolicLOINC {
			continue
		}
		subject := subjectRef(res)
		effective, _ := res["effectiveDateTime"].(string)
		candidates = append(candidates, bpCandidate{
			idx:       i,
			resource:  res,
			loincCode: code,
			subject:   subject,
			effective: effective,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Pair Systolic with Diastolic by subject (and prefer matching effectiveDateTime).
	paired := make(map[int]bool) // candidate indices already consumed
	panelIdx := 1

	for i, sys := range candidates {
		if sys.loincCode != bpSystolicLOINC || paired[i] {
			continue
		}
		for j, dia := range candidates {
			if dia.loincCode != bpDiastolicLOINC || paired[j] {
				continue
			}
			if sys.subject != dia.subject {
				continue
			}

			// Found a pair.
			paired[i] = true
			paired[j] = true

			effective := sys.effective
			if effective == "" {
				effective = dia.effective
			}

			sysID, _ := sys.resource["id"].(string)
			diaID, _ := dia.resource["id"].(string)
			panelID := fmt.Sprintf("bp-panel-%d", panelIdx)
			panelIdx++

			panel := buildBPPanel(panelID, sys.resource, dia.resource, sys.subject, effective)
			ctx.Resources = append(ctx.Resources, panel)

			ctx.Removed[fmt.Sprintf("Observation/%s", sysID)] = true
			ctx.Removed[fmt.Sprintf("Observation/%s", diaID)] = true
			ctx.DedupRedirects[fmt.Sprintf("Observation/%s", sysID)] = fmt.Sprintf("Observation/%s", panelID)
			ctx.DedupRedirects[fmt.Sprintf("Observation/%s", diaID)] = fmt.Sprintf("Observation/%s", panelID)

			if ctx.Log != nil {
				ev := mappinglog.AssemblyEvent{
					Rule:         r.Name(),
					Action:       "synthesized",
					ResourceType: "Observation",
					IDs:          []string{sysID, diaID, panelID},
					SurvivorID:   panelID,
					Detail:       "synthesized BP panel 85354-9 from 8480-6 + 8462-4",
				}
				if ctx.Config.DeepLineage {
					ev.Lineage = map[string]mappinglog.ResourceLineage{}
					if l, ok := assembly.ExtractLineage(sys.resource); ok {
						ev.Lineage[sysID] = l
					}
					if l, ok := assembly.ExtractLineage(dia.resource); ok {
						ev.Lineage[diaID] = l
					}
					// panelID has no CDA lineage of its own — it is synthesized,
					// not CDA-sourced — so it is intentionally absent from Lineage.
				}
				ctx.Log.AddAssemblyEvent(ev)
			}
			break
		}
	}
	return nil
}

// buildBPPanel assembles the Blood Pressure Panel Observation resource.
func buildBPPanel(id string, systolic, diastolic map[string]interface{}, subjectRef, effective string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Observation",
		"id":           id,
		"status":       observationStatus(systolic),
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "vital-signs",
						"display": "Vital Signs",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": loincSystem,
					"code":   bpPanelLOINC,
				},
			},
			"text": bpPanelText,
		},
	}
	if subjectRef != "" {
		r["subject"] = map[string]interface{}{"reference": subjectRef}
	}
	if effective != "" {
		r["effectiveDateTime"] = effective
	}
	// Systolic and Diastolic share the same /author in C-CDA (both
	// components of one Vital Signs Organizer), so either source carries
	// the same performer. Previously dropped entirely: extractComponent
	// only ever copies code+value[x], and nothing else in this function set
	// performer on the synthesized panel -- real data loss found auditing
	// the 99397 sample's Vital Signs section, where both components had an
	// identical, non-trivial performer reference that vanished once the
	// two were merged into the 85354-9 panel.
	if perf, ok := systolic["performer"]; ok {
		r["performer"] = perf
	} else if perf, ok := diastolic["performer"]; ok {
		r["performer"] = perf
	}

	var components []interface{}
	if comp := extractComponent(systolic); comp != nil {
		components = append(components, comp)
	}
	if comp := extractComponent(diastolic); comp != nil {
		components = append(components, comp)
	}
	if len(components) > 0 {
		r["component"] = components
	}
	return r
}

// extractComponent converts a standalone Observation into a BP panel component.
func extractComponent(obs map[string]interface{}) map[string]interface{} {
	comp := map[string]interface{}{}
	if code, ok := obs["code"]; ok {
		comp["code"] = code
	}
	// Copy whichever value[x] key is present.
	for _, key := range []string{
		"valueQuantity", "valueCodeableConcept", "valueString",
		"valueInteger", "valueBoolean", "valuePeriod",
	} {
		if v, ok := obs[key]; ok {
			comp[key] = v
			break
		}
	}
	if len(comp) == 0 {
		return nil
	}
	return comp
}

// firstCodingCode extracts the code from the first Coding in Observation.code.coding.
func firstCodingCode(obs map[string]interface{}) string {
	code, ok := obs["code"].(map[string]interface{})
	if !ok {
		return ""
	}
	codings, ok := code["coding"].([]interface{})
	if !ok || len(codings) == 0 {
		return ""
	}
	c, ok := codings[0].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := c["code"].(string)
	return v
}

// subjectRef extracts the reference string from Observation.subject.reference.
func subjectRef(obs map[string]interface{}) string {
	subj, ok := obs["subject"].(map[string]interface{})
	if !ok {
		return ""
	}
	ref, _ := subj["reference"].(string)
	return ref
}

// observationStatus returns the status from an Observation resource (default "final").
func observationStatus(obs map[string]interface{}) string {
	if s, ok := obs["status"].(string); ok && s != "" {
		return s
	}
	return "final"
}
