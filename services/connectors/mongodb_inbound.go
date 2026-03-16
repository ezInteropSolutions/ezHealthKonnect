// services/connectors/mongodb_inbound.go
// MongoDB Inbound Connector — polls a collection for new documents and converts them to messages.
//
// Two delivery modes:
//   - Polling  (default): List documents via Find() using an incremental filter on a
//     timestamp or ObjectID column. After processing: nothing | delete | update_flag.
//   - Change Stream: Watch collection for insert/update events in real-time (requires
//     MongoDB replica set or Atlas cluster). Set watch_changes: true.
//
// Configuration:
//
//	connection_string  string  MongoDB URI, e.g. mongodb://host:27017
//	database           string  Database name
//	collection         string  Collection name
//	query              string  JSON filter, e.g. {"status": "pending"}  (default: {})
//	incremental_field  string  Field used for incremental polling (e.g. _id, updatedAt)
//	polling_interval   int     Seconds between polls (default: 60)
//	max_records        int     Documents per poll (default: 100)
//	after_processing   string  "nothing" | "delete" | "update_flag"
//	processed_flag_field string Field to set for update_flag mode (e.g. processed)
//	processed_flag_value string Value to set (e.g. true)
//	watch_changes      bool    Use change streams instead of polling (requires replica set)
//	connect_timeout    int     Seconds (default: 10)
//	operation_timeout  int     Seconds per operation (default: 30)

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBInboundConfig holds all configuration for the MongoDB inbound connector.
type MongoDBInboundConfig struct {
	ConnectionString    string `json:"connection_string"`
	Database            string `json:"database"`
	Collection          string `json:"collection"`
	Query               string `json:"query"`              // JSON filter, default "{}"
	IncrementalField    string `json:"incremental_field"`  // field for incremental cursor
	PollingInterval     int    `json:"polling_interval"`   // seconds
	MaxRecords          int    `json:"max_records"`
	AfterProcessing     string `json:"after_processing"`   // nothing | delete | update_flag
	ProcessedFlagField  string `json:"processed_flag_field"`
	ProcessedFlagValue  string `json:"processed_flag_value"`
	WatchChanges        bool   `json:"watch_changes"`      // change stream mode
	ConnectTimeout      int    `json:"connect_timeout"`
	OperationTimeout    int    `json:"operation_timeout"`
	Username            string `json:"username"`           // alternative to URI creds
	Password            string `json:"password"`
	Host                string `json:"host"`               // alternative: host + port
	Port                int    `json:"port"`
}

// MongoDBInboundConnector polls MongoDB for new documents.
type MongoDBInboundConnector struct {
	*BaseInboundConnector
	config          MongoDBInboundConfig
	client          *mongo.Client
	lastID          primitive.ObjectID // last _id seen for incremental pull
	lastTimestamp   time.Time
	mu              sync.Mutex
}

// NewMongoDBInboundConnector creates a production MongoDB inbound connector.
func NewMongoDBInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mongodb_inbound",
		DisplayName:        "MongoDB Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_tls":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_filters":          true,
		},
	}
	return &MongoDBInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize configures the connector and verifies MongoDB connectivity.
func (c *MongoDBInboundConnector) Initialize(config []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := json.Unmarshal(config, &c.config); err != nil {
		return fmt.Errorf("failed to parse MongoDB inbound config: %w", err)
	}

	// Defaults
	if c.config.PollingInterval == 0 {
		c.config.PollingInterval = 60
	}
	if c.config.MaxRecords == 0 {
		c.config.MaxRecords = 100
	}
	if c.config.AfterProcessing == "" {
		c.config.AfterProcessing = "nothing"
	}
	if c.config.ConnectTimeout == 0 {
		c.config.ConnectTimeout = 10
	}
	if c.config.OperationTimeout == 0 {
		c.config.OperationTimeout = 30
	}

	// Build connection URI
	uri := c.buildURI()

	// Connect to MongoDB
	connectCtx, cancel := context.WithTimeout(context.Background(),
		time.Duration(c.config.ConnectTimeout)*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	c.client = client
	c.BaseInboundConnector.BaseConnector.initialized = true
	c.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ MongoDB Inbound: Initialized (db=%s, collection=%s)",
		c.config.Database, c.config.Collection)
	return nil
}

// buildURI constructs a MongoDB connection URI from config.
func (c *MongoDBInboundConnector) buildURI() string {
	if c.config.ConnectionString != "" {
		return c.config.ConnectionString
	}
	host := c.config.Host
	if host == "" {
		host = "localhost"
	}
	port := c.config.Port
	if port == 0 {
		port = 27017
	}
	if c.config.Username != "" {
		return fmt.Sprintf("mongodb://%s:%s@%s:%d",
			c.config.Username, c.config.Password, host, port)
	}
	return fmt.Sprintf("mongodb://%s:%d", host, port)
}

// TestConnection verifies the MongoDB connection is alive.
func (c *MongoDBInboundConnector) TestConnection(ctx context.Context) error {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client == nil {
		return fmt.Errorf("not initialized")
	}
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}
	return nil
}

// Validate checks that required fields are set.
func (c *MongoDBInboundConnector) Validate() error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if c.config.Database == "" {
		return fmt.Errorf("database is required")
	}
	if c.config.Collection == "" {
		return fmt.Errorf("collection is required")
	}
	return nil
}

// SupportsCron returns true — polling mode supports scheduled execution.
func (c *MongoDBInboundConnector) SupportsCron() bool { return true }

// Start begins polling or watching the MongoDB collection.
func (c *MongoDBInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !c.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	c.BaseInboundConnector.SetState(StateRunning)

	if c.config.WatchChanges {
		go c.watchLoop(ctx, messageChan)
	} else {
		go c.pollLoop(ctx, messageChan)
	}
	return nil
}

// pollLoop periodically queries the collection for new documents.
func (c *MongoDBInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	ticker := time.NewTicker(time.Duration(c.config.PollingInterval) * time.Second)
	defer ticker.Stop()

	// Poll immediately on start
	c.poll(ctx, messageChan)

	for {
		select {
		case <-ctx.Done():
			log.Printf("🍃 MongoDB Inbound: Context cancelled, stopping poll loop")
			return
		case <-c.BaseInboundConnector.stopCh:
			log.Printf("🍃 MongoDB Inbound: Stop signal received")
			return
		case <-ticker.C:
			c.poll(ctx, messageChan)
		}
	}
}

// poll executes one query against the configured collection.
func (c *MongoDBInboundConnector) poll(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	if client == nil {
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, time.Duration(c.config.OperationTimeout)*time.Second)
	defer cancel()

	coll := client.Database(c.config.Database).Collection(c.config.Collection)

	// Build filter
	filter, err := c.buildFilter()
	if err != nil {
		log.Printf("❌ MongoDB Inbound: Invalid filter: %v", err)
		return
	}

	// Find options
	findOpts := options.Find().SetLimit(int64(c.config.MaxRecords))
	if c.config.IncrementalField != "" {
		findOpts = findOpts.SetSort(bson.D{{Key: c.config.IncrementalField, Value: 1}})
	}

	cursor, err := coll.Find(opCtx, filter, findOpts)
	if err != nil {
		log.Printf("❌ MongoDB Inbound: Find failed: %v", err)
		return
	}
	defer cursor.Close(opCtx)

	count := 0
	var processedIDs []primitive.ObjectID

	for cursor.Next(opCtx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			log.Printf("❌ MongoDB Inbound: Decode error: %v", err)
			continue
		}

		msg := c.docToMessage(doc)
		messageChan <- msg
		count++

		// Track ObjectID for incremental / after-processing
		if id, ok := doc["_id"].(primitive.ObjectID); ok {
			processedIDs = append(processedIDs, id)
			if id.Timestamp().After(c.lastTimestamp) {
				c.lastID = id
				c.lastTimestamp = id.Timestamp()
			}
		}
	}

	if count > 0 {
		log.Printf("✅ MongoDB Inbound: Polled %d documents from %s.%s",
			count, c.config.Database, c.config.Collection)
		if len(processedIDs) > 0 {
			c.afterProcess(ctx, coll, processedIDs)
		}
	}
}

// buildFilter constructs the BSON filter from config, applying incremental cursor if active.
func (c *MongoDBInboundConnector) buildFilter() (bson.M, error) {
	var base bson.M

	// Parse user-supplied query
	if c.config.Query != "" && c.config.Query != "{}" {
		if err := bson.UnmarshalExtJSON([]byte(c.config.Query), true, &base); err != nil {
			return nil, fmt.Errorf("invalid query JSON: %w", err)
		}
	} else {
		base = bson.M{}
	}

	// Apply incremental filter on _id (ObjectID timestamp)
	if c.config.IncrementalField == "_id" || c.config.IncrementalField == "" {
		if !c.lastID.IsZero() {
			base["_id"] = bson.M{"$gt": c.lastID}
		}
	} else if c.config.IncrementalField != "" && !c.lastTimestamp.IsZero() {
		base[c.config.IncrementalField] = bson.M{"$gt": c.lastTimestamp}
	}

	return base, nil
}

// afterProcess runs the configured after-processing action on a batch of document IDs.
func (c *MongoDBInboundConnector) afterProcess(ctx context.Context, coll *mongo.Collection, ids []primitive.ObjectID) {
	switch c.config.AfterProcessing {
	case "delete":
		filter := bson.M{"_id": bson.M{"$in": ids}}
		result, err := coll.DeleteMany(ctx, filter)
		if err != nil {
			log.Printf("❌ MongoDB Inbound: after-processing delete failed: %v", err)
		} else {
			log.Printf("🗑️  MongoDB Inbound: Deleted %d processed documents", result.DeletedCount)
		}
	case "update_flag":
		if c.config.ProcessedFlagField == "" {
			log.Printf("⚠️  MongoDB Inbound: update_flag requires processed_flag_field")
			return
		}
		var flagValue interface{} = c.config.ProcessedFlagValue
		if flagValue == "" {
			flagValue = true
		}
		filter := bson.M{"_id": bson.M{"$in": ids}}
		update := bson.M{"$set": bson.M{c.config.ProcessedFlagField: flagValue}}
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			log.Printf("❌ MongoDB Inbound: after-processing update_flag failed: %v", err)
		} else {
			log.Printf("✅ MongoDB Inbound: Flagged %d documents as processed", result.ModifiedCount)
		}
	}
}

// watchLoop uses MongoDB Change Streams to receive documents in real-time.
func (c *MongoDBInboundConnector) watchLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()

	coll := client.Database(c.config.Database).Collection(c.config.Collection)
	// Watch inserts and updates
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"operationType": bson.M{"$in": bson.A{"insert", "update", "replace"}},
		}}},
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.BaseInboundConnector.stopCh:
			return
		default:
		}

		stream, err := coll.Watch(ctx, pipeline, options.ChangeStream().SetFullDocument(options.UpdateLookup))
		if err != nil {
			log.Printf("❌ MongoDB Inbound: Change stream error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("🔄 MongoDB Inbound: Change stream active on %s.%s", c.config.Database, c.config.Collection)
		for stream.Next(ctx) {
			var event bson.M
			if err := stream.Decode(&event); err != nil {
				log.Printf("❌ MongoDB Inbound: Change stream decode error: %v", err)
				continue
			}
			// Extract the full document from the event
			if fullDoc, ok := event["fullDocument"].(bson.M); ok {
				messageChan <- c.docToMessage(fullDoc)
			}
		}
		_ = stream.Close(ctx)
	}
}

// docToMessage converts a MongoDB BSON document to an InboundMessage.
func (c *MongoDBInboundConnector) docToMessage(doc bson.M) *models.InboundMessage {
	// Try common message content fields first
	contentFields := []string{"raw_message", "message", "content", "payload", "data", "body"}
	content := ""
	contentType := "application/json"

	for _, field := range contentFields {
		if val, ok := doc[field]; ok && val != nil {
			switch v := val.(type) {
			case string:
				content = v
			case []byte:
				content = string(v)
			case primitive.Binary:
				content = string(v.Data)
			}
			if content != "" {
				break
			}
		}
	}

	// Fall back to full document JSON
	if content == "" {
		b, _ := json.Marshal(doc)
		content = string(b)
	} else {
		// Detect content type
		if len(content) >= 4 && content[:4] == "MSH|" {
			contentType = "application/hl7-v2"
		} else if len(content) > 0 && content[0] == '{' {
			contentType = "application/json"
		} else if len(content) > 0 && content[0] == '<' {
			contentType = "application/xml"
		}
	}

	// Extract message ID from _id field
	msgID := fmt.Sprintf("mongo_%s_%d", c.config.Collection, time.Now().UnixNano())
	if id, ok := doc["_id"].(primitive.ObjectID); ok {
		msgID = fmt.Sprintf("mongo_%s_%s", c.config.Collection, id.Hex())
	} else if id, ok := doc["_id"].(string); ok {
		msgID = fmt.Sprintf("mongo_%s_%s", c.config.Collection, id)
	}

	// Extract message type if present
	msgType := ""
	for _, f := range []string{"message_type", "msg_type", "type"} {
		if v, ok := doc[f].(string); ok && v != "" {
			msgType = v
			break
		}
	}

	return &models.InboundMessage{
		MessageID:      msgID,
		Content:        content,
		ContentType:    contentType,
		SourceType:     "mongodb",
		SourceEndpoint: fmt.Sprintf("%s.%s", c.config.Database, c.config.Collection),
		MessageType:    msgType,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Database":   c.config.Database,
			"Collection": c.config.Collection,
		},
	}
}

// Stop halts the connector and closes the MongoDB client.
func (c *MongoDBInboundConnector) Stop() error {
	log.Printf("🍃 MongoDB Inbound: Stopping")
	if err := c.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.client.Disconnect(ctx); err != nil {
			return fmt.Errorf("failed to disconnect MongoDB client: %w", err)
		}
		c.client = nil
	}
	log.Printf("✅ MongoDB Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (c *MongoDBInboundConnector) Close() error { return c.Stop() }

// GetStatus returns connector status with MongoDB-specific metadata.
func (c *MongoDBInboundConnector) GetStatus() ConnectorStatus {
	status := c.BaseInboundConnector.GetStatus()
	c.mu.Lock()
	client := c.client
	c.mu.Unlock()
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := client.Ping(ctx, nil); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	return status
}
