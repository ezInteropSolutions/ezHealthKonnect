// Enhanced login form handler - EXACT copy of original working logic
document.getElementById('loginForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    
    const email = document.getElementById('email').value;
    const password = document.getElementById('password').value;
    const loginButton = document.getElementById('loginButton');
    const buttonText = loginButton.querySelector('.button-text');
    
    // Enhanced loading state
    loginButton.disabled = true;
    loginButton.classList.add('loading');
    buttonText.textContent = 'Signing In...';
    
    try {
        const response = await fetch('/api/auth/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ email, password })
        });
        
        const result = await response.json();
        
        if (response.ok) {
            showMessage('Login successful! Redirecting to dashboard...', 'success');

            // Store token securely
            if (result.token) {
                localStorage.setItem('accessToken', result.token);
            }

            // Smooth redirect - use relative path
            setTimeout(() => {
                window.location.href = 'dashboard.html';
            }, 1500);
        } else if (response.status === 403 && result.requiresPasswordReset) {
            // Admin has forced a password reset — redirect to the reset page
            showMessage('Password reset required. Redirecting…', 'error');
            setTimeout(() => {
                window.location.href = `force-reset.html?userId=${result.userId}`;
            }, 1200);
        } else if (response.status === 423) {
            // Account locked
            const until = result.lockedUntil ? ` Try again after ${new Date(result.lockedUntil).toLocaleTimeString()}.` : '';
            showMessage((result.message || 'Account is temporarily locked.') + until, 'error');
        } else {
            showMessage(result.message || 'Invalid credentials. Please try again.', 'error');
        }
    } catch (error) {
        console.error('Login error:', error);
        showMessage('Connection error. Please check your internet connection.', 'error');
    } finally {
        // Reset button state
        loginButton.disabled = false;
        loginButton.classList.remove('loading');
        buttonText.textContent = 'Sign In';
    }
});

function showMessage(message, type) {
    const messageDiv = document.getElementById('statusMessage');
    console.log('Showing message:', message, 'Type:', type);
    
    if (!messageDiv) {
        console.error('statusMessage element not found!');
        AppDialogs.toast(message, type === 'error' ? 'error' : 'info'); // Fallback toast
        return;
    }
    
    messageDiv.textContent = message;
    messageDiv.className = `status-message ${type}`;
    messageDiv.style.display = 'block';
    
    // Auto-hide after 5 seconds
    setTimeout(() => {
        messageDiv.style.display = 'none';
        messageDiv.textContent = '';
        messageDiv.className = 'status-message';
    }, 5000);
}

function showRegister() {
    showMessage('Registration will be available soon. Contact your administrator for access.', 'info');
}

function showForgotPassword() {
    showMessage('Password reset functionality coming soon. Contact support for assistance.', 'info');
}

// Clear messages when user starts typing
['email', 'password'].forEach(id => {
    document.getElementById(id).addEventListener('input', () => {
        const messageDiv = document.getElementById('statusMessage');
        if (messageDiv.style.display === 'block') {
            messageDiv.style.display = 'none';
        }
    });
});

// Enhanced form interactions
document.querySelectorAll('.form-input').forEach(input => {
    input.addEventListener('focus', function() {
        this.parentElement.classList.add('focused');
    });
    
    input.addEventListener('blur', function() {
        this.parentElement.classList.remove('focused');
    });
});