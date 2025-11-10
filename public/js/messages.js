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

            // Set default date filter (last 1 day)
            this.setDefaultDateFilter();

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
                    <label class="form-label text-muted">
                        <i class="fas fa-filter me-1"></i>Current Interface:
                    </label>
                    <select class="form-select form-select-sm" id="currentInterfaceSelect" onchange="messageManager.switchInterface(this.value)">
                        <option value="">Loading interfaces...</option>
                    </select>
                </div>
            `;
            pageHeader.insertAdjacentHTML('afterbegin', selectorHTML);
        }
    }

    setDefaultDateFilter() {
        // Set date filter to last 1 day by default
        const now = new Date();
        const yesterday = new Date(now.getTime() - (24 * 60 * 60 * 1000)); // 1 day ago

        // Format dates for datetime-local input (YYYY-MM-DDTHH:MM)
        const formatDateTimeLocal = (date) => {
            const year = date.getFullYear();
            const month = String(date.getMonth() + 1).padStart(2, '0');
            const day = String(date.getDate()).padStart(2, '0');
            const hours = String(date.getHours()).padStart(2, '0');
            const minutes = String(date.getMinutes()).padStart(2, '0');
            return `${year}-${month}-${day}T${hours}:${minutes}`;
        };

        const dateFromInput = document.getElementById('filterDateFrom');
        const dateToInput = document.getElementById('filterDateTo');

        if (dateFromInput) {
            dateFromInput.value = formatDateTimeLocal(yesterday);
            this.filters.dateFrom = formatDateTimeLocal(yesterday);
        }

        if (dateToInput) {
            dateToInput.value = formatDateTimeLocal(now);
            this.filters.dateTo = formatDateTimeLocal(now);
        }

        console.log('✅ Default date filter set: Last 1 day');
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
                limit: this.pageSize,
                sortBy: 'received_at',  // Sort by received_at by default
                sortOrder: 'desc'       // Descending order (newest first)
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
                    ${this.renderProcessingTime(message)}
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
            // Reception statuses
            'received': { class: 'bg-info', text: 'Received' },
            'queued': { class: 'bg-secondary', text: 'Queued' },

            // Processing statuses
            'processing': { class: 'bg-warning text-dark', text: 'Processing' },
            'parsed': { class: 'bg-primary', text: 'Parsed' },
            'parsing_failed': { class: 'bg-danger', text: 'Parsing Failed' },
            'transformed': { class: 'bg-primary', text: 'Transformed' },
            'transformation_failed': { class: 'bg-danger', text: 'Transform Failed' },

            // Delivery statuses
            'sent': { class: 'bg-success', text: 'Sent' },
            'delivered': { class: 'bg-success', text: 'Delivered' },
            'delivery_failed': { class: 'bg-danger', text: 'Delivery Failed' },

            // Error statuses
            'failed': { class: 'bg-danger', text: 'Failed' },
            'error': { class: 'bg-danger', text: 'Error' },

            // Retry statuses
            'reprocessing': { class: 'bg-warning text-dark', text: 'Reprocessing' },
            'retry': { class: 'bg-warning text-dark', text: 'Retry' }
        };

        const config = statusConfig[status] || { class: 'bg-secondary', text: status };
        return `<span class="badge ${config.class} status-badge">${config.text}</span>`;
    }

    renderProcessingTime(message) {
        // 1. If processing_time_ms is explicitly set, use it
        if (message.processing_time_ms != null && message.processing_time_ms > 0) {
            return `${message.processing_time_ms}ms`;
        }

        // 2. If processing is completed, calculate from timestamps
        if (message.processing_completed_at && message.received_at) {
            try {
                const receivedAt = new Date(message.received_at);
                const completedAt = new Date(message.processing_completed_at);
                const diffMs = completedAt - receivedAt;

                if (diffMs >= 0) {
                    return `${Math.round(diffMs)}ms`;
                }
            } catch (e) {
                console.error('Error calculating processing time:', e);
            }
        }

        // 3. If still processing or queued, show status indicator
        const inProgressStatuses = ['processing', 'queued', 'reprocessing'];
        if (inProgressStatuses.includes(message.status)) {
            return '<span class="text-muted"><i class="fas fa-spinner fa-pulse"></i> Processing...</span>';
        }

        // 4. If just received, show pending
        if (message.status === 'received') {
            return '<span class="text-muted">Pending</span>';
        }

        // 5. If parsed but no processing time, it was instant
        if (message.status === 'parsed' || message.status === 'transformed') {
            return '<span class="text-success">< 1ms</span>';
        }

        // 6. Default: no time available
        return '<span class="text-muted">-</span>';
    }

    calculateProcessingTimeForDetail(message) {
        // 1. If processing_time_ms is explicitly set, use it
        if (message.processing_time_ms != null && message.processing_time_ms > 0) {
            return `${message.processing_time_ms}ms`;
        }

        // 2. If parsing_time_ms is set, use that
        if (message.parsing_time_ms != null && message.parsing_time_ms > 0) {
            return `${message.parsing_time_ms}ms (parsing)`;
        }

        // 3. If processing is completed, calculate from timestamps
        if (message.processing_completed_at && message.received_at) {
            try {
                const receivedAt = new Date(message.received_at);
                const completedAt = new Date(message.processing_completed_at);
                const diffMs = completedAt - receivedAt;

                if (diffMs >= 0) {
                    return `${Math.round(diffMs)}ms (calculated)`;
                }
            } catch (e) {
                console.error('Error calculating processing time:', e);
            }
        }

        // 4. If parsed_at exists, calculate from received to parsed
        if (message.parsed_at && message.received_at) {
            try {
                const receivedAt = new Date(message.received_at);
                const parsedAt = new Date(message.parsed_at);
                const diffMs = parsedAt - receivedAt;

                if (diffMs >= 0) {
                    return `${Math.round(diffMs)}ms (parsing)`;
                }
            } catch (e) {
                console.error('Error calculating parsing time:', e);
            }
        }

        // 5. If message was delivered, show approximate time
        if (message.status === 'delivered' || message.delivery_status === 'delivered') {
            return '< 1000ms (estimated)';
        }

        // 6. If parsed/transformed, show instant
        if (message.status === 'parsed' || message.status === 'transformed') {
            return '< 1ms';
        }

        // 7. Default
        return 'N/A';
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

                // Show/hide errors tab based on error_count (V23 - Error Handling Enhancement)
                const errorsTabBtn = document.getElementById('errorsTabBtn');
                if (errorsTabBtn) {
                    errorsTabBtn.style.display =
                        (message.error_count && message.error_count > 0) ? 'inline-block' : 'none';
                }
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
            const container = document.getElementById('messageLineageView');

            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading data lineage...</div>';

            // Get lineage data from backend
            const lineageResponse = await fetch(`/api/messages/${messageId}/lineage`);

            if (!lineageResponse.ok) {
                throw new Error('Failed to fetch lineage data');
            }

            const lineageData = await lineageResponse.json();

            if (!lineageData.success) {
                throw new Error(lineageData.error || 'Failed to load lineage');
            }

            this.renderDataLineage(lineageData.data);
        } catch (error) {
            console.error('Failed to load data lineage:', error);
            document.getElementById('messageLineageView').innerHTML =
                `<div class="alert alert-warning">
                    <h6>⚠️ Unable to load data lineage</h6>
                    <p>${error.message}</p>
                </div>`;
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

    renderDataLineage(lineage) {
        const container = document.getElementById('messageLineageView');

        // Check if we have the new lineage structure with input/transformation/output/target
        if (!lineage.input) {
            container.innerHTML = '<div class="alert alert-info">No lineage data available</div>';
            return;
        }

        // Store lineage data for copy functionality
        this.currentLineageData = lineage;

        const { input, transformation, output, target, flowStatus } = lineage;

        container.innerHTML = `
            <div class="lineage-container" style="max-width: 1400px; margin: 0 auto;">
                <!-- Header -->
                <div style="background: linear-gradient(to right, white 0%, #fdf2f8 100%); padding: 1.5rem; border-radius: 8px; margin-bottom: 1.5rem; border-left: 4px solid #ec4899; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                    <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
                        <div>
                            <h5 style="color: #1e3a8a; margin: 0; font-weight: 600; font-size: 1.25rem;">
                                Message Journey
                            </h5>
                            <p style="color: #64748b; margin: 0.25rem 0 0 0; font-size: 0.9rem;">
                                <code style="background: #fce7f3; padding: 0.25rem 0.5rem; border-radius: 4px; color: #ec4899; font-size: 0.85rem;">${lineage.correlationId}</code>
                            </p>
                        </div>
                        <div style="display: flex; gap: 0.75rem;">
                            <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem; background: ${flowStatus.inputReceived ? '#fce7f3' : '#fef2f2'}; border-radius: 6px; border: 1px solid ${flowStatus.inputReceived ? '#fbcfe8' : '#fecaca'};">
                                <div style="width: 8px; height: 8px; border-radius: 50%; background: ${flowStatus.inputReceived ? '#ec4899' : '#ef4444'};"></div>
                                <span style="font-size: 0.85rem; color: #64748b; font-weight: 500;">Received</span>
                            </div>
                            <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem; background: ${flowStatus.transformed ? '#fce7f3' : '#fef2f2'}; border-radius: 6px; border: 1px solid ${flowStatus.transformed ? '#fbcfe8' : '#fecaca'};">
                                <div style="width: 8px; height: 8px; border-radius: 50%; background: ${flowStatus.transformed ? '#ec4899' : '#ef4444'};"></div>
                                <span style="font-size: 0.85rem; color: #64748b; font-weight: 500;">Transformed</span>
                            </div>
                            <div style="display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem; background: ${flowStatus.delivered ? '#fce7f3' : '#fef2f2'}; border-radius: 6px; border: 1px solid ${flowStatus.delivered ? '#fbcfe8' : '#fecaca'};">
                                <div style="width: 8px; height: 8px; border-radius: 50%; background: ${flowStatus.delivered ? '#ec4899' : '#ef4444'};"></div>
                                <span style="font-size: 0.85rem; color: #64748b; font-weight: 500;">Delivered</span>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Step 1: Input Message -->
                <div style="background: white; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); border-left: 4px solid #1e3a8a;">
                    <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem;">
                        <div style="width: 40px; height: 40px; background: #eff6ff; border-radius: 8px; display: flex; align-items: center; justify-content: center;">
                            <svg style="width: 20px; height: 20px; color: #1e3a8a;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
                            </svg>
                        </div>
                        <div style="flex: 1;">
                            <h6 style="color: #1e3a8a; margin: 0; font-weight: 600;">1. Message Received</h6>
                            <p style="color: #64748b; margin: 0; font-size: 0.85rem;">${this.formatDateTime(input.receivedAt)}</p>
                        </div>
                        <div style="background: #fce7f3; color: #ec4899; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">
                            ${input.messageType}
                        </div>
                    </div>
                    <div class="row g-3">
                        <div class="col-md-4">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; height: 100%;">
                                <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                                    <tr><th style="width: 45%; color: #64748b; font-weight: 500;">Interface:</th><td style="color: #1e293b;"><strong>${input.interfaceName}</strong></td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Message ID:</th><td style="color: #1e293b; font-family: monospace; font-size: 0.8rem;">${input.messageId}</td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Source:</th><td style="color: #1e293b;">${input.sourceType}</td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Size:</th><td style="color: #1e293b;">${this.formatBytes(input.messageSize)}</td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Status:</th><td>${this.renderStatusBadge(input.status)}</td></tr>
                                </table>
                            </div>
                        </div>
                        <div class="col-md-8">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; height: 100%; position: relative;">
                                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem;">
                                    <div style="color: #64748b; font-size: 0.85rem; font-weight: 500;">Raw HL7 Message</div>
                                    ${input.rawContent ? `<button onclick="messageManager.copyRawMessage()"
                                            style="background: #ec4899; color: white; border: none; padding: 0.35rem 0.65rem; border-radius: 4px; cursor: pointer; font-size: 0.8rem; display: flex; align-items: center; gap: 0.35rem;"
                                            title="Copy raw message">
                                        <svg style="width: 13px; height: 13px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                                        </svg>
                                        Copy
                                    </button>` : ''}
                                </div>
                                ${input.rawContent ? this.formatMessageContent(input.rawContent) : '<div style="padding: 2rem; text-align: center; color: #cbd5e1;"><svg style="width: 48px; height: 48px; margin-bottom: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg><p style="margin: 0;">No content available</p></div>'}
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Step 2: Transformation -->
                ${transformation ? `
                <div style="background: white; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); border-left: 4px solid #fce7f3;">
                    <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem;">
                        <div style="width: 40px; height: 40px; background: #fce7f3; border-radius: 8px; display: flex; align-items: center; justify-content: center;">
                            <svg style="width: 20px; height: 20px; color: #ec4899;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>
                            </svg>
                        </div>
                        <div style="flex: 1;">
                            <h6 style="color: #1e3a8a; margin: 0; font-weight: 600;">2. HL7 → FHIR Transformation</h6>
                            <p style="color: #64748b; margin: 0; font-size: 0.85rem;">${transformation.parsedAt ? this.formatDateTime(transformation.parsedAt) : 'N/A'}</p>
                        </div>
                        <div style="background: #eff6ff; color: #1e3a8a; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">
                            ${transformation.parsingTimeMs || 0}ms
                        </div>
                    </div>
                    <div class="row g-3">
                        <div class="col-md-4">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px;">
                                <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                                    <tr><th style="width: 45%; color: #64748b; font-weight: 500;">Status:</th><td><span class="badge bg-success">Success</span></td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Parse Time:</th><td style="color: #1e293b;"><strong>${transformation.parsingTimeMs || 0}ms</strong></td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Format:</th><td style="color: #1e293b;">HL7 v2 → FHIR R4</td></tr>
                                </table>
                            </div>
                        </div>
                        <div class="col-md-8">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px;">
                                <div style="color: #64748b; font-size: 0.85rem; font-weight: 500; margin-bottom: 0.75rem;">Transformation Output</div>
                                <div style="padding: 1rem; background: white; border-radius: 4px; border: 1px solid #e2e8f0;">
                                    <div style="display: flex; align-items: center; gap: 0.5rem; color: #64748b; font-size: 0.9rem;">
                                        <svg style="width: 16px; height: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                        </svg>
                                        <span>Parsed HL7 structure and converted to FHIR Bundle</span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
                ` : '<div style="background: white; border-radius: 8px; padding: 2rem; text-align: center; color: #cbd5e1; box-shadow: 0 1px 3px rgba(0,0,0,0.05);"><svg style="width: 48px; height: 48px; margin-bottom: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"/></svg><p style="margin: 0;">No transformation data</p></div>'}

                <!-- Step 3: Output & Delivery -->
                ${output ? `
                <div style="background: white; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); border-left: 4px solid #1e3a8a;">
                    <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem;">
                        <div style="width: 40px; height: 40px; background: #eff6ff; border-radius: 8px; display: flex; align-items: center; justify-content: center;">
                            <svg style="width: 20px; height: 20px; color: #1e3a8a;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8"/>
                            </svg>
                        </div>
                        <div style="flex: 1;">
                            <h6 style="color: #1e3a8a; margin: 0; font-weight: 600;">3. Sent to Destination</h6>
                            <p style="color: #64748b; margin: 0; font-size: 0.85rem;">${output.deliveryCompletedAt ? this.formatDateTime(output.deliveryCompletedAt) : 'In progress'}</p>
                        </div>
                        <div style="display: flex; gap: 0.5rem;">
                            <div style="background: ${output.deliveryStatus === 'delivered' ? '#f0fdf4' : '#fef2f2'}; color: ${output.deliveryStatus === 'delivered' ? '#166534' : '#991b1b'}; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">
                                ${output.deliveryStatus}
                            </div>
                            <div style="background: #eff6ff; color: #1e3a8a; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">
                                ${output.deliveryTimeMs || 0}ms
                            </div>
                        </div>
                    </div>
                    <div class="row g-3">
                        <div class="col-md-4">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px;">
                                <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                                    <tr><th style="width: 50%; color: #64748b; font-weight: 500;">HTTP Status:</th><td style="color: #1e293b;"><span class="badge" style="background: #dcfce7; color: #166534;">${output.deliveryStatusCode || 'N/A'}</span></td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Transform Time:</th><td style="color: #1e293b;">${output.transformationTimeMs || 0}ms</td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Delivery Time:</th><td style="color: #1e293b;">${output.deliveryTimeMs || 0}ms</td></tr>
                                    <tr><th style="color: #64748b; font-weight: 500;">Retry Count:</th><td style="color: #1e293b;">${output.retryCount || 0}</td></tr>
                                </table>
                            </div>
                        </div>
                        <div class="col-md-8">
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; height: 100%;">
                                <div style="color: #64748b; font-size: 0.85rem; font-weight: 500; margin-bottom: 0.75rem;">FHIR Bundle Sent</div>
                                ${output.transformedMessage ? (
                                    // Check if it's MongoDB metadata or actual FHIR content
                                    (typeof output.transformedMessage === 'object' && output.transformedMessage.mongo_reference) ?
                                    `<div style="padding: 1.5rem; background: white; border-radius: 6px; border: 1px solid #e2e8f0;">
                                        <div style="display: flex; align-items: start; gap: 1rem;">
                                            <div style="width: 40px; height: 40px; background: white; border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                                                <svg style="width: 20px; height: 20px; color: #ec4899;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 19a2 2 0 01-2-2V7a2 2 0 012-2h4l2 2h4a2 2 0 012 2v1M5 19h14a2 2 0 002-2v-5a2 2 0 00-2-2H9a2 2 0 00-2 2v5a2 2 0 01-2 2z"/>
                                                </svg>
                                            </div>
                                            <div style="flex: 1;">
                                                <div style="color: #1e3a8a; font-weight: 600; margin-bottom: 0.5rem;">✓ FHIR Bundle Stored</div>
                                                <div style="color: #64748b; font-size: 0.9rem; margin-bottom: 0.75rem;">Full FHIR R4 Bundle successfully stored in MongoDB for scalability</div>
                                                <div style="background: white; padding: 0.75rem; border-radius: 4px; font-family: monospace; font-size: 0.8rem; color: #ec4899; word-break: break-all;">
                                                    ${output.transformedMessage.mongo_reference}
                                                </div>
                                            </div>
                                        </div>
                                    </div>` :
                                    this.renderFHIRBundle(output.transformedMessage)
                                ) : '<div style="padding: 2rem; text-align: center; color: #cbd5e1;"><svg style="width: 48px; height: 48px; margin-bottom: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/></svg><p style="margin: 0;">No FHIR content available</p></div>'}
                            </div>
                        </div>
                    </div>
                </div>
                ` : '<div style="background: white; border-radius: 8px; padding: 2rem; text-align: center; color: #cbd5e1; box-shadow: 0 1px 3px rgba(0,0,0,0.05);"><svg style="width: 48px; height: 48px; margin-bottom: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"/></svg><p style="margin: 0;">No output data</p></div>'}

                <!-- Step 4: Target Response -->
                ${target ? `
                <div style="background: white; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); border-left: 4px solid #fce7f3;">
                    <div style="display: flex; align-items: center; gap: 1rem; margin-bottom: 1.5rem;">
                        <div style="width: 40px; height: 40px; background: #fce7f3; border-radius: 8px; display: flex; align-items: center; justify-content: center;">
                            <svg style="width: 20px; height: 20px; color: #ec4899;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                            </svg>
                        </div>
                        <div style="flex: 1;">
                            <h6 style="color: #1e3a8a; margin: 0; font-weight: 600;">4. Response from ${target.interfaceName}</h6>
                            <p style="color: #64748b; margin: 0; font-size: 0.85rem;">${this.formatDateTime(target.receivedAt)}</p>
                        </div>
                        <div style="background: #f0fdf4; color: #166534; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">
                            Acknowledged
                        </div>
                    </div>
                    <div style="background: #f8fafc; padding: 1rem; border-radius: 6px;">
                        <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                            <tr><th style="width: 30%; color: #64748b; font-weight: 500;">Target:</th><td style="color: #1e293b;"><strong>${target.interfaceName}</strong></td></tr>
                            <tr><th style="color: #64748b; font-weight: 500;">Message ID:</th><td style="color: #1e293b; font-family: monospace; font-size: 0.8rem;">${target.messageId}</td></tr>
                            <tr><th style="color: #64748b; font-weight: 500;">Status:</th><td>${this.renderStatusBadge(target.status)}</td></tr>
                            <tr><th style="color: #64748b; font-weight: 500;">Size:</th><td style="color: #1e293b;">${this.formatBytes(target.messageSize)}</td></tr>
                        </table>
                    </div>
                </div>
                ` : '<div style="background: white; border-radius: 8px; padding: 2rem; text-align: center; color: #cbd5e1; box-shadow: 0 1px 3px rgba(0,0,0,0.05);"><svg style="width: 48px; height: 48px; margin-bottom: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg><p style="margin: 0;">Awaiting target acknowledgment</p></div>'}

                <!-- Complete Timeline -->
                <div style="background: white; border-radius: 8px; padding: 1.5rem; margin-top: 2rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                    <h6 style="color: #1e3a8a; margin-bottom: 1.5rem; font-weight: 600;">
                        <svg style="width: 20px; height: 20px; display: inline-block; margin-right: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/>
                        </svg>
                        Complete Timeline
                    </h6>
                    <div style="position: relative; padding-left: 2rem;">
                        <!-- Timeline line -->
                        <div style="position: absolute; left: 19px; top: 0; bottom: 0; width: 2px; background: #e2e8f0;"></div>

                        <!-- Timeline item: Received -->
                        <div style="position: relative; padding-bottom: 2rem;">
                            <div style="position: absolute; left: -2rem; width: 40px; height: 40px; background: #eff6ff; border: 3px solid #1e3a8a; border-radius: 50%; display: flex; align-items: center; justify-content: center;">
                                <div style="width: 12px; height: 12px; background: #1e3a8a; border-radius: 50%;"></div>
                            </div>
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; margin-left: 1.5rem;">
                                <div style="display: flex; justify-content: between; align-items: center; margin-bottom: 0.5rem;">
                                    <strong style="color: #1e293b;">Message Received</strong>
                                    <span style="color: #64748b; font-size: 0.85rem; margin-left: auto;">${this.formatDateTime(input.receivedAt)}</span>
                                </div>
                                <div style="color: #64748b; font-size: 0.9rem;">Received at ${input.interfaceName} interface</div>
                            </div>
                        </div>

                        ${transformation ? `
                        <!-- Timeline item: Transformed -->
                        <div style="position: relative; padding-bottom: 2rem;">
                            <div style="position: absolute; left: -2rem; width: 40px; height: 40px; background: #fce7f3; border: 3px solid #ec4899; border-radius: 50%; display: flex; align-items: center; justify-content: center;">
                                <div style="width: 12px; height: 12px; background: #ec4899; border-radius: 50%;"></div>
                            </div>
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; margin-left: 1.5rem;">
                                <div style="display: flex; justify-content: between; align-items: center; margin-bottom: 0.5rem;">
                                    <strong style="color: #1e293b;">HL7 → FHIR Transformation</strong>
                                    <span style="color: #64748b; font-size: 0.85rem; margin-left: auto;">${transformation.parsedAt ? this.formatDateTime(transformation.parsedAt) : 'N/A'}</span>
                                </div>
                                <div style="color: #64748b; font-size: 0.9rem;">Completed in ${transformation.parsingTimeMs || 0}ms</div>
                            </div>
                        </div>
                        ` : ''}

                        ${output ? `
                        <!-- Timeline item: Delivered -->
                        <div style="position: relative; padding-bottom: 2rem;">
                            <div style="position: absolute; left: -2rem; width: 40px; height: 40px; background: #eff6ff; border: 3px solid #1e3a8a; border-radius: 50%; display: flex; align-items: center; justify-content: center;">
                                <div style="width: 12px; height: 12px; background: #1e3a8a; border-radius: 50%;"></div>
                            </div>
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; margin-left: 1.5rem;">
                                <div style="display: flex; justify-content: between; align-items: center; margin-bottom: 0.5rem;">
                                    <strong style="color: #1e293b;">Sent to Destination</strong>
                                    <span style="color: #64748b; font-size: 0.85rem; margin-left: auto;">${output.deliveryCompletedAt ? this.formatDateTime(output.deliveryCompletedAt) : 'In progress'}</span>
                                </div>
                                <div style="color: #64748b; font-size: 0.9rem;">HTTP ${output.deliveryStatusCode || 'N/A'} - Delivered in ${output.deliveryTimeMs || 0}ms</div>
                            </div>
                        </div>
                        ` : ''}

                        ${target ? `
                        <!-- Timeline item: Acknowledged -->
                        <div style="position: relative;">
                            <div style="position: absolute; left: -2rem; width: 40px; height: 40px; background: #fce7f3; border: 3px solid #ec4899; border-radius: 50%; display: flex; align-items: center; justify-content: center;">
                                <svg style="width: 16px; height: 16px; color: #ec4899;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"/>
                                </svg>
                            </div>
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; margin-left: 1.5rem;">
                                <div style="display: flex; justify-content: between; align-items: center; margin-bottom: 0.5rem;">
                                    <strong style="color: #1e293b;">Target Acknowledged</strong>
                                    <span style="color: #64748b; font-size: 0.85rem; margin-left: auto;">${this.formatDateTime(target.receivedAt)}</span>
                                </div>
                                <div style="color: #64748b; font-size: 0.9rem;">Received at ${target.interfaceName}</div>
                            </div>
                        </div>
                        ` : ''}
                    </div>
                </div>
            </div>
        `;
    }


    async loadTransformations(messageId) {
        try {
            const container = document.getElementById('messageTransformationsView');

            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading transformation pipeline...</div>';

            // Get lineage data from backend which includes transformation steps
            const lineageResponse = await fetch(`/api/messages/${messageId}/lineage`);

            if (!lineageResponse.ok) {
                throw new Error('Failed to fetch lineage data');
            }

            const lineageData = await lineageResponse.json();

            if (!lineageData.success) {
                throw new Error(lineageData.error || 'Failed to load lineage');
            }

            const transformationSteps = lineageData.data?.output?.transformation_steps || [];

            if (transformationSteps.length === 0) {
                container.innerHTML = `
                    <div style="max-width: 1400px; margin: 0 auto;">
                        <div style="background: linear-gradient(135deg, #eff6ff 0%, #fdf2f8 100%); padding: 2rem; border-radius: 8px; text-align: center; border: 2px dashed #e2e8f0;">
                            <svg style="width: 48px; height: 48px; color: #94a3b8; margin-bottom: 1rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                            </svg>
                            <h6 style="color: #1e3a8a; margin-bottom: 0.5rem;">No Transformations Found</h6>
                            <p style="color: #64748b; margin: 0;">This message was not transformed or transformation data is not available.</p>
                        </div>
                    </div>
                `;
                return;
            }

            // Render transformation steps
            const transformationsHtml = transformationSteps.map((step, index) => {
                const stepTypeColors = {
                    'pre.validation': { bg: '#dbeafe', color: '#1e40af', icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z' },
                    'core.mapping': { bg: '#fce7f3', color: '#ec4899', icon: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15' },
                    'post.validation': { bg: '#dcfce7', color: '#16a34a', icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z' }
                };
                const typeStyle = stepTypeColors[step.steptype] || stepTypeColors['core.mapping'];
                const success = step.success;
                const duration = step.durationms?.low || 0;

                return `
                <div style="background: white; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); margin-bottom: 1.5rem; border-left: 4px solid ${typeStyle.color};">
                    <div style="display: flex; align-items: start; gap: 1rem; margin-bottom: 1.5rem;">
                        <div style="width: 40px; height: 40px; background: ${typeStyle.bg}; border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                            <svg style="width: 20px; height: 20px; color: ${typeStyle.color};" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${typeStyle.icon}"/>
                            </svg>
                        </div>
                        <div style="flex: 1;">
                            <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
                                <div>
                                    <h6 style="color: #1e3a8a; margin: 0; font-weight: 600; font-size: 1rem;">${step.stepname || `Step ${index + 1}`}</h6>
                                    <div style="display: flex; gap: 0.5rem; margin-top: 0.5rem; flex-wrap: wrap;">
                                        <span style="background: ${typeStyle.bg}; color: ${typeStyle.color}; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.8rem; font-weight: 500;">${step.steptype}</span>
                                        ${success ?
                                            '<span style="background: #dcfce7; color: #16a34a; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.8rem;">✓ Success</span>' :
                                            '<span style="background: #fee2e2; color: #dc2626; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.8rem;">✗ Failed</span>'
                                        }
                                        <span style="background: #f1f5f9; color: #64748b; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.8rem;">⏱ ${duration}ms</span>
                                    </div>
                                </div>
                            </div>
                            ${step.error ? `
                            <div style="margin-top: 1rem; padding: 0.75rem; background: #fee2e2; border-left: 3px solid #dc2626; border-radius: 4px;">
                                <div style="color: #dc2626; font-weight: 600; font-size: 0.85rem; margin-bottom: 0.25rem;">Error:</div>
                                <div style="color: #991b1b; font-size: 0.85rem;">${this.escapeHtml(step.error)}</div>
                            </div>
                            ` : ''}
                        </div>
                    </div>

                    <div style="display: grid; grid-template-columns: 1px 1fr; gap: 1rem;">
                        <div style="background: ${typeStyle.color}; width: 2px; margin-left: 19px;"></div>
                        <div>
                            <div style="background: #f8fafc; padding: 1rem; border-radius: 6px; border: 1px solid #e2e8f0;">
                                <div style="color: #64748b; font-size: 0.85rem; margin-bottom: 0.5rem; font-weight: 500;">Step Details</div>
                                <table style="width: 100%; font-size: 0.85rem;">
                                    <tr>
                                        <td style="color: #64748b; padding: 0.4rem 0;">Started:</td>
                                        <td style="color: #1e293b; padding: 0.4rem 0;">${new Date(step.startedat).toLocaleString()}</td>
                                    </tr>
                                    <tr>
                                        <td style="color: #64748b; padding: 0.4rem 0;">Completed:</td>
                                        <td style="color: #1e293b; padding: 0.4rem 0;">${new Date(step.completedat).toLocaleString()}</td>
                                    </tr>
                                    <tr>
                                        <td style="color: #64748b; padding: 0.4rem 0;">Duration:</td>
                                        <td style="color: #1e293b; padding: 0.4rem 0;"><strong>${duration}ms</strong></td>
                                    </tr>
                                    <tr>
                                        <td style="color: #64748b; padding: 0.4rem 0;">Step ID:</td>
                                        <td style="color: #1e293b; padding: 0.4rem 0; font-family: monospace; font-size: 0.75rem;">${step.stepid}</td>
                                    </tr>
                                </table>
                            </div>
                        </div>
                    </div>
                </div>
                `;
            }).join('');

            container.innerHTML = `
                <div style="max-width: 1400px; margin: 0 auto;">
                    <div style="background: linear-gradient(to right, white 0%, #fdf2f8 100%); padding: 1.5rem; border-radius: 8px; margin-bottom: 1.5rem; border-left: 4px solid #ec4899; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                        <h5 style="color: #1e3a8a; margin: 0; font-weight: 600;">Message Transformations</h5>
                        <p style="color: #64748b; margin: 0.25rem 0 0 0; font-size: 0.9rem;">View transformation steps and mapping rules</p>
                    </div>
                    ${transformationsHtml}
                </div>
            `;
        } catch (error) {
            console.error('Failed to load transformations:', error);
            document.getElementById('messageTransformationsView').innerHTML =
                '<div class="alert alert-warning">Unable to load transformation data.</div>';
        }
    }

    async loadErrors(messageId) {
        try {
            const container = document.getElementById('messageErrorsView');
            const message = this.messageData.message;

            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading errors...</div>';

            // Fetch errors from API
            const response = await fetch(`/api/messages/${messageId}/errors?interfaceId=${message.interface_id}`);

            if (!response.ok) {
                throw new Error('Failed to fetch errors');
            }

            const result = await response.json();

            if (!result.success) {
                throw new Error(result.error || 'Failed to load errors');
            }

            const { errors, summary } = result.data;

            // If no errors, show a success message
            if (!errors || errors.length === 0) {
                container.innerHTML = `
                    <div style="max-width: 1400px; margin: 0 auto;">
                        <div style="background: linear-gradient(135deg, #dcfce7 0%, #f0fdf4 100%); padding: 3rem; border-radius: 8px; text-align: center; border: 2px dashed #86efac;">
                            <svg style="width: 64px; height: 64px; color: #16a34a; margin-bottom: 1rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/>
                            </svg>
                            <h5 style="color: #166534; margin-bottom: 0.5rem; font-weight: 600;">No Errors or Warnings</h5>
                            <p style="color: #16a34a; margin: 0;">This message processed successfully without any errors.</p>
                        </div>
                    </div>
                `;
                // Hide the errors tab if no errors
                document.getElementById('errorsTabBtn').style.display = 'none';
                return;
            }

            // Render errors
            container.innerHTML = this.renderErrorStack(errors, summary);

        } catch (error) {
            console.error('Failed to load errors:', error);
            document.getElementById('messageErrorsView').innerHTML =
                '<div class="alert alert-warning">Unable to load error data.</div>';
        }
    }

    renderErrorStack(errors, summary) {
        const severityConfig = {
            'critical': {
                bg: '#fee2e2',
                color: '#dc2626',
                icon: '🔴',
                label: 'CRITICAL'
            },
            'error': {
                bg: '#fed7aa',
                color: '#ea580c',
                icon: '❌',
                label: 'ERROR'
            },
            'warning': {
                bg: '#fef3c7',
                color: '#d97706',
                icon: '⚠️',
                label: 'WARNING'
            }
        };

        const errorsByType = errors.reduce((acc, err) => {
            const severity = err.severity || 'error';
            if (!acc[severity]) acc[severity] = [];
            acc[severity].push(err);
            return acc;
        }, {});

        const errorHtml = Object.entries(errorsByType)
            .sort(([a], [b]) => {
                const order = { 'critical': 0, 'error': 1, 'warning': 2 };
                return order[a] - order[b];
            })
            .map(([severity, severityErrors]) => {
                const config = severityConfig[severity];
                return severityErrors.map((err, index) => {
                    const timestamp = new Date(err.error_timestamp).toLocaleString();
                    const hasStackTrace = err.stack_trace && err.stack_trace.length > 0;

                    return `
                        <div style="background: white; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05); margin-bottom: 1.5rem; border-left: 4px solid ${config.color};">
                            <div style="display: flex; align-items: start; gap: 1rem; margin-bottom: 1rem;">
                                <div style="width: 48px; height: 48px; background: ${config.bg}; border-radius: 8px; display: flex; align-items: center; justify-content: center; flex-shrink: 0; font-size: 24px;">
                                    ${config.icon}
                                </div>
                                <div style="flex: 1;">
                                    <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem; margin-bottom: 0.75rem;">
                                        <div>
                                            <span style="background: ${config.bg}; color: ${config.color}; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.75rem; font-weight: 600; margin-right: 0.5rem;">${config.label}</span>
                                            <span style="color: #64748b; font-size: 0.85rem;">${timestamp}</span>
                                        </div>
                                        <span style="background: #f1f5f9; color: #64748b; padding: 0.25rem 0.65rem; border-radius: 4px; font-size: 0.75rem;">${err.stage || 'unknown'}</span>
                                    </div>
                                    <h6 style="color: #1e3a8a; margin: 0 0 0.5rem 0; font-weight: 600; font-size: 1rem;">${this.escapeHtml(err.message)}</h6>
                                    <div style="color: #64748b; font-size: 0.9rem; margin-bottom: 0.75rem;">
                                        <strong>Type:</strong> ${err.error_type || 'Unknown'}
                                        ${err.recovery_action ? `<span style="margin-left: 1rem;"><strong>Recovery:</strong> ${err.recovery_action}</span>` : ''}
                                    </div>
                                    ${err.details ? `
                                        <div style="background: #f8fafc; padding: 0.75rem; border-radius: 4px; border-left: 3px solid ${config.color}; margin-bottom: 0.75rem;">
                                            <div style="color: ${config.color}; font-weight: 600; font-size: 0.85rem; margin-bottom: 0.25rem;">Details:</div>
                                            <div style="color: #1e293b; font-size: 0.85rem; white-space: pre-wrap;">${this.escapeHtml(err.details)}</div>
                                        </div>
                                    ` : ''}
                                    ${hasStackTrace ? `
                                        <details style="margin-top: 0.75rem;">
                                            <summary style="cursor: pointer; color: #64748b; font-size: 0.85rem; padding: 0.5rem; background: #f8fafc; border-radius: 4px;">
                                                <i class="fas fa-code"></i> View Stack Trace
                                            </summary>
                                            <div style="margin-top: 0.75rem; background: #1e293b; color: #f8fafc; padding: 1rem; border-radius: 4px; overflow-x: auto; font-family: 'Courier New', monospace; font-size: 0.75rem; max-height: 300px; overflow-y: auto;">
                                                <pre style="margin: 0; white-space: pre-wrap; word-wrap: break-word;">${this.escapeHtml(err.stack_trace)}</pre>
                                            </div>
                                        </details>
                                    ` : ''}
                                </div>
                            </div>
                        </div>
                    `;
                }).join('');
            }).join('');

        return `
            <div style="max-width: 1400px; margin: 0 auto;">
                <div style="background: linear-gradient(to right, white 0%, #fef2f2 100%); padding: 1.5rem; border-radius: 8px; margin-bottom: 1.5rem; border-left: 4px solid #dc2626; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                    <h5 style="color: #1e3a8a; margin: 0 0 0.5rem 0; font-weight: 600;">Errors & Warnings (${errors.length})</h5>
                    <div style="display: flex; gap: 1.5rem; flex-wrap: wrap; margin-top: 1rem;">
                        ${errorsByType.critical ? `<div><span style="font-weight: 600; color: #dc2626;">Critical:</span> ${errorsByType.critical.length}</div>` : ''}
                        ${errorsByType.error ? `<div><span style="font-weight: 600; color: #ea580c;">Errors:</span> ${errorsByType.error.length}</div>` : ''}
                        ${errorsByType.warning ? `<div><span style="font-weight: 600; color: #d97706;">Warnings:</span> ${errorsByType.warning.length}</div>` : ''}
                    </div>
                </div>
                ${errorHtml}
            </div>
        `;
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    getDeliveryStatusClass(status) {
        const statusClasses = {
            'pending': 'bg-secondary',
            'delivered': 'bg-success',
            'failed': 'bg-danger',
            'retrying': 'bg-warning text-dark'
        };
        return statusClasses[status] || 'bg-secondary';
    }

    formatMessageContent(content, contentType) {
        if (!content) return '<em class="text-muted">No content available</em>';

        try {
            // Try to parse as JSON and format it
            const parsed = JSON.parse(content);
            const formatted = JSON.stringify(parsed, null, 2);
            return `<pre class="bg-light p-3" style="max-height: 600px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word;"><code>${this.escapeHtml(formatted)}</code></pre>`;
        } catch (e) {
            // Not JSON, check if it's XML
            if (content.trim().startsWith('<')) {
                // Try to pretty-print XML
                try {
                    const formatted = this.formatXml(content);
                    return `<pre class="bg-light p-3" style="max-height: 600px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word;"><code>${this.escapeHtml(formatted)}</code></pre>`;
                } catch (xmlError) {
                    // XML formatting failed, show as-is
                }
            }

            // Show as plain text
            return `<pre class="bg-light p-3" style="max-height: 600px; overflow-y: auto; white-space: pre-wrap; word-wrap: break-word;"><code>${this.escapeHtml(content)}</code></pre>`;
        }
    }

    renderFHIRBundle(fhirData) {
        if (!fhirData) return '<div style="padding: 2rem; text-align: center; color: #cbd5e1;">No FHIR data</div>';

        // Extract deliveryPayload if it exists, otherwise use the whole object
        const bundleToShow = fhirData.deliveryPayload || fhirData.fhirBundle || fhirData;
        const bundleStr = JSON.stringify(bundleToShow, null, 2);
        const bundleId = 'fhir-bundle-' + Date.now();

        return `
            <div>
                <div style="display: flex; justify-content: flex-end; margin-bottom: 0.5rem;">
                    <button onclick="messageManager.copyToClipboard('${bundleId}')"
                            style="background: #ec4899; color: white; border: none; padding: 0.35rem 0.65rem; border-radius: 4px; cursor: pointer; font-size: 0.8rem; display: flex; align-items: center; gap: 0.35rem;"
                            title="Copy to clipboard">
                        <svg style="width: 13px; height: 13px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/>
                        </svg>
                        Copy
                    </button>
                </div>
                <pre id="${bundleId}" style="background: #f8fafc; padding: 1rem; border-radius: 6px; border: 1px solid #e2e8f0; max-height: 500px; overflow: auto; margin: 0;"><code style="color: #1e293b; font-size: 0.85rem;">${this.escapeHtml(bundleStr)}</code></pre>
            </div>
        `;
    }

    copyToClipboard(elementId) {
        const element = document.getElementById(elementId);
        if (!element) return;

        const text = element.textContent;
        navigator.clipboard.writeText(text).then(() => {
            // Show success message
            const btn = event.target.closest('button');
            const originalHTML = btn.innerHTML;
            btn.innerHTML = `
                <svg style="width: 16px; height: 16px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>
                Copied!
            `;
            setTimeout(() => {
                btn.innerHTML = originalHTML;
            }, 2000);
        });
    }

    copyRawMessage() {
        // Get the raw message from the lineage data
        const lineageData = this.currentLineageData;
        if (!lineageData || !lineageData.input || !lineageData.input.rawContent) return;

        const text = lineageData.input.rawContent;
        navigator.clipboard.writeText(text).then(() => {
            // Find the copy button and update it
            const btn = event.target.closest('button');
            const originalHTML = btn.innerHTML;
            btn.innerHTML = `<svg style="width: 13px; height: 13px;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                        </svg> Copied!`;
            setTimeout(() => {
                btn.innerHTML = originalHTML;
            }, 2000);
        }).catch(err => {
            console.error('Failed to copy:', err);
        });
    }


    formatXml(xml) {
        // Simple XML formatter
        const PADDING = '  ';
        const reg = /(>)(<)(\/*)/g;
        let formatted = '';
        let pad = 0;

        xml = xml.replace(reg, '$1\r\n$2$3');
        xml.split('\r\n').forEach(node => {
            let indent = 0;
            if (node.match(/.+<\/\w[^>]*>$/)) {
                indent = 0;
            } else if (node.match(/^<\/\w/)) {
                if (pad !== 0) {
                    pad -= 1;
                }
            } else if (node.match(/^<\w([^>]*[^\/])?>.*$/)) {
                indent = 1;
            } else {
                indent = 0;
            }

            formatted += PADDING.repeat(pad) + node + '\r\n';
            pad += indent;
        });

        return formatted.trim();
    }

    renderMessageDetail(data) {
        const { message, content, transformations } = data;
        const container = document.getElementById('messageDetailContent');

        container.innerHTML = `
            <div style="max-width: 1400px; margin: 0 auto;">
                <!-- Message Overview Card -->
                <div style="background: linear-gradient(to right, white 0%, #fdf2f8 100%); padding: 1.5rem; border-radius: 8px; margin-bottom: 1.5rem; border-left: 4px solid #ec4899; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                    <div style="display: flex; justify-content: space-between; align-items: start; flex-wrap: wrap; gap: 1rem;">
                        <div>
                            <h5 style="color: #1e3a8a; margin: 0 0 0.5rem 0; font-weight: 600;">Message Details</h5>
                            <div style="display: flex; gap: 1rem; flex-wrap: wrap;">
                                <span style="font-size: 0.9rem; color: #64748b;">
                                    <strong>ID:</strong> <code style="background: #fce7f3; padding: 0.25rem 0.5rem; border-radius: 4px; color: #ec4899; font-size: 0.85rem;">${message.message_id}</code>
                                </span>
                                <span style="font-size: 0.9rem; color: #64748b;">
                                    <strong>Type:</strong> <span style="background: #fce7f3; color: #ec4899; padding: 0.25rem 0.75rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500;">${message.message_type || 'Unknown'}</span>
                                </span>
                            </div>
                        </div>
                        <div style="display: flex; gap: 0.75rem; align-items: center;">
                            ${this.renderStatusBadge(message.status)}
                            <span style="background: ${message.delivery_status === 'delivered' ? '#fce7f3' : '#fef2f2'}; color: ${message.delivery_status === 'delivered' ? '#ec4899' : '#991b1b'}; padding: 0.5rem 1rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500; border: 1px solid ${message.delivery_status === 'delivered' ? '#fbcfe8' : '#fecaca'};">
                                ${message.delivery_status || 'N/A'}
                            </span>
                        </div>
                    </div>
                </div>

                <!-- Info Grid -->
                <div class="row g-3">
                    <div class="col-md-6">
                        <div style="background: white; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                            <h6 style="color: #1e3a8a; margin-bottom: 1rem; font-weight: 600;">
                                <svg style="width: 18px; height: 18px; display: inline-block; margin-right: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                                </svg>
                                Message Information
                            </h6>
                            <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                                <tr><th style="width: 40%; color: #64748b; font-weight: 500;">Interface:</th><td style="color: #1e293b;"><strong>${message.interface_name}</strong></td></tr>
                                <tr><th style="color: #64748b; font-weight: 500;">Size:</th><td style="color: #1e293b;">${this.formatBytes(message.message_size)}</td></tr>
                                <tr><th style="color: #64748b; font-weight: 500;">Source:</th><td style="color: #1e293b;">${message.source_type} (${message.source_endpoint || 'N/A'})</td></tr>
                                <tr><th style="color: #64748b; font-weight: 500;">Received:</th><td style="color: #1e293b;">${this.formatDateTime(message.received_at)}</td></tr>
                                ${message.processing_completed_at ? `
                                    <tr><th style="color: #64748b; font-weight: 500;">Completed:</th><td style="color: #1e293b;">${this.formatDateTime(message.processing_completed_at)}</td></tr>
                                ` : ''}
                                <tr><th style="color: #64748b; font-weight: 500;">Processing Time:</th><td style="color: #1e293b;"><strong>${this.calculateProcessingTimeForDetail(message)}</strong></td></tr>
                                ${message.correlation_id ? `
                                    <tr><th style="color: #64748b; font-weight: 500;">Correlation ID:</th><td style="color: #64748b; font-family: monospace; font-size: 0.8rem;">${message.correlation_id}</td></tr>
                                ` : ''}
                            </table>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div style="background: white; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 3px rgba(0,0,0,0.05);">
                            <h6 style="color: #1e3a8a; margin-bottom: 1rem; font-weight: 600;">
                                <svg style="width: 18px; height: 18px; display: inline-block; margin-right: 0.5rem;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/>
                                </svg>
                                Processing Details
                            </h6>
                            <table class="table table-sm table-borderless mb-0" style="font-size: 0.9rem;">
                                <tr><th style="width: 45%; color: #64748b; font-weight: 500;">Delivery Attempts:</th><td style="color: #1e293b;">${message.delivery_attempts || 0}</td></tr>
                                ${message.source_ip ? `
                                    <tr><th style="color: #64748b; font-weight: 500;">Source IP:</th><td style="color: #1e293b;">${message.source_ip}</td></tr>
                                ` : ''}
                                ${message.error_count > 0 ? `
                                    <tr><th style="color: #64748b; font-weight: 500;">Errors:</th><td><span style="color: #ef4444; font-weight: 600;">${message.error_count}</span></td></tr>
                                ` : ''}
                                ${message.last_error_message ? `
                                    <tr><th style="color: #64748b; font-weight: 500;">Last Error:</th><td style="color: #ef4444; font-size: 0.85rem;">${message.last_error_message}</td></tr>
                                ` : ''}
                            </table>
                        </div>
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

            <!-- Raw Message Content Section -->
            <div style="background: white; padding: 1.5rem; border-radius: 8px; border: 1px solid #e2e8f0; margin-top: 1.5rem;">
                <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 1rem;">
                    <div style="width: 32px; height: 32px; background: linear-gradient(135deg, #fce7f3 0%, #fdf2f8 100%); border-radius: 6px; display: flex; align-items: center; justify-content: center; flex-shrink: 0;">
                        <svg style="width: 18px; height: 18px; color: #ec4899;" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
                        </svg>
                    </div>
                    <div style="flex: 1;">
                        <h6 style="color: #1e3a8a; margin: 0; font-weight: 600;">Raw Message Content</h6>
                        <div style="display: flex; gap: 1rem; margin-top: 0.25rem;">
                            <span style="font-size: 0.85rem; color: #64748b;">
                                Size: <strong style="color: #ec4899;">${this.formatBytes(message.message_size)}</strong>
                            </span>
                            <span style="font-size: 0.85rem; color: #64748b;">
                                Encoding: <strong style="color: #ec4899;">${message.message_encoding || 'UTF-8'}</strong>
                            </span>
                        </div>
                    </div>
                </div>
                ${content.length > 0 ? content.map(c => `
                    <div style="margin-bottom: 1.5rem;">
                        <div style="background: #fdf2f8; padding: 0.5rem 0.75rem; border-radius: 4px; margin-bottom: 0.75rem; border-left: 3px solid #ec4899;">
                            <span style="font-size: 0.85rem; color: #1e3a8a; font-weight: 600; text-transform: capitalize;">
                                ${c.content_type.replace('_', ' ')} Content
                            </span>
                        </div>
                        <div class="message-content" style="background: #f8fafc; padding: 1rem; border-radius: 6px; border: 1px solid #e2e8f0;">
                            ${this.formatContent(c.content_data, c.content_type)}
                        </div>
                        <div style="margin-top: 0.5rem; font-size: 0.8rem; color: #94a3b8;">
                            Size: ${this.formatBytes(c.content_size)} | Encoding: ${c.content_encoding}
                        </div>
                    </div>
                `).join('') : message.raw_message ? `
                    <div class="message-content" style="background: #f8fafc; padding: 1rem; border-radius: 6px; border: 1px solid #e2e8f0;">
                        ${this.formatMessageContent(message.raw_message, message.message_encoding)}
                    </div>
                ` : `
                    <div style="padding: 2rem; text-align: center; color: #94a3b8; font-style: italic; background: #f8fafc; border-radius: 6px; border: 1px dashed #e2e8f0;">
                        No message content available
                    </div>
                `}
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
            let isSuccess = false;

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
                    isSuccess = true;
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

                const responseData = await response.json();
                console.log('🔍 Send message response:', responseData);

                if (response.ok && responseData.success) {
                    // Show detailed connectivity success message
                    const endpoint = responseData.interface?.endpoint || 'endpoint';
                    const method = responseData.delivery?.method || 'connectivity';
                    const interfaceName = responseData.interface?.name || 'Unknown';
                    const interfaceType = responseData.interface?.type || 'Unknown';
                    const messageId = responseData.messageId || 'Unknown';

                    this.showSuccess(`📡 Message sent via ${method.toUpperCase()}!<br>
                                     <small>Interface: ${interfaceName} (${interfaceType})<br>
                                     Endpoint: ${endpoint}<br>
                                     Message ID: ${messageId}</small>`);
                    isSuccess = true;
                } else {
                    // Show detailed connectivity error message
                    let errorMsg = responseData.error || 'Failed to send message';
                    if (responseData.details) {
                        errorMsg += `<br><small>Details: ${responseData.details}</small>`;
                    }
                    if (responseData.message) {
                        errorMsg += `<br><small>${responseData.message}</small>`;
                    }
                    this.showError(errorMsg);
                }
            }

            if (isSuccess) {
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
        // Add visual feedback for refresh
        const refreshBtn = document.querySelector('button[onclick="refreshMessages()"]');
        const refreshIcon = refreshBtn?.querySelector('i');

        if (refreshIcon) {
            // Add spinning animation
            refreshIcon.classList.add('fa-spin');
            refreshBtn.disabled = true;
        }

        // Show toast notification
        this.showSuccess('Refreshing messages...');

        // Perform refresh
        Promise.all([
            this.loadMessages(),
            this.loadStats()
        ]).then(() => {
            // Remove spinning animation
            if (refreshIcon) {
                refreshIcon.classList.remove('fa-spin');
                refreshBtn.disabled = false;
            }
            this.showSuccess('Messages refreshed successfully!');
        }).catch(error => {
            // Remove spinning animation on error
            if (refreshIcon) {
                refreshIcon.classList.remove('fa-spin');
                refreshBtn.disabled = false;
            }
            this.showError('Failed to refresh messages');
            console.error('Refresh error:', error);
        });
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
    } else if (tabName === 'errors' && messageManager.selectedMessageId) {
        messageManager.loadErrors(messageManager.selectedMessageId);
    }
}

// Initialize when page loads
let messageManager;
document.addEventListener('DOMContentLoaded', () => {
    messageManager = new MessageManager();
});