// routes/users.js
const bcrypt = require('bcryptjs');
const crypto = require('crypto');
const express = require('express');
const router = express.Router();

const userService = require('../services/userService');
const auditService = require('../services/auditService');
const { requireAuth, requireAdmin } = require('../middleware/auth');

// Debug route to test if users routes are working
router.get('/test', (req, res) => {
    res.json({ message: 'Users routes are working!' });
});

// GET /api/users - List users with server-side search, filter, sort and pagination (admin only)
// Query params: search, role, status, sortBy, sortDir, page, limit
router.get('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        const sequelize = userService.database.sequelize;

        const search   = (req.query.search  || '').trim();
        const role     = (req.query.role    || '').trim();
        const status   = (req.query.status  || '').trim();
        const page     = Math.max(1, parseInt(req.query.page  || '1',  10));
        const limit    = Math.min(200, Math.max(1, parseInt(req.query.limit || '100', 10)));
        const offset   = (page - 1) * limit;

        const ALLOWED_SORT = {
            name:      "LOWER(COALESCE(first_name,'') || ' ' || COALESCE(last_name,''))",
            email:     'email',
            role:      'role',
            status:    'status',
            lastLogin: 'last_login_at',
            createdAt: 'created_at'
        };
        const sortKey = ALLOWED_SORT[req.query.sortBy] || ALLOWED_SORT.name;
        const sortDir = req.query.sortDir === 'desc' ? 'DESC' : 'ASC';

        // Build WHERE conditions
        const conditions = [];
        const replacements = {};

        if (search) {
            conditions.push(`(LOWER(COALESCE(first_name,'')) || ' ' || LOWER(COALESCE(last_name,'')) || ' ' || LOWER(email) LIKE :search)`);
            replacements.search = `%${search.toLowerCase()}%`;
        }
        if (role)   { conditions.push('role   = :role');   replacements.role   = role; }
        if (status) { conditions.push('status = :status'); replacements.status = status; }

        const where = conditions.length ? `WHERE ${conditions.join(' AND ')}` : '';

        const countSql = `SELECT COUNT(*) AS total FROM users ${where}`;
        const dataSql  = `
            SELECT id, email, first_name, last_name, role, status, department,
                   last_login_at, last_login_ip, login_attempts, locked_until,
                   force_password_reset, email_verified, created_at, updated_at
            FROM users
            ${where}
            ORDER BY ${sortKey} ${sortDir}
            LIMIT :limit OFFSET :offset
        `;

        replacements.limit  = limit;
        replacements.offset = offset;

        const [[countRow], users] = await Promise.all([
            sequelize.query(countSql, { replacements, type: sequelize.QueryTypes.SELECT }),
            sequelize.query(dataSql,  { replacements, type: sequelize.QueryTypes.SELECT })
        ]);

        const total = parseInt(countRow?.total || 0, 10);

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USERS_VIEWED',
            entityType: 'User',
            metadata: { total, page, limit, search: search || null, role: role || null, status: status || null },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'low'
        });

        // Return wrapped object so frontend can read total for pagination
        res.json({ users, total, page, limit, pages: Math.ceil(total / limit) });

    } catch (error) {
        console.error('❌ Error fetching users:', error);
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USERS_VIEW_FAILED',
            entityType: 'User',
            metadata: { error: error.message },
            ipAddress: req.clientIP,
            result: 'error',
            riskLevel: 'medium'
        });
        res.status(500).json({ message: 'Failed to fetch users', error: process.env.NODE_ENV === 'development' ? error.message : 'Internal server error' });
    }
});

// POST /api/users - Create new user (admin only)
router.post('/', requireAuth, requireAdmin, async (req, res) => {
    try {
        console.log('📝 Creating new user:', req.body);
        
        const { email, password, name, role } = req.body;
        
        // Validate input
        if (!email || !password || !name || !role) {
            return res.status(400).json({ message: 'All fields are required' });
        }
        
        if (!['admin', 'user'].includes(role)) {
            return res.status(400).json({ message: 'Invalid role. Must be admin or user' });
        }
        
        const newUser = await userService.createUser(
            { email, password, name, role },
            req.session.user.email
        );
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_CREATED',
            entityType: 'User',
            entityId: newUser.id,
            metadata: { 
                email: newUser.email,
                role: newUser.role,
                createdBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium',
            complianceFlags: { user_management: true }
        });
        
        console.log('✅ User created successfully:', newUser.email);
        
        res.status(201).json({
            message: 'User created successfully',
            user: newUser
        });
        
    } catch (error) {
        console.error('❌ Error creating user:', error);
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_CREATE_FAILED',
            entityType: 'User',
            metadata: { 
                error: error.message,
                attempted_email: req.body.email
            },
            ipAddress: req.clientIP,
            result: 'error',
            riskLevel: 'high'
        });
        
        const statusCode = error.message.includes('already exists') ? 400 : 500;
        res.status(statusCode).json({ 
            message: error.message.includes('already exists') ? error.message : 'Failed to create user'
        });
    }
});

// PUT /api/users/:id - Update user (admin only)
router.put('/:id', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        const { email, name, role, password } = req.body;
        
        console.log(`📝 Updating user ${id}:`, { email, name, role });
        
        // Find the user first
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        // Prepare update data
        const updateData = {};
        if (email) updateData.email = email.toLowerCase();
        if (name) {
            const [firstName, ...lastNameParts] = name.split(' ');
            updateData.first_name = firstName;
            updateData.last_name = lastNameParts.join(' ') || '';
        }
        if (role) updateData.role = role;
        if (password) {
            updateData.password_hash = await bcrypt.hash(password, 12);
        }
        
        await user.update(updateData);
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_UPDATED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                updatedFields: Object.keys(updateData),
                updatedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });
        
        res.json({ message: 'User updated successfully' });
        
    } catch (error) {
        console.error('❌ Error updating user:', error);
        res.status(500).json({ message: 'Failed to update user' });
    }
});

// PATCH /api/users/:id/status - Toggle user status (admin only)
router.patch('/:id/status', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        const { status } = req.body;
        
        if (!['active', 'inactive'].includes(status)) {
            return res.status(400).json({ message: 'Invalid status' });
        }
        
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        await user.update({ status });
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_STATUS_CHANGED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                newStatus: status,
                updatedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });
        
        res.json({ message: `User ${status === 'active' ? 'activated' : 'deactivated'} successfully` });
        
    } catch (error) {
        console.error('❌ Error updating user status:', error);
        res.status(500).json({ message: 'Failed to update user status' });
    }
});

// GET /api/users/stats - User statistics (admin only)
router.get('/stats', requireAuth, requireAdmin, async (req, res) => {
    try {
        const sequelize = userService.database.sequelize;
        const now = new Date().toISOString();

        const [rows] = await sequelize.query(`
            SELECT
                COUNT(*)                                                             AS total,
                COUNT(*) FILTER (WHERE status = 'active')                           AS active,
                COUNT(*) FILTER (WHERE status = 'inactive')                         AS inactive,
                COUNT(*) FILTER (WHERE status = 'suspended')                        AS suspended,
                COUNT(*) FILTER (WHERE status = 'pending')                          AS pending,
                COUNT(*) FILTER (WHERE role   = 'admin')                            AS admins,
                COUNT(*) FILTER (WHERE locked_until IS NOT NULL AND locked_until > :now) AS locked
            FROM users
        `, { replacements: { now }, type: sequelize.QueryTypes.SELECT });

        const r = rows || {};
        res.json({
            total:     parseInt(r.total     || 0, 10),
            active:    parseInt(r.active    || 0, 10),
            inactive:  parseInt(r.inactive  || 0, 10),
            suspended: parseInt(r.suspended || 0, 10),
            pending:   parseInt(r.pending   || 0, 10),
            admins:    parseInt(r.admins    || 0, 10),
            locked:    parseInt(r.locked    || 0, 10)
        });
    } catch (error) {
        console.error('❌ Error fetching user stats:', error);
        res.status(500).json({ message: 'Failed to fetch user stats', detail: error.message });
    }
});

// POST /api/users/bulk - Bulk operations (admin only)
router.post('/bulk', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { action, userIds } = req.body;
        if (!action || !Array.isArray(userIds) || userIds.length === 0) {
            return res.status(400).json({ message: 'action and userIds[] required' });
        }
        if (!['activate', 'deactivate', 'suspend', 'delete'].includes(action)) {
            return res.status(400).json({ message: 'Invalid bulk action' });
        }

        const User = userService.database.models.User;
        const { Op } = require('sequelize');

        // Prevent acting on self
        const safeIds = userIds.filter(id => id !== req.session.user.id);

        if (action === 'delete') {
            await User.destroy({ where: { id: { [Op.in]: safeIds } } });
        } else {
            const statusMap = { activate: 'active', deactivate: 'inactive', suspend: 'suspended' };
            await User.update({ status: statusMap[action] }, { where: { id: { [Op.in]: safeIds } } });
        }

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USERS_BULK_ACTION',
            entityType: 'User',
            metadata: { action, count: safeIds.length, userIds: safeIds, performedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: action === 'delete' ? 'high' : 'medium'
        });

        res.json({ message: `Bulk ${action} completed for ${safeIds.length} user(s)` });
    } catch (error) {
        console.error('❌ Bulk action error:', error);
        res.status(500).json({ message: 'Bulk action failed' });
    }
});

// POST /api/users/invite - Generate invitation link (admin only)
router.post('/invite', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { email, role } = req.body;
        if (!email || !role) {
            return res.status(400).json({ message: 'email and role are required' });
        }
        if (!['admin', 'operator', 'viewer', 'user'].includes(role)) {
            return res.status(400).json({ message: 'Invalid role' });
        }

        const User = userService.database.models.User;

        // Check if user already exists and is active
        const existing = await User.findOne({ where: { email: email.toLowerCase() } });
        if (existing && existing.status !== 'pending') {
            return res.status(400).json({ message: 'A user with this email already exists' });
        }

        const token = crypto.randomBytes(32).toString('hex');
        const expires = new Date(Date.now() + 48 * 60 * 60 * 1000); // 48 hours

        if (existing) {
            // Re-invite pending user
            await existing.update({
                email_verification_token: token,
                invitation_expires_at: expires,
                invited_by: req.session.user.id,
                role
            });
        } else {
            // Create pending user placeholder
            await User.create({
                email: email.toLowerCase(),
                password_hash: crypto.randomBytes(32).toString('hex'), // unusable placeholder
                first_name: '',
                last_name: '',
                role,
                status: 'pending',
                email_verified: false,
                email_verification_token: token,
                invitation_expires_at: expires,
                invited_by: req.session.user.id
            });
        }

        const inviteLink = `${req.protocol}://${req.get('host')}/invite.html?token=${token}`;

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_INVITED',
            entityType: 'User',
            metadata: { email, role, invitedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });

        res.json({ message: 'Invitation created', inviteLink, expiresAt: expires });
    } catch (error) {
        console.error('❌ Invite error:', error);
        res.status(500).json({ message: 'Failed to create invitation' });
    }
});

// POST /api/users/accept-invite - Accept invitation and set password (public)
router.post('/accept-invite', async (req, res) => {
    try {
        const { token, firstName, lastName, password } = req.body;
        if (!token || !firstName || !password) {
            return res.status(400).json({ message: 'token, firstName and password are required' });
        }
        if (password.length < 8) {
            return res.status(400).json({ message: 'Password must be at least 8 characters' });
        }

        const User = userService.database.models.User;
        const { Op } = require('sequelize');

        const user = await User.findOne({
            where: {
                email_verification_token: token,
                status: 'pending',
                invitation_expires_at: { [Op.gt]: new Date() }
            }
        });

        if (!user) {
            return res.status(400).json({ message: 'Invalid or expired invitation token' });
        }

        const passwordHash = await bcrypt.hash(password, 12);
        await user.update({
            first_name: firstName,
            last_name: lastName || '',
            password_hash: passwordHash,
            status: 'active',
            email_verified: true,
            email_verification_token: null,
            invitation_expires_at: null,
            data_consent_given: true,
            data_consent_date: new Date()
        });

        await auditService.logEvent({
            userId: user.id,
            action: 'USER_INVITE_ACCEPTED',
            entityType: 'User',
            metadata: { email: user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'low'
        });

        res.json({ message: 'Account activated successfully. You can now log in.' });
    } catch (error) {
        console.error('❌ Accept invite error:', error);
        res.status(500).json({ message: 'Failed to activate account' });
    }
});

// GET /api/users/:id/activity - User's audit log entries (admin only)
// NOTE: Must be registered BEFORE GET /:id to prevent route shadowing
router.get('/:id/activity', requireAuth, requireAdmin, async (req, res) => {
    try {
        const AuditLog = userService.database.models.AuditLog;
        if (!AuditLog) return res.json([]);

        // Use raw query to avoid UUID type casting issues on non-UUID entity_id values
        const sequelize = userService.database.sequelize;
        const logs = await sequelize.query(
            `SELECT id, user_id, action, entity_type, entity_id, ip_address, result, risk_level, metadata, created_at
             FROM audit_logs
             WHERE user_id = :userId::uuid
             ORDER BY created_at DESC
             LIMIT 50`,
            { replacements: { userId: req.params.id }, type: sequelize.QueryTypes.SELECT }
        );

        res.json(logs || []);
    } catch (error) {
        console.error('❌ Error fetching user activity:', error);
        res.status(500).json({ message: 'Failed to fetch activity' });
    }
});

// POST /api/users/:id/unlock - Clear account lockout (admin only)
router.post('/:id/unlock', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        await user.update({ locked_until: null, login_attempts: 0 });

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_UNLOCKED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { unlockedUser: user.email, unlockedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });

        res.json({ message: 'Account unlocked successfully' });
    } catch (error) {
        console.error('❌ Unlock error:', error);
        res.status(500).json({ message: 'Failed to unlock account' });
    }
});

// POST /api/users/:id/force-reset - Force password reset on next login (admin only)
router.post('/:id/force-reset', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        await user.update({ force_password_reset: true });

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_FORCE_RESET',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { targetUser: user.email, requestedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });

        res.json({ message: 'Password reset required on next login' });
    } catch (error) {
        console.error('❌ Force reset error:', error);
        res.status(500).json({ message: 'Failed to set force reset' });
    }
});

// PUT /api/users/:id/compliance - Update GDPR/HIPAA compliance fields (admin only)
router.put('/:id/compliance', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        const allowed = ['data_consent_given', 'data_consent_date', 'data_retention_until', 'data_anonymized'];
        const updates = {};
        for (const key of allowed) {
            if (req.body[key] !== undefined) updates[key] = req.body[key];
        }

        // Consent date is immutable once recorded — GDPR Article 6 legal timestamp
        if (updates.data_consent_date && user.data_consent_date) {
            delete updates.data_consent_date;
        }

        await user.update(updates);

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_COMPLIANCE_UPDATED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { updatedFields: Object.keys(updates), updatedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium',
            complianceFlags: { gdpr: true, hipaa: true }
        });

        res.json({ message: 'Compliance fields updated' });
    } catch (error) {
        console.error('❌ Compliance update error:', error);
        res.status(500).json({ message: 'Failed to update compliance fields' });
    }
});

// POST /api/users/:id/gdpr-request - Flag GDPR deletion request (admin only)
router.post('/:id/gdpr-request', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        await user.update({ gdpr_delete_requested: true, gdpr_delete_requested_at: new Date() });

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'GDPR_DELETE_REQUESTED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { targetUser: user.email, requestedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'high',
            complianceFlags: { gdpr: true }
        });

        res.json({ message: 'GDPR deletion request recorded' });
    } catch (error) {
        console.error('❌ GDPR request error:', error);
        res.status(500).json({ message: 'Failed to record GDPR request' });
    }
});

// GET /api/users/:id/gdpr-export - GDPR Article 15 (right of access) data export (admin only)
// Returns a downloadable JSON file: the user's own profile fields (credentials/
// tokens excluded — those were never "their data" to disclose) plus their full
// audit trail, both as actor (user_id = this user) and as subject
// (entity_type='User', entity_id = this user) — the complete picture of what
// this system holds and has done involving them, per Article 15.
router.get('/:id/gdpr-export', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        const sequelize = userService.database.sequelize;
        const auditTrail = await sequelize.query(
            `SELECT id, user_id, action, entity_type, entity_id, old_values, new_values,
                    metadata, ip_address, result, risk_level, compliance_flags, created_at
             FROM audit_logs
             WHERE user_id = :userId::uuid
                OR (entity_type = 'User' AND entity_id = :userIdText)
             ORDER BY created_at ASC`,
            { replacements: { userId: req.params.id, userIdText: req.params.id }, type: sequelize.QueryTypes.SELECT }
        );

        const exportData = {
            exportGeneratedAt: new Date().toISOString(),
            exportGeneratedBy: req.session.user.email,
            profile: {
                id: user.id,
                email: user.email,
                first_name: user.first_name,
                last_name: user.last_name,
                role: user.role,
                status: user.status,
                phone: user.phone,
                organization: user.organization,
                job_title: user.job_title,
                department: user.department,
                timezone: user.timezone,
                locale: user.locale,
                preferences: user.preferences,
                data_consent_given: user.data_consent_given,
                data_consent_date: user.data_consent_date,
                data_retention_until: user.data_retention_until,
                last_login_at: user.last_login_at,
                created_at: user.created_at,
                updated_at: user.updated_at
                // Deliberately excluded: password_hash, email_verification_token,
                // password_reset_token — credentials/security tokens were never
                // "the data subject's data" to disclose under Article 15.
            },
            auditTrail
        };

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'GDPR_DATA_EXPORTED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { targetUser: user.email, exportedBy: req.session.user.email, auditRowCount: auditTrail.length },
            ipAddress: req.clientIP,
            result: 'success'
        });

        const filename = `gdpr-export-${req.params.id}-${Date.now()}.json`;
        res.setHeader('Content-Type', 'application/json');
        res.setHeader('Content-Disposition', `attachment; filename="${filename}"`);
        res.send(JSON.stringify(exportData, null, 2));
    } catch (error) {
        console.error('❌ GDPR export error:', error);
        res.status(500).json({ message: 'Failed to generate GDPR export' });
    }
});

// POST /api/users/:id/gdpr-erase - GDPR Article 17 (right to erasure) execution (admin only)
// A DELIBERATE SECOND STEP, separate from POST /gdpr-request above (which only
// flags a request) — requires gdpr_delete_requested to already be true and a
// typed reason, matching the reason-required precedent already established by
// the Go cda_dedupe_registry purge endpoint. ANONYMIZES rather than hard-
// deletes: overwrites PII fields in place, keeps the row (id/role/timestamps)
// and every audit_logs row referencing this user completely untouched — those
// audit rows only ever store user_id (a UUID FK), never a denormalized copy of
// name/email, so anonymizing this row automatically "anonymizes" every past
// and future audit-log lookup for this person without touching audit_logs at
// all. This satisfies GDPR Art. 17 (erase personal identifiers) while
// satisfying HIPAA's own audit-trail retention requirement (Art. 17(3)(b)'s
// legal-obligation exception) — the audit trail's evidentiary shape survives,
// only who it was is erased.
router.post('/:id/gdpr-erase', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        if (!user.gdpr_delete_requested) {
            return res.status(400).json({ message: 'User has not been flagged for GDPR deletion — use POST /:id/gdpr-request first' });
        }
        if (user.data_anonymized) {
            return res.status(400).json({ message: 'User data has already been anonymized' });
        }
        const reason = (req.body && req.body.reason || '').trim();
        if (!reason) {
            return res.status(400).json({ message: 'A reason is required for a GDPR erasure action' });
        }

        const fieldsAnonymized = ['email', 'first_name', 'last_name', 'phone', 'organization', 'job_title', 'department', 'preferences'];
        await user.update({
            // Per-user-unique placeholder (not a single shared value) — email
            // has a UNIQUE constraint, a shared placeholder would violate it
            // on the second erasure. ".invalid" is the IANA-reserved
            // special-use TLD meant for exactly this (RFC 2606).
            email: `deleted-${user.id}@anonymized.invalid`,
            first_name: 'Deleted',
            last_name: 'User',
            phone: null,
            organization: null,
            job_title: null,
            department: null,
            preferences: null,
            // password_hash is NOT NULL — replaced with an unusable random
            // value (never nulled), matching the existing invite-flow
            // precedent (a hash nothing can ever hash-compare true against).
            password_hash: crypto.randomBytes(32).toString('hex'),
            email_verification_token: null,
            password_reset_token: null,
            password_reset_expires: null,
            status: 'inactive',
            data_anonymized: true
        });

        // Deliberately NOT logging the original email/name/phone as oldValues
        // — doing so would permanently retain exactly the PII this action is
        // meant to erase, defeating the entire point. Only the fact that
        // erasure happened, who did it, and why is recorded.
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'GDPR_DATA_ERASED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { anonymizedBy: req.session.user.email, reason, fieldsAnonymized },
            ipAddress: req.clientIP,
            result: 'success'
        });

        res.json({ message: 'User data anonymized per GDPR Article 17 erasure request' });
    } catch (error) {
        console.error('❌ GDPR erasure error:', error);
        res.status(500).json({ message: 'Failed to execute GDPR erasure' });
    }
});

// PUT /api/users/:id/profile - Update full user profile (admin only)
router.put('/:id/profile', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        // Captured before update — the only way to know what a role change
        // was FROM, and the input to the isRoleChange check below.
        const oldRole = user.role;

        const allowed = ['first_name', 'last_name', 'phone', 'organization', 'job_title', 'department', 'timezone', 'locale'];
        const updates = {};
        for (const key of allowed) {
            if (req.body[key] !== undefined) updates[key] = req.body[key];
        }
        if (req.body.email) updates.email = req.body.email.toLowerCase();
        if (req.body.role) {
            if (!['admin', 'operator', 'viewer', 'user'].includes(req.body.role)) {
                return res.status(400).json({ message: 'Invalid role' });
            }
            updates.role = req.body.role;
        }
        if (req.body.password && req.body.password.length >= 8) {
            updates.password_hash = await bcrypt.hash(req.body.password, 12);
        }

        await user.update(updates);

        // Role and password changes get their own distinct, high-risk audit
        // events — this endpoint can silently carry either alongside routine
        // profile edits (name, phone, timezone, ...), and blending an access-
        // control change (role) or a credential change (password) into the
        // same flat 'medium' risk as a phone-number edit would hide exactly
        // the kind of event HIPAA/GDPR audit review cares about most. Both
        // fire ADDITIONALLY, not instead of, USER_PROFILE_UPDATED below —
        // same "one general event + a distinct elevated-risk event for the
        // sensitive subset" pattern used for INTERFACE_CREDENTIALS_UPDATED.
        const isRoleChange = updates.role !== undefined && updates.role !== oldRole;
        if (isRoleChange) {
            auditService.logEvent({
                userId: req.session.user.id,
                action: 'USER_ROLE_CHANGED',
                entityType: 'User',
                entityId: req.params.id,
                oldValues: { role: oldRole },
                newValues: { role: updates.role },
                metadata: { updatedBy: req.session.user.email },
                ipAddress: req.clientIP,
                result: 'success',
                riskLevel: 'high'
            }).catch(err => console.warn('⚠️ Failed to write USER_ROLE_CHANGED audit log:', err.message));
        }
        if (updates.password_hash !== undefined) {
            // Never log the hash itself — presence of the change is the signal.
            auditService.logEvent({
                userId: req.session.user.id,
                action: 'USER_PASSWORD_RESET_BY_ADMIN',
                entityType: 'User',
                entityId: req.params.id,
                metadata: { updatedBy: req.session.user.email },
                ipAddress: req.clientIP,
                result: 'success',
                riskLevel: 'high'
            }).catch(err => console.warn('⚠️ Failed to write USER_PASSWORD_RESET_BY_ADMIN audit log:', err.message));
        }

        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_PROFILE_UPDATED',
            entityType: 'User',
            entityId: req.params.id,
            metadata: { updatedFields: Object.keys(updates), updatedBy: req.session.user.email },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'medium'
        });

        res.json({ message: 'Profile updated successfully' });
    } catch (error) {
        console.error('❌ Profile update error:', error);
        res.status(500).json({ message: 'Failed to update profile' });
    }
});

// GET /api/users/:id - Full user detail (admin only)
// NOTE: Registered AFTER all /:id/sub-routes to avoid shadowing them
router.get('/:id', requireAuth, requireAdmin, async (req, res) => {
    try {
        const user = await userService.database.models.User.findByPk(req.params.id, {
            attributes: { exclude: ['password_hash', 'email_verification_token', 'password_reset_token'] }
        });
        if (!user) return res.status(404).json({ message: 'User not found' });
        res.json(user);
    } catch (error) {
        console.error('❌ Error fetching user detail:', error);
        res.status(500).json({ message: 'Failed to fetch user' });
    }
});

// DELETE /api/users/:id - Delete user (admin only)
router.delete('/:id', requireAuth, requireAdmin, async (req, res) => {
    try {
        const { id } = req.params;
        
        const user = await userService.database.models.User.findByPk(id);
        if (!user) {
            return res.status(404).json({ message: 'User not found' });
        }
        
        // Prevent deleting yourself
        if (user.id === req.session.user.id) {
            return res.status(400).json({ message: 'Cannot delete your own account' });
        }
        
        await user.destroy();
        
        await auditService.logEvent({
            userId: req.session.user.id,
            action: 'USER_DELETED',
            entityType: 'User',
            entityId: id,
            metadata: { 
                deletedUser: user.email,
                deletedBy: req.session.user.email
            },
            ipAddress: req.clientIP,
            result: 'success',
            riskLevel: 'high'
        });
        
        res.json({ message: 'User deleted successfully' });
        
    } catch (error) {
        console.error('❌ Error deleting user:', error);
        res.status(500).json({ message: 'Failed to delete user' });
    }
});

module.exports = router;