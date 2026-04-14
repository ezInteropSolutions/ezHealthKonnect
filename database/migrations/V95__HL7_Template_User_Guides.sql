-- V95: Add user_guide column to hl7_fhir_templates and populate OOB guides.
--
-- Each guide explains:
--   1. What the message type does (clinical context)
--   2. What FHIR resources are produced automatically (no wizard required)
--   3. What the HL7 → FHIR wizard adds on top
--   4. Segment → resource reference table
--   5. A link to the FHIR R4 spec for the primary resource
--
-- Guides are fetched by GET /api/wizard/message-guide/:messageType
-- and rendered as a collapsible info panel in wizard Step 4.

ALTER TABLE hl7_fhir_templates
    ADD COLUMN IF NOT EXISTS user_guide TEXT;

-- ─────────────────────────────────────────────────────────────────────────────
-- ADT^A01  — Admit / Visit Notification
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE hl7_fhir_templates SET user_guide = $$
# ADT^A01 — Admit / Visit Notification

**Clinical purpose**: Sent when a patient is admitted or registered.  Carries demographics, visit information, insurance, allergies, diagnoses, and procedures.

---

## What works without the wizard

The system automatically produces the following FHIR R4 resources from every ADT^A01 message with no configuration required:

| HL7 Segment(s) | FHIR R4 Resource | Always / Conditional |
|---|---|---|
| MSH | MessageHeader | Always |
| PID + PD1 | Patient | Always |
| PV1 + PV2 | Encounter | Always |
| IN1 / PV1.20 | Coverage | When insurance data is present |
| NK1 | RelatedPerson | When next-of-kin is present |
| AL1 | AllergyIntolerance | When allergy segments are present |
| DG1 | Condition | When diagnosis segments are present |
| PR1 | Procedure | When procedure segments are present |
| PD1 / OBX | Observation | When observation segments are present |

**You can send ADT^A01 messages and receive a complete FHIR R4 Bundle immediately — no wizard configuration needed.**

---

## What the wizard adds

Run the HL7 → FHIR wizard when your site needs any of the following:

| Need | What to configure |
|---|---|
| Custom patient identifier systems | Map PID.3.4 (Assigning Authority) to your OID or URI |
| Non-standard gender / marital status codes | Add a value map in the wizard's field editor |
| US Core Patient profile compliance | Set profile to `us-core-patient` in the Mapping step |
| Additional PID fields your EHR sends | Add custom field mappings (e.g. PID.25 → extension) |
| Encounter class codes from a proprietary table | Override the default HL7 Table 0004 value map |
| Custom encounter identifiers | Map PV1.19 to a specific Encounter.identifier system |

---

## Key field mappings (automatic)

| HL7 Field | FHIR Path | Notes |
|---|---|---|
| PID.3 | Patient.identifier[] | Full CX type: value + system (OID) + type (MR/SS/etc.) |
| PID.5 | Patient.name[0] | XPN: family, given[], prefix, suffix |
| PID.7 | Patient.birthDate | HL7 TS → FHIR date (YYYY-MM-DD) |
| PID.8 | Patient.gender | Table 0001: M→male, F→female, O→other |
| PID.11 | Patient.address[] | XAD: line, city, state, postalCode, country |
| PID.13/14 | Patient.telecom[] | XTN: phone, fax, email |
| PV1.2 | Encounter.class | Table 0004: I→IMP, O→AMB, E→EMER |
| PV1.44/45 | Encounter.period.start/end | Admit / discharge times |
| DG1.3 | Condition.code | CE → CodeableConcept (ICD-10 system resolved automatically) |
| AL1.3 | AllergyIntolerance.code | CE → CodeableConcept |

---

## FHIR R4 references

- [Patient](https://hl7.org/fhir/R4/patient.html)
- [Encounter](https://hl7.org/fhir/R4/encounter.html)
- [Condition](https://hl7.org/fhir/R4/condition.html)
- [AllergyIntolerance](https://hl7.org/fhir/R4/allergyintolerance.html)
$$
WHERE message_type = 'ADT^A01' AND is_system = true;

-- ─────────────────────────────────────────────────────────────────────────────
-- ORU^R01  — Observation Result
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE hl7_fhir_templates SET user_guide = $$
# ORU^R01 — Observation Result

**Clinical purpose**: Carries laboratory results, radiology reports, vital signs, and other clinical observations.  One ORU message typically contains one or more observation orders (OBR) each with multiple result lines (OBX).

---

## What works without the wizard

| HL7 Segment(s) | FHIR R4 Resource | Always / Conditional |
|---|---|---|
| MSH | MessageHeader | Always |
| PID | Patient | Always |
| PV1 | Encounter | When visit data is present |
| ORC + OBR | DiagnosticReport | Always (one per OBR group) |
| ORC + OBR | ServiceRequest | When order data is present |
| OBX | Observation | Always (one per OBX segment) |
| OBR / SPM | Specimen | When specimen data is present |
| PRT | PractitionerRole | When practitioner participation is present |

**Grouped assembly is automatic**: OBX segments are linked to their parent DiagnosticReport via `DiagnosticReport.result[]`.  Text-type OBX segments (TX/FT) that share the same OBX.3 identifier are automatically collapsed into a single Observation — this is the standard radiology / pathology report continuation pattern.

---

## What the wizard adds

| Need | What to configure |
|---|---|
| Custom LOINC panel codes for OBR | Map OBR.4 to a specific LOINC system |
| Non-LOINC observation codes (proprietary) | Add Tier-3 facility namespace for OBX.3 codes |
| Custom result interpretation codes | Override OBX.8 value map |
| DiagnosticReport category override | Set service-specific category (radiology, laboratory, etc.) |
| Additional OBX value types | The engine handles NM, ST, TX, FT, CE, CWE, TS, SN, ED, RP automatically; extend via wizard for custom types |
| US Core DiagnosticReport profile | Set profile in the Mapping step |

---

## Key field mappings (automatic)

| HL7 Field | FHIR Path | Notes |
|---|---|---|
| OBR.4 | DiagnosticReport.code | CE/CWE → CodeableConcept (LOINC auto-resolved) |
| OBR.7 | DiagnosticReport.effectiveDateTime | Observation date/time |
| OBR.25 | DiagnosticReport.status | HL7 Table 0123: F→final, P→preliminary, C→corrected |
| OBX.3 | Observation.code | CE/CWE → CodeableConcept |
| OBX.5 | Observation.value[x] | Dispatched by OBX.2 type: NM→valueQuantity, ST/TX→valueString, CE→valueCodeableConcept, etc. |
| OBX.6 | Observation.valueQuantity.unit | UCUM unit code |
| OBX.7 | Observation.referenceRange[] | Structured low/high or text |
| OBX.8 | Observation.interpretation[] | HL7 Table 0078: H→high, L→low, N→normal, etc. |
| OBX.11 | Observation.status | HL7 Table 0085: F→final, P→preliminary, C→corrected |
| OBX.14 | Observation.effectiveDateTime | Result date/time |

---

## FHIR R4 references

- [DiagnosticReport](https://hl7.org/fhir/R4/diagnosticreport.html)
- [Observation](https://hl7.org/fhir/R4/observation.html)
- [ServiceRequest](https://hl7.org/fhir/R4/servicerequest.html)
$$
WHERE message_type = 'ORU^R01' AND is_system = true;

-- ─────────────────────────────────────────────────────────────────────────────
-- MDM^T02  — Medical Document Management
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE hl7_fhir_templates SET user_guide = $$
# MDM^T02 — Medical Document Management

**Clinical purpose**: Carries clinical documents — discharge summaries, procedure notes, radiology reports stored as documents.  The TXA segment is the document header; OBX segments carry the document body.

---

## What works without the wizard

| HL7 Segment(s) | FHIR R4 Resource | Always / Conditional |
|---|---|---|
| MSH | MessageHeader | Always |
| PID | Patient | Always |
| TXA + OBX | DocumentReference | Always (one per message) |

**Document body assembly is automatic**:

| OBX.2 value type | How it's stored |
|---|---|
| TX / FT / ST | Text lines concatenated, base64-encoded in `DocumentReference.content[0].attachment.data` (contentType: text/plain) |
| HTML | Same but contentType: text/html |
| ED (Encapsulated Data) | Decoded as MIME binary attachment |
| RP (Reference Pointer) | External URL in `attachment.url` |

---

## What the wizard adds

| Need | What to configure |
|---|---|
| Custom document type codes (TXA.2) | Map to LOINC document type codes |
| FHIR DocumentReference.category | Set the document category (clinical-note, discharge-summary, etc.) |
| US Core DocumentReference profile | Set profile in the Mapping step |
| Practitioner resources | The author / authenticator from TXA.9/22 are display-only by default; wizard lets you link to a Practitioner resource |

---

## Key field mappings (automatic)

| HL7 Field | FHIR Path | Notes |
|---|---|---|
| TXA.2 | DocumentReference.type | CE → CodeableConcept |
| TXA.4 / TXA.6 | DocumentReference.date | Activity / origination date |
| TXA.9 | DocumentReference.author[0] | XCN → display name (no Practitioner bundle entry) |
| TXA.12 | DocumentReference.masterIdentifier | Unique document number |
| TXA.17 | DocumentReference.docStatus | Table 0271: AU/LA→final, OC→entered-in-error, others→preliminary |
| TXA.19 | DocumentReference.status | Table 0273: AV→current, UN→superseded, OB→entered-in-error |
| TXA.22 | DocumentReference.authenticator | XCN → display name |
| OBX.5 | DocumentReference.content[0].attachment.data | Base64-encoded document body |

---

## FHIR R4 references

- [DocumentReference](https://hl7.org/fhir/R4/documentreference.html)
$$
WHERE message_type = 'MDM^T02' AND is_system = true;

-- ─────────────────────────────────────────────────────────────────────────────
-- DFT^P03  — Post Detail Financial Transaction
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE hl7_fhir_templates SET user_guide = $$
# DFT^P03 — Post Detail Financial Transaction

**Clinical purpose**: Posts individual charge-level financial transactions.  Used for billing workflows — each FT1 segment is one charge line item, typically corresponding to a procedure performed on the patient.

---

## What works without the wizard

| HL7 Segment(s) | FHIR R4 Resource | Always / Conditional |
|---|---|---|
| MSH | MessageHeader | Always |
| PID | Patient | Always |
| FT1 (×N) | ChargeItem (×N) | One per FT1 line item |
| PR1 (×N) | Procedure (×N) | One per PR1, linked to same set-ID ChargeItem |
| DG1 (×N) | Condition (×N) | One per DG1 diagnosis |

**Set-ID linking is automatic**: each FT1 and its corresponding PR1 share a set-ID — the engine wires `ChargeItem.service → Procedure` automatically.

**Procedure coding system resolution** (PR1.2 field):

| PR1.2 value | FHIR system |
|---|---|
| CPT / CPT4 | http://www.ama-assn.org/go/cpt |
| ICD10PCS | http://www.cms.gov/Medicare/Coding/ICD10 |
| ICD9 / ICD-9 | http://hl7.org/fhir/sid/icd-9-cm |
| HCPCS | http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets |
| Other | Facility namespace (urn:facility:{sending-facility}) |

**Diagnosis codes** (DG1.3, FT1.19 repeating CE fields) — ICD-10 (I10), ICD-9 (I9), and SNOMED CT system codes are automatically resolved to their FHIR system URIs.

---

## What the wizard adds

| Need | What to configure |
|---|---|
| Custom charge code systems (FT1.7) | Map to a specific CodeSystem URI |
| Unit price / financial extension | Map FT1.11 (unit price) to a FHIR Money extension |
| Insurance linkage | Map to a Coverage resource |
| Claim assembly | DFT^P03 posts individual charges; if you need a FHIR Claim use the Da Vinci PAS template instead |
| US Core Condition profile compliance | Set profile in the Mapping step |

---

## Key field mappings (automatic)

| HL7 Field | FHIR Path | Notes |
|---|---|---|
| FT1.4 | ChargeItem.occurrenceDateTime | Service date |
| FT1.6 | ChargeItem.status | CG→billable, CD/CR→entered-in-error, PT→billed |
| FT1.7 + FT1.8 | ChargeItem.code | CE code + description |
| FT1.10 | ChargeItem.quantity.value | Number of units |
| FT1.19 | ChargeItem.reason[] | Repeating diagnosis CE codes (~ separated) |
| FT1.20 | ChargeItem.performer[0].actor.display | Performing provider (display only) |
| PR1.2 + PR1.3 | Procedure.code | Coding system (PR1.2) + code (PR1.3) |
| PR1.4 | Procedure.code.text | Procedure description |
| PR1.5 | Procedure.performedDateTime | Procedure date |
| PR1.12 | Procedure.performer[0].actor.display | Surgeon / performer (display only) |
| DG1.3 | Condition.code | CE → CodeableConcept (ICD system auto-resolved) |
| DG1.5 | Condition.recordedDate | Diagnosis date |
| DG1.6 | Condition.verificationStatus | F→confirmed, W/A→provisional |
| DG1.17 | Condition.asserter.display | Diagnosing clinician (display only) |

---

## FHIR R4 references

- [ChargeItem](https://hl7.org/fhir/R4/chargeitem.html)
- [Procedure](https://hl7.org/fhir/R4/procedure.html)
- [Condition](https://hl7.org/fhir/R4/condition.html)
$$
WHERE message_type = 'DFT^P03' AND is_system = true;

-- ─────────────────────────────────────────────────────────────────────────────
-- ORM^O01  — General Order Message
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE hl7_fhir_templates SET user_guide = $$
# ORM^O01 — General Order Message

**Clinical purpose**: Places orders for laboratory tests, radiology exams, or other services.  Each ORC + OBR group is one order; RXO segments carry medication orders.

---

## What works without the wizard

| HL7 Segment(s) | FHIR R4 Resource | Always / Conditional |
|---|---|---|
| MSH | MessageHeader | Always |
| PID | Patient | Always |
| PV1 | Encounter | When visit data is present |
| ORC + OBR | ServiceRequest | One per ORC/OBR group |
| ORC | Task | When task management data is present |
| RXO | MedicationRequest | When medication orders are present |
| DG1 | Condition | When diagnosis segments are present |
| OBX | Observation | When clinical context observations are present |

---

## What the wizard adds

| Need | What to configure |
|---|---|
| Custom order priority mapping | Override ORC.7 / OBR.27 value map |
| Specific LOINC codes for order panels | Map OBR.4 to LOINC system |
| Insurance pre-auth linkage | Map to a Coverage resource |
| US Core ServiceRequest profile | Set profile in the Mapping step |
| Medication request details | Map RXO fields beyond the OOB set |

---

## Key field mappings (automatic)

| HL7 Field | FHIR Path | Notes |
|---|---|---|
| ORC.1 | ServiceRequest.intent | NW→order, CA→revoked, DC→stopped |
| ORC.5 | ServiceRequest.status | IP→active, CM→completed, CA→revoked |
| OBR.4 | ServiceRequest.code | CE → CodeableConcept (LOINC resolved) |
| OBR.6 | ServiceRequest.requestedPeriod.start | Requested service date |
| OBR.14 | ServiceRequest.occurrenceDateTime | Specimen received date |
| ORC.7 | ServiceRequest.priority | R→routine, S→stat, A→asap |
| DG1.3 | Condition.code | CE → CodeableConcept |

---

## FHIR R4 references

- [ServiceRequest](https://hl7.org/fhir/R4/servicerequest.html)
- [Task](https://hl7.org/fhir/R4/task.html)
- [MedicationRequest](https://hl7.org/fhir/R4/medicationrequest.html)
$$
WHERE message_type = 'ORM^O01' AND is_system = true;
