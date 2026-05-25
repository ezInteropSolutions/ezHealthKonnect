// services/ai/builtin_knowledge.go
// Static healthcare standards knowledge embedded directly in the binary.
// This covers X12 835/837/270/271, CCD/CDA — formats where we have no local schema files.
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
