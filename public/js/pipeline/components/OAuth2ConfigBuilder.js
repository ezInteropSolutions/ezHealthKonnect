// OAuth2ConfigBuilder.js - OAuth 2.0 configuration builder component
// Provides user-friendly OAuth 2.0 setup with grant type selection

class OAuth2ConfigBuilder {
    constructor(container, initialConfig = {}) {
        this.container = container;
        this.config = initialConfig;
        this.grantType = initialConfig.grantType || 'client_credentials';
        this.init();
    }

    init() {
        this.container.innerHTML = '';
        // Keep existing classes and add oauth2-config-builder
        if (!this.container.classList.contains('oauth2-config-builder')) {
            this.container.classList.add('oauth2-config-builder');
        }

        // Create header
        const header = document.createElement('div');
        header.className = 'oauth2-config-header';
        header.innerHTML = `
            <h4>OAuth 2.0 Configuration</h4>
            <small style="color: #6b7280;">Configure OAuth 2.0 settings, then use "🧪 Test API Endpoint" to test</small>
        `;
        this.container.appendChild(header);

        // Grant type selector
        const grantTypeSelector = document.createElement('div');
        grantTypeSelector.className = 'form-group';
        grantTypeSelector.innerHTML = `
            <label>Grant Type <span class="text-danger">*</span></label>
            <select class="form-control grant-type-select">
                <option value="client_credentials" ${this.grantType === 'client_credentials' ? 'selected' : ''}>
                    Client Credentials (Backend Integration)
                </option>
                <option value="password" ${this.grantType === 'password' ? 'selected' : ''}>
                    Password Grant (User Credentials)
                </option>
                <option value="refresh_token" ${this.grantType === 'refresh_token' ? 'selected' : ''}>
                    Refresh Token (Token Renewal)
                </option>
            </select>
            <small class="form-text text-muted grant-type-description"></small>
        `;
        this.container.appendChild(grantTypeSelector);

        this.grantTypeSelect = grantTypeSelector.querySelector('.grant-type-select');
        this.grantTypeDescription = grantTypeSelector.querySelector('.grant-type-description');

        // Configuration form
        this.formContainer = document.createElement('div');
        this.formContainer.className = 'oauth2-form-container';
        this.container.appendChild(this.formContainer);

        // Token display area
        const tokenDisplay = document.createElement('div');
        tokenDisplay.className = 'token-display';
        tokenDisplay.style.display = 'none';
        tokenDisplay.innerHTML = `
            <div class="alert alert-success">
                <div class="token-display-header">
                    <strong><i class="fas fa-check-circle"></i> Token Obtained Successfully</strong>
                    <button class="btn btn-sm btn-outline-secondary copy-token-btn" type="button">
                        <i class="fas fa-copy"></i> Copy Token
                    </button>
                </div>
                <div class="token-info">
                    <div class="token-field">
                        <label>Access Token:</label>
                        <code class="access-token-value"></code>
                    </div>
                    <div class="token-field">
                        <label>Token Type:</label>
                        <span class="token-type-value"></span>
                    </div>
                    <div class="token-field">
                        <label>Expires In:</label>
                        <span class="expires-in-value"></span>
                    </div>
                    <div class="token-field">
                        <label>Scope:</label>
                        <span class="scope-value"></span>
                    </div>
                </div>
            </div>
        `;
        this.container.appendChild(tokenDisplay);

        this.tokenDisplay = tokenDisplay;

        // Event listeners
        this.grantTypeSelect.addEventListener('change', () => {
            this.grantType = this.grantTypeSelect.value;
            this.renderForm();
            this.onChange();
        });

        // Initial render
        this.renderForm();
        this.updateGrantTypeDescription();

        // Add styles
        this.injectStyles();
    }

    renderForm() {
        this.formContainer.innerHTML = '';

        // Common fields
        this.addFormField('tokenURL', 'Token URL', 'text', 'https://example.com/oauth/token', true);
        this.addFormField('clientID', 'Client ID', 'text', 'your-client-id', true);
        this.addFormField('clientSecret', 'Client Secret', 'password', 'your-client-secret', true);

        // Grant-type specific fields
        if (this.grantType === 'password') {
            this.addFormField('username', 'Username', 'text', 'user@example.com', true);
            this.addFormField('password', 'Password', 'password', '', true);
        }

        if (this.grantType === 'refresh_token') {
            this.addFormField('refreshToken', 'Refresh Token', 'textarea', 'your-refresh-token', true);
        }

        // Optional fields (all grant types)
        this.addFormField('scope', 'Scope', 'text', 'read write', false);
        this.addFormField('audience', 'Audience', 'text', 'https://api.example.com/', false);

        this.updateGrantTypeDescription();
    }

    addFormField(name, label, type, placeholder, required = false) {
        const formGroup = document.createElement('div');
        formGroup.className = 'form-group';

        const value = this.config[name] || '';

        if (type === 'textarea') {
            formGroup.innerHTML = `
                <label>${label} ${required ? '<span class="text-danger">*</span>' : ''}</label>
                <textarea class="form-control oauth2-field" data-field="${name}"
                          placeholder="${placeholder}" rows="3">${this.escapeHtml(value)}</textarea>
            `;
        } else {
            formGroup.innerHTML = `
                <label>${label} ${required ? '<span class="text-danger">*</span>' : ''}</label>
                <input type="${type}" class="form-control oauth2-field" data-field="${name}"
                       value="${this.escapeHtml(value)}" placeholder="${placeholder}">
            `;
        }

        this.formContainer.appendChild(formGroup);

        const input = formGroup.querySelector('.oauth2-field');
        input.addEventListener('input', () => this.onChange());
    }

    updateGrantTypeDescription() {
        const descriptions = {
            client_credentials: 'Best for backend integrations (machine-to-machine). Used by Epic FHIR, Cerner, and most EHR systems.',
            password: 'Uses username and password to obtain access token. Less secure, use only when necessary.',
            refresh_token: 'Uses a refresh token to obtain a new access token when the current one expires.'
        };

        this.grantTypeDescription.textContent = descriptions[this.grantType] || '';
    }

    getConfig() {
        const config = {
            grantType: this.grantType
        };

        this.formContainer.querySelectorAll('.oauth2-field').forEach(field => {
            const name = field.dataset.field;
            const value = field.value.trim();
            if (value) {
                config[name] = value;
            }
        });

        return config;
    }

    setConfig(config) {
        this.config = config;
        this.grantType = config.grantType || 'client_credentials';
        this.grantTypeSelect.value = this.grantType;
        this.renderForm();
    }

    async testConnection() {
        const config = this.getConfig();
        const btn = this.container.querySelector('.test-oauth-btn');
        const originalHTML = btn.innerHTML;

        // Validate required fields
        if (!config.tokenURL || !config.clientID || !config.clientSecret) {
            this.showNotification('Please fill in all required fields (Token URL, Client ID, Client Secret)', 'warning');
            return;
        }

        if (config.grantType === 'password' && (!config.username || !config.password)) {
            this.showNotification('Please fill in username and password for Password Grant', 'warning');
            return;
        }

        if (config.grantType === 'refresh_token' && !config.refreshToken) {
            this.showNotification('Please provide a refresh token', 'warning');
            return;
        }

        // Show loading state
        btn.disabled = true;
        btn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Testing...';

        try {
            // Make OAuth 2.0 token request
            const response = await this.requestToken(config);

            // Show success
            btn.innerHTML = '<i class="fas fa-check-circle"></i> Success!';
            btn.classList.add('btn-success');
            btn.classList.remove('btn-outline-success');

            // Display token information
            this.displayToken(response);

            setTimeout(() => {
                btn.innerHTML = originalHTML;
                btn.classList.remove('btn-success');
                btn.classList.add('btn-outline-success');
                btn.disabled = false;
            }, 3000);

        } catch (error) {
            // Show error
            btn.innerHTML = '<i class="fas fa-exclamation-circle"></i> Failed';
            btn.classList.add('btn-danger');

            this.showNotification(`OAuth 2.0 Test Failed: ${error.message}`, 'error');

            setTimeout(() => {
                btn.innerHTML = originalHTML;
                btn.classList.remove('btn-danger');
                btn.disabled = false;
            }, 3000);
        }
    }

    /**
     * Show in-app notification
     */
    showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: ${type === 'success' ? '#10b981' : type === 'error' ? '#ef4444' : type === 'warning' ? '#f59e0b' : '#06b6d4'};
            color: white;
            padding: 12px 20px;
            border-radius: 6px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            z-index: 10000;
            max-width: 400px;
        `;
        notification.textContent = message;
        document.body.appendChild(notification);
        setTimeout(() => notification.remove(), 4000);
    }

    async requestToken(config) {
        // Build request body
        const body = new URLSearchParams();
        body.append('grant_type', config.grantType);

        if (config.grantType === 'password') {
            body.append('username', config.username);
            body.append('password', config.password);
        }

        if (config.grantType === 'refresh_token') {
            body.append('refresh_token', config.refreshToken);
        }

        if (config.scope) {
            body.append('scope', config.scope);
        }

        // Create Basic Auth header
        const authHeader = 'Basic ' + btoa(`${config.clientID}:${config.clientSecret}`);

        // Make request
        const response = await fetch(config.tokenURL, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded',
                'Authorization': authHeader
            },
            body: body.toString()
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        return await response.json();
    }

    displayToken(tokenData) {
        this.tokenDisplay.style.display = 'block';

        const accessToken = tokenData.access_token || 'N/A';
        const tokenType = tokenData.token_type || 'Bearer';
        const expiresIn = tokenData.expires_in || 0;
        const scope = tokenData.scope || 'N/A';

        this.tokenDisplay.querySelector('.access-token-value').textContent =
            accessToken.substring(0, 50) + (accessToken.length > 50 ? '...' : '');
        this.tokenDisplay.querySelector('.token-type-value').textContent = tokenType;
        this.tokenDisplay.querySelector('.expires-in-value').textContent =
            `${expiresIn} seconds (${Math.floor(expiresIn / 60)} minutes)`;
        this.tokenDisplay.querySelector('.scope-value').textContent = scope;

        // Copy token button
        const copyBtn = this.tokenDisplay.querySelector('.copy-token-btn');
        copyBtn.onclick = () => {
            navigator.clipboard.writeText(accessToken).then(() => {
                const originalHTML = copyBtn.innerHTML;
                copyBtn.innerHTML = '<i class="fas fa-check"></i> Copied!';
                setTimeout(() => {
                    copyBtn.innerHTML = originalHTML;
                }, 2000);
            });
        };
    }

    onChange() {
        // Trigger custom event
        const event = new CustomEvent('oauth2ConfigChanged', {
            detail: { config: this.getConfig() }
        });
        this.container.dispatchEvent(event);
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    injectStyles() {
        if (document.getElementById('oauth2-config-builder-styles')) return;

        const style = document.createElement('style');
        style.id = 'oauth2-config-builder-styles';
        style.textContent = `
            .oauth2-config-builder {
                border: 1px solid #ddd;
                border-radius: 4px;
                padding: 20px;
                background: #f8f9fa;
            }

            .oauth2-config-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 20px;
            }

            .oauth2-config-header h4 {
                margin: 0;
                font-size: 16px;
                font-weight: 600;
            }

            .oauth2-form-container {
                background: white;
                padding: 20px;
                border-radius: 4px;
                margin-bottom: 15px;
            }

            .oauth2-form-container .form-group {
                margin-bottom: 15px;
            }

            .oauth2-form-container label {
                font-weight: 600;
                font-size: 14px;
                margin-bottom: 5px;
            }

            .grant-type-select {
                font-weight: 500;
            }

            .grant-type-description {
                margin-top: 5px;
                font-style: italic;
            }

            .token-display {
                margin-top: 15px;
            }

            .token-display-header {
                display: flex;
                justify-content: space-between;
                align-items: center;
                margin-bottom: 15px;
            }

            .token-info {
                margin-top: 15px;
            }

            .token-field {
                margin-bottom: 10px;
                padding: 8px;
                background: #f8f9fa;
                border-radius: 4px;
            }

            .token-field label {
                font-weight: 600;
                font-size: 12px;
                color: #6c757d;
                display: block;
                margin-bottom: 4px;
            }

            .token-field code {
                font-size: 13px;
                background: white;
                padding: 4px 8px;
                border-radius: 3px;
                display: block;
                word-break: break-all;
            }

            .test-oauth-btn {
                font-size: 13px;
            }
        `;
        document.head.appendChild(style);
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = OAuth2ConfigBuilder;
}
