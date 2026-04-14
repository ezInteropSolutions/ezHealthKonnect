/**
 * app-dialogs.js — Global in-app dialog utilities.
 * Replaces browser alert(), confirm(), and prompt() with styled in-app equivalents.
 *
 * Usage:
 *   AppDialogs.alert('Message', { title: 'Info', type: 'info' })
 *   const ok = await AppDialogs.confirm('Delete item?', { title: 'Confirm', type: 'danger' })
 *   const value = await AppDialogs.prompt('Enter name:', { defaultValue: '' })
 *   AppDialogs.toast('Saved successfully', 'success')   // fire-and-forget toast
 */
(function () {
    'use strict';

    // ─── Ensure animation styles are injected once ────────────────────────────
    function ensureStyles() {
        if (document.getElementById('app-dialogs-styles')) return;
        const style = document.createElement('style');
        style.id = 'app-dialogs-styles';
        style.textContent = `
            @keyframes appDialogFadeIn  { from { opacity:0; } to { opacity:1; } }
            @keyframes appDialogScaleIn { from { opacity:0; transform:scale(.92); } to { opacity:1; transform:scale(1); } }
            @keyframes appToastSlideIn  { from { opacity:0; transform:translateX(110%); } to { opacity:1; transform:translateX(0); } }
            @keyframes appToastFadeOut  { from { opacity:1; } to { opacity:0; } }
            .app-dialog-overlay {
                position:fixed; inset:0;
                background:rgba(0,0,0,.5);
                display:flex; align-items:center; justify-content:center;
                z-index:999999;
                animation:appDialogFadeIn .18s ease;
            }
            .app-dialog-box {
                background:#fff; border-radius:12px; width:90%; max-width:420px;
                box-shadow:0 20px 40px rgba(0,0,0,.18);
                overflow:hidden;
                animation:appDialogScaleIn .18s ease;
            }
            .app-dialog-header {
                padding:14px 18px;
                display:flex; align-items:center; gap:10px;
                border-bottom:1px solid #e5e7eb;
                font-weight:600; font-size:15px;
            }
            .app-dialog-body {
                padding:16px 18px;
                font-size:14px; color:#374151; line-height:1.5;
            }
            .app-dialog-input {
                width:100%; margin-top:10px; padding:8px 10px;
                border:1px solid #d1d5db; border-radius:6px;
                font-size:14px; box-sizing:border-box;
                outline:none;
            }
            .app-dialog-input:focus { border-color:#3b82f6; box-shadow:0 0 0 2px rgba(59,130,246,.2); }
            .app-dialog-footer {
                padding:12px 18px;
                display:flex; justify-content:flex-end; gap:8px;
                border-top:1px solid #e5e7eb;
            }
            .app-dialog-btn {
                padding:8px 18px; border:none; border-radius:7px;
                font-size:13px; font-weight:600; cursor:pointer;
                transition:filter .15s;
            }
            .app-dialog-btn:hover { filter:brightness(.92); }
            .app-dialog-btn-cancel { background:#f3f4f6; color:#374151; }
            .app-dialog-btn-confirm { color:#fff; }
        `;
        document.head.appendChild(style);
    }

    // ─── Color palette by type ─────────────────────────────────────────────────
    const PALETTE = {
        info:    { bg:'#dbeafe', border:'#93c5fd', btn:'#3b82f6', icon:'ℹ️' },
        success: { bg:'#dcfce7', border:'#86efac', btn:'#16a34a', icon:'✅' },
        warning: { bg:'#fef3c7', border:'#fcd34d', btn:'#f59e0b', icon:'⚠️' },
        danger:  { bg:'#fee2e2', border:'#fca5a5', btn:'#ef4444', icon:'🗑️' },
        error:   { bg:'#fee2e2', border:'#fca5a5', btn:'#ef4444', icon:'❌' },
    };
    function palette(type) { return PALETTE[type] || PALETTE.info; }

    // ─── Build overlay + box ───────────────────────────────────────────────────
    function buildOverlay() {
        const overlay = document.createElement('div');
        overlay.className = 'app-dialog-overlay';
        overlay.addEventListener('click', function (e) {
            if (e.target === overlay) overlay._dismiss && overlay._dismiss();
        });
        return overlay;
    }

    // ─── alert ────────────────────────────────────────────────────────────────
    function alert(message, opts) {
        opts = opts || {};
        ensureStyles();
        const p = palette(opts.type || 'info');
        return new Promise(function (resolve) {
            const overlay = buildOverlay();
            overlay._dismiss = close;
            overlay.innerHTML = `
                <div class="app-dialog-box">
                    <div class="app-dialog-header" style="background:${p.bg};border-bottom-color:${p.border}">
                        <span>${p.icon}</span>
                        <span>${opts.title || 'Notice'}</span>
                    </div>
                    <div class="app-dialog-body">${message}</div>
                    <div class="app-dialog-footer">
                        <button class="app-dialog-btn app-dialog-btn-confirm" style="background:${p.btn}" id="_appDlgOk">
                            ${opts.okText || 'OK'}
                        </button>
                    </div>
                </div>`;
            document.body.appendChild(overlay);
            overlay.querySelector('#_appDlgOk').focus();
            overlay.querySelector('#_appDlgOk').addEventListener('click', close);
            function close() { overlay.remove(); resolve(); }
        });
    }

    // ─── confirm ──────────────────────────────────────────────────────────────
    function confirm(message, opts) {
        opts = opts || {};
        ensureStyles();
        const p = palette(opts.type || 'warning');
        return new Promise(function (resolve) {
            const overlay = buildOverlay();
            overlay._dismiss = function () { resolve(false); overlay.remove(); };
            overlay.innerHTML = `
                <div class="app-dialog-box">
                    <div class="app-dialog-header" style="background:${p.bg};border-bottom-color:${p.border}">
                        <span>${p.icon}</span>
                        <span>${opts.title || 'Confirm'}</span>
                    </div>
                    <div class="app-dialog-body">${message}</div>
                    <div class="app-dialog-footer">
                        <button class="app-dialog-btn app-dialog-btn-cancel" id="_appDlgCancel">
                            ${opts.cancelText || 'Cancel'}
                        </button>
                        <button class="app-dialog-btn app-dialog-btn-confirm" style="background:${p.btn}" id="_appDlgOk">
                            ${opts.confirmText || 'Confirm'}
                        </button>
                    </div>
                </div>`;
            document.body.appendChild(overlay);
            overlay.querySelector('#_appDlgOk').focus();
            overlay.querySelector('#_appDlgOk').addEventListener('click', function () { resolve(true);  overlay.remove(); });
            overlay.querySelector('#_appDlgCancel').addEventListener('click', function () { resolve(false); overlay.remove(); });
        });
    }

    // ─── prompt ───────────────────────────────────────────────────────────────
    function prompt(message, opts) {
        opts = opts || {};
        ensureStyles();
        const p = palette(opts.type || 'info');
        return new Promise(function (resolve) {
            const overlay = buildOverlay();
            overlay._dismiss = function () { resolve(null); overlay.remove(); };
            overlay.innerHTML = `
                <div class="app-dialog-box">
                    <div class="app-dialog-header" style="background:${p.bg};border-bottom-color:${p.border}">
                        <span>${p.icon}</span>
                        <span>${opts.title || 'Input'}</span>
                    </div>
                    <div class="app-dialog-body">
                        ${message}
                        <input class="app-dialog-input" id="_appDlgInput" type="text"
                               value="${(opts.defaultValue || '').replace(/"/g,'&quot;')}"
                               placeholder="${(opts.placeholder || '').replace(/"/g,'&quot;')}">
                    </div>
                    <div class="app-dialog-footer">
                        <button class="app-dialog-btn app-dialog-btn-cancel" id="_appDlgCancel">
                            ${opts.cancelText || 'Cancel'}
                        </button>
                        <button class="app-dialog-btn app-dialog-btn-confirm" style="background:${p.btn}" id="_appDlgOk">
                            ${opts.okText || 'OK'}
                        </button>
                    </div>
                </div>`;
            document.body.appendChild(overlay);
            const input = overlay.querySelector('#_appDlgInput');
            input.focus();
            input.select();
            input.addEventListener('keydown', function (e) {
                if (e.key === 'Enter') submit();
                if (e.key === 'Escape') cancel();
            });
            overlay.querySelector('#_appDlgOk').addEventListener('click', submit);
            overlay.querySelector('#_appDlgCancel').addEventListener('click', cancel);
            function submit() { const v = input.value; overlay.remove(); resolve(v); }
            function cancel() { overlay.remove(); resolve(null); }
        });
    }

    // ─── toast ────────────────────────────────────────────────────────────────
    const TOAST_BG = { success:'#16a34a', error:'#dc2626', warning:'#f59e0b', info:'#3b82f6' };
    function toast(message, type, durationMs) {
        ensureStyles();
        type = type || 'info';
        durationMs = durationMs || 4000;
        const el = document.createElement('div');
        el.style.cssText = `
            position:fixed; top:20px; right:20px; z-index:1000000;
            padding:12px 20px; border-radius:8px; color:#fff;
            background:${TOAST_BG[type] || TOAST_BG.info};
            font-size:14px; font-weight:500; max-width:360px;
            box-shadow:0 4px 12px rgba(0,0,0,.2);
            animation:appToastSlideIn .3s ease;
        `;
        el.textContent = message;
        document.body.appendChild(el);
        setTimeout(function () {
            el.style.animation = 'appToastFadeOut .4s ease forwards';
            setTimeout(function () { el.remove(); }, 400);
        }, durationMs);
    }

    window.AppDialogs = { alert: alert, confirm: confirm, prompt: prompt, toast: toast };
}());
