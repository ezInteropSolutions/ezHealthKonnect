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

        // Lazy JSON tree viewer state (see _renderJsonTree). Nodes are keyed by an
        // incrementing id; each node's children are only materialized into the DOM
        // the first time the user expands it, so a multi-MB parsed CCD/FHIR document
        // never gets fully stringified/laid-out up front (was causing the UI to freeze).
        this._jsonTreeSeq = 0;
        this._jsonTreeNodes = {};
        this._jsonTreeRaw = {};

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
            AppDialogs.toast('Error initializing messages page: ' + error.message, 'error');
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
        AppDialogs.toast('Please select an interface to view messages. Each interface has its own message viewer.', 'info');
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
                const fmt = interfaceItem.sourceFormat || interfaceItem.messageType || interfaceItem.sourceType || '';
                sendOption.textContent = fmt ? `${interfaceItem.name} (${fmt})` : interfaceItem.name;
                sendSelect.appendChild(sendOption);
            });
        }

        // Current interface selector (for interface-specific mode)
        if (currentInterfaceSelect && this.isInterfaceSpecific) {
            currentInterfaceSelect.innerHTML = '<option value="">Select Interface</option>';
            this.interfaces.forEach(interfaceItem => {
                const option = document.createElement('option');
                option.value = interfaceItem.id;
                const ifmt = interfaceItem.sourceFormat || interfaceItem.messageType || interfaceItem.sourceType || '';
                option.textContent = ifmt ? `${interfaceItem.name} (${ifmt})` : interfaceItem.name;
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

        tbody.innerHTML = messages.map(message => {
            const ds = (message.delivery_status || '').toLowerCase();
            const st = (message.status || '').toLowerCase();
            const hasDLQ = ds === 'failed'
                || st === 'failed' || st === 'error';
            const dlqBadge = hasDLQ
                ? `<span class="dlq-badge" title="Pipeline or delivery failure — click for details"
                      onclick="event.stopPropagation(); showMessageDetail('${message.message_id}')"
                      style="cursor:pointer;display:inline-flex;align-items:center;gap:3px;
                             background:#fef3c7;border:1px solid #fcd34d;border-radius:10px;
                             padding:1px 7px;font-size:11px;font-weight:600;color:#92400e;margin-left:6px;">
                      <i class="fas fa-exclamation-triangle" style="font-size:9px;"></i> Failed
                   </span>`
                : '';
            return `
            <tr class="message-row" onclick="showMessageDetail('${message.message_id}')">
                <td>
                    <div class="fw-bold">${message.message_id}</div>
                    ${message.correlation_id ? `<small class="text-muted">Corr: ${message.correlation_id}</small>` : ''}
                </td>
                <td>${message.interface_name}</td>
                <td>${message.message_type || 'Unknown'}</td>
                <td>${this.renderStatusBadge(message.status, message.delivery_status)}${dlqBadge}</td>
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
                        <button class="btn btn-outline-primary btn-sm" onclick="event.stopPropagation(); showMessageDetail('${message.message_id}')">
                            <i class="fas fa-eye"></i>
                        </button>
                        ${hasDLQ ? `
                            <button class="btn btn-sm" title="Pipeline or delivery failure — click for details"
                                style="background:#fef3c7;border:1px solid #fcd34d;color:#92400e;"
                                onclick="event.stopPropagation(); showMessageDetail('${message.message_id}')">
                                <i class="fas fa-exclamation-triangle"></i>
                            </button>
                        ` : ''}
                        <button class="btn btn-outline-danger btn-sm" onclick="event.stopPropagation(); confirmDeleteMessage('${message.id}')">
                            <i class="fas fa-trash"></i>
                        </button>
                    </div>
                </td>
            </tr>`;
        }).join('');
    }

    // Resolve the single display status from the two DB columns (status + delivery_status).
    // This is the canonical mapping used by both the badge and the filter.
    resolveDisplayStatus(status, deliveryStatus) {
        const s  = (status         || '').toLowerCase();
        const ds = (deliveryStatus || '').toLowerCase();

        if (s === 'received')   return 'received';
        if (s === 'processing') return 'processing';
        if (s === 'reprocessing') return 'processing';
        if (ds === 'failed')    return 'failed';
        if (s === 'failed' || s === 'error') return 'failed';
        if (ds === 'delivered' || s === 'delivered') return 'delivered';
        if (ds === 'not_required') return 'completed';
        if (ds === 'pending' && (s === 'processed' || s === 'delivered')) return 'pending_delivery';
        if (s === 'processed')  return 'completed'; // fallback: processed with unknown delivery
        return s || 'unknown';
    }

    renderStatusBadge(statusOrMessage, deliveryStatus) {
        // Accept either a plain status string (legacy) or resolve from two columns.
        let display;
        if (deliveryStatus !== undefined) {
            display = this.resolveDisplayStatus(statusOrMessage, deliveryStatus);
        } else if (statusOrMessage && typeof statusOrMessage === 'string') {
            display = statusOrMessage;
        } else {
            display = 'unknown';
        }

        const statusConfig = {
            'received':         { cls: 'status-received',         text: 'Received' },
            'processing':       { cls: 'status-processing',       text: 'Processing' },
            'delivered':        { cls: 'status-delivered',        text: 'Delivered' },
            'completed':        { cls: 'status-completed',        text: 'Completed' },
            'pending_delivery': { cls: 'status-pending-delivery', text: 'Pending Delivery' },
            'failed':           { cls: 'status-failed',           text: 'Failed' },
        };

        const config = statusConfig[display] || { cls: 'status-unknown', text: display };
        return `<span class="badge status-badge ${config.cls}">${config.text}</span>`;
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

        // Reset to Overview tab
        document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
        const overviewTab = document.getElementById('overviewTab');
        if (overviewTab) overviewTab.classList.add('active');
        const overviewBtn = document.getElementById('overviewTabBtn');
        if (overviewBtn) overviewBtn.classList.add('active');

        // Clear content tab cache
        const contentView = document.getElementById('messageContentView');
        if (contentView) contentView.innerHTML = '<div class="loading">Loading message content...</div>';
        const journeyView = document.getElementById('messageLineageView');
        if (journeyView) journeyView.innerHTML = '<div class="loading">Loading journey data...</div>';

        showMessageDetailModal();

        try {
            const ifaceParam = this.currentInterfaceId ? `?interfaceId=${this.currentInterfaceId}` : '';
            const response = await fetch(`/api/messages/${messageId}${ifaceParam}`);
            if (response.ok) {
                const data = await response.json();
                this.renderMessageDetail(data.data);
                this.messageData = data.data; // Store for lineage

                // Update modal subtitle with message context
                const message = data.data.message;
                const subtitle = document.getElementById('modalMessageSubtitle');
                if (subtitle) {
                    subtitle.textContent = `${message.message_id}${message.message_type ? ' · ' + message.message_type : ''}`;
                }

                // Show reprocess button for all messages (not just failures)
                const reprocessBtn = document.getElementById('reprocessBtn');
                if (reprocessBtn) {
                    reprocessBtn.style.display = 'inline-block';
                    const isFailed = ['failed', 'error'].includes(message.status);
                    reprocessBtn.textContent = isFailed ? 'Reprocess' : 'Re-run';
                    reprocessBtn.title = isFailed
                        ? 'Retry this failed message through the pipeline'
                        : 'Run this message through the pipeline again';
                }
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
            const container = document.getElementById('messageLineageView');

            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading data lineage...</div>';

            // Get lineage data from backend. Dedupe suppression lineage is a
            // separate, independent concept (cross-message dedup, not message
            // flow) and fetched in parallel — its own failure/emptiness (the
            // common case: most messages have no cda.dedupe step or nothing
            // was suppressed) must never block the main lineage view.
            const [lineageResponse, dedupeResponse] = await Promise.all([
                fetch(`/api/messages/${messageId}/lineage`),
                fetch(`/api/messages/${messageId}/dedupe-suppressions`).catch(() => null),
            ]);

            if (!lineageResponse.ok) {
                throw new Error('Failed to fetch lineage data');
            }

            const lineageData = await lineageResponse.json();

            if (!lineageData.success) {
                throw new Error(lineageData.error || 'Failed to load lineage');
            }

            let dedupeSuppressions = [];
            if (dedupeResponse && dedupeResponse.ok) {
                const dedupeData = await dedupeResponse.json().catch(() => null);
                if (dedupeData && dedupeData.success) dedupeSuppressions = dedupeData.data || [];
            }

            this.renderDataLineage(lineageData.data, dedupeSuppressions);
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

    renderDataLineage(lineage, dedupeSuppressions = []) {
        const container = document.getElementById('messageLineageView');

        if (!lineage.input) {
            container.innerHTML = '<div class="alert alert-info">No lineage data available</div>';
            return;
        }

        this.currentLineageData = lineage;
        const { input, transformation, output, target, flowStatus } = lineage;

        // Build steps array for accordion
        const steps = [];

        steps.push({
            id: 'lj-received',
            color: '#2563eb',
            icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12',
            label: 'Received',
            status: input.status || 'completed',
            statusOk: true,
            time: this.formatDateTime(input.receivedAt),
            detail: `
                <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                    <tr><th style="width:38%;color:#64748b;font-weight:500;">Interface</th><td style="color:#1e293b;"><strong>${input.interfaceName}</strong></td></tr>
                    <tr><th style="color:#64748b;font-weight:500;">Message type</th><td><span style="background:#dbeafe;color:#1e40af;padding:0.1rem 0.5rem;border-radius:3px;font-size:0.78rem;">${input.messageType || '—'}</span></td></tr>
                    <tr><th style="color:#64748b;font-weight:500;">Source</th><td style="color:#1e293b;">${input.sourceType}</td></tr>
                    <tr><th style="color:#64748b;font-weight:500;">Size</th><td style="color:#1e293b;">${this.formatBytes(input.messageSize)}</td></tr>
                </table>`
        });

        if (transformation) {
            const transformLabel = this._buildTransformLabel(
                transformation.sourceFormat || input.interfaceSourceType,
                transformation.targetFormat || input.interfaceTargetType
            );
            const steps_html = this._buildTransformStepsHtml(transformation.steps || []);
            steps.push({
                id: 'lj-transform',
                color: '#6366f1',
                icon: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
                label: 'Transformation',
                status: transformLabel,
                statusOk: true,
                time: transformation.parsedAt ? this.formatDateTime(transformation.parsedAt) : '—',
                detail: `
                    <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                        <tr><th style="width:38%;color:#64748b;font-weight:500;">Format</th><td style="color:#1e293b;">${transformLabel}</td></tr>
                        <tr><th style="color:#64748b;font-weight:500;">Parse time</th><td style="color:#1e293b;"><strong>${transformation.parsingTimeMs || 0}ms</strong></td></tr>
                        <tr><th style="color:#64748b;font-weight:500;">Result</th><td><span class="badge bg-success">Success</span></td></tr>
                    </table>
                    ${steps_html}`
            });
        }

        // Cross-message dedup suppression lineage — only shown when this
        // message's cda.dedupe step actually suppressed something (the
        // common case has nothing here, since crossMessage suppression
        // requires a prior message to have already delivered the same fact).
        if (Array.isArray(dedupeSuppressions) && dedupeSuppressions.length > 0) {
            const rows = dedupeSuppressions.map(s => `
                <tr>
                    <td style="color:#1e293b;">${this.escapeHtml(s.sectionKey)}</td>
                    <td style="font-family:monospace;font-size:0.76rem;color:#475569;word-break:break-all;">${this.escapeHtml(s.identityKey)}</td>
                    <td style="color:#1e293b;">${s.firstSeenAt ? this.formatDateTime(s.firstSeenAt) : '—'}</td>
                    <td>${s.firstSeenMessageId
                        ? `<a href="#" onclick="messageManager.showMessageDetail('${this.escapeHtml(s.firstSeenMessageId)}');return false;" style="color:#2563eb;font-family:monospace;font-size:0.76rem;">${this.escapeHtml(s.firstSeenMessageId)}</a>`
                        : '—'}</td>
                </tr>`).join('');
            steps.push({
                id: 'lj-dedupe',
                color: '#f472b6',
                icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
                label: 'Dedup Suppressions',
                status: `${dedupeSuppressions.length} suppressed`,
                statusOk: true,
                time: '',
                detail: `
                    <div style="font-size:0.78rem;color:#64748b;margin-bottom:0.6rem;">
                        These facts were dropped from this message because an earlier message already delivered them (cda.dedupe crossMessage). Click a message ID to jump to where it was first seen.
                    </div>
                    <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                        <thead>
                            <tr style="color:#94a3b8;font-size:0.7rem;text-transform:uppercase;">
                                <th style="font-weight:600;">Section</th>
                                <th style="font-weight:600;">Identity</th>
                                <th style="font-weight:600;">First Seen</th>
                                <th style="font-weight:600;">First Message</th>
                            </tr>
                        </thead>
                        <tbody>${rows}</tbody>
                    </table>`
            });
        }

        if (output) {
            const delivSt = output.deliveryStatus || 'pending';
            const isOk = delivSt === 'delivered';
            const isNotRequired = delivSt === 'not_required' || delivSt === 'not_configured';
            const reqMethod = output.requestMethod || 'POST';
            const reqEndpoint = output.deliveryEndpoint || '—';
            const respStatus = output.deliveryStatusCode;
            const respBody = output.deliveryResponse;
            const reqBody = output.requestBody; // populated by executor after fix

            let httpDetail = `
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem;">
                    <div>
                        <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;color:#94a3b8;margin-bottom:0.4rem;">Request</div>
                        <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                            <tr><th style="width:45%;color:#64748b;font-weight:500;">Method</th><td style="color:#1e293b;">${reqMethod}</td></tr>
                            <tr><th style="color:#64748b;font-weight:500;">Endpoint</th><td style="color:#1e293b;word-break:break-all;font-size:0.78rem;">${reqEndpoint}</td></tr>
                            <tr><th style="color:#64748b;font-weight:500;">Size</th><td style="color:#1e293b;">${output.payloadSizeBytes ? this.formatBytes(output.payloadSizeBytes) : '—'}</td></tr>
                        </table>
                        ${reqBody ? `<details style="margin-top:0.5rem;"><summary style="font-size:0.78rem;color:#64748b;cursor:pointer;">Request body</summary><pre style="margin-top:0.35rem;background:#0f172a;color:#e2e8f0;padding:0.5rem;border-radius:4px;font-size:0.72rem;overflow:auto;max-height:120px;white-space:pre-wrap;">${this.escapeHtml(typeof reqBody==='string'?reqBody:JSON.stringify(reqBody,null,2))}</pre></details>` : ''}
                    </div>
                    <div>
                        <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;color:#94a3b8;margin-bottom:0.4rem;">Response</div>
                        <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                            <tr><th style="width:45%;color:#64748b;font-weight:500;">Status</th><td><span style="background:${isOk?'#dcfce7':'#fee2e2'};color:${isOk?'#166534':'#991b1b'};padding:0.1rem 0.5rem;border-radius:3px;font-size:0.78rem;font-weight:600;">${respStatus || '—'}</span></td></tr>
                            <tr><th style="color:#64748b;font-weight:500;">Latency</th><td style="color:#1e293b;"><strong>${output.deliveryTimeMs || 0}ms</strong></td></tr>
                            <tr><th style="color:#64748b;font-weight:500;">Retries</th><td style="color:#1e293b;">${output.retryCount || 0}</td></tr>
                        </table>
                        ${respBody ? `<details style="margin-top:0.5rem;"><summary style="font-size:0.78rem;color:#64748b;cursor:pointer;">Response body</summary><pre style="margin-top:0.35rem;background:#0f172a;color:#e2e8f0;padding:0.5rem;border-radius:4px;font-size:0.72rem;overflow:auto;max-height:120px;white-space:pre-wrap;">${this.escapeHtml(typeof respBody==='string'?respBody:JSON.stringify(respBody,null,2))}</pre></details>` : ''}
                    </div>
                </div>
                ${output.transformedMessage && typeof output.transformedMessage === 'object' && output.transformedMessage.mongo_reference ?
                    `<div style="margin-top:0.75rem;background:#eff6ff;border-radius:5px;padding:0.5rem 0.75rem;font-family:monospace;font-size:0.72rem;color:#1e40af;word-break:break-all;">📦 ${output.transformedMessage.mongo_reference}</div>` : ''}`;

            steps.push({
                id: 'lj-delivery',
                color: isOk ? '#0ea5e9' : isNotRequired ? '#94a3b8' : '#ef4444',
                icon: 'M12 19l9 2-9-18-9 18 9-2zm0 0v-8',
                label: 'Sent to Destination',
                status: isNotRequired ? 'not configured' : delivSt,
                statusOk: isOk,
                statusNeutral: isNotRequired,
                time: output.deliveryCompletedAt
                    ? this.formatDateTime(output.deliveryCompletedAt)
                    : isNotRequired ? 'No outbound connector' : 'In progress',
                detail: isNotRequired
                    ? `<div style="padding:0.75rem;background:#f8fafc;border-radius:6px;color:#64748b;font-size:0.83rem;">
                         No outbound connector is configured for this interface. The pipeline ran successfully —
                         add a <strong>Connector (Outbound)</strong> step to the pipeline to deliver messages to a destination.
                       </div>`
                    : httpDetail
            });
        }

        if (target) {
            steps.push({
                id: 'lj-target',
                color: '#16a34a',
                icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
                label: `Response from ${target.interfaceName}`,
                status: 'acknowledged',
                statusOk: true,
                time: this.formatDateTime(target.receivedAt),
                detail: `
                    <table class="table table-sm table-borderless mb-0" style="font-size:0.83rem;">
                        <tr><th style="width:38%;color:#64748b;font-weight:500;">Target</th><td style="color:#1e293b;"><strong>${target.interfaceName}</strong></td></tr>
                        <tr><th style="color:#64748b;font-weight:500;">Status</th><td>${this.renderStatusBadge(target.status, target.delivery_status)}</td></tr>
                        <tr><th style="color:#64748b;font-weight:500;">Size</th><td style="color:#1e293b;">${this.formatBytes(target.messageSize)}</td></tr>
                    </table>`
            });
        }

        // Render accordion
        const stepsHtml = steps.map((step, i) => `
            <div style="border:1px solid #e2e8f0;border-radius:7px;margin-bottom:0.5rem;overflow:hidden;">
                <button onclick="messageManager._toggleJourneyStep('${step.id}')"
                        style="width:100%;display:flex;align-items:center;gap:0.75rem;padding:0.75rem 1rem;background:white;border:none;cursor:pointer;text-align:left;">
                    <!-- Step number circle -->
                    <span style="width:24px;height:24px;border-radius:50%;background:${step.color};color:white;font-size:0.7rem;font-weight:700;display:flex;align-items:center;justify-content:center;flex-shrink:0;">${i+1}</span>
                    <!-- Label -->
                    <span style="flex:1;font-weight:600;color:#1e293b;font-size:0.875rem;">${step.label}</span>
                    <!-- Status badge -->
                    <span style="background:${step.statusOk ? '#dcfce7' : step.statusNeutral ? '#f1f5f9' : '#fee2e2'};color:${step.statusOk ? '#166534' : step.statusNeutral ? '#64748b' : '#991b1b'};padding:0.15rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">${step.status}</span>
                    <!-- Time -->
                    <span style="color:#94a3b8;font-size:0.75rem;min-width:140px;text-align:right;">${step.time}</span>
                    <!-- Chevron -->
                    <svg id="${step.id}-chevron" style="width:16px;height:16px;color:#94a3b8;flex-shrink:0;transition:transform 0.15s;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
                </button>
                <div id="${step.id}-body" style="display:none;padding:1rem;background:#f8fafc;border-top:1px solid #e2e8f0;">
                    ${step.detail}
                </div>
            </div>`).join('');

        const statusDot = (active, label) =>
            `<span style="display:inline-flex;align-items:center;gap:0.35rem;font-size:0.78rem;color:${active?'#1e40af':'#94a3b8'};">
                <span style="width:6px;height:6px;border-radius:50%;background:${active?'#2563eb':'#cbd5e1'};display:inline-block;"></span>${label}</span>`;

        container.innerHTML = `
            <div>
                <div style="display:flex;gap:1rem;margin-bottom:0.85rem;align-items:center;flex-wrap:wrap;">
                    ${statusDot(flowStatus.inputReceived, 'Received')}
                    ${statusDot(flowStatus.transformed, 'Transformed')}
                    ${statusDot(flowStatus.delivered, 'Delivered')}
                    ${lineage.correlationId ? `<code style="margin-left:auto;font-size:0.72rem;color:#64748b;background:#f1f5f9;padding:0.15rem 0.5rem;border-radius:3px;">${lineage.correlationId}</code>` : ''}
                </div>
                ${stepsHtml}
            </div>`;
    }

    _toggleJourneyStep(id) {
        const body = document.getElementById(id + '-body');
        const chevron = document.getElementById(id + '-chevron');
        if (!body) return;
        const open = body.style.display === 'block';
        body.style.display = open ? 'none' : 'block';
        if (chevron) chevron.style.transform = open ? '' : 'rotate(180deg)';
    }

    _formatTypeName(type) {
        if (!type) return null;
        const t = type.toLowerCase();
        if (['hl7v2', 'hl7', 'tcp', 'mllp'].includes(t)) return 'HL7 v2';
        if (['fhir', 'fhir_rest', 'http_fhir'].includes(t)) return 'FHIR R4';
        if (['json', 'http_json'].includes(t)) return 'JSON';
        if (['xml', 'http_xml'].includes(t)) return 'XML';
        if (['csv'].includes(t)) return 'CSV';
        if (['database', 'postgresql'].includes(t)) return 'Database';
        return type;
    }

    _buildTransformLabel(sourceFormat, targetFormat) {
        const src = this._formatTypeName(sourceFormat);
        const tgt = this._formatTypeName(targetFormat);
        if (src && tgt) return `${src} → ${tgt}`;
        if (src) return `${src} → ?`;
        if (tgt) return `? → ${tgt}`;
        return 'Transformation';
    }

    _buildTransformStepsHtml(steps) {
        if (!steps || steps.length === 0) return '';
        const rows = steps.map(s => {
            const ok = s.success !== false && !s.error;
            const badge = ok
                ? `<span style="background:#dcfce7;color:#166534;padding:0.1rem 0.45rem;border-radius:3px;font-size:0.72rem;font-weight:600;">ok</span>`
                : `<span style="background:#fee2e2;color:#991b1b;padding:0.1rem 0.45rem;border-radius:3px;font-size:0.72rem;font-weight:600;">failed</span>`;
            const dur = s.duration_ms != null ? `${s.duration_ms}ms` : '—';
            const errNote = s.error ? `<div style="color:#dc2626;font-size:0.75rem;margin-top:0.2rem;">${this.escapeHtml(s.error)}</div>` : '';
            return `<tr>
                <td style="padding:0.25rem 0.5rem;color:#374151;font-size:0.8rem;">${this.escapeHtml(s.step_name || s.step_type || '—')}</td>
                <td style="padding:0.25rem 0.5rem;color:#6b7280;font-size:0.78rem;font-family:monospace;">${this.escapeHtml(s.step_type || '—')}</td>
                <td style="padding:0.25rem 0.5rem;color:#6b7280;font-size:0.78rem;">${dur}</td>
                <td style="padding:0.25rem 0.5rem;">${badge}${errNote}</td>
            </tr>`;
        }).join('');
        return `
            <div style="margin-top:0.75rem;">
                <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;color:#94a3b8;margin-bottom:0.4rem;">Pipeline Steps</div>
                <table style="width:100%;border-collapse:collapse;">
                    <thead><tr style="border-bottom:1px solid #e5e7eb;">
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Step</th>
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Type</th>
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Duration</th>
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Status</th>
                    </tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            </div>`;
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

    /**
     * Load logs for a message from MongoDB (V33 - Interface-Level Logging)
     */
    async loadLogs(messageId) {
        try {
            const container = document.getElementById('messageLogsView');
            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading logs...</div>';

            // Resolve interfaceId — from URL param, stored message data, or omit (Go auto-resolves)
            const ifaceId = this.currentInterfaceId || this.messageData?.message?.interface_id || '';
            const logsResponse = await fetch(`/api/messages/${messageId}/logs${ifaceId ? '?interfaceId=' + encodeURIComponent(ifaceId) : ''}`);

            // Object storage unavailable
            if (logsResponse.status === 503) {
                container.innerHTML = `
                    <div style="background:#f8fafc;border-radius:8px;padding:1.5rem;text-align:center;border:1px dashed #cbd5e1;">
                        <i class="fas fa-cloud" style="font-size:1.75rem;color:#94a3b8;display:block;margin-bottom:0.5rem;"></i>
                        <div style="font-weight:600;color:#475569;margin-bottom:0.35rem;">Object storage not configured</div>
                        <div style="color:#64748b;font-size:0.85rem;">Processing logs are stored in object storage (MinIO/S3).<br>Configure OBJECT_STORAGE_* environment variables to enable logs.</div>
                    </div>`;
                return;
            }

            if (!logsResponse.ok) {
                throw new Error('Failed to fetch logs');
            }

            const logsData = await logsResponse.json();

            if (!logsData.success) {
                throw new Error(logsData.error || 'Failed to load logs');
            }

            // Go response: { success, logs: [{ts, level, stage, message, fields}] }
            // Normalise to { timestamp, level, category, message, details }
            const rawLogs = logsData.logs || logsData.data?.logs || [];
            const logs = rawLogs.map(e => ({
                timestamp: e.ts || e.timestamp,
                level:     (e.level || 'info').toLowerCase(),
                category:  e.stage || e.category || '',
                message:   e.message,
                details:   e.fields || e.details || {}
            }));
            const summary = {
                total:    logs.length,
                errors:   logs.filter(l => l.level === 'error').length,
                warnings: logs.filter(l => l.level === 'warning').length,
                info:     logs.filter(l => l.level === 'info').length,
                debug:    logs.filter(l => l.level === 'debug').length
            };

            if (logs.length === 0) {
                const logUri = this.messageData?.message?.log_uri;
                container.innerHTML = `
                    <div style="background: #f8fafc; padding: 2rem; border-radius: 8px; text-align: center; border: 2px dashed #e2e8f0;">
                        <i class="fas fa-clipboard-list" style="font-size:1.75rem;color:#94a3b8;display:block;margin-bottom:0.5rem;"></i>
                        <div style="font-weight: 600; color: #64748b; margin-bottom: 0.35rem;">No logs for this message</div>
                        <div style="color: #94a3b8; font-size: 0.82rem;">Processing logs are written to object storage during pipeline execution.<br>Logs appear here when the interface has debug logging enabled.</div>
                        ${logUri ? `<div style="margin-top:0.75rem;font-family:monospace;font-size:0.72rem;color:#1e40af;background:#eff6ff;padding:0.3rem 0.6rem;border-radius:4px;display:inline-block;">📦 ${logUri}</div>` : ''}
                    </div>
                `;
                return;
            }

            const logsHTML = logs.map(log => {
                const levelColors = {
                    'error': { bg: '#fef2f2', border: '#f87171', text: '#991b1b', icon: 'exclamation-circle' },
                    'warning': { bg: '#fef3c7', border: '#fbbf24', text: '#92400e', icon: 'exclamation-triangle' },
                    'info': { bg: '#eff6ff', border: '#60a5fa', text: '#1e40af', icon: 'info-circle' },
                    'debug': { bg: '#f5f3ff', border: '#a78bfa', text: '#5b21b6', icon: 'bug' }
                };
                const style = levelColors[log.level] || levelColors.info;

                let detailsHTML = '';
                if (log.details && Object.keys(log.details).length > 0) {
                    detailsHTML = log.category === 'delivery'
                        ? this._renderDeliveryDetails(log.details, style)
                        : this._renderGenericDetails(log.details);
                }

                // Add stack trace display
                let stackTraceHTML = '';
                if (log.stack_trace) {
                    stackTraceHTML = `
                        <details style="margin-top: 0.75rem; background: rgba(0,0,0,0.05); padding: 0.5rem; border-radius: 4px;">
                            <summary style="cursor: pointer; font-weight: 600; color: ${style.text}; user-select: none;">
                                <i class="fas fa-layer-group"></i> Stack Trace
                            </summary>
                            <pre style="margin: 0.5rem 0 0 0; font-size: 0.75rem; font-family: 'Courier New', monospace; overflow-x: auto; white-space: pre-wrap; word-wrap: break-word;">${log.stack_trace}</pre>
                        </details>
                    `;
                }

                // Add error code if present
                const errorCodeBadge = log.error_code ? `<span style="background: ${style.border}; color: white; padding: 0.125rem 0.5rem; border-radius: 3px; font-size: 0.7rem; margin-left: 0.5rem;">${log.error_code}</span>` : '';

                return `
                    <div style="background: ${style.bg}; border-left: 3px solid ${style.border}; padding: 0.75rem 1rem; margin-bottom: 0.5rem; border-radius: 4px;">
                        <div style="display: flex; justify-content: space-between;">
                            <div style="flex: 1;">
                                <div style="display: flex; align-items: center;">
                                    <i class="fas fa-${style.icon}" style="color: ${style.text};"></i> 
                                    <strong style="margin-left: 0.5rem;">${log.level.toUpperCase()}</strong> 
                                    <span style="color: #64748b; margin: 0 0.5rem;">•</span>
                                    <span style="color: #64748b;">${log.category}</span>
                                    ${errorCodeBadge}
                                </div>
                                <div style="margin-top: 0.5rem; color: #1e293b;">${log.message}</div>
                                ${detailsHTML}
                                ${stackTraceHTML}
                            </div>
                            <div style="color: #94a3b8; font-size: 0.75rem; white-space: nowrap; margin-left: 1rem;">${new Date(log.timestamp).toLocaleTimeString()}</div>
                        </div>
                    </div>
                `;
            }).join('');

            container.innerHTML = `
                <div>
                    <div style="margin-bottom: 1rem;">
                        <strong>Total:</strong> ${summary.total} |
                        <strong style="color: #dc2626;">Errors:</strong> ${summary.errors} |
                        <strong style="color: #d97706;">Warnings:</strong> ${summary.warnings} |
                        <strong style="color: #2563eb;">Info:</strong> ${summary.info} |
                        <strong style="color: #7c3aed;">Debug:</strong> ${summary.debug}
                    </div>
                    ${logsHTML}
                </div>
            `;

        } catch (error) {
            console.error('Failed to load logs:', error);
            document.getElementById('messageLogsView').innerHTML = '<div class="alert alert-warning">Unable to load log data.</div>';
        }

    }

    /**
     * Load the CDA→FHIR mapping log for a message (section timings, resource
     * counts, dedup/synthesis events). Only populated for messages processed
     * through a cda.to_fhir pipeline step; gracefully shows an empty state
     * for everything else (mirrors loadLogs()'s 503/empty handling).
     */
    async loadMappingLog(messageId) {
        try {
            const container = document.getElementById('messageMappingLogView');
            container.innerHTML = '<div class="text-center"><i class="fas fa-spinner fa-spin"></i> Loading mapping log...</div>';

            const ifaceId = this.currentInterfaceId || this.messageData?.message?.interface_id || '';
            const response = await fetch(`/api/messages/${messageId}/mapping-log${ifaceId ? '?interfaceId=' + encodeURIComponent(ifaceId) : ''}`);

            if (response.status === 503) {
                container.innerHTML = `
                    <div style="background:#f8fafc;border-radius:8px;padding:1.5rem;text-align:center;border:1px dashed #cbd5e1;">
                        <i class="fas fa-cloud" style="font-size:1.75rem;color:#94a3b8;display:block;margin-bottom:0.5rem;"></i>
                        <div style="font-weight:600;color:#475569;margin-bottom:0.35rem;">Object storage not configured</div>
                        <div style="color:#64748b;font-size:0.85rem;">Mapping logs are stored in object storage (MinIO/S3).<br>Configure OBJECT_STORAGE_* environment variables to enable this view.</div>
                    </div>`;
                return;
            }

            if (response.status === 404) {
                const data = await response.json().catch(() => ({}));
                container.innerHTML = `
                    <div style="background: #f8fafc; padding: 2rem; border-radius: 8px; text-align: center; border: 2px dashed #e2e8f0;">
                        <i class="fas fa-sitemap" style="font-size:1.75rem;color:#94a3b8;display:block;margin-bottom:0.5rem;"></i>
                        <div style="font-weight: 600; color: #64748b; margin-bottom: 0.35rem;">No mapping log for this message</div>
                        <div style="color: #94a3b8; font-size: 0.82rem;">${this.escapeHtml(data.error || 'This message was not processed through a CDA→FHIR pipeline step, or the message is older than today.')}</div>
                    </div>`;
                return;
            }

            if (!response.ok) {
                throw new Error('Failed to fetch mapping log');
            }

            const data = await response.json();
            if (!data.success) {
                throw new Error(data.error || 'Failed to load mapping log');
            }

            container.innerHTML = this._renderMappingLog(data.mappingLog || {});
        } catch (error) {
            console.error('Failed to load mapping log:', error);
            document.getElementById('messageMappingLogView').innerHTML = '<div class="alert alert-warning">Unable to load mapping log.</div>';
        }
    }

    /**
     * Renders the mapping log summary cards, per-section breakdown table, and
     * assembly events (dedup/synthesis) list.
     */
    _renderMappingLog(log) {
        const summary = log.summary || {};
        const sections = log.sections || [];
        const assembly = log.assembly || [];

        const statCard = (label, value, color) => `
            <div style="background:#f8fafc;border-radius:6px;padding:0.6rem 0.9rem;text-align:center;min-width:90px;">
                <div style="font-size:1.05rem;font-weight:700;color:${color || '#1e3a8a'};">${value}</div>
                <div style="font-size:0.68rem;color:#94a3b8;text-transform:uppercase;font-weight:600;margin-top:0.15rem;">${label}</div>
            </div>`;

        const summaryHTML = `
            <div style="display:flex;gap:0.6rem;flex-wrap:wrap;margin-bottom:1rem;">
                ${statCard('Resources', summary.totalResources ?? 0)}
                ${statCard('Total Time', `${summary.totalTimeMs ?? 0}ms`)}
                ${statCard('Mapping', `${summary.mappingTimeMs ?? 0}ms`)}
                ${statCard('Assembly', `${summary.assemblyTimeMs ?? 0}ms`)}
                ${statCard('Deduplicated', summary.deduplicatedCount ?? 0, summary.deduplicatedCount ? '#d97706' : undefined)}
                ${statCard('Synthesized', summary.synthesizedCount ?? 0, summary.synthesizedCount ? '#7c3aed' : undefined)}
            </div>`;

        // Toggle JS shared by every section row — expands/collapses the entry-level
        // drill-down row directly beneath it without needing a named handler.
        const toggleJs = "var d=this.nextElementSibling; var open=d.style.display==='table-row'; d.style.display=open?'none':'table-row'; var c=this.querySelector('.mlog-chevron'); if(c) c.style.transform=open?'rotate(0deg)':'rotate(90deg)';";

        const entryRow = (e) => {
            const refsHTML = (e.resourceRefs && e.resourceRefs.length > 0)
                ? e.resourceRefs.map(ref => `<span style="background:#eff6ff;color:#1e40af;padding:0.1rem 0.4rem;border-radius:3px;font-size:0.72rem;font-family:monospace;margin-right:0.3rem;display:inline-block;margin-bottom:0.2rem;">${this.escapeHtml(ref)}</span>`).join('')
                : `<span style="color:#94a3b8;font-size:0.75rem;">not mapped</span>`;
            const codeText = e.code ? `${this.escapeHtml(e.code)}${e.codeSystem ? ' (' + this.escapeHtml(e.codeSystem) + ')' : ''}` : '—';
            return `<tr style="border-bottom:1px solid #f1f5f9;">
                <td style="padding:0.3rem 0.5rem;color:#94a3b8;font-size:0.75rem;">${e.entryIndex}</td>
                <td style="padding:0.3rem 0.5rem;color:#6b7280;font-size:0.75rem;">${this.escapeHtml(e.entryType || '—')}</td>
                <td style="padding:0.3rem 0.5rem;color:#6b7280;font-size:0.75rem;font-family:monospace;">${codeText}</td>
                <td style="padding:0.3rem 0.5rem;color:#374151;font-size:0.78rem;">${this.escapeHtml(e.displayName || '—')}</td>
                <td style="padding:0.3rem 0.5rem;">${refsHTML}</td>
            </tr>`;
        };

        const entriesDetailTable = (entries) => `
            <table style="width:100%;border-collapse:collapse;background:#fafbfc;">
                <thead><tr style="border-bottom:1px solid #e5e7eb;">
                    <th style="padding:0.2rem 0.5rem;text-align:left;font-size:0.68rem;color:#9ca3af;font-weight:500;">#</th>
                    <th style="padding:0.2rem 0.5rem;text-align:left;font-size:0.68rem;color:#9ca3af;font-weight:500;">Entry Type</th>
                    <th style="padding:0.2rem 0.5rem;text-align:left;font-size:0.68rem;color:#9ca3af;font-weight:500;">Code</th>
                    <th style="padding:0.2rem 0.5rem;text-align:left;font-size:0.68rem;color:#9ca3af;font-weight:500;">Description</th>
                    <th style="padding:0.2rem 0.5rem;text-align:left;font-size:0.68rem;color:#9ca3af;font-weight:500;">→ FHIR Resource(s)</th>
                </tr></thead>
                <tbody>${entries.map(entryRow).join('')}</tbody>
            </table>`;

        const sectionRows = sections.map(s => {
            const hasErrors = s.errors && s.errors.length > 0;
            const errorBadge = hasErrors
                ? `<span style="background:#fee2e2;color:#991b1b;padding:0.1rem 0.45rem;border-radius:3px;font-size:0.72rem;font-weight:600;">${s.errors.length} error${s.errors.length > 1 ? 's' : ''}</span>`
                : `<span style="color:#94a3b8;font-size:0.78rem;">—</span>`;
            // Warnings are informational, non-blocking notices (e.g. a CDA
            // entry with more than one <value> -- only the first was
            // mapped) -- distinct from errors, never implies the section
            // failed. See SectionLog.Warnings' doc comment (mapping_log/
            // section_log.go) for what populates this.
            const hasWarnings = s.warnings && s.warnings.length > 0;
            const warningBadge = hasWarnings
                ? `<span title="${this.escapeHtml(s.warnings.join('\n'))}" style="background:#fef3c7;color:#92400e;padding:0.1rem 0.45rem;border-radius:3px;font-size:0.72rem;font-weight:600;cursor:help;">${s.warnings.length} warning${s.warnings.length > 1 ? 's' : ''}</span>`
                : `<span style="color:#94a3b8;font-size:0.78rem;">—</span>`;
            const hasEntries = s.entries && s.entries.length > 0;
            const chevron = hasEntries
                ? `<i class="fas fa-chevron-right mlog-chevron" style="font-size:0.65rem;color:#94a3b8;margin-right:0.4rem;display:inline-block;transition:transform 0.15s;"></i>`
                : `<span style="display:inline-block;width:1rem;"></span>`;
            const mainRow = `<tr ${hasEntries ? `style="cursor:pointer;" onclick="${toggleJs}"` : ''}>
                <td style="padding:0.25rem 0.5rem;color:#374151;font-size:0.8rem;">${chevron}${this.escapeHtml(s.title || s.sectionKey || '—')}</td>
                <td style="padding:0.25rem 0.5rem;color:#6b7280;font-size:0.78rem;text-align:right;">${s.entriesIn ?? 0}</td>
                <td style="padding:0.25rem 0.5rem;color:#6b7280;font-size:0.78rem;text-align:right;">${s.resourcesOut ?? 0}</td>
                <td style="padding:0.25rem 0.5rem;color:#6b7280;font-size:0.78rem;text-align:right;">${s.processingTimeMs ?? 0}ms</td>
                <td style="padding:0.25rem 0.5rem;">${errorBadge}</td>
                <td style="padding:0.25rem 0.5rem;">${warningBadge}</td>
            </tr>`;
            const detailRow = hasEntries
                ? `<tr style="display:none;"><td colspan="6" style="padding:0.4rem 0.5rem 0.6rem 1.5rem;">${entriesDetailTable(s.entries)}</td></tr>`
                : '';
            return mainRow + detailRow;
        }).join('');

        const sectionsHTML = sections.length > 0 ? `
            <div style="margin-top:0.75rem;">
                <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;color:#94a3b8;margin-bottom:0.4rem;">Sections (${sections.length}) — click a row to see entry→resource detail</div>
                <table style="width:100%;border-collapse:collapse;">
                    <thead><tr style="border-bottom:1px solid #e5e7eb;">
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Section</th>
                        <th style="padding:0.25rem 0.5rem;text-align:right;font-size:0.72rem;color:#9ca3af;font-weight:500;">Entries In</th>
                        <th style="padding:0.25rem 0.5rem;text-align:right;font-size:0.72rem;color:#9ca3af;font-weight:500;">Resources Out</th>
                        <th style="padding:0.25rem 0.5rem;text-align:right;font-size:0.72rem;color:#9ca3af;font-weight:500;">Time</th>
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Errors</th>
                        <th style="padding:0.25rem 0.5rem;text-align:left;font-size:0.72rem;color:#9ca3af;font-weight:500;">Warnings</th>
                    </tr></thead>
                    <tbody>${sectionRows}</tbody>
                </table>
            </div>` : '';

        // The "Source" sub-list only appears when the interface has deep debug
        // logging enabled; only show the explanatory help line when at least one
        // event actually has it, so messages without deep lineage don't show a
        // help line that doesn't apply to anything on screen.
        const hasAnyLineage = assembly.some(e => e.lineage && Object.keys(e.lineage).length > 0);
        const assemblyHelpHTML = hasAnyLineage ? `
            <div style="font-size:0.72rem;color:#94a3b8;margin-bottom:0.4rem;margin-top:-0.1rem;">
                "Source" shows which CDA section + entry (and matching identifier) produced each resource.
                <code style="background:#f1f5f9;padding:0 0.2rem;border-radius:2px;">section[entryIndex]</code>
                identifies the entry within that section, not the exact XML field — for entries with several
                nested participants/components, you may still need to open that one entry to find the specific match.
            </div>` : '';

        const assemblyHTML = assembly.length > 0 ? `
            <div style="margin-top:1rem;">
                <div style="font-size:0.7rem;font-weight:700;text-transform:uppercase;color:#94a3b8;margin-bottom:0.4rem;">Assembly Events (${assembly.length})</div>
                ${assemblyHelpHTML}
                ${assembly.map(e => {
                    // Lineage is only present when the interface has deep debug
                    // logging enabled (debug_logging/log_level=debug) — absent
                    // for every other mapping log, so this block renders nothing
                    // extra in that case.
                    const lineageEntries = e.lineage ? Object.entries(e.lineage) : [];
                    const sourceHTML = lineageEntries.length > 0 ? `
                        <div style="margin-top:0.35rem;padding-top:0.35rem;border-top:1px solid #ede9fe;font-size:0.74rem;color:#6d28d9;">
                            <div style="font-weight:600;color:#5b21b6;margin-bottom:0.15rem;">Source</div>
                            ${lineageEntries.map(([resId, l]) => `
                                <div style="color:#475569;">
                                    <span style="font-family:monospace;">${this.escapeHtml(resId)}</span>
                                    ← ${this.escapeHtml(l.sectionKey || 'unknown section')}[${l.entryIndex ?? '?'}]
                                    ${l.cdaIds && l.cdaIds.length > 0 ? `(${this.escapeHtml(l.cdaIds.join(', '))})` : ''}
                                </div>
                            `).join('')}
                        </div>` : '';
                    return `
                    <div style="background:#faf5ff;border-left:3px solid #a78bfa;padding:0.5rem 0.75rem;margin-bottom:0.4rem;border-radius:4px;font-size:0.8rem;">
                        <strong style="color:#5b21b6;">${this.escapeHtml(e.action || '')}</strong>
                        <span style="color:#64748b;"> · ${this.escapeHtml(e.rule || '')} · ${this.escapeHtml(e.resourceType || '')}</span>
                        <div style="color:#475569;margin-top:0.2rem;">${this.escapeHtml(e.detail || '')}</div>
                        ${sourceHTML}
                    </div>
                `;
                }).join('')}
            </div>` : '';

        if (sections.length === 0 && assembly.length === 0) {
            return `
                <div style="background: #f8fafc; padding: 2rem; border-radius: 8px; text-align: center; border: 2px dashed #e2e8f0;">
                    <div style="font-weight: 600; color: #64748b;">Mapping log is empty</div>
                </div>`;
        }

        return `<div>${summaryHTML}${sectionsHTML}${assemblyHTML}</div>`;
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
                            style="background: #2563eb; color: white; border: none; padding: 0.35rem 0.65rem; border-radius: 4px; cursor: pointer; font-size: 0.8rem; display: flex; align-items: center; gap: 0.35rem;"
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
        const { message } = data;
        const container = document.getElementById('messageDetailContent');

        const _ds = message.delivery_status || '';
        const _dsBg  = _ds === 'delivered' ? '#dcfce7' : (_ds === 'not_required' || _ds === 'not_configured') ? '#f1f5f9' : (_ds === 'failed' ? '#fee2e2' : '#fef3c7');
        const _dsFg  = _ds === 'delivered' ? '#166534' : (_ds === 'not_required' || _ds === 'not_configured') ? '#64748b' : (_ds === 'failed' ? '#991b1b' : '#92400e');
        const _dsLabel = _ds === 'not_required' ? 'no outbound' : (_ds || 'pending');
        const deliveryBadge = `<span style="background:${_dsBg};color:${_dsFg};padding:0.35rem 0.75rem;border-radius:6px;font-size:0.8rem;font-weight:500;">${_dsLabel}</span>`;

        // Storage URIs — show as chip links if populated
        const uriChip = (label, uri) => uri
            ? `<span style="display:inline-flex;align-items:center;gap:0.3rem;background:#eff6ff;color:#1e40af;border-radius:4px;padding:0.2rem 0.55rem;font-size:0.75rem;font-family:monospace;margin:0.15rem 0.15rem 0 0;">${label}</span>`
            : '';
        const storageRow = (message.raw_content_uri || message.parsed_content_uri || message.transformed_content_uri)
            ? `<tr><th style="color:#64748b;font-weight:500;padding:0.3rem 0;vertical-align:top;">Storage</th><td style="padding:0.3rem 0;">
                ${uriChip('raw', message.raw_content_uri)}
                ${uriChip('parsed', message.parsed_content_uri)}
                ${uriChip('transformed', message.transformed_content_uri)}
               </td></tr>`
            : '';

        // Parsing row
        const parsingRow = message.parsed_at
            ? `<tr><th style="color:#64748b;font-weight:500;padding:0.3rem 0;">Parsed</th><td style="color:#1e293b;padding:0.3rem 0;">${this.formatDateTime(message.parsed_at)}${message.parsing_time_ms ? ` <span style="color:#94a3b8;font-size:0.78rem;">(${message.parsing_time_ms}ms)</span>` : ''}</td></tr>`
            : '';

        const _ds2 = (message.delivery_status || '').toLowerCase();
        const _st2 = (message.status || '').toLowerCase();
        const hasDLQSection = _ds2 === 'failed'
            || _st2 === 'failed' || _st2 === 'error';

        container.innerHTML = `
            <div>
                <!-- Status bar -->
                <div style="display: flex; align-items: center; gap: 0.75rem; padding: 0.85rem 1.25rem; background: #f8fafc; border-radius: 8px; margin-bottom: 1.25rem; border: 1px solid #e2e8f0; flex-wrap: wrap;">
                    ${this.renderStatusBadge(message.status, message.delivery_status)}
                    ${deliveryBadge}
                    <span style="margin-left: auto; font-size: 0.8rem; color: #64748b;">${this.formatDateTime(message.received_at)}</span>
                </div>

                <!-- Info grid -->
                <div class="row g-3">
                    <div class="col-md-6">
                        <div style="background: white; border-radius: 8px; padding: 1.25rem; box-shadow: 0 1px 3px rgba(0,0,0,0.06); height: 100%;">
                            <div style="font-size: 0.7rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.08em; color: #94a3b8; margin-bottom: 0.85rem;">Message</div>
                            <table class="table table-sm table-borderless mb-0" style="font-size: 0.875rem;">
                                <tr><th style="width: 42%; color: #64748b; font-weight: 500; padding: 0.3rem 0;">Interface</th><td style="color: #1e293b; padding: 0.3rem 0;"><strong>${message.interface_name || '—'}</strong></td></tr>
                                <tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Type</th><td style="padding: 0.3rem 0;"><span style="background:#dbeafe;color:#1e40af;padding:0.1rem 0.5rem;border-radius:3px;font-size:0.78rem;">${message.message_type || '—'}</span></td></tr>
                                <tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Size</th><td style="color: #1e293b; padding: 0.3rem 0;">${this.formatBytes(message.message_size)}</td></tr>
                                <tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Encoding</th><td style="color: #1e293b; padding: 0.3rem 0;">${message.message_encoding || 'UTF-8'}</td></tr>
                                ${storageRow}
                            </table>
                        </div>
                    </div>
                    <div class="col-md-6">
                        <div style="background: white; border-radius: 8px; padding: 1.25rem; box-shadow: 0 1px 3px rgba(0,0,0,0.06); height: 100%;">
                            <div style="font-size: 0.7rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.08em; color: #94a3b8; margin-bottom: 0.85rem;">Processing</div>
                            <table class="table table-sm table-borderless mb-0" style="font-size: 0.875rem;">
                                <tr><th style="width: 48%; color: #64748b; font-weight: 500; padding: 0.3rem 0;">Source</th><td style="color: #1e293b; padding: 0.3rem 0;">${message.source_type || '—'}${message.source_endpoint ? ' · <span style="font-family:monospace;font-size:0.78rem;">' + this.escapeHtml(message.source_endpoint) + '</span>' : ''}</td></tr>
                                ${message.source_ip ? `<tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Source IP</th><td style="color: #1e293b; padding: 0.3rem 0;">${message.source_ip}</td></tr>` : ''}
                                <tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Duration</th><td style="color: #1e293b; padding: 0.3rem 0;"><strong>${this.calculateProcessingTimeForDetail(message)}</strong></td></tr>
                                ${parsingRow}
                                <tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Delivery tries</th><td style="color: #1e293b; padding: 0.3rem 0;">${message.delivery_attempts || 0}</td></tr>
                                ${message.error_count > 0 ? `<tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Errors</th><td style="padding: 0.3rem 0;"><span style="color: #ef4444; font-weight: 600;">${message.error_count}</span></td></tr>` : ''}
                                ${message.last_error_message ? `<tr><th style="color: #64748b; font-weight: 500; padding: 0.3rem 0;">Last error</th><td style="color: #ef4444; font-size: 0.82rem; padding: 0.3rem 0;">${this.escapeHtml(message.last_error_message)}</td></tr>` : ''}
                            </table>
                        </div>
                    </div>
                </div>

                ${hasDLQSection ? `
                <!-- Delivery Failures (DLQ) section -->
                <div id="dlqSection" style="margin-top:1.25rem;">
                    <div style="display:flex;align-items:center;gap:8px;margin-bottom:10px;">
                        <i class="fas fa-exclamation-triangle" style="color:#d97706;font-size:13px;"></i>
                        <span style="font-size:0.75rem;font-weight:700;text-transform:uppercase;letter-spacing:0.06em;color:#92400e;">Delivery Failures</span>
                        <span id="dlqLoadingSpinner" style="font-size:12px;color:#9ca3af;"><i class="fas fa-spinner fa-spin"></i></span>
                    </div>
                    <div id="dlqRowsContainer"></div>
                </div>` : ''}
            </div>
        `;

        if (hasDLQSection) {
            this.loadDLQRowsForMessage(message.message_id);
        }
    }

    async loadDLQRowsForMessage(messageId) {
        const container = document.getElementById('dlqRowsContainer');
        const spinner   = document.getElementById('dlqLoadingSpinner');
        if (!container) return;

        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(
                `/api/fhir/dlq?message_id=${encodeURIComponent(messageId)}&status=&limit=10`,
                { credentials: 'include', headers }
            );
            const data = await res.json();
            if (spinner) spinner.style.display = 'none';
            if (!data.success || !data.data || !data.data.length) {
                container.innerHTML = `<div style="font-size:12px;color:#9ca3af;padding:8px 0;">No active DLQ rows for this message.</div>`;
                return;
            }
            container.innerHTML = data.data.map(row => this.renderDLQRow(row)).join('');
        } catch (err) {
            if (spinner) spinner.style.display = 'none';
            if (container) container.innerHTML = `<div style="font-size:12px;color:#dc2626;">Failed to load DLQ data.</div>`;
        }
    }

    renderDLQRow(row) {
        const statusColour = { pending: '#92400e', retrying: '#1e40af', abandoned: '#991b1b', resolved: '#166534' };
        const statusBg     = { pending: '#fef3c7', retrying: '#dbeafe', abandoned: '#fee2e2', resolved: '#dcfce7' };
        const st = row.Status || 'pending';
        const isAbandoned = st === 'abandoned';
        const fmtDate = iso => iso ? new Date(iso).toLocaleString(undefined, { month:'short', day:'2-digit', hour:'2-digit', minute:'2-digit' }) : '—';

        return `
        <div style="background:#fff;border:1px solid #e5e7eb;border-radius:8px;padding:12px 14px;margin-bottom:8px;font-size:12px;">
            <div style="display:flex;align-items:center;gap:8px;flex-wrap:wrap;margin-bottom:6px;">
                <span style="background:${statusBg[st]};color:${statusColour[st]};padding:2px 8px;border-radius:10px;font-size:11px;font-weight:600;text-transform:uppercase;">${this.escapeHtml(st)}</span>
                <span style="color:#6b7280;">Connector: <strong>${this.escapeHtml(row.ConnectorType || '—')}</strong></span>
                <span style="color:#6b7280;">Attempts: <strong>${row.AttemptCount}</strong></span>
                ${row.NextRetryAt && st === 'pending' ? `<span style="color:#6b7280;">Next retry: <strong>${fmtDate(row.NextRetryAt)}</strong></span>` : ''}
                <span style="margin-left:auto;color:#9ca3af;">Created ${fmtDate(row.CreatedAt)}</span>
            </div>
            ${row.ErrorMessage ? `<div style="color:#dc2626;font-size:11px;margin-bottom:8px;word-break:break-word;">${this.escapeHtml(row.ErrorMessage)}</div>` : ''}
            <div style="display:flex;gap:6px;flex-wrap:wrap;">
                <button class="btn btn-sm" style="background:#2563eb;color:#fff;border:none;font-size:11px;padding:3px 10px;border-radius:4px;"
                    onclick="event.stopPropagation(); messageManager.dlqRedriveFromModal('${this.escapeHtml(row.ID)}', this)">
                    <i class="fas fa-${isAbandoned ? 'rotate-right' : 'redo'}"></i> ${isAbandoned ? 'Reactivate' : 'Redrive Now'}
                </button>
                <button class="btn btn-sm" style="background:#fff;color:#2563eb;border:1px solid #93c5fd;font-size:11px;padding:3px 10px;border-radius:4px;"
                    onclick="event.stopPropagation(); messageManager.dlqScheduleFromModal('${this.escapeHtml(row.ID)}', this)">
                    <i class="fas fa-calendar-alt"></i> Schedule
                </button>
                ${isAbandoned ? '' : `
                <button class="btn btn-sm" style="background:#fff;color:#dc2626;border:1px solid #fca5a5;font-size:11px;padding:3px 10px;border-radius:4px;"
                    onclick="event.stopPropagation(); messageManager.dlqAbandonFromModal('${this.escapeHtml(row.ID)}', this)">
                    <i class="fas fa-ban"></i> Abandon
                </button>`}
                <a href="admin-dlq.html?message_id=${encodeURIComponent(row.MessageID || '')}" target="_blank"
                   style="font-size:11px;color:#6b7280;padding:3px 8px;border:1px solid #e5e7eb;border-radius:4px;text-decoration:none;display:inline-flex;align-items:center;gap:4px;"
                   onclick="event.stopPropagation()">
                    <i class="fas fa-external-link-alt"></i> Full DLQ
                </a>
            </div>
        </div>`;
    }

    async dlqRedriveFromModal(dlqId, btn) {
        btn.disabled = true;
        btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i>';
        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(`/api/fhir/dlq/${dlqId}/redrive`, {
                method: 'POST', credentials: 'include', headers,
                body: JSON.stringify({ mode: 'from_failed_step' }),
            });
            const data = await res.json();
            if (data.success) {
                AppDialogs.toast('Redrive triggered', 'success');
                // Reload DLQ section
                const msgId = this.selectedMessageId;
                if (msgId) this.loadDLQRowsForMessage(msgId);
            } else {
                AppDialogs.toast(data.error || 'Redrive failed', 'error');
                btn.disabled = false;
                btn.innerHTML = '<i class="fas fa-redo"></i> Redrive Now';
            }
        } catch (_) {
            AppDialogs.toast('Network error', 'error');
            btn.disabled = false;
        }
    }

    async dlqScheduleFromModal(dlqId, btn) {
        const atStr = prompt('Schedule redrive at (YYYY-MM-DDTHH:MM, local time):',
            new Date(Date.now() + 3600000).toISOString().slice(0,16));
        if (!atStr) return;
        const at = new Date(atStr).toISOString();
        btn.disabled = true;
        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(`/api/fhir/dlq/${dlqId}/schedule`, {
                method: 'POST', credentials: 'include', headers,
                body: JSON.stringify({ mode: 'from_failed_step', at }),
            });
            const data = await res.json();
            if (data.success) {
                AppDialogs.toast('Redrive scheduled', 'success');
                if (this.selectedMessageId) this.loadDLQRowsForMessage(this.selectedMessageId);
            } else {
                AppDialogs.toast(data.error || 'Schedule failed', 'error');
            }
        } catch (_) {
            AppDialogs.toast('Network error', 'error');
        } finally {
            btn.disabled = false;
        }
    }

    async dlqAbandonFromModal(dlqId, btn) {
        if (!confirm('Abandon this DLQ row? It will not be retried again.')) return;
        btn.disabled = true;
        try {
            const token = localStorage.getItem('accessToken');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            const res = await fetch(`/api/fhir/dlq/${dlqId}/abandon`, {
                method: 'POST', credentials: 'include', headers,
            });
            const data = await res.json();
            if (data.success) {
                AppDialogs.toast('Row abandoned', 'success');
                if (this.selectedMessageId) this.loadDLQRowsForMessage(this.selectedMessageId);
            } else {
                AppDialogs.toast(data.error || 'Abandon failed', 'error');
                btn.disabled = false;
            }
        } catch (_) {
            AppDialogs.toast('Network error', 'error');
            btn.disabled = false;
        }
    }

    openDLQForMessage(messageId) {
        window.open(`admin-dlq.html?message_id=${encodeURIComponent(messageId)}`, '_blank');
    }

    async loadMessageContent(messageId) {
        const container = document.getElementById('messageContentView');
        if (!container || !this.messageData) return;

        const { message } = this.messageData;

        container.innerHTML = '<div class="text-center" style="padding:2rem;"><i class="fas fa-spinner fa-spin"></i> Loading content...</div>';

        // Fetch inbound and outbound content in parallel
        const [rawResult, transformedResult] = await Promise.allSettled([
            fetch(`/api/messages/${messageId}/raw`).then(r => r.ok ? r.json() : null),
            fetch(`/api/messages/${messageId}/transformed`).then(r => r.ok ? r.json() : null),
        ]);

        const rawData         = rawResult.status === 'fulfilled' ? rawResult.value : null;
        const transformedData = transformedResult.status === 'fulfilled' ? transformedResult.value : null;
        let rawContent      = rawData?.content || null;
        let outboundContent = transformedData?.content || null;
        // source = "outbound" → exact delivered payload; "pipeline" → fallback pipeline context
        const outboundSource  = transformedData?.source || 'pipeline';

        // Fallback: use inline raw_message from the already-loaded message record
        if (!rawContent && message.raw_message) {
            rawContent = message.raw_message;
        }

        // Fallback: fetch stored FHIR bundle from cda_documents for CDA/CCD messages
        if (!outboundContent && this.currentInterfaceId &&
            (message.message_type === 'CCD' || (rawContent && rawContent.trimStart().startsWith('<')))) {
            try {
                const fhirResp = await fetch(
                    `/api/messages/interface/${this.currentInterfaceId}/message/${messageId}/fhir-output`
                ).then(r => r.ok ? r.json() : null);
                if (fhirResp && fhirResp.success && fhirResp.content) {
                    outboundContent = fhirResp.content;
                }
            } catch (_) { /* degrade gracefully */ }
        }

        if (!rawContent && !outboundContent) {
            container.innerHTML = `<div style="padding:2.5rem;text-align:center;color:#94a3b8;background:#f8fafc;border-radius:8px;border:2px dashed #e2e8f0;">
                <i class="fas fa-cloud-upload-alt" style="font-size:2rem;margin-bottom:0.75rem;display:block;"></i>
                <div style="font-weight:600;color:#64748b;margin-bottom:0.35rem;">Content not yet available</div>
                <div style="font-size:0.82rem;">Message content is stored in object storage.<br>It may still be uploading or object storage may be unavailable.</div>
            </div>`;
            return;
        }

        // Detect CDA/CCD content: message_type "CCD" (agreed convention) OR XML ClinicalDocument content
        const isCDA = message.message_type === 'CCD'
            || (rawContent && (rawContent.includes('<ClinicalDocument') || rawContent.includes('ClinicalDocument>')));

        if (isCDA && typeof ClinicalDocumentViewer !== 'undefined') {
            const height = 'calc(90vh - 280px)';
            const interfaceId = this.currentInterfaceId || '';

            // Left pane: three overlapping panes (Raw XML / CDA Viewer / Parsed CCD), switched
            // via a persistent button bar shown identically in all three (_cdaButtonBar).
            // Right pane: pipeline output / sent payload (existing toggle)
            container.innerHTML = `
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;height:${height};min-height:0;">
                    <div id="cdaLeftPanel"
                         data-message-id="${messageId}"
                         data-interface-id="${interfaceId}"
                         style="height:100%;min-height:0;position:relative;">
                        <div id="cdaRawPane" data-cda-pane="raw" style="height:100%;display:flex;flex-direction:column;">
                            ${this._contentPanel('inbound', 'Inbound', rawContent, message.raw_content_uri, { source: 'raw', messageId, customToggleHtml: this._cdaButtonBar('raw') })}
                        </div>
                        <div id="cdaViewerPane" data-cda-pane="viewer" style="position:absolute;inset:0;display:none;flex-direction:column;background:#fff;border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;">
                            <div style="display:flex;align-items:center;gap:0.5rem;padding:0.6rem 0.85rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;flex-shrink:0;">
                                <span style="font-size:0.8rem;font-weight:700;color:#374151;">Clinical Document</span>
                                <div style="margin-left:auto;"><span data-cda-btnbar>${this._cdaButtonBar('viewer')}</span></div>
                            </div>
                            <div id="cdaViewerMount" style="flex:1;min-height:0;overflow:auto;"></div>
                        </div>
                        <div id="cdaParsedPane" data-cda-pane="parsed" style="position:absolute;inset:0;display:none;flex-direction:column;background:#fff;border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;">
                            <div style="display:flex;align-items:center;gap:0.5rem;padding:0.6rem 0.85rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;flex-shrink:0;">
                                <span style="font-size:0.8rem;font-weight:700;color:#374151;">Clinical Document</span>
                                <div style="margin-left:auto;display:flex;gap:0.4rem;align-items:center;">
                                    <span data-cda-btnbar>${this._cdaButtonBar('parsed')}</span>
                                    <button onclick="messageManager._downloadJsonTree('cdaParsedMount','parsed-ccd-${messageId}.json')"
                                        style="background:#f1f5f9;color:#475569;border:1px solid #cbd5e1;padding:0.25rem 0.6rem;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                                        <i class="fas fa-download"></i> Download</button>
                                    <button onclick="messageManager._copyJsonTree('cdaParsedMount')"
                                        style="background:#2563eb;color:white;border:none;padding:0.25rem 0.6rem;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                                        <i class="fas fa-copy"></i> Copy</button>
                                </div>
                            </div>
                            <div id="cdaParsedMount" style="flex:1;min-height:0;overflow:auto;"></div>
                        </div>
                    </div>
                    ${this._contentPanel('outbound', 'Pipeline Output', outboundContent, message.transformed_content_uri, { source: outboundSource, messageId })}
                </div>`;
            return;
        }

        // Detect FHIR Bundle on the outbound side — offer narrative view
        const isFHIRBundle = outboundContent && (() => {
            try { const p = JSON.parse(outboundContent); return p.resourceType === 'Bundle'; }
            catch { return false; }
        })();

        // Outbound panel label and toggle depend on whether we have the exact sent payload
        const outboundLabel = outboundSource === 'outbound' ? 'Outbound (Sent)' : 'Pipeline Output';

        if (isFHIRBundle && typeof ClinicalDocumentViewer !== 'undefined') {
            const height = 'calc(90vh - 280px)';
            container.innerHTML = `
                <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;height:${height};min-height:0;">
                    ${this._contentPanel('inbound', 'Inbound', rawContent, message.raw_content_uri, { source: 'raw', messageId })}
                    <div id="fhirViewerMount" style="height:100%;min-height:0;"></div>
                </div>`;

            const fhirBundle = JSON.parse(outboundContent);
            const viewer = new ClinicalDocumentViewer(
                document.getElementById('fhirViewerMount'),
                { format: 'fhir', parsedContent: fhirBundle }
            );
            await viewer.mount();
            return;
        }

        container.innerHTML = `<div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;height:calc(90vh - 280px);min-height:0;">
            ${this._contentPanel('inbound', 'Inbound', rawContent, message.raw_content_uri, { source: 'raw', messageId })}
            ${this._contentPanel('outbound', outboundLabel, outboundContent, message.transformed_content_uri, { source: outboundSource, messageId })}
        </div>`;
    }

    async _toggleOutboundSource(messageId, currentSource) {
        const nextSource = currentSource === 'outbound' ? 'pipeline' : 'outbound';
        const url = nextSource === 'pipeline'
            ? `/api/messages/${messageId}/transformed?source=pipeline`
            : `/api/messages/${messageId}/transformed`;
        try {
            const data = await fetch(url).then(r => r.ok ? r.json() : null);
            if (!data) return;
            const container = document.getElementById('messageContentView');
            if (!container) return;
            // Re-render just the outbound panel — replace it in the grid
            const rightPanel = container.querySelector('[data-panel="outbound"]');
            if (!rightPanel) return;
            const label = (data.source || nextSource) === 'outbound' ? 'Outbound (Sent)' : 'Pipeline Output';
            rightPanel.outerHTML = this._contentPanel('outbound', label, data.content, null, { source: data.source || nextSource, messageId });
        } catch(e) { /* ignore */ }
    }

    // Toggle the CDA left panel between raw XML and the rendered CDA viewer.
    // Renders the persistent 3-button bar (Raw XML / CDA Viewer / Parsed CCD) shown
    // identically in all three CDA content panes; the active pane's button is disabled
    // and highlighted so users can jump directly between any of the three at any time.
    _cdaButtonBar(active) {
        const buttons = [
            { key: 'raw',    label: 'Raw XML',    icon: 'fa-code' },
            { key: 'viewer', label: 'CDA Viewer',  icon: 'fa-file-medical-alt' },
            { key: 'parsed', label: 'Parsed CCD',  icon: 'fa-sitemap' },
        ];
        return `<div style="display:flex;gap:0.4rem;">` + buttons.map(b => {
            const isActive = b.key === active;
            return `<button onclick="messageManager._switchCDAPane('${b.key}')" ${isActive ? 'disabled' : ''}
                style="background:${isActive ? '#2563eb' : '#f1f5f9'};color:${isActive ? '#fff' : '#475569'};border:1px solid ${isActive ? '#2563eb' : '#cbd5e1'};padding:0.2rem 0.5rem;border-radius:4px;cursor:${isActive ? 'default' : 'pointer'};font-size:0.72rem;font-weight:${isActive ? '600' : '400'};">
                <i class="fas ${b.icon}"></i> ${b.label}</button>`;
        }).join('') + `</div>`;
    }

    // Switches the CDA left panel between Raw XML / CDA Viewer / Parsed CCD, lazy-mounting
    // each view on first visit and re-rendering the button bar in all three panes so the
    // active state stays consistent regardless of which pane the user switches from.
    async _switchCDAPane(target) {
        const leftPanel = document.getElementById('cdaLeftPanel');
        if (!leftPanel) return;
        const panes = {
            raw:    document.getElementById('cdaRawPane'),
            viewer: document.getElementById('cdaViewerPane'),
            parsed: document.getElementById('cdaParsedPane'),
        };
        for (const [key, el] of Object.entries(panes)) {
            if (!el) continue;
            el.style.display = (key === target) ? 'flex' : 'none';
            // Re-render the button bar inside this pane's header so all three stay in sync.
            const bar = el.querySelector('[data-cda-btnbar]');
            if (bar) bar.innerHTML = this._cdaButtonBar(target);
        }

        const interfaceId = leftPanel.dataset.interfaceId;
        const messageId   = leftPanel.dataset.messageId;

        if (target === 'viewer') {
            const mountEl = document.getElementById('cdaViewerMount');
            if (mountEl && !mountEl.dataset.mounted && typeof ClinicalDocumentViewer !== 'undefined') {
                mountEl.dataset.mounted = '1';
                const viewer = new ClinicalDocumentViewer(mountEl, { interfaceId, messageId, format: 'ccda' });
                await viewer.mount();
            }
        } else if (target === 'parsed') {
            const mountEl = document.getElementById('cdaParsedMount');
            if (mountEl && !mountEl.dataset.loaded) {
                mountEl.dataset.loaded = '1';
                mountEl.innerHTML = '<div style="padding:1rem;color:#94a3b8;font-size:0.85rem;"><i class="fas fa-spinner fa-spin"></i> Loading parsed CCD JSON...</div>';
                try {
                    const resp = await fetch('/api/messages/' + encodeURIComponent(messageId) +
                        '/parsed-content?interfaceId=' + encodeURIComponent(interfaceId));
                    const data = await resp.json();
                    if (resp.ok && data.success && data.parsedContent) {
                        this._renderJsonTree(mountEl, data.parsedContent);
                    } else {
                        mountEl.innerHTML = `<div style="padding:1rem;color:#94a3b8;font-size:0.85rem;">${this._escapeHtml(data.error || 'Parsed CCD JSON not available for this message')}</div>`;
                    }
                } catch (err) {
                    mountEl.innerHTML = `<div style="padding:1rem;color:#94a3b8;font-size:0.85rem;">Failed to load parsed CCD JSON: ${this._escapeHtml(err.message)}</div>`;
                }
            }
        }
    }

    // opts: { source: 'outbound'|'pipeline'|'raw', messageId }
    _contentPanel(side, label, content, uri, opts = {}) {
        const idSuffix = side + '-' + Date.now();
        const isEmpty = !content;
        const { source, messageId } = opts;

        const isHL7  = content && (content.startsWith('MSH|') || content.includes('\rMSH|'));
        const isCDAXML = !isHL7 && content && (content.includes('<ClinicalDocument') || content.includes('ClinicalDocument>'));
        const isFHIR = !isHL7 && !isCDAXML && content && (content.trim().startsWith('{') || content.trim().startsWith('['));

        let typeTag, highlighted;
        if (isEmpty) {
            typeTag = '';
            highlighted = `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:#94a3b8;font-size:0.85rem;">
                <span><i class="fas fa-minus-circle" style="margin-right:0.4rem;"></i>Not available</span></div>`;
        } else if (isHL7) {
            typeTag = `<span style="background:#dbeafe;color:#1e40af;padding:0.2rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">HL7 v2</span>`;
            highlighted = this._highlightHL7(content);
        } else if (isCDAXML) {
            typeTag = `<span style="background:#fef3c7;color:#92400e;padding:0.2rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">CDA/CCD XML</span>`;
            highlighted = this._escapeHtml(content);
        } else if (isFHIR) {
            typeTag = `<span style="background:#dcfce7;color:#166534;padding:0.2rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">FHIR JSON</span>`;
            try { highlighted = this._escapeHtml(JSON.stringify(JSON.parse(content), null, 2)); }
            catch { highlighted = this._escapeHtml(content); }
        } else {
            typeTag = `<span style="background:#f3f4f6;color:#374151;padding:0.2rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">Text</span>`;
            highlighted = this._escapeHtml(content);
        }

        const sizeStr = content ? this.formatBytes(content.length) : '';
        const uriHtml = uri ? `<span style="color:#94a3b8;font-size:0.7rem;font-family:monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:200px;" title="${uri}">📦 ${uri.split('/').slice(-1)[0]}</span>` : '';

        // Toggle buttons: pipeline-output toggle (outbound), or a caller-supplied bar
        // (e.g. the CDA Viewer/Raw XML/Parsed CCD button group) that replaces all
        // default toggle logic for this panel.
        let toggleBtn = '';
        let sourceNote = '';
        if (opts.customToggleHtml) {
            toggleBtn = opts.customToggleHtml;
        } else if (side === 'outbound' && messageId) {
            if (source === 'outbound') {
                toggleBtn = `<button onclick="messageManager._toggleOutboundSource('${messageId}','outbound')"
                    title="View full pipeline context (all step variables)"
                    style="background:#f1f5f9;color:#475569;border:1px solid #cbd5e1;padding:0.2rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;">
                    <i class="fas fa-code-branch"></i> Pipeline Output</button>`;
            } else if (source === 'pipeline') {
                sourceNote = `<span style="background:#fef3c7;color:#92400e;border:1px solid #fde68a;padding:0.15rem 0.5rem;border-radius:4px;font-size:0.72rem;" title="No outbound connector payload found; showing pipeline context">⚠ Pipeline fallback</span>`;
                toggleBtn = `<button onclick="messageManager._toggleOutboundSource('${messageId}','pipeline')"
                    title="Try to load the exact delivered payload"
                    style="background:#f1f5f9;color:#475569;border:1px solid #cbd5e1;padding:0.2rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;">
                    <i class="fas fa-arrow-left"></i> Sent Payload</button>`;
            }
        }
        // Parsed JSON toggle: shown on inbound panels that don't supply their own button
        // bar — the literal ParserResult.ParsedJSON that engine_message_processor.go's
        // auto JSON-conversion step produces and hands to the transformation pipeline as
        // input. Available for every format (HL7, CDA, FHIR).
        let parsedJsonBtn = '';
        if (!opts.customToggleHtml && side === 'inbound' && messageId) {
            parsedJsonBtn = `<button onclick="messageManager._toggleParsedJSON(true,'${idSuffix}','${messageId}')"
                title="Show the JSON document this message was converted to for downstream pipeline processing"
                style="background:#ede9fe;color:#5b21b6;border:1px solid #ddd6fe;padding:0.2rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;">
                <i class="fas fa-code"></i> Parsed JSON</button>`;
        }

        return `
        <div data-panel="${side}" style="position:relative;height:100%;min-height:0;display:flex;flex-direction:column;background:#fff;border:1px solid #e2e8f0;border-radius:8px;overflow:hidden;">
            <div style="display:flex;align-items:center;gap:0.5rem;padding:0.6rem 0.85rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;flex-shrink:0;flex-wrap:wrap;">
                <span style="font-size:0.8rem;font-weight:700;color:#374151;">${label}</span>
                ${typeTag}
                ${sourceNote}
                <span style="color:#94a3b8;font-size:0.78rem;">${sizeStr}</span>
                ${uriHtml}
                <div style="margin-left:auto;display:flex;gap:0.4rem;align-items:center;">
                    <span data-cda-btnbar>${toggleBtn}</span>
                    ${parsedJsonBtn}
                    ${!isEmpty ? `<button onclick="messageManager._copyContentById('${idSuffix}')"
                        style="background:#2563eb;color:white;border:none;padding:0.25rem 0.6rem;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                        <i class="fas fa-copy"></i> Copy</button>` : ''}
                </div>
            </div>
            <div id="${idSuffix}" style="flex:1;overflow:auto;padding:1rem;background:#0f172a;color:#e2e8f0;font-family:'Courier New',monospace;font-size:0.78rem;line-height:1.6;white-space:pre-wrap;word-break:break-all;">${highlighted}</div>
            ${parsedJsonBtn ? `
            <div id="parsedJsonPane-${idSuffix}" style="position:absolute;inset:0;display:none;flex-direction:column;background:#fff;">
                <div style="display:flex;align-items:center;gap:0.5rem;padding:0.6rem 0.85rem;background:#f8fafc;border-bottom:1px solid #e2e8f0;flex-shrink:0;">
                    <span style="font-size:0.8rem;font-weight:700;color:#374151;">Parsed JSON</span>
                    <span style="background:#ede9fe;color:#5b21b6;padding:0.2rem 0.55rem;border-radius:4px;font-size:0.75rem;font-weight:600;">Downstream pipeline input</span>
                    <div style="margin-left:auto;display:flex;gap:0.4rem;align-items:center;">
                        <button onclick="messageManager._toggleParsedJSON(false,'${idSuffix}')"
                            style="background:#f1f5f9;color:#475569;border:1px solid #cbd5e1;padding:0.2rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;">
                            <i class="fas fa-arrow-left"></i> Back</button>
                        <button onclick="messageManager._downloadJsonTree('parsedJsonMount-${idSuffix}','parsed-content-${messageId}.json')"
                            style="background:#f1f5f9;color:#475569;border:1px solid #cbd5e1;padding:0.2rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;">
                            <i class="fas fa-download"></i> Download</button>
                        <button onclick="messageManager._copyJsonTree('parsedJsonMount-${idSuffix}')"
                            style="background:#2563eb;color:white;border:none;padding:0.25rem 0.6rem;border-radius:4px;cursor:pointer;font-size:0.75rem;">
                            <i class="fas fa-copy"></i> Copy</button>
                    </div>
                </div>
                <div id="parsedJsonMount-${idSuffix}" style="flex:1;overflow:auto;padding:1rem;background:#0f172a;color:#e2e8f0;font-family:'Courier New',monospace;font-size:0.78rem;line-height:1.6;white-space:pre-wrap;word-break:break-all;"></div>
            </div>` : ''}
        </div>`;
    }

    // Toggles the generic "Parsed JSON" overlay for any inbound content panel —
    // fetches /api/messages/:messageId/parsed-content (the JSON produced by the
    // automatic format-detection/parse step that feeds the transformation pipeline).
    async _toggleParsedJSON(show, idSuffix, messageId) {
        const panel = document.getElementById('parsedJsonPane-' + idSuffix);
        const rawEl = document.getElementById(idSuffix);
        if (!panel || !rawEl) return;

        if (!show) {
            panel.style.display = 'none';
            rawEl.style.display = 'block';
            return;
        }

        rawEl.style.display = 'none';
        panel.style.display = 'flex';
        const mountEl = document.getElementById('parsedJsonMount-' + idSuffix);
        if (!mountEl || mountEl.dataset.loaded) return;
        mountEl.dataset.loaded = '1';
        mountEl.innerHTML = '<div style="color:#94a3b8;"><i class="fas fa-spinner fa-spin"></i> Loading parsed JSON...</div>';
        try {
            const interfaceId = this.currentInterfaceId || '';
            const resp = await fetch('/api/messages/' + encodeURIComponent(messageId) +
                '/parsed-content?interfaceId=' + encodeURIComponent(interfaceId));
            const data = await resp.json();
            if (resp.ok && data.success && data.parsedContent) {
                this._renderJsonTree(mountEl, data.parsedContent);
            } else {
                mountEl.innerHTML = `<div style="color:#94a3b8;">${this._escapeHtml(data.error || 'Parsed JSON not available for this message')}</div>`;
            }
        } catch (err) {
            mountEl.innerHTML = `<div style="color:#94a3b8;">Failed to load parsed JSON: ${this._escapeHtml(err.message)}</div>`;
        }
    }

    _escapeHtml(s) {
        return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
    }

    // Render delivery-stage log details as a structured HTTP request/response card
    _renderDeliveryDetails(d, style) {
        const req = d.request || {};
        const statusCode = d.response_status;
        const isOk = statusCode && parseInt(statusCode) < 400;
        const statusColor = isOk ? '#166534' : '#dc2626';
        const statusBg    = isOk ? '#dcfce7' : '#fee2e2';

        const row = (label, value, mono) => value != null && value !== ''
            ? `<tr><td style="color:#64748b;padding:0.2rem 0.6rem 0.2rem 0;white-space:nowrap;font-size:0.78rem;">${label}</td>
               <td style="font-family:${mono?'monospace':'inherit'};font-size:0.78rem;">${this._escapeHtml(String(value))}</td></tr>`
            : '';

        // Request body: if body_uri exists the full payload is in the Content tab (outbound panel).
        // Show the preview here; the toggle button lets users switch to Content tab for the full view.
        const isTruncated = req.body_preview && req.body_preview.endsWith('... (truncated)');
        const fullPayloadNote = req.body_uri
            ? `<div style="margin-top:0.35rem;font-size:0.75rem;color:#6366f1;">
                 <i class="fas fa-arrow-up" style="margin-right:0.3rem;"></i>Full payload shown in <strong>Content tab → Outbound (Sent)</strong>
               </div>`
            : (isTruncated ? `<div style="margin-top:0.35rem;font-size:0.75rem;color:#f59e0b;">⚠ Preview truncated — full payload unavailable (older message)</div>` : '');

        const reqBody = req.body_preview
            ? `<details style="margin-top:0.5rem;">
                 <summary style="cursor:pointer;font-size:0.78rem;color:#3b82f6;font-weight:600;">Request Body Preview</summary>
                 <pre style="margin:0.4rem 0 0;background:#0f172a;color:#e2e8f0;padding:0.6rem;border-radius:4px;font-size:0.75rem;overflow:auto;max-height:160px;white-space:pre-wrap;">${this._escapeHtml(req.body_preview)}</pre>
                 ${fullPayloadNote}
               </details>` : fullPayloadNote;

        const respBody = d.response_body_preview
            ? `<details style="margin-top:0.5rem;">
                 <summary style="cursor:pointer;font-size:0.78rem;color:#10b981;font-weight:600;">Response Body</summary>
                 <pre style="margin:0.4rem 0 0;background:#0f172a;color:#e2e8f0;padding:0.6rem;border-radius:4px;font-size:0.75rem;overflow:auto;max-height:200px;white-space:pre-wrap;">${this._escapeHtml(d.response_body_preview)}</pre>
               </details>` : '';

        return `<div style="margin-top:0.6rem;display:grid;grid-template-columns:1fr 1fr;gap:0.6rem;">
            <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem;">
                <div style="font-size:0.75rem;font-weight:700;color:#374151;margin-bottom:0.4rem;text-transform:uppercase;letter-spacing:0.05em;">Request</div>
                <table style="width:100%;border-collapse:collapse;">
                    ${row('Connector', d.connector_type, true)}
                    ${row('Destination', req.endpoint, true)}
                    ${row('Method', req.method, false)}
                    ${row('Content-Type', req.content_type, true)}
                    ${row('Size', req.body_size ? this.formatBytes(req.body_size) : null, false)}
                </table>
                ${reqBody}
            </div>
            <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:0.6rem;">
                <div style="font-size:0.75rem;font-weight:700;color:#374151;margin-bottom:0.4rem;text-transform:uppercase;letter-spacing:0.05em;">Response</div>
                ${statusCode ? `<div style="display:inline-block;background:${statusBg};color:${statusColor};font-weight:700;font-size:0.85rem;padding:0.15rem 0.55rem;border-radius:4px;margin-bottom:0.4rem;">${statusCode}</div>` : ''}
                <table style="width:100%;border-collapse:collapse;">
                    ${row('Duration', d.duration_ms != null ? d.duration_ms + ' ms' : null, false)}
                    ${row('Bytes Sent', d.bytes_sent ? this.formatBytes(d.bytes_sent) : null, false)}
                    ${row('Success', d.success != null ? String(d.success) : null, false)}
                </table>
                ${respBody}
            </div>
        </div>`;
    }

    // Render non-delivery log details as compact key: value lines
    _renderGenericDetails(details) {
        const lines = Object.entries(details)
            .filter(([, v]) => v != null && v !== '')
            .map(([k, v]) => `<span style="color:#64748b;">${k}:</span> <span style="font-family:monospace;">${this._escapeHtml(typeof v === 'object' ? JSON.stringify(v) : String(v))}</span>`)
            .join(' &nbsp;·&nbsp; ');
        return `<div style="margin-top:0.4rem;font-size:0.8rem;padding:0.4rem 0.6rem;background:rgba(0,0,0,0.03);border-radius:4px;line-height:1.7;">${lines}</div>`;
    }

    _highlightHL7(raw) {
        const esc = s => s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
        // Normalize segment delimiters to \n for display
        const lines = raw.replace(/\r\n/g, '\r').replace(/\n/g, '\r').split('\r').filter(l => l.trim());
        return lines.map(line => {
            const seg = line.substring(0, 3);
            const rest = esc(line.substring(3));
            const segColors = { MSH: '#93c5fd', PID: '#86efac', PV1: '#fde68a', OBX: '#f9a8d4', OBR: '#c4b5fd', NK1: '#6ee7b7', EVN: '#fdba74' };
            const color = segColors[seg] || '#cbd5e1';
            return `<span style="color:${color};font-weight:600;">${seg}</span>${rest}`;
        }).join('\n');
    }

    _copyContentById(elementId) {
        const el = document.getElementById(elementId);
        if (!el) return;
        navigator.clipboard.writeText(el.textContent).then(() => {
            const btn = event.target.closest('button');
            const orig = btn.innerHTML;
            btn.innerHTML = '<i class="fas fa-check"></i> Copied!';
            setTimeout(() => { btn.innerHTML = orig; }, 2000);
        });
    }

    // ---- Lazy JSON tree viewer --------------------------------------------------
    // Renders a parsed CCD/FHIR/HL7 JSON document as a collapsible tree instead of
    // dumping the whole JSON.stringify() output into one pre-wrap text block. Only
    // the root level is built/expanded eagerly; every nested object/array starts
    // collapsed as a one-line "{n keys}" / "[n items]" summary and its rows are only
    // built into the DOM the first time it's expanded. Wide arrays/objects page in
    // batches of 200 via a "Show more" row. This keeps render cost proportional to
    // what the user actually opens, regardless of total document size — avoids the
    // full-document JSON.stringify + giant pre-wrap textContent that froze the tab.
    _renderJsonTree(mountEl, rawValue) {
        mountEl.dataset.loaded = '1';
        // Only one tree is ever mounted per message-detail view, so it's safe (and
        // necessary to avoid leaking every previously-viewed document's full parsed
        // tree in memory) to drop the previous tree's node registry here.
        this._jsonTreeNodes = {};
        this._jsonTreeSeq = 0;
        this._jsonTreeRaw[mountEl.id] = rawValue;
        mountEl.style.cssText = "flex:1;min-height:0;overflow:auto;padding:0.85rem 1rem;background:#0f172a;color:#e2e8f0;font-family:'Courier New',monospace;font-size:0.78rem;line-height:1.6;";
        mountEl.innerHTML = this._renderJsonValue(rawValue, 0);
    }

    _renderJsonValue(value, depth) {
        if (value === null || value === undefined) return `<span style="color:#64748b;">null</span>`;
        const t = typeof value;
        if (t === 'string') return `<span style="color:#86efac;">"${this._escapeHtml(value)}"</span>`;
        if (t === 'number') return `<span style="color:#93c5fd;">${value}</span>`;
        if (t === 'boolean') return `<span style="color:#fca5a5;">${value}</span>`;
        if (Array.isArray(value)) return this._renderJsonContainer(value, true, depth);
        if (t === 'object') return this._renderJsonContainer(value, false, depth);
        return this._escapeHtml(String(value));
    }

    _renderJsonContainer(value, isArray, depth) {
        const entries = isArray ? value.map((v, i) => [i, v]) : Object.entries(value);
        if (entries.length === 0) return isArray ? '[]' : '{}';

        const id = 'jt' + (++this._jsonTreeSeq);
        this._jsonTreeNodes[id] = { entries, isArray, depth };
        const expanded = depth === 0;
        const noun = isArray ? (entries.length === 1 ? 'item' : 'items') : (entries.length === 1 ? 'key' : 'keys');
        const summary = `${entries.length} ${noun}`;

        const head = `<span data-jt-toggle="${id}" onclick="messageManager._toggleJsonNode('${id}')" style="cursor:pointer;user-select:none;">` +
            `<i class="fas fa-caret-${expanded ? 'down' : 'right'}" style="width:0.85em;display:inline-block;color:#64748b;"></i>` +
            `${isArray ? '[' : '{'}` +
            `<span data-jt-summary="${id}" style="display:${expanded ? 'none' : 'inline'};color:#64748b;font-style:italic;"> ${summary} ${isArray ? ']' : '}'}</span>` +
            `</span>`;

        const body = `<div data-jt-body="${id}" style="display:${expanded ? 'block' : 'none'};margin-left:1.2rem;border-left:1px dashed #334155;padding-left:0.7rem;">` +
            (expanded ? this._buildJsonChildrenHtml(id, 0, 200) : '') +
            `</div>`;
        const closeBracket = `<span data-jt-close="${id}" style="display:${expanded ? 'inline' : 'none'};">${isArray ? ']' : '}'}</span>`;

        return head + body + closeBracket;
    }

    _buildJsonChildrenHtml(id, offset, limit) {
        const node = this._jsonTreeNodes[id];
        if (!node) return '';
        const { entries, isArray, depth } = node;
        const slice = entries.slice(offset, offset + limit);
        let html = slice.map(([key, val], i) => {
            const isLast = (offset + i) === entries.length - 1;
            const keyHtml = isArray ? '' : `<span style="color:#7dd3fc;">"${this._escapeHtml(String(key))}"</span><span style="color:#64748b;">: </span>`;
            return `<div>${keyHtml}${this._renderJsonValue(val, depth + 1)}${isLast ? '' : '<span style="color:#64748b;">,</span>'}</div>`;
        }).join('');

        const remaining = entries.length - (offset + limit);
        if (remaining > 0) {
            const nextOffset = offset + limit;
            html += `<div data-jt-more><button onclick="messageManager._loadMoreJsonNode('${id}', ${nextOffset})" ` +
                `style="background:#1e293b;color:#94a3b8;border:1px solid #334155;padding:0.15rem 0.5rem;border-radius:4px;cursor:pointer;font-size:0.72rem;margin:0.2rem 0;">` +
                `Show ${Math.min(200, remaining)} more (${remaining} remaining)</button></div>`;
        }
        return html;
    }

    _toggleJsonNode(id) {
        const body = document.querySelector(`[data-jt-body="${id}"]`);
        const toggle = document.querySelector(`[data-jt-toggle="${id}"]`);
        const summary = document.querySelector(`[data-jt-summary="${id}"]`);
        const close = document.querySelector(`[data-jt-close="${id}"]`);
        if (!body || !toggle) return;

        const isExpanded = body.style.display !== 'none';
        if (isExpanded) {
            body.style.display = 'none';
            if (summary) summary.style.display = 'inline';
            if (close) close.style.display = 'none';
            toggle.querySelector('i').className = 'fas fa-caret-right';
        } else {
            if (!body.dataset.built) {
                body.innerHTML = this._buildJsonChildrenHtml(id, 0, 200);
                body.dataset.built = '1';
            }
            body.style.display = 'block';
            if (summary) summary.style.display = 'none';
            if (close) close.style.display = 'inline';
            toggle.querySelector('i').className = 'fas fa-caret-down';
        }
    }

    _loadMoreJsonNode(id, offset) {
        const body = document.querySelector(`[data-jt-body="${id}"]`);
        if (!body) return;
        const moreRow = body.querySelector('[data-jt-more]');
        if (moreRow) moreRow.remove();
        body.insertAdjacentHTML('beforeend', this._buildJsonChildrenHtml(id, offset, 200));
    }

    // Copies the full raw JSON document, independent of what's currently expanded
    // in the tree — the parsed object is kept in _jsonTreeRaw, not derived from DOM.
    _copyJsonTree(mountId) {
        const raw = this._jsonTreeRaw[mountId];
        if (raw === undefined) return;
        navigator.clipboard.writeText(JSON.stringify(raw, null, 2)).then(() => {
            const btn = event.target.closest('button');
            if (!btn) return;
            const orig = btn.innerHTML;
            btn.innerHTML = '<i class="fas fa-check"></i> Copied!';
            setTimeout(() => { btn.innerHTML = orig; }, 2000);
        });
    }

    _downloadJsonTree(mountId, filename) {
        const raw = this._jsonTreeRaw[mountId];
        if (raw === undefined) return;
        const blob = new Blob([JSON.stringify(raw, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename || 'parsed-content.json';
        a.click();
        URL.revokeObjectURL(url);
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

    // Auto-detect content type and message type from raw pasted content so the
    // user doesn't have to pick them manually. Mirrors the sniffing the Go
    // format detector does for live messages (services/format_detector.go).
    detectMessageFormat(content) {
        const trimmed = (content || '').trim();

        if (/^MSH\|/.test(trimmed)) {
            // HL7 v2: MSH.9 (message type, e.g. ADT^A01) is the 9th field.
            const fields = trimmed.split(/\r?\n/)[0].split('|');
            const messageType = fields[8] || 'HL7';
            return { contentType: 'application/hl7-v2', messageType };
        }

        if (/^</.test(trimmed)) {
            const messageType = /ClinicalDocument/i.test(trimmed) ? 'CDA' : 'XML';
            return { contentType: 'application/xml', messageType };
        }

        try {
            const parsed = JSON.parse(trimmed);
            if (parsed && parsed.resourceType) {
                return { contentType: 'application/fhir+json', messageType: parsed.resourceType };
            }
            return { contentType: 'application/json', messageType: 'JSON' };
        } catch (e) {
            // Not JSON either — send as-is.
        }

        return { contentType: 'text/plain', messageType: 'unknown' };
    }

    async sendMessage() {
        const form = document.getElementById('sendMessageForm');
        if (!form.checkValidity()) {
            form.reportValidity();
            return;
        }

        const sourceInterfaceId = document.getElementById('sendInterface').value;
        const messageContent = document.getElementById('sendMessageContent').value;
        const { contentType, messageType } = this.detectMessageFormat(messageContent);

        try {
            // Routes the content through the interface's real connectivity
            // (TCP/MLLP, HTTP, or pipeline inject) — same path a live message takes.
            const response = await fetch(`/api/messages/send/${sourceInterfaceId}`, {
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

                closeSendMessageModal();
                form.reset();
                this.loadMessages(); // Refresh messages
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

        } catch (error) {
            console.error('Failed to send message:', error);
            this.showError('Failed to send message');
        }
    }

    loadSampleMessage(type) {
        const textarea = document.getElementById('sendMessageContent');

        if (type === 'fhir') {
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
                this.showSuccess('Re-run started — watching for completion…');
                // Refresh modal immediately to show 'Processing' state; keep it open
                // so the user can see the final result without hunting for the right row.
                if (this.selectedMessageId === id) {
                    await this.showMessageDetail(id);
                }
                // Poll until status is no longer in-flight, then do a final refresh
                this._pollReprocessStatus(id);
            } else {
                const error = await response.json();
                this.showError(error.error || 'Failed to reprocess message');
            }
        } catch (error) {
            console.error('Failed to reprocess message:', error);
            this.showError('Failed to reprocess message');
        }
    }

    // Poll the message status every 1.5 s until the pipeline finishes (max 30 s),
    // then refresh the modal and the list so the user sees the actual final state.
    async _pollReprocessStatus(messageId) {
        const IN_FLIGHT = new Set(['received', 'reprocessing', 'processing', 'queued']);
        const MAX_POLLS = 20; // 20 × 1.5 s = 30 s
        let polls = 0;

        const poll = async () => {
            polls++;
            try {
                const r = await fetch(`/api/messages/${messageId}`);
                if (r.ok) {
                    const data = await r.json();
                    const msg = (data.data && data.data.message) || data.data || data;
                    const status = (msg.status || '').toLowerCase();
                    if (!IN_FLIGHT.has(status) || polls >= MAX_POLLS) {
                        // Pipeline finished — refresh modal (only if still open) and list
                        const modalOpen = document.getElementById('messageDetailModal')?.classList.contains('show');
                        if (modalOpen && this.selectedMessageId === messageId) {
                            await this.showMessageDetail(messageId);
                        }
                        this.loadMessages();
                        return;
                    }
                }
            } catch (_) { /* ignore, keep polling */ }
            setTimeout(poll, 1500);
        };

        // First check after 1.5 s (pipeline needs at least that long to finish)
        setTimeout(poll, 1500);
    }

    async confirmDeleteMessage(messageId) {
        const ok = await AppDialogs.confirm('Are you sure you want to delete this message? This action cannot be undone.', { title: 'Delete Message', type: 'danger', confirmText: 'Delete' });
        if (ok) this.deleteMessage(messageId);
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
    event.target.closest('.tab-btn').classList.add('active');

    // Load tab-specific content
    if (tabName === 'journey' && messageManager.selectedMessageId) {
        messageManager.loadDataLineage(messageManager.selectedMessageId);
    } else if (tabName === 'content' && messageManager.selectedMessageId) {
        messageManager.loadMessageContent(messageManager.selectedMessageId);
    } else if (tabName === 'logs' && messageManager.selectedMessageId) {
        messageManager.loadLogs(messageManager.selectedMessageId);
    } else if (tabName === 'mappingLog' && messageManager.selectedMessageId) {
        messageManager.loadMappingLog(messageManager.selectedMessageId);
    }
}

// Initialize when page loads
let messageManager;
document.addEventListener('DOMContentLoaded', () => {
    messageManager = new MessageManager();
});