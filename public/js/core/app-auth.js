/**
 * AppAuth — the browser's single source of truth for "who am I and what may I do".
 *
 * The SERVER enforces every permission; this class only keeps the UI honest with it, so nobody is
 * offered a button that is certain to fail, and a refused action never fails silently.
 *
 *   appAuth.whenReady().then(user => ...)   resolved once the session is known
 *   appAuth.can('write')                    capability check (see CAPABILITIES)
 *   appAuth.isAdmin()
 *
 * Declarative markup (handled here, including elements rendered later by page scripts):
 *   <button data-requires="write">     disabled, with an explanatory tooltip, for roles that cannot write
 *   <div    data-requires="admin" data-requires-mode="hide">    hidden for non-admins
 *   <body   data-requires-page="admin">  the page shows an "Access denied" panel for non-admins
 *
 * CAPABILITIES mirrors the role lists in app.js (GO_READ_ROLES / GO_WRITE_ROLES), the Node route gates
 * (requireRole('admin', 'operator', ...)) and the Go handlers that are admin-only (e.g. /api/fhir/dlq).
 * When a server rule changes, change it here too — tests/playwright/security/roles-and-endpoint-auth.spec.js
 * fails if the two drift apart.
 *
 * Writes the server refuses (403 + a message) are turned into one visible toast by installRefusalNotice().
 */
(function () {
    'use strict';

    var ADMIN_ROLES = ['admin', 'super_admin', 'superadmin'];

    var CAPABILITIES = {
        // Create / change / delete / activate interfaces, reprocess messages, run pipeline tests …
        write:     ADMIN_ROLES.concat(['operator', 'engineer']),
        // Anything that reads operational data.
        read:      ADMIN_ROLES.concat(['operator', 'viewer', 'engineer']),
        // Administration: users, settings, audit, dead-letter queue (admin-only on the server).
        admin:     ADMIN_ROLES,
        // Integration-team-only pages (e.g. Mapping Review): requireProxiedSuperAdmin() in Go.
        superAdmin: ['super_admin', 'superadmin'],
        dlq:       ADMIN_ROLES,
        aiSettings: ADMIN_ROLES
    };

    var DENIED_TITLE = {
        write: 'Your role (%role%) is read-only. Changing this needs the operator or administrator role.',
        admin: 'This needs the administrator role. Your role is %role%.'
    };

    class AppAuth {
        constructor() {
            this.user = null;
            this.access = null; // { interfaceScope, messageContent } from the session, set by Settings -> Access policy
            this._readyResolve = null;
            this._ready = new Promise(function (resolve) { this._readyResolve = resolve; }.bind(this));
            this._lastRefusalAt = 0;
            this._loadCached();
        }

        // Instant role from the cache the sidebar has always kept (avoids a flash of controls).
        _loadCached() {
            try {
                var cached = localStorage.getItem('user') || localStorage.getItem('currentUser');
                if (cached) this.user = JSON.parse(cached);
            } catch (_) { /* no cache — wait for the session */ }
        }

        /** Fetches /api/auth/session once; every later call reuses the same promise. */
        start() {
            if (this._started) return this._ready;
            this._started = true;
            var self = this;
            fetch('/api/auth/session', { credentials: 'include' })
                .then(function (r) {
                    if (r.status === 401) { window.location.href = '/login.html'; return null; }
                    return r.ok ? r.json() : null;
                })
                .then(function (d) {
                    if (d && d.authenticated) {
                        self.user = d.user || d;
                        self.access = d.access || null;
                        try { localStorage.setItem('user', JSON.stringify(self.user)); } catch (_) { /* private mode */ }
                    }
                })
                .catch(function () { /* network error: keep whatever the cache gave us */ })
                .then(function () {
                    self._readyResolve(self.user);
                    self.applyAll();
                    document.dispatchEvent(new CustomEvent('appauth:ready', { detail: { user: self.user } }));
                });
            return this._ready;
        }

        whenReady() { this.start(); return this._ready; }

        get role() { return ((this.user && this.user.role) || '').toLowerCase(); }

        /** True when the server would let this role do `capability`. Unknown role or capability: false. */
        can(capability) {
            var roles = CAPABILITIES[capability];
            return !!roles && roles.indexOf(this.role) !== -1;
        }

        isAdmin() { return this.can('admin'); }

        /** False only when the server said this role gets message details but not content. Unknown: true (the server still enforces). */
        canViewMessageContent() { return !(this.access && this.access.messageContent === 'metadata'); }

        // ── Declarative controls ──────────────────────────────────────────────────

        /** Applies data-requires rules inside `root` (default: the whole document). */
        apply(root) {
            if (!this.user) return; // role not known yet: leave controls alone, the server still enforces
            var scope = root && root.querySelectorAll ? root : document;
            var nodes = Array.prototype.slice.call(scope.querySelectorAll('[data-requires]'));
            if (scope !== document && scope.matches && scope.matches('[data-requires]')) nodes.push(scope);
            nodes.forEach(this._applyToElement, this);
        }

        applyAll() {
            this.apply(document);
            this._guardPage();
        }

        _applyToElement(el) {
            var capability = el.getAttribute('data-requires');
            if (this.can(capability)) {
                if (el.hasAttribute('data-auth-denied')) this._restore(el);
                return;
            }
            if (el.hasAttribute('data-auth-denied')) return;
            el.setAttribute('data-auth-denied', capability);
            if (el.getAttribute('data-requires-mode') === 'hide' || capability === 'admin') {
                el.setAttribute('data-auth-prev-display', el.style.display || '');
                el.style.display = 'none';
                return;
            }
            el.setAttribute('data-auth-prev-disabled', el.disabled ? '1' : '0');
            el.setAttribute('data-auth-prev-title', el.getAttribute('title') || '');
            el.disabled = true;
            el.setAttribute('aria-disabled', 'true');
            el.style.opacity = '0.5';
            el.style.cursor = 'not-allowed';
            el.title = (DENIED_TITLE[capability] || DENIED_TITLE.write).replace('%role%', this.role || 'unknown');
        }

        _restore(el) {
            if (el.hasAttribute('data-auth-prev-display')) {
                el.style.display = el.getAttribute('data-auth-prev-display');
            } else {
                el.disabled = el.getAttribute('data-auth-prev-disabled') === '1';
                el.removeAttribute('aria-disabled');
                el.style.opacity = '';
                el.style.cursor = '';
                var title = el.getAttribute('data-auth-prev-title');
                if (title) el.title = title; else el.removeAttribute('title');
            }
            ['data-auth-denied', 'data-auth-prev-display', 'data-auth-prev-disabled', 'data-auth-prev-title']
                .forEach(function (a) { el.removeAttribute(a); });
        }

        /** Re-applies the rules to controls that page scripts add or re-render later. */
        observe() {
            if (this._observer || typeof MutationObserver === 'undefined') return;
            var self = this;
            this._observer = new MutationObserver(function (mutations) {
                if (!self.user) return;
                mutations.forEach(function (m) {
                    Array.prototype.forEach.call(m.addedNodes, function (n) {
                        if (n.nodeType === 1) self.apply(n);
                    });
                });
            });
            this._observer.observe(document.documentElement, { childList: true, subtree: true });
        }

        /**
         * Page scripts re-enable and re-render buttons as interface state changes, so `disabled` alone
         * is not a guarantee. A capture-phase click guard stops any click on a control this role may
         * not use, before the page's own handler runs, and says why.
         */
        installClickGuard() {
            if (this._guardInstalled) return;
            this._guardInstalled = true;
            var self = this;
            document.addEventListener('click', function (event) {
                var el = event.target && event.target.closest ? event.target.closest('[data-auth-denied]') : null;
                if (!el) return;
                event.preventDefault();
                event.stopImmediatePropagation();
                var capability = el.getAttribute('data-auth-denied');
                self._announceRefusal((DENIED_TITLE[capability] || DENIED_TITLE.write).replace('%role%', self.role || 'unknown'));
            }, true);
        }

        // ── Pages that need a role ────────────────────────────────────────────────

        _guardPage() {
            var required = document.body && document.body.getAttribute('data-requires-page');
            if (!required || !this.user || this.can(required)) return;
            if (document.getElementById('appAuthDenied')) return;
            var main = document.querySelector('.main-content, main, .page-content, #mainContent') || document.body;
            var panel = document.createElement('div');
            panel.id = 'appAuthDenied';
            panel.setAttribute('role', 'alert');
            panel.style.cssText = 'max-width:560px;margin:64px auto;padding:28px 32px;border:1px solid #fecaca;' +
                'background:#fef2f2;border-radius:12px;color:#7f1d1d;font-family:inherit;';
            var title = document.createElement('h2');
            title.style.cssText = 'margin:0 0 8px;font-size:20px;';
            title.textContent = 'Access denied';
            var body = document.createElement('p');
            body.style.cssText = 'margin:0;line-height:1.5;';
            body.textContent = 'This page needs the ' + ({ admin: 'administrator', superAdmin: 'super administrator' }[required] || required) +
                ' role. You are signed in as ' + (this.role || 'a user without a role') +
                '. Ask an administrator if you need access.';
            panel.appendChild(title);
            panel.appendChild(body);
            Array.prototype.slice.call(main.children).forEach(function (child) {
                if (child.tagName !== 'SCRIPT' && !child.classList.contains('sidebar') && child.tagName !== 'NAV') {
                    child.style.display = 'none';
                }
            });
            main.appendChild(panel);
            document.dispatchEvent(new CustomEvent('appauth:denied', { detail: { required: required } }));
        }

        // ── Refused writes are never silent ───────────────────────────────────────

        /**
         * Wraps window.fetch once. A write (not GET/HEAD) answered 403 with a JSON `message` raises one
         * toast with the server's own wording ("Access requires one of: admin, operator"). Pages that
         * already show their own error still do; the toast is de-duplicated within 2 seconds.
         */
        installRefusalNotice() {
            if (window.__appAuthFetchWrapped) return;
            window.__appAuthFetchWrapped = true;
            var original = window.fetch.bind(window);
            var self = this;
            window.fetch = function (input, init) {
                var method = ((init && init.method) || (input && input.method) || 'GET').toUpperCase();
                return original(input, init).then(function (response) {
                    if (response.status === 403 && method !== 'GET' && method !== 'HEAD') {
                        response.clone().json().then(function (body) {
                            if (body && body.message) self._announceRefusal(body.message);
                        }).catch(function () { /* not JSON */ });
                    }
                    return response;
                });
            };
        }

        _announceRefusal(message) {
            var now = Date.now();
            if (now - this._lastRefusalAt < 2000) return;
            this._lastRefusalAt = now;
            // Server refusals already start with "Access requires…"; local ones are full sentences.
            if (window.AppDialogs && typeof window.AppDialogs.toast === 'function') {
                window.AppDialogs.toast('Not allowed: ' + message, 'error');
            } else {
                console.warn('[AppAuth] Not allowed: ' + message);
            }
        }
    }

    // Clipboard fallback. navigator.clipboard exists only in a secure context (HTTPS or localhost) and can still refuse
    // (no permission); the installer sets up plain-HTTP installs by default, where every "Copy" button in the app
    // (22 call sites) threw "Cannot read properties of undefined (reading 'writeText')". Same call, old-style copy
    // when the modern one is missing or refused; still rejects when neither works, so callers' .catch handlers run.
    (function installClipboardFallback() {
        try {
            var nativeClip = navigator.clipboard;
            var nativeWrite = nativeClip && nativeClip.writeText ? nativeClip.writeText.bind(nativeClip) : null;
            var legacyCopy = function (text) {
                return new Promise(function (resolve, reject) {
                    var ta = document.createElement('textarea');
                    ta.value = String(text);
                    ta.setAttribute('readonly', '');
                    ta.style.cssText = 'position:fixed;top:-1000px;opacity:0';
                    document.body.appendChild(ta);
                    ta.select();
                    var ok = false;
                    try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
                    document.body.removeChild(ta);
                    if (ok) resolve(); else reject(new Error('Copy to clipboard is not permitted in this browser'));
                });
            };
            var writeText = function (text) {
                return nativeWrite ? nativeWrite(text).catch(function () { return legacyCopy(text); }) : legacyCopy(text);
            };
            if (!nativeClip) Object.defineProperty(navigator, 'clipboard', { value: { writeText: writeText }, configurable: true });
            else nativeClip.writeText = writeText;
        } catch (e) { /* leave the browser's own behaviour */ }
    })();

    var instance = new AppAuth();
    instance.installRefusalNotice();
    instance.installClickGuard();
    window.AppAuth = AppAuth;
    window.appAuth = instance;

    function boot() { instance.observe(); instance.apply(document); instance.start(); }
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot); else boot();
})();
