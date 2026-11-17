# ezIdentitySense FHIR Subscription to FHIR Server

## What this does

ezIdentitySense, the enterprise master patient index, publishes its patient
changes as standard FHIR topic-based subscriptions (HL7 *Subscriptions R5
Backport* on FHIR R4). This interface subscribes to them and keeps another
FHIR server's copy of the patients current: a clinical data repository, a
FHIR facade, an analytics store.

Each patient is written there as its **golden record**, the Patient
ezIdentitySense shows, under its enterprise ID (`Patient/EID-…`).

| Topic in ezIdentitySense | What is written |
|---|---|
| `patient-created`, `patient-changed`, `patient-records` | `PUT Patient/{EID}`: the Patient as it is now |
| `patient-merged` | `PUT` the surviving Patient, then the retired one (inactive, with a `replaced-by` link to the survivor) |
| `patient-separated` | `PUT` both Patients as they are now |
| `patient-erased` | `DELETE Patient/{EID}` (already gone is fine) |

## How delivery works

- **The subscriptions are managed for you.** When the interface starts it
  creates one subscription per topic in ezIdentitySense and renews them
  every 12 hours, so a subscription that was paused, edited or deleted is
  put right. Each subscription carries a token only this interface can
  produce, which ezHealthKonnect checks on every notification.
- **Events are numbered and handled one at a time, in order.**
  ezHealthKonnect answers ezIdentitySense only once the target server has
  accepted every write for the event. An event is never skipped. The order
  is kept **within each topic**: each topic is its own subscription, so a
  write the target refuses holds up only its own topic. Each write is the
  Patient as it is now, so the target ends up current either way.
- **Nothing is lost.** If a notification shows a gap in the numbers, the
  missing events are fetched from ezIdentitySense first and processed in
  order. If the target server is down, ezIdentitySense retries later,
  starting from that event. Writes are `PUT` and `DELETE`, so doing one
  again is harmless.
- Every event appears in this interface's message list like any other
  message, with each write made.

## Before you start

You need:

1. **ezHealthKonnect reachable from ezIdentitySense** over HTTPS on the
   listen port you choose below (put it behind your TLS proxy or load
   balancer; ezIdentitySense refuses plain-HTTP addresses outside
   development).
2. **An OAuth 2.0 client for ezHealthKonnect** in the identity provider
   ezIdentitySense trusts, using the client-credentials grant: its token
   URL, client ID and secret (and audience, if your provider uses one).
3. **The target FHIR server**: its base address, and an OAuth 2.0 client
   there allowed to write Patients (SMART backend services or any
   client-credentials grant). **It must allow client-assigned ids** (update
   as create, `PUT` to a Patient that doesn't exist yet). HAPI FHIR does by
   default; on a cloud FHIR service check its setting.

## Step 1: in ezIdentitySense

Sign in as an integration administrator and open **Integrations**.

1. Under **API clients**, register the OAuth client from "Before you start"
   (same client ID). It needs the `identities.subscribe`,
   `identities.read` and `identities.read.demographics` scopes, and it
   should be bound to the source systems whose patients the target server
   may hold.
2. Under **Subscription topics**, check that the six patient topics are
   switched on. If the client's topics are limited, allow it all six.

That's all: you don't create the subscriptions yourself.

## Step 2: in ezHealthKonnect

Click **Use** on this template and fill in:

- **Listen Port**, and the **Address ezIdentitySense Sends To**: the https
  address that reaches that port, ending in `/fhir-notifications`.
- **ezIdentitySense FHIR Address**: its FHIR base, ending in `/fhir/r4`.
- The ezIdentitySense OAuth client's **Token URL**, **Client ID**, **Client
  Secret** and **Audience**.
- **Target FHIR Server**: its base address, and its OAuth client's token
  URL, client ID, secret and, if it needs one, scope (by default
  `system/Patient.write`).

Then activate the interface. Within a few seconds its subscriptions show on
ezIdentitySense's **Integrations** page as *active*; changes from then on
are written.

## Changing things later

- **Only some patients:** set *Filters* on the *ezIdentitySense
  Subscriptions* step, for example `source-system=EHR`. The subscriptions are
  updated when the interface restarts.
- **Rotating a secret:** ezIdentitySense's is on the *ezIdentitySense
  Subscriptions* step; the target server's is on each of the six write
  steps (*Put Patient*, *Put Survivor*, *Put Retired*, *Put Restored*, *Put
  Separated*, *Delete Patient*).

## Good to know

- This keeps patients **from now on**. To fill the target with the patients
  ezIdentitySense already holds, take a FHIR Bulk Data export from
  ezIdentitySense (`Patient/$export`) and load it there first; then
  activate this interface.
- Patients are written **as they are when ezIdentitySense sends the
  notification**, so a quick run of changes may write the same latest
  details more than once.
- A retired (merged-away) Patient is kept, inactive, so references to it in
  the target's other resources still resolve; follow its `replaced-by` link
  to the survivor.
- The Patients carry identifiers with ezIdentitySense's assigning-authority
  systems. If the target validates against a profile that requires other
  systems, adjust in Pipeline Builder.
- To send HL7 v2 ADT instead, use *ezIdentitySense FHIR Subscription to HL7
  ADT*.
