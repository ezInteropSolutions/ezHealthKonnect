package transform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// DataMaskingExecutor masks sensitive data fields (PHI/PII) for HIPAA compliance
type DataMaskingExecutor struct {
	*executors.BaseExecutor
}

func NewDataMaskingExecutor() *DataMaskingExecutor {
	return &DataMaskingExecutor{
		BaseExecutor: executors.NewBaseExecutor("data_masking", models.ExecutorMetadata{
			Name:        "Data Masking",
			Description: "Masks sensitive PHI/PII fields for HIPAA compliance",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Security",
		}),
	}
}

type maskingRule struct {
	Field           string `json:"field"`           // Field path (e.g., PID.3, Patient.name)
	Strategy        string `json:"strategy"`        // mask, hash, redact, partial, tokenize, substitute
	MaskChar        string `json:"maskChar"`        // Character for masking (default: *)
	KeepFirst       int    `json:"keepFirst"`       // For partial: keep first N chars
	KeepLast        int    `json:"keepLast"`        // For partial: keep last N chars
	HashSalt        string `json:"hashSalt"`        // Salt for hashing
	Pattern         string `json:"pattern"`         // Regex pattern for selective masking
	SubstituteType  string `json:"substituteType"`  // For substitute: name|ssn|phone|email|date|address|custom
	SubstituteValue string `json:"substituteValue"` // For substitute+custom: the fixed replacement value
}

type dataMaskingConfig struct {
	Rules            []maskingRule `json:"rules"`
	MaskAllPHI       bool          `json:"maskAllPHI"`         // Auto-mask common PHI fields
	MaskAllPHIFormat string        `json:"maskAllPHIFormat"`   // "hl7v2" (default) | "fhir" | "json"
	PreserveFormat   bool          `json:"preserveFormat"`     // Keep format (e.g., ###-##-#### for SSN)
}

// commonPHIFields covers 13 of the 18 HIPAA Safe Harbor identifiers applicable to
// standard HL7 v2 messages. Mirrors DataMaskingBuilder.PHI_FIELDS in the frontend.
//
// Not applicable to HL7 v2 (omitted): #12 Vehicle IDs, #14 Web URLs,
// #15 IP addresses, #16 Full-face photos, #17 Biometric identifiers.
var commonPHIFields = []maskingRule{
	// HIPAA #1 — Names
	{Field: "PID.5", Strategy: "mask"},  // Patient name
	{Field: "PID.6", Strategy: "mask"},  // Mother's maiden name
	{Field: "NK1.2", Strategy: "mask"},  // Next of kin name
	{Field: "GT1.3", Strategy: "mask"},  // Guarantor name
	// HIPAA #2 — Geographic data (all sub-state units smaller than state)
	{Field: "PID.11.1", Strategy: "mask"},                             // Street address
	{Field: "PID.11.3", Strategy: "mask"},                             // City
	{Field: "PID.11.5", Strategy: "partial", KeepFirst: 3, KeepLast: 0}, // Zip: keep first 3 digits
	// HIPAA #3 — Dates (all dates except year)
	{Field: "PID.7",  Strategy: "partial", KeepFirst: 4, KeepLast: 0}, // DOB: keep year only
	{Field: "PID.29", Strategy: "redact"},                              // Date of death
	{Field: "PV1.44", Strategy: "partial", KeepFirst: 4, KeepLast: 0}, // Admit date: keep year only
	{Field: "PV1.45", Strategy: "partial", KeepFirst: 4, KeepLast: 0}, // Discharge date: keep year only
	// HIPAA #4 & #5 — Telephone and fax numbers
	{Field: "PID.13", Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // Home phone/fax
	{Field: "PID.14", Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // Work phone/fax
	// HIPAA #6 — Electronic mail addresses
	{Field: "PID.13.4", Strategy: "mask"}, // Email in XTN component 4
	// HIPAA #7 — Social security numbers
	{Field: "PID.19", Strategy: "hash"}, // SSN
	// HIPAA #8 — Medical record numbers
	{Field: "PID.3", Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // MRN: keep last 4
	// HIPAA #9 — Health plan beneficiary numbers
	{Field: "IN1.49", Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // Member ID
	{Field: "IN1.2",  Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // Insurance plan ID
	// HIPAA #10 — Account numbers
	{Field: "PID.18", Strategy: "partial", KeepFirst: 0, KeepLast: 4}, // Patient account number
	// HIPAA #11 — Certificate and license numbers
	{Field: "PID.20", Strategy: "hash"}, // Driver's license / certificate number
	// HIPAA #13 — Device identifiers and serial numbers
	{Field: "OBX.18", Strategy: "hash"}, // Equipment instance ID
	// HIPAA #18 — Any other unique identifying number or code
	{Field: "PID.2", Strategy: "hash"}, // Patient alias ID
	{Field: "PID.4", Strategy: "hash"}, // Alternate patient ID
}

// fhirCommonPHIFields covers the same 13 HIPAA identifiers for FHIR R4 resources.
// Uses indexed array paths (e.g., Patient.identifier[0].value) as supported by GetFieldValue.
var fhirCommonPHIFields = []maskingRule{
	// HIPAA #1 — Names
	{Field: "Patient.name[0].family",       Strategy: "mask"},
	{Field: "Patient.name[0].given",        Strategy: "mask"},
	{Field: "RelatedPerson.name[0].family", Strategy: "mask"},
	// HIPAA #2 — Geographic data
	{Field: "Patient.address[0].line[0]",    Strategy: "mask"},
	{Field: "Patient.address[0].city",       Strategy: "mask"},
	{Field: "Patient.address[0].postalCode", Strategy: "partial", KeepFirst: 3, KeepLast: 0},
	// HIPAA #3 — Dates
	{Field: "Patient.birthDate",        Strategy: "partial", KeepFirst: 4, KeepLast: 0},
	{Field: "Patient.deceasedDateTime", Strategy: "redact"},
	{Field: "Encounter.period.start",   Strategy: "partial", KeepFirst: 4, KeepLast: 0},
	{Field: "Encounter.period.end",     Strategy: "partial", KeepFirst: 4, KeepLast: 0},
	// HIPAA #4 & #5 — Telecom (phone + fax)
	{Field: "Patient.telecom[0].value", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "Patient.telecom[1].value", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #6 — Email
	{Field: "Patient.telecom[2].value", Strategy: "mask"},
	// HIPAA #7 — SSN / primary identifier
	{Field: "Patient.identifier[0].value", Strategy: "hash"},
	// HIPAA #8 — MRN / secondary identifier
	{Field: "Patient.identifier[1].value", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #9 — Insurance
	{Field: "Coverage.subscriberId",          Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "Coverage.identifier[0].value",   Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #10 — Account numbers
	{Field: "Account.identifier[0].value", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #11 — License numbers
	{Field: "Practitioner.identifier[0].value", Strategy: "hash"},
	// HIPAA #13 — Device identifiers
	{Field: "Device.identifier[0].value", Strategy: "hash"},
	// HIPAA #18 — Other unique identifiers
	{Field: "Patient.identifier[2].value", Strategy: "hash"},
}

// jsonCommonPHIFields covers the same 13 HIPAA identifiers for generic JSON messages.
// Field paths follow common patient data schema conventions (patient.* prefix).
var jsonCommonPHIFields = []maskingRule{
	// HIPAA #1 — Names
	{Field: "patient.name",      Strategy: "mask"},
	{Field: "patient.firstName", Strategy: "mask"},
	{Field: "patient.lastName",  Strategy: "mask"},
	// HIPAA #2 — Geographic data
	{Field: "patient.streetAddress", Strategy: "mask"},
	{Field: "patient.city",          Strategy: "mask"},
	{Field: "patient.zipCode",       Strategy: "partial", KeepFirst: 3, KeepLast: 0},
	// HIPAA #3 — Dates
	{Field: "patient.dateOfBirth", Strategy: "partial", KeepFirst: 4, KeepLast: 0},
	{Field: "patient.dob",         Strategy: "partial", KeepFirst: 4, KeepLast: 0},
	// HIPAA #4 & #5 — Phone and fax
	{Field: "patient.phone",       Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "patient.phoneNumber", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "patient.fax",         Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #6 — Email
	{Field: "patient.email", Strategy: "mask"},
	// HIPAA #7 — SSN
	{Field: "patient.ssn",                  Strategy: "hash"},
	{Field: "patient.socialSecurityNumber", Strategy: "hash"},
	// HIPAA #8 — MRN
	{Field: "patient.mrn",                 Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "patient.medicalRecordNumber", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #9 — Insurance
	{Field: "patient.insuranceId", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	{Field: "patient.memberId",    Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #10 — Account numbers
	{Field: "patient.accountNumber", Strategy: "partial", KeepFirst: 0, KeepLast: 4},
	// HIPAA #11 — License numbers
	{Field: "patient.driversLicense", Strategy: "hash"},
	// HIPAA #13 — Device identifiers
	{Field: "device.serialNumber", Strategy: "hash"},
	// HIPAA #18 — Other unique identifiers
	{Field: "patient.alternateId", Strategy: "hash"},
}

// commonPHIFieldsByFormat maps the maskAllPHIFormat config value to the correct rule set.
// Unknown formats fall back to HL7 v2.
var commonPHIFieldsByFormat = map[string][]maskingRule{
	"hl7v2": commonPHIFields,
	"fhir":  fhirCommonPHIFields,
	"json":  jsonCommonPHIFields,
}

func (e *DataMaskingExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := dataMaskingConfig{}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	// Build rule set — select format-specific PHI fields when MaskAllPHI is enabled
	rules := config.Rules
	if config.MaskAllPHI {
		format := config.MaskAllPHIFormat
		if format == "" {
			format = "hl7v2" // backward-compatible default
		}
		phiFields, ok := commonPHIFieldsByFormat[format]
		if !ok {
			phiFields = commonPHIFields // unknown format → fall back to HL7 v2
		}
		rules = append(phiFields, rules...)
	}

	if len(rules) == 0 {
		log.Printf("  ⚠️ Data Masking: no rules configured")
		return inputData, nil
	}

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Convert any typed HL7 EnhancedSegment structs → generic map[string]interface{}
	// BEFORE the masking loop so GetFieldValue / UpdateFieldValue can traverse them.
	// (The typed structs come from hl7.ParseWithRealSchema and cannot be updated in-place
	// via the format-agnostic utilities, which operate on map[string]interface{} only.)
	dataMaskingConvertTypedSegments(outputData)

	// Resolve "steps.{baseName}.xxx" → "steps.{baseName}_{shortID}.xxx" paths.
	// The pipeline generates namespaces as "{baseName}_{6-char-UUID}" (see generateStepNamespace).
	// Users and the variable picker reference steps by base name only — normalize them here
	// so GetFieldValue / UpdateFieldValue can find the correct key in the stepsSnapshot.
	rules = dataMaskingResolveStepNamespaces(rules, outputData)

	maskedCount := 0
	maskedFields := []string{}

	// Pre-compute the "message" wrapper map once so both Get and Update use the
	// same target. In a multi-step pipeline executeStepWithContext places the
	// parsed HL7 data at inputData["message"], so resolveHL7FieldValue must look
	// ONE level in. GetFieldValue(outputData, path) already does this internally,
	// but when a prior step (e.g. HL7→FHIR transform) has merged its own output
	// into execCtx.Message the internal lookup path can be shadowed. Passing the
	// inner map directly as the root bypasses any such interference and is
	// guaranteed to be structurally equivalent.
	innerMsg, hasInnerMsg := outputData["message"].(map[string]interface{})

	for _, rule := range rules {
		if rule.MaskChar == "" {
			rule.MaskChar = "*"
		}

		// First try the top-level outputData (handles fields already at root,
		// direct enhancedSegments, or any FHIR/JSON paths).
		val := executors.GetFieldValue(outputData, rule.Field)
		targetData := outputData

		// Fallback: pass the inner "message" map directly as root.
		// This is required when the HL7 enhancedSegments live at
		// outputData["message"]["enhancedSegments"] but the top-level lookup
		// returns nil due to intermediate step output merges in the pipeline.
		if val == nil && hasInnerMsg {
			val = executors.GetFieldValue(innerMsg, rule.Field)
			if val != nil {
				targetData = innerMsg
			}
		}

		// FHIR bundle entry fallback: the user-configured (or GetOutputVariables-declared)
		// path may contain entry[N] with a hardcoded index that does not match the actual
		// runtime position. Search all entries in the bundle array for the first match.
		if val == nil {
			if resolvedPath, resolvedData, resolvedVal := dataMaskingFindInBundleEntries(outputData, innerMsg, rule.Field); resolvedVal != nil {
				val = resolvedVal
				targetData = resolvedData
				rule.Field = resolvedPath
			}
		}

		if val == nil {
			log.Printf("  🔍 [DataMask] Field NOT found: %s", rule.Field)
			continue // field not present in this message — skip silently
		}

		masked := e.applyMasking(fmt.Sprintf("%v", val), rule, config.PreserveFormat)
		if executors.UpdateFieldValue(targetData, rule.Field, masked) {
			maskedCount++
			maskedFields = append(maskedFields, rule.Field)
		}
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"masked_count":  maskedCount,
		"masked_fields": maskedFields,
		"total_rules":   len(rules),
	}
	details := map[string]interface{}{
		"duration_ms":   durationMs,
		"success":       true,
		"masked_count":  maskedCount,
		"total_rules":   len(rules),
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Data Masking complete: %d/%d fields masked", maskedCount, len(rules))
	return outputData, nil
}

func (e *DataMaskingExecutor) applyMasking(value string, rule maskingRule, preserveFormat bool) string {
	if value == "" {
		return value
	}

	// Apply pattern filter if specified
	if rule.Pattern != "" {
		re, err := regexp.Compile(rule.Pattern)
		if err == nil {
			return re.ReplaceAllStringFunc(value, func(match string) string {
				return e.maskString(match, rule, preserveFormat)
			})
		}
	}

	return e.maskString(value, rule, preserveFormat)
}

func (e *DataMaskingExecutor) maskString(value string, rule maskingRule, preserveFormat bool) string {
	switch rule.Strategy {
	case "hash":
		salt := rule.HashSalt
		if salt == "" {
			salt = "ezHealthKonnect"
		}
		h := sha256.Sum256([]byte(salt + value))
		return hex.EncodeToString(h[:])[:16] // Return first 16 chars of hash

	case "redact":
		return "[REDACTED]"

	case "partial":
		if preserveFormat {
			// Format-preserving partial mask: mask digit characters only, keep separators in place.
			// keepFirst/keepLast apply to the digit sequence, not the full string.
			// Example: "555-867-5309" keepLast=4 → "***-***-5309"
			var digits []rune
			for _, ch := range value {
				if ch >= '0' && ch <= '9' {
					digits = append(digits, ch)
				}
			}
			nDigits := len(digits)
			// Determine digit index range to mask
			maskFrom := rule.KeepFirst
			maskUntil := nDigits - rule.KeepLast
			if maskUntil < 0 {
				maskUntil = 0
			}
			if maskFrom > nDigits {
				maskFrom = nDigits
			}
			maskCh := rune('*')
			if len(rule.MaskChar) > 0 {
				maskCh = []rune(rule.MaskChar)[0]
			}
			digitIdx := 0
			var result strings.Builder
			for _, ch := range value {
				if ch >= '0' && ch <= '9' {
					if digitIdx >= maskFrom && digitIdx < maskUntil {
						result.WriteRune(maskCh)
					} else {
						result.WriteRune(ch)
					}
					digitIdx++
				} else {
					result.WriteRune(ch) // preserve separators
				}
			}
			return result.String()
		}
		if len(value) <= rule.KeepFirst+rule.KeepLast {
			return strings.Repeat(rule.MaskChar, len(value))
		}
		masked := value[:rule.KeepFirst]
		masked += strings.Repeat(rule.MaskChar, len(value)-rule.KeepFirst-rule.KeepLast)
		masked += value[len(value)-rule.KeepLast:]
		return masked

	case "tokenize":
		h := sha256.Sum256([]byte(value))
		return "TOK-" + hex.EncodeToString(h[:])[:12]

	case "substitute":
		return e.generateSubstituteValue(value, rule)

	default: // "mask"
		return strings.Repeat(rule.MaskChar, len(value))
	}
}

// substituteFirstNames is a pool of gender-neutral first names used by the substitute strategy.
// Values are selected deterministically via SHA-256 of the original field value.
var substituteFirstNames = []string{
	"Alex", "Blake", "Casey", "Dana", "Emery", "Finley", "Gray", "Harper",
	"Indigo", "Jordan", "Kendall", "Lane", "Morgan", "Noel", "Oakley",
	"Parker", "Quinn", "Riley", "Sage", "Taylor", "Umber", "Val",
	"Winter", "Xen", "Yael", "Zephyr",
}

// substituteLastNames is a pool of common last names used by the substitute strategy.
var substituteLastNames = []string{
	"Adams", "Baker", "Chen", "Davis", "Evans", "Foster", "Garcia",
	"Hill", "Ingram", "Johnson", "Kim", "Lee", "Martinez", "Nelson",
	"Ortiz", "Patel", "Quinn", "Rivera", "Smith", "Taylor", "Upton",
	"Vega", "Walker", "Xiong", "Young", "Zhang",
}

// substituteStreetNames is a pool of clearly fictional street names used by the substitute strategy.
var substituteStreetNames = []string{
	"Test Street", "Sample Avenue", "Example Drive", "Demo Lane",
	"Placeholder Road", "Mock Boulevard", "Dummy Court", "Staging Way",
	"Development Path", "Fictional Circle",
}

// generateSubstituteValue returns a deterministic, realistic-looking fake value for the
// given original input. The output is reproducible: the same input always produces the
// same substitute, which preserves referential integrity across a test dataset.
//
// Safe formats used for each type:
//   - name:    gender-neutral pool; no real person
//   - ssn:     9XX-XX-XXXX  (ITIN range — never a valid US SSN)
//   - phone:   (555) XXX-XXXX  (555 exchange is fictional by convention)
//   - email:   test.{8hexchars}@example-test.com  (.com is reserved for testing by convention)
//   - date:    preserve year (if detectable), randomise month/day
//   - address: 1000-9999 + fictional street name
//   - custom:  rule.SubstituteValue or "[TEST DATA]"
func (e *DataMaskingExecutor) generateSubstituteValue(value string, rule maskingRule) string {
	// Custom type: return the fixed replacement string without any hashing
	if rule.SubstituteType == "custom" {
		if rule.SubstituteValue != "" {
			return rule.SubstituteValue
		}
		return "[TEST DATA]"
	}

	// All other types: use SHA-256 of the original value as a deterministic seed
	h := sha256.Sum256([]byte(value))

	switch rule.SubstituteType {
	case "name":
		first := substituteFirstNames[int(h[0])%len(substituteFirstNames)]
		last := substituteLastNames[int(h[1])%len(substituteLastNames)]
		return first + " " + last

	case "ssn":
		// Format: 9XX-XX-XXXX — the leading 9 puts this in the ITIN range (900-999),
		// which the SSA has never issued as regular SSNs, so it can never be a real SSN.
		return fmt.Sprintf("9%02d-%02d-%02d%02d",
			int(h[0])%100,
			int(h[1])%100,
			int(h[2])%100,
			int(h[3])%100)

	case "phone":
		// Format: (555) XXX-XXXX — the 555 exchange is reserved for fictional use
		// in North American media and is safe to use in test data.
		return fmt.Sprintf("(555) %d%d%d-%d%d%d%d",
			int(h[0])%10, int(h[1])%10, int(h[2])%10,
			int(h[3])%10, int(h[4])%10, int(h[5])%10, int(h[6])%10)

	case "email":
		// example-test.com is not a real domain; 8 hex chars give 4 billion possible addresses
		return "test." + hex.EncodeToString(h[:4]) + "@example-test.com"

	case "date":
		// Preserve the year (if the original starts with 4 digits that look like a year)
		// so that age-based analytics remain plausible; randomise month and day.
		year := "1970"
		if len(value) >= 4 {
			candidate := value[:4]
			isYear := true
			for _, c := range candidate {
				if c < '0' || c > '9' {
					isYear = false
					break
				}
			}
			if isYear {
				year = candidate
			}
		}
		month := int(h[0])%12 + 1  // 1–12
		day := int(h[1])%28 + 1    // 1–28 (safe for all months including February)
		return fmt.Sprintf("%s-%02d-%02d", year, month, day)

	case "address":
		// House number 1000–9999 (avoids low single-digit numbers that could match real addresses)
		num := 1000 + (int(h[0])*14+int(h[1]))%9000
		street := substituteStreetNames[int(h[2])%len(substituteStreetNames)]
		return fmt.Sprintf("%d %s", num, street)

	default:
		return "[TEST DATA]"
	}
}

// entryIndexPattern matches a field path that contains "entry[N]".
// Capture groups: (1) path up to and including "entry", (2) suffix after "[N]".
var entryIndexPattern = regexp.MustCompile(`^(.*\bentry)\[\d+\](.*)$`)

// dataMaskingFindInBundleEntries searches all entries of a FHIR bundle when a path
// containing entry[N] returns nil. It tries indices 0…len(entry)-1 and returns
// (resolvedPath, targetData, value) for the first index that yields a non-nil value.
//
// This handles the common case where GetOutputVariables() (or a user rule) declares
// "fhir_bundle.entry[0].resource.patient.name[0].family" but the runtime FHIR bundle
// places the Patient resource at entry[1] (entry[0] being metadata / an empty slot).
func dataMaskingFindInBundleEntries(outputData, innerMsg map[string]interface{}, path string) (string, map[string]interface{}, interface{}) {
	match := entryIndexPattern.FindStringSubmatch(path)
	if match == nil {
		return "", nil, nil // path does not contain entry[N] — nothing to search
	}
	arrayPrefix := match[1] // e.g. "steps.ns.step_output.fhir_bundle.entry"
	subSuffix := match[2]   // e.g. ".resource.patient.name[0].family"

	roots := []map[string]interface{}{outputData}
	if innerMsg != nil {
		roots = append(roots, innerMsg)
	}
	for _, root := range roots {
		arr, ok := executors.GetFieldValue(root, arrayPrefix).([]interface{})
		if !ok || len(arr) == 0 {
			continue
		}
		for i := range arr {
			candidate := fmt.Sprintf("%s[%d]%s", arrayPrefix, i, subSuffix)
			if v := executors.GetFieldValue(root, candidate); v != nil {
				log.Printf("  🔧 [DataMask] entry[*] fallback resolved %s → %s", path, candidate)
				return candidate, root, v
			}
		}
	}
	return "", nil, nil
}

// dataMaskingResolveStepNamespaces rewrites rule.Field paths of the form
// "steps.{baseName}.rest" → "steps.{baseName}_{shortID}.rest" by doing a
// prefix match against the stepsSnapshot keyed in outputData["steps"].
//
// Background: executeStepWithContext stores prior step outputs in a snapshot map
// whose keys are the full generated namespaces "{baseName}_{shortID}" (first 6
// chars of the step UUID). The UI variable picker and manual user entry typically
// use the base name without the suffix. This function bridges that gap so masking
// rules referencing "steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0]…"
// correctly find the value at "steps.hl7_fhir_transform_a1b2c3.step_output.…".
func dataMaskingResolveStepNamespaces(rules []maskingRule, outputData map[string]interface{}) []maskingRule {
	stepsMap, ok := outputData["steps"].(map[string]interface{})
	if !ok || len(stepsMap) == 0 {
		log.Printf("  🔍 [DataMask] No steps snapshot (ok=%v, len=%d)", ok, len(stepsMap))
		return rules // no snapshot to resolve against
	}
	// Debug: log all available namespace keys
	nsKeys := make([]string, 0, len(stepsMap))
	for k := range stepsMap { nsKeys = append(nsKeys, k) }
	log.Printf("  🔍 [DataMask] Available step namespaces: %v", nsKeys)

	resolved := make([]maskingRule, len(rules))
	copy(resolved, rules)

	for i, rule := range resolved {
		if !strings.HasPrefix(rule.Field, "steps.") {
			continue
		}
		// Extract the namespace token — the part between "steps." and the next "."
		rest := rule.Field[len("steps."):]
		dotIdx := strings.Index(rest, ".")
		if dotIdx < 0 {
			continue // malformed — no sub-path after namespace
		}
		baseName := rest[:dotIdx]
		subPath := rest[dotIdx:] // includes leading "."

		// Exact match — nothing to do
		if _, exists := stepsMap[baseName]; exists {
			continue
		}

		// Prefix match: find any namespace key that starts with "{baseName}_"
		prefix := baseName + "_"
		for fullNs := range stepsMap {
			if strings.HasPrefix(fullNs, prefix) {
				resolved[i].Field = "steps." + fullNs + subPath
				log.Printf("  🔧 [DataMask] Resolved step namespace: %s → %s", baseName, fullNs)
				break
			}
		}
	}

	return resolved
}

// dataMaskingConvertTypedSegments converts typed hl7.EnhancedSegment maps to
// generic map[string]interface{} in-place so that GetFieldValue / UpdateFieldValue
// can traverse and mutate HL7 fields. Called once per Execute() before masking.
func dataMaskingConvertTypedSegments(data map[string]interface{}) {
	convert := func(container map[string]interface{}) {
		if typed, ok := container["enhancedSegments"].(map[string]hl7.EnhancedSegment); ok {
			if b, err := json.Marshal(typed); err == nil {
				var generic map[string]interface{}
				if json.Unmarshal(b, &generic) == nil {
					container["enhancedSegments"] = generic
				}
			}
		}
	}
	convert(data)
	if msg, ok := data["message"].(map[string]interface{}); ok {
		convert(msg)
	}
}

func (e *DataMaskingExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("data masking requires config with rules or maskAllPHI")
	}
	return nil
}

// GetOutputVariables implements the VariableProvider interface.
// Returns statically-known output paths for IntelliSense and the Variables tab.
// Paths follow the steps.{namespace}.step_output.{field} convention.
func (e *DataMaskingExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	ns := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	base := "steps." + ns + ".step_output"

	return []models.VariableDefinition{
		{
			Name:        "masked_count",
			Path:        base + ".masked_count",
			DataType:    "integer",
			Description: "Number of fields successfully masked",
			Category:    "Data Masking",
		},
		{
			Name:        "masked_fields",
			Path:        base + ".masked_fields",
			DataType:    "array",
			Description: "Ordered list of field paths that were masked (e.g. [\"PID.5\", \"PID.19\"])",
			Category:    "Data Masking",
		},
		{
			Name:        "total_rules",
			Path:        base + ".total_rules",
			DataType:    "integer",
			Description: "Total number of masking rules evaluated (includes maskAllPHI rules when enabled)",
			Category:    "Data Masking",
		},
	}
}

// GetConfigSchema implements the Describable interface.
// Returns a JSON Schema for the dataMaskingConfig struct to enable UI validation.
func (e *DataMaskingExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"rules": map[string]interface{}{
				"type":        "array",
				"description": "Ordered list of field masking rules",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"field":    map[string]interface{}{"type": "string", "description": "Field path (HL7, FHIR, or JSON dot notation)"},
						"strategy":        map[string]interface{}{"type": "string", "enum": []string{"mask", "redact", "partial", "hash", "tokenize", "substitute"}},
						"maskChar": map[string]interface{}{"type": "string", "description": "Replacement character (default: *)"},
						"keepFirst": map[string]interface{}{"type": "integer", "description": "Chars to preserve from the start (partial strategy)"},
						"keepLast":  map[string]interface{}{"type": "integer", "description": "Chars to preserve from the end (partial strategy)"},
						"hashSalt":  map[string]interface{}{"type": "string", "description": "Salt prepended before SHA-256 hash"},
						"pattern":         map[string]interface{}{"type": "string", "description": "Optional regex — only matching substrings are masked"},
					"substituteType":  map[string]interface{}{"type": "string", "enum": []string{"name", "ssn", "phone", "email", "date", "address", "custom"}, "description": "Category of realistic fake data to generate (substitute strategy)"},
					"substituteValue": map[string]interface{}{"type": "string", "description": "Fixed replacement string when substituteType=custom"},
					},
					"required": []string{"field", "strategy"},
				},
			},
			"maskAllPHI":       map[string]interface{}{"type": "boolean", "description": "Auto-apply pre-configured HIPAA PHI rules for the selected data format"},
			"maskAllPHIFormat": map[string]interface{}{"type": "string", "enum": []string{"hl7v2", "fhir", "json"}, "description": "Data format for auto-PHI rules: 'hl7v2' (default), 'fhir', or 'json'"},
			"preserveFormat":   map[string]interface{}{"type": "boolean", "description": "For partial strategy: mask digits only, preserve separators (e.g. 555-**-5309)"},
		},
	}
}

// GetConfigExample implements the Describable interface.
// Returns a representative config example used by the documentation panel.
func (e *DataMaskingExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"field": "PID.5", "strategy": "mask"},
			map[string]interface{}{"field": "PID.19", "strategy": "hash", "hashSalt": "mySecret"},
			map[string]interface{}{"field": "PID.3", "strategy": "partial", "keepFirst": 0, "keepLast": 4},
		},
		"maskAllPHI":     false,
		"preserveFormat": false,
	}
}
