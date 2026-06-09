/**
 * FlowRegistry — single source of truth for all wizard transformation flow definitions.
 *
 * Each entry controls:
 *   - Wizard UI rendering  (WizardView, WizardModel step-skip logic)
 *   - Pipeline generation  (TransformationPipelineService.FLOW_STEP_TEMPLATES — same keys)
 *
 * Adding a new flow: add one entry here; the wizard and pipeline service pick it up automatically.
 */
const FLOW_REGISTRY = {
    hl7_to_fhir: {
        label: 'HL7 v2.x → FHIR R4 (Recommended)',
        group: 'auto',
        sourceFormat: 'hl7v2',
        targetFormat: 'fhir_r4',
        showFamilyFilter: true,
        showMappingUI: true,
        isSinkOnly: false,
        hasDelivery: true,
        skipUploadSteps: false,
        redirectToPipelineBuilder: false
    },
    hl7_to_fhir_r5: {
        label: 'HL7 v2.x → FHIR R5 (Emerging Standard)',
        group: 'auto',
        sourceFormat: 'hl7v2',
        targetFormat: 'fhir_r5',
        showFamilyFilter: true,
        showMappingUI: true,
        isSinkOnly: false,
        hasDelivery: true,
        skipUploadSteps: false,
        redirectToPipelineBuilder: false
    },
    hl7_to_fhir_stu3: {
        label: 'HL7 v2.x → FHIR STU3',
        group: 'auto',
        sourceFormat: 'hl7v2',
        targetFormat: 'fhir_stu3',
        showFamilyFilter: true,
        showMappingUI: true,
        isSinkOnly: false,
        hasDelivery: true,
        skipUploadSteps: false,
        redirectToPipelineBuilder: false
    },
    ccd_to_fhir: {
        label: 'CCD/C-CDA → FHIR R4 (Automatic Transformation)',
        group: 'auto',
        sourceFormat: 'ccda',
        targetFormat: 'fhir_r4',
        showFamilyFilter: false,
        showMappingUI: false,      // CDA uses its own section panel; step 4 (HL7 mapping) is skipped
        isSinkOnly: false,
        hasDelivery: true,
        skipUploadSteps: false,    // Step 2 (CDA upload) and step 3 (CDA review) are used
        redirectToPipelineBuilder: false
    },
    passthrough: {
        label: 'Passthrough (Store only, no transformation)',
        group: 'sink',
        sourceFormat: null,
        targetFormat: null,
        showFamilyFilter: false,
        showMappingUI: false,
        isSinkOnly: true,
        hasDelivery: false,
        skipUploadSteps: true,
        redirectToPipelineBuilder: false
    },
    fhir_receiver: {
        label: 'FHIR Receiver (Direct storage, user-driven)',
        group: 'sink',
        sourceFormat: 'fhir',
        targetFormat: null,
        showFamilyFilter: false,
        showMappingUI: false,
        isSinkOnly: true,
        hasDelivery: false,
        skipUploadSteps: true,
        redirectToPipelineBuilder: false
    },
    file_processor: {
        label: 'File Processor (Batch processing, user-driven)',
        group: 'sink',
        sourceFormat: null,
        targetFormat: null,
        showFamilyFilter: false,
        showMappingUI: false,
        isSinkOnly: true,
        hasDelivery: false,
        skipUploadSteps: true,
        redirectToPipelineBuilder: false
    },
    others: {
        label: 'Other / Custom (Configure in Pipeline Builder)',
        group: 'custom',
        sourceFormat: null,
        targetFormat: null,
        showFamilyFilter: false,
        showMappingUI: false,
        isSinkOnly: true,
        hasDelivery: false,
        skipUploadSteps: true,
        redirectToPipelineBuilder: true   // skip completion modal; go straight to pipeline builder
    }
};

/**
 * Retrieve a flow definition by key.  Returns the hl7_to_fhir entry for unknown/empty keys.
 * @param {string} flowKey
 * @returns {object}
 */
function getFlow(flowKey) {
    return FLOW_REGISTRY[flowKey] || FLOW_REGISTRY['hl7_to_fhir'];
}
