package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"ezhealthkonnect/services/hl7assembly"
)

// AssemblyProcedureFunc is a named, parameterised assembly procedure that can be
// referenced from a v2.0 profile composite using "procedure": "<name>".
// It receives the full parsed HL7 data, the current resource slice, and any
// JSON params from the profile.  Returns the updated resource slice + warnings.
type AssemblyProcedureFunc func(
	parsedHL7 map[string]interface{},
	resources []map[string]interface{},
	params map[string]interface{},
) ([]map[string]interface{}, []string)

// ProfileComposite holds one rule parsed from a resource's "composites" array.
type ProfileComposite struct {
	FHIRResource    string                 // e.g. "Appointment"
	FHIRPath        string                 // e.g. "participant"
	CompositeType   string                 // "condition" | "forEach" | "firstOf" | "procedure"
	ConditionField  string                 // for condition composites: HL7 field that must be non-empty
	ForEachSeg      string                 // for forEach composites: segment name to iterate (e.g. "AIP")
	FirstOfFields   []string               // for firstOf composites: ordered list of HL7 field paths
	ValueTemplate   map[string]interface{} // value object with {{FIELD}} placeholders
	TemplateArray   []interface{}          // for firstOf: array template
	ProcedureName   string                 // for procedure composites: registered name
	ProcedureParams map[string]interface{} // for procedure composites: JSON params passed to the func
}

// assemblyProcedureRegistry holds all named assembly procedures registered at
// init() time from services/hl7assembly/procedure_registry.go.
var assemblyProcedureRegistry = map[string]AssemblyProcedureFunc{}

// RegisterAssemblyProcedure makes a named procedure available to the profile engine.
// Called from init() in procedure_registry.go so registration is automatic.
func RegisterAssemblyProcedure(name string, fn AssemblyProcedureFunc) {
	assemblyProcedureRegistry[name] = fn
}

// getProfileComposites loads composite rules + valueSets for a message type from
// the v2.0 profile stored in hl7_fhir_templates.
// Returns ([]ProfileComposite, valueSets map, error implicitly logged).
func (s *HL7FHIRTransformServiceV3) getProfileComposites(
	ctx context.Context,
	messageType string,
) ([]ProfileComposite, map[string]map[string]interface{}) {

	query := `
		SELECT template_config
		FROM   hl7_fhir_templates
		WHERE  message_type = $1
		  AND  is_default   = true
		  AND  profile_version = '2.0'
		LIMIT 1`

	var configJSON string
	if err := s.db.QueryRowContext(ctx, query, messageType).Scan(&configJSON); err != nil {
		if err != sql.ErrNoRows {
			log.Printf("⚠️ ProfileComposites: DB error for %s: %v", messageType, err)
		}
		return nil, nil
	}

	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &profile); err != nil {
		log.Printf("⚠️ ProfileComposites: invalid JSON for %s: %v", messageType, err)
		return nil, nil
	}

	// ── Extract valueSets ──────────────────────────────────────────────────
	valueSets := map[string]map[string]interface{}{}
	if vsRaw, ok := profile["valueSets"]; ok {
		if vsMap, ok := vsRaw.(map[string]interface{}); ok {
			for k, v := range vsMap {
				if entries, ok := v.(map[string]interface{}); ok {
					valueSets[k] = entries
				}
			}
		}
	}

	// ── Extract composites from each resource definition ──────────────────
	var composites []ProfileComposite

	mappingsRaw, _ := profile["mappings"].(map[string]interface{})
	for resourceKey, resDef := range mappingsRaw {
		resMap, ok := resDef.(map[string]interface{})
		if !ok {
			continue
		}
		resourceType := resourceKey // e.g. "Appointment"
		if rt, _ := resMap["resourceType"].(string); rt != "" {
			resourceType = rt
		}

		compRaw, ok := resMap["composites"].([]interface{})
		if !ok {
			continue
		}

		for _, ci := range compRaw {
			c, ok := ci.(map[string]interface{})
			if !ok {
				continue
			}

			pc := ProfileComposite{
				FHIRResource:  resourceType,
				FHIRPath:      getString(c, "fhir"),
				ValueTemplate: getMap(c, "value"),
			}

			switch {
			case c["condition"] != nil:
				pc.CompositeType = "condition"
				pc.ConditionField = getString(c, "condition")
			case c["forEach"] != nil:
				pc.CompositeType = "forEach"
				pc.ForEachSeg = getString(c, "forEach")
			case c["firstOf"] != nil:
				pc.CompositeType = "firstOf"
				if arr, ok := c["firstOf"].([]interface{}); ok {
					for _, f := range arr {
						if s, ok := f.(string); ok {
							pc.FirstOfFields = append(pc.FirstOfFields, s)
						}
					}
				}
				if tmpl, ok := c["template"].([]interface{}); ok {
					pc.TemplateArray = tmpl
				}
			case getString(c, "procedure") != "":
				pc.CompositeType = "procedure"
				pc.ProcedureName = getString(c, "procedure")
				if p, ok := c["params"].(map[string]interface{}); ok {
					pc.ProcedureParams = p
				}
			}

			composites = append(composites, pc)
		}
	}

	log.Printf("✅ ProfileComposites: loaded %d composites for %s", len(composites), messageType)
	return composites, valueSets
}

// applyProfileComposites executes composite rules against the parsed HL7 data and
// appends results to the already-field-mapped FHIR resources slice.
// Each element in the slice is a FHIR resource map containing at minimum a "resourceType" key.
func (s *HL7FHIRTransformServiceV3) applyProfileComposites(
	resources []map[string]interface{},
	parsedHL7 map[string]interface{},
	composites []ProfileComposite,
	valueSets map[string]map[string]interface{},
) []map[string]interface{} {

	for _, pc := range composites {
		// Find or create the target FHIR resource in the slice
		res, resources2 := s.getOrCreateProfileResource(resources, pc.FHIRResource)
		resources = resources2
		if res == nil {
			continue
		}

		switch pc.CompositeType {
		case "condition":
			// Emit exactly one entry when the condition field is non-empty
			condVal := s.resolveHL7FieldFromParsed(parsedHL7, pc.ConditionField)
			if condVal == "" {
				continue
			}
			entry := resolveTemplateMap(pc.ValueTemplate, nil, parsedHL7, condVal)
			if !isEmptyResolved(entry) {
				appendToFHIRArray(res, pc.FHIRPath, entry)
			}

		case "forEach":
			// Emit one entry per occurrence of the named segment
			segs := hl7assembly.ExtractSegmentGroup(parsedHL7, pc.ForEachSeg)
			for _, seg := range segs {
				segMap := segmentToMap(seg)
				entry := resolveTemplateMap(pc.ValueTemplate, segMap, parsedHL7, "")
				if !isEmptyResolved(entry) {
					appendToFHIRArray(res, pc.FHIRPath, entry)
				}
			}

		case "firstOf":
			// Use the first non-empty HL7 field value
			var chosen string
			for _, field := range pc.FirstOfFields {
				v := s.resolveHL7FieldFromParsed(parsedHL7, field)
				if v != "" {
					chosen = v
					break
				}
			}
			if chosen == "" {
				continue
			}
			if len(pc.TemplateArray) > 0 {
				// Expand array template with the chosen value
				for _, ti := range pc.TemplateArray {
					if tmpl, ok := ti.(map[string]interface{}); ok {
						entry := resolveTemplateMap(tmpl, nil, parsedHL7, chosen)
						if !isEmptyResolved(entry) {
							appendToFHIRArray(res, pc.FHIRPath, entry)
						}
					}
				}
			} else {
				appendToFHIRArray(res, pc.FHIRPath, chosen)
			}

		case "procedure":
			// Dispatch to a named, registered Go assembly procedure.
			// The procedure receives the full resource slice and may replace/add/remove
			// resources (not just append to a single FHIR array).
			fn, ok := assemblyProcedureRegistry[pc.ProcedureName]
			if !ok {
				log.Printf("⚠️ ProfileComposites: unknown procedure %q — register it in procedure_registry.go", pc.ProcedureName)
				continue
			}
			var procWarnings []string
			resources, procWarnings = fn(parsedHL7, resources, pc.ProcedureParams)
			if len(procWarnings) > 0 {
				log.Printf("⚠️ Procedure %s warnings: %v", pc.ProcedureName, procWarnings)
			}
			// After a procedure call the resource slice may have changed entirely;
			// skip the res pointer update and re-enter the loop with the new slice.
			continue
		}
	}

	return resources
}

// getOrCreateProfileResource finds a FHIR resource in the slice by resourceType,
// or creates a new skeleton resource and appends it to the slice.
// Returns the resource pointer and the (possibly extended) slice.
func (s *HL7FHIRTransformServiceV3) getOrCreateProfileResource(
	resources []map[string]interface{},
	resourceType string,
) (map[string]interface{}, []map[string]interface{}) {
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); strings.EqualFold(rt, resourceType) {
			return r, resources
		}
	}
	// Not found — create a skeleton and append
	r := map[string]interface{}{"resourceType": resourceType}
	resources = append(resources, r)
	return r, resources
}

// resolveTemplateMap deep-copies a template map replacing {{FIELD}} placeholders.
// segFields: segment fields keyed by "SEG.N" (used for forEach composites).
// parsedHL7: full parsed data (for cross-segment placeholders like {{PID.3}}).
// condVal:   value passed for condition composites (replaces {{_value}}).
func resolveTemplateMap(
	tmpl map[string]interface{},
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) map[string]interface{} {
	result := make(map[string]interface{}, len(tmpl))
	for k, v := range tmpl {
		result[k] = resolveTemplateValue(v, segFields, parsedHL7, condVal)
	}
	return result
}

func resolveTemplateValue(
	v interface{},
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) interface{} {
	switch tv := v.(type) {
	case string:
		return resolvePlaceholders(tv, segFields, parsedHL7, condVal)
	case map[string]interface{}:
		return resolveTemplateMap(tv, segFields, parsedHL7, condVal)
	case []interface{}:
		result := make([]interface{}, len(tv))
		for i, item := range tv {
			result[i] = resolveTemplateValue(item, segFields, parsedHL7, condVal)
		}
		return result
	default:
		return v
	}
}

var rePlaceholder = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// resolvePlaceholders replaces {{FIELD}} tokens in a string template.
// Lookup order: condVal (for {{_value}}), segFields (e.g. {{AIP.3}}), parsedHL7.
func resolvePlaceholders(
	s string,
	segFields map[string]string,
	parsedHL7 map[string]interface{},
	condVal string,
) string {
	return rePlaceholder.ReplaceAllStringFunc(s, func(match string) string {
		inner := match[2 : len(match)-2] // strip {{ }}
		if inner == "_value" {
			return condVal
		}
		// Check segment field map first (forEach context)
		if segFields != nil {
			if val, ok := segFields[inner]; ok {
				return val
			}
		}
		// Fall back to full parsed HL7 data (cross-segment).
		// Supports 2-part (SEG.F) and 3-part (SEG.F.C) paths.
		if parsedHL7 != nil {
			parts := strings.SplitN(inner, ".", 3)
			if len(parts) >= 2 {
				segs := hl7assembly.ExtractSegmentGroup(parsedHL7, parts[0])
				if len(segs) > 0 {
					fieldKey := parts[0] + "." + parts[1]
					for _, f := range segs[0].Fields {
						if f.Key == fieldKey {
							if len(parts) == 2 {
								return strings.TrimSpace(f.Value)
							}
							// 3-part: resolve component
							subKey := fieldKey + "." + parts[2]
							for _, sf := range f.Subfields {
								if sf.Key == subKey {
									return strings.TrimSpace(sf.Value)
								}
							}
							// Fall back to splitting raw value by ^
							compIdx := 0
							fmt.Sscanf(parts[2], "%d", &compIdx)
							if compIdx > 0 {
								comps := strings.Split(f.Value, "^")
								if compIdx <= len(comps) {
									return strings.TrimSpace(comps[compIdx-1])
								}
							}
							return ""
						}
					}
				}
			}
		}
		return "" // placeholder not resolved → empty string
	})
}

// segmentToMap converts an hl7.EnhancedSegment into a flat map[segKey]value
// suitable for placeholder resolution (e.g. "AIP.3" → "12345").
func segmentToMap(seg interface{}) map[string]string {
	// Accept either typed or interface{}
	b, err := json.Marshal(seg)
	if err != nil {
		return nil
	}
	var enhSeg struct {
		Key    string `json:"key"`
		Fields []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(b, &enhSeg); err != nil {
		return nil
	}
	m := make(map[string]string, len(enhSeg.Fields))
	for _, f := range enhSeg.Fields {
		m[f.Key] = f.Value
	}
	return m
}
