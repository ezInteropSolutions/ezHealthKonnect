# Wizard Mapping Storage - Canonical Flow

**Date:** December 28, 2025
**Status:** 🔴 BROKEN - Needs Fix
**Problem:** Wizard mappings being lost after code changes

---

## The Problem

You ran the wizard and saw HL7→FHIR mappings in the pipeline. After code changes, the mappings disappeared. This indicates **unstable storage flow** with multiple competing approaches.

---

## Root Cause Analysis

### Multiple Storage Mechanisms (CONFLICT):

1. **Legacy Approach (Pre-V9):**
   - Store in `interfaces.transformation_mapping` JSONB column
   - Used by: Old wizard code
   - Status: ❌ Deprecated but still in code

2. **V9 Message-Type-Centric (September 2024):**
   - Store in `interface_message_mappings` table
   - Used by: MessageTypeMappingService
   - Status: ⚠️ Implemented but not working

3. **Transformation Pipeline Approach (October 2024):**
   - Store pipeline steps in `transformation_pipelines` + `transformation_steps`
   - Reference template in `hl7_fhir_templates`
   - Used by: Pipeline execution engine
   - Status: ✅ Working (fallback to OOB template)

### The Conflict:

```javascript
// wizardController.js - Line 348
transformationMapping: interfaceData.transformationMapping  // ← Goes to interfaces table

// wizardController.js - Line 388
await this.mappingService.saveWizardConfiguration(...)  // ← Should go to interface_message_mappings

// BUT: Line 402-409 swallows errors!
} catch (mappingError) {
    console.error('⚠️ Failed to save message-type mappings:', mappingError.message);
    // Don't fail the entire operation
}
```

**Result:** Wizard creates interface with empty mapping, pipeline falls back to OOB template.

---

## Canonical Flow (FINAL DECISION)

### ✅ ONE TRUE PATH - Transformation Pipeline Architecture

**Rationale:**
1. Already implemented and working
2. Supports multiple message types per interface
3. Supports template reuse across interfaces
4. Executor already uses this flow
5. No backward compatibility needed

### Storage Locations:

```
┌─────────────────────────────────────────────────────────────┐
│ WIZARD COMPLETION                                           │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 1. INTERFACE RECORD (interfaces table)                     │
│    - Basic metadata (name, source, target, status)         │
│    - transformation_mapping = NULL (deprecated)             │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. TRANSFORMATION PIPELINE (transformation_pipelines)       │
│    - interface_id + message_type                            │
│    - pipeline_name                                          │
│    - Unique per interface+message_type                      │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. PIPELINE STEPS (transformation_steps)                   │
│    - Sequence: 10, 20, 30, 100, 200                        │
│    - Step 100: HL7→FHIR Transform (core.mapping)           │
│    - Config: { template_id: <uuid>, custom_overrides: {} } │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. TEMPLATE STORAGE (Choose ONE)                           │
│                                                             │
│ Option A: Use Standard Template                            │
│   - template_id → hl7_fhir_templates.id                    │
│   - custom_overrides = {}                                  │
│   - 99% storage reduction                                  │
│                                                             │
│ Option B: Custom Mapping                                   │
│   - template_id = NULL                                     │
│   - custom_overrides = { atomicMappings: [...] }           │
│   - Full custom mapping stored in step config              │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Phase 1: Clean Up Wizard Controller ✅

**File:** `controllers/wizardController.js`

**Changes:**

1. **Remove Legacy Storage (Line 348):**
```javascript
// OLD:
transformationMapping: interfaceData.transformationMapping

// NEW:
transformationMapping: null  // Deprecated - using pipeline architecture
```

2. **Remove Deprecated saveWizardConfiguration Call (Lines 384-409):**
```javascript
// DELETE THIS ENTIRE BLOCK:
if (interfaceData.transformationMapping) {
    try {
        const mappingResult = await this.mappingService.saveWizardConfiguration(...);
    } catch (mappingError) {
        console.error('⚠️ Failed to save message-type mappings:', mappingError.message);
    }
}
```

3. **Add Pipeline Creation:**
```javascript
// NEW: Create transformation pipeline with HL7→FHIR step
async createInterfaceWithMappings(interfaceData, userId, userEmail) {
    // Step 1: Create interface
    const result = await interfaceService.createInterface(serviceData, userId, userEmail);

    // Step 2: Create transformation pipeline
    await this.createTransformationPipeline(interfaceId, interfaceData, userId);

    return { interfaceId, name };
}

async createTransformationPipeline(interfaceId, interfaceData, userId) {
    const { messageType, transformationMapping } = interfaceData;

    // 1. Create pipeline record
    const pipelineId = await this.createPipeline(interfaceId, messageType, interfaceData.name);

    // 2. Add HL7→FHIR mapping step
    const templateId = await this.getOrCreateTemplate(messageType, transformationMapping);
    await this.addMappingStep(pipelineId, templateId, transformationMapping);

    // 3. Add other default steps (validation, enrichment, etc.)
    await this.addDefaultPipelineSteps(pipelineId);
}
```

### Phase 2: Create Pipeline Service ✅

**New File:** `services/TransformationPipelineService.js`

```javascript
class TransformationPipelineService {
    constructor(pgPool) {
        this.pool = pgPool;
    }

    /**
     * Create pipeline for interface + message type
     */
    async createPipeline(interfaceId, messageType, pipelineName, userId) {
        const query = `
            INSERT INTO transformation_pipelines (
                interface_id,
                message_type,
                pipeline_name,
                description,
                is_active,
                created_by
            ) VALUES ($1, $2, $3, $4, true, $5)
            RETURNING id
        `;

        const result = await this.pool.query(query, [
            interfaceId,
            messageType,
            pipelineName,
            `Auto-generated pipeline for ${messageType}`,
            userId
        ]);

        return result.rows[0].id;
    }

    /**
     * Add HL7→FHIR mapping step to pipeline
     */
    async addMappingStep(pipelineId, templateId, customMappings, userId) {
        // Determine if using standard template or custom mapping
        const useTemplate = customMappings ? this.shouldUseTemplate(customMappings) : true;

        const stepConfig = useTemplate
            ? {
                fhir_version: 'R4',
                use_template: true,
                template_id: templateId
              }
            : {
                fhir_version: 'R4',
                use_template: false,
                custom_mapping: customMappings
              };

        const query = `
            INSERT INTO transformation_steps (
                pipeline_id,
                step_name,
                step_type,
                sequence,
                config,
                enabled,
                created_by
            ) VALUES ($1, $2, $3, $4, $5, true, $6)
            RETURNING id
        `;

        await this.pool.query(query, [
            pipelineId,
            'HL7→FHIR Transform',
            'core.mapping',
            100,  // Core mapping sequence
            JSON.stringify(stepConfig),
            userId
        ]);
    }

    /**
     * Get or create standard template for message type
     */
    async getOrCreateTemplate(messageType, mappings) {
        // Check if standard template exists
        const existingTemplate = await this.getStandardTemplate(messageType);
        if (existingTemplate) {
            return existingTemplate.id;
        }

        // Create new template from wizard mappings
        return await this.createTemplate(messageType, mappings);
    }

    async getStandardTemplate(messageType) {
        const query = `
            SELECT id FROM hl7_fhir_templates
            WHERE message_type = $1 AND is_default = true
            ORDER BY created_at DESC
            LIMIT 1
        `;
        const result = await this.pool.query(query, [messageType]);
        return result.rows[0];
    }

    async createTemplate(messageType, mappings) {
        const query = `
            INSERT INTO hl7_fhir_templates (
                message_type,
                template_name,
                template_description,
                template_config,
                is_default
            ) VALUES ($1, $2, $3, $4, false)
            RETURNING id
        `;

        const templateConfig = {
            version: '2.0',
            messageType: messageType,
            resources: this.convertMappingsToResourceFormat(mappings),
            atomicMappings: mappings.atomicMappings || []
        };

        const result = await this.pool.query(query, [
            messageType,
            `Custom ${messageType} Mapping`,
            `Custom mapping created via wizard`,
            JSON.stringify(templateConfig)
        ]);

        return result.rows[0].id;
    }

    /**
     * Add default pipeline steps (validation, enrichment)
     */
    async addDefaultPipelineSteps(pipelineId, userId) {
        const defaultSteps = [
            { sequence: 10, name: 'Field Validation', type: 'pre.validation', config: {} },
            { sequence: 20, name: 'API Enrichment', type: 'pre.enrichment.api', config: {} },
            { sequence: 30, name: 'Database Enrichment', type: 'pre.enrichment.database', config: {} },
            { sequence: 50, name: 'Field Mapping', type: 'core.transformation', config: {} },
            { sequence: 200, name: 'FHIR Validation', type: 'post.fhir.validation', config: {} }
        ];

        for (const step of defaultSteps) {
            await this.pool.query(`
                INSERT INTO transformation_steps (
                    pipeline_id, step_name, step_type, sequence, config, enabled, created_by
                ) VALUES ($1, $2, $3, $4, $5, true, $6)
            `, [pipelineId, step.name, step.type, step.sequence, JSON.stringify(step.config), userId]);
        }
    }

    shouldUseTemplate(customMappings) {
        // Compare with standard template
        // If mappings match standard > 90%, use template
        // Otherwise, store custom mapping
        return false; // For now, always create custom template
    }

    convertMappingsToResourceFormat(mappings) {
        // Convert atomic mappings array to resource-grouped format
        // Group by FHIR resource (Patient, Encounter, etc.)
        const resources = {};

        for (const mapping of (mappings.atomicMappings || [])) {
            const resourceType = this.extractResourceType(mapping.fhirPath);
            if (!resources[resourceType]) {
                resources[resourceType] = { mappings: [] };
            }
            resources[resourceType].mappings.push(mapping);
        }

        return resources;
    }

    extractResourceType(fhirPath) {
        // Extract resource type from FHIR path
        // Example: "Patient.identifier[0].value" → "Patient"
        const match = fhirPath.match(/^([A-Z][a-zA-Z]+)\./);
        return match ? match[1] : 'Unknown';
    }
}

module.exports = TransformationPipelineService;
```

### Phase 3: Update Wizard Controller ✅

**File:** `controllers/wizardController.js`

```javascript
const TransformationPipelineService = require('../services/TransformationPipelineService');

class WizardController {
    constructor() {
        this.pipelineService = new TransformationPipelineService(
            require('./interfaceService').pool
        );
    }

    async createInterfaceWithMappings(interfaceData, userId, userEmail) {
        try {
            // Step 1: Create interface record (metadata only)
            const serviceData = {
                name: interfaceData.name,
                description: interfaceData.description,
                sourceType: interfaceData.sourceType,
                targetType: interfaceData.targetType,
                messageType: interfaceData.messageType,
                sourceConfig: interfaceData.sourceConfig,
                targetConfig: interfaceData.targetConfig,
                transformationMapping: null  // DEPRECATED - using pipeline
            };

            const result = await interfaceService.createInterface(serviceData, userId, userEmail);
            const interfaceId = this.extractInterfaceId(result);

            // Step 2: Create transformation pipeline with HL7→FHIR step
            console.log('📦 Creating transformation pipeline...');
            const pipelineId = await this.pipelineService.createPipeline(
                interfaceId,
                interfaceData.messageType,
                interfaceData.name,
                userId
            );

            // Step 3: Add HL7→FHIR mapping step with template
            console.log('🔀 Adding HL7→FHIR mapping step...');
            const templateId = await this.pipelineService.getOrCreateTemplate(
                interfaceData.messageType,
                interfaceData.transformationMapping
            );

            await this.pipelineService.addMappingStep(
                pipelineId,
                templateId,
                interfaceData.transformationMapping,
                userId
            );

            // Step 4: Add default pipeline steps
            console.log('⚙️ Adding default pipeline steps...');
            await this.pipelineService.addDefaultPipelineSteps(pipelineId, userId);

            console.log('✅ Pipeline created successfully:', {
                interfaceId,
                pipelineId,
                templateId,
                messageType: interfaceData.messageType
            });

            return { interfaceId, name: interfaceData.name };

        } catch (error) {
            console.error('❌ Failed to create interface with pipeline:', error);
            throw error;
        }
    }
}
```

---

## Migration Path

### For Existing Interfaces:

**Option 1: Leave As-Is**
- Existing interfaces continue using OOB templates
- Works fine, no action needed

**Option 2: Migrate to Pipeline Architecture**
- Create migration script
- Convert `transformation_mapping` → `transformation_steps`
- Store in proper pipeline structure

### For New Wizard Completions:

**✅ Use Pipeline Architecture ONLY**
- Remove legacy storage paths
- Create pipeline + steps atomically
- No fallback to interface_message_mappings

---

## Testing Plan

### Test 1: New Wizard Completion

```javascript
// Complete wizard with custom mapping
POST /api/wizard/complete
{
    "name": "Test Interface",
    "sourceType": "hl7v2",
    "targetType": "fhir",
    "messageType": "ADT^A01",
    "mappings": [
        { hl7Field: "PID.3.1", fhirPath: "Patient.identifier[0].value", ... },
        ...
    ]
}

// Verify storage:
1. ✅ interface created in interfaces table
2. ✅ pipeline created in transformation_pipelines
3. ✅ HL7→FHIR step created in transformation_steps
4. ✅ template created/referenced in hl7_fhir_templates
5. ✅ step config has template_id OR custom_mapping
```

### Test 2: Pipeline Execution

```javascript
// Send HL7 message
POST /api/interfaces/{id}/process
{ hl7Message: "MSH|^~\\&|..." }

// Verify:
1. ✅ Pipeline loads from transformation_pipelines
2. ✅ HL7→FHIR step executes
3. ✅ Template/mapping loads correctly
4. ✅ FHIR output generated
5. ✅ No fallback to deprecated storage
```

---

## Rollout Plan

### Phase 1: Implement (3-4 hours)
- ✅ Create TransformationPipelineService
- ✅ Update WizardController
- ✅ Remove legacy storage paths
- ✅ Add pipeline creation

### Phase 2: Test (1 hour)
- ✅ Test new wizard completion
- ✅ Verify database storage
- ✅ Test pipeline execution
- ✅ Verify mapping retrieval

### Phase 3: Deploy (15 min)
- ✅ Build Docker image
- ✅ Restart containers
- ✅ Smoke test

---

## Success Criteria

✅ **Wizard completion creates:**
1. Interface record (metadata only)
2. Transformation pipeline (interface + message type)
3. Pipeline steps (including HL7→FHIR step)
4. Template reference (standard or custom)

✅ **NO MORE:**
1. ❌ transformation_mapping in interfaces table (deprecated)
2. ❌ interface_message_mappings table usage (deprecated)
3. ❌ Silent error swallowing
4. ❌ Multiple competing storage mechanisms

✅ **Pipeline execution:**
1. Loads pipeline from transformation_pipelines
2. Executes steps in sequence
3. HL7→FHIR step uses template OR custom mapping
4. FHIR output generated correctly

---

## Documentation Updates

1. ✅ WIZARD_MAPPING_STORAGE_CANONICAL_FLOW.md (this file)
2. ✅ Update SYSTEM_DOCUMENTATION.md with pipeline architecture
3. ✅ Update CLAUDE.md with canonical flow
4. ✅ Mark interface_message_mappings as DEPRECATED

---

**Status:** 📋 READY FOR IMPLEMENTATION
**Approval Required:** YES
**Breaking Changes:** NO (new wizard flow only)
**Estimated Time:** 4-5 hours total
