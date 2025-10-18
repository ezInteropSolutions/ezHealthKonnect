// processing/engine_message_processor.go
// Message processing for ProcessingEngine

package processing

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/services"
)

// processMessages processes incoming messages from the TCP connector channel
func (pe *ProcessingEngine) processMessages(interfaceID string, messageChan <-chan Message) {
	log.Printf("📨 Message processor started for interface %s", interfaceID)

	for msg := range messageChan {
		startTime := time.Now()

		log.Printf("📥 Processing message for interface %s: ID=%s, Size=%d bytes",
			interfaceID, msg.ID, len(msg.Content))

		// STEP 1: Store message in PostgreSQL interface table
		err := pe.storeMessage(interfaceID, &msg)
		if err != nil {
			log.Printf("❌ Failed to store message for interface %s: %v", interfaceID, err)
			pe.updateInterfaceError(interfaceID)
			continue
		}

		processingTime := time.Since(startTime)
		log.Printf("✅ Message stored in PostgreSQL for interface %s: ID=%s (took %v)",
			interfaceID, msg.ID, processingTime)

		// STEP 2: Store in MongoDB + Parse to JSON (async)
		go pe.storeAndParse(interfaceID, &msg)

		// Update interface statistics
		pe.updateInterfaceStats(interfaceID, processingTime)
	}

	log.Printf("📪 Message processor stopped for interface %s", interfaceID)
}

// storeMessage stores the raw message in PostgreSQL
func (pe *ProcessingEngine) storeMessage(interfaceID string, msg *Message) error {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))

	query := fmt.Sprintf(`
		INSERT INTO %s (
			message_id, interface_id, status, priority,
			received_at, source_type, source_endpoint, source_ip,
			message_type, message_size, message_encoding,
			raw_message, created_at, updated_at
		) VALUES (
			$1, $2, 'received', 1,
			$3, 'tcp', $4, $5,
			'hl7v2', $6, 'UTF-8',
			$7, NOW(), NOW()
		)
	`, tableName)

	_, err := pe.db.Exec(query,
		msg.ID,
		interfaceID,
		msg.ReceivedAt,
		msg.Source,
		msg.Source,
		len(msg.Content),
		msg.Content,
	)

	return err
}

// sanitizeInterfaceID converts UUID to table-safe format
func sanitizeInterfaceID(id string) string {
	return strings.ReplaceAll(id, "-", "_")
}

// updateInterfaceStats updates interface processing statistics
func (pe *ProcessingEngine) updateInterfaceStats(interfaceID string, processingTime time.Duration) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if iface, exists := pe.activeInterfaces[interfaceID]; exists {
		iface.MessagesProcessed++
		iface.LastActivity = time.Now()
		pe.stats.LastActivity = time.Now()
	}
}

// updateInterfaceError increments error count
func (pe *ProcessingEngine) updateInterfaceError(interfaceID string) {
	pe.mutex.Lock()
	defer pe.mutex.Unlock()

	if iface, exists := pe.activeInterfaces[interfaceID]; exists {
		iface.Errors++
		iface.LastActivity = time.Now()
		pe.stats.LastActivity = time.Now()
	}
}

// storeAndParse stores raw message in MongoDB and triggers JSON parsing
func (pe *ProcessingEngine) storeAndParse(interfaceID string, msg *Message) {
	ctx := context.Background()

	// STEP 1: Store raw message in MongoDB (if available)
	if pe.mongoService != nil {
		rawDoc := &services.RawMessageDocument{
			MessageID:       msg.ID,
			InterfaceID:     interfaceID,
			MessageType:     "hl7v2",
			SourceType:      "tcp",
			SourceEndpoint:  msg.Source,
			ReceivedAt:      msg.ReceivedAt,
			MessageSize:     len(msg.Content),
			MessageEncoding: "UTF-8",
			RawContent:      msg.Content,
		}

		err := pe.mongoService.StoreRawMessage(ctx, rawDoc)
		if err != nil {
			log.Printf("⚠️  Failed to store raw message in MongoDB: %v", err)
			// Update PostgreSQL mongo_synced status
			pe.updateMongoSyncStatus(interfaceID, msg.ID, false)
		} else {
			log.Printf("💾 Message stored in MongoDB: %s", msg.ID)
			// Update PostgreSQL mongo_synced status
			pe.updateMongoSyncStatus(interfaceID, msg.ID, true)
		}
	}

	// STEP 2: Parse to JSON (if parser service available)
	if pe.parserService != nil {
		log.Printf("🔄 Starting JSON conversion for message: %s", msg.ID)
		result, err := pe.parserService.ParseToJSON(ctx, msg.ID, interfaceID, msg.Content)
		if err != nil {
			log.Printf("❌ JSON conversion failed for %s: %v", msg.ID, err)
		} else {
			log.Printf("✅ JSON conversion completed for %s in %dms",
				msg.ID, result.ParsingTime.Milliseconds())

			// STEP 3: Trigger transformation pipeline (when ready)
			// TODO: Uncomment when transformation pipeline is ready
			// if pe.transformationService != nil {
			//     go pe.transformationService.ExecutePipeline(ctx, interfaceID, msg.ID, result)
			// }
		}
	}
}

// updateMongoSyncStatus updates the mongo_synced flag in PostgreSQL
func (pe *ProcessingEngine) updateMongoSyncStatus(interfaceID string, messageID string, synced bool) {
	tableName := fmt.Sprintf("messages_intf_%s", sanitizeInterfaceID(interfaceID))
	query := fmt.Sprintf(`UPDATE %s SET mongo_synced = $1 WHERE message_id = $2`, tableName)

	_, err := pe.db.Exec(query, synced, messageID)
	if err != nil {
		log.Printf("⚠️  Failed to update mongo_synced status: %v", err)
	}
}
