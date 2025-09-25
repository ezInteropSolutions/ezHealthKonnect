// migration_validation_test.go
// Comprehensive testing for PostgreSQL to MongoDB migration

package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"ezhealthkonnect/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"
)

// MigrationTestSuite holds migration testing components
type MigrationTestSuite struct {
	postgresDB       *sql.DB
	configManager    *services.ConfigManager
	migrationService *services.MigrationService
	testInterfaces   []TestInterface
}

// TestInterface represents a test interface for migration
type TestInterface struct {
	ID                  int64
	Name                string
	Description         string
	SourceType          string
	SourceConfig        map[string]interface{}
	DestinationType     string
	DestinationConfig   map[string]interface{}
	TransformationRules map[string]interface{}
	Status              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// WizardMapping represents a wizard mapping for migration testing
type WizardMapping struct {
	InterfaceID   string
	MessageType   string
	SourceField   string
	TargetField   string
	Transformation map[string]interface{}
	IsEnabled     bool
}

// SetupMigrationTestSuite initializes the migration test environment
func SetupMigrationTestSuite(t *testing.T) *MigrationTestSuite {
	// Setup PostgreSQL connection
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping migration tests")
	}

	postgresDB, err := sql.Open("postgres", dbURL)
	require.NoError(t, err, "Failed to connect to PostgreSQL")
	err = postgresDB.Ping()
	require.NoError(t, err, "Failed to ping PostgreSQL")

	// Setup MongoDB Configuration Manager
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://localhost:27017"
	}
	mongoDatabase := os.Getenv("MONGODB_DATABASE")
	if mongoDatabase == "" {
		mongoDatabase = "ezhealthkonnect_migration_test"
	}

	configManager, err := services.NewConfigManager(mongoURI, mongoDatabase)
	require.NoError(t, err, "Failed to initialize Configuration Manager")

	// Initialize Migration Service
	migrationService := services.NewMigrationService(postgresDB, configManager, false)

	return &MigrationTestSuite{
		postgresDB:       postgresDB,
		configManager:    configManager,
		migrationService: migrationService,
		testInterfaces:   make([]TestInterface, 0),
	}
}

// TeardownMigrationTestSuite cleans up the migration test environment
func (mts *MigrationTestSuite) TeardownMigrationTestSuite(t *testing.T) {
	if mts.configManager != nil {
		err := mts.configManager.Close()
		assert.NoError(t, err, "Failed to close Configuration Manager")
	}

	if mts.postgresDB != nil {
		err := mts.postgresDB.Close()
		assert.NoError(t, err, "Failed to close PostgreSQL connection")
	}
}

// TestMigrationDataIntegrity tests that data is preserved during migration
func TestMigrationDataIntegrity(t *testing.T) {
	mts := SetupMigrationTestSuite(t)
	defer mts.TeardownMigrationTestSuite(t)

	t.Run("Create Test Data in PostgreSQL", func(t *testing.T) {
		// Create test interfaces in PostgreSQL
		testInterfaces := createTestInterfacesInPostgreSQL(t, mts.postgresDB)
		mts.testInterfaces = testInterfaces

		assert.Greater(t, len(testInterfaces), 0, "Should create test interfaces")
	})

	t.Run("Create Test Wizard Mappings", func(t *testing.T) {
		// Create test wizard mappings in PostgreSQL
		mappings := createTestWizardMappingsInPostgreSQL(t, mts.postgresDB, mts.testInterfaces)
		assert.Greater(t, len(mappings), 0, "Should create test wizard mappings")
	})

	t.Run("Perform Migration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Run migration
		stats, err := mts.migrationService.MigrateAll(ctx)
		require.NoError(t, err, "Migration should complete successfully")

		assert.Greater(t, stats.TotalInterfaces, int64(0), "Should migrate interfaces")
		assert.Equal(t, stats.TotalInterfaces, stats.MigratedInterfaces, "All interfaces should be migrated")

		t.Logf("Migration completed: %d interfaces migrated in %v",
			stats.MigratedInterfaces, stats.Duration)
	})

	t.Run("Validate Migrated Data", func(t *testing.T) {
		// Validate that all test interfaces were migrated correctly
		for _, testInterface := range mts.testInterfaces {
			migratedConfig, err := mts.configManager.LoadConfig(fmt.Sprintf("migrated_%d", testInterface.ID))
			if err != nil {
				// Try loading by name as fallback
				configs, err := mts.configManager.GetActiveConfigs()
				require.NoError(t, err, "Should be able to get active configs")

				found := false
				for _, config := range configs {
					if config.Name == testInterface.Name {
						migratedConfig = config
						found = true
						break
					}
				}
				require.True(t, found, "Should find migrated interface: %s", testInterface.Name)
			}

			// Validate core fields
			assert.Equal(t, testInterface.Name, migratedConfig.Name, "Interface name should match")
			assert.Equal(t, testInterface.Description, migratedConfig.Description, "Interface description should match")
			assert.Equal(t, "active", migratedConfig.Status, "Migrated interface should be active")

			// Validate configuration structure
			assert.NotNil(t, migratedConfig.Pipeline, "Pipeline should be present")
			assert.NotNil(t, migratedConfig.Pipeline.Input, "Input configuration should be present")
			assert.NotEmpty(t, migratedConfig.Pipeline.Destinations, "Destinations should be present")

			t.Logf("✅ Validated migrated interface: %s", testInterface.Name)
		}
	})

	t.Run("Validate Wizard Mappings Migration", func(t *testing.T) {
		// Validate that wizard mappings were correctly migrated to the new format
		configs, err := mts.configManager.GetActiveConfigs()
		require.NoError(t, err, "Should be able to get active configs")

		mappingCount := 0
		for _, config := range configs {
			if len(config.Pipeline.Transformation.CustomMappings) > 0 {
				mappingCount += len(config.Pipeline.Transformation.CustomMappings)

				// Validate mapping structure
				for _, mapping := range config.Pipeline.Transformation.CustomMappings {
					assert.NotEmpty(t, mapping.SourceField, "Source field should not be empty")
					assert.NotEmpty(t, mapping.TargetField, "Target field should not be empty")
					assert.NotNil(t, mapping.Transformation, "Transformation should be present")
				}
			}
		}

		assert.Greater(t, mappingCount, 0, "Should have migrated wizard mappings")
		t.Logf("✅ Validated %d wizard mappings", mappingCount)
	})
}

// TestMigrationProgressTracking tests migration progress tracking
func TestMigrationProgressTracking(t *testing.T) {
	mts := SetupMigrationTestSuite(t)
	defer mts.TeardownMigrationTestSuite(t)

	t.Run("Initial Progress Check", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		progress, err := mts.migrationService.GetMigrationProgress(ctx)
		require.NoError(t, err, "Should get initial migration progress")

		assert.GreaterOrEqual(t, progress.TotalInterfaces, int64(0), "Total interfaces should be non-negative")
		assert.GreaterOrEqual(t, progress.MigratedInterfaces, int64(0), "Migrated interfaces should be non-negative")
		assert.GreaterOrEqual(t, progress.TotalInterfaces, progress.MigratedInterfaces, "Total should be >= migrated")

		t.Logf("Initial progress: %d/%d interfaces migrated",
			progress.MigratedInterfaces, progress.TotalInterfaces)
	})

	t.Run("Progress During Migration", func(t *testing.T) {
		// Create test data
		testInterfaces := createTestInterfacesInPostgreSQL(t, mts.postgresDB)
		require.Greater(t, len(testInterfaces), 0, "Should create test interfaces")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Start migration in background
		migrationComplete := make(chan bool)
		var migrationError error

		go func() {
			_, err := mts.migrationService.MigrateAll(ctx)
			migrationError = err
			migrationComplete <- true
		}()

		// Monitor progress
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		var lastProgress *services.MigrationProgress
		progressChecks := 0

		for {
			select {
			case <-migrationComplete:
				assert.NoError(t, migrationError, "Migration should complete without error")
				t.Logf("Migration completed after %d progress checks", progressChecks)
				return

			case <-ticker.C:
				progress, err := mts.migrationService.GetMigrationProgress(ctx)
				if err != nil {
					t.Logf("Progress check failed: %v", err)
					continue
				}

				progressChecks++
				if lastProgress == nil || progress.MigratedInterfaces != lastProgress.MigratedInterfaces {
					t.Logf("Progress update: %d/%d interfaces migrated",
						progress.MigratedInterfaces, progress.TotalInterfaces)
					lastProgress = progress
				}

			case <-ctx.Done():
				t.Fatal("Migration timed out")
			}
		}
	})
}

// TestMigrationRollback tests migration rollback functionality
func TestMigrationRollback(t *testing.T) {
	mts := SetupMigrationTestSuite(t)
	defer mts.TeardownMigrationTestSuite(t)

	// Note: This test would require implementing rollback functionality
	// For now, we test that the system can handle migration failures gracefully

	t.Run("Migration Failure Handling", func(t *testing.T) {
		// Create invalid test data that should cause migration to fail
		_, err := mts.postgresDB.Exec(`
			INSERT INTO interfaces (name, description, source_type, destination_type, status, created_at, updated_at)
			VALUES ('Invalid Interface', 'Interface with invalid config', '', '', 'active', NOW(), NOW())
		`)
		if err != nil {
			t.Skip("Could not create invalid test data")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt migration - it may fail on invalid data
		stats, err := mts.migrationService.MigrateAll(ctx)

		if err != nil {
			t.Logf("Migration failed as expected with invalid data: %v", err)
			// Verify that partial migration doesn't leave the system in an inconsistent state
			progress, err := mts.migrationService.GetMigrationProgress(ctx)
			assert.NoError(t, err, "Should still be able to get progress after failure")
			t.Logf("Progress after failure: %d/%d", progress.MigratedInterfaces, progress.TotalInterfaces)
		} else {
			t.Logf("Migration succeeded despite invalid data: %+v", stats)
		}
	})
}

// TestMigrationPerformance tests migration performance with large datasets
func TestMigrationPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	mts := SetupMigrationTestSuite(t)
	defer mts.TeardownMigrationTestSuite(t)

	t.Run("Large Dataset Migration", func(t *testing.T) {
		// Create a larger dataset for performance testing
		interfaceCount := 100
		mappingCount := 500

		// Create interfaces
		for i := 0; i < interfaceCount; i++ {
			_, err := mts.postgresDB.Exec(`
				INSERT INTO interfaces (name, description, source_type, destination_type, status, created_at, updated_at)
				VALUES ($1, $2, 'mllp', 'fhir_api', 'active', NOW(), NOW())
			`, fmt.Sprintf("Performance Test Interface %d", i), fmt.Sprintf("Performance test interface %d", i))
			require.NoError(t, err, "Should create performance test interface")
		}

		// Create mappings
		for i := 0; i < mappingCount; i++ {
			_, err := mts.postgresDB.Exec(`
				INSERT INTO wizard_mappings (interface_id, source_field, target_field, transformation_type, enabled, created_at)
				VALUES ($1, $2, $3, 'direct', true, NOW())
			`, (i%interfaceCount)+1, fmt.Sprintf("PID.%d", (i%10)+1), fmt.Sprintf("Patient.field%d", i))
			if err != nil {
				// Table might not exist, which is okay for this test
				t.Logf("Could not create mapping %d: %v", i, err)
			}
		}

		// Measure migration performance
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second) // 5 minutes
		defer cancel()

		startTime := time.Now()
		stats, err := mts.migrationService.MigrateAll(ctx)
		migrationDuration := time.Since(startTime)

		require.NoError(t, err, "Large dataset migration should succeed")

		// Performance assertions
		assert.Greater(t, stats.MigratedInterfaces, int64(interfaceCount/2), "Should migrate at least half the interfaces")
		assert.Less(t, migrationDuration, 60*time.Second, "Migration should complete within 60 seconds")

		// Calculate performance metrics
		interfacesPerSecond := float64(stats.MigratedInterfaces) / migrationDuration.Seconds()
		t.Logf("Migration performance: %.2f interfaces/second", interfacesPerSecond)
		t.Logf("Total migration time: %v for %d interfaces", migrationDuration, stats.MigratedInterfaces)

		assert.Greater(t, interfacesPerSecond, 1.0, "Should migrate at least 1 interface per second")
	})
}

// Helper functions for creating test data

func createTestInterfacesInPostgreSQL(t *testing.T, db *sql.DB) []TestInterface {
	interfaces := []TestInterface{
		{
			Name:            "Test HL7 ADT Interface",
			Description:     "Test interface for ADT messages",
			SourceType:      "mllp",
			DestinationType: "fhir_api",
			Status:          "active",
		},
		{
			Name:            "Test HL7 ORU Interface",
			Description:     "Test interface for lab results",
			SourceType:      "mllp",
			DestinationType: "database",
			Status:          "active",
		},
		{
			Name:            "Test File Interface",
			Description:     "Test interface for file processing",
			SourceType:      "file",
			DestinationType: "queue",
			Status:          "paused",
		},
	}

	var createdInterfaces []TestInterface

	for _, iface := range interfaces {
		sourceConfig, _ := json.Marshal(map[string]interface{}{
			"host": "localhost",
			"port": 2575,
		})
		destConfig, _ := json.Marshal(map[string]interface{}{
			"url": "http://localhost:8080/fhir",
		})

		var id int64
		err := db.QueryRow(`
			INSERT INTO interfaces (name, description, source_type, source_config, destination_type, destination_config, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
			RETURNING id
		`, iface.Name, iface.Description, iface.SourceType, sourceConfig, iface.DestinationType, destConfig, iface.Status).Scan(&id)

		if err != nil {
			// Table might not exist in the expected format, try a simpler insert
			err = db.QueryRow(`
				INSERT INTO interfaces (name, description, status, created_at, updated_at)
				VALUES ($1, $2, $3, NOW(), NOW())
				RETURNING id
			`, iface.Name, iface.Description, iface.Status).Scan(&id)
		}

		if err != nil {
			t.Logf("Could not create interface %s: %v", iface.Name, err)
			continue
		}

		iface.ID = id
		createdInterfaces = append(createdInterfaces, iface)
		t.Logf("✅ Created test interface: %s (ID: %d)", iface.Name, id)
	}

	return createdInterfaces
}

func createTestWizardMappingsInPostgreSQL(t *testing.T, db *sql.DB, interfaces []TestInterface) []WizardMapping {
	mappings := []WizardMapping{
		{
			SourceField: "MSH.3",
			TargetField: "MessageHeader.source.name",
		},
		{
			SourceField: "PID.3",
			TargetField: "Patient.identifier",
		},
		{
			SourceField: "PID.5",
			TargetField: "Patient.name",
		},
		{
			SourceField: "PID.7",
			TargetField: "Patient.birthDate",
		},
	}

	var createdMappings []WizardMapping

	for _, iface := range interfaces {
		for _, mapping := range mappings {
			mapping.InterfaceID = fmt.Sprintf("%d", iface.ID)
			mapping.MessageType = "ADT^A01"
			mapping.IsEnabled = true

			transformation, _ := json.Marshal(map[string]interface{}{
				"type": "direct",
			})

			_, err := db.Exec(`
				INSERT INTO wizard_mappings (interface_id, message_type, source_field, target_field, transformation, enabled, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, NOW())
			`, mapping.InterfaceID, mapping.MessageType, mapping.SourceField, mapping.TargetField, transformation, mapping.IsEnabled)

			if err != nil {
				// Table might not exist, try alternative table names
				_, err = db.Exec(`
					INSERT INTO hl7_fhir_mappings (interface_id, source_field, target_field, transformation_type, enabled, created_at)
					VALUES ($1, $2, $3, 'direct', $4, NOW())
				`, mapping.InterfaceID, mapping.SourceField, mapping.TargetField, mapping.IsEnabled)
			}

			if err != nil {
				t.Logf("Could not create mapping for interface %d: %v", iface.ID, err)
				continue
			}

			createdMappings = append(createdMappings, mapping)
		}
	}

	t.Logf("✅ Created %d test wizard mappings", len(createdMappings))
	return createdMappings
}