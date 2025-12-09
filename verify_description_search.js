/**
 * Verification Script: Description-Based Search
 * Tests the complete flow from database → API → search scoring
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

// Simulate the search algorithm from XPathAutocomplete
function searchPaths(flattenedPaths, query) {
    const lowerQuery = query.toLowerCase();

    const scored = flattenedPaths.map(item => {
        const pathLower = item.path.toLowerCase();
        const nameLower = item.name.toLowerCase();
        const descLower = (item.description || '').toLowerCase();

        let score = 0;

        // Exact match (highest priority)
        if (pathLower === lowerQuery) score += 100;
        if (nameLower === lowerQuery) score += 95;
        if (descLower === lowerQuery) score += 90;

        // Starts with (high priority for natural typing)
        if (pathLower.startsWith(lowerQuery)) score += 50;
        if (nameLower.startsWith(lowerQuery)) score += 45;
        if (descLower.startsWith(lowerQuery)) score += 40;

        // Contains (boost description matching significantly)
        if (pathLower.includes(lowerQuery)) score += 20;
        if (nameLower.includes(lowerQuery)) score += 25;
        if (descLower.includes(lowerQuery)) score += 30;

        // Bonus for shorter paths (more specific)
        score += Math.max(0, 10 - item.level);

        return { item, score };
    })
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score)
    .slice(0, 10);

    return scored;
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

async function runTests() {
    console.log('🧪 Description-Based Search Verification\n');
    console.log('━'.repeat(60));

    try {
        // Step 1: Load schema from API
        console.log('\n📡 Step 1: Loading universal HL7 fields from API...');
        const apiResponse = await testAPI();

        if (!apiResponse.success) {
            throw new Error(`API error: ${apiResponse.error}`);
        }

        const tree = apiResponse.xpathTree;
        console.log(`✅ Loaded tree with ${tree.children.length} segments`);

        // Step 2: Flatten the tree
        console.log('\n🔄 Step 2: Flattening schema tree...');
        const flattenedPaths = flattenSchemaTree(tree);
        console.log(`✅ Flattened to ${flattenedPaths.length} paths`);

        // Step 3: Sample the PID fields
        console.log('\n📋 Step 3: Sample PID field paths:');
        const pidPaths = flattenedPaths.filter(p => p.path.includes('.PID.'));
        pidPaths.slice(0, 5).forEach(p => {
            console.log(`   • ${p.name} - "${p.description}" (${p.dataType || 'N/A'})`);
        });

        // Step 4: Test description-based searches
        console.log('\n🔍 Step 4: Testing description-based searches:');
        console.log('━'.repeat(60));

        const testQueries = [
            'patient',
            'name',
            'birth',
            'sex',
            'PID.5'
        ];

        for (const query of testQueries) {
            console.log(`\n🔎 Query: "${query}"`);
            const results = searchPaths(flattenedPaths, query);

            if (results.length > 0) {
                console.log(`   ✅ Found ${results.length} results (showing top 3):`);
                results.slice(0, 3).forEach(({ item, score }) => {
                    console.log(`      [${score}] ${item.name} - "${item.description}"`);
                    console.log(`          Path: ${item.path}`);
                });
            } else {
                console.log(`   ⚠️  No results found`);
            }
        }

        // Step 5: Verify description scoring
        console.log('\n\n✨ Step 5: Verification Summary:');
        console.log('━'.repeat(60));

        const verifications = [
            { check: 'API endpoint responding', status: apiResponse.success },
            { check: 'Tree has segments', status: tree.children.length > 0 },
            { check: 'Paths flattened', status: flattenedPaths.length > 0 },
            { check: 'PID fields have descriptions', status: pidPaths.some(p => p.description) },
            { check: 'Search "patient" finds results', status: searchPaths(flattenedPaths, 'patient').length > 0 },
            { check: 'Search "name" finds results', status: searchPaths(flattenedPaths, 'name').length > 0 }
        ];

        verifications.forEach(v => {
            console.log(`${v.status ? '✅' : '❌'} ${v.check}`);
        });

        const allPassed = verifications.every(v => v.status);

        console.log('\n' + '━'.repeat(60));
        if (allPassed) {
            console.log('🎉 All verification checks passed!');
            console.log('\n📝 Next Steps:');
            console.log('   1. Open http://localhost:3000/test-xpath-search.html');
            console.log('   2. Try searching for "patient", "name", "birth"');
            console.log('   3. Verify field keys (PID.5) show in blue badges');
            console.log('   4. Verify descriptions are highlighted on match');
        } else {
            console.log('⚠️  Some checks failed - review output above');
        }
        console.log('━'.repeat(60) + '\n');

    } catch (error) {
        console.error('❌ Error:', error.message);
        process.exit(1);
    }
}

runTests();
