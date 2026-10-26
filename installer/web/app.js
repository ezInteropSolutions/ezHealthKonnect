// ezHealthKonnect Setup — web UI for the local installer process.
// Every API call carries the per-run token the installer put in the URL.
'use strict';

const TOKEN = (() => {
  const p = new URLSearchParams(location.search);
  const t = p.get('t') || sessionStorage.getItem('ezhk-setup-token') || '';
  if (p.get('t')) {
    sessionStorage.setItem('ezhk-setup-token', t);
    const action = p.get('action');
    history.replaceState(null, '', action ? '/?action=' + encodeURIComponent(action) : '/');
  }
  return t;
})();
const START_ACTION = new URLSearchParams(location.search).get('action');

const state = {
  step: 0,
  checksOK: false,
  platform: '',
  pgToInstall: 17,
  cfg: {},
  installed: null,
  browseTarget: '',
  browsePath: '',
};

// ── helpers ───────────────────────────────────────────────────────────────────
function esc(s) {
  return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}
async function api(path, body) {
  const opts = { headers: { 'X-Installer-Token': TOKEN } };
  if (body !== undefined) {
    opts.method = 'POST';
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (!res.ok && res.status !== 202) {
    throw new Error((await res.text()).trim() || ('HTTP ' + res.status));
  }
  const ct = res.headers.get('Content-Type') || '';
  return ct.includes('application/json') ? res.json() : null;
}
const $ = (id) => document.getElementById(id);
const val = (id) => { const el = $(id); return el ? el.value.trim() : ''; };
function setContent(html) { $('content').innerHTML = html; }
function setHeader(title, desc) { $('pageTitle').textContent = title; $('pageDesc').textContent = desc || ''; }
function showFooter(show) { $('footer').style.display = show ? '' : 'none'; }

// ── wizard navigation ─────────────────────────────────────────────────────────
const STEPS = [
  ['Welcome to ezHealthKonnect', 'Healthcare integration engine — HL7, FHIR, CDA, EDI X12, NCPDP and medical devices'],
  ['Checking Prerequisites', 'Verifying this server before anything is changed'],
  ['Configuration', 'Install location, ports and database'],
  ['Installing', 'This takes a few minutes'],
  ['Installation Complete', 'ezHealthKonnect is running as a service'],
];

function setStep(n) {
  state.step = n;
  document.querySelectorAll('.step-item').forEach((el, i) => {
    el.classList.toggle('active', i === n);
    el.classList.toggle('done', i < n);
  });
  setHeader(STEPS[n][0], STEPS[n][1]);
  if (n === 0) renderWelcome();
  if (n === 1) renderChecks();
  if (n === 2) renderConfig();
  updateNav();
}

function updateNav() {
  const back = $('btnBack'), next = $('btnNext');
  back.style.display = (state.step > 0 && state.step < 3) ? '' : 'none';
  next.style.display = state.step < 3 ? '' : 'none';
  next.disabled = state.step === 1 && !state.checksOK;
  next.textContent = ['Get Started →', 'Next →', 'Install Now'][state.step] || '';
  next.className = state.step === 2 ? 'btn btn-pink' : 'btn btn-primary';
}

$('btnNext').addEventListener('click', () => {
  if (state.step === 0) setStep(1);
  else if (state.step === 1) setStep(2);
  else if (state.step === 2) startInstall();
});
$('btnBack').addEventListener('click', () => { if (state.step > 0 && state.step < 3) setStep(state.step - 1); });

// ── screens ───────────────────────────────────────────────────────────────────
function renderWelcome() {
  setContent(`
  <div class="welcome-hero">
    <img src="/logo.jpeg" alt="ezHealthKonnect" class="welcome-logo-big" style="border-radius:16px;object-fit:contain;background:#fff;">
    <div class="brand">ezHealth<span>Konnect</span></div>
    <div class="tagline">Healthcare integration engine</div>
    <div class="feature-grid">
      <div class="feature-card"><div class="icon">🔀</div><strong>HL7 v2 · FHIR · CDA</strong><span>Parse, transform and route clinical messages</span></div>
      <div class="feature-card"><div class="icon">💳</div><strong>EDI X12 · NCPDP</strong><span>Claims, eligibility, remittance, e-prescribing</span></div>
      <div class="feature-card"><div class="icon">🩺</div><strong>Medical devices</strong><span>HL7 over TCP or serial, ASTM, DICOM</span></div>
      <div class="feature-card"><div class="icon">🔐</div><strong>Secure by default</strong><span>Encrypted credentials, audit trail, roles</span></div>
    </div>
  </div>
  <div class="callout callout-info" style="margin-top:24px">
    <span>ℹ️</span>
    <span>Installs ezHealthKonnect as a ${esc(state.platform)} service, with PostgreSQL ${esc(state.pgToInstall)} unless you use an existing database. No Docker required.</span>
  </div>`);
}

function renderChecks() {
  setContent(`
  <div id="checkRows"><div class="check-row"><div class="spinner"></div><div class="check-name">Checking…</div></div></div>
  <div class="callout callout-warn" id="checksHint" style="display:none"><span>⚠️</span><span id="checksHintText"></span></div>
  <div style="margin-top:16px"><button class="btn btn-ghost" data-action="recheck">↻ Check again</button></div>`);
  runChecks();
}

async function runChecks() {
  state.checksOK = false;
  updateNav();
  let items;
  try {
    items = await api('/api/checks', { appPort: state.cfg.appPort || '3000', apiPort: state.cfg.apiPort || '8080' });
  } catch (e) {
    $('checkRows').innerHTML = `<div class="check-row"><div class="check-icon">❌</div><div class="check-name">${esc(e.message)}</div></div>`;
    return;
  }
  let allOK = true, warn = false;
  $('checkRows').innerHTML = items.map(c => {
    if (!c.ok) allOK = false;
    if (c.warning) warn = true;
    const icon = !c.ok ? '❌' : c.warning ? '⚠️' : '✅';
    const detail = !c.ok ? `<div class="check-error">${esc(c.error)}</div>`
      : c.warning ? `<div class="check-error" style="color:var(--yellow)">${esc(c.warning)}</div>`
      : `<div class="check-version">${esc(c.version || '')}</div>`;
    return `<div class="check-row"><div class="check-icon">${icon}</div><div class="check-name">${esc(c.name)}</div>${detail}</div>`;
  }).join('');
  const hint = $('checksHint');
  hint.style.display = (!allOK || warn) ? 'flex' : 'none';
  $('checksHintText').textContent = !allOK
    ? 'Some requirements are not met. Fix them, then check again.'
    : 'Warnings do not block installation — you can adjust ports in the next step.';
  state.checksOK = allOK;
  updateNav();
}

function renderConfig() {
  const c = state.cfg;
  const isWin = state.platform === 'Windows';
  setContent(`
  <div class="section-title">Install Location</div>
  <div class="form-group">
    <label for="installDir">Install Directory</label>
    <div class="dir-row">
      <input type="text" id="installDir" value="${esc(c.installDir)}">
      <button class="btn-icon" title="Browse…" data-action="browse" data-target="installDir">📁</button>
    </div>
    <div class="hint">Must be empty. Application files, configuration, logs, stored messages and upgrade backups live here.</div>
  </div>

  <div class="section-title">Ports</div>
  <div class="form-row">
    <div class="form-group"><label for="appPort">Web UI Port</label><input type="text" id="appPort" value="${esc(c.appPort)}"></div>
    <div class="form-group"><label for="apiPort">Engine API Port (internal)</label><input type="text" id="apiPort" value="${esc(c.apiPort)}"></div>
  </div>

  <div class="section-title">Database</div>
  <div class="db-source-toggle">
    <div class="db-source-card ${c.dbMode !== 'external' ? 'selected' : ''}" data-action="db-mode" data-mode="local">
      <span class="ds-icon">🖥️</span>
      <div><strong>PostgreSQL on this server</strong><br><span style="font-size:11px;color:var(--muted)">Installed or reused automatically; password generated</span></div>
    </div>
    <div class="db-source-card ${c.dbMode === 'external' ? 'selected' : ''}" data-action="db-mode" data-mode="external">
      <span class="ds-icon">☁️</span>
      <div><strong>Existing database</strong><br><span style="font-size:11px;color:var(--muted)">Managed / cloud PostgreSQL 15+, empty database</span></div>
    </div>
  </div>
  <div id="localDb" style="display:${c.dbMode === 'external' ? 'none' : 'block'}">
    ${isWin ? `<div class="form-group">
      <label for="pgAdminPassword">PostgreSQL superuser password <span style="font-weight:400;color:var(--muted)">(only if PostgreSQL is already installed on this server)</span></label>
      <input type="password" id="pgAdminPassword" autocomplete="off">
      <div class="hint">Leave empty to have PostgreSQL ${esc(state.pgToInstall)} installed. Used once to create the database; not stored.</div>
    </div>` : ''}
  </div>
  <div id="externalDb" style="display:${c.dbMode === 'external' ? 'block' : 'none'}">
    <div class="form-row">
      <div class="form-group"><label for="dbHost">Host</label><input type="text" id="dbHost" value="${esc(c.dbHost === 'localhost' ? '' : c.dbHost)}" placeholder="db.example.internal"></div>
      <div class="form-group"><label for="dbPort">Port</label><input type="text" id="dbPort" value="${esc(c.dbPort)}"></div>
    </div>
    <div class="form-row">
      <div class="form-group"><label for="dbName">Database name</label><input type="text" id="dbName" value="${esc(c.dbName)}"></div>
      <div class="form-group"><label for="dbUser">Application user</label><input type="text" id="dbUser" value="${esc(c.dbUser)}"></div>
    </div>
    <div class="form-group"><label for="dbSslMode">TLS</label>
      <select id="dbSslMode">
        <option value="require" ${c.dbSslMode === 'require' ? 'selected' : ''}>Required</option>
        <option value="verify-full" ${c.dbSslMode === 'verify-full' ? 'selected' : ''}>Required + verify certificate</option>
        <option value="disable" ${c.dbSslMode === 'disable' ? 'selected' : ''}>Off (this machine or a private network)</option>
      </select>
      <div class="hint">A PostgreSQL on this machine normally has TLS off; it is switched to Off for localhost.</div></div>

    <label class="checkbox-row" for="dbAuthAdmin">
      <input type="radio" name="dbAuth" id="dbAuthAdmin" value="admin" checked>
      <div class="cb-text"><strong>Create the database and user for me</strong>
        <span>Sign in as an administrator once; setup creates both, with a generated password for the user.</span></div>
    </label>
    <div id="dbAuthAdminFields" class="form-row">
      <div class="form-group"><label for="dbAdminUser">Administrator user</label><input type="text" id="dbAdminUser" value="postgres" autocomplete="off"></div>
      <div class="form-group"><label for="dbAdminPassword">Administrator password</label><input type="password" id="dbAdminPassword" autocomplete="off">
        <div class="hint">Used once, then forgotten. It is never saved.</div></div>
    </div>
    <label class="checkbox-row" for="dbAuthExisting">
      <input type="radio" name="dbAuth" id="dbAuthExisting" value="existing">
      <div class="cb-text"><strong>They already exist</strong>
        <span>The database must exist, be empty and be owned by the user above.</span></div>
    </label>
    <div id="dbAuthExistingFields" class="form-group" style="display:none">
      <label for="dbPassword">Password of the application user</label><input type="password" id="dbPassword" autocomplete="off">
    </div>
  </div>

  <div class="section-title">Options</div>
  <label class="checkbox-row" for="chkAI">
    <input type="checkbox" id="chkAI" ${c.withAI ? 'checked' : ''}>
    <div class="cb-text"><strong>Enable ezCompanion (AI assistant)</strong>
      <span>Requires Ollama on this server with llama3.2:3b and nomic-embed-text (~2.3 GB). Can be enabled later.</span></div>
  </label>
  <div id="configError" class="field-error"></div>`);
}

function collectConfig() {
  const mode = document.querySelector('.db-source-card.selected')?.dataset.mode || 'local';
  const c = {
    installDir: val('installDir'), appPort: val('appPort'), apiPort: val('apiPort'),
    dbMode: mode, withAI: $('chkAI').checked,
    dbPort: val('dbPort') || '5432', dbName: val('dbName') || 'ezhealthkonnect', dbUser: val('dbUser') || 'ezhealth_user',
  };
  if (mode === 'external') {
    c.dbHost = val('dbHost'); c.dbSslMode = val('dbSslMode');
    if (document.querySelector('input[name="dbAuth"]:checked')?.value === 'existing') {
      c.dbPassword = $('dbPassword').value;
    } else {
      c.dbAdminUser = val('dbAdminUser') || 'postgres'; c.dbAdminPassword = $('dbAdminPassword').value;
    }
  } else if ($('pgAdminPassword')) {
    c.pgAdminPassword = $('pgAdminPassword').value;
  }
  return c;
}

function validateConfig(c) {
  const port = (p) => /^\d+$/.test(p) && +p > 0 && +p < 65536;
  if (!c.installDir) return 'Choose an install directory.';
  if (!port(c.appPort) || !port(c.apiPort)) return 'Ports must be numbers between 1 and 65535.';
  if (c.appPort === c.apiPort) return 'The web port and the engine port must differ.';
  if (c.dbMode === 'external') {
    if (!c.dbHost) return 'Enter the database host.';
    if (c.dbAdminPassword !== undefined && !c.dbAdminPassword) return 'Enter the administrator password.';
    if (c.dbAdminPassword === undefined && !c.dbPassword) return 'Enter the password of the database user.';
  }
  return '';
}

// ── progress (install / upgrade / rollback / uninstall) ──────────────────────
function renderProgress(title) {
  setContent(`
  <div class="progress-label"><span id="progressStep">${esc(title)}</span><span id="progressPct">0%</span></div>
  <div class="progress-wrap"><div class="progress-bar" id="progressBar"></div></div>
  <div class="log" id="log"></div>`);
}
let progress = 0;
function advance(target, label) {
  progress = target != null ? target : Math.min(94, progress + Math.max(0.5, (95 - progress) * 0.06));
  if ($('progressBar')) $('progressBar').style.width = Math.round(progress) + '%';
  if ($('progressPct')) $('progressPct').textContent = Math.round(progress) + '%';
  if (label && $('progressStep')) $('progressStep').textContent = label;
}
function appendLog(level, msg) {
  const log = $('log');
  if (!log) return;
  const div = document.createElement('div');
  div.className = 'log-line-' + (level === 'step' ? 'info' : level);
  div.textContent = msg;
  if (level === 'step') div.style.fontWeight = '700';
  log.appendChild(div);
  while (log.childNodes.length > 600) log.removeChild(log.firstChild);
  log.scrollTop = log.scrollHeight;
}

// run starts an operation and resolves with its result (or rejects). The
// operation is started BEFORE subscribing: the server resets its event history
// when an operation starts and replays it to every subscriber, so nothing is
// missed — and a stale "done" from an earlier operation is never seen.
async function run(path, body, title) {
  progress = 0;
  renderProgress(title);
  await api(path, body);
  return new Promise((resolve, reject) => {
    const es = new EventSource('/api/progress?t=' + encodeURIComponent(TOKEN));
    es.onmessage = (e) => {
      const m = JSON.parse(e.data);
      if (m.level === 'done') { es.close(); advance(100, 'Complete'); resolve(m.msg ? JSON.parse(m.msg) : {}); return; }
      if (m.level === 'failed') {
        es.close(); advance(100, 'Failed');
        $('progressBar').style.background = 'var(--red)';
        reject(new Error(m.msg)); return;
      }
      appendLog(m.level, m.msg);
      advance(null, m.level === 'step' ? m.msg : null);
    };
    es.onerror = () => appendLog('warn', 'Lost connection to the installer — it may still be working. Do not close this window.');
  });
}

function showFailure(err, retryLabel, retry) {
  appendLog('error', '');
  appendLog('error', err.message);
  const div = document.createElement('div');
  div.style.marginTop = '16px';
  div.innerHTML = `<div class="callout callout-warn"><span>⚠️</span><span>${esc(err.message)}</span></div>`;
  if (retry) {
    const b = document.createElement('button');
    b.className = 'btn btn-ghost';
    b.textContent = retryLabel;
    b.addEventListener('click', retry);
    div.appendChild(b);
  }
  $('content').appendChild(div);
}

async function startInstall() {
  const c = collectConfig();
  const problem = validateConfig(c);
  if (problem) { $('configError').textContent = problem; return; }
  state.cfg = Object.assign(state.cfg, c);
  state.step = 3;
  document.querySelectorAll('.step-item').forEach((el, i) => { el.classList.toggle('active', i === 3); el.classList.toggle('done', i < 3); });
  setHeader(STEPS[3][0], STEPS[3][1]);
  updateNav();
  try {
    const res = await run('/api/install', c, 'Starting installation…');
    renderInstallDone(res);
  } catch (err) {
    showFailure(err, '← Back to Configuration', () => setStep(2));
  }
}

function renderInstallDone(res) {
  state.step = 4;
  document.querySelectorAll('.step-item').forEach(el => el.classList.add('done'));
  setHeader(STEPS[4][0], STEPS[4][1]);
  updateNav();
  setContent(`
  <div class="success-card">
    <div class="success-icon">🎉</div>
    <h2>ezHealthKonnect ${esc(res.version)} is installed</h2>
    <p>Next, create the first administrator account. This one-time link works only until an administrator exists — keep it private.</p>
    <div class="secret-box" id="setupUrl">${esc(res.setupUrl)}</div>
    <div class="done-actions">
      <button class="btn btn-success" data-action="open" data-url="${esc(res.setupUrl)}">Create administrator</button>
      <button class="btn btn-ghost" data-action="copy" data-url="${esc(res.setupUrl)}">Copy link</button>
      <button class="btn btn-ghost" data-action="close">Close setup</button>
    </div>
    <p class="note">Web UI: ${esc(res.appUrl)} — to reach it from other computers, allow port ${esc(state.cfg.appPort)} in the firewall (preferably behind HTTPS).</p>
  </div>`);
}

// ── installed: manage screen ──────────────────────────────────────────────────
function renderManage(st) {
  state.installed = st;
  $('stepList').style.display = 'none';
  showFooter(false);
  setHeader('ezHealthKonnect is installed', st.installDir);
  const appURL = `http://localhost:${st.appPort || '3000'}`;
  let upgradeBtn = '';
  if (st.upgradeAction === 'upgrade') {
    upgradeBtn = `<button class="btn btn-pink" data-action="upgrade">⬆ Upgrade ${esc(st.version)} → ${esc(st.bundleVersion)}</button>`;
  } else if (st.upgradeAction === 'repair') {
    upgradeBtn = `<button class="btn btn-ghost" data-action="upgrade">🔧 Reinstall ${esc(st.version)} (repair)</button>`;
  } else if (st.upgradeAction === 'older') {
    upgradeBtn = `<div class="callout callout-warn"><span>⚠️</span><span>This setup contains ${esc(st.bundleVersion)}, older than the installed ${esc(st.version)}. Downgrades are not supported.</span></div>`;
  }
  setContent(`
  <div style="max-width:560px;margin:0 auto;padding-top:8px">
    <div class="manage-grid">
      <div><div class="k">Installed version</div>${esc(st.version)}</div>
      <div><div class="k">Status</div>${st.serviceRunning ? '<span style="color:var(--green)">● Running</span>' : '<span style="color:var(--yellow)">● Not responding</span>'}</div>
      <div><div class="k">Install directory</div><span style="font-family:monospace">${esc(st.installDir)}</span></div>
      <div><div class="k">Web UI</div>${esc(appURL)}</div>
    </div>
    <div class="manage-actions">
      <button class="btn btn-success" data-action="open" data-url="${esc(appURL)}">🚀 Open ezHealthKonnect</button>
      ${upgradeBtn}
      ${st.rollbackTo ? `<button class="btn btn-ghost" data-action="rollback">↩ Roll back to ${esc(st.rollbackTo)}</button>` : ''}
      <button class="btn btn-danger" data-action="uninstall">🗑 Uninstall</button>
    </div>
    <p class="note">Upgrades back up the database, configuration and application first, and restore them automatically if anything fails.</p>
  </div>`);
}

async function doUpgrade() {
  const st = state.installed;
  if (!confirm(`Upgrade ezHealthKonnect ${st.version} → ${st.bundleVersion}?\n\n` +
    '• The service stops for a few minutes — message senders should retry (most do automatically).\n' +
    '• The database, configuration and application are backed up first.\n' +
    '• If anything fails, the previous version is restored automatically.')) return;
  setHeader('Upgrading', st.installDir);
  try {
    const res = await run('/api/upgrade', { installDir: st.installDir }, 'Upgrading…');
    setContent(`<div class="success-card"><div class="success-icon">✅</div><h2>Upgraded to ${esc(res.version)}</h2>
      <p>Your data and configuration were kept. A backup of the previous version is in ${esc(st.installDir)}${st.installDir.includes('\\') ? '\\' : '/'}backups.</p>
      <div class="done-actions"><button class="btn btn-success" data-action="open" data-url="${esc(res.appUrl)}">Open ezHealthKonnect</button>
      <button class="btn btn-ghost" data-action="close">Close setup</button></div></div>`);
  } catch (err) {
    showFailure(err, 'Back', () => boot());
  }
}

async function doRollback() {
  const st = state.installed;
  if (!confirm(`Roll back to ${st.rollbackTo}?\n\nThe database is restored to how it was just before the upgrade to ${st.version}.\n` +
    'Messages and changes made since that upgrade will be LOST.\n\nOnly continue if the new version cannot be used.')) return;
  setHeader('Rolling back', st.installDir);
  try {
    await run('/api/rollback', { installDir: st.installDir, confirm: true }, 'Rolling back…');
    setContent(`<div class="success-card"><div class="success-icon">↩</div><h2>Rolled back to ${esc(st.rollbackTo)}</h2>
      <div class="done-actions"><button class="btn btn-ghost" data-action="close">Close setup</button></div></div>`);
  } catch (err) {
    showFailure(err, 'Back', () => boot());
  }
}

async function doUninstall() {
  const st = state.installed;
  const dir = st ? st.installDir : '';
  if (!confirm(`Uninstall ezHealthKonnect?\n\n• The service is stopped and removed\n• Everything in ${dir} is deleted, including stored messages and backups\n` +
    (st && st.removesDatabase
      ? '• The database that setup created is dropped, with all its data\n'
      : '• The database was provided by you and is kept\n') +
    '\nThis cannot be undone.')) return;
  setHeader('Uninstalling', dir);
  try {
    await run('/api/uninstall', { installDir: dir, confirm: true }, 'Uninstalling…');
    setContent(`<div class="success-card"><div class="success-icon">🗑</div><h2>Uninstalled</h2>
      <p style="color:var(--muted)">ezHealthKonnect has been removed. You may close this window.</p>
      <div class="done-actions"><button class="btn btn-ghost" data-action="close">Close</button></div></div>`);
  } catch (err) {
    showFailure(err, 'Back', () => boot());
  }
}

// ── folder browser ────────────────────────────────────────────────────────────
async function loadBrowse(path) {
  state.browsePath = path;
  $('browsePath').textContent = path;
  $('browseSelected').textContent = path;
  $('browseBody').innerHTML = '<div style="padding:24px;text-align:center"><div class="spinner" style="margin:0 auto"></div></div>';
  let data;
  try { data = await api('/api/browse?path=' + encodeURIComponent(path)); } catch (e) { $('browseBody').textContent = e.message; return; }
  state.browsePath = data.path;
  $('browsePath').textContent = data.path;
  $('browseSelected').textContent = data.path;
  const rows = [];
  if (data.parent) rows.push({ path: data.parent, label: '.. (up)', up: true });
  for (const d of data.dirs) rows.push({ path: d.path, label: d.name });
  const body = $('browseBody');
  body.innerHTML = '';
  if (!rows.length) { body.innerHTML = '<div style="padding:20px;text-align:center;color:var(--muted)">No subdirectories</div>'; return; }
  for (const r of rows) {
    const div = document.createElement('div');
    div.className = 'dir-entry' + (r.up ? ' up' : '');
    div.textContent = (r.up ? '⬆ ' : '📁 ') + r.label;
    div.addEventListener('click', () => loadBrowse(r.path)); // path never interpolated into code
    body.appendChild(div);
  }
}

// ── event delegation ──────────────────────────────────────────────────────────
// Existing server: show the fields for the chosen way of signing in.
document.addEventListener('change', (e) => {
  if (e.target && e.target.name === 'dbAuth') {
    const existing = e.target.value === 'existing';
    $('dbAuthAdminFields').style.display = existing ? 'none' : '';
    $('dbAuthExistingFields').style.display = existing ? '' : 'none';
  }
});
// A PostgreSQL on this machine normally has TLS off, and the default
// "required" would fail against it: follow the host unless the person chose.
document.addEventListener('input', (e) => {
  if (e.target && e.target.id === 'dbHost' && $('dbSslMode') && !$('dbSslMode').dataset.chosen) {
    const local = /^(localhost|127\.0\.0\.1|::1)$/i.test(e.target.value.trim());
    $('dbSslMode').value = local ? 'disable' : 'require';
  }
});
document.addEventListener('change', (e) => {
  if (e.target && e.target.id === 'dbSslMode') e.target.dataset.chosen = '1';
});

document.addEventListener('click', async (e) => {
  const el = e.target.closest('[data-action]');
  if (!el) return;
  const a = el.dataset.action;
  if (a === 'recheck') runChecks();
  if (a === 'db-mode') {
    document.querySelectorAll('.db-source-card').forEach(c => c.classList.toggle('selected', c === el));
    $('localDb').style.display = el.dataset.mode === 'external' ? 'none' : 'block';
    $('externalDb').style.display = el.dataset.mode === 'external' ? 'block' : 'none';
  }
  if (a === 'browse') {
    state.browseTarget = el.dataset.target;
    $('browseModal').style.display = 'flex';
    loadBrowse(val(state.browseTarget) || '');
  }
  if (a === 'browse-cancel') $('browseModal').style.display = 'none';
  if (a === 'browse-select') { $(state.browseTarget).value = state.browsePath; $('browseModal').style.display = 'none'; }
  if (a === 'open') window.open(el.dataset.url, '_blank', 'noopener');
  if (a === 'copy') { try { await navigator.clipboard.writeText(el.dataset.url); el.textContent = 'Copied'; } catch { /* clipboard unavailable */ } }
  if (a === 'upgrade') doUpgrade();
  if (a === 'rollback') doRollback();
  if (a === 'uninstall') doUninstall();
  if (a === 'close') { try { await api('/api/close', {}); } catch { /* already closed */ } window.close(); setContent('<p style="text-align:center;padding:60px;color:var(--muted)">Setup has finished. You can close this window.</p>'); }
});
$('browseModal').addEventListener('click', (e) => { if (e.target === $('browseModal')) $('browseModal').style.display = 'none'; });

// ── boot ──────────────────────────────────────────────────────────────────────
async function boot() {
  if (!TOKEN) {
    setHeader('ezHealthKonnect Setup', '');
    setContent('<div class="callout callout-warn"><span>⚠️</span><span>Open setup using the address printed by the installer (it contains a one-time access key).</span></div>');
    showFooter(false);
    return;
  }
  try {
    const d = await api('/api/defaults');
    state.platform = d.platform;
    state.pgToInstall = d.postgresToInstall;
    state.cfg = d.config;
    const st = await api('/api/state');
    if (st.installed) {
      renderManage(st);
      if (START_ACTION === 'uninstall') doUninstall();
      return;
    }
  } catch (e) {
    setContent(`<div class="callout callout-warn"><span>⚠️</span><span>${esc(e.message)}</span></div>`);
    return;
  }
  $('stepList').style.display = '';
  showFooter(true);
  setStep(0);
}

// Heartbeat: setup exits a couple of minutes after this window is closed.
setInterval(() => { if (TOKEN) api('/api/ping').catch(() => { /* setup already closed */ }); }, 15000);
boot();
