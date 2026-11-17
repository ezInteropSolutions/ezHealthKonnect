# IHE PIX and PDQ over HL7 v2: what a responder must do

Requirements for answering **PIX Query (ITI-9, QBP^Q23 → RSP^K23)** and
**Patient Demographics Query (ITI-21, QBP^Q22 → RSP^K22)**, as the
Patient Identifier Cross-reference Manager and Patient Demographics
Supplier. Used to build and review the *HL7 PIX/PDQ Queries to
ezIdentitySense* template. Read from the published IHE Technical
Framework on 2026-11-10; re-check against the current revision before
claiming conformance.

Sources:
- ITI TF-2, 3.9 PIX Query [ITI-9], Rev 20.0 (2023-08-04):
  https://profiles.ihe.net/ITI/TF/Volume2/ITI-9.html
- ITI TF-2, 3.21 Patient Demographics Query [ITI-21], Rev 20.2 (2025-11-11):
  https://profiles.ihe.net/ITI/TF/Volume2/ITI-21.html
- ITI TF-2, Appendix C HL7 v2.x Messages, Rev 20.0 (2023-08-04):
  https://profiles.ihe.net/ITI/TF/Volume2/ch-C.html

## Common to both (Appendix C)

- MLLP; the responder answers "over the open connection". Original mode
  acknowledgment: the RSP **is** the acknowledgment — no separate ACK is
  sent before it (ITI-9 3.9.4.1.2, ITI-21 3.21.4.1.2).
- MSH: MSH-1, -2, -7, -9, -10, -11, -12 required; MSH-3, -4, -5, -6
  required by IHE (R+). MSH-18 only when not ASCII. MSH-21 recommended as
  `ITI-9^IHE` / `ITI-21^IHE`. HL7 v2.5.
- MSA: MSA-1 `AA` / `AE` / `AR`; **MSA-2 = the query's MSH-10**. MSA-3 and
  MSA-6 not supported — errors go in ERR.
- ERR: ERR-2 location (ERL), ERR-3 HL7 error code (table 0357, e.g.
  `204^Unknown key identifier^HL70357`), ERR-4 severity `E`/`W`/`I`
  (which one per case isn't specified; `E` for errors).
- An unrecognised message type or trigger event → `AR`, ERR-2
  `MSH^1^9^1^1` (type) or `…^2` (trigger), ERR-3 200 / 201.

## PIX Query (ITI-9)

Request QBP^Q23 (MSH-9 `QBP^Q23^QBP_Q21`): QPD-1 `IHE PIX Query`, QPD-2
query tag, QPD-3 one patient ID (CX; QPD-3.4 by namespace, or by
universal ID + type, or all three — the responder must accept every valid
combination), QPD-4 optional repeating "what domains returned" (each a CX
with only component 4). RCP-1 `I`.

Response RSP^K23 — MSH-9 `RSP^K23^RSP_K23`; segments
`MSH, MSA, [ERR], QAK, QPD, [PID]`:
- QPD: echo the request's QPD.
- QAK-1 the query tag; QAK-2 `OK`, `NF` or `AE`.
- PID (only when there are identifiers to return): **PID-3** one
  repetition per identifier in another domain — **not** the queried
  identifier — each with a fully qualified assigning authority (namespace
  ID, universal ID and universal ID type all valued); limited to the
  QPD-4 domains when given, else all domains (subject to local
  publication restrictions). **PID-5**: first repetition empty, second
  repetition only component 7 = `S` (`~^^^^^^S`). No other PID fields.

| Case | MSA-1 | QAK-2 | ERR | PID |
|---|---|---|---|---|
| 1, 6: found, identifiers in requested domains | AA | OK | — | yes |
| 2: found, none in requested domains | AA | NF | — | no |
| 3: domain known, patient ID unknown | AE | AE | `QPD^1^3^1^1`, 204 | no |
| 4: QPD-3 domain unknown | AE | AE | `QPD^1^3^1^4`, 204 | no |
| 5: a QPD-4 domain unknown | AE | AE | `QPD^1^4^<repetition>`, 204, one per domain | — |

## Patient Demographics Query (ITI-21)

Request QBP^Q22 (MSH-9 `QBP^Q22^QBP_Q21`): QPD-1 `IHE PDQ Query`; QPD-2
query tag; **QPD-3** repeating `@<seg>.<field>.<comp>.<sub>^value`
(e.g. `@PID.5.1.1^SMITH~@PID.8^F`); `@PID.3.1` requires `@PID.3.4`.
The supplier **shall support** PID.3, PID.5, PID.7, PID.8, PID.11 and
PID.18 as parameters (PID.6 and PID.13 for the Pediatric Demographics
option). **QPD-8** repeating domains to return (component 4 only).
RCP-1 `I`; RCP-2 `<n>^RD` quantity limit. The supplier **shall support
the HL7 continuation protocol** (DSC-1 pointer; cancel by QCN^J01).

Matching: return at least all exact matches; "best fit to all of the
search criteria"; phonetic/variant/case handling is the supplier's
choice.

Response RSP^K22 — MSH-9 `RSP^K22^RSP_K21`; segments
`MSH, MSA, [ERR], QAK, QPD, [{PID, [PD1], [QRI]}], [DSC]`:
- **MSH-3 = the query's MSH-5** (the patient information source that
  answered). Each response comes from a single source.
- QAK-1 = QPD-2; QPD echoed.
- One PID group per matching patient: PID-3 (domains limited by QPD-8),
  PID-5 required; PID-7, -8, -11, -18 when known; and every other PID
  field the supplier can supply. QRI optional.

| Case | MSA-1 | QAK-2 | ERR |
|---|---|---|---|
| 1, 2: patients found | AA | OK | — |
| 3: a QPD-8 domain unknown | AE | AE | `QPD^1^8^<repetition>`, 204, one per domain |
| no patient found | not stated in ITI-21; HL7 table 0208 `NF` | | |

## Not stated by these pages

ERR-4 per case; the zero-match codes for PDQ; MSH-4/5/6 of the
responses; PDQ result ordering and QRI content; how long continuation
results are kept.
