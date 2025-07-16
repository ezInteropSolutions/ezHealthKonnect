// app.js - Complete PostgreSQL-only application setup
const express = require('express');
const path = require('path');
const session = require('express-session');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const interfacesRoutes = require('./routes/interfacesRoutes');


const app = express();

// Load environment variables
require('dotenv').config();

// Session configuration
const sessionConfig = {
    secret: process.env.SESSION_SECRET || 'your-session-secret',
    resave: false,
    saveUninitialized: false,
    name: 'ezhealth.sid',
    cookie: { 
        secure: process.env.NODE_ENV === 'production',
        httpOnly: true,
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
        sameSite: 'strict'
    }
};

// Basic middleware
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
app.use(express.static('public'));
app.use('/uploads', express.static('uploads'));


// Session middleware
app.use(session(sessionConfig));

// Add this after: app.use(session(sessionConfig));
app.use((req, res, next) => {
    console.log(`🌐 ${req.method} ${req.url} - Session: ${!!req.session?.user}`);
    next();
});

// Add request ID for audit logging
app.use((req, res, next) => {
    req.requestId = require('crypto').randomUUID();
    req.clientIP = req.ip || req.connection.remoteAddress || 'unknown';
    next();
});

// Import routes
const authRoutes = require('./routes/auth');
const userRoutes = require('./routes/users');

// Debug: Log when routes are loaded
console.log('📁 Loading auth routes...');
console.log('📁 Loading user routes...');

// Mount API routes with trailing slash handling
app.use('/api/auth', authRoutes);
app.use('/api/users', userRoutes);
app.use('/api/users/', userRoutes); // Handle trailing slash



console.log('✅ API routes mounted successfully');

// Legacy route for compatibility (duplicate login logic for /api/login)
app.post('/api/login', async (req, res) => {
    try {
        const userService = require('./services/userService');
        const auditService = require('./services/auditService');
        
        const { email, password } = req.body;
        
        console.log(`🔐 Login attempt for: ${email} (legacy route)`);
        
        // Find user
        const user = await userService.findByEmail(email);
        if (!user) {
            await auditService.logEvent({
                action: 'LOGIN_FAILED',
                entityType: 'User',
                metadata: { email, reason: 'User not found', route: 'legacy' },
                ipAddress: req.clientIP,
                userAgent: req.get('User-Agent'),
                sessionId: req.sessionID,
                result: 'failure',
                riskLevel: 'medium'
            });
            
            return res.status(401).json({ message: 'Invalid email or password' });
        }
        
        // Check if user is active
        if (user.status !== 'active') {
            return res.status(401).json({ message: 'Account is inactive. Please contact administrator.' });
        }
        
        // Check password
        const isValid = await bcrypt.compare(password, user.password);
        if (!isValid) {
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
                route: 'legacy',
                sessionId: req.sessionID
            },
            ipAddress: req.clientIP,
            userAgent: req.get('User-Agent'),
            result: 'success',
            riskLevel: 'low'
        });
        
        console.log(`✅ Login successful for: ${user.name} (${user.source}) - legacy route`);
        
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
        console.error('Login error (legacy route):', error);
        res.status(500).json({ message: 'Internal server error' });
    }
});

// Protected routes
app.get('/dashboard', (req, res) => {
    if (!req.session.user) {
        return res.redirect('/login.html');
    }
    res.sendFile(path.join(__dirname, 'public', 'dashboard.html'));
});

app.get('/user-management', (req, res) => {
    if (!req.session.user || req.session.user.role !== 'admin') {
        return res.status(403).send('Admin access required');
    }
    res.sendFile(path.join(__dirname, 'public', 'user-management.html'));
});

// API endpoint to get current user info (for frontend)
app.get('/api/user-info', (req, res) => {
    if (!req.session.user) {
        return res.status(401).json({ message: 'Not authenticated' });
    }
    
    // Return user data directly (not wrapped in a "user" object)
    // This matches what your frontend JavaScript expects
    res.json({
        id: req.session.user.id,
        name: req.session.user.name,
        email: req.session.user.email,
        role: req.session.user.role
    });
});

// Add this to your app.js BEFORE app.use('/api/interfaces', interfacesRoutes);

// Debug route to test if routing works
app.get('/api/interfaces/debug', (req, res) => {
    console.log('🔧 DEBUG: Interfaces debug route hit');
    console.log('🔧 Session user:', req.session?.user?.email);
    res.json({ 
        message: 'Debug route working', 
        user: req.session?.user?.email || 'Not logged in',
        timestamp: new Date().toISOString() 
    });
});

// Your existing interfaces routes
app.use('/api/interfaces', interfacesRoutes);

app.use('/api/interfaces', interfacesRoutes);

// API endpoint for logout
app.post('/api/logout', (req, res) => {
    req.session.destroy((err) => {
        if (err) {
            return res.status(500).json({ message: 'Could not log out' });
        }
        res.json({ message: 'Logged out successfully' });
    });
});

// API status endpoint
app.get('/api/status', async (req, res) => {
    let pgHealth = null;
    let userCount = 0;
    
    try {
        const database = require('./config/database');
        pgHealth = await database.healthCheck();
        userCount = pgHealth.userCount || 0;
    } catch (error) {
        pgHealth = { status: 'error', error: 'PostgreSQL connection failed' };
    }
    
    res.json({
        message: 'ezHealthKonnect server is running',
        status: pgHealth?.status === 'healthy' ? 'healthy' : 'unhealthy',
        timestamp: new Date().toISOString(),
        authenticated: !!req.session.user,
        activeUsers: userCount,
        storage: {
            mode: 'postgresql_only',
            postgresql: pgHealth
        },
        compliance: {
            hipaa: process.env.HIPAA_COMPLIANCE_MODE === 'true',
            gdpr: process.env.GDPR_COMPLIANCE_MODE === 'true',
            auditLogging: 'postgresql_required'
        }
    });
});

// Main route
app.get('/', (req, res) => {
    res.redirect('/login.html');
});

// Error handling middleware
app.use((error, req, res, next) => {
    console.error('Server error:', error);
    res.status(500).json({ 
        message: 'Internal server error',
        error: process.env.NODE_ENV === 'development' ? error.message : 'Something went wrong'
    });
});


module.exports = app;