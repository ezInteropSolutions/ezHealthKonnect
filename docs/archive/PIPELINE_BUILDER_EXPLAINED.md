# Pipeline Builder - Concept & Architecture Explained

## 🎯 The Big Picture

### What Problem Does This Solve?

**Before Pipeline Builder:**
- Transformation logic was hardcoded in `transformation_mapping` JSON field
- Required manual JSON editing for every change
- No visual way to see the transformation flow
- Difficult to add validation, enrichment, or custom logic
- One monolithic transformation step per interface

**After Pipeline Builder:**
- Visual drag-and-drop interface to build transformation workflows
- Multiple steps in sequence or parallel
- Pre-processing (validation, enrichment) before transformation
- Post-processing (validation, delivery) after transformation
- Reusable templates and custom scripts

---

## 📐 Three-Pane Layout Explained

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          PIPELINE BUILDER                                 │
├──────────────┬─────────────────────────────────────┬─────────────────────┤
│              │                                     │                     │
│   LEFT       │            CENTER                   │       RIGHT         │
│  TOOLBOX     │            CANVAS                   │    PROPERTIES       │
│              │                                     │                     │
│  (Library)   │         (Workspace)                 │  (Configuration)    │
│              │                                     │                     │
└──────────────┴─────────────────────────────────────┴─────────────────────┘
```

### LEFT PANE: Toolbox (Component Library)

**Purpose**: Your "parts drawer" - all available components you can use

**What You See:**
- **Templates Section**: Pre-built, ready-to-use transformation steps
- **Pre-Processing Section**: Validation and enrichment steps
- **Transformation Section**: HL7→FHIR mapping steps
- **Post-Processing Section**: Validation and delivery steps
- **Custom Scripts Section**: Create your own JavaScript/Lua scripts

**How It Works:**
1. Browse available components
2. **Drag** a component card
3. **Drop** it in the center canvas
4. Component becomes a step in your pipeline

**Think of it as**: Lego bricks drawer - you pick which pieces you want

---

### CENTER PANE: Canvas (Your Workspace)

**Purpose**: Visual representation of your transformation pipeline

**What You See:**
Three horizontal layers stacked vertically:

```
┌─────────────────────────────────────────────────────┐
│  🔵 PRE-PROCESSING LAYER                           │
│  ┌───────────────┐  ┌───────────────┐             │
│  │ Validate      │  │ Enrich Data   │             │
│  └───────────────┘  └───────────────┘             │
└─────────────────────────────────────────────────────┘
                        ↓ (Data flows down)
┌─────────────────────────────────────────────────────┐
│  🟡 CORE TRANSFORMATION LAYER                      │
│  ┌─────────────────────────────────────┐           │
│  │  HL7 → FHIR Mapping                │           │
│  └─────────────────────────────────────┘           │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│  🟢 POST-PROCESSING LAYER                          │
│  ┌───────────────┐  ┌───────────────┐             │
│  │ Validate FHIR │  │ Deliver       │             │
│  └───────────────┘  └───────────────┘             │
└─────────────────────────────────────────────────────┘
```

**How It Works:**
1. Drop components from left pane into these layers
2. Arrange steps in the order you want them to execute
3. Connect steps with visual arrows
4. Data flows from top (Pre) → middle (Core) → bottom (Post)

**Think of it as**: Assembly line - raw material enters at top, finished product exits at bottom

---

### RIGHT PANE: Properties (Configuration Editor)

**Purpose**: Configure the selected step's behavior

**When You See It:**
- Empty by default (shows "Select a step to configure")
- Populates when you **click** on any step in the canvas

**What You Can Configure:**
- **Step Name**: "Validate Patient ID"
- **Description**: What this step does
- **Timeout**: How long before step fails (milliseconds)
- **Error Strategy**: What happens if step fails (Fail/Skip/Default)
- **Configuration (JSON)**: Step-specific settings
- **Custom Script**: If it's a custom script step, write your code here

**How It Works:**
1. Click any step in center canvas
2. Right pane shows that step's configuration
3. Edit settings
4. Click "Save" button
5. Step updates in canvas

**Think of it as**: Settings panel - like adjusting settings on your phone

---

## 🔄 How It Links to Current Transformation Mapping

### Current System (V9 - Before Pipeline Builder)

**Database Tables:**
- `hl7_fhir_templates`: Standard ADT^A01, ORU^R01 mappings
- `interface_message_mappings`: Interface-specific mappings

**How It Worked:**
```
Message Arrives
    ↓
Load transformation_mapping from interface_message_mappings
    ↓
Apply HL7→FHIR mapping in one step
    ↓
Done
```

**Limitations:**
- Only one transformation step
- No pre-processing validation
- No post-processing delivery
- No custom logic
- Hard to visualize

---

### New System (V21 - With Pipeline Builder)

**New Database Tables:**
- `transformation_pipelines`: Pipeline metadata (one per interface+message_type)
- `execution_groups`: Organize steps into parallel/sequential groups
- `transformation_steps`: Individual steps in pipeline
- `step_dependencies`: Define execution order
- `pipeline_visual_configs`: Store canvas layout

**How It Works:**
```
Message Arrives
    ↓
Load Pipeline (from transformation_pipelines)
    ↓
Execute PRE-PROCESSING Layer
    ├─ Step 1: Validate Required Fields
    └─ Step 2: Enrich Patient Data (parallel)
    ↓
Execute CORE TRANSFORMATION Layer
    └─ Step 3: HL7 → FHIR Mapping (uses your existing transformation_mapping!)
    ↓
Execute POST-PROCESSING Layer
    ├─ Step 4: Validate FHIR Bundle
    └─ Step 5: Deliver to FHIR Server
    ↓
Done
```

---

## 🔗 Linking Pipeline Builder to Existing Transformation Mapping

### Strategy: Wrap Existing Mapping in a Pipeline Step

**Step 1: Your Current Transformation Mapping is Preserved**

Your existing `transformation_mapping` JSON in `interface_message_mappings` table:
```json
{
  "MSH": { "target": "MessageHeader", "mappings": [...] },
  "PID": { "target": "Patient", "mappings": [...] },
  "PV1": { "target": "Encounter", "mappings": [...] }
}
```

**This becomes a TEMPLATE** that can be used in the pipeline builder.

---

**Step 2: Pipeline Builder Creates a Pipeline Around It**

When you use Pipeline Builder:
```
┌─────────────────────────────────────────────────────┐
│  PRE-PROCESSING                                     │
│  ┌───────────────────────────────────┐             │
│  │ [NEW] Validate Required Fields    │             │
│  └───────────────────────────────────┘             │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│  CORE TRANSFORMATION                                │
│  ┌─────────────────────────────────────┐           │
│  │ [EXISTING] Your HL7→FHIR Mapping    │ ← USES YOUR EXISTING MAPPING!
│  │ (from interface_message_mappings)   │           │
│  └─────────────────────────────────────┘           │
└─────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────┐
│  POST-PROCESSING                                    │
│  ┌───────────────────────────────────┐             │
│  │ [NEW] Validate FHIR Bundle        │             │
│  └───────────────────────────────────┘             │
└─────────────────────────────────────────────────────┘
```

---

**Step 3: The "HL7 to FHIR Mapping" Template**

This template (in left toolbox) is special - it reads from your existing mapping:

```javascript
// When you drag "HL7 to FHIR Mapping" template
{
  id: 'hl7-fhir-mapping',
  name: 'HL7 to FHIR Mapping',
  type: 'core.mapping',
  config: {
    fhir_version: 'R4',
    use_template: true,  // ← KEY: Uses existing transformation_mapping
    interface_id: 'INT_001',
    message_type: 'ADT^A01'
  }
}
```

**Backend Executor** (in Go):
```go
// services/hl7_fhir_mapping_executor.go (already exists!)
func (executor *HL7FHIRMappingExecutor) Execute(step, inputData) {
    // Load transformation_mapping from interface_message_mappings
    mapping := loadExistingMapping(step.Config.InterfaceID, step.Config.MessageType)

    // Apply the mapping (your existing logic!)
    fhirBundle := applyMapping(inputData, mapping)

    return fhirBundle
}
```

---

## 🎬 Real-World Example: ADT^A01 Patient Admission

### Scenario
Hospital sends ADT^A01 message when patient is admitted. You want to:
1. Validate message has required fields
2. Enrich patient data from Epic API
3. Transform HL7 → FHIR Patient resource
4. Validate FHIR bundle is valid
5. Send to FHIR server

### Without Pipeline Builder (Current)
You'd have to:
- Write custom validation code
- Manually call Epic API in transformation logic
- Hope FHIR output is valid
- Manually send to FHIR server

### With Pipeline Builder (New)

**Step 1: Open Pipeline Builder**
- From interfaces page, click 🔀 on your ADT^A01 interface

**Step 2: Drag Components from Left Toolbox**

Drag these components into canvas:

**Pre-Processing Layer:**
1. Drag "Validate Required Fields" → Drop in blue Pre layer
2. Drag "Enrich Patient Data" → Drop in blue Pre layer

**Core Transformation Layer:**
3. Drag "HL7 to FHIR Mapping" → Drop in yellow Core layer

**Post-Processing Layer:**
4. Drag "Validate FHIR Bundle" → Drop in green Post layer
5. Drag "Deliver to FHIR Server" → Drop in green Post layer

**Step 3: Configure Each Step**

Click "Validate Required Fields" step:
```json
{
  "rules": [
    { "field": "MSH.9", "required": true },
    { "field": "PID.3", "required": true },
    { "field": "PID.5", "required": true }
  ]
}
```

Click "Enrich Patient Data" step:
```json
{
  "source": "epic",
  "endpoint": "https://epic-api/patient",
  "fields": ["demographics", "insurance", "allergies"]
}
```

Click "HL7 to FHIR Mapping" step:
```json
{
  "fhir_version": "R4",
  "use_template": true,  // Uses your existing mapping!
  "interface_id": "INT_001",
  "message_type": "ADT^A01"
}
```

Click "Deliver to FHIR Server" step:
```json
{
  "endpoint": "http://fhir-server:8080/fhir",
  "resource": "Patient",
  "method": "POST"
}
```

**Step 4: Save Pipeline**
- Click "Save Pipeline" button
- Pipeline saved to database

**Step 5: Test**
- Click "Test" button
- Paste sample HL7 message
- See results of each step

---

## 📊 Data Flow Visualization

### What Happens When Message Arrives

```
┌─────────────────────────────────────────────────────────────┐
│ 1. TCP LISTENER receives HL7 message                        │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. STORE raw message in MongoDB (raw_messages_intf_xxx)    │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. JSON CONVERSION (existing parser)                        │
│    Output: Enhanced segments with full schema               │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. LOAD PIPELINE (NEW!)                                     │
│    - Query: transformation_pipelines by interface+msg_type  │
│    - Get: execution_groups, steps, dependencies             │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 5. EXECUTE PRE-PROCESSING LAYER                             │
│    Step 1: Validate Required Fields                         │
│       Input: { enhancedSegments: {...} }                    │
│       Output: { enhancedSegments: {...}, validated: true }  │
│    Step 2: Enrich Patient Data                              │
│       Input: { enhancedSegments: {...}, validated: true }   │
│       Calls: Epic API                                        │
│       Output: { enhancedSegments: {...}, enriched: {...} }  │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 6. EXECUTE CORE TRANSFORMATION LAYER                        │
│    Step 3: HL7 to FHIR Mapping                              │
│       Input: { enhancedSegments: {...}, enriched: {...} }   │
│       Loads: transformation_mapping from DB                  │
│       Applies: Your existing mapping logic!                  │
│       Output: { fhirBundle: {...} }                         │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 7. EXECUTE POST-PROCESSING LAYER                            │
│    Step 4: Validate FHIR Bundle                             │
│       Input: { fhirBundle: {...} }                          │
│       Validates: Against FHIR R4 spec                        │
│       Output: { fhirBundle: {...}, fhirValid: true }        │
│    Step 5: Deliver to FHIR Server                           │
│       Input: { fhirBundle: {...}, fhirValid: true }         │
│       Posts: To FHIR endpoint                                │
│       Output: { delivered: true, response: {...} }          │
└─────────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────────┐
│ 8. STORE RESULTS                                            │
│    - Update message status                                   │
│    - Store execution logs                                    │
│    - Store FHIR output in MongoDB                           │
└─────────────────────────────────────────────────────────────┘
```

---

## 🔧 How to Migrate Existing Interface

### Option 1: Automatic Migration (Recommended)

When you click 🔀 on an existing interface:

1. **Pipeline Builder detects**: "No pipeline exists yet"
2. **Loads existing mapping**: From `interface_message_mappings`
3. **Creates default pipeline**:
   ```
   PRE: (empty)
   CORE: HL7→FHIR Mapping (using existing transformation_mapping)
   POST: (empty)
   ```
4. **You can now add**: Pre/Post processing steps

### Option 2: Manual Setup

1. Open Pipeline Builder for interface
2. Drag "HL7 to FHIR Mapping" template to Core layer
3. Configure it:
   ```json
   {
     "use_template": true,
     "interface_id": "YOUR_INTERFACE_ID",
     "message_type": "ADT^A01"
   }
   ```
4. Add any pre/post processing you want
5. Save

---

## ❓ FAQ

### Q: What if I don't want to use Pipeline Builder?
**A:** Your existing transformation still works! Pipeline is optional enhancement.

### Q: Can I still edit transformation_mapping JSON directly?
**A:** Yes! Pipeline Builder is an alternative, not a replacement.

### Q: What happens to my existing mappings?
**A:** They're preserved and can be used as templates in Pipeline Builder.

### Q: Is this production-ready?
**A:** Phase 1C (UI) is complete. Needs integration testing with backend.

### Q: Can I export/import pipelines?
**A:** Not yet - planned for Phase 2.

### Q: Can multiple users edit same pipeline?
**A:** Not yet - single-user editing only. Collaboration in Phase 3.

---

## 🎓 Quick Glossary

- **Pipeline**: Complete transformation workflow (Pre + Core + Post)
- **Layer**: Horizontal section in canvas (Pre/Core/Post)
- **Step**: Individual transformation/validation/enrichment action
- **Template**: Pre-built step configuration you can reuse
- **Execution Group**: Container for parallel or sequential steps
- **Properties**: Configuration settings for a step

---

**Next Steps**: Refresh the page and you should now see template cards in the left panel! Try dragging one to the canvas.
