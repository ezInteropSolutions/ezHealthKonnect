# UAT Functional Test Cases — by Persona and Workflow

Companion to [QA_STRATEGY.md](QA_STRATEGY.md) §6. These are **functional acceptance tests**: a person
with a goal uses a feature and the result must be what they expect. Each case is written from the
user's side (what they do, what they see), not from the code's. Cases were derived from the product's
real surface — the navigation, every page's controls (captured from the live pages), and
`docs/CAPABILITIES_GUIDE.md`.

**Legend** — Pri: P1 must pass to release · P2 should pass · P3 nice to have.
Auto: `A` automated and run (`tests/playwright/uat-functional.spec.js`) · `P` planned — automatable, not yet built · `M` manual only.
Result column is filled by the run log at the bottom (Pass / Fail / Blocked / Not run).

## Personas

| ID | Persona | Role | Goal |
|---|---|---|---|
| PRIYA | IT administrator | admin / super_admin | Install, secure, maintain the system and its users |
| MARCUS | Integration engineer | operator / engineer | Build, change and operate interfaces and pipelines |
| DANA | Operations analyst | operator | Watch feeds, find lost messages, handle alerts and failures |
| SAM | Compliance officer | admin | Prove who did what, control who sees data |
| LEE | Clinical-informatics viewer | viewer | Look, never change |
| PAT | Basic user | user | Use the HL7 Reader on their own data |
| NEO | New hire | invited user | Learn the product with no help |
| PARTNER | Trading-partner system | partner credential | Machine calls to real-time endpoints |
| EVE | Careless / hostile user | any | Wrong input, curiosity, probing limits |

## Conventions
Every case also implies: no console error, no failed request, no unreadable text or missing icon (the
UI gates in `helpers/ui-gates.js`), and an audit entry for any state change.

---

## 1. Getting in — access, identity, first run (PRIYA, NEO, LEE, PAT, EVE)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| ACC-01 | PRIYA | First-run setup | Open setup link with the installer token; fill org, name, email, password | Admin created, redirected to login; reuse of the link is refused | P1 | M |
| ACC-02 | PRIYA | Setup with wrong/missing token | Open setup with bad token | Clear refusal, no admin created | P1 | M |
| ACC-03 | PRIYA | Weak/mismatched password at setup | Enter 3-char or mismatching confirm | Field-level message, nothing created | P2 | M |
| ACC-04 | any | Sign in | Valid email + password | Lands on Dashboard, name and role shown in sidebar | P1 | A |
| ACC-05 | EVE | Sign in with wrong password | Valid email, wrong password | Generic "invalid email or password"; no hint whether the email exists | P1 | A |
| ACC-06 | EVE | Sign in with empty/odd input | Empty fields; spaces; 5 000-char email | Validation message, never a server error | P1 | A |
| ACC-07 | EVE | Repeated failures | 5+ wrong passwords for one user | Account locks with a clear message and unlock time; admin can see the event | P1 | M |
| ACC-08 | EVE | Throttle | 12 rapid sign-ins | 429 with "try again in N seconds" | P2 | P |
| ACC-09 | any | Sign out | Click Logout | Back to login; Back button cannot reveal pages; API calls now 401 | P1 | A |
| ACC-10 | any | Deep link while signed out | Open `/interfaces.html` signed out | Redirect to login, then returns to the page after sign-in | P2 | A |
| ACC-11 | any | Session expiry mid-work | Expire the session, press Save | Clear "session expired, sign in again"; unsaved form content not lost silently | P2 | M |
| ACC-12 | NEO | Invited user accepts invite | Open invite link, set name + password | Account active, can sign in; link cannot be reused | P1 | M |
| ACC-13 | NEO | Forced password reset | First sign-in with admin-set temp password | Prompted to set a new password; old one stops working | P1 | P |
| ACC-14 | any | Change my password | Sidebar → Change password; wrong current pw; then right | Wrong current pw refused; right one works; new pw required at next sign-in; old fails | P1 | A |
| ACC-15 | EVE | Reset someone else's password | Call forced-reset for another user id | Refused (known open defect — see report) | P1 | P |
| ACC-16 | LEE | Viewer experience | Sign in as viewer; visit every page | No write control is usable; disabled controls explain why; no raw 403 pages | P1 | A |
| ACC-17 | PAT | Basic-user experience | Sign in as `user`; try other pages by URL | Only HL7 Reader usable; other pages show friendly "access denied" | P1 | A |
| ACC-18 | LEE | Deep link to forbidden page | Viewer opens `user-management.html` | "Access denied" panel, not an empty shell | P2 | A |

## 2. Building and operating interfaces (MARCUS)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| IFC-01 | MARCUS | Create via wizard (TCP/MLLP → HTTP) | + Create Interface → name, source format, TCP/MLLP, port → Next → Skip sample → Next → destination URL → Finish | "Interface created" dialog; row appears in list; pipeline exists; DB row present | P1 | P |
| IFC-02 | MARCUS | Wizard validation | Leave name empty / 2 chars / duplicate name | Next blocked with a reason; duplicate name refused | P1 | A |
| IFC-03 | MARCUS | Wizard with sample HL7 | Step 2: paste ADT^A01, Parse | Segments/fields shown; Next enabled; mapping preview reflects the sample | P1 | P |
| IFC-04 | MARCUS | Wizard back/forward keeps data | Fill steps 1–3, Previous twice, Next | Entered values preserved | P2 | P |
| IFC-05 | MARCUS | Wizard cancel | Fill step 1, press × | Confirms discard; nothing created | P2 | A |
| IFC-06 | MARCUS | Wizard — every inbound connector type | Select each type in the dropdown (25) | Each shows its own fields; required ones enforced; Test Connection gives a readable result | P1 | P |
| IFC-07 | MARCUS | Wizard — every outbound connector type | Same for the target | Same | P1 | P |
| IFC-08 | MARCUS | Create from template | Dashboard → Templates → pick one → configure → Create | Interface + pipeline created with the template's steps; redirected to builder | P1 | P |
| IFC-09 | MARCUS | List: search | Type part of a name | List narrows; clearing restores; special characters safe | P1 | A |
| IFC-10 | MARCUS | List: filters | Status, type, message-type filters; combine | Counts and rows match the filters | P1 | A |
| IFC-11 | MARCUS | List: paging | Page size 10/25/50; Next/Prev/last page | Right rows, right total, page state survives refresh of filters | P1 | A |
| IFC-12 | MARCUS | Start / pause / stop processing | Use each runtime button on one interface | State label changes; DB and engine agree; messages stop/resume accordingly | P1 | A |
| IFC-13 | MARCUS | Stop Engine / start | Press Stop Engine, confirm | All interfaces stop, clear banner; start restores them | P2 | M |
| IFC-14 | MARCUS | Edit interface | More Actions → edit; change description/port; save; reopen | Saved values shown; running interface behaviour matches (restart prompt if needed) | P1 | P |
| IFC-15 | MARCUS | Edit credentials | Edit a connector with a password/key | Secret masked on screen; unchanged secret not overwritten when saving other fields | P1 | P |
| IFC-16 | MARCUS | Clone interface | More Actions → clone | Copy with new name/ids, same pipeline, inactive | P2 | P |
| IFC-17 | MARCUS | Delete / restore | Delete (soft) → confirm; find deleted; restore if offered | Gone from list, messages retained per choice, audit entry | P1 | P |
| IFC-18 | MARCUS | Delete with retention choice | Delete choosing keep_all vs delete_all | Data retained/removed exactly as chosen | P2 | M |
| IFC-19 | MARCUS | Copy interface ID | Click copy | ID on clipboard; confirmation shown | P3 | P |
| IFC-20 | MARCUS | Interface detail | Open an interface | Overview shows name, type, status, counters matching the list | P2 | A |
| IFC-21 | MARCUS | Save as template | More Actions → save as template; preview sanitisation | Secrets/hosts removed in preview; template appears in gallery | P2 | M |
| IFC-22 | MARCUS | Port conflict | Create two inbound interfaces on the same port; activate both | Second one refused with an actionable message | P1 | M |
| IFC-23 | MARCUS | Activation with a bad config | Activate with an unreachable destination | Warning shown, not a silent success | P1 | P |

## 3. Pipelines (MARCUS)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| PIP-01 | MARCUS | Open builder from an interface | Configure Pipeline | Existing steps shown on canvas in order | P1 | P |
| PIP-02 | MARCUS | Add a step | Search "validation" → double-click / drag | Step on canvas; its config panel opens with sensible defaults | P1 | P |
| PIP-03 | MARCUS | Every step type opens | Open each of the ~60 step types | Config panel renders, no console error (permanent toolbox guard exists) | P1 | P |
| PIP-04 | MARCUS | Connect / reorder steps | Connect mode, link two steps; Auto Layout | Order saved; layout readable | P1 | P |
| PIP-05 | MARCUS | Save and reload | Save Pipeline; refresh | Identical steps and settings | P1 | P |
| PIP-06 | MARCUS | Test Pipeline | Test with a sample message | Per-step input/output shown; failing step highlighted with the reason | P1 | P |
| PIP-07 | MARCUS | Fix an error found by test | Break a mapping, test, fix, retest | Error then pass | P1 | M |
| PIP-08 | MARCUS | Conditional routing | Add if/then/else or switch; route to two branches | Only the matching branch runs | P1 | P |
| PIP-09 | MARCUS | Script step | Add a script step with a custom function | Output visible in test; syntax error shown readably | P1 | P |
| PIP-10 | MARCUS | Delete a step | Remove a middle step | Neighbours reconnect or warn | P2 | P |
| PIP-11 | MARCUS | Clear canvas | Clear Canvas → confirm | Empty; Undo/cancel keeps content | P2 | P |
| PIP-12 | MARCUS | Edit while live | Change a running pipeline; save | Takes effect on next message without losing queued ones | P1 | M |
| PIP-13 | MARCUS | 50-step pipeline | Build a long pipeline | Still usable (pan/zoom), saves, runs | P3 | M |
| PIP-14 | MARCUS | Code templates in scripts | Insert a code-template function in a script step | Function available and works | P2 | M |

## 4. Messages and tracing (DANA, MARCUS)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| MSG-01 | DANA | Send a test message | Messages → pick interface → Send Message → ADT sample | Message appears with status progressing to processed; counts update | P1 | P |
| MSG-02 | DANA | Send over the real listener | Send HL7 over MLLP to the interface port | ACK AA returned; message stored; raw content equals what was sent byte for byte | P1 | A |
| MSG-03 | DANA | Find a lost message | Filter by status, patient, date range; open the match | Right message found within a few seconds | P1 | A |
| MSG-04 | DANA | Message detail | Open a message | Tabs: raw, parsed, transformed, mapping log, journey; each loads or says clearly why not | P1 | P |
| MSG-05 | DANA | Trace by ID | Message Trace → paste ID → Trace | Steps with timestamps and outcomes; unknown ID gives a friendly "not found" | P1 | A |
| MSG-06 | DANA | Failed message | Send a malformed message | Status failed/error with a readable reason, visible in errors and DLQ if retried out | P1 | A |
| MSG-07 | DANA | Reprocess | Reprocess a failed message after fixing the cause | Exactly one new successful run; original retained | P1 | P |
| MSG-08 | DANA | Bulk select | Select all on page; apply a bulk action | Confirmation lists the count; result summary shown | P2 | P |
| MSG-09 | DANA | Pagination at volume | Interface with thousands of messages | Page loads fast; paging and totals correct | P2 | P |
| MSG-10 | LEE | Viewer sees metadata only | Viewer opens a message (default policy) | Notice that content is restricted; no content anywhere (UI or API) | P1 | A |
| MSG-11 | DANA | Duplicate message | Send the same control ID twice | Behaviour matches the documented duplicate policy | P2 | M |
| MSG-12 | DANA | Large message | Send 5 MB message | Accepted or refused with clear limit message, never a crash | P2 | M |
| MSG-13 | DANA | Dead-letter queue | After repeated failures, open Dead-Letter Q | Entry with reason; filter by status/interface | P1 | P |
| MSG-14 | DANA | Redrive from DLQ | Fix cause; Redrive selected | Delivered once; leaves queue; audit entry | P1 | P |
| MSG-15 | DANA | Abandon from DLQ | Abandon selected | Marked abandoned with a reason; not redriven | P2 | P |
| MSG-16 | DANA | Review Queue | Open FHIR validation review queue; filter by severity | Items listed; badge count matches; resolving updates the badge | P2 | P |
| MSG-17 | DANA | Failed message shows why | Open a failed message | Red "This message failed — <reason>" banner on Overview, red count badge on the Errors tab, and a structured error (stage, severity, type, recovery, reason) in the tab | P1 | A |
| MSG-18 | EVE | Errors endpoint refuses injected ids | Call `/errors?interfaceId=` with SQL text / a table name | 400, no SQL executed | P1 | A |
| MSG-19 | DANA | Trace is scoped to an interface | Trace by message id and by a reused control id; without an interface; in another interface | Exact id wins; a reused id is reported as ambiguous with every match; no interface → 400; other interface → 404 | P1 | A |
| MSG-20 | DANA | Message Trace page | Pick an interface, paste an id; paste an unknown id | Timeline; the not-found message names the interface | P1 | A |
| MSG-21 | MARCUS | HL7 passthrough delivers HL7 | Passthrough MLLP → HTTP/MLLP/file | The destination receives the ORIGINAL message, byte for byte (not parsed JSON) | P1 | A |
| MSG-22 | DANA | Message page numbers are right | Open a processed interface | Success rate > 0 when messages succeeded; "Active interfaces" = active ones; no "(unknown)" label | P2 | A |
| MSG-23 | DANA | Content tab opens fast | Open a message → Content | Loads in well under 4 s | P2 | A |
| MSG-24 | DANA | Failed delivery still shows what was attempted | Open a message whose delivery failed → Content | The attempted payload is shown, labelled "Outbound (attempted, not delivered)" | P1 | A |

## 5. Watching the system (DANA, PRIYA)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| OPS-01 | DANA | Dashboard at a glance | Open Dashboard | Counts (interfaces, messages, queue/DLQ, memory, AI status) equal the DB/API truth | P1 | A |
| OPS-02 | DANA | Quick-action tiles | Click each tile | Each goes to the stated page | P2 | A |
| OPS-03 | DANA | Monitoring page | Open Monitoring; send a message | Live feed shows it; connector health and interface health sensible | P1 | A |
| OPS-04 | DANA | Engine metrics | Open Engine Metrics; Refresh Now | Throughput, latency, queue, reliability, capacity populated with plausible numbers | P2 | A |
| OPS-05 | DANA | Alert appears | Cause a failure that should alert | Alert in list and nav badge increments | P1 | M |
| OPS-06 | DANA | Alert actions | Ack one, Ack All, Resolve one; filter by interface/type | State changes, counts and badge update, audit entry | P1 | A |
| OPS-07 | DANA | Create alert rule | + New alert rule with a threshold | Saved; fires when crossed; email sent if SMTP configured | P1 | M |
| OPS-08 | DANA | Reports gallery | Run a gallery report | Opens with data, correct dates and totals; exportable | P2 | A |
| OPS-09 | DANA | Ad-hoc report | Build, run, save, reopen under My Saved Reports | Same result after reopen | P2 | P |
| OPS-10 | DANA | Data accuracy | Compare dashboard/monitoring counts with a count of messages sent | Equal | P1 | P |
| OPS-11 | PRIYA | Smoke test | Run System Smoke Test | Each check reports pass/fail with reasons | P2 | P |

## 6. Tools (MARCUS, PAT, DANA)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| TLS-01 | PAT | HL7 Reader: sample | Click ADT^A01 sample → Parse & Analyze | Segment/field breakdown with names and values | P1 | A |
| TLS-02 | PAT | HL7 Reader: own message | Paste a real message (v2.3, v2.5) | Correct breakdown; version detected | P1 | P |
| TLS-03 | PAT | HL7 Reader: invalid input | Paste garbage / empty | Friendly error, no crash | P1 | A |
| TLS-04 | PAT | HL7 Reader: validation levels | Switch validation level | Findings change accordingly | P2 | P |
| TLS-05 | PAT | HL7 Reader: Clear | Clear | Input and results reset | P3 | A |
| TLS-06 | MARCUS | FHIR Validator: valid | Patient sample → Validate | "Valid" result | P1 | A |
| TLS-07 | MARCUS | FHIR Validator: invalid | Invalid sample → Validate | Errors listed with paths and text | P1 | A |
| TLS-08 | MARCUS | FHIR Validator: profile/version | Choose US Core, other version | Result reflects the profile | P2 | P |
| TLS-09 | MARCUS | Code Templates CRUD | New Template → save → edit → delete; Flush Cache | Persisted, searchable, used by scripts; changes take effect | P1 | P |
| TLS-10 | MARCUS | Code Translations CRUD | Add translation (shared and per-interface) → edit → delete | Persisted; per-interface wins over shared; duplicates refused | P1 | P |
| TLS-11 | MARCUS | Translations CSV round trip | Export CSV, edit, Import CSV | Same data back; bad rows reported by line | P1 | A |
| TLS-12 | MARCUS | Translation applied | Pipeline using a translation processes a message | Output code is the translated one | P1 | M |
| TLS-13 | MARCUS | Git integration | Manage Repos → add; commit selected interfaces; Pull/Push | Commit appears in history; clear errors if git/remote unavailable | P2 | P |
| TLS-14 | MARCUS | Mirth migration | Drop a Mirth channel export | Channels listed with conversion result/warnings; creates interface | P2 | M |

## 7. Administration and compliance (PRIYA, SAM)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| ADM-01 | PRIYA | Create user | Create User with each role | User listed; can sign in with temp password and is forced to reset | P1 | A |
| ADM-02 | PRIYA | Invite user | Invite User by email | Invite created; email sent if SMTP set, link otherwise | P1 | M |
| ADM-03 | PRIYA | Edit role | View/Edit → change role | Takes effect on next request; UI of that user changes | P1 | P |
| ADM-04 | PRIYA | Disable/delete user | Delete a user | Cannot sign in; sessions end; audit entry; own account protected | P1 | P |
| ADM-05 | PRIYA | Find users | Search, filter by role/status, page size, select all | Results match | P2 | A |
| ADM-06 | PRIYA | Roles & Permissions tab | Open | Matrix equals what the server enforces | P2 | A |
| ADM-07 | SAM | Audit log | Audit Logs tab: filter by user/action/date/risk; search | Every earlier action present with the right actor/time | P1 | A |
| ADM-08 | SAM | Audit completeness | Perform create/edit/delete/sign-in/failed-sign-in/role change | One audit entry each, correct risk level | P1 | P |
| ADM-09 | SAM | Access policy | Settings → Access Policy: set viewer to metadata/own-interfaces; verify as that user | Policy effective immediately and everywhere (UI + API) | P1 | P |
| ADM-10 | PRIYA | SMTP settings | Enter server, save, reload, Send Test Email | Values persist; password stays masked; test mail result clear | P1 | A |
| ADM-11 | PRIYA | Other settings categories | Change retention, password policy, etc.; reload | Persist and take effect | P1 | P |
| ADM-12 | SAM | Credentials protection | Inspect UI/API/DB for an interface secret | Masked in UI/API, encrypted in DB | P1 | P |
| ADM-13 | SAM | GDPR export / erase | Export a user's audit data; erase | Export complete; erasure anonymises, keeps the record of erasure | P2 | M |
| ADM-14 | PRIYA | Mapping Review | Open from Admin menu | Page loads for an admin (known defect: shows "Access denied") | P1 | A |
| ADM-15 | PRIYA | CDA dedupe registry | Search by interface/patient; purge with reason | Rows shown; purge asks for a reason and is audited | P2 | P |
| ADM-16 | PRIYA | Backup / upgrade / rollback | Upgrade from previous release with data | Data intact; failure auto-restores | P1 | M |
| ADM-17 | PRIYA | Restart recovery | Restart the service during traffic | Interfaces auto-resume; no message lost or duplicated | P1 | P |
| ADM-18 | PRIYA | Schema packs | Install a pack; features depending on it unlock | Clear prompt before, working after | P2 | M |

## 8. Real-time and partner endpoints (PARTNER, EVE)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| PRT-01 | PARTNER | 270 eligibility check | POST valid 270 with the interface's API key | 271 returned synchronously | P1 | P |
| PRT-02 | PARTNER | 276 / 278 / pharmacy B1 | Same for each endpoint | Correct response type | P1 | P |
| PRT-03 | PARTNER | Auth variants | api_key, basic, bearer; none; wrong | Right accepted; none/wrong → 401 | P1 | A |
| PRT-04 | EVE | Lockout | 11 wrong credentials | 429 + Retry-After; audited | P1 | P |
| PRT-05 | PARTNER | Malformed / oversize payload | Bad EDI; 20 MB | Clear 4xx, no crash | P1 | P |
| PRT-06 | PARTNER | HTTP FHIR receiver | POST a Patient resource | Stored and processed; invalid one rejected with OperationOutcome | P1 | P |
| PRT-07 | PARTNER | MLLP sender | Send ADT; send non-HL7 | AA; AR for the garbage frame | P1 | P |

## 9. End-to-end business workflows (MARCUS, DANA)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| E2E-01 | MARCUS | HL7 ADT → FHIR bundle | Template → activate → send ADT → read FHIR output | Patient/Encounter resources correct; fields match the source | P1 | A |
| E2E-02 | MARCUS | HL7 ORU → FHIR Observations | Same with a lab result | Observations with values/units/refs | P1 | A |
| E2E-03 | MARCUS | EDI 835 → FHIR | 835 template + real sample | EOB per claim + PaymentReconciliation | P1 | A |
| E2E-04 | MARCUS | EDI 837P/837I → FHIR | Samples | Claim, Patient, Coverage, Organization | P1 | A |
| E2E-05 | MARCUS | EDI 270/271, 276/277, 278, 834 | Each template | Expected resources | P2 | A |
| E2E-06 | MARCUS | CDA/CCD parse → FHIR | Send a CCD | Sections mapped; coverage audit shows gaps honestly | P1 | A |
| E2E-07 | MARCUS | NCPDP SCRIPT NewRx → FHIR | Template + sample | MedicationRequest + Patient + Practitioner + Organization | P2 | A |
| E2E-08 | MARCUS | FHIR → NewRx outbound | Post MedicationRequest to FHIR receiver | NewRx XML delivered to destination | P2 | A |
| E2E-09 | MARCUS | Device ASTM / DICOM / HL7-serial | Connect-a-device wizard; send from a simulator | Message stored; clear warnings if the device cannot connect | P2 | A |
| E2E-10 | MARCUS | File / SFTP / database connectors | Drop a file / insert a row | Picked up once, processed, archived | P2 | M |
| E2E-11 | MARCUS | Outbound delivery with retry | Destination down then up | Retries per setting, then delivered once; DLQ if exhausted | P1 | A |
| E2E-12 | DANA | Day in the life | Morning check → find a failed message → fix → reprocess → confirm | Full loop without leaving the UI or asking an engineer | P1 | M |
| E2E-13 | MARCUS | Universal EDI receiver | Drop 835, 837P, 278 into one interface | Each routed to the right mapping | P2 | A |
| E2E-14 | MARCUS | ezCompanion help | Ask "how do I create an ADT interface?" | Correct, product-specific answer, not a generic one | P2 | A |

## 10. First-time and careless users (NEO, EVE)

| ID | Persona | Scenario | Steps | Expected | Pri | Auto |
|---|---|---|---|---|---|---|
| NEW-01 | NEO | First interface in 15 minutes | Only the in-app help | Done without outside help; every blocker is a documentation defect | P1 | M |
| NEW-02 | NEO | Help is findable | Use Help / Docs / quick prompts | Relevant, current, matches the screen | P2 | P |
| NEW-03 | NEO | Empty states | Visit every page on a fresh install | Each says what it is and what to do next | P2 | P |
| EVE-01 | EVE | Hostile text everywhere | `<script>`, quotes, emoji, 10 000 chars in every field | Stored/escaped safely; no script runs; layout intact | P1 | A |
| EVE-02 | EVE | Double submit / two tabs | Double-click Save; edit same record in two tabs | One record; conflict handled predictably | P2 | P |
| EVE-03 | EVE | URL tampering | Change IDs in URLs; open another user's interface | Refused or empty, never someone else's data | P1 | A |
| EVE-04 | EVE | Back/refresh mid-flow | Refresh in the wizard; browser Back after Save | Warned or recovered; no duplicate | P2 | P |

---

## Run log

Filled in by each run — one row per executed case. See `tests/uat/coverage-matrix.md` for the summary.

### Run 1 — 2026-10-27, dev stack (Docker), Chromium 1366×768, admin + created viewer/operator/user accounts

**128 cases defined · 42 automated and run · 61 planned (automatable) · 25 manual (not run).**
The 42 case IDs are covered by 41 tests: **35 pass, 6 fail on a real product defect** (D-01 to D-06 below);
D-07 comes from the page sweep (`qa-ui-sweep.spec.js`), not this spec. Every failure was triaged; several early
"failures" were my own wrong assumptions and are not counted (see "Withdrawn").

| Defect | Case | Persona | What the user sees | Severity |
|---|---|---|---|---|
| D-01 | ACC-17 | PAT | The `user` role exists only to use the HL7 Reader, but **Parse & Analyze fails**: `POST /api/hl7/parse` answers 403 because `/api/hl7` is gated to read roles that exclude `user`. The nav also offers pages they cannot use, and a 403 poll (`/api/alerts/fired`) fires on every page. | High |
| D-02 | IFC-02 | MARCUS | Creating an interface with a **name that already exists goes straight through to step 2** with no message. The API flags the duplicate (`isDuplicate` is the existing record), but the wizard only blocks on `isDuplicate === true`. | Medium |
| D-03 | TLS-01b | PAT / MARCUS | HL7 Reader segment headings print markup as text: `MSH <span class="badge-mini required" …>R</span>` (a double-escape after the HtmlSafe migration). The Reader's built-in ADT^A01 sample also fails its own validation (missing EVN). | Medium |
| D-04 | ADM-14 | PRIYA | **Mapping Review** is offered in the Admin menu, but an `admin` gets "Access denied" (its API is super-admin only). | Medium |
| D-05 | OPS-10 | DANA | Interfaces header reads **0 RUNNING / 0 STOPPED / 0 PAUSED** while the same page lists 18 Listening and 7 Stopped rows (API: 769 active, 201 inactive). | Medium |
| D-06 | ACC-09 | any | After **Logout, the Back button restores the signed-in dashboard** (name, interface count, system status, full nav) from the browser cache although the session is dead. Matters on shared clinical workstations. | Medium–High |
| D-07 | (all pages) | any | Interface action buttons (Pause/Stop/View/More) and many nav icons are **icon-only**; they disappear when the icon font cannot load, and 6 icon classes render as missing-glyph boxes even online. See `docs/QA_ESCAPES.md`. | Critical (offline) / High |

Passing highlights: sign in/out, wrong-password message, hostile input, deep-link redirect, forced password
change (old password stops working), viewer cannot use write controls or read message content, basic user
cannot open other pages or other people's interfaces, interface search/filter/paging/pause-resume, real MLLP
send (AA, stored, raw content intact) and non-HL7 frame (AR, nothing stored), messages page, message trace
(real id and unknown id both give a readable result), dashboard count equals the API, tile navigation,
monitoring live feed, engine metrics, alerts, reports, FHIR Validator valid/invalid, translations export,
user create/search/audit, SMTP settings persist and mask the password, partner endpoints refuse anonymous calls.

**Withdrawn after checking (not defects):** "Back after logout" on `interfaces.html` (reproduces only on the
dashboard — see D-06); "Message Trace gives a blank result for an unknown id" (it shows *No message found for
this correlation ID*; my first read looked at the wrong element); "dropdown empty" (options load ~1 s after the
wizard opens, then 25 are present).

### Run 2 — 2026-10-27, format × connectivity end-to-end matrix + message viewer (`tests/uat/format-matrix.js`)

Persona MARCUS/DANA: *"create an interface for each format and each connectivity, use different pipeline steps per
format, send an output, then check the message viewer, messages and logs."* Each scenario creates the interface the way
the wizard/templates do, activates it, sends a real sample through the real listener, waits for a terminal status,
checks the output **arrived** at a local receiver (HTTP, MLLP, WebSocket, or a file in the container), then probes every
message-viewer endpoint. 16 scenarios (+1 variant): **15 pass end to end; 1 (CDA) fails on delivered content.**

| Scenario | Inbound → pipeline steps → outbound | Result |
|---|---|---|
| F01 HL7 v2 ORU → FHIR | MLLP → `enrichment.validation` · `hl7_fhir_transform` · script · `hl7.build` → HTTP | Pass |
| F02 FHIR Patient passthrough | HTTP FHIR → HTTP FHIR | Pass |
| F03 EDI 835 | HTTP REST → `edi.parse` · `edi.validate` · script · `fhir.build`×2 · `payload.builder` · `fhir_validation` → HTTP | Pass (EOB + PaymentReconciliation) |
| F04 EDI 837P | same chain, `fhir.build`×4 | Pass (Claim) |
| F05 EDI 270 | same | Pass |
| **F06 / F06b CDA C-CDA** | HTTP REST → `cda.normalize` · `cda.parse` · `cda.to_fhir` → HTTP FHIR | **FAIL — delivers the original CDA XML, not the FHIR Bundle (D-11)** |
| F07 NCPDP SCRIPT NewRx | `ncpdp.parse` · `ncpdp.validate` · `fhir.build`×4 · `payload.builder` | Pass (MedicationRequest) |
| F08 NCPDP D.0 B1 claim | `ncpdptelecom.parse` · `validate` · script · `fhir.build`×3 | Pass (Claim) |
| F09 ASTM lab results | ASTM/TCP → `astm.parse` · `astm.validate` → HTTP | Pass |
| F10 FHIR MedicationRequest → NewRx XML | script · `ncpdp.map_to_canonical` · `ncpdp.validate` · `ncpdp.build` | Pass (NewRx XML delivered) |
| C01–C03 HL7 over MLLP | → HTTP / → MLLP / → file writer | Pass (see D-12) |
| C04 HTTP REST → sink · C05 WebSocket → WebSocket · C06 file listener → file writer | | Pass |

**Message viewer, 15 processed messages (every format):**

| Viewer part | Result |
|---|---|
| Overview/detail, Content (raw, parsed, transformed), **Logs** | **Accessible and populated on all 15** (logs hold 9–27 entries with stage, level and detail) |
| Journey (lineage) | **Broken for every message** — D-10, found, fixed, unit-tested and verified live (now Received → Sent to Destination) |
| Errors | **500 on all 15** — D-08 |
| Message Trace | **Finds none correctly** — D-09 |
| Mapping Log | 404 "not a CDA→FHIR conversion" on non-CDA — correct |

| Defect | Found by | What the user sees | Severity | Status |
|---|---|---|---|---|
| D-08 | MSG viewer | The Errors endpoint answers 500 `column "error_stack" does not exist`. The message tables no longer have the column that `get_message_errors()` (V23) and the Go `ErrorCaptureService` use: **2,195 of 2,199 interface tables lack it**, and the app log shows `error_stack` write failures. Failure *diagnosis* is the viewer's purpose. | High | **Fixed** (Run 3: V284/V285, Errors tab) |
| D-09 | MSG-05 | Message Trace (Operations menu) can't find a message. Only HL7-over-MLLP messages have a correlation ID (the sender's MSH-10, which is reused), so it returns *another* message from any interface (the scan covers all interface tables); EDI/NCPDP/FHIR/ASTM/CDA and HL7 over other transports have none, and always answer "no message found". | High | **Fixed** (Run 3: trace scoped to an interface) |
| D-10 | MSG-04 | Journey tab: "Message not found" for every message. `getDataLineage` shadowed the access-policy flag (`const allInterfaces` twice) → ReferenceError. Introduced by this release's access-policy change. | High | **Fixed** (`MessageController.lineage.test.js`), verified live |
| D-11 | E2E-06 | The shipped CDA → FHIR template builds a correct 7-entry Bundle, then **delivers the original CDA XML** (`application/hl7-cda+xml`, 6,204 B) while reporting "Delivery successful". Reproduced with the unmodified template, in Test Pipeline and in a real run. | High | **Fixed** (Run 3: executor fix, verified end to end) |
| D-12 | C01–C03, C06 | "HL7 passthrough" delivers a 387 KB parsed-JSON document (full schema embedded) for a 4-segment message, not the original HL7. May be intended; needs a product decision (bandwidth/storage and what "passthrough" promises). | Medium | **Fixed** (Run 4: passthrough delivers the original; V286) |

Display issues seen on the message page (observations): the interface selector shows "(unknown)", an EDI message's
type column says "Unknown", **"Success Rate 0.0%" while the single message is Delivered**, and the Journey legend lists
"Transformed" though the timeline shows two steps. Lineage cost grows with the number of interfaces (1.6–3.3 s at 974).

### Run 3 — 2026-10-28: fixes for D-08, D-09, D-10, D-11 verified (user decisions: add the error columns and expose errors; fix CDA delivery; scope Trace to the interface)

| Defect | Fix | Evidence |
|---|---|---|
| D-08 Errors | `error_stack`, `last_error_timestamp`, `last_error_stage`, `error_severity` are now part of the standard message-table schema (`InterfaceTableManager` 1.3): new tables get them, existing ones are upgraded lazily, and **V284** added them to all 2,225 existing message tables (verified: 2,225/2,225 have all four). `add_error_to_message_stack()` no longer swallows failures. **V285** keeps one failure = one error and keeps the *reason* in the summary. The viewer has an **Errors tab** (red count badge), a **"This message failed — <reason>"** banner on Overview, and a summary-only fallback card so a failure is never hidden. | X01 (refused connection) and X02 (destination 500) end `failed`; the errors API returns the structured error (stage `transformation`, severity `error`, type `TransformationError`, full reason); `error_count` = 1; screenshots reviewed. Jest: `InterfaceTableManager.errorColumns`, `MessageController.errors`. Playwright MSG-17, MSG-17b, MSG-18. |
| D-08b (found while fixing) | `getMessageErrors` built `FROM ${tableName}` from the raw `interfaceId` query parameter (**authenticated SQL injection**) and had no ownership/access-policy check. Now: UUID only, the same `(user_id = :userId OR <policy>)` rule as its siblings, quoted identifier. | Injection strings → 400 with no SQL executed (Jest + MSG-18). |
| D-08c (found while fixing) | With capture working, every failure was recorded **twice** (the engine's own status update sets `error_count = 1` and the full reason; capture then added +1 and overwrote the reason with a generic title): "2 errors" and a banner that no longer said why. Fixed in **V285** (a new migration — V284 was already applied). | Red then green: MSG-17 failed on "Pipeline execution failed" with no reason before V285; passes after. |
| D-09 Trace | `GET /api/system/message-trace/:id?interfaceId=` — interface required (UUID), matched against that interface's message id / row uuid / correlation id (exact first, then newest first), `ambiguous` + all `matches` returned; Node `requireInterfaceAccess` enforces interface visibility before the Go forward. The page has an interface typeahead, a "trace another" chooser, and a **Trace** link from the message viewer. | Go: `message_trace_resolver_test.go`; Jest: `interfaceAccess.test.js` (9); Playwright MSG-19 (required / exact / ambiguous / other interface → 404), MSG-20, and the existing trace spec updated to the scoped contract (TC-TRACE-004/005/006/008). **A race found by the existing spec:** pressing Trace before the interface list loaded said `No interface matches "<name>"` about an interface that exists; the page now waits for the list. |
| D-10 Journey | (Run 2) `allInterfaces` shadowing fixed. | Verified live: Journey shows Received → Sent to Destination. |
| D-11 CDA | `resolveAndSerializeContent`: an explicit `contentField` that resolves to a FHIR resource is delivered as FHIR JSON — the *content* decides the format, not the input message's `_format`. An explicit `contentType` still wins. | Go test red against the pre-fix executor (`ct="application/hl7-cda+xml"`, CDA body) and green after. **F06 / F06b now deliver an 11,142-byte FHIR Bundle** (was the 6,204-byte CDA XML), and the CDA viewer is 10/10 including the mapping log. |

**Regression check:** the full 18-scenario matrix was re-run after the fixes. Every previously passing scenario delivers the **same bytes** as before (F01 6,215, F02 2,543, F03 4,551, F04 5,826, F05 3,315, F07 4,736, F08 3,733, F09 1,990, F10 1,786, C01–C03 387,380, C05 4,096, C06 387,403). With the probe corrected to the ids the UI sends, every *processed* message passes all viewer checks (`mapping-log` 404 is expected for non-CDA); the failed ones also pass except `transformed` (404 "Transformed content not yet available" — for a message that failed at delivery this reads as "wait" when it means "none was produced"; wording only, recorded as an observation). Existing Playwright specs that touch the changed pages: 90 passed, 8 skipped, 1 failed (the trace race above, fixed and re-run green).

**Still open from Run 2:** D-12 (HL7 "passthrough" delivers a 387 KB parsed-JSON document labelled `application/fhir+json`; needs a product decision); the display issues on the message page ("Success Rate 0.0%", "(unknown)", "Unknown" type); the other Go-forwarded message routes (`raw`, `transformed`, `parsed-content`, `mapping-log`, `parsed-cda`) have no per-interface ownership check, so an `interfaceScope: own` access policy is not enforced on them.

### Run 4 — 2026-10-28: the five open items from Run 3 (user directions: run the DB-backed Go tests; HL7 passthrough must be HL7; explain the ownership gap; a failed message must have its content; explain the display issue)

| # | Item | Outcome |
|---|---|---|
| 1 | Go tests needing a live database | `go test ./controllers` ran against the dev Postgres: **84 tests passed, none failed, none skipped**. With the final code: `go vet` clean; `processing`, `controllers` (185 s) and the full `services/executors/transform` package (802 s, schemas mounted) all **ok**. |
| 2 (D-12) | HL7 passthrough delivered ~387 KB of parsed JSON labelled `application/fhir+json` | **Fixed.** Root cause: the wizard (`TransformationPipelineService.addConnectorStep`) hard-wired every outbound step to `contentField: fhirBundle` + `application/fhir+json` whatever the flow. A passthrough builds no bundle, so the executor delivered the whole parsed envelope. Now a data table (`FLOW_OUTBOUND_CONTENT`) gives passthrough flows `contentField: raw` (the original message). **FHIR passthrough had the same bug** (a FHIR server received `{"_format":"fhir","_semantic_index":…}`, 2,543 B for a 126-B resource) and is fixed the same way. **V286** repaired the 437 HL7 and 79 FHIR passthrough pipelines that already existed (only pipelines whose sole steps are an inbound and an outbound, still carrying the wrong default). Verified live: C01 (HTTP), C02 (MLLP), C03 (file), C06 (file→file) deliver **259 B, EXACT original** (were 387,380 B); F02 delivers **126 B EXACT original** (was 2,543 B). Also found and fixed in the same function: `normalizeWizardConfig` kept only `directory_path`/`file_pattern` for file connectors, silently discarding a writer's `filename_pattern`/`create_subdirs`/`overwrite_existing`/`encoding` and a listener's `polling_interval`/`after_processing`/`archive_path`. |
| 3 | Go message routes ignore an "own interfaces" policy | **Explained and demonstrated, not yet changed** (needs a design decision). With the policy set to "operators see only their own interfaces", an operator who owns nothing asking for another person's message gets **404 from every Node route and from the new trace, but 200 with the message contents from 4 of 4 Go content routes** (raw HL7, parsed content, transformed content, logs). Policy restored and verified identical. Risk is nil on shipped defaults (operators/viewers see all interfaces; the basic `user` role cannot reach these routes) and real wherever an administrator chooses "own interfaces only". |
| 4 | A message that failed at delivery had no transformed/outbound content ("Transformed content not yet available") | **Fixed at the cause.** The outbound step stored "the exact bytes sent" only after a successful send, so a failed delivery stored nothing. It now stores the payload **before** attempting delivery (and links it from the failed step's details). Go test red (stored 0 times) → green. Where content genuinely does not exist (the message failed before delivery), the pane now says so and points to the Errors tab, the server no longer says "not yet available", and the panel is labelled **"Outbound (attempted, not delivered)"**, not "Sent". |
| 5 | Message-page display issues | **Explained and fixed** — see below. |

**Item 5, in plain words.** The message page showed numbers and labels that were wrong or meaningless, so an operator could not trust it:
- **"Success Rate 0.0%"** while every message had been delivered — the page counted only status `sent`/`delivered` as success, but the status a successfully processed message *ends in* is `processed`. Almost every interface read 0%. Fixed in `getInterfaceTableStats` (processed/sent/delivered = success; transformed/reprocessing = still in progress). Live: 100.0% for a processed interface, 0.0% for a failed one.
- **"Active Interfaces 974"** — the number of interfaces the page happened to have loaded (including inactive, draft and errored), not the active ones. Now 772, equal to the API's active count.
- **"Name (unknown)"** in the interface selector, and **"Unknown" in the Type column** for EDI, NCPDP, CDA-over-REST and ASTM messages — only HL7 over MLLP records a type at ingest; the parser worked out the type (835, NewRx, ASTM…) but the engine never saved it. Now saved when the connector supplied none (it never overwrites a connector-supplied type): live `835`, `ASTM`, `FHIR:Patient`. A meaningless "(unknown)" suffix is no longer shown.
- **Content tab 6–9 s to open** (found while verifying): the UI asked for `/raw` and `/transformed` without the interface, so the server scanned every interface's table (2,225 here). It now passes the interface: **0.5 s**.

Evidence: Jest `InterfaceTableManager.stats` (red→green), `TransformationPipelineService.passthroughContent` (red→green, 14 tests), Go `outbound_connector_store_payload_test.go` (red→green); Playwright MSG-21…MSG-24 pass; the matrix now asserts **exact** delivered content for every passthrough scenario.

**Still open:** item 3 (the ownership gap — recommendation: enforce interface access on the Go message routes, as was done for Trace); the other connector settings the wizard may drop (`normalizeWizardConfig` keeps only `host`/`port` for TCP/MLLP, for example) were not audited; lineage cost still grows with the number of interfaces.

**Known-defect tests** are annotated `test.fail(true, reason)`: CI stays green, and the run fails loudly the
moment one of them starts passing, which is the signal to delete the annotation.

**Not run:** the 61 planned and 25 manual cases. Persona journeys for PRIYA (install/upgrade), DANA (alert
handling), SAM (GDPR), NEO (first interface in 15 minutes), and all end-to-end format workflows (E2E-01 to
E2E-14) are still open; they are where the next run should go.
