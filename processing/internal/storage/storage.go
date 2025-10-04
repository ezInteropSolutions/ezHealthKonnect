// internal/storage/storage.go
// Universal storage abstraction for MongoDB and PostgreSQL

package storage

import (
	"context"
	"time"

	"ezhealthkonnect/processing/pkg"
)

// StorageProvider defines the unified storage interface
type StorageProvider interface {
	// Connection management
	Connect(ctx context.Context) error
	Disconnect() error
	Ping(ctx context.Context) error
	IsHealthy() bool

	// Message operations
	StoreMessage(ctx context.Context, message *pkg.UniversalMessage) error
	GetMessage(ctx context.Context, messageID string) (*pkg.UniversalMessage, error)
	UpdateMessage(ctx context.Context, message *pkg.UniversalMessage) error
	DeleteMessage(ctx context.Context, messageID string) error

	// Message queries
	FindMessages(ctx context.Context, filter MessageFilter) ([]*pkg.UniversalMessage, error)
	CountMessages(ctx context.Context, filter MessageFilter) (int64, error)
	FindMessagesByStatus(ctx context.Context, status pkg.MessageStatus, limit int) ([]*pkg.UniversalMessage, error)
	FindMessagesByInterface(ctx context.Context, interfaceID string, limit int) ([]*pkg.UniversalMessage, error)

	// Lineage operations
	StoreLineage(ctx context.Context, lineage *pkg.MessageLineage) error
	GetLineage(ctx context.Context, correlationID string) (*pkg.MessageLineage, error)
	GetMessageLineage(ctx context.Context, messageID string) ([]*pkg.MessageLineage, error)

	// Interface configuration
	StoreInterface(ctx context.Context, interfaceConfig *pkg.InterfaceDefinition) error
	GetInterface(ctx context.Context, interfaceID string) (*pkg.InterfaceDefinition, error)
	UpdateInterface(ctx context.Context, interfaceConfig *pkg.InterfaceDefinition) error
	DeleteInterface(ctx context.Context, interfaceID string) error
	ListInterfaces(ctx context.Context) ([]*pkg.InterfaceDefinition, error)

	// Routing rules
	StoreRoutingRule(ctx context.Context, rule *pkg.RoutingRule) error
	GetRoutingRules(ctx context.Context, interfaceID string) ([]*pkg.RoutingRule, error)
	UpdateRoutingRule(ctx context.Context, rule *pkg.RoutingRule) error
	DeleteRoutingRule(ctx context.Context, ruleID string) error

	// Transformation configurations
	StoreTransformation(ctx context.Context, transform *pkg.TransformationConfiguration) error
	GetTransformation(ctx context.Context, transformID string) (*pkg.TransformationConfiguration, error)
	ListTransformations(ctx context.Context) ([]*pkg.TransformationConfiguration, error)

	// Metrics and analytics
	GetProcessingMetrics(ctx context.Context, interfaceID string, timeRange TimeRange) (*pkg.ProcessingMetrics, error)
	StoreMetrics(ctx context.Context, metrics *pkg.ProcessingMetrics) error

	// Maintenance operations
	CleanupExpiredMessages(ctx context.Context, retentionPeriod time.Duration) (int64, error)
	ArchiveMessages(ctx context.Context, filter MessageFilter) (int64, error)
	GetStorageStatistics(ctx context.Context) (*StorageStatistics, error)

	// Transaction support
	BeginTransaction(ctx context.Context) (Transaction, error)

	// Provider-specific information
	GetProviderType() ProviderType
	GetConnectionInfo() map[string]interface{}
}

// Transaction interface for transactional operations
type Transaction interface {
	Commit() error
	Rollback() error
	StoreMessage(ctx context.Context, message *pkg.UniversalMessage) error
	UpdateMessage(ctx context.Context, message *pkg.UniversalMessage) error
}

// MessageFilter defines filtering criteria for message queries
type MessageFilter struct {
	MessageIDs      []string            `json:"message_ids,omitempty"`
	CorrelationIDs  []string            `json:"correlation_ids,omitempty"`
	InterfaceIDs    []string            `json:"interface_ids,omitempty"`
	Statuses        []pkg.MessageStatus `json:"statuses,omitempty"`
	ContentTypes    []string            `json:"content_types,omitempty"`
	SourceSystems   []string            `json:"source_systems,omitempty"`
	TargetSystems   []string            `json:"target_systems,omitempty"`

	// Time range filters
	ReceivedAfter   *time.Time `json:"received_after,omitempty"`
	ReceivedBefore  *time.Time `json:"received_before,omitempty"`
	ProcessedAfter  *time.Time `json:"processed_after,omitempty"`
	ProcessedBefore *time.Time `json:"processed_before,omitempty"`

	// Content filters
	MessageTypes    []string `json:"message_types,omitempty"`
	PatientIDs      []string `json:"patient_ids,omitempty"`
	VisitIDs        []string `json:"visit_ids,omitempty"`

	// Processing filters
	ErrorsOnly      bool `json:"errors_only,omitempty"`
	FailedOnly      bool `json:"failed_only,omitempty"`
	RetryableOnly   bool `json:"retryable_only,omitempty"`

	// Pagination
	Offset          int `json:"offset,omitempty"`
	Limit           int `json:"limit,omitempty"`
	SortBy          string `json:"sort_by,omitempty"`
	SortOrder       SortOrder `json:"sort_order,omitempty"`

	// Custom filters
	CustomFilters   map[string]interface{} `json:"custom_filters,omitempty"`
}

// TimeRange defines a time range for queries
type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// SortOrder defines sort order
type SortOrder string

const (
	SortAscending  SortOrder = "asc"
	SortDescending SortOrder = "desc"
)

// ProviderType defines the storage provider type
type ProviderType string

const (
	ProviderPostgreSQL ProviderType = "postgresql"
	ProviderMongoDB    ProviderType = "mongodb"
	ProviderHybrid     ProviderType = "hybrid"
)

// StorageStatistics provides storage usage statistics
type StorageStatistics struct {
	TotalMessages     int64             `json:"total_messages"`
	MessagesByStatus  map[string]int64  `json:"messages_by_status"`
	MessagesByType    map[string]int64  `json:"messages_by_type"`
	StorageSize       int64             `json:"storage_size_bytes"`
	IndexSize         int64             `json:"index_size_bytes"`
	OldestMessage     *time.Time        `json:"oldest_message,omitempty"`
	NewestMessage     *time.Time        `json:"newest_message,omitempty"`
	AverageMessageSize int64            `json:"average_message_size_bytes"`
	CollectionStats   map[string]interface{} `json:"collection_stats,omitempty"`
}

// StorageConfiguration defines storage configuration
type StorageConfiguration struct {
	ProviderType     ProviderType     `json:"provider_type"`
	PostgresConfig   *PostgresConfig  `json:"postgres_config,omitempty"`
	MongoConfig      *MongoConfig     `json:"mongo_config,omitempty"`
	RetentionPolicy  *RetentionPolicy `json:"retention_policy,omitempty"`
	PerformanceConfig *PerformanceConfig `json:"performance_config,omitempty"`
}

// PostgresConfig defines PostgreSQL-specific configuration
type PostgresConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Database        string        `json:"database"`
	Username        string        `json:"username"`
	Password        string        `json:"password"`
	SSLMode         string        `json:"ssl_mode"`
	MaxConnections  int           `json:"max_connections"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	Schema          string        `json:"schema"`
}

// MongoConfig defines MongoDB-specific configuration
type MongoConfig struct {
	URI             string        `json:"uri"`
	Database        string        `json:"database"`
	Collection      string        `json:"collection"`
	MaxPoolSize     int           `json:"max_pool_size"`
	MinPoolSize     int           `json:"min_pool_size"`
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time"`
	AuthDatabase    string        `json:"auth_database,omitempty"`
}

// RetentionPolicy defines data retention rules
type RetentionPolicy struct {
	MessageRetention    time.Duration `json:"message_retention"`
	LineageRetention    time.Duration `json:"lineage_retention"`
	MetricsRetention    time.Duration `json:"metrics_retention"`
	ArchiveBeforeDelete bool          `json:"archive_before_delete"`
	ArchiveLocation     string        `json:"archive_location,omitempty"`
}

// PerformanceConfig defines performance-related settings
type PerformanceConfig struct {
	BatchSize           int           `json:"batch_size"`
	BulkOperationSize   int           `json:"bulk_operation_size"`
	QueryTimeout        time.Duration `json:"query_timeout"`
	ConnectionTimeout   time.Duration `json:"connection_timeout"`
	EnableQueryLogging  bool          `json:"enable_query_logging"`
	OptimizeForReads    bool          `json:"optimize_for_reads"`
	OptimizeForWrites   bool          `json:"optimize_for_writes"`
}

// StorageManager manages multiple storage providers
type StorageManager struct {
	providers  map[ProviderType]StorageProvider
	primary    StorageProvider
	secondary  StorageProvider
	config     *StorageConfiguration
}

// NewStorageManager creates a new storage manager
func NewStorageManager(config *StorageConfiguration) *StorageManager {
	return &StorageManager{
		providers: make(map[ProviderType]StorageProvider),
		config:    config,
	}
}

// RegisterProvider registers a storage provider
func (sm *StorageManager) RegisterProvider(providerType ProviderType, provider StorageProvider) {
	sm.providers[providerType] = provider
}

// SetPrimaryProvider sets the primary storage provider
func (sm *StorageManager) SetPrimaryProvider(providerType ProviderType) error {
	provider, exists := sm.providers[providerType]
	if !exists {
		return ErrProviderNotFound
	}
	sm.primary = provider
	return nil
}

// SetSecondaryProvider sets the secondary storage provider for replication
func (sm *StorageManager) SetSecondaryProvider(providerType ProviderType) error {
	provider, exists := sm.providers[providerType]
	if !exists {
		return ErrProviderNotFound
	}
	sm.secondary = provider
	return nil
}

// GetPrimaryProvider returns the primary storage provider
func (sm *StorageManager) GetPrimaryProvider() StorageProvider {
	return sm.primary
}

// GetSecondaryProvider returns the secondary storage provider
func (sm *StorageManager) GetSecondaryProvider() StorageProvider {
	return sm.secondary
}

// GetProvider returns a specific storage provider
func (sm *StorageManager) GetProvider(providerType ProviderType) (StorageProvider, error) {
	provider, exists := sm.providers[providerType]
	if !exists {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

// Connect connects to all registered providers
func (sm *StorageManager) Connect(ctx context.Context) error {
	for providerType, provider := range sm.providers {
		if err := provider.Connect(ctx); err != nil {
			return &StorageError{
				Type:         ErrorTypeConnection,
				Provider:     providerType,
				Operation:    "connect",
				Message:      "failed to connect to provider",
				OriginalError: err,
			}
		}
	}
	return nil
}

// Disconnect disconnects from all providers
func (sm *StorageManager) Disconnect() error {
	var lastError error
	for _, provider := range sm.providers {
		if err := provider.Disconnect(); err != nil {
			lastError = err
		}
	}
	return lastError
}

// HealthCheck checks the health of all providers
func (sm *StorageManager) HealthCheck(ctx context.Context) map[ProviderType]bool {
	health := make(map[ProviderType]bool)
	for providerType, provider := range sm.providers {
		health[providerType] = provider.IsHealthy()
	}
	return health
}

// StoreMessage stores a message using primary provider with optional replication
func (sm *StorageManager) StoreMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	if sm.primary == nil {
		return ErrNoPrimaryProvider
	}

	// Store in primary
	if err := sm.primary.StoreMessage(ctx, message); err != nil {
		return &StorageError{
			Type:         ErrorTypeWrite,
			Provider:     sm.primary.GetProviderType(),
			Operation:    "store_message",
			Message:      "failed to store message in primary provider",
			OriginalError: err,
		}
	}

	// Replicate to secondary if available
	if sm.secondary != nil {
		go func() {
			// Use background context for replication
			bgCtx := context.Background()
			if err := sm.secondary.StoreMessage(bgCtx, message); err != nil {
				// Log replication error but don't fail the primary operation
				// TODO: Add proper logging
			}
		}()
	}

	return nil
}

// GetMessage retrieves a message from primary provider with fallback
func (sm *StorageManager) GetMessage(ctx context.Context, messageID string) (*pkg.UniversalMessage, error) {
	if sm.primary == nil {
		return nil, ErrNoPrimaryProvider
	}

	// Try primary first
	message, err := sm.primary.GetMessage(ctx, messageID)
	if err == nil {
		return message, nil
	}

	// Fallback to secondary if available
	if sm.secondary != nil {
		return sm.secondary.GetMessage(ctx, messageID)
	}

	return nil, err
}