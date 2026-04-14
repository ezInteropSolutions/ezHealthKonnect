// user-management.js — Enterprise User Management App (v10)

(function () {
    'use strict';

    // ─── State ────────────────────────────────────────────────────────────
    const state = {
        users: [],
        filtered: [],
        sortKey: 'name',
        sortDir: 'asc',
        page: 1,
        pageSize: 25,
        selected: new Set(),
        searchTerm: '',
        filterRole: '',
        filterStatus: '',
        activeTab: 'users',
        drawer: { open: false, userId: null, activeTab: 'profile' },
        audit: { page: 1, total: 0 }
    };

    // ─── Helpers ──────────────────────────────────────────────────────────
    const esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

    function fmt(d) {
        if (!d) return '—';
        const dt = new Date(d);
        return isNaN(dt) ? '—' : dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    function fmtDate(d) {
        if (!d) return '';
        const dt = new Date(d);
        return isNaN(dt) ? '' : dt.toISOString().slice(0, 10);
    }

    function initials(u) {
        const fn = u.first_name || u.name || '';
        const ln = u.last_name || '';
        return ((fn[0] || '') + (ln[0] || fn[1] || '')).toUpperCase() || '?';
    }

    function roleBadge(role) {
        const cls = { admin: 'role-admin', operator: 'role-operator', viewer: 'role-viewer', user: 'role-user' }[role] || 'role-user';
        return `<span class="role-badge ${cls}">${esc(role)}</span>`;
    }

    function statusBadge(status) {
        const cls = { active: 'status-active', inactive: 'status-inactive', suspended: 'status-suspended', pending: 'status-pending' }[status] || '';
        return `<span class="status-badge ${cls}">${esc(status)}</span>`;
    }

    function riskBadge(level) {
        const cls = { low: 'risk-low', medium: 'risk-medium', high: 'risk-high', critical: 'risk-critical' }[level] || '';
        return `<span class="risk-badge ${cls}">${esc(level || '—')}</span>`;
    }

    function showAlert(msg, type = 'success') {
        const el = document.getElementById('um-alert');
        if (!el) return;
        el.innerHTML = `<div class="um-alert um-alert--${type}">${esc(msg)}</div>`;
        setTimeout(() => { el.innerHTML = ''; }, 4000);
    }

    async function api(method, path, body) {
        const opts = { method, headers: { 'Content-Type': 'application/json' }, credentials: 'include' };
        if (body) opts.body = JSON.stringify(body);
        const r = await fetch(path, opts);
        const data = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(data.message || `HTTP ${r.status}`);
        return data;
    }

    // ─── Stats ────────────────────────────────────────────────────────────
    async function loadStats() {
        try {
            const s = await api('GET', '/api/users/stats');
            document.getElementById('stat-total').textContent    = s.total    ?? 0;
            document.getElementById('stat-active').textContent   = s.active   ?? 0;
            document.getElementById('stat-inactive').textContent = s.inactive ?? 0;
            document.getElementById('stat-locked').textContent   = s.locked   ?? 0;
            document.getElementById('stat-admins').textContent   = s.admins   ?? 0;
        } catch (e) {
            console.error('Stats load failed:', e.message);
            ['stat-total','stat-active','stat-inactive','stat-locked','stat-admins']
                .forEach(id => { const el = document.getElementById(id); if (el) el.textContent = '!'; });
        }
    }

    // ─── Users List (server-side search / filter / sort / page) ──────────
    let _searchDebounce = null;

    function buildUsersURL() {
        const p = new URLSearchParams();
        if (state.searchTerm) p.set('search',  state.searchTerm);
        if (state.filterRole)   p.set('role',    state.filterRole);
        if (state.filterStatus) p.set('status',  state.filterStatus);
        p.set('sortBy',  state.sortKey);
        p.set('sortDir', state.sortDir);
        p.set('page',    state.page);
        p.set('limit',   state.pageSize);
        return `/api/users?${p.toString()}`;
    }

    async function loadUsers() {
        const tbody = document.getElementById('um-tbody');
        tbody.innerHTML = `<tr><td colspan="8" class="um-loading"><i class="fas fa-spinner fa-spin"></i> Loading users…</td></tr>`;
        try {
            const res = await api('GET', buildUsersURL());
            // API now returns { users, total, page, pages } but handle old array response too
            if (Array.isArray(res)) {
                state.users    = res;
                state.filtered = res;
                state.serverTotal = res.length;
                state.serverPages = 1;
            } else {
                state.users    = res.users || [];
                state.filtered = state.users;
                state.serverTotal = res.total || state.users.length;
                state.serverPages = res.pages || 1;
            }
            renderTable();
        } catch (e) {
            tbody.innerHTML = `<tr><td colspan="8" class="um-error"><i class="fas fa-exclamation-circle"></i> Failed to load users: ${esc(e.message)}</td></tr>`;
        }
    }

    // applyFilter resets to page 1 and reloads from server
    function applyFilter() {
        state.page = 1;
        state.selected.clear();
        loadUsers();
    }

    // Debounced search — fires 350ms after the user stops typing
    function applyFilterDebounced() {
        clearTimeout(_searchDebounce);
        _searchDebounce = setTimeout(applyFilter, 350);
    }

    function renderTable() {
        const tbody = document.getElementById('um-tbody');
        const { users, selected } = state;

        if (!users.length) {
            tbody.innerHTML = `<tr><td colspan="8" class="um-empty"><i class="fas fa-users-slash"></i><br>No users found</td></tr>`;
            renderPagination();
            return;
        }

        tbody.innerHTML = users.map(u => {
            const name = u.first_name ? `${esc(u.first_name)} ${esc(u.last_name || '')}`.trim() : esc(u.name || '—');
            const login = u.last_login_at || u.lastLogin;
            const dept = u.department || '—';
            return `
            <tr class="um-row ${selected.has(u.id) ? 'um-row--selected' : ''}" data-id="${esc(u.id)}">
                <td class="um-td-check"><input type="checkbox" class="um-row-check" data-id="${esc(u.id)}" ${selected.has(u.id) ? 'checked' : ''}></td>
                <td class="um-td-name">
                    <div class="um-user-cell">
                        <div class="um-user-avatar-sm">${initials(u)}</div>
                        <span class="um-user-name-link" data-id="${esc(u.id)}">${name}</span>
                    </div>
                </td>
                <td>${esc(u.email)}</td>
                <td>
                    <select class="um-role-select role-badge role-${esc(u.role)}" data-id="${esc(u.id)}" title="Change role">
                        <option value="admin"    ${u.role==='admin'    ?'selected':''}>admin</option>
                        <option value="operator" ${u.role==='operator' ?'selected':''}>operator</option>
                        <option value="viewer"   ${u.role==='viewer'   ?'selected':''}>viewer</option>
                        <option value="user"     ${u.role==='user'     ?'selected':''}>user</option>
                    </select>
                </td>
                <td>${statusBadge(u.status)}</td>
                <td>${esc(dept)}</td>
                <td>${fmt(login)}</td>
                <td class="um-td-actions">
                    <button class="um-action-btn" title="View / Edit" data-action="edit" data-id="${esc(u.id)}"><i class="fas fa-pen-to-square"></i></button>
                    <button class="um-action-btn um-action-btn--danger" title="Delete" data-action="delete" data-id="${esc(u.id)}"><i class="fas fa-trash"></i></button>
                </td>
            </tr>`;
        }).join('');

        renderPagination();
        updateBulkBar();
    }

    function renderPagination() {
        const total = state.serverTotal || state.users.length;
        const pages = state.serverPages || Math.ceil(total / state.pageSize) || 1;
        const start = (state.page - 1) * state.pageSize + 1;
        const end   = Math.min(state.page * state.pageSize, total);
        document.getElementById('um-page-info').textContent = total ? `${start}–${end} of ${total} users` : '0 users';
        document.getElementById('um-page-num').textContent  = `${state.page} / ${pages}`;
        document.getElementById('um-prev-page').disabled    = state.page <= 1;
        document.getElementById('um-next-page').disabled    = state.page >= pages;
    }

    // ─── Selection & Bulk ─────────────────────────────────────────────────
    function updateBulkBar() {
        const bar = document.getElementById('um-bulk-bar');
        const count = state.selected.size;
        if (count > 0) {
            document.getElementById('um-bulk-count').textContent = `${count} selected`;
            bar.style.display = 'flex';
        } else {
            bar.style.display = 'none';
        }
    }

    async function handleBulkAction(action) {
        if (!state.selected.size) return;
        const verb = { activate: 'activate', deactivate: 'deactivate', delete: 'permanently delete' }[action] || action;
        if (!await AppDialogs.confirm(`${verb} ${state.selected.size} selected user(s)?`, { title: 'Bulk Action', type: 'warning', confirmText: 'Confirm' })) return;
        try {
            const r = await api('POST', '/api/users/bulk', { action, userIds: [...state.selected] });
            showAlert(r.message);
            state.selected.clear();
            await reload();
        } catch (e) {
            showAlert(e.message, 'error');
        }
    }

    // ─── Create User ──────────────────────────────────────────────────────
    async function handleCreate(form) {
        const fd = new FormData(form);
        const firstName = fd.get('first_name') || '';
        const lastName = fd.get('last_name') || '';
        const name = [firstName, lastName].filter(Boolean).join(' ');
        try {
            await api('POST', '/api/users', {
                name,
                email: fd.get('email'),
                password: fd.get('password'),
                role: fd.get('role')
            });
            showAlert('User created successfully');
            closeModal('modal-create');
            form.reset();
            await reload();
        } catch (e) {
            showAlert(e.message, 'error');
        }
    }

    // ─── Invite User ──────────────────────────────────────────────────────
    async function handleInvite(form) {
        const fd = new FormData(form);
        try {
            const r = await api('POST', '/api/users/invite', { email: fd.get('email'), role: fd.get('role') });
            document.getElementById('invite-result').style.display = 'block';
            document.getElementById('invite-link-input').value = r.inviteLink;
            const exp = r.expiresAt ? new Date(r.expiresAt).toLocaleString() : '';
            document.getElementById('invite-expiry-text').textContent = exp ? `Expires: ${exp}` : '';
            form.style.display = 'none';
            await reload();
        } catch (e) {
            showAlert(e.message, 'error');
        }
    }

    // ─── Delete User ──────────────────────────────────────────────────────
    async function deleteUser(id) {
        const u = state.users.find(x => x.id === id);
        if (!u) return;
        const name = u.first_name ? `${u.first_name} ${u.last_name || ''}`.trim() : (u.name || u.email);
        if (!await AppDialogs.confirm(`Delete user "<b>${name}</b>"? This cannot be undone.`, { title: 'Delete User', type: 'danger', confirmText: 'Delete' })) return;
        try {
            await api('DELETE', `/api/users/${id}`);
            showAlert('User deleted');
            await reload();
        } catch (e) {
            showAlert(e.message, 'error');
        }
    }

    // ─── Drawer ───────────────────────────────────────────────────────────
    async function openDrawer(userId) {
        state.drawer.userId = userId;
        state.drawer.open = true;
        const drawer = document.getElementById('um-drawer');
        const overlay = document.getElementById('um-drawer-overlay');
        drawer.style.display = 'flex';
        overlay.style.display = 'block';
        requestAnimationFrame(() => drawer.classList.add('um-drawer--open'));
        switchDrawerTab('profile');
        await loadDrawerUser(userId);
    }

    function closeDrawer() {
        const drawer = document.getElementById('um-drawer');
        drawer.classList.remove('um-drawer--open');
        setTimeout(() => {
            drawer.style.display = 'none';
            document.getElementById('um-drawer-overlay').style.display = 'none';
        }, 280);
        state.drawer.open = false;
        state.drawer.userId = null;
    }

    function switchDrawerTab(tab) {
        state.drawer.activeTab = tab;
        document.querySelectorAll('.um-drawer-tab').forEach(b => b.classList.toggle('active', b.dataset.dtab === tab));
        document.querySelectorAll('.um-drawer-tab-content').forEach(p => p.style.display = 'none');
        const panel = document.getElementById(`dtab-${tab}`);
        if (panel) panel.style.display = 'block';
        if (tab === 'activity' && state.drawer.userId) loadActivity(state.drawer.userId);
    }

    async function loadDrawerUser(userId) {
        try {
            const u = await api('GET', `/api/users/${userId}`);
            populateDrawer(u);
        } catch (e) {
            showAlert('Failed to load user details: ' + e.message, 'error');
        }
    }

    function populateDrawer(u) {
        document.getElementById('drawer-avatar').textContent = initials(u);
        document.getElementById('drawer-name').textContent = [u.first_name, u.last_name].filter(Boolean).join(' ') || u.email;
        document.getElementById('drawer-role-badge').className = `role-badge role-${u.role}`;
        document.getElementById('drawer-role-badge').textContent = u.role;
        document.getElementById('drawer-status-badge').className = `status-badge status-${u.status}`;
        document.getElementById('drawer-status-badge').textContent = u.status;

        // Profile
        document.getElementById('dp-first-name').value = u.first_name || '';
        document.getElementById('dp-last-name').value = u.last_name || '';
        document.getElementById('dp-email').value = u.email || '';
        document.getElementById('dp-phone').value = u.phone || '';
        document.getElementById('dp-org').value = u.organization || '';
        document.getElementById('dp-dept').value = u.department || '';
        document.getElementById('dp-title').value = u.job_title || '';
        document.getElementById('dp-timezone').value = u.timezone || 'UTC';
        document.getElementById('dp-locale').value = u.locale || 'en-US';

        // Security
        document.getElementById('ds-role').value = u.role || 'user';
        document.getElementById('ds-status').value = u.status || 'active';
        document.getElementById('ds-password').value = '';
        document.getElementById('ds-attempts').textContent = u.login_attempts ?? '0';
        document.getElementById('ds-locked').textContent = u.locked_until ? fmt(u.locked_until) : 'Not locked';
        document.getElementById('ds-last-login').textContent = fmt(u.last_login_at);
        document.getElementById('ds-login-ip').textContent = u.last_login_ip || '—';
        document.getElementById('ds-force-reset').textContent = u.force_password_reset ? 'Yes' : 'No';

        // Compliance
        document.getElementById('dc-consent').checked = !!u.data_consent_given;
        document.getElementById('dc-consent-date').value = fmtDate(u.data_consent_date);
        document.getElementById('dc-retention').value = fmtDate(u.data_retention_until);
        document.getElementById('dc-anon').checked = !!u.data_anonymized;
        const gdprRequested = !!u.gdpr_delete_requested;
        const gdprStatusEl = document.getElementById('dc-gdpr-status');
        gdprStatusEl.textContent = gdprRequested ? 'Requested' : 'Not requested';
        gdprStatusEl.className = `status-badge ${gdprRequested ? 'status-suspended' : 'status-active'}`;
        const gdprDateRow = document.getElementById('dc-gdpr-date-row');
        if (gdprRequested && u.gdpr_delete_requested_at) {
            gdprDateRow.style.display = 'block';
            document.getElementById('dc-gdpr-date').value = fmt(u.gdpr_delete_requested_at);
        } else {
            gdprDateRow.style.display = 'none';
        }
        const gdprBtn = document.getElementById('dc-btn-gdpr');
        if (gdprBtn) gdprBtn.disabled = gdprRequested;
    }

    async function loadActivity(userId) {
        const list = document.getElementById('activity-list');
        list.innerHTML = '<div class="um-loading"><i class="fas fa-spinner fa-spin"></i> Loading…</div>';
        try {
            const logs = await api('GET', `/api/users/${userId}/activity`);
            if (!logs.length) { list.innerHTML = '<div class="um-empty">No activity recorded yet</div>'; return; }
            list.innerHTML = logs.map(log => `
                <div class="um-activity-item">
                    <div class="um-activity-icon ${riskClass(log.risk_level)}"><i class="fas ${actionIcon(log.action)}"></i></div>
                    <div class="um-activity-detail">
                        <div class="um-activity-action">${esc(log.action)}</div>
                        <div class="um-activity-meta">${fmt(log.created_at)} &bull; IP: ${esc(log.ip_address || '—')}</div>
                    </div>
                    <div>${riskBadge(log.risk_level)}</div>
                </div>`).join('');
        } catch (e) {
            list.innerHTML = `<div class="um-error">${esc(e.message)}</div>`;
        }
    }

    function riskClass(level) {
        return { low: 'risk-icon-low', medium: 'risk-icon-medium', high: 'risk-icon-high', critical: 'risk-icon-critical' }[level] || '';
    }

    function actionIcon(action) {
        if (!action) return 'fa-circle';
        if (action.includes('LOGIN')) return 'fa-right-to-bracket';
        if (action.includes('DELETE')) return 'fa-trash';
        if (action.includes('UPDATE') || action.includes('UPDATED')) return 'fa-pen';
        if (action.includes('CREATE')) return 'fa-plus';
        if (action.includes('GDPR')) return 'fa-shield-halved';
        if (action.includes('UNLOCK')) return 'fa-lock-open';
        if (action.includes('BULK')) return 'fa-layer-group';
        return 'fa-circle-dot';
    }

    async function saveProfile() {
        const form = document.getElementById('drawer-profile-form');
        const fd = new FormData(form);
        const body = {};
        fd.forEach((v, k) => { if (v !== '') body[k] = v; });
        try {
            await api('PUT', `/api/users/${state.drawer.userId}/profile`, body);
            showAlert('Profile saved');
            await reload();
            await loadDrawerUser(state.drawer.userId);
        } catch (e) { showAlert(e.message, 'error'); }
    }

    async function saveSecurity() {
        const body = {
            role: document.getElementById('ds-role').value,
            status: document.getElementById('ds-status').value
        };
        const pw = document.getElementById('ds-password').value;
        if (pw) {
            if (pw.length < 8) { showAlert('Password must be at least 8 characters', 'error'); return; }
            body.password = pw;
        }
        try {
            await api('PUT', `/api/users/${state.drawer.userId}/profile`, body);
            showAlert('Security settings saved');
            await reload();
            await loadDrawerUser(state.drawer.userId);
        } catch (e) { showAlert(e.message, 'error'); }
    }

    async function saveCompliance() {
        const body = {
            data_consent_given: document.getElementById('dc-consent').checked,
            data_consent_date: document.getElementById('dc-consent-date').value || null,
            data_retention_until: document.getElementById('dc-retention').value || null,
            data_anonymized: document.getElementById('dc-anon').checked
        };
        try {
            await api('PUT', `/api/users/${state.drawer.userId}/compliance`, body);
            showAlert('Compliance fields saved');
        } catch (e) { showAlert(e.message, 'error'); }
    }

    async function unlockUser() {
        try {
            const r = await api('POST', `/api/users/${state.drawer.userId}/unlock`);
            showAlert(r.message);
            await loadDrawerUser(state.drawer.userId);
            await loadStats();
        } catch (e) { showAlert(e.message, 'error'); }
    }

    async function forceReset() {
        try {
            const r = await api('POST', `/api/users/${state.drawer.userId}/force-reset`);
            showAlert(r.message);
            await loadDrawerUser(state.drawer.userId);
        } catch (e) { showAlert(e.message, 'error'); }
    }

    async function gdprRequest() {
        if (!await AppDialogs.confirm('Flag this user for GDPR deletion? This will be logged and is irreversible.', { title: 'GDPR Deletion Request', type: 'danger', confirmText: 'Flag for Deletion' })) return;
        try {
            const r = await api('POST', `/api/users/${state.drawer.userId}/gdpr-request`);
            showAlert(r.message);
            await loadDrawerUser(state.drawer.userId);
        } catch (e) { showAlert(e.message, 'error'); }
    }

    // ─── Audit Logs ───────────────────────────────────────────────────────
    async function loadAuditLogs(page = 1) {
        const action = document.getElementById('audit-filter-action').value;
        const risk = document.getElementById('audit-filter-risk').value;
        const from = document.getElementById('audit-from').value;
        const to = document.getElementById('audit-to').value;

        const params = new URLSearchParams({ page, limit: 50 });
        if (action) params.append('action', action);
        if (risk) params.append('riskLevel', risk);
        if (from) params.append('from', from);
        if (to) params.append('to', to);

        const tbody = document.getElementById('audit-tbody');
        tbody.innerHTML = `<tr><td colspan="7" class="um-loading"><i class="fas fa-spinner fa-spin"></i> Loading…</td></tr>`;

        try {
            const r = await api('GET', `/api/audit-logs?${params}`);
            state.audit.page = page;
            state.audit.total = r.total;

            if (!r.logs || !r.logs.length) {
                tbody.innerHTML = '<tr><td colspan="7" class="um-empty">No audit events found</td></tr>';
            } else {
                tbody.innerHTML = r.logs.map(log => `
                    <tr>
                        <td class="um-td-mono">${fmt(log.created_at)}</td>
                        <td class="um-td-mono" style="font-size:0.78rem">${esc((log.user_id || '').slice(0, 8))}…</td>
                        <td class="um-action-cell">${esc(log.action)}</td>
                        <td>${esc(log.entity_type || '—')}</td>
                        <td class="um-td-mono">${esc(log.ip_address || '—')}</td>
                        <td>${statusBadge(log.result || '—')}</td>
                        <td>${riskBadge(log.risk_level)}</td>
                    </tr>`).join('');
            }

            document.getElementById('audit-page-info').textContent = `${r.total} total events`;
            document.getElementById('audit-page-num').textContent = `${r.page} / ${r.totalPages}`;
            document.getElementById('audit-prev').disabled = r.page <= 1;
            document.getElementById('audit-next').disabled = r.page >= r.totalPages;

            const exportParams = new URLSearchParams(params);
            exportParams.set('export', 'csv');
            exportParams.set('limit', '500');
            document.getElementById('audit-export-csv').href = `/api/audit-logs?${exportParams}`;
        } catch (e) {
            tbody.innerHTML = `<tr><td colspan="7" class="um-error">${esc(e.message)}</td></tr>`;
        }
    }

    // ─── Modal helpers ────────────────────────────────────────────────────
    function openModal(id) { const el = document.getElementById(id); if (el) el.style.display = 'flex'; }
    function closeModal(id) { const el = document.getElementById(id); if (el) el.style.display = 'none'; }

    // ─── Password strength ────────────────────────────────────────────────
    function pwStrength(pw) {
        if (!pw) return { score: 0, label: '', cls: '' };
        let score = 0;
        if (pw.length >= 8) score++;
        if (pw.length >= 12) score++;
        if (/[A-Z]/.test(pw)) score++;
        if (/[0-9]/.test(pw)) score++;
        if (/[^A-Za-z0-9]/.test(pw)) score++;
        const labels = ['', 'Very Weak', 'Weak', 'Fair', 'Strong', 'Very Strong'];
        const classes = ['', 'pw-very-weak', 'pw-weak', 'pw-fair', 'pw-strong', 'pw-very-strong'];
        return { score, label: labels[score] || '', cls: classes[score] || '' };
    }

    function attachPwStrength(inputEl, barEl) {
        if (!inputEl || !barEl) return;
        inputEl.addEventListener('input', () => {
            const { score, label, cls } = pwStrength(inputEl.value);
            barEl.innerHTML = inputEl.value
                ? `<div class="pw-bar"><div class="pw-fill pw-fill--${score}" style="width:${score * 20}%"></div></div><span class="${cls}">${label}</span>`
                : '';
        });
    }

    // ─── Tab switching ────────────────────────────────────────────────────
    function switchTab(tab) {
        state.activeTab = tab;
        document.querySelectorAll('.um-tab').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));
        ['users', 'audit', 'roles'].forEach(t => {
            const panel = document.getElementById(`tab-${t}`);
            if (panel) panel.style.display = t === tab ? 'block' : 'none';
        });
    }

    // ─── Reload ───────────────────────────────────────────────────────────
    async function reload() {
        await Promise.all([loadStats(), loadUsers()]);
    }

    // ─── Event Wiring ─────────────────────────────────────────────────────
    function wire() {
        // Main tabs
        document.querySelectorAll('.um-tab').forEach(btn => btn.addEventListener('click', () => switchTab(btn.dataset.tab)));

        // Search — debounced server call
        document.getElementById('um-search').addEventListener('input', e => {
            state.searchTerm = e.target.value;
            applyFilterDebounced();
        });

        // Role / status filter — immediate server call
        document.getElementById('um-filter-role').addEventListener('change', e => { state.filterRole = e.target.value; applyFilter(); });
        document.getElementById('um-filter-status').addEventListener('change', e => { state.filterStatus = e.target.value; applyFilter(); });

        // Sort headers — server-side sort
        document.querySelectorAll('#um-table th.sortable').forEach(th => th.addEventListener('click', () => {
            const key = th.dataset.sort;
            state.sortDir = state.sortKey === key && state.sortDir === 'asc' ? 'desc' : 'asc';
            state.sortKey = key;
            document.querySelectorAll('#um-table th.sortable i').forEach(i => i.className = 'fas fa-sort');
            th.querySelector('i').className = `fas fa-sort-${state.sortDir === 'asc' ? 'up' : 'down'}`;
            state.page = 1;
            loadUsers();
        }));

        // Pagination — navigate server pages
        document.getElementById('um-prev-page').addEventListener('click', () => {
            if (state.page > 1) { state.page--; loadUsers(); }
        });
        document.getElementById('um-next-page').addEventListener('click', () => {
            const pages = state.serverPages || 1;
            if (state.page < pages) { state.page++; loadUsers(); }
        });
        document.getElementById('um-page-size').addEventListener('change', e => { state.pageSize = parseInt(e.target.value, 10); state.page = 1; loadUsers(); });

        // Select all — selects all users on the current server page
        document.getElementById('um-select-all').addEventListener('change', e => {
            state.users.forEach(u => e.target.checked ? state.selected.add(u.id) : state.selected.delete(u.id));
            renderTable();
        });

        // Row delegation
        document.getElementById('um-tbody').addEventListener('click', e => {
            const check = e.target.closest('.um-row-check');
            if (check) {
                const id = check.dataset.id;
                check.checked ? state.selected.add(id) : state.selected.delete(id);
                updateBulkBar();
                check.closest('tr').classList.toggle('um-row--selected', check.checked);
                return;
            }
            const link = e.target.closest('.um-user-name-link');
            if (link) { openDrawer(link.dataset.id); return; }
            const btn = e.target.closest('[data-action]');
            if (btn) {
                if (btn.dataset.action === 'edit') openDrawer(btn.dataset.id);
                if (btn.dataset.action === 'delete') deleteUser(btn.dataset.id);
            }
        });

        // Inline role change
        document.getElementById('um-tbody').addEventListener('change', async e => {
            const sel = e.target.closest('.um-role-select');
            if (!sel) return;
            const userId = sel.dataset.id;
            const newRole = sel.value;
            const prevRole = [...sel.options].find(o => !o.selected && o.dataset?.prev)?.value;
            // Update colour immediately
            sel.className = `um-role-select role-badge role-${newRole}`;
            try {
                await api('PUT', `/api/users/${userId}/profile`, { role: newRole });
                showAlert(`Role updated to "${newRole}"`);
                // Keep local state in sync
                const u = state.users.find(x => x.id === userId);
                if (u) u.role = newRole;
            } catch (err) {
                showAlert(err.message, 'error');
                // Revert visual
                await loadUsers();
            }
        });

        // Bulk bar
        document.querySelectorAll('[data-bulk]').forEach(btn => btn.addEventListener('click', () => handleBulkAction(btn.dataset.bulk)));
        document.getElementById('um-bulk-cancel').addEventListener('click', () => { state.selected.clear(); renderTable(); });

        // Toolbar
        document.getElementById('btnRefresh').addEventListener('click', reload);
        document.getElementById('btnCreateUser').addEventListener('click', () => openModal('modal-create'));
        document.getElementById('btnInviteUser').addEventListener('click', () => {
            document.getElementById('invite-result').style.display = 'none';
            document.getElementById('form-invite').style.display = 'block';
            document.getElementById('form-invite').reset();
            openModal('modal-invite');
        });

        // Create form
        document.getElementById('form-create').addEventListener('submit', async e => { e.preventDefault(); await handleCreate(e.target); });

        // Invite form
        document.getElementById('form-invite').addEventListener('submit', async e => { e.preventDefault(); await handleInvite(e.target); });

        // Copy invite link
        document.getElementById('btn-copy-invite').addEventListener('click', () => {
            const inp = document.getElementById('invite-link-input');
            inp.select();
            try { document.execCommand('copy'); } catch (_) { navigator.clipboard?.writeText(inp.value); }
            showAlert('Invite link copied to clipboard!');
        });

        // Modal close
        document.querySelectorAll('[data-close]').forEach(btn => btn.addEventListener('click', () => closeModal(btn.dataset.close)));
        document.querySelectorAll('.um-modal-overlay').forEach(overlay => overlay.addEventListener('click', e => { if (e.target === overlay) overlay.style.display = 'none'; }));

        // Drawer
        document.getElementById('um-drawer-close').addEventListener('click', closeDrawer);
        document.getElementById('um-drawer-overlay').addEventListener('click', closeDrawer);
        document.querySelectorAll('.um-drawer-tab').forEach(btn => btn.addEventListener('click', () => switchDrawerTab(btn.dataset.dtab)));

        // Drawer forms
        document.getElementById('drawer-profile-form').addEventListener('submit', async e => { e.preventDefault(); await saveProfile(); });
        document.getElementById('drawer-security-form').addEventListener('submit', async e => { e.preventDefault(); await saveSecurity(); });
        document.getElementById('drawer-compliance-form').addEventListener('submit', async e => { e.preventDefault(); await saveCompliance(); });
        document.getElementById('ds-btn-unlock').addEventListener('click', unlockUser);
        document.getElementById('ds-btn-force-reset').addEventListener('click', forceReset);
        document.getElementById('dc-btn-gdpr').addEventListener('click', gdprRequest);

        // Audit log controls
        document.getElementById('audit-apply').addEventListener('click', () => loadAuditLogs(1));
        document.getElementById('audit-prev').addEventListener('click', () => loadAuditLogs(state.audit.page - 1));
        document.getElementById('audit-next').addEventListener('click', () => loadAuditLogs(state.audit.page + 1));

        // Password strength
        attachPwStrength(document.querySelector('#form-create [name="password"]'), document.getElementById('create-pw-strength'));
        attachPwStrength(document.getElementById('ds-password'), document.getElementById('ds-pw-strength'));

        // Clock
        const clockEl = document.getElementById('currentTime');
        function tick() { if (clockEl) clockEl.textContent = new Date().toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }); }
        tick();
        setInterval(tick, 30000);
    }

    // ─── Auth guard ───────────────────────────────────────────────────────
    async function checkAuth() {
        try {
            const r = await fetch('/api/auth/session', { credentials: 'include' });
            if (!r.ok) { window.location.href = '/'; return false; }
            const data = await r.json();
            if (!data.authenticated) { window.location.href = '/'; return false; }
            if (data.user.role !== 'admin') {
                document.querySelector('.main-content').innerHTML = '<div style="padding:3rem;text-align:center;color:#888"><i class="fas fa-lock fa-3x"></i><p style="margin-top:1rem">Admin access required.</p></div>';
                return false;
            }
            const adminSec = document.getElementById('adminSection');
            if (adminSec) adminSec.style.display = 'block';
            const av = document.getElementById('userAvatar');
            const un = document.getElementById('userName');
            const ur = document.getElementById('userRole');
            if (av) av.textContent = (data.user.name || data.user.email || 'A')[0].toUpperCase();
            if (un) un.textContent = data.user.name || data.user.email;
            if (ur) ur.textContent = data.user.role.toUpperCase();
            return true;
        } catch (e) {
            window.location.href = '/';
            return false;
        }
    }

    // ─── Bootstrap ────────────────────────────────────────────────────────
    async function init() {
        const ok = await checkAuth();
        if (!ok) return;
        wire();
        await reload();
    }

    window.logout = async function () {
        await fetch('/api/auth/logout', { method: 'POST', credentials: 'include' });
        window.location.href = '/';
    };

    document.addEventListener('DOMContentLoaded', init);
})();
