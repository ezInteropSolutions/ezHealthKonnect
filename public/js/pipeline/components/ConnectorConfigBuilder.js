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

        // Tab bar — inbound: Connection | Acknowledgment; outbound: Connection | Payload | Recovery
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
                <button class="connector-tab" data-tab="recovery">
                    <i class="fas fa-exclamation-triangle"></i> Recovery
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
        // FHIR connectors skip this — they expose requestTimeoutSeconds in their own Advanced section
        const isFHIRConnector = (this.config.connectorType || '').includes('fhir');
        if (this.direction === 'inbound' && !isFHIRConnector) {
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

            // ── Recovery Queue panel (outbound only) ──
            const recoveryPanel = this.createElement('div', {
                class: 'connector-tab-panel',
                id: 'connPanel-recovery',
                'data-panel': 'recovery',
                style: 'display:none'
            });
            recoveryPanel.appendChild(this._buildRecoveryPanel());
            panels.appendChild(recoveryPanel);
        }

        this.container.appendChild(panels);

        // Load connector types from API
        this.loadConnectorTypes();
    }

    _buildRecoveryPanel() {
        const panel = this.createElement('div', { class: 'recovery-config-panel' });
        panel.innerHTML = `
            <p style="font-size:12px;color:#6b7280;margin-bottom:14px;">
                Configure how failed deliveries for this interface are retried.
                These settings apply to all outbound connectors on this interface and
                are saved immediately when you click <strong>Save Recovery Config</strong>.
            </p>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
                <div class="form-group">
                    <label style="font-size:12px;font-weight:600;">Max Attempts</label>
                    <input type="number" class="form-control dlq-field" data-dlq-field="max_attempts"
                           min="1" max="100" step="1" placeholder="10">
                    <small class="form-text text-muted">Attempts before abandonment (default 10)</small>
                </div>
                <div class="form-group">
                    <label style="font-size:12px;font-weight:600;">Initial Delay (s)</label>
                    <input type="number" class="form-control dlq-field" data-dlq-field="initial_delay_s"
                           min="0" step="1" placeholder="30">
                    <small class="form-text text-muted">Wait before first retry (default 30 s)</small>
                </div>
                <div class="form-group">
                    <label style="font-size:12px;font-weight:600;">Retry Delay (s)</label>
                    <input type="number" class="form-control dlq-field" data-dlq-field="retry_delay_s"
                           min="1" step="1" placeholder="60">
                    <small class="form-text text-muted">Base delay between retries (default 60 s)</small>
                </div>
                <div class="form-group">
                    <label style="font-size:12px;font-weight:600;">Backoff Multiplier</label>
                    <input type="number" class="form-control dlq-field" data-dlq-field="backoff_multiplier"
                           min="1" max="10" step="0.1" placeholder="2.0">
                    <small class="form-text text-muted">Exponential factor per attempt (default 2.0)</small>
                </div>
                <div class="form-group" style="grid-column:1/-1;">
                    <label style="font-size:12px;font-weight:600;">Payload Expires After (hours)</label>
                    <input type="number" class="form-control dlq-field" data-dlq-field="expires_after_hours"
                           min="0" step="1" placeholder="0">
                    <small class="form-text text-muted">Hours until payload is purged. 0 = never.</small>
                </div>
            </div>
            <div style="margin-top:14px;">
                <button type="button" class="btn btn-sm btn-warning save-dlq-config-btn"
                    style="font-size:12px;">
                    <i class="fas fa-save"></i> Save Recovery Config
                </button>
                <span class="dlq-save-status" style="font-size:12px;margin-left:10px;"></span>
            </div>
        `;

        // Pre-populate from interface DLQ config (loaded async)
        this._loadAndPopulateDLQConfig(panel);

        // Wire save button
        setTimeout(() => {
            const btn = panel.querySelector('.save-dlq-config-btn');
            if (btn) btn.addEventListener('click', () => this._saveDLQConfig(panel));
        }, 0);

        return panel;
    }

    _getInterfaceId() {
        const params = new URLSearchParams(window.location.search);
        return params.get('interfaceId') || null;
    }

    async _loadAndPopulateDLQConfig(panel) {
        const ifaceId = this._getInterfaceId();
        if (!ifaceId) return;
        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(`/api/interfaces/${ifaceId}`, { credentials: 'include', headers });
            if (!res.ok) return;
            const data = await res.json();
            const dlq = (data.data || data.interface || data).dlq_config || {};
            panel.querySelectorAll('.dlq-field').forEach(input => {
                const key = input.dataset.dlqField;
                if (key && dlq[key] != null) input.value = dlq[key];
            });
        } catch (_) { /* non-fatal */ }
    }

    async _saveDLQConfig(panel) {
        const ifaceId = this._getInterfaceId();
        if (!ifaceId) {
            const status = panel.querySelector('.dlq-save-status');
            if (status) { status.textContent = 'No interface ID in URL'; status.style.color = '#dc2626'; }
            return;
        }

        const dlqConfig = {};
        panel.querySelectorAll('.dlq-field').forEach(input => {
            const key = input.dataset.dlqField;
            const raw = input.value.trim();
            if (!raw) return;
            dlqConfig[key] = key === 'backoff_multiplier' ? parseFloat(raw) : parseInt(raw, 10);
        });

        const status = panel.querySelector('.dlq-save-status');
        const btn    = panel.querySelector('.save-dlq-config-btn');
        if (btn) btn.disabled = true;
        if (status) { status.textContent = 'Saving…'; status.style.color = '#6b7280'; }

        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(`/api/interfaces/${ifaceId}`, {
                method: 'PUT',
                credentials: 'include',
                headers,
                body: JSON.stringify({ dlq_config: dlqConfig }),
            });
            const json = await res.json();
            if (json.success || json.id) {
                if (status) { status.textContent = 'Saved'; status.style.color = '#16a34a'; }
                setTimeout(() => { if (status) status.textContent = ''; }, 3000);
            } else {
                if (status) { status.textContent = json.error || 'Save failed'; status.style.color = '#dc2626'; }
            }
        } catch (err) {
            if (status) { status.textContent = 'Network error'; status.style.color = '#dc2626'; }
        } finally {
            if (btn) btn.disabled = false;
        }
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

            /* ── FHIR No-Code UX ────────────────────────────────────────────── */

            .fhir-schema-notice {
                display: flex;
                align-items: center;
                gap: 10px;
                padding: 10px 14px;
                background: linear-gradient(135deg, #eff6ff 0%, #f0f9ff 100%);
                border: 1px solid #bfdbfe;
                border-left: 3px solid #1d4ed8;
                border-radius: 7px;
                margin-bottom: 14px;
                font-size: 12px;
            }
            .fhir-schema-notice-icon { color: #1d4ed8; font-size: 15px; flex-shrink: 0; }
            .fhir-schema-notice-body { flex: 1; color: #1e3a8a; line-height: 1.5; }
            .fhir-schema-notice-body strong { color: #1e3a8a; }
            .fhir-schema-notice-body a { color: #1d4ed8; font-weight: 600; text-decoration: none; margin-left: 6px; }
            .fhir-schema-notice-body a:hover { text-decoration: underline; }
            .fhir-schema-notice-dismiss {
                background: none; border: none; color: #94a3b8;
                cursor: pointer; font-size: 12px; padding: 2px; line-height: 1; flex-shrink: 0;
            }
            .fhir-schema-notice-dismiss:hover { color: #475569; }

            /* Section cards */
            .fhir-section {
                border: 1px solid #e2e8f0;
                border-radius: 10px;
                margin-bottom: 10px;
                overflow: hidden;
            }
            .fhir-section-header {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 11px 14px;
                background: linear-gradient(90deg, #f8fafc 0%, #f1f5f9 100%);
                cursor: pointer;
                user-select: none;
                border-bottom: 1px solid #e2e8f0;
                transition: background 0.15s;
            }
            .fhir-section-header:hover { background: linear-gradient(90deg, #f1f5f9 0%, #e8edf5 100%); }
            .fhir-section.collapsed .fhir-section-header { border-bottom-color: transparent; }
            .fhir-section-header-left { display: flex; align-items: center; gap: 8px; }
            .fhir-section-header-right { display: flex; align-items: center; gap: 8px; }
            .fhir-section-icon { color: #1e3a8a; font-size: 12px; width: 15px; text-align: center; }
            .fhir-section-title { font-weight: 600; font-size: 13px; color: #1e293b; }
            .fhir-section-summary { font-size: 11px; color: #64748b; font-weight: 500; }
            .fhir-section-chevron { color: #94a3b8; font-size: 11px; transition: transform 0.2s; }
            .fhir-section.collapsed .fhir-section-chevron { transform: rotate(-90deg); }
            .fhir-section-body { padding: 14px; }
            .fhir-section.collapsed .fhir-section-body { display: none; }

            /* Field helpers */
            .fhir-field-label { font-weight: 600; font-size: 12px; color: #374151; margin-bottom: 5px; display: block; }
            .fhir-required { color: #ef4444; }
            .fhir-hint { font-size: 11px; color: #94a3b8; margin-top: 3px; display: block; line-height: 1.4; }
            .fhir-field-row { display: flex; gap: 10px; }
            .fhir-field-row .fhir-field { flex: 1; min-width: 0; }

            /* Intent cards — inbound "what will senders do?" */
            .fhir-intent-grid {
                display: grid;
                grid-template-columns: repeat(3, 1fr);
                gap: 8px;
                margin-top: 6px;
            }
            .fhir-intent-card {
                border: 2px solid #e2e8f0;
                border-radius: 8px;
                padding: 11px 8px;
                cursor: pointer;
                transition: border-color 0.15s, background 0.15s, box-shadow 0.15s;
                background: #fff;
                display: block;
            }
            .fhir-intent-card input[type="radio"] { display: none; }
            .fhir-intent-card:hover { border-color: #93c5fd; background: #f0f7ff; }
            .fhir-intent-card.selected {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, #eff6ff 0%, #f0f4ff 100%);
                box-shadow: 0 0 0 1px #1e3a8a;
            }
            .fhir-intent-card.selected .fhir-intent-body i { color: #1e3a8a; }
            .fhir-intent-body { display: flex; flex-direction: column; align-items: center; gap: 5px; text-align: center; }
            .fhir-intent-body i { font-size: 18px; color: #94a3b8; transition: color 0.15s; }
            .fhir-intent-title { font-size: 12px; font-weight: 600; color: #1e293b; }
            .fhir-intent-sub { font-size: 10px; color: #94a3b8; line-height: 1.3; }

            /* Bundle mode toggle */
            .fhir-bundle-toggle { display: flex; flex-direction: column; gap: 6px; margin-top: 6px; }
            .fhir-bundle-option {
                border: 2px solid #e2e8f0;
                border-radius: 8px;
                padding: 10px 12px;
                cursor: pointer;
                transition: border-color 0.15s, background 0.15s;
                background: #fff;
                display: block;
            }
            .fhir-bundle-option input[type="radio"] { display: none; }
            .fhir-bundle-option:hover { border-color: #93c5fd; background: #f0f7ff; }
            .fhir-bundle-option.selected {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, #eff6ff 0%, #f0f4ff 100%);
            }
            .fhir-bundle-body { display: flex; align-items: center; gap: 10px; }
            .fhir-bundle-body > i { font-size: 16px; color: #94a3b8; width: 18px; text-align: center; flex-shrink: 0; }
            .fhir-bundle-option.selected .fhir-bundle-body > i { color: #1e3a8a; }
            .fhir-bundle-title { font-size: 12px; font-weight: 600; color: #1e293b; }
            .fhir-bundle-sub { font-size: 11px; color: #64748b; line-height: 1.3; margin-top: 2px; }

            /* Routing info callout */
            .fhir-routing-info {
                display: flex;
                align-items: flex-start;
                gap: 8px;
                padding: 10px 12px;
                background: #f0fdf4;
                border: 1px solid #bbf7d0;
                border-radius: 6px;
                margin-top: 14px;
                font-size: 12px;
                color: #166534;
                line-height: 1.5;
            }
            .fhir-routing-info i { color: #16a34a; font-size: 13px; flex-shrink: 0; margin-top: 1px; }
            .fhir-routing-info code {
                background: #dcfce7; color: #14532d;
                padding: 1px 4px; border-radius: 3px; font-size: 11px;
            }

            /* Auth credential fields */
            .fhir-auth-cred { margin-top: 4px; }

            /* Outbound send-mode cards */
            .fhir-send-mode-grid { display: flex; flex-direction: column; gap: 6px; margin-top: 6px; }
            .fhir-send-card {
                border: 2px solid #e2e8f0;
                border-radius: 8px;
                padding: 10px 12px;
                cursor: pointer;
                transition: border-color 0.15s, background 0.15s, box-shadow 0.15s;
                background: #fff;
                display: block;
            }
            .fhir-send-card input[type="radio"] { display: none; }
            .fhir-send-card:hover { border-color: #93c5fd; background: #f0f7ff; }
            .fhir-send-card.selected {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, #eff6ff 0%, #f0f4ff 100%);
                box-shadow: 0 0 0 1px #1e3a8a;
            }
            .fhir-send-card-body { display: flex; align-items: center; gap: 12px; }
            .fhir-send-card-icon {
                width: 34px; height: 34px;
                background: #dbeafe; border-radius: 7px;
                display: flex; align-items: center; justify-content: center;
                color: #1d4ed8; font-size: 14px; flex-shrink: 0;
                transition: background 0.15s, color 0.15s;
            }
            .fhir-send-card.selected .fhir-send-card-icon {
                background: linear-gradient(135deg, #1e3a8a 0%, #1d4ed8 100%);
                color: #fff;
            }
            .fhir-send-card-title { font-size: 13px; font-weight: 600; color: #1e293b; }
            .fhir-send-card-sub { font-size: 11px; color: #64748b; margin-top: 1px; line-height: 1.3; }

            /* URL preview — dark terminal-style card */
            .fhir-url-preview-card {
                display: flex;
                align-items: center;
                gap: 10px;
                padding: 11px 14px;
                background: #0f172a;
                border-radius: 8px;
                font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                font-size: 12px;
                border: 1px solid #1e293b;
            }
            .fhir-url-preview-verb {
                font-weight: 700;
                font-size: 10px;
                padding: 3px 7px;
                border-radius: 4px;
                flex-shrink: 0;
                letter-spacing: 0.5px;
            }
            .fhir-verb-post   { background: #1d4ed8; color: #fff; }
            .fhir-verb-put    { background: #b45309; color: #fff; }
            .fhir-verb-get    { background: #047857; color: #fff; }
            .fhir-verb-delete { background: #b91c1c; color: #fff; }
            .fhir-url-preview-url { flex: 1; color: #7dd3fc; word-break: break-all; min-width: 0; }
            .fhir-url-copy-btn {
                background: none; border: none; color: #334155; cursor: pointer;
                font-size: 13px; padding: 2px; flex-shrink: 0; transition: color 0.15s;
            }
            .fhir-url-copy-btn:hover { color: #cbd5e1; }
            .fhir-url-preview-note { font-size: 11px; color: #64748b; margin-top: 5px; font-style: italic; }

            /* Operations list */
            .fhir-ops-list { display: flex; flex-direction: column; gap: 4px; }
            .fhir-ops-item {
                display: flex;
                align-items: center;
                gap: 10px;
                padding: 8px 10px;
                border: 1px solid #e2e8f0;
                border-radius: 7px;
                cursor: pointer;
                transition: border-color 0.15s, background 0.15s;
                background: #fff;
            }
            .fhir-ops-item input[type="radio"] { display: none; }
            .fhir-ops-item:hover { border-color: #93c5fd; background: #f0f7ff; }
            .fhir-ops-item:has(input:checked) {
                border-color: #1e3a8a;
                background: linear-gradient(135deg, #eff6ff 0%, #f0f4ff 100%);
            }
            .fhir-ops-icon { color: #94a3b8; font-size: 12px; width: 15px; text-align: center; flex-shrink: 0; }
            .fhir-ops-item:has(input:checked) .fhir-ops-icon { color: #1e3a8a; }
            .fhir-ops-title { font-size: 12px; font-weight: 600; color: #1e293b; }
            .fhir-ops-sub { font-size: 11px; color: #64748b; line-height: 1.3; margin-top: 1px; }
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

            // Type name aliases: DB may store a legacy name while step config stores the
            // canonical Phase A name, or vice-versa. Both directions listed.
            const TYPE_ALIASES = {
                // MLLP
                'tcp_mllp_inbound':  'tcp_mllp',
                'tcp_mllp':          'tcp_mllp_inbound',
                // HTTP generic legacy
                'http_rest_inbound': 'http_rest',
                'http_rest':         'http_rest_inbound',
            };
            const storedType = this.config.connectorType || '';

            // Build grouped dropdown options using ui_category if available
            const UI_CATEGORY_ORDER = [
                'EMR / EHR', 'Healthcare Protocols', 'Healthcare Network',
                'Payers & Revenue Cycle', 'Databases', 'NoSQL Databases',
                'Data Warehouses', 'Message Queues', 'Cloud Storage',
                'File Transfer', 'Other',
            ];

            typeSelect.innerHTML = '<option value="">-- Select Connector Type --</option>';
            let matchedTypeName = '';

            // Group types by ui_category (fall back to flat list if no ui_category)
            const hasCategories = types.some(ct => ct.ui_category);
            if (hasCategories) {
                const grouped = {};
                types.forEach(ct => {
                    const cat = ct.ui_category || 'Other';
                    if (!grouped[cat]) grouped[cat] = [];
                    grouped[cat].push(ct);
                });
                // Render in defined order, then any remaining categories
                const catOrder = [...UI_CATEGORY_ORDER, ...Object.keys(grouped).filter(c => !UI_CATEGORY_ORDER.includes(c))];
                catOrder.forEach(cat => {
                    const items = grouped[cat];
                    if (!items || items.length === 0) return;
                    const optgroup = document.createElement('optgroup');
                    optgroup.label = cat;
                    items.forEach(ct => {
                        const option = document.createElement('option');
                        option.value = ct.type_name;
                        option.textContent = `${ct.icon || ''} ${ct.display_name}${ct.is_beta ? ' (Beta)' : ''}`;
                        const isMatch = ct.type_name === storedType || ct.type_name === TYPE_ALIASES[storedType];
                        if (isMatch) { option.selected = true; matchedTypeName = ct.type_name; }
                        optgroup.appendChild(option);
                    });
                    typeSelect.appendChild(optgroup);
                });
            } else {
                // Flat fallback (no ui_category column yet)
                types.forEach(ct => {
                    const option = document.createElement('option');
                    option.value = ct.type_name;
                    option.textContent = `${ct.icon || ''} ${ct.display_name}${ct.is_beta ? ' (Beta)' : ''}`;
                    const isMatch = ct.type_name === storedType || ct.type_name === TYPE_ALIASES[storedType];
                    if (isMatch) { option.selected = true; matchedTypeName = ct.type_name; }
                    typeSelect.appendChild(option);
                });
            }

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
            // MLLP
            'tcp_mllp_inbound':  'tcp_mllp',
            'tcp_mllp':          'tcp_mllp_inbound',
            // HTTP generic legacy
            'http_rest_inbound': 'http_rest',
            'http_rest':         'http_rest_inbound',
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
        // MLLP / raw TCP — send the original HL7 wire string
        if (name.includes('mllp') || name.includes('tcp')) return 'raw';
        // FHIR-aware HTTP sender — default to 'payload' (output of a payload.builder step)
        // Fall back to 'fhirBundle' if no payload.builder step precedes this one.
        if (name.includes('fhir_outbound') || name === 'fhir_r4_outbound') return 'payload';
        // Generic HTTP — could be FHIR or arbitrary JSON; default to fhirBundle for healthcare flows
        if (name.includes('http')) return 'fhirBundle';
        // File, MQ, cloud storage, DB — send the whole message object
        return 'message';
    }

    // ========================================
    // DYNAMIC CONFIG RENDERING (Schema-Driven)
    // ========================================

    renderDynamicConfig(container) {
        container.innerHTML = '';

        // FHIR connectors get a dedicated no-code UX — skip generic schema rendering
        if ((this.selectedType?.type_name || '').includes('fhir')) {
            this._renderFHIRConnectorUI(container);
            return;
        }

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

        // FHIR connectors: fully handled by _renderFHIRConnectorUI (via renderDynamicConfig)
        const isFHIR = typeName.includes('fhir');
        if (isFHIR) return;

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

        // Collect FHIR inbound TLS config (uses custom IDs, not connector-config-field)
        const tlsEnabledEl = this.container.querySelector('#fhirTLSEnabled');
        if (tlsEnabledEl) {
            const tls = { enabled: tlsEnabledEl.checked };
            const certEl = this.container.querySelector('#fhirTLSCert');
            const keyEl  = this.container.querySelector('#fhirTLSKey');
            if (certEl?.value.trim()) tls.certFile = certEl.value.trim();
            if (keyEl?.value.trim())  tls.keyFile  = keyEl.value.trim();
            config.tls = tls;
        }

        // Collect FHIR inbound IP allowlist (one entry per line)
        const ipEl = this.container.querySelector('#fhirAllowedIPs');
        if (ipEl) {
            const ips = ipEl.value.trim().split(/\s+/).filter(Boolean);
            config.allowedIPs = ips;
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

    // ─────────────────────────────────────────────────────────────────────────
    // FHIR-specific field enhancers
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Wires dynamic show/hide of credential fields for FHIR connectors.
     * Handles the camelCase schema (authType: none|basic|bearer|smart|api_key)
     * used by http_fhir_outbound, http_fhir_inbound, fhir_r4_outbound, fhir_r4_inbound.
     * Also upgrades bare enum option text to user-friendly labels.
     */
    _applyFHIRAuthDynamics(container, existingConfig) {
        const authSelect = container.querySelector('.connector-config-field[data-field="authType"]');
        if (!authSelect) return;

        // Upgrade enum option labels
        const LABELS = {
            'none':    'No Authentication',
            'basic':   'Basic Auth (Username / Password)',
            'bearer':  'Bearer Token',
            'smart':   'SMART on FHIR (Bearer token + token endpoint)',
            'api_key': 'API Key',
        };
        authSelect.querySelectorAll('option').forEach(opt => {
            if (LABELS[opt.value]) opt.textContent = LABELS[opt.value];
        });

        const showField = (fieldName, visible) => {
            const el = container.querySelector(`.connector-config-field[data-field="${fieldName}"]`)
                                ?.closest('.form-group');
            if (el) el.style.display = visible ? '' : 'none';
        };

        const applyVisibility = (type) => {
            showField('username',     type === 'basic');
            showField('password',     type === 'basic');
            showField('bearerToken',  type === 'bearer' || type === 'smart');
            showField('apiKey',       type === 'api_key');
            showField('apiKeyHeader', type === 'api_key');
        };

        const currentAuth = authSelect.value || existingConfig.authType || 'none';
        applyVisibility(currentAuth);
        authSelect.addEventListener('change', () => applyVisibility(authSelect.value));
    }

    /**
     * Replaces the bare `resourceType` text input on FHIR outbound connectors
     * with a smart combo widget:
     *   - Dropdown: "Auto-detect" (blank) + 40+ FHIR R4 resource types + "Custom…"
     *   - Custom text input appears when "Custom…" is selected (developer escape hatch)
     * A hidden input with data-field="resourceType" stays as the real form value so
     * getConfig() picks it up without any override.
     */
    _enhanceFHIRResourceTypeField(container, existingConfig) {
        const input = container.querySelector('.connector-config-field[data-field="resourceType"]');
        if (!input || input.tagName === 'SELECT') return;

        const FHIR_R4_TYPES = [
            // Administrative
            'Patient', 'Practitioner', 'PractitionerRole', 'Organization', 'Location',
            'HealthcareService', 'RelatedPerson', 'Person', 'Group',
            // Clinical
            'Encounter', 'EpisodeOfCare', 'Condition', 'Observation', 'DiagnosticReport',
            'Procedure', 'FamilyMemberHistory', 'ClinicalImpression', 'DetectedIssue',
            // Medications
            'Medication', 'MedicationRequest', 'MedicationAdministration',
            'MedicationDispense', 'MedicationStatement',
            // Allergies & Immunizations
            'AllergyIntolerance', 'Immunization', 'ImmunizationRecommendation',
            // Care planning
            'CarePlan', 'CareTeam', 'Goal', 'ServiceRequest', 'ReferralRequest',
            // Documents & communications
            'DocumentReference', 'Composition', 'Communication', 'Flag', 'Task',
            // Orders & referrals
            'Appointment', 'AppointmentResponse', 'Schedule', 'Slot',
            // Diagnostics & imaging
            'ImagingStudy', 'Media', 'BodyStructure', 'Specimen',
            // Financial
            'Claim', 'ClaimResponse', 'ExplanationOfBenefit', 'Coverage',
            'CoverageEligibilityRequest', 'CoverageEligibilityResponse',
            // Infrastructure
            'Bundle', 'List', 'MessageHeader', 'Parameters',
        ];

        const currentVal = input.value || existingConfig.resourceType || '';
        const isCustom   = currentVal !== '' && !FHIR_R4_TYPES.includes(currentVal);

        // Hidden real-field input — getConfig reads this
        const hidden = document.createElement('input');
        hidden.type = 'hidden';
        hidden.className = 'connector-config-field';
        hidden.dataset.field = 'resourceType';
        hidden.value = currentVal;

        // Detach original from getConfig
        input.removeAttribute('data-field');
        input.classList.remove('connector-config-field');

        // Build dropdown
        const sel = document.createElement('select');
        sel.className = 'form-control form-control-sm';

        const mkOpt = (val, text, selected) => {
            const o = document.createElement('option');
            o.value = val;
            o.textContent = text;
            if (selected) o.selected = true;
            return o;
        };

        sel.appendChild(mkOpt('', '✦  Auto-detect from message body (recommended)', currentVal === ''));
        FHIR_R4_TYPES.forEach(rt =>
            sel.appendChild(mkOpt(rt, rt, !isCustom && rt === currentVal))
        );
        sel.appendChild(mkOpt('_custom_', '✎  Custom resource type…', isCustom));

        // Developer custom input
        const customInput = document.createElement('input');
        customInput.type = 'text';
        customInput.className = 'form-control form-control-sm';
        customInput.placeholder = 'Enter any FHIR resource type, e.g. MolecularSequence';
        customInput.value = isCustom ? currentVal : '';
        customInput.style.cssText = `display:${isCustom ? '' : 'none'};margin-top:4px;`;

        const updateHidden = () => {
            hidden.value = sel.value === '_custom_' ? customInput.value.trim() : sel.value;
            this.onChange();
        };

        sel.addEventListener('change', () => {
            customInput.style.display = sel.value === '_custom_' ? '' : 'none';
            if (sel.value === '_custom_') customInput.focus();
            updateHidden();
        });
        customInput.addEventListener('input', updateHidden);

        // Help text
        const help = document.createElement('small');
        help.className = 'form-text text-muted';
        help.innerHTML =
            '<strong>Auto-detect</strong>: reads <code>resourceType</code> from the FHIR message body — ' +
            'correct for HL7&rarr;FHIR pipelines. Select a specific type to override the URL ' +
            '(e.g. force all messages to <code>POST /Patient</code>).';

        // Swap input → select + custom input + hidden + help
        const formGroup = input.closest('.form-group');
        if (formGroup) {
            input.replaceWith(sel);
            sel.after(customInput);
            customInput.after(hidden);
            formGroup.appendChild(help);
        }
    }

    /**
     * Replaces the bare `operation` text input with a smart combo widget.
     * Groups common FHIR operations by level (server / type / instance) so
     * no-code users can pick the right operation, while developers can type any custom one.
     */
    _enhanceFHIROperationField(container, existingConfig) {
        const input = container.querySelector('.connector-config-field[data-field="operation"]');
        if (!input || input.tagName === 'SELECT') return;

        // Grouped FHIR operations: [value, label, level-hint]
        const FHIR_OPS = [
            // group: Server-level
            { group: 'Server-level operations', ops: [
                { v: '$process-message', l: '$process-message — process a FHIR message Bundle' },
                { v: '$export',          l: '$export — bulk data export (SMART on FHIR)' },
                { v: '$convert',         l: '$convert — format conversion' },
            ]},
            // group: Type-level (no id needed)
            { group: 'Type-level operations (no Resource ID)', ops: [
                { v: '$validate',   l: '$validate — validate a resource against profiles' },
                { v: '$match',      l: '$match — patient matching (MPI)' },
                { v: '$everything', l: '$everything — return all related resources (type)' },
                { v: '$expand',     l: '$expand — expand a ValueSet' },
                { v: '$lookup',     l: '$lookup — terminology lookup' },
                { v: '$translate',  l: '$translate — concept translation' },
                { v: '$apply',      l: '$apply — apply a PlanDefinition' },
                { v: '$evaluate',   l: '$evaluate — evaluate a Measure' },
            ]},
            // group: Instance-level (requires Resource ID)
            { group: 'Instance-level operations (Resource ID required)', ops: [
                { v: '$everything',    l: '$everything — all info for this resource instance' },
                { v: '$validate',      l: '$validate — validate this specific resource' },
                { v: '$meta',          l: '$meta — read metadata tags for this resource' },
                { v: '$meta-add',      l: '$meta-add — add metadata tags' },
                { v: '$meta-delete',   l: '$meta-delete — delete metadata tags' },
                { v: '$document',      l: '$document — generate a document (Composition)' },
            ]},
        ];

        const currentVal = input.value || existingConfig.operation || '';
        const allOpValues = FHIR_OPS.flatMap(g => g.ops.map(o => o.v));
        // Deduplicate ($everything and $validate appear in multiple groups)
        const knownValues = [...new Set(allOpValues)];
        const isCustom    = currentVal !== '' && !knownValues.includes(currentVal);

        // Hidden real field
        const hidden = document.createElement('input');
        hidden.type = 'hidden';
        hidden.className = 'connector-config-field';
        hidden.dataset.field = 'operation';
        hidden.value = currentVal;

        input.removeAttribute('data-field');
        input.classList.remove('connector-config-field');

        // Build select
        const sel = document.createElement('select');
        sel.className = 'form-control form-control-sm';
        sel.id = 'fhir-operation-select';

        const mkOpt = (v, t, sel) => { const o = document.createElement('option'); o.value = v; o.textContent = t; if (sel) o.selected = true; return o; };
        sel.appendChild(mkOpt('', '— No operation (standard CRUD)', currentVal === ''));

        FHIR_OPS.forEach(group => {
            const og = document.createElement('optgroup');
            og.label = group.group;
            // deduplicate within group display
            const seen = new Set();
            group.ops.forEach(op => {
                if (seen.has(op.v)) return;
                seen.add(op.v);
                og.appendChild(mkOpt(op.v, op.l, !isCustom && op.v === currentVal));
            });
            sel.appendChild(og);
        });
        sel.appendChild(mkOpt('_custom_', '✎  Custom operation…', isCustom));

        // Custom input
        const customInput = document.createElement('input');
        customInput.type = 'text';
        customInput.className = 'form-control form-control-sm';
        customInput.placeholder = 'e.g. $myCustomOp  ($ prefix optional)';
        customInput.value = isCustom ? currentVal : '';
        customInput.style.cssText = `display:${isCustom ? '' : 'none'};margin-top:4px;`;

        const updateHidden = () => {
            hidden.value = sel.value === '_custom_' ? customInput.value.trim() : sel.value;
            this.onChange();
        };
        sel.addEventListener('change', () => {
            customInput.style.display = sel.value === '_custom_' ? '' : 'none';
            if (sel.value === '_custom_') customInput.focus();
            updateHidden();
        });
        customInput.addEventListener('input', updateHidden);

        // Help
        const help = document.createElement('small');
        help.className = 'form-text text-muted';
        help.innerHTML = 'Appended to the URL as <code>/<b>$operation</b></code>. Level (server / type / instance) is determined by Resource Type and Resource ID settings above.';

        const formGroup = input.closest('.form-group');
        if (formGroup) {
            input.replaceWith(sel);
            sel.after(customInput);
            customInput.after(hidden);
            formGroup.appendChild(help);
        }
    }

    /**
     * Adds a live endpoint URL preview panel below the resource routing fields
     * on FHIR outbound connectors.  Updates in real-time as baseUrl, resourceType,
     * resourceId, and bundleType change — exactly mirroring fhir_utils.BuildFHIRURL.
     */
    _addFHIREndpointPreview(container, existingConfig) {
        // Find the Basic group body to append the preview after the last basic field
        const basicGroup = container.querySelector('.connector-config-group');
        if (!basicGroup) return;

        const preview = document.createElement('div');
        preview.id = 'fhir-endpoint-preview';
        preview.style.cssText = `
            background:#f0f7ff;border:1px solid #b3d1f7;border-radius:6px;
            padding:10px 12px;margin-top:12px;font-size:12px;
        `;

        const title = document.createElement('div');
        title.style.cssText = 'color:#1a5276;font-weight:600;margin-bottom:4px;';
        title.textContent = 'Resolved Endpoint';
        preview.appendChild(title);

        const urlLine = document.createElement('div');
        urlLine.style.cssText = 'font-family:monospace;color:#1a73e8;word-break:break-all;';
        preview.appendChild(urlLine);

        const helpLine = document.createElement('div');
        helpLine.style.cssText = 'color:#555;margin-top:4px;font-size:11px;';
        preview.appendChild(helpLine);

        basicGroup.appendChild(preview);

        // ── JS mirror of fhir_utils.BuildFHIRURL ───────────────────────────
        // Keep in sync with Go implementation in fhir_utils.go.
        const buildFHIRURL = (baseUrl, resourceType, resourceId, bundleType, operation, queryParams, method) => {
            baseUrl = (baseUrl || '').replace(/\/+$/, '');
            // Normalise operation
            let op = (operation || '').trim();
            if (op && !op.startsWith('$')) op = '$' + op;
            method = (method || 'POST').toUpperCase();
            let url, note;

            if (!resourceType && !op) {
                // Auto-detect: runtime reads resourceType from message body
                const rt = '<resourceType>';
                url  = baseUrl ? `${baseUrl}/${rt}` : rt;
                note = 'resourceType auto-detected from message body at runtime — URL shown is illustrative';
            } else if (!resourceType && op) {
                url  = `${baseUrl}/${op}`;
                note = `Server-level FHIR operation`;
            } else if (resourceType === 'Bundle') {
                if (bundleType === 'transaction') { url = `${baseUrl}/$transaction`; note = 'FHIR transaction bundle — atomic'; }
                else if (bundleType === 'batch')  { url = `${baseUrl}/$batch`;       note = 'FHIR batch bundle — independent ops'; }
                else                              { url = baseUrl;                    note = 'FHIR collection/document bundle'; }
            } else {
                url = `${baseUrl}/${resourceType}`;
                if (resourceId) {
                    url += `/${resourceId}`;
                    if (!op) { method = 'PUT'; note = `Update ${resourceType}/${resourceId}`; }
                }
                if (op) {
                    url += `/${op}`;
                    if (resourceId) note = `Instance-level operation on ${resourceType}/${resourceId}`;
                    else            note = `Type-level operation on ${resourceType}`;
                } else if (!note) {
                    note = resourceId ? `Update ${resourceType}` : `Create ${resourceType}`;
                }
            }

            // Append query string
            const qs = (queryParams || '').trim();
            if (qs) url += (qs.startsWith('?') ? qs : '?' + qs);

            // configMethod wins
            if (method !== 'POST' && method !== 'PUT') note = (note || '') + ` (method override: ${method})`;

            return { url, method, note: note || '' };
        };

        const refresh = () => {
            const baseUrl = container.querySelector('.connector-config-field[data-field="baseUrl"]')?.value?.trim()
                         || container.querySelector('.connector-config-field[data-field="url"]')?.value?.trim()
                         || container.querySelector('.connector-config-field[data-field="endpoint"]')?.value?.trim()
                         || '';
            const rtSelect     = container.querySelector('#fhir-resource-type-select');
            const rtHidden     = container.querySelector('.connector-config-field[data-field="resourceType"]');
            const resourceType = rtSelect
                ? (rtSelect.value === '_custom_' ? (rtHidden?.value || '') : rtSelect.value)
                : (rtHidden?.value || '');
            const resourceId   = container.querySelector('.connector-config-field[data-field="resourceId"]')?.value?.trim()  || '';
            const bundleType   = container.querySelector('.connector-config-field[data-field="bundleType"]')?.value          || '';
            const operation    = container.querySelector('.connector-config-field[data-field="operation"]')?.value?.trim()   || '';
            const queryParams  = container.querySelector('.connector-config-field[data-field="queryParams"]')?.value?.trim() || '';
            const method       = container.querySelector('.connector-config-field[data-field="method"]')?.value              || 'POST';

            const { url, method: verb, note } = buildFHIRURL(baseUrl, resourceType, resourceId, bundleType, operation, queryParams, method);

            const isRuntime = !resourceType && !operation;
            urlLine.style.color = isRuntime ? '#666' : '#1a73e8';
            urlLine.style.fontStyle = isRuntime ? 'italic' : 'normal';
            urlLine.innerHTML = `<strong style="color:${isRuntime ? '#555' : '#0d47a1'}">${verb}</strong>  ${this.escapeHtml(url)}`;
            helpLine.textContent = note;
        };

        // Wire to all relevant fields
        ['baseUrl', 'url', 'endpoint', 'resourceId', 'bundleType', 'operation', 'queryParams', 'method'].forEach(field => {
            container.querySelectorAll(`.connector-config-field[data-field="${field}"]`).forEach(el => {
                el.addEventListener('input',  refresh);
                el.addEventListener('change', refresh);
            });
        });
        // Resource type + operation selects added asynchronously — wire after a tick
        setTimeout(() => {
            container.querySelector('#fhir-resource-type-select')?.addEventListener('change', refresh);
            container.querySelector('#fhir-operation-select')?.addEventListener('change', refresh);
            refresh();
        }, 50);
    }

    /**
     * Adds a routing hint panel to FHIR inbound connectors explaining that the
     * connector accepts ALL FHIR resource types and routing per resource type
     * is done in the pipeline using Switch/Case or If-Then-Else steps.
     */
    _addFHIRInboundRoutingHint(container) {
        const basicGroup = container.querySelector('.connector-config-group');
        if (!basicGroup) return;

        const hint = document.createElement('div');
        hint.style.cssText = `
            background:#e8f5e9;border:1px solid #a5d6a7;border-radius:6px;
            padding:10px 12px;margin-top:12px;font-size:12px;color:#1b5e20;
        `;
        hint.innerHTML = `
            <strong>Accepts all FHIR resource types</strong><br>
            This connector receives any valid FHIR resource or Bundle on the configured base path.
            To route different resource types differently (e.g. handle <code>Patient</code> and
            <code>Observation</code> through separate pipeline branches), add a
            <strong>Switch / Case</strong> step after this connector that conditions on
            <code>messageType</code> (e.g. <code>FHIR:Patient</code>, <code>FHIR:Bundle:transaction</code>).
        `;
        basicGroup.appendChild(hint);
    }

    // =========================================================================
    // FHIR CONNECTOR — NO-CODE UX
    // =========================================================================

    _renderFHIRConnectorUI(container) {
        if (this.direction === 'inbound') {
            this._buildFHIRInboundUI(container);
        } else {
            this._buildFHIROutboundUI(container);
        }
    }

    /** Dismissible blue notice nudging users to install schema packages. */
    _buildFHIRSchemaBanner() {
        if (sessionStorage.getItem('fhir-schema-notice-dismissed')) return null;
        const banner = document.createElement('div');
        banner.className = 'fhir-schema-notice';
        banner.innerHTML = `
            <i class="fas fa-cube fhir-schema-notice-icon"></i>
            <div class="fhir-schema-notice-body">
                <strong>Schema packages</strong> power FHIR validation and rich field mapping.
                <a href="/settings.html#section-schemas" target="_blank">
                    Install schemas <i class="fas fa-external-link-alt" style="font-size:10px"></i>
                </a>
            </div>
            <button type="button" class="fhir-schema-notice-dismiss" title="Dismiss">
                <i class="fas fa-times"></i>
            </button>
        `;
        banner.querySelector('.fhir-schema-notice-dismiss').addEventListener('click', () => {
            banner.style.display = 'none';
            sessionStorage.setItem('fhir-schema-notice-dismissed', '1');
        });
        return banner;
    }

    /**
     * Creates a collapsible section card used throughout the FHIR no-code UX.
     * Returns a .fhir-section element with .fhir-section-body already wired for toggle.
     */
    _makeFHIRSection(title, iconClass, expanded) {
        const section = document.createElement('div');
        section.className = `fhir-section${expanded ? '' : ' collapsed'}`;
        section.innerHTML = `
            <div class="fhir-section-header">
                <div class="fhir-section-header-left">
                    <i class="${iconClass} fhir-section-icon"></i>
                    <span class="fhir-section-title">${title}</span>
                </div>
                <div class="fhir-section-header-right">
                    <span class="fhir-section-summary"></span>
                    <i class="fas fa-chevron-down fhir-section-chevron"></i>
                </div>
            </div>
            <div class="fhir-section-body"></div>
        `;
        section.querySelector('.fhir-section-header').addEventListener('click', () => {
            section.classList.toggle('collapsed');
        });
        return section;
    }

    // ── FHIR version loader ───────────────────────────────────────────────────
    // Fetches installed FHIR versions from /api/schemas/fhir/versions and
    // populates the given <select>. Falls back to the current value only.
    // Add a new FHIR version schema dir → it automatically appears here.

    _loadFHIRVersions(selectEl, currentValue) {
        if (!selectEl) return;

        const VERSION_LABELS = { R4: 'R4', R5: 'R5', R6: 'R6', STU3: 'STU3', STU2: 'STU2' };

        fetch('/api/schemas/fhir/versions')
            .then(r => r.ok ? r.json() : Promise.reject(r.status))
            .then(data => {
                const versions = data.versions || [];
                if (!versions.length) return; // keep existing fallback option

                selectEl.innerHTML = '';
                versions.forEach(v => {
                    const opt = document.createElement('option');
                    opt.value = v.value;
                    opt.textContent = VERSION_LABELS[v.value] || v.value;
                    if (v.value === currentValue) opt.selected = true;
                    selectEl.appendChild(opt);
                });

                // If saved value isn't in the list, default to first available
                if (!selectEl.value) selectEl.options[0].selected = true;

                selectEl.addEventListener('change', () => this.onChange());
            })
            .catch(() => {
                // API unavailable — keep the single fallback option already rendered
            });
    }

    // ── FHIR INBOUND ─────────────────────────────────────────────────────────

    _buildFHIRInboundUI(container) {
        const cfg = this.config.config || {};
        const esc = s => this.escapeHtml(String(s ?? ''));

        container.innerHTML = '';
        const banner = this._buildFHIRSchemaBanner();
        if (banner) container.appendChild(banner);

        // ── Endpoint Setup ────────────────────────────────────────────────
        const endSec = this._makeFHIRSection('Endpoint Setup', 'fas fa-server', true);
        const endBody = endSec.querySelector('.fhir-section-body');

        endBody.innerHTML = `
            <div class="fhir-field-row">
                <div class="fhir-field">
                    <label class="fhir-field-label">Listen Port <span class="fhir-required">*</span></label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="port" data-field-type="number"
                           value="${esc(cfg.port || 2576)}" min="1" max="65535" placeholder="2576">
                    <small class="fhir-hint">Port this FHIR endpoint listens on</small>
                </div>
                <div class="fhir-field">
                    <label class="fhir-field-label">FHIR Version</label>
                    <select class="form-control form-control-sm connector-config-field" data-field="fhirVersion" id="fhirVersionSelect">
                        <option value="${esc(cfg.fhirVersion || 'R4')}" selected>${esc(cfg.fhirVersion || 'R4')}</option>
                    </select>
                </div>
            </div>

            <div class="fhir-field" style="margin-top:14px">
                <label class="fhir-field-label">Which operations should this endpoint accept?</label>
                <div class="fhir-intent-grid" id="fhirInboundIntentGrid">
                    <label class="fhir-intent-card" data-methods='["POST","PUT","DELETE"]'>
                        <input type="radio" name="fhir-inbound-intent" value="write">
                        <div class="fhir-intent-body">
                            <i class="fas fa-inbox"></i>
                            <div class="fhir-intent-title">Receive writes</div>
                            <div class="fhir-intent-sub">POST · PUT · DELETE</div>
                        </div>
                    </label>
                    <label class="fhir-intent-card" data-methods='["GET"]'>
                        <input type="radio" name="fhir-inbound-intent" value="read">
                        <div class="fhir-intent-body">
                            <i class="fas fa-search"></i>
                            <div class="fhir-intent-title">Serve queries</div>
                            <div class="fhir-intent-sub">GET · search</div>
                        </div>
                    </label>
                    <label class="fhir-intent-card" data-methods='["GET","POST","PUT","PATCH","DELETE"]'>
                        <input type="radio" name="fhir-inbound-intent" value="full">
                        <div class="fhir-intent-body">
                            <i class="fas fa-layer-group"></i>
                            <div class="fhir-intent-title">Full FHIR API</div>
                            <div class="fhir-intent-sub">All methods</div>
                        </div>
                    </label>
                </div>
                <input type="hidden" class="connector-config-field" data-field="allowedMethods"
                       data-field-type="array" id="fhirAllowedMethodsHidden">
            </div>

            <div class="fhir-field" style="margin-top:14px">
                <label class="fhir-field-label">When you receive a FHIR Bundle</label>
                <div class="fhir-bundle-toggle">
                    <label class="fhir-bundle-option">
                        <input type="radio" name="fhir-bundle-mode" value="bundle_as_unit">
                        <div class="fhir-bundle-body">
                            <i class="fas fa-box"></i>
                            <div>
                                <div class="fhir-bundle-title">One message</div>
                                <div class="fhir-bundle-sub">Process the whole Bundle together — best for transactions</div>
                            </div>
                        </div>
                    </label>
                    <label class="fhir-bundle-option">
                        <input type="radio" name="fhir-bundle-mode" value="bundle_unwrap">
                        <div class="fhir-bundle-body">
                            <i class="fas fa-boxes"></i>
                            <div>
                                <div class="fhir-bundle-title">Split into resources</div>
                                <div class="fhir-bundle-sub">Each Bundle entry becomes its own message — best for batch ingestion</div>
                            </div>
                        </div>
                    </label>
                </div>
                <input type="hidden" class="connector-config-field" data-field="bundleMode" id="fhirBundleModeHidden">
            </div>

            <div class="fhir-routing-info">
                <i class="fas fa-route"></i>
                <div>
                    <strong>Accepts all FHIR resource types.</strong>
                    Use a <em>Switch / Case</em> step after this connector to branch by resource type
                    (<code>FHIR:Patient</code>, <code>FHIR:Bundle:transaction</code>, …)
                </div>
            </div>
        `;

        container.appendChild(endSec);

        // Populate FHIR version select dynamically from installed schemas
        this._loadFHIRVersions(endBody.querySelector('#fhirVersionSelect'), cfg.fhirVersion || 'R4');

        // Wire intent grid
        const intentGrid    = endBody.querySelector('#fhirInboundIntentGrid');
        const methodsHidden = endBody.querySelector('#fhirAllowedMethodsHidden');
        const currMethods   = Array.isArray(cfg.allowedMethods) ? cfg.allowedMethods : ['GET','POST','PUT','PATCH','DELETE'];

        const intentFromMethods = (m) => {
            const s = new Set(m);
            if (s.has('GET') && s.has('POST') && s.size >= 4) return 'full';
            if (s.has('GET') && !s.has('POST')) return 'read';
            return 'write';
        };
        const activeIntent = intentFromMethods(currMethods);
        methodsHidden.value = currMethods.join(', ');

        intentGrid.querySelectorAll('.fhir-intent-card').forEach(card => {
            const radio = card.querySelector('input[type="radio"]');
            if (radio.value === activeIntent) { radio.checked = true; card.classList.add('selected'); }
            card.addEventListener('click', () => {
                intentGrid.querySelectorAll('.fhir-intent-card').forEach(c => c.classList.remove('selected'));
                card.classList.add('selected');
                methodsHidden.value = JSON.parse(card.dataset.methods || '[]').join(', ');
                this.onChange();
            });
        });

        // Wire bundle mode
        const bundleHidden = endBody.querySelector('#fhirBundleModeHidden');
        const currBundle   = cfg.bundleMode || 'bundle_as_unit';
        bundleHidden.value = currBundle;
        endBody.querySelectorAll('input[name="fhir-bundle-mode"]').forEach(radio => {
            if (radio.value === currBundle) { radio.checked = true; radio.closest('.fhir-bundle-option').classList.add('selected'); }
            radio.addEventListener('change', () => {
                endBody.querySelectorAll('.fhir-bundle-option').forEach(o => o.classList.remove('selected'));
                radio.closest('.fhir-bundle-option').classList.add('selected');
                bundleHidden.value = radio.value;
                this.onChange();
            });
        });

        endBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => this.onChange());
            el.addEventListener('change', () => this.onChange());
        });

        // ── Security ──────────────────────────────────────────────────────
        const secSec  = this._makeFHIRSection('Security', 'fas fa-shield-alt', false);
        const secBody = secSec.querySelector('.fhir-section-body');
        const authType = cfg.authType || 'none';

        secBody.innerHTML = `
            <div class="fhir-field">
                <label class="fhir-field-label">Who can call this endpoint?</label>
                <select class="form-control form-control-sm connector-config-field"
                        data-field="authType" id="fhirInboundAuthSelect">
                    <option value="none"    ${authType === 'none'    ? 'selected' : ''}>Anyone — no authentication</option>
                    <option value="basic"   ${authType === 'basic'   ? 'selected' : ''}>Username &amp; Password</option>
                    <option value="bearer"  ${authType === 'bearer'  ? 'selected' : ''}>Bearer Token</option>
                    <option value="api_key" ${authType === 'api_key' ? 'selected' : ''}>API Key</option>
                </select>
            </div>
            <div id="fhirInboundAuthCreds" style="margin-top:10px">
                <div class="fhir-auth-cred" data-auth="basic">
                    <label class="fhir-field-label" style="margin-top:8px">Username</label>
                    <input type="text" class="form-control form-control-sm connector-config-field"
                           data-field="username" value="${esc(cfg.username || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="basic">
                    <label class="fhir-field-label" style="margin-top:8px">Password</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="password" value="${esc(cfg.password || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="bearer">
                    <label class="fhir-field-label" style="margin-top:8px">Bearer Token</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="bearerToken" value="${esc(cfg.bearerToken || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="api_key">
                    <label class="fhir-field-label" style="margin-top:8px">API Key</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="apiKey" value="${esc(cfg.apiKey || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="api_key">
                    <label class="fhir-field-label" style="margin-top:8px">API Key Header</label>
                    <input type="text" class="form-control form-control-sm connector-config-field"
                           data-field="apiKeyHeader" value="${esc(cfg.apiKeyHeader || 'X-API-Key')}">
                </div>
            </div>
        `;

        const AUTH_LABELS = { none: 'None', basic: 'Username / Password', bearer: 'Bearer token', api_key: 'API key' };
        const applyInboundAuth = (type) => {
            secSec.querySelector('.fhir-section-summary').textContent = AUTH_LABELS[type] || type;
            secBody.querySelectorAll('.fhir-auth-cred').forEach(el => {
                el.style.display = el.dataset.auth === type ? '' : 'none';
            });
        };
        applyInboundAuth(authType);
        secBody.querySelector('#fhirInboundAuthSelect').addEventListener('change', e => { applyInboundAuth(e.target.value); this.onChange(); });
        secBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => this.onChange());
            el.addEventListener('change', () => this.onChange());
        });
        container.appendChild(secSec);

        // ── Advanced ──────────────────────────────────────────────────────
        const advSec  = this._makeFHIRSection('Advanced', 'fas fa-sliders-h', false);
        const advBody = advSec.querySelector('.fhir-section-body');

        advBody.innerHTML = `
            <div class="fhir-field-row">
                <div class="fhir-field">
                    <label class="fhir-field-label">Base Path</label>
                    <input type="text" class="form-control form-control-sm connector-config-field"
                           data-field="basePath" value="${esc(cfg.basePath || '/fhir/r4')}" placeholder="/fhir/r4">
                    <small class="fhir-hint">URL prefix for all resources on this connector</small>
                </div>
                <div class="fhir-field">
                    <label class="fhir-field-label">Max Body Size (MB)</label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="maxBodySizeMB" data-field-type="number"
                           value="${esc(cfg.maxBodySizeMB || 10)}" min="1" max="500">
                </div>
            </div>
            <div class="fhir-field-row" style="margin-top:10px">
                <div class="fhir-field">
                    <label class="fhir-field-label">Request Timeout (seconds)</label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="requestTimeoutSeconds" data-field-type="number"
                           value="${esc(cfg.requestTimeoutSeconds || 30)}" min="1" max="300">
                </div>
                <div class="fhir-field" style="display:flex;align-items:flex-end;padding-bottom:2px">
                    <label style="display:flex;align-items:center;gap:7px;cursor:pointer;margin:0">
                        <input type="checkbox" class="connector-config-field" data-field="enableCORS"
                               data-field-type="boolean" ${cfg.enableCORS !== false ? 'checked' : ''}>
                        <span class="fhir-field-label" style="margin:0">Enable CORS</span>
                    </label>
                    <small class="fhir-hint" style="display:block;margin-top:2px">Allow browser-based FHIR clients</small>
                </div>
            </div>
        `;

        advBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => this.onChange());
            el.addEventListener('change', () => this.onChange());
        });
        container.appendChild(advSec);

        // ── TLS / HTTPS ───────────────────────────────────────────────────
        const tlsCfg   = cfg.tls || {};
        const tlsSec   = this._makeFHIRSection('TLS / HTTPS', 'fas fa-lock', false);
        const tlsBody  = tlsSec.querySelector('.fhir-section-body');
        const tlsEnabled = tlsCfg.enabled === true;

        tlsBody.innerHTML = `
            <div class="fhir-field" style="margin-bottom:10px">
                <label style="display:flex;align-items:center;gap:8px;cursor:pointer;margin:0">
                    <input type="checkbox" id="fhirTLSEnabled" ${tlsEnabled ? 'checked' : ''}>
                    <span class="fhir-field-label" style="margin:0">Enable HTTPS (TLS)</span>
                </label>
                <small class="fhir-hint">Recommended for production. Requires a signed certificate.</small>
            </div>
            <div id="fhirTLSFields" style="${tlsEnabled ? '' : 'display:none'}">
                <div class="fhir-field-row">
                    <div class="fhir-field">
                        <label class="fhir-field-label">Certificate File <span class="fhir-required">*</span></label>
                        <input type="text" class="form-control form-control-sm" id="fhirTLSCert"
                               value="${esc(tlsCfg.certFile || '')}" placeholder="/etc/ssl/certs/server.crt">
                        <small class="fhir-hint">Absolute path to PEM certificate file</small>
                    </div>
                    <div class="fhir-field">
                        <label class="fhir-field-label">Key File <span class="fhir-required">*</span></label>
                        <input type="text" class="form-control form-control-sm" id="fhirTLSKey"
                               value="${esc(tlsCfg.keyFile || '')}" placeholder="/etc/ssl/private/server.key">
                        <small class="fhir-hint">Absolute path to PEM private key file</small>
                    </div>
                </div>
            </div>
        `;

        tlsBody.querySelector('#fhirTLSEnabled').addEventListener('change', e => {
            tlsBody.querySelector('#fhirTLSFields').style.display = e.target.checked ? '' : 'none';
            tlsSec.querySelector('.fhir-section-summary').textContent = e.target.checked ? 'HTTPS enabled' : 'HTTP only';
            this.onChange();
        });
        tlsBody.querySelectorAll('input').forEach(el => el.addEventListener('input', () => this.onChange()));
        tlsSec.querySelector('.fhir-section-summary').textContent = tlsEnabled ? 'HTTPS enabled' : 'HTTP only';
        container.appendChild(tlsSec);

        // ── IP Allowlist ──────────────────────────────────────────────────
        const allowedIPs = Array.isArray(cfg.allowedIPs) ? cfg.allowedIPs.join('\n') : '';
        const ipSec      = this._makeFHIRSection('IP Allowlist', 'fas fa-shield-alt', false);
        const ipBody     = ipSec.querySelector('.fhir-section-body');

        ipBody.innerHTML = `
            <div class="fhir-field">
                <label class="fhir-field-label">Allowed IPs / CIDRs</label>
                <textarea class="form-control form-control-sm" id="fhirAllowedIPs"
                          rows="4" placeholder="Leave empty to allow all&#10;192.168.1.0/24&#10;10.0.0.5"
                          style="font-family:monospace;font-size:12px">${esc(allowedIPs)}</textarea>
                <small class="fhir-hint">One IP or CIDR per line. Empty = accept from anywhere.</small>
            </div>
        `;

        ipBody.querySelector('#fhirAllowedIPs').addEventListener('input', () => {
            const count = ipBody.querySelector('#fhirAllowedIPs').value.trim().split(/\s+/).filter(Boolean).length;
            ipSec.querySelector('.fhir-section-summary').textContent = count ? `${count} rule${count > 1 ? 's' : ''}` : 'Open';
            this.onChange();
        });
        ipSec.querySelector('.fhir-section-summary').textContent = allowedIPs.trim() ?
            `${allowedIPs.trim().split(/\s+/).filter(Boolean).length} rule(s)` : 'Open';
        container.appendChild(ipSec);
    }

    // ── FHIR OUTBOUND ────────────────────────────────────────────────────────

    _buildFHIROutboundUI(container) {
        const cfg = this.config.config || {};
        const esc = s => this.escapeHtml(String(s ?? ''));

        container.innerHTML = '';
        const banner = this._buildFHIRSchemaBanner();
        if (banner) container.appendChild(banner);

        const FHIR_R4_TYPES = [
            'AllergyIntolerance','Appointment','Bundle','CarePlan','CareTeam',
            'Claim','ClaimResponse','Communication','Composition','Condition',
            'Coverage','DiagnosticReport','DocumentReference','Encounter',
            'EpisodeOfCare','ExplanationOfBenefit','Flag','Goal','Group',
            'HealthcareService','ImagingStudy','Immunization','List','Location',
            'Medication','MedicationAdministration','MedicationDispense',
            'MedicationRequest','MessageHeader','Observation','Organization',
            'Parameters','Patient','Practitioner','PractitionerRole','Procedure',
            'RelatedPerson','Schedule','ServiceRequest','Slot','Specimen','Task',
        ];

        const currSendMode = cfg.resourceType === 'Bundle' && cfg.bundleType === 'transaction' ? 'transaction'
            : cfg.resourceType === 'Bundle' && cfg.bundleType === 'batch' ? 'batch'
            : (cfg.resourceType && cfg.resourceType !== 'Bundle') ? 'specific'
            : 'auto';

        // ── Destination ───────────────────────────────────────────────────
        const destSec  = this._makeFHIRSection('Destination', 'fas fa-location-arrow', true);
        const destBody = destSec.querySelector('.fhir-section-body');

        destBody.innerHTML = `
            <div class="fhir-field">
                <label class="fhir-field-label">FHIR Server URL <span class="fhir-required">*</span></label>
                <input type="text" class="form-control form-control-sm connector-config-field"
                       data-field="baseUrl" value="${esc(cfg.baseUrl || '')}"
                       placeholder="https://fhir.yourserver.com/fhir/R4"
                       autocomplete="off" spellcheck="false">
                <div style="display:flex;align-items:center;gap:8px;margin-top:6px">
                    <select class="form-control form-control-sm connector-config-field"
                            data-field="fhirVersion" style="max-width:150px" id="fhirOutboundVersionSelect">
                        <option value="${esc(cfg.fhirVersion || 'R4')}" selected>${esc(cfg.fhirVersion || 'R4')}</option>
                    </select>
                    <small class="fhir-hint">Sent in Accept &amp; Content-Type headers</small>
                </div>
            </div>

            <div class="fhir-field" style="margin-top:14px">
                <label class="fhir-field-label">What are you sending?</label>
                <div class="fhir-send-mode-grid" id="fhirSendModeGrid">
                    <label class="fhir-send-card" data-mode="auto">
                        <input type="radio" name="fhir-send-mode" value="auto">
                        <div class="fhir-send-card-body">
                            <div class="fhir-send-card-icon"><i class="fas fa-magic"></i></div>
                            <div>
                                <div class="fhir-send-card-title">Auto-detect</div>
                                <div class="fhir-send-card-sub">Reads resourceType from message body — ideal for HL7→FHIR pipelines</div>
                            </div>
                        </div>
                    </label>
                    <label class="fhir-send-card" data-mode="specific">
                        <input type="radio" name="fhir-send-mode" value="specific">
                        <div class="fhir-send-card-body">
                            <div class="fhir-send-card-icon"><i class="fas fa-bullseye"></i></div>
                            <div>
                                <div class="fhir-send-card-title">Specific resource type</div>
                                <div class="fhir-send-card-sub">Always route to one resource endpoint</div>
                            </div>
                        </div>
                    </label>
                    <label class="fhir-send-card" data-mode="transaction">
                        <input type="radio" name="fhir-send-mode" value="transaction">
                        <div class="fhir-send-card-body">
                            <div class="fhir-send-card-icon"><i class="fas fa-layer-group"></i></div>
                            <div>
                                <div class="fhir-send-card-title">Transaction bundle</div>
                                <div class="fhir-send-card-sub">Atomic — all-or-nothing commit</div>
                            </div>
                        </div>
                    </label>
                    <label class="fhir-send-card" data-mode="batch">
                        <input type="radio" name="fhir-send-mode" value="batch">
                        <div class="fhir-send-card-body">
                            <div class="fhir-send-card-icon"><i class="fas fa-stream"></i></div>
                            <div>
                                <div class="fhir-send-card-title">Batch bundle</div>
                                <div class="fhir-send-card-sub">Independent requests — partial success allowed</div>
                            </div>
                        </div>
                    </label>
                </div>
            </div>

            <div id="fhirSpecificResourceRow" style="display:none;margin-top:10px">
                <label class="fhir-field-label">Resource Type</label>
                <select class="form-control form-control-sm" id="fhirOutboundResourceTypeSelect">
                    <option value="">— Select resource type —</option>
                    ${FHIR_R4_TYPES.map(t => `<option value="${t}" ${cfg.resourceType === t ? 'selected' : ''}>${t}</option>`).join('')}
                    <option value="_custom_">✎  Other (type manually)…</option>
                </select>
                <input type="text" class="form-control form-control-sm" id="fhirResourceTypeCustomInput"
                       placeholder="e.g. MolecularSequence" style="display:none;margin-top:4px">
            </div>

            <input type="hidden" class="connector-config-field" data-field="resourceType" id="fhirResourceTypeHidden" value="${esc(cfg.resourceType || '')}">
            <input type="hidden" class="connector-config-field" data-field="bundleType"   id="fhirBundleTypeHidden"   value="${esc(cfg.bundleType || '')}">
            <input type="hidden" class="connector-config-field" data-field="method"       id="fhirMethodHidden"       value="${esc(cfg.method || 'POST')}">

            <div class="fhir-url-preview-card" id="fhirOutboundUrlPreview" style="margin-top:16px">
                <div class="fhir-url-preview-verb fhir-verb-post" id="fhirUrlVerb">POST</div>
                <div class="fhir-url-preview-url" id="fhirUrlText" style="font-style:italic;opacity:0.6">/{baseUrl}/&lt;resourceType&gt;</div>
                <button type="button" class="fhir-url-copy-btn" id="fhirUrlCopy" title="Copy URL">
                    <i class="fas fa-copy"></i>
                </button>
            </div>
            <div class="fhir-url-preview-note" id="fhirUrlNote">resourceType auto-detected from message body at runtime</div>
        `;

        container.appendChild(destSec);

        // Populate FHIR version select dynamically from installed schemas
        this._loadFHIRVersions(destBody.querySelector('#fhirOutboundVersionSelect'), cfg.fhirVersion || 'R4');

        // Wire send-mode cards
        const sendModeGrid    = destBody.querySelector('#fhirSendModeGrid');
        const rtHidden        = destBody.querySelector('#fhirResourceTypeHidden');
        const btHidden        = destBody.querySelector('#fhirBundleTypeHidden');
        const methodHidden    = destBody.querySelector('#fhirMethodHidden');
        const specificRow     = destBody.querySelector('#fhirSpecificResourceRow');
        const rtSelect        = destBody.querySelector('#fhirOutboundResourceTypeSelect');
        const rtCustom        = destBody.querySelector('#fhirResourceTypeCustomInput');

        const applyMode = (mode) => {
            sendModeGrid.querySelectorAll('.fhir-send-card').forEach(c => {
                c.classList.toggle('selected', c.dataset.mode === mode);
                const r = c.querySelector('input[type="radio"]');
                if (r) r.checked = c.dataset.mode === mode;
            });
            specificRow.style.display = mode === 'specific' ? '' : 'none';
            if (mode === 'auto')        { rtHidden.value = '';        btHidden.value = '';             methodHidden.value = 'POST'; }
            else if (mode === 'specific'){ rtHidden.value = rtSelect.value === '_custom_' ? rtCustom.value.trim() : (rtSelect.value || ''); btHidden.value = ''; methodHidden.value = 'POST'; }
            else if (mode === 'transaction') { rtHidden.value = 'Bundle'; btHidden.value = 'transaction'; methodHidden.value = 'POST'; }
            else if (mode === 'batch')   { rtHidden.value = 'Bundle'; btHidden.value = 'batch';        methodHidden.value = 'POST'; }
            this._refreshFHIROutboundPreview(container);
            this.onChange();
        };

        sendModeGrid.querySelectorAll('.fhir-send-card').forEach(card => {
            card.addEventListener('click', () => applyMode(card.dataset.mode));
        });

        rtSelect.addEventListener('change', () => {
            rtCustom.style.display = rtSelect.value === '_custom_' ? '' : 'none';
            if (rtSelect.value !== '_custom_') rtHidden.value = rtSelect.value;
            this._refreshFHIROutboundPreview(container);
            this.onChange();
        });
        rtCustom.addEventListener('input', () => {
            rtHidden.value = rtCustom.value.trim();
            this._refreshFHIROutboundPreview(container);
            this.onChange();
        });

        destBody.querySelector('#fhirUrlCopy').addEventListener('click', () => {
            const url = destBody.querySelector('#fhirUrlText')?.textContent || '';
            navigator.clipboard?.writeText(url).catch(() => {});
        });

        destBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => { this._refreshFHIROutboundPreview(container); this.onChange(); });
            el.addEventListener('change', () => { this._refreshFHIROutboundPreview(container); this.onChange(); });
        });

        applyMode(currSendMode);

        // ── Authentication ────────────────────────────────────────────────
        const authSec  = this._makeFHIRSection('Authentication', 'fas fa-lock', false);
        const authBody = authSec.querySelector('.fhir-section-body');
        const authType = cfg.authType || 'none';

        authBody.innerHTML = `
            <div class="fhir-field">
                <label class="fhir-field-label">Authentication method</label>
                <select class="form-control form-control-sm connector-config-field"
                        data-field="authType" id="fhirOutboundAuthSelect">
                    <option value="none"    ${authType === 'none'    ? 'selected' : ''}>No authentication</option>
                    <option value="basic"   ${authType === 'basic'   ? 'selected' : ''}>Username &amp; Password</option>
                    <option value="bearer"  ${authType === 'bearer'  ? 'selected' : ''}>Bearer Token</option>
                    <option value="smart"   ${authType === 'smart'   ? 'selected' : ''}>SMART on FHIR</option>
                    <option value="api_key" ${authType === 'api_key' ? 'selected' : ''}>API Key</option>
                </select>
                <small class="fhir-hint" id="fhirAuthHint"></small>
            </div>
            <div id="fhirOutboundAuthCreds" style="margin-top:10px">
                <div class="fhir-auth-cred" data-auth="basic">
                    <label class="fhir-field-label">Username</label>
                    <input type="text" class="form-control form-control-sm connector-config-field"
                           data-field="username" value="${esc(cfg.username || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="basic">
                    <label class="fhir-field-label" style="margin-top:8px">Password</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="password" value="${esc(cfg.password || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="bearer smart">
                    <label class="fhir-field-label" style="margin-top:8px">Bearer Token</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="bearerToken" value="${esc(cfg.bearerToken || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="smart" style="margin-top:4px">
                    <small class="fhir-hint" style="color:#1d4ed8">
                        SMART on FHIR — OAuth2 for Epic, Cerner, and Azure FHIR certified systems
                    </small>
                </div>
                <div class="fhir-auth-cred" data-auth="api_key">
                    <label class="fhir-field-label" style="margin-top:8px">API Key</label>
                    <input type="password" class="form-control form-control-sm connector-config-field"
                           data-field="apiKey" value="${esc(cfg.apiKey || '')}">
                </div>
                <div class="fhir-auth-cred" data-auth="api_key">
                    <label class="fhir-field-label" style="margin-top:8px">Header name</label>
                    <input type="text" class="form-control form-control-sm connector-config-field"
                           data-field="apiKeyHeader" value="${esc(cfg.apiKeyHeader || 'X-API-Key')}">
                </div>
            </div>
        `;

        const AUTH_HINTS = {
            none: '', basic: '',
            bearer: 'Sent as: Authorization: Bearer {token}',
            smart:  'Obtain a token from your system\'s OAuth2 token endpoint, paste it as the Bearer Token above',
            api_key: 'Sent as a custom HTTP header (default: X-API-Key)',
        };
        const AUTH_LABELS_OUT = { none: 'None', basic: 'Basic auth', bearer: 'Bearer token', smart: 'SMART on FHIR', api_key: 'API key' };
        const applyOutboundAuth = (type) => {
            authSec.querySelector('.fhir-section-summary').textContent = AUTH_LABELS_OUT[type] || type;
            authBody.querySelector('#fhirAuthHint').textContent = AUTH_HINTS[type] || '';
            authBody.querySelectorAll('.fhir-auth-cred').forEach(el => {
                const auths = (el.dataset.auth || '').split(' ');
                el.style.display = auths.includes(type) ? '' : 'none';
            });
        };
        applyOutboundAuth(authType);
        authBody.querySelector('#fhirOutboundAuthSelect').addEventListener('change', e => { applyOutboundAuth(e.target.value); this.onChange(); });
        authBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => this.onChange());
            el.addEventListener('change', () => this.onChange());
        });
        container.appendChild(authSec);

        // ── Operations ────────────────────────────────────────────────────
        const opsSec  = this._makeFHIRSection('Operations', 'fas fa-bolt', false);
        const opsBody = opsSec.querySelector('.fhir-section-body');
        const currOp  = cfg.operation || '';

        const FHIR_OPS = [
            { v: '',               l: 'Standard CRUD',    sub: 'POST / PUT / GET / DELETE on resources',             icon: 'fa-exchange-alt' },
            { v: '$validate',      l: 'Validate',          sub: 'Validate a resource against profiles ($validate)',   icon: 'fa-check-circle' },
            { v: '$process-message', l: 'Process message', sub: 'Send a FHIR Message Bundle ($process-message)',      icon: 'fa-envelope' },
            { v: '$everything',    l: 'Everything',        sub: 'Fetch all related data for a resource ($everything)',icon: 'fa-globe' },
            { v: '$match',         l: 'Patient match',     sub: 'Match patients across systems ($match)',             icon: 'fa-user-check' },
            { v: '$export',        l: 'Bulk export',       sub: 'SMART bulk data export ($export)',                   icon: 'fa-download' },
            { v: '_custom_',       l: 'Custom operation',  sub: 'Type any $operation manually',                      icon: 'fa-code' },
        ];

        const knownOps   = FHIR_OPS.map(o => o.v).filter(v => v !== '_custom_');
        const isCustomOp = currOp !== '' && !knownOps.includes(currOp);
        const activeOpV  = isCustomOp ? '_custom_' : currOp;

        opsBody.innerHTML = `
            <div class="fhir-ops-list" id="fhirOpsList">
                ${FHIR_OPS.map(op => `
                    <label class="fhir-ops-item" data-op="${esc(op.v)}">
                        <input type="radio" name="fhir-operation" value="${esc(op.v)}"
                               ${activeOpV === op.v ? 'checked' : ''}>
                        <i class="fas ${op.icon} fhir-ops-icon"></i>
                        <div>
                            <div class="fhir-ops-title">${esc(op.l)}</div>
                            <div class="fhir-ops-sub">${esc(op.sub)}</div>
                        </div>
                    </label>
                `).join('')}
            </div>
            <div id="fhirCustomOpRow" style="display:${isCustomOp ? '' : 'none'};margin-top:8px">
                <input type="text" class="form-control form-control-sm" id="fhirCustomOpInput"
                       placeholder="e.g. $myCustomOp" value="${esc(isCustomOp ? currOp : '')}">
            </div>
            <input type="hidden" class="connector-config-field" data-field="operation"
                   id="fhirOperationHidden" value="${esc(currOp)}">
            <div style="margin-top:12px">
                <label class="fhir-field-label">
                    Resource ID
                    <small style="font-weight:400;color:#94a3b8"> — optional, for instance-level operations</small>
                </label>
                <input type="text" class="form-control form-control-sm connector-config-field"
                       data-field="resourceId" value="${esc(cfg.resourceId || '')}"
                       placeholder="Leave empty for create / type-level operations">
            </div>
        `;

        const opHidden      = opsBody.querySelector('#fhirOperationHidden');
        const customOpRow   = opsBody.querySelector('#fhirCustomOpRow');
        const customOpInput = opsBody.querySelector('#fhirCustomOpInput');

        opsBody.querySelectorAll('input[name="fhir-operation"]').forEach(radio => {
            radio.addEventListener('change', () => {
                customOpRow.style.display = radio.value === '_custom_' ? '' : 'none';
                opHidden.value = radio.value === '_custom_' ? customOpInput.value.trim() : radio.value;
                this._refreshFHIROutboundPreview(container);
                this.onChange();
            });
        });
        customOpInput.addEventListener('input', () => {
            opHidden.value = customOpInput.value.trim();
            this._refreshFHIROutboundPreview(container);
            this.onChange();
        });
        opsBody.querySelector('.connector-config-field[data-field="resourceId"]').addEventListener('input', () => {
            this._refreshFHIROutboundPreview(container);
            this.onChange();
        });
        container.appendChild(opsSec);

        // ── Reliability ───────────────────────────────────────────────────
        const relSec  = this._makeFHIRSection('Reliability', 'fas fa-sync-alt', false);
        const relBody = relSec.querySelector('.fhir-section-body');

        relBody.innerHTML = `
            <div class="fhir-field-row">
                <div class="fhir-field">
                    <label class="fhir-field-label">Retry attempts</label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="retryAttempts" data-field-type="number"
                           value="${esc(cfg.retryAttempts ?? 3)}" min="0" max="10">
                    <small class="fhir-hint">0 = no retry</small>
                </div>
                <div class="fhir-field">
                    <label class="fhir-field-label">Retry delay (s)</label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="retryDelay" data-field-type="number"
                           value="${esc(cfg.retryDelay ?? 1)}" min="0">
                </div>
                <div class="fhir-field">
                    <label class="fhir-field-label">Timeout (s)</label>
                    <input type="number" class="form-control form-control-sm connector-config-field"
                           data-field="timeout" data-field-type="number"
                           value="${esc(cfg.timeout || 30)}" min="1" max="300">
                </div>
            </div>
            <div class="fhir-field" style="margin-top:10px">
                <label class="fhir-field-label">Extra query parameters</label>
                <input type="text" class="form-control form-control-sm connector-config-field"
                       data-field="queryParams" value="${esc(cfg.queryParams || '')}"
                       placeholder="_format=json&amp;_pretty=true">
                <small class="fhir-hint">Appended to every request URL</small>
            </div>
        `;

        relBody.querySelectorAll('.connector-config-field').forEach(el => {
            el.addEventListener('input', () => { this._refreshFHIROutboundPreview(container); this.onChange(); });
            el.addEventListener('change', () => { this._refreshFHIROutboundPreview(container); this.onChange(); });
        });
        container.appendChild(relSec);

        this._refreshFHIROutboundPreview(container);
    }

    /** Mirrors fhir_utils.BuildFHIRURL — keep in sync with Go implementation. */
    _refreshFHIROutboundPreview(container) {
        const verbEl = container.querySelector('#fhirUrlVerb');
        const urlEl  = container.querySelector('#fhirUrlText');
        const noteEl = container.querySelector('#fhirUrlNote');
        if (!verbEl || !urlEl) return;

        const baseUrl      = (container.querySelector('.connector-config-field[data-field="baseUrl"]')?.value || '').replace(/\/+$/, '');
        const resourceType = container.querySelector('#fhirResourceTypeHidden')?.value || container.querySelector('.connector-config-field[data-field="resourceType"]')?.value || '';
        const bundleType   = container.querySelector('#fhirBundleTypeHidden')?.value   || container.querySelector('.connector-config-field[data-field="bundleType"]')?.value   || '';
        const resourceId   = (container.querySelector('.connector-config-field[data-field="resourceId"]')?.value || '').trim();
        const operation    = (container.querySelector('#fhirOperationHidden')?.value   || container.querySelector('.connector-config-field[data-field="operation"]')?.value   || '').trim();

        let op = operation;
        if (op && !op.startsWith('$')) op = '$' + op;

        let url, verb = 'POST', note = '';

        if (!resourceType) {
            const base = baseUrl || '/{baseUrl}';
            url  = `${base}/<resourceType>`;
            note = 'resourceType auto-detected from message body at runtime';
        } else if (resourceType === 'Bundle' && bundleType === 'transaction') {
            url  = baseUrl || '/';
            note = 'FHIR transaction bundle — atomic, all-or-nothing commit';
        } else if (resourceType === 'Bundle' && bundleType === 'batch') {
            url  = baseUrl || '/';
            note = 'FHIR batch bundle — independent requests, partial success allowed';
        } else {
            url = `${baseUrl}/${resourceType}`;
            if (resourceId) { url += `/${resourceId}`; verb = op ? 'POST' : 'PUT'; }
            if (op)         { url += `/${op}`; note = resourceId ? `Instance-level: ${op}` : `Type-level: ${op}`; }
            else            { note = resourceId ? `Update ${resourceType}` : `Create ${resourceType}`; }
        }

        const isRuntime = !resourceType;
        verbEl.textContent  = verb;
        verbEl.className    = `fhir-url-preview-verb fhir-verb-${verb.toLowerCase()}`;
        urlEl.textContent   = url;
        urlEl.style.opacity   = isRuntime ? '0.55' : '1';
        urlEl.style.fontStyle = isRuntime ? 'italic' : 'normal';
        if (noteEl) noteEl.textContent = note;
    }

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
