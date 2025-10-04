package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBConnectionService manages MongoDB connection lifecycle
type MongoDBConnectionService struct {
	client   *mongo.Client
	database string
	uri      string
	isConnected bool
}

// MongoDBConfig holds MongoDB connection configuration
type MongoDBConfig struct {
	URI      string
	Database string
}

// NewMongoDBConnectionService creates a new MongoDB connection service
// Follows OOB pattern: auto-configures from environment variables
func NewMongoDBConnectionService() (*MongoDBConnectionService, error) {
	config := loadMongoDBConfigFromEnv()

	if config.URI == "" {
		return nil, fmt.Errorf("MongoDB not configured: MONGODB_URI environment variable not set")
	}

	service := &MongoDBConnectionService{
		uri:         config.URI,
		database:    config.Database,
		isConnected: false,
	}

	return service, nil
}

// loadMongoDBConfigFromEnv loads MongoDB configuration from environment variables (OOB)
func loadMongoDBConfigFromEnv() *MongoDBConfig {
	return &MongoDBConfig{
		URI:      os.Getenv("MONGODB_URI"),
		Database: getEnvOrDefault("MONGODB_DATABASE", "ezhealthkonnect"),
	}
}

// Connect establishes connection to MongoDB
func (mcs *MongoDBConnectionService) Connect(ctx context.Context) error {
	if mcs.isConnected {
		return nil // Already connected
	}

	fmt.Printf("🔄 Connecting to MongoDB...\n")

	clientOptions := options.Client().ApplyURI(mcs.uri)
	clientOptions.SetServerSelectionTimeout(5 * time.Second)
	clientOptions.SetConnectTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Verify connection with ping
	err = client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	mcs.client = client
	mcs.isConnected = true

	fmt.Printf("✅ MongoDB connected successfully (database: %s)\n", mcs.database)
	return nil
}

// GetClient returns the MongoDB client
func (mcs *MongoDBConnectionService) GetClient() *mongo.Client {
	return mcs.client
}

// GetDatabase returns the database name
func (mcs *MongoDBConnectionService) GetDatabase() string {
	return mcs.database
}

// IsConnected returns whether MongoDB is connected
func (mcs *MongoDBConnectionService) IsConnected() bool {
	return mcs.isConnected
}

// Disconnect closes the MongoDB connection
func (mcs *MongoDBConnectionService) Disconnect(ctx context.Context) error {
	if !mcs.isConnected {
		return nil
	}

	if mcs.client != nil {
		err := mcs.client.Disconnect(ctx)
		if err != nil {
			return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
		}
	}

	mcs.isConnected = false
	fmt.Printf("🔌 MongoDB disconnected\n")
	return nil
}

// HealthCheck performs a health check on MongoDB connection
func (mcs *MongoDBConnectionService) HealthCheck(ctx context.Context) error {
	if !mcs.isConnected || mcs.client == nil {
		return fmt.Errorf("MongoDB not connected")
	}

	err := mcs.client.Ping(ctx, nil)
	if err != nil {
		return fmt.Errorf("MongoDB health check failed: %w", err)
	}

	return nil
}

// getEnvOrDefault gets environment variable or returns default value
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
