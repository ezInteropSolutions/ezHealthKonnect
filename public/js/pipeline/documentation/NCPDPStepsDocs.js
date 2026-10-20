/**
 * NCPDPStepsDocs — Documentation for ncpdp.* pipeline steps (NCPDP SCRIPT
 * pharmacy e-prescribing, XML).
 *
 * ncpdp.parse, ncpdp.validate, ncpdp.map_to_canonical, ncpdp.build.
 *
 * Self-registers into StepDocumentationRegistry at load time (loaded via
 * <script> after StepDocumentationRegistry.js), mirroring EDIStepsDocs.js.
 * Content is taken from each executor's own source
 * (services/executors/transform/ncpdp_*_executor.go) — defaults, config keys
 * and step output names are the real ones.
 *
 * Supported SCRIPT 2017071 transactions (ncpdp/schemas/script_2017071):
 * NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse,
 * RxRenewalRequest, RxRenewalResponse, RxFill, Status, Error, Verify, GetMessage.
 * NCPDP Telecommunication D.0 (pharmacy CLAIMS) is a different standard — see
 * NCPDPTelecomStepsDocs.js.
 */
(function () {
    const docs = {};
    const SCRIPT_TRANSACTIONS = 'NewRx, CancelRx, CancelRxResponse, RxChangeRequest, RxChangeResponse, RxRenewalRequest, RxRenewalResponse, RxFill, Status, Error, Verify, GetMessage';

    docs['ncpdp.parse'] = {
        description: 'Parses a raw NCPDP SCRIPT (e-prescribing) XML message into structured JSON — {transactionType, messageAttrs, header, body} — without any FHIR mapping. NCPDP SCRIPT content arriving through a connector is already parsed automatically right after ingestion (the platform detects TransactionDomain="SCRIPT"), so you only need this step to re-parse SCRIPT XML that appears mid-pipeline (from a database lookup, a file read, an HTTP response, etc.). The transaction type is detected from the message itself. Supported: ' + SCRIPT_TRANSACTIONS + '.',
        useCases: [
            'Re-parse SCRIPT XML fetched mid-pipeline by a database, file or HTTP step',
            'Read prescription fields (patient, prescriber, pharmacy, medication) in a Script or conditional step to route NewRx vs CancelRx vs RxRenewalRequest',
            'Feed ncpdp.validate or a FHIR mapping (fhir.build) that reads the parsed header/body',
        ],
        example: { sourceField: 'raw', outputField: 'parsedNCPDP' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the raw SCRIPT XML. Default: "raw" (message.raw is also checked as a fallback).' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the parsed JSON is written. Default: "parsedNCPDP".' },
        ],
        bestPractices: [
            {
                practice: 'Address parsed fields from LATER steps using the snake_case form',
                reason: 'When a later step reads this step\'s output through steps.<step_name>.step_output, every key is snake_cased (medicationPrescribed becomes medication_prescribed, humanPatient becomes human_patient).',
                example: 'steps.parse_new_rx.step_output.parsed_ncpdp.body.medication_prescribed.drug_description',
            },
        ],
        troubleshooting: [
            {
                issue: 'A later step\'s sourcePath resolves to nothing even though parsing succeeded',
                cause: 'The path uses the camelCase key names, or the step-name key does not match the parse step\'s real name.',
                fix: 'Use snake_case keys, and build the steps.<key> part from the parse step\'s NAME (lowercase, spaces to underscores) — "Parse New Rx" is steps.parse_new_rx.',
            },
            {
                issue: '"unsupported transaction" error',
                cause: 'The message body\'s transaction element is not one of the supported SCRIPT transactions.',
                fix: 'Check the <Body> element of the message. Supported: ' + SCRIPT_TRANSACTIONS + '.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedNCPDP.transactionType', type: 'string', description: 'Detected transaction, e.g. "NewRx".' },
                { name: '_stepOutput.parsedNCPDP.header', type: 'object', description: 'Message header: to/from mailbox IDs, messageID, sentTime, ...' },
                { name: '_stepOutput.parsedNCPDP.body', type: 'object', description: 'Transaction body: patient, pharmacy, prescriber, medicationPrescribed, response outcome groups (Approved/Denied/...), depending on the transaction.' },
                { name: '_stepOutput.fieldCount', type: 'number', description: 'Number of fields extracted.' },
            ],
        },
    };

    docs['ncpdp.validate'] = {
        description: 'Checks a parsed NCPDP SCRIPT message for missing or empty REQUIRED fields and groups. A missing required item is an error (valid=false); validation never stops the pipeline by itself — branch on "valid" with control.if_then_else if invalid messages should take a different path. By default it reads the output of a prior ncpdp.parse step; if it finds raw XML there instead, it parses it itself.',
        useCases: [
            'Send prescriptions missing required data to a review/DLQ path before mapping them to FHIR',
            'Check an outbound NewRx built by ncpdp.map_to_canonical before ncpdp.build serializes it',
        ],
        example: { sourceField: 'parsedNCPDP', outputField: 'ncpdpValidation' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Prior ncpdp.parse (or ncpdp.map_to_canonical) output, or raw SCRIPT XML. Default: "parsedNCPDP".' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the result is written. Default: "ncpdpValidation".' },
        ],
        bestPractices: [
            {
                practice: 'Place it between ncpdp.map_to_canonical and ncpdp.build in outbound pipelines',
                reason: 'It reports exactly which required SCRIPT fields your mapping did not populate, before a malformed message is sent to a pharmacy.',
                example: 'map_to_canonical → ncpdp.validate → ncpdp.build → connector.outbound',
            },
        ],
        troubleshooting: [
            {
                issue: 'A CancelRx reports no errors although it has no quantity or sig',
                cause: 'By design: those fields are optional because a cancellation only needs to identify the original prescription.',
                fix: 'No action needed. NewRx messages in real use still carry them.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.ncpdpValidation.valid', type: 'boolean', description: 'False when at least one error-severity issue was found.' },
                { name: '_stepOutput.ncpdpValidation.issues', type: 'array', description: 'Each issue: {severity, path, message}.' },
                { name: '_stepOutput.issueCount', type: 'number', description: 'Total number of issues.' },
            ],
        },
    };

    docs['ncpdp.map_to_canonical'] = {
        description: 'The no-code way to BUILD an NCPDP SCRIPT message from data that is not SCRIPT already — a FHIR MedicationRequest, CSV rows, database columns, or any JSON. You map source fields onto the SCRIPT header and body (patient, prescriber, pharmacy, medicationPrescribed, ... as a nested group tree), producing the same structure ncpdp.parse outputs, which ncpdp.build then serializes to XML. No code is needed — only step configuration.',
        useCases: [
            'Turn a FHIR MedicationRequest from an EHR into a NewRx for the pharmacy (see the "FHIR MedicationRequest to NCPDP SCRIPT NewRx (Outbound)" template)',
            'Build CancelRx/RxRenewalResponse messages from a database row',
        ],
        example: {
            transactionType: 'NewRx',
            outputField: 'parsedNCPDP',
            headerFields: [{ fieldKey: 'to', literalValue: 'PHARMACY-MAILBOX-ID' }, { fieldKey: 'messageID', sourcePath: 'id' }],
            bodyGroups: [{ groupKey: 'patient', groups: [{ groupKey: 'humanPatient', fields: [{ fieldKey: 'lastName', sourcePath: 'patient_last_name' }] }] }],
        },
        parameters: [
            { name: 'transactionType', type: 'string', required: false, description: 'Which SCRIPT transaction\'s field tree to build. Default: "NewRx".' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the canonical JSON is written. Default: "parsedNCPDP" (what ncpdp.build reads by default).' },
            { name: 'headerFields', type: 'array', required: false, description: 'Header fields: [{fieldKey, sourcePath | literalValue, transform?}].' },
            { name: 'headerGroups', type: 'array', required: false, description: 'Nested header groups: [{groupKey, rowsPath?, fields, groups?}].' },
            { name: 'bodyFields', type: 'array', required: false, description: 'Fields directly on the transaction body.' },
            { name: 'bodyGroups', type: 'array', required: false, description: 'Nested body groups (patient, pharmacy, prescriber, medicationPrescribed, ...), nestable to any depth. rowsPath maps a repeatable group from an array of rows.' },
        ],
        bestPractices: [
            {
                practice: 'Use literalValue for trading-partner constants FHIR cannot carry',
                reason: 'Mailbox IDs (to/from) and similar enrollment values do not exist on a MedicationRequest.',
                example: '{ fieldKey: "from", literalValue: "YOUR-PRESCRIBER-MAILBOX" }',
            },
            {
                practice: 'Put a derive (enrichment.script) step before this one when names or IDs need reshaping',
                reason: 'Mapping only copies/transforms single values; splitting "Doe, Jane" into last/first is easier in a small script.',
                example: 'enrichment.script → ncpdp.map_to_canonical → ncpdp.validate → ncpdp.build',
            },
        ],
        troubleshooting: [
            {
                issue: 'A mapped field is missing from the built XML',
                cause: 'The sourcePath does not resolve on the step input, or the field sits in the wrong group.',
                fix: 'Run Test Pipeline and inspect this step\'s output; check the group path against the transaction\'s field tree shown in the config panel.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedNCPDP', type: 'object', description: 'Canonical {transactionType, header, body} — the input ncpdp.build expects.' },
            ],
        },
    };

    docs['ncpdp.build'] = {
        description: 'Serializes canonical SCRIPT JSON (from ncpdp.parse or ncpdp.map_to_canonical) into a complete NCPDP SCRIPT XML message, ready for a connector.outbound step. A parse → build round trip needs no reshaping.',
        useCases: [
            'Produce the NewRx XML sent to a pharmacy after ncpdp.map_to_canonical',
            'Re-emit a modified SCRIPT message after editing fields in a Script step',
        ],
        example: { sourceField: 'parsedNCPDP', transactionType: 'NewRx', outputField: 'ncpdpScript' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Canonical source JSON. Default: "parsedNCPDP".' },
            { name: 'transactionType', type: 'string', required: false, description: 'Transaction to build. Default: "NewRx". Supported: ' + SCRIPT_TRANSACTIONS + '.' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the XML text is written. Default: "ncpdpScript" — point connector.outbound\'s contentField here.' },
        ],
        bestPractices: [
            {
                practice: 'Validate before building',
                reason: 'ncpdp.validate names every missing required field; ncpdp.build only serializes what it is given.',
                example: 'ncpdp.validate → ncpdp.build',
            },
        ],
        troubleshooting: [
            {
                issue: 'connector.outbound sends an empty body',
                cause: 'contentField does not point at this step\'s outputField.',
                fix: 'Set the outbound step\'s contentField to "ncpdpScript" (or your custom outputField).',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.ncpdpScript', type: 'string', description: 'The built SCRIPT XML.' },
                { name: '_stepOutput.byteCount', type: 'number', description: 'Size of the built message in bytes.' },
            ],
        },
    };

    for (const [type, doc] of Object.entries(docs)) {
        StepDocumentationRegistry.register(type, doc);
    }
})();
