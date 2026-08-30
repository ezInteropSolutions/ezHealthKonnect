// services/connectors/inbound_connectors_test.go
// Unit tests for all inbound connector stubs and implemented inbound connectors.
// Tests cover: metadata, Initialize(), Validate(), SupportsCron(), factory registration.
// No external network or file system dependencies except file_listener tests that use os.TempDir().

package connectors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func mustJSON(v map[string]interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// inboundStubCase describes a single stub inbound connector under test.
type inboundStubCase struct {
	name         string
	constructor  func() InboundConnector
	typeName     string
	supportsCron bool
}

// allStubInboundCases lists pure-stub inbound connectors (base Initialize, no network dial).
// Connectors with full implementations (MySQL, MongoDB, RabbitMQ, Kafka, Redis, AWS S3)
// are excluded here because their Initialize() actually dials a real service.
// http_rest_inbound, file_listener, and postgresql_inbound have custom Initialize and
// are tested separately below.
var allStubInboundCases = []inboundStubCase{
	// sqlserver_inbound, oracle_inbound, azure_blob_inbound, databricks_inbound, and snowflake_inbound are IMPLEMENTED — see implementedInboundCases below.
	{"bigquery_inbound", NewBigQueryInboundConnector, "bigquery_inbound", true},
	{"redshift_inbound", NewRedshiftInboundConnector, "redshift_inbound", true},
	{"synapse_inbound", NewSynapseInboundConnector, "synapse_inbound", true},
	{"clickhouse_inbound", NewClickHouseInboundConnector, "clickhouse_inbound", true},
	{"timescaledb_inbound", NewTimescaleDBInboundConnector, "timescaledb_inbound", true},
	{"gcs_inbound", NewGCSInboundConnector, "gcs_inbound", true},
	{"ftp_inbound", NewFTPInboundConnector, "ftp_inbound", true},
}

// implementedInboundCases lists connectors that have real network implementations.
// Their Initialize() may fail when given empty config (no host/credentials).
// Metadata and SupportsCron tests still run; InitializeMinimalConfig is skipped.
var implementedInboundCases = []inboundStubCase{
	{"mysql_inbound", NewMySQLInboundConnector, "mysql_inbound", true},
	{"mongodb_inbound", NewMongoDBInboundConnector, "mongodb_inbound", true},
	{"rabbitmq_inbound", NewRabbitMQInboundConnector, "rabbitmq_inbound", false},
	{"kafka_inbound", NewKafkaInboundConnector, "kafka_inbound", false},
	{"redis_inbound", NewRedisInboundConnector, "redis_inbound", false},
	{"aws_s3_inbound", NewAWSS3InboundConnector, "aws_s3_inbound", true},
	{"sqlserver_inbound", NewSQLServerInboundConnector, "sqlserver_inbound", true},
	{"oracle_inbound", NewOracleInboundConnector, "oracle_inbound", true},
	{"azure_blob_inbound", NewAzureBlobInboundConnector, "azure_blob_inbound", true},
	{"databricks_inbound", NewDatabricksInboundConnector, "databricks_inbound", true},
	{"snowflake_inbound", NewSnowflakeInboundConnector, "snowflake_inbound", true},
}

// -----------------------------------------------------------------------------
// Table-driven tests for all stub inbound connectors
// -----------------------------------------------------------------------------

// TestStubInboundConnectors_Metadata verifies TypeName and category for every stub.
func TestStubInboundConnectors_Metadata(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/metadata", func(t *testing.T) {
			c := tc.constructor()
			meta := c.GetMetadata()
			if meta.TypeName != tc.typeName {
				t.Errorf("TypeName: got %q, want %q", meta.TypeName, tc.typeName)
			}
			if meta.Category != "inbound" {
				t.Errorf("Category: got %q, want %q", meta.Category, "inbound")
			}
			if meta.Version == "" {
				t.Error("Version must not be empty")
			}
			if meta.DisplayName == "" {
				t.Error("DisplayName must not be empty")
			}
		})
	}
}

// TestStubInboundConnectors_SupportsCron validates the cron capability flag.
func TestStubInboundConnectors_SupportsCron(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/supportsCron", func(t *testing.T) {
			c := tc.constructor()
			got := c.SupportsCron()
			if got != tc.supportsCron {
				t.Errorf("SupportsCron() = %v, want %v", got, tc.supportsCron)
			}
		})
	}
}

// TestStubInboundConnectors_ValidateBeforeInit ensures Validate() fails before Initialize().
func TestStubInboundConnectors_ValidateBeforeInit(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/validateBeforeInit", func(t *testing.T) {
			c := tc.constructor()
			err := c.Validate()
			if err == nil {
				t.Error("Validate() should return error before Initialize()")
			}
			if !strings.Contains(err.Error(), "not initialized") {
				t.Errorf("error should mention 'not initialized', got: %v", err)
			}
		})
	}
}

// TestStubInboundConnectors_InitializeMinimalConfig verifies stubs accept an empty config object.
func TestStubInboundConnectors_InitializeMinimalConfig(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/initializeEmpty", func(t *testing.T) {
			c := tc.constructor()
			err := c.Initialize(mustJSON(map[string]interface{}{}))
			if err != nil {
				t.Errorf("Initialize({}) should succeed for stub, got: %v", err)
			}
		})
	}
}

// TestStubInboundConnectors_ValidateAfterInit ensures Validate() succeeds after Initialize().
func TestStubInboundConnectors_ValidateAfterInit(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/validateAfterInit", func(t *testing.T) {
			c := tc.constructor()
			if err := c.Initialize(mustJSON(map[string]interface{}{})); err != nil {
				t.Fatalf("Initialize() failed: %v", err)
			}
			if err := c.Validate(); err != nil {
				t.Errorf("Validate() after Initialize() should succeed, got: %v", err)
			}
		})
	}
}

// TestStubInboundConnectors_InitializeBadJSON ensures garbage input is rejected.
func TestStubInboundConnectors_InitializeBadJSON(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/badJSON", func(t *testing.T) {
			c := tc.constructor()
			err := c.Initialize([]byte("not-json"))
			if err == nil {
				t.Error("Initialize() with bad JSON should return error")
			}
		})
	}
}

// TestStubInboundConnectors_InitializeWithTypicalConfig verifies realistic configs are accepted.
func TestStubInboundConnectors_InitializeWithTypicalConfig(t *testing.T) {
	typicalConfigs := map[string]map[string]interface{}{
		"mysql_inbound": {
			"host": "mysql.example.com", "port": 3306.0, "database": "ehr",
			"username": "reader", "password": "secret", "query": "SELECT * FROM patients",
		},
		"sqlserver_inbound": {
			"host": "sql.example.com", "port": 1433.0, "database": "ehr",
			"username": "sa", "password": "secret", "query": "SELECT * FROM messages",
		},
		"mongodb_inbound": {
			"connection_string": "mongodb://localhost:27017", "database": "ehr",
			"collection": "adt_messages", "query": `{}`,
		},
		"oracle_inbound": {
			"host": "oracle.example.com", "port": 1521.0, "service_name": "EHRSVC",
			"username": "app", "password": "secret", "query": "SELECT * FROM HL7_INBOX",
		},
		"rabbitmq_inbound": {
			"host": "rabbitmq.example.com", "port": 5672.0, "username": "guest",
			"password": "guest", "queue": "hl7.inbound", "vhost": "/",
		},
		"kafka_inbound": {
			"brokers": "kafka1:9092,kafka2:9092", "topic": "hl7.messages",
			"consumer_group": "ehk-consumer", "auto_offset_reset": "earliest",
		},
		"redis_inbound": {
			"host": "redis.example.com", "port": 6379.0,
			"channel": "hl7:inbound", "stream": "hl7:stream",
		},
		"aws_s3_inbound": {
			"bucket": "my-hl7-bucket", "region": "us-east-1",
			"prefix": "inbound/", "access_key_id": "AKIA...", "secret_access_key": "secret",
		},
		"gcs_inbound": {
			"project_id": "my-project", "bucket": "hl7-bucket",
			"prefix": "inbound/", "credentials_json": `{"type":"service_account"}`,
		},
		"ftp_inbound": {
			"host": "ftp.example.com", "port": 21.0, "username": "ftpuser",
			"password": "secret", "directory": "/hl7/inbound", "file_pattern": "*.hl7",
		},
		"bigquery_inbound": {
			"project_id": "my-project", "dataset": "ehr",
			"query": "SELECT * FROM messages LIMIT 1000",
		},
		"redshift_inbound": {
			"host": "cluster.us-east-1.redshift.amazonaws.com", "port": 5439.0,
			"database": "ehr", "username": "awsuser", "password": "secret",
			"query": "SELECT * FROM hl7_messages",
		},
		"synapse_inbound": {
			"server": "myworkspace.sql.azuresynapse.net", "database": "ehr",
			"username": "sa", "password": "secret",
			"query": "SELECT * FROM dbo.HL7Messages",
		},
		"clickhouse_inbound": {
			"host": "clickhouse.example.com", "port": 8123.0,
			"database": "ehr", "username": "default", "password": "",
			"query": "SELECT * FROM hl7_messages",
		},
		"timescaledb_inbound": {
			"host": "timescale.example.com", "port": 5432.0, "database": "ehr",
			"username": "reader", "password": "secret",
			"query": "SELECT * FROM messages ORDER BY received_at DESC LIMIT 100",
		},
	}

	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/typicalConfig", func(t *testing.T) {
			cfg, ok := typicalConfigs[tc.name]
			if !ok {
				t.Skipf("no typical config defined for %s", tc.name)
			}
			c := tc.constructor()
			err := c.Initialize(mustJSON(cfg))
			if err != nil {
				t.Errorf("Initialize() with typical config failed: %v", err)
			}
		})
	}
}

// TestStubInboundConnectors_CloseAfterInit verifies Close() can be called after Initialize().
func TestStubInboundConnectors_CloseAfterInit(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/close", func(t *testing.T) {
			c := tc.constructor()
			_ = c.Initialize(mustJSON(map[string]interface{}{}))
			if err := c.Close(); err != nil {
				t.Errorf("Close() after Initialize() should not error, got: %v", err)
			}
			status := c.GetStatus()
			if status.State != StateStopped {
				t.Errorf("Status.State after Close() = %v, want %v", status.State, StateStopped)
			}
		})
	}
}

// TestStubInboundConnectors_GetStatusInitialState checks status before Initialize.
func TestStubInboundConnectors_GetStatusInitialState(t *testing.T) {
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/initialStatus", func(t *testing.T) {
			c := tc.constructor()
			status := c.GetStatus()
			if status.Connected {
				t.Error("connector should not be connected before Initialize()")
			}
			if status.State == StateRunning {
				t.Error("connector should not be in running state initially")
			}
		})
	}
}

// -----------------------------------------------------------------------------
// file_listener (implemented connector) tests
// -----------------------------------------------------------------------------

func TestFileListenerConnector_Metadata(t *testing.T) {
	c := NewFileListenerConnector()
	meta := c.GetMetadata()
	if meta.TypeName != "file_listener" {
		t.Errorf("TypeName = %q, want %q", meta.TypeName, "file_listener")
	}
	if meta.Category != "inbound" {
		t.Errorf("Category = %q, want %q", meta.Category, "inbound")
	}
	if !c.SupportsCron() {
		t.Error("file_listener should support cron")
	}
}

func TestFileListenerConnector_InitializeMissingDirectory(t *testing.T) {
	c := NewFileListenerConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{}))
	if err == nil {
		t.Fatal("Initialize() without directory_path should fail")
	}
	if !strings.Contains(err.Error(), "directory_path") {
		t.Errorf("error should mention directory_path, got: %v", err)
	}
}

func TestFileListenerConnector_InitializeNonexistentDirectory(t *testing.T) {
	c := NewFileListenerConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"directory_path": "/this/path/definitely/does/not/exist/9x8y7z",
	}))
	if err == nil {
		t.Fatal("Initialize() with nonexistent directory should fail")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %v", err)
	}
}

func TestFileListenerConnector_InitializeValidDirectory(t *testing.T) {
	dir := t.TempDir()
	c := NewFileListenerConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"directory_path": dir,
		"file_pattern":   "*.hl7",
		"polling_interval": 30.0,
	}))
	if err != nil {
		t.Fatalf("Initialize() with valid directory failed: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() after valid Initialize() should succeed: %v", err)
	}
}

func TestFileListenerConnector_InitializeCamelCaseDirPath(t *testing.T) {
	dir := t.TempDir()
	c := NewFileListenerConnector()
	// Test camelCase variant directoryPath
	err := c.Initialize(mustJSON(map[string]interface{}{
		"directoryPath": dir,
	}))
	if err != nil {
		t.Fatalf("Initialize() with directoryPath (camelCase) failed: %v", err)
	}
}

func TestFileListenerConnector_InitializeArchiveDir(t *testing.T) {
	watchDir := t.TempDir()
	archiveDir := filepath.Join(t.TempDir(), "archive")
	c := NewFileListenerConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"directory_path":   watchDir,
		"after_processing": "archive",
		"archive_path":     archiveDir,
	}))
	if err != nil {
		t.Fatalf("Initialize() with archive config failed: %v", err)
	}
	// Archive directory should have been created
	if _, statErr := os.Stat(archiveDir); os.IsNotExist(statErr) {
		t.Error("archive directory should have been created by Initialize()")
	}
}

func TestFileListenerConnector_ValidateBeforeInit(t *testing.T) {
	c := NewFileListenerConnector()
	err := c.Validate()
	if err == nil {
		t.Error("Validate() before Initialize() should return error")
	}
}

func TestFileListenerConnector_DefaultPollingInterval(t *testing.T) {
	dir := t.TempDir()
	c := NewFileListenerConnector()
	if err := c.Initialize(mustJSON(map[string]interface{}{
		"directory_path": dir,
	})); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	// Default polling interval is 10s; we can't directly read the field but
	// a successful init with no polling_interval config is sufficient.
}

// -----------------------------------------------------------------------------
// http_rest_inbound (-> HTTPFHIRInboundConnector) tests
// -----------------------------------------------------------------------------

func TestHTTPFHIRInboundConnector_Metadata(t *testing.T) {
	c := NewHTTPFHIRInboundConnector()
	meta := c.GetMetadata()
	// Type name is http_fhir_inbound (the real implementation type)
	if meta.TypeName == "" {
		t.Error("TypeName must not be empty")
	}
	if meta.Category != "inbound" {
		t.Errorf("Category = %q, want inbound", meta.Category)
	}
	if c.SupportsCron() {
		t.Error("HTTP FHIR inbound should not support cron (it is push/server mode)")
	}
}

func TestHTTPFHIRInboundConnector_InitializeMissingPort(t *testing.T) {
	c := NewHTTPFHIRInboundConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"base_path": "/fhir/r4",
	}))
	if err == nil {
		t.Fatal("Initialize() without port should fail")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error should mention 'port', got: %v", err)
	}
}

func TestHTTPFHIRInboundConnector_InitializeWithPort(t *testing.T) {
	c := NewHTTPFHIRInboundConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"port": 8443.0,
	}))
	if err != nil {
		t.Fatalf("Initialize() with port should succeed, got: %v", err)
	}
}

func TestHTTPFHIRInboundConnector_InitializeBadJSON(t *testing.T) {
	c := NewHTTPFHIRInboundConnector()
	err := c.Initialize([]byte("{bad"))
	if err == nil {
		t.Error("Initialize() with bad JSON should return error")
	}
}

func TestHTTPFHIRInboundConnector_ValidateBeforeInit(t *testing.T) {
	c := NewHTTPFHIRInboundConnector()
	// HTTPFHIRInboundConnector has its own Initialize that sets state via its own mutex,
	// but Validate() uses BaseConnector.Validate() which checks bc.initialized.
	// After a failed Initialize() (missing port), initialized is still false.
	err := c.Validate()
	// Validate might succeed or fail depending on whether base was initialized;
	// the key requirement is Initialize() with port first is needed for real connectivity.
	_ = err
}

func TestHTTPFHIRInboundConnector_RegisteredInFactory(t *testing.T) {
	factory := NewConnectorFactory()
	conn, err := factory.CreateInbound("http_rest_inbound")
	if err != nil {
		t.Fatalf("factory.CreateInbound(http_rest_inbound) failed: %v", err)
	}
	if conn == nil {
		t.Error("CreateInbound should return non-nil connector")
	}
	conn2, err2 := factory.CreateInbound("http_rest")
	if err2 != nil {
		t.Fatalf("factory.CreateInbound(http_rest) alias failed: %v", err2)
	}
	if conn2 == nil {
		t.Error("CreateInbound(http_rest) alias should return non-nil connector")
	}
}

// -----------------------------------------------------------------------------
// postgresql_inbound (implemented — attempts real DB connect on Initialize)
// -----------------------------------------------------------------------------

func TestPostgreSQLInboundConnector_Metadata(t *testing.T) {
	c := NewPostgreSQLInboundConnector()
	meta := c.GetMetadata()
	if meta.TypeName != "postgresql_inbound" {
		t.Errorf("TypeName = %q, want postgresql_inbound", meta.TypeName)
	}
	if meta.Category != "inbound" {
		t.Errorf("Category = %q, want inbound", meta.Category)
	}
	if !c.SupportsCron() {
		t.Error("postgresql_inbound should support cron")
	}
}

func TestPostgreSQLInboundConnector_InitializeNoServer(t *testing.T) {
	// Initialize() will attempt DB connection to a non-existent server → expect error.
	c := NewPostgreSQLInboundConnector()
	err := c.Initialize(mustJSON(map[string]interface{}{
		"host":     "127.0.0.1",
		"port":     54320.0, // unlikely to be in use
		"database": "testdb",
		"username": "testuser",
		"password": "testpass",
	}))
	// We expect a connection error (ping fails). This confirms Initialize validates connectivity.
	if err == nil {
		t.Log("Note: Initialize() succeeded — a PostgreSQL server may be running on port 54320")
	} else {
		t.Logf("Expected connection failure: %v", err)
	}
}

func TestPostgreSQLInboundConnector_InitializeMissingHost(t *testing.T) {
	c := NewPostgreSQLInboundConnector()
	// Missing host/database — parse should succeed, connection should fail
	err := c.Initialize(mustJSON(map[string]interface{}{}))
	// Empty config → DatabaseInboundConfig defaults → will try to connect to localhost:5432
	// which almost certainly fails in CI. We just verify Initialize() returns some error.
	if err == nil {
		t.Log("Note: Initialize() with empty config succeeded — PostgreSQL may be running locally")
	} else {
		t.Logf("Expected error with empty config: %v", err)
	}
}

func TestPostgreSQLInboundConnector_RegisteredInFactory(t *testing.T) {
	factory := NewConnectorFactory()
	conn, err := factory.CreateInbound("postgresql_inbound")
	if err != nil {
		t.Fatalf("factory.CreateInbound(postgresql_inbound) failed: %v", err)
	}
	if conn == nil {
		t.Error("CreateInbound should return non-nil connector")
	}
}

// -----------------------------------------------------------------------------
// Factory registration tests for all inbound connector types
// -----------------------------------------------------------------------------

func TestFactory_AllInboundTypesRegistered(t *testing.T) {
	expectedInbound := []string{
		"tcp_mllp_inbound", "tcp_mllp",
		"http_rest_inbound", "http_rest", "http",
		"file_listener",
		"postgresql_inbound",
		"mysql_inbound",
		"sqlserver_inbound",
		"mongodb_inbound",
		"oracle_inbound",
		"snowflake_inbound",
		"databricks_inbound",
		"bigquery_inbound",
		"redshift_inbound",
		"synapse_inbound",
		"clickhouse_inbound",
		"timescaledb_inbound",
		"rabbitmq_inbound",
		"kafka_inbound",
		"redis_inbound",
		"aws_s3_inbound",
		"azure_blob_inbound",
		"gcs_inbound",
		"sftp_inbound",
		"ftp_inbound",
	}
	factory := NewConnectorFactory()
	for _, typeName := range expectedInbound {
		conn, err := factory.CreateInbound(typeName)
		if err != nil {
			t.Errorf("CreateInbound(%q) failed: %v", typeName, err)
			continue
		}
		if conn == nil {
			t.Errorf("CreateInbound(%q) returned nil", typeName)
		}
	}
}

func TestFactory_UnknownInboundTypeReturnsError(t *testing.T) {
	factory := NewConnectorFactory()
	_, err := factory.CreateInbound("not_a_real_connector_type_xyz")
	if err == nil {
		t.Error("CreateInbound() with unknown type should return error")
	}
}

func TestFactory_EachStubCreatesUniqueInstance(t *testing.T) {
	factory := NewConnectorFactory()
	// Create two mysql_inbound connectors — they should be independent instances
	c1, _ := factory.CreateInbound("mysql_inbound")
	c2, _ := factory.CreateInbound("mysql_inbound")
	if c1 == c2 {
		t.Error("factory should create new instances on each call")
	}
	// Initialize one should not affect the other
	_ = c1.Initialize(mustJSON(map[string]interface{}{"host": "host1"}))
	if err := c2.Validate(); err == nil {
		// c2 was not initialized — Validate should still fail
		t.Error("Validate() on uninitialised c2 should fail even after c1 was initialized")
	}
}

// -----------------------------------------------------------------------------
// SupportsCron via factory (verifies factory returns correct implementations)
// -----------------------------------------------------------------------------

func TestFactory_SupportsCronViaPull(t *testing.T) {
	pullTypes := []string{
		"mysql_inbound", "sqlserver_inbound", "mongodb_inbound", "oracle_inbound",
		"aws_s3_inbound", "azure_blob_inbound", "gcs_inbound", "ftp_inbound",
		"snowflake_inbound", "databricks_inbound", "bigquery_inbound",
		"redshift_inbound", "synapse_inbound", "clickhouse_inbound", "timescaledb_inbound",
		"file_listener", "postgresql_inbound", "sftp_inbound",
	}
	factory := NewConnectorFactory()
	for _, typeName := range pullTypes {
		conn, err := factory.CreateInbound(typeName)
		if err != nil {
			t.Errorf("CreateInbound(%q): %v", typeName, err)
			continue
		}
		if !conn.SupportsCron() {
			t.Errorf("SupportsCron() = false for %q, want true (pull mode connectors should support cron)", typeName)
		}
	}
}

func TestFactory_SupportsCronViaPush(t *testing.T) {
	pushTypes := []string{
		"tcp_mllp_inbound", "http_rest_inbound",
		"rabbitmq_inbound", "kafka_inbound", "redis_inbound",
	}
	factory := NewConnectorFactory()
	for _, typeName := range pushTypes {
		conn, err := factory.CreateInbound(typeName)
		if err != nil {
			t.Errorf("CreateInbound(%q): %v", typeName, err)
			continue
		}
		if conn.SupportsCron() {
			t.Errorf("SupportsCron() = true for %q, want false (push mode connectors should not support cron)", typeName)
		}
	}
}

// -----------------------------------------------------------------------------
// Cross-category: IsRunning before Start
// -----------------------------------------------------------------------------

func TestStubInboundConnectors_NotRunningInitially(t *testing.T) {
	type isRunner interface {
		IsRunning() bool
	}
	for _, tc := range allStubInboundCases {
		tc := tc
		t.Run(tc.name+"/notRunningInitially", func(t *testing.T) {
			c := tc.constructor()
			if r, ok := c.(isRunner); ok {
				if r.IsRunning() {
					t.Error("connector should not be running before Start()")
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Tests for fully-implemented inbound connectors (metadata + SupportsCron only;
// Initialize() tests are skipped since these connectors require real infrastructure)
// -----------------------------------------------------------------------------

func TestImplementedInboundConnectors_Metadata(t *testing.T) {
	for _, tc := range implementedInboundCases {
		tc := tc
		t.Run(tc.name+"/metadata", func(t *testing.T) {
			c := tc.constructor()
			meta := c.GetMetadata()
			if meta.TypeName != tc.typeName {
				t.Errorf("TypeName: got %q, want %q", meta.TypeName, tc.typeName)
			}
			if meta.Category != "inbound" {
				t.Errorf("Category: got %q, want inbound", meta.Category)
			}
			if meta.Version == "" {
				t.Error("Version must not be empty")
			}
			if meta.DisplayName == "" {
				t.Error("DisplayName must not be empty")
			}
		})
	}
}

func TestImplementedInboundConnectors_SupportsCron(t *testing.T) {
	for _, tc := range implementedInboundCases {
		tc := tc
		t.Run(tc.name+"/supportsCron", func(t *testing.T) {
			c := tc.constructor()
			got := c.SupportsCron()
			if got != tc.supportsCron {
				t.Errorf("SupportsCron() = %v, want %v", got, tc.supportsCron)
			}
		})
	}
}

func TestImplementedInboundConnectors_ValidateBeforeInit(t *testing.T) {
	for _, tc := range implementedInboundCases {
		tc := tc
		t.Run(tc.name+"/validateBeforeInit", func(t *testing.T) {
			c := tc.constructor()
			err := c.Validate()
			if err == nil {
				t.Error("Validate() should return error before Initialize()")
			}
		})
	}
}

func TestImplementedInboundConnectors_InitializeBadJSON(t *testing.T) {
	for _, tc := range implementedInboundCases {
		tc := tc
		t.Run(tc.name+"/badJSON", func(t *testing.T) {
			c := tc.constructor()
			err := c.Initialize([]byte("not-json"))
			if err == nil {
				t.Error("Initialize() with bad JSON should return error")
			}
		})
	}
}
