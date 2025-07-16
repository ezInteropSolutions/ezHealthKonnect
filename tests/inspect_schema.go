// Add this to your project - accepts command line arguments

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"schemas/hl7" // Adjust import path
)

func main() {
	// Define command line flags
	var (
		schemaDir  = flag.String("dir", "./schemas/hl7", "Schema directory path")
		schemaFile = flag.String("file", "", "Specific schema file to inspect")
		showPaths  = flag.Bool("paths", false, "Show all JSON paths")
		showAll    = flag.Bool("all", false, "Inspect all schemas in directory")
	)
	flag.Parse()

	fmt.Printf("🔍 Schema Structure Inspector\n")
	fmt.Printf("============================\n\n")

	// Option 1: Inspect specific file
	if *schemaFile != "" {
		fmt.Printf("📄 Inspecting file: %s\n", *schemaFile)

		if *showPaths {
			if err := hl7.ShowJSONPaths(*schemaFile); err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := hl7.InspectSchemaStructure(*schemaFile); err != nil {
				fmt.Printf("❌ Error: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	// Option 2: Inspect all schemas in directory
	if *showAll {
		fmt.Printf("📁 Inspecting all schemas in: %s\n", *schemaDir)
		if err := hl7.InspectAllSchemas(*schemaDir); err != nil {
			fmt.Printf("❌ Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Option 3: Interactive mode
	runInteractiveInspector(*schemaDir)
}

func runInteractiveInspector(schemaDir string) {
	fmt.Printf("📁 Schema directory: %s\n\n", schemaDir)

	// List available schema files
	schemaFiles, err := filepath.Glob(filepath.Join(schemaDir, "**", "*.gz"))
	if err != nil {
		fmt.Printf("❌ Error listing files: %v\n", err)
		return
	}

	if len(schemaFiles) == 0 {
		fmt.Printf("❌ No schema files found in %s\n", schemaDir)
		fmt.Printf("💡 Expected structure: %s/v2.5.1/ADT_A04.gz\n", schemaDir)
		return
	}

	fmt.Printf("📋 Available schema files:\n")
	for i, file := range schemaFiles {
		fmt.Printf("  %d. %s\n", i+1, file)
	}

	fmt.Printf("\nSelect options:\n")
	fmt.Printf("  📄 Inspect specific file: --file=path/to/schema.gz\n")
	fmt.Printf("  📁 Inspect all files: --all\n")
	fmt.Printf("  🗺️  Show JSON paths: --file=path/to/schema.gz --paths\n")
	fmt.Printf("\nExample usage:\n")
	fmt.Printf("  go run main.go --file=%s\n", schemaFiles[0])
	fmt.Printf("  go run main.go --all\n")
	fmt.Printf("  go run main.go --file=%s --paths\n", schemaFiles[0])
}
