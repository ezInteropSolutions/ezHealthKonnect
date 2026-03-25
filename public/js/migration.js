'use strict';

// ============================================================================
// State
// ============================================================================

let currentXMLText = null;      // raw channel XML string
let currentPreview  = null;     // MigrationPreview object from server

const MAX_INTERFACE_NAME_LEN = 255;
const MAX_DESCRIPTION_LEN    = 1000;

// ============================================================================
// Tab navigation
// ============================================================================

function switchTab(tab) {
    document.querySelectorAll('.leftnav-tab').forEach(n => n.classList.remove('active'));
    const navEl = document.querySelector(`.leftnav-tab[data-tab="${tab}"]`);
    if (navEl) navEl.classList.add('active');

    document.querySelectorAll('.tab-section').forEach(el => el.classList.remove('active'));
    const target = document.getElementById('tab-' + tab);
    if (target) target.classList.add('active');

    if (tab === 'history') loadHistory();
}

// ============================================================================
// Upload zone interactions
// ============================================================================

function onDragOver(e) {
    e.preventDefault();
    document.getElementById('upload-zone').classList.add('dragover');
}

function onDragLeave(e) {
    document.getElementById('upload-zone').classList.remove('dragover');
}

function onDrop(e) {
    e.preventDefault();
    document.getElementById('upload-zone').classList.remove('dragover');
    const file = e.dataTransfer.files[0];
    if (file) processFile(file);
}

function onFileSelected(e) {
    const file = e.target.files[0];
    if (file) processFile(file);
}

function processFile(file) {
    if (!file.name.toLowerCase().endsWith('.xml')) {
        showToast('Please select a Mirth Connect .xml channel export file.', 'error');
        return;
    }
    const MAX_BYTES = 10 * 1024 * 1024; // 10 MB — matches server limit
    if (file.size > MAX_BYTES) {
        showToast(`File is too large (${(file.size / 1024 / 1024).toFixed(1)} MB). Maximum is 10 MB.`, 'error');
        return;
    }

    const reader = new FileReader();
    reader.onload = (e) => {
        currentXMLText = e.target.result;
        fetchPreview(currentXMLText);
    };
    reader.readAsText(file);
}

// ============================================================================
// Preview
// ============================================================================

async function fetchPreview(xmlText) {
    setUploadZoneLoading(true);

    try {
        const res = await fetch('/api/migration/mirth/preview', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ channelXml: xmlText }),
            credentials: 'include',
        });

        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Preview failed');

        currentPreview = data.preview;
        renderPreview(currentPreview);

    } catch (err) {
        showToast('Failed to analyse channel: ' + err.message, 'error');
    } finally {
        setUploadZoneLoading(false);
    }
}

function setUploadZoneLoading(loading) {
    const zone = document.getElementById('upload-zone');
    if (loading) {
        zone.innerHTML = `<div><i class="fas fa-spinner fa-spin" style="font-size:32px;color:#6b7280"></i></div>
            <h3 style="margin-top:12px">Analysing channel…</h3>`;
    }
}

function renderPreview(p) {
    // Hide upload zone, reveal Start over button
    document.getElementById('upload-zone').style.display = 'none';
    document.getElementById('btn-reset').style.display = 'inline-flex';

    // Channel info
    document.getElementById('channel-info').innerHTML = `
        <h2>${esc(p.channelName)}</h2>
        ${p.description ? `<p style="font-size:13px;color:#6b7280;margin:4px 0 0">${esc(p.description)}</p>` : ''}
        <div class="channel-meta">
            <div class="channel-meta-item"><i class="fas fa-tag"></i> Channel ID: <strong>${esc(p.channelId)}</strong></div>
            <div class="channel-meta-item"><i class="fas fa-code-branch"></i> Mirth Version: <strong>${esc(p.mirthVersion || 'unknown')}</strong></div>
            <div class="channel-meta-item"><i class="fas fa-toggle-${p.enabled ? 'on' : 'off'}"></i> Status: <strong>${p.enabled ? 'Enabled' : 'Disabled'}</strong></div>
        </div>`;

    // Summary badges
    const s = p.summary;
    document.getElementById('summary-row').innerHTML = `
        <div class="summary-badge auto"><i class="fas fa-check-circle"></i> ${s.autoMapped} auto-mapped</div>
        <div class="summary-badge review"><i class="fas fa-exclamation-circle"></i> ${s.needsReview} need review</div>
        ${s.unsupported ? `<div class="summary-badge unsupported"><i class="fas fa-times-circle"></i> ${s.unsupported} unsupported</div>` : ''}
        <div class="summary-badge" style="background:#f3f4f6;color:#374151"><i class="fas fa-list"></i> ${s.totalSteps} steps</div>`;

    // Source connector
    document.getElementById('source-body').innerHTML = renderConnectorRow(p.source);

    // Destinations
    if (p.destinations && p.destinations.length > 0) {
        document.getElementById('destinations-body').innerHTML =
            p.destinations.map(d => renderConnectorRow(d)).join('');
    } else {
        document.getElementById('destinations-body').innerHTML =
            '<p style="font-size:13px;color:#9ca3af;margin:0">No destination connectors found.</p>';
    }

    // Steps
    document.getElementById('steps-badge').textContent =
        `— ${p.steps.length} step${p.steps.length !== 1 ? 's' : ''}`;
    const tbody = document.getElementById('steps-tbody');
    tbody.innerHTML = p.steps.map((st, i) => renderStepRow(st, i)).join('');

    // Import form
    const nameInput = document.getElementById('input-interface-name');
    nameInput.value = p.channelName;
    document.getElementById('input-description').value =
        p.description || `Migrated from Mirth Connect (channel: ${p.channelName})`;

    showPanel('preview-panel');
    document.getElementById('import-form').classList.add('visible');
}

function renderConnectorRow(c) {
    const pillClass = c.status === 'auto' ? 'pill-auto' : c.status === 'review' ? 'pill-review' : 'pill-unsupported';
    const pillLabel = c.status === 'auto' ? 'Auto' : c.status === 'review' ? 'Review' : 'Unsupported';

    const configPreview = Object.keys(c.config || {}).length > 0
        ? Object.entries(c.config).map(([k, v]) => `${k}: ${v}`).join(' · ')
        : '';

    return `
        <div class="connector-row">
            <div class="connector-mirth"><i class="fas fa-plug" style="color:#9ca3af"></i> ${esc(c.mirthTransport)}</div>
            <div class="connector-arrow"><i class="fas fa-arrow-right"></i></div>
            <div>
                <div class="connector-ezhk">${esc(c.ezHKConnectorType || '—')}</div>
                ${configPreview ? `<div class="connector-config-preview">${esc(configPreview)}</div>` : ''}
            </div>
            <span class="pill ${pillClass}">${pillLabel}</span>
            ${c.note ? `<span style="font-size:11px;color:#9ca3af;max-width:220px">${esc(c.note)}</span>` : ''}
        </div>`;
}

function renderStepRow(st, idx) {
    const typeIcon = st.isFilterRule ? '🔍' : '⚙️';
    const conversion = st.conversion || (st.autoConverted ? 'auto' : 'manual');

    // Badge HTML based on conversion state
    function convBadge(state) {
        if (state === 'auto')   return '<span class="conv-badge conv-auto"><i class="fas fa-check"></i> Auto-converted</span>';
        if (state === 'ai')     return '<span class="conv-badge conv-ai"><i class="fas fa-robot"></i> AI-rewritten</span>';
        return '';
    }

    // --- Mapper / field_mapping step ---
    if (conversion === 'auto' && st.fieldMappings && st.fieldMappings.length > 0) {
        const rows = st.fieldMappings.map(m => `
            <tr>
                <td style="font-family:monospace;font-size:11px;color:#6d28d9">${esc(m.source)}</td>
                <td style="font-size:11px;color:#9ca3af;padding:0 6px">→</td>
                <td style="font-family:monospace;font-size:11px;color:#065f46">${esc(m.target)}</td>
                ${m.defaultValue ? `<td style="font-size:10px;color:#9ca3af">${esc(m.defaultValue)}</td>` : '<td></td>'}
            </tr>`).join('');

        return `
            <tr id="step-row-${idx}">
                <td><input type="checkbox" class="skip-checkbox" id="skip-${idx}" onchange="onSkipChange(${idx})"></td>
                <td style="color:#9ca3af">${st.sequence}</td>
                <td>
                    <div style="font-weight:500;margin-bottom:6px">⚙️ ${esc(st.name)} ${convBadge(conversion)}</div>
                    <table class="mapping-table">
                        <thead><tr>
                            <th>HL7 Field (source)</th><th></th>
                            <th>Variable (target)</th>
                            <th>Default</th>
                        </tr></thead>
                        <tbody>${rows}</tbody>
                    </table>
                </td>
                <td><code style="font-size:11px">${esc(st.mirthType)}</code></td>
                <td><code style="font-size:11px;color:#7c3aed">${esc(st.suggestedType)}</code></td>
                <td style="font-size:11px;color:#6b7280;max-width:200px">${esc(st.note || '')}</td>
            </tr>`;
    }

    // --- JavaScript / script step (manual or ai-rewritten) ---
    const hasScript = st.script && st.script.trim() !== '';
    const aiDone    = conversion === 'ai';
    const canRewrite = !aiDone && hasScript &&
        (st.mirthType === 'JavaScript' || st.mirthType === 'Message Builder' || st.mirthType === 'XSLT');

    const scriptSection = hasScript ? `
        <div style="margin-top:6px">
            <span class="step-script-toggle" onclick="toggleScript(${idx})">
                ${aiDone ? 'View rewritten script' : 'View script'}
            </span>
            ${aiDone && st.originalScript ? `
                <span class="step-script-toggle" style="margin-left:8px;color:#9ca3af"
                    onclick="toggleOriginalScript(${idx})">View original</span>` : ''}
            <div class="step-script-block" id="script-block-${idx}"><pre id="script-pre-${idx}">${esc(st.script)}</pre></div>
            ${aiDone && st.originalScript ? `
                <div class="step-script-block" id="orig-block-${idx}">
                    <div style="font-size:10px;color:#9ca3af;margin-bottom:4px">ORIGINAL MIRTH SCRIPT:</div>
                    <pre>${esc(st.originalScript)}</pre>
                </div>` : ''}
        </div>` : '';

    const rewriteBtn = canRewrite ? `
        <button class="btn-rewrite" id="rewrite-btn-${idx}"
            onclick="rewriteWithAI(${idx})"
            title="Use AI to convert this Mirth script to ezHealthKonnect-compatible JavaScript">
            <i class="fas fa-robot"></i> Rewrite with AI
        </button>` : '';

    return `
        <tr id="step-row-${idx}">
            <td><input type="checkbox" class="skip-checkbox" id="skip-${idx}" onchange="onSkipChange(${idx})"></td>
            <td style="color:#9ca3af">${st.sequence}</td>
            <td>
                <div style="font-weight:500;display:flex;align-items:center;gap:8px;flex-wrap:wrap">
                    ${typeIcon} ${esc(st.name)}
                    ${convBadge(conversion)}
                    ${rewriteBtn}
                </div>
                ${scriptSection}
            </td>
            <td><code style="font-size:11px">${esc(st.mirthType)}</code>${st.isFilterRule ? ' <span style="font-size:10px;color:#9ca3af">(filter)</span>' : ''}</td>
            <td><code style="font-size:11px;color:#7c3aed">${esc(st.suggestedType)}</code></td>
            <td style="font-size:11px;color:#6b7280;max-width:200px">${esc(st.note || '')}</td>
        </tr>`;
}

function toggleScript(idx) {
    const block = document.getElementById('script-block-' + idx);
    block.classList.toggle('visible');
}

function toggleOriginalScript(idx) {
    const block = document.getElementById('orig-block-' + idx);
    if (block) block.classList.toggle('visible');
}

function onSkipChange(idx) {
    const row = document.getElementById('step-row-' + idx);
    row.classList.toggle('skipped', document.getElementById('skip-' + idx).checked);
}

// ============================================================================
// AI Step Rewriter
// ============================================================================

async function rewriteWithAI(idx) {
    if (!currentPreview || !currentPreview.steps[idx]) return;
    const st = currentPreview.steps[idx];

    const btn = document.getElementById('rewrite-btn-' + idx);
    if (btn) {
        btn.disabled = true;
        btn.innerHTML = '<span class="spinner" style="width:12px;height:12px;border-width:2px"></span> Rewriting…';
    }

    try {
        const res = await fetch('/api/migration/mirth/rewrite-step', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                script:   st.script,
                stepName: st.name,
                stepType: st.mirthType,
            }),
        });

        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'AI rewrite failed');

        // Save original and update state in currentPreview so import uses rewritten script
        if (!st.originalScript) st.originalScript = st.script;
        st.script     = data.rewrittenScript;
        st.conversion = 'ai';
        st.note       = data.note || st.note;

        // Re-render this row in place
        const row = document.getElementById('step-row-' + idx);
        if (row) {
            const tmp = document.createElement('tbody');
            tmp.innerHTML = renderStepRow(st, idx);
            row.replaceWith(tmp.firstElementChild);
        }

        showToast('Step rewritten by AI — review before importing.', 'success');

    } catch (err) {
        showToast('AI rewrite failed: ' + err.message, 'error');
        if (btn) {
            btn.disabled = false;
            btn.innerHTML = '<i class="fas fa-robot"></i> Rewrite with AI';
        }
    }
}

// ============================================================================
// Import
// ============================================================================

async function doImport() {
    if (!currentXMLText || !currentPreview) return;

    const interfaceName = document.getElementById('input-interface-name').value.trim();
    const description   = document.getElementById('input-description').value.trim();

    if (!interfaceName) {
        showToast('Please enter an interface name.', 'error');
        return;
    }
    if (interfaceName.length > MAX_INTERFACE_NAME_LEN) {
        showToast(`Interface name must not exceed ${MAX_INTERFACE_NAME_LEN} characters.`, 'error');
        return;
    }
    if (description.length > MAX_DESCRIPTION_LEN) {
        showToast(`Description must not exceed ${MAX_DESCRIPTION_LEN} characters.`, 'error');
        return;
    }

    // Collect skipped step indices
    const skipIndices = [];
    currentPreview.steps.forEach((_, i) => {
        if (document.getElementById('skip-' + i)?.checked) skipIndices.push(i);
    });

    const btn = document.getElementById('btn-import');
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> Importing…';
    document.getElementById('import-status').textContent = '';

    try {
        const res = await fetch('/api/migration/mirth/import', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                channelXml:      currentXMLText,   // raw XML — server validates + size-checks
                interfaceName,
                description,
                skipStepIndices: skipIndices,
                // userId intentionally omitted — extracted server-side from X-User-ID header
            }),
        });

        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Import failed');

        const r = data.result;
        showToast(`✅ Interface created! ${r.stepsCreated} steps imported.`, 'success');

        document.getElementById('import-status').innerHTML =
            `Interface created: <strong>${esc(r.interfaceName)}</strong> —
             <a href="pipeline-builder.html?interfaceId=${r.interfaceId}" style="color:var(--navy)">Open in Pipeline Builder →</a>`;

        btn.disabled = false;
        btn.innerHTML = '<i class="fas fa-download"></i> Import Channel';

        if (r.warnings && r.warnings.length > 0) {
            console.warn('Migration warnings:', r.warnings);
        }

    } catch (err) {
        showToast('Import failed: ' + err.message, 'error');
        btn.disabled = false;
        btn.innerHTML = '<i class="fas fa-download"></i> Import Channel';
    }
}

// ============================================================================
// History
// ============================================================================

async function loadHistory() {
    const container = document.getElementById('history-content');
    container.innerHTML = '<p style="color:#6b7280;font-size:13px">Loading…</p>';

    try {
        const res = await fetch('/api/migration/mirth/history?limit=50', { credentials: 'include' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Failed to load history');

        if (!data.data || data.data.length === 0) {
            container.innerHTML = `
                <div style="background:#fff;border:1px solid #e5e7eb;border-radius:10px;padding:24px;text-align:center">
                    <i class="fas fa-history" style="font-size:32px;color:#d1d5db;margin-bottom:12px"></i>
                    <p style="font-size:14px;color:#6b7280;margin:0">No migrations yet. Import your first Mirth channel from the Mirth Connect tab.</p>
                </div>`;
            return;
        }

        const rows = data.data.map(e => `
            <tr>
                <td>${new Date(e.createdAt).toLocaleString()}</td>
                <td><strong>${esc(e.channelName)}</strong><br><span style="font-size:11px;color:#9ca3af">${esc(e.channelId)}</span></td>
                <td>${esc(e.mirthVersion || '—')}</td>
                <td>${e.stepsCreated}</td>
                <td>${e.stepsSkipped}</td>
                <td>
                    ${e.interfaceId
                        ? `<a href="pipeline-builder.html?interfaceId=${esc(e.interfaceId)}" style="color:var(--navy);font-size:12px">${esc(e.interfaceName || e.interfaceId)}</a>`
                        : '<span style="color:#9ca3af">deleted</span>'}
                </td>
                <td style="font-size:12px;color:#6b7280">${esc(e.migratedBy)}</td>
            </tr>`).join('');

        container.innerHTML = `
            <div style="background:#fff;border:1px solid #e5e7eb;border-radius:10px;overflow:hidden">
                <table style="width:100%;border-collapse:collapse;font-size:13px">
                    <thead style="background:#f9fafb">
                        <tr>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Date</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Channel</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Mirth Ver</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Steps</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Skipped</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">Interface</th>
                            <th style="padding:10px 14px;text-align:left;font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase">By</th>
                        </tr>
                    </thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>`;
    } catch (err) {
        container.innerHTML = `<p style="color:#dc2626;font-size:13px">Failed to load history: ${esc(err.message)}</p>`;
    }
}

// ============================================================================
// UI helpers
// ============================================================================

function showPanel(id) {
    const el = document.getElementById(id);
    if (el) { el.classList.add('visible'); }
}

function toggleSection(id) {
    const body = document.getElementById(id);
    const chevron = document.getElementById('chevron-' + id);
    const open = !body.style.display || body.style.display !== 'none';
    body.style.display = open ? 'none' : 'block';
    if (chevron) chevron.style.transform = open ? 'rotate(-90deg)' : '';
}

function resetMigration() {
    currentXMLText = null;
    currentPreview = null;

    // Restore upload zone
    const zone = document.getElementById('upload-zone');
    zone.style.display = '';
    zone.innerHTML = `
        <div><i class="fas fa-cloud-upload-alt"></i></div>
        <h3>Drop your Mirth channel export here</h3>
        <p>Supports Mirth Connect 3.x and NextGen Connect .xml channel exports</p>
        <div class="or">— or —</div>
        <button class="btn btn-primary" onclick="event.stopPropagation(); document.getElementById('file-input').click()">
            <i class="fas fa-folder-open"></i> Browse files
        </button>
        <input type="file" id="file-input" accept=".xml" style="display:none" onchange="onFileSelected(event)">`;

    // Re-wire drag-drop on restored zone
    zone.ondragover  = onDragOver;
    zone.ondragleave = onDragLeave;
    zone.ondrop      = onDrop;
    zone.onclick     = () => document.getElementById('file-input').click();

    document.getElementById('preview-panel').classList.remove('visible');
    document.getElementById('btn-reset').style.display = 'none';
    document.getElementById('import-status').textContent = '';
    document.getElementById('import-form').classList.remove('visible');
}

function esc(str) {
    if (str == null) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

function showToast(msg, type) {
    const t = document.getElementById('toast');
    t.className = 'toast ' + type;
    t.innerHTML = `<i class="fas fa-${type === 'success' ? 'check-circle' : 'exclamation-circle'}"></i> ${esc(msg)}`;
    t.style.display = 'flex';
    setTimeout(() => { t.style.display = 'none'; }, 4500);
}
