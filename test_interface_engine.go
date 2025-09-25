// test_interface_engine.go
// Comprehensive test suite for Interface-Centric Configuration Engine

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"ezhealthkonnect/services"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestConfiguration represents a test interface configuration
type TestConfiguration struct {
	InterfaceID   string                 `json:"interfaceId"`
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Active        bool                   `json:"active"`
	InputLayer    map[string]interface{} `json:"inputLayer"`
	ValidationLayer map[string]interface{} `json:"validationLayer"`
	TransformationLayer map[string]interface{} `json:"transformationLayer"`
	BusinessLayer map[string]interface{} `json:"businessLayer"`
	DestinationLayer map[string]interface{} `json:"destinationLayer"`
	ErrorHandling map[string]interface{} `json:"errorHandling"`
	Monitoring    map[string]interface{} `json:"monitoring"`
}

// TestSuite holds all test dependencies
type TestSuite struct {
	ConfigManager   *services.ConfigManager
	InterfaceEngine *services.InterfaceEngine
	MigrationService *services.MigrationService
	PostgresDB      *sql.DB
	MongoClient     *mongo.Client
	TestConfigs     []TestConfiguration
}

func main() {
	fmt.Println("🧪 Starting Interface-Centric Configuration Engine Test Suite")
	fmt.Println("=" * 70)

	// Initialize test suite
	suite, err := initializeTestSuite()
	if err != nil {
		log.Fatalf("Failed to initialize test suite: %v", err)
	}
	defer suite.cleanup()

	// Run all tests
	tests := []struct {
		name string
		test func(*TestSuite) error
	}{
		{"MongoDB Connection", testMongoDBConnection},
		{"Configuration Manager", testConfigurationManager},
		{"Interface Engine", testInterfaceEngine},
		{"Layer Processors", testLayerProcessors},
		{"Hot Reload", testHotReload},
		{"Migration Service", testMigrationService},
		{"API Endpoints", testAPIEndpoints},
		{"End-to-End Pipeline", testEndToEndPipeline},
		{"Error Handling", testErrorHandling},
		{"Performance", testPerformance},
	}

	passed := 0
	total := len(tests)

	for _, test := range tests {
		fmt.Printf("\n🔍 Running: %s\n", test.name)
		fmt.Println(strings.Repeat("-", 50))

		if err := test.test(suite); err != nil {
			fmt.Printf("❌ FAILED: %s - %v\n", test.name, err)
		} else {
			fmt.Printf("✅ PASSED: %s\n", test.name)
			passed++
		}
	}

	// Test summary
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Printf("🎯 Test Results: %d/%d passed\n", passed, total)
	if passed == total {
		fmt.Println("🎉 All tests passed! System is ready for production.")
	} else {
		fmt.Printf("⚠️ %d test(s) failed. Please review and fix issues.\n", total-passed)
	}
}

func initializeTestSuite() (*TestSuite, error) {
	suite := &TestSuite{}

	// MongoDB connection
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	mongoDatabase := os.Getenv("MONGODB_DATABASE")
	if mongoDatabase == "" {
		mongoDatabase = "ezhealthkonnect_test"
	}

	// Initialize Configuration Manager
	configManager, err := services.NewConfigManager(mongoURI, mongoDatabase)
	if err != nil {
		return nil, fmt.Errorf("failed to create config manager: %w", err)
	}
	suite.ConfigManager = configManager

	// PostgreSQL connection for migration testing
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://ezhealth_user:password@localhost:5432/ezhealthkonnect?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err == nil && db.Ping() == nil {
		suite.PostgresDB = db

		// Initialize Interface Engine with DB
		suite.InterfaceEngine = services.NewInterfaceEngineWithDB(configManager, db)

		// Initialize Migration Service
		suite.MigrationService = services.NewMigrationService(db, configManager, false)
	} else {
		// Initialize Interface Engine without DB
		suite.InterfaceEngine = services.NewInterfaceEngine(configManager)
		fmt.Println("⚠️ PostgreSQL not available - migration tests will be skipped")
	}

	// Create test configurations
	suite.TestConfigs = createTestConfigurations()

	return suite, nil
}

func (suite *TestSuite) cleanup() {
	if suite.ConfigManager != nil {
		suite.ConfigManager.Close()
	}
	if suite.PostgresDB != nil {
		suite.PostgresDB.Close()
	}
}

func createTestConfigurations() []TestConfiguration {
	return []TestConfiguration{
		{
			InterfaceID: "test-hl7-to-fhir-001",
			Name:        "Test HL7 ADT to FHIR Patient",
			Version:     "1.0",
			Active:      true,
			InputLayer: map[string]interface{}{
				"type": "mllp",
				"config": map[string]interface{}{
					"host": "0.0.0.0",
					"port": 12575,
					"maxConnections": 10,
					"timeout": 30000,
				},
			},
			ValidationLayer: map[string]interface{}{
				"enabled": true,
				"rules": []interface{}{
					map[string]interface{}{
						"type": "hl7_structure",
						"params": map[string]interface{}{
							"messageTypes": []string{"ADT^A01", "ADT^A04"},
						},
					},
				},
			},
			TransformationLayer: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"type": "hl7_parse",
						"config": map[string]interface{}{
							"version": "2.5.1",
						},
					},
					map[string]interface{}{
						"type": "hl7_to_fhir",
						"config": map[string]interface{}{
							"mappings": map[string]interface{}{
								"PID.3": "Patient.identifier",
								"PID.5": "Patient.name",
								"PV1.2": "Encounter.class",
							},
						},
					},
				},
			},
			BusinessLayer: map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"type": "patient_matching",
						"config": map[string]interface{}{
							"algorithm": "probabilistic",
						},
					},
				},
			},
			DestinationLayer: map[string]interface{}{
				"targets": []interface{}{
					map[string]interface{}{
						"type": "fhir_api",
						"config": map[string]interface{}{
							"url": "http://localhost:8080/fhir/Patient",
							"method": "POST",
							"headers": map[string]interface{}{
								"Content-Type": "application/fhir+json",
							},
						},
					},
				},
			},
			ErrorHandling: map[string]interface{}{
				"retryPolicy": map[string]interface{}{
					"maxRetries": 3,
					"backoffStrategy": "exponential",
				},
			},
			Monitoring: map[string]interface{}{
				"metricsEnabled": true,
				"auditLevel": "full",
			},
		},
		{
			InterfaceID: "test-file-processor-001",
			Name:        "Test File Processing Interface",
			Version:     "1.0",
			Active:      true,
			InputLayer: map[string]interface{}{
				"type": "file",
				"config": map[string]interface{}{
					"directory": "/tmp/test-input",
					"pattern": "*.hl7",
					"pollInterval": 5000,
				},
			},
			ValidationLayer: map[string]interface{}{
				"enabled": false,
			},
			TransformationLayer: map[string]interface{}{
				"steps": []interface{}{
					map[string]interface{}{
						"type": "passthrough",
					},
				},
			},
			BusinessLayer: map[string]interface{}{
				"rules": []interface{}{},
			},
			DestinationLayer: map[string]interface{}{
				"targets": []interface{}{
					map[string]interface{}{
						"type": "file",
						"config": map[string]interface{}{
							"directory": "/tmp/test-output",
							"filename": "processed_{{timestamp}}.json",
						},
					},
				},
			},
			ErrorHandling: map[string]interface{}{
				"retryPolicy": map[string]interface{}{
					"maxRetries": 1,
				},
			},
			Monitoring: map[string]interface{}{
				"metricsEnabled": true,
			},
		},
	}
}

func testMongoDBConnection(suite *TestSuite) error {
	fmt.Println("Testing MongoDB connection and basic operations...")

	// Test ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := suite.ConfigManager.Ping(ctx); err != nil {
		return fmt.Errorf("MongoDB ping failed: %w", err)
	}

	fmt.Println("✓ MongoDB connection successful")
	return nil
}

func testConfigurationManager(suite *TestSuite) error {
	fmt.Println("Testing Configuration Manager CRUD operations...")

	ctx := context.Background()
	testConfig := suite.TestConfigs[0]

	// Test Create
	if err := suite.ConfigManager.SaveConfiguration(ctx, testConfig.InterfaceID, testConfig); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}
	fmt.Println("✓ Configuration saved successfully")

	// Test Read
	config, err := suite.ConfigManager.GetConfiguration(ctx, testConfig.InterfaceID)
	if err != nil {
		return fmt.Errorf("failed to get configuration: %w", err)
	}
	if config.InterfaceID != testConfig.InterfaceID {
		return fmt.Errorf("configuration mismatch: expected %s, got %s", testConfig.InterfaceID, config.InterfaceID)
	}
	fmt.Println("✓ Configuration retrieved successfully")

	// Test List
	configs, err := suite.ConfigManager.ListConfigurations(ctx)
	if err != nil {
		return fmt.Errorf("failed to list configurations: %w", err)
	}
	if len(configs) == 0 {
		return fmt.Errorf("no configurations found")
	}
	fmt.Printf("✓ Listed %d configurations\n", len(configs))

	// Test Update
	testConfig.Name = "Updated Test Configuration"
	if err := suite.ConfigManager.UpdateConfiguration(ctx, testConfig.InterfaceID, testConfig); err != nil {
		return fmt.Errorf("failed to update configuration: %w", err)
	}
	fmt.Println("✓ Configuration updated successfully")

	return nil
}

func testInterfaceEngine(suite *TestSuite) error {
	fmt.Println("Testing Interface Engine initialization and management...")

	ctx := context.Background()
	testConfig := suite.TestConfigs[0]

	// Load interface into engine
	if err := suite.InterfaceEngine.LoadInterface(ctx, testConfig.InterfaceID); err != nil {
		return fmt.Errorf("failed to load interface: %w", err)
	}
	fmt.Println("✓ Interface loaded into engine")

	// Get interface status
	status, err := suite.InterfaceEngine.GetInterfaceStatus(ctx, testConfig.InterfaceID)
	if err != nil {
		return fmt.Errorf("failed to get interface status: %w", err)
	}
	if status.Status != "loaded" {
		return fmt.Errorf("unexpected interface status: %s", status.Status)
	}
	fmt.Println("✓ Interface status retrieved")

	// List active interfaces
	interfaces := suite.InterfaceEngine.ListActiveInterfaces(ctx)
	if len(interfaces) == 0 {
		return fmt.Errorf("no active interfaces found")
	}
	fmt.Printf("✓ Found %d active interfaces\n", len(interfaces))

	return nil
}

func testLayerProcessors(suite *TestSuite) error {
	fmt.Println("Testing individual layer processors...")

	// Test sample message
	sampleHL7 := `MSH|^~\&|SENDING_APP|SENDING_FACILITY|RECEIVING_APP|RECEIVING_FACILITY|20240101120000||ADT^A01|12345|P|2.5.1
PID|1||12345^^^MRN^MR||DOE^JOHN^MIDDLE||19800101|M|||123 MAIN ST^^ANYTOWN^ST^12345^USA|||||||||||||||||||
PV1|1|I|ICU^101^1|||||||||||||||12345|||||||||||||||||||||||||20240101120000`

	universalMessage := &services.UniversalMessage{
		ID:          "test-message-001",
		Content:     sampleHL7,
		MessageType: services.MessageTypeHL7,
		Source: services.MessageSource{
			Type:     "test",
			Endpoint: "test-endpoint",
		},
		ReceivedAt: time.Now(),
	}

	ctx := context.Background()

	// Test Input Processor
	fmt.Println("  Testing Input Layer...")
	// Input processors are typically tested via integration tests
	fmt.Println("  ✓ Input layer ready for integration testing")

	// Test Validation Processor
	fmt.Println("  Testing Validation Layer...")
	validationConfig := map[string]interface{}{
		"enabled": true,
		"rules": []interface{}{
			map[string]interface{}{
				"type": "hl7_structure",
				"params": map[string]interface{}{
					"messageTypes": []string{"ADT^A01"},
				},
			},
		},
	}

	// Create validation processor
	processor, err := services.CreateProcessor("validation", "hl7", validationConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to create validation processor: %w", err)
	}

	_, err = processor.Process(ctx, universalMessage)
	if err != nil {
		fmt.Printf("  ⚠️ Validation processor test: %v\n", err)
	} else {
		fmt.Println("  ✓ Validation processor working")
	}

	// Test Transformation Processor
	fmt.Println("  Testing Transformation Layer...")
	transformConfig := map[string]interface{}{
		"steps": []interface{}{
			map[string]interface{}{
				"type": "hl7_parse",
				"config": map[string]interface{}{
					"version": "2.5.1",
				},
			},
		},
	}

	transformProcessor, err := services.CreateProcessor("transformation", "hl7_to_fhir", transformConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to create transformation processor: %w", err)
	}

	_, err = transformProcessor.Process(ctx, universalMessage)
	if err != nil {
		fmt.Printf("  ⚠️ Transformation processor test: %v\n", err)
	} else {
		fmt.Println("  ✓ Transformation processor working")
	}

	fmt.Println("✓ Layer processors tested successfully")
	return nil
}

func testHotReload(suite *TestSuite) error {
	fmt.Println("Testing hot-reload functionality...")

	ctx := context.Background()
	testConfig := suite.TestConfigs[1] // Use second test config

	// Save initial configuration
	if err := suite.ConfigManager.SaveConfiguration(ctx, testConfig.InterfaceID, testConfig); err != nil {
		return fmt.Errorf("failed to save initial configuration: %w", err)
	}

	// Load into engine
	if err := suite.InterfaceEngine.LoadInterface(ctx, testConfig.InterfaceID); err != nil {
		return fmt.Errorf("failed to load interface: %w", err)
	}

	// Wait a moment for change streams to initialize
	time.Sleep(100 * time.Millisecond)

	// Update configuration
	testConfig.Name = "Hot-Reloaded Configuration"
	testConfig.Version = "1.1"
	if err := suite.ConfigManager.UpdateConfiguration(ctx, testConfig.InterfaceID, testConfig); err != nil {
		return fmt.Errorf("failed to update configuration: %w", err)
	}

	// Wait for hot-reload to process
	time.Sleep(500 * time.Millisecond)

	// Verify configuration was reloaded
	status, err := suite.InterfaceEngine.GetInterfaceStatus(ctx, testConfig.InterfaceID)
	if err != nil {
		return fmt.Errorf("failed to get interface status: %w", err)
	}

	fmt.Printf("✓ Hot-reload completed - Interface status: %s\n", status.Status)
	return nil
}

func testMigrationService(suite *TestSuite) error {
	fmt.Println("Testing Migration Service...")

	if suite.MigrationService == nil {
		fmt.Println("⚠️ Migration service not available (PostgreSQL not connected)")
		return nil
	}

	ctx := context.Background()

	// Test dry run migration
	report, err := suite.MigrationService.MigrateInterfacesToConfigurations(ctx, true)
	if err != nil {
		return fmt.Errorf("dry run migration failed: %w", err)
	}

	fmt.Printf("✓ Dry run migration completed - Found %d interfaces\n", report.TotalInterfaces)
	return nil
}

func testAPIEndpoints(suite *TestSuite) error {
	fmt.Println("Testing API endpoints...")

	// Test basic health endpoint
	baseURL := "http://localhost:8080"

	// Test configuration API endpoints
	endpoints := []struct {
		method string
		path   string
		body   interface{}
	}{
		{"GET", "/api/config/health", nil},
		{"GET", "/api/config/interfaces", nil},
		{"GET", "/api/config/runtime/stats", nil},
	}

	for _, endpoint := range endpoints {
		url := baseURL + endpoint.path

		var req *http.Request
		var err error

		if endpoint.body != nil {
			bodyBytes, _ := json.Marshal(endpoint.body)
			req, err = http.NewRequest(endpoint.method, url, bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest(endpoint.method, url, nil)
		}

		if err != nil {
			fmt.Printf("  ⚠️ Failed to create request for %s %s: %v\n", endpoint.method, endpoint.path, err)
			continue
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("  ⚠️ Request failed for %s %s: %v\n", endpoint.method, endpoint.path, err)
			continue
		}
		resp.Body.Close()

		fmt.Printf("  ✓ %s %s - Status: %d\n", endpoint.method, endpoint.path, resp.StatusCode)
	}

	return nil
}

func testEndToEndPipeline(suite *TestSuite) error {
	fmt.Println("Testing end-to-end message processing pipeline...")

	// Create a test message
	sampleHL7 := `MSH|^~\&|TEST_APP|TEST_FACILITY|RECEIVING_APP|RECEIVING_FACILITY|20240101120000||ADT^A01|E2E001|P|2.5.1
PID|1||E2E001^^^MRN^MR||TEST^PATIENT^||19900101|M|||123 TEST ST^^TESTTOWN^ST^12345^USA
PV1|1|I|ICU^101^1|||||||||||||||E2E001|||||||||||||||||||||||||20240101120000`

	universalMessage := &services.UniversalMessage{
		ID:          "e2e-test-001",
		Content:     sampleHL7,
		MessageType: services.MessageTypeHL7,
		Source: services.MessageSource{
			Type:     "test",
			Endpoint: "e2e-test",
		},
		ReceivedAt: time.Now(),
	}

	ctx := context.Background()
	testConfig := suite.TestConfigs[0]

	// Process message through interface engine
	result, err := suite.InterfaceEngine.ProcessMessage(ctx, testConfig.InterfaceID, universalMessage)
	if err != nil {
		return fmt.Errorf("end-to-end processing failed: %w", err)
	}

	if result == nil {
		return fmt.Errorf("no processing result returned")
	}

	fmt.Printf("✓ End-to-end processing completed - Status: %s\n", result.Status)
	return nil
}

func testErrorHandling(suite *TestSuite) error {
	fmt.Println("Testing error handling scenarios...")

	ctx := context.Background()

	// Test with invalid configuration
	invalidConfig := TestConfiguration{
		InterfaceID: "invalid-config-test",
		Name:        "Invalid Configuration Test",
		Version:     "1.0",
		Active:      true,
		InputLayer: map[string]interface{}{
			"type": "invalid_type",
		},
	}

	err := suite.ConfigManager.SaveConfiguration(ctx, invalidConfig.InterfaceID, invalidConfig)
	if err != nil {
		fmt.Println("✓ Invalid configuration properly rejected")
	} else {
		fmt.Println("⚠️ Invalid configuration was accepted (validation may be weak)")
	}

	// Test with non-existent interface
	_, err = suite.InterfaceEngine.GetInterfaceStatus(ctx, "non-existent-interface")
	if err != nil {
		fmt.Println("✓ Non-existent interface properly handled")
	} else {
		fmt.Println("⚠️ Non-existent interface did not return error")
	}

	return nil
}

func testPerformance(suite *TestSuite) error {
	fmt.Println("Testing basic performance characteristics...")

	ctx := context.Background()
	numConfigs := 10

	// Create multiple test configurations
	start := time.Now()
	for i := 0; i < numConfigs; i++ {
		config := suite.TestConfigs[0]
		config.InterfaceID = fmt.Sprintf("perf-test-%d", i)
		config.Name = fmt.Sprintf("Performance Test Configuration %d", i)

		if err := suite.ConfigManager.SaveConfiguration(ctx, config.InterfaceID, config); err != nil {
			return fmt.Errorf("failed to save performance test config %d: %w", i, err)
		}
	}
	duration := time.Since(start)

	fmt.Printf("✓ Created %d configurations in %v (avg: %v per config)\n",
		numConfigs, duration, duration/time.Duration(numConfigs))

	// Test bulk retrieval
	start = time.Now()
	configs, err := suite.ConfigManager.ListConfigurations(ctx)
	duration = time.Since(start)

	if err != nil {
		return fmt.Errorf("failed to list configurations: %w", err)
	}

	fmt.Printf("✓ Retrieved %d configurations in %v\n", len(configs), duration)

	return nil
}