// public/js/components/EndpointAuthEditor.js
//
// Edit-modal section for an interface's partner-endpoint authentication
// (interfaces.source_config.endpoint_auth). Payer / pharmacy / provider
// systems calling this interface's synchronous endpoints —
//   POST /api/eligibility/:interfaceId/check
//   POST /api/claim-status/:interfaceId/check
//   POST /api/prior-auth/:interfaceId/check
//   POST /api/pharmacy-claim/:interfaceId/submit
// — must present these credentials (verified server-side in Go by
// controllers/sync_endpoint_auth.go). Without them every partner call is
// refused (secure by default).
//
// Secrets come back from the API masked ("••••••••"); leaving a masked field
// untouched keeps the stored value, matching the connector-credential
// convention elsewhere in the edit modal.
class EndpointAuthEditor {
    static MASK = '••••••••';

    static TYPES = [
        { value: '', label: 'Not configured — partner calls are refused' },
        { value: 'api_key', label: 'API key (header)' },
        { value: 'basic_auth', label: 'Basic (username / password)' },
        { value: 'bearer_token', label: 'Bearer token' },
    ];

    // Which inputs each type uses: [fieldKey, label, inputType, placeholder]
    static FIELDS = {
        api_key: [
            ['api_key_header', 'Header name', 'text', 'X-API-Key'],
            ['api_key', 'API key', 'password', ''],
        ],
        basic_auth: [
            ['username', 'Username', 'text', ''],
            ['password', 'Password', 'password', ''],
        ],
        bearer_token: [
            ['bearer_token', 'Bearer token', 'password', ''],
        ],
    };

    constructor(rootId = 'editEndpointAuthSection') {
        this.rootId = rootId;
        this.original = null; // endpoint_auth as returned by the API (masked)
    }

    // Markup for the section; inserted by modal-components.js.
    render() {
        const options = EndpointAuthEditor.TYPES
            .map(t => `<option value="${t.value}">${t.label}</option>`).join('');
        return `
            <div class="form-group" id="${this.rootId}" style="margin-top: 16px;">
                <div style="background: linear-gradient(to right, #eff6ff, #f5f3ff); border-left: 3px solid #6366f1; padding: 14px; border-radius: 6px;">
                    <label for="${this.rootId}Type" style="font-weight: 600; color: #3730a3; font-size: 0.95rem; display: block; margin-bottom: 4px;">
                        &#128273; Partner Endpoint Authentication
                    </label>
                    <p style="font-size: 0.82rem; color: #6b7280; margin: 0 0 10px;">
                        Credentials an external system must send to call this interface's real-time endpoints
                        (eligibility, claim status, prior authorization, pharmacy claim). Calls are refused until this is configured.
                    </p>
                    <select id="${this.rootId}Type" data-endpoint-auth-type
                            style="width:100%;padding:7px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;color:#1f2937;background:#fff;">
                        ${options}
                    </select>
                    <div id="${this.rootId}Fields" style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-top:10px;"></div>
                </div>
            </div>`;
    }

    // Wires the type selector; call once after render() markup is in the DOM.
    bind() {
        const select = this._typeSelect();
        if (select && !select.dataset.bound) {
            select.addEventListener('change', () => this._renderFields(select.value, {}));
            select.dataset.bound = 'true';
        }
    }

    // Fills the section from interfaceData.sourceConfig.endpoint_auth.
    populate(endpointAuth) {
        this.bind();
        this.original = endpointAuth && typeof endpointAuth === 'object' ? endpointAuth : null;
        const type = (this.original && this.original.authentication_type) || '';
        const select = this._typeSelect();
        if (select) select.value = type;
        this._renderFields(type, this.original || {});
    }

    // Returns the request value for the `endpointAuth` field:
    //   undefined — section absent from the DOM (send nothing, leave unchanged)
    //   null      — "Not configured" selected (remove)
    //   object    — the configured credentials
    collect() {
        const select = this._typeSelect();
        if (!select) return undefined;
        const type = select.value;
        if (!type) return this.original ? null : undefined;
        const result = { authentication_type: type };
        for (const [key] of EndpointAuthEditor.FIELDS[type] || []) {
            const input = document.getElementById(this._fieldId(key));
            if (input) result[key] = input.value.trim();
        }
        return result;
    }

    // Returns an error message when the selected type is missing a required value.
    validate() {
        const value = this.collect();
        if (!value) return null;
        for (const [key, label] of EndpointAuthEditor.FIELDS[value.authentication_type] || []) {
            if (key === 'api_key_header') continue; // optional, defaults to X-API-Key
            if (!value[key]) return `Partner Endpoint Authentication: ${label} is required.`;
        }
        return null;
    }

    _renderFields(type, values) {
        const container = document.getElementById(`${this.rootId}Fields`);
        if (!container) return;
        const fields = EndpointAuthEditor.FIELDS[type] || [];
        container.innerHTML = fields.map(([key, label, inputType, placeholder]) => `
            <div>
                <label for="${this._fieldId(key)}" style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">${label}</label>
                <input type="${inputType}" id="${this._fieldId(key)}" data-endpoint-auth-field="${key}"
                       autocomplete="new-password" placeholder="${placeholder}"
                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
            </div>`).join('');
        // Values set via .value (never interpolated into HTML).
        for (const [key] of fields) {
            const input = document.getElementById(this._fieldId(key));
            if (input && values[key] != null) input.value = values[key];
        }
    }

    _typeSelect() { return document.getElementById(`${this.rootId}Type`); }
    _fieldId(key) { return `${this.rootId}_${key}`; }
}

window.EndpointAuthEditor = EndpointAuthEditor;
window.editEndpointAuthEditor = new EndpointAuthEditor();
