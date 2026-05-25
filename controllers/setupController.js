// controllers/setupController.js
// First-run setup wizard API.
// These endpoints are PUBLIC — they must be reachable before any user is logged in.

'use strict';

const express  = require('express');
const bcrypt   = require('bcryptjs');
const router   = express.Router();

const settingsService = require('../services/settingsService');

// ─── Helpers ──────────────────────────────────────────────────────────────────

async function isSetupRequired() {
    const val = await settingsService.getSetting('setup_complete', false);
    // JSONB boolean comes back as JS boolean; JSONB string "false" would also be falsy
    return val !== true;
}

async function markSetupComplete() {
    const { sequelize } = require('../config/database');
    await sequelize.query(
        `UPDATE system_settings SET value = 'true'::jsonb, updated_at = NOW()
         WHERE key = 'setup_complete'`
    );
    settingsService.invalidate('setup_complete');
}

async function setOrgName(orgName) {
    const { sequelize } = require('../config/database');
    await sequelize.query(
        `UPDATE system_settings SET value = $1::jsonb, updated_at = NOW()
         WHERE key = 'org_name'`,
        { bind: [JSON.stringify(orgName)] }
    );
    settingsService.invalidate('org_name');
}

// ─── GET /api/setup/status ────────────────────────────────────────────────────
// Returns whether setup is still required. Safe to call any time.
router.get('/status', async (req, res) => {
    try {
        const required = await isSetupRequired();
        res.json({ setupRequired: required });
    } catch (err) {
        console.error('[setup] status error:', err.message);
        res.status(500).json({ message: 'Could not determine setup status' });
    }
});

// ─── POST /api/setup/complete ────────────────────────────────────────────────
// Completes the first-run wizard:
//   - Validates that setup is still required (idempotency guard)
//   - Updates the default admin account with the provided name / email / password
//   - Writes org_name to system_settings
//   - Marks setup_complete = true
router.post('/complete', async (req, res) => {
    try {
        const required = await isSetupRequired();
        if (!required) {
            return res.status(409).json({ message: 'Setup has already been completed.' });
        }

        const { orgName, firstName, lastName, email, password } = req.body;

        // ── Validate inputs ──────────────────────────────────────────────────
        const missing = [];
        if (!orgName   || !orgName.trim())   missing.push('orgName');
        if (!firstName || !firstName.trim()) missing.push('firstName');
        if (!lastName  || !lastName.trim())  missing.push('lastName');
        if (!email     || !email.trim())     missing.push('email');
        if (!password)                        missing.push('password');
        if (missing.length) {
            return res.status(400).json({ message: `Missing required fields: ${missing.join(', ')}` });
        }

        const emailLC = email.trim().toLowerCase();
        const emailRE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        if (!emailRE.test(emailLC)) {
            return res.status(400).json({ message: 'Invalid email address.' });
        }
        if (password.length < 8) {
            return res.status(400).json({ message: 'Password must be at least 8 characters.' });
        }

        const { models } = require('../config/database');
        const User = models.User;

        // ── Email uniqueness check (if a different user already uses this email) ──
        const existingByEmail = await User.findOne({ where: { email: emailLC } });
        const defaultAdminEmail = 'admin@ezhealthkonnect.com';
        if (existingByEmail && existingByEmail.email !== defaultAdminEmail) {
            return res.status(409).json({ message: 'An account with that email already exists.' });
        }

        // ── Find the placeholder admin created by V2 migration ───────────────
        const defaultAdmin = await User.findOne({ where: { email: defaultAdminEmail } });

        const passwordHash = await bcrypt.hash(password, 12);

        if (defaultAdmin) {
            // Transform the placeholder admin into the real admin account
            await defaultAdmin.update({
                email:                emailLC,
                first_name:           firstName.trim(),
                last_name:            lastName.trim(),
                password_hash:        passwordHash,
                force_password_reset: false,
                status:               'active',
                email_verified:       true
            });
            console.log(`[setup] Default admin updated → ${emailLC}`);
        } else {
            // Edge case: placeholder was deleted — create a fresh admin
            await User.create({
                email:          emailLC,
                first_name:     firstName.trim(),
                last_name:      lastName.trim(),
                password_hash:  passwordHash,
                role:           'admin',
                status:         'active',
                email_verified: true,
                force_password_reset: false,
                data_consent_given:   true,
                data_consent_date:    new Date()
            });
            console.log(`[setup] New admin created: ${emailLC}`);
        }

        // ── Persist org name and mark setup complete ─────────────────────────
        await setOrgName(orgName.trim());
        await markSetupComplete();

        // Invalidate the in-memory setup-check cache so the middleware
        // stops redirecting immediately (no 5-second wait).
        try {
            const { invalidateSetupCache } = require('../middleware/setupCheck');
            invalidateSetupCache();
        } catch (_) { /* middleware may not be loaded yet in test environments */ }

        console.log(`[setup] First-run setup complete. Org: "${orgName.trim()}", Admin: ${emailLC}`);

        res.json({
            message: 'Setup complete. You can now log in.',
            email:   emailLC
        });

    } catch (err) {
        console.error('[setup] complete error:', err);
        res.status(500).json({ message: 'Setup failed. Please try again.' });
    }
});

module.exports = router;
module.exports.isSetupRequired = isSetupRequired;
