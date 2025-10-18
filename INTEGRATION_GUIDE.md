# Pipeline Builder Integration Guide

## Step 1: Add to main.go

Add the following code after line 82 (after Processing Engine initialization):

```go
// Initialize Hybrid Execution Engine (Pipeline Builder)
var hybridExecutionEngine *services.HybridExecutionEngine
var pipelineManagementService *services.PipelineManagementService

if db != nil {
    // Create hybrid execution engine
    hybridExecutionEngine = services.NewHybridExecutionEngine(db)
    log.Printf("✅ Hybrid Execution Engine initialized")

    // Create pipeline management service
    pipelineManagementService = services.NewPipelineManagementService(db)
    log.Printf("✅ Pipeline Management Service initialized")
}
```

## Step 2: Register Pipeline Routes in main.go

Add the following code after line 327 (in the API routes section, after FHIR routes):

```go
// PIPELINE BUILDER ROUTES (NEW)
if hybridExecutionEngine != nil && pipelineManagementService != nil {
    // Pipeline execution controller
    pipelineExecCtrl := controllers.NewPipelineExecutionController(hybridExecutionEngine)
    pipelineExecCtrl.RegisterRoutes(api)

    // Pipeline management controller
    pipelineMgmtCtrl := controllers.NewPipelineManagementController(pipelineManagementService)
    pipelineMgmtCtrl.RegisterRoutes(api)

    log.Printf("✅ Pipeline Builder routes registered")
}
```

## Step 3: Register Node.js Routes in app.js

Add to app.js:

```javascript
// Pipeline Builder Routes (after line with existing routes)
const pipelineRoutes = require('./routes/pipelineRoutes');
app.use('/api', pipelineRoutes);
```

## Step 4: Run Database Migration

```bash
# Using Docker
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect < database/migrations/V21__Add_Execution_Groups_And_Dependencies.sql

# Or using Flyway (automatically runs on container restart)
docker-compose restart flyway
```

## Step 5: Test the API

### Test 1: Save a Pipeline

```bash
curl -X POST http://localhost:8080/api/pipelines \
  -H "Content-Type: application/json" \
  -d '{
    "id": "",
    "interfaceId": "your-interface-id",
    "messageType": "ADT^A01",
    "name": "Test Pipeline",
    "layers": {
      "pre": {
        "mode": "parallel",
        "groups": [
          {
            "id": "parallel_1",
            "type": "parallel",
            "steps": [
              {
                "id": "",
                "name": "Validate Patient ID",
                "type": "pre.validation",
                "config": {
                  "rules": [
                    {"field": "PID.3", "required": true}
                  ]
                },
                "required": true,
                "onErrorStrategy": "fail"
              }
            ],
            "dependsOn": []
          }
        ]
      },
      "core": {
        "mode": "waterfall",
        "groups": [
          {
            "id": "core_1",
            "type": "inline",
            "steps": [
              {
                "id": "",
                "name": "HL7 to FHIR",
                "type": "core.mapping",
                "config": {},
                "required": true,
                "onErrorStrategy": "fail"
              }
            ],
            "dependsOn": ["parallel_1"]
          }
        ]
      },
      "post": {
        "mode": "parallel",
        "groups": []
      }
    }
  }'
```

### Test 2: Load a Pipeline

```bash
curl http://localhost:8080/api/pipelines/{pipeline_id}
```

### Test 3: Test Pipeline Execution

```bash
curl -X POST http://localhost:8080/api/pipelines/test \
  -H "Content-Type: application/json" \
  -d '{
    "pipeline_id": "your-pipeline-id",
    "input_data": {
      "enhancedSegments": {
        "MSH": {...},
        "PID": {...}
      },
      "messageType": "ADT^A01"
    }
  }'
```

### Test 4: Get Pipeline Stats

```bash
curl http://localhost:8080/api/pipelines/{pipeline_id}/stats
```

## Step 6: Test Node.js Layer

```bash
# Through Node.js proxy (port 3000)
curl -X POST http://localhost:3000/api/pipelines \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -d '{...same payload as above...}'
```

## Available Endpoints

### Pipeline Management (Go Backend)
- POST   `/api/pipelines` - Save pipeline
- GET    `/api/pipelines/:id` - Load pipeline
- GET    `/api/pipelines/interface/:interfaceId/:messageType` - Load by interface
- GET    `/api/pipelines/interface/:interfaceId` - List pipelines
- DELETE `/api/pipelines/:id` - Delete pipeline
- POST   `/api/pipelines/:id/clone` - Clone pipeline

### Pipeline Execution (Go Backend)
- POST `/api/pipelines/execute` - Execute pipeline (production)
- POST `/api/pipelines/test` - Test pipeline
- GET  `/api/pipelines/:id/stats` - Get statistics

### Step Execution (Go Backend)
- POST `/api/processing/execute/validation` - Execute validation step
- POST `/api/processing/execute/enrichment` - Execute enrichment step
- POST `/api/processing/execute/mapping` - Execute mapping step
- POST `/api/processing/execute/custom` - Execute custom JavaScript

### Template Library (Node.js)
- GET  `/api/templates` - List all templates
- GET  `/api/templates/:id` - Get template
- POST `/api/templates` - Create custom template

## Frontend Integration (Next Phase)

The frontend will use these endpoints through the Node.js proxy layer (port 3000).

Files needed for frontend:
- `public/pipeline-builder.html` - Main UI
- `public/css/pipeline-builder.css` - Styles
- `public/js/pipeline/PipelineBuilder.js` - Main component
- `public/js/pipeline/DragDropManager.js` - Drag-drop logic
- `public/js/pipeline/ExecutionEngine.js` - Client-side execution
- `public/js/pipeline/CanvasRenderer.js` - Visual rendering

## Verification Checklist

- [ ] V21 migration applied successfully
- [ ] Go services initialized without errors
- [ ] API endpoints accessible
- [ ] Node.js proxy routes working
- [ ] Pipeline save/load working
- [ ] Pipeline execution working
- [ ] Templates loading correctly
- [ ] Frontend UI renders (next phase)

## Troubleshooting

### Error: "table execution_groups does not exist"
**Solution**: Run V21 migration:
```bash
docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect < database/migrations/V21__Add_Execution_Groups_And_Dependencies.sql
```

### Error: "Hybrid Execution Engine not initialized"
**Solution**: Ensure database connection is working in main.go

### Error: "Cannot proxy to Go backend"
**Solution**: Verify GO_BACKEND_URL in Node.js environment (.env):
```bash
GO_BACKEND_URL=http://localhost:8080
```

### Error: "Circular dependency detected"
**Solution**: Check step dependencies - ensure no loops in execution graph

## Next Steps

1. ✅ Phase 1A Complete: Database & Backend Services
2. ✅ Phase 1B Complete: API Layer & Integration
3. ⏳ Phase 1C: Frontend Drag-Drop UI (Next)
4. ⏳ Phase 1D: Testing & Polish
5. ⏳ Phase 2: Multi-Format Support
