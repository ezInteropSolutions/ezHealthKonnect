// services/connectors/mongodb_outbound.go
// MongoDB Outbound Connector — persists pipeline output documents to MongoDB.
//
// Primary use-case: FHIR Bundle persistence after HL7→FHIR transformation.
// The connector parses message.Content as JSON and inserts / upserts the
// resulting document into the configured collection.
//
// Configuration:
//
//	uri               string  MongoDB connection string (e.g. mongodb://host:27017)
//	database          string  Target database name
//	collection        string  Target collection name (default: "messages")
//	write_mode        string  "insert" | "upsert" (default: "insert")
//	upsert_field      string  Field used as the upsert key (default: "_id")
//	content_field     string  If set, only this sub-field of the document is stored
//	connect_timeout   int     Seconds (default 10)
//	operation_timeout int     Seconds per insert/upsert (default 30)
//	max_pool_size     int     MongoDB connection pool size (default 10)
package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBOutboundConnector persists documents to MongoDB.
type MongoDBOutboundConnector struct {
	*BaseOutboundConnector

	// config
	uri              string
	database         string
	collection       string
	writeMode        string // "insert" | "upsert"
	upsertField      string
	contentField     string
	connectTimeout   time.Duration
	operationTimeout time.Duration
	maxPoolSize      uint64

	// runtime
	mu     sync.Mutex
	client *mongo.Client
}

// NewMongoDBOutboundConnector creates a production MongoDB outbound connector.
func NewMongoDBOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mongodb_outbound",
		DisplayName:        "MongoDB Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_tls":    true,
			"supports_upsert": true,
			"supports_pool":   true,
		},
	}
	return &MongoDBOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses configuration and creates the MongoDB client.
func (c *MongoDBOutboundConnector) Initialize(config []byte) error {
	if err := c.BaseOutboundConnector.Initialize(config); err != nil {
		return err
	}
	cfg := c.GetConfig()

	c.uri = cfg.GetString("uri")
	if c.uri == "" {
		c.uri = cfg.GetString("connection_string")
	}
	if c.uri == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "initialize",
			fmt.Errorf("uri or connection_string is required"), false)
	}

	c.database = cfg.GetString("database")
	if c.database == "" {
		c.database = "ezhealthkonnect"
	}

	c.collection = cfg.GetString("collection")
	if c.collection == "" {
		c.collection = "messages"
	}

	c.writeMode = cfg.GetString("write_mode")
	if c.writeMode == "" {
		c.writeMode = "insert"
	}

	c.upsertField = cfg.GetString("upsert_field")
	if c.upsertField == "" {
		c.upsertField = "_id"
	}

	c.contentField = cfg.GetString("content_field")

	connectSec := cfg.GetInt("connect_timeout")
	if connectSec == 0 {
		connectSec = 10
	}
	c.connectTimeout = time.Duration(connectSec) * time.Second

	opSec := cfg.GetInt("operation_timeout")
	if opSec == 0 {
		opSec = 30
	}
	c.operationTimeout = time.Duration(opSec) * time.Second

	poolSize := cfg.GetInt("max_pool_size")
	if poolSize == 0 {
		poolSize = 10
	}
	c.maxPoolSize = uint64(poolSize)

	c.SetMetadata("database", c.database)
	c.SetMetadata("collection", c.collection)
	c.SetMetadata("write_mode", c.writeMode)
	return nil
}

// Validate checks configuration validity.
func (c *MongoDBOutboundConnector) Validate() error {
	if err := c.BaseOutboundConnector.Validate(); err != nil {
		return err
	}
	if c.uri == "" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("uri is required"), false)
	}
	if c.writeMode != "insert" && c.writeMode != "upsert" {
		return NewConnectorError(c.GetMetadata().TypeName, "validate",
			fmt.Errorf("write_mode must be 'insert' or 'upsert'"), false)
	}
	return nil
}

// TestConnection verifies the MongoDB connection.
func (c *MongoDBOutboundConnector) TestConnection(ctx context.Context) error {
	client, err := c.getClient(ctx)
	if err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	pingCtx, cancel := context.WithTimeout(ctx, c.connectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		return NewConnectorError(c.GetMetadata().TypeName, "test_connection", err, true)
	}
	return nil
}

// Send inserts or upserts the message content into MongoDB.
func (c *MongoDBOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	start := time.Now()
	typeName := c.GetMetadata().TypeName

	if message.Content == "" {
		return nil, NewConnectorError(typeName, "send", fmt.Errorf("message content is empty"), false)
	}

	// Parse JSON content
	doc, err := c.parseContent(message)
	if err != nil {
		return nil, NewConnectorError(typeName, "send", err, false)
	}

	// Inject standard metadata
	if message.MessageID != "" {
		doc["_message_id"] = message.MessageID
	}
	if message.InterfaceID != "" {
		doc["_interface_id"] = message.InterfaceID
	}
	doc["_stored_at"] = time.Now().UTC()

	client, err := c.getClient(ctx)
	if err != nil {
		c.RecordError(err)
		return &DeliveryResult{Success: false, MessageID: message.MessageID,
			Timestamp: time.Now(), ErrorMessage: err.Error(), DurationMs: time.Since(start).Milliseconds()}, err
	}

	coll := c.resolveCollection(message, client)
	opCtx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	var insertedID interface{}

	switch c.writeMode {
	case "upsert":
		filterVal, hasFilter := doc[c.upsertField]
		if !hasFilter || filterVal == nil {
			// Fall back to insert when upsert field is absent
			res, insErr := coll.InsertOne(opCtx, doc)
			if insErr != nil {
				c.RecordError(insErr)
				return &DeliveryResult{Success: false, MessageID: message.MessageID,
					Timestamp: time.Now(), ErrorMessage: insErr.Error(), DurationMs: time.Since(start).Milliseconds()}, insErr
			}
			insertedID = res.InsertedID
		} else {
			filter := bson.M{c.upsertField: filterVal}
			update := bson.M{"$set": doc}
			opts := options.Update().SetUpsert(true)
			res, updErr := coll.UpdateOne(opCtx, filter, update, opts)
			if updErr != nil {
				c.RecordError(updErr)
				return &DeliveryResult{Success: false, MessageID: message.MessageID,
					Timestamp: time.Now(), ErrorMessage: updErr.Error(), DurationMs: time.Since(start).Milliseconds()}, updErr
			}
			if res.UpsertedID != nil {
				insertedID = res.UpsertedID
			} else {
				insertedID = filterVal
			}
		}
	default: // insert
		res, insErr := coll.InsertOne(opCtx, doc)
		if insErr != nil {
			c.RecordError(insErr)
			return &DeliveryResult{Success: false, MessageID: message.MessageID,
				Timestamp: time.Now(), ErrorMessage: insErr.Error(), DurationMs: time.Since(start).Milliseconds()}, insErr
		}
		insertedID = res.InsertedID
	}

	c.IncrementMessagesSent()

	return &DeliveryResult{
		Success:    true,
		MessageID:  message.MessageID,
		Timestamp:  time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Metadata:   map[string]interface{}{"inserted_id": fmt.Sprintf("%v", insertedID)},
	}, nil
}

// SendBatch inserts multiple messages using a single bulk write.
func (c *MongoDBOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	results := make([]*DeliveryResult, len(messages))
	for i, msg := range messages {
		res, err := c.Send(ctx, msg)
		if err != nil {
			results[i] = &DeliveryResult{
				Success:      false,
				MessageID:    msg.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: err.Error(),
			}
		} else {
			results[i] = res
		}
	}
	return results, nil
}

// Close disconnects the MongoDB client.
func (c *MongoDBOutboundConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.client.Disconnect(ctx)
		c.client = nil
	}
	return c.BaseOutboundConnector.Close()
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// getClient returns (and lazily creates) the shared MongoDB client.
func (c *MongoDBOutboundConnector) getClient(ctx context.Context) (*mongo.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	opts := options.Client().
		ApplyURI(c.uri).
		SetMaxPoolSize(c.maxPoolSize).
		SetConnectTimeout(c.connectTimeout)

	connectCtx, cancel := context.WithTimeout(ctx, c.connectTimeout)
	defer cancel()
	client, err := mongo.Connect(connectCtx, opts)
	if err != nil {
		return nil, fmt.Errorf("mongodb connect: %w", err)
	}
	c.client = client
	return client, nil
}

// resolveCollection returns the target collection.
// If the message metadata includes "mongo_collection", it overrides the config.
func (c *MongoDBOutboundConnector) resolveCollection(message *models.OutboundMessage, client *mongo.Client) *mongo.Collection {
	collName := c.collection
	if message.Metadata != nil {
		if override, ok := message.Metadata["mongo_collection"]; ok && override != "" {
			collName = override
		}
	}
	return client.Database(c.database).Collection(collName)
}

// parseContent extracts a JSON document from the message.
// If contentField is set, only that nested field is extracted.
//
// Decodes into map[string]interface{} first because encoding/json produces
// map[string]interface{} for nested objects (not bson.M / primitive.M).
// A bson.M type assertion on a nested JSON-decoded value would always fail.
func (c *MongoDBOutboundConnector) parseContent(message *models.OutboundMessage) (bson.M, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &raw); err != nil {
		// Content is not JSON — store as plain string
		return bson.M{"content": message.Content, "content_type": message.ContentType}, nil
	}

	if c.contentField == "" {
		return bson.M(raw), nil
	}

	// Navigate to the nested sub-field using map[string]interface{} assertions
	// (bson.M is a named type — json.Unmarshal produces map[string]interface{} for nested objects)
	parts := strings.Split(c.contentField, ".")
	var cur interface{} = raw
	for _, p := range parts {
		m, ok := cur.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("content_field path '%s' not found: segment '%s' is not an object",
				c.contentField, p)
		}
		cur = m[p]
		if cur == nil {
			return nil, fmt.Errorf("content_field path '%s' not found: key '%s' missing", c.contentField, p)
		}
	}

	subDoc, ok := cur.(map[string]interface{})
	if !ok {
		// Sub-field is a scalar — wrap it
		return bson.M{"value": cur}, nil
	}
	return bson.M(subDoc), nil
}
