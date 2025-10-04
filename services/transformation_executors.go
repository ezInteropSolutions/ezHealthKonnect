package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"ezhealthkonnect/models"

	"github.com/dop251/goja"
)

type ValidationExecutor struct{}
func NewValidationExecutor() *ValidationExecutor { return &ValidationExecutor{} }
func (ve *ValidationExecutor) GetSupportedType() string { return "pre.validation" }
func (ve *ValidationExecutor) Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error) {
	return input, nil
}

type HL7ToFHIRMappingExecutor struct {
	transformService *HL7FHIRTransformServiceV3
}
func NewHL7ToFHIRMappingExecutor(db *sql.DB) *HL7ToFHIRMappingExecutor {
	return &HL7ToFHIRMappingExecutor{transformService: NewHL7FHIRTransformServiceV3(db)}
}
func (hfe *HL7ToFHIRMappingExecutor) GetSupportedType() string { return "core.mapping" }
func (hfe *HL7ToFHIRMappingExecutor) Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error) {
	interfaceID, _ := step.Config["interface_id"].(string)
	if interfaceID == "" {
		return input, fmt.Errorf("interface_id required")
	}

	// Get message_type from injected config (OOB runtime context)
	messageType, _ := step.Config["message_type"].(string)

	// DEBUG: Log what we received
	log.Printf("🔍 HL7ToFHIRMappingExecutor: interface_id=%s, message_type=%s", interfaceID, messageType)
	log.Printf("🔍 HL7ToFHIRMappingExecutor: step.Config keys: %v", getKeys(step.Config))

	// The input is the complete parsed HL7 structure (with enhancedSegments, messageType, etc.)
	// Pass it directly to the transform service along with messageType from config
	req := &TransformRequest{
		ParsedHL7Data: input, // Pass complete structure with metadata
		MessageType:   messageType, // Use messageType from injected config
		FHIRVersion:   "R4",
		CreateBundle:  true,
		InterfaceID:   interfaceID,
	}

	log.Printf("✅ HL7ToFHIRMappingExecutor: Calling Transform with MessageType=%s", req.MessageType)

	resp, err := hfe.transformService.Transform(ctx, req)
	if err != nil {
		return input, fmt.Errorf("HL7→FHIR transformation failed: %w", err)
	}

	// Return ONLY the FHIR bundle (not the original HL7 structure)
	// The input message is already stored separately
	output := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "transaction",
		"entry":        resp.Bundle,
	}

	return output, nil
}

type FHIRValidationExecutor struct{}
func NewFHIRValidationExecutor() *FHIRValidationExecutor { return &FHIRValidationExecutor{} }
func (fve *FHIRValidationExecutor) GetSupportedType() string { return "post.validation" }
func (fve *FHIRValidationExecutor) Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error) {
	return input, nil
}

type CustomScriptExecutor struct{}
func NewCustomScriptExecutor() *CustomScriptExecutor { return &CustomScriptExecutor{} }
func (cse *CustomScriptExecutor) GetSupportedType() string { return "custom" }
func (cse *CustomScriptExecutor) Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error) {
	if step.ScriptContent == nil {
		return input, fmt.Errorf("script required")
	}
	vm := goja.New()
	inputJSON, _ := json.Marshal(input)
	script := fmt.Sprintf(`
		var input = %s;
		%s
		JSON.stringify(transform(input));
	`, string(inputJSON), *step.ScriptContent)
	value, err := vm.RunString(script)
	if err != nil {
		return input, err
	}
	var output map[string]interface{}
	json.Unmarshal([]byte(value.String()), &output)
	return output, nil
}
