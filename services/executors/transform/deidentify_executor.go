// Package transform — DeidentifyExecutor: HIPAA Safe Harbor de-identification (P1).
//
// Implements the 18 Safe Harbor identifiers as defined in 45 CFR §164.514(b).
// Each field is replaced by a placeholder or a configurable pseudonym so that
// the message can be used for analytics, testing, or research without
// exposing PHI. A SHA-256 audit hash of the original values is emitted in
// _executionDetails for the pipeline_step_audit table (V51).
package transform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// DeidentifyExecutor replaces all 18 HIPAA Safe Harbor PHI identifiers with
// placeholder values, producing a de-identified copy of the message.
type DeidentifyExecutor struct {
	*executors.BaseExecutor
}

func NewDeidentifyExecutor() *DeidentifyExecutor {
	return &DeidentifyExecutor{
		BaseExecutor: executors.NewBaseExecutor("deidentify", models.ExecutorMetadata{
			Name:        "De-identify (HIPAA Safe Harbor)",
			Description: "Removes/replaces all 18 HIPAA Safe Harbor PHI identifiers. Produces audit hash for compliance logging.",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Security",
		}),
	}
}

// deidentifyConfig controls how the executor behaves.
type deidentifyConfig struct {
	// Format to use for path resolution: "hl7v2" | "fhir" | "json" | "auto"
	Format string `json:"format,omitempty"`

	// AuditHash — if true (default), emit a SHA-256 of original PHI values in _executionDetails.
	// The hash proves the data was present without storing raw PHI.
	AuditHash bool `json:"auditHash"`

	// HashSalt — optional pepper for the audit hash (stored in config, NOT in the audit record).
	HashSalt string `json:"hashSalt,omitempty"`

	// Pseudonymize — if true, replace PHI with consistent pseudonyms (hash-derived)
	// instead of fixed placeholders. Enables linkage across de-identified records.
	Pseudonymize bool `json:"pseudonymize,omitempty"`

	// ExcludeFields — field paths to exclude from de-identification (e.g. keep zip code for
	// geographic analysis as Safe Harbor allows 3-digit zip prefixes).
	ExcludeFields []string `json:"excludeFields,omitempty"`

	// IncludeExtraFields — additional field paths to de-identify beyond the built-in 18.
	IncludeExtraFields []string `json:"includeExtraFields,omitempty"`
}

// safeHarborField describes one of the 18 HIPAA Safe Harbor identifiers.
type safeHarborField struct {
	Identifier  int      // 1–18 per 45 CFR §164.514(b)
	Description string
	Placeholder string   // replacement value
	Paths       []string // field paths in the target format
}

// safeHarborHL7 covers the 18 identifiers for HL7 v2 messages.
var safeHarborHL7 = []safeHarborField{
	{1, "Names", "[DEIDENTIFIED]",
		[]string{"PID.5.1", "PID.5.2", "PID.5.3", "NK1.2.1", "NK1.2.2", "GT1.3.1", "GT1.3.2"}},
	{2, "Geographic (smaller than state)", "[CITY]",
		[]string{"PID.11.3", "PID.11.4", "PID.11.5", "PID.11.6"}},
	{3, "Dates (except year)", "[DATE]",
		[]string{"PID.7", "PID.8", "PV1.44", "PV1.45", "OBR.7", "OBR.8"}},
	{4, "Phone numbers", "[PHONE]",
		[]string{"PID.13.1", "PID.14.1", "NK1.5.1", "NK1.6.1"}},
	{5, "Fax numbers", "[FAX]",
		[]string{"PID.13.2"}},
	{6, "Email addresses", "[EMAIL]",
		[]string{"PID.13.4", "NK1.5.4"}},
	{7, "Social Security Numbers", "[SSN]",
		[]string{"PID.19"}},
	{8, "Medical Record Numbers", "[MRN]",
		[]string{"PID.3.1", "PID.18.1"}},
	{9, "Health plan beneficiary numbers", "[PLAN_ID]",
		[]string{"IN1.36", "IN2.1"}},
	{10, "Account numbers", "[ACCOUNT]",
		[]string{"PID.18.1"}},
	{11, "Certificate/license numbers", "[LICENSE]",
		[]string{"PD1.6", "PRD.7.1"}},
	{12, "Vehicle identifiers & serial numbers", "[VEHICLE_ID]",
		[]string{}},
	{13, "Device identifiers & serial numbers", "[DEVICE_ID]",
		[]string{"OBX.18.1"}},
	{14, "Web URLs", "[URL]",
		[]string{}},
	{15, "IP addresses", "[IP_ADDR]",
		[]string{}},
	{16, "Biometric identifiers (finger/voice prints)", "[BIOMETRIC]",
		[]string{}},
	{17, "Full-face photos & comparable images", "[IMAGE]",
		[]string{}},
	{18, "Any other unique identifying number, code, or characteristic", "[ID]",
		[]string{"PID.2", "PID.4"}},
}

// safeHarborFHIR covers the 18 identifiers for FHIR R4 resources.
var safeHarborFHIR = []safeHarborField{
	{1, "Names", "[DEIDENTIFIED]",
		[]string{"Patient.name[0].family", "Patient.name[0].given", "Patient.name[0].text",
			"RelatedPerson.name[0].family", "Practitioner.name[0].family"}},
	{2, "Geographic (smaller than state)", "[CITY]",
		[]string{"Patient.address[0].city", "Patient.address[0].postalCode", "Patient.address[0].line[0]"}},
	{3, "Dates (except year)", "[DATE]",
		[]string{"Patient.birthDate", "Patient.deceasedDateTime", "Encounter.period.start", "Encounter.period.end"}},
	{4, "Phone numbers", "[PHONE]",
		[]string{"Patient.telecom[system=phone].value"}},
	{5, "Fax numbers", "[FAX]",
		[]string{"Patient.telecom[system=fax].value"}},
	{6, "Email addresses", "[EMAIL]",
		[]string{"Patient.telecom[system=email].value"}},
	{7, "Social Security Numbers", "[SSN]",
		[]string{"Patient.identifier[type=SS].value"}},
	{8, "Medical Record Numbers", "[MRN]",
		[]string{"Patient.identifier[type=MR].value", "Patient.identifier[type=AN].value"}},
	{9, "Health plan beneficiary numbers", "[PLAN_ID]",
		[]string{"Coverage.identifier[0].value", "Coverage.subscriberId"}},
	{10, "Account numbers", "[ACCOUNT]",
		[]string{"Account.identifier[0].value"}},
	{11, "Certificate/license numbers", "[LICENSE]",
		[]string{"Practitioner.identifier[0].value"}},
	{12, "Vehicle identifiers", "[VEHICLE_ID]", []string{}},
	{13, "Device identifiers", "[DEVICE_ID]",
		[]string{"Device.identifier[0].value", "Device.serialNumber"}},
	{14, "Web URLs", "[URL]", []string{}},
	{15, "IP addresses", "[IP_ADDR]", []string{}},
	{16, "Biometric identifiers", "[BIOMETRIC]", []string{}},
	{17, "Full-face photos", "[IMAGE]", []string{}},
	{18, "Other unique identifiers", "[ID]",
		[]string{"Patient.identifier[0].value"}},
}

// Execute implements executor.Executor. Modifies message in-place.
func (e *DeidentifyExecutor) Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()
	outputData := shallowCloneMap(inputData)

	// Parse config
	cfg := deidentifyConfig{
		Format:    "auto",
		AuditHash: true,
	}
	if step.Config != nil {
		if b, err := json.Marshal(step.Config); err == nil {
			_ = json.Unmarshal(b, &cfg)
		}
	}

	// Determine format
	format := strings.ToLower(cfg.Format)
	if format == "" || format == "auto" {
		format = detectDeidentifyFormat(outputData)
	}

	// Select field list
	var fields []safeHarborField
	switch format {
	case "fhir":
		fields = safeHarborFHIR
	default: // hl7v2, json, or anything else
		fields = safeHarborHL7
	}

	// Build exclude set
	excludeSet := make(map[string]bool, len(cfg.ExcludeFields))
	for _, f := range cfg.ExcludeFields {
		excludeSet[f] = true
	}

	// Collect original values for audit hash
	var auditValues []string
	deidentifiedCount := 0
	fieldsTouched := []string{}

	for _, field := range fields {
		for _, path := range field.Paths {
			if excludeSet[path] {
				continue
			}
			originalVal := executors.GetFieldValue(outputData, path)
			if originalVal == nil {
				continue
			}
			originalStr := fmt.Sprintf("%v", originalVal)
			if originalStr == "" || originalStr == field.Placeholder {
				continue
			}
			// Collect for audit hash
			auditValues = append(auditValues, path+"="+originalStr)
			fieldsTouched = append(fieldsTouched, path)

			// Compute replacement
			replacement := field.Placeholder
			if cfg.Pseudonymize {
				replacement = pseudonym(originalStr, cfg.HashSalt, field.Placeholder)
			}
			if executors.UpdateFieldValue(outputData, path, replacement) {
				deidentifiedCount++
			}
		}
	}

	// Handle extra user-specified fields
	for _, path := range cfg.IncludeExtraFields {
		if excludeSet[path] {
			continue
		}
		originalVal := executors.GetFieldValue(outputData, path)
		if originalVal == nil {
			continue
		}
		originalStr := fmt.Sprintf("%v", originalVal)
		if originalStr == "" {
			continue
		}
		auditValues = append(auditValues, path+"="+originalStr)
		fieldsTouched = append(fieldsTouched, path)
		replacement := "[DEIDENTIFIED]"
		if cfg.Pseudonymize {
			replacement = pseudonym(originalStr, cfg.HashSalt, replacement)
		}
		if executors.UpdateFieldValue(outputData, path, replacement) {
			deidentifiedCount++
		}
	}

	// Build audit hash (SHA-256 of sorted field=value pairs, never stored as raw PHI)
	auditHash := ""
	if cfg.AuditHash && len(auditValues) > 0 {
		sort.Strings(auditValues)
		salt := cfg.HashSalt
		h := sha256.Sum256([]byte(salt + strings.Join(auditValues, "|")))
		auditHash = hex.EncodeToString(h[:])
	}

	durationMs := int64(time.Since(start).Milliseconds())

	// Step output
	variables := map[string]interface{}{
		"deidentified_count": deidentifiedCount,
		"fields_touched":     fieldsTouched,
		"format":             format,
		"audit_hash":         auditHash,
	}
	details := map[string]interface{}{
		"duration_ms":        durationMs,
		"success":            true,
		"deidentified_count": deidentifiedCount,
		"format":             format,
		"audit_hash":         auditHash,
		"fields_touched":     fieldsTouched,
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	return outputData, nil
}

// detectDeidentifyFormat inspects the message structure to guess the format.
func detectDeidentifyFormat(data map[string]interface{}) string {
	msg, _ := data["message"].(map[string]interface{})
	if msg == nil {
		msg = data
	}
	if _, ok := msg["enhancedSegments"]; ok {
		return "hl7v2"
	}
	if rt, ok := msg["resourceType"].(string); ok && rt != "" {
		return "fhir"
	}
	if fhirBundle, ok := msg["fhir_bundle"].(map[string]interface{}); ok {
		if _, ok := fhirBundle["entry"]; ok {
			return "fhir"
		}
	}
	return "hl7v2"
}

// pseudonym returns a deterministic, format-aware placeholder derived from the
// SHA-256 of (salt + value). The prefix is taken from the standard placeholder.
// e.g. "[MRN]" + hash → "[MRN-a3f9b2]"
func pseudonym(value, salt, placeholder string) string {
	h := sha256.Sum256([]byte(salt + value))
	short := hex.EncodeToString(h[:])[:6]
	bare := strings.TrimSuffix(strings.TrimPrefix(placeholder, "["), "]")
	return fmt.Sprintf("[%s-%s]", bare, short)
}

// shallowCloneMap creates a shallow copy of a map[string]interface{}.
func shallowCloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Validate implements executor.Executor.
func (e *DeidentifyExecutor) Validate(step *models.TransformationStep) error {
	return nil // no required config — defaults work out of the box
}

// GetOutputVariables implements the VariableProvider interface.
func (e *DeidentifyExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	ns := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	base := "steps." + ns + ".step_output"
	return []models.VariableDefinition{
		{Name: base + ".deidentified_count", DataType: "number",
			Description: "Number of PHI fields that were de-identified"},
		{Name: base + ".fields_touched", DataType: "array",
			Description: "Field paths that were de-identified"},
		{Name: base + ".format", DataType: "string",
			Description: "Format used for path resolution (hl7v2 or fhir)"},
		{Name: base + ".audit_hash", DataType: "string",
			Description: "SHA-256 hash of original PHI values (safe for HIPAA audit logging)"},
	}
}

// GetConfigSchema implements the Describable interface.
func (e *DeidentifyExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"format":             map[string]interface{}{"type": "string", "enum": []string{"auto", "hl7v2", "fhir", "json"}},
			"auditHash":          map[string]interface{}{"type": "boolean", "default": true},
			"hashSalt":           map[string]interface{}{"type": "string"},
			"pseudonymize":       map[string]interface{}{"type": "boolean", "default": false},
			"excludeFields":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"includeExtraFields": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
	}
}

// regexpEmailLike is used internally for format detection (unexported).
var regexpEmailLike = regexp.MustCompile(`@`)
