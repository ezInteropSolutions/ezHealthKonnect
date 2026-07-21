/**
 * EnrichmentStepsDocs — Documentation for pre.enrichment.* steps
 *
 * pre.enrichment.api, pre.enrichment.database, pre.enrichment.script, pre.enrichment, plus the
 * legacy layer-prefix-removed aliases (enrichment.api/database/script).
 *
 * Self-registers into StepDocumentationRegistry at load time — this file must be
 * loaded (via <script>) after StepDocumentationRegistry.js and before any step's
 * Documentation tab is opened. Mirrors the StepBuilderRegistry.register() pattern
 * already used by every step's Configuration-tab builder
 * (public/js/pipeline/components/StepBuilderRegistry.js).
 */

(function () {
    const docs = {
            'pre.enrichment.api': {
                description: 'Enriches HL7 messages by querying external REST APIs (EMPI, EHR, LIMS, insurance systems). Supports all authentication methods including OAuth 2.0 with automatic token management. 100% Postman feature parity.',
                useCases: [
                    'Epic FHIR EMPI lookup - Get complete patient demographics using MRN',
                    'Cerner EMPI - Retrieve patient master index data with OAuth 2.0',
                    'LIMS integration - Fetch pending lab orders for patient',
                    'Insurance verification - Check coverage and eligibility in real-time',
                    'Provider directory - Lookup NPI, specialty, DEA number from external API'
                ],
                example: {
                    endpoint: 'https://epic-fhir.hospital.org/api/FHIR/R4/Patient/{patientId}',
                    method: 'GET',
                    authType: 'oauth2',
                    oauth2Config: {
                        grantType: 'client_credentials',
                        tokenURL: 'https://epic-fhir.hospital.org/oauth2/token',
                        clientID: 'integration-engine',
                        clientSecret: '***',
                        scope: 'patient/*.read'
                    },
                    headers: {
                        'Accept': 'application/fhir+json',
                        'Epic-Client-ID': 'integration-engine'
                    },
                    queryParams: {
                        '_format': 'json',
                        '_pretty': 'true'
                    },
                    fieldMappings: {
                        patientId: 'enhancedSegments.PID.fields[2].value'
                    },
                    targetPath: 'enriched.empi',
                    timeoutMs: 5000,
                    retryCount: 2
                },
                parameters: [
                    { name: 'endpoint', type: 'string', required: true, description: 'API endpoint URL. Use {placeholder} for dynamic values from HL7 fields. Example: https://api.empi.org/patients/{patientId}' },
                    { name: 'method', type: 'enum (GET|POST|PUT|PATCH)', required: true, description: 'HTTP method for the API request' },
                    { name: 'authType', type: 'enum (none|basic|bearer|apikey|oauth2)', required: false, description: 'Authentication method: none (no auth), basic (username/password), bearer (token), apikey (API key in header), oauth2 (OAuth 2.0 with automatic token management)' },
                    { name: 'oauth2Config', type: 'object', required: false, description: 'OAuth 2.0 configuration - ONLY when authType=oauth2. Includes: grantType (client_credentials|password|refresh_token), tokenURL, clientID, clientSecret, scope. Automatic token caching and refresh.' },
                    { name: 'headers', type: 'object', required: false, description: 'HTTP headers as key-value pairs. Use HeaderBuilder UI for visual configuration. Example: {"Accept": "application/json", "Epic-Client-ID": "integration-engine"}' },
                    { name: 'queryParams', type: 'object', required: false, description: 'Query parameters as key-value pairs. Use QueryParamBuilder UI for visual configuration with live URL preview. Example: {"_format": "json", "_count": "10"}' },
                    { name: 'fieldMappings', type: 'object (JSON)', required: false, description: 'Maps placeholder names in URL to HL7 field paths. Example: {"patientId": "enhancedSegments.PID.fields[2].value"} replaces {patientId} in URL with PID-3 value' },
                    { name: 'targetPath', type: 'string', required: false, description: 'Where to store API response in message data using dot notation. Default: "enriched.api". Example: "enriched.empi" stores response at message.enriched.empi' },
                    { name: 'timeoutMs', type: 'number (100-30000)', required: false, description: 'API request timeout in milliseconds. Default: 5000 (5 seconds). Prevents hanging on slow APIs' },
                    { name: 'retryCount', type: 'number (0-5)', required: false, description: 'Number of retry attempts on failure before applying step-level Error Strategy. Default: 0. Uses exponential backoff for network resilience' },
                    { name: 'Error Handling', type: 'Step-level setting', required: false, description: 'Use "On Error Strategy" in Execution Settings to control pipeline behavior on API failure: "Fail" stops pipeline, "Skip" continues without data, "Use Default Value" continues with defaultValue (if configured in backend executor)' }
                ]
            },
            'pre.enrichment.database': {
                description: 'Enriches HL7 messages by querying databases (PostgreSQL, MySQL, SQL Server, MongoDB, Redis, Oracle). NO-CODE: Visual builders for query parameters and result mapping. Test queries before saving with real database results. Works even when query returns 0 rows - column autocomplete always available!',
                useCases: [
                    'EMPI lookup - Query patient master index database using MRN or SSN',
                    'Provider credentials - Fetch NPI, DEA, specialty from provider database',
                    'Patient demographics - Retrieve complete patient profile from EHR database',
                    'Lab reference ranges - Get normal ranges from LIMS database based on test code',
                    'Insurance verification - Check coverage in billing database using policy number',
                    'Facility master data - Lookup facility addresses, phone numbers, identifiers'
                ],
                databaseConfigs: {
                    mysql: {
                        name: '🐬 MySQL',
                        connectionFormat: 'username:password@tcp(host:port)/database',
                        example: 'app_user:mypassword@tcp(mysql.example.com:3306)/healthcare_db',
                        queryFormat: 'Use ? for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = ? AND dob = ?',
                        features: [
                            'Default port: 3306',
                            'Multi-statement queries supported',
                            'JSON functions (JSON_EXTRACT, JSON_VALUE)',
                            'Date formatting (DATE_FORMAT)',
                            'Use PID.3.1 (not PID.3) to match simple varchar columns'
                        ]
                    },
                    postgresql: {
                        name: '🐘 PostgreSQL',
                        connectionFormat: 'host=hostname port=port user=username password=password dbname=database sslmode=disable',
                        example: 'host=postgres.example.com port=5432 user=app_user password=mypassword dbname=healthcare_db sslmode=disable',
                        queryFormat: 'Use $1, $2, $3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = $1 AND dob = $2',
                        features: [
                            'Default port: 5432',
                            'Advanced JSON/JSONB support',
                            'Array operations',
                            'Window functions',
                            'SSL modes: disable, require, verify-ca, verify-full'
                        ]
                    },
                    sqlserver: {
                        name: '🏢 SQL Server',
                        connectionFormat: 'sqlserver://username:password@host:port?database=dbname',
                        example: 'sqlserver://app_user:mypassword@sqlserver.example.com:1433?database=HealthcareDB',
                        queryFormat: 'Use @p1, @p2, @p3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = @p1 AND DateOfBirth = @p2',
                        features: [
                            'Default port: 1433',
                            'Windows Authentication supported',
                            'TOP clause for limiting rows',
                            'JSON support (JSON_VALUE, FOR JSON PATH)',
                            'Common Table Expressions (CTEs)'
                        ]
                    },
                    mongodb: {
                        name: '🍃 MongoDB',
                        connectionFormat: 'mongodb://username:password@host:port/database?authSource=admin',
                        example: 'mongodb://app_user:mypassword@mongodb.example.com:27017/healthcare_db?authSource=admin',
                        queryFormat: 'Visual query builders for filter and projection (NO raw JSON required!)',
                        queryExample: '{ "mrn": "{{ PID.3.1 }}", "status": "active" }',
                        features: [
                            'Default port: 27017',
                            'NoSQL document database',
                            'Filter Builder for match conditions',
                            'Projection Builder for field selection',
                            'Advanced Mode for raw query editing',
                            'Nested document queries',
                            'Array operations ($in, $elemMatch)'
                        ]
                    },
                    redis: {
                        name: '⚡ Redis',
                        connectionFormat: 'redis://[:password@]host:port/database',
                        example: 'redis://:mypassword@redis.example.com:6379/0',
                        queryFormat: 'Redis commands (GET, HGETALL, SMEMBERS, etc.)',
                        queryExample: 'GET patient:{{ PID.3.1 }}',
                        features: [
                            'Default port: 6379',
                            'Key-value operations',
                            'Hash field retrieval (HGETALL)',
                            'Set operations (SMEMBERS)',
                            'Caching and temporary storage',
                            'Fast lookups for frequently accessed data'
                        ]
                    },
                    oracle: {
                        name: '🔴 Oracle',
                        connectionFormat: 'oracle://username:password@host:port/servicename',
                        example: 'oracle://app_user:mypassword@oracle.example.com:1521/ORCL',
                        queryFormat: 'Use :1, :2, :3... for parameter placeholders',
                        queryExample: 'SELECT * FROM patients WHERE mrn = :1 AND date_of_birth = :2',
                        features: [
                            'Default port: 1521',
                            'TNS Names format supported',
                            'ROWNUM for limiting rows',
                            'Hierarchical queries (CONNECT BY)',
                            'Advanced date functions (TO_CHAR)',
                            'Enterprise-grade features'
                        ]
                    }
                },
                example: {
                    databaseType: 'PostgreSQL',
                    connectionString: 'postgresql://user:pass@hostname:5432/database',
                    query: 'SELECT id, email, role, created_at FROM users WHERE email = $1',
                    queryParams: { '1': 'enhancedSegments.PID.fields[13].value' },
                    resultMapping: {
                        'id': 'userId',
                        'email': 'userEmail',
                        'role': 'userRole',
                        'created_at': 'userCreatedAt'
                    },
                    targetPath: 'enriched.user',
                    timeoutMs: 3000,
                    failOnError: false
                },
                parameters: [
                    {
                        name: 'databaseType',
                        type: 'enum (PostgreSQL|MySQL|SQL Server|MongoDB|Oracle)',
                        required: true,
                        description: 'Database type. Determines SQL driver and connection protocol.'
                    },
                    {
                        name: 'connectionString',
                        type: 'string',
                        required: true,
                        description: 'Database connection string. Format varies by database type. Examples: PostgreSQL: "postgresql://user:pass@host:5432/db", MySQL: "mysql://user:pass@host:3306/db", SQL Server: "sqlserver://user:pass@host:1433?database=db"'
                    },
                    {
                        name: 'query',
                        type: 'string (SQL)',
                        required: true,
                        description: 'SQL query to execute. Use $1, $2, $3... for parameter placeholders (PostgreSQL syntax). Use ? for MySQL/SQL Server. Test your query with the Query Tester before saving!'
                    },
                    {
                        name: 'queryParams (Visual Builder)',
                        type: 'object (Auto-built)',
                        required: false,
                        description: 'NO-CODE: Visual table maps SQL parameters to HL7 field paths. NO JSON EDITING REQUIRED! Example: $1 → enhancedSegments.PID.fields[13].value maps first parameter to PID-13 (Phone Number). Click [+ Add Parameter] to add rows.'
                    },
                    {
                        name: 'resultMapping (Visual Builder)',
                        type: 'object (Auto-built)',
                        required: false,
                        description: 'NO-CODE: Visual table maps database columns to output field names. NO JSON EDITING REQUIRED! Test your query first, then click [+ Add to Mapping] on result fields to auto-populate this builder. Auto converts snake_case to camelCase (created_at → createdAt).'
                    },
                    {
                        name: 'targetPath',
                        type: 'string',
                        required: false,
                        description: 'Where to store database results in message data using dot notation. Default: "enriched.database". Example: "enriched.empi" stores results at message.enriched.empi. Use different paths for multiple database enrichment steps.'
                    },
                    {
                        name: 'timeoutMs',
                        type: 'number (100-30000)',
                        required: false,
                        description: 'Database query timeout in milliseconds. Default: 3000 (3 seconds). Prevents hanging on slow queries or network issues.'
                    },
                    {
                        name: 'failOnError',
                        type: 'boolean',
                        required: false,
                        description: 'Whether to stop pipeline if database query fails. Default: false (continue without enrichment data). Set to true for critical enrichment that MUST succeed.'
                    }
                ],
                noCodeFeatures: [
                    {
                        feature: 'Query Parameter Builder',
                        description: 'Visual table for mapping SQL parameters ($1, $2, etc.) to HL7 field paths. NO JSON EDITING!',
                        howTo: 'Click [+ Add Parameter] → Select HL7 field from dropdown or type field path → Parameter automatically numbered',
                        benefit: 'Eliminates JSON syntax errors, provides field autocomplete, shows parameter order visually'
                    },
                    {
                        feature: 'Result Mapping Builder',
                        description: 'Visual table for mapping database columns to output field names. AUTO camelCase conversion!',
                        howTo: 'Click [+ Add Mapping] → Enter DB column name → Enter output field name (or use suggestion) → Auto-converts snake_case to camelCase',
                        benefit: 'No JSON editing, auto naming conventions, visual feedback on mappings'
                    },
                    {
                        feature: 'Database Query Tester',
                        description: 'Test SQL queries BEFORE saving pipeline! See real database results with click-to-add mapping.',
                        howTo: '1) Configure connection & query 2) Enter test parameter values 3) Click [▶ Run Query] 4) See actual results 5) Click [+ Add to Mapping] on any field',
                        benefit: 'Instant feedback, verify queries work, one-click configuration from real data, no trial-and-error'
                    },
                    {
                        feature: 'Click-to-Add Mapping',
                        description: 'Click [+ Add to Mapping] on query results to auto-populate Result Mapping Builder',
                        howTo: 'Run query → Click [+ Add to Mapping] next to any field → Automatically adds row to Result Mapping Builder with smart field name',
                        benefit: '80% time savings, zero typos, smart camelCase conversion (created_at → createdAt)'
                    }
                ],
                workflow: [
                    { step: 1, action: 'Select database type', description: 'Choose PostgreSQL, MySQL, SQL Server, MongoDB, or Oracle' },
                    { step: 2, action: 'Enter connection string', description: 'Database connection URL with credentials (secure storage recommended)' },
                    { step: 3, action: 'Write SQL query', description: 'Use $1, $2... for parameters. Example: SELECT * FROM patients WHERE mrn = $1' },
                    { step: 4, action: 'Map query parameters (Visual Builder)', description: 'Click [+ Add Parameter] → Select HL7 field path for each $1, $2, etc.' },
                    { step: 5, action: 'Test your query! 🧪', description: 'Scroll to Query Tester → Enter test values → Click [▶ Run Query] → See real results!' },
                    { step: 6, action: 'Click-to-add result mappings', description: 'Click [+ Add to Mapping] on fields you want to include → Auto-populates Result Mapping Builder' },
                    { step: 7, action: 'Configure target path', description: 'Set where to store results (e.g., enriched.empi, enriched.provider)' },
                    { step: 8, action: 'Save step', description: 'Configuration saved with visual builders + tested query' }
                ],
                bestPractices: [
                    {
                        practice: 'Always test queries first',
                        reason: 'Query Tester shows real results and prevents SQL errors in production',
                        example: 'Run test query with sample MRN before deploying pipeline'
                    },
                    {
                        practice: 'Use named connection strings (future)',
                        reason: 'Centralized connection management, credential rotation, environment-specific configs',
                        example: 'Instead of hardcoding connection string, reference named connection "EMPI_PROD"'
                    },
                    {
                        practice: 'Limit result rows in query',
                        reason: 'Prevents performance issues from large result sets',
                        example: 'Add LIMIT 1 to queries that should return single row (patient lookup, provider lookup)'
                    },
                    {
                        practice: 'Use specific column names',
                        reason: 'SELECT * causes issues when schema changes. Specify exact columns needed.',
                        example: 'Use SELECT id, name, mrn instead of SELECT *'
                    },
                    {
                        practice: 'Set appropriate timeout',
                        reason: 'Balance between allowing slow queries and preventing pipeline hangs',
                        example: 'Simple lookups: 1000ms, complex joins: 5000ms, reporting queries: 10000ms'
                    },
                    {
                        practice: 'Use failOnError wisely',
                        reason: 'Critical enrichment should fail pipeline; optional enrichment should continue',
                        example: 'EMPI lookup (critical): failOnError=true, Provider specialty (nice-to-have): failOnError=false'
                    }
                ],
                troubleshooting: [
                    {
                        issue: 'Connection failed: dial tcp: lookup failed',
                        cause: 'Hostname not resolvable or database server not reachable',
                        fix: 'Check connection string hostname. For Docker: use service name (e.g., postgres) not localhost'
                    },
                    {
                        issue: 'Query execution failed: syntax error',
                        cause: 'Invalid SQL syntax for selected database type',
                        fix: 'Test query in Query Tester. Check parameter syntax ($1 vs ? based on database type)'
                    },
                    {
                        issue: 'Query returned no rows',
                        cause: 'Query executed but no matching records found',
                        fix: 'Verify test parameter values match actual data. Check WHERE clause conditions.'
                    },
                    {
                        issue: 'Timeout exceeded',
                        cause: 'Query took longer than timeoutMs setting',
                        fix: 'Increase timeout, optimize query with indexes, or limit result set size'
                    },
                    {
                        issue: 'Parameter mismatch: expected 2 got 1',
                        cause: 'Query has more/fewer parameters than mapped in Query Parameter Builder',
                        fix: 'Count $1, $2, $3... in query. Add corresponding rows in Query Parameter Builder.'
                    }
                ],
                securityNotes: [
                    {
                        note: 'Secure credential storage',
                        detail: 'Connection strings contain passwords. Use environment variables or secret management systems (HashiCorp Vault, AWS Secrets Manager) instead of hardcoding.',
                        recommendation: 'Future feature: Named connections with credential vault integration'
                    },
                    {
                        note: 'SQL injection prevention',
                        detail: 'Always use parameterized queries ($1, $2...) - NEVER concatenate user input into SQL strings',
                        recommendation: 'Query Tester validates parameter usage. Backend uses prepared statements for safety.'
                    },
                    {
                        note: 'Least privilege database access',
                        detail: 'Database user should have SELECT-only permissions for read operations',
                        recommendation: 'Create dedicated integration user with minimal required permissions'
                    },
                    {
                        note: 'Connection pooling',
                        detail: 'Test queries use isolated connections with max 1 connection to prevent resource exhaustion',
                        recommendation: 'Production executor uses connection pooling for performance'
                    }
                ]
            },
            'pre.enrichment.script': {
                description: 'Execute custom JavaScript logic to calculate, transform, or enrich data from HL7 messages and previous enrichment steps. Use this for complex business rules, risk scoring, data validation, custom calculations, and conditional logic that cannot be achieved through simple mappings. Results are stored in enriched_data and can be referenced by subsequent steps.',
                useCases: [
                    'Calculate patient risk scores based on age, diagnoses, and lab values',
                    'Determine insurance eligibility by combining patient demographics with external data',
                    'Perform complex date calculations (e.g., days since last admission, appointment intervals)',
                    'Apply business rules for message routing or prioritization',
                    'Validate and transform data formats (e.g., normalize phone numbers, format addresses)',
                    'Enrich data by combining HL7 fields with database/API results from previous steps',
                    'Generate derived fields (e.g., BMI from height/weight, age from date of birth)',
                    'Implement conditional logic for data transformation (if-then-else scenarios)'
                ],
                example: {
                    script: `// Access HL7 message fields
var patientName = getHL7Field(input, "PID.5");
var dateOfBirth = getHL7Field(input, "PID.7");
var smokingStatus = getHL7Field(input, "PV1.17");

// Access enriched data from previous database enrichment step
var chronicConditions = getNestedValue(input, '["database_enrichment"].enriched_data.chronicConditions');
var lastAdmission = getNestedValue(input, '["database_enrichment"].enriched_data.lastAdmission');

// Access data from previous API enrichment step
var insuranceStatus = getNestedValue(input, '["API_Enrichment"].enriched_data.insuranceActive');

// Perform calculations
var patientAge = calculateAge(dateOfBirth);
var daysSinceLastAdmission = calculateDaysSince(lastAdmission);

// Calculate risk score based on multiple factors
var riskScore = 0;
if (patientAge > 65) riskScore += 3;
if (chronicConditions > 2) riskScore += 4;
if (smokingStatus === "current") riskScore += 2;
if (daysSinceLastAdmission < 30) riskScore += 3;

// Determine risk level
var riskLevel = "low";
if (riskScore >= 8) riskLevel = "high";
else if (riskScore >= 5) riskLevel = "moderate";

// Build risk factors array
var riskFactors = [];
if (chronicConditions > 2) riskFactors.push(chronicConditions + " chronic conditions");
if (smokingStatus === "current") riskFactors.push("Current smoker");
if (daysSinceLastAdmission < 30) riskFactors.push("Recent admission (< 30 days)");

// Return enriched data (stored in enriched_data for use in subsequent steps)
return {
    patientAge: patientAge,
    riskScore: riskScore,
    riskLevel: riskLevel,
    riskFactors: riskFactors,
    chronicConditions: chronicConditions,
    smokingStatus: smokingStatus,
    daysSinceLastAdmission: daysSinceLastAdmission,
    calculatedAt: new Date().toISOString()
};`,
                    timeout_ms: 5000,
                    failOnError: false
                },
                parameters: [
                    {
                        name: 'script',
                        type: 'string (JavaScript code)',
                        required: true,
                        description: 'JavaScript code to execute. The script receives the input object containing HL7 message and enriched data from previous steps. Must return an object with calculated/enriched fields.'
                    },
                    {
                        name: 'timeout_ms',
                        type: 'number',
                        required: false,
                        description: 'Maximum execution time in milliseconds (default: 5000). Script will be terminated if it exceeds this limit.'
                    },
                    {
                        name: 'failOnError',
                        type: 'boolean',
                        required: false,
                        description: 'Whether to fail the entire pipeline if script execution fails (default: false). Set to true for critical calculations.'
                    }
                ],
                referenceVariables: {
                    title: 'Accessing Data in Script Enrichment',
                    description: 'Your script can access HL7 message fields and enriched data from previous steps using helper functions. All returned fields are automatically available to subsequent steps.',
                    examples: [
                        {
                            scenario: 'Access HL7 Message Fields',
                            code: 'var patientId = getHL7Field(input, "PID.3");',
                            explanation: 'Use getHL7Field(input, "segment.field") to extract values from the HL7 message'
                        },
                        {
                            scenario: 'Access Database Enrichment Results',
                            code: 'var chronicConditions = getNestedValue(input, \'["database_enrichment"].enriched_data.chronicConditions\');',
                            explanation: 'Use getNestedValue(input, xpath) to access data from previous database enrichment steps'
                        },
                        {
                            scenario: 'Access API Enrichment Results',
                            code: 'var externalId = getNestedValue(input, \'["API_Enrichment"].enriched_data.externalPatientId\');',
                            explanation: 'Access data returned from API calls in previous enrichment steps'
                        },
                        {
                            scenario: 'Access Metadata Fields',
                            code: 'var customValue = getNestedValue(input, "metadata.customField");',
                            explanation: 'Access metadata fields added by the metadata enrichment step'
                        },
                        {
                            scenario: 'Return Calculated Values',
                            code: 'return { riskScore: 9, riskLevel: "moderate", calculatedAt: new Date().toISOString() };',
                            explanation: 'Return an object with calculated fields. These will be stored in ["Script_Enrichment"].enriched_data and available to subsequent steps including field mapping.'
                        }
                    ]
                },
                availableFunctions: {
                    title: 'Available Helper Functions',
                    description: 'The following functions are available in your script execution environment:',
                    functions: [
                        {
                            name: 'getHL7Field(input, path)',
                            description: 'Extract a field value from the HL7 message',
                            parameters: 'input: message object, path: HL7 field path (e.g., "PID.5", "PV1.3")',
                            returns: 'string',
                            example: 'var name = getHL7Field(input, "PID.5");'
                        },
                        {
                            name: 'getNestedValue(input, xpath)',
                            description: 'Access nested data from enriched_data fields using XPath notation',
                            parameters: 'input: message object, xpath: XPath to enriched field (e.g., \'["database_enrichment"].enriched_data.chronicConditions\')',
                            returns: 'any',
                            example: 'var conditions = getNestedValue(input, \'["database_enrichment"].enriched_data.chronicConditions\');'
                        },
                        {
                            name: 'calculateAge(dateOfBirth)',
                            description: 'Calculate age in years from date of birth',
                            parameters: 'dateOfBirth: date string in HL7 format (YYYYMMDD)',
                            returns: 'number',
                            example: 'var age = calculateAge("19800515");'
                        },
                        {
                            name: 'calculateDaysSince(dateString)',
                            description: 'Calculate number of days between a date and now',
                            parameters: 'dateString: date string in HL7 format (YYYYMMDD)',
                            returns: 'number',
                            example: 'var days = calculateDaysSince("20240101");'
                        },
                        {
                            name: 'formatDate(hl7Date, format)',
                            description: 'Convert HL7 date format to custom format',
                            parameters: 'hl7Date: HL7 date string, format: target format (e.g., "YYYY-MM-DD")',
                            returns: 'string',
                            example: 'var isoDate = formatDate("20240115", "YYYY-MM-DD");'
                        }
                    ]
                },
                bestPractices: [
                    {
                        practice: 'Error Handling',
                        recommendation: 'Always validate input data before performing calculations to avoid runtime errors',
                        example: 'if (chronicConditions && chronicConditions > 0) { /* safe to use */ }'
                    },
                    {
                        practice: 'Performance',
                        recommendation: 'Keep scripts lightweight and avoid complex loops. Use database/API enrichment for heavy data operations.',
                        example: 'Perform database queries in database enrichment step, use script enrichment only for calculations on retrieved data'
                    },
                    {
                        practice: 'Naming',
                        recommendation: 'Use clear, descriptive field names in your return object for easy reference in subsequent steps',
                        example: 'return { patientRiskScore: score } instead of return { r: score }'
                    },
                    {
                        practice: 'Testing',
                        recommendation: 'Use the Test Execution feature to verify script logic with sample HL7 messages before deployment',
                        example: 'Click "Test Execution" button and review the Script Enrichment output in step_outputs'
                    },
                    {
                        practice: 'Data Access',
                        recommendation: 'Click the "Variables" tab to see all available data from previous steps with copy-paste ready XPaths',
                        example: 'Copy XPath from Variables tab and use with getNestedValue() function'
                    }
                ]
            },
            'pre.enrichment': {
                description: 'Enriches HL7 messages with additional data from external systems (EMPI, EHR, etc.). Enhances message content before FHIR transformation.',
                useCases: [
                    'Add complete patient demographics from EMPI using partial identifier',
                    'Fetch latest lab results from LIMS',
                    'Retrieve insurance information from billing system',
                    'Augment provider data with NPI and specialty information',
                    'Add facility details (address, contact info) from master data management'
                ],
                example: {
                    sources: ['EMPI', 'EHR'],
                    timeout_ms: 3000,
                    failOnError: false,
                    caching: {
                        enabled: true,
                        ttl_seconds: 300
                    }
                },
                parameters: [
                    { name: 'sources', type: 'Array<string>', required: true, description: 'List of data sources to query (EMPI, EHR, LIMS, etc.)' },
                    { name: 'timeout_ms', type: 'number', required: false, description: 'Maximum time to wait for enrichment (default: 3000)' },
                    { name: 'failOnError', type: 'boolean', required: false, description: 'Whether to fail pipeline if enrichment fails (default: false)' }
                ]
            },
    };
    Object.keys(docs).forEach((stepType) => StepDocumentationRegistry.register(stepType, docs[stepType]));
})();

StepDocumentationRegistry.registerAlias('enrichment.script', 'pre.enrichment.script');
StepDocumentationRegistry.registerAlias('enrichment.api', 'pre.enrichment.api');
StepDocumentationRegistry.registerAlias('enrichment.database', 'pre.enrichment.database');
