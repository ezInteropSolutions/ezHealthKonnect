// config_engine_integration_test.go
// Integration tests that validate existing services work with the new configuration engine

package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"ezhealthkonnect/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExistingServiceIntegration tests integration with existing services
func TestExistingServiceIntegration(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	t.Run("MLLP Service Integration", func(t *testing.T) {
		if ts.postgresDB == nil {
			t.Skip("PostgreSQL connection required for MLLP integration testing")
		}

		// Create configuration for MLLP interface
		config := createMLLPInterfaceConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save MLLP configuration")

		// Verify MLLP configuration can be loaded and processed
		loadedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load MLLP configuration")

		// Validate MLLP-specific settings
		assert.Equal(t, "mllp", loadedConfig.Pipeline.Input.Type, "Input type should be MLLP")
		mllpConfig := loadedConfig.Pipeline.Input.ConnectorConfig
		assert.Contains(t, mllpConfig, "host", "MLLP config should contain host")
		assert.Contains(t, mllpConfig, "port", "MLLP config should contain port")

		// Test message processing with MLLP configuration
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "MLLP message processing should succeed")
		assert.NotEmpty(t, processedMessage.ID, "Processed message should have ID")
		assert.Equal(t, "mllp", processedMessage.Metadata.SourceType, "Source type should be MLLP")
	})

	t.Run("HL7-FHIR Transformation Integration", func(t *testing.T) {
		// Create configuration that uses existing HL7-FHIR transformation services
		config := createHL7FHIRTransformationConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save HL7-FHIR configuration")

		// Test transformation processing
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "HL7-FHIR transformation should succeed")

		// Verify transformation occurred
		assert.NotEmpty(t, processedMessage.ProcessingSteps, "Should have processing steps")
		transformationStep := findProcessingStep(processedMessage.ProcessingSteps, "transformation")
		assert.NotNil(t, transformationStep, "Should have transformation step")
		assert.Equal(t, "completed", transformationStep.Status, "Transformation should complete successfully")
	})

	t.Run("Business Logic Layer Integration", func(t *testing.T) {
		// Test integration with existing business logic services
		config := createBusinessLogicConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save business logic configuration")

		// Process message with business rules
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "Business logic processing should succeed")

		// Verify business logic processing
		businessLogicStep := findProcessingStep(processedMessage.ProcessingSteps, "business_logic")
		assert.NotNil(t, businessLogicStep, "Should have business logic step")
		assert.Equal(t, "completed", businessLogicStep.Status, "Business logic should complete successfully")
	})

	t.Run("Node.js Wizard Compatibility", func(t *testing.T) {
		// Test that configurations created through Node.js wizard are compatible
		config := createWizardCompatibleConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		// Save configuration that mimics wizard output
		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save wizard-compatible configuration")

		// Verify wizard mapping template is preserved
		loadedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load wizard configuration")

		assert.Equal(t, "standard_adt_v4", loadedConfig.Pipeline.Transformation.MappingTemplate,
			"Wizard mapping template should be preserved")
		assert.NotEmpty(t, loadedConfig.Pipeline.Transformation.CustomMappings,
			"Custom mappings from wizard should be preserved")

		// Verify processing works with wizard configuration
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "Wizard configuration processing should succeed")
		assert.NotEmpty(t, processedMessage.ProcessingSteps, "Should complete all processing steps")
	})
}

// TestDatabaseIntegration tests database-related integrations
func TestDatabaseIntegration(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	if ts.postgresDB == nil {
		t.Skip("PostgreSQL connection required for database integration testing")
	}

	t.Run("PostgreSQL Interface Table Integration", func(t *testing.T) {
		// Test that interface-specific tables are properly handled
		config := createDatabaseIntegrationConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save database configuration")

		// Process message that should be stored in interface table
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "Database integration processing should succeed")

		// Verify database destination was processed
		deliveryStep := findProcessingStep(processedMessage.ProcessingSteps, "delivery")
		assert.NotNil(t, deliveryStep, "Should have delivery step")
		assert.Equal(t, "completed", deliveryStep.Status, "Delivery should complete successfully")
	})

	t.Run("Message Metadata Preservation", func(t *testing.T) {
		// Test that message metadata is properly preserved through the pipeline
		config := createMetadataPreservationConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save metadata configuration")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		hl7Message := createHL7ADTMessage()
		processedMessage, err := ts.interfaceEngine.ProcessMessage(ctx, config.InterfaceID, []byte(hl7Message))
		require.NoError(t, err, "Metadata preservation processing should succeed")

		// Verify metadata is populated
		assert.NotEmpty(t, processedMessage.Metadata.MessageType, "Message type should be identified")
		assert.NotEmpty(t, processedMessage.Metadata.SourceType, "Source type should be set")
		assert.Greater(t, processedMessage.Metadata.MessageSize, 0, "Message size should be calculated")
		assert.NotEmpty(t, processedMessage.Metadata.Encoding, "Encoding should be detected")
	})
}

// TestBackwardsCompatibility ensures new system works with existing data
func TestBackwardsCompatibility(t *testing.T) {
	ts := SetupTestSuite(t)
	defer ts.TeardownTestSuite(t)

	if ts.postgresDB == nil {
		t.Skip("PostgreSQL connection required for backwards compatibility testing")
	}

	t.Run("Existing Interface Migration", func(t *testing.T) {
		// Test that existing interfaces can be migrated to new configuration format
		// This would typically load data from PostgreSQL and convert it

		if ts.migrationService == nil {
			t.Skip("Migration service not available")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get migration progress to verify service is working
		progress, err := ts.migrationService.GetMigrationProgress(ctx)
		require.NoError(t, err, "Migration service should be accessible")
		assert.NotNil(t, progress, "Migration progress should be available")

		// Log current state for debugging
		t.Logf("Migration progress: %d/%d interfaces migrated",
			progress.MigratedInterfaces, progress.TotalInterfaces)
	})

	t.Run("Wizard Mapping Preservation", func(t *testing.T) {
		// Test that existing wizard mappings are preserved
		config := createLegacyWizardConfiguration()
		ts.testConfigs = append(ts.testConfigs, config)

		err := ts.configManager.SaveConfig(config)
		require.NoError(t, err, "Failed to save legacy wizard configuration")

		// Load and verify mapping preservation
		loadedConfig, err := ts.configManager.LoadConfig(config.InterfaceID)
		require.NoError(t, err, "Failed to load legacy configuration")

		// Verify critical wizard mappings are preserved
		assert.Contains(t, loadedConfig.Pipeline.Transformation.CustomMappings,
			services.FieldMapping{
				SourceField: "MSH.3",
				TargetField: "MessageHeader.source.name",
			}, "MSH.3 mapping should be preserved")

		assert.Contains(t, loadedConfig.Pipeline.Transformation.CustomMappings,
			services.FieldMapping{
				SourceField: "PID.5",
				TargetField: "Patient.name",
			}, "PID.5 mapping should be preserved")
	})
}

// Helper functions to create specific test configurations

func createMLLPInterfaceConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "MLLP Integration Test Interface"
	config.InterfaceID = uuid.New().String()

	// MLLP-specific configuration
	config.Pipeline.Input = services.InputConfig{
		Type: "mllp",
		ConnectorConfig: map[string]interface{}{
			"host":              "0.0.0.0",
			"port":              2575,
			"max_connections":   50,
			"read_timeout_ms":   30000,
			"write_timeout_ms":  10000,
			"keep_alive":        true,
			"acknowledgment":    true,
			"character_set":     "UTF-8",
		},
	}

	return config
}

func createHL7FHIRTransformationConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "HL7-FHIR Transformation Test Interface"
	config.InterfaceID = uuid.New().String()

	// Focus on transformation configuration
	config.Pipeline.Transformation = services.TransformationConfig{
		Engine:          "hl7_to_fhir",
		MappingTemplate: "standard_adt_v4",
		CustomMappings: []services.FieldMapping{
			{
				SourceField: "MSH.3",
				TargetField: "MessageHeader.source.name",
				Transformation: services.TransformationStep{
					Type: "direct",
				},
			},
			{
				SourceField: "PID.5",
				TargetField: "Patient.name",
				Transformation: services.TransformationStep{
					Type:   "name_formatter",
					Config: map[string]interface{}{"format": "fhir_humanname"},
				},
			},
		},
	}

	return config
}

func createBusinessLogicConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "Business Logic Test Interface"
	config.InterfaceID = uuid.New().String()

	// Enhanced business logic configuration
	config.Pipeline.BusinessLogic = services.BusinessLogicConfig{
		RulesEngine: services.RulesEngineConfig{
			Enabled: true,
			Rules: []services.BusinessLogicRule{
				{
					RuleID:    "patient_id_validation",
					Condition: "Patient.identifier[0].value != null",
					Action: map[string]interface{}{
						"type":    "validate",
						"message": "Patient ID is required",
					},
				},
				{
					RuleID:    "duplicate_patient_check",
					Condition: "Patient.identifier exists",
					Action: map[string]interface{}{
						"type":   "deduplicate",
						"method": "merge_latest",
					},
				},
			},
		},
		WorkflowAutomation: []services.WorkflowAction{
			{
				Trigger: "patient_admitted",
				Actions: []map[string]interface{}{
					{
						"type":      "notify",
						"recipient": "nursing_station",
						"message":   "New patient admission",
					},
				},
			},
		},
	}

	return config
}

func createWizardCompatibleConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "Wizard Compatible Test Interface"
	config.InterfaceID = uuid.New().String()

	// Mimic wizard-generated configuration
	config.Pipeline.Transformation = services.TransformationConfig{
		Engine:          "hl7_to_fhir",
		MappingTemplate: "standard_adt_v4",
		CustomMappings: []services.FieldMapping{
			// Typical wizard mappings
			{
				SourceField: "MSH.3",
				TargetField: "MessageHeader.source.name",
				Transformation: services.TransformationStep{
					Type: "direct",
				},
			},
			{
				SourceField: "MSH.4",
				TargetField: "MessageHeader.source.endpoint",
				Transformation: services.TransformationStep{
					Type: "direct",
				},
			},
			{
				SourceField: "PID.3",
				TargetField: "Patient.identifier",
				Transformation: services.TransformationStep{
					Type:   "identifier_formatter",
					Config: map[string]interface{}{"system": "http://hospital.local/mrn"},
				},
			},
			{
				SourceField: "PID.5",
				TargetField: "Patient.name",
				Transformation: services.TransformationStep{
					Type:   "name_formatter",
					Config: map[string]interface{}{"format": "fhir_humanname"},
				},
			},
			{
				SourceField: "PID.7",
				TargetField: "Patient.birthDate",
				Transformation: services.TransformationStep{
					Type:   "date_formatter",
					Config: map[string]interface{}{"format": "YYYY-MM-DD"},
				},
			},
		},
	}

	return config
}

func createDatabaseIntegrationConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "Database Integration Test Interface"
	config.InterfaceID = uuid.New().String()

	// Add database destination
	config.Pipeline.Destinations = append(config.Pipeline.Destinations, services.DestinationConfig{
		DestinationID: uuid.New().String(),
		Type:          "database",
		Config: map[string]interface{}{
			"table_name":       fmt.Sprintf("messages_intf_%s", config.InterfaceID[:8]),
			"connection_pool":  5,
			"batch_insert":     true,
			"batch_size":       20,
			"include_metadata": true,
		},
		ErrorHandling: services.ErrorHandlingConfig{
			RetryCount:      3,
			RetryDelay:      1000,
			DeadLetterQueue: true,
		},
	})

	return config
}

func createMetadataPreservationConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "Metadata Preservation Test Interface"
	config.InterfaceID = uuid.New().String()

	// Configure input to capture detailed metadata
	config.Pipeline.Input.ConnectorConfig["capture_source_ip"] = true
	config.Pipeline.Input.ConnectorConfig["capture_timestamps"] = true
	config.Pipeline.Input.ConnectorConfig["preserve_encoding"] = true

	// Add metadata enrichment in post-processing
	config.Pipeline.Transformation.PostProcessing = []services.PostProcessingStep{
		{
			Type: "add_metadata",
			Config: map[string]interface{}{
				"source_system":    "test_system",
				"processing_node":  "test_node",
				"interface_id":     config.InterfaceID,
				"correlation_id":   true,
			},
		},
	}

	return config
}

func createLegacyWizardConfiguration() *services.InterfaceConfig {
	config := createAdvancedTestConfiguration()
	config.Name = "Legacy Wizard Test Interface"
	config.InterfaceID = uuid.New().String()

	// Simulate legacy wizard mappings
	config.Pipeline.Transformation = services.TransformationConfig{
		Engine:          "hl7_to_fhir",
		MappingTemplate: "standard_adt_v4",
		CustomMappings: []services.FieldMapping{
			{
				SourceField: "MSH.3",
				TargetField: "MessageHeader.source.name",
				Transformation: services.TransformationStep{
					Type: "direct",
				},
			},
			{
				SourceField: "PID.5",
				TargetField: "Patient.name",
				Transformation: services.TransformationStep{
					Type: "direct",
				},
			},
		},
	}

	return config
}

// Helper function to find a processing step by type
func findProcessingStep(steps []services.ProcessingStep, stepType string) *services.ProcessingStep {
	for i := range steps {
		if steps[i].StepType == stepType {
			return &steps[i]
		}
	}
	return nil
}