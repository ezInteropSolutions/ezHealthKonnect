package services

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBMessageService handles storage and retrieval of raw messages in MongoDB
type MongoDBMessageService struct {
	client   *mongo.Client
	database string
}

// RawMessageDocument represents the unstructured message document in MongoDB
type RawMessageDocument struct {
	MessageID       string                 `bson:"message_id"`
	InterfaceID     string                 `bson:"interface_id"`
	CorrelationID   string                 `bson:"correlation_id,omitempty"`
	MessageType     string                 `bson:"message_type"`
	SourceType      string                 `bson:"source_type"`
	SourceEndpoint  string                 `bson:"source_endpoint,omitempty"`
	SourceIP        string                 `bson:"source_ip,omitempty"`
	ReceivedAt      time.Time              `bson:"received_at"`
	MessageSize     int                    `bson:"message_size"`
	MessageEncoding string                 `bson:"message_encoding"`
	RawContent      string                 `bson:"raw_content"` // Full HL7 message
	ParsedContent   map[string]interface{} `bson:"parsed_content,omitempty"` // Parsed JSON structure
	Metadata        map[string]interface{} `bson:"metadata,omitempty"` // Additional flexible metadata
	CreatedAt       time.Time              `bson:"created_at"`
}

// TransformedMessageDocument represents the transformed output message in MongoDB
type TransformedMessageDocument struct {
	MessageID            string                 `bson:"message_id"`
	InterfaceID          string                 `bson:"interface_id"`
	CorrelationID        string                 `bson:"correlation_id,omitempty"`
	SourceMessageID      string                 `bson:"source_message_id"` // Reference to raw message
	TransformationType   string                 `bson:"transformation_type"` // e.g., "hl7_to_fhir"
	TransformedAt        time.Time              `bson:"transformed_at"`
	TransformedContent   map[string]interface{} `bson:"transformed_content"` // FHIR Bundle or other output
	TransformationConfig map[string]interface{} `bson:"transformation_config,omitempty"` // Config used
	ResourcesGenerated   []string               `bson:"resources_generated,omitempty"` // e.g., ["Patient", "Encounter"]
	Metadata             map[string]interface{} `bson:"metadata,omitempty"`
	CreatedAt            time.Time              `bson:"created_at"`
}

// NewMongoDBMessageService creates a new MongoDB message service
func NewMongoDBMessageService(client *mongo.Client, database string) *MongoDBMessageService {
	return &MongoDBMessageService{
		client:   client,
		database: database,
	}
}

// StoreRawMessage stores a raw message in MongoDB
func (mms *MongoDBMessageService) StoreRawMessage(ctx context.Context, message *RawMessageDocument) error {
	if mms.client == nil {
		return fmt.Errorf("MongoDB client not available")
	}

	// Use interface-specific collection for better performance and isolation
	collectionName := fmt.Sprintf("raw_messages_%s", message.InterfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	// Set created timestamp
	message.CreatedAt = time.Now()

	// Insert document
	_, err := collection.InsertOne(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to store raw message in MongoDB: %w", err)
	}

	// Ensure indexes exist (idempotent operation)
	go mms.ensureIndexes(collectionName)

	return nil
}

// StoreTransformedMessage stores a transformed message in MongoDB
func (mms *MongoDBMessageService) StoreTransformedMessage(ctx context.Context, message *TransformedMessageDocument) error {
	if mms.client == nil {
		return fmt.Errorf("MongoDB client not available")
	}

	// Use interface-specific collection for better performance
	collectionName := fmt.Sprintf("transformed_messages_%s", message.InterfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	// Set created timestamp
	message.CreatedAt = time.Now()

	// Insert document
	_, err := collection.InsertOne(ctx, message)
	if err != nil {
		return fmt.Errorf("failed to store transformed message in MongoDB: %w", err)
	}

	// Ensure indexes exist (idempotent operation)
	go mms.ensureIndexes(collectionName)

	return nil
}

// GetRawMessage retrieves a raw message by message ID
func (mms *MongoDBMessageService) GetRawMessage(ctx context.Context, interfaceID, messageID string) (*RawMessageDocument, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	var message RawMessageDocument
	filter := bson.M{"message_id": messageID}

	err := collection.FindOne(ctx, filter).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		return nil, fmt.Errorf("failed to retrieve raw message: %w", err)
	}

	return &message, nil
}

// GetParsedContent retrieves only the parsed_content field from a raw message
func (mms *MongoDBMessageService) GetParsedContent(ctx context.Context, interfaceID, messageID string) (map[string]interface{}, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	// Query only the parsed_content field for efficiency
	filter := bson.M{"message_id": messageID}
	projection := bson.M{"parsed_content": 1, "_id": 0}

	var result struct {
		ParsedContent map[string]interface{} `bson:"parsed_content"`
	}

	err := collection.FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		return nil, fmt.Errorf("failed to retrieve parsed content: %w", err)
	}

	return result.ParsedContent, nil
}

// GetTransformedMessage retrieves a transformed message by message ID
func (mms *MongoDBMessageService) GetTransformedMessage(ctx context.Context, interfaceID, messageID string) (*TransformedMessageDocument, error) {
	collectionName := fmt.Sprintf("transformed_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	var message TransformedMessageDocument
	filter := bson.M{"message_id": messageID}

	err := collection.FindOne(ctx, filter).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("transformed message not found: %s", messageID)
		}
		return nil, fmt.Errorf("failed to retrieve transformed message: %w", err)
	}

	return &message, nil
}

// GetRawMessagesByInterface retrieves raw messages for an interface with pagination
func (mms *MongoDBMessageService) GetRawMessagesByInterface(ctx context.Context, interfaceID string, limit int64, skip int64) ([]*RawMessageDocument, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	findOptions := options.Find()
	findOptions.SetLimit(limit)
	findOptions.SetSkip(skip)
	findOptions.SetSort(bson.D{{Key: "received_at", Value: -1}}) // Most recent first

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to query raw messages: %w", err)
	}
	defer cursor.Close(ctx)

	var messages []*RawMessageDocument
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode raw messages: %w", err)
	}

	return messages, nil
}

// GetMessageCount returns the total number of raw messages for an interface
func (mms *MongoDBMessageService) GetMessageCount(ctx context.Context, interfaceID string) (int64, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	count, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}

// ensureIndexes creates indexes for performance (idempotent)
func (mms *MongoDBMessageService) ensureIndexes(collectionName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	collection := mms.client.Database(mms.database).Collection(collectionName)

	// Define indexes
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "message_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "received_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "message_type", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "correlation_id", Value: 1}},
		},
		{
			Keys: bson.D{
				{Key: "interface_id", Value: 1},
				{Key: "received_at", Value: -1},
			},
		},
	}

	// Create indexes (will skip if they already exist)
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to create MongoDB indexes for %s: %v\n", collectionName, err)
	}
}

// ArchiveOldMessages moves old messages to archive collection (for data retention)
func (mms *MongoDBMessageService) ArchiveOldMessages(ctx context.Context, interfaceID string, olderThan time.Time) (int64, error) {
	sourceCollection := fmt.Sprintf("raw_messages_%s", interfaceID)
	archiveCollection := fmt.Sprintf("archived_messages_%s", interfaceID)

	source := mms.client.Database(mms.database).Collection(sourceCollection)
	archive := mms.client.Database(mms.database).Collection(archiveCollection)

	// Find old messages
	filter := bson.M{"received_at": bson.M{"$lt": olderThan}}
	cursor, err := source.Find(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to find old messages: %w", err)
	}
	defer cursor.Close(ctx)

	var messages []interface{}
	if err := cursor.All(ctx, &messages); err != nil {
		return 0, fmt.Errorf("failed to decode messages for archiving: %w", err)
	}

	if len(messages) == 0 {
		return 0, nil // No messages to archive
	}

	// Insert into archive
	_, err = archive.InsertMany(ctx, messages)
	if err != nil {
		return 0, fmt.Errorf("failed to insert into archive: %w", err)
	}

	// Delete from source
	deleteResult, err := source.DeleteMany(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete archived messages: %w", err)
	}

	fmt.Printf("📦 Archived %d messages for interface %s\n", deleteResult.DeletedCount, interfaceID)
	return deleteResult.DeletedCount, nil
}

// GetStatistics returns message statistics for an interface
func (mms *MongoDBMessageService) GetStatistics(ctx context.Context, interfaceID string) (map[string]interface{}, error) {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	collection := mms.client.Database(mms.database).Collection(collectionName)

	// Aggregate statistics
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$message_type"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "total_size", Value: bson.D{{Key: "$sum", Value: "$message_size"}}},
			{Key: "avg_size", Value: bson.D{{Key: "$avg", Value: "$message_size"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate statistics: %w", err)
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("failed to decode statistics: %w", err)
	}

	// Total count
	totalCount, _ := collection.CountDocuments(ctx, bson.M{})

	stats := map[string]interface{}{
		"interface_id":      interfaceID,
		"total_messages":    totalCount,
		"by_message_type":   results,
		"collection":        collectionName,
		"generated_at":      time.Now(),
	}

	return stats, nil
}

// ParsedContentUpdate contains parsed content data
type ParsedContentUpdate struct {
	ParsedContent    map[string]interface{}
	ParsedAt         time.Time
	ParsingTimeMs    int
	Format           string
	MessageType      string
	ValidationResult interface{}
}

// UpdateParsedContent updates MongoDB document with parsed JSON
func (mms *MongoDBMessageService) UpdateParsedContent(
	ctx context.Context,
	interfaceID string,
	messageID string,
	update *ParsedContentUpdate,
) error {
	collection := mms.getRawMessageCollection(interfaceID)

	filter := bson.M{"message_id": messageID}
	updateDoc := bson.M{
		"$set": bson.M{
			"parsed_content":     update.ParsedContent,
			"parsed_at":          update.ParsedAt,
			"parsing_time_ms":    update.ParsingTimeMs,
			"parsed_format":      update.Format,
			"message_type":       update.MessageType,
			"validation_result":  update.ValidationResult,
			"updated_at":         time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, filter, updateDoc)
	if err != nil {
		return fmt.Errorf("failed to update parsed content: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	fmt.Printf("✅ Parsed content stored in MongoDB for message: %s\n", messageID)
	return nil
}

// getRawMessageCollection returns the raw message collection for an interface
func (mms *MongoDBMessageService) getRawMessageCollection(interfaceID string) *mongo.Collection {
	collectionName := fmt.Sprintf("raw_messages_%s", interfaceID)
	return mms.client.Database(mms.database).Collection(collectionName)
}

// GetCollection returns a MongoDB collection by name (public helper for external packages)
func (mms *MongoDBMessageService) GetCollection(collectionName string) *mongo.Collection {
	return mms.client.Database(mms.database).Collection(collectionName)
}
