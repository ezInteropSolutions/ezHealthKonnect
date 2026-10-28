# QA Strategy — From "Tests Pass" to "A Customer Can Use It"

**Why this exists.** v26.10 shipped with a green regression suite and two defects a customer saw
within minutes of installing: icons rendering as raw text, and several wizard flows broken. Both
escaped because our QA proves that *the code we wrote works in the environment we built it in*.
A customer's machine differs in network, data, accounts, browser, upgrade path and expectations.
This document defines the QA that closes that difference. The `qa` skill (`.claude/skills/qa/`)
executes it.

**Principle.** A test passes only if a person looking at the screen would agree the feature works.
"The DOM element exists" and "the API returned 200" are necessary, not sufficient.

---

## 1. Why v26.10 escaped (root causes, verified against the repo)

| # | Escape | Root cause | Evidence |
|---|---|---|---|
| 1 | Icons show as text | Icons load from a public CDN (Font Awesome on cdnjs, e.g. `public/dashboard.html:8`); the CSP allows only that CDN for fonts (`app.js:52`). Every CI run has internet, so the failure cannot occur there. On an install without outbound access the font never loads. | Needs confirming on the affected install (Network tab: blocked cdnjs request). |
| 2 | Nothing asserts what a user sees | No test fails on a failed request, console error, missing glyph, clipped text or layout overflow. `csp-every-page.spec.js` checks CSP violations only. | Spec review |
| 3 | Wizard flows broken | Wizard has many flows (device, DICOM, ASTM TCP/serial, templates, generic). The Windows installed job runs 1 wizard spec; flows are mostly tested against seeded/API-created interfaces rather than clicked through on a clean install. | `installer.yml:346-352` |
| 4 | Cross-browser / responsive never run | Firefox, `mobile-chrome` and `tablet` projects exist in `playwright.config.js` but CI invokes only `--project=chromium`. Firefox is 3 smoke specs; responsive is one spec. | `ci.yml:390`, `installer.yml:251` |
| 5 | Clean-install state differs from dev | Fresh installs have no schema packs, a first-run setup flow, no history. Dev/CI fixtures are pre-seeded. | CLAUDE.md "Installed-app runs" |
| 6 | Builder-written, builder-verified | Same author writes feature, fixture and assertion; fixtures encode the author's assumptions. | Recurring pattern in prior bug write-ups |

---

## 2. The six layers

| Layer | Question | Today |
|---|---|---|
| L1 Unit / integration | Does the logic work? | Strong |
| L2 Feature E2E | Does each feature work through UI and API? | Strong breadth, shallow assertions |
| L3 Environment matrix | Does it work where customers run it? | Missing |
| L4 Journey / UAT | Can a customer do their job, install to daily use? | Missing |
| L5 Non-functional | Fast, secure, accessible, resilient, supportable? | Security only |
| L6 Release / upgrade | Does the shipped artifact install, upgrade, roll back, uninstall? | Partial |

A release needs all six.

---

## 3. UI testing (L2/L3) — what to verify on EVERY page

There are 29 pages in `public/`. Each is exercised against this checklist; the automated parts
live in one shared fixture so a new page is covered by adding it to the page list, not by
writing new code.

### 3.1 Automatic gates (fail the test, no judgement needed)
- No console errors or warnings of type `error`.
- No failed network requests (4xx/5xx, aborted, blocked) except those a test explicitly expects.
- No requests to hosts outside the app origin (this catches every CDN dependency).
- **Icon integrity:** every icon element renders a glyph (`document.fonts.check`, computed
  `font-family` resolves to the icon font, pseudo-element `content` is not the fallback text).
  No visible element's text matches known icon names / ligatures (`fa-`, `material`, `\f0..`).
- No uncaught promise rejections.
- No horizontal page scroll at supported widths (unless the page is intentionally a canvas).
- No element overflowing its container (`scrollWidth > clientWidth` on non-scrollable elements).
- No text clipped by `overflow:hidden` without an ellipsis + tooltip (`scrollWidth > clientWidth`
  with `text-overflow` unset).
- No overlapping interactive elements (hit-test centre of every button/link returns itself).
- Page `<title>` set, `lang` set, one `<h1>`.
- Axe accessibility scan: zero critical/serious violations.

### 3.2 Every interactive element
Enumerate with a locator sweep (`a[href]`, `button`, `[role=button]`, `[onclick]`,
`[data-action]`, `input`, `select`, `textarea`, tabs, menu items, table row actions) and for each:
- **Links:** resolve (no 404), correct destination, open in the right target, back button works,
  deep link works when logged out (redirects to login, then returns).
- **Buttons:** click produces the *stated* outcome (modal, navigation, API call, toast, state
  change) — assert the outcome, not just that the click did not throw.
- **Disabled states:** disabled when they should be (role, missing input, in-flight), enabled
  when they should be; disabled controls explain why (tooltip).
- **Double-click / rapid click:** no duplicate submissions.
- **Keyboard:** reachable by Tab in logical order, visible focus ring, Enter/Space activate,
  Esc closes modals and returns focus.
- **Destructive actions:** confirm dialog, cancel truly cancels, confirm truly deletes, audit row written.

### 3.3 Forms and inputs
- Required fields: empty submit shows an error next to the field, names the field, focus moves to it.
- Boundaries: min/max length, 0, negative, decimal, very large numbers, leading/trailing spaces.
- Hostile / awkward text: `<script>`, `'"`, `%`, emoji, RTL text, 10 000-char strings, newlines,
  non-ASCII names (e.g. "O'Brien", "Müller", "李").
- Paste, autofill, browser back after submit, refresh mid-form (is data lost? is it warned?).
- Unsaved-changes warning when leaving a dirty form.
- Server-side error surfaced to the user in plain language, not "[object Object]" or a stack trace.
- Success feedback is visible and not auto-dismissed before it can be read.

### 3.4 Dropdowns, selects, comboboxes, multiselects, date/time pickers
- Opens, closes on outside click and Esc, closes when another opens.
- **Options are not clipped** by a parent with `overflow:hidden`, a modal edge, or the viewport
  bottom (open each near the bottom of the screen and in a scrolled container).
- Long option labels wrap or truncate with tooltip; they do not push the layout.
- Empty list shows "No options" instead of a blank box; loading state while options fetch.
- 0, 1, 10, 1 000 options (virtualised / searchable when large). Search filters correctly,
  case-insensitively, with special characters.
- Selected value persists after save and reload; pre-selected value on edit matches the DB.
- Dependent dropdowns reset correctly when the parent changes.
- Keyboard: arrows move, type-ahead, Enter selects.
- Multiselect: select all / none, chip removal, count display.

### 3.5 Tables, lists, pagination
- Empty state (with call-to-action), loading skeleton/spinner, error state with retry.
- 0, 1, 25, 10 000 rows: pagination controls correct, page size honoured, last page correct,
  total count matches DB, sort asc/desc/stable, sort + filter + page combined.
- Column truncation with tooltip; no column pushed off-screen; sticky header; horizontal scroll
  where intended; row actions menu not clipped at the last row.
- Timestamps: timezone shown or documented, relative vs absolute consistent, locale formats.
- Selection (single, page, all), bulk action confirmation and result summary.
- Auto-refresh does not reset scroll, selection, open menus or typed filters.

### 3.6 Layout, display and responsiveness
- Widths: 360, 768, 1024, 1366×768 (most common laptop), 1920, 2560. Zoom 100/125/150/200 %.
  High-DPI. Browser text-size increase.
- Nothing cut off: titles, buttons, badges, table cells, toasts, tooltips, modals taller than the
  viewport (modal body scrolls, header and footer stay reachable).
- Sidebar collapsed / expanded; long interface names; long user names and emails; 50-character
  step names in the pipeline builder.
- Light/dark or print stylesheet if offered.
- Modals/toasts stack correctly; z-order is right (dropdown inside modal is visible).
- Images/logos load with no broken-image icon; favicon present.

### 3.7 Pipeline builder / canvas (special case)
Drag, drop, connect, delete, undo, zoom, pan, select multiple, auto-layout, 50-step pipeline,
overlapping nodes, step config panel for every step type (each opens without error and shows
its defaults), save, reload, clone, run Test Pipeline and read the result.

### 3.8 Content and wording
Spelling, tone, consistent terms, no developer jargon or raw keys in the UI, no `undefined`,
`null`, `NaN`, `[object Object]`, no lorem ipsum, error messages say what to do next.

### 3.9 State, session and navigation
Session expiry mid-action (what does the user see?), logout in another tab, two tabs editing the
same record, browser back/forward, refresh on every page, bookmark of a deep URL, first-run flow
(`/setup.html`), invite link, forced password reset, change password.

### 3.10 Browsers and devices
Chromium, Firefox, WebKit (Safari), Edge. Tablet and phone widths for pages the product claims
to support. **CI must run these projects**; today it runs Chromium only.

---

## 4. Database testing (L1/L5)

### 4.1 Persistence
- Create → read back → update → read back → delete → confirm gone, for every entity, through the
  UI **and** verified directly in the database (not just from the API response — see CLAUDE.md
  tri-state JSONB lesson).
- **Reload test:** after every save, hard-refresh and confirm the value shown equals the DB value.
- **Restart test:** restart the service, then confirm configuration, activated interfaces,
  queues and message history survived (`service-restart-recovery.spec.js` is the model).
- Round-trip fidelity: unicode, very long text, empty string vs null vs missing, JSONB null
  sentinel, timestamps/timezones, decimals, large payloads, binary/base64 content.
- Defaults applied; constraints enforced (unique, FK, NOT NULL) with a friendly error, not a 500.
- Soft delete vs hard delete behaves as documented; deleted data not resurfaced by joins.
- Concurrent edit of the same row: last-write-wins vs conflict is deliberate and tested.
- Transactions: a failure midway leaves no partial rows (create interface = interface + pipeline +
  message table + connector config, all or nothing).
- Retention jobs delete what they should and nothing else.
- Encryption at rest of credentials verified in the table, masked on every read.

### 4.2 Migrations
- Fresh install applies every migration from V1 cleanly on PG 15 and 17.
- **Upgrade from each of the last two releases** with a realistic dataset; row counts and key
  values preserved; checksum history intact; re-run is a no-op.
- Migration against a database with long-lived, messy customer-like data (nulls, odd enums).
- Rollback / restore-from-backup path works (installer already backs up before upgrade).

### 4.3 Database performance
- Dataset tiers: empty, 100 k, 1 M, 10 M message rows (per-interface tables).
- Budgets (initial, tune after the first baseline): list/search page < 500 ms p95 at 1 M rows,
  message detail < 300 ms, dashboard < 1 s, write path sustained at the documented msg/s.
- `EXPLAIN (ANALYZE, BUFFERS)` for each list/search/report query; fail on seq scan of large
  tables where an index is expected; check the audit-log search (trigram) path.
- Connection pool: exhaustion behaviour, leaked connections after errors, behaviour when
  Postgres restarts under load.
- Table/index bloat and growth after a soak; retention keeps disk bounded.
- Lock contention: bulk delete, reprocess and live ingestion at the same time.

---

## 5. API testing (L2/L5)

Every route (Node routes in `routes/`, Go routes in `main.go`, and the proxied surface in
`app.js`) has a row in a generated route inventory. The route list is extracted from code, so a
new route without tests shows up as a gap.

### 5.1 Functional contract
- Happy path: status, body schema (validated against a schema, not eyeballed), headers,
  content type, pagination envelope, idempotency of GET/PUT/DELETE.
- Each documented error: 400 (validation, field-level messages), 401, 403, 404, 409, 413, 415,
  422, 429, 5xx. Error body is JSON with a stable shape; no stack traces, SQL, or file paths.
- Input matrix per field: missing, null, wrong type, empty, too long, boundary, unicode,
  injection strings, extra unknown fields (ignored or rejected, deliberately).
- Pagination/sort/filter parameters: invalid values, huge page size, negative offset.
- Backward compatibility: response fields consumed by the UI are not renamed silently
  (contract test between API and `public/js`).
- Partner endpoints (`/api/eligibility`, `/api/claim-status`, `/api/prior-auth`,
  `/api/pharmacy-claim`) with each content type and malformed payloads.

### 5.2 Security
- **AuthN:** every route rejects anonymous callers except the allow-list (health, login, setup,
  invite, partner endpoints with their own credential). `tests/unit/app.forwardToGoAuth.test.js`
  enforces this for forwarded routes; extend to all Node routes.
- **AuthZ per role:** a matrix of route × role (admin, super_admin, operator, engineer, viewer,
  user, anonymous, partner credential) with the expected status. Includes horizontal checks
  (user A cannot read user B's interface/message by ID — IDOR) and vertical checks (viewer
  cannot write).
- Access-policy gating of message content; metadata-only roles never receive content in any
  response, log, export, or error.
- Session: cookie flags (HttpOnly, Secure, SameSite), fixation, logout invalidates server-side,
  expiry, concurrent sessions, brute-force lockout (login, partner endpoints), password policy.
- CSRF on state-changing routes; CORS not wildcard; security headers (CSP, HSTS, X-Content-Type,
  frame-ancestors); no verbose server banner.
- Injection: SQL (every filter/sort/search param), NoSQL, command, path traversal (file browser,
  storage keys), SSRF (connector test URLs, FHIR/HTTP outbound), XML (XXE in CDA/SCRIPT parsers),
  template/script sandbox escapes (goja scripts), ReDoS, zip/decompression bombs, oversized
  bodies, malformed multipart.
- Secrets: never in responses, logs, error messages, URLs, audit rows; `ENC:v1:` values masked on read.
- Dependency scan (npm audit, govulncheck — already in CI), container image scan, SBOM.
- Rate limiting on auth and expensive routes; verified to fire.
- File upload: type and size limits, filename sanitisation, storage outside web root.
- Audit: every privileged action writes an audit row with the real user id.
- Periodic third-party penetration test before each major release.

### 5.3 API performance
- Per-route latency p50/p95/p99 at 1, 10, 50 concurrent users with the realistic dataset.
- Throughput of the ingestion path (messages/s) per connector type, with and without pipeline.
- Payload sizes: 1 KB, 100 KB, 10 MB, at the configured maximum, and one byte over.
- Soak (8 h): memory, goroutines, open file descriptors, DB connections must be flat.
- Backpressure: slow downstream, full queue — correct NACK/AE and no message loss.
- Degradation: DB down, MongoDB/object store down, disk full — clear error, no data corruption,
  automatic recovery.
- Regression gate: p95 may not worsen by more than 20 % versus the stored baseline.

---

## 6. UAT — act as the user (L4)

UAT is performed on the **release artifact installed on a clean machine** (VM, no dev tools, no
internet unless the scenario says otherwise). A person other than the feature's author runs it,
following the scripts below, and records screenshots and notes. Where possible the same journey
is also scripted in Playwright so it runs every release.

The tester's job is to *think like the persona*, not to follow the happy path: try the wrong
button, leave a field blank, go back, come back tomorrow, hand the task to a colleague.

### 6.1 Personas

| Persona | Goal | Role / access |
|---|---|---|
| **Priya — IT administrator** | Install, secure, maintain, upgrade | admin / super_admin |
| **Marcus — integration engineer** | Build and operate interfaces and pipelines | operator / engineer |
| **Dana — support / operations analyst** | Watch feeds, find a lost message, resolve alerts | operator |
| **Sam — compliance / security officer** | Audit trail, access control, retention, GDPR | admin (audit) |
| **Lee — clinical-informatics viewer** | Look at results, never change anything | viewer |
| **Pat — basic user** | Use the HL7 reader on their own data | user |
| **Trading partner system** | Machine calls partner endpoints | partner credential |
| **New hire / first-time user** | Learn the product without help | invited user |
| **Hostile or careless user** | Wrong input, curiosity, probing limits | any |

### 6.2 Journeys (each is a script with expected outcomes)

**Priya — administrator**
1. Download the installer, install on a clean Windows Server and a clean Linux host (no
   internet variant too). Follow only `docs/INSTALL_GUIDE.md` — any step the guide omits is a defect.
2. First-run: open setup link with token, create the first admin, log in. Try reusing the token,
   a wrong token, a weak password, a mismatched confirmation.
3. Settings: set organisation values, SMTP, retention, password policy, access policy; save;
   reload; confirm persisted and effective.
4. Create users of every role, send invite, accept invite in a second browser, forced password reset.
5. Install schema packs; confirm dependent features unlock and tell the user what is missing before.
6. Back up, then upgrade from the previous release with live interfaces; confirm nothing changed
   (interfaces, messages, users, credentials); force an upgrade failure and confirm auto-restore.
7. Restart the server / reboot the machine; confirm interfaces auto-resume and no messages are lost.
8. Uninstall; confirm what remains is documented. Reinstall over the old data directory.
9. Run out of disk, break the DB connection, expire the licence/cert — what does the admin see
   and is it actionable?

**Marcus — integration engineer** (every template and connector is a row in the generated matrix)
1. "I need to receive HL7 ADT from the hospital": Use Template → configure → activate → send a
   message from an external tool → see it processed → open the trace.
2. Same for each wizard flow, each OOB template, each connector type, inbound and outbound,
   including bad configuration (wrong host, wrong port, wrong credentials, TLS mismatch) — the
   error must say what is wrong.
3. Build a pipeline from scratch: add each step type, configure, validate, Test Pipeline, fix an
   error found by the test, save, activate, send real traffic, edit while live, roll back.
4. Edit an existing interface (every tab), change connector type, rename, clone, delete, restore.
5. Bulk: 20 interfaces, search/filter/sort them, find one by partial name.
6. Mapping: build HL7→FHIR mapping, custom code-template, code translation import/export CSV.
7. Reprocess a failed message; redrive from the DLQ; confirm exactly one output.
8. Large and awkward messages: 5 MB, 10 000 segments, unusual encodings, batch files, duplicates.

**Dana — operations**
1. Morning check: dashboard, alerts, monitoring, engine metrics — do the numbers match reality?
2. "Patient X's lab result did not arrive": search messages by ID, patient, date, status; trace
   the journey; see where it stopped; take the next action.
3. Respond to an alert; acknowledge; see it clear; confirm notifications actually delivered.
4. Export a report; open it; check dates, totals, headers.
5. Pause/resume an interface; confirm queued messages are neither lost nor duplicated.

**Sam — compliance**
1. Review the audit log: filter by user, action, date; every earlier journey step must appear
   with the correct actor, time and risk level.
2. Verify a viewer/user cannot see message content; flip the access policy and verify the effect.
3. GDPR export and erasure of a user; confirm the result and the audit entry.
4. Confirm credentials are masked in UI, API, logs, exports, and encrypted in the DB.
5. Retention job: confirm old data purged, recent data intact.

**Lee — viewer / Pat — user**
1. Log in; every write control is disabled or hidden with an explanation; no raw 403 pages.
2. Deep-link into a page they may not open — friendly "access denied", not a blank page.
3. Pat uses only the HL7 reader; cannot reach other pages by URL guessing.

**Trading partner**
1. Call each partner endpoint with correct credentials, wrong credentials, no credentials,
   locked-out credentials, malformed payload, oversized payload.

**New hire**
1. Without documentation, can they create their first interface in 15 minutes? Where do they
   get stuck? Is the in-app help, tooltip or documentation tab enough?

**Hostile / careless user**
1. Paste scripts in every text field, upload the wrong file type, double-submit, use two tabs,
   log out mid-save, edit the URL IDs, replay an old request, disable JavaScript, block cookies.

### 6.3 Cross-cutting UAT questions (ask on every screen)
- Do I know where I am and how to get back?
- Does every action give feedback (and the right feedback)?
- Can I undo or recover from a mistake?
- Would I understand this error without calling support?
- Does it look finished (alignment, spacing, icons, wording)?
- Is anything slow enough that I would click twice or leave?
- Does the data I see match reality (counts, times, statuses)?

### 6.4 Recording UAT
For each journey: pass / fail / blocked, build number, environment, tester, screenshots, defects
raised, and a free-text "what felt wrong". Stored under `tests/uat/<release>/`.

---

## 7. Other areas easy to miss

| Area | What to check |
|---|---|
| **Install / uninstall / upgrade** | Clean OS, no dev tools, no internet, non-default paths and ports, low-privilege account, antivirus/firewall interference, service starts on reboot, port conflicts, path with spaces, non-English OS locale |
| **Time** | Timezones, DST change, clock skew, leap day, date formats, message timestamps vs server timestamps |
| **Internationalisation** | UTF-8 end to end (HL7 charset handling, names with accents/CJK), long strings, sorting order |
| **Resilience** | Kill -9 during write, power loss, network partition, downstream timeouts, retry storms, duplicate delivery, poison messages |
| **Observability / supportability** | Version visible in UI, logs findable and rotated, health endpoint, diagnostic bundle, log level change without restart, no PHI in logs |
| **Data protection** | PHI never in URLs/logs/analytics; backups encrypted; test data de-identified |
| **Accessibility** | Keyboard-only use, screen reader labels, contrast, focus order, zoom to 200 % |
| **Documentation** | Install guide, runbook and in-app help match the shipped behaviour; screenshots current |
| **Licensing / legal** | Third-party licence list, notices bundled in the installer |
| **Compatibility** | Browser versions supported (policy written down), PG 15/17, OS versions, screen readers |
| **Integration with real peers** | Real interface engines and test tools (HL7 test harnesses, Mirth, Postman/SoapUI, openssl s_client, sftp servers) not only our own clients |
| **Negative / chaos** | Fuzzing of parsers (HL7, X12, XML, MLLP), malformed TLS, slow-loris, partial frames |
| **Cost / capacity** | Disk growth per million messages, memory per interface, max interfaces per node |
| **Third-party dependency failure** | CDN, DNS, NTP, SMTP server, LDAP/SSO down |
| **Error budget of UX** | Time to first interface, time to find a message, number of clicks for top tasks |

---

## 8. Release gate and process

A release tag requires **all** of:
1. L1–L2 green on Chromium **and** Firefox, WebKit, mobile and tablet projects (to be added to CI).
2. L3 environment matrix green: offline run, Windows + Linux install, PG 15 + 17.
3. Automatic UI gates (section 3.1) green on all pages, zero allow-listed exceptions without a ticket.
4. API security matrix and performance baseline within budget.
5. Upgrade from the previous two releases passes with data intact.
6. UAT journeys (section 6) executed by someone other than the author, signed off, zero
   open severity-1/2 defects.
7. Documentation walk-through done against the built artifact.

**Escape rule.** Every defect found after release gets (a) a test that fails without the fix,
merged with the fix, and (b) a one-line note in `docs/QA_ESCAPES.md`: which layer should have
caught it and what changed so it will. If the answer is "no layer could have", add the layer.

**Independence.** The author of a feature writes its L1/L2 tests. UAT and the clean-machine run
are done by someone else (or by a fresh agent session with no memory of the implementation).

**Traceability.** Maintain `tests/uat/coverage-matrix.md`: feature × journey × environment ×
last-run result. Blank cells are known gaps, visible to everyone.

---

## 9. Implementation roadmap

| Phase | Deliverable | Closes |
|---|---|---|
| 0 | Vendor Font Awesome and other CDN assets into `public/vendor/`, tighten CSP to `'self'` | Escape 1 |
| 1 | Shared Playwright fixture: console/request/off-origin/icon/overflow/axe gates; page sweep over all 29 pages | Escapes 1, 2 |
| 2 | Offline CI job (block outbound) for the installed build | Escape 1 |
| 3 | Run Firefox, WebKit, mobile, tablet projects in CI; widen Firefox beyond 3 specs | Escape 4 |
| 4 | Generated journey matrix: every wizard flow × every template × every connector, clicked on a clean install (Linux and Windows) | Escape 3 |
| 5 | Screenshot baselines for pages and wizard steps | Layout regressions |
| 6 | Route inventory + route × role authorisation matrix + schema-validated API contract tests | API gaps |
| 7 | Performance harness: dataset generator, latency budgets, soak, DB `EXPLAIN` checks | Performance |
| 8 | Upgrade-from-previous-release job with seeded data | Escape 5 |
| 9 | UAT scripts in `tests/uat/`, coverage matrix, escape log, release checklist in `docs/RELEASE_CHECKLIST.md` | Escape 6 |

Phases 0–3 are small and remove the failure class seen in v26.10; 4–9 make QA comprehensive.

## Running the exploratory sweeps
`qa-ui-sweep.spec.js` (every page x 4 viewports) and `qa-click-sweep.spec.js` (presses every non-destructive button) are collectors, not CI gates: they report findings, including known open items. They are skipped unless `QA_SWEEP=1` or `QA_REPORT_ONLY=1` is set (`QA_OFFLINE=1` also aborts every off-origin request).
