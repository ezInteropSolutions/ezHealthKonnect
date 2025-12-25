# Database Enrichment Step - No-Code Improvements

## Current State Analysis

### ❌ Current Issues (Not Truly No-Code)

1. **Raw JSON Configuration** - Users must write JSON manually:
   ```json
   {
     "queryParams": {"patientId": "PID.3"},
     "resultMapping": {"patient_name": "fullName"}
   }
   ```
   - Error-prone (syntax errors, typos)
   - Requires JSON knowledge
   - No validation until execution
   - No field autocomplete or guidance

2. **Connection String in Plain Text** - Security issue:
   ```
   postgresql://user:password@localhost:5432/dbname
   ```
   - Passwords visible in UI
   - No connection testing before saving
   - No connection pooling/reuse across steps

3. **No Query Testing** - Can't verify query works before saving pipeline

4. **No Visual Query Builder** - Users must know SQL

5. **No Documentation/Examples** - Users don't know what fields are available

6. **Poor Field Path Selection** - Manual typing of `PID.3` instead of dropdown

---

## ✅ Proposed Improvements (True No-Code)

### Improvement 1: Visual Query Parameter Builder (Like API Enrichment)

**Current (Raw JSON)**:
```
Query Parameters (JSON)
┌─────────────────────────────────────┐
│ {"patientId": "PID.3"}              │
└─────────────────────────────────────┘
```

**Proposed (Visual Builder)**:
```
Query Parameters
┌───────────────────┬──────────────────┬─────────┐
│ Parameter Name    │ HL7 Field Path   │ Actions │
├───────────────────┼──────────────────┼─────────┤
│ patientId         │ PID.3 ▼          │  [🗑️]   │
│ visitNumber       │ PV1.19 ▼         │  [🗑️]   │
│                   │                  │         │
│ [+ Add Parameter]                             │
└───────────────────────────────────────────────┘
```

**Implementation**: Reuse `QueryParamBuilder` component (already exists for API enrichment!)

---

### Improvement 2: Visual Result Mapping Builder

**Current (Raw JSON)**:
```
Result Mapping (JSON)
┌─────────────────────────────────────────────────┐
│ {"patient_name": "fullName",                    │
│  "dob": "dateOfBirth"}                          │
└─────────────────────────────────────────────────┘
```

**Proposed (Visual Builder)**:
```
Result Mapping (Map database columns to output fields)
┌──────────────────┬──────────────────┬─────────┐
│ DB Column Name   │ Output Field     │ Actions │
├──────────────────┼──────────────────┼─────────┤
│ patient_name     │ fullName         │  [🗑️]   │
│ dob              │ dateOfBirth      │  [🗑️]   │
│ mrn              │ medicalRecordNum │  [🗑️]   │
│                  │                  │         │
│ [+ Add Mapping]                               │
└───────────────────────────────────────────────┘
```

**NEW COMPONENT**: Create `ResultMappingBuilder.js` (similar to QueryParamBuilder)

---

### Improvement 3: Database Connection Manager (Named Connections)

**Problem**: Connection strings with passwords in every step

**Solution**: Centralized connection management

```
Database Connection
┌─────────────────────────────────────────────────┐
│ ○ Use Saved Connection                          │
│   ┌────────────────────────┐                    │
│   │ Epic Production DB ▼   │  [Test] [Manage]   │
│   └────────────────────────┘                    │
│                                                  │
│ ○ Custom Connection String                      │
│   ┌────────────────────────────────────────┐    │
│   │ postgresql://...                       │    │
│   └────────────────────────────────────────┘    │
│   [Test Connection]                              │
└─────────────────────────────────────────────────┘
```

**Backend**: Stored in `database_connections` table (encrypted passwords)

---

### Improvement 4: Database Query Tester (Like API Endpoint Tester!)

**Concept**: Test query BEFORE configuring pipeline

```
🧪 Test Database Query
┌─────────────────────────────────────────────────────────────┐
│ Connection: Epic Production DB                         [▶ Run]│
│                                                               │
│ SQL Query:                                                    │
│ ┌───────────────────────────────────────────────────────┐    │
│ │ SELECT patient_id, patient_name, dob                  │    │
│ │ FROM patients                                         │    │
│ │ WHERE patient_id = $1                                 │    │
│ └───────────────────────────────────────────────────────┘    │
│                                                               │
│ Test Parameters:                                              │
│ ┌──────────────┬─────────────────┐                           │
│ │ $1           │ 12345           │                           │
│ └──────────────┴─────────────────┘                           │
│                                                               │
│ ✅ Query Result (1 row):                                      │
│ ┌───────────────────────────────────────────────────────┐    │
│ │ {                                                     │    │
│ │   "patient_id": "12345",         [➕ Add to Mapping]  │    │
│ │   "patient_name": "John Doe",    [➕ Add to Mapping]  │    │
│ │   "dob": "1980-05-15"            [➕ Add to Mapping]  │    │
│ │ }                                                     │    │
│ └───────────────────────────────────────────────────────┘    │
│                                                               │
│ 💡 Click [➕] to automatically add field to Result Mapping    │
└─────────────────────────────────────────────────────────────┘
```

**NEW COMPONENT**: `DatabaseQueryTester.js` (similar to `APIEndpointTester.js`)

---

### Improvement 5: Documentation Panel (Inline Help)

**Add collapsible documentation section**:

```
📖 Database Enrichment Documentation  [Expand ▼]
┌─────────────────────────────────────────────────────────────┐
│ Purpose: Query databases to enrich message data             │
│                                                              │
│ Example Use Cases:                                           │
│ • Look up patient demographics from EMPI                     │
│ • Get provider details from credentials database            │
│ • Validate insurance coverage from payer database           │
│                                                              │
│ How It Works:                                                │
│ 1. Extract parameters from HL7 message (e.g., PID.3)        │
│ 2. Execute SQL query with parameters                         │
│ 3. Map database columns to output fields                     │
│ 4. Store results in message data (default: enriched.database)│
│                                                              │
│ 💡 Tip: Use the Query Tester to verify your query works     │
│    before saving the pipeline!                               │
└─────────────────────────────────────────────────────────────┘
```

---

### Improvement 6: JSON Config View (Advanced Mode)

**Keep JSON view for power users, but as secondary option**:

```
Configuration Mode:
┌──────────────────────────────────────────┐
│ ○ Visual Builder (Recommended - No-Code) │
│ ● JSON Editor (Advanced Users)           │
└──────────────────────────────────────────┘

JSON Configuration  [Format JSON] [Validate]
┌────────────────────────────────────────────┐
│ {                                          │
│   "databaseType": "postgresql",            │
│   "connectionName": "epic-prod",           │
│   "query": "SELECT * FROM ...",            │
│   "queryParams": {                         │
│     "patientId": "PID.3"                   │
│   },                                       │
│   "resultMapping": {                       │
│     "patient_name": "fullName"             │
│   },                                       │
│   "targetPath": "enriched.patient",        │
│   "timeoutMs": 3000                        │
│ }                                          │
└────────────────────────────────────────────┘

✅ Valid JSON
```

---

## Implementation Plan

### Phase 1: Core Visual Builders (2-3 days)

**1.1 Create ResultMappingBuilder Component (4 hours)**

```javascript
// public/js/pipeline/components/ResultMappingBuilder.js
class ResultMappingBuilder {
    constructor(container, initialMappings = {}) {
        this.container = container;
        this.mappings = initialMappings; // {"db_column": "output_field"}
        this.render();
    }

    render() {
        this.container.innerHTML = `
            <div class="result-mapping-builder">
                <table class="mapping-table">
                    <thead>
                        <tr>
                            <th>Database Column</th>
                            <th>Output Field Name</th>
                            <th>Actions</th>
                        </tr>
                    </thead>
                    <tbody id="mappingRows"></tbody>
                </table>
                <button type="button" class="add-mapping-btn">+ Add Mapping</button>
            </div>
        `;

        this.renderRows();
        this.attachEventListeners();
    }

    renderRows() {
        const tbody = this.container.querySelector('#mappingRows');
        tbody.innerHTML = '';

        Object.entries(this.mappings).forEach(([dbColumn, outputField]) => {
            this.addRow(dbColumn, outputField);
        });
    }

    addRow(dbColumn = '', outputField = '') {
        const tbody = this.container.querySelector('#mappingRows');
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>
                <input type="text"
                       class="db-column-input"
                       value="${dbColumn}"
                       placeholder="patient_name"
                       required>
            </td>
            <td>
                <input type="text"
                       class="output-field-input"
                       value="${outputField}"
                       placeholder="fullName"
                       required>
            </td>
            <td>
                <button type="button" class="delete-row-btn">🗑️</button>
            </td>
        `;

        tbody.appendChild(row);

        // Add delete handler
        row.querySelector('.delete-row-btn').addEventListener('click', () => {
            row.remove();
        });
    }

    attachEventListeners() {
        this.container.querySelector('.add-mapping-btn').addEventListener('click', () => {
            this.addRow();
        });
    }

    getMappings() {
        const mappings = {};
        const rows = this.container.querySelectorAll('#mappingRows tr');

        rows.forEach(row => {
            const dbColumn = row.querySelector('.db-column-input').value.trim();
            const outputField = row.querySelector('.output-field-input').value.trim();

            if (dbColumn && outputField) {
                mappings[dbColumn] = outputField;
            }
        });

        return mappings;
    }

    // Auto-add mapping from query tester
    addMappingFromQueryResult(dbColumn, suggestedOutputField = null) {
        const outputField = suggestedOutputField || dbColumn;
        this.addRow(dbColumn, outputField);
    }
}
```

**1.2 Integrate QueryParamBuilder (Already Exists!) (2 hours)**

Update PropertiesPanel.js to use QueryParamBuilder for database enrichment:

```javascript
// PropertiesPanel.js - Database enrichment section
'pre.enrichment.database': {
    fields: [
        // ... existing fields ...
        {
            key: 'queryParamsBuilder',
            label: 'Query Parameters',
            type: 'query-param-builder',  // Reuse existing component!
            required: false,
            help: 'Map SQL parameters to HL7 field paths'
        },
        {
            key: 'resultMappingBuilder',
            label: 'Result Mapping',
            type: 'result-mapping-builder',  // New component!
            required: false,
            help: 'Map database columns to output field names'
        }
    ]
}
```

**1.3 Update Form Rendering to Support New Component Types (2 hours)**

```javascript
// PropertiesPanel.js - renderFormField method
renderFormField(field, value) {
    // ... existing cases ...

    switch (field.type) {
        // ... existing types ...

        case 'query-param-builder':
            return this.renderQueryParamBuilder(field, value);

        case 'result-mapping-builder':
            return this.renderResultMappingBuilder(field, value);
    }
}

renderQueryParamBuilder(field, initialParams = {}) {
    const containerId = `queryParamBuilder_${Date.now()}`;
    const initialParamsJSON = JSON.stringify(initialParams).replace(/"/g, '&quot;');

    return `
        <div class="form-group">
            <label>${field.label}</label>
            <div class="query-param-builder-container"
                 id="${containerId}"
                 data-initial-params='${initialParamsJSON}'>
            </div>
            <small class="form-text text-muted">${field.help || ''}</small>
        </div>
    `;
}

renderResultMappingBuilder(field, initialMappings = {}) {
    const containerId = `resultMappingBuilder_${Date.now()}`;
    const initialMappingsJSON = JSON.stringify(initialMappings).replace(/"/g, '&quot;');

    return `
        <div class="form-group">
            <label>${field.label}</label>
            <div class="result-mapping-builder-container"
                 id="${containerId}"
                 data-initial-mappings='${initialMappingsJSON}'>
            </div>
            <small class="form-text text-muted">${field.help || ''}</small>
        </div>
    `;
}
```

**1.4 Initialize Result Mapping Builder (1 hour)**

```javascript
// PropertiesPanel.js - attachFormEventListeners method
// Add after Query Param Builder initialization

// === Result Mapping Builder Initialization ===
const resultMappingContainers = form.querySelectorAll('.result-mapping-builder-container');
resultMappingContainers.forEach(container => {
    const initialMappingsJSON = container.dataset.initialMappings;
    const initialMappings = initialMappingsJSON ? JSON.parse(initialMappingsJSON) : {};

    // Instantiate ResultMappingBuilder component
    const builder = new ResultMappingBuilder(container, initialMappings);

    // Store reference for later access
    container._resultMappingBuilderInstance = builder;
});
```

**1.5 Update getCurrentStepConfig to Read from Builders (1 hour)**

```javascript
// PropertiesPanel.js - getCurrentStepConfig method
getCurrentStepConfig() {
    const config = {};

    // ... existing field reading ...

    // Read from Query Param Builder (for database enrichment)
    const queryParamContainer = form.querySelector('.query-param-builder-container');
    if (queryParamContainer && queryParamContainer._queryParamBuilderInstance) {
        config.queryParams = queryParamContainer._queryParamBuilderInstance.getQueryParams();
    }

    // Read from Result Mapping Builder (NEW!)
    const resultMappingContainer = form.querySelector('.result-mapping-builder-container');
    if (resultMappingContainer && resultMappingContainer._resultMappingBuilderInstance) {
        config.resultMapping = resultMappingContainer._resultMappingBuilderInstance.getMappings();
    }

    return config;
}
```

---

### Phase 2: Database Query Tester (3-4 days)

**2.1 Create DatabaseQueryTester Component (6 hours)**

```javascript
// public/js/pipeline/components/DatabaseQueryTester.js
class DatabaseQueryTester {
    constructor(container, config = {}) {
        this.container = container;
        this.config = config;
        this.onAddMappingCallback = null;
        this.render();
    }

    render() {
        this.container.innerHTML = `
            <div class="database-query-tester">
                <div class="tester-header">
                    <h4>🧪 Test Database Query</h4>
                    <button type="button" class="run-query-btn">▶ Run Query</button>
                </div>

                <div class="test-params-section">
                    <label>Test Parameter Values</label>
                    <div id="testParamsTable"></div>
                    <small class="text-muted">
                        Enter sample values to test your query. Example: If query uses $1 for patient ID, enter "12345"
                    </small>
                </div>

                <div class="query-results-section" style="display: none;">
                    <div class="results-header">
                        <span class="results-status"></span>
                        <button type="button" class="clear-results-btn">Clear</button>
                    </div>
                    <div class="results-content"></div>
                </div>

                <div class="query-error-section" style="display: none;">
                    <div class="alert alert-danger">
                        <strong>❌ Query Failed</strong>
                        <pre class="error-message"></pre>
                    </div>
                </div>
            </div>
        `;

        this.attachEventListeners();
        this.renderTestParamsTable();
    }

    renderTestParamsTable() {
        const container = this.container.querySelector('#testParamsTable');

        // Get query params from config
        const queryParams = this.config.queryParams || {};
        const paramCount = Object.keys(queryParams).length;

        if (paramCount === 0) {
            container.innerHTML = '<p class="text-muted">No query parameters defined yet</p>';
            return;
        }

        let html = '<table class="test-params-table"><thead><tr>';
        html += '<th>Parameter</th><th>Field Path</th><th>Test Value</th>';
        html += '</tr></thead><tbody>';

        Object.entries(queryParams).forEach(([paramName, fieldPath], index) => {
            html += `
                <tr>
                    <td>$${index + 1}</td>
                    <td>${fieldPath}</td>
                    <td><input type="text" class="test-param-value" data-param="${paramName}" placeholder="Enter test value"></td>
                </tr>
            `;
        });

        html += '</tbody></table>';
        container.innerHTML = html;
    }

    attachEventListeners() {
        const runBtn = this.container.querySelector('.run-query-btn');
        runBtn.addEventListener('click', () => this.executeQuery());

        const clearBtn = this.container.querySelector('.clear-results-btn');
        clearBtn.addEventListener('click', () => this.clearResults());
    }

    async executeQuery() {
        const runBtn = this.container.querySelector('.run-query-btn');
        runBtn.disabled = true;
        runBtn.textContent = '⏳ Running...';

        this.clearResults();

        try {
            // Collect test parameter values
            const testParams = {};
            const paramInputs = this.container.querySelectorAll('.test-param-value');
            paramInputs.forEach(input => {
                testParams[input.dataset.param] = input.value;
            });

            // Build request payload
            const payload = {
                databaseType: this.config.databaseType,
                connectionString: this.config.connectionString,
                connectionName: this.config.connectionName,
                query: this.config.query,
                queryParams: this.config.queryParams,
                testParams: testParams
            };

            console.log('🔍 Testing database query:', payload);

            // Call backend API to test query
            const response = await fetch('/api/database/test-query', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            });

            const result = await response.json();

            if (response.ok && result.success) {
                this.displayResults(result.data);
            } else {
                this.displayError(result.error || 'Query failed');
            }
        } catch (error) {
            console.error('Query test error:', error);
            this.displayError(error.message);
        } finally {
            runBtn.disabled = false;
            runBtn.textContent = '▶ Run Query';
        }
    }

    displayResults(rows) {
        const resultsSection = this.container.querySelector('.query-results-section');
        const resultsContent = this.container.querySelector('.results-content');
        const resultsStatus = this.container.querySelector('.results-status');

        resultsSection.style.display = 'block';
        resultsStatus.innerHTML = `✅ Query Result (${rows.length} row${rows.length !== 1 ? 's' : ''})`;

        if (rows.length === 0) {
            resultsContent.innerHTML = '<p class="text-muted">No rows returned</p>';
            return;
        }

        // Display first row with "Add to Mapping" buttons
        const firstRow = rows[0];
        let html = '<div class="query-result-row">';

        Object.entries(firstRow).forEach(([column, value]) => {
            html += `
                <div class="result-field">
                    <div class="field-info">
                        <strong>${column}</strong>
                        <span class="field-value">${this.formatValue(value)}</span>
                    </div>
                    <button type="button"
                            class="add-mapping-btn"
                            data-column="${column}">
                        ➕ Add to Mapping
                    </button>
                </div>
            `;
        });

        html += '</div>';

        // Show all rows as JSON (collapsed)
        html += `
            <details>
                <summary>View all ${rows.length} rows (JSON)</summary>
                <pre class="all-rows-json">${JSON.stringify(rows, null, 2)}</pre>
            </details>
        `;

        resultsContent.innerHTML = html;

        // Attach add mapping handlers
        this.attachAddMappingHandlers();
    }

    attachAddMappingHandlers() {
        const addBtns = this.container.querySelectorAll('.add-mapping-btn');
        addBtns.forEach(btn => {
            btn.addEventListener('click', () => {
                const column = btn.dataset.column;
                this.addMapping(column);
            });
        });
    }

    addMapping(dbColumn) {
        if (this.onAddMappingCallback) {
            this.onAddMappingCallback(dbColumn);
        }

        // Visual feedback
        const btn = this.container.querySelector(`[data-column="${dbColumn}"]`);
        btn.textContent = '✅ Added';
        btn.disabled = true;

        setTimeout(() => {
            btn.textContent = '➕ Add to Mapping';
            btn.disabled = false;
        }, 2000);
    }

    setOnAddMapping(callback) {
        this.onAddMappingCallback = callback;
    }

    displayError(errorMessage) {
        const errorSection = this.container.querySelector('.query-error-section');
        const errorMsg = this.container.querySelector('.error-message');

        errorSection.style.display = 'block';
        errorMsg.textContent = errorMessage;
    }

    clearResults() {
        this.container.querySelector('.query-results-section').style.display = 'none';
        this.container.querySelector('.query-error-section').style.display = 'none';
    }

    formatValue(value) {
        if (value === null) return '<em>null</em>';
        if (typeof value === 'string') return `"${value}"`;
        return String(value);
    }

    // Update config (when user changes query/params in form)
    updateConfig(config) {
        this.config = config;
        this.renderTestParamsTable();
        this.clearResults();
    }
}
```

**2.2 Add Backend API Endpoint for Query Testing (4 hours)**

```go
// controllers/database_test_controller.go (NEW FILE)
package controllers

import (
    "context"
    "database/sql"
    "encoding/json"
    "ezhealthkonnect/models"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

type DatabaseTestController struct {
    db *sql.DB
}

func NewDatabaseTestController(db *sql.DB) *DatabaseTestController {
    return &DatabaseTestController{db: db}
}

type TestQueryRequest struct {
    DatabaseType     string            `json:"databaseType"`
    ConnectionString string            `json:"connectionString"`
    ConnectionName   string            `json:"connectionName"`
    Query            string            `json:"query"`
    QueryParams      map[string]string `json:"queryParams"`
    TestParams       map[string]string `json:"testParams"`  // User-provided test values
}

func (c *DatabaseTestController) TestQuery(ctx *gin.Context) {
    var req TestQueryRequest
    if err := ctx.ShouldBindJSON(&req); err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Invalid request: " + err.Error(),
        })
        return
    }

    // Validate required fields
    if req.Query == "" {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Query is required",
        })
        return
    }

    // Get database connection
    db, err := c.getTestConnection(req)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Connection failed: " + err.Error(),
        })
        return
    }
    defer db.Close()

    // Build parameter values
    params := make([]interface{}, 0)
    for _, testValue := range req.TestParams {
        params = append(params, testValue)
    }

    // Execute query with timeout
    queryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    rows, err := db.QueryContext(queryCtx, req.Query, params...)
    if err != nil {
        ctx.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Query execution failed: " + err.Error(),
        })
        return
    }
    defer rows.Close()

    // Get column names
    columns, err := rows.Columns()
    if err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to get columns: " + err.Error(),
        })
        return
    }

    // Build result set
    results := make([]map[string]interface{}, 0)

    for rows.Next() {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range columns {
            valuePtrs[i] = &values[i]
        }

        if err := rows.Scan(valuePtrs...); err != nil {
            ctx.JSON(http.StatusInternalServerError, gin.H{
                "success": false,
                "error":   "Failed to scan row: " + err.Error(),
            })
            return
        }

        row := make(map[string]interface{})
        for i, colName := range columns {
            val := values[i]
            if b, ok := val.([]byte); ok {
                row[colName] = string(b)
            } else {
                row[colName] = val
            }
        }

        results = append(results, row)
    }

    if err := rows.Err(); err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Error iterating rows: " + err.Error(),
        })
        return
    }

    ctx.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    results,
        "count":   len(results),
    })
}

func (c *DatabaseTestController) getTestConnection(req TestQueryRequest) (*sql.DB, error) {
    // If connection name specified, look up from database_connections table
    if req.ConnectionName != "" {
        // TODO: Implement connection registry
        return nil, fmt.Errorf("named connections not yet implemented")
    }

    // Use provided connection string
    if req.ConnectionString == "" {
        return nil, fmt.Errorf("connectionString is required")
    }

    driverName := c.getDriverName(req.DatabaseType)
    db, err := sql.Open(driverName, req.ConnectionString)
    if err != nil {
        return nil, err
    }

    // Test connection
    if err := db.Ping(); err != nil {
        return nil, err
    }

    return db, nil
}

func (c *DatabaseTestController) getDriverName(dbType string) string {
    switch dbType {
    case "postgresql", "postgres":
        return "postgres"
    case "mysql":
        return "mysql"
    case "sqlserver", "mssql":
        return "sqlserver"
    case "oracle":
        return "oracle"
    default:
        return "postgres"
    }
}
```

**2.3 Register API Route in main.go (1 hour)**

```go
// main.go
dbTestController := controllers.NewDatabaseTestController(db)
apiGroup.POST("/database/test-query", dbTestController.TestQuery)
```

**2.4 Integrate DatabaseQueryTester into PropertiesPanel (2 hours)**

```javascript
// PropertiesPanel.js - Add to field configuration
'pre.enrichment.database': {
    fields: [
        // ... existing fields ...
        {
            key: 'databaseQueryTester',
            label: '🧪 Test Database Query',
            type: 'database-query-tester',
            required: false,
            help: 'Test your query with sample parameters before saving. Click [➕] on result fields to automatically add them to Result Mapping.'
        }
    ]
}

// Add rendering method
renderDatabaseQueryTester(field, value) {
    const containerId = `dbQueryTester_${Date.now()}`;

    return `
        <div class="form-group">
            <label>${field.label}</label>
            <div class="database-query-tester-container" id="${containerId}"></div>
            <small class="form-text text-muted">${field.help || ''}</small>
        </div>
    `;
}

// Initialize in attachFormEventListeners
const dbQueryTesterContainers = form.querySelectorAll('.database-query-tester-container');
dbQueryTesterContainers.forEach(container => {
    const tester = new DatabaseQueryTester(container, {});

    // Update tester when query/params change
    const queryInput = form.querySelector('[name="config_query"]');
    const updateTesterConfig = () => {
        tester.updateConfig({
            databaseType: form.querySelector('[name="config_databaseType"]')?.value,
            connectionString: form.querySelector('[name="config_connectionString"]')?.value,
            query: queryInput?.value,
            queryParams: queryParamBuilder?.getQueryParams() || {}
        });
    };

    queryInput?.addEventListener('blur', updateTesterConfig);

    // Set callback for adding mappings
    tester.setOnAddMapping((dbColumn) => {
        const resultMappingBuilder = form.querySelector('.result-mapping-builder-container')?._resultMappingBuilderInstance;
        if (resultMappingBuilder) {
            resultMappingBuilder.addMappingFromQueryResult(dbColumn);
        }
    });

    container._databaseQueryTesterInstance = tester;
});
```

---

### Phase 3: Connection Manager (Optional - 2-3 days)

**3.1 Database Connection Table Migration**

```sql
-- V31__Add_Database_Connections.sql
CREATE TABLE database_connections (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL UNIQUE,
    database_type VARCHAR(50) NOT NULL,  -- postgresql, mysql, etc.
    connection_string TEXT NOT NULL,     -- Encrypted
    description TEXT,
    created_by VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_db_connections_name ON database_connections(name);
```

**3.2 UI for Managing Connections**

New page: `/database-connections.html`

---

### Phase 4: Documentation Panel (1 day)

Add collapsible help section to database enrichment form.

---

## Summary of Changes

### New Files Created
1. `public/js/pipeline/components/ResultMappingBuilder.js` - Visual result mapping builder
2. `public/js/pipeline/components/DatabaseQueryTester.js` - Test queries before saving
3. `controllers/database_test_controller.go` - Backend API for query testing
4. `database/migrations/V31__Add_Database_Connections.sql` - Named connections (optional)

### Files Modified
1. `public/js/pipeline/managers/PropertiesPanel.js` - Add new component types and initialization
2. `public/pipeline-builder.html` - Include new component scripts
3. `main.go` - Register test query API endpoint

### CSS Needed
1. `public/css/result-mapping-builder.css` - Styling for result mapping table
2. `public/css/database-query-tester.css` - Styling for query tester component

---

## Before vs After Comparison

### Current User Experience (Not No-Code)
```
User Task: Configure database enrichment to look up patient demographics

Steps:
1. Select "Database Enrichment" step
2. Type connection string with password in plain text
3. Write SQL query manually
4. Write JSON for query params: {"patientId": "PID.3"}
5. Write JSON for result mapping: {"patient_name": "fullName"}
6. Save pipeline
7. Send test message
8. Check logs to see if query worked
9. If failed, go back to step 3 and guess what's wrong

Pain Points:
❌ Raw JSON editing (error-prone)
❌ No validation until execution
❌ Can't test query before saving
❌ No field autocomplete
❌ Passwords visible in UI
❌ No guidance on what fields are available
```

### Proposed User Experience (True No-Code)
```
User Task: Configure database enrichment to look up patient demographics

Steps:
1. Select "Database Enrichment" step
2. Choose saved connection "Epic Production DB" from dropdown
3. Paste SQL query (or use visual query builder - future enhancement)
4. Click "Add Parameter" → Select "PID.3" from field dropdown
5. Click "▶ Run Query" with test value "12345"
6. See actual results from database
7. Click "➕" next to each field to auto-add to Result Mapping
8. Save pipeline

Benefits:
✅ Visual builders (no JSON editing)
✅ Saved connections (no passwords in UI)
✅ Test query before saving (instant feedback)
✅ Auto-complete for field paths
✅ Click-to-add mapping from query results
✅ See actual database response before configuring
✅ Validation before execution
```

---

## Estimated Effort

| Phase | Effort | Priority |
|-------|--------|----------|
| Phase 1: Visual Builders | 2-3 days | HIGH |
| Phase 2: Query Tester | 3-4 days | HIGH |
| Phase 3: Connection Manager | 2-3 days | MEDIUM |
| Phase 4: Documentation | 1 day | LOW |
| **Total** | **8-11 days** | - |

---

## Recommendation

**Start with Phases 1 & 2** (Visual Builders + Query Tester) = **5-7 days**

This gives you:
- ✅ No-code visual configuration
- ✅ Ability to test queries before saving
- ✅ Auto-mapping from query results
- ✅ Compliance with no-code goal

**Defer Phase 3** (Connection Manager) until you have multiple users who need shared connections.

**Defer Phase 4** (Documentation) - nice-to-have but not critical.

---

## Next Steps

Should I implement:
1. **Phase 1 only** (Visual Builders) - Quickest win, 2-3 days
2. **Phases 1 + 2** (Visual Builders + Query Tester) - Recommended, 5-7 days
3. **All phases** (Complete solution) - 8-11 days

What's your preference?
