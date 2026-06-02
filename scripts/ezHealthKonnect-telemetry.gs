// ezHealthKonnect — Telemetry Receiver
// Receives install pings and feedback submissions from all deployed instances.
// Source IP is captured automatically from the HTTP connection (no code needed).
//
// Deployed at:
//   https://script.google.com/macros/s/AKfycbzh4wdZHEi2Wg2rc3wEnb08Lcr83tVj5amaxq43gQdwLmFctlKj9AxTsF_mp4azIVMZ/exec
//
// Sheet ID: 1V14aMyhuHK7IbMtf7bMchCNvzsI-Zu4WUdYq3Eu8DIE
// Sheet tabs: "installs" (install_ping) | "feedback" (feedback_submit)
//
// To redeploy after changes: Deploy → Manage deployments → edit → new version.
// The URL stays the same — no need to update telemetryEndpoint in telemetry_service.go.

const HMAC_SECRET = "ehk-t3lem-v1-xK9mPqR7nW2sB4dL";  // must match telemetry_service.go
const SHEET_ID    = "1V14aMyhuHK7IbMtf7bMchCNvzsI-Zu4WUdYq3Eu8DIE";

function doPost(e) {
  try {
    const data = JSON.parse(e.postData.contents);

    // Verify HMAC signature — rejects anything not from a real ezHK install
    if (!verifySignature(data)) {
      return respond(false, "unauthorized");
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
    data.install_id      || "",
    data.instance_name   || "",
    data.product_version || "",
    data.edition         || "community",
    data.os              || "",
    data.arch            || "",
    data.admin_email     || "",
    data.public_ip       || "",
    data.timezone        || "",
    data.registered_at   || "",
  ]);
}

function appendFeedback(ss, data) {
  const sheet = getOrCreateSheet(ss, "feedback", [
    "Timestamp", "Install ID", "Version", "Edition",
    "Period Days", "Total", "Positive %", "KB Ingested", "Admin Comment"
  ]);
  sheet.appendRow([
    new Date(),
    data.install_id      || "",
    data.product_version || "",
    data.edition         || "community",
    data.period_days     || 0,
    data.total           || 0,
    data.positive_pct    || 0,
    data.kb_ingested     || 0,
    data.admin_comment   || "",
  ]);
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function verifySignature(data) {
  if (!data.sig || data.sig.length < 8) return false;

  let input = "";
  if (data.event_type === "install_ping") {
    input = (data.install_id || "") + (data.product_version || "");
  } else if (data.event_type === "feedback_submit") {
    input = (data.install_id || "") + (data.product_version || "") + String(data.total || 0);
  }

  const key      = Utilities.newBlob(HMAC_SECRET).getBytes();
  const message  = Utilities.newBlob(input).getBytes();
  const sigBytes = Utilities.computeHmacSha256Signature(message, key);
  const expected = sigBytes.map(b => ('0' + (b & 0xFF).toString(16)).slice(-2)).join('').slice(0, 16);

  return data.sig === expected;
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
