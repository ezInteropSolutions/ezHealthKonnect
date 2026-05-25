-- V92: Add user_guide column to interface_templates for in-app documentation.
-- Stores markdown text rendered in the template gallery "Guide" tab.
-- No Flyway checksum issue — new column with NULL default, non-breaking.

ALTER TABLE interface_templates
    ADD COLUMN IF NOT EXISTS user_guide TEXT;

-- ─────────────────────────────────────────────────────────────────────────────
-- Da Vinci PAS user guide
-- ─────────────────────────────────────────────────────────────────────────────
UPDATE interface_templates
SET user_guide = $$
# Da Vinci PAS — Prior Authorization Support

**Standard**: HL7 Da Vinci PAS IG STU2 (CMS-0057-F)
**Purpose**: Submit electronic prior authorization requests to payers and receive real-time decisions via FHIR R4.

---

## What This Template Does

1. You provide patient, coverage, provider, and service details in **Zone 1** (the user_mapping step at sequence 10)
2. The pipeline assembles a conformant FHIR R4 Bundle (Claim + Patient + Coverage + Practitioner + Organization)
3. The bundle is submitted to the payer FHIR `$submit` endpoint via HTTP POST with OAuth2 authentication
4. The payer response is parsed and routed to Approved, Denied, Partial, or Pended branches automatically

---

## Prerequisites

Before using this template you need:

| What | Where to find it |
|---|---|
| Payer FHIR base URL | Payer developer portal or trading partner agreement |
| OAuth2 client_id | Issued by payer during registration |
| OAuth2 client_secret | Issued by payer during registration |
| OAuth2 token URL | Payer developer portal |
| Patient demographics | Your EHR or source system |
| Member or coverage ID | Insurance card or eligibility system |
| Provider NPI | NPPES registry (nppes.cms.hhs.gov) |
| Organization NPI | NPPES registry (nppes.cms.hhs.gov) |

---

## Zone 1 — Field Reference

Open the pipeline in **Pipeline Builder**, click the **user_mapping** step (sequence 10), and fill in each field. These are the only fields you need to configure — the rest of the pipeline runs automatically.

| Field | Description | Format or Example |
|---|---|---|
| `patient_first_name` | Patient given name | John |
| `patient_last_name` | Patient family name | Smith |
| `patient_dob` | Date of birth | YYYY-MM-DD: 1980-03-15 |
| `patient_gender` | Gender code | male, female, other, unknown |
| `patient_id` | Your MRN or patient identifier | MRN-12345 |
| `member_id` | Insurance member ID | INS-98765 |
| `payer_name` | Payer display name | Acme Health Plan |
| `payer_id` | Payer NPI or identifier | 1234567890 |
| `provider_npi` | Requesting provider NPI (10 digits) | 9876543210 |
| `provider_name` | Provider display name | Dr. Jane Doe |
| `org_npi` | Organization NPI (10 digits) | 1122334455 |
| `org_name` | Organization display name | City Medical Center |
| `service_code` | CPT or HCPCS procedure code | 27447 |
| `service_description` | Human-readable service description | Total knee replacement |
| `icd10_code` | Primary diagnosis ICD-10 code | M17.11 |
| `icd10_description` | Diagnosis description | Primary osteoarthritis, right knee |
| `service_start_date` | Requested service date | YYYY-MM-DD: 2026-05-01 |
| `units_requested` | Number of units or visits | 1 |
| `priority` | Request priority | normal, urgent, stat |

---

## Payer HTTP Configuration

In **Pipeline Builder**, click the **submit_to_payer** step (sequence 150) and configure the connection fields:

| Config Field | Value |
|---|---|
| Endpoint | https://your-payer.com/fhir/r4/Claim/$submit |
| Method | POST |
| Auth type | oauth2_client_credentials |
| Token URL | https://your-payer.com/oauth/token |
| Client ID | (from payer registration) |
| Client Secret | (from payer registration) |
| Scope | pas (or as specified by payer) |

The request body is the FHIR Bundle assembled by the previous step — you do not need to configure it separately.

---

## Decision Codes

After the pipeline runs, the prior authorization decision is available in `steps.parse_response.step_output`:

| Field | Value | Meaning |
|---|---|---|
| `decision` | AA | Approved — services are authorized |
| `decision` | AD | Denied — payer declined the request |
| `decision` | CP | Partial — some items approved, some denied |
| `decision` | PE | Pended — under manual review, check back later |
| `review_action` | A1 to A6 | Raw X12 005010 review action code from payer |
| `error_message` | string | Present when payer returns an error response |

The **route_decision** step (sequence 210) routes the message to the correct branch based on the decision automatically.

---

## Testing the Pipeline

1. Open the interface in **Pipeline Builder**
2. Click **Test** in the toolbar to run with sample input data
3. Review the step-by-step output panel — each step shows its input, output, and timing
4. Check the `steps.parse_response.step_output.decision` value to verify routing works

For offline testing without a real payer, point the HTTP step at a mock FHIR server (such as HAPI FHIR or Smile CDR) configured to return a sample ClaimResponse bundle.

---

## Compliance Note

This template implements the FHIR API submission layer required by **CMS-0057-F** and the **HL7 Da Vinci PAS IG STU2**. The payer handles the internal X12 5010/306 translation. Your responsibilities:

- Submit a conformant FHIR R4 Bundle containing Claim, Patient, Coverage, Practitioner, and Organization resources
- Handle pended decisions — either poll for updates or implement the `$inquire` operation (not included in this template)
- Maintain audit logs of all prior authorization requests — automatically handled by the ezHealthKonnect audit trail
$$
WHERE slug = 'davinci-pas-r4';
