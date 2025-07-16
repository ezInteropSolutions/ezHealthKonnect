// services/auditService.js - PostgreSQL only version
const fs = require('fs');
const path = require('path');

class AuditService {
    constructor() {
        this.database = null;
        this.usePostgreSQL = false;
        
        try {
            this.database = require('../config/database');
            this.usePostgreSQL = true;
            console.log('✅ PostgreSQL audit logging enabled');
        } catch (error) {
            console.error('❌ PostgreSQL audit logging not available:', error.message);
            throw new Error('PostgreSQL is required for audit logging');
        }
        
        // Keep file logging as secondary backup only
        this.logsDir = path.join(__dirname, '..', 'logs');
        if (!fs.existsSync(this.logsDir)) {
            fs.mkdirSync(this.logsDir, { recursive: true });
        }
    }

    async logEvent(data) {
        const auditData = {
            timestamp: new Date().toISOString(),
            ...data
        };

        try {
            // Primary: Log to PostgreSQL (required)
            if (this.database?.models?.AuditLog) {
                await this.database.models.AuditLog.logAction({
                    user_id: data.userId || null,
                    session_id: data.sessionId || null,
                    action: data.action,
                    entity_type: data.entityType || 'User',
                    entity_id: data.entityId || null,
                    old_values: data.oldValues || null,
                    new_values: data.newValues || null,
                    metadata: data.metadata || {},
                    ip_address: data.ipAddress || null,
                    user_agent: data.userAgent || null,
                    risk_level: data.riskLevel || 'low',
                    result: data.result || 'success',
                    compliance_flags: data.complianceFlags || {}
                });
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
            console.error('❌ Critical: Audit logging failed:', error.message);
            
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
                console.error('❌ Emergency audit logging also failed:', emergencyError.message);
            }
            
            // Re-throw for critical audit events
            if (data.riskLevel === 'critical' || data.action.includes('LOGIN')) {
                throw new Error(`Critical audit logging failure: ${error.message}`);
            }
        }
    }
}

module.exports = new AuditService();