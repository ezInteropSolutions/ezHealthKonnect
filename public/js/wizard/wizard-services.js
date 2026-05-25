// wizard-services.js - Core services for the ezHealthKonnect Interface Wizard

/**
 * Notification Service - Handles user notifications
 */
class NotificationService {
    constructor() {
        this.notifications = [];
        this.maxNotifications = 5;
    }

    show(message, type = 'info') {
        console.log(`📢 Notification [${type.toUpperCase()}]: ${message}`);
        
        const notification = {
            id: Date.now(),
            message,
            type,
            timestamp: new Date()
        };
        
        this.notifications.unshift(notification);
        
        // Keep only recent notifications
        if (this.notifications.length > this.maxNotifications) {
            this.notifications = this.notifications.slice(0, this.maxNotifications);
        }
        
        // Show visual notification
        this.displayNotification(notification);
        
        return notification;
    }

    displayNotification(notification) {
        const toast = document.createElement('div');
        toast.className = `notification-toast ${notification.type}`;
        toast.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            background: ${this.getNotificationColor(notification.type)};
            color: white;
            padding: 12px 16px;
            border-radius: 4px;
            font-weight: 500;
            z-index: 10000;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            max-width: 300px;
            transform: translateX(100%);
            transition: transform 0.3s ease;
        `;
        toast.textContent = notification.message;
        document.body.appendChild(toast);
        
        // Animate in
        setTimeout(() => {
            toast.style.transform = 'translateX(0)';
        }, 100);
        
        // Auto remove
        setTimeout(() => {
            toast.style.transform = 'translateX(100%)';
            setTimeout(() => {
                if (toast.parentNode) {
                    toast.parentNode.removeChild(toast);
                }
            }, 300);
        }, notification.type === 'error' ? 5000 : 3000);
    }

    getNotificationColor(type) {
        switch (type) {
            case 'error': return '#dc3545';
            case 'warning': return '#ffc107';
            case 'success': return '#28a745';
            case 'info': 
            default: return '#17a2b8';
        }
    }

    error(message) {
        return this.show(message, 'error');
    }

    success(message) {
        return this.show(message, 'success');
    }

    warning(message) {
        return this.show(message, 'warning');
    }

    info(message) {
        return this.show(message, 'info');
    }
}

/**
 * Validation Service - Handles form and data validation
 */
class ValidationService {
    constructor() {
        this.rules = {};
    }

    addRule(field, rule, message) {
        if (!this.rules[field]) {
            this.rules[field] = [];
        }
        this.rules[field].push({ rule, message });
    }

    validate(data) {
        const errors = [];
        
        for (const [field, rules] of Object.entries(this.rules)) {
            const value = data[field];
            
            for (const { rule, message } of rules) {
                if (typeof rule === 'function') {
                    if (!rule(value)) {
                        errors.push({ field, message });
                    }
                } else if (rule === 'required' && (!value || value.trim() === '')) {
                    errors.push({ field, message });
                }
            }
        }
        
        return {
            isValid: errors.length === 0,
            errors
        };
    }

    isValidHL7File(file) {
        if (!file) return false;
        
        const allowedTypes = ['.hl7', '.txt', '.dat', '.msg'];
        const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
        
        return allowedTypes.includes(fileExtension) || file.type === 'text/plain';
    }

    isValidFileSize(file, maxSizeMB = 5) {
        if (!file) return false;
        return file.size <= maxSizeMB * 1024 * 1024;
    }
}

/**
 * HL7 Service - Handles HL7 message parsing and processing
 */
// Replace the HL7Service class in wizard-services.js with this clean version:

/**
 * HL7 Service - Real API calls only
 */
class HL7Service {
    constructor(apiBaseUrl) {
        this.apiBaseUrl = apiBaseUrl || 'http://localhost:8080/api';
    }

    async parseHL7Message(content, validateStrict = true) {
        console.log('🔍 HL7Service: Parsing HL7 message...');
        
        try {
            // Basic validation
            if (!content || typeof content !== 'string') {
                throw new Error('Invalid HL7 content provided');
            }

            if (!content.includes('MSH|') && !content.startsWith('MSH')) {
                throw new Error('Content does not appear to be a valid HL7 message');
            }

            console.log('📤 Sending HL7 message to API for parsing...');
            
            // ✅ REAL API CALL - Exactly like interface-wizard.js
            const response = await fetch(`${this.apiBaseUrl}/hl7/parse`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    rawMessage: content.trim(),
                    useEnhanced: validateStrict
                })
            });
            
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(`HTTP ${response.status}: ${errorText}`);
            }
            
            const result = await response.json();
            console.log('✅ HL7Service: Message parsed successfully');
            
            // ✅ Return exactly what the API gives us
            return result;
            
        } catch (error) {
            console.error('❌ HL7Service: Parse error:', error);
            return {
                success: false,
                error: error.message,
                data: null
            };
        }
    }
}

/**
 * Loading Service - Handles loading states and overlays
 */
class LoadingService {
    constructor() {
        this.loadingStates = new Map();
    }

    show(id, message, subtext = '') {
        console.log(`⏳ Loading: ${message}`);
        this.loadingStates.set(id, { message, subtext, startTime: Date.now() });
        
        // Dispatch custom event for UI components
        window.dispatchEvent(new CustomEvent('loadingStart', {
            detail: { id, message, subtext }
        }));
    }

    hide(id) {
        const state = this.loadingStates.get(id);
        if (state) {
            const duration = Date.now() - state.startTime;
            console.log(`✅ Loading complete: ${state.message} (${duration}ms)`);
            this.loadingStates.delete(id);
            
            window.dispatchEvent(new CustomEvent('loadingEnd', {
                detail: { id, duration }
            }));
        }
    }

    isLoading(id) {
        return this.loadingStates.has(id);
    }
}

// Make services available globally
window.NotificationService = NotificationService;
window.ValidationService = ValidationService;
window.HL7Service = HL7Service;
window.LoadingService = LoadingService;

// Also export for module systems
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        NotificationService,
        ValidationService,
        HL7Service,
        LoadingService
    };
}

console.log('✅ Wizard services loaded and available globally');