/**
 * NCPDPTelecomStepsDocs — Documentation for ncpdptelecom.* pipeline steps
 * (NCPDP Telecommunication D.0: real-time pharmacy CLAIMS).
 *
 * ncpdptelecom.parse, ncpdptelecom.validate, ncpdptelecom.map_to_canonical,
 * ncpdptelecom.build.
 *
 * Self-registers into StepDocumentationRegistry, mirroring EDIStepsDocs.js.
 * Defaults, config keys and outputs are taken from each executor's source
 * (services/executors/transform/ncpdptelecom_*_executor.go).
 *
 * D.0 is a different standard from NCPDP SCRIPT (see NCPDPStepsDocs.js): a
 * control-character-delimited claim format exchanged with payers/switches.
 * Supported transaction codes: B1 (claim billing), B2 (reversal),
 * B3 (rebill), E1 (eligibility verification) — each request and response.
 */
(function () {
    const docs = {};
    const CODES = 'B1 (claim billing), B2 (reversal), B3 (rebill), E1 (eligibility)';

    docs['ncpdptelecom.parse'] = {
        description: 'Parses a raw NCPDP Telecommunication D.0 transmission (a real-time pharmacy claim or its adjudication response) into structured JSON: {transactionCode, direction, header, transmissionGroup, transactionGroups}. Request and response use the same transaction code on the wire; the direction is detected automatically from the segments present unless you set it. D.0 content received by a connector is already parsed automatically after ingestion — use this step for D.0 that appears mid-pipeline. Supported: ' + CODES + '.',
        useCases: [
            'Re-parse a D.0 claim or response fetched mid-pipeline (file, database, HTTP)',
            'Read claim fields (BIN, NDC, quantity, prices) or the response status in a conditional step',
        ],
        example: { sourceField: 'raw', outputField: 'parsedTelecom' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Field holding the raw D.0 transmission. Default: "raw".' },
            { name: 'direction', type: 'string', required: false, description: '"request" or "response". Leave empty to detect automatically; set it when you already know.' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the parsed JSON is written. Default: "parsedTelecom".' },
        ],
        bestPractices: [
            {
                practice: 'Use transactionGroups for per-claim data',
                reason: 'A transmission can carry several claims (one GS-delimited transaction group per drug/claim); patient, insurance and pharmacy appear once in transmissionGroup.',
                example: 'parsedTelecom.transactionGroups[0].Claim.productServiceId',
            },
        ],
        troubleshooting: [
            {
                issue: '"unsupported transaction" error with an unexpected code',
                cause: 'The 56-byte fixed header is shorter or misaligned (for example a truncated transmission), so the transaction code is read from the wrong bytes.',
                fix: 'Check that the header is complete (BIN, version D0, transaction code, processor control number, ...). Truncated input is rejected safely rather than misparsed.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedTelecom.transactionCode', type: 'string', description: 'e.g. "B1".' },
                { name: '_stepOutput.parsedTelecom.direction', type: 'string', description: '"request" or "response".' },
                { name: '_stepOutput.parsedTelecom.header', type: 'object', description: 'Fixed-width header: binNumber, version, processorControlNumber, serviceProviderId, dateOfService, ...' },
                { name: '_stepOutput.parsedTelecom.transmissionGroup', type: 'object', description: 'Segments sent once: Patient, Insurance, PharmacyProvider, Prescriber (request) or ResponseMessage (response).' },
                { name: '_stepOutput.parsedTelecom.transactionGroups', type: 'array', description: 'One entry per claim: Claim and Pricing (request) or ResponseStatus, ResponseClaim, ResponsePricing (response).' },
                { name: '_stepOutput.transactionGroupCount', type: 'number', description: 'Number of claims in the transmission.' },
            ],
        },
    };

    docs['ncpdptelecom.validate'] = {
        description: 'Checks a parsed D.0 transmission for missing or malformed REQUIRED fields and segments (valid=false on errors). It never stops the pipeline by itself — branch on "valid". By default it reads a prior ncpdptelecom.parse result; raw D.0 found there is parsed automatically.',
        useCases: [
            'Hold back a claim with missing required data before sending it to a payer',
            'Check an outbound B1 built by ncpdptelecom.map_to_canonical before ncpdptelecom.build',
        ],
        example: { sourceField: 'parsedTelecom', outputField: 'telecomValidation' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Prior parse/map output, or raw D.0. Default: "parsedTelecom".' },
            { name: 'direction', type: 'string', required: false, description: 'Only used when sourceField holds raw content.' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "telecomValidation".' },
        ],
        bestPractices: [
            {
                practice: 'Expect a Pricing gap when the claim is built from a FHIR MedicationDispense',
                reason: 'Ingredient cost, usual-and-customary charge and gross amount due are the pharmacy\'s own point-of-sale figures; MedicationDispense does not carry them, and inventing $0.00 would mislead a payer.',
                example: 'Add the pricing from your pharmacy system (enrichment step) before ncpdptelecom.build.',
            },
        ],
        troubleshooting: [
            {
                issue: 'A rejected/denied response reports no pricing errors',
                cause: 'By design: ResponsePricing is optional because a denied claim is not priced.',
                fix: 'No action needed.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.telecomValidation.valid', type: 'boolean', description: 'False when an error-severity issue was found.' },
                { name: '_stepOutput.telecomValidation.issues', type: 'array', description: 'Each issue: {severity, path, message}.' },
                { name: '_stepOutput.issueCount', type: 'number', description: 'Total number of issues.' },
            ],
        },
    };

    docs['ncpdptelecom.map_to_canonical'] = {
        description: 'The no-code way to BUILD a D.0 transmission from non-D.0 data (a FHIR MedicationDispense, CSV, database rows, any JSON). You map source fields onto the fixed-width header, the segments sent once per transmission, and the per-claim segments (one transaction group per row), producing the structure ncpdptelecom.build serializes. Simpler than SCRIPT mapping: D.0 segments are flat field lists with one repeating construct (claims).',
        useCases: [
            'Turn a FHIR MedicationDispense into a B1 claim for a payer (see the "FHIR MedicationDispense to NCPDP D.0 B1 Claim (Outbound)" template)',
            'Submit several claims from a batch of database rows in one transmission',
        ],
        example: {
            transactionCode: 'B1',
            direction: 'request',
            headerFields: [{ fieldKey: 'binNumber', literalValue: '610014' }, { fieldKey: 'dateOfService', sourcePath: 'whenHandedOver', transform: 'datetime_to_ncpdp_date' }],
            transmissionGroupSegments: [{ segmentKey: 'Insurance', fields: [{ fieldKey: 'cardholderId', sourcePath: 'member_id' }] }],
            transactionGroupSegments: [{ segmentKey: 'Claim', fields: [{ fieldKey: 'productServiceId', sourcePath: 'ndc' }] }],
        },
        parameters: [
            { name: 'transactionCode', type: 'string', required: false, description: 'Default: "B1". Supported: ' + CODES + '.' },
            { name: 'direction', type: 'string', required: false, description: '"request" (default) or "response".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "parsedTelecom" (what ncpdptelecom.build reads).' },
            { name: 'headerFields', type: 'array', required: false, description: 'Fixed-width header fields. transactionCode and version are filled in automatically if you do not map them.' },
            { name: 'transmissionGroupSegments', type: 'array', required: false, description: '[{segmentKey, fields}] built once per transmission (Patient, Insurance, PharmacyProvider, Prescriber).' },
            { name: 'transactionGroupRowsPath', type: 'string', required: false, description: 'Path to an array of rows, one claim each. Omit to build exactly one claim from top-level data.' },
            { name: 'transactionGroupSegments', type: 'array', required: false, description: '[{segmentKey, fields}] applied per row (Claim, Pricing).' },
        ],
        bestPractices: [
            {
                practice: 'Use the datetime_to_ncpdp_date transform for dates',
                reason: 'D.0 dates are CCYYMMDD; FHIR dates are ISO-8601.',
                example: '{ fieldKey: "dateOfService", sourcePath: "whenHandedOver", transform: "datetime_to_ncpdp_date" }',
            },
        ],
        troubleshooting: [
            {
                issue: 'The built transmission cannot be re-parsed',
                cause: 'A required header value (BIN, service provider ID) was left unmapped.',
                fix: 'Add an ncpdptelecom.validate step after this one; it names the missing fields.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedTelecom', type: 'object', description: 'Canonical {transactionCode, direction, header, transmissionGroup, transactionGroups}.' },
            ],
        },
    };

    docs['ncpdptelecom.build'] = {
        description: 'Serializes canonical D.0 JSON (from ncpdptelecom.parse or ncpdptelecom.map_to_canonical) into a complete D.0 transmission — fixed header, control-character separators, signed-overpunch amounts — ready for connector.outbound or the real-time /api/pharmacy-claim endpoint.',
        useCases: [
            'Produce the B1 claim sent to a payer or switch',
            'Build the response returned by a synchronous pharmacy-claim pipeline',
        ],
        example: { sourceField: 'parsedTelecom', transactionCode: 'B1', direction: 'request', outputField: 'ncpdpTelecom' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Default: "parsedTelecom".' },
            { name: 'transactionCode', type: 'string', required: false, description: 'Default: "B1".' },
            { name: 'direction', type: 'string', required: false, description: 'Default: "request".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "ncpdpTelecom" — point connector.outbound\'s contentField here.' },
        ],
        bestPractices: [
            {
                practice: 'Validate before building',
                reason: 'Payers reject incomplete claims; ncpdptelecom.validate names missing fields first.',
                example: 'map_to_canonical → ncpdptelecom.validate → ncpdptelecom.build',
            },
        ],
        troubleshooting: [
            {
                issue: 'Amounts look wrong at the receiver',
                cause: 'D.0 money fields use signed overpunch (the last digit is a letter); the receiver must decode it.',
                fix: 'This is the D.0 standard encoding, not corruption. Re-parse with ncpdptelecom.parse to confirm the values.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.ncpdpTelecom', type: 'string', description: 'The built D.0 transmission.' },
                { name: '_stepOutput.byteCount', type: 'number', description: 'Size in bytes.' },
            ],
        },
    };

    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
