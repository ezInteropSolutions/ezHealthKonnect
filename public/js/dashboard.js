// Simple dashboard manager - matching your backend API
// Note: sidebar toggle + logout are handled by sidebar-nav.js (universal component)

// Update time
function updateTime() {
    const now = new Date();
    const timeString = now.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    document.getElementById('currentTime').textContent = timeString;
}

// Global variable to store user info
let currentUser = null;

// Navigation handling
document.querySelectorAll('.nav-item, .action-tile').forEach(item => {
    item.addEventListener('click', function(e) {
        // Get the target from href or data attribute
        const target = this.getAttribute('href') || this.getAttribute('data-target');

        // Allow normal navigation for actual HTML pages
        if (target && (target.endsWith('.html') || target.startsWith('http'))) {
            // Don't prevent default - allow normal navigation
            return;
        }

        // Only prevent default for hash-based navigation
        e.preventDefault();

        // Remove active class from all nav items
        document.querySelectorAll('.nav-item').forEach(nav => nav.classList.remove('active'));

        // Add active class to clicked nav item (if it's a nav item)
        if (this.classList.contains('nav-item')) {
            this.classList.add('active');
        }

        // Handle different routes
        handleNavigation(target);
    });
});

// Handle navigation routing
function handleNavigation(target) {
    console.log('Navigating to:', target);
    
    switch(target) {
        case '#dashboard':
            showDashboardContent();
            break;
            
        case '#interfaces':
            showInterfacesContent();
            break;
            
        case '#messages':
            showMessagesContent();
            break;
            
        case '#templates':
            showTemplatesContent();
            break;
            
        case '#monitoring':
            showMonitoringContent();
            break;
            
        case '#reports':
            showReportsContent();
            break;
            
        case '#users':
            showUserManagementContent();
            break;
            
        case '#settings':
            showSettingsContent();
            break;
            
        default:
            console.log('Unknown route:', target);
            showNotImplemented(target);
    }
}

// On page load, honour any hash in the URL (e.g. dashboard.html#templates)
window.addEventListener('DOMContentLoaded', () => {
    const hash = window.location.hash;
    if (hash && hash !== '#dashboard') {
        handleNavigation(hash);
        // Mark the matching nav item as active
        document.querySelectorAll('.nav-item').forEach(nav => {
            nav.classList.toggle('active', nav.getAttribute('href') === hash);
        });
    }
});

// Content display functions
function showDashboardContent() {
    updatePageTitle('Dashboard', 'Overview of your healthcare integration platform');
    // Dashboard is already visible - no change needed
}

function showInterfacesContent() {
    // Redirect to dedicated interfaces page
    window.location.href = 'interfaces.html';
}

function showMessagesContent() {
    updatePageTitle('Messages', 'Monitor and track message processing');
    showPlaceholderContent('Messages', 'View real-time message processing, transformation logs, and troubleshoot any integration issues.');
}

// ── Templates Gallery ──────────────────────────────────────────────────────

const TEMPLATE_CATEGORY_META = {
    all:        { label: 'All',          icon: '📋' },
    emr:        { label: 'EMR / EHR',    icon: '🏥' },
    hl7v2:      { label: 'HL7 v2',       icon: '🔀' },
    fhir:       { label: 'FHIR',         icon: '🌐' },
    payer:      { label: 'Payers',       icon: '💰' },
    regulatory: { label: 'Regulatory',   icon: '⚖️' },
    specialty:  { label: 'Specialty',    icon: '🔬' },
    custom:     { label: 'Mine',         icon: '⭐' },
};

let _templatesState = { all: [], filtered: [], activeCategory: 'all', search: '' };

function showTemplatesContent() {
    updatePageTitle('Templates', 'Pre-built integration templates');

    // Replace entire main-content so there's no leftover grid space
    const mainContent = document.querySelector('.main-content');
    mainContent.innerHTML = `
        <style>
            .tg-wrap { display: flex; flex-direction: column; height: 100%; background: #f7f8fc; }

            /* ── Top bar ── */
            .tg-topbar { display: flex; align-items: center; gap: 14px; padding: 16px 28px 14px; background: #fff; border-bottom: 1px solid #e8ecf0; flex-shrink: 0; }
            .tg-topbar-title { font-size: 1.15rem; font-weight: 700; color: #1a1a2e; margin: 0; }
            .tg-topbar-subtitle { font-size: 0.8rem; color: #888; margin: 0 0 0 4px; }
            .tg-search { padding: 8px 14px 8px 36px; border: 1px solid #dde1e7; border-radius: 8px; font-size: 0.88rem; width: 260px; background: #f9fafb url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='14' height='14' viewBox='0 0 24 24' fill='none' stroke='%23aaa' stroke-width='2'%3E%3Ccircle cx='11' cy='11' r='8'/%3E%3Cpath d='m21 21-4.35-4.35'/%3E%3C/svg%3E") no-repeat 11px center; }
            .tg-search:focus { outline: none; border-color: #667eea; box-shadow: 0 0 0 3px rgba(102,126,234,0.12); background-color: #fff; }
            .tg-cat-tabs { display: flex; gap: 6px; flex-wrap: wrap; margin-left: 4px; }
            .tg-cat-tab { padding: 6px 15px; border: 1.5px solid #e0e4ea; border-radius: 20px; background: #fff; cursor: pointer; font-size: 0.8rem; color: #555; font-weight: 500; transition: all 0.13s; white-space: nowrap; }
            .tg-cat-tab:hover { border-color: #667eea; color: #667eea; background: #f0f2ff; }
            .tg-cat-tab.active { background: #667eea; border-color: #667eea; color: #fff; font-weight: 600; }
            .tg-count-badge { font-size: 0.72rem; opacity: 0.75; margin-left: 3px; }

            /* ── Grid ── */
            .tg-grid-wrap { flex: 1; overflow-y: auto; padding: 22px 28px; }
            .tg-results-line { font-size: 0.78rem; color: #aaa; margin-bottom: 14px; }
            .tg-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 16px; }

            /* ── Cards ── */
            .tg-card { border: 1px solid #e8ecf0; border-radius: 12px; background: #fff; overflow: hidden; cursor: pointer; transition: box-shadow 0.18s, transform 0.14s, border-color 0.14s; display: flex; flex-direction: column; }
            .tg-card:hover { box-shadow: 0 6px 24px rgba(102,126,234,0.13); transform: translateY(-3px); border-color: #c5cff9; }
            .tg-card-top { padding: 16px 18px 12px; flex: 1; }
            .tg-card-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
            .tg-card-title { font-size: 0.95rem; font-weight: 700; color: #1a1a2e; line-height: 1.3; }
            .tg-info-btn { background: none; border: 1px solid #dde1e7; border-radius: 6px; padding: 3px 8px; font-size: 0.72rem; color: #888; cursor: pointer; flex-shrink: 0; transition: all 0.12s; }
            .tg-info-btn:hover { border-color: #667eea; color: #667eea; background: #f0f2ff; }
            .tg-card-desc { font-size: 0.78rem; color: #777; line-height: 1.45; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; margin-bottom: 12px; }

            /* connector flow strip */
            .tg-conn-flow { display: flex; align-items: center; gap: 6px; margin-bottom: 10px; }
            .tg-conn-pill { background: #f0f4ff; color: #4a63d3; border-radius: 6px; padding: 3px 9px; font-size: 0.72rem; font-weight: 500; white-space: nowrap; }
            .tg-conn-arrow { color: #c5cff9; font-size: 0.85rem; flex-shrink: 0; }

            /* pipeline icon chain */
            .tg-steps-chain { display: flex; align-items: center; gap: 3px; flex-wrap: wrap; row-gap: 4px; }
            .tg-step-icon { font-size: 0.78rem; background: #f4f5f7; border-radius: 4px; padding: 2px 6px; color: #555; }
            .tg-step-sep { color: #d0d4dc; font-size: 0.7rem; }

            .tg-card-bottom { padding: 10px 18px 14px; border-top: 1px solid #f0f2f5; display: flex; align-items: center; gap: 8px; }
            .tg-meta-pill { font-size: 0.69rem; border-radius: 5px; padding: 2px 7px; font-weight: 500; }
            .tg-meta-oob { background: #e8f5e9; color: #2e7d32; }
            .tg-meta-msgtype { background: #f0f4ff; color: #4a63d3; }
            .tg-meta-time { background: #f7f8fa; color: #888; margin-left: auto; }
            .tg-use-btn { background: #667eea; color: #fff; border: none; border-radius: 7px; padding: 7px 18px; font-size: 0.82rem; font-weight: 600; cursor: pointer; transition: background 0.14s; flex-shrink: 0; }
            .tg-use-btn:hover { background: #5a72d8; }
            .tg-del-btn { background: none; border: 1px solid #e8ecf0; border-radius: 7px; padding: 7px 10px; font-size: 0.82rem; cursor: pointer; color: #bbb; transition: all 0.13s; flex-shrink: 0; }
            .tg-del-btn:hover { border-color: #e53935; color: #e53935; background: #fff5f5; }

            .tg-tags { display: flex; flex-wrap: wrap; gap: 4px; padding: 0 18px 12px; }
            .tg-tag { background: #f5f6ff; color: #7c8fc9; border-radius: 4px; padding: 1px 6px; font-size: 0.67rem; }

            .tg-empty { grid-column: 1/-1; text-align: center; padding: 80px 20px; color: #aaa; }
            .tg-empty-icon { font-size: 2.8rem; margin-bottom: 14px; }
            .tg-spinner { grid-column: 1/-1; text-align: center; padding: 80px; color: #bbb; font-size: 0.9rem; }

            /* ── Guide button on card ── */
            .tg-guide-btn { background: none; border: 1px solid #c8f0dc; border-radius: 6px; padding: 3px 8px; font-size: 0.72rem; color: #38a169; cursor: pointer; flex-shrink: 0; transition: all 0.12s; }
            .tg-guide-btn:hover { background: #f0fff4; border-color: #38a169; }

            /* ── Preview modal ── */
            .tg-modal-overlay { position: fixed; inset: 0; background: rgba(15,20,40,0.45); z-index: 1000; display: flex; align-items: center; justify-content: center; }
            .tg-modal { background: #fff; border-radius: 14px; width: 540px; max-width: 96vw; max-height: 88vh; overflow-y: auto; box-shadow: 0 24px 64px rgba(0,0,0,0.22); }
            .tg-modal-header { padding: 22px 24px 16px; border-bottom: 1px solid #eee; display: flex; justify-content: space-between; align-items: flex-start; }
            .tg-modal-title { font-size: 1.08rem; font-weight: 700; color: #1a1a2e; margin: 0 0 3px; }
            .tg-modal-cat { font-size: 0.76rem; color: #999; }
            .tg-modal-close { background: none; border: none; font-size: 1.4rem; cursor: pointer; color: #bbb; padding: 0; line-height: 1; }
            .tg-modal-close:hover { color: #555; }
            .tg-modal-body { padding: 20px 24px; }
            .tg-modal-desc { color: #555; font-size: 0.87rem; margin-bottom: 20px; line-height: 1.55; }
            .tg-modal-section { margin-bottom: 18px; }
            .tg-modal-section-title { font-size: 0.73rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.07em; color: #aaa; margin-bottom: 9px; }
            .tg-pipeline-flow { display: flex; flex-direction: column; gap: 5px; }
            .tg-pipeline-step { display: flex; align-items: center; gap: 9px; font-size: 0.84rem; color: #333; padding: 6px 12px; background: #f7f8fa; border-radius: 7px; }
            .tg-required-field { display: flex; align-items: center; gap: 10px; padding: 7px 0; border-bottom: 1px solid #f0f2f5; font-size: 0.84rem; }
            .tg-required-field:last-child { border-bottom: none; }
            .tg-req-badge { border-radius: 5px; padding: 2px 7px; font-size: 0.69rem; font-weight: 600; flex-shrink: 0; }
            .tg-req-badge.required { background: #fff3e0; color: #e65100; }
            .tg-req-badge.optional { background: #f3f4f6; color: #999; }
            .tg-modal-footer { padding: 16px 24px; border-top: 1px solid #eee; display: flex; gap: 10px; justify-content: flex-end; }
            .tg-modal-btn { padding: 9px 22px; border-radius: 8px; font-size: 0.87rem; font-weight: 600; cursor: pointer; border: none; transition: background 0.13s; }
            .tg-modal-btn.cancel { background: #f0f2f5; color: #555; }
            .tg-modal-btn.cancel:hover { background: #e2e6ea; }
            .tg-modal-btn.use { background: #667eea; color: #fff; }
            .tg-modal-btn.use:hover { background: #5a72d8; }

            /* ── Modal tab bar ── */
            .tg-tab-bar { display: flex; border-bottom: 1px solid #eee; padding: 0 24px; background: #fafbfc; }
            .tg-tab-btn { background: none; border: none; border-bottom: 2px solid transparent; padding: 10px 16px; font-size: 0.83rem; font-weight: 500; color: #888; cursor: pointer; transition: color 0.13s, border-color 0.13s; margin-bottom: -1px; }
            .tg-tab-btn:hover { color: #667eea; }
            .tg-tab-btn.active { color: #667eea; border-bottom-color: #667eea; font-weight: 600; }

            /* ── User guide panel ── */
            .tg-guide-body { padding: 20px 24px; font-size: 0.86rem; line-height: 1.68; color: #333; }
            .tg-guide-body h2 { font-size: 1rem; font-weight: 700; color: #1a1a2e; margin: 0 0 10px; }
            .tg-guide-body h3 { font-size: 0.9rem; font-weight: 700; color: #2d3748; margin: 20px 0 8px; border-top: 1px solid #f0f2f5; padding-top: 16px; }
            .tg-guide-body h4 { font-size: 0.83rem; font-weight: 600; color: #4a5568; margin: 14px 0 5px; }
            .tg-guide-body p { margin: 0 0 10px; }
            .tg-guide-body hr.tg-hr { border: none; border-top: 1px solid #eee; margin: 18px 0; }
            .tg-guide-body ul.tg-guide-ul { margin: 0 0 10px 20px; padding: 0; }
            .tg-guide-body ul.tg-guide-ul li { margin-bottom: 4px; }
            .tg-guide-body pre.tg-code { background: #f4f5f7; border-radius: 6px; padding: 12px 14px; font-size: 0.78rem; overflow-x: auto; margin: 0 0 12px; line-height: 1.55; }
            .tg-guide-body code.tg-icode { background: #f0f1f3; border-radius: 3px; padding: 1px 5px; font-size: 0.82em; color: #c0392b; font-family: monospace; }
            .tg-table-wrap { overflow-x: auto; margin-bottom: 14px; }
            .tg-guide-table { border-collapse: collapse; width: 100%; font-size: 0.81rem; }
            .tg-guide-table th { background: #f7f8fc; font-weight: 600; color: #555; padding: 6px 10px; border: 1px solid #e8ecf0; text-align: left; white-space: nowrap; }
            .tg-guide-table td { padding: 5px 10px; border: 1px solid #e8ecf0; vertical-align: top; }
            .tg-guide-table td:first-child { font-family: monospace; font-size: 0.78rem; color: #c0392b; white-space: nowrap; }
        </style>

        <div class="tg-wrap">
            <div class="tg-topbar">
                <div>
                    <p class="tg-topbar-title">Integration Templates</p>
                    <p class="tg-topbar-subtitle">Click a template to create a new interface — credentials only, everything else is pre-configured</p>
                </div>
                <input class="tg-search" id="tgSearch" placeholder="Search templates…" oninput="filterTemplates()" style="margin-left:auto;">
                <div class="tg-cat-tabs" id="tgCatTabs"></div>
            </div>
            <div class="tg-grid-wrap">
                <div class="tg-grid" id="tgGrid">
                    <div class="tg-spinner">Loading templates…</div>
                </div>
            </div>
        </div>
    `;

    _loadTemplates();
}

async function _loadTemplates() {
    try {
        const res = await fetch('/api/interface-templates?include_categories=true', { credentials: 'include' });
        const data = await res.json();

        if (!data.success) throw new Error(data.error || 'Failed to load templates');

        _templatesState.all = data.templates || [];
        _templatesState.filtered = _templatesState.all;

        _renderCategoryTabs(data.categories || []);
        _renderTemplateGrid(_templatesState.all);
    } catch (err) {
        const grid = document.getElementById('tgGrid');
        if (grid) grid.innerHTML = `
            <div class="tg-empty">
                <div class="tg-empty-icon">⚠️</div>
                <p>Could not load templates: ${err.message}</p>
                <button class="tg-btn use" style="width:auto;padding:8px 20px;" onclick="_loadTemplates()">Retry</button>
            </div>`;
    }
}

function _renderCategoryTabs(categories) {
    const tabs = document.getElementById('tgCatTabs');
    if (!tabs) return;

    const totals = { all: _templatesState.all.length };
    for (const c of categories) totals[c.category] = c.count;

    const order = ['all', 'emr', 'hl7v2', 'fhir', 'payer', 'regulatory', 'specialty', 'custom'];
    const present = new Set(['all', ...(categories.map(c => c.category))]);

    tabs.innerHTML = order
        .filter(k => present.has(k))
        .map(k => {
            const meta = TEMPLATE_CATEGORY_META[k] || { label: k, icon: '📋' };
            const count = totals[k] || 0;
            const active = k === _templatesState.activeCategory ? ' active' : '';
            return `<button class="tg-cat-tab${active}" onclick="setTemplateCategory('${k}')">${meta.icon} ${meta.label}${count ? ` <span style="opacity:0.65">(${count})</span>` : ''}</button>`;
        }).join('');
}

function setTemplateCategory(cat) {
    _templatesState.activeCategory = cat;
    document.querySelectorAll('.tg-cat-tab').forEach(t => {
        t.classList.toggle('active', t.textContent.toLowerCase().startsWith((TEMPLATE_CATEGORY_META[cat]?.icon || cat)));
    });
    // simpler: re-render tabs
    const allTabs = document.querySelectorAll('.tg-cat-tab');
    allTabs.forEach(t => {
        t.classList.remove('active');
        const onclick = t.getAttribute('onclick') || '';
        if (onclick.includes(`'${cat}'`)) t.classList.add('active');
    });
    filterTemplates();
}

function filterTemplates() {
    const search = (document.getElementById('tgSearch')?.value || '').toLowerCase().trim();
    const cat = _templatesState.activeCategory;

    _templatesState.filtered = _templatesState.all.filter(t => {
        if (cat !== 'all' && t.category !== cat) return false;
        if (search) {
            const haystack = `${t.name} ${t.description || ''} ${(t.tags || []).join(' ')}`.toLowerCase();
            if (!haystack.includes(search)) return false;
        }
        return true;
    });

    _renderTemplateGrid(_templatesState.filtered);
}

function _renderTemplateGrid(templates) {
    const grid = document.getElementById('tgGrid');
    if (!grid) return;

    if (!templates.length) {
        grid.innerHTML = `<div class="tg-empty"><div class="tg-empty-icon">🔍</div><p>No templates found. Try a different category or search term.</p></div>`;
        return;
    }

    grid.innerHTML = templates.map(t => _renderTemplateCard(t)).join('');
}

function _renderTemplateCard(t) {
    const steps = (t.preview_steps || []);
    const stepsChain = steps.length
        ? steps.map((s, i) => `${i > 0 ? '<span class="tg-step-sep">›</span>' : ''}<span class="tg-step-icon">${s.icon || '⚙️'} ${_escHtml(s.name)}</span>`).join('')
        : '';

    const connFlow = [
        t.source_connector_type ? `<span class="tg-conn-pill">📥 ${_escHtml(_formatConnType(t.source_connector_type))}</span>` : '',
        t.target_connector_type ? `<span class="tg-conn-arrow">→</span><span class="tg-conn-pill">📤 ${_escHtml(_formatConnType(t.target_connector_type))}</span>` : '',
    ].filter(Boolean).join('');

    const tags = (t.tags || []).slice(0, 4).map(tag => `<span class="tg-tag">${_escHtml(tag)}</span>`).join('');

    return `
        <div class="tg-card" onclick="useTemplate('${t.id}')" title="Click to use this template">
            <div class="tg-card-top">
                <div class="tg-card-title-row">
                    <span class="tg-card-title">${t.icon || '📋'} ${_escHtml(t.name)}</span>
                    <button class="tg-info-btn" onclick="event.stopPropagation();previewTemplate('${t.id}','details')">ℹ Details</button>
                    ${t.has_guide ? `<button class="tg-guide-btn" onclick="event.stopPropagation();previewTemplate('${t.id}','guide')">📖 Guide</button>` : ''}
                </div>
                <p class="tg-card-desc">${_escHtml(t.description || '')}</p>
                ${connFlow ? `<div class="tg-conn-flow">${connFlow}</div>` : ''}
                ${stepsChain ? `<div class="tg-steps-chain">${stepsChain}</div>` : ''}
            </div>
            ${tags ? `<div class="tg-tags">${tags}</div>` : ''}
            <div class="tg-card-bottom">
                ${t.is_system ? '<span class="tg-meta-pill tg-meta-oob">⭐ OOB</span>' : ''}
                ${t.message_type ? `<span class="tg-meta-pill tg-meta-msgtype">${_escHtml(t.message_type)}</span>` : ''}
                ${t.estimated_setup_minutes ? `<span class="tg-meta-pill tg-meta-time">~${t.estimated_setup_minutes}m</span>` : ''}
                ${!t.is_system ? `<button class="tg-del-btn" onclick="event.stopPropagation();deleteTemplate('${t.id}', '${_escHtml(t.name)}')" title="Delete template">🗑</button>` : ''}
                <button class="tg-use-btn" onclick="event.stopPropagation();useTemplate('${t.id}')">▶ Use</button>
            </div>
        </div>`;
}

// Sensitive field keys that must be blanked when instantiating from a template.
// Must stay in sync with services/interfaceTemplateSanitizer.js SENSITIVE_PATTERNS.
const _SENSITIVE_KEYS = new Set([
    'password','secret','token','api_key','access_key','client_secret','private_key',
    'cert_content','certificate_content','bearer',
    'host','hostname','ip_address','server','endpoint','url','base_url','fhir_base_url',
    'server_url','connection_string','dsn','port',
    'username','client_id','tenant',
    'database','db_name','table_name','collection','schema_name','index_name',
    'account','warehouse','workspace_id','cluster','project_id',
    'bucket','container_name','region','queue_name','topic','namespace',
    'directory','folder','file_path',
]);
const _SAFE_KEYS = new Set([
    'method','content_type','auth_type','auth_method','mode','enable_tls','tls',
    'ssl_mode','encoding','max_connections','polling_interval','batch_size',
    'timeout','retry','version','scope','sending_app','sending_facility',
    'ack','on_error','text_success','text_error',
]);

function _sensitiveKey(key) {
    const k = key.toLowerCase();
    for (const s of _SAFE_KEYS) { if (k === s || k.startsWith(s + '_') || k.endsWith('_' + s)) return false; }
    for (const p of _SENSITIVE_KEYS) { if (k.includes(p)) return true; }
    return false;
}

function _sanitizeStepConfig(cfg) {
    if (!cfg || typeof cfg !== 'object' || Array.isArray(cfg)) return cfg;
    const out = {};
    for (const [k, v] of Object.entries(cfg)) {
        if (_sensitiveKey(k)) {
            out[k] = '';
        } else if (v && typeof v === 'object' && !Array.isArray(v)) {
            out[k] = _sanitizeStepConfig(v);
        } else {
            out[k] = v;
        }
    }
    return out;
}

function _escHtml(str) {
    return String(str || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function _formatConnType(type) {
    return type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()).replace(' Inbound','').replace(' Outbound','');
}

// ── Inject user-provided connector values into sanitized pipeline steps ────────
function _injectConnectorValues(groups, srcVals, tgtVals) {
    const hasSrc = Object.keys(srcVals).length > 0;
    const hasTgt = Object.keys(tgtVals).length > 0;
    if (!hasSrc && !hasTgt) return groups;
    return groups.map(g => ({
        ...g,
        steps: (g.steps || []).map(step => {
            const sType = step.step_type || step.type || '';
            if (sType === 'connector.inbound' && hasSrc) {
                return { ...step, config: { ...step.config, config: { ...(step.config?.config || {}), ...srcVals } } };
            }
            if (sType === 'connector.outbound' && hasTgt) {
                return { ...step, config: { ...step.config, config: { ...(step.config?.config || {}), ...tgtVals } } };
            }
            return step;
        }),
    }));
}

// ── Tab switcher for preview modal ────────────────────────────────────────────
function _tgSwitchTab(modalId, tabId) {
    const el = document.getElementById(modalId);
    if (!el) return;
    el.querySelectorAll('.tg-tab-btn').forEach(b => b.classList.toggle('active', b.dataset.tab === tabId));
    el.querySelectorAll('.tg-tab-panel').forEach(p => { p.style.display = p.dataset.panel === tabId ? '' : 'none'; });
}

// ── Minimal markdown → safe HTML renderer (for user_guide content) ─────────────
function _mdToHtml(md) {
    if (!md) return '';

    // 1. Extract fenced code blocks
    const codeBlocks = [];
    let text = md.replace(/```[\s\S]*?```/g, m => {
        const body = m.slice(3, -3).replace(/^\S*\n/, ''); // strip optional language tag
        codeBlocks.push(_escHtml(body));
        return '\x02CB' + (codeBlocks.length - 1) + '\x02';
    });

    // 2. Extract inline code
    const inlineCodes = [];
    text = text.replace(/`([^`\n]+)`/g, (_, c) => {
        inlineCodes.push(_escHtml(c));
        return '\x02IC' + (inlineCodes.length - 1) + '\x02';
    });

    // Restore inline code + bold in a string already passed through _escHtml
    function inl(t) {
        return t
            .replace(/\x02IC(\d+)\x02/g, (_, n) => `<code class="tg-icode">${inlineCodes[+n]}</code>`)
            .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    }

    const lines = text.split('\n');
    const out = [];
    let tableOpen = false, tableHeaderDone = false, listOpen = false;

    const closeList  = () => { if (listOpen)  { out.push('</ul>');              listOpen = false; } };
    const closeTable = () => { if (tableOpen) { out.push('</tbody></table></div>'); tableOpen = false; tableHeaderDone = false; } };

    for (let i = 0; i < lines.length; i++) {
        const raw = lines[i];

        // Fenced code block placeholder
        const cbm = raw.trim().match(/^\x02CB(\d+)\x02$/);
        if (cbm) {
            closeList(); closeTable();
            out.push(`<pre class="tg-code"><code>${codeBlocks[+cbm[1]]}</code></pre>`);
            continue;
        }

        // Headings
        if (/^### /.test(raw)) { closeList(); closeTable(); out.push(`<h4>${inl(_escHtml(raw.slice(4)))}</h4>`); continue; }
        if (/^## /.test(raw))  { closeList(); closeTable(); out.push(`<h3>${inl(_escHtml(raw.slice(3)))}</h3>`); continue; }
        if (/^# /.test(raw))   { closeList(); closeTable(); out.push(`<h2>${inl(_escHtml(raw.slice(2)))}</h2>`); continue; }

        // Horizontal rule
        if (/^---+$/.test(raw.trim())) { closeList(); closeTable(); out.push('<hr class="tg-hr">'); continue; }

        // Table row
        if (raw.trim().startsWith('|')) {
            closeList();
            // Separator row (|---|---|)
            if (/^\|[\s\-:|]+\|$/.test(raw.trim())) {
                if (tableOpen && !tableHeaderDone) { out.push('</thead><tbody>'); tableHeaderDone = true; }
                continue;
            }
            const cells = raw.trim().replace(/^\||\|$/g, '').split('|').map(c => c.trim());
            if (!tableOpen) {
                out.push('<div class="tg-table-wrap"><table class="tg-guide-table"><thead>');
                tableOpen = true;
            }
            const tag = tableHeaderDone ? 'td' : 'th';
            out.push('<tr>' + cells.map(c => `<${tag}>${inl(_escHtml(c))}</${tag}>`).join('') + '</tr>');
            continue;
        }

        // List item
        const listMatch = raw.match(/^[-*] (.*)/);
        if (listMatch) {
            closeTable();
            if (!listOpen) { out.push('<ul class="tg-guide-ul">'); listOpen = true; }
            out.push(`<li>${inl(_escHtml(listMatch[1]))}</li>`);
            continue;
        }

        // Empty line
        if (!raw.trim()) { closeList(); closeTable(); continue; }

        // Paragraph
        closeList(); closeTable();
        out.push(`<p>${inl(_escHtml(raw))}</p>`);
    }
    closeList(); closeTable();
    return out.join('\n');
}

async function previewTemplate(id, activeTab) {
    activeTab = activeTab || 'details';
    try {
        const res = await fetch(`/api/interface-templates/${id}`, { credentials: 'include' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error);
        const t = data.template;

        const required = (t.required_connection_fields || []);
        const steps = (t.preview_steps || []);

        const stepsHtml = steps.length
            ? steps.map(s => `<div class="tg-pipeline-step"><span>${s.icon || '⚙️'}</span><span>${_escHtml(s.name)}</span></div>`).join('')
            : '<p style="color:#aaa;font-size:0.82rem">No pipeline steps recorded.</p>';

        const connHtml = [
            t.source_connector_type ? `<div class="tg-pipeline-step"><span>📥</span><span>Source: ${_escHtml(_formatConnType(t.source_connector_type))}</span></div>` : '',
            t.target_connector_type ? `<div class="tg-pipeline-step"><span>📤</span><span>Destination: ${_escHtml(_formatConnType(t.target_connector_type))}</span></div>` : '',
        ].filter(Boolean).join('');

        const reqHtml = required.length
            ? required.map(f => `
                <div class="tg-required-field">
                    <span class="tg-req-badge ${f.required ? 'required' : 'optional'}">${f.required ? 'Required' : 'Optional'}</span>
                    <span>${_escHtml(f.label)}</span>
                    <span style="color:#aaa;font-size:0.78rem;margin-left:auto">${f.section}</span>
                </div>`).join('')
            : '<p style="color:#888;font-size:0.82rem">This template requires no additional connection setup — all defaults are pre-filled.</p>';

        const hasGuide = !!(t.user_guide && t.user_guide.trim());
        const tabBar = hasGuide ? `
            <div class="tg-tab-bar">
                <button class="tg-tab-btn${activeTab === 'details' ? ' active' : ''}" data-tab="details"
                    onclick="_tgSwitchTab('tgPreviewModal','details')">Details</button>
                <button class="tg-tab-btn${activeTab === 'guide' ? ' active' : ''}" data-tab="guide"
                    onclick="_tgSwitchTab('tgPreviewModal','guide')">📖 User Guide</button>
            </div>` : '';

        const detailsHidden = hasGuide && activeTab !== 'details' ? ' style="display:none"' : '';
        const guideHidden   = !hasGuide || activeTab !== 'guide'  ? ' style="display:none"' : '';

        const modal = document.createElement('div');
        modal.className = 'tg-modal-overlay';
        modal.id = 'tgPreviewModal';
        modal.innerHTML = `
            <div class="tg-modal">
                <div class="tg-modal-header">
                    <div>
                        <div class="tg-modal-title">${t.icon || '📋'} ${_escHtml(t.name)}</div>
                        <div class="tg-modal-cat">${(TEMPLATE_CATEGORY_META[t.category] || {}).icon || ''} ${_escHtml(t.category)} ${t.subcategory ? '· ' + t.subcategory : ''}</div>
                    </div>
                    <button class="tg-modal-close" onclick="document.getElementById('tgPreviewModal').remove()">×</button>
                </div>
                ${tabBar}
                <div class="tg-tab-panel tg-modal-body" data-panel="details"${detailsHidden}>
                    <p class="tg-modal-desc">${_escHtml(t.description || 'No description available.')}</p>
                    <div class="tg-modal-section">
                        <div class="tg-modal-section-title">Connectors</div>
                        <div class="tg-pipeline-flow">${connHtml || '<p style="color:#aaa">No connectors defined.</p>'}</div>
                    </div>
                    <div class="tg-modal-section">
                        <div class="tg-modal-section-title">Pipeline Flow</div>
                        <div class="tg-pipeline-flow">${stepsHtml}</div>
                    </div>
                    <div class="tg-modal-section">
                        <div class="tg-modal-section-title">You'll Need to Provide</div>
                        ${reqHtml}
                    </div>
                </div>
                ${hasGuide ? `<div class="tg-tab-panel tg-guide-body" data-panel="guide"${guideHidden}>${_mdToHtml(t.user_guide)}</div>` : ''}
                <div class="tg-modal-footer">
                    <button class="tg-modal-btn cancel" onclick="document.getElementById('tgPreviewModal').remove()">Close</button>
                    <button class="tg-modal-btn use" onclick="document.getElementById('tgPreviewModal').remove(); useTemplate('${t.id}')">▶ Configure & Create</button>
                </div>
            </div>`;

        document.body.appendChild(modal);
        modal.addEventListener('click', e => { if (e.target === modal) modal.remove(); });
    } catch (err) {
        AppDialogs.toast('Could not load template preview: ' + err.message, 'error');
    }
}

async function deleteTemplate(id, name) {
    const ok = await AppDialogs.confirm(`Delete template "<b>${name}</b>"?<br><br>This cannot be undone.`, { title: 'Delete Template', type: 'danger', confirmText: 'Delete' });
    if (!ok) return;
    try {
        const res = await fetch(`/api/interface-templates/${id}`, {
            method: 'DELETE',
            credentials: 'include',
        });
        const data = await res.json();
        if (!data.success) throw new Error(data.error || 'Delete failed');
        // Remove from local state and re-render
        _templatesState.all = _templatesState.all.filter(t => t.id !== id);
        filterTemplates();
    } catch (err) {
        AppDialogs.toast('Could not delete template: ' + err.message, 'error');
    }
}

async function useTemplate(id) {
    document.getElementById('tgConfigureModal')?.remove();

    // Only need the template — no connector config fields shown here
    let t;
    try {
        const res = await fetch(`/api/interface-templates/${id}`, { credentials: 'include' });
        const data = await res.json();
        if (!data.success) throw new Error(data.error);
        t = data.template;
    } catch (err) {
        AppDialogs.toast('Could not load template: ' + err.message, 'error');
        return;
    }

    // Pipeline step preview
    const steps = t.preview_steps || [];
    const stepsHtml = steps.length
        ? steps.map((s, i) =>
            `${i > 0 ? '<span style="color:#c5cff9;margin:0 4px">›</span>' : ''}<span style="background:#f0f4ff;color:#4a63d3;border-radius:5px;padding:3px 8px;font-size:0.75rem;white-space:nowrap">${s.icon || '⚙️'} ${_escHtml(s.name)}</span>`
          ).join('')
        : '<span style="color:#aaa;font-size:0.8rem">No steps recorded</span>';

    // Connector type badges — informational
    const connFlow = [
        t.source_connector_type ? `<span style="background:#e8f0fe;color:#3b5bdb;border-radius:5px;padding:3px 9px;font-size:0.75rem">📥 ${_escHtml(_formatConnType(t.source_connector_type))}</span>` : '',
        t.target_connector_type ? `<span style="color:#aaa;font-size:0.8rem">→</span><span style="background:#e8f0fe;color:#3b5bdb;border-radius:5px;padding:3px 9px;font-size:0.75rem">📤 ${_escHtml(_formatConnType(t.target_connector_type))}</span>` : '',
    ].filter(Boolean).join(' ');

    const overlay = document.createElement('div');
    overlay.id = 'tgConfigureModal';
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(15,20,40,0.45);z-index:1100;display:flex;align-items:center;justify-content:center';
    overlay.innerHTML = `
        <div style="background:#fff;border-radius:14px;width:480px;max-width:96vw;max-height:92vh;overflow-y:auto;box-shadow:0 24px 64px rgba(0,0,0,0.22)">
            <div style="padding:20px 24px 14px;border-bottom:1px solid #eee;display:flex;justify-content:space-between;align-items:flex-start">
                <div>
                    <div style="font-size:1.05rem;font-weight:700;color:#1a1a2e;margin-bottom:2px">${t.icon || '📋'} ${_escHtml(t.name)}</div>
                    <div style="font-size:0.75rem;color:#999">${t.message_type ? _escHtml(t.message_type) + ' · ' : ''}pipeline pre-configured · add connectivity after</div>
                </div>
                <button onclick="document.getElementById('tgConfigureModal').remove()" style="background:none;border:none;font-size:1.4rem;cursor:pointer;color:#bbb;line-height:1;padding:0;margin-left:12px">×</button>
            </div>
            <div style="padding:20px 24px">

                <div style="margin-bottom:20px">
                    <label style="display:block;font-size:0.78rem;font-weight:600;color:#444;margin-bottom:5px">Interface Name <span style="color:#e65100">*</span></label>
                    <input type="text" id="tcf_name" value="${_escHtml(t.name)}" required autocomplete="off"
                        style="width:100%;box-sizing:border-box;padding:9px 12px;border:1px solid #dde1e7;border-radius:7px;font-size:0.9rem;color:#1a1a2e;background:#fafbfc">
                </div>

                <div style="margin-bottom:18px;padding:14px;background:#f7f8ff;border-radius:8px;border:1px solid #e8ecf0">
                    <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;letter-spacing:.07em;color:#aaa;margin-bottom:8px">Pipeline Steps (pre-configured)</div>
                    <div style="display:flex;flex-wrap:wrap;gap:5px;align-items:center">${stepsHtml}</div>
                </div>

                ${connFlow ? `
                <div style="padding:10px 14px;background:#f0f4ff;border-radius:8px;border:1px solid #c5cff9;margin-bottom:12px;font-size:0.79rem;color:#3b4a9e;line-height:1.55">
                    <strong>📥 ${_escHtml(_formatConnType(t.source_connector_type || ''))}
                    ${t.target_connector_type ? '→ 📤 ' + _escHtml(_formatConnType(t.target_connector_type)) : ''}</strong><br>
                    After creation you'll be guided to configure these connectors using the full connector editor in Pipeline Builder.
                </div>` : ''}

                <div id="tcf_error" style="display:none;color:#c0392b;font-size:0.82rem;padding:8px 12px;background:#fef2f2;border-radius:6px;margin-bottom:4px"></div>
            </div>
            <div style="padding:14px 24px 18px;border-top:1px solid #eee;display:flex;gap:10px;justify-content:flex-end">
                <button onclick="document.getElementById('tgConfigureModal').remove()"
                    style="padding:9px 20px;border-radius:8px;font-size:0.87rem;font-weight:600;cursor:pointer;border:none;background:#f0f2f5;color:#555">Cancel</button>
                <button id="tcf_submit" onclick="_submitConfigureTemplate('${t.id}')"
                    style="padding:9px 22px;border-radius:8px;font-size:0.87rem;font-weight:600;cursor:pointer;border:none;background:#667eea;color:#fff">
                    ▶ Create Interface
                </button>
            </div>
        </div>`;

    document.body.appendChild(overlay);
    overlay.addEventListener('click', e => { if (e.target === overlay) overlay.remove(); });
    document.getElementById('tcf_name')?.focus();
    document.getElementById('tcf_name')?.select();
}

async function _submitConfigureTemplate(templateId) {
    const btn = document.getElementById('tcf_submit');
    const errEl = document.getElementById('tcf_error');
    errEl.style.display = 'none';

    const name = document.getElementById('tcf_name')?.value?.trim();
    if (!name) {
        errEl.textContent = 'Interface name is required.';
        errEl.style.display = 'block';
        return;
    }

    btn.textContent = 'Creating…';
    btn.disabled = true;

    try {
        // 1. Get the template scaffold (pipeline_config + message_type)
        const scaffoldRes = await fetch(`/api/interface-templates/${templateId}/use`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ interface_name: name }),
        });
        const scaffoldData = await scaffoldRes.json();
        if (!scaffoldData.success) throw new Error(scaffoldData.error || 'Failed to load template');
        const s = scaffoldData.interface_scaffold;

        // 2. Create the interface — no connector type locked in; user configures connectivity in the editor
        const ifaceRes = await fetch('/api/interfaces', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
                name: s.name,
                messageType: s.message_type || 'HL7v2',
                description: `Created from template: ${s.template_name}`,
                sourceType: '',
                targetType: '',
                sourceConfig: {},
                targetConfig: {},
            }),
        });
        const ifaceData = await ifaceRes.json();
        if (!ifaceData.success) throw new Error(ifaceData.error || ifaceData.debug || 'Failed to create interface');
        const interfaceId = ifaceData.interface?.id || ifaceData.id;

        // 3. Save the transformation pipeline steps (connector-agnostic — no connector.inbound/outbound)
        //    User will add their own source/destination connectors via the pipeline builder
        const pipelineCfg = s.pipeline_config || {};

        // Diagnose: log embedded_mappings presence in template pipeline
        const _allSteps = (pipelineCfg.execution_groups || []).flatMap(g => g.steps || []);
        console.log(`[UseTemplate] Template has ${_allSteps.length} steps`);
        _allSteps.forEach(st => {
            const hasEmb = !!(st.config && st.config.embedded_mappings);
            const embCount = hasEmb && st.config.embedded_mappings.atomicMappings
                ? st.config.embedded_mappings.atomicMappings.length : 'n/a';
            console.log(`[UseTemplate] Step "${st.step_name}" type="${st.step_type}" has_embedded_mappings=${hasEmb} atomic_count=${embCount}`);
        });
        // Strip step IDs so the controller generates fresh UUIDs — avoids duplicate key
        // errors when the template was saved from an existing interface's pipeline.
        // Also strip sensitive fields from step configs as a defence-in-depth measure
        // (template may have been saved before sanitization was in place).
        const CONNECTOR_TYPES = new Set(['connector.inbound', 'connector.outbound']);
        const groups = (pipelineCfg.execution_groups || []).map(g => ({
            ...g,
            steps: (g.steps || []).map(({ id, ...step }) => ({
                ...step,
                // Only sanitize connector steps; preserve all other step configs intact
                config: CONNECTOR_TYPES.has(step.step_type || step.type || '')
                    ? _sanitizeStepConfig(step.config)
                    : step.config,
            })),
        }));
        if (groups.length > 0 && interfaceId) {
            const pipelineRes = await fetch('/api/pipelines', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include',
                body: JSON.stringify({
                    interface_id: interfaceId,
                    message_type: s.message_type || 'ADT^A01',
                    name: `${name} Pipeline`,
                    execution_groups: groups,
                    pipeline_config: pipelineCfg,
                }),
            });
            const pipelineData = await pipelineRes.json();
            if (!pipelineData.success) {
                throw new Error(`Pipeline save failed: ${pipelineData.error || 'unknown error'}`);
            }
            console.log(`✅ Template pipeline saved: ${pipelineData.steps_saved} steps`);
        }

        document.getElementById('tgConfigureModal')?.remove();
        window.location.href = `pipeline-builder.html?interfaceId=${encodeURIComponent(interfaceId)}`;
    } catch (err) {
        errEl.textContent = err.message;
        errEl.style.display = 'block';
        btn.textContent = '▶ Create Interface';
        btn.disabled = false;
    }
}

function showMonitoringContent() {
    updatePageTitle('Monitoring', 'System performance and health monitoring');
    showPlaceholderContent('Monitoring', 'Real-time dashboard showing system performance, message throughput, error rates, and health metrics.');
}

function showReportsContent() {
    updatePageTitle('Reports', 'Analytics and reporting dashboard');
    showPlaceholderContent('Reports', 'Generate detailed reports on message volumes, transformation success rates, and system analytics.');
}

function showUserManagementContent() {
    // Check if user is admin
    if (!currentUser || currentUser.role !== 'admin') {
        showAccessDenied('User Management', 'This section requires administrator privileges.');
        return;
    }
    
    // Redirect to dedicated user management page
    window.location.href = 'user-management.html';
}

function showSettingsContent() {
    // Check if user is admin
    if (!currentUser || currentUser.role !== 'admin') {
        showAccessDenied('System Settings', 'This section requires administrator privileges.');
        return;
    }
    
    updatePageTitle('System Settings', 'Configure platform settings');
    showPlaceholderContent('System Settings', 'Configure system-wide settings, security options, and integration preferences.');
}

// New function for access denied
function showAccessDenied(sectionName, message) {
    updatePageTitle('Access Denied', 'Insufficient permissions');
    
    // Hide dashboard grid and show access denied
    const dashboardGrid = document.querySelector('.dashboard-grid');
    
    // Remove any existing placeholder
    const existingPlaceholder = document.querySelector('.content-placeholder');
    if (existingPlaceholder) {
        existingPlaceholder.remove();
    }
    
    // Create access denied content
    const placeholder = document.createElement('div');
    placeholder.className = 'content-placeholder';
    placeholder.innerHTML = `
        <div class="placeholder-card access-denied">
            <div class="placeholder-icon">🔒</div>
            <h2 class="placeholder-title">Access Denied</h2>
            <p class="placeholder-description">You don't have permission to access <strong>${sectionName}</strong>. ${message}</p>
            <div class="placeholder-actions">
                <button class="placeholder-btn primary" onclick="showDashboard()">
                    <span class="nav-icon">🏠</span>
                    Back to Dashboard
                </button>
                <span class="admin-badge error">Admin Required</span>
            </div>
        </div>
    `;
    
    // Hide dashboard grid and show placeholder
    dashboardGrid.style.display = 'none';
    document.querySelector('.main-content').appendChild(placeholder);
}

function showNotImplemented(route) {
    updatePageTitle('Coming Soon', 'This feature is under development');
    showPlaceholderContent('Feature Under Development', `The ${route} section is currently being built and will be available soon.`);
}

// Helper function to update page title
function updatePageTitle(title, subtitle) {
    document.querySelector('.page-title').textContent = title;
    // You could add a subtitle element if needed
}

// Helper function to show placeholder content
function showPlaceholderContent(title, description) {
    // Hide dashboard grid and show placeholder
    const dashboardGrid = document.querySelector('.dashboard-grid');
    
    // Remove any existing placeholder
    const existingPlaceholder = document.querySelector('.content-placeholder');
    if (existingPlaceholder) {
        existingPlaceholder.remove();
    }
    
    // Create placeholder content
    const placeholder = document.createElement('div');
    placeholder.className = 'content-placeholder';
    placeholder.innerHTML = `
        <div class="placeholder-card">
            <div class="placeholder-icon">${getIconForSection(title)}</div>
            <h2 class="placeholder-title">${title}</h2>
            <p class="placeholder-description">${description}</p>
            <div class="placeholder-actions">
                <button class="placeholder-btn primary" onclick="showDashboard()">
                    <span class="nav-icon">🏠</span>
                    Back to Dashboard
                </button>
            </div>
        </div>
    `;
    
    // Hide dashboard grid and show placeholder
    dashboardGrid.style.display = 'none';
    document.querySelector('.main-content').appendChild(placeholder);
}

// Helper function to get appropriate icons
function getIconForSection(title) {
    const icons = {
        'Interfaces': '🔗',
        'Messages': '📧',
        'Templates': '📄',
        'Monitoring': '📊',
        'Reports': '📈',
        'User Management': '👥',
        'System Settings': '⚙️'
    };
    return icons[title] || '📋';
}

// Function to show dashboard (hide placeholder)
function showDashboard() {
    const placeholder = document.querySelector('.content-placeholder');
    if (placeholder) {
        placeholder.remove();
    }
    document.querySelector('.dashboard-grid').style.display = 'grid';
    
    // Update active nav
    document.querySelectorAll('.nav-item').forEach(nav => nav.classList.remove('active'));
    const dashNav = document.querySelector('.nav-item[href="dashboard.html"]');
    if (dashNav) dashNav.classList.add('active');
    
    updatePageTitle('Welcome back!', 'Dashboard overview');
}

// Fetch interface count and update tile
async function loadInterfaceCount() {
    try {
        const response = await fetch('/api/interfaces', { credentials: 'include' });
        if (response.ok) {
            const data = await response.json();
            const count = data.total ?? (Array.isArray(data.interfaces) ? data.interfaces.length : 0);
            const el = document.getElementById('interfaceCount');
            if (el) el.textContent = count;

            // Also update the header "Active" stat with running interfaces
            const active = Array.isArray(data.interfaces)
                ? data.interfaces.filter(i => i.status === 'active' || i.status === 'running').length
                : 0;
            const activeEl = document.querySelector('.header-stats .stat-item:first-child .stat-value');
            if (activeEl) activeEl.textContent = active;
        }
    } catch (e) {
        console.warn('Could not load interface count:', e);
    }
}

// Fetch template count and update tile
async function loadTemplateCount() {
    try {
        const response = await fetch('/api/interface-templates', { credentials: 'include' });
        if (response.ok) {
            const data = await response.json();
            const count = data.total ?? (Array.isArray(data.templates) ? data.templates.length : 0);
            const el = document.getElementById('templateCount');
            if (el) el.textContent = count;
        }
    } catch (e) {
        console.warn('Could not load template count:', e);
    }
}

// Fetch recent activity from audit_logs
async function loadRecentActivity() {
    const container = document.getElementById('activityList');
    if (!container) return;
    try {
        const response = await fetch('/api/dashboard/recent-activity', { credentials: 'include' });
        if (!response.ok) throw new Error('Non-OK response');
        const data = await response.json();
        if (!data.success || !data.activity || data.activity.length === 0) {
            container.innerHTML = `<div class="activity-item">
                <span class="activity-dot"></span>
                <div class="activity-content"><span class="activity-text" style="color:#aaa">No recent activity</span></div>
            </div>`;
            return;
        }

        const ACTION_LABELS = {
            login: 'User logged in',
            logout: 'User logged out',
            create_interface: 'Interface created',
            update_interface: 'Interface updated',
            delete_interface: 'Interface deleted',
            activate_interface: 'Interface activated',
            deactivate_interface: 'Interface deactivated',
            save_pipeline: 'Pipeline saved',
            create_template: 'Template created',
            delete_template: 'Template deleted',
            table_maintenance: 'Table maintenance run',
            table_cleanup: 'Table cleanup run',
        };

        const SUCCESS_ACTIONS = new Set(['login', 'create_interface', 'activate_interface', 'create_template', 'save_pipeline']);

        container.innerHTML = data.activity.map(row => {
            const label = ACTION_LABELS[row.action] || row.action.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
            const isSuccess = row.result === 'success' || SUCCESS_ACTIONS.has(row.action);
            const dotClass = isSuccess ? 'activity-dot success' : 'activity-dot';
            const who = row.user_name ? `by ${row.user_name.split(' ')[0]}` : '';
            const when = row.created_at ? _relativeTime(new Date(row.created_at)) : '';
            return `<div class="activity-item">
                <span class="${dotClass}"></span>
                <div class="activity-content">
                    <span class="activity-text">${label}${who ? ' <span style="opacity:.65">'+who+'</span>' : ''}</span>
                    <span class="activity-time">${when}</span>
                </div>
            </div>`;
        }).join('');
    } catch (e) {
        if (container) container.innerHTML = `<div class="activity-item">
            <span class="activity-dot"></span>
            <div class="activity-content"><span class="activity-text" style="color:#aaa">Could not load activity</span></div>
        </div>`;
    }
}

function _relativeTime(date) {
    const diff = Math.floor((Date.now() - date.getTime()) / 1000);
    if (diff < 60) return 'Just now';
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
    return `${Math.floor(diff / 86400)}d ago`;
}

// Fetch user count (admin only — silently skip if not admin)
async function loadUserCount() {
    try {
        const response = await fetch('/api/users', { credentials: 'include' });
        if (response.ok) {
            const data = await response.json();
            const count = Array.isArray(data) ? data.length : (data.total ?? 0);
            const el = document.getElementById('userCount');
            if (el) el.textContent = count;
        }
    } catch (e) {
        // Non-admin users will get 403 — expected, no need to log
    }
}

// Fetch system status: memory, queue depth, DLQ count
async function loadSystemStatus() {
    // Memory — Go runtime metrics
    try {
        const r = await fetch('/api/system/metrics', { credentials: 'include' });
        if (r.ok) {
            const d = await r.json();
            const memEl = document.getElementById('memoryStatus');
            if (memEl && d.metrics && d.metrics.system) {
                const pct = d.metrics.system.memory_pct;
                memEl.textContent = d.metrics.system.memory_display;
                memEl.className = 'status-value' + (pct > 80 ? ' error' : pct > 60 ? ' warning' : '');
            }
        }
    } catch (e) { /* non-critical */ }

    // Queue depth (processing) — backpressure module
    try {
        const r = await fetch('/api/system/queue-depths', { credentials: 'include' });
        if (r.ok) {
            const d = await r.json();
            const depths = d.data || {};
            const total = Object.values(depths).reduce((s, v) => s + (v && v.queue_depth ? v.queue_depth : 0), 0);
            const el = document.getElementById('queueDepth');
            if (el) el.textContent = total;
        }
    } catch (e) { /* non-critical */ }

    // DLQ count — unresolved dead letter entries
    try {
        const r = await fetch('/api/system/dlq?page_size=1', { credentials: 'include' });
        if (r.ok) {
            const d = await r.json();
            const count = d.pagination ? d.pagination.total : 0;
            const el = document.getElementById('dlqCount');
            const metric = document.getElementById('dlqMetric');
            if (el) el.textContent = count;
            if (metric) metric.classList.toggle('status-dual-metric--alert', count > 0);
        }
    } catch (e) { /* non-critical */ }
}

// Initialize on page load
window.addEventListener('load', function() {
    loadUserInfo();
    loadInterfaceCount();
    loadTemplateCount();
    loadRecentActivity();
    loadSystemStatus();
    updateTime();
    setInterval(updateTime, 60000); // Update time every minute
});

// Load dashboard-specific data that depends on the current user's role
// Sidebar, auth, user profile handled by sidebar-nav.js (universal component)
async function loadUserInfo() {
    try {
        const response = await fetch('/api/auth/session');
        if (response.ok) {
            const data = await response.json();
            const user = data.user || data;
            currentUser = user;

            // Update page greeting (dashboard-specific)
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            const pageTitle = document.querySelector('.page-title');
            if (pageTitle) pageTitle.textContent = `Welcome back, ${firstName}!`;

            if (user.role === 'admin' || user.role === 'superadmin') {
                const adminCard = document.getElementById('adminCard');
                if (adminCard) adminCard.style.display = 'block';
                loadUserCount();
            }
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        }
    } catch (error) {
        console.error('Error loading user info:', error);
        window.location.href = 'login.html';
    }
}