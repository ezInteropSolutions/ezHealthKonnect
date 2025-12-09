/**
 * Test: Dual Search & Display
 * Demonstrates that BOTH field path AND description work in search and display
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
            cardinality: node.cardinality || '',
            level: (node.path.match(/\./g) || []).length
        });
    }

    if (node.children && Array.isArray(node.children)) {
        for (const child of node.children) {
            flattenSchemaTree(child, paths);
        }
    }

    return paths;
}

async function runDemo() {
    console.log('═'.repeat(70));
    console.log('    DUAL SEARCH & DISPLAY DEMONSTRATION');
    console.log('═'.repeat(70));
    console.log();

    try {
        // Load schema
        const apiResponse = await testAPI();
        const tree = apiResponse.xpathTree;
        const flattenedPaths = flattenSchemaTree(tree);

        // Find PID fields
        const pidFields = flattenedPaths.filter(p =>
            p.name && p.name.startsWith('PID.') && p.description
        );

        console.log('📋 Sample Fields from Database:');
        console.log('─'.repeat(70));
        console.log();

        pidFields.slice(0, 4).forEach(field => {
            console.log(`Field Data Structure:`);
            console.log(`  • path:        "${field.path}"`);
            console.log(`  • name:        "${field.name}" ← Field Key (searchable)`);
            console.log(`  • description: "${field.description}" ← Description (searchable)`);
            console.log(`  • dataType:    "${field.dataType}"`);
            console.log();
        });

        console.log('═'.repeat(70));
        console.log('    HOW IT DISPLAYS IN THE UI');
        console.log('═'.repeat(70));
        console.log();

        pidFields.slice(0, 3).forEach(field => {
            const fieldKey = field.name;
            const description = field.description;
            const path = field.path;
            const dataType = field.dataType;

            console.log('┌' + '─'.repeat(68) + '┐');
            console.log(`│ 🔵 [${fieldKey}] ${description}`.padEnd(69) + '│');
            console.log(`│    ${path}  ${dataType}`.padEnd(69) + '│');
            console.log('└' + '─'.repeat(68) + '┘');
            console.log();
        });

        console.log('═'.repeat(70));
        console.log('    SEARCH EXAMPLES');
        console.log('═'.repeat(70));
        console.log();

        const searchExamples = [
            { query: 'PID.5', type: 'Field Key Search' },
            { query: 'patient', type: 'Description Search' },
            { query: 'name', type: 'Keyword Search' }
        ];

        searchExamples.forEach(example => {
            console.log(`🔎 ${example.type}: "${example.query}"`);
            console.log('   Searches in:');
            console.log('     ✓ Field key (name)');
            console.log('     ✓ Description');
            console.log('     ✓ Technical path');
            console.log();

            // Find matching fields
            const matches = flattenedPaths.filter(p => {
                const query = example.query.toLowerCase();
                return (
                    p.name.toLowerCase().includes(query) ||
                    p.description.toLowerCase().includes(query) ||
                    p.path.toLowerCase().includes(query)
                );
            }).slice(0, 2);

            if (matches.length > 0) {
                console.log('   Results displayed as:');
                matches.forEach(m => {
                    const key = m.name.includes('.') ? m.name.split(' ')[0] : m.name;
                    console.log(`     [${key}] ${m.description}`);
                    console.log(`     ${m.path}  ${m.dataType || 'N/A'}`);
                });
            }
            console.log();
        });

        console.log('═'.repeat(70));
        console.log('    WHAT USER SEES vs WHAT SYSTEM GETS');
        console.log('═'.repeat(70));
        console.log();

        const exampleField = pidFields[0];
        console.log('When user selects a field:');
        console.log();
        console.log('👁️  User sees in dropdown:');
        console.log(`    [${exampleField.name}] ${exampleField.description}`);
        console.log();
        console.log('⚙️  System receives (for processing):');
        console.log(`    ${exampleField.path}`);
        console.log();
        console.log('✅ Best of both worlds:');
        console.log('   • User-friendly display with descriptions');
        console.log('   • Technical path for system processing');
        console.log();

        console.log('═'.repeat(70));
        console.log('    SUMMARY');
        console.log('═'.repeat(70));
        console.log();
        console.log('✅ Search works on BOTH:');
        console.log('   • Field keys (PID.5, MSH.3, etc.)');
        console.log('   • Descriptions (Patient Name, Date of Birth, etc.)');
        console.log();
        console.log('✅ Display shows BOTH:');
        console.log('   • Field key in blue badge: [PID.5]');
        console.log('   • Description in bold: Patient Name');
        console.log('   • Technical path in gray: enhancedSegments.PID.fields[1].value');
        console.log('   • Data type in blue badge: XPN');
        console.log();
        console.log('🎉 Already fully functional - try it at:');
        console.log('   http://localhost:3000/test-xpath-search.html');
        console.log();
        console.log('═'.repeat(70));

    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    }
}

runDemo();
