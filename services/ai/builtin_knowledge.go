// services/ai/builtin_knowledge.go
// Static healthcare standards knowledge + app documentation embedded in the binary.
// This covers X12 835/837/270/271, CCD/CDA, HL7 core segments, FHIR R4 resources,
// and ezHealthKonnect pipeline/connector behaviour (no source code).
// Each entry becomes one or more vector-store chunks.
package ai

type kbEntry struct {
	ref     string
	format  string
	content string
}

// ─── X12 Knowledge Base ───────────────────────────────────────────────────────

var x12KnowledgeBase = []kbEntry{

	// ── X12 835 Overview ──────────────────────────────────────────────────────
	{
		ref:    "X12_835_Overview",
		format: "835",
		content: `X12 835 Health Care Claim Payment/Advice (version 005010X221A1)

The 835 is sent by a health plan (payer) to a provider or clearinghouse to:
  - Make electronic funds transfer (EFT) payment
  - Provide an Explanation of Benefits (EOB) remittance advice
  - Or both together

Key Loops (hierarchical groupings):
  Loop 1000A  Payer Identification (N1*PR)
  Loop 1000B  Payee Identification (N1*PE)
  Loop 2000   Header Number / Claim Group
  Loop 2100   Claim Payment Information — one per claim (CLP segment)
  Loop 2110   Service Payment Information — one per service line (SVC segment)

Envelope Segments:
  ISA  Interchange Control Header
  GS   Functional Group Header
  ST   Transaction Set Header (ST*835)
  BPR  Financial Information — payment amount, date, payment method
  TRN  Reassociation Trace Number — links payment to remittance
  DTM  Production Date
  SE   Transaction Set Trailer
  GE   Functional Group Trailer
  IEA  Interchange Control Trailer

FHIR Mapping Target: ClaimResponse resource`,
	},

	// ── X12 835 Key Segments ──────────────────────────────────────────────────
	{
		ref:    "X12_835_BPR_TRN",
		format: "835",
		content: `X12 835 — BPR and TRN Segments

BPR (Financial Information):
  BPR01  Transaction Handling Code: C=Payment+Remit, D=Remit Only, I=No Payment
  BPR02  Total Actual Provider Payment Amount (monetary value)
  BPR03  Credit/Debit Flag: C=Credit, D=Debit
  BPR04  Payment Method: ACH=Electronic, CHK=Check, NON=No Payment
  BPR16  Payment Date (CCYYMMDD)

FHIR Mapping:
  BPR02 → ClaimResponse.payment.amount.value
  BPR16 → ClaimResponse.payment.date
  BPR04 → ClaimResponse.payment.type.coding.code

TRN (Reassociation Trace Number):
  TRN01  Trace Type Code: 1=Current Transaction, 2=Referenced Transaction
  TRN02  Check/EFT Trace Number (unique payment identifier)
  TRN03  Originating Company Identifier (payer bank routing)

FHIR Mapping:
  TRN02 → ClaimResponse.payment.identifier.value`,
	},

	// ── X12 835 CLP Segment ───────────────────────────────────────────────────
	{
		ref:    "X12_835_CLP_Loop2100",
		format: "835",
		content: `X12 835 — Loop 2100: Claim Payment Information (CLP Segment)

One CLP loop per claim in the remittance. Contains the adjudication result.

CLP Segment Elements:
  CLP01  Claim Submitter's Identifier (patient account number or claim ID from the 837)
  CLP02  Claim Status Code:
           1 = Processed as Primary
           2 = Processed as Secondary
           4 = Denied
           19 = Processed as Primary, Forwarded to Additional Payer(s)
           22 = Reversal of Previous Payment
  CLP03  Total Charge Amount (billed amount from the 837)
  CLP04  Claim Payment Amount (amount paid by payer)
  CLP05  Patient Responsibility Amount (patient owes this amount)
  CLP06  Claim Filing Indicator Code: MB=Medicare Part B, MC=Medicaid, BL=Blue Cross, CI=Commercial
  CLP07  Payer Claim Control Number (payer's internal claim ID)
  CLP08  Facility Type Code
  CLP09  Claim Frequency Type Code

FHIR Mapping:
  CLP01 → ClaimResponse.identifier (use / Claim.identifier lookup)
  CLP02 → ClaimResponse.outcome (complete/partial/error/queued)
  CLP03 → Claim.total.value (billed)
  CLP04 → ClaimResponse.payment.amount.value
  CLP05 → ClaimResponse.total[patientresponsibility].amount.value
  CLP07 → ClaimResponse.identifier (payer)`,
	},

	// ── X12 835 CAS Segment ───────────────────────────────────────────────────
	{
		ref:    "X12_835_CAS_Adjustments",
		format: "835",
		content: `X12 835 — CAS (Claim Adjustment) Segment

CAS explains WHY the payer did not pay the full billed amount.
One CAS can contain up to 6 adjustment reason groups (triplets).

CAS Segment Structure (repeating triplets):
  CAS01  Claim Adjustment Group Code:
           CO = Contractual Obligations (payer/provider contract)
           CR = Corrections and Reversals
           OA = Other Adjustments
           PI = Payer Initiated Reductions
           PR = Patient Responsibility
  CAS02  Claim Adjustment Reason Code (CARC)
  CAS03  Monetary Amount of Adjustment
  (repeats up to 6 triplets per CAS segment)

Common CARC Codes:
  1   = Deductible amount
  2   = Coinsurance amount
  3   = Co-payment amount
  45  = Charge exceeds fee schedule/maximum allowable
  97  = Payment is included in the allowance for another service/procedure
  96  = Non-covered charge(s)
  CO45 = Most common — means payer contracted rate is lower than billed amount

FHIR Mapping:
  CAS Group + CARC → ClaimResponse.item.adjudication.category + ClaimResponse.item.adjudication.reason
  CAS03 (amount) → ClaimResponse.item.adjudication.amount.value
  PR group → ClaimResponse.total[patientresponsibility]`,
	},

	// ── X12 835 SVC Segment ───────────────────────────────────────────────────
	{
		ref:    "X12_835_SVC_Loop2110",
		format: "835",
		content: `X12 835 — Loop 2110: Service Payment Information (SVC Segment)

One SVC loop per service line (procedure) within a claim.

SVC Segment Elements:
  SVC01  Composite Medical Procedure Identifier:
           SVC01-1 = Procedure Code Qualifier (HC=HCPCS/CPT, AD=ADA, NU=NDC)
           SVC01-2 = Procedure Code (e.g., CPT 99213)
           SVC01-3 = Procedure Modifier (optional, e.g., 25, GT)
  SVC02  Line Item Charge Amount (billed for this service)
  SVC03  Line Item Provider Payment Amount (paid for this service)
  SVC04  National Uniform Billing Committee Revenue Code
  SVC05  Units of Service Paid Count
  SVC06  Original Units of Service Count

Associated segments in Loop 2110:
  DTM*472  Service Date
  CAS      Service-level adjustments (same structure as claim-level CAS)
  AMT      Line-level amounts (AMT*B6 = Allowed Amount, AMT*KH = Deductible)
  QTY      Quantity information

FHIR Mapping:
  SVC01-2 → ClaimResponse.item.adjudication / ExplanationOfBenefit.item.productOrService.coding.code
  SVC02 → ClaimResponse.item.adjudication[submitted].amount.value
  SVC03 → ClaimResponse.item.adjudication[benefit].amount.value`,
	},

	// ── X12 837P Overview ─────────────────────────────────────────────────────
	{
		ref:    "X12_837P_Overview",
		format: "837",
		content: `X12 837P Health Care Claim: Professional (version 005010X222A2)

The 837P is submitted by a provider (physician, clinic) to a payer for professional services.

Key Loops:
  Loop 1000A  Submitter Name
  Loop 1000B  Receiver Name
  Loop 2000A  Billing Provider Hierarchical Level (HL)
  Loop 2000B  Subscriber Hierarchical Level
  Loop 2000C  Patient Hierarchical Level (when patient ≠ subscriber)
  Loop 2010AA Billing Provider Name (NM1*85)
  Loop 2010BA Subscriber Name (NM1*IL)
  Loop 2010CA Patient Name (NM1*QC)
  Loop 2300   Claim Information (CLM segment) — one per claim
  Loop 2310   Referring/Rendering/Service Facility Provider
  Loop 2320   Other Subscriber Information (COB scenarios)
  Loop 2400   Service Line Information (LX + SV1) — one per procedure

Key Segments:
  CLM  Claim Information (claim ID, total amount, place of service, claim type)
  SV1  Professional Service (CPT/HCPCS code, charge, units, modifiers)
  DTP  Date of Service
  NM1  Individual/Organizational Name (many loops)
  REF  Reference Identification (NPI, EIN, claim number)
  HI   Health Care Diagnosis Codes (ICD-10-CM)

FHIR Mapping Target: Claim resource (with referenced Patient, Practitioner, Organization)`,
	},

	// ── X12 837P CLM Segment ──────────────────────────────────────────────────
	{
		ref:    "X12_837P_CLM_SV1",
		format: "837",
		content: `X12 837P — CLM and SV1 Segments

CLM (Claim Information) — one per claim:
  CLM01  Patient Account Number / Claim ID (provider-assigned)
  CLM02  Total Claim Charge Amount
  CLM03  (not used)
  CLM04  (not used)
  CLM05  Health Care Service Location Information:
           CLM05-1 = Place of Service Code (11=Office, 21=Inpatient Hospital, 22=Outpatient, 23=ER)
           CLM05-2 = Facility Code Qualifier
           CLM05-3 = Claim Frequency Type Code (1=Original, 7=Replacement, 8=Void)
  CLM06  Provider or Supplier Signature Indicator (Y/N)
  CLM07  Medicare Assignment Code
  CLM08  Benefits Assignment Certification Indicator (Y/N)
  CLM09  Release of Information Code (Y=signed release on file)

FHIR Mapping:
  CLM01 → Claim.identifier
  CLM02 → Claim.total.value
  CLM05-1 → Claim.facility.type or Claim.locationCodeableConcept

SV1 (Professional Service) — one per service line:
  SV101  Composite Medical Procedure Identifier:
           SV101-1 = Procedure Code Qualifier (HC=HCPCS/CPT)
           SV101-2 = Procedure Code (e.g., 99213)
           SV101-3..6 = Modifiers (e.g., 25, GT, 59)
  SV102  Line Item Charge Amount
  SV103  Unit/Basis for Measurement Code (UN=Unit, MJ=Minutes)
  SV104  Service Unit Count (quantity)
  SV107  Composite Diagnosis Code Pointer (links to HI segment)

FHIR Mapping:
  SV101-2 → Claim.item.productOrService.coding.code
  SV101-3..6 → Claim.item.modifier[].coding.code
  SV102 → Claim.item.unitPrice.value
  SV104 → Claim.item.quantity.value`,
	},

	// ── X12 837P NPI and Diagnosis ────────────────────────────────────────────
	{
		ref:    "X12_837P_NPI_HI",
		format: "837",
		content: `X12 837P — NPI, REF, and HI (Diagnosis) Segments

NM1 (Individual/Organizational Name) — used in many loops:
  NM101  Entity Identifier Code:
           85 = Billing Provider
           87 = Pay-to Provider
           82 = Rendering Provider
           77 = Service Location
           IL = Insured (subscriber)
           QC = Patient
  NM102  Entity Type: 1=Person, 2=Non-Person Entity
  NM103  Last Name / Organization Name
  NM104  First Name
  NM108  ID Code Qualifier: XX=NPI, ZZ=Mutually Defined, MI=Member ID
  NM109  ID Code (the actual NPI, member ID, etc.)

FHIR Mapping:
  NM1*85 NM109 (NPI) → Claim.provider.identifier (NPI system)
  NM1*IL NM109 → Claim.insurance.subscriber.identifier (member ID)
  NM1*QC → Claim.patient reference (Patient resource)

REF (Reference Identification):
  REF01  Reference ID Qualifier: EI=EIN, SY=SSN, 1G=Provider Site Number, G2=Provider Commercial Number
  REF02  Reference Identification (the actual value)

HI (Health Care Diagnosis Codes):
  HI01  Principal Diagnosis:
           HI01-1 = ABK (ICD-10-CM) or BK (ICD-9-CM)
           HI01-2 = ICD-10 code (e.g., Z00.00, J06.9)
  HI02..12 = Additional diagnoses (same structure)

FHIR Mapping:
  HI01-2 → Claim.diagnosis[0].diagnosisCodeableConcept (ICD-10 system)
  HI02-2..HI12-2 → Claim.diagnosis[1..] (secondary diagnoses)`,
	},

	// ── X12 270/271 Eligibility ───────────────────────────────────────────────
	{
		ref:    "X12_270_271_Eligibility",
		format: "270/271",
		content: `X12 270/271 Health Care Eligibility Benefit Inquiry and Response

270 = Eligibility Inquiry (provider asks payer "is this patient covered?")
271 = Eligibility Response (payer answers with benefit details)

Key Loops in 270:
  Loop 2000A  Information Source (Payer)
  Loop 2000B  Information Receiver (Provider)
  Loop 2000C  Subscriber
  Loop 2000D  Dependent (if patient ≠ subscriber)

Key Segments:
  BHT  Beginning of Hierarchical Transaction
  HL   Hierarchical Level
  NM1  Name (payer, provider, subscriber, dependent)
  TRN  Trace Number (links inquiry to response)
  EQ   Eligibility or Benefit Inquiry (what benefit to check: 30=Health Benefit Plan, 1=Medical)
  DMG  Demographic Information (DOB, gender)
  DTP  Date of Service (planned service date)

271 Response adds:
  EB   Eligibility or Benefit Information:
         EB01 = Benefit Information Code (1=Active Coverage, 6=Deductible, C=Copayment, A=Co-Insurance)
         EB02 = Coverage Level Code (FAM=Family, IND=Individual, EMP=Employee)
         EB03 = Service Type Code (30=Health Benefit Plan, 98=Professional Physician Visit Office)
         EB04 = Insurance Type Code (HM=HMO, PP=PPO, MC=Medicaid, MB=Medicare Part B)
         EB06 = Monetary Amount (deductible, OOP max, etc.)
         EB07 = Percentage (coinsurance rate, e.g., 20 = 20%)
  MSG  Message Text (free-text notes from payer)

FHIR Mapping Target: CoverageEligibilityRequest / CoverageEligibilityResponse`,
	},
}

// ─── CCD/CDA Knowledge Base ───────────────────────────────────────────────────

var ccdKnowledgeBase = []kbEntry{

	{
		ref: "CCD_Overview",
		content: `CCD (Continuity of Care Document) — C-CDA R2.1

CCD is an HL7 CDA (Clinical Document Architecture) XML document that summarizes
a patient's health information for care transitions and information exchange.

Standard: HL7 CDA R2 / Consolidated CDA (C-CDA) R2.1
XML Namespace: urn:hl7-org:v3

Document Structure:
  ClinicalDocument  Root element
    realmCode         US realm
    typeId            Document type (CCD LOINC: 34133-9)
    templateId        C-CDA template OID
    id                Document unique identifier
    code              Document type code (LOINC 34133-9 = Summarization of Episode Note)
    title             Human-readable title
    effectiveTime     Document creation time
    recordTarget      Patient (patientRole → patient)
    author            Document author (assignedAuthor → assignedPerson)
    custodian         Organization responsible (assignedCustodian)
    component
      structuredBody
        component → section (one per clinical domain)

FHIR Mapping Target: Composition resource (with section references)
  ClinicalDocument.id → Composition.identifier
  recordTarget → Composition.subject (Patient reference)
  effectiveTime → Composition.date`,
	},

	{
		ref: "CCD_Sections_LOINC",
		content: `CCD/C-CDA — Standard Section Codes (LOINC)

Each section is identified by a LOINC code and a templateId (OID).

Section LOINC Codes → FHIR AllergyIntolerance / Condition / MedicationStatement / etc.:

  10160-0  History of Medication Use (Medications)
             templateId: 2.16.840.1.113883.10.20.22.2.1.1
             FHIR: MedicationStatement or MedicationRequest
             Key entry: substanceAdministration → medication (RxNorm code)

  48765-2  Allergies and Adverse Reactions
             templateId: 2.16.840.1.113883.10.20.22.2.6.1
             FHIR: AllergyIntolerance
             Key entry: observation → value (allergen), entryRelationship → reaction

  11450-4  Problem List
             templateId: 2.16.840.1.113883.10.20.22.2.5.1
             FHIR: Condition
             Key entry: observation → value (ICD-10 or SNOMED code), effectiveTime (onset)

  30954-2  Relevant Diagnostic Tests / Laboratory Data
             templateId: 2.16.840.1.113883.10.20.22.2.3.1
             FHIR: DiagnosticReport + Observation
             Key entry: observation → code (LOINC), value, referenceRange

  47519-4  History of Procedures
             templateId: 2.16.840.1.113883.10.20.22.2.7.1
             FHIR: Procedure
             Key entry: procedure → code (CPT/SNOMED), effectiveTime

  46240-8  History of Hospitalizations + Outpatient Visits
             templateId: 2.16.840.1.113883.10.20.22.2.22
             FHIR: Encounter

  18776-5  Plan of Treatment
             templateId: 2.16.840.1.113883.10.20.22.2.10
             FHIR: CarePlan

  29762-2  Social History
             templateId: 2.16.840.1.113883.10.20.22.2.17
             FHIR: Observation (social-history category)

  8716-3   Vital Signs
             templateId: 2.16.840.1.113883.10.20.22.2.4.1
             FHIR: Observation (vital-signs category)
             Key LOINC codes: 8302-2=Height, 29463-7=Weight, 8480-6=BP Systolic, 8462-4=BP Diastolic`,
	},

	{
		ref: "CCD_Patient_Demographics",
		content: `CCD/C-CDA — Patient Demographics (recordTarget)

XML Path: ClinicalDocument/recordTarget/patientRole

  patientRole/id        Patient identifier (root=OID, extension=MRN or SSN)
  patientRole/addr      Address (streetAddressLine, city, state, postalCode, country)
  patientRole/telecom   Phone/email (use=HP for home phone, WP for work, MC for mobile)
  patientRole/patient/name
    family              Last name
    given               First name (may repeat for middle)
    suffix              Jr., Sr., etc.
  patientRole/patient/administrativeGenderCode
    code                M=Male, F=Female, UN=Undifferentiated
    codeSystem          2.16.840.1.113883.5.1 (HL7 AdministrativeGender)
  patientRole/patient/birthTime
    value               YYYYMMDD or YYYYMMDDHHmmss
  patientRole/patient/raceCode
    code                CDC race code (2054-5=Black or African American, 2106-3=White)
  patientRole/patient/ethnicGroupCode
    code                2135-2=Hispanic or Latino, 2186-5=Not Hispanic or Latino
  patientRole/patient/languageCommunication/languageCode
    code                BCP 47 code (en, es, zh)

FHIR Mapping → Patient resource:
  name → Patient.name (HumanName)
  birthTime → Patient.birthDate
  administrativeGenderCode → Patient.gender
  addr → Patient.address
  telecom → Patient.telecom
  id/@extension → Patient.identifier.value`,
	},

	{
		ref: "CCD_Medications_Entry",
		content: `CCD/C-CDA — Medications Section Entry Structure

XML Path: section[LOINC:10160-0]/entry/substanceAdministration

  substanceAdministration
    @classCode="SBADM"
    @moodCode="EVN" (event) or "INT" (intended)
    templateId  2.16.840.1.113883.10.20.22.4.16 (Medication Activity)
    id          Entry identifier
    statusCode/@code  active | completed | aborted | suspended
    effectiveTime
      low/@value   Start date (YYYYMMDD)
      high/@value  Stop date (null if ongoing, use nullFlavor="UNK")
    repeatNumber  Refills allowed
    doseQuantity/@value  Dose amount
    doseQuantity/@unit   UCUM unit (mg, mL, etc.)
    rateQuantity  Infusion rate (for IV meds)
    routeCode/@code  Route of administration:
                     C38288=Oral, C38276=Intravenous, C38305=Topical
                     (codeSystem: FDA NCI Thesaurus 2.16.840.1.113883.3.26.1.1)
    consumable/manufacturedProduct/manufacturedMaterial
      code/@code        RxNorm concept code
      code/@displayName Drug name (e.g., "Metformin 500 MG Oral Tablet")
      code/@codeSystem  2.16.840.1.113883.6.88 (RxNorm)

FHIR Mapping → MedicationStatement or MedicationRequest:
  statusCode → MedicationStatement.status
  effectiveTime → MedicationStatement.effectivePeriod
  code (RxNorm) → MedicationStatement.medication.coding (system: http://www.nlm.nih.gov/research/umls/rxnorm)
  doseQuantity → MedicationStatement.dosage.doseAndRate.doseQuantity
  routeCode → MedicationStatement.dosage.route`,
	},
}

// ─── ezHealthKonnect App Documentation Knowledge ──────────────────────────────
// Describes HOW the system works, not HOW it is implemented.
// Safe to expose to users via RAG — no source code, no implementation details.

var appBuiltinKnowledge = []kbEntry{

	// ── Pipeline Architecture ─────────────────────────────────────────────────
	{
		ref: "EHK_Pipeline_Architecture",
		content: `ezHealthKonnect Transformation Pipeline Architecture

A Transformation Pipeline processes messages for one Interface + Message Type combination.
Each pipeline contains ordered Steps that execute sequentially (or in parallel groups).

Pipeline Lifecycle:
  1. Message received by Inbound Connector (e.g. TCP/MLLP, HTTP, SFTP, Kafka)
  2. Raw message stored in PostgreSQL + MongoDB
  3. JSON conversion step: HL7 parsed to enhancedSegments structure
  4. Transformation Pipeline executes steps in sequence order
  5. Output delivered by Outbound Connector (e.g. TCP/MLLP, HTTP/FHIR, PostgreSQL)

Step Sequence Numbers:
  Seq   5:  connector.inbound  — listens for messages (auto-added by wizard)
  Seq  10–99:  pre-processing steps (validation, enrichment, conditional logic)
  Seq 100–199: core mapping steps (HL7→FHIR field mapping, format conversion)
  Seq 200–294: post-processing steps (FHIR validation, data masking, deduplication)
  Seq 295:  connector.outbound — delivers the transformed message (auto-added by wizard)

Pipeline Builder UI: drag-and-drop visual editor at /pipeline-builder.html?interfaceId=<id>
  - Canvas shows steps as nodes
  - Sidebar shows step type palette
  - Click a step to open its configuration panel
  - Save button commits the pipeline to the database`,
	},

	// NOTE: step-type-specific entries (field_mapping/enrichment.*, conditional/
	// switch_case, data_masking, remove_duplicates, connector.inbound/outbound,
	// fhir_validation) used to live here as hand-written Go string literals.
	// Removed — that content had already drifted from the real step schema
	// (e.g. field_mapping's example used {"source","target"} instead of the
	// actual {"hl7Field","fhirPath"} keys) and covered none of the CDA/format-
	// builder step families. Pipeline-step documentation is now sourced from
	// the real, continuously-maintained public/js/pipeline/documentation/
	// registry via IngestPipelineStepDocs (pipeline_step_docs_ingestion.go),
	// wired into IngestAppDocs below — one source of truth instead of two.
	// format_conversion (previously documented here) never had a real
	// executor anywhere in the Go codebase and was dropped entirely, not
	// migrated.

	// ── Connectors Reference ──────────────────────────────────────────────────
	{
		ref: "EHK_Connectors_Network",
		content: `ezHealthKonnect Connector Catalog — Network Connectors

tcp_mllp_inbound
  Listens for HL7 v2 messages over TCP using the MLLP protocol.
  Config: { "host": "0.0.0.0", "port": 2575, "tls": false,
            "maxConnections": 10, "keepAlive": true,
            "ack": { "mode": "immediate", "on_error": "suppress" } }
  ACK modes: "immediate" (send AA after queue), "none" (no ACK)
  on_error: "suppress" (always AA), "nack" (send AE on error)
  Default MLLP port: 2575. Port 2576 is also common.

tcp_mllp_outbound
  Sends HL7 v2 messages over TCP/MLLP to a downstream system.
  Config: { "host": "destination.host", "port": 2575, "tls": false,
            "connectTimeoutSeconds": 10, "reconnectDelaySeconds": 5 }

http_fhir_inbound
  Receives FHIR R4 resources via HTTP/HTTPS REST API.
  Config: { "path": "/fhir", "port": 8080, "auth": "bearer" }

http_fhir_outbound
  Sends FHIR R4 resources to a FHIR server.
  Config: { "endpoint": "https://fhir.example.com/R4", "auth": "bearer",
            "token": "<credential-ref>", "resourceType": "Bundle" }

http_outbound
  Sends any payload to a REST endpoint (non-FHIR).
  Config: { "endpoint": "https://api.example.com/webhook", "method": "POST",
            "headers": {"Authorization": "Bearer <token>"} }

http_rest_inbound
  Generic REST inbound (stub — use http_fhir_inbound for FHIR).`,
	},

	{
		ref: "EHK_Connectors_File_DB_Queue",
		content: `ezHealthKonnect Connector Catalog — File, Database & Queue Connectors

file_listener (File System Inbound)
  Watches a directory for new files and reads them into the pipeline.
  Config: { "path": "/data/input", "pattern": "*.hl7", "pollingIntervalSeconds": 5,
            "moveToDir": "/data/processed", "encoding": "UTF-8" }

file_writer (File System Outbound)
  Writes pipeline output to files.
  Config: { "path": "/data/output", "filename": "msg-{timestamp}.json",
            "encoding": "UTF-8", "append": false }

postgresql_inbound / postgresql_outbound
  Reads from / writes to a PostgreSQL database.
  Config: { "host": "db.host", "port": 5432, "database": "ehk",
            "user": "user", "password": "<credential-ref>",
            "query": "SELECT * FROM messages WHERE processed = false",
            "table": "outbound_messages" }

mysql_inbound / sqlserver_inbound / sqlserver_outbound
  Same pattern as PostgreSQL. SQL Server uses port 1433 by default.

mongodb_inbound / mongodb_outbound
  Config: { "uri": "mongodb://host:27017", "database": "ehk",
            "collection": "messages", "filter": {"status": "pending"} }

oracle_inbound / oracle_outbound
  Config: { "host": "ora.host", "port": 1521, "service": "ORCL",
            "user": "user", "password": "<credential-ref>" }

kafka_inbound
  Consumes messages from a Kafka topic.
  Config: { "brokers": ["kafka:9092"], "topic": "hl7-inbound",
            "groupId": "ezHealthKonnect", "autoOffsetReset": "latest" }

rabbitmq_inbound
  Consumes from a RabbitMQ queue.
  Config: { "uri": "amqp://user:pass@rabbitmq:5672", "queue": "hl7-messages",
            "prefetch": 10 }

redis_inbound
  Reads from a Redis list (BLPOP).
  Config: { "addr": "redis:6379", "list": "hl7-queue", "password": "" }

aws_s3_inbound
  Polls an S3 bucket prefix for new objects.
  Config: { "region": "us-east-1", "bucket": "my-bucket", "prefix": "hl7/",
            "accessKey": "<credential-ref>", "secretKey": "<credential-ref>" }

sftp_inbound / sftp_outbound
  Reads from / writes to an SFTP server.
  Config: { "host": "sftp.host", "port": 22, "user": "user",
            "password": "<credential-ref>", "remotePath": "/upload",
            "pattern": "*.hl7" }`,
	},

	// ── Interface & Wizard ────────────────────────────────────────────────────
	{
		ref: "EHK_Interface_Wizard",
		content: `ezHealthKonnect Interface Setup Wizard

An Interface connects a source system to a target system and defines the message flow.
The Wizard guides you through creating an interface in 5 steps:

Step 1 — Basic Info
  Name, description, message type (ADT^A01, ORU^R01, ORM^O01, etc.)

Step 2 — Source Connectivity
  Choose inbound connector type and configure it:
  - TCP/MLLP: host, port, TLS settings
  - HTTP: path, port, auth
  - File: directory path, file pattern
  - Database: connection string, query, polling interval

Step 3 — Target Connectivity
  Choose outbound connector type and configure the destination.

Step 4 — HL7→FHIR Mapping
  Map HL7 fields to FHIR resource fields.
  Standard templates are provided for ADT^A01 → Patient/Encounter and ORU^R01 → Observation.
  Custom mappings can be added or modified.

Step 5 — Review & Activate
  Review the generated pipeline. Click Activate to start the interface.

After creation:
  - Interface appears on the Dashboard
  - Pipeline can be edited in Pipeline Builder
  - Messages appear in the Messages tab for the interface
  - Monitoring shows real-time throughput and error rates

Key API endpoints:
  POST /api/interfaces            — create interface
  GET  /api/interfaces/:id       — get interface details
  POST /api/interfaces/:id/activate   — start the interface
  POST /api/interfaces/:id/deactivate — stop the interface
  GET  /api/messages/interface/:id    — get messages for an interface`,
	},

	// ── HL7 Core Segments ─────────────────────────────────────────────────────
	{
		ref: "HL7v2_Core_Segments_MSH_PID",
		content: `HL7 v2 Core Segments — MSH and PID

MSH — Message Header (every HL7 v2 message starts with MSH)
  MSH.1  Field Separator (always |)
  MSH.2  Encoding Characters (always ^~\&)
  MSH.3  Sending Application (e.g. "EPIC", "CERNER")
  MSH.4  Sending Facility
  MSH.5  Receiving Application
  MSH.6  Receiving Facility
  MSH.7  Date/Time of Message (YYYYMMDDHHMMSS)
  MSH.8  Security (usually empty)
  MSH.9  Message Type: MSH.9.1=MessageCode (ADT,ORU,ORM), MSH.9.2=TriggerEvent (A01,R01,O01)
  MSH.10 Message Control ID (unique per message, used for ACK)
  MSH.11 Processing ID (P=Production, T=Training, D=Debugging)
  MSH.12 Version ID (2.3, 2.4, 2.5, 2.5.1, 2.6)

FHIR Mapping:
  MSH.3 → MessageHeader.source.name
  MSH.7 → MessageHeader.timestamp
  MSH.10 → MessageHeader.id (correlation)
  MSH.9 → MessageHeader.eventCoding

PID — Patient Identification
  PID.1  Set ID (usually 1)
  PID.2  Patient ID (external; deprecated in 2.5+, use PID.3)
  PID.3  Patient Identifier List (CX: ID^CheckDigit^AssigningAuth^IDTypeCode)
            PID.3.1 = MRN or patient ID value
            PID.3.4 = Assigning Authority (e.g. "EPIC")
            PID.3.5 = ID Type Code (MR=Medical Record, PI=Patient Internal, SS=SSN)
  PID.5  Patient Name (XPN: FamilyName^GivenName^MiddleInitial^Suffix^Prefix)
            PID.5.1 = Last Name
            PID.5.2 = First Name
  PID.7  Date/Time of Birth (YYYYMMDD)
  PID.8  Administrative Sex (M, F, O, U)
  PID.10 Race (CE: code^text^codeSystem)
  PID.11 Patient Address (XAD: Street^Other^City^State^Zip^Country)
  PID.13 Phone Number - Home (XTN)
  PID.14 Phone Number - Business
  PID.18 Patient Account Number (visit/account ID)
  PID.19 SSN (deprecated — avoid using)

FHIR Mapping:
  PID.3.1 → Patient.identifier.value (system from PID.3.4)
  PID.5.1 → Patient.name.family
  PID.5.2 → Patient.name.given
  PID.7   → Patient.birthDate (convert YYYYMMDD → YYYY-MM-DD)
  PID.8   → Patient.gender (M→male, F→female, O→other, U→unknown)
  PID.11  → Patient.address`,
	},

	{
		ref: "HL7v2_Core_Segments_PV1_OBR_OBX",
		content: `HL7 v2 Core Segments — PV1, OBR, OBX, DG1

PV1 — Patient Visit
  PV1.2  Patient Class (I=Inpatient, O=Outpatient, E=Emergency, P=Preadmit, R=Recurring)
  PV1.3  Assigned Patient Location (PL: Point of Care^Room^Bed^Facility)
  PV1.7  Attending Doctor (XCN: ID^Last^First^Middle^^Suffix^Prefix^Degree^AssigningAuth)
  PV1.10 Hospital Service (e.g. MED=Medicine, SUR=Surgery)
  PV1.17 Admitting Doctor
  PV1.19 Visit Number (unique visit/encounter ID)
  PV1.44 Admit Date/Time
  PV1.45 Discharge Date/Time

FHIR Mapping:
  PV1.2   → Encounter.class (I→IMP, O→AMB, E→EMER)
  PV1.3   → Encounter.location[].location
  PV1.7   → Encounter.participant[].individual (Practitioner reference)
  PV1.19  → Encounter.identifier
  PV1.44  → Encounter.period.start
  PV1.45  → Encounter.period.end

OBR — Observation Request (used in ORU, ORM messages)
  OBR.1  Set ID
  OBR.2  Placer Order Number (order ID from ordering system)
  OBR.3  Filler Order Number (order ID from performing lab)
  OBR.4  Universal Service Identifier (CE: LOINC code^text^LN)
  OBR.7  Observation Date/Time
  OBR.16 Ordering Provider (XCN)
  OBR.22 Results Report/Change Date/Time
  OBR.25 Result Status (F=Final, P=Preliminary, C=Correction, X=Cancelled)

FHIR Mapping:
  OBR.4  → DiagnosticReport.code (LOINC)
  OBR.7  → DiagnosticReport.effectiveDateTime
  OBR.25 → DiagnosticReport.status (F→final, P→preliminary)

OBX — Observation Result (repeats per test result)
  OBX.1  Set ID
  OBX.2  Value Type (NM=Numeric, ST=String, CWE=Coded, DT=Date, TX=Text, SN=Structured Numeric)
  OBX.3  Observation Identifier (CE: LOINC code^text^LN)
  OBX.5  Observation Value (actual result — format depends on OBX.2)
  OBX.6  Units (CE: unit code^text^UCUM)
  OBX.7  Reference Range (e.g. "3.5-5.0")
  OBX.8  Abnormal Flags (H=High, L=Low, A=Abnormal, N=Normal, C=Critical)
  OBX.11 Observation Result Status (F=Final, P=Preliminary, C=Correction)
  OBX.14 Date/Time of Observation
  OBX.19 Date/Time of Analysis

FHIR Mapping:
  OBX.3  → Observation.code (LOINC)
  OBX.5  → Observation.value[x] (type based on OBX.2: NM→valueQuantity, ST→valueString, CWE→valueCodeableConcept)
  OBX.6  → Observation.valueQuantity.unit + system (UCUM)
  OBX.7  → Observation.referenceRange[0].text
  OBX.8  → Observation.interpretation.coding
  OBX.11 → Observation.status (F→final, P→preliminary, C→corrected)

DG1 — Diagnosis
  DG1.3  Diagnosis Code (CE: ICD-10 code^text^ICD10CM)
  DG1.4  Diagnosis Description
  DG1.5  Diagnosis Date/Time
  DG1.6  Diagnosis Type (A=Admitting, W=Working, F=Final)
  DG1.16 Diagnosing Clinician

FHIR Mapping:
  DG1.3  → Condition.code (ICD-10-CM system)
  DG1.5  → Condition.onsetDateTime
  DG1.6  → Condition.category (encounter-diagnosis)`,
	},

	// ── FHIR R4 Core Resources ────────────────────────────────────────────────
	{
		ref: "FHIR_R4_Core_Resources_Patient_Encounter",
		content: `FHIR R4 Core Resources — Patient and Encounter

Patient Resource
  Represents a person receiving healthcare services.
  Key elements:
    identifier[]      MRN, SSN, insurance ID — each has system (URL) + value
    name[]            HumanName: family, given[], prefix[], suffix[], use (official|usual|temp)
    telecom[]         ContactPoint: system (phone|email|fax), value, use (home|work|mobile)
    gender            "male" | "female" | "other" | "unknown"
    birthDate         YYYY-MM-DD (ISO 8601 date)
    address[]         Address: line[], city, state, postalCode, country, use (home|work|billing)
    maritalStatus     CodeableConcept (S=Single, M=Married, D=Divorced, W=Widowed)
    communication[]   { language: CodeableConcept, preferred: boolean }
    generalPractitioner[]  Reference(Practitioner)
    active            boolean (default true)

  Identifiers system URLs:
    MRN:  http://hospital.org/fhir/mrn (or site-specific)
    NPI:  http://hl7.org/fhir/sid/us-npi
    SSN:  http://hl7.org/fhir/sid/us-ssn
    DEA:  http://hl7.org/fhir/sid/us-dea

Encounter Resource
  Represents an interaction between patient and healthcare provider.
  Key elements:
    status            "planned" | "arrived" | "triaged" | "in-progress" | "onleave" |
                      "finished" | "cancelled"
    class             Coding: AMB=Ambulatory, IMP=Inpatient, EMER=Emergency, VR=Virtual
    type[]            CodeableConcept — type of encounter
    subject           Reference(Patient) — required
    participant[]     { type[], individual: Reference(Practitioner) }
    period            { start: dateTime, end: dateTime }
    reasonCode[]      CodeableConcept (ICD-10, SNOMED)
    hospitalization   { admitSource, dischargeDisposition }
    location[]        { location: Reference(Location), status }
    serviceProvider   Reference(Organization)
    identifier[]      Visit number, encounter ID`,
	},

	{
		ref: "FHIR_R4_Core_Resources_Observation_DiagnosticReport",
		content: `FHIR R4 Core Resources — Observation and DiagnosticReport

Observation Resource
  Represents a measurement or assertion (lab result, vital sign, survey response).
  Key elements:
    status            "registered" | "preliminary" | "final" | "amended" |
                      "corrected" | "cancelled" | "entered-in-error"
    category[]        CodeableConcept: laboratory, vital-signs, imaging, social-history
    code              CodeableConcept (LOINC preferred for labs/vitals)
    subject           Reference(Patient) — required
    encounter         Reference(Encounter)
    effective[x]      effectiveDateTime | effectivePeriod
    value[x]          The actual result:
                        valueQuantity        { value: decimal, unit: string, system: UCUM, code: UCUM }
                        valueCodeableConcept CodeableConcept (coded result)
                        valueString          free text
                        valueBoolean         true/false
                        valueRange           { low: Quantity, high: Quantity }
    dataAbsentReason  CodeableConcept — why value is missing
    interpretation[]  CodeableConcept: H=High, L=Low, A=Abnormal, N=Normal, LL=Critical Low, HH=Critical High
    referenceRange[]  { low: Quantity, high: Quantity, text: string }
    component[]       Sub-observations (e.g. systolic + diastolic in one BP observation)

  UCUM system URL: http://unitsofmeasure.org
  LOINC system URL: http://loinc.org

DiagnosticReport Resource
  Groups Observations from a single diagnostic study (lab panel, radiology).
  Key elements:
    status            "registered" | "partial" | "preliminary" | "final" | "amended" | "corrected"
    category[]        CodeableConcept: LAB, RAD, PATH, etc.
    code              CodeableConcept (LOINC for the panel)
    subject           Reference(Patient)
    encounter         Reference(Encounter)
    effective[x]      effectiveDateTime | effectivePeriod
    result[]          Reference(Observation) — individual test results
    conclusion        string — radiologist/pathologist narrative
    presentedForm[]   Attachment — PDF report

Condition Resource
  Represents a clinical condition, problem, or diagnosis.
  Key elements:
    clinicalStatus    CodeableConcept: active | recurrence | relapse | inactive | remission | resolved
    verificationStatus CodeableConcept: unconfirmed | provisional | differential | confirmed | refuted
    category[]        CodeableConcept: problem-list-item | encounter-diagnosis
    code              CodeableConcept (ICD-10-CM, SNOMED CT)
    subject           Reference(Patient)
    encounter         Reference(Encounter)
    onset[x]          onsetDateTime | onsetAge | onsetPeriod | onsetRange | onsetString
    recordedDate      dateTime`,
	},

	{
		ref: "FHIR_R4_Bundle_And_References",
		content: `FHIR R4 Bundle and Resource References

Bundle Resource
  A container for a collection of FHIR resources.
  Key elements:
    type    "transaction" | "batch" | "document" | "message" | "searchset" | "collection"
    entry[] { fullUrl: string, resource: Resource, request: { method, url } }

  Transaction Bundle (most common for HL7→FHIR):
    type: "transaction"
    entry[].request.method: "POST" | "PUT" | "GET" | "DELETE"
    entry[].request.url: "Patient" | "Patient?identifier=MRN|12345"

  Conditional Create (upsert by identifier):
    method: "POST"
    url: "Patient?identifier=http://hospital.org/mrn|12345"
    — Server creates if not found, returns existing if found

Contained Resources
  Small resources embedded inside another (avoid when possible):
    { "resourceType": "Patient", "contained": [{ "resourceType": "Organization", "id": "org1" }] }
    Reference to contained: "#org1"

Resource References
  { "reference": "Patient/123" }           — literal reference (preferred)
  { "reference": "Patient?identifier=..." } — conditional reference
  { "display": "Jane Doe" }                — display only (no reference)

Common System URLs
  SNOMED CT:  http://snomed.info/sct
  LOINC:      http://loinc.org
  ICD-10-CM:  http://hl7.org/fhir/sid/icd-10-cm
  RxNorm:     http://www.nlm.nih.gov/research/umls/rxnorm
  CPT:        http://www.ama-assn.org/go/cpt
  NPI:        http://hl7.org/fhir/sid/us-npi
  UCUM:       http://unitsofmeasure.org
  HL7 V3:     http://terminology.hl7.org/CodeSystem/v3-*
  HL7 V2:     http://terminology.hl7.org/CodeSystem/v2-*`,
	},

	// ── HL7 Message Types ─────────────────────────────────────────────────────
	{
		ref: "HL7v2_Message_Types_ADT_ORU",
		content: `HL7 v2 Common Message Types

ADT — Admit, Discharge, Transfer
  ADT^A01  Admit/Visit Notification          → FHIR: Encounter (status: in-progress) + Patient
  ADT^A02  Transfer a Patient                → FHIR: Encounter location update
  ADT^A03  Discharge/End Visit              → FHIR: Encounter (status: finished)
  ADT^A04  Register a Patient               → FHIR: Patient create + Encounter (status: arrived)
  ADT^A05  Pre-Admit a Patient              → FHIR: Encounter (status: planned)
  ADT^A06  Change Outpatient to Inpatient   → FHIR: Encounter class change
  ADT^A08  Update Patient Information       → FHIR: Patient update
  ADT^A11  Cancel Admit / Visit Notification → FHIR: Encounter (status: cancelled)
  ADT^A13  Cancel Discharge                 → FHIR: Encounter re-open
  ADT^A28  Add Person Information           → FHIR: Patient create
  ADT^A31  Update Person Information        → FHIR: Patient update
  ADT^A40  Merge Patient                    → FHIR: Patient merge operation

  Segment structure: MSH | EVN | PID | [PD1] | PV1 | [PV2] | [DG1] | [AL1]

ORU — Unsolicited Observation (Lab Results)
  ORU^R01  Unsolicited Observation Message  → FHIR: DiagnosticReport + Observation[]
  ORU^R30  Unsolicited Point-Of-Care Observation

  Segment structure: MSH | [PID | PV1] | OBR | OBX[] | [NTE]
  One OBR per test panel, multiple OBX per panel (one per individual result)

ORM — Order Message
  ORM^O01  General Order Message            → FHIR: ServiceRequest + Task

ORC — Common Order Segment (in ORM)
  ORC.1  Order Control (NW=New, CA=Cancel, DC=Discontinue, RP=Replace)
  ORC.2  Placer Order Number
  ORC.3  Filler Order Number
  ORC.7  Quantity/Timing
  ORC.9  Date/Time of Transaction
  ORC.12 Ordering Provider

MDM — Medical Document Management
  MDM^T02  Original Document Notification with Content → FHIR: DocumentReference + Composition

SIU — Scheduling Information Unsolicited
  SIU^S12  Notification of New Appointment  → FHIR: Appointment (status: booked)
  SIU^S15  Notification of Appointment Cancellation → FHIR: Appointment (status: cancelled)

DFT — Detailed Financial Transaction
  DFT^P03  Post Detail Financial Transaction → FHIR: ChargeItem`,
	},

	// NOTE: EHK_Step_Types_Script_Config (enrichment.script config reference)
	// removed for the same reason as the block above — fully superseded by
	// public/js/pipeline/documentation/EnrichmentStepsDocs.js's
	// pre.enrichment.script entry (aliased from enrichment.script), now
	// ingested via IngestPipelineStepDocs.

	// ── HL7 ACK Codes ─────────────────────────────────────────────────────────
	{
		ref: "EHK_HL7_ACK_Codes",
		content: `HL7 v2 Acknowledgement (ACK) Codes — MSA Segment

When ezHealthKonnect receives an HL7 message over TCP/MLLP it sends back an ACK message.
The ACK message contains the MSA segment which tells the sender what happened.

MSA Segment Fields:
  MSA.1  Acknowledgement Code (AA, AE, AR)
  MSA.2  Message Control ID — echoed from the original MSH.10 so the sender can match it
  MSA.3  Text Message — human-readable description of the result

Acknowledgement Codes:
  AA  Application Accept   — message received and accepted successfully
  AE  Application Error    — message received but a processing error occurred (e.g. validation failed, required field missing)
  AR  Application Reject   — message rejected at the protocol level (e.g. unsupported message type, malformed structure)

Operational meaning:
  AA → sender can mark the message as delivered; no retry needed
  AE → sender SHOULD retry after fixing the content issue; the problem is in the message data
  AR → sender must not retry without structural changes; the message type or format is wrong

ezHealthKonnect ACK behaviour (TCP/MLLP Inbound connector):
  Default mode: "immediate" — AA sent as soon as message is queued (before pipeline runs)
  on_error "suppress" (default): always send AA even if pipeline later fails; sender is not notified of pipeline errors
  on_error "nack": send AE if the inbound queue is full or a critical pre-queue error occurs
  Custom script: a JavaScript buildACK(msg) function can override the code and text per-message

If you receive AE from a downstream system (as the sender):
  Check MSA.3 text for the error description
  Review ERR segment if present — it contains field-level error details
  Common causes: missing required field, invalid code value, wrong message structure`,
	},

	// ── Admin Operations ──────────────────────────────────────────────────────
	{
		ref: "EHK_Admin_Operations",
		content: `ezHealthKonnect — Admin Operations Guide

--- FREQUENTLY ASKED QUESTIONS ---

Q: How do I enable or disable the AI assistant (ezCompanion)?
A: Go to Settings -> AI tab (URL: /settings.html, click the AI tab). Toggle the "Enable AI Assistant" switch and click Save Settings. When disabled, the ezCompanion chat icon is hidden for all users across the platform.

Q: How do I check which Ollama model is currently active?
A: Go to Settings -> AI tab -> Model Library section. Under "Installed Models" you will see all locally downloaded models. The active model is marked with a "Set Active" badge or highlighted. To switch to a different model click "Set Active" next to it.

Q: How do I change or update the active Ollama model?
A: Settings -> AI tab -> Model Library. To install a new model, find it in the "Model Catalog" section and click "Download". Once downloaded it appears under "Installed Models". Click "Set Active" to make it the running model.

Q: How do I create a new user and assign them the admin role?
A: Navigate to Admin -> User Management (/user-management.html), click "Add User", fill in Full Name, Email, Password, and set Role to "admin", then click Save.

--- FULL GUIDE ---

USER MANAGEMENT
Navigate to: top nav → Admin → User Management (URL: /user-management.html)

Create a new user:
  1. Click "Add User" button (top right of the User Management page)
  2. Fill in: Full Name, Email, Password, Role
  3. Select Role: "user" (standard), "admin" (full admin access), "super_admin" (system-level)
  4. Click Save — the user receives an email if SMTP is configured

Change a user's role:
  1. Find the user in the list → click the edit (pencil) icon
  2. Change the Role dropdown
  3. Click Save

Deactivate / delete a user:
  1. Find the user → click the delete (trash) icon
  2. Confirm the dialog — users cannot delete their own account

AI ASSISTANT SETTINGS
Navigate to: top nav → Settings → AI tab (URL: /settings.html, then click the AI tab)

Enable / disable ezCompanion:
  Toggle "Enable AI Assistant" switch → Save Settings
  When disabled, the ezCompanion icon is hidden for all users

Change the active Ollama model:
  Settings → AI tab → Model Library section
  "Installed Models" shows locally available models with a "Set Active" button
  "Model Catalog" shows recommended models with a "Download" button for new pulls
  After download completes, click "Set Active" to switch the running model

SYSTEM SETTINGS
Navigate to: Settings → General tab
  - Application name, logo, timezone
  - SMTP configuration for email notifications
  - Session timeout

INTERFACE MANAGEMENT
Navigate to: Interfaces page (main nav)
  - View all interfaces and their status (active/inactive)
  - Click an interface card to open its detail page
  - Start/Stop interface: use the toggle on the interface detail page
  - View messages: click the 💬 icon on an interface card → /messages.html?interfaceId=<id>

BACKUP / EXPORT
Interface configurations are stored in the PostgreSQL database.
To export: use pg_dump on the database (docker exec ezhealthkonnect-postgres pg_dump -U <user> <db>)
Pipeline steps, connector configs, and mappings are all in the database and included in the dump.
There is no built-in one-click export in the UI — database-level backup is the supported method.`,
	},

	// ── Support Troubleshooting & Error Layers ────────────────────────────────
	{
		ref: "EHK_Support_Troubleshooting",
		content: `ezHealthKonnect — Support & Troubleshooting Guide

═══ ERROR LAYERS ═══════════════════════════════════════════════

There are four layers where errors can appear. Check them in this order:

1. UI / BROWSER ERRORS
   Symptoms: button doesn't work, page won't load, form won't save
   Where to look:
     - Browser DevTools → Console tab (F12 in Chrome/Edge): JavaScript errors shown in red
     - Browser DevTools → Network tab: look for red requests to /api/* with 4xx or 5xx status
     - Click the failing request → Preview tab to see the error message from the server
   Common causes: session expired (401 → log in again), permission denied (403), server down (502/503)

2. NODE.JS BACKEND ERRORS
   Symptoms: API calls fail, user management broken, interface config not saving, wizard errors
   Where to look:
     - Docker logs: docker-compose logs app | grep -i error (run on the host)
     - Monitoring → System Logs page in the UI (if available)
     - Look for lines prefixed with ❌ or ERROR
   Common causes: database connection lost, session store failure, missing environment variable

3. GO BACKEND ERRORS
   Symptoms: pipeline execution fails, connector won't start, FHIR transformation errors
   Where to look:
     - Same docker logs: docker-compose logs app (Go and Node.js share the container, Go logs go to stdout)
     - Go errors are timestamped and often include the failing function name
     - In the UI: Monitoring → Interface Logs for per-interface connector activity
   Common causes: Ollama unreachable, PostgreSQL query error, connector config invalid, Go panic (rare)

4. OLLAMA / AI ERRORS
   Symptoms: ezCompanion gives no response, "AI unavailable" message
   Where to look:
     - docker-compose logs ollama
     - Settings → AI tab → status indicator shows if Ollama is reachable
     - The model may not be loaded: check Settings → AI → Installed Models

═══ MESSAGE PROCESSING STAGES ══════════════════════════════════

A message goes through these stages. Failures at each stage leave a trace in a different place:

Stage 1 — INBOUND CONNECTION (connector receives the message)
  Failure signs: message never appears in the interface message viewer
  Where to look: Monitoring → Interface Logs → filter by your interface
  Common causes: port not open, TLS certificate mismatch, MLLP framing error, file path wrong

Stage 2 — RAW STORAGE (message written to PostgreSQL + MongoDB)
  Failure signs: message appears but shows parsing_status = failed
  Where to look: docker-compose logs app | grep "JSON conversion"
  Common causes: malformed HL7 (missing MSH), database write error

Stage 3 — PIPELINE EXECUTION (transformation steps run)
  Failure signs: message status = "failed" in the interface message viewer
  Where to look:
    - Interface message viewer (/messages.html?interfaceId=<id>) → click the message
    - The message detail panel shows a Pipeline Trace: each step with status ✅/❌, duration, and error text
    - The failing step shows the exact error (e.g. "Cannot read property 'value' of undefined" in a script)
  Common causes: transform() script error, missing HL7 field, enrichment API timeout, FHIR validation failure

Stage 4 — OUTBOUND DELIVERY (connector sends the result)
  Failure signs: message status = "failed" or delivery_status = "failed" after pipeline succeeded
  Where to look: Monitoring → Interface Logs → outbound connector events
  Common causes: destination host unreachable, wrong port, authentication failure, payload rejected by receiver

Stage 5 — DEAD LETTER QUEUE (all retries exhausted)
  Navigate to: Monitoring → Dead Letter Queue (/dlq.html)
  The DLQ shows messages that failed after all retry attempts
  Actions:
    - Click a message to see the full error history
    - Select one or more messages → click "Redrive" to re-send through the full pipeline from the top
    - Fix the underlying issue (config, mapping, destination) before redriving

═══ MESSAGE STATUS REFERENCE ════════════════════════════════════

  pending      → received, waiting to enter the pipeline
  processing   → pipeline is currently executing
  completed    → all steps succeeded, message delivered
  failed       → pipeline or delivery failed (check pipeline trace)
  dead_letter  → moved to DLQ after retry limit reached

═══ QUICK CHECKLIST FOR A STUCK MESSAGE ════════════════════════

1. Find the message in /messages.html?interfaceId=<id>
2. Note the status field
3. Click the message → read the Pipeline Trace for the failing step and its error text
4. If status = dead_letter → go to /dlq.html, find the message, click to see full error chain
5. Check docker-compose logs app for the same timestamp as the message failure
6. Fix the root cause (script bug, config error, destination issue)
7. Redrive from DLQ or re-send the original message`,
	},

	// ── General HL7 Primer ───────────────────────────────────────────────────
	{
		ref: "EHK_HL7_Primer",
		content: `HL7 — What It Is and Why It Matters in Healthcare

HL7 (Health Level Seven) is a set of international standards for the exchange, integration, sharing,
and retrieval of electronic health information. "Level Seven" refers to the 7th (application) layer
of the OSI networking model.

HL7 is the backbone of healthcare data exchange in the US and globally. Without it, hospitals,
labs, pharmacies, insurance companies, and EHR systems (Epic, Cerner, Meditech, etc.) cannot
share patient data reliably.

--- HL7 v2 (Version 2) ---

HL7 v2 is the most widely deployed healthcare messaging standard (90%+ of US hospitals use it).
It is a pipe-delimited text format like this:

  MSH|^~\&|EPIC|HOSP|LAB|LABSYS|20240101120000||ADT^A01|12345|P|2.5
  PID|1||MRN123^^^HOSP^MR||SMITH^JOHN^A||19800101|M|||123 MAIN ST^^CITY^ST^12345

Key characteristics:
  - Each line is a "segment" (MSH, PID, PV1, OBX, etc.)
  - Fields separated by | (pipe)
  - Sub-fields separated by ^ (caret)
  - Repeating fields separated by ~ (tilde)
  - Escape character: \

Common HL7 v2 message types (MSH.9):
  ADT  Admit/Discharge/Transfer (patient registration, admits, discharges)
  ORU  Observation Result (lab results, vitals)
  ORM  Orders (lab orders, medication orders)
  SIU  Scheduling (appointments)
  MDM  Medical Documents (clinical notes, reports)
  DFT  Financial (charges, billing)

--- FHIR (Fast Healthcare Interoperability Resources) ---

FHIR R4 is HL7's modern standard, introduced in 2014. It uses JSON/XML/RDF instead of pipes.
FHIR is REST-based: each resource (Patient, Encounter, Observation) has a URL and can be
accessed via standard HTTP GET/POST/PUT.

  Example FHIR Patient:
  {
    "resourceType": "Patient",
    "id": "123",
    "name": [{ "family": "Smith", "given": ["John"] }],
    "birthDate": "1980-01-01",
    "gender": "male"
  }

--- HL7 v2 vs FHIR ---

Feature         HL7 v2                      FHIR R4
Format          Pipe-delimited text         JSON / XML / RDF
Protocol        TCP/MLLP, SFTP, file        REST HTTP
Adoption        90%+ of US hospitals        Growing rapidly; US Core mandated by CMS
Flexibility     Highly customizable         Standardized, profiled
Tooling         Requires HL7 parser         Standard HTTP/REST libraries
Validation      Weak (mostly text)          Strong (profiles, StructureDefinition)

--- Why ezHealthKonnect uses both ---

Most healthcare source systems (Epic, Cerner, Meditech, lab systems) SEND HL7 v2 messages.
Most modern API consumers (patient portals, mobile apps, payers) EXPECT FHIR R4.
ezHealthKonnect bridges the gap: it receives HL7 v2 and transforms it to FHIR R4 in real-time.`,
	},

	// ── ADT Message Type Comparison ──────────────────────────────────────────
	{
		ref: "EHK_ADT_Message_Types",
		content: `HL7 v2 ADT Message Types — Detailed Comparison

ADT (Admit, Discharge, Transfer) messages are the most common HL7 v2 message type.
They notify receiving systems about changes to a patient's registration or visit status.

--- ADT^A01 — Admit / Visit Notification ---

Trigger: Patient is admitted to the hospital (inpatient) or an outpatient visit begins.
Use case: ED admit, scheduled inpatient surgery begins, outpatient clinic check-in.
Creates in receiving system:
  - A new Encounter record (status: in-progress, class: IMP for inpatient)
  - Upsert Patient demographics from PID segment
  - PV1 carries the bed assignment, attending physician, admit date/time

FHIR mapping: ADT^A01 → Encounter (status: in-progress) + Patient
Key segments: MSH | EVN | PID | [PD1] | PV1 | [PV2] | [DG1] | [AL1]

--- ADT^A28 — Add Person Information ---

Trigger: A new person is registered in the source system WITHOUT a visit.
Use case: Pre-registration, creating a patient record before an appointment,
          registering a guarantor or a contact person.
Creates in receiving system:
  - Only a Patient record — NO Encounter
  - Used for demographic-only updates when no visit is happening

FHIR mapping: ADT^A28 → Patient (create or update) — NO Encounter created

--- Key Differences: A01 vs A28 ---

Aspect         ADT^A01                         ADT^A28
Purpose        Admit patient / start visit     Register person (no visit)
Encounter?     YES — creates Encounter          NO — Patient only
PV1 segment    Required (visit location, bed)  Present but minimal or absent
EVN segment    Admission event                 Add person event
FHIR result    Patient + Encounter             Patient only
Common trigger Hospital admission, ED visit    Pre-registration, guarantor setup

--- Other Common ADT Trigger Events ---

ADT^A02  Transfer a patient (room/unit change)    - Encounter location update
ADT^A03  Discharge patient                        - Encounter (status: finished)
ADT^A04  Register outpatient visit               - Patient + Encounter (status: arrived)
ADT^A08  Update patient demographics              - Patient update (no Encounter change)
ADT^A11  Cancel admit                             - Encounter (status: cancelled)
ADT^A13  Cancel discharge                         - Encounter re-opened (status: in-progress)
ADT^A31  Update person information                - Patient update (same as A28 but for existing)
ADT^A40  Merge patient records                    - FHIR Patient merge operation

--- Quick Rule ---

A01 = "Patient is HERE right now" (triggers Encounter)
A28 = "We know this person exists" (just a Patient record, no active visit)`,
	},

	// ── Compliance & PHI Isolation ────────────────────────────────────────────
	{
		ref: "EHK_Compliance_PHI_Isolation",
		content: `ezHealthKonnect — Compliance, PHI Protection & AI Isolation

--- FREQUENTLY ASKED QUESTIONS ---

Q: Is any patient data sent to external AI services like OpenAI or Anthropic? Does ezHealthKonnect use ChatGPT or cloud AI?
A: NO. Patient data (PHI) is NEVER sent to any external AI service. OpenAI, Anthropic, ChatGPT, GPT-4, and all cloud AI providers are intentionally not connected and were removed from the codebase for HIPAA compliance. The AI assistant (ezCompanion) uses Ollama exclusively — a fully local LLM runtime that runs inside the customer's own Docker network. No internet call is made during AI inference.

Q: What encryption is used for credentials stored in the system?
A: Connector credentials (passwords, API keys, certificates) are encrypted at rest using AES-256-GCM. The encryption key is derived from the APP_CREDENTIAL_KEY environment variable (a base64-encoded 32-byte key). If APP_CREDENTIAL_KEY is not set, credentials are stored in plaintext with a warning — this is only acceptable in development mode, not production.

Q: How is PHI protected during message processing?
A: PHI is protected through: (1) AES-256-GCM encryption for stored credentials, (2) TLS in transit for MLLP/SFTP/HTTP connectors, (3) all message storage in on-premise PostgreSQL and MongoDB — never synced externally, (4) role-based access control limiting who can view messages, (5) HIPAA audit logging of all access events.

Q: Does ezHealthKonnect support HIPAA-compliant audit logging?
A: Yes. All user actions are written to the audit_logs table in PostgreSQL. Events logged include: login, logout, user creation, interface changes, pipeline saves, and message access. Each entry records user_id, user_email, action, resource_type, resource_id, ip_address, and timestamp. A file-based backup log also writes to logs/audit.log (append-only).

--- FULL COMPLIANCE DETAILS ---

--- PHI DOES NOT LEAVE THE NETWORK ---

ezHealthKonnect is designed for on-premise deployment. Protected Health Information (PHI)
never leaves the customer network. This applies to all components including the AI assistant.

AI ASSISTANT — COMPLETE LOCAL ISOLATION
  - The AI assistant (ezCompanion) runs entirely via Ollama — a local, self-hosted LLM runtime
  - Ollama runs inside the customer's Docker stack (container: ezhealthkonnect-ollama)
  - All LLM inference happens on the local machine — no internet call is made
  - Anthropic, OpenAI, ChatGPT, GPT-4, and all cloud AI services are NOT connected
  - These providers were intentionally removed from the codebase for HIPAA compliance
  - User questions to ezCompanion stay inside the Docker network
  - RAG (vector search) runs against the local PostgreSQL/pgvector database
  - Conversation history is stored in the local ai_conversations table — never synced externally

CREDENTIAL STORAGE
  - Connector credentials (passwords, API keys, certificates) are encrypted at rest
  - Encryption: AES-256-GCM with a key derived from APP_CREDENTIAL_KEY environment variable
  - If APP_CREDENTIAL_KEY is not set: credentials stored in plaintext with a warning (dev only)
  - Credentials are never logged or included in AI context

AUDIT LOGGING (HIPAA)
  - All user actions are written to the audit_logs table in PostgreSQL
  - Logged events: login, logout, user creation, interface changes, pipeline saves, message access
  - Audit log fields: user_id, user_email, action, resource_type, resource_id, ip_address, timestamp
  - File-based backup log: logs/audit.log (append-only)
  - Logs are never deleted automatically — retention policy is the customer's responsibility

DATA IN TRANSIT
  - Node.js ↔ Go backend: internal Docker network (not exposed externally)
  - Browser ↔ Application: HTTPS when TLS is configured (recommended for production)
  - MLLP: supports TLS 1.2/1.3 on the TCP/MLLP inbound connector (configure cert + key in connector config)
  - SFTP/S3/other connectors: use encrypted transport natively

MESSAGE STORAGE
  - Raw HL7 messages stored in PostgreSQL (messages_intf_<id> tables) and optionally MongoDB
  - MongoDB connection is internal to the Docker network
  - No message content is sent to any external service at any stage

HIPAA COMPLIANCE FEATURES SUMMARY
  ✅ All PHI stays on-premise — no cloud dependencies required
  ✅ AES-256-GCM credential encryption
  ✅ HIPAA audit trail (audit_logs table + file backup)
  ✅ Role-based access control (user / admin / super_admin)
  ✅ Session-based authentication with configurable timeout
  ✅ Local-only AI inference — Ollama, no external API calls
  ✅ TLS support for MLLP, SFTP, and HTTP connectors`,
	},
}
