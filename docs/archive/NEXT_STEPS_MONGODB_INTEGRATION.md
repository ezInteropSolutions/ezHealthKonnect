# MongoDB + Parsing + Transformation Integration - Next Steps

## Current Status (2025-10-18)

### ✅ What Works NOW
1. **Message Reception**: TCP/MLLP listener on port 6661
2. **ACK Sending**: Immediate ACK after receiving message
3. **PostgreSQL Storage**: Message stored in `messages_intf_<interface_id>` table
4. **Interface Activation**: Via API `POST /api/processing/interfaces/:id/activate`

### ❌ What's Missing
1. **MongoDB Storage**: Raw message not stored in MongoDB collection
2. **HL7 Parsing**: Message not parsed to JSON with enhanced schema
3. **Transformation**: HL7→FHIR transformation not triggered

---

## The Issue

The current `processing/engine.go` line 186 calls:
```go
go pe.processMessages(interfaceID, messageChan)
```

But the `processMessages` function in `processing/engine_message_processor.go` ONLY does PostgreSQL storage (lines 42-73). It does NOT:
- Store to MongoDB
- Parse HL7 to JSON
- Trigger transformation

---

## The Solution

Update `processing/engine_message_processor.go` to add 3 missing steps:

### Step 1: MongoDB Storage (after line 40)

```go
// Store in MongoDB if available
mongoConnService, err := services.NewMongoDBConnectionService()
if err == nil {
    ctx := context.Background()
    if mongoConnService.Connect(ctx) == nil {
        mongoService := services.NewMongoDBMessageService(
            mongoConnService.GetClient(),
            mongoConnService.GetDatabase(),
        )

        // Store raw message
        mongoDoc := &services.RawMessageDocument{
            MessageID:     msg.ID,
            InterfaceID:   interfaceID,
            RawContent:    msg.Content,
            ReceivedAt:    msg.ReceivedAt,
            MessageSize:   len(msg.Content),
            SourceType:    "tcp",
            SourceIP:      msg.Source,
            Metadata:      msg.Metadata,
        }

        err := mongoService.StoreRawMessage(ctx, mongoDoc)
        if err != nil {
            log.Printf("⚠️  MongoDB storage failed: %v", err)
        } else {
            log.Printf("✅ Message stored in MongoDB: %s", msg.ID)
        }
    }
}
```

### Step 2: HL7 Parsing (after MongoDB storage)

```go
// Parse HL7 to JSON
import "ezhealthkonnect/hl7"

parsed := hl7.ParseWithRealSchema(msg.Content)
if parsed != nil && parsed.Success {
    // Extract message type
    messageType := ""
    if parsed.MessageType.Code != "" && parsed.MessageType.Event != "" {
        messageType = parsed.MessageType.Code + "^" + parsed.MessageType.Event
    }

    // Convert to JSON map
    parsedJSON := map[string]interface{}{
        "enhancedSegments": parsed.EnhancedSegments,
        "segmentOrder":     parsed.SegmentOrder,
        "messageType":      parsed.MessageType,
        "version":          parsed.Version,
        "parsedAt":         parsed.ParsedAt,
        "dictionaryUsed":   parsed.DictionaryUsed,
    }

    // Update MongoDB with parsed content
    if mongoService != nil {
        update := &services.ParsedContentUpdate{
            ParsedContent:    parsedJSON,
            ParsedAt:         time.Now(),
            ParsingTimeMs:    0,
            Format:           "hl7v2",
            MessageType:      messageType,
        }

        err := mongoService.UpdateParsedContent(ctx, interfaceID, msg.ID, update)
        if err != nil {
            log.Printf("⚠️  Failed to update parsed content: %v", err)
        } else {
            log.Printf("✅ Parsed JSON stored in MongoDB: %s", msg.ID)
        }
    }

    // Update PostgreSQL with parsing status
    pe.db.Exec(`
        UPDATE messages_intf_`+sanitizeInterfaceID(interfaceID)+`
        SET status = 'parsed',
            parsing_status = 'success',
            message_type = $1,
            parsed_at = NOW()
        WHERE message_id = $2
    `, messageType, msg.ID)

} else {
    log.Printf("⚠️  Parsing failed for message: %s", msg.ID)
}
```

### Step 3: Trigger Transformation (after parsing)

```go
// Trigger transformation if transformationService available
if pe.transformationService != nil && parsed != nil && parsed.Success {
    go func() {
        ctx := context.Background()
        result, err := pe.transformationService.TransformStoredMessage(
            ctx,
            interfaceID,
            msg.ID,
        )

        if err != nil {
            log.Printf("❌ Transformation failed for %s: %v", msg.ID, err)
        } else if result.Success {
            log.Printf("✅ Transformation completed for %s: %s",
                msg.ID, result.FHIRResourceType)
        }
    }()
}
```

---

## Why This Wasn't Done Yet

**Build Conflicts**: Multiple `.bak` files were restored which caused:
1. `mllp_connectivity_service.go` references undefined `MessageParserService`
2. `transformation_pipeline_service.go` tries to register undefined executors
3. `main.go` had dead code referencing old MLLP service

**The Cleanup Needed**:
1. Remove `services/mllp_connectivity_service.go` (not used - we use ProcessingEngine)
2. Comment out executor registration in `transformation_pipeline_service.go`
3. Build and test

---

## Quick Test After Implementation

1. **Activate interface**:
   ```bash
   curl -X POST http://localhost:8080/api/processing/interfaces/629ac1e8-0c50-447a-b93f-ebfc15830a7d/activate
   ```

2. **Send HL7 message** to port 6661

3. **Check PostgreSQL**:
   ```sql
   SELECT message_id, status, parsing_status, message_type
   FROM messages_intf_629ac1e8_0c50_447a_b93f_ebfc15830a7d
   ORDER BY received_at DESC LIMIT 1;
   ```
   Expected: `status = 'parsed'`, `parsing_status = 'success'`, `message_type = 'ADT^A01'`

4. **Check MongoDB**:
   ```javascript
   db.getCollection('raw_messages_629ac1e8-0c50-447a-b93f-ebfc15830a7d').findOne(
       {parsed_content: {$exists: true}},
       {sort: {received_at: -1}}
   )
   ```
   Expected: Document with `parsed_content.enhancedSegments` populated

5. **Check Transformation** (if service active):
   ```javascript
   db.getCollection('transformed_messages_intf_629ac1e8-0c50-447a-b93f-ebfc15830a7d').findOne(
       {},
       {sort: {transformed_at: -1}}
   )
   ```
   Expected: Document with FHIR `transformed_content`

---

## Files That Need Updates

1. **`processing/engine_message_processor.go`** - Add 3 steps above
2. **`processing/engine.go`** - Ensure `transformationService` is initialized (line 67)
3. **`services/transformation_pipeline_service.go`** - Keep executors commented out for now

---

## Current Working System

The EXISTING running container (started 4 days ago) HAS the working code. To preserve it:
1. Don't rebuild until build issues are resolved
2. Continue using current container for testing
3. Manually trigger transformation via API if needed:
   ```bash
   curl -X POST http://localhost:8080/api/processing/transform/message/629ac1e8-0c50-447a-b93f-ebfc15830a7d/{message_id}
   ```

---

**Last Updated**: 2025-10-18
**Status**: Documented - Ready for implementation once build conflicts resolved
