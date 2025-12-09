const http = require('http');

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
            const json = JSON.parse(data);
            if (json.success) {
                console.log('✅ Universal HL7 fields loaded successfully');
                const tree = json.xpathTree;
                console.log('Root:', tree.name);
                console.log('Segments:', tree.children.length);

                // Find PID segment
                const pid = tree.children.find(c => c.name === 'PID');
                if (pid && pid.children) {
                    const fields = pid.children.find(c => c.name === 'fields');
                    if (fields && fields.children) {
                        console.log('\nPID fields:', fields.children.length);
                        console.log('First 3 PID fields:');
                        fields.children.slice(0, 3).forEach(f => {
                            console.log(`  - ${f.name}: ${f.description}`);
                        });
                    }
                }
            } else {
                console.log('❌ Error:', json.error);
            }
        } catch (error) {
            console.error('Parse error:', error.message);
            console.log('Response:', data.substring(0, 200));
        }
    });
});

req.on('error', error => console.error('Request error:', error.message));
req.end();
