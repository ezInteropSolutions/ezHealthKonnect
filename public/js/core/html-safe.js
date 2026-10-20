/**
 * html-safe.js — the one way the UI turns data into HTML.
 *
 * Every page loads this first. Anything that comes from a user, a message, a
 * connected system or the API is text, and must reach the DOM escaped:
 *
 *   el.innerHTML = HtmlSafe.html`<td>${iface.name}</td>`;   // escaped
 *   el.textContent = iface.name;                            // also fine
 *
 * `html` escapes each interpolated value unless it is itself the result of
 * `html` (so templates nest) or an array of such values (so .map() lists
 * work). `HtmlSafe.raw(markup)` marks markup the code itself produced as
 * trusted — it is the explicit, greppable opt-out and must never wrap data.
 *
 * Why: the UI builds most views with innerHTML template strings, and the
 * Content-Security-Policy still has to allow inline handlers ('unsafe-inline'),
 * so it cannot stop an injected <img onerror>. Escaping is the defense, and
 * eslint-plugin-no-unsanitized (npm run lint:xss) enforces that every HTML
 * sink goes through this module.
 */
(function (root) {
    'use strict';

    var ENTITIES = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;' };

    /** Escape a value for use in HTML text or a quoted attribute. null/undefined → ''. */
    function escapeHtml(value) {
        if (value === null || value === undefined) return '';
        return String(value).replace(/[&<>"'`]/g, function (c) { return ENTITIES[c]; });
    }

    /** Markup that is already safe. Only `html` and `raw` create it. */
    function SafeHtml(markup) { this.markup = markup; }
    SafeHtml.prototype.toString = function () { return this.markup; };

    function render(value) {
        if (value instanceof SafeHtml) return value.markup;
        if (Array.isArray(value)) return value.map(render).join('');
        return escapeHtml(value);
    }

    /** Tagged template: escapes every interpolation (see above). */
    function html(strings) {
        var out = strings[0];
        for (var i = 1; i < strings.length; i++) {
            out += render(arguments[i]) + strings[i];
        }
        return new SafeHtml(out);
    }

    /** Trusted markup produced by the application's own code — never data. */
    function raw(markup) {
        return new SafeHtml(markup === null || markup === undefined ? '' : String(markup));
    }

    var api = Object.freeze({ escapeHtml: escapeHtml, html: html, raw: raw, SafeHtml: SafeHtml });
    if (typeof module === 'object' && module.exports) module.exports = api;
    if (root) root.HtmlSafe = api;
})(typeof window !== 'undefined' ? window : null);
