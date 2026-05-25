# HL7→FHIR Production Readiness Plan

**Status**: Planning
**Owner**: Integration Team
**Last updated**: 2026-05-23

---

## Current State Assessment

The transformation engine has been scale-tested end-to-end:

- **41 messages** across **14 HL7 message type variants** sent via TCP/MLLP
- **100% delivery** (all messages ACK'd and FHIR bundles retrieved from MinIO)
- **100% structural pass** at standard validation level

What this proves: the happy path works. What it does not prove: semantic
correctness, US Core profile compliance, vendor-specific EHR quirks, or
behaviour under malformed/edge-case input.

**Confidence in production readiness: ~60%.**

---

## Internal Mapping Improvement Loop

This is a closed internal process. Users are not involved. The integration
team owns it end to end.

```
Production traffic
      │
      ▼
Transformation engine scores every output
(resource count, unresolved fields, Z-segments dropped, validation errors)
      │
      ├─ Score OK ──► archived, sampled periodically for manual review
      │
      └─ Score below threshold ──► flagged into mapping_review_queue
                                          │
                                          ▼
                            Integration team reviews
                            (HL7 input + FHIR output side-by-side)
                                          │
                            ┌─────────────┴─────────────┐
                            │                           │
                      OOB template gap          Vendor-specific quirk
                            │                           │
                    Update hl7_fhir_templates   Add interface override
                            │                           │
                            └─────────────┬─────────────┘
                                          │
                                  Add to regression suite
                                          │
                                   Next version release
```

### Scoring heuristics (automated, no user action)

| Signal | Flag condition |
|--------|---------------|
| Expected resource missing | Any required resource absent for that message type |
| Empty required FHIR field | `Patient.identifier`, `Observation.code`, `Encounter.status` null |
| Z-segment dropped | Any Z-segment in input not handled |
| Unrecognised code system | OBX code not in LOINC/SNOMED/local table |
| Resource count outlier | Count > 2 std deviations from interface baseline |

### Review cadence

- **Continuous**: automated flags reviewed within 48 hours
- **Weekly**: random sample of 20 passing transformations reviewed manually
- **Per new customer**: full review of first 100 messages from their EHR before go-live
- **Per new EHR vendor**: dedicated review session; findings catalogued in the vendor quirks registry

### Vendor quirks registry

A growing internal document (`architecture/VENDOR_HL7_QUIRKS.md`) that
catalogues confirmed non-standard field usage per EHR vendor. Starts empty,
populated from production review. Examples of what gets captured:

- Epic sends PID.3 with multiple identifiers in different repetitions — which repetition is the MRN?
- Cerner populates OBX.6 units as free text rather than UCUM codes
- Meditech omits EVN segment on ADT^A08

Each entry links to the template fix that addresses it.

### Regression suite

Every mapping bug found in production automatically becomes a regression test
case. The test harness (`tests/hl7-scale-test.js`) is extended with the exact
HL7 that exposed the bug and the expected FHIR output. CI runs it on every
build. No mapping regression can ship undetected.

---

## Production Readiness Workstreams

### 1. Semantic correctness — HARD GATE (4–5 weeks)

Nothing ships to a customer until this is done.

- **Field-level validation**: extend the test harness to assert actual field
  values, not just resource presence. `Patient.identifier[0].value` must equal
  `PID.3`. `Observation.valueQuantity.unit` must be a valid UCUM code.
- **US Core profile compliance**: run the HL7 FHIR validator against every
  bundle in CI. US Core is the baseline for any Epic/Cerner integration.
- **Real-world HL7 corpus**: obtain de-identified samples from the first 2–3
  pilot customers. Run them through and catalogue failures. This alone will
  surface more gaps than months of synthetic testing.
- **Z-segment handling**: unknown Z-segments currently dropped silently. They
  must be flagged and logged so the team knows what is being lost.
- **Mapping correction workflow**: the internal review queue
  (`/admin/mapping-review`) that drives the improvement loop above. Build this
  during workstream 1 because that is when the most template fixing happens.

**Definition of done**: 100% pass on US Core validator for all 14 tested
message types; real-world corpus from at least one pilot customer at 95%+ pass.

---

### 2. Delivery guarantees (3–4 weeks)

Healthcare integrations require at-least-once delivery. A message dropped
because the downstream FHIR receiver was temporarily unavailable is not
acceptable.

- **Dead letter queue**: failed deliveries written to `delivery_dlq` table with
  full payload. Visible in admin UI. Ops can retry or re-route.
- **Configurable retry**: `maxRetries`, `initialDelayMs`, `backoffMultiplier`
  per interface. Already in the error handling design; needs to be wired to the
  outbound connector.
- **Ordered delivery option** *(deferred — Phase 2)*: ADT event sequences
  (A01 → A08 → A03) for the same patient must arrive in order when the
  downstream EHR is order-sensitive. Deferred because most HL7 senders already
  sequence at their end and the first pilot customers can be validated manually.

  **Chosen design (implement when needed):**
  ```
  Table: delivery_sequence
    id              UUID PK
    interface_id    UUID NOT NULL
    patient_id      TEXT NOT NULL        -- PID.3 value
    sequence_number BIGINT NOT NULL      -- monotonic counter per (interface, patient)
    message_id      TEXT NOT NULL
    hold_until      TIMESTAMPTZ          -- NULL = can deliver immediately
    delivered_at    TIMESTAMPTZ
    created_at      TIMESTAMPTZ DEFAULT NOW()

  UNIQUE (interface_id, patient_id, sequence_number)
  INDEX  (interface_id, patient_id, sequence_number) WHERE delivered_at IS NULL
  ```
  Per-interface flag `enforce_ordering: bool` on the `interfaces` table.
  The outbound connector executor checks: "is there an undelivered lower
  sequence number for this patient on this interface?" If yes, hold and
  re-queue. The DLQ poller also respects ordering before redrive.
- **HTTP delivery receipt**: outbound connector must confirm HTTP 2xx from the
  downstream system before marking a message delivered. TCP ACK is not enough.

---

### 3. Observability (3 weeks, parallel with #2)

- **Per-interface metrics**: transformation success rate, p50/p95/p99 latency,
  error rate, DLQ depth — stored in PostgreSQL, surfaced in a dashboard page.
- **Alerting**: configurable thresholds per interface. Alert via email or
  webhook when error rate or DLQ depth exceeds threshold.
- **HIPAA audit trail**: every transformation logs: correlation ID, input hash,
  output hash, pipeline steps executed, disposition, timestamp, user or system
  that triggered it. Tamper-evident. `audit_logs` table is there; needs
  consistent wiring through the transformation pipeline.
- **Correlation ID threading**: every message gets a correlation ID at inbound
  that appears in every log line through all pipeline steps. Already in the
  schema; needs consistent use in logging calls.

---

### 4. Security hardening (3–4 weeks, parallel with #3)

- **TLS default on MLLP listeners**: TLS support exists but is opt-in. Flip the
  default. Self-signed certs are not acceptable; add a cert provisioning step
  to the interface setup wizard.
- **PHI encryption at rest**: MinIO bucket server-side encryption; PostgreSQL
  `raw_message` column encrypted with AES-256. Key rotation policy defined.
- **Secrets management**: `.env` file is not acceptable for production.
  Production deployments must use a secrets store (Vault, AWS Secrets Manager,
  or equivalent). Secrets must never appear in source control.
- **No-auth Go endpoints**: `MessageContentController` endpoints on port 8080
  are unauthenticated. These must be placed behind network policy (internal
  only) or given auth middleware before production.
- **Data retention enforcement**: retention periods configured per interface.
  The maintenance service exists; make retention mandatory, not optional.
- **Penetration test**: external pen test before first customer. Scope: API
  surface on both Node.js (3000) and Go (8080) services.

---

### 5. Load and resilience testing (2–3 weeks, after #1 and #2)

- **Throughput baseline**: messages/second per interface under sustained load.
  Identify the bottleneck (PostgreSQL writes, MinIO puts, or the Go engine).
- **Concurrent interface test**: 10 interfaces at volume simultaneously. Look
  for connection pool exhaustion, goroutine leaks, memory growth.
- **Chaos scenarios**: defined and tested behaviour for each failure mode:
  MinIO unreachable, FHIR receiver returns 503, PostgreSQL slow, Go service
  OOM. Each must have a documented expected outcome (DLQ, retry, alert) not
  undefined behaviour.
- **24-hour soak test**: Go service under moderate load for 24 hours. Memory
  profile before and after. No unexplained growth.

---

### 6. Operational readiness (2 weeks, overlaps with #5)

- **Health and readiness probes**: `/health` (process alive) and `/ready`
  (DB connected, MinIO reachable, pipeline engine running). Required for Docker
  and Kubernetes health checks.
- **Graceful shutdown**: in-flight messages must complete before the process
  exits. Currently `docker restart` can kill mid-transformation.
- **Migration rollback scripts**: every Flyway migration needs a corresponding
  rollback. Today they are forward-only.
- **Runbook**: documented procedures for the top 5 failure modes. Ops must be
  able to respond without involving engineering for routine incidents.
- **Customer onboarding checklist**: formal checklist per new interface — test
  with sample data, validate against US Core, confirm TLS, confirm retention
  policy, confirm DLQ alerting configured.

---

## Timeline

Workstreams 3 and 4 run in parallel. Workstreams 5 and 6 run in parallel after
1 and 2 complete.

```
Week  1–5   Workstream 1: Semantic correctness (GATE)
Week  3–6   Workstream 2: Delivery guarantees
Week  3–6   Workstream 3: Observability
Week  3–7   Workstream 4: Security hardening
Week  7–9   Workstream 5: Load and resilience testing
Week  7–9   Workstream 6: Operational readiness
──────────────────────────────────────────────
Week 9–10   Integration testing + first pilot customer go-live
```

Total: **10–12 weeks** to enterprise production grade.

---

## Scale Test Reference

The scale test harness lives at `tests/hl7-scale-test.js`. Run it with:

```powershell
# Full corpus, standard validation
node tests\hl7-scale-test.js --wait 15 --level standard

# Specific message types
node tests\hl7-scale-test.js --types "ORU^R01,VXU^V04" --wait 10

# Strict FHIR field validation
node tests\hl7-scale-test.js --wait 15 --level strict

# Stress: 10 repetitions of each variant
node tests\hl7-scale-test.js --repeat 10 --wait 30 --level standard
```

Results written to `tests/hl7-scale-test-results/`. Failing messages saved
with both the raw HL7 input and the FHIR bundle received, for use as regression
cases.
