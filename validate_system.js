// validate_system.js
// Docker-aware system validation script for Interface-Centric Configuration Engine

const http = require('http');
const https = require('https');
const { exec } = require('child_process');

console.log('🔍 ezHealthKonnect Docker System Validation');
console.log('==========================================');

// Docker Configuration
const NODE_PORT = process.env.PORT || 3000;
const GO_PORT = process.env.API_PORT || 8080;
const BASE_URL = `http://localhost:${NODE_PORT}`;
const GO_BASE_URL = `http://localhost:${GO_PORT}`;

// Docker service names for internal communication
const DOCKER_SERVICES = {
    'node-frontend': `http://localhost:${NODE_PORT}`,
    'go-backend': `http://localhost:${GO_PORT}`,
    'postgres': 'postgresql://localhost:5432',
    'mongodb': 'mongodb://localhost:27017'
};

// Test endpoints
const endpoints = [
    // Node.js endpoints
    { name: 'Node.js Health', url: `${BASE_URL}/api/auth/health`, expected: 200 },
    { name: 'Node.js Interfaces', url: `${BASE_URL}/api/interfaces`, expected: 200 },

    // Go backend endpoints
    { name: 'Go System Health', url: `${GO_BASE_URL}/api/system/health`, expected: 200 },
    { name: 'Go FHIR Health', url: `${GO_BASE_URL}/api/fhir/transform/schemas`, expected: 200 },
    { name: 'Go HL7 Stats', url: `${GO_BASE_URL}/api/hl7/stats`, expected: 200 },
    { name: 'Go MLLP Health', url: `${GO_BASE_URL}/api/mllp/health`, expected: 200 },

    // Configuration Engine endpoints
    { name: 'Config Health', url: `${GO_BASE_URL}/api/config/health`, expected: [200, 503] },
    { name: 'Config Interfaces', url: `${GO_BASE_URL}/api/config/interfaces`, expected: [200, 503] },
    { name: 'Config Runtime Stats', url: `${GO_BASE_URL}/api/config/runtime/stats`, expected: [200, 503] },

    // Transformation Engine endpoints
    { name: 'Transform Health', url: `${GO_BASE_URL}/api/transform/status`, expected: 200 },
    { name: 'Transform Metrics', url: `${GO_BASE_URL}/api/transform/metrics`, expected: 200 },
];

// Test results
let results = [];
let passed = 0;
let total = endpoints.length;

// Helper function to make HTTP requests
function makeRequest(url) {
    return new Promise((resolve, reject) => {
        const urlObj = new URL(url);
        const client = urlObj.protocol === 'https:' ? https : http;

        const req = client.request(url, {
            method: 'GET',
            timeout: 5000
        }, (res) => {
            resolve({
                statusCode: res.statusCode,
                headers: res.headers
            });
        });

        req.on('error', (error) => {
            reject(error);
        });

        req.on('timeout', () => {
            req.destroy();
            reject(new Error('Request timeout'));
        });

        req.end();
    });
}

// Test function
async function testEndpoint(endpoint) {
    try {
        const result = await makeRequest(endpoint.url);
        const expectedStatuses = Array.isArray(endpoint.expected) ? endpoint.expected : [endpoint.expected];
        const success = expectedStatuses.includes(result.statusCode);

        return {
            name: endpoint.name,
            url: endpoint.url,
            status: result.statusCode,
            success: success,
            error: null
        };
    } catch (error) {
        return {
            name: endpoint.name,
            url: endpoint.url,
            status: null,
            success: false,
            error: error.message
        };
    }
}

// Docker service check function
function checkDockerServices() {
    return new Promise((resolve) => {
        exec('docker-compose ps --services --filter status=running', (error, stdout, stderr) => {
            if (error) {
                console.log('⚠️ Docker Compose not available or not running');
                resolve([]);
                return;
            }

            const runningServices = stdout.trim().split('\n').filter(s => s.length > 0);
            console.log('🐳 Docker Services Status:');

            const expectedServices = ['postgres', 'mongodb'];
            expectedServices.forEach(service => {
                const isRunning = runningServices.includes(service);
                console.log(`   ${isRunning ? '✅' : '❌'} ${service}: ${isRunning ? 'running' : 'not running'}`);
            });

            resolve(runningServices);
        });
    });
}

// Main validation function
async function validateSystem() {
    // Check Docker services first
    console.log('\n🐳 Checking Docker Services...\n');
    const dockerServices = await checkDockerServices();

    console.log(`\n📡 Testing ${total} endpoints...\n`);

    for (const endpoint of endpoints) {
        const result = await testEndpoint(endpoint);
        results.push(result);

        if (result.success) {
            console.log(`✅ ${result.name} - Status: ${result.status}`);
            passed++;
        } else {
            const errorInfo = result.error || `Status: ${result.status}`;
            console.log(`❌ ${result.name} - ${errorInfo}`);
        }
    }

    // Summary
    console.log('\n' + '='.repeat(50));
    console.log(`📊 Test Results: ${passed}/${total} endpoints passed`);

    if (passed === total) {
        console.log('🎉 All systems operational!');
        console.log('\n✅ System Status: HEALTHY');
        process.exit(0);
    } else {
        console.log(`⚠️  ${total - passed} endpoint(s) failed`);
        console.log('\n⚠️  System Status: PARTIAL');

        // Detailed failure analysis
        console.log('\n🔍 Failed Endpoints Analysis:');
        const failures = results.filter(r => !r.success);
        failures.forEach(failure => {
            console.log(`  • ${failure.name}: ${failure.error || failure.status}`);

            if (failure.name.includes('Config') && failure.status === 503) {
                console.log('    💡 MongoDB Configuration Engine may not be running');
                console.log('    💡 Set MONGODB_URI environment variable to enable');
            }
        });

        console.log('\n📋 System Status Breakdown:');
        console.log(`   Node.js Frontend: ${results.filter(r => r.url.includes(`:${NODE_PORT}`)).filter(r => r.success).length}/${results.filter(r => r.url.includes(`:${NODE_PORT}`)).length} endpoints`);
        console.log(`   Go Backend: ${results.filter(r => r.url.includes(`:${GO_PORT}`) && !r.name.includes('Config')).filter(r => r.success).length}/${results.filter(r => r.url.includes(`:${GO_PORT}`) && !r.name.includes('Config')).length} endpoints`);
        console.log(`   Configuration Engine: ${results.filter(r => r.name.includes('Config')).filter(r => r.success).length}/${results.filter(r => r.name.includes('Config')).length} endpoints`);

        process.exit(1);
    }
}

// Environment check
console.log('🔧 Environment Configuration:');
console.log(`   NODE_ENV: ${process.env.NODE_ENV || 'not set'}`);
console.log(`   Node.js Port: ${NODE_PORT}`);
console.log(`   Go Backend Port: ${GO_PORT}`);
console.log(`   MongoDB URI: ${process.env.MONGODB_URI ? 'configured' : 'not set'}`);
console.log(`   Database URL: ${process.env.DATABASE_URL ? 'configured' : 'not set'}`);

// Wait for services to be ready
console.log('\n⏳ Waiting 3 seconds for services to initialize...');
setTimeout(() => {
    validateSystem().catch(error => {
        console.error('❌ System validation failed:', error);
        process.exit(1);
    });
}, 3000);