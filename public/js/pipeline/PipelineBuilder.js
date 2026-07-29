/**
 * Pipeline Builder - Main Orchestrator
 * Coordinates all managers and handles pipeline operations
 */

class PipelineBuilder {
    constructor() {
        this.pipeline = null;
        this.interfaceId = null;
        this.messageType = null;
        this.isSaved = true;

        // Initialize managers
        this.dragDropManager = null;
        this.canvasRenderer = null;
        this.stepNodeManager = null;
        this.toolboxManager = null;
        this.propertiesPanel = null;
        this.layerContainer = null;
        this.flowchartRenderer = null;

        // View mode state
        this.viewMode = 'flowchart'; // Default to flowchart (list disabled)

        this.init();
    }

    async init() {
        // Parse URL parameters
        this.parseURLParams();

        // Initialize managers
        this.initializeManagers();

        // Load or create pipeline
        await this.loadPipeline();

        // Update header now that all async data (interface name, message type) is resolved.
        // Must run here because flowchart mode calls renderFlowchart() not renderPipeline(),
        // so updateHeaderInfo() inside renderPipeline() is never reached in the default view.
        this.updateHeaderInfo();

        // Setup UI event listeners
        this.setupEventListeners();

        // Setup auto-save
        this.setupAutoSave();

        console.log('Pipeline Builder initialized');
    }

    /**
     * Parse URL parameters
     */
    parseURLParams() {
        const params = new URLSearchParams(window.location.search);
        this.interfaceId    = params.get('interfaceId');
        this.messageType    = params.get('messageType');
        const pipelineId = params.get('pipelineId');

        if (pipelineId) {
            this.pipelineId = pipelineId;
        }

        // Update header info
        this.updateHeaderInfo();
    }

    /**
     * Initialize all managers
     */
    initializeManagers() {
        this.dragDropManager = new DragDropManager(this);
        this.canvasRenderer = new CanvasRenderer(this);
        this.stepNodeManager = new StepNodeManager(this);
        this.propertiesPanel = new PropertiesPanel(this);
        this.layerContainer = new LayerContainer(this);
        this.toolboxManager = new ToolboxManager(this);

        // Initialize flowchart renderer - V2 (horizontal swim lanes)
        const canvasWrapper = document.getElementById('canvasWrapper');
        if (canvasWrapper) {
            // Toggle between V1 and V2 here
            const useV2 = true; // Set to true to use new horizontal layout

            if (useV2) {
                this.flowchartRenderer = new FlowchartOrchestratorV2(canvasWrapper, this);
                console.log('✅ Using Flowchart V2 (Horizontal Swim Lanes)');
            } else {
                this.flowchartRenderer = new FlowchartRenderer(canvasWrapper, this);
                console.log('✅ Using Flowchart V1 (Vertical Layout)');
            }
        }

        // Load view mode preference
        this.loadViewModePreference();

        // Make PropertiesPanel globally accessible for row click handlers
        window.propertiesPanel = this.propertiesPanel;
    }

    /**
     * Load or create pipeline
     */
    async loadPipeline() {
        try {
            if (this.pipelineId) {
                // Load existing pipeline by ID
                this.pipeline = await window.pipelineAPI.loadPipeline(this.pipelineId);
                this.interfaceId = this.pipeline.interfaceId;
                this.messageType = this.pipeline.messageType;
                // Fetch interface name (not included in the pipeline payload)
                if (this.interfaceId && !this.interfaceName) {
                    try {
                        const ifaceResp = await fetch(`/api/interfaces/${this.interfaceId}`);
                        if (ifaceResp.ok) {
                            const ifaceData = await ifaceResp.json();
                            const ifaceObj  = ifaceData.data || ifaceData.interface || ifaceData;
                            this.interfaceName = ifaceObj.name || ifaceObj.interface_name || null;
                        }
                    } catch (_) { /* non-critical — header will fall back to pipeline name */ }
                }
            } else if (this.interfaceId) {
                // Always fetch interface data to get name + resolve message type
                console.log('📡 Loading interface data...');
                const interfaceResponse = await fetch(`/api/interfaces/${this.interfaceId}`);
                if (interfaceResponse.ok) {
                    const interfaceData = await interfaceResponse.json();
                    // wizard route returns { success, data: {...} }
                    // interfaces route returns { success, interface: {...} }
                    const ifaceObj = interfaceData.data || interfaceData.interface || interfaceData;
                    // Store name for header
                    this.interfaceName = ifaceObj.name || ifaceObj.interface_name || null;
                    // Resolve message type only if not already set from URL params
                    if (!this.messageType || this.messageType === 'hl7v2') {
                        const dbMessageType = ifaceObj.message_type || ifaceObj.messageType;
                        this.messageType = (dbMessageType && dbMessageType !== 'hl7v2')
                            ? dbMessageType
                            : 'ADT^A01';
                    }
                    console.log(`✅ Interface loaded: "${this.interfaceName}", messageType: ${this.messageType}`);
                } else {
                    console.warn('⚠️ Failed to load interface, defaulting to ADT^A01');
                    this.messageType = this.messageType || 'ADT^A01';
                }

                // Try to load existing pipeline for interface/message type
                this.pipeline = await window.pipelineAPI.loadPipelineByInterface(
                    this.interfaceId,
                    this.messageType
                );

                // Fallback: if not found by message type, pick the first pipeline for this interface
                // (handles cases where interface.message_type is NULL but pipeline was saved with a specific type)
                if (!this.pipeline) {
                    const fallback = await window.pipelineAPI.loadFirstPipelineForInterface(this.interfaceId);
                    if (fallback) {
                        this.pipeline = fallback;
                        this.messageType = fallback.messageType || this.messageType;
                        console.log(`✅ Pipeline found via fallback lookup (message_type: ${this.messageType})`);
                    }
                }

                if (!this.pipeline) {
                    // Create new pipeline
                    this.pipeline = new VisualPipeline({
                        interfaceId: this.interfaceId,
                        messageType: this.messageType,
                        name: `${this.messageType} Pipeline`
                    });
                }
            } else {
                // Create blank pipeline
                this.pipeline = new VisualPipeline({
                    name: 'New Pipeline'
                });
            }

            // Render in current view mode (flowchart by default)
            console.log('🔄 About to switch view mode to:', this.viewMode);
            this.switchViewMode(this.viewMode, true); // Force initial render
            this.isSaved = true;

        } catch (error) {
            console.error('Failed to load pipeline:', error);
            this.pipeline = new VisualPipeline({
                name: 'New Pipeline'
            });
            this.switchViewMode(this.viewMode, true); // Force initial render
        }
    }

    /**
     * Render pipeline to canvas
     */
    renderPipeline() {
        this.layerContainer.renderCanvas();
        this.updateHeaderInfo();

        // Redraw connections after render
        setTimeout(() => {
            this.canvasRenderer.redrawAllConnections();
        }, 100);

        // If flowchart mode is active, also re-render flowchart
        if (this.viewMode === 'flowchart') {
            console.log('🔄 Re-rendering flowchart after pipeline load');
            this.renderFlowchart();
        }
    }

    /**
     * Update header info
     */
    updateHeaderInfo() {
        // ── Breadcrumb ──────────────────────────────────────────────────────────
        const nameEl    = document.getElementById('breadcrumbInterfaceName');
        const ifaceLink = document.getElementById('breadcrumbInterface');
        const msgEl     = document.getElementById('breadcrumbMsgType');

        const displayName = this.interfaceName || (this.pipeline && this.pipeline.name) || 'Interface';
        if (nameEl)    nameEl.textContent  = displayName;
        if (ifaceLink && this.interfaceId) {
            ifaceLink.href  = `/interfaces.html?highlight=${encodeURIComponent(this.interfaceId)}`;
            ifaceLink.title = `Back to interface: ${displayName}`;
        }
        if (msgEl) {
            msgEl.textContent = this.messageType
                ? `${this.messageType} Pipeline`
                : 'Pipeline Builder';
        }

        // Update page title too
        document.title = displayName
            ? `${displayName} — Pipeline Builder`
            : 'Pipeline Builder';

        // ── Shortcut links (context-aware) ─────────────────────────────────────
        const msgShortcut = document.getElementById('shortcutMessages');
        if (msgShortcut) {
            if (this.interfaceId) {
                msgShortcut.href    = `/messages.html?interfaceId=${encodeURIComponent(this.interfaceId)}`;
                msgShortcut.title   = `View messages for: ${displayName}`;
                msgShortcut.style.display = '';
            } else {
                msgShortcut.style.display = 'none';
            }
        }

        // Legacy elements — keep backward-compat if anything else reads them
        const titleEl = document.getElementById('pipelineTitle');
        const infoEl  = document.getElementById('interfaceInfo');
        if (titleEl && this.pipeline) titleEl.textContent = displayName;
        if (infoEl  && this.messageType) {
            infoEl.textContent    = this.messageType;
            infoEl.style.display  = 'inline-block';
        }
    }

    /**
     * Setup event listeners
     */
    setupEventListeners() {
        // Back button
        const backBtn = document.getElementById('backBtn');
        if (backBtn) {
            backBtn.addEventListener('click', () => this.navigateBack());
        }

        // Save button
        const saveBtn = document.getElementById('savePipelineBtn');
        if (saveBtn) {
            saveBtn.addEventListener('click', () => this.savePipeline());
        }

        // Test button
        const testBtn = document.getElementById('testPipelineBtn');
        if (testBtn) {
            testBtn.addEventListener('click', () => this.openTestModal());
        }

        // View mode toggle (List vs Flowchart)
        const listViewBtn = document.getElementById('listViewBtn');
        const flowchartViewBtn = document.getElementById('flowchartViewBtn');

        if (listViewBtn) {
            listViewBtn.addEventListener('click', () => this.switchViewMode('list'));
        }

        if (flowchartViewBtn) {
            flowchartViewBtn.addEventListener('click', () => this.switchViewMode('flowchart'));
        }

        // Auto layout
        const autoLayoutBtn = document.getElementById('autoLayoutBtn');
        if (autoLayoutBtn) {
            autoLayoutBtn.addEventListener('click', () => this.canvasRenderer.autoLayout());
        }

        // Clear canvas
        const clearBtn = document.getElementById('clearCanvasBtn');
        if (clearBtn) {
            clearBtn.addEventListener('click', () => this.clearCanvas());
        }

        // Pipeline Settings button
        const settingsBtn = document.getElementById('pipelineSettingsBtn');
        if (settingsBtn) {
            settingsBtn.addEventListener('click', () => this.openPipelineSettings());
        }

        // Test modal
        this.setupTestModal();

        // Prevent accidental navigation
        window.addEventListener('beforeunload', (e) => {
            if (!this.isSaved) {
                e.preventDefault();
                e.returnValue = 'You have unsaved changes. Are you sure you want to leave?';
                return e.returnValue;
            }
        });
    }

    /**
     * Setup test modal
     */
    setupTestModal() {
        const modal = document.getElementById('testModal');
        const closeButtons = modal?.querySelectorAll('.modal-close');
        const runTestBtn = document.getElementById('runTestBtn');

        closeButtons?.forEach(btn => {
            btn.addEventListener('click', () => {
                modal.classList.remove('active');
            });
        });

        if (runTestBtn) {
            runTestBtn.addEventListener('click', () => this.runTest());
        }

        // Close on outside click
        modal?.addEventListener('click', (e) => {
            if (e.target === modal) {
                modal.classList.remove('active');
            }
        });
    }

    /**
     * Open Pipeline Settings modal for configuring pipeline-level defaults
     */
    openPipelineSettings() {
        const config = this.pipeline.pipelineConfig || {};
        const dr = config.defaultRetry || {};
        const deh = config.defaultErrorHandling || {};

        // Create modal if it doesn't exist
        let modal = document.getElementById('pipelineSettingsModal');
        if (modal) modal.remove();

        modal = document.createElement('div');
        modal.id = 'pipelineSettingsModal';
        modal.className = 'modal active';
        modal.innerHTML = `
            <div class="modal-content" style="max-width:560px;">
                <div class="modal-header">
                    <h3><i class="fas fa-cog" style="color:var(--primary-color);"></i> Pipeline Settings</h3>
                    <button class="modal-close">&times;</button>
                </div>
                <div class="modal-body">
                    <p style="color:var(--text-secondary);margin-bottom:16px;font-size:13px;">
                        Configure defaults that apply to <strong>all steps</strong> unless overridden at the step level.
                    </p>

                    <!-- Default Retry Section -->
                    <div style="margin-bottom:16px;border:2px solid var(--border-color);border-radius:8px;padding:16px;background:var(--bg-primary);">
                        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px;">
                            <h4 style="margin:0;color:var(--primary-color);font-size:14px;font-weight:600;">
                                <i class="fas fa-redo" style="color:var(--primary-light);margin-right:6px;"></i>Default Retry
                            </h4>
                            <label style="position:relative;display:inline-block;width:40px;height:22px;cursor:pointer;">
                                <input type="checkbox" id="ps-retryEnabled" ${dr.enabled ? 'checked' : ''}
                                    style="opacity:0;width:0;height:0;position:absolute;">
                                <span class="ps-toggle-track" style="position:absolute;cursor:pointer;top:0;left:0;right:0;bottom:0;
                                    background:${dr.enabled ? 'var(--primary-color)' : 'var(--border-hover)'};border-radius:22px;transition:.3s;">
                                    <span style="position:absolute;height:16px;width:16px;left:${dr.enabled ? '20px' : '3px'};bottom:3px;
                                        background:#fff;border-radius:50%;transition:.3s;box-shadow:0 1px 3px rgba(0,0,0,0.2);"></span>
                                </span>
                            </label>
                        </div>
                        <div id="ps-retryConfig" style="display:${dr.enabled ? 'block' : 'none'};">
                            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Max Retries</label>
                                    <input type="number" id="ps-maxRetries" value="${dr.maxRetries || 3}" min="1" max="10"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Delay (ms)</label>
                                    <input type="number" id="ps-delayMs" value="${dr.delayMs || 1000}" min="100" max="60000" step="100"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Backoff Multiplier</label>
                                    <input type="number" id="ps-backoffMultiplier" value="${dr.backoffMultiplier || 2.0}" min="1" max="5" step="0.5"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Max Delay (ms)</label>
                                    <input type="number" id="ps-maxDelayMs" value="${dr.maxDelayMs || 60000}" min="1000" max="300000" step="1000"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                            </div>
                        </div>
                    </div>

                    <!-- Default Error Handling Section -->
                    <div style="margin-bottom:16px;border:2px solid var(--border-color);border-radius:8px;padding:16px;background:var(--bg-primary);">
                        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:12px;">
                            <h4 style="margin:0;color:var(--primary-color);font-size:14px;font-weight:600;">
                                <i class="fas fa-shield-alt" style="color:var(--danger-color);margin-right:6px;"></i>Default Error Handling
                            </h4>
                            <label style="position:relative;display:inline-block;width:40px;height:22px;cursor:pointer;">
                                <input type="checkbox" id="ps-ehEnabled" ${deh.enabled ? 'checked' : ''}
                                    style="opacity:0;width:0;height:0;position:absolute;">
                                <span class="ps-toggle-track" style="position:absolute;cursor:pointer;top:0;left:0;right:0;bottom:0;
                                    background:${deh.enabled ? 'var(--danger-color)' : 'var(--border-hover)'};border-radius:22px;transition:.3s;">
                                    <span style="position:absolute;height:16px;width:16px;left:${deh.enabled ? '20px' : '3px'};bottom:3px;
                                        background:#fff;border-radius:50%;transition:.3s;box-shadow:0 1px 3px rgba(0,0,0,0.2);"></span>
                                </span>
                            </label>
                        </div>
                        <div id="ps-ehConfig" style="display:${deh.enabled ? 'block' : 'none'};">
                            <div style="margin-bottom:12px;">
                                <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">On Error</label>
                                <select id="ps-onError"
                                    style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);cursor:pointer;transition:border-color 0.2s;"
                                    onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                    onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                    <option value="catch" ${(deh.onError || 'catch') === 'catch' ? 'selected' : ''}>Catch (suppress & continue)</option>
                                    <option value="suppress" ${deh.onError === 'suppress' ? 'selected' : ''}>Suppress (ignore & continue)</option>
                                    <option value="rethrow" ${deh.onError === 'rethrow' ? 'selected' : ''}>Rethrow (stop pipeline)</option>
                                </select>
                            </div>
                            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Default Field (optional)</label>
                                    <input type="text" id="ps-defaultField" value="${deh.defaultField || ''}" placeholder="e.g. PID.3"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                                <div>
                                    <label style="color:var(--text-secondary);font-size:12px;font-weight:500;display:block;margin-bottom:4px;">Default Value (optional)</label>
                                    <input type="text" id="ps-defaultValue" value="${deh.defaultValue || ''}" placeholder="e.g. UNKNOWN"
                                        style="width:100%;padding:8px 10px;border:2px solid var(--border-color);border-radius:6px;font-size:13px;color:var(--text-primary);background:var(--bg-primary);transition:border-color 0.2s;"
                                        onfocus="this.style.borderColor='var(--primary-color)';this.style.boxShadow='0 0 0 3px rgba(30,58,138,0.1)'"
                                        onblur="this.style.borderColor='var(--border-color)';this.style.boxShadow='none'">
                                </div>
                            </div>
                        </div>
                    </div>

                    <div style="color:var(--text-secondary);font-size:12px;padding:10px 14px;background:var(--bg-tertiary);border-radius:6px;border-left:3px solid var(--primary-light);">
                        <i class="fas fa-info-circle" style="color:var(--primary-light);margin-right:4px;"></i>
                        Steps inherit these defaults unless they have their own config. Steps can opt-out by explicitly disabling retry or error handling.
                    </div>
                </div>
                <div class="modal-footer">
                    <button id="ps-cancelBtn" class="btn btn-secondary">Cancel</button>
                    <button id="ps-saveBtn" class="btn btn-primary">Apply Settings</button>
                </div>
            </div>
        `;
        document.body.appendChild(modal);

        // Wire up toggle switches
        const retryToggle = document.getElementById('ps-retryEnabled');
        const ehToggle = document.getElementById('ps-ehEnabled');
        const retryConfig = document.getElementById('ps-retryConfig');
        const ehConfig = document.getElementById('ps-ehConfig');

        const updateToggleVisual = (checkbox, track) => {
            const dot = track.querySelector('span');
            if (checkbox.checked) {
                track.style.background = checkbox.id.includes('retry') ? 'var(--primary-color)' : 'var(--danger-color)';
                dot.style.left = '20px';
            } else {
                track.style.background = 'var(--border-hover)';
                dot.style.left = '3px';
            }
        };

        retryToggle.addEventListener('change', () => {
            retryConfig.style.display = retryToggle.checked ? 'block' : 'none';
            updateToggleVisual(retryToggle, retryToggle.nextElementSibling);
        });

        ehToggle.addEventListener('change', () => {
            ehConfig.style.display = ehToggle.checked ? 'block' : 'none';
            updateToggleVisual(ehToggle, ehToggle.nextElementSibling);
        });

        // Close handlers
        modal.querySelector('.modal-close').addEventListener('click', () => modal.remove());
        document.getElementById('ps-cancelBtn').addEventListener('click', () => modal.remove());
        modal.addEventListener('click', (e) => { if (e.target === modal) modal.remove(); });

        // Save handler
        document.getElementById('ps-saveBtn').addEventListener('click', () => {
            const newConfig = {};

            if (retryToggle.checked) {
                newConfig.defaultRetry = {
                    enabled: true,
                    maxRetries: parseInt(document.getElementById('ps-maxRetries').value) || 3,
                    delayMs: parseInt(document.getElementById('ps-delayMs').value) || 1000,
                    backoffMultiplier: parseFloat(document.getElementById('ps-backoffMultiplier').value) || 2.0,
                    maxDelayMs: parseInt(document.getElementById('ps-maxDelayMs').value) || 60000
                };
            }

            if (ehToggle.checked) {
                newConfig.defaultErrorHandling = {
                    enabled: true,
                    onError: document.getElementById('ps-onError').value || 'catch',
                    defaultField: document.getElementById('ps-defaultField').value || '',
                    defaultValue: document.getElementById('ps-defaultValue').value || ''
                };
            }

            this.pipeline.pipelineConfig = newConfig;
            this.isSaved = false;
            this.updateAutoSaveStatus('unsaved');
            modal.remove();

            if (this.dragDropManager) {
                this.dragDropManager.showNotification('Pipeline settings updated. Save pipeline to persist.', 'success');
            }
        });
    }

    /**
     * Open test modal
     */
    openTestModal() {
        const modal = document.getElementById('testModal');
        if (modal) {
            // Reset test results when opening modal
            const resultsDiv = document.getElementById('testResults');
            const resultsContent = document.getElementById('testResultsContent');
            if (resultsDiv) resultsDiv.style.display = 'none';
            if (resultsContent) resultsContent.innerHTML = '';

            modal.classList.add('active');
        }
    }

    /**
     * Run pipeline test
     */
    async runTest() {
        const messageInput = document.getElementById('testMessageInput');
        const resultsDiv = document.getElementById('testResults');
        const resultsContent = document.getElementById('testResultsContent');

        if (!messageInput || !resultsDiv || !resultsContent) return;

        const sampleMessage = messageInput.value.trim();
        const formatSelect = document.getElementById('testMessageFormat');
        const messageFormat = formatSelect ? formatSelect.value : 'auto';

        if (!sampleMessage) {
            this.dragDropManager.showNotification('Please enter a sample message', 'warning');
            return;
        }

        try {
            resultsContent.innerHTML = '<p style="text-align: center;"><i class="fas fa-spinner fa-spin"></i> Running test...</p>';
            resultsDiv.style.display = 'block';

            const result = await window.pipelineAPI.testPipeline(this.pipeline, sampleMessage, messageFormat);

            // Cache test output so IntelliSense can walk runtime step variables
            // without requiring a re-test when the properties panel opens.
            // Also persist the sample message so silent background refreshes work
            // automatically (no-code UX: user never needs to re-open the test modal).
            if (result?.steps) {
                window.pipelineLastTestOutput = result;
                console.log('[PipelineBuilder] Cached test output for IntelliSense:', Object.keys(result.steps));
                // Persist both the message and the step output so the path picker
                // works across page reloads with no user action required.
                try {
                    localStorage.setItem('pipeline_last_sample_message', sampleMessage);
                    localStorage.setItem('pipeline_last_test_output', JSON.stringify(result));
                } catch (_) {}
            }

            // Display results with enhanced FHIR resource rendering
            resultsContent.innerHTML = this.renderTestResults(result);
            this.loadCDAComplianceReport();

            // Check success field from test response
            const isSuccess = result.success === true;
            this.dragDropManager.showNotification(
                isSuccess ? 'Test passed' : 'Test failed',
                isSuccess ? 'success' : 'error'
            );

        } catch (error) {
            console.error('Test execution error:', error);

            // Extract meaningful error message
            let errorMessage = 'Unknown error occurred';
            if (error.message) {
                errorMessage = error.message;
            } else if (typeof error === 'string') {
                errorMessage = error;
            } else if (error.error) {
                errorMessage = error.error;
            } else {
                errorMessage = JSON.stringify(error, null, 2);
            }

            resultsContent.innerHTML = `
                <div class="test-result error">
                    <h4><i class="fas fa-times-circle"></i> Test Error</h4>
                    <p>${this.escapeHtml(errorMessage)}</p>
                    <details style="margin-top: 0.5rem;">
                        <summary style="cursor: pointer; font-size: 0.875rem;">View Error Details</summary>
                        <pre style="background: #fef2f2; padding: 0.5rem; border-radius: 0.25rem; overflow: auto; max-height: 300px; font-size: 0.75rem;">${this.escapeHtml(JSON.stringify(error, null, 2))}</pre>
                    </details>
                </div>
            `;
            this.dragDropManager.showNotification('Test failed: ' + errorMessage, 'error');
        }
    }

    /**
     * Render test results with proper FHIR resource display
     */
    renderTestResults(result) {
        // STANDARDIZED response format:
        // - input: MessageContext (what came in)
        // - output: MessageContext (what went out)
        // - steps: Object keyed by step name { "stepName": { step_output: {...}, step_metadata: { duration_ms, success } } }
        // - success: boolean

        const stepsExecuted = result.steps ? Object.keys(result.steps).length : 0;

        // Extract final output from MessageContext
        const finalOutput = result.output?.payload || result.output || {};

        const validationErrors = result.error ? [result.error] : [];

        // Per-step "warnings" (generic step_metadata.warnings[] -- any
        // executor can populate this via SetStepOutputWithDetails; the
        // cda.to_fhir executor is the first real producer, surfacing CDA
        // entries with more than one <value> sibling that only had its
        // first value mapped). Collected here, keyed by step, both for the
        // per-step badge below and the top-level "Validation Warnings"
        // panel (previously always empty -- this is the same array, just
        // now actually fed).
        const warningsByStep = {};
        const validationWarnings = [];
        if (result.steps) {
            for (const [stepName, stepData] of Object.entries(result.steps)) {
                const stepWarnings = stepData?.step_metadata?.warnings;
                if (Array.isArray(stepWarnings) && stepWarnings.length > 0) {
                    warningsByStep[stepName] = stepWarnings;
                    for (const w of stepWarnings) {
                        validationWarnings.push(`[${stepName}] ${w}`);
                    }
                }
            }
        }

        // Find FHIR bundle from steps
        let fhirBundle = null;
        let resourcesCreated = [];
        let mappingValidationErrors = [];

        if (result.steps) {
            // Check HL7→FHIR Transform step - using STANDARDIZED structure
            const transformStep = result.steps['hl7_fhir_transform'] || result.steps['hl7_to_fhir_transform'];
            if (transformStep?.step_output?.fhir_bundle) {
                fhirBundle = transformStep.step_output.fhir_bundle;
            }

            // Fallback: scan every step's output for a bundle. The CDA→FHIR
            // step (step type "cda.to_fhir") is keyed by the user's own step
            // name (e.g. "cda_fhir_transform"), not a fixed constant, so it
            // can never be matched by name like the legacy HL7 step above.
            // Its output key is "fhirBundle" (camelCase) too -- unlike other
            // step_output fields, NormalizeStepOutput deliberately leaves
            // this one un-snake_cased (models/output_normalizer.go) so the
            // FHIR resource tree's own field names (resourceType,
            // birthDate, ...) aren't mangled.
            if (!fhirBundle) {
                for (const stepData of Object.values(result.steps)) {
                    const bundle = stepData?.step_output?.fhirBundle || stepData?.step_output?.fhir_bundle;
                    if (bundle) {
                        fhirBundle = bundle;
                        break;
                    }
                }
            }

            // Check Field Mapping step - using STANDARDIZED structure
            const mappingStep = result.steps['field_mapping'];
            if (mappingStep?.step_output) {
                resourcesCreated = mappingStep.step_output.resources_created || [];
                mappingValidationErrors = mappingStep.step_output.validation_errors || [];
            }
        }

        // Also check final output for bundle (camelCase "fhirBundle" is what
        // cda_to_fhir_executor.go and hl7_fhir_transform_executor actually
        // write at the outputData root; "fhir_bundle" kept for safety).
        if (!fhirBundle && (finalOutput.fhirBundle || finalOutput.fhir_bundle)) {
            fhirBundle = finalOutput.fhirBundle || finalOutput.fhir_bundle;
        }

        // Also check if finalOutput IS a FHIR bundle
        if (!fhirBundle && finalOutput.resourceType === 'Bundle') {
            fhirBundle = finalOutput;
        }

        // Find a cda.build step's CDA XML output, for the live spec-compliance
        // panel below. Step output keys are snake_cased server-side
        // (models/output_normalizer.go), so — mirroring the fhirBundle
        // fallback-scan directly above — this content-sniffs rather than
        // assuming a literal "cdaXML" key.
        this._lastTestCdaXML = null;
        if (result.steps) {
            outer:
            for (const stepData of Object.values(result.steps)) {
                for (const val of Object.values(stepData?.step_output || {})) {
                    if (typeof val === 'string' && val.includes('<ClinicalDocument')) {
                        this._lastTestCdaXML = val;
                        break outer;
                    }
                }
            }
        }

        // Check if test passed
        const isSuccess = result.success === true;

        // Calculate total execution time from all steps - using STANDARDIZED structure
        let totalTimeMs = 0;
        if (result.steps) {
            totalTimeMs = Object.values(result.steps).reduce((sum, step) =>
                sum + (step.step_metadata?.duration_ms || step.duration_ms || 0), 0);
        }
        const executionTimeMs = totalTimeMs > 0 ? totalTimeMs : 'N/A';

        let html = `
            <div style="margin-bottom: 15px; text-align: right;">
                <button onclick="window.pipelineBuilder.runTest()" class="btn-secondary" style="padding: 8px 16px;">
                    <i class="fas fa-redo"></i> Run Test Again
                </button>
            </div>
            <div class="test-result ${isSuccess ? 'success' : 'error'}">
                <h4>
                    <i class="fas fa-${isSuccess ? 'check-circle' : 'times-circle'}"></i>
                    ${isSuccess ? 'Test Passed' : 'Test Failed'}
                </h4>
                <p><strong>Execution Time:</strong> ${executionTimeMs}ms</p>
                <p><strong>Steps Executed:</strong> ${stepsExecuted}</p>
                <p><strong>Status:</strong> ${result.status || 'unknown'}</p>
                ${result.errors?.length > 0 ? `<p class="error-message"><strong>Errors:</strong> ${result.errors.length} error(s) occurred</p>` : ''}
            </div>
        `;

        // Render per-step results with pass/fail indicators
        console.log('[TestResults] result keys:', Object.keys(result));
        console.log('[TestResults] typeof result.steps:', typeof result.steps);
        if (result.steps) {
            console.log('[TestResults] step names:', Object.keys(result.steps));
            const firstKey = Object.keys(result.steps)[0];
            if (firstKey) console.log('[TestResults] first step structure:', JSON.stringify(result.steps[firstKey], null, 2).substring(0, 500));
        } else {
            console.log('[TestResults] NO steps field! Full result keys:', JSON.stringify(result, null, 2).substring(0, 2000));
        }
        // Build error lookup from top-level errors array
        const errorsByStep = {};
        if (result.errors && Array.isArray(result.errors)) {
            for (const err of result.errors) {
                if (err.step && err.step !== '_pipeline') {
                    errorsByStep[err.step] = err;
                }
            }
        }

        if (result.steps && Object.keys(result.steps).length > 0) {
            const stepEntries = Object.entries(result.steps);
            const getSuccess = (s) => s.step_metadata?.success ?? s.success ?? true;
            const failedCount = stepEntries.filter(([name, s]) => !getSuccess(s) || (errorsByStep[name] && !errorsByStep[name].caught)).length;
            const caughtCount = stepEntries.filter(([name]) => errorsByStep[name]?.caught === true).length;
            const warningOnlyCount = stepEntries.filter(([name]) => warningsByStep[name] && !errorsByStep[name]).length;
            const passedCount = stepEntries.length - failedCount - caughtCount - warningOnlyCount;

            html += `
                <div class="step-results-section">
                    <h4 class="step-results-header">
                        <i class="fas fa-tasks"></i> Step Results
                        <span class="step-results-summary">
                            <span class="step-count-pass">${passedCount} passed</span>
                            ${caughtCount > 0 ? `<span style="color:var(--warning-color);font-weight:600;">${caughtCount} caught</span>` : ''}
                            ${warningOnlyCount > 0 ? `<span style="color:var(--warning-color);font-weight:600;">${warningOnlyCount} with warnings</span>` : ''}
                            ${failedCount > 0 ? `<span class="step-count-fail">${failedCount} failed</span>` : ''}
                        </span>
                    </h4>
                    <div class="step-results-list">
            `;

            for (const [stepName, stepData] of stepEntries) {
                const stepSuccess = getSuccess(stepData);
                const durationMs = stepData.step_metadata?.duration_ms ?? stepData.duration_ms;
                const duration = durationMs != null ? `${durationMs}ms` : '';
                const stepError = errorsByStep[stepName];
                const isCaughtError = stepError?.caught === true;
                const isFailedError = stepError && !stepError.caught;
                const stepWarnings = !stepError ? warningsByStep[stepName] : null;
                const hasWarningsOnly = Array.isArray(stepWarnings) && stepWarnings.length > 0;
                const statusClass = isFailedError ? 'step-fail' : (isCaughtError ? 'step-caught' : (hasWarningsOnly ? 'step-warning' : 'step-pass'));
                const statusIcon = isFailedError ? 'fa-times-circle' : (isCaughtError ? 'fa-shield-alt' : (hasWarningsOnly ? 'fa-exclamation-triangle' : 'fa-check-circle'));

                html += `
                    <div class="step-result-item ${statusClass}">
                        <div class="step-result-main">
                            <i class="fas ${statusIcon} step-result-icon"></i>
                            <span class="step-result-name">${this.escapeHtml(stepName)}</span>
                            ${isCaughtError ? '<span style="font-size:11px;background:var(--warning-color);color:#fff;padding:1px 6px;border-radius:4px;margin-left:6px;">CAUGHT</span>' : ''}
                            ${hasWarningsOnly ? `<span style="font-size:11px;background:var(--warning-color);color:#fff;padding:1px 6px;border-radius:4px;margin-left:6px;">${stepWarnings.length} WARNING${stepWarnings.length > 1 ? 'S' : ''}</span>` : ''}
                            ${duration ? `<span class="step-result-duration">${duration}</span>` : ''}
                        </div>
                        ${isFailedError ? `
                            <div class="step-result-error">
                                <i class="fas fa-exclamation-triangle"></i> ${this.escapeHtml(stepError.error)}
                            </div>
                        ` : ''}
                        ${isCaughtError ? `
                            <div class="step-result-error" style="color:var(--warning-color);background:rgba(245,158,11,0.08);border-left-color:var(--warning-color);">
                                <i class="fas fa-shield-alt"></i> ${this.escapeHtml(stepError.error)}
                                ${stepError.default_applied ? `<br><i class="fas fa-edit" style="margin-left:2px;"></i> Default: <code>${this.escapeHtml(stepError.default_applied.field)} = ${this.escapeHtml(String(stepError.default_applied.value))}</code>` : ''}
                            </div>
                        ` : ''}
                        ${hasWarningsOnly ? `
                            <div class="step-result-error" style="color:var(--warning-color);background:rgba(245,158,11,0.08);border-left-color:var(--warning-color);">
                                ${stepWarnings.map(w => `<div><i class="fas fa-exclamation-triangle"></i> ${this.escapeHtml(w)}</div>`).join('')}
                            </div>
                        ` : ''}
                    </div>
                `;
            }

            html += `
                    </div>
                </div>
            `;
        }

        // Render top-level errors summary (above FHIR output)
        if (result.errors && Array.isArray(result.errors) && result.errors.length > 0) {
            const uncaught = result.errors.filter(e => !e.caught);
            const caught = result.errors.filter(e => e.caught);
            html += `
                <div style="margin:16px 0;border-radius:8px;overflow:hidden;border:2px solid ${uncaught.length > 0 ? 'var(--danger-color)' : 'var(--warning-color)'};">
                    <div style="padding:10px 14px;background:${uncaught.length > 0 ? 'rgba(239,68,68,0.08)' : 'rgba(245,158,11,0.08)'};display:flex;align-items:center;gap:8px;">
                        <i class="fas fa-exclamation-circle" style="color:${uncaught.length > 0 ? 'var(--danger-color)' : 'var(--warning-color)'};"></i>
                        <strong style="font-size:14px;">Pipeline Errors (${result.errors.length})</strong>
                        ${caught.length > 0 ? `<span style="font-size:12px;color:var(--warning-color);margin-left:auto;">${caught.length} caught</span>` : ''}
                        ${uncaught.length > 0 ? `<span style="font-size:12px;color:var(--danger-color);margin-left:${caught.length > 0 ? '8px' : 'auto'};">${uncaught.length} uncaught</span>` : ''}
                    </div>
                    <div style="padding:0;">
            `;
            for (const err of result.errors) {
                if (err.step === '_pipeline') continue;
                const bg = err.caught ? 'rgba(245,158,11,0.04)' : 'rgba(239,68,68,0.04)';
                const borderColor = err.caught ? 'var(--warning-color)' : 'var(--danger-color)';
                const icon = err.caught ? 'fa-shield-alt' : 'fa-times-circle';
                const iconColor = err.caught ? 'var(--warning-color)' : 'var(--danger-color)';
                html += `
                    <div style="padding:10px 14px;border-top:1px solid var(--border-color);background:${bg};display:flex;gap:10px;align-items:flex-start;">
                        <i class="fas ${icon}" style="color:${iconColor};margin-top:2px;flex-shrink:0;"></i>
                        <div style="flex:1;min-width:0;">
                            <div style="display:flex;align-items:center;gap:8px;margin-bottom:4px;">
                                <strong style="font-size:13px;">${this.escapeHtml(err.step)}</strong>
                                <span style="font-size:11px;padding:1px 6px;border-radius:4px;color:#fff;background:${err.caught ? 'var(--warning-color)' : 'var(--danger-color)'};">
                                    ${err.caught ? 'CAUGHT' : 'FAILED'}
                                </span>
                                ${err.handler ? `<span style="font-size:11px;color:var(--text-tertiary);">handler: ${this.escapeHtml(err.handler)}</span>` : ''}
                            </div>
                            <div style="font-size:12px;color:var(--text-secondary);word-break:break-word;">${this.escapeHtml(err.error)}</div>
                            ${err.default_applied ? `
                                <div style="font-size:12px;margin-top:4px;color:var(--primary-light);">
                                    <i class="fas fa-edit"></i> Default applied: <code style="background:var(--bg-tertiary);padding:1px 4px;border-radius:3px;">${this.escapeHtml(err.default_applied.field)} = ${this.escapeHtml(String(err.default_applied.value))}</code>
                                </div>
                            ` : ''}
                        </div>
                    </div>
                `;
            }
            html += `
                    </div>
                </div>
            `;
        }

        // Show validation warnings prominently if any
        if (validationWarnings.length > 0) {
            html += this.renderValidationWarnings(validationWarnings);
        }

        // Show validation errors prominently if any
        const allErrors = [...validationErrors, ...mappingValidationErrors];
        if (allErrors.length > 0) {
            html += this.renderValidationErrors(allErrors);
        }

        // Render transformed message output (FHIR Bundle)
        if (fhirBundle) {
            html += this.renderTransformedMessage(fhirBundle);
        }

        // Placeholder for the CDA spec-compliance report -- populated
        // asynchronously by loadCDAComplianceReport() right after this HTML
        // is inserted into the DOM (renderTestResults itself is synchronous
        // and can't await the /api/cda/validate round trip inline).
        if (this._lastTestCdaXML) {
            html += `<div id="cdaComplianceReport" style="margin-top:1rem;"><p style="text-align:center;color:var(--text-tertiary);"><i class="fas fa-spinner fa-spin"></i> Checking C-CDA spec compliance...</p></div>`;
        }

        // Render FHIR resources with narratives
        if (resourcesCreated && Array.isArray(resourcesCreated) && resourcesCreated.length > 0) {
            html += this.renderFHIRResources(resourcesCreated);
        }

        // Copy buttons — only offer to copy an output that actually exists for
        // THIS pipeline (a CDA-only pipeline like cda.map_to_canonical +
        // cda.build has no FHIR bundle, and vice versa); "Copy Full Results"
        // (raw JSON) is always available regardless of pipeline type.
        html += `
            <div style="margin-top: 1rem; display: flex; gap: 0.5rem; flex-wrap: wrap;">
                ${fhirBundle ? `
                <button id="copyBundleBtn" class="btn-copy">
                    <i class="fas fa-copy"></i> Copy FHIR Bundle
                </button>` : ''}
                ${this._lastTestCdaXML ? `
                <button id="copyCdaXmlBtn" class="btn-copy">
                    <i class="fas fa-copy"></i> Copy CCD/CDA XML
                </button>` : ''}
                <button id="copyResultsBtn" class="btn btn-secondary">
                    <i class="fas fa-file-code"></i> Copy Full Results
                </button>
            </div>
        `;

        // Application details in collapsible section
        html += `
            <details style="margin-top: 1rem;">
                <summary style="cursor: pointer; font-weight: 600; color: #64748b;">
                    <i class="fas fa-cog"></i> Application Details
                </summary>
                <pre id="fullResultsJSON" style="background: #f1f5f9; padding: 1rem; border-radius: 0.375rem; overflow: auto; max-height: 400px; font-size: 0.75rem;">${this.escapeHtml(JSON.stringify(result, null, 2))}</pre>
            </details>
        `;

        // Setup copy buttons after rendering
        setTimeout(() => {
            const copyBundleBtn = document.getElementById('copyBundleBtn');
            if (copyBundleBtn) {
                copyBundleBtn.addEventListener('click', () => this.copyBundleToClipboard(fhirBundle));
            }

            const copyCdaXmlBtn = document.getElementById('copyCdaXmlBtn');
            if (copyCdaXmlBtn) {
                copyCdaXmlBtn.addEventListener('click', () => this.copyCdaXmlToClipboard(this._lastTestCdaXML));
            }

            const copyBtn = document.getElementById('copyResultsBtn');
            if (copyBtn) {
                copyBtn.addEventListener('click', () => this.copyResultsToClipboard(result));
            }
        }, 100);

        return html;
    }

    /**
     * Fetches a C-CDA 2.1 conformance report for the last test run's cda.build
     * output (this._lastTestCdaXML, set by renderTestResults) and injects it
     * into the #cdaComplianceReport placeholder. Reuses the existing, already-
     * live POST /api/cda/validate endpoint (build -> parse -> validate round
     * trip) -- zero new Go validation logic, pure UI composition.
     */
    async loadCDAComplianceReport() {
        const container = document.getElementById('cdaComplianceReport');
        if (!container || !this._lastTestCdaXML) return;

        try {
            const response = await fetch('/api/cda/validate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ xml: this._lastTestCdaXML }),
            });
            const data = await response.json();
            if (!response.ok || !data.success) {
                container.innerHTML = `<p style="color:var(--text-tertiary);font-size:0.85rem;">Could not check C-CDA spec compliance: ${this.escapeHtml(data.error || response.statusText)}</p>`;
                return;
            }
            container.innerHTML = this.renderComplianceReport(data.report);
        } catch (err) {
            container.innerHTML = `<p style="color:var(--text-tertiary);font-size:0.85rem;">Could not check C-CDA spec compliance: ${this.escapeHtml(err.message)}</p>`;
        }
    }

    /**
     * Renders a cda/validator.ComplianceReport (SHALL/SHOULD score, per-
     * section, per-field status) as a collapsible panel — the guided-
     * configuration UIs' (MapToCanonicalBuilder, CDABuildStepBuilder)
     * static/pre-population checklist is about SAVED CONFIG; this is the
     * same conformance rubric checked against a REAL built-and-reparsed
     * document from actual test data, the strongest completeness signal
     * available.
     */
    renderComplianceReport(report) {
        if (!report) return '';
        const pct = v => Math.round((v || 0) * 100);
        const scoreColor = v => v >= 1 ? 'var(--success-color, #16a34a)' : (v >= 0.7 ? 'var(--warning-color, #d97706)' : 'var(--danger-color, #dc2626)');

        const sectionRows = (report.sectionReports || []).map(sr => {
            const statusIcon = sr.status === 'missing' ? 'fa-times-circle' : (sr.status === 'present_empty' ? 'fa-exclamation-triangle' : 'fa-check-circle');
            const statusColor = sr.status === 'missing' ? 'var(--danger-color, #dc2626)' : (sr.status === 'present_empty' ? 'var(--warning-color, #d97706)' : 'var(--success-color, #16a34a)');
            const fieldRows = (sr.fieldReports || []).filter(fr => fr.status !== 'populated').map(fr =>
                `<div style="font-size:0.75rem;color:var(--text-tertiary);padding-left:1.5rem;">
                    <i class="fas fa-circle" style="font-size:0.4rem;vertical-align:middle;"></i>
                    ${this.escapeHtml(fr.fieldPath)} (${this.escapeHtml(fr.conformance)}) — ${this.escapeHtml(fr.status)}${fr.valueSummary ? ': ' + this.escapeHtml(fr.valueSummary) : ''}
                </div>`).join('');
            return `
                <div style="padding:0.4rem 0;border-bottom:1px solid var(--border-color, #e2e8f0);">
                    <div style="display:flex;align-items:center;gap:0.5rem;font-size:0.85rem;">
                        <i class="fas ${statusIcon}" style="color:${statusColor};"></i>
                        <strong>${this.escapeHtml(sr.title || sr.sectionKey)}</strong>
                        <span style="font-size:0.7rem;color:var(--text-tertiary);">${this.escapeHtml(sr.conformance)} · ${this.escapeHtml(sr.status)}${sr.entryCount ? ` · ${sr.entryCount} entries` : ''}</span>
                    </div>
                    ${fieldRows}
                </div>`;
        }).join('');

        return `
            <details class="transformed-message-section" style="margin-top:1rem;">
                <summary style="cursor:pointer;font-weight:600;display:flex;align-items:center;gap:0.5rem;">
                    <i class="fas fa-file-medical-alt"></i> C-CDA Spec Compliance — ${this.escapeHtml(report.documentType)}
                    <span style="font-size:0.8rem;font-weight:400;color:${scoreColor(report.shallScore)};">SHALL ${pct(report.shallScore)}%</span>
                    <span style="font-size:0.8rem;font-weight:400;color:${scoreColor(report.shouldScore)};">SHOULD ${pct(report.shouldScore)}%</span>
                </summary>
                <div style="margin-top:0.75rem;">${sectionRows}</div>
            </details>`;
    }

    /**
     * Render transformed FHIR message output
     */
    renderTransformedMessage(bundle) {
        if (!bundle) return '';

        return `
            <div class="transformed-message-section">
                <h5>
                    <i class="fas fa-exchange-alt"></i>
                    Transformed FHIR Bundle
                </h5>
                <div style="margin-bottom: 1rem; color: #64748b; font-size: 0.875rem;">
                    <strong>Type:</strong> ${bundle.type || 'N/A'} |
                    <strong>Resources:</strong> ${bundle.entry?.length || 0} |
                    <strong>Timestamp:</strong> ${bundle.timestamp || 'N/A'}
                </div>
                <div class="message-output">
                    <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word;">${this.escapeHtml(JSON.stringify(bundle, null, 2))}</pre>
                </div>
            </div>
        `;
    }

    /**
     * Render FHIR resources with proper HTML narrative display
     */
    renderFHIRResources(resources) {
        let html = `
            <div class="fhir-resources-section">
                <h5><i class="fas fa-file-medical"></i> FHIR Resources (${resources.length})</h5>
        `;

        resources.forEach((resource, index) => {
            const resourceType = resource.resourceType || 'Unknown';
            const resourceId = resource.id || 'N/A';

            html += `
                <details class="resource-card">
                    <summary>
                        <i class="fas fa-file-medical-alt"></i> ${resourceType} (ID: ${resourceId})
                    </summary>
                    <div class="resource-card-content">
            `;

            // Render narrative HTML if available
            if (resource.text && resource.text.div) {
                html += `
                    <div class="narrative-section">
                        <h6>Human-Readable Summary</h6>
                        <div class="narrative-content">
                            ${resource.text.div}
                        </div>
                    </div>
                `;
            }

            // Show resource JSON
            html += `
                <details style="margin-top: 0.5rem;">
                    <summary style="cursor: pointer; font-size: 0.875rem; color: #64748b;">
                        <i class="fas fa-code"></i> View Full ${resourceType} JSON
                    </summary>
                    <pre style="background: #f8fafc; padding: 0.75rem; border-radius: 0.25rem; overflow: auto; max-height: 300px; font-size: 0.75rem; margin-top: 0.5rem;">${this.escapeHtml(JSON.stringify(resource, null, 2))}</pre>
                </details>
            `;

            html += `
                    </div>
                </details>
            `;
        });

        html += `</div>`;
        return html;
    }

    /**
     * Render validation warnings (pipeline continues, message accepted)
     */
    renderValidationWarnings(warnings) {
        if (!warnings || warnings.length === 0) return '';

        let html = `
            <div class="validation-warnings-section" style="background: #fffbeb; border: 1px solid #fbbf24; border-radius: 0.5rem; padding: 1rem; margin: 1rem 0;">
                <h5 style="color: #b45309; margin: 0 0 0.75rem 0; display: flex; align-items: center; gap: 0.5rem;">
                    <i class="fas fa-exclamation-triangle"></i> Validation Warnings (${warnings.length})
                    <span style="font-size: 0.75rem; font-weight: normal; color: #92400e; background: #fef3c7; padding: 0.25rem 0.5rem; border-radius: 0.25rem;">Pipeline Continued (ACK Sent)</span>
                </h5>
                <ul style="margin: 0; padding-left: 1.5rem; color: #78350f;">
        `;

        warnings.forEach(warning => {
            html += `<li style="margin-bottom: 0.5rem;">${this.escapeHtml(warning)}</li>`;
        });

        html += `
                </ul>
                <p style="margin: 0.75rem 0 0 0; font-size: 0.875rem; color: #92400e;">
                    <i class="fas fa-info-circle"></i> These warnings indicate data quality issues but did not stop processing.
                    The message was accepted (ACK) and the pipeline continued successfully.
                </p>
            </div>
        `;

        return html;
    }

    /**
     * Render validation errors (pipeline stopped, message rejected)
     */
    renderValidationErrors(errors) {
        if (!errors || errors.length === 0) return '';

        let html = `
            <div class="validation-errors-section" style="background: #fef2f2; border: 1px solid #ef4444; border-radius: 0.5rem; padding: 1rem; margin: 1rem 0;">
                <h5 style="color: #991b1b; margin: 0 0 0.75rem 0; display: flex; align-items: center; gap: 0.5rem;">
                    <i class="fas fa-times-circle"></i> Validation Errors (${errors.length})
                    <span style="font-size: 0.75rem; font-weight: normal; color: #7f1d1d; background: #fee2e2; padding: 0.25rem 0.5rem; border-radius: 0.25rem;">Pipeline Stopped (NACK Sent)</span>
                </h5>
                <ul style="margin: 0; padding-left: 1.5rem; color: #7f1d1d;">
        `;

        errors.forEach(error => {
            html += `<li style="margin-bottom: 0.5rem;">${this.escapeHtml(error)}</li>`;
        });

        html += `
                </ul>
                <p style="margin: 0.75rem 0 0 0; font-size: 0.875rem; color: #991b1b;">
                    <i class="fas fa-exclamation-circle"></i> These critical validation errors stopped pipeline execution.
                    The message was rejected (NACK) and will not be processed further.
                </p>
            </div>
        `;

        return html;
    }

    /**
     * Copy FHIR bundle to clipboard
     */
    async copyBundleToClipboard(bundle) {
        if (!bundle) {
            this.dragDropManager.showNotification('No FHIR bundle to copy', 'warning');
            return;
        }

        try {
            await navigator.clipboard.writeText(JSON.stringify(bundle, null, 2));
            this.dragDropManager.showNotification('FHIR Bundle copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy:', error);
            this.dragDropManager.showNotification('Failed to copy bundle', 'error');
        }
    }

    /**
     * Copy CCD/C-CDA XML to clipboard (mirrors copyBundleToClipboard for the
     * FHIR side) — xml is this._lastTestCdaXML, the raw XML string a
     * cda.build step's output already content-sniffed for, so no
     * JSON.stringify/parse round trip is needed here.
     */
    async copyCdaXmlToClipboard(xml) {
        if (!xml) {
            this.dragDropManager.showNotification('No CCD/CDA XML to copy', 'warning');
            return;
        }

        try {
            await navigator.clipboard.writeText(xml);
            this.dragDropManager.showNotification('CCD/CDA XML copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy:', error);
            this.dragDropManager.showNotification('Failed to copy CDA XML', 'error');
        }
    }

    /**
     * Copy results to clipboard
     */
    async copyResultsToClipboard(result) {
        try {
            await navigator.clipboard.writeText(JSON.stringify(result, null, 2));
            this.dragDropManager.showNotification('Full results copied to clipboard', 'success');
        } catch (error) {
            console.error('Failed to copy:', error);
            this.dragDropManager.showNotification('Failed to copy results', 'error');
        }
    }

    /**
     * Escape HTML to prevent XSS
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    /**
     * Synchronize branch relationships before saving
     * Updates parentConditionalStepId and branchType on target steps
     * based on conditional step configs (onTrue/onFalse route_to_step actions)
     * AND propagates branch membership through chains based on visual position
     */
    synchronizeBranchRelationships() {
        const allSteps = this.getAllSteps();
        if (!allSteps || allSteps.length === 0) return;

        console.log('[PipelineBuilder] Synchronizing branch relationships...');

        // First, clear existing branch relationships (they'll be re-established)
        allSteps.forEach(step => {
            step.parentConditionalStepId = null;
            step.branchType = null;
        });

        // PHASE 1: Mark direct routing targets from conditional steps
        allSteps.forEach(conditionalStep => {
            // Use VisualStep utility for OOP-compliant type detection
            const isLogicStep = VisualStep.isConditionalStep(conditionalStep);

            if (!isLogicStep || !conditionalStep.config || !conditionalStep.config.conditions) {
                return;
            }

            conditionalStep.config.conditions.forEach(condition => {
                // Check onTrue action
                const trueAction = condition.onTrue || condition.ifTrue;
                if (trueAction && trueAction.action === 'route_to_step' && trueAction.stepId) {
                    const targetStep = allSteps.find(s => s.id === trueAction.stepId);
                    if (targetStep) {
                        targetStep.parentConditionalStepId = conditionalStep.id;
                        targetStep.branchType = 'true';
                        console.log(`  TRUE branch (direct): ${conditionalStep.stepName} → ${targetStep.stepName}`);
                    }
                }

                // Check onFalse action
                const falseAction = condition.onFalse || condition.ifFalse;
                if (falseAction && falseAction.action === 'route_to_step' && falseAction.stepId) {
                    const targetStep = allSteps.find(s => s.id === falseAction.stepId);
                    if (targetStep) {
                        targetStep.parentConditionalStepId = conditionalStep.id;
                        targetStep.branchType = 'false';
                        console.log(`  FALSE branch (direct): ${conditionalStep.stepName} → ${targetStep.stepName}`);
                    }
                }
            });
        });

        // PHASE 2: Propagate branch membership through chains based on visual position
        // Steps that come AFTER a branch step (higher X position, same Y band) inherit branch membership
        this.propagateBranchMembership(allSteps);

        console.log('[PipelineBuilder] Branch relationships synchronized');
    }

    /**
     * Propagate branch membership through chains
     * If step A is in TRUE branch, and step B follows A (by position or sequence),
     * then B should also be in the TRUE branch
     */
    propagateBranchMembership(allSteps) {
        // Get steps with branch membership (marked in phase 1)
        let branchSteps = allSteps.filter(s => s.parentConditionalStepId && s.branchType);

        if (branchSteps.length === 0) return;

        console.log('[PipelineBuilder] Propagating branch membership...');

        const Y_TOLERANCE = 50;

        // Keep propagating until no more changes
        let changed = true;
        let iterations = 0;
        const maxIterations = 10; // Prevent infinite loops

        while (changed && iterations < maxIterations) {
            changed = false;
            iterations++;

            // Get current branch steps (may have grown from previous iteration)
            branchSteps = allSteps.filter(s => s.parentConditionalStepId && s.branchType);

            branchSteps.forEach(branchStep => {
                const branchY = branchStep.position_y || 0;
                const branchX = branchStep.position_x || 0;
                const branchSeq = branchStep.sequence || 0;
                const parentId = branchStep.parentConditionalStepId;
                const branchType = branchStep.branchType;

                // Find steps that should be in this branch:
                // 1. Same Y band and higher X position, OR
                // 2. Higher sequence number (for vertical branch chains)
                const stepsInChain = allSteps.filter(s => {
                    if (s.id === branchStep.id) return false;
                    if (s.parentConditionalStepId) return false; // Already has branch membership

                    const stepY = s.position_y || 0;
                    const stepX = s.position_x || 0;
                    const stepSeq = s.sequence || 0;

                    // Option 1: Same Y band and higher X position
                    const sameRowAndAfter = Math.abs(stepY - branchY) <= Y_TOLERANCE && stepX > branchX;

                    // Option 2: Higher sequence number AND positioned below or to the right
                    // (for vertical branch chains like FALSE branch)
                    const higherSequenceAndRelated = stepSeq > branchSeq &&
                                                     (stepX >= branchX - 100) && // Not too far left
                                                     (stepY >= branchY - Y_TOLERANCE); // Not above

                    return sameRowAndAfter || higherSequenceAndRelated;
                });

                // For each candidate, check if it's likely part of this branch
                // by verifying it's not part of a different branch (different Y region)
                stepsInChain.forEach(chainStep => {
                    // Don't propagate to steps that are clearly on a different Y region
                    // (e.g., TRUE branch is at Y=100, FALSE branch is at Y=300)
                    const stepY = chainStep.position_y || 0;
                    const isOnDifferentBranchRegion = Math.abs(stepY - branchY) > Y_TOLERANCE * 3;

                    if (!isOnDifferentBranchRegion) {
                        chainStep.parentConditionalStepId = parentId;
                        chainStep.branchType = branchType;
                        changed = true;
                        console.log(`  ${branchType.toUpperCase()} branch (propagated): ${branchStep.stepName} → ${chainStep.stepName}`);
                    }
                });
            });
        }

        console.log(`[PipelineBuilder] Branch propagation completed in ${iterations} iteration(s)`);
    }

    /**
     * Sync connections from flowchart to pipeline model
     * This ensures connections are saved to the database
     */
    syncConnectionsToPipeline() {
        if (!this.flowchartRenderer || !this.flowchartRenderer.layout) {
            console.log('[PipelineBuilder] No flowchart layout - skipping connection sync');
            return;
        }

        const connections = this.flowchartRenderer.layout.connections || [];

        // Convert connections to a serializable format
        this.pipeline.connections = connections.map(conn => ({
            from: conn.from,
            to: conn.to,
            type: conn.type || 'sequential'
        }));

        console.log(`[PipelineBuilder] Synced ${this.pipeline.connections.length} connections to pipeline model`);
    }

    /**
     * Save pipeline
     */
    async savePipeline() {
        try {
            const saveBtn = document.getElementById('savePipelineBtn');
            if (saveBtn) {
                saveBtn.disabled = true;
                saveBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Saving...';
            }

            // Synchronize branch relationships before saving
            this.synchronizeBranchRelationships();

            // Auto-sequence steps based on connections (no manual sequence entry needed)
            if (this.flowchartRenderer && typeof this.flowchartRenderer.autoSequenceSteps === 'function') {
                console.log('🔢 Auto-sequencing steps before save...');
                this.flowchartRenderer.autoSequenceSteps(true); // silent mode
            }

            // Sync connections from flowchart to pipeline model (for database persistence)
            this.syncConnectionsToPipeline();

            const result = await window.pipelineAPI.savePipeline(this.pipeline);

            if (result.success) {
                this.isSaved = true;
                this.updateAutoSaveStatus('saved');
                this.dragDropManager.showNotification('Pipeline saved successfully', 'success');

                // Invalidate test output cache — pipeline changed, re-test needed for runtime paths
                window.pipelineLastTestOutput = null;

                // Update pipeline ID with the actual DB id (handles ON CONFLICT returning original id)
                const returnedId = result.pipeline?.id || result.data?.id;
                if (returnedId) {
                    this.pipeline.id = returnedId;
                }
            }

        } catch (error) {
            console.error('Save failed:', error);
            this.dragDropManager.showNotification('Failed to save pipeline: ' + error.message, 'error');
        } finally {
            const saveBtn = document.getElementById('savePipelineBtn');
            if (saveBtn) {
                saveBtn.disabled = false;
                saveBtn.innerHTML = '<i class="fas fa-save"></i> Save Pipeline';
            }
        }
    }

    /**
     * Setup auto-save
     */
    setupAutoSave() {
        setInterval(() => {
            if (!this.isSaved) {
                this.autoSave();
            }
        }, 30000); // Auto-save every 30 seconds
    }

    /**
     * Auto-save pipeline
     */
    async autoSave() {
        try {
            this.updateAutoSaveStatus('saving');
            // Synchronize branch relationships before auto-saving
            this.synchronizeBranchRelationships();
            // Auto-sequence steps based on connections
            if (this.flowchartRenderer && typeof this.flowchartRenderer.autoSequenceSteps === 'function') {
                this.flowchartRenderer.autoSequenceSteps(true); // silent mode
            }
            // Sync connections to pipeline model
            this.syncConnectionsToPipeline();
            await window.pipelineAPI.savePipeline(this.pipeline);
            this.isSaved = true;
            this.updateAutoSaveStatus('saved');
        } catch (error) {
            console.error('Auto-save failed:', error);
        }
    }

    /**
     * Update auto-save status indicator
     */
    updateAutoSaveStatus(status) {
        const indicator = document.getElementById('autoSaveStatus');
        if (!indicator) return;

        if (status === 'saving') {
            indicator.textContent = 'Saving...';
            indicator.className = 'status-indicator saving';
        } else if (status === 'saved') {
            indicator.textContent = 'All changes saved';
            indicator.className = 'status-indicator saved';
        } else {
            indicator.textContent = '';
            indicator.className = 'status-indicator';
        }
    }

    /**
     * Mark as unsaved
     */
    markAsUnsaved() {
        this.isSaved = false;
        this.updateAutoSaveStatus('');
    }

    /**
     * Set execution mode
     */
    setExecutionMode(mode) {
        this.layerContainer.setExecutionMode(mode);

        // Update button states
        const parallelBtn = document.getElementById('parallelModeBtn');
        const inlineBtn = document.getElementById('inlineModeBtn');

        if (mode === 'parallel') {
            parallelBtn?.classList.add('active');
            inlineBtn?.classList.remove('active');
        } else {
            inlineBtn?.classList.add('active');
            parallelBtn?.classList.remove('active');
        }
    }

    /**
     * Clear canvas
     */
    async clearCanvas() {
        const confirmed = await this.dragDropManager.showConfirmDialog(
            'Clear all steps from the canvas? This cannot be undone.',
            {
                title: 'Clear Canvas',
                confirmText: 'Clear All',
                cancelText: 'Cancel',
                type: 'danger'
            }
        );

        if (confirmed) {
            this.layerContainer.clearCanvas();
            this.canvasRenderer.clearConnections();
            this.markAsUnsaved();
            this.dragDropManager.showNotification('Canvas cleared', 'info');
        }
    }

    /**
     * Navigate back to interfaces
     */
    async navigateBack() {
        console.log('🔙 Navigate back clicked');
        console.log('isSaved:', this.isSaved);

        if (!this.isSaved) {
            const confirmLeave = await this.dragDropManager.showConfirmDialog(
                'You have unsaved changes. Are you sure you want to leave?',
                {
                    title: 'Unsaved Changes',
                    confirmText: 'Leave',
                    cancelText: 'Stay',
                    type: 'warning'
                }
            );
            console.log('User confirmation:', confirmLeave);
            if (!confirmLeave) {
                return;
            }
        }

        console.log('Navigating to interfaces.html...');
        console.log('Referrer:', document.referrer);
        console.log('History length:', window.history.length);

        // Try to go back in history, or fall back to interfaces page
        if (window.history.length > 1 && document.referrer.includes('interfaces.html')) {
            console.log('Using history.back()');
            window.history.back();
        } else {
            console.log('Using window.location.href');
            window.location.href = '/interfaces.html';
        }
    }

    /**
     * Add step to pipeline (called by DragDropManager)
     */
    addStep(step) {
        this.layerContainer.addStep(step);
        this.markAsUnsaved();

        // In flowchart mode, trigger re-render
        if (this.viewMode === 'flowchart' && this.flowchartRenderer) {
            console.log('🔄 Triggering flowchart re-render after adding step:', step.stepName);
            const allSteps = this.getAllStepsFlat();
            this.flowchartRenderer.render(allSteps);
        }
    }

    /**
     * @deprecated Use addStep() instead
     */
    addStepToLayer(step, _layerName) {
        this.addStep(step);
    }

    /**
     * Add step to specific group
     */
    addStepToGroup(step, groupId) {
        for (const group of this.pipeline.executionGroups) {
            if (group.id === groupId || group.groupId === groupId) {
                if (!(step instanceof VisualStep)) {
                    step = new VisualStep(step);
                }
                step.sequence = this.layerContainer.getNextStepSequence(group);
                group.addStep(step);
                this.layerContainer.renderCanvas();
                this.canvasRenderer.redrawAllConnections();
                this.markAsUnsaved();
                return;
            }
        }
    }

    /**
     * Delete step from pipeline (finds the group automatically)
     */
    deleteStep(stepId) {
        console.log('🗑️ Deleting step from pipeline:', stepId);

        for (let i = 0; i < this.pipeline.executionGroups.length; i++) {
            const group = this.pipeline.executionGroups[i];
            const stepIndex = group.steps.findIndex(s => s.id === stepId);
            if (stepIndex !== -1) {
                console.log(`✅ Found step at index ${stepIndex} in group ${group.groupId || group.id}`);

                // Remove the step
                group.steps.splice(stepIndex, 1);

                // Mark as unsaved
                this.markAsUnsaved();

                // Show notification
                if (this.dragDropManager) {
                    this.dragDropManager.showNotification(`Deleted step`, 'success');
                }

                return true;
            }
        }

        console.error('❌ Step not found:', stepId);
        return false;
    }

    /**
     * Remove step from group
     */
    removeStepFromGroup(stepId, groupId) {
        this.layerContainer.removeStepFromGroup(stepId, groupId);
        this.markAsUnsaved();
    }

    /**
     * Reorder steps based on current visual order
     */
    reorderSteps() {
        let allSteps = [];
        this.pipeline.executionGroups.forEach(group => {
            group.steps.forEach(step => {
                allSteps.push({ step, group });
            });
        });

        // Update sequence numbers based on current order (increments of 10)
        allSteps.forEach((item, index) => {
            item.step.sequence = (index + 1) * 10;
        });

        this.layerContainer.renderCanvas();
        this.markAsUnsaved();
    }

    /**
     * @deprecated Use reorderSteps() instead
     */
    reorderStepsInLayer(_layerName) {
        this.reorderSteps();
    }

    /**
     * Update step
     */
    updateStep(updatedStep) {
        for (const group of this.pipeline.executionGroups) {
            const stepIndex = group.steps.findIndex(s => s.id === updatedStep.id);
            if (stepIndex !== -1) {
                if (!(updatedStep instanceof VisualStep)) {
                    updatedStep = new VisualStep(updatedStep);
                }
                group.steps[stepIndex] = updatedStep;
                this.markAsUnsaved();
                return;
            }
        }
    }

    /**
     * Find step in pipeline
     */
    findStep(stepId, groupId) {
        for (const group of this.pipeline.executionGroups) {
            if (groupId && (group.id !== groupId && group.groupId !== groupId)) continue;
            const step = group.getStep ? group.getStep(stepId) : group.steps.find(s => s.id === stepId);
            if (step) return step;
        }
        return null;
    }

    /**
     * Switch view mode (list vs flowchart)
     */
    switchViewMode(mode, force = false) {
        console.log(`🔄 switchViewMode called:`, { currentMode: this.viewMode, requestedMode: mode, force });

        if (this.viewMode === mode && !force) {
            console.log('⏭️ Already in requested mode, skipping switch');
            return;
        }

        this.viewMode = mode;

        // Update button states
        const listBtn = document.getElementById('listViewBtn');
        const flowchartBtn = document.getElementById('flowchartViewBtn');

        if (listBtn && flowchartBtn) {
            if (mode === 'list') {
                listBtn.classList.add('active');
                flowchartBtn.classList.remove('active');
            } else {
                listBtn.classList.remove('active');
                flowchartBtn.classList.add('active');
            }
        }

        // Switch canvas display
        const canvasWrapper = document.getElementById('canvasWrapper');
        const canvasLayers = document.getElementById('canvasLayers');

        if (!canvasWrapper) return;

        if (mode === 'flowchart') {
            // Hide list view
            if (canvasLayers) {
                canvasLayers.style.display = 'none';
            }

            // Add flowchart mode class
            canvasWrapper.classList.add('flowchart-mode');

            // Show flowchart canvas
            const flowchartCanvas = this.flowchartRenderer.getCanvas();
            if (flowchartCanvas && !flowchartCanvas.parentNode) {
                canvasWrapper.appendChild(flowchartCanvas);
            }

            // Ensure canvas is populated first (needed for getAllStepsFlat)
            if (this.pipeline && this.pipeline.executionGroups.length > 0) {
                this.layerContainer.renderCanvas();
            }

            // Render flowchart
            this.renderFlowchart();
        } else {
            // Hide flowchart
            const flowchartCanvas = this.flowchartRenderer.getCanvas();
            if (flowchartCanvas && flowchartCanvas.parentNode) {
                flowchartCanvas.remove();
            }

            // Remove flowchart mode class
            canvasWrapper.classList.remove('flowchart-mode');

            // Show list view
            if (canvasLayers) {
                canvasLayers.style.display = '';
            }

            // Re-render list view
            this.renderPipeline();
        }

        // Reinitialize drag-drop zones for new view mode
        if (this.dragDropManager) {
            this.dragDropManager.reinitialize();
        }

        // Save preference
        this.saveViewModePreference(mode);

        console.log(`✅ Switched to ${mode} view`);
    }

    /**
     * Render flowchart mode
     */
    renderFlowchart() {
        if (!this.flowchartRenderer) {
            console.error('❌ No flowchart renderer available');
            return;
        }

        console.log('🔍 Pipeline state:', {
            hasPipeline: !!this.pipeline,
            executionGroups: this.pipeline?.executionGroups?.length || 0
        });

        const steps = this.getAllStepsFlat();
        console.log('🎨 Rendering flowchart with steps:', steps.length, steps);

        if (steps.length === 0) {
            console.warn('⚠️ No steps to render in flowchart - pipeline may be empty');
        }

        this.flowchartRenderer.render(steps);
    }

    /**
     * Get pipeline data (for external components like IfThenElseBuilder)
     */
    getPipeline() {
        return {
            ...this.pipeline,
            steps: this.getAllStepsFlat()
        };
    }

    /**
     * Get all steps as flat array
     */
    getAllStepsFlat() {
        if (!this.pipeline) {
            return [];
        }

        // Use getAllSteps if available (from VisualPipeline)
        if (this.pipeline.getAllSteps) {
            return this.pipeline.getAllSteps();
        }

        // Fallback: iterate executionGroups directly
        const steps = [];
        const groups = this.pipeline.executionGroups || [];
        groups.forEach(group => {
            if (group.steps && Array.isArray(group.steps)) {
                steps.push(...group.steps);
            }
        });

        steps.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));
        return steps;
    }

    /**
     * Alias for getAllStepsFlat (for backward compatibility)
     */
    getAllSteps() {
        return this.getAllStepsFlat();
    }

    /**
     * Load view mode preference from localStorage
     */
    loadViewModePreference() {
        const savedMode = localStorage.getItem('pipelineViewMode');
        console.log('🔍 Loading view mode preference:', { savedMode, currentViewMode: this.viewMode });

        if (savedMode === 'flowchart' || savedMode === 'list') {
            this.viewMode = savedMode;
        }

        // Always initialize with flowchart mode (hide list view from start)
        const canvasLayers = document.getElementById('canvasLayers');
        if (canvasLayers && this.viewMode === 'flowchart') {
            console.log('✅ Hiding canvasLayers during initialization');
            canvasLayers.style.display = 'none';
        }
    }

    /**
     * Save view mode preference to localStorage
     */
    saveViewModePreference(mode) {
        localStorage.setItem('pipelineViewMode', mode);
    }

    /**
     * Callback for step selection (from flowchart)
     */
    onStepSelected(stepId) {
        const allSteps = this.getAllStepsFlat();
        const step = allSteps.find(s => s.id === stepId);
        if (step) {
            this.openStepProperties(step);
        }
    }

    /**
     * Open step properties modal
     */
    openStepProperties(step) {
        if (this.propertiesPanel) {
            this.propertiesPanel.showStepProperties(step);
        }
    }
}

// Export
if (typeof window !== 'undefined') {
    window.PipelineBuilder = PipelineBuilder;
}

if (typeof module !== 'undefined' && module.exports) {
    module.exports = PipelineBuilder;
}
