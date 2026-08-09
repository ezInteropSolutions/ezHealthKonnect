'use strict';

var _interfaceId   = new URLSearchParams(location.search).get('interfaceId');
var _interfaceData = null;
var _tabLoaded     = {};

// ── Tab routing ────────────────────────────────────────────────────────────────
function activateTab(name) {
    document.querySelectorAll('.leftnav-tab').forEach(function(el) {
        el.classList.toggle('active', el.dataset.tab === name);
    });
    document.querySelectorAll('.tab-section').forEach(function(el) {
        el.classList.toggle('active', el.id === 'tab-' + name);
    });
    location.hash = name;
    if (!_tabLoaded[name]) {
        _tabLoaded[name] = true;
        if      (name === 'overview')  loadOverview();
        else if (name === 'pipeline')  loadPipelineTab();
        else if (name === 'messages')  loadMessagesTab();
        else if (name === 'alerts')    loadAlertsTab();
    }
}

// ── API helpers ────────────────────────────────────────────────────────────────
function apiGet(path) {
    return fetch(path, { credentials: 'include' }).then(function(r) { return r.json(); });
}

function apiPut(path, body) {
    return fetch(path, {
        method: 'PUT', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    }).then(function(r) { return r.json(); });
}

function showToast(msg, ok) {
    var t = document.createElement('div');
    t.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:9999;padding:10px 18px;border-radius:8px;font-size:13px;font-weight:600;color:#fff;background:' + (ok ? '#16a34a' : '#dc2626');
    t.textContent = msg;
    document.body.appendChild(t);
    setTimeout(function() { t.remove(); }, 3500);
}

function esc(s) {
    return String(s || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

// YYYY-MM-DD using LOCAL date parts (not toISOString(), which is UTC-based
// and can roll the date to the next/previous day for users off UTC+0) —
// mirrors formatDateYMD in public/js/interfaces.js.
function formatDateYMD(date) {
    var y = date.getFullYear();
    var m = String(date.getMonth() + 1).padStart(2, '0');
    var d = String(date.getDate()).padStart(2, '0');
    return y + '-' + m + '-' + d;
}

// Same date, plus local clock time — mirrors formatDateTimeYMD in interfaces.js.
function formatDateTimeYMD(date) {
    var time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    return formatDateYMD(date) + ' ' + time;
}

function copyInterfaceId(id, btn) {
    navigator.clipboard.writeText(id).then(function() {
        var orig = btn.innerHTML;
        btn.innerHTML = '<i class="fas fa-check"></i>';
        setTimeout(function() { btn.innerHTML = orig; }, 1500);
    });
}

// ── Load interface header info ─────────────────────────────────────────────────
function loadInterfaceHeader() {
    if (!_interfaceId) {
        document.getElementById('interfaceName').textContent = 'Interface not found';
        return;
    }
    apiGet('/api/interfaces/' + _interfaceId)
        .then(function(res) {
            var iface = res.data || res.interface || res;
            if (!iface || !iface.id) return;
            _interfaceData = iface;

            document.getElementById('interfaceName').textContent = iface.name || 'Unnamed Interface';
            document.getElementById('interfaceDesc').textContent = iface.description || '';
            document.title = (iface.name || 'Interface') + ' – ezHealthKonnect';

            var createdText = iface.createdAt ? formatDateTimeYMD(new Date(iface.createdAt)) : 'Unknown';
            var updatedText = iface.updatedAt ? formatDateTimeYMD(new Date(iface.updatedAt)) : 'Unknown';
            document.getElementById('interfaceMeta').innerHTML =
                '<span>ID: <code>' + esc(iface.id) + '</code> ' +
                '<button class="copy-id-btn" onclick="copyInterfaceId(\'' + esc(iface.id) + '\', this)" title="Copy interface ID">' +
                '<i class="fas fa-copy"></i></button></span>' +
                '<span>Created: ' + createdText + '</span>' +
                '<span>Updated: ' + updatedText + '</span>';

            var pill = document.getElementById('statusPill');
            // Runtime status (is it actually running right now) takes priority over
            // interface_status (user intent) — otherwise a halted interface that's
            // still "meant" to be active would show ACTIVE instead of the real ERROR.
            var status = (iface.status || iface.interface_status || 'draft').toLowerCase();
            pill.textContent = status.toUpperCase();
            pill.className   = 'status-pill ' + status;
            pill.style.display = '';

            var err = status === 'error' && iface.processingStats && iface.processingStats.error;
            pill.title = err ? (err.reason || '') : '';
            renderErrorBanner(err || null);

            var pipelineLink = document.getElementById('pipelineLink');
            pipelineLink.href = 'pipeline-builder.html?interfaceId=' + _interfaceId;
            pipelineLink.style.display = '';

            document.getElementById('openPipelineBtn').href = 'pipeline-builder.html?interfaceId=' + _interfaceId;
            document.getElementById('openMessagesBtn').href = 'messages.html?interfaceId=' + _interfaceId;

            loadDLQDepthBadge();
        })
        .catch(function() {
            document.getElementById('interfaceName').textContent = 'Interface not found';
        });
}

// Surfaces the reason a halted interface actually shows ERROR (port/directory
// conflict, HIPAA halt, etc.) — previously invisible anywhere on this page once
// you clicked through from the interfaces list.
function renderErrorBanner(err) {
    var el = document.getElementById('interfaceErrorBanner');
    if (!el) return;
    if (!err) { el.style.display = 'none'; el.innerHTML = ''; return; }

    var ts = err.timestamp ? new Date(err.timestamp).toLocaleString() : '';
    var detail = [
        err.port      != null ? 'port ' + esc(err.port)           : '',
        err.directory ? 'directory ' + esc(err.directory) : '',
        err.pattern   ? 'pattern '   + esc(err.pattern)   : ''
    ].filter(Boolean).join(', ');

    el.innerHTML =
        '<div class="alert-card critical" style="margin-bottom:16px">' +
            '<div class="alert-sev-dot critical"></div>' +
            '<div class="alert-body">' +
                '<div class="alert-title">Interface halted' + (ts ? ' — ' + esc(ts) : '') + '</div>' +
                '<div class="alert-meta">' + esc(err.reason || 'Unknown error') +
                    (detail ? ' (' + detail + ')' : '') + '</div>' +
            '</div>' +
            '<button onclick="document.getElementById(\'interfaceErrorBanner\').style.display=\'none\'" ' +
                'style="border:none;background:none;cursor:pointer;color:var(--muted)" title="Dismiss">' +
                '<i class="fas fa-xmark"></i></button>' +
        '</div>';
    el.style.display = '';
}

function loadDLQDepthBadge() {
    if (!_interfaceId) return;
    var token = localStorage.getItem('accessToken');
    var headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = 'Bearer ' + token;
    fetch('/api/fhir/dlq/interface/' + _interfaceId + '/stats', { credentials: 'include', headers: headers })
        .then(function(r) { return r.ok ? r.json() : null; })
        .then(function(res) {
            if (!res || !res.success) return;
            var d = res.data || {};
            var depth = (d.pending || 0) + (d.abandoned || 0);
            var badge = document.getElementById('dlqDepthBadge');
            if (!badge) return;
            if (depth > 0) {
                document.getElementById('dlqDepthCount').textContent = depth;
                badge.href = 'admin-dlq.html?interface_id=' + _interfaceId;
                badge.style.display = 'flex';
            } else {
                badge.style.display = 'none';
            }
        })
        .catch(function() { /* non-fatal */ });
}

// ── Overview ──────────────────────────────────────────────────────────────────
// Manual Refresh only re-fetched the KPI grid, so the header pill could sit on a
// stale status while the KPI grid moved on — keep both in sync on every refresh.
function refreshOverviewTab() {
    loadInterfaceHeader();
    loadOverview();
}

function loadOverview() {
    if (!_interfaceId) return;

    // Load KPIs from interface health endpoint
    apiGet('/api/analytics/monitoring/interface-health')
        .then(function(res) {
            var cards = res.data || [];
            var card  = cards.find(function(c) { return c.interface_id === _interfaceId; });
            renderKPIs(card);
        })
        .catch(function() { renderKPIs(null); });

    // Load recent messages
    apiGet('/api/messages/interface/' + _interfaceId + '?limit=10')
        .then(function(res) {
            renderRecentMessages(res.data || res.messages || []);
        })
        .catch(function() {
            document.getElementById('recentMessages').innerHTML =
                '<tr><td colspan="5" style="text-align:center;padding:20px;color:var(--muted)">No messages found</td></tr>';
        });
}

function renderKPIs(card) {
    var el = document.getElementById('kpiGrid');
    if (!card) {
        el.innerHTML = kpiCard('Messages Today', '—', '') +
                       kpiCard('Error Rate', '—', '') +
                       kpiCard('Avg Processing', '—', '') +
                       kpiCard('Status', _interfaceData ? (_interfaceData.interface_status || 'draft').toUpperCase() : '—', '');
        return;
    }
    var errColor = card.error_rate_today > 10 ? 'color:#dc2626' : card.error_rate_today > 5 ? 'color:#d97706' : 'color:#16a34a';
    el.innerHTML =
        kpiCard('Messages Today', (card.messages_today || 0).toLocaleString(), 'last 24h') +
        kpiCard('Error Rate', card.error_rate_today != null ? card.error_rate_today.toFixed(1) + '%' : '0%', '24h window', errColor) +
        kpiCard('Avg Processing', card.avg_processing_time_ms != null ? Math.round(card.avg_processing_time_ms) + ' ms' : '—', 'last hour') +
        kpiCard('Status', (card.status || 'unknown').toUpperCase(), '');
}

function kpiCard(label, value, sub, style) {
    return '<div class="kpi-card">' +
        '<div class="kpi-label">' + esc(label) + '</div>' +
        '<div class="kpi-value"' + (style ? ' style="' + style + '"' : '') + '>' + esc(value) + '</div>' +
        (sub ? '<div class="kpi-sub">' + esc(sub) + '</div>' : '') +
    '</div>';
}

function renderRecentMessages(messages) {
    var tbody = document.getElementById('recentMessages');
    if (!messages.length) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;padding:20px;color:var(--muted)">No messages yet</td></tr>';
        return;
    }
    tbody.innerHTML = messages.slice(0, 10).map(function(m) {
        var status = (m.status || '').toLowerCase();
        var ts = m.received_at ? new Date(m.received_at).toLocaleString(undefined, { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' }) : '—';
        return '<tr>' +
            '<td style="font-family:monospace;font-size:11px">' + esc((m.message_id || m.id || '').slice(0, 16)) + '…</td>' +
            '<td>' + esc(m.message_type || '—') + '</td>' +
            '<td><span class="status-dot ' + status + '"></span>' + esc(status) + '</td>' +
            '<td>' + ts + '</td>' +
            '<td>' + (m.processing_time_ms != null ? m.processing_time_ms : '—') + '</td>' +
        '</tr>';
    }).join('');
}

// ── Pipeline tab ──────────────────────────────────────────────────────────────
function loadPipelineTab() {
    // The HTML already sets the href; nothing more to do on load.
}

// ── Messages tab ──────────────────────────────────────────────────────────────
function loadMessagesTab() {
    // The HTML already sets the href; nothing more to do on load.
}

// ── Alerts tab ────────────────────────────────────────────────────────────────
function loadAlertsTab() {
    if (!_interfaceId) return;
    loadInterfaceAlerts();
    loadInterfaceRules();
}

function loadInterfaceAlerts() {
    fetch('/api/alerts/fired?interfaceId=' + encodeURIComponent(_interfaceId), { credentials: 'include' })
        .then(function(r) { return r.json(); })
        .then(function(res) {
            var alerts = res.data || [];
            var el = document.getElementById('interfaceAlertList');
            if (!alerts.length) {
                el.innerHTML = '<div class="empty-state"><i class="fas fa-circle-check" style="color:#22c55e;font-size:28px"></i><p>No active alerts for this interface</p></div>';
                return;
            }
            el.innerHTML = alerts.map(function(a) {
                return '<div class="alert-card ' + a.severity + '">' +
                    '<div class="alert-sev-dot ' + a.severity + '"></div>' +
                    '<div class="alert-body">' +
                        '<div class="alert-title">' + esc(a.alert_type.replace(/_/g,' ')) + '</div>' +
                        '<div class="alert-meta">' + esc(a.message) + '</div>' +
                    '</div>' +
                    '<button onclick="ackAlert(\'' + a.id + '\')" style="padding:5px 12px;font-size:12px;border-radius:5px;cursor:pointer;border:1px solid var(--pink-mid);background:var(--pink-light);color:var(--pink)">Ack</button>' +
                '</div>';
            }).join('');
        })
        .catch(function() {});
}

function ackAlert(id) {
    fetch('/api/alerts/fired/' + id + '/acknowledge', {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ acknowledged_by: 'system' })
    }).then(function() { loadInterfaceAlerts(); });
}

function loadInterfaceRules() {
    fetch('/api/alerts/rules?interfaceId=' + encodeURIComponent(_interfaceId), { credentials: 'include' })
        .then(function(r) { return r.json(); })
        .then(function(res) {
            var rules = res.data || [];
            var el = document.getElementById('interfaceRuleList');
            if (!rules.length) {
                el.innerHTML = '<div class="empty-state"><i class="fas fa-list-check"></i><p>No rules configured. Global rules apply by default.</p></div>';
                return;
            }
            el.innerHTML = rules.map(function(r) {
                var thresholds = [];
                if (r.threshold_warning  != null) thresholds.push('⚠ ' + r.threshold_warning);
                if (r.threshold_critical != null) thresholds.push('🔴 ' + r.threshold_critical);
                return '<div class="rule-card">' +
                    '<span class="rule-type-badge">' + esc(r.rule_type.replace(/_/g,' ')) + '</span>' +
                    '<div class="rule-info">' +
                        '<div class="rule-name">' + esc(r.name) +
                            (!r.interface_id ? ' <span style="font-size:10px;color:var(--muted)">(global)</span>' : '') +
                        '</div>' +
                        '<div class="rule-desc">' + thresholds.join('  ') + '  window: ' + r.evaluation_window_minutes + 'min</div>' +
                    '</div>' +
                    '<a href="alerts.html#rules" style="font-size:12px;color:var(--pink);text-decoration:none">Edit →</a>' +
                '</div>';
            }).join('');
        })
        .catch(function() {});
}

function openNewRuleForInterface() {
    // Navigate to alerts page with pre-filled intent
    window.location.href = 'alerts.html#rules?newFor=' + encodeURIComponent(_interfaceId);
}

// Settings tab (name/description/status inline editing, Error Threshold,
// Recovery Queue/DLQ config, CDA Coverage Audit config, Delete Interface)
// was removed — it duplicated the "Edit Interface Configuration" modal
// (interfaces.html), which only ever sent the fields IT knew about, so
// saving from either editor silently reset whatever the other one owned.
// DLQ config / Error Threshold / CDA Coverage Audit now live on that modal's
// Basic tab instead (see modal-components.js, interfaces.js). Interface
// deletion remains available from the Interfaces list page, unchanged.

// ── Init ──────────────────────────────────────────────────────────────────────
window.addEventListener('load', function() {
    if (!_interfaceId) {
        document.getElementById('interfaceName').textContent = 'No interface selected';
        return;
    }
    loadInterfaceHeader();

    var hash = location.hash.replace('#', '').split('?')[0] || 'overview';
    activateTab(hash);
});
