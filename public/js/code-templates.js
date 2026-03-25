/**
 * Code Templates — management page JS
 * Inspired by Mirth Connect Code Template Libraries.
 * Functions in a template are automatically injected into every
 * enrichment.script step's goja VM as a preamble.
 */
'use strict';

// ──────────────────────────────────────────────────────────────────────────────
// STATE
// ──────────────────────────────────────────────────────────────────────────────
let allTemplates = [];
let activeCategory = '';
let activeScope = '';         // '' = both, 'global', 'interface'
let selectedIfaceIds = [];    // interface IDs selected in the link picker

// ──────────────────────────────────────────────────────────────────────────────
// INIT
// ──────────────────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
    loadTemplates();
    loadInterfaces();
});

// ──────────────────────────────────────────────────────────────────────────────
// DATA LOADING
// ──────────────────────────────────────────────────────────────────────────────
async function loadTemplates() {
    try {
        const res = await fetch('/api/code-templates', { credentials: 'include' });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Load failed');
        allTemplates = json.data || [];
        updateCounts();
        renderGrid();
    } catch (e) {
        showToast('Failed to load templates: ' + e.message, true);
    }
}

async function loadInterfaces() {
    try {
        const res = await fetch('/api/interfaces', { credentials: 'include' });
        const json = await res.json();
        const ifaces = json.data || json.interfaces || [];
        const sel = document.getElementById('f-interfaces');
        ifaces.forEach(iface => {
            const opt = document.createElement('option');
            opt.value = iface.id;
            opt.textContent = iface.name || iface.id;
            sel.appendChild(opt);
        });
    } catch (_) { /* silently skip — interface linking still works via API */ }
}

// ──────────────────────────────────────────────────────────────────────────────
// FILTER / RENDER
// ──────────────────────────────────────────────────────────────────────────────
function updateCounts() {
    const categories = ['all', 'hl7', 'fhir', 'date_time', 'string', 'validation', 'general'];
    categories.forEach(cat => {
        const el = document.getElementById('count-' + cat);
        if (!el) return;
        const n = cat === 'all' ? allTemplates.length : allTemplates.filter(t => t.category === cat).length;
        el.textContent = n;
    });
}

function setCategory(el, cat) {
    document.querySelectorAll('.ct-filter[data-cat]').forEach(f => f.classList.remove('active'));
    el.classList.add('active');
    activeCategory = cat;
    renderGrid();
}

function toggleScope(el, scope) {
    if (el.classList.contains('active')) {
        el.classList.remove('active');
        activeScope = '';
    } else {
        document.querySelectorAll('.ct-filter[data-scope]').forEach(f => f.classList.remove('active'));
        el.classList.add('active');
        activeScope = scope;
    }
    renderGrid();
}

function renderGrid() {
    const query = (document.getElementById('searchInput')?.value || '').toLowerCase();
    let list = allTemplates;

    if (activeCategory) list = list.filter(t => t.category === activeCategory);
    if (activeScope)    list = list.filter(t => t.scope === activeScope);
    if (query) {
        list = list.filter(t =>
            t.name.toLowerCase().includes(query) ||
            (t.description || '').toLowerCase().includes(query) ||
            (t.function_signatures || []).some(s => s.toLowerCase().includes(query))
        );
    }

    document.getElementById('result-count').textContent = list.length + ' template' + (list.length === 1 ? '' : 's');

    const grid = document.getElementById('ct-grid');
    const empty = document.getElementById('ct-empty');
    grid.innerHTML = '';

    if (list.length === 0) {
        empty.style.display = '';
        return;
    }
    empty.style.display = 'none';
    list.forEach(t => grid.appendChild(buildCard(t)));
}

function buildCard(t) {
    const card = document.createElement('div');
    card.className = 'ct-card';

    const catIcons = { hl7:'hospital-alt', fhir:'fire', date_time:'calendar-alt', string:'font', validation:'check-circle', general:'cube' };
    const icon = catIcons[t.category] || 'code';

    const sigs = (t.function_signatures || []).slice(0, 5);
    const moreSigs = (t.function_signatures || []).length - 5;

    card.innerHTML = `
        <div class="ct-card-header">
            <div class="ct-card-icon"><i class="fas fa-${icon}"></i></div>
            <div style="flex:1;min-width:0">
                <div class="ct-card-title" title="${esc(t.name)}">${esc(t.name)}</div>
                <div class="ct-card-meta">
                    <span class="ct-badge ct-badge-cat">${esc(t.category.replace('_', ' '))}</span>
                    <span class="ct-badge ${t.scope === 'global' ? 'ct-badge-glob' : 'ct-badge-iface'}">
                        ${t.scope === 'global' ? '🌐 global' : '🔗 interface'}
                    </span>
                    ${t.is_system ? '<span class="ct-badge ct-badge-sys">system</span>' : ''}
                    ${!t.is_active ? '<span class="ct-badge ct-badge-off">inactive</span>' : ''}
                </div>
            </div>
        </div>
        ${t.description ? `<div class="ct-description">${esc(t.description)}</div>` : ''}
        ${sigs.length > 0 ? `
        <div class="ct-sigs">
            ${sigs.map(s => `<span class="ct-sig">${esc(s)}</span>`).join('')}
            ${moreSigs > 0 ? `<span class="ct-sig" style="color:#6b7280">+${moreSigs} more</span>` : ''}
        </div>` : ''}
        <div class="ct-card-footer">
            <span class="ct-usage">${t.usage_count || 0} use${t.usage_count !== 1 ? 's' : ''}</span>
            <span class="spacer"></span>
            <button class="btn btn-ghost" style="padding:5px 10px;font-size:12px" onclick="openModal(${esc(JSON.stringify(JSON.stringify(t)))})">
                <i class="fas fa-pencil-alt"></i> Edit
            </button>
            ${!t.is_system ? `
            <button class="btn btn-danger" style="padding:5px 10px;font-size:12px" onclick="deleteTemplate('${esc(t.id)}','${esc(t.name)}')">
                <i class="fas fa-trash"></i>
            </button>` : ''}
        </div>`;
    return card;
}

const esc = s => String(s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');

// ──────────────────────────────────────────────────────────────────────────────
// MODAL
// ──────────────────────────────────────────────────────────────────────────────
function openModal(templateJsonStr) {
    const t = templateJsonStr ? JSON.parse(templateJsonStr) : null;
    document.getElementById('modal-title').textContent = t ? 'Edit Code Template' : 'New Code Template';
    document.getElementById('edit-id').value = t ? t.id : '';
    document.getElementById('f-name').value = t ? t.name : '';
    document.getElementById('f-description').value = t ? (t.description || '') : '';
    document.getElementById('f-category').value = t ? t.category : 'general';
    document.getElementById('f-code').value = t ? t.code : '';
    document.getElementById('f-active').checked = t ? t.is_active : true;

    // Scope radios
    const scope = t ? t.scope : 'global';
    document.querySelectorAll('input[name="scope"]').forEach(r => { r.checked = r.value === scope; });
    onScopeChange();

    // Signatures
    renderSigTags(t ? (t.function_signatures || []) : []);

    // Interface links (loaded async when editing interface-scoped templates)
    selectedIfaceIds = [];
    renderIfaceChips();

    // Lock code field for system templates
    const codeField = document.getElementById('f-code');
    codeField.readOnly = t && t.is_system;
    codeField.style.opacity = (t && t.is_system) ? '0.6' : '1';

    document.getElementById('modal').classList.add('open');
    document.getElementById('f-name').focus();
}

function closeModal() {
    document.getElementById('modal').classList.remove('open');
}

// Close on overlay click
document.getElementById('modal').addEventListener('click', e => {
    if (e.target === document.getElementById('modal')) closeModal();
});

// ──────────────────────────────────────────────────────────────────────────────
// SCOPE CHANGE
// ──────────────────────────────────────────────────────────────────────────────
function onScopeChange() {
    const isIface = document.querySelector('input[name="scope"]:checked')?.value === 'interface';
    document.getElementById('iface-selector').classList.toggle('visible', isIface);
}

// ──────────────────────────────────────────────────────────────────────────────
// FUNCTION SIGNATURES TAG INPUT
// ──────────────────────────────────────────────────────────────────────────────
function getSigs() {
    return Array.from(document.getElementById('sig-tags').querySelectorAll('.sig-tag')).map(el => el.dataset.sig);
}
function renderSigTags(sigs) {
    const container = document.getElementById('sig-tags');
    container.innerHTML = '';
    sigs.forEach(sig => {
        const tag = document.createElement('div');
        tag.className = 'sig-tag';
        tag.dataset.sig = sig;
        tag.innerHTML = `<code>${esc(sig)}</code><button type="button" onclick="removeSig(this)" title="Remove">&times;</button>`;
        container.appendChild(tag);
    });
}
function addSig() {
    const input = document.getElementById('sig-input');
    const val = input.value.trim();
    if (!val) return;
    const existing = getSigs();
    if (!existing.includes(val)) renderSigTags([...existing, val]);
    input.value = '';
    input.focus();
}
function removeSig(btn) {
    btn.closest('.sig-tag').remove();
}
function onSigKey(event) {
    if (event.key === 'Enter') { event.preventDefault(); addSig(); }
}

// ──────────────────────────────────────────────────────────────────────────────
// INTERFACE CHIP PICKER
// ──────────────────────────────────────────────────────────────────────────────
function addIfaceChip(select) {
    const id = select.value;
    const name = select.options[select.selectedIndex]?.text;
    if (!id || selectedIfaceIds.includes(id)) { select.value = ''; return; }
    selectedIfaceIds.push(id);
    renderIfaceChips();
    select.value = '';
}
function removeIfaceChip(id) {
    selectedIfaceIds = selectedIfaceIds.filter(x => x !== id);
    renderIfaceChips();
}
function renderIfaceChips() {
    const container = document.getElementById('iface-chips');
    container.innerHTML = '';
    selectedIfaceIds.forEach(id => {
        const sel = document.getElementById('f-interfaces');
        const name = Array.from(sel.options).find(o => o.value === id)?.text || id;
        const chip = document.createElement('div');
        chip.className = 'iface-chip';
        chip.innerHTML = `${esc(name)}<button type="button" onclick="removeIfaceChip('${esc(id)}')">&times;</button>`;
        container.appendChild(chip);
    });
}

// ──────────────────────────────────────────────────────────────────────────────
// SAVE
// ──────────────────────────────────────────────────────────────────────────────
async function saveTemplate() {
    const id = document.getElementById('edit-id').value;
    const name = document.getElementById('f-name').value.trim();
    const code = document.getElementById('f-code').value.trim();

    if (!name) { showToast('Name is required', true); return; }
    if (!code)  { showToast('Code is required', true); return; }

    const payload = {
        name,
        description: document.getElementById('f-description').value.trim(),
        category:    document.getElementById('f-category').value,
        code,
        function_signatures: getSigs(),
        scope:    document.querySelector('input[name="scope"]:checked')?.value || 'global',
        is_active: document.getElementById('f-active').checked,
        sort_order: 100,
    };

    try {
        const url    = id ? `/api/code-templates/${id}` : '/api/code-templates';
        const method = id ? 'PUT' : 'POST';
        const res  = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Save failed');

        // If interface-scoped and interfaces selected, link them
        if (payload.scope === 'interface' && selectedIfaceIds.length > 0) {
            const savedId = json.data?.id || id;
            await Promise.all(selectedIfaceIds.map(ifaceId =>
                fetch('/api/code-templates/link', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ template_id: savedId, interface_id: ifaceId }),
                })
            ));
        }

        showToast(id ? 'Template updated' : 'Template created');
        closeModal();
        loadTemplates();
    } catch (e) {
        showToast('Save failed: ' + e.message, true);
    }
}

// ──────────────────────────────────────────────────────────────────────────────
// DELETE
// ──────────────────────────────────────────────────────────────────────────────
async function deleteTemplate(id, name) {
    if (!confirm(`Delete code template "${name}"? This cannot be undone.`)) return;
    try {
        const res = await fetch(`/api/code-templates/${id}`, { method: 'DELETE' });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Delete failed');
        showToast('Template deleted');
        loadTemplates();
    } catch (e) {
        showToast('Delete failed: ' + e.message, true);
    }
}

// ──────────────────────────────────────────────────────────────────────────────
// INVALIDATE CACHE
// ──────────────────────────────────────────────────────────────────────────────
async function invalidateCache() {
    try {
        const res = await fetch('/api/code-templates/invalidate-cache', { method: 'POST' });
        const json = await res.json();
        if (!json.success) throw new Error(json.error || 'Failed');
        showToast('Cache flushed — next script execution will reload from DB');
    } catch (e) {
        showToast('Cache flush failed: ' + e.message, true);
    }
}

// ──────────────────────────────────────────────────────────────────────────────
// TOAST
// ──────────────────────────────────────────────────────────────────────────────
let toastTimer;
function showToast(msg, isError = false) {
    const el = document.getElementById('toast');
    el.textContent = msg;
    el.className = 'toast show' + (isError ? ' error' : '');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.remove('show'), 3500);
}
