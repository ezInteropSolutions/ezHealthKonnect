/**
 * TransformMiscStepsDocs — Documentation for the remaining flat-namespace steps
 *
 * file_parser, remove_duplicates, data_masking, plus the legacy post.data_masking alias.
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {};
        docs['file_parser'] = {
            description: 'Parses structured files into an array of records for downstream pipeline steps. Supports CSV, TSV, fixed-width positional (CCLF, NACHA, X12), Excel .xlsx/.xls, Apache Avro, and Apache Parquet. Three source modes: read content from a pipeline field (field), from the server filesystem (local_path), or from a URI stored in a pipeline field — including s3://, https://, and file:// (field_as_path). Includes OOB healthcare templates for common fixed-width formats so no column mapping is needed. Typically placed after an Inbound Connector step, followed by Remove Duplicates and a Loop to process each record.',
            useCases: [
                'Parse a CSV feed received via SFTP — content stored in a pipeline field by the Inbound Connector',
                'Parse a fixed-width CCLF Part A file from a local volume mount using the cclf1 OOB template — no column mapping required',
                'Parse a NACHA ACH payment file using the nacha_entry OOB template',
                'Read a CSV from S3: a prior API step stores the S3 URI in a field, field_as_path fetches and parses it (credentials from interface connectivity config)',
                'Parse an Excel .xlsx spreadsheet received as base64 content in a pipeline field',
                'Parse Apache Avro files from Kafka consumers or data pipelines — column names come from the embedded Avro schema',
                'Parse Apache Parquet exports from data warehouses (Snowflake, BigQuery, Databricks) — supports files up to 500 MB',
                'Batch mode: parse all CCLF files in a directory matching a glob pattern and combine into one record array',
                'Auto-detect format: drop any file and let magic bytes and heuristics determine CSV/TSV/XLSX/Avro/Parquet automatically',
                'Sample a large CSV with maxRecords: 100 — streaming read, only 100 rows loaded into memory regardless of file size'
            ],
            example: {
                sourceType: 'field',
                sourceField: 'enriched.connector_result.content',
                autoDetect: true
            },
            parameters: [
                {
                    name: 'sourceType',
                    type: 'enum: field | local_path | field_as_path',
                    required: false,
                    description: 'Where to read the file from. "field" (default) — raw file content already in a pipeline field (set by an Inbound Connector). "local_path" — read directly from the server/container filesystem; supports glob pattern + batch mode. "field_as_path" — a pipeline field holds a URI that is resolved at runtime: s3://bucket/key (AWS S3, credentials from interface connectivity config), https://... (HTTP GET), file:///... (local filesystem).'
                },
                {
                    name: 'sourceField',
                    type: 'string',
                    required: true,
                    description: 'For sourceType=field: the pipeline field holding raw file content (e.g. "enriched.sftp_content"). For sourceType=field_as_path: the pipeline field holding the URI to resolve (e.g. "enriched.s3_uri").'
                },
                {
                    name: 'filePath',
                    type: 'string',
                    required: false,
                    description: 'For sourceType=local_path: absolute path to the file or directory on the server/container (e.g. "/data/cclf/PARTA.T.ACO.D250101.T000001"). With batchMode=true, this is the base directory and filePattern is the glob.'
                },
                {
                    name: 'batchMode',
                    type: 'boolean',
                    required: false,
                    description: 'local_path only. When true, processes all files matching filePattern in the filePath directory and returns a combined array of results with per-file record counts. Use for daily batch feeds where an entire directory needs to be processed.'
                },
                {
                    name: 'filePattern',
                    type: 'string',
                    required: false,
                    description: 'Glob filename pattern used in batch mode (e.g. "PARTA*.T.*"). Combined with filePath to produce the full glob. Example: filePath="/data/cclf" + filePattern="PARTA*.T.*" matches all Part A files in the directory.'
                },
                {
                    name: 'autoDetect',
                    type: 'boolean',
                    required: false,
                    description: 'Enable automatic format detection. Magic bytes are checked first (detects XLSX, XLS, Avro, Parquet from binary headers). Then file extension (for local_path). Then delimiter heuristics (CSV vs TSV). Also auto-infers hasHeader from column name patterns. Overrides fileFormat.'
                },
                {
                    name: 'fileFormat',
                    type: 'enum',
                    required: false,
                    description: 'Explicit format: csv, tsv, fixed_width, xlsx, xls, avro, parquet, auto. Binary formats (xlsx, xls, avro, parquet) are auto-detected from magic bytes when sourceType=local_path, overriding whatever is configured. Avro and Parquet derive column names from their embedded schema — hasHeader is ignored for these.'
                },
                {
                    name: 'template',
                    type: 'string',
                    required: false,
                    description: 'OOB template key for fixed-width formats — no manual column mapping needed. Available templates: cclf1 (Part A Claims Header), cclf2 (Part A Revenue), cclf3 (Part A PPS/SNF), cclf4 (Part B Physicians), cclf5 (Part B DME), cclf6 (Part D Drug Events), cclf7 (Beneficiary Demographics), cclf8 (Beneficiary XREF), nacha_entry (ACH Entry Detail), era_835_header (X12 835 Interchange).'
                },
                {
                    name: 'columns',
                    type: 'array',
                    required: false,
                    description: 'Manual column definitions for fixed-width format. Each entry: { name, start, length }. Start is 1-based byte position, length is field width in bytes. Overrides template if both are set.'
                },
                {
                    name: 'delimiter',
                    type: 'string',
                    required: false,
                    description: 'Field separator character for CSV/TSV. Default: comma for CSV, tab for TSV. Ignored for binary formats (xlsx, xls, avro, parquet).'
                },
                {
                    name: 'hasHeader',
                    type: 'boolean',
                    required: false,
                    description: 'Whether the first row contains column names. Default: true. When false, columns are named col_1, col_2, ... for CSV/TSV. Ignored for Avro and Parquet (schema is embedded).'
                },
                {
                    name: 'sheetName',
                    type: 'string',
                    required: false,
                    description: 'xlsx/xls: Name of the sheet to parse. Leave empty to use the first sheet. Use sheetIndex if the sheet has no name.'
                },
                {
                    name: 'sheetIndex',
                    type: 'number',
                    required: false,
                    description: 'xlsx/xls: 0-based sheet index. Used when sheetName is empty. Default: 0 (first sheet).'
                },
                {
                    name: 'contentEncoding',
                    type: 'enum',
                    required: false,
                    description: 'Set to "base64" when binary file content (Excel, Avro, Parquet) was base64-encoded before being stored in a pipeline field. Common when binary data passes through JSON-based APIs or message queues. The executor decodes it before parsing.'
                },
                {
                    name: 'trimFields',
                    type: 'boolean',
                    required: false,
                    description: 'Trim leading/trailing whitespace from all string values. Default: true. Applies to CSV, TSV, fixed-width, and string fields in Avro/Parquet.'
                },
                {
                    name: 'skipRows',
                    type: 'number',
                    required: false,
                    description: 'Number of rows/records to skip from the top before parsing begins. Useful for files with non-data header rows (e.g. report title, generation date). For Avro/Parquet: skips that many records in the stream.'
                },
                {
                    name: 'maxRecords',
                    type: 'number',
                    required: false,
                    description: 'Maximum records to parse (0 = unlimited). For CSV/TSV: uses streaming when set — only maxRecords rows are read from the file, making it O(maxRecords) memory regardless of file size. Ideal for sampling large files. For Avro/Parquet: stops reading after maxRecords records.'
                },
                {
                    name: 'maxFileSizeMB',
                    type: 'number',
                    required: false,
                    description: 'File size limit in MB for local_path and file:// sources (0 = default 100 MB, hard cap 500 MB). The file size is checked via stat() before reading — oversized files are rejected immediately with a descriptive error. Use maxRecords to sample files larger than the limit.'
                },
                {
                    name: 'interface_id',
                    type: 'string',
                    required: false,
                    description: 'Required when sourceType=field_as_path and the URI is an s3:// address. Identifies which interface connectivity config to look up for AWS credentials (access key ID, secret access key, region). Credentials are AES-256-GCM decrypted at runtime — never stored in plaintext in the step config.'
                }
            ],
            examples: [
                {
                    label: 'CSV from pipeline field (SFTP content)',
                    config: { sourceType: 'field', sourceField: 'enriched.sftp_content', fileFormat: 'csv', hasHeader: true, trimFields: true }
                },
                {
                    label: 'CCLF1 fixed-width from local file (OOB template)',
                    config: { sourceType: 'local_path', filePath: '/data/cclf/PARTA.T.A0001.ACO.ZC1Y24.D250101.T000001', fileFormat: 'fixed_width', template: 'cclf1' }
                },
                {
                    label: 'NACHA ACH payment file (OOB template)',
                    config: { sourceType: 'local_path', filePath: '/data/nacha/ACH20260201.txt', fileFormat: 'fixed_width', template: 'nacha_entry' }
                },
                {
                    label: 'S3 file via URI in pipeline field',
                    config: { sourceType: 'field_as_path', sourceField: 'enriched.s3_uri', fileFormat: 'csv', hasHeader: true, interface_id: 'your-interface-id' }
                },
                {
                    label: 'Excel from pipeline field (base64-encoded)',
                    config: { sourceType: 'field', sourceField: 'enriched.excel_b64', fileFormat: 'xlsx', contentEncoding: 'base64', sheetName: 'Claims', hasHeader: true }
                },
                {
                    label: 'Apache Avro — auto-detect format',
                    config: { sourceType: 'field', sourceField: 'enriched.avro_bytes', autoDetect: true }
                },
                {
                    label: 'Apache Parquet from local file with size limit',
                    config: { sourceType: 'local_path', filePath: '/data/warehouse/claims_2026.parquet', fileFormat: 'parquet', maxRecords: 5000, maxFileSizeMB: 200 }
                },
                {
                    label: 'Batch: all CCLF1 files in a directory',
                    config: { sourceType: 'local_path', filePath: '/data/cclf/', filePattern: 'PARTA*.T.*.ZC1*.T*', fileFormat: 'fixed_width', template: 'cclf1', batchMode: true }
                },
                {
                    label: 'Sample first 100 rows from a large CSV',
                    config: { sourceType: 'local_path', filePath: '/data/large_feed.csv', fileFormat: 'csv', hasHeader: true, maxRecords: 100 }
                }
            ],
            oobTemplates: [
                { key: 'cclf1', name: 'CCLF1 — Part A Claims Header', note: 'CMS Medicare claims, Part A inpatient' },
                { key: 'cclf2', name: 'CCLF2 — Part A Claims Revenue', note: 'Revenue center detail lines' },
                { key: 'cclf3', name: 'CCLF3 — Part A PPS / SNF', note: 'Prospective Payment System / Skilled Nursing' },
                { key: 'cclf4', name: 'CCLF4 — Part B Physicians', note: 'Physician and supplier claims' },
                { key: 'cclf5', name: 'CCLF5 — Part B DME', note: 'Durable Medical Equipment claims' },
                { key: 'cclf6', name: 'CCLF6 — Part D Drug Events', note: 'Prescription drug events' },
                { key: 'cclf7', name: 'CCLF7 — Beneficiary Demographics', note: 'Patient demographic data' },
                { key: 'cclf8', name: 'CCLF8 — Beneficiary XREF', note: 'Beneficiary cross-reference' },
                { key: 'nacha_entry', name: 'NACHA ACH Entry Detail', note: 'ACH payment record (94-char fixed)' },
                { key: 'era_835_header', name: 'ERA 835 Interchange Header', note: 'X12 835 remittance advice' }
            ]
        };
        docs['remove_duplicates'] = {
            description: 'Removes duplicate records from an array field using configurable key fields and merge strategies. ' +
                'Supports full-record hashing (no key fields needed), multi-field composite keys, three merge strategies, ' +
                'case-insensitive matching, and flexible handling of records with missing keys.\n\n' +
                'Source arrays come from prior steps via the steps.{namespace}.step_output path — ' +
                'for example a File Parser step produces steps.{ns}.step_output.records, ' +
                'a Database Enrichment step produces steps.{ns}.step_output.rows. ' +
                'Use the "Detect fields" button in the UI to auto-populate key field candidates.',
            useCases: [
                'Remove duplicate patient rows parsed from a CSV or CCLF file (key: patient_mrn)',
                'Dedup FHIR bundle entries by resource.id before loading into a FHIR server',
                'Dedup claims rows from an 835/837 file by claim_id + line_num',
                'Dedup HL7 OBX observations by test_code + observation_date within one message',
                'Merge partial records — keep first occurrence, backfill missing fields from later duplicates (strategy: merge)',
                'Drop records with null patient_id (nullKeyBehavior: remove) before FHIR mapping',
                'Deduplicate API enrichment results that return the same record multiple times'
            ],
            example: {
                sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                keyFields: ['patient_id', 'visit_date'],
                strategy: 'first',
                caseSensitive: false,
                nullKeyBehavior: 'remove'
            },
            examples: [
                {
                    label: 'Dedup CSV records by patient ID (keep first)',
                    config: {
                        sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                        keyFields: ['patient_id'],
                        strategy: 'first',
                        caseSensitive: false,
                        nullKeyBehavior: 'remove'
                    }
                },
                {
                    label: 'Dedup claims by claim ID + line number (keep last = most recent correction)',
                    config: {
                        sourceField: 'steps.file_parser_a1b2c3.step_output.records',
                        keyFields: ['claim_id', 'line_num'],
                        strategy: 'last',
                        caseSensitive: true,
                        nullKeyBehavior: 'group'
                    }
                },
                {
                    label: 'Merge partial lab records — backfill missing fields from later duplicates',
                    config: {
                        sourceField: 'steps.script_enrichment_b9fb33.step_output.observations',
                        keyFields: ['test_code', 'obs_date'],
                        strategy: 'merge',
                        caseSensitive: false,
                        nullKeyBehavior: 'keep'
                    }
                },
                {
                    label: 'Dedup database rows, write to new field (preserve original)',
                    config: {
                        sourceField: 'steps.database_enrichment_c3d4e5.step_output.rows',
                        keyFields: ['member_id'],
                        strategy: 'first',
                        caseSensitive: false,
                        outputField: 'deduped_members'
                    }
                },
                {
                    label: 'Full-record strict dedup — no key fields (every field must match)',
                    config: {
                        sourceField: 'steps.api_enrichment_f1e2d3.step_output.results',
                        strategy: 'first',
                        caseSensitive: true,
                        nullKeyBehavior: 'group'
                    }
                }
            ],
            parameters: [
                {
                    name: 'sourceField',
                    type: 'string',
                    required: true,
                    description: 'Dot-path to the array to deduplicate. ' +
                        'Must point to an array produced by a prior step — ' +
                        'File Parser: steps.{ns}.step_output.records; ' +
                        'Database Enrichment: steps.{ns}.step_output.rows; ' +
                        'Script Enrichment: steps.{ns}.step_output.{yourField}. ' +
                        'Use the Source Step picker in the UI to auto-build this path.'
                },
                {
                    name: 'keyFields',
                    type: 'Array<string>',
                    required: false,
                    description: 'Field names within each record that form the unique key. ' +
                        'Supports dot-paths for nested fields (e.g. "patient.id"). ' +
                        'Leave empty to hash the entire record — every field must match for a record to count as a duplicate. ' +
                        'Use the "Detect fields" button to auto-populate from upstream step output.'
                },
                {
                    name: 'strategy',
                    type: 'enum: first | last | merge',
                    required: false,
                    description: '"first" (default) — keep the earliest occurrence, discard all later duplicates. ' +
                        '"last" — keep the most recent occurrence (useful when later records are corrections). ' +
                        '"merge" — keep the first record and non-destructively backfill any absent fields from later records.'
                },
                {
                    name: 'caseSensitive',
                    type: 'boolean',
                    required: false,
                    description: 'When false, key field values are lowercased before hashing so "SMITH" = "smith". Default: true.'
                },
                {
                    name: 'nullKeyBehavior',
                    type: 'enum: group | keep | remove',
                    required: false,
                    description: '"group" (default) — records with null/absent key fields share one dedup bucket. ' +
                        '"keep" — records with missing keys bypass dedup and are always kept. ' +
                        '"remove" — records with null or absent key fields are dropped entirely.'
                },
                {
                    name: 'outputField',
                    type: 'string',
                    required: false,
                    description: 'Write the deduplicated array to this field instead of updating sourceField in-place. ' +
                        'Leave empty to update the source array directly. ' +
                        'Example: "deduped_patients" — downstream steps access it as steps.{thisStep}.step_output.result_path.'
                },
                {
                    name: 'previewLimit',
                    type: 'number',
                    required: false,
                    description: 'Maximum records to include in the step_output.records preview (default 100, max 1000). ' +
                        'The full deduplicated array is always written to outputField/sourceField regardless of this setting.'
                }
            ],
            stepOutput: {
                description: 'The full deduplicated array is written to outputField (or sourceField when outputField is blank). ' +
                    'The following variables are also available in downstream steps via steps.{ns}.step_output:',
                fields: [
                    { name: 'result_path', type: 'string', description: 'Dot-path where the full deduplicated array was written (use this in downstream sourceField configs)' },
                    { name: 'records', type: 'array', description: 'Preview of up to previewLimit (default 100) deduplicated records — visible in the test panel' },
                    { name: 'records_truncated', type: 'boolean', description: 'true when the full result has more rows than the preview limit' },
                    { name: 'original_count', type: 'number', description: 'Total records in the source array before dedup' },
                    { name: 'dedup_count', type: 'number', description: 'Records in the deduplicated output' },
                    { name: 'removed_count', type: 'number', description: 'Duplicate records discarded' },
                    { name: 'null_key_kept', type: 'number', description: 'Records with missing keys that were kept (nullKeyBehavior=keep)' },
                    { name: 'null_key_removed', type: 'number', description: 'Records with missing keys that were dropped (nullKeyBehavior=remove)' }
                ]
            }
        };
        docs['data_masking'] = {
            description:
                'Masks or anonymizes sensitive PHI/PII fields for HIPAA compliance. Applies ordered masking ' +
                'rules to individual fields using six strategies:\n\n' +
                '  • mask — replace every character with maskChar (default *)\n' +
                '  • redact — replace with [REDACTED]\n' +
                '  • partial — reveal first N and/or last N characters, mask the middle\n' +
                '  • hash — SHA-256 deterministic 16-char hex (same salt = same output = join-safe de-ID)\n' +
                '  • tokenize — non-reversible TOK-{12hex} token\n' +
                '  • substitute — replace with realistic fake data (name, SSN, phone, email, date, address, or custom fixed value)\n\n' +
                'Field paths are format-agnostic — any of these work:\n' +
                '  • HL7 v2:  PID.5, PID.19, MSH.9.1\n' +
                '  • FHIR R4: steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family\n' +
                '    (entry index is auto-resolved — if the Patient is at entry[1], it will be found automatically)\n' +
                '  • Cross-step JSON: steps.api_enrichment.step_output.email\n' +
                '  • CSV / DB records: steps.file_parser.step_output.records[0].ssn\n' +
                '    (index [0] is auto-searched across all records in the array)\n\n' +
                'Enable "Mask All PHI" to instantly apply pre-configured HIPAA Safe Harbor rules for the ' +
                'selected format (HL7 v2: 23 rules / FHIR R4: 20 rules / JSON: 18 rules). Custom rules run after.\n\n' +
                'Step output is available downstream via steps.{namespace}.step_output.*',

            useCases: [
                'Mask HL7 patient name (PID.5) and DOB (PID.7) before sending to a test environment',
                'Hash SSN (PID.19) for de-identification while keeping cross-dataset join capability',
                'Partial-mask phone — keep last 4 digits: 555-867-5309 → ***-***-5309 (preserveFormat)',
                'Substitute patient name with realistic fake name for synthetic test data generation',
                'Mask FHIR Patient.name.family after an HL7→FHIR transform step — entry index resolved automatically',
                'Mask email returned by an API enrichment step: steps.api_enrichment.step_output.email',
                'Mask SSN column in all CSV rows from a File Parser step: steps.file_parser.step_output.records[0].ssn',
                'Enable Mask All PHI → Format = FHIR to auto-mask 20 FHIR R4 PHI fields with one toggle',
                'Redact all PHI before storing to audit log (maskAllPHI: true, strategy override: redact)',
                'Use preserveFormat with partial to keep SSN dashes: 123-45-6789 → ***-**-6789'
            ],

            example: {
                maskAllPHI: false,
                maskAllPHIFormat: 'hl7v2',
                preserveFormat: false,
                rules: [
                    { field: 'PID.5',  strategy: 'mask' },
                    { field: 'PID.19', strategy: 'hash', hashSalt: 'myOrgSecret2025' },
                    { field: 'PID.13', strategy: 'partial', keepFirst: 0, keepLast: 4 },
                    { field: 'PID.5',  strategy: 'substitute', substituteType: 'name' },
                    {
                        field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                        strategy: 'mask'
                    },
                    { field: 'steps.api_enrichment.step_output.email', strategy: 'mask' }
                ]
            },

            examples: [
                {
                    label: 'HL7 — full mask patient name',
                    config: { rules: [{ field: 'PID.5', strategy: 'mask' }], maskAllPHI: false }
                },
                {
                    label: 'HL7 — hash SSN for join-safe de-identification',
                    config: {
                        rules: [{ field: 'PID.19', strategy: 'hash', hashSalt: 'myOrgSecret2025' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'HL7 — partial-mask phone, keep last 4, preserve dashes',
                    config: {
                        rules: [{ field: 'PID.13', strategy: 'partial', keepFirst: 0, keepLast: 4 }],
                        maskAllPHI: false,
                        preserveFormat: true
                    }
                },
                {
                    label: 'HL7 — substitute patient name with realistic fake name',
                    config: {
                        rules: [{ field: 'PID.5', strategy: 'substitute', substituteType: 'name' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'FHIR — mask family name from HL7→FHIR transform step output',
                    config: {
                        rules: [{
                            field: 'steps.hl7_fhir_transform.step_output.fhir_bundle.entry[0].resource.patient.name[0].family',
                            strategy: 'mask'
                        }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'FHIR — auto-mask all 20 FHIR PHI fields (Mask All PHI)',
                    config: { rules: [], maskAllPHI: true, maskAllPHIFormat: 'fhir' }
                },
                {
                    label: 'Cross-step — mask email from API enrichment result',
                    config: {
                        rules: [{ field: 'steps.api_enrichment.step_output.email', strategy: 'mask' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'CSV / DB records — hash SSN in all records from File Parser',
                    config: {
                        rules: [{ field: 'steps.file_parser.step_output.records[0].ssn', strategy: 'hash', hashSalt: 'myOrgSecret2025' }],
                        maskAllPHI: false
                    }
                },
                {
                    label: 'Auto-mask all HL7 HIPAA PHI — 23 fields, 13 identifiers',
                    config: { rules: [], maskAllPHI: true, maskAllPHIFormat: 'hl7v2' }
                },
                {
                    label: 'Substitute DOB with realistic fake date (preserve year)',
                    config: {
                        rules: [{ field: 'PID.7', strategy: 'substitute', substituteType: 'date' }],
                        maskAllPHI: false
                    }
                }
            ],

            parameters: [
                {
                    name: 'rules',
                    type: 'MaskingRule[]',
                    required: false,
                    description: 'Ordered list of field masking rules. Each rule specifies a field path and strategy. ' +
                        'Rules run in sequence. If multiple rules match the same field, the last one applied wins.'
                },
                {
                    name: 'rules[].field',
                    type: 'string',
                    required: true,
                    description: 'Field path to mask. Supports all formats:\n' +
                        '  HL7 v2: PID.5, PID.19, MSH.9.1, PID.5.1\n' +
                        '  FHIR R4: steps.{step}.step_output.fhir_bundle.entry[0].resource.patient.name[0].family\n' +
                        '    (entry[N] index is auto-searched if the field is not found at index N)\n' +
                        '  Cross-step JSON: steps.{step}.step_output.email\n' +
                        '  CSV / DB records: steps.{step}.step_output.records[0].field_name\n' +
                        '    (array index is auto-searched across all records)\n' +
                        '  Generic JSON: data.patient.ssn, message.payload.name'
                },
                {
                    name: 'rules[].strategy',
                    type: 'enum',
                    required: true,
                    description: 'Masking strategy:\n' +
                        '  "mask"       — replace all chars with maskChar (default *). Example: John → ****\n' +
                        '  "redact"     — replace with [REDACTED]. Example: John → [REDACTED]\n' +
                        '  "partial"    — keep keepFirst chars at start and keepLast chars at end, mask middle.\n' +
                        '                 Example: 555-867-5309 with keepLast=4 → 555-867-****\n' +
                        '                 Set preserveFormat=true to also keep non-digit separators in place.\n' +
                        '  "hash"       — SHA-256 of (hashSalt + value), returns first 16 hex chars.\n' +
                        '                 Deterministic: same input + same salt = same output (join-safe de-ID).\n' +
                        '  "tokenize"   — non-reversible token: TOK-{12 hex chars}. Each unique value gets a unique token.\n' +
                        '  "substitute" — replace with realistic fake data. Set substituteType to control output:\n' +
                        '                 name, ssn (9XX-XX-XXXX ITIN range), phone ((555) XXX-XXXX),\n' +
                        '                 email (test.{hex}@example-test.com), date (preserves year),\n' +
                        '                 address (1000-9999 fictional street), custom (fixed substituteValue).\n' +
                        '                 All substitute values are deterministic — same input = same fake output.'
                },
                {
                    name: 'rules[].substituteType',
                    type: 'enum',
                    required: false,
                    description: 'Category of fake data to generate (substitute strategy only). Options: ' +
                        'name, ssn, phone, email, date, address, custom. ' +
                        'All types produce values that are clearly non-real and safe for test environments.'
                },
                {
                    name: 'rules[].substituteValue',
                    type: 'string',
                    required: false,
                    description: 'Fixed replacement string used when substituteType is "custom". ' +
                        'Example: substituteValue: "[TEST PATIENT]". The same value is used for every field match.'
                },
                {
                    name: 'rules[].maskChar',
                    type: 'string (1 char)',
                    required: false,
                    description: 'Replacement character for mask and partial strategies. Default: *.'
                },
                {
                    name: 'rules[].keepFirst',
                    type: 'integer',
                    required: false,
                    description: 'Characters to reveal at the start of the value (partial strategy). Default: 0.'
                },
                {
                    name: 'rules[].keepLast',
                    type: 'integer',
                    required: false,
                    description: 'Characters to reveal at the end of the value (partial strategy). Default: 4.'
                },
                {
                    name: 'rules[].hashSalt',
                    type: 'string',
                    required: false,
                    description: 'Salt prepended before SHA-256 hashing (hash strategy). Default: "ezHealthKonnect". ' +
                        'Use the same salt across environments to preserve cross-dataset join capability on hashed values.'
                },
                {
                    name: 'rules[].pattern',
                    type: 'string (regex)',
                    required: false,
                    description: 'Optional regex. When set, only substrings matching the pattern are masked — ' +
                        'non-matching characters are preserved. Example: \\d{4} on "Acct 1234-5678" masks only the digits.'
                },
                {
                    name: 'maskAllPHI',
                    type: 'boolean',
                    required: false,
                    description: 'When true, prepends format-specific HIPAA Safe Harbor rules before any custom rules:\n' +
                        '  HL7 v2 (23 rules): Names (PID.5/6, NK1.2, GT1.3), Geographic (PID.11.x), Dates (PID.7/29, PV1.44/45),\n' +
                        '    Phone/Fax (PID.13/14), Email (PID.13.4), SSN (PID.19 hash), MRN (PID.3 partial),\n' +
                        '    Insurance (IN1.49/2), Account (PID.18), License (PID.20 hash), Device ID (OBX.18 hash), Other IDs (PID.2/4)\n' +
                        '  FHIR R4 (20 rules): Patient.name, Patient.address, Patient.birthDate, Patient.telecom,\n' +
                        '    Patient.identifier, Encounter.period, Coverage, Account, Practitioner, Device\n' +
                        '  JSON (18 rules): patient.name/firstName/lastName, patient.dateOfBirth/dob, patient.ssn,\n' +
                        '    patient.phone/fax, patient.email, patient.mrn, patient.address, patient.zipCode, patient.insuranceId\n' +
                        'Custom rules still apply after the auto-PHI rules.'
                },
                {
                    name: 'maskAllPHIFormat',
                    type: 'enum',
                    required: false,
                    description: 'Format for auto-PHI rules when maskAllPHI is true. Options: "hl7v2" (default), "fhir", "json". ' +
                        'Set to "fhir" when masking data after an HL7→FHIR transform step, ' +
                        'or "json" when masking generic JSON patient records from a DB or CSV source.'
                },
                {
                    name: 'preserveFormat',
                    type: 'boolean',
                    required: false,
                    description: 'For partial strategy: mask digit characters only and keep non-digit separators in place. ' +
                        'Example: 555-867-5309 with keepLast=4 → ***-***-5309. ' +
                        'keepFirst/keepLast count only digits — separators are never counted or masked.'
                }
            ],

            stepOutput: {
                description: 'Reference these in downstream steps via steps.{namespace}.step_output.*',
                fields: [
                    { name: 'masked_count',  type: 'integer',  description: 'Number of fields successfully masked. A field not found in the message is not counted.' },
                    { name: 'masked_fields', type: 'string[]', description: 'Ordered list of resolved field paths that were masked (may differ from the configured path when entry index was auto-resolved).' },
                    { name: 'total_rules',   type: 'integer',  description: 'Total rules evaluated including maskAllPHI auto-rules.' }
                ]
            }
        };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('post.data_masking', 'data_masking');
