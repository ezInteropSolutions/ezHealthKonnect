# QA Coverage Matrix — v26.10 QA pass (2026-10-27)

Environment: Docker dev stack on Windows 11 (app rebuilt from the working tree, Postgres 17, Chromium only).
**Not** the installer artifact on a clean machine — see "Not tested".

| Area | What was run | Result |
|---|---|---|
| UI page sweep, online | 29 pages × 360/768/1366/1920 (116 views): console/request/off-origin/icon-glyph/overflow gates | 88 views with broken icons (6 icon classes), 2 views with horizontal overflow, 12 with HTTP errors |
| UI page sweep, offline (cdnjs blocked) | same, outbound traffic aborted | 96 of 116 views lose every icon (all signed-in pages) |
| Anonymous API probe | 208 routes, no cookie | no protected data reachable; 6 unauthenticated info leaks (see report) |
| Role matrix | 71 routes × 5 identities (anon, viewer, user, operator, admin); writes only with empty/nonexistent ids | 0 confirmed privilege escalations; 1 missing role gate (`/interface-templates/:id/use`); **found a server-crash defect (fixed)** |
| API performance | 12 endpoints × 40 sequential + 40 concurrent(10) | 2 endpoints out of budget (`/api/interfaces`, `/api/ai/status`) |
| Oversize payload | 12 MB JSON body | 500 instead of 413; server survives |
| DB structural checks | PKs, unindexed FKs, seq scans | 1 table without PK, ~15 unindexed FKs (low) |
| UAT Marcus (engineer) | wizard: create TCP/MLLP→HTTP interface, Skip sample, Finish | works; interface + pipeline persisted; layout defects noted |
| Fixes with regression tests | WizardMappingController crash; login 500 | Jest 443/443 passing; both verified live |

## UAT functional run 1 (persona workflows) — see docs/UAT_TEST_CASES.md
128 cases defined (9 personas, 10 workflow areas). 42 case IDs automated and run as 41 tests: **35 pass, 6 expose real defects** (D-01…D-06), all kept as `test.fail` known-defect tests. 61 cases planned, 25 manual — not yet run.

## Format × connectivity end-to-end matrix + message viewer (`tests/uat/format-matrix.js`)
16 scenarios, each: create → activate → real sample through the real listener → output arrived? → message viewer probed.
**Formats (10):** HL7 v2 ORU, FHIR passthrough, EDI 835 / 837P / 270, CDA, NCPDP SCRIPT NewRx, NCPDP D.0 B1, ASTM, FHIR→NewRx. **Inbound connectors driven (7):** TCP/MLLP, HTTP REST, HTTP FHIR, WebSocket, file listener, ASTM/TCP (+ SFTP/EDI-SFTP replaced by HTTP REST). **Outbound verified (6):** HTTP, HTTP FHIR, TCP/MLLP, WebSocket, file writer, sink.
**Result (after the Run 3 fixes, 2026-10-28): all 16 scenarios + the CDA variant and two failure scenarios (X01 refused connection, X02 destination 500) behave correctly.** CDA now delivers its FHIR Bundle (D-11); every processed message passes all message-viewer checks (Journey D-10, Errors D-08, Trace D-09 fixed; `mapping-log` 404 is expected for non-CDA); failed messages end `failed` with a structured error visible in the Errors tab. Before the fixes: 15 of 16 delivered correctly and the viewer's Journey, Errors and Trace did not work.
**Not run — connector types needing an external service not present in this environment:** SFTP/FTP, Kafka, RabbitMQ, Redis, MongoDB, MySQL, SQL Server, Oracle, Azure Blob, GCS, Databricks, Snowflake, BigQuery & other warehouses, AS2, Direct Messaging (SMTP/IMAP), DICOM (needs an SCU), serial/RS232; PostgreSQL and S3 (MinIO) *could* be driven here and were not built yet. Not run: EDI 837I, 271, 276/277, 278, 834, universal EDI receiver, NCPDP lifecycle messages.

## Run 4 (2026-10-28) — final state
- Go: `go vet` clean; `processing`, `controllers` (against the live Postgres, 84 tests) and the full `services/executors/transform` package all **ok**.
- Matrix: every passthrough scenario now asserts the delivered body is **exactly** the original (HL7 259 B, FHIR 126 B); failed deliveries store and show the attempted payload; the stored message type is populated for EDI/NCPDP/ASTM/FHIR; viewer = all parts 200 for processed messages (`mapping-log` 404 expected for non-CDA).
- Authorization with a **restrictive** policy (operators "own interfaces only"): Node routes and the scoped trace refuse; **4 Go content routes do not** (open, see docs/QA_ESCAPES.md).
- Not audited: other wizard connector types' dropped settings (TCP/MLLP keeps only host/port); Go content routes' ownership.

## Not tested (and why)
- **Installer artifact on a clean VM, upgrade from previous release, uninstall** — no clean machine available in this session. Highest remaining risk.
- Firefox, WebKit, Edge — only Chromium installed locally (and CI runs only Chromium).
- Mobile/tablet *behaviour* beyond layout screenshots at 360/768.
- Accessibility (axe) — dependency not installed; only title/h1 checks run.
- DB persistence round-trips across restart, retention jobs, migration upgrade path, data tiers above the current dev DB (2,580 interfaces, 331k audit rows), `EXPLAIN ANALYZE` of list queries.
- Soak, memory/goroutine growth, backpressure/downstream-failure behaviour.
- Personas Priya, Dana, Sam, Lee, Pat, trading partner, new hire, hostile user (only Marcus's wizard journey run); every other wizard flow and OOB template.
- Pen-test style injection (SQL/SSRF/XXE/path traversal) beyond input-shape probes.
- Real-device and real-trading-partner interoperability.
