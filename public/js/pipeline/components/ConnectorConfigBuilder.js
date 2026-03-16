/**
 * ConnectorConfigBuilder - OOP builder for inbound/outbound connector step configuration
 *
 * Extends BaseStepConfigBuilder (Template Method pattern).
 * Single builder for both inbound and outbound connectors - the `direction` parameter
 * controls which connector types are shown.
 *
 * Features:
 * - Fetches connector types from API (data-driven, not hardcoded)
 * - Renders config fields dynamically from connector's config_schema (JSON Schema)
 * - Groups fields by parameter_groups (basic, security, advanced)
 * - Embeds OAuth2ConfigBuilder for connectors requiring auth
 * - Embeds HeaderBuilder for HTTP connectors
 * - Test Connection button
 *
 * Dependencies:
 * - BaseStepConfigBuilder (abstract base class)
 * - ConfigUtils, DOMUtils (utilities)
 * - window.pipelineAPI (PipelineAPIService singleton)
 * - OAuth2ConfigBuilder (optional, embedded for auth)
 * - HeaderBuilder (optional, embedded for HTTP headers)
 *
 * @class ConnectorConfigBuilder
 * @extends BaseStepConfigBuilder
 */
class ConnectorConfigBuilder extends BaseStepConfigBuilder {
    /**
     * @param {HTMLElement} container - DOM container for rendering
     * @param {Object} initialConfig - Initial step configuration
     * @param {string} direction - 'inbound' or 'outbound'
     */
    constructor(container, initialConfig = {}, direction = 'outbound', panel = null) {
        super(container, initialConfig);
        this.direction = direction;
        this._panel = panel;
        this.connectorTypes = [];
        this.selectedType = null;
        this.configSchema = null;
        this.parameterGroups = null;
        this.embeddedBuilders = {};
        this._contentFieldAC = null;
    }

    // ========================================
    // ABSTRACT METHOD IMPLEMENTATIONS
    // ========================================

    getDefaultConfig() {
        return {
            connectorType: '',
            config: {},
            contentField: '',
            contentType: 'application/json',
            timeoutMs: 30000
        };
    }

    // Returns true if the currently selected connector uses MLLP and is inbound
    get isMLLPInbound() {
        return this.direction === 'inbound' &&
            (this.config.connectorType === 'tcp_mllp_inbound' ||
             (this.config.connectorType || '').includes('mllp'));
    }

    render() {
        this.clear();
        this.container.classList.add('connector-config-builder');

        // Header
        const header = this.createElement('div', { class: 'connector-builder-header' }, `
            <div class="connector-builder-title">
                <i class="fas fa-${this.direction === 'inbound' ? 'download' : 'upload'}"></i>
                <h4>${this.direction === 'inbound' ? 'Inbound' : 'Outbound'} Connector Configuration</h4>
            </div>
            <small class="text-muted">Configure the ${this.direction} connector for this step</small>
        `);
        this.container.appendChild(header);

        // Tab bar — inbound: Connection | Acknowledgment; outbound: Connection | Payload
        const tabBar = this.createElement('div', { class: 'connector-tab-bar', id: 'connectorTabBar' });
        if (this.direction === 'inbound') {
            tabBar.innerHTML = `
                <button class="connector-tab active" data-tab="connection">
                    <i class="fas fa-plug"></i> Connection
                </button>
                <button class="connector-tab" data-tab="acknowledgment" id="ackTabBtn" style="display:none">
                    <i class="fas fa-reply"></i> Acknowledgment
                </button>
            `;
        } else {
            tabBar.innerHTML = `
                <button class="connector-tab active" data-tab="connection">
                    <i class="fas fa-plug"></i> Connection
                </button>
                <button class="connector-tab" data-tab="payload">
                    <i class="fas fa-file-export"></i> Payload
                </button>
            `;
        }
        this.container.appendChild(tabBar);

        // Tab panels wrapper
        const panels = this.createElement('div', { class: 'connector-tab-panels' });

        // ── Connection panel ──────────────────────────────────────────────────
        const connPanel = this.createElement('div', {
            class: 'connector-tab-panel',
            id: 'connPanel-connection',
            'data-panel': 'connection'
        });

        // Connector type selector
        const typeSection = this.createElement('div', { class: 'connector-type-section' });
        typeSection.innerHTML = `
            <div class="form-group">
                <label>Connector Type <span class="text-danger">*</span></label>
                <select class="form-control connector-type-select" id="connectorTypeSelect">
                    <option value="">Loading connector types...</option>
                </select>
                <small class="form-text text-muted connector-type-description"></small>
            </div>
        `;
        connPanel.appendChild(typeSection);

        // Dynamic config section
        const configSection = this.createElement('div', {
            class: 'connector-dynamic-config',
            id: 'connectorDynamicConfig'
        });
        connPanel.appendChild(configSection);

        // Direction-specific fields in Connection panel (inbound only - timeout)
        // Outbound payload source / content-type moved to dedicated Payload tab
        if (this.direction === 'inbound') {
            const extraFields = this.createElement('div', { class: 'connector-extra-fields' });
            extraFields.innerHTML = `
                <div class="form-group">
                    <label>Timeout (ms)</label>
                    <input type="number" class="form-control connector-timeout"
                           value="${this.config.timeoutMs || 30000}"
                           min="1000" max="300000" step="1000">
                    <small class="form-text text-muted">Maximum wait time for data fetch</small>
                </div>
            `;
            connPanel.appendChild(extraFields);
        }

        // Test connection button
        const testSection = this.createElement('div', { class: 'connector-test-section' });
        testSection.innerHTML = `
            <button type="button" class="btn btn-outline-info btn-sm test-connection-btn" disabled>
                <i class="fas fa-plug"></i> Test Connection
            </button>
            <span class="test-connection-status"></span>
        `;
        connPanel.appendChild(testSection);
        panels.appendChild(connPanel);

        // ── Acknowledgment panel (inbound only, rendered but hidden until MLLP selected) ──
        if (this.direction === 'inbound') {
            const ackPanel = this.createElement('div', {
                class: 'connector-tab-panel',
                id: 'connPanel-acknowledgment',
                'data-panel': 'acknowledgment',
                style: 'display:none'
            });
            ackPanel.appendChild(this._buildACKPanel());
            panels.appendChild(ackPanel);
        }

        // ── Payload panel (outbound only) ──
        if (this.direction === 'outbound') {
            const payloadPanel = this.createElement('div', {
                class: 'connector-tab-panel',
                id: 'connPanel-payload',
                'data-panel': 'payload',
                style: 'display:none'
            });
            payloadPanel.appendChild(this._buildPayloadPanel());
            panels.appendChild(payloadPanel);
        }

        this.container.appendChild(panels);

        // Load connector types from API
        this.loadConnectorTypes();
    }

    _buildACKPanel() {
        const ack = (this.config.config && this.config.config.ack) || {};
        const mode = ack.mode || 'immediate';
        const onError = ack.on_error || 'suppress';
        const sendingApp = ack.sending_app || '';
        const sendingFacility = ack.sending_facility || '';
        const textSuccess = ack.text_success || '';
        const textError = ack.text_error || '';
        const script = ack.script || '';

        const panel = this.createElement('div', { class: 'ack-config-panel' });
        panel.innerHTML = `
            <p class="text-muted" style="font-size:12px; margin-bottom:12px;">
                Configure how this connector acknowledges received HL7 messages.
                The script option (advanced) gives full control over the ACK content.
            </p>

            <div class="connector-config-group">
                <div class="connector-config-group-header">
                    <span class="connector-config-group-title"><i class="fas fa-cog"></i> Basic</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                </div>
                <div class="connector-config-group-body">
                    <div class="form-group">
                        <label>ACK Mode</label>
                        <select class="form-control form-control-sm ack-field" data-ack-field="mode">
                            <option value="immediate" selected>Immediate — AA sent as soon as message is accepted</option>
                        </select>
                        <small class="form-text text-muted">MLLP protocol requires every received message to get an acknowledgment — this cannot be disabled</small>
                    </div>
                    <div class="form-group">
                        <label>On Error</label>
                        <select class="form-control form-control-sm ack-field" data-ack-field="on_error">
                            <option value="suppress" ${onError === 'suppress' ? 'selected' : ''}>Suppress — Sender always gets AA (handle errors internally)</option>
                            <option value="nack" ${onError === 'nack' ? 'selected' : ''}>NACK — Send AE so sender can retry</option>
                        </select>
                        <small class="form-text text-muted">Applies when the message queue is full or a critical error occurs</small>
                    </div>
                </div>
            </div>

            <div class="connector-config-group collapsed">
                <div class="connector-config-group-header">
                    <span class="connector-config-group-title"><i class="fas fa-id-card"></i> Sender Identity</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                </div>
                <div class="connector-config-group-body">
                    <div class="form-group">
                        <label>Sending Application (MSH-3)</label>
                        <input type="text" class="form-control form-control-sm ack-field" data-ack-field="sending_app"
                               value="${this.escapeHtml(sendingApp)}" placeholder="Default: ezHealthKonnect">
                    </div>
                    <div class="form-group">
                        <label>Sending Facility (MSH-4)</label>
                        <input type="text" class="form-control form-control-sm ack-field" data-ack-field="sending_facility"
                               value="${this.escapeHtml(sendingFacility)}" placeholder="Default: EHK">
                    </div>
                </div>
            </div>

            <div class="connector-config-group collapsed">
                <div class="connector-config-group-header">
                    <span class="connector-config-group-title"><i class="fas fa-comment-alt"></i> Message Text</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                </div>
                <div class="connector-config-group-body">
                    <div class="form-group">
                        <label>Success Text (MSA-3)</label>
                        <input type="text" class="form-control form-control-sm ack-field" data-ack-field="text_success"
                               value="${this.escapeHtml(textSuccess)}" placeholder="Default: Message received successfully">
                    </div>
                    <div class="form-group">
                        <label>Error Text (MSA-3 on NACK)</label>
                        <input type="text" class="form-control form-control-sm ack-field" data-ack-field="text_error"
                               value="${this.escapeHtml(textError)}" placeholder="Default: Message processing error">
                    </div>
                </div>
            </div>

            <div class="connector-config-group collapsed">
                <div class="connector-config-group-header">
                    <span class="connector-config-group-title"><i class="fas fa-code"></i> Custom Script (Advanced)</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                </div>
                <div class="connector-config-group-body">
                    <div class="ack-script-info">
                        <div class="ack-script-info-title"><i class="fas fa-info-circle"></i> Script contract</div>
                        <ul class="ack-script-info-list">
                            <li>Function name must be <code>buildACK(msg)</code></li>
                            <li>Return <code>{ ackCode, textMessage }</code> — valid codes: <code>AA</code> · <code>AE</code> · <code>AR</code></li>
                            <li><code>msg</code> properties: <code>controlID</code>, <code>messageType</code>, <code>sendingApp</code>, <code>sendingFacility</code>, <code>raw</code>, <code>defaultCode</code>, <code>defaultText</code></li>
                            <li>Errors or missing return values fall back to the configured default</li>
                        </ul>
                    </div>
                    <textarea class="form-control ack-field ack-script-editor" data-ack-field="script"
                              rows="10" spellcheck="false"
                              placeholder="function buildACK(msg) {&#10;  if (msg.messageType !== 'ADT^A01') {&#10;    return { ackCode: 'AR', textMessage: 'Unsupported message type' };&#10;  }&#10;  return { ackCode: 'AA', textMessage: 'Accepted' };&#10;}">${this.escapeHtml(script)}</textarea>
                </div>
            </div>
        `;

        // Wire collapse toggles
        panel.querySelectorAll('.connector-config-group-header').forEach(h => {
            h.addEventListener('click', () => h.closest('.connector-config-group').classList.toggle('collapsed'));
        });

        // Wire change events
        panel.querySelectorAll('.ack-field').forEach(el => {
            el.addEventListener('input', () => this.onChange());
            el.addEventListener('change', () => this.onChange());
        });

        return panel;
    }

    _buildPayloadPanel() {
        const contentField = this.config.contentField || '';
        const contentType = this.config.contentType || 'application/json';

        const panel = this.createElement('div', { class: 'payload-config-panel' });
        panel.innerHTML = `
            <div class="form-group">
                <label style="font-weight:600; font-size:13px;">Payload Source</label>
                <input type="text" class="form-control connector-content-field"
                       value="${this.escapeHtml(contentField)}"
                       placeholder="Search pipeline variables or type a path..."
                       autocomplete="off" spellcheck="false">
                <div class="connector-payload-quickpick">
                    <button type="button" class="connector-quickpick-btn" data-value="fhirBundle"
                            title="FHIR Bundle from HL7→FHIR transform">
                        <i class="fas fa-fire-alt"></i> FHIR Bundle
                    </button>
                    <button type="button" class="connector-quickpick-btn" data-value="raw"
                            title="Original HL7 v2 wire-format string">
                        <i class="fas fa-code"></i> HL7 Raw
                    </button>
                    <button type="button" class="connector-quickpick-btn" data-value="message"
                            title="Full parsed message object (JSON)">
                        <i class="fas fa-envelope-open-text"></i> Full Message
                    </button>
                    <button type="button" class="connector-quickpick-btn" data-value="payload"
                            title="Output from a Payload Builder step">
                        <i class="fas fa-file-export"></i> Payload Builder
                    </button>
                </div>
                <small class="form-text text-muted">
                    Start typing to search pipeline variables from the last test run.
                    Quick-picks cover the most common sources.
                </small>
            </div>
            <div class="form-group">
                <label style="font-weight:600; font-size:13px;">Content Type</label>
                <select class="form-control connector-content-type">
                    <option value="application/json"     ${contentType === 'application/json'     ? 'selected' : ''}>application/json</option>
                    <option value="application/fhir+json" ${contentType === 'application/fhir+json' ? 'selected' : ''}>application/fhir+json (FHIR R4)</option>
                    <option value="application/hl7-v2"   ${contentType === 'application/hl7-v2'   ? 'selected' : ''}>application/hl7-v2 (HL7 v2)</option>
                    <option value="text/plain"            ${contentType === 'text/plain'            ? 'selected' : ''}>text/plain</option>
                    <option value="text/csv"              ${contentType === 'text/csv'              ? 'selected' : ''}>text/csv</option>
                    <option value="application/xml"       ${contentType === 'application/xml'       ? 'selected' : ''}>application/xml (CDA / C-CDA)</option>
                </select>
                <small class="form-text text-muted">MIME type sent with the payload to the destination system</small>
            </div>
        `;

        // Wire quick-pick buttons
        panel.querySelectorAll('.connector-quickpick-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const val = btn.dataset.value;
                const input = panel.querySelector('.connector-content-field');
                if (input) {
                    input.value = val;
                    input.dispatchEvent(new Event('input', { bubbles: true }));
                    this.onChange();
                }
            });
        });

        // Wire change events
        panel.querySelector('.connector-content-type')?.addEventListener('change', () => this.onChange());

        return panel;
    }

    attachEvents() {
        // Tab switching
        this.container.querySelectorAll('.connector-tab').forEach(tab => {
            tab.addEventListener('click', () => {
                const target = tab.dataset.tab;
                this.container.querySelectorAll('.connector-tab').forEach(t => t.classList.remove('active'));
                tab.classList.add('active');
                this.container.querySelectorAll('.connector-tab-panel').forEach(p => {
                    p.style.display = p.dataset.panel === target ? '' : 'none';
                });
            });
        });

        // Connector type dropdown change
        const typeSelect = this.container.querySelector('.connector-type-select');
        if (typeSelect) {
            typeSelect.addEventListener('change', (e) => {
                this.onConnectorTypeChange(e.target.value);
                this.onChange();
            });
        }

        // Payload source field (outbound only) — attach variable autocomplete
        const contentField = this.container.querySelector('.connector-content-field');
        if (contentField) {
            contentField.addEventListener('input', () => this.onChange());
            if (this.direction === 'outbound' && window.FieldPathSearchComponent) {
                try {
                    const commonVars = [
                        { name: 'fhirBundle',    path: 'fhirBundle',    description: 'FHIR Bundle from HL7→FHIR transform', category: 'Pipeline Variables' },
                        { name: 'raw',           path: 'raw',           description: 'Original HL7 v2 wire-format string',  category: 'Pipeline Variables' },
                        { name: 'message',       path: 'message',       description: 'Full parsed message object (JSON)',    category: 'Pipeline Variables' },
                        { name: 'payload',       path: 'payload',       description: 'Output from a Payload Builder step',  category: 'Pipeline Variables' },
                        { name: 'fhir_bundle',   path: 'fhir_bundle',   description: 'FHIR Bundle (snake_case key)',         category: 'Pipeline Variables' },
                        { name: 'transformed',   path: 'transformed',   description: 'Output of the last transform step',   category: 'Pipeline Variables' },
                    ];
                    this._contentFieldAC = new FieldPathSearchComponent(contentField, {
                        placeholder: 'Search pipeline variables...',
                        allowCustom: true,
                        includeHL7Fields: false,
                        maxSuggestions: 20,
                        additionalFields: commonVars,
                        getStepVariables: () => {
                            if (this._panel && typeof this._panel.getStepVariablesForSearch === 'function') {
                                return this._panel.getStepVariablesForSearch();
                            }
                            return [];
                        },
                    });
                } catch (e) {
                    console.warn('[ConnectorConfigBuilder] FieldPathSearchComponent init failed:', e);
                }
            }
        }

        const contentType = this.container.querySelector('.connector-content-type');
        if (contentType) {
            contentType.addEventListener('change', () => this.onChange());
        }

        const timeoutField = this.container.querySelector('.connector-timeout');
        if (timeoutField) {
            timeoutField.addEventListener('input', () => this.onChange());
        }

        // Test connection button
        const testBtn = this.container.querySelector('.test-connection-btn');
        if (testBtn) {
            testBtn.addEventListener('click', () => this.testConnection());
        }
    }

    // ========================================
    // HOOK METHODS
    // ========================================

    addStyles() {
        if (document.getElementById('connector-config-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'connector-config-builder-styles';
        style.textContent = `
            .connector-config-builder {
                padding: 0;
            }

            .connector-builder-header {
                margin-bottom: 16px;
            }

            .connector-builder-title {
                display: flex;
                align-items: center;
                gap: 8px;
                margin-bottom: 4px;
            }

            .connector-builder-title h4 {
                margin: 0;
                font-size: 15px;
                font-weight: 600;
                color: #1e293b;
            }

            .connector-builder-title i {
                color: #6366f1;
                font-size: 16px;
            }

            .connector-type-section {
                margin-bottom: 16px;
            }

            .connector-type-section .connector-type-select {
                font-weight: 500;
            }

            .connector-type-description {
                margin-top: 4px;
                font-style: italic;
            }

            .connector-dynamic-config {
                margin-bottom: 16px;
            }

            .connector-config-group {
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                margin-bottom: 12px;
                overflow: hidden;
            }

            .connector-config-group-header {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 10px 14px;
                background: #f1f5f9;
                cursor: pointer;
                user-select: none;
                border-bottom: 1px solid #e2e8f0;
            }

            .connector-config-group-header:hover {
                background: #e2e8f0;
            }

            .connector-config-group-title {
                font-weight: 600;
                font-size: 13px;
                color: #334155;
                text-transform: capitalize;
            }

            .connector-config-group-toggle {
                color: #94a3b8;
                font-size: 12px;
                transition: transform 0.2s;
            }

            .connector-config-group.collapsed .connector-config-group-toggle {
                transform: rotate(-90deg);
            }

            .connector-config-group-body {
                padding: 12px 14px;
            }

            .connector-config-group.collapsed .connector-config-group-body {
                display: none;
            }

            .connector-config-group .form-group {
                margin-bottom: 12px;
            }

            .connector-config-group .form-group:last-child {
                margin-bottom: 0;
            }

            .connector-config-group .form-group label {
                font-weight: 500;
                font-size: 13px;
                color: #475569;
                margin-bottom: 4px;
                display: block;
            }

            .connector-extra-fields {
                margin-bottom: 16px;
                padding: 12px 14px;
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
            }

            .connector-extra-fields .form-group {
                margin-bottom: 12px;
            }

            .connector-extra-fields .form-group:last-child {
                margin-bottom: 0;
            }

            .connector-test-section {
                display: flex;
                align-items: center;
                gap: 12px;
                padding-top: 8px;
            }

            .test-connection-btn {
                font-size: 13px;
            }

            .test-connection-status {
                font-size: 13px;
            }

            .connector-no-config {
                color: #94a3b8;
                font-style: italic;
                padding: 12px;
                text-align: center;
            }

            .connector-loading {
                display: flex;
                align-items: center;
                gap: 8px;
                padding: 12px;
                color: #64748b;
                font-size: 13px;
            }

            .connector-embedded-builder {
                margin-top: 12px;
                border: 1px solid #c7d2fe;
                border-radius: 8px;
                padding: 12px;
                background: #eef2ff;
            }

            /* Tab bar */
            .connector-tab-bar {
                display: flex;
                gap: 4px;
                border-bottom: 2px solid #e2e8f0;
                margin-bottom: 16px;
            }

            .connector-tab {
                background: none;
                border: none;
                border-bottom: 2px solid transparent;
                margin-bottom: -2px;
                padding: 7px 14px;
                font-size: 13px;
                font-weight: 500;
                color: #64748b;
                cursor: pointer;
                border-radius: 4px 4px 0 0;
                display: flex;
                align-items: center;
                gap: 5px;
                transition: color 0.15s, border-color 0.15s;
            }

            .connector-tab:hover {
                color: #334155;
                background: #f1f5f9;
            }

            .connector-tab.active {
                color: #6366f1;
                border-bottom-color: #6366f1;
                background: none;
            }

            .connector-tab-panels {
                /* no extra styling needed */
            }

            /* ACK panel */
            .ack-config-panel {
                padding: 0;
            }

            /* Script info box */
            .ack-script-info {
                background: #f0f4ff;
                border: 1px solid #c7d2fe;
                border-radius: 6px;
                padding: 10px 14px;
                margin-bottom: 10px;
                font-size: 12px;
            }
            .ack-script-info-title {
                font-weight: 600;
                color: #4338ca;
                margin-bottom: 6px;
            }
            .ack-script-info-list {
                margin: 0;
                padding-left: 18px;
                color: #374151;
                line-height: 1.7;
            }
            .ack-script-info-list code {
                background: #e0e7ff;
                color: #3730a3;
                padding: 1px 4px;
                border-radius: 3px;
                font-size: 11px;
            }

            /* Code editor textarea */
            .ack-script-editor {
                width: 100%;
                box-sizing: border-box;
                display: block;
                font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                font-size: 12px;
                line-height: 1.5;
                background: #1e1e2e;
                color: #cdd6f4;
                border: 1px solid #45475a;
                border-radius: 6px;
                padding: 10px 12px;
                resize: vertical;
                min-height: 180px;
                tab-size: 2;
                white-space: pre;
                overflow-wrap: normal;
                overflow-x: auto;
            }
            .ack-script-editor::placeholder {
                color: #585b70;
            }
            .ack-script-editor:focus {
                outline: none;
                border-color: #6366f1;
                box-shadow: 0 0 0 2px rgba(99,102,241,0.2);
            }

            /* Payload panel */
            .payload-config-panel {
                padding: 0;
            }
            .payload-config-panel .form-group {
                margin-bottom: 16px;
            }

            /* Quick-pick chips below the payload source input */
            .connector-payload-quickpick {
                display: flex;
                flex-wrap: wrap;
                gap: 6px;
                margin-top: 8px;
            }

            .connector-quickpick-btn {
                display: inline-flex;
                align-items: center;
                gap: 4px;
                padding: 3px 10px;
                font-size: 12px;
                font-weight: 500;
                color: #4f46e5;
                background: #eef2ff;
                border: 1px solid #c7d2fe;
                border-radius: 20px;
                cursor: pointer;
                transition: background 0.15s, border-color 0.15s, color 0.15s;
                white-space: nowrap;
            }

            .connector-quickpick-btn:hover {
                background: #e0e7ff;
                border-color: #a5b4fc;
                color: #3730a3;
            }

            .connector-quickpick-btn i {
                font-size: 11px;
            }
        `;
        document.head.appendChild(style);
    }

    // ========================================
    // CONNECTOR TYPE MANAGEMENT
    // ========================================

    async loadConnectorTypes() {
        const typeSelect = this.container.querySelector('.connector-type-select');
        if (!typeSelect) return;

        try {
            const response = await window.pipelineAPI.getConnectorTypes(this.direction);
            const types = response.data || response || [];
            this.connectorTypes = types;

            // Type name aliases: DB may store 'tcp_mllp' while step config stores 'tcp_mllp_inbound'
            const TYPE_ALIASES = {
                'tcp_mllp_inbound': 'tcp_mllp',   'tcp_mllp': 'tcp_mllp_inbound',
                'http_rest_inbound': 'http_rest',  'http_rest': 'http_rest_inbound'
            };
            const storedType = this.config.connectorType || '';

            // Build dropdown options
            typeSelect.innerHTML = '<option value="">-- Select Connector Type --</option>';
            let matchedTypeName = '';
            types.forEach(ct => {
                const option = document.createElement('option');
                option.value = ct.type_name;
                option.textContent = `${ct.icon || ''} ${ct.display_name}`;
                if (ct.is_beta) option.textContent += ' (Beta)';
                const isMatch = ct.type_name === storedType || ct.type_name === TYPE_ALIASES[storedType];
                if (isMatch) {
                    option.selected = true;
                    matchedTypeName = ct.type_name;
                }
                typeSelect.appendChild(option);
            });

            // If we have a pre-selected connector type, load its config
            if (storedType) {
                this.onConnectorTypeChange(storedType);
                // Ensure dropdown reflects the DB type name (the option's value)
                if (matchedTypeName && matchedTypeName !== storedType) {
                    typeSelect.value = matchedTypeName;
                }
            }

            // Enable test button if type is selected
            this.updateTestButton();

            console.log(`[ConnectorConfigBuilder] Loaded ${types.length} ${this.direction} connector types`);
        } catch (error) {
            console.error('[ConnectorConfigBuilder] Failed to load connector types:', error);
            typeSelect.innerHTML = '<option value="">Failed to load connector types</option>';
        }
    }

    onConnectorTypeChange(typeName) {
        this.config.connectorType = typeName;

        // Update ACK tab visibility immediately — isMLLPInbound is pure (no API dependency)
        const ackTabBtn = this.container.querySelector('#ackTabBtn');
        if (ackTabBtn) {
            ackTabBtn.style.display = this.isMLLPInbound ? '' : 'none';
        }

        const configContainer = this.container.querySelector('#connectorDynamicConfig');
        if (!configContainer) return;

        // Destroy embedded builders
        Object.values(this.embeddedBuilders).forEach(b => {
            if (b && typeof b.destroy === 'function') b.destroy();
        });
        this.embeddedBuilders = {};

        if (!typeName) {
            configContainer.innerHTML = '<div class="connector-no-config">Select a connector type to configure</div>';
            this.selectedType = null;
            this.configSchema = null;
            this.parameterGroups = null;
            this.updateTestButton();
            this.updateTypeDescription('');
            return;
        }

        // Find the selected type data — try exact match first, then aliases
        // (DB stores 'tcp_mllp' but wizard may write 'tcp_mllp_inbound'; engine accepts both)
        const TYPE_ALIASES = {
            'tcp_mllp_inbound': 'tcp_mllp',
            'tcp_mllp': 'tcp_mllp_inbound',
            'http_rest_inbound': 'http_rest',
            'http_rest': 'http_rest_inbound'
        };
        this.selectedType = this.connectorTypes.find(ct => ct.type_name === typeName)
            || this.connectorTypes.find(ct => ct.type_name === TYPE_ALIASES[typeName]);
        if (!this.selectedType) {
            configContainer.innerHTML = '<div class="connector-no-config">Connector type not found</div>';
            this.updateTestButton();
            return;
        }

        this.configSchema = this.parseJSON(this.selectedType.config_schema);
        this.parameterGroups = this.parseJSON(this.selectedType.parameter_groups);

        this.updateTypeDescription(this.selectedType.description || '');
        this.renderDynamicConfig(configContainer);
        this.updateTestButton();

        // Set smart default contentField when unset (outbound only)
        if (this.direction === 'outbound') {
            const contentFieldInput = this.container.querySelector('.connector-content-field');
            if (contentFieldInput && !contentFieldInput.value) {
                const smartDefault = this._getSmartContentFieldDefault(typeName);
                if (smartDefault) {
                    contentFieldInput.value = smartDefault;
                    this.config.contentField = smartDefault;
                }
            }
        }
    }

    /** Returns a sensible default payload source path for the given outbound connector type. */
    _getSmartContentFieldDefault(connectorType) {
        const name = (connectorType || '').toLowerCase();
        if (name.includes('mllp') || name.includes('tcp')) return 'raw';
        if (name.includes('http')) return 'fhirBundle';
        // File, MQ, cloud storage, DB — send the whole message object
        return 'message';
    }

    // ========================================
    // DYNAMIC CONFIG RENDERING (Schema-Driven)
    // ========================================

    renderDynamicConfig(container) {
        container.innerHTML = '';

        if (!this.configSchema || !this.configSchema.properties) {
            container.innerHTML = '<div class="connector-no-config">No configuration required for this connector</div>';
            return;
        }

        const properties = this.configSchema.properties;
        const required = this.configSchema.required || [];
        const existingConfig = this.config.config || {};

        // Build dependency map: boolean toggle fields control related fields' visibility
        // e.g., enable_tls → [tls_cert_path, tls_key_path], enable_authentication → [authentication_method]
        this.fieldDependencies = this.buildFieldDependencies(properties);

        // If parameter_groups is defined, render grouped
        if (this.parameterGroups && Object.keys(this.parameterGroups).length > 0) {
            const renderedFields = new Set();

            for (const [groupName, fieldNames] of Object.entries(this.parameterGroups)) {
                if (!Array.isArray(fieldNames) || fieldNames.length === 0) continue;

                const group = this.createElement('div', { class: 'connector-config-group' });

                // Group header (collapsible)
                const groupHeader = this.createElement('div', { class: 'connector-config-group-header' }, `
                    <span class="connector-config-group-title">${this.formatGroupName(groupName)}</span>
                    <i class="fas fa-chevron-down connector-config-group-toggle"></i>
                `);
                group.appendChild(groupHeader);

                // Group body
                const groupBody = this.createElement('div', { class: 'connector-config-group-body' });

                fieldNames.forEach(fieldName => {
                    if (properties[fieldName]) {
                        const fieldEl = this.renderSchemaField(fieldName, properties[fieldName], required.includes(fieldName), existingConfig[fieldName]);
                        // Mark dependent fields as hidden initially if their toggle is off
                        if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                            fieldEl.style.display = 'none';
                            fieldEl.dataset.dependentField = 'true';
                        }
                        groupBody.appendChild(fieldEl);
                        renderedFields.add(fieldName);
                    }
                });

                group.appendChild(groupBody);
                container.appendChild(group);

                // Toggle collapse
                groupHeader.addEventListener('click', () => {
                    group.classList.toggle('collapsed');
                });

                // Collapse non-basic groups by default if no values set
                if (groupName !== 'basic') {
                    const hasValues = fieldNames.some(fn => existingConfig[fn] !== undefined && existingConfig[fn] !== '' && existingConfig[fn] !== false);
                    if (!hasValues) {
                        group.classList.add('collapsed');
                    }
                }
            }

            // Render any ungrouped fields
            for (const [fieldName, schema] of Object.entries(properties)) {
                if (!renderedFields.has(fieldName)) {
                    const fieldEl = this.renderSchemaField(fieldName, schema, required.includes(fieldName), existingConfig[fieldName]);
                    if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                        fieldEl.style.display = 'none';
                        fieldEl.dataset.dependentField = 'true';
                    }
                    container.appendChild(fieldEl);
                }
            }
        } else {
            // No groups - render all fields flat
            for (const [fieldName, schema] of Object.entries(properties)) {
                const fieldEl = this.renderSchemaField(fieldName, schema, required.includes(fieldName), existingConfig[fieldName]);
                if (this.isDependentField(fieldName) && !this.isToggleEnabled(fieldName, existingConfig)) {
                    fieldEl.style.display = 'none';
                    fieldEl.dataset.dependentField = 'true';
                }
                container.appendChild(fieldEl);
            }
        }

        // Attach toggle listeners for conditional visibility
        this.attachToggleListeners(container);

        // Add embedded builders for special types (conditionally)
        this.addEmbeddedBuilders(container, existingConfig);
    }

    // ========================================
    // CONDITIONAL FIELD VISIBILITY
    // ========================================

    /**
     * Build a map of toggle fields to their dependent fields.
     * Convention: boolean fields named `enable_*` control related fields in the same group.
     * E.g., enable_tls → [tls_cert_path, tls_key_path], enable_authentication → [authentication_method]
     */
    buildFieldDependencies(properties) {
        const deps = {};
        const toggleFields = Object.keys(properties).filter(name =>
            properties[name].type === 'boolean' && name.startsWith('enable_')
        );

        for (const toggle of toggleFields) {
            // Extract the feature name: enable_tls → tls, enable_authentication → authentication
            const feature = toggle.replace('enable_', '');
            // Find fields whose names contain this feature (but aren't the toggle itself)
            const dependents = Object.keys(properties).filter(name =>
                name !== toggle && (name.includes(feature) || name.startsWith(feature + '_'))
            );
            if (dependents.length > 0) {
                deps[toggle] = dependents;
            }
        }

        return deps;
    }

    isDependentField(fieldName) {
        if (!this.fieldDependencies) return false;
        return Object.values(this.fieldDependencies).some(deps => deps.includes(fieldName));
    }

    isToggleEnabled(fieldName, config) {
        if (!this.fieldDependencies) return true;
        for (const [toggle, deps] of Object.entries(this.fieldDependencies)) {
            if (deps.includes(fieldName)) {
                return config[toggle] === true || config[toggle] === 'true';
            }
        }
        return true;
    }

    attachToggleListeners(container) {
        if (!this.fieldDependencies) return;

        for (const [toggleField, dependentFields] of Object.entries(this.fieldDependencies)) {
            const toggleInput = container.querySelector(`.connector-config-field[data-field="${toggleField}"]`);
            if (!toggleInput) continue;

            toggleInput.addEventListener('change', () => {
                const isEnabled = toggleInput.checked;
                dependentFields.forEach(depField => {
                    const depEl = container.querySelector(`.connector-config-field[data-field="${depField}"]`);
                    if (depEl) {
                        const formGroup = depEl.closest('.form-group') || depEl.closest('.form-check')?.parentElement;
                        if (formGroup) {
                            formGroup.style.display = isEnabled ? '' : 'none';
                        }
                    }
                });
            });
        }
    }

    renderSchemaField(fieldName, schema, isRequired, currentValue) {
        const formGroup = this.createElement('div', { class: 'form-group' });
        const title = schema.title || this.formatFieldName(fieldName);
        const value = currentValue !== undefined ? currentValue : (schema.default !== undefined ? schema.default : '');

        // Label
        const label = document.createElement('label');
        label.innerHTML = `${this.escapeHtml(title)}${isRequired ? ' <span class="text-danger">*</span>' : ''}`;
        formGroup.appendChild(label);

        let inputEl;

        if (schema.enum) {
            // Select dropdown
            inputEl = document.createElement('select');
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;

            const emptyOpt = document.createElement('option');
            emptyOpt.value = '';
            emptyOpt.textContent = `-- Select ${title} --`;
            inputEl.appendChild(emptyOpt);

            schema.enum.forEach(enumVal => {
                const opt = document.createElement('option');
                opt.value = enumVal;
                opt.textContent = enumVal;
                if (String(value) === String(enumVal)) opt.selected = true;
                inputEl.appendChild(opt);
            });
        } else if (schema.type === 'boolean') {
            // Checkbox wrapper
            const checkWrapper = this.createElement('div', { class: 'form-check' });
            inputEl = document.createElement('input');
            inputEl.type = 'checkbox';
            inputEl.className = 'form-check-input connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'boolean';
            inputEl.checked = value === true || value === 'true';
            inputEl.id = `connector-field-${fieldName}`;

            const checkLabel = document.createElement('label');
            checkLabel.className = 'form-check-label';
            checkLabel.htmlFor = inputEl.id;
            checkLabel.textContent = title;

            checkWrapper.appendChild(inputEl);
            checkWrapper.appendChild(checkLabel);

            // For boolean, replace the existing label with the checkbox wrapper
            formGroup.innerHTML = '';
            formGroup.appendChild(checkWrapper);
            inputEl.addEventListener('change', () => this.onChange());
            return formGroup;
        } else if (schema.type === 'integer' || schema.type === 'number') {
            inputEl = document.createElement('input');
            inputEl.type = 'number';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'number';
            inputEl.value = value;
            if (schema.minimum !== undefined) inputEl.min = schema.minimum;
            if (schema.maximum !== undefined) inputEl.max = schema.maximum;
            inputEl.placeholder = schema.default !== undefined ? `Default: ${schema.default}` : '';
        } else if (schema.type === 'array') {
            // Render as comma-separated text input
            inputEl = document.createElement('input');
            inputEl.type = 'text';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.dataset.fieldType = 'array';
            inputEl.value = Array.isArray(value) ? value.join(', ') : (value || '');
            inputEl.placeholder = schema.items?.enum
                ? `Options: ${schema.items.enum.join(', ')}`
                : 'Comma-separated values';
        } else {
            // Default: text input (or password for sensitive fields)
            inputEl = document.createElement('input');
            const isSensitive = schema.format === 'password' ||
                                fieldName.toLowerCase().includes('password') ||
                                fieldName.toLowerCase().includes('secret') ||
                                (fieldName.toLowerCase().includes('key') && !fieldName.toLowerCase().includes('key_path'));
            inputEl.type = isSensitive ? 'password' : 'text';
            inputEl.className = 'form-control form-control-sm connector-config-field';
            inputEl.dataset.field = fieldName;
            inputEl.value = value;
            inputEl.placeholder = schema.default !== undefined ? `Default: ${schema.default}` : '';
        }

        if (inputEl) {
            formGroup.appendChild(inputEl);
            inputEl.addEventListener('input', () => this.onChange());
            inputEl.addEventListener('change', () => this.onChange());
        }

        return formGroup;
    }

    addEmbeddedBuilders(container, existingConfig = {}) {
        if (!this.selectedType) return;

        const typeName = this.selectedType.type_name || '';
        const isHTTP = typeName.includes('http');
        const authType = existingConfig.authentication_type || '';

        // Credential fields: show/hide based on authentication_type selection
        if (isHTTP) {
            const authTypeField = container.querySelector('.connector-config-field[data-field="authentication_type"]');
            if (authTypeField) {
                const applyAuthVisibility = (type) => {
                    const show = (field, visible) => {
                        const input = container.querySelector(`.connector-config-field[data-field="${field}"]`);
                        const el = input?.closest('.form-group');
                        if (el) el.style.display = visible ? '' : 'none';
                    };
                    show('username',      type === 'basic_auth');
                    show('password',      type === 'basic_auth');
                    show('bearer_token',  type === 'bearer_token');
                    show('api_key',       type === 'api_key');
                    show('api_key_header', type === 'api_key');
                };
                // Apply on load
                applyAuthVisibility(authType);
                // Apply on change
                authTypeField.addEventListener('change', () => applyAuthVisibility(authTypeField.value));
            }
        }

        // OAuth2 only for HTTP connectors when authentication_type is 'oauth2'
        if (isHTTP && typeof OAuth2ConfigBuilder !== 'undefined') {
            const authSection = this.createElement('div', {
                class: 'connector-embedded-builder connector-oauth2-section',
                id: 'connectorOAuth2Section'
            });
            authSection.innerHTML = '<h5 style="margin: 0 0 8px; font-size: 13px; font-weight: 600;">Authentication</h5>';
            const authContainer = this.createElement('div', {});
            authSection.appendChild(authContainer);
            container.appendChild(authSection);

            // Only show if authentication_type is oauth2
            if (authType !== 'oauth2') {
                authSection.style.display = 'none';
            }

            const authBuilder = new OAuth2ConfigBuilder(authContainer, existingConfig.oauth2 || {});
            this.embeddedBuilders.oauth2 = authBuilder;

            // Listen for authentication_type changes to show/hide OAuth2
            const authTypeField2 = container.querySelector('.connector-config-field[data-field="authentication_type"]');
            if (authTypeField2) {
                authTypeField2.addEventListener('change', () => {
                    authSection.style.display = authTypeField2.value === 'oauth2' ? '' : 'none';
                });
            }
        }

        // Headers only for HTTP connectors
        if (isHTTP && typeof HeaderBuilder !== 'undefined') {
            const headersSection = this.createElement('div', { class: 'connector-embedded-builder' });
            headersSection.innerHTML = '<h5 style="margin: 0 0 8px; font-size: 13px; font-weight: 600;">Custom Headers</h5>';
            const headersContainer = this.createElement('div', {});
            headersSection.appendChild(headersContainer);
            container.appendChild(headersSection);

            const headerBuilder = new HeaderBuilder(headersContainer, existingConfig.headers || {});
            this.embeddedBuilders.headers = headerBuilder;
        }
    }

    // ========================================
    // PUBLIC API OVERRIDES
    // ========================================

    getConfig() {
        // Gather config from dynamic fields
        const config = {};
        const fields = this.container.querySelectorAll('.connector-config-field');
        fields.forEach(field => {
            const fieldName = field.dataset.field;
            if (!fieldName) return;

            let value;
            if (field.dataset.fieldType === 'boolean') {
                value = field.checked;
            } else if (field.dataset.fieldType === 'number') {
                value = field.value ? Number(field.value) : undefined;
            } else if (field.dataset.fieldType === 'array') {
                value = field.value ? field.value.split(',').map(v => v.trim()).filter(v => v) : [];
            } else {
                value = field.value.trim();
            }

            if (value !== undefined && value !== '') {
                config[fieldName] = value;
            }
        });

        // Merge embedded builder configs
        if (this.embeddedBuilders.oauth2) {
            config.oauth2 = this.embeddedBuilders.oauth2.getConfig();
        }
        if (this.embeddedBuilders.headers) {
            const headerConfig = this.embeddedBuilders.headers.getConfig
                ? this.embeddedBuilders.headers.getConfig()
                : this.embeddedBuilders.headers.getHeaders
                    ? this.embeddedBuilders.headers.getHeaders()
                    : {};
            config.headers = headerConfig;
        }

        // Collect ACK config from acknowledgment tab (inbound MLLP only)
        const ackFields = this.container.querySelectorAll('.ack-field');
        if (ackFields.length > 0) {
            const ackConfig = {};
            ackFields.forEach(el => {
                const key = el.dataset.ackField;
                if (key) ackConfig[key] = el.value.trim();
            });
            // Only attach if at least one field is non-default
            const hasACKConfig = Object.values(ackConfig).some(v => v !== '');
            if (hasACKConfig) {
                config.ack = ackConfig;
            }
        }

        // Build final step config
        const stepConfig = {
            connectorType: this.config.connectorType || '',
            config: config
        };

        // Direction-specific fields
        if (this.direction === 'outbound') {
            const contentField = this.container.querySelector('.connector-content-field');
            const contentType = this.container.querySelector('.connector-content-type');
            stepConfig.contentField = contentField ? contentField.value.trim() : '';
            stepConfig.contentType = contentType ? contentType.value : 'application/json';
        } else {
            const timeoutField = this.container.querySelector('.connector-timeout');
            stepConfig.timeoutMs = timeoutField ? Number(timeoutField.value) || 30000 : 30000;
        }

        this.config = ConfigUtils.mergeConfig(this.config, stepConfig);
        return this.config;
    }

    validate() {
        const errors = [];

        if (!this.config.connectorType) {
            errors.push('Connector type is required');
        }

        // Check required fields from schema
        if (this.configSchema && this.configSchema.required) {
            const currentConfig = this.config.config || {};
            this.configSchema.required.forEach(field => {
                if (!currentConfig[field] && currentConfig[field] !== 0 && currentConfig[field] !== false) {
                    const title = this.configSchema.properties?.[field]?.title || field;
                    errors.push(`${title} is required`);
                }
            });
        }

        return {
            valid: errors.length === 0,
            errors: errors
        };
    }

    destroy() {
        // Destroy payload source autocomplete
        if (this._contentFieldAC && typeof this._contentFieldAC.destroy === 'function') {
            this._contentFieldAC.destroy();
            this._contentFieldAC = null;
        }
        // Destroy embedded builders first
        Object.values(this.embeddedBuilders).forEach(b => {
            if (b && typeof b.destroy === 'function') b.destroy();
        });
        this.embeddedBuilders = {};
        super.destroy();
    }

    // ========================================
    // TEST CONNECTION
    // ========================================

    async testConnection() {
        const testBtn = this.container.querySelector('.test-connection-btn');
        const statusEl = this.container.querySelector('.test-connection-status');
        if (!testBtn || !statusEl) return;

        const currentConfig = this.getConfig();
        if (!currentConfig.connectorType) {
            statusEl.innerHTML = '<span style="color: #ef4444;">Select a connector type first</span>';
            return;
        }

        const originalHTML = testBtn.innerHTML;
        testBtn.disabled = true;
        testBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Testing...';
        statusEl.innerHTML = '';

        try {
            const response = await window.pipelineAPI.request('POST', '/connectivity/test', {
                connector_type: currentConfig.connectorType,
                config: currentConfig.config
            });

            if (response.success) {
                testBtn.innerHTML = '<i class="fas fa-check-circle"></i> Connected!';
                testBtn.classList.add('btn-success');
                testBtn.classList.remove('btn-outline-info');
                statusEl.innerHTML = '<span style="color: #10b981;">Connection successful</span>';
            } else {
                throw new Error(response.error || 'Connection failed');
            }
        } catch (error) {
            testBtn.innerHTML = '<i class="fas fa-times-circle"></i> Failed';
            testBtn.classList.add('btn-danger');
            testBtn.classList.remove('btn-outline-info');
            statusEl.innerHTML = `<span style="color: #ef4444;">${this.escapeHtml(error.message)}</span>`;
        }

        setTimeout(() => {
            testBtn.innerHTML = originalHTML;
            testBtn.classList.remove('btn-success', 'btn-danger');
            testBtn.classList.add('btn-outline-info');
            testBtn.disabled = false;
        }, 3000);
    }

    // ========================================
    // UTILITY METHODS
    // ========================================

    onChange() {
        const event = new CustomEvent('connectorConfigChanged', {
            detail: { config: this.getConfig() }
        });
        this.container.dispatchEvent(event);
    }

    updateTypeDescription(description) {
        const descEl = this.container.querySelector('.connector-type-description');
        if (descEl) {
            descEl.textContent = description;
        }
    }

    updateTestButton() {
        const testBtn = this.container.querySelector('.test-connection-btn');
        if (testBtn) {
            testBtn.disabled = !this.config.connectorType;
        }
    }

    formatGroupName(name) {
        const icons = {
            basic: '<i class="fas fa-cog"></i> ',
            security: '<i class="fas fa-shield-alt"></i> ',
            advanced: '<i class="fas fa-sliders-h"></i> '
        };
        return (icons[name] || '') + name.charAt(0).toUpperCase() + name.slice(1);
    }

    formatFieldName(name) {
        return name.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
    }

    parseJSON(value) {
        if (!value) return null;
        if (typeof value === 'object') return value;
        try {
            return JSON.parse(value);
        } catch {
            return null;
        }
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }
}

// ========================================
// EXPORT & REGISTRATION
// ========================================

if (typeof window !== 'undefined') {
    window.ConnectorConfigBuilder = ConnectorConfigBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = ConnectorConfigBuilder;
}

// Auto-register with factory if available
if (typeof stepConfigBuilderFactory !== 'undefined') {
    stepConfigBuilderFactory.register('connector.outbound', ConnectorConfigBuilder, {
        displayName: 'Outbound Connector',
        category: 'Connectivity',
        description: 'Send data to external systems via configurable connectors',
        icon: '📤'
    });

    stepConfigBuilderFactory.register('connector.inbound', ConnectorConfigBuilder, {
        displayName: 'Inbound Connector',
        category: 'Connectivity',
        description: 'Fetch data from external systems via configurable connectors',
        icon: '📥'
    });

    console.log('[ConnectorConfigBuilder] Registered with factory for connector.outbound and connector.inbound');
}

console.log('[ConnectorConfigBuilder] Loaded - OOP connector configuration builder ready');
