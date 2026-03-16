// sidebar-nav.js — Collapsible sidebar sections
// Sections auto-collapse on load; the section containing the active .nav-item stays open.
// Clicking a section header toggles it open/closed.

(function () {
    'use strict';

    function initCollapsibleSections() {
        var sidebar = document.getElementById('sidebar');
        if (!sidebar) return;

        var sections = sidebar.querySelectorAll('.nav-section');
        sections.forEach(function (section) {
            var header = section.querySelector('.section-header');
            var items  = section.querySelector('.nav-items');
            if (!header || !items) return;

            // Collapse all sections that don't contain the active link
            var hasActive = section.querySelector('.nav-item.active') !== null;
            if (!hasActive) {
                section.classList.add('collapsed');
            }

            header.addEventListener('click', function () {
                section.classList.toggle('collapsed');
            });
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initCollapsibleSections);
    } else {
        initCollapsibleSections();
    }
}());
