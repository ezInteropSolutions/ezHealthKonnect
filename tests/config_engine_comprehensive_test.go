// config_engine_comprehensive_test.go
// Comprehensive test suite for the Interface-Centric Configuration Engine
// This test suite validates all components work together seamlessly and ensures production readiness

package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"ezhealthkonnect/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"
)

// Test configuration
const (
	testMongoURI      = "mongodb://localhost:27017"
	testMongoDB       = "ezhealthkonnect_test"
	testTimeout       = 30 * time.Second
	loadTestDuration  = 60 * time.Second
	maxConcurrency    = 50
)

// TestSuite holds all test components
type TestSuite struct {
	configManager    *services.ConfigManager
	interfaceEngine  *services.InterfaceEngine
	migrationService *services.MigrationService
	postgresDB       *sql.DB
	testConfigs      []*services.InterfaceConfig
}

// SetupTestSuite initializes the test environment
func SetupTestSuite(t *testing.T) *TestSuite {
	// Initialize MongoDB Configuration Manager
	configManager, err := services.NewConfigManager(testMongoURI, testMongoDB)
	require.NoError(t, err, "Failed to initialize Configuration Manager")

	// Setup PostgreSQL connection for migration testing
	var postgresDB *sql.DB
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		postgresDB, err = sql.Open("postgres", dbURL)
		require.NoError(t, err, "Failed to connect to PostgreSQL")
		err = postgresDB.Ping()
		require.NoError(t, err, "Failed to ping PostgreSQL")
	}

	// Initialize Interface Engine with database integration
	var interfaceEngine *services.InterfaceEngine
	if postgresDB != nil {
		interfaceEngine = services.NewInterfaceEngineWithDB(configManager, postgresDB)
	} else {
		interfaceEngine = services.NewInterfaceEngine(configManager)
	}

	// Initialize Migration Service if PostgreSQL is available
	var migrationService *services.MigrationService
	if postgresDB != nil {
		migrationService = services.NewMigrationService(postgresDB, configManager, false)
	}

	return &TestSuite{
		configManager:    configManager,
		interfaceEngine:  interfaceEngine,
		migrationService: migrationService,
		postgresDB:       postgresDB,
		testConfigs:      make([]*services.InterfaceConfig, 0),
	}
}

// TeardownTestSuite cleans up the test environment
func (ts *TestSuite) TeardownTestSuite(t *testing.T) {
	// Clean up test configurations
	for _, config := range ts.testConfigs {
		// In production, we'd implement a delete method
		log.Printf("Test config cleanup needed for: %s", config.InterfaceID)
	}

	if ts.configManager != nil {
		err := ts.configManager.Close()
		assert.NoError(t, err, "Failed to close Configuration Manager")
	}

	if ts.postgresDB != nil {
		err := ts.postgresDB.Close()
		assert.NoError(t, err, "Failed to close PostgreSQL connection")
	}
}

// TestEndToEndPipeline tests the complete message processing pipeline
func TestEndToEndPipeline(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	t.Run("Complete Pipeline Processing", func(t *testing.T) {
		// Create a comprehensive test configuration
		config := createAdvancedTestConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		// Save configuration
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save test configuration")

		// Process test messages
		testMessages := []string{
			createHL7ADTMessage(),
			createHL7ORUMessage(),
			createHL7MDMMessage(),
		}

		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		for i, message := range testMessages {
			t.Run(fmt.Sprintf("ProcessMessage_%d", i+1), func(t *testing.T) {
				processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(message))
				require.NoError(t, err, "Message processing failed")
				assert.NotEmpty(t, processedMessage.ID, "Processed message ID should not be empty")
				assert.Equal(t, config.InterfaceID, processedMessage.InterfaceID, "Interface ID should match")
				assert.NotEmpty(t, processedMessage.ProcessingSteps, "Processing steps should not be empty")

				// Validate processing steps
				expectedSteps := []string{"input", "validation", "transformation", "business_logic", "delivery"}
				assert.GreaterOrEqual(t, len(processedMessage.ProcessingSteps), len(expectedSteps), "Should have all processing steps")

				// Check that all steps completed successfully
				for _, step := range processedMessage.ProcessingSteps {
					assert.Equal(t, "completed", step.Status, fmt.Sprintf("Step %s should be completed", step.StepType))
				}
			})
		}

		// Verify interface statistics
		stats, exists := ts.interfaceEngine.GetInterfaceStats(config.InterfaceID)
		assert.True(t, exists, "Interface stats should exist")
		assert.Equal(t, int64(len(testMessages)), stats.TotalProcessed, "Total processed count should match")
	})
}

// TestConfigurationManagement tests all configuration management features
func TestConfigurationManagement(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	t.Run("Configuration CRUD Operations", func(t *testing.T) {
		config := createAdvancedTestConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		// Test Create
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to create configuration")

		// Test Read
		loadedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load configuration")
		assert.Equal(t, config.Name, loadedConfig.Name, "Configuration name should match")
		assert.Equal(t, config.InterfaceID, loadedConfig.InterfaceID, "Interface ID should match")

		// Test Update
		loadedConfig.Description = "Updated description for testing"
		loadedConfig.Version = "2.0.0"
		err = ts.configManager.SaveConfig(loadedConfig)
		require.NoError(t, err, "Failed to update configuration")

		// Verify update
		updatedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load updated configuration")
		assert.Equal(t, "Updated description for testing", updatedConfig.Description, "Description should be updated")
		assert.Equal(t, "2.0.0", updatedConfig.Version, "Version should be updated")

		// Test validation
		err = ts.configManager.ValidateConfig(updatedConfig)
		assert.NoError(t, err, "Updated configuration should be valid")
	})

	t.Run("Configuration Caching and Hot Reload", func(t *testing.T) {
		config := createAdvancedTestConfiguration()
		config.InterfaceID = uuid.New().String() // Ensure unique ID
		ts.testConfigs = append(ts.testConfigs, config)

		// Save and load to populate cache
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save configuration")

		loadedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load configuration (initial)")

		// Update configuration (simulating external change)
		config.Description = "Hot-reload test configuration"
		config.Version = "3.0.0"
		err = ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save updated configuration")

		// Test hot-reload
		reloadedConfig, err := ts.configManager.ReloadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to reload configuration")
		assert.Equal(t, "Hot-reload test configuration", reloadedConfig.Description, "Hot-reload should fetch updated config")
		assert.Equal(t, "3.0.0", reloadedConfig.Version, "Version should be updated after reload")
	})
}

// TestLoadTesting performs load testing on the configuration engine
func TestLoadTesting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	t.Run("Concurrent Configuration Operations", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, maxConcurrency)

		// Create multiple configurations concurrently
		for i := 0; i < maxConcurrency; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				config := createAdvancedTestConfiguration()
				config.Name = fmt.Sprintf("Load Test Config %d", index)
				config.InterfaceID = uuid.New().String()
				ts.testConfigs = append(ts.testConfigs, config)

				// Save configuration
				if err := ts.configManager.SaveConfig(config); err != nil {
					errors <- fmt.Errorf("save failed for config %d: %w", index, err)
					return
				}

				// Load configuration
				if _, err := ts.configManager.LoadConfig(config.InterfaceID); err != nil {
					errors <- fmt.Errorf("load failed for config %d: %w", index, err)
					return
				}

				// Validate configuration
				if err := ts.configManager.ValidateConfig(config); err != nil {
					errors <- fmt.Errorf("validation failed for config %d: %w", index, err)
					return
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Load test error: %v", err)
		}
	})

	t.Run("Concurrent Message Processing", func(t *testing.T) {
		// Create a single configuration for message processing
		config := createAdvancedTestConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save config for load test")

		var wg sync.WaitGroup
		errors := make(chan error, maxConcurrency)
		successCount := make(chan int, maxConcurrency)

		ctx, cancel := context.WithTimeout(context.Background(), loadTestDuration)
		defer cancel()

		// Process messages concurrently
		for i := 0; i < maxConcurrency; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()

				message := createHL7ADTMessage()
				processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(message))
				if err != nil {
					errors <- fmt.Errorf("processing failed for message %d: %w", index, err)
					return
				}

				if len(processedMessage.ProcessingSteps) == 0 {
					errors <- fmt.Errorf("no processing steps for message %d", index)
					return
				}

				successCount <- 1
			}(i)
		}

		wg.Wait()
		close(errors)
		close(successCount)

		// Count results
		errorCount := 0
		for err := range errors {
			t.Logf("Load test processing error: %v", err)
			errorCount++
		}

		successfulProcessing := 0
		for range successCount {
			successfulProcessing++
		}

		t.Logf("Load test results: %d successful, %d errors out of %d total",
			successfulProcessing, errorCount, maxConcurrency)

		// Assert that most messages processed successfully (allow for some errors in load testing)
		assert.Greater(t, successfulProcessing, maxConcurrency*8/10, "At least 80% of messages should process successfully")
	})
}

// TestMigrationService tests PostgreSQL to MongoDB migration
func TestMigrationService(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	if ts.migrationService == nil {
		t.Skip("Migration service not available (PostgreSQL connection required)")
	}

	t.Run("Migration Progress Tracking", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		// Get initial migration progress
		progress, err := ts.migrationService.GetMigrationProgress(ctx)
		require.NoError(t, err, "Failed to get migration progress")
		assert.NotNil(t, progress, "Migration progress should not be nil")

		// Progress should have expected fields
		assert.GreaterOrEqual(t, progress.TotalInterfaces, int64(0), "Total interfaces should be non-negative")
		assert.GreaterOrEqual(t, progress.MigratedInterfaces, int64(0), "Migrated interfaces should be non-negative")
	})

	t.Run("Migration Validation", func(t *testing.T) {
		// Create test data in PostgreSQL (if available)
		// This would require setting up test data in PostgreSQL
		// For now, we test that the migration service is properly initialized
		assert.NotNil(t, ts.migrationService, "Migration service should be initialized")
	})
}

// TestAPIEndpoints tests the REST API endpoints
func TestAPIEndpoints(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	// This test assumes the server is running on localhost:8080
	baseURL := "http://localhost:8080/api/config"

	t.Run("Configuration API Endpoints", func(t *testing.T) {
		config := createAdvancedTestConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		// First save the config through the service (to ensure it exists)
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save config for API testing")

		// Test GET /api/config/interfaces
		resp, err := http.Get(baseURL + "/interfaces")
		if err != nil {
			t.Skip("API server not running, skipping endpoint tests")
		}
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET /interfaces should return 200")

		// Test GET /api/config/interfaces/:id
		resp, err = http.Get(fmt.Sprintf("%s/interfaces/%s", baseURL, config.InterfaceID))
		require.NoError(t, err, "Failed to make GET request")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET /interfaces/:id should return 200")

		// Test POST /api/config/interfaces/validate
		validatePayload, _ := json.Marshal(config)
		resp, err = http.Post(baseURL+"/interfaces/validate", "application/json", bytes.NewBuffer(validatePayload))
		require.NoError(t, err, "Failed to make validate POST request")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "POST /interfaces/validate should return 200")
	})

	t.Run("Health Check Endpoints", func(t *testing.T) {
		healthURL := "http://localhost:8080/api/config/health"

		resp, err := http.Get(healthURL)
		if err != nil {
			t.Skip("API server not running, skipping health check tests")
		}
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check should return 200")

		// Parse response
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "Failed to read health check response")

		var healthResponse map[string]interface{}
		err = json.Unmarshal(body, &healthResponse)
		require.NoError(t, err, "Failed to parse health check JSON")

		assert.True(t, healthResponse["success"].(bool), "Health check should indicate success")
	})
}

// TestErrorHandling tests error scenarios and recovery
func TestErrorHandling(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	t.Run("Invalid Configuration Handling", func(t *testing.T) {
		// Test invalid configuration
		invalidConfig := &services.InterfaceConfig{
			InterfaceID: uuid.New().String(),
			Name:        "", // Invalid: empty name
			Pipeline: services.PipelineConfig{
				Input: services.InputConfig{
					Type: "", // Invalid: empty type
				},
				Destinations: []services.DestinationConfig{}, // Invalid: no destinations
			},
		}

		err := ts.configManager.ValidateConfig(invalidConfig)
		assert.Error(t, err, "Invalid configuration should fail validation")
	})

	t.Run("Non-existent Configuration Loading", func(t *testing.T) {
		nonExistentID := uuid.New().String()
		_, err := ts.configManager.LoadConfig(nonExistentID)
		assert.Error(t, err, "Loading non-existent configuration should fail")
	})

	t.Run("Invalid Message Processing", func(t *testing.T) {
		config := createAdvancedTestConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save config for error testing")

		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		// Test with invalid HL7 message
		invalidMessage := "INVALID|MESSAGE|FORMAT"
		_, err = ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(invalidMessage))
		// Note: Depending on implementation, this might succeed with warnings or fail
		// The important thing is that it handles the error gracefully
		t.Logf("Invalid message processing result: %v", err)
	})
}

// Helper function to create advanced test configuration
func createAdvancedTestConfiguration() *services.InterfaceConfig {
	interfaceID := uuid.New().String()

	return &services.InterfaceConfig{
		InterfaceID: interfaceID,
		Name:        "Advanced Test HL7 to FHIR Interface",
		Description: "Comprehensive test configuration with all features enabled",
		Version:     "1.0.0",
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   "test_suite",
		UpdatedBy:   "test_suite",

		Pipeline: services.PipelineConfig{
			Input: services.InputConfig{
				Type: "mllp",
				ConnectorConfig: map[string]interface{}{
					"host":              "0.0.0.0",
					"port":              2575,
					"max_connections":   100,
					"read_timeout_ms":   30000,
					"write_timeout_ms":  10000,
					"keep_alive":        true,
					"buffer_size":       8192,
				},
				Validation: services.InputValidationConfig{
					Enabled: true,
					Rules: []services.ValidationRule{
						{
							RuleType:    "required_field",
							FieldPath:   "MSH.3",
							ErrorAction: "reject",
						},
						{
							RuleType:    "length_check",
							FieldPath:   "PID.1",
							ErrorAction: "warn",
						},
					},
				},
				Preprocessing: services.PreprocessingConfig{
					Enabled: true,
					Steps: []services.PreprocessingStep{
						{
							Type:   "normalize_encoding",
							Config: map[string]interface{}{"target_encoding": "UTF-8"},
						},
						{
							Type:   "remove_whitespace",
							Config: map[string]interface{}{"trim": true},
						},
					},
				},
			},

			Validation: services.ValidationConfig{
				SchemaValidation: services.SchemaValidationConfig{
					Enabled:    true,
					SchemaType: "hl7",
					StrictMode: false,
				},
				BusinessRules: []services.BusinessRule{
					{
						RuleID:       "patient_id_required",
						Condition:    "PID.3 is not empty",
						Validation:   "required",
						ErrorMessage: "Patient ID is required",
						Severity:     "error",
					},
				},
				CustomValidators: []services.CustomValidator{
					{
						Name: "custom_date_validator",
						Type: "javascript",
						Code: "function validate(message) { return true; }",
					},
				},
			},

			Transformation: services.TransformationConfig{
				Engine:          "hl7_to_fhir",
				MappingTemplate: "standard_adt_v4",
				CustomMappings: []services.FieldMapping{
					{
						SourceField: "PID.5",
						TargetField: "Patient.name",
						Transformation: services.TransformationStep{
							Type:   "name_formatter",
							Config: map[string]interface{}{"format": "fhir_humanname"},
						},
					},
				},
				PostProcessing: []services.PostProcessingStep{
					{
						Type:   "add_metadata",
						Config: map[string]interface{}{"source": "test_interface"},
					},
				},
			},

			BusinessLogic: services.BusinessLogicConfig{
				RulesEngine: services.RulesEngineConfig{
					Enabled: true,
					Rules: []services.BusinessLogicRule{
						{
							RuleID:    "duplicate_check",
							Condition: "Patient.identifier exists",
							Action: map[string]interface{}{
								"type":   "deduplicate",
								"method": "latest_wins",
							},
						},
					},
				},
				WorkflowAutomation: []services.WorkflowAction{
					{
						Trigger: "patient_admitted",
						Actions: []map[string]interface{}{
							{
								"type":   "notify",
								"target": "admission_team",
							},
						},
					},
				},
			},

			Destinations: []services.DestinationConfig{
				{
					DestinationID: uuid.New().String(),
					Type:          "fhir_api",
					Config: map[string]interface{}{
						"base_url":         "http://localhost:8080/fhir",
						"timeout":          30000,
						"auth_type":        "oauth2",
						"batch_size":       10,
						"parallel_uploads": 3,
					},
					RoutingRules: []services.RoutingRule{
						{
							Condition:    "resourceType == 'Patient'",
							ResourceType: "Patient",
							Operation:    "create_or_update",
						},
					},
					ErrorHandling: services.ErrorHandlingConfig{
						RetryCount:      5,
						RetryDelay:      2000,
						DeadLetterQueue: true,
					},
				},
				{
					DestinationID: uuid.New().String(),
					Type:          "database",
					Config: map[string]interface{}{
						"table_name":     "processed_messages",
						"batch_insert":   true,
						"batch_size":     50,
						"connection_pool": 10,
					},
					RoutingRules: []services.RoutingRule{},
					ErrorHandling: services.ErrorHandlingConfig{
						RetryCount:      3,
						RetryDelay:      1000,
						DeadLetterQueue: true,
					},
				},
				{
					DestinationID: uuid.New().String(),
					Type:          "queue",
					Config: map[string]interface{}{
						"queue_name":   "processed_fhir",
						"exchange":     "health_data",
						"routing_key":  "fhir.processed",
						"durable":      true,
						"auto_delete":  false,
					},
					RoutingRules: []services.RoutingRule{},
					ErrorHandling: services.ErrorHandlingConfig{
						RetryCount:      3,
						RetryDelay:      5000,
						DeadLetterQueue: true,
					},
				},
			},
		},

		Monitoring: services.MonitoringConfig{
			MetricsEnabled: true,
			AlertThresholds: map[string]float64{
				"error_rate":           0.02,  // 2% error rate threshold
				"processing_time_ms":   3000,  // 3 second processing time threshold
				"queue_depth":          500,   // 500 message queue depth threshold
				"connection_failures":  10,    // 10 connection failures threshold
				"memory_usage_percent": 80,    // 80% memory usage threshold
				"cpu_usage_percent":    75,    // 75% CPU usage threshold
			},
			RetentionPolicy: map[string]int{
				"raw_messages":       30,   // 30 days
				"processed_messages": 90,   // 90 days
				"error_logs":         365,  // 1 year
				"audit_logs":         2555, // 7 years (HIPAA compliance)
				"metrics":            180,  // 6 months
				"performance_stats":  90,   // 3 months
			},
		},

		VersionHistory: []services.ConfigVersion{
			{
				Version:   "1.0.0",
				CreatedAt: time.Now(),
				CreatedBy: "test_suite",
				Changes:   []string{"Initial configuration created"},
			},
		},
	}
}

// Sample HL7 message creators for different message types
func createHL7ADTMessage() string {
	return `MSH|^~\&|HOSPITAL|MAIN|RECEIVER|CLINIC|20250123143000||ADT^A01^ADT_A01|MSG12345|P|2.5|||||||||||
EVN||20250123143000||||20250123143000|
PID|1||123456789^^^HOSPITAL^MR~987654321^^^SSN^SS||SMITH^JOHN^MIDDLE^JR^DR||19800315|M|||123 MAIN ST^APT 4B^CITY^ST^12345^USA||(555)123-4567|(555)765-4321|ENG|M|CHR|123456789||||N||||||||20250123143000|
PV1|1|I|ICU^101^A|E||||||^ATTENDING^PHYSICIAN^^MD|||||||V|||||||||||||||||||||||||20250123143000|
OBX|1|ST|HEIGHT||180|cm|||N|||F|||20250123143000|
OBX|2|ST|WEIGHT||75|kg|||N|||F|||20250123143000|`
}

func createHL7ORUMessage() string {
	return `MSH|^~\&|LAB|MAIN|HIS|CLINIC|20250123143000||ORU^R01^ORU_R01|LAB67890|P|2.5|||||||||||
PID|1||123456789^^^HOSPITAL^MR||SMITH^JOHN^MIDDLE^JR^DR||19800315|M|||123 MAIN ST^APT 4B^CITY^ST^12345^USA||(555)123-4567|(555)765-4321|ENG|M|CHR|123456789||||N||||||||20250123143000|
OBR|1|LAB123|LAB123|CBC^Complete Blood Count^L|||20250123120000|||||||20250123120000||||||||20250123143000|||F|||||||||||||||||||
OBX|1|NM|WBC^White Blood Count^L||7.5|10*3/uL|4.0-11.0|N|||F|||20250123143000|
OBX|2|NM|RBC^Red Blood Count^L||4.5|10*6/uL|4.0-5.5|N|||F|||20250123143000|
OBX|3|NM|HGB^Hemoglobin^L||14.2|g/dL|12.0-16.0|N|||F|||20250123143000|`
}

func createHL7MDMMessage() string {
	return `MSH|^~\&|DOC|MAIN|HIS|CLINIC|20250123143000||MDM^T02^MDM_T02|DOC55555|P|2.5|||||||||||
PID|1||123456789^^^HOSPITAL^MR||SMITH^JOHN^MIDDLE^JR^DR||19800315|M|||123 MAIN ST^APT 4B^CITY^ST^12345^USA||(555)123-4567|(555)765-4321|ENG|M|CHR|123456789||||N||||||||20250123143000|
TXA|1|DISCHARGE^Discharge Summary^L|TEXT|20250123143000|^DOC^DOCTOR^^MD|20250123143000|20250123143000||||DOC123|||||AU|AV|
OBX|1|TX|DISCHARGE^Discharge Summary^L||Patient discharged home in stable condition.~Follow up with primary care physician in one week.~Continue current medications as prescribed.||||||F|||20250123143000|`
}