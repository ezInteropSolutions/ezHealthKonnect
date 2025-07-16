// Ultra Compact Interfaces Management
let interfaces = [];
let filteredInterfaces = [];
let currentUser = null;

// Pagination state
let currentPage = 1;
let pageSize = 25;
let totalPages = 1;

// Auto-refresh state
let autoRefreshInterval = null;
let autoRefreshEnabled = true;
let refreshRate = 30000; // 30 seconds default
let isUserInteracting = false;
let interactionTimeout = null;

// Initialize on page load
window.addEventListener('load', function() {
    initializeInterfacesPage();
});

// Initialize the interfaces page
async function initializeInterfacesPage() {
    await loadUserInfo();
    updateTime();
    setInterval(updateTime, 60000);
    
    await loadInterfaces();
    setupEventListeners();
    setupAutoRefresh();
}

// Load user info
async function loadUserInfo() {
    try {
        const response = await fetch('/api/user-info');
        if (response.ok) {
            const user = await response.json();
            currentUser = user;
            
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            document.getElementById('userName').textContent = firstName;
            document.getElementById('userRole').textContent = (user.role || 'USER').toUpperCase();
            document.getElementById('userAvatar').textContent = firstName.charAt(0).toUpperCase();
            
            if (user.role === 'admin') {
                document.getElementById('adminSection').style.display = 'block';
            }
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        }
    } catch (error) {
        console.error('Error loading user info:', error);
        window.location.href = 'login.html';
    }
}

// Update time
function updateTime() {
    const now = new Date();
    const timeString = now.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    document.getElementById('currentTime').textContent = timeString;
}

// Logout function
async function logout() {
    if (confirm('Are you sure you want to logout?')) {
        try {
            await fetch('/api/logout', { method: 'POST' });
        } catch (error) {
            console.error('Logout error:', error);
        } finally {
            window.location.href = 'login.html';
        }
    }
}

// Set up event listeners
function setupEventListeners() {
    // Filter event listeners
    document.getElementById('statusFilter').addEventListener('change', applyFilters);
    document.getElementById('typeFilter').addEventListener('change', applyFilters);
    
    // Page size selector
    document.getElementById('pageSize').addEventListener('change', handlePageSizeChange);
    
    // Form submission
    document.getElementById('createInterfaceForm').addEventListener('submit', handleCreateInterface);
    
    // User interaction detection (pause auto-refresh when user is active)
    const interactionEvents = ['click', 'keydown', 'scroll', 'mousemove'];
    interactionEvents.forEach(event => {
        document.addEventListener(event, handleUserInteraction, { passive: true });
    });
    
    // Page visibility API (pause when tab not active)
    document.addEventListener('visibilitychange', handleVisibilityChange);
    
    // Sidebar toggle
    document.getElementById('sidebarToggle').addEventListener('click', function() {
        const sidebar = document.getElementById('sidebar');
        const toggleIcon = document.querySelector('.toggle-icon');
        
        let sidebarCollapsed = sidebar.classList.contains('collapsed');
        sidebarCollapsed = !sidebarCollapsed;
        
        if (sidebarCollapsed) {
            sidebar.classList.add('collapsed');
            toggleIcon.textContent = '›';
        } else {
            sidebar.classList.remove('collapsed');
            toggleIcon.textContent = '‹';
        }
    });
}

// Setup smart auto-refresh system
function setupAutoRefresh() {
    // Add auto-refresh indicator to page
    addAutoRefreshIndicator();
    
    // Determine optimal refresh rate based on interface states
    updateRefreshRate();
    
    // Start auto-refresh
    startAutoRefresh();
    
    console.log(`🔄 Auto-refresh enabled: ${refreshRate / 1000}s interval`);
}

// Add visual indicator for auto-refresh
function addAutoRefreshIndicator() {
    const header = document.querySelector('.header-actions');
    const indicator = document.createElement('div');
    indicator.id = 'autoRefreshIndicator';
    indicator.style.cssText = `
        display: flex; align-items: center; gap: 4px; font-size: 10px; 
        color: #64748b; padding: 4px 8px; border: 1px solid #f8bbd9; 
        border-radius: 4px; background: white; margin-right: 8px;
    `;
    indicator.innerHTML = `
        <span style="width: 6px; height: 6px; background: #22c55e; border-radius: 50%; animation: pulse 2s infinite;"></span>
        <span>Auto-refresh: ${refreshRate / 1000}s</span>
    `;
    
    // Add CSS animation
    const style = document.createElement('style');
    style.textContent = `
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.3; }
        }
        .refreshing { animation: spin 1s linear infinite; }
        @keyframes spin {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
        }
    `;
    document.head.appendChild(style);
    
    header.insertBefore(indicator, header.firstChild);
}

// Start auto-refresh interval
function startAutoRefresh() {
    if (autoRefreshInterval) clearInterval(autoRefreshInterval);
    
    autoRefreshInterval = setInterval(async () => {
        if (autoRefreshEnabled && !document.hidden && !isUserInteracting) {
            await performAutoRefresh();
        }
    }, refreshRate);
}

// Stop auto-refresh
function stopAutoRefresh() {
    if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        autoRefreshInterval = null;
    }
}

// Handle user interaction (pause auto-refresh temporarily)
function handleUserInteraction() {
    isUserInteracting = true;
    
    // Clear existing timeout
    if (interactionTimeout) clearTimeout(interactionTimeout);
    
    // Resume auto-refresh after 10 seconds of inactivity
    interactionTimeout = setTimeout(() => {
        isUserInteracting = false;
    }, 10000);
}

// Handle page visibility changes
function handleVisibilityChange() {
    if (document.hidden) {
        // Page is hidden, reduce refresh rate
        console.log('🔄 Page hidden, reducing refresh rate');
    } else {
        // Page is visible, restore normal refresh rate
        console.log('🔄 Page visible, restoring refresh rate');
        updateRefreshRate();
        startAutoRefresh();
    }
}

// Perform auto-refresh
async function performAutoRefresh() {
    const indicator = document.getElementById('autoRefreshIndicator');
    const refreshIcon = indicator?.querySelector('span');
    
    try {
        // Visual feedback
        if (refreshIcon) refreshIcon.classList.add('refreshing');
        
        // Refresh data silently (no loading state)
        const response = await fetch('/api/interfaces', {
            method: 'GET',
            headers: { 'Content-Type': 'application/json' }
        });
        
        if (response.ok) {
            const data = await response.json();
            
            // Only update if data actually changed
            const newInterfacesJson = JSON.stringify(data.interfaces);
            const currentInterfacesJson = JSON.stringify(interfaces);
            
            if (newInterfacesJson !== currentInterfacesJson) {
                interfaces = data.interfaces || [];
                filteredInterfaces = [...interfaces];
                
                updateSummaryCards();
                calculatePagination();
                renderInterfacesTable();
                updateRefreshRate(); // Adjust rate based on new states
                
                console.log(`🔄 Auto-refreshed: ${interfaces.length} interfaces`);
            }
        }
    } catch (error) {
        console.log('🔄 Auto-refresh failed, will retry next cycle');
    } finally {
        // Remove visual feedback
        if (refreshIcon) refreshIcon.classList.remove('refreshing');
    }
}

// Update refresh rate based on interface states
function updateRefreshRate() {
    const hasErrors = interfaces.some(i => i.status === 'error');
    const hasRunning = interfaces.some(i => i.status === 'running');
    const totalInterfaces = interfaces.length;
    
    // Determine optimal refresh rate
    if (hasErrors) {
        refreshRate = 15000; // 15 seconds if there are errors
    } else if (hasRunning && totalInterfaces > 5) {
        refreshRate = 30000; // 30 seconds for active systems
    } else if (hasRunning) {
        refreshRate = 45000; // 45 seconds for smaller systems
    } else {
        refreshRate = 60000; // 60 seconds for idle systems
    }
    
    // Update indicator
    const indicator = document.getElementById('autoRefreshIndicator');
    if (indicator) {
        const textSpan = indicator.querySelector('span:last-child');
        if (textSpan) {
            textSpan.textContent = `Auto-refresh: ${refreshRate / 1000}s`;
        }
    }
    
    // Restart with new rate
    if (autoRefreshInterval) {
        startAutoRefresh();
    }
}

// Load interfaces from backend
async function loadInterfaces() {
    try {
        console.log('🔄 Loading interfaces from API...');
        
        const response = await fetch('/api/interfaces', {
            method: 'GET',
            headers: { 'Content-Type': 'application/json' }
        });
        
        console.log('📡 API Response:', response.status);
        
        if (response.ok) {
            const data = await response.json();
            interfaces = data.interfaces || [];
            filteredInterfaces = [...interfaces];
            console.log('✅ Interfaces loaded:', interfaces.length);
            
            updateSummaryCards();
            calculatePagination();
            renderInterfacesTable();
        } else if (response.status === 401) {
            console.log('❌ Unauthorized - redirecting to login');
            window.location.href = 'login.html';
            return;
        } else {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
    } catch (error) {
        console.error('❌ Error loading interfaces:', error);
        console.log('🔄 Falling back to mock data');
        
        // Enhanced mock data with clear distinction between lastUpdated and lastActivity
        const now = new Date();
        interfaces = [
            {
                id: 'INT_001',
                userId: 1,
                name: 'ADT Patient Admissions',
                description: 'Real-time ADT admission messages to FHIR Patient resources',
                sourceType: 'tcp',
                targetType: 'fhir',
                messageType: 'ADT^A01',
                status: 'running',
                statistics: { totalProcessed: 2847, successful: 2831, failed: 16 },
                createdAt: new Date(now - 86400000 * 5).toISOString(), // 5 days ago
                lastUpdated: new Date(now - 3600000).toISOString(), // Interface was updated 1 hour ago
                lastActivity: new Date(now - 120000).toISOString(), // Last message processed 2 min ago
                createdBy: 'admin@ezhealthkonnect.com'
            },
            {
                id: 'INT_002',
                userId: 1,
                name: 'Lab Results Processor',
                description: 'ORU lab results processing with validation and alerts',
                sourceType: 'file',
                targetType: 'database',
                messageType: 'ORU^R01',
                status: 'stopped',
                statistics: { totalProcessed: 1156, successful: 1152, failed: 4 },
                createdAt: new Date(now - 86400000 * 3).toISOString(), // 3 days ago
                lastUpdated: new Date(now - 7200000).toISOString(), // Interface updated 2 hours ago
                lastActivity: new Date(now - 3600000).toISOString(), // Last message 1 hour ago
                createdBy: 'admin@ezhealthkonnect.com'
            },
            {
                id: 'INT_003',
                userId: 1,
                name: 'CCD Document Converter',
                description: 'Convert C-CDA documents to FHIR Bundle with validation',
                sourceType: 'http',
                targetType: 'fhir',
                messageType: 'CCD',
                status: 'paused',
                statistics: { totalProcessed: 89, successful: 89, failed: 0 },
                createdAt: new Date(now - 86400000 * 2).toISOString(), // 2 days ago
                lastUpdated: new Date(now - 1800000).toISOString(), // Interface updated 30 min ago
                lastActivity: new Date(now - 1800000).toISOString(), // Last message 30 min ago
                createdBy: 'admin@ezhealthkonnect.com'
            },
            {
                id: 'INT_004',
                userId: 1,
                name: 'Prescription Orders',
                description: 'ORM prescription order processing with drug interaction checks',
                sourceType: 'tcp',
                targetType: 'database',
                messageType: 'ORM^O01',
                status: 'error',
                statistics: { totalProcessed: 245, successful: 198, failed: 47 },
                createdAt: new Date(now - 86400000).toISOString(), // 1 day ago
                lastUpdated: new Date(now - 900000).toISOString(), // Interface updated 15 min ago
                lastActivity: new Date(now - 900000).toISOString(), // Last message 15 min ago
                createdBy: 'admin@ezhealthkonnect.com'
            }
        ];
        filteredInterfaces = [...interfaces];
        
        updateSummaryCards();
        calculatePagination();
        renderInterfacesTable();
        showError('Using demo data (API not available)');
    }
}

// Refresh interfaces (now toggles auto-refresh)
async function refreshInterfaces() {
    const refreshBtn = document.querySelector('.header-btn[onclick="refreshInterfaces()"]');
    const originalContent = refreshBtn.innerHTML;
    
    if (autoRefreshEnabled) {
        // Disable auto-refresh
        autoRefreshEnabled = false;
        stopAutoRefresh();
        refreshBtn.innerHTML = '<span class="btn-icon">⏸</span><span class="btn-label">Auto-refresh Off</span>';
        refreshBtn.style.background = '#fef3c7';
        refreshBtn.style.color = '#d97706';
        refreshBtn.style.borderColor = '#fed7aa';
        
        // Update indicator
        const indicator = document.getElementById('autoRefreshIndicator');
        if (indicator) {
            indicator.style.background = '#fef3c7';
            indicator.querySelector('span:last-child').textContent = 'Auto-refresh: OFF';
            indicator.querySelector('span:first-child').style.background = '#f59e0b';
        }
        
        showSuccess('Auto-refresh disabled');
    } else {
        // Enable auto-refresh
        autoRefreshEnabled = true;
        refreshBtn.innerHTML = '<span class="btn-icon">🔄</span><span class="btn-label">Auto-refresh On</span>';
        refreshBtn.style.background = '';
        refreshBtn.style.color = '';
        refreshBtn.style.borderColor = '';
        
        // Update indicator  
        const indicator = document.getElementById('autoRefreshIndicator');
        if (indicator) {
            indicator.style.background = 'white';
            indicator.querySelector('span:last-child').textContent = `Auto-refresh: ${refreshRate / 1000}s`;
            indicator.querySelector('span:first-child').style.background = '#22c55e';
        }
        
        // Perform immediate refresh then start auto-refresh
        refreshBtn.innerHTML = '<span class="btn-icon">⏳</span><span class="btn-label">Refreshing...</span>';
        refreshBtn.disabled = true;
        
        try {
            await loadInterfaces();
            setupAutoRefresh();
            showSuccess('Auto-refresh enabled');
        } finally {
            refreshBtn.innerHTML = '<span class="btn-icon">🔄</span><span class="btn-label">Auto-refresh On</span>';
            refreshBtn.disabled = false;
        }
    }
}

// Update summary cards
function updateSummaryCards() {
    const total = interfaces.length;
    const running = interfaces.filter(i => i.status === 'running').length;
    const stopped = interfaces.filter(i => i.status === 'stopped').length;
    const paused = interfaces.filter(i => i.status === 'paused').length;
    
    document.getElementById('totalInterfaces').textContent = total;
    document.getElementById('runningInterfaces').textContent = running;
    document.getElementById('stoppedInterfaces').textContent = stopped;
    document.getElementById('pausedInterfaces').textContent = paused;
}

// Calculate pagination
function calculatePagination() {
    totalPages = Math.ceil(filteredInterfaces.length / pageSize);
    if (currentPage > totalPages) currentPage = 1;
    if (totalPages === 0) totalPages = 1;
}

// Render ultra compact interfaces table
function renderInterfacesTable() {
    const tbody = document.getElementById('interfacesTableBody');
    
    if (filteredInterfaces.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="6" class="empty-state">
                    <div class="empty-icon">🔗</div>
                    <div><strong>No interfaces found</strong></div>
                    <div>Create your first HL7 processing interface</div>
                </td>
            </tr>
        `;
        updatePaginationInfo();
        return;
    }
    
    // Get items for current page
    const startIndex = (currentPage - 1) * pageSize;
    const endIndex = startIndex + pageSize;
    const pageItems = filteredInterfaces.slice(startIndex, endIndex);
    
    tbody.innerHTML = pageItems.map(interface => createCompactTableRow(interface)).join('');
    updatePaginationInfo();
    updatePaginationControls();
}

// Create ultra compact table row
function createCompactTableRow(interface) {
    const lastUpdated = interface.lastUpdated 
        ? formatCompactTime(new Date(interface.lastUpdated))
        : { time: 'Never', date: '' };
    
    const lastActivity = interface.lastActivity 
        ? formatCompactTime(new Date(interface.lastActivity))
        : { time: 'Never', date: '' };
    
    const successRate = interface.statistics.totalProcessed > 0 
        ? ((interface.statistics.successful / interface.statistics.totalProcessed) * 100).toFixed(1)
        : '0';
    
    const hasErrors = interface.statistics.failed > 0;
    const errorClass = hasErrors ? 'has-errors' : '';
    
    return `
        <tr onclick="showInterfaceDetails('${interface.id}')">
            <td>
                <div class="interface-name-cell">
                    <div class="interface-name">${interface.name}</div>
                    <div class="interface-description">${interface.description}</div>
                </div>
            </td>
            <td>
                <span class="interface-status status-${interface.status}">${interface.status}</span>
            </td>
            <td>
                <div class="date-cell">
                    <div class="date-time last-updated">${lastUpdated.time}</div>
                    <div class="date-day">${lastUpdated.date}</div>
                </div>
            </td>
            <td>
                <div class="stats-cell">
                    <div class="stat-line">
                        <span class="stat-label">Total:</span>
                        <span class="stat-value">${formatCompactNumber(interface.statistics.totalProcessed)}</span>
                    </div>
                    <div class="stat-line">
                        <span class="stat-label">Success:</span>
                        <span class="success-rate">${successRate}%</span>
                    </div>
                    ${hasErrors ? `
                    <div class="stat-line">
                        <span class="stat-label">Errors:</span>
                        <span class="error-indicator ${errorClass}">${interface.statistics.failed}</span>
                    </div>
                    ` : ''}
                </div>
            </td>
            <td>
                <div class="date-cell">
                    <div class="date-time last-activity">${lastActivity.time}</div>
                    <div class="date-day">${lastActivity.date}</div>
                </div>
            </td>
            <td onclick="event.stopPropagation()">
                <div class="action-buttons">
                    ${getMiniActionButtons(interface)}
                </div>
            </td>
        </tr>
    `;
}

// Get mini icon-only action buttons
function getMiniActionButtons(interface) {
    const buttons = [];
    
    // Edit button always available (except when running)
    if (interface.status !== 'running') {
        buttons.push(`<button class="action-btn edit" data-tooltip="Edit" onclick="showEditModal('${interface.id}')">✏</button>`);
    }
    
    switch (interface.status) {
        case 'stopped':
            buttons.push(`<button class="action-btn start" data-tooltip="Start" onclick="startInterface('${interface.id}')">▶</button>`);
            buttons.push(`<button class="action-btn delete" data-tooltip="Delete" onclick="deleteInterface('${interface.id}')">✕</button>`);
            break;
            
        case 'running':
            buttons.push(`<button class="action-btn stop" data-tooltip="Stop" onclick="stopInterface('${interface.id}')">⏹</button>`);
            buttons.push(`<button class="action-btn pause" data-tooltip="Pause" onclick="pauseInterface('${interface.id}')">⏸</button>`);
            break;
            
        case 'paused':
            buttons.push(`<button class="action-btn start" data-tooltip="Resume" onclick="startInterface('${interface.id}')">▶</button>`);
            buttons.push(`<button class="action-btn stop" data-tooltip="Stop" onclick="stopInterface('${interface.id}')">⏹</button>`);
            break;
            
        case 'error':
            buttons.push(`<button class="action-btn start" data-tooltip="Restart" onclick="startInterface('${interface.id}')">↻</button>`);
            buttons.push(`<button class="action-btn stop" data-tooltip="Stop" onclick="stopInterface('${interface.id}')">⏹</button>`);
            break;
    }
    
    buttons.push(`<button class="action-btn details" data-tooltip="Details" onclick="showInterfaceDetails('${interface.id}')">⋯</button>`);
    
    return buttons.join('');
}

// Format time in ultra compact format
function formatCompactTime(date) {
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    
    let timeText;
    if (diffMins < 1) timeText = 'Now';
    else if (diffMins < 60) timeText = `${diffMins}m`;
    else if (diffHours < 24) timeText = `${diffHours}h`;
    else if (diffDays < 7) timeText = `${diffDays}d`;
    else timeText = `${Math.floor(diffDays / 7)}w`;
    
    return {
        time: timeText,
        date: date.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
    };
}

// Format numbers in compact format
function formatCompactNumber(num) {
    if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M';
    if (num >= 1000) return (num / 1000).toFixed(1) + 'K';
    return num.toString();
}

// Apply filters
function applyFilters() {
    const statusFilter = document.getElementById('statusFilter').value;
    const typeFilter = document.getElementById('typeFilter').value;
    
    filteredInterfaces = interfaces.filter(interface => {
        const statusMatch = statusFilter === 'all' || interface.status === statusFilter;
        const typeMatch = typeFilter === 'all' || interface.sourceType === typeFilter;
        return statusMatch && typeMatch;
    });
    
    currentPage = 1;
    calculatePagination();
    renderInterfacesTable();
}

// Pagination functions
function handlePageSizeChange() {
    pageSize = parseInt(document.getElementById('pageSize').value);
    currentPage = 1;
    calculatePagination();
    renderInterfacesTable();
}

function goToPage(page) {
    if (page >= 1 && page <= totalPages) {
        currentPage = page;
        renderInterfacesTable();
    }
}

function goToPreviousPage() {
    if (currentPage > 1) {
        currentPage--;
        renderInterfacesTable();
    }
}

function goToNextPage() {
    if (currentPage < totalPages) {
        currentPage++;
        renderInterfacesTable();
    }
}

// Update pagination info and controls
function updatePaginationInfo() {
    const startItem = (currentPage - 1) * pageSize + 1;
    const endItem = Math.min(currentPage * pageSize, filteredInterfaces.length);
    const totalItems = filteredInterfaces.length;
    
    const infoElement = document.querySelector('.pagination-info');
    if (infoElement) {
        infoElement.textContent = `${startItem}-${endItem} of ${totalItems}`;
    }
}

function updatePaginationControls() {
    const paginationContainer = document.querySelector('.pagination-controls');
    if (!paginationContainer) return;
    
    let html = '';
    
    // Previous button
    html += `<button class="pagination-btn" onclick="goToPreviousPage()" ${currentPage === 1 ? 'disabled' : ''}>‹</button>`;
    
    // Page numbers (max 3 visible)
    const maxVisiblePages = 3;
    let startPage = Math.max(1, currentPage - 1);
    let endPage = Math.min(totalPages, startPage + maxVisiblePages - 1);
    
    if (endPage - startPage < maxVisiblePages - 1) {
        startPage = Math.max(1, endPage - maxVisiblePages + 1);
    }
    
    for (let i = startPage; i <= endPage; i++) {
        html += `<button class="pagination-btn ${i === currentPage ? 'active' : ''}" onclick="goToPage(${i})">${i}</button>`;
    }
    
    // Next button
    html += `<button class="pagination-btn" onclick="goToNextPage()" ${currentPage === totalPages ? 'disabled' : ''}>›</button>`;
    
    paginationContainer.innerHTML = html;
}

// Interface Actions
async function startInterface(interfaceId) {
    try {
        const response = await fetch(`/api/interfaces/${interfaceId}/start`, { method: 'POST' });
        
        if (response.ok) {
            const data = await response.json();
            showSuccess(data.message);
            await loadInterfaces();
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else {
            throw new Error('Failed to start interface');
        }
    } catch (error) {
        console.error('Error starting interface:', error);
        showError('Failed to start interface');
    }
}

async function stopInterface(interfaceId) {
    try {
        const response = await fetch(`/api/interfaces/${interfaceId}/stop`, { method: 'POST' });
        
        if (response.ok) {
            const data = await response.json();
            showSuccess(data.message);
            await loadInterfaces();
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else {
            throw new Error('Failed to stop interface');
        }
    } catch (error) {
        console.error('Error stopping interface:', error);
        showError('Failed to stop interface');
    }
}

async function pauseInterface(interfaceId) {
    try {
        const response = await fetch(`/api/interfaces/${interfaceId}/pause`, { method: 'POST' });
        
        if (response.ok) {
            const data = await response.json();
            showSuccess(data.message);
            await loadInterfaces();
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else {
            throw new Error('Failed to pause interface');
        }
    } catch (error) {
        console.error('Error pausing interface:', error);
        showError('Failed to pause interface');
    }
}

async function deleteInterface(interfaceId) {
    const interfaceName = interfaces.find(i => i.id === interfaceId)?.name || 'this interface';
    
    if (!confirm(`Delete "${interfaceName}"?`)) {
        return;
    }
    
    try {
        const response = await fetch(`/api/interfaces/${interfaceId}`, { method: 'DELETE' });
        
        if (response.ok) {
            const data = await response.json();
            showSuccess(data.message);
            await loadInterfaces();
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else {
            throw new Error('Failed to delete interface');
        }
    } catch (error) {
        console.error('Error deleting interface:', error);
        showError('Failed to delete interface');
    }
}

// Modal Functions
function showCreateModal() {
    document.getElementById('createModal').classList.add('show');
    document.getElementById('interfaceName').focus();
}

function closeCreateModal() {
    document.getElementById('createModal').classList.remove('show');
    document.getElementById('createInterfaceForm').reset();
}

function showEditModal(interfaceId) {
    const interface = interfaces.find(i => i.id === interfaceId);
    if (!interface) return;
    
    // Populate form with existing data
    document.getElementById('editInterfaceName').value = interface.name;
    document.getElementById('editInterfaceDescription').value = interface.description || '';
    document.getElementById('editSourceType').value = interface.sourceType;
    document.getElementById('editTargetType').value = interface.targetType;
    document.getElementById('editMessageType').value = interface.messageType;
    
    // Store interface ID for submission
    document.getElementById('editInterfaceForm').dataset.interfaceId = interfaceId;
    
    document.getElementById('editModal').classList.add('show');
    document.getElementById('editInterfaceName').focus();
}

function closeEditModal() {
    document.getElementById('editModal').classList.remove('show');
    document.getElementById('editInterfaceForm').reset();
    delete document.getElementById('editInterfaceForm').dataset.interfaceId;
}

function showInterfaceDetails(interfaceId) {
    const interface = interfaces.find(i => i.id === interfaceId);
    if (!interface) return;
    
    document.getElementById('detailsTitle').textContent = interface.name;
    document.getElementById('detailsContent').innerHTML = createDetailsContent(interface);
    document.getElementById('detailsModal').classList.add('show');
}

function closeDetailsModal() {
    document.getElementById('detailsModal').classList.remove('show');
}

// Create detailed content for interface
function createDetailsContent(interface) {
    const successRate = interface.statistics.totalProcessed > 0 
        ? ((interface.statistics.successful / interface.statistics.totalProcessed) * 100).toFixed(1)
        : '0';
    
    return `
        <div class="details-grid">
            <div class="details-section">
                <h4>Interface Information</h4>
                <div class="details-table">
                    <div class="detail-row">
                        <span class="detail-label">Name</span>
                        <span class="detail-value">${interface.name}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Description</span>
                        <span class="detail-value">${interface.description}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Status</span>
                        <span class="detail-value">
                            <span class="interface-status status-${interface.status}">${interface.status}</span>
                        </span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Message Type</span>
                        <span class="detail-value">${interface.messageType}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Source Type</span>
                        <span class="detail-value">${interface.sourceType.toUpperCase()}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Target Type</span>
                        <span class="detail-value">${interface.targetType.toUpperCase()}</span>
                    </div>
                </div>
            </div>
            
            <div class="details-section">
                <h4>Timeline</h4>
                <div class="details-table">
                    <div class="detail-row">
                        <span class="detail-label">Created</span>
                        <span class="detail-value">${new Date(interface.createdAt).toLocaleString()}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Last Updated</span>
                        <span class="detail-value">${interface.lastUpdated ? new Date(interface.lastUpdated).toLocaleString() : 'Never'}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Last Activity</span>
                        <span class="detail-value">${interface.lastActivity ? new Date(interface.lastActivity).toLocaleString() : 'Never'}</span>
                    </div>
                    <div class="detail-row">
                        <span class="detail-label">Created By</span>
                        <span class="detail-value">${interface.createdBy}</span>
                    </div>
                </div>
            </div>
            
            <div class="details-section">
                <h4>Statistics</h4>
                <div class="stats-grid">
                    <div class="stat-card">
                        <div class="stat-number">${interface.statistics.totalProcessed.toLocaleString()}</div>
                        <div class="stat-label">Total</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${interface.statistics.successful.toLocaleString()}</div>
                        <div class="stat-label">Success</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${interface.statistics.failed.toLocaleString()}</div>
                        <div class="stat-label">Failed</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${successRate}%</div>
                        <div class="stat-label">Success Rate</div>
                    </div>
                </div>
            </div>
        </div>
        
        <style>
            .details-grid { display: grid; gap: 16px; }
            .details-section h4 { margin-bottom: 10px; color: #1e3a8a; font-weight: 600; font-size: 13px; }
            .details-table { display: flex; flex-direction: column; gap: 8px; }
            .detail-row { display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid #f8bbd9; }
            .detail-label { color: #64748b; font-weight: 500; font-size: 10px; }
            .detail-value { color: #1e3a8a; font-weight: 600; text-align: right; font-size: 10px; }
            .stats-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px; }
            .stat-card { background: white; padding: 8px; border-radius: 4px; text-align: center; border: 1px solid #f8bbd9; }
            .stat-card .stat-number { font-size: 16px; font-weight: 700; color: #1e3a8a; }
            .stat-card .stat-label { font-size: 9px; color: #64748b; margin-top: 2px; }
        </style>
    `;
}

// Handle create interface form
async function handleCreateInterface(event) {
    event.preventDefault();
    
    const formData = new FormData(event.target);
    const interfaceData = {
        name: formData.get('name'),
        description: formData.get('description'),
        sourceType: formData.get('sourceType'),
        targetType: formData.get('targetType'),
        messageType: formData.get('messageType'),
        sourceConfig: {},
        targetConfig: {},
        processingRules: {}
    };
    
    try {
        const response = await fetch('/api/interfaces', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(interfaceData)
        });
        
        if (response.ok) {
            const data = await response.json();
            showSuccess('Interface created!');
            closeCreateModal();
            await loadInterfaces();
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else {
            const error = await response.json();
            throw new Error(error.error || 'Failed to create interface');
        }
    } catch (error) {
        console.error('Error creating interface:', error);
        showError(error.message || 'Failed to create interface');
    }
}

// Compact Utility Functions
function showSuccess(message) {
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed; top: 15px; right: 15px; z-index: 2000;
        background: white; color: #166534; padding: 8px 12px; border-radius: 4px; 
        border: 2px solid #bbf7d0; font-weight: 600; 
        box-shadow: 0 3px 10px rgba(0,0,0,0.1); font-size: 11px;
    `;
    notification.textContent = message;
    document.body.appendChild(notification);
    
    setTimeout(() => notification.remove(), 2500);
}

function showError(message) {
    const notification = document.createElement('div');
    notification.style.cssText = `
        position: fixed; top: 15px; right: 15px; z-index: 2000;
        background: white; color: #dc2626; padding: 8px 12px; border-radius: 4px; 
        border: 2px solid #fecaca; font-weight: 600; 
        box-shadow: 0 3px 10px rgba(0,0,0,0.1); font-size: 11px;
    `;
    notification.textContent = message;
    document.body.appendChild(notification);
    
    setTimeout(() => notification.remove(), 4000);
}

// Close modals when clicking outside
document.addEventListener('click', function(event) {
    if (event.target.classList.contains('modal-overlay')) {
        if (event.target.id === 'createModal') {
            closeCreateModal();
        } else if (event.target.id === 'detailsModal') {
            closeDetailsModal();
        }
    }
});

// Close modals with Escape key
document.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closeCreateModal();
        closeDetailsModal();
    }
});

// Cleanup auto-refresh when page unloads
window.addEventListener('beforeunload', function() {
    stopAutoRefresh();
});