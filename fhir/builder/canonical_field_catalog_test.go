package builder

import (
	"testing"

	"ezhealthkonnect/fhir/r4"
)

const testFHIRSchemaDir = "../../schemas/fhir"

func initRegistry(t *testing.T) {
	t.Helper()
	if err := r4.ForceReinit(testFHIRSchemaDir); err != nil {
		t.Fatalf("r4.ForceReinit failed: %v", err)
	}
}

func TestResourceTypeCatalog_IncludesPatient(t *testing.T) {
	initRegistry(t)
	types := ResourceTypeCatalog("R4")
	found := false
	for _, rt := range types {
		if rt == "Patient" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ResourceTypeCatalog(\"R4\") to include Patient, got %v", types)
	}
}

func TestProfileCatalog_IncludesBase(t *testing.T) {
	initRegistry(t)
	profiles := ProfileCatalog("R4", "Patient")
	found := false
	for _, p := range profiles {
		if p == "base" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ProfileCatalog(\"R4\", \"Patient\") to include base, got %v", profiles)
	}
}

// TestFieldCatalog_StripsResourceTypePrefix verifies element paths compiled as
// "Patient.birthDate" are exposed with the ResourceType prefix stripped
// (fhir_build_executor.go's TargetPath is always relative to the resource
// being built, matching CDAFieldDef.Key's flat convention on the CDA side).
func TestFieldCatalog_StripsResourceTypePrefix(t *testing.T) {
	initRegistry(t)
	fields := FieldCatalog("R4", "Patient", "base")
	if len(fields) == 0 {
		t.Fatal("expected non-empty field catalog for Patient/base/R4")
	}
	found := false
	for _, f := range fields {
		if f.Key == "Patient.birthDate" {
			t.Errorf("expected ResourceType prefix to be stripped, got raw key %q", f.Key)
		}
		if f.Key == "birthDate" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field catalog to include stripped key \"birthDate\", got %+v", fields)
	}
}

func TestFieldCatalog_UnknownResourceType_ReturnsNil(t *testing.T) {
	initRegistry(t)
	fields := FieldCatalog("R4", "NotARealResourceType", "base")
	if fields != nil {
		t.Errorf("expected nil for unknown resourceType, got %+v", fields)
	}
}

// TestFieldCatalog_IncludesBindingInfo verifies terminology binding data
// (previously validator-internal only — fhir/r4/validator.go's own
// validateTerminology was the sole consumer) is now surfaced through the
// same catalog the fhir.build config UI reads. Patient.gender/base is a
// real, already-compiled fixture: a required binding to AdministrativeGender.
func TestFieldCatalog_IncludesBindingInfo(t *testing.T) {
	initRegistry(t)
	fields := FieldCatalog("R4", "Patient", "base")
	var gender *CanonicalFieldInfo
	for i := range fields {
		if fields[i].Key == "gender" {
			gender = &fields[i]
			break
		}
	}
	if gender == nil {
		t.Fatal("expected field catalog to include \"gender\"")
	}
	if gender.BindingStrength != "required" {
		t.Errorf("Patient.gender BindingStrength = %q, want \"required\"", gender.BindingStrength)
	}
	if gender.ValueSetURL != "http://hl7.org/fhir/ValueSet/administrative-gender" {
		t.Errorf("Patient.gender ValueSetURL = %q, want administrative-gender ValueSet", gender.ValueSetURL)
	}
}

// TestFieldCatalog_UnboundField_HasNoBindingInfo verifies a field with no
// terminology binding (e.g. birthDate, a date field) leaves the new fields
// at their zero value rather than some sentinel/placeholder.
func TestFieldCatalog_UnboundField_HasNoBindingInfo(t *testing.T) {
	initRegistry(t)
	fields := FieldCatalog("R4", "Patient", "base")
	for _, f := range fields {
		if f.Key == "birthDate" {
			if f.BindingStrength != "" || f.ValueSetURL != "" {
				t.Errorf("birthDate should have no binding info, got BindingStrength=%q ValueSetURL=%q", f.BindingStrength, f.ValueSetURL)
			}
			return
		}
	}
	t.Fatal("expected field catalog to include \"birthDate\"")
}
