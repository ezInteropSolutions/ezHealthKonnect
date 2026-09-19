// edi/real_samples_278_834_test.go
// Proves the real, spec-sourced 278/834 schemas parse GENUINE, unedited
// third-party X12 content -- not just this project's own hand-built
// round-trip fixtures. Sourced from X12.org's own official worked examples
// (x12.org/examples/005010x217, x12.org/examples/005010x220 -- the
// standards body's own published educational material, the same low-
// licensing-risk source already used successfully for 837P/837I's own real-
// sample verification). Envelope segments (ISA/GS/GE/IEA) are synthesized
// (the pages show only ST...SE segments); every body segment between ST and
// SE was extracted directly from each page's own raw HTML <p class="data">
// content, not AI-summarized or retyped from memory.
package edi_test

import (
	"os"
	"path/filepath"
	"testing"

	"ezhealthkonnect/edi"
)

func readRealSample(t *testing.T, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(realSchemaDir(t), "..", "..", "testdata", "real_samples", filename))
	if err != nil {
		t.Fatalf("reading real sample %s: %v", filename, err)
	}
	return string(content)
}

// TestRealSamples_278_X12Org_ReferralRequestAndResponse proves X12.org's own
// official "Example 1a/1b: Referral" round trip correctly -- a request with
// NO dependent (2000E nests directly under 2000C, the "subscriber is the
// patient" branch this session's own derive script already handles) and its
// real matching response (HCR present, RD8 date-range DTP).
func TestRealSamples_278_X12Org_ReferralRequestAndResponse(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	t.Run("request", func(t *testing.T) {
		content := readRealSample(t, "x12org_278_1a_referral_request_sample.txt")
		result, err := edi.ParseTransactionSet(spec, content)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
		loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
		loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
		if loop2000C["loops"].(map[string]interface{})["2010C"].(map[string]interface{})["NM1"].(map[string]interface{})["nameLastOrOrganizationName"] != "SMITH" {
			t.Errorf("subscriber NM1 = %#v, want SMITH", loop2000C["loops"])
		}
		// No 2000D (no dependent in this real example) -- the patient event
		// (2000E) sits DIRECTLY under 2000C, the "subscriber is the patient"
		// sibling-of-2000D slot.
		if _, hasDep := loop2000C["loops"].(map[string]interface{})["2000D"]; hasDep {
			t.Errorf("did not expect a 2000D (no dependent in this real example), got %#v", loop2000C["loops"])
		}
		event := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
		if event["HCR"] != nil {
			t.Errorf("request should carry no HCR, got %#v", event["HCR"])
		}
		if event["UM"].(map[string]interface{})["requestCategoryCode"] != "SC" {
			t.Errorf("UM = %#v, want requestCategoryCode=SC (Specialty Care Review -- the REAL code this example uses, not the 'HS' this session's own synthetic fixture guessed)", event["UM"])
		}
		// HI has maxUse=">1" in its own segment definition, so the real
		// parser always array-wraps it, even for a single occurrence.
		if event["HI"].([]map[string]interface{})[0]["codes"].([]map[string]interface{})[0]["code"].(map[string]interface{})["code"] != "41090" {
			t.Errorf("HI = %#v, want diagnosis code 41090", event["HI"])
		}
	})

	t.Run("response", func(t *testing.T) {
		content := readRealSample(t, "x12org_278_1b_referral_response_sample.txt")
		result, err := edi.ParseTransactionSet(spec, content)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
		loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
		loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
		event := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
		if event["HCR"] == nil {
			t.Fatal("response should carry HCR")
		}
		if event["HCR"].(map[string]interface{})["actionCode"] != "A1" {
			t.Errorf("HCR = %#v, want actionCode=A1 (Certified in total)", event["HCR"])
		}
		// DTP*AAH*RD8*20050502-20050602~ -- a REAL RD8 date RANGE (not a
		// single D8 date), proving DTP.json's own datePeriod-as-AN modeling
		// (established for 837's own RD8 usage) also holds for 278.
		dtp := event["DTP"].([]map[string]interface{})[0]
		if dtp["dateTimeQualifier"] != "AAH" || dtp["datePeriod"] != "20050502-20050602" {
			t.Errorf("DTP = %#v, want dateTimeQualifier=AAH datePeriod=20050502-20050602 (a real RD8 range)", dtp)
		}
	})
}

// TestRealSamples_278_X12Org_AdmissionRequestAndResponse proves X12.org's
// own official "Example 2a/2b: Admission for Surgery" round trips -- the
// example that carries a REAL CL1 (Institutional Claim Code) occurrence at
// 2000E, directly confirming the CL1.json usage widening (required ->
// situational) this session made was not just theoretically necessary but
// is exercised by genuine real-world data.
func TestRealSamples_278_X12Org_AdmissionRequestAndResponse(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	t.Run("request", func(t *testing.T) {
		content := readRealSample(t, "x12org_278_2a_admission_request_sample.txt")
		result, err := edi.ParseTransactionSet(spec, content)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
		loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
		loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
		event := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
		if event["CL1"] == nil {
			t.Fatal("expected a real CL1 (Institutional Claim Code) occurrence -- the exact reason this session widened CL1's own usage from required to situational")
		}
		if event["CL1"].(map[string]interface{})["admissionTypeCode"] != "2" {
			t.Errorf("CL1 = %#v, want admissionTypeCode=2", event["CL1"])
		}
		// The facility (NM1*FA) sits at 2010EA alongside its own N3/N4
		// address -- proving 2010EA's own N3/N4 segmentIds are exercised by
		// real data too.
		facilityLoops := event["loops"].(map[string]interface{})["2010EA"].([]map[string]interface{})
		if len(facilityLoops) != 1 || facilityLoops[0]["NM1"].(map[string]interface{})["nameLastOrOrganizationName"] != "MONTGOMERY HOSPITAL" {
			t.Errorf("2010EA = %#v, want MONTGOMERY HOSPITAL", facilityLoops)
		}
		if facilityLoops[0]["N4"].(map[string]interface{})["cityName"] != "ANYTOWN" {
			t.Errorf("2010EA N4 = %#v, want cityName=ANYTOWN", facilityLoops[0]["N4"])
		}
		// A real 2000F (Service Level) with SV2 (institutional service line,
		// the SV2.json usage-widening this session made) and a real PRV.
		svcLevel := event["loops"].(map[string]interface{})["2000F"].([]map[string]interface{})[0]
		if svcLevel["SV2"] == nil {
			t.Fatal("expected a real SV2 (Institutional Service Line) occurrence")
		}
		if svcLevel["SV2"].(map[string]interface{})["procedureCode"].(map[string]interface{})["code"] != "33510" {
			t.Errorf("SV2 = %#v, want procedureCode.code=33510", svcLevel["SV2"])
		}
	})

	t.Run("response", func(t *testing.T) {
		content := readRealSample(t, "x12org_278_2b_admission_response_sample.txt")
		result, err := edi.ParseTransactionSet(spec, content)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
		loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
		loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
		event := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
		if event["HCR"].(map[string]interface{})["actionCode"] != "A6" {
			t.Errorf("event-level HCR = %#v, want actionCode=A6 (Modified) -- genuinely distinct from the service-level HCR", event["HCR"])
		}
		// The response carries a SECOND, genuinely different HCR at the
		// 2000F (service) level (A1, Certified) -- proving HCR is correctly
		// captured at BOTH tree positions independently.
		svcLevel := event["loops"].(map[string]interface{})["2000F"].([]map[string]interface{})[0]
		if svcLevel["HCR"].(map[string]interface{})["actionCode"] != "A1" {
			t.Errorf("service-level HCR = %#v, want actionCode=A1 (Certified), genuinely distinct from the event-level A6 (Modified)", svcLevel["HCR"])
		}
	})
}

// TestRealSamples_278_X12Org_HomeHealthCare proves X12.org's own official
// "Example 4: Request for Home Health Care" -- the ONE real example this
// session found that exercises CR6 (Home Health Care Certification), a
// MULTI-occurrence HI (two diagnosis codes in one HI segment, not two
// separate HI segments), and TWO separate 2000F service levels for a single
// patient event.
func TestRealSamples_278_X12Org_HomeHealthCare(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	content := readRealSample(t, "x12org_278_4_home_health_request_sample.txt")
	result, err := edi.ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
	event := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})

	if event["CR6"] == nil {
		t.Fatal("expected a real CR6 (Home Health Care Certification) occurrence")
	}

	// HI*BF:1831*BF:2630~ -- TWO diagnosis codes within ONE HI segment
	// instance (the repeat-group mechanism proven by 837's own HI usage,
	// now confirmed exercised by 278 too).
	codes := event["HI"].([]map[string]interface{})[0]["codes"].([]map[string]interface{})
	if len(codes) != 2 {
		t.Fatalf("expected 2 diagnosis codes in one HI occurrence, got %d: %+v", len(codes), codes)
	}
	if codes[0]["code"].(map[string]interface{})["code"] != "1831" || codes[1]["code"].(map[string]interface{})["code"] != "2630" {
		t.Errorf("HI codes = %+v, want [1831, 2630]", codes)
	}

	// TWO SEPARATE 2000F Service Level instances for this one patient event
	// -- HCPCS G0154 and B4184, each its own HL.
	svcLevels := event["loops"].(map[string]interface{})["2000F"].([]map[string]interface{})
	if len(svcLevels) != 2 {
		t.Fatalf("expected 2 service levels, got %d", len(svcLevels))
	}
	codesFound := map[string]bool{}
	for _, s := range svcLevels {
		codesFound[s["SV1"].(map[string]interface{})["procedureCode"].(map[string]interface{})["code"].(string)] = true
	}
	if !codesFound["G0154"] || !codesFound["B4184"] {
		t.Errorf("service level procedure codes = %+v, want G0154 and B4184 both present", codesFound)
	}
}

// TestRealSamples_834_X12Org_MultiProductEnrollment proves X12.org's own
// official "Example 01: Enroll Employee in Multiple Health Care Insurance
// Products" -- a SINGLE member with THREE separate 2300 Health Coverage
// occurrences (HLT/DEN/VIS) on the SAME file, the real-world shape this
// session's own "member x coverage" flattening (one Coverage per 2300
// occurrence, not one per member) is specifically designed for.
func TestRealSamples_834_X12Org_MultiProductEnrollment(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	content := readRealSample(t, "x12org_834_1_multi_product_enrollment_sample.txt")
	result, err := edi.ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// 1000B (Payer) uses N1*IN* in EVERY real X12.org 834 example -- the
	// exact discriminator correction this session made after finding this.
	loop1000B := result.Loops["1000B"].(map[string]interface{})
	if loop1000B["N1"].(map[string]interface{})["entityIdentifierCode"] != "IN" {
		t.Errorf("1000B N1 = %#v, want entityIdentifierCode=IN", loop1000B["N1"])
	}

	members := result.Loops["2000"].([]map[string]interface{})
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	member := members[0]
	if member["INS"].(map[string]interface{})["maintenanceTypeCode"] != "021" {
		t.Errorf("INS = %#v, want maintenanceTypeCode=021 (Addition)", member["INS"])
	}
	// INS08 (employmentStatusCode) = "FT" -- confirms the INS.json position
	// 6-17 extension this session added is reachable by real data.
	if member["INS"].(map[string]interface{})["employmentStatusCode"] != "FT" {
		t.Errorf("INS = %#v, want employmentStatusCode=FT (the extended INS position this session added)", member["INS"])
	}

	coverages := member["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(coverages) != 3 {
		t.Fatalf("expected 3 real Health Coverage occurrences (HLT/DEN/VIS), got %d", len(coverages))
	}
	lines := map[string]bool{}
	for _, c := range coverages {
		lines[c["HD"].(map[string]interface{})["insuranceLineCode"].(string)] = true
	}
	if !lines["HLT"] || !lines["DEN"] || !lines["VIS"] {
		t.Errorf("insurance line codes = %+v, want HLT, DEN, and VIS all present", lines)
	}

	// A real COB (Coordination of Benefits) occurrence nested at the first
	// coverage's own 2320 child loop (COB is not a direct 2300 segment --
	// it lives one level deeper).
	cob2320, ok := coverages[0]["loops"].(map[string]interface{})["2320"].([]map[string]interface{})
	if !ok || len(cob2320) != 1 || cob2320[0]["COB"] == nil {
		t.Errorf("expected a real 2320/COB occurrence nested under the first (HLT) coverage, got loops=%#v", coverages[0]["loops"])
	}
}

// TestRealSamples_834_X12Org_AddDependentStudent proves X12.org's own
// official "Example 02: Add a Dependent (Full-Time Student)" -- the ONE real
// example this session found that exercises 2100E (Member School), whose
// own real discriminator ('M8') this session corrected after finding it was
// wrongly guessed as '83' during initial schema authoring.
func TestRealSamples_834_X12Org_AddDependentStudent(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	content := readRealSample(t, "x12org_834_2_add_dependent_student_sample.txt")
	result, err := edi.ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	members := result.Loops["2000"].([]map[string]interface{})
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	member := members[0]
	if member["INS"].(map[string]interface{})["individualRelationshipCode"] != "19" {
		t.Errorf("INS = %#v, want individualRelationshipCode=19 (child)", member["INS"])
	}
	// INS09 (studentStatusCode) = "F" (full-time) -- another real exercise
	// of the extended INS positions.
	if member["INS"].(map[string]interface{})["studentStatusCode"] != "F" {
		t.Errorf("INS = %#v, want studentStatusCode=F (full-time)", member["INS"])
	}

	school := member["loops"].(map[string]interface{})["2100E"].([]map[string]interface{})
	if len(school) != 1 {
		t.Fatalf("expected the real 2100E (Member School) loop to resolve via its own 'M8' discriminator, got %d instances", len(school))
	}
	if school[0]["NM1"].(map[string]interface{})["nameLastOrOrganizationName"] != "PENN STATE UNIVERSITY" {
		t.Errorf("2100E NM1 = %#v, want PENN STATE UNIVERSITY", school[0]["NM1"])
	}
}

// TestRealSamples_834_X12Org_TerminateEligibility proves X12.org's own
// official "Example 07: Terminate Eligibility for a Subscriber" -- the
// sample that caught this session's own maintenanceTypeCode->Coverage.status
// mapping bug (an earlier version mapped "030" to cancelled and "024" to
// active -- genuinely backwards; the REAL X12 element 875 standard code for
// Cancellation/Termination is "024", and "030" is "Audit or Compare", not a
// termination code at all).
func TestRealSamples_834_X12Org_TerminateEligibility(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	content := readRealSample(t, "x12org_834_7_terminate_eligibility_sample.txt")
	result, err := edi.ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	members := result.Loops["2000"].([]map[string]interface{})
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	ins := members[0]["INS"].(map[string]interface{})
	if ins["maintenanceTypeCode"] != "024" {
		t.Errorf("INS = %#v, want maintenanceTypeCode=024 (Cancellation/Termination -- the REAL code, confirmed by this file's own title), not the '030' an earlier version of this schema's own derive script incorrectly treated as termination", ins)
	}
}
