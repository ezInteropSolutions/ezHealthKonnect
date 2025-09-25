// js/messages.js
// Enhanced message management UI

class MessageManager {
    constructor() {
        console.log('🚨🚨🚨 MessageManager constructor called');
        this.currentPage = 1;
        this.pageSize = 50;
        this.filters = {};
        this.selectedMessageId = null;
        this.interfaces = [];
        this.currentInterfaceId = null; // NEW: Track current interface
        this.isInterfaceSpecific = false; // NEW: Flag for interface-specific mode

        console.log('🚨🚨🚨 About to call init()');
        this.init();
    }

    async init() {
        console.log('🚨🚨🚨 init() method called');
        try {
            // Check for URL parameters (interface filter)
            this.handleURLParameters();

            console.log('🚨🚨🚨 About to call loadUserInfo()');
            await this.loadUserInfo();
            console.log('✅ loadUserInfo completed successfully');

            await this.loadInterfaces();
            console.log('✅ loadInterfaces completed successfully');

            await this.loadMessages();
            console.log('✅ loadMessages completed successfully');

            await this.loadStats();
            console.log('✅ loadStats completed successfully');

            this.setupEventListeners();
            console.log('✅ setupEventListeners completed successfully');

            // Auto-refresh every 30 seconds
            setInterval(() => {
                this.loadMessages();
                this.loadStats();
            }, 30000);

            console.log('✅ MessageManager initialization completed successfully');
        } catch (error) {
            console.error('💥💥💥 CRITICAL ERROR in MessageManager.init():', error);
            console.error('💥 Error stack:', error.stack);
            console.error('💥 Error message:', error.message);
            // Don't redirect, just log the error
            alert('Error initializing messages page: ' + error.message);
        }
    }

    handleURLParameters() {
        const urlParams = new URLSearchParams(window.location.search);
        const interfaceId = urlParams.get('interfaceId');

        if (interfaceId) {
            // Interface-specific mode (REQUIRED)
            this.currentInterfaceId = interfaceId;
            this.isInterfaceSpecific = true;
            console.log(`🎯 Interface-specific mode enabled for interface: ${interfaceId}`);

            // Update page title and UI
            this.updateInterfaceSpecificUI();
        } else {
            // No interface specified - redirect to interface selection
            console.log('❌ No interface specified - redirecting to interface selection');
            this.redirectToInterfaceSelection();
            return;
        }
    }

    redirectToInterfaceSelection() {
        // Show interface selection modal or redirect to interfaces page
        alert('Please select an interface to view messages.\n\nFor better performance, each interface has its own message viewer.');
        window.location.href = '/interfaces.html';
    }

    updateInterfaceSpecificUI() {
        if (this.isInterfaceSpecific) {
            // Update page title
            const pageTitle = document.querySelector('.page-title');
            if (pageTitle) {
                pageTitle.textContent = 'Interface Messages';
            }

            // Hide global interface filter (not needed in interface-specific mode)
            const interfaceFilterContainer = document.querySelector('#filterInterface').closest('.col-md-3');
            if (interfaceFilterContainer) {
                interfaceFilterContainer.style.display = 'none';
            }

            // Add interface selector instead
            this.addInterfaceSelector();
        }
    }

    addInterfaceSelector() {
        const pageHeader = document.querySelector('.page-header .d-flex');
        if (pageHeader && !document.getElementById('interfaceSelector')) {
            const selectorHTML = `
                <div id="interfaceSelector" class="me-3">
                    <label class="form-label text-muted">Current Interface:</label>
                    <select class="form-select form-select-sm" id="currentInterfaceSelect" onchange="messageManager.switchInterface(this.value)">
                        <option value="">Loading interfaces...</option>
                    </select>
                </div>
            `;
            pageHeader.insertAdjacentHTML('afterbegin', selectorHTML);
        }
    }

    async loadUserInfo() {
        console.log('🚨🚨🚨 loadUserInfo() CALLED in MessageManager');
        try {
            console.log('🔍 Messages page: Loading user info...');
            console.log('🔍 Current URL:', window.location.href);
            console.log('🔍 Document cookies:', document.cookie);

            const response = await fetch('/api/user-info');
            console.log('🔍 User info response status:', response.status);
            console.log('🔍 Response headers:', [...response.headers.entries()]);

            if (response.ok) {
                const user = await response.json();
                console.log('User loaded successfully:', user.name);

                // Update user info using dashboard structure
                const firstName = user.name ? user.name.split(' ')[0] : 'User';
                document.getElementById('userName').textContent = firstName;
                document.getElementById('userRole').textContent = (user.role || 'USER').toUpperCase();
                document.getElementById('userAvatar').textContent = firstName.charAt(0).toUpperCase();

                // Show admin sections if user is admin
                if (user.role === 'admin') {
                    const adminSection = document.getElementById('adminSection');
                    if (adminSection) {
                        adminSection.style.display = 'block';
                    }
                }
            } else if (response.status === 401) {
                console.log('Authentication failed, redirecting to login');
                // Redirect to login if not authenticated
                window.location.href = 'login.html';
                return;
            } else {
                console.log('Unexpected response status:', response.status);
                const errorText = await response.text();
                console.log('Error response:', errorText);
            }
        } catch (error) {
            console.error('Failed to load user info:', error);
            // Only redirect to login if it's definitely an auth issue
            if (error.message.includes('401') || error.message.includes('Unauthorized')) {
                window.location.href = 'login.html';
            }
        }
    }

    async loadInterfaces() {
        try {
            const response = await fetch('/api/interfaces', {
                credentials: 'include'
            });
            if (response.ok) {
                const data = await response.json();
                this.interfaces = data.interfaces || [];
                this.populateInterfaceSelects();
            } else if (response.status === 401) {
                window.location.href = 'login.html';
            }
        } catch (error) {
            console.error('Failed to load interfaces:', error);
        }
    }

    populateInterfaceSelects() {
        const filterSelect = document.getElementById('filterInterface');
        const sendSelect = document.getElementById('sendInterface');
        const currentInterfaceSelect = document.getElementById('currentInterfaceSelect');

        // Global interface filter (only show if not in interface-specific mode)
        if (filterSelect && !this.isInterfaceSpecific) {
            filterSelect.innerHTML = '<option value="">All Interfaces</option>';
            this.interfaces.forEach(interfaceItem => {
                const filterOption = document.createElement('option');
                filterOption.value = interfaceItem.id;
                filterOption.textContent = interfaceItem.name;
                filterSelect.appendChild(filterOption);
            });
        }

        // Send interface selector
        if (sendSelect) {
            sendSelect.innerHTML = '<option value="">Select Interface</option>';
            this.interfaces.forEach(interfaceItem => {
                const sendOption = document.createElement('option');
                sendOption.value = interfaceItem.id;
                sendOption.textContent = `${interfaceItem.name} (${interfaceItem.format})`;
                sendSelect.appendChild(sendOption);
            });
        }

        // Target interface selector (for HL7→FHIR flow)
        const targetSelect = document.getElementById('sendTargetInterface');
        if (targetSelect) {
            targetSelect.innerHTML = '<option value="">Select FHIR Interface</option>';
            this.interfaces
                .filter(i => i.format === 'FHIR' || i.format === 'fhir')
                .forEach(interfaceItem => {
                    const option = document.createElement('option');
                    option.value = interfaceItem.id;
                    option.textContent = interfaceItem.name;
                    targetSelect.appendChild(option);
                });
        }

        // Current interface selector (for interface-specific mode)
        if (currentInterfaceSelect && this.isInterfaceSpecific) {
            currentInterfaceSelect.innerHTML = '<option value="">Select Interface</option>';
            this.interfaces.forEach(interfaceItem => {
                const option = document.createElement('option');
                option.value = interfaceItem.id;
                option.textContent = `${interfaceItem.name} (${interfaceItem.format})`;
                if (interfaceItem.id === this.currentInterfaceId) {
                    option.selected = true;
                }
                currentInterfaceSelect.appendChild(option);
            });
        }
    }

    // NEW: Switch to different interface in interface-specific mode
    switchInterface(interfaceId) {
        if (interfaceId && interfaceId !== this.currentInterfaceId) {
            const newUrl = `${window.location.pathname}?interfaceId=${interfaceId}`;
            window.location.href = newUrl;
        }
    }

    async loadMessages() {
        try {
            // INTERFACE-SPECIFIC MODE ONLY
            if (!this.isInterfaceSpecific || !this.currentInterfaceId) {
                console.error('❌ Interface-specific mode required');
                this.redirectToInterfaceSelection();
                return;
            }

            const params = new URLSearchParams({
                page: this.currentPage,
                limit: this.pageSize
            });

            // Add filters (excluding interfaceId since it's in the URL)
            Object.keys(this.filters).forEach(key => {
                if (key !== 'interfaceId' && this.filters[key]) {
                    params.append(key, this.filters[key]);
                }
            });

            const apiUrl = `/api/messages/interface/${this.currentInterfaceId}?${params}`;
            console.log(`🎯 Loading messages for interface: ${this.currentInterfaceId}`);

            const response = await fetch(apiUrl);
            if (response.ok) {
                const data = await response.json();
                this.renderMessages(data.data.messages);
                this.renderPagination(data.data.pagination);
                this.updateMessageCount(data.data.pagination.totalCount);

                // Update interface info if available
                if (data.data.interfaceInfo) {
                    this.updateInterfaceInfo(data.data.interfaceInfo);
                }
            } else {
                this.showError('Failed to load messages');
            }
        } catch (error) {
            console.error('Failed to load messages:', error);
            this.showError('Failed to load messages');
        }
    }

    updateInterfaceInfo(interfaceInfo) {
        // Update page subtitle with interface name
        const pageHeader = document.querySelector('.page-header p');
        if (pageHeader && this.isInterfaceSpecific) {
            pageHeader.textContent = `Messages for ${interfaceInfo.name} interface`;
        }
    }

    renderMessages(messages) {
        const tbody = document.getElementById('messagesTableBody');

        if (messages.length === 0) {
            tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">No messages found</td></tr>';
            return;
        }

        tbody.innerHTML = messages.map(message => `
            <tr class="message-row" onclick="showMessageDetail('${message.id}')">
                <td>
                    <div class="fw-bold">${message.message_id}</div>
                    ${message.correlation_id ? `<small class="text-muted">Corr: ${message.correlation_id}</small>` : ''}
                </td>
                <td>${message.interface_name}</td>
                <td>${message.message_type || 'Unknown'}</td>
                <td>${this.renderStatusBadge(message.status)}</td>
                <td class="message-size">${this.formatBytes(message.message_size)}</td>
                <td>
                    <div>${this.formatDateTime(message.received_at)}</div>
                    <small class="text-muted">${message.source_type}</small>
                </td>
                <td>
                    ${message.processing_time_ms ? `${message.processing_time_ms}ms` : '-'}
                    ${message.error_count > 0 ? `<br><small class="text-danger">${message.error_count} errors</small>` : ''}
                </td>
                <td>
                    <div class="btn-group btn-group-sm">
                        <button class="btn btn-outline-primary btn-sm" onclick="event.stopPropagation(); showMessageDetail('${message.id}')">
                            <i class="fas fa-eye"></i>
                        </button>
                        ${message.status === 'failed' || message.status === 'error' ? `
                            <button class="btn btn-outline-warning btn-sm" onclick="event.stopPropagation(); reprocessMessage('${message.id}')">
                                <i class="fas fa-redo"></i>
                            </button>
                        ` : ''}
                        <button class="btn btn-outline-danger btn-sm" onclick="event.stopPropagation(); confirmDeleteMessage('${message.id}')">
                            <i class="fas fa-trash"></i>
                        </button>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    renderStatusBadge(status) {
        const statusConfig = {
            'received': { class: 'bg-info', text: 'Received' },
            'processing': { class: 'bg-warning text-dark', text: 'Processing' },
            'transformed': { class: 'bg-primary', text: 'Transformed' },
            'sent': { class: 'bg-success', text: 'Sent' },
            'failed': { class: 'bg-danger', text: 'Failed' },
            'error': { class: 'bg-danger', text: 'Error' },
            'reprocessing': { class: 'bg-warning text-dark', text: 'Reprocessing' }
        };

        const config = statusConfig[status] || { class: 'bg-secondary', text: status };
        return `<span class="badge ${config.class} status-badge">${config.text}</span>`;
    }

    renderPagination(pagination) {
        const nav = document.getElementById('pagination');

        if (pagination.totalPages <= 1) {
            nav.innerHTML = '';
            return;
        }

        let html = '';

        // Previous button
        html += `
            <li class="page-item ${pagination.hasPreviousPage ? '' : 'disabled'}">
                <a class="page-link" href="#" onclick="messageManager.changePage(${pagination.currentPage - 1})">Previous</a>
            </li>
        `;

        // Page numbers
        const startPage = Math.max(1, pagination.currentPage - 2);
        const endPage = Math.min(pagination.totalPages, pagination.currentPage + 2);

        if (startPage > 1) {
            html += '<li class="page-item"><a class="page-link" href="#" onclick="messageManager.changePage(1)">1</a></li>';
            if (startPage > 2) {
                html += '<li class="page-item disabled"><span class="page-link">...</span></li>';
            }
        }

        for (let i = startPage; i <= endPage; i++) {
            html += `
                <li class="page-item ${i === pagination.currentPage ? 'active' : ''}">
                    <a class="page-link" href="#" onclick="messageManager.changePage(${i})">${i}</a>
                </li>
            `;
        }

        if (endPage < pagination.totalPages) {
            if (endPage < pagination.totalPages - 1) {
                html += '<li class="page-item disabled"><span class="page-link">...</span></li>';
            }
            html += `<li class="page-item"><a class="page-link" href="#" onclick="messageManager.changePage(${pagination.totalPages})">${pagination.totalPages}</a></li>`;
        }

        // Next button
        html += `
            <li class="page-item ${pagination.hasNextPage ? '' : 'disabled'}">
                <a class="page-link" href="#" onclick="messageManager.changePage(${pagination.currentPage + 1})">Next</a>
            </li>
        `;

        nav.innerHTML = html;
    }

    async loadStats() {
        try {
            // INTERFACE-SPECIFIC STATS ONLY
            if (!this.isInterfaceSpecific || !this.currentInterfaceId) {
                console.error('❌ Interface-specific mode required for stats');
                return;
            }

            const params = new URLSearchParams({
                timeRange: '24h'
            });

            const apiUrl = `/api/messages/interface/${this.currentInterfaceId}/stats?${params}`;

            const response = await fetch(apiUrl);
            if (response.ok) {
                const data = await response.json();
                this.renderStats(data.data.stats || data.data);
            }
        } catch (error) {
            console.error('Failed to load stats:', error);
        }
    }

    renderStats(stats) {
        const container = document.getElementById('quickStats');
        if (!container) {
            console.log('⚠️ quickStats container not found, using alternative stats display');
            this.renderStatsAlternative(stats);
            return;
        }

        const successRate = stats.total_messages > 0 ?
            ((stats.successful_messages / stats.total_messages) * 100).toFixed(1) : 0;

        container.innerHTML = `
            <div class="row g-2 text-center">
                <div class="col-6">
                    <div class="h6 mb-0 text-primary">${stats.total_messages || 0}</div>
                    <small class="text-muted">Total</small>
                </div>
                <div class="col-6">
                    <div class="h6 mb-0 text-success">${stats.successful_messages || 0}</div>
                    <small class="text-muted">Success</small>
                </div>
                <div class="col-6">
                    <div class="h6 mb-0 text-danger">${stats.failed_messages || 0}</div>
                    <small class="text-muted">Failed</small>
                </div>
                <div class="col-6">
                    <div class="h6 mb-0 text-warning">${stats.processing_messages || 0}</div>
                    <small class="text-muted">Processing</small>
                </div>
            </div>
            <hr class="my-2">
            <div class="text-center">
                <div class="small text-muted">Success Rate</div>
                <div class="h6 mb-0">${successRate}%</div>
            </div>
            ${stats.avg_processing_time ? `
                <hr class="my-2">
                <div class="text-center">
                    <div class="small text-muted">Avg Processing</div>
                    <div class="h6 mb-0">${Math.round(stats.avg_processing_time)}ms</div>
                </div>
            ` : ''}
        `;
    }

    renderStatsAlternative(stats) {
        // Update the existing stats elements in the page header
        const totalMessages = document.getElementById('totalMessages');
        const activeInterfaces = document.getElementById('activeInterfaces');
        const successRate = document.getElementById('successRate');

        const successPercent = stats.total_messages > 0 ?
            ((stats.successful_messages / stats.total_messages) * 100).toFixed(1) : 0;

        if (totalMessages) totalMessages.textContent = stats.total_messages || 0;
        if (activeInterfaces) activeInterfaces.textContent = this.interfaces?.length || 0;
        if (successRate) successRate.textContent = `${successPercent}%`;

        console.log('✅ Stats updated via alternative method');
    }

    async showMessageDetail(messageId) {
        this.selectedMessageId = messageId;
        showMessageDetailModal();

        try {
            const response = await fetch(`/api/messages/${messageId}`);
            if (response.ok) {
                const data = await response.json();
                this.renderMessageDetail(data.data);
                this.messageData = data.data; // Store for lineage

                // Show/hide action buttons based on message status
                const message = data.data.message;
                document.getElementById('reprocessBtn').style.display =
                    ['failed', 'error'].includes(message.status) ? 'inline-block' : 'none';
                document.getElementById('deleteBtn').style.display = 'inline-block';
            } else {
                this.showError('Failed to load message details');
            }
        } catch (error) {
            console.error('Failed to load message details:', error);
            this.showError('Failed to load message details');
        }
    }

    async loadDataLineage(messageId) {
        try {
            const message = this.messageData.message;

            // Get lineage data
            const lineageResponse = await fetch(`/api/messages/${messageId}/lineage`);
            let lineageData = null;

            if (lineageResponse.ok) {
                lineageData = await lineageResponse.json();
            } else {
                // Create lineage data from available information
                lineageData = await this.constructLineageData(message);
            }

            this.renderDataLineage(lineageData);
        } catch (error) {
            console.error('Failed to load data lineage:', error);
            document.getElementById('messageLineageView').innerHTML =
                '<div class="alert alert-warning">Unable to load data lineage information.</div>';
        }
    }

    async constructLineageData(message) {
        // Construct lineage data based on correlation ID and interface configuration
        const correlationId = message.correlation_id || message.message_id;

        try {
            // Search for related messages across all interfaces
            const searchResponse = await fetch(`/api/messages/search?correlation_id=${correlationId}`);
            const relatedMessages = searchResponse.ok ? await searchResponse.json() : { data: [] };

            // Get interface configurations
            const interfacesResponse = await fetch('/api/interfaces');
            const interfaces = interfacesResponse.ok ? await interfacesResponse.json() : { data: [] };

            return {
                sourceMessage: message,
                relatedMessages: relatedMessages.data || [],
                interfaces: interfaces.data || [],
                correlationId: correlationId
            };
        } catch (error) {
            console.error('Error constructing lineage data:', error);
            return {
                sourceMessage: message,
                relatedMessages: [],
                interfaces: [],
                correlationId: correlationId
            };
        }
    }

    renderDataLineage(lineageData) {
        const container = document.getElementById('messageLineageView');
        const { sourceMessage, relatedMessages, interfaces, correlationId } = lineageData;

        // Find source interface
        const sourceInterface = interfaces.find(i => i.id === sourceMessage.interface_id);

        // Find target interfaces based on routing configuration
        let targetInterfaces = [];
        if (sourceInterface?.processing_rules) {
            const rules = JSON.parse(sourceInterface.processing_rules);
            if (rules.targetFhirInterface) {
                const targetInterface = interfaces.find(i => i.id === rules.targetFhirInterface);
                if (targetInterface) targetInterfaces.push(targetInterface);
            }
        }

        // Check if messages reached target interfaces
        const reachedTargets = relatedMessages.filter(msg =>
            targetInterfaces.some(target => target.id === msg.interface_id)
        );

        container.innerHTML = `
            <div class="lineage-container">
                <h5>🔍 Message Flow Analysis</h5>
                <p><strong>Correlation ID:</strong> <code>${correlationId}</code></p>

                <div class="lineage-flow">
                    <div class="lineage-interface source">
                        <h6>📥 Source Interface</h6>
                        <div><strong>${sourceInterface?.name || 'Unknown'}</strong></div>
                        <div class="text-muted">${sourceInterface?.source_type || 'Unknown'} → ${sourceInterface?.target_type || 'Unknown'}</div>
                        <div class="mt-2">
                            <span class="badge bg-success">Message Received</span><br>
                            <small>${this.formatDateTime(sourceMessage.received_at)}</small>
                        </div>
                    </div>

                    <div class="lineage-arrow ${sourceMessage.status === 'sent' ? 'success' : 'error'}">
                        ${sourceMessage.status === 'sent' ? '✅ →' : '❌ →'}
                    </div>

                    ${targetInterfaces.length > 0 ? targetInterfaces.map(target => {
                        const reachedTarget = reachedTargets.find(msg => msg.interface_id === target.id);
                        return `
                            <div class="lineage-interface ${reachedTarget ? 'target' : 'missing'}">
                                <h6>📤 Target Interface</h6>
                                <div><strong>${target.name}</strong></div>
                                <div class="text-muted">${target.source_type} → ${target.target_type}</div>
                                <div class="mt-2">
                                    ${reachedTarget ?
                                        `<span class="badge bg-success">Message Received</span><br>
                                         <small>${this.formatDateTime(reachedTarget.received_at)}</small>` :
                                        `<span class="badge bg-danger">Message Missing</span><br>
                                         <small>No message found</small>`
                                    }
                                </div>
                            </div>
                        `;
                    }).join('') : `
                        <div class="lineage-interface missing">
                            <h6>⚠️ Direct Routing</h6>
                            <div><strong>External Endpoint</strong></div>
                            <div class="text-muted">Direct HTTP/TCP connection</div>
                            <div class="mt-2">
                                <span class="badge bg-warning">Bypassed Interface</span><br>
                                <small>No tracking available</small>
                            </div>
                        </div>
                    `}
                </div>

                <h6 class="mt-4">📋 Message Timeline</h6>
                <table class="lineage-table">
                    <thead>
                        <tr>
                            <th>Timestamp</th>
                            <th>Interface</th>
                            <th>Event</th>
                            <th>Status</th>
                            <th>Details</th>
                        </tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>${this.formatDateTime(sourceMessage.received_at)}</td>
                            <td>${sourceInterface?.name || 'Unknown'}</td>
                            <td>Message Received</td>
                            <td><span class="badge bg-info">received</span></td>
                            <td>Type: ${sourceMessage.message_type}, Size: ${this.formatBytes(sourceMessage.message_size)}</td>
                        </tr>
                        ${sourceMessage.processing_completed_at ? `
                        <tr>
                            <td>${this.formatDateTime(sourceMessage.processing_completed_at)}</td>
                            <td>${sourceInterface?.name || 'Unknown'}</td>
                            <td>Processing Completed</td>
                            <td><span class="badge bg-${sourceMessage.status === 'sent' ? 'success' : 'danger'}">${sourceMessage.status}</span></td>
                            <td>Processing time: ${sourceMessage.processing_time_ms || 0}ms</td>
                        </tr>
                        ` : ''}
                        ${reachedTargets.map(target => `
                        <tr>
                            <td>${this.formatDateTime(target.received_at)}</td>
                            <td>${interfaces.find(i => i.id === target.interface_id)?.name || 'Unknown'}</td>
                            <td>Message Delivered</td>
                            <td><span class="badge bg-success">received</span></td>
                            <td>Correlation maintained</td>
                        </tr>
                        `).join('')}
                    </tbody>
                </table>

                ${targetInterfaces.length === 0 || reachedTargets.length === 0 ? `
                <div class="alert alert-warning mt-3">
                    <h6>⚠️ Lineage Gap Detected</h6>
                    <p>This message was processed but may not have reached its intended target interface:</p>
                    <ul>
                        <li>Source interface shows status: <strong>${sourceMessage.status}</strong></li>
                        <li>Delivery status: <strong>${sourceMessage.delivery_status || 'unknown'}</strong></li>
                        ${sourceMessage.last_error_message ? `<li>Last error: <code>${sourceMessage.last_error_message}</code></li>` : ''}
                        <li>Recommendation: Check interface routing configuration</li>
                    </ul>
                </div>
                ` : ''}
            </div>
        `;
    }

    async loadMessageContent(messageId) {
        try {
            const container = document.getElementById('messageContentView');
            const message = this.messageData.message;
            const content = this.messageData.content;

            container.innerHTML = `
                <div class="content-container">
                    <div class="row">
                        <div class="col-md-6">
                            <h6>📄 Raw Message Content</h6>
                            <div class="card">
                                <div class="card-body">
                                    <pre class="bg-light p-3" style="max-height: 400px; overflow-y: auto;"><code>${this.escapeHtml(content?.raw_message || 'No content available')}</code></pre>
                                </div>
                            </div>
                        </div>
                        <div class="col-md-6">
                            <h6>🔍 Message Properties</h6>
                            <table class="table table-sm">
                                <tr><th>Encoding:</th><td>${message.message_encoding || 'Unknown'}</td></tr>
                                <tr><th>Size:</th><td>${this.formatBytes(message.message_size)}</td></tr>
                                <tr><th>Type:</th><td>${message.message_type || 'Unknown'}</td></tr>
                                <tr><th>Source IP:</th><td>${message.source_ip || 'N/A'}</td></tr>
                                <tr><th>Source Endpoint:</th><td>${message.source_endpoint || 'N/A'}</td></tr>
                            </table>

                            ${content?.parsed_content ? `
                            <h6 class="mt-3">📋 Parsed Fields</h6>
                            <div class="card">
                                <div class="card-body">
                                    <pre class="bg-light p-3" style="max-height: 300px; overflow-y: auto;"><code>${this.escapeHtml(JSON.stringify(content.parsed_content, null, 2))}</code></pre>
                                </div>
                            </div>
                            ` : ''}
                        </div>
                    </div>
                </div>
            `;
        } catch (error) {
            console.error('Failed to load message content:', error);
            document.getElementById('messageContentView').innerHTML =
                '<div class="alert alert-warning">Unable to load message content.</div>';
        }
    }

    async loadTransformations(messageId) {
        try {
            const container = document.getElementById('messageTransformationsView');
            const transformations = this.messageData.transformations || [];

            if (transformations.length === 0) {
                container.innerHTML = `
                    <div class="alert alert-info">
                        <h6>ℹ️ No Transformations Found</h6>
                        <p>This message was not transformed or transformation data is not available.</p>
                    </div>
                `;
                return;
            }

            const transformationsHtml = transformations.map((transform, index) => `
                <div class="card mb-3">
                    <div class="card-header">
                        <h6>🔄 Transformation ${index + 1}: ${transform.type || 'Unknown'}</h6>
                    </div>
                    <div class="card-body">
                        <div class="row">
                            <div class="col-md-6">
                                <h6>Input</h6>
                                <pre class="bg-light p-3" style="max-height: 300px; overflow-y: auto;"><code>${this.escapeHtml(transform.input || 'No input data')}</code></pre>
                            </div>
                            <div class="col-md-6">
                                <h6>Output</h6>
                                <pre class="bg-light p-3" style="max-height: 300px; overflow-y: auto;"><code>${this.escapeHtml(transform.output || 'No output data')}</code></pre>
                            </div>
                        </div>
                        ${transform.mapping_rules ? `
                        <div class="mt-3">
                            <h6>Mapping Rules</h6>
                            <table class="table table-sm">
                                ${Object.entries(transform.mapping_rules).map(([source, target]) => `
                                <tr>
                                    <td><code>${source}</code></td>
                                    <td>→</td>
                                    <td><code>${target}</code></td>
                                </tr>
                                `).join('')}
                            </table>
                        </div>
                        ` : ''}
                    </div>
                </div>
            `).join('');

            container.innerHTML = `
                <div class="transformations-container">
                    <h5>🔄 Message Transformations</h5>
                    ${transformationsHtml}
                </div>
            `;
        } catch (error) {
            console.error('Failed to load transformations:', error);
            document.getElementById('messageTransformationsView').innerHTML =
                '<div class="alert alert-warning">Unable to load transformation data.</div>';
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    renderMessageDetail(data) {
        const { message, content, transformations } = data;
        const container = document.getElementById('messageDetailContent');

        container.innerHTML = `
            <div class="row">
                <div class="col-md-6">
                    <h6>Message Information</h6>
                    <table class="table table-sm">
                        <tr><th>Message ID:</th><td>${message.message_id}</td></tr>
                        <tr><th>Interface:</th><td>${message.interface_name}</td></tr>
                        <tr><th>Status:</th><td>${this.renderStatusBadge(message.status)}</td></tr>
                        <tr><th>Type:</th><td>${message.message_type || 'Unknown'}</td></tr>
                        <tr><th>Size:</th><td>${this.formatBytes(message.message_size)}</td></tr>
                        <tr><th>Source:</th><td>${message.source_type} (${message.source_endpoint || 'N/A'})</td></tr>
                        <tr><th>Received:</th><td>${this.formatDateTime(message.received_at)}</td></tr>
                        ${message.processing_completed_at ? `
                            <tr><th>Completed:</th><td>${this.formatDateTime(message.processing_completed_at)}</td></tr>
                        ` : ''}
                        ${message.processing_time_ms ? `
                            <tr><th>Processing Time:</th><td>${message.processing_time_ms}ms</td></tr>
                        ` : ''}
                        ${message.error_count > 0 ? `
                            <tr><th>Errors:</th><td class="text-danger">${message.error_count}</td></tr>
                        ` : ''}
                        ${message.last_error_message ? `
                            <tr><th>Last Error:</th><td class="text-danger">${message.last_error_message}</td></tr>
                        ` : ''}
                    </table>
                </div>
                <div class="col-md-6">
                    <h6>Processing Information</h6>
                    <table class="table table-sm">
                        <tr><th>Transformation:</th><td>${message.transformation_applied ? 'Applied' : 'None'}</td></tr>
                        <tr><th>Retry Count:</th><td>${message.retry_count}</td></tr>
                        <tr><th>Delivery Status:</th><td>${message.delivery_status || 'N/A'}</td></tr>
                        <tr><th>Delivery Attempts:</th><td>${message.delivery_attempts || 0}</td></tr>
                        ${message.source_ip ? `
                            <tr><th>Source IP:</th><td>${message.source_ip}</td></tr>
                        ` : ''}
                        ${message.correlation_id ? `
                            <tr><th>Correlation ID:</th><td>${message.correlation_id}</td></tr>
                        ` : ''}
                    </table>
                </div>
            </div>

            ${transformations.length > 0 ? `
                <div class="mt-4">
                    <h6>Transformation History</h6>
                    ${transformations.map(t => `
                        <div class="transformation-step ${!t.success ? 'failed' : ''}">
                            <div class="d-flex justify-content-between">
                                <strong>Step ${t.transformation_step}: ${t.transformation_name}</strong>
                                <span class="badge ${t.success ? 'bg-success' : 'bg-danger'}">${t.success ? 'Success' : 'Failed'}</span>
                            </div>
                            <div class="text-muted">Type: ${t.transformation_type} | Time: ${t.processing_time_ms}ms</div>
                            ${t.errors_count > 0 ? `<div class="text-danger">Errors: ${t.errors_count}</div>` : ''}
                            ${t.warnings_count > 0 ? `<div class="text-warning">Warnings: ${t.warnings_count}</div>` : ''}
                        </div>
                    `).join('')}
                </div>
            ` : ''}

            <div class="mt-4">
                <h6>Message Content</h6>
                <div class="row">
                    ${content.length > 0 ? content.map(c => `
                        <div class="col-md-${content.length > 1 ? '6' : '12'} mb-3">
                            <h6 class="text-capitalize">${c.content_type.replace('_', ' ')} Content</h6>
                            <div class="message-content">
                                ${this.formatContent(c.content_data, c.content_type)}
                            </div>
                            <small class="text-muted">Size: ${this.formatBytes(c.content_size)} | Encoding: ${c.content_encoding}</small>
                        </div>
                    `).join('') : message.raw_message ? `
                        <div class="col-12 mb-3">
                            <h6>Raw Message Content</h6>
                            <div class="message-content">
                                <pre style="background-color: #f8f9fa; padding: 10px; border-radius: 4px; font-family: monospace; white-space: pre-wrap; word-wrap: break-word;">${message.raw_message.replace(/</g, '&lt;').replace(/>/g, '&gt;')}</pre>
                            </div>
                            <small class="text-muted">Size: ${this.formatBytes(message.message_size)} | Encoding: ${message.message_encoding}</small>
                        </div>
                    ` : `
                        <div class="col-12">
                            <em class="text-muted">No message content available</em>
                        </div>
                    `}
                </div>
            </div>
        `;
    }

    formatContent(content, contentType) {
        if (!content) return '<em>No content</em>';

        try {
            if (contentType === 'original' || contentType === 'transformed') {
                // Try to format as JSON
                const parsed = JSON.parse(content);
                return JSON.stringify(parsed, null, 2);
            }
        } catch (e) {
            // Not JSON, return as-is
        }

        return content.replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    // NEW: Handle flow type changes
    handleFlowTypeChange() {
        const flowType = document.getElementById('sendFlowType').value;
        const targetGroup = document.getElementById('targetInterfaceGroup');
        const sourceLabel = document.querySelector('label[for="sendInterface"]');

        if (flowType === 'hl7-to-fhir') {
            targetGroup.style.display = 'block';
            if (sourceLabel) {
                sourceLabel.textContent = 'Source HL7 Interface';
            }

            // Auto-set HL7 content type
            document.getElementById('sendContentType').value = 'application/hl7-v2';
            document.getElementById('sendMessageType').value = 'ADT^A01';
        } else {
            targetGroup.style.display = 'none';
            if (sourceLabel) {
                sourceLabel.textContent = 'Target Interface';
            }
        }
    }

    async sendMessage() {
        const form = document.getElementById('sendMessageForm');
        if (!form.checkValidity()) {
            form.reportValidity();
            return;
        }

        const flowType = document.getElementById('sendFlowType').value;
        const sourceInterfaceId = document.getElementById('sendInterface').value;
        const messageType = document.getElementById('sendMessageType').value;
        const contentType = document.getElementById('sendContentType').value;
        const messageContent = document.getElementById('sendMessageContent').value;

        try {
            let response;

            if (flowType === 'hl7-to-fhir') {
                // HL7 → FHIR message flow
                const targetInterfaceId = document.getElementById('sendTargetInterface').value;

                if (!targetInterfaceId) {
                    this.showError('Please select a target FHIR interface');
                    return;
                }

                response = await fetch('/api/messages/flow/hl7-to-fhir', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        sourceInterfaceId,
                        targetInterfaceId,
                        hl7Message: messageContent,
                        messageType,
                        priority: 5
                    })
                });

                if (response.ok) {
                    const data = await response.json();
                    this.showSuccess(`HL7→FHIR flow initiated! Correlation ID: ${data.data.correlationId}`);
                } else {
                    const error = await response.json();
                    this.showError(error.error || 'Failed to initiate HL7→FHIR flow');
                }
            } else {
                // Single interface test message
                response = await fetch(`/api/messages/send/${sourceInterfaceId}`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({
                        messageContent,
                        messageType,
                        contentType
                    })
                });

                if (response.ok) {
                    const data = await response.json();
                    this.showSuccess('Message sent successfully!');
                } else {
                    const error = await response.json();
                    this.showError(error.error || 'Failed to send message');
                }
            }

            if (response.ok) {
                closeSendMessageModal();
                form.reset();
                this.loadMessages(); // Refresh messages
            }

        } catch (error) {
            console.error('Failed to send message:', error);
            this.showError('Failed to send message');
        }
    }

    loadSampleMessage(type) {
        const textarea = document.getElementById('sendMessageContent');
        const messageTypeInput = document.getElementById('sendMessageType');
        const contentTypeSelect = document.getElementById('sendContentType');

        if (type === 'fhir') {
            messageTypeInput.value = 'Patient';
            contentTypeSelect.value = 'application/fhir+json';
            textarea.value = JSON.stringify({
                "resourceType": "Patient",
                "id": "example-patient-001",
                "name": [{
                    "family": "Doe",
                    "given": ["John"]
                }],
                "gender": "male",
                "birthDate": "1990-05-15",
                "telecom": [{
                    "system": "phone",
                    "value": "+1-555-123-4567"
                }],
                "address": [{
                    "line": ["123 Main St"],
                    "city": "Anytown",
                    "state": "ST",
                    "postalCode": "12345",
                    "country": "US"
                }]
            }, null, 2);
        } else if (type === 'hl7') {
            messageTypeInput.value = 'ADT^A01';
            contentTypeSelect.value = 'application/hl7-v2';
            textarea.value = 'MSH|^~\\&|SYSTEM|SENDER|RECEIVER|DESTINATION|20250915141530||ADT^A01|12345|P|2.5\r\n' +
                'PID|1||12345^^^MRN||Doe^John^||19900515|M|||123 Main St^^Anytown^ST^12345^US||(555)123-4567|||S||12345|||US\r\n' +
                'PV1|1|I|ICU^001^01||||DOC001^Doctor^Attending|||||||||||12345||\r\n';
        }
    }

    formatJson() {
        const textarea = document.getElementById('sendMessageContent');
        try {
            const parsed = JSON.parse(textarea.value);
            textarea.value = JSON.stringify(parsed, null, 2);
        } catch (e) {
            this.showError('Invalid JSON format');
        }
    }

    async reprocessMessage(messageId = null) {
        const id = messageId || this.selectedMessageId;
        if (!id) return;

        try {
            const response = await fetch(`/api/messages/${id}/reprocess`, {
                method: 'POST'
            });

            if (response.ok) {
                this.showSuccess('Message queued for reprocessing');
                this.loadMessages();
                if (this.selectedMessageId === id) {
                    closeMessageDetailModal();
                }
            } else {
                const error = await response.json();
                this.showError(error.error || 'Failed to reprocess message');
            }
        } catch (error) {
            console.error('Failed to reprocess message:', error);
            this.showError('Failed to reprocess message');
        }
    }

    confirmDeleteMessage(messageId) {
        if (confirm('Are you sure you want to delete this message? This action cannot be undone.')) {
            this.deleteMessage(messageId);
        }
    }

    async deleteMessage(messageId = null) {
        const id = messageId || this.selectedMessageId;
        if (!id) return;

        try {
            const response = await fetch(`/api/messages/${id}`, {
                method: 'DELETE'
            });

            if (response.ok) {
                this.showSuccess('Message deleted successfully');
                this.loadMessages();
                if (this.selectedMessageId === id) {
                    closeMessageDetailModal();
                }
            } else {
                const error = await response.json();
                this.showError(error.error || 'Failed to delete message');
            }
        } catch (error) {
            console.error('Failed to delete message:', error);
            this.showError('Failed to delete message');
        }
    }

    setupEventListeners() {
        // Filter changes
        ['filterInterface', 'filterStatus', 'filterMessageType', 'filterDateFrom', 'filterDateTo'].forEach(id => {
            document.getElementById(id).addEventListener('change', () => {
                this.applyFilters();
            });
        });

        // Enter key in message type filter
        document.getElementById('filterMessageType').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                this.applyFilters();
            }
        });
    }

    applyFilters() {
        this.filters = {
            interfaceId: document.getElementById('filterInterface').value,
            status: document.getElementById('filterStatus').value,
            messageType: document.getElementById('filterMessageType').value,
            dateFrom: document.getElementById('filterDateFrom').value,
            dateTo: document.getElementById('filterDateTo').value
        };

        // Remove empty filters
        Object.keys(this.filters).forEach(key => {
            if (!this.filters[key]) delete this.filters[key];
        });

        this.currentPage = 1;
        this.loadMessages();
    }

    changePage(page) {
        this.currentPage = page;
        this.loadMessages();
    }

    refreshMessages() {
        this.loadMessages();
        this.loadStats();
    }

    updateMessageCount(count) {
        document.getElementById('messageCount').textContent = `${count} messages`;
    }

    formatDateTime(dateString) {
        return new Date(dateString).toLocaleString();
    }

    formatBytes(bytes) {
        if (!bytes) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    showSuccess(message) {
        this.showAlert(message, 'success');
    }

    showError(message) {
        this.showAlert(message, 'danger');
    }

    showAlert(message, type) {
        const alertDiv = document.createElement('div');
        alertDiv.className = `alert alert-${type} alert-dismissible fade show position-fixed`;
        alertDiv.style.cssText = 'top: 20px; right: 20px; z-index: 9999; min-width: 300px;';
        alertDiv.innerHTML = `
            ${message}
            <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
        `;
        document.body.appendChild(alertDiv);

        setTimeout(() => {
            if (alertDiv.parentNode) {
                alertDiv.parentNode.removeChild(alertDiv);
            }
        }, 5000);
    }
}

// Global functions for onclick handlers
function showMessageDetail(messageId) {
    messageManager.showMessageDetail(messageId);
}

function showSendMessageModal() {
    document.getElementById('sendMessageModal').classList.add('show');
}

function closeSendMessageModal() {
    document.getElementById('sendMessageModal').classList.remove('show');
}

function showMessageDetailModal() {
    document.getElementById('messageDetailModal').classList.add('show');
}

function closeMessageDetailModal() {
    document.getElementById('messageDetailModal').classList.remove('show');
}

function sendMessage() {
    messageManager.sendMessage();
}

function reprocessMessage() {
    messageManager.reprocessMessage();
}

function deleteMessage() {
    messageManager.deleteMessage();
}

// Logout function
function logout() {
    fetch('/api/logout', { method: 'POST' })
    .then(() => {
        window.location.href = 'login.html';
    })
    .catch(error => {
        console.error('Logout error:', error);
        window.location.href = 'login.html';
    });
}

function refreshMessages() {
    messageManager.refreshMessages();
}

function applyFilters() {
    messageManager.applyFilters();
}

function loadSampleMessage(type) {
    messageManager.loadSampleMessage(type);
}

function formatJson() {
    messageManager.formatJson();
}

function logout() {
    fetch('/api/logout', { method: 'POST' })
        .then(() => window.location.href = 'login.html');
}

// Tab switching functionality
function switchTab(tabName) {
    // Hide all tab contents
    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });

    // Remove active class from all tab buttons
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.classList.remove('active');
    });

    // Show selected tab content
    document.getElementById(tabName + 'Tab').classList.add('active');

    // Add active class to clicked button
    event.target.classList.add('active');

    // Load tab-specific content
    if (tabName === 'lineage' && messageManager.selectedMessageId) {
        messageManager.loadDataLineage(messageManager.selectedMessageId);
    } else if (tabName === 'content' && messageManager.selectedMessageId) {
        messageManager.loadMessageContent(messageManager.selectedMessageId);
    } else if (tabName === 'transformations' && messageManager.selectedMessageId) {
        messageManager.loadTransformations(messageManager.selectedMessageId);
    }
}

// Initialize when page loads
let messageManager;
document.addEventListener('DOMContentLoaded', () => {
    messageManager = new MessageManager();
});