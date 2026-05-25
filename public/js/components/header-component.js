// js/components/header-component.js - Interface Header Component Loader
// Loads the interface header with status tiles

(function() {
    'use strict';
    
    function loadHeaderComponent() {
        const container = document.getElementById('interface-header-container');
        if (!container) {
            console.warn('⚠️ Interface header container not found');
            return;
        }
        
        // ✅ PRESERVED: Header HTML with all original IDs and structure
        container.innerHTML = `
            <!-- Ultra Compact Header with Inline Status Tiles -->
            <header class="main-header">
                <div class="header-left">
                    <h1 class="page-title">Interfaces</h1>
                    
                    <!-- Status tiles inline in header -->
                    <div class="header-status-tiles">
                        <div class="status-tile">
                            <i class="far fa-layer-group status-tile-icon"></i>
                            <span class="status-tile-value" id="totalInterfaces">0</span>
                            <span class="status-tile-label">Total</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-circle status-tile-icon tile-icon-running"></i>
                            <span class="status-tile-value" id="runningInterfaces">0</span>
                            <span class="status-tile-label">Running</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-circle status-tile-icon tile-icon-stopped"></i>
                            <span class="status-tile-value" id="stoppedInterfaces">0</span>
                            <span class="status-tile-label">Stopped</span>
                        </div>

                        <div class="status-tile">
                            <i class="far fa-pause-circle status-tile-icon tile-icon-paused"></i>
                            <span class="status-tile-value" id="pausedInterfaces">0</span>
                            <span class="status-tile-label">Paused</span>
                        </div>

                        <!-- Auto-refresh indicator -->
                        <div class="status-tile" id="autoRefreshIndicator">
                            <i class="far fa-clock status-tile-icon tile-icon-auto"></i>
                            <span class="status-tile-label">Auto: <span id="refreshStatus">ON</span></span>
                        </div>

                        <!-- Clock -->
                        <div class="status-tile" id="timeDisplay">
                            <i class="far fa-clock status-tile-icon"></i>
                            <span class="status-tile-label"><span id="currentTime">Loading...</span></span>
                        </div>
                    </div>
                </div>
                
                <div class="header-right">
                    <button onclick="openInterfacesHelp()" title="Help" style="
                        display:inline-flex;align-items:center;justify-content:center;
                        width:32px;height:32px;border-radius:50%;
                        border:1px solid #bfdbfe;background:#eff6ff;
                        color:#1e40af;font-size:15px;cursor:pointer;
                        margin-right:8px;transition:background 0.15s;
                    " onmouseover="this.style.background='#dbeafe'" onmouseout="this.style.background='#eff6ff'">
                        <i class="fas fa-question"></i>
                    </button>
                    <button class="create-btn" onclick="openWizard()" id="createInterfaceBtn">
                        <span class="btn-icon">+</span>
                        <span class="btn-text">Create Interface</span>
                    </button>
                </div>
            </header>

            <!-- ── Interfaces Help Modal ──────────────────────────────────── -->
            <div id="interfacesHelpModal" style="
                display:none;position:fixed;inset:0;z-index:9999;
                background:rgba(15,23,42,0.45);backdrop-filter:blur(2px);
                align-items:center;justify-content:center;
            " onclick="if(event.target===this)closeInterfacesHelp()">
                <div style="
                    background:#fff;border-radius:14px;
                    width:min(820px,94vw);max-height:88vh;
                    overflow-y:auto;box-shadow:0 24px 60px rgba(0,0,0,0.18);
                    display:flex;flex-direction:column;
                ">
                    <!-- Header -->
                    <div style="
                        display:flex;align-items:center;justify-content:space-between;
                        padding:18px 22px;border-bottom:1px solid #e5e7eb;
                        position:sticky;top:0;background:#fff;z-index:1;
                        border-radius:14px 14px 0 0;
                    ">
                        <div style="display:flex;align-items:center;gap:10px">
                            <div style="
                                width:34px;height:34px;border-radius:8px;
                                background:#eff6ff;display:flex;align-items:center;
                                justify-content:center;color:#1e3a8a;font-size:16px;
                            "><i class="fas fa-plug"></i></div>
                            <div>
                                <div style="font-size:15px;font-weight:700;color:#111827">Interfaces — Help</div>
                                <div style="font-size:12px;color:#6b7280">How to create and manage integration channels</div>
                            </div>
                        </div>
                        <button onclick="closeInterfacesHelp()" style="
                            width:30px;height:30px;border-radius:50%;border:none;
                            background:#f1f5f9;cursor:pointer;font-size:16px;
                            color:#64748b;display:flex;align-items:center;justify-content:center;
                        " title="Close">&times;</button>
                    </div>

                    <!-- Body -->
                    <div style="padding:22px;display:flex;flex-direction:column;gap:20px">

                        <!-- What is an interface -->
                        <div>
                            <div style="font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;color:#94a3b8;margin-bottom:10px">Overview</div>
                            <div style="
                                background:#f8fafc;border:1px solid #e5e7eb;
                                border-left:3px solid #1e3a8a;border-radius:8px;
                                padding:14px 16px;font-size:13px;color:#374151;line-height:1.7;
                            ">
                                An <strong>Interface</strong> is a named integration channel between two systems.
                                Each interface has an <strong>inbound connector</strong> (how messages arrive),
                                a <strong>transformation pipeline</strong> (parse, validate, enrich, convert),
                                and an <strong>outbound connector</strong> (where results are delivered).
                                Interfaces are fully isolated — a busy feed never affects another.
                            </div>
                        </div>

                        <!-- Create workflow -->
                        <div>
                            <div style="font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;color:#94a3b8;margin-bottom:10px">How to create an interface</div>
                            <div style="display:flex;align-items:flex-start;gap:0;flex-wrap:wrap">
                                ${[
                                    ['1','Click + Create'],
                                    ['2','Pick a Template<br><span style="font-size:10px;color:#9ca3af">or start blank</span>'],
                                    ['3','Configure<br>Inbound'],
                                    ['4','Build<br>Pipeline'],
                                    ['5','Add<br>Outbound'],
                                    ['6','Activate<br>&amp; Monitor'],
                                ].map(([n, label], i, arr) => `
                                    <div style="flex:1;min-width:80px;text-align:center">
                                        <div style="
                                            width:30px;height:30px;border-radius:50%;
                                            background:#1e3a8a;color:#fff;
                                            font-size:12px;font-weight:700;
                                            display:flex;align-items:center;justify-content:center;
                                            margin:0 auto 6px;
                                        ">${n}</div>
                                        <div style="font-size:11.5px;color:#374151;line-height:1.4">${label}</div>
                                    </div>
                                    ${i < arr.length - 1 ? '<div style="padding-top:10px;color:#93c5fd;font-size:18px;flex-shrink:0">›</div>' : ''}
                                `).join('')}
                            </div>
                        </div>

                        <!-- Two-col: status + actions -->
                        <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px">

                            <div style="background:#f8fafc;border:1px solid #e5e7eb;border-radius:8px;padding:14px 16px">
                                <div style="font-size:12.5px;font-weight:700;color:#1e3a8a;margin-bottom:10px;display:flex;align-items:center;gap:6px">
                                    <i class="fas fa-circle-dot"></i> Status Reference
                                </div>
                                ${[
                                    ['#dcfce7','#16a34a','Running','Connector is listening and processing messages'],
                                    ['#f1f5f9','#64748b','Stopped','Configured but not started — no messages accepted'],
                                    ['#e0f2fe','#0369a1','Paused','Queuing messages but pipeline is suspended'],
                                    ['#fee2e2','#dc2626','Error','Fatal runtime error — check Monitoring'],
                                    ['#fef3c7','#d97706','Draft','Wizard incomplete — not yet deployable'],
                                ].map(([bg, color, label, desc]) => `
                                    <div style="display:flex;align-items:flex-start;gap:8px;margin-bottom:7px">
                                        <span style="
                                            background:${bg};color:${color};border-radius:10px;
                                            padding:1px 9px;font-size:11px;font-weight:600;
                                            white-space:nowrap;flex-shrink:0;margin-top:1px;
                                        ">${label}</span>
                                        <span style="font-size:12px;color:#4b5563">${desc}</span>
                                    </div>
                                `).join('')}
                            </div>

                            <div style="background:#f8fafc;border:1px solid #e5e7eb;border-radius:8px;padding:14px 16px">
                                <div style="font-size:12.5px;font-weight:700;color:#1e3a8a;margin-bottom:10px;display:flex;align-items:center;gap:6px">
                                    <i class="fas fa-ellipsis-vertical"></i> Row Actions
                                </div>
                                ${[
                                    ['▶ / ⏹','Activate or stop the interface'],
                                    ['Pipeline','Open the visual transformation editor'],
                                    ['💬 Messages','Browse messages received by this interface'],
                                    ['Edit','Change name, description, connector config'],
                                    ['Save as Template','Reuse this pipeline for future interfaces'],
                                    ['Delete','Permanently removes interface and its message table'],
                                ].map(([action, desc]) => `
                                    <div style="display:flex;gap:8px;margin-bottom:6px;font-size:12px;color:#4b5563">
                                        <strong style="color:#1e293b;white-space:nowrap;min-width:90px">${action}</strong>
                                        ${desc}
                                    </div>
                                `).join('')}
                            </div>
                        </div>

                        <!-- Connector types -->
                        <div style="background:#f8fafc;border:1px solid #e5e7eb;border-radius:8px;padding:14px 16px">
                            <div style="font-size:12.5px;font-weight:700;color:#1e3a8a;margin-bottom:10px;display:flex;align-items:center;gap:6px">
                                <i class="fas fa-network-wired"></i> Available Connector Types
                            </div>
                            <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:8px">
                                ${[
                                    ['fa-ethernet','TCP / MLLP','HL7 v2 over persistent socket — ports 6610–6670'],
                                    ['fa-globe','HTTP / REST','FHIR or REST endpoints — ports 8081–8099'],
                                    ['fa-folder-open','File','Watch a folder for CSV, HL7 batch, or any file'],
                                    ['fa-database','Database','Poll PostgreSQL, MySQL, SQL Server, Oracle'],
                                    ['fa-layer-group','Message Queue','RabbitMQ, Kafka, or Redis streams'],
                                    ['fa-cloud','Cloud Storage','AWS S3, Azure Blob, or Google Cloud Storage'],
                                ].map(([icon, name, desc]) => `
                                    <div style="background:#fff;border:1px solid #e5e7eb;border-radius:6px;padding:10px 12px">
                                        <div style="font-size:12.5px;font-weight:600;color:#1e293b;margin-bottom:3px;display:flex;align-items:center;gap:6px">
                                            <i class="fas ${icon}" style="color:#1e3a8a;font-size:12px"></i>${name}
                                        </div>
                                        <div style="font-size:11.5px;color:#6b7280;line-height:1.4">${desc}</div>
                                    </div>
                                `).join('')}
                            </div>
                        </div>

                        <!-- Tips -->
                        <div style="
                            background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;
                            padding:12px 16px;display:flex;gap:10px;align-items:flex-start;
                            font-size:12.5px;color:#1e40af;
                        ">
                            <i class="fas fa-lightbulb" style="color:#3b82f6;margin-top:1px;flex-shrink:0"></i>
                            <div>
                                <strong>Quick start tip:</strong> Use a <strong>Template</strong> to skip repetitive setup.
                                The <strong>Processing Engine</strong> (top bar) must be running before any interface can process messages.
                                If field names appear as <em>Unknown</em> in the HL7 Reader, install the matching schema version in
                                <a href="/settings.html#section-schemas" style="color:#1d4ed8;font-weight:600">Settings → Schema Packages</a>.
                            </div>
                        </div>

                    </div><!-- /body -->
                </div>
            </div>
        `;
        
        console.log('✅ Interface header component loaded');
    }

    window.openInterfacesHelp = function () {
        const m = document.getElementById('interfacesHelpModal');
        if (m) { m.style.display = 'flex'; document.body.style.overflow = 'hidden'; }
    };

    window.closeInterfacesHelp = function () {
        const m = document.getElementById('interfacesHelpModal');
        if (m) { m.style.display = 'none'; document.body.style.overflow = ''; }
    };

    // Close on Escape key
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape') window.closeInterfacesHelp?.();
    });

    // ✅ Load component when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', loadHeaderComponent);
    } else {
        loadHeaderComponent();
    }

})();