// services/transformation_fhir_validation.go
// FHIR Validation and Bundle Creation Methods
//
// 🎯 PURPOSE: FHIR resource validation, bundle creation, and quality assessment
package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// FHIR RESOURCE VALIDATION
// =====================================

// validateFHIRResources validates FHIR resources against profiles and schemas
func (s *FHIRTransformationService) validateFHIRResources(resources []FHIRResource, validationLevel, targetProfile string) ([]FHIRValidationResult, error) {
	var results []FHIRValidationResult

	for _, resource := range resources {
		result, err := s.validateFHIRResource(&resource, validationLevel, targetProfile)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to validate %s resource: %v", resource.ResourceType, err)
			continue
		}

		if result != nil {
			results = append(results, *result)
		}
	}

	log.Printf("✅ Validated %d FHIR resources", len(results))
	return results, nil
}

// validateFHIRResource validates a single FHIR resource
func (s *FHIRTransformationService) validateFHIRResource(resource *FHIRResource, validationLevel, targetProfile string) (*FHIRValidationResult, error) {
	startTime := time.Now()

	result := &FHIRValidationResult{
		ResourceID:   resource.ID,
		ResourceType: resource.ResourceType,
		IsValid:      true,
		ProfileValid: true,
		Severity:     "information",
		Issues:       []FHIRValidationIssue{},
		Score:        100.0,
		Timestamp:    startTime,
		ValidatorID:  "FHIRTransformationService",
		Metadata:     make(map[string]interface{}),
	}

	// Structural validation
	structuralIssues := s.validateResourceStructure(resource)
	result.Issues = append(result.Issues, structuralIssues...)

	// Profile validation
	if targetProfile != "base" {
		profileIssues := s.validateResourceProfile(resource, targetProfile)
		result.Issues = append(result.Issues, profileIssues...)
	}

	// Data type validation
	dataTypeIssues := s.validateResourceDataTypes(resource)
	result.Issues = append(result.Issues, dataTypeIssues...)

	// Required field validation
	requiredFieldIssues := s.validateRequiredFields(resource)
	result.Issues = append(result.Issues, requiredFieldIssues...)

	// Terminology validation
	if validationLevel == "STRICT" {
		terminologyIssues := s.validateResourceTerminology(resource)
		result.Issues = append(result.Issues, terminologyIssues...)
	}

	// Reference validation
	referenceIssues := s.validateResourceReferences(resource)
	result.Issues = append(result.Issues, referenceIssues...)

	// Calculate validation outcomes
	errorCount := 0
	warningCount := 0

	for _, issue := range result.Issues {
		switch issue.Severity {
		case "error":
			errorCount++
			result.Severity = "error"
			result.IsValid = false
		case "warning":
			warningCount++
			if result.Severity != "error" {
				result.Severity = "warning"
			}
		}
	}

	// Calculate validation score
	totalIssues := len(result.Issues)
	if totalIssues > 0 {
		passedChecks := totalIssues - errorCount - warningCount
		result.Score = float64(passedChecks) / float64(totalIssues) * 100.0
	}

	// Profile compliance check
	if targetProfile != "base" {
		profileCompliance := s.checkProfileCompliance(resource, targetProfile)
		result.ProfileValid = profileCompliance > 80.0
		result.Metadata["profileCompliance"] = profileCompliance
	}

	result.Metadata["validationTime"] = time.Since(startTime).String()
	result.Metadata["totalIssues"] = totalIssues
	result.Metadata["errorCount"] = errorCount
	result.Metadata["warningCount"] = warningCount

	// Update resource validation status
	resource.ValidationStatus = result.Severity
	resource.ValidationIssues = result.Issues

	return result, nil
}

// validateResourceStructure validates basic FHIR resource structure
func (s *FHIRTransformationService) validateResourceStructure(resource *FHIRResource) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	content := resource.Content

	// Check resourceType
	if resourceType, exists := content["resourceType"]; !exists {
		issues = append(issues, FHIRValidationIssue{
			Severity:    "error",
			Code:        "structure",
			Details:     "Resource must have a resourceType",
			Location:    "resourceType",
			Expression:  "resourceType",
		})
	} else if resourceType != resource.ResourceType {
		issues = append(issues, FHIRValidationIssue{
			Severity:    "error",
			Code:        "structure",
			Details:     fmt.Sprintf("Resource type mismatch: expected %s, got %v", resource.ResourceType, resourceType),
			Location:    "resourceType",
			Expression:  "resourceType",
		})
	}

	// Check id format
	if id, exists := content["id"]; exists {
		if idStr, ok := id.(string); ok {
			if !isValidFHIRID(idStr) {
				issues = append(issues, FHIRValidationIssue{
					Severity:    "error",
					Code:        "value",
					Details:     fmt.Sprintf("Invalid FHIR ID format: %s", idStr),
					Location:    "id",
					Expression:  "id",
				})
			}
		}
	}

	// Check for narrative if present
	if text, exists := content["text"]; exists {
		if textMap, ok := text.(map[string]interface{}); ok {
			if status, exists := textMap["status"]; !exists {
				issues = append(issues, FHIRValidationIssue{
					Severity:    "warning",
					Code:        "structure",
					Details:     "Narrative text must have status",
					Location:    "text.status",
					Expression:  "text.status",
				})
			} else if !isValidNarrativeStatus(fmt.Sprintf("%v", status)) {
				issues = append(issues, FHIRValidationIssue{
					Severity:    "error",
					Code:        "value",
					Details:     fmt.Sprintf("Invalid narrative status: %v", status),
					Location:    "text.status",
					Expression:  "text.status",
				})
			}
		}
	}

	return issues
}

// validateResourceProfile validates resource against specific profile
func (s *FHIRTransformationService) validateResourceProfile(resource *FHIRResource, targetProfile string) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	profileKey := fmt.Sprintf("%s_%s_R4", resource.ResourceType, targetProfile)
	profile, exists := s.profileCache[profileKey]
	if !exists {
		issues = append(issues, FHIRValidationIssue{
			Severity:    "warning",
			Code:        "not-found",
			Details:     fmt.Sprintf("Profile %s not found for validation", targetProfile),
			Location:    "meta.profile",
			Expression:  "meta.profile",
		})
		return issues
	}

	// Check must-support elements
	for _, mustSupportPath := range profile.MustSupport {
		if !s.hasElementAtPath(resource.Content, mustSupportPath) {
			issues = append(issues, FHIRValidationIssue{
				Severity:    "warning",
				Code:        "business-rule",
				Details:     fmt.Sprintf("Must-support element missing: %s", mustSupportPath),
				Location:    mustSupportPath,
				Expression:  mustSupportPath,
			})
		}
	}

	// Check constraints
	for _, constraint := range profile.Constraints {
		if !s.evaluateConstraint(resource.Content, constraint) {
			severity := constraint.Severity
			if severity == "" {
				severity = "error"
			}

			issues = append(issues, FHIRValidationIssue{
				Severity:    severity,
				Code:        "business-rule",
				Details:     constraint.Human,
				Diagnostics: constraint.Expression,
				Location:    constraint.Key,
				Expression:  constraint.Expression,
			})
		}
	}

	return issues
}

// validateResourceDataTypes validates data types of resource elements
func (s *FHIRTransformationService) validateResourceDataTypes(resource *FHIRResource) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	content := resource.Content

	// Validate common data types
	if birthDate, exists := content["birthDate"]; exists && resource.ResourceType == "Patient" {
		if !isValidDate(fmt.Sprintf("%v", birthDate)) {
			issues = append(issues, FHIRValidationIssue{
				Severity:    "error",
				Code:        "value",
				Details:     fmt.Sprintf("Invalid date format: %v", birthDate),
				Location:    "birthDate",
				Expression:  "birthDate",
			})
		}
	}

	// Validate coding elements
	if s.hasElementAtPath(content, "code.coding") {
		if codings := s.getElementAtPath(content, "code.coding"); codings != nil {
			if codingArray, ok := codings.([]interface{}); ok {
				for i, coding := range codingArray {
					if codingMap, ok := coding.(map[string]interface{}); ok {
						if !s.isValidCoding(codingMap) {
							issues = append(issues, FHIRValidationIssue{
								Severity:    "warning",
								Code:        "value",
								Details:     "Invalid coding structure",
								Location:    fmt.Sprintf("code.coding[%d]", i),
								Expression:  fmt.Sprintf("code.coding[%d]", i),
							})
						}
					}
				}
			}
		}
	}

	// Validate references
	s.validateDataTypeReferences(content, "", &issues)

	return issues
}

// validateRequiredFields validates required fields for resource type
func (s *FHIRTransformationService) validateRequiredFields(resource *FHIRResource) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	content := resource.Content
	resourceType := resource.ResourceType

	// Define required fields per resource type
	requiredFields := map[string][]string{
		"Patient":       {},
		"Organization":  {},
		"MessageHeader": {"eventCoding", "source"},
		"Observation":   {"status", "code"},
		"Encounter":     {"status", "class"},
		"Practitioner":  {},
	}

	if required, exists := requiredFields[resourceType]; exists {
		for _, fieldPath := range required {
			if !s.hasElementAtPath(content, fieldPath) {
				issues = append(issues, FHIRValidationIssue{
					Severity:    "error",
					Code:        "required",
					Details:     fmt.Sprintf("Required field missing: %s", fieldPath),
					Location:    fieldPath,
					Expression:  fieldPath,
				})
			}
		}
	}

	return issues
}

// validateResourceTerminology validates terminology bindings
func (s *FHIRTransformationService) validateResourceTerminology(resource *FHIRResource) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	content := resource.Content

	// Validate gender for Patient
	if resource.ResourceType == "Patient" {
		if gender, exists := content["gender"]; exists {
			if !isValidGender(fmt.Sprintf("%v", gender)) {
				issues = append(issues, FHIRValidationIssue{
					Severity:    "error",
					Code:        "code-invalid",
					Details:     fmt.Sprintf("Invalid gender value: %v", gender),
					Location:    "gender",
					Expression:  "gender",
				})
			}
		}
	}

	// Validate status fields
	if status, exists := content["status"]; exists {
		if !s.isValidStatusForResourceType(fmt.Sprintf("%v", status), resource.ResourceType) {
			issues = append(issues, FHIRValidationIssue{
				Severity:    "error",
				Code:        "code-invalid",
				Details:     fmt.Sprintf("Invalid status value for %s: %v", resource.ResourceType, status),
				Location:    "status",
				Expression:  "status",
			})
		}
	}

	return issues
}

// validateResourceReferences validates reference elements
func (s *FHIRTransformationService) validateResourceReferences(resource *FHIRResource) []FHIRValidationIssue {
	var issues []FHIRValidationIssue

	content := resource.Content

	// Validate references recursively
	s.validateReferencesInElement(content, "", &issues)

	return issues
}

// =====================================
// FHIR BUNDLE CREATION
// =====================================

// createFHIRBundle creates a FHIR Bundle from resources
func (s *FHIRTransformationService) createFHIRBundle(resources []FHIRResource, bundleType, messageID string) (*FHIRBundle, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources to bundle")
	}

	bundle := &FHIRBundle{
		ID:        fmt.Sprintf("bundle-%s", uuid.New().String()[:8]),
		Type:      bundleType,
		Timestamp: time.Now(),
		Total:     len(resources),
		Entry:     []FHIRBundleEntry{},
		Metadata:  make(map[string]interface{}),
	}

	// Get bundle template
	template, exists := s.bundleManager.bundleTemplates[bundleType]
	if !exists {
		template = s.bundleManager.bundleTemplates["message"] // Default to message
	}

	// Add resources to bundle
	for i, resource := range resources {
		entry := FHIRBundleEntry{
			ID:      fmt.Sprintf("entry-%d", i),
			FullUrl: s.generateFullUrl(resource),
			Resource: &resource,
		}

		// Add request/response for transaction bundles
		if bundleType == "transaction" {
			entry.Request = &FHIRBundleRequest{
				Method: "POST",
				Url:    resource.ResourceType,
			}
		}

		bundle.Entry = append(bundle.Entry, entry)
	}

	// Add bundle metadata
	bundle.Metadata["createdBy"] = "FHIRTransformationService"
	bundle.Metadata["createdAt"] = time.Now()
	bundle.Metadata["sourceMessageId"] = messageID
	bundle.Metadata["template"] = template.Type
	bundle.Metadata["profile"] = template.Profile

	// Validate bundle structure
	if err := s.validateBundleStructure(bundle, template); err != nil {
		log.Printf("⚠️ Warning: Bundle validation failed: %v", err)
	}

	log.Printf("✅ Created FHIR %s bundle with %d resources", bundleType, len(resources))
	return bundle, nil
}

// validateBundleStructure validates bundle against template
func (s *FHIRTransformationService) validateBundleStructure(bundle *FHIRBundle, template FHIRBundleTemplate) error {
	// Check required resource types
	presentTypes := make(map[string]bool)
	for _, entry := range bundle.Entry {
		if entry.Resource != nil {
			presentTypes[entry.Resource.ResourceType] = true
		}
	}

	// Validate required resources
	for _, required := range template.Required {
		if !presentTypes[required] {
			return fmt.Errorf("required resource type missing: %s", required)
		}
	}

	// Check bundle type consistency
	if bundle.Type != template.Type {
		return fmt.Errorf("bundle type mismatch: expected %s, got %s", template.Type, bundle.Type)
	}

	return nil
}

// generateFullUrl generates full URL for resource
func (s *FHIRTransformationService) generateFullUrl(resource FHIRResource) string {
	baseUrl := s.bundleManager.linkGenerator.baseURL
	return fmt.Sprintf("%s/%s/%s", baseUrl, resource.ResourceType, resource.ID)
}

// =====================================
// NARRATIVE GENERATION
// =====================================

// generateNarratives generates human-readable narratives for resources
func (s *FHIRTransformationService) generateNarratives(resources []FHIRResource) (int, error) {
	narrativeCount := 0

	for i := range resources {
		resource := &resources[i]

		narrative := s.generateResourceNarrative(resource)
		if narrative != nil {
			if resource.Content == nil {
				resource.Content = make(map[string]interface{})
			}
			resource.Content["text"] = narrative
			narrativeCount++
		}
	}

	log.Printf("✅ Generated %d narratives", narrativeCount)
	return narrativeCount, nil
}

// generateResourceNarrative generates narrative for a specific resource
func (s *FHIRTransformationService) generateResourceNarrative(resource *FHIRResource) map[string]interface{} {
	var narrativeText string

	switch resource.ResourceType {
	case "Patient":
		narrativeText = s.generatePatientNarrative(resource.Content)
	case "Organization":
		narrativeText = s.generateOrganizationNarrative(resource.Content)
	case "MessageHeader":
		narrativeText = s.generateMessageHeaderNarrative(resource.Content)
	case "Encounter":
		narrativeText = s.generateEncounterNarrative(resource.Content)
	case "Observation":
		narrativeText = s.generateObservationNarrative(resource.Content)
	default:
		narrativeText = s.generateGenericNarrative(resource)
	}

	if narrativeText == "" {
		return nil
	}

	return map[string]interface{}{
		"status": "generated",
		"div":    fmt.Sprintf("<div xmlns=\"http://www.w3.org/1999/xhtml\">%s</div>", narrativeText),
	}
}

// Narrative generation methods
func (s *FHIRTransformationService) generatePatientNarrative(content map[string]interface{}) string {
	var parts []string

	if name := s.extractHumanName(content, "name"); name != "" {
		parts = append(parts, fmt.Sprintf("Patient: %s", name))
	}

	if gender, exists := content["gender"]; exists {
		parts = append(parts, fmt.Sprintf("Gender: %v", gender))
	}

	if birthDate, exists := content["birthDate"]; exists {
		parts = append(parts, fmt.Sprintf("Date of Birth: %v", birthDate))
	}

	if identifier := s.extractIdentifier(content, "identifier"); identifier != "" {
		parts = append(parts, fmt.Sprintf("ID: %s", identifier))
	}

	return strings.Join(parts, ", ")
}

func (s *FHIRTransformationService) generateOrganizationNarrative(content map[string]interface{}) string {
	var parts []string

	if name, exists := content["name"]; exists {
		parts = append(parts, fmt.Sprintf("Organization: %v", name))
	}

	if identifier := s.extractIdentifier(content, "identifier"); identifier != "" {
		parts = append(parts, fmt.Sprintf("ID: %s", identifier))
	}

	return strings.Join(parts, ", ")
}

func (s *FHIRTransformationService) generateMessageHeaderNarrative(content map[string]interface{}) string {
	var parts []string

	if eventCoding := s.extractCoding(content, "eventCoding"); eventCoding != "" {
		parts = append(parts, fmt.Sprintf("Message: %s", eventCoding))
	}

	if source := s.extractSource(content); source != "" {
		parts = append(parts, fmt.Sprintf("From: %s", source))
	}

	return strings.Join(parts, ", ")
}

func (s *FHIRTransformationService) generateEncounterNarrative(content map[string]interface{}) string {
	var parts []string

	if class := s.extractCoding(content, "class"); class != "" {
		parts = append(parts, fmt.Sprintf("Encounter Class: %s", class))
	}

	if status, exists := content["status"]; exists {
		parts = append(parts, fmt.Sprintf("Status: %v", status))
	}

	return strings.Join(parts, ", ")
}

func (s *FHIRTransformationService) generateObservationNarrative(content map[string]interface{}) string {
	var parts []string

	if code := s.extractCodeableConcept(content, "code"); code != "" {
		parts = append(parts, fmt.Sprintf("Observation: %s", code))
	}

	if value := s.extractObservationValue(content); value != "" {
		parts = append(parts, fmt.Sprintf("Value: %s", value))
	}

	if status, exists := content["status"]; exists {
		parts = append(parts, fmt.Sprintf("Status: %v", status))
	}

	return strings.Join(parts, ", ")
}

func (s *FHIRTransformationService) generateGenericNarrative(resource *FHIRResource) string {
	return fmt.Sprintf("%s resource (ID: %s)", resource.ResourceType, resource.ID)
}

// =====================================
// METRICS AND PERFORMANCE
// =====================================

// calculateResourceMetrics calculates metrics for transformed resources
func (s *FHIRTransformationService) calculateResourceMetrics(resources []FHIRResource) FHIRResourceMetrics {
	metrics := FHIRResourceMetrics{
		ResourceCounts:       make(map[string]int),
		TotalResources:       len(resources),
		ValidatedResources:   0,
		ProfileCompliantResources: 0,
		ReferencesCreated:    0,
		ExtensionsAdded:      0,
		NarrativesGenerated:  0,
	}

	for _, resource := range resources {
		// Count by type
		metrics.ResourceCounts[resource.ResourceType]++

		// Count validated resources
		if resource.ValidationStatus != "NOT_VALIDATED" {
			metrics.ValidatedResources++
		}

		// Count profile compliant resources
		if resource.ValidationStatus == "information" || resource.ValidationStatus == "warning" {
			metrics.ProfileCompliantResources++
		}

		// Count references
		metrics.ReferencesCreated += len(resource.References)

		// Count extensions
		if extensions := s.getElementAtPath(resource.Content, "extension"); extensions != nil {
			if extArray, ok := extensions.([]interface{}); ok {
				metrics.ExtensionsAdded += len(extArray)
			}
		}

		// Count narratives
		if s.hasElementAtPath(resource.Content, "text.div") {
			metrics.NarrativesGenerated++
		}
	}

	return metrics
}

// updatePerformanceMetrics updates service performance metrics
func (s *FHIRTransformationService) updatePerformanceMetrics(processingMetrics ProcessingMetrics, resourceCount int) {
	s.performanceMetrics.ResourcesProcessed += int64(resourceCount)

	if resourceCount > 0 {
		s.performanceMetrics.BundlesCreated++
	}

	// Update running averages
	totalProcessed := float64(s.performanceMetrics.ResourcesProcessed)
	if totalProcessed > 0 {
		s.performanceMetrics.AverageTransformTime = time.Duration(
			(float64(s.performanceMetrics.AverageTransformTime)*(totalProcessed-float64(resourceCount)) +
				float64(processingMetrics.TransformTime)*float64(resourceCount)) / totalProcessed)
	}
}

// =====================================
// UTILITY AND HELPER METHODS
// =====================================

// Validation helper methods
func isValidFHIRID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	// FHIR ID pattern: [A-Za-z0-9\-\.]{1,64}
	for _, char := range id {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '.') {
			return false
		}
	}
	return true
}

func isValidNarrativeStatus(status string) bool {
	validStatuses := []string{"generated", "extensions", "additional", "empty"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

func isValidDate(date string) bool {
	formats := []string{"2006-01-02", "2006-01", "2006"}
	for _, format := range formats {
		if len(date) == len(format) {
			if _, err := time.Parse(format, date); err == nil {
				return true
			}
		}
	}
	return false
}

func isValidGender(gender string) bool {
	validGenders := []string{"male", "female", "other", "unknown"}
	for _, valid := range validGenders {
		if gender == valid {
			return true
		}
	}
	return false
}

func (s *FHIRTransformationService) isValidCoding(coding map[string]interface{}) bool {
	// Must have either code or display
	_, hasCode := coding["code"]
	_, hasDisplay := coding["display"]
	return hasCode || hasDisplay
}

func (s *FHIRTransformationService) isValidStatusForResourceType(status, resourceType string) bool {
	validStatuses := map[string][]string{
		"Observation": {"registered", "preliminary", "final", "amended", "corrected", "cancelled", "entered-in-error", "unknown"},
		"Encounter":   {"planned", "arrived", "triaged", "in-progress", "onleave", "finished", "cancelled", "entered-in-error", "unknown"},
	}

	if statuses, exists := validStatuses[resourceType]; exists {
		for _, valid := range statuses {
			if status == valid {
				return true
			}
		}
		return false
	}

	return true // Unknown resource types pass by default
}

// Element access helper methods
func (s *FHIRTransformationService) hasElementAtPath(data map[string]interface{}, path string) bool {
	_, exists := s.getElementAtPath(data, path)
	return exists != nil
}

func (s *FHIRTransformationService) getElementAtPath(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			// Handle array access
			arrayName := part[:strings.Index(part, "[")]
			indexStr := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

			if array, exists := current[arrayName]; exists {
				if arraySlice, ok := array.([]interface{}); ok {
					if index, err := strconv.Atoi(indexStr); err == nil && index < len(arraySlice) {
						if i == len(parts)-1 {
							return arraySlice[index]
						}
						if nextMap, ok := arraySlice[index].(map[string]interface{}); ok {
							current = nextMap
						} else {
							return nil
						}
					} else {
						return nil
					}
				} else {
					return nil
				}
			} else {
				return nil
			}
		} else {
			if i == len(parts)-1 {
				return current[part]
			}

			if next, exists := current[part]; exists {
				if nextMap, ok := next.(map[string]interface{}); ok {
					current = nextMap
				} else {
					return nil
				}
			} else {
				return nil
			}
		}
	}

	return nil
}

func (s *FHIRTransformationService) evaluateConstraint(data map[string]interface{}, constraint FHIRConstraint) bool {
	// Simple constraint evaluation - in production would use FHIRPath
	// For now, just check if the constraint path exists
	return s.hasElementAtPath(data, constraint.Key)
}

func (s *FHIRTransformationService) checkProfileCompliance(resource *FHIRResource, targetProfile string) float64 {
	// Simple compliance check - count must-support elements present
	profileKey := fmt.Sprintf("%s_%s_R4", resource.ResourceType, targetProfile)
	profile, exists := s.profileCache[profileKey]
	if !exists {
		return 100.0 // No profile to check against
	}

	if len(profile.MustSupport) == 0 {
		return 100.0 // No must-support requirements
	}

	presentCount := 0
	for _, mustSupportPath := range profile.MustSupport {
		if s.hasElementAtPath(resource.Content, mustSupportPath) {
			presentCount++
		}
	}

	return float64(presentCount) / float64(len(profile.MustSupport)) * 100.0
}

func (s *FHIRTransformationService) validateDataTypeReferences(data interface{}, path string, issues *[]FHIRValidationIssue) {
	switch v := data.(type) {
	case map[string]interface{}:
		// Check if this is a Reference type
		if reference, isRef := v["reference"]; isRef {
			if refStr, ok := reference.(string); ok {
				if !s.isValidReference(refStr) {
					*issues = append(*issues, FHIRValidationIssue{
						Severity:    "warning",
						Code:        "value",
						Details:     fmt.Sprintf("Invalid reference format: %s", refStr),
						Location:    fmt.Sprintf("%s.reference", path),
						Expression:  fmt.Sprintf("%s.reference", path),
					})
				}
			}
		}

		// Recursively check nested objects
		for key, value := range v {
			newPath := key
			if path != "" {
				newPath = fmt.Sprintf("%s.%s", path, key)
			}
			s.validateDataTypeReferences(value, newPath, issues)
		}

	case []interface{}:
		// Check arrays
		for i, item := range v {
			newPath := fmt.Sprintf("%s[%d]", path, i)
			s.validateDataTypeReferences(item, newPath, issues)
		}
	}
}

func (s *FHIRTransformationService) validateReferencesInElement(data interface{}, path string, issues *[]FHIRValidationIssue) {
	s.validateDataTypeReferences(data, path, issues)
}

func (s *FHIRTransformationService) isValidReference(reference string) bool {
	// Basic reference validation
	if reference == "" {
		return false
	}

	// Check for valid patterns: ResourceType/id or absolute URL
	if strings.Contains(reference, "/") {
		parts := strings.Split(reference, "/")
		if len(parts) >= 2 {
			return true // Basic check passed
		}
	}

	return strings.HasPrefix(reference, "http://") || strings.HasPrefix(reference, "https://")
}

// Narrative extraction helper methods
func (s *FHIRTransformationService) extractHumanName(content map[string]interface{}, path string) string {
	if nameArray := s.getElementAtPath(content, path); nameArray != nil {
		if names, ok := nameArray.([]interface{}); ok && len(names) > 0 {
			if name, ok := names[0].(map[string]interface{}); ok {
				var nameParts []string

				if given, exists := name["given"]; exists {
					if givenArray, ok := given.([]interface{}); ok {
						for _, g := range givenArray {
							nameParts = append(nameParts, fmt.Sprintf("%v", g))
						}
					}
				}

				if family, exists := name["family"]; exists {
					nameParts = append(nameParts, fmt.Sprintf("%v", family))
				}

				return strings.Join(nameParts, " ")
			}
		}
	}
	return ""
}

func (s *FHIRTransformationService) extractIdentifier(content map[string]interface{}, path string) string {
	if identifierArray := s.getElementAtPath(content, path); identifierArray != nil {
		if identifiers, ok := identifierArray.([]interface{}); ok && len(identifiers) > 0 {
			if identifier, ok := identifiers[0].(map[string]interface{}); ok {
				if value, exists := identifier["value"]; exists {
					return fmt.Sprintf("%v", value)
				}
			}
		}
	}
	return ""
}

func (s *FHIRTransformationService) extractCoding(content map[string]interface{}, path string) string {
	if coding := s.getElementAtPath(content, path); coding != nil {
		if codingMap, ok := coding.(map[string]interface{}); ok {
			if display, exists := codingMap["display"]; exists {
				return fmt.Sprintf("%v", display)
			}
			if code, exists := codingMap["code"]; exists {
				return fmt.Sprintf("%v", code)
			}
		}
	}
	return ""
}

func (s *FHIRTransformationService) extractCodeableConcept(content map[string]interface{}, path string) string {
	if concept := s.getElementAtPath(content, path); concept != nil {
		if conceptMap, ok := concept.(map[string]interface{}); ok {
			if text, exists := conceptMap["text"]; exists {
				return fmt.Sprintf("%v", text)
			}
			if coding := s.getElementAtPath(conceptMap, "coding[0]"); coding != nil {
				return s.extractCoding(map[string]interface{}{"coding": coding}, "coding")
			}
		}
	}
	return ""
}

func (s *FHIRTransformationService) extractSource(content map[string]interface{}) string {
	if source := s.getElementAtPath(content, "source"); source != nil {
		if sourceMap, ok := source.(map[string]interface{}); ok {
			if name, exists := sourceMap["name"]; exists {
				return fmt.Sprintf("%v", name)
			}
		}
	}
	return ""
}

func (s *FHIRTransformationService) extractObservationValue(content map[string]interface{}) string {
	// Check various value types
	valueTypes := []string{"valueString", "valueQuantity", "valueCodeableConcept", "valueBoolean", "valueInteger", "valueDateTime"}

	for _, valueType := range valueTypes {
		if value := s.getElementAtPath(content, valueType); value != nil {
			if valueType == "valueQuantity" {
				if valueMap, ok := value.(map[string]interface{}); ok {
					var parts []string
					if val, exists := valueMap["value"]; exists {
						parts = append(parts, fmt.Sprintf("%v", val))
					}
					if unit, exists := valueMap["unit"]; exists {
						parts = append(parts, fmt.Sprintf("%v", unit))
					}
					return strings.Join(parts, " ")
				}
			} else if valueType == "valueCodeableConcept" {
				return s.extractCodeableConcept(map[string]interface{}{valueType: value}, valueType)
			} else {
				return fmt.Sprintf("%v", value)
			}
		}
	}

	return ""
}

// startTransformationStep creates a transformation step record
func (s *FHIRTransformationService) startTransformationStep(stepName, inputType, outputType string) TransformationStep {
	return TransformationStep{
		StepID:     fmt.Sprintf("step_%d", time.Now().UnixNano()),
		StepName:   stepName,
		InputType:  inputType,
		OutputType: outputType,
		StartTime:  time.Now(),
		Success:    false,
		Metadata:   make(map[string]interface{}),
	}
}

// completeTransformationStep completes a transformation step record
func (s *FHIRTransformationService) completeTransformationStep(step *TransformationStep, err error, inputSize, outputSize int) {
	step.EndTime = time.Now()
	step.Duration = step.EndTime.Sub(step.StartTime)
	step.Success = err == nil
	step.InputSize = int64(inputSize)
	step.OutputSize = int64(outputSize)

	if err != nil {
		step.ErrorMessage = err.Error()
	}
}

// GetPerformanceMetrics returns current performance metrics
func (s *FHIRTransformationService) GetPerformanceMetrics() FHIRPerformanceMetrics {
	return s.performanceMetrics
}

// ResetPerformanceMetrics resets performance tracking
func (s *FHIRTransformationService) ResetPerformanceMetrics() {
	s.performanceMetrics = FHIRPerformanceMetrics{}
}

// GetSupportedProfiles returns list of supported FHIR profiles
func (s *FHIRTransformationService) GetSupportedProfiles() []string {
	profiles := make([]string, 0, len(s.profileCache))
	for profileID := range s.profileCache {
		profiles = append(profiles, profileID)
	}
	return profiles
}

// ClearCache clears the profile and resource caches
func (s *FHIRTransformationService) ClearCache() {
	s.profileCache = make(map[string]*FHIRProfile)
	s.resourceCache = make(map[string]*FHIRResource)
}