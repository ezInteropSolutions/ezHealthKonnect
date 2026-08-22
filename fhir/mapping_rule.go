// fhir/mapping_rule.go
//
// MappingRule was originally defined in fhir/transformation_engine.go
// alongside the now-deleted TransformationEngine (dead code — its two real
// handlers were permanent 501 stubs that never invoked it, see
// controllers/schema_fhir_transform_controller.go's own comment). MappingRule
// itself is a plain data struct with no dependency on TransformationEngine,
// and is still a real, live type — controllers/wizard_api_controller.go
// builds and reads these for the interface wizard UI — so it moved here
// rather than being deleted with the rest of that file.
package fhir

type MappingRule struct {
	ID              int                    `json:"id"`
	HL7MessageType  string                 `json:"hl7MessageType"`
	HL7Segment      string                 `json:"hl7Segment"`
	HL7Field        string                 `json:"hl7Field"`
	HL7Component    string                 `json:"hl7Component,omitempty"`
	FHIRResource    string                 `json:"fhirResource"`
	FHIRProfile     string                 `json:"fhirProfile"`
	FHIRPath        string                 `json:"fhirPath"`
	TransformLogic  map[string]interface{} `json:"transformLogic"`
	Condition       string                 `json:"condition,omitempty"`
	Priority        int                    `json:"priority"`
	Required        bool                   `json:"required"`
	SchemaValidated bool                   `json:"schemaValidated"`
}
