// routes/auth.js
const express = require('express');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const router = express.Router();

const userService = require('../services/userService');
const auditService = require('../services/auditService');
const { requireAuth } = require('../middleware/auth');
const settingsService = require('../services/settingsService');
const accessPolicy = require('../services/AccessPolicyService');
const RateLimiter = require('../middleware/rateLimiter');
const passwordResetTokens = require('../services/PasswordResetTokenService');
const { PasswordResetTokenService } = passwordResetTokens;

// Per-IP throttle on login attempts — separate from the existing per-account
// lockout below (which only kicks in after repeated failures against ONE email).
// Without this, credential stuffing across many usernames from a single IP was
// entirely unthrottled. 10 req/min per IP.
const loginRateLimiter = new RateLimiter({ windowMs: 60 * 1000, max: 10 });

// POST /api/auth/login - User login
router.post('/login', loginRateLimiter.middleware(), async (req, res) => {
    try {
        const { email, password } = req.body || {};

        // Malformed input is a client error. Without this, a missing/non-string value reached the user
        // lookup, threw, and surfaced as a 500.
        if (typeof email !== 'string' || typeof password !== 'string' || !email.trim() || !password) {
            return res.status(400).json({ message: 'Email and password are required' });
        }

        console.log(`🔐 Login attempt for: ${email}`);
        
        // Find user
        const user = await userService.findByEmail(email);
        if (!user) {
            await auditService.logEvent({
                action: 'LOGIN_FAILED',
                entityType: 'User',
                metadata: { email, reason: 'User not found' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                sessionId: req.sessionID,
                result: 'failure',
                riskLevel: 'medium',
                complianceFlags: { failed_authentication: true }
            });
            
            return res.status(401).json({ message: 'Invalid email or password' });
        }
        
        console.log(`✅ User found in ${user.source}: ${user.name}`);

        // Check account lockout
        if (user.lockedUntil && new Date(user.lockedUntil) > new Date()) {
            const unlockAt = new Date(user.lockedUntil).toISOString();
            await auditService.logEvent({
                userId: user.id,
                action: 'LOGIN_FAILED',
                entityType: 'User',
                metadata: { email, reason: 'Account locked', lockedUntil: unlockAt },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                result: 'failure',
                riskLevel: 'high'
            });
            return res.status(423).json({ message: 'Account is temporarily locked. Please try again later or contact your administrator.', lockedUntil: unlockAt });
        }

        // Check if user is active
        if (user.status !== 'active') {
            await auditService.logEvent({
                userId: user.id,
                action: 'LOGIN_FAILED',
                entityType: 'User',
                metadata: { email, reason: 'Account inactive' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                result: 'failure',
                riskLevel: 'high'
            });
            
            return res.status(401).json({ message: 'Account is inactive. Please contact administrator.' });
        }
        
        // Load security policy (cached 5 min — no per-request DB hit in steady state)
        const secPolicy = await settingsService.getSecuritySettings();
        const maxAttempts     = secPolicy.max_login_attempts       || 5;
        const lockoutMs       = (secPolicy.lockout_duration_minutes || 30) * 60 * 1000;

        // Check password
        const isValid = await bcrypt.compare(password, user.password);
        if (!isValid) {
            // Increment failed login attempts and lock if threshold reached
            try {
                const dbUser = await userService.database.models.User.findByPk(user.id);
                if (dbUser) {
                    const newAttempts = (dbUser.login_attempts || 0) + 1;
                    const updatePayload = { login_attempts: newAttempts };
                    if (newAttempts >= maxAttempts) {
                        updatePayload.locked_until = new Date(Date.now() + lockoutMs);
                    }
                    await dbUser.update(updatePayload);
                }
            } catch (lockErr) {
                console.error('Failed to update login attempts:', lockErr.message);
            }

            await auditService.logEvent({
                userId: user.id,
                action: 'LOGIN_FAILED',
                entityType: 'User',
                metadata: { email, reason: 'Invalid password' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                result: 'failure',
                riskLevel: 'high',
                complianceFlags: { authentication_failure: true }
            });

            return res.status(401).json({ message: 'Invalid email or password' });
        }
        
        // Clear failed login attempts on successful password verification
        try {
            const dbUser = await userService.database.models.User.findByPk(user.id);
            if (dbUser && (dbUser.login_attempts > 0 || dbUser.locked_until)) {
                await dbUser.update({ login_attempts: 0, locked_until: null });
            }
        } catch (clearErr) {
            console.warn('Could not clear login attempts:', clearErr.message);
        }

        // Check force password reset BEFORE creating session
        if (user.forcePasswordReset) {
            await auditService.logEvent({
                userId: user.id,
                action: 'LOGIN_FORCE_RESET',
                entityType: 'User',
                metadata: { email: user.email, reason: 'Password reset required by admin' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                result: 'success',
                riskLevel: 'medium',
                complianceFlags: { forced_password_reset: true }
            });
            // The password was just verified, so this caller may complete the reset: hand out the single-use token
            // that /reset-password requires (it no longer trusts a bare userId).
            const dbUserForReset = await userService.database.models.User.findByPk(user.id);
            return res.status(403).json({
                message: 'Your password must be changed before you can continue.',
                requiresPasswordReset: true,
                userId: user.id,
                resetToken: await passwordResetTokens.issue(dbUserForReset)
            });
        }

        // Update last login
        await userService.updateLastLogin(user.id, user.email, req.clientIP);

        // Create JWT token — expiry from DB settings (default 24 h)
        const jwtExpiryHours = secPolicy.jwt_expiry_hours || 24;
        const token = jwt.sign(
            { userId: user.id, email: user.email, role: user.role },
            process.env.JWT_SECRET, // enforced at startup by config/SecretsGuard.js — no fallback
            { expiresIn: `${jwtExpiryHours}h` }
        );

        // Store in session
        req.session.user = {
            id: user.id,
            email: user.email,
            name: user.name,
            role: user.role
        };

        // Log successful login
        await auditService.logEvent({
            userId: user.id,
            action: 'LOGIN_SUCCESS',
            entityType: 'User',
            metadata: {
                email: user.email,
                source: user.source,
                sessionId: req.sessionID
            },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'low',
            complianceFlags: {
                successful_authentication: true,
                data_source: user.source
            }
        });

        console.log(`✅ Login successful for: ${user.name} (${user.source})`);

        // Persist session before responding so the cookie is valid on the
        // very next request the client makes after redirect.
        req.session.save((saveErr) => {
            if (saveErr) {
                console.error('Session save error:', saveErr);
                return res.status(500).json({ message: 'Session error — please try again' });
            }
            res.json({
                message: 'Login successful',
                token: token,
                user: {
                    id: user.id,
                    email: user.email,
                    name: user.name,
                    role: user.role
                }
            });
        });
        return;
        
    } catch (error) {
        console.error('Login error:', error);
        
        await auditService.logEvent({
            action: 'LOGIN_ERROR',
            entityType: 'System',
            metadata: { error: error.message },
            ipAddress: req.clientIP,
            result: 'error',
            riskLevel: 'critical'
        });
        
        res.status(500).json({ message: 'Internal server error' });
    }
});

// POST /api/auth/reset-password — Complete a forced password reset (no session needed).
// Requires the single-use resetToken that /login issued after the CURRENT password was verified; a bare userId is
// not a credential. Every failure answers identically so the endpoint cannot be used to learn which accounts exist
// or are flagged. Rate limited per IP.
const resetPasswordRateLimiter = new RateLimiter({ windowMs: 60 * 1000, max: 5 });
router.post('/reset-password', resetPasswordRateLimiter.middleware(), async (req, res) => {
    const INVALID = { message: 'This password reset link is invalid or has expired. Please sign in again.' };
    try {
        const { userId, resetToken, password } = req.body || {};
        if (typeof userId !== 'string' || typeof resetToken !== 'string' || typeof password !== 'string' || !userId || !resetToken || !password) {
            return res.status(400).json({ message: 'userId, resetToken and password are required' });
        }
        const secPolicy = await settingsService.getSecuritySettings();
        const minLength = secPolicy.password_min_length || 8;
        if (password.length < minLength) {
            return res.status(400).json({ message: `Password must be at least ${minLength} characters` });
        }

        const User = userService.database.models.User;
        const user = /^[0-9a-f-]{36}$/i.test(userId) ? await User.findByPk(userId) : null;
        if (!user || !user.force_password_reset || !passwordResetTokens.verify(user, resetToken)) {
            await auditService.logEvent({
                userId: user ? user.id : null,
                action: 'PASSWORD_RESET_COMPLETED',
                entityType: 'User',
                metadata: { method: 'forced_reset', reason: 'invalid or expired reset token' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                result: 'failure',
                riskLevel: 'high',
                complianceFlags: { forced_password_reset: true }
            });
            return res.status(400).json(INVALID);
        }

        // The model's attribute is password_hash. A `password` key is not an attribute and Sequelize silently
        // ignores it, which left the old password valid after a "successful" reset.
        await user.update({
            password_hash: await bcrypt.hash(password, 12),
            force_password_reset: false, login_attempts: 0, locked_until: null,
            ...PasswordResetTokenService.cleared(),   // single use
        });

        await auditService.logEvent({
            userId: user.id,
            action: 'PASSWORD_RESET_COMPLETED',
            entityType: 'User',
            metadata: { email: user.email, method: 'forced_reset' },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'medium',
            complianceFlags: { forced_password_reset: true }
        });

        res.json({ message: 'Password updated successfully. You may now log in.' });
    } catch (error) {
        console.error('❌ Reset password error:', error);
        res.status(500).json({ message: 'Failed to reset password' });
    }
});

// POST /api/auth/change-password — A signed-in user changes their OWN password (current password required).
// Rate limited per IP so a stolen session cannot be used to guess the current password.
const changePasswordRateLimiter = new RateLimiter({ windowMs: 60 * 1000, max: 5 });
router.post('/change-password', requireAuth, changePasswordRateLimiter.middleware(), async (req, res) => {
    const sessionUser = req.session.user;
    const audit = (result, reason) => auditService.logEvent({
        userId: sessionUser.id,
        action: 'PASSWORD_CHANGED',
        entityType: 'User',
        entityId: sessionUser.id,
        metadata: { email: sessionUser.email, method: 'self_service', ...(reason ? { reason } : {}) },
        ipAddress: req.clientIP,
        userAgent: req.get('User-Agent'),
        sessionId: req.sessionID,
        result,
        riskLevel: result === 'success' ? 'medium' : 'high'
    });

    try {
        const { currentPassword, newPassword } = req.body || {};
        if (typeof currentPassword !== 'string' || typeof newPassword !== 'string' || !currentPassword || !newPassword) {
            return res.status(400).json({ message: 'Current password and new password are required' });
        }

        const secPolicy = await settingsService.getSecuritySettings();
        const minLength = secPolicy.password_min_length || 8;
        if (newPassword.length < minLength) {
            return res.status(400).json({ message: `New password must be at least ${minLength} characters` });
        }

        const User = userService.database.models.User;
        const user = await User.findByPk(sessionUser.id);
        if (!user) return res.status(404).json({ message: 'User not found' });

        if (!(await bcrypt.compare(currentPassword, user.password_hash))) {
            await audit('failure', 'current password incorrect');
            return res.status(400).json({ message: 'Current password is incorrect' });
        }
        if (await bcrypt.compare(newPassword, user.password_hash)) {
            return res.status(400).json({ message: 'New password must be different from the current password' });
        }

        await user.update({ password_hash: await bcrypt.hash(newPassword, 12), force_password_reset: false });
        await audit('success');

        res.json({ message: 'Password changed successfully' });
    } catch (error) {
        console.error('❌ Change password error:', error);
        res.status(500).json({ message: 'Failed to change password' });
    }
});

// POST /api/auth/logout - User logout
router.post('/logout', (req, res) => {
    req.session.destroy();
    res.json({ message: 'Logged out successfully' });
});

// GET /api/auth/profile - Get user profile
router.get('/profile', requireAuth, (req, res) => {
    res.json(req.session.user);
});

// GET /api/auth/session - Check if session is valid
router.get('/session', async (req, res) => {
    if (req.session && req.session.user) {
        res.json({
            authenticated: true,
            user: {
                id: req.session.user.id,
                email: req.session.user.email,
                name: req.session.user.name,
                role: req.session.user.role
            },
            // What this role may see (Settings -> Access policy), so pages can explain a restriction up front.
            access: await accessPolicy.policyFor(req.session.user.role)
        });
    } else {
        res.status(401).json({
            authenticated: false,
            message: 'No active session'
        });
    }
});

module.exports = router;