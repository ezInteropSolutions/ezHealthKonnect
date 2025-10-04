package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

// HybridMessageStorage coordinates message storage between PostgreSQL (metadata) and MongoDB (raw content)
type HybridMessageStorage struct {
	postgresService *InterfaceMessageService
	mongoService    *MongoDBMessageService
}

// HybridMessageData combines metadata and raw content for storage
type HybridMessageData struct {
	// Metadata (PostgreSQL)
	MessageID       string
	CorrelationID   string
	InterfaceID     string
	Status          string
	Priority        int
	ReceivedAt      time.Time
	SourceType      string
	SourceEndpoint  string
	SourceIP        string
	MessageType     string
	MessageSize     int
	MessageEncoding string

	// Raw Content (MongoDB)
	RawContent    string
	ParsedContent map[string]interface{}
	Metadata      map[string]interface{}
}

// NewHybridMessageStorage creates a new hybrid storage service
func NewHybridMessageStorage(db *sql.DB, mongoClient *mongo.Client, mongoDatabase string) *HybridMessageStorage {
	return &HybridMessageStorage{
		postgresService: NewInterfaceMessageService(db),
		mongoService:    NewMongoDBMessageService(mongoClient, mongoDatabase),
	}
}

// InitializeHybridStorage creates hybrid storage with OOB MongoDB detection
func InitializeHybridStorage(db *sql.DB) *HybridMessageStorage {
	// Try to initialize MongoDB connection service (OOB)
	mongoService, err := NewMongoDBConnectionService()
	if err != nil {
		fmt.Printf("ℹ️  MongoDB not configured, hybrid storage unavailable (PostgreSQL only mode): %v\n", err)
		return nil
	}

	// Connect to MongoDB
	ctx := context.Background()
	err = mongoService.Connect(ctx)
	if err != nil {
		fmt.Printf("ℹ️  Failed to connect to MongoDB, hybrid storage unavailable: %v\n", err)
		return nil
	}

	mongoClient := mongoService.GetClient()
	mongoDatabase := mongoService.GetDatabase()

	hybridStorage := NewHybridMessageStorage(db, mongoClient, mongoDatabase)
	fmt.Printf("✅ Hybrid storage initialized (MongoDB + PostgreSQL)\n")

	return hybridStorage
}

// StoreMessage stores message metadata in PostgreSQL and raw content in MongoDB
func (hms *HybridMessageStorage) StoreMessage(ctx context.Context, data *HybridMessageData) error {
	// Step 1: Store raw content in MongoDB
	mongoDoc := &RawMessageDocument{
		MessageID:       data.MessageID,
		InterfaceID:     data.InterfaceID,
		CorrelationID:   data.CorrelationID,
		MessageType:     data.MessageType,
		SourceType:      data.SourceType,
		SourceEndpoint:  data.SourceEndpoint,
		SourceIP:        data.SourceIP,
		ReceivedAt:      data.ReceivedAt,
		MessageSize:     data.MessageSize,
		MessageEncoding: data.MessageEncoding,
		RawContent:      data.RawContent,
		ParsedContent:   data.ParsedContent,
		Metadata:        data.Metadata,
	}

	err := hms.mongoService.StoreRawMessage(ctx, mongoDoc)
	if err != nil {
		return fmt.Errorf("failed to store raw message in MongoDB: %w", err)
	}

	fmt.Printf("✅ Raw message stored in MongoDB: %s (collection: raw_messages_%s)\n", data.MessageID, data.InterfaceID)

	// Step 2: Store metadata in PostgreSQL with reference to MongoDB
	pgMetadata := &MessageData{
		MessageID:       data.MessageID,
		CorrelationID:   data.CorrelationID,
		InterfaceID:     data.InterfaceID,
		Status:          data.Status,
		Priority:        data.Priority,
		ReceivedAt:      data.ReceivedAt,
		SourceType:      data.SourceType,
		SourceEndpoint:  data.SourceEndpoint,
		SourceIP:        data.SourceIP,
		MessageType:     data.MessageType,
		MessageSize:     data.MessageSize,
		MessageEncoding: data.MessageEncoding,
		MongoDocumentID: data.MessageID, // Use message_id as reference
	}

	err = hms.postgresService.StoreMessage(data.InterfaceID, pgMetadata)
	if err != nil {
		return fmt.Errorf("failed to store message metadata in PostgreSQL: %w", err)
	}

	fmt.Printf("✅ Message metadata stored in PostgreSQL: %s (table: messages_intf_%s)\n", data.MessageID, data.InterfaceID)

	return nil
}

// GetCompleteMessage retrieves complete message (metadata from PostgreSQL + content from MongoDB)
func (hms *HybridMessageStorage) GetCompleteMessage(ctx context.Context, interfaceID, messageID string) (*HybridMessageData, error) {
	// Retrieve raw content from MongoDB
	mongoDoc, err := hms.mongoService.GetRawMessage(ctx, interfaceID, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve raw message from MongoDB: %w", err)
	}

	// Combine into hybrid data structure
	hybridData := &HybridMessageData{
		MessageID:       mongoDoc.MessageID,
		CorrelationID:   mongoDoc.CorrelationID,
		InterfaceID:     mongoDoc.InterfaceID,
		MessageType:     mongoDoc.MessageType,
		SourceType:      mongoDoc.SourceType,
		SourceEndpoint:  mongoDoc.SourceEndpoint,
		SourceIP:        mongoDoc.SourceIP,
		ReceivedAt:      mongoDoc.ReceivedAt,
		MessageSize:     mongoDoc.MessageSize,
		MessageEncoding: mongoDoc.MessageEncoding,
		RawContent:      mongoDoc.RawContent,
		ParsedContent:   mongoDoc.ParsedContent,
		Metadata:        mongoDoc.Metadata,
		Status:          "received", // Default status
		Priority:        5,
	}

	return hybridData, nil
}

// StoreTransformedMessage stores transformed output message in MongoDB
func (hms *HybridMessageStorage) StoreTransformedMessage(ctx context.Context, data *TransformedMessageDocument) error {
	err := hms.mongoService.StoreTransformedMessage(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to store transformed message: %w", err)
	}

	fmt.Printf("✅ Transformed message stored in MongoDB: %s (collection: transformed_messages_%s)\n", data.MessageID, data.InterfaceID)

	// Update status in PostgreSQL metadata
	err = hms.postgresService.UpdateMessageStatus(data.InterfaceID, data.SourceMessageID, "transformed", 0, "")
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to update message status in PostgreSQL: %v\n", err)
	}

	return nil
}

// GetMessageStatistics retrieves message statistics from both stores
func (hms *HybridMessageStorage) GetMessageStatistics(ctx context.Context, interfaceID string) (map[string]interface{}, error) {
	// Get MongoDB statistics (raw messages)
	mongoStats, err := hms.mongoService.GetStatistics(ctx, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MongoDB statistics: %w", err)
	}

	// Get PostgreSQL count (metadata)
	pgCount, err := hms.postgresService.GetMessageCount(interfaceID)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to get PostgreSQL count: %v\n", err)
		pgCount = 0
	}

	combinedStats := map[string]interface{}{
		"interface_id":          interfaceID,
		"metadata_count_pg":     pgCount,
		"raw_message_count_mongo": mongoStats["total_messages"],
		"mongo_statistics":      mongoStats,
		"storage_architecture":  "hybrid",
		"generated_at":          time.Now(),
	}

	return combinedStats, nil
}

// UpdateMessageStatus updates message processing status in PostgreSQL
func (hms *HybridMessageStorage) UpdateMessageStatus(interfaceID, messageID, status string, processingTimeMs int64, errorMessage string) error {
	return hms.postgresService.UpdateMessageStatus(interfaceID, messageID, status, processingTimeMs, errorMessage)
}

// ArchiveOldMessages archives old messages in MongoDB
func (hms *HybridMessageStorage) ArchiveOldMessages(ctx context.Context, interfaceID string, olderThan time.Time) (int64, error) {
	return hms.mongoService.ArchiveOldMessages(ctx, interfaceID, olderThan)
}

// GetRawMessagesByInterface retrieves raw messages with pagination from MongoDB
func (hms *HybridMessageStorage) GetRawMessagesByInterface(ctx context.Context, interfaceID string, limit int64, skip int64) ([]*RawMessageDocument, error) {
	return hms.mongoService.GetRawMessagesByInterface(ctx, interfaceID, limit, skip)
}

// DEPRECATED: Use OutputMessageService for transformation output storage
// This struct and method are kept for backward compatibility with MLLP service
// TODO: Migrate MLLP service to use OutputMessageService directly
