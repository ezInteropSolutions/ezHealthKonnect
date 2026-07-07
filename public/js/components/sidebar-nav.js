/**
 * sidebar-nav.js — Universal sidebar component (single source of truth)
 *
 * Renders the complete sidebar HTML, handles collapse/toggle, fetches the
 * current user from /api/auth/me, shows the Admin section for admins, and
 * exposes window.logout() for any inline onclick handlers.
 *
 * Usage: any page just needs  <nav class="sidebar" id="sidebar"></nav>
 * and a <script src="js/components/sidebar-nav.js"></script>.
 */
(function () {
    'use strict';

    // ── Nav definition ─────────────────────────────────────────────────────────
    // To change nav items edit ONLY this array.  Every page picks up the change.
    var NAV = [
        {
            title: 'Main',
            items: [
                { label: 'Dashboard',    href: 'dashboard.html',           icon: 'far fa-house' },
                { label: 'Interfaces',   href: 'interfaces.html',          icon: 'fas fa-plug' },
                { label: 'Review Queue', href: 'review-queue.html',        icon: 'far fa-circle-exclamation', badgeId: 'rqNavBadge' },
                { label: 'Templates',    href: 'dashboard.html#templates', icon: 'far fa-file-lines' }
            ]
        },
        {
            title: 'Analytics',
            items: [
                { label: 'Monitoring',     href: 'monitoring.html',      icon: 'far fa-chart-bar' },
                { label: 'Engine Metrics', href: 'engine-metrics.html',  icon: 'fas fa-gauge-high' },
                { label: 'Reports',        href: 'reports.html',         icon: 'fas fa-chart-line' },
                { label: 'Alerts',         href: 'alerts.html',          icon: 'fas fa-bell', badgeId: 'alertsNavBadge' }
            ]
        },
        {
            title: 'Tools',
            items: [
                { label: 'HL7 Reader',      href: 'hl7-reader.html',       icon: 'fas fa-stethoscope' },
                { label: 'FHIR Validator',  href: 'fhir-validator.html',   icon: 'fas fa-shield-alt' },
                { label: 'Code Templates',  href: 'code-templates.html',   icon: 'fas fa-code' },
                { label: 'Git',             href: 'git.html',              icon: 'fab fa-git-alt' },
                { label: 'Migration',       href: 'migration.html',        icon: 'fas fa-exchange-alt' }
            ]
        },
        {
            title: 'Operations',
            sectionId: 'opsSection',
            roles: ['engineer', 'admin', 'superadmin', 'super_admin'],
            items: [
                { label: 'Dead-Letter Q',  href: 'admin-dlq.html',           icon: 'fas fa-exclamation-triangle', badgeId: 'dlqBadge' },
                { label: 'Message Trace',  href: 'message-trace.html',      icon: 'fas fa-route' }
            ]
        },
        {
            title: 'Admin',
            sectionId: 'adminSection',
            adminOnly: true,
            items: [
                { label: 'Users',          href: 'user-management.html',     icon: 'fas fa-users' },
                { label: 'Settings',       href: 'settings.html',            icon: 'fas fa-sliders' },
                { label: 'Mapping Review', href: 'admin-mapping-review.html', icon: 'fas fa-flag',             badgeId: 'mappingReviewBadge' }
            ]
        }
    ];

    // ── Active-link detection ──────────────────────────────────────────────────
    function getPageFile() {
        var parts = window.location.pathname.split('/');
        return parts[parts.length - 1] || 'dashboard.html';
    }

    function isActive(href) {
        // Never mark the Templates shortcut as active (it's a hash-anchor on dashboard)
        if (href.indexOf('#') !== -1) return false;
        return getPageFile() === href.split('/').pop();
    }

    // Returns true when the current page lives inside a restricted section
    function isAdminPage() {
        var current = getPageFile();
        for (var i = 0; i < NAV.length; i++) {
            if (!NAV[i].adminOnly && !NAV[i].roles) continue;
            for (var j = 0; j < NAV[i].items.length; j++) {
                if (NAV[i].items[j].href.split('/').pop() === current) return true;
            }
        }
        return false;
    }

    // Returns the section a page belongs to (by sectionId), or null
    function getSectionForPage() {
        var current = getPageFile();
        for (var i = 0; i < NAV.length; i++) {
            for (var j = 0; j < NAV[i].items.length; j++) {
                if (NAV[i].items[j].href.split('/').pop() === current) return NAV[i].sectionId || null;
            }
        }
        return null;
    }

    // ── HTML builder ───────────────────────────────────────────────────────────
    function esc(s) {
        return String(s)
            .replace(/&/g, '&amp;').replace(/</g, '&lt;')
            .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    }

    function buildHTML() {
        var h = '';

        // Collapse toggle button
        h += '<button class="sidebar-toggle" id="sidebarToggle">' +
             '<span class="toggle-icon">&#x2039;</span></button>';

        // Logo
        h += '<div class="logo-container">' +
             '<img src="/assets/logos/ezHealthKonnect.jpeg" alt="ezHealthKonnect" class="logo-image">' +
             '<div class="logo-text">' +
             '<h3 class="brand-name">ez<span class="brand-accent">Health</span>Konnect</h3>' +
             '<p class="brand-tagline">Healthcare Integration</p>' +
             '</div></div>';

        // Nav menu
        h += '<div class="nav-menu">';
        var currentSection = getSectionForPage();
        NAV.forEach(function (section) {
            var isRestricted = section.adminOnly || section.roles;
            var idAttr   = section.sectionId ? ' id="' + section.sectionId + '"' : '';
            // Hide restricted sections by default — show if we're on a page within this section
            var hideAttr = isRestricted && currentSection !== section.sectionId ? ' style="display:none"' : '';
            h += '<div class="nav-section' + (isRestricted ? ' admin-section' : '') + '"' + idAttr + hideAttr + '>';
            h += '<div class="section-header">' +
                 '<span class="section-title">' + esc(section.title) + '</span>' +
                 '<i class="fas fa-chevron-down nav-section-chevron"></i>' +
                 '</div>';
            h += '<div class="nav-items">';
            section.items.forEach(function (item) {
                var active = isActive(item.href) ? ' active' : '';
                h += '<a href="' + esc(item.href) + '" class="nav-item' + active + '">';
                h += '<span class="nav-icon"><i class="' + esc(item.icon) + '"></i></span>';
                h += '<span class="nav-label">' + esc(item.label) + '</span>';
                if (item.badgeId) {
                    h += '<span class="nav-badge" id="' + esc(item.badgeId) + '" style="display:none"></span>';
                }
                h += '</a>';
            });
            h += '</div></div>';
        });
        h += '</div>'; // .nav-menu

        // User profile
        h += '<div class="user-profile">' +
             '<div class="user-info">' +
             '<div class="user-avatar" id="userAvatar">U</div>' +
             '<div class="user-details">' +
             '<span class="user-name" id="userName">Loading...</span>' +
             '<span class="user-role" id="userRole">USER</span>' +
             '</div></div>' +
             '<button class="logout-btn" id="sidebarLogoutBtn" title="Logout">' +
             '<span class="nav-icon"><i class="fas fa-right-from-bracket"></i></span>' +
             '<span class="nav-label">Logout</span>' +
             '</button></div>';

        return h;
    }

    // ── Collapsible sections ───────────────────────────────────────────────────
    function initCollapsible(sidebar) {
        sidebar.querySelectorAll('.nav-section').forEach(function (sec) {
            var header = sec.querySelector('.section-header');
            if (!header) return;
            // Keep the section that contains the active link open; collapse all others.
            // Note: querySelector searches regardless of display:none, so an active link
            // inside a hidden admin section is still found correctly.
            var hasActive = !!sec.querySelector('.nav-item.active');
            if (!hasActive) {
                sec.classList.add('collapsed');
            }
            header.addEventListener('click', function () {
                sec.classList.toggle('collapsed');
            });
        });
    }

    // ── Sidebar toggle (collapse/expand the whole nav) ─────────────────────────
    function initToggle(sidebar) {
        var btn = document.getElementById('sidebarToggle');
        if (!btn || btn.dataset.sidebarBound) return;
        btn.dataset.sidebarBound = '1';
        btn.addEventListener('click', function () {
            sidebar.classList.toggle('collapsed');
            var icon = btn.querySelector('.toggle-icon');
            if (icon) icon.textContent = sidebar.classList.contains('collapsed') ? '\u203a' : '\u2039';
        });
    }

    // ── Logout (also exposed as window.logout for inline onclick="logout()") ───
    function doLogout() {
        fetch('/api/auth/logout', { method: 'POST', credentials: 'include' })
            .finally(function () { window.location.href = '/login.html'; });
    }

    window.logout = doLogout; // global shim

    function initLogoutBtn(sidebar) {
        var btn = sidebar.querySelector('#sidebarLogoutBtn');
        if (btn) btn.addEventListener('click', doLogout);
    }

    // ── User profile loader ────────────────────────────────────────────────────
    var ADMIN_ROLES = ['admin', 'superadmin'];

    function applyUser(sidebar, user) {
        if (!user) return;
        var name = user.full_name || user.name || user.username || 'User';
        var role = (user.role || 'user').toUpperCase();

        var nameEl   = sidebar.querySelector('#userName');
        var roleEl   = sidebar.querySelector('#userRole');
        var avatarEl = sidebar.querySelector('#userAvatar');
        if (nameEl)   nameEl.textContent   = name;
        if (roleEl)   roleEl.textContent   = role;
        if (avatarEl) avatarEl.textContent = name.charAt(0).toUpperCase();

        var role = (user.role || '').toLowerCase();
        var activeSectionId = getSectionForPage();

        // Show/hide each restricted section based on role
        NAV.forEach(function (section) {
            if (!section.sectionId) return;
            var sec = sidebar.querySelector('#' + section.sectionId);
            if (!sec) return;

            var allowed = false;
            if (section.adminOnly) {
                allowed = ADMIN_ROLES.indexOf(role) !== -1;
            } else if (section.roles) {
                allowed = section.roles.indexOf(role) !== -1;
            }

            if (allowed) {
                sec.style.display = '';
            } else if (activeSectionId !== section.sectionId) {
                sec.style.display = 'none';
            }
        });
    }

    function loadUser(sidebar) {
        // ── Fast path: read cached user from localStorage (instant, no flash) ──
        try {
            var cached = localStorage.getItem('user') || localStorage.getItem('currentUser');
            if (cached) {
                var parsed = JSON.parse(cached);
                applyUser(sidebar, parsed);
            }
        } catch (_) {}

        // ── Authoritative path: fetch from API ─────────────────────────────────
        fetch('/api/auth/session', { credentials: 'include' })
            .then(function (r) {
                if (r.status === 401) { window.location.href = '/login.html'; return null; }
                if (!r.ok) return null;
                return r.json();
            })
            .then(function (d) {
                if (!d || !d.authenticated) return;
                var user = d.user || d;
                // Cache for next page load (instant display)
                try { localStorage.setItem('user', JSON.stringify(user)); } catch (_) {}
                applyUser(sidebar, user);
            })
            .catch(function () { /* network error – don't force redirect */ });
    }

    // ── Alert badge poller ────────────────────────────────────────────────────
    // Runs on every page (60s interval, lightweight countOnly query).
    // Does nothing if not authenticated (fetch will get 401 and silently fail).
    function startAlertBadgePoller() {
        function updateBadge() {
            fetch('/api/alerts/fired?countOnly=true', { credentials: 'include' })
                .then(function(r) { return r.ok ? r.json() : null; })
                .then(function(d) {
                    var badge = document.getElementById('alertsNavBadge');
                    if (!badge || !d) return;
                    var n = d.count || 0;
                    badge.textContent    = n > 99 ? '99+' : n;
                    badge.style.display  = n > 0 ? '' : 'none';
                })
                .catch(function() {});
        }
        updateBadge();
        setInterval(updateBadge, 60000);
    }

    // ── Mapping review badge poller ───────────────────────────────────────────
    // Lightweight check every 2 minutes; only runs for admins (non-admins get 403).
    function startMappingReviewBadgePoller() {
        function updateBadge() {
            fetch('/api/fhir/quality/flagged?limit=1&offset=0', { credentials: 'include' })
                .then(function (r) { return r.ok ? r.json() : null; })
                .then(function (d) {
                    var badge = document.getElementById('mappingReviewBadge');
                    if (!badge || !d) return;
                    var n = d.total || 0;
                    badge.textContent   = n > 99 ? '99+' : n;
                    badge.style.display = n > 0 ? '' : 'none';
                })
                .catch(function () {});
        }
        updateBadge();
        setInterval(updateBadge, 120000);
    }

    // ── DLQ badge poller ─────────────────────────────────────────────────────
    // Shows pending count on the Dead-Letter Q nav item. Runs every 2 minutes.
    function startDLQBadgePoller() {
        function updateBadge() {
            fetch('/api/fhir/dlq/stats', { credentials: 'include' })
                .then(function (r) { return r.ok ? r.json() : null; })
                .then(function (d) {
                    var badge = document.getElementById('dlqBadge');
                    if (!badge || !d || !d.success) return;
                    var n = (d.data && d.data.pending) || 0;
                    badge.textContent   = n > 99 ? '99+' : n;
                    badge.style.display = n > 0 ? '' : 'none';
                })
                .catch(function () {});
        }
        updateBadge();
        setInterval(updateBadge, 120000);
    }

    // ── Main init ─────────────────────────────────────────────────────────────
    function init() {
        var sidebar = document.getElementById('sidebar');
        if (!sidebar) return;

        // Replace entire sidebar content with canonical HTML
        sidebar.innerHTML = buildHTML();

        initCollapsible(sidebar);
        initToggle(sidebar);
        initLogoutBtn(sidebar);
        loadUser(sidebar);
        startAlertBadgePoller();
        startMappingReviewBadgePoller();
        startDLQBadgePoller();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
}());
