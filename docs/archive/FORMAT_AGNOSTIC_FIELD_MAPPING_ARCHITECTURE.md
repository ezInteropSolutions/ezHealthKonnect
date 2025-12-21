# Format-Agnostic Field Mapping Architecture
## Universal Field References Across HL7, FHIR, CCD, and Custom Formats

**Date**: January 29, 2025
**Problem**: How does a validation step find "Patient Identifier" when HL7 stores it at `PID.3` but FHIR stores it at `identifier[0].value`?

---

## 🎯 The Core Problem

### User Story
> "I want to validate that Patient Identifier is required, regardless of whether the message is HL7, FHIR, or CCD."

### The Challenge

**Same Concept, Different Paths**:

| Format | Patient Identifier Path |
|--------|------------------------|
| **HL7 v2.x** | `enhancedSegments.PID.fields[2].value` |
| **FHIR R4** | `entry[0].resource.identifier[0].value` |
| **CCD** | `recordTarget.patientRole.id.extension` |
| **X12** | `NM1*IL*1*{lastName}~{firstName}~{identifier}` |

**User Expectation**: Create ONE validation rule that works for ALL formats.

---

## 🏗️ Solution Architecture

### Approach 1: **Semantic Field Mapping** (Recommended)
Use **semantic field names** (clinical concepts) that map to format-specific paths.

### Approach 2: **Format-Specific Config** (Current Reality)
Store separate paths for each format in step configuration.

### Approach 3: **Hybrid** (Best Balance)
Semantic mapping with format-specific overrides.

---

## 📐 Solution 1: Semantic Field Mapping (Recommended)

### Concept
Instead of referencing format-specific paths, reference **universal clinical concepts**.

### Example Configuration

**User Creates Validation Step**:
```javascript
{
  "stepName": "Validate Required Patient Fields",
  "stepType": "pre.validation",
  "config": {
    "rules": [
      {
        "field": "@patient.identifier",  // ← Semantic reference
        "type": "required",
        "errorMessage": "Patient identifier is required"
      },
      {
        "field": "@patient.name.family",  // ← Semantic reference
        "type": "required",
        "errorMessage": "Patient last name is required"
      },
      {
        "field": "@patient.birthDate",  // ← Semantic reference
        "type": "required",
        "errorMessage": "Patient date of birth is required"
      }
    ]
  }
}
```

**@ Prefix** = Semantic field (not literal path)

### Semantic Field Registry

**File**: `models/semantic_field_registry.go`

```go
package models

// SemanticFieldRegistry maps universal clinical concepts to format-specific paths
type SemanticFieldRegistry struct {
	mappings map[MessageFormat]map[string]string
}

func NewSemanticFieldRegistry() *SemanticFieldRegistry {
	registry := &SemanticFieldRegistry{
		mappings: make(map[MessageFormat]map[string]string),
	}
	registry.initializeStandardMappings()
	return registry
}

func (sfr *SemanticFieldRegistry) initializeStandardMappings() {
	// HL7 v2.x mappings
	sfr.mappings[FormatHL7v2] = map[string]string{
		// Patient Demographics
		"@patient.identifier":           "enhancedSegments.PID.fields[2].value",
		"@patient.identifier.authority": "enhancedSegments.PID.fields[2].subfields[3].value",
		"@patient.name.family":          "enhancedSegments.PID.fields[4].subfields[0].value",
		"@patient.name.given":           "enhancedSegments.PID.fields[4].subfields[1].value",
		"@patient.name.middle":          "enhancedSegments.PID.fields[4].subfields[2].value",
		"@patient.birthDate":            "enhancedSegments.PID.fields[6].value",
		"@patient.gender":               "enhancedSegments.PID.fields[7].value",
		"@patient.address.street":       "enhancedSegments.PID.fields[10].subfields[0].value",
		"@patient.address.city":         "enhancedSegments.PID.fields[10].subfields[2].value",
		"@patient.address.state":        "enhancedSegments.PID.fields[10].subfields[3].value",
		"@patient.address.postalCode":   "enhancedSegments.PID.fields[10].subfields[4].value",
		"@patient.phone":                "enhancedSegments.PID.fields[12].value",
		"@patient.ssn":                  "enhancedSegments.PID.fields[18].value",

		// Encounter
		"@encounter.id":          "enhancedSegments.PV1.fields[1].value",
		"@encounter.class":       "enhancedSegments.PV1.fields[1].value",
		"@encounter.admitDate":   "enhancedSegments.PV1.fields[43].value",
		"@encounter.location":    "enhancedSegments.PV1.fields[2].value",
		"@encounter.physician":   "enhancedSegments.PV1.fields[6].value",

		// Message Metadata
		"@message.id":            "enhancedSegments.MSH.fields[9].value",
		"@message.type":          "enhancedSegments.MSH.fields[8].value",
		"@message.timestamp":     "enhancedSegments.MSH.fields[6].value",
		"@message.sendingApp":    "enhancedSegments.MSH.fields[2].value",
		"@message.sendingFac":    "enhancedSegments.MSH.fields[3].value",
		"@message.receivingApp":  "enhancedSegments.MSH.fields[4].value",
		"@message.receivingFac":  "enhancedSegments.MSH.fields[5].value",
	}

	// FHIR R4 mappings
	sfr.mappings[FormatFHIR] = map[string]string{
		// Patient Demographics (assumes Patient is first entry)
		"@patient.identifier":           "entry[?(@.resource.resourceType=='Patient')].resource.identifier[0].value",
		"@patient.identifier.authority": "entry[?(@.resource.resourceType=='Patient')].resource.identifier[0].system",
		"@patient.name.family":          "entry[?(@.resource.resourceType=='Patient')].resource.name[0].family",
		"@patient.name.given":           "entry[?(@.resource.resourceType=='Patient')].resource.name[0].given[0]",
		"@patient.birthDate":            "entry[?(@.resource.resourceType=='Patient')].resource.birthDate",
		"@patient.gender":               "entry[?(@.resource.resourceType=='Patient')].resource.gender",
		"@patient.address.street":       "entry[?(@.resource.resourceType=='Patient')].resource.address[0].line[0]",
		"@patient.address.city":         "entry[?(@.resource.resourceType=='Patient')].resource.address[0].city",
		"@patient.address.state":        "entry[?(@.resource.resourceType=='Patient')].resource.address[0].state",
		"@patient.address.postalCode":   "entry[?(@.resource.resourceType=='Patient')].resource.address[0].postalCode",
		"@patient.phone":                "entry[?(@.resource.resourceType=='Patient')].resource.telecom[?(@.system=='phone')].value",
		"@patient.ssn":                  "entry[?(@.resource.resourceType=='Patient')].resource.identifier[?(@.system=='http://hl7.org/fhir/sid/us-ssn')].value",

		// Encounter
		"@encounter.id":          "entry[?(@.resource.resourceType=='Encounter')].resource.id",
		"@encounter.class":       "entry[?(@.resource.resourceType=='Encounter')].resource.class.code",
		"@encounter.admitDate":   "entry[?(@.resource.resourceType=='Encounter')].resource.period.start",
		"@encounter.location":    "entry[?(@.resource.resourceType=='Encounter')].resource.location[0].location.display",

		// Message Metadata (Bundle level)
		"@message.id":            "id",
		"@message.type":          "type",
		"@message.timestamp":     "timestamp",
	}

	// CCD/C-CDA mappings
	sfr.mappings[FormatCCDA] = map[string]string{
		"@patient.identifier":     "recordTarget.patientRole.id.extension",
		"@patient.name.family":    "recordTarget.patientRole.patient.name.family",
		"@patient.name.given":     "recordTarget.patientRole.patient.name.given",
		"@patient.birthDate":      "recordTarget.patientRole.patient.birthTime.value",
		"@patient.gender":         "recordTarget.patientRole.patient.administrativeGenderCode.code",
		"@patient.address.street": "recordTarget.patientRole.addr.streetAddressLine",
		"@patient.address.city":   "recordTarget.patientRole.addr.city",
		"@patient.address.state":  "recordTarget.patientRole.addr.state",
	}
}

// ResolvePath converts semantic field to format-specific path
func (sfr *SemanticFieldRegistry) ResolvePath(
	semanticField string,
	format MessageFormat,
) (string, error) {
	// Check if this is a semantic field (starts with @)
	if !strings.HasPrefix(semanticField, "@") {
		// Not semantic, return as-is (literal path)
		return semanticField, nil
	}

	// Get format-specific mappings
	formatMappings, exists := sfr.mappings[format]
	if !exists {
		return "", fmt.Errorf("no mappings for format: %s", format)
	}

	// Resolve semantic field to actual path
	actualPath, exists := formatMappings[semanticField]
	if !exists {
		return "", fmt.Errorf("semantic field '%s' not defined for format '%s'", semanticField, format)
	}

	return actualPath, nil
}

// GetAllSemanticFields returns all available semantic fields
func (sfr *SemanticFieldRegistry) GetAllSemanticFields() []string {
	// Collect unique semantic fields across all formats
	fieldsMap := make(map[string]bool)
	for _, formatMappings := range sfr.mappings {
		for semanticField := range formatMappings {
			fieldsMap[semanticField] = true
		}
	}

	// Convert to sorted slice
	fields := make([]string, 0, len(fieldsMap))
	for field := range fieldsMap {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

// GetFieldsForFormat returns semantic fields available for a specific format
func (sfr *SemanticFieldRegistry) GetFieldsForFormat(format MessageFormat) []string {
	formatMappings, exists := sfr.mappings[format]
	if !exists {
		return []string{}
	}

	fields := make([]string, 0, len(formatMappings))
	for field := range formatMappings {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}
```

### Updated Validation Executor

**File**: `services/executor_registry.go` (ValidationExecutor)

```go
type ValidationExecutor struct {
	db               *sql.DB
	semanticRegistry *models.SemanticFieldRegistry
}

func NewValidationExecutor(db *sql.DB) *ValidationExecutor {
	return &ValidationExecutor{
		db:               db,
		semanticRegistry: models.NewSemanticFieldRegistry(),
	}
}

func (ve *ValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {

	// Get current format from input data
	currentFormat := getCurrentFormat(inputData)

	// Extract validation rules
	rules, ok := step.Config["rules"].([]interface{})
	if !ok {
		return inputData, nil
	}

	var errors []string

	for _, ruleInterface := range rules {
		rule, ok := ruleInterface.(map[string]interface{})
		if !ok {
			continue
		}

		field, _ := rule["field"].(string)
		validationType, _ := rule["type"].(string)
		errorMessage, _ := rule["errorMessage"].(string)

		// ✅ Resolve semantic field to actual path
		actualPath, err := ve.semanticRegistry.ResolvePath(field, currentFormat)
		if err != nil {
			log.Printf("⚠️  Failed to resolve field '%s' for format '%s': %v", field, currentFormat, err)
			continue  // Skip this rule if field doesn't exist in this format
		}

		log.Printf("🔍 Resolved: %s → %s (format: %s)", field, actualPath, currentFormat)

		// Get value from resolved path
		value := getNestedValue(inputData, actualPath)

		// Perform validation based on type
		switch validationType {
		case "required":
			if value == nil || value == "" {
				if errorMessage == "" {
					errorMessage = fmt.Sprintf("Required field missing: %s", field)
				}
				errors = append(errors, errorMessage)
			}

		case "format":
			formatType, _ := rule["formatType"].(string)
			if !validateFormat(value, formatType) {
				if errorMessage == "" {
					errorMessage = fmt.Sprintf("Invalid format for field: %s", field)
				}
				errors = append(errors, errorMessage)
			}

		case "length":
			minLength, _ := rule["minLength"].(float64)
			maxLength, _ := rule["maxLength"].(float64)
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) < int(minLength) || len(valueStr) > int(maxLength) {
				if errorMessage == "" {
					errorMessage = fmt.Sprintf("Length validation failed for field: %s", field)
				}
				errors = append(errors, errorMessage)
			}

		case "pattern":
			pattern, _ := rule["pattern"].(string)
			valueStr := fmt.Sprintf("%v", value)
			matched, _ := regexp.MatchString(pattern, valueStr)
			if !matched {
				if errorMessage == "" {
					errorMessage = fmt.Sprintf("Pattern validation failed for field: %s", field)
				}
				errors = append(errors, errorMessage)
			}
		}
	}

	if len(errors) > 0 {
		return inputData, fmt.Errorf("validation failed: %v", errors)
	}

	return inputData, nil
}
```

---

## 🎨 UI: Semantic Field Selector

### Pipeline Builder - Validation Rule Builder

**File**: `public/js/pipeline/components/SemanticFieldSelector.js`

```javascript
/**
 * SemanticFieldSelector - Dropdown for selecting semantic fields
 * Automatically filters fields based on current pipeline format
 */
class SemanticFieldSelector {
    constructor(container, options = {}) {
        this.container = container;
        this.currentFormat = options.format || 'hl7v2';
        this.selectedField = options.initialValue || '';
        this.onChange = options.onChange || (() => {});

        this.semanticFields = this.loadSemanticFields();
        this.render();
    }

    loadSemanticFields() {
        // In production, fetch from backend API
        // For now, static list matching Go registry
        return {
            'patient': [
                { value: '@patient.identifier', label: 'Patient Identifier', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.identifier.authority', label: 'Patient ID Authority', formats: ['hl7v2', 'fhir'] },
                { value: '@patient.name.family', label: 'Patient Last Name', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.name.given', label: 'Patient First Name', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.name.middle', label: 'Patient Middle Name', formats: ['hl7v2'] },
                { value: '@patient.birthDate', label: 'Patient Date of Birth', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.gender', label: 'Patient Gender', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.address.street', label: 'Patient Street Address', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.address.city', label: 'Patient City', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.address.state', label: 'Patient State', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.address.postalCode', label: 'Patient ZIP Code', formats: ['hl7v2', 'fhir', 'ccd'] },
                { value: '@patient.phone', label: 'Patient Phone Number', formats: ['hl7v2', 'fhir'] },
                { value: '@patient.ssn', label: 'Patient SSN', formats: ['hl7v2', 'fhir'] },
            ],
            'encounter': [
                { value: '@encounter.id', label: 'Encounter ID', formats: ['hl7v2', 'fhir'] },
                { value: '@encounter.class', label: 'Encounter Class', formats: ['hl7v2', 'fhir'] },
                { value: '@encounter.admitDate', label: 'Admit Date/Time', formats: ['hl7v2', 'fhir'] },
                { value: '@encounter.location', label: 'Encounter Location', formats: ['hl7v2', 'fhir'] },
                { value: '@encounter.physician', label: 'Attending Physician', formats: ['hl7v2'] },
            ],
            'message': [
                { value: '@message.id', label: 'Message Control ID', formats: ['hl7v2', 'fhir'] },
                { value: '@message.type', label: 'Message Type', formats: ['hl7v2', 'fhir'] },
                { value: '@message.timestamp', label: 'Message Timestamp', formats: ['hl7v2', 'fhir'] },
                { value: '@message.sendingApp', label: 'Sending Application', formats: ['hl7v2'] },
                { value: '@message.sendingFac', label: 'Sending Facility', formats: ['hl7v2'] },
            ]
        };
    }

    render() {
        const html = `
            <div class="semantic-field-selector">
                <label>Field to Validate</label>
                <select class="field-select">
                    <option value="">Select a field...</option>
                    ${this.renderFieldGroups()}
                </select>
                <div class="field-help">
                    <i class="fas fa-info-circle"></i>
                    <span class="help-text">Semantic fields work across all formats</span>
                </div>
            </div>
        `;

        this.container.innerHTML = html;
        this.attachEvents();
    }

    renderFieldGroups() {
        let html = '';

        for (const [category, fields] of Object.entries(this.semanticFields)) {
            // Filter fields by current format
            const compatibleFields = fields.filter(field =>
                field.formats.includes(this.currentFormat)
            );

            if (compatibleFields.length === 0) continue;

            html += `<optgroup label="${this.capitalize(category)} Fields">`;

            for (const field of compatibleFields) {
                const selected = field.value === this.selectedField ? 'selected' : '';
                const formatBadge = field.formats.length === 3 ? '🌐' : '';

                html += `
                    <option value="${field.value}" ${selected}>
                        ${formatBadge} ${field.label}
                    </option>
                `;
            }

            html += `</optgroup>`;
        }

        // Add option for custom literal path
        html += `
            <optgroup label="Advanced">
                <option value="__custom__">⚙️ Custom Path (Advanced)</option>
            </optgroup>
        `;

        return html;
    }

    attachEvents() {
        const select = this.container.querySelector('.field-select');

        select.addEventListener('change', (e) => {
            const value = e.target.value;

            if (value === '__custom__') {
                this.showCustomPathInput();
            } else {
                this.selectedField = value;
                this.onChange(value);
            }
        });
    }

    showCustomPathInput() {
        const customInput = `
            <div class="custom-path-input">
                <label>Custom Path (format-specific)</label>
                <input type="text" placeholder="e.g., enhancedSegments.PID.fields[2].value" />
                <button class="btn-save">Save</button>
                <button class="btn-cancel">Cancel</button>
            </div>
        `;

        this.container.innerHTML += customInput;

        // Event handlers for custom path
        this.container.querySelector('.btn-save').addEventListener('click', () => {
            const customPath = this.container.querySelector('input').value;
            this.selectedField = customPath;  // No @ prefix = literal path
            this.onChange(customPath);
            this.render();
        });

        this.container.querySelector('.btn-cancel').addEventListener('click', () => {
            this.render();
        });
    }

    capitalize(str) {
        return str.charAt(0).toUpperCase() + str.slice(1);
    }

    setFormat(format) {
        this.currentFormat = format;
        this.render();
    }

    getValue() {
        return this.selectedField;
    }
}
```

### Updated ValidationRuleBuilder Integration

**File**: `public/js/pipeline/components/ValidationRuleBuilder.js`

```javascript
class ValidationRuleBuilder {
    renderFieldInput(rule, index) {
        // Create container for semantic field selector
        const containerId = `semantic-field-${index}`;

        setTimeout(() => {
            const container = document.getElementById(containerId);
            if (container) {
                new SemanticFieldSelector(container, {
                    format: this.currentFormat || 'hl7v2',
                    initialValue: rule.field || '',
                    onChange: (value) => {
                        this.rules[index].field = value;
                        this.updateRulesFromDOM();
                    }
                });
            }
        }, 0);

        return `
            <div class="rule-field">
                <div id="${containerId}"></div>
            </div>
        `;
    }
}
```

---

## 📊 Real-World Example: Multi-Format Validation

### Scenario
**Hospital receives messages in both HL7 and FHIR formats. Both must validate Patient Identifier.**

### Configuration (ONE validation rule)

```json
{
  "stepName": "Validate Required Patient Fields",
  "stepType": "pre.validation",
  "sourceFormats": ["*"],
  "config": {
    "rules": [
      {
        "field": "@patient.identifier",
        "type": "required",
        "errorMessage": "Patient identifier is required"
      },
      {
        "field": "@patient.birthDate",
        "type": "required",
        "errorMessage": "Patient date of birth is required"
      }
    ]
  }
}
```

### Execution Flow

#### Message 1: HL7 ADT^A01
```
Input Data:
{
  "enhancedSegments": {
    "PID": {
      "fields": [
        {...},
        {...},
        {"key": "PID.3", "value": "MRN123456"}  ← Patient Identifier
      ]
    }
  }
}

Validation Executor:
1. Detects format: hl7v2
2. Resolves "@patient.identifier" → "enhancedSegments.PID.fields[2].value"
3. Gets value: "MRN123456"
4. Checks required: ✅ Present
5. Result: SUCCESS
```

#### Message 2: FHIR Bundle
```
Input Data:
{
  "resourceType": "Bundle",
  "entry": [
    {
      "resource": {
        "resourceType": "Patient",
        "identifier": [
          {"value": "MRN123456"}  ← Patient Identifier
        ]
      }
    }
  ]
}

Validation Executor:
1. Detects format: fhir
2. Resolves "@patient.identifier" → "entry[?(@.resource.resourceType=='Patient')].resource.identifier[0].value"
3. Gets value: "MRN123456"
4. Checks required: ✅ Present
5. Result: SUCCESS
```

**Same validation rule works for both formats!** ✅

---

## 🔄 Solution 2: Format-Specific Configuration (Alternative)

### For Users Who Want Explicit Control

```json
{
  "stepName": "Validate Patient Identifier",
  "stepType": "pre.validation",
  "sourceFormats": ["hl7v2", "fhir"],
  "config": {
    "rules": [
      {
        "hl7v2": {
          "field": "enhancedSegments.PID.fields[2].value",
          "type": "required"
        },
        "fhir": {
          "field": "entry[0].resource.identifier[0].value",
          "type": "required"
        }
      }
    ]
  }
}
```

**Validation Executor**:
```go
func (ve *ValidationExecutor) Execute(
    ctx context.Context,
    step *models.TransformationStep,
    inputData map[string]interface{},
) (map[string]interface{}, error) {

    currentFormat := getCurrentFormat(inputData)

    rules, _ := step.Config["rules"].([]interface{})

    for _, ruleInterface := range rules {
        rule, _ := ruleInterface.(map[string]interface{})

        // Get format-specific configuration
        formatConfig, exists := rule[string(currentFormat)]
        if !exists {
            log.Printf("⚠️  No validation rule for format: %s", currentFormat)
            continue
        }

        formatRule, _ := formatConfig.(map[string]interface{})
        field, _ := formatRule["field"].(string)
        validationType, _ := formatRule["type"].(string)

        // Validate using format-specific path
        value := getNestedValue(inputData, field)

        if validationType == "required" && (value == nil || value == "") {
            return inputData, fmt.Errorf("required field missing: %s", field)
        }
    }

    return inputData, nil
}
```

---

## 🎯 Solution 3: Hybrid Approach (Recommended for Production)

### Combine Both Approaches

**Priority**:
1. Check for semantic field (`@patient.identifier`)
2. If found, resolve to format-specific path
3. If not found, check for format-specific override
4. If neither found, use literal path

```go
func (ve *ValidationExecutor) resolveFieldPath(
    rule map[string]interface{},
    format models.MessageFormat,
) (string, error) {

    field, _ := rule["field"].(string)

    // 1. Check if semantic field
    if strings.HasPrefix(field, "@") {
        // Try semantic resolution
        path, err := ve.semanticRegistry.ResolvePath(field, format)
        if err == nil {
            return path, nil
        }
        // Fall through to format-specific override if semantic fails
    }

    // 2. Check for format-specific override
    if formatOverride, exists := rule[string(format)]; exists {
        if overrideMap, ok := formatOverride.(map[string]interface{}); ok {
            if overridePath, ok := overrideMap["field"].(string); ok {
                return overridePath, nil
            }
        }
    }

    // 3. Use literal path as-is
    return field, nil
}
```

**Example Configuration**:
```json
{
  "field": "@patient.identifier",  // Semantic (works for HL7, FHIR, CCD)
  "type": "required",

  // Optional: Override for specific formats
  "x12": {
    "field": "NM1*IL*1*{identifier}",  // X12 has different structure
    "type": "required"
  }
}
```

---

## 🎨 UI Mockup

### Validation Rule Builder with Semantic Fields

```
┌─────────────────────────────────────────────────────────────┐
│ Validation Rule Builder                                      │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│ Rule 1:                                                       │
│ ┌────────────────────┬──────────────┬─────────────────────┐ │
│ │ Field to Validate  │ Validation   │ Error Message       │ │
│ ├────────────────────┼──────────────┼─────────────────────┤ │
│ │ [▼ Select Field]   │ [▼ Required] │ Patient ID required │ │
│ │   Patient Fields   │              │                     │ │
│ │   🌐 Patient ID    │              │                     │ │
│ │   🌐 Last Name     │              │                     │ │
│ │   🌐 First Name    │              │                     │ │
│ │   🌐 Date of Birth │              │                     │ │
│ │   ...              │              │                     │ │
│ │   Encounter Fields │              │                     │ │
│ │   🌐 Encounter ID  │              │                     │ │
│ │   ...              │              │                     │ │
│ │   ────────────     │              │                     │ │
│ │   ⚙️ Custom Path   │              │                     │ │
│ └────────────────────┴──────────────┴─────────────────────┘ │
│                                                               │
│ ℹ️ Semantic fields (🌐) work across all formats              │
│                                                               │
│ [+ Add Rule]                                                  │
└─────────────────────────────────────────────────────────────┘
```

**When user selects "🌐 Patient ID"**:
- Field value becomes: `@patient.identifier`
- Works automatically for HL7, FHIR, CCD
- UI shows hint: "✅ Compatible with HL7, FHIR, CCD"

**When user selects "⚙️ Custom Path"**:
- Shows text input for manual path entry
- Shows warning: "⚠️ Custom paths are format-specific"
- Example: `enhancedSegments.PID.fields[2].value`

---

## 📋 Implementation Checklist

### Backend (Go)
- [ ] Create `models/semantic_field_registry.go`
- [ ] Add HL7 semantic mappings
- [ ] Add FHIR semantic mappings
- [ ] Add CCD semantic mappings
- [ ] Update ValidationExecutor to use semantic registry
- [ ] Add API endpoint: `GET /api/semantic-fields/:format`
- [ ] Update all format-aware executors (enrichment, transformation)

### Frontend (JavaScript)
- [ ] Create `SemanticFieldSelector.js` component
- [ ] Update `ValidationRuleBuilder.js` to use selector
- [ ] Add format dropdown to Pipeline Builder
- [ ] Update step configuration to store semantic fields
- [ ] Add UI hints for semantic vs literal paths

### Database
- [ ] No schema changes needed (stores `@patient.identifier` in config JSON)

### Testing
- [ ] Test HL7 validation with semantic fields
- [ ] Test FHIR validation with same semantic fields
- [ ] Test mixed HL7/FHIR pipeline
- [ ] Test custom path override
- [ ] Test format-specific override

---

## ✨ Benefits Summary

| Approach | Pros | Cons |
|----------|------|------|
| **Semantic Mapping** | ✅ Works across formats<br>✅ User-friendly<br>✅ Maintainable | ⚠️ Limited to predefined fields<br>⚠️ Requires registry maintenance |
| **Format-Specific** | ✅ Full flexibility<br>✅ No limitations | ❌ Complex configuration<br>❌ Error-prone<br>❌ Not reusable |
| **Hybrid** | ✅ Best of both worlds<br>✅ Semantic + custom<br>✅ Future-proof | ⚠️ Slightly more complex implementation |

**Recommendation**: **Hybrid Approach** (Semantic fields with format-specific overrides)

---

## 🚀 Next Steps

1. **Implement Semantic Field Registry** (2-3 hours)
2. **Update ValidationExecutor** (1 hour)
3. **Create SemanticFieldSelector UI** (2 hours)
4. **Test with HL7 and FHIR** (1 hour)
5. **Expand to other executors** (enrichment, transformation)

**Total Effort**: 6-7 hours
**Benefit**: Universal field references across all formats

---

*Document Generated: January 29, 2025*
*Solution: Format-Agnostic Field Mapping*
*Problem: Different field paths across HL7, FHIR, CCD formats*
