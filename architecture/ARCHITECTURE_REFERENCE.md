# 🧠 ezHealthKonnect Architecture Reference & Memory

## 📋 **Project Overview**

**Mission**: Build a Universal Healthcare Data Integration Engine capable of processing millions of messages with Out-of-Box (OOB) templates and visual no-code customization.

**Core Philosophy**:
- **OOB + Customization**: Start with high-confidence templates, customize as needed
- **Universal Format Support**: HL7, FHIR, CCD, EDI, Excel, Parquet, Avro, Database, Flat Files
- **No-Code Visual Interface**: Drag-drop logic builder with conditions, loops, transformations
- **Production Scale**: Million+ message processing capability

---

## 🏗️ **Architecture Decisions Made**

### **1. Hierarchical Mapping Strategy**
```
Master OOB Templates (Immutable)
    ↓
Interface-Specific Overrides (Customizable)
    ↓
Runtime Resolution (Smart Merging)
```

**Key Principle**: Master templates are **READ-ONLY** golden standards. Customizations are stored separately and merged at runtime.

### **2. Universal Data Integration Architecture**
```
┌─────────────────────────────────────────────────────────────────┐
│                    INTERFACE ENGINE                             │
├─────────────────────────────────────────────────────────────────┤
│  🔄 TRANSFORMATION ORCHESTRATOR (Format Agnostic)              │
├─────────────────────────────────────────────────────────────────┤
│  📊 DATA MODEL ABSTRACTION LAYER                               │
├─────────────────────────────────────────────────────────────────┤
│  🔌 FORMAT ADAPTERS (Pluggable)                                │
├─────────────────────────────────────────────────────────────────┤
│  📦 SOURCE CONNECTORS | 🎯 TARGET CONNECTORS                   │
└─────────────────────────────────────────────────────────────────┘
```

### **3. User Journey: Wizard → Deploy → Enhance**
**Decision**: Progressive enhancement approach beats in-wizard complexity
- **Step 1**: Wizard generates OOB interface (immediate value)
- **Step 2**: User deploys and tests (builds confidence)
- **Step 3**: User enhances with visual coder (when ready)

---

## 📊 **Database Architecture**

### **Core Tables**
```sql
-- Master OOB templates (immutable)
hl7_fhir_master_templates (
    id, message_type, version, template_config,
    confidence_score, field_count, is_active
)

-- Interface-specific overrides
interface_message_mappings (
    id, interface_id, message_type,
    mapping_strategy, override_config, override_type
)

-- Universal interface configuration
interfaces (
    id, name, source_type, source_config,
    target_type, target_config, transformation_strategy
)

-- Universal schema definitions
data_schemas (
    id, format_type, version, schema_name,
    schema_definition, schema_metadata
)

-- Universal transformation templates
transformation_templates (
    id, source_format, source_schema,
    target_format, target_schema, template_config
)
```

### **Resolution Logic**
```go
func ResolveMappingConfig(interfaceID, messageType, version) {
    // 1. Check interface override
    override := getInterfaceOverride(interfaceID, messageType)

    // 2. Load master template
    master := getMasterTemplate(messageType, version)

    // 3. Apply resolution strategy
    switch override.MappingStrategy {
    case "master": return master
    case "override": return applyOverride(master, override)
    case "custom": return parseCustomConfig(override)
    }
}
```

---

## 🎨 **Visual No-Code Architecture**

### **Block-Based Programming**
```typescript
enum BlockType {
    // Data Blocks
    'field_input', 'field_output', 'constant', 'variable',

    // Logic Blocks
    'if_condition', 'switch_case', 'loop_for', 'loop_foreach',

    // Transform Blocks
    'transform_func', 'value_map', 'data_convert', 'string_ops',

    // Flow Control
    'sequence', 'parallel', 'merge', 'filter', 'aggregate',

    // Error Handling
    'try_catch', 'validate', 'default_value'
}
```

### **Universal Format Adapter Pattern**
```go
type FormatAdapter interface {
    GetFormatType() string
    Parse(data []byte, config *ParseConfig) (*UniversalDataModel, error)
    Generate(model *UniversalDataModel, config *GenerateConfig) ([]byte, error)
    Validate(data []byte, schema *UniversalSchema) (*ValidationResult, error)
}
```

---

## 🎯 **Real Schema-Based Mappings Generated**

### **What We Actually Built**
1. **Analyzed Real Schemas**:
   - `schemas/hl7/v2.5.1/ADT_A01.gz` → Patient admission mappings
   - `schemas/hl7/v2.5.1/ORU_R01.gz` → Lab results mappings
   - `schemas/fhir/R4/resources/Patient.gz` → FHIR target structures

2. **Generated Production Templates**:
   - **ADT^A01**: 34 mappings, 99% confidence for core demographics
   - **ORU^R01**: 42 mappings, comprehensive observation coverage

3. **Key Mappings Examples**:
   ```json
   PID.3.1 → Patient.identifier[0].value (99% confidence)
   PID.8 → Patient.gender (M→male, F→female, U→unknown)
   OBX.11 → Observation.status (F→final, P→preliminary)
   PV1.2 → Encounter.class.code (I→IMP, O→AMB, E→EMER)
   ```

---

## 🔄 **Data Flow Architecture**

### **Message Processing Pipeline**
```
HL7 Message → Parse & Extract → Message Type Detection
    ↓
Load OOB Template (with interface overrides)
    ↓
Apply Mappings (34 real field transformations)
    ↓
Generate FHIR Resources (Patient + Encounter + MessageHeader)
    ↓
Create Bundle → Store/Route → Monitor
```

### **Transformation Service Integration**
```go
func (s *HL7FHIRTransformServiceV3) TransformMessage(interfaceID, hl7Message) {
    // 1. Parse and detect message type
    parsedHL7, messageType := parseHL7Message(hl7Message)

    // 2. Resolve mapping (master + overrides)
    config := s.mappingResolver.ResolveMappingConfig(interfaceID, messageType)

    // 3. Apply transformations
    bundle := s.transformUsingConfig(parsedHL7, config)

    return bundle
}
```

---

## 🎛️ **Visual Logic Builder Features**

### **Advanced Logic Blocks**
- **Conditional Mapping**: If PID.8="M" then Patient.gender="male"
- **Loop Processing**: For each OBX segment, create Observation
- **Switch Cases**: Map encounter class with multiple conditions
- **Error Handling**: Try-catch blocks with default values
- **Data Validation**: Field presence and format validation

### **AI-Powered Suggestions**
```typescript
const SuggestionsEngine = {
    generateSuggestions(sourceSchema, targetSchema) {
        // 1. Semantic similarity matching
        // 2. Data type compatibility
        // 3. OOB template learning
        // 4. ML confidence scoring
        return sortedSuggestions;
    }
}
```

### **Real-Time Execution Preview**
- Live data flow visualization
- Step-by-step execution tracing
- Sample data testing
- Output preview with validation

---

## 🚀 **Future-Proof Plugin Architecture**

### **Extensibility Points**
```go
// New format support
type FormatPlugin interface {
    FormatAdapter
    Initialize(config) error
    GetPluginInfo() PluginInfo
}

// Custom transform functions
type TransformFunction interface {
    Execute(input, params) (output, error)
    GetSignature() FunctionSignature
}
```

### **Supported Formats Roadmap**
- **Current**: HL7 v2.x, FHIR R4
- **Phase 1**: CCD, EDI X12, Excel, CSV
- **Phase 2**: Parquet, Avro, JSON, XML
- **Phase 3**: Database connectors, API endpoints
- **Phase 4**: Custom binary formats via plugins

---

## 📈 **Performance & Scale Design**

### **Million+ Message Capability**
- **Pre-compiled Templates**: Zero runtime parsing overhead
- **Memory-Optimized**: Compressed JSON storage + indexing
- **Parallel Processing**: Batch transformation with worker pools
- **Caching Strategy**: Template resolution + schema caching
- **Database Indexing**: Fast lookup by interface + message type

### **Monitoring & Analytics**
```go
type TransformationMetrics struct {
    InterfaceID, SourceFormat, TargetFormat string
    ProcessingTime time.Duration
    RecordsProcessed, ErrorCount int
    TemplateUsed string
    CustomizationsUsed int
}
```

---

## 🎯 **Implementation Status**

### **✅ Completed**
1. **Architecture Design**: Universal, pluggable, future-proof
2. **Real Schema Analysis**: Generated from actual gzipped files
3. **Master Template System**: ADT^A01 & ORU^R01 production-ready
4. **Database Schema**: V9 hierarchical mapping approach
5. **Visual Logic Specification**: Complete block-based system
6. **User Journey Design**: Wizard → Deploy → Enhance workflow

### **📋 Next Implementation Steps**
1. **Load Templates**: Insert real mappings into database
2. **Update Transform Service**: Use hierarchical resolution
3. **Build Visual Coder**: React-based drag-drop interface (Phase 2)
4. **Extend Message Types**: ORM, MDM, SIU, DFT, RDE
5. **Plugin System**: Format adapter registration
6. **Performance Testing**: Million message validation

---

## 🔄 **Pipeline Builder Architecture (Phase 1 - October 2025)**

### **Purpose**: Enable users to build, deploy, and test transformation pipelines

**Design Principles**:
1. **Progressive Deployment**: Wizard → Pipeline → Deploy → Test → Enhance
2. **Dual-Mode Support**: Express Mode (quick setup) + Step-by-Step Mode (full control)
3. **Optional Layers**: Pre-processing and post-processing are optional for performance
4. **Wizard Integration**: Core transformation reuses existing wizard mappings
5. **AI-Assisted**: Claude chat integration for natural language pipeline building

### **Pipeline Structure**

```
┌─────────────────────────────────────────────────────────────┐
│  PIPELINE = Configurable Transformation Flow               │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ Pre-Process  │  │ Core Transform│ │ Post-Process │     │
│  │ (Optional)   │→ │ (Required)    │→│ (Optional)   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                             │
│  Pre: Validation, Enrichment, Custom Logic                 │
│  Core: Wizard Mapping (HL7→FHIR or any format)            │
│  Post: FHIR Validation, Delivery, Anonymization            │
└─────────────────────────────────────────────────────────────┘
```

### **Express Mode (Quick Deployment)**

For users who want immediate deployment with sensible defaults:

```
Step 1: Source & Target
- From: HL7 v2.x
- To: FHIR R4
- Message Type: ADT^A01

Step 2: Use Wizard Mapping
- ☑ Use existing wizard mapping (recommended)
- Interface: Test Interface1
- Mapping: ADT^A01 → FHIR Patient

Step 3: Quality Options
- ☑ Validate HL7 input (schema-based)
- ☑ Validate FHIR output (schema-based)
- ☐ Enable enrichment (optional)
- ☐ Enable PHI anonymization (optional)

[Deploy Pipeline] → Ready to test immediately
```

### **Step-by-Step Mode (Full Control)**

For advanced users who need custom logic:

- **Drag-drop interface** with three collapsible layers
- **Template library** with pre-built steps
- **Custom steps** via JSON or AI chat
- **Visual connections** showing data flow
- **Real-time validation** of pipeline structure

### **Pipeline Database Schema**

```sql
-- Pipeline definitions
transformation_pipelines (
    id UUID PRIMARY KEY,
    interface_id UUID REFERENCES interfaces(id),
    message_type VARCHAR(50),
    pipeline_name VARCHAR(255),
    mode VARCHAR(20),  -- 'express' | 'step-by-step'
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Pipeline steps (ordered execution)
transformation_steps (
    id UUID PRIMARY KEY,
    pipeline_id UUID REFERENCES transformation_pipelines(id),
    step_name VARCHAR(255),
    step_type VARCHAR(50),  -- 'validation', 'enrichment', 'mapping', 'custom'
    layer VARCHAR(20),      -- 'pre', 'core', 'post'
    sequence INT,           -- Execution order
    is_optional BOOLEAN DEFAULT false,
    config JSONB,           -- Step-specific configuration

    -- Wizard integration
    wizard_mapping_id UUID REFERENCES interface_message_mappings(id),

    created_at TIMESTAMP
);

-- Pipeline execution logs
pipeline_executions (
    id UUID PRIMARY KEY,
    pipeline_id UUID REFERENCES transformation_pipelines(id),
    message_id VARCHAR(255),
    status VARCHAR(50),     -- 'success', 'failed', 'partial'
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration_ms INT,
    steps_executed JSONB,   -- Array of step execution details
    error_details JSONB
);
```

### **AI Integration (Claude Chat)**

**Claude assists with:**

1. **Natural Language Step Creation**
   - User: "Validate patient ID is 10 digits"
   - Claude: Generates validation step config

2. **Schema-Aware Suggestions**
   - Reads HL7/FHIR schemas
   - Suggests required fields for message type
   - Warns about missing mappings

3. **Pipeline Optimization**
   - Identifies redundant steps
   - Suggests performance improvements
   - Recommends error handling

4. **Troubleshooting**
   - Analyzes execution logs
   - Suggests fixes for failures
   - Explains validation errors

### **Schema-Driven Validation Engine**

**Universal validation for any format with a schema:**

```go
type ValidationEngine struct {
    schemaLoader SchemaLoader
}

func (ve *ValidationEngine) Validate(message, format, version string) ValidationResult {
    schema := ve.schemaLoader.Load(format, version)

    return ValidationResult{
        StructureValid: ve.validateStructure(message, schema),
        RequiredFields: ve.validateRequired(message, schema),
        DataTypes:      ve.validateDataTypes(message, schema),
        Formats:        ve.validateFormats(message, schema),
        Bindings:       ve.validateBindings(message, schema),  // Code tables
        Cardinality:    ve.validateCardinality(message, schema)
    }
}
```

**For HL7:**
- Structure: Segment order, field positions
- Required: MSH, PID, PV1 for ADT^A01
- Data Types: TS (timestamp), CX (extended composite), etc.
- Formats: YYYYMMDD for dates, regex for IDs
- Bindings: HL70001 (gender), HL70002 (marital status)
- Cardinality: 0..1, 1..*, etc.

**For FHIR:**
- Structure: Resource type, element hierarchy
- Required: Patient.identifier, Patient.name
- Data Types: dateTime, CodeableConcept, Reference
- Formats: FHIR date formats, URI patterns
- Bindings: ValueSets (administrative-gender, etc.)
- Cardinality: Must support elements

### **Wizard Mapping Integration**

**Core transformation step = Wizard mapping:**

```javascript
{
  "step_type": "mapping",
  "step_name": "HL7 to FHIR Patient",
  "layer": "core",
  "sequence": 100,

  // Uses existing wizard mapping!
  "wizard_mapping_id": "abc123...",
  "interface_id": "interface_uuid",
  "message_type": "ADT^A01",

  // OR create inline mapping
  "inline_mapping": {
    "PID.3": "Patient.identifier[0].value",
    "PID.5.1": "Patient.name[0].family"
    // ... manual mappings
  },

  // OR AI-generated
  "ai_generated": true,
  "ai_confidence": 0.95
}
```

### **Future: Visual Block System (Phase 2)**

Eventually expand to full block-based programming:

- **Logic Blocks**: if/switch/loop
- **Transform Blocks**: value_map, string_ops, calculations
- **Flow Control**: parallel/merge/filter/aggregate
- **Error Handling**: try/catch, default_value

**Implementation timeline**: After Pipeline Builder Phase 1 is deployed and tested

### **Technology Stack**

**Frontend**:
- Vanilla JavaScript (current Phase 1C)
- HTML5 Drag & Drop API
- Future: React + TypeScript for visual blocks (Phase 2)

**Backend**:
- Node.js: Pipeline routes, API layer
- Go: Transformation execution engine
- PostgreSQL: Pipeline storage, execution logs
- MongoDB: Message storage (hybrid approach)

**AI Integration**:
- Claude 3.5 Sonnet API
- Real-time chat interface
- Schema context injection
- Step generation from natural language

---

## 🧠 **Key Architectural Principles**

### **Design Philosophy**
1. **Universal First**: Every component designed for multiple formats
2. **OOB Excellence**: High-confidence templates as foundation
3. **Progressive Enhancement**: Simple to advanced user journey
4. **Visual Simplicity**: No-code for complex logic
5. **Production Scale**: Million+ message architecture
6. **Plugin Extensibility**: Future format support without core changes

### **Technology Choices**
- **Backend**: Go for transformation engine (performance)
- **Frontend**: React with TypeScript (visual components)
- **Database**: PostgreSQL with JSONB (flexible + performant)
- **Caching**: In-memory + Redis for high-volume scenarios
- **Monitoring**: Real-time metrics + audit trails

### **Success Metrics**
- **Coverage**: 80%+ of common fields mapped OOB
- **Performance**: 10,000+ messages/second processing
- **Confidence**: 95%+ mapping accuracy for core data
- **Usability**: Non-technical users can create interfaces
- **Extensibility**: New formats via plugins without core changes

---

## 💡 **Remember: Core Value Propositions**

1. **Immediate Value**: OOB templates provide instant working interfaces
2. **No-Code Power**: Visual logic builder for non-developers
3. **Universal Support**: Any format to any format transformation
4. **Production Ready**: Million+ message processing capability
5. **Future Proof**: Plugin architecture for unlimited extensibility

**This architecture positions ezHealthKonnect as the universal healthcare data integration platform that scales from startup to enterprise while maintaining ease of use.**

---

*Reference created: 2024-09-26*
*Conversation Context: Complete architectural design from schema analysis to visual no-code implementation*