# ezIdentitySense FHIR Subscription to HL7 ADT

## What this does

ezIdentitySense, the enterprise master patient index, publishes its patient
changes as standard FHIR topic-based subscriptions (HL7 *Subscriptions R5
Backport* on FHIR R4). This interface subscribes to them and turns each
change into an HL7 v2 ADT message for a downstream system (an EHR, a lab or
radiology system, a data warehouse), sent over MLLP.

| Topic in ezIdentitySense | Message sent |
|---|---|
| `patient-created`: a new patient identity | ADT^A28 (add person) |
| `patient-changed`: a source system sent an updated record, or a steward corrected the golden record | ADT^A31 (update person) |
| `patient-records`: a record was linked, moved or removed | ADT^A31 (update person) |
| `patient-merged`: two identities became one | ADT^A40 (merge), with the retired identity in MRG |
| `patient-separated`: a merge was undone, or an identity split | ADT^A37 (unlink), with both identities' PID |
| `patient-erased`: an identity was erased | ADT^A29 (delete person), enterprise ID only |

Each notification carries the patients concerned, as their golden record
(*full-resource*), so this interface never has to call ezIdentitySense back
for them. PID carries name, birth date, sex, addresses, phone numbers,
mother's maiden name, and every identifier with its assigning authority.

## How delivery works

- **The subscriptions are managed for you.** When the interface starts it
  creates one subscription per topic in ezIdentitySense and renews them
  every 12 hours, so a subscription that was paused, edited or deleted is
  put right. Each subscription carries a token only this interface can
  produce, which ezHealthKonnect checks on every notification.
- **Events are numbered and handled one at a time, in order.**
  ezHealthKonnect answers ezIdentitySense only once the downstream system has
  accepted the message (MLLP `AA`). An event is never skipped and never sent
  twice. The order is kept **within each topic**: each topic is its own
  subscription, so if the downstream system rejects one patient's A28, that
  patient's A31 (another topic) can still go ahead of it. Fix the rejection
  and the held message follows.
- **Nothing is lost.** If a notification shows a gap in the numbers (one was
  missed while this interface was stopped, for example), the missing events
  are fetched from ezIdentitySense first and processed in order. If the
  downstream system is down, ezIdentitySense retries later, starting from
  that event.
- Every event appears in this interface's message list like any other
  message, with the ADT that was sent.

## Before you start

You need:

1. **ezHealthKonnect reachable from ezIdentitySense** over HTTPS on the
   listen port you choose below (put it behind your TLS proxy or load
   balancer; ezIdentitySense refuses plain-HTTP addresses outside
   development).
2. **An OAuth 2.0 client for ezHealthKonnect** in the identity provider
   ezIdentitySense trusts, using the client-credentials grant. Note its
   token URL, client ID and secret (and audience, if your provider uses
   one).
3. **The downstream system's MLLP host and port.**

## Step 1: in ezIdentitySense

Sign in as an integration administrator and open **Integrations**.

1. Under **API clients**, register the OAuth client from "Before you start"
   (same client ID). It needs the `identities.subscribe`,
   `identities.read` and `identities.read.demographics` scopes, and it
   should be bound to the source systems whose patients the downstream
   system may see.
2. Under **Subscription topics**, check that the six patient topics are
   switched on. If the client's topics are limited, allow it all six (or
   remove the ones you don't want from this interface's *Topics* setting).

That's all: you don't create the subscriptions yourself.

## Step 2: in ezHealthKonnect

Click **Use** on this template and fill in:

- **Listen Port**, and the **Address ezIdentitySense Sends To**: the https
  address that reaches that port, ending in `/fhir-notifications`.
- **ezIdentitySense FHIR Address**: its FHIR base, ending in `/fhir/r4`.
- The OAuth client's **Token URL**, **Client ID**, **Client Secret** and
  **Audience**.
- Optionally the receiving application and facility for MSH-5 and MSH-6.
- **Downstream System Host** and **Port**.

Then activate the interface. Within a few seconds its subscriptions show on
ezIdentitySense's **Integrations** page as *active*; changes from then on
are sent.

## Choosing the ADT event for a change

Setup has an optional **Which ADT Event Each Change Sends** table, one
`CHANGE=EVENT` per line, for a receiver that expects other trigger events
than the table above. The changes are the topic names
`patient-created`, `patient-changed` and `patient-records`. The events you
may name are A28, A31, A04 and A08: they carry the same patient and visit
segments, so a new identity can be sent as A04 and an update as A08, for
instance `patient-changed=A08`. Merges (A40), unlinks (A37) and deletes (A29)
always keep their own events, because their messages carry segments the
others lack, and ezIdentitySense has no change that means "link" (A24) or
"identifier changed" (A47). Naming any other event makes that change fail
visibly, with the list of events this interface can send, instead of sending
the wrong message.

## Changing things later

- **Only some patients:** set *Filters* on the *ezIdentitySense
  Subscriptions* step, for example `source-system=EHR` (patients with a
  record from that source system) or `patient=EID-…`. The subscriptions are
  updated when the interface restarts.
- **Fewer topics:** remove them from *Topics* on the same step.
- **Rotating the client secret:** edit the *ezIdentitySense Subscriptions*
  step. The subscriptions' tokens change with it and are renewed at once
  when the interface restarts.
- **Field mappings:** the *Build ADT* step maps the patient onto PID; adjust
  it in Pipeline Builder like any HL7 Build step.

## Good to know

- Messages describe the patient **as they are when ezIdentitySense sends
  the notification**, so a quick run of changes may show the same latest
  details more than once. That is what downstream systems need.
- A new patient produces an A28, then an A31 when its first record is linked.
- In an A40 the PID is the surviving identity and MRG-1 the retired one; in
  an A37 the first PID is the identity named by the event, the second the
  one separated from it.
- **Enterprise IDs are longer than HL7 v2.5.1 allows for an identifier.**
  ezIdentitySense's IDs (e.g. `EID-7K3M-9QX2-TB4R`, 18 characters) exceed the
  15-character limit v2.5.1 sets on PID-3.1 and MRG-1.1. Most systems accept
  them; check with your receiving system if it validates lengths strictly.
- An A29 for an erased identity carries its enterprise ID only: the name and
  every other detail were erased with it, so PID-5 (required in v2.5.1) is
  empty.
- Countries are sent as the three-letter codes of HL7 table 0399 (`USA`),
  converted from the two-letter codes ezIdentitySense stores (`US`).
- ezHealthKonnect's own HL7 validator reports an A37 as "PID appears 2
  times": its schema files can't express the second PID that ADT_A37
  defines. The message itself is laid out as the standard defines it.
- Email addresses are not sent (PID-13 carries phone numbers).
- The older *ezIdentitySense Patient Events to HL7 ADT* template does the
  same from ezIdentitySense's CloudEvents webhooks; use one or the other for
  a downstream system, not both. To keep another FHIR server's patients
  current instead, use *ezIdentitySense FHIR Subscription to FHIR Server*.
