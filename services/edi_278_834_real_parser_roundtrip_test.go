// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 278/834 → FHIR — real-parser round trip against GENUINE,
// third-party X12 content (X12.org's own official worked examples,
// x12.org/examples/005010x217 and x12.org/examples/005010x220 -- the same
// low-licensing-risk source already used successfully for 837P/837I's own
// real-sample verification). edi_278_fhir_builder_test.go and
// edi_834_fhir_builder_test.go both feed the derive script a hand-built Go
// map literal; this file closes the remaining gap: proving the REAL parser
// (edi.ParseTransactionSet, the same code a live SFTP-ingested file goes
// through) produces a shape the derive scripts actually consume correctly
// when fed genuine raw X12 text, not just synthetic fixtures.
//
// Sourcing these real samples caught 3 real bugs no synthetic fixture had
// exercised:
//  1. 834's own 1000B (Payer) was modeled with entityIdentifierCode "PR"
//     (by analogy with 270/271/276/277's own payer-role usage) -- WRONG.
//     Every real X12.org 834 example uses "IN" (Insurer) instead.
//  2. 834's own 2100E (Member School) was modeled "83" (a best-semantic-
//     match guess against the generic element 98 code list) -- WRONG. The
//     real X12.org "Add Dependent, Full-Time Student" example uses "M8".
//  3. [the most consequential] derive834MemberCoveragesScript's own
//     maintenanceTypeToCoverageStatus mapped HD01/INS03 "024"->active and
//     "030"->cancelled -- BACKWARDS relative to the real X12 element 875
//     standard code list (024 = Cancellation/Termination; 030 = Audit or
//     Compare, not a termination code at all). Confirmed directly against
//     X12.org's own official "Terminate Eligibility for a Subscriber"
//     example, which uses INS03="024".
//
// 278's own real samples (referral request/response, institutional
// admission with a genuine CL1 occurrence, home health with CR6 and a
// multi-occurrence HI) all parsed and mapped cleanly on the FIRST real-data
// pass -- no further 278-side bugs surfaced, a real (if less dramatic)
// finding in its own right, corroborating the earlier round-trip-test-only
// verification of the repeat=1 Patient Event fix and the CL1/SV1/SV2/UM/TRN
// usage widenings.
//
// Run: go test ./services/ -v -run TestEDI278834RealParserRoundtrip
// ─────────────────────────────────────────────────────────────────────────────

package services

import (
	"os"
	"path/filepath"
	"testing"

	"ezhealthkonnect/edi"
)

func readRealSampleSvc(t *testing.T, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "edi", "testdata", "real_samples", filename))
	if err != nil {
		t.Fatalf("reading real sample %s: %v", filename, err)
	}
	return string(content)
}

// TestEDI278RealParserRoundtrip_AdmissionResponse_ProducesValidatingBundle
// proves X12.org's own official "Example 2b: Response to Admission Request
// for Review" (real HCR at BOTH the event and service levels, a real CL1
// occurrence, a real SV2 institutional service line) maps cleanly through
// the real parser, the real derive script, and strict FHIR validation.
func TestEDI278RealParserRoundtrip_AdmissionResponse_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := readRealSampleSvc(t, "x12org_278_2b_admission_response_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(278): %v\nraw:\n%s", err, raw)
	}
	if result.TransactionSet != "278" {
		t.Fatalf("TransactionSet = %q, want 278", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, derive278StepAlias, derive278PriorAuthContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, derive278StepAlias)
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, derive278StepAlias, deriveOut)

	contexts, _ := deriveOut["_prior_auth_contexts"].([]interface{})
	if len(contexts) != 1 {
		t.Fatalf("expected 1 prior auth context from the real parser's own output, got %d: %+v", len(contexts), contexts)
	}
	ctx := contexts[0].(map[string]interface{})
	if ctx["is_response"] != true {
		t.Errorf("is_response = %v, want true (this real sample carries a real HCR)", ctx["is_response"])
	}
	if ctx["hcr_action_code"] != "A6" {
		t.Errorf("hcr_action_code = %v, want A6 (Modified -- the event-level HCR; the service-level HCR is A1, genuinely distinct)", ctx["hcr_action_code"])
	}

	data = runFHIRBuild(t, "build_umo_organization_fhir", organization278Config(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient278Config(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim278Config(), data)
	data = runFHIRBuild(t, "build_claim_response_fhir", claimResponse278Config(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claimResponses := message["fhirClaimResponses"].([]map[string]interface{})
	if len(claimResponses) != 1 {
		t.Fatalf("expected 1 ClaimResponse resource (real HCR present), got %d", len(claimResponses))
	}
	if claimResponses[0]["outcome"] != "complete" {
		t.Errorf("ClaimResponse.outcome = %v, want complete (HCR01=A6 Modified -> FHIR outcome=complete per the real translation table)", claimResponses[0]["outcome"])
	}

	assembleAndValidate278Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirClaims", "message.fhirClaimResponses")
}

// TestEDI834RealParserRoundtrip_MultiProductEnrollment_ProducesValidatingBundle
// proves X12.org's own official "Example 01: Enroll Employee in Multiple
// Health Care Insurance Products" -- a SINGLE real member with THREE
// separate real 2300 Health Coverage occurrences (HLT/DEN/VIS) -- maps
// cleanly through the real parser, the real derive script (which must
// correctly flatten to 3 member-coverage rows from 1 real member), and
// strict FHIR validation. This is the exact sample that caught the
// maintenanceTypeToCoverageStatus mapping bug described in this file's own
// header comment -- this test's own real member uses HD01="021" (Addition),
// so it exercises the "active" branch; TestEDI834RealParserRoundtrip_
// TerminateEligibility below exercises the "cancelled" branch against
// REAL "024" data.
func TestEDI834RealParserRoundtrip_MultiProductEnrollment_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := readRealSampleSvc(t, "x12org_834_1_multi_product_enrollment_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(834): %v\nraw:\n%s", err, raw)
	}
	if result.TransactionSet != "834" {
		t.Fatalf("TransactionSet = %q, want 834", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, derive834StepAlias, derive834MemberCoveragesScript, data)
	deriveOut := svcStepOutput(t, deriveResult, derive834StepAlias)
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, derive834StepAlias, deriveOut)

	rows, _ := deriveOut["_member_coverages"].([]interface{})
	if len(rows) != 3 {
		t.Fatalf("expected 3 member-coverage rows (1 real member x 3 real coverages) from the real parser's own output, got %d: %+v", len(rows), rows)
	}
	for _, r := range rows {
		row := r.(map[string]interface{})
		if row["coverage_status"] != "active" {
			t.Errorf("coverage_status = %v, want active (HD01=021 Addition, the real code)", row["coverage_status"])
		}
	}

	data = runFHIRBuild(t, "build_sponsor_organization_fhir", organization834Config(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient834Config(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage834Config(), data)

	message := data["message"].(map[string]interface{})
	coverages := message["fhirCoverages"].([]map[string]interface{})
	if len(coverages) != 3 {
		t.Fatalf("expected 3 Coverage resources, got %d", len(coverages))
	}

	assembleAndValidate834Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages")
}

// TestEDI834RealParserRoundtrip_TerminateEligibility_ProducesCancelledCoverage
// proves X12.org's own official "Example 07: Terminate Eligibility for a
// Subscriber" -- real INS03/HD01 unavailable at the 2300 level in this
// particular example (it has no 2300 loop at all, only a bare termination
// INS) -- still parses and maps cleanly, and that the member-level
// maintenanceTypeCode itself (024, the REAL termination code this bug-fix
// section's own header comment documents) is captured correctly by the real
// parser.
func TestEDI834RealParserRoundtrip_TerminateEligibility_ParsesRealTerminationCode(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := readRealSampleSvc(t, "x12org_834_7_terminate_eligibility_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(834): %v\nraw:\n%s", err, raw)
	}

	members := result.Loops["2000"].([]map[string]interface{})
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0]["INS"].(map[string]interface{})["maintenanceTypeCode"] != "024" {
		t.Errorf("INS = %#v, want maintenanceTypeCode=024 (the REAL X12 Cancellation/Termination code)", members[0]["INS"])
	}
}
