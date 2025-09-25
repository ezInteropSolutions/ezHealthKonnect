// services/transformation_fhir.go
// FHIR Transformation Service for Universal Interface Engine
//
// 🎯 PURPOSE: Comprehensive FHIR resource mapping, validation, and transformation
// Supports FHIR R4/R5, resource creation, bundle management, and profile validation
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"ezhealthkonnect/fhir"
)

// =====================================
// FHIR TRANSFORMATION SERVICE
// =====================================

// FHIRTransformationService handles FHIR resource processing
type FHIRTransformationService struct {
	db             *sql.DB
	fhirLoader     *fhir.FHIRSchemaLoader
	profileCache   map[string]*FHIRProfile
	resourceCache  map[string]*FHIRResource
	bundleManager  *FHIRBundleManager
	validator      *FHIRValidator
	performanceMetrics FHIRPerformanceMetrics
}

// FHIRPerformanceMetrics tracks service performance
type FHIRPerformanceMetrics struct {
	ResourcesProcessed   int64         `json:"resourcesProcessed"`
	BundlesCreated       int64         `json:"bundlesCreated"`
	AverageTransformTime time.Duration `json:"averageTransformTime"`
	AverageValidateTime  time.Duration `json:"averageValidateTime"`
	ProfileCacheHitRatio float64       `json:"profileCacheHitRatio"`
	ErrorRate            float64       `json:"errorRate"`
	ThroughputPerSecond  float64       `json:"throughputPerSecond"`
}

// FHIRTransformRequest defines FHIR transformation request
type FHIRTransformRequest struct {
	MessageID         string                 `json:"messageId"`
	SourceData        map[string]interface{} `json:"sourceData"`
	SourceFormat      MessageType            `json:"sourceFormat"`
	TargetProfile     string                 `json:"targetProfile,omitempty"`     // us-core, base, custom
	FHIRVersion       string                 `json:"fhirVersion,omitempty"`       // R4, R5
	CreateBundle      bool                   `json:"createBundle"`
	BundleType        string                 `json:"bundleType,omitempty"`        // document, message, transaction
	ValidationLevel   string                 `json:"validationLevel,omitempty"`   // STRICT, MODERATE, LENIENT
	IncludeReferences bool                   `json:"includeReferences"`
	GenerateNarrative bool                   `json:"generateNarrative"`
	CustomMappings    map[string]interface{} `json:"customMappings,omitempty"`
	TransformationOptions map[string]interface{} `json:"transformationOptions,omitempty"`
}

// FHIRTransformResponse defines FHIR transformation response
type FHIRTransformResponse struct {
	Success              bool                   `json:"success"`
	MessageID            string                 `json:"messageId"`
	FHIRVersion          string                 `json:"fhirVersion"`
	Profile              string                 `json:"profile"`
	Resources            []FHIRResource         `json:"resources"`
	Bundle               *FHIRBundle            `json:"bundle,omitempty"`
	ValidationResults    []FHIRValidationResult `json:"validationResults"`
	TransformationSteps  []TransformationStep   `json:"transformationSteps"`
	ResourceMetrics      FHIRResourceMetrics    `json:"resourceMetrics"`
	ProcessingMetrics    ProcessingMetrics      `json:"processingMetrics"`
	Warnings             []string               `json:"warnings"`
	Errors               []string               `json:"errors"`
	QualityScore         float64                `json:"qualityScore"`
	ComplianceFlags      map[string]bool        `json:"complianceFlags"`
	RecommendedActions   []string               `json:"recommendedActions"`
}

// FHIRResource represents a FHIR resource with metadata
type FHIRResource struct {
	ID               string                 `json:"id"`
	ResourceType     string                 `json:"resourceType"`
	Profile          string                 `json:"profile,omitempty"`
	Version          string                 `json:"version,omitempty"`
	Content          map[string]interface{} `json:"content"`
	References       []FHIRReference        `json:"references,omitempty"`
	ValidationStatus string                 `json:"validationStatus"`
	ValidationIssues []FHIRValidationIssue  `json:"validationIssues,omitempty"`
	Metadata         FHIRResourceMetadata   `json:"metadata"`
	Extensions       []FHIRExtension        `json:"extensions,omitempty"`
	CreatedAt        time.Time              `json:"createdAt"`
	ModifiedAt       time.Time              `json:"modifiedAt"`
}

// FHIRBundle represents a FHIR Bundle resource
type FHIRBundle struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Timestamp   time.Time              `json:"timestamp"`
	Total       int                    `json:"total"`
	Entry       []FHIRBundleEntry      `json:"entry"`
	Link        []FHIRBundleLink       `json:"link,omitempty"`
	Signature   *FHIRSignature         `json:"signature,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// FHIRBundleEntry represents an entry in a FHIR Bundle
type FHIRBundleEntry struct {
	ID            string                 `json:"id,omitempty"`
	Link          []FHIRBundleLink       `json:"link,omitempty"`
	FullUrl       string                 `json:"fullUrl,omitempty"`
	Resource      *FHIRResource          `json:"resource,omitempty"`
	Search        *FHIRBundleSearch      `json:"search,omitempty"`
	Request       *FHIRBundleRequest     `json:"request,omitempty"`
	Response      *FHIRBundleResponse    `json:"response,omitempty"`
}

// Supporting FHIR structures
type FHIRBundleLink struct {
	Relation string `json:"relation"`
	Url      string `json:"url"`
}

type FHIRBundleSearch struct {
	Mode  string  `json:"mode,omitempty"`
	Score float64 `json:"score,omitempty"`
}

type FHIRBundleRequest struct {
	Method string `json:"method"`
	Url    string `json:"url"`
}

type FHIRBundleResponse struct {
	Status   string `json:"status"`
	Location string `json:"location,omitempty"`
	Etag     string `json:"etag,omitempty"`
}

type FHIRSignature struct {
	Type      []FHIRCoding           `json:"type"`
	When      time.Time              `json:"when"`
	Who       FHIRReference          `json:"who"`
	OnBehalfOf *FHIRReference        `json:"onBehalfOf,omitempty"`
	Data      string                 `json:"data,omitempty"`
}

// FHIRReference represents a reference to another resource
type FHIRReference struct {
	Reference    string      `json:"reference,omitempty"`
	Type         string      `json:"type,omitempty"`
	Identifier   *FHIRIdentifier `json:"identifier,omitempty"`
	Display      string      `json:"display,omitempty"`
}

// FHIRIdentifier represents a FHIR Identifier
type FHIRIdentifier struct {
	Use      string              `json:"use,omitempty"`
	Type     *FHIRCodeableConcept `json:"type,omitempty"`
	System   string              `json:"system,omitempty"`
	Value    string              `json:"value,omitempty"`
	Period   *FHIRPeriod         `json:"period,omitempty"`
	Assigner *FHIRReference      `json:"assigner,omitempty"`
}

// FHIRCodeableConcept represents a FHIR CodeableConcept
type FHIRCodeableConcept struct {
	Coding []FHIRCoding `json:"coding,omitempty"`
	Text   string       `json:"text,omitempty"`
}

// FHIRCoding represents a FHIR Coding
type FHIRCoding struct {
	System       string `json:"system,omitempty"`
	Version      string `json:"version,omitempty"`
	Code         string `json:"code,omitempty"`
	Display      string `json:"display,omitempty"`
	UserSelected bool   `json:"userSelected,omitempty"`
}

// FHIRPeriod represents a FHIR Period
type FHIRPeriod struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// FHIRExtension represents a FHIR Extension
type FHIRExtension struct {
	URL   string      `json:"url"`
	Value interface{} `json:"value,omitempty"`
}

// FHIRResourceMetadata contains resource processing metadata
type FHIRResourceMetadata struct {
	SourceMapping     string                 `json:"sourceMapping"`
	TransformationID  string                 `json:"transformationId"`
	DataCompleteness  float64                `json:"dataCompleteness"`
	MustSupportFields []string               `json:"mustSupportFields"`
	ProfileCompliance float64                `json:"profileCompliance"`
	GenerationMethod  string                 `json:"generationMethod"`
	ProcessingTime    time.Duration          `json:"processingTime"`
	CustomMetadata    map[string]interface{} `json:"customMetadata"`
}

// FHIRResourceMetrics contains transformation metrics
type FHIRResourceMetrics struct {
	ResourceCounts       map[string]int `json:"resourceCounts"`
	TotalResources       int            `json:"totalResources"`
	ValidatedResources   int            `json:"validatedResources"`
	ProfileCompliantResources int       `json:"profileCompliantResources"`
	ReferencesCreated    int            `json:"referencesCreated"`
	ExtensionsAdded      int            `json:"extensionsAdded"`
	NarrativesGenerated  int            `json:"narrativesGenerated"`
}

// FHIRValidationResult contains validation outcome
type FHIRValidationResult struct {
	ResourceID    string                `json:"resourceId"`
	ResourceType  string                `json:"resourceType"`
	IsValid       bool                  `json:"isValid"`
	ProfileValid  bool                  `json:"profileValid"`
	Severity      string                `json:"severity"`
	Issues        []FHIRValidationIssue `json:"issues"`
	Score         float64               `json:"score"`
	Timestamp     time.Time             `json:"timestamp"`
	ValidatorID   string                `json:"validatorId"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// FHIRValidationIssue represents validation problems
type FHIRValidationIssue struct {
	Severity    string `json:"severity"`       // error, warning, information
	Code        string `json:"code"`           // structure, required, value, etc.
	Details     string `json:"details"`
	Diagnostics string `json:"diagnostics,omitempty"`
	Location    string `json:"location,omitempty"`
	Expression  string `json:"expression,omitempty"`
}

// FHIRProfile represents a FHIR profile definition
type FHIRProfile struct {
	ID               string                      `json:"id"`
	URL              string                      `json:"url"`
	Name             string                      `json:"name"`
	Title            string                      `json:"title"`
	Status           string                      `json:"status"`
	Version          string                      `json:"version"`
	FHIRVersion      string                      `json:"fhirVersion"`
	Description      string                      `json:"description"`
	BaseDefinition   string                      `json:"baseDefinition"`
	Type             string                      `json:"type"`
	Elements         map[string]FHIRElementDef   `json:"elements"`
	MustSupport      []string                    `json:"mustSupport"`
	Constraints      []FHIRConstraint            `json:"constraints"`
	Extensions       []FHIRExtensionDefinition   `json:"extensions"`
	ValueSets        map[string]FHIRValueSet     `json:"valueSets"`
	LoadedAt         time.Time                   `json:"loadedAt"`
}

// FHIRElementDef represents a FHIR element definition
type FHIRElementDef struct {
	Path         string              `json:"path"`
	Min          int                 `json:"min"`
	Max          string              `json:"max"`
	Type         []FHIRElementType   `json:"type"`
	MustSupport  bool                `json:"mustSupport"`
	Binding      *FHIRBinding        `json:"binding,omitempty"`
	Constraints  []FHIRConstraint    `json:"constraints"`
	Mapping      []FHIRMapping       `json:"mapping"`
	Definition   string              `json:"definition"`
	Comment      string              `json:"comment,omitempty"`
}

type FHIRElementType struct {
	Code    string `json:"code"`
	Profile string `json:"profile,omitempty"`
}

type FHIRBinding struct {
	Strength  string `json:"strength"`
	ValueSet  string `json:"valueSet,omitempty"`
}

type FHIRConstraint struct {
	Key         string `json:"key"`
	Severity    string `json:"severity"`
	Human       string `json:"human"`
	Expression  string `json:"expression"`
	Source      string `json:"source,omitempty"`
}

type FHIRMapping struct {
	Identity string `json:"identity"`
	Map      string `json:"map"`
	Comment  string `json:"comment,omitempty"`
}

type FHIRExtensionDefinition struct {
	URL         string            `json:"url"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Type        []FHIRElementType `json:"type"`
	Context     []FHIRContext     `json:"context"`
}

type FHIRContext struct {
	Type       string `json:"type"`
	Expression string `json:"expression"`
}

type FHIRValueSet struct {
	URL      string           `json:"url"`
	Name     string           `json:"name"`
	Title    string           `json:"title"`
	Status   string           `json:"status"`
	Compose  FHIRValueSetCompose `json:"compose"`
}

type FHIRValueSetCompose struct {
	Include []FHIRValueSetInclude `json:"include"`
	Exclude []FHIRValueSetInclude `json:"exclude,omitempty"`
}

type FHIRValueSetInclude struct {
	System   string                `json:"system,omitempty"`
	Version  string                `json:"version,omitempty"`
	Concept  []FHIRValueSetConcept `json:"concept,omitempty"`
	Filter   []FHIRValueSetFilter  `json:"filter,omitempty"`
}

type FHIRValueSetConcept struct {
	Code        string             `json:"code"`
	Display     string             `json:"display,omitempty"`
	Designation []FHIRDesignation  `json:"designation,omitempty"`
}

type FHIRDesignation struct {
	Language string      `json:"language,omitempty"`
	Use      FHIRCoding  `json:"use,omitempty"`
	Value    string      `json:"value"`
}

type FHIRValueSetFilter struct {
	Property string `json:"property"`
	Op       string `json:"op"`
	Value    string `json:"value"`
}

// =====================================
// BUNDLE MANAGER
// =====================================

// FHIRBundleManager handles FHIR Bundle creation and management
type FHIRBundleManager struct {
	bundleTemplates map[string]FHIRBundleTemplate
	linkGenerator   *FHIRLinkGenerator
}

type FHIRBundleTemplate struct {
	Type        string            `json:"type"`
	Profile     string            `json:"profile,omitempty"`
	Structure   []string          `json:"structure"`
	Required    []string          `json:"required"`
	Optional    []string          `json:"optional"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type FHIRLinkGenerator struct {
	baseURL    string
	linkRules  map[string]LinkRule
}

type LinkRule struct {
	ResourceType string `json:"resourceType"`
	Pattern      string `json:"pattern"`
	Required     bool   `json:"required"`
}

// =====================================
// FHIR VALIDATOR
// =====================================

// FHIRValidator handles FHIR resource validation
type FHIRValidator struct {
	profileValidators map[string]*ProfileValidator
	schemaValidator   *SchemaValidator
	terminologyService *TerminologyService
}

type ProfileValidator struct {
	Profile          *FHIRProfile
	MustSupportRules []MustSupportRule
	ConstraintRules  []ConstraintRule
}

type MustSupportRule struct {
	Path     string `json:"path"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

type ConstraintRule struct {
	Key        string `json:"key"`
	Expression string `json:"expression"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
}

type SchemaValidator struct {
	schemas     map[string]interface{}
	validators  map[string]interface{}
}

type TerminologyService struct {
	codeSystems map[string]CodeSystem
	valueSets   map[string]ValueSet
}

type CodeSystem struct {
	URL      string            `json:"url"`
	Concepts map[string]Concept `json:"concepts"`
}

type Concept struct {
	Code        string `json:"code"`
	Display     string `json:"display"`
	Definition  string `json:"definition,omitempty"`
}

type ValueSet struct {
	URL     string   `json:"url"`
	Codes   []string `json:"codes"`
	Systems []string `json:"systems"`
}

// =====================================
// SERVICE CONSTRUCTOR AND INITIALIZATION
// =====================================

// NewFHIRTransformationService creates a new FHIR transformation service
func NewFHIRTransformationService(database *sql.DB) *FHIRTransformationService {
	service := &FHIRTransformationService{
		db:             database,
		fhirLoader:     fhir.GetFHIRSchemaLoader(),
		profileCache:   make(map[string]*FHIRProfile),
		resourceCache:  make(map[string]*FHIRResource),
		bundleManager:  NewFHIRBundleManager(),
		validator:      NewFHIRValidator(),
		performanceMetrics: FHIRPerformanceMetrics{},
	}

	// Initialize FHIR profiles and schemas
	if err := service.initializeFHIRProfiles(); err != nil {
		log.Printf("⚠️ Warning: Failed to initialize FHIR profiles: %v", err)
	}

	log.Printf("✅ FHIRTransformationService initialized")
	return service
}

// NewFHIRBundleManager creates a new bundle manager
func NewFHIRBundleManager() *FHIRBundleManager {
	manager := &FHIRBundleManager{
		bundleTemplates: make(map[string]FHIRBundleTemplate),
		linkGenerator:   &FHIRLinkGenerator{
			baseURL:   "http://localhost:8080/fhir",
			linkRules: make(map[string]LinkRule),
		},
	}

	manager.initializeBundleTemplates()
	return manager
}

// NewFHIRValidator creates a new FHIR validator
func NewFHIRValidator() *FHIRValidator {
	return &FHIRValidator{
		profileValidators: make(map[string]*ProfileValidator),
		schemaValidator:   &SchemaValidator{
			schemas:    make(map[string]interface{}),
			validators: make(map[string]interface{}),
		},
		terminologyService: &TerminologyService{
			codeSystems: make(map[string]CodeSystem),
			valueSets:   make(map[string]ValueSet),
		},
	}
}

// initializeFHIRProfiles loads FHIR profiles
func (s *FHIRTransformationService) initializeFHIRProfiles() error {
	if s.fhirLoader == nil {
		return fmt.Errorf("FHIR schema loader not available")
	}

	// Load common profiles
	profiles := []string{
		"Patient", "Organization", "Practitioner", "Encounter",
		"Observation", "DiagnosticReport", "MessageHeader",
		"us-core-patient", "us-core-organization", "us-core-practitioner",
	}

	for _, profile := range profiles {
		if err := s.loadFHIRProfile(profile, "base", "R4"); err != nil {
			log.Printf("⚠️ Warning: Failed to load profile %s: %v", profile, err)
		}
	}

	log.Printf("✅ Initialized %d FHIR profiles", len(s.profileCache))
	return nil
}

// loadFHIRProfile loads a specific FHIR profile
func (s *FHIRTransformationService) loadFHIRProfile(profileName, profileType, version string) error {
	cacheKey := fmt.Sprintf("%s_%s_%s", profileName, profileType, version)

	// Check cache first
	if _, exists := s.profileCache[cacheKey]; exists {
		return nil
	}

	// TODO: Load actual profile from FHIR schema loader
	// For now, create a basic profile structure
	profile := &FHIRProfile{
		ID:             cacheKey,
		URL:            fmt.Sprintf("http://hl7.org/fhir/StructureDefinition/%s", profileName),
		Name:           profileName,
		Title:          fmt.Sprintf("%s Profile", profileName),
		Status:         "active",
		Version:        version,
		FHIRVersion:    version,
		Description:    fmt.Sprintf("FHIR %s resource profile", profileName),
		BaseDefinition: fmt.Sprintf("http://hl7.org/fhir/StructureDefinition/%s", profileName),
		Type:           profileName,
		Elements:       make(map[string]FHIRElementDef),
		MustSupport:    []string{},
		Constraints:    []FHIRConstraint{},
		Extensions:     []FHIRExtensionDefinition{},
		ValueSets:      make(map[string]FHIRValueSet),
		LoadedAt:       time.Now(),
	}

	// Add basic elements based on resource type
	s.addBasicElements(profile, profileName)

	s.profileCache[cacheKey] = profile
	return nil
}

// addBasicElements adds basic elements to a profile
func (s *FHIRTransformationService) addBasicElements(profile *FHIRProfile, resourceType string) {
	// Common elements for all resources
	commonElements := map[string]FHIRElementDef{
		"id": {
			Path: "id",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "id"}},
			Definition: "Logical id of this artifact",
		},
		"meta": {
			Path: "meta",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "Meta"}},
			Definition: "Metadata about the resource",
		},
		"text": {
			Path: "text",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "Narrative"}},
			Definition: "Text summary of the resource",
		},
	}

	// Add common elements
	for path, element := range commonElements {
		profile.Elements[path] = element
	}

	// Add resource-specific elements
	switch resourceType {
	case "Patient":
		profile.Elements["identifier"] = FHIRElementDef{
			Path: "identifier",
			Min:  0,
			Max:  "*",
			Type: []FHIRElementType{{Code: "Identifier"}},
			Definition: "An identifier for this patient",
		}
		profile.Elements["name"] = FHIRElementDef{
			Path: "name",
			Min:  0,
			Max:  "*",
			Type: []FHIRElementType{{Code: "HumanName"}},
			Definition: "A name associated with the patient",
		}
		profile.Elements["gender"] = FHIRElementDef{
			Path: "gender",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "code"}},
			Definition: "male | female | other | unknown",
			Binding: &FHIRBinding{
				Strength: "required",
				ValueSet: "http://hl7.org/fhir/ValueSet/administrative-gender",
			},
		}
		profile.Elements["birthDate"] = FHIRElementDef{
			Path: "birthDate",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "date"}},
			Definition: "The date of birth for the individual",
		}
	case "Organization":
		profile.Elements["identifier"] = FHIRElementDef{
			Path: "identifier",
			Min:  0,
			Max:  "*",
			Type: []FHIRElementType{{Code: "Identifier"}},
			Definition: "Identifies this organization across multiple systems",
		}
		profile.Elements["name"] = FHIRElementDef{
			Path: "name",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "string"}},
			Definition: "Name used for the organization",
		}
	case "MessageHeader":
		profile.Elements["eventCoding"] = FHIRElementDef{
			Path: "eventCoding",
			Min:  0,
			Max:  "1",
			Type: []FHIRElementType{{Code: "Coding"}},
			Definition: "Code for the event this message represents or link to event definition",
		}
		profile.Elements["source"] = FHIRElementDef{
			Path: "source",
			Min:  1,
			Max:  "1",
			Type: []FHIRElementType{{Code: "BackboneElement"}},
			Definition: "Message source application",
		}
	}
}

// initializeBundleTemplates sets up bundle templates
func (m *FHIRBundleManager) initializeBundleTemplates() {
	// Message bundle template
	m.bundleTemplates["message"] = FHIRBundleTemplate{
		Type:      "message",
		Structure: []string{"MessageHeader"},
		Required:  []string{"MessageHeader"},
		Optional:  []string{"Patient", "Organization", "Encounter", "Observation"},
		Metadata: map[string]interface{}{
			"description": "FHIR Message Bundle",
			"purpose":     "Transport HL7 v2 messages as FHIR",
		},
	}

	// Document bundle template
	m.bundleTemplates["document"] = FHIRBundleTemplate{
		Type:      "document",
		Structure: []string{"Composition"},
		Required:  []string{"Composition"},
		Optional:  []string{"Patient", "Practitioner", "Organization"},
		Metadata: map[string]interface{}{
			"description": "FHIR Document Bundle",
			"purpose":     "Clinical document",
		},
	}

	// Transaction bundle template
	m.bundleTemplates["transaction"] = FHIRBundleTemplate{
		Type:      "transaction",
		Structure: []string{},
		Required:  []string{},
		Optional:  []string{},
		Metadata: map[string]interface{}{
			"description": "FHIR Transaction Bundle",
			"purpose":     "Atomic transaction",
		},
	}
}

// =====================================
// MAIN TRANSFORMATION INTERFACE
// =====================================

// Transform transforms a UniversalMessage containing source data to FHIR resources
func (s *FHIRTransformationService) Transform(ctx context.Context, message *UniversalMessage) error {
	// Start transformation tracking
	transformRecord := message.StartTransformation("FHIRTransformationService", message.MessageType, MessageTypeFHIR)

	startTime := time.Now()
	var outputSize int64 = 0
	var transformError error

	defer func() {
		message.CompleteTransformation(transformError == nil, outputSize, func() string {
			if transformError != nil {
				return transformError.Error()
			}
			return ""
		}())
	}()

	// Update message status
	message.UpdateStatus(StatusTransforming, "FHIRTransformationService", "Starting FHIR resource transformation")

	// Create transformation request
	request := &FHIRTransformRequest{
		MessageID:         message.ID,
		SourceData:        message.ParsedContent,
		SourceFormat:      message.MessageType,
		TargetProfile:     "base",
		FHIRVersion:       "R4",
		CreateBundle:      true,
		BundleType:        "message",
		ValidationLevel:   "MODERATE",
		IncludeReferences: true,
		GenerateNarrative: true,
		TransformationOptions: map[string]interface{}{
			"preserveSourceIdentifiers": true,
			"generateMissingIds":        true,
			"createCrossReferences":     true,
		},
	}

	// Perform transformation
	response, err := s.TransformToFHIR(ctx, request)
	if err != nil {
		transformError = err
		message.AddError("TRANSFORMATION", "FHIRTransformationService", "FHIR_TRANSFORM_FAILED",
			"Failed to transform to FHIR", err.Error(), true)
		return err
	}

	if !response.Success {
		transformError = fmt.Errorf("FHIR transformation failed: %v", response.Errors)
		for _, error := range response.Errors {
			message.AddError("TRANSFORMATION", "FHIRTransformationService", "FHIR_RESOURCE_ERROR",
				"FHIR resource creation error", error, false)
		}
		return transformError
	}

	// Store FHIR resources in parsed content
	fhirContent := map[string]interface{}{
		"fhirVersion": response.FHIRVersion,
		"profile":     response.Profile,
		"resources":   response.Resources,
		"bundle":      response.Bundle,
		"metrics":     response.ResourceMetrics,
	}
	message.ParsedContent = fhirContent

	// Add transformed content
	outputBytes, _ := json.Marshal(fhirContent)
	outputSize = int64(len(outputBytes))
	message.AddTransformedContent(MessageTypeFHIR, outputBytes, transformRecord.ID)

	// Add validation warnings
	for _, warning := range response.Warnings {
		message.AddWarning("VALIDATION", "FHIRTransformationService", "FHIR_VALIDATION_WARNING",
			warning, "Resource quality impact")
	}

	// Update status to transformed
	message.UpdateStatus(StatusTransformed, "FHIRTransformationService",
		fmt.Sprintf("FHIR transformation completed (%d resources, Quality: %.2f)",
			response.ResourceMetrics.TotalResources, response.QualityScore))

	// Update transformation metadata
	if transformRecord != nil {
		transformRecord.Metadata["fhirVersion"] = response.FHIRVersion
		transformRecord.Metadata["resourceCount"] = response.ResourceMetrics.TotalResources
		transformRecord.Metadata["qualityScore"] = response.QualityScore
		transformRecord.Metadata["profile"] = response.Profile
	}

	log.Printf("✅ FHIR transformation completed for message %s (%d resources, Duration: %v)",
		message.ID, response.ResourceMetrics.TotalResources, time.Since(startTime))

	return nil
}

// =====================================
// CORE FHIR TRANSFORMATION LOGIC
// =====================================

// TransformToFHIR performs complete FHIR transformation
func (s *FHIRTransformationService) TransformToFHIR(ctx context.Context, request *FHIRTransformRequest) (*FHIRTransformResponse, error) {
	startTime := time.Now()

	response := &FHIRTransformResponse{
		Success:              false,
		MessageID:            request.MessageID,
		FHIRVersion:          request.FHIRVersion,
		Profile:              request.TargetProfile,
		Resources:            []FHIRResource{},
		ValidationResults:    []FHIRValidationResult{},
		TransformationSteps:  []TransformationStep{},
		Warnings:             []string{},
		Errors:               []string{},
		ComplianceFlags:      make(map[string]bool),
		ProcessingMetrics:    ProcessingMetrics{},
	}

	// Step 1: Analyze source data and determine resource mapping
	analyzeStep := s.startTransformationStep("ANALYZE_SOURCE", string(request.SourceFormat), "MAPPING")
	resourceMappings, analyzeErr := s.analyzeSourceData(request.SourceData, request.SourceFormat)
	s.completeTransformationStep(&analyzeStep, analyzeErr, 0, len(resourceMappings))
	response.TransformationSteps = append(response.TransformationSteps, analyzeStep)

	if analyzeErr != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Source analysis error: %v", analyzeErr))
		return response, analyzeErr
	}

	// Step 2: Create FHIR resources
	createStep := s.startTransformationStep("CREATE_RESOURCES", "MAPPING", "FHIR")
	resources, createErr := s.createFHIRResources(resourceMappings, request)
	s.completeTransformationStep(&createStep, createErr, 0, len(resources))
	response.TransformationSteps = append(response.TransformationSteps, createStep)

	if createErr != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Resource creation error: %v", createErr))
		return response, createErr
	}

	response.Resources = resources

	// Step 3: Validate resources (if requested)
	if request.ValidationLevel != "NONE" {
		validateStep := s.startTransformationStep("VALIDATE_RESOURCES", "FHIR", "VALIDATED")
		validationResults, validateErr := s.validateFHIRResources(resources, request.ValidationLevel, request.TargetProfile)
		s.completeTransformationStep(&validateStep, validateErr, len(resources), len(validationResults))
		response.TransformationSteps = append(response.TransformationSteps, validateStep)

		response.ValidationResults = validationResults

		// Process validation results
		errorCount := 0
		warningCount := 0
		for _, result := range validationResults {
			if !result.IsValid {
				for _, issue := range result.Issues {
					if issue.Severity == "error" {
						errorCount++
						response.Errors = append(response.Errors,
							fmt.Sprintf("Validation error in %s: %s", result.ResourceType, issue.Details))
					} else if issue.Severity == "warning" {
						warningCount++
						response.Warnings = append(response.Warnings,
							fmt.Sprintf("Validation warning in %s: %s", result.ResourceType, issue.Details))
					}
				}
			}
		}

		// Calculate quality score
		totalValidations := len(validationResults)
		if totalValidations > 0 {
			validResources := totalValidations - errorCount
			response.QualityScore = float64(validResources) / float64(totalValidations) * 100.0
		} else {
			response.QualityScore = 100.0
		}

		// Fail if strict validation and errors found
		if request.ValidationLevel == "STRICT" && errorCount > 0 {
			return response, fmt.Errorf("strict validation failed with %d errors", errorCount)
		}
	}

	// Step 4: Create bundle (if requested)
	if request.CreateBundle {
		bundleStep := s.startTransformationStep("CREATE_BUNDLE", "FHIR", "BUNDLE")
		bundle, bundleErr := s.createFHIRBundle(resources, request.BundleType, request.MessageID)
		bundleSize := 0
		if bundle != nil {
			bundleSize = len(bundle.Entry)
		}
		s.completeTransformationStep(&bundleStep, bundleErr, len(resources), bundleSize)
		response.TransformationSteps = append(response.TransformationSteps, bundleStep)

		if bundleErr != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("Bundle creation warning: %v", bundleErr))
		} else {
			response.Bundle = bundle
		}
	}

	// Step 5: Generate narratives (if requested)
	if request.GenerateNarrative {
		narrativeStep := s.startTransformationStep("GENERATE_NARRATIVES", "FHIR", "ENHANCED")
		narrativeCount, narrativeErr := s.generateNarratives(resources)
		s.completeTransformationStep(&narrativeStep, narrativeErr, len(resources), narrativeCount)
		response.TransformationSteps = append(response.TransformationSteps, narrativeStep)

		if narrativeErr != nil {
			response.Warnings = append(response.Warnings, fmt.Sprintf("Narrative generation warning: %v", narrativeErr))
		}
	}

	// Calculate resource metrics
	response.ResourceMetrics = s.calculateResourceMetrics(resources)

	response.Success = true

	// Calculate processing metrics
	response.ProcessingMetrics = ProcessingMetrics{
		TotalTime:     time.Since(startTime),
		TransformTime: time.Since(startTime),
		MemoryUsage:   int64(len(response.Resources) * 1024), // Rough estimate
	}

	// Update service metrics
	s.updatePerformanceMetrics(response.ProcessingMetrics, len(resources))

	log.Printf("✅ FHIR transformation completed (Message: %s, Resources: %d, Quality: %.2f%%)",
		response.MessageID, len(resources), response.QualityScore)

	return response, nil
}

// Continue with remaining FHIR transformation methods in next part...