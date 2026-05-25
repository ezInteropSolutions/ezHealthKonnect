// services/PipelineService.js
// Pipeline service for template library and utilities
// MVC: This is a SERVICE - contains business logic

class PipelineService {
    constructor() {
        // Template library (OOB: preloaded templates)
        this.templates = this.initializeTemplateLibrary();
    }

    /**
     * Initialize template library with OOB templates
     */
    initializeTemplateLibrary() {
        return [
            // =============================================
            // PRE-PROCESSING TEMPLATES
            // =============================================
            {
                id: 'validate-patient-id',
                name: 'Validate Patient ID',
                category: 'Pre-Processing',
                type: 'pre.validation',
                icon: '✓',
                description: 'Validates that Patient ID (PID.3) is present and valid',
                tags: ['validation', 'required', 'patient'],
                defaultConfig: {
                    rules: [
                        {
                            field: 'PID.3',
                            required: true,
                            minLength: 1,
                            description: 'Patient ID'
                        }
                    ]
                },
                layer: 'pre',
                executionMode: 'parallel'
            },
            {
                id: 'validate-patient-demographics',
                name: 'Validate Patient Demographics',
                category: 'Pre-Processing',
                type: 'pre.validation',
                icon: '✓',
                description: 'Validates patient name, DOB, and gender',
                tags: ['validation', 'demographics'],
                defaultConfig: {
                    rules: [
                        {
                            field: 'PID.5',
                            required: true,
                            description: 'Patient Name'
                        },
                        {
                            field: 'PID.7',
                            required: false,
                            description: 'Date of Birth'
                        },
                        {
                            field: 'PID.8',
                            required: false,
                            description: 'Gender'
                        }
                    ]
                },
                layer: 'pre',
                executionMode: 'parallel'
            },
            {
                id: 'check-duplicate-patient',
                name: 'Check Duplicate Patient',
                category: 'Pre-Processing',
                type: 'pre.validation',
                icon: '🔍',
                description: 'Checks if patient already exists in the system',
                tags: ['validation', 'duplicate', 'patient'],
                defaultConfig: {
                    checkFields: ['PID.3', 'PID.5', 'PID.7'],
                    action: 'warn' // 'warn', 'fail', 'skip'
                },
                layer: 'pre',
                executionMode: 'parallel'
            },
            {
                id: 'enrich-from-epic',
                name: 'Enrich from Epic API',
                category: 'Pre-Processing',
                type: 'pre.enrichment',
                icon: '🔍',
                description: 'Enriches patient data from Epic FHIR API',
                tags: ['enrichment', 'epic', 'api'],
                defaultConfig: {
                    apiEndpoint: '',
                    apiKey: '',
                    timeout: 2000,
                    optional: true
                },
                layer: 'pre',
                executionMode: 'inline'
            },
            {
                id: 'custom-vip-detection',
                name: 'Mark VIP Patients',
                category: 'Pre-Processing',
                type: 'custom.javascript',
                icon: '⭐',
                description: 'Custom script to identify and mark VIP patients',
                tags: ['custom', 'vip', 'business-logic'],
                scriptTemplate: `function transform(input) {
    // Access patient name from HL7
    var pid = input.enhancedSegments.PID;
    var patientName = pid.fields.find(f => f.key === "PID.5");

    // Check if VIP (example logic)
    if (patientName && patientName.value.includes("VIP")) {
        input._metadata = input._metadata || {};
        input._metadata.priority = "high";
        input._metadata.isVIP = true;
    }

    return input;
}`,
                layer: 'pre',
                executionMode: 'parallel'
            },

            // =============================================
            // CORE MAPPING TEMPLATES
            // =============================================
            {
                id: 'hl7-fhir-adt-a01',
                name: 'HL7 ADT^A01 → FHIR Patient',
                category: 'Core Mapping',
                type: 'core.mapping',
                icon: '🔄',
                description: 'Transforms HL7 ADT^A01 admission message to FHIR Patient resource',
                tags: ['hl7', 'fhir', 'adt', 'patient'],
                defaultConfig: {
                    fhir_version: 'R4',
                    bundle_type: 'transaction',
                    use_template: true,
                    template_id: 'ADT_A01_to_FHIR_Patient',
                    strict_mode: false
                },
                layer: 'core',
                executionMode: 'waterfall'
            },
            {
                id: 'hl7-fhir-oru-r01',
                name: 'HL7 ORU^R01 → FHIR Observation',
                category: 'Core Mapping',
                type: 'core.mapping',
                icon: '🔄',
                description: 'Transforms HL7 ORU^R01 lab results to FHIR Observation resources',
                tags: ['hl7', 'fhir', 'oru', 'lab', 'observation'],
                defaultConfig: {
                    fhir_version: 'R4',
                    bundle_type: 'transaction',
                    use_template: true,
                    template_id: 'ORU_R01_to_FHIR_Observation',
                    strict_mode: false
                },
                layer: 'core',
                executionMode: 'waterfall'
            },

            // =============================================
            // POST-PROCESSING TEMPLATES
            // =============================================
            {
                id: 'validate-fhir-bundle',
                name: 'Validate FHIR Bundle',
                category: 'Post-Processing',
                type: 'post.validation',
                icon: '✅',
                description: 'Validates generated FHIR bundle against R4 specification',
                tags: ['validation', 'fhir', 'bundle'],
                defaultConfig: {
                    fhir_version: 'R4',
                    strict_mode: false,
                    validate_references: true
                },
                layer: 'post',
                executionMode: 'parallel'
            },
            {
                id: 'anonymize-test-patients',
                name: 'Anonymize Test Patients',
                category: 'Post-Processing',
                type: 'custom.javascript',
                icon: '🔒',
                description: 'Anonymizes PHI for test/demo patients',
                tags: ['custom', 'anonymization', 'phi'],
                scriptTemplate: `function transform(input) {
    var fhirBundle = input.fhirBundle;

    // Check if test patient
    if (fhirBundle.entry && fhirBundle.entry[0].resource.resourceType === "Patient") {
        var patient = fhirBundle.entry[0].resource;

        if (patient.name && patient.name[0].family === "TEST") {
            // Anonymize
            patient.name[0].family = "***";
            patient.name[0].given = ["***"];
            if (patient.telecom) {
                patient.telecom = [];
            }
        }
    }

    return input;
}`,
                layer: 'post',
                executionMode: 'parallel'
            },
            {
                id: 'audit-log-transformation',
                name: 'Audit Log Transformation',
                category: 'Post-Processing',
                type: 'custom.javascript',
                icon: '📝',
                description: 'Logs transformation details for audit trail',
                tags: ['audit', 'logging', 'compliance'],
                scriptTemplate: `function transform(input) {
    // Add audit metadata
    input._audit = {
        timestamp: new Date().toISOString(),
        transformedBy: "ezHealthKonnect",
        messageType: input.messageType,
        success: true
    };

    return input;
}`,
                layer: 'post',
                executionMode: 'parallel'
            }
        ];
    }

    /**
     * Get all templates
     */
    async getTemplateLibrary() {
        return this.templates;
    }

    /**
     * Get template by ID
     */
    async getTemplate(templateId) {
        return this.templates.find(t => t.id === templateId);
    }

    /**
     * Get templates by category
     */
    async getTemplatesByCategory(category) {
        return this.templates.filter(t => t.category === category);
    }

    /**
     * Get templates by type
     */
    async getTemplatesByType(type) {
        return this.templates.filter(t => t.type === type);
    }

    /**
     * Search templates
     */
    async searchTemplates(query) {
        const lowerQuery = query.toLowerCase();
        return this.templates.filter(t =>
            t.name.toLowerCase().includes(lowerQuery) ||
            t.description.toLowerCase().includes(lowerQuery) ||
            t.tags.some(tag => tag.toLowerCase().includes(lowerQuery))
        );
    }

    /**
     * Create custom template
     */
    async createCustomTemplate(templateData) {
        const template = {
            id: `custom_${Date.now()}`,
            name: templateData.name,
            category: templateData.category || 'Custom',
            type: templateData.type,
            icon: templateData.icon || '⚙️',
            description: templateData.description || '',
            tags: templateData.tags || ['custom'],
            defaultConfig: templateData.config || {},
            scriptTemplate: templateData.scriptTemplate,
            layer: templateData.layer || 'pre',
            executionMode: templateData.executionMode || 'inline',
            isCustom: true
        };

        this.templates.push(template);

        return template;
    }

    /**
     * Get OOB pipeline templates (complete pipelines)
     */
    async getCompletePipelineTemplates() {
        return [
            {
                id: 'adt-a01-standard',
                name: 'ADT^A01 Standard Pipeline',
                messageType: 'ADT^A01',
                description: 'Complete pipeline for patient admission messages',
                layers: {
                    pre: {
                        mode: 'parallel',
                        groups: [
                            {
                                id: 'parallel_validation',
                                type: 'parallel',
                                steps: [
                                    { templateId: 'validate-patient-id' },
                                    { templateId: 'validate-patient-demographics' },
                                    { templateId: 'check-duplicate-patient' }
                                ],
                                dependsOn: []
                            }
                        ]
                    },
                    core: {
                        mode: 'waterfall',
                        groups: [
                            {
                                id: 'core_mapping',
                                type: 'inline',
                                steps: [
                                    { templateId: 'hl7-fhir-adt-a01' }
                                ],
                                dependsOn: ['parallel_validation']
                            }
                        ]
                    },
                    post: {
                        mode: 'parallel',
                        groups: [
                            {
                                id: 'post_processing',
                                type: 'parallel',
                                steps: [
                                    { templateId: 'validate-fhir-bundle' },
                                    { templateId: 'audit-log-transformation' }
                                ],
                                dependsOn: ['core_mapping']
                            }
                        ]
                    }
                }
            },
            {
                id: 'oru-r01-standard',
                name: 'ORU^R01 Lab Results Pipeline',
                messageType: 'ORU^R01',
                description: 'Complete pipeline for lab results',
                layers: {
                    pre: {
                        mode: 'parallel',
                        groups: [
                            {
                                id: 'validation',
                                type: 'parallel',
                                steps: [
                                    { templateId: 'validate-patient-id' }
                                ],
                                dependsOn: []
                            }
                        ]
                    },
                    core: {
                        mode: 'waterfall',
                        groups: [
                            {
                                id: 'mapping',
                                type: 'inline',
                                steps: [
                                    { templateId: 'hl7-fhir-oru-r01' }
                                ],
                                dependsOn: ['validation']
                            }
                        ]
                    },
                    post: {
                        mode: 'parallel',
                        groups: [
                            {
                                id: 'post',
                                type: 'parallel',
                                steps: [
                                    { templateId: 'validate-fhir-bundle' }
                                ],
                                dependsOn: ['mapping']
                            }
                        ]
                    }
                }
            }
        ];
    }

    /**
     * Expand template into full step configuration
     */
    async expandTemplate(templateId) {
        const template = await this.getTemplate(templateId);

        if (!template) {
            throw new Error(`Template not found: ${templateId}`);
        }

        return {
            id: `step_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`,
            name: template.name,
            type: template.type,
            templateId: template.id,
            config: { ...template.defaultConfig },
            required: template.defaultConfig.required !== false,
            scriptContent: template.scriptTemplate || '',
            onErrorStrategy: template.defaultConfig.onErrorStrategy || 'fail',
            executionGroup: {
                groupType: template.executionMode,
                layer: template.layer
            }
        };
    }
}

// Export singleton instance
module.exports = new PipelineService();
