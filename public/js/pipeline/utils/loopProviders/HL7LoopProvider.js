/**
 * HL7LoopProvider - Loop type provider for HL7 v2.x messages
 *
 * Provides human-friendly loop options for common HL7 repeating segments.
 * Users see "Each Insurance Record" instead of "IN1".
 *
 * @see docs/LOOP_TYPE_PROVIDERS.md for full architecture documentation
 * @extends BaseLoopTypeProvider
 */
class HL7LoopProvider extends BaseLoopTypeProvider {
    constructor() {
        super('hl7', 'HL7 v2.x');
    }

    /**
     * Detect if a message is HL7 v2.x format
     * @param {Object|string} message - The message to check
     * @returns {boolean} True if message is HL7
     */
    detectFormat(message) {
        // Check for parsed HL7 structure
        if (message?.enhancedSegments || message?.segments) {
            return true;
        }
        // Check for raw HL7 string
        if (typeof message === 'string') {
            return message.startsWith('MSH|') || message.includes('\rMSH|');
        }
        // Check for messageType indicator
        if (message?.messageType || message?.type?.includes?.('HL7')) {
            return true;
        }
        return false;
    }

    /**
     * Get HL7-specific loop options
     * Options are organized by category for easy browsing
     *
     * @returns {Array<LoopOption>} HL7 loop options
     */
    getLoopOptions() {
        return [
            // ============================================
            // INSURANCE & FINANCIAL
            // ============================================
            {
                id: 'insurance',
                icon: 'fas fa-hospital',
                label: 'Each Insurance',
                description: 'Loop through each insurance plan (IN1/IN2 segments)',
                technicalPath: 'IN1',
                category: 'financial',
                segmentGroup: ['IN1', 'IN2', 'IN3'],
                availableVariables: [
                    { name: 'loop.item', description: 'Current insurance record with plan details' },
                    { name: 'loop.item.planId', description: 'Insurance plan ID' },
                    { name: 'loop.item.companyName', description: 'Insurance company name' },
                    ...this.getStandardVariables().slice(1) // Skip duplicate item
                ]
            },
            {
                id: 'guarantor',
                icon: 'fas fa-user-shield',
                label: 'Each Guarantor',
                description: 'Loop through each guarantor (GT1 segments)',
                technicalPath: 'GT1',
                category: 'financial',
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // CLINICAL RESULTS
            // ============================================
            {
                id: 'observation',
                icon: 'fas fa-flask',
                label: 'Each Test Result',
                description: 'Loop through each observation/result (OBX segments)',
                technicalPath: 'OBX',
                category: 'results',
                availableVariables: [
                    { name: 'loop.item', description: 'Current test result' },
                    { name: 'loop.item.testCode', description: 'Test/observation code' },
                    { name: 'loop.item.value', description: 'Result value' },
                    { name: 'loop.item.units', description: 'Units of measure' },
                    { name: 'loop.item.status', description: 'Result status' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'order',
                icon: 'fas fa-clipboard-list',
                label: 'Each Order',
                description: 'Loop through each order (ORC/OBR segments)',
                technicalPath: 'OBR',
                category: 'results',
                segmentGroup: ['ORC', 'OBR'],
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // DIAGNOSES & PROCEDURES
            // ============================================
            {
                id: 'diagnosis',
                icon: 'fas fa-diagnoses',
                label: 'Each Diagnosis',
                description: 'Loop through each diagnosis (DG1 segments)',
                technicalPath: 'DG1',
                category: 'clinical',
                availableVariables: [
                    { name: 'loop.item', description: 'Current diagnosis' },
                    { name: 'loop.item.code', description: 'Diagnosis code (ICD)' },
                    { name: 'loop.item.description', description: 'Diagnosis description' },
                    { name: 'loop.item.type', description: 'Diagnosis type (admitting, final, etc.)' },
                    ...this.getStandardVariables().slice(1)
                ]
            },
            {
                id: 'procedure',
                icon: 'fas fa-procedures',
                label: 'Each Procedure',
                description: 'Loop through each procedure (PR1 segments)',
                technicalPath: 'PR1',
                category: 'clinical',
                availableVariables: this.getStandardVariables()
            },
            {
                id: 'allergy',
                icon: 'fas fa-allergies',
                label: 'Each Allergy',
                description: 'Loop through each allergy (AL1 segments)',
                technicalPath: 'AL1',
                category: 'clinical',
                availableVariables: [
                    { name: 'loop.item', description: 'Current allergy record' },
                    { name: 'loop.item.allergen', description: 'Allergen name/code' },
                    { name: 'loop.item.reaction', description: 'Reaction description' },
                    { name: 'loop.item.severity', description: 'Severity level' },
                    ...this.getStandardVariables().slice(1)
                ]
            },

            // ============================================
            // CONTACTS & RELATIONSHIPS
            // ============================================
            {
                id: 'next-of-kin',
                icon: 'fas fa-users',
                label: 'Each Contact',
                description: 'Loop through each next of kin/contact (NK1 segments)',
                technicalPath: 'NK1',
                category: 'contacts',
                availableVariables: [
                    { name: 'loop.item', description: 'Current contact person' },
                    { name: 'loop.item.name', description: 'Contact name' },
                    { name: 'loop.item.relationship', description: 'Relationship to patient' },
                    { name: 'loop.item.phone', description: 'Phone number' },
                    ...this.getStandardVariables().slice(1)
                ]
            },

            // ============================================
            // SCHEDULING
            // ============================================
            {
                id: 'appointment',
                icon: 'fas fa-calendar-check',
                label: 'Each Appointment',
                description: 'Loop through each appointment/resource (AIS/AIG segments)',
                technicalPath: 'AIS',
                category: 'scheduling',
                segmentGroup: ['AIS', 'AIG', 'AIL', 'AIP'],
                availableVariables: this.getStandardVariables()
            },

            // ============================================
            // NOTES & COMMENTS
            // ============================================
            {
                id: 'notes',
                icon: 'fas fa-sticky-note',
                label: 'Each Note',
                description: 'Loop through each note/comment (NTE segments)',
                technicalPath: 'NTE',
                category: 'notes',
                availableVariables: [
                    { name: 'loop.item', description: 'Current note' },
                    { name: 'loop.item.text', description: 'Note text content' },
                    { name: 'loop.item.type', description: 'Note type/category' },
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
            { id: 'financial', label: 'Financial', icon: 'fas fa-dollar-sign', order: 1 },
            { id: 'results', label: 'Results & Orders', icon: 'fas fa-vials', order: 2 },
            { id: 'clinical', label: 'Clinical', icon: 'fas fa-heartbeat', order: 3 },
            { id: 'contacts', label: 'Contacts', icon: 'fas fa-address-book', order: 4 },
            { id: 'scheduling', label: 'Scheduling', icon: 'fas fa-calendar', order: 5 },
            { id: 'notes', label: 'Notes', icon: 'fas fa-comment-medical', order: 6 },
            { id: 'utility', label: 'Other Options', icon: 'fas fa-tools', order: 99 }
        ];
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = HL7LoopProvider;
}
