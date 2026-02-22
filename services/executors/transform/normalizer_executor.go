package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"
)

// NormalizerExecutor handles data normalization, pivot, and transpose operations
type NormalizerExecutor struct {
	*executors.BaseExecutor
}

func NewNormalizerExecutor() *NormalizerExecutor {
	return &NormalizerExecutor{
		BaseExecutor: executors.NewBaseExecutor("normalizer", models.ExecutorMetadata{
			Name:        "Normalizer / Pivot / Transpose",
			Description: "Normalizes, pivots, or transposes dataset structures",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Data Transform",
		}),
	}
}

type normalizerConfig struct {
	Operation    string            `json:"operation"`    // "normalize", "pivot", "transpose", "flatten", "unflatten"
	SourceField  string            `json:"sourceField"`  // Path to source data
	// Normalize config
	NormalizeFields []string       `json:"normalizeFields"` // Fields to normalize into rows
	KeyColumn       string         `json:"keyColumn"`       // Name for the key column
	ValueColumn     string         `json:"valueColumn"`     // Name for the value column
	PreserveFields  []string       `json:"preserveFields"`  // Fields to keep in each row
	// Pivot config
	PivotField   string            `json:"pivotField"`   // Field whose values become columns
	ValueField   string            `json:"valueField"`   // Field to use as values
	GroupBy      []string          `json:"groupBy"`      // Fields to group by
	// Flatten config
	Delimiter    string            `json:"delimiter"`    // Separator for flattened keys (default: ".")
	MaxDepth     int               `json:"maxDepth"`     // Max depth for flattening (0 = unlimited)
	// Column rename
	RenameMap    map[string]string `json:"renameMap"`    // Old name -> new name
	// Case transform
	CaseTransform string           `json:"caseTransform"` // "lower", "upper", "camel", "snake"
}

func (e *NormalizerExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()

	config := normalizerConfig{
		Delimiter:   ".",
		KeyColumn:   "field",
		ValueColumn: "value",
	}
	if step.Config != nil {
		configJSON, _ := json.Marshal(step.Config)
		json.Unmarshal(configJSON, &config)
	}

	if config.Operation == "" {
		return nil, fmt.Errorf("normalizer requires 'operation' in config")
	}

	log.Printf("  📐 Normalizer: operation=%s, source=%s", config.Operation, config.SourceField)

	outputData := make(map[string]interface{})
	for k, v := range inputData {
		outputData[k] = v
	}

	// Get source data
	var sourceData interface{}
	if config.SourceField != "" {
		sourceData = executors.GetFieldValue(outputData, config.SourceField)
		if sourceData == nil {
			if msg, ok := outputData["message"].(map[string]interface{}); ok {
				sourceData = executors.GetFieldValue(msg, config.SourceField)
			}
		}
	}

	var result interface{}
	var err error

	switch config.Operation {
	case "normalize":
		result, err = e.normalize(sourceData, config)
	case "pivot":
		result, err = e.pivot(sourceData, config)
	case "transpose":
		result, err = e.transpose(sourceData, config)
	case "flatten":
		result, err = e.flatten(sourceData, config)
	case "unflatten":
		result, err = e.unflatten(sourceData, config)
	default:
		return nil, fmt.Errorf("unknown normalizer operation: %s", config.Operation)
	}

	if err != nil {
		return nil, fmt.Errorf("normalizer %s failed: %w", config.Operation, err)
	}

	// Apply column rename if specified
	if len(config.RenameMap) > 0 {
		result = e.applyRename(result, config.RenameMap)
	}

	// Apply case transform if specified
	if config.CaseTransform != "" {
		result = e.applyCaseTransform(result, config.CaseTransform)
	}

	// Store result back at the source field (in-place transformation)
	if config.SourceField != "" {
		executors.UpdateFieldValue(outputData, config.SourceField, result)
	}

	durationMs := time.Since(startTime).Milliseconds()

	variables := map[string]interface{}{
		"operation":    config.Operation,
		"result_count": e.countItems(result),
	}
	details := map[string]interface{}{
		"duration_ms":  durationMs,
		"success":      true,
		"operation":    config.Operation,
		"result_count": e.countItems(result),
	}
	e.SetStepOutputWithDetails(outputData, variables, details)

	log.Printf("  ✅ Normalizer %s complete: %d items", config.Operation, e.countItems(result))
	return outputData, nil
}

// normalize converts columns to rows (unpivot)
// Input: [{name: "John", age: 30, city: "NYC"}]
// Output: [{key: "name", value: "John"}, {key: "age", value: 30}, {key: "city", value: "NYC"}]
func (e *NormalizerExecutor) normalize(data interface{}, config normalizerConfig) (interface{}, error) {
	items := e.toSlice(data)
	if items == nil {
		return nil, fmt.Errorf("source data is not an array")
	}

	var result []map[string]interface{}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		fieldsToNormalize := config.NormalizeFields
		if len(fieldsToNormalize) == 0 {
			for k := range itemMap {
				fieldsToNormalize = append(fieldsToNormalize, k)
			}
		}

		for _, field := range fieldsToNormalize {
			row := map[string]interface{}{
				config.KeyColumn:   field,
				config.ValueColumn: itemMap[field],
			}
			// Preserve specified fields
			for _, pf := range config.PreserveFields {
				if val, exists := itemMap[pf]; exists {
					row[pf] = val
				}
			}
			result = append(result, row)
		}
	}

	return result, nil
}

// pivot converts rows to columns
// Input: [{category: "A", metric: "count", value: 10}, {category: "A", metric: "total", value: 100}]
// Output: [{category: "A", count: 10, total: 100}]
func (e *NormalizerExecutor) pivot(data interface{}, config normalizerConfig) (interface{}, error) {
	items := e.toSlice(data)
	if items == nil {
		return nil, fmt.Errorf("source data is not an array")
	}

	if config.PivotField == "" || config.ValueField == "" {
		return nil, fmt.Errorf("pivot requires pivotField and valueField")
	}

	groups := make(map[string]map[string]interface{})

	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Build group key
		groupKey := e.buildGroupKey(itemMap, config.GroupBy)

		if _, exists := groups[groupKey]; !exists {
			groups[groupKey] = make(map[string]interface{})
			for _, gb := range config.GroupBy {
				groups[groupKey][gb] = itemMap[gb]
			}
		}

		pivotVal := fmt.Sprintf("%v", itemMap[config.PivotField])
		groups[groupKey][pivotVal] = itemMap[config.ValueField]
	}

	var result []map[string]interface{}
	for _, group := range groups {
		result = append(result, group)
	}

	return result, nil
}

// transpose swaps rows and columns
func (e *NormalizerExecutor) transpose(data interface{}, config normalizerConfig) (interface{}, error) {
	items := e.toSlice(data)
	if items == nil || len(items) == 0 {
		return nil, fmt.Errorf("source data is empty or not an array")
	}

	// Collect all keys
	allKeys := make(map[string]bool)
	for _, item := range items {
		if itemMap, ok := item.(map[string]interface{}); ok {
			for k := range itemMap {
				allKeys[k] = true
			}
		}
	}

	// Transpose: each key becomes a row, each row becomes a column
	var result []map[string]interface{}
	for key := range allKeys {
		row := map[string]interface{}{
			"field": key,
		}
		for i, item := range items {
			if itemMap, ok := item.(map[string]interface{}); ok {
				row[fmt.Sprintf("row_%d", i)] = itemMap[key]
			}
		}
		result = append(result, row)
	}

	return result, nil
}

// flatten converts nested objects to flat key-value pairs
func (e *NormalizerExecutor) flatten(data interface{}, config normalizerConfig) (interface{}, error) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("flatten requires an object, got %T", data)
	}

	result := make(map[string]interface{})
	e.flattenRecursive(dataMap, "", config.Delimiter, config.MaxDepth, 0, result)
	return result, nil
}

func (e *NormalizerExecutor) flattenRecursive(data map[string]interface{}, prefix, delimiter string, maxDepth, currentDepth int, result map[string]interface{}) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + delimiter + key
		}

		if maxDepth > 0 && currentDepth >= maxDepth {
			result[fullKey] = value
			continue
		}

		switch v := value.(type) {
		case map[string]interface{}:
			e.flattenRecursive(v, fullKey, delimiter, maxDepth, currentDepth+1, result)
		default:
			result[fullKey] = value
		}
	}
}

// unflatten converts flat keys back to nested objects
func (e *NormalizerExecutor) unflatten(data interface{}, config normalizerConfig) (interface{}, error) {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unflatten requires an object, got %T", data)
	}

	result := make(map[string]interface{})
	for key, value := range dataMap {
		parts := strings.Split(key, config.Delimiter)
		current := result
		for i, part := range parts {
			if i == len(parts)-1 {
				current[part] = value
			} else {
				if _, exists := current[part]; !exists {
					current[part] = make(map[string]interface{})
				}
				if next, ok := current[part].(map[string]interface{}); ok {
					current = next
				}
			}
		}
	}

	return result, nil
}

func (e *NormalizerExecutor) applyRename(data interface{}, renameMap map[string]string) interface{} {
	switch v := data.(type) {
	case []map[string]interface{}:
		for i, item := range v {
			v[i] = e.renameKeys(item, renameMap)
		}
		return v
	case []interface{}:
		for i, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				v[i] = e.renameKeys(m, renameMap)
			}
		}
		return v
	case map[string]interface{}:
		return e.renameKeys(v, renameMap)
	}
	return data
}

func (e *NormalizerExecutor) renameKeys(m map[string]interface{}, renameMap map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if newKey, ok := renameMap[k]; ok {
			result[newKey] = v
		} else {
			result[k] = v
		}
	}
	return result
}

func (e *NormalizerExecutor) applyCaseTransform(data interface{}, transform string) interface{} {
	switch v := data.(type) {
	case []map[string]interface{}:
		for i, item := range v {
			v[i] = e.transformKeys(item, transform)
		}
		return v
	case []interface{}:
		for i, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				v[i] = e.transformKeys(m, transform)
			}
		}
		return v
	case map[string]interface{}:
		return e.transformKeys(v, transform)
	}
	return data
}

func (e *NormalizerExecutor) transformKeys(m map[string]interface{}, transform string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		var newKey string
		switch transform {
		case "lower":
			newKey = strings.ToLower(k)
		case "upper":
			newKey = strings.ToUpper(k)
		case "snake":
			newKey = toSnakeCase(k)
		case "camel":
			newKey = toCamelCase(k)
		default:
			newKey = k
		}
		result[newKey] = v
	}
	return result
}

func (e *NormalizerExecutor) buildGroupKey(item map[string]interface{}, groupBy []string) string {
	if len(groupBy) == 0 {
		return "default"
	}
	var parts []string
	for _, field := range groupBy {
		parts = append(parts, fmt.Sprintf("%v", item[field]))
	}
	return strings.Join(parts, "|")
}

func (e *NormalizerExecutor) toSlice(data interface{}) []interface{} {
	switch v := data.(type) {
	case []interface{}:
		return v
	case []map[string]interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result
	}
	return nil
}

func (e *NormalizerExecutor) countItems(data interface{}) int {
	switch v := data.(type) {
	case []interface{}:
		return len(v)
	case []map[string]interface{}:
		return len(v)
	case map[string]interface{}:
		return len(v)
	}
	return 0
}

func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		if r >= 'A' && r <= 'Z' {
			result = append(result, r+32)
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func toCamelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	if len(parts) > 0 {
		parts[0] = strings.ToLower(parts[0][:1]) + parts[0][1:]
	}
	return strings.Join(parts, "")
}

func (e *NormalizerExecutor) Validate(step *models.TransformationStep) error {
	if step.Config == nil {
		return fmt.Errorf("normalizer requires config with operation")
	}
	op, _ := step.Config["operation"].(string)
	validOps := map[string]bool{
		"normalize": true, "pivot": true, "transpose": true,
		"flatten": true, "unflatten": true,
	}
	if !validOps[op] {
		return fmt.Errorf("invalid normalizer operation: '%s' (must be normalize, pivot, transpose, flatten, or unflatten)", op)
	}
	return nil
}
