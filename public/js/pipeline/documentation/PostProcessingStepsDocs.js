/**
 * PostProcessingStepsDocs — Documentation for post.* steps
 *
 * post.validation, post.delivery, plus the legacy fhir_validation alias.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'post.validation': {
                description: 'Validates FHIR data against the R4 specification. Works in two modes: <strong>Bundle mode</strong> (validates bundle structure + all entry resources) when <code>fhirBundle</code> contains a Bundle, and <strong>Resource mode</strong> (validates a single standalone resource) when <code>fhirResource</code> is present or <code>fhirBundle</code> contains a non-Bundle resource. Searches <code>fhirBundle</code>, <code>fhirResource</code>, <code>message.*</code>, and <code>enriched.*</code>.',
                useCases: [
                    'Validate FHIR bundle from HL7→FHIR Transform (bundle mode)',
                    'Validate a standalone FHIR resource from an API enrichment step (resource mode)',
                    'Validate required fields per resource type (14 types with hardcoded rules)',
                    'Check internal bundle references point to existing resources (bundle mode)',
                    'Enforce specific resource types must be present (e.g., Patient + Encounter)',
                    'Full R4 schema validation using 146 JSON schema definitions (strict mode)',
                    'Stop the pipeline on validation failure (fail_on_error: true)'
                ],
                example: {
                    validation_level: 'standard',
                    required_resources: ['Patient', 'Encounter'],
                    validate_references: true,
                    validate_required_fields: true,
                    fail_on_error: false
                },
                parameters: [
                    { name: 'validation_level', type: 'string', required: true, description: '"basic" (resourceType + id only), "standard" (+ required fields + references), "strict" (+ full R4 JSON schema). Default: "standard".' },
                    { name: 'required_resources', type: 'string[]', required: false, description: 'FHIR resource types that must be present. In bundle mode, checks all entries. In resource mode, checks the single resource matches. Example: ["Patient", "Encounter"].' },
                    { name: 'validate_references', type: 'boolean', required: false, description: 'Check that reference fields resolve to existing resources (by fullUrl or ResourceType/id). Most useful in bundle mode where references can be cross-checked. Default: true.' },
                    { name: 'validate_required_fields', type: 'boolean', required: false, description: 'Check FHIR-spec-required fields per resource type (e.g., Encounter must have status and class, Observation must have status and code). Default: true.' },
                    { name: 'fail_on_error', type: 'boolean', required: false, description: 'When true, the pipeline stops if any validation error is found. When false, errors are reported in step output but the pipeline continues. Default: false.' }
                ],
                validationTypes: [
                    {
                        type: 'basic',
                        description: 'Minimal structural check. Only verifies that each resource in the bundle has a resourceType and id.',
                        usedFor: 'Quick sanity check, development/debugging, high-throughput pipelines where speed matters.',
                        example: { validation_level: 'basic' }
                    },
                    {
                        type: 'standard',
                        description: 'Checks required fields per resource type using hardcoded rules for 14 common FHIR resource types. Also validates that internal bundle references point to existing resources.',
                        usedFor: 'Production pipelines. Catches common mapping errors like missing Encounter.status or MessageHeader.event.',
                        example: {
                            validation_level: 'standard',
                            required_resources: ['Patient', 'Encounter'],
                            validate_references: true,
                            validate_required_fields: true,
                            fail_on_error: true
                        }
                    },
                    {
                        type: 'strict',
                        description: 'Full R4 JSON schema validation using 146 FHIR resource schema definitions. Validates required fields from the official spec, element data types, and cardinality constraints.',
                        usedFor: 'Compliance validation, regulatory submissions, interoperability testing.',
                        example: {
                            validation_level: 'strict',
                            required_resources: ['Patient', 'Encounter', 'Observation'],
                            fail_on_error: true
                        }
                    }
                ]
            },
            'post.delivery': {
                description: 'Delivers transformed FHIR resources to target FHIR server or other destinations. Handles retries and error recovery.',
                useCases: [
                    'Send FHIR Bundle to healthcare FHIR server',
                    'Post individual resources to RESTful FHIR API',
                    'Deliver to multiple endpoints (primary + backup)',
                    'Archive to object storage (S3, Azure Blob)',
                    'Queue for async processing'
                ],
                example: {
                    endpoint: 'http://fhir-server:8080/fhir',
                    resource: 'Bundle',
                    method: 'POST',
                    retry_count: 3,
                    retry_delay_ms: 1000,
                    auth: {
                        type: 'bearer',
                        token: '${FHIR_TOKEN}'
                    }
                },
                parameters: [
                    { name: 'endpoint', type: 'string', required: true, description: 'FHIR server URL' },
                    { name: 'resource', type: 'string', required: true, description: 'Resource type to send (Patient, Bundle, etc.)' },
                    { name: 'retry_count', type: 'number', required: false, description: 'Number of retry attempts (default: 3)' }
                ]
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('fhir_validation', 'post.validation');
