// Enhanced app.js with simple working proxy configuration
const express = require('express');
const path = require('path');
const session = require('express-session');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');

const app = express();

// Load environment variables
require('dotenv').config();

// DEBUGGING: Check environment and ports
const FRONTEND_PORT = process.env.PORT || 3000;
const GO_BACKEND_PORT = process.env.API_PORT || 8080;
const GO_BACKEND_URL = `http://localhost:${GO_BACKEND_PORT}`;

console.log('🔧 === PROXY CONFIGURATION DEBUG ===');
console.log(`📍 Frontend Port: ${FRONTEND_PORT}`);
console.log(`📍 Go Backend Port: ${GO_BACKEND_PORT}`);
console.log(`📍 Go Backend URL: ${GO_BACKEND_URL}`);
console.log(`📍 Node Environment: ${process.env.NODE_ENV || 'development'}`);
console.log('=====================================');

// Basic middleware FIRST
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
app.use(express.static('public'));
app.use('/uploads', express.static('uploads'));

// REQUEST LOGGING MIDDLEWARE - Add this EARLY to catch all requests
app.use((req, res, next) => {
    const timestamp = new Date().toISOString();
    console.log(`🌐 [${timestamp}] ${req.method} ${req.originalUrl} - IP: ${req.ip}`);
    
    // Log if this is an API request
    if (req.originalUrl.startsWith('/api/')) {
        console.log(`🔍 API Request detected: ${req.method} ${req.originalUrl}`);
        
        // Check if this should be proxied
        if (req.originalUrl.startsWith('/api/fhir') || 
            req.originalUrl.startsWith('/api/hl7') || 
            req.originalUrl.startsWith('/api/system')) {
            console.log(`🎯 Should be proxied to Go backend: ${GO_BACKEND_URL}${req.originalUrl}`);
        } else {
            console.log(`📝 Should be handled by Node.js locally`);
        }
    }
    
    next();
});

// TEST GO BACKEND CONNECTIVITY
async function testGoBackendConnectivity() {
    console.log('🧪 Testing Go backend connectivity...');
    
    try {
        // Try to use node-fetch, fallback to native fetch
        let fetch;
        try {
            fetch = require('node-fetch');
        } catch {
            fetch = global.fetch;
            if (!fetch) {
                throw new Error('No fetch implementation available');
            }
        }
        
        const response = await fetch(`${GO_BACKEND_URL}/api/system/health`, {
            method: 'GET',
            timeout: 5000
        });
        
        if (response.ok) {
            const data = await response.json();
            console.log('✅ Go backend is responsive:', data);
            return true;
        } else {
            console.log(`⚠️ Go backend returned status ${response.status}`);
            return false;
        }
    } catch (error) {
        console.error('❌ Go backend connectivity test failed:', error.message);
        return false;
    }
}

// ========================================================================
// SIMPLE WORKING PROXY - NO EXTERNAL MIDDLEWARE
// ========================================================================

console.log('🔧 Setting up SIMPLE working proxy (no middleware dependencies)...');

// Helper function to forward requests to Go backend
const forwardToGo = async (req, res) => {
    const targetUrl = `${GO_BACKEND_URL}${req.originalUrl}`;
    
    console.log(`🎯 FORWARDING: ${req.method} ${req.originalUrl} -> ${targetUrl}`);
    
    try {
        // Get fetch implementation
        let fetch;
        try {
            fetch = require('node-fetch');
        } catch {
            fetch = global.fetch;
        }
        
        if (!fetch) {
            throw new Error('No fetch implementation available');
        }
        
        // Prepare request options
        const options = {
            method: req.method,
            headers: {
                'Content-Type': req.get('Content-Type') || 'application/json',
                'Accept': req.get('Accept') || 'application/json',
                'User-Agent': req.get('User-Agent') || 'Node.js Proxy'
            }
        };
        
        // Add body for POST/PUT/PATCH requests
        if (['POST', 'PUT', 'PATCH'].includes(req.method) && req.body) {
            options.body = JSON.stringify(req.body);
        }
        
        console.log(`📡 Request options:`, options);
        
        // Make the request to Go backend
        const response = await fetch(targetUrl, options);
        
        console.log(`📥 Response: ${response.status} ${response.statusText}`);
        
        // Get response data
        let data;
        const contentType = response.headers.get('content-type');
        
        if (contentType && contentType.includes('application/json')) {
            data = await response.json();
        } else {
            data = await response.text();
        }
        
        // Copy important response headers
        const headersToForward = ['content-type', 'cache-control', 'expires', 'last-modified'];
        headersToForward.forEach(header => {
            const value = response.headers.get(header);
            if (value) {
                res.set(header, value);
            }
        });
        
        // Send response with correct status
        res.status(response.status);
        if (typeof data === 'string') {
            res.send(data);
        } else {
            res.json(data);
        }
        
        console.log(`✅ PROXY SUCCESS: ${response.status} for ${req.originalUrl}`);
        
    } catch (error) {
        console.error(`❌ PROXY ERROR for ${req.originalUrl}:`, error.message);
        res.status(500).json({
            error: 'Go backend service unavailable',
            details: error.message,
            path: req.originalUrl,
            target: targetUrl,
            timestamp: new Date().toISOString()
        });
    }
};

// Apply explicit route handlers for Go backend routes
app.use('/api/fhir', forwardToGo);
app.use('/api/hl7', forwardToGo);
app.use('/api/system', forwardToGo);

console.log('✅ Simple proxy configured successfully');
console.log('📋 Proxy routes:');
console.log('   /api/fhir/* -> Go Backend');
console.log('   /api/hl7/* -> Go Backend');
console.log('   /api/system/* -> Go Backend');
console.log('   All other /api/* -> Node.js');

// ========================================================================
// END OF PROXY CONFIGURATION
// ========================================================================

// PROXY TEST ROUTES FOR DEBUGGING
app.get('/api/proxy/test', async (req, res) => {
    console.log('🧪 Proxy test endpoint called');
    
    const backendHealthy = await testGoBackendConnectivity();
    
    res.json({
        success: true,
        proxy: {
            frontend_port: FRONTEND_PORT,
            backend_port: GO_BACKEND_PORT,
            backend_url: GO_BACKEND_URL,
            backend_healthy: backendHealthy,
            proxy_type: 'custom_simple',
            timestamp: new Date().toISOString()
        },
        test_urls: {
            fhir: `http://localhost:${FRONTEND_PORT}/api/fhir/transform/resources/test/ADT^A01`,
            hl7: `http://localhost:${FRONTEND_PORT}/api/hl7/stats`,
            system: `http://localhost:${FRONTEND_PORT}/api/system/health`
        }
    });
});

app.get('/api/proxy/test-direct-backend', async (req, res) => {
    console.log('🧪 Testing direct backend connection...');
    
    try {
        let fetch;
        try {
            fetch = require('node-fetch');
        } catch {
            fetch = global.fetch;
        }
        
        const response = await fetch(`${GO_BACKEND_URL}/api/system/health`);
        const data = await response.json();
        
        res.json({
            success: true,
            message: 'Direct backend connection successful',
            backend_response: data,
            backend_status: response.status
        });
    } catch (error) {
        res.status(500).json({
            success: false,
            error: 'Direct backend connection failed',
            details: error.message
        });
    }
});

// Session configuration
const sessionConfig = {
    secret: process.env.SESSION_SECRET || 'your-session-secret',
    resave: false,
    saveUninitialized: false,
    name: 'ezhealth.sid',
    cookie: { 
        secure: process.env.NODE_ENV === 'production',
        httpOnly: true,
        maxAge: 24 * 60 * 60 * 1000,
        sameSite: 'strict'
    }
};

app.use(session(sessionConfig));

// Add request ID for audit logging
app.use((req, res, next) => {
    req.requestId = require('crypto').randomUUID();
    req.clientIP = req.ip || req.connection.remoteAddress || 'unknown';
    next();
});

// IMPORTANT: All other routes AFTER proxy routes
const authRoutes = require('./routes/auth');
const userRoutes = require('./routes/users');
const interfacesRoutes = require('./routes/interfacesRoutes');

console.log('📁 Loading local API routes...');

// Mount LOCAL API routes (these should NOT conflict with proxy routes)
app.use('/api/auth', authRoutes);
app.use('/api/users', userRoutes);
app.use('/api/interfaces', interfacesRoutes);

console.log('✅ Local API routes mounted');

// Legacy login route
app.post('/api/login', async (req, res) => {
    try {
        const userService = require('./services/userService');
        const auditService = require('./services/auditService');
        
        const { email, password } = req.body;
        
        console.log(`🔐 Login attempt for: ${email} (legacy route)`);
        
        const user = await userService.findByEmail(email);
        if (!user) {
            return res.status(401).json({ message: 'Invalid email or password' });
        }
        
        if (user.status !== 'active') {
            return res.status(401).json({ message: 'Account is inactive. Please contact administrator.' });
        }
        
        const isValid = await bcrypt.compare(password, user.password);
        if (!isValid) {
            return res.status(401).json({ message: 'Invalid email or password' });
        }
        
        await userService.updateLastLogin(user.id, user.email, req.clientIP);
        
        const token = jwt.sign(
            { userId: user.id, email: user.email, role: user.role },
            process.env.JWT_SECRET || 'your-secret-key-change-this-in-production',
            { expiresIn: '24h' }
        );
        
        req.session.user = { 
            id: user.id, 
            email: user.email, 
            name: user.name,
            role: user.role 
        };
        
        console.log(`✅ Login successful for: ${user.name}`);
        
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
        res.status(500).json({ message: 'Internal server error' });
    }
});

// Other routes...
app.get('/dashboard', (req, res) => {
    if (!req.session.user) {
        return res.redirect('/login.html');
    }
    res.sendFile(path.join(__dirname, 'public', 'dashboard.html'));
});

app.get('/api/user-info', (req, res) => {
    if (!req.session.user) {
        return res.status(401).json({ message: 'Not authenticated' });
    }
    
    res.json({
        id: req.session.user.id,
        name: req.session.user.name,
        email: req.session.user.email,
        role: req.session.user.role
    });
});

app.post('/api/logout', (req, res) => {
    req.session.destroy((err) => {
        if (err) {
            return res.status(500).json({ message: 'Could not log out' });
        }
        res.json({ message: 'Logged out successfully' });
    });
});

// Enhanced status endpoint with proxy info
app.get('/api/status', async (req, res) => {
    const backendHealthy = await testGoBackendConnectivity();
    
    res.json({
        message: 'ezHealthKonnect server is running',
        status: 'healthy',
        timestamp: new Date().toISOString(),
        authenticated: !!req.session.user,
        proxy: {
            enabled: true,
            backend_url: GO_BACKEND_URL,
            backend_healthy: backendHealthy,
            proxy_type: 'custom_simple'
        },
        ports: {
            frontend: FRONTEND_PORT,
            backend: GO_BACKEND_PORT
        }
    });
});

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

// Start server with connectivity test
const startServer = async () => {
    console.log('🚀 Starting ezHealthKonnect server...');
    
    // Test backend connectivity
    const backendHealthy = await testGoBackendConnectivity();
    if (!backendHealthy) {
        console.warn('⚠️ Go backend is not responding. Proxy routes may not work.');
        console.warn(`⚠️ Make sure Go backend is running on ${GO_BACKEND_URL}`);
    }
    
    app.listen(FRONTEND_PORT, () => {
        console.log(`✅ Frontend server running on port ${FRONTEND_PORT}`);
        console.log(`🔗 Frontend URL: http://localhost:${FRONTEND_PORT}`);
        console.log(`🎯 Backend URL: ${GO_BACKEND_URL}`);
        console.log(`🧪 Test proxy: http://localhost:${FRONTEND_PORT}/api/proxy/test`);
        console.log(`🧪 Test FHIR: http://localhost:${FRONTEND_PORT}/api/fhir/transform/resources/test/ADT^A01`);
        console.log(`🧪 Test System: http://localhost:${FRONTEND_PORT}/api/system/health`);
        console.log('='.repeat(80));
        console.log('🎯 SIMPLE CUSTOM PROXY CONFIGURED');
        console.log('📋 No external proxy middleware - direct HTTP forwarding');
        console.log('✅ This eliminates all path stripping issues');
        console.log('='.repeat(80));
    });
};

if (require.main === module) {
    startServer();
}

module.exports = app;