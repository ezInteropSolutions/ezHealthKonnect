# Release Notes

## v26.10 — 2026-10-20

*Previous release: [v26.08](../../releases/tag/v26.08) (2026-08-30). Migrations V229–V281.*

This release adds three new standards families — **EDI X12**, **NCPDP** (SCRIPT e-prescribing and
Telecommunication D.0 pharmacy claims) and **medical-device connectivity** (HL7 over TCP/IP or RS232,
ASTM, DICOM) — plus a security hardening pass, new healthcare transports, a compliance-grade audit
trail, and a much better-informed ezCompanion assistant.

> **Upgrade notes — read before upgrading**
> - **Partner real-time endpoints now require credentials.** Calls to `/api/eligibility`,
>   `/api/claim-status`, `/api/prior-auth` and `/api/pharmacy-claim` are refused (`401`) until an
>   engineer sets **Partner Endpoint Authentication** (API key, Basic or Bearer) on the interface.
> - **Role checks are enforced.** Users with the plain `user` role can no longer change interfaces,
>   pipelines or code templates, or use ezCompanion; give them `operator` or `viewer`.
> - **Credential encryption key.** Installers now generate `APP_CREDENTIAL_KEY` (kept on
>   re-install). Existing connector passwords/tokens are encrypted at first start. **Back up
>   `.env`** — losing this key makes stored credentials unreadable.
> - **Production refuses weak secrets.** The server will not start in production with a missing,
>   placeholder or short (< 32 chars) `SESSION_SECRET` or `JWT_SECRET`, or with any secret
>   that matches a publicly known value (see `config/compromised_secrets.json`). If you ever
>   copied secrets from this repository's `docker-compose.yml`, generate new ones before upgrading.
>   `APP_CREDENTIAL_KEY` is the exception: replacing it needs your stored credentials re-encrypted,
>   so contact support first.
> - **No secret defaults.** Secrets no longer fall back to built-in values. The processing engine
>   rejects proxied requests when no internal proxy secret (or `JWT_SECRET`) is set.
> - **Running from source:** `docker-compose.yml` no longer contains secrets. Run
>   `npm run setup:env` once to create `.env`.
> - **Device ACK default changed.** The HL7 listener now answers a frame it cannot read with `AR`
>   and a message it cannot queue with `AE` (the device resends), instead of `AA`. Existing
>   interfaces keep their stored `on_error` setting.
> - **New installer; the Docker edition is retired.** ezHealthKonnect now installs natively
>   (Windows service or Linux systemd), and the Docker install scripts and production compose
>   files are gone. Installations made with the v26.08 installers cannot be upgraded in place:
>   back up the database and `.env`, then install v26.10. From v26.10 on, run the newer
>   installer to upgrade; it backs up first and rolls back automatically if anything fails.
> - **No default administrator.** The `admin@ezhealthkonnect.com` / `admin123` account is gone.
>   Create the first administrator with the one-time setup link the installer prints.
> - **Legacy FHIR receiver URLs removed.** `/fhir/receiver/{interfaceId}` and
>   `/api/messages/fhir/{resourceType}` now return 404. They were early prototypes: one had
>   optional authentication (its Basic check accepted any password) and the other had none.
>   Neither ever ran a pipeline, and both had stopped working in this release's schema. Receive
>   FHIR with the **HTTP FHIR Receiver** connector (`http_fhir_inbound`) instead.
> - **HTTP FHIR Receiver authentication now fails closed.** An interface set to Basic, Bearer
>   or API-key authentication without the matching secret no longer activates. Previously an
>   empty secret let a request with no credentials through, and an unrecognised auth type left
>   the listener open. Interfaces using `none` are unaffected.
> - **MinIO is no longer provided.** Messages are stored on local disk by default. S3-compatible
>   storage is still supported through `OBJECT_STORAGE_DRIVER=s3`.
> - **Behind HTTPS:** set `TRUST_PROXY` and `SESSION_COOKIE_SECURE=auto` (see Capabilities Guide).
> - **First start after upgrade** re-embeds ezCompanion's knowledge in the background (about
>   30–60 minutes of CPU on a CPU-only server); the app is fully usable meanwhile.

---

### EDI X12 005010 (new)

- Schema-driven parse / validate / build engine (`edi/`) — a new transaction set is schema data,
  not new code. Delimiters are read from each file's own ISA header.
- Transaction sets: **835, 837P, 837I, 999, 270, 271, 276, 277, 278, 834**.
- X12 → FHIR R4 templates for each (Claim, ExplanationOfBenefit, PaymentReconciliation, Coverage,
  Patient, Organization, Task, ClaimResponse), plus a **Universal Receiver** template that accepts
  any of them on one interface and routes each file by its own transaction set.
- `edi.map_to_canonical` builds outbound X12 from any source shape; `edi.generate_999` produces a
  999 acknowledgment from validation results; trading-partner custom validation rules.
- Synchronous request/response endpoints for eligibility (270→271), claim status (276→277) and
  prior authorization (278).
- Verified against real X12.org worked examples and third-party samples; several real parsing-
  engine and mapping bugs found that way were fixed (sibling-loop discrimination, RD8 date ranges,
  ICD-9/ICD-10 qualifiers, maintenance-type codes).

### NCPDP (new)

- **SCRIPT 2017071** (e-prescribing XML): NewRx, CancelRx/Response, RxChangeRequest/Response,
  RxRenewalRequest/Response, RxFill, Status, Error, Verify, GetMessage — with SCRIPT → FHIR templates
  (MedicationRequest, MedicationDispense, Task). Verified against 33 real third-party fixtures.
- **Telecommunication D.0** (pharmacy claims): B1, B2, B3, E1 request/response, including signed-
  overpunch decimals; B1 → FHIR Claim/ClaimResponse templates and a synchronous
  `/api/pharmacy-claim/{interfaceId}/submit` endpoint.
- **Outbound**: FHIR MedicationRequest → NewRx and FHIR MedicationDispense → B1 claim templates; the
  no-code mapping works from HL7, CSV or database sources too.

### Medical device connectivity (new)

- **Connect a Device** wizard with templates named by how the device talks: HL7 over TCP/IP, **HL7
  over Serial Port (RS232)**, ASTM over TCP/IP, ASTM over Serial Port, DICOM Storage SCP.
- ASTM E1394-97 engine (parse / validate / build) and transports; DICOM Storage SCP with TLS,
  mutual TLS and a per-instance size cap. A go-dicom protocol bug that broke C-STORE from
  non-go-dicom senders was found (using dcm4che as an independent device stand-in) and fixed.
- HL7 over RS232 uses the same MLLP framing and HL7 ACK as the network listener; the serial
  connector's new **Protocol** setting chooses ASTM (default) or HL7.
- The wizard shows the real reason when a listener cannot start (e.g. port already in use) instead
  of waiting for a device that can never connect.

### Transports

- **AS2** inbound/outbound (signed + encrypted HTTP with synchronous MDN).
- **Direct Messaging** (DirectTrust S/MIME over IMAP/SMTP).
- **WebSocket** inbound/outbound; outbound connector responses (HTTP body, MLLP ACK code, WebSocket
  reply) are available to later pipeline steps.

### Security

- Every API route that reaches the processing engine requires a logged-in user **and** an
  appropriate role (reads: viewer and above; changes: operator and above), enforced in both the
  Node and Go layers. Previously several route groups were reachable without logging in.
- Per-interface credentials for partner endpoints, with brute-force lockout (10 failures / 15 min
  per caller address → 15-minute block) and audit of failed attempts.
- Connector credentials in interface settings are encrypted at rest (AES-256-GCM) and masked on
  every read and in logs.
- Content-Security-Policy, real client IP in audit records, HTTPS-aware session cookies, and a
  production secrets guard.
- No committed secrets and no secret defaults: production refuses publicly known secret values
  (checked in both the web server and the processing engine), and recurrence is blocked by a
  gitleaks rule, a repository policy test, a pre-commit hook and per-run CI secrets.
- Go 1.26.10 and Node.js 22.23.4 (the installer's bundled runtime; both are security releases)
  plus updated dependencies: govulncheck reports 0 reachable vulnerabilities; `npm audit` reports
  0 production vulnerabilities.
- The browser UI escapes by default. Every page now builds HTML through one shared helper, which
  fixes stored and reflected cross-site scripting found in interface, step and user names, alert
  rules, templates, reports, the dead-letter queue, clinical documents and message content. A CI
  lint gate blocks new unescaped HTML. Unused frontend files (old backups, unreferenced scripts
  and developer test pages) are no longer shipped.

### Audit logging and compliance

- Unified audit taxonomy shared by the Go and Node services; coverage for interface, message,
  pipeline, credential, role and password events.
- **ATNA export**: RFC 5424 syslog with DICOM AuditMessage XML, configurable live in Settings.
- **GDPR**: subject data export (Art. 15) and erasure by anonymization (Art. 17).
- Query indexes for the audit log viewer; graceful shutdown fixes so audit records are flushed
  on container stop.

### Coverage Audit

- Extended from C-CDA to **HL7 v2, FHIR, EDI X12 and NCPDP**, at element level (e.g. `PID.5.1`),
  including live tracking for HL7 → FHIR transforms.

### ezCompanion

- Learns this release's capabilities: documentation for every new pipeline step, the EDI / NCPDP /
  D.0 / ASTM schemas, a condensed HL7 v2.5.1 and FHIR R4 (plus US Core and Da Vinci PAS) summary,
  and a new question-oriented **Capabilities Guide**.
- Knowledge refreshes automatically on upgrade (only changed sources are re-embedded).
- Fixed: it had no knowledge of configured interfaces (every interface failed to load), and
  category-filtered searches returned nothing; documentation now always gets a share of each answer.

### Installation and upgrades (new)

- One installer per OS, `ezHealthKonnect-Setup-Win64.exe` and `ezHealthKonnect-Setup-Linux-x64`.
  Each includes the whole application and its own Node.js runtime. Use the setup window or the
  headless commands `install --config answers.json`, `upgrade`, `rollback --yes`,
  `uninstall --yes`, `status`. See [docs/INSTALL_GUIDE.md](docs/INSTALL_GUIDE.md).
- New installs get PostgreSQL 17. Existing PostgreSQL 15+ servers, including managed cloud
  databases, are supported.
- **Safe upgrades.** Before changing anything, an upgrade checks the database, the backup tools
  and the free disk space. It then backs up the database and the application, and installs and
  migrates the new version. If the health check fails it restores both automatically. Customer
  data folders and `.env` are never touched. Downgrades are refused, and `rollback` returns to
  the previous version.
- Database migrations run through the application's built-in migration engine (`go-api
  migrate`), which replaces Flyway. It is fully compatible with the existing
  `flyway_schema_history`.
- Services run with least privilege. On Linux: a dedicated user with hardened systemd units. On
  Windows: a dedicated virtual service account instead of LocalSystem, which can read the
  application and write only its data folders.
- Every release's installers are tested end to end before publication on Ubuntu 22.04, Ubuntu
  24.04, AlmaLinux 9 and Windows. The tests cover install, an upgrade, a forced upgrade failure
  that must roll back, a rollback and uninstall.

### Fixes worth noting

- Interfaces created via "Use Template" could be left without their message table.
- Re-running the wizard on an existing interface called a method that did not exist.
- Template gallery tabs dropped the EDI, NCPDP and CDA categories.
- Several pipeline-engine fixes found through real interop testing (field validation was inert,
  untaken switch/if branches still ran, single-object repeating segments were dropped on build).

### Known limitations

- HL7 over serial requires MLLP framing; the bidirectional order-query mode is TCP-only.
- NCPDP D.0: E2, D0/D1, N/C/S transactions and the batch standard are not supported; a B1 built
  from a FHIR MedicationDispense carries no pricing (FHIR does not model point-of-sale prices).
- DICOM: Modality Worklist, MPPS and Storage Commitment are not supported.
- The Windows installer is not code-signed yet (SmartScreen warns on first run). Linux installers
  are x86_64 only.
- ezCompanion runs on a small local model by default; it can misread detailed schema lists, and
  some answers still favour saved configurations over the reference documentation.

---

## v26.08 — 2026-08-30

*Previous release: [v26.06.08](../../releases/tag/v26.06.08) (2026-06-08). 30 commits, ~4,700 files changed.*

This release is dominated by a large new C-CDA/CCD processing subsystem (built from scratch this
window), a full USCDI v3 vocabulary bridge, seven new connector implementations, and a round of
CVE remediation. Details below by area.

---

### C-CDA / CCD Processing (new subsystem)

The Go backend gained a complete, schema-driven CDA parse/build/validate engine — none of this
existed at v26.06.08.

- **Parsing & building**: inbound C-CDA parsing (`cda/document`) and outbound document generation
  (`cda/builder`) for all **12 official HL7 C-CDA R2.1 document types** — CCD, Discharge Summary,
  Referral Note, History and Physical, Consultation Note, Progress Note, Care Plan, Diagnostic
  Imaging Report, Operative Note, Procedure Note, Transfer Summary, Unstructured Document. Both
  are generic, schema-driven engines (section/entry archetypes, not one hand-written function per
  section) — new sections are added as JSON data, not new Go code.
- **HL7→FHIR mapping**: `services/cda_fhir` maps parsed CDA sections to FHIR resources; declarative
  OOB mapping rules (V189 CareTeam.managingOrganization, Coverage-on-FHIR-IG alignment,
  interpretationCode handling, and others) with drift-guard tests against the seeded rules.
- **Coverage Audit**: element-level tracking of what was/wasn't mapped, including document-header
  fields (patient/author/custodian/encompassingEncounter) — previously out of scope, now shipped.
  Four tracking bugs found and fixed along the way (entry-root wrapper mis-indexing, organizer
  flattening collisions, entry-level-only percentages, header multiSegment indexing).
- **Cross-document dedupe** (`cda.dedupe`, `crossMessage` mode): turns a cumulative CCD feed
  (every document restates the full problem/med/allergy list) into an effective incremental one —
  each clinical fact is delivered downstream once per patient per interface. Backed by
  `cda_dedupe_registry` (atomic upsert, no extra round trip), with per-message suppression lineage
  on the Journey tab, an admin registry viewer with audit logging (`CDA_DEDUPE_REGISTRY_VIEWED`/
  `_PURGED`), and a 7-year default retention job. Explicitly *not* a patient-summary/CDR builder —
  documented as an intentional scope boundary.
- **CSV export** (`cda.section_to_csv`): grew from 9 to 35 supported sections, with a source-file
  column, multi-value column support, and a no-code step-config UI.
- **Schema restructuring**: the monolithic 4,118-line `ccda_2_1.json` was split into a
  `manifest.json` + 69 per-section files + 5 reusable entry-archetype templates
  (`cda/schemas/ccda_2_1/`), removing duplicated structural definitions across sections while
  producing a byte-for-byte-verified (golden snapshot) identical runtime schema.
- **Spec-conformance corrections**: independently re-audited SHALL/SHOULD/MAY section lists for
  CCD, Discharge Summary, Referral Note, Consultation Note, History and Physical, and Progress
  Note against the C-CDA IG and Companion Guide — found and fixed real defects in all six
  (Progress Note's SHALL list, in particular, was entirely wrong). Added a new "choice constraint"
  schema construct (e.g. "SHALL contain Assessment and Plan, OR both Assessment and Plan of
  Treatment") that a flat SHALL/SHOULD/MAY list can't express.
- **External validator**: 25 rounds of validation against a real external CDA validator; structural
  errors reduced to zero early on, warnings reduced from 85 → 41 → 38 → 31 over the course of the
  work.
- **Known, documented gap**: DICOM Object Catalog Section (Diagnostic Imaging Report) is not
  implemented — its 3-level polymorphic nesting and title-less/positional constraints don't fit
  the current generic engine. Decided (user-confirmed) as a permanent, spec-acknowledged
  limitation rather than a one-off engine extension.

### USCDI v3 Vocabulary Bridge

- Curated, ONC-verified USCDI v3 dataset (`cda/schemas/uscdi_v3.json`) covering all 19 classes /
  92 elements, wired into both **Coverage Audit** (which USCDI classes a document actually
  represents) and the **Requirements catalog** (which classes a SHALL/SHOULD/MAY section maps to),
  via a shared `USCDIVocabulary.ClassesForSection()` used by both consumers.
- Closed the last two vocabulary gaps this round: **Medication Fill Status** (new
  `medicationDispenses` repeating group) and **UDI** — Tier 1 (Product Instance's own device
  identifier) and Tier 2 (the full 12-sub-observation UDI Organizer). Tier 2 required a genuinely
  new schema construct (`ComponentGroup`/`ComponentDef`) since neither existing repeating-group nor
  fixed-anchor mechanism could express "a fixed set of independently-optional, differently-shaped
  sub-observations." 87 of 92 USCDI v3 elements are now mapped.
- Found and fixed a real bug where a vocabulary display label (`USCDIElement`) was being used
  verbatim as literal narrative table header text in generated documents (4 fields affected,
  corrected).

### HL7 v2

- **Guarded, schema-driven builder** for `hl7.build`: schema-based field/segment pickers and
  ordering, repeat-gating, Z-segment support, conditional segment/field population, and
  nested/grouped segments addressed by path (not a flat list).
- **HL7 v2 conformance validator** (`hl7/validator`): segment/cardinality/datatype/table checks
  with 3 user-selectable strictness levels, wired into the HL7 reader UI.
- **Repeat-instancing fix**: repeating segments (NK1, GT1, AL1, DG1, PR1, ROL, RXA, FT1, …) were
  silently collapsing to a single occurrence across 495 of 700 OOB HL7→FHIR templates. Fixed via a
  spec-driven per-segment table in the template *generator* rather than editing templates directly
  — the generator self-regenerates every boot, so DB-level fixes alone would have been silently
  reverted. Also added configurable segment-merge priority and field-collision warnings.

### FHIR

- **Narrative consolidation**: four separate hand-written per-resource-type narrative generators
  merged into one generic engine (`services/fhir_narrative`), plus a new per-interface
  `narrative_fields` config. Found two more real bugs (criticality/reaction shape mismatches)
  only by testing against real generated output.
- New FHIR narrative generator added for the `Location` resource.
- Continued in-house `fhir/r4` validator strategy (HAPI FHIR integration evaluated and declined).

### Connectivity — 7 stub-to-real connector conversions this window

Registered connector types grew to 53 (17 of 26 inbound, 16 of 27 outbound now real, up from 26
real at the start of this window):

- **Snowflake** (inbound + outbound) — official `gosnowflake/v2` driver, username/password auth
  (key-pair/JWT explicitly rejected rather than silently ignored; unverified against a live
  warehouse — no test credentials available).
- **Databricks** (inbound + outbound) — official `databricks-sql-go` driver, PAT auth only.
- **Azure Blob** (inbound + outbound).
- **HTTP/REST Inbound** — split out as a genuinely separate, non-FHIR-validating generic HTTP
  receiver (previously aliased to the FHIR connector); a leftover duplicate stub from that split
  was found and removed.
- **MySQL outbound, AWS S3 outbound, Kafka outbound.**
- **SFTP** (both directions) rewritten on the real SFTP protocol (`github.com/pkg/sftp`) — the
  prior implementation used SSH shell-exec, which fails against properly locked-down SFTP-only
  servers.

Also this window:
- **TCP/MLLP ACK/NACK configuration**: per-listener ACK mode (immediate/none), on-error behavior
  (suppress/NACK), sender-identity and message-text overrides, and a custom `buildACK(msg)` goja
  JavaScript hook for fully dynamic ACK logic, with a dedicated UI tab and 79 new tests
  (36 Go + 43 Playwright).
- **Inbound schema completeness audit**: every real inbound connector's config schema checked
  against its actual `Initialize()` code; found and fixed a second bug class (correct field names
  but missing fields) in tcp_mllp, kafka, redis, and mongodb inbound connectors.
- Remaining stubs (20 registered types, unchanged): analytics warehouses (BigQuery, Redshift,
  Synapse, ClickHouse, TimescaleDB), RabbitMQ/Redis outbound, GCS, FTP, and Phase 4 healthcare
  protocols (EDI X12, Direct Messaging).

### Pipeline & Interface Configuration

- **File Parser executor**: parses CSV, TSV, fixed-width, Excel, Avro, and Parquet into pipeline
  records, with streaming CSV, magic-byte auto-detection, OOB healthcare templates (CCLF1–8,
  NACHA, 835), and encrypted S3 credential resolution.
- **Multi-step routing**: Switch/Case and If/Then/Else steps can now route to *multiple* target
  steps per branch (`targetStepIds[]`), not just one, with backward-compatible auto-migration from
  the old single-`targetStepId` config.
- **Format-agnostic field utilities**: a single shared strategy-pattern API
  (`GetFieldValue`/`UpdateFieldValue`) for reading/writing fields across HL7, FHIR, and generic
  JSON paths, replacing per-format one-off getters in individual executors.
- **Interface Templates system** (Phases 1–3 complete): OOB template gallery, "Save as Template"
  and connector-agnostic "Use Template" flows, 43 connectivity types with UI categorization.
- **Da Vinci PAS template reliability** (V210–V212): 7 bugs fixed (validation config keys,
  source-field resolution, decision routing/delivery, a retry gap); the payload-assembly zone was
  rebuilt on the generic `fhir.build`/`payload.builder` steps instead of 4 hand-written scripts,
  fixing a fullUrl/reference-form bug structurally rather than patching each script.
- **Interface editor consolidation**: the separate Settings tab on the interface detail page was
  folded into the Edit modal.

---

### Security

- **CVE-2026-9496 (pacote)** closed fully — a first pass removed npm's top-level bundled `pacote`,
  but Trivy still flagged a second, independently-versioned copy nested inside
  `@npmcli/metavuln-calculator`; both are now removed and verified with a clean local Trivy scan
  (0 CRITICAL/HIGH).
- Dependency bumps for four more CVEs surfaced by `govulncheck`/Trivy against the new
  Snowflake/Databricks connector dependency tree: **go-jose/v3** (GO-2026-4945, reachable via the
  Databricks connector init chain), **go-retryablehttp** (GO-2024-2947, reachable via
  `SettingsController.TestAIConnection`), **otel**, and **thrift**.
- Gitleaks configuration and telemetry-script security/rate-limiting hardened.

### Breaking / Migration Note — schema binaries no longer tracked in git

`schemas/fhir/R4/**` and `schemas/hl7/**` (~3,900 files) have been untracked from the main repo.
They've been installable via **Settings → Schema Packages** for months; this change only stops
git from tracking the already-redundant committed copies. Distributed instead via the public
[ezhealthkonnect-schemas](https://github.com/ezinteropsolutions/ezhealthkonnect-schemas) releases
repo (currently v1.1.0, which also fixes a `.gz` corruption bug present in every v1.0.0 asset).
Docker was never affected (schemas/ was already excluded from the build context). If you have a
local checkout with a pre-existing working tree, your on-disk schema files are untouched — only
git's tracking of them changed.

### CI / Developer Experience

- CI workflow now installs schema packages as an explicit step, with two follow-on fixes for
  real failures hit on GitHub's non-root runners: `unzip`'s benign warning exit code on
  Windows-built zips, and broken Unix permission bits on the zips' directory entries.
- Dockerfile and docker-compose reworked for cross-platform compatibility (including a
  CI-specific compose file removed in favor of one shared config).
- Playwright global setup gained a first-run admin-authentication bootstrap; several specs updated
  to skip cleanly when no MLLP listener is configured rather than failing.
- Substantial new unit/integration test coverage added this window: PAS FHIR Builder, USCDI
  vocabulary integrity, V209 migration drift guards, FHIR bundle assembly and narrative generation
  (including a `bundle-1` "no total on collection/transaction bundle" constraint test), timezone
  offset parsing, RateLimiter middleware, HL7MessageAnalyzer, InterfaceTableManager,
  WizardMappingService, interface-template sanitization, `map_to_canonical` executor, and
  end-to-end HL7/FHIR and CDA-guided-configuration pipelines.

### Known Issues

- Main's CI had been red since 2026-07-13 on `govulncheck`/Trivy dependency-CVE gates; this
  release's final two commits are direct remediation for those findings. As of this tag, the
  latest CI run for this commit was still in progress — verify current pipeline status before
  relying on this note for CI health.
- 20 connector types remain stub implementations (see Connectivity section above).
- DICOM Object Catalog Section (Diagnostic Imaging Report) remains permanently out of scope under
  the current CDA engine architecture (see C-CDA section above).
