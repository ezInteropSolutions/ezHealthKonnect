# FHIR Patient Relay to ezIdentitySense

## What this does

A system that already speaks FHIR (a FHIR server, an app, an integration
engine) sends its patients here as it would to any FHIR server. Each one is
passed on to ezIdentitySense, the enterprise master patient index, through
its IHE PIXm Patient Identity Feed, which registers or updates that system's
record and links it to the right enterprise identity.

| The system sends | What happens in ezIdentitySense |
|---|---|
| `PUT /fhir/r4/Patient/{id}` or `POST /fhir/r4/Patient` with a Patient | The record is registered, or updated if it exists |
| `DELETE /fhir/r4/Patient/{id}` | The record whose local ID is `{id}` is deactivated: it stops being matched, but stays linked, for history. Sending a newer version brings it back |

The Patient is sent on as received. Its **local ID** is its identifier whose
system is this source's ID authority (set at setup). A Patient without that
identifier is refused by ezIdentitySense. The Patient's `meta.lastUpdated` is
its version: ezIdentitySense ignores a Patient older than one it has already
applied, so a delayed or resent one never overwrites newer information.

Send one Patient at a time. A Bundle is not unpacked by this template.

## Choose how it answers

The setup form has one switch, **Answer with ezIdentitySense's own reply**.
It can be changed later in the interface's first step (the FHIR receiver),
under *How to answer a create, update or delete*.

### Off: acknowledge at once, send on in order (the default)

- The sender gets an immediate "accepted" (HTTP 201) and the Patient is sent
  to ezIdentitySense after, **one at a time, in the order received**.
- Every Patient appears in this interface's message list. If ezIdentitySense
  refuses one, it shows there as failed, with its reason, and can be
  re-run.
- The sender does not learn the result. If it must know, it has to look here
  or ask ezIdentitySense.

**Use it when** the sender sends a steady flow and does not act on each
answer: a nightly or continuous sync, a system that cannot wait, or where
the order of changes matters and a visible, re-runnable record of each
message is wanted.

### On: send first, answer with ezIdentitySense's own reply

- The sender waits while the Patient is sent, then gets what ezIdentitySense
  said, unchanged: its status code (200 or 201 when it filed the Patient,
  400 for a Patient it refuses, 404 for a delete of a record it does not
  have, and so on) and its OperationOutcome.
- Requests are handled **one at a time, in the order they arrive**, so
  changes cannot overtake each other; a busy relay answers 503 or 504
  rather than keep a sender waiting past **Give up after** (20 seconds by
  default).
- **Requests are not listed in this interface's message list**, so there is
  no re-run: the sender is told the outcome and is the one to retry.

**Use it when** a person or system must know at once whether the patient was
accepted: a registration screen, an app showing "saved" or an error, or a
sender that retries on failure. It suits low to moderate volume; for a bulk
history, use the file load template instead.

### Summary

| | Off (default) | On |
|---|---|---|
| Sender's answer | "accepted" | ezIdentitySense's own status and OperationOutcome |
| Order | In the order received | One at a time, in arrival order |
| Message list | Every message, with failures | Not listed |
| Re-run a failed one | Yes | The sender retries |
| Best for | Steady flows, queues, visible audit | Interactive senders that need the result |

## Setting up

1. Register a client for ezHealthKonnect in ezIdentitySense with the
   `records.write` scope, bound to the source system. Enter its address,
   token URL, client ID and secret in the form.
2. Enter this source's ID authority, exactly as set in ezIdentitySense.
3. Choose **Who may send** and enter its credential. Choose *Anyone* only to
   test: it carries patient data.
4. Pick a port in the range this installation publishes, not used by another
   interface.
5. Point the sending system at `http://<this host>:<port>/fhir/r4`.

## When something goes wrong

- **400 from ezIdentitySense** saying the Patient must carry the identifier:
  it has none in this source's ID authority (or the authority is not
  registered there).
- **200 "unchanged"** for a Patient older than one already applied: nothing
  was changed, and the sender can ignore it.
- **404 on a delete**: ezIdentitySense has no record with that local ID.
- **401 from this relay**: the sender's credential does not match the
  setting.
- **503 or 504 from this relay** (immediate-reply mode): the relay is busy
  or ezIdentitySense did not answer in time; the sender retries.
