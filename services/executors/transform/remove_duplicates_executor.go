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
	"sort"
	"strings"
	"time"
)

// RemoveDuplicatesExecutor removes duplicate records from datasets/collections
type RemoveDuplicatesExecutor struct {
	*executors.BaseExecutor
}

func NewRemoveDuplicatesExecutor() *RemoveDuplicatesExecutor {
	return &RemoveDuplicatesExecutor{
		BaseExecutor: executors.NewBaseExecutor("remove_duplicates", models.ExecutorMetadata{
			Name:        "Remove Duplicates",
			Description: "Removes duplicate records from datasets based on key fields",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Data Transform",
		}),
	}
}

type removeDuplicatesConfig struct {
	SourceField  string   `json:"sourceField"`  // Path to the array/collection (e.g., "enriched.results", "items")
	KeyFields    []string `json:"keyFields"`    // Fields to use as dedup key (empty = full record hash)
	Strategy     string   `json:"strategy"`     // "first" (keep first), "last" (keep last), "merge" (merge records)
	OutputField  string   `json:"outputField"`  // Where to store deduplicated data (default: same as source)
	CaseSensitive bool   `json:"caseSensitive"` // Case-sensitive comparison (default: true)
}

func (e *RemoveDuplicatesExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := removeDuplicatesConfig{
		Strategy:      "first",
		CaseSensitive: true,
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	if config.SourceField == "" {
		return nil, fmt.Errorf("remove_duplicates requires sourceField in config")
	}
	if config.OutputField == "" {
		config.OutputField = config.SourceField
	}

	log.Printf("  🔍 Remove Duplicates: source=%s, keys=%v, strategy=%s",
		config.SourceField, config.KeyFields, config.Strategy)

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Find the source collection
	var sourceData []interface{}
	rawSource := executors.GetFieldValue(outputData, config.SourceField)
	if rawSource == nil {
		if msg, ok := outputData["message"].(map[string]interface{}); ok {
			rawSource = executors.GetFieldValue(msg, config.SourceField)
		}
	}

	switch v := rawSource.(type) {
	case []interface{}:
		sourceData = v
	case []map[string]interface{}:
		sourceData = make([]interface{}, len(v))
		for i, item := range v {
			sourceData[i] = item
		}
	default:
		if rawSource != nil {
			return nil, fmt.Errorf("sourceField '%s' is not an array (type: %T)", config.SourceField, rawSource)
		}
		log.Printf("  ⚠️ Remove Duplicates: sourceField '%s' not found or empty", config.SourceField)
		return inputData, nil
	}

	originalCount := len(sourceData)

	// Deduplicate
	seen := make(map[string]int) // hash -> first index
	var deduplicated []interface{}

	for i, item := range sourceData {
		hash := e.computeHash(item, config.KeyFields, config.CaseSensitive)
		if existingIdx, exists := seen[hash]; exists {
			if config.Strategy == "last" {
				// Replace with latest
				deduplicated[existingIdx] = item
			} else if config.Strategy == "merge" {
				// Merge fields
				if existing, ok := deduplicated[existingIdx].(map[string]interface{}); ok {
					if newItem, ok := item.(map[string]interface{}); ok {
						for k, v := range newItem {
							if _, exists := existing[k]; !exists {
								existing[k] = v
							}
						}
					}
				}
			}
			// "first" strategy: skip duplicate (do nothing)
			continue
		}
		seen[hash] = len(deduplicated)
		_ = i
		deduplicated = append(deduplicated, item)
	}

	removedCount := originalCount - len(deduplicated)

	// Store deduplicated data
	executors.UpdateFieldValue(outputData, config.OutputField, deduplicated)

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"original_count": originalCount,
		"dedup_count":    len(deduplicated),
		"removed_count":  removedCount,
	}
	details := map[string]interface{}{
		"duration_ms":    durationMs,
		"success":        true,
		"original_count": originalCount,
		"dedup_count":    len(deduplicated),
		"removed_count":  removedCount,
		"strategy":       config.Strategy,
		"key_fields":     config.KeyFields,
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Remove Duplicates: %d → %d records (%d removed)",
		originalCount, len(deduplicated), removedCount)

	return outputData, nil
}

func (e *RemoveDuplicatesExecutor) computeHash(item interface{}, keyFields []string, caseSensitive bool) string {
	if len(keyFields) == 0 {
		// Hash entire record
		data, _ := json.Marshal(item)
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:])
	}

	// Hash specific key fields
	itemMap, ok := item.(map[string]interface{})
	if !ok {
		data, _ := json.Marshal(item)
		h := sha256.Sum256(data)
		return hex.EncodeToString(h[:])
	}

	var parts []string
	for _, field := range keyFields {
		val := executors.GetFieldValue(itemMap, field)
		s := fmt.Sprintf("%v", val)
		if !caseSensitive {
			s = strings.ToLower(s)
		}
		parts = append(parts, field+"="+s)
	}
	sort.Strings(parts)
	combined := strings.Join(parts, "|")
	h := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(h[:])
}

func (e *RemoveDuplicatesExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("remove_duplicates requires config with sourceField")
	}
	if src, ok := step.Config["sourceField"].(string); !ok || src == "" {
		return fmt.Errorf("remove_duplicates requires sourceField in config")
	}
	return nil
}
