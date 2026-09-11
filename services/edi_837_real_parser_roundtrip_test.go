// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 837P/837I → FHIR — real-parser round trip.
//
// edi_837p_fhir_builder_test.go and edi_837i_fhir_builder_test.go both feed
// the derive script a hand-built Go map literal shaped to match
// edi/loop_engine.go's own documented PARSE-direction output exactly (array
// vs. bare-object per segment MaxUse, verified directly against that file's
// source, not assumed). This file closes the remaining gap: proving the
// REAL parser (edi.ParseTransactionSet, the same code a live SFTP-ingested
// file goes through) actually produces that shape when fed genuine raw X12
// text, not just that the derive script correctly consumes a shape *I*
// typed by hand.
//
// Two sample sources are used:
//   - 2 small, self-authored samples (not sourced from any third-party
//     file), deliberately including a DTP*472*RD8 date RANGE (not just D8)
//     -- the exact real-world shape that caught a genuine bug earlier this
//     round (Claim.item.servicedDate can't hold "20260210-20260212"; fixed
//     by resolveServicedDate splitting RD8 into servicedPeriod.start/.end).
//   - edi/testdata/real_samples/databricks_837{p,i}_sample.txt -- genuine,
//     unedited files from databricks-industry-solutions/x12-edi-parser
//     (test-fixture use only, per its own "DB license" scope, not
//     distributed as part of this application -- user-confirmed 2026-09-11).
//     The 837I sample caught TWO further real bugs no synthetic fixture (or
//     the self-authored samples above) had exercised:
//       1. Real institutional claims carry MULTIPLE separate HI segment
//          occurrences per claim (one per qualifier-group), not one --
//          edi/schemas/x12_005010/segments/HI.json's own maxUse was wrongly
//          "1"; corrected to ">1" (see that file's own maxUse-correction
//          note), with both derive scripts' extractDiagnoses/extractProcedures
//          updated to flatten codes[] across every HI occurrence.
//       2. Real revenue-only service lines (no SV202 procedure code -- common
//          for room & board, etc.) would otherwise leave the unconditionally-
//          required Claim.item.productOrService unpopulated; 837I's own
//          extractServiceLines now falls back to the revenue code itself.
//   - edi/testdata/real_samples/x12org_837{p,i}_*_sample.txt -- X12.org's own
//     official worked examples ("Ben Kildare Service" 837P,
//     x12.org/examples/005010x222/example-1-commercial-health-insurance;
//     "Jones Hospital" 837I,
//     x12.org/examples/005010x223/example-1a-institutional-claim), the
//     standards body's own published educational material, envelope
//     segments synthesized (the page shows only ST...SE) but every body
//     segment preserved exactly as published. These caught THREE further
//     real bugs a Databricks-shaped or synthetic fixture never exercised:
//       3. [937I bug #2 correction] Revised per user direction: institutional
//          Claim.item.productOrService is NOT a conditional CPT-else-revenue
//          fallback -- it's revenue code ALWAYS as coding[0] (the one
//          identifier every real institutional line carries) plus the CPT
//          code, when SV202 is actually present, ALWAYS as an ADDITIONAL
//          coding[1] (never a replacement). The Jones Hospital sample proves
//          this directly: both its service lines carry a revenue code AND a
//          CPT code together (SV2*0305*HC:85025*... /
//          SV2*0730*HC:93005*...). See 837I's own extractServiceLines and
//          claim837IBuildConfig's item repeatingGroup for the corrected
//          shape.
//       4. 837I's HI-qualifier diagnosis/procedure filter only ever matched
//          the "AB"-prefixed ICD-10 qualifiers (ABK/ABF/ABJ). The Jones
//          Hospital sample predates the ICD-10 transition and uses the
//          equally-valid ICD-9 qualifiers (BK/BF/BJ) instead -- silently
//          dropping every diagnosis with the old filter. Fixed with an
//          explicit qualifier whitelist covering both eras
//          (DIAGNOSIS_HI_QUALIFIERS/PROCEDURE_HI_QUALIFIERS in
//          edi_837i_fhir_builder_test.go).
//       5. [core, affects both variants] The dependent-patient relationship
//          code (Coverage.relationship, e.g. "child") was read from the
//          SUBSCRIBER's own SBR02 (2000B). Real, spec-compliant 837 data
//          leaves SBR02 BLANK whenever a 2000C dependent loop exists and
//          carries the real relationship on the DEPENDENT's own PAT01
//          (2000C) instead -- confirmed directly in the Ben Kildare sample
//          (SBR*P**2222-SJ*******CI [SBR02 blank] -> PAT*19 [child]).
//          edi/schemas/x12_005010/segments/PAT.json had PAT01 dropped
//          entirely (marked usage=N based on a misreading of the IG map);
//          re-added, and both derive scripts now prefer PAT01, falling back
//          to SBR02 for non-compliant trading partners that populate it
//          there instead.
//
// Run: go test ./services/ -v -run TestEDI837RealParserRoundtrip
// ─────────────────────────────────────────────────────────────────────────────

package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

// realSchemaDirSvc mirrors edi/real_schema_integration_test.go's own
// realSchemaDir, adjusted for this package's depth (services/ -> repo root
// is 1 level up).
func realSchemaDirSvc(t *testing.T) string {
	t.Helper()
	return "../edi/schemas/x12_005010"
}

const raw837PSample = "ISA*00*          *00*          *ZZ*SENDER123      *ZZ*RECEIVER456    *260115*1200*^*00501*000000001*0*P*:~" +
	"GS*HC*SENDER123*RECEIVER456*20260115*1200*1*X*005010X222A1~" +
	"ST*837*0001*005010X222A1~" +
	"BHT*0019*00*TX0001*20260115*1200*CH~" +
	"NM1*41*2*ACME BILLING*****46*SUB001~" +
	"NM1*40*2*PAYER1*****46*RECV001~" +
	"HL*1**20*1~" +
	"NM1*85*2*ACME CLINIC*****XX*1234567890~" +
	"HL*2*1*22*0~" +
	"SBR*P*18**GROUPNAME*****CI~" +
	"NM1*IL*1*SMITH*JANE****MI*SUB123~" +
	"DMG*D8*19750322*F~" +
	"NM1*PR*2*PAYER1*****PI*PAYER001~" +
	"CLM*CLM-TEST-001*250***11:B:1*Y*A*Y*Y~" +
	"HI*ABK:Z0000~" +
	"LX*1~" +
	"SV1*HC:99213*150*UN*1***1~" +
	"DTP*472*RD8*20260210-20260212~" +
	"SE*17*0001~" +
	"GE*1*1~" +
	"IEA*1*000000001~"

const raw837ISample = "ISA*00*          *00*          *ZZ*SENDER789      *ZZ*RECEIVER012    *260115*1200*^*00501*000000002*0*P*:~" +
	"GS*HC*SENDER789*RECEIVER012*20260115*1200*1*X*005010X223A2~" +
	"ST*837*0002*005010X223A2~" +
	"BHT*0019*00*TX0002*20260115*1200*CH~" +
	"NM1*41*2*GEN HOSPITAL*****46*SUB002~" +
	"NM1*40*2*PAYER1*****46*RECV001~" +
	"HL*1**20*1~" +
	"NM1*85*2*GENERAL HOSPITAL*****XX*9876543210~" +
	"HL*2*1*22*0~" +
	"SBR*P*18**GROUPNAME*****CI~" +
	"NM1*IL*1*WILSON*MARY****MI*SUB789~" +
	"DMG*D8*19601005*F~" +
	"NM1*PR*2*PAYER1*****PI*PAYER001~" +
	"CLM*CLM-TEST-INPT-001*5000***21:B:1*Y*A*Y*Y~" +
	"CL1*3*1*01~" +
	"HI*ABK:I214~" +
	"LX*1~" +
	"SV2*0250*HC:99223*3000*UN*1~" +
	"DTP*472*RD8*20260210-20260212~" +
	"SE*18*0002~" +
	"GE*1*1~" +
	"IEA*1*000000002~"

func TestEDI837RealParserRoundtrip_837P_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	result, err := edi.ParseTransactionSet(spec, raw837PSample)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P): %v\nraw:\n%s", err, raw837PSample)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context from the real parser's own output, got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	items := claims[0]["item"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 service line item, got %d", len(items))
	}
	item := items[0].(map[string]interface{})

	// The RD8 range (20260210-20260212) must produce servicedPeriod, NOT
	// servicedDate -- the exact real-world shape that caught the bug this
	// test exists to guard against.
	if _, hasDate := item["servicedDate"]; hasDate {
		t.Errorf("item.servicedDate = %v, want absent (RD8 range must map to servicedPeriod instead)", item["servicedDate"])
	}
	period, ok := item["servicedPeriod"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item.servicedPeriod to be a map, got %T", item["servicedPeriod"])
	}
	if period["start"] != "2026-02-10" || period["end"] != "2026-02-12" {
		t.Errorf("item.servicedPeriod = %v, want start=2026-02-10 end=2026-02-12", period)
	}

	bundleJSON := assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
	_ = bundleJSON
}

func TestEDI837RealParserRoundtrip_837I_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	result, err := edi.ParseTransactionSet(spec, raw837ISample)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837I): %v\nraw:\n%s", err, raw837ISample)
	}
	if result.TransactionSet != "837I" {
		t.Fatalf("TransactionSet = %q, want 837I", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837i_claim_contexts", derive837IClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837i_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837i_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context from the real parser's own output, got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837IBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	items := claim["item"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 service line item, got %d", len(items))
	}
	item := items[0].(map[string]interface{})
	if _, hasDate := item["servicedDate"]; hasDate {
		t.Errorf("item.servicedDate = %v, want absent (RD8 range must map to servicedPeriod instead)", item["servicedDate"])
	}
	period, ok := item["servicedPeriod"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item.servicedPeriod to be a map, got %T", item["servicedPeriod"])
	}
	if period["start"] != "2026-02-10" || period["end"] != "2026-02-12" {
		t.Errorf("item.servicedPeriod = %v, want start=2026-02-10 end=2026-02-12", period)
	}

	revenue := item["revenue"].(map[string]interface{})
	revCoding := revenue["coding"].([]interface{})[0].(map[string]interface{})
	if revCoding["code"] != "0250" {
		t.Errorf("item.revenue.coding[0].code = %v, want 0250", revCoding["code"])
	}

	supportingInfo, _ := claim["supportingInfo"].([]interface{})
	if len(supportingInfo) != 3 {
		t.Fatalf("expected 3 supportingInfo entries from CL1, got %d", len(supportingInfo))
	}

	bundleJSON := assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
	_ = bundleJSON
}

// assembleAndValidate837Bundle runs payload.builder (fhir_bundle mode) then
// fhir_validation (strict) against data, failing the test on any unexpected
// validation error (the one known, pre-existing ClaimTypes ValueSet gap is
// excluded, matching every other test in this round). Returns the raw
// assembled Bundle JSON for any caller that wants to inspect it further.
func assembleAndValidate837Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_real_sample_bundle"),
		StepType:  "payload.builder",
		Enabled:   true,
		Config: map[string]interface{}{
			"mode":       "fhir_bundle",
			"fhirBundle": map[string]interface{}{"bundleType": "collection", "resourcePaths": rp},
		},
	}
	pbResult, err := pbExec.Execute(context.Background(), pbStep, data)
	if err != nil {
		t.Fatalf("payload.builder error: %v", err)
	}
	pbOut := svcStepOutput(t, pbResult, "assemble_real_sample_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_real_sample_bundle", pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("payload.builder did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}

	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate FHIR Bundle",
		StepAlias: strPtrSvc("validate_real_sample_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_real_sample_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_real_sample_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_real_sample_bundle")

	errs := asStringSlice(vOut["errors"])
	var unexpected []string
	for _, s := range errs {
		if strings.Contains(s, "invalid-code") && strings.Contains(s, "Claim.type") {
			continue
		}
		unexpected = append(unexpected, s)
	}
	if len(unexpected) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors beyond the known ClaimTypes ValueSet gap:\n%s\nbundle: %s",
			strings.Join(unexpected, "\n"), b)
	}

	return bundleJSON
}

// loadRealSampleSvc mirrors edi/real_sample_diagnostic_test.go's own
// loadRealSample, adjusted for this package's depth.
func loadRealSampleSvc(t *testing.T, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "edi", "testdata", "real_samples", filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	return string(content)
}

// numEquals compares a raw JSON/goja-sourced numeric interface{} (which may
// surface as float64, int, int64, or json.Number depending on which path
// produced it -- a whole-number JS value doesn't always round-trip through
// goja as float64) against a wanted float64, tolerant of that type variance.
func numEquals(v interface{}, want float64) bool {
	switch n := v.(type) {
	case float64:
		return n == want
	case float32:
		return float64(n) == want
	case int:
		return float64(n) == want
	case int64:
		return float64(n) == want
	case json.Number:
		f, err := n.Float64()
		return err == nil && f == want
	default:
		return false
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Real, unedited samples (databricks-industry-solutions/x12-edi-parser,
// test-fixture use only — see this file's own header comment).
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837RealParserRoundtrip_837P_DatabricksSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "databricks_837p_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P databricks sample): %v", err)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	// Both real service lines use DTP*472*RD8 (same start/end date) --
	// proving the RD8 fix against genuine payer data, not just a
	// deliberately-constructed edge case.
	items := claim["item"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 service line items, got %d", len(items))
	}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		if _, hasDate := item["servicedDate"]; hasDate {
			t.Errorf("item[%d].servicedDate = %v, want absent (RD8 must map to servicedPeriod)", i, item["servicedDate"])
		}
		if _, hasPeriod := item["servicedPeriod"]; !hasPeriod {
			t.Errorf("item[%d]: expected servicedPeriod to be present", i)
		}
	}

	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnoses (ABK + ABF), got %d", len(diags))
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

func TestEDI837RealParserRoundtrip_837I_DatabricksSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "databricks_837i_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837I databricks sample): %v", err)
	}
	if result.TransactionSet != "837I" {
		t.Fatalf("TransactionSet = %q, want 837I", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837i_claim_contexts", derive837IClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837i_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837i_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837IBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	// The real sample's HI codes span 5 separate HI segment occurrences
	// (ABK/ABJ diagnosis, DR/DRG [correctly skipped], ABF other-diagnosis x4,
	// BE value codes [correctly skipped]) -- proving allHICodes' own
	// cross-instance flattening against genuine multi-HI-occurrence data.
	// 2 (first HI) + 4 (fourth HI) = 6 real diagnoses; DR and BE are not
	// diagnosis/procedure qualifiers and must not appear.
	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 6 {
		t.Fatalf("expected 6 diagnoses across all HI occurrences, got %d: %+v", len(diags), diags)
	}
	firstDiag := diags[0].(map[string]interface{})
	onAdmission, ok := firstDiag["onAdmission"].(map[string]interface{})
	if !ok {
		t.Fatalf("diagnosis[0]: expected onAdmission to be present (real sample's ABK carries POA indicator Y), got %T", firstDiag["onAdmission"])
	}
	if onAdmission["coding"].([]interface{})[0].(map[string]interface{})["code"] != "Y" {
		t.Errorf("diagnosis[0].onAdmission code = %v, want Y", onAdmission["coding"])
	}

	// No procedure codes exist in this sample's HI data (DR/BE aren't
	// BBR/BR) -- procedure[] must be genuinely empty, not erroring.
	procs, _ := claim["procedure"].([]interface{})
	if len(procs) != 0 {
		t.Errorf("expected 0 procedures (sample's HI has none), got %d", len(procs))
	}

	// Every one of the real sample's 9 service lines has an EMPTY SV202
	// (procedure code) -- revenue-only lines. productOrService is ALWAYS
	// anchored on the revenue code (coding[0]); since no line here has a
	// procedure code, coding[] must stay a single-entry array, never a
	// phantom empty coding[1].
	items := claim["item"].([]interface{})
	if len(items) != 9 {
		t.Fatalf("expected 9 service line items, got %d", len(items))
	}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		pos, ok := item["productOrService"].(map[string]interface{})
		if !ok {
			t.Fatalf("item[%d]: expected productOrService to be present (base FHIR requires it), got %T", i, item["productOrService"])
		}
		codingArr := pos["coding"].([]interface{})
		if len(codingArr) != 1 {
			t.Fatalf("item[%d]: expected exactly 1 productOrService coding (revenue code only, no procedure code in this sample), got %d: %+v", i, len(codingArr), codingArr)
		}
		coding := codingArr[0].(map[string]interface{})
		revenue := item["revenue"].(map[string]interface{})
		revCoding := revenue["coding"].([]interface{})[0].(map[string]interface{})
		if coding["code"] != revCoding["code"] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want it to equal revenue.coding[0].code %v", i, coding["code"], revCoding["code"])
		}
		if coding["system"] != "https://codesystem.x12.org/005010/234" {
			t.Errorf("item[%d]: productOrService.coding[0].system = %v, want the revenue-code system", i, coding["system"])
		}
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

// ─────────────────────────────────────────────────────────────────────────────
// X12.org's own official worked examples (see this file's own header
// comment for the full provenance + the 3 real bugs these caught).
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837RealParserRoundtrip_837P_X12OrgBenKildareSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837p_ben_kildare_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P Ben Kildare sample): %v", err)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	// The patient (TED SMITH) is a DEPENDENT of the subscriber (JANE SMITH) --
	// 2000B's own SBR02 is blank (real, spec-compliant data leaves it blank
	// whenever a 2000C loop exists) and the real relationship code ("19",
	// child) lives on the dependent's own PAT01 in 2000C instead. This is
	// bug #5 from this file's header comment: proves the fix reads PAT01, not
	// the (blank) subscriber SBR02.
	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}
	ctx := claimContexts[0].(map[string]interface{})
	patientInfo := ctx["patientInfo"].(map[string]interface{})
	if patientInfo["relationshipCode"] != "19" {
		t.Errorf("patientInfo.relationshipCode = %v, want 19 (from the dependent's own PAT01, not the blank subscriber SBR02)", patientInfo["relationshipCode"])
	}
	if patientInfo["relationshipFHIR"] != "child" {
		t.Errorf("patientInfo.relationshipFHIR = %v, want child", patientInfo["relationshipFHIR"])
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})

	// Coverage.relationship.coding[0].code is the FHIR-facing surface of the
	// same PAT01 fix -- must read "child", not "other" (the fallback a blank
	// SBR02 would have produced before the fix).
	coverages := message["fhirCoverages"].([]map[string]interface{})
	if len(coverages) != 1 {
		t.Fatalf("expected 1 Coverage resource, got %d", len(coverages))
	}
	relationship := coverages[0]["relationship"].(map[string]interface{})
	relCoding := relationship["coding"].([]interface{})[0].(map[string]interface{})
	if relCoding["code"] != "child" {
		t.Errorf("Coverage.relationship.coding[0].code = %v, want child", relCoding["code"])
	}

	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnoses (BK:0340, BF:V7389), got %d: %+v", len(diags), diags)
	}

	// All 4 real service lines use a plain DTP*472*D8 single date (not a
	// range) -- the first 837P sample to exercise that path; every other
	// 837P sample so far (self-authored + Databricks) only ever used RD8.
	items := claim["item"].([]interface{})
	if len(items) != 4 {
		t.Fatalf("expected 4 service line items, got %d", len(items))
	}
	wantCodes := []string{"99213", "87070", "99214", "86663"}
	wantNet := []float64{40, 15, 35, 10}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		pos := item["productOrService"].(map[string]interface{})
		coding := pos["coding"].([]interface{})[0].(map[string]interface{})
		if coding["code"] != wantCodes[i] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want %v", i, coding["code"], wantCodes[i])
		}
		if coding["system"] != "http://www.ama-assn.org/go/cpt" {
			t.Errorf("item[%d]: productOrService.coding[0].system = %v, want the CPT system", i, coding["system"])
		}
		net := item["net"].(map[string]interface{})
		if !numEquals(net["value"], wantNet[i]) {
			t.Errorf("item[%d]: net.value = %v, want %v", i, net["value"], wantNet[i])
		}
		if _, hasPeriod := item["servicedPeriod"]; hasPeriod {
			t.Errorf("item[%d]: servicedPeriod present, want absent (plain D8, not a range)", i)
		}
		if _, hasDate := item["servicedDate"]; !hasDate {
			t.Errorf("item[%d]: expected servicedDate to be present for a plain D8 date", i)
		}
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

func TestEDI837RealParserRoundtrip_837I_X12OrgJonesHospitalSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837i_jones_hospital_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837I Jones Hospital sample): %v", err)
	}
	if result.TransactionSet != "837I" {
		t.Fatalf("TransactionSet = %q, want 837I", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837i_claim_contexts", derive837IClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837i_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837i_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837IBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	// The real sample uses the ICD-9-CM era qualifiers (BK/BF), not the
	// "AB"-prefixed ICD-10 forms -- proving bug #4's whitelist fix against
	// genuine ICD-9 data, not just a hand-constructed one. 1 (BK:3669) + 2
	// (BF:4019, BF:79431) = 3. The 4 BH (occurrence code) codes and 1 BG
	// (condition code) must NOT appear.
	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 3 {
		t.Fatalf("expected 3 diagnoses across BK/BF qualifiers, got %d: %+v", len(diags), diags)
	}
	procs, _ := claim["procedure"].([]interface{})
	if len(procs) != 0 {
		t.Errorf("expected 0 procedures (sample's HI has no BR/BBR/BQ/BBQ), got %d", len(procs))
	}

	// CL1*3**01 -- admission source (CL1-02) is genuinely blank in this real
	// sample; only admission type + patient status should surface.
	supportingInfo, _ := claim["supportingInfo"].([]interface{})
	if len(supportingInfo) != 2 {
		t.Fatalf("expected 2 supportingInfo entries (CL1-02 admission source is blank in this sample), got %d: %+v", len(supportingInfo), supportingInfo)
	}

	// Both real service lines carry a revenue code AND a CPT code together
	// (SV2*0305*HC:85025*... / SV2*0730*HC:93005*...) -- this is the DIRECT
	// proof of the corrected productOrService design (bug #3 / the user's own
	// "revenue for institutional, CPT for professional" correction): revenue
	// as coding[0], CPT as an ADDITIONAL coding[1], never a conditional
	// either/or.
	items := claim["item"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("expected 2 service line items, got %d", len(items))
	}
	wantRevenue := []string{"0305", "0730"}
	wantCPT := []string{"85025", "93005"}
	wantNet := []float64{13.39, 76.54}
	for i, raw := range items {
		item := raw.(map[string]interface{})

		revenue := item["revenue"].(map[string]interface{})
		revCoding := revenue["coding"].([]interface{})[0].(map[string]interface{})
		if revCoding["code"] != wantRevenue[i] {
			t.Errorf("item[%d]: revenue.coding[0].code = %v, want %v", i, revCoding["code"], wantRevenue[i])
		}

		pos := item["productOrService"].(map[string]interface{})
		codingArr := pos["coding"].([]interface{})
		if len(codingArr) != 2 {
			t.Fatalf("item[%d]: expected exactly 2 productOrService codings (revenue + CPT together), got %d: %+v", i, len(codingArr), codingArr)
		}
		c0 := codingArr[0].(map[string]interface{})
		c1 := codingArr[1].(map[string]interface{})
		if c0["code"] != wantRevenue[i] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want revenue code %v", i, c0["code"], wantRevenue[i])
		}
		if c1["code"] != wantCPT[i] {
			t.Errorf("item[%d]: productOrService.coding[1].code = %v, want CPT code %v", i, c1["code"], wantCPT[i])
		}
		if c1["system"] != "http://www.ama-assn.org/go/cpt" {
			t.Errorf("item[%d]: productOrService.coding[1].system = %v, want the CPT system", i, c1["system"])
		}

		net := item["net"].(map[string]interface{})
		if !numEquals(net["value"], wantNet[i]) {
			t.Errorf("item[%d]: net.value = %v, want %v", i, net["value"], wantNet[i])
		}
		if item["servicedDate"] != "1996-09-11" {
			t.Errorf("item[%d]: servicedDate = %v, want 1996-09-11", i, item["servicedDate"])
		}
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

// ─────────────────────────────────────────────────────────────────────────────
// A broader sweep of X12.org's own official examples, per the user's own
// explicit "test with examples from here too" instruction (x12.org/examples
// lists 12 examples for 837P alone) — chosen for scenario diversity rather
// than exhaustively running all 14: COB (a genuinely untested secondary-payer
// shape), Ambulance (CR1 Ambulance Certification -- a segment never
// previously exercised, plus 2310E/F address-only provider loops), Anesthesia
// (a clean, uncomplicated case directly proving the new Claim.facility fix),
// and Property & Casualty/Automobile Accident (institutional, a third
// independent real sample proving the revenue+CPT dual-coding design, plus
// previously-unproven HI qualifiers PR/BN).
//
// This sweep found ONE further real, genuine gap (not a crash, a silent
// coverage hole): 837P's own extractCareTeam only ever read 2310A
// (Referring)/2310B (Rendering) -- 2310C (Service Facility Location), 2310D
// (Supervising), and 2310E/F (Ambulance Pick-up/Drop-off Location) were
// silently dropped even when populated with a real NPI, despite the ENGINE
// already correctly discriminating all 6 roles. Example 3a's own 2310C
// carried a real NPI that the prior version of extractCareTeam ignored
// entirely. Fixed in two parts:
//   - 2310D (Supervising) added to extractCareTeam as a genuine care-team role.
//   - 2310C (837P) / 2310E (837I) -- NOT care-team members, they're a PLACE --
//     mapped instead to Claim.facility (base FHIR R4's own dedicated 0..1
//     Reference field for exactly this concept, confirmed against this
//     project's own decompressed Claim.gz schema), via the same
//     logical/identifier-only reference pattern careTeam[].provider already
//     uses.
// 2310E/F (Ambulance Pick-up/Drop-off Location) remain a named, deliberate
// gap -- confirmed via this round's own Ambulance sample that these carry NO
// name or identifier at all (just a bare address), so there is nothing to
// build even a logical reference from; base FHIR Claim has no dedicated
// "service location" field beyond the single `facility`, which is already
// spoken for by 2310C.
// ─────────────────────────────────────────────────────────────────────────────

func TestEDI837RealParserRoundtrip_837P_X12OrgExample3aCOBSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837p_example3a_cob_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P Example 3a COB sample): %v", err)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}
	ctx := claimContexts[0].(map[string]interface{})
	patientInfo := ctx["patientInfo"].(map[string]interface{})
	if patientInfo["relationshipFHIR"] != "child" {
		t.Errorf("patientInfo.relationshipFHIR = %v, want child (PAT*19 in 2000C)", patientInfo["relationshipFHIR"])
	}
	claimMap := ctx["claim"].(map[string]interface{})
	if claimMap["facilityNpi"] != "1581234567" {
		t.Errorf("claim.facilityNpi = %v, want 1581234567 (2310C Service Facility Location)", claimMap["facilityNpi"])
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	// facility.identifier -- the direct FHIR-facing proof of the new
	// Claim.facility fix.
	facility, ok := claim["facility"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected claim.facility to be present, got %T", claim["facility"])
	}
	facilityID := facility["identifier"].(map[string]interface{})
	if facilityID["value"] != "1581234567" {
		t.Errorf("facility.identifier.value = %v, want 1581234567", facilityID["value"])
	}

	// careTeam -- only 2310B (rendering) is present in this sample; 2310C
	// (now correctly routed to facility, not careTeam) must NOT also appear
	// here as a phantom entry.
	careTeam, _ := claim["careTeam"].([]interface{})
	if len(careTeam) != 1 {
		t.Fatalf("expected 1 careTeam entry (rendering only), got %d: %+v", len(careTeam), careTeam)
	}
	ct0 := careTeam[0].(map[string]interface{})
	role := ct0["role"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if role["code"] != "rendering" {
		t.Errorf("careTeam[0].role = %v, want rendering", role["code"])
	}

	// HI*BK:4779*BF:2724*BF:2780*BF:53081 -- 4 diagnoses in one HI occurrence
	// (837P doesn't filter by qualifier, unlike 837I).
	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 4 {
		t.Fatalf("expected 4 diagnoses, got %d: %+v", len(diags), diags)
	}

	// 3 service lines, diagnosisCodePointer composites of 4 and 2 pointers --
	// proves the C004 composite parses regardless of how many of its 4
	// possible positions are populated.
	items := claim["item"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("expected 3 service line items, got %d", len(items))
	}
	wantCodes := []string{"99213", "90782", "J3301"}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		coding := item["productOrService"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
		if coding["code"] != wantCodes[i] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want %v", i, coding["code"], wantCodes[i])
		}
	}

	// 2320/2330A/2330B (secondary payer COB loop, SBR*S*01...NM1*IL...NM1*PR)
	// must not have broken parsing of anything after it -- already implicitly
	// proven by the assertions above succeeding, but the bundle must also
	// still validate cleanly.
	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

func TestEDI837RealParserRoundtrip_837P_X12OrgExample5AmbulanceSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837p_example5_ambulance_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P Example 5 Ambulance sample): %v", err)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	// This is the sample proving CR1 (Ambulance Certification), a segment
	// declared in the schema but never previously exercised by any real
	// sample in this test suite, does not derail subsequent HI parsing --
	// asserted implicitly by the diagnosis count below succeeding at all.
	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}
	ctx := claimContexts[0].(map[string]interface{})
	claimMap := ctx["claim"].(map[string]interface{})
	if _, hasFacility := claimMap["facilityNpi"]; hasFacility {
		t.Errorf("claim.facilityNpi = %v, want absent (2310E/F in this sample carry no name/ID, only an address)", claimMap["facilityNpi"])
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	if _, hasFacility := claim["facility"]; hasFacility {
		t.Errorf("claim.facility = %v, want absent", claim["facility"])
	}

	// No 2310A/B/D present -- 2310E/F (ambulance pickup/dropoff, address-only)
	// must NOT produce phantom careTeam entries either.
	careTeam, _ := claim["careTeam"].([]interface{})
	if len(careTeam) != 0 {
		t.Errorf("expected 0 careTeam entries (only address-only 2310E/F present), got %d: %+v", len(careTeam), careTeam)
	}

	// HI*BK:8628*BF:E8888*BF:9592*BF:8540 -- proves CR1 didn't swallow or
	// misalign the HI segment that immediately follows it.
	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 4 {
		t.Fatalf("expected 4 diagnoses (proves CR1 didn't derail HI parsing), got %d: %+v", len(diags), diags)
	}

	// 4 service lines with a 3-part procedure code composite (HC:A0427:RH --
	// qualifier + code + modifier) and non-1 quantities (21 on line 2).
	items := claim["item"].([]interface{})
	if len(items) != 4 {
		t.Fatalf("expected 4 service line items, got %d", len(items))
	}
	wantCodes := []string{"A0427", "A0425", "A0422", "A0382"}
	wantQty := []float64{1, 21, 1, 1}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		coding := item["productOrService"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
		if coding["code"] != wantCodes[i] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want %v (modifier 'RH' correctly not captured as part of the code)", i, coding["code"], wantCodes[i])
		}
		qty := item["quantity"].(map[string]interface{})
		if !numEquals(qty["value"], wantQty[i]) {
			t.Errorf("item[%d]: quantity.value = %v, want %v", i, qty["value"], wantQty[i])
		}
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

func TestEDI837RealParserRoundtrip_837P_X12OrgExample9AnesthesiaSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837p_example9_anesthesia_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837P Example 9 Anesthesia sample): %v", err)
	}
	if result.TransactionSet != "837P" {
		t.Fatalf("TransactionSet = %q, want 837P", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837p_claim_contexts", derive837PClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837p_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837p_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}
	ctx := claimContexts[0].(map[string]interface{})
	claimMap := ctx["claim"].(map[string]interface{})
	if claimMap["facilityNpi"] != "432198765" {
		t.Errorf("claim.facilityNpi = %v, want 432198765 (2310C, PROVIDER OP HOSP)", claimMap["facilityNpi"])
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837PBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837PBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	facility, ok := claim["facility"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected claim.facility to be present, got %T", claim["facility"])
	}
	facilityID := facility["identifier"].(map[string]interface{})
	if facilityID["value"] != "432198765" {
		t.Errorf("facility.identifier.value = %v, want 432198765", facilityID["value"])
	}
	if facilityID["system"] != "http://hl7.org/fhir/sid/us-npi" {
		t.Errorf("facility.identifier.system = %v, want the NPI system", facilityID["system"])
	}

	careTeam, _ := claim["careTeam"].([]interface{})
	if len(careTeam) != 1 {
		t.Fatalf("expected 1 careTeam entry (rendering, 2310B), got %d: %+v", len(careTeam), careTeam)
	}

	// SV1*HC:00142:QK:QS:P1*827*MJ*61***1 -- a 3-modifier procedure code
	// composite (QK/QS/P1) and a non-"UN" unit of measurement (MJ, anesthesia
	// minutes) -- proves neither derails procedureCode/quantity extraction.
	items := claim["item"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("expected 1 service line item, got %d", len(items))
	}
	item := items[0].(map[string]interface{})
	coding := item["productOrService"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if coding["code"] != "00142" {
		t.Errorf("item[0]: productOrService.coding[0].code = %v, want 00142 (modifiers QK/QS/P1 correctly not captured as part of the code)", coding["code"])
	}
	qty := item["quantity"].(map[string]interface{})
	if !numEquals(qty["value"], 61) {
		t.Errorf("item[0]: quantity.value = %v, want 61 (anesthesia minutes, unit MJ)", qty["value"])
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}

func TestEDI837RealParserRoundtrip_837I_X12OrgExample2aAutoAccidentSample_ProducesValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	loader, err := edi.NewX12SchemaLoader(realSchemaDirSvc(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	raw := loadRealSampleSvc(t, "x12org_837i_example2a_auto_accident_sample.txt")
	result, err := edi.ParseTransactionSet(spec, raw)
	if err != nil {
		t.Fatalf("ParseTransactionSet(837I Example 2a Auto Accident sample): %v", err)
	}
	if result.TransactionSet != "837I" {
		t.Fatalf("TransactionSet = %q, want 837I", result.TransactionSet)
	}

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": result.TransactionSet,
			"loops":          result.Loops,
			"header":         result.Header,
			"interchange":    result.Interchange,
		},
	}

	deriveResult := runScriptSvc(t, "derive_837i_claim_contexts", derive837IClaimContextsScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_837i_claim_contexts")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_837i_claim_contexts", deriveOut)
	if cc, ok := deriveOut["_claim_contexts"]; ok {
		data["_claim_contexts"] = cc
	}
	if bp, ok := deriveOut["_billing_provider"]; ok {
		data["_billing_provider"] = bp
	}

	// PAT*21 -- an X12 relationship code this codebase's relationshipToFHIR
	// map does NOT recognize (only 18/01/19/20 are mapped) -- proves the
	// fallback-to-"other" path works correctly on genuine real data rather
	// than crashing or silently mis-mapping to some other relationship.
	claimContexts, _ := data["_claim_contexts"].([]interface{})
	if len(claimContexts) != 1 {
		t.Fatalf("expected 1 claim context, got %d: %+v", len(claimContexts), claimContexts)
	}
	ctx := claimContexts[0].(map[string]interface{})
	patientInfo := ctx["patientInfo"].(map[string]interface{})
	if patientInfo["relationshipFHIR"] != "other" {
		t.Errorf("patientInfo.relationshipFHIR = %v, want other (PAT*21 is not in relationshipToFHIR's map)", patientInfo["relationshipFHIR"])
	}
	// The patient (RON MEXICO, NM1*QC*1*MEXICO*RON) carries no identification
	// code at all -- memberId must fall back to subscriberId-DEP1.
	if patientInfo["memberId"] != "B999777791G-DEP1" {
		t.Errorf("patientInfo.memberId = %v, want B999777791G-DEP1 (patient NM1 has no ID, falls back to subscriber+DEP suffix)", patientInfo["memberId"])
	}

	data = runFHIRBuild(t, "build_organization_fhir", organization837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage837IBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim837IBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims := message["fhirClaims"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 Claim resource, got %d", len(claims))
	}
	claim := claims[0]

	if _, hasFacility := claim["facility"]; hasFacility {
		t.Errorf("claim.facility = %v, want absent (no 2310E in this sample)", claim["facility"])
	}

	careTeam, _ := claim["careTeam"].([]interface{})
	if len(careTeam) != 1 {
		t.Fatalf("expected 1 careTeam entry (attending, 2310A), got %d: %+v", len(careTeam), careTeam)
	}
	ctRole := careTeam[0].(map[string]interface{})["role"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if ctRole["code"] != "attending" {
		t.Errorf("careTeam[0].role = %v, want attending", ctRole["code"])
	}

	// HI*BK:8842 / HI*PR:8842 / HI*BN:E9750*BN:E9860 -- proves the PR and BN
	// diagnosis qualifiers (added speculatively to DIAGNOSIS_HI_QUALIFIERS
	// without a real sample to confirm) are genuinely correct against real
	// X12.org data, across 3 separate HI occurrences.
	diags := claim["diagnosis"].([]interface{})
	if len(diags) != 4 {
		t.Fatalf("expected 4 diagnoses (BK+PR+2xBN across 3 HI occurrences), got %d: %+v", len(diags), diags)
	}
	procs, _ := claim["procedure"].([]interface{})
	if len(procs) != 0 {
		t.Errorf("expected 0 procedures, got %d", len(procs))
	}

	// CL1*3*7*1 -- all three admission-info elements present, unlike the
	// Jones Hospital sample where admission source was blank.
	supportingInfo, _ := claim["supportingInfo"].([]interface{})
	if len(supportingInfo) != 3 {
		t.Fatalf("expected 3 supportingInfo entries (CL1 fully populated in this sample), got %d: %+v", len(supportingInfo), supportingInfo)
	}

	// 4 service lines, every one carrying BOTH a revenue code and a CPT code
	// together -- a THIRD independent real institutional sample confirming
	// the revenue-primary/CPT-secondary dual-coding design.
	items := claim["item"].([]interface{})
	if len(items) != 4 {
		t.Fatalf("expected 4 service line items, got %d", len(items))
	}
	wantRevenue := []string{"0450", "0360", "0312", "0360"}
	wantCPT := []string{"98765", "26591", "86225", "99283"}
	wantQty := []float64{1, 1, 2, 1}
	for i, raw := range items {
		item := raw.(map[string]interface{})
		pos := item["productOrService"].(map[string]interface{})
		codingArr := pos["coding"].([]interface{})
		if len(codingArr) != 2 {
			t.Fatalf("item[%d]: expected 2 productOrService codings, got %d: %+v", i, len(codingArr), codingArr)
		}
		if codingArr[0].(map[string]interface{})["code"] != wantRevenue[i] {
			t.Errorf("item[%d]: productOrService.coding[0].code = %v, want revenue %v", i, codingArr[0].(map[string]interface{})["code"], wantRevenue[i])
		}
		if codingArr[1].(map[string]interface{})["code"] != wantCPT[i] {
			t.Errorf("item[%d]: productOrService.coding[1].code = %v, want CPT %v", i, codingArr[1].(map[string]interface{})["code"], wantCPT[i])
		}
		qty := item["quantity"].(map[string]interface{})
		if !numEquals(qty["value"], wantQty[i]) {
			t.Errorf("item[%d]: quantity.value = %v, want %v", i, qty["value"], wantQty[i])
		}
	}

	assembleAndValidate837Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages", "message.fhirClaims")
}
