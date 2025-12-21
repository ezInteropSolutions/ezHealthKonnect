/**
 * Test: Field Paths End with .value
 * Verifies that field paths return the actual value, not the object
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

function flattenSchemaTree(node, paths = []) {
    if (!node) return paths;

    if (node.path) {
        paths.push({
            path: node.path,
            name: node.name || node.path,
            description: node.description || '',
            dataType: node.dataType || '',
            type: node.type || ''
        });
    }

    if (node.children && Array.isArray(node.children)) {
        for (const child of node.children) {
            flattenSchemaTree(child, paths);
        }
    }

    return paths;
}

async function runTest() {
    console.log('═'.repeat(70));
    console.log('  FIELD PATH VERIFICATION TEST');
    console.log('═'.repeat(70));
    console.log();

    try {
        const apiResponse = await testAPI();
        const tree = apiResponse.xpathTree;
        const flattenedPaths = flattenSchemaTree(tree);

        // Find fields that should have .value paths
        const pidFields = flattenedPaths.filter(p =>
            p.name && p.name.match(/^PID\.\d+$/) && p.type === 'field-value'
        );

        console.log('✅ Field Value Paths (Primary - what users select):');
        console.log('─'.repeat(70));
        pidFields.forEach(field => {
            const endsWithValue = field.path.endsWith('.value');
            const icon = endsWithValue ? '✅' : '❌';
            console.log(`${icon} ${field.name} - "${field.description}"`);
            console.log(`   Path: ${field.path}`);
            console.log(`   Type: ${field.type}`);
            console.log();
        });

        // Find object paths (advanced users)
        const objectPaths = flattenedPaths.filter(p =>
            p.type === 'field-object'
        );

        if (objectPaths.length > 0) {
            console.log('📦 Field Object Paths (Advanced - full field object):');
            console.log('─'.repeat(70));
            objectPaths.slice(0, 3).forEach(field => {
                console.log(`   ${field.name}`);
                console.log(`   Path: ${field.path}`);
                console.log(`   Description: ${field.description}`);
                console.log();
            });
        }

        // Verification
        console.log('═'.repeat(70));
        console.log('  VERIFICATION RESULTS');
        console.log('═'.repeat(70));
        console.log();

        const allValuePathsCorrect = pidFields.every(f => f.path.endsWith('.value'));
        const hasDescriptions = pidFields.every(f => f.description);

        console.log(`✅ All primary field paths end with .value: ${allValuePathsCorrect ? 'YES' : 'NO'}`);
        console.log(`✅ All fields have descriptions: ${hasDescriptions ? 'YES' : 'NO'}`);
        console.log(`✅ Total field-value paths: ${pidFields.length}`);
        console.log(`✅ Total field-object paths: ${objectPaths.length}`);
        console.log();

        if (allValuePathsCorrect && hasDescriptions) {
            console.log('🎉 All checks passed!');
            console.log();
            console.log('Search for "Date of Birth" will return:');
            const dob = pidFields.find(f => f.description.toLowerCase().includes('birth'));
            if (dob) {
                console.log(`  ✅ ${dob.name} - ${dob.description}`);
                console.log(`  ✅ Path: ${dob.path} (ends with .value)`);
            }
        } else {
            console.log('⚠️  Some checks failed - review above');
        }
        console.log();
        console.log('═'.repeat(70));

    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    }
}

runTest();
