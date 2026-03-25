// public/js/review-queue.js  v1
// Review Queue page logic — FHIR Validation failures

'use strict';

// ── State ────────────────────────────────────────────────────────────────────
let rqAllItems   = [];   // full fetched list
let rqFiltered   = [];   // after filter
let rqPage       = 1;
let rqPageSize   = 25;
let rqActiveItem = null; // item currently open in modal

// ── URL context ───────────────────────────────────────────────────────────────
// When navigated from an interface row, ?interfaceId= pre-filters the view
const _urlParams     = new URLSearchParams(window.location.search);
const _ctxInterfaceId = _urlParams.get('interfaceId') || null;

// ── Init ─────────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', async () => {
    // Use the shared AuthService (checks /api/auth/session, populates user UI)
    const ok = await window.AuthService.initialize();
    if (!ok) return; // AuthService handles redirect to login

    initSidebar();
    applyInterfaceContext();
    loadQueue();
    startClock();
    setInterval(loadQueue, 60_000);   // auto-refresh every 60 s
});

// Apply interface context from URL param
function applyInterfaceContext() {
    if (!_ctxInterfaceId) return;

    // Pre-populate the search box with the interface ID so the filter activates on load
    const searchEl = document.getElementById('rqSearch');
    if (searchEl) searchEl.value = _ctxInterfaceId;

    // Update page header subtitle to show scoped context
    const subtitle = document.querySelector('.rq-subtitle');
    if (subtitle) subtitle.textContent = `Showing validation failures for interface: ${_ctxInterfaceId}`;

    // Show breadcrumb back link
    const header = document.querySelector('.rq-header-left');
    if (header) {
        const back = document.createElement('a');
        back.href = 'interfaces.html';
        back.innerHTML = '<i class="far fa-arrow-left" style="margin-right:5px;"></i>Back to Interfaces';
        back.style.cssText = 'font-size:12px;color:#2563eb;text-decoration:none;white-space:nowrap;';
        header.insertBefore(back, header.firstChild);
    }
}

function logout() {
    window.AuthService.logout();
}

// ── Sidebar toggle ────────────────────────────────────────────────────────────
function initSidebar() {
    const toggle  = document.getElementById('sidebarToggle');
    const sidebar = document.getElementById('sidebar');
    const main    = document.getElementById('mainContent');
    if (!toggle || !sidebar) return;

    toggle.addEventListener('click', () => {
        const collapsed = sidebar.classList.toggle('collapsed');
        if (main) main.classList.toggle('sidebar-collapsed', collapsed);
        const icon = toggle.querySelector('.toggle-icon');
        if (icon) icon.textContent = collapsed ? '›' : '‹';
    });

    // section collapsing (same pattern as interfaces.html)
    document.querySelectorAll('.section-header').forEach(hdr => {
        hdr.addEventListener('click', () => {
            const items = hdr.nextElementSibling;
            if (items) items.classList.toggle('collapsed');
            const chevron = hdr.querySelector('.nav-section-chevron');
            if (chevron) chevron.classList.toggle('rotated');
        });
    });
}

// ── Clock ─────────────────────────────────────────────────────────────────────
function startClock() {
    function tick() {
        const now = new Date();
        const el = document.getElementById('currentTime');
        if (el) el.textContent = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }
    tick();
    setInterval(tick, 60_000);
}

// ── Load queue data ───────────────────────────────────────────────────────────
async function loadQueue() {
    try {
        const res = await fetch('/api/fhir/validation-queue', { credentials: 'include' });
        if (res.status === 401) { window.location.href = '/login.html'; return; }

        let items = [];
        if (res.ok) {
            const body = await res.json();
            items = Array.isArray(body) ? body :
                    Array.isArray(body.data) ? body.data : [];
        } else {
            // API not yet deployed — show demo data so the UI is testable
            items = getDemoItems();
        }

        rqAllItems = items;
        updateStats();
        updateNavBadge();
        applyRQFilters();
    } catch {
        rqAllItems = getDemoItems();
        updateStats();
        updateNavBadge();
        applyRQFilters();
    }
}

// ── Stats chips ───────────────────────────────────────────────────────────────
function updateStats() {
    let pending = 0, inReview = 0, resolved = 0;
    rqAllItems.forEach(item => {
        const s = (item.status || '').toLowerCase();
        if (s === 'pending_review')                        pending++;
        else if (s === 'in_review')                        inReview++;
        else if (s.startsWith('resolved') || s === 'rejected') resolved++;
    });
    setText('statPending',  pending);
    setText('statInReview', inReview);
    setText('statResolved', resolved);
}

function updateNavBadge() {
    const pending = rqAllItems.filter(i => (i.status || '') === 'pending_review').length;
    const badge   = document.getElementById('rqNavBadge');
    if (!badge) return;
    if (pending > 0) {
        badge.textContent   = pending > 99 ? '99+' : String(pending);
        badge.style.display = '';
    } else {
        badge.style.display = 'none';
    }
}

// ── Filtering ─────────────────────────────────────────────────────────────────
function applyRQFilters() {
    const search   = (val('rqSearch')          || '').toLowerCase();
    const status   = val('rqStatusFilter')     || 'all';
    const msgType  = val('rqMsgTypeFilter')    || 'all';
    const severity = val('rqSeverityFilter')   || 'all';

    rqFiltered = rqAllItems.filter(item => {
        // interface context filter (from URL param) takes priority over text search
        if (_ctxInterfaceId && item.interface_id !== _ctxInterfaceId) return false;
        // text search
        if (search && !_ctxInterfaceId) {
            const hay = [
                item.interface_name, item.interface_id,
                item.message_type, item.message_id,
            ].join(' ').toLowerCase();
            if (!hay.includes(search)) return false;
        }
        // status
        if (status !== 'all' && item.status !== status) return false;
        // message type prefix (ADT, ORU, …)
        if (msgType !== 'all') {
            const mt = (item.message_type || '').toUpperCase();
            if (!mt.startsWith(msgType)) return false;
        }
        // severity
        if (severity !== 'all') {
            const failures = item.validation_failures || [];
            const hasSeverity = failures.some(f => (f.severity || 'error') === severity);
            if (!hasSeverity) return false;
        }
        return true;
    });

    rqPage = 1;
    renderTable();
}

// ── Table render ──────────────────────────────────────────────────────────────
function renderTable() {
    const tbody = document.getElementById('rqTableBody');
    if (!tbody) return;

    const total = rqFiltered.length;
    const start = (rqPage - 1) * rqPageSize;
    const page  = rqFiltered.slice(start, start + rqPageSize);

    if (total === 0) {
        tbody.innerHTML = `<tr><td colspan="6">
            <div class="rq-empty">
                <i class="far fa-inbox"></i>
                <strong>No items found</strong>
                <span>The review queue is empty or no items match your filters.</span>
            </div>
        </td></tr>`;
        setText('rqInfo', '');
        document.getElementById('rqPagination').innerHTML = '';
        return;
    }

    tbody.innerHTML = page.map(item => renderRow(item)).join('');
    setText('rqInfo', `Showing ${start + 1}–${Math.min(start + rqPageSize, total)} of ${total}`);
    renderPagination(total);
}

function renderRow(item) {
    const failures   = item.validation_failures || [];
    const errorCount = failures.filter(f => (f.severity || 'error') === 'error').length;
    const warnCount  = failures.filter(f => f.severity === 'warning').length;

    const chips = [];
    failures.slice(0, 3).forEach(f => {
        const cls = (f.severity === 'warning') ? 'failure-chip warn' : 'failure-chip';
        chips.push(`<span class="${cls}">${esc(f.field || f.fhir_path || 'unknown')}</span>`);
    });
    if (failures.length > 3) {
        chips.push(`<span class="failure-chip" style="background:#f1f5f9;color:#64748b;border-color:#e2e8f0;">+${failures.length - 3} more</span>`);
    }

    const badge = statusBadge(item.status);
    const ts    = item.received_at ? relTime(item.received_at) : '—';

    return `<tr onclick="openRQModal('${esc(item.id)}')" title="Click to review">
        <td>
            <div style="font-weight:600;font-size:12px;color:#1e293b;">${esc(item.interface_name || item.interface_id || '—')}</div>
            <div style="font-size:10px;color:#94a3b8;margin-top:1px;">${esc(item.message_id || '')}</div>
        </td>
        <td><span style="font-family:monospace;font-size:12px;">${esc(item.message_type || '—')}</span></td>
        <td style="font-size:12px;color:#64748b;white-space:nowrap;">${ts}</td>
        <td>
            <div class="failure-chips">${chips.join('')}</div>
            <div style="font-size:10px;color:#94a3b8;margin-top:3px;">
                ${errorCount > 0 ? `${errorCount} required` : ''}
                ${warnCount  > 0 ? `${warnCount} warning${warnCount > 1 ? 's' : ''}` : ''}
            </div>
        </td>
        <td>${badge}</td>
        <td>
            <div class="rq-actions" onclick="event.stopPropagation()">
                <button class="rq-btn rq-btn-primary" onclick="openRQModal('${esc(item.id)}')">
                    <i class="far fa-pen-to-square" style="margin-right:3px;"></i>Review
                </button>
                <button class="rq-btn rq-btn-danger" onclick="quickReject('${esc(item.id)}')">
                    <i class="far fa-ban"></i>
                </button>
            </div>
        </td>
    </tr>`;
}

function statusBadge(status) {
    const map = {
        pending_review:    ['rq-badge-pending',  'Pending'],
        in_review:         ['rq-badge-review',   'In Review'],
        resolved_override: ['rq-badge-resolved', 'Overridden'],
        resolved_provided: ['rq-badge-resolved', 'Resolved'],
        resolved_rejected: ['rq-badge-rejected', 'Rejected'],
        rejected:          ['rq-badge-rejected', 'Rejected'],
    };
    const [cls, label] = map[status] || ['rq-badge-pending', status || 'Unknown'];
    return `<span class="rq-badge ${cls}">${label}</span>`;
}

// ── Pagination ────────────────────────────────────────────────────────────────
function renderPagination(total) {
    const pages = Math.ceil(total / rqPageSize);
    if (pages <= 1) { document.getElementById('rqPagination').innerHTML = ''; return; }

    const maxVisible = 7;
    let btns = [];

    btns.push(pageBtn('‹', rqPage - 1, rqPage === 1));

    if (pages <= maxVisible) {
        for (let i = 1; i <= pages; i++) btns.push(pageBtn(i, i, false, i === rqPage));
    } else {
        btns.push(pageBtn(1, 1, false, rqPage === 1));
        if (rqPage > 3)       btns.push('<span style="padding:3px 6px;color:#94a3b8;">…</span>');
        const start = Math.max(2, rqPage - 1);
        const end   = Math.min(pages - 1, rqPage + 1);
        for (let i = start; i <= end; i++) btns.push(pageBtn(i, i, false, i === rqPage));
        if (rqPage < pages - 2) btns.push('<span style="padding:3px 6px;color:#94a3b8;">…</span>');
        btns.push(pageBtn(pages, pages, false, rqPage === pages));
    }

    btns.push(pageBtn('›', rqPage + 1, rqPage === pages));
    document.getElementById('rqPagination').innerHTML = btns.join('');
}

function pageBtn(label, page, disabled, active = false) {
    const cls = 'rq-page-btn' + (active ? ' active' : '');
    const dis = disabled ? 'disabled style="opacity:0.4;cursor:default;"' : `onclick="goRQPage(${page})"`;
    return `<button class="${cls}" ${dis}>${label}</button>`;
}

function goRQPage(page) {
    rqPage = page;
    renderTable();
    document.getElementById('rqTableWrap')?.scrollTo(0, 0);
}

function handleRQPageSize() {
    rqPageSize = parseInt(val('rqPageSize') || '25', 10);
    rqPage = 1;
    renderTable();
}

// ── Modal ─────────────────────────────────────────────────────────────────────
function openRQModal(id) {
    const item = rqAllItems.find(i => i.id === id);
    if (!item) return;
    rqActiveItem = item;

    // mark as in_review if still pending
    if (item.status === 'pending_review') {
        item.status = 'in_review';
        updateItemStatus(id, 'in_review').catch(() => {});
        updateStats();
        renderTable();
    }

    // title
    setText('rqModalTitle',
        `Review: ${item.interface_name || item.interface_id || ''} · ${item.message_type || ''}`);

    // failures tab
    renderFailureList(item.validation_failures || []);

    // clear notes
    const notes = document.getElementById('rqReviewNotes');
    if (notes) notes.value = '';

    // raw HL7
    const hl7El = document.getElementById('rqRawHL7');
    if (hl7El) hl7El.textContent = item.raw_message || '— not available —';

    // partial FHIR
    const fhirEl = document.getElementById('rqPartialFHIR');
    if (fhirEl) {
        try {
            fhirEl.textContent = item.partial_fhir
                ? JSON.stringify(
                    typeof item.partial_fhir === 'string'
                        ? JSON.parse(item.partial_fhir)
                        : item.partial_fhir,
                    null, 2)
                : '— not available —';
        } catch {
            fhirEl.textContent = String(item.partial_fhir || '— not available —');
        }
    }

    // reset to failures tab
    switchRQTab('failures',
        document.querySelector('.rq-tab'));

    document.getElementById('rqModal').classList.add('open');
}

function closeRQModal() {
    document.getElementById('rqModal').classList.remove('open');
    rqActiveItem = null;
}

// close on overlay click
document.addEventListener('DOMContentLoaded', () => {
    const overlay = document.getElementById('rqModal');
    if (overlay) {
        overlay.addEventListener('click', e => {
            if (e.target === overlay) closeRQModal();
        });
    }
});

// ── Tab switching ─────────────────────────────────────────────────────────────
function switchRQTab(name, btn) {
    // deactivate all tabs
    document.querySelectorAll('.rq-tab').forEach(t => t.classList.remove('active'));
    // hide all panels
    ['failures', 'hl7', 'fhir'].forEach(n => {
        const el = document.getElementById(`rqTab-${n}`);
        if (el) el.style.display = 'none';
    });
    // activate selected
    const panel = document.getElementById(`rqTab-${name}`);
    if (panel) panel.style.display = '';
    // mark button active — btn might be the clicked button or we find it
    const activBtn = btn || document.querySelector(`.rq-tab[onclick*="'${name}'"]`);
    if (activBtn) activBtn.classList.add('active');
}

// ── Failure list renderer ─────────────────────────────────────────────────────
function renderFailureList(failures) {
    const list = document.getElementById('rqFailureList');
    if (!list) return;

    if (failures.length === 0) {
        list.innerHTML = `<div style="text-align:center;padding:20px;color:#94a3b8;font-size:13px;">
            No validation failures recorded.
        </div>`;
        return;
    }

    list.innerHTML = failures.map((f, idx) => {
        const isWarn = f.severity === 'warning';
        const rowCls = isWarn ? 'failure-row warn' : 'failure-row';
        const icoCls = isWarn ? 'far fa-triangle-exclamation' : 'fas fa-circle-xmark';
        const field  = f.field || f.fhir_path || 'unknown';
        const desc   = f.message || f.description || `Missing required field: ${field}`;
        const policy = f.on_missing || f.onMissing || 'queue_review';

        return `<div class="${rowCls}" id="failure-row-${idx}">
            <div class="failure-row-icon"><i class="${icoCls}"></i></div>
            <div class="failure-row-body">
                <div class="failure-row-field">${esc(field)}</div>
                <div class="failure-row-detail">${esc(desc)}</div>
                ${renderOnMissingBadge(policy)}
                <div class="override-input">
                    <input type="text"
                           id="override-val-${idx}"
                           data-field="${esc(field)}"
                           placeholder="Enter value to provide for this field…">
                    <button class="rq-btn rq-btn-primary"
                            style="padding:3px 8px;font-size:10px;"
                            onclick="prefillValue(${idx})">Use</button>
                </div>
            </div>
        </div>`;
    }).join('');
}

function renderOnMissingBadge(policy) {
    const labels = {
        proceed:      ['#6b7280', '#f1f5f9', 'proceed'],
        warn:         ['#d97706', '#fffbeb', 'warn'],
        queue_review: ['#2563eb', '#eff6ff', 'queue_review'],
        reject:       ['#dc2626', '#fef2f2', 'reject'],
        default:      ['#64748b', '#f8fafc', 'default'],
    };
    const [color, bg, label] = labels[policy] || labels.default;
    return `<div style="margin-top:3px;">
        <span style="font-size:9px;background:${bg};color:${color};border-radius:4px;padding:1px 5px;border:1px solid ${color}33;font-weight:600;">
            policy: ${label}
        </span>
    </div>`;
}

function prefillValue(idx) {
    const input = document.getElementById(`override-val-${idx}`);
    if (!input || !input.value.trim()) {
        showToast('Enter a value first', 'warn');
        return;
    }
    input.style.borderColor = '#10b981';
    showToast(`Value staged for "${input.dataset.field}"`, 'success');
}

// ── Actions ───────────────────────────────────────────────────────────────────
async function resolveWithValues() {
    if (!rqActiveItem) return;

    // collect override values
    const overrides = {};
    document.querySelectorAll('#rqFailureList [data-field]').forEach(inp => {
        const v = inp.value.trim();
        if (v) overrides[inp.dataset.field] = v;
    });

    const notes = document.getElementById('rqReviewNotes')?.value?.trim() || '';

    try {
        const res = await fetch(`/api/fhir/validation-queue/${rqActiveItem.id}/resolve`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ overrides, notes, resolution: 'provided' }),
        });

        if (res.ok) {
            updateLocalItem(rqActiveItem.id, 'resolved_provided');
            closeRQModal();
            showToast('Values applied — message re-queued for delivery', 'success');
        } else {
            showToast('Failed to resolve: ' + (await safeText(res)), 'error');
        }
    } catch {
        // optimistic update when API not available
        updateLocalItem(rqActiveItem.id, 'resolved_provided');
        closeRQModal();
        showToast('Resolved (offline mode)', 'success');
    }
}

async function overrideItem() {
    if (!rqActiveItem) return;
    const notes = document.getElementById('rqReviewNotes')?.value?.trim() || '';

    try {
        const res = await fetch(`/api/fhir/validation-queue/${rqActiveItem.id}/override`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ notes }),
        });

        if (res.ok) {
            updateLocalItem(rqActiveItem.id, 'resolved_override');
            closeRQModal();
            showToast('Message delivered as-is (override)', 'warn');
        } else {
            showToast('Override failed: ' + (await safeText(res)), 'error');
        }
    } catch {
        updateLocalItem(rqActiveItem.id, 'resolved_override');
        closeRQModal();
        showToast('Overridden (offline mode)', 'warn');
    }
}

async function rejectItem() {
    if (!rqActiveItem) return;
    const notes = document.getElementById('rqReviewNotes')?.value?.trim() || '';

    if (!confirm(`Reject this message and move to DLQ?\n\nMessage: ${rqActiveItem.message_id || rqActiveItem.id}`)) return;

    try {
        const res = await fetch(`/api/fhir/validation-queue/${rqActiveItem.id}/reject`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ notes }),
        });

        if (res.ok) {
            updateLocalItem(rqActiveItem.id, 'rejected');
            closeRQModal();
            showToast('Message rejected and moved to DLQ', 'error');
        } else {
            showToast('Rejection failed: ' + (await safeText(res)), 'error');
        }
    } catch {
        updateLocalItem(rqActiveItem.id, 'rejected');
        closeRQModal();
        showToast('Rejected (offline mode)', 'error');
    }
}

async function quickReject(id) {
    if (!confirm('Reject this message and move to DLQ?')) return;
    try {
        const res = await fetch(`/api/fhir/validation-queue/${id}/reject`, {
            method: 'PUT',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({}),
        });
        if (res.ok || !res.ok) {  // optimistic either way
            updateLocalItem(id, 'rejected');
            showToast('Rejected', 'error');
        }
    } catch {
        updateLocalItem(id, 'rejected');
        showToast('Rejected (offline mode)', 'error');
    }
}

// ── Helpers ───────────────────────────────────────────────────────────────────
async function updateItemStatus(id, status) {
    return fetch(`/api/fhir/validation-queue/${id}/status`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status }),
    });
}

function updateLocalItem(id, status) {
    const item = rqAllItems.find(i => i.id === id);
    if (item) item.status = status;
    updateStats();
    updateNavBadge();
    applyRQFilters();
}

function val(id) {
    const el = document.getElementById(id);
    return el ? el.value : '';
}

function setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = String(text);
}

function esc(s) {
    return String(s || '')
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

async function safeText(res) {
    try { return await res.text(); } catch { return res.status; }
}

function relTime(iso) {
    const ms   = Date.now() - new Date(iso).getTime();
    const secs = Math.floor(ms / 1000);
    if (secs < 60)   return 'just now';
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
    if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
    return new Date(iso).toLocaleDateString([], { month: 'short', day: 'numeric' });
}

// ── Toast ─────────────────────────────────────────────────────────────────────
let _toastTimer;
function showToast(msg, type = 'success') {
    const el = document.getElementById('rqToast');
    if (!el) return;
    const colors = {
        success: ['#059669', '#ecfdf5'],
        warn:    ['#d97706', '#fffbeb'],
        error:   ['#dc2626', '#fef2f2'],
    };
    const [fg, bg] = colors[type] || colors.success;
    el.style.color      = fg;
    el.style.background = bg;
    el.style.border     = `1px solid ${fg}44`;
    el.style.boxShadow  = `0 4px 16px ${fg}22`;
    el.textContent      = msg;
    el.style.display    = 'block';
    clearTimeout(_toastTimer);
    _toastTimer = setTimeout(() => { el.style.display = 'none'; }, 3500);
}

// ── Demo data (used when API endpoint not yet available) ──────────────────────
function getDemoItems() {
    const now = Date.now();
    return [
        {
            id: 'demo-1',
            interface_name: 'Epic ADT Feed',
            interface_id: 'intf-001',
            message_id: 'MSG-20260316-001',
            message_type: 'ADT^A01',
            status: 'pending_review',
            received_at: new Date(now - 5 * 60_000).toISOString(),
            validation_failures: [
                { field: 'Patient.identifier', fhir_path: 'Patient.identifier', severity: 'error',
                  message: 'Patient MRN (PID.3) is missing or empty', on_missing: 'queue_review' },
                { field: 'Patient.birthDate', fhir_path: 'Patient.birthDate', severity: 'error',
                  message: 'Date of birth (PID.7) required for patient matching', on_missing: 'queue_review' },
            ],
            raw_message: 'MSH|^~\\&|EPIC|HOSPA|EHK|EHK|20260316120000||ADT^A01|MSG001|P|2.5\rPID|1||^^^^^MR||SMITH^JOHN||19800101|M',
            partial_fhir: { resourceType: 'Bundle', type: 'transaction', entry: [
                { resource: { resourceType: 'Patient', name: [{ family: 'SMITH', given: ['JOHN'] }] } }
            ]},
        },
        {
            id: 'demo-2',
            interface_name: 'Lab System ORU',
            interface_id: 'intf-002',
            message_id: 'MSG-20260316-002',
            message_type: 'ORU^R01',
            status: 'pending_review',
            received_at: new Date(now - 23 * 60_000).toISOString(),
            validation_failures: [
                { field: 'Observation.status', fhir_path: 'Observation.status', severity: 'error',
                  message: 'OBX.11 (result status) is empty — required for Observation.status', on_missing: 'queue_review' },
                { field: 'Observation.code.coding[0].code', severity: 'warning',
                  message: 'LOINC code (OBX.3.1) not found in LOINC registry — may be non-standard', on_missing: 'warn' },
            ],
            raw_message: 'MSH|^~\\&|LAB|HOSPA|EHK|EHK|20260316113000||ORU^R01|MSG002|P|2.5\rOBR|1|ORD001|FIL001|^GLUCOSE|||20260316||||||||||||||F\rOBX|1||59408-5^OXYGEN SAT||98.6|%|95-100|N|||',
            partial_fhir: null,
        },
        {
            id: 'demo-3',
            interface_name: 'Epic ADT Feed',
            interface_id: 'intf-001',
            message_id: 'MSG-20260315-099',
            message_type: 'ADT^A08',
            status: 'in_review',
            received_at: new Date(now - 4 * 3600_000).toISOString(),
            validation_failures: [
                { field: 'Patient.address', severity: 'warning',
                  message: 'PID.11 (patient address) is empty — optional but expected', on_missing: 'warn' },
            ],
            raw_message: 'MSH|^~\\&|EPIC|HOSPA|EHK|EHK|20260315080000||ADT^A08|MSG099|P|2.5\rPID|1||MRN-123456^^^HOSPA^MR||DOE^JANE||19750304|F',
            partial_fhir: { resourceType: 'Bundle', type: 'transaction', entry: [] },
        },
        {
            id: 'demo-4',
            interface_name: 'Cardiology Dept',
            interface_id: 'intf-003',
            message_id: 'MSG-20260314-050',
            message_type: 'ORM^O01',
            status: 'resolved_provided',
            received_at: new Date(now - 48 * 3600_000).toISOString(),
            validation_failures: [
                { field: 'ServiceRequest.requester', severity: 'error',
                  message: 'Ordering provider (ORC.12) missing', on_missing: 'queue_review' },
            ],
            raw_message: null,
            partial_fhir: null,
        },
    ];
}
