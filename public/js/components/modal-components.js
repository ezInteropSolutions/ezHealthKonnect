// js/components/modal-components-refactored.js - Modal Components Loader (Refactored)
// Uses shared InterfaceConfigComponents for consistency

(function() {
    'use strict';

    function loadModalComponents() {
        loadCreateModal();
        loadEditModal();
        loadDetailsModal();
    }

    function loadCreateModal() {
        const container = document.getElementById('create-modal-container');
        if (!container) {
            console.warn('⚠️ Create modal container not found');
            return;
        }
        console.log('✅ Loading create modal...');

        // Simple create modal - redirects to wizard for full configuration
        container.innerHTML = `
            <!-- Compact Create Interface Modal -->
            <div class="modal-overlay" id="createModal">
                <div class="modal-content">
                    <div class="modal-header">
                        <h3 class="modal-title">Create New Interface</h3>
                        <button class="modal-close" onclick="closeCreateModal()">&times;</button>
                    </div>
                    <div class="modal-body">
                        <form id="createInterfaceForm">
                            <div class="form-group">
                                <label for="interfaceName">Interface Name</label>
                                <input type="text" id="interfaceName" name="name" required
                                       placeholder="e.g., ADT Patient Admissions">
                            </div>

                            <div class="form-group">
                                <label for="interfaceDescription">Description</label>
                                <textarea id="interfaceDescription" name="description"
                                          placeholder="Brief description of this interface"></textarea>
                            </div>

                            <div class="form-row">
                                <div class="form-group">
                                    <label for="sourceType">Source Type</label>
                                    <select id="sourceType" name="sourceType" required>
                                        <option value="">Select source...</option>
                                        <option value="file">File</option>
                                        <option value="tcp">TCP</option>
                                        <option value="http">HTTP</option>
                                        <option value="database">Database</option>
                                    </select>
                                </div>

                                <div class="form-group">
                                    <label for="targetType">Target Type</label>
                                    <select id="targetType" name="targetType" required>
                                        <option value="">Select target...</option>
                                        <option value="file">File</option>
                                        <option value="tcp">TCP</option>
                                        <option value="http">HTTP</option>
                                        <option value="database">Database</option>
                                        <option value="fhir">FHIR</option>
                                    </select>
                                </div>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeCreateModal()">Cancel</button>
                        <button type="submit" class="modal-btn primary" form="createInterfaceForm">Create Interface</button>
                    </div>
                </div>
            </div>
        `;
    }

    function loadEditModal() {
        const container = document.getElementById('edit-modal-container');
        if (!container) {
            console.warn('⚠️ Edit modal container not found');
            return;
        }
        console.log('✅ Loading edit modal (REFACTORED with shared components)...');

        // ✅ REFACTORED: Using shared InterfaceConfigComponents with tabs and maximize
        container.innerHTML = `
            <style>
                .family-chip {
                    cursor: pointer; padding: 4px 10px; border-radius: 12px;
                    border: 2px solid #e2e8f0; font-size: 0.78rem; font-weight: 600;
                    color: #64748b; background: #f8fafc; transition: all 0.15s; user-select: none;
                }
                .family-chip:hover { border-color: #93c5fd; color: #1e3a8a; background: #eff6ff; }
                .family-chip.selected { background: #0369a1; color: #fff; border-color: #0369a1; }
            </style>
            <!-- Edit Interface Modal (Refactored with Tabs) -->
            <div class="modal-overlay" id="editModal">
                <div class="modal-content large" id="editModalContent">
                    <div class="modal-header">
                        <h3 class="modal-title" id="editTitle">Edit Interface Configuration</h3>
                        <div class="modal-controls">
                            <button class="modal-control-btn" id="editModalMaximize" onclick="toggleEditModalMaximize()" title="Maximize">
                                <span id="editMaximizeIcon">⛶</span>
                            </button>
                            <button class="modal-close" onclick="closeEditModal()">&times;</button>
                        </div>
                    </div>

                    <!-- Tab Navigation -->
                    <div class="modal-tabs">
                        <button class="modal-tab active" data-tab="basic" onclick="switchEditTab('basic')">
                            <span class="tab-icon">&#9776;</span> Basic
                        </button>
                        <button class="modal-tab" data-tab="source" onclick="switchEditTab('source')">
                            <span class="tab-icon">&#8594;</span> Source
                        </button>
                        <button class="modal-tab" data-tab="target" onclick="switchEditTab('target')">
                            <span class="tab-icon">&#9654;</span> Target
                        </button>
                        <button class="modal-tab" data-tab="deployment" onclick="switchEditTab('deployment')">
                            <span class="tab-icon">&#9881;</span> Deploy
                        </button>
                    </div>

                    <div class="modal-body">
                        <form id="editInterfaceForm" onsubmit="handleEditInterface(event)">
                            <input type="hidden" id="editInterfaceId" name="id">

                            <!-- Tab 1: Basic Information -->
                            <div class="tab-content active" id="editTabBasic">
                                <div class="config-section">
                                    <div class="form-group">
                                        <label for="editInterfaceName" class="form-label required">Interface Name</label>
                                        <input type="text" id="editInterfaceName" class="form-control" name="name" required>
                                    </div>

                                    <div class="form-group">
                                        <label for="editInterfaceDescription" class="form-label">Description</label>
                                        <textarea id="editInterfaceDescription" class="form-control" name="description" rows="3"></textarea>
                                    </div>

                                    <div class="form-row">
                                        <div class="form-group">
                                            <label for="editStatus" class="form-label">Interface Status</label>
                                            <select id="editStatus" class="form-control" name="status">
                                                <option value="draft">📝 Draft - Initial configuration</option>
                                                <option value="configured">⚙️ Configured - Setup complete</option>
                                                <option value="testing">🧪 Testing - Under validation</option>
                                                <option value="active">✅ Active - Production ready</option>
                                                <option value="inactive">⏸️ Inactive - Temporarily disabled</option>
                                                <option value="error">❌ Error - Requires attention</option>
                                            </select>
                                            <small style="color: #64748b; display: block; margin-top: 0.5rem;">
                                                Lifecycle: Draft → Configured → Testing → Active
                                            </small>
                                        </div>
                                    </div>

                                    <!-- Logging & Troubleshooting Section -->
                                    <div style="margin-top: 2rem; padding-top: 1.5rem; border-top: 2px solid #e2e8f0;">
                                        <h4 style="color: #1e3a8a; font-size: 1.1rem; margin-bottom: 1rem; font-weight: 600;">
                                            <i class="fas fa-file-alt"></i> Logging & Troubleshooting
                                        </h4>

                                        <!-- Log Level Selector -->
                                        <div class="form-group">
                                            <div style="background: linear-gradient(to right, #f0f9ff, #f5f3ff); border-left: 3px solid #60a5fa; padding: 14px; border-radius: 6px;">
                                                <label for="editLogLevel" style="font-weight: 600; color: #1e3a8a; font-size: 0.95rem; display: block; margin-bottom: 10px;">
                                                    📝 Log Level
                                                </label>
                                                <div style="display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px;" id="logLevelGroup">
                                                    <label class="log-level-option" data-level="debug" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" id="editLogLevel" value="debug" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            🔬 Debug
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">All logs</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="info" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="info" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ℹ️ Info
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Info+</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="warning" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="warning" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ⚠️ Warn
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Warn+</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="error" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="error" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            ❌ Error
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">Errors only</div>
                                                    </label>
                                                    <label class="log-level-option" data-level="off" style="cursor:pointer;text-align:center;">
                                                        <input type="radio" name="log_level" value="off" style="display:none;">
                                                        <div class="log-level-chip" style="padding:6px 4px;border-radius:6px;border:2px solid #e2e8f0;font-size:0.78rem;font-weight:600;transition:all 0.15s;">
                                                            🔕 Off
                                                        </div>
                                                        <div style="font-size:0.7rem;color:#94a3b8;margin-top:3px;">No logs</div>
                                                    </label>
                                                </div>
                                                <div id="logLevelHint" style="font-size:0.82rem;color:#6b7280;margin-top:10px;line-height:1.4;">
                                                    🔬 <strong>Debug</strong> — captures every step for full visibility. Uses more storage.
                                                </div>
                                                <!-- hidden checkbox kept for backward-compat with save logic -->
                                                <input type="checkbox" id="editDebugLogging" name="debug_logging" checked style="display:none;">
                                            </div>
                                        </div>

                                        <!-- Log Retention Period & Error Retention -->
                                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px;">
                                            <div class="form-group" style="margin: 0;">
                                                <label for="editLogRetention" class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                                    🗑️ Log Retention Period
                                                </label>
                                                <div style="display: flex; align-items: center; gap: 8px;">
                                                    <input type="number" id="editLogRetention" class="form-control" name="log_retention_days"
                                                           value="30" min="1" max="365"
                                                           style="max-width: 100px; text-align: center; font-weight: 600;">
                                                    <span style="color: #6b7280; font-size: 0.9rem;">days</span>
                                                </div>
                                                <small style="color: #64748b; display: block; margin-top: 6px;">
                                                    Debug/info logs auto-deleted after this period
                                                </small>
                                            </div>

                                            <div class="form-group" style="margin: 0;">
                                                <label class="form-label" style="font-weight: 600; color: #1e3a8a; margin-bottom: 8px;">
                                                    ♾️ Error Log Retention
                                                </label>
                                                <div style="background: #f0f9ff; border: 1px solid #bfdbfe; padding: 10px 12px; border-radius: 6px;">
                                                    <label style="display: flex; align-items: center; cursor: pointer; margin: 0;">
                                                        <input type="checkbox" id="editRetainErrors" name="retain_error_logs_forever"
                                                               checked style="margin-right: 8px; width: 18px; height: 18px; cursor: pointer; accent-color: #0369a1;">
                                                        <span style="font-size: 0.9rem; color: #1e3a8a; font-weight: 500;">Keep errors forever</span>
                                                    </label>
                                                </div>
                                                <small style="color: #64748b; display: block; margin-top: 6px;">
                                                    ✅ Recommended for compliance
                                                </small>
                                            </div>
                                        </div>

                                        <!-- Info Panel -->
                                        <div style="background: #f9fafb; border: 1px solid #e5e7eb; padding: 12px; border-radius: 6px; margin-top: 16px;">
                                            <div style="font-size: 0.875rem; color: #374151; line-height: 1.6;">
                                                <strong style="color: #1e3a8a;">Retention Policy Summary:</strong><br>
                                                • <strong>Debug/Info logs:</strong> Deleted after <span id="editRetentionSummary" style="color: #0369a1; font-weight: 600;">30 days</span><br>
                                                • <strong>Error/Warning logs:</strong> <span id="editErrorRetentionSummary" style="color: #059669; font-weight: 600;">Kept forever</span><br>
                                                • <strong>Audit logs:</strong> <span style="color: #059669; font-weight: 600;">Always retained (HIPAA compliance)</span>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <!-- FHIR Validation Policy Section -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <label class="form-label" for="fhirValidationPolicy" style="font-weight:600;color:#374151;font-size:13px;margin-bottom:4px;display:block;">
                                        FHIR Validation Policy
                                    </label>
                                    <select id="fhirValidationPolicy" name="fhirValidationPolicy"
                                            style="width:100%;padding:7px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;color:#1f2937;background:#fff;cursor:pointer;">
                                        <option value="proceed">Proceed — omit missing field and continue (default)</option>
                                        <option value="warn">Warn — deliver with OperationOutcome warning in bundle</option>
                                        <option value="reject">Reject — fail message, route to dead-letter queue</option>
                                        <option value="queue_review">Queue for Review — hold for manual operator correction</option>
                                    </select>
                                </div>

                                <!-- Message Family Filter Section -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <div style="background: linear-gradient(to right, #f0fdf4, #f0f9ff); border-left: 3px solid #34d399; padding: 14px; border-radius: 6px;">
                                        <label style="font-weight: 600; color: #1e3a8a; font-size: 0.95rem; display: block; margin-bottom: 4px;">
                                            &#128259; Message Family Filter
                                        </label>
                                        <p style="font-size: 0.82rem; color: #6b7280; margin: 0 0 12px;">
                                            Restrict which HL7 message families this interface accepts. Unmatched messages receive a NACK (AR) before storage. Leave unrestricted to accept all.
                                        </p>
                                        <label style="display: flex; align-items: center; cursor: pointer; margin-bottom: 12px;">
                                            <input type="checkbox" id="editFamilyFilterEnabled"
                                                   onchange="toggleFamilyFilter(this.checked)"
                                                   style="margin-right: 8px; width: 16px; height: 16px; cursor: pointer; accent-color: #0369a1;">
                                            <span style="font-size: 0.9rem; color: #1e3a8a; font-weight: 500;">Restrict to specific message families</span>
                                        </label>
                                        <div id="editFamilyFilterPicker" style="display:none;">
                                            <div style="font-size: 0.79rem; color: #64748b; margin-bottom: 10px;">
                                                Click to select. MFN events must be chosen individually — each maps to a different FHIR resource structure.
                                            </div>
                                            <div style="display: flex; flex-wrap: wrap; gap: 6px;" id="editFamilyChips">
                                                <span class="family-chip" data-family="ADT" onclick="toggleFamilyChip('ADT')" title="Admit/Discharge/Transfer">ADT</span>
                                                <span class="family-chip" data-family="ORU" onclick="toggleFamilyChip('ORU')" title="Observation Results">ORU</span>
                                                <span class="family-chip" data-family="ORM" onclick="toggleFamilyChip('ORM')" title="Order Entry">ORM</span>
                                                <span class="family-chip" data-family="SIU" onclick="toggleFamilyChip('SIU')" title="Scheduling">SIU</span>
                                                <span class="family-chip" data-family="MDM" onclick="toggleFamilyChip('MDM')" title="Medical Document Management">MDM</span>
                                                <span class="family-chip" data-family="VXU" onclick="toggleFamilyChip('VXU')" title="Vaccination">VXU</span>
                                                <span class="family-chip" data-family="RDE" onclick="toggleFamilyChip('RDE')" title="Pharmacy Orders">RDE</span>
                                                <span class="family-chip" data-family="BAR" onclick="toggleFamilyChip('BAR')" title="Billing/Account">BAR</span>
                                                <span class="family-chip" data-family="DFT" onclick="toggleFamilyChip('DFT')" title="Detailed Financial Transaction">DFT</span>
                                                <span style="width:100%;font-size:0.75rem;color:#94a3b8;padding-top:4px;font-weight:500;">MFN — select individually (structurally different per event):</span>
                                                <span class="family-chip" data-family="MFN^M02" onclick="toggleFamilyChip('MFN^M02')" title="Staff / Practitioner">MFN^M02 Staff</span>
                                                <span class="family-chip" data-family="MFN^M04" onclick="toggleFamilyChip('MFN^M04')" title="Charge Description Master">MFN^M04 Charge</span>
                                                <span class="family-chip" data-family="MFN^M05" onclick="toggleFamilyChip('MFN^M05')" title="Patient Location">MFN^M05 Location</span>
                                                <span class="family-chip" data-family="MFN^M12" onclick="toggleFamilyChip('MFN^M12')" title="Observation Catalog">MFN^M12 Observation</span>
                                                <span class="family-chip" data-family="MFN^M13" onclick="toggleFamilyChip('MFN^M13')" title="Generic Master File">MFN^M13 Generic</span>
                                            </div>
                                        </div>
                                        <input type="hidden" id="editAcceptedMessageFamilies" value="">
                                    </div>
                                </div>

                                <!-- Error Threshold -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <label class="form-label" for="editErrorThreshold" style="font-weight:600;color:#374151;font-size:13px;margin-bottom:4px;display:block;">
                                        Error Threshold (%)
                                    </label>
                                    <input type="number" id="editErrorThreshold" min="0" max="100" step="1" placeholder="10"
                                           style="width:100%;padding:7px 10px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;color:#1f2937;">
                                    <p style="font-size: 0.79rem; color: #6b7280; margin: 4px 0 0;">
                                        Default error rate threshold for this interface. Used when no per-interface alert rule is configured.
                                    </p>
                                </div>

                                <!-- Recovery Queue (DLQ) config -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <div style="background: linear-gradient(to right, #fffbeb, #fff7ed); border-left: 3px solid #f59e0b; padding: 14px; border-radius: 6px;">
                                        <label style="font-weight: 600; color: #92400e; font-size: 0.95rem; display: block; margin-bottom: 4px;">
                                            &#9888; Recovery Queue (DLQ)
                                        </label>
                                        <p style="font-size: 0.82rem; color: #6b7280; margin: 0 0 12px;">
                                            Retry behaviour for messages that fail delivery from this interface.
                                        </p>
                                        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
                                            <div>
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Max Attempts</label>
                                                <input type="number" id="editDlqMaxAttempts" min="1" max="100" step="1" placeholder="10"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                            </div>
                                            <div>
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Initial Delay (s)</label>
                                                <input type="number" id="editDlqInitialDelay" min="0" step="1" placeholder="30"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                            </div>
                                            <div>
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Retry Delay (s)</label>
                                                <input type="number" id="editDlqRetryDelay" min="1" step="1" placeholder="60"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                            </div>
                                            <div>
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Backoff Multiplier</label>
                                                <input type="number" id="editDlqBackoff" min="1" max="10" step="0.1" placeholder="2.0"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                            </div>
                                            <div style="grid-column:1/-1;">
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Payload Expires After (hours)</label>
                                                <input type="number" id="editDlqExpiresAfter" min="0" step="1" placeholder="0"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                                <p style="font-size: 0.75rem; color: #6b7280; margin: 4px 0 0;">Hours until an abandoned row's payload is purged. 0 = never expire.</p>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <!-- CDA Coverage Audit config -->
                                <div class="form-group" style="margin-top: 16px;">
                                    <div style="background: linear-gradient(to right, #fefce8, #fff7ed); border-left: 3px solid #f59e0b; padding: 14px; border-radius: 6px;">
                                        <label style="display: flex; align-items: center; cursor: pointer; margin-bottom: 4px;">
                                            <input type="checkbox" id="editCdaCoverageAuditEnabled"
                                                   onchange="_toggleEditCoverageAuditNotify(this.checked)"
                                                   style="margin-right: 8px; width: 16px; height: 16px; cursor: pointer; accent-color: #d97706;">
                                            <span style="font-weight: 600; color: #92400e; font-size: 0.95rem;">&#128203; CDA Coverage Audit</span>
                                        </label>
                                        <p style="font-size: 0.82rem; color: #6b7280; margin: 0 0 10px;">
                                            Flags CCD sections/entries never used by this pipeline. Off by default; only applies when this interface's input is CDA/CCD. A badge appears on affected messages; reports never contain clinical values, only which section/entry was never used.
                                        </p>
                                        <label style="display: flex; align-items: center; cursor: pointer; margin-bottom: 10px;">
                                            <input type="checkbox" id="editCdaCoverageAuditElementLevel"
                                                   style="margin-right: 8px; width: 15px; height: 15px; cursor: pointer; accent-color: #d97706;">
                                            <span style="font-size: 0.82rem; color: #92400e;">Element-level detail <span style="color:#b45309;font-weight:600;">(beta)</span> — also flags which specific fields within a used entry (e.g. its effective time) were never read, not just whole unused entries. Applies across every section.</span>
                                        </label>
                                        <div id="editCdaCoverageAuditNotifyBlock" style="display:none;padding-top:10px;border-top:1px dashed #fbbf24;">
                                            <div style="font-size:12px;font-weight:600;color:#92400e;margin-bottom:6px;">Notify (optional)</div>
                                            <p style="font-size: 0.79rem; color: #6b7280; margin: 0 0 8px;">
                                                The badge is always shown when enabled — this additionally pushes a notification through channels configured in Alerts settings when a gap is found, throttled by the cooldown below.
                                            </p>
                                            <div id="editCdaCoverageAuditChannelList" style="max-height:140px;overflow-y:auto;border:1px solid #fde68a;border-radius:6px;padding:6px 10px;background:#fff;">
                                                <div style="font-size:11px;color:#6b7280;">Loading channels…</div>
                                            </div>
                                            <div style="margin-top:10px;max-width:220px;">
                                                <label style="font-size:12px;font-weight:600;color:#374151;display:block;margin-bottom:4px;">Cooldown (minutes)</label>
                                                <input type="number" id="editCdaCoverageAuditCooldownMinutes" min="1" step="1" placeholder="60"
                                                       style="width:100%;padding:6px 9px;border:1px solid #d1d5db;border-radius:6px;font-size:13px;">
                                                <p style="font-size: 0.75rem; color: #6b7280; margin: 4px 0 0;">At most one notification per interface within this window (default 60).</p>
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Tab 2: Source (Inbound) Connector — powered by ConnectorConfigBuilder -->
                            <div class="tab-content" id="editTabSource">
                                <div id="editInboundConnectorContainer" style="padding: 4px 0;"></div>
                            </div>

                            <!-- Tab 3: Target (Outbound) Connector — powered by ConnectorConfigBuilder -->
                            <div class="tab-content" id="editTabTarget">
                                <div id="editOutboundConnectorContainer" style="padding: 4px 0;"></div>
                            </div>

                            <!-- Tab 4: Deployment Settings -->
                            <div class="tab-content" id="editTabDeployment">
                                <div id="editDeploymentSettingsContainer"></div>
                            </div>

                        </form>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeEditModal()">Cancel</button>
                        <button type="submit" class="modal-btn primary" form="editInterfaceForm">Save Changes</button>
                    </div>
                </div>
            </div>
        `;

        // Setup tab switching and maximize functions
        setupEditModalFunctions();
    }

    function loadDetailsModal() {
        const container = document.getElementById('details-modal-container');
        if (!container) {
            console.warn('⚠️ Details modal container not found');
            return;
        }
        console.log('✅ Loading details modal...');

        container.innerHTML = `
            <!-- Interface Details Modal -->
            <div class="modal-overlay" id="detailsModal">
                <div class="modal-content large">
                    <div class="modal-header">
                        <h3 class="modal-title" id="detailsTitle">Interface Details</h3>
                        <button class="modal-close" onclick="closeDetailsModal()">&times;</button>
                    </div>
                    <div class="modal-body" id="detailsContent">
                        <!-- Content populated dynamically -->
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="modal-btn secondary" onclick="closeDetailsModal()">Close</button>
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * Populate Edit Form with Interface Data
     * Uses shared InterfaceConfigComponents
     */
    window.populateEditForm = function(interfaceData) {
        console.log('📝 Populating edit form with data:', interfaceData);

        // Basic fields
        document.getElementById('editInterfaceId').value = interfaceData.id || '';
        document.getElementById('editInterfaceName').value = interfaceData.name || '';
        document.getElementById('editInterfaceDescription').value = interfaceData.description || '';
        document.getElementById('editStatus').value = interfaceData.status || 'inactive';

        // Logging settings — set log_level selector (default: debug)
        const effectiveLevel = interfaceData.log_level || (interfaceData.debug_logging ? 'debug' : 'debug');
        setLogLevel(effectiveLevel);
        document.getElementById('editLogRetention').value = interfaceData.log_retention_days || 30;
        document.getElementById('editRetainErrors').checked = interfaceData.retain_error_logs_forever !== false;
        // ── Single source of truth: use connector step config from transformation_steps ──
        // inboundStepConfig = { connectorType, config: { host, port, ... } }
        // Fall back to source_connectivity for interfaces that pre-date the pipeline wizard.
        let sourceConnectivityValue;
        let sourceConfigData = {};

        if (interfaceData.inboundStepConfig && interfaceData.inboundStepConfig.connectorType) {
            const step = interfaceData.inboundStepConfig;
            // Derive connectivity type from connectorType (tcp_mllp_inbound → tcp, http_rest_inbound → http)
            sourceConnectivityValue = step.connectorType.replace('_inbound', '').replace('_mllp', '').replace('tcp', 'tcp');
            if (step.connectorType.includes('mllp') || step.connectorType.includes('tcp')) sourceConnectivityValue = 'tcp';
            else if (step.connectorType.includes('http')) sourceConnectivityValue = 'http';
            else if (step.connectorType.includes('file')) sourceConnectivityValue = 'file';
            else if (step.connectorType.includes('database') || step.connectorType.includes('postgresql') || step.connectorType.includes('mysql')) sourceConnectivityValue = 'database';
            else if (step.connectorType.includes('sftp')) sourceConnectivityValue = 'sftp';
            else if (step.connectorType.includes('rabbitmq')) sourceConnectivityValue = 'rabbitmq';
            else if (step.connectorType.includes('kafka')) sourceConnectivityValue = 'kafka';
            sourceConfigData = step.config || {};
            console.log('✅ Source config from connector.inbound step (single source of truth):', sourceConfigData);
        } else {
            // Fallback: legacy source_connectivity column
            sourceConnectivityValue = interfaceData.source_connectivity || interfaceData.sourceConnectivity;
            sourceConfigData = interfaceData.source_config || interfaceData.sourceConfig || {};
            if (typeof sourceConnectivityValue === 'object' && sourceConnectivityValue !== null) {
                sourceConfigData = { ...sourceConnectivityValue.config, ...sourceConfigData };
                sourceConnectivityValue = sourceConnectivityValue.type || 'tcp';
            }
            console.log('⚠️ Source config from legacy source_connectivity (no connector step found):', sourceConfigData);
        }

        // Store step IDs in hidden fields so the save can write back to the right rows
        let hiddenStepIds = document.getElementById('editConnectorStepIds');
        if (!hiddenStepIds) {
            hiddenStepIds = document.createElement('div');
            hiddenStepIds.id = 'editConnectorStepIds';
            hiddenStepIds.style.display = 'none';
            document.getElementById('editInterfaceForm')?.appendChild(hiddenStepIds);
        }
        hiddenStepIds.dataset.inboundStepId = interfaceData.inboundStepId || '';
        hiddenStepIds.dataset.outboundStepId = interfaceData.outboundStepId || '';

        // ── Inbound ConnectorConfigBuilder (Source tab) ──────────────────────────
        // Destroy any previous instance before recreating
        if (window._editInboundBuilder) {
            try { window._editInboundBuilder.destroy(); } catch (_) {}
            window._editInboundBuilder = null;
        }
        const inboundContainer = document.getElementById('editInboundConnectorContainer');
        if (inboundContainer && typeof ConnectorConfigBuilder !== 'undefined') {
            // inboundStepConfig = full step config: { connectorType, config: {...}, ... }
            // Fall back to building a minimal config from legacy fields when no step exists yet.
            const inboundStepCfg = interfaceData.inboundStepConfig && interfaceData.inboundStepConfig.connectorType
                ? interfaceData.inboundStepConfig
                : { connectorType: sourceConnectivityValue === 'http' ? 'http_rest_inbound' : 'tcp_mllp_inbound', config: sourceConfigData };
            window._editInboundBuilder = new ConnectorConfigBuilder(inboundContainer, inboundStepCfg, 'inbound');
            window._editInboundBuilder.init();
            console.log('✅ Inbound ConnectorConfigBuilder initialized with:', inboundStepCfg);
        }

        // ── Outbound ConnectorConfigBuilder (Target tab) ──────────────────────
        if (window._editOutboundBuilder) {
            try { window._editOutboundBuilder.destroy(); } catch (_) {}
            window._editOutboundBuilder = null;
        }
        const outboundContainer = document.getElementById('editOutboundConnectorContainer');
        if (outboundContainer && typeof ConnectorConfigBuilder !== 'undefined') {
            // No connector.outbound pipeline step exists yet (e.g. sink-target interfaces,
            // or ones built before the connector-step migration) — pick a fallback
            // connectorType that matches this interface's OWN target_type instead of
            // always assuming FHIR. Assuming 'http_outbound' unconditionally forced every
            // sink/database/file-target interface without a step through FHIR-only
            // validation ("FHIR Base URL is required") on every save, even for edits that
            // have nothing to do with the target (e.g. just renaming, or toggling Error
            // Threshold). 'sink_outbound' is the safe default when target_type itself is
            // unknown/absent — SinkInterfaceHandler.validateConfiguration() always passes.
            const TARGET_TYPE_TO_FALLBACK_CONNECTOR = {
                fhir: 'http_outbound',
                hl7v2: 'tcp_mllp_outbound',
                hl7: 'tcp_mllp_outbound',
                database: 'postgresql_outbound',
                file: 'file_writer',
                sink: 'sink_outbound',
            };
            const fallbackConnectorType = TARGET_TYPE_TO_FALLBACK_CONNECTOR[interfaceData.targetType]
                || TARGET_TYPE_TO_FALLBACK_CONNECTOR[interfaceData.target_type]
                || 'sink_outbound';
            const outboundStepCfg = interfaceData.outboundStepConfig && interfaceData.outboundStepConfig.connectorType
                ? interfaceData.outboundStepConfig
                : { connectorType: fallbackConnectorType, config: interfaceData.target_config || interfaceData.targetConfig || {} };
            window._editOutboundBuilder = new ConnectorConfigBuilder(outboundContainer, outboundStepCfg, 'outbound');
            window._editOutboundBuilder.init();
            console.log('✅ Outbound ConnectorConfigBuilder initialized with:', outboundStepCfg);
        }

        // Deployment Settings - use shared component
        const deploymentSettingsContainer = document.getElementById('editDeploymentSettingsContainer');
        if (deploymentSettingsContainer) {
            console.log('🚀 Populating deployment settings with data:', {
                deployment_mode: interfaceData.deployment_mode,
                auto_start: interfaceData.auto_start,
                deployment_delay_seconds: interfaceData.deployment_delay_seconds
            });
            deploymentSettingsContainer.innerHTML = InterfaceConfigComponents.getDeploymentSettingsPanel(
                interfaceData,
                'edit'
            );
            // Initialize event handlers for deployment settings
            InterfaceConfigComponents.initDeploymentSettingsEvents(document, 'edit');
        } else {
            console.warn('⚠️ editDeploymentSettingsContainer not found in DOM');
        }

        // FHIR Validation Policy — set saved value on the select (default: 'proceed')
        const policy = interfaceData.fhir_validation_policy || interfaceData.fhirValidationPolicy || 'proceed';
        const policySelect = document.getElementById('fhirValidationPolicy');
        if (policySelect) policySelect.value = policy;

        // Message Family Filter
        const families = interfaceData.accepted_message_families || interfaceData.acceptedMessageFamilies;
        if (typeof window.setFamilyFilterValue === 'function') {
            window.setFamilyFilterValue(families || null);
        }

        // Error Threshold
        const errThresholdEl = document.getElementById('editErrorThreshold');
        if (errThresholdEl) errThresholdEl.value = interfaceData.error_threshold != null ? interfaceData.error_threshold : '';

        // Recovery Queue (DLQ) config
        const dlq = interfaceData.dlq_config || {};
        const dlqMax = document.getElementById('editDlqMaxAttempts');
        if (dlqMax) dlqMax.value = dlq.max_attempts != null ? dlq.max_attempts : '';
        const dlqInit = document.getElementById('editDlqInitialDelay');
        if (dlqInit) dlqInit.value = dlq.initial_delay_s != null ? dlq.initial_delay_s : '';
        const dlqRetry = document.getElementById('editDlqRetryDelay');
        if (dlqRetry) dlqRetry.value = dlq.retry_delay_s != null ? dlq.retry_delay_s : '';
        const dlqBackoff = document.getElementById('editDlqBackoff');
        if (dlqBackoff) dlqBackoff.value = dlq.backoff_multiplier != null ? dlq.backoff_multiplier : '';
        const dlqExpires = document.getElementById('editDlqExpiresAfter');
        if (dlqExpires) dlqExpires.value = dlq.expires_after_hours != null ? dlq.expires_after_hours : '';

        // CDA Coverage Audit config — NULL (or JSON null) = disabled (default, V207/V209).
        const coverageConfig = interfaceData.cda_coverage_audit_config;
        const coverageEnabledEl = document.getElementById('editCdaCoverageAuditEnabled');
        const isCoverageEnabled = !!(coverageConfig && typeof coverageConfig === 'object' && coverageConfig.enabled);
        if (coverageEnabledEl) coverageEnabledEl.checked = isCoverageEnabled;
        const coverageElementLevelEl = document.getElementById('editCdaCoverageAuditElementLevel');
        if (coverageElementLevelEl) coverageElementLevelEl.checked = !!(coverageConfig && coverageConfig.granularity === 'element');
        const coverageNotify = (coverageConfig && coverageConfig.notify) || {};
        const coverageCooldownEl = document.getElementById('editCdaCoverageAuditCooldownMinutes');
        if (coverageCooldownEl) coverageCooldownEl.value = coverageNotify.cooldown_minutes != null ? coverageNotify.cooldown_minutes : '';
        if (typeof window._loadEditCoverageAuditChannels === 'function') {
            window._loadEditCoverageAuditChannels(coverageNotify.channel_ids || []);
        }
        if (typeof window._toggleEditCoverageAuditNotify === 'function') {
            window._toggleEditCoverageAuditNotify(isCoverageEnabled);
        }
    };

    // Shows/hides the optional "Notify" sub-block based on whether CDA
    // Coverage Audit itself is enabled — no point offering a channel picker
    // for a disabled feature. Bound to the checkbox's onchange in the
    // template above, and called once from populateEditForm on open.
    window._toggleEditCoverageAuditNotify = function(enabled) {
        const block = document.getElementById('editCdaCoverageAuditNotifyBlock');
        if (block) block.style.display = enabled ? 'block' : 'none';
    };

    // Loads existing alert channels (same GET /api/alerts/channels endpoint
    // alerts.html's own channel-assignment UI and this app's other coverage
    // Audit UI already use) into a checkbox list, pre-checked from selectedIds.
    // A small, deliberate duplicate of the same ~20 lines the old Settings-tab
    // Coverage Audit UI used — not worth a shared module at this size.
    window._loadEditCoverageAuditChannels = function(selectedIds) {
        const el = document.getElementById('editCdaCoverageAuditChannelList');
        if (!el) return;
        fetch('/api/alerts/channels', { credentials: 'include' })
            .then(function(r) { return r.json(); })
            .then(function(res) {
                const channels = (res && res.data) || [];
                if (!channels.length) {
                    el.innerHTML = '<div style="font-size:11px;color:#6b7280;">No alert channels configured yet — <a href="alerts.html" target="_blank">create one in Alerts settings</a>.</div>';
                    return;
                }
                el.innerHTML = channels.map(function(ch) {
                    const checked = selectedIds.indexOf(ch.id) !== -1;
                    const iconClass = ch.channel_type === 'email' ? 'fa-envelope' : ch.channel_type === 'slack' ? 'fa-slack' : 'fa-globe';
                    return '<label style="display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid #fef3c7;cursor:pointer;font-size:12px;">' +
                        '<input type="checkbox" data-channel-id="' + ch.id + '" ' + (checked ? 'checked' : '') + ' style="width:auto;">' +
                        '<i class="fab ' + iconClass + '" style="width:16px;color:#92400e;"></i>' +
                        '<span>' + ch.name + '</span>' +
                        '<span style="font-size:10px;color:#6b7280;margin-left:auto;">' + ch.channel_type + '</span>' +
                    '</label>';
                }).join('');
            })
            .catch(function() {
                el.innerHTML = '<div style="font-size:11px;color:#6b7280;">Could not load alert channels.</div>';
            });
    };

    /**
     * Setup Edit Modal Functions (tabs, maximize)
     */
    function setupEditModalFunctions() {
        // Expose functions globally
        window.switchEditTab = function(tabName) {
            // Update tab buttons
            document.querySelectorAll('.modal-tab').forEach(tab => {
                tab.classList.remove('active');
                if (tab.dataset.tab === tabName) {
                    tab.classList.add('active');
                }
            });

            // Update tab content
            document.querySelectorAll('.tab-content').forEach(content => {
                content.classList.remove('active');
            });

            const tabContent = document.getElementById(`editTab${tabName.charAt(0).toUpperCase() + tabName.slice(1)}`);
            if (tabContent) {
                tabContent.classList.add('active');
            }
        };

        window.toggleFamilyFilter = function(enabled) {
            const picker = document.getElementById('editFamilyFilterPicker');
            const hiddenInput = document.getElementById('editAcceptedMessageFamilies');
            if (picker) picker.style.display = enabled ? 'block' : 'none';
            if (!enabled) {
                if (hiddenInput) hiddenInput.value = '';
                document.querySelectorAll('#editFamilyChips .family-chip').forEach(chip => chip.classList.remove('selected'));
            }
        };

        window.toggleFamilyChip = function(family) {
            const chip = document.querySelector(`#editFamilyChips [data-family="${family}"]`);
            const hiddenInput = document.getElementById('editAcceptedMessageFamilies');
            if (!chip || !hiddenInput) return;
            chip.classList.toggle('selected');
            let current = [];
            try { current = hiddenInput.value ? JSON.parse(hiddenInput.value) : []; } catch (e) {}
            const idx = current.indexOf(family);
            if (idx >= 0) current.splice(idx, 1); else current.push(family);
            hiddenInput.value = current.length > 0 ? JSON.stringify(current) : '';
        };

        window.setFamilyFilterValue = function(families) {
            const toggle = document.getElementById('editFamilyFilterEnabled');
            const hiddenInput = document.getElementById('editAcceptedMessageFamilies');
            if (!Array.isArray(families) || families.length === 0) {
                if (toggle) toggle.checked = false;
                window.toggleFamilyFilter(false);
                return;
            }
            if (toggle) toggle.checked = true;
            window.toggleFamilyFilter(true);
            if (hiddenInput) hiddenInput.value = JSON.stringify(families);
            families.forEach(family => {
                const chip = document.querySelector(`#editFamilyChips [data-family="${family}"]`);
                if (chip) chip.classList.add('selected');
            });
        };

        window.toggleEditModalMaximize = function() {
            const modalContent = document.getElementById('editModalContent');
            const icon = document.getElementById('editMaximizeIcon');

            if (modalContent) {
                modalContent.classList.toggle('maximized');
                if (modalContent.classList.contains('maximized')) {
                    icon.textContent = '⧉';
                } else {
                    icon.textContent = '⛶';
                }
            }
        };

        console.log('✅ Edit modal functions setup complete');
    }

    // Run on load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadModalComponents);
    } else {
        loadModalComponents();
    }

    console.log('✅ Modal components loader initialized (REFACTORED VERSION)');
})();
