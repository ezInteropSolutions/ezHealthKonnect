# ezHealthKonnect Capabilities Guide

A question-oriented reference for integration engineers: what the platform supports, which template to start from, and how the pieces fit. ezCompanion (the in-app assistant) reads this document; the pipeline builder's Documentation tab has step-by-step details for every pipeline step.

## Which message standards does ezHealthKonnect support?

| Standard | Typical use | Parse | Validate | Build (outbound) | Map to FHIR |
|---|---|---|---|---|---|
| HL7 v2 (2.1 – 2.8) | ADT, ORU, ORM, SIU and device results | yes | yes | `hl7.build` | yes (`hl7_fhir_transform`) |
| FHIR R4 | APIs, Bundles | yes | `fhir_validation` | `fhir.build` | native |
| C-CDA R2.1 (12 document types) | CCD, Discharge Summary, Referral, ... | `cda.parse` | yes | `cda.build` | `cda.to_fhir` |
| EDI X12 005010 | 835, 837P, 837I, 999, 270, 271, 276, 277, 278, 834 | `edi.parse` | `edi.validate` | `edi.build` | yes (templates) |
| NCPDP SCRIPT 2017071 | e-prescribing: NewRx, CancelRx, RxChange, RxRenewal, RxFill, Status/Error/Verify | `ncpdp.parse` | `ncpdp.validate` | `ncpdp.build` | yes (templates) |
| NCPDP Telecommunication D.0 | pharmacy claims: B1 (billing), B2 (reversal), B3 (rebill), E1 (eligibility) | `ncpdptelecom.parse` | `ncpdptelecom.validate` | `ncpdptelecom.build` | B1 (templates) |
| ASTM E1394-97 | lab analyzers | `astm.parse` | `astm.validate` | `astm.build` | via mapping steps |
| DICOM | imaging modalities | `dicom.parse` (metadata) | — | — | via mapping steps |

Messages arriving through a connector are detected and parsed automatically — a parse step is only needed for content that appears in the middle of a pipeline.

## How do I connect a medical device?

Open **Templates → Devices** (the "Connect a Device" wizard). Templates are named by how the device talks, not by brand, so any device using the same protocol and transport uses the same template.

| Device speaks | Template | Connector | Key settings |
|---|---|---|---|
| HL7 v2 over the network (ventilators, patient monitors, many analyzers) | **HL7 over TCP/IP** | `tcp_mllp_inbound` | port, results destination URL, optional bidirectional host-query mode |
| ASTM over the network | **ASTM over TCP/IP** | `astm_tcp_inbound` | port, max_connections, checksum_severity |
| HL7 v2 over RS232 (MLLP-framed) | **HL7 over Serial Port** | `serial_inbound`, protocol `hl7_mllp` | port_name (COM3, /dev/ttyUSB0), baud_rate (9600), data_bits, parity, stop_bits |
| ASTM over RS232 | **ASTM over Serial Port** | `serial_inbound`, protocol `astm` (default) | port_name, baud_rate, data_bits, parity, stop_bits, checksum_severity |
| DICOM (CT, MR, X-ray, ultrasound) | **DICOM Storage SCP** | `dicom_storage_inbound` | ae_title, port, optional TLS / mutual TLS, max_instance_size_mb (default 100) |

The wizard creates the interface, starts the listener, and waits for the device's first message. If the listener cannot start (for example the port is already used by another interface) the wizard shows the reason instead of waiting.

### Can I connect a device that sends HL7 over RS232?

Yes — use **HL7 over Serial Port**. The device must wrap each message in MLLP framing (start byte 0x0B, end bytes 0x1C 0x0D), the same framing HL7 uses over TCP, and it gets the same HL7 ACK back on the serial line. HL7 over serial without MLLP framing is not supported, and the bidirectional order-query mode is TCP-only. The machine running ezHealthKonnect must be cabled to the device (directly or through a USB-to-serial adapter).

If the device also offers HL7 over the network (LAN), prefer **HL7 over TCP/IP**. Many ventilators and monitors use a vendor-specific protocol on their serial port rather than HL7; for those, use the vendor's gateway, which typically sends HL7 over the network.

### How does the HL7 listener acknowledge device messages?

Every MLLP message — over TCP/IP or over a serial line — gets an HL7 ACK:
- **AA** — the message was received and queued for processing.
- **AR** — the frame was not an HL7 v2 message (no MSH segment). It is not processed; the device should treat it as rejected.
- **AE** — a valid message could not be queued (for example the processing queue is full). This is the default `on_error: nack` behavior: the device should resend. The opt-in `suppress` mode sends AA instead and **drops the message** — use it only for senders that cannot handle AE.

Settings live in the connector's **Acknowledgment** tab (sending app/facility, ACK text, optional custom `buildACK` script that can override the code).

### Which network ports can devices use?

In an installed (Standalone) system, a listener binds the port configured on its connector directly. Any free port works; on Linux this includes ports below 1024, such as DICOM's 104. The installer does not change the operating system's firewall. Allow each listener port in the Windows Firewall, `firewalld` or `ufw`, and in any network firewall between the devices and the server. The usual conventions are 2575 for HL7/MLLP and 104 or 11112 for DICOM.

(The development `docker-compose.yml` publishes only MLLP 2575 and 6610–6670 and DICOM 11112–11120.)

## How do I install, upgrade or roll back ezHealthKonnect?

See `docs/INSTALL_GUIDE.md`.
- **Install:** run `ezHealthKonnect-Setup-Win64.exe` (Windows) or `ezHealthKonnect-Setup-Linux-x64` (Linux) as administrator or root. On a server without a desktop, use `install --config answers.json`. Docker is not used in production.
- **First administrator:** open the one-time setup link the installer prints. There is no default login.
- **Upgrade:** run the newer installer. Before changing anything, it checks the database, the backup tools and at least 2 GB of free disk space. It then backs up the database and the application, installs and migrates the new version, and waits for the health check. On any failure it restores the previous version and its data automatically. `.env` and the data folders (`storage`, `logs`, `uploads`, `schemas`, `backups`) are never touched. Downgrades are refused.
- **Roll back:** `rollback --yes` returns to the version before the last upgrade. Data received since that upgrade is lost.
- **Uninstall:** `uninstall --yes` removes the services and the whole install directory, including stored messages and backups. It drops the database only if the installer created it; add `--keep-database` to keep it.
- **Database migrations** are applied by the application itself (`go-api migrate`). Flyway is not needed.

## How do I exchange EDI X12?

Start from a template in **Templates → EDI X12**:
- One template per transaction set → FHIR (835, 837P, 837I, 270, 271, 276, 277, 278, 834), received over SFTP (`edi_x12_inbound`).
- **EDI X12 Universal Receiver** accepts any of them on one interface and routes each file to the right FHIR mapping by its own transaction set.
- AS2 transport is available as the `as2_inbound` / `as2_outbound` connectors.
- `edi.generate_999` produces a 999 acknowledgment from validation results.

Real-time request/response (synchronous) endpoints: `POST /api/eligibility/{interfaceId}/check` (270→271), `/api/claim-status/{interfaceId}/check` (276→277), `/api/prior-auth/{interfaceId}/check` (278).

## How do I exchange NCPDP pharmacy messages?

- **Inbound to FHIR**: templates for every SCRIPT transaction (NewRx, CancelRx, CancelRxResponse, RxChangeRequest/Response, RxRenewalRequest/Response, RxFill) and for D.0 B1 claim request/response.
- **Outbound from FHIR**: "FHIR MedicationRequest to NCPDP SCRIPT NewRx (Outbound)" and "FHIR MedicationDispense to NCPDP D.0 B1 Claim (Outbound)". Mapping is no-code (`ncpdp.map_to_canonical` / `ncpdptelecom.map_to_canonical`), so the same pattern works from HL7, CSV or database sources.
- Real-time pharmacy claim submission: `POST /api/pharmacy-claim/{interfaceId}/submit`.
- A B1 built from a MedicationDispense lacks pricing (ingredient cost, U&C, gross amount due): FHIR does not carry the pharmacy's point-of-sale prices, so add them from the pharmacy system before sending.

## How does an external system call the real-time endpoints securely?

The four synchronous endpoints (eligibility, claim-status, prior-auth, pharmacy-claim) are called by payer, pharmacy or provider systems that have no ezHealthKonnect login. Each interface carries its own credentials:

1. Open the interface → **Edit** → **Partner Endpoint Authentication**.
2. Choose **API key** (header name, default `X-API-Key`), **Basic** (username/password) or **Bearer token**, and save. Secrets are encrypted and never shown again.
3. Give the partner the credential and the endpoint URL.

Until this is configured, partner calls are refused (`401 endpoint authentication is not configured`). After 10 wrong credentials within 15 minutes, that caller's address is blocked for 15 minutes (`429`, with `Retry-After`); other callers are unaffected. Failed attempts appear in the audit log as `SYNC_ENDPOINT_AUTH_FAILED`.

## Which roles can do what?

| Role | Can |
|---|---|
| admin | everything, including users, settings, audit logs |
| operator | create, change, activate and deactivate interfaces and pipelines |
| viewer | read interfaces, messages and monitoring; cannot change anything |
| user | no access until given an operator or viewer role (including ezCompanion) |

ezCompanion is available to viewers, operators and admins. It answers read-only questions (interface health, monitoring, DLQ statistics, analytics) directly; any action it proposes (activate, deactivate, retry) waits for an admin's approval before it runs.

## How are credentials and secrets protected?

- Connector credentials (passwords, tokens, API keys, private keys) are encrypted at rest with AES-256-GCM using `APP_CREDENTIAL_KEY`, and masked (`••••••••`) whenever configuration is shown. Leaving a masked field untouched keeps the stored value.
- The installer generates `APP_CREDENTIAL_KEY` once and keeps it on re-install. **Never change it** — stored credentials would become unreadable.
- In production the server refuses to start if `SESSION_SECRET` or `JWT_SECRET` is missing, a known placeholder, or shorter than 32 characters.
- In production the server also refuses to start if any secret (`SESSION_SECRET`, `JWT_SECRET`, `INTERNAL_PROXY_SECRET`, `APP_CREDENTIAL_KEY`, `DB_PASSWORD`, `MINIO_PASSWORD`, `OBJECT_STORAGE_SECRET_KEY`) matches a value that is publicly known, either because it was once published or because it is a placeholder. The check uses SHA-256 fingerprints in `config/compromised_secrets.json`, so the list holds no secret values. Both the web server and the processing engine run this check. The engine checks on its own because Standalone installs run it as a separate service.
- No secret has a built-in default. If the internal proxy secret is missing, the processing engine rejects every proxied request instead of accepting it.
- Running from source: `npm run setup:env` creates a local `.env` with freshly generated secrets. It never overwrites a value that is already there. `docker-compose.yml` refuses to start until `.env` provides them, and no secret value is ever committed to the repository. CI checks this with gitleaks, a repository policy test and a pre-commit hook.

## How do I run behind HTTPS?

Put a TLS-terminating reverse proxy (nginx, a load balancer) in front of the app, then set:
- `TRUST_PROXY=1` (or the proxy's address/subnet) so the app sees the real client address and knows the request was HTTPS;
- `SESSION_COOKIE_SECURE=auto` so session cookies are only sent over HTTPS.

The HL7/MLLP listener supports TLS 1.2/1.3 (`enable_tls`); the DICOM Storage SCP supports TLS and mutual TLS (`tls_ca_file`). Many devices cannot do TLS — in that case keep device traffic on an isolated clinical network segment.
