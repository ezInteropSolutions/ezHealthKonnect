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

    // ── sanitize: markup from outside that is MEANT to be shown as HTML ──────────
    // Used for clinical narratives (FHIR Resource.text.div). Follows the FHIR
    // narrative rules: a fixed set of formatting elements, no scripts, forms,
    // frames or styles blocks, no event attributes. The markup is parsed in an
    // inert document (nothing runs, nothing loads) and only allowed nodes are
    // rebuilt; svg/math/style/template are never allowed, which closes the
    // known serialize-and-reparse ("mutation XSS") tricks.
    var ALLOWED_TAGS = {};
    ('a abbr acronym address b big blockquote br caption cite code col colgroup dd del dfn div dl dt em ' +
     'h1 h2 h3 h4 h5 h6 hr i img ins kbd li ol p pre q samp small span strong sub sup table tbody td tfoot ' +
     'th thead tr tt u ul var').split(' ').forEach(function (t) { ALLOWED_TAGS[t] = true; });
    var ALLOWED_ATTRS = {};
    ('class title lang dir colspan rowspan align valign width height border cellpadding cellspacing span ' +
     'alt style href src').split(' ').forEach(function (a) { ALLOWED_ATTRS[a] = true; });
    // Elements whose whole content is dropped, not just the tag.
    var DROP_WITH_CONTENT = { script: 1, style: 1, template: 1, noscript: 1, iframe: 1, object: 1,
        embed: 1, svg: 1, math: 1, textarea: 1, select: 1, title: 1, head: 1 };

    function safeUrl(value, forImage) {
        var v = String(value).replace(/[\u0000- ]+/g, '').toLowerCase();
        if (v.charAt(0) === '#') return !forImage;
        if (forImage) return /^https:/.test(v) || /^data:image\/(png|gif|jpe?g|webp);/.test(v);
        return /^(https?:|mailto:)/.test(v);
    }

    function safeStyle(value) {
        return !/url\s*\(|expression\s*\(|@import|javascript:|behavior\s*:|-moz-binding/i.test(value);
    }

    function sanitize(markup) {
        if (markup === null || markup === undefined || markup === '') return new SafeHtml('');
        if (typeof DOMParser === 'undefined' || typeof document === 'undefined') {
            return new SafeHtml(escapeHtml(markup)); // no DOM: show as text
        }
        var inert = new DOMParser().parseFromString('<!DOCTYPE html><body>' + String(markup), 'text/html');
        var out = inert.createElement('div');
        (function copy(from, to) {
            for (var n = from.firstChild; n; n = n.nextSibling) {
                if (n.nodeType === 3) { to.appendChild(inert.createTextNode(n.nodeValue)); continue; }
                if (n.nodeType !== 1) continue;
                var tag = n.localName;
                if (DROP_WITH_CONTENT[tag]) continue;
                if (!ALLOWED_TAGS[tag] || n.namespaceURI !== 'http://www.w3.org/1999/xhtml') { copy(n, to); continue; }
                var el = inert.createElement(tag);
                for (var i = 0; i < n.attributes.length; i++) {
                    var name = n.attributes[i].name.toLowerCase();
                    var value = n.attributes[i].value;
                    if (!ALLOWED_ATTRS[name]) continue;
                    if (name === 'href' && (tag !== 'a' || !safeUrl(value, false))) continue;
                    if (name === 'src' && (tag !== 'img' || !safeUrl(value, true))) continue;
                    if (name === 'style' && !safeStyle(value)) continue;
                    el.setAttribute(name, value);
                }
                if (tag === 'a' && el.hasAttribute('href')) {
                    el.setAttribute('target', '_blank');
                    el.setAttribute('rel', 'noopener noreferrer');
                }
                copy(n, el);
                to.appendChild(el);
            }
        })(inert.body, out);
        return new SafeHtml(out.innerHTML);
    }

    var api = Object.freeze({ escapeHtml: escapeHtml, html: html, raw: raw, sanitize: sanitize, SafeHtml: SafeHtml });
    if (typeof module === 'object' && module.exports) module.exports = api;
    if (root) root.HtmlSafe = api;
})(typeof window !== 'undefined' ? window : null);
