'use strict';
/**
 * requireInterfaceAccess — for routes that take an `interfaceId` query parameter (e.g. the Go-forwarded message
 * trace). Lets the request through only when the id is a UUID and the caller may see that interface: their own, or
 * any when the access policy lets their role see all interfaces (services/AccessPolicyService). Same rule as every
 * Node message endpoint: `(user_id = :userId OR <policy flag>)`.
 *
 * - 400 for a missing / non-UUID id, before any SQL runs;
 * - 404 (not 403) for an interface the caller cannot see, so interface ids cannot be probed;
 * - fails closed on any error.
 *
 * Dependencies are injected so the rule is unit-testable and not tied to a global.
 */

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function requireInterfaceAccess({
    database = require('../config/database'),
    accessPolicy = require('../services/AccessPolicyService'),
    param = 'interfaceId',
} = {}) {
    return async function interfaceAccess(req, res, next) {
        try {
            const interfaceId = req.query && req.query[param];
            if (!interfaceId || !UUID_RE.test(String(interfaceId))) {
                return res.status(400).json({ success: false, error: `${param} query parameter is required and must be a UUID` });
            }

            const user = (req.session && req.session.user) || req.user;
            if (!user || !user.id) {
                return res.status(401).json({ success: false, error: 'Authentication required' });
            }

            // A boolean from the policy service, interpolated as a literal — never user input.
            const allInterfaces = await accessPolicy.canSeeAllInterfaces(user) ? 'true' : 'false';
            const rows = await database.sequelize.query(
                `SELECT id FROM interfaces WHERE id = :interfaceId AND (user_id = :userId OR ${allInterfaces})`,
                { replacements: { interfaceId, userId: user.id }, type: database.sequelize.QueryTypes.SELECT }
            );
            if (!rows || rows.length === 0) {
                return res.status(404).json({ success: false, error: 'Interface not found' });
            }
            return next();
        } catch (err) {
            console.error('❌ interfaceAccess error:', err.message);
            return res.status(500).json({ success: false, error: 'Could not verify access to this interface' });
        }
    };
}

module.exports = { requireInterfaceAccess, UUID_RE };
