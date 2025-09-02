// services/auditService.js - PostgreSQL only version
// Updated to be compatible with wizardController.js calls
const fs = require('fs');
const path = require('path');

class AuditService {
    constructor() {
        this.database = null;
        this.usePostgreSQL = false;
        
        try {
            this.database = require('../config/database');
            this.usePostgreSQL = true;
            console.log('PostgreSQL audit logging enabled');
        } catch (error) {
            console.error('PostgreSQL audit logging not available:', error.message);
            throw new Error('PostgreSQL is required for audit logging');
        }
        
        // Keep file logging as secondary backup only
        this.logsDir = path.join(__dirname, '..', 'logs');
        if (!fs.existsSync(this.logsDir)) {
            fs.mkdirSync(this.logsDir, { recursive: true });
        }
    }

    /**
     * Log an audit event - Compatible with wizardController.js calls
     * @param {Object} eventData - Event data object
     * @param {string} eventData.userId - User ID
     * @param {string} eventData.sessionId - Session ID
     * @param {string} eventData.action - Action performed
     * @param {string} eventData.entityType - Entity type (Interface, User, etc.)
     * @param {string} eventData.entityId - Entity ID
     * @param {Object} eventData.oldValues - Previous values
     * @param {Object} eventData.newValues - New values
     * @param {Object} eventData.metadata - Additional metadata
     * @param {string} eventData.ipAddress - Client IP
     * @param {string} eventData.userAgent - User agent
     * @param {string} eventData.requestId - Request ID
     * @param {string} eventData.result - Result (success/failure/error)
     * @param {string} eventData.riskLevel - Risk level (low/medium/high)
     * @param {string} eventData.errorMessage - Error message if applicable
     * @param {Object} eventData.complianceFlags - Compliance flags
     */
    async logEvent(eventData) {
        const auditData = {
            timestamp: new Date().toISOString(),
            user_id: eventData.userId || null,
            session_id: eventData.sessionId || null,
            action: eventData.action,
            entity_type: eventData.entityType || 'User',
            entity_id: eventData.entityId || null,
            old_values: eventData.oldValues || null,
            new_values: eventData.newValues || null,
            metadata: eventData.metadata || {},
            ip_address: eventData.ipAddress || null,
            user_agent: eventData.userAgent || null,
            request_id: eventData.requestId || null,
            result: eventData.result || 'success',
            error_message: eventData.errorMessage || null,
            risk_level: eventData.riskLevel || 'low',
            compliance_flags: eventData.complianceFlags || {}
        };

        try {
            // Primary: Log to PostgreSQL (required)
            if (this.database?.models?.AuditLog) {
                await this.database.models.AuditLog.create({
                    user_id: auditData.user_id,
                    session_id: auditData.session_id,
                    action: auditData.action,
                    entity_type: auditData.entity_type,
                    entity_id: auditData.entity_id,
                    old_values: auditData.old_values,
                    new_values: auditData.new_values,
                    metadata: auditData.metadata,
                    ip_address: auditData.ip_address,
                    user_agent: auditData.user_agent,
                    request_id: auditData.request_id,
                    result: auditData.result,
                    error_message: auditData.error_message,
                    risk_level: auditData.risk_level,
                    compliance_flags: auditData.compliance_flags
                });
                
                console.log(`[AUDIT] ${auditData.action} - ${auditData.entity_type} - ${auditData.result}`);
            } else {
                throw new Error('PostgreSQL AuditLog model not available');
            }

            // Secondary: File backup (optional)
            const logFile = path.join(this.logsDir, 'audit.log');
            try {
                fs.appendFileSync(logFile, JSON.stringify(auditData) + '\n');
            } catch (fileError) {
                console.warn('File audit logging failed (non-critical):', fileError.message);
            }

        } catch (error) {
            console.error('Critical: Audit logging failed:', error.message);
            
            // For compliance reasons, we should know when audit logging fails
            // Try to log to file as emergency backup
            try {
                const emergencyLog = path.join(this.logsDir, 'audit-failures.log');
                const failureData = {
                    timestamp: new Date().toISOString(),
                    error: error.message,
                    originalData: auditData
                };
                fs.appendFileSync(emergencyLog, JSON.stringify(failureData) + '\n');
            } catch (emergencyError) {
                console.error('Emergency audit logging also failed:', emergencyError.message);
            }
            
            // Re-throw for critical audit events
            if (auditData.risk_level === 'critical' || auditData.action.includes('LOGIN')) {
                throw new Error(`Critical audit logging failure: ${error.message}`);
            }
        }
    }

    /**
     * Original method signature for backward compatibility
     */
    async logAction(data) {
        return this.logEvent({
            userId: data.user_id,
            sessionId: data.session_id,
            action: data.action,
            entityType: data.entity_type,
            entityId: data.entity_id,
            oldValues: data.old_values,
            newValues: data.new_values,
            metadata: data.metadata,
            ipAddress: data.ip_address,
            userAgent: data.user_agent,
            riskLevel: data.risk_level,
            result: data.result,
            complianceFlags: data.compliance_flags
        });
    }
}

module.exports = new AuditService();