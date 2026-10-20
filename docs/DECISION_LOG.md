# ezHealthKonnect Decision Log

Product and architecture decisions that change what the platform supports or how it behaves by default. Newest first. Each entry records the decision, the evidence behind it and what it means for users.

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
