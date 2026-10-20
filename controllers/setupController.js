// controllers/setupController.js
// First-run setup wizard API.
// These endpoints are PUBLIC — they must be reachable before any user is logged in.

'use strict';

const express  = require('express');
const bcrypt   = require('bcryptjs');
const crypto   = require('crypto');
const router   = express.Router();

const settingsService = require('../services/settingsService');

/**
 * Guards first-run setup with the one-time token the installer writes to
 * SETUP_TOKEN (and gives only to the person installing). Without it, whoever
 * reached /setup.html first — anyone on the network — could create the first
 * administrator. When SETUP_TOKEN is not configured (local development)
 * setup stays open, as before.
 */
class SetupGate {
    constructor(env = process.env) {
        this.token = (env.SETUP_TOKEN || '').trim();
    }

    get tokenRequired() {
        return this.token !== '';
    }

    verify(candidate) {
        if (!this.tokenRequired) return true;
        if (typeof candidate !== 'string' || candidate === '') return false;
        const a = crypto.createHash('sha256').update(candidate).digest();
        const b = crypto.createHash('sha256').update(this.token).digest();
        return crypto.timingSafeEqual(a, b);
    }
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

async function isSetupRequired() {
    const val = await settingsService.getSetting('setup_complete', false);
    // JSONB boolean comes back as JS boolean; JSONB string "false" would also be falsy
    return val !== true;
}

/**
 * Atomically claims setup completion inside `transaction`: only the request
 * that flips setup_complete from not-true to true gets `true`, so two
 * simultaneous submissions can never both create an administrator.
 */
async function claimSetupCompletion(transaction) {
    const { sequelize } = require('../config/database');
    const [rows] = await sequelize.query(
        `UPDATE system_settings SET value = 'true'::jsonb, updated_at = NOW()
          WHERE key = 'setup_complete' AND value IS DISTINCT FROM 'true'::jsonb
          RETURNING key`,
        { transaction }
    );
    return rows.length === 1;
}

async function setOrgName(orgName, transaction) {
    const { sequelize } = require('../config/database');
    await sequelize.query(
        `UPDATE system_settings SET value = $1::jsonb, updated_at = NOW()
         WHERE key = 'org_name'`,
        { bind: [JSON.stringify(orgName)], transaction }
    );
}

// ─── GET /api/setup/status ────────────────────────────────────────────────────
// Returns whether setup is still required. Safe to call any time.
router.get('/status', async (req, res) => {
    try {
        const required = await isSetupRequired();
        res.json({ setupRequired: required, tokenRequired: required && new SetupGate().tokenRequired });
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

        const { orgName, firstName, lastName, email, password, setupToken } = req.body || {};
        if (!new SetupGate().verify(setupToken)) {
            console.warn('[setup] rejected a setup attempt without a valid setup token from', req.ip);
            return res.status(403).json({
                message: 'Open this page using the setup link shown by the installer (it contains a one-time token).'
            });
        }

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
        const { sequelize } = require('../config/database');

        // One transaction: claim completion first (only one request can), then
        // create the administrator. Any failure leaves setup still available.
        const claimed = await sequelize.transaction(async (transaction) => {
            if (!(await claimSetupCompletion(transaction))) return false;
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
                }, { transaction });
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
                }, { transaction });
            }
            await setOrgName(orgName.trim(), transaction);
            return true;
        });
        if (!claimed) {
            return res.status(409).json({ message: 'Setup has already been completed.' });
        }
        settingsService.invalidate('setup_complete');
        settingsService.invalidate('org_name');
        console.log(`[setup] Administrator account set up: ${emailLC}`);

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
module.exports.SetupGate = SetupGate;
module.exports.isSetupRequired = isSetupRequired;
