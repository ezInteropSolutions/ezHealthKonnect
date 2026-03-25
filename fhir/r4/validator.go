package r4

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Validation options & result types
// ─────────────────────────────────────────────────────────────────────────────

// ValidationLevel controls how deeply the validator inspects a resource.
type ValidationLevel int

const (
	// LevelBasic checks only that resourceType is a known FHIR R4 type.
	LevelBasic ValidationLevel = iota
	// LevelStandard adds structure validation: required fields and cardinality
	// derived from the compiled profile.
	LevelStandard
	// LevelStrict adds terminology binding checks and constraint predicate evaluation.
	LevelStrict
)

// ParseLevel converts a string label ("basic", "standard", "strict") to a
// ValidationLevel.  Unknown strings return LevelStandard.
func ParseLevel(s string) ValidationLevel {
	switch strings.ToLower(s) {
	case "basic":
		return LevelBasic
	case "strict":
		return LevelStrict
	default:
		return LevelStandard
	}
}

// ValidationOptions configures a single validation call.
type ValidationOptions struct {
	// Version of the FHIR specification to validate against. Default: "R4".
	Version string
	// Profile to apply on top of the base profile. Default: "base".
	// Use "us-core" to enforce US Core Must-Support requirements (warnings).
	Profile string
	// Level controls validation depth.
	Level ValidationLevel
	// RequiredTypes lists resource types that MUST appear at least once in a Bundle.
	RequiredTypes []string
	// ValidateRefs enables internal-reference resolution within a Bundle.
	ValidateRefs bool
}

// ValidationIssue is a single finding from a validation run.
type ValidationIssue struct {
	Severity string // "error" | "warning" | "information"
	Layer    string // "structure" | "terminology" | "constraint" | "reference" | "bundle"
	Code     string // machine-readable code, e.g. "required-field", "obs-6"
	Path     string // element path, e.g. "Patient.gender"
	Message  string
}

// ResourceResult holds the validation outcome for a single FHIR resource.
type ResourceResult struct {
	ResourceType string
	ResourceID   string
	Valid         bool // true when there are zero errors (warnings are OK)
	Errors        []ValidationIssue
	Warnings      []ValidationIssue
}

// BundleResult aggregates validation outcomes for an entire FHIR Bundle.
type BundleResult struct {
	Valid         bool
	BundleIssues  []ValidationIssue // Bundle-level issues (bun-1, bun-7, missing type…)
	Resources     []ResourceResult
	ErrorCount    int
	WarningCount  int
}

// ─────────────────────────────────────────────────────────────────────────────
// FHIRR4Validator — top-level orchestrator
// ─────────────────────────────────────────────────────────────────────────────

// FHIRR4Validator implements ResourceValidator by composing a ProfileProvider,
// TerminologyLookup, and ConstraintChecker.  All three dependencies are
// injected so they can be swapped out in tests.
type FHIRR4Validator struct {
	profiles     ProfileProvider
	terminology  TerminologyLookup
	constraints  ConstraintChecker
}

// NewFHIRR4Validator constructs a validator with explicit dependencies.
// Pass nil for any dependency to use the package-level singletons.
func NewFHIRR4Validator(p ProfileProvider, t TerminologyLookup, c ConstraintChecker) *FHIRR4Validator {
	if p == nil {
		p = globalRegistry
	}
	if t == nil {
		t = globalTermRegistry
	}
	if c == nil {
		c = globalConstraintRegistry
	}
	return &FHIRR4Validator{profiles: p, terminology: t, constraints: c}
}

var globalValidator *FHIRR4Validator

// GetValidator returns the package-level singleton FHIRR4Validator.
// The singleton is created lazily on first call using the package singletons.
func GetValidator() *FHIRR4Validator {
	if globalValidator == nil {
		globalValidator = NewFHIRR4Validator(nil, nil, nil)
	}
	return globalValidator
}

// ─────────────────────────────────────────────────────────────────────────────
// ResourceValidator implementation
// ─────────────────────────────────────────────────────────────────────────────

// ValidateResource validates a standalone FHIR resource (not inside a Bundle).
func (v *FHIRR4Validator) ValidateResource(
	resource map[string]interface{},
	opts ValidationOptions,
) ResourceResult {
	applyDefaults(&opts)
	rt, _ := resource["resourceType"].(string)
	id, _ := resource["id"].(string)

	res := ResourceResult{ResourceType: rt, ResourceID: id, Valid: true}

	// ── Layer 0: resourceType presence ────────────────────────────────────────
	if rt == "" {
		res.addError(issue("error", "structure", "missing-resource-type", "",
			"Resource is missing required 'resourceType' field"))
		return res
	}

	// ── Layer 1: known R4 type ────────────────────────────────────────────────
	if !isKnownR4Type(rt) {
		res.addError(issue("error", "structure", "unknown-resource-type", "resourceType",
			fmt.Sprintf("'%s' is not a known FHIR %s resource type", rt, opts.Version)))
		if opts.Level == LevelBasic {
			return res
		}
	}

	if opts.Level == LevelBasic {
		return res
	}

	// ── Layer 2: structure (requires compiled profile) ─────────────────────────
	cp := v.resolveProfile(opts.Version, rt, opts.Profile)
	if cp != nil {
		v.validateStructure(resource, cp, &res)
	} else {
		res.addWarning(issue("warning", "structure", "no-profile", "resourceType",
			fmt.Sprintf("No compiled profile found for %s/%s/%s — structure validation skipped",
				opts.Version, rt, opts.Profile)))
	}

	if opts.Level < LevelStrict {
		return res
	}

	// ── Layer 3: terminology + constraints ────────────────────────────────────
	if cp != nil {
		v.validateTerminology(resource, cp, &res)
	}
	v.validateConstraints(resource, rt, &res)

	return res
}

// ValidateBundle validates a FHIR Bundle and every resource entry within it.
func (v *FHIRR4Validator) ValidateBundle(
	bundle map[string]interface{},
	opts ValidationOptions,
) BundleResult {
	applyDefaults(&opts)
	result := BundleResult{Valid: true}

	// ── Bundle-level structure ─────────────────────────────────────────────────
	rt, _ := bundle["resourceType"].(string)
	if rt != "Bundle" {
		result.addBundleError(issue("error", "bundle", "not-a-bundle", "resourceType",
			fmt.Sprintf("Expected Bundle, got '%s'", rt)))
		return result
	}

	bundleType, _ := bundle["type"].(string)
	if bundleType == "" {
		result.addBundleError(issue("error", "bundle", "missing-bundle-type", "Bundle.type",
			"Bundle.type is required"))
	}

	// ── Bundle-level constraint checks (bun-1, bun-2, bun-3, bun-7) ──────────
	if v.constraints != nil && opts.Level >= LevelStrict {
		for _, viol := range v.constraints.Check("Bundle", bundle) {
			result.addBundleIssue(issue(viol.Severity, "constraint", viol.Key, "Bundle",
				viol.Description))
		}
	}

	// ── Entry iteration ───────────────────────────────────────────────────────
	entries, _ := toSlice(bundle["entry"])
	referenceTargets := collectReferenceTargets(entries) // for internal ref checks

	typesFound := make(map[string]bool)
	for i, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			result.addBundleError(issue("error", "bundle", "invalid-entry",
				fmt.Sprintf("Bundle.entry[%d]", i), "Entry is not a valid object"))
			continue
		}
		resource, ok := entry["resource"].(map[string]interface{})
		if !ok {
			result.addBundleError(issue("error", "bundle", "missing-resource",
				fmt.Sprintf("Bundle.entry[%d]", i), "Entry has no 'resource' object"))
			continue
		}

		rr := v.ValidateResource(resource, opts)

		// Internal reference validation
		if opts.ValidateRefs && opts.Level >= LevelStandard {
			v.validateInternalRefs(resource, referenceTargets, &rr)
		}

		rt, _ := resource["resourceType"].(string)
		typesFound[rt] = true
		result.Resources = append(result.Resources, rr)
	}

	// ── Required types ────────────────────────────────────────────────────────
	for _, required := range opts.RequiredTypes {
		if !typesFound[required] {
			result.addBundleError(issue("error", "bundle", "missing-required-type", "Bundle.entry",
				fmt.Sprintf("Required resource type '%s' not found in Bundle", required)))
		}
	}

	// ── Tally ─────────────────────────────────────────────────────────────────
	for _, iss := range result.BundleIssues {
		if iss.Severity == "error" {
			result.ErrorCount++
		} else {
			result.WarningCount++
		}
	}
	for _, rr := range result.Resources {
		result.ErrorCount += len(rr.Errors)
		result.WarningCount += len(rr.Warnings)
	}
	result.Valid = result.ErrorCount == 0

	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer 2: structure validation
// ─────────────────────────────────────────────────────────────────────────────

func (v *FHIRR4Validator) validateStructure(
	resource map[string]interface{},
	cp *CompiledProfile,
	res *ResourceResult,
) {
	rt := cp.ResourceType

	// Check required elements
	for path := range cp.Required {
		// Strip "ResourceType." prefix to get the field name (top-level only)
		field := topLevelField(rt, path)
		if field == "" {
			continue // nested path — skip (would need recursive descent)
		}
		val, exists := resource[field]
		if !exists || val == nil {
			res.addError(issue("error", "structure", "required-field", path,
				fmt.Sprintf("%s: required field '%s' is missing", rt, field)))
		}
	}

	// Check id presence (warning, not error — R4 base spec allows no id in some contexts)
	if id, _ := resource["id"].(string); id == "" {
		res.addWarning(issue("warning", "structure", "missing-id", rt+".id",
			fmt.Sprintf("%s: 'id' field is absent", rt)))
	}

	// Warn on modifier elements present in the resource
	for path := range cp.IsModifier {
		field := topLevelField(rt, path)
		if field == "" {
			continue
		}
		if resource[field] != nil {
			res.addWarning(issue("warning", "structure", "modifier-element", path,
				fmt.Sprintf("%s: modifier element '%s' present — verify business logic impact", rt, field)))
		}
	}

	// US Core Must-Support: emit information-level findings
	for _, path := range cp.MustSupport {
		field := topLevelField(rt, path)
		if field == "" {
			continue
		}
		if resource[field] == nil {
			res.addWarning(issue("information", "structure", "must-support", path,
				fmt.Sprintf("%s: must-support element '%s' is not populated", rt, field)))
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer 3a: terminology validation
// ─────────────────────────────────────────────────────────────────────────────

func (v *FHIRR4Validator) validateTerminology(
	resource map[string]interface{},
	cp *CompiledProfile,
	res *ResourceResult,
) {
	if v.terminology == nil {
		return
	}
	rt := cp.ResourceType

	for _, binding := range cp.Bindings {
		field := topLevelField(rt, binding.Path)
		if field == "" {
			continue
		}
		val := resource[field]
		if val == nil {
			continue // absent field — already caught by structure layer
		}

		code := extractCode(val) // handles string, CodeableConcept, Coding
		if code == "" {
			continue
		}

		// Use ContainsEx if the registry supports it (gives disclosure info).
		var inSet, known bool
		var externalSystems []string

		if tr, ok := v.terminology.(*TerminologyRegistry); ok {
			inSet, known, externalSystems = tr.ContainsEx(binding.ValueSetURL, code)
		} else {
			inSet, known = v.terminology.Contains(binding.ValueSetURL, code)
		}

		if !known {
			continue // ValueSet not in our files → pass-through (no false errors)
		}

		if !inSet {
			// If the code is not in our embedded set but the ValueSet includes
			// external licensed systems (SNOMED, LOINC, …), we cannot confirm it
			// is invalid — emit an information-level disclosure instead of an error.
			if len(externalSystems) > 0 {
				res.addInfo(issue("information", "terminology", "external-terminology",
					binding.Path,
					fmt.Sprintf("%s.%s: code '%s' could not be fully validated — %s "+
						"codes require a terminology server for complete verification",
						rt, field, code, strings.Join(externalSystems, ", "))))
				continue
			}

			if binding.BindingStrength == "required" {
				res.addError(issue("error", "terminology", "invalid-code", binding.Path,
					fmt.Sprintf("%s.%s: code '%s' is not in required ValueSet %s",
						rt, field, code, binding.ValueSetURL)))
			} else {
				res.addWarning(issue("warning", "terminology",
					"extensible-code-not-in-valueset", binding.Path,
					fmt.Sprintf("%s.%s: code '%s' is not in extensible ValueSet %s "+
						"(extension allowed)", rt, field, code, binding.ValueSetURL)))
			}
		} else if len(externalSystems) > 0 {
			// Code found in our embedded portion of a mixed VS — note the partial coverage.
			res.addInfo(issue("information", "terminology", "partial-terminology",
				binding.Path,
				fmt.Sprintf("%s.%s: code '%s' validated against embedded codes; "+
					"%s subset not checked locally",
					rt, field, code, strings.Join(externalSystems, ", "))))
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer 3b: constraint validation
// ─────────────────────────────────────────────────────────────────────────────

func (v *FHIRR4Validator) validateConstraints(
	resource map[string]interface{},
	resourceType string,
	res *ResourceResult,
) {
	if v.constraints == nil {
		return
	}
	for _, viol := range v.constraints.Check(resourceType, resource) {
		res.addIssue(issue(viol.Severity, "constraint", viol.Key,
			resourceType, viol.Description))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal reference validation
// ─────────────────────────────────────────────────────────────────────────────

func (v *FHIRR4Validator) validateInternalRefs(
	resource map[string]interface{},
	targets map[string]bool,
	res *ResourceResult,
) {
	walkRefs(resource, targets, res, "")
}

func walkRefs(
	node interface{},
	targets map[string]bool,
	res *ResourceResult,
	path string,
) {
	switch v := node.(type) {
	case map[string]interface{}:
		if ref, ok := v["reference"].(string); ok {
			// Skip absolute URLs and contained (#) references
			if !strings.HasPrefix(ref, "http") && !strings.HasPrefix(ref, "#") {
				if !targets[ref] {
					res.addWarning(issue("warning", "reference", "unresolved-reference", path,
						fmt.Sprintf("Reference '%s' cannot be resolved within this Bundle", ref)))
				}
			}
		}
		for k, child := range v {
			walkRefs(child, targets, res, path+"."+k)
		}
	case []interface{}:
		for i, item := range v {
			walkRefs(item, targets, res, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// collectReferenceTargets builds the set of fullUrls and "ResourceType/id"
// strings present in a Bundle's entry array for internal-reference resolution.
func collectReferenceTargets(entries []interface{}) map[string]bool {
	targets := make(map[string]bool, len(entries))
	for _, raw := range entries {
		e, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if fu, ok := e["fullUrl"].(string); ok && fu != "" {
			targets[fu] = true
		}
		res, ok := e["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		rt, _ := res["resourceType"].(string)
		id, _ := res["id"].(string)
		if rt != "" && id != "" {
			targets[rt+"/"+id] = true
		}
	}
	return targets
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func applyDefaults(opts *ValidationOptions) {
	if opts.Version == "" {
		opts.Version = "R4"
	}
	if opts.Profile == "" {
		opts.Profile = "base"
	}
}

// resolveProfile returns the best-matching compiled profile, preferring the
// requested profile name and falling back to "base" if not found.
func (v *FHIRR4Validator) resolveProfile(version, resourceType, profile string) *CompiledProfile {
	if v.profiles == nil {
		return nil
	}
	if cp, ok := v.profiles.Get(version, resourceType, profile); ok {
		return cp
	}
	// Fallback: base profile
	if profile != "base" {
		if cp, ok := v.profiles.Get(version, resourceType, "base"); ok {
			return cp
		}
	}
	return nil
}

// topLevelField returns the field name for an element path when the path
// contains exactly one dot (e.g. "Patient.gender" → "gender").
// Returns "" for nested paths (e.g. "Patient.name.family").
func topLevelField(resourceType, path string) string {
	prefix := resourceType + "."
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if strings.Contains(rest, ".") {
		return "" // nested — skip
	}
	return rest
}

// extractCode extracts a primitive code string from a FHIR value that may be:
//   - a plain string (for code, string primitives)
//   - a CodeableConcept map with coding[0].code
//   - a Coding map with .code
func extractCode(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		// Try Coding.code
		if code, ok := v["code"].(string); ok {
			return code
		}
		// Try CodeableConcept.coding[0].code
		if codings, ok := v["coding"].([]interface{}); ok && len(codings) > 0 {
			if first, ok := codings[0].(map[string]interface{}); ok {
				if code, ok := first["code"].(string); ok {
					return code
				}
			}
		}
	}
	return ""
}

// isKnownR4Type checks the canonical set of FHIR R4 resource type names.
// Identical to the set in fhir_utils.go but kept here to keep r4 self-contained.
func isKnownR4Type(rt string) bool {
	return knownR4Types[rt]
}

// knownR4Types is the authoritative set of FHIR R4 resource types.
var knownR4Types = map[string]bool{
	"Account": true, "ActivityDefinition": true, "AdverseEvent": true,
	"AllergyIntolerance": true, "Appointment": true, "AppointmentResponse": true,
	"AuditEvent": true, "Basic": true, "Binary": true, "BiologicallyDerivedProduct": true,
	"BodyStructure": true, "Bundle": true, "CapabilityStatement": true,
	"CarePlan": true, "CareTeam": true, "CatalogEntry": true,
	"ChargeItem": true, "ChargeItemDefinition": true, "Claim": true,
	"ClaimResponse": true, "ClinicalImpression": true, "CodeSystem": true,
	"Communication": true, "CommunicationRequest": true, "CompartmentDefinition": true,
	"Composition": true, "ConceptMap": true, "Condition": true, "Consent": true,
	"Contract": true, "Coverage": true, "CoverageEligibilityRequest": true,
	"CoverageEligibilityResponse": true, "DetectedIssue": true, "Device": true,
	"DeviceDefinition": true, "DeviceMetric": true, "DeviceRequest": true,
	"DeviceUseStatement": true, "DiagnosticReport": true, "DocumentManifest": true,
	"DocumentReference": true, "EffectEvidenceSynthesis": true, "Encounter": true,
	"Endpoint": true, "EnrollmentRequest": true, "EnrollmentResponse": true,
	"EpisodeOfCare": true, "EventDefinition": true, "Evidence": true,
	"EvidenceVariable": true, "ExampleScenario": true, "ExplanationOfBenefit": true,
	"FamilyMemberHistory": true, "Flag": true, "Goal": true, "GraphDefinition": true,
	"Group": true, "GuidanceResponse": true, "HealthcareService": true,
	"ImagingStudy": true, "Immunization": true, "ImmunizationEvaluation": true,
	"ImmunizationRecommendation": true, "ImplementationGuide": true,
	"InsurancePlan": true, "Invoice": true, "Library": true, "Linkage": true,
	"List": true, "Location": true, "Measure": true, "MeasureReport": true,
	"Media": true, "Medication": true, "MedicationAdministration": true,
	"MedicationDispense": true, "MedicationKnowledge": true, "MedicationRequest": true,
	"MedicationStatement": true, "MedicinalProduct": true,
	"MedicinalProductAuthorization": true, "MedicinalProductContraindication": true,
	"MedicinalProductIndication": true, "MedicinalProductIngredient": true,
	"MedicinalProductInteraction": true, "MedicinalProductManufactured": true,
	"MedicinalProductPackaged": true, "MedicinalProductPharmaceutical": true,
	"MedicinalProductUndesirableEffect": true, "MessageDefinition": true,
	"MessageHeader": true, "MolecularSequence": true, "NamingSystem": true,
	"NutritionOrder": true, "Observation": true, "ObservationDefinition": true,
	"OperationDefinition": true, "OperationOutcome": true, "Organization": true,
	"OrganizationAffiliation": true, "Parameters": true, "Patient": true,
	"PaymentNotice": true, "PaymentReconciliation": true, "Person": true,
	"PlanDefinition": true, "Practitioner": true, "PractitionerRole": true,
	"Procedure": true, "Provenance": true, "Questionnaire": true,
	"QuestionnaireResponse": true, "RelatedPerson": true, "RequestGroup": true,
	"ResearchDefinition": true, "ResearchElementDefinition": true,
	"ResearchStudy": true, "ResearchSubject": true, "RiskAssessment": true,
	"RiskEvidenceSynthesis": true, "Schedule": true, "SearchParameter": true,
	"ServiceRequest": true, "Slot": true, "Specimen": true,
	"SpecimenDefinition": true, "StructureDefinition": true, "StructureMap": true,
	"Subscription": true, "Substance": true, "SubstanceNucleicAcid": true,
	"SubstancePolymer": true, "SubstanceProtein": true,
	"SubstanceReferenceInformation": true, "SubstanceSourceMaterial": true,
	"SubstanceSpecification": true, "SupplyDelivery": true, "SupplyRequest": true,
	"Task": true, "TerminologyCapabilities": true, "TestReport": true,
	"TestScript": true, "ValueSet": true, "VerificationResult": true,
	"VisionPrescription": true,
}

// ─────────────────────────────────────────────────────────────────────────────
// Result helpers — keep mutation co-located with type definitions
// ─────────────────────────────────────────────────────────────────────────────

func issue(severity, layer, code, path, message string) ValidationIssue {
	return ValidationIssue{Severity: severity, Layer: layer, Code: code, Path: path, Message: message}
}

func (r *ResourceResult) addError(i ValidationIssue) {
	r.Errors = append(r.Errors, i)
	r.Valid = false
}
func (r *ResourceResult) addWarning(i ValidationIssue) { r.Warnings = append(r.Warnings, i) }
func (r *ResourceResult) addInfo(i ValidationIssue)    { r.Warnings = append(r.Warnings, i) }
func (r *ResourceResult) addIssue(i ValidationIssue) {
	if i.Severity == "error" {
		r.addError(i)
	} else {
		r.addWarning(i)
	}
}

func (b *BundleResult) addBundleError(i ValidationIssue) {
	b.BundleIssues = append(b.BundleIssues, i)
	b.Valid = false
}
func (b *BundleResult) addBundleIssue(i ValidationIssue) {
	b.BundleIssues = append(b.BundleIssues, i)
	if i.Severity == "error" {
		b.Valid = false
	}
}
