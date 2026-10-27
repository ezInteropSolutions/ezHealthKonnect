// middleware/auth.js
const jwt = require('jsonwebtoken');

// Middleware to require authentication
function requireAuth(req, res, next) {
    if (!req.session.user) {
        return res.status(401).json({ message: 'Authentication required' });
    }
    next();
}

// Middleware to require admin role (includes super_admin)
function requireAdmin(req, res, next) {
    const role = req.session.user && req.session.user.role;
    if (!role || (role !== 'admin' && role !== 'super_admin')) {
        return res.status(403).json({ message: 'Admin access required' });
    }
    next();
}

// Middleware to require super_admin role — for vendor-only operations such as
// rebuilding OOB templates.  Client admins with role 'admin' are denied.
function requireSuperAdmin(req, res, next) {
    if (!req.session.user || req.session.user.role !== 'super_admin') {
        return res.status(403).json({ message: 'Super-admin access required' });
    }
    next();
}

// Middleware factory: require one of the specified roles
// Usage: requireRole('admin', 'operator')
function requireRole(...roles) {
    return (req, res, next) => {
        if (!req.session.user) {
            return res.status(401).json({ message: 'Authentication required' });
        }
        if (!roles.includes(req.session.user.role)) {
            return res.status(403).json({ message: `Access requires one of: ${roles.join(', ')}` });
        }
        next();
    };
}

// Middleware to verify JWT token (optional, for API authentication).
// Accepts either a Bearer token (API callers) or a valid session cookie
// (browser requests from pages like sidebar-nav.js pollers).
function verifyToken(req, res, next) {
    const token = req.header('Authorization')?.replace('Bearer ', '') || req.query.token;

    if (token) {
        try {
            const decoded = jwt.verify(token, process.env.JWT_SECRET);
            req.user = decoded;
            return next();
        } catch (error) {
            return res.status(401).json({ message: 'Invalid or expired token' });
        }
    }

    // Fall back to session auth for browser requests (no Bearer token sent)
    if (req.session && req.session.user) {
        req.user = req.session.user;
        return next();
    }

    return res.status(401).json({ message: 'Access token required' });
}

// Like verifyToken, but never rejects: attaches req.user when the caller is a
// logged-in app user (valid app JWT or session) and otherwise just continues.
// Used on partner-facing endpoints whose real authentication happens per
// interface in Go — there, an Authorization header is often the PARTNER's own
// bearer token / Basic credentials, not an app JWT, so failing verification
// must not be treated as an error here.
function attachUserIfPresent(req, res, next) {
    const auth = req.header('Authorization') || '';
    if (auth.startsWith('Bearer ')) {
        try {
            req.user = jwt.verify(auth.slice(7), process.env.JWT_SECRET);
            return next();
        } catch (_) { /* not an app JWT — may be a partner credential */ }
    }
    if (req.session && req.session.user) {
        req.user = req.session.user;
    }
    return next();
}

// Role check keyed on the HTTP method: safe methods (GET/HEAD/OPTIONS) need one
// of readRoles, everything else one of writeRoles. Works for both session and
// JWT callers (requireRole above only reads the session). Run after
// verifyToken. Used on Go-forwarded route groups, which previously checked only
// that a user was logged in — the 2026-10-19 authorization probe found a
// no-permission user could deactivate any interface, write pipeline code
// templates and delete CDA documents through them.
const SAFE_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);
function requireRoleByMethod(readRoles, writeRoles) {
    return (req, res, next) => {
        const user = req.user || (req.session && req.session.user);
        if (!user) return res.status(401).json({ message: 'Authentication required' });
        const allowed = SAFE_METHODS.has(req.method) ? readRoles : writeRoles;
        if (!allowed.includes(user.role)) {
            return res.status(403).json({ message: `Access requires one of: ${allowed.join(', ')}` });
        }
        return next();
    };
}

// Marks a request so forwardToGo also forwards the caller's own credential
// headers (Authorization, API-key headers) to Go. Only for partner-facing
// endpoints that authenticate per interface — every other proxied route
// keeps forwardToGo's default of building Go-bound headers from scratch.
function forwardPartnerCredentials(req, res, next) {
    req.forwardPartnerCredentials = true;
    return next();
}

module.exports = {
    requireAuth,
    requireAdmin,
    requireSuperAdmin,
    requireRole,
    verifyToken,
    attachUserIfPresent,
    forwardPartnerCredentials,
    requireRoleByMethod
};
