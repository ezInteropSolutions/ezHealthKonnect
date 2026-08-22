// services/parsers/fhir_parser_service.go
// FHIR Parser Service - implements MessageParser for FHIR R4/R5 resources.
// Mirrors HL7ParserService: ParsedJSON carries the raw resource fields for
// backward compat with FHIRCollectionResolver; EnhancedFields provides the
// format-agnostic schema-annotated view.

package parsers

import (
	"encoding/json"
	"log"
	"sort"
	"time"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
)

// FHIRParserService implements MessageParser for FHIR JSON resources.
type FHIRParserService struct{}

// NewFHIRParserService creates a FHIRParserService.
func NewFHIRParserService() *FHIRParserService { return &FHIRParserService{} }

// Format implements MessageParser.
func (fp *FHIRParserService) Format() string { return string(models.FormatFHIR) }

// Parse implements MessageParser.
// ParsedJSON contains all raw resource fields at root so FHIRCollectionResolver
// and existing step accessors (resourceType, id, subject, …) keep working.
// EnhancedFields provides the format-agnostic schema-annotated view.
func (fp *FHIRParserService) Parse(rawContent string) *models.ParserResult {
	startTime := time.Now()

	// Must be valid JSON
	var resource map[string]interface{}
	if err := json.Unmarshal([]byte(rawContent), &resource); err != nil {
		return fp.fallback(rawContent, "JSON parse failed: "+err.Error(), startTime)
	}

	resourceType, _ := resource["resourceType"].(string)
	if resourceType == "" {
		// Not a FHIR resource — treat as plain JSON via passthrough
		return fp.fallback(rawContent, "missing resourceType — treated as plain JSON", startTime)
	}

	// Build ParsedJSON: all raw resource fields + envelope keys
	parsedJSON := make(map[string]interface{}, len(resource)+4)
	for k, v := range resource {
		parsedJSON[k] = v
	}
	parsedJSON["raw"] = rawContent
	parsedJSON["_format"] = "fhir"
	parsedJSON["parsedAt"] = time.Now().Format(time.RFC3339)

	// Build EnhancedFields with schema annotations
	enhancedFields, fieldOrder, typeName, typeDesc, schemaLoaded := fp.buildEnhancedFields(resource, resourceType)
	parsedJSON["schemaLoaded"] = schemaLoaded

	log.Printf("🔍 FHIR Parser: resourceType=%s schemaLoaded=%v fields=%d",
		resourceType, schemaLoaded, len(enhancedFields))

	return &models.ParserResult{
		Success:    true,
		Format:     models.FormatFHIR,
		ParsedJSON: parsedJSON,
		Metadata: models.ParserMetadata{
			DetectedVersion: "R4",
			MessageType:     resourceType,
			FieldCount:      len(enhancedFields),
			ParsedAt:        time.Now(),
		},
		ValidationResult: models.ValidationResult{IsValid: true},
		EnhancedFields:   enhancedFields,
		FieldOrder:       fieldOrder,
		TypeName:         typeName,
		TypeDescription:  typeDesc,
		ParsingTime:      time.Since(startTime),
	}
}

// buildEnhancedFields annotates every top-level field of the resource (or Bundle)
// with schema metadata. For a Bundle, each entry's contained resource is also
// annotated so the enhanced view covers all resources in the message.
func (fp *FHIRParserService) buildEnhancedFields(
	resource map[string]interface{},
	resourceType string,
) (fields map[string]*models.EnhancedField, order []string, typeName, typeDesc string, schemaLoaded bool) {
	fields = make(map[string]*models.EnhancedField)

	// Annotate the top-level resource (Bundle or single resource)
	schema := fp.loadSchema(resourceType)
	if schema != nil {
		schemaLoaded = true
		typeName = schema.Name
		typeDesc = schema.Description
	}
	fp.annotateResource(resource, resourceType, schema, fields)

	// For Bundles: also annotate each contained resource so consumers
	// get full schema context for every entry — whether input is a
	// single resource or a Bundle.
	if resourceType == "Bundle" {
		if entries, ok := resource["entry"].([]interface{}); ok {
			for _, e := range entries {
				entry, ok := e.(map[string]interface{})
				if !ok {
					continue
				}
				res, ok := entry["resource"].(map[string]interface{})
				if !ok {
					continue
				}
				rt, _ := res["resourceType"].(string)
				if rt == "" {
					continue
				}
				entrySchema := fp.loadSchema(rt)
				if entrySchema != nil {
					schemaLoaded = true
				}
				fp.annotateResource(res, rt, entrySchema, fields)
			}
		}
	}

	for path := range fields {
		order = append(order, path)
	}
	sort.Strings(order)
	return
}

// loadSchema loads the FHIR schema for a resource type, returning nil on failure.
func (fp *FHIRParserService) loadSchema(resourceType string) *r4.CompiledProfile {
	reg := r4.GetRegistry()
	if reg == nil {
		return nil
	}
	cp, ok := reg.Get("R4", resourceType, "base")
	if !ok {
		return nil
	}
	return cp
}

// annotateResource adds EnhancedFields for all top-level keys of a resource,
// merged into the shared fields map keyed as "ResourceType.element".
func (fp *FHIRParserService) annotateResource(
	resource map[string]interface{},
	resourceType string,
	schema *r4.CompiledProfile,
	fields map[string]*models.EnhancedField,
) {
	for key, value := range resource {
		if key == "resourceType" {
			continue
		}
		path := resourceType + "." + key
		f := &models.EnhancedField{
			Path:     path,
			Value:    value,
			HasValue: value != nil,
		}
		if schema != nil {
			if _, ok := schema.MinCard[path]; ok {
				f.Name = schema.ElementNames[path]
				f.Description = schema.ElementDescriptions[path]
				f.DataType = schema.DataTypes[path]
				f.Cardinality = schema.Cardinality(path)
				f.Required = schema.Required[path]
				f.MustSupport = schema.IsMustSupport(path)
				f.IsModifier = schema.IsModifier[path]
				f.IsSummary = schema.IsSummary[path]
				if vs, bs, ok := schema.Binding(path); ok {
					f.ValueSet = vs
					f.BindingStrength = bs
				}
				f.SchemaFound = true
			}
		}
		fields[path] = f
	}
}

// fallback returns a degraded result for non-FHIR or unparseable payloads,
// delegating to PassthroughParser so the pipeline still runs.
func (fp *FHIRParserService) fallback(raw, reason string, startTime time.Time) *models.ParserResult {
	log.Printf("⚠️  FHIR Parser: %s — using passthrough", reason)
	pt := &PassthroughParser{format: string(models.FormatFHIR)}
	result := pt.Parse(raw)
	result.ParsingTime = time.Since(startTime)
	return result
}
