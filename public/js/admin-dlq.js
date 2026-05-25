// public/js/admin-dlq.js
// Dead-Letter Queue management page — IIFE module
(function () {
    'use strict';

    // ─── State ────────────────────────────────────────────────────────────────
    let _rows        = [];
    let _selected    = new Set();
    let _redriveTarget = null;   // single row ID or null (bulk uses _selected)
    let _interfaces  = {};       // id → { name, id } — populated on init
    let _scheduleOpen = false;   // whether the schedule picker panel is visible

    // ─── DOM helpers ──────────────────────────────────────────────────────────
    const esc = s => String(s ?? '').replace(/[&<>"']/g, c =>
        ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    const el = id => document.getElementById(id);

    function fmtDate(iso) {
        if (!iso) return '—';
        const d = new Date(iso);
        return d.toLocaleString(undefined, { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    }

    function statusBadge(status) {
        return `<span class="badge-status badge-${esc(status)}">${esc(status)}</span>`;
    }

    // ─── Auth headers ─────────────────────────────────────────────────────────
    function _authHeaders() {
        const token = localStorage.getItem('accessToken');
        const h = { 'Content-Type': 'application/json' };
        if (token) h['Authorization'] = 'Bearer ' + token;
        return h;
    }

    async function apiFetch(path, opts = {}) {
        const res = await fetch('/api/fhir/dlq' + path, {
            credentials: 'include',
            headers: _authHeaders(),
            ...opts,
        });
        return res.json();
    }

    async function apiFetchInterfaces() {
        const res = await fetch('/api/interfaces', {
            credentials: 'include',
            headers: _authHeaders(),
        });
        return res.json();
    }

    // ─── Interface name lookup ────────────────────────────────────────────────
    async function loadInterfaces() {
        try {
            const res = await apiFetchInterfaces();
            const list = res.data || res.interfaces || res || [];
            _interfaces = {};
            list.forEach(i => { _interfaces[i.id] = i.name || i.id; });
            _buildInterfaceDatalist(list);
        } catch (_) { /* non-fatal — IDs still show */ }
    }

    function ifaceName(id) {
        return _interfaces[id] || id;
    }

    function _buildInterfaceDatalist(list) {
        const dl = el('ifaceDatalist');
        if (!dl) return;
        dl.innerHTML = list.map(i =>
            `<option value="${esc(i.name || i.id)}" data-id="${esc(i.id)}">`
        ).join('');
    }

    // Resolve the text in #filterInterface to an interface ID for the API call.
    function _resolveFilterInterfaceID() {
        const txt = (el('filterInterface').value || '').trim();
        if (!txt) return '';
        // If it looks like a UUID already, use it directly
        if (/^[0-9a-f-]{36}$/i.test(txt)) return txt;
        // Otherwise look up by name
        const match = Object.entries(_interfaces).find(([, name]) =>
            name.toLowerCase() === txt.toLowerCase()
        );
        return match ? match[0] : txt; // fall back to the raw string
    }

    // ─── URL param pre-filter ─────────────────────────────────────────────────
    // If opened from the messages page with ?message_id=xxx, lock the filter
    // to that message so the DLQ table shows only those rows.
    function _applyURLPreFilter() {
        const params = new URLSearchParams(window.location.search);
        const msgId  = params.get('message_id');
        if (!msgId) return;

        // Show all statuses when coming from a message link
        el('filterStatus').value = '';
        // Store the message_id as a hidden filter (not exposed in the UI filter row)
        el('filterInterface').dataset.lockedMessageId = msgId;
        // Surface it visually so users know they're in a filtered view
        const notice = document.createElement('div');
        notice.style.cssText = 'background:#fef3c7;border:1px solid #fcd34d;border-radius:6px;' +
            'padding:8px 14px;font-size:12px;color:#92400e;display:flex;align-items:center;gap:8px;margin-bottom:10px;';
        notice.innerHTML = `<i class="fas fa-filter"></i> Showing DLQ rows for message <strong>${msgId}</strong>` +
            `<button onclick="this.parentElement.remove(); delete document.getElementById('filterInterface').dataset.lockedMessageId; window._dlq.load();"` +
            ` style="margin-left:auto;background:none;border:none;cursor:pointer;color:#92400e;font-size:13px;">✕ Clear</button>`;
        const layout = document.querySelector('.dlq-layout');
        if (layout) layout.insertBefore(notice, layout.firstChild);
    }

    // ─── Stats ────────────────────────────────────────────────────────────────
    async function loadStats() {
        try {
            const res = await apiFetch('/stats');
            if (res.success) {
                const d = res.data || {};
                const total = (d.pending ?? 0) + (d.retrying ?? 0) + (d.abandoned ?? 0) + (d.resolved ?? 0);
                el('statAll').textContent      = total;
                el('statPending').textContent   = d.pending   ?? 0;
                el('statRetrying').textContent  = d.retrying  ?? 0;
                el('statAbandoned').textContent = d.abandoned ?? 0;
                el('statResolved').textContent  = d.resolved  ?? 0;
                _syncStatTileHighlight();
            }
        } catch (_) { /* non-fatal */ }
    }

    // Highlight the stat tile matching the current filter selection.
    function _syncStatTileHighlight() {
        const status = el('filterStatus').value;
        const map = {
            '':         'statCardAll',
            'pending':  'statCardPending',
            'retrying': 'statCardRetrying',
            'abandoned':'statCardAbandoned',
            'resolved': 'statCardResolved',
        };
        Object.values(map).forEach(id => {
            const tile = el(id);
            if (tile) tile.classList.remove('active-filter');
        });
        const active = el(map[status] || 'statCardAll');
        if (active) active.classList.add('active-filter');
    }

    // Click a stat tile to instantly filter the table.
    function filterByStatus(status) {
        el('filterStatus').value = status;
        _syncStatTileHighlight();
        load();
    }

    // ─── Table load ───────────────────────────────────────────────────────────
    async function load() {
        el('dlqTableBody').innerHTML =
            '<tr class="loading-row"><td colspan="11"><i class="fas fa-spinner fa-spin"></i> Loading…</td></tr>';
        clearSelection();

        const status = el('filterStatus').value;
        const ifaceID = _resolveFilterInterfaceID();
        const lockedMsgId = (el('filterInterface').dataset || {}).lockedMessageId || '';
        let url = `?limit=100&offset=0`;
        if (status)      url += `&status=${encodeURIComponent(status)}`;
        if (ifaceID)     url += `&interface_id=${encodeURIComponent(ifaceID)}`;
        if (lockedMsgId) url += `&message_id=${encodeURIComponent(lockedMsgId)}`;

        try {
            const [res] = await Promise.all([apiFetch(url), loadStats()]);
            if (!res.success) throw new Error(res.error || 'Request failed');
            _rows = res.data || [];
            renderTable(_rows);
        } catch (err) {
            el('dlqTableBody').innerHTML =
                `<tr class="loading-row"><td colspan="11" style="color:#dc2626;">` +
                `<i class="fas fa-exclamation-circle"></i> ${esc(err.message)}</td></tr>`;
        }
    }

    function renderTable(rows) {
        if (!rows.length) {
            el('dlqTableBody').innerHTML =
                `<tr><td colspan="11"><div class="empty-state">` +
                `<i class="fas fa-inbox"></i><p>No DLQ rows for this filter</p></div></td></tr>`;
            return;
        }
        el('dlqTableBody').innerHTML = rows.map(r => {
            const isAbandoned = r.Status === 'abandoned';
            const redriveLabel = isAbandoned ? 'Reactivate' : 'Redrive';
            const redriveIcon  = isAbandoned ? 'fa-rotate-right' : 'fa-redo';
            return `
            <tr id="row-${esc(r.ID)}" class="${_selected.has(r.ID) ? 'selected' : ''} ${isAbandoned ? 'row-abandoned' : ''}">
                <td><input type="checkbox" ${_selected.has(r.ID) ? 'checked' : ''}
                    onchange="window._dlq.toggleRow('${esc(r.ID)}', this)"></td>
                <td>${statusBadge(r.Status)}</td>
                <td><span class="truncate" title="${esc(r.InterfaceID)}">${esc(ifaceName(r.InterfaceID))}</span></td>
                <td><span class="mono truncate" title="${esc(r.MessageID)}">${esc(r.MessageID || '—')}</span></td>
                <td><span class="truncate" title="${esc(r.StepName)}">${esc(r.StepName || '—')}</span></td>
                <td><span class="mono-sm">${esc(r.ConnectorType || '—')}</span></td>
                <td><span class="err-cell" title="${esc(r.ErrorMessage)}">${esc(r.ErrorMessage)}</span></td>
                <td style="text-align:center;">${r.AttemptCount}</td>
                <td>${r.Status === 'pending' ? fmtDate(r.NextRetryAt) : '—'}</td>
                <td>${fmtDate(r.CreatedAt)}</td>
                <td>
                    <button class="btn-sm btn-redrive" title="${redriveLabel}"
                        onclick="window._dlq.openRedriveModal('${esc(r.ID)}')">
                        <i class="fas ${redriveIcon}"></i>
                    </button>
                    <button class="btn-sm btn-abandon" title="Abandon" style="margin-left:4px;"
                        onclick="window._dlq.abandonOne('${esc(r.ID)}')"
                        ${isAbandoned ? 'disabled style="margin-left:4px;opacity:0.4;cursor:not-allowed;"' : ''}>
                        <i class="fas fa-ban"></i>
                    </button>
                    <button class="btn-sm btn-secondary" title="Detail"
                        style="margin-left:4px; padding:4px 8px;"
                        onclick="window._dlq.openDrawer('${esc(r.ID)}')">
                        <i class="fas fa-search"></i>
                    </button>
                </td>
            </tr>`;
        }).join('');
    }

    // ─── Selection ────────────────────────────────────────────────────────────
    function toggleRow(id, cb) {
        if (cb.checked) _selected.add(id);
        else _selected.delete(id);
        const row = document.getElementById('row-' + id);
        if (row) row.classList.toggle('selected', cb.checked);
        updateBulkBar();
    }

    function toggleAll(cb) {
        _rows.forEach(r => {
            if (cb.checked) _selected.add(r.ID);
            else _selected.delete(r.ID);
        });
        document.querySelectorAll('#dlqTableBody input[type=checkbox]').forEach(c => c.checked = cb.checked);
        document.querySelectorAll('#dlqTableBody tr').forEach(tr => tr.classList.toggle('selected', cb.checked));
        updateBulkBar();
    }

    function clearSelection() {
        _selected.clear();
        document.querySelectorAll('#dlqTableBody input[type=checkbox]').forEach(c => c.checked = false);
        document.querySelectorAll('#dlqTableBody tr').forEach(tr => tr.classList.remove('selected'));
        el('checkAll').checked = false;
        updateBulkBar();
    }

    function updateBulkBar() {
        const bar = el('bulkBar');
        const cnt = _selected.size;
        el('bulkCount').textContent = `${cnt} selected`;
        bar.classList.toggle('visible', cnt > 0);
    }

    // ─── Redrive modal ────────────────────────────────────────────────────────
    function openRedriveModal(id) {
        _redriveTarget = id;
        _scheduleOpen  = false;
        _resetModeSelection();
        // Single message: simple title, no schedule option
        el('modalTitle').innerHTML    = '<i class="fas fa-redo" style="color:#2563eb;"></i> Redrive Message';
        el('modalSubtitle').textContent = 'Choose how to re-run this message through the pipeline.';
        _renderSchedulePanel(false);
        el('redriveModal').classList.add('open');
    }

    function closeRedriveModal() {
        _redriveTarget = null;
        _scheduleOpen  = false;
        el('redriveModal').classList.remove('open');
    }

    function _resetModeSelection() {
        document.querySelectorAll('input[name=redriveMode]').forEach(r => { r.checked = r.value === 'from_failed_step'; });
        el('modeOptionFailed').classList.add('selected');
        el('modeOptionStart').classList.remove('selected');
    }

    // Click handler on the div — selects the radio inside it
    function selectModeDiv(value) {
        document.querySelectorAll('input[name=redriveMode]').forEach(r => { r.checked = r.value === value; });
        el('modeOptionFailed').classList.toggle('selected', value === 'from_failed_step');
        el('modeOptionStart').classList.toggle('selected',  value === 'from_start');
    }

    function selectMode(radio) {
        el('modeOptionFailed').classList.toggle('selected', radio.value === 'from_failed_step');
        el('modeOptionStart').classList.toggle('selected',  radio.value === 'from_start');
    }

    // Toggle the schedule picker panel open/closed.
    function toggleSchedulePanel() {
        _scheduleOpen = !_scheduleOpen;
        _renderSchedulePanel(_scheduleOpen);
    }

    function _renderSchedulePanel(open) {
        const panel      = el('schedulePanel');
        const confirmBtn = el('confirmScheduleBtn');
        const redriveBtn = el('redriveNowBtn');
        if (!panel) return;
        panel.style.display = open ? 'block' : 'none';
        if (confirmBtn) confirmBtn.style.display = open ? '' : 'none';
        if (redriveBtn) redriveBtn.style.display  = open ? 'none' : '';

        if (open && !el('scheduleAt').value) {
            _applyPreset(1);
        }
    }

    // Preset chips: set #scheduleAt to a future ISO datetime
    function _applyPreset(hours) {
        const d = new Date(Date.now() + hours * 3600 * 1000);
        // datetime-local format: YYYY-MM-DDTHH:mm
        const pad = n => String(n).padStart(2, '0');
        el('scheduleAt').value =
            `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}` +
            `T${pad(d.getHours())}:${pad(d.getMinutes())}`;
        document.querySelectorAll('.preset-chip').forEach(c =>
            c.classList.toggle('active', c.dataset.hours == hours));
    }

    // "Redrive Now" button in the modal footer
    async function confirmRedrive() {
        const mode = document.querySelector('input[name=redriveMode]:checked')?.value || 'from_failed_step';
        closeRedriveModal();
        if (_redriveTarget) {
            await singleRedrive(_redriveTarget, mode);
        } else {
            await bulkRedriveWithMode(mode);
        }
        load();
    }

    // "Confirm Schedule" button in the modal footer (only visible when panel is open)
    async function confirmSchedule() {
        const mode = document.querySelector('input[name=redriveMode]:checked')?.value || 'from_failed_step';
        const atVal = el('scheduleAt').value;
        if (!atVal) { showToast('Pick a scheduled time', 'warning'); return; }
        // datetime-local gives "2026-05-25T10:00" — append Z for UTC
        const at = new Date(atVal).toISOString();
        closeRedriveModal();
        if (_redriveTarget) {
            await scheduleOne(_redriveTarget, mode, at);
        } else {
            await bulkScheduleWithMode(mode, at);
        }
        load();
    }

    async function singleRedrive(id, mode) {
        try {
            const res = await apiFetch(`/${id}/redrive`, {
                method: 'POST',
                body: JSON.stringify({ mode }),
            });
            if (!res.success) throw new Error(res.error);
            showToast('Redrive triggered', 'success');
        } catch (err) {
            showToast('Redrive failed: ' + err.message, 'error');
        }
    }

    async function scheduleOne(id, mode, at) {
        try {
            const res = await apiFetch(`/${id}/schedule`, {
                method: 'POST',
                body: JSON.stringify({ mode, at }),
            });
            if (!res.success) throw new Error(res.error);
            showToast('Redrive scheduled for ' + fmtDate(at), 'success');
        } catch (err) {
            showToast('Schedule failed: ' + err.message, 'error');
        }
    }

    // Bulk redrive — opens modal for selected rows (no schedule)
    function bulkRedrive() {
        _redriveTarget = null;
        _scheduleOpen  = false;
        _resetModeSelection();
        const n = _selected.size;
        el('modalTitle').innerHTML    = `<i class="fas fa-redo" style="color:#2563eb;"></i> Redrive ${n} Message${n !== 1 ? 's' : ''}`;
        el('modalSubtitle').textContent = 'Choose how to re-run the selected messages through the pipeline.';
        _renderSchedulePanel(false);
        el('redriveModal').classList.add('open');
    }

    // Bulk schedule — opens modal with schedule panel pre-expanded
    function bulkSchedule() {
        _redriveTarget = null;
        _scheduleOpen  = true;
        _resetModeSelection();
        const n = _selected.size;
        el('modalTitle').innerHTML    = `<i class="fas fa-calendar-alt" style="color:#2563eb;"></i> Schedule ${n} Message${n !== 1 ? 's' : ''}`;
        el('modalSubtitle').textContent = 'Pick a time and the selected messages will be redriven then.';
        el('redriveModal').classList.add('open');
        _renderSchedulePanel(true);
    }

    async function bulkRedriveWithMode(mode) {
        const ids = [..._selected];
        if (!ids.length) return;
        try {
            const res = await apiFetch('/bulk-redrive', {
                method: 'POST',
                body: JSON.stringify({ ids, mode }),
            });
            if (!res.success) throw new Error(res.error);
            const ok   = (res.data || []).filter(r => r.success).length;
            const fail = ids.length - ok;
            showToast(`${ok} redriven${fail ? ', ' + fail + ' failed' : ''}`, fail ? 'warning' : 'success');
        } catch (err) {
            showToast('Bulk redrive failed: ' + err.message, 'error');
        }
    }

    async function bulkScheduleWithMode(mode, at) {
        const ids = [..._selected];
        if (!ids.length) return;
        try {
            const res = await apiFetch('/bulk-schedule', {
                method: 'POST',
                body: JSON.stringify({ ids, mode, at }),
            });
            if (!res.success) throw new Error(res.error);
            showToast(`${res.affected || ids.length} row(s) scheduled`, 'success');
        } catch (err) {
            showToast('Bulk schedule failed: ' + err.message, 'error');
        }
    }

    // ─── Abandon ──────────────────────────────────────────────────────────────
    async function abandonOne(id) {
        if (!confirm('Abandon this DLQ row? It will not be retried again.')) return;
        try {
            const res = await apiFetch(`/${id}/abandon`, { method: 'POST' });
            if (!res.success) throw new Error(res.error);
            showToast('Row abandoned', 'success');
            load();
        } catch (err) {
            showToast('Abandon failed: ' + err.message, 'error');
        }
    }

    async function bulkAbandon() {
        const ids = [..._selected];
        if (!ids.length) return;
        if (!confirm(`Abandon ${ids.length} row(s)? They will not be retried.`)) return;
        try {
            const res = await apiFetch('/bulk-abandon', {
                method: 'POST',
                body: JSON.stringify({ ids }),
            });
            if (!res.success) throw new Error(res.error);
            showToast(`${res.affected || ids.length} row(s) abandoned`, 'success');
            load();
        } catch (err) {
            showToast('Bulk abandon failed: ' + err.message, 'error');
        }
    }

    // ─── Detail drawer ────────────────────────────────────────────────────────
    async function openDrawer(id) {
        el('drawerContent').innerHTML =
            '<p style="color:#9ca3af; font-size:13px;"><i class="fas fa-spinner fa-spin"></i> Loading…</p>';
        el('drawerOverlay').classList.add('open');
        el('detailDrawer').style.transform = 'translateX(0)';

        try {
            const res = await apiFetch('/' + id);
            if (!res.success) throw new Error(res.error);
            renderDrawer(res.data);
        } catch (err) {
            el('drawerContent').innerHTML = `<p style="color:#dc2626;">${esc(err.message)}</p>`;
        }
    }

    function closeDrawer() {
        el('drawerOverlay').classList.remove('open');
        el('detailDrawer').style.transform = 'translateX(100%)';
    }

    function renderDrawer(r) {
        const isAbandoned = r.Status === 'abandoned';
        el('drawerContent').innerHTML = `
            <div class="detail-section">
                <h4>Identity</h4>
                <div class="detail-grid">
                    <span class="key">ID</span>           <span class="val mono">${esc(r.ID)}</span>
                    <span class="key">Message ID</span>   <span class="val mono">${esc(r.MessageID)}</span>
                    <span class="key">Interface</span>    <span class="val">${esc(ifaceName(r.InterfaceID))}
                        <span class="mono" style="font-size:10px;color:#9ca3af;"> ${esc(r.InterfaceID)}</span></span>
                    <span class="key">Pipeline ID</span>  <span class="val mono">${esc(r.PipelineID || '—')}</span>
                    <span class="key">Failed Step</span>  <span class="val">${esc(r.StepName || '—')}</span>
                    <span class="key">Step ID</span>      <span class="val mono">${esc(r.FailedStepID || '—')}</span>
                </div>
            </div>
            <div class="detail-section">
                <h4>Delivery</h4>
                <div class="detail-grid">
                    <span class="key">Connector</span>    <span class="val mono">${esc(r.ConnectorType || '—')}</span>
                    <span class="key">Content Type</span> <span class="val">${esc(r.ContentType || '—')}</span>
                    <span class="key">Status</span>       <span class="val">${statusBadge(r.Status)}</span>
                    <span class="key">Attempts</span>     <span class="val">${r.AttemptCount}</span>
                    <span class="key">Redrive Mode</span> <span class="val">${esc(r.RedriveMode)}</span>
                    <span class="key">Next Retry</span>   <span class="val">${fmtDate(r.NextRetryAt)}</span>
                    <span class="key">Expires At</span>   <span class="val">${fmtDate(r.ExpiresAt) || 'never'}</span>
                    <span class="key">Created At</span>   <span class="val">${fmtDate(r.CreatedAt)}</span>
                </div>
            </div>
            <div class="detail-section">
                <h4>Error</h4>
                <pre class="payload" style="color:#fca5a5;">${esc(r.ErrorMessage)}</pre>
            </div>
            ${r.Payload ? `
            <div class="detail-section">
                <h4>Payload (failed delivery content)</h4>
                <pre class="payload">${esc(r.Payload.length > 4000
                    ? r.Payload.slice(0, 4000) + '\n… (truncated)'
                    : r.Payload)}</pre>
            </div>` : ''}
            <div class="detail-section" style="margin-top:18px;">
                <button class="btn-redrive btn-sm" onclick="window._dlq.openRedriveModal('${esc(r.ID)}')">
                    <i class="fas ${isAbandoned ? 'fa-rotate-right' : 'fa-redo'}"></i>
                    ${isAbandoned ? 'Reactivate' : 'Redrive'}
                </button>
                ${isAbandoned ? '' : `
                <button class="btn-abandon btn-sm" onclick="window._dlq.abandonOne('${esc(r.ID)}')"
                    style="margin-left:8px;">
                    <i class="fas fa-ban"></i> Abandon
                </button>`}
            </div>
        `;
    }

    // ─── Toast notification ───────────────────────────────────────────────────
    function showToast(msg, type = 'info') {
        const colours = { success: '#16a34a', error: '#dc2626', warning: '#d97706', info: '#2563eb' };
        const toast = document.createElement('div');
        toast.style.cssText =
            `position:fixed; bottom:20px; right:20px; background:${colours[type]};` +
            `color:#fff; padding:10px 16px; border-radius:8px; font-size:13px;` +
            `box-shadow:0 4px 16px rgba(0,0,0,0.2); z-index:9999; animation:fadeIn 0.2s ease;`;
        toast.textContent = msg;
        document.body.appendChild(toast);
        setTimeout(() => toast.remove(), 3500);
    }

    // ─── Public API ───────────────────────────────────────────────────────────
    window._dlq = {
        load,
        filterByStatus,
        toggleRow,
        toggleAll,
        clearSelection,
        openRedriveModal,
        closeRedriveModal,
        selectMode,
        selectModeDiv,
        confirmRedrive,
        confirmSchedule,
        toggleSchedulePanel,
        applyPreset: _applyPreset,
        bulkRedrive,
        bulkSchedule,
        bulkAbandon,
        abandonOne,
        openDrawer,
        closeDrawer,
    };

    // ─── Init ─────────────────────────────────────────────────────────────────
    document.addEventListener('DOMContentLoaded', () => {
        _applyURLPreFilter();
        _syncStatTileHighlight();
        loadInterfaces().then(() => load());
    });
})();
