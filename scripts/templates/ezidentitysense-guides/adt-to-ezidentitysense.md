# HL7 ADT to ezIdentitySense

## What this does

A source system (an EHR, a registration system, a lab) sends its HL7 v2 ADT
messages here over MLLP. Each one updates that system's patient record in
ezIdentitySense, the enterprise master patient index, which links it to the
right enterprise identity.

| ADT message | What happens in ezIdentitySense |
|---|---|
| A01, A04, A05, A08, A28, A31, and any other ADT carrying PID | The record is registered, or updated if it exists |
| A40 (merge patient) | The record in MRG-1 is merged, in the source, into the record in PID-3 |
| A23 (delete visit), A29 (delete person) | The record is removed from matching |

It uses ezIdentitySense's standard IHE PIXm Patient Identity Feed (ITI-104),
sending the patient as a FHIR Patient built from PID: every identifier in
PID-3, every name in PID-5 with its type (legal, maiden, …), birth date,
sex, addresses, phone numbers and email, and mother's maiden name (PID-6).

## How delivery works

- Messages are acknowledged on receipt (MLLP `AA`) and then processed **one
  at a time, in the order received**.
- MSH-7 (the message date/time) is sent as the record's version.
  ezIdentitySense ignores a message older than one it has already applied,
  so a delayed or resent message never overwrites newer information. If
  MSH-7 has no time zone it is taken as UTC; keep each source consistent.
- A message ezIdentitySense refuses (an unknown assigning authority, a
  record it can't read) is marked **failed** in this interface's message
  list with ezIdentitySense's reason. Fix the cause and reprocess it from
  there.

## Before you start

1. **In ezIdentitySense**, as an integration administrator, open
   **Integrations**:
   - Under **Source systems**, the source system for this feed must have an
     **ID authority**: the assigning authority of its own patient numbers
     (its MRNs), e.g. `urn:oid:2.16.840.1.113883.3.999.1`. That is how
     ezIdentitySense knows which identifier in PID-3 is the record's local
     ID.
   - Under **Assigning authorities**, register every other authority whose
     identifiers the source sends in PID-3 (national IDs, insurance
     numbers…). An identifier from an unregistered authority is refused.
     Social-security-type numbers need ezIdentitySense's sensitive
     identifier storage to be set up first.
   - Under **API clients**, register an OAuth client for ezHealthKonnect
     with the `records.write` scope, bound to this source system.
2. **Identifiers in PID-3** must name their assigning authority by OID
   (`MRN^^^HOSP&2.16.840.1.113883.3.999.1&ISO^MR`): the OID becomes
   `urn:oid:…`, and must match what ezIdentitySense has.

## Setup

Click **Use** on this template and fill in:

- **MLLP Port**: where the source system sends.
- **This Source's ID Authority**: exactly as set in ezIdentitySense.
- **ezIdentitySense Connection**: its address, and the OAuth client's token
  URL, client ID and secret (and audience if your provider needs one).

Activate the interface, then point the source system's ADT feed at this
server and port.

## Changing things later

- **Rotating the client secret:** edit the four steps that call
  ezIdentitySense (*Register Patient*, *Merge Record*, *Remove Record A23*,
  *Remove Record A29*) in Pipeline Builder.
- **Which events merge or remove:** the *ADT Event* step decides by MSH-9.2;
  add a case there to route another trigger event.

## Good to know

- A40's MRG-1 is read from its first repetition, which must be the retired
  record's number in this source's ID authority.
- A message whose PID-3 has no identifier in this source's ID authority
  can't be filed and fails; check the source's assigning authority settings.
- A17 (swap patients) and other two-patient events aren't handled as such:
  they update the first patient only.
- Sending data *out* of ezIdentitySense uses the *ezIdentitySense Patient
  Events to HL7 ADT* template; PIX and PDQ queries use *HL7 PIX/PDQ Queries
  to ezIdentitySense*.
