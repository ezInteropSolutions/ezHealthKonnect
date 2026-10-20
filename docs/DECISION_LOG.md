# ezHealthKonnect Decision Log

Product and architecture decisions that change what the platform supports or how it behaves by default. Newest first. Each entry records the decision, the evidence behind it and what it means for users.

---

## 2026-10-20 — Production installs are Standalone only; Docker is for development

**Decision:** Customers install ezHealthKonnect natively, as a Windows service or as Linux systemd services, using one self-contained installer per OS. The Docker edition, its install scripts and its production compose files are removed. `docker-compose.yml` remains only for developing ezHealthKonnect itself. The installer upgrades in place with a backup first and restores automatically on any failure. New installs get PostgreSQL 17; existing servers need PostgreSQL 15 or newer. Message content is stored on local disk by default; MinIO is not provided, and S3-compatible storage stays available.

**Why:** Depending on Docker in production meant depending on Docker Desktop licensing and on engine changes outside our control, and many hospital servers do not allow it. MinIO only ever stood in for S3 during development. Upgrades were manual ("back up, re-run, hope") with no automatic recovery, which is not acceptable for live clinical interfaces. PostgreSQL 15 reaches end of life in November 2027.

**Impact:** Installations made with the v26.08 installers cannot be upgraded in place: back up the database and `.env`, then install v26.10. Every release's installers are tested end to end (install, upgrade, a forced failed upgrade that must roll back, rollback, uninstall) on Ubuntu 22.04/24.04, AlmaLinux 9 and Windows before they are published.

---

## 2026-10-20 — The application runs its own database migrations (Flyway retired)

**Decision:** `go-api migrate` (`services/migrator`) applies `database/migrations` in place of Flyway. It uses Flyway's own history table (`flyway_schema_history`) and checksum algorithm. It refuses to run against a database that has tables but no migration history, unless an explicit baseline is given.

**Why:** Flyway needed a separate Java runtime, or a Docker container, on every customer server just to apply SQL files. The built-in engine ships inside the application binary.

**Evidence:** All 279 checksums match the values Flyway recorded. On a fresh database, the engine produces a schema identical to Flyway's (a full `pg_dump` diff is empty) and the same data in all 87 seeded tables. `flyway validate` accepts its history afterwards.

---

## 2026-10-20 — No default administrator account

**Decision:** No account with a known password exists after install. The installer writes a random `SETUP_TOKEN` to `.env` and prints a one-time link. The setup page creates the first administrator only with that token, and completing setup is atomic.

**Why:** A published default login (`admin123`) on a healthcare integration engine is a takeover risk for every install that is reachable before someone changes it.

---

## 2026-10-20 — No committed secrets, no secret defaults, refuse publicly-known values

**Decision:** No secret value is ever committed, and no code falls back to a built-in secret. Every secret comes from `.env` (from source: `npm run setup:env`; installers generate their own). In production, the web server and the processing engine both refuse to start if any secret matches a publicly known value. The processing engine fails closed when its proxy secret is missing.

**Why:** The development `docker-compose.yml` in this public repository held a working credential-encryption key and the dev session, JWT, database and object-storage secrets for weeks, and the code had built-in fallback secrets. Anyone who copied that file or ran with a missing variable used secrets the whole internet could read. The generic gitleaks rules caught only the key.

**How it works:**
- `config/compromised_secrets.json` lists SHA-256 fingerprints of every value that was published (plus common placeholders). Add new leaks with `node scripts/fingerprint-secret.js`.
- Recurrence is blocked at four points: a project gitleaks rule for secret variables in config files, a Jest policy test (`tests/unit/noCommittedSecrets.test.js`) covering config literals and code fallbacks, a gitleaks pre-commit hook (`.githooks/`, installed by `npm install`), and CI that generates its own secrets per run.
- Git history was not rewritten, because the published values are permanently public anyway. They are burned instead: refused in production and rotated everywhere.

**Impact:** A developer running from source must run `npm run setup:env` once. Customer installs are unaffected, because installers already generate unique secrets.

---

## 2026-10-19 — Support HL7 v2 over RS232 serial

**Decision:** Build it. The serial connector (`serial_inbound`) gains a **Protocol** setting: `astm` (default, unchanged) or `hl7_mllp`. A new device template, **HL7 over Serial Port**, uses it.

**Why:** A prospect's ICU equipment lists "HL7, LAN and RS232". HL7 v2 over a serial line is used in practice, but rarely: mostly older point-of-care and lab analyzers. Most modern devices send HL7 over the network, and most ventilators and monitors use a vendor-specific protocol on their serial port, reaching HL7 only through a vendor gateway.

**How it works:** Each message must arrive in MLLP framing (start byte `0x0B`, end bytes `0x1C 0x0D`), exactly as over TCP. The framing and the HL7 ACK (AA / AE / AR) are produced by the same code as the TCP listener. Noise on the line before a message starts is ignored.

**Limits:**
- HL7 over serial **without** MLLP framing is not supported.
- Bidirectional host-query (order lookup) is TCP-only.
- The server must be physically cabled to the device.

**Guidance:** Prefer **HL7 over TCP/IP** whenever the device offers it.

---

## 2026-10-19 — Device listeners reject what they cannot read

**Decision:** The HL7 listener answers a frame that is not an HL7 message with **AR**, and answers a valid message it cannot queue with **AE**, so the device resends it. The old behaviour is still available as the explicit opt-in `on_error: suppress`, which sends AA and drops the message.

**Why:** Before this change both cases got **AA** ("accepted"). A device told "accepted" never resends, so the message was silently lost. For a clinical feed that is not acceptable.

**Impact:** Existing interfaces keep their stored setting. The "HL7 over TCP/IP" template now defaults to `nack`.

---

## 2026-10-19 — Every partner endpoint requires per-interface credentials

**Decision:** The real-time endpoints (eligibility, claim-status, prior-auth, pharmacy-claim) refuse calls until the engineer configures an API key, Basic or Bearer credential on the interface. There is deliberately no "no authentication" option. After 10 wrong attempts from one address within 15 minutes, that address is blocked for 15 minutes.

**Why:** These endpoints run pipelines and return patient data to systems that have no ezHealthKonnect login.

---

## 2026-10-19 — ezCompanion knowledge refreshes per source

**Decision:** Each knowledge source (docs, step docs, each schema family, built-in overviews) is fingerprinted. At startup, only sources whose content changed are re-embedded. Documentation is always given a share of the passages ezCompanion reads before answering.

**Why:** Previously an upgraded install kept the previous release's knowledge until an admin re-ingested by hand. Configured interfaces also crowded the documentation out of answers.

**HL7 v2 and FHIR definitions:** ezCompanion learns a condensed summary rather than the raw dictionaries, which are far too large to embed (about 1.5 GB compressed for HL7). The summary covers each HL7 v2.5.1 segment with its fields, each message structure, and each FHIR R4 resource plus the US Core and Da Vinci PAS profiles. It comes to about 1,400 passages, embedded once.
