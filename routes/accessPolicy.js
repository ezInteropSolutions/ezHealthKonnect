'use strict';
// routes/accessPolicy.js — Settings -> Access policy.
//
//   GET /api/access-policy   effective policy per configurable role + the allowed choices (admin only)
//   PUT /api/access-policy   save it (admin only, audited)
//
// What the values mean and their defaults live in services/AccessPolicyService.js; this file only validates,
// stores and audits. Stored in system_settings under "access_policy", the same table every other setting uses.
// Responses use the { success, data } / { success: false, error } shape the Settings page helpers expect.

const express = require('express');
const router = express.Router();

const { requireAuth, requireAdmin } = require('../middleware/auth');
const auditService = require('../services/auditService');
const settingsService = require('../services/settingsService');
const accessPolicy = require('../services/AccessPolicyService');

const { INTERFACE_SCOPES, MESSAGE_CONTENT_LEVELS, DEFAULT_ROLE_POLICIES, ALL_INTERFACES_ROLES, SETTING_KEY } = accessPolicy;
const CONFIGURABLE_ROLES = Object.keys(DEFAULT_ROLE_POLICIES);

const fail = (res, status, error) => res.status(status).json({ success: false, error, message: error });

router.get('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        res.json({
            success: true,
            data: {
                roles: await accessPolicy.effectiveRoles(),
                options: { interfaceScope: INTERFACE_SCOPES, messageContent: MESSAGE_CONTENT_LEVELS },
            },
        });
    } catch (error) {
        console.error('❌ Access policy read error:', error);
        fail(res, 500, 'Failed to read the access policy');
    }
});

router.put('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        const incoming = (req.body && req.body.roles) || {};
        const roles = {};
        for (const role of CONFIGURABLE_ROLES) {
            const p = incoming[role];
            if (!p) return fail(res, 400, `The policy for "${role}" is missing`);
            if (!INTERFACE_SCOPES.includes(p.interfaceScope)) {
                return fail(res, 400, `"${role}": interfaces must be one of ${INTERFACE_SCOPES.join(', ')}`);
            }
            if (ALL_INTERFACES_ROLES.includes(role) && p.interfaceScope !== 'all') {
                return fail(res, 400, `"${role}" always sees all interfaces; only message content can be restricted`);
            }
            if (!MESSAGE_CONTENT_LEVELS.includes(p.messageContent)) {
                return fail(res, 400, `"${role}": message content must be one of ${MESSAGE_CONTENT_LEVELS.join(', ')}`);
            }
            roles[role] = { interfaceScope: p.interfaceScope, messageContent: p.messageContent };
        }

        const before = await accessPolicy.effectiveRoles();
        const { sequelize } = require('../config/database');
        await sequelize.query(
            `INSERT INTO system_settings (key, value, updated_by, updated_at)
             VALUES (:key, :value::jsonb, :by, NOW())
             ON CONFLICT (key) DO UPDATE
               SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
            { replacements: { key: SETTING_KEY, value: JSON.stringify({ roles }), by: req.session.user.email } }
        );
        settingsService.invalidate(SETTING_KEY);

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'ACCESS_POLICY_CHANGED',
            entityType: 'Settings',
            entityId: SETTING_KEY,
            oldValues: before,
            newValues: roles,
            metadata: { changedBy: req.session.user.email },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'high',
        });

        res.json({ success: true, data: { roles: await accessPolicy.effectiveRoles() } });
    } catch (error) {
        console.error('❌ Access policy save error:', error);
        fail(res, 500, 'Failed to save the access policy');
    }
});

module.exports = router;
