# ezHealthKonnect — Feature Reference for LinkedIn Content

> Purpose: a single source of truth Claude (or a human) can pull from to draft LinkedIn posts about ezHealthKonnect. Organized by feature pillar, each with what it does, why a healthcare-IT audience cares, and a proof point to cite. Recency/status notes are included so posts don't overclaim.
>
> Last compiled: 2026-07-13. Status snapshot only — verify specifics (connector counts, test counts) against the codebase before quoting exact numbers publicly, as these change fast.

---

## Elevator Pitch (use as a post opener or About-section line)

ezHealthKonnect is an integration engine for healthcare data — it ingests messages in whatever format a system speaks (HL7v2, FHIR R4, C-CDA/CCD, X12, flat files, databases, queues) and moves them wherever they need to go, transforming along the way through a visual, no-code pipeline. It's built to be the middleware layer between EHRs, payers, labs, and downstream systems, without vendor lock-in or a stack of point-to-point interfaces.

---

## Core Architecture (credibility bullets — technical audience)

- **Dual-language design**: Node.js/Express for auth, UI, and orchestration; a high-performance Go backend for the actual HL7/FHIR/CDA transformation engine.
- **Hybrid storage**: PostgreSQL for structured metadata + audit trail, MongoDB for full message content (raw + parsed JSON), so query performance and document flexibility don't trade off against each other.
- **Interface-isolated message tables**: every configured interface gets its own dedicated message table — a noisy, high-volume feed can't degrade query performance for a quiet one.
- **DAG-based pipeline execution**: transformation steps run as a directed graph (not a rigid numbered sequence) — supports branching, multi-target routing, and parallel step groups converging back together.
- **Format-agnostic core**: a shared field-resolution layer understands HL7 paths (`PID.5.1`), FHIR paths (`Patient.name[0].given`), CDA/USCDI paths, and generic JSON — one pipeline step syntax works across every message format.

---

## Feature Pillars

### 1. HL7v2 ↔ FHIR R4 Transformation Engine
What it does: parses HL7v2 messages (ADT, ORU, ORM, SIU, MDM, and more) into a structured, enhanced schema, then maps to FHIR R4 resources — and back.
Why it matters: this is the single most common integration pain point in healthcare IT — most EHRs still speak HL7v2, most modern APIs expect FHIR.
Proof point: schema-driven parsing with LOINC/CVX/SNOMED-aware field dictionaries; supports full round-trip (HL7→FHIR and FHIR→HL7).

### 2. C-CDA / CCD Document Processing
What it does: ingests C-CDA 2.1, CCD, HITSP C32, and legacy CDA documents; converts clinical sections (allergies, medications, problems, vital signs, results, immunizations, patient demographics) into FHIR resources and back, with human-readable clinical narrative rendering.
Why it matters: CDA/CCD is still the backbone of care-summary exchange (transitions of care, CMS interoperability rules) — most integration engines treat it as an afterthought.
Proof point: USCDI v3-aligned vocabulary (36 data elements) drives field search/labeling; a dedicated clinical document viewer renders narrative HTML for both CDA and FHIR bundles side by side.

### 3. Visual, No-Code Pipeline Builder
What it does: drag-and-drop pipeline builder for defining how a message gets validated, enriched, transformed, and routed — no code required for standard cases, full JavaScript escape hatch when you need custom logic.
Why it matters: integration analysts (not just engineers) can configure and maintain interfaces, cutting the dependency on scarce HL7/FHIR developer time.
Proof point: switch/case and if/then/else steps support multi-step routing to several downstream steps at once, not just single-branch logic.

### 4. Universal Connectivity — 32-Connector Catalog
What it does: one framework for inbound and outbound connectivity — TCP/MLLP, HTTP/FHIR REST, file systems, SFTP, PostgreSQL/MySQL/SQL Server/Oracle/MongoDB, Kafka/RabbitMQ/Redis, AWS S3, and more — all behind the same interface contract (`Initialize`, `Validate`, `TestConnection`, `Start`/`Send`).
Why it matters: healthcare data doesn't arrive through one door. A lab feeds a TCP socket, a payer feeds SFTP, a partner feeds a REST API — one platform needs to speak all of them.
Proof point: 26 connectors are fully implemented end-to-end (not stubs), covering every connector needed for a production HL7/FHIR MVP; the framework is a factory-pattern registry so adding a new connector doesn't touch existing code.

### 5. Configurable TCP/MLLP Acknowledgments
What it does: per-interface control over ACK/NACK behavior on MLLP listeners — immediate vs. no-ack modes, suppress-vs-NACK on error, custom sender identity, and a fully custom JavaScript function for dynamic accept/reject logic per message.
Why it matters: every trading partner has slightly different ACK expectations; hardcoded ACK behavior is a constant source of integration friction.
Proof point: 36 Go unit/integration tests + 43 Playwright E2E tests cover this feature alone.

### 6. Structured File Parsing Engine
What it does: parses CSV, TSV, fixed-width, Excel, Avro, and Parquet files into pipeline-ready records, with auto-format-detection and streaming support for large files.
Why it matters: a lot of healthcare data movement is still batch files — claims (X12/835), CMS claims line feeds (CCLF), payment files (NACHA) — and most integration engines only think in messages, not files.
Proof point: ships with out-of-box templates for CCLF1–8, NACHA entries, and 835 remittance headers; streams row-by-row for large files instead of loading the whole file into memory.

### 7. Reusable Interface Templates
What it does: a gallery of complete, pre-built interface configurations (source connector + full transformation pipeline + destination) that can be instantiated in one click, with sensitive fields (passwords, hosts, tokens) automatically stripped for sharing.
Why it matters: turns "build an ADT feed integration" from a multi-day task into picking a template and filling in connection details.
Proof point: ships with OOB templates spanning common HL7v2 message types and FHIR R4 patterns; any interface can be saved back as a new reusable template.

### 8. Security & Compliance Posture
What it does: AES-256-GCM encryption for stored credentials, PHI egress gating before outbound delivery, full audit logging (PostgreSQL + file backup) of user actions.
Why it matters: this is healthcare data — security isn't a feature, it's a prerequisite.
Proof point: every outbound connector step passes through a PHI egress check before delivery; credentials are encrypted at rest and only decrypted+masked on authorized read.
Guardrail for posts: describe this as "built with HIPAA-minded controls" (audit logging, encryption, access control) — do NOT claim "HIPAA certified" or "HIPAA compliant" as a certification, since HIPAA has no formal product certification body. Compliance is a shared responsibility of the deployment, not a badge a product earns.

### 9. Format-Agnostic Semantic Layer
What it does: builds a semantic index across whatever format a message arrives in (HL7, FHIR, CDA, JSON) so pipeline steps can reference "patient name" or "diagnosis code" once and have it resolve correctly regardless of source format.
Why it matters: removes the need to write separate transformation logic per source format — write the business rule once.

### 10. Local-First AI Companion (ezCompanion)
What it does: optional AI assistant layer running on local LLMs via Ollama — no data leaves the deployment, no PHI ever touches a third-party API.
Why it matters: healthcare orgs want AI-assisted mapping/configuration help without adding a new PHI exposure surface or a new BAA to negotiate.

### 11. Flexible Deployment
What it does: ships as a Docker Edition (single-command install) and a Standalone Edition, with a setup wizard that walks through admin account creation instead of relying on default credentials.
Why it matters: healthcare IT teams range from "we run Kubernetes" to "we have one sysadmin" — deployment shouldn't gate who can use the platform.

### 12. Test Rigor
What it does: Go unit/integration test suites alongside a ~150-test Playwright E2E suite covering auth, dashboards, interfaces, pipeline building, monitoring, messages, settings, admin, and dead-letter-queue handling.
Why it matters: signals engineering discipline to a technical buyer audience, not just feature breadth.

---

## Recent Shipped Milestones (good for "what we just shipped" posts — verify dates before publishing)

- **CDA/CCD full pipeline** (Sprints A–E, completed June 2026): raw C-CDA/CCD/C32 ingestion → FHIR conversion → clinical narrative rendering, end to end.
- **TCP/MLLP configurable ACK/NACK** with custom JavaScript override (March 2026).
- **Multi-step routing** in switch/case and if/then/else pipeline steps (January 2025/2026 cycle).
- **File Parser Executor** with healthcare-specific fixed-width templates (February 2026).
- **Interface Templates gallery** — reusable full-interface configs with automatic credential sanitization (March 2026).
- **26-of-32 connector catalog** fully implemented, covering the complete MVP connector set.

## In Design / Roadmap (frame as "coming soon," not shipped)

- DAG parallel execution model (replacing sequence-number-only execution) — architecture designed, rollout in progress.
- Remaining connector stubs: analytics warehouses (Snowflake, BigQuery, Redshift, etc.), cloud storage outbound (S3/Azure/GCS write), FTP, and Phase 4 healthcare protocols (native FHIR R4 polling connectors, EDI X12, Direct Messaging).
- Public schema registry for modular HL7/FHIR schema downloads (infrastructure built; publishing pending).

---

## Suggested LinkedIn Content Pillars

1. **"Built for the messy middle"** — posts about handling the format diversity healthcare actually has (HL7v2 + FHIR + CDA + X12 + flat files) instead of pretending everyone's on FHIR already.
2. **"No-code, not low-trust"** — the visual pipeline builder plus the JS escape hatch: accessible to analysts, powerful enough for engineers.
3. **Under-the-hood engineering** — Go performance, DAG execution, format-agnostic field resolution. Good for a technical/developer audience.
4. **Security-first healthcare middleware** — encryption, audit trails, PHI egress gating, local-first AI. Good for compliance/IT-leadership audience.
5. **Building in public** — sprint completions, test counts, connector rollout progress. Humanizes the product and shows momentum.

## Guardrails Before Publishing Any Post

- Don't state exact connector/test/migration counts without verifying against current code — these change weekly.
- Don't claim formal certifications (HIPAA, SOC 2, HITRUST) unless one has actually been obtained.
- Don't claim production customer deployments unless confirmed — current status is pre-launch/MVP per project memory.
- Frame roadmap items ("in design," "in progress") distinctly from shipped features.
