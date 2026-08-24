// ezHealthKonnect — Telemetry Receiver
// Receives install pings and feedback submissions from all deployed instances.
// Source IP is self-reported by the client (services/telemetry_service.go's
// fetchPublicIP), NOT independently observed by Apps Script — doPost(e) has
// no access to the caller's real IP, so treat public_ip as unverified.
//
// Deployed at:
//   https://script.google.com/macros/s/AKfycbzh4wdZHEi2Wg2rc3wEnb08Lcr83tVj5amaxq43gQdwLmFctlKj9AxTsF_mp4azIVMZ/exec
//
// Sheet ID: 1V14aMyhuHK7IbMtf7bMchCNvzsI-Zu4WUdYq3Eu8DIE
// Sheet tabs: "installs" (install_ping) | "feedback" (feedback_submit)
//
// To redeploy after changes: Deploy → Manage deployments → edit → new version.
// The URL stays the same — no need to update telemetryEndpoint in telemetry_service.go.
//
// SECURITY MODEL: HMAC_SECRET is a single shared secret compiled into every
// ezHealthKonnect install (see main.go's SeedIfEmpty fallback for community
// builds) — anyone who has the product already has this value. It is NOT a
// per-user credential and does not authenticate individual installs against
// each other; its only job is filtering out random internet noise hitting
// this public webhook. Treat it as an anti-spam token, not an auth boundary
// — don't over-invest in rotation/per-install-keying without a reason beyond
// "it's technically possible." The things that actually matter for a public,
// low-signature-coverage endpoint like this: sanitizing untrusted input
// before it reaches the spreadsheet (see sanitizeText_/sanitizeNumber_ below
// — most fields aren't even covered by the signature, see verifySignature),
// and basic volume throttling (see checkRateLimit_).
//
// SETUP: HMAC_SECRET lives in this script's Script Properties, not in source
// (must match whatever value is seeded into product_config.telemetry_secret
// for official installer builds — see services/telemetry_service.go). Set it
// once from the Apps Script editor: Project Settings (gear icon) → Script
// Properties → Add script property → key "HMAC_SECRET". Alternatively, paste
// the value into setHmacSecret_() below, run it once from the editor's Run
// menu, then delete the pasted value so it isn't left sitting in source.

const SHEET_ID = "1V14aMyhuHK7IbMtf7bMchCNvzsI-Zu4WUdYq3Eu8DIE";

// Max submissions per install_id per event type per window. CacheService's
// TTL caps out at 6 hours, so this bounds the window itself rather than
// pretending to enforce a longer one. This only stops a single install_id
// from flooding (e.g. a retry-loop bug) — an attacker who already has the
// shared secret can still rotate install_ids to bypass it; see the security
// model note above for why that's an accepted limitation here, not a gap
// worth closing with more infrastructure.
const RATE_LIMIT_WINDOW_SECONDS = 3600;
const RATE_LIMIT_MAX_PER_WINDOW = 5;

function getHmacSecret_() {
  const secret = PropertiesService.getScriptProperties().getProperty("HMAC_SECRET");
  if (!secret) {
    throw new Error("HMAC_SECRET is not set — see the SETUP note at the top of this file.");
  }
  return secret;
}

// One-time setup helper — run once from the Apps Script editor's Run menu
// with the real secret pasted in, then remove the pasted value again.
function setHmacSecret_(secret) {
  PropertiesService.getScriptProperties().setProperty("HMAC_SECRET", secret);
}

function doPost(e) {
  try {
    const data = JSON.parse(e.postData.contents);

    // Verify HMAC signature — rejects anything not from a real ezHK install
    if (!verifySignature(data)) {
      return respond(false, "unauthorized");
    }

    // Rate limit AFTER signature verification, not before — an attacker
    // without the secret can't reach this far at all, and can't burn
    // another install_id's rate-limit budget by spoofing it without also
    // knowing the secret needed to pass verifySignature above.
    if (!checkRateLimit_(data.install_id, data.event_type)) {
      return respond(false, "rate limited");
    }

    const ss = SpreadsheetApp.openById(SHEET_ID);

    if (data.event_type === "install_ping") {
      appendInstall(ss, data);
    } else if (data.event_type === "feedback_submit") {
      appendFeedback(ss, data);
    } else {
      return respond(false, "unknown event_type");
    }

    return respond(true, "ok");
  } catch (err) {
    return respond(false, err.toString());
  }
}

function appendInstall(ss, data) {
  const sheet = getOrCreateSheet(ss, "installs", [
    "Timestamp", "Install ID", "Instance Name", "Version", "Edition",
    "OS", "Arch", "Admin Email", "Public IP", "Timezone", "Registered At"
  ]);
  sheet.appendRow([
    new Date(),
    sanitizeText_(data.install_id),
    sanitizeText_(data.instance_name),
    sanitizeText_(data.product_version),
    sanitizeText_(data.edition || "community"),
    sanitizeText_(data.os),
    sanitizeText_(data.arch),
    sanitizeText_(data.admin_email),
    sanitizeText_(data.public_ip),
    sanitizeText_(data.timezone),
    sanitizeText_(data.registered_at),
  ]);
}

function appendFeedback(ss, data) {
  const sheet = getOrCreateSheet(ss, "feedback", [
    "Timestamp", "Install ID", "Version", "Edition",
    "Period Days", "Total", "Positive %", "KB Ingested", "Admin Comment"
  ]);
  sheet.appendRow([
    new Date(),
    sanitizeText_(data.install_id),
    sanitizeText_(data.product_version),
    sanitizeText_(data.edition || "community"),
    sanitizeNumber_(data.period_days),
    sanitizeNumber_(data.total),
    sanitizeNumber_(data.positive_pct),
    sanitizeNumber_(data.kb_ingested),
    sanitizeText_(data.admin_comment),
  ]);
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// verifySignature only covers install_id + product_version (+ total for
// feedback_submit) — every OTHER field written to the sheet (instance_name,
// admin_email, os, arch, admin_comment, etc.) is unsigned and fully
// attacker-controlled content-wise even on an otherwise-valid, correctly-
// signed request. sanitizeText_/sanitizeNumber_ below are what actually
// protects the sheet, not the signature.
function verifySignature(data) {
  if (!data.sig || data.sig.length < 8) return false;

  let input = "";
  if (data.event_type === "install_ping") {
    input = (data.install_id || "") + (data.product_version || "");
  } else if (data.event_type === "feedback_submit") {
    input = (data.install_id || "") + (data.product_version || "") + String(data.total || 0);
  }

  const key      = Utilities.newBlob(getHmacSecret_()).getBytes();
  const message  = Utilities.newBlob(input).getBytes();
  const sigBytes = Utilities.computeHmacSha256Signature(message, key);
  const expected = sigBytes.map(b => ('0' + (b & 0xFF).toString(16)).slice(-2)).join('').slice(0, 16);

  return data.sig === expected;
}

// checkRateLimit_ returns false once a given (install_id, event_type) pair
// has been seen RATE_LIMIT_MAX_PER_WINDOW times within the current window.
// No install_id at all is let through uncounted — there's nothing to key a
// per-install throttle on, and the signature check already gates entry.
function checkRateLimit_(installId, eventType) {
  if (!installId) return true;

  const cache = CacheService.getScriptCache();
  const key = "rl_" + eventType + "_" + installId;
  const count = parseInt(cache.get(key), 10) || 0;

  if (count >= RATE_LIMIT_MAX_PER_WINDOW) {
    return false;
  }
  cache.put(key, String(count + 1), RATE_LIMIT_WINDOW_SECONDS);
  return true;
}

// Prevents CSV/spreadsheet formula injection: Sheets treats a cell value
// starting with =, +, -, or @ as a live formula when written via setValue/
// appendRow — same as if a human had typed it into the UI. A leading single
// quote forces Sheets to treat the rest as plain text instead (the quote
// itself is a parsing instruction, not stored/displayed content) — this is
// the standard mitigation for this class of injection in Sheets/Excel/CSV
// consumers generally.
function sanitizeText_(value) {
  const str = value == null ? "" : String(value);
  return /^[=+\-@]/.test(str) ? "'" + str : str;
}

// Fields that are supposed to be numbers must be coerced to actual JS
// numbers, not just falsy-checked (`data.total || 0`) — a payload could
// smuggle a STRING like "=1+1" into a JSON field the schema expects to be
// numeric, and an unconverted string value would still be written as live
// text (and therefore parsed as a formula) rather than a number, since
// nothing here enforces the incoming JSON's field types.
function sanitizeNumber_(value) {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
}

function getOrCreateSheet(ss, name, headers) {
  let sheet = ss.getSheetByName(name);
  if (!sheet) {
    sheet = ss.insertSheet(name);
    sheet.appendRow(headers);
    sheet.getRange(1, 1, 1, headers.length).setFontWeight("bold");
    sheet.setFrozenRows(1);
  }
  return sheet;
}

function respond(success, message) {
  return ContentService
    .createTextOutput(JSON.stringify({ success: success, message: message }))
    .setMimeType(ContentService.MimeType.JSON);
}
