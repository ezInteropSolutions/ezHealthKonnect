/**
 * Authentication Service (OOB)
 *
 * Universal authentication module for all pages in ezHealthKonnect.
 * Follows MVC and Out-of-Box (OOB) principles for reusability.
 *
 * Usage:
 *   Include this script in any page that requires authentication:
 *   <script src="/js/services/AuthService.js"></script>
 *
 *   Then call: AuthService.initialize();
 */

class AuthService {
    constructor() {
        this.currentUser = null;
        this.sessionCheckInterval = null;
        this.sessionCheckFrequency = 60000; // Check session every 60 seconds
        this.isRedirecting = false;
    }

    /**
     * Initialize authentication
     * Call this on page load
     */
    async initialize() {
        console.log('🔐 AuthService: Initializing...');

        try {
            // Check session validity
            const isValid = await this.checkSession();

            if (!isValid) {
                console.warn('🚫 AuthService: Session invalid, redirecting to login');
                this.redirectToLogin();
                return false;
            }

            // Load user info
            await this.loadUserInfo();

            // Setup periodic session checking
            this.startSessionMonitoring();

            console.log('✅ AuthService: Initialized successfully');
            return true;

        } catch (error) {
            console.error('❌ AuthService: Initialization failed:', error);
            this.redirectToLogin();
            return false;
        }
    }

    /**
     * Check if session is valid
     * @returns {Promise<boolean>}
     */
    async checkSession() {
        try {
            const response = await fetch('/api/auth/session', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            if (response.status === 401 || response.status === 403) {
                console.warn('🚫 AuthService: Session expired (401/403)');
                return false;
            }

            if (!response.ok) {
                console.warn(`⚠️ AuthService: Session check failed (${response.status})`);
                return false;
            }

            const data = await response.json();
            return data.authenticated === true;

        } catch (error) {
            console.error('❌ AuthService: Session check error:', error);
            return false;
        }
    }

    /**
     * Load user information
     * @returns {Promise<object|null>}
     */
    async loadUserInfo() {
        try {
            const response = await fetch('/api/user-info', {
                method: 'GET',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            if (response.status === 401 || response.status === 403) {
                console.warn('🚫 AuthService: Unauthorized access');
                this.redirectToLogin();
                return null;
            }

            if (!response.ok) {
                throw new Error(`Failed to load user info: ${response.status}`);
            }

            const user = await response.json();
            this.currentUser = user;

            console.log('✅ AuthService: User loaded:', user.email);

            // Update UI if elements exist
            this.updateUserUI(user);

            return user;

        } catch (error) {
            console.error('❌ AuthService: Error loading user info:', error);
            this.redirectToLogin();
            return null;
        }
    }

    /**
     * Update user interface elements
     * @param {object} user
     */
    updateUserUI(user) {
        // Update user name
        const userNameEl = document.getElementById('userName');
        if (userNameEl) {
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            userNameEl.textContent = firstName;
        }

        // Update user role
        const userRoleEl = document.getElementById('userRole');
        if (userRoleEl) {
            userRoleEl.textContent = (user.role || 'USER').toUpperCase();
        }

        // Update user avatar
        const userAvatarEl = document.getElementById('userAvatar');
        if (userAvatarEl) {
            const firstName = user.name ? user.name.split(' ')[0] : 'User';
            userAvatarEl.textContent = firstName.charAt(0).toUpperCase();
        }

        // Show admin section if admin role
        if (user.role === 'admin') {
            const adminSection = document.getElementById('adminSection');
            if (adminSection) {
                adminSection.style.display = 'block';
            }
        }
    }

    /**
     * Start periodic session monitoring
     */
    startSessionMonitoring() {
        // Clear existing interval
        if (this.sessionCheckInterval) {
            clearInterval(this.sessionCheckInterval);
        }

        // Check session periodically
        this.sessionCheckInterval = setInterval(async () => {
            const isValid = await this.checkSession();
            if (!isValid && !this.isRedirecting) {
                console.warn('🚫 AuthService: Session expired during monitoring');
                this.redirectToLogin();
            }
        }, this.sessionCheckFrequency);

        console.log(`🔄 AuthService: Session monitoring started (${this.sessionCheckFrequency / 1000}s interval)`);
    }

    /**
     * Stop session monitoring
     */
    stopSessionMonitoring() {
        if (this.sessionCheckInterval) {
            clearInterval(this.sessionCheckInterval);
            this.sessionCheckInterval = null;
            console.log('⏹️ AuthService: Session monitoring stopped');
        }
    }

    /**
     * Redirect to login page
     */
    redirectToLogin() {
        if (this.isRedirecting) {
            return; // Prevent multiple redirects
        }

        this.isRedirecting = true;

        // Stop session monitoring
        this.stopSessionMonitoring();

        // Store current URL to redirect back after login
        const currentPath = window.location.pathname + window.location.search;
        sessionStorage.setItem('redirectAfterLogin', currentPath);

        // Show session expired message
        this.showSessionExpiredMessage();

        // Redirect after short delay
        setTimeout(() => {
            window.location.href = '/login.html';
        }, 1500);
    }

    /**
     * Show session expired message
     */
    showSessionExpiredMessage() {
        // Remove existing message if any
        const existingMessage = document.getElementById('sessionExpiredMessage');
        if (existingMessage) {
            existingMessage.remove();
        }

        const messageDiv = document.createElement('div');
        messageDiv.id = 'sessionExpiredMessage';
        messageDiv.style.cssText = `
            position: fixed;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            background: linear-gradient(135deg, #1e3a8a 0%, #2563eb 100%);
            color: white;
            padding: 2rem 3rem;
            border-radius: 0.5rem;
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.3);
            text-align: center;
            z-index: 10000;
            animation: fadeIn 0.3s ease;
        `;

        messageDiv.innerHTML = `
            <i class="fas fa-sign-out-alt" style="font-size: 3rem; margin-bottom: 1rem;"></i>
            <h2 style="margin-bottom: 0.5rem;">Session Expired</h2>
            <p style="opacity: 0.9;">Redirecting to login page...</p>
        `;

        document.body.appendChild(messageDiv);
    }

    /**
     * Logout user
     * @returns {Promise<void>}
     */
    async logout() {
        if (!confirm('Are you sure you want to logout?')) {
            return;
        }

        try {
            // Stop session monitoring
            this.stopSessionMonitoring();

            // Call logout API
            await fetch('/api/auth/logout', {
                method: 'POST',
                credentials: 'include',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            console.log('✅ AuthService: Logout successful');

        } catch (error) {
            console.error('❌ AuthService: Logout error:', error);
        } finally {
            // Always redirect to login
            window.location.href = '/login.html';
        }
    }

    /**
     * Get current user
     * @returns {object|null}
     */
    getCurrentUser() {
        return this.currentUser;
    }

    /**
     * Check if user has specific role
     * @param {string} role
     * @returns {boolean}
     */
    hasRole(role) {
        return this.currentUser && this.currentUser.role === role;
    }

    /**
     * Check if user is admin
     * @returns {boolean}
     */
    isAdmin() {
        return this.hasRole('admin');
    }
}

// Create global singleton instance
window.AuthService = new AuthService();

// Add fadeIn animation CSS
if (!document.getElementById('authServiceStyles')) {
    const style = document.createElement('style');
    style.id = 'authServiceStyles';
    style.textContent = `
        @keyframes fadeIn {
            from { opacity: 0; transform: translate(-50%, -60%); }
            to { opacity: 1; transform: translate(-50%, -50%); }
        }
    `;
    document.head.appendChild(style);
}

console.log('✅ AuthService loaded and ready');
