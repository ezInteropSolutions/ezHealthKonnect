/**
 * PreProcessingStepsDocs — Documentation for validation/branching pre-processing steps
 *
 * pre.validation, pre.logic.switch, plus the legacy field_validation alias.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'pre.validation': {
                description: 'Validates incoming HL7 messages against defined rules. Use step-level controls (Required + Error Strategy) to control whether validation failures stop the pipeline (NACK) or continue with warnings (ACK).',
                useCases: [
                    'Critical validation (Required=true): Patient safety fields → NACK on failure, stop pipeline',
                    'Data quality monitoring (Required=false + Error Strategy=continue): Accept with warnings → ACK, continue pipeline',
                    'Skip validation (Enabled=false): Emergency bypass → Skip all checks',
                    'Validate field formats (dates, numeric values, coded values)',
                    'Enforce business rules (age ranges, allowed codes, required fields)'
                ],
                example: {
                    rules: [
                        { field: 'enhancedSegments.PID.fields[0].value', type: 'required', errorMessage: 'Patient ID is required' },
                        { field: 'enhancedSegments.PID.fields[2].value', type: 'format', pattern: '^\\d{8}$', errorMessage: 'Birth date must be YYYYMMDD format' },
                        { field: 'enhancedSegments.PID.fields[1].subfields[0].value', type: 'required', errorMessage: 'Family name is required' },
                        { field: 'enhancedSegments.PV1.fields[1].value', type: 'enum', allowedValues: ['I', 'O', 'E'], errorMessage: 'Patient class must be I, O, or E' }
                    ]
                },
                parameters: [
                    {
                        name: 'field',
                        type: 'string',
                        required: true,
                        description: 'JSONPath to the field in parsed HL7 message. Use field selector autocomplete to choose from available fields. Examples: "enhancedSegments.PID.fields[0].value" for Patient ID, "enhancedSegments.PID.fields[1].subfields[0].value" for Family Name (atomic subfield).'
                    },
                    {
                        name: 'type',
                        type: 'enum',
                        required: true,
                        description: 'Validation type to apply. Available types: "required" (field must have a value), "format" (value must match regex pattern), "range" (numeric value within min/max), "length" (string length constraints), "enum" (value must be in allowed list), "date" (valid date format), "custom" (custom JavaScript validation).'
                    },
                    {
                        name: 'errorMessage',
                        type: 'string',
                        required: true,
                        description: 'Human-readable error message displayed when validation fails. Auto-populated based on field and validation type, but can be customized. Example: "Patient ID is required" or "Birth date must be YYYYMMDD format".'
                    },
                    {
                        name: 'pattern',
                        type: 'string (regex)',
                        required: false,
                        description: 'Regular expression pattern for "format" validation type. Example: "^\\d{8}$" for 8-digit date, "^[A-Z]{2}\\d{5}$" for 2 letters + 5 digits.'
                    },
                    {
                        name: 'minLength / maxLength',
                        type: 'number',
                        required: false,
                        description: 'String length constraints for "length" validation type. Example: minLength=1, maxLength=100 for patient name.'
                    },
                    {
                        name: 'min / max',
                        type: 'number',
                        required: false,
                        description: 'Numeric value range for "range" validation type. Example: min=0, max=120 for patient age.'
                    },
                    {
                        name: 'allowedValues',
                        type: 'Array<string>',
                        required: false,
                        description: 'List of allowed values for "enum" validation type. Example: ["M", "F", "U"] for administrative sex.'
                    },
                    {
                        name: 'customScript',
                        type: 'string (JavaScript)',
                        required: false,
                        description: 'Custom JavaScript function for "custom" validation type. Function receives field value and returns true/false. Example: "function(value) { return new Date(value) > new Date(\'1900-01-01\'); }"'
                    }
                ],
                validationTypes: [
                    {
                        type: 'required',
                        description: 'Field must have a non-empty value',
                        example: { field: 'enhancedSegments.PID.fields[0].value', type: 'required', errorMessage: 'Patient ID is required' },
                        usedFor: 'Ensuring critical fields like Patient ID, Message Type, or Date of Birth are present'
                    },
                    {
                        type: 'format',
                        description: 'Field value must match a regular expression pattern',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'format', pattern: '^\\d{8}$', errorMessage: 'Birth date must be YYYYMMDD' },
                        usedFor: 'Validating date formats, ID patterns, phone numbers, postal codes'
                    },
                    {
                        type: 'length',
                        description: 'String length must be within specified min/max range',
                        example: { field: 'enhancedSegments.PID.fields[1].subfields[0].value', type: 'length', minLength: 1, maxLength: 50, errorMessage: 'Family name must be 1-50 characters' },
                        usedFor: 'Enforcing name length limits, comment field constraints'
                    },
                    {
                        type: 'range',
                        description: 'Numeric value must be within specified min/max range',
                        example: { field: 'enhancedSegments.OBX.fields[4].value', type: 'range', min: 0, max: 300, errorMessage: 'Glucose level must be 0-300' },
                        usedFor: 'Validating lab values, patient age, vital signs'
                    },
                    {
                        type: 'enum',
                        description: 'Field value must be one of the allowed values',
                        example: { field: 'enhancedSegments.PID.fields[3].value', type: 'enum', allowedValues: ['M', 'F', 'U', 'O'], errorMessage: 'Sex must be M, F, U, or O' },
                        usedFor: 'Validating coded values like gender, patient class, result status'
                    },
                    {
                        type: 'date',
                        description: 'Field must be a valid date in specified format',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'date', format: 'YYYYMMDD', errorMessage: 'Invalid birth date format' },
                        usedFor: 'Validating birth dates, admission dates, observation timestamps'
                    },
                    {
                        type: 'custom',
                        description: 'Custom JavaScript validation function',
                        example: { field: 'enhancedSegments.PID.fields[2].value', type: 'custom', customScript: 'function(value) { const age = (Date.now() - new Date(value)) / 31557600000; return age >= 0 && age <= 120; }', errorMessage: 'Patient age must be 0-120 years' },
                        usedFor: 'Complex business rules, cross-field validation, calculated validations'
                    }
                ],
                fieldExamples: [
                    { path: 'enhancedSegments.MSH.fields[1].value', description: 'Message Type (e.g., ADT^A01)', segment: 'MSH', field: 'MSH.9' },
                    { path: 'enhancedSegments.PID.fields[0].value', description: 'Patient ID', segment: 'PID', field: 'PID.3' },
                    { path: 'enhancedSegments.PID.fields[1].value', description: 'Patient Name (full)', segment: 'PID', field: 'PID.5' },
                    { path: 'enhancedSegments.PID.fields[1].subfields[0].value', description: 'Family Name (atomic)', segment: 'PID', field: 'PID.5.1' },
                    { path: 'enhancedSegments.PID.fields[1].subfields[1].value', description: 'Given Name (atomic)', segment: 'PID', field: 'PID.5.2' },
                    { path: 'enhancedSegments.PID.fields[2].value', description: 'Date of Birth', segment: 'PID', field: 'PID.7' },
                    { path: 'enhancedSegments.PID.fields[3].value', description: 'Administrative Sex', segment: 'PID', field: 'PID.8' },
                    { path: 'enhancedSegments.PV1.fields[1].value', description: 'Patient Class (I/O/E)', segment: 'PV1', field: 'PV1.2' },
                    { path: 'enhancedSegments.PV1.fields[10].value', description: 'Admission Date/Time', segment: 'PV1', field: 'PV1.44' }
                ]
            },
            'pre.logic.switch': {
                description: 'Routes messages through different processing paths based on field value matching. Evaluates a single field against multiple case values and executes corresponding actions. Supports multi-step routing where a single case can trigger a sequence of steps to execute in order. Ideal for message type routing, status-based processing, and complex multi-branch conditional workflows.',
                useCases: [
                    'Route messages by type (ADT^A01 → admission flow, ADT^A03 → discharge flow, ORU^R01 → lab flow)',
                    'Process by patient class (I → inpatient enrichment, O → outpatient enrichment, E → emergency fast-track)',
                    'Handle status codes (A → active processing, C → cancelled cleanup, P → pending queue)',
                    'Route by facility (FAC001 → Epic integration, FAC002 → Cerner integration, FAC003 → custom flow)',
                    'Process by priority (STAT → immediate, ROUTINE → normal queue, ASAP → expedited)',
                    'Branch by insurance type (MEDICARE → CMS validation, MEDICAID → state rules, COMMERCIAL → standard)',
                    'Multi-step workflows (ADT^A01 → [Validate Patient, Enrich Demographics, Route to ADT Handler])',
                    'Skip specific steps for certain cases (ORU^R01 → skip patient enrichment, go directly to lab processing)'
                ],
                example: {
                    description: 'Route by message type with multi-step execution',
                    field: 'MSH.9.1',
                    cases: [
                        {
                            value: 'ADT',
                            label: 'ADT Messages',
                            actions: [
                                { action: 'set_value', targetField: 'metadata.category', value: 'admission' },
                                { action: 'route_to_step', targetStepIds: ['validate-patient', 'enrich-demographics', 'adt-handler'] }
                            ]
                        },
                        {
                            value: 'ORU',
                            label: 'Lab Results',
                            actions: [
                                { action: 'set_value', targetField: 'metadata.category', value: 'lab' },
                                { action: 'route_to_step', targetStepIds: ['validate-results', 'lab-handler'] }
                            ]
                        },
                        {
                            value: 'ORM',
                            label: 'Orders',
                            actions: [
                                { action: 'route_to_step', targetStepId: 'order-handler' }
                            ]
                        }
                    ],
                    default: {
                        actions: [
                            { action: 'set_value', targetField: 'metadata.category', value: 'unknown' },
                            { action: 'route_to_step', targetStepIds: ['log-warning', 'error-handler'] }
                        ]
                    },
                    options: { caseInsensitive: false, trimWhitespace: true }
                },
                parameters: [
                    { name: 'field', type: 'string', required: true, description: 'HL7 field path to evaluate. Use the field selector to choose from available fields. Examples: "MSH.9.1" for message type code, "PV1.2" for patient class, "PID.8" for gender.' },
                    { name: 'cases', type: 'Array<CaseDefinition>', required: true, description: 'Array of case definitions. Each case has a value to match and actions to execute when matched. Cases are evaluated in order; first match wins.' },
                    { name: 'cases[].value', type: 'string', required: true, description: 'The value to match against the field. Exact match by default (use options.caseInsensitive for case-insensitive matching).' },
                    { name: 'cases[].label', type: 'string', required: false, description: 'Human-readable label for this case. Displayed in the UI for documentation purposes.' },
                    { name: 'cases[].actions', type: 'Array<Action>', required: true, description: 'Actions to execute when this case matches. Multiple actions can be defined and execute in order.' },
                    { name: 'default', type: 'object', required: false, description: 'Fallback configuration when no cases match. Contains actions array.' },
                    { name: 'default.actions', type: 'Array<Action>', required: false, description: 'Actions to execute when no cases match. Typically "continue" or error handling.' },
                    { name: 'options.caseInsensitive', type: 'boolean', required: false, description: 'Perform case-insensitive value matching. Default: false. Set to true for values like "M"/"m" or "STAT"/"stat".' },
                    { name: 'options.trimWhitespace', type: 'boolean', required: false, description: 'Trim leading/trailing whitespace before comparison. Default: true. Handles HL7 padding automatically.' }
                ],
                actions: [
                    {
                        action: 'continue',
                        description: 'Continue to the next step in sequence',
                        usedFor: 'Normal flow - process continues to the next step based on sequence number',
                        parameters: 'None'
                    },
                    {
                        action: 'stop',
                        description: 'Stop pipeline execution immediately',
                        usedFor: 'Halting processing for invalid/unsupported message types, or when no further processing needed',
                        parameters: 'None'
                    },
                    {
                        action: 'set_value',
                        description: 'Set a field value in the message data',
                        usedFor: 'Tagging messages with category, routing metadata, or computed values',
                        parameters: 'targetField (where to set), value (what to set)'
                    },
                    {
                        action: 'copy_field',
                        description: 'Copy value from one field to another',
                        usedFor: 'Duplicating field values, creating backup copies, or preparing data for downstream steps',
                        parameters: 'sourceField (copy from), targetField (copy to)'
                    },
                    {
                        action: 'transform',
                        description: 'Apply a transformation to a field value',
                        usedFor: 'Formatting field values - uppercase, lowercase, trim, capitalize, substring',
                        parameters: 'targetField (field to transform), transformType (uppercase|lowercase|trim|capitalize)'
                    },
                    {
                        action: 'route_to_step',
                        description: 'Route to one or more specific steps by ID',
                        usedFor: 'Branching to different processing paths. Supports multi-step routing for complex workflows.',
                        parameters: 'targetStepId (single step) OR targetStepIds (array of steps to execute in order)'
                    },
                    {
                        action: 'skip_steps',
                        description: 'Skip specified steps and continue',
                        usedFor: 'Bypassing steps that are not relevant for this case (e.g., skip patient enrichment for lab messages)',
                        parameters: 'skipStepIds (array of step IDs to skip)'
                    }
                ],
                multiStepRouting: {
                    title: 'Multi-Step Routing',
                    description: 'A single case can route to multiple steps that execute in sequence. This enables complex workflows where one condition triggers a chain of processing steps.',
                    howToUse: [
                        'In the Configuration tab, select a case',
                        'Choose "Route to Step(s)" action',
                        'Use the dropdown to add multiple target steps',
                        'Steps are displayed as numbered chips (1. Step A, 2. Step B, etc.)',
                        'Click × on any chip to remove a step from the sequence'
                    ],
                    example: {
                        scenario: 'ADT messages need validation, enrichment, then specialized handling',
                        config: {
                            value: 'ADT',
                            actions: [{
                                action: 'route_to_step',
                                targetStepIds: ['validate-patient', 'enrich-demographics', 'adt-handler']
                            }]
                        },
                        execution: 'validate-patient runs first, then enrich-demographics, then adt-handler'
                    }
                },
                comparisonWithIfThenElse: {
                    title: 'Switch/Case vs If-Then-Else',
                    description: 'Both steps support conditional logic but are optimized for different scenarios:',
                    comparison: [
                        { feature: 'Best for', switchCase: 'Single field with multiple possible values', ifThenElse: 'Complex conditions with multiple fields and operators' },
                        { feature: 'Condition type', switchCase: 'Exact value matching (equals)', ifThenElse: 'Any comparison (equals, contains, greater than, less than, regex, is_empty)' },
                        { feature: 'Number of branches', switchCase: 'Many branches (3+ cases)', ifThenElse: 'Two branches (true path / false path)' },
                        { feature: 'Default handling', switchCase: 'Explicit default case', ifThenElse: 'Else branch for false conditions' },
                        { feature: 'Use Case Example', switchCase: 'Route ADT^A01/A02/A03/A04 to different flows', ifThenElse: 'If patient age > 65 AND has chronic condition, flag high-risk' }
                    ],
                    recommendation: 'Use Switch/Case when you have a single field with many possible values (like message type, patient class, facility code). Use If-Then-Else when you need complex boolean logic combining multiple conditions.'
                },
                bestPractices: [
                    {
                        practice: 'Use meaningful case labels',
                        reason: 'Labels make the configuration self-documenting and easier to maintain',
                        example: 'Label "Admission" for value "ADT^A01" instead of leaving blank'
                    },
                    {
                        practice: 'Always define a default case',
                        reason: 'Handles unexpected values gracefully instead of silent failures',
                        example: 'Default: log warning + continue, or route to error handler'
                    },
                    {
                        practice: 'Use caseInsensitive for user-entered data',
                        reason: 'HL7 data may have inconsistent casing (e.g., "M" vs "m" for male)',
                        example: 'Enable caseInsensitive for PID.8 (gender), PV1.2 (patient class)'
                    },
                    {
                        practice: 'Keep actions simple per case',
                        reason: 'Complex logic should be in dedicated steps, not crammed into switch actions',
                        example: 'Use route_to_step to branch to specialized processing steps rather than multiple set_value actions'
                    },
                    {
                        practice: 'Test with edge cases',
                        reason: 'Empty values, null fields, and whitespace can cause unexpected matching',
                        example: 'Test with empty MSH.9, whitespace-padded values, and unexpected message types'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Case not matching expected value',
                        cause: 'Whitespace in field value, case sensitivity mismatch, or wrong field path',
                        fix: 'Enable trimWhitespace option (default), enable caseInsensitive option if needed, and verify field path using field selector'
                    },
                    {
                        issue: 'Default case always executing',
                        cause: 'Field path returns null/undefined, no cases defined, or field value has unexpected format',
                        fix: 'Check field path is correct using field selector, add case definitions, and use test message to verify actual field values'
                    },
                    {
                        issue: 'Multi-step routing not executing all steps',
                        cause: 'Step IDs are incorrect, target steps are disabled, or an earlier step has a stop action',
                        fix: 'Verify step IDs match exactly (case-sensitive), check that target steps are enabled, and review step configurations for stop actions'
                    },
                    {
                        issue: 'Actions not being applied',
                        cause: 'Wrong action type selected, missing required parameters, or target field path invalid',
                        fix: 'Verify action type matches your intent, ensure all required parameters are filled (e.g., targetField for set_value), and check field paths are valid'
                    }
                ]
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('field_validation', 'pre.validation');
