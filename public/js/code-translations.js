/**
 * Code Translations — management page.
 * Lists, adds, edits, deletes, imports and exports the rows of cda_code_translations through
 * /api/cda/code-translations. A row is either shared (every interface) or belongs to one interface.
 *
 * Three small classes, wired together at the bottom:
 *   TranslationsApi       – the HTTP calls
 *   CsvCodec              – CSV text <-> row objects
 *   CodeTranslationsPage  – state, rendering and user actions
 */
'use strict';

class TranslationsApi {
    constructor(baseUrl = '/api/cda/code-translations') {
        this.baseUrl = baseUrl;
    }

    async _request(method, url, body) {
        const res = await fetch(url, {
            method,
            credentials: 'include',
            headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body)
        });
        let json = {};
        try { json = await res.json(); } catch (_) { /* non-JSON error body */ }
        if (!res.ok || json.success === false) {
            // A refused write already raised a toast through AppAuth; still reject so callers stop.
            throw new Error(json.error || json.message || `Request failed (HTTP ${res.status})`);
        }
        return json;
    }

    list({ scope, interfaceId, search, limit, offset }) {
        const q = new URLSearchParams({ scope, limit, offset });
        if (interfaceId) q.set('interfaceId', interfaceId);
        if (search) q.set('search', search);
        return this._request('GET', `${this.baseUrl}?${q}`);
    }

    create(row) { return this._request('POST', this.baseUrl, row); }
    update(id, row) { return this._request('PUT', `${this.baseUrl}/${encodeURIComponent(id)}`, row); }
    remove(id) { return this._request('DELETE', `${this.baseUrl}/${encodeURIComponent(id)}`); }
    importRows(rows) { return this._request('POST', `${this.baseUrl}/import`, { rows }); }
}

class CsvCodec {
    static COLUMNS = ['sourceSystem', 'sourceCode', 'targetSystem', 'targetCode', 'targetDisplay'];

    // Header names are matched ignoring case, spaces, dashes and underscores ("Source System" = sourceSystem).
    static _key(h) { return String(h).toLowerCase().replace(/[^a-z0-9]/g, ''); }

    /** Parses RFC 4180-style CSV (quoted fields, doubled quotes, CRLF or LF). Returns an array of string arrays. */
    static _records(text) {
        const out = [];
        let row = [], field = '', inQuotes = false;
        const src = text.replace(/^﻿/, '');
        for (let i = 0; i < src.length; i++) {
            const ch = src[i];
            if (inQuotes) {
                if (ch === '"' && src[i + 1] === '"') { field += '"'; i++; }
                else if (ch === '"') inQuotes = false;
                else field += ch;
            } else if (ch === '"') inQuotes = true;
            else if (ch === ',') { row.push(field); field = ''; }
            else if (ch === '\n' || ch === '\r') {
                if (ch === '\r' && src[i + 1] === '\n') i++;
                row.push(field); field = '';
                if (row.some(c => c.trim() !== '')) out.push(row);
                row = [];
            } else field += ch;
        }
        row.push(field);
        if (row.some(c => c.trim() !== '')) out.push(row);
        return out;
    }

    /** @returns {{rows: object[], error: string|null}} */
    static parse(text) {
        const records = CsvCodec._records(text);
        if (records.length === 0) return { rows: [], error: 'The file is empty.' };
        const header = records[0].map(CsvCodec._key);
        const index = {};
        for (const col of CsvCodec.COLUMNS) index[col] = header.indexOf(CsvCodec._key(col));
        const missing = CsvCodec.COLUMNS.filter(c => c !== 'targetDisplay' && index[c] < 0);
        if (missing.length) {
            return { rows: [], error: `The first row must be a header containing: ${CsvCodec.COLUMNS.join(', ')}. Missing: ${missing.join(', ')}.` };
        }
        const rows = records.slice(1).map(r => {
            const o = {};
            for (const col of CsvCodec.COLUMNS) o[col] = index[col] >= 0 ? (r[index[col]] || '').trim() : '';
            return o;
        });
        return { rows, error: rows.length ? null : 'The file has a header but no rows.' };
    }

    static stringify(items) {
        const esc = v => {
            const s = v == null ? '' : String(v);
            return /[",\r\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
        };
        const lines = [CsvCodec.COLUMNS.join(',')];
        for (const it of items) lines.push(CsvCodec.COLUMNS.map(c => esc(it[c])).join(','));
        return lines.join('\r\n') + '\r\n';
    }
}

class CodeTranslationsPage {
    static PAGE_SIZE = 50;
    static IMPORT_CHUNK = 1000;

    constructor(api) {
        this.api = api;
        this.interfaces = [];            // [{id, name}]
        this.items = [];
        this.total = 0;
        this.offset = 0;
        this.searchTimer = null;
        this.el = {
            scope: document.getElementById('scopeSelect'),
            search: document.getElementById('searchInput'),
            area: document.getElementById('tableArea'),
            summary: document.getElementById('summary'),
            prev: document.getElementById('prevBtn'),
            next: document.getElementById('nextBtn'),
            pageInfo: document.getElementById('pageInfo'),
            add: document.getElementById('addBtn'),
            importBtn: document.getElementById('importBtn'),
            importFile: document.getElementById('importFile'),
            exportBtn: document.getElementById('exportBtn')
        };
    }

    async init() {
        this._bind();
        // Dialogs that offer the interface list wait on this, so clicking Add straight after the page opens
        // never shows an empty list.
        this.interfacesReady = this._loadInterfaces();
        await this.interfacesReady;
        await this.reload();
    }

    _bind() {
        this.el.scope.addEventListener('change', () => { this.offset = 0; this.reload(); });
        this.el.search.addEventListener('input', () => {
            clearTimeout(this.searchTimer);
            this.searchTimer = setTimeout(() => { this.offset = 0; this.reload(); }, 300);
        });
        this.el.prev.addEventListener('click', () => { this.offset = Math.max(0, this.offset - CodeTranslationsPage.PAGE_SIZE); this.reload(); });
        this.el.next.addEventListener('click', () => { this.offset += CodeTranslationsPage.PAGE_SIZE; this.reload(); });
        this.el.add.addEventListener('click', () => this.openEditor(null));
        this.el.importBtn.addEventListener('click', () => this.el.importFile.click());
        this.el.importFile.addEventListener('change', () => this._onImportFile());
        this.el.exportBtn.addEventListener('click', () => this.exportCsv());
        // Row actions (rows are re-rendered, so one delegated listener)
        this.el.area.addEventListener('click', (e) => {
            const btn = e.target.closest('button[data-action]');
            if (!btn) return;
            const item = this.items.find(i => i.id === btn.dataset.id);
            if (!item) return;
            if (btn.dataset.action === 'edit') this.openEditor(item);
            if (btn.dataset.action === 'delete') this.confirmDelete(item);
        });
    }

    async _loadInterfaces() {
        try {
            const res = await fetch('/api/interfaces', { credentials: 'include' });
            const json = await res.json();
            const list = json.data || json.interfaces || [];
            this.interfaces = list.map(i => ({ id: i.id, name: i.name || i.id }))
                .sort((a, b) => a.name.localeCompare(b.name));
        } catch (_) {
            this.interfaces = []; // the page still works for shared translations
        }
        for (const i of this.interfaces) {
            const opt = document.createElement('option');
            opt.value = i.id;
            opt.textContent = `Interface: ${i.name}`;
            this.el.scope.appendChild(opt);
        }
    }

    // ── The scope select holds: all | global | <interface id>
    _filter() {
        const v = this.el.scope.value;
        return {
            scope: v === 'all' || v === 'global' ? v : 'interface',
            interfaceId: v === 'all' || v === 'global' ? '' : v,
            search: this.el.search.value.trim()
        };
    }

    async reload() {
        try {
            const res = await this.api.list({ ...this._filter(), limit: CodeTranslationsPage.PAGE_SIZE, offset: this.offset });
            this.items = res.data || [];
            this.total = res.total || 0;
            if (this.items.length === 0 && this.offset > 0) { // deleted the last row of a page
                this.offset = Math.max(0, this.offset - CodeTranslationsPage.PAGE_SIZE);
                return this.reload();
            }
            this.render();
        } catch (e) {
            AppDialogs.toast(`Could not load translations: ${e.message}`, 'error');
        }
    }

    render() {
        const size = CodeTranslationsPage.PAGE_SIZE;
        if (this.items.length === 0) {
            this.el.area.innerHTML = HtmlSafe.html`<div class="tr-empty"><i class="fas fa-language"></i>
                <p>${this.el.search.value.trim() ? 'No translations match your search.' : 'No translations yet. Add one, or import a CSV file.'}</p></div>`;
        } else {
            this.el.area.innerHTML = HtmlSafe.html`<table class="tr-table">
                <thead><tr><th>Applies to</th><th>Source system</th><th>Source code</th><th>Target system</th><th>Target code</th><th>Display</th><th><span class="sr-only">Actions</span></th></tr></thead>
                <tbody>${this.items.map(i => this._row(i))}</tbody></table>`;
        }
        const from = this.total === 0 ? 0 : this.offset + 1;
        const to = Math.min(this.offset + size, this.total);
        this.el.summary.textContent = `${this.total} translation${this.total === 1 ? '' : 's'}`;
        this.el.pageInfo.textContent = this.total === 0 ? '' : `Showing ${from}–${to} of ${this.total}`;
        this.el.prev.disabled = this.offset === 0;
        this.el.next.disabled = this.offset + size >= this.total;
    }

    _row(i) {
        const scope = i.interfaceId
            ? HtmlSafe.html`<span class="badge badge-iface" title="Only this interface">${i.interfaceName || 'Interface'}</span>`
            : HtmlSafe.html`<span class="badge badge-shared" title="Every interface">Shared</span>`;
        return HtmlSafe.html`<tr>
            <td>${scope}</td>
            <td class="mono">${i.sourceSystem}</td><td class="mono">${i.sourceCode}</td>
            <td class="mono">${i.targetSystem}</td><td class="mono">${i.targetCode}</td>
            <td>${i.targetDisplay || ''}</td>
            <td class="actions">
                <button class="btn btn-ghost btn-sm" type="button" data-action="edit" data-id="${i.id}" data-requires="write"><i class="fas fa-pen"></i> Edit</button>
                <button class="btn btn-danger-ghost btn-sm" type="button" data-action="delete" data-id="${i.id}" data-requires="write"><i class="fas fa-trash"></i> Delete</button>
            </td></tr>`;
    }

    _appliesToOptions() {
        return [{ value: '', label: 'Shared (all interfaces)' },
            ...this.interfaces.map(i => ({ value: i.id, label: `Interface: ${i.name}` }))];
    }

    _rowFields(item) {
        return [
            { name: 'sourceSystem', label: 'Source system', required: true, value: item?.sourceSystem, placeholder: 'e.g. urn:oid:1.2.840.113883.3.1234 or LAB-LOCAL' },
            { name: 'sourceCode', label: 'Source code', required: true, value: item?.sourceCode },
            { name: 'targetSystem', label: 'Target system', required: true, value: item?.targetSystem || 'http://loinc.org', placeholder: 'e.g. http://loinc.org' },
            { name: 'targetCode', label: 'Target code', required: true, value: item?.targetCode },
            { name: 'targetDisplay', label: 'Display text (optional)', value: item?.targetDisplay }
        ];
    }

    async openEditor(item) {
        await this.interfacesReady;
        const editing = !!item;
        const f = this._filter();
        const fields = this._rowFields(item);
        if (!editing) {
            fields.unshift({ name: 'interfaceId', label: 'Applies to', options: this._appliesToOptions(), value: f.interfaceId });
        }
        const saved = await AppDialogs.form({
            title: editing ? 'Edit translation' : 'Add translation',
            description: editing ? `Applies to: ${item.interfaceId ? (item.interfaceName || 'one interface') : 'every interface (shared)'}` : '',
            submitText: editing ? 'Save changes' : 'Add translation',
            fields,
            onSubmit: (v) => editing ? this.api.update(item.id, v) : this.api.create(v)
        });
        if (saved) {
            AppDialogs.toast(editing ? 'Translation updated' : 'Translation added', 'success');
            await this.reload();
        }
    }

    async confirmDelete(item) {
        const ok = await AppDialogs.confirm(
            `Delete the translation ${item.sourceCode} → ${item.targetCode}? Messages that use it will keep the original code.`,
            { title: 'Delete translation', type: 'danger', confirmText: 'Delete' });
        if (!ok) return;
        try {
            await this.api.remove(item.id);
            AppDialogs.toast('Translation deleted', 'success');
            await this.reload();
        } catch (e) {
            AppDialogs.toast(`Could not delete: ${e.message}`, 'error');
        }
    }

    // ── CSV import ────────────────────────────────────────────────────────────
    async _onImportFile() {
        const file = this.el.importFile.files[0];
        this.el.importFile.value = ''; // allow picking the same file again
        if (!file) return;
        const { rows, error } = CsvCodec.parse(await file.text());
        if (error) { await AppDialogs.alert(error, { title: 'Cannot import this file', type: 'error' }); return; }
        await this.interfacesReady;

        const result = { saved: 0, rejected: [] };
        const done = await AppDialogs.form({
            title: `Import ${rows.length} translation${rows.length === 1 ? '' : 's'}`,
            description: 'Existing translations with the same source system, source code and target system are updated.',
            submitText: 'Import',
            fields: [{ name: 'interfaceId', label: 'Apply to', options: this._appliesToOptions(), value: this._filter().interfaceId }],
            onSubmit: async (v) => {
                for (let start = 0; start < rows.length; start += CodeTranslationsPage.IMPORT_CHUNK) {
                    const chunk = rows.slice(start, start + CodeTranslationsPage.IMPORT_CHUNK).map(r => ({ ...r, interfaceId: v.interfaceId }));
                    const res = await this.api.importRows(chunk);
                    result.saved += res.saved;
                    for (const r of res.rejected || []) result.rejected.push({ row: r.row + start, error: r.error });
                }
            }
        });
        if (!done) return;
        await this.reload();
        if (result.rejected.length === 0) {
            AppDialogs.toast(`Imported ${result.saved} translation${result.saved === 1 ? '' : 's'}`, 'success');
        } else {
            const shown = result.rejected.slice(0, 10).map(r => `Row ${r.row + 1}: ${r.error}`).join('\n'); // +1: the header is row 1
            const more = result.rejected.length > 10 ? `\n…and ${result.rejected.length - 10} more.` : '';
            await AppDialogs.alert(`${result.saved} saved, ${result.rejected.length} skipped.\n\n${shown}${more}`,
                { title: 'Import finished with problems', type: 'warning' });
        }
    }

    // ── CSV export of whatever the current filters select ────────────────────────
    async exportCsv() {
        try {
            const all = [];
            for (let offset = 0; ; offset += 500) {
                const res = await this.api.list({ ...this._filter(), limit: 500, offset });
                all.push(...(res.data || []));
                if (all.length >= res.total || (res.data || []).length === 0) break;
            }
            const blob = new Blob([CsvCodec.stringify(all)], { type: 'text/csv;charset=utf-8' });
            const a = document.createElement('a');
            a.href = URL.createObjectURL(blob);
            a.download = 'code-translations.csv';
            document.body.appendChild(a);
            a.click();
            a.remove();
            setTimeout(() => URL.revokeObjectURL(a.href), 1000);
        } catch (e) {
            AppDialogs.toast(`Could not export: ${e.message}`, 'error');
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    window.codeTranslationsPage = new CodeTranslationsPage(new TranslationsApi());
    window.codeTranslationsPage.init();
});
