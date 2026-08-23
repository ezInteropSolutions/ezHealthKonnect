/**
 * PreProcessingStepsDocs — Documentation for validation/branching pre-processing steps
 *
 * pre.validation, if_then_else (alias: pre.logic), pre.logic.switch (alias: switch_case),
 * plus the legacy field_validation alias.
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
            'if_then_else': {
                description: 'Conditional branching step (services/executors/control/conditional_executor.go, "If-Then-Else" v2.0.0). Evaluates one or more independent {field, operator, value} conditions in order and runs the matching onTrue/onFalse action(s) for each. Uses the same shared operator evaluator as Switch/Case and hl7.build\'s per-segment/field conditions (services/executors/condition.go) — 12 operators, cross-field comparisons via compareToField, not just literal-value matching. If ANY condition evaluates true, the step\'s overall branch is "true" and the pipeline auto-skips the false branch\'s steps (and vice versa) via _routing.branchTaken — no manual skip_steps needed for a simple two-branch flow. See pre.logic.switch\'s own documentation for a side-by-side comparison of when to use which.',
                useCases: [
                    'Flag a patient as high-risk when age > 65 AND a chronic-condition field is set — chained conditions a single-field Switch/Case can\'t express',
                    'Reject a message outright (action: "reject") when a required cross-field relationship is violated, e.g. discharge date before admission date',
                    'Route to a specific downstream step only when a field is missing (operator: "not_exists") without needing a Switch/Case default case',
                    'Compare two fields against each other (compareToField) rather than a field against a literal — e.g. flag when PV1.44 (admission) is later than PV1.45 (discharge)',
                    'Tag metadata (set_metadata) for later steps to read, without altering the message body itself',
                ],
                example: {
                    conditions: [
                        {
                            name: 'High-risk age check',
                            condition: { field: 'message.patientAge', operator: 'greater_than', value: '65' },
                            onTrue: { action: 'set_field', field: 'message.riskFlag', value: 'high' },
                            onFalse: { action: 'continue' },
                        },
                    ],
                },
                parameters: [
                    { name: 'conditions', type: 'array', required: true, description: 'One or more independent condition blocks, evaluated in order: {name, condition, onTrue, onFalse}. Every condition in the array is evaluated and its matching branch\'s actions run — this is not a first-match-wins list like Switch/Case\'s cases.' },
                    { name: 'conditions[].condition', type: 'object', required: true, description: '{field, operator, value, compareToField}. field is the path to evaluate; operator is one of equals, not_equals, contains, starts_with, ends_with, greater_than, greater_than_or_equal, less_than, less_than_or_equal, exists, not_exists, regex_match, in_list (services/executors/condition.go — the same evaluator Switch/Case and hl7.build\'s conditions use). value is the literal to compare against (unused/optional for exists/not_exists). compareToField, when set, compares against another field\'s current value instead of the literal value.' },
                    { name: 'conditions[].onTrue / conditions[].onFalse', type: 'object | array', required: false, description: 'The action(s) to run for that branch. Accepts either a single action object ({action: "..."}) or an array of action objects — both are supported; use an array when a branch needs more than one action.' },
                ],
                actions: [
                    { action: 'continue', description: 'No-op — just continue.', usedFor: 'The branch that needs no side effect.', parameters: 'None' },
                    { action: 'reject', description: 'Fails the message with an error, stopping the pipeline for it.', usedFor: 'Hard validation failures that must NACK/stop rather than continue.', parameters: 'errorMessage, severity (default "error")' },
                    { action: 'log_warning', description: 'Logs a warning.', usedFor: 'Non-fatal data-quality notices.', parameters: 'message, continue (bool, default true — set false to stop processing after the warning)' },
                    { action: 'log_error', description: 'Logs an error.', usedFor: 'Serious but sometimes-recoverable issues.', parameters: 'message, continue (bool, default FALSE — unlike log_warning, this stops processing by default unless continue:true is set explicitly)' },
                    { action: 'set_metadata', description: 'Merges keys into the message\'s _metadata object.', usedFor: 'Tagging for later steps/reporting without touching the message body.', parameters: 'metadata (object of key/value pairs to merge)' },
                    { action: 'route_to', description: 'Sets a routing destination/queue and/or a single conditional next step.', usedFor: 'Lower-level routing distinct from route_to_step — sets _routing.destination/.queue directly.', parameters: 'destination, queue, nextStep (or stepId)' },
                    { action: 'set_field (alias: set_value)', description: 'Sets a field value on the message.', usedFor: 'Writing a computed/flag value onto the message for downstream steps.', parameters: 'field (target path), value — NOT targetField; see the troubleshooting entry below, this differs from Switch/Case\'s own set_value action.' },
                    { action: 'copy_field', description: 'Copies one field\'s value to another.', usedFor: 'Duplicating a value onto a normalized field name.', parameters: 'source, target — NOT sourceField/targetField; see the troubleshooting entry below, this differs from Switch/Case\'s own copy_field action.' },
                    { action: 'delete_field', description: 'Removes a field from the message.', usedFor: 'Stripping a field once it\'s no longer needed downstream.', parameters: 'field' },
                    { action: 'route_to_step', description: 'Routes to one or more specific steps by ID, same multi-step mechanism Switch/Case uses.', usedFor: 'Branching to different processing paths.', parameters: 'stepId (or targetStepId) for a single step, OR targetStepIds (array) for multiple in order; optional skipSteps (array) to force the OTHER branch\'s steps to be skipped even if they weren\'t auto-excluded — see the exclusive-branch note in Best Practices.' },
                    { action: 'log_info / log_debug', description: 'Logs an informational/debug message.', usedFor: 'Tracing/diagnostics.', parameters: 'message' },
                ],
                bestPractices: [
                    {
                        practice: 'Every condition in the array is evaluated — this is not first-match-wins',
                        reason: 'Unlike Switch/Case\'s cases (first match wins), all of if_then_else\'s conditions run and each one\'s matching branch executes. Two conditions that are both true both fire their onTrue actions.',
                        example: 'Two independent checks (age > 65, AND separately has-chronic-condition) — both conditions run and both can tag metadata, even though neither one "wins" over the other.',
                    },
                    {
                        practice: 'Only the LAST routing action across all conditions actually takes effect',
                        reason: 'route_to_step/route_to actions are collected and executed once, after every condition has run — if more than one condition\'s branch calls a routing action, only the final one encountered is applied; earlier ones are silently overwritten, not combined.',
                        example: 'If both Condition 1\'s onTrue and Condition 2\'s onFalse call route_to_step, only Condition 2\'s routing is the one that actually executes.',
                    },
                    {
                        practice: 'Let branchTaken handle simple two-branch exclusion; use skipSteps only for more complex cases',
                        reason: '_routing.branchTaken ("true"/"false") is set automatically from whether ANY condition matched, and the pipeline already uses it to skip the other branch\'s steps for a straightforward if/else — route_to_step\'s own skipSteps parameter exists for cases branchTaken\'s simple true/false split can\'t express (e.g. multiple independent conditions with different step sets to exclude).',
                        example: 'A single condition with a true-path step and a false-path step needs no skipSteps at all — branchTaken already excludes the untaken path.',
                    },
                ],
                troubleshooting: [
                    {
                        issue: 'set_value/set_field action runs without error but the field never actually updates',
                        cause: 'If-Then-Else\'s set_field/set_value action reads its target from the "field" key — a config written for (or copied from) a Switch/Case case, which uses "targetField" for the same action name, is silently ignored here since "targetField" isn\'t a key this action reads.',
                        fix: 'Use {"action": "set_field", "field": "message.yourField", "value": "..."} — "field", not "targetField".',
                    },
                    {
                        issue: 'copy_field silently copies nothing',
                        cause: 'Same class of gotcha as set_field — this action reads "source"/"target", not Switch/Case\'s "sourceField"/"targetField".',
                        fix: 'Use {"action": "copy_field", "source": "message.a", "target": "message.b"}.',
                    },
                    {
                        issue: 'Only one of two conditions\' routing actions seems to run',
                        cause: 'Not a bug — see the Best Practices entry above: only the last routing action across ALL conditions in the array is executed, by design (routing is deferred and applied once, after every condition has been evaluated).',
                        fix: 'Keep at most one condition emitting a routing action, or order conditions so the one you want to win is evaluated last.',
                    },
                    {
                        issue: '"condition is required" error even though conditions look correctly configured',
                        cause: 'The executor accepts three config shapes for backward compatibility (newest: conditions[] array; newer: ifThenElse.conditions; legacy: a single top-level condition/then_actions/else_actions) — a config with the array present but each entry missing its own nested "condition" object still fails, since each array entry needs {name, condition, onTrue, onFalse}, not a flat condition at the array level.',
                        fix: 'Confirm each entry in conditions[] has its own nested condition object: {name: "...", condition: {field, operator, value}, onTrue: {...}, onFalse: {...}}.',
                    },
                ],
                stepOutput: {
                    description: 'Available to every step after this one.',
                    fields: [
                        { name: '_executionDetails.conditions_processed', type: 'number', description: 'How many condition blocks were evaluated.' },
                        { name: '_executionDetails.conditions_results', type: 'array', description: 'Per-condition {name, evaluated, branch, actions_executed} breakdown.' },
                        { name: '_executionDetails.branch_taken', type: 'string', description: '"true" if ANY condition matched, else "false" — mirrors _routing.branchTaken.' },
                        { name: '_routing.branchTaken', type: 'string', description: '"true" or "false" — read by the pipeline service to auto-skip the untaken branch\'s steps.' },
                    ],
                },
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('field_validation', 'pre.validation');
StepDocumentationRegistry.registerAlias('pre.logic', 'if_then_else');
StepDocumentationRegistry.registerAlias('switch_case', 'pre.logic.switch');
