// processing/config_manager.go
// MongoDB Configuration Manager for Interface-Centric Processing Engine

package processing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ConfigurationManager manages interface configurations with MongoDB storage and hot-reload capability
type ConfigurationManager struct {
	client          *mongo.Client
	database        *mongo.Database
	configCache     map[string]*InterfaceConfiguration
	cacheMutex      sync.RWMutex
	changeStream    *mongo.ChangeStream
	hotReloadActive bool
	ctx             context.Context
}

// NewConfigurationManager creates a new configuration manager
func NewConfigurationManager(mongoURI string, databaseName string) (*ConfigurationManager, error) {
	ctx := context.Background()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	database := client.Database(databaseName)

	// Create indexes for performance
	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "interface_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err = database.Collection("interface_configurations").Indexes().CreateMany(ctx, indexModels)
	if err != nil {
		log.Printf("Warning: Failed to create indexes: %v", err)
	}

	cm := &ConfigurationManager{
		client:      client,
		database:    database,
		configCache: make(map[string]*InterfaceConfiguration),
		ctx:         ctx,
	}

	// Load initial configurations
	if err := cm.LoadAllConfigurations(); err != nil {
		return nil, fmt.Errorf("failed to load initial configurations: %v", err)
	}

	return cm, nil
}

// InterfaceConfiguration represents a complete interface configuration
type InterfaceConfiguration struct {
	InterfaceID    string                `bson:"interface_id" json:"interface_id"`
	Name           string                `bson:"name" json:"name"`
	Description    string                `bson:"description" json:"description"`
	Status         string                `bson:"status" json:"status"` // active, inactive, testing
	Version        string                `bson:"version" json:"version"`

	// Processing Pipeline Configuration
	InputLayer         InputLayerConfig         `bson:"input_layer" json:"input_layer"`
	ValidationLayer    ValidationLayerConfig    `bson:"validation_layer" json:"validation_layer"`
	TransformationLayer TransformationLayerConfig `bson:"transformation_layer" json:"transformation_layer"`
	BusinessLayer      BusinessLayerConfig      `bson:"business_layer" json:"business_layer"`
	DestinationLayer   DestinationLayerConfig   `bson:"destination_layer" json:"destination_layer"`

	// Metadata
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at" json:"updated_at"`
	CreatedBy     string    `bson:"created_by" json:"created_by"`
	UpdatedBy     string    `bson:"updated_by" json:"updated_by"`
}

// Layer Configuration Types
type InputLayerConfig struct {
	Protocol      string                 `bson:"protocol" json:"protocol"` // mllp, http, file, tcp
	Port          int                    `bson:"port" json:"port"`
	Encoding      string                 `bson:"encoding" json:"encoding"`
	Preprocessing PreprocessingConfig    `bson:"preprocessing" json:"preprocessing"`
}

type ValidationLayerConfig struct {
	SchemaValidation SchemaValidationConfig `bson:"schema_validation" json:"schema_validation"`
	BusinessRules    []ProcessingBusinessRule         `bson:"business_rules" json:"business_rules"`
	CustomValidators []ProcessingCustomValidator      `bson:"custom_validators" json:"custom_validators"`
}

type TransformationLayerConfig struct {
	Engine           string                     `bson:"engine" json:"engine"` // hl7_to_fhir, custom, passthrough
	MappingTemplate  string                     `bson:"mapping_template" json:"mapping_template"`
	CustomMappings   []ProcessingFieldMapping         `bson:"custom_mappings" json:"custom_mappings"`
	PostProcessing   []PostProcessingStep       `bson:"post_processing" json:"post_processing"`
}

type BusinessLayerConfig struct {
	Rules        []ProcessingBusinessRule `bson:"rules" json:"rules"`
	Workflows    []WorkflowStep     `bson:"workflows" json:"workflows"`
	Integration  IntegrationConfig  `bson:"integration" json:"integration"`
}

type DestinationLayerConfig struct {
	Endpoints   []DestinationEndpoint `bson:"endpoints" json:"endpoints"`
	Routing     RoutingConfig         `bson:"routing" json:"routing"`
	Persistence PersistenceConfig     `bson:"persistence" json:"persistence"`
}

// Configuration Sub-types
type PreprocessingConfig struct {
	Enabled bool                   `bson:"enabled" json:"enabled"`
	Steps   []PreprocessingStep    `bson:"steps" json:"steps"`
}

type PreprocessingStep struct {
	Type   string                 `bson:"type" json:"type"`
	Config map[string]interface{} `bson:"config" json:"config"`
}

type SchemaValidationConfig struct {
	Enabled    bool   `bson:"enabled" json:"enabled"`
	SchemaType string `bson:"schema_type" json:"schema_type"`
	StrictMode bool   `bson:"strict_mode" json:"strict_mode"`
}

type ProcessingBusinessRule struct {
	RuleID       string `bson:"rule_id" json:"rule_id"`
	Condition    string `bson:"condition" json:"condition"`
	Validation   string `bson:"validation" json:"validation"`
	ErrorMessage string `bson:"error_message" json:"error_message"`
	Severity     string `bson:"severity" json:"severity"`
}

type ProcessingCustomValidator struct {
	Name string `bson:"name" json:"name"`
	Type string `bson:"type" json:"type"`
	Code string `bson:"code" json:"code"`
}

type ProcessingFieldMapping struct {
	SourceField    string                 `bson:"source_field" json:"source_field"`
	TargetField    string                 `bson:"target_field" json:"target_field"`
	Transformation BasicTransformationStep     `bson:"transformation" json:"transformation"`
}

type BasicTransformationStep struct {
	Type   string                 `bson:"type" json:"type"`
	Config map[string]interface{} `bson:"config" json:"config"`
}

type PostProcessingStep struct {
	Type   string                 `bson:"type" json:"type"`
	Config map[string]interface{} `bson:"config" json:"config"`
}

type WorkflowStep struct {
	StepID    string                 `bson:"step_id" json:"step_id"`
	Type      string                 `bson:"type" json:"type"`
	Condition string                 `bson:"condition" json:"condition"`
	Action    map[string]interface{} `bson:"action" json:"action"`
}

type IntegrationConfig struct {
	ExternalSystems []ExternalSystemConfig `bson:"external_systems" json:"external_systems"`
	APIs           []APIConfig            `bson:"apis" json:"apis"`
}

type ExternalSystemConfig struct {
	SystemID string                 `bson:"system_id" json:"system_id"`
	Type     string                 `bson:"type" json:"type"`
	Config   map[string]interface{} `bson:"config" json:"config"`
}

type APIConfig struct {
	Name     string            `bson:"name" json:"name"`
	URL      string            `bson:"url" json:"url"`
	Method   string            `bson:"method" json:"method"`
	Headers  map[string]string `bson:"headers" json:"headers"`
}

type DestinationEndpoint struct {
	EndpointID string                 `bson:"endpoint_id" json:"endpoint_id"`
	Type       string                 `bson:"type" json:"type"` // http, mllp, file, database
	Address    string                 `bson:"address" json:"address"`
	Config     map[string]interface{} `bson:"config" json:"config"`
}

type RoutingConfig struct {
	Strategy string                 `bson:"strategy" json:"strategy"` // broadcast, conditional, round_robin
	Rules    []RoutingRule          `bson:"rules" json:"rules"`
}

type RoutingRule struct {
	Condition   string   `bson:"condition" json:"condition"`
	Destinations []string `bson:"destinations" json:"destinations"`
}

type PersistenceConfig struct {
	Enabled    bool   `bson:"enabled" json:"enabled"`
	Collection string `bson:"collection" json:"collection"`
	TTL        int    `bson:"ttl" json:"ttl"` // seconds
}

// Core Configuration Methods
func (cm *ConfigurationManager) SaveConfiguration(config *InterfaceConfiguration) error {
	cm.cacheMutex.Lock()
	defer cm.cacheMutex.Unlock()

	config.UpdatedAt = time.Now()

	collection := cm.database.Collection("interface_configurations")

	filter := bson.M{"interface_id": config.InterfaceID}
	update := bson.M{"$set": config}
	opts := options.Update().SetUpsert(true)

	_, err := collection.UpdateOne(cm.ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to save configuration: %v", err)
	}

	// Update cache
	cm.configCache[config.InterfaceID] = config

	log.Printf("Configuration saved for interface %s", config.InterfaceID)
	return nil
}

func (cm *ConfigurationManager) GetConfiguration(interfaceID string) (*InterfaceConfiguration, error) {
	cm.cacheMutex.RLock()
	defer cm.cacheMutex.RUnlock()

	if config, exists := cm.configCache[interfaceID]; exists {
		return config, nil
	}

	// Not in cache, load from database
	return cm.loadConfigurationFromDB(interfaceID)
}

func (cm *ConfigurationManager) GetAllConfigurations() map[string]*InterfaceConfiguration {
	cm.cacheMutex.RLock()
	defer cm.cacheMutex.RUnlock()

	// Return a copy to prevent external modifications
	result := make(map[string]*InterfaceConfiguration)
	for k, v := range cm.configCache {
		result[k] = v
	}
	return result
}

func (cm *ConfigurationManager) DeleteConfiguration(interfaceID string) error {
	cm.cacheMutex.Lock()
	defer cm.cacheMutex.Unlock()

	collection := cm.database.Collection("interface_configurations")
	filter := bson.M{"interface_id": interfaceID}

	_, err := collection.DeleteOne(cm.ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete configuration: %v", err)
	}

	// Remove from cache
	delete(cm.configCache, interfaceID)

	log.Printf("Configuration deleted for interface %s", interfaceID)
	return nil
}

func (cm *ConfigurationManager) LoadAllConfigurations() error {
	collection := cm.database.Collection("interface_configurations")

	cursor, err := collection.Find(cm.ctx, bson.M{})
	if err != nil {
		return fmt.Errorf("failed to load configurations: %v", err)
	}
	defer cursor.Close(cm.ctx)

	cm.cacheMutex.Lock()
	defer cm.cacheMutex.Unlock()

	// Clear existing cache
	cm.configCache = make(map[string]*InterfaceConfiguration)

	for cursor.Next(cm.ctx) {
		var config InterfaceConfiguration
		if err := cursor.Decode(&config); err != nil {
			log.Printf("Warning: Failed to decode configuration: %v", err)
			continue
		}
		cm.configCache[config.InterfaceID] = &config
	}

	log.Printf("Loaded %d interface configurations", len(cm.configCache))
	return nil
}

func (cm *ConfigurationManager) loadConfigurationFromDB(interfaceID string) (*InterfaceConfiguration, error) {
	collection := cm.database.Collection("interface_configurations")

	var config InterfaceConfiguration
	err := collection.FindOne(cm.ctx, bson.M{"interface_id": interfaceID}).Decode(&config)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("configuration not found for interface %s", interfaceID)
		}
		return nil, fmt.Errorf("failed to load configuration: %v", err)
	}

	// Update cache
	cm.cacheMutex.Lock()
	cm.configCache[interfaceID] = &config
	cm.cacheMutex.Unlock()

	return &config, nil
}

// Hot-reload functionality using MongoDB Change Streams
func (cm *ConfigurationManager) StartHotReload() error {
	if cm.hotReloadActive {
		return nil // Already running
	}

	collection := cm.database.Collection("interface_configurations")
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "operationType", Value: bson.D{{Key: "$in", Value: bson.A{"insert", "update", "delete", "replace"}}}}}}},
	}

	var err error
	cm.changeStream, err = collection.Watch(cm.ctx, pipeline)
	if err != nil {
		return fmt.Errorf("failed to create change stream: %v", err)
	}

	cm.hotReloadActive = true

	go cm.processChangeStream()

	log.Println("Hot-reload activated for interface configurations")
	return nil
}

func (cm *ConfigurationManager) StopHotReload() error {
	if !cm.hotReloadActive || cm.changeStream == nil {
		return nil
	}

	cm.hotReloadActive = false
	err := cm.changeStream.Close(cm.ctx)
	cm.changeStream = nil

	log.Println("Hot-reload deactivated")
	return err
}

func (cm *ConfigurationManager) processChangeStream() {
	for cm.hotReloadActive && cm.changeStream.Next(cm.ctx) {
		var changeDoc bson.M
		if err := cm.changeStream.Decode(&changeDoc); err != nil {
			log.Printf("Error decoding change stream document: %v", err)
			continue
		}

		operationType := changeDoc["operationType"].(string)

		switch operationType {
		case "insert", "update", "replace":
			if fullDoc, ok := changeDoc["fullDocument"]; ok {
				var config InterfaceConfiguration
				docBytes, _ := bson.Marshal(fullDoc)
				if err := bson.Unmarshal(docBytes, &config); err != nil {
					log.Printf("Error unmarshaling configuration: %v", err)
					continue
				}

				cm.cacheMutex.Lock()
				cm.configCache[config.InterfaceID] = &config
				cm.cacheMutex.Unlock()

				log.Printf("Configuration updated for interface %s via hot-reload", config.InterfaceID)
			}
		case "delete":
			if docKey, ok := changeDoc["documentKey"]; ok {
				if keyDoc, ok := docKey.(bson.M); ok {
					if interfaceID, ok := keyDoc["interface_id"].(string); ok {
						cm.cacheMutex.Lock()
						delete(cm.configCache, interfaceID)
						cm.cacheMutex.Unlock()

						log.Printf("Configuration deleted for interface %s via hot-reload", interfaceID)
					}
				}
			}
		}
	}
}

// Validation Methods
func (cm *ConfigurationManager) ValidateConfiguration(config *InterfaceConfiguration) []string {
	var errors []string

	if config.InterfaceID == "" {
		errors = append(errors, "interface_id is required")
	}

	if config.Name == "" {
		errors = append(errors, "name is required")
	}

	if config.InputLayer.Protocol == "" {
		errors = append(errors, "input_layer.protocol is required")
	}

	if config.InputLayer.Protocol == "mllp" || config.InputLayer.Protocol == "tcp" {
		if config.InputLayer.Port == 0 {
			errors = append(errors, "input_layer.port is required for MLLP/TCP protocols")
		}
	}

	return errors
}

func (cm *ConfigurationManager) GetConfigurationStats() map[string]interface{} {
	cm.cacheMutex.RLock()
	defer cm.cacheMutex.RUnlock()

	stats := map[string]interface{}{
		"total_configurations": len(cm.configCache),
		"active_configurations": 0,
		"inactive_configurations": 0,
		"hot_reload_active": cm.hotReloadActive,
	}

	for _, config := range cm.configCache {
		if config.Status == "active" {
			stats["active_configurations"] = stats["active_configurations"].(int) + 1
		} else {
			stats["inactive_configurations"] = stats["inactive_configurations"].(int) + 1
		}
	}

	return stats
}

// Close cleans up resources
func (cm *ConfigurationManager) Close() error {
	if err := cm.StopHotReload(); err != nil {
		log.Printf("Error stopping hot-reload: %v", err)
	}

	if cm.client != nil {
		return cm.client.Disconnect(cm.ctx)
	}

	return nil
}