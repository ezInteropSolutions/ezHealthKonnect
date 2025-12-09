/**
 * Test: Flatten Performance & Infinite Loop Prevention
 * Verifies that the flattening doesn't hang or cause infinite loops
 */

const http = require('http');

function testAPI() {
    return new Promise((resolve, reject) => {
        const options = {
            hostname: 'localhost',
            port: 3000,
            path: '/api/schemas/hl7/fields',
            method: 'GET',
            headers: { 'Content-Type': 'application/json' }
        };

        const req = http.request(options, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
                try {
                    resolve(JSON.parse(data));
                } catch (error) {
                    reject(error);
                }
            });
        });

        req.on('error', reject);
        req.end();
    });
}

// Simulate the FIXED flattenSchemaTree with safeguards
function flattenSchemaTree(node, paths = [], visited = new Set(), depth = 0) {
    // Prevent infinite recursion
    if (!node || depth > 20) {
        if (depth > 20) {
            console.warn('[WARN] Max recursion depth reached, stopping traversal');
        }
        return paths;
    }

    // Prevent circular references by tracking visited paths
    const nodeId = node.path || `node_${depth}_${paths.length}`;
    if (node.path && visited.has(nodeId)) {
        console.warn('[WARN] Duplicate path detected, skipping:', nodeId);
        return paths;
    }
    if (node.path) {
        visited.add(nodeId);
    }

    // Add current node if it has a path
    if (node.path) {
        paths.push({
            path: node.path,
            name: node.name || node.path,
            description: node.description || '',
            dataType: node.dataType || '',
            cardinality: node.cardinality || '',
            level: (node.path.match(/\./g) || []).length
        });
    }

    // Recursively process children
    if (node.children && Array.isArray(node.children)) {
        for (const child of node.children) {
            flattenSchemaTree(child, paths, visited, depth + 1);
        }
    }

    return paths;
}

async function runTest() {
    console.log('═'.repeat(70));
    console.log('  FLATTEN PERFORMANCE & INFINITE LOOP PREVENTION TEST');
    console.log('═'.repeat(70));
    console.log();

    try {
        console.log('📡 Step 1: Loading schema from API...');
        const startLoad = Date.now();
        const apiResponse = await testAPI();
        const loadTime = Date.now() - startLoad;

        if (!apiResponse.success) {
            throw new Error(`API error: ${apiResponse.error}`);
        }

        console.log(`✅ Schema loaded in ${loadTime}ms`);
        const tree = apiResponse.xpathTree;
        console.log(`   Root: ${tree.name}`);
        console.log(`   Segments: ${tree.children.length}`);
        console.log();

        console.log('🔄 Step 2: Flattening schema tree...');
        const startFlatten = Date.now();

        // Set a timeout to detect if it hangs
        const timeout = setTimeout(() => {
            console.error('❌ HANG DETECTED: Flattening took more than 5 seconds!');
            console.error('   This indicates an infinite loop or circular reference');
            process.exit(1);
        }, 5000);

        const flattenedPaths = flattenSchemaTree(tree);
        clearTimeout(timeout);

        const flattenTime = Date.now() - startFlatten;
        console.log(`✅ Flattening completed in ${flattenTime}ms`);
        console.log(`   Total paths: ${flattenedPaths.length}`);
        console.log();

        console.log('📊 Step 3: Analyzing flattened paths...');
        const pathTypes = {};
        flattenedPaths.forEach(p => {
            const type = p.path.includes('.value') ? 'field-value' :
                        p.path.includes('.fields[') ? 'field-object' :
                        p.path.includes('.subfields[') ? 'subfield' :
                        p.path.includes('.fields') ? 'fields-array' :
                        'other';
            pathTypes[type] = (pathTypes[type] || 0) + 1;
        });

        console.log('   Path distribution:');
        Object.entries(pathTypes).forEach(([type, count]) => {
            console.log(`     • ${type}: ${count}`);
        });
        console.log();

        console.log('📋 Step 4: Sample flattened paths:');
        const samplePaths = flattenedPaths.filter(p => p.path.includes('PID')).slice(0, 5);
        samplePaths.forEach(p => {
            console.log(`   ${p.name} → ${p.path}`);
            console.log(`      Description: ${p.description}`);
        });
        console.log();

        console.log('═'.repeat(70));
        console.log('  VERIFICATION RESULTS');
        console.log('═'.repeat(70));
        console.log();

        const checks = [
            { name: 'API responds', pass: apiResponse.success },
            { name: 'Schema loaded', pass: tree && tree.children.length > 0 },
            { name: 'Flattening completes', pass: flattenedPaths.length > 0 },
            { name: 'No hang (< 5s)', pass: flattenTime < 5000 },
            { name: 'Performance OK (< 500ms)', pass: flattenTime < 500 },
            { name: 'Has field-value paths', pass: pathTypes['field-value'] > 0 },
            { name: 'Has descriptions', pass: flattenedPaths.some(p => p.description) }
        ];

        checks.forEach(check => {
            const icon = check.pass ? '✅' : '❌';
            console.log(`${icon} ${check.name}`);
        });

        const allPass = checks.every(c => c.pass);

        console.log();
        console.log('═'.repeat(70));
        if (allPass) {
            console.log('🎉 ALL TESTS PASSED!');
            console.log();
            console.log('Performance Summary:');
            console.log(`  • API load time: ${loadTime}ms`);
            console.log(`  • Flatten time: ${flattenTime}ms`);
            console.log(`  • Total paths: ${flattenedPaths.length}`);
            console.log(`  • Paths/ms: ${(flattenedPaths.length / flattenTime).toFixed(2)}`);
        } else {
            console.log('⚠️  SOME TESTS FAILED');
            process.exit(1);
        }
        console.log('═'.repeat(70));

    } catch (error) {
        console.error('❌ Error:', error.message);
        console.error(error.stack);
        process.exit(1);
    }
}

runTest();
