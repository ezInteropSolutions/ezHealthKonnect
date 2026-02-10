package transform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Field      string `json:"field"`      // Field path (e.g., PID.3, Patient.name)
	Strategy   string `json:"strategy"`   // mask, hash, redact, partial, tokenize
	MaskChar   string `json:"maskChar"`   // Character for masking (default: *)
	KeepFirst  int    `json:"keepFirst"`  // For partial: keep first N chars
	KeepLast   int    `json:"keepLast"`   // For partial: keep last N chars
	HashSalt   string `json:"hashSalt"`   // Salt for hashing
	Pattern    string `json:"pattern"`    // Regex pattern for selective masking
}

type dataMaskingConfig struct {
	Rules           []maskingRule `json:"rules"`
	MaskAllPHI      bool          `json:"maskAllPHI"`      // Auto-mask common PHI fields
	PreserveFormat  bool          `json:"preserveFormat"`  // Keep format (e.g., ###-##-#### for SSN)
}

// Common PHI fields that should be masked for HIPAA compliance
var commonPHIFields = []maskingRule{
	{Field: "PID.3", Strategy: "partial", KeepFirst: 0, KeepLast: 4},   // Patient ID - keep last 4
	{Field: "PID.5", Strategy: "mask"},                                    // Patient Name
	{Field: "PID.7", Strategy: "mask"},                                    // DOB
	{Field: "PID.13", Strategy: "partial", KeepFirst: 0, KeepLast: 4},   // Phone
	{Field: "PID.19", Strategy: "hash"},                                   // SSN
	{Field: "PID.18", Strategy: "partial", KeepFirst: 0, KeepLast: 4},   // Account Number
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

	// Build rule set
	rules := config.Rules
	if config.MaskAllPHI {
		rules = append(commonPHIFields, rules...)
	}

	if len(rules) == 0 {
		log.Printf("  ⚠️ Data Masking: no rules configured")
		return inputData, nil
	}

	log.Printf("  🔒 Data Masking starting: %d rules", len(rules))

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	maskedCount := 0
	maskedFields := []string{}

	for _, rule := range rules {
		if rule.MaskChar == "" {
			rule.MaskChar = "*"
		}

		// Get current value
		value := executors.GetFieldValue(outputData, rule.Field)
		if value == nil {
			// Try in message wrapper
			if msg, ok := outputData["message"].(map[string]interface{}); ok {
				value = executors.GetFieldValue(msg, rule.Field)
				if value != nil {
					maskedValue := e.applyMasking(fmt.Sprintf("%v", value), rule)
					executors.UpdateFieldValue(msg, rule.Field, maskedValue)
					maskedCount++
					maskedFields = append(maskedFields, rule.Field)
					continue
				}
			}
			continue
		}

		maskedValue := e.applyMasking(fmt.Sprintf("%v", value), rule)
		executors.UpdateFieldValue(outputData, rule.Field, maskedValue)
		maskedCount++
		maskedFields = append(maskedFields, rule.Field)
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

	log.Printf("  ✅ Data Masking complete: %d fields masked", maskedCount)
	return outputData, nil
}

func (e *DataMaskingExecutor) applyMasking(value string, rule maskingRule) string {
	if value == "" {
		return value
	}

	// Apply pattern filter if specified
	if rule.Pattern != "" {
		re, err := regexp.Compile(rule.Pattern)
		if err == nil {
			return re.ReplaceAllStringFunc(value, func(match string) string {
				return e.maskString(match, rule)
			})
		}
	}

	return e.maskString(value, rule)
}

func (e *DataMaskingExecutor) maskString(value string, rule maskingRule) string {
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

	default: // "mask"
		return strings.Repeat(rule.MaskChar, len(value))
	}
}

func (e *DataMaskingExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("data masking requires config with rules or maskAllPHI")
	}
	return nil
}
