'use strict';

/**
 * GitPanel — floating Git integration panel for the pipeline builder.
 *
 * Attaches to the bottom-right corner of the screen. Shows:
 *   - Current branch indicator
 *   - Uncommitted-changes badge
 *   - Commit textarea + Commit button
 *   - Pull button
 *   - Changed-files list
 *   - Recent commit log (toggle)
 *   - Repository selector (if multiple repos configured)
 *
 * Usage:
 *   window.gitPanel = new GitPanel(interfaceId);
 */
class GitPanel {
    constructor(interfaceId) {
        this.interfaceId = interfaceId;
        this.repoId = null;
        this.repos = [];
        this.currentBranch = '—';
        this.statusFiles = [];
        this.commitLog = [];
        this.logVisible = false;
        this._el = null;       // root panel element

        this._init();
    }

    // ── Lifecycle ─────────────────────────────────────────────────────────────

    async _init() {
        this._injectStyles();
        this._buildDOM();
        await this._loadRepos();
    }

    _injectStyles() {
        if (document.getElementById('git-panel-styles')) return;
        const style = document.createElement('style');
        style.id = 'git-panel-styles';
        style.textContent = `
            #gitPanelRoot {
                position: fixed;
                bottom: 24px;
                right: 24px;
                z-index: 1200;
                width: 340px;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
                font-size: 13px;
                box-shadow: 0 8px 32px rgba(0,0,0,.18);
                border-radius: 10px;
                overflow: hidden;
                background: #fff;
                border: 1px solid #e2e8f0;
            }
            #gitPanelRoot .gp-header {
                background: linear-gradient(135deg,#1e3a8a,#2563eb);
                color: #fff;
                display: flex;
                align-items: center;
                gap: 8px;
                padding: 10px 14px;
                cursor: pointer;
                user-select: none;
            }
            #gitPanelRoot .gp-branch {
                font-weight: 600;
                font-size: 12px;
                flex: 1;
                overflow: hidden;
                text-overflow: ellipsis;
                white-space: nowrap;
            }
            #gitPanelRoot .gp-badge {
                background: #f59e0b;
                color: #fff;
                border-radius: 50%;
                width: 8px;
                height: 8px;
                display: inline-block;
                flex-shrink: 0;
            }
            #gitPanelRoot .gp-toggle-btn {
                background: none;
                border: none;
                color: rgba(255,255,255,.8);
                cursor: pointer;
                padding: 0;
                font-size: 14px;
                line-height: 1;
            }
            #gitPanelRoot .gp-toggle-btn:hover { color: #fff; }
            #gitPanelRoot .gp-body {
                display: none;
                max-height: 480px;
                overflow-y: auto;
            }
            #gitPanelRoot.gp-open .gp-body { display: block; }
            #gitPanelRoot .gp-section {
                padding: 12px 14px;
                border-bottom: 1px solid #f1f5f9;
            }
            #gitPanelRoot .gp-section:last-child { border-bottom: none; }
            #gitPanelRoot .gp-label {
                font-weight: 600;
                color: #374151;
                font-size: 11px;
                text-transform: uppercase;
                letter-spacing: .5px;
                margin-bottom: 6px;
            }
            #gitPanelRoot textarea {
                width: 100%;
                box-sizing: border-box;
                border: 1px solid #d1d5db;
                border-radius: 6px;
                padding: 7px 10px;
                font-size: 12px;
                resize: vertical;
                min-height: 52px;
                font-family: inherit;
                color: #1e293b;
            }
            #gitPanelRoot textarea:focus { outline: none; border-color: #3b82f6; }
            #gitPanelRoot .gp-actions {
                display: flex;
                gap: 6px;
                margin-top: 8px;
            }
            #gitPanelRoot .gp-btn {
                border: none;
                border-radius: 6px;
                padding: 6px 12px;
                font-size: 12px;
                font-weight: 600;
                cursor: pointer;
                display: flex;
                align-items: center;
                gap: 5px;
            }
            #gitPanelRoot .gp-btn-primary {
                background: #1e3a8a;
                color: #fff;
                flex: 1;
            }
            #gitPanelRoot .gp-btn-primary:hover { background: #1e40af; }
            #gitPanelRoot .gp-btn-primary:disabled { background: #93c5fd; cursor: not-allowed; }
            #gitPanelRoot .gp-btn-secondary {
                background: #f1f5f9;
                color: #374151;
            }
            #gitPanelRoot .gp-btn-secondary:hover { background: #e2e8f0; }
            #gitPanelRoot .gp-status-list { font-family: monospace; font-size: 11px; }
            #gitPanelRoot .gp-status-item {
                display: flex;
                gap: 8px;
                padding: 2px 0;
                color: #475569;
            }
            #gitPanelRoot .gp-status-item .gp-status-code {
                flex-shrink: 0;
                color: #f59e0b;
                font-weight: 700;
            }
            #gitPanelRoot .gp-status-item .gp-status-code.staged { color: #16a34a; }
            #gitPanelRoot .gp-log-item {
                padding: 5px 0;
                border-bottom: 1px solid #f8fafc;
                font-size: 11px;
            }
            #gitPanelRoot .gp-log-sha {
                font-family: monospace;
                background: #f1f5f9;
                padding: 1px 5px;
                border-radius: 3px;
                color: #3b82f6;
                font-size: 10px;
            }
            #gitPanelRoot .gp-log-msg { color: #1e293b; margin-top: 2px; }
            #gitPanelRoot .gp-log-meta { color: #94a3b8; font-size: 10px; }
            #gitPanelRoot .gp-toast {
                background: #1e293b;
                color: #fff;
                padding: 6px 12px;
                font-size: 11px;
                text-align: center;
            }
            #gitPanelRoot .gp-toast.success { background: #166534; }
            #gitPanelRoot .gp-toast.error   { background: #991b1b; }
            #gitPanelRoot select.gp-select {
                width: 100%;
                box-sizing: border-box;
                border: 1px solid #d1d5db;
                border-radius: 6px;
                padding: 5px 8px;
                font-size: 12px;
                color: #374151;
                background: #fff;
            }
            #gitPanelRoot .gp-log-toggle {
                background: none;
                border: none;
                color: #3b82f6;
                cursor: pointer;
                font-size: 11px;
                padding: 0;
                font-weight: 600;
            }
            #gitPanelRoot .gp-log-toggle:hover { text-decoration: underline; }
            #gitPanelRoot .gp-branch-row {
                display: flex;
                gap: 6px;
                align-items: center;
            }
            #gitPanelRoot .gp-branch-row select {
                flex: 1;
                border: 1px solid #d1d5db;
                border-radius: 6px;
                padding: 4px 8px;
                font-size: 12px;
            }
        `;
        document.head.appendChild(style);
    }

    _buildDOM() {
        const root = document.createElement('div');
        root.id = 'gitPanelRoot';
        root.innerHTML = `
            <div class="gp-header" id="gpHeader">
                <i class="fab fa-git-alt"></i>
                <span class="gp-branch" id="gpBranchName">Loading…</span>
                <span class="gp-badge" id="gpBadge" style="display:none" title="Uncommitted changes"></span>
                <button class="gp-toggle-btn" id="gpToggleBtn" title="Toggle Git panel">
                    <i class="fas fa-chevron-up" id="gpChevron"></i>
                </button>
            </div>
            <div class="gp-body" id="gpBody">
                <!-- Toast / status -->
                <div id="gpToast" style="display:none" class="gp-toast"></div>

                <!-- Repo selector (shown only if >1 repo) -->
                <div class="gp-section" id="gpRepoSection" style="display:none">
                    <div class="gp-label">Repository</div>
                    <select class="gp-select" id="gpRepoSelect" onchange="window.gitPanel._onRepoChange()"></select>
                </div>

                <!-- Branch selector -->
                <div class="gp-section" id="gpBranchSection" style="display:none">
                    <div class="gp-label">Branch</div>
                    <div class="gp-branch-row">
                        <select id="gpBranchSelect" onchange="window.gitPanel._onBranchChange()"></select>
                        <button class="gp-btn gp-btn-secondary" onclick="window.gitPanel._newBranch()" title="Create new branch">
                            <i class="fas fa-plus"></i>
                        </button>
                    </div>
                </div>

                <!-- Commit section -->
                <div class="gp-section">
                    <div class="gp-label">Commit Interface to Git</div>
                    <textarea id="gpCommitMsg" placeholder="Describe what changed…"></textarea>
                    <div class="gp-actions">
                        <button class="gp-btn gp-btn-primary" id="gpCommitBtn" onclick="window.gitPanel._commit()">
                            <i class="fas fa-code-commit"></i> Commit
                        </button>
                        <button class="gp-btn gp-btn-secondary" onclick="window.gitPanel._pull()" title="Pull latest from remote">
                            <i class="fas fa-download"></i> Pull
                        </button>
                        <button class="gp-btn gp-btn-secondary" onclick="window.gitPanel._refreshStatus()" title="Refresh status">
                            <i class="fas fa-rotate"></i>
                        </button>
                    </div>
                </div>

                <!-- Changed files -->
                <div class="gp-section" id="gpStatusSection" style="display:none">
                    <div class="gp-label">Changed Files</div>
                    <div class="gp-status-list" id="gpStatusList"></div>
                </div>

                <!-- Commit log -->
                <div class="gp-section">
                    <div class="gp-label" style="display:flex;align-items:center;justify-content:space-between">
                        Recent Commits
                        <button class="gp-log-toggle" onclick="window.gitPanel._toggleLog()">
                            <span id="gpLogToggleLabel">show</span>
                        </button>
                    </div>
                    <div id="gpLogList" style="display:none"></div>
                </div>
            </div>
        `;
        document.body.appendChild(root);
        this._el = root;

        // Toggle open/close on header click
        root.querySelector('#gpHeader').addEventListener('click', () => this._toggle());
    }

    // ── Repo loading ──────────────────────────────────────────────────────────

    async _loadRepos() {
        try {
            const res = await fetch('/api/git', { credentials: 'include' });
            const body = await res.json();
            this.repos = (body.success && body.data) ? body.data : [];
        } catch {
            this.repos = [];
        }

        if (!this.repos.length) {
            // No repos configured — show a minimal hint
            document.getElementById('gpBranchName').textContent = 'No repo configured';
            document.getElementById('gitPanelRoot').style.display = '';
            return;
        }

        // Populate repo select
        const sel = document.getElementById('gpRepoSelect');
        sel.innerHTML = this.repos.map(r =>
            `<option value="${r.id}">${r.name} (${r.branch})</option>`
        ).join('');
        this.repoId = this.repos[0].id;

        if (this.repos.length > 1) {
            document.getElementById('gpRepoSection').style.display = '';
        }

        // Show the panel button in the pipeline header if present
        const btn = document.getElementById('gitPanelBtn');
        if (btn) btn.style.display = '';

        // Show the panel
        document.getElementById('gitPanelRoot').style.display = '';

        await this._refreshAll();
    }

    async _refreshAll() {
        await Promise.all([
            this._refreshBranches(),
            this._refreshStatus(),
            this._refreshLog(),
        ]);
    }

    // ── Branch ops ────────────────────────────────────────────────────────────

    async _refreshBranches() {
        if (!this.repoId) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/branches`, { credentials: 'include' });
            const body = await res.json();
            if (!body.success) return;

            this.currentBranch = body.current || '—';
            document.getElementById('gpBranchName').textContent = this.currentBranch;

            const branches = body.data || [];
            const sel = document.getElementById('gpBranchSelect');
            sel.innerHTML = branches.map(b =>
                `<option value="${b}" ${b === this.currentBranch ? 'selected' : ''}>${b}</option>`
            ).join('');

            document.getElementById('gpBranchSection').style.display = branches.length > 1 ? '' : 'none';
        } catch { /* ignore */ }
    }

    async _onBranchChange() {
        const branch = document.getElementById('gpBranchSelect').value;
        if (!this.repoId || branch === this.currentBranch) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/checkout`, {
                method: 'POST', credentials: 'include',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ branch }),
            });
            const body = await res.json();
            this._toast(body.success ? `Switched to ${branch}` : body.error, body.success ? 'success' : 'error');
            if (body.success) await this._refreshAll();
        } catch (e) {
            this._toast(e.message, 'error');
        }
    }

    async _newBranch() {
        const name = prompt('New branch name:');
        if (!name) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/branches`, {
                method: 'POST', credentials: 'include',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name }),
            });
            const body = await res.json();
            this._toast(body.success ? `Branch created: ${name}` : body.error, body.success ? 'success' : 'error');
            if (body.success) await this._refreshBranches();
        } catch (e) {
            this._toast(e.message, 'error');
        }
    }

    // ── Repo selection ────────────────────────────────────────────────────────

    async _onRepoChange() {
        this.repoId = document.getElementById('gpRepoSelect').value;
        await this._refreshAll();
    }

    // ── Status ────────────────────────────────────────────────────────────────

    async _refreshStatus() {
        if (!this.repoId) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/status`, { credentials: 'include' });
            const body = await res.json();
            this.statusFiles = body.success ? (body.data || []) : [];
        } catch {
            this.statusFiles = [];
        }
        this._renderStatus();
    }

    _renderStatus() {
        const badge = document.getElementById('gpBadge');
        const section = document.getElementById('gpStatusSection');
        const list = document.getElementById('gpStatusList');

        if (!this.statusFiles.length) {
            badge.style.display = 'none';
            section.style.display = 'none';
            return;
        }

        badge.style.display = 'inline-block';
        section.style.display = '';
        list.innerHTML = this.statusFiles.map(f => {
            const code = (f.staging !== ' ' && f.staging !== '?') ? f.staging : f.worktree;
            const cls = (f.staging !== ' ' && f.staging !== '?') ? 'staged' : '';
            return `<div class="gp-status-item">
                <span class="gp-status-code ${cls}">${this._esc(code)}</span>
                <span>${this._esc(f.path)}</span>
            </div>`;
        }).join('');
    }

    // ── Commit log ────────────────────────────────────────────────────────────

    async _refreshLog() {
        if (!this.repoId) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/log?limit=10`, { credentials: 'include' });
            const body = await res.json();
            this.commitLog = body.success ? (body.data || []) : [];
        } catch {
            this.commitLog = [];
        }
        this._renderLog();
    }

    _renderLog() {
        const list = document.getElementById('gpLogList');
        if (!this.commitLog.length) {
            list.innerHTML = '<div style="color:#94a3b8;font-size:11px;padding:4px 0">No commits yet.</div>';
            return;
        }
        list.innerHTML = this.commitLog.map(c => {
            const ts = new Date(c.timestamp).toLocaleString([], { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit' });
            return `<div class="gp-log-item">
                <span class="gp-log-sha">${this._esc(c.short_sha)}</span>
                <div class="gp-log-msg">${this._esc(c.message.split('\n')[0])}</div>
                <div class="gp-log-meta">${this._esc(c.author)} · ${ts}</div>
            </div>`;
        }).join('');
    }

    _toggleLog() {
        this.logVisible = !this.logVisible;
        document.getElementById('gpLogList').style.display = this.logVisible ? '' : 'none';
        document.getElementById('gpLogToggleLabel').textContent = this.logVisible ? 'hide' : 'show';
        if (this.logVisible) this._refreshLog();
    }

    // ── Commit ────────────────────────────────────────────────────────────────

    async _commit() {
        if (!this.repoId) {
            this._toast('No repository selected. Configure one in Settings → Git Integration.', 'error');
            return;
        }

        const msg = document.getElementById('gpCommitMsg').value.trim();
        if (!msg) {
            document.getElementById('gpCommitMsg').focus();
            this._toast('Enter a commit message.', 'error');
            return;
        }

        const btn = document.getElementById('gpCommitBtn');
        btn.disabled = true;
        btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Committing…';

        try {
            const res = await fetch(`/api/git/${this.repoId}/commit-interface`, {
                method: 'POST', credentials: 'include',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    interface_id:   this.interfaceId,
                    commit_message: msg,
                }),
            });
            const body = await res.json();
            if (!body.success) throw new Error(body.error);
            document.getElementById('gpCommitMsg').value = '';
            this._toast(`✓ Committed ${body.commit_sha ? body.commit_sha.slice(0, 7) : ''}`, 'success');
            await Promise.all([this._refreshStatus(), this._refreshLog()]);
        } catch (e) {
            this._toast(e.message, 'error');
        } finally {
            btn.disabled = false;
            btn.innerHTML = '<i class="fas fa-code-commit"></i> Commit';
        }
    }

    // ── Pull ──────────────────────────────────────────────────────────────────

    async _pull() {
        if (!this.repoId) return;
        try {
            const res = await fetch(`/api/git/${this.repoId}/pull`, {
                method: 'POST', credentials: 'include',
            });
            const body = await res.json();
            this._toast(body.success ? body.message : body.error, body.success ? 'success' : 'error');
            if (body.success) await this._refreshAll();
        } catch (e) {
            this._toast(e.message, 'error');
        }
    }

    // ── Panel toggle ──────────────────────────────────────────────────────────

    _toggle() {
        const root = document.getElementById('gitPanelRoot');
        const isOpen = root.classList.toggle('gp-open');
        document.getElementById('gpChevron').className =
            isOpen ? 'fas fa-chevron-down' : 'fas fa-chevron-up';
        if (isOpen) this._refreshStatus();
    }

    show() {
        document.getElementById('gitPanelRoot').style.display = '';
    }

    hide() {
        document.getElementById('gitPanelRoot').style.display = 'none';
    }

    // ── Toast ─────────────────────────────────────────────────────────────────

    _toast(msg, type = '') {
        const el = document.getElementById('gpToast');
        el.textContent = msg;
        el.className = 'gp-toast' + (type ? ' ' + type : '');
        el.style.display = '';
        clearTimeout(this._toastTimer);
        this._toastTimer = setTimeout(() => { el.style.display = 'none'; }, 4000);
    }

    // ── Utilities ─────────────────────────────────────────────────────────────

    _esc(s) {
        return String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }
}
