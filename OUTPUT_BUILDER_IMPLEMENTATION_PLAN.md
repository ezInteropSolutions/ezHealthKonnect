# Output Builder Implementation Plan

## Executive Summary

This document outlines a minimal-lift implementation plan for adding a **Visual Output Builder** to the ezHealthKonnect pipeline system. The output builder enables users to define what the final message looks like using a no-code interface, supporting HL7, FHIR, and custom JSON output formats.

---

## Current State Analysis

### What Already Exists (Reusable)

| Component | Location | Reuse Strategy |
|-----------|----------|----------------|
| **DeliveryPayload** | `models/delivery_models.go` | Extend with output template reference |
| **TransmissionPayload** | `models/delivery_models.go` | Use as-is for final bytes |
| **PipelineExecutionContext** | `models/transformation_models.go` | Add `Output` field for `tmp` |
| **hl7_fhir_templates** | `V9 migration` | Reference for FHIR output templates |
| **transformation_templates** | `V20 migration` | Reference for step templates |
| **VisualStep/VisualPipeline** | `PipelineModels.js` | Add output contribution config |
| **ResultMappingBuilder** | `ResultMappingBuilder.js` | Reuse pattern for output field mapping |
| **FieldPathInput** | `FieldPathInput.js` | Reuse for target path selection |
| **HL7FHIRTransformServiceV3** | `hl7_fhir_transform_service_v3.go` | Already creates FHIR output |
| **PassthroughExecutor** | `executor_registry.go` | Already creates delivery payloads |

### What Needs to Be Built

| Component | Effort | Priority |
|-----------|--------|----------|
| Output template database schema | Low | P0 |
| OutputTemplate Go model | Low | P0 |
| Add `Output` to execution context | Low | P0 |
| Output Builder Step executor | Medium | P0 |
| Visual Output Builder UI | Medium | P1 |
| HL7 Serializer (JSON → HL7 string) | Medium | P1 |
| Step output contribution toggle | Low | P2 |

---

## Architecture Design

### Core Concept: `msg` + `tmp`

```
┌─────────────────────────────────────────────────────────────┐
│ Pipeline Execution Context                                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  msg (Read-Only)              tmp (Output Being Built)      │
│  ┌──────────────────┐         ┌──────────────────────┐      │
│  │ Original input   │         │ Starts empty or from │      │
│  │ parsed to JSON   │         │ template at pipeline │      │
│  │                  │         │ start                │      │
│  └──────────────────┘         └──────────────────────┘      │
│                                                              │
│  step_outputs                                                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Each step's isolated output for reference            │   │
│  │ step_outputs.database.mrn                            │   │
│  │ step_outputs.api.risk_score                          │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Output Template Data Model

```go
// OutputTemplate defines the structure and field mappings for output message
type OutputTemplate struct {
    ID              string                 `json:"id"`
    PipelineID      string                 `json:"pipeline_id"`

    // Format configuration
    Format          string                 `json:"format"`           // hl7v2, fhir, json
    FormatVersion   string                 `json:"format_version"`   // 2.5, R4, etc.
    OutputEncoding  string                 `json:"output_encoding"`  // json, xml, er7

    // Template initialization
    InitMode        string                 `json:"init_mode"`        // empty, copy_input, template
    BaseTemplateID  string                 `json:"base_template_id,omitempty"` // For FHIR: hl7_fhir_templates ref

    // Element definitions
    Elements        []OutputElement        `json:"elements"`

    // Metadata
    CreatedAt       time.Time              `json:"created_at"`
    UpdatedAt       time.Time              `json:"updated_at"`
}

// OutputElement defines a single field/segment/resource in the output
type OutputElement struct {
    ID              string                 `json:"id"`
    ParentID        *string                `json:"parent_id,omitempty"`

    // Element identification
    ElementType     string                 `json:"element_type"`     // segment, resource, field
    Name            string                 `json:"name"`             // MSH, Patient, PID.3
    Path            string                 `json:"path"`             // Full path

    // Cardinality
    Cardinality     string                 `json:"cardinality"`      // required, optional, repeating

    // Value source
    ValueType       string                 `json:"value_type"`       // literal, variable, expression
    Value           string                 `json:"value"`            // Static value or ${variable}

    // For repeating elements
    RepeatSource    string                 `json:"repeat_source,omitempty"`

    // Children (nested)
    Children        []OutputElement        `json:"children,omitempty"`
}
```

---

## Implementation Phases

### Phase 1: Core Infrastructure (P0) - 2-3 days

#### 1.1 Database Migration (V39)

```sql
-- V39__Output_Template_Support.sql

-- Output templates table
CREATE TABLE IF NOT EXISTS output_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES transformation_pipelines(id) ON DELETE CASCADE,

    -- Format configuration
    format VARCHAR(20) NOT NULL,           -- hl7v2, fhir, json
    format_version VARCHAR(20),            -- 2.5, R4
    output_encoding VARCHAR(20) DEFAULT 'json', -- json, xml, er7

    -- Initialization
    init_mode VARCHAR(20) DEFAULT 'empty', -- empty, copy_input, template
    base_template_id UUID,                 -- Reference to hl7_fhir_templates if using template

    -- Template definition
    elements JSONB NOT NULL DEFAULT '[]',

    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_output_template_pipeline UNIQUE (pipeline_id)
);

CREATE INDEX idx_output_templates_pipeline ON output_templates(pipeline_id);
CREATE INDEX idx_output_templates_format ON output_templates(format);

-- Add output_contribution config to transformation_steps
ALTER TABLE transformation_steps
ADD COLUMN IF NOT EXISTS output_contribution JSONB DEFAULT NULL;

COMMENT ON COLUMN transformation_steps.output_contribution IS
'Optional config for step to contribute fields to output (tmp). Format: {enabled: bool, mappings: [{source, target}]}';
```

#### 1.2 Go Model Updates

**File: `models/output_template_models.go` (NEW)**

```go
package models

import "time"

// OutputTemplate defines the output message structure for a pipeline
type OutputTemplate struct {
    ID              string           `json:"id"`
    PipelineID      string           `json:"pipeline_id"`
    Format          string           `json:"format"`
    FormatVersion   string           `json:"format_version,omitempty"`
    OutputEncoding  string           `json:"output_encoding"`
    InitMode        string           `json:"init_mode"`
    BaseTemplateID  string           `json:"base_template_id,omitempty"`
    Elements        []OutputElement  `json:"elements"`
    CreatedAt       time.Time        `json:"created_at"`
    UpdatedAt       time.Time        `json:"updated_at"`
}

// OutputElement defines a single element in the output template
type OutputElement struct {
    ID           string          `json:"id"`
    ParentID     *string         `json:"parent_id,omitempty"`
    ElementType  string          `json:"element_type"`
    Name         string          `json:"name"`
    Path         string          `json:"path"`
    Cardinality  string          `json:"cardinality"`
    ValueType    string          `json:"value_type"`
    Value        string          `json:"value"`
    RepeatSource string          `json:"repeat_source,omitempty"`
    Children     []OutputElement `json:"children,omitempty"`
}

// StepOutputContribution defines how a step contributes to the output
type StepOutputContribution struct {
    Enabled  bool                  `json:"enabled"`
    Mappings []OutputFieldMapping  `json:"mappings"`
}

// OutputFieldMapping maps a step result field to an output path
type OutputFieldMapping struct {
    SourceField string `json:"source_field"` // Field from step result
    TargetPath  string `json:"target_path"`  // Path in output (tmp)
}
```

**File: `models/transformation_models.go` (UPDATE)**

```go
// Add Output field to PipelineExecutionContext
type PipelineExecutionContext struct {
    Message         map[string]interface{}            `json:"message"`          // msg (input)
    Output          map[string]interface{}            `json:"output"`           // tmp (output being built) ← NEW
    StepOutputs     map[string]StepOutput             `json:"step_outputs"`
    VariableContext *PipelineVariableContext          `json:"variable_context"`
    Metadata        map[string]interface{}            `json:"metadata"`
}
```

#### 1.3 Output Template Service

**File: `services/output_template_service.go` (NEW)**

```go
package services

// OutputTemplateService handles output template CRUD and initialization
type OutputTemplateService struct {
    db *sql.DB
}

func NewOutputTemplateService(db *sql.DB) *OutputTemplateService {
    return &OutputTemplateService{db: db}
}

// GetTemplateForPipeline retrieves the output template for a pipeline
func (s *OutputTemplateService) GetTemplateForPipeline(ctx context.Context, pipelineID string) (*models.OutputTemplate, error)

// InitializeOutput creates initial tmp based on template configuration
func (s *OutputTemplateService) InitializeOutput(
    ctx context.Context,
    template *models.OutputTemplate,
    inputData map[string]interface{},
) (map[string]interface{}, error)

// ApplyStepContribution applies a step's output contribution to tmp
func (s *OutputTemplateService) ApplyStepContribution(
    tmp map[string]interface{},
    stepOutput map[string]interface{},
    contribution *models.StepOutputContribution,
) error

// ResolveVariables replaces ${variable} references with actual values
func (s *OutputTemplateService) ResolveVariables(
    template *models.OutputTemplate,
    msg map[string]interface{},
    stepOutputs map[string]interface{},
) (map[string]interface{}, error)
```

#### 1.4 Pipeline Execution Updates

**File: `services/transformation_pipeline_helpers.go` (UPDATE)**

```go
// In ExecutePipeline function:

// 1. Initialize tmp based on output template
outputTemplate, err := tps.outputTemplateService.GetTemplateForPipeline(ctx, pipeline.ID)
if err == nil && outputTemplate != nil {
    execCtx.Output, err = tps.outputTemplateService.InitializeOutput(ctx, outputTemplate, inputData)
    if err != nil {
        log.Printf("⚠️ Failed to initialize output template: %v", err)
        execCtx.Output = make(map[string]interface{})
    }
} else {
    execCtx.Output = make(map[string]interface{})
}

// 2. After each step, check for output contribution
if step.OutputContribution != nil && step.OutputContribution.Enabled {
    err := tps.outputTemplateService.ApplyStepContribution(
        execCtx.Output,
        stepOutput.OutputData,
        step.OutputContribution,
    )
    if err != nil {
        log.Printf("⚠️ Failed to apply output contribution: %v", err)
    }
}

// 3. At end, use execCtx.Output for delivery payload
```

---

### Phase 2: Output Builder UI (P1) - 3-4 days

#### 2.1 Visual Output Builder Component

**File: `public/js/pipeline/components/OutputBuilderPanel.js` (NEW)**

Key features:
- Tab-based format selection (HL7, FHIR, JSON)
- Visual element tree with drag-drop ordering
- Variable picker for `${msg.X}` and `${step_outputs.Y}` references
- Live preview panel
- Cardinality indicators (Required, Optional, Repeating)

```javascript
class OutputBuilderPanel {
    constructor(container, options = {}) {
        this.container = container;
        this.format = options.format || 'fhir';
        this.template = options.template || null;
        this.variableContext = options.variableContext || {};
    }

    render() {
        // Main structure with format tabs and element tree
    }

    renderFormatSelector() {
        // HL7 | FHIR | JSON tabs
    }

    renderElementTree() {
        // Hierarchical view of output structure
    }

    renderVariablePicker() {
        // Available variables from msg and step_outputs
    }

    renderPreview() {
        // Live preview of output with sample data
    }

    getValue() {
        // Return OutputTemplate JSON
    }
}
```

#### 2.2 HL7 Segment Builder

**File: `public/js/pipeline/components/HL7SegmentBuilder.js` (NEW)**

```javascript
class HL7SegmentBuilder {
    constructor(container, options = {}) {
        this.container = container;
        this.segments = options.segments || [];
        this.schema = options.schema || null; // HL7 schema for field definitions
    }

    renderSegment(segment) {
        // Visual segment with fields
        // MSH.3 [${msg.MSH.3}  ▼]
    }

    addSegment(segmentType) {
        // Add new segment with schema-based fields
    }

    setCardinality(segmentId, cardinality) {
        // Required | Optional | Repeating
    }
}
```

#### 2.3 FHIR Resource Builder

**File: `public/js/pipeline/components/FHIRResourceBuilder.js` (NEW)**

```javascript
class FHIRResourceBuilder {
    constructor(container, options = {}) {
        this.container = container;
        this.resources = options.resources || [];
        this.schema = options.schema || null; // FHIR schema
    }

    renderResource(resource) {
        // Visual resource with elements
    }

    addResource(resourceType) {
        // Add Patient, Encounter, Observation, etc.
    }

    renderElement(element, parentPath) {
        // Recursive element rendering with variable support
    }
}
```

---

### Phase 3: Serializers (P1) - 2-3 days

#### 3.1 HL7 Serializer

**File: `services/serializers/hl7_serializer.go` (NEW)**

```go
package serializers

// HL7Serializer converts output JSON to HL7 ER7 string
type HL7Serializer struct {
    fieldSeparator    string
    componentSep      string
    repetitionSep     string
    escapeSep         string
    subcomponentSep   string
}

func NewHL7Serializer() *HL7Serializer {
    return &HL7Serializer{
        fieldSeparator:  "|",
        componentSep:    "^",
        repetitionSep:   "~",
        escapeSep:       "\\",
        subcomponentSep: "&",
    }
}

// Serialize converts output JSON to HL7 string
func (s *HL7Serializer) Serialize(output map[string]interface{}) (string, error) {
    // 1. Get segment order
    // 2. For each segment, build field string
    // 3. Handle components and subcomponents
    // 4. Apply escaping
    // 5. Join with segment terminators
}

// SerializeSegment builds a single HL7 segment string
func (s *HL7Serializer) SerializeSegment(segmentName string, fields map[string]interface{}) (string, error)

// EscapeValue escapes special characters in values
func (s *HL7Serializer) EscapeValue(value string) string
```

#### 3.2 Output Serializer Factory

**File: `services/serializers/serializer_factory.go` (NEW)**

```go
package serializers

type OutputSerializer interface {
    Serialize(output map[string]interface{}) ([]byte, string, error) // bytes, contentType, error
}

func GetSerializer(format string, encoding string) (OutputSerializer, error) {
    switch format {
    case "fhir":
        if encoding == "xml" {
            return NewFHIRXMLSerializer(), nil
        }
        return NewJSONSerializer("application/fhir+json"), nil
    case "hl7v2":
        if encoding == "er7" {
            return NewHL7Serializer(), nil
        }
        return NewHL7XMLSerializer(), nil
    case "json":
        return NewJSONSerializer("application/json"), nil
    default:
        return NewJSONSerializer("application/json"), nil
    }
}
```

---

### Phase 4: Step Output Contribution (P2) - 1-2 days

#### 4.1 UI Toggle for Steps

Add to PropertiesPanel.js:

```javascript
renderOutputContribution(step) {
    return `
        <div class="output-contribution-section">
            <div class="section-header">
                <input type="checkbox" id="enableOutputContribution"
                       ${step.outputContribution?.enabled ? 'checked' : ''}>
                <label for="enableOutputContribution">Add to Output (tmp)</label>
            </div>

            <div class="contribution-mappings" style="${step.outputContribution?.enabled ? '' : 'display:none'}">
                <table class="mapping-table">
                    <thead>
                        <tr>
                            <th>From Step Result</th>
                            <th>To Output Path</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="contributionMappings"></tbody>
                </table>
                <button class="btn btn-sm btn-primary" id="addContributionMapping">
                    <i class="fas fa-plus"></i> Add Mapping
                </button>
            </div>
        </div>
    `;
}
```

---

## Reuse Matrix

| Existing Component | Reuse For | Changes Needed |
|-------------------|-----------|----------------|
| `ResultMappingBuilder` | Output field mapping UI | Minimal - different labels |
| `FieldPathInput` | Target path input | None - use as-is |
| `FieldPathSelector` | Variable picker | Add step_outputs source |
| `VisualStep.config` | Store output_contribution | Add property |
| `PipelineExecutionContext` | Add Output field | One field addition |
| `DeliveryPayload` | Final output delivery | Use as-is |
| `hl7_fhir_templates` | FHIR template source | Reference only |

---

## Migration Path

1. **V39 Migration**: Add output_templates table and output_contribution column
2. **Existing Pipelines**: Continue working as-is (no output template = legacy behavior)
3. **New Pipelines**: Can optionally use output builder
4. **HL7→FHIR**: Existing HL7FHIRMappingExecutor creates FHIR directly (no change needed)
5. **Gradual Adoption**: Steps can individually enable output contribution

---

## API Endpoints

```
# Output Template Management
GET    /api/pipelines/:id/output-template
POST   /api/pipelines/:id/output-template
PUT    /api/pipelines/:id/output-template
DELETE /api/pipelines/:id/output-template

# Template Preview/Validation
POST   /api/pipelines/:id/output-template/preview
POST   /api/pipelines/:id/output-template/validate

# Schema/Hints
GET    /api/output-schemas/hl7/:version
GET    /api/output-schemas/fhir/:version/:resourceType
```

---

## Effort Estimate Summary

| Phase | Scope | Effort |
|-------|-------|--------|
| Phase 1 | Core Infrastructure | 2-3 days |
| Phase 2 | Output Builder UI | 3-4 days |
| Phase 3 | Serializers | 2-3 days |
| Phase 4 | Step Contribution | 1-2 days |
| **Total** | **Full Implementation** | **8-12 days** |

### Minimal MVP (Just HL7→FHIR with tmp)
- Phase 1 only: 2-3 days
- FHIR output works via existing HL7FHIRMappingExecutor
- Steps can add to tmp via output_contribution

---

## Decision Points

1. **Template Storage**: JSONB in PostgreSQL vs separate MongoDB collection?
   - **Recommendation**: PostgreSQL JSONB (consistent with existing pattern)

2. **HL7 Serializer Approach**: Full schema-driven vs preserve-original?
   - **Recommendation**: Start with preserve-original + delta for safety

3. **UI Integration**: Separate tab vs inline in properties panel?
   - **Recommendation**: Pipeline-level tab for output template, step-level inline for contribution

4. **Default Behavior**: What happens if no output template defined?
   - **Recommendation**: Fall back to existing behavior (HL7FHIRMappingExecutor output or passthrough)

---

## Next Steps

1. Review and approve this plan
2. Create V39 database migration
3. Implement Phase 1 core infrastructure
4. Build MVP UI for output builder
5. Test with existing HL7→FHIR pipeline
6. Add HL7 serializer for HL7→HL7 use cases
