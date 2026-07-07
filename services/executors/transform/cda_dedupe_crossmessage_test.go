// services/executors/transform/cda_dedupe_crossmessage_test.go
// Non-DB unit tests for cda.dedupe's crossMessage support: patient-identifier
// resolution and config validation. The actual registry check
// (checkAndRegisterCrossMessage) needs a real Postgres connection — per this
// codebase's convention of verifying DB-backed behavior via live Docker
// integration rather than mocking *sql.DB, that path is verified live
// (see project verification notes), not with a Go-level fake here.
package transform

import (
	"context"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
)

// ── resolvePatientIdentifierKey ────────────────────────────────────────────

func TestResolvePatientIdentifierKey_FindsMatchingRoot(t *testing.T) {
	doc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{
				Ids: []cdadocument.CDAII{
					{Root: "2.16.840.1.113883.4.1", Extension: "123-45-6789"}, // SSN
					{Root: "1.2.3.4.5.6", Extension: "MRN00042"},             // site MRN
				},
			},
		},
	}
	key, ok := resolvePatientIdentifierKey(doc, "1.2.3.4.5.6")
	if !ok || key != "MRN00042" {
		t.Fatalf("resolvePatientIdentifierKey = (%q, %v), want (\"MRN00042\", true)", key, ok)
	}
}

func TestResolvePatientIdentifierKey_RootNotPresent_NotFound(t *testing.T) {
	doc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{
				Ids: []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.1", Extension: "123-45-6789"}},
			},
		},
	}
	_, ok := resolvePatientIdentifierKey(doc, "9.9.9.9")
	if ok {
		t.Error("resolvePatientIdentifierKey found a match for a root the patient doesn't have")
	}
}

func TestResolvePatientIdentifierKey_EmptyRootConfig_NotFound(t *testing.T) {
	doc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{Ids: []cdadocument.CDAII{{Root: "1.2.3", Extension: "X"}}},
		},
	}
	_, ok := resolvePatientIdentifierKey(doc, "")
	if ok {
		t.Error("resolvePatientIdentifierKey should never match with an empty configured root — no auto-detection")
	}
}

func TestResolvePatientIdentifierKey_NoPatientIds_NotFound(t *testing.T) {
	doc := &cdadocument.CDADocument{}
	_, ok := resolvePatientIdentifierKey(doc, "1.2.3")
	if ok {
		t.Error("resolvePatientIdentifierKey on a document with no patient identifiers should not match")
	}
}

// ── Validate: crossMessage requires patientIdentifierRoot ─────────────────

func TestValidate_CrossMessageWithoutPatientRoot_Errors(t *testing.T) {
	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{
		StepType: "cda.dedupe",
		Config:   map[string]interface{}{"crossMessage": true},
	}
	if err := exec.Validate(step); err == nil {
		t.Error("Validate() = nil, want an error (crossMessage without patientIdentifierRoot)")
	}
}

func TestValidate_CrossMessageWithPatientRoot_Accepted(t *testing.T) {
	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{
		StepType: "cda.dedupe",
		Config:   map[string]interface{}{"crossMessage": true, "patientIdentifierRoot": "1.2.3.4.5.6"},
	}
	if err := exec.Validate(step); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidate_CrossMessageFalse_PatientRootNotRequired(t *testing.T) {
	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{
		StepType: "cda.dedupe",
		Config:   map[string]interface{}{"crossMessage": false},
	}
	if err := exec.Validate(step); err != nil {
		t.Errorf("Validate() = %v, want nil (crossMessage off shouldn't require patientIdentifierRoot)", err)
	}
}

// ── Execute: crossMessage requested but db is nil (no connection configured) ──

func TestExecute_CrossMessageRequested_NoDBConfigured_ReturnsError(t *testing.T) {
	doc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{Ids: []cdadocument.CDAII{{Root: "1.2.3", Extension: "MRN1"}}},
		},
		SectionsByKey: map[string]*cdadocument.CDASection{
			"vitalSigns": {Key: "vitalSigns", Entries: []cdadocument.CDAEntry{vitalEntry("8480-6", "20200101")}},
		},
	}
	exec := NewCDADedupeExecutor(nil) // no DB
	step := &models.TransformationStep{
		StepType: "cda.dedupe", Enabled: true,
		Config: map[string]interface{}{
			"sections":              []interface{}{"vitalSigns"},
			"crossMessage":          true,
			"patientIdentifierRoot": "1.2.3",
		},
	}
	inputData := map[string]interface{}{
		"interfaceId": "test-interface-1", // resolveInterfaceIDForDedupe's first tier
		"message":     map[string]interface{}{"_cdaDocument": doc},
	}

	_, err := exec.Execute(context.Background(), step, inputData)
	if err == nil {
		t.Error("Execute() = nil error, want an error (crossMessage on, patient key + interface_id resolvable, but no db configured)")
	}
}
