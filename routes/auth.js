// routes/auth.js
const express = require('express');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const router = express.Router();

const userService = require('../services/userService');
const auditService = require('../services/auditService');
const { requireAuth } = require('../middleware/auth');

// POST /api/auth/login - User login
router.post('/login', async (req, res) => {
    try {
        const { email, password } = req.body;
        
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
        
        // Check password
        const isValid = await bcrypt.compare(password, user.password);
        if (!isValid) {
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
        
        // Update last login
        await userService.updateLastLogin(user.id, user.email, req.clientIP);
        
        // Create JWT token
        const token = jwt.sign(
            { userId: user.id, email: user.email, role: user.role },
            process.env.JWT_SECRET || 'your-secret-key-change-this-in-production',
            { expiresIn: '24h' }
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

// POST /api/auth/logout - User logout
router.post('/logout', (req, res) => {
    req.session.destroy();
    res.json({ message: 'Logged out successfully' });
});

// GET /api/auth/profile - Get user profile
router.get('/profile', requireAuth, (req, res) => {
    res.json(req.session.user);
});

module.exports = router;