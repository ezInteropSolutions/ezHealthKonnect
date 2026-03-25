// middleware/auth.js
const jwt = require('jsonwebtoken');

// Middleware to require authentication
function requireAuth(req, res, next) {
    if (!req.session.user) {
        return res.status(401).json({ message: 'Authentication required' });
    }
    next();
}

// Middleware to require admin role
function requireAdmin(req, res, next) {
    if (!req.session.user || req.session.user.role !== 'admin') {
        return res.status(403).json({ message: 'Admin access required' });
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

// Middleware to verify JWT token (optional, for API authentication)
function verifyToken(req, res, next) {
    const token = req.header('Authorization')?.replace('Bearer ', '') || req.query.token;

    if (!token) {
        return res.status(401).json({ message: 'Access token required' });
    }

    try {
        const decoded = jwt.verify(token, process.env.JWT_SECRET);
        req.user = decoded;
        next();
    } catch (error) {
        return res.status(401).json({ message: 'Invalid or expired token' });
    }
}

module.exports = {
    requireAuth,
    requireAdmin,
    requireRole,
    verifyToken
};
