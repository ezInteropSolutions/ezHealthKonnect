// User Management JavaScript - Complete Working Version

let users = [];
let currentUser = null;

// Sidebar toggle functionality
let sidebarCollapsed = false;

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

// Load users on page load
window.addEventListener('load', function() {
    loadCurrentUser();
    loadUsers();
    updateTime();
    setInterval(updateTime, 60000);
});

async function loadCurrentUser() {
    try {
        // Use the correct backend endpoint: /api/user-info (not /api/auth/profile)
        const response = await fetch('/api/user-info');
        if (response.ok) {
            const user = await response.json();
            currentUser = user;
            
            // Update user info
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            document.getElementById('userName').textContent = firstName;
            document.getElementById('userRole').textContent = (user.role || 'USER').toUpperCase();
            document.getElementById('userAvatar').textContent = firstName.charAt(0).toUpperCase();
            
            // Check if user is admin
            if (user.role !== 'admin') {
                // Redirect non-admin users
                window.location.href = 'dashboard.html';
                return;
            }
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        }
    } catch (error) {
        console.error('Error loading user info:', error);
        window.location.href = 'login.html';
    }
}

async function loadUsers() {
    try {
        showLoading(true);
        
        console.log('🔄 Loading users from API...');
        
        // Use your actual API endpoint
        const response = await fetch('/api/users', {
            method: 'GET',
            headers: {
                'Content-Type': 'application/json',
            }
        });
        
        console.log('📡 API Response:', response.status);
        
        if (response.ok) {
            users = await response.json();
            console.log('✅ Users loaded from API:', users.length, users);
            renderUsersTable();
            updateUserStats();
        } else if (response.status === 401) {
            console.log('❌ Unauthorized - redirecting to login');
            window.location.href = 'login.html';
            return;
        } else if (response.status === 403) {
            console.log('❌ Access denied - not admin');
            showAlert('Access denied. Admin privileges required.', 'error');
            window.location.href = 'dashboard.html';
            return;
        } else {
            console.log('❌ API Error:', response.status, response.statusText);
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
        
        showLoading(false);
        
    } catch (error) {
        console.error('❌ Error loading users:', error);
        console.log('🔄 Falling back to mock data for development');
        
        // Fallback to mock data for development
        users = getMockUsers();
        console.log('📝 Using mock users:', users);
        renderUsersTable();
        updateUserStats();
        showAlert('Using demo data (API not available)', 'info');
        showLoading(false);
    }
}

function getMockUsers() {
    return [
        {
            id: 1,
            name: 'System Administrator',
            email: 'admin@ezhealthkonnect.com',
            role: 'admin',
            status: 'active',
            createdAt: new Date().toISOString(),
            lastLogin: new Date().toISOString()
        },
        {
            id: 2,
            name: 'Healthcare User',
            email: 'user@ezhealthkonnect.com',
            role: 'user',
            status: 'active',
            createdAt: new Date(Date.now() - 86400000).toISOString(),
            lastLogin: new Date(Date.now() - 3600000).toISOString()
        }
    ];
}

function updateUserStats() {
    const totalUsers = users.length;
    const activeUsers = users.filter(u => u.status === 'active').length;
    const adminUsers = users.filter(u => u.role === 'admin').length;
    
    document.getElementById('totalUsers').textContent = totalUsers;
    document.getElementById('activeUsers').textContent = activeUsers;
    document.getElementById('adminUsers').textContent = adminUsers;
}

function renderUsersTable() {
    const tbody = document.getElementById('usersTableBody');
    
    if (users.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="7">
                    <div class="empty-state">
                        <div class="empty-icon">👥</div>
                        <div>No users found</div>
                    </div>
                </td>
            </tr>
        `;
        return;
    }
    
    tbody.innerHTML = users.map(user => `
        <tr>
            <td>
                <div style="display: flex; align-items: center; gap: 8px;">
                    <strong>${user.name}</strong>
                    <button class="btn-edit" onclick="showEditUserModal('${user.id}')" title="Edit Profile">
                        ✏️
                    </button>
                </div>
            </td>
            <td>${user.email}</td>
            <td>
                <span class="role-badge role-${user.role}">
                    ${user.role.toUpperCase()}
                </span>
            </td>
            <td>
                <span class="status-badge status-${user.status}">
                    ${user.status.toUpperCase()}
                </span>
            </td>
            <td>${formatDate(user.createdAt)}</td>
            <td>${user.lastLogin ? formatDate(user.lastLogin) : 'Never'}</td>
            <td>
                <div class="action-buttons">
                    ${user.status === 'active' ? 
                        `<button class="btn btn-warning btn-sm" onclick="toggleUserStatus('${user.id}', 'inactive')">
                            <span>⏸</span>
                            <span>Deactivate</span>
                        </button>` :
                        `<button class="btn btn-primary btn-sm" onclick="toggleUserStatus('${user.id}', 'active')">
                            <span>▶</span>
                            <span>Activate</span>
                        </button>`
                    }
                </div>
            </td>
        </tr>
    `).join('');
}

function formatDate(dateString) {
    if (!dateString) return 'N/A';
    const date = new Date(dateString);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
}

function showCreateUserModal() {
    document.getElementById('createUserModal').style.display = 'block';
}

function closeCreateUserModal() {
    document.getElementById('createUserModal').style.display = 'none';
    document.getElementById('createUserForm').reset();
}

function showEditUserModal(userId) {
    const user = users.find(u => u.id == userId);
    if (!user) return;
    
    // Populate form with user data
    document.getElementById('editUserId').value = user.id;
    document.getElementById('editUserName').value = user.name;
    document.getElementById('editUserEmail').value = user.email;
    document.getElementById('editUserRole').value = user.role;
    document.getElementById('editUserPassword').value = ''; // Always empty for security
    
    document.getElementById('editUserModal').style.display = 'block';
}

function closeEditUserModal() {
    document.getElementById('editUserModal').style.display = 'none';
    document.getElementById('editUserForm').reset();
}

// Handle create user form submission
document.getElementById('createUserForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    
    const formData = new FormData(e.target);
    const userData = {
        name: formData.get('name'),
        email: formData.get('email'),
        password: formData.get('password'),
        role: formData.get('role')
    };
    
    try {
        showLoading(true);
        
        // Use your actual API endpoint
        const response = await fetch('/api/users', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(userData)
        });
        
        if (response.ok) {
            const result = await response.json();
            showAlert(result.message || 'User created successfully!', 'success');
            closeCreateUserModal();
            loadUsers(); // Refresh from server
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else if (response.status === 403) {
            showAlert('Access denied. Admin privileges required.', 'error');
        } else {
            const result = await response.json();
            showAlert(result.message || 'Failed to create user', 'error');
        }
        
        showLoading(false);
        
    } catch (apiError) {
        console.log('API not available, using mock functionality');
        // Mock creation for development
        const newUser = {
            id: Math.max(...users.map(u => u.id), 0) + 1,
            ...userData,
            status: 'active',
            createdAt: new Date().toISOString(),
            lastLogin: null
        };
        delete newUser.password; // Don't store password in frontend
        
        users.push(newUser);
        renderUsersTable();
        updateUserStats();
        showAlert('User created successfully! (Development mode)', 'success');
        closeCreateUserModal();
        showLoading(false);
    }
});

// Handle edit user form submission
document.getElementById('editUserForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    
    const formData = new FormData(e.target);
    const userId = formData.get('id');
    const userData = {
        name: formData.get('name'),
        email: formData.get('email'),
        role: formData.get('role')
    };
    
    // Only include password if it's provided
    const password = formData.get('password');
    if (password && password.trim()) {
        userData.password = password;
    }
    
    try {
        showLoading(true);
        
        // Use your actual API endpoint
        const response = await fetch(`/api/users/${userId}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(userData)
        });
        
        if (response.ok) {
            const result = await response.json();
            showAlert(result.message || 'User updated successfully!', 'success');
            closeEditUserModal();
            loadUsers(); // Refresh from server
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else if (response.status === 403) {
            showAlert('Access denied. Admin privileges required.', 'error');
        } else {
            const result = await response.json();
            showAlert(result.message || 'Failed to update user', 'error');
        }
        
        showLoading(false);
        
    } catch (error) {
        console.error('Error updating user:', error);
        showAlert('Error updating user: ' + error.message, 'error');
        showLoading(false);
    }
});

async function toggleUserStatus(userId, newStatus) {
    const user = users.find(u => u.id == userId);
    if (!user) {
        showAlert('User not found', 'error');
        return;
    }
    
    const action = newStatus === 'active' ? 'activate' : 'deactivate';
    
    if (!confirm(`Are you sure you want to ${action} user "${user.email}"?`)) {
        return;
    }
    
    try {
        showLoading(true);
        
        // Use your actual API endpoint
        const response = await fetch(`/api/users/${userId}/status`, {
            method: 'PATCH',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ status: newStatus })
        });
        
        if (response.ok) {
            const result = await response.json();
            showAlert(result.message || `User ${action}d successfully!`, 'success');
            loadUsers(); // Refresh from server
        } else if (response.status === 401) {
            window.location.href = 'login.html';
        } else if (response.status === 403) {
            showAlert('Access denied. Admin privileges required.', 'error');
        } else {
            const result = await response.json();
            showAlert(result.message || `Failed to ${action} user`, 'error');
        }
        
        showLoading(false);
        
    } catch (error) {
        console.error(`Error ${action}ing user:`, error);
        showAlert(`Error ${action}ing user: ` + error.message, 'error');
        showLoading(false);
    }
}

function refreshUsers() {
    loadUsers();
}

function showAlert(message, type) {
    const alertContainer = document.getElementById('alertContainer');
    const alertClass = type === 'success' ? 'alert-success' : 
                      type === 'info' ? 'alert-info' : 'alert-error';
    
    alertContainer.innerHTML = `
        <div class="alert ${alertClass}" style="display: block;">
            ${message}
        </div>
    `;
    
    // Auto-hide after 5 seconds
    setTimeout(() => {
        alertContainer.innerHTML = '';
    }, 5000);
}

function showLoading(show) {
    const container = document.querySelector('.user-management-content');
    if (show) {
        container.classList.add('loading');
    } else {
        container.classList.remove('loading');
    }
}

// Logout function - using correct backend endpoint /api/logout
function logout() {
    if (confirm('Are you sure you want to logout?')) {
        fetch('/api/logout', { method: 'POST' })
            .then(() => window.location.href = 'login.html');
    }
}

// Close modal when clicking outside
window.addEventListener('click', function(event) {
    const createModal = document.getElementById('createUserModal');
    const editModal = document.getElementById('editUserModal');
    
    if (event.target === createModal) {
        closeCreateUserModal();
    }
    if (event.target === editModal) {
        closeEditUserModal();
    }
});

// Handle escape key to close modal
window.addEventListener('keydown', function(event) {
    if (event.key === 'Escape') {
        closeCreateUserModal();
        closeEditUserModal();
    }
});