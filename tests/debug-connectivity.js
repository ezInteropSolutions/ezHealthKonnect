// Debug script to test connectivity between Node.js (3000) and Go (8080)
// Save as: debug-connectivity.js
// Run with: node debug-connectivity.js

let fetch;
try {
    fetch = require('node-fetch');
    console.log('✅ Loaded node-fetch for HTTP requests');
} catch (err) {
    if (typeof global.fetch !== 'undefined') {
        fetch = global.fetch;
        console.log('✅ Using global fetch');
    } else {
        console.error('❌ No fetch implementation available');
        console.error('💡 Install node-fetch: npm install node-fetch');
        process.exit(1);
    }
}

const FRONTEND_PORT = process.env.PORT || 3000;
const BACKEND_PORT = process.env.API_PORT || 8080;

const FRONTEND_URL = `http://localhost:${FRONTEND_PORT}`;
const BACKEND_URL = `http://localhost:${BACKEND_PORT}`;

console.log('🔍 === CONNECTIVITY DEBUG SCRIPT ===');
console.log(`Frontend: ${FRONTEND_URL}`);
console.log(`Backend: ${BACKEND_URL}`);
console.log('=====================================\n');

async function testEndpoint(url, description) {
    try {
        console.log(`🧪 Testing: ${description}`);
        console.log(`📡 URL: ${url}`);

        const startTime = Date.now();
        const response = await fetch(url, {
            method: 'GET',
            headers: {
                'Accept': 'application/json',
                'User-Agent': 'Connectivity-Debug-Script'
            },
            timeout: 10000
        });
        const endTime = Date.now();

        console.log(`⏱️  Response time: ${endTime - startTime}ms`);
        console.log(`📥 Status: ${response.status} ${response.statusText}`);

        if (response.ok) {
            try {
                const data = await response.json();
                console.log(`✅ SUCCESS: ${description}`);
                console.log(`📄 Response:`, JSON.stringify(data, null, 2));
            } catch (jsonError) {
                const text = await response.text();
                console.log(`✅ SUCCESS: ${description} (non-JSON response)`);
                console.log(`📄 Response:`, text.substring(0, 200));
            }
        } else {
            console.log(`❌ FAILED: ${description}`);
            try {
                const errorData = await response.json();
                console.log(`📄 Error response:`, JSON.stringify(errorData, null, 2));
            } catch {
                const errorText = await response.text();
                console.log(`📄 Error response:`, errorText.substring(0, 200));
            }
        }

        return response.ok;
    } catch (error) {
        console.log(`❌ ERROR: ${description}`);
        console.log(`🔥 Error details: ${error.message}`);

        if (error.code === 'ECONNREFUSED') {
            console.log(`💡 Server is not running on this port`);
        } else if (error.code === 'ETIMEDOUT') {
            console.log(`💡 Request timed out - server may be running but not responding`);
        }

        return false;
    } finally {
        console.log('─'.repeat(60) + '\n');
    }
}

async function runTests() {
    console.log('Starting connectivity tests...\n');

    const tests = [
        { url: `${BACKEND_URL}/api/system/health`, description: 'Direct Go Backend - System Health' },
        { url: `${BACKEND_URL}/api/fhir/transform/resources/test/ADT^A01`, description: 'Direct Go Backend - FHIR Resources' },
        { url: `${BACKEND_URL}/health`, description: 'Direct Go Backend - Basic Health' },

        { url: `${FRONTEND_URL}/api/status`, description: 'Node.js Frontend - Status' },
        { url: `${FRONTEND_URL}/api/proxy/test`, description: 'Node.js Frontend - Proxy Test' },

        { url: `${FRONTEND_URL}/api/system/health`, description: 'Proxy Route - System Health (should route to Go)' },
        { url: `${FRONTEND_URL}/api/fhir/transform/resources/test/ADT^A01`, description: 'Proxy Route - FHIR Resources (should route to Go)' },
        { url: `${FRONTEND_URL}/api/hl7/stats`, description: 'Proxy Route - HL7 Stats (should route to Go)' }
    ];

    const results = {};

    for (const test of tests) {
        results[test.description] = await testEndpoint(test.url, test.description);
        await new Promise(resolve => setTimeout(resolve, 1000)); // Delay between tests
    }

    // Summary
    console.log('🏁 === TEST RESULTS SUMMARY ===');
    const successful = Object.values(results).filter(Boolean).length;
    const total = Object.keys(results).length;

    console.log(`📊 Success Rate: ${successful}/${total} (${Math.round(successful / total * 100)}%)`);
    console.log('\n📝 Detailed Results:');
    Object.entries(results).forEach(([test, success]) => {
        console.log(`${success ? '✅' : '❌'} ${test}`);
    });

    // Troubleshooting tips
    console.log('\n🔧 === TROUBLESHOOTING GUIDE ===');

    if (!results['Direct Go Backend - System Health']) {
        console.log('❌ Go backend is not running or not accessible');
        console.log('💡 Solutions:');
        console.log('   1. Start the Go backend: go run main.go');
        console.log('   2. Check if port 8080 is available');
        console.log('   3. Verify API_PORT environment variable');
    }

    if (!results['Node.js Frontend - Status']) {
        console.log('❌ Node.js frontend is not running');
        console.log('💡 Solutions:');
        console.log('   1. Start the frontend: npm start or node app.js');
        console.log('   2. Check if port 3000 is available');
        console.log('   3. Verify PORT environment variable');
    }

    if (results['Direct Go Backend - System Health'] &&
        results['Node.js Frontend - Status'] &&
        !results['Proxy Route - System Health (should route to Go)']) {
        console.log('❌ Proxy routing is not working');
        console.log('💡 Solutions:');
        console.log('   1. Install http-proxy-middleware: npm install http-proxy-middleware');
        console.log('   2. Check proxy configuration in app.js');
        console.log('   3. Verify route order (proxy routes must be first)');
        console.log('   4. Check for route conflicts');
    }

    console.log('\n🎯 === EXPECTED BEHAVIOR ===');
    console.log('✅ Direct backend tests should pass (Go server running)');
    console.log('✅ Frontend tests should pass (Node.js server running)');
    console.log('✅ Proxy tests should return Go backend responses');
    console.log('✅ Proxy responses should match direct backend responses');

    console.log('\n📞 === QUICK FIXES ===');
    console.log('1. npm install http-proxy-middleware');
    console.log('2. Restart both servers');
    console.log('3. Check firewall/antivirus blocking localhost connections');
    console.log('4. Try different ports if conflicts exist');
}

// Run the tests
runTests().catch(error => {
    console.error('❌ Test execution failed:', error);
    process.exit(1);
});
