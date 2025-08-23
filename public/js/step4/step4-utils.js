// step4-utils.js - MINIMAL FIX - Just add missing notification functionality

class Step4Utils {
    constructor() {
        this.notificationTimeout = null;
    }

    // Simple notification system that won't conflict with existing UI
    showNotification(message, type = 'info') {
        // Remove existing notification
        const existing = document.querySelector('.step4-notification');
        if (existing) {
            existing.remove();
        }

        const toast = document.createElement('div');
        toast.className = 'step4-notification';
        
        const colors = {
            info: '#3b82f6',
            success: '#10b981',
            warning: '#f59e0b',
            error: '#ef4444'
        };
        
        toast.style.cssText = `
            position: fixed;
            top: 24px;
            right: 24px;
            padding: 12px 20px;
            border-radius: 8px;
            color: white;
            font-size: 14px;
            font-weight: 500;
            z-index: 10001;
            max-width: 350px;
            background: ${colors[type]};
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
            transform: translateX(100%);
            transition: transform 0.3s ease;
        `;
        
        toast.textContent = message;
        document.body.appendChild(toast);
        
        // Animate in
        requestAnimationFrame(() => {
            toast.style.transform = 'translateX(0)';
        });
        
        // Clear existing timeout
        if (this.notificationTimeout) {
            clearTimeout(this.notificationTimeout);
        }
        
        // Animate out and remove
        this.notificationTimeout = setTimeout(() => {
            toast.style.transform = 'translateX(100%)';
            setTimeout(() => {
                if (toast.parentNode) {
                    toast.remove();
                }
            }, 300);
        }, type === 'error' ? 4000 : 3000);
    }

    // Simple utility functions that won't interfere with existing code
    truncateValue(value, maxLength = 30) {
        if (!value) return '';
        const str = String(value);
        return str.length > maxLength ? str.substring(0, maxLength) + '...' : str;
    }

    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    // Safe DOM helpers
    getElementById(id) {
        return document.getElementById(id);
    }

    // JSON formatting
    formatJSON(obj, indent = 2) {
        try {
            return JSON.stringify(obj, null, indent);
        } catch (error) {
            console.error('Failed to format JSON:', error);
            return String(obj);
        }
    }

    // Simple clipboard utility
    async copyToClipboard(text) {
        try {
            await navigator.clipboard.writeText(text);
            this.showNotification('✅ Copied to clipboard', 'success');
            return true;
        } catch (error) {
            console.error('Failed to copy to clipboard:', error);
            this.showNotification('❌ Failed to copy to clipboard', 'error');
            return false;
        }
    }

    // Download utility
    downloadAsFile(content, filename, mimeType = 'application/json') {
        const blob = new Blob([content], { type: mimeType });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        
        a.href = url;
        a.download = filename;
        a.click();
        
        URL.revokeObjectURL(url);
        this.showNotification(`✅ Downloaded ${filename}`, 'success');
    }

    // Simple error handling
    handleApiError(error, context = 'API') {
        let message = 'An unknown error occurred';
        
        if (error.message) {
            message = error.message;
        } else if (typeof error === 'string') {
            message = error;
        }
        
        console.error(`❌ ${context} error:`, error);
        this.showNotification(`${context} Error: ${message}`, 'error');
        return message;
    }

    // Debug utilities
    logDebug(message, data = null) {
        if (window.DEBUG || localStorage.getItem('step4-debug') === 'true') {
            console.log(`[Step 4 Debug] ${message}`, data || '');
        }
    }

    logError(message, error = null) {
        console.error(`[Step 4 Error] ${message}`, error || '');
    }

    logInfo(message, data = null) {
        console.log(`[Step 4 Info] ${message}`, data || '');
    }
}

// Make available globally
window.Step4Utils = Step4Utils;

console.log('✅ Step 4 utilities (minimal) loaded');