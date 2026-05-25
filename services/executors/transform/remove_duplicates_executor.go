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

// ─────────────────────────────────────────────────────────────────────────────
// Strategy pattern — what to do when two records share the same dedup key
// ─────────────────────────────────────────────────────────────────────────────

// dedupStrategy determines which record survives when two share the same key.
type dedupStrategy interface {
	// apply selects or merges the record to retain when a duplicate is found.
	// existing is the record already in the output slice.
	// incoming is the newer record that shares the same key.
	apply(existing, incoming interface{}) interface{}
}

// firstStrategy keeps the earliest occurrence; all later duplicates are discarded.
type firstStrategy struct{}

func (firstStrategy) apply(existing, _ interface{}) interface{} { return existing }

// lastStrategy replaces the retained record with the most recent occurrence.
type lastStrategy struct{}

func (lastStrategy) apply(_, incoming interface{}) interface{} { return incoming }

// mergeStrategy keeps the first record and backfills any fields that are absent
// in it from later occurrences — non-destructive, in-place mutation (no allocation).
type mergeStrategy struct{}

func (mergeStrategy) apply(existing, incoming interface{}) interface{} {
	existMap, ok1 := existing.(map[string]interface{})
	inMap, ok2 := incoming.(map[string]interface{})
	if !ok1 || !ok2 {
		return existing
	}
	for k, v := range inMap {
		if _, alreadySet := existMap[k]; !alreadySet {
			existMap[k] = v
		}
	}
	return existing // same pointer, mutated in place
}

func newDedupStrategy(name string) dedupStrategy {
	switch name {
	case "last":
		return lastStrategy{}
	case "merge":
		return mergeStrategy{}
	default:
		return firstStrategy{} // "first" is the safe, predictable default
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Null-key handling — records that are missing one or more key field values
// ─────────────────────────────────────────────────────────────────────────────

type nullKeyAction int

const (
	nullKeyGroup  nullKeyAction = iota // group all null-key records; apply strategy among them
	nullKeyKeep                        // always keep every null-key record (bypass dedup)
	nullKeyRemove                      // always drop null-key records
)

func parseNullKeyAction(s string) nullKeyAction {
	switch s {
	case "keep":
		return nullKeyKeep
	case "remove":
		return nullKeyRemove
	default:
		return nullKeyGroup
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Key extractor — stable FNV-64a hash for any record
// ─────────────────────────────────────────────────────────────────────────────

// nullKeyMarker is a sentinel FNV-64a hash shared by all records that are missing
// one or more key fields when nullKeyBehavior == "group".
// Value: FNV-64a of the literal string "__NULL_KEY__".
const nullKeyMarker uint64 = 0xB8E7_4CE2_3E93_D0D7

// keyExtractor computes a deterministic hash for a record given a set of key fields.
// Empty keyFields triggers full-record dedup (hash of the entire JSON representation).
type keyExtractor struct {
	keyFields     []string
	caseSensitive bool
}

// extract returns (hash, hasNull).
// Time complexity: O(k) where k = len(keyFields).
// Memory: O(k) for the parts slice; no persistent allocation.
func (ke *keyExtractor) extract(item interface{}) (hash uint64, hasNull bool) {
	h := fnv.New64a()

	if len(ke.keyFields) == 0 {
		// Full-record dedup: hash the entire JSON representation
		data, _ := json.Marshal(item)
		h.Write(data)
		return h.Sum64(), false
	}

	itemMap, ok := item.(map[string]interface{})
	if !ok {
		// Non-map item: treat the whole value as the key
		data, _ := json.Marshal(item)
		h.Write(data)
		return h.Sum64(), false
	}

	parts := make([]string, 0, len(ke.keyFields))
	for _, field := range ke.keyFields {
		val := executors.GetFieldValue(itemMap, field)
		if val == nil {
			return nullKeyMarker, true // short-circuit: one missing field is sufficient
		}
		s := fmt.Sprintf("%v", val)
		if !ke.caseSensitive {
			s = strings.ToLower(s)
		}
		parts = append(parts, field+"="+s)
	}

	sort.Strings(parts) // order-independent: field order in config must not change the key
	h.Write([]byte(strings.Join(parts, "|")))
	return h.Sum64(), false
}

// ─────────────────────────────────────────────────────────────────────────────
// Statistics value object
// ─────────────────────────────────────────────────────────────────────────────

type dedupStats struct {
	OriginalCount  int
	DedupCount     int
	RemovedCount   int
	NullKeyKept    int
	NullKeyRemoved int
}

// ─────────────────────────────────────────────────────────────────────────────
// Dedup engine — orchestrates strategy + extractor + null-key handling
// ─────────────────────────────────────────────────────────────────────────────

type dedupEngine struct {
	strategy      dedupStrategy
	extractor     *keyExtractor
	nullKeyAction nullKeyAction
}

func newDedupEngine(cfg *removeDuplicatesConfig) *dedupEngine {
	return &dedupEngine{
		strategy:      newDedupStrategy(cfg.Strategy),
		extractor:     &keyExtractor{keyFields: cfg.KeyFields, caseSensitive: cfg.CaseSensitive},
		nullKeyAction: parseNullKeyAction(cfg.NullKeyBehavior),
	}
}

// process deduplicates input in O(n) time and O(k) space where k = unique keys.
// The input slice is never modified; a new slice is returned.
func (eng *dedupEngine) process(input []interface{}) (output []interface{}, stats dedupStats) {
	seen := make(map[uint64]int, len(input)) // hash → index in output
	output = make([]interface{}, 0, len(input))
	stats.OriginalCount = len(input)

	for _, item := range input {
		hash, hasNull := eng.extractor.extract(item)

		if hasNull && len(eng.extractor.keyFields) > 0 {
			switch eng.nullKeyAction {
			case nullKeyKeep:
				output = append(output, item)
				stats.NullKeyKept++
				continue
			case nullKeyRemove:
				stats.NullKeyRemoved++
				continue
			// nullKeyGroup: fall through — sentinel hash groups null-key records
			}
		}

		if existingIdx, exists := seen[hash]; exists {
			output[existingIdx] = eng.strategy.apply(output[existingIdx], item)
		} else {
			seen[hash] = len(output)
			output = append(output, item)
		}
	}

	stats.DedupCount = len(output)
	stats.RemovedCount = stats.OriginalCount - stats.DedupCount - stats.NullKeyRemoved
	return output, stats
}

// ─────────────────────────────────────────────────────────────────────────────
// Executor
// ─────────────────────────────────────────────────────────────────────────────

// RemoveDuplicatesExecutor removes duplicate records from an array field.
//
// # Data flow contract
//
//   - The full deduplicated array is written once to outputField (or sourceField
//     in-place). Downstream steps reference it at that exact path.
//   - step_output contains statistics and a bounded preview (≤ previewLimit rows)
//     for test-panel inspection — NOT the full array — so large datasets incur no
//     double-memory or JSON-serialisation overhead.
//   - step_output.result_path tells downstream steps exactly where the full array is.
//
// # Configuration
//
//   - sourceField:     required dot-path to the source array
//   - keyFields:       fields forming the unique key; empty = full-record hash
//   - strategy:        "first" (default) | "last" | "merge"
//   - outputField:     optional alternate destination; empty = in-place update of sourceField
//   - nullKeyBehavior: "group" (default) | "keep" | "remove"
//   - caseSensitive:   default true
//   - maxInputRecords: safety cap; 0 = 1M default; hard cap 10M
//   - previewLimit:    rows in step_output.records for test panel; 0 = 100 default; max 1000
type RemoveDuplicatesExecutor struct {
	*executors.BaseExecutor
}

func NewRemoveDuplicatesExecutor() *RemoveDuplicatesExecutor {
	return &RemoveDuplicatesExecutor{
		BaseExecutor: executors.NewBaseExecutor("remove_duplicates", models.ExecutorMetadata{
			Name:        "Remove Duplicates",
			Description: "Removes duplicate records from arrays using configurable key fields and merge strategies",
			Version:     "3.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Data Transform",
		}),
	}
}

type removeDuplicatesConfig struct {
	SourceField     string   `json:"sourceField"`     // required: dot-path to source array
	KeyFields       []string `json:"keyFields"`       // dedup key fields; empty = full-record hash
	CaseSensitive   bool     `json:"caseSensitive"`   // default true
	Strategy        string   `json:"strategy"`        // "first" | "last" | "merge"
	OutputField     string   `json:"outputField"`     // optional: write result here instead of in-place
	NullKeyBehavior string   `json:"nullKeyBehavior"` // "group" | "keep" | "remove"
	MaxInputRecords int      `json:"maxInputRecords"` // 0 = 1M default; hard cap 10M
	PreviewLimit    int      `json:"previewLimit"`    // rows in step_output.records; 0 = 100 default; max 1000
}

const (
	defaultMaxInputRecords = 1_000_000
	hardCapInputRecords    = 10_000_000
	defaultPreviewLimit    = 100
	maxPreviewCap          = 1_000
)

func (e *RemoveDuplicatesExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	cfg := removeDuplicatesConfig{
		Strategy:        "first",
		CaseSensitive:   true,
		NullKeyBehavior: "group",
	}
	if step.Config != nil {
		if b, err := json.Marshal(step.Config); err == nil {
			json.Unmarshal(b, &cfg) //nolint:errcheck
		}
	}

	if cfg.SourceField == "" {
		return nil, fmt.Errorf("remove_duplicates requires sourceField in config")
	}

	maxIn := cfg.MaxInputRecords
	if maxIn <= 0 {
		maxIn = defaultMaxInputRecords
	}
	if maxIn > hardCapInputRecords {
		maxIn = hardCapInputRecords
	}

	previewLim := cfg.PreviewLimit
	if previewLim <= 0 {
		previewLim = defaultPreviewLimit
	}
	if previewLim > maxPreviewCap {
		previewLim = maxPreviewCap
	}

	log.Printf("  🔍 Remove Duplicates: source=%s, keys=%v, strategy=%s, nullKey=%s",
		cfg.SourceField, cfg.KeyFields, cfg.Strategy, cfg.NullKeyBehavior)

	// Shallow-copy input so we don't mutate the caller's map
	outputData := make(map[string]interface{}, len(inputData))
	for k, v := range inputData {
		outputData[k] = v
	}

	// Resolve source array
	sourceData, err := e.resolveSourceArray(outputData, cfg.SourceField)
	if err != nil {
		return nil, err
	}
	if sourceData == nil {
		log.Printf("  ⚠️  Remove Duplicates: sourceField '%s' not found — no-op", cfg.SourceField)
		e.SetStepOutputWithDetails(outputData,
			map[string]interface{}{
				"result_path":    cfg.SourceField,
				"records":        []interface{}{},
				"original_count": 0,
				"dedup_count":    0,
				"removed_count":  0,
				"no_op":          true,
				"reason":         fmt.Sprintf("sourceField '%s' not found or empty", cfg.SourceField),
			},
			map[string]interface{}{"success": true, "no_op": true},
		)
		return outputData, nil
	}

	if len(sourceData) > maxIn {
		return nil, fmt.Errorf(
			"remove_duplicates: input has %d records, exceeds maxInputRecords limit of %d",
			len(sourceData), maxIn,
		)
	}

	// Run deduplication
	engine := newDedupEngine(&cfg)
	deduplicated, stats := engine.process(sourceData)

	// ── Write full result ────────────────────────────────────────────────────
	// This is the ONLY copy of the deduplicated array in the pipeline data map.
	// Downstream steps access it via this path — NOT via step_output.
	dest := cfg.SourceField
	if cfg.OutputField != "" {
		dest = cfg.OutputField
	}
	executors.UpdateFieldValue(outputData, dest, deduplicated)

	durationMs := time.Since(startTime).Milliseconds()

	// ── Build bounded preview (sub-slice header, no copy) ────────────────────
	preview, truncated := e.buildPreview(deduplicated, previewLim)

	// ── Emit step output ─────────────────────────────────────────────────────
	// step_output is small by design: stats + preview only.
	// result_path is the explicit contract for downstream steps.
	stepOutput := map[string]interface{}{
		"result_path":       dest,    // ← downstream steps: use this path for the full array
		"records":           preview, // ← test-panel inspection, ≤ previewLimit rows
		"records_truncated": truncated,
		"original_count":    stats.OriginalCount,
		"dedup_count":       stats.DedupCount,
		"removed_count":     stats.RemovedCount,
		"null_key_kept":     stats.NullKeyKept,
		"null_key_removed":  stats.NullKeyRemoved,
	}
	execDetails := map[string]interface{}{
		"duration_ms":       durationMs,
		"success":           true,
		"strategy":          cfg.Strategy,
		"key_fields":        cfg.KeyFields,
		"null_key_behavior": cfg.NullKeyBehavior,
		"result_path":       dest,
		"original_count":    stats.OriginalCount,
		"dedup_count":       stats.DedupCount,
		"removed_count":     stats.RemovedCount,
		"null_key_kept":     stats.NullKeyKept,
		"null_key_removed":  stats.NullKeyRemoved,
		"records_truncated": truncated,
	}
	e.SetStepOutputWithDetails(outputData, stepOutput, execDetails)

	log.Printf("  ✅ Remove Duplicates: %d → %d records (%d removed, %d null-key handled) in %dms",
		stats.OriginalCount, stats.DedupCount, stats.RemovedCount,
		stats.NullKeyKept+stats.NullKeyRemoved, durationMs)

	return outputData, nil
}

// resolveSourceArray retrieves and normalises the source value to []interface{}.
// Returns (nil, nil) when the field does not exist — the caller treats this as a no-op.
func (e *RemoveDuplicatesExecutor) resolveSourceArray(
	data map[string]interface{},
	sourceField string,
) ([]interface{}, error) {
	raw := executors.GetFieldValue(data, sourceField)
	if raw == nil {
		// Fallback: check inside a top-level "message" envelope
		if msg, ok := data["message"].(map[string]interface{}); ok {
			raw = executors.GetFieldValue(msg, sourceField)
		}
	}
	if raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case []interface{}:
		return v, nil
	case []map[string]interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out, nil
	default:
		return nil, fmt.Errorf("sourceField '%s' is not an array (got %T)", sourceField, raw)
	}
}

// buildPreview returns a sub-slice of at most n elements and a truncation flag.
// When truncation is not required the original slice is returned (zero allocation).
func (e *RemoveDuplicatesExecutor) buildPreview(slice []interface{}, n int) (interface{}, bool) {
	if len(slice) <= n {
		return slice, false
	}
	return slice[:n], true // sub-slice header only, no data copy
}

// GetOutputVariables implements VariableProvider.
// Exposes statically-known output paths for IntelliSense and the Variables tab.
func (e *RemoveDuplicatesExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	cfg := removeDuplicatesConfig{
		Strategy:        "first",
		CaseSensitive:   true,
		NullKeyBehavior: "group",
	}
	if step.Config != nil {
		if b, _ := json.Marshal(step.Config); b != nil {
			json.Unmarshal(b, &cfg) //nolint:errcheck
		}
	}

	ns := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	base := "steps." + ns + ".step_output"

	// Determine the actual output path for the full array
	outputPath := cfg.SourceField
	if cfg.OutputField != "" {
		outputPath = cfg.OutputField
	}

	defs := []models.VariableDefinition{
		{
			Name: "output_array", Path: outputPath, DataType: "array",
			Description: fmt.Sprintf("Full deduplicated array at '%s' — use this in downstream steps", outputPath),
			Category:    "Data Transform",
		},
		{
			Name: "result_path", Path: base + ".result_path", DataType: "string",
			Description: "Dot-path where the full deduplicated array was written",
			Category:    "Data Transform",
		},
		{
			Name: "records", Path: base + ".records", DataType: "array",
			Description: fmt.Sprintf("Preview of deduplicated records (≤%d rows); full array is at result_path", defaultPreviewLimit),
			Category:    "Data Transform",
		},
		{
			Name: "dedup_count", Path: base + ".dedup_count", DataType: "integer",
			Description: "Records remaining after deduplication", Category: "Data Transform",
		},
		{
			Name: "original_count", Path: base + ".original_count", DataType: "integer",
			Description: "Total records before deduplication", Category: "Data Transform",
		},
		{
			Name: "removed_count", Path: base + ".removed_count", DataType: "integer",
			Description: "Duplicate records that were removed", Category: "Data Transform",
		},
		{
			Name: "null_key_kept", Path: base + ".null_key_kept", DataType: "integer",
			Description: "Records with null key fields kept (nullKeyBehavior=keep)", Category: "Data Transform",
		},
		{
			Name: "null_key_removed", Path: base + ".null_key_removed", DataType: "integer",
			Description: "Records with null key fields dropped (nullKeyBehavior=remove)", Category: "Data Transform",
		},
	}

	return defs
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
