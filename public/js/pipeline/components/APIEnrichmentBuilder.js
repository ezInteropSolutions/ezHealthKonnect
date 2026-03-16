/**
 * @fileoverview APIEnrichmentBuilder — no-code UI for the API Enrichment pipeline step.
 * Sections: endpoint/method, auth (none/basic/bearer/apikey/oauth2), field mappings,
 * headers, query params, request body, timeout/retry, circuit breaker, cache.
 * @version 1.0.0
 */

class APIEnrichmentBuilder {

    constructor(panel) {
        /** @private */
        this._panel = panel;
        /** @private @type {FieldPathSearchComponent[]} */
        this._acs = [];
    }

    // ─── Public Builder Contract ─────────────────────────────────────────────

    render(step) {
        if (!step.config) step.config = {};
        const html = this._buildHTML(step);
        setTimeout(() => this._attachEvents(step), 0);
        return html;
    }

    collectConfig(step) {
        if (!step.config) step.config = {};
        const cfg = step.config;
        const num = (id, def) => { const v = parseInt(document.getElementById(id)?.value || String(def), 10); return isNaN(v) ? def : v; };

        cfg.endpoint     = document.getElementById('aebEndpoint')?.value?.trim()  || '';
        cfg.method       = document.getElementById('aebMethod')?.value             || 'GET';
        cfg.authType     = document.getElementById('aebAuthType')?.value           || 'none';
        cfg.timeoutMs    = num('aebTimeoutMs', 5000);
        cfg.retryCount   = num('aebRetryCount', 0);
        cfg.retryDelayMs = num('aebRetryDelayMs', 1000);
        cfg.failureThreshold    = num('aebFailureThreshold', 5);
        cfg.openDurationSeconds = num('aebOpenDurationSeconds', 60);
        cfg.cacheTtlSeconds     = num('aebCacheTtlSeconds', 0);
        cfg.cacheMaxEntries     = num('aebCacheMaxEntries', 1000);

        // Auth fields
        const at = cfg.authType;
        if (at === 'basic') {
            cfg.username = document.getElementById('aebUsername')?.value?.trim()     || '';
            cfg.password = document.getElementById('aebPassword')?.value?.trim()     || '';
        } else if (at === 'bearer') {
            cfg.bearerToken = document.getElementById('aebBearerToken')?.value?.trim() || '';
        } else if (at === 'apikey') {
            cfg.apiKey = document.getElementById('aebApiKey')?.value?.trim()         || '';
        } else if (at === 'oauth2') {
            cfg.oauth2TokenUrl     = document.getElementById('aebOAuth2TokenUrl')?.value?.trim()     || '';
            cfg.oauth2ClientId     = document.getElementById('aebOAuth2ClientId')?.value?.trim()     || '';
            cfg.oauth2ClientSecret = document.getElementById('aebOAuth2ClientSecret')?.value?.trim() || '';
            cfg.oauth2GrantType    = document.getElementById('aebOAuth2GrantType')?.value            || 'client_credentials';
            cfg.oauth2Scope        = document.getElementById('aebOAuth2Scope')?.value?.trim()        || '';
            cfg.oauth2Audience     = document.getElementById('aebOAuth2Audience')?.value?.trim()     || '';
        }

        // Field mappings: paramName → pipeline field path
        cfg.fieldMappings = {};
        document.querySelectorAll('#aebFMTable .aeb-fm-row').forEach(row => {
            const param = row.querySelector('.aeb-fm-param')?.value?.trim();
            const field = row.querySelector('.aeb-fm-field')?.value?.trim();
            if (param) cfg.fieldMappings[param] = field || '';
        });
        if (!Object.keys(cfg.fieldMappings).length) delete cfg.fieldMappings;

        // Headers
        cfg.headers = this._collectKV('#aebHdrTable');
        if (!Object.keys(cfg.headers).length) delete cfg.headers;

        // Query params
        cfg.queryParams = this._collectKV('#aebQPTable');
        if (!Object.keys(cfg.queryParams).length) delete cfg.queryParams;

        // Request body (POST/PUT/PATCH)
        const bodyText = document.getElementById('aebRequestBody')?.value?.trim();
        if (bodyText) {
            try { cfg.requestBody = JSON.parse(bodyText); }
            catch (_) { cfg.requestBody = { _raw: bodyText }; }
        } else { delete cfg.requestBody; }
    }

    destroy() {
        for (const ac of this._acs) {
            try { ac.destroy(); } catch (_) {}
        }
        this._acs = [];
    }

    // ─── HTML ─────────────────────────────────────────────────────────────────

    _buildHTML(step) {
        const cfg    = step.config || {};
        const esc    = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        const at     = cfg.authType || 'none';
        const method = cfg.method   || 'GET';
        const showBody = ['POST', 'PUT', 'PATCH'].includes(method);
        const hasFM  = Object.keys(cfg.fieldMappings || {}).length > 0;
        const hasHdr = Object.keys(cfg.headers       || {}).length > 0;
        const hasQP  = Object.keys(cfg.queryParams   || {}).length > 0;

        return `
<div class="aeb-builder">
<style>
.aeb-builder { font-size:13px; color:#1e3a5f; }
.aeb-section { border:1px solid rgba(30,58,138,0.15); border-radius:8px; margin-bottom:8px; overflow:hidden; }
.aeb-shdr {
    display:flex; align-items:center; gap:6px; padding:9px 12px;
    background:rgba(30,58,138,0.05); cursor:pointer; user-select:none;
    font-weight:600; font-size:12px;
}
.aeb-shdr:hover { background:rgba(30,58,138,0.10); }
.aeb-caret { margin-left:auto; font-size:10px; transition:transform .2s; }
.aeb-shdr.collapsed .aeb-caret { transform:rotate(-90deg); }
.aeb-shdr.collapsed + .aeb-sbody { display:none; }
.aeb-sbody { padding:12px; }
.aeb-badge { display:inline-block; padding:1px 7px; border-radius:10px; font-size:10px; font-weight:700; background:#dbeafe; color:#1d4ed8; }
.aeb-row { display:flex; gap:8px; margin-bottom:8px; align-items:flex-start; }
.aeb-col { display:flex; flex-direction:column; gap:3px; }
.aeb-col.grow { flex:1; min-width:0; }
.aeb-label { font-size:11px; font-weight:600; color:#374151; }
.aeb-hint  { font-size:10px; color:#94a3b8; margin-top:1px; }
.aeb-inp, .aeb-sel {
    padding:6px 8px; border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    font-size:12px; background:#fff; color:#1e3a5f; box-sizing:border-box; width:100%;
}
.aeb-inp:focus, .aeb-sel:focus { outline:none; border-color:#3b82f6; box-shadow:0 0 0 2px rgba(59,130,246,.15); }
.aeb-ta {
    padding:7px 9px; border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    font-size:11px; font-family:monospace; background:#f8fafc; color:#1e3a5f;
    width:100%; min-height:80px; resize:vertical; box-sizing:border-box;
}
.aeb-ta:focus { outline:none; border-color:#3b82f6; }
.aeb-irow { display:flex; gap:12px; flex-wrap:wrap; margin-bottom:8px; }
.aeb-irow .aeb-col { flex:1; min-width:90px; }

/* KV / FM rows */
.aeb-kv-row { display:grid; grid-template-columns:1fr 1fr 22px; gap:5px; margin-bottom:4px; align-items:center; }
.aeb-fm-row { display:grid; grid-template-columns:1fr 1fr 22px; gap:5px; margin-bottom:4px; align-items:center; }
.aeb-rm {
    width:22px; height:22px; border:none; border-radius:4px; background:#fee2e2;
    color:#ef4444; font-size:13px; cursor:pointer; padding:0; display:flex;
    align-items:center; justify-content:center; flex-shrink:0;
}
.aeb-rm:hover { background:#fca5a5; }
.aeb-add {
    display:inline-flex; align-items:center; gap:4px; padding:5px 10px;
    border:1px dashed rgba(30,58,138,0.35); border-radius:6px; background:none;
    color:#3b82f6; font-size:12px; cursor:pointer; margin-top:2px;
}
.aeb-add:hover { background:rgba(59,130,246,.07); }
.aeb-col-hdr { font-size:10px; font-weight:600; color:#64748b; margin-bottom:2px; }

/* Auth panels */
.aeb-auth-pnl { display:none; }
.aeb-auth-pnl.active { display:block; }
</style>

<!-- ① Request -->
<div class="aeb-section">
  <div class="aeb-shdr" data-aeb-sec="aebSRequest">
    🌐 Request <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSRequest">
    <div class="aeb-row">
      <div class="aeb-col grow">
        <span class="aeb-label">Endpoint URL *</span>
        <input id="aebEndpoint" class="aeb-inp" value="${esc(cfg.endpoint)}"
               placeholder="https://api.example.com/patients/{patientId}">
        <span class="aeb-hint">Use {paramName} for dynamic values from Field Mappings below</span>
      </div>
      <div class="aeb-col" style="width:110px">
        <span class="aeb-label">Method</span>
        <select id="aebMethod" class="aeb-sel">
          ${['GET','POST','PUT','PATCH','DELETE'].map(m =>
            `<option value="${m}"${method===m?' selected':''}>${m}</option>`).join('')}
        </select>
      </div>
    </div>
    <div class="aeb-irow">
      <div class="aeb-col">
        <span class="aeb-label">Timeout (ms)</span>
        <input id="aebTimeoutMs" class="aeb-inp" type="number" min="100" max="60000"
               value="${esc(cfg.timeoutMs ?? 5000)}" style="width:100px">
      </div>
      <div class="aeb-col">
        <span class="aeb-label">Retries</span>
        <input id="aebRetryCount" class="aeb-inp" type="number" min="0" max="10"
               value="${esc(cfg.retryCount ?? 0)}" style="width:70px">
      </div>
      <div class="aeb-col">
        <span class="aeb-label">Retry Delay (ms)</span>
        <input id="aebRetryDelayMs" class="aeb-inp" type="number" min="100" max="30000"
               value="${esc(cfg.retryDelayMs ?? 1000)}" style="width:100px">
      </div>
    </div>
  </div>
</div>

<!-- ② Auth -->
<div class="aeb-section">
  <div class="aeb-shdr${at==='none'?' collapsed':''}" data-aeb-sec="aebSAuth">
    🔑 Authentication${at!=='none'?` <span class="aeb-badge">${esc(at)}</span>`:''} <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSAuth">
    <div class="aeb-col" style="margin-bottom:10px">
      <span class="aeb-label">Auth Type</span>
      <select id="aebAuthType" class="aeb-sel" style="max-width:220px">
        ${[['none','None'],['basic','Basic (Username / Password)'],['bearer','Bearer Token'],
           ['apikey','API Key'],['oauth2','OAuth 2.0 (Client Credentials)']].map(
          ([v,l]) => `<option value="${v}"${at===v?' selected':''}>${l}</option>`).join('')}
      </select>
    </div>

    <div id="aebAuthNone" class="aeb-auth-pnl${at==='none'?' active':''}">
      <span class="aeb-hint">No credentials required.</span>
    </div>

    <div id="aebAuthBasic" class="aeb-auth-pnl${at==='basic'?' active':''}">
      <div class="aeb-irow">
        <div class="aeb-col">
          <span class="aeb-label">Username</span>
          <input id="aebUsername" class="aeb-inp" value="${esc(cfg.username)}" placeholder="user@example.com">
        </div>
        <div class="aeb-col">
          <span class="aeb-label">Password</span>
          <input id="aebPassword" class="aeb-inp" type="password" value="${esc(cfg.password)}" placeholder="••••••••">
        </div>
      </div>
    </div>

    <div id="aebAuthBearer" class="aeb-auth-pnl${at==='bearer'?' active':''}">
      <div class="aeb-col">
        <span class="aeb-label">Bearer Token</span>
        <input id="aebBearerToken" class="aeb-inp" value="${esc(cfg.bearerToken)}" placeholder="eyJhbGciOi...">
        <span class="aeb-hint">Sent as: Authorization: Bearer {token}</span>
      </div>
    </div>

    <div id="aebAuthApikey" class="aeb-auth-pnl${at==='apikey'?' active':''}">
      <div class="aeb-col">
        <span class="aeb-label">API Key</span>
        <input id="aebApiKey" class="aeb-inp" value="${esc(cfg.apiKey)}" placeholder="sk-...">
        <span class="aeb-hint">Sent as: X-API-Key: {key}</span>
      </div>
    </div>

    <div id="aebAuthOauth2" class="aeb-auth-pnl${at==='oauth2'?' active':''}">
      <div class="aeb-row">
        <div class="aeb-col grow">
          <span class="aeb-label">Token URL *</span>
          <input id="aebOAuth2TokenUrl" class="aeb-inp" value="${esc(cfg.oauth2TokenUrl)}"
                 placeholder="https://auth.example.com/oauth/token">
        </div>
        <div class="aeb-col" style="width:175px">
          <span class="aeb-label">Grant Type</span>
          <select id="aebOAuth2GrantType" class="aeb-sel">
            ${[['client_credentials','Client Credentials'],['password','Password'],['refresh_token','Refresh Token']].map(
              ([v,l]) => `<option value="${v}"${(cfg.oauth2GrantType||'client_credentials')===v?' selected':''}>${l}</option>`).join('')}
          </select>
        </div>
      </div>
      <div class="aeb-irow">
        <div class="aeb-col">
          <span class="aeb-label">Client ID *</span>
          <input id="aebOAuth2ClientId" class="aeb-inp" value="${esc(cfg.oauth2ClientId)}" placeholder="client_id">
        </div>
        <div class="aeb-col">
          <span class="aeb-label">Client Secret *</span>
          <input id="aebOAuth2ClientSecret" class="aeb-inp" type="password" value="${esc(cfg.oauth2ClientSecret)}" placeholder="••••••••">
        </div>
      </div>
      <div class="aeb-irow">
        <div class="aeb-col">
          <span class="aeb-label">Scope</span>
          <input id="aebOAuth2Scope" class="aeb-inp" value="${esc(cfg.oauth2Scope)}" placeholder="read:patients openid">
        </div>
        <div class="aeb-col">
          <span class="aeb-label">Audience</span>
          <input id="aebOAuth2Audience" class="aeb-inp" value="${esc(cfg.oauth2Audience)}" placeholder="https://api.example.com/">
        </div>
      </div>
    </div>
  </div>
</div>

<!-- ③ Field Mappings -->
<div class="aeb-section">
  <div class="aeb-shdr${!hasFM?' collapsed':''}" data-aeb-sec="aebSFM">
    🗺️ Field Mappings <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSFM">
    <p class="aeb-hint" style="margin:0 0 8px">Map pipeline fields into API parameters. Use {paramName} in the URL or request body.</p>
    <div style="display:grid;grid-template-columns:1fr 1fr 22px;gap:5px" class="aeb-col-hdr">
      <span>API Param Name</span><span>Pipeline Field Path</span><span></span>
    </div>
    <div id="aebFMTable">
      ${Object.entries(cfg.fieldMappings || {}).map(([p, f]) => this._fmRow(p, f, esc)).join('')}
    </div>
    <button class="aeb-add" id="aebAddFM" type="button">+ Add Mapping</button>
  </div>
</div>

<!-- ④ Headers -->
<div class="aeb-section">
  <div class="aeb-shdr${!hasHdr?' collapsed':''}" data-aeb-sec="aebSHdr">
    📋 Request Headers <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSHdr">
    <div style="display:grid;grid-template-columns:1fr 1fr 22px;gap:5px" class="aeb-col-hdr">
      <span>Header Name</span><span>Value</span><span></span>
    </div>
    <div id="aebHdrTable">
      ${Object.entries(cfg.headers || {}).map(([k, v]) => this._kvRow(k, v, esc)).join('')}
    </div>
    <button class="aeb-add" id="aebAddHdr" type="button">+ Add Header</button>
  </div>
</div>

<!-- ⑤ Query Parameters -->
<div class="aeb-section">
  <div class="aeb-shdr${!hasQP?' collapsed':''}" data-aeb-sec="aebSQP">
    🔍 Query Parameters <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSQP">
    <p class="aeb-hint" style="margin:0 0 8px">Static key=value pairs appended to the URL (?key=value&amp;...)</p>
    <div style="display:grid;grid-template-columns:1fr 1fr 22px;gap:5px" class="aeb-col-hdr">
      <span>Parameter</span><span>Value</span><span></span>
    </div>
    <div id="aebQPTable">
      ${Object.entries(cfg.queryParams || {}).map(([k, v]) => this._kvRow(k, v, esc)).join('')}
    </div>
    <button class="aeb-add" id="aebAddQP" type="button">+ Add Parameter</button>
  </div>
</div>

<!-- ⑥ Request Body -->
<div class="aeb-section${showBody?'':' aeb-hidden'}" id="aebBodySection">
  <div class="aeb-shdr${!cfg.requestBody?' collapsed':''}" data-aeb-sec="aebSBody">
    📝 Request Body (JSON) <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSBody">
    <p class="aeb-hint" style="margin:0 0 6px">JSON template. Use {paramName} from Field Mappings for dynamic values.</p>
    <textarea id="aebRequestBody" class="aeb-ta">${esc(cfg.requestBody ? JSON.stringify(cfg.requestBody, null, 2) : '')}</textarea>
  </div>
</div>

<!-- ⑦ Circuit Breaker -->
<div class="aeb-section">
  <div class="aeb-shdr collapsed" data-aeb-sec="aebSCB">
    🔴 Circuit Breaker <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSCB">
    <p class="aeb-hint" style="margin:0 0 8px">Opens after N consecutive failures; stays open M seconds before probing again.</p>
    <div class="aeb-irow">
      <div class="aeb-col">
        <span class="aeb-label">Failure Threshold</span>
        <input id="aebFailureThreshold" class="aeb-inp" type="number" min="1" max="100"
               value="${esc(cfg.failureThreshold ?? 5)}" style="width:90px">
        <span class="aeb-hint">consecutive failures</span>
      </div>
      <div class="aeb-col">
        <span class="aeb-label">Open Duration</span>
        <input id="aebOpenDurationSeconds" class="aeb-inp" type="number" min="1" max="3600"
               value="${esc(cfg.openDurationSeconds ?? 60)}" style="width:90px">
        <span class="aeb-hint">seconds</span>
      </div>
    </div>
  </div>
</div>

<!-- ⑧ Cache -->
<div class="aeb-section">
  <div class="aeb-shdr collapsed" data-aeb-sec="aebSCache">
    💾 Result Cache <span class="aeb-caret">▾</span>
  </div>
  <div class="aeb-sbody" id="aebSCache">
    <p class="aeb-hint" style="margin:0 0 8px">In-memory LRU+TTL cache keyed by field mapping values. Set TTL=0 to disable.</p>
    <div class="aeb-irow">
      <div class="aeb-col">
        <span class="aeb-label">Cache TTL</span>
        <input id="aebCacheTtlSeconds" class="aeb-inp" type="number" min="0" max="86400"
               value="${esc(cfg.cacheTtlSeconds ?? 0)}" style="width:100px">
        <span class="aeb-hint">seconds (0 = disabled)</span>
      </div>
      <div class="aeb-col">
        <span class="aeb-label">Max Entries</span>
        <input id="aebCacheMaxEntries" class="aeb-inp" type="number" min="1" max="100000"
               value="${esc(cfg.cacheMaxEntries ?? 1000)}" style="width:100px">
      </div>
    </div>
  </div>
</div>
</div>`;
    }

    // ─── Row Renderers ────────────────────────────────────────────────────────

    _kvRow(k, v, esc) {
        if (!esc) esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        return `<div class="aeb-kv-row">
            <input class="aeb-inp aeb-kv-key" value="${esc(k)}" placeholder="Name">
            <input class="aeb-inp aeb-kv-val" value="${esc(v)}" placeholder="Value">
            <button class="aeb-rm" type="button" title="Remove">×</button>
        </div>`;
    }

    _fmRow(param, field, esc) {
        if (!esc) esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        const uid = `aebFmF_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`;
        return `<div class="aeb-fm-row">
            <input class="aeb-inp aeb-fm-param" value="${esc(param)}" placeholder="paramName (e.g. patientId)">
            <input class="aeb-inp aeb-fm-field" id="${uid}" value="${esc(field)}" placeholder="e.g. PID.3">
            <button class="aeb-rm" type="button" title="Remove">×</button>
        </div>`;
    }

    // ─── Events ──────────────────────────────────────────────────────────────

    _attachEvents(step) {
        // Collapse toggles (CSS .collapsed + .aeb-sbody handles visibility)
        document.querySelectorAll('.aeb-shdr[data-aeb-sec]').forEach(hdr => {
            hdr.addEventListener('click', () => hdr.classList.toggle('collapsed'));
        });

        // Auth type change
        document.getElementById('aebAuthType')?.addEventListener('change', e => {
            this._showAuthPanel(e.target.value);
        });

        // Method change → show/hide body section
        document.getElementById('aebMethod')?.addEventListener('change', e => {
            const show = ['POST', 'PUT', 'PATCH'].includes(e.target.value);
            const s = document.getElementById('aebBodySection');
            if (s) s.style.display = show ? '' : 'none';
        });

        // Add header row
        document.getElementById('aebAddHdr')?.addEventListener('click', () => {
            this._appendKVRow('#aebHdrTable');
        });

        // Add query param row
        document.getElementById('aebAddQP')?.addEventListener('click', () => {
            this._appendKVRow('#aebQPTable');
        });

        // Add field mapping row
        document.getElementById('aebAddFM')?.addEventListener('click', () => {
            const table = document.querySelector('#aebFMTable');
            if (!table) return;
            const div = document.createElement('div');
            div.innerHTML = this._fmRow('', '');
            const row = div.firstElementChild;
            table.appendChild(row);
            this._wireRemoveBtn(row);
            const input = row.querySelector('.aeb-fm-field');
            if (input) this._attachFieldAC(input);
        });

        // Wire remove buttons on initial rows
        document.querySelectorAll('#aebHdrTable .aeb-rm, #aebQPTable .aeb-rm').forEach(btn => {
            this._wireRemoveBtn(btn.closest('.aeb-kv-row'));
        });
        document.querySelectorAll('#aebFMTable .aeb-rm').forEach(btn => {
            this._wireRemoveBtn(btn.closest('.aeb-fm-row'));
        });

        // Attach FieldPathSearch to existing FM field inputs
        document.querySelectorAll('#aebFMTable .aeb-fm-field').forEach(input => {
            this._attachFieldAC(input);
        });
    }

    _showAuthPanel(authType) {
        const panelMap = {
            none:   'aebAuthNone',
            basic:  'aebAuthBasic',
            bearer: 'aebAuthBearer',
            apikey: 'aebAuthApikey',
            oauth2: 'aebAuthOauth2',
        };
        Object.entries(panelMap).forEach(([t, id]) => {
            const el = document.getElementById(id);
            if (el) el.classList.toggle('active', t === authType);
        });
    }

    _appendKVRow(tableSelector) {
        const table = document.querySelector(tableSelector);
        if (!table) return;
        const div = document.createElement('div');
        div.innerHTML = this._kvRow('', '');
        const row = div.firstElementChild;
        this._wireRemoveBtn(row);
        table.appendChild(row);
        row.querySelector('.aeb-kv-key')?.focus();
    }

    _wireRemoveBtn(row) {
        const btn = row?.querySelector('.aeb-rm');
        if (btn && !btn._aebBound) {
            btn._aebBound = true;
            btn.addEventListener('click', () => row.remove());
        }
    }

    _attachFieldAC(inputEl) {
        if (!inputEl || !window.FieldPathSearchComponent) return;
        try {
            const ac = new FieldPathSearchComponent(inputEl, {
                builder: this._panel?.builder,
                onSelect: null,
                placeholder: 'Search pipeline vars...',
                maxSuggestions: 20,
            });
            this._acs.push(ac);
        } catch (_) {}
    }

    _collectKV(tableSelector) {
        const result = {};
        document.querySelectorAll(`${tableSelector} .aeb-kv-row`).forEach(row => {
            const k = row.querySelector('.aeb-kv-key')?.value?.trim();
            const v = row.querySelector('.aeb-kv-val')?.value?.trim();
            if (k) result[k] = v || '';
        });
        return result;
    }
}

StepBuilderRegistry.register('enrichment.api', APIEnrichmentBuilder);
StepBuilderRegistry.register('pre.enrichment.api', APIEnrichmentBuilder);
