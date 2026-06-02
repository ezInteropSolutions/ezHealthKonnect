package services

import (
	"context"
	"encoding/json"
	"log"
	"strings"
)

// loadAssemblyRulesForType returns the assembly rules for one message type,
// using an in-memory cache keyed by messageType.  The cache is populated on
// first access and can be invalidated by calling InvalidateAssemblyRuleCache.
func (s *HL7FHIRTransformServiceV3) loadAssemblyRulesForType(ctx context.Context, messageType string) []AssemblyRule {
	s.assemblyRulesMu.RLock()
	if s.assemblyRules != nil {
		rules := s.assemblyRules[messageType]
		s.assemblyRulesMu.RUnlock()
		return rules
	}
	s.assemblyRulesMu.RUnlock()

	// Cache miss — load ALL rules from DB once.
	s.assemblyRulesMu.Lock()
	defer s.assemblyRulesMu.Unlock()
	if s.assemblyRules != nil { // double-check after acquiring write lock
		return s.assemblyRules[messageType]
	}

	all := make(map[string][]AssemblyRule)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, message_type, rule_type, source_resource, target_resource,
		       reference_path, COALESCE(condition_expr,''), sequence,
		       COALESCE(config::text, '{}')
		FROM   assembly_rules
		WHERE  is_active = true
		ORDER  BY message_type, sequence`)
	if err != nil {
		log.Printf("⚠️  assembly_rules query failed: %v — cross-resource wiring disabled", err)
		s.assemblyRules = all // empty cache prevents repeated DB errors
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var r AssemblyRule
		var configJSON string
		if err2 := rows.Scan(&r.ID, &r.MessageType, &r.RuleType,
			&r.SourceResource, &r.TargetResource,
			&r.ReferencePath, &r.ConditionExpr, &r.Sequence,
			&configJSON); err2 != nil {
			log.Printf("⚠️  assembly_rules scan error: %v", err2)
			continue
		}
		if configJSON != "" && configJSON != "{}" {
			if err3 := json.Unmarshal([]byte(configJSON), &r.Config); err3 != nil {
				log.Printf("⚠️  assembly_rules config parse error (rule %s): %v", r.ID, err3)
			}
		}
		all[r.MessageType] = append(all[r.MessageType], r)
	}
	s.assemblyRules = all
	log.Printf("📋 assembly_rules cache loaded: %d message types", len(all))
	return all[messageType]
}

// InvalidateAssemblyRuleCache clears the in-memory cache so the next
// transformation reloads from the database.  Call after migrating new rules.
func (s *HL7FHIRTransformServiceV3) InvalidateAssemblyRuleCache() {
	s.assemblyRulesMu.Lock()
	s.assemblyRules = nil
	s.assemblyRulesMu.Unlock()
	log.Printf("🔄 assembly_rules cache invalidated")
}

// applyAssemblyRules executes every rule for messageType against the resource
// slice and returns the updated slice.
func (s *HL7FHIRTransformServiceV3) applyAssemblyRules(
	ctx context.Context,
	messageType string,
	resources []map[string]interface{},
) []map[string]interface{} {
	rules := s.loadAssemblyRulesForType(ctx, messageType)
	if len(rules) == 0 {
		return resources
	}
	for _, rule := range rules {
		resources = s.applyOneAssemblyRule(rule, resources)
	}
	log.Printf("✅ assembly_rules applied: %d rules for %s", len(rules), messageType)
	return resources
}

// applyOneAssemblyRule applies a single assembly rule to the resource slice.
func (s *HL7FHIRTransformServiceV3) applyOneAssemblyRule(
	rule AssemblyRule,
	resources []map[string]interface{},
) []map[string]interface{} {
	switch rule.RuleType {

	case "reference", "subject", "encounter":
		// Set source.referencePath = { reference: "TargetType/id" } when absent.
		// FHIR paths with cardinality 0..* (e.g. Encounter.appointment, DiagnosticReport.basedOn)
		// must be JSON arrays; wrap the reference object accordingly.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		arrayPaths := map[string]bool{
			"appointment":     true,
			"basedOn":         true,
			"partOf":          true,
			"reasonReference": true,
		}
		useArray := arrayPaths[rule.ReferencePath]
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			existing := r[rule.ReferencePath]
			if existing != nil {
				// "subject" rules must win over a heuristic display-only value
				// (e.g. {"display": "Colfer, Eoin"} with no "reference" key).
				// Other rule types skip when any value is already present.
				if rule.RuleType != "subject" {
					continue
				}
				if em, ok := existing.(map[string]interface{}); ok {
					if _, hasRef := em["reference"]; hasRef {
						continue // already correctly wired — leave it
					}
					// display-only: fall through and overwrite
				} else {
					continue // non-map value (raw string etc.) — leave it
				}
			}
			if useArray {
				r[rule.ReferencePath] = []interface{}{ref}
			} else {
				r[rule.ReferencePath] = ref
			}
		}

	case "focus":
		// Append { reference: "TargetType/id" } to source.focus[] when not present.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			focus, _ := r[rule.ReferencePath].([]interface{})
			if !assemblyRefPresent(focus, targetID) {
				r[rule.ReferencePath] = append(focus, ref)
			}
		}

	case "result":
		// Collect ALL target resources and set source.referencePath = [refs…].
		// Overwrites any existing value so the field-mapper's placeholder is replaced.
		var refs []interface{}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.TargetResource {
				continue
			}
			if id, _ := r["id"].(string); id != "" {
				refs = append(refs, map[string]interface{}{
					"reference": rule.TargetResource + "/" + id,
				})
			}
		}
		if len(refs) == 0 {
			return resources
		}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt == rule.SourceResource {
				r[rule.ReferencePath] = refs
			}
		}

	case "performer", "author":
		// Set source.referencePath = [{ reference: "TargetType/id" }] when absent.
		targetID := assemblyFindID(resources, rule.TargetResource)
		if targetID == "" {
			return resources
		}
		ref := map[string]interface{}{"reference": rule.TargetResource + "/" + targetID}
		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}
			if _, exists := r[rule.ReferencePath]; !exists {
				r[rule.ReferencePath] = []interface{}{ref}
			}
		}

	case "logical_ref":
		// Build an identifier-based FHIR reference instead of a URL reference.
		// Produces: source.referencePath = [{identifier:{system,value}, display}]
		//
		// Config keys (from assembly_rules.config JSONB):
		//   identifier_system  — system URI for the identifier (e.g. "urn:local:payer")
		//   identifier_field   — field name on the target resource holding the value
		//   display_field      — field name on the target resource for display text
		//                        (defaults to identifier_field when absent)
		//
		// Resolution priority:
		//   1. Target resource exists in bundle → read identifier_field / display_field
		//   2. Existing display-only reference at source.referencePath (from field mapping)
		//      → use display value as both identifier value and display text
		identifierSystem, _ := rule.Config["identifier_system"].(string)
		identifierField, _  := rule.Config["identifier_field"].(string)
		displayField, _     := rule.Config["display_field"].(string)
		if displayField == "" {
			displayField = identifierField
		}

		for _, r := range resources {
			if rt, _ := r["resourceType"].(string); rt != rule.SourceResource {
				continue
			}

			// Priority 1: find target resource in bundle and read its fields.
			identifierValue, displayValue := "", ""
			for _, t := range resources {
				if trt, _ := t["resourceType"].(string); trt != rule.TargetResource {
					continue
				}
				if identifierField != "" {
					identifierValue, _ = t[identifierField].(string)
				}
				if displayField != "" {
					displayValue, _ = t[displayField].(string)
				}
				break
			}

			// Priority 2: existing display-only reference at the path (from field mapping).
			if identifierValue == "" {
				switch existing := r[rule.ReferencePath].(type) {
				case []interface{}:
					if len(existing) > 0 {
						if m, ok := existing[0].(map[string]interface{}); ok {
							if d, _ := m["display"].(string); d != "" {
								identifierValue = d
								if displayValue == "" {
									displayValue = d
								}
							}
						}
					}
				case map[string]interface{}:
					if d, _ := existing["display"].(string); d != "" {
						identifierValue = d
						if displayValue == "" {
							displayValue = d
						}
					}
				}
			}

			if identifierValue == "" {
				continue // nothing to wire for this resource
			}

			ref := map[string]interface{}{}
			if identifierSystem != "" {
				ref["identifier"] = map[string]interface{}{
					"system": identifierSystem,
					"value":  identifierValue,
				}
			}
			if displayValue != "" {
				ref["display"] = displayValue
			}

			// referencePath is always an array for logical_ref targets (e.g. Coverage.payor is 1..*)
			r[rule.ReferencePath] = []interface{}{ref}
		}
	}

	return resources
}

// assemblyFindID returns the id of the first resource with the given type.
func assemblyFindID(resources []map[string]interface{}, resourceType string) string {
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == resourceType {
			if id, _ := r["id"].(string); id != "" {
				return id
			}
		}
	}
	return ""
}

// assemblyRefPresent reports whether a focus/result slice already contains a
// reference to the given id (prevents duplicates when the template also wires focus).
func assemblyRefPresent(refs []interface{}, targetID string) bool {
	for _, f := range refs {
		if fm, ok := f.(map[string]interface{}); ok {
			if ref, _ := fm["reference"].(string); strings.Contains(ref, targetID) {
				return true
			}
		}
	}
	return false
}

// augmentMessageHeaderFocus ensures MessageHeader.focus references every resource
// whose type is listed in focusTypes.  It is called after segment processors run
// so that processor-added resources (e.g. the prior Patient from MRGProcessor)
// are included.  Existing references are not duplicated.
// References use "ResourceType/id" format; rewriteReferences() converts them to
// urn:uuid: during bundle assembly.
func augmentMessageHeaderFocus(resources []map[string]interface{}, focusTypes []string) {
	if len(focusTypes) == 0 {
		return
	}

	// Build a fast lookup for which types belong in focus.
	wantedTypes := make(map[string]bool, len(focusTypes))
	for _, ft := range focusTypes {
		wantedTypes[ft] = true
	}

	// Find the MessageHeader.
	var mh map[string]interface{}
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == "MessageHeader" {
			mh = r
			break
		}
	}
	if mh == nil {
		return
	}

	// Build a set of references already present so we don't duplicate.
	existing := make(map[string]bool)
	var focus []interface{}
	if f, ok := mh["focus"].([]interface{}); ok {
		focus = f
		for _, entry := range f {
			if fm, ok := entry.(map[string]interface{}); ok {
				if ref, _ := fm["reference"].(string); ref != "" {
					existing[ref] = true
				}
			}
		}
	}

	// Append a focus reference for every resource of a wanted type not yet present.
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		if !wantedTypes[rt] {
			continue
		}
		id, _ := r["id"].(string)
		if id == "" {
			continue
		}
		ref := rt + "/" + id
		if !existing[ref] {
			focus = append(focus, map[string]interface{}{"reference": ref})
			existing[ref] = true
		}
	}

	mh["focus"] = focus
}

