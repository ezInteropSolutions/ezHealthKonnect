# ezIdentitySense Patient Events to HL7 ADT

## What this does

ezIdentitySense, the enterprise master patient index, tells ezHealthKonnect
whenever a patient identity changes. This interface turns each change into an
HL7 v2 ADT message and sends it to a downstream system (an EHR, a lab or
radiology system, a data warehouse) over MLLP.

| What happened in ezIdentitySense | Message sent |
|---|---|
| A new patient identity was created | ADT^A28 (add person) |
| A record was linked, updated, moved or deactivated, or a steward corrected the golden record | ADT^A31 (update person) |
| Two identities were merged | ADT^A40 (merge), with the retired identity in MRG |
| A merge was undone, or an identity was split | ADT^A37 (unlink), naming both identities |
| An identity was erased | ADT^A29 (delete person) |

Each message carries the patient's golden record as ezIdentitySense shows it
at the time the message is built: name, birth date, sex, addresses, phone
numbers, mother's maiden name, and every identifier in PID-3 (the enterprise
ID first, then each linked record's local ID and other identifiers, each with
its assigning authority).

## How delivery works

- ezIdentitySense sends events in order, signed with a shared secret.
  ezHealthKonnect checks the signature on every request.
- Events are handled **one at a time**. ezHealthKonnect answers
  ezIdentitySense only after the downstream system has accepted the message
  (MLLP `AA`). An event is never skipped and never sent twice.
- If the downstream system is down or rejects a message, ezHealthKonnect
  answers with an error and ezIdentitySense tries again later, starting from
  that event. Nothing after it is sent first. ezIdentitySense keeps retrying
  for its retry window (24 hours by default), then pauses the webhook without
  losing anything; resume it on ezIdentitySense's **Integrations** page once
  the problem is fixed.
- Every event appears in this interface's message list like any other
  message, with the ADT that was sent.

## Before you start

You need:

1. **ezHealthKonnect reachable from ezIdentitySense** over HTTPS on the
   webhook port you choose below (put it behind your TLS proxy or load
   balancer; ezIdentitySense refuses plain-HTTP webhook addresses outside
   development).
2. **An OAuth 2.0 client for ezHealthKonnect** in the identity provider
   ezIdentitySense trusts, using the client-credentials grant. Note its
   token URL, client ID and secret (and audience, if your provider uses
   one).
3. **The downstream system's MLLP host and port.**

## Step 1: in ezIdentitySense

Sign in as an integration administrator and open **Integrations**.

1. Under **API clients**, register the OAuth client from "Before you start"
   (same client ID). It needs the `identities.read` and
   `identities.read.demographics` scopes, and it should be bound to the
   source systems whose patients the downstream system may see.
2. Create a **webhook**:
   - **Endpoint:** `https://<ezHealthKonnect address>:<webhook port>/events`
   - **Events:** `identity.created`, `record.linked`, `record.relinked`,
     `record.updated`, `record.deactivated`, `identity.golden-changed`,
     `identity.merged`, `identity.unmerged`, `identity.split`,
     `identity.purged`. (Other event types, such as review-task events, are
     ignored by this interface anyway, so choosing "all" also works.)
   - Copy the **signing secret** (`whsec_…`). It is shown once.

If you want the downstream system to see each source system's own local
record number in PID-3, give that source system an **ID authority** in
ezIdentitySense; records of a source without one are listed by their other
identifiers only.

## Step 2: in ezHealthKonnect

Click **Use** on this template and fill in:

- **Webhook Port** and **Webhook Signing Secret** (from step 1).
- **ezIdentitySense Connection:** its address, and the OAuth client's token
  URL, client ID, secret and audience. Optionally the receiving application
  and facility for MSH-5 and MSH-6.
- **Downstream System Host** and **Port**.

Then activate the interface. ezIdentitySense starts delivering new events
straight away; use **Send test event** on the webhook page to check the
connection.

## Changing things later

- **Rotating the ezIdentitySense client secret:** edit the *Fetch Patient*
  step in Pipeline Builder.
- **Rotating the webhook signing secret:** rotate it on ezIdentitySense's
  webhook page; it signs with both the old and the new secret until you drop
  the old one, so update this interface's *ezIdentitySense Events* step
  (Signing Secret) at any point in between.
- **Field mappings:** the *Build ADT* step maps the golden record onto PID;
  adjust it in Pipeline Builder like any HL7 Build step.

## Good to know

- Messages describe the patient **as they are now**, not as they were when
  the event happened: if several updates arrive close together, each A31
  carries the latest golden record. That is what downstream systems need.
- A new patient produces an A28 followed by an A31 (the identity is created,
  then its first record is linked).
- MRG-1 carries the retired enterprise ID without its assigning authority;
  PID-3 of the same message names the authority of enterprise IDs.
- Email addresses are not sent (PID-13 carries phone numbers).
- This interface sends **from** ezIdentitySense. To feed ADT **into**
  ezIdentitySense, use the *HL7 ADT to ezIdentitySense* template; for PIX
  and PDQ queries, use *HL7 PIX/PDQ Queries to ezIdentitySense*.
