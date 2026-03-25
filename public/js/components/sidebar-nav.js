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
                { label: 'Monitoring', href: 'monitoring.html', icon: 'far fa-chart-bar' },
                { label: 'Reports',    href: 'reports.html',    icon: 'fas fa-chart-line' },
                { label: 'Alerts',     href: 'alerts.html',     icon: 'fas fa-bell', badgeId: 'alertsNavBadge' }
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
            title: 'Admin',
            adminOnly: true,
            items: [
                { label: 'Users',    href: 'user-management.html', icon: 'fas fa-users' },
                { label: 'Settings', href: 'settings.html',        icon: 'fas fa-sliders' }
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

    // Returns true when the current page lives inside an admin-only section
    function isAdminPage() {
        var current = getPageFile();
        for (var i = 0; i < NAV.length; i++) {
            if (!NAV[i].adminOnly) continue;
            for (var j = 0; j < NAV[i].items.length; j++) {
                if (NAV[i].items[j].href.split('/').pop() === current) return true;
            }
        }
        return false;
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
        var onAdminPage = isAdminPage();
        NAV.forEach(function (section) {
            // Hide admin section by default — UNLESS we are currently on an admin page
            // (in which case the section must be visible for the active link to show)
            var adminAttr = section.adminOnly && !onAdminPage
                ? ' id="adminSection" style="display:none"'
                : (section.adminOnly ? ' id="adminSection"' : '');
            h += '<div class="nav-section' + (section.adminOnly ? ' admin-section' : '') + '"' + adminAttr + '>';
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

        // Show admin section for admin/superadmin users (may already be visible on admin pages)
        if (ADMIN_ROLES.indexOf((user.role || '').toLowerCase()) !== -1) {
            var sec = sidebar.querySelector('#adminSection');
            if (sec) sec.style.display = '';
        } else if (!isAdminPage()) {
            // Non-admin user on a regular page: ensure admin section stays hidden
            var sec2 = sidebar.querySelector('#adminSection');
            if (sec2) sec2.style.display = 'none';
        }
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
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
}());
