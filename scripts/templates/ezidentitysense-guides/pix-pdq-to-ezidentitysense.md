# HL7 PIX/PDQ Queries to ezIdentitySense

## What this does

Systems that ask the enterprise master patient index questions over HL7 v2
send their queries here over MLLP. Each query is answered from
ezIdentitySense on the same connection.

| Query | What it asks | Reply |
|---|---|---|
| PIX Query, `QBP^Q23` (IHE ITI-9) | "This patient is `000123` at HOSPA: what are their numbers elsewhere?" | `RSP^K23`: the patient's identifiers in the other domains, in PID-3 |
| Patient Demographics Query, `QBP^Q22` (IHE ITI-21) | "Find patients named SYNTHWELL born 1987-03-14" | `RSP^K22`: one PID per patient found, with their identifiers, names, birth date, sex, addresses, phone numbers and email |
| Cancel Query, `QCN^J01` | "Stop that demographics query" | `ACK^J01` |
| Anything else | | `ACK` with `AR` and an ERR saying what isn't supported |

It uses ezIdentitySense's standard IHE PIXm (`$ihe-pix`, ITI-83) and PDQm
(ITI-78) services, so what a query can find is exactly what those return.

## What the replies contain

- **MSH**: from the system the query was sent to, to the one that sent it
  (MSH-3/4 are the query's MSH-5/6), with MSH-21 `ITI-9^IHE` or
  `ITI-21^IHE`. **MSA-2** is the query's MSH-10; **QAK-1** its query tag;
  **QPD** is the query's own QPD, echoed.
- **PIX**: `AA`/`OK` with the identifiers (never the one asked about), each
  with its assigning authority fully qualified (namespace, OID, `ISO`), and
  PID-5 as IHE prescribes (`~^^^^^^S`). Only the domains asked for in QPD-4
  when it lists any. `AA`/`NF` when the patient has none there. `AE` with
  ERR when the identifier isn't known (`QPD^1^3^1^1`), its assigning
  authority isn't a known domain (`QPD^1^3^1^4`), or a requested domain isn't
  (`QPD^1^4`), each with error code 204.
- **PDQ**: `AA`/`OK` with one PID per patient, QAK-4 the total found; `NF`
  when nobody matches. Identifiers are limited to the domains in QPD-8 when
  it lists any. Search parameters supported in QPD-3: `@PID.3.1` with its
  assigning authority (`@PID.3.4`, or `.1`/`.2`/`.3`), `@PID.5.1.1` family
  and `@PID.5.2` given name (a trailing `*` is allowed, names match from the
  start), `@PID.6.1.1` mother's maiden name, `@PID.7` birth date (a year, a
  month or a day), `@PID.8` sex, `@PID.11.1/.3/.4/.5/.6` address, `@PID.13`
  phone or email. A parameter it doesn't support (for example `@PID.18`, the
  account number, which ezIdentitySense doesn't keep) is answered `AE`
  rather than ignored, so a search is never silently widened.
- **Enough to search on:** ezIdentitySense searches only on an identifier,
  or on the family name together with the birth date (or with the sex, if
  that is switched on in ezIdentitySense). A query with less is answered
  `AE`, error 101 at `QPD^1^3`, saying what is missing.
- **More results than asked for** (RCP-2, e.g. `10^RD`; 10 when not given,
  at most 50): the reply ends with `DSC|<pointer>|I`. Send the same query
  again with that pointer in DSC-1 for the next ones. A pointer works for
  10 minutes, only for the same sender and query tag; `QCN^J01` with the
  query tag in QID-1 drops it. An unknown or expired pointer is answered
  `AE` (`DSC^1^1`).
- If ezIdentitySense can't be reached or refuses the call, the reply is `AE`
  with code 207, never silence.

## Before you start

1. **In ezIdentitySense**, as an integration administrator, open
   **Integrations**:
   - The **domains** a PIX query can name are ezIdentitySense's own
     enterprise ID system and each source system's **ID authority**.
     Identifiers in other domains aren't cross-referenced.
   - Under **API clients**, register an OAuth client for ezHealthKonnect
     with the scopes `identities.read`, `identities.search` and
     `identities.read.demographics`. Without the last one, demographics
     replies carry no names, dates or addresses.
2. **Assigning authorities** in queries are best sent with their OID
   (`000123^^^HOSPA&2.16.840.1.113883.3.999.1&ISO`). One sent by namespace
   only (`000123^^^HOSPA`) is looked up in the table below; one that is in
   neither is answered as unknown.

## Setup

Click **Use** on this template and fill in:

- **MLLP Port**: where the querying systems connect.
- **Assigning Authorities by Namespace** (optional): one line per domain,
  `NAMESPACE=OID`, e.g. `HOSPA=2.16.840.1.113883.3.999.1`, or
  `NAMESPACE=URI` for a domain whose system isn't an OID. Replies name each
  domain by its namespace from this table; a domain not in it is named by
  its OID.
- **ezIdentitySense Connection**: its address, and the OAuth client's token
  URL, client ID and secret (and audience if your provider needs one).

Activate the interface, then give the querying systems this server and port.

## Changing things later

- **Rotating the client secret:** edit *PIX Lookup* and *PDQ Search* in
  Pipeline Builder.
- **Adding a namespace:** edit the `namespaces` table in *PIX Query* and
  *PDQ Query*, and the reversed one (OID to namespace) in the PID-3
  mappings of *PIX Response* and *PDQ Response*.
- **Search parameters:** the *PDQ Query* step's `parameters` table maps each
  `@PID…` field to a PDQm search parameter.

## Good to know

- Every reply is built by this interface itself, so each query is answered
  in about the time ezIdentitySense takes; the connection waits up to
  10 seconds.
- A PIX reply can't say *which* of several requested domains ezIdentitySense
  doesn't know: its ERR points at QPD-4 as a whole. One that can't be
  resolved here at all (a namespace not in the table, with no OID) is named
  by its repetition.
- Enterprise IDs are sent as they are; with the default hyphenated format
  they are longer than HL7 v2.5's 15-character CX.1. Systems that enforce
  that limit should be given a shorter EID format in ezIdentitySense.
- Feeding patients *into* ezIdentitySense uses the *HL7 ADT to
  ezIdentitySense* template; sending its changes out uses *ezIdentitySense
  Patient Events to HL7 ADT*.
- This template hasn't been tested with Gazelle or at an IHE Connectathon,
  and claims no IHE conformance.
