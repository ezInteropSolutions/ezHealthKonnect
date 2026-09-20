// services/auditService.js - PostgreSQL only version
// Updated to be compatible with wizardController.js calls
const fs = require('fs');
const path = require('path');
const atnaSyslog = require('./audit/atnaSyslogExporter');

// ── Shared taxonomy (services/audit/audit_events.json) ─────────────────────
// The SAME file services/audit/taxonomy.go embeds on the Go side — one
// taxonomy, read by both languages, so an action fired from Node gets the
// identical ATNA/RFC 3881 shape and HIPAA-compliance defaults as one fired
// from Go, instead of each call site inventing its own riskLevel/
// complianceFlags ad hoc (exactly the drift this whole mechanism exists to
// end). Loaded once, synchronously, at module load — this file's own
// existing require() of ../config/database already makes module init
// synchronous-and-fallible, so a sync readFileSync here is consistent, not
// a new risk.
const TAXONOMY_PATH = path.join(__dirname, 'audit', 'audit_events.json');
let TAXONOMY = {};
try {
    TAXONOMY = JSON.parse(fs.readFileSync(TAXONOMY_PATH, 'utf8'));
} catch (error) {
    console.error(`⚠️  auditService: failed to load shared taxonomy from ${TAXONOMY_PATH}: ${error.message} — every action will fall back to generic low/success defaults`);
}

// atnaOutcomeIndicator mirrors services/audit/audit_logger.go's own
// atnaOutcomeIndicator exactly (RFC 3881 EventOutcomeIndicator: 0=Success,
// 4=Minor failure, 8=Serious failure — this codebase's result vocabulary
// never reaches "Major failure" (12), so 8 is the ceiling here too).
function atnaOutcomeIndicator(result) {
    switch (result) {
        case 'success': return '0';
        case 'failure': return '4';
        case 'error': return '8';
        default: return '4';
    }
}

// lookupTaxonomy mirrors services/audit/taxonomy.go's own lookupTaxonomy:
// an action with no entry never blocks the write, it just loses its ATNA
// enrichment and gets a generic low/success fallback — logged once per call
// so the gap gets noticed, not silently swallowed.
function lookupTaxonomy(action) {
    const entry = TAXONOMY[action];
    if (entry) return entry;
    console.warn(`⚠️  [audit] action "${action}" has no taxonomy entry in services/audit/audit_events.json — using generic defaults`);
    return { defaultRiskLevel: 'low', defaultResult: 'success' };
}

// resolveEvent mirrors services/audit/audit_logger.go's own resolveEvent:
// eventData's own result/riskLevel/complianceFlags win when supplied (a
// caller that knows the real outcome differs from the action's default —
// e.g. a failure branch), the taxonomy entry supplies the default
// otherwise, and the ATNA identity fields are folded into
// metadata.atna exactly as the Go side does (audit_logs has no dedicated
// ATNA columns yet).
function resolveEvent(eventData) {
    const tax = lookupTaxonomy(eventData.action);

    const result = eventData.result || tax.defaultResult || 'success';
    const riskLevel = eventData.riskLevel || tax.defaultRiskLevel || 'low';
    const complianceFlags = eventData.complianceFlags || tax.complianceFlags || {};

    const metadata = { ...(eventData.metadata || {}) };
    if (tax.atnaEventId) {
        metadata.atna = {
            event_id: tax.atnaEventId,
            event_id_display: tax.atnaEventIdDisplay,
            event_action_code: tax.eventActionCode,
            event_outcome_indicator: atnaOutcomeIndicator(result)
        };
    }

    return { result, riskLevel, complianceFlags, metadata };
}

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
        // Shared taxonomy lookup (services/audit/audit_events.json) — supplies
        // result/riskLevel/complianceFlags defaults + ATNA metadata for
        // eventData.action when the caller doesn't override them, the same
        // action->defaults resolution services/audit/taxonomy.go implements
        // on the Go side. Explicit eventData.result/riskLevel/complianceFlags
        // still win when a caller supplies them (see resolveEvent above).
        const resolved = resolveEvent(eventData);

        // Best-effort ATNA syslog export (Node-side mirror of the Go
        // decorator in services/audit/syslog_audit_logger.go) — fire-and-
        // forget, never awaited, never allowed to affect the DB write below
        // or this function's own caller. A no-op unless ATNA_SYSLOG_HOST is
        // configured. Runs regardless of whether the DB write below
        // succeeds, same as the Go side.
        atnaSyslog.exportEvent(eventData, resolved, TAXONOMY[eventData.action]);

        const auditData = {
            timestamp: new Date().toISOString(),
            user_id: eventData.userId || null,
            session_id: eventData.sessionId || null,
            action: eventData.action,
            entity_type: eventData.entityType || 'User',
            entity_id: eventData.entityId || null,
            old_values: eventData.oldValues || null,
            new_values: eventData.newValues || null,
            metadata: resolved.metadata,
            ip_address: eventData.ipAddress || null,
            user_agent: eventData.userAgent || null,
            request_id: eventData.requestId || null,
            result: resolved.result,
            error_message: eventData.errorMessage || null,
            risk_level: resolved.riskLevel,
            compliance_flags: resolved.complianceFlags
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
                console.warn('PostgreSQL AuditLog model not available, logging to console');
                console.log('[AUDIT FALLBACK]', auditData);
            }

            // Secondary: File backup (optional)
            const logFile = path.join(this.logsDir, 'audit.log');
            try {
                fs.appendFileSync(logFile, JSON.stringify(auditData) + '\n');
            } catch (fileError) {
                console.warn('File audit logging failed (non-critical):', fileError.message);
            }

        } catch (error) {
            console.error('Audit logging failed (non-critical):', error.message);
            
            // For compliance, log to console as fallback
            console.log('[AUDIT CONSOLE FALLBACK]:', auditData);
            
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
            
            // Don't throw error for audit failures - just log and continue
            // Re-throw only for critical security events
            if (auditData.risk_level === 'critical' && auditData.action.includes('LOGIN')) {
                throw new Error(`Critical audit logging failure: ${error.message}`);
            }
        }
    }

    /**
     * logActivity method - Required by wizardController.js
     * Maps to the existing logEvent method with proper parameter transformation
     * @param {Object} activity - Activity data
     * @param {string} activity.userId - User ID
     * @param {string} activity.action - Action performed
     * @param {string} activity.resource - Resource type
     * @param {string} activity.resourceId - Resource ID
     * @param {string} activity.details - Activity details
     * @param {Object} activity.metadata - Additional metadata
     */
    async logActivity(activity) {
        return this.logEvent({
            userId: activity.userId,
            action: activity.action,
            entityType: activity.resource || 'interface',
            entityId: activity.resourceId,
            metadata: {
                details: activity.details,
                ...(activity.metadata || {})
            },
            result: 'success',
            riskLevel: 'low'
        });
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

// Exported for unit testing only (tests/unit/services/auditService.taxonomy.test.js).
// Not part of the AuditService's public API — mirrors
// controllers/pipelineController.js's own "exported for unit testing only"
// convention for its analogous pure helpers.
module.exports.resolveEvent = resolveEvent;
module.exports.lookupTaxonomy = lookupTaxonomy;
module.exports.atnaOutcomeIndicator = atnaOutcomeIndicator;
module.exports.TAXONOMY = TAXONOMY;