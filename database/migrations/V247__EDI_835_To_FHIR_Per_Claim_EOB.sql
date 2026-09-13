-- V247: EDI X12 835 to FHIR -- per-claim ExplanationOfBenefit (supersedes V231/V234)
-- Applied: 2026-09-13

-- ============================================================
-- EDI 835 -> FHIR (ExplanationOfBenefit + PaymentReconciliation) TEMPLATE
-- ============================================================
-- Re-ships the SAME template slug ('edi-835-to-fhir-sftp') V231 created and
-- V234 patched -- never editing an already-applied migration in place (same
-- precedent V234 itself set). This round closes the ONE real gap V231's own
-- comment named as impossible at the time: a per-claim ExplanationOfBenefit
-- resource, one per X12 CLP/2100 loop instance, not just the interchange-
-- level PaymentReconciliation V231 already builds.
--
--   seq 5    connector.inbound   (edi_x12_inbound  — polls an SFTP directory for 835 files)
--   seq 20   edi.parse           (raw X12 -> structured JSON)
--   seq 30   edi.validate        (base X12 5010 standard checks; errors block, warnings never do)
--   seq 95   enrichment.script   (Derive 835 Claim Context — pure data reshaping only)
--   seq 100  fhir.build          (ExplanationOfBenefit — rowsPath mode, one per claim)
--   seq 110  fhir.build          (PaymentReconciliation — UNCHANGED from V234, one per interchange)
--   seq 200  payload.builder     (fhir_bundle mode — assembles the Bundle)
--   seq 210  fhir_validation     (strict — validates the assembled Bundle)
--   seq 295  connector.outbound  (sink_outbound — store-only terminal, no external I/O)
--
-- V231's own blocker (a per-claim control.loop step's own childStepIds can
-- only hold real DB-assigned step UUIDs, which don't exist at template-
-- authoring time) is solved the same generic way 837P/837I's own templates
-- already solved it: fhir_build_executor.go's rowsPath mode builds ONE
-- resource PER ROW found at a path, writing an ARRAY to outputField, with
-- no control.loop wrapper or step-ID wiring needed at all.
--
-- A real mechanism gap found and fixed while building this (not 835-
-- specific): rowsPath's own field resolution is ROW-ONLY (resolveRawValue
-- is handed only the current row, never inputData) — a claim row (a 2100
-- loop instance) has no reachable path back up to the interchange header,
-- so the NEW "Derive 835 Claim Context" script above copies the 2
-- header-level values every claim needs (BPR16 payment date, 1000A payer
-- name) onto EACH row before fhir.build ever sees them. Also closed in the
-- same round: fhir_build_executor.go gained a generic "_rowIndex" primitive
-- (every row from ANY rowsPath or repeatingGroup RowsPath now carries its
-- own 1-based position under that key) — closing EOB.item.sequence, a gap
-- V231's own predecessor test had left unmapped for lack of any per-row
-- auto-index primitive.
--
-- Config here is transcribed verbatim (via a small Node script that reads
-- the Go source directly and JSON-encodes it, avoiding hand-retyping risk)
-- from services/executors/transform/edi_835_to_fhir_test.go
-- (TestEDI835ToFHIR_BlueCrossNC_SingleClaim_BuildsCorrectBundleShape,
-- TestEDI835ToFHIR_EMedNY_MultiClaim_BuildsOneEOBPerClaim), which chains the
-- real executors — edi.parse -> enrichment.script -> fhir.build(EOB,
-- rowsPath) -> fhir.build(PaymentReconciliation) -> payload.builder ->
-- fhir_validation(strict) — against all real 835 samples in
-- edi/testdata/real_samples/ and asserts the resulting Bundle validates at
-- strict level with ONLY the 2 named gaps below as errors, nothing else.
--
-- Named gaps, stated up front rather than silently left out (unchanged from
-- V231's own predecessor test, still real and evidence-based):
--   - EOB.type stays hardcoded "professional" — an 835 alone can't reliably
--     distinguish claim types without deeper facility-type-code knowledge
--     this pass doesn't verify.
--   - EOB.provider and EOB.insurance stay unmapped. provider was attempted
--     via NM1[entityIdentifierCode=82] (X12's own "Rendering Provider" role
--     code) but neither real 835 sample in this repo carries an "82" NM1
--     occurrence at the claim level — the mapping is correct, the source
--     data for it is simply often absent. insurance (a Coverage reference)
--     was never attempted — no Coverage resource is built in this pass.
--   - PLB (provider-level balance) is NOT mapped — none of the 3 real 835
--     samples in this repo carries a PLB segment; this project's own
--     established discipline is to not fabricate fixtures where real ones
--     don't exist.
--   - CAS-derived adjudication entries put the literal CARC code directly on
--     category.coding[0].code (system = the verified X12 CARC CodeSystem
--     URL) — a deliberate, spec-legal choice (that value set's binding
--     strength is "example" per hl7.org/fhir/R4/explanationofbenefit-
--     definitions.html), not a fabricated crosswalk.

INSERT INTO interface_templates
    (id, name, slug, description, category, subcategory, tags, icon, difficulty,
     source_connector_type, source_config_template,
     target_connector_type, target_config_template,
     pipeline_config, message_type,
     required_connection_fields, sanitized_fields, preview_steps,
     estimated_setup_minutes, is_system, is_public, author, usage_count)
VALUES
(
    gen_random_uuid(),
    'EDI X12 835 to FHIR (SFTP)',
    'edi-835-to-fhir-sftp',
    'Poll an SFTP directory for X12 835 remittance files, parse and validate them, then build and validate a FHIR R4 Bundle containing one ExplanationOfBenefit resource per claim plus one PaymentReconciliation resource for the whole interchange.',
    'edi',
    null,
    ARRAY['edi','x12','835','remittance','payers','sftp','fhir'],
    '💰',
    'intermediate',
    'edi_x12_inbound',
    '{"transport": "sftp", "remote_path": "/incoming", "file_pattern": "*.edi", "polling_interval_seconds": 300, "after_processing": "archive", "transaction_types": ["835"]}',
    'sink_outbound',
    '{"enable_logging": true, "enable_validation": true}',
    $pipeline$
{
    "execution_groups": [
        {
            "sequence": 5,
            "steps": [
                {
                    "step_name": "Receive 835 File (SFTP)",
                    "step_type": "connector.inbound",
                    "sequence": 5,
                    "config": {
                        "connectorType": "edi_x12_inbound",
                        "config": {
                            "transport": "sftp",
                            "remote_path": "/incoming",
                            "file_pattern": "*.edi",
                            "polling_interval_seconds": 300,
                            "after_processing": "archive",
                            "transaction_types": [
                                "835"
                            ]
                        }
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 20,
            "steps": [
                {
                    "step_name": "Parse 835 -> JSON",
                    "step_type": "edi.parse",
                    "sequence": 20,
                    "config": {
                        "sourceField": "raw",
                        "outputField": "parsedEDI",
                        "transactionSet": "835"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 30,
            "steps": [
                {
                    "step_name": "Validate Against X12 5010",
                    "step_type": "edi.validate",
                    "sequence": 30,
                    "config": {
                        "sourceField": "raw",
                        "outputField": "ediValidation",
                        "customRules": []
                    },
                    "enabled": true,
                    "required": false
                }
            ]
        },
        {
            "sequence": 95,
            "steps": [
                {
                    "step_name": "Derive 835 Claim Context",
                    "step_alias": "derive_835_claim_context",
                    "step_type": "enrichment.script",
                    "sequence": 95,
                    "description": "Pure data reshaping only -- no FHIR resource-shape knowledge. rowsPath's own field resolution is row-only (no automatic fallback to interchange-header data), so this copies the 2 header-level values every claim needs (BPR16 payment date, 1000A payer name) onto EACH claim row, and fully flattens CLP/NM1/SVC/CAS into one flat claim_rows[] array the ExplanationOfBenefit build below can consume declaratively via rowsPath, with no control.loop wrapper step needed.",
                    "config": {
                        "script": "\n// A prior step's plain top-level output field (edi.parse's own \"parsedEDI\",\n// per its outputField config) is NOT exposed to a later step's own \"input\"\n// at the top level -- the real pipeline engine nests it under\n// input.message.parsedEDI instead (confirmed by the 837P/837I derive\n// scripts' own real Test-Pipeline-run finding, which applies here too).\n// Prefer that real shape; fall back to the bare top-level key for\n// Go-level test harnesses that construct { parsedEDI: ... } directly\n// without a \"message\" wrapper (as edi_835_to_fhir_test.go does).\nvar parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};\nvar loops = parsed.loops || {};\nvar header = parsed.header || {};\n\nfunction arr(v) {\n  if (!v) return [];\n  return Array.isArray(v) ? v : [v];\n}\n\nfunction byEntityIdentifierCode(nm1, code) {\n  var list = arr(nm1);\n  for (var i = 0; i < list.length; i++) {\n    if (list[i].entityIdentifierCode === code) return list[i];\n  }\n  return {};\n}\n\nvar headerPaymentDate = (header.BPR && header.BPR.paymentEffectiveDate) || \"\";\nvar payerName = ((loops[\"1000A\"] || {}).N1 || {}).name || \"\";\n\n// CAS is maxUse \">1\" within a service line (a line can carry more than one\n// CAS occurrence, e.g. one per claim-adjustment-group-code), each with its\n// own up-to-6 reason/amount/quantity trios in its own \"adjustments\" repeat\n// group (edi/schemas/x12_005010/segments/CAS.json). Flatten every\n// occurrence's trios into one combined list here, in JS, rather than\n// relying on fhir.build's own \"CAS[*].adjustments\" wildcard-flatten path —\n// this script now owns all the row reshaping, matching the 837P/837I\n// precedent of doing this flattening once, in one place.\nfunction extractAdjustments(casList) {\n  var instances = arr(casList);\n  var out = [];\n  for (var i = 0; i < instances.length; i++) {\n    var trios = instances[i].adjustments || [];\n    for (var j = 0; j < trios.length; j++) {\n      if (!trios[j].reasonCode) continue;\n      out.push({ reason_code: trios[j].reasonCode, amount: trios[j].amount || \"0\" });\n    }\n  }\n  return out;\n}\n\nfunction extractServiceLines(claimRow) {\n  var lines = arr((claimRow.loops || {})[\"2110\"]);\n  var out = [];\n  for (var i = 0; i < lines.length; i++) {\n    var svc = lines[i].SVC || {};\n    var proc = svc.procedureCode || {};\n    out.push({\n      procedure_code: proc.code || \"\",\n      charge_amount: svc.chargeAmount || \"0\",\n      paid_amount: svc.paidAmount || \"0\",\n      adjustments: extractAdjustments(lines[i].CAS)\n    });\n  }\n  return out;\n}\n\n// 2000 (Header Number) is maxUse \">1\" -- every real sample checked into this\n// repo only ever carries one, but the spec allows more (e.g. a\n// clearinghouse batching multiple LX groups into one interchange), so every\n// group is walked rather than assumed away, mirroring the same\n// walk-every-occurrence fix already made for 837's own 2000A billing\n// provider loop.\nvar claimRows = [];\nvar headerGroups = arr(loops[\"2000\"]);\nfor (var g = 0; g < headerGroups.length; g++) {\n  var claims = arr((headerGroups[g].loops || {})[\"2100\"]);\n  for (var c = 0; c < claims.length; c++) {\n    var claimRow = claims[c];\n    var clp = claimRow.CLP || {};\n    var patientNM1 = byEntityIdentifierCode(claimRow.NM1, \"QC\");\n    // Rendering Provider (role 82) -- named, evidence-based gap: neither\n    // real sample in this repo carries an \"82\" NM1 occurrence at the claim\n    // level, so this resolves empty today, but the mapping itself is\n    // correct per the X12 835 IG and is kept rather than removed.\n    var providerNM1 = byEntityIdentifierCode(claimRow.NM1, \"82\");\n    claimRows.push({\n      patient_control_number: clp.patientControlNumber || \"\",\n      claim_payment_amount: clp.claimPaymentAmount || \"0\",\n      patient_name: patientNM1.nameLastOrOrganizationName || \"\",\n      provider_name: providerNM1.nameLastOrOrganizationName || \"\",\n      header_payment_date: headerPaymentDate,\n      payer_name: payerName,\n      service_lines: extractServiceLines(claimRow)\n    });\n  }\n}\n\nreturn { claim_rows: claimRows };\n"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 100,
            "steps": [
                {
                    "step_name": "Build ExplanationOfBenefit",
                    "step_alias": "build_eob_fhir",
                    "step_type": "fhir.build",
                    "sequence": 100,
                    "description": "Builds one FHIR ExplanationOfBenefit resource PER CLAIM (rowsPath mode) from the derive step above. item.sequence uses the new $rowIndex-style \"_rowIndex\" primitive (fhir_build_executor.go) every row carries automatically. Real, named gaps left unmapped: EOB.type is hardcoded \"professional\" (an 835 alone can't reliably distinguish claim types); EOB.provider (no role-82 NM1 in real 835 samples) and EOB.insurance (no Coverage resource built in this pass) are left unpopulated -- strict validation will report exactly these 2 required-field gaps, nothing else.",
                    "config": {
                        "resourceType": "ExplanationOfBenefit",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.explanationOfBenefits",
                        "rowsPath": "steps.derive_835_claim_context.step_output.claim_rows",
                        "fields": [
                            {
                                "targetPath": "id",
                                "sourcePath": "patient_control_number"
                            },
                            {
                                "targetPath": "status",
                                "literalValue": "active"
                            },
                            {
                                "targetPath": "type.coding[0].system",
                                "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"
                            },
                            {
                                "targetPath": "type.coding[0].code",
                                "literalValue": "professional"
                            },
                            {
                                "targetPath": "use",
                                "literalValue": "claim"
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "complete"
                            },
                            {
                                "targetPath": "created",
                                "sourcePath": "header_payment_date",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "patient.display",
                                "sourcePath": "patient_name"
                            },
                            {
                                "targetPath": "provider.display",
                                "sourcePath": "provider_name"
                            },
                            {
                                "targetPath": "insurer.display",
                                "sourcePath": "payer_name"
                            },
                            {
                                "targetPath": "payment.amount.value",
                                "sourcePath": "claim_payment_amount",
                                "transform": "cda_decimal_string_to_number"
                            }
                        ],
                        "repeatingGroups": [
                            {
                                "targetPath": "item",
                                "rowsPath": "service_lines",
                                "fields": [
                                    {
                                        "targetPath": "sequence",
                                        "sourcePath": "_rowIndex",
                                        "transform": "cda_decimal_string_to_number"
                                    },
                                    {
                                        "targetPath": "productOrService.coding[0].system",
                                        "literalValue": "https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"
                                    },
                                    {
                                        "targetPath": "productOrService.coding[0].code",
                                        "sourcePath": "procedure_code"
                                    },
                                    {
                                        "targetPath": "adjudication[0].category.coding[0].system",
                                        "literalValue": "http://terminology.hl7.org/CodeSystem/adjudication"
                                    },
                                    {
                                        "targetPath": "adjudication[0].category.coding[0].code",
                                        "literalValue": "submitted"
                                    },
                                    {
                                        "targetPath": "adjudication[0].amount.value",
                                        "sourcePath": "charge_amount",
                                        "transform": "cda_decimal_string_to_number"
                                    },
                                    {
                                        "targetPath": "adjudication[1].category.coding[0].system",
                                        "literalValue": "http://terminology.hl7.org/CodeSystem/adjudication"
                                    },
                                    {
                                        "targetPath": "adjudication[1].category.coding[0].code",
                                        "literalValue": "benefit"
                                    },
                                    {
                                        "targetPath": "adjudication[1].amount.value",
                                        "sourcePath": "paid_amount",
                                        "transform": "cda_decimal_string_to_number"
                                    }
                                ],
                                "repeatingGroups": [
                                    {
                                        "targetPath": "adjudication",
                                        "rowsPath": "adjustments",
                                        "fields": [
                                            {
                                                "targetPath": "category.coding[0].system",
                                                "literalValue": "https://x12.org/codes/claim-adjustment-reason-codes"
                                            },
                                            {
                                                "targetPath": "category.coding[0].code",
                                                "sourcePath": "reason_code"
                                            },
                                            {
                                                "targetPath": "amount.value",
                                                "sourcePath": "amount",
                                                "transform": "cda_decimal_string_to_number"
                                            }
                                        ]
                                    }
                                ]
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 110,
            "steps": [
                {
                    "step_name": "Build PaymentReconciliation",
                    "step_alias": "build_payment_reconciliation",
                    "step_type": "fhir.build",
                    "sequence": 110,
                    "description": "Builds one PaymentReconciliation resource from the interchange's BPR (payment amount/date) and TRN (check/EFT trace number) header segments, plus the payer's own N1 identity from loop 1000A. detail[].response.reference is keyed off the same CLP.patientControlNumber value the ExplanationOfBenefit build above uses for its own id, so payload.builder's fullUrl-rewrite correctly cross-references each EOB. Unchanged from this template's prior version (V231/V234) -- reads parsedEDI directly, never through a script's own step_output, so none of the row-scoping/snake-casing rules governing the EOB step above apply here.",
                    "config": {
                        "resourceType": "PaymentReconciliation",
                        "profile": "base",
                        "version": "R4",
                        "outputField": "message.paymentReconciliation",
                        "fields": [
                            {
                                "targetPath": "id",
                                "sourcePath": "message.parsedEDI.header.TRN.checkOrEFTTraceNumber"
                            },
                            {
                                "targetPath": "status",
                                "literalValue": "active"
                            },
                            {
                                "targetPath": "outcome",
                                "literalValue": "complete"
                            },
                            {
                                "targetPath": "created",
                                "sourcePath": "message.parsedEDI.header.BPR.paymentEffectiveDate",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "paymentDate",
                                "sourcePath": "message.parsedEDI.header.BPR.paymentEffectiveDate",
                                "transform": "x12_date_to_fhir_date"
                            },
                            {
                                "targetPath": "paymentAmount.value",
                                "sourcePath": "message.parsedEDI.header.BPR.totalActualProviderPaymentAmount",
                                "transform": "cda_decimal_string_to_number"
                            },
                            {
                                "targetPath": "paymentIssuer.display",
                                "sourcePath": "message.parsedEDI.loops.1000A.N1.name"
                            }
                        ],
                        "repeatingGroups": [
                            {
                                "targetPath": "detail",
                                "rowsPath": "message.parsedEDI.loops.2000[0].loops.2100",
                                "fields": [
                                    {
                                        "targetPath": "amount.value",
                                        "sourcePath": "CLP.claimPaymentAmount",
                                        "transform": "cda_decimal_string_to_number"
                                    },
                                    {
                                        "targetPath": "response.reference",
                                        "sourcePath": "CLP.patientControlNumber",
                                        "transform": "string_prefix",
                                        "valueMap": {
                                            "prefix": "ExplanationOfBenefit/"
                                        }
                                    }
                                ]
                            }
                        ]
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 200,
            "steps": [
                {
                    "step_name": "Assemble 835 FHIR Bundle",
                    "step_alias": "assemble_835_fhir_bundle",
                    "step_type": "payload.builder",
                    "sequence": 200,
                    "description": "Assembles the ExplanationOfBenefit array (one entry per claim) plus the single PaymentReconciliation resource into one FHIR R4 Bundle with correctly cross-referenced fullUrls.",
                    "config": {
                        "mode": "fhir_bundle",
                        "fhirBundle": {
                            "bundleType": "collection",
                            "resourcePaths": [
                                "message.explanationOfBenefits",
                                "message.paymentReconciliation"
                            ]
                        }
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        },
        {
            "sequence": 210,
            "steps": [
                {
                    "step_name": "Validate FHIR Bundle",
                    "step_type": "fhir_validation",
                    "sequence": 210,
                    "config": {
                        "validation_level": "strict",
                        "profile": "base",
                        "fhir_version": "R4",
                        "source_field": "payload"
                    },
                    "enabled": true,
                    "required": false
                }
            ]
        },
        {
            "sequence": 295,
            "steps": [
                {
                    "step_name": "Store Result",
                    "step_type": "connector.outbound",
                    "sequence": 295,
                    "config": {
                        "connectorType": "sink_outbound",
                        "config": {
                            "enable_logging": true,
                            "enable_validation": true
                        },
                        "contentField": "payload"
                    },
                    "enabled": true,
                    "required": true
                }
            ]
        }
    ]
}
    $pipeline$::jsonb,
    '835',
    '[
    {
        "section": "source",
        "field": "host",
        "label": "SFTP Host",
        "type": "string",
        "hint": "Hostname or IP of the trading partner''s SFTP server",
        "required": true
    },
    {
        "section": "source",
        "field": "username",
        "label": "SFTP Username",
        "type": "string",
        "required": true
    },
    {
        "section": "source",
        "field": "password",
        "label": "SFTP Password",
        "type": "password",
        "hint": "Leave blank if using key-based auth instead",
        "required": false
    },
    {
        "section": "source",
        "field": "remote_path",
        "label": "Remote Directory",
        "type": "string",
        "hint": "Directory to poll for 835 files, e.g. /incoming",
        "required": false
    }
]',
    ARRAY['source.password'],
    '[
    {
        "name": "Receive 835 (SFTP)",
        "icon": "📥",
        "step_type": "connector.inbound"
    },
    {
        "name": "Parse 835",
        "icon": "📄",
        "step_type": "edi.parse"
    },
    {
        "name": "Validate X12",
        "icon": "✅",
        "step_type": "edi.validate"
    },
    {
        "name": "Derive Claim Header Context",
        "icon": "🧩",
        "step_type": "enrichment.script"
    },
    {
        "name": "Build ExplanationOfBenefit",
        "icon": "🧾",
        "step_type": "fhir.build"
    },
    {
        "name": "Build PaymentReconciliation",
        "icon": "💰",
        "step_type": "fhir.build"
    },
    {
        "name": "Assemble FHIR Bundle",
        "icon": "📦",
        "step_type": "payload.builder"
    },
    {
        "name": "Validate FHIR",
        "icon": "🛡️",
        "step_type": "fhir_validation"
    },
    {
        "name": "Store Result",
        "icon": "💾",
        "step_type": "connector.outbound"
    }
]',
    20,
    true,
    true,
    'ezHealthKonnect',
    0
)
ON CONFLICT (slug) DO UPDATE SET
    name                   = EXCLUDED.name,
    description            = EXCLUDED.description,
    pipeline_config        = EXCLUDED.pipeline_config,
    preview_steps          = EXCLUDED.preview_steps,
    tags                   = EXCLUDED.tags,
    updated_at             = NOW();
