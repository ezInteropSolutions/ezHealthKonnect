// Simple dashboard manager - matching your backend API
let sidebarCollapsed = false;

// Sidebar toggle functionality
document.getElementById('sidebarToggle').addEventListener('click', function() {
    const sidebar = document.getElementById('sidebar');
    const toggleIcon = document.querySelector('.toggle-icon');
    
    sidebarCollapsed = !sidebarCollapsed;
    
    if (sidebarCollapsed) {
        sidebar.classList.add('collapsed');
        toggleIcon.textContent = '›';
    } else {
        sidebar.classList.remove('collapsed');
        toggleIcon.textContent = '‹';
    }
});

// Update time
function updateTime() {
    const now = new Date();
    const timeString = now.toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});
    document.getElementById('currentTime').textContent = timeString;
}

// Global variable to store user info
let currentUser = null;

// Load user info - using your backend's /api/user-info endpoint
async function loadUserInfo() {
    try {
        const response = await fetch('/api/user-info');
        if (response.ok) {
            const user = await response.json();
            currentUser = user; // Store user info globally
            
            // Update user info
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            document.getElementById('userName').textContent = firstName;
            document.getElementById('userRole').textContent = (user.role || 'USER').toUpperCase();
            document.getElementById('userAvatar').textContent = firstName.charAt(0).toUpperCase();
            
            // Update welcome message
            document.querySelector('.page-title').textContent = `Welcome back, ${firstName}!`;
            
            // Show admin sections if user is admin
            if (user.role === 'admin') {
                document.getElementById('adminSection').style.display = 'block';
                document.getElementById('adminCard').style.display = 'block';
            }
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        }
    } catch (error) {
        console.error('Error loading user info:', error);
        window.location.href = 'login.html';
    }
}

// Logout function - using your backend's /api/logout endpoint
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

// Navigation handling
document.querySelectorAll('.nav-item, .action-tile').forEach(item => {
    item.addEventListener('click', function(e) {
        // Get the target from href or data attribute
        const target = this.getAttribute('href') || this.getAttribute('data-target');

        // Allow normal navigation for actual HTML pages
        if (target && (target.endsWith('.html') || target.startsWith('http'))) {
            // Don't prevent default - allow normal navigation
            return;
        }

        // Only prevent default for hash-based navigation
        e.preventDefault();

        // Remove active class from all nav items
        document.querySelectorAll('.nav-item').forEach(nav => nav.classList.remove('active'));

        // Add active class to clicked nav item (if it's a nav item)
        if (this.classList.contains('nav-item')) {
            this.classList.add('active');
        }

        // Handle different routes
        handleNavigation(target);
    });
});

// Handle navigation routing
function handleNavigation(target) {
    console.log('Navigating to:', target);
    
    switch(target) {
        case '#dashboard':
            showDashboardContent();
            break;
            
        case '#interfaces':
            showInterfacesContent();
            break;
            
        case '#messages':
            showMessagesContent();
            break;
            
        case '#templates':
            showTemplatesContent();
            break;
            
        case '#monitoring':
            showMonitoringContent();
            break;
            
        case '#reports':
            showReportsContent();
            break;
            
        case '#users':
            showUserManagementContent();
            break;
            
        case '#settings':
            showSettingsContent();
            break;
            
        default:
            console.log('Unknown route:', target);
            showNotImplemented(target);
    }
}

// Content display functions
function showDashboardContent() {
    updatePageTitle('Dashboard', 'Overview of your healthcare integration platform');
    // Dashboard is already visible - no change needed
}

function showInterfacesContent() {
    // Redirect to dedicated interfaces page
    window.location.href = 'interfaces.html';
}

function showMessagesContent() {
    updatePageTitle('Messages', 'Monitor and track message processing');
    showPlaceholderContent('Messages', 'View real-time message processing, transformation logs, and troubleshoot any integration issues.');
}

function showTemplatesContent() {
    updatePageTitle('Templates', 'Pre-built integration templates');
    showPlaceholderContent('Templates', 'Browse and use pre-built templates for common healthcare integration scenarios like ADT, ORU, and more.');
}

function showMonitoringContent() {
    updatePageTitle('Monitoring', 'System performance and health monitoring');
    showPlaceholderContent('Monitoring', 'Real-time dashboard showing system performance, message throughput, error rates, and health metrics.');
}

function showReportsContent() {
    updatePageTitle('Reports', 'Analytics and reporting dashboard');
    showPlaceholderContent('Reports', 'Generate detailed reports on message volumes, transformation success rates, and system analytics.');
}

function showUserManagementContent() {
    // Check if user is admin
    if (!currentUser || currentUser.role !== 'admin') {
        showAccessDenied('User Management', 'This section requires administrator privileges.');
        return;
    }
    
    // Redirect to dedicated user management page
    window.location.href = 'user-management.html';
}

function showSettingsContent() {
    // Check if user is admin
    if (!currentUser || currentUser.role !== 'admin') {
        showAccessDenied('System Settings', 'This section requires administrator privileges.');
        return;
    }
    
    updatePageTitle('System Settings', 'Configure platform settings');
    showPlaceholderContent('System Settings', 'Configure system-wide settings, security options, and integration preferences.');
}

// New function for access denied
function showAccessDenied(sectionName, message) {
    updatePageTitle('Access Denied', 'Insufficient permissions');
    
    // Hide dashboard grid and show access denied
    const dashboardGrid = document.querySelector('.dashboard-grid');
    
    // Remove any existing placeholder
    const existingPlaceholder = document.querySelector('.content-placeholder');
    if (existingPlaceholder) {
        existingPlaceholder.remove();
    }
    
    // Create access denied content
    const placeholder = document.createElement('div');
    placeholder.className = 'content-placeholder';
    placeholder.innerHTML = `
        <div class="placeholder-card access-denied">
            <div class="placeholder-icon">🔒</div>
            <h2 class="placeholder-title">Access Denied</h2>
            <p class="placeholder-description">You don't have permission to access <strong>${sectionName}</strong>. ${message}</p>
            <div class="placeholder-actions">
                <button class="placeholder-btn primary" onclick="showDashboard()">
                    <span class="nav-icon">🏠</span>
                    Back to Dashboard
                </button>
                <span class="admin-badge error">Admin Required</span>
            </div>
        </div>
    `;
    
    // Hide dashboard grid and show placeholder
    dashboardGrid.style.display = 'none';
    document.querySelector('.main-content').appendChild(placeholder);
}

function showNotImplemented(route) {
    updatePageTitle('Coming Soon', 'This feature is under development');
    showPlaceholderContent('Feature Under Development', `The ${route} section is currently being built and will be available soon.`);
}

// Helper function to update page title
function updatePageTitle(title, subtitle) {
    document.querySelector('.page-title').textContent = title;
    // You could add a subtitle element if needed
}

// Helper function to show placeholder content
function showPlaceholderContent(title, description) {
    // Hide dashboard grid and show placeholder
    const dashboardGrid = document.querySelector('.dashboard-grid');
    
    // Remove any existing placeholder
    const existingPlaceholder = document.querySelector('.content-placeholder');
    if (existingPlaceholder) {
        existingPlaceholder.remove();
    }
    
    // Create placeholder content
    const placeholder = document.createElement('div');
    placeholder.className = 'content-placeholder';
    placeholder.innerHTML = `
        <div class="placeholder-card">
            <div class="placeholder-icon">${getIconForSection(title)}</div>
            <h2 class="placeholder-title">${title}</h2>
            <p class="placeholder-description">${description}</p>
            <div class="placeholder-actions">
                <button class="placeholder-btn primary" onclick="showDashboard()">
                    <span class="nav-icon">🏠</span>
                    Back to Dashboard
                </button>
            </div>
        </div>
    `;
    
    // Hide dashboard grid and show placeholder
    dashboardGrid.style.display = 'none';
    document.querySelector('.main-content').appendChild(placeholder);
}

// Helper function to get appropriate icons
function getIconForSection(title) {
    const icons = {
        'Interfaces': '🔗',
        'Messages': '📧',
        'Templates': '📄',
        'Monitoring': '📊',
        'Reports': '📈',
        'User Management': '👥',
        'System Settings': '⚙️'
    };
    return icons[title] || '📋';
}

// Function to show dashboard (hide placeholder)
function showDashboard() {
    const placeholder = document.querySelector('.content-placeholder');
    if (placeholder) {
        placeholder.remove();
    }
    document.querySelector('.dashboard-grid').style.display = 'grid';
    
    // Update active nav
    document.querySelectorAll('.nav-item').forEach(nav => nav.classList.remove('active'));
    document.querySelector('.nav-item[href="#dashboard"]').classList.add('active');
    
    updatePageTitle('Welcome back!', 'Dashboard overview');
}

// Initialize on page load
window.addEventListener('load', function() {
    loadUserInfo();
    updateTime();
    setInterval(updateTime, 60000); // Update time every minute
});