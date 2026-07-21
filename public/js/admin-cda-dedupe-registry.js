// public/js/admin-cda-dedupe-registry.js
// CDA Dedupe Registry admin viewer — IIFE module.
// Mirrors admin-dlq.js's fetch/auth pattern for a much simpler read+purge UI.
(function () {
    'use strict';

    let _rows = [];
    let _currentInterfaceId = '';
    let _currentPatientKey = '';

    const esc = s => String(s ?? '').replace(/[&<>"']/g, c =>
        ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
    const el = id => document.getElementById(id);

    function fmtDate(iso) {
        if (!iso) return '—';
        const d = new Date(iso);
        return d.toLocaleString(undefined, { year: 'numeric', month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    }

    function _authHeaders() {
        const token = localStorage.getItem('accessToken');
        const h = { 'Content-Type': 'application/json' };
        if (token) h['Authorization'] = 'Bearer ' + token;
        return h;
    }

    async function loadInterfaces() {
        try {
            const res = await fetch('/api/interfaces', { credentials: 'include', headers: _authHeaders() });
            const body = await res.json();
            const list = body.data || body.interfaces || body || [];
            const select = el('searchInterface');
            select.innerHTML = '<option value="">Select an interface…</option>' +
                list.map(i => `<option value="${esc(i.id)}">${esc(i.name || i.id)}</option>`).join('');
        } catch (_) { /* non-fatal — user can still paste a patient key once an interface loads */ }
    }

    function renderRows(rows) {
        const tbody = el('registryTableBody');
        if (!rows.length) {
            tbody.innerHTML = `<tr><td colspan="7" class="empty-state"><i class="fas fa-circle-check" style="color:#22c55e;"></i><br>No matching registry entries.</td></tr>`;
            return;
        }
        tbody.innerHTML = rows.map(r => `
            <tr>
                <td>${esc(r.sectionKey)}</td>
                <td class="mono">${esc(r.identityKey)}</td>
                <td>${fmtDate(r.firstSeenAt)}</td>
                <td>${fmtDate(r.lastSeenAt)}</td>
                <td>${esc(r.seenCount)}</td>
                <td class="mono">${esc(r.firstSeenMessageId || '—')}</td>
                <td class="mono">${esc(r.lastSeenMessageId || '—')}</td>
            </tr>`).join('');
    }

    async function load() {
        const interfaceId = el('searchInterface').value;
        const patientKey = el('searchPatientKey').value.trim();
        _currentInterfaceId = interfaceId;
        _currentPatientKey = patientKey;

        const purgeBtn = el('purgeBtn');
        purgeBtn.disabled = !(interfaceId && patientKey);

        if (!interfaceId) {
            el('registryTableBody').innerHTML = `<tr><td colspan="7" class="empty-state"><i class="fas fa-fingerprint"></i><br>Select an interface and search to view registry entries.</td></tr>`;
            return;
        }

        el('registryTableBody').innerHTML = `<tr class="loading-row"><td colspan="7"><i class="fas fa-spinner fa-spin"></i> Loading…</td></tr>`;
        try {
            const params = new URLSearchParams({ interfaceId });
            if (patientKey) params.set('patientKey', patientKey);
            const res = await fetch('/api/cda/dedupe/registry?' + params.toString(), {
                credentials: 'include',
                headers: _authHeaders(),
            });
            const body = await res.json();
            if (!res.ok || !body.success) {
                el('registryTableBody').innerHTML = `<tr><td colspan="7" class="empty-state">${esc(body.error || ('Request failed (' + res.status + ')'))}</td></tr>`;
                return;
            }
            _rows = body.data || [];
            renderRows(_rows);
        } catch (e) {
            el('registryTableBody').innerHTML = `<tr><td colspan="7" class="empty-state">Network error: ${esc(e.message)}</td></tr>`;
        }
    }

    // ─── Purge flow ───────────────────────────────────────────────────────────

    function openPurgeModal() {
        if (!_currentInterfaceId || !_currentPatientKey) return;
        el('purgePatientLabel').textContent = _currentPatientKey;
        el('purgeReason').value = '';
        el('purgeModal').classList.add('open');
    }

    function closePurgeModal() {
        el('purgeModal').classList.remove('open');
    }

    async function confirmPurge() {
        const reason = el('purgeReason').value.trim();
        if (!reason) {
            alert('A reason is required before purging.');
            return;
        }
        try {
            const res = await fetch('/api/cda/dedupe/registry', {
                method: 'DELETE',
                credentials: 'include',
                headers: _authHeaders(),
                body: JSON.stringify({
                    interfaceId: _currentInterfaceId,
                    patientKey: _currentPatientKey,
                    reason: reason,
                }),
            });
            const body = await res.json();
            if (!res.ok || !body.success) {
                alert('Purge failed: ' + (body.error || res.status));
                return;
            }
            closePurgeModal();
            alert(`Purged ${body.rowsDeleted} row(s) for this patient.`);
            load();
        } catch (e) {
            alert('Network error: ' + e.message);
        }
    }

    window._dedupeRegistry = { load, openPurgeModal, closePurgeModal, confirmPurge };

    window.addEventListener('load', function () {
        loadInterfaces();
    });
})();
