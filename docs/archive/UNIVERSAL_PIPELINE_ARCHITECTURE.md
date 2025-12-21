# Universal Transformation Pipeline Architecture
## Format-Agnostic, Smart, and Infinitely Reusable

---

## 🎯 Design Principles

### **1. Message Format Abstraction**
**Problem**: Different healthcare formats (HL7, FHIR, CCD, X12, DICOM, etc.)
**Solution**: Universal Message Envelope Pattern

### **2. Pipeline Independence**
**Problem**: Pipeline logic shouldn't know about specific formats
**Solution**: Schema-driven processing with adapters

### **3. Infinite Extensibility**
**Problem**: New formats emerge constantly
**Solution**: Plugin architecture with zero core changes

---

## 📐 Core Architecture

### **Layer 1: Message Envelope (Universal)**

```javascript
// Universal Message Container
{
    // Metadata (format-independent)
    "envelope": {
        "messageId": "uuid-here",
        "receivedAt": "2025-01-29T10:00:00Z",
        "sourceFormat": "hl7v2",              // HL7, FHIR, CCD, X12, etc.
        "sourceVersion": "2.5",               // Format version
        "messageType": "ADT^A01",             // Format-specific type
        "interfaceId": "interface-uuid",
        "correlationId": "correlation-uuid",

        // Schema information
        "schema": {
            "schemaId": "hl7v2-enhanced-schema",
            "version": "1.0",
            "validated": true
        }
    },

    // Parsed content (format-specific but standardized structure)
    "content": {
        // For HL7: Enhanced segments
        // For FHIR: Resource bundle
        // For CCD: XML sections
        // For X12: Segments
    },

    // Processing metadata
    "processing": {
        "pipelineId": "pipeline-uuid",
        "currentStep": 3,
        "stepHistory": [],
        "errors": [],
        "warnings": []
    }
}
```

---

## 🏗️ Architecture Layers

### **Layer 1: Format Adapters (Input)**
**Responsibility**: Convert any format → Universal Envelope

```
┌─────────────────────────────────────────┐
│         Format Adapters (Input)         │
├─────────────────────────────────────────┤
│                                          │
│  HL7Adapter                              │
│  ├─ Parse HL7 → Enhanced Segments       │
│  ├─ Extract metadata (MSH fields)       │
│  └─ Create envelope                     │
│                                          │
│  FHIRAdapter                             │
│  ├─ Parse FHIR JSON → Resource Bundle   │
│  ├─ Extract metadata (Bundle.meta)      │
│  └─ Create envelope                     │
│                                          │
│  CCDAdapter                              │
│  ├─ Parse CCD XML → Sections            │
│  ├─ Extract metadata (header)           │
│  └─ Create envelope                     │
│                                          │
│  X12Adapter                              │
│  ├─ Parse X12 → Segments                │
│  ├─ Extract metadata (ISA/GS)           │
│  └─ Create envelope                     │
│                                          │
│  DIACOMAdapter (future)                 │
│  CSVAdapter (future)                    │
│  HL7FHIRMappingAdapter (HL7→FHIR)      │
│                                          │
└─────────────────────────────────────────┘
         ↓
    Universal Envelope
```

### **Layer 2: Transformation Pipeline (Format-Agnostic)**
**Responsibility**: Process envelopes using format-aware executors

```
┌─────────────────────────────────────────┐
│      Transformation Pipeline            │
│      (Format-Agnostic Core)             │
├─────────────────────────────────────────┤
│                                          │
│  ┌────────────────────────────────┐     │
│  │  Pre-Processing Layer          │     │
│  ├────────────────────────────────┤     │
│  │  • Validation (format-aware)   │     │
│  │  • Enrichment                  │     │
│  │  • Data Quality Checks         │     │
│  └────────────────────────────────┘     │
│           ↓                              │
│  ┌────────────────────────────────┐     │
│  │  Core Transformation Layer     │     │
│  ├────────────────────────────────┤     │
│  │  • HL7 → FHIR                  │     │
│  │  • FHIR → HL7                  │     │
│  │  • CCD → FHIR                  │     │
│  │  • X12 → HL7                   │     │
│  │  • Custom Transformations      │     │
│  └────────────────────────────────┘     │
│           ↓                              │
│  ┌────────────────────────────────┐     │
│  │  Post-Processing Layer         │     │
│  ├────────────────────────────────┤     │
│  │  • Output Validation           │     │
│  │  • Anonymization               │     │
│  │  • Routing Logic               │     │
│  └────────────────────────────────┘     │
│                                          │
└─────────────────────────────────────────┘
         ↓
    Processed Envelope
```

### **Layer 3: Format Adapters (Output)**
**Responsibility**: Convert Universal Envelope → Target Format

```
┌─────────────────────────────────────────┐
│        Format Adapters (Output)         │
├─────────────────────────────────────────┤
│                                          │
│  HL7Serializer                           │
│  ├─ Envelope → HL7 Message              │
│  └─ Validate HL7 format                 │
│                                          │
│  FHIRSerializer                          │
│  ├─ Envelope → FHIR Bundle              │
│  └─ Validate FHIR spec                  │
│                                          │
│  CCDSerializer                           │
│  ├─ Envelope → CCD XML                  │
│  └─ Validate CDA schema                 │
│                                          │
│  X12Serializer                           │
│  ├─ Envelope → X12 Transaction          │
│  └─ Validate X12 syntax                 │
│                                          │
│  JSONSerializer (generic)                │
│  PDFSerializer (reports)                │
│                                          │
└─────────────────────────────────────────┘
         ↓
    Output Message
```

---

## 🔧 Smart Executor Design (Format-Aware)

### **Executor Interface (Universal)**

```go
// Universal Executor Interface
type Executor interface {
    // Metadata
    GetID() string
    GetName() string
    GetSupportedFormats() []string  // ["hl7v2", "fhir", "ccd", "*"]
    GetType() string                // "validation", "transformation", etc.

    // Execution
    Execute(ctx context.Context, envelope *MessageEnvelope) (*ExecutionResult, error)

    // Validation
    CanProcess(envelope *MessageEnvelope) bool
    Validate(config map[string]interface{}) error
}

// Message Envelope
type MessageEnvelope struct {
    Envelope   EnvelopeMetadata       `json:"envelope"`
    Content    interface{}            `json:"content"`  // Flexible content
    Processing ProcessingMetadata     `json:"processing"`
}

type EnvelopeMetadata struct {
    MessageID     string    `json:"messageId"`
    ReceivedAt    time.Time `json:"receivedAt"`
    SourceFormat  string    `json:"sourceFormat"`   // "hl7v2", "fhir", "ccd"
    SourceVersion string    `json:"sourceVersion"`  // "2.5", "R4", etc.
    MessageType   string    `json:"messageType"`    // Format-specific
    InterfaceID   string    `json:"interfaceId"`
    Schema        SchemaInfo `json:"schema"`
}
```

### **Format-Specific Executors**

```go
// HL7 Validation Executor
type HL7ValidationExecutor struct {
    BaseExecutor
}

func (e *HL7ValidationExecutor) GetSupportedFormats() []string {
    return []string{"hl7v2"}  // Only processes HL7
}

func (e *HL7ValidationExecutor) Execute(ctx context.Context, envelope *MessageEnvelope) (*ExecutionResult, error) {
    // Type assertion for HL7 content
    hl7Content, ok := envelope.Content.(*HL7EnhancedContent)
    if !ok {
        return nil, fmt.Errorf("invalid content type for HL7 executor")
    }

    // HL7-specific validation
    return e.validateHL7Fields(hl7Content)
}

// FHIR Validation Executor
type FHIRValidationExecutor struct {
    BaseExecutor
}

func (e *FHIRValidationExecutor) GetSupportedFormats() []string {
    return []string{"fhir"}  // Only processes FHIR
}

func (e *FHIRValidationExecutor) Execute(ctx context.Context, envelope *MessageEnvelope) (*ExecutionResult, error) {
    // Type assertion for FHIR content
    fhirBundle, ok := envelope.Content.(*FHIRBundle)
    if !ok {
        return nil, fmt.Errorf("invalid content type for FHIR executor")
    }

    // FHIR-specific validation (profile conformance)
    return e.validateFHIRResources(fhirBundle)
}

// Universal Validation Executor
type UniversalValidationExecutor struct {
    BaseExecutor
}

func (e *UniversalValidationExecutor) GetSupportedFormats() []string {
    return []string{"*"}  // Processes any format
}

func (e *UniversalValidationExecutor) Execute(ctx context.Context, envelope *MessageEnvelope) (*ExecutionResult, error) {
    // Format-agnostic validation (required fields, data types, etc.)
    switch envelope.Envelope.SourceFormat {
    case "hl7v2":
        return e.validateHL7(envelope)
    case "fhir":
        return e.validateFHIR(envelope)
    case "ccd":
        return e.validateCCD(envelope)
    default:
        return e.validateGeneric(envelope)
    }
}
```

---

## 📋 Step Configuration (Format-Aware)

### **Database Schema**

```sql
-- Transformation Steps (format-aware)
CREATE TABLE transformation_steps (
    id UUID PRIMARY KEY,
    pipeline_id UUID NOT NULL,
    step_name VARCHAR(255) NOT NULL,
    step_type VARCHAR(100) NOT NULL,
    sequence INTEGER NOT NULL,

    -- Format awareness
    source_formats TEXT[],  -- ['hl7v2', 'fhir'] or ['*'] for universal
    target_format VARCHAR(50),  -- 'fhir', 'hl7v2', null for passthrough

    -- Executor selection
    executor_id VARCHAR(100) NOT NULL,  -- 'hl7_validation', 'fhir_validation', 'universal_validation'

    -- Configuration (format-specific)
    config JSONB,  -- Flexible config based on executor

    -- Conditional execution
    condition JSONB,  -- { "if": "envelope.sourceFormat == 'hl7v2'" }

    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Example step configurations
INSERT INTO transformation_steps VALUES
(
    uuid_generate_v4(),
    'pipeline-uuid',
    'Validate HL7 Required Fields',
    'pre.validation',
    10,
    ARRAY['hl7v2'],  -- Only runs for HL7
    NULL,
    'hl7_field_validation',
    '{
        "rules": [
            {"field": "MSH.9", "type": "required"},
            {"field": "PID.3", "type": "required"}
        ]
    }',
    NULL,
    true
),
(
    uuid_generate_v4(),
    'pipeline-uuid',
    'Validate FHIR Bundle',
    'pre.validation',
    10,
    ARRAY['fhir'],  -- Only runs for FHIR
    NULL,
    'fhir_bundle_validation',
    '{
        "profiles": ["http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"],
        "strictMode": true
    }',
    NULL,
    true
),
(
    uuid_generate_v4(),
    'pipeline-uuid',
    'Transform HL7 to FHIR',
    'core.transformation',
    100,
    ARRAY['hl7v2'],  -- Input: HL7
    'fhir',          -- Output: FHIR
    'hl7_to_fhir_transformer',
    '{
        "fhirVersion": "R4",
        "mappingTemplate": "standard_adt_a01"
    }',
    NULL,
    true
);
```

---

## 🔄 Execution Flow

### **Step 1: Message Arrives**
```
Raw HL7 Message
    ↓
[Format Detector] → Identifies format: "hl7v2"
    ↓
[HL7Adapter] → Parses HL7 → Creates Universal Envelope
    ↓
{
    "envelope": {
        "sourceFormat": "hl7v2",
        "messageType": "ADT^A01"
    },
    "content": { /* HL7 enhanced segments */ }
}
```

### **Step 2: Pipeline Selection**
```
SELECT * FROM transformation_pipelines
WHERE interface_id = $1
  AND (
    'hl7v2' = ANY(supported_formats)  -- Pipeline supports HL7
    OR '*' = ANY(supported_formats)    -- Universal pipeline
  )
```

### **Step 3: Step Filtering**
```
For each step in pipeline:
    if step.source_formats contains envelope.sourceFormat OR '*':
        if step.condition is null OR evaluateCondition(step.condition, envelope):
            executeStep(step, envelope)
```

### **Step 4: Execution**
```go
func executeStep(step *TransformationStep, envelope *MessageEnvelope) error {
    // Get executor
    executor := getExecutor(step.ExecutorID)

    // Check if executor can process this format
    if !executor.CanProcess(envelope) {
        return fmt.Errorf("executor %s cannot process format %s",
            executor.GetID(), envelope.Envelope.SourceFormat)
    }

    // Execute
    result, err := executor.Execute(ctx, envelope)

    // Update envelope
    if step.TargetFormat != "" {
        envelope.Envelope.SourceFormat = step.TargetFormat
    }

    return err
}
```

---

## 🎨 UI Design (Format-Agnostic)

### **Pipeline Builder Enhancement**

```javascript
// Step Template with Format Awareness
class StepTemplate {
    constructor({
        id,
        name,
        type,
        supportedFormats,  // ['hl7v2', 'fhir', '*']
        targetFormat,      // 'fhir', 'hl7v2', null
        config
    }) {
        this.id = id;
        this.name = name;
        this.type = type;
        this.supportedFormats = supportedFormats;
        this.targetFormat = targetFormat;
        this.config = config;
    }

    canProcessFormat(sourceFormat) {
        return this.supportedFormats.includes('*')
            || this.supportedFormats.includes(sourceFormat);
    }
}

// Example: Format-Specific Steps
const hl7ValidationStep = new StepTemplate({
    id: 'hl7-validation',
    name: 'HL7 Field Validation',
    type: 'pre.validation',
    supportedFormats: ['hl7v2'],  // HL7 only
    targetFormat: null,
    config: { /* HL7-specific config */ }
});

const fhirValidationStep = new StepTemplate({
    id: 'fhir-validation',
    name: 'FHIR Resource Validation',
    type: 'pre.validation',
    supportedFormats: ['fhir'],  // FHIR only
    targetFormat: null,
    config: { /* FHIR-specific config */ }
});

const hl7ToFhirStep = new StepTemplate({
    id: 'hl7-to-fhir',
    name: 'HL7 → FHIR Transformation',
    type: 'core.transformation',
    supportedFormats: ['hl7v2'],  // Input: HL7
    targetFormat: 'fhir',         // Output: FHIR
    config: { /* Transformation config */ }
});

const universalEnrichmentStep = new StepTemplate({
    id: 'universal-enrichment',
    name: 'Patient Data Enrichment',
    type: 'pre.enrichment',
    supportedFormats: ['*'],  // Works with any format
    targetFormat: null,
    config: { /* Format-agnostic enrichment */ }
});
```

### **Toolbox Filtering**

```javascript
class ToolboxManager {
    filterStepsByFormat(currentFormat) {
        return this.allSteps.filter(step =>
            step.canProcessFormat(currentFormat)
        );
    }

    renderToolbox(pipelineFormat) {
        const compatibleSteps = this.filterStepsByFormat(pipelineFormat);

        // Group by category
        const categories = {
            'HL7-Specific': compatibleSteps.filter(s => s.supportedFormats.includes('hl7v2')),
            'FHIR-Specific': compatibleSteps.filter(s => s.supportedFormats.includes('fhir')),
            'Universal': compatibleSteps.filter(s => s.supportedFormats.includes('*')),
            'Transformations': compatibleSteps.filter(s => s.targetFormat !== null)
        };

        // Render UI
        this.render(categories);
    }
}
```

---

## 🧩 Reusable Components

### **1. Format Registry**
```javascript
class FormatRegistry {
    static formats = {
        'hl7v2': {
            name: 'HL7 v2.x',
            versions: ['2.3', '2.4', '2.5', '2.6', '2.7', '2.8'],
            adapter: 'HL7Adapter',
            serializer: 'HL7Serializer',
            icon: 'fas fa-file-medical',
            color: '#3b82f6'
        },
        'fhir': {
            name: 'FHIR',
            versions: ['DSTU2', 'STU3', 'R4', 'R5'],
            adapter: 'FHIRAdapter',
            serializer: 'FHIRSerializer',
            icon: 'fas fa-fire',
            color: '#ef4444'
        },
        'ccd': {
            name: 'CCD (C-CDA)',
            versions: ['R1.1', 'R2.1'],
            adapter: 'CCDAdapter',
            serializer: 'CCDSerializer',
            icon: 'fas fa-file-code',
            color: '#22c55e'
        },
        'x12': {
            name: 'X12 EDI',
            versions: ['5010', '5010A'],
            adapter: 'X12Adapter',
            serializer: 'X12Serializer',
            icon: 'fas fa-money-check',
            color: '#f59e0b'
        }
    };

    static getAdapter(format) {
        return this.formats[format]?.adapter;
    }

    static getSerializer(format) {
        return this.formats[format]?.serializer;
    }
}
```

### **2. Executor Registry (Plugin Architecture)**
```go
// Global executor registry
var executorRegistry = make(map[string]Executor)

// Register executors at startup
func init() {
    // HL7-specific
    RegisterExecutor("hl7_field_validation", &HL7ValidationExecutor{})
    RegisterExecutor("hl7_segment_validation", &HL7SegmentValidationExecutor{})
    RegisterExecutor("hl7_to_fhir", &HL7ToFHIRTransformer{})

    // FHIR-specific
    RegisterExecutor("fhir_bundle_validation", &FHIRBundleValidationExecutor{})
    RegisterExecutor("fhir_profile_validation", &FHIRProfileValidationExecutor{})
    RegisterExecutor("fhir_to_hl7", &FHIRToHL7Transformer{})

    // CCD-specific
    RegisterExecutor("ccd_section_validation", &CCDSectionValidationExecutor{})
    RegisterExecutor("ccd_to_fhir", &CCDToFHIRTransformer{})

    // Universal
    RegisterExecutor("universal_enrichment", &UniversalEnrichmentExecutor{})
    RegisterExecutor("universal_routing", &UniversalRoutingExecutor{})
    RegisterExecutor("universal_logging", &UniversalLoggingExecutor{})
}

func RegisterExecutor(id string, executor Executor) {
    executorRegistry[id] = executor
}

func GetExecutor(id string) (Executor, error) {
    executor, ok := executorRegistry[id]
    if !ok {
        return nil, fmt.Errorf("executor not found: %s", id)
    }
    return executor, nil
}
```

---

## 📊 Content Type Patterns

### **HL7 Content Structure**
```json
{
    "envelope": { "sourceFormat": "hl7v2" },
    "content": {
        "enhancedSegments": {
            "MSH": { /* segment details */ },
            "PID": { /* segment details */ }
        },
        "segmentOrder": ["MSH", "PID", "PV1"],
        "messageType": { "code": "ADT", "event": "A01" }
    }
}
```

### **FHIR Content Structure**
```json
{
    "envelope": { "sourceFormat": "fhir" },
    "content": {
        "resourceType": "Bundle",
        "type": "message",
        "entry": [
            { "resource": { "resourceType": "Patient", /* ... */ } },
            { "resource": { "resourceType": "Encounter", /* ... */ } }
        ]
    }
}
```

### **CCD Content Structure**
```json
{
    "envelope": { "sourceFormat": "ccd" },
    "content": {
        "header": { /* CDA header */ },
        "sections": [
            { "code": "allergies", "entries": [ /* ... */ ] },
            { "code": "medications", "entries": [ /* ... */ ] }
        ]
    }
}
```

---

## 🚀 Benefits of This Architecture

### **1. Format Independence**
- Add new formats without changing pipeline core
- HL7, FHIR, CCD, X12, DICOM, CSV - all treated equally

### **2. Infinite Reusability**
- Universal executors work with any format
- Format-specific executors for specialized needs
- Mix and match in any pipeline

### **3. Smart Filtering**
- UI shows only compatible steps
- Backend validates format compatibility
- No runtime format mismatch errors

### **4. Zero Coupling**
- Pipeline doesn't know about HL7/FHIR/CCD specifics
- Executors are self-contained plugins
- Easy to add/remove/swap executors

### **5. Future-Proof**
- New healthcare formats (HL7 v3, custom JSONs)
- IoT health data (wearables, sensors)
- AI/ML model outputs
- Just add adapter + executor

---

## 📝 Implementation Roadmap

### **Phase 1: Foundation** (Current)
- ✅ HL7 adapter implemented
- ✅ JSON conversion pipeline
- ⏳ Universal envelope structure

### **Phase 2: Multi-Format Support**
- Add FHIR adapter
- Add CCD adapter
- Implement format registry
- Update UI for format selection

### **Phase 3: Executor Framework**
- Implement executor interface
- Create format-specific executors
- Build executor registry
- Add plugin loading system

### **Phase 4: Advanced Features**
- Bi-directional transformations (HL7↔FHIR)
- Multi-format pipelines (HL7→FHIR→CCD)
- Format conversion testing
- Visual format mapping tool

---

## ✨ Summary

**This architecture provides**:
- ✅ **Format Agnostic**: Works with any healthcare format
- ✅ **Smart**: Auto-filters incompatible steps
- ✅ **Reusable**: Executors shared across formats
- ✅ **Extensible**: Plugin architecture for new formats
- ✅ **Maintainable**: Clean separation of concerns
- ✅ **Scalable**: Add formats without changing core

**Key Innovation**: Universal Message Envelope + Format-Aware Executors = Infinite Flexibility
