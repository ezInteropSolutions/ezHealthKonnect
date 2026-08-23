// services/hl7_fhir_transform_v9_template_test.go
//
// convertV9TemplateToFieldMappings is the function that parses EVERY OOB
// template's stored JSON (hl7_fhir_templates.template_config) into the
// []FieldMapping the rest of the engine runs on — used by both
// loadFromV9OOBTemplates (the main OOB-template load path) and, indirectly,
// by ComputeDelta's base-template comparison. Pure JSON-shape parsing, no DB,
// and previously untested despite being on the hot path for most templates.
package services

import "testing"

func TestConvertV9TemplateToFieldMappings_ResourcesKeyFormat(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"Patient": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"hl7Path": "PID.8", "fhirPath": "Patient.gender", "transform": "gender_mapping", "required": true, "confidence": 0.95},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("got %d mappings, want 1", len(mappings))
	}
	fm := mappings[0]
	if fm.SegmentName != "PID" || fm.HL7Field != "8" {
		t.Errorf("HL7 source = %s.%s, want PID.8", fm.SegmentName, fm.HL7Field)
	}
	if fm.FHIRResourceType != "Patient" || fm.FHIRElementPath != "Patient.gender" {
		t.Errorf("FHIR target = %s / %s, want Patient / Patient.gender", fm.FHIRResourceType, fm.FHIRElementPath)
	}
	if !fm.IsRequired || fm.Confidence != 0.95 {
		t.Errorf("required/confidence = %v/%v, want true/0.95", fm.IsRequired, fm.Confidence)
	}
}

func TestConvertV9TemplateToFieldMappings_LegacyMappingsKeyFormat(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"mappings": map[string]interface{}{
			"Patient": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"hl7Path": "PID.7", "fhirPath": "Patient.birthDate"},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 || mappings[0].HL7Field != "7" {
		t.Errorf("got %+v, want a single PID.7 mapping via the legacy 'mappings' key", mappings)
	}
}

func TestConvertV9TemplateToFieldMappings_NeitherKeyPresent_ReturnsError(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	_, err := s.convertV9TemplateToFieldMappings(map[string]interface{}{})
	if err == nil {
		t.Error("expected an error when neither 'resources' nor 'mappings' is present, got nil")
	}
}

func TestConvertV9TemplateToFieldMappings_InvalidResourcesFormat_ReturnsError(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	_, err := s.convertV9TemplateToFieldMappings(map[string]interface{}{"resources": "not-a-map"})
	if err == nil {
		t.Error("expected an error for a non-map 'resources' value, got nil")
	}
}

func TestConvertV9TemplateToFieldMappings_SkipsSequenceIDMappedToResourceID(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"DiagnosticReport": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"hl7Path": "OBR.1", "fhirPath": "DiagnosticReport.id", "hl7DataType": "SI"},
					map[string]interface{}{"hl7Path": "OBR.4", "fhirPath": "DiagnosticReport.code"},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 || mappings[0].HL7Field != "4" {
		t.Errorf("got %+v, want only OBR.4 (OBR.1 SI->.id mapping must be skipped)", mappings)
	}
}

func TestConvertV9TemplateToFieldMappings_CapturesValueMap(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"Encounter": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{
						"hl7Path": "PV1.2", "fhirPath": "Encounter.class.code",
						"valueMap": map[string]interface{}{"I": "IMP", "O": "AMB"},
					},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vm, ok := mappings[0].TransformationRules["valueMap"]
	if !ok {
		t.Fatal("expected TransformationRules[\"valueMap\"] to be populated")
	}
	vmMap, ok := vm.(map[string]interface{})
	if !ok || vmMap["I"] != "IMP" {
		t.Errorf("valueMap = %+v, want I->IMP present", vm)
	}
}

func TestConvertV9TemplateToFieldMappings_LookupTransform_ResolvesValueSetAndFallback(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"valueSets": map[string]interface{}{
			"siuFillerStatus": map[string]interface{}{"BOOKED": "booked", "CANCELLED": "cancelled"},
		},
		"resources": map[string]interface{}{
			"Appointment": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{
						"hl7Path": "SCH.25", "fhirPath": "Appointment.status",
						"transform": "lookup:siuFillerStatus",
						"fallbackTransform": "default_status", "fallbackField": "SCH.1",
					},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := mappings[0].TransformationRules
	vs, ok := rules["valueSet"].(map[string]interface{})
	if !ok || vs["BOOKED"] != "booked" {
		t.Errorf("valueSet = %+v, want the resolved siuFillerStatus map with BOOKED->booked", rules["valueSet"])
	}
	if rules["fallbackTransform"] != "default_status" || rules["fallbackField"] != "SCH.1" {
		t.Errorf("fallback fields = %+v, want fallbackTransform=default_status, fallbackField=SCH.1", rules)
	}
}

func TestConvertV9TemplateToFieldMappings_CapturesHL7TableAndCondition(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"Patient": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{
						"hl7Path": "PID.16", "fhirPath": "Patient.maritalStatus",
						"hl7Table": "0002", "condition": "PID.16 present",
					},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := mappings[0].TransformationRules
	if rules["hl7Table"] != "0002" {
		t.Errorf("hl7Table = %v, want 0002", rules["hl7Table"])
	}
	if rules["condition"] != "PID.16 present" {
		t.Errorf("condition = %v, want \"PID.16 present\"", rules["condition"])
	}
}

func TestConvertV9TemplateToFieldMappings_StaticValue_EmptyHL7Source(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"MessageHeader": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"hl7Path": "", "fhirPath": "MessageHeader.source.name", "staticValue": "ezHealthKonnect"},
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("got %d mappings, want 1", len(mappings))
	}
	fm := mappings[0]
	if fm.StaticValue != "ezHealthKonnect" {
		t.Errorf("StaticValue = %q, want ezHealthKonnect", fm.StaticValue)
	}
	if fm.SegmentName != "" || fm.HL7Field != "" {
		t.Errorf("HL7 source = %s.%s, want empty (static_value has no HL7 source)", fm.SegmentName, fm.HL7Field)
	}
}

func TestConvertV9TemplateToFieldMappings_SkipsInvalidEntries(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	templateData := map[string]interface{}{
		"resources": map[string]interface{}{
			"Patient": map[string]interface{}{
				"mappings": []interface{}{
					map[string]interface{}{"fhirPath": "Patient.gender"},        // missing hl7Path
					map[string]interface{}{"hl7Path": "PID.8"},                   // missing fhirPath
					"not-even-a-map",                                            // wrong shape entirely
					map[string]interface{}{"hl7Path": "PID.7", "fhirPath": "Patient.birthDate"}, // the only valid one
				},
			},
		},
	}
	mappings, err := s.convertV9TemplateToFieldMappings(templateData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mappings) != 1 || mappings[0].HL7Field != "7" {
		t.Errorf("got %+v, want only the single valid PID.7 mapping, malformed entries skipped", mappings)
	}
}
