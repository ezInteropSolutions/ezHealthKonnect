# Step Configuration User Guide
## How Users Configure Steps with Field Path Selection

**Date**: January 29, 2025
**Audience**: Product Owners, UX Designers, Developers

---

## 🎯 Core Concept

> **User selects the source data path → Step performs the required work**

Every transformation step needs to know:
1. **WHERE** to find the data (field path)
2. **WHAT** to do with it (step logic)

---

## 📋 Universal Step Configuration Pattern

### All Steps Follow This Pattern

```javascript
{
  "stepName": "User-defined name",
  "stepType": "validation/enrichment/transformation/etc",
  "config": {
    // User selects paths for all required fields
    "sourceField": "@patient.identifier",    // WHERE to get data
    "targetField": "output.patientId",       // WHERE to put result
    "operation": "validate/transform/copy",  // WHAT to do
    // ... step-specific parameters
  }
}
```

---

## 🔍 Example 1: Validation Step

### User Configuration Flow

**Step 1: User Adds Validation Step**
```
Pipeline Builder → Drag "Field Validation" to canvas
```

**Step 2: Configure Fields to Validate**
```
┌─────────────────────────────────────────────────────────┐
│ Field Validation Configuration                          │
├─────────────────────────────────────────────────────────┤
│                                                          │
│ Rule 1:                                                  │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:        [▼ @patient.identifier      ]  ││
│ │ Validation Type:     [▼ Required                  ]  ││
│ │ Error Message:       Patient ID is required          ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ Rule 2:                                                  │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:        [▼ @patient.birthDate       ]  ││
│ │ Validation Type:     [▼ Format                    ]  ││
│ │   Format Type:       [▼ Date (YYYYMMDD)          ]  ││
│ │ Error Message:       Invalid date format             ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ [+ Add Rule]                                             │
└─────────────────────────────────────────────────────────┘
```

**Step 3: Saved Configuration (JSON)**
```json
{
  "stepName": "Validate Patient Fields",
  "stepType": "pre.validation",
  "config": {
    "rules": [
      {
        "sourceField": "@patient.identifier",
        "validationType": "required",
        "errorMessage": "Patient ID is required"
      },
      {
        "sourceField": "@patient.birthDate",
        "validationType": "format",
        "formatType": "date_yyyymmdd",
        "errorMessage": "Invalid date format"
      }
    ]
  }
}
```

**Step 4: Runtime Execution**
```go
// ValidationExecutor.Execute()
for each rule:
    1. Resolve: "@patient.identifier" → "enhancedSegments.PID.fields[2].value"
    2. Extract value from source path
    3. Validate according to type (required, format, length, pattern)
    4. Return success or error
```

---

## 🔄 Example 2: Field Mapping Step (Copy/Transform Fields)

### User Configuration Flow

**Step 1: User Adds Field Mapping Step**
```
Pipeline Builder → Drag "Field Mapping" to canvas
```

**Step 2: Configure Field Mappings**
```
┌─────────────────────────────────────────────────────────┐
│ Field Mapping Configuration                             │
├─────────────────────────────────────────────────────────┤
│                                                          │
│ Mapping 1:                                               │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:   [▼ @patient.identifier           ]  ││
│ │ Target Field:   [  output.medicalRecordNumber    ]  ││
│ │ Transformation: [▼ Copy as-is                     ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ Mapping 2:                                               │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:   [▼ @patient.name.family          ]  ││
│ │ Target Field:   [  output.lastName               ]  ││
│ │ Transformation: [▼ Uppercase                      ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ Mapping 3:                                               │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:   [▼ @patient.birthDate            ]  ││
│ │ Target Field:   [  output.dob                    ]  ││
│ │ Transformation: [▼ Convert Date Format            ]  ││
│ │   From Format:  [  YYYYMMDD                      ]  ││
│ │   To Format:    [  YYYY-MM-DD                    ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ [+ Add Mapping]                                          │
└─────────────────────────────────────────────────────────┘
```

**Step 3: Saved Configuration (JSON)**
```json
{
  "stepName": "Map Patient Fields",
  "stepType": "data.transformation",
  "config": {
    "mappings": [
      {
        "sourceField": "@patient.identifier",
        "targetField": "output.medicalRecordNumber",
        "transformation": "copy"
      },
      {
        "sourceField": "@patient.name.family",
        "targetField": "output.lastName",
        "transformation": "uppercase"
      },
      {
        "sourceField": "@patient.birthDate",
        "targetField": "output.dob",
        "transformation": "date_format",
        "fromFormat": "YYYYMMDD",
        "toFormat": "YYYY-MM-DD"
      }
    ]
  }
}
```

**Step 4: Runtime Execution**
```go
// FieldMappingExecutor.Execute()
outputData := make(map[string]interface{})

for each mapping:
    1. Resolve source: "@patient.identifier" → "enhancedSegments.PID.fields[2].value"
    2. Extract source value: "MRN123456"
    3. Apply transformation:
       - copy: "MRN123456"
       - uppercase: "MRN123456" → "MRN123456"
       - date_format: "19800101" → "1980-01-01"
    4. Set target: outputData["medicalRecordNumber"] = "MRN123456"

return outputData
```

---

## 🧮 Example 3: Data Enrichment Step

### User Configuration Flow

**Step 1: User Adds Enrichment Step**
```
Pipeline Builder → Drag "API Enrichment" to canvas
```

**Step 2: Configure API Call and Field Mapping**
```
┌─────────────────────────────────────────────────────────┐
│ API Enrichment Configuration                            │
├─────────────────────────────────────────────────────────┤
│                                                          │
│ API Details:                                             │
│ ┌──────────────────────────────────────────────────────┐│
│ │ API Endpoint:    https://epic.hospital.org/api/v1   ││
│ │ Method:          [▼ GET                           ]  ││
│ │ Authentication:  [▼ Bearer Token                  ]  ││
│ │ Token:           [••••••••••••••••••••••••••••••] ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ Request Parameters (Query String):                      │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Parameter Name:  patientId                          ││
│ │ Source Field:    [▼ @patient.identifier          ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ Response Field Mapping:                                 │
│ ┌──────────────────────────────────────────────────────┐│
│ │ API Response:    response.demographics.email        ││
│ │ Target Field:    enrichedData.email                 ││
│ ├──────────────────────────────────────────────────────┤│
│ │ API Response:    response.demographics.phone        ││
│ │ Target Field:    enrichedData.phoneNumber           ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ [+ Add Response Mapping]                                 │
└─────────────────────────────────────────────────────────┘
```

**Step 3: Saved Configuration (JSON)**
```json
{
  "stepName": "Enrich with Epic API",
  "stepType": "pre.enrichment",
  "config": {
    "api": {
      "endpoint": "https://epic.hospital.org/api/v1",
      "method": "GET",
      "auth": {
        "type": "bearer",
        "token": "{{secrets.epic_token}}"
      }
    },
    "requestParams": [
      {
        "paramName": "patientId",
        "sourceField": "@patient.identifier"
      }
    ],
    "responseMapping": [
      {
        "responseField": "response.demographics.email",
        "targetField": "enrichedData.email"
      },
      {
        "responseField": "response.demographics.phone",
        "targetField": "enrichedData.phoneNumber"
      }
    ]
  }
}
```

**Step 4: Runtime Execution**
```go
// EnrichmentExecutor.Execute()

// 1. Build API request
patientId := getNestedValue(inputData, "enhancedSegments.PID.fields[2].value") // Resolved from @patient.identifier
apiURL := config.API.Endpoint + "?patientId=" + patientId

// 2. Call API
response := httpClient.Get(apiURL, headers)

// 3. Map response fields to output
inputData["enrichedData"] = map[string]interface{}{
    "email": response.Demographics.Email,        // From API response
    "phoneNumber": response.Demographics.Phone,  // From API response
}

return inputData // Now includes enrichedData
```

---

## 🔀 Example 4: Conditional Logic Step (If-Then-Else)

### User Configuration Flow

**Step 1: User Adds Conditional Step**
```
Pipeline Builder → Drag "If-Then-Else" to canvas
```

**Step 2: Configure Condition and Actions**
```
┌─────────────────────────────────────────────────────────┐
│ If-Then-Else Configuration                              │
├─────────────────────────────────────────────────────────┤
│                                                          │
│ Condition:                                               │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Source Field:    [▼ @patient.gender              ]  ││
│ │ Operator:        [▼ Equals                        ]  ││
│ │ Compare Value:   [ M                              ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ THEN Action (if condition is true):                     │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Action Type:     [▼ Set Field Value               ]  ││
│ │ Target Field:    [ output.genderFull              ]  ││
│ │ Value:           [ Male                           ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
│ ELSE Action (if condition is false):                    │
│ ┌──────────────────────────────────────────────────────┐│
│ │ Action Type:     [▼ Set Field Value               ]  ││
│ │ Target Field:    [ output.genderFull              ]  ││
│ │ Value:           [ Female                         ]  ││
│ └──────────────────────────────────────────────────────┘│
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Step 3: Saved Configuration (JSON)**
```json
{
  "stepName": "Expand Gender Code",
  "stepType": "conditional.if_then_else",
  "config": {
    "condition": {
      "sourceField": "@patient.gender",
      "operator": "equals",
      "compareValue": "M"
    },
    "thenAction": {
      "actionType": "set_field",
      "targetField": "output.genderFull",
      "value": "Male"
    },
    "elseAction": {
      "actionType": "set_field",
      "targetField": "output.genderFull",
      "value": "Female"
    }
  }
}
```

**Step 4: Runtime Execution**
```go
// IfThenElseExecutor.Execute()

// 1. Evaluate condition
genderValue := getNestedValue(inputData, "enhancedSegments.PID.fields[7].value") // Resolved from @patient.gender

conditionMet := (genderValue == config.Condition.CompareValue) // "M" == "M" → true

// 2. Execute appropriate action
if conditionMet {
    // THEN action
    setNestedValue(inputData, "output.genderFull", "Male")
} else {
    // ELSE action
    setNestedValue(inputData, "output.genderFull", "Female")
}

return inputData
```

---

## 📐 Field Path Selector Component

### Universal UI Component for All Steps

```javascript
/**
 * FieldPathSelector - Reusable component for selecting source/target fields
 * Used by ALL step configuration forms
 */
class FieldPathSelector {
    constructor(container, options = {}) {
        this.container = container;
        this.fieldType = options.fieldType || 'source';  // 'source' or 'target'
        this.currentFormat = options.format || 'hl7v2';
        this.allowCustomPath = options.allowCustomPath !== false;
        this.selectedPath = options.initialValue || '';
        this.onChange = options.onChange || (() => {});

        this.render();
    }

    render() {
        const html = `
            <div class="field-path-selector">
                <label>${this.getLabel()}</label>

                <!-- Semantic Fields (Recommended) -->
                <div class="field-option">
                    <input type="radio" name="pathType-${this.id}" value="semantic"
                           ${this.isSemanticPath() ? 'checked' : ''}>
                    <label>Semantic Field (Recommended)</label>
                    <select class="semantic-select" ${!this.isSemanticPath() ? 'disabled' : ''}>
                        <option value="">Select a field...</option>
                        <optgroup label="Patient Fields">
                            <option value="@patient.identifier">Patient Identifier</option>
                            <option value="@patient.name.family">Last Name</option>
                            <option value="@patient.name.given">First Name</option>
                            <option value="@patient.birthDate">Date of Birth</option>
                            <option value="@patient.gender">Gender</option>
                        </optgroup>
                        <optgroup label="Encounter Fields">
                            <option value="@encounter.id">Encounter ID</option>
                            <option value="@encounter.admitDate">Admit Date</option>
                        </optgroup>
                    </select>
                    <span class="hint">✅ Works across all formats</span>
                </div>

                <!-- Custom Path (Advanced) -->
                ${this.allowCustomPath ? `
                <div class="field-option">
                    <input type="radio" name="pathType-${this.id}" value="custom"
                           ${!this.isSemanticPath() ? 'checked' : ''}>
                    <label>Custom Path (Advanced)</label>
                    <input type="text" class="custom-path-input"
                           placeholder="e.g., enhancedSegments.PID.fields[2].value"
                           value="${!this.isSemanticPath() ? this.selectedPath : ''}"
                           ${this.isSemanticPath() ? 'disabled' : ''}>
                    <span class="hint">⚠️ Format-specific path</span>
                </div>
                ` : ''}

                <!-- Path Preview -->
                <div class="path-preview">
                    <strong>Resolved Path (${this.currentFormat}):</strong>
                    <code>${this.getResolvedPath()}</code>
                </div>
            </div>
        `;

        this.container.innerHTML = html;
        this.attachEvents();
    }

    getLabel() {
        return this.fieldType === 'source'
            ? 'Source Field (where to get data)'
            : 'Target Field (where to put data)';
    }

    isSemanticPath() {
        return this.selectedPath.startsWith('@');
    }

    getResolvedPath() {
        if (!this.selectedPath) return 'No field selected';

        if (this.isSemanticPath()) {
            // In production, call API to resolve
            // For demo, static mapping
            const mappings = {
                '@patient.identifier': 'enhancedSegments.PID.fields[2].value',
                '@patient.name.family': 'enhancedSegments.PID.fields[4].subfields[0].value',
                // ... etc
            };
            return mappings[this.selectedPath] || 'Resolving...';
        }

        return this.selectedPath;  // Custom path used as-is
    }

    attachEvents() {
        // Radio button toggle
        this.container.querySelectorAll('input[type="radio"]').forEach(radio => {
            radio.addEventListener('change', (e) => {
                const isSemantic = e.target.value === 'semantic';
                this.container.querySelector('.semantic-select').disabled = !isSemantic;
                if (this.allowCustomPath) {
                    this.container.querySelector('.custom-path-input').disabled = isSemantic;
                }
            });
        });

        // Semantic field selection
        this.container.querySelector('.semantic-select').addEventListener('change', (e) => {
            this.selectedPath = e.target.value;
            this.updatePreview();
            this.onChange(this.selectedPath);
        });

        // Custom path input
        if (this.allowCustomPath) {
            this.container.querySelector('.custom-path-input').addEventListener('input', (e) => {
                this.selectedPath = e.target.value;
                this.updatePreview();
                this.onChange(this.selectedPath);
            });
        }
    }

    updatePreview() {
        this.container.querySelector('.path-preview code').textContent = this.getResolvedPath();
    }

    getValue() {
        return this.selectedPath;
    }

    setValue(value) {
        this.selectedPath = value;
        this.render();
    }
}
```

---

## 🎨 Integration with Step Configuration Forms

### Example: Validation Step Configuration

```javascript
class ValidationStepConfigForm {
    constructor(step) {
        this.step = step;
        this.rules = step.config.rules || [];
    }

    renderRuleRow(rule, index) {
        const container = document.createElement('div');
        container.className = 'validation-rule-row';

        // Source Field Selector
        const sourceFieldContainer = container.appendChild(document.createElement('div'));
        new FieldPathSelector(sourceFieldContainer, {
            fieldType: 'source',
            format: this.getCurrentFormat(),
            initialValue: rule.sourceField || '',
            onChange: (value) => {
                this.rules[index].sourceField = value;
                this.saveConfig();
            }
        });

        // Validation Type Dropdown
        const validationTypeHTML = `
            <div class="validation-type">
                <label>Validation Type</label>
                <select class="type-select">
                    <option value="required">Required</option>
                    <option value="format">Format Check</option>
                    <option value="length">Length Check</option>
                    <option value="pattern">Regex Pattern</option>
                </select>
            </div>
        `;
        container.innerHTML += validationTypeHTML;

        return container;
    }

    getCurrentFormat() {
        // Get from pipeline configuration or default to hl7v2
        return this.step.pipeline?.inputFormat || 'hl7v2';
    }

    saveConfig() {
        this.step.config.rules = this.rules;
        // Trigger pipeline save
        window.pipelineBuilder.savePipeline();
    }
}
```

---

## 📊 Configuration Summary Table

| Step Type | Source Field Selection | Target Field Selection | Additional Config |
|-----------|------------------------|------------------------|-------------------|
| **Validation** | ✅ Yes (what to validate) | ❌ No (validation only) | Validation type, error message |
| **Field Mapping** | ✅ Yes (source data) | ✅ Yes (where to put result) | Transformation type |
| **Enrichment** | ✅ Yes (API request params) | ✅ Yes (API response mapping) | API endpoint, auth |
| **Conditional Logic** | ✅ Yes (condition field) | ✅ Yes (then/else actions) | Operator, compare value |
| **Data Transformation** | ✅ Yes (input field) | ✅ Yes (output field) | Transformation function |
| **HL7→FHIR Mapping** | ✅ Yes (HL7 segments) | ✅ Yes (FHIR resources) | FHIR version, templates |

**Pattern**: Every step lets user select source field paths for input data and target field paths for output data.

---

## ✨ Key Principles

### 1. **Always Show Field Path Selector**
Every configuration form includes field path selectors for all data inputs/outputs.

### 2. **Semantic by Default, Custom as Fallback**
Recommend semantic fields (`@patient.identifier`), allow custom paths for edge cases.

### 3. **Format-Aware Resolution**
System automatically resolves semantic → actual path based on current message format.

### 4. **Visual Preview**
Always show resolved path so user understands what will happen at runtime.

### 5. **Reusable Component**
`FieldPathSelector` component used across ALL step configuration forms.

---

## 🚀 Implementation Checklist

### Frontend Components
- [ ] Create `FieldPathSelector.js` (universal component)
- [ ] Create `SemanticFieldDropdown.js` (grouped fields)
- [ ] Add path preview/resolution display
- [ ] Create field path validator (checks if path exists)

### Backend API
- [ ] `GET /api/semantic-fields/:format` - Get available semantic fields
- [ ] `POST /api/validate-field-path` - Validate custom path against sample data
- [ ] `POST /api/resolve-field-path` - Resolve semantic → actual path

### Step Configuration Forms
- [ ] Update ValidationStepConfig to use FieldPathSelector
- [ ] Update FieldMappingStepConfig to use FieldPathSelector
- [ ] Update EnrichmentStepConfig to use FieldPathSelector
- [ ] Update ConditionalLogicStepConfig to use FieldPathSelector
- [ ] Apply to ALL 25+ step types

### Testing
- [ ] Test semantic field selection across HL7/FHIR/CCD
- [ ] Test custom path input
- [ ] Test path resolution preview
- [ ] Test step execution with configured paths

---

## 💡 Summary

**The Pattern**:
1. **User configures step** → Selects source field paths (where to get data)
2. **User configures step** → Selects target field paths (where to put results)
3. **System resolves paths** → Semantic fields → Format-specific paths
4. **Step executes** → Extracts data from source paths → Processes it → Writes to target paths

**The Benefit**:
- ✅ User doesn't need to know HL7 field numbers
- ✅ Same configuration works across formats
- ✅ Visual, intuitive interface
- ✅ Advanced users can still use custom paths

---

*Document Generated: January 29, 2025*
*Purpose: Explain step configuration with field path selection*
*Audience: Product, UX, Development teams*
