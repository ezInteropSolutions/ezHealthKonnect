/**
 * DeviceStepsDocs — Documentation for device-integration pipeline steps:
 * astm.parse, astm.validate, astm.build (lab analyzers, ASTM E1394-97) and
 * dicom.parse (imaging, DICOM Part 10).
 *
 * Self-registers into StepDocumentationRegistry, mirroring EDIStepsDocs.js.
 * Defaults, config keys and outputs are taken from each executor's source
 * (services/executors/transform/{astm,dicom}_*_executor.go).
 *
 * HL7-speaking devices (ventilators, monitors) use the normal HL7 steps — the
 * "HL7 over TCP/IP" device template receives them over MLLP. ASTM devices
 * connect over TCP (astm_tcp_inbound) or RS232 (serial_inbound); DICOM
 * modalities send to the DICOM Storage SCP (dicom_storage_inbound).
 */
(function () {
    const docs = {};

    docs['astm.parse'] = {
        description: 'Parses an ASTM E1394-97 message from a lab analyzer into structured JSON: {header, headerComments, patientBlocks, queryBlocks, trailer}. Each patient block nests that patient\'s orders (O) and results (R). The connector removes the ENQ/STX/ETX/EOT framing and checksums first, and ASTM received by an ASTM connector (TCP or serial) is already parsed automatically after ingestion — use this step only for ASTM content that appears mid-pipeline.',
        useCases: [
            'Re-parse ASTM records fetched from a file or database mid-pipeline',
            'Read result values (R records) in a Script or conditional step, e.g. to flag abnormal results',
        ],
        example: { sourceField: 'raw', outputField: 'parsedASTM' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Field holding the destuffed ASTM record stream. Default: "raw".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "parsedASTM".' },
        ],
        bestPractices: [
            {
                practice: 'Do not add it right after an ASTM connector',
                reason: 'The message is already parsed automatically after ingestion; a second parse only repeats the work.',
                example: 'connector.inbound (astm_tcp_inbound) → astm.validate → mapping steps',
            },
        ],
        troubleshooting: [
            {
                issue: 'Parse error about missing header (H) or terminator (L)',
                cause: 'The record stream is incomplete — usually a dropped connection mid-transfer, or content that still contains transport framing.',
                fix: 'Check the analyzer link (cabling/baud rate for serial) and that the content comes from an ASTM connector, which removes framing.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedASTM.header', type: 'object', description: 'H record: sender (analyzer) identification, delimiters, timestamp.' },
                { name: '_stepOutput.parsedASTM.patientBlocks', type: 'array', description: 'One per P record, each with its orders (O) and their results (R) and comments (C).' },
                { name: '_stepOutput.parsedASTM.queryBlocks', type: 'array', description: 'Q records — host queries from the analyzer asking for pending orders.' },
                { name: '_stepOutput.parsedASTM.trailer', type: 'object', description: 'L (terminator) record.' },
                { name: '_stepOutput.patientBlockCount', type: 'number', description: 'Number of patients in the message.' },
            ],
        },
    };

    docs['astm.validate'] = {
        description: 'Re-checks every field value of a parsed ASTM message against its declared data type. A malformed value is an error (valid=false); missing optional fields are never errors. Record structure (an H first, an L last) is already enforced when parsing. Transport checksums are verified by the connector, not here. It never stops the pipeline by itself — branch on "valid".',
        useCases: [
            'Route results with malformed values (for example a non-numeric result) to review before they reach the LIS/EHR',
        ],
        example: { sourceField: 'parsedASTM', outputField: 'astmValidation' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Prior astm.parse output, or raw ASTM. Default: "parsedASTM".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "astmValidation".' },
        ],
        bestPractices: [
            {
                practice: 'Branch on valid with control.if_then_else',
                reason: 'Validation reports issues but does not stop processing.',
                example: 'astm.validate → if astmValidation.valid → mapping, else → DLQ/review',
            },
        ],
        troubleshooting: [],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.astmValidation.valid', type: 'boolean', description: 'False when an error-severity issue was found.' },
                { name: '_stepOutput.astmValidation.issues', type: 'array', description: 'Each issue: {severity, path, message}.' },
                { name: '_stepOutput.issueCount', type: 'number', description: 'Total number of issues.' },
            ],
        },
    };

    docs['astm.build'] = {
        description: 'Builds an ASTM E1394-97 record stream (records separated by CR) from canonical JSON — the same {header, patientBlocks, queryBlocks, trailer} shape astm.parse produces. Used to send orders or host-query replies back to an analyzer. Transport framing (ENQ/STX/checksum/EOT) is added by the ASTM connector when sending, not by this step.',
        useCases: [
            'Answer an analyzer\'s host query (Q record) with the pending orders for that sample',
            'Download orders to a bidirectional analyzer',
        ],
        example: { sourceField: 'parsedASTM', outputField: 'astmMessage' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Default: "parsedASTM".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "astmMessage" — the ASTM outbound connector sends this field.' },
        ],
        bestPractices: [],
        troubleshooting: [
            {
                issue: 'The analyzer does not accept the message',
                cause: 'Most analyzers expect a specific subset of fields per record and their own delimiters from the H record.',
                fix: 'Compare the built H/P/O records with the analyzer\'s interface specification; set the delimiters in the header to match.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.astmMessage', type: 'string', description: 'The built record stream.' },
                { name: '_stepOutput.byteCount', type: 'number', description: 'Size in bytes.' },
            ],
        },
    };

    docs['dicom.parse'] = {
        description: 'Extracts patient, study, series and instance metadata from a DICOM Part 10 file — patientId, patientName, studyInstanceUID, seriesInstanceUID, sopInstanceUID, sopClassUID, modality, accessionNumber, manufacturer, model, institution, station. The image pixel data is never copied into pipeline JSON (it stays in the stored original). Images received by the DICOM Storage SCP connector are already parsed automatically — use this step for DICOM content obtained mid-pipeline.',
        useCases: [
            'Build a FHIR ImagingStudy or notify an EHR when a modality stores a new image',
            'Route images by modality (CT, MR, DX) or accession number',
        ],
        example: { sourceField: 'raw', outputField: 'parsedDICOM' },
        parameters: [
            { name: 'sourceField', type: 'string', required: false, description: 'Field holding the raw DICOM Part 10 bytes. Default: "raw".' },
            { name: 'outputField', type: 'string', required: false, description: 'Default: "parsedDICOM".' },
        ],
        bestPractices: [
            {
                practice: 'Correlate with the order using accessionNumber',
                reason: 'The accession number links the image to the radiology order in the RIS/EHR.',
                example: 'parsedDICOM.accessionNumber',
            },
        ],
        troubleshooting: [
            {
                issue: 'Some metadata fields are empty',
                cause: 'The modality did not populate those tags; DICOM leaves many of them optional.',
                fix: 'Check the device\'s DICOM conformance statement for which tags it sends.',
            },
        ],
        stepOutput: {
            description: 'Available to every step after this one.',
            fields: [
                { name: '_stepOutput.parsedDICOM', type: 'object', description: 'The extracted metadata fields listed above.' },
                { name: '_stepOutput.fieldCount', type: 'number', description: 'Number of metadata fields extracted.' },
            ],
        },
    };

    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();
