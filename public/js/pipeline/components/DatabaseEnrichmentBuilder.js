/**
 * @fileoverview DatabaseEnrichmentBuilder — no-code UI for the Database Enrichment pipeline step.
 * Covers: DB type selector, connection, query (SQL/MongoDB/Redis/DynamoDB adaptive),
 * query params, result mapping, cache config, connection pool config.
 * @version 1.0.0
 */

class DatabaseEnrichmentBuilder {

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

        cfg.databaseType       = document.getElementById('debDbType')?.value           || 'postgresql';
        cfg.timeoutMs          = num('debTimeoutMs', 3000);
        cfg.cacheResults       = document.getElementById('debCacheResults')?.checked   || false;
        cfg.cacheTTL           = num('debCacheTTL', 0);
        cfg.cacheMaxEntries    = num('debCacheMaxEntries', 1000);
        cfg.poolMaxOpen        = num('debPoolMaxOpen', 10);
        cfg.poolMaxIdle        = num('debPoolMaxIdle', 5);
        cfg.poolMaxLifetimeMinutes = num('debPoolMaxLifetime', 5);

        // Connection
        const connMode = document.querySelector('input[name="debConnMode"]:checked')?.value || 'fields';
        if (connMode === 'string') {
            cfg.connectionString = document.getElementById('debConnStr')?.value?.trim() || '';
            delete cfg.dbHost; delete cfg.dbPort; delete cfg.dbName;
            delete cfg.dbUser; delete cfg.dbPassword;
        } else {
            cfg.dbHost     = document.getElementById('debDbHost')?.value?.trim()  || '';
            cfg.dbPort     = num('debDbPort', this._defaultPort(cfg.databaseType));
            cfg.dbName     = document.getElementById('debDbName')?.value?.trim()  || '';
            cfg.dbUser     = document.getElementById('debDbUser')?.value?.trim()  || '';
            cfg.dbPassword = document.getElementById('debDbPassword')?.value?.trim() || '';
            delete cfg.connectionString;
        }

        // Query section depends on DB type
        const dbType = cfg.databaseType;
        if (dbType === 'mongodb') {
            cfg.collection = document.getElementById('debCollection')?.value?.trim() || '';
            const filterText = document.getElementById('debMongoFilter')?.value?.trim();
            if (filterText) {
                try { cfg.filter = JSON.parse(filterText); }
                catch (_) { cfg.filter = { _raw: filterText }; }
            } else { delete cfg.filter; }
            const projText = document.getElementById('debMongoProjection')?.value?.trim();
            if (projText) {
                try { cfg.projection = JSON.parse(projText); }
                catch (_) {}
            } else { delete cfg.projection; }
        } else if (dbType === 'redis') {
            cfg.redisKey     = document.getElementById('debRedisKey')?.value?.trim()  || '';
            cfg.redisCommand = document.getElementById('debRedisCommand')?.value       || 'GET';
        } else if (dbType === 'dynamodb') {
            cfg.dynamodbTable    = document.getElementById('debDynamoTable')?.value?.trim()        || '';
            cfg.keyCondition     = document.getElementById('debDynamoKeyCondition')?.value?.trim() || '';
            cfg.filterExpression = document.getElementById('debDynamoFilter')?.value?.trim()       || '';
        } else {
            // SQL-family (postgresql, mysql, sqlserver, oracle, snowflake, databricks, cassandra)
            cfg.query = document.getElementById('debQuery')?.value?.trim() || '';
        }

        // Query params: paramName → field path
        cfg.queryParams = {};
        document.querySelectorAll('#debQPTable .deb-qp-row').forEach(row => {
            const k = row.querySelector('.deb-qp-key')?.value?.trim();
            const v = row.querySelector('.deb-qp-val')?.value?.trim();
            if (k) cfg.queryParams[k] = v || '';
        });
        if (!Object.keys(cfg.queryParams).length) delete cfg.queryParams;

        // Result mapping: DB column → output field
        cfg.resultMapping = {};
        document.querySelectorAll('#debRMTable .deb-kv-row').forEach(row => {
            const k = row.querySelector('.deb-kv-key')?.value?.trim();
            const v = row.querySelector('.deb-kv-val')?.value?.trim();
            if (k) cfg.resultMapping[k] = v || '';
        });
        if (!Object.keys(cfg.resultMapping).length) delete cfg.resultMapping;
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
        const dbType = cfg.databaseType || 'postgresql';
        const hasQP  = Object.keys(cfg.queryParams  || {}).length > 0;
        const hasRM  = Object.keys(cfg.resultMapping || {}).length > 0;
        const connMode = cfg.connectionString ? 'string' : 'fields';

        const isMongo   = dbType === 'mongodb';
        const isRedis   = dbType === 'redis';
        const isDynamo  = dbType === 'dynamodb';
        const isSQLfam  = !isMongo && !isRedis && !isDynamo;

        return `
<div class="deb-builder">
<style>
.deb-builder { font-size:13px; color:#1e3a5f; }
.deb-section { border:1px solid rgba(30,58,138,0.15); border-radius:8px; margin-bottom:8px; overflow:hidden; }
.deb-shdr {
    display:flex; align-items:center; gap:6px; padding:9px 12px;
    background:rgba(30,58,138,0.05); cursor:pointer; user-select:none;
    font-weight:600; font-size:12px;
}
.deb-shdr:hover { background:rgba(30,58,138,0.10); }
.deb-caret { margin-left:auto; font-size:10px; transition:transform .2s; }
.deb-shdr.collapsed .deb-caret { transform:rotate(-90deg); }
.deb-shdr.collapsed + .deb-sbody { display:none; }
.deb-sbody { padding:12px; }
.deb-row { display:flex; gap:8px; margin-bottom:8px; align-items:flex-start; }
.deb-col { display:flex; flex-direction:column; gap:3px; }
.deb-col.grow { flex:1; min-width:0; }
.deb-label { font-size:11px; font-weight:600; color:#374151; }
.deb-hint  { font-size:10px; color:#94a3b8; margin-top:1px; }
.deb-inp, .deb-sel {
    padding:6px 8px; border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    font-size:12px; background:#fff; color:#1e3a5f; box-sizing:border-box; width:100%;
}
.deb-inp:focus, .deb-sel:focus { outline:none; border-color:#3b82f6; box-shadow:0 0 0 2px rgba(59,130,246,.15); }
.deb-ta {
    padding:7px 9px; border:1px solid rgba(30,58,138,0.25); border-radius:6px;
    font-size:11px; font-family:monospace; background:#f8fafc; color:#1e3a5f;
    width:100%; min-height:90px; resize:vertical; box-sizing:border-box;
}
.deb-ta:focus { outline:none; border-color:#3b82f6; }
.deb-irow { display:flex; gap:12px; flex-wrap:wrap; margin-bottom:8px; }
.deb-irow .deb-col { flex:1; min-width:90px; }

/* DB type picker grid */
.deb-db-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(110px,1fr)); gap:6px; }
.deb-db-btn {
    display:flex; align-items:center; gap:6px; padding:7px 10px;
    border:1.5px solid rgba(30,58,138,0.20); border-radius:7px;
    background:#fff; cursor:pointer; font-size:12px; color:#1e3a5f;
    transition:all .15s;
}
.deb-db-btn:hover { border-color:#3b82f6; background:rgba(59,130,246,.05); }
.deb-db-btn.active { border-color:#3b82f6; background:rgba(59,130,246,.10); font-weight:600; }
.deb-db-icon { font-size:16px; }

/* Conn mode radio */
.deb-conn-mode { display:flex; gap:16px; margin-bottom:10px; }
.deb-conn-mode label { display:flex; align-items:center; gap:5px; font-size:12px; cursor:pointer; }
.deb-conn-panel { display:none; }
.deb-conn-panel.active { display:block; }

/* KV rows */
.deb-kv-row, .deb-qp-row { display:grid; grid-template-columns:1fr 1fr 22px; gap:5px; margin-bottom:4px; align-items:center; }
.deb-rm {
    width:22px; height:22px; border:none; border-radius:4px; background:#fee2e2;
    color:#ef4444; font-size:13px; cursor:pointer; padding:0; display:flex;
    align-items:center; justify-content:center;
}
.deb-rm:hover { background:#fca5a5; }
.deb-add {
    display:inline-flex; align-items:center; gap:4px; padding:5px 10px;
    border:1px dashed rgba(30,58,138,0.35); border-radius:6px; background:none;
    color:#3b82f6; font-size:12px; cursor:pointer; margin-top:2px;
}
.deb-add:hover { background:rgba(59,130,246,.07); }
.deb-col-hdr { font-size:10px; font-weight:600; color:#64748b; margin-bottom:2px; }

/* Query sections */
.deb-query-section { display:none; }
.deb-query-section.active { display:block; }

/* Cache toggle */
.deb-cache-row { display:flex; align-items:center; gap:8px; margin-bottom:10px; }
.deb-toggle { position:relative; display:inline-block; width:36px; height:20px; flex-shrink:0; }
.deb-toggle input { opacity:0; width:0; height:0; }
.deb-toggle-slider {
    position:absolute; cursor:pointer; top:0; left:0; right:0; bottom:0;
    background:#d1d5db; border-radius:20px; transition:.2s;
}
.deb-toggle input:checked + .deb-toggle-slider { background:#3b82f6; }
.deb-toggle-slider:before {
    content:''; position:absolute; height:14px; width:14px; left:3px; bottom:3px;
    background:white; border-radius:50%; transition:.2s;
}
.deb-toggle input:checked + .deb-toggle-slider:before { transform:translateX(16px); }
</style>

<!-- ① Database Type -->
<div class="deb-section">
  <div class="deb-shdr" data-deb-sec="debSType">
    🗄️ Database Type <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSType">
    <div class="deb-db-grid" id="debDbTypePicker">
      ${DatabaseEnrichmentBuilder.DB_TYPES.map(({ value, icon, label }) => `
        <button class="deb-db-btn${dbType === value ? ' active' : ''}" data-dbtype="${value}" type="button">
          <span class="deb-db-icon">${icon}</span>${label}
        </button>`).join('')}
    </div>
    <input type="hidden" id="debDbType" value="${esc(dbType)}">
  </div>
</div>

<!-- ② Connection -->
<div class="deb-section">
  <div class="deb-shdr" data-deb-sec="debSConn">
    🔌 Connection <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSConn">
    <div class="deb-conn-mode">
      <label>
        <input type="radio" name="debConnMode" value="fields"${connMode==='fields'?' checked':''}>
        Individual fields
      </label>
      <label>
        <input type="radio" name="debConnMode" value="string"${connMode==='string'?' checked':''}>
        Connection string
      </label>
    </div>

    <div id="debConnFields" class="deb-conn-panel${connMode==='fields'?' active':''}">
      <div class="deb-irow">
        <div class="deb-col" style="flex:2">
          <span class="deb-label">Host</span>
          <input id="debDbHost" class="deb-inp" value="${esc(cfg.dbHost)}" placeholder="localhost">
        </div>
        <div class="deb-col" style="width:80px">
          <span class="deb-label">Port</span>
          <input id="debDbPort" class="deb-inp" type="number" value="${esc(cfg.dbPort || this._defaultPort(dbType))}" placeholder="5432">
        </div>
      </div>
      <div class="deb-irow">
        <div class="deb-col">
          <span class="deb-label">Database Name</span>
          <input id="debDbName" class="deb-inp" value="${esc(cfg.dbName)}" placeholder="mydb">
        </div>
        <div class="deb-col">
          <span class="deb-label">Username</span>
          <input id="debDbUser" class="deb-inp" value="${esc(cfg.dbUser)}" placeholder="postgres">
        </div>
        <div class="deb-col">
          <span class="deb-label">Password</span>
          <input id="debDbPassword" class="deb-inp" type="password" value="${esc(cfg.dbPassword)}" placeholder="••••••••">
        </div>
      </div>
    </div>

    <div id="debConnString" class="deb-conn-panel${connMode==='string'?' active':''}">
      <div class="deb-col">
        <span class="deb-label">Connection String</span>
        <input id="debConnStr" class="deb-inp" value="${esc(cfg.connectionString)}"
               placeholder="postgresql://user:pass@host:5432/dbname">
        <span class="deb-hint">Supports all standard DSN formats including MongoDB URI</span>
      </div>
    </div>

    <div class="deb-irow" style="margin-top:8px">
      <div class="deb-col">
        <span class="deb-label">Timeout (ms)</span>
        <input id="debTimeoutMs" class="deb-inp" type="number" min="100" max="60000"
               value="${esc(cfg.timeoutMs ?? 3000)}" style="width:100px">
      </div>
    </div>
  </div>
</div>

<!-- ③ Query -->
<div class="deb-section">
  <div class="deb-shdr" data-deb-sec="debSQuery">
    📋 Query <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSQuery">

    <!-- SQL family -->
    <div class="deb-query-section${isSQLfam?' active':''}" id="debQuerSQL">
      <div class="deb-col">
        <span class="deb-label">SQL Query</span>
        <textarea id="debQuery" class="deb-ta" placeholder="SELECT * FROM patients WHERE patient_id = $1">${esc(cfg.query)}</textarea>
        <span class="deb-hint">Use $1, $2, … (PostgreSQL/Oracle) or ? (MySQL/SQL Server) for params. Map them in Query Parameters below.</span>
      </div>
    </div>

    <!-- MongoDB -->
    <div class="deb-query-section${isMongo?' active':''}" id="debQuerMongo">
      <div class="deb-col" style="margin-bottom:8px">
        <span class="deb-label">Collection *</span>
        <input id="debCollection" class="deb-inp" value="${esc(cfg.collection)}" placeholder="patients">
      </div>
      <div class="deb-col" style="margin-bottom:8px">
        <span class="deb-label">Filter (JSON)</span>
        <textarea id="debMongoFilter" class="deb-ta" style="min-height:70px"
                  placeholder='{"patient_id": "{{patientId}}"}'>${esc(cfg.filter ? JSON.stringify(cfg.filter, null, 2) : '')}</textarea>
        <span class="deb-hint">Use {"field": "{{paramName}}"} — paramName is resolved from Query Parameters.</span>
      </div>
      <div class="deb-col">
        <span class="deb-label">Projection (JSON, optional)</span>
        <textarea id="debMongoProjection" class="deb-ta" style="min-height:50px"
                  placeholder='{"_id": 0, "name": 1, "dob": 1}'>${esc(cfg.projection ? JSON.stringify(cfg.projection, null, 2) : '')}</textarea>
      </div>
    </div>

    <!-- Redis -->
    <div class="deb-query-section${isRedis?' active':''}" id="debQuerRedis">
      <div class="deb-irow">
        <div class="deb-col grow">
          <span class="deb-label">Key Pattern</span>
          <input id="debRedisKey" class="deb-inp" value="${esc(cfg.redisKey)}" placeholder="patient:{patientId}">
          <span class="deb-hint">Use {paramName} — resolved from Query Parameters below</span>
        </div>
        <div class="deb-col" style="width:140px">
          <span class="deb-label">Command</span>
          <select id="debRedisCommand" class="deb-sel">
            ${['GET','HGETALL','SMEMBERS','LRANGE','ZRANGE','MGET'].map(c =>
              `<option value="${c}"${(cfg.redisCommand||'GET')===c?' selected':''}>${c}</option>`).join('')}
          </select>
        </div>
      </div>
    </div>

    <!-- DynamoDB -->
    <div class="deb-query-section${isDynamo?' active':''}" id="debQuerDynamo">
      <div class="deb-col" style="margin-bottom:8px">
        <span class="deb-label">Table Name</span>
        <input id="debDynamoTable" class="deb-inp" value="${esc(cfg.dynamodbTable)}" placeholder="PatientRecords">
      </div>
      <div class="deb-col" style="margin-bottom:8px">
        <span class="deb-label">Key Condition Expression</span>
        <input id="debDynamoKeyCondition" class="deb-inp" value="${esc(cfg.keyCondition)}"
               placeholder="PatientId = :patientId">
      </div>
      <div class="deb-col">
        <span class="deb-label">Filter Expression (optional)</span>
        <input id="debDynamoFilter" class="deb-inp" value="${esc(cfg.filterExpression)}"
               placeholder="Status = :status">
      </div>
    </div>

  </div>
</div>

<!-- ④ Query Parameters -->
<div class="deb-section">
  <div class="deb-shdr${!hasQP?' collapsed':''}" data-deb-sec="debSQP">
    🔗 Query Parameters <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSQP">
    <p class="deb-hint" style="margin:0 0 8px">Map SQL param names ($1 → first key) or template placeholders ({paramName}) to pipeline field paths.</p>
    <div style="display:grid;grid-template-columns:1fr 1fr 22px;gap:5px" class="deb-col-hdr">
      <span>Param Name</span><span>Pipeline Field Path</span><span></span>
    </div>
    <div id="debQPTable">
      ${Object.entries(cfg.queryParams || {}).map(([k, v]) => this._qpRow(k, v, esc)).join('')}
    </div>
    <button class="deb-add" id="debAddQP" type="button">+ Add Parameter</button>
  </div>
</div>

<!-- ⑤ Result Mapping -->
<div class="deb-section">
  <div class="deb-shdr${!hasRM?' collapsed':''}" data-deb-sec="debSRM">
    📤 Result Mapping <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSRM">
    <p class="deb-hint" style="margin:0 0 8px">Rename DB columns in the step output. Leave empty to pass all columns as-is.</p>
    <div style="display:grid;grid-template-columns:1fr 1fr 22px;gap:5px" class="deb-col-hdr">
      <span>DB Column</span><span>Output Field Name</span><span></span>
    </div>
    <div id="debRMTable">
      ${Object.entries(cfg.resultMapping || {}).map(([k, v]) => this._kvRow(k, v, esc)).join('')}
    </div>
    <button class="deb-add" id="debAddRM" type="button">+ Add Mapping</button>
  </div>
</div>

<!-- ⑥ Cache -->
<div class="deb-section">
  <div class="deb-shdr collapsed" data-deb-sec="debSCache">
    💾 Result Cache <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSCache">
    <div class="deb-cache-row">
      <label class="deb-toggle">
        <input type="checkbox" id="debCacheResults"${cfg.cacheResults?' checked':''}>
        <span class="deb-toggle-slider"></span>
      </label>
      <span class="deb-label">Enable caching</span>
    </div>
    <div class="deb-irow">
      <div class="deb-col">
        <span class="deb-label">Cache TTL</span>
        <input id="debCacheTTL" class="deb-inp" type="number" min="0" max="86400"
               value="${esc(cfg.cacheTTL ?? 0)}" style="width:100px">
        <span class="deb-hint">seconds (0 = disabled)</span>
      </div>
      <div class="deb-col">
        <span class="deb-label">Max Entries</span>
        <input id="debCacheMaxEntries" class="deb-inp" type="number" min="1" max="100000"
               value="${esc(cfg.cacheMaxEntries ?? 1000)}" style="width:100px">
      </div>
    </div>
  </div>
</div>

<!-- ⑦ Connection Pool -->
<div class="deb-section">
  <div class="deb-shdr collapsed" data-deb-sec="debSPool">
    ♟️ Connection Pool <span class="deb-caret">▾</span>
  </div>
  <div class="deb-sbody" id="debSPool">
    <div class="deb-irow">
      <div class="deb-col">
        <span class="deb-label">Max Open</span>
        <input id="debPoolMaxOpen" class="deb-inp" type="number" min="1" max="500"
               value="${esc(cfg.poolMaxOpen ?? 10)}" style="width:80px">
      </div>
      <div class="deb-col">
        <span class="deb-label">Max Idle</span>
        <input id="debPoolMaxIdle" class="deb-inp" type="number" min="0" max="100"
               value="${esc(cfg.poolMaxIdle ?? 5)}" style="width:80px">
      </div>
      <div class="deb-col">
        <span class="deb-label">Max Lifetime</span>
        <input id="debPoolMaxLifetime" class="deb-inp" type="number" min="1" max="120"
               value="${esc(cfg.poolMaxLifetimeMinutes ?? 5)}" style="width:80px">
        <span class="deb-hint">minutes</span>
      </div>
    </div>
  </div>
</div>
</div>`;
    }

    // ─── Row Renderers ────────────────────────────────────────────────────────

    _kvRow(k, v, esc) {
        if (!esc) esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        return `<div class="deb-kv-row">
            <input class="deb-inp deb-kv-key" value="${esc(k)}" placeholder="Column">
            <input class="deb-inp deb-kv-val" value="${esc(v)}" placeholder="Output field">
            <button class="deb-rm" type="button" title="Remove">×</button>
        </div>`;
    }

    _qpRow(k, v, esc) {
        if (!esc) esc = s => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/"/g, '&quot;');
        const uid = `debQpV_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`;
        return `<div class="deb-qp-row">
            <input class="deb-inp deb-qp-key" value="${esc(k)}" placeholder="patientId">
            <input class="deb-inp deb-qp-val" id="${uid}" value="${esc(v)}" placeholder="e.g. PID.3">
            <button class="deb-rm" type="button" title="Remove">×</button>
        </div>`;
    }

    // ─── Events ──────────────────────────────────────────────────────────────

    _attachEvents(step) {
        // Section collapse toggles
        document.querySelectorAll('.deb-shdr[data-deb-sec]').forEach(hdr => {
            hdr.addEventListener('click', () => hdr.classList.toggle('collapsed'));
        });

        // DB type picker
        document.querySelectorAll('#debDbTypePicker .deb-db-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('#debDbTypePicker .deb-db-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                const dbType = btn.dataset.dbtype;
                document.getElementById('debDbType').value = dbType;
                this._showQuerySection(dbType);
                // Update port placeholder
                const portEl = document.getElementById('debDbPort');
                if (portEl && !portEl.value) portEl.placeholder = String(this._defaultPort(dbType));
            });
        });

        // Connection mode toggle
        document.querySelectorAll('input[name="debConnMode"]').forEach(radio => {
            radio.addEventListener('change', () => {
                const mode = radio.value;
                document.getElementById('debConnFields').classList.toggle('active', mode === 'fields');
                document.getElementById('debConnString').classList.toggle('active', mode === 'string');
            });
        });

        // Add query param row
        document.getElementById('debAddQP')?.addEventListener('click', () => {
            const table = document.querySelector('#debQPTable');
            if (!table) return;
            const div = document.createElement('div');
            div.innerHTML = this._qpRow('', '');
            const row = div.firstElementChild;
            this._wireRemoveBtn(row, '.deb-rm');
            table.appendChild(row);
            const valInput = row.querySelector('.deb-qp-val');
            if (valInput) this._attachFieldAC(valInput);
        });

        // Add result mapping row
        document.getElementById('debAddRM')?.addEventListener('click', () => {
            this._appendKVRow('#debRMTable', '.deb-kv-row', this._kvRow.bind(this));
        });

        // Wire existing remove buttons
        document.querySelectorAll('#debQPTable .deb-rm').forEach(btn =>
            this._wireRemoveBtn(btn.closest('.deb-qp-row'), '.deb-rm'));
        document.querySelectorAll('#debRMTable .deb-rm').forEach(btn =>
            this._wireRemoveBtn(btn.closest('.deb-kv-row'), '.deb-rm'));

        // Attach FieldPathSearch to existing QP value inputs
        document.querySelectorAll('#debQPTable .deb-qp-val').forEach(input => {
            this._attachFieldAC(input);
        });
    }

    _showQuerySection(dbType) {
        const sectionMap = {
            mongodb:  'debQuerMongo',
            redis:    'debQuerRedis',
            dynamodb: 'debQuerDynamo',
        };
        const target = sectionMap[dbType] || 'debQuerSQL';
        document.querySelectorAll('.deb-query-section').forEach(el => {
            el.classList.toggle('active', el.id === target);
        });
    }

    _wireRemoveBtn(row, rmSel) {
        const btn = row?.querySelector(rmSel || '.deb-rm');
        if (btn && !btn._debBound) {
            btn._debBound = true;
            btn.addEventListener('click', () => row.remove());
        }
    }

    _appendKVRow(tableSelector, rowClass, rowFn) {
        const table = document.querySelector(tableSelector);
        if (!table) return;
        const div = document.createElement('div');
        div.innerHTML = rowFn('', '');
        const row = div.firstElementChild;
        this._wireRemoveBtn(row, '.deb-rm');
        table.appendChild(row);
        row.querySelector('.deb-kv-key')?.focus();
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

    _defaultPort(dbType) {
        const ports = {
            postgresql: 5432, mysql: 3306, sqlserver: 1433, oracle: 1521,
            mongodb: 27017, redis: 6379, cassandra: 9042,
        };
        return ports[dbType] || 5432;
    }
}

DatabaseEnrichmentBuilder.DB_TYPES = [
    { value: 'postgresql', icon: '🐘', label: 'PostgreSQL' },
    { value: 'mysql',      icon: '🐬', label: 'MySQL' },
    { value: 'sqlserver',  icon: '🪟', label: 'SQL Server' },
    { value: 'oracle',     icon: '🔴', label: 'Oracle' },
    { value: 'mongodb',    icon: '🍃', label: 'MongoDB' },
    { value: 'redis',      icon: '⚡', label: 'Redis' },
    { value: 'dynamodb',   icon: '☁️',  label: 'DynamoDB' },
    { value: 'cassandra',  icon: '👁️',  label: 'Cassandra' },
    { value: 'snowflake',  icon: '❄️',  label: 'Snowflake' },
    { value: 'databricks', icon: '🧱', label: 'Databricks' },
];

StepBuilderRegistry.register('database_enrichment', DatabaseEnrichmentBuilder);
StepBuilderRegistry.register('pre.enrichment.database', DatabaseEnrichmentBuilder);
