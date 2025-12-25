# Database Enrichment - Complete Test Plan

## 🎯 Test Objectives

1. Verify Phase 1 (Visual Builders) functions correctly
2. Verify Phase 2 (Database Query Tester) works as expected
3. Verify Documentation Tab displays comprehensive help
4. Verify Import JSON Tab handles database enrichment configuration correctly
5. Verify Fullscreen mode works properly
6. End-to-end integration testing

## ✅ Test Environment Setup

### Prerequisites
- Docker containers running (app, postgres, mongodb)
- Access to http://localhost:3000/pipeline-builder.html
- Sample database with test data (users table)
- Browser with developer tools (F12)

### Test Database Setup
```sql
-- Verify users table exists
SELECT * FROM users LIMIT 5;

-- If needed, create test data
INSERT INTO users (email, role, created_at)
VALUES ('test@example.com', 'admin', NOW())
ON CONFLICT DO NOTHING;
```

---

## 📋 Test Cases

### TEST SUITE 1: Phase 1 - Visual Builders

#### Test Case 1.1: Query Parameter Builder - Basic Functionality
**Objective**: Verify Query Parameter Builder appears and functions correctly

**Steps**:
1. Open http://localhost:3000/pipeline-builder.html
2. Drag "Database Enrichment" from Pre-Processing toolbox
3. Drop on canvas
4. Click the step to open Properties Panel modal
5. Verify Configuration tab is active

**Expected Results**:
- ✅ Query Parameters section displays with visual table (NOT JSON textarea)
- ✅ Empty state shows: "No query parameters defined yet"
- ✅ Info message: "Add parameters in the 'Query Parameters' section above first"
- ✅ No raw JSON editing visible

**Pass Criteria**: Visual table displays instead of JSON textarea

---

#### Test Case 1.2: Query Parameter Builder - Add Parameter
**Objective**: Verify adding query parameters works correctly

**Steps**:
1. Complete Test Case 1.1
2. Scroll to "Query Parameters" section
3. Click "[+ Add Parameter]" button
4. In the new row, click the HL7 Field Path dropdown
5. Select "enhancedSegments.PID.fields[2].value" (Patient ID)
6. Verify parameter number displays as "$1"

**Expected Results**:
- ✅ New row appears in Query Parameters table
- ✅ Parameter displays as "$1" (auto-numbered)
- ✅ Field path dropdown shows HL7 fields
- ✅ Selected field path appears in the input

**Pass Criteria**: Parameter row added with correct numbering

---

#### Test Case 1.3: Query Parameter Builder - Multiple Parameters
**Objective**: Verify adding multiple parameters with correct sequencing

**Steps**:
1. Complete Test Case 1.2
2. Click "[+ Add Parameter]" again
3. Select "enhancedSegments.PID.fields[3].value" (Date of Birth)
4. Click "[+ Add Parameter]" a third time
5. Select "enhancedSegments.PID.fields[4].value" (Sex)
6. Verify parameter numbers: $1, $2, $3

**Expected Results**:
- ✅ Three parameter rows displayed
- ✅ Parameters numbered: $1, $2, $3
- ✅ Each row has correct field path
- ✅ Order is maintained

**Pass Criteria**: All parameters correctly numbered and displayed

---

#### Test Case 1.4: Query Parameter Builder - Remove Parameter
**Objective**: Verify removing parameters works and re-numbers correctly

**Steps**:
1. Complete Test Case 1.3 (3 parameters: $1, $2, $3)
2. Click 🗑️ (trash icon) on the second parameter ($2)
3. Verify parameters re-number to $1, $2 (not $1, $3)

**Expected Results**:
- ✅ Second parameter removed
- ✅ Remaining parameters re-number: $1, $2
- ✅ No gaps in parameter numbering
- ✅ Field paths remain correct

**Pass Criteria**: Parameters re-number correctly after deletion

---

#### Test Case 1.5: Result Mapping Builder - Basic Functionality
**Objective**: Verify Result Mapping Builder appears correctly

**Steps**:
1. Open database enrichment step
2. Scroll to "Result Mapping" section
3. Verify visual table builder appears (NOT JSON textarea)

**Expected Results**:
- ✅ Result Mapping table displays with columns: "DB Column", "Output Field", "Actions"
- ✅ Empty state shows: "No mappings defined yet"
- ✅ "[+ Add Mapping]" button visible
- ✅ No raw JSON editing

**Pass Criteria**: Visual table displays instead of JSON textarea

---

#### Test Case 1.6: Result Mapping Builder - Add Mapping
**Objective**: Verify adding result mappings manually

**Steps**:
1. Complete Test Case 1.5
2. Click "[+ Add Mapping]" button
3. In "DB Column" field, type: `id`
4. In "Output Field" field, type: `userId`
5. Verify row appears in table

**Expected Results**:
- ✅ New row added to Result Mapping table
- ✅ DB Column shows: `id`
- ✅ Output Field shows: `userId`
- ✅ 🗑️ delete button appears

**Pass Criteria**: Mapping row added with correct values

---

#### Test Case 1.7: Result Mapping Builder - CamelCase Conversion
**Objective**: Verify auto camelCase conversion for snake_case columns

**Steps**:
1. Complete Test Case 1.6
2. Click "[+ Add Mapping]" again
3. In "DB Column" field, type: `created_at`
4. Tab to "Output Field" or click elsewhere
5. Verify Output Field auto-populates with: `createdAt` (camelCase)

**Expected Results**:
- ✅ DB Column: `created_at`
- ✅ Output Field auto-converts to: `createdAt`
- ✅ Snake_case converted to camelCase automatically
- ✅ User can override the suggested name if desired

**Pass Criteria**: Auto camelCase conversion works correctly

---

#### Test Case 1.8: Result Mapping Builder - Remove Mapping
**Objective**: Verify removing mappings works

**Steps**:
1. Complete Test Case 1.7 (2 mappings)
2. Click 🗑️ on the first mapping row
3. Verify row is removed
4. Verify remaining mapping is still intact

**Expected Results**:
- ✅ First mapping removed
- ✅ Second mapping remains
- ✅ No errors in console

**Pass Criteria**: Mapping removed successfully

---

### TEST SUITE 2: Phase 2 - Database Query Tester

#### Test Case 2.1: Query Tester - Visibility
**Objective**: Verify Database Query Tester component appears

**Steps**:
1. Open database enrichment step
2. Scroll down to "🧪 Test Database Query" section

**Expected Results**:
- ✅ Purple gradient header displays
- ✅ Title: "🧪 Test Database Query"
- ✅ "▶ Run Query" button visible (white button on right)
- ✅ Tester body with "Test Parameter Values" section

**Pass Criteria**: Query Tester displays with correct styling

---

#### Test Case 2.2: Query Tester - Configure and Test Simple Query
**Objective**: Verify testing a basic SQL query works

**Steps**:
1. Complete Test Case 2.1
2. Configure step:
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://postgres:postgres@postgres:5432/ezhealthkonnect`
   - SQL Query: `SELECT id, email, role, created_at FROM users LIMIT 1`
3. Leave Query Parameters empty (no parameters)
4. Scroll to Query Tester
5. Click "▶ Run Query" button

**Expected Results**:
- ✅ Button shows spinner: "Running..."
- ✅ After ~500ms, results appear
- ✅ Green checkmark: "✅ Query Result (1 row)"
- ✅ Each database field displays in a card:
  - 🗄️ id: `1` [➕ Add to Mapping]
  - 🗄️ email: `"admin@..."` [➕ Add to Mapping]
  - 🗄️ role: `"admin"` [➕ Add to Mapping]
  - 🗄️ created_at: `"2024-..."` [➕ Add to Mapping]
- ✅ "View as JSON" collapsible section
- ✅ Tip box: "Click [➕ Add to Mapping] to automatically add fields..."

**Pass Criteria**: Query executes and results display correctly

---

#### Test Case 2.3: Query Tester - Click-to-Add Mapping
**Objective**: Verify clicking "Add to Mapping" populates Result Mapping Builder

**Steps**:
1. Complete Test Case 2.2 (query results visible)
2. Scroll up to Result Mapping Builder - verify it's empty or has few mappings
3. Scroll back to query results
4. Click "[➕ Add to Mapping]" next to the `email` field
5. Immediately scroll up to Result Mapping Builder

**Expected Results**:
- ✅ Button changes to "✅ Added" (green) for 2 seconds
- ✅ New row appears in Result Mapping Builder:
  - DB Column: `email`
  - Output Field: `email`
- ✅ No page reload required
- ✅ Mapping persists if you save step

**Pass Criteria**: Mapping auto-added from query result

---

#### Test Case 2.4: Query Tester - Click-to-Add Multiple Mappings
**Objective**: Verify adding multiple mappings from query results

**Steps**:
1. Complete Test Case 2.3
2. Click "[➕ Add to Mapping]" next to `id`
3. Click "[➕ Add to Mapping]" next to `role`
4. Click "[➕ Add to Mapping]" next to `created_at`
5. Scroll to Result Mapping Builder

**Expected Results**:
- ✅ Result Mapping Builder now has 4 mappings:
  1. `email` → `email`
  2. `id` → `id`
  3. `role` → `role`
  4. `created_at` → `createdAt` (camelCase!)
- ✅ Each field added only once (no duplicates if clicked twice)

**Pass Criteria**: All mappings added with correct camelCase conversion

---

#### Test Case 2.5: Query Tester - Parameterized Query
**Objective**: Verify testing queries with parameters

**Steps**:
1. Open new database enrichment step
2. Configure:
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://postgres:postgres@postgres:5432/ezhealthkonnect`
   - SQL Query: `SELECT id, email, role FROM users WHERE role = $1`
3. Add Query Parameter:
   - Click "[+ Add Parameter]"
   - Field Path: `enhancedSegments.PID.fields[0].value` (any field)
4. Scroll to Query Tester
5. Verify "Test Parameter Values" table appears with:
   - Parameter: `$1`
   - Field Path: `enhancedSegments.PID.fields[0].value`
   - Test Value: [empty input field]
6. Type `admin` in the Test Value field
7. Click "▶ Run Query"

**Expected Results**:
- ✅ Test Parameter Values table displays
- ✅ Shows parameter $1 with field path
- ✅ Test Value input field visible
- ✅ Query executes with test value "admin"
- ✅ Results show only users with role="admin"

**Pass Criteria**: Parameterized query executes correctly

---

#### Test Case 2.6: Query Tester - Error Handling (Invalid Query)
**Objective**: Verify error handling for SQL syntax errors

**Steps**:
1. Open database enrichment step
2. Configure:
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://postgres:postgres@postgres:5432/ezhealthkonnect`
   - SQL Query: `SELECT * FROM nonexistent_table`
3. Click "▶ Run Query"

**Expected Results**:
- ✅ Red error box appears
- ✅ Title: "❌ Query Failed"
- ✅ Error message displays: "Query execution failed: relation 'nonexistent_table' does not exist"
- ✅ No results section visible
- ✅ User can fix query and try again

**Pass Criteria**: Error displayed clearly with helpful message

---

#### Test Case 2.7: Query Tester - Error Handling (Connection Failed)
**Objective**: Verify error handling for connection errors

**Steps**:
1. Open database enrichment step
2. Configure:
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://baduser:badpass@postgres:5432/ezhealthkonnect`
   - SQL Query: `SELECT 1`
3. Click "▶ Run Query"

**Expected Results**:
- ✅ Red error box appears
- ✅ Error message: "Connection failed: pq: password authentication failed for user 'baduser'"
- ✅ Clear indication of connection issue
- ✅ User can fix connection string and retry

**Pass Criteria**: Connection error handled gracefully

---

#### Test Case 2.8: Query Tester - Empty Result Set
**Objective**: Verify handling of queries that return no rows

**Steps**:
1. Open database enrichment step
2. Configure valid connection and query:
   - SQL Query: `SELECT * FROM users WHERE id = 999999`
3. Click "▶ Run Query"

**Expected Results**:
- ✅ Green checkmark: "✅ Query Result (0 rows)"
- ✅ Message: "🗄️ Query executed successfully but returned no rows"
- ✅ No error displayed (query was valid, just no matching data)
- ✅ Clear indication that query works but no data matches

**Pass Criteria**: Empty results handled correctly

---

#### Test Case 2.9: Query Tester - Real-Time Config Updates
**Objective**: Verify tester updates when user changes query/connection

**Steps**:
1. Open database enrichment step
2. Configure initial query with 1 parameter: `SELECT * FROM users WHERE email = $1`
3. Add one query parameter
4. Verify Query Tester shows 1 parameter in Test Parameter Values
5. Edit SQL Query to: `SELECT * FROM users WHERE email = $1 AND role = $2`
6. Click outside the query textarea (blur event)
7. Scroll to Query Tester

**Expected Results**:
- ✅ Query Tester detects query changed
- ✅ Test Parameter Values table still shows 1 parameter (doesn't auto-update count)
- ✅ User must add second parameter in Query Parameter Builder
- ✅ After adding second parameter, Query Tester updates to show 2 parameters

**Pass Criteria**: Config updates propagate to tester on blur

---

### TEST SUITE 3: Documentation Tab

#### Test Case 3.1: Documentation Tab - Visibility
**Objective**: Verify Documentation tab displays comprehensive help

**Steps**:
1. Open database enrichment step
2. Click "Documentation" tab
3. Scroll through all sections

**Expected Results**:
- ✅ Tab switches to Documentation
- ✅ Content displays in readable format
- ✅ All sections visible:
  - Description
  - Use Cases (6 examples)
  - Configuration Example
  - Parameters (8 parameters documented)
  - NO-CODE Features (4 features)
  - Workflow (8 steps)
  - Best Practices (6 practices)
  - Troubleshooting (5 issues)
  - Security Notes (4 notes)

**Pass Criteria**: All documentation sections display correctly

---

#### Test Case 3.2: Documentation - NO-CODE Features Section
**Objective**: Verify NO-CODE Features section explains visual builders

**Steps**:
1. Complete Test Case 3.1
2. Find "NO-CODE Features" section
3. Read each feature description

**Expected Results**:
- ✅ 4 features documented:
  1. Query Parameter Builder
  2. Result Mapping Builder
  3. Database Query Tester
  4. Click-to-Add Mapping
- ✅ Each feature has:
  - Feature name
  - Description (what it is)
  - How-to (how to use it)
  - Benefit (why it's valuable)
- ✅ Clear, actionable instructions

**Pass Criteria**: NO-CODE features clearly explained

---

#### Test Case 3.3: Documentation - Workflow Section
**Objective**: Verify step-by-step workflow guide is present

**Steps**:
1. Complete Test Case 3.1
2. Find "Workflow" section
3. Verify 8-step process is documented

**Expected Results**:
- ✅ 8 steps documented:
  1. Select database type
  2. Enter connection string
  3. Write SQL query
  4. Map query parameters
  5. **Test your query! 🧪**
  6. Click-to-add result mappings
  7. Configure target path
  8. Save step
- ✅ Step 5 emphasizes testing before saving
- ✅ Clear, sequential workflow

**Pass Criteria**: Workflow provides clear step-by-step guidance

---

#### Test Case 3.4: Documentation - Troubleshooting Section
**Objective**: Verify troubleshooting guide helps users solve common issues

**Steps**:
1. Complete Test Case 3.1
2. Find "Troubleshooting" section
3. Verify common issues are documented

**Expected Results**:
- ✅ 5 common issues documented:
  1. Connection failed (hostname resolution)
  2. Query execution failed (SQL syntax)
  3. Query returned no rows (data mismatch)
  4. Timeout exceeded (query optimization)
  5. Parameter mismatch (parameter count)
- ✅ Each issue has:
  - Issue description
  - Cause
  - Fix
- ✅ Practical, actionable fixes

**Pass Criteria**: Troubleshooting guide is helpful and complete

---

### TEST SUITE 4: Import JSON Tab

#### Test Case 4.1: JSON Tab - Export Configuration
**Objective**: Verify JSON tab exports database enrichment config correctly

**Steps**:
1. Open database enrichment step
2. Configure:
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://user:pass@host:5432/db`
   - SQL Query: `SELECT id, email FROM users WHERE email = $1`
   - Add 1 query parameter: `1` → `enhancedSegments.PID.fields[13].value`
   - Add 2 result mappings: `id` → `userId`, `email` → `userEmail`
   - Target Path: `enriched.user`
3. Click "Import JSON" tab
4. Review JSON in textarea

**Expected Results**:
- ✅ JSON displays formatted (2-space indent)
- ✅ Contains all configured fields:
  ```json
  {
    "stepName": "Database Enrichment",
    "stepType": "pre.enrichment.database",
    "config": {
      "databaseType": "PostgreSQL",
      "connectionString": "postgresql://user:pass@host:5432/db",
      "query": "SELECT id, email FROM users WHERE email = $1",
      "queryParams": { "1": "enhancedSegments.PID.fields[13].value" },
      "queryParamsBuilder": { "1": "enhancedSegments.PID.fields[13].value" },
      "resultMapping": { "id": "userId", "email": "userEmail" },
      "resultMappingBuilder": { "id": "userId", "email": "userEmail" },
      "targetPath": "enriched.user"
    }
  }
  ```
- ✅ Both `queryParams` and `queryParamsBuilder` keys present (dual-key compatibility)
- ✅ Both `resultMapping` and `resultMappingBuilder` keys present

**Pass Criteria**: JSON export includes all configuration with dual-key compatibility

---

#### Test Case 4.2: JSON Tab - Import Configuration
**Objective**: Verify importing JSON populates visual builders

**Steps**:
1. Open new database enrichment step
2. Click "Import JSON" tab
3. Clear existing JSON
4. Paste this JSON:
   ```json
   {
     "stepName": "Imported DB Enrichment",
     "stepType": "pre.enrichment.database",
     "config": {
       "databaseType": "MySQL",
       "connectionString": "mysql://user:pass@host:3306/db",
       "query": "SELECT name, age FROM patients WHERE mrn = ?",
       "queryParams": { "1": "enhancedSegments.PID.fields[2].value" },
       "resultMapping": { "name": "patientName", "age": "patientAge" }
     }
   }
   ```
5. Click "✅ Apply JSON" button
6. Switch to "Configuration" tab

**Expected Results**:
- ✅ Success message: "✅ Configuration imported successfully"
- ✅ Configuration tab shows:
  - Database Type: `MySQL`
  - Connection String: `mysql://user:pass@host:3306/db`
  - SQL Query: `SELECT name, age FROM patients WHERE mrn = ?`
- ✅ Query Parameter Builder shows: $1 → `enhancedSegments.PID.fields[2].value`
- ✅ Result Mapping Builder shows:
  - `name` → `patientName`
  - `age` → `patientAge`

**Pass Criteria**: JSON import populates all visual builders correctly

---

#### Test Case 4.3: JSON Tab - Validate JSON
**Objective**: Verify JSON validation catches errors

**Steps**:
1. Open database enrichment step
2. Click "Import JSON" tab
3. Paste invalid JSON:
   ```
   {
     "stepName": "Invalid",
     "config": {
       "databaseType": "PostgreSQL"
       "query": "SELECT 1"
     }
   }
   ```
   (Missing comma after `"PostgreSQL"`)
4. Click "✅ Validate" button

**Expected Results**:
- ✅ Red validation error appears
- ✅ Error message: "❌ Invalid JSON: Unexpected string in JSON at position..."
- ✅ Helpful error message indicates syntax issue
- ✅ "Apply JSON" button disabled or shows warning

**Pass Criteria**: Invalid JSON detected and reported clearly

---

#### Test Case 4.4: JSON Tab - Copy to Clipboard
**Objective**: Verify copy to clipboard functionality

**Steps**:
1. Configure database enrichment step with sample data
2. Click "Import JSON" tab
3. Click "📋 Copy" button
4. Open a text editor
5. Paste (Ctrl+V)

**Expected Results**:
- ✅ Success message: "✅ Copied to clipboard"
- ✅ JSON pastes successfully in text editor
- ✅ All configuration preserved
- ✅ Formatted JSON (readable)

**Pass Criteria**: Copy to clipboard works correctly

---

### TEST SUITE 5: Fullscreen Mode

#### Test Case 5.1: Fullscreen Toggle - Enter Fullscreen
**Objective**: Verify entering fullscreen mode

**Steps**:
1. Open database enrichment step
2. Look at modal header (blue gradient)
3. Click the "⛶" (expand) icon next to the X button
4. Observe modal size change

**Expected Results**:
- ✅ Modal expands to fill entire viewport (100vw x 100vh)
- ✅ Icon changes from expand (⛶) to compress (⛶)
- ✅ Title updates to "Exit Fullscreen"
- ✅ No rounded corners (fullscreen fills screen edge-to-edge)
- ✅ More space for configuration forms
- ✅ Console log: "✅ Entered fullscreen mode"

**Pass Criteria**: Modal enters fullscreen correctly

---

#### Test Case 5.2: Fullscreen Toggle - Exit Fullscreen
**Objective**: Verify exiting fullscreen mode

**Steps**:
1. Complete Test Case 5.1 (in fullscreen)
2. Click the compress icon
3. Observe modal size change

**Expected Results**:
- ✅ Modal returns to normal size (900px max-width, 90vh max-height)
- ✅ Icon changes back to expand (⛶)
- ✅ Title returns to "Toggle Fullscreen"
- ✅ Rounded corners restored
- ✅ Console log: "✅ Exited fullscreen mode"

**Pass Criteria**: Modal exits fullscreen correctly

---

#### Test Case 5.3: Fullscreen - F11 Keyboard Shortcut
**Objective**: Verify F11 key toggles fullscreen

**Steps**:
1. Open database enrichment step (normal size)
2. Press F11 key
3. Observe modal enters fullscreen
4. Press F11 again
5. Observe modal exits fullscreen

**Expected Results**:
- ✅ F11 enters fullscreen (same as clicking button)
- ✅ F11 again exits fullscreen
- ✅ Browser's own fullscreen not triggered (preventDefault)
- ✅ Icon updates correctly on each toggle

**Pass Criteria**: F11 keyboard shortcut works

---

#### Test Case 5.4: Fullscreen - ESC Key Behavior
**Objective**: Verify ESC key closes modal (not just exit fullscreen)

**Steps**:
1. Open database enrichment step
2. Enter fullscreen mode
3. Press ESC key

**Expected Results**:
- ✅ Modal closes completely
- ✅ Fullscreen state resets
- ✅ Next time step opens, it's in normal (not fullscreen) mode

**Pass Criteria**: ESC closes modal and resets fullscreen state

---

### TEST SUITE 6: End-to-End Integration

#### Test Case 6.1: E2E - Complete Configuration Flow
**Objective**: Test complete user workflow from start to finish

**Steps**:
1. Open pipeline builder
2. Add Database Enrichment step
3. Configure basic settings:
   - Step Name: "EMPI Patient Lookup"
   - Database Type: `PostgreSQL`
   - Connection String: `postgresql://postgres:postgres@postgres:5432/ezhealthkonnect`
   - SQL Query: `SELECT id, email, role, created_at FROM users WHERE email = $1 LIMIT 1`
4. Add query parameter:
   - Click "[+ Add Parameter]"
   - Field Path: `enhancedSegments.PID.fields[13].value`
5. Test query:
   - Scroll to Query Tester
   - Enter test value: `admin@ezhealthkonnect.com`
   - Click "▶ Run Query"
   - Verify results appear
6. Click-to-add mappings:
   - Click "[➕ Add to Mapping]" on `id`
   - Click "[➕ Add to Mapping]" on `email`
   - Click "[➕ Add to Mapping]" on `role`
   - Click "[➕ Add to Mapping]" on `created_at`
7. Configure target path:
   - Target Path: `enriched.empi`
8. Save step:
   - Click "Save" button
   - Close modal
9. Verify step on canvas shows configuration
10. Save pipeline
11. Reload page
12. Click step again
13. Verify all configuration persisted

**Expected Results**:
- ✅ All configuration steps complete without errors
- ✅ Query tester works and shows results
- ✅ Mappings auto-added from query results
- ✅ Step saves successfully
- ✅ Configuration persists after page reload
- ✅ Both visual builders and JSON config saved correctly

**Pass Criteria**: Complete E2E flow works perfectly

---

#### Test Case 6.2: E2E - Backend Compatibility
**Objective**: Verify saved configuration works with backend executor

**Steps**:
1. Complete Test Case 6.1
2. Open database and query transformation steps:
   ```sql
   SELECT step_name, config
   FROM transformation_steps
   WHERE step_type = 'pre.enrichment.database'
   ORDER BY created_at DESC
   LIMIT 1;
   ```
3. Verify config structure

**Expected Results**:
- ✅ Config contains both UI keys and backend keys:
  - `queryParams` (backend key)
  - `queryParamsBuilder` (UI key)
  - `resultMapping` (backend key)
  - `resultMappingBuilder` (UI key)
- ✅ Values match between both keys
- ✅ Backend executor can read `queryParams` and `resultMapping`
- ✅ UI can read `queryParamsBuilder` and `resultMappingBuilder`

**Pass Criteria**: Dual-key storage ensures backward compatibility

---

#### Test Case 6.3: E2E - Multiple Database Enrichment Steps
**Objective**: Verify multiple database enrichment steps in one pipeline

**Steps**:
1. Create pipeline with 3 database enrichment steps:
   - Step 1: EMPI lookup (users table)
   - Step 2: Provider lookup (users table with role='provider')
   - Step 3: Facility lookup (users table)
2. Configure each with different:
   - SQL queries
   - Query parameters
   - Result mappings
   - Target paths: `enriched.empi`, `enriched.provider`, `enriched.facility`
3. Save all steps
4. Save pipeline
5. Reload page
6. Verify all 3 steps show correct configuration

**Expected Results**:
- ✅ All 3 steps independent
- ✅ Each step's configuration isolated
- ✅ No config leakage between steps
- ✅ All configurations persist correctly

**Pass Criteria**: Multiple database enrichment steps work independently

---

## 🔍 Browser Compatibility Testing

### Browsers to Test
- ✅ Chrome/Edge (Chromium-based)
- ✅ Firefox
- ✅ Safari (if available)

### Key Areas
1. Visual builders display correctly
2. Query tester styling (purple gradient)
3. Fullscreen mode works
4. Click events (Add Parameter, Add Mapping, Run Query)
5. F11 keyboard shortcut
6. ESC to close modal

---

## 📊 Test Results Summary Template

### Test Execution Date: _______________
### Tester: _______________
### Browser: _______________
### Environment: Docker Compose (Local)

| Test Suite | Test Case | Status | Notes |
|------------|-----------|--------|-------|
| Suite 1 | 1.1 Query Parameter Builder - Basic | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.2 Add Parameter | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.3 Multiple Parameters | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.4 Remove Parameter | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.5 Result Mapping - Basic | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.6 Add Mapping | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.7 CamelCase Conversion | ⬜ PASS / ⬜ FAIL | |
| Suite 1 | 1.8 Remove Mapping | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.1 Query Tester - Visibility | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.2 Simple Query | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.3 Click-to-Add Mapping | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.4 Multiple Mappings | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.5 Parameterized Query | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.6 Error - Invalid Query | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.7 Error - Connection Failed | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.8 Empty Result Set | ⬜ PASS / ⬜ FAIL | |
| Suite 2 | 2.9 Real-Time Config Updates | ⬜ PASS / ⬜ FAIL | |
| Suite 3 | 3.1 Documentation - Visibility | ⬜ PASS / ⬜ FAIL | |
| Suite 3 | 3.2 NO-CODE Features | ⬜ PASS / ⬜ FAIL | |
| Suite 3 | 3.3 Workflow Section | ⬜ PASS / ⬜ FAIL | |
| Suite 3 | 3.4 Troubleshooting Section | ⬜ PASS / ⬜ FAIL | |
| Suite 4 | 4.1 JSON Export | ⬜ PASS / ⬜ FAIL | |
| Suite 4 | 4.2 JSON Import | ⬜ PASS / ⬜ FAIL | |
| Suite 4 | 4.3 JSON Validate | ⬜ PASS / ⬜ FAIL | |
| Suite 4 | 4.4 Copy to Clipboard | ⬜ PASS / ⬜ FAIL | |
| Suite 5 | 5.1 Enter Fullscreen | ⬜ PASS / ⬜ FAIL | |
| Suite 5 | 5.2 Exit Fullscreen | ⬜ PASS / ⬜ FAIL | |
| Suite 5 | 5.3 F11 Keyboard Shortcut | ⬜ PASS / ⬜ FAIL | |
| Suite 5 | 5.4 ESC Key Behavior | ⬜ PASS / ⬜ FAIL | |
| Suite 6 | 6.1 E2E Complete Flow | ⬜ PASS / ⬜ FAIL | |
| Suite 6 | 6.2 Backend Compatibility | ⬜ PASS / ⬜ FAIL | |
| Suite 6 | 6.3 Multiple Steps | ⬜ PASS / ⬜ FAIL | |

### Overall Status:
- ⬜ ALL TESTS PASSED
- ⬜ SOME TESTS FAILED (see notes)
- ⬜ BLOCKED (environment issues)

### Critical Bugs Found:
1. _______________________________
2. _______________________________
3. _______________________________

### Recommendations:
1. _______________________________
2. _______________________________
3. _______________________________

---

## ✅ Sign-Off

**Tested By**: _______________
**Date**: _______________
**Signature**: _______________

**Ready for Production**: ⬜ YES / ⬜ NO

---

## 📚 Reference Documentation

- [DATABASE_ENRICHMENT_PHASE1_TEST_GUIDE.md](DATABASE_ENRICHMENT_PHASE1_TEST_GUIDE.md) - Phase 1 specific guide
- [DATABASE_ENRICHMENT_PHASE2_TEST_GUIDE.md](DATABASE_ENRICHMENT_PHASE2_TEST_GUIDE.md) - Phase 2 specific guide
- [DATABASE_ENRICHMENT_DOCUMENTATION_UPDATE.md](DATABASE_ENRICHMENT_DOCUMENTATION_UPDATE.md) - Documentation update details
- [DATABASE_ENRICHMENT_NO_CODE_IMPROVEMENTS.md](DATABASE_ENRICHMENT_NO_CODE_IMPROVEMENTS.md) - Original requirements

---

## 🎯 Success Criteria

Database Enrichment feature is **PRODUCTION READY** when:
- ✅ All 31 test cases pass
- ✅ Zero critical bugs
- ✅ Documentation tab complete and helpful
- ✅ Import JSON tab handles configuration correctly
- ✅ Fullscreen mode works across browsers
- ✅ Backend compatibility verified
- ✅ User can configure database enrichment without editing JSON
- ✅ Query tester enables testing before deployment
- ✅ Click-to-add mapping saves 80% of configuration time
