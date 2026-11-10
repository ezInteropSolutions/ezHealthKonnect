'use strict';
/**
 * Automatic UI gates (docs/QA_STRATEGY.md §3.1).
 *
 * Attach before navigation, run `collect()` after the page has settled. Every gate answers
 * "would a person looking at this screen see something broken?" — not "did the code run".
 *
 *   const gates = attachGates(page, { origin });
 *   await page.goto(url);
 *   const findings = await gates.collect();
 */

// Strings that must never be visible to a user.
const LEAK_PATTERN = /\bundefined\b|\[object Object\]|\bNaN\b|\{\{[^}]*\}\}|\$\{[^}]*\}/;

function attachGates(page, { origin, allowStatus = [] } = {}) {
    const consoleErrors = [];
    const pageErrors = [];
    const failedRequests = [];
    const badResponses = [];
    const offOrigin = new Set();

    page.on('console', (m) => {
        if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 300));
    });
    page.on('pageerror', (e) => pageErrors.push(String(e.message || e).slice(0, 300)));
    page.on('requestfailed', (r) => {
        failedRequests.push(`${r.method()} ${r.url().slice(0, 160)} — ${r.failure() && r.failure().errorText}`);
    });
    page.on('response', (r) => {
        const s = r.status();
        if (s >= 400 && !allowStatus.includes(s)) badResponses.push(`${s} ${r.request().method()} ${r.url().slice(0, 160)}`);
    });
    page.on('request', (r) => {
        const u = r.url();
        if (u.startsWith('data:') || u.startsWith('blob:') || u.startsWith('about:')) return;
        try {
            if (new URL(u).origin !== origin) offOrigin.add(new URL(u).origin);
        } catch (_) { /* ignore */ }
    });

    async function collect() {
        const dom = await page.evaluate((leakSrc) => {
            const out = { icons: [], overflowPage: false, clipped: [], leaks: [], overlap: [], noTitle: false, noH1: 0, smallTargets: 0 };
            const leak = new RegExp(leakSrc);
            const vis = (el) => {
                const r = el.getBoundingClientRect();
                const cs = getComputedStyle(el);
                return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none' && cs.opacity !== '0';
            };
            const sel = (el) => {
                if (el.id) return `#${el.id}`;
                const c = (el.className && typeof el.className === 'string') ? '.' + el.className.trim().split(/\s+/).slice(0, 2).join('.') : '';
                return el.tagName.toLowerCase() + c;
            };

            out.overflowPage = document.documentElement.scrollWidth > document.documentElement.clientWidth + 1;
            out.noTitle = !document.title || !document.title.trim();
            out.noH1 = document.querySelectorAll('h1').length;

            // A glyph the font does not contain is drawn as the fallback "missing glyph" box. Render the
            // icon's codepoint and a deliberately unassigned codepoint with the same font; identical
            // bitmaps mean the icon is missing (e.g. a Pro-only icon or a style the free build lacks).
            const probe = document.createElement('canvas');
            probe.width = 40; probe.height = 40;
            const pctx = probe.getContext('2d', { willReadFrequently: true });
            const bitmap = (ch, font) => {
                pctx.clearRect(0, 0, 40, 40);
                pctx.font = font;
                pctx.textBaseline = 'top';
                pctx.fillText(ch, 4, 4);
                return pctx.getImageData(0, 0, 40, 40).data.join(',');
            };
            const glyphMissing = (content, weight, family) => {
                const ch = (content || '').replace(/^["']|["']$/g, '');
                if (!ch || ch.length > 2) return false;
                const font = `${weight} 28px ${family}`;
                return bitmap(ch, font) === bitmap('', font);
            };

            // Icons: font icon elements must render a glyph and the icon font must be present.
            const iconEls = document.querySelectorAll('i[class*="fa-"], .fa, .fas, .far, .fab, .fa-solid, .fa-regular, .fa-brands, .material-icons, [class*="bi-"]');
            iconEls.forEach((el) => {
                // An icon whose font failed to load has no glyph and therefore zero size, so judge
                // visibility by its container, never by the icon itself.
                const host = el.parentElement;
                if (!host || !vis(host) || getComputedStyle(el).display === 'none') return;
                const before = getComputedStyle(el, '::before');
                const content = before.content;
                const family = before.fontFamily || getComputedStyle(el).fontFamily;
                const hasGlyph = content && content !== 'none' && content !== 'normal' && content !== '""';
                const familyLoaded = /Font Awesome|Material|bootstrap-icons/i.test(family);
                let fontReady = true;
                try { fontReady = document.fonts.check(`900 1em ${family.split(',')[0]}`) ; } catch (_) { /* ignore */ }
                // A glyph from a font that never loaded shows the fallback font's tofu/blank, and
                // its text content (ligature name) shows through for ligature icon fonts.
                const ligatureText = (el.textContent || '').trim();
                const missing = hasGlyph && familyLoaded && fontReady && glyphMissing(content, before.fontWeight, family);
                if (!hasGlyph || !familyLoaded || !fontReady || missing || (el.classList.contains('material-icons') && ligatureText)) {
                    out.icons.push({ el: sel(el), cls: String(el.className).slice(0, 60), content, family: family.slice(0, 60), fontReady, missing });
                }
            });

            // Visible text that leaks template/placeholder internals.
            const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
            let n;
            while ((n = walker.nextNode())) {
                const p = n.parentElement;
                if (!p || ['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEXTAREA', 'CODE', 'PRE'].includes(p.tagName)) continue;
                const t = n.textContent.trim();
                if (t && leak.test(t) && vis(p)) out.leaks.push(`${sel(p)}: ${t.slice(0, 80)}`);
            }

            // Clipped text: hidden overflow with no ellipsis, and the text is actually cut.
            document.querySelectorAll('body *').forEach((el) => {
                if (out.clipped.length >= 25) return;
                const cs = getComputedStyle(el);
                if (!vis(el)) return;
                const hides = ['hidden', 'clip'].includes(cs.overflowX);
                if (hides && el.scrollWidth > el.clientWidth + 2 && el.clientWidth > 0 && (el.textContent || '').trim()) {
                    const ellipsis = cs.textOverflow === 'ellipsis';
                    // Ellipsis without a tooltip means the user can never read the full value.
                    const hasTip = el.title || el.getAttribute('aria-label') || el.getAttribute('data-tooltip');
                    if (!ellipsis || !hasTip) {
                        out.clipped.push({ el: sel(el), ellipsis, tip: !!hasTip, text: el.textContent.trim().slice(0, 50), sw: el.scrollWidth, cw: el.clientWidth });
                    }
                }
            });

            // Elements poking outside the viewport horizontally.
            const vw = document.documentElement.clientWidth;
            let off = 0;
            document.querySelectorAll('body *').forEach((el) => {
                if (off >= 15 || !vis(el)) return;
                const r = el.getBoundingClientRect();
                if (r.right > vw + 2 && r.left < vw && !el.closest('[style*="overflow"], .table-responsive, .modal, pre, table')) {
                    out.overlap.push({ el: sel(el), right: Math.round(r.right), vw });
                    off++;
                }
            });

            // Touch targets (informational on small viewports).
            document.querySelectorAll('button, a[href], [role=button], input, select').forEach((el) => {
                if (!vis(el)) return;
                const r = el.getBoundingClientRect();
                if (r.width < 24 || r.height < 24) out.smallTargets++;
            });
            return out;
        }, LEAK_PATTERN.source);

        return {
            consoleErrors, pageErrors, failedRequests, badResponses,
            offOrigin: [...offOrigin],
            ...dom,
        };
    }

    return { collect };
}

module.exports = { attachGates };
