/**
 * FHIRLoopProvider - Loop type provider for FHIR R4 resources
 *
 * Provides human-friendly loop options for common FHIR repeating elements.
 * Users see "Each Bundle Entry" instead of "Bundle.entry".
 *
 * @see docs/LOOP_TYPE_PROVIDERS.md for full architecture documentation
 * @extends BaseLoopTypeProvider
 */
class FHIRLoopProvider extends BaseLoopTypeProvider {
    constructor() {
        super('fhir', 'FHIR R4');
    }

    /**
     * Detect if a message is FHIR format
     * @param {Object|string} message - The message to check
     * @returns {boolean} True if message is FHIR
     */
    detectFormat(message) {
        // Check for FHIR resource type
        if (message?.resourceType) {
            return true;
        }
        // Check for FHIR Bundle
        if (message?.type && message?.entry) {
            return true;
        }
        // Check for JSON string containing resourceType
        if (typeof message === 'string') {
            try {
                const parsed = JSON.parse(message);
                return !!parsed.resourceType;
            } catch {
                return false;
            }
        }
        return false;
    }

    /**
     * Get FHIR-specific loop options
     * Options are organized by category for easy browsing
     *
     * @returns {Array<LoopOption>} FHIR loop options
     */
    getLoopOptions() {
        return [
            // ============================================
            // BUNDLE & COLLECTIONS
            // ============================================
            {
                id: 'bundle-entry',
                icon: 'fas fa-layer-group',
                label: 'Each Bundle Entry',
                description: 'Loop through each resource in a FHIR Bundle',
                technicalPath: 'Bundle.entry',
                category: 'bundle',
                availableVariables: [
                    { name: 'loop.item', description: 'Current bundle entry' },
                    { name: 'loop.item.resource', description: 'The FHIR resource' },
                    { name: 'loop.item.fullUrl', description: 'Full URL of the resource' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'contained',
                icon: 'fas fa-box',
                label: 'Each Contained Resource',
                description: 'Loop through contained resources',
                technicalPath: 'contained',
                category: 'bundle',
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // PATIENT DEMOGRAPHICS
            // ============================================
            {
                id: 'identifier',
                icon: 'fas fa-id-card',
                label: 'Each Identifier',
                description: 'Loop through patient/resource identifiers (MRN, SSN, etc.)',
                technicalPath: 'identifier',
                category: 'demographics',
                availableVariables: [
                    { name: 'loop.item', description: 'Current identifier' },
                    { name: 'loop.item.system', description: 'Identifier system/namespace' },
                    { name: 'loop.item.value', description: 'Identifier value' },
                    { name: 'loop.item.type', description: 'Identifier type' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'name',
                icon: 'fas fa-signature',
                label: 'Each Name',
                description: 'Loop through person names (legal, nickname, etc.)',
                technicalPath: 'name',
                category: 'demographics',
                availableVariables: [
                    { name: 'loop.item', description: 'Current name' },
                    { name: 'loop.item.family', description: 'Family/last name' },
                    { name: 'loop.item.given', description: 'Given/first names' },
                    { name: 'loop.item.use', description: 'Name use (official, nickname)' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'address',
                icon: 'fas fa-map-marker-alt',
                label: 'Each Address',
                description: 'Loop through addresses (home, work, etc.)',
                technicalPath: 'address',
                category: 'demographics',
                availableVariables: [
                    { name: 'loop.item', description: 'Current address' },
                    { name: 'loop.item.line', description: 'Street address lines' },
                    { name: 'loop.item.city', description: 'City' },
                    { name: 'loop.item.state', description: 'State/province' },
                    { name: 'loop.item.postalCode', description: 'Postal/ZIP code' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'telecom',
                icon: 'fas fa-phone',
                label: 'Each Contact Method',
                description: 'Loop through phone numbers, emails, etc.',
                technicalPath: 'telecom',
                category: 'demographics',
                availableVariables: [
                    { name: 'loop.item', description: 'Current contact method' },
                    { name: 'loop.item.system', description: 'Type (phone, email, etc.)' },
                    { name: 'loop.item.value', description: 'The actual number/address' },
                    { name: 'loop.item.use', description: 'Use (home, work, mobile)' },
                    ...this.getStandardVariables().slice(1)
                ]
            },

            // ============================================
            // CLINICAL DATA
            // ============================================
            {
                id: 'condition',
                icon: 'fas fa-diagnoses',
                label: 'Each Condition',
                description: 'Loop through conditions/diagnoses',
                technicalPath: 'Condition',
                category: 'clinical',
                resourceType: true,
                availableVariables: [
                    { name: 'loop.item', description: 'Current condition resource' },
                    { name: 'loop.item.code', description: 'Condition code' },
                    { name: 'loop.item.clinicalStatus', description: 'Clinical status' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'observation',
                icon: 'fas fa-flask',
                label: 'Each Observation',
                description: 'Loop through observations/lab results',
                technicalPath: 'Observation',
                category: 'clinical',
                resourceType: true,
                availableVariables: [
                    { name: 'loop.item', description: 'Current observation' },
                    { name: 'loop.item.code', description: 'Observation type' },
                    { name: 'loop.item.value', description: 'Result value' },
                    { name: 'loop.item.status', description: 'Status' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'medication',
                icon: 'fas fa-pills',
                label: 'Each Medication',
                description: 'Loop through medications',
                technicalPath: 'MedicationRequest',
                category: 'clinical',
                resourceType: true,
                availableVariables: this.getStandardVariables()
            },
            {
                id: 'allergy',
                icon: 'fas fa-allergies',
                label: 'Each Allergy',
                description: 'Loop through allergy intolerances',
                technicalPath: 'AllergyIntolerance',
                category: 'clinical',
                resourceType: true,
                availableVariables: this.getStandardVariables()
            },
            {
                id: 'procedure',
                icon: 'fas fa-procedures',
                label: 'Each Procedure',
                description: 'Loop through procedures',
                technicalPath: 'Procedure',
                category: 'clinical',
                resourceType: true,
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // COVERAGE & FINANCIAL
            // ============================================
            {
                id: 'coverage',
                icon: 'fas fa-hospital',
                label: 'Each Coverage',
                description: 'Loop through insurance coverages',
                technicalPath: 'Coverage',
                category: 'financial',
                resourceType: true,
                availableVariables: [
                    { name: 'loop.item', description: 'Current coverage' },
                    { name: 'loop.item.payor', description: 'Insurance payor' },
                    { name: 'loop.item.subscriberId', description: 'Subscriber ID' },
                    ...this.getStandardVariables().slice(1)
                ]
            },

            // ============================================
            // ENCOUNTERS & SCHEDULING
            // ============================================
            {
                id: 'encounter',
                icon: 'fas fa-hospital-user',
                label: 'Each Encounter',
                description: 'Loop through patient encounters/visits',
                technicalPath: 'Encounter',
                category: 'encounters',
                resourceType: true,
                availableVariables: this.getStandardVariables()
            },
            {
                id: 'appointment',
                icon: 'fas fa-calendar-check',
                label: 'Each Appointment',
                description: 'Loop through appointments',
                technicalPath: 'Appointment',
                category: 'encounters',
                resourceType: true,
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // COMMON ARRAYS (within any resource)
            // ============================================
            {
                id: 'extension',
                icon: 'fas fa-puzzle-piece',
                label: 'Each Extension',
                description: 'Loop through FHIR extensions',
                technicalPath: 'extension',
                category: 'structure',
                availableVariables: [
                    { name: 'loop.item', description: 'Current extension' },
                    { name: 'loop.item.url', description: 'Extension URL/identifier' },
                    { name: 'loop.item.value', description: 'Extension value' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'coding',
                icon: 'fas fa-code',
                label: 'Each Coding',
                description: 'Loop through codings in a CodeableConcept',
                technicalPath: 'coding',
                category: 'structure',
                availableVariables: [
                    { name: 'loop.item', description: 'Current coding' },
                    { name: 'loop.item.system', description: 'Code system (SNOMED, LOINC, etc.)' },
                    { name: 'loop.item.code', description: 'The code value' },
                    { name: 'loop.item.display', description: 'Human-readable display' },
                    ...this.getStandardVariables().slice(1)
                ]
            }
        ];
    }

    /**
     * Get category metadata for UI organization
     * @returns {Array<CategoryInfo>} Category information
     */
    getCategoryInfo() {
        return [
            { id: 'bundle', label: 'Bundle & Collections', icon: 'fas fa-layer-group', order: 1 },
            { id: 'demographics', label: 'Demographics', icon: 'fas fa-user', order: 2 },
            { id: 'clinical', label: 'Clinical', icon: 'fas fa-heartbeat', order: 3 },
            { id: 'financial', label: 'Financial', icon: 'fas fa-dollar-sign', order: 4 },
            { id: 'encounters', label: 'Encounters', icon: 'fas fa-hospital', order: 5 },
            { id: 'structure', label: 'Structure', icon: 'fas fa-sitemap', order: 6 },
            { id: 'utility', label: 'Other Options', icon: 'fas fa-tools', order: 99 }
        ];
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = FHIRLoopProvider;
}
