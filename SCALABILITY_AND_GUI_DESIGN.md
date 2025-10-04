# Scalability Analysis & No-Code GUI Design

## Executive Summary

**Questions Addressed**:
1. ✅ Can handle millions of messages/day with proper architecture
2. ✅ No-code drag-and-drop GUI design provided
3. ✅ Hierarchical layers + parallel execution supported
4. ✅ Migration path for existing HL7→FHIR mappings defined
5. ✅ MVC and OOB principles maintained

---

## 1. Scalability Analysis - Millions of Messages Per Day

### Current Architecture Assessment

**Bottlenecks Identified**:
- ❌ Synchronous step execution (sequential only)
- ❌ Single-threaded pipeline execution
- ❌ MongoDB write for every step (high I/O)
- ❌ No connection pooling limits
- ❌ No message queuing/buffering

**Target Performance**:
- **1 million messages/day** = ~12 messages/second
- **10 million messages/day** = ~116 messages/second
- **100 million messages/day** = ~1,157 messages/second

### Scalability Improvements Required

#### A. Async + Parallel Execution

**Current Design** (Sequential):
```
Step 10 → Step 20 → Step 30 → Step 100 → Step 200
(5 seconds total if each takes 1 second)
```

**Improved Design** (Parallel within layers):
```
Layer: Pre-Processing (Parallel)
├─ Step 10: Validation       │
├─ Step 20: Enrichment       │ Run in parallel
└─ Step 30: Custom Logic     │ (1 second total)
         ↓
Layer: Core Mapping (Single)
└─ Step 100: HL7→FHIR        (1 second)
         ↓
Layer: Post-Processing (Parallel)
├─ Step 200: FHIR Validation │
└─ Step 210: Anonymization   │ Run in parallel
                              (1 second total)

Total: 3 seconds (vs 5 seconds sequential)
```

**Implementation**:
```go
// services/transformation_pipeline_service.go

func (tps *TransformationPipelineService) ExecuteLayer(
    ctx context.Context,
    layerSteps []TransformationStep,
    inputData map[string]interface{},
) (*LayerResult, error) {

    // Check if steps can run in parallel
    if tps.canRunInParallel(layerSteps) {
        return tps.executeParallel(ctx, layerSteps, inputData)
    }

    // Sequential execution (for dependent steps)
    return tps.executeSequential(ctx, layerSteps, inputData)
}

func (tps *TransformationPipelineService) executeParallel(
    ctx context.Context,
    steps []TransformationStep,
    inputData map[string]interface{},
) (*LayerResult, error) {

    // Use worker pool pattern
    resultChan := make(chan *StepResult, len(steps))
    errorChan := make(chan error, len(steps))

    var wg sync.WaitGroup

    for _, step := range steps {
        wg.Add(1)
        go func(s TransformationStep) {
            defer wg.Done()

            result, err := tps.ExecuteStep(ctx, s, inputData)
            if err != nil {
                errorChan <- err
                return
            }

            resultChan <- result
        }(step)
    }

    // Wait for all goroutines
    wg.Wait()
    close(resultChan)
    close(errorChan)

    // Check for errors
    if len(errorChan) > 0 {
        return nil, <-errorChan
    }

    // Merge results
    mergedOutput := tps.mergeStepResults(resultChan, inputData)

    return &LayerResult{
        Success:    true,
        OutputData: mergedOutput,
    }, nil
}
```

**Database Schema Update** (Add to transformation_steps):
```sql
ALTER TABLE transformation_steps
ADD COLUMN execution_mode VARCHAR(20) DEFAULT 'sequential';
-- Values: 'sequential', 'parallel', 'conditional'

ADD COLUMN merge_strategy VARCHAR(20) DEFAULT 'deep_merge';
-- How to merge parallel results: 'deep_merge', 'shallow_merge', 'array_append'
```

#### B. Message Queue Integration

**Problem**: Spikes in message volume overwhelm system

**Solution**: Queue-based architecture

```
┌──────────────┐      ┌───────────────┐      ┌──────────────────┐
│   Messages   │──────▶│  Message Queue│──────▶│  Worker Pool     │
│   Arriving   │      │  (RabbitMQ/   │      │  (N Workers)     │
│              │      │   Redis)      │      │                  │
└──────────────┘      └───────────────┘      └──────────────────┘
                                                     │
                                                     ▼
                                            ┌──────────────────┐
                                            │ Transformation   │
                                            │ Pipeline         │
                                            └──────────────────┘
```

**Implementation**:
```go
// services/message_queue_service.go

type MessageQueueService struct {
    redisClient *redis.Client
    queueName   string
    workerCount int
}

func (mqs *MessageQueueService) EnqueueMessage(
    messageID string,
    interfaceID string,
    parsedJSON map[string]interface{},
) error {
    payload := map[string]interface{}{
        "message_id":   messageID,
        "interface_id": interfaceID,
        "parsed_json":  parsedJSON,
        "timestamp":    time.Now(),
    }

    jsonData, _ := json.Marshal(payload)

    // Add to Redis queue
    return mqs.redisClient.RPush(
        context.Background(),
        mqs.queueName,
        jsonData,
    ).Err()
}

func (mqs *MessageQueueService) StartWorkers(
    transformService *TransformationPipelineService,
) {
    for i := 0; i < mqs.workerCount; i++ {
        go mqs.worker(i, transformService)
    }
}

func (mqs *MessageQueueService) worker(
    workerID int,
    transformService *TransformationPipelineService,
) {
    for {
        // Pop message from queue (blocking)
        result, err := mqs.redisClient.BLPop(
            context.Background(),
            0, // Wait indefinitely
            mqs.queueName,
        ).Result()

        if err != nil {
            continue
        }

        var payload map[string]interface{}
        json.Unmarshal([]byte(result[1]), &payload)

        // Process transformation
        transformService.ExecuteTransformation(
            context.Background(),
            payload["message_id"].(string),
            payload["interface_id"].(string),
            payload["message_type"].(string),
            payload["parsed_json"].(map[string]interface{}),
        )
    }
}
```

**Environment Configuration**:
```bash
# .env
REDIS_HOST=redis
REDIS_PORT=6379
TRANSFORMATION_WORKERS=20  # Scale based on load
TRANSFORMATION_QUEUE=transformation_queue
```

#### C. Database Optimization

**Problem**: Too many MongoDB writes (one per step)

**Solution**: Batch writes at layer boundaries

```go
type LayerExecutionContext struct {
    IntermediateResults map[string]interface{}
    StepOutputs         []StepResult
    FinalOutput         map[string]interface{}
}

// Only write to MongoDB at layer completion
func (tps *TransformationPipelineService) executeLayer(...) {
    // Execute all steps in layer
    results := ...

    // Write once per layer, not per step
    tps.storeLayerCheckpoint(layerName, results)
}
```

**Connection Pooling**:
```go
// MongoDB connection pool
mongoClientOptions := options.Client().
    SetMaxPoolSize(100).          // Max connections
    SetMinPoolSize(10).           // Min connections
    SetMaxConnIdleTime(5 * time.Minute)

// PostgreSQL connection pool
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)
```

#### D. Caching Strategy

**Problem**: Repeated lookups for pipeline config, templates, external data

**Solution**: Multi-layer caching

```go
// services/cache_service.go

type CacheService struct {
    localCache  *ristretto.Cache  // In-memory (Ristretto)
    redisCache  *redis.Client     // Distributed (Redis)
    ttl         time.Duration
}

func (cs *CacheService) GetPipeline(
    interfaceID string,
    messageType string,
) (*TransformationPipeline, error) {

    cacheKey := fmt.Sprintf("pipeline:%s:%s", interfaceID, messageType)

    // L1: Check local cache (microseconds)
    if val, found := cs.localCache.Get(cacheKey); found {
        return val.(*TransformationPipeline), nil
    }

    // L2: Check Redis (milliseconds)
    if val, err := cs.redisCache.Get(ctx, cacheKey).Result(); err == nil {
        var pipeline TransformationPipeline
        json.Unmarshal([]byte(val), &pipeline)

        // Store in L1 for next time
        cs.localCache.Set(cacheKey, &pipeline, 1)

        return &pipeline, nil
    }

    // L3: Load from database (10-50ms)
    pipeline, err := cs.loadFromDatabase(interfaceID, messageType)

    // Cache in both layers
    cs.localCache.Set(cacheKey, pipeline, 1)
    cs.cacheInRedis(cacheKey, pipeline, cs.ttl)

    return pipeline, err
}
```

**Cache Invalidation**:
```sql
-- Trigger on pipeline update
CREATE TRIGGER tr_invalidate_pipeline_cache
AFTER UPDATE ON transformation_pipelines
FOR EACH ROW
EXECUTE FUNCTION invalidate_cache('pipeline', NEW.id);
```

#### E. Horizontal Scaling

**Architecture**:
```
                    ┌─────────────────┐
                    │  Load Balancer  │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼─────┐       ┌────▼─────┐       ┌────▼─────┐
    │ Worker 1 │       │ Worker 2 │       │ Worker N │
    │ (20 CPUs)│       │ (20 CPUs)│       │ (20 CPUs)│
    └────┬─────┘       └────┬─────┘       └────┬─────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Shared Redis   │
                    │  Message Queue  │
                    └─────────────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼─────┐       ┌────▼─────┐       ┌────▼─────┐
    │PostgreSQL│       │ MongoDB  │       │  Redis   │
    │(Primary) │       │ (Replica │       │ (Cache)  │
    │          │       │   Set)   │       │          │
    └──────────┘       └──────────┘       └──────────┘
```

**Docker Compose Scaling**:
```bash
# Scale workers
docker-compose up --scale transformation-worker=10

# Each worker pulls from same Redis queue
```

### Performance Projections

**With Optimizations**:

| Messages/Day | Messages/Second | Workers Needed | Estimated Cost |
|--------------|----------------|----------------|----------------|
| 1M           | 12/sec         | 2-5 workers    | $100/month     |
| 10M          | 116/sec        | 10-20 workers  | $500/month     |
| 100M         | 1,157/sec      | 50-100 workers | $2,000/month   |

**Assumptions**:
- Average transformation time: 100ms per message
- Parallel execution: 3x speedup (3 layers)
- Queue-based buffering: Handles spikes up to 10x average

**Bottleneck Analysis**:
- **1M/day**: No bottleneck, current design sufficient
- **10M/day**: Requires worker pool + caching
- **100M/day**: Requires horizontal scaling + database sharding

---

## 2. No-Code GUI Design - Drag & Drop Flow Builder

### Visual Flow Builder Design

```
┌────────────────────────────────────────────────────────────────────────┐
│  Transformation Pipeline Builder                    [Save] [Test] [×]  │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│  Interface: Hospital A          Message Type: ADT^A01                 │
│  Pipeline Name: Admission Processing                                  │
│                                                                        │
│  ┌──────────────┐                                                     │
│  │  TOOLBOX     │                                                     │
│  ├──────────────┤                                                     │
│  │ 📋 Templates │                                                     │
│  │ ├─ Validate  │                                                     │
│  │ ├─ Enrich    │                                                     │
│  │ ├─ Transform │                                                     │
│  │ └─ Custom    │                                                     │
│  │              │                                                     │
│  │ 🔧 Logic     │                                                     │
│  │ ├─ If/Then   │                                                     │
│  │ ├─ Loop      │                                                     │
│  │ ├─ Merge     │                                                     │
│  │ └─ Split     │                                                     │
│  │              │                                                     │
│  │ 🔌 Connectors│                                                     │
│  │ ├─ HTTP API  │                                                     │
│  │ ├─ Database  │                                                     │
│  │ └─ File      │                                                     │
│  └──────────────┘                                                     │
│                                                                        │
│       CANVAS (Drag & Drop Here)                                       │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                                                              │   │
│  │  ┌──────────────┐                                            │   │
│  │  │   START      │                                            │   │
│  │  │   (ADT^A01)  │                                            │   │
│  │  └──────┬───────┘                                            │   │
│  │         │                                                    │   │
│  │         ▼                                                    │   │
│  │  ┌────────────────────────────────────────────┐             │   │
│  │  │  LAYER: Pre-Processing                     │             │   │
│  │  │  Execution: Parallel                       │             │   │
│  │  ├────────────────────────────────────────────┤             │   │
│  │  │                                            │             │   │
│  │  │  ┌──────────┐  ┌──────────┐  ┌─────────┐ │             │   │
│  │  │  │Validate  │  │ Enrich   │  │ Mark    │ │             │   │
│  │  │  │Patient ID│  │from Epic │  │VIP      │ │             │   │
│  │  │  │          │  │          │  │Patients │ │             │   │
│  │  │  │Required  │  │Optional  │  │Custom JS│ │             │   │
│  │  │  └────┬─────┘  └────┬─────┘  └────┬────┘ │             │   │
│  │  │       └─────────────┼─────────────┘      │             │   │
│  │  │                     │ (Merge)            │             │   │
│  │  └─────────────────────┼────────────────────┘             │   │
│  │                        │                                   │   │
│  │                        ▼                                   │   │
│  │  ┌────────────────────────────────────────────┐           │   │
│  │  │  LAYER: Core Mapping                       │           │   │
│  │  │  Execution: Sequential                     │           │   │
│  │  ├────────────────────────────────────────────┤           │   │
│  │  │                                            │           │   │
│  │  │       ┌──────────────────────┐             │           │   │
│  │  │       │   HL7 → FHIR         │             │           │   │
│  │  │       │   Template: ADT^A01  │             │           │   │
│  │  │       │   Required           │             │           │   │
│  │  │       └──────────┬───────────┘             │           │   │
│  │  │                  │                         │           │   │
│  │  └──────────────────┼─────────────────────────┘           │   │
│  │                     │                                      │   │
│  │                     ▼                                      │   │
│  │  ┌────────────────────────────────────────────┐           │   │
│  │  │  LAYER: Post-Processing                    │           │   │
│  │  │  Execution: Parallel                       │           │   │
│  │  ├────────────────────────────────────────────┤           │   │
│  │  │                                            │           │   │
│  │  │  ┌──────────┐           ┌──────────────┐  │           │   │
│  │  │  │ Validate │           │ Anonymize    │  │           │   │
│  │  │  │ FHIR     │           │ Test         │  │           │   │
│  │  │  │ Bundle   │           │ Patients     │  │           │   │
│  │  │  │ Required │           │ Custom JS    │  │           │   │
│  │  │  └────┬─────┘           └──────┬───────┘  │           │   │
│  │  │       └────────────────────────┘          │           │   │
│  │  │                  │ (Merge)                │           │   │
│  │  └──────────────────┼────────────────────────┘           │   │
│  │                     │                                     │   │
│  │                     ▼                                     │   │
│  │              ┌──────────────┐                             │   │
│  │              │    OUTPUT    │                             │   │
│  │              │ (FHIR Bundle)│                             │   │
│  │              └──────────────┘                             │   │
│  │                                                           │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  Properties Panel (Selected: "Validate Patient ID")              │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │ Step Name: Validate Patient ID                           │   │
│  │ Step Type: Validation                                    │   │
│  │ Required:  [✓] Yes  [ ] No                               │   │
│  │ Timeout:   [5000] ms                                     │   │
│  │                                                          │   │
│  │ Validation Rules:                                        │   │
│  │ ┌────────────────────────────────────────────────┐      │   │
│  │ │ Field: PID.3 (Patient ID)                      │      │   │
│  │ │ Rule:  Required                                │      │   │
│  │ │ Min Length: 5                                  │      │   │
│  │ │ Pattern: ^[A-Z0-9]+$                           │      │   │
│  │ └────────────────────────────────────────────────┘      │   │
│  │                                                          │   │
│  │ [+ Add Rule]                                             │   │
│  │                                                          │   │
│  │ On Error:                                                │   │
│  │ ( ) Continue with default  ( ) Skip  (•) Fail pipeline  │   │
│  │                                                          │   │
│  │ [Apply]  [Cancel]                                        │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────┘
```

### GUI Components

#### A. Layer Container
```typescript
// frontend/components/LayerContainer.tsx

interface Layer {
    id: string;
    name: string;
    type: 'pre' | 'core' | 'post';
    execution_mode: 'sequential' | 'parallel';
    steps: Step[];
    merge_strategy?: 'deep_merge' | 'shallow_merge';
}

const LayerContainer: React.FC<LayerProps> = ({ layer, onDrop, onUpdate }) => {
    const [steps, setSteps] = useState(layer.steps);

    const handleDrop = (e: DragEvent) => {
        // Add step from toolbox to layer
        const stepTemplate = JSON.parse(e.dataTransfer.getData('stepTemplate'));

        const newStep = {
            ...stepTemplate,
            id: uuidv4(),
            sequence: steps.length * 10,
        };

        setSteps([...steps, newStep]);
    };

    return (
        <div
            className="layer-container"
            onDrop={handleDrop}
            onDragOver={(e) => e.preventDefault()}
        >
            <div className="layer-header">
                <h3>{layer.name}</h3>
                <select
                    value={layer.execution_mode}
                    onChange={(e) => onUpdate({ execution_mode: e.target.value })}
                >
                    <option value="sequential">Sequential</option>
                    <option value="parallel">Parallel</option>
                </select>
            </div>

            <div className={`steps-container ${layer.execution_mode}`}>
                {steps.map(step => (
                    <StepNode
                        key={step.id}
                        step={step}
                        parallelMode={layer.execution_mode === 'parallel'}
                    />
                ))}
            </div>
        </div>
    );
};
```

#### B. Step Node
```typescript
// frontend/components/StepNode.tsx

interface Step {
    id: string;
    name: string;
    type: string;
    required: boolean;
    config: any;
}

const StepNode: React.FC<StepProps> = ({ step, parallelMode, onClick }) => {
    return (
        <div
            className={`step-node ${step.required ? 'required' : 'optional'}`}
            onClick={() => onClick(step)}
            draggable
        >
            <div className="step-icon">
                {getIconForType(step.type)}
            </div>
            <div className="step-content">
                <div className="step-name">{step.name}</div>
                <div className="step-type">{step.type}</div>
            </div>
            {step.required && <div className="required-badge">Required</div>}
        </div>
    );
};
```

#### C. Template Toolbox
```typescript
// frontend/components/TemplateToolbox.tsx

const templates = [
    {
        id: 'validate-patient-id',
        name: 'Validate Patient ID',
        type: 'pre.validation',
        icon: '✓',
        defaultConfig: {
            rules: [
                { field: 'PID.3', required: true, minLength: 5 }
            ]
        }
    },
    {
        id: 'enrich-epic',
        name: 'Enrich from Epic',
        type: 'pre.enrichment',
        icon: '🔍',
        defaultConfig: {
            apiEndpoint: '',
            apiKey: '',
            timeout: 2000
        }
    },
    {
        id: 'hl7-fhir-mapping',
        name: 'HL7 → FHIR Mapping',
        type: 'core.mapping',
        icon: '🔄',
        defaultConfig: {
            templateId: '',
            strictMode: false
        }
    }
];

const TemplateToolbox: React.FC = () => {
    const handleDragStart = (template: Template, e: DragEvent) => {
        e.dataTransfer.setData('stepTemplate', JSON.stringify(template));
    };

    return (
        <div className="template-toolbox">
            <h3>Templates</h3>
            {templates.map(template => (
                <div
                    key={template.id}
                    className="template-item"
                    draggable
                    onDragStart={(e) => handleDragStart(template, e)}
                >
                    <span className="icon">{template.icon}</span>
                    <span className="name">{template.name}</span>
                </div>
            ))}

            <button onClick={() => setShowCustomScriptModal(true)}>
                + Custom Script
            </button>
        </div>
    );
};
```

#### D. Properties Panel
```typescript
// frontend/components/PropertiesPanel.tsx

const PropertiesPanel: React.FC<{ step: Step, onSave }> = ({ step, onSave }) => {
    const [config, setConfig] = useState(step.config);

    const renderConfigEditor = () => {
        switch (step.type) {
            case 'pre.validation':
                return <ValidationConfigEditor config={config} onChange={setConfig} />;
            case 'pre.enrichment':
                return <EnrichmentConfigEditor config={config} onChange={setConfig} />;
            case 'pre.custom':
                return <CustomScriptEditor config={config} onChange={setConfig} />;
            case 'core.mapping':
                return <MappingConfigEditor config={config} onChange={setConfig} />;
            default:
                return <JSONEditor value={config} onChange={setConfig} />;
        }
    };

    return (
        <div className="properties-panel">
            <h3>Properties: {step.name}</h3>

            <div className="property-group">
                <label>Step Name</label>
                <input
                    type="text"
                    value={step.name}
                    onChange={(e) => setStep({...step, name: e.target.value})}
                />
            </div>

            <div className="property-group">
                <label>Required</label>
                <input
                    type="checkbox"
                    checked={step.required}
                    onChange={(e) => setStep({...step, required: e.target.checked})}
                />
            </div>

            <div className="property-group">
                <label>Timeout (ms)</label>
                <input
                    type="number"
                    value={step.config.timeout || 5000}
                    onChange={(e) => setConfig({...config, timeout: parseInt(e.target.value)})}
                />
            </div>

            <div className="config-editor">
                {renderConfigEditor()}
            </div>

            <div className="error-handling">
                <h4>On Error</h4>
                <select
                    value={step.on_error_strategy}
                    onChange={(e) => setStep({...step, on_error_strategy: e.target.value})}
                >
                    <option value="fail">Fail Pipeline</option>
                    <option value="skip">Skip & Continue</option>
                    <option value="default">Use Default Value</option>
                </select>
            </div>

            <button onClick={() => onSave(step, config)}>Apply</button>
        </div>
    );
};
```

#### E. Custom Script Editor
```typescript
// frontend/components/CustomScriptEditor.tsx

const CustomScriptEditor: React.FC = ({ config, onChange }) => {
    const [code, setCode] = useState(config.script || SCRIPT_TEMPLATE);

    const SCRIPT_TEMPLATE = `
function transform(input) {
    // Access parsed HL7 data
    var pid = input.enhancedSegments.PID;
    var patientName = pid.fields.find(f => f.key === "PID.5");

    // Your custom logic here
    // ...

    return input;
}
    `.trim();

    return (
        <div className="script-editor">
            <h4>Custom JavaScript</h4>

            <div className="editor-toolbar">
                <button onClick={() => setCode(SCRIPT_TEMPLATE)}>Reset Template</button>
                <button onClick={validateScript}>Validate</button>
                <button onClick={testScript}>Test</button>
            </div>

            <MonacoEditor
                language="javascript"
                value={code}
                onChange={setCode}
                options={{
                    minimap: { enabled: false },
                    lineNumbers: 'on',
                    theme: 'vs-dark',
                }}
            />

            <div className="script-help">
                <h5>Available Objects:</h5>
                <ul>
                    <li><code>input.enhancedSegments</code> - Parsed HL7 segments</li>
                    <li><code>input.messageType</code> - Message type info</li>
                    <li><code>input._metadata</code> - Custom metadata</li>
                </ul>

                <h5>Helper Functions:</h5>
                <ul>
                    <li><code>getField(segment, key)</code> - Extract field value</li>
                    <li><code>setMetadata(key, value)</code> - Add metadata</li>
                    <li><code>callAPI(url, params)</code> - HTTP request</li>
                </ul>
            </div>
        </div>
    );
};
```

### GUI Data Model

**Frontend State**:
```typescript
interface PipelineBuilder {
    pipeline: {
        id: string;
        interface_id: string;
        message_type: string;
        pipeline_name: string;
        version: number;
    };
    layers: Layer[];
    selectedStep?: Step;
    isDirty: boolean;
}

interface Layer {
    id: string;
    name: string;
    type: 'pre' | 'core' | 'post';
    execution_mode: 'sequential' | 'parallel';
    merge_strategy: 'deep_merge' | 'shallow_merge';
    steps: Step[];
}

interface Step {
    id: string;
    name: string;
    step_type: string;
    sequence: number;
    required: boolean;
    timeout_ms: number;
    config: any;
    script_type?: 'javascript' | 'lua';
    script_content?: string;
    on_error_strategy: 'fail' | 'skip' | 'default';
    depends_on_steps?: string[];
}
```

**Save to Database**:
```typescript
const savePipeline = async (builder: PipelineBuilder) => {
    // 1. Save/Update pipeline
    const pipelineResponse = await api.post('/api/transformation/pipelines', {
        ...builder.pipeline,
    });

    const pipelineId = pipelineResponse.data.id;

    // 2. Delete old steps
    await api.delete(`/api/transformation/pipelines/${pipelineId}/steps`);

    // 3. Save new steps
    let sequence = 0;
    for (const layer of builder.layers) {
        for (const step of layer.steps) {
            await api.post('/api/transformation/steps', {
                pipeline_id: pipelineId,
                ...step,
                sequence: sequence++,
                layer: layer.type,
                execution_mode: layer.execution_mode,
            });
        }
    }

    return pipelineId;
};
```

---

## 3. Hierarchical Layers + Parallel Execution

### Database Schema Enhancement

```sql
-- Add to transformation_steps table
ALTER TABLE transformation_steps
ADD COLUMN layer VARCHAR(20) NOT NULL DEFAULT 'core';
-- Values: 'pre', 'core', 'post'

ADD COLUMN execution_mode VARCHAR(20) DEFAULT 'sequential';
-- Values: 'sequential', 'parallel'

ADD COLUMN merge_strategy VARCHAR(20) DEFAULT 'deep_merge';
-- How to combine results from parallel steps
-- Values: 'deep_merge', 'shallow_merge', 'first_wins', 'last_wins'

ADD COLUMN parallel_group INTEGER;
-- Steps with same parallel_group run in parallel within a layer
-- NULL means sequential

-- Example data:
-- Layer: pre, parallel_group: 1 → All run in parallel
-- Layer: pre, parallel_group: 2 → All run in parallel (after group 1)
-- Layer: core, parallel_group: NULL → Sequential
```

### Execution Engine Update

```go
// services/transformation_pipeline_service.go

func (tps *TransformationPipelineService) ExecuteTransformation(
    ctx context.Context,
    messageID string,
    interfaceID string,
    messageType string,
    parsedJSON map[string]interface{},
) (*TransformationResult, error) {

    // Get pipeline
    pipeline, err := tps.GetPipeline(interfaceID, messageType)

    // Group steps by layer and parallel groups
    layerGroups := tps.groupStepsByLayerAndParallelism(pipeline.Steps)

    currentData := parsedJSON

    // Execute layers in order: pre → core → post
    for _, layer := range []string{"pre", "core", "post"} {
        groups := layerGroups[layer]

        for _, group := range groups {
            if group.ExecutionMode == "parallel" {
                // Execute all steps in group in parallel
                result, err := tps.executeParallelGroup(ctx, group.Steps, currentData)
                if err != nil {
                    return nil, err
                }
                currentData = result.OutputData
            } else {
                // Execute steps sequentially
                for _, step := range group.Steps {
                    result, err := tps.ExecuteStep(ctx, step, currentData)
                    if err != nil && step.Required {
                        return nil, err
                    }
                    currentData = result.OutputData
                }
            }
        }
    }

    return &TransformationResult{
        Success:    true,
        OutputData: currentData,
    }, nil
}

func (tps *TransformationPipelineService) groupStepsByLayerAndParallelism(
    steps []TransformationStep,
) map[string][]StepGroup {

    grouped := make(map[string][]StepGroup)

    for _, layer := range []string{"pre", "core", "post"} {
        layerSteps := filterByLayer(steps, layer)

        // Group by parallel_group
        parallelGroups := make(map[int][]TransformationStep)
        sequentialSteps := []TransformationStep{}

        for _, step := range layerSteps {
            if step.ParallelGroup != nil {
                parallelGroups[*step.ParallelGroup] = append(
                    parallelGroups[*step.ParallelGroup],
                    step,
                )
            } else {
                sequentialSteps = append(sequentialSteps, step)
            }
        }

        // Create step groups
        groups := []StepGroup{}

        // Add parallel groups
        for _, steps := range parallelGroups {
            groups = append(groups, StepGroup{
                ExecutionMode: "parallel",
                Steps:         steps,
            })
        }

        // Add sequential steps
        if len(sequentialSteps) > 0 {
            groups = append(groups, StepGroup{
                ExecutionMode: "sequential",
                Steps:         sequentialSteps,
            })
        }

        grouped[layer] = groups
    }

    return grouped
}
```

### Visual Representation in GUI

```
┌────────────────────────────────────────────────────┐
│  LAYER: Pre-Processing  [Sequential/Parallel ▼]    │
├────────────────────────────────────────────────────┤
│                                                    │
│  Parallel Group 1:                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐        │
│  │Validate  │  │ Validate │  │ Check    │        │
│  │Patient ID│  │ Date     │  │ Duplicate│        │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘        │
│       └─────────────┼─────────────┘              │
│                     │ (Deep Merge)               │
│                     ▼                            │
│  Sequential:                                      │
│       ┌─────────────────┐                         │
│       │ Enrich from     │                         │
│       │ Epic API        │                         │
│       │ (depends on     │                         │
│       │  validation)    │                         │
│       └────────┬────────┘                         │
│                │                                  │
│                ▼                                  │
│  Parallel Group 2:                                │
│  ┌──────────┐           ┌──────────┐             │
│  │ Mark VIP │           │ Calculate│             │
│  │ Patients │           │ Risk     │             │
│  └────┬─────┘           └────┬─────┘             │
│       └──────────────────────┘                   │
│                │ (Shallow Merge)                  │
└────────────────┼──────────────────────────────────┘
                 │
                 ▼
```

---

## 4. Migration of Existing HL7→FHIR Mappings

### Migration Strategy

**Current Tables**:
- `hl7_fhir_templates` - Standard mapping templates
- `interface_message_mappings` - Per-interface customizations

**Migration Plan**:

#### Step 1: Auto-Create Transformation Pipelines

```sql
-- For each interface + message type combination, create pipeline
INSERT INTO transformation_pipelines (
    interface_id,
    message_type,
    pipeline_name,
    enabled,
    created_at
)
SELECT DISTINCT
    imm.interface_id,
    imm.message_type,
    CONCAT(i.name, ' - ', imm.message_type, ' Pipeline'),
    true,
    CURRENT_TIMESTAMP
FROM interface_message_mappings imm
JOIN interfaces i ON i.id = imm.interface_id;
```

#### Step 2: Create Core Mapping Steps

```sql
-- For each pipeline, create core mapping step
INSERT INTO transformation_steps (
    pipeline_id,
    step_name,
    step_type,
    sequence,
    layer,
    required,
    enabled,
    config
)
SELECT
    tp.id as pipeline_id,
    'HL7 to FHIR Mapping' as step_name,
    'core.mapping' as step_type,
    100 as sequence,
    'core' as layer,
    true as required,
    true as enabled,
    CASE
        WHEN imm.uses_standard_template THEN
            jsonb_build_object(
                'mapping_type', 'template',
                'template_id', imm.standard_template_id,
                'template_name', hft.template_name
            )
        ELSE
            jsonb_build_object(
                'mapping_type', 'custom',
                'custom_config', imm.custom_mapping_config
            )
    END as config
FROM transformation_pipelines tp
JOIN interface_message_mappings imm
    ON imm.interface_id = tp.interface_id
    AND imm.message_type = tp.message_type
LEFT JOIN hl7_fhir_templates hft
    ON hft.id = imm.standard_template_id;
```

#### Step 3: Add Default Pre/Post Steps

```sql
-- Add default validation step (pre-processing)
INSERT INTO transformation_steps (
    pipeline_id,
    step_name,
    step_type,
    sequence,
    layer,
    required,
    config
)
SELECT
    id as pipeline_id,
    'Validate Required Fields' as step_name,
    'pre.validation' as step_type,
    10 as sequence,
    'pre' as layer,
    false as required,
    jsonb_build_object(
        'rules', jsonb_build_array(
            jsonb_build_object('field', 'MSH.9', 'required', true),
            jsonb_build_object('field', 'PID.3', 'required', true)
        )
    ) as config
FROM transformation_pipelines;

-- Add default FHIR validation step (post-processing)
INSERT INTO transformation_steps (
    pipeline_id,
    step_name,
    step_type,
    sequence,
    layer,
    required,
    config
)
SELECT
    id as pipeline_id,
    'Validate FHIR Bundle' as step_name,
    'post.validation' as step_type,
    200 as sequence,
    'post' as layer,
    true as required,
    jsonb_build_object(
        'fhir_version', 'R4',
        'strict_mode', false
    ) as config
FROM transformation_pipelines;
```

#### Step 4: Migration Script

```bash
#!/bin/bash
# migrations/migrate_existing_mappings.sh

echo "🔄 Migrating existing HL7→FHIR mappings to transformation pipeline..."

docker-compose exec postgres psql -U ezhealth_user -d ezhealthkonnect << 'EOF'

BEGIN;

-- Step 1: Create pipelines
INSERT INTO transformation_pipelines (...)
...

-- Step 2: Create core mapping steps
INSERT INTO transformation_steps (...)
...

-- Step 3: Add default steps
INSERT INTO transformation_steps (...)
...

-- Verify migration
SELECT
    COUNT(*) as total_pipelines,
    (SELECT COUNT(*) FROM transformation_steps WHERE step_type = 'core.mapping') as core_mappings,
    (SELECT COUNT(*) FROM transformation_steps WHERE layer = 'pre') as pre_steps,
    (SELECT COUNT(*) FROM transformation_steps WHERE layer = 'post') as post_steps
FROM transformation_pipelines;

COMMIT;

EOF

echo "✅ Migration complete!"
```

### Backward Compatibility

**Keep old tables for now**:
```sql
-- DON'T drop old tables yet
-- Keep for backward compatibility and rollback

-- Add migration status tracking
ALTER TABLE interface_message_mappings
ADD COLUMN migrated_to_pipeline UUID REFERENCES transformation_pipelines(id),
ADD COLUMN migration_date TIMESTAMP WITH TIME ZONE;

-- Mark migrated mappings
UPDATE interface_message_mappings imm
SET
    migrated_to_pipeline = tp.id,
    migration_date = CURRENT_TIMESTAMP
FROM transformation_pipelines tp
WHERE tp.interface_id = imm.interface_id
  AND tp.message_type = imm.message_type;
```

---

## 5. MVC and OOB Compliance

### ✅ MVC Pattern Adherence

**Models** (Data structures):
```
models/transformation_models.go
├─ TransformationPipeline
├─ TransformationStep
├─ LayerExecutionContext
├─ StepResult
└─ TransformationResult
```

**Services** (Business logic):
```
services/transformation_pipeline_service.go    # Main orchestrator
services/transformation_executors.go           # Step executors
services/transformation_template_service.go    # Template management
services/cache_service.go                      # Caching layer
services/message_queue_service.go              # Queue management
```

**Controllers** (Orchestration only):
```
processing/engine.go
├─ executeTransformationPipeline() → Calls service
└─ No business logic, just delegation

controllers/transformation_pipeline_controller.go
├─ CreatePipeline() → Calls service
├─ UpdatePipeline() → Calls service
└─ API endpoints only, no business logic
```

**✅ Separation maintained**: No business logic in controllers

### ✅ OOB Pattern Adherence

**Auto-Configuration**:
```go
// Auto-detect environment and configure
func NewTransformationPipelineService(db *sql.DB) *TransformationPipelineService {
    // Auto-configure cache
    var cacheService *CacheService
    if os.Getenv("REDIS_HOST") != "" {
        cacheService = NewRedisCacheService() // Auto-create
    } else {
        cacheService = NewInMemoryCacheService() // Fallback
    }

    // Auto-configure message queue
    var queueService *MessageQueueService
    if os.Getenv("RABBITMQ_URL") != "" {
        queueService = NewRabbitMQService()
    } else if os.Getenv("REDIS_HOST") != "" {
        queueService = NewRedisQueueService()
    } else {
        queueService = NewInMemoryQueueService()
    }

    // Auto-register executors
    executors := make(map[TransformationType]TransformationExecutor)
    executors[TransformTypeValidation] = NewValidationExecutor()
    executors[TransformTypeEnrichment] = NewEnrichmentExecutor()
    executors[TransformTypeCoreMapping] = NewCoreMappingExecutor(db)
    executors[TransformTypeCustomScript] = NewCustomScriptExecutor()

    return &TransformationPipelineService{
        db:           db,
        cache:        cacheService,
        queue:        queueService,
        executors:    executors,
        workerCount:  getWorkerCount(), // From env or CPU count
    }
}
```

**Zero Manual Configuration**:
```bash
# All configuration from environment
REDIS_HOST=redis                    # Enable Redis cache + queue
TRANSFORMATION_WORKERS=20           # Worker pool size
TRANSFORMATION_QUEUE=transform_q    # Queue name
ENABLE_PARALLEL_EXECUTION=true     # Enable parallel steps
MAX_PARALLEL_STEPS=10              # Max concurrent steps
```

**✅ No hardcoding**: Everything auto-detected or env-configured

---

## Summary

### 1. ✅ Scalability - Millions of Messages/Day

**Supported with enhancements**:
- Queue-based architecture (Redis/RabbitMQ)
- Worker pool pattern (horizontal scaling)
- Parallel execution within layers
- Multi-layer caching (in-memory + Redis)
- Connection pooling
- Database optimization

**Performance**: 1M-100M messages/day achievable

### 2. ✅ No-Code GUI Design

**Features**:
- Drag-and-drop flow builder
- Visual layer management
- Pre-built template toolbox
- Custom script editor with syntax highlighting
- Properties panel for configuration
- Test runner

**User Experience**: Zero coding required for standard flows

### 3. ✅ Hierarchical Layers + Parallel Execution

**Structure**:
- 3 layers: pre → core → post
- Within each layer: sequential or parallel groups
- Merge strategies for parallel results
- Dependency support

**GUI automatically manages**: Sequence generation from visual flow

### 4. ✅ Migration Path

**Existing mappings migrate automatically**:
- `hl7_fhir_templates` → Core mapping steps
- `interface_message_mappings` → Pipeline configuration
- Backward compatible (old tables kept)
- One-time migration script

### 5. ✅ MVC & OOB Compliance

**MVC**: Clean separation (Models/Services/Controllers)
**OOB**: Zero manual config, auto-detection, env-based configuration

---

## Next Steps

1. **Review scalability requirements** - Confirm target volume
2. **Approve GUI design** - Mockups and UX flow
3. **Decide on queue technology** - Redis vs RabbitMQ vs Kafka
4. **Create V20 migration** - 5 tables + enhancements
5. **Build transformation service** - Core engine
6. **Implement GUI** - React-based flow builder
7. **Run migration** - Convert existing mappings

**Timeline**: 8-10 weeks (with GUI)
