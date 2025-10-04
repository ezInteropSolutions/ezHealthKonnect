// mapping_generator/cmd/main.go
// CLI tool for generating OOB HL7-FHIR mapping templates
// Usage: go run main.go -schemas ../schemas -output ../templates

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mapping_generator"
)

func main() {
	var (
		schemasPath    = flag.String("schemas", "../schemas", "Path to HL7/FHIR schemas directory")
		outputPath     = flag.String("output", "../templates", "Output directory for generated templates")
		minConfidence  = flag.Float64("confidence", 0.85, "Minimum confidence threshold for mappings")
		messageTypes   = flag.String("types", "", "Comma-separated message types (default: all)")
		hl7Versions    = flag.String("versions", "2.3,2.5.1", "Comma-separated HL7 versions")
		verbose        = flag.Bool("verbose", false, "Enable verbose output")
		validate       = flag.Bool("validate", true, "Validate generated templates")
		help           = flag.Bool("help", false, "Show help message")
	)

	flag.Parse()

	if *help {
		printUsage()
		return
	}

	fmt.Println("🚀 HL7-FHIR OOB Template Generator")
	fmt.Println("==================================")

	// Validate inputs
	if err := validateInputs(*schemasPath, *outputPath); err != nil {
		log.Fatal("❌ Validation error:", err)
	}

	// Create template generator
	generator := mapping_generator.NewTemplateGenerator(*schemasPath, *outputPath)

	// Configure generation
	config := createGenerationConfig(*outputPath, *minConfidence, *messageTypes, *hl7Versions, *validate)

	if *verbose {
		printConfig(config)
	}

	fmt.Printf("📂 Schemas path: %s\n", *schemasPath)
	fmt.Printf("📁 Output path: %s\n", *outputPath)
	fmt.Printf("🎯 Confidence threshold: %.2f\n", *minConfidence)
	fmt.Println()

	// Generate templates
	fmt.Println("🔄 Starting template generation...")
	startTime := time.Now()

	results, err := generator.GenerateOOBTemplates(config)
	if err != nil {
		log.Fatal("❌ Generation failed:", err)
	}

	// Print results
	generator.PrintGenerationReport(results)

	// Print success summary
	fmt.Printf("\n🎉 Template generation completed successfully!\n")
	fmt.Printf("⏱️  Total time: %v\n", time.Since(startTime))
	fmt.Printf("📊 Success rate: %.1f%% (%d/%d)\n",
		float64(results.SuccessfulTemplates)/float64(results.TotalTemplates)*100,
		results.SuccessfulTemplates,
		results.TotalTemplates)

	if results.FailedTemplates > 0 {
		fmt.Printf("⚠️  %d templates failed to generate\n", results.FailedTemplates)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("HL7-FHIR OOB Template Generator")
	fmt.Println("===============================")
	fmt.Println()
	fmt.Println("Generates Out-of-Box mapping templates for standard HL7 message types.")
	fmt.Println("Templates serve as high-confidence starting points for interface implementations.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run main.go [options]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Generate all templates with default settings")
	fmt.Println("  go run main.go")
	fmt.Println()
	fmt.Println("  # Generate specific message types")
	fmt.Println("  go run main.go -types \"ADT^A01,ORU^R01\" -confidence 0.90")
	fmt.Println()
	fmt.Println("  # Generate for specific HL7 versions")
	fmt.Println("  go run main.go -versions \"2.5.1\" -output ./production_templates")
	fmt.Println()
	fmt.Println("Supported Message Types:")
	fmt.Println("  ADT^A01, ADT^A04, ADT^A08, ADT^A03 (Admission/Discharge/Transfer)")
	fmt.Println("  ORU^R01                           (Observation Results)")
	fmt.Println("  ORM^O01                           (Order Message)")
	fmt.Println("  MDM^T01, MDM^T02                  (Medical Document Management)")
	fmt.Println("  SIU^S12                           (Scheduling Information)")
	fmt.Println("  DFT^P03                           (Detailed Financial Transaction)")
	fmt.Println("  RDE^O11                           (Pharmacy/Treatment Encoded Order)")
	fmt.Println()
	fmt.Println("Target FHIR Resources:")
	fmt.Println("  Patient, Encounter, Observation, ServiceRequest, DocumentReference,")
	fmt.Println("  Appointment, MedicationRequest, Account, ChargeItem, MessageHeader")
}

func validateInputs(schemasPath, outputPath string) error {
	// Check if schemas directory exists
	if _, err := os.Stat(schemasPath); os.IsNotExist(err) {
		return fmt.Errorf("schemas directory does not exist: %s", schemasPath)
	}

	// Check for required schema subdirectories
	hl7Dir := filepath.Join(schemasPath, "hl7")
	if _, err := os.Stat(hl7Dir); os.IsNotExist(err) {
		return fmt.Errorf("HL7 schemas directory not found: %s", hl7Dir)
	}

	fhirDir := filepath.Join(schemasPath, "fhir")
	if _, err := os.Stat(fhirDir); os.IsNotExist(err) {
		return fmt.Errorf("FHIR schemas directory not found: %s", fhirDir)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	return nil
}

func createGenerationConfig(outputPath string, minConfidence float64, messageTypes, hl7Versions string, validate bool) mapping_generator.GenerationConfig {
	config := mapping_generator.GenerationConfig{
		OutputPath:        outputPath,
		MinConfidence:     minConfidence,
		IncludeMetadata:   true,
		CompressOutput:    false,
		ValidateTemplates: validate,
	}

	// Parse message types
	if messageTypes == "" {
		// Use default comprehensive set
		config.MessageTypes = []string{
			"ADT^A01", "ADT^A04", "ADT^A08", "ADT^A03", // ADT family
			"ORU^R01",                                   // Lab results
			"ORM^O01",                                   // Orders
			"MDM^T01", "MDM^T02",                       // Documents
			"SIU^S12",                                   // Scheduling
			"DFT^P03",                                   // Financial
			"RDE^O11",                                   // Pharmacy orders
		}
	} else {
		config.MessageTypes = parseCommaSeparated(messageTypes)
	}

	// Parse HL7 versions
	if hl7Versions == "" {
		config.HL7Versions = []string{"2.3", "2.5.1"}
	} else {
		config.HL7Versions = parseCommaSeparated(hl7Versions)
	}

	return config
}

func parseCommaSeparated(input string) []string {
	var result []string
	for _, item := range strings.Split(input, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func printConfig(config mapping_generator.GenerationConfig) {
	fmt.Println("📋 Generation Configuration:")
	fmt.Printf("   Message Types: %v\n", config.MessageTypes)
	fmt.Printf("   HL7 Versions: %v\n", config.HL7Versions)
	fmt.Printf("   Min Confidence: %.2f\n", config.MinConfidence)
	fmt.Printf("   Include Metadata: %t\n", config.IncludeMetadata)
	fmt.Printf("   Validate Templates: %t\n", config.ValidateTemplates)
	fmt.Println()
}