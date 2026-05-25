-- V91: Da Vinci Prior Authorization Support (PAS) OOB Pipeline Template
-- Implements CMS-0057-F compliant prior authorization workflow following the
-- Da Vinci PAS IG (http://hl7.org/fhir/us/davinci-pas/STU2/).
--
-- Architecture:
--   Zone 1  (seq 10)       USER MAPPING    — map any inbound format to PAS envelope
--   Zone 2  (seq 100-140)  PAS CORE        — locked OOB, builds Da Vinci FHIR resources
--   Zone 3  (seq 150)      PAYER SUBMIT    — SMART on FHIR OAuth2 + $submit endpoint
--   Zone 4  (seq 200-210)  DECISION ROUTE  — parse ClaimResponse, route AA/AD/CP/PE
--
-- Dependencies: V67 (interface_templates table)

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
    'Da Vinci Prior Authorization (PAS)',
    'davinci-pas-r4',
    'CMS-0057-F compliant prior authorization pipeline following the Da Vinci PAS IG STU2. Accepts any inbound message format — map your fields once in the PAS Envelope step, and the pipeline builds a conformant FHIR R4 Claim bundle, submits to the payer via SMART on FHIR OAuth2, then routes the decision (Approved / Denied / Partial / Pended) back to your system.',
    'regulatory',
    'prior-authorization',
    ARRAY['prior-auth','da-vinci','pas','cms-0057-f','fhir-r4','smart-on-fhir','claim','coverage'],
    '🏥',
    'advanced',

    -- Source: accept PA requests from any HTTP caller (EHR, HIS, etc.)
    'http_rest_inbound',
    '{"path": "/pa-request", "method": "POST", "auth_type": "bearer", "content_type": "application/json"}',

    -- Target: POST decision back to originating system
    'http_outbound',
    '{"method": "POST", "content_type": "application/json", "auth_type": "bearer", "timeout_seconds": 30}',

    -- ──────────────────────────────────────────────────────────────────────────
    -- PIPELINE CONFIG
    -- ──────────────────────────────────────────────────────────────────────────
    '{
    "execution_groups": [
        {
            "sequence": 10,
            "label": "Zone 1 \u2014 User Mapping (YOU edit this)",
            "description": "The only step you need to configure. Map your inbound message fields (rhs) to the standard PAS envelope fields (lhs). Everything below this step is pre-built and locked.",
            "steps": [
                {
                    "step_name": "Map to PAS Envelope",
                    "step_alias": "pas_envelope",
                    "step_type": "pas_envelope_mapping",
                    "sequence": 10,
                    "enabled": true,
                    "required": true,
                    "description": "Map your source fields to the PAS envelope. ''lhs'' is the target PAS path. ''rhs'' is the path in YOUR inbound message (e.g. body.patient.firstName) or a literal value (e.g. \"MEM-12345\"). Required fields must be mapped for the Claim bundle to be valid.",
                    "config": {
                        "_zone": "user_mapping",
                        "_hint": "Set each ''rhs'' to the path in your inbound message. Required fields are marked \u2014 the pipeline will produce an invalid Claim if they are empty.",
                        "mappings": [
                            {
                                "lhs": "_pas_envelope.patient.firstName",
                                "rhs": "",
                                "_label": "Patient First Name",
                                "_required": true,
                                "_hint": "Legal first name as on the insurance card"
                            },
                            {
                                "lhs": "_pas_envelope.patient.lastName",
                                "rhs": "",
                                "_label": "Patient Last Name",
                                "_required": true,
                                "_hint": "Legal last name as on the insurance card"
                            },
                            {
                                "lhs": "_pas_envelope.patient.dob",
                                "rhs": "",
                                "_label": "Date of Birth (YYYY-MM-DD)",
                                "_required": true,
                                "_hint": "ISO format: 1985-03-22"
                            },
                            {
                                "lhs": "_pas_envelope.patient.gender",
                                "rhs": "",
                                "_label": "Gender",
                                "_required": false,
                                "_hint": "male | female | other (defaults to unknown)"
                            },
                            {
                                "lhs": "_pas_envelope.patient.memberId",
                                "rhs": "",
                                "_label": "Payer Member ID",
                                "_required": true,
                                "_hint": "ID from the patient insurance card \u2014 becomes FHIR Patient.id"
                            },
                            {
                                "lhs": "_pas_envelope.coverage.payerId",
                                "rhs": "",
                                "_label": "Payer ID (NPI or OID)",
                                "_required": true,
                                "_hint": "The payer NPI \u2014 must match Claim.insurer"
                            },
                            {
                                "lhs": "_pas_envelope.coverage.planId",
                                "rhs": "",
                                "_label": "Plan ID",
                                "_required": false,
                                "_hint": "e.g. GOLD-PPO \u2014 some payers require this"
                            },
                            {
                                "lhs": "_pas_envelope.coverage.groupNumber",
                                "rhs": "",
                                "_label": "Group Number",
                                "_required": false,
                                "_hint": "Employer group number from the insurance card"
                            },
                            {
                                "lhs": "_pas_envelope.provider.npi",
                                "rhs": "",
                                "_label": "Requesting Provider NPI",
                                "_required": true,
                                "_hint": "Individual clinician 10-digit NPI"
                            },
                            {
                                "lhs": "_pas_envelope.provider.name",
                                "rhs": "",
                                "_label": "Provider Full Name",
                                "_required": false,
                                "_hint": "e.g. Alice Johnson \u2014 split into family/given automatically"
                            },
                            {
                                "lhs": "_pas_envelope.provider.facilityNpi",
                                "rhs": "",
                                "_label": "Facility NPI",
                                "_required": false,
                                "_hint": "Billing organisation NPI (falls back to provider NPI)"
                            },
                            {
                                "lhs": "_pas_envelope.request.serviceCode",
                                "rhs": "",
                                "_label": "CPT / HCPCS Service Code",
                                "_required": true,
                                "_hint": "Procedure code being requested, e.g. 99213"
                            },
                            {
                                "lhs": "_pas_envelope.request.diagnosisCodes",
                                "rhs": "",
                                "_label": "ICD-10 Diagnosis Codes",
                                "_required": true,
                                "_hint": "Array of ICD-10-CM codes, e.g. [\"J18.9\",\"Z00.00\"]"
                            },
                            {
                                "lhs": "_pas_envelope.request.urgency",
                                "rhs": "",
                                "_label": "Urgency",
                                "_required": false,
                                "_hint": "routine (default) or urgent \u2014 sets Claim.priority"
                            },
                            {
                                "lhs": "_pas_envelope.request.serviceStartDate",
                                "rhs": "",
                                "_label": "Requested Service Start Date",
                                "_required": false,
                                "_hint": "ISO date YYYY-MM-DD \u2014 defaults to today"
                            },
                            {
                                "lhs": "_pas_envelope.request.serviceEndDate",
                                "rhs": "",
                                "_label": "Requested Service End Date",
                                "_required": false,
                                "_hint": "ISO date YYYY-MM-DD \u2014 defaults to start date"
                            },
                            {
                                "lhs": "_pas_envelope.request.quantity",
                                "rhs": "",
                                "_label": "Service Quantity",
                                "_required": false,
                                "_hint": "Number of units requested (defaults to 1)"
                            }
                        ]
                    }
                }
            ]
        },
        {
            "sequence": 100,
            "label": "Zone 2 \u2014 Build FHIR Patient",
            "description": "Pre-built and locked. Converts _pas_envelope.patient.* into a FHIR R4 Patient resource conforming to the Da Vinci PAS Subscriber profile. Output available as steps.build_patient.step_output.patient.",
            "steps": [
                {
                    "step_name": "Build FHIR Patient",
                    "step_alias": "build_patient",
                    "step_type": "enrichment.script",
                    "sequence": 100,
                    "enabled": true,
                    "required": true,
                    "description": "Converts _pas_envelope.patient.* into a FHIR R4 Patient resource. Sets Patient.id = memberId. Adds a Member Number (MB) identifier. Normalises gender to FHIR-allowed values (male/female/other/unknown). Profile: Da Vinci PAS Subscriber.",
                    "config": {
                        "_zone": "pas_core",
                        "_locked": true,
                        "script": "// -- Build FHIR Patient (Da Vinci PAS Subscriber profile) --\\n// Reads from : input._pas_envelope.patient.*\\n// Writes to  : steps.build_patient.step_output.patient\\n//\\n// HOW TO USE: Map these fields in ''Map to PAS Envelope'':\\n//   _pas_envelope.patient.firstName  - patient first name       (REQUIRED)\\n//   _pas_envelope.patient.lastName   - patient last name        (REQUIRED)\\n//   _pas_envelope.patient.dob        - date of birth YYYY-MM-DD (REQUIRED)\\n//   _pas_envelope.patient.memberId   - payer member ID          (REQUIRED, becomes FHIR Patient.id)\\n//   _pas_envelope.patient.gender     - male / female / other    (optional)\\n\\nvar env      = input._pas_envelope || {};\\nvar pat      = env.patient || {};\\n\\n// memberId becomes both the FHIR Patient.id and the MB identifier value\\nvar memberId = pat.memberId || (\\\"pat-\\\" + Date.now());\\n\\n// FHIR only accepts: male | female | other | unknown\\nvar gender = (pat.gender || \\\"unknown\\\").toLowerCase();\\nif (gender !== \\\"male\\\" && gender !== \\\"female\\\" && gender !== \\\"other\\\" && gender !== \\\"unknown\\\") {\\n  gender = \\\"unknown\\\";\\n}\\n\\nvar patient = {\\n  resourceType: \\\"Patient\\\",\\n  id:           memberId,\\n\\n  // Da Vinci PAS Subscriber profile URL (required by the IG)\\n  meta: { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-subscriber\\\"] },\\n\\n  // Member Number identifier (MB type) - links back to the payer member roster\\n  identifier: [{\\n    type:   { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/v2-0203\\\", code: \\\"MB\\\", display: \\\"Member Number\\\" }] },\\n    system: \\\"urn:oid:2.16.840.1.113883.4.6\\\",\\n    value:  memberId\\n  }],\\n\\n  // Official legal name as it appears on the insurance card\\n  name:      [{ use: \\\"official\\\", family: pat.lastName || \\\"\\\", given: [pat.firstName || \\\"\\\"] }],\\n  birthDate: pat.dob || \\\"\\\",\\n  gender:    gender\\n};\\n\\nreturn ({ patient: patient });"
                    }
                }
            ]
        },
        {
            "sequence": 110,
            "label": "Zone 2 \u2014 Build FHIR Coverage",
            "description": "Pre-built and locked. Converts _pas_envelope.coverage.* into a FHIR R4 Coverage resource conforming to the Da Vinci PAS Coverage profile. Output: steps.build_coverage.step_output.coverage.",
            "steps": [
                {
                    "step_name": "Build FHIR Coverage",
                    "step_alias": "build_coverage",
                    "step_type": "enrichment.script",
                    "sequence": 110,
                    "enabled": true,
                    "required": true,
                    "description": "Builds the FHIR Coverage resource that links the Patient to the Payer. Subscriber relationship defaults to ''self''. Adds Coverage.class entries for planId and groupNumber when mapped. Profile: Da Vinci PAS Coverage.",
                    "config": {
                        "_zone": "pas_core",
                        "_locked": true,
                        "script": "// -- Build FHIR Coverage (payer insurance card) --\\n// Reads from : input._pas_envelope.coverage.* and .patient.*\\n// Writes to  : steps.build_coverage.step_output.coverage\\n//\\n// HOW TO USE: Map these fields in ''Map to PAS Envelope'':\\n//   _pas_envelope.coverage.payerId      - payer NPI or OID       (REQUIRED)\\n//   _pas_envelope.coverage.planId       - plan ID, e.g. GOLD-PPO (optional)\\n//   _pas_envelope.coverage.groupNumber  - employer group number   (optional)\\n//\\n// Subscriber relationship defaults to ''self'' (patient = subscriber).\\n\\nvar env      = input._pas_envelope || {};\\nvar cov      = env.coverage || {};\\nvar pat      = env.patient  || {};\\nvar memberId = pat.memberId || \\\"patient-1\\\";\\n\\nvar coverage = {\\n  resourceType: \\\"Coverage\\\",\\n  id:           \\\"coverage-1\\\",\\n\\n  // Da Vinci PAS Coverage profile URL\\n  meta:         { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-coverage\\\"] },\\n  status:       \\\"active\\\",\\n\\n  // Subscriber = Patient (self scenario, most common for prior auth)\\n  subscriber:   { reference: \\\"Patient/\\\" + memberId },\\n  subscriberId: memberId,\\n  beneficiary:  { reference: \\\"Patient/\\\" + memberId },\\n  relationship: { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/subscriber-relationship\\\", code: \\\"self\\\" }] },\\n\\n  // Payer identified by NPI - must match Claim.insurer\\n  payor: [{ identifier: { system: \\\"http://hl7.org/fhir/sid/us-npi\\\", value: cov.payerId || \\\"\\\" } }]\\n};\\n\\n// Optional: add plan class (e.g. GOLD-PPO) - required by some payer APIs\\nif (cov.planId) {\\n  coverage.class = [{\\n    type:  { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/coverage-class\\\", code: \\\"plan\\\" }] },\\n    value: cov.planId,\\n    name:  \\\"Health Plan\\\"\\n  }];\\n}\\n\\n// Optional: add employer group number\\nif (cov.groupNumber) {\\n  coverage.class = coverage.class || [];\\n  coverage.class.push({\\n    type:  { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/coverage-class\\\", code: \\\"group\\\" }] },\\n    value: cov.groupNumber\\n  });\\n}\\n\\nreturn ({ coverage: coverage });"
                    }
                }
            ]
        },
        {
            "sequence": 120,
            "label": "Zone 2 \u2014 Build FHIR Provider Resources",
            "description": "Pre-built and locked. Converts _pas_envelope.provider.* into two FHIR R4 resources: Practitioner (ordering clinician) and Organization (billing facility). Output: steps.build_provider.step_output.{practitioner, organization}.",
            "steps": [
                {
                    "step_name": "Build Provider Resources",
                    "step_alias": "build_provider",
                    "step_type": "enrichment.script",
                    "sequence": 120,
                    "enabled": true,
                    "required": true,
                    "description": "Creates two FHIR resources: (1) Practitioner \u2014 the individual ordering clinician, referenced on Claim.item.provider; (2) Organization \u2014 the billing entity, referenced on Claim.provider. facilityNpi falls back to provider NPI when not mapped separately. Provider name is split into family/given automatically.",
                    "config": {
                        "_zone": "pas_core",
                        "_locked": true,
                        "script": "// -- Build Provider Resources (Practitioner + Organization) --\\n// Reads from : input._pas_envelope.provider.*\\n// Writes to  : steps.build_provider.step_output.practitioner\\n//              steps.build_provider.step_output.organization\\n//\\n// HOW TO USE: Map these fields in ''Map to PAS Envelope'':\\n//   _pas_envelope.provider.npi              - requesting clinician NPI  (REQUIRED)\\n//   _pas_envelope.provider.name             - full name e.g. ''Alice Johnson'' (optional)\\n//   _pas_envelope.provider.facilityNpi      - facility NPI (falls back to clinician NPI)\\n//   _pas_envelope.provider.organizationName - org display name (optional)\\n//   _pas_envelope.provider.phone            - org contact phone (optional)\\n//\\n// Claim references Organization as provider (billing entity).\\n// Line item references Practitioner (individual ordering clinician).\\n\\nvar env  = input._pas_envelope || {};\\nvar prov = env.provider || {};\\n\\n// Split ''Alice Johnson'' -> family=''Johnson'', given=[''Alice'']\\nvar nameParts  = (prov.name || \\\"Unknown Provider\\\").split(\\\" \\\");\\nvar lastName   = nameParts.length > 1 ? nameParts[nameParts.length - 1] : nameParts[0];\\nvar firstNames = nameParts.length > 1 ? nameParts.slice(0, nameParts.length - 1) : [];\\n\\n// Practitioner = the individual ordering / requesting clinician\\nvar practitioner = {\\n  resourceType: \\\"Practitioner\\\",\\n  id:           \\\"practitioner-1\\\",\\n  meta:         { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-practitioner\\\"] },\\n  identifier:   [{ system: \\\"http://hl7.org/fhir/sid/us-npi\\\", value: prov.npi || \\\"\\\" }],\\n  name:         [{ family: lastName, given: firstNames }]\\n};\\n\\n// Organization = the requesting facility / billing entity on the Claim\\n// Falls back to clinician NPI when no separate facilityNpi is provided\\nvar facilityNpi = prov.facilityNpi || prov.npi || \\\"\\\";\\n\\nvar organization = {\\n  resourceType: \\\"Organization\\\",\\n  id:           \\\"organization-1\\\",\\n  meta:         { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-requestor\\\"] },\\n  identifier:   [{ system: \\\"http://hl7.org/fhir/sid/us-npi\\\", value: facilityNpi }],\\n  name:         prov.organizationName || prov.name || \\\"Requesting Organization\\\"\\n};\\n\\n// Optional contact phone on the organization\\nif (prov.phone) {\\n  organization.telecom = [{ system: \\\"phone\\\", value: prov.phone, use: \\\"work\\\" }];\\n}\\n\\nreturn ({ practitioner: practitioner, organization: organization });"
                    }
                }
            ]
        },
        {
            "sequence": 130,
            "label": "Zone 2 \u2014 Assemble Da Vinci PAS Request Bundle",
            "description": "Pre-built and locked. Collects all FHIR resources from steps 100-120 and assembles them into a Da Vinci PAS-conformant FHIR R4 Bundle containing Claim + Patient + Coverage + Practitioner + Organization. Output: steps.assemble_pas_bundle.step_output.pas_bundle (this is what gets sent to the payer).",
            "steps": [
                {
                    "step_name": "Assemble PAS Bundle",
                    "step_alias": "assemble_pas_bundle",
                    "step_type": "enrichment.script",
                    "sequence": 130,
                    "enabled": true,
                    "required": true,
                    "description": "Assembles a FHIR R4 collection Bundle with: Claim (use=preauthorization, required by PAS IG), Patient, Coverage, Practitioner, Organization. Sets Claim.priority=stat for urgent requests. Diagnosis codes accept a JS array or JSON-encoded string. To extend the bundle (add SupportingInfo, multiple line items, authRefNumber), add a script step at seq 135 that reads and modifies steps.assemble_pas_bundle.step_output.pas_bundle.",
                    "config": {
                        "_zone": "pas_core",
                        "_locked": true,
                        "script": "// -- Assemble Da Vinci PAS Request Bundle --\\n// Reads from : steps.build_patient / build_coverage / build_provider step outputs\\n//              input._pas_envelope.request.*\\n// Writes to  : steps.assemble_pas_bundle.step_output.pas_bundle  (POSTed to payer)\\n//              steps.assemble_pas_bundle.step_output.claim_id / bundle_id\\n//\\n// HOW TO USE: No edits needed here. The bundle assembles automatically from the\\n// three Build steps above. To add SupportingInfo, authRefNumber, or extra line\\n// items, add a script step AFTER this one that modifies pas_bundle.\\n//\\n// Urgency: _pas_envelope.request.urgency = ''urgent'' -> Claim.priority = stat\\n//          Anything else (or omitted)               -> Claim.priority = normal\\n\\nvar env = input._pas_envelope || {};\\nvar req = env.request || {};\\nvar pat = env.patient || {};\\n\\nvar ts       = new Date().toISOString();\\nvar claimId  = \\\"claim-\\\"      + Date.now();\\nvar bundleId = \\\"pas-bundle-\\\" + Date.now();\\n\\n// -- Pull FHIR resources from the preceding build steps --\\n// Each falls back to a minimal stub if a build step was skipped.\\nvar patient = (input.steps && input.steps.build_patient &&\\n               input.steps.build_patient.step_output &&\\n               input.steps.build_patient.step_output.patient)\\n  ? input.steps.build_patient.step_output.patient\\n  : { resourceType: \\\"Patient\\\", id: pat.memberId || \\\"patient-1\\\" };\\n\\nvar coverage = (input.steps && input.steps.build_coverage &&\\n                input.steps.build_coverage.step_output &&\\n                input.steps.build_coverage.step_output.coverage)\\n  ? input.steps.build_coverage.step_output.coverage\\n  : { resourceType: \\\"Coverage\\\", id: \\\"coverage-1\\\" };\\n\\nvar practitioner = (input.steps && input.steps.build_provider &&\\n                    input.steps.build_provider.step_output &&\\n                    input.steps.build_provider.step_output.practitioner)\\n  ? input.steps.build_provider.step_output.practitioner\\n  : { resourceType: \\\"Practitioner\\\", id: \\\"practitioner-1\\\" };\\n\\nvar organization = (input.steps && input.steps.build_provider &&\\n                    input.steps.build_provider.step_output &&\\n                    input.steps.build_provider.step_output.organization)\\n  ? input.steps.build_provider.step_output.organization\\n  : { resourceType: \\\"Organization\\\", id: \\\"organization-1\\\" };\\n\\n// -- Diagnosis codes -> Claim.diagnosis array --\\n// Accepts a JS array or a JSON-encoded string: [\\\"J18.9\\\", \\\"Z87.891\\\"]\\nvar diagCodes = req.diagnosisCodes || [];\\nif (typeof diagCodes === \\\"string\\\") {\\n  try { diagCodes = JSON.parse(diagCodes); } catch (e) { diagCodes = [diagCodes]; }\\n}\\n\\nvar diagnoses = diagCodes.map(function(code, idx) {\\n  return {\\n    sequence: idx + 1,\\n    diagnosisCodeableConcept: {\\n      coding: [{ system: \\\"http://hl7.org/fhir/sid/icd-10-cm\\\", code: String(code) }]\\n    }\\n  };\\n});\\n\\n// -- Priority and service dates --\\nvar priority    = (req.urgency === \\\"urgent\\\") ? \\\"stat\\\" : \\\"normal\\\";\\nvar serviceDate = req.serviceStartDate || ts.substring(0, 10);\\nvar endDate     = req.serviceEndDate   || serviceDate;\\n\\n// -- Build the Claim --\\n// Claim.use MUST be ''preauthorization'' per the Da Vinci PAS IG.\\n// Enforced by the FHIR Validate step (seq 140).\\nvar claim = {\\n  resourceType: \\\"Claim\\\",\\n  id:           claimId,\\n  meta:         { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim\\\"] },\\n  status:       \\\"active\\\",\\n  type:         { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/claim-type\\\", code: \\\"professional\\\" }] },\\n  use:          \\\"preauthorization\\\",\\n\\n  patient:      { reference: \\\"Patient/\\\" + (pat.memberId || \\\"patient-1\\\") },\\n  created:      ts,\\n\\n  // Insurer = the payer, identified by NPI\\n  insurer:      { identifier: { system: \\\"http://hl7.org/fhir/sid/us-npi\\\", value: (env.coverage || {}).payerId || \\\"\\\" } },\\n\\n  // Provider = requesting organisation (billing entity, not individual clinician)\\n  provider:     { reference: \\\"Organization/organization-1\\\" },\\n\\n  priority:     { coding: [{ system: \\\"http://terminology.hl7.org/CodeSystem/processpriority\\\", code: priority }] },\\n  insurance:    [{ sequence: 1, focal: true, coverage: { reference: \\\"Coverage/coverage-1\\\" } }],\\n  diagnosis:    diagnoses,\\n\\n  // Line item: the service being requested for authorisation\\n  item: [{\\n    sequence: 1,\\n\\n    // PAS extension: the date range the service is requested for\\n    extension: [{\\n      url:         \\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-serviceItemRequestedDate\\\",\\n      valuePeriod: { start: serviceDate, end: endDate }\\n    }],\\n\\n    category:         { coding: [{ system: \\\"https://codesystem.x12.org/005010/1365\\\", code: \\\"1\\\", display: \\\"Medical Care\\\" }] },\\n    productOrService: { coding: [{ system: \\\"http://www.ama-assn.org/go/cpt\\\", code: req.serviceCode || \\\"\\\" }] },\\n    quantity:         { value: req.quantity ? Number(req.quantity) : 1 },\\n    provider:         { reference: \\\"Practitioner/practitioner-1\\\" }\\n  }]\\n};\\n\\n// -- Wrap in a Da Vinci PAS Request Bundle --\\nvar bundle = {\\n  resourceType: \\\"Bundle\\\",\\n  id:           bundleId,\\n  meta:         { profile: [\\\"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle\\\"] },\\n  type:         \\\"collection\\\",\\n  timestamp:    ts,\\n  entry: [\\n    { fullUrl: \\\"urn:uuid:\\\" + claimId,         resource: claim         },\\n    { fullUrl: \\\"urn:uuid:\\\" + patient.id,      resource: patient       },\\n    { fullUrl: \\\"urn:uuid:coverage-1\\\",         resource: coverage      },\\n    { fullUrl: \\\"urn:uuid:practitioner-1\\\",     resource: practitioner  },\\n    { fullUrl: \\\"urn:uuid:organization-1\\\",     resource: organization  }\\n  ]\\n};\\n\\nreturn ({ pas_bundle: bundle, claim_id: claimId, bundle_id: bundleId, created_at: ts });"
                    }
                }
            ]
        },
        {
            "sequence": 140,
            "label": "Zone 2 \u2014 Validate FHIR PAS Bundle",
            "description": "Pre-built. Validates the assembled bundle against Da Vinci PAS R4 profiles before sending to the payer. Logs violations to the audit trail. Non-blocking by default \u2014 set fail_on_error=true to prevent submission of non-conformant bundles.",
            "steps": [
                {
                    "step_name": "Validate PAS Bundle",
                    "step_alias": "validate_pas_bundle",
                    "step_type": "fhir_validation",
                    "sequence": 140,
                    "enabled": true,
                    "required": false,
                    "description": "Runs Da Vinci PAS constraint checks: (1) Claim.use must be ''preauthorization''; (2) Bundle must contain exactly one Claim; (3) Bundle must contain at least one Patient and Coverage. Violations are recorded in the execution audit trail. Set fail_on_error=true to make this step block submission when violations are found.",
                    "config": {
                        "_zone": "pas_core",
                        "profile": "davinci-pas",
                        "level": "standard",
                        "source_field": "steps.assemble_pas_bundle.step_output.pas_bundle",
                        "fail_on_error": false,
                        "required_resource_types": [
                            "Claim",
                            "Patient",
                            "Coverage"
                        ]
                    }
                }
            ]
        },
        {
            "sequence": 150,
            "label": "Zone 3 \u2014 Submit to Payer (SMART on FHIR)",
            "description": "Pre-built. POSTs the assembled PAS Bundle to the payer FHIR server using the $submit operation, authenticated with SMART on FHIR client_credentials OAuth2. Connection details are entered once at template setup time. Includes 2 retries and a circuit breaker.",
            "steps": [
                {
                    "step_name": "Submit to Payer",
                    "step_alias": "submit_to_payer",
                    "step_type": "enrichment.api",
                    "sequence": 150,
                    "enabled": true,
                    "required": true,
                    "description": "POSTs the PAS Bundle to {payer_fhir_base}/$submit. Obtains a bearer token from {payer_token_url} using OAuth2 client_credentials with scope ''system/Claim.write system/ClaimResponse.read''. Timeout: 30 s. Retry: 2x with 2 s delay. Circuit breaker: opens after 5 failures for 60 s. The raw ClaimResponse body is stored in steps.submit_to_payer.step_output and consumed by the Parse Claim Response step.",
                    "config": {
                        "_zone": "pas_core",
                        "_hint": "Enter payer_fhir_base, payer_token_url, payer_client_id, payer_client_secret in the connection fields at template-use time.",
                        "method": "POST",
                        "endpoint": "$${_payer_fhir_base}/$submit",
                        "authType": "oauth2",
                        "oauth2GrantType": "client_credentials",
                        "oauth2TokenUrl": "$${_payer_token_url}",
                        "oauth2ClientId": "$${_payer_client_id}",
                        "oauth2ClientSecret": "$${_payer_client_secret}",
                        "oauth2Scope": "system/Claim.write system/ClaimResponse.read",
                        "headers": {
                            "Content-Type": "application/fhir+json",
                            "Accept": "application/fhir+json"
                        },
                        "bodyRef": "steps.assemble_pas_bundle.step_output.pas_bundle",
                        "timeoutMs": 30000,
                        "retryCount": 2,
                        "retryDelayMs": 2000,
                        "failureThreshold": 5,
                        "openDurationSeconds": 60
                    }
                }
            ]
        },
        {
            "sequence": 200,
            "label": "Zone 4 \u2014 Parse ClaimResponse",
            "description": "Pre-built and locked. Extracts the Da Vinci PAS review action code from the payer ClaimResponse and maps it to a plain decision label. Output: steps.parse_claim_response.step_output.decision.",
            "steps": [
                {
                    "step_name": "Parse Claim Response",
                    "step_alias": "parse_claim_response",
                    "step_type": "enrichment.script",
                    "sequence": 200,
                    "enabled": true,
                    "required": true,
                    "description": "Reads ClaimResponse.item[0].extension[reviewAction] and maps X12 005010/306 codes: A1=AA (Approved), A2=CP (Certified Partial), A3/A4=AD (Denied), A6=PE (Pended). Falls back to ClaimResponse.outcome for payers that do not use PAS extensions. Denial reason text is extracted and available as denial_reason. All outputs available as steps.parse_claim_response.step_output.*",
                    "config": {
                        "_zone": "pas_core",
                        "_locked": true,
                        "script": "// -- Parse Da Vinci PAS ClaimResponse --\\n// Reads from : steps.submit_to_payer.step_output (payer HTTP response body)\\n// Writes to  : steps.parse_claim_response.step_output.decision\\n//              steps.parse_claim_response.step_output.review_action\\n//              steps.parse_claim_response.step_output.denial_reason\\n//              steps.parse_claim_response.step_output.raw_response\\n//\\n// Da Vinci PAS review action codes (X12 005010/306):\\n//   A1 -> AA   Approved - all items approved as submitted\\n//   A2 -> CP   Certified Partial - some items approved, some not\\n//   A3 -> AD   Denied - not medically necessary or plan exclusion\\n//   A4 -> AD   Denied (modified) - payer changed qty/dates, treated as denial\\n//   A6 -> PE   Pended - payer needs more time or additional information\\n//\\n// ''Route on Decision'' step (seq 210) branches on the decision value.\\n\\n// Pull the payer ClaimResponse from the API step output\\nvar apiResp = (input.steps && input.steps.submit_to_payer &&\\n               input.steps.submit_to_payer.step_output)\\n  ? input.steps.submit_to_payer.step_output\\n  : {};\\n\\n// API executor wraps the parsed response body under .response;\\n// fall back to root of the step output for direct assignments.\\nvar claimResp = apiResp.response || apiResp;\\n\\nvar decision     = \\\"unknown\\\";\\nvar reviewAction = \\\"\\\";\\nvar denialReason = \\\"\\\";\\nvar reviewUrl    = \\\"\\\";\\n\\n// -- Walk ClaimResponse.item[0].extension to find the reviewAction --\\nif (claimResp && claimResp.item && claimResp.item.length > 0) {\\n  var firstItem = claimResp.item[0];\\n\\n  if (firstItem.extension) {\\n    for (var i = 0; i < firstItem.extension.length; i++) {\\n      var ext = firstItem.extension[i];\\n\\n      // reviewAction extension contains sub-extensions: code and reasonCode\\n      if (ext.url && ext.url.indexOf(\\\"reviewAction\\\") !== -1 && ext.extension) {\\n        for (var j = 0; j < ext.extension.length; j++) {\\n          var sub = ext.extension[j];\\n\\n          // ''code'' -> X12 review action code (A1/A2/A3/A4/A6)\\n          if (sub.url === \\\"code\\\" &&\\n              sub.valueCodeableConcept &&\\n              sub.valueCodeableConcept.coding &&\\n              sub.valueCodeableConcept.coding.length > 0) {\\n            reviewAction = sub.valueCodeableConcept.coding[0].code || \\\"\\\";\\n          }\\n\\n          // ''reasonCode'' -> denial reason text (present for AD decisions)\\n          if (sub.url === \\\"reasonCode\\\" &&\\n              sub.valueCodeableConcept &&\\n              sub.valueCodeableConcept.coding &&\\n              sub.valueCodeableConcept.coding.length > 0) {\\n            denialReason = sub.valueCodeableConcept.coding[0].display ||\\n                           sub.valueCodeableConcept.coding[0].code    || \\\"\\\";\\n          }\\n        }\\n      }\\n    }\\n  }\\n}\\n\\n// -- Map X12 action code -> friendly decision label --\\nif      (reviewAction === \\\"A1\\\")                          { decision = \\\"AA\\\"; }\\nelse if (reviewAction === \\\"A3\\\" || reviewAction === \\\"A4\\\") { decision = \\\"AD\\\"; }\\nelse if (reviewAction === \\\"A2\\\")                          { decision = \\\"CP\\\"; }\\nelse if (reviewAction === \\\"A6\\\")                          { decision = \\\"PE\\\"; }\\n\\n// Fallback: some payers use ClaimResponse.outcome instead of PAS extensions\\nelse if (claimResp && claimResp.outcome) {\\n  if      (claimResp.outcome === \\\"complete\\\") { decision = \\\"AA\\\"; }\\n  else if (claimResp.outcome === \\\"error\\\")    { decision = \\\"AD\\\"; }\\n  else if (claimResp.outcome === \\\"partial\\\")  { decision = \\\"CP\\\"; }\\n}\\n\\nvar claimResponseId = claimResp ? (claimResp.id || \\\"\\\") : \\\"\\\";\\n\\nreturn ({\\n  decision:          decision,          // AA | AD | CP | PE | unknown\\n  review_action:     reviewAction,      // raw X12 code: A1/A2/A3/A4/A6\\n  denial_reason:     denialReason,      // populated for AD decisions\\n  review_url:        reviewUrl,         // populated if payer returns review URL\\n  claim_response_id: claimResponseId,   // ClaimResponse.id from payer\\n  raw_response:      claimResp          // full ClaimResponse for downstream steps\\n});"
                    }
                }
            ]
        },
        {
            "sequence": 210,
            "label": "Zone 4 \u2014 Route on Decision (AA / AD / CP / PE)",
            "description": "Pre-built. Switch/Case on the decision value from the Parse step. Sets _pa_status and _pa_status_label pipeline variables. Add your own actions to each case (route_to_step, send_notification, call webhook) in the pipeline builder.",
            "steps": [
                {
                    "step_name": "Route on Decision",
                    "step_alias": "route_on_decision",
                    "step_type": "switch_case",
                    "sequence": 210,
                    "enabled": true,
                    "required": true,
                    "description": "Branches on steps.parse_claim_response.step_output.decision. Each branch sets two variables: _pa_status (approved | denied | partial | pended | unknown) and _pa_status_label (human-readable string). To drive downstream workflow, add route_to_step or send_notification actions to each case \u2014 or add new steps after this one that read _pa_status.",
                    "config": {
                        "_zone": "pas_core",
                        "field": "steps.parse_claim_response.step_output.decision",
                        "cases": [
                            {
                                "value": "AA",
                                "label": "Approved (AA)",
                                "description": "All service items approved as submitted. X12 A1. Proceed with scheduling or notifying the patient.",
                                "actions": [
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status",
                                        "value": "approved"
                                    },
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status_label",
                                        "value": "Prior Authorization Approved"
                                    }
                                ]
                            },
                            {
                                "value": "AD",
                                "label": "Denied (AD)",
                                "description": "Request denied (A3) or approved with payer modifications treated as denial (A4). Check steps.parse_claim_response.step_output.denial_reason for the reason code.",
                                "actions": [
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status",
                                        "value": "denied"
                                    },
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status_label",
                                        "value": "Prior Authorization Denied"
                                    }
                                ]
                            },
                            {
                                "value": "CP",
                                "label": "Partial Approval (CP)",
                                "description": "Some service items approved, others not. X12 A2 (Certified Partial). Review which items were approved in the raw_response.",
                                "actions": [
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status",
                                        "value": "partial"
                                    },
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status_label",
                                        "value": "Prior Authorization Partially Approved"
                                    }
                                ]
                            },
                            {
                                "value": "PE",
                                "label": "Pended / Deferred (PE)",
                                "description": "Payer needs more time or additional information before deciding. X12 A6. Notify patient and provider; poll or await payer callback.",
                                "actions": [
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status",
                                        "value": "pended"
                                    },
                                    {
                                        "action": "set_variable",
                                        "variable": "_pa_status_label",
                                        "value": "Prior Authorization Pended \u2014 Awaiting Payer Review"
                                    }
                                ]
                            }
                        ],
                        "default": {
                            "label": "Unknown Decision",
                            "description": "Payer returned a ClaimResponse without a recognised review action code or outcome. Inspect steps.parse_claim_response.step_output.raw_response for details.",
                            "actions": [
                                {
                                    "action": "set_variable",
                                    "variable": "_pa_status",
                                    "value": "unknown"
                                },
                                {
                                    "action": "set_variable",
                                    "variable": "_pa_status_label",
                                    "value": "Prior Authorization Decision Unknown"
                                }
                            ]
                        }
                    }
                }
            ]
        }
    ]
}',

    -- Message type: any (this template is format-agnostic for inbound)
    null,

    -- Required connection fields (shown to user at template-use time)
    '[
        {"section": "source", "field": "host",               "label": "Inbound Host / IP",             "type": "string",   "hint": "IP or hostname that callers will connect to",                    "required": true},
        {"section": "source", "field": "port",               "label": "Inbound Port",                  "type": "number",   "hint": "Default: 8443",                                                  "required": false},
        {"section": "source", "field": "token",              "label": "Inbound Bearer Token",           "type": "password", "hint": "Token callers must send to authenticate",                        "required": false},
        {"section": "payer",  "field": "payer_fhir_base",    "label": "Payer FHIR Base URL",            "type": "url",      "hint": "e.g. https://fhir.uhc.com/R4  (do not include $submit)",        "required": true},
        {"section": "payer",  "field": "payer_token_url",    "label": "Payer SMART Token URL",          "type": "url",      "hint": "OAuth2 token endpoint, e.g. https://auth.uhc.com/oauth2/token", "required": true},
        {"section": "payer",  "field": "payer_client_id",    "label": "Client ID",                     "type": "string",   "hint": "SMART on FHIR client_credentials client ID",                    "required": true},
        {"section": "payer",  "field": "payer_client_secret","label": "Client Secret",                 "type": "password", "hint": "SMART on FHIR client secret — stored encrypted",                "required": true},
        {"section": "target", "field": "endpoint",           "label": "Decision Callback URL",         "type": "url",      "hint": "Where to POST the PA decision (your EHR or HIS endpoint)",      "required": true},
        {"section": "target", "field": "token",              "label": "Callback Bearer Token",         "type": "password", "hint": "Bearer token for the callback endpoint",                         "required": false}
    ]',

    -- Sanitized fields (credentials scrubbed before storing)
    ARRAY[
        'source.token',
        'payer.payer_client_secret',
        'target.token'
    ],

    -- Preview steps shown on the template card
    '[
        {"name": "Map to PAS Envelope",    "icon": "🗺️",  "step_type": "field_mapping"},
        {"name": "Build FHIR Resources",   "icon": "🏗️",  "step_type": "enrichment.script"},
        {"name": "Assemble PAS Bundle",    "icon": "📦",  "step_type": "enrichment.script"},
        {"name": "Validate FHIR R4",       "icon": "✅",  "step_type": "fhir_validation"},
        {"name": "Submit via SMART OAuth", "icon": "🔐",  "step_type": "enrichment.api"},
        {"name": "Route AA/AD/CP/PE",      "icon": "🔀",  "step_type": "switch_case"}
    ]',

    45, true, true, 'ezHealthKonnect', 0
)
ON CONFLICT (slug) DO NOTHING;
