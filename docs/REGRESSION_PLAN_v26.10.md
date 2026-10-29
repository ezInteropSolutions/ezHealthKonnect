# v26.10 Regression Plan — change-to-test inventory

**Purpose:** prove that every app change since v26.08 still works, in the UI as
well as the API, before v26.10 is tagged. Every change area is mapped to the
test that proves it, and every gap has a fix and an owner step. Update this file
as gaps close.

**Scope:** 26 commits and 268 app files since `v26.08`.

## Where we stand (CI, commit d00990b)

| Suite | Result |
|---|---|
| Go unit + DB tests | green |
| Node unit (Jest) | 381 passed |
| Playwright (Docker dev stack, fresh DB) | **421 passed, 53 skipped** |
| Installer E2E (Ubuntu 22.04/24.04, AlmaLinux 9, Windows fresh + existing PG) | green |

**The 53 skips are the main finding.** They are not random. On a fresh
database, whole UI areas never run:

| Skipped | Spec | Root cause |
|---|---|---|
| 22 of 22 | `fhir-hl7-build-e2e` (fhir.build / hl7.build step builders) | depends on a hard-coded interface ID that exists only in one developer's database |
| 6 | `03-interfaces` (cards, filters, search, navigation) | "skip if no interfaces"; a fresh DB has none |
| 6 | `06-messages` (search, bulk bar, detail, tabs) | "skip if no messages" |
| 5 | `07-settings` (section nav, notifications, password validation, database) | to be diagnosed |
| 5 | `cda-fhir-pipeline` (CDA builders, template) | to be diagnosed |
| 2 | `04-interface-detail` | data-dependent |
| 2 | `engine-metrics` | needs a tripped circuit breaker / failing interface |
| 2 | `message-trace` | needs a live MLLP listener on 6613 that no fixture creates |
| 2 | `step-type-fix-validation` (CCD, EDI 835) | depend on interfaces in one developer's database |
| 1 | `05-monitoring` | data-dependent |

Second finding: **Playwright only runs against the Docker dev stack.** Customers
now run the installed app, which differs in ways that matter:
- bundled Node runtime and system services;
- local-disk storage, no MinIO;
- `SETUP_TOKEN` first-run setup (the Docker stack has no token, so this check is off);
- the built-in migration engine;
- a non-Docker settings page.

## Change areas → proof

✅ covered and running in CI · ⚠️ covered, but some tests skip in CI · ❌ no test

| # | Change area (commits) | Proof today | Status | Gap → fix |
|---|---|---|---|---|
| 1 | EDI X12: 835/837/999/270-271/276-277/278/834, universal receiver, step UI, templates | Go suites; `edi-*` specs; `edi-pipeline-ui` | ✅ | — |
| 2 | Sync endpoints (eligibility, claim status, prior auth, pharmacy claim) + partner endpoint auth | `sync-*` specs; `sync-endpoint-auth-e2e`; Go controller tests | ✅ | — |
| 3 | NCPDP SCRIPT and D.0: engine, templates, outbound | Go suites; `ncpdp-*` specs | ✅ | — |
| 4 | Pipeline engine fixes (field validation, if/switch branches, edi.build repeats, branch linking) | `step-type-fix-validation`, `pipeline-branch-linking` | ⚠️ | 2 tests depend on dev-only interfaces → G2 |
| 5 | fhir.build / hl7.build builder UI (conditions, nested groups, guardrails) | `fhir-hl7-build-e2e` | ⚠️ **all skip** | → G1 |
| 6 | CDA pipeline UI and template | `cda-fhir-pipeline`, `cda-guided-config` | ⚠️ | 5 skip → G2 |
| 7 | Devices: ASTM (serial, TCP), DICOM, HL7 over RS232, ACK changes, activation warnings | device/astm/dicom/serial specs; `activation-warnings-regression`; Go | ✅ | — |
| 8 | Audit: ATNA syslog export, GDPR export/erasure | `settings-atna-syslog`, `gdpr-export-erasure` | ✅ | — |
| 9 | Security: auth + role gates on every Go route; CSP; credential encryption at rest | Jest route-auth test; `csp-every-page`; Jest credential tests | ⚠️ | No browser test logs in as **viewer/operator** and checks the UI stays usable (read-only views load, writes are refused cleanly) → G5 |
| 10 | Interface Edit modal: Partner Endpoint Authentication editor | `sync-endpoint-auth-e2e` (API) | ⚠️ | Editor UI itself not driven in a browser → G5 |
| 11 | Secrets policy, setup token, locked placeholder admin, `admin123` removed | Jest `noCommittedSecrets`, `setupGate`; installer E2E (API) | ⚠️ | Setup page with `?token=` never driven in a browser; token is off in Docker → G4 |
| 12 | Built-in migration engine (replaces Flyway) | Go migrator DB tests; CI compose `migrate`; installer E2E | ✅ | — |
| 13 | Local storage driver hardening | Go tests; installer E2E stores and reads an object | ⚠️ | Content viewer (raw/parsed tabs) on local storage not exercised in the UI → G3 |
| 14 | Engine: recovery and reprocess read content from storage (V60 column) | Go build; `bulk-reprocess-e2e` | ⚠️ | Startup recovery has no test; single-message Reprocess in the UI skips on a fresh DB → G6, G1-seed |
| 15 | Wizard creates interfaces with the standard message table | wizard specs create interfaces | ⚠️ | Nothing asserts the table schema or that a message then flows → G6 |
| 16 | Legacy FHIR receivers removed; HTTP FHIR Receiver fails closed | Jest guard; `legacy-fhir-receiver-removed`; Go tests | ✅ | — |
| 17 | Interfaces list / detail / messages pages (`interfaces.js`, `messages.js`, `dashboard.js` edits) | `03`, `04`, `06` specs | ⚠️ | 14 tests skip on a fresh DB → G1-seed |
| 18 | Settings page: Docker ports hidden outside Docker | none | ❌ | → G3 (installed-app run) |
| 19 | ezCompanion knowledge refresh | Go ingestion tests | ✅ | Chat UI not in scope for v26.10 |
| 20 | Installer: install / upgrade / failed-upgrade rollback / rollback / uninstall; Windows virtual service account | installer E2E, 5 platforms | ✅ | — |

## Gap fixes (in order)

- **G1-seed — real test data on a fresh DB.** A shared Playwright fixture creates and activates a real interface through the same APIs the UI uses, sends real messages through it, and tears everything down afterwards. Specs `03`, `04`, `05`, `06` and `message-trace` use the fixture instead of skipping. A spec may skip only for an explicit, documented environment reason, never "no data".
- **G1 — builder suite.** `fhir-hl7-build-e2e` creates its own interface instead of using a hard-coded ID.
- **G2 — dev-only interfaces.** `step-type-fix-validation` (CCD, EDI 835) and `cda-fhir-pipeline` create what they need from templates; diagnose the `07-settings` skips the same way.
- **G3 — installed-app Playwright job.** CI installs with the real installer on Ubuntu (fresh DB, setup with the token), then runs the full Playwright suite against it. A Windows browser smoke set follows.
- **G4 — setup token in the browser.** A new spec: no token or a wrong token is refused, the `?token=` link completes setup, a second setup is refused, and the placeholder admin can't log in before setup.
- **G5 — roles in the UI.** Log in as viewer and as operator: read pages load, write actions are refused with a clear message, and no page breaks. Drive the Partner Endpoint Authentication editor (set, mask, keep, remove).
- **G6 — engine and wizard data path.** Create an interface through the wizard, send a message, and assert the standard table schema and the stored content. Then **Reprocess** it from the message view, and leave a message stuck in processing, restart the service, and assert recovery picks it up (installer E2E, where the service can be restarted).

## Status of the gap fixes

| Gap | Status | Where |
|---|---|---|
| G1-seed | ✅ done | `helpers/seeded.js` — worker-scoped fixture: an activated MLLP interface with 3 real messages, plus an inactive second interface; used by specs 03, 04, 05, 06, `message-trace`. Skip-on-no-data guards became assertions. |
| G1 | ✅ done | `fhir-hl7-build-e2e` uses the seeded interface; FHB-E2-005 persists its own pipeline (no migration ever created the demo one). 22 of 22 run. |
| G2 | ✅ done | `step-type-fix-validation` (CCD, EDI 835) builds its interfaces from the OOB templates (`ApiHelper.createInterfaceFromTemplate`); `cda-fhir-pipeline`, `07-settings`, `engine-metrics` (real tripped circuit breaker) rewritten against what the product really has. |
| G3 | ✅ proven locally, CI run pending | `installer.yml` jobs `playwright-installed` (Linux) and `playwright-windows-smoke`. The Linux flow was run for real here: the production installer inside a systemd container, first-run setup with the token through the browser, then the whole suite. It found that an install has **no schema packs** (every FHIR/HL7/EDI-to-FHIR flow fails until they are installed from Settings → Schemas), so the job now installs `fhir-r4-base`, `fhir-r4-us-core` and `hl7-v2-25` first. The Windows smoke set (first-run, dashboard, interfaces, messages, wizard data path, roles, restart recovery) is written but can only be proven by a CI run. |
| G4 | ✅ already existed | `first-run-setup.spec.js` (6 tests); it runs for real in the installed-app job. |
| G5 | ✅ done | `roles-and-endpoint-auth.spec.js`: viewer/operator in the browser (read pages, disabled write controls with the reason, blocked re-enabled controls, visible refusal messages, "Access denied" panels, menu contents), the Partner Endpoint Authentication editor (set, masked, keep, replace, remove) and the Edit modal's File Writer validation message. The UI side is `public/js/core/app-auth.js` (see CLAUDE.md, "Role-Aware UI"). |
| G6 | ✅ done | `wizard-data-path-e2e.spec.js` (wizard → activate → MLLP message → stored content → table schema → Reprocess in the UI) and `service-restart-recovery.spec.js` (stuck message recovered after a real restart; needs `DATABASE_URL` + `RESTART_COMMAND`, set in both CI jobs). |
| Wizard flows | ✅ done | `wizard-flow-navigation.spec.js`: every processing flow × source format, every connector type. |

## Real defects found while closing the gaps (all fixed unless stated)

1. **Wizard: Custom / File Processor / Passthrough landed on the FHIR mapping page, then "Invalid step".** Step skipping jumped to model step 5 (the CDA layout's target step). Fixed in `WizardModel.js` / `FlowRegistry.js`; File Processor and Custom now show the real outbound connector builder.
2. **Wizard-created Passthrough / FHIR Receiver interfaces failed every message** ("outbound connector type 'sink' not found"). The wizard saved connector type `sink`; the registered type is `sink_outbound`. Fixed in `TransformationPipelineService.js`; `V282` repairs stored steps.
3. **Startup recovery never worked for real messages.** It inserted the message id into a UUID column of `message_processing_queue`, and the queue's consumer never updated the message's own row anyway. Recovery now re-runs stale messages through the engine's reprocess path (`processing/engine.go`).
4. **UI showed write controls and admin pages to everyone, refused writes were silent, and every non-admin page made two admin-only calls that returned 403.** Fixed with `app-auth.js`: capability table mirroring the server, `data-requires` markup, a click guard, a refusal toast, an "Access denied" panel on admin pages, per-item menu gating (Message Trace for every reader, Dead-Letter queue for admins only), and role-gated pollers. `user-management.js` also locked `super_admin` out of the Users page (`role !== 'admin'`); fixed.
5. *Two findings from earlier in this work were wrong, and are corrected here:* the interfaces list **does** have a search box and status/type/format filters (`table-component.js`; my first check only searched the HTML page), and the Edit modal **does** show a toast when connector validation fails (the toast is a plain `<div>`, which my probe's selector missed). The real cause of the failing File Writer save was the test fixture sending `outputPath`, which the File Writer connector ignores (it reads `directory_path`); fixed, and toasts now carry `role="alert"`/`status` so they are findable.
7. **`GET /api/ai/status` answered 503 whenever the AI service was not reachable.** That is the normal state of an install without Ollama, so every dashboard load logged a console error and four template specs failed on the installed app only ("expected no console errors"). It now always answers 200 with the state in the body (`controllers/ai_controller.go`). Found by the installed-app run; the four specs pass on the install after the fix.
6. *Still open, not fixed:* the interface detail page has no Configuration tab; Settings has no Database section or change-password form; `/api/cda/mappings/translations` is not implemented (these remain `test.fixme` with the reason stated). **Open question for the product owner:** a viewer or operator sees the global live-message feed (interface names and message ids of other users' interfaces) although their interface list is scoped to the ones they own; the role matrix in User Management says viewers have full message access, which the ownership scoping contradicts.

**Release gate:** all four suites (Docker, installed app, installer E2E, Go/Jest) are green, there are **zero "no data" skips**, and the hands-on pass on the installed Windows build is done.
