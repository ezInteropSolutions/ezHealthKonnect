package services

// procedure_registry.go
//
// Registers all named assembly procedures used by the profile-driven composite
// engine (profile_version = "2.0"). Each procedure wraps an existing typed
// assembly function in the AssemblyProcedureFunc uniform interface so that a
// profile JSON can declare:
//
//	"procedure": "assembleObservationsFromOBX",
//	"params": { "collapseTextOBX": true, "linkToDiagnosticReport": true }
//
// Registration order does not matter; all procedures are registered at
// package init time so they are available before any message is processed.
//
// To add a new procedure:
//  1. Write your typed assembly function in a *_assembly.go file.
//  2. Add a RegisterAssemblyProcedure call below bridging it to a uniform name.
//  3. Reference the name in the profile JSON.

import (
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/hl7assembly"
)

func init() {

	// ── ORU^R01 — Observations / DiagnosticReport ─────────────────────────────
	//
	// Params (all optional, default ON when absent):
	//   collapseTextOBX         bool  – merge consecutive TX/FT OBX by OBX.3
	//   linkToDiagnosticReport  bool  – wire Observation refs into DR.result[]
	//   liftEDAttachments       bool  – promote OBX.5 ED binary to DocumentReference
	RegisterAssemblyProcedure("assembleObservationsFromOBX",
		func(
			parsedHL7 map[string]interface{},
			resources []map[string]interface{},
			params map[string]interface{},
		) ([]map[string]interface{}, []string) {

			rules := hl7assembly.AssemblyRules{}

			if v, ok := params["collapseTextOBX"]; ok {
				b := asBool(v)
				rules.CollapseTextOBX = &b
			}
			if v, ok := params["linkToDiagnosticReport"]; ok {
				b := asBool(v)
				rules.DRResultLinks = &b
			}
			if v, ok := params["liftEDAttachments"]; ok {
				// Governs OBX.5 ED binary lift (same rule as ObsValueDispatch path)
				b := asBool(v)
				rules.ObsValueDispatch = &b
			}
			if v, ok := params["facilityNamespace"]; ok {
				if s, ok := v.(string); ok {
					rules.FacilityNamespace = s
				}
			}

			return hl7assembly.AssembleORUObservations(parsedHL7, resources, rules)
		},
	)

	// ── MDM^T01-T11 — DocumentReference / TXA ────────────────────────────────
	//
	// Params: none currently exposed; all behaviour driven by TXA fields.
	RegisterAssemblyProcedure("buildDocumentReferenceFromTXA",
		func(
			parsedHL7 map[string]interface{},
			resources []map[string]interface{},
			params map[string]interface{},
		) ([]map[string]interface{}, []string) {

			rules := hl7assembly.AssemblyRules{}
			if v, ok := params["facilityNamespace"]; ok {
				if s, ok := v.(string); ok {
					rules.FacilityNamespace = s
				}
			}

			return hl7assembly.AssembleMDMDocument(parsedHL7, resources, rules)
		},
	)

	// ── VXU^V04 — Immunization ────────────────────────────────────────────────
	//
	// Params: none currently exposed; vaccination groups resolved positionally.
	RegisterAssemblyProcedure("assembleVXUImmunizations",
		func(
			parsedHL7 map[string]interface{},
			resources []map[string]interface{},
			params map[string]interface{},
		) ([]map[string]interface{}, []string) {

			rules := hl7assembly.AssemblyRules{}
			if v, ok := params["facilityNamespace"]; ok {
				if s, ok := v.(string); ok {
					rules.FacilityNamespace = s
				}
			}

			return hl7assembly.AssembleVXUImmunizations(parsedHL7, resources, rules)
		},
	)

	// ── DFT^P03 — ChargeItem / Procedure / Condition ─────────────────────────
	//
	// Params: none currently exposed; FT1↔PR1 correlation done internally.
	RegisterAssemblyProcedure("assembleDFTCharges",
		func(
			parsedHL7 map[string]interface{},
			resources []map[string]interface{},
			params map[string]interface{},
		) ([]map[string]interface{}, []string) {

			rules := hl7assembly.AssemblyRules{}
			if v, ok := params["facilityNamespace"]; ok {
				if s, ok := v.(string); ok {
					rules.FacilityNamespace = s
				}
			}

			return hl7assembly.AssembleDFTCharges(parsedHL7, resources, rules)
		},
	)

	// ── MFN — Master File Notification ───────────────────────────────────────
	//
	// Params (optional):
	//   mfiOverrides  map[string]string – override MFI.1 → FHIR resource type
	//                   e.g. {"EMP": "Organization", "INS": "Coverage"}
	RegisterAssemblyProcedure("assembleMFNResources",
		func(
			parsedHL7 map[string]interface{},
			resources []map[string]interface{},
			params map[string]interface{},
		) ([]map[string]interface{}, []string) {

			var runtimeCfg *models.MFNRuntimeConfig

			if overridesRaw, ok := params["mfiOverrides"]; ok {
				if overridesMap, ok := overridesRaw.(map[string]interface{}); ok && len(overridesMap) > 0 {
					typed := make(map[string]string, len(overridesMap))
					for k, v := range overridesMap {
						if s, ok := v.(string); ok {
							typed[k] = s
						}
					}
					runtimeCfg = &models.MFNRuntimeConfig{MFIOverrides: typed}
				}
			}

			rules := hl7assembly.AssemblyRules{}
			if v, ok := params["facilityNamespace"]; ok {
				if s, ok := v.(string); ok {
					rules.FacilityNamespace = s
				}
			}

			_ = runtimeCfg // MFI overrides handled via V107 field-mapping templates
			return hl7assembly.AssembleMFNResources(parsedHL7, resources, rules)
		},
	)
}

// asBool coerces an interface{} value to bool.
// Accepts bool, string ("true"/"1"/"yes"), and float64/int (non-zero = true).
func asBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1" || t == "yes"
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return false
}
