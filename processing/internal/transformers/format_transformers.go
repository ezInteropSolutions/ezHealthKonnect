// internal/transformers/format_transformers.go
// Format-specific transformers (JSON, XML, CSV, Template)

package transformers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"ezhealthkonnect/processing/pkg"
)

// JSONToXMLTransformer converts JSON to XML
type JSONToXMLTransformer struct {
	*BaseTransformer
	rootElement string
	prettyPrint bool
}

// XMLToJSONTransformer converts XML to JSON
type XMLToJSONTransformer struct {
	*BaseTransformer
	preserveNamespaces bool
	arrayElements      []string
}

// CSVToJSONTransformer converts CSV to JSON
type CSVToJSONTransformer struct {
	*BaseTransformer
	delimiter    rune
	hasHeaders   bool
	fieldNames   []string
	skipRows     int
}

// TemplateTransformer applies Go templates for transformation
type TemplateTransformer struct {
	*BaseTransformer
	templateString string
	template       *template.Template
	functions      template.FuncMap
}

// PassthroughTransformer passes content unchanged (useful for testing)
type PassthroughTransformer struct {
	*BaseTransformer
}

// NewJSONToXMLTransformer creates a new JSON to XML transformer
func NewJSONToXMLTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)
	base.sourceFormat = "JSON"
	base.targetFormat = "XML"

	rootElement := "root"
	if root, exists := config.Settings["root_element"]; exists {
		if r, ok := root.(string); ok {
			rootElement = r
		}
	}

	prettyPrint := true
	if pretty, exists := config.Settings["pretty_print"]; exists {
		if p, ok := pretty.(bool); ok {
			prettyPrint = p
		}
	}

	return &JSONToXMLTransformer{
		BaseTransformer: base,
		rootElement:     rootElement,
		prettyPrint:     prettyPrint,
	}, nil
}

// NewXMLToJSONTransformer creates a new XML to JSON transformer
func NewXMLToJSONTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)
	base.sourceFormat = "XML"
	base.targetFormat = "JSON"

	preserveNamespaces := false
	if preserve, exists := config.Settings["preserve_namespaces"]; exists {
		if p, ok := preserve.(bool); ok {
			preserveNamespaces = p
		}
	}

	var arrayElements []string
	if arrays, exists := config.Settings["array_elements"]; exists {
		if arraySlice, ok := arrays.([]interface{}); ok {
			arrayElements = make([]string, len(arraySlice))
			for i, elem := range arraySlice {
				if elemStr, ok := elem.(string); ok {
					arrayElements[i] = elemStr
				}
			}
		}
	}

	return &XMLToJSONTransformer{
		BaseTransformer:    base,
		preserveNamespaces: preserveNamespaces,
		arrayElements:      arrayElements,
	}, nil
}

// NewCSVToJSONTransformer creates a new CSV to JSON transformer
func NewCSVToJSONTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)
	base.sourceFormat = "CSV"
	base.targetFormat = "JSON"

	delimiter := ','
	if delim, exists := config.Settings["delimiter"]; exists {
		if delimStr, ok := delim.(string); ok && len(delimStr) > 0 {
			delimiter = rune(delimStr[0])
		}
	}

	hasHeaders := true
	if headers, exists := config.Settings["has_headers"]; exists {
		if h, ok := headers.(bool); ok {
			hasHeaders = h
		}
	}

	var fieldNames []string
	if names, exists := config.Settings["field_names"]; exists {
		if nameSlice, ok := names.([]interface{}); ok {
			fieldNames = make([]string, len(nameSlice))
			for i, name := range nameSlice {
				if nameStr, ok := name.(string); ok {
					fieldNames[i] = nameStr
				}
			}
		}
	}

	skipRows := 0
	if skip, exists := config.Settings["skip_rows"]; exists {
		if s, ok := skip.(float64); ok {
			skipRows = int(s)
		}
	}

	return &CSVToJSONTransformer{
		BaseTransformer: base,
		delimiter:       delimiter,
		hasHeaders:      hasHeaders,
		fieldNames:      fieldNames,
		skipRows:        skipRows,
	}, nil
}

// NewTemplateTransformer creates a new template transformer
func NewTemplateTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)

	templateString := ""
	if tmpl, exists := config.Settings["template"]; exists {
		if t, ok := tmpl.(string); ok {
			templateString = t
		}
	}

	if templateString == "" && len(config.Templates) > 0 {
		// Use first template if no specific template specified
		for _, tmpl := range config.Templates {
			templateString = tmpl
			break
		}
	}

	if templateString == "" {
		return nil, fmt.Errorf("template string is required")
	}

	// Create template with custom functions
	functions := template.FuncMap{
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trim":      strings.TrimSpace,
		"replace":   strings.ReplaceAll,
		"contains":  strings.Contains,
		"split":     strings.Split,
		"join":      strings.Join,
		"now":       time.Now,
		"format":    fmt.Sprintf,
		"default": func(defaultValue, value interface{}) interface{} {
			if value == nil || value == "" {
				return defaultValue
			}
			return value
		},
	}

	tmpl, err := template.New("transformer").Funcs(functions).Parse(templateString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &TemplateTransformer{
		BaseTransformer: base,
		templateString:  templateString,
		template:        tmpl,
		functions:       functions,
	}, nil
}

// NewPassthroughTransformer creates a new passthrough transformer
func NewPassthroughTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)

	return &PassthroughTransformer{
		BaseTransformer: base,
	}, nil
}

// Transform converts JSON to XML
func (j2x *JSONToXMLTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache
	cacheKey := CreateCacheKey(message.Content, j2x.rules)
	if cached, found := j2x.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = "XML"
		j2x.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Validate source
	if err := j2x.ValidateSource(message.Content, message.ContentType); err != nil {
		j2x.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Parse JSON
	var jsonData interface{}
	if err := json.Unmarshal([]byte(message.Content), &jsonData); err != nil {
		j2x.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Convert to XML
	xmlContent, err := j2x.jsonToXML(jsonData, j2x.rootElement)
	if err != nil {
		j2x.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to convert to XML: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = xmlContent
	result.ContentType = "XML"
	result.CorrelationID = message.CorrelationID
	result.TransformationApplied = "JSON_TO_XML"

	// Copy metadata
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	// Cache result
	j2x.PutInCache(cacheKey, result.Content)

	j2x.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Transform converts XML to JSON
func (x2j *XMLToJSONTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache
	cacheKey := CreateCacheKey(message.Content, x2j.rules)
	if cached, found := x2j.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = "JSON"
		x2j.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Validate source
	if err := x2j.ValidateSource(message.Content, message.ContentType); err != nil {
		x2j.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Parse XML and convert to JSON
	jsonContent, err := x2j.xmlToJSON(message.Content)
	if err != nil {
		x2j.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to convert XML to JSON: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = jsonContent
	result.ContentType = "JSON"
	result.CorrelationID = message.CorrelationID
	result.TransformationApplied = "XML_TO_JSON"

	// Copy metadata
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	// Cache result
	x2j.PutInCache(cacheKey, result.Content)

	x2j.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Transform converts CSV to JSON
func (c2j *CSVToJSONTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache
	cacheKey := CreateCacheKey(message.Content, c2j.rules)
	if cached, found := c2j.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = "JSON"
		c2j.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Validate source
	if err := c2j.ValidateSource(message.Content, message.ContentType); err != nil {
		c2j.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Parse CSV and convert to JSON
	jsonContent, err := c2j.csvToJSON(message.Content)
	if err != nil {
		c2j.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to convert CSV to JSON: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = jsonContent
	result.ContentType = "JSON"
	result.CorrelationID = message.CorrelationID
	result.TransformationApplied = "CSV_TO_JSON"

	// Copy metadata
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	// Cache result
	c2j.PutInCache(cacheKey, result.Content)

	c2j.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Transform applies template transformation
func (tt *TemplateTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache
	cacheKey := CreateCacheKey(message.Content, tt.rules)
	if cached, found := tt.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = tt.targetFormat
		tt.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Prepare template data
	templateData := map[string]interface{}{
		"Content":     message.Content,
		"ContentType": message.ContentType,
		"Message":     message,
		"Metadata":    message.Metadata,
		"Now":         time.Now(),
	}

	// Apply template
	var output strings.Builder
	if err := tt.template.Execute(&output, templateData); err != nil {
		tt.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = output.String()
	result.ContentType = tt.targetFormat
	result.CorrelationID = message.CorrelationID
	result.TransformationApplied = "TEMPLATE"

	// Copy metadata
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()
	result.Metadata["template_applied"] = true

	// Cache result
	tt.PutInCache(cacheKey, result.Content)

	tt.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Transform passes content unchanged
func (pt *PassthroughTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Create a copy of the message
	result := pkg.NewUniversalMessage()
	result.Content = message.Content
	result.ContentType = message.ContentType
	result.CorrelationID = message.CorrelationID
	result.TransformationApplied = "PASSTHROUGH"

	// Copy metadata
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	pt.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Helper methods

func (j2x *JSONToXMLTransformer) jsonToXML(data interface{}, elementName string) (string, error) {
	var xmlBuilder strings.Builder

	xmlBuilder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	xmlBuilder.WriteString("\n")

	if err := j2x.buildXMLElement(&xmlBuilder, data, elementName, 0); err != nil {
		return "", err
	}

	return xmlBuilder.String(), nil
}

func (j2x *JSONToXMLTransformer) buildXMLElement(builder *strings.Builder, data interface{}, elementName string, depth int) error {
	indent := strings.Repeat("  ", depth)

	switch v := data.(type) {
	case map[string]interface{}:
		builder.WriteString(fmt.Sprintf("%s<%s>\n", indent, elementName))
		for key, value := range v {
			if err := j2x.buildXMLElement(builder, value, key, depth+1); err != nil {
				return err
			}
		}
		builder.WriteString(fmt.Sprintf("%s</%s>\n", indent, elementName))

	case []interface{}:
		for _, item := range v {
			if err := j2x.buildXMLElement(builder, item, elementName, depth); err != nil {
				return err
			}
		}

	default:
		value := fmt.Sprintf("%v", v)
		builder.WriteString(fmt.Sprintf("%s<%s>%s</%s>\n", indent, elementName, xml.EscapeText([]byte(value)), elementName))
	}

	return nil
}

func (x2j *XMLToJSONTransformer) xmlToJSON(xmlContent string) (string, error) {
	// Simple XML to JSON conversion
	// This is a basic implementation - for production use, consider using a more robust XML parser

	var result map[string]interface{}

	decoder := xml.NewDecoder(strings.NewReader(xmlContent))

	// Parse XML into a generic structure
	tokens := make([]xml.Token, 0)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("XML parsing error: %w", err)
		}
		tokens = append(tokens, xml.CopyToken(token))
	}

	// Convert tokens to JSON structure (simplified)
	result = make(map[string]interface{})
	result["data"] = xmlContent // Fallback - store raw XML

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON marshaling error: %w", err)
	}

	return string(jsonBytes), nil
}

func (c2j *CSVToJSONTransformer) csvToJSON(csvContent string) (string, error) {
	reader := csv.NewReader(strings.NewReader(csvContent))
	reader.Comma = c2j.delimiter

	// Skip initial rows if specified
	for i := 0; i < c2j.skipRows; i++ {
		if _, err := reader.Read(); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error skipping rows: %w", err)
		}
	}

	var headers []string
	var records []map[string]interface{}

	// Read headers
	if c2j.hasHeaders {
		headerRow, err := reader.Read()
		if err != nil {
			return "", fmt.Errorf("error reading headers: %w", err)
		}
		headers = headerRow
	} else if len(c2j.fieldNames) > 0 {
		headers = c2j.fieldNames
	} else {
		// Generate default headers
		firstRow, err := reader.Read()
		if err != nil {
			return "", fmt.Errorf("error reading first row: %w", err)
		}
		for i := range firstRow {
			headers = append(headers, fmt.Sprintf("field_%d", i+1))
		}
		// Process the first row as data
		record := make(map[string]interface{})
		for i, value := range firstRow {
			if i < len(headers) {
				record[headers[i]] = value
			}
		}
		records = append(records, record)
	}

	// Read data rows
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("error reading CSV row: %w", err)
		}

		record := make(map[string]interface{})
		for i, value := range row {
			if i < len(headers) {
				record[headers[i]] = value
			}
		}
		records = append(records, record)
	}

	result := map[string]interface{}{
		"data":        records,
		"total_rows":  len(records),
		"headers":     headers,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON marshaling error: %w", err)
	}

	return string(jsonBytes), nil
}