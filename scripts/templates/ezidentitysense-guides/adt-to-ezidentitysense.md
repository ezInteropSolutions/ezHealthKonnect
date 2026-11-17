# HL7 ADT to ezIdentitySense

## What this does

A source system (an EHR, a registration system, a lab) sends its HL7 v2 ADT
messages here over MLLP. Each one updates that system's patient record in
ezIdentitySense, the enterprise master patient index, which links it to the
right enterprise identity.

| ADT message | What happens in ezIdentitySense |
|---|---|
| A01, A04, A05, A08, A28, A31, and any other ADT carrying PID | The record is registered, or updated if it exists |
| A40 (merge patient), and the older A30, A34 and A18 | The record in MRG-1 is merged, in the source, into the record in PID-3 |
| A47 (change patient identifier list) | PID-3 is the new identifier and MRG-1 the old one: the new record is registered, then the old record is merged into it |
| A23 (delete visit), A29 (delete person) | The record is removed from matching |
| A24 (link), A37 (unlink), A35, A36 and A41 to A45 (account-level and visit moves) | **Refused, with a NACK.** See "Links and account events" below |

The event meanings follow HL7 v2.5.1 chapter 3 (Patient Administration);
the identifier change is HL7's A47 with the old identifier in MRG-1. Which
trigger events a given source sends is for the source to say: send only what
it really uses.

It uses ezIdentitySense's standard IHE PIXm Patient Identity Feed (ITI-104),
sending the patient as a FHIR Patient built from PID: every identifier in
PID-3, every name in PID-5 with its type (legal, maiden, …), birth date,
sex, addresses, phone numbers and email, and mother's maiden name (PID-6).

## How delivery works

- Messages are processed **one at a time, in the order received**. The
  sending system gets its MLLP acknowledgement only when ezIdentitySense has
  finished with the message: `AA` when it was filed, `AE` when it failed or
  nothing was heard within the wait (30 seconds by default; **ACK wait** in
  the listener's acknowledgement settings). A sender that resends on `AE`
  can deliver a message twice; that is safe, because a message older than
  one already applied is ignored (see the next point).
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

- **Rotating the client secret:** edit the five steps that call
  ezIdentitySense (*Register Patient*, *Merge Record*, *Remove Record*,
  *Register New ID*, *Merge Old ID*) in Pipeline Builder.
- **What an event does:** the setup form's **What Each ADT Event Does**
  takes one `EVENT=ACTION` per line, and what you write wins over the
  defaults above. The actions are `register`, `merge`, `remove`,
  `replace_id` (identifier changed), `ignore` (acknowledge and do nothing)
  and `unsupported` (refuse with a NACK). For example `A02=ignore` stops
  transfers updating demographics, and `A34=ignore` drops the older merge.
  An event with no line, and no default, registers the PID's patient.

## Links and account events {#links}

HL7 link (A24) and unlink (A37) say two patient records belong to the same
person, or don't, while both stay alive. ezIdentitySense decides who is the
same person by matching and by a steward's review, never because a source
says so, so these events are refused with a NACK rather than silently
dropped: the sender sees them fail. If your source sends them and you want
them acknowledged, set the event to `ignore` in the setup form. A37 is
also not turned into an unmerge, because it is not said which of its two
patients was the merged one; undo a source merge through ezIdentitySense's
unmerge call or a steward.

Account-level events (A35, A36, A41 to A45) concern accounts and visits,
not who the patient is, and are refused the same way.

## Good to know

- A47's and A40's MRG-1 are read from its first repetition, which must be the retired
  (A47: the old) record's number in this source's ID authority. If the old
  record was never sent to ezIdentitySense, the merge fails visibly.
  An A47 is two calls, in order: the new identifier is registered first, then
  the old record is merged into it. If the second fails, the message is
  refused (NACK) with the new record already registered; resending it
  finishes the job, and registering twice changes nothing.
- A message whose PID-3 has no identifier in this source's ID authority
  can't be filed and fails; check the source's assigning authority settings.
- A17 (swap patients) and other two-patient events aren't handled as such:
  they update the first patient only.
- Sending data *out* of ezIdentitySense uses the *ezIdentitySense Patient
  Events to HL7 ADT* template; PIX and PDQ queries use *HL7 PIX/PDQ Queries
  to ezIdentitySense*.
