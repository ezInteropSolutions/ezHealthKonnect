/**
 * EDIStepsDocs — Documentation for edi.* pipeline steps
 *
 * edi.parse, edi.validate, edi.map_to_canonical, edi.build.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/EDIStepBuilder.js).
 *
 * All 4 step types are phase-1 scoped to the 835 (Health Care Claim
 * Payment/Advice) transaction set — see CLAUDE.md's "EDI X12 Support" section
 * for the full engine design and named future phases (837, 270/271, AS2).
 */
(function () {
    const docs = {};

    docs['edi.parse'] = {
        description: 'Parses raw X12 EDI content into a structured JSON document map — {_format, transactionSet, envelopePresent, interchange, header, loops, trailer} — without doing any FHIR or canonical mapping. Note: EDI content received by an edi_x12_inbound connector is already parsed automatically right after ingestion (the same generic MessageParserService/ParserFactory path HL7 and CDA use) — you only need an explicit edi.parse step to re-parse EDI content that shows up mid-pipeline from somewhere other than the original connector (a DB lookup step, a mid-pipeline connector.inbound pull, etc.).',
        useCases: [
            'Re-parse raw X12 content fetched by a database or file step mid-pipeline (not the original inbound connector)',
            'Inspect parsed 835 fields in a Script or conditional step to drive routing decisions before edi.validate/edi.build run',
            'Feed edi.map_to_canonical or a custom mapping step that reads the structured loops/header JSON directly',
        ],
        example: {
            sourceField: 'raw',
            outputField: 'parsedEDI',
        },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the raw X12 EDI string. Default: "raw".' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the parsed JSON is written. Default: "parsedEDI". Special value "__root__" merges the parsed keys (_format, transactionSet, interchange, header, loops, trailer) directly into the root of the pipeline data instead of nesting them under one field.' },
        ],
        bestPractices: [
            {
                practice: 'Only add this step when something needs the parsed structure earlier than usual, or the content arrived mid-pipeline',
                reason: 'edi_x12_inbound already auto-parses every message right after ingestion — adding a redundant edi.parse step right after the connector just re-parses the same content a second time for no benefit.',
                example: 'A pipeline that just does connector.inbound (edi_x12_inbound) → edi.validate → connector.outbound never needs its own edi.parse step; the parsed JSON already exists.',
            },
        ],
        troubleshooting: [
            {
                issue: '"source field ... is empty or not a string" error',
                cause: 'sourceField doesn\'t point to where the raw X12 content actually landed on the pipeline data.',
                fix: 'Check what field the step before this one actually wrote the raw EDI text to, and set sourceField to match. This step also automatically checks the top-level "raw" field and message.raw as fallbacks.',
            },
            {
                issue: 'The "Transaction Set" dropdown in the config panel doesn\'t seem to change anything',
                cause: 'Known gap: the Form UI collects this into step.config.transactionSet, but the backend executor\'s config struct does not read that key at all — the parser always auto-detects the transaction set from the message\'s own ST segment (ST01).',
                fix: 'Not fixable from the UI today. The dropdown currently only has one real option (835) anyway, so this has no practical effect in phase 1.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedEDI', type: 'object', description: 'The full structured document: {_format, transactionSet, envelopePresent, interchange, header, loops, trailer}.' },
                { name: '_stepOutput.parsedEDI.transactionSet', type: 'string', description: 'X12 transaction set identifier, e.g. "835".' },
                { name: '_stepOutput.parsedEDI.header', type: 'object', description: '835 header segments: ST, BPR (payment method/amount), TRN (trace/check number), CUR, REF, DTM.' },
                { name: '_stepOutput.parsedEDI.loops', type: 'object', description: 'Loop-ID-keyed nested structure. 1000A (Payer) and 1000B (Payee) are single objects. loops["2000"] is an array of header-number loops, each with loops["2100"] (an array of claims), each with loops["2110"] (an array of service lines) — cardinality (single object vs. array) is decided by the X12 schema itself, not by how many actually appear in a given file.' },
                { name: '_stepOutput.parsedEDI.trailer', type: 'object', description: 'PLB (provider-level adjustments, an array) and SE (transaction set trailer).' },
                { name: '_stepOutput.fieldCount', type: 'number', description: 'Number of flat, loop-qualified fields extracted from the source content.' },
            ],
        },
    };

    docs['edi.validate'] = {
        description: 'Re-checks a raw X12 EDI message against the X12 5010 standard: malformed field VALUES (wrong type, wrong length, a fixed-value mismatch) are reported as errors and set valid=false; business-rule constraints from the spec\'s own P/C/L/R/E syntax notes (e.g. "if BPR06 is present, BPR07 is required too") are reported as warnings and never block valid. This is a deliberate product decision — the platform is meant to be flexible about real-world 835s, not rigid — so a pipeline should branch on valid via control.if_then_else/control.switch_case rather than assume validation failure stops the message. The config panel\'s "OOB Validation Rules" section lists every one of these business-rule constraints for the 835 (currently on BPR, CAS, N1, N4, and PER — the only segments with any) with a checkbox each, so you can selectively suppress a specific rule for THIS step only (the shared schema itself is never edited, and other pipelines/steps are unaffected) — useful when a trading partner\'s own real-world files routinely, legitimately violate a base-standard business-rule note you don\'t want cluttering _stepOutput.issues. On top of that, "Custom Rules" lets you ADD your own business-rule constraints the base standard doesn\'t have at all (see How Custom Rules Work below).',
        useCases: [
            'Route a message to a dead-letter/review path when a malformed 835 would otherwise break downstream mapping',
            'Suppress an OOB business-rule warning that a specific trading partner\'s files legitimately, routinely trigger, without touching the shared schema',
            'Layer your own trading-partner-specific rules (custom rules) on top of the base X12 5010 checks, for constraints the standard itself doesn\'t enforce',
        ],
        example: {
            sourceField: 'raw',
            outputField: 'ediValidation',
            disabledRules: [
                { segmentId: 'PER', type: 'P', positions: ['03', '04'] },
            ],
            customRules: [
                { segmentId: 'PER', type: 'R', positions: ['communicationNumberQualifier1', 'communicationNumberQualifier2', 'communicationNumberQualifier3'] },
            ],
        },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Pipeline field holding the raw X12 EDI string. Default: "raw" — deliberately NOT "parsedEDI", even after an edi.parse step already ran: the validator\'s business-rule checks need the original typed parse data, which doesn\'t survive being converted to plain JSON between pipeline steps, so this step re-parses from raw content itself (a cheap, side-effect-free operation, the same one edi.parse performs).' },
            { name: 'outputField', type: 'string', required: false, description: 'Where the validation result is written. Default: "ediValidation".' },
            { name: 'disabledRules', type: 'array', required: false, description: 'Selectively suppresses specific OOB (schema-defined) business-rule constraints for this step only, each {segmentId, type, positions}. positions here are the schema\'s own RAW position numbers (e.g. "06"), not element keys — you never type these yourself, the checkbox list in the config panel\'s "OOB Validation Rules" section fills this in for you when you uncheck a rule. A malformed field VALUE (an error) is never affected by this — only business-rule warnings can be disabled.' },
            { name: 'customRules', type: 'array', required: false, description: 'Optional trading-partner-specific rules layered on top of the base X12 5010 checks, each {segmentId, type, positions}. type is one of the 5 X12 syntax-rule kinds (Paired, Conditional, List Conditional, Required, Exclusion — the config panel shows a plain-English description for each). positions are element KEYS from that segment\'s own schema, picked from a live dropdown — never raw numeric positions. Scoped to a segment\'s plain elements only in phase 1, not intra-segment repeat groups like CAS\'s reason/amount/quantity trios.' },
        ],
        bestPractices: [
            {
                practice: 'Branch on _stepOutput.valid, not on the presence of any issues at all',
                reason: 'valid is false only when at least one ERROR-severity issue exists. Warnings (business-rule mismatches) are expected on real-world 835s and never affect valid — treating any non-empty issues array as a failure will misroute otherwise-good messages.',
                example: 'control.if_then_else on _stepOutput.valid === false → route to a review queue; otherwise continue the normal pipeline.',
            },
            {
                practice: 'Prefer unchecking an OOB rule over reaching for a custom rule when the constraint already exists',
                reason: 'disabledRules and customRules solve opposite problems: disabledRules REMOVES a base-standard check you don\'t want; customRules ADDS a check the base standard doesn\'t have. Adding a "custom rule" that duplicates an OOB one just produces two warnings for the same violation instead of one.',
                example: 'To stop enforcing PER\'s existing paired-contact-fields rule, uncheck it under OOB Validation Rules — don\'t add a matching custom rule and leave the OOB one checked too.',
            },
        ],
        examples: [
            {
                label: 'Disable an OOB rule your trading partner\'s files always trip',
                description: 'A real, unedited sample from one payer legitimately has PER03 (a phone-number qualifier) populated with PER04 (the phone number itself) blank — violating PER\'s own OOB Paired rule (positions 03/04) every single time. Rather than accumulate that same warning on every message from this trading partner, uncheck it for this pipeline\'s edi.validate step only.',
                config: {
                    sourceField: 'raw',
                    outputField: 'ediValidation',
                    disabledRules: [
                        { segmentId: 'PER', type: 'P', positions: ['03', '04'] },
                    ],
                },
            },
            {
                label: 'Custom rule — require at least one contact method on PER',
                description: 'The base X12 standard only pairs each PER contact-number-qualifier with its own number (e.g. if 03 is present, 04 must be too) — it never REQUIRES that any contact method be given at all. A custom Required ("R" — at least one) rule over all three qualifier positions closes that gap: if none of the three qualifiers (phone/fax/email, whichever the segment supports) are populated, this fires a warning.',
                config: {
                    sourceField: 'raw',
                    outputField: 'ediValidation',
                    customRules: [
                        { segmentId: 'PER', type: 'R', positions: ['communicationNumberQualifier1', 'communicationNumberQualifier2', 'communicationNumberQualifier3'] },
                    ],
                },
            },
        ],
        troubleshooting: [
            {
                issue: 'How Custom Rules actually work, step by step',
                cause: 'Not an error — an explanation, since the mechanism isn\'t obvious from the config panel alone.',
                fix: '(1) Click "+ Add Custom Rule". (2) Pick a Segment from the live dropdown (fed by the real schema, e.g. NM1, REF, PER). (3) Pick a Rule Type — Paired (all-or-nothing), Conditional (first field present → rest required), List Conditional (any of the rest present → first required), Required (at least one), or Exclusion (at most one). (4) Check which Elements the rule applies to, by their real, human-readable key — never a raw numeric position. Under the hood: edi.validate translates your chosen element keys into their real numeric positions, clones ONLY that one segment\'s schema definition (the shared spec every other pipeline uses is never touched), appends your rule tagged source="custom", then parses and validates as normal. A violation lands in _stepOutput.issues exactly like an OOB rule, except its source field reads "custom" instead of being empty — so you can tell which rules fired where. Rules check ELEMENT PRESENCE only, never a specific VALUE — you can require "REF02 must be present whenever REF01 is," but not "...only when REF01 equals a specific code."',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.valid', type: 'boolean', description: 'false only when at least one ERROR-severity issue exists — warnings never affect this.' },
                { name: '_stepOutput.issues', type: 'array', description: 'Every finding: {severity, path, message, source}. source is "custom" for a user-supplied rule, empty for a base X12 5010 spec rule. A rule listed in disabledRules never produces an issue at all.' },
                { name: '_stepOutput.errorCount', type: 'number', description: 'Count of ERROR-severity issues (malformed field values).' },
                { name: '_stepOutput.warningCount', type: 'number', description: 'Count of WARNING-severity issues (business-rule / syntax-note mismatches, never blocking).' },
            ],
        },
    };

    docs['edi.map_to_canonical'] = {
        description: 'The no-code, format-agnostic on-ramp for building an 835 from data that never went through edi.parse at all — maps CSV columns, database query columns, or arbitrary upstream JSON fields onto the same canonical header/loops JSON edi.parse itself produces, so edi.build can serialize a complete interchange from any source system with zero new code, only step configuration. Not a reuse of cda.map_to_canonical — X12\'s loop-ID-keyed shape genuinely nests (835 alone needs 2000 → 2100 → 2110, three levels deep), which needed its own recursive mapper.',
        useCases: [
            'Generate an outbound 835 directly from a payer\'s claims-adjudication database, with no upstream X12 content at all',
            'Reshape a CSV export of claim payment rows into the exact canonical JSON edi.build expects',
            'Combine with edi.build to round-trip test the X12 engine from synthetic/manually-authored data',
        ],
        example: {
            outputField: 'canonicalEDI',
            transactionSet: '835',
            header: [
                { segmentId: 'BPR', elementKey: 'totalActualProviderPaymentAmount', sourcePath: 'payment.total_amount' },
            ],
            loops: [
                { loopId: '2000', loops: [
                    { loopId: '2100', rowsPath: 'claims', fields: [
                        { segmentId: 'CLP', elementKey: 'claimPaymentAmount', sourcePath: 'paid_amount' },
                    ] },
                ] },
            ],
        },
        parameters: [
            { name: 'outputField', type: 'string', required: false, description: 'Where the canonical JSON is written. Default: "canonicalEDI".' },
            { name: 'transactionSet', type: 'string', required: false, description: 'Which transaction set\'s loop tree to consult for cardinality decisions. Default: "835".' },
            { name: 'header', type: 'array', required: false, description: 'Flat, single-instance header segment mappings: [{segmentId, elementKey, sourcePath, transform?, literalValue?}], for ST/BPR/TRN/CUR/REF/DTM.' },
            { name: 'loops', type: 'array', required: false, description: 'Recursive loop mappings: [{loopId, rowsPath?, fields: [...same shape as header...], loops?: [...nested, same shape...]}]. rowsPath selects which row(s) to map — for a nested loop, resolved RELATIVE TO the parent loop\'s current row, not the whole pipeline data. Omitting rowsPath maps from the current single row.' },
        ],
        bestPractices: [
            {
                practice: 'Don\'t worry about single-row vs. array shape — map one row and let the schema decide',
                reason: 'The step consults the real X12 loop tree at execution time and always wraps a schema-repeating loop\'s (e.g. 2100, 2110) output as an array — even when you only mapped exactly one row — so edi.build never silently drops a single-claim 835. You never need to manually force array vs. object shape.',
                example: 'Map exactly one claim under loops[loopId=2100].rowsPath; the executor still writes loops["2100"] as a 1-element array, matching what edi.build requires.',
            },
        ],
        troubleshooting: [
            {
                issue: 'Trailer (PLB provider-level adjustments) never appears in the built output',
                cause: 'Deliberately out of scope for this step in phase 1 — PLB is a bare repeating segment, not a loop, so it doesn\'t fit this step\'s recursive loop-mapping UI.',
                fix: 'edi.build still accepts a hand-supplied "trailer" key in its own source data if you need PLB rows — it\'s just not produced by this mapping step.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.canonicalEDI', type: 'object', description: 'The mapped canonical {header, loops} JSON — the same shape edi.build\'s own sourceField expects.' },
            ],
        },
    };

    docs['edi.build'] = {
        description: 'Builds a complete ISA...IEA X12 interchange from canonical JSON — the same header/loops shape edi.parse\'s own output and edi.map_to_canonical\'s own output both produce, so a parse → build or map_to_canonical → build chain needs no reshaping. The write-direction mirror of edi.parse, via the same schema-driven engine (a new transaction set is a schema file, not new Go code).',
        useCases: [
            'Deliver a built 835 to a payer\'s SFTP endpoint via connector.outbound (edi_x12_outbound)',
            'Round-trip test the X12 engine: edi.parse → (modify fields) → edi.build → edi.validate',
            'Build an outbound 835 entirely from edi.map_to_canonical\'s output, with no upstream X12 content at all',
        ],
        example: {
            sourceField: 'parsedEDI',
            transactionSet: '835',
            outputField: 'ediX12',
            isaSenderId: '',
            isaReceiverId: '',
        },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Dot-path to the canonical source data (interchange/header/loops/trailer shape). Default: "parsedEDI".' },
            { name: 'transactionSet', type: 'string', required: false, description: 'Which transaction set to build. Default: "835" — the only one implemented end-to-end in phase 1.' },
            { name: 'outputField', type: 'string', required: false, description: 'Dot-path to write the built EDI text to. Default: "ediX12".' },
            { name: 'isaSenderId', type: 'string', required: false, description: 'ISA06 — your deployment\'s trading-partner sender identifier.' },
            { name: 'isaReceiverId', type: 'string', required: false, description: 'ISA08 — the receiving trading partner\'s identifier.' },
            { name: 'gsSenderCode', type: 'string', required: false, description: 'GS02. Defaults to isaSenderId when left blank — the common real-world convention, though some trading partners do use a distinct GS-level code.' },
            { name: 'gsReceiverCode', type: 'string', required: false, description: 'GS03. Defaults to isaReceiverId when left blank, same convention as gsSenderCode.' },
        ],
        bestPractices: [
            {
                practice: 'Feed the built output straight into connector.outbound\'s contentField — no intermediate step needed',
                reason: 'edi_x12_outbound is a transport-only "dumb byte shipper" — all X12 envelope/business logic lives in this step, not the connector, matching every other connector in this codebase.',
                example: 'edi.build (outputField: "ediX12") → connector.outbound (connectorType: "edi_x12_outbound", contentField: "ediX12").',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.ediX12', type: 'string', description: 'The complete built ISA...IEA X12 interchange text, ready to send as-is.' },
            ],
        },
    };

    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
