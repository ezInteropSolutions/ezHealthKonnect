// js/modules/validation-integration.js
// Integration module that ties together all validation components for ezHealthKonnect

import { HL7Validator } from './hl7-validator.js';
import { HL7Schemas } from './hl7-schemas.js';
import { HealthcareRules } from './healthcare-rules.js';
import { ValidationUI } from './validation-ui.js';

class ValidationIntegration {
    constructor(options = {}) {
        this.options = {
            enableRealTimeValidation: true,
            autoExpandErrors: true,
            showSuggestions: true,
            enableDrillDown: true,
            customRules: [],
            customSchemas: [],
            ...options
        };

        this.schemas = new HL7Schemas();
        this.healthcareRules = new HealthcareRules();
        this.validator = new HL7Validator(this.schemas, this.healthcareRules);
        this.ui = null;
        this.currentMessage = null;
        this.validationCache = new Map();
        
        this.initializeCustomizations();
    }

    /**
     * Initialize the validation system with UI container
     */
    initialize(containerId) {
        this.ui = new ValidationUI(containerId, this.validator);
        
        // Set up event listeners
        this.setupEventListeners();
        
        // Apply initial options
        this.applyOptions();
        
        return this;
    }

    /**
     * Validate an HL7 message and display results
     */
    async validateMessage(messageText, messageType = null) {
        try {
            // Parse the HL7 message
            const parsedMessage = this.parseHL7Message(messageText);
            
            // Auto-detect message type if not provided
            if (!messageType) {
                messageType = this.detectMessageType(parsedMessage);
            }
            
            // Check cache first
            const cacheKey = this.generateCacheKey(messageText, messageType);
            if (this.validationCache.has(cacheKey)) {
                const cachedResults = this.validationCache.get(cacheKey);
                this.displayResults(cachedResults);
                return cachedResults;
            }
            
            // Perform validation
            const validationResults = this.validator.validateMessage(parsedMessage, messageType);
            
            // Enhance results with additional information
            const enhancedResults = this.enhanceValidationResults(validationResults, parsedMessage);
            
            // Cache results
            this.validationCache.set(cacheKey, enhancedResults);
            
            // Display results
            this.displayResults(enhancedResults);
            
            // Store current message for drill-down operations
            this.currentMessage = parsedMessage;
            
            return enhancedResults;
            
        } catch (error) {
            console.error('Validation error:', error);
            const errorResult = {
                isValid: false,
                error: error.message,
                summary: { totalFields: 0, validFields: 0, errors: 1, warnings: 0, missing: 0 }
            };
            
            this.displayResults(errorResult);
            return errorResult;
        }
    }

    /**
     * Validate message in real-time as user types
     */
    validateRealTime(messageText, messageType = null, debounceMs = 500) {
        if (!this.options.enableRealTimeValidation) return;
        
        // Clear existing timeout
        if (this.realTimeTimeout) {
            clearTimeout(this.realTimeTimeout);
        }
        
        // Set new timeout for debounced validation
        this.realTimeTimeout = setTimeout(() => {
            this.validateMessage(messageText, messageType);
        }, debounceMs);
    }

    /**
     * Parse HL7 message text into structured object
     */
    parseHL7Message(messageText) {
        if (!messageText || typeof messageText !== 'string') {
            throw new Error('Invalid message text provided');
        }
        
        const lines = messageText.trim().split(/\r?\n/);
        const parsed = {};
        
        for (const line of lines) {
            if (!line.trim()) continue;
            
            const segments = line.split('|');
            const segmentType = segments[0];
            
            if (!segmentType) continue;
            
            // Create segment object
            const segmentData = {};
            segments.forEach((field, index) => {
                if (index > 0) { // Skip segment type
                    segmentData[index.toString()] = field;
                }
            });
            
            // Handle repeating segments
            if (parsed[segmentType]) {
                if (!Array.isArray(parsed[segmentType])) {
                    parsed[segmentType] = [parsed[segmentType]];
                }
                parsed[segmentType].push(segmentData);
            } else {
                parsed[segmentType] = segmentData;
            }
        }
        
        return parsed;
    }

    /**
     * Auto-detect message type from parsed message
     */
    detectMessageType(parsedMessage) {
        const msh = parsedMessage.MSH;
        if (!msh || !msh['9']) {
            return 'ADT_A01'; // Default fallback
        }
        
        const msgType = msh['9'];
        
        // Handle composite message type (MSG^EVENT^STRUCTURE)
        if (msgType.includes('^')) {
            const parts = msgType.split('^');
            return `${parts[0]}_${parts[1]}`;
        }
        
        return msgType.replace('^', '_');
    }

    /**
     * Enhance validation results with additional context
     */
    enhanceValidationResults(results, parsedMessage) {
        const enhanced = { ...results };
        
        // Add message statistics
        enhanced.messageStats = this.calculateMessageStats(parsedMessage);
        
        // Add segment analysis
        enhanced.segmentAnalysis = this.analyzeSegments(parsedMessage);
        
        // Add compliance score
        enhanced.complianceScore = this.calculateComplianceScore(results);
        
        // Add recommendations
        enhanced.recommendations = this.generateRecommendations(results, parsedMessage);
        
        // Add quick fixes
        enhanced.quickFixes = this.generateQuickFixes(results);
        
        return enhanced;
    }

    /**
     * Calculate message statistics
     */
    calculateMessageStats(parsedMessage) {
        const stats = {
            totalSegments: 0,
            uniqueSegmentTypes: new Set(),
            totalFields: 0,
            populatedFields: 0,
            emptyFields: 0,
            repeatingSegments: 0
        };
        
        for (const [segmentType, segmentData] of Object.entries(parsedMessage)) {
            stats.uniqueSegmentTypes.add(segmentType);
            
            if (Array.isArray(segmentData)) {
                stats.totalSegments += segmentData.length;
                stats.repeatingSegments++;
                
                segmentData.forEach(segment => {
                    const fieldCount = Object.keys(segment).length;
                    stats.totalFields += fieldCount;
                    stats.populatedFields += Object.values(segment).filter(v => v && v.trim()).length;
                    stats.emptyFields += Object.values(segment).filter(v => !v || !v.trim()).length;
                });
            } else {
                stats.totalSegments++;
                const fieldCount = Object.keys(segmentData).length;
                stats.totalFields += fieldCount;
                stats.populatedFields += Object.values(segmentData).filter(v => v && v.trim()).length;
                stats.emptyFields += Object.values(segmentData).filter(v => !v || !v.trim()).length;
            }
        }
        
        stats.populationRate = stats.totalFields > 0 ? 
            Math.round((stats.populatedFields / stats.totalFields) * 100) : 0;
        
        return stats;
    }

    /**
     * Analyze segments for completeness
     */
    analyzeSegments(parsedMessage) {
        const analysis = {};
        
        for (const [segmentType, segmentData] of Object.entries(parsedMessage)) {
            const segments = Array.isArray(segmentData) ? segmentData : [segmentData];
            
            analysis[segmentType] = {
                count: segments.length,
                isRepeating: Array.isArray(segmentData),
                completeness: this.calculateSegmentCompleteness(segments),
                criticalFields: this.identifyCriticalFields(segmentType, segments)
            };
        }
        
        return analysis;
    }

    /**
     * Calculate segment completeness percentage
     */
    calculateSegmentCompleteness(segments) {
        let totalFields = 0;
        let populatedFields = 0;
        
        segments.forEach(segment => {
            totalFields += Object.keys(segment).length;
            populatedFields += Object.values(segment).filter(v => v && v.trim()).length;
        });
        
        return totalFields > 0 ? Math.round((populatedFields / totalFields) * 100) : 0;
    }

    /**
     * Identify critical fields in segments
     */
    identifyCriticalFields(segmentType, segments) {
        const criticalFieldMap = {
            'MSH': ['3', '4', '5', '6', '7', '9', '10', '11', '12'],
            'PID': ['3', '5', '7', '8'],
            'PV1': ['2', '3', '44'],
            'OBR': ['3', '4', '7'],
            'OBX': ['3', '5', '11']
        };
        
        const criticalFields = criticalFieldMap[segmentType] || [];
        const missing = [];
        const present = [];
        
        segments.forEach((segment, index) => {
            criticalFields.forEach(fieldNum => {
                const fieldValue = segment[fieldNum];
                const fieldPath = segments.length > 1 ? `${segmentType}[${index}].${fieldNum}` : `${segmentType}.${fieldNum}`;
                
                if (!fieldValue || !fieldValue.trim()) {
                    missing.push(fieldPath);
                } else {
                    present.push(fieldPath);
                }
            });
        });
        
        return { missing, present, total: criticalFields.length * segments.length };
    }

    /**
     * Calculate overall compliance score
     */
    calculateComplianceScore(results) {
        const summary = results.summary;
        const total = summary.totalFields;
        
        if (total === 0) return 0;
        
        // Weight errors more heavily than warnings
        const errorPenalty = summary.errors * 2;
        const warningPenalty = summary.warnings * 1;
        const missingPenalty = summary.missing * 1.5;
        
        const totalPenalty = errorPenalty + warningPenalty + missingPenalty;
        const maxPossiblePenalty = total * 2; // If everything was an error
        
        const score = Math.max(0, Math.round(((maxPossiblePenalty - totalPenalty) / maxPossiblePenalty) * 100));
        
        return {
            score,
            grade: this.getComplianceGrade(score),
            breakdown: {
                errors: summary.errors,
                warnings: summary.warnings,
                missing: summary.missing,
                valid: summary.validFields
            }
        };
    }

    /**
     * Get compliance grade based on score
     */
    getComplianceGrade(score) {
        if (score >= 95) return 'A+';
        if (score >= 90) return 'A';
        if (score >= 85) return 'B+';
        if (score >= 80) return 'B';
        if (score >= 75) return 'C+';
        if (score >= 70) return 'C';
        if (score >= 60) return 'D';
        return 'F';
    }

    /**
     * Generate recommendations based on validation results
     */
    generateRecommendations(results, parsedMessage) {
        const recommendations = [];
        
        // High-level recommendations based on error patterns
        if (results.summary.missing > 0) {
            recommendations.push({
                type: 'missing_fields',
                priority: 'high',
                title: 'Complete Missing Required Fields',
                description: `${results.summary.missing} required fields are missing. Complete these to improve message validity.`,
                action: 'Review and populate all required fields marked as missing.'
            });
        }
        
        if (results.summary.errors > 5) {
            recommendations.push({
                type: 'data_quality',
                priority: 'high',
                title: 'Address Data Quality Issues',
                description: 'Multiple data quality issues detected. Review field formats and values.',
                action: 'Use validation-friendly test data and verify field formats match HL7 specifications.'
            });
        }
        
        if (results.typeViolations && results.typeViolations.length > 0) {
            recommendations.push({
                type: 'data_types',
                priority: 'medium',
                title: 'Fix Data Type Violations',
                description: 'Some fields contain data that doesn\'t match their expected data types.',
                action: 'Verify numeric fields contain only numbers, dates follow HL7 format, etc.'
            });
        }
        
        if (results.bindingDeviations && results.bindingDeviations.length > 0) {
            recommendations.push({
                type: 'code_values',
                priority: 'medium',
                title: 'Review Code Values',
                description: 'Some coded fields use values not found in standard code tables.',
                action: 'Use standard HL7 code values or verify custom codes are properly defined.'
            });
        }
        
        // Message-specific recommendations
        const messageType = results.messageType;
        if (messageType && messageType.startsWith('ADT')) {
            if (!parsedMessage.PV1) {
                recommendations.push({
                    type: 'message_structure',
                    priority: 'high',
                    title: 'Add Patient Visit Information',
                    description: 'ADT messages should include PV1 segment for visit information.',
                    action: 'Add PV1 segment with patient class, location, and attending physician.'
                });
            }
        }
        
        return recommendations;
    }

    /**
     * Generate quick fix suggestions
     */
    generateQuickFixes(results) {
        const fixes = [];
        
        // Auto-fixable issues
        results.missingRequired?.forEach(item => {
            if (this.canAutoFix(item)) {
                fixes.push({
                    type: 'auto_populate',
                    field: item.path,
                    description: `Auto-populate ${item.path} with default value`,
                    action: () => this.autoPopulateField(item.path),
                    confidence: 'high'
                });
            }
        });
        
        results.typeViolations?.forEach(item => {
            if (item.type === 'INVALID_TIMESTAMP' && this.canFixTimestamp(item.value)) {
                fixes.push({
                    type: 'format_fix',
                    field: item.path,
                    description: `Fix timestamp format for ${item.path}`,
                    suggestion: this.suggestTimestampFix(item.value),
                    confidence: 'medium'
                });
            }
        });
        
        return fixes;
    }

    /**
     * Display validation results using UI
     */
    displayResults(results) {
        if (this.ui) {
            this.ui.displayResults(results);
            
            // Auto-expand error sections if enabled
            if (this.options.autoExpandErrors && results.summary.errors > 0) {
                setTimeout(() => {
                    this.ui.expandAll();
                }, 100);
            }
        }
        
        // Emit validation complete event
        this.emitEvent('validationComplete', { results });
    }

    /**
     * Set up event listeners
     */
    setupEventListeners() {
        if (this.ui && this.ui.container) {
            // Listen for suggestion applications
            this.ui.container.addEventListener('validationSuggestionApplied', (event) => {
                this.handleSuggestionApplied(event.detail.suggestion);
            });
            
            // Listen for drill-down requests
            this.ui.container.addEventListener('drillDownRequested', (event) => {
                this.handleDrillDownRequest(event.detail.path);
            });
        }
    }

    /**
     * Handle suggestion application
     */
    handleSuggestionApplied(suggestion) {
        this.emitEvent('suggestionApplied', { suggestion });
        
        // Could trigger re-validation if needed
        if (this.options.enableRealTimeValidation && this.currentMessage) {
            // Re-validate after suggestion application
            setTimeout(() => {
                this.validateMessage(this.currentMessage);
            }, 100);
        }
    }

    /**
     * Handle drill-down requests
     */
    handleDrillDownRequest(fieldPath) {
        if (this.validator && this.validator.getFieldDrillDown) {
            const drillDownInfo = this.validator.getFieldDrillDown(fieldPath);
            this.emitEvent('drillDownInfo', { path: fieldPath, info: drillDownInfo });
        }
    }

    /**
     * Initialize custom rules and schemas
     */
    initializeCustomizations() {
        // Add custom schemas
        this.options.customSchemas.forEach(schema => {
            this.schemas.addSchema(schema.messageType, schema.definition);
        });
        
        // Add custom rules
        this.options.customRules.forEach(rule => {
            this.healthcareRules.addRule(rule.id, rule.definition);
        });
    }

    /**
     * Apply configuration options
     */
    applyOptions() {
        if (this.ui) {
            // Configure UI based on options
            if (!this.options.showSuggestions) {
                // Hide suggestion elements
                const style = document.createElement('style');
                style.textContent = '.suggestions { display: none !important; }';
                document.head.appendChild(style);
            }
            
            if (!this.options.enableDrillDown) {
                // Hide drill-down buttons
                const style = document.createElement('style');
                style.textContent = '.drill-down-btn { display: none !important; }';
                document.head.appendChild(style);
            }
        }
    }

    /**
     * Utility methods
     */
    
    generateCacheKey(messageText, messageType) {
        const hash = this.simpleHash(messageText + messageType);
        return `${messageType}_${hash}`;
    }
    
    simpleHash(str) {
        let hash = 0;
        for (let i = 0; i < str.length; i++) {
            const char = str.charCodeAt(i);
            hash = ((hash << 5) - hash) + char;
            hash = hash & hash; // Convert to 32-bit integer
        }
        return Math.abs(hash).toString(36);
    }
    
    canAutoFix(item) {
        // Determine if an issue can be automatically fixed
        const autoFixableTypes = ['MISSING_REQUIRED'];
        return autoFixableTypes.includes(item.type);
    }
    
    canFixTimestamp(value) {
        // Check if timestamp can be auto-corrected
        return /^\d{4}-\d{2}-\d{2}/.test(value); // ISO date format
    }
    
    suggestTimestampFix(value) {
        // Suggest HL7 timestamp format
        const date = new Date(value);
        if (!isNaN(date.getTime())) {
            return date.toISOString().replace(/[-:T]/g, '').slice(0, 14);
        }
        return null;
    }
    
    autoPopulateField(fieldPath) {
        // Generate default value for field
        const defaults = {
            'MSH.7': new Date().toISOString().replace(/[-:T]/g, '').slice(0, 14),
            'MSH.10': `MSG${Date.now()}`,
            'MSH.11': 'P',
            'MSH.12': '2.5'
        };
        
        return defaults[fieldPath] || '';
    }

    emitEvent(eventName, detail) {
        if (typeof window !== 'undefined' && window.dispatchEvent) {
            const event = new CustomEvent(`ezHealthKonnect:${eventName}`, { detail });
            window.dispatchEvent(event);
        }
    }

    /**
     * Public API methods
     */
    
    // Clear validation cache
    clearCache() {
        this.validationCache.clear();
    }
    
    // Get cache statistics
    getCacheStats() {
        return {
            size: this.validationCache.size,
            keys: Array.from(this.validationCache.keys())
        };
    }
    
    // Export validation report
    exportReport(format = 'json') {
        if (!this.ui || !this.ui.currentResults) return null;
        
        switch (format) {
            case 'json':
                return this.ui.exportResults();
            case 'csv':
                return this.exportToCSV(this.ui.currentResults);
            case 'html':
                return this.exportToHTML(this.ui.currentResults);
            default:
                return null;
        }
    }
    
    exportToCSV(results) {
        // Implementation for CSV export
        const rows = [['Type', 'Severity', 'Path', 'Message', 'Value']];
        
        const addItems = (items, type) => {
            items?.forEach(item => {
                rows.push([type, item.severity, item.path, item.message, item.value || '']);
            });
        };
        
        addItems(results.missingRequired, 'Missing Required');
        addItems(results.typeViolations, 'Type Violation');
        addItems(results.bindingDeviations, 'Binding Deviation');
        addItems(results.healthcareViolations, 'Healthcare Rule');
        
        return rows.map(row => row.map(cell => `"${cell}"`).join(',')).join('\n');
    }
    
    exportToHTML(results) {
        // Implementation for HTML export
        return `
            <!DOCTYPE html>
            <html>
            <head>
                <title>HL7 Validation Report</title>
                <style>
                    body { font-family: Arial, sans-serif; margin: 20px; }
                    .summary { background: #f8f9fa; padding: 15px; border-radius: 5px; margin-bottom: 20px; }
                    .violation { margin-bottom: 10px; padding: 10px; border-left: 4px solid #dc3545; background: #f8f9fa; }
                    .warning { border-left-color: #ffc107; }
                </style>
            </head>
            <body>
                <h1>HL7 Validation Report</h1>
                <div class="summary">
                    <h2>Summary</h2>
                    <p>Total Fields: ${results.summary.totalFields}</p>
                    <p>Valid Fields: ${results.summary.validFields}</p>
                    <p>Errors: ${results.summary.errors}</p>
                    <p>Warnings: ${results.summary.warnings}</p>
                </div>
                ${this.buildHTMLViolations(results)}
            </body>
            </html>
        `;
    }
    
    buildHTMLViolations(results) {
        let html = '';
        const sections = [
            { title: 'Missing Required Fields', items: results.missingRequired },
            { title: 'Type Violations', items: results.typeViolations },
            { title: 'Binding Deviations', items: results.bindingDeviations },
            { title: 'Healthcare Violations', items: results.healthcareViolations }
        ];
        
        sections.forEach(section => {
            if (section.items && section.items.length > 0) {
                html += `<h2>${section.title}</h2>`;
                section.items.forEach(item => {
                    const cssClass = item.severity === 'error' ? 'violation' : 'violation warning';
                    html += `
                        <div class="${cssClass}">
                            <strong>${item.path || 'N/A'}</strong>: ${item.message}
                            ${item.value ? `<br><em>Value: ${item.value}</em>` : ''}
                        </div>
                    `;
                });
            }
        });
        
        return html;
    }
    
    // Update configuration
    updateOptions(newOptions) {
        Object.assign(this.options, newOptions);
        this.applyOptions();
    }
    
    // Get current configuration
    getOptions() {
        return { ...this.options };
    }
    
    // Destroy instance and clean up
    destroy() {
        if (this.realTimeTimeout) {
            clearTimeout(this.realTimeTimeout);
        }
        
        this.clearCache();
        
        if (this.ui) {
            this.ui.clear();
        }
    }
}

// Export for use in applications
export { ValidationIntegration };