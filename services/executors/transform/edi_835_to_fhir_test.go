// services/executors/transform/edi_835_to_fhir_test.go
//
// Proves the 835 -> FHIR mapping end to end against REAL, unedited 835
// samples (edi/testdata/real_samples/) — not synthetic fixtures — using the
// real pipeline mechanism a template migration can actually run:
// edi.parse -> enrichment.script (derive per-claim row context) ->
// fhir.build(ExplanationOfBenefit, rowsPath) [ONE call, one resource per
// claim] -> fhir.build(PaymentReconciliation) -> payload.builder(fhir_bundle)
// -> fhir_validation(strict). This supersedes an earlier version of this
// file that hand-looped fhir.build once per claim in Go test code (the only
// option before fhir.build gained its own rowsPath mechanism during the
// 837P/837I work) — a real pipeline template has no way to do that looping
// itself (control.loop's childStepIds can only hold real DB-assigned step
// UUIDs, which don't exist at template-authoring time; see
// V231__EDI_835_To_FHIR_OOB_Pipeline_Template.sql's own comment).
//
// Why a derive script is needed at all: fhir.build's rowsPath field
// resolution is ROW-ONLY (see fhir_build_executor.go's resolveRawValue,
// which is handed only the current row, never inputData, for
// sourcePath/fallbackPaths resolution) — a claim row (a 2100 loop instance)
// has no reachable path back up to the interchange header, so the 2 header
// values every claim needs (BPR16 payment date, 1000A payer name) are
// copied onto EACH row by the derive script below, once, before fhir.build
// ever sees them. CLP/NM1/SVC/CAS values are extracted into semantic,
// already-snake_case field names rather than passed through as raw segment
// sub-objects — required, not a style choice: this script's return value
// only reaches the later fhir.build step via
// "steps.<alias>.step_output.<key>" (BaseExecutor.SetStepOutputWithDetails/
// executeStepWithContext), and in a REAL pipeline run that snapshot is
// always passed through models.OutputNormalizer.NormalizeStepOutput, which
// snake_cases every key it doesn't already recognize as snake_case —
// confirmed directly in code, not assumed, and already the established
// precedent from the 837P/837I derive scripts
// (services/edi_837p_fhir_builder_test.go).
//
// Field-mapping scope, named rather than silently guessed at (per this
// project's "build what we can with accuracy and evidence" standard):
//   - CAS-derived adjudication entries put the literal CARC code on
//     category.coding[0].code, system = the verified X12 CARC CodeSystem URL
//     (https://x12.org/codes/claim-adjustment-reason-codes) -- NOT a
//     fabricated crosswalk to FHIR's small base adjudication category
//     value set. That value set's binding strength is "example" (verified
//     against hl7.org/fhir/R4/explanationofbenefit-definitions.html), so a
//     CARC code as the category is spec-legal.
//   - EOB.type is hardcoded "professional" — an 835 alone doesn't reliably
//     distinguish claim types without deeper facility-type-code knowledge
//     this pass didn't verify; named here rather than silently assumed.
//   - EOB.item.sequence IS now mapped, closing a previously-named gap: this
//     round added a genuinely reusable "_rowIndex" primitive to
//     fhir_build_executor.go (every row from a rowsPath or repeatingGroup's
//     own RowsPath carries its 1-based position under that key), not an
//     835-specific hack.
//   - EOB.provider and EOB.insurance are still NOT mapped. provider was
//     attempted via NM1[entityIdentifierCode=82] (the X12-documented
//     "Rendering Provider" role code) but neither real sample actually
//     carries an "82" NM1 occurrence at the claim level — a real,
//     evidence-based finding from running against real data, not a
//     hardcoded assumption that happened to be wrong: this mapping is
//     directly correct, the source data for it is simply often absent.
//     insurance (linking to a Coverage resource) was never attempted at all
//     — no Coverage resource is built in this pass. Both are real, expected
//     strict-validation errors, asserted as the ONLY errors below rather
//     than silently ignored.
//   - EOB.created / PaymentReconciliation.created / PaymentReconciliation.
//     paymentDate all use header.BPR.paymentEffectiveDate (BPR16) via the
//     "x12_date_to_fhir_date" transform (services/cda_fhir/
//     declarative_transform_registry.go).
//   - PLB (provider-level balance) is deliberately NOT mapped this pass —
//     none of the 3 real 835 samples in this repo carries a PLB segment;
//     this project's own established discipline is to not fabricate
//     fixtures where real ones don't exist.
package transform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/enrichment"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

// carcSystemURL is the verified X12 Claim Adjustment Reason Codes CodeSystem
// canonical URL (terminology.hl7.org/4.0.0/CodeSystem-X12ClaimAdjustmentReasonCodes.html,
// v1.0.0, active 2022-05-26) — fetched directly during Phase 5 planning, not
// recalled from training data.
const carcSystemURL = "https://x12.org/codes/claim-adjustment-reason-codes"

// derive835StepAlias is the step_alias a real migration must give the
// "Derive 835 Claim Header Context" enrichment.script step — eobBuildConfig's
// own rowsPath addresses this script's output via
// "steps.<alias>.step_output.claim_rows", so the alias here and the alias a
// real migration configures on that step MUST match exactly (a real,
// previously-found migration-authoring gotcha for this exact addressing
// scheme — see [[project_edi_phase3_eligibility]]'s own "step_name's
// normalized form must match step_alias" finding).
const derive835StepAlias = "derive_835_claim_context"

// derive835ClaimContextScript reshapes the parsed 835 document into one flat
// claim_rows[] array, each row carrying everything the EOB build below needs
// — including the 2 header-level values (header_payment_date, payer_name)
// copied onto every row, since rowsPath's own field resolution can't reach
// back up to inputData (see this file's own package doc comment).
func derive835ClaimContextScript() string {
	return `
// A prior step's plain top-level output field (edi.parse's own "parsedEDI",
// per its outputField config) is NOT exposed to a later step's own "input"
// at the top level -- the real pipeline engine nests it under
// input.message.parsedEDI instead (confirmed by the 837P/837I derive
// scripts' own real Test-Pipeline-run finding, which applies here too).
// Prefer that real shape; fall back to the bare top-level key for
// Go-level test harnesses that construct { parsedEDI: ... } directly
// without a "message" wrapper (as edi_835_to_fhir_test.go does).
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};
var header = parsed.header || {};

function arr(v) {
  if (!v) return [];
  return Array.isArray(v) ? v : [v];
}

function byEntityIdentifierCode(nm1, code) {
  var list = arr(nm1);
  for (var i = 0; i < list.length; i++) {
    if (list[i].entityIdentifierCode === code) return list[i];
  }
  return {};
}

var headerPaymentDate = (header.BPR && header.BPR.paymentEffectiveDate) || "";
var payerName = ((loops["1000A"] || {}).N1 || {}).name || "";

// CAS is maxUse ">1" within a service line (a line can carry more than one
// CAS occurrence, e.g. one per claim-adjustment-group-code), each with its
// own up-to-6 reason/amount/quantity trios in its own "adjustments" repeat
// group (edi/schemas/x12_005010/segments/CAS.json). Flatten every
// occurrence's trios into one combined list here, in JS, rather than
// relying on fhir.build's own "CAS[*].adjustments" wildcard-flatten path —
// this script now owns all the row reshaping, matching the 837P/837I
// precedent of doing this flattening once, in one place.
function extractAdjustments(casList) {
  var instances = arr(casList);
  var out = [];
  for (var i = 0; i < instances.length; i++) {
    var trios = instances[i].adjustments || [];
    for (var j = 0; j < trios.length; j++) {
      if (!trios[j].reasonCode) continue;
      out.push({ reason_code: trios[j].reasonCode, amount: trios[j].amount || "0" });
    }
  }
  return out;
}

function extractServiceLines(claimRow) {
  var lines = arr((claimRow.loops || {})["2110"]);
  var out = [];
  for (var i = 0; i < lines.length; i++) {
    var svc = lines[i].SVC || {};
    var proc = svc.procedureCode || {};
    out.push({
      procedure_code: proc.code || "",
      charge_amount: svc.chargeAmount || "0",
      paid_amount: svc.paidAmount || "0",
      adjustments: extractAdjustments(lines[i].CAS)
    });
  }
  return out;
}

// 2000 (Header Number) is maxUse ">1" -- every real sample checked into this
// repo only ever carries one, but the spec allows more (e.g. a
// clearinghouse batching multiple LX groups into one interchange), so every
// group is walked rather than assumed away, mirroring the same
// walk-every-occurrence fix already made for 837's own 2000A billing
// provider loop.
var claimRows = [];
var headerGroups = arr(loops["2000"]);
for (var g = 0; g < headerGroups.length; g++) {
  var claims = arr((headerGroups[g].loops || {})["2100"]);
  for (var c = 0; c < claims.length; c++) {
    var claimRow = claims[c];
    var clp = claimRow.CLP || {};
    var patientNM1 = byEntityIdentifierCode(claimRow.NM1, "QC");
    // Rendering Provider (role 82) -- named, evidence-based gap: neither
    // real sample in this repo carries an "82" NM1 occurrence at the claim
    // level, so this resolves empty today, but the mapping itself is
    // correct per the X12 835 IG and is kept rather than removed.
    var providerNM1 = byEntityIdentifierCode(claimRow.NM1, "82");
    claimRows.push({
      patient_control_number: clp.patientControlNumber || "",
      claim_payment_amount: clp.claimPaymentAmount || "0",
      patient_name: patientNM1.nameLastOrOrganizationName || "",
      provider_name: providerNM1.nameLastOrOrganizationName || "",
      header_payment_date: headerPaymentDate,
      payer_name: payerName,
      service_lines: extractServiceLines(claimRow)
    });
  }
}

return { claim_rows: claimRows };
`
}

// eobBuildConfig is the fhir.build config for EVERY ExplanationOfBenefit in
// the file, built in ONE call via rowsPath — one resource per entry in the
// derive script's own claim_rows[] output. Every sourcePath below is
// row-scoped (resolves against one claim_rows[] entry), matching
// fhir.build's row-only field resolution.
func eobBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"outputField":  "explanationOfBenefits",
		"rowsPath":     "steps." + derive835StepAlias + ".step_output.claim_rows",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patient_control_number"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
			map[string]interface{}{"targetPath": "outcome", "literalValue": "complete"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "header_payment_date", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "patient.display", "sourcePath": "patient_name"},
			map[string]interface{}{"targetPath": "provider.display", "sourcePath": "provider_name"},
			map[string]interface{}{"targetPath": "insurer.display", "sourcePath": "payer_name"},
			map[string]interface{}{"targetPath": "payment.amount.value", "sourcePath": "claim_payment_amount", "transform": "cda_decimal_string_to_number"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "service_lines",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "_rowIndex", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].system", "literalValue": "https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].code", "sourcePath": "procedure_code"},
					map[string]interface{}{"targetPath": "adjudication[0].category.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/adjudication"},
					map[string]interface{}{"targetPath": "adjudication[0].category.coding[0].code", "literalValue": "submitted"},
					map[string]interface{}{"targetPath": "adjudication[0].amount.value", "sourcePath": "charge_amount", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "adjudication[1].category.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/adjudication"},
					map[string]interface{}{"targetPath": "adjudication[1].category.coding[0].code", "literalValue": "benefit"},
					map[string]interface{}{"targetPath": "adjudication[1].amount.value", "sourcePath": "paid_amount", "transform": "cda_decimal_string_to_number"},
				},
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "adjudication",
						"rowsPath":   "adjustments",
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "category.coding[0].system", "literalValue": carcSystemURL},
							map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "reason_code"},
							map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
						},
					},
				},
			},
		},
	}
}

// paymentReconciliationBuildConfig is the fhir.build config for the ONE
// PaymentReconciliation per interchange. UNCHANGED from before this round —
// it reads parsedEDI directly (never through a derive script's own
// step_output), so none of the row-scoping/snake-casing rules above apply
// to it. response.reference is keyed off the same CLP.patientControlNumber
// value the new EOB config uses for its own "id" (patient_control_number),
// so payload.builder's fullUrl-rewrite still matches each EOB correctly.
func paymentReconciliationBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "PaymentReconciliation",
		"outputField":  "paymentReconciliation",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "parsedEDI.header.TRN.checkOrEFTTraceNumber"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "outcome", "literalValue": "complete"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "paymentDate", "sourcePath": "parsedEDI.header.BPR.paymentEffectiveDate", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "paymentAmount.value", "sourcePath": "parsedEDI.header.BPR.totalActualProviderPaymentAmount", "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "paymentIssuer.display", "sourcePath": "parsedEDI.loops.1000A.N1.name"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "detail",
				"rowsPath":   "parsedEDI.loops.2000[0].loops.2100",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "amount.value", "sourcePath": "CLP.claimPaymentAmount", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{
						"targetPath": "response.reference", "sourcePath": "CLP.patientControlNumber",
						"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "ExplanationOfBenefit/"},
					},
				},
			},
		},
	}
}

// runScriptEDI runs the real enrichment.script executor (same executor a
// real pipeline uses), mirroring services/pas_fhir_builder_test.go's own
// runScriptSvc — reimplemented locally since that helper lives in package
// services, not this package (transform).
func runScriptEDI(t *testing.T, alias, script string, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := enrichment.NewScriptEnrichmentExecutor()
	step := &models.TransformationStep{
		StepName: alias, StepAlias: strPtrEDI(alias), StepType: "enrichment.script", Enabled: true,
		Config: map[string]interface{}{"script": script},
	}
	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("[%s] script error: %v", alias, err)
	}
	return result
}

// ediStepOutput extracts a script's own raw "_stepOutput" — mirrors
// pas_fhir_builder_test.go's svcStepOutput.
func ediStepOutput(t *testing.T, result map[string]interface{}, label string) map[string]interface{} {
	t.Helper()
	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("[%s] _stepOutput missing or not a map", label)
	}
	return out
}

// injectEDIStepOutput stashes a script's own output under
// steps.<alias>.step_output — the same reference path a real fhir.build
// rowsPath config addresses (see eobBuildConfig's own rowsPath). Mirrors
// pas_fhir_builder_test.go's svcInjectStepOutput.
func injectEDIStepOutput(data map[string]interface{}, alias string, output map[string]interface{}) {
	steps, ok := data["steps"].(map[string]interface{})
	if !ok {
		steps = map[string]interface{}{}
		data["steps"] = steps
	}
	steps[alias] = map[string]interface{}{"step_output": output}
}

func strPtrEDI(s string) *string { return &s }

// build835FHIRBundle chains edi.parse -> enrichment.script (derive) ->
// fhir.build(EOB, rowsPath) -> fhir.build(PaymentReconciliation) ->
// payload.builder(fhir_bundle) against one real sample file, returning the
// assembled Bundle as a map plus the number of claims found (for
// assertions) — sourced from the derive script's own claim_rows[] output,
// the single source of truth for how many EOBs get built.
func build835FHIRBundle(t *testing.T, sampleFile string) (map[string]interface{}, int) {
	t.Helper()
	initFHIRRegistry(t)

	parseExec := newTestEDIParseExecutor(t)
	raw := readRealSample(t, sampleFile)
	parseStep := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedEDI"},
	}
	parsedOutput, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("edi.parse Execute failed: %v", err)
	}
	if _, ok := parsedOutput["parsedEDI"].(map[string]interface{}); !ok {
		t.Fatalf("expected parsedEDI to be a map, got %T", parsedOutput["parsedEDI"])
	}

	scriptResult := runScriptEDI(t, derive835StepAlias, derive835ClaimContextScript(), parsedOutput)
	deriveOut := ediStepOutput(t, scriptResult, derive835StepAlias)
	injectEDIStepOutput(scriptResult, derive835StepAlias, deriveOut)

	claimRows, ok := deriveOut["claim_rows"].([]interface{})
	if !ok {
		t.Fatalf("expected claim_rows to be a []interface{}, got %T", deriveOut["claim_rows"])
	}

	fhirBuild := NewFHIRBuildExecutor()
	eobStep := &models.TransformationStep{
		StepName: "Build EOB", StepType: "fhir.build", Enabled: true, Config: eobBuildConfig(),
	}
	eobOut, err := fhirBuild.Execute(context.Background(), eobStep, scriptResult)
	if err != nil {
		t.Fatalf("fhir.build(EOB) Execute failed: %v", err)
	}
	eobs, ok := eobOut["explanationOfBenefits"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected explanationOfBenefits to be a []map[string]interface{}, got %T", eobOut["explanationOfBenefits"])
	}
	if len(eobs) != len(claimRows) {
		t.Fatalf("expected %d EOB(s) (one per claim_rows entry), got %d", len(claimRows), len(eobs))
	}

	prStep := &models.TransformationStep{
		StepName: "Build PaymentReconciliation", StepType: "fhir.build", Enabled: true, Config: paymentReconciliationBuildConfig(),
	}
	prOut, err := fhirBuild.Execute(context.Background(), prStep, eobOut)
	if err != nil {
		t.Fatalf("fhir.build(PaymentReconciliation) Execute failed: %v", err)
	}
	paymentReconciliation, ok := prOut["paymentReconciliation"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected paymentReconciliation to be a map, got %T", prOut["paymentReconciliation"])
	}

	payloadBuilder := payload.NewPayloadBuilderExecutor(nil)
	bundleStep := &models.TransformationStep{
		StepName: "Assemble 835 FHIR Bundle", StepType: "payload.builder", Enabled: true,
		Config: map[string]interface{}{
			"mode": "fhir_bundle",
			"fhirBundle": map[string]interface{}{
				"bundleType":    "collection",
				"resourcePaths": []interface{}{"explanationOfBenefits", "paymentReconciliation"},
			},
		},
	}
	bundleOut, err := payloadBuilder.Execute(context.Background(), bundleStep, map[string]interface{}{
		"explanationOfBenefits": eobs, "paymentReconciliation": paymentReconciliation,
	})
	if err != nil {
		t.Fatalf("payload.builder(fhir_bundle) Execute failed: %v", err)
	}
	payloadStr, ok := bundleOut["payload"].(string)
	if !ok || payloadStr == "" {
		t.Fatalf("expected a non-empty payload string, got %v (%T)", bundleOut["payload"], bundleOut["payload"])
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &bundle); err != nil {
		t.Fatalf("failed to unmarshal assembled bundle: %v", err)
	}
	return bundle, len(claimRows)
}

// runStrictValidation runs the real fhir_validation executor (strict level)
// against bundle, returning its valid/errors/warnings — the actual FHIR R4
// validator, not a hand-rolled check.
func runStrictValidation(t *testing.T, bundle map[string]interface{}) (bool, []string, []string) {
	t.Helper()
	initFHIRRegistry(t)
	validator := validation.NewFHIRValidationExecutor()
	step := &models.TransformationStep{
		StepName: "Validate 835 Bundle", StepType: "fhir_validation", Enabled: true,
		Config: map[string]interface{}{"validation_level": "strict"},
	}
	out, err := validator.Execute(context.Background(), step, map[string]interface{}{"fhirBundle": bundle})
	if err != nil {
		t.Fatalf("fhir_validation Execute failed: %v", err)
	}
	stepOutput, ok := out["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be a map, got %T", out["_stepOutput"])
	}
	valid, _ := stepOutput["valid"].(bool)
	errs := toStringSlice(stepOutput["errors"])
	warns := toStringSlice(stepOutput["warnings"])
	return valid, errs, warns
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]string)
	if ok {
		return arr
	}
	if ifaceArr, ok := v.([]interface{}); ok {
		out := make([]string, 0, len(ifaceArr))
		for _, e := range ifaceArr {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// assertOnlyNamedEDI835ValidationGaps fails the test if any strict-validation
// error is NOT one of the 2 real, evidence-based, named gaps this pass
// leaves open (EOB.provider, EOB.insurance — see this file's own package doc
// comment). Mirrors the 837 test suite's own "one known, pre-existing,
// out-of-scope gap explicitly excluded, not silently tolerated for anything
// else" precedent — surprises here should fail loudly, not be swallowed.
func assertOnlyNamedEDI835ValidationGaps(t *testing.T, errs []string) {
	t.Helper()
	allowedSubstrings := []string{"provider", "insurance"}
	for _, e := range errs {
		known := false
		lowerE := strings.ToLower(e)
		for _, allowed := range allowedSubstrings {
			if strings.Contains(lowerE, allowed) {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("unexpected strict-validation error (not one of the named provider/insurance gaps): %s", e)
		}
	}
}

// TestEDI835ToFHIR_BlueCrossNC_SingleClaim_BuildsCorrectBundleShape exercises
// the simplest real sample (1 claim, per this file's own earlier CLP-count
// check against the real fixture) — proving the mechanical shape (entry
// count, cross-references) is right before layering on the multi-claim case.
func TestEDI835ToFHIR_BlueCrossNC_SingleClaim_BuildsCorrectBundleShape(t *testing.T) {
	bundle, claimCount := build835FHIRBundle(t, "blue_cross_nc_sample.txt")
	if claimCount != 1 {
		t.Fatalf("expected 1 claim in blue_cross_nc_sample.txt, got %d", claimCount)
	}

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 bundle entries (1 EOB + 1 PaymentReconciliation), got %v", bundle["entry"])
	}

	eobEntry := entries[0].(map[string]interface{})
	eobResource := eobEntry["resource"].(map[string]interface{})
	if eobResource["resourceType"] != "ExplanationOfBenefit" {
		t.Errorf("entry[0].resource.resourceType = %v, want ExplanationOfBenefit", eobResource["resourceType"])
	}
	items, ok := eobResource["item"].([]interface{})
	if !ok || len(items) == 0 {
		t.Fatalf("expected EOB.item to be a non-empty array, got %v", eobResource["item"])
	}
	firstItem := items[0].(map[string]interface{})
	if seq := firstItem["sequence"]; seq != float64(1) {
		t.Errorf("EOB.item[0].sequence = %v (%T), want 1", seq, seq)
	}
	adjudication, ok := firstItem["adjudication"].([]interface{})
	if !ok || len(adjudication) < 2 {
		t.Fatalf("expected EOB.item[0].adjudication to have at least the 2 fixed entries, got %v", firstItem["adjudication"])
	}
	if cat := adjudication[0].(map[string]interface{})["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; cat != "submitted" {
		t.Errorf("adjudication[0] category code = %v, want submitted", cat)
	}
	if len(items) > 1 {
		secondItem := items[1].(map[string]interface{})
		if seq := secondItem["sequence"]; seq != float64(2) {
			t.Errorf("EOB.item[1].sequence = %v (%T), want 2", seq, seq)
		}
	}

	prEntry := entries[1].(map[string]interface{})
	prResource := prEntry["resource"].(map[string]interface{})
	if prResource["resourceType"] != "PaymentReconciliation" {
		t.Errorf("entry[1].resource.resourceType = %v, want PaymentReconciliation", prResource["resourceType"])
	}
	detail, ok := prResource["detail"].([]interface{})
	if !ok || len(detail) != 1 {
		t.Fatalf("expected PaymentReconciliation.detail to have 1 entry, got %v", prResource["detail"])
	}
	responseRef, _ := detail[0].(map[string]interface{})["response"].(map[string]interface{})["reference"].(string)
	eobFullURL, _ := eobEntry["fullUrl"].(string)
	if responseRef != eobFullURL {
		t.Errorf("PaymentReconciliation.detail[0].response.reference = %q, want it rewritten to the EOB's fullUrl %q", responseRef, eobFullURL)
	}

	valid, errs, warns := runStrictValidation(t, bundle)
	t.Logf("strict validation: valid=%v errors=%v warnings=%v", valid, errs, warns)
	assertOnlyNamedEDI835ValidationGaps(t, errs)
}

// TestEDI835ToFHIR_EMedNY_MultiClaim_BuildsOneEOBPerClaim proves the N-claim
// case: eMedNY's sample carries 3 CLP occurrences (per this file's own
// earlier real-sample CLP-count check) -> 3 EOB entries + 1
// PaymentReconciliation, each EOB independently addressable and correctly
// spread by payload.builder's array-valued resourcePaths support.
func TestEDI835ToFHIR_EMedNY_MultiClaim_BuildsOneEOBPerClaim(t *testing.T) {
	bundle, claimCount := build835FHIRBundle(t, "emedny_sample.txt")
	if claimCount != 3 {
		t.Fatalf("expected 3 claims in emedny_sample.txt, got %d", claimCount)
	}

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 4 {
		t.Fatalf("expected 4 bundle entries (3 EOBs + 1 PaymentReconciliation), got %d: %v", len(entries), bundle["entry"])
	}
	eobCount := 0
	for _, e := range entries {
		res := e.(map[string]interface{})["resource"].(map[string]interface{})
		if res["resourceType"] == "ExplanationOfBenefit" {
			eobCount++
			items, _ := res["item"].([]interface{})
			for i, it := range items {
				seq := it.(map[string]interface{})["sequence"]
				if seq != float64(i+1) {
					t.Errorf("EOB %v item[%d].sequence = %v, want %d", res["id"], i, seq, i+1)
				}
			}
		}
	}
	if eobCount != 3 {
		t.Errorf("expected 3 ExplanationOfBenefit entries, got %d", eobCount)
	}

	prResource := entries[3].(map[string]interface{})["resource"].(map[string]interface{})
	detail, ok := prResource["detail"].([]interface{})
	if !ok || len(detail) != 3 {
		t.Fatalf("expected PaymentReconciliation.detail to have 3 entries (one per claim), got %v", prResource["detail"])
	}

	valid, errs, warns := runStrictValidation(t, bundle)
	t.Logf("strict validation: valid=%v errors=%v warnings=%v", valid, errs, warns)
	assertOnlyNamedEDI835ValidationGaps(t, errs)
}
