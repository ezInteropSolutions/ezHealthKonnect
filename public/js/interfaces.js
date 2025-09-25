// Ultra Compact Interfaces Management - FIXED VERSION
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
    setupRuntimeMonitoring();

    // FIX: Add tooltip setup
    setupTooltips();

    // DEBUG: Check modal containers
    debugModalContainers();
}

// REPLACE your setupTooltips function with this JavaScript-based solution:

function setupTooltips() {
    console.log('🔧 Setting up JavaScript tooltips...');
    
    // Tooltip data
    const tooltips = {
        'Dashboard': 'Dashboard - Overview and statistics',
        'Interfaces': 'Interfaces - Manage HL7 integrations', 
        'Messages': 'Messages - View and track messages',
        'Templates': 'Templates - Pre-built configurations',
        'Monitoring': 'Monitoring - System performance',
        'Reports': 'Reports - Analytics and insights',
        'Alerts': 'Alerts - System notifications',
        'Validation': 'Validation - Test and verify',
        'Testing': 'Testing - Interface testing tools',
        'Mapping': 'Mapping - Data transformation',
        'Configuration': 'Configuration - System settings',
        'Audit': 'Audit - Activity logs',
        'Users': 'User Management - Manage users',
        'Settings': 'Settings - System configuration'
    };
    
    // Create tooltip element
    let tooltip = document.getElementById('js-tooltip');
    if (!tooltip) {
        tooltip = document.createElement('div');
        tooltip.id = 'js-tooltip';
        tooltip.style.cssText = `
            position: fixed;
            background: #1e3a8a;
            color: white;
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 500;
            white-space: nowrap;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            border: 1px solid #1e40af;
            z-index: 9999;
            opacity: 0;
            visibility: hidden;
            transition: all 0.2s ease;
            pointer-events: none;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        `;
        document.body.appendChild(tooltip);
    }
    
    // Setup tooltips for nav items
    document.querySelectorAll('.nav-item').forEach(navItem => {
        const label = navItem.querySelector('.nav-label')?.textContent.trim();
        const tooltipText = tooltips[label] || label;
        
        if (label) {
            // Remove any existing listeners
            navItem.removeEventListener('mouseenter', showTooltip);
            navItem.removeEventListener('mouseleave', hideTooltip);
            navItem.removeEventListener('mousemove', moveTooltip);
            
            // Add new listeners
            navItem.addEventListener('mouseenter', function(e) {
                showTooltip(e, tooltipText);
            });
            navItem.addEventListener('mouseleave', hideTooltip);
            navItem.addEventListener('mousemove', moveTooltip);
        }
    });
    
    // Setup logout button tooltip
    const logoutBtn = document.querySelector('.logout-btn');
    if (logoutBtn) {
        logoutBtn.removeEventListener('mouseenter', showTooltip);
        logoutBtn.removeEventListener('mouseleave', hideTooltip);
        logoutBtn.removeEventListener('mousemove', moveTooltip);
        
        logoutBtn.addEventListener('mouseenter', function(e) {
            showTooltip(e, 'Logout');
        });
        logoutBtn.addEventListener('mouseleave', hideTooltip);
        logoutBtn.addEventListener('mousemove', moveTooltip);
    }
    
    function showTooltip(e, text) {
        const sidebar = document.getElementById('sidebar');
        
        // Only show tooltip when sidebar is collapsed
        if (!sidebar || !sidebar.classList.contains('collapsed')) {
            return;
        }
        
        tooltip.textContent = text;
        tooltip.style.opacity = '1';
        tooltip.style.visibility = 'visible';
        
        // Position tooltip
        const rect = e.target.closest('.nav-item, .logout-btn').getBoundingClientRect();
        tooltip.style.left = (rect.right + 12) + 'px';
        tooltip.style.top = (rect.top + rect.height / 2 - tooltip.offsetHeight / 2) + 'px';
    }
    
    function hideTooltip() {
        tooltip.style.opacity = '0';
        tooltip.style.visibility = 'hidden';
    }
    
    function moveTooltip(e) {
        const sidebar = document.getElementById('sidebar');
        if (!sidebar || !sidebar.classList.contains('collapsed')) {
            return;
        }
        
        // Update position on mouse move
        const rect = e.target.closest('.nav-item, .logout-btn').getBoundingClientRect();
        tooltip.style.left = (rect.right + 12) + 'px';
        tooltip.style.top = (rect.top + rect.height / 2 - tooltip.offsetHeight / 2) + 'px';
    }
    
    console.log('✅ JavaScript tooltips set up successfully');
}

// DEBUG: Check modal containers
function debugModalContainers() {
    console.log('🔍 DEBUG: Checking modal containers...');

    const containers = [
        'create-modal-container',
        'edit-modal-container',
        'details-modal-container',
        'wizard-modal-container'
    ];

    containers.forEach(containerId => {
        const container = document.getElementById(containerId);
        console.log(`📋 ${containerId}:`, {
            exists: !!container,
            hasContent: !!container?.innerHTML,
            contentLength: container?.innerHTML?.length || 0
        });
    });

    // Check for actual modals
    const modals = ['createModal', 'editModal', 'detailsModal'];
    modals.forEach(modalId => {
        const modal = document.getElementById(modalId);
        console.log(`🔲 ${modalId}:`, {
            exists: !!modal,
            classes: modal?.className,
            display: modal?.style.display
        });
    });

    console.log('✅ Modal container debug complete');
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

// FIXED: Clean event listeners setup
function setupEventListeners() {
    console.log('🔧 Setting up event listeners...');
    
    // Filter event listeners
    const statusFilter = document.getElementById('statusFilter');
    const typeFilter = document.getElementById('typeFilter');
    const createForm = document.getElementById('createInterfaceForm');
    
    if (statusFilter) {
        statusFilter.addEventListener('change', applyFilters);
        console.log('✅ Status filter listener attached');
    }
    
    if (typeFilter) {
        typeFilter.addEventListener('change', applyFilters);
        console.log('✅ Type filter listener attached');
    }
    
    if (createForm) {
        createForm.addEventListener('submit', handleCreateInterface);
        console.log('✅ Create form listener attached');
    }
    
    // FIX: Single, clean pagination setup
    setupPaginationListener();
    
    // User interaction detection (pause auto-refresh when user is active)
    const interactionEvents = ['click', 'keydown', 'scroll', 'mousemove'];
    interactionEvents.forEach(event => {
        document.addEventListener(event, handleUserInteraction, { passive: true });
    });
    
    // Page visibility API (pause when tab not active)
    document.addEventListener('visibilitychange', handleVisibilityChange);
    
    // Sidebar toggle
    setupSidebarToggle();
    
    console.log('✅ All event listeners set up');
}

// FIX: Dedicated pagination setup function
function setupPaginationListener() {
    const pageSizeSelect = document.getElementById('pageSize');
    
    if (!pageSizeSelect) {
        console.warn('⚠️ Page size select element not found');
        return;
    }
    
    // FIX: Set the current value to show in dropdown
    pageSizeSelect.value = pageSize.toString();
    
    // FIX: Clean event listener setup (remove duplicates)
    const newHandler = function(event) {
        console.log('🔄 Page size change triggered:', event.target.value);
        handlePageSizeChange();
    };
    
    // Remove any existing listeners
    pageSizeSelect.removeEventListener('change', handlePageSizeChange);
    
    // Add new listener
    pageSizeSelect.addEventListener('change', newHandler);
    
    console.log('✅ Pagination dropdown setup complete, current value:', pageSizeSelect.value);
}

// FIX: Enhanced handlePageSizeChange with better error handling
function handlePageSizeChange() {
    const pageSizeSelect = document.getElementById('pageSize');
    
    if (!pageSizeSelect) {
        console.error('❌ Page size select element not found during change');
        return;
    }
    
    const selectedValue = pageSizeSelect.value;
    const newPageSize = parseInt(selectedValue);
    
    console.log('🔍 Page size change:', { selectedValue, newPageSize, isValid: !isNaN(newPageSize) });
    
    if (isNaN(newPageSize) || newPageSize <= 0) {
        console.error('❌ Invalid page size:', selectedValue);
        // Reset to default
        pageSizeSelect.value = '25';
        pageSize = 25;
    } else {
        pageSize = newPageSize;
    }
    
    currentPage = 1;
    calculatePagination();
    renderInterfacesTable();
    
    console.log('✅ Page size updated to:', pageSize);
}

// FIX: Separate sidebar toggle setup
function setupSidebarToggle() {
    const sidebarToggle = document.getElementById('sidebarToggle');
    const sidebar = document.getElementById('sidebar');
    
    if (!sidebarToggle || !sidebar) {
        console.warn('⚠️ Sidebar toggle elements not found');
        return;
    }
    
    sidebarToggle.addEventListener('click', function(e) {
        e.preventDefault();
        e.stopPropagation();
        
        const toggleIcon = document.querySelector('.toggle-icon');
        const isCollapsed = sidebar.classList.contains('collapsed');
        
        if (isCollapsed) {
            sidebar.classList.remove('collapsed');
            if (toggleIcon) toggleIcon.textContent = '‹';
            localStorage.setItem('sidebarCollapsed', 'false');
        } else {
            sidebar.classList.add('collapsed');
            if (toggleIcon) toggleIcon.textContent = '›';
            localStorage.setItem('sidebarCollapsed', 'true');
        }
        
        console.log('🔄 Sidebar toggled:', isCollapsed ? 'expanded' : 'collapsed');
    });
    
    // Restore saved state
    const savedState = localStorage.getItem('sidebarCollapsed');
    if (savedState === 'true') {
        sidebar.classList.add('collapsed');
        const toggleIcon = document.querySelector('.toggle-icon');
        if (toggleIcon) toggleIcon.textContent = '›';
    }
    
    console.log('✅ Sidebar toggle setup complete');
}

// Setup smart auto-refresh system
function setupAutoRefresh() {
    try {
        // Add auto-refresh indicator to page
        addAutoRefreshIndicator();
        
        // Determine optimal refresh rate based on interface states
        updateRefreshRate();
        
        // Start auto-refresh
        startAutoRefresh();
        
        console.log(`🔄 Auto-refresh enabled: ${refreshRate / 1000}s interval`);
    } catch (error) {
        console.error('❌ Error setting up auto-refresh:', error);
        // Continue without auto-refresh rather than breaking the page
        autoRefreshEnabled = false;
    }
}

// FIXED: Add visual indicator for auto-refresh
function addAutoRefreshIndicator() {
    // FIX: Use correct header selector based on your HTML structure
    const headerRight = document.querySelector('.header-right');
    const headerLeft = document.querySelector('.header-left');
    
    // Try multiple possible locations for the indicator
    let targetContainer = null;
    
    if (headerRight) {
        targetContainer = headerRight;
    } else if (headerLeft) {
        targetContainer = headerLeft;
    } else {
        // Fallback: create container if none exists
        const mainHeader = document.querySelector('.main-header');
        if (mainHeader) {
            const fallbackContainer = document.createElement('div');
            fallbackContainer.className = 'header-actions';
            fallbackContainer.style.cssText = 'display: flex; align-items: center; gap: 8px;';
            mainHeader.appendChild(fallbackContainer);
            targetContainer = fallbackContainer;
        }
    }
    
    // Exit if no suitable container found
    if (!targetContainer) {
        console.warn('⚠️ Could not find suitable container for auto-refresh indicator');
        return;
    }

    // Remove existing indicator if present
    const existingIndicator = document.getElementById('autoRefreshIndicator');
    if (existingIndicator) {
        existingIndicator.remove();
    }

    const indicator = document.createElement('div');
    indicator.id = 'autoRefreshIndicator';
    indicator.className = 'status-tile'; // Use existing status-tile styling
    indicator.style.cssText = `
        display: flex; 
        align-items: center; 
        gap: 6px; 
        font-size: 9px; 
        color: #6b7280; 
        padding: 4px 8px; 
        border: 1px solid #f8bbd9; 
        border-radius: 4px; 
        background: #f9fafb; 
        margin-right: 8px;
        min-width: 90px;
        height: 32px;
    `;
    
    indicator.innerHTML = `
        <span class="status-tile-icon" style="width: 6px; height: 6px; background: #22c55e; border-radius: 50%; animation: pulse 2s infinite;"></span>
        <span class="status-tile-label">Auto: ${refreshRate / 1000}s</span>
    `;
    
    // Add CSS animation if not already present
    if (!document.getElementById('auto-refresh-styles')) {
        const style = document.createElement('style');
        style.id = 'auto-refresh-styles';
        style.textContent = `
            @keyframes pulse {
                0%, 100% { opacity: 1; }
                50% { opacity: 0.3; }
            }
            .refreshing { 
                animation: spin 1s linear infinite; 
            }
            @keyframes spin {
                from { transform: rotate(0deg); }
                to { transform: rotate(360deg); }
            }
        `;
        document.head.appendChild(style);
    }
    
    // Insert the indicator
    try {
        if (targetContainer.className.includes('header-right')) {
            // If adding to header-right, add before the create button
            const createBtn = targetContainer.querySelector('.create-btn');
            if (createBtn) {
                targetContainer.insertBefore(indicator, createBtn);
            } else {
                targetContainer.appendChild(indicator);
            }
        } else {
            // If adding to header-left or other container, append to end
            targetContainer.appendChild(indicator);
        }
        
        console.log('✅ Auto-refresh indicator added successfully');
    } catch (error) {
        console.error('❌ Error adding auto-refresh indicator:', error);
    }
}

// ENHANCED: Update refresh rate with better error handling
function updateRefreshRate() {
    try {
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
        
        // Update indicator safely
        const indicator = document.getElementById('autoRefreshIndicator');
        if (indicator) {
            const textSpan = indicator.querySelector('.status-tile-label');
            if (textSpan) {
                textSpan.textContent = `Auto: ${refreshRate / 1000}s`;
            }
        }
        
        // Restart with new rate
        if (autoRefreshInterval) {
            startAutoRefresh();
        }
    } catch (error) {
        console.error('❌ Error updating refresh rate:', error);
    }
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
                <td colspan="7" class="empty-state">
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
                <div class="runtime-status-cell">
                    <span class="runtime-status" id="runtime-${interface.id}">
                        <span class="status-indicator checking">●</span>
                        <span class="status-text">Checking...</span>
                    </span>
                </div>
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

    // Edit button always available (enhanced with more debug info)
    buttons.push(`<button class="action-btn edit" data-tooltip="Edit Configuration" onclick="console.log('🖱️ Edit button clicked for interface:', '${interface.id}', '${interface.name}'); console.log('🖱️ Button element:', this); showEditModal('${interface.id}')">⚙️</button>`);

    // Runtime Processing Controls
    buttons.push(`<button class="action-btn runtime-activate" data-tooltip="Start Processing" onclick="activateInterfaceProcessing('${interface.id}')" id="activate-${interface.id}">▶️</button>`);
    buttons.push(`<button class="action-btn runtime-deactivate" data-tooltip="Stop Processing" onclick="deactivateInterfaceProcessing('${interface.id}')" id="deactivate-${interface.id}" style="display:none">⏹️</button>`);

    // Traditional Interface Controls (if needed)
    switch (interface.status) {
        case 'stopped':
            buttons.push(`<button class="action-btn delete" data-tooltip="Delete Interface" onclick="deleteInterface('${interface.id}')">🗑️</button>`);
            break;

        case 'error':
            buttons.push(`<button class="action-btn restart" data-tooltip="Reset Interface" onclick="resetInterface('${interface.id}')">🔄</button>`);
            break;
    }

    // Messages and monitoring
    buttons.push(`<button class="action-btn messages" data-tooltip="View Messages" onclick="viewInterfaceMessages('${interface.id}')">💬</button>`);
    buttons.push(`<button class="action-btn monitor" data-tooltip="View Processing History" onclick="showProcessingHistory('${interface.id}')">📈</button>`);
    buttons.push(`<button class="action-btn details" data-tooltip="Interface Details" onclick="showInterfaceDetails('${interface.id}')">ℹ️</button>`);

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
    if (!interface) {
        console.error('Interface not found:', interfaceId);
        return;
    }

    console.log('🔧 Opening enhanced edit modal for:', interface.name);

    // Check if modal exists
    const editModal = document.getElementById('editModal');
    if (!editModal) {
        console.error('❌ Edit modal not found! Modal components may not be loaded yet.');

        // Try to reload modal components
        if (typeof loadModalComponents === 'function') {
            console.log('🔄 Attempting to reload modal components...');
            loadModalComponents();
            // Retry after a brief delay
            setTimeout(() => showEditModal(interfaceId), 500);
            return;
        }

        alert('Edit modal not available. Please refresh the page and try again.');
        return;
    }

    // Check if required form elements exist
    const editForm = document.getElementById('editInterfaceForm');
    const editInterfaceId = document.getElementById('editInterfaceId');
    const editInterfaceName = document.getElementById('editInterfaceName');

    if (!editForm || !editInterfaceId || !editInterfaceName) {
        console.error('❌ Edit form elements not found!', {
            form: !!editForm,
            idField: !!editInterfaceId,
            nameField: !!editInterfaceName
        });
        alert('Edit form not properly loaded. Please refresh the page and try again.');
        return;
    }

    // Use the enhanced configuration manager to populate the form
    if (window.interfaceConfigManager) {
        console.log('✅ Using enhanced configuration manager');
        window.interfaceConfigManager.populateEditForm(interface);
    } else {
        console.log('⚠️ Configuration manager not available, using basic population');
        // Basic fallback only for essential fields
        editInterfaceId.value = interface.id;
        editInterfaceName.value = interface.name;
        document.getElementById('editInterfaceDescription').value = interface.description || '';
        document.getElementById('editSourceType').value = interface.sourceType || 'tcp';
        document.getElementById('editTargetType').value = interface.targetType || 'fhir';
    }

    // Store interface ID for submission
    editForm.dataset.interfaceId = interfaceId;

    // Show the modal
    console.log('🔍 Modal before show:', {
        exists: !!editModal,
        classes: editModal?.className,
        display: editModal?.style.display
    });

    // CRITICAL FIX: Clear any inline styles that prevent showing
    editModal.removeAttribute('style');
    editModal.classList.add('show');

    console.log('🔍 Modal after show:', {
        classes: editModal.className,
        display: editModal.style.display,
        visible: window.getComputedStyle(editModal).display
    });

    editInterfaceName.focus();

    console.log('✅ Edit modal opened successfully for:', interface.name);
}

function closeEditModal() {
    console.log('🔄 Attempting to close edit modal...');

    const editModal = document.getElementById('editModal');
    const editForm = document.getElementById('editInterfaceForm');

    console.log('🔍 Modal state before close:', {
        modalExists: !!editModal,
        hasShowClass: editModal?.classList?.contains('show'),
        currentClasses: editModal?.className,
        display: editModal?.style.display
    });

    if (editModal) {
        // SIMPLE FIX: Just remove show class and let CSS handle the rest
        editModal.classList.remove('show');

        // Ensure the modal is immediately hidden without conflicting styles
        editModal.style.display = 'none';

        console.log('🔍 Modal state after close attempt:', {
            hasShowClass: editModal.classList.contains('show'),
            currentClasses: editModal.className,
            display: editModal.style.display
        });
    }

    if (editForm) {
        // Clean reset without interference
        editForm.reset();
        delete editForm.dataset.interfaceId;

        // Clear any dynamic content
        const sourceConfigFields = document.getElementById('sourceConfigFields');
        const fhirServerConfig = document.getElementById('fhirServerConfig');

        if (sourceConfigFields) sourceConfigFields.innerHTML = '';
        if (fhirServerConfig) fhirServerConfig.style.display = 'block';
    }

    console.log('✅ Edit modal close operation completed');
}

// Make closeEditModal globally accessible for debugging
window.forceCloseEditModal = function() {
    console.log('🔧 FORCE CLOSE: Manually closing edit modal...');
    const editModal = document.getElementById('editModal');
    if (editModal) {
        editModal.classList.remove('show');
        editModal.style.display = 'none !important';
        editModal.style.visibility = 'hidden';
        editModal.style.opacity = '0';
        console.log('🔧 FORCE CLOSE: Modal forcibly hidden');
    }
};

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

/**
 * Navigate to messages page filtered by interface
 */
function viewInterfaceMessages(interfaceId) {
    // Navigate to messages page with interface filter
    window.location.href = `messages.html?interfaceId=${interfaceId}`;
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

/**
 * Collect all interface form data including configurations
 */
function collectInterfaceFormData() {
    console.log('📋 Collecting interface form data...');

    // Basic interface information
    const interfaceData = {
        id: document.getElementById('editInterfaceId')?.value,
        name: document.getElementById('editInterfaceName')?.value,
        description: document.getElementById('editInterfaceDescription')?.value,
        sourceType: document.getElementById('editSourceType')?.value,
        targetType: document.getElementById('editTargetType')?.value,
        status: document.getElementById('editStatus')?.value,
        format: document.getElementById('editFormat')?.value
    };

    // Source Configuration
    const sourceConfig = {
        host: document.getElementById('editSourceHost')?.value || 'localhost',
        port: parseInt(document.getElementById('editSourcePort')?.value) || null,
        connectivity: document.getElementById('editSourceConnectivity')?.value || 'inbound'
    };

    // Target Configuration - Enhanced Collection
    const targetConfig = collectTargetConfiguration();

    // Processing Rules and Additional Config
    const processingRules = {
        routingMode: document.getElementById('editRoutingMode')?.value || 'direct',
        targetFhirInterface: document.getElementById('editTargetFhirInterface')?.value,
        transformationEngine: document.getElementById('editTransformationEngine')?.value || 'go-engine',
        retryPolicy: document.getElementById('editRetryPolicy')?.value || '3'
    };

    // Additional fields
    const additionalConfig = {
        sourceConnectivity: document.getElementById('editSourceConnectivity')?.value,
        targetConnectivity: document.getElementById('editTargetConnectivity')?.value,
        tableStrategy: document.getElementById('editTableStrategy')?.value || 'shared',
        expectedVolume: document.getElementById('editExpectedVolume')?.value || 'low'
    };

    const formData = {
        ...interfaceData,
        sourceConfig: sourceConfig,
        targetConfig: targetConfig,
        processingRules: processingRules,
        ...additionalConfig
    };

    console.log('✅ Form data collected:', formData);
    return formData;
}

/**
 * Collect target configuration from form
 */
function collectTargetConfiguration() {
    console.log('🎯 Collecting target configuration...');

    const targetConfig = {
        // Basic FHIR Server Configuration
        host: 'localhost',
        port: 8080,
        protocol: 'http',
        path: '/Patient'
    };

    // Get FHIR Server URL and parse it
    const fhirServerUrl = document.getElementById('editFhirServerUrl')?.value;
    if (fhirServerUrl) {
        try {
            const url = new URL(fhirServerUrl);
            targetConfig.protocol = url.protocol.replace(':', '');
            targetConfig.host = url.hostname;
            targetConfig.port = parseInt(url.port) || (url.protocol === 'https:' ? 443 : 80);
            targetConfig.path = url.pathname.endsWith('/fhir') ?
                url.pathname.replace('/fhir', '') : url.pathname;
        } catch (error) {
            console.warn('⚠️ Invalid FHIR Server URL, using parts:', fhirServerUrl);
            // If URL parsing fails, try to extract meaningful parts
            targetConfig.host = fhirServerUrl.includes('://') ?
                fhirServerUrl.split('://')[1].split(':')[0].split('/')[0] :
                fhirServerUrl.split(':')[0];
        }
    }

    // Override port if specified separately
    const targetPort = document.getElementById('editTargetPort')?.value;
    if (targetPort) {
        targetConfig.port = parseInt(targetPort);
    }

    // Resource endpoint
    const resourceEndpoint = document.getElementById('editResourceEndpoint')?.value;
    if (resourceEndpoint) {
        targetConfig.path = resourceEndpoint.startsWith('/') ? resourceEndpoint : `/${resourceEndpoint}`;
    }

    // Target connectivity
    const targetConnectivity = document.getElementById('editTargetConnectivity')?.value;
    if (targetConnectivity) {
        targetConfig.connectivity = targetConnectivity;
    }

    // Routing strategy and endpoints
    const routingStrategy = document.getElementById('editRoutingStrategy')?.value;
    if (routingStrategy) {
        targetConfig.routing_strategy = routingStrategy;
    }

    // Multiple endpoints configuration
    const multipleEndpointsEnabled = document.getElementById('enableMultipleEndpoints')?.checked;
    if (multipleEndpointsEnabled) {
        targetConfig.endpoints = collectMultipleEndpoints();
    } else {
        // Single endpoint configuration
        targetConfig.endpoints = [{
            id: 'primary_endpoint',
            name: 'Primary Endpoint',
            type: document.getElementById('editTargetType')?.value || 'fhir',
            url: fhirServerUrl || `${targetConfig.protocol}://${targetConfig.host}:${targetConfig.port}/fhir`,
            resource_endpoint: resourceEndpoint || 'Patient',
            priority: 1,
            weight: 100,
            enabled: true
        }];
    }

    // Routing rules (if any)
    const routingRules = document.getElementById('editRoutingRules')?.value;
    if (routingRules) {
        try {
            targetConfig.routing_rules = JSON.parse(routingRules);
        } catch (error) {
            console.warn('⚠️ Invalid routing rules JSON:', error.message);
            targetConfig.routing_rules = [];
        }
    }

    console.log('✅ Target configuration collected:', targetConfig);
    return targetConfig;
}

/**
 * Collect multiple endpoints configuration
 */
function collectMultipleEndpoints() {
    const endpoints = [];
    const endpointElements = document.querySelectorAll('.endpoint-item');

    endpointElements.forEach((element, index) => {
        const name = element.querySelector(`input[name="endpointName_${index}"]`)?.value;
        const url = element.querySelector(`input[name="endpointUrl_${index}"]`)?.value;
        const priority = parseInt(element.querySelector(`select[name="endpointPriority_${index}"]`)?.value) || 1;
        const enabled = element.querySelector(`input[name="endpointEnabled_${index}"]`)?.checked;

        if (name && url) {
            endpoints.push({
                id: `endpoint_${index}`,
                name: name,
                type: 'fhir',
                url: url,
                priority: priority,
                weight: 100 / (priority || 1), // Higher priority = higher weight
                enabled: enabled !== false
            });
        }
    });

    return endpoints;
}

// Enhanced edit interface handler
async function handleEditInterface(event) {
    event.preventDefault();

    // Prevent multiple simultaneous submissions
    if (handleEditInterface.isProcessing) {
        console.log('⚠️ Edit submission already in progress, ignoring duplicate');
        return;
    }

    handleEditInterface.isProcessing = true;
    console.log('🔄 Handling interface edit submission');

    // Provide immediate visual feedback
    const saveButton = document.querySelector('#editModal .modal-btn.primary');
    const originalText = saveButton?.textContent;
    if (saveButton) {
        saveButton.textContent = '💾 Saving...';
        saveButton.disabled = true;
    }

    let interfaceData;

    // Use enhanced config manager if available, otherwise fallback to basic collection
    if (window.interfaceConfigManager) {
        console.log('✅ Using enhanced configuration manager for data collection');
        interfaceData = window.interfaceConfigManager.collectFormData();

        // Validate configuration
        const errors = window.interfaceConfigManager.validateConfiguration(interfaceData);
        if (errors.length > 0) {
            alert('Configuration errors:\n' + errors.join('\n'));
            handleEditInterface.isProcessing = false; // Reset flag on validation error

            // Reset button state
            if (saveButton) {
                saveButton.textContent = originalText || '💾 Save Changes';
                saveButton.disabled = false;
            }
            return;
        }
    } else {
        console.log('⚠️ Using basic form data collection');
        interfaceData = collectInterfaceFormData();
    }

    console.log('🔄 Submitting interface update:', interfaceData);

    try {
        const response = await fetch(`/api/interfaces/${interfaceData.id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(interfaceData)
        });

        if (response.ok) {
            const result = await response.json();
            console.log('✅ Interface updated successfully:', result);

            showSuccess(`Interface "${interfaceData.name}" updated successfully!`);

            console.log('🔄 About to close modal after successful save...');

            // Ensure modal closes with error handling
            try {
                closeEditModal();
                console.log('🔄 Modal close call completed successfully');
            } catch (closeError) {
                console.error('❌ Error closing modal:', closeError);
                // Force close as fallback
                const editModal = document.getElementById('editModal');
                if (editModal) {
                    editModal.classList.remove('show');
                    editModal.style.display = 'none';
                    console.log('🔧 Modal forcibly closed as fallback');
                }
            }

            // Refresh interfaces list
            setTimeout(async () => {
                await loadInterfaces();
            }, 500);

        } else {
            const error = await response.json();
            console.error('❌ Failed to update interface:', error);

            let errorMessage = 'Failed to update interface: ';
            errorMessage += error.error || error.message || `HTTP ${response.status}`;

            alert(errorMessage);
        }
    } catch (error) {
        console.error('❌ Network error updating interface:', error);
        alert('Network error updating interface: ' + error.message);
    } finally {
        // Reset processing flag and button state
        handleEditInterface.isProcessing = false;

        if (saveButton) {
            saveButton.textContent = originalText || '💾 Save Changes';
            saveButton.disabled = false;
        }
    }
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

// AUTOMATIC TOOLTIP SETUP - Add to your interfaces.js or main JS file
function setupSidebarTooltips() {
    console.log('🔧 Setting up sidebar tooltips...');
    
    // Auto-generate tooltips from existing nav labels
    const navItems = document.querySelectorAll('.nav-item');
    
    navItems.forEach(navItem => {
        const navLabel = navItem.querySelector('.nav-label');
        if (navLabel) {
            const tooltipText = navLabel.textContent.trim();
            navItem.setAttribute('data-tooltip', tooltipText);
        }
    });
    
    // Special tooltips for specific items
    const specialTooltips = {
        '🏠': 'Dashboard - Overview and statistics',
        '🔗': 'Interfaces - Manage HL7 integrations', 
        '📧': 'Messages - View and track messages',
        '📄': 'Templates - Pre-built configurations',
        '📊': 'Monitoring - System performance',
        '📈': 'Reports - Analytics and insights',
        '🔔': 'Alerts - System notifications',
        '✓': 'Validation - Test and verify',
        '🧪': 'Testing - Interface testing tools',
        '🗺️': 'Mapping - Data transformation',
        '⚙️': 'Configuration - System settings',
        '📋': 'Audit - Activity logs',
        '👥': 'User Management - Manage users'
    };
    
    // Apply enhanced tooltips
    navItems.forEach(navItem => {
        const icon = navItem.querySelector('.nav-icon');
        if (icon && specialTooltips[icon.textContent]) {
            navItem.setAttribute('data-tooltip', specialTooltips[icon.textContent]);
        }
    });
    
    console.log(`✅ Tooltips configured for ${navItems.length} navigation items`);
}

// CSS Injection for Tooltips (if you haven't added the CSS to your stylesheet)
function injectTooltipStyles() {
    // Check if styles already exist
    if (document.getElementById('sidebar-tooltip-styles')) return;
    
    const style = document.createElement('style');
    style.id = 'sidebar-tooltip-styles';
    style.textContent = `
        /* Sidebar Collapsed Tooltips */
        .sidebar.collapsed .nav-item {
            position: relative;
        }
        
        .sidebar.collapsed .nav-item::after {
            content: attr(data-tooltip);
            position: absolute;
            left: 100%;
            top: 50%;
            transform: translateY(-50%);
            margin-left: 12px;
            background: #1e3a8a;
            color: white;
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 500;
            white-space: nowrap;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            border: 1px solid #1e40af;
            opacity: 0;
            visibility: hidden;
            pointer-events: none;
            transition: all 0.2s ease;
            z-index: 9999;
        }
        
        .sidebar.collapsed .nav-item::before {
            content: '';
            position: absolute;
            left: 100%;
            top: 50%;
            transform: translateY(-50%);
            margin-left: 6px;
            width: 0;
            height: 0;
            border-top: 6px solid transparent;
            border-bottom: 6px solid transparent;
            border-right: 6px solid #1e3a8a;
            opacity: 0;
            visibility: hidden;
            pointer-events: none;
            transition: all 0.2s ease;
            z-index: 9998;
        }
        
        .sidebar.collapsed .nav-item:hover::after,
        .sidebar.collapsed .nav-item:hover::before {
            opacity: 1;
            visibility: visible;
        }
        
        .sidebar.collapsed .nav-item:hover {
            background: rgba(248, 187, 217, 0.1);
            transform: translateX(2px);
            transition: all 0.2s ease;
        }
        
        .sidebar.collapsed .nav-icon {
            transition: all 0.2s ease;
        }
        
        .sidebar.collapsed .nav-item:hover .nav-icon {
            transform: scale(1.1);
            color: #1e3a8a;
        }
        
        /* Hide on mobile */
        @media (max-width: 768px) {
            .sidebar.collapsed .nav-item::after,
            .sidebar.collapsed .nav-item::before {
                display: none;
            }
        }
        
        /* Logout button tooltip */
        .sidebar.collapsed .logout-btn {
            position: relative;
        }
        
        .sidebar.collapsed .logout-btn::after {
            content: "Logout";
            position: absolute;
            left: 100%;
            top: 50%;
            transform: translateY(-50%);
            margin-left: 12px;
            background: #dc2626;
            color: white;
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 500;
            white-space: nowrap;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
            border: 1px solid #ef4444;
            opacity: 0;
            visibility: hidden;
            pointer-events: none;
            transition: all 0.2s ease;
            z-index: 9999;
        }
        
        .sidebar.collapsed .logout-btn::before {
            content: '';
            position: absolute;
            left: 100%;
            top: 50%;
            transform: translateY(-50%);
            margin-left: 6px;
            width: 0;
            height: 0;
            border-top: 6px solid transparent;
            border-bottom: 6px solid transparent;
            border-right: 6px solid #dc2626;
            opacity: 0;
            visibility: hidden;
            pointer-events: none;
            transition: all 0.2s ease;
            z-index: 9998;
        }
        
        .sidebar.collapsed .logout-btn:hover::after,
        .sidebar.collapsed .logout-btn:hover::before {
            opacity: 1;
            visibility: visible;
        }
    `;
    
    document.head.appendChild(style);
    console.log('✅ Tooltip styles injected');
}

// Initialize tooltips when page loads
function initializeSidebarTooltips() {
    // Wait for DOM to be ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function() {
            injectTooltipStyles();
            setupSidebarTooltips();
        });
    } else {
        injectTooltipStyles();
        setupSidebarTooltips();
    }
}

// Call initialization
initializeSidebarTooltips();

// Re-setup tooltips if content changes dynamically
window.refreshSidebarTooltips = setupSidebarTooltips;

// FIX: Add this after page load to ensure dropdown shows correct value
window.addEventListener('load', function() {
    // Ensure pagination dropdown shows the correct value after everything is loaded
    setTimeout(function() {
        const pageSizeSelect = document.getElementById('pageSize');
        if (pageSizeSelect && pageSizeSelect.value !== pageSize.toString()) {
            pageSizeSelect.value = pageSize.toString();
            console.log('🔄 Corrected pagination dropdown value:', pageSize);
        }
    }, 100);
});

// ============================================================================
// RUNTIME PROCESSING ENGINE INTEGRATION
// ============================================================================

// Runtime monitoring state
let runtimeMonitoringInterval = null;
let processingEngineStatus = 'unknown';

/**
 * Setup runtime monitoring system
 */
function setupRuntimeMonitoring() {
    console.log('🔧 Setting up runtime monitoring...');

    // Check engine status immediately
    checkProcessingEngineStatus();

    // Start periodic monitoring
    runtimeMonitoringInterval = setInterval(() => {
        checkProcessingEngineStatus();
        updateInterfaceRuntimeStatuses();
    }, 10000); // Every 10 seconds

    console.log('✅ Runtime monitoring active');
}

/**
 * Check processing engine status
 */
async function checkProcessingEngineStatus() {
    try {
        const response = await fetch('/api/runtime/engine/status', {
            method: 'GET',
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            processingEngineStatus = data.engine?.isRunning ? 'running' : 'stopped';
            updateEngineStatusDisplay(data);
        } else {
            processingEngineStatus = 'error';
            console.warn('❌ Engine status check failed:', response.status);
            updateEngineStatusDisplay(null);
        }
    } catch (error) {
        processingEngineStatus = 'offline';
        console.warn('❌ Engine status check error:', error.message);
        updateEngineStatusDisplay(null);
    }
}

/**
 * Update engine status display
 */
function updateEngineStatusDisplay(data) {
    const engineLight = document.getElementById('engine-light');
    const engineText = document.getElementById('engine-text');
    const startBtn = document.getElementById('start-engine-btn');
    const stopBtn = document.getElementById('stop-engine-btn');
    const activeInterfacesCount = document.getElementById('active-interfaces-count');
    const messagesTodayCount = document.getElementById('messages-today-count');
    const successRate = document.getElementById('success-rate');

    if (!engineLight || !engineText) return;

    // Update engine status
    switch (processingEngineStatus) {
        case 'running':
            engineLight.className = 'engine-light running';
            engineText.textContent = 'Processing Engine: Running';
            if (startBtn) startBtn.style.display = 'none';
            if (stopBtn) stopBtn.style.display = 'inline-block';
            break;
        case 'stopped':
            engineLight.className = 'engine-light stopped';
            engineText.textContent = 'Processing Engine: Stopped';
            if (startBtn) startBtn.style.display = 'inline-block';
            if (stopBtn) stopBtn.style.display = 'none';
            break;
        case 'error':
            engineLight.className = 'engine-light error';
            engineText.textContent = 'Processing Engine: Error';
            if (startBtn) startBtn.style.display = 'inline-block';
            if (stopBtn) stopBtn.style.display = 'none';
            break;
        default:
            engineLight.className = 'engine-light offline';
            engineText.textContent = 'Processing Engine: Offline';
            if (startBtn) startBtn.style.display = 'inline-block';
            if (stopBtn) stopBtn.style.display = 'none';
    }

    // Update stats if data is available
    if (data && data.engine) {
        const dbStats = data.engine.database || {};
        const todayStats = data.engine.today || {};

        if (activeInterfacesCount) {
            activeInterfacesCount.textContent = dbStats.active_interfaces || '0';
        }
        if (messagesTodayCount) {
            messagesTodayCount.textContent = todayStats.messages_today || '0';
        }
        if (successRate && dbStats.total_messages_processed > 0) {
            const rate = ((dbStats.total_messages_processed / (dbStats.total_messages_processed + (dbStats.total_messages_failed || 0))) * 100).toFixed(1);
            successRate.textContent = rate + '%';
        }
    }
}

/**
 * Start processing engine
 */
async function startProcessingEngine() {
    const startBtn = document.getElementById('start-engine-btn');
    if (startBtn) {
        startBtn.disabled = true;
        startBtn.textContent = '⏳ Starting...';
    }

    try {
        console.log('🚀 Starting processing engine...');

        const response = await fetch('/api/runtime/engine/start', {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        const data = await response.json();

        if (response.ok) {
            console.log('✅ Processing engine started:', data.message);
            showSuccess('Processing engine started successfully');

            // Update status immediately
            await checkProcessingEngineStatus();
        } else {
            console.error('❌ Engine start failed:', data.message);
            showError(data.message || 'Failed to start processing engine');
        }
    } catch (error) {
        console.error('❌ Engine start error:', error);
        showError('Failed to start processing engine: ' + error.message);
    } finally {
        if (startBtn) {
            startBtn.disabled = false;
            startBtn.textContent = '🚀 Start Engine';
        }
    }
}

/**
 * Stop processing engine
 */
async function stopProcessingEngine() {
    const stopBtn = document.getElementById('stop-engine-btn');
    if (stopBtn) {
        stopBtn.disabled = true;
        stopBtn.textContent = '⏳ Stopping...';
    }

    try {
        console.log('⏹️ Stopping processing engine...');

        const response = await fetch('/api/runtime/engine/stop', {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        const data = await response.json();

        if (response.ok) {
            console.log('✅ Processing engine stopped:', data.message);
            showSuccess('Processing engine stopped successfully');

            // Update status immediately
            await checkProcessingEngineStatus();
        } else {
            console.error('❌ Engine stop failed:', data.message);
            showError(data.message || 'Failed to stop processing engine');
        }
    } catch (error) {
        console.error('❌ Engine stop error:', error);
        showError('Failed to stop processing engine: ' + error.message);
    } finally {
        if (stopBtn) {
            stopBtn.disabled = false;
            stopBtn.textContent = '⏹️ Stop Engine';
        }
    }
}

/**
 * Update interface runtime statuses
 */
async function updateInterfaceRuntimeStatuses() {
    for (const interface of interfaces) {
        await updateInterfaceRuntimeStatus(interface.id);
    }
}

/**
 * Update individual interface runtime status
 */
async function updateInterfaceRuntimeStatus(interfaceId) {
    const statusElement = document.getElementById(`runtime-${interfaceId}`);
    if (!statusElement) return;

    try {
        const response = await fetch(`/api/runtime/interfaces/${interfaceId}/status`, {
            method: 'GET',
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            const isProcessing = data.interface?.processingActive || false;
            const stats = data.interface?.processingStats;

            updateRuntimeStatusDisplay(statusElement, isProcessing, stats);
            updateActionButtonsVisibility(interfaceId, isProcessing);
        } else if (response.status === 404) {
            updateRuntimeStatusDisplay(statusElement, false, null, 'not_configured');
        } else {
            updateRuntimeStatusDisplay(statusElement, false, null, 'error');
        }
    } catch (error) {
        updateRuntimeStatusDisplay(statusElement, false, null, 'offline');
    }
}

/**
 * Update runtime status display
 */
function updateRuntimeStatusDisplay(element, isProcessing, stats, errorState = null) {
    const indicator = element.querySelector('.status-indicator');
    const text = element.querySelector('.status-text');

    if (errorState) {
        switch (errorState) {
            case 'offline':
                indicator.className = 'status-indicator offline';
                text.textContent = 'Offline';
                break;
            case 'error':
                indicator.className = 'status-indicator error';
                text.textContent = 'Error';
                break;
            case 'not_configured':
                indicator.className = 'status-indicator stopped';
                text.textContent = 'Not Active';
                break;
        }
    } else if (isProcessing) {
        indicator.className = 'status-indicator active';
        text.textContent = stats ?
            `Processing (${stats.processedCount || 0} msgs)` : 'Processing';
    } else {
        indicator.className = 'status-indicator stopped';
        text.textContent = 'Stopped';
    }
}

/**
 * Update action button visibility
 */
function updateActionButtonsVisibility(interfaceId, isProcessing) {
    const activateBtn = document.getElementById(`activate-${interfaceId}`);
    const deactivateBtn = document.getElementById(`deactivate-${interfaceId}`);

    if (activateBtn && deactivateBtn) {
        if (isProcessing) {
            activateBtn.style.display = 'none';
            deactivateBtn.style.display = 'inline-block';
        } else {
            activateBtn.style.display = 'inline-block';
            deactivateBtn.style.display = 'none';
        }
    }
}

/**
 * Activate interface processing
 */
async function activateInterfaceProcessing(interfaceId) {
    const activateBtn = document.getElementById(`activate-${interfaceId}`);
    if (activateBtn) {
        activateBtn.disabled = true;
        activateBtn.textContent = '⏳';
    }

    try {
        console.log(`🚀 Activating interface processing: ${interfaceId}`);

        const response = await fetch(`/api/runtime/interfaces/${interfaceId}/activate`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        const data = await response.json();

        if (response.ok) {
            console.log('✅ Interface activated:', data.message);
            showSuccess(`Interface activated successfully`);

            // Immediately update status
            await updateInterfaceRuntimeStatus(interfaceId);
        } else {
            console.error('❌ Activation failed:', data.message);
            showError(data.message || 'Failed to activate interface');
        }
    } catch (error) {
        console.error('❌ Activation error:', error);
        showError('Failed to activate interface: ' + error.message);
    } finally {
        if (activateBtn) {
            activateBtn.disabled = false;
            activateBtn.textContent = '🚀';
        }
    }
}

/**
 * Deactivate interface processing
 */
async function deactivateInterfaceProcessing(interfaceId) {
    const deactivateBtn = document.getElementById(`deactivate-${interfaceId}`);
    if (deactivateBtn) {
        deactivateBtn.disabled = true;
        deactivateBtn.textContent = '⏳';
    }

    try {
        console.log(`⏸️ Deactivating interface processing: ${interfaceId}`);

        const response = await fetch(`/api/runtime/interfaces/${interfaceId}/deactivate`, {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                reason: 'manual_stop'
            })
        });

        const data = await response.json();

        if (response.ok) {
            console.log('✅ Interface deactivated:', data.message);
            showSuccess(`Interface processing stopped`);

            // Immediately update status
            await updateInterfaceRuntimeStatus(interfaceId);
        } else {
            console.error('❌ Deactivation failed:', data.message);
            showError(data.message || 'Failed to deactivate interface');
        }
    } catch (error) {
        console.error('❌ Deactivation error:', error);
        showError('Failed to deactivate interface: ' + error.message);
    } finally {
        if (deactivateBtn) {
            deactivateBtn.disabled = false;
            deactivateBtn.textContent = '⏸️';
        }
    }
}

/**
 * Show processing history for interface
 */
async function showProcessingHistory(interfaceId) {
    try {
        const response = await fetch(`/api/runtime/interfaces/${interfaceId}/history?limit=100`, {
            method: 'GET',
            credentials: 'include'
        });

        if (response.ok) {
            const data = await response.json();
            displayProcessingHistoryModal(interfaceId, data.history);
        } else {
            showError('Failed to load processing history');
        }
    } catch (error) {
        console.error('❌ History error:', error);
        showError('Failed to load processing history');
    }
}

/**
 * Display processing history modal
 */
function displayProcessingHistoryModal(interfaceId, history) {
    // Create modal HTML
    const modal = document.createElement('div');
    modal.className = 'modal-overlay';
    modal.innerHTML = `
        <div class="modal-container processing-history-modal">
            <div class="modal-header">
                <h2>Processing History</h2>
                <button class="close-btn" onclick="this.closest('.modal-overlay').remove()">×</button>
            </div>
            <div class="modal-content">
                <div class="history-stats">
                    <div class="stat-card">
                        <div class="stat-number">${history.length}</div>
                        <div class="stat-label">Total Messages</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${history.filter(h => h.status === 'completed').length}</div>
                        <div class="stat-label">Successful</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-number">${history.filter(h => h.status === 'failed').length}</div>
                        <div class="stat-label">Failed</div>
                    </div>
                </div>
                <div class="history-table">
                    <table class="processing-history-table">
                        <thead>
                            <tr>
                                <th>Message ID</th>
                                <th>Status</th>
                                <th>Processing Time</th>
                                <th>Timestamp</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${history.map(h => `
                                <tr class="history-row status-${h.status}">
                                    <td class="message-id">${h.message_id}</td>
                                    <td class="status">
                                        <span class="status-badge ${h.status}">${h.status}</span>
                                    </td>
                                    <td class="processing-time">${h.transformation_time_ms || 0}ms</td>
                                    <td class="timestamp">${new Date(h.created_at).toLocaleString()}</td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    `;

    document.body.appendChild(modal);
}

/**
 * Reset interface (clear error state)
 */
async function resetInterface(interfaceId) {
    try {
        console.log(`↻ Resetting interface: ${interfaceId}`);

        const response = await fetch(`/api/interfaces/${interfaceId}`, {
            method: 'PUT',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                status: 'stopped'
            })
        });

        if (response.ok) {
            console.log('✅ Interface reset successfully');
            showSuccess('Interface reset successfully');
            await loadInterfaces(); // Refresh the list
        } else {
            showError('Failed to reset interface');
        }
    } catch (error) {
        console.error('❌ Reset interface error:', error);
        showError('Failed to reset interface: ' + error.message);
    }
}

// Cleanup on page unload
window.addEventListener('beforeunload', function() {
    if (runtimeMonitoringInterval) {
        clearInterval(runtimeMonitoringInterval);
    }
});