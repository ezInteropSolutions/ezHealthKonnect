// Enhanced app.js with working proxy configuration (database handled by server.js)
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
const GO_BACKEND_URL = `http://127.0.0.1:${GO_BACKEND_PORT}`;

console.log('Proxy configuration:');
console.log(`Frontend Port: ${FRONTEND_PORT}`);
console.log(`Go Backend Port: ${GO_BACKEND_PORT}`);
console.log(`Go Backend URL: ${GO_BACKEND_URL}`);
console.log(`Node Environment: ${process.env.NODE_ENV || 'development'}`);

// Basic middleware FIRST
//app.use(express.json());
//app.use(express.urlencoded({ extended: true }));
// Basic middleware FIRST - Updated for HL7-FHIR mapping payloads
app.use(express.json({ limit: '2mb' }));
app.use(express.urlencoded({ limit: '2mb', extended: true }));
// Monitor large requests for debugging
app.use((req, res, next) => {
    if (req.headers['content-length']) {
        const sizeKB = Math.round(parseInt(req.headers['content-length']) / 1024);
        if (sizeKB > 100) {
            console.log(`Large request: ${req.method} ${req.path} - ${sizeKB}KB`);
        }
    }
    next();
});
// Disable caching for JavaScript and HTML files in development
app.use((req, res, next) => {
    if (req.url.endsWith('.js') || req.url.endsWith('.html') || req.url.endsWith('.css')) {
        res.setHeader('Cache-Control', 'no-store, no-cache, must-revalidate, proxy-revalidate');
        res.setHeader('Pragma', 'no-cache');
        res.setHeader('Expires', '0');
        res.setHeader('Surrogate-Control', 'no-store');
    }
    next();
});

app.use(express.static('public'));
app.use('/uploads', express.static('uploads'));

// REQUEST LOGGING MIDDLEWARE - Add this EARLY to catch all requests
app.use((req, res, next) => {
    const timestamp = new Date().toISOString();
    console.log(`[${timestamp}] ${req.method} ${req.originalUrl} - IP: ${req.ip}`);
    
    // Log if this is an API request
    if (req.originalUrl.startsWith('/api/')) {
        console.log(`API Request detected: ${req.method} ${req.originalUrl}`);
        
        // Check if this should be proxied
        if (req.originalUrl.startsWith('/api/fhir') ||
            req.originalUrl.startsWith('/api/hl7') ||
            req.originalUrl.startsWith('/api/system') ||
            req.originalUrl.startsWith('/api/processing') ||
            req.originalUrl.startsWith('/api/mllp') ||
            req.originalUrl.startsWith('/api/database') ||
            req.originalUrl.startsWith('/api/pipeline')) {
            console.log(`Should be proxied to Go backend: ${GO_BACKEND_URL}${req.originalUrl}`);
        } else {
            console.log(`Should be handled by Node.js locally`);
        }
    }

    // Special debugging for user-info requests
    if (req.originalUrl.includes('user-info')) {
        console.log('🔥🔥🔥 USER-INFO REQUEST DETECTED:');
        console.log('🔥 Full URL:', req.originalUrl);
        console.log('🔥 Method:', req.method);
        console.log('🔥 Cookies:', req.headers.cookie);
        console.log('🔥 Session middleware loaded:', typeof req.sessionID !== 'undefined' ? 'YES' : 'NO');
    }

    next();
});

// TEST GO BACKEND CONNECTIVITY
async function testGoBackendConnectivity() {
    console.log('Testing Go backend connectivity...');
    
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
            console.log('Go backend is responsive:', data);
            return true;
        } else {
            console.log(`Go backend returned status ${response.status}`);
            return false;
        }
    } catch (error) {
        console.error('Go backend connectivity test failed:', error.message);
        return false;
    }
}

// ========================================================================
// SIMPLE WORKING PROXY - NO EXTERNAL MIDDLEWARE
// ========================================================================

console.log('Setting up simple proxy...');

// Helper function to forward requests to Go backend
const forwardToGo = async (req, res) => {
    const targetUrl = `${GO_BACKEND_URL}${req.originalUrl}`;
    
    console.log(`FORWARDING: ${req.method} ${req.originalUrl} -> ${targetUrl}`);
    
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

        // Forward authenticated user identity to Go backend.
        // req.user is set by JWT verifyToken; req.session.user by session-based requireAuth.
        if (req.user && req.user.id) {
            options.headers['X-User-ID'] = String(req.user.id);
        } else if (req.session && req.session.user && req.session.user.id) {
            options.headers['X-User-ID'] = String(req.session.user.id);
        }
        
        // Add body for POST/PUT/PATCH requests
        if (['POST', 'PUT', 'PATCH'].includes(req.method) && req.body) {
            options.body = JSON.stringify(req.body);
        }
        
        // Make the request to Go backend
        const response = await fetch(targetUrl, options);
        
        console.log(`Response: ${response.status} ${response.statusText}`);
        
        const contentType = response.headers.get('content-type') || '';

        // Stream SSE responses directly — do not buffer them
        if (contentType.includes('text/event-stream')) {
            res.status(response.status);
            res.set('Content-Type', 'text/event-stream');
            res.set('Cache-Control', 'no-cache');
            res.set('X-Accel-Buffering', 'no');
            res.set('Connection', 'keep-alive');
            response.body.pipe(res);
            response.body.on('end', () => res.end());
            return; // pipe handles the rest
        }

        // Get response data
        let data;
        if (contentType.includes('application/json')) {
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

        console.log(`PROXY SUCCESS: ${response.status} for ${req.originalUrl}`);
        
    } catch (error) {
        console.error(`PROXY ERROR for ${req.originalUrl}:`, error.message);
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
app.use('/api/processing', forwardToGo);  // NEW: Processing engine routes
app.use('/api/mllp', forwardToGo);        // NEW: MLLP connectivity routes
app.use('/api/connectivity', forwardToGo); // Connector types + interface connectivity
app.use('/api/ai', forwardToGo);           // Local AI assistant (Ollama/RAG — no PHI leaves network)
app.use('/api/code-templates', forwardToGo); // Code Template Libraries (JS function injection into script steps)
// /api/git is registered below (after session middleware, with requireAuth)
app.post('/api/pipeline/reference-variables', forwardToGo);  // Exact match - avoid intercepting /api/pipelines/*

// Database query testing routes - use explicit route matching
app.post('/api/database/test-query', forwardToGo);  // NEW: Database query testing
console.log('✅ Database test-query route registered');
console.log('✅ Pipeline routes registered (generic, reusable)');

// File parser routes - preview + template listing + server file browser
app.get('/api/file-parser/browse', forwardToGo);   // explicit GET before the catch-all use()
app.use('/api/file-parser', forwardToGo);           // file-parser/preview + file-parser/templates
console.log('✅ File parser routes registered (preview + templates + browse)');

console.log('Simple proxy configured successfully');

// Session configuration
// SESSION_TIMEOUT_MINUTES can be set by server.js after reading system_settings at startup.
// Default: 1440 minutes (24 h).  Admin can change via Settings → Security.
const _sessionTimeoutMinutes = parseInt(process.env.SESSION_TIMEOUT_MINUTES || '1440', 10);
const sessionConfig = {
    secret: process.env.SESSION_SECRET || 'your-session-secret',
    resave: false,
    saveUninitialized: false,
    name: 'ezhealth.sid',
    cookie: {
        secure: process.env.NODE_ENV === 'production',
        httpOnly: true,
        maxAge: _sessionTimeoutMinutes * 60 * 1000,
        sameSite: 'lax'
    }
};

app.use(session(sessionConfig));

// Analytics + Alerts + Git + Migration routes — require session auth before forwarding to Go
const { requireAuth: _analyticsAuth } = require('./middleware/auth');
app.use('/api/analytics', _analyticsAuth, forwardToGo);
app.use('/api/alerts',    _analyticsAuth, forwardToGo);
// Override the early /api/git registration with an auth-protected version (session is now initialized)
app.use('/api/git',       _analyticsAuth, forwardToGo);
// Mirth Connect migration — auth required
app.use('/api/migration', _analyticsAuth, forwardToGo);

// Add request ID for audit logging
app.use((req, res, next) => {
    req.requestId = require('crypto').randomUUID();
    req.clientIP = req.ip || req.connection.remoteAddress || 'unknown';
    next();
});

// Initialize services (database connection handled by server.js)
const interfaceService = require('./services/interfaceService');
const userService = require('./services/userService');
const auditService = require('./services/auditService');

// Load routes
console.log('Loading local API routes...');

const authRoutes = require('./routes/auth');
const userRoutes = require('./routes/users');
const interfacesRoutes = require('./routes/interfacesRoutes');
const wizardRoutes = require('./routes/wizardRoutes');

console.log('🔄 About to require messageRoutes...');
try {
    const messageRoutes = require('./routes/messageRoutes');
    console.log('✅ messageRoutes loaded successfully');
} catch (error) {
    console.error('❌ Failed to load messageRoutes:', error.message);
    console.error('❌ Stack:', error.stack);
    throw error;
}
const messageRoutes = require('./routes/messageRoutes');

// Mount essential routes first (before problematic ones)
console.log('🔄 Mounting /api/auth...');
app.use('/api/auth', authRoutes);
console.log('🔄 Mounting /api/users...');
app.use('/api/users', userRoutes);
console.log('🔄 Mounting /api/audit-logs...');
const auditLogRoutes = require('./controllers/auditController');
app.use('/api/audit-logs', auditLogRoutes);
console.log('✅ Audit log routes mounted');
console.log('🔄 Mounting /api/interfaces...');
app.use('/api/interfaces', interfacesRoutes);
console.log('🔄 Mounting /api/interface-templates...');
const interfaceTemplateRoutes = require('./routes/interfaceTemplateRoutes');
app.use('/api/interface-templates', interfaceTemplateRoutes);
// Save-as-template endpoints live under /api/interfaces/:id/...
const ifaceTemplateCtrl = require('./controllers/interfaceTemplateController');
const { requireAuth: _isAuth } = require('./middleware/auth');
app.post('/api/interfaces/:interfaceId/save-as-template', _isAuth, ifaceTemplateCtrl.saveInterfaceAsTemplate);
app.post('/api/interfaces/:interfaceId/template-preview', ifaceTemplateCtrl.previewSanitization);
console.log('✅ Interface template routes mounted');
console.log('🔄 Mounting /api/wizard...');
app.use('/api/wizard', wizardRoutes);
// Forward object-storage endpoints to Go BEFORE local message routes
app.get('/api/messages/:messageId/raw', forwardToGo);
app.get('/api/messages/:messageId/transformed', forwardToGo);
app.get('/api/messages/:messageId/logs', forwardToGo);
console.log('🔄 Mounting /api/messages...');
app.use('/api/messages', messageRoutes);

// Schema routes (HL7/FHIR schema loading for XPath IntelliSense)
console.log('🔄 Mounting /api/schemas...');
const schemaRoutes = require('./routes/schemaRoutes');
app.use('/api/schemas', schemaRoutes);
console.log('✅ Schema routes mounted at /api/schemas');

// Deployment management routes
console.log('🔄 Mounting /api/deployment...');
const deploymentRoutes = require('./routes/deploymentRoutes');
app.use('/api/deployment', deploymentRoutes);
console.log('✅ Deployment routes mounted at /api/deployment');

// FHIR Receiver routes - Node.js handles incoming FHIR resources
console.log('🔄 Mounting /fhir (FHIR Receiver)...');
const fhirReceiverRoutes = require('./routes/fhirReceiverRoutes');
app.use('/fhir', fhirReceiverRoutes);
console.log('✅ FHIR Receiver routes mounted at /fhir');

// Pipeline Builder routes - with debug logging
console.log('🔄 Mounting /api/pipelines...');
try {
    const pipelineRoutes = require('./routes/pipelineRoutes');
    console.log('✅ pipelineRoutes loaded successfully');
    app.use('/api', pipelineRoutes);
    console.log('✅ Pipeline routes mounted successfully');
} catch (error) {
    console.error('❌ Failed to load pipeline routes:', error.message);
    console.error('Stack:', error.stack);
}

console.log('✅ Essential routes mounted successfully');

console.log('About to require interfaceLifecycle routes...');
try {
    const interfaceLifecycleRoutes = require('./routes/interfaceLifecycle');
    console.log('✅ interfaceLifecycle routes loaded successfully');

    console.log('About to mount /api/runtime routes...');
    app.use('/api/runtime', interfaceLifecycleRoutes);
    console.log('✅ /api/runtime routes mounted successfully');

} catch (error) {
    console.error('❌ Failed to load interfaceLifecycle routes:', error.message);
    console.error('Stack:', error.stack);
    console.log('⚠️ Continuing without /api/runtime routes');
}

console.log('Local API routes mounted');

// DEBUG MIDDLEWARE - TRACE ALL REQUESTS TO USER-INFO
app.use('/api/user-info', (req, res, next) => {
    console.log('🚨 MIDDLEWARE: /api/user-info request intercepted');
    console.log('🚨 Method:', req.method);
    console.log('🚨 URL:', req.url);
    console.log('🚨 Original URL:', req.originalUrl);
    console.log('🚨 Headers:', req.headers);
    next();
});

// TEST ROUTE TO VERIFY SERVER IS WORKING
app.get('/api/test-route', (req, res) => {
    console.log('🧪 TEST ROUTE HIT!');
    res.json({ message: 'Test route works', timestamp: new Date().toISOString() });
});

// USER INFO ROUTE - MUST BE BEFORE MESSAGE ROUTES TO AVOID CONFLICTS
app.get('/api/user-info', (req, res) => {
    try {
        console.log('🔍 /api/user-info called');
        console.log('Session ID:', req.sessionID);
        console.log('Session exists:', !!req.session);
        console.log('Session user:', req.session ? req.session.user : 'No session');
        console.log('Cookie header:', req.headers.cookie);

        if (!req.session || !req.session.user) {
            console.log('❌ No session user, returning 401');
            return res.status(401).json({ message: 'Not authenticated' });
        }

        console.log('✅ User authenticated:', req.session.user.name);
        res.json({
            id: req.session.user.id,
            name: req.session.user.name,
            email: req.session.user.email,
            role: req.session.user.role
        });
    } catch (error) {
        console.error('❌ Error in /api/user-info:', error);
        res.status(500).json({ message: 'Internal server error', error: error.message });
    }
});

// DASHBOARD: Recent activity from audit_logs
app.get('/api/dashboard/recent-activity', async (req, res) => {
    if (!req.session || !req.session.user) return res.status(401).json({ success: false });
    try {
        const { sequelize } = require('./config/database');
        const { QueryTypes } = require('sequelize');
        const rows = await sequelize.query(`
            SELECT
                al.action,
                al.entity_type,
                al.result,
                al.created_at,
                u.name AS user_name
            FROM audit_logs al
            LEFT JOIN users u ON u.id = al.user_id
            ORDER BY al.created_at DESC
            LIMIT 8
        `, { type: QueryTypes.SELECT });
        res.json({ success: true, activity: rows });
    } catch (e) {
        res.json({ success: false, error: e.message });
    }
});

// PROXY TEST ROUTES FOR DEBUGGING
app.get('/api/proxy/test', async (req, res) => {
    console.log('Proxy test endpoint called');
    
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

// Legacy login route
app.post('/api/login', async (req, res) => {
    try {
        const { email, password } = req.body;
        
        console.log(`Login attempt for: ${email} (legacy route)`);
        
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
        
        console.log(`Login successful for: ${user.name}`);
        
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
    // JSON body parse errors (e.g. sending non-JSON with Content-Type: application/json)
    if (error.type === 'entity.parse.failed' || error instanceof SyntaxError) {
        return res.status(400).json({ success: false, error: error.message });
    }
    console.error('Server error:', error);
    res.status(500).json({
        message: 'Internal server error',
        error: process.env.NODE_ENV === 'development' ? error.message : 'Something went wrong'
    });
});

module.exports = app;