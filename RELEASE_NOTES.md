# Release Notes

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
