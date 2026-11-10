// tests/unit/htmlSafe.test.js — public/js/core/html-safe.js and its wiring.
const fs = require('fs');
const path = require('path');
const { escapeHtml, html, raw, sanitize } = require('../../public/js/core/html-safe');

const ROOT = path.join(__dirname, '..', '..');

describe('HtmlSafe.escapeHtml', () => {
    it('escapes every character that can break out of text or a quoted attribute', () => {
        expect(escapeHtml(`<img src=x onerror="a('b')">&\``))
            .toBe('&lt;img src=x onerror=&quot;a(&#39;b&#39;)&quot;&gt;&amp;&#96;');
    });
    it('renders null and undefined as empty, other values as strings', () => {
        expect(escapeHtml(null)).toBe('');
        expect(escapeHtml(undefined)).toBe('');
        expect(escapeHtml(0)).toBe('0');
        expect(escapeHtml(false)).toBe('false');
    });
});

describe('HtmlSafe.html', () => {
    it('escapes interpolated values and keeps the template markup', () => {
        const name = '<script>x()</script>';
        expect(String(html`<td title="${name}">${name}</td>`))
            .toBe('<td title="&lt;script&gt;x()&lt;/script&gt;">&lt;script&gt;x()&lt;/script&gt;</td>');
    });
    it('nests: an html result is inserted as markup, not escaped again', () => {
        const cell = html`<b>${'a&b'}</b>`;
        expect(String(html`<td>${cell}</td>`)).toBe('<td><b>a&amp;b</b></td>');
    });
    it('renders arrays (e.g. .map()) item by item, escaping plain strings', () => {
        const rows = ['<i>', html`<i>${'x'}</i>`];
        expect(String(html`<ul>${rows}</ul>`)).toBe('<ul>&lt;i&gt;<i>x</i></ul>');
    });
    it('raw() passes trusted markup through', () => {
        expect(String(html`<p>${raw('<br>')}</p>`)).toBe('<p><br></p>');
    });
});

describe('HtmlSafe.sanitize without a DOM', () => {
    // The browser path (allowlist rebuild in an inert document) is covered by
    // tests/playwright/security/ui-xss-regression.spec.js; without DOMParser it must
    // never pass markup through.
    it('falls back to showing the markup as text', () => {
        expect(String(sanitize('<b onclick="x()">hi</b>'))).toBe('&lt;b onclick=&quot;x()&quot;&gt;hi&lt;/b&gt;');
        expect(String(sanitize(null))).toBe('');
    });
});

describe('every page loads html-safe.js before any other script', () => {
    const pages = fs.readdirSync(path.join(ROOT, 'public')).filter(f => f.endsWith('.html'));
    it.each(pages.map(p => [p]))('%s', (page) => {
        const src = fs.readFileSync(path.join(ROOT, 'public', page), 'utf8');
        const first = src.match(/<script\b[^>]*>/i);
        expect(first && first[0]).toMatch(/src="\/js\/core\/html-safe\.js"/);
    });
});
