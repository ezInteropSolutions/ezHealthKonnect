package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"hash/fnv"
	"log"
	"sort"
	"strings"
	"time"
)

// RemoveDuplicatesExecutor removes duplicate records from an array field using
// configurable key fields, merge strategies, and null-key handling.
//
// Key concepts:
//   - sourceField: dot-path to the array to deduplicate (e.g. "enriched.results")
//   - keyFields:   which record fields form the dedup key; empty = full-record SHA-256 hash
//   - strategy:    what to do when two records share the same key
//       "first"   (default) keep the first occurrence, discard later ones
//       "last"    keep the last occurrence (overwrites first in place)
//       "merge"   keep the first record and copy any fields present in later
//                 records that are absent in the first (non-destructive merge)
//   - caseSensitive: when false, key field values are lowercased before hashing
//   - outputField: where to write the deduplicated array; empty = update sourceField in-place
//   - nullKeyBehavior: how to handle records where one or more key fields are missing/null
//       "group"  (default) treat all null-key records as one group and apply strategy
//       "keep"   always keep every null-key record (bypass dedup)
//       "remove" drop all null-key records entirely
type RemoveDuplicatesExecutor struct {
	*executors.BaseExecutor
}

func NewRemoveDuplicatesExecutor() *RemoveDuplicatesExecutor {
	return &RemoveDuplicatesExecutor{
		BaseExecutor: executors.NewBaseExecutor("remove_duplicates", models.ExecutorMetadata{
			Name:        "Remove Duplicates",
			Description: "Removes duplicate records from arrays using configurable key fields and merge strategies",
			Version:     "2.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Data Transform",
		}),
	}
}

type removeDuplicatesConfig struct {
	// Required
	SourceField string `json:"sourceField"` // dot-path to source array

	// Dedup key
	KeyFields     []string `json:"keyFields"`     // fields forming the key; empty = full hash
	CaseSensitive bool     `json:"caseSensitive"` // default true

	// Strategy on duplicate found
	Strategy string `json:"strategy"` // "first" | "last" | "merge"

	// Output destination — defaults to in-place (sourceField)
	OutputField string `json:"outputField"` // optional: write result to a different field

	// Null / missing key handling
	NullKeyBehavior string `json:"nullKeyBehavior"` // "group" | "keep" | "remove"

	// Large dataset safety: max records before aborting (0 = default 1M; hard cap 10M)
	MaxInputRecords int `json:"maxInputRecords"`
}

// nullKeyMarker is the sentinel hash value used when nullKeyBehavior == "group".
// All records with missing/null key fields share this single bucket so that the
// configured strategy (first/last/merge) is applied among them.
// Value is FNV-64a hash of the literal string "__NULL_KEY__".
const nullKeyMarker uint64 = 0xB8E7_4CE2_3E93_D0D7

// Large dataset limits applied when MaxInputRecords == 0 (default).
const (
	defaultMaxInputRecords = 1_000_000
	hardCapInputRecords    = 10_000_000
)

func (e *RemoveDuplicatesExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	// Defaults
	config := removeDuplicatesConfig{
		Strategy:        "first",
		CaseSensitive:   true,
		NullKeyBehavior: "group",
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config) //nolint:errcheck
	}

	if config.SourceField == "" {
		return nil, fmt.Errorf("remove_duplicates requires sourceField in config")
	}

	// Apply MaxInputRecords guard — prevent OOM on very large datasets
	maxIn := config.MaxInputRecords
	if maxIn <= 0 {
		maxIn = defaultMaxInputRecords
	}
	if maxIn > hardCapInputRecords {
		maxIn = hardCapInputRecords
	}

	log.Printf("  🔍 Remove Duplicates: source=%s, keys=%v, strategy=%s, nullKey=%s",
		config.SourceField, config.KeyFields, config.Strategy, config.NullKeyBehavior)

	// Shallow-copy input so we don't mutate the caller's map
	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}

	// ── Resolve source array ─────────────────────────────────────
	rawSource := executors.GetFieldValue(outputData, config.SourceField)
	if rawSource == nil {
		// Fallback: look inside top-level "message" key
		if msg, ok := outputData["message"].(map[string]interface{}); ok {
			rawSource = executors.GetFieldValue(msg, config.SourceField)
		}
	}

	var sourceData []interface{}
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
			return nil, fmt.Errorf("sourceField '%s' is not an array (got %T)", config.SourceField, rawSource)
		}
		log.Printf("  ⚠️  Remove Duplicates: sourceField '%s' not found or empty — no-op", config.SourceField)
		return inputData, nil
	}

	originalCount := len(sourceData)

	if originalCount > maxIn {
		return nil, fmt.Errorf("remove_duplicates: input has %d records, exceeds maxInputRecords limit of %d", originalCount, maxIn)
	}

	// ── Deduplicate ──────────────────────────────────────────────
	seen := make(map[uint64]int) // hash → index in deduplicated slice
	var deduplicated []interface{}
	nullKeyKept := 0
	nullKeyRemoved := 0

	for _, item := range sourceData {
		hash, hasNullKey := e.computeHash(item, config.KeyFields, config.CaseSensitive)

		// Handle records with missing/null key fields
		if hasNullKey && len(config.KeyFields) > 0 {
			switch config.NullKeyBehavior {
			case "keep":
				// Always keep — bypass dedup logic entirely
				deduplicated = append(deduplicated, item)
				nullKeyKept++
				continue
			case "remove":
				// Drop silently
				nullKeyRemoved++
				continue
			default: // "group" — fall through; nullKeyMarker makes them one bucket
			}
		}

		existingIdx, exists := seen[hash]
		if !exists {
			seen[hash] = len(deduplicated)
			deduplicated = append(deduplicated, item)
			continue
		}

		// Duplicate found — apply strategy
		switch config.Strategy {
		case "last":
			deduplicated[existingIdx] = item

		case "merge":
			if existing, ok := deduplicated[existingIdx].(map[string]interface{}); ok {
				if newItem, ok := item.(map[string]interface{}); ok {
					for k, v := range newItem {
						if _, alreadySet := existing[k]; !alreadySet {
							existing[k] = v
						}
					}
				}
			}
		// "first": do nothing — keep existing
		}
	}

	removedCount := originalCount - len(deduplicated) - nullKeyRemoved

	// ── Write result ─────────────────────────────────────────────
	dest := config.SourceField
	if config.OutputField != "" {
		dest = config.OutputField
	}
	executors.UpdateFieldValue(outputData, dest, deduplicated)

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"original_count":    originalCount,
		"dedup_count":       len(deduplicated),
		"removed_count":     removedCount,
		"null_key_kept":     nullKeyKept,
		"null_key_removed":  nullKeyRemoved,
	}
	details := map[string]interface{}{
		"duration_ms":      durationMs,
		"success":          true,
		"original_count":   originalCount,
		"dedup_count":      len(deduplicated),
		"removed_count":    removedCount,
		"null_key_kept":    nullKeyKept,
		"null_key_removed": nullKeyRemoved,
		"strategy":         config.Strategy,
		"key_fields":       config.KeyFields,
		"null_key_behavior": config.NullKeyBehavior,
		"output_field":     dest,
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Remove Duplicates: %d → %d records (%d dupes removed, %d null-key handled)",
		originalCount, len(deduplicated), removedCount, nullKeyKept+nullKeyRemoved)

	return outputData, nil
}

// computeHash returns the dedup key hash for an item and a flag indicating whether
// any of the requested key fields were missing or null in the record.
// When keyFields is empty, the entire record is hashed (full-record dedup).
// Uses FNV-64a for speed (~5× faster than SHA-256) and memory efficiency (8 bytes vs 32+).
func (e *RemoveDuplicatesExecutor) computeHash(item interface{}, keyFields []string, caseSensitive bool) (hash uint64, hasNullKey bool) {
	h := fnv.New64a()

	if len(keyFields) == 0 {
		data, _ := json.Marshal(item)
		h.Write(data)
		return h.Sum64(), false
	}

	itemMap, ok := item.(map[string]interface{})
	if !ok {
		// Not a map — treat entire item as the key
		data, _ := json.Marshal(item)
		h.Write(data)
		return h.Sum64(), false
	}

	var parts []string
	for _, field := range keyFields {
		val := executors.GetFieldValue(itemMap, field)
		if val == nil {
			hasNullKey = true
			continue
		}
		s := fmt.Sprintf("%v", val)
		if !caseSensitive {
			s = strings.ToLower(s)
		}
		parts = append(parts, field+"="+s)
	}

	if hasNullKey {
		// Return sentinel so null-key records can still be grouped together
		return nullKeyMarker, true
	}

	sort.Strings(parts)
	h.Write([]byte(strings.Join(parts, "|")))
	return h.Sum64(), false
}

// GetOutputVariables returns the variables this executor will produce.
// Implements the VariableProvider interface — works at config time, no test run needed.
func (e *RemoveDuplicatesExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	// Parse config to find output field
	config := removeDuplicatesConfig{
		Strategy:        "first",
		CaseSensitive:   true,
		NullKeyBehavior: "group",
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config) //nolint:errcheck
	}

	// Compute step namespace (same logic the pipeline service uses at runtime)
	namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	statsBase := "steps." + namespace + ".step_output"

	// Stat variables — always emitted via SetStepOutputWithDetails
	statFields := []struct {
		name string
		desc string
	}{
		{"original_count", "Total records before deduplication"},
		{"dedup_count", "Records remaining after deduplication"},
		{"removed_count", "Duplicate records that were removed"},
		{"null_key_kept", "Records with null key fields kept (nullKeyBehavior=keep)"},
		{"null_key_removed", "Records with null key fields dropped (nullKeyBehavior=remove)"},
	}
	for _, sf := range statFields {
		variables = append(variables, models.VariableDefinition{
			Name:        sf.name,
			Path:        statsBase + "." + sf.name,
			DataType:    "integer",
			Description: sf.desc,
			Category:    "Data Transform",
		})
	}

	// Output array — written to outputField (falls back to sourceField)
	outputArrayPath := config.SourceField
	if config.OutputField != "" {
		outputArrayPath = config.OutputField
	}
	if outputArrayPath != "" {
		variables = append(variables, models.VariableDefinition{
			Name:        "output_array",
			Path:        outputArrayPath,
			DataType:    "array",
			Description: fmt.Sprintf("Deduplicated records written to '%s'", outputArrayPath),
			Category:    "Data Transform",
		})
	}

	return variables
}

func (e *RemoveDuplicatesExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("remove_duplicates requires config with sourceField")
	}
	src, ok := step.Config["sourceField"].(string)
	if !ok || src == "" {
		return fmt.Errorf("remove_duplicates requires sourceField in config")
	}
	if strategy, ok := step.Config["strategy"].(string); ok {
		switch strategy {
		case "", "first", "last", "merge":
			// valid
		default:
			return fmt.Errorf("remove_duplicates: invalid strategy %q (must be first, last, or merge)", strategy)
		}
	}
	if nullKey, ok := step.Config["nullKeyBehavior"].(string); ok {
		switch nullKey {
		case "", "group", "keep", "remove":
			// valid
		default:
			return fmt.Errorf("remove_duplicates: invalid nullKeyBehavior %q (must be group, keep, or remove)", nullKey)
		}
	}
	return nil
}
