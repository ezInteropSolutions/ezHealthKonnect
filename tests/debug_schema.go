// FILE: debug_schema.go
// Standalone debug script to test schema loader initialization
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"ezhealthkonnect/config"
	"ezhealthkonnect/hl7"
)

func main() {
	fmt.Printf("🔧 Schema Loader Debug Script\n")
	fmt.Printf("=============================\n\n")

	// Load configuration
	cfg := config.Load()
	schemaDir := cfg.GetSchemaDirectory()

	fmt.Printf("1. Configuration Check:\n")
	fmt.Printf("   Schema Directory: %s\n", schemaDir)
	fmt.Printf("   Use Filesystem: %v\n", cfg.UseFilesystemSchema())
	fmt.Printf("   Schema Source: %s\n", cfg.SchemaSource)

	// Check if directory exists
	fmt.Printf("\n2. Directory Check:\n")
	if _, err := os.Stat(schemaDir); os.IsNotExist(err) {
		fmt.Printf("   ❌ Directory does not exist: %s\n", schemaDir)
		fmt.Printf("   💡 Creating directory...\n")
		if err := os.MkdirAll(schemaDir, 0755); err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
		fmt.Printf("   ✅ Directory created\n")
	} else {
		fmt.Printf("   ✅ Directory exists: %s\n", schemaDir)
	}

	// Scan for existing files
	fmt.Printf("\n3. File Scan:\n")
	err := filepath.Walk(schemaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			relPath, _ := filepath.Rel(schemaDir, path)
			fmt.Printf("   📄 Found file: %s\n", relPath)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("   ⚠️ Error scanning directory: %v\n", err)
	}

	// Check config in detail
	fmt.Printf("\n4. Detailed Config Check:\n")
	fmt.Printf("   SchemaSource: '%s'\n", cfg.SchemaSource)
	fmt.Printf("   UseFilesystemSchema(): %v\n", cfg.UseFilesystemSchema())
	fmt.Printf("   SchemaDirectory: '%s'\n", cfg.SchemaDirectory)

	// Initialize schema loader manually for testing
	fmt.Printf("\n5. Manual Schema Loader Initialization:\n")
	fmt.Printf("   🚀 Calling InitSchemaLoader...\n")
	hl7.InitSchemaLoader(schemaDir)

	// Check if real loader was created
	fmt.Printf("\n6. Real Loader Check:\n")
	realLoader := hl7.GetRealSchemaLoader()
	fmt.Printf("   Real Loader: %p\n", realLoader)
	fmt.Printf("   Not Nil: %v\n", realLoader != nil)

	if realLoader != nil {
		stats := realLoader.GetStats()
		fmt.Printf("   Cache Size: %d\n", stats.CacheSize)
		fmt.Printf("   Total Loads: %d\n", stats.TotalLoads)
		fmt.Printf("   Load Errors: %d\n", stats.LoadErrors)
	}

	// Test schema loading
	fmt.Printf("\n7. Schema Loading Test:\n")
	if realLoader != nil {
		schema, err := realLoader.LoadRealSchema("2.5.1", "ADT", "A04")
		if err != nil {
			fmt.Printf("   ❌ Failed to load schema: %v\n", err)
		} else {
			fmt.Printf("   ✅ Schema loaded successfully\n")
			fmt.Printf("   Schema Type: %s\n", schema.MessageType)
			fmt.Printf("   Segments: %d\n", len(schema.Segments))
		}
	}

	// Test enhanced parsing
	fmt.Printf("\n8. Enhanced Parsing Test:\n")
	testMessage := `MSH|^~\&|SENDER|SENDERFAC|RECEIVER|RECEIVERFAC|20240710120000||ADT^A04|12345|P|2.5.1
PID|1||123456789^^^MRN||DOE^JOHN^MIDDLE||19800101|M|||123 MAIN ST^^CITY^ST^12345||555-1234|||||||123456789`

	result := hl7.ParseHL7Enhanced(testMessage)
	if result != nil && result.Success {
		fmt.Printf("   ✅ Enhanced parsing successful\n")
		fmt.Printf("   Message Type: %s^%s\n", result.MessageType.Code, result.MessageType.Event)
		fmt.Printf("   Enhanced Segments Found: %d\n", len(result.EnhancedSegments))
		fmt.Printf("   Dictionary Used: %v\n", result.DictionaryUsed)
		fmt.Printf("   Schema Loaded: %v\n", result.SchemaLoaded)

		// Show segment details
		for segName, segment := range result.EnhancedSegments {
			fmt.Printf("   📋 %s: %s (%d fields)\n", segName, segment.Description, segment.FieldCount)
		}
	} else {
		fmt.Printf("   ❌ Enhanced parsing failed\n")
		if result != nil {
			fmt.Printf("   Error: %s\n", result.Error)
		}
	}

	// Show status
	fmt.Printf("\n9. Final Status:\n")
	status := hl7.GetSchemaLoaderStatus()
	for key, value := range status {
		fmt.Printf("   %s: %v\n", key, value)
	}

	fmt.Printf("\n✅ Debug script completed\n")
}
