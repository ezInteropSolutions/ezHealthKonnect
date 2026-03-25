package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	"ezhealthkonnect/fhir/r4"

	"github.com/gin-gonic/gin"
)

// FHIRValidatorController exposes an ad-hoc FHIR validation endpoint used by
// the FHIR Validator tool page (fhir-validator.html).
type FHIRValidatorController struct{}

func NewFHIRValidatorController() *FHIRValidatorController {
	return &FHIRValidatorController{}
}

func (c *FHIRValidatorController) RegisterRoutes(group *gin.RouterGroup) {
	group.POST("/validate", c.Validate)
}

// validateRequest is the JSON body sent from the browser tool.
type validateRequest struct {
	// Resource holds the raw FHIR JSON the user pasted.
	Resource json.RawMessage `json:"resource" binding:"required"`
	// Options
	Level   string `json:"level"`   // "basic" | "standard" | "strict"
	Profile string `json:"profile"` // "base" | "us-core"
	Version string `json:"version"` // "R4" | "R5"
}

// Validate validates a raw FHIR JSON resource/bundle submitted from the tool UI.
func (c *FHIRValidatorController) Validate(ctx *gin.Context) {
	var req validateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request: " + err.Error()})
		return
	}

	// Parse the raw JSON into a generic map.
	var resource map[string]interface{}
	if err := json.Unmarshal(req.Resource, &resource); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "resource is not valid JSON: " + err.Error()})
		return
	}

	// Defaults
	level := req.Level
	if level == "" {
		level = "standard"
	}
	profile := req.Profile
	if profile == "" {
		profile = "base"
	}
	version := req.Version
	if version == "" {
		version = "R4"
	}

	opts := r4.ValidationOptions{
		Version:      version,
		Profile:      profile,
		Level:        r4.ParseLevel(level),
		ValidateRefs: true,
	}

	start := time.Now()
	validator := r4.GetValidator()

	rt, _ := resource["resourceType"].(string)
	isBundle := rt == "Bundle"

	type resourceSummary struct {
		ResourceType string   `json:"resource_type"`
		ResourceID   string   `json:"resource_id"`
		Valid        bool     `json:"valid"`
		Errors       []string `json:"errors,omitempty"`
		Warnings     []string `json:"warnings,omitempty"`
	}

	var allErrors, allWarnings []string
	var resources []resourceSummary
	var resourceCount int

	if isBundle {
		result := validator.ValidateBundle(resource, opts)
		resourceCount = len(result.Resources)
		for _, iss := range result.BundleIssues {
			s := fmtValidatorIssue(iss)
			if iss.Severity == "error" {
				allErrors = append(allErrors, s)
			} else {
				allWarnings = append(allWarnings, s)
			}
		}
		for _, rr := range result.Resources {
			sum := resourceSummary{
				ResourceType: rr.ResourceType,
				ResourceID:   rr.ResourceID,
				Valid:        rr.Valid,
			}
			for _, e := range rr.Errors {
				s := fmtValidatorIssue(e)
				sum.Errors = append(sum.Errors, s)
				allErrors = append(allErrors, s)
			}
			for _, w := range rr.Warnings {
				s := fmtValidatorIssue(w)
				sum.Warnings = append(sum.Warnings, s)
				allWarnings = append(allWarnings, s)
			}
			resources = append(resources, sum)
		}
	} else {
		rr := validator.ValidateResource(resource, opts)
		resourceCount = 1
		sum := resourceSummary{
			ResourceType: rr.ResourceType,
			ResourceID:   rr.ResourceID,
			Valid:        rr.Valid,
		}
		for _, e := range rr.Errors {
			s := fmtValidatorIssue(e)
			sum.Errors = append(sum.Errors, s)
			allErrors = append(allErrors, s)
		}
		for _, w := range rr.Warnings {
			s := fmtValidatorIssue(w)
			sum.Warnings = append(sum.Warnings, s)
			allWarnings = append(allWarnings, s)
		}
		resources = append(resources, sum)
	}

	durationMs := time.Since(start).Milliseconds()

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"valid":          len(allErrors) == 0,
			"resource_count": resourceCount,
			"error_count":    len(allErrors),
			"warning_count":  len(allWarnings),
			"errors":         allErrors,
			"warnings":       allWarnings,
			"resources":      resources,
			"mode":           map[bool]string{true: "bundle", false: "resource"}[isBundle],
			"level":          level,
			"profile":        profile,
			"version":        version,
			"duration_ms":    durationMs,
		},
	})
}

func fmtValidatorIssue(iss r4.ValidationIssue) string {
	if iss.Path != "" {
		return "[" + iss.Code + "] " + iss.Path + ": " + iss.Message
	}
	return "[" + iss.Code + "] " + iss.Message
}
