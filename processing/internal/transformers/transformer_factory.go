// internal/transformers/transformer_factory.go
// Pluggable transformer factory and base implementations

package transformers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ezhealthkonnect/processing/pkg"
)

// TransformerFactory manages transformer creation and registration
type TransformerFactory struct {
	transformers map[string]TransformerCreator
	mutex        sync.RWMutex
}

// TransformerCreator defines how to create a transformer
type TransformerCreator func(config TransformerConfig) (pkg.MessageTransformer, error)

// TransformerConfig defines transformer configuration
type TransformerConfig struct {
	Type           string                 `json:"type"`
	SourceFormat   string                 `json:"sourceFormat"`
	TargetFormat   string                 `json:"targetFormat"`
	Rules          map[string]interface{} `json:"rules"`
	Templates      map[string]string      `json:"templates"`
	Settings       map[string]interface{} `json:"settings"`
	EnableCache    bool                   `json:"enableCache"`
	CacheSize      int                    `json:"cacheSize"`
	ValidateInput  bool                   `json:"validateInput"`
	ValidateOutput bool                   `json:"validateOutput"`
}

// BaseTransformer provides common functionality for all transformers
type BaseTransformer struct {
	config       TransformerConfig
	sourceFormat string
	targetFormat string
	rules        map[string]interface{}
	cache        *TransformCache
	stats        *TransformerStats
	mutex        sync.RWMutex
}

// TransformerStats tracks transformation performance
type TransformerStats struct {
	TotalTransforms int64   `json:"totalTransforms"`
	SuccessCount    int64   `json:"successCount"`
	ErrorCount      int64   `json:"errorCount"`
	AverageTimeMs   float64 `json:"averageTimeMs"`
	CacheHits       int64   `json:"cacheHits"`
	CacheMisses     int64   `json:"cacheMisses"`
}

// TransformCache provides caching for transformation results
type TransformCache struct {
	cache    map[string]string
	maxSize  int
	mutex    sync.RWMutex
}

// TransformationRule defines a transformation rule
type TransformationRule struct {
	Name        string                 `json:"name"`
	Source      string                 `json:"source"`      // Source field path
	Target      string                 `json:"target"`      // Target field path
	Type        string                 `json:"type"`        // direct, template, function, conditional
	Template    string                 `json:"template"`    // Template for transformation
	Function    string                 `json:"function"`    // Function name for transformation
	Condition   string                 `json:"condition"`   // Condition for conditional transformation
	Default     interface{}            `json:"default"`     // Default value
	Required    bool                   `json:"required"`    // Is this field required
	Validation  map[string]interface{} `json:"validation"`  // Validation rules
	Enabled     bool                   `json:"enabled"`     // Is this rule enabled
}

// TransformationContext provides context for transformation
type TransformationContext struct {
	SourceMessage  *pkg.UniversalMessage
	TargetMessage  *pkg.UniversalMessage
	Rules          []TransformationRule
	Variables      map[string]interface{}
	Functions      map[string]TransformFunction
	Templates      map[string]string
	Errors         []string
	Warnings       []string
}

// TransformFunction defines a transformation function
type TransformFunction func(input interface{}, args ...interface{}) (interface{}, error)

// Global transformer factory instance
var globalFactory = NewTransformerFactory()

// NewTransformerFactory creates a new transformer factory
func NewTransformerFactory() *TransformerFactory {
	factory := &TransformerFactory{
		transformers: make(map[string]TransformerCreator),
	}

	// Register built-in transformers
	factory.RegisterTransformer("hl7_to_fhir", NewHL7ToFHIRTransformer)
	factory.RegisterTransformer("fhir_to_hl7", NewFHIRToHL7Transformer)
	factory.RegisterTransformer("json_to_xml", NewJSONToXMLTransformer)
	factory.RegisterTransformer("xml_to_json", NewXMLToJSONTransformer)
	factory.RegisterTransformer("csv_to_json", NewCSVToJSONTransformer)
	factory.RegisterTransformer("template", NewTemplateTransformer)
	factory.RegisterTransformer("passthrough", NewPassthroughTransformer)

	return factory
}

// RegisterTransformer registers a new transformer type
func (tf *TransformerFactory) RegisterTransformer(name string, creator TransformerCreator) {
	tf.mutex.Lock()
	defer tf.mutex.Unlock()
	tf.transformers[strings.ToLower(name)] = creator
}

// CreateTransformer creates a transformer instance
func (tf *TransformerFactory) CreateTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	tf.mutex.RLock()
	creator, exists := tf.transformers[strings.ToLower(config.Type)]
	tf.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown transformer type: %s", config.Type)
	}

	return creator(config)
}

// GetAvailableTransformers returns a list of available transformer types
func (tf *TransformerFactory) GetAvailableTransformers() []string {
	tf.mutex.RLock()
	defer tf.mutex.RUnlock()

	types := make([]string, 0, len(tf.transformers))
	for transformerType := range tf.transformers {
		types = append(types, transformerType)
	}
	return types
}

// GetTransformerForFormats finds a suitable transformer for format conversion
func (tf *TransformerFactory) GetTransformerForFormats(sourceFormat, targetFormat string) (pkg.MessageTransformer, error) {
	sourceFormat = strings.ToLower(sourceFormat)
	targetFormat = strings.ToLower(targetFormat)

	// Direct format mappings
	formatMappings := map[string]string{
		"hl7->fhir":   "hl7_to_fhir",
		"fhir->hl7":   "fhir_to_hl7",
		"json->xml":   "json_to_xml",
		"xml->json":   "xml_to_json",
		"csv->json":   "csv_to_json",
	}

	key := fmt.Sprintf("%s->%s", sourceFormat, targetFormat)
	if transformerType, exists := formatMappings[key]; exists {
		config := TransformerConfig{
			Type:         transformerType,
			SourceFormat: sourceFormat,
			TargetFormat: targetFormat,
		}
		return tf.CreateTransformer(config)
	}

	// If no direct mapping, use template transformer
	config := TransformerConfig{
		Type:         "template",
		SourceFormat: sourceFormat,
		TargetFormat: targetFormat,
	}
	return tf.CreateTransformer(config)
}

// Global factory functions

// RegisterTransformer registers a transformer globally
func RegisterTransformer(name string, creator TransformerCreator) {
	globalFactory.RegisterTransformer(name, creator)
}

// CreateTransformer creates a transformer using the global factory
func CreateTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	return globalFactory.CreateTransformer(config)
}

// GetTransformerForFormats gets a transformer for format conversion
func GetTransformerForFormats(sourceFormat, targetFormat string) (pkg.MessageTransformer, error) {
	return globalFactory.GetTransformerForFormats(sourceFormat, targetFormat)
}

// GetAvailableTransformers gets available transformer types
func GetAvailableTransformers() []string {
	return globalFactory.GetAvailableTransformers()
}

// BaseTransformer implementation

// NewBaseTransformer creates a new base transformer
func NewBaseTransformer(config TransformerConfig) *BaseTransformer {
	cache := &TransformCache{
		cache:   make(map[string]string),
		maxSize: 1000,
	}

	if config.CacheSize > 0 {
		cache.maxSize = config.CacheSize
	}

	return &BaseTransformer{
		config:       config,
		sourceFormat: config.SourceFormat,
		targetFormat: config.TargetFormat,
		rules:        config.Rules,
		cache:        cache,
		stats:        &TransformerStats{},
	}
}

// GetSupportedFormats returns supported source and target formats
func (bt *BaseTransformer) GetSupportedFormats() ([]string, []string) {
	return []string{bt.sourceFormat}, []string{bt.targetFormat}
}

// GetTransformationType returns the transformation type
func (bt *BaseTransformer) GetTransformationType() string {
	return bt.config.Type
}

// ValidateSource validates source content
func (bt *BaseTransformer) ValidateSource(content string, contentType string) error {
	if !bt.config.ValidateInput {
		return nil
	}

	if content == "" {
		return fmt.Errorf("source content is empty")
	}

	if !strings.EqualFold(contentType, bt.sourceFormat) {
		return fmt.Errorf("content type %s does not match expected source format %s", contentType, bt.sourceFormat)
	}

	return nil
}

// ValidateTarget validates target content
func (bt *BaseTransformer) ValidateTarget(content string, contentType string) error {
	if !bt.config.ValidateOutput {
		return nil
	}

	if content == "" {
		return fmt.Errorf("target content is empty")
	}

	if !strings.EqualFold(contentType, bt.targetFormat) {
		return fmt.Errorf("content type %s does not match expected target format %s", contentType, bt.targetFormat)
	}

	return nil
}

// GetFromCache retrieves a cached transformation result
func (bt *BaseTransformer) GetFromCache(key string) (string, bool) {
	if !bt.config.EnableCache {
		return "", false
	}

	bt.cache.mutex.RLock()
	defer bt.cache.mutex.RUnlock()

	if result, exists := bt.cache.cache[key]; exists {
		bt.stats.CacheHits++
		return result, true
	}

	bt.stats.CacheMisses++
	return "", false
}

// PutInCache stores a transformation result in cache
func (bt *BaseTransformer) PutInCache(key, value string) {
	if !bt.config.EnableCache {
		return
	}

	bt.cache.mutex.Lock()
	defer bt.cache.mutex.Unlock()

	// Simple LRU: if cache is full, remove oldest entry
	if len(bt.cache.cache) >= bt.cache.maxSize {
		// Find and remove one entry (not truly LRU, but simple)
		for k := range bt.cache.cache {
			delete(bt.cache.cache, k)
			break
		}
	}

	bt.cache.cache[key] = value
}

// UpdateStats updates transformation statistics
func (bt *BaseTransformer) UpdateStats(success bool, durationMs int64) {
	bt.mutex.Lock()
	defer bt.mutex.Unlock()

	bt.stats.TotalTransforms++
	if success {
		bt.stats.SuccessCount++
	} else {
		bt.stats.ErrorCount++
	}

	// Update rolling average
	if bt.stats.TotalTransforms == 1 {
		bt.stats.AverageTimeMs = float64(durationMs)
	} else {
		bt.stats.AverageTimeMs = (bt.stats.AverageTimeMs*0.9) + (float64(durationMs)*0.1)
	}
}

// GetStats returns transformer statistics
func (bt *BaseTransformer) GetStats() *TransformerStats {
	bt.mutex.RLock()
	defer bt.mutex.RUnlock()

	// Return a copy
	return &TransformerStats{
		TotalTransforms: bt.stats.TotalTransforms,
		SuccessCount:    bt.stats.SuccessCount,
		ErrorCount:      bt.stats.ErrorCount,
		AverageTimeMs:   bt.stats.AverageTimeMs,
		CacheHits:       bt.stats.CacheHits,
		CacheMisses:     bt.stats.CacheMisses,
	}
}

// NewTransformationContext creates a new transformation context
func NewTransformationContext(source *pkg.UniversalMessage) *TransformationContext {
	return &TransformationContext{
		SourceMessage: source,
		TargetMessage: pkg.NewUniversalMessage(),
		Variables:     make(map[string]interface{}),
		Functions:     GetStandardFunctions(),
		Templates:     make(map[string]string),
		Errors:        []string{},
		Warnings:      []string{},
	}
}

// AddError adds an error to the transformation context
func (tc *TransformationContext) AddError(message string) {
	tc.Errors = append(tc.Errors, message)
}

// AddWarning adds a warning to the transformation context
func (tc *TransformationContext) AddWarning(message string) {
	tc.Warnings = append(tc.Warnings, message)
}

// HasErrors returns whether the context has errors
func (tc *TransformationContext) HasErrors() bool {
	return len(tc.Errors) > 0
}

// HasWarnings returns whether the context has warnings
func (tc *TransformationContext) HasWarnings() bool {
	return len(tc.Warnings) > 0
}

// GetStandardFunctions returns standard transformation functions
func GetStandardFunctions() map[string]TransformFunction {
	return map[string]TransformFunction{
		"upper": func(input interface{}, args ...interface{}) (interface{}, error) {
			if str, ok := input.(string); ok {
				return strings.ToUpper(str), nil
			}
			return input, nil
		},
		"lower": func(input interface{}, args ...interface{}) (interface{}, error) {
			if str, ok := input.(string); ok {
				return strings.ToLower(str), nil
			}
			return input, nil
		},
		"trim": func(input interface{}, args ...interface{}) (interface{}, error) {
			if str, ok := input.(string); ok {
				return strings.TrimSpace(str), nil
			}
			return input, nil
		},
		"substring": func(input interface{}, args ...interface{}) (interface{}, error) {
			if str, ok := input.(string); ok && len(args) >= 2 {
				if start, ok := args[0].(int); ok {
					if end, ok := args[1].(int); ok {
						if start >= 0 && end <= len(str) && start <= end {
							return str[start:end], nil
						}
					}
				}
			}
			return input, nil
		},
		"replace": func(input interface{}, args ...interface{}) (interface{}, error) {
			if str, ok := input.(string); ok && len(args) >= 2 {
				if old, ok := args[0].(string); ok {
					if new, ok := args[1].(string); ok {
						return strings.ReplaceAll(str, old, new), nil
					}
				}
			}
			return input, nil
		},
		"concat": func(input interface{}, args ...interface{}) (interface{}, error) {
			result := fmt.Sprintf("%v", input)
			for _, arg := range args {
				result += fmt.Sprintf("%v", arg)
			}
			return result, nil
		},
		"default": func(input interface{}, args ...interface{}) (interface{}, error) {
			if input == nil || input == "" {
				if len(args) > 0 {
					return args[0], nil
				}
			}
			return input, nil
		},
	}
}

// CreateCacheKey creates a cache key from input and rules
func CreateCacheKey(input string, rules map[string]interface{}) string {
	// Simple hash-like key generation
	key := fmt.Sprintf("%d_%d", len(input), len(rules))
	if len(input) < 100 {
		key += "_" + input
	}
	return key
}