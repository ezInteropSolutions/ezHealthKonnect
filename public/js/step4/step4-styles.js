// step4-styles.js - Enhanced with comprehensive working styles

class Step4Styles {
    constructor() {
        this.injected = false;
    }

    inject() {
        if (this.injected) return;

        const styleId = 'step4-enhanced-styles';
        if (document.getElementById(styleId)) return;

        const style = document.createElement('style');
        style.id = styleId;
        style.textContent = this.getEnhancedStyles();
        document.head.appendChild(style);
        
        this.injected = true;
        console.log('✅ Step 4 enhanced styles injected');
    }

    getEnhancedStyles() {
        return `
            /* Enhanced HL7 to FHIR Mapping Styles - Working Functionality */
            
            /* Better space utilization */
            .main-container {
                max-width: 1600px;
                margin: 0 auto;
                padding: 16px;
            }

            .step-header {
                background: linear-gradient(135deg, #ffffff 0%, #f8f9ff 100%);
                border: 2px solid #e0e7ff;
                border-radius: 12px;
                padding: 20px;
                margin-bottom: 20px;
                box-shadow: 0 2px 8px rgba(59, 130, 246, 0.1);
                text-align: center;
            }

            .step-title {
                font-size: 28px;
                margin-bottom: 8px;
                display: flex;
                align-items: center;
                justify-content: center;
                gap: 12px;
                color: #1e3a8a;
            }

            .step-description {
                font-size: 15px;
                margin-bottom: 0;
                color: #64748b;
            }

            /* Enhanced Grid Layout for Better Space Usage */
            .transformation-layout {
                display: grid;
                grid-template-columns: 1fr 300px;
                gap: 16px;
                margin-bottom: 16px;
            }

            .main-content-area {
                background: white;
                border-radius: 12px;
                box-shadow: 0 2px 8px rgba(0,0,0,0.1);
                overflow: hidden;
                min-height: 600px;
            }

            .sidebar-panel {
                background: white;
                border-radius: 12px;
                padding: 18px;
                box-shadow: 0 2px 8px rgba(0,0,0,0.1);
                max-height: 700px;
                overflow-y: auto;
            }

            /* Functional Action Bar */
            .action-toolbar {
                background: #f8fafc;
                padding: 12px 20px;
                border-bottom: 1px solid #e2e8f0;
                display: flex;
                justify-content: space-between;
                align-items: center;
                gap: 10px;
                flex-wrap: wrap;
            }

            .btn-group {
                display: flex;
                gap: 6px;
            }

            .enhanced-btn {
                display: inline-flex;
                align-items: center;
                gap: 6px;
                padding: 8px 14px;
                border: none;
                border-radius: 6px;
                font-weight: 500;
                cursor: pointer;
                transition: all 0.2s;
                font-size: 13px;
                line-height: 1.2;
                text-decoration: none;
            }

            .enhanced-btn.primary {
                background: #3b82f6;
                color: white;
            }

            .enhanced-btn.primary:hover {
                background: #2563eb;
                transform: translateY(-1px);
            }

            .enhanced-btn.secondary {
                background: #f1f5f9;
                color: #475569;
                border: 1px solid #cbd5e1;
            }

            .enhanced-btn.secondary:hover {
                background: #e2e8f0;
            }

            .enhanced-btn.success {
                background: #10b981;
                color: white;
            }

            .enhanced-btn.success:hover {
                background: #059669;
            }

            .enhanced-btn:disabled {
                opacity: 0.5;
                cursor: not-allowed;
                transform: none !important;
            }

            /* Working button styles for existing IDs */
            #expandAllBtn, #collapseAllBtn, #viewJsonBtn, #validateBtn, #saveConfigBtn {
                display: inline-flex;
                align-items: center;
                gap: 6px;
                padding: 8px 14px;
                border: none;
                border-radius: 6px;
                font-weight: 500;
                cursor: pointer;
                transition: all 0.2s;
                font-size: 13px;
                line-height: 1.2;
            }

            #expandAllBtn, #collapseAllBtn, #viewJsonBtn {
                background: #f1f5f9;
                color: #475569;
                border: 1px solid #cbd5e1;
            }

            #expandAllBtn:hover, #collapseAllBtn:hover, #viewJsonBtn:hover {
                background: #e2e8f0;
                transform: translateY(-1px);
            }

            #validateBtn {
                background: #10b981;
                color: white;
            }

            #validateBtn:hover {
                background: #059669;
                transform: translateY(-1px);
            }

            #saveConfigBtn {
                background: #3b82f6;
                color: white;
            }

            #saveConfigBtn:hover {
                background: #2563eb;
                transform: translateY(-1px);
            }

            /* Enhanced Resource Cards */
            .resources-container {
                padding: 16px 20px;
            }

            .resource-section {
                border: 1px solid #e2e8f0;
                border-radius: 10px;
                margin-bottom: 12px;
                overflow: hidden;
                transition: all 0.2s;
                border-bottom: 1px solid #e5e7eb;
            }

            .resource-section:last-child {
                border-bottom: none;
            }

            .resource-section:hover {
                box-shadow: 0 4px 12px rgba(0,0,0,0.1);
            }

            .resource-header {
                background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
                padding: 14px 18px;
                display: flex;
                justify-content: space-between;
                align-items: center;
                cursor: pointer;
                border-bottom: 1px solid #e2e8f0;
                transition: background 0.2s;
            }

            .resource-header:hover {
                background: #f3f4f6;
            }

            .resource-info {
                display: flex;
                align-items: center;
                gap: 12px;
            }

            .resource-icon {
                width: 36px;
                height: 36px;
                border-radius: 8px;
                display: flex;
                align-items: center;
                justify-content: center;
                font-size: 16px;
                font-weight: bold;
                background: #dbeafe;
                color: #1e40af;
            }

            .resource-details h4 {
                font-size: 15px;
                color: #1e3a8a;
                margin-bottom: 3px;
                line-height: 1.2;
            }

            .resource-meta {
                font-size: 13px;
                color: #64748b;
                line-height: 1.2;
            }

            .expand-indicator {
                display: flex;
                align-items: center;
                gap: 6px;
                color: #6366f1;
                font-weight: 500;
                font-size: 13px;
            }

            .expand-arrow {
                transition: transform 0.2s;
                font-size: 12px;
            }

            .resource-section.expanded .expand-arrow {
                transform: rotate(90deg);
            }

            .resource-content {
                display: none;
                padding: 16px 18px;
                background: #ffffff;
            }

            .resource-section.expanded .resource-content {
                display: block;
                animation: slideDown 0.2s ease;
            }

            @keyframes slideDown {
                from { opacity: 0; transform: translateY(-10px); }
                to { opacity: 1; transform: translateY(0); }
            }

            /* Enhanced Mapping Display */
            .mapping-table {
                display: grid;
                gap: 10px;
            }

            .mapping-row {
                background: #f8fafc;
                border: 1px solid #e2e8f0;
                border-radius: 8px;
                padding: 12px 14px;
                display: grid;
                grid-template-columns: 1fr auto 1fr auto;
                gap: 12px;
                align-items: center;
            }

            .field-info {
                display: flex;
                flex-direction: column;
                gap: 2px;
            }

            .field-path {
                font-family: 'SF Mono', 'Monaco', 'Inconsolata', 'Roboto Mono', 'Courier New', monospace;
                font-size: 12px;
                color: #1e40af;
                background: rgba(30, 64, 175, 0.08);
                padding: 3px 6px;
                border-radius: 4px;
                display: inline-block;
                line-height: 1.2;
            }

            .field-value {
                font-size: 13px;
                color: #374151;
                font-weight: 500;
                margin-top: 2px;
                line-height: 1.2;
            }

            .mapping-arrow {
                color: #10b981;
                font-size: 16px;
                font-weight: bold;
            }

            .status-indicator {
                padding: 3px 10px;
                border-radius: 16px;
                font-size: 11px;
                font-weight: 600;
                text-align: center;
                line-height: 1.2;
            }

            .status-success {
                background: #dcfce7;
                color: #166534;
            }

            .status-warning {
                background: #fef3c7;
                color: #92400e;
            }

            .status-error {
                background: #fee2e2;
                color: #dc2626;
            }

            /* Enhanced Sidebar */
            .sidebar-panel h3 {
                color: #1e3a8a;
                margin-bottom: 14px;
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 15px;
                line-height: 1.2;
            }

            /* Enhanced Validation Section */
            .validation-panel h3 {
                color: #1e3a8a;
                margin-bottom: 14px;
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 15px;
            }

            .validation-summary {
                background: #f8fafc;
                border-radius: 8px;
                padding: 14px;
                margin-bottom: 16px;
            }

            .validation-stats {
                display: grid;
                grid-template-columns: 1fr 1fr;
                gap: 10px;
                margin-bottom: 10px;
            }

            .stat-item {
                text-align: center;
            }

            .stat-number {
                font-size: 22px;
                font-weight: 700;
                margin-bottom: 3px;
                line-height: 1;
            }

            .stat-number.success { color: #10b981; }
            .stat-number.error { color: #ef4444; }

            .stat-label {
                font-size: 11px;
                color: #6b7280;
                line-height: 1.2;
            }

            .validation-score {
                background: linear-gradient(135deg, #10b981 0%, #059669 100%);
                color: white;
                padding: 10px;
                border-radius: 8px;
                text-align: center;
                font-weight: 600;
                font-size: 13px;
                line-height: 1.2;
            }

            /* FHIR Metadata Section */
            .metadata-panel h3 {
                color: #1e3a8a;
                margin-bottom: 14px;
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 15px;
            }

            .metadata-item {
                background: #f8fafc;
                border-radius: 6px;
                padding: 10px;
                margin-bottom: 6px;
            }

            .metadata-label {
                font-size: 11px;
                color: #4b5563;
                margin-bottom: 3px;
                line-height: 1.2;
            }

            .metadata-value {
                font-size: 13px;
                color: #1f2937;
                font-weight: 500;
                line-height: 1.2;
            }

            /* Modal Enhancements */
            .modal-overlay {
                position: fixed;
                top: 0;
                left: 0;
                right: 0;
                bottom: 0;
                background: rgba(0, 0, 0, 0.7);
                display: none;
                align-items: center;
                justify-content: center;
                z-index: 10000;
                padding: 20px;
            }

            .modal-overlay.active, .modal-overlay.show {
                display: flex !important;
            }

            .modal-container {
                background: white;
                border-radius: 12px;
                max-width: 90vw;
                max-height: 90vh;
                overflow: hidden;
                box-shadow: 0 10px 40px rgba(0,0,0,0.3);
            }

            .modal-header {
                background: linear-gradient(135deg, #1e3a8a 0%, #3b82f6 100%);
                color: white;
                padding: 16px 20px;
                display: flex;
                justify-content: space-between;
                align-items: center;
            }

            .modal-title {
                font-size: 18px;
                font-weight: 600;
                display: flex;
                align-items: center;
                gap: 8px;
            }

            .modal-close {
                background: none;
                border: none;
                color: white;
                font-size: 20px;
                cursor: pointer;
                width: 30px;
                height: 30px;
                border-radius: 4px;
                display: flex;
                align-items: center;
                justify-content: center;
                transition: background 0.2s;
            }

            .modal-close:hover {
                background: rgba(255,255,255,0.1);
            }

            .modal-body {
                padding: 20px;
                max-height: 70vh;
                overflow-y: auto;
            }

            /* JSON Viewer */
            #jsonContent {
                background: #1a202c;
                color: #e2e8f0;
                padding: 20px;
                border-radius: 6px;
                font-family: 'Courier New', monospace;
                font-size: 14px;
                line-height: 1.5;
                overflow-x: auto;
                margin: 0;
                white-space: pre-wrap;
            }

            /* Loading States */
            .loading-state {
                display: flex;
                align-items: center;
                justify-content: center;
                padding: 40px;
                color: #6b7280;
            }

            .spinner {
                width: 20px;
                height: 20px;
                border: 2px solid #e5e7eb;
                border-top: 2px solid #3b82f6;
                border-radius: 50%;
                animation: spin 1s linear infinite;
                margin-right: 12px;
            }

            @keyframes spin {
                0% { transform: rotate(0deg); }
                100% { transform: rotate(360deg); }
            }

            /* Enhanced Responsive Design */
            @media (max-width: 1400px) {
                .main-container {
                    max-width: 100%;
                    padding: 12px;
                }
                
                .transformation-layout {
                    grid-template-columns: 1fr 280px;
                }
            }

            @media (max-width: 1200px) {
                .transformation-layout {
                    grid-template-columns: 1fr;
                    gap: 12px;
                }
                
                .sidebar-panel {
                    max-height: none;
                    padding: 16px;
                }
            }

            @media (max-width: 768px) {
                .main-container {
                    padding: 8px;
                }
                
                .step-header {
                    padding: 16px;
                    margin-bottom: 16px;
                }
                
                .step-title {
                    font-size: 24px;
                    margin-bottom: 6px;
                }
                
                .action-toolbar {
                    flex-direction: column;
                    align-items: stretch;
                    gap: 8px;
                    padding: 12px 16px;
                }
                
                .btn-group {
                    justify-content: center;
                    flex-wrap: wrap;
                }
                
                .mapping-row {
                    grid-template-columns: 1fr;
                    gap: 8px;
                    text-align: center;
                    padding: 12px;
                }
                
                .mapping-arrow {
                    transform: rotate(90deg);
                    margin: 4px 0;
                }
                
                .resource-header {
                    padding: 12px 16px;
                }
                
                .resource-content {
                    padding: 12px 16px;
                }
                
                .sidebar-panel {
                    padding: 12px;
                }
            }

            /* Focus improvements for accessibility */
            .enhanced-btn:focus, 
            #expandAllBtn:focus, #collapseAllBtn:focus, #viewJsonBtn:focus, #validateBtn:focus, #saveConfigBtn:focus,
            .resource-header:focus {
                outline: 2px solid #3b82f6;
                outline-offset: 2px;
            }

            /* Print Styles for Better Print Space Usage */
            @media print {
                * {
                    box-shadow: none !important;
                    text-shadow: none !important;
                }
                
                .main-container {
                    max-width: 100%;
                    padding: 0;
                    margin: 0;
                }
                
                .transformation-layout {
                    display: block;
                }
                
                .action-toolbar {
                    display: none;
                }
                
                .resource-content {
                    display: block !important;
                }
                
                .modal-overlay {
                    display: none !important;
                }
                
                .sidebar-panel {
                    margin-top: 20px;
                    padding: 15px;
                    border: 1px solid #ccc;
                    page-break-inside: avoid;
                }
            }

            /* Notification System */
            .notification {
                position: fixed;
                top: 20px;
                right: 20px;
                background: #3b82f6;
                color: white;
                padding: 12px 20px;
                border-radius: 6px;
                box-shadow: 0 4px 12px rgba(0,0,0,0.2);
                z-index: 10001;
                transform: translateX(100%);
                transition: transform 0.3s ease;
                font-weight: 500;
            }

            .notification.show {
                transform: translateX(0);
            }

            .notification.success {
                background: #10b981;
            }

            .notification.error {
                background: #ef4444;
            }

            .notification.warning {
                background: #f59e0b;
            }
        `;
    }
}

// Export for use
window.Step4Styles = Step4Styles;

console.log('✅ Enhanced Step4Styles loaded with working functionality');