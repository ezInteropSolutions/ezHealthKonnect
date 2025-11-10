# Transformation Flow Selector - Explicit Flow Definition

**Date**: October 27, 2025
**Status**: ✅ Implemented

## Problem Statement

**User Feedback**:
> "I think you misunderstood, we do have transformation pipeline, in UI i do not define the flow, may be in step 1 we give user option to choose the flow, this way on the last step it will enable bundle and individual resource option"

### The Issue

Previously, the system tried to **infer** the transformation flow from source + target selection:
- Source: HL7 v2.x + Target: FHIR → Assumed HL7→FHIR transformation
- This was implicit and didn't give users control

### The Solution

Users now **explicitly select** the transformation flow in Step 1:
- Clear dropdown with transformation flows vs passthrough flows
- Delivery mode (Bundle/Individual) only appears for transformation flows
- System knows exactly what processing to apply

## Implementation

### New UI Element in Step 1

**File**: `public/js/wizard/optimized/WizardView.js` (Lines 372-412)

```html
<!-- Transformation Flow Section -->
<div class="config-section">
    <h4 class="section-title">🔄 Transformation Flow</h4>
    <div class="form-group">
        <label for="transformationFlow" class="form-label required">Select Processing Flow</label>
        <select id="transformationFlow" class="form-control" required>
            <option value="">Choose transformation flow...</option>

            <optgroup label="🔀 Transformation Flows (Auto-processing)">
                <option value="hl7_to_fhir">
                    HL7 v2.x → FHIR R4 (Automatic Transformation)
                </option>
                <option value="ccd_to_fhir">
                    CCD/C-CDA → FHIR R4 (Automatic Transformation)
                </option>
                <option value="hl7_to_fhir_stu3">
                    HL7 v2.x → FHIR STU3 (Automatic Transformation)
                </option>
            </optgroup>

            <optgroup label="📦 Passthrough Flows (No transformation)">
                <option value="passthrough">
                    Passthrough (Store only, no transformation)
                </option>
                <option value="fhir_receiver">
                    FHIR Receiver (Direct storage, user-driven)
                </option>
                <option value="file_processor">
                    File Processor (Batch processing, user-driven)
                </option>
            </optgroup>
        </select>
        <div class="form-hint">
            ℹ️ Transformation flows automatically process messages.
            Passthrough flows store for manual processing.
        </div>
    </div>

    <!-- Flow Description (Dynamic) -->
    <div id="flowDescription" class="alert alert-info" style="display: none;">
        <strong id="flowDescTitle"></strong>
        <p id="flowDescText"></p>
    </div>
</div>
```

### Flow Descriptions (Dynamic)

When user selects a flow, a description appears:

**HL7 v2.x → FHIR R4**:
> 🔀 HL7 v2.x → FHIR R4 Transformation
>
> Automatically converts incoming HL7 v2.x messages to FHIR R4 resources and delivers them to your FHIR server. Supports ADT, ORU, ORM, and other message types.

**CCD/C-CDA → FHIR R4**:
> 🔀 CCD/C-CDA → FHIR R4 Transformation
>
> Automatically converts C-CDA (Consolidated Clinical Document Architecture) documents to FHIR R4 resources. Ideal for EHR interoperability.

**Passthrough**:
> 📦 Passthrough (No Transformation)
>
> Stores messages without transformation. Useful for archiving, auditing, or manual processing later.

**FHIR Receiver**:
> 📦 FHIR Receiver (Direct Storage)
>
> Receives FHIR resources via HTTP and stores them without modification. No automatic forwarding - user-driven processing.

### Updated Delivery Mode Logic

**Before** (Lines 569-587 - OLD):
```javascript
shouldShowDeliveryMode(sourceType, targetType) {
    const transformationFlows = [
        { source: 'hl7v2', target: 'fhir', name: 'HL7→FHIR' },
        { source: 'ccda', target: 'fhir', name: 'CCD→FHIR' },
    ];
    // Inferred from source + target
    return transformationFlows.some(flow =>
        flow.source === sourceType && flow.target === targetType
    );
}
```

**After** (Lines 615-625 - NEW):
```javascript
shouldShowDeliveryMode(transformationFlow) {
    // Explicit check of selected flow
    const transformationFlowsWithDelivery = [
        'hl7_to_fhir',         // HL7 v2.x → FHIR R4
        'ccd_to_fhir',         // CCD/C-CDA → FHIR R4
        'hl7_to_fhir_stu3',    // HL7 v2.x → FHIR STU3
        // Easy to add: 'x12_to_fhir', 'csv_to_fhir', etc.
    ];

    return transformationFlowsWithDelivery.includes(transformationFlow);
}
```

### Event Listener (Lines 3906-3961)

```javascript
// Transformation Flow selector
const transformationFlowSelect = container.querySelector('#transformationFlow');
const flowDescriptionDiv = container.querySelector('#flowDescription');

if (transformationFlowSelect) {
    transformationFlowSelect.addEventListener('change', (e) => {
        const flow = e.target.value;
        console.log('🔄 Transformation flow selected:', flow);

        // Show flow description
        const flowDescriptions = {
            'hl7_to_fhir': {
                title: '🔀 HL7 v2.x → FHIR R4 Transformation',
                text: 'Automatically converts incoming HL7 v2.x messages...'
            },
            // ... other flows
        };

        const desc = flowDescriptions[flow];
        if (desc) {
            flowDescTitle.textContent = desc.title;
            flowDescText.textContent = desc.text;
            flowDescriptionDiv.style.display = 'block';
        }

        // Update wizard model
        this.dispatchEvent(new CustomEvent('fieldChange', {
            detail: { field: 'transformationFlow', value: flow }
        }));
    });
}
```

## User Experience Flow

### Step-by-Step: HL7 → FHIR Transformation

1. **Open Wizard** → New Interface

2. **Step 1 - Basic Configuration**:
   - **Name**: "ADT Interface - Hospital XYZ"
   - **Description**: Auto-generated

3. **Step 1 - Source Configuration**:
   - **Source Format**: HL7 v2.x (Standard)
   - **Connectivity**: TCP/MLLP (Standard)
   - **Port**: 6662

4. **Step 1 - Transformation Flow** ⬅️ **NEW SECTION**:
   - **Select Processing Flow**:
     ```
     [HL7 v2.x → FHIR R4 (Automatic Transformation) ▼]
     ```
   - **Description appears**:
     > 🔀 HL7 v2.x → FHIR R4 Transformation
     >
     > Automatically converts incoming HL7 v2.x messages to FHIR R4 resources...

5. **Step 1 - Target Configuration**:
   - **Target Type**: FHIR R4
   - **Target Connectivity**: HTTP
   - **FHIR Base URL**: `http://localhost:8080/fhir`

6. **Delivery Mode Appears** ⬅️ **Because transformation flow is selected**:
   ```
   Delivery Mode 🏷️ Transformation Flow
   ⦿ Bundle (Single transaction) - Recommended
   ○ Individual (Separate API calls per resource)
   ```

7. **If Individual selected** → Resource selection panel appears:
   ```
   Select Resources to Send
   ☑ Patient    ☑ Encounter    ☑ Observation
   ☑ Condition  ☑ Procedure    ☑ MedicationRequest
   ```

8. **Authentication** (optional):
   - Enable Authentication
   - Select: Basic Authentication
   - Username: `fhir-user`
   - Password: `•••••••`

9. **Continue to Step 2-4** → Complete wizard

### Step-by-Step: FHIR Receiver (Passthrough)

1. **Open Wizard** → New Interface

2. **Step 1 - Basic Configuration**:
   - **Name**: "FHIR Receiver Endpoint"

3. **Step 1 - Source Configuration**:
   - **Source Format**: FHIR
   - **Connectivity**: HTTP/REST

4. **Step 1 - Transformation Flow** ⬅️ **NEW SECTION**:
   - **Select Processing Flow**:
     ```
     [FHIR Receiver (Direct storage, user-driven) ▼]
     ```
   - **Description appears**:
     > 📦 FHIR Receiver (Direct Storage)
     >
     > Receives FHIR resources via HTTP and stores them without modification...

5. **Step 1 - Target Configuration**:
   - **Target Type**: Database
   - **Target Connectivity**: PostgreSQL

6. **Delivery Mode Does NOT Appear** ⬅️ **Because it's a passthrough flow**:
   - No Bundle/Individual options
   - No resource selection
   - Just stores for user-driven processing

## Configuration Storage

### Wizard Model

```javascript
{
  name: "ADT Interface - Hospital XYZ",
  sourceType: "hl7v2",
  sourceConnectivity: "tcp",
  sourcePort: 6662,

  transformationFlow: "hl7_to_fhir", // NEW - Explicitly selected

  targetType: "fhir",
  targetConnectivity: "http",
  targetEndpoint: "http://localhost:8080/fhir",

  // Delivery mode (only if transformationFlow is transformation type)
  deliveryMode: "bundle",
  individualResources: ["Patient", "Encounter", "Observation"],

  // Authentication
  targetAuthEnabled: true,
  authType: "basic",
  authUsername: "fhir-user",
  authPassword: "secure-password"
}
```

### Database Storage (interfaces table)

```sql
UPDATE interfaces
SET
  source_connectivity = '{
    "type": "tcp",
    "port": 6662,
    "host": "0.0.0.0"
  }'::jsonb,

  transformation_flow = 'hl7_to_fhir', -- NEW column

  target_connectivity = '{
    "type": "http",
    "endpoint": "http://localhost:8080/fhir",
    "version": "R4",
    "format": "json",
    "deliveryMode": "bundle",
    "authEnabled": true,
    "authType": "basic",
    "authUsername": "fhir-user",
    "authPassword": "encrypted_hash"
  }'::jsonb
WHERE id = 'interface-uuid';
```

## Flow Registry (Easy to Extend)

### Adding a New Transformation Flow

To add X12 → FHIR transformation:

**1. Add to UI dropdown** (Line 380):
```html
<option value="x12_to_fhir">
    X12 EDI → FHIR R4 (Automatic Transformation)
</option>
```

**2. Add flow description** (Line 3920):
```javascript
'x12_to_fhir': {
    title: '🔀 X12 EDI → FHIR R4 Transformation',
    text: 'Automatically converts X12 EDI transactions (837, 835, 270/271) to FHIR R4 resources.'
}
```

**3. Add to delivery mode registry** (Line 618):
```javascript
const transformationFlowsWithDelivery = [
    'hl7_to_fhir',
    'ccd_to_fhir',
    'hl7_to_fhir_stu3',
    'x12_to_fhir' // NEW
];
```

**4. Implement backend processor** (processing/transformation_engine.go):
```go
case "x12_to_fhir":
    return executeX12ToFHIRTransformation(ctx, msg)
```

That's it! The UI will automatically show delivery mode for X12 → FHIR flows.

## Console Logging

### Flow Selection
```
✅ Transformation flow selector listener attached

// When user selects flow:
🔄 Transformation flow selected: hl7_to_fhir
```

### Delivery Mode Visibility
```
// If transformation flow:
✅ Delivery mode shown for transformation flow: hl7_to_fhir

// If passthrough flow:
⏭️ Delivery mode hidden for passthrough flow: fhir_receiver
```

## Backend Integration (Next Steps)

### 1. Add transformation_flow Column

```sql
-- Migration: V31__Add_Transformation_Flow.sql
ALTER TABLE interfaces
ADD COLUMN transformation_flow VARCHAR(50);

CREATE INDEX idx_interfaces_transformation_flow
ON interfaces(transformation_flow);
```

### 2. Read Flow in Processing Engine

```go
// processing/engine.go
func (pe *ProcessingEngine) processMessage(msg *models.InboundMessage) {
    // Load interface configuration
    iface := pe.loadInterface(msg.InterfaceID)

    // Check transformation flow
    switch iface.TransformationFlow {
    case "hl7_to_fhir":
        pe.executeHL7ToFHIRTransformation(msg, iface)
    case "ccd_to_fhir":
        pe.executeCCDToFHIRTransformation(msg, iface)
    case "passthrough":
        // Just store, no transformation
        log.Printf("Passthrough flow - no transformation")
    case "fhir_receiver":
        // Store FHIR resource directly
        log.Printf("FHIR receiver - direct storage")
    default:
        log.Printf("Unknown flow: %s", iface.TransformationFlow)
    }
}
```

### 3. Apply Delivery Mode

```go
func (pe *ProcessingEngine) executeHL7ToFHIRTransformation(msg, iface) {
    // Transform HL7 → FHIR
    fhirResources := pe.transformToFHIR(msg)

    // Check delivery mode from target_connectivity
    targetConfig := iface.TargetConnectivity
    deliveryMode := targetConfig["deliveryMode"].(string)

    if deliveryMode == "bundle" {
        pe.sendFHIRBundle(fhirResources, targetConfig)
    } else if deliveryMode == "individual" {
        selectedResources := targetConfig["individualResources"].([]string)
        pe.sendFHIRIndividual(fhirResources, selectedResources, targetConfig)
    }
}
```

## Benefits

### 1. **Explicit Control**
- Users explicitly choose transformation behavior
- No ambiguity or inference
- Clear understanding of what will happen

### 2. **Flexible Configuration**
- Easy to add new flows
- Each flow can have different options
- Passthrough vs transformation clearly separated

### 3. **Conditional UI**
- Delivery mode only shown when relevant
- Resource selection only for individual mode
- Authentication configurable per interface

### 4. **Easy Extension**
- Adding X12 → FHIR: 4 lines of code
- Adding CSV → FHIR: 4 lines of code
- No complex logic changes needed

## Testing

### Test 1: Transformation Flow Selection

1. Open wizard
2. Fill Step 1 fields
3. Select "HL7 v2.x → FHIR R4" from Transformation Flow dropdown
4. ✅ **VERIFY**: Flow description appears
5. ✅ **VERIFY**: Console logs: `🔄 Transformation flow selected: hl7_to_fhir`
6. Scroll to FHIR configuration
7. ✅ **VERIFY**: Delivery Mode section appears with "🏷️ Transformation Flow" badge

### Test 2: Passthrough Flow Selection

1. Open wizard
2. Fill Step 1 fields
3. Select "Passthrough (Store only)" from Transformation Flow dropdown
4. ✅ **VERIFY**: Flow description appears (Passthrough description)
5. ✅ **VERIFY**: Console logs: `🔄 Transformation flow selected: passthrough`
6. Scroll to target configuration
7. ✅ **VERIFY**: Delivery Mode section does NOT appear

### Test 3: Flow Switching

1. Select "HL7 v2.x → FHIR R4" → Delivery mode appears
2. Switch to "FHIR Receiver" → Delivery mode disappears
3. Switch back to "CCD/C-CDA → FHIR R4" → Delivery mode appears again
4. ✅ **VERIFY**: UI updates dynamically without page reload

## Status

✅ **Complete**:
- Transformation flow selector UI
- Flow descriptions (dynamic)
- Delivery mode conditional visibility
- Event listeners and model updates
- Easy extension pattern

⏳ **Next Sprint**:
- Database migration (add transformation_flow column)
- Backend flow processor implementation
- Flow-to-processor routing

## Documentation

- [FHIR_DELIVERY_MODE_AND_AUTH_COMPLETE.md](FHIR_DELIVERY_MODE_AND_AUTH_COMPLETE.md) - Delivery mode and authentication
- [WIZARD_MODAL_FHIR_CONFIG_FIX.md](WIZARD_MODAL_FHIR_CONFIG_FIX.md) - Initial wizard fixes
- [TRANSFORMATION_PIPELINE_DESIGN.md](TRANSFORMATION_PIPELINE_DESIGN.md) - Transformation pipeline architecture
