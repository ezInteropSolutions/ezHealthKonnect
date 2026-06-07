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

module.exports = {
    requireAuth,
    requireAdmin,
    requireSuperAdmin,
    requireRole,
    verifyToken
};
